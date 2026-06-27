package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

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
