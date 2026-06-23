package production

import (
	"fmt"
	"reflect"
	"sync"

	"gameproject/internal/game/foundation"
)

// BuildingMutationDurableCommitStore is the DB-adapter contract for committing
// a validated building mutation reference, pending outbox rows, and material
// ledger rows atomically.
type BuildingMutationDurableCommitStore interface {
	ApplyBuildingMutationDurableCommitPlan(BuildingMutationDurableCommitPlan) (BuildingMutationDurableCommitResult, error)
}

// BuildingMutationDurableCommitReader is the recovery/readback side of the
// durable building mutation commit adapter.
type BuildingMutationDurableCommitReader interface {
	CommittedBuildingMutationDurableCommitPlan(foundation.IdempotencyKey) (BuildingMutationDurableCommitPlan, bool, error)
}

// BuildingMutationDurableCommitResult reports the rows accepted by the durable
// building mutation boundary. Exact replays return the original rows with
// Duplicate set instead of appending new rows.
type BuildingMutationDurableCommitResult struct {
	Reference      *BuildingMutationReferenceRecord
	OutboxRecords  []ProductionOutboxRecord
	MaterialLedger []BuildingMaterialLedgerEntry
	Duplicate      bool
}

// InMemoryBuildingMutationDurableCommitStore is a process-local durable-table
// contract used by tests and future DB adapters.
type InMemoryBuildingMutationDurableCommitStore struct {
	mu         sync.RWMutex
	plans      map[foundation.IdempotencyKey]BuildingMutationDurableCommitPlan
	references []foundation.IdempotencyKey
}

// NewInMemoryBuildingMutationDurableCommitStore returns an empty building
// mutation durable commit adapter contract.
func NewInMemoryBuildingMutationDurableCommitStore() *InMemoryBuildingMutationDurableCommitStore {
	return &InMemoryBuildingMutationDurableCommitStore{
		plans: make(map[foundation.IdempotencyKey]BuildingMutationDurableCommitPlan),
	}
}

// ApplyBuildingMutationDurableCommitPlan atomically records a non-empty
// durable building mutation plan. Empty plans are no-ops; exact reference
// replays are idempotent; conflicting reference reuse is rejected before
// mutation.
func (store *InMemoryBuildingMutationDurableCommitStore) ApplyBuildingMutationDurableCommitPlan(
	plan BuildingMutationDurableCommitPlan,
) (BuildingMutationDurableCommitResult, error) {
	if store == nil {
		return BuildingMutationDurableCommitResult{}, ErrInvalidBuildingMutationDurableCommit
	}
	if buildingMutationDurableCommitPlanIsNoOp(plan) {
		return BuildingMutationDurableCommitResult{}, nil
	}
	normalized, err := NewBuildingMutationDurableCommitPlan(&plan.Reference, plan.OutboxRecords, plan.MaterialLedger)
	if err != nil {
		return BuildingMutationDurableCommitResult{}, err
	}

	key := normalized.Reference.ReferenceKey
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if existing, ok := store.plans[key]; ok {
		if !buildingMutationDurableCommitPlansEqual(existing, normalized) {
			return BuildingMutationDurableCommitResult{}, fmt.Errorf("reference_conflict: %w", ErrInvalidBuildingMutationDurableCommit)
		}
		return buildingMutationDurableCommitResultFromPlan(existing, true), nil
	}
	store.plans[key] = cloneBuildingMutationDurableCommitPlan(normalized)
	store.references = append(store.references, key)
	return buildingMutationDurableCommitResultFromPlan(normalized, false), nil
}

// BuildingMutationReferences returns committed building mutation references in
// commit order.
func (store *InMemoryBuildingMutationDurableCommitStore) BuildingMutationReferences() []BuildingMutationReferenceRecord {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	records := make([]BuildingMutationReferenceRecord, 0, len(store.references))
	for _, key := range store.references {
		records = append(records, cloneBuildingMutationReferenceRecord(store.plans[key].Reference))
	}
	return records
}

// OutboxRecords returns committed building mutation outbox rows in commit
// order.
func (store *InMemoryBuildingMutationDurableCommitStore) OutboxRecords() []ProductionOutboxRecord {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	var records []ProductionOutboxRecord
	for _, key := range store.references {
		records = append(records, store.plans[key].OutboxRecords...)
	}
	return cloneProductionOutboxRecords(records)
}

// BuildingMaterialLedgerEntries returns committed building material ledger rows
// in commit order.
func (store *InMemoryBuildingMutationDurableCommitStore) BuildingMaterialLedgerEntries() []BuildingMaterialLedgerEntry {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	var rows []BuildingMaterialLedgerEntry
	for _, key := range store.references {
		rows = append(rows, store.plans[key].MaterialLedger...)
	}
	return cloneBuildingMaterialLedgerEntries(rows)
}

// CommittedBuildingMutationDurableCommitPlan returns the validated committed
// row bundle for one building mutation reference.
func (store *InMemoryBuildingMutationDurableCommitStore) CommittedBuildingMutationDurableCommitPlan(
	referenceKey foundation.IdempotencyKey,
) (BuildingMutationDurableCommitPlan, bool, error) {
	if store == nil {
		return BuildingMutationDurableCommitPlan{}, false, ErrInvalidBuildingMutationDurableCommit
	}
	if err := referenceKey.Validate(); err != nil {
		return BuildingMutationDurableCommitPlan{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	plan, ok := store.plans[referenceKey]
	if !ok {
		return BuildingMutationDurableCommitPlan{}, false, nil
	}
	cloned := cloneBuildingMutationDurableCommitPlan(plan)
	normalized, err := NewBuildingMutationDurableCommitPlan(&cloned.Reference, cloned.OutboxRecords, cloned.MaterialLedger)
	if err != nil {
		return BuildingMutationDurableCommitPlan{}, false, err
	}
	return normalized, true, nil
}

func (store *InMemoryBuildingMutationDurableCommitStore) ensureMapsLocked() {
	if store.plans == nil {
		store.plans = make(map[foundation.IdempotencyKey]BuildingMutationDurableCommitPlan)
	}
}

func buildingMutationDurableCommitPlanIsNoOp(plan BuildingMutationDurableCommitPlan) bool {
	return reflect.DeepEqual(plan, BuildingMutationDurableCommitPlan{})
}

func buildingMutationDurableCommitResultFromPlan(
	plan BuildingMutationDurableCommitPlan,
	duplicate bool,
) BuildingMutationDurableCommitResult {
	reference := cloneBuildingMutationReferenceRecord(plan.Reference)
	return BuildingMutationDurableCommitResult{
		Reference:      &reference,
		OutboxRecords:  cloneProductionOutboxRecords(plan.OutboxRecords),
		MaterialLedger: cloneBuildingMaterialLedgerEntries(plan.MaterialLedger),
		Duplicate:      duplicate,
	}
}

func cloneBuildingMutationDurableCommitPlan(plan BuildingMutationDurableCommitPlan) BuildingMutationDurableCommitPlan {
	plan.Reference = cloneBuildingMutationReferenceRecord(plan.Reference)
	plan.OutboxRecords = cloneProductionOutboxRecords(plan.OutboxRecords)
	plan.MaterialLedger = cloneBuildingMaterialLedgerEntries(plan.MaterialLedger)
	return plan
}

func buildingMutationDurableCommitPlansEqual(
	left BuildingMutationDurableCommitPlan,
	right BuildingMutationDurableCommitPlan,
) bool {
	return reflect.DeepEqual(cloneBuildingMutationDurableCommitPlan(left), cloneBuildingMutationDurableCommitPlan(right))
}

func buildingMutationOutboxRecordsEqual(left []ProductionOutboxRecord, right []ProductionOutboxRecord) bool {
	return reflect.DeepEqual(cloneProductionOutboxRecords(left), cloneProductionOutboxRecords(right))
}

func buildingMutationMaterialLedgerEqual(left []BuildingMaterialLedgerEntry, right []BuildingMaterialLedgerEntry) bool {
	return reflect.DeepEqual(cloneBuildingMaterialLedgerEntries(left), cloneBuildingMaterialLedgerEntries(right))
}
