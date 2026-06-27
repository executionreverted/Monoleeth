package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"gameproject/internal/game/discovery"
)

// ClaimProductionInitializationDurableStore is the Postgres-backed durable
// adapter for one committed claim production-initialization plan.
type ClaimProductionInitializationDurableStore struct {
	store *Store
}

var (
	_ discovery.ClaimProductionInitializationDurableStore  = (*ClaimProductionInitializationDurableStore)(nil)
	_ discovery.ClaimProductionInitializationDurableReader = (*ClaimProductionInitializationDurableStore)(nil)
)

func NewClaimProductionInitializationDurableStore(store *Store) (*ClaimProductionInitializationDurableStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &ClaimProductionInitializationDurableStore{store: store}, nil
}

func (s *ClaimProductionInitializationDurableStore) ApplyClaimProductionInitializationDurablePlan(
	plan discovery.ClaimProductionInitializationDurablePlan,
) (result discovery.ClaimProductionInitializationDurableResult, err error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return discovery.ClaimProductionInitializationDurableResult{}, ErrNilDatabase
	}
	reference := plan.Initialization.ClaimReference
	if reference == "" {
		return discovery.ClaimProductionInitializationDurableResult{}, nil
	}
	planJSON, mErr := json.Marshal(plan)
	if mErr != nil {
		return discovery.ClaimProductionInitializationDurableResult{}, fmt.Errorf("marshal claim production init durable plan: %w", mErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	tx, txErr := s.store.db.BeginTx(ctx, nil)
	if txErr != nil {
		return discovery.ClaimProductionInitializationDurableResult{}, txErr
	}
	defer rollbackUnlessCommitted(tx, &err)

	var existingRaw string
	err = tx.QueryRowContext(ctx, `
		SELECT plan_json FROM claim_production_initialization_durable WHERE claim_reference = $1
	`, string(reference)).Scan(&existingRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return discovery.ClaimProductionInitializationDurableResult{}, err
	}
	if err == nil {
		var existing discovery.ClaimProductionInitializationDurablePlan
		if jErr := json.Unmarshal([]byte(existingRaw), &existing); jErr != nil {
			return discovery.ClaimProductionInitializationDurableResult{}, jErr
		}
		if err = tx.Commit(); err != nil {
			return discovery.ClaimProductionInitializationDurableResult{}, err
		}
		return discovery.ClaimProductionInitializationDurableResult{Plan: existing, Duplicate: true}, nil
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO claim_production_initialization_durable(claim_reference, plan_json)
		VALUES ($1, $2::jsonb)
	`, string(reference), string(planJSON)); err != nil {
		return discovery.ClaimProductionInitializationDurableResult{}, err
	}

	if err = tx.Commit(); err != nil {
		return discovery.ClaimProductionInitializationDurableResult{}, err
	}
	return discovery.ClaimProductionInitializationDurableResult{Plan: plan}, nil
}

func (s *ClaimProductionInitializationDurableStore) CommittedClaimProductionInitializationDurablePlan(
	reference discovery.PlanetClaimReference,
) (discovery.ClaimProductionInitializationDurablePlan, bool, error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return discovery.ClaimProductionInitializationDurablePlan{}, false, ErrNilDatabase
	}
	if err := reference.Validate(); err != nil {
		return discovery.ClaimProductionInitializationDurablePlan{}, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	var raw string
	err := s.store.db.QueryRowContext(ctx, `
		SELECT plan_json FROM claim_production_initialization_durable WHERE claim_reference = $1
	`, string(reference)).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return discovery.ClaimProductionInitializationDurablePlan{}, false, nil
		}
		return discovery.ClaimProductionInitializationDurablePlan{}, false, err
	}
	var plan discovery.ClaimProductionInitializationDurablePlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return discovery.ClaimProductionInitializationDurablePlan{}, false, err
	}
	return plan, true, nil
}

func (s *ClaimProductionInitializationDurableStore) PendingClaimProductionInitializationDurablePlans(
	limit int,
) ([]discovery.ClaimProductionInitializationDurablePlan, error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return nil, ErrNilDatabase
	}
	if limit <= 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	rows, err := s.store.db.QueryContext(ctx, `
		SELECT plan_json FROM claim_production_initialization_durable
		ORDER BY committed_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plans := make([]discovery.ClaimProductionInitializationDurablePlan, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var plan discovery.ClaimProductionInitializationDurablePlan
		if err := json.Unmarshal([]byte(raw), &plan); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Initialization.ClaimReference < plans[j].Initialization.ClaimReference
	})
	return plans, rows.Err()
}
