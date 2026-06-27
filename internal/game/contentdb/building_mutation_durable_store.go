package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
)

// BuildingMutationDurableStore is the Postgres-backed durable adapter for one
// committed planet building mutation. It persists the validated
// production.BuildingMutationDurableCommitPlan bundle and resolves idempotent
// replays versus reference conflicts the same way the in-memory default does,
// but across process restarts.
type BuildingMutationDurableStore struct {
	store *Store
}

var (
	_ production.BuildingMutationDurableCommitStore  = (*BuildingMutationDurableStore)(nil)
	_ production.BuildingMutationDurableCommitReader = (*BuildingMutationDurableStore)(nil)
)

func NewBuildingMutationDurableStore(store *Store) (*BuildingMutationDurableStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &BuildingMutationDurableStore{store: store}, nil
}

func (s *BuildingMutationDurableStore) ApplyBuildingMutationDurableCommitPlan(
	plan production.BuildingMutationDurableCommitPlan,
) (result production.BuildingMutationDurableCommitResult, err error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return production.BuildingMutationDurableCommitResult{}, ErrNilDatabase
	}
	referenceKey := plan.Reference.ReferenceKey
	if referenceKey == "" {
		return production.BuildingMutationDurableCommitResult{}, nil
	}
	planJSON, mErr := marshalBuildingMutationDurablePlan(plan)
	if mErr != nil {
		return production.BuildingMutationDurableCommitResult{}, mErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	tx, txErr := s.store.db.BeginTx(ctx, nil)
	if txErr != nil {
		return production.BuildingMutationDurableCommitResult{}, txErr
	}
	defer rollbackUnlessCommitted(tx, &err)

	existing, ok, lErr := loadBuildingMutationDurablePlanLocked(ctx, tx, referenceKey)
	if lErr != nil {
		return production.BuildingMutationDurableCommitResult{}, lErr
	}
	if ok {
		existingJSON, mErr := marshalBuildingMutationDurablePlan(existing)
		if mErr != nil {
			return production.BuildingMutationDurableCommitResult{}, mErr
		}
		if !claimDurableLifecyclePlanJSONEqual(planJSON, existingJSON) {
			return production.BuildingMutationDurableCommitResult{}, fmt.Errorf("building_mutation_reference_conflict: %w", production.ErrInvalidBuildingMutationDurableCommit)
		}
		if err = tx.Commit(); err != nil {
			return production.BuildingMutationDurableCommitResult{}, err
		}
		return buildingMutationDurableResultFromPlan(existing, true), nil
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO building_mutation_durable_commits(reference_key, plan_json)
		VALUES ($1, $2::jsonb)
	`, string(referenceKey), string(planJSON)); err != nil {
		return production.BuildingMutationDurableCommitResult{}, err
	}

	if err = tx.Commit(); err != nil {
		return production.BuildingMutationDurableCommitResult{}, err
	}
	return buildingMutationDurableResultFromPlan(plan, false), nil
}

func (s *BuildingMutationDurableStore) CommittedBuildingMutationDurableCommitPlan(
	referenceKey foundation.IdempotencyKey,
) (production.BuildingMutationDurableCommitPlan, bool, error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return production.BuildingMutationDurableCommitPlan{}, false, ErrNilDatabase
	}
	if err := referenceKey.Validate(); err != nil {
		return production.BuildingMutationDurableCommitPlan{}, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	return loadBuildingMutationDurablePlan(ctx, s.store.db, referenceKey)
}

func (s *BuildingMutationDurableStore) CommittedBuildingMutationOutboxDispatchPlan(
	referenceKey foundation.IdempotencyKey,
) (production.BuildingMutationOutboxDispatchPlan, bool, error) {
	plan, ok, err := s.CommittedBuildingMutationDurableCommitPlan(referenceKey)
	if err != nil || !ok {
		return production.BuildingMutationOutboxDispatchPlan{}, ok, err
	}
	dispatch, err := production.NewBuildingMutationOutboxDispatchPlan(&plan.Reference, plan.OutboxRecords)
	if err != nil {
		return production.BuildingMutationOutboxDispatchPlan{}, false, err
	}
	return dispatch, true, nil
}

// BuildingMutationReferences returns committed building mutation references in
// commit order for diagnostics and parity with the in-memory runtime adapter.
func (s *BuildingMutationDurableStore) BuildingMutationReferences() []production.BuildingMutationReferenceRecord {
	plans, err := s.buildingMutationDurablePlans()
	if err != nil {
		return nil
	}
	records := make([]production.BuildingMutationReferenceRecord, 0, len(plans))
	for _, plan := range plans {
		records = append(records, plan.Reference)
	}
	return records
}

// OutboxRecords returns committed production outbox rows in commit order for
// diagnostics and parity with the in-memory runtime adapter.
func (s *BuildingMutationDurableStore) OutboxRecords() []production.ProductionOutboxRecord {
	plans, err := s.buildingMutationDurablePlans()
	if err != nil {
		return nil
	}
	records := make([]production.ProductionOutboxRecord, 0)
	for _, plan := range plans {
		records = append(records, plan.OutboxRecords...)
	}
	return records
}

// BuildingMaterialLedgerEntries returns committed material ledger rows in
// commit order for diagnostics and parity with the in-memory runtime adapter.
func (s *BuildingMutationDurableStore) BuildingMaterialLedgerEntries() []production.BuildingMaterialLedgerEntry {
	plans, err := s.buildingMutationDurablePlans()
	if err != nil {
		return nil
	}
	entries := make([]production.BuildingMaterialLedgerEntry, 0)
	for _, plan := range plans {
		entries = append(entries, plan.MaterialLedger...)
	}
	return entries
}

func (s *BuildingMutationDurableStore) ClaimPendingProductionOutboxRecords(
	limit int,
	claimedAt time.Time,
) ([]production.ProductionOutboxRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	claimedAt = claimedAt.UTC()
	if claimedAt.IsZero() {
		claimedAt = time.Unix(0, 0).UTC()
	}
	return s.mutateBuildingMutationOutboxRecords(limit, func(record *production.ProductionOutboxRecord) (bool, bool) {
		if record.Status != production.ProductionOutboxStatusPending {
			return false, false
		}
		record.Status = production.ProductionOutboxStatusInFlight
		record.ClaimedAt = claimedAt
		record.Attempts++
		record.ClaimToken = fmt.Sprintf("%s-attempt-%d", record.OutboxID, record.Attempts)
		return true, true
	})
}

func (s *BuildingMutationDurableStore) MarkProductionOutboxPublished(
	outboxID string,
	claimToken string,
	publishedAt time.Time,
) (production.ProductionOutboxRecord, bool, error) {
	records, err := s.mutateBuildingMutationOutboxRecords(1, func(record *production.ProductionOutboxRecord) (bool, bool) {
		if record.OutboxID != outboxID {
			return false, false
		}
		if record.Status != production.ProductionOutboxStatusInFlight || record.ClaimToken == "" || record.ClaimToken != claimToken {
			return false, false
		}
		record.Status = production.ProductionOutboxStatusPublished
		record.PublishedAt = publishedAt.UTC()
		record.FailedAt = time.Time{}
		record.LastError = ""
		return true, true
	})
	if err != nil || len(records) == 0 {
		return production.ProductionOutboxRecord{}, false, err
	}
	return records[0], true, nil
}

func (s *BuildingMutationDurableStore) MarkProductionOutboxFailed(
	outboxID string,
	claimToken string,
	reason string,
	failedAt time.Time,
) (production.ProductionOutboxRecord, bool, error) {
	records, err := s.mutateBuildingMutationOutboxRecords(1, func(record *production.ProductionOutboxRecord) (bool, bool) {
		if record.OutboxID != outboxID {
			return false, false
		}
		if record.Status != production.ProductionOutboxStatusInFlight || record.ClaimToken == "" || record.ClaimToken != claimToken {
			return false, false
		}
		record.Status = production.ProductionOutboxStatusFailed
		record.FailedAt = failedAt.UTC()
		record.LastError = reason
		return true, true
	})
	if err != nil || len(records) == 0 {
		return production.ProductionOutboxRecord{}, false, err
	}
	return records[0], true, nil
}

func (s *BuildingMutationDurableStore) ReleaseExpiredProductionOutboxRecords(
	limit int,
	claimedBefore time.Time,
	releasedAt time.Time,
) ([]production.ProductionOutboxRecord, error) {
	if limit <= 0 || claimedBefore.IsZero() {
		return nil, nil
	}
	claimedBefore = claimedBefore.UTC()
	releasedAt = releasedAt.UTC()
	return s.mutateBuildingMutationOutboxRecords(limit, func(record *production.ProductionOutboxRecord) (bool, bool) {
		if record.Status != production.ProductionOutboxStatusInFlight ||
			record.ClaimedAt.IsZero() ||
			!record.ClaimedAt.Before(claimedBefore) {
			return false, false
		}
		record.Status = production.ProductionOutboxStatusPending
		record.ClaimedAt = time.Time{}
		record.ClaimToken = ""
		record.RetriedAt = releasedAt
		return true, true
	})
}

func (s *BuildingMutationDurableStore) RetryFailedProductionOutboxRecords(
	limit int,
	retriedAt time.Time,
) ([]production.ProductionOutboxRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	retriedAt = retriedAt.UTC()
	return s.mutateBuildingMutationOutboxRecords(limit, func(record *production.ProductionOutboxRecord) (bool, bool) {
		if record.Status != production.ProductionOutboxStatusFailed {
			return false, false
		}
		record.Status = production.ProductionOutboxStatusPending
		record.ClaimedAt = time.Time{}
		record.ClaimToken = ""
		record.RetriedAt = retriedAt
		return true, true
	})
}

func (s *BuildingMutationDurableStore) mutateBuildingMutationOutboxRecords(
	limit int,
	mutate func(*production.ProductionOutboxRecord) (bool, bool),
) (records []production.ProductionOutboxRecord, err error) {
	if s == nil || s.store == nil || s.store.db == nil || mutate == nil {
		return nil, production.ErrInvalidProductionOutboxPublisher
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	tx, txErr := s.store.db.BeginTx(ctx, nil)
	if txErr != nil {
		return nil, txErr
	}
	defer rollbackUnlessCommitted(tx, &err)

	rows, qErr := tx.QueryContext(ctx, `
		SELECT reference_key, plan_json FROM building_mutation_durable_commits
		ORDER BY committed_at, reference_key
		FOR UPDATE
	`)
	if qErr != nil {
		return nil, qErr
	}
	type mutatedPlan struct {
		referenceKey foundation.IdempotencyKey
		plan         production.BuildingMutationDurableCommitPlan
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
		var plan production.BuildingMutationDurableCommitPlan
		if err := json.Unmarshal([]byte(raw), &plan); err != nil {
			_ = rows.Close()
			return nil, err
		}
		changed := false
		for index := range plan.OutboxRecords {
			if limit > 0 && len(records) >= limit {
				break
			}
			include, recordChanged := mutate(&plan.OutboxRecords[index])
			if include {
				records = append(records, plan.OutboxRecords[index])
			}
			if recordChanged {
				changed = true
			}
		}
		if changed {
			mutated = append(mutated, mutatedPlan{
				referenceKey: foundation.IdempotencyKey(referenceRaw),
				plan:         plan,
			})
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, entry := range mutated {
		planJSON, err := marshalBuildingMutationDurablePlan(entry.plan)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE building_mutation_durable_commits
			SET plan_json = $2::jsonb
			WHERE reference_key = $1
		`, string(entry.referenceKey), string(planJSON)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *BuildingMutationDurableStore) buildingMutationDurablePlans() ([]production.BuildingMutationDurableCommitPlan, error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return nil, ErrNilDatabase
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	rows, err := s.store.db.QueryContext(ctx, `
		SELECT plan_json FROM building_mutation_durable_commits ORDER BY committed_at, reference_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plans := make([]production.BuildingMutationDurableCommitPlan, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var plan production.BuildingMutationDurableCommitPlan
		if err := json.Unmarshal([]byte(raw), &plan); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

func loadBuildingMutationDurablePlan(
	ctx context.Context,
	runner queryContextRunner,
	referenceKey foundation.IdempotencyKey,
) (production.BuildingMutationDurableCommitPlan, bool, error) {
	row := runner.QueryRowContext(ctx, `
		SELECT plan_json FROM building_mutation_durable_commits WHERE reference_key = $1
	`, string(referenceKey))
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return production.BuildingMutationDurableCommitPlan{}, false, nil
		}
		return production.BuildingMutationDurableCommitPlan{}, false, err
	}
	var plan production.BuildingMutationDurableCommitPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return production.BuildingMutationDurableCommitPlan{}, false, err
	}
	return plan, true, nil
}

func loadBuildingMutationDurablePlanLocked(
	ctx context.Context,
	tx *sql.Tx,
	referenceKey foundation.IdempotencyKey,
) (production.BuildingMutationDurableCommitPlan, bool, error) {
	return loadBuildingMutationDurablePlan(ctx, tx, referenceKey)
}

func marshalBuildingMutationDurablePlan(plan production.BuildingMutationDurableCommitPlan) (json.RawMessage, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("marshal building mutation durable plan: %w", err)
	}
	return data, nil
}

func buildingMutationDurableResultFromPlan(plan production.BuildingMutationDurableCommitPlan, duplicate bool) production.BuildingMutationDurableCommitResult {
	reference := plan.Reference
	return production.BuildingMutationDurableCommitResult{
		Reference:      &reference,
		OutboxRecords:  plan.OutboxRecords,
		MaterialLedger: plan.MaterialLedger,
		Duplicate:      duplicate,
	}
}
