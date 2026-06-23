package discovery

import (
	"fmt"
	"reflect"
	"sync"
)

// ClaimDurableLifecycleStore is the durable adapter contract for committing one
// completed planet-claim lifecycle: X Core storage mutation evidence, owner-CAS
// begin evidence, optional production initialization, and completion/outbox
// evidence.
type ClaimDurableLifecycleStore interface {
	ApplyClaimDurableLifecyclePlan(ClaimDurableLifecyclePlan) (ClaimDurableLifecycleResult, error)
}

// ClaimDurableLifecycleReader is the recovery/readback side of the durable
// claim lifecycle adapter.
type ClaimDurableLifecycleReader interface {
	CommittedClaimDurableLifecyclePlan(PlanetClaimReference) (ClaimDurableLifecyclePlan, bool, error)
}

// ClaimDurableLifecycleResult reports the rows accepted by the durable claim
// lifecycle boundary. Exact replays return the original rows with Duplicate set.
type ClaimDurableLifecycleResult struct {
	Plan      ClaimDurableLifecyclePlan
	Duplicate bool
}

// InMemoryClaimDurableLifecycleStore is a process-local durable-table contract
// used by tests and future DB adapters.
type InMemoryClaimDurableLifecycleStore struct {
	mu         sync.RWMutex
	plans      map[PlanetClaimReference]ClaimDurableLifecyclePlan
	references []PlanetClaimReference
}

// NewInMemoryClaimDurableLifecycleStore returns an empty claim lifecycle
// durable commit adapter contract.
func NewInMemoryClaimDurableLifecycleStore() *InMemoryClaimDurableLifecycleStore {
	return &InMemoryClaimDurableLifecycleStore{
		plans: make(map[PlanetClaimReference]ClaimDurableLifecyclePlan),
	}
}

// ApplyClaimDurableLifecyclePlan atomically records one non-empty claim
// lifecycle. Empty plans are no-ops; exact reference replays are idempotent;
// conflicting reference reuse is rejected before mutation.
func (store *InMemoryClaimDurableLifecycleStore) ApplyClaimDurableLifecyclePlan(
	plan ClaimDurableLifecyclePlan,
) (ClaimDurableLifecycleResult, error) {
	if store == nil {
		return ClaimDurableLifecycleResult{}, ErrInvalidClaimDurableCommit
	}
	if claimDurableLifecyclePlanIsNoOp(plan) {
		return ClaimDurableLifecycleResult{}, nil
	}
	normalized, err := normalizeClaimDurableLifecyclePlan(plan)
	if err != nil {
		return ClaimDurableLifecycleResult{}, err
	}
	reference := normalized.Commit.Boundary.ClaimReference

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if existing, ok := store.plans[reference]; ok {
		if !claimDurableLifecyclePlansEqual(existing, normalized) {
			return ClaimDurableLifecycleResult{}, fmt.Errorf("claim_reference_conflict: %w", ErrInvalidClaimDurableCommit)
		}
		return ClaimDurableLifecycleResult{Plan: cloneClaimDurableLifecyclePlan(existing), Duplicate: true}, nil
	}
	store.plans[reference] = cloneClaimDurableLifecyclePlan(normalized)
	store.references = append(store.references, reference)
	return ClaimDurableLifecycleResult{Plan: cloneClaimDurableLifecyclePlan(normalized)}, nil
}

// ClaimReferences returns committed claim references in commit order.
func (store *InMemoryClaimDurableLifecycleStore) ClaimReferences() []PlanetClaimReference {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	return append([]PlanetClaimReference(nil), store.references...)
}

// CommittedClaimDurableLifecyclePlan returns the validated committed lifecycle
// plan for one claim reference.
func (store *InMemoryClaimDurableLifecycleStore) CommittedClaimDurableLifecyclePlan(
	reference PlanetClaimReference,
) (ClaimDurableLifecyclePlan, bool, error) {
	if store == nil {
		return ClaimDurableLifecyclePlan{}, false, ErrInvalidClaimDurableCommit
	}
	if err := reference.Validate(); err != nil {
		return ClaimDurableLifecyclePlan{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	plan, ok := store.plans[reference]
	if !ok {
		return ClaimDurableLifecyclePlan{}, false, nil
	}
	cloned := cloneClaimDurableLifecyclePlan(plan)
	normalized, err := normalizeClaimDurableLifecyclePlan(cloned)
	if err != nil {
		return ClaimDurableLifecyclePlan{}, false, err
	}
	return normalized, true, nil
}

func (store *InMemoryClaimDurableLifecycleStore) ensureMapsLocked() {
	if store.plans == nil {
		store.plans = make(map[PlanetClaimReference]ClaimDurableLifecyclePlan)
	}
}

func normalizeClaimDurableLifecyclePlan(plan ClaimDurableLifecyclePlan) (ClaimDurableLifecyclePlan, error) {
	if !plan.HasProductionInit && !reflect.DeepEqual(plan.ProductionInitialized, ClaimProductionInitializationDurablePlan{}) {
		return ClaimDurableLifecyclePlan{}, fmt.Errorf("production_initialization: %w", ErrInvalidClaimDurableCommit)
	}
	var productionInit *ClaimProductionInitializationDurablePlan
	if plan.HasProductionInit {
		productionInit = &plan.ProductionInitialized
	}
	return NewClaimDurableLifecyclePlan(&plan.Begin, productionInit, &plan.Commit)
}

func claimDurableLifecyclePlanIsNoOp(plan ClaimDurableLifecyclePlan) bool {
	return reflect.DeepEqual(plan, ClaimDurableLifecyclePlan{})
}

func cloneClaimDurableLifecyclePlan(plan ClaimDurableLifecyclePlan) ClaimDurableLifecyclePlan {
	plan.Begin = cloneClaimDurableBeginPlan(plan.Begin)
	plan.ProductionInitialized = cloneClaimProductionInitializationDurablePlan(plan.ProductionInitialized)
	plan.Commit = cloneClaimDurableCommitPlan(plan.Commit)
	return plan
}

func claimDurableLifecyclePlansEqual(left ClaimDurableLifecyclePlan, right ClaimDurableLifecyclePlan) bool {
	return reflect.DeepEqual(cloneClaimDurableLifecyclePlan(left), cloneClaimDurableLifecyclePlan(right))
}
