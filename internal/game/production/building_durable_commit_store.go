package production

import (
	"fmt"
	"reflect"
	"sync"
	"time"

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
	CommittedBuildingMutationOutboxDispatchPlan(foundation.IdempotencyKey) (BuildingMutationOutboxDispatchPlan, bool, error)
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

// ClaimPendingProductionOutboxRecords moves committed pending building
// mutation outbox rows to in-flight in commit order.
func (store *InMemoryBuildingMutationDurableCommitStore) ClaimPendingProductionOutboxRecords(
	limit int,
	claimedAt time.Time,
) ([]ProductionOutboxRecord, error) {
	if store == nil {
		return nil, ErrInvalidProductionOutboxPublisher
	}
	if limit <= 0 {
		return nil, nil
	}
	claimedAt = claimedAt.UTC()
	if claimedAt.IsZero() {
		claimedAt = time.Unix(0, 0).UTC()
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateBuildingMutationDurableCommitReadbacksLocked(); err != nil {
		return nil, err
	}

	records := make([]ProductionOutboxRecord, 0, limit)
	for _, key := range store.references {
		plan := store.plans[key]
		mutated := false
		for index := range plan.OutboxRecords {
			if len(records) >= limit {
				break
			}
			if plan.OutboxRecords[index].Status != ProductionOutboxStatusPending {
				continue
			}
			plan.OutboxRecords[index].Status = ProductionOutboxStatusInFlight
			plan.OutboxRecords[index].ClaimedAt = claimedAt
			plan.OutboxRecords[index].Attempts++
			plan.OutboxRecords[index].ClaimToken = productionOutboxClaimToken(
				plan.OutboxRecords[index].OutboxID,
				plan.OutboxRecords[index].Attempts,
			)
			records = append(records, cloneProductionOutboxRecord(plan.OutboxRecords[index]))
			mutated = true
		}
		if mutated {
			store.plans[key] = cloneBuildingMutationDurableCommitPlan(plan)
		}
		if len(records) >= limit {
			break
		}
	}
	return records, nil
}

// MarkProductionOutboxPublished records successful delivery for the current
// claim token on a committed building mutation outbox row.
func (store *InMemoryBuildingMutationDurableCommitStore) MarkProductionOutboxPublished(
	outboxID string,
	claimToken string,
	publishedAt time.Time,
) (ProductionOutboxRecord, bool, error) {
	if store == nil {
		return ProductionOutboxRecord{}, false, ErrInvalidProductionOutboxPublisher
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateBuildingMutationDurableCommitReadbacksLocked(); err != nil {
		return ProductionOutboxRecord{}, false, err
	}

	key, index, ok := store.outboxIndexLocked(outboxID)
	if !ok {
		return ProductionOutboxRecord{}, false, nil
	}
	plan := store.plans[key]
	if !buildingMutationDurableOutboxClaimMatches(plan.OutboxRecords[index], claimToken) {
		return ProductionOutboxRecord{}, false, nil
	}
	plan.OutboxRecords[index].Status = ProductionOutboxStatusPublished
	plan.OutboxRecords[index].PublishedAt = publishedAt.UTC()
	store.plans[key] = cloneBuildingMutationDurableCommitPlan(plan)
	return cloneProductionOutboxRecord(plan.OutboxRecords[index]), true, nil
}

// MarkProductionOutboxFailed records failed delivery for the current claim
// token on a committed building mutation outbox row.
func (store *InMemoryBuildingMutationDurableCommitStore) MarkProductionOutboxFailed(
	outboxID string,
	claimToken string,
	reason string,
	failedAt time.Time,
) (ProductionOutboxRecord, bool, error) {
	if store == nil {
		return ProductionOutboxRecord{}, false, ErrInvalidProductionOutboxPublisher
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateBuildingMutationDurableCommitReadbacksLocked(); err != nil {
		return ProductionOutboxRecord{}, false, err
	}

	key, index, ok := store.outboxIndexLocked(outboxID)
	if !ok {
		return ProductionOutboxRecord{}, false, nil
	}
	plan := store.plans[key]
	if !buildingMutationDurableOutboxClaimMatches(plan.OutboxRecords[index], claimToken) {
		return ProductionOutboxRecord{}, false, nil
	}
	plan.OutboxRecords[index].Status = ProductionOutboxStatusFailed
	plan.OutboxRecords[index].FailedAt = failedAt.UTC()
	plan.OutboxRecords[index].LastError = reason
	store.plans[key] = cloneBuildingMutationDurableCommitPlan(plan)
	return cloneProductionOutboxRecord(plan.OutboxRecords[index]), true, nil
}

// ReleaseExpiredProductionOutboxRecords returns stale committed building
// mutation outbox leases to pending.
func (store *InMemoryBuildingMutationDurableCommitStore) ReleaseExpiredProductionOutboxRecords(
	limit int,
	claimedBefore time.Time,
	releasedAt time.Time,
) ([]ProductionOutboxRecord, error) {
	if store == nil {
		return nil, ErrInvalidProductionOutboxPublisher
	}
	if limit <= 0 || claimedBefore.IsZero() {
		return nil, nil
	}
	claimedBefore = claimedBefore.UTC()
	releasedAt = releasedAt.UTC()

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateBuildingMutationDurableCommitReadbacksLocked(); err != nil {
		return nil, err
	}

	records := make([]ProductionOutboxRecord, 0, limit)
	for _, key := range store.references {
		plan := store.plans[key]
		mutated := false
		for index := range plan.OutboxRecords {
			if len(records) >= limit {
				break
			}
			record := plan.OutboxRecords[index]
			if record.Status != ProductionOutboxStatusInFlight ||
				record.ClaimedAt.IsZero() ||
				!record.ClaimedAt.Before(claimedBefore) {
				continue
			}
			plan.OutboxRecords[index].Status = ProductionOutboxStatusPending
			plan.OutboxRecords[index].ClaimedAt = time.Time{}
			plan.OutboxRecords[index].ClaimToken = ""
			plan.OutboxRecords[index].RetriedAt = releasedAt
			records = append(records, cloneProductionOutboxRecord(plan.OutboxRecords[index]))
			mutated = true
		}
		if mutated {
			store.plans[key] = cloneBuildingMutationDurableCommitPlan(plan)
		}
		if len(records) >= limit {
			break
		}
	}
	return records, nil
}

// RetryFailedProductionOutboxRecords returns failed committed building mutation
// outbox rows to pending in commit order while preserving failure evidence.
func (store *InMemoryBuildingMutationDurableCommitStore) RetryFailedProductionOutboxRecords(
	limit int,
	retriedAt time.Time,
) ([]ProductionOutboxRecord, error) {
	if store == nil {
		return nil, ErrInvalidProductionOutboxPublisher
	}
	if limit <= 0 {
		return nil, nil
	}
	retriedAt = retriedAt.UTC()

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateBuildingMutationDurableCommitReadbacksLocked(); err != nil {
		return nil, err
	}

	records := make([]ProductionOutboxRecord, 0, limit)
	for _, key := range store.references {
		plan := store.plans[key]
		mutated := false
		for index := range plan.OutboxRecords {
			if len(records) >= limit {
				break
			}
			if plan.OutboxRecords[index].Status != ProductionOutboxStatusFailed {
				continue
			}
			plan.OutboxRecords[index].Status = ProductionOutboxStatusPending
			plan.OutboxRecords[index].ClaimedAt = time.Time{}
			plan.OutboxRecords[index].ClaimToken = ""
			plan.OutboxRecords[index].RetriedAt = retriedAt
			records = append(records, cloneProductionOutboxRecord(plan.OutboxRecords[index]))
			mutated = true
		}
		if mutated {
			store.plans[key] = cloneBuildingMutationDurableCommitPlan(plan)
		}
		if len(records) >= limit {
			break
		}
	}
	return records, nil
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
	if err := validateBuildingMutationDurableCommitReadbackPlan(cloned); err != nil {
		return BuildingMutationDurableCommitPlan{}, false, err
	}
	return cloned, true, nil
}

// CommittedBuildingMutationOutboxDispatchPlan returns the validated publisher
// dispatch handoff for one committed building mutation reference.
func (store *InMemoryBuildingMutationDurableCommitStore) CommittedBuildingMutationOutboxDispatchPlan(
	referenceKey foundation.IdempotencyKey,
) (BuildingMutationOutboxDispatchPlan, bool, error) {
	plan, ok, err := store.CommittedBuildingMutationDurableCommitPlan(referenceKey)
	if err != nil || !ok {
		return BuildingMutationOutboxDispatchPlan{}, ok, err
	}
	dispatch, err := NewBuildingMutationOutboxDispatchPlan(&plan.Reference, plan.OutboxRecords)
	if err != nil {
		return BuildingMutationOutboxDispatchPlan{}, false, err
	}
	return dispatch, true, nil
}

func (store *InMemoryBuildingMutationDurableCommitStore) ensureMapsLocked() {
	if store.plans == nil {
		store.plans = make(map[foundation.IdempotencyKey]BuildingMutationDurableCommitPlan)
	}
}

func (store *InMemoryBuildingMutationDurableCommitStore) outboxIndexLocked(
	outboxID string,
) (foundation.IdempotencyKey, int, bool) {
	for _, key := range store.references {
		plan := store.plans[key]
		for index, record := range plan.OutboxRecords {
			if record.OutboxID == outboxID {
				return key, index, true
			}
		}
	}
	return "", 0, false
}

func buildingMutationDurableOutboxClaimMatches(record ProductionOutboxRecord, claimToken string) bool {
	return record.Status == ProductionOutboxStatusInFlight && record.ClaimToken != "" && record.ClaimToken == claimToken
}

func (store *InMemoryBuildingMutationDurableCommitStore) validateBuildingMutationDurableCommitReadbacksLocked() error {
	for _, key := range store.references {
		plan, ok := store.plans[key]
		if !ok {
			return ErrInvalidBuildingMutationDurableCommit
		}
		if err := validateBuildingMutationDurableCommitReadbackPlan(plan); err != nil {
			return err
		}
	}
	return nil
}

func validateBuildingMutationDurableCommitReadbackPlan(plan BuildingMutationDurableCommitPlan) error {
	cloned := cloneBuildingMutationDurableCommitPlan(plan)
	if err := validateProductionOutboxReadbackStates(cloned.OutboxRecords, ErrInvalidBuildingMutationDurableCommit); err != nil {
		return err
	}
	pendingOutbox := pendingProductionOutboxRecordsForCommitValidation(cloned.OutboxRecords)
	if _, err := NewBuildingMutationDurableCommitPlan(&cloned.Reference, pendingOutbox, cloned.MaterialLedger); err != nil {
		return err
	}
	if !buildingMutationOutboxRecordsEqual(cloned.Reference.Result.OutboxRecords, pendingOutbox) {
		return fmt.Errorf("outbox.result: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	if !buildingMutationMaterialLedgerEqual(cloned.Reference.Result.MaterialLedger, cloned.MaterialLedger) {
		return fmt.Errorf("material_ledger.result: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	return nil
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
