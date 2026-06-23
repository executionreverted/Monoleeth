package production

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

type AutomationRouteDurableCommitStore interface {
	ApplyAutomationRouteDurableCommitPlan(AutomationRouteDurableCommitPlan) (AutomationRouteDurableCommitResult, error)
}

type AutomationRouteDurableReader interface {
	CommittedAutomationRouteDurableRecord(foundation.RouteID) (AutomationRouteDurableRecord, bool, error)
	CommittedAutomationRouteDurableRecordByReference(foundation.IdempotencyKey) (AutomationRouteDurableRecord, bool, error)
	CommittedAutomationRouteDurableRecordsForOwner(foundation.PlayerID) ([]AutomationRouteDurableRecord, error)
}

type AutomationRouteDurableCommitPlan struct {
	Route            AutomationRoute           `json:"route"`
	ReferenceKey     foundation.IdempotencyKey `json:"reference_key"`
	ExpectedRevision uint64                    `json:"expected_revision"`
	RecordedAt       time.Time                 `json:"recorded_at"`
}

type AutomationRouteDurableRecord struct {
	Route        AutomationRoute           `json:"route"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_key"`
	Revision     uint64                    `json:"revision"`
	RecordedAt   time.Time                 `json:"recorded_at"`
}

type AutomationRouteDurableCommitResult struct {
	Record    AutomationRouteDurableRecord
	Duplicate bool
}

type InMemoryAutomationRouteDurableStore struct {
	mu         sync.RWMutex
	records    map[foundation.RouteID]AutomationRouteDurableRecord
	references map[foundation.IdempotencyKey]foundation.RouteID
}

func NewInMemoryAutomationRouteDurableStore() *InMemoryAutomationRouteDurableStore {
	return &InMemoryAutomationRouteDurableStore{
		records:    make(map[foundation.RouteID]AutomationRouteDurableRecord),
		references: make(map[foundation.IdempotencyKey]foundation.RouteID),
	}
}

func (plan AutomationRouteDurableCommitPlan) ApplyDurableRouteCommit(
	store AutomationRouteDurableCommitStore,
) (AutomationRouteDurableCommitResult, error) {
	if store == nil {
		return AutomationRouteDurableCommitResult{}, ErrInvalidAutomationRouteDurableCommit
	}
	return store.ApplyAutomationRouteDurableCommitPlan(plan)
}

func (store *InMemoryAutomationRouteDurableStore) ApplyAutomationRouteDurableCommitPlan(
	plan AutomationRouteDurableCommitPlan,
) (AutomationRouteDurableCommitResult, error) {
	if store == nil {
		return AutomationRouteDurableCommitResult{}, ErrInvalidAutomationRouteDurableCommit
	}
	if automationRouteDurableCommitPlanIsNoOp(plan) {
		return AutomationRouteDurableCommitResult{}, nil
	}
	if err := plan.Validate(); err != nil {
		return AutomationRouteDurableCommitResult{}, err
	}
	normalized := normalizeAutomationRouteDurableCommitPlan(plan)

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if routeID, ok := store.references[normalized.ReferenceKey]; ok {
		record := store.records[routeID]
		if routeID != normalized.Route.RouteID ||
			!automationRouteDurableRecordsEqual(record, automationRouteDurableRecordFromPlan(normalized, record.Revision)) {
			return AutomationRouteDurableCommitResult{}, fmt.Errorf("reference_conflict: %w", ErrInvalidAutomationRouteDurableCommit)
		}
		return AutomationRouteDurableCommitResult{
			Record:    cloneAutomationRouteDurableRecord(record),
			Duplicate: true,
		}, nil
	}

	existing, exists := store.records[normalized.Route.RouteID]
	switch {
	case !exists && normalized.ExpectedRevision != 0:
		return AutomationRouteDurableCommitResult{}, fmt.Errorf("route %q expected revision %d: %w", normalized.Route.RouteID, normalized.ExpectedRevision, ErrStaleAutomationRouteDurableCommit)
	case exists && existing.Revision != normalized.ExpectedRevision:
		return AutomationRouteDurableCommitResult{}, fmt.Errorf("route %q expected revision %d current %d: %w", normalized.Route.RouteID, normalized.ExpectedRevision, existing.Revision, ErrStaleAutomationRouteDurableCommit)
	}

	record := automationRouteDurableRecordFromPlan(normalized, normalized.ExpectedRevision+1)
	store.records[record.Route.RouteID] = cloneAutomationRouteDurableRecord(record)
	store.references[record.ReferenceKey] = record.Route.RouteID
	return AutomationRouteDurableCommitResult{Record: cloneAutomationRouteDurableRecord(record)}, nil
}

func (store *InMemoryAutomationRouteDurableStore) CommittedAutomationRouteDurableRecord(
	routeID foundation.RouteID,
) (AutomationRouteDurableRecord, bool, error) {
	if err := routeID.Validate(); err != nil {
		return AutomationRouteDurableRecord{}, false, err
	}
	if store == nil {
		return AutomationRouteDurableRecord{}, false, ErrInvalidAutomationRouteDurableCommit
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	record, ok := store.records[routeID]
	if !ok {
		return AutomationRouteDurableRecord{}, false, nil
	}
	return cloneAutomationRouteDurableRecord(record), true, nil
}

func (store *InMemoryAutomationRouteDurableStore) CommittedAutomationRouteDurableRecordByReference(
	referenceKey foundation.IdempotencyKey,
) (AutomationRouteDurableRecord, bool, error) {
	if err := referenceKey.Validate(); err != nil {
		return AutomationRouteDurableRecord{}, false, err
	}
	if store == nil {
		return AutomationRouteDurableRecord{}, false, ErrInvalidAutomationRouteDurableCommit
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	routeID, ok := store.references[referenceKey]
	if !ok {
		return AutomationRouteDurableRecord{}, false, nil
	}
	record, ok := store.records[routeID]
	if !ok {
		return AutomationRouteDurableRecord{}, false, nil
	}
	return cloneAutomationRouteDurableRecord(record), true, nil
}

func (store *InMemoryAutomationRouteDurableStore) CommittedAutomationRouteDurableRecordsForOwner(
	playerID foundation.PlayerID,
) ([]AutomationRouteDurableRecord, error) {
	if err := playerID.Validate(); err != nil {
		return nil, err
	}
	if store == nil {
		return nil, ErrInvalidAutomationRouteDurableCommit
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	routeIDs := make([]foundation.RouteID, 0, len(store.records))
	for routeID, record := range store.records {
		if record.Route.OwnerPlayerID == playerID {
			routeIDs = append(routeIDs, routeID)
		}
	}
	sort.Slice(routeIDs, func(i, j int) bool {
		return routeIDs[i] < routeIDs[j]
	})
	records := make([]AutomationRouteDurableRecord, 0, len(routeIDs))
	for _, routeID := range routeIDs {
		records = append(records, cloneAutomationRouteDurableRecord(store.records[routeID]))
	}
	return records, nil
}

func (store *InMemoryAutomationRouteDurableStore) AutomationRouteDurableRecords() []AutomationRouteDurableRecord {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	routeIDs := make([]foundation.RouteID, 0, len(store.records))
	for routeID := range store.records {
		routeIDs = append(routeIDs, routeID)
	}
	sort.Slice(routeIDs, func(i, j int) bool {
		return routeIDs[i] < routeIDs[j]
	})
	records := make([]AutomationRouteDurableRecord, 0, len(routeIDs))
	for _, routeID := range routeIDs {
		records = append(records, cloneAutomationRouteDurableRecord(store.records[routeID]))
	}
	return records
}

func (input AutomationRouteDurableCommitPlan) Validate() error {
	if err := input.Route.Validate(); err != nil {
		return err
	}
	if err := input.ReferenceKey.Validate(); err != nil {
		return err
	}
	if input.RecordedAt.IsZero() {
		return fmt.Errorf("recorded_at: %w", ErrZeroProductionTimestamp)
	}
	return nil
}

func (store *InMemoryAutomationRouteDurableStore) ensureMapsLocked() {
	if store.records == nil {
		store.records = make(map[foundation.RouteID]AutomationRouteDurableRecord)
	}
	if store.references == nil {
		store.references = make(map[foundation.IdempotencyKey]foundation.RouteID)
	}
}

func automationRouteDurableRecordFromPlan(
	plan AutomationRouteDurableCommitPlan,
	revision uint64,
) AutomationRouteDurableRecord {
	return AutomationRouteDurableRecord{
		Route:        cloneAutomationRoute(plan.Route),
		ReferenceKey: plan.ReferenceKey,
		Revision:     revision,
		RecordedAt:   plan.RecordedAt.UTC(),
	}
}

func automationRouteDurableCommitPlanIsNoOp(plan AutomationRouteDurableCommitPlan) bool {
	return reflect.DeepEqual(plan, AutomationRouteDurableCommitPlan{})
}

func normalizeAutomationRouteDurableCommitPlan(plan AutomationRouteDurableCommitPlan) AutomationRouteDurableCommitPlan {
	plan.Route = cloneAutomationRoute(plan.Route)
	plan.RecordedAt = plan.RecordedAt.UTC()
	return plan
}

func automationRouteDurableRecordsEqual(left AutomationRouteDurableRecord, right AutomationRouteDurableRecord) bool {
	return reflect.DeepEqual(cloneAutomationRouteDurableRecord(left), cloneAutomationRouteDurableRecord(right))
}

func cloneAutomationRouteDurableRecord(record AutomationRouteDurableRecord) AutomationRouteDurableRecord {
	record.Route = cloneAutomationRoute(record.Route)
	record.RecordedAt = record.RecordedAt.UTC()
	return record
}
