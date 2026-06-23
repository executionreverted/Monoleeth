package production

import (
	"fmt"
	"reflect"
	"sync"

	"gameproject/internal/game/foundation"
)

// SettlementDurableCommitStore is the DB-adapter contract for committing a
// validated settlement reference, pending outbox rows, and any route storage
// ledger rows atomically.
type SettlementDurableCommitStore interface {
	ApplySettlementDurableCommitPlan(SettlementDurableCommitPlan) (SettlementDurableCommitResult, error)
}

// SettlementDurableCommitResult reports the rows accepted by the durable
// settlement commit boundary. Duplicate exact replays return the original rows
// with Duplicate set instead of appending new rows.
type SettlementDurableCommitResult struct {
	Reference          *SettlementReferenceRecord
	OutboxRecords      []ProductionOutboxRecord
	RouteStorageLedger []RouteStorageLedgerEntry
	Duplicate          bool
}

// InMemorySettlementDurableCommitStore is a process-local durable-table
// contract used by tests and future DB adapters. It enforces the same uniqueness
// and replay rules expected from a SQL reference row plus outbox/ledger commit.
type InMemorySettlementDurableCommitStore struct {
	mu         sync.RWMutex
	plans      map[foundation.IdempotencyKey]SettlementDurableCommitPlan
	references []foundation.IdempotencyKey
}

// NewInMemorySettlementDurableCommitStore returns an empty settlement durable
// commit adapter contract.
func NewInMemorySettlementDurableCommitStore() *InMemorySettlementDurableCommitStore {
	return &InMemorySettlementDurableCommitStore{
		plans: make(map[foundation.IdempotencyKey]SettlementDurableCommitPlan),
	}
}

// ApplySettlementDurableCommitPlan atomically records a non-empty durable
// settlement plan. Empty plans are no-ops; exact reference replays are
// idempotent; conflicting reference reuse is rejected before mutation.
func (store *InMemorySettlementDurableCommitStore) ApplySettlementDurableCommitPlan(
	plan SettlementDurableCommitPlan,
) (SettlementDurableCommitResult, error) {
	if store == nil {
		return SettlementDurableCommitResult{}, ErrInvalidSettlementDurableCommit
	}
	if settlementDurableCommitPlanIsNoOp(plan) {
		return SettlementDurableCommitResult{}, nil
	}
	if plan.Outbox.Reference.ReferenceKey != plan.Reference.ReferenceKey ||
		plan.Outbox.Reference.SettlementWindow != plan.Reference.SettlementWindow ||
		plan.Outbox.Reference.Kind != plan.Reference.Kind ||
		plan.Outbox.Reference.PlanetID != plan.Reference.PlanetID ||
		plan.Outbox.Reference.RouteID != plan.Reference.RouteID ||
		!plan.Outbox.Reference.AppliedAt.Equal(plan.Reference.AppliedAt) ||
		!plan.Outbox.Reference.RecordedAt.Equal(plan.Reference.RecordedAt) {
		return SettlementDurableCommitResult{}, fmt.Errorf("outbox.reference: %w", ErrInvalidSettlementDurableCommit)
	}
	normalized, err := NewSettlementDurableCommitPlan(
		&plan.Reference,
		plan.Outbox.OutboxRecords,
		plan.RouteStorageLedger,
	)
	if err != nil {
		return SettlementDurableCommitResult{}, err
	}
	if len(normalized.Outbox.OutboxRecords) == 0 {
		return SettlementDurableCommitResult{}, fmt.Errorf("outbox: %w", ErrInvalidSettlementDurableCommit)
	}

	key := normalized.Reference.ReferenceKey
	store.mu.Lock()
	defer store.mu.Unlock()

	if existing, ok := store.plans[key]; ok {
		if !settlementDurableCommitPlansEqual(existing, normalized) {
			return SettlementDurableCommitResult{}, fmt.Errorf("reference_conflict: %w", ErrInvalidSettlementDurableCommit)
		}
		return settlementDurableCommitResultFromPlan(existing, true), nil
	}
	store.ensureMapsLocked()
	store.plans[key] = cloneSettlementDurableCommitPlan(normalized)
	store.references = append(store.references, key)
	return settlementDurableCommitResultFromPlan(normalized, false), nil
}

// SettlementReferences returns committed settlement references in commit order.
func (store *InMemorySettlementDurableCommitStore) SettlementReferences() []SettlementReferenceRecord {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	records := make([]SettlementReferenceRecord, 0, len(store.references))
	for _, key := range store.references {
		records = append(records, cloneSettlementReferenceRecord(store.plans[key].Reference))
	}
	return records
}

// OutboxRecords returns committed settlement outbox rows in commit order.
func (store *InMemorySettlementDurableCommitStore) OutboxRecords() []ProductionOutboxRecord {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	var records []ProductionOutboxRecord
	for _, key := range store.references {
		records = append(records, store.plans[key].Outbox.OutboxRecords...)
	}
	return cloneProductionOutboxRecords(records)
}

// RouteStorageLedgerEntries returns committed route storage ledger rows in
// commit order.
func (store *InMemorySettlementDurableCommitStore) RouteStorageLedgerEntries() []RouteStorageLedgerEntry {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	var rows []RouteStorageLedgerEntry
	for _, key := range store.references {
		rows = append(rows, store.plans[key].RouteStorageLedger...)
	}
	return cloneRouteStorageLedgerEntries(rows)
}

func (store *InMemorySettlementDurableCommitStore) ensureMapsLocked() {
	if store.plans == nil {
		store.plans = make(map[foundation.IdempotencyKey]SettlementDurableCommitPlan)
	}
}

func settlementDurableCommitPlanIsNoOp(plan SettlementDurableCommitPlan) bool {
	return reflect.DeepEqual(plan, SettlementDurableCommitPlan{})
}

func settlementDurableCommitResultFromPlan(
	plan SettlementDurableCommitPlan,
	duplicate bool,
) SettlementDurableCommitResult {
	reference := cloneSettlementReferenceRecord(plan.Reference)
	return SettlementDurableCommitResult{
		Reference:          &reference,
		OutboxRecords:      cloneProductionOutboxRecords(plan.Outbox.OutboxRecords),
		RouteStorageLedger: cloneRouteStorageLedgerEntries(plan.RouteStorageLedger),
		Duplicate:          duplicate,
	}
}

func cloneSettlementDurableCommitPlan(plan SettlementDurableCommitPlan) SettlementDurableCommitPlan {
	plan.Reference = cloneSettlementReferenceRecord(plan.Reference)
	plan.Outbox.Reference = cloneSettlementReferenceRecord(plan.Outbox.Reference)
	plan.Outbox.OutboxRecords = cloneProductionOutboxRecords(plan.Outbox.OutboxRecords)
	plan.RouteStorageLedger = cloneRouteStorageLedgerEntries(plan.RouteStorageLedger)
	return plan
}

func settlementDurableCommitPlansEqual(left SettlementDurableCommitPlan, right SettlementDurableCommitPlan) bool {
	return reflect.DeepEqual(cloneSettlementDurableCommitPlan(left), cloneSettlementDurableCommitPlan(right))
}
