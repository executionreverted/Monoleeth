package discovery

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// ClaimProductionInitializationDurableStore is the durable adapter contract for
// committing one production-initialization row before the full claim lifecycle
// has necessarily completed.
type ClaimProductionInitializationDurableStore interface {
	ApplyClaimProductionInitializationDurablePlan(ClaimProductionInitializationDurablePlan) (ClaimProductionInitializationDurableResult, error)
}

// ClaimProductionInitializationDurableReader is the recovery/readback side of
// the durable production-initialization adapter.
type ClaimProductionInitializationDurableReader interface {
	CommittedClaimProductionInitializationDurablePlan(PlanetClaimReference) (ClaimProductionInitializationDurablePlan, bool, error)
	PendingClaimProductionInitializationDurablePlans(limit int) ([]ClaimProductionInitializationDurablePlan, error)
}

// ClaimProductionInitializationDurableResult reports the row accepted by the
// durable production-initialization boundary. Exact replays return the original
// row with Duplicate set.
type ClaimProductionInitializationDurableResult struct {
	Plan      ClaimProductionInitializationDurablePlan
	Duplicate bool
}

// ApplyDurableProductionInitialization validates and records this production
// initialization plan through a durable adapter.
func (plan ClaimProductionInitializationDurablePlan) ApplyDurableProductionInitialization(
	store ClaimProductionInitializationDurableStore,
) (ClaimProductionInitializationDurableResult, error) {
	if store == nil {
		return ClaimProductionInitializationDurableResult{}, ErrInvalidClaimDurableCommit
	}
	return store.ApplyClaimProductionInitializationDurablePlan(plan)
}

// InMemoryClaimProductionInitializationDurableStore is a process-local
// durable-table contract used by tests and future DB adapters.
type InMemoryClaimProductionInitializationDurableStore struct {
	mu    sync.RWMutex
	plans map[PlanetClaimReference]ClaimProductionInitializationDurablePlan
}

// NewInMemoryClaimProductionInitializationDurableStore returns an empty
// production-initialization durable commit adapter contract.
func NewInMemoryClaimProductionInitializationDurableStore() *InMemoryClaimProductionInitializationDurableStore {
	return &InMemoryClaimProductionInitializationDurableStore{
		plans: make(map[PlanetClaimReference]ClaimProductionInitializationDurablePlan),
	}
}

// ApplyClaimProductionInitializationDurablePlan atomically records one
// non-empty production-initialization row. Empty plans are no-ops; exact
// reference replays are idempotent; conflicting reference reuse is rejected
// before mutation.
func (store *InMemoryClaimProductionInitializationDurableStore) ApplyClaimProductionInitializationDurablePlan(
	plan ClaimProductionInitializationDurablePlan,
) (ClaimProductionInitializationDurableResult, error) {
	if store == nil {
		return ClaimProductionInitializationDurableResult{}, ErrInvalidClaimDurableCommit
	}
	if claimProductionInitializationDurablePlanIsNoOp(plan) {
		return ClaimProductionInitializationDurableResult{}, nil
	}
	normalized, err := normalizeClaimProductionInitializationDurablePlan(plan)
	if err != nil {
		return ClaimProductionInitializationDurableResult{}, err
	}
	reference := normalized.Initialization.ClaimReference

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if existing, ok := store.plans[reference]; ok {
		if advanced, ok := advanceClaimProductionInitializationDurablePlan(existing, normalized); ok {
			store.plans[reference] = cloneClaimProductionInitializationDurablePlan(advanced)
			return ClaimProductionInitializationDurableResult{Plan: cloneClaimProductionInitializationDurablePlan(advanced)}, nil
		}
		if claimProductionInitializationDurablePlanIsStalePendingReplay(existing, normalized) {
			return ClaimProductionInitializationDurableResult{Plan: cloneClaimProductionInitializationDurablePlan(existing), Duplicate: true}, nil
		}
		if !claimProductionInitializationDurablePlansEqual(existing, normalized) {
			return ClaimProductionInitializationDurableResult{}, fmt.Errorf("claim_reference_conflict: %w", ErrInvalidClaimDurableCommit)
		}
		return ClaimProductionInitializationDurableResult{Plan: cloneClaimProductionInitializationDurablePlan(existing), Duplicate: true}, nil
	}
	store.plans[reference] = cloneClaimProductionInitializationDurablePlan(normalized)
	return ClaimProductionInitializationDurableResult{Plan: cloneClaimProductionInitializationDurablePlan(normalized)}, nil
}

// ClaimReferences returns committed claim references in deterministic order.
func (store *InMemoryClaimProductionInitializationDurableStore) ClaimReferences() []PlanetClaimReference {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	if len(store.plans) == 0 {
		return nil
	}
	refs := make([]PlanetClaimReference, 0, len(store.plans))
	for ref := range store.plans {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i] < refs[j]
	})
	return refs
}

// PendingClaimProductionInitializationDurablePlans returns pending production
// initialization rows in deterministic claim-reference order. Durable recovery
// workers should use this shape to find claim side effects that were initialized
// but not yet advanced into a completed claim lifecycle.
func (store *InMemoryClaimProductionInitializationDurableStore) PendingClaimProductionInitializationDurablePlans(
	limit int,
) ([]ClaimProductionInitializationDurablePlan, error) {
	if store == nil {
		return nil, ErrInvalidClaimDurableCommit
	}
	if limit <= 0 {
		return nil, nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	refs := make([]PlanetClaimReference, 0, len(store.plans))
	for ref, plan := range store.plans {
		if plan.Boundary.Status == ClaimBoundaryStatusPendingSideEffects {
			refs = append(refs, ref)
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i] < refs[j]
	})

	capacity := len(refs)
	if limit < capacity {
		capacity = limit
	}
	plans := make([]ClaimProductionInitializationDurablePlan, 0, capacity)
	for _, ref := range refs {
		if len(plans) >= limit {
			break
		}
		cloned := cloneClaimProductionInitializationDurablePlan(store.plans[ref])
		normalized, err := normalizeClaimProductionInitializationDurablePlan(cloned)
		if err != nil {
			return nil, err
		}
		plans = append(plans, normalized)
	}
	return plans, nil
}

// CommittedClaimProductionInitializationDurablePlan returns the validated
// committed production-initialization plan for one claim reference.
func (store *InMemoryClaimProductionInitializationDurableStore) CommittedClaimProductionInitializationDurablePlan(
	reference PlanetClaimReference,
) (ClaimProductionInitializationDurablePlan, bool, error) {
	if store == nil {
		return ClaimProductionInitializationDurablePlan{}, false, ErrInvalidClaimDurableCommit
	}
	if err := reference.Validate(); err != nil {
		return ClaimProductionInitializationDurablePlan{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	plan, ok := store.plans[reference]
	if !ok {
		return ClaimProductionInitializationDurablePlan{}, false, nil
	}
	cloned := cloneClaimProductionInitializationDurablePlan(plan)
	normalized, err := normalizeClaimProductionInitializationDurablePlan(cloned)
	if err != nil {
		return ClaimProductionInitializationDurablePlan{}, false, err
	}
	return normalized, true, nil
}

func (store *InMemoryClaimProductionInitializationDurableStore) ensureMapsLocked() {
	if store.plans == nil {
		store.plans = make(map[PlanetClaimReference]ClaimProductionInitializationDurablePlan)
	}
}

func normalizeClaimProductionInitializationDurablePlan(
	plan ClaimProductionInitializationDurablePlan,
) (ClaimProductionInitializationDurablePlan, error) {
	var boundary *ClaimBoundaryRecord
	if !reflect.DeepEqual(plan.Boundary, ClaimBoundaryRecord{}) {
		boundary = &plan.Boundary
	}
	return NewClaimProductionInitializationDurablePlan(&plan.Initialization, boundary)
}

func claimProductionInitializationDurablePlanIsNoOp(plan ClaimProductionInitializationDurablePlan) bool {
	return reflect.DeepEqual(plan, ClaimProductionInitializationDurablePlan{})
}

func claimProductionInitializationDurablePlansEqual(
	left ClaimProductionInitializationDurablePlan,
	right ClaimProductionInitializationDurablePlan,
) bool {
	return reflect.DeepEqual(
		cloneClaimProductionInitializationDurablePlan(left),
		cloneClaimProductionInitializationDurablePlan(right),
	)
}

func advanceClaimProductionInitializationDurablePlan(
	existing ClaimProductionInitializationDurablePlan,
	next ClaimProductionInitializationDurablePlan,
) (ClaimProductionInitializationDurablePlan, bool) {
	if !reflect.DeepEqual(existing.Initialization, next.Initialization) {
		return ClaimProductionInitializationDurablePlan{}, false
	}
	if existing.Boundary.Status != ClaimBoundaryStatusPendingSideEffects ||
		next.Boundary.Status != ClaimBoundaryStatusComplete {
		return ClaimProductionInitializationDurablePlan{}, false
	}
	if existing.Boundary.ClaimReference != next.Boundary.ClaimReference ||
		existing.Boundary.ReferenceKey != next.Boundary.ReferenceKey ||
		existing.Boundary.PlayerID != next.Boundary.PlayerID ||
		existing.Boundary.PlanetID != next.Boundary.PlanetID ||
		existing.Boundary.EventID != next.Boundary.EventID ||
		existing.Boundary.StaleIntelCount != next.Boundary.StaleIntelCount ||
		!existing.Boundary.ClaimedAt.Equal(next.Boundary.ClaimedAt) ||
		!existing.Boundary.RecordedAt.Equal(next.Boundary.RecordedAt) {
		return ClaimProductionInitializationDurablePlan{}, false
	}
	if next.Boundary.CompletedAt.Before(existing.Boundary.ClaimedAt) {
		return ClaimProductionInitializationDurablePlan{}, false
	}
	return cloneClaimProductionInitializationDurablePlan(next), true
}

func claimProductionInitializationDurablePlanIsStalePendingReplay(
	existing ClaimProductionInitializationDurablePlan,
	next ClaimProductionInitializationDurablePlan,
) bool {
	if !reflect.DeepEqual(existing.Initialization, next.Initialization) {
		return false
	}
	if existing.Boundary.Status != ClaimBoundaryStatusComplete ||
		next.Boundary.Status != ClaimBoundaryStatusPendingSideEffects {
		return false
	}
	return next.Boundary.ClaimReference == existing.Boundary.ClaimReference &&
		next.Boundary.ReferenceKey == existing.Boundary.ReferenceKey &&
		next.Boundary.PlayerID == existing.Boundary.PlayerID &&
		next.Boundary.PlanetID == existing.Boundary.PlanetID &&
		next.Boundary.EventID == existing.Boundary.EventID &&
		next.Boundary.StaleIntelCount == existing.Boundary.StaleIntelCount &&
		next.Boundary.ClaimedAt.Equal(existing.Boundary.ClaimedAt) &&
		next.Boundary.RecordedAt.Equal(existing.Boundary.RecordedAt)
}
