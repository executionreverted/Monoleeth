package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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
	if claimProductionInitializationPlanIsNoOp(plan) {
		return discovery.ClaimProductionInitializationDurableResult{}, nil
	}
	normalized, nErr := normalizeClaimProductionInitializationPlan(plan)
	if nErr != nil {
		return discovery.ClaimProductionInitializationDurableResult{}, nErr
	}
	reference := normalized.Initialization.ClaimReference
	planJSON, mErr := json.Marshal(normalized)
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
		SELECT plan_json FROM claim_production_initialization_durable WHERE claim_reference = $1 FOR UPDATE
	`, string(reference)).Scan(&existingRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return discovery.ClaimProductionInitializationDurableResult{}, err
	}
	if err == nil {
		var existing discovery.ClaimProductionInitializationDurablePlan
		if jErr := json.Unmarshal([]byte(existingRaw), &existing); jErr != nil {
			return discovery.ClaimProductionInitializationDurableResult{}, jErr
		}
		existing, nErr = normalizeClaimProductionInitializationPlan(existing)
		if nErr != nil {
			return discovery.ClaimProductionInitializationDurableResult{}, nErr
		}
		if advanced, ok := advanceClaimProductionInitializationPlan(existing, normalized); ok {
			advancedJSON, jErr := json.Marshal(advanced)
			if jErr != nil {
				return discovery.ClaimProductionInitializationDurableResult{}, fmt.Errorf("marshal advanced claim production init durable plan: %w", jErr)
			}
			if _, err = tx.ExecContext(ctx, `
				UPDATE claim_production_initialization_durable
				SET plan_json = $2::jsonb, committed_at = now()
				WHERE claim_reference = $1
			`, string(reference), string(advancedJSON)); err != nil {
				return discovery.ClaimProductionInitializationDurableResult{}, err
			}
			if err = tx.Commit(); err != nil {
				return discovery.ClaimProductionInitializationDurableResult{}, err
			}
			return discovery.ClaimProductionInitializationDurableResult{Plan: advanced}, nil
		}
		if stalePendingReplay(existing, normalized) {
			if err = tx.Commit(); err != nil {
				return discovery.ClaimProductionInitializationDurableResult{}, err
			}
			return discovery.ClaimProductionInitializationDurableResult{Plan: existing, Duplicate: true}, nil
		}
		if !reflect.DeepEqual(existing, normalized) {
			return discovery.ClaimProductionInitializationDurableResult{}, fmt.Errorf("claim_reference_conflict: %w", discovery.ErrInvalidClaimDurableCommit)
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
	return discovery.ClaimProductionInitializationDurableResult{Plan: normalized}, nil
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
	normalized, err := normalizeClaimProductionInitializationPlan(plan)
	if err != nil {
		return discovery.ClaimProductionInitializationDurablePlan{}, false, err
	}
	return normalized, true, nil
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
		normalized, err := normalizeClaimProductionInitializationPlan(plan)
		if err != nil {
			return nil, err
		}
		if normalized.Boundary.Status != discovery.ClaimBoundaryStatusPendingSideEffects {
			continue
		}
		plans = append(plans, normalized)
	}
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Initialization.ClaimReference < plans[j].Initialization.ClaimReference
	})
	return plans, rows.Err()
}

func claimProductionInitializationPlanIsNoOp(plan discovery.ClaimProductionInitializationDurablePlan) bool {
	return reflect.DeepEqual(plan, discovery.ClaimProductionInitializationDurablePlan{})
}

func normalizeClaimProductionInitializationPlan(
	plan discovery.ClaimProductionInitializationDurablePlan,
) (discovery.ClaimProductionInitializationDurablePlan, error) {
	if claimProductionInitializationPlanIsNoOp(plan) {
		return discovery.ClaimProductionInitializationDurablePlan{}, nil
	}
	return plan.Initialization.DurablePlan(&plan.Boundary)
}

func advanceClaimProductionInitializationPlan(
	existing discovery.ClaimProductionInitializationDurablePlan,
	next discovery.ClaimProductionInitializationDurablePlan,
) (discovery.ClaimProductionInitializationDurablePlan, bool) {
	if !reflect.DeepEqual(existing.Initialization, next.Initialization) {
		return discovery.ClaimProductionInitializationDurablePlan{}, false
	}
	if existing.Boundary.Status != discovery.ClaimBoundaryStatusPendingSideEffects ||
		next.Boundary.Status != discovery.ClaimBoundaryStatusComplete {
		return discovery.ClaimProductionInitializationDurablePlan{}, false
	}
	if !sameClaimProductionInitializationBoundaryIdentity(existing.Boundary, next.Boundary) {
		return discovery.ClaimProductionInitializationDurablePlan{}, false
	}
	if next.Boundary.CompletedAt.Before(existing.Boundary.ClaimedAt) {
		return discovery.ClaimProductionInitializationDurablePlan{}, false
	}
	return next, true
}

func stalePendingReplay(
	existing discovery.ClaimProductionInitializationDurablePlan,
	next discovery.ClaimProductionInitializationDurablePlan,
) bool {
	return reflect.DeepEqual(existing.Initialization, next.Initialization) &&
		existing.Boundary.Status == discovery.ClaimBoundaryStatusComplete &&
		next.Boundary.Status == discovery.ClaimBoundaryStatusPendingSideEffects &&
		sameClaimProductionInitializationBoundaryIdentity(existing.Boundary, next.Boundary)
}

func sameClaimProductionInitializationBoundaryIdentity(
	left discovery.ClaimBoundaryRecord,
	right discovery.ClaimBoundaryRecord,
) bool {
	return left.ClaimReference == right.ClaimReference &&
		left.ReferenceKey == right.ReferenceKey &&
		left.PlayerID == right.PlayerID &&
		left.PlanetID == right.PlanetID &&
		left.EventID == right.EventID &&
		left.StaleIntelCount == right.StaleIntelCount &&
		left.ClaimedAt.Equal(right.ClaimedAt) &&
		left.RecordedAt.Equal(right.RecordedAt)
}
