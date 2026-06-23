package production

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

// SettlementDurableCommitStore is the DB-adapter contract for committing a
// validated settlement reference, pending outbox rows, and any route storage
// ledger rows atomically.
type SettlementDurableCommitStore interface {
	ApplySettlementDurableCommitPlan(SettlementDurableCommitPlan) (SettlementDurableCommitResult, error)
}

// SettlementDurableCommitReader is the recovery/readback side of the durable
// settlement commit adapter. DB-backed workers should use this shape to rebuild
// validated commit and dispatch plans from committed rows after a restart.
type SettlementDurableCommitReader interface {
	CommittedSettlementDurableCommitPlan(foundation.IdempotencyKey) (SettlementDurableCommitPlan, bool, error)
	CommittedSettlementOutboxDispatchPlan(foundation.IdempotencyKey) (SettlementOutboxDispatchPlan, bool, error)
	CommittedProductionSettlementDurableCommitPlan(foundation.PlanetID, string) (SettlementDurableCommitPlan, bool, error)
	CommittedProductionSettlementOutboxDispatchPlan(foundation.PlanetID, string) (SettlementOutboxDispatchPlan, bool, error)
	CommittedRouteSettlementDurableCommitPlan(foundation.RouteID, string) (SettlementDurableCommitPlan, bool, error)
	CommittedRouteSettlementOutboxDispatchPlan(foundation.RouteID, string) (SettlementOutboxDispatchPlan, bool, error)
}

// SettlementDurableCommitResult reports the rows accepted by the durable
// settlement commit boundary. Duplicate exact replays return the original rows
// with Duplicate set instead of appending new rows.
type SettlementDurableCommitResult struct {
	Reference          *SettlementReferenceRecord
	OutboxRecords      []ProductionOutboxRecord
	RouteRow           *AutomationRouteDurableRecord
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
		plan.RouteRow,
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

// ClaimPendingProductionOutboxRecords moves committed pending settlement
// outbox rows to in-flight in commit order.
func (store *InMemorySettlementDurableCommitStore) ClaimPendingProductionOutboxRecords(
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

	records := make([]ProductionOutboxRecord, 0, limit)
	for _, key := range store.references {
		plan := store.plans[key]
		mutated := false
		for index := range plan.Outbox.OutboxRecords {
			if len(records) >= limit {
				break
			}
			if plan.Outbox.OutboxRecords[index].Status != ProductionOutboxStatusPending {
				continue
			}
			plan.Outbox.OutboxRecords[index].Status = ProductionOutboxStatusInFlight
			plan.Outbox.OutboxRecords[index].ClaimedAt = claimedAt
			plan.Outbox.OutboxRecords[index].Attempts++
			plan.Outbox.OutboxRecords[index].ClaimToken = productionOutboxClaimToken(
				plan.Outbox.OutboxRecords[index].OutboxID,
				plan.Outbox.OutboxRecords[index].Attempts,
			)
			records = append(records, cloneProductionOutboxRecord(plan.Outbox.OutboxRecords[index]))
			mutated = true
		}
		if mutated {
			store.plans[key] = cloneSettlementDurableCommitPlan(plan)
		}
		if len(records) >= limit {
			break
		}
	}
	return records, nil
}

// MarkProductionOutboxPublished records successful delivery for the current
// claim token on a committed settlement outbox row.
func (store *InMemorySettlementDurableCommitStore) MarkProductionOutboxPublished(
	outboxID string,
	claimToken string,
	publishedAt time.Time,
) (ProductionOutboxRecord, bool, error) {
	if store == nil {
		return ProductionOutboxRecord{}, false, ErrInvalidProductionOutboxPublisher
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	key, index, ok := store.outboxIndexLocked(outboxID)
	if !ok {
		return ProductionOutboxRecord{}, false, nil
	}
	plan := store.plans[key]
	if !settlementDurableOutboxClaimMatches(plan.Outbox.OutboxRecords[index], claimToken) {
		return ProductionOutboxRecord{}, false, nil
	}
	plan.Outbox.OutboxRecords[index].Status = ProductionOutboxStatusPublished
	plan.Outbox.OutboxRecords[index].PublishedAt = publishedAt.UTC()
	store.plans[key] = cloneSettlementDurableCommitPlan(plan)
	return cloneProductionOutboxRecord(plan.Outbox.OutboxRecords[index]), true, nil
}

// MarkProductionOutboxFailed records failed delivery for the current claim
// token on a committed settlement outbox row.
func (store *InMemorySettlementDurableCommitStore) MarkProductionOutboxFailed(
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

	key, index, ok := store.outboxIndexLocked(outboxID)
	if !ok {
		return ProductionOutboxRecord{}, false, nil
	}
	plan := store.plans[key]
	if !settlementDurableOutboxClaimMatches(plan.Outbox.OutboxRecords[index], claimToken) {
		return ProductionOutboxRecord{}, false, nil
	}
	plan.Outbox.OutboxRecords[index].Status = ProductionOutboxStatusFailed
	plan.Outbox.OutboxRecords[index].FailedAt = failedAt.UTC()
	plan.Outbox.OutboxRecords[index].LastError = reason
	store.plans[key] = cloneSettlementDurableCommitPlan(plan)
	return cloneProductionOutboxRecord(plan.Outbox.OutboxRecords[index]), true, nil
}

// ReleaseExpiredProductionOutboxRecords returns stale committed settlement
// outbox leases to pending.
func (store *InMemorySettlementDurableCommitStore) ReleaseExpiredProductionOutboxRecords(
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

	records := make([]ProductionOutboxRecord, 0, limit)
	for _, key := range store.references {
		plan := store.plans[key]
		mutated := false
		for index := range plan.Outbox.OutboxRecords {
			if len(records) >= limit {
				break
			}
			record := plan.Outbox.OutboxRecords[index]
			if record.Status != ProductionOutboxStatusInFlight ||
				record.ClaimedAt.IsZero() ||
				!record.ClaimedAt.Before(claimedBefore) {
				continue
			}
			plan.Outbox.OutboxRecords[index].Status = ProductionOutboxStatusPending
			plan.Outbox.OutboxRecords[index].ClaimedAt = time.Time{}
			plan.Outbox.OutboxRecords[index].ClaimToken = ""
			plan.Outbox.OutboxRecords[index].RetriedAt = releasedAt
			records = append(records, cloneProductionOutboxRecord(plan.Outbox.OutboxRecords[index]))
			mutated = true
		}
		if mutated {
			store.plans[key] = cloneSettlementDurableCommitPlan(plan)
		}
		if len(records) >= limit {
			break
		}
	}
	return records, nil
}

// RetryFailedProductionOutboxRecords returns failed committed settlement
// outbox rows to pending in commit order while preserving failure evidence.
func (store *InMemorySettlementDurableCommitStore) RetryFailedProductionOutboxRecords(
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

	records := make([]ProductionOutboxRecord, 0, limit)
	for _, key := range store.references {
		plan := store.plans[key]
		mutated := false
		for index := range plan.Outbox.OutboxRecords {
			if len(records) >= limit {
				break
			}
			if plan.Outbox.OutboxRecords[index].Status != ProductionOutboxStatusFailed {
				continue
			}
			plan.Outbox.OutboxRecords[index].Status = ProductionOutboxStatusPending
			plan.Outbox.OutboxRecords[index].ClaimedAt = time.Time{}
			plan.Outbox.OutboxRecords[index].ClaimToken = ""
			plan.Outbox.OutboxRecords[index].RetriedAt = retriedAt
			records = append(records, cloneProductionOutboxRecord(plan.Outbox.OutboxRecords[index]))
			mutated = true
		}
		if mutated {
			store.plans[key] = cloneSettlementDurableCommitPlan(plan)
		}
		if len(records) >= limit {
			break
		}
	}
	return records, nil
}

// CommittedSettlementDurableCommitPlan returns the validated committed row
// bundle for one settlement reference.
func (store *InMemorySettlementDurableCommitStore) CommittedSettlementDurableCommitPlan(
	referenceKey foundation.IdempotencyKey,
) (SettlementDurableCommitPlan, bool, error) {
	if store == nil {
		return SettlementDurableCommitPlan{}, false, ErrInvalidSettlementDurableCommit
	}
	if err := referenceKey.Validate(); err != nil {
		return SettlementDurableCommitPlan{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	plan, ok := store.plans[referenceKey]
	if !ok {
		return SettlementDurableCommitPlan{}, false, nil
	}
	cloned := cloneSettlementDurableCommitPlan(plan)
	if _, err := NewSettlementDurableCommitPlan(&cloned.Reference, cloned.Outbox.OutboxRecords, cloned.RouteStorageLedger, cloned.RouteRow); err != nil {
		return SettlementDurableCommitPlan{}, false, err
	}
	return cloned, true, nil
}

// CommittedSettlementOutboxDispatchPlan returns the validated publisher
// dispatch handoff for one committed settlement reference.
func (store *InMemorySettlementDurableCommitStore) CommittedSettlementOutboxDispatchPlan(
	referenceKey foundation.IdempotencyKey,
) (SettlementOutboxDispatchPlan, bool, error) {
	plan, ok, err := store.CommittedSettlementDurableCommitPlan(referenceKey)
	if err != nil || !ok {
		return SettlementOutboxDispatchPlan{}, ok, err
	}
	dispatch, err := NewSettlementOutboxDispatchPlan(&plan.Reference, plan.Outbox.OutboxRecords)
	if err != nil {
		return SettlementOutboxDispatchPlan{}, false, err
	}
	return dispatch, true, nil
}

// CommittedProductionSettlementDurableCommitPlan rebuilds the committed
// production settlement row bundle for one planet/window identity.
func (store *InMemorySettlementDurableCommitStore) CommittedProductionSettlementDurableCommitPlan(
	planetID foundation.PlanetID,
	settlementWindow string,
) (SettlementDurableCommitPlan, bool, error) {
	referenceKey, err := productionSettlementReferenceKey(planetID, settlementWindow)
	if err != nil {
		return SettlementDurableCommitPlan{}, false, err
	}
	plan, ok, err := store.CommittedSettlementDurableCommitPlan(referenceKey)
	if err != nil || !ok {
		return SettlementDurableCommitPlan{}, ok, err
	}
	if plan.Reference.Kind != SettlementKindProduction ||
		plan.Reference.PlanetID != planetID ||
		plan.Reference.RouteID != "" ||
		plan.Reference.SettlementWindow != settlementWindow {
		return SettlementDurableCommitPlan{}, false, ErrInvalidSettlementDurableCommit
	}
	return plan, true, nil
}

// CommittedProductionSettlementOutboxDispatchPlan rebuilds the publisher
// dispatch handoff for one committed production settlement window.
func (store *InMemorySettlementDurableCommitStore) CommittedProductionSettlementOutboxDispatchPlan(
	planetID foundation.PlanetID,
	settlementWindow string,
) (SettlementOutboxDispatchPlan, bool, error) {
	plan, ok, err := store.CommittedProductionSettlementDurableCommitPlan(planetID, settlementWindow)
	if err != nil || !ok {
		return SettlementOutboxDispatchPlan{}, ok, err
	}
	dispatch, err := NewSettlementOutboxDispatchPlan(&plan.Reference, plan.Outbox.OutboxRecords)
	if err != nil {
		return SettlementOutboxDispatchPlan{}, false, err
	}
	return dispatch, true, nil
}

// CommittedRouteSettlementDurableCommitPlan rebuilds the committed route
// settlement row bundle for one route/window identity.
func (store *InMemorySettlementDurableCommitStore) CommittedRouteSettlementDurableCommitPlan(
	routeID foundation.RouteID,
	settlementWindow string,
) (SettlementDurableCommitPlan, bool, error) {
	referenceKey, err := routeSettlementReferenceKey(routeID, settlementWindow)
	if err != nil {
		return SettlementDurableCommitPlan{}, false, err
	}
	plan, ok, err := store.CommittedSettlementDurableCommitPlan(referenceKey)
	if err != nil || !ok {
		return SettlementDurableCommitPlan{}, ok, err
	}
	if plan.Reference.Kind != SettlementKindRoute ||
		plan.Reference.PlanetID != "" ||
		plan.Reference.RouteID != routeID ||
		plan.Reference.SettlementWindow != settlementWindow {
		return SettlementDurableCommitPlan{}, false, ErrInvalidSettlementDurableCommit
	}
	return plan, true, nil
}

// CommittedRouteSettlementOutboxDispatchPlan rebuilds the publisher dispatch
// handoff for one committed route settlement window.
func (store *InMemorySettlementDurableCommitStore) CommittedRouteSettlementOutboxDispatchPlan(
	routeID foundation.RouteID,
	settlementWindow string,
) (SettlementOutboxDispatchPlan, bool, error) {
	plan, ok, err := store.CommittedRouteSettlementDurableCommitPlan(routeID, settlementWindow)
	if err != nil || !ok {
		return SettlementOutboxDispatchPlan{}, ok, err
	}
	dispatch, err := NewSettlementOutboxDispatchPlan(&plan.Reference, plan.Outbox.OutboxRecords)
	if err != nil {
		return SettlementOutboxDispatchPlan{}, false, err
	}
	return dispatch, true, nil
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
		RouteRow:           cloneAutomationRouteDurableRecordPointer(plan.RouteRow),
		RouteStorageLedger: cloneRouteStorageLedgerEntries(plan.RouteStorageLedger),
		Duplicate:          duplicate,
	}
}

func cloneSettlementDurableCommitPlan(plan SettlementDurableCommitPlan) SettlementDurableCommitPlan {
	plan.Reference = cloneSettlementReferenceRecord(plan.Reference)
	plan.Outbox.Reference = cloneSettlementReferenceRecord(plan.Outbox.Reference)
	plan.Outbox.OutboxRecords = cloneProductionOutboxRecords(plan.Outbox.OutboxRecords)
	plan.RouteRow = cloneAutomationRouteDurableRecordPointer(plan.RouteRow)
	plan.RouteStorageLedger = cloneRouteStorageLedgerEntries(plan.RouteStorageLedger)
	return plan
}

func settlementDurableCommitPlansEqual(left SettlementDurableCommitPlan, right SettlementDurableCommitPlan) bool {
	return reflect.DeepEqual(cloneSettlementDurableCommitPlan(left), cloneSettlementDurableCommitPlan(right))
}

func (store *InMemorySettlementDurableCommitStore) outboxIndexLocked(
	outboxID string,
) (foundation.IdempotencyKey, int, bool) {
	for _, key := range store.references {
		plan := store.plans[key]
		for index, record := range plan.Outbox.OutboxRecords {
			if record.OutboxID == outboxID {
				return key, index, true
			}
		}
	}
	return "", 0, false
}

func settlementDurableOutboxClaimMatches(record ProductionOutboxRecord, claimToken string) bool {
	return record.Status == ProductionOutboxStatusInFlight && record.ClaimToken != "" && record.ClaimToken == claimToken
}

func productionSettlementReferenceKey(
	planetID foundation.PlanetID,
	settlementWindow string,
) (foundation.IdempotencyKey, error) {
	if err := planetID.Validate(); err != nil {
		return "", err
	}
	if err := validateSettlementWindow(settlementWindow); err != nil {
		return "", err
	}
	return foundation.OfflineSettlementIdempotencyKey(planetID, settlementWindow)
}

func routeSettlementReferenceKey(
	routeID foundation.RouteID,
	settlementWindow string,
) (foundation.IdempotencyKey, error) {
	if err := routeID.Validate(); err != nil {
		return "", err
	}
	if err := validateSettlementWindow(settlementWindow); err != nil {
		return "", err
	}
	return foundation.RouteSettlementIdempotencyKey(routeID, settlementWindow)
}
