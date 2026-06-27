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

// SettlementDurableStore is the Postgres-backed durable adapter for one
// committed production/route settlement. It persists the validated
// production.SettlementDurableCommitPlan bundle and resolves idempotent
// replays versus reference conflicts the same way the in-memory default does,
// but across process restarts. Planet/route-window lookups scan committed
// plans and match the settlement reference fields.
type SettlementDurableStore struct {
	store *Store
}

var (
	_ production.SettlementDurableCommitStore  = (*SettlementDurableStore)(nil)
	_ production.SettlementDurableCommitReader = (*SettlementDurableStore)(nil)
)

func NewSettlementDurableStore(store *Store) (*SettlementDurableStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &SettlementDurableStore{store: store}, nil
}

func (s *SettlementDurableStore) ApplySettlementDurableCommitPlan(
	plan production.SettlementDurableCommitPlan,
) (result production.SettlementDurableCommitResult, err error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return production.SettlementDurableCommitResult{}, ErrNilDatabase
	}
	referenceKey := plan.Reference.ReferenceKey
	if referenceKey == "" {
		return production.SettlementDurableCommitResult{}, nil
	}
	planJSON, mErr := marshalSettlementDurablePlan(plan)
	if mErr != nil {
		return production.SettlementDurableCommitResult{}, mErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	tx, txErr := s.store.db.BeginTx(ctx, nil)
	if txErr != nil {
		return production.SettlementDurableCommitResult{}, txErr
	}
	defer rollbackUnlessCommitted(tx, &err)

	existing, ok, lErr := loadSettlementDurablePlanLocked(ctx, tx, referenceKey)
	if lErr != nil {
		return production.SettlementDurableCommitResult{}, lErr
	}
	if ok {
		existingJSON, mErr := marshalSettlementDurablePlan(existing)
		if mErr != nil {
			return production.SettlementDurableCommitResult{}, mErr
		}
		if !claimDurableLifecyclePlanJSONEqual(planJSON, existingJSON) {
			return production.SettlementDurableCommitResult{}, fmt.Errorf("settlement_reference_conflict: %w", production.ErrInvalidSettlementDurableCommit)
		}
		if err = tx.Commit(); err != nil {
			return production.SettlementDurableCommitResult{}, err
		}
		return settlementDurableResultFromPlan(existing, true), nil
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO settlement_durable_commits(reference_key, plan_json)
		VALUES ($1, $2::jsonb)
	`, string(referenceKey), string(planJSON)); err != nil {
		return production.SettlementDurableCommitResult{}, err
	}

	if err = tx.Commit(); err != nil {
		return production.SettlementDurableCommitResult{}, err
	}
	return settlementDurableResultFromPlan(plan, false), nil
}

func (s *SettlementDurableStore) CommittedSettlementDurableCommitPlan(
	referenceKey foundation.IdempotencyKey,
) (production.SettlementDurableCommitPlan, bool, error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return production.SettlementDurableCommitPlan{}, false, ErrNilDatabase
	}
	if err := referenceKey.Validate(); err != nil {
		return production.SettlementDurableCommitPlan{}, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	return loadSettlementDurablePlan(ctx, s.store.db, referenceKey)
}

func (s *SettlementDurableStore) CommittedSettlementOutboxDispatchPlan(
	referenceKey foundation.IdempotencyKey,
) (production.SettlementOutboxDispatchPlan, bool, error) {
	plan, ok, err := s.CommittedSettlementDurableCommitPlan(referenceKey)
	if err != nil || !ok {
		return production.SettlementOutboxDispatchPlan{}, ok, err
	}
	dispatch, err := production.NewSettlementOutboxDispatchPlan(&plan.Reference, plan.Outbox.OutboxRecords)
	if err != nil {
		return production.SettlementOutboxDispatchPlan{}, false, err
	}
	return dispatch, true, nil
}

func (s *SettlementDurableStore) CommittedProductionSettlementDurableCommitPlan(
	planetID foundation.PlanetID,
	window string,
) (production.SettlementDurableCommitPlan, bool, error) {
	return s.findSettlementDurablePlan(func(plan production.SettlementDurableCommitPlan) bool {
		return plan.Reference.PlanetID == planetID && plan.Reference.SettlementWindow == window
	})
}

func (s *SettlementDurableStore) CommittedProductionSettlementOutboxDispatchPlan(
	planetID foundation.PlanetID,
	window string,
) (production.SettlementOutboxDispatchPlan, bool, error) {
	plan, ok, err := s.CommittedProductionSettlementDurableCommitPlan(planetID, window)
	if err != nil || !ok {
		return production.SettlementOutboxDispatchPlan{}, ok, err
	}
	dispatch, err := production.NewSettlementOutboxDispatchPlan(&plan.Reference, plan.Outbox.OutboxRecords)
	if err != nil {
		return production.SettlementOutboxDispatchPlan{}, false, err
	}
	return dispatch, true, nil
}

func (s *SettlementDurableStore) CommittedRouteSettlementDurableCommitPlan(
	routeID foundation.RouteID,
	window string,
) (production.SettlementDurableCommitPlan, bool, error) {
	return s.findSettlementDurablePlan(func(plan production.SettlementDurableCommitPlan) bool {
		return plan.Reference.RouteID == routeID && plan.Reference.SettlementWindow == window
	})
}

func (s *SettlementDurableStore) CommittedRouteSettlementOutboxDispatchPlan(
	routeID foundation.RouteID,
	window string,
) (production.SettlementOutboxDispatchPlan, bool, error) {
	plan, ok, err := s.CommittedRouteSettlementDurableCommitPlan(routeID, window)
	if err != nil || !ok {
		return production.SettlementOutboxDispatchPlan{}, ok, err
	}
	dispatch, err := production.NewSettlementOutboxDispatchPlan(&plan.Reference, plan.Outbox.OutboxRecords)
	if err != nil {
		return production.SettlementOutboxDispatchPlan{}, false, err
	}
	return dispatch, true, nil
}

func (s *SettlementDurableStore) findSettlementDurablePlan(
	match func(production.SettlementDurableCommitPlan) bool,
) (production.SettlementDurableCommitPlan, bool, error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return production.SettlementDurableCommitPlan{}, false, ErrNilDatabase
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	rows, err := s.store.db.QueryContext(ctx, `
		SELECT plan_json FROM settlement_durable_commits ORDER BY committed_at
	`)
	if err != nil {
		return production.SettlementDurableCommitPlan{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return production.SettlementDurableCommitPlan{}, false, err
		}
		var plan production.SettlementDurableCommitPlan
		if err := json.Unmarshal([]byte(raw), &plan); err != nil {
			return production.SettlementDurableCommitPlan{}, false, err
		}
		if match(plan) {
			return plan, true, nil
		}
	}
	return production.SettlementDurableCommitPlan{}, false, rows.Err()
}

func loadSettlementDurablePlan(
	ctx context.Context,
	runner queryContextRunner,
	referenceKey foundation.IdempotencyKey,
) (production.SettlementDurableCommitPlan, bool, error) {
	row := runner.QueryRowContext(ctx, `
		SELECT plan_json FROM settlement_durable_commits WHERE reference_key = $1
	`, string(referenceKey))
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return production.SettlementDurableCommitPlan{}, false, nil
		}
		return production.SettlementDurableCommitPlan{}, false, err
	}
	var plan production.SettlementDurableCommitPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return production.SettlementDurableCommitPlan{}, false, err
	}
	return plan, true, nil
}

func loadSettlementDurablePlanLocked(
	ctx context.Context,
	tx *sql.Tx,
	referenceKey foundation.IdempotencyKey,
) (production.SettlementDurableCommitPlan, bool, error) {
	return loadSettlementDurablePlan(ctx, tx, referenceKey)
}

func marshalSettlementDurablePlan(plan production.SettlementDurableCommitPlan) (json.RawMessage, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("marshal settlement durable plan: %w", err)
	}
	return data, nil
}

func settlementDurableResultFromPlan(plan production.SettlementDurableCommitPlan, duplicate bool) production.SettlementDurableCommitResult {
	reference := plan.Reference
	result := production.SettlementDurableCommitResult{
		Reference:     &reference,
		OutboxRecords: plan.Outbox.OutboxRecords,
		Duplicate:     duplicate,
	}
	if plan.ProductionState != nil {
		state := *plan.ProductionState
		result.ProductionState = &state
	}
	result.StorageRows = plan.StorageRows
	if plan.RouteRow != nil {
		routeRow := *plan.RouteRow
		result.RouteRow = &routeRow
	}
	result.RouteStorageLedger = plan.RouteStorageLedger
	return result
}
