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
