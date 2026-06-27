package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/discovery"
)

// claimLifecycleStoreTimeout bounds each Postgres lifecycle read/write so a
// stalled connection cannot block the caller indefinitely. The discovery
// durable store contract has no context parameter, so the adapter owns the
// deadline.
const claimLifecycleStoreTimeout = 15 * time.Second

// queryContextRunner is the minimal DB surface that satisfies both *sql.DB
// and *sql.Tx for QueryRowContext reads used by the claim lifecycle adapter.
type queryContextRunner interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ClaimDurableLifecycleStore is the Postgres-backed durable adapter for one
// committed planet-claim lifecycle. It persists the validated
// discovery.ClaimDurableLifecyclePlan bundle and resolves idempotent replays
// (same plan bytes → duplicate) versus reference conflicts (same
// claim_reference, different plan → rejected) the same way the in-memory
// default does, but across process restarts.
type ClaimDurableLifecycleStore struct {
	store *Store
}

// Ensure the Postgres adapter satisfies the discovery durable lifecycle store
// and reader contracts expected by the runtime.
var (
	_ discovery.ClaimDurableLifecycleStore  = (*ClaimDurableLifecycleStore)(nil)
	_ discovery.ClaimDurableLifecycleReader = (*ClaimDurableLifecycleStore)(nil)
)

// NewClaimDurableLifecycleStore returns a Postgres-backed claim lifecycle
// adapter bound to store.
func NewClaimDurableLifecycleStore(store *Store) (*ClaimDurableLifecycleStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &ClaimDurableLifecycleStore{store: store}, nil
}

// ApplyClaimDurableLifecyclePlan atomically commits one non-empty claim
// lifecycle plan. Empty plans are no-ops; exact replays (same serialized plan
// bytes for the same claim reference) return the committed plan with Duplicate
// set; conflicting plan reuse for an existing claim reference is rejected
// before any mutation.
func (s *ClaimDurableLifecycleStore) ApplyClaimDurableLifecyclePlan(
	plan discovery.ClaimDurableLifecyclePlan,
) (result discovery.ClaimDurableLifecycleResult, err error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return discovery.ClaimDurableLifecycleResult{}, ErrNilDatabase
	}
	reference := plan.Commit.Boundary.ClaimReference
	if reference == "" {
		return discovery.ClaimDurableLifecycleResult{}, nil
	}
	planJSON, mErr := marshalClaimDurableLifecyclePlan(plan)
	if mErr != nil {
		return discovery.ClaimDurableLifecycleResult{}, mErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	tx, txErr := s.store.db.BeginTx(ctx, nil)
	if txErr != nil {
		return discovery.ClaimDurableLifecycleResult{}, txErr
	}
	defer rollbackUnlessCommitted(tx, &err)

	existing, ok, lErr := loadClaimDurableLifecyclePlanLocked(ctx, tx, reference)
	if lErr != nil {
		return discovery.ClaimDurableLifecycleResult{}, lErr
	}
	if ok {
		existingJSON, mErr := marshalClaimDurableLifecyclePlan(existing)
		if mErr != nil {
			return discovery.ClaimDurableLifecycleResult{}, mErr
		}
		if !claimDurableLifecyclePlanJSONEqual(planJSON, existingJSON) {
			return discovery.ClaimDurableLifecycleResult{}, fmt.Errorf("claim_reference_conflict: %w", discovery.ErrInvalidClaimDurableCommit)
		}
		if err = tx.Commit(); err != nil {
			return discovery.ClaimDurableLifecycleResult{}, err
		}
		return discovery.ClaimDurableLifecycleResult{Plan: existing, Duplicate: true}, nil
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO claim_durable_lifecycles(claim_reference, player_id, planet_id, plan_json)
		VALUES ($1, $2, $3, $4::jsonb)
	`, string(reference),
		string(plan.Commit.Boundary.PlayerID),
		string(plan.Commit.Boundary.PlanetID),
		string(planJSON)); err != nil {
		return discovery.ClaimDurableLifecycleResult{}, err
	}

	if err = tx.Commit(); err != nil {
		return discovery.ClaimDurableLifecycleResult{}, err
	}
	return discovery.ClaimDurableLifecycleResult{Plan: plan}, nil
}

// CommittedClaimDurableLifecyclePlan returns the validated committed lifecycle
// plan for one claim reference.
func (s *ClaimDurableLifecycleStore) CommittedClaimDurableLifecyclePlan(
	reference discovery.PlanetClaimReference,
) (discovery.ClaimDurableLifecyclePlan, bool, error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return discovery.ClaimDurableLifecyclePlan{}, false, ErrNilDatabase
	}
	if err := reference.Validate(); err != nil {
		return discovery.ClaimDurableLifecyclePlan{}, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	return loadClaimDurableLifecyclePlan(ctx, s.store.db, reference)
}

// CommittedClaimOutboxDispatchPlan returns the publisher dispatch handoff for
// one committed claim reference, derived from its committed outbox row.
func (s *ClaimDurableLifecycleStore) CommittedClaimOutboxDispatchPlan(
	reference discovery.PlanetClaimReference,
) (discovery.ClaimOutboxDispatchPlan, bool, error) {
	plan, ok, err := s.CommittedClaimDurableLifecyclePlan(reference)
	if err != nil || !ok {
		return discovery.ClaimOutboxDispatchPlan{}, ok, err
	}
	dispatch, err := discovery.NewClaimOutboxDispatchPlan(&plan.Commit.Reference, &plan.Commit.Outbox)
	if err != nil {
		return discovery.ClaimOutboxDispatchPlan{}, false, err
	}
	return dispatch, true, nil
}

// ClaimReferences returns committed claim references in commit order for
// diagnostics and parity with the in-memory runtime adapter.
func (s *ClaimDurableLifecycleStore) ClaimReferences() []discovery.PlanetClaimReference {
	if s == nil || s.store == nil || s.store.db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	rows, err := s.store.db.QueryContext(ctx, `
		SELECT claim_reference FROM claim_durable_lifecycles ORDER BY committed_at, claim_reference
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	references := make([]discovery.PlanetClaimReference, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil
		}
		references = append(references, discovery.PlanetClaimReference(raw))
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return references
}

// OutboxRecords returns committed claim outbox rows in commit order for
// diagnostics and parity with the in-memory runtime adapter.
func (s *ClaimDurableLifecycleStore) OutboxRecords() []discovery.ClaimOutboxRecord {
	plans, err := s.claimDurableLifecyclePlans()
	if err != nil {
		return nil
	}
	records := make([]discovery.ClaimOutboxRecord, 0, len(plans))
	for _, plan := range plans {
		records = append(records, plan.Commit.Outbox)
	}
	return records
}

func (s *ClaimDurableLifecycleStore) ClaimPendingClaimOutboxRecordsForPublish(
	limit int,
	claimedAt time.Time,
) ([]discovery.ClaimOutboxRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	claimedAt = claimedAt.UTC()
	if claimedAt.IsZero() {
		claimedAt = time.Unix(0, 0).UTC()
	}
	return s.mutateClaimOutboxRecords(limit, func(record *discovery.ClaimOutboxRecord) (bool, bool) {
		if record.Status != discovery.ClaimOutboxStatusPending {
			return false, false
		}
		record.Status = discovery.ClaimOutboxStatusInFlight
		record.ClaimedAt = claimedAt
		record.Attempts++
		record.ClaimToken = fmt.Sprintf("%s-attempt-%d", record.OutboxID, record.Attempts)
		return true, true
	})
}

func (s *ClaimDurableLifecycleStore) MarkClaimOutboxPublished(
	outboxID string,
	claimToken string,
	publishedAt time.Time,
) (discovery.ClaimOutboxRecord, bool, error) {
	records, err := s.mutateClaimOutboxRecords(1, func(record *discovery.ClaimOutboxRecord) (bool, bool) {
		if record.OutboxID != outboxID {
			return false, false
		}
		if record.Status != discovery.ClaimOutboxStatusInFlight || record.ClaimToken == "" || record.ClaimToken != claimToken {
			return false, false
		}
		record.Status = discovery.ClaimOutboxStatusPublished
		record.PublishedAt = publishedAt.UTC()
		record.FailedAt = time.Time{}
		record.LastError = ""
		return true, true
	})
	if err != nil || len(records) == 0 {
		return discovery.ClaimOutboxRecord{}, false, err
	}
	return records[0], true, nil
}

func (s *ClaimDurableLifecycleStore) MarkClaimOutboxFailed(
	outboxID string,
	claimToken string,
	reason string,
	failedAt time.Time,
) (discovery.ClaimOutboxRecord, bool, error) {
	records, err := s.mutateClaimOutboxRecords(1, func(record *discovery.ClaimOutboxRecord) (bool, bool) {
		if record.OutboxID != outboxID {
			return false, false
		}
		if record.Status != discovery.ClaimOutboxStatusInFlight || record.ClaimToken == "" || record.ClaimToken != claimToken {
			return false, false
		}
		record.Status = discovery.ClaimOutboxStatusFailed
		record.FailedAt = failedAt.UTC()
		record.LastError = reason
		return true, true
	})
	if err != nil || len(records) == 0 {
		return discovery.ClaimOutboxRecord{}, false, err
	}
	return records[0], true, nil
}

func (s *ClaimDurableLifecycleStore) ReleaseExpiredClaimOutboxRecordsForPublish(
	limit int,
	claimedBefore time.Time,
	releasedAt time.Time,
) ([]discovery.ClaimOutboxRecord, error) {
	if limit <= 0 || claimedBefore.IsZero() {
		return nil, nil
	}
	claimedBefore = claimedBefore.UTC()
	releasedAt = releasedAt.UTC()
	return s.mutateClaimOutboxRecords(limit, func(record *discovery.ClaimOutboxRecord) (bool, bool) {
		if record.Status != discovery.ClaimOutboxStatusInFlight ||
			record.ClaimedAt.IsZero() ||
			!record.ClaimedAt.Before(claimedBefore) {
			return false, false
		}
		record.Status = discovery.ClaimOutboxStatusPending
		record.ClaimedAt = time.Time{}
		record.ClaimToken = ""
		record.RetriedAt = releasedAt
		return true, true
	})
}

func (s *ClaimDurableLifecycleStore) RetryFailedClaimOutboxRecordsForPublish(
	limit int,
	retriedAt time.Time,
) ([]discovery.ClaimOutboxRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	retriedAt = retriedAt.UTC()
	return s.mutateClaimOutboxRecords(limit, func(record *discovery.ClaimOutboxRecord) (bool, bool) {
		if record.Status != discovery.ClaimOutboxStatusFailed {
			return false, false
		}
		record.Status = discovery.ClaimOutboxStatusPending
		record.ClaimedAt = time.Time{}
		record.ClaimToken = ""
		record.RetriedAt = retriedAt
		return true, true
	})
}

func (s *ClaimDurableLifecycleStore) mutateClaimOutboxRecords(
	limit int,
	mutate func(*discovery.ClaimOutboxRecord) (bool, bool),
) (records []discovery.ClaimOutboxRecord, err error) {
	if s == nil || s.store == nil || s.store.db == nil || mutate == nil {
		return nil, discovery.ErrInvalidClaimOutboxPublisher
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	tx, txErr := s.store.db.BeginTx(ctx, nil)
	if txErr != nil {
		return nil, txErr
	}
	defer rollbackUnlessCommitted(tx, &err)

	rows, qErr := tx.QueryContext(ctx, `
		SELECT claim_reference, plan_json FROM claim_durable_lifecycles
		ORDER BY committed_at, claim_reference
		FOR UPDATE
	`)
	if qErr != nil {
		return nil, qErr
	}
	type mutatedPlan struct {
		reference discovery.PlanetClaimReference
		plan      discovery.ClaimDurableLifecyclePlan
	}
	mutated := make([]mutatedPlan, 0)
	for rows.Next() {
		if limit > 0 && len(records) >= limit {
			break
		}
		var referenceRaw string
		var raw string
		if err := rows.Scan(&referenceRaw, &raw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		var plan discovery.ClaimDurableLifecyclePlan
		if err := json.Unmarshal([]byte(raw), &plan); err != nil {
			_ = rows.Close()
			return nil, err
		}
		include, changed := mutate(&plan.Commit.Outbox)
		if include {
			records = append(records, plan.Commit.Outbox)
		}
		if changed {
			mutated = append(mutated, mutatedPlan{reference: discovery.PlanetClaimReference(referenceRaw), plan: plan})
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, entry := range mutated {
		planJSON, err := marshalClaimDurableLifecyclePlan(entry.plan)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE claim_durable_lifecycles
			SET plan_json = $2::jsonb
			WHERE claim_reference = $1
		`, string(entry.reference), string(planJSON)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *ClaimDurableLifecycleStore) claimDurableLifecyclePlans() ([]discovery.ClaimDurableLifecyclePlan, error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return nil, ErrNilDatabase
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	rows, err := s.store.db.QueryContext(ctx, `
		SELECT plan_json FROM claim_durable_lifecycles ORDER BY committed_at, claim_reference
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plans := make([]discovery.ClaimDurableLifecyclePlan, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var plan discovery.ClaimDurableLifecyclePlan
		if err := json.Unmarshal([]byte(raw), &plan); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

func loadClaimDurableLifecyclePlan(
	ctx context.Context,
	runner queryContextRunner,
	reference discovery.PlanetClaimReference,
) (discovery.ClaimDurableLifecyclePlan, bool, error) {
	row := runner.QueryRowContext(ctx, `
		SELECT plan_json FROM claim_durable_lifecycles WHERE claim_reference = $1
	`, string(reference))
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return discovery.ClaimDurableLifecyclePlan{}, false, nil
		}
		return discovery.ClaimDurableLifecyclePlan{}, false, err
	}
	var plan discovery.ClaimDurableLifecyclePlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return discovery.ClaimDurableLifecyclePlan{}, false, err
	}
	return plan, true, nil
}

func loadClaimDurableLifecyclePlanLocked(
	ctx context.Context,
	tx *sql.Tx,
	reference discovery.PlanetClaimReference,
) (discovery.ClaimDurableLifecyclePlan, bool, error) {
	return loadClaimDurableLifecyclePlan(ctx, tx, reference)
}

func marshalClaimDurableLifecyclePlan(plan discovery.ClaimDurableLifecyclePlan) (json.RawMessage, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("marshal claim durable lifecycle plan: %w", err)
	}
	return data, nil
}

func claimDurableLifecyclePlanJSONEqual(left, right json.RawMessage) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	lc, err1 := canonicalClaimDurableLifecycleJSON(left)
	rc, err2 := canonicalClaimDurableLifecycleJSON(right)
	if err1 != nil || err2 != nil {
		return false
	}
	return lc == rc
}

// canonicalClaimDurableLifecycleJSON round-trips raw JSON through an untyped
// value and back so map key ordering is normalized for deterministic
// comparison.
func canonicalClaimDurableLifecycleJSON(raw json.RawMessage) (string, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(canonical), nil
}
