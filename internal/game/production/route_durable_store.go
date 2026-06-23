package production

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
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
	Route                 AutomationRoute           `json:"route"`
	SourceProductionState *PlanetProductionState    `json:"source_production_state,omitempty"`
	ReferenceKey          foundation.IdempotencyKey `json:"reference_key"`
	ExpectedRevision      uint64                    `json:"expected_revision"`
	RecordedAt            time.Time                 `json:"recorded_at"`
}

type AutomationRouteDurableRecord struct {
	Route                 AutomationRoute           `json:"route"`
	SourceProductionState *PlanetProductionState    `json:"source_production_state,omitempty"`
	ReferenceKey          foundation.IdempotencyKey `json:"reference_key"`
	Revision              uint64                    `json:"revision"`
	RecordedAt            time.Time                 `json:"recorded_at"`
}

type AutomationRouteDurableCommitResult struct {
	Record    AutomationRouteDurableRecord
	Duplicate bool
}

type InMemoryAutomationRouteDurableStore struct {
	mu         sync.RWMutex
	records    map[foundation.RouteID]AutomationRouteDurableRecord
	references map[foundation.IdempotencyKey]AutomationRouteDurableRecord
}

func NewInMemoryAutomationRouteDurableStore() *InMemoryAutomationRouteDurableStore {
	return &InMemoryAutomationRouteDurableStore{
		records:    make(map[foundation.RouteID]AutomationRouteDurableRecord),
		references: make(map[foundation.IdempotencyKey]AutomationRouteDurableRecord),
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

	store.mu.Lock()
	defer store.mu.Unlock()
	return applyAutomationRouteDurableCommitPlanToMaps(&store.records, &store.references, plan)
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
	if err := validateAutomationRouteDurableRecordForRoute(record, routeID); err != nil {
		return AutomationRouteDurableRecord{}, false, err
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

	record, ok := store.references[referenceKey]
	if !ok {
		return AutomationRouteDurableRecord{}, false, nil
	}
	if err := validateAutomationRouteDurableRecordForReference(record, referenceKey, store.records); err != nil {
		return AutomationRouteDurableRecord{}, false, err
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

	if err := validateAutomationRouteDurableRecordMap(store.records); err != nil {
		return nil, err
	}
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
	if input.SourceProductionState != nil {
		if err := input.SourceProductionState.Validate(); err != nil {
			return err
		}
		if input.SourceProductionState.PlanetID != input.Route.SourcePlanetID {
			return fmt.Errorf("route source %q production state %q: %w", input.Route.SourcePlanetID, input.SourceProductionState.PlanetID, ErrInvalidAutomationRouteDurableCommit)
		}
		if input.Route.Enabled && input.SourceProductionState.EnergyReservedPerHour < input.Route.EnergyCostPerHour {
			return fmt.Errorf("route energy %d source reserved %d: %w", input.Route.EnergyCostPerHour, input.SourceProductionState.EnergyReservedPerHour, ErrInvalidAutomationRouteDurableCommit)
		}
		if !input.SourceProductionState.UpdatedAt.Equal(input.RecordedAt) {
			return fmt.Errorf("source production updated_at %s recorded_at %s: %w", input.SourceProductionState.UpdatedAt, input.RecordedAt, ErrInvalidAutomationRouteDurableCommit)
		}
	}
	return nil
}

func (store *InMemoryAutomationRouteDurableStore) ensureMapsLocked() {
	ensureAutomationRouteDurableMaps(&store.records, &store.references)
}

func (store *InMemoryStore) ApplyAutomationRouteDurableCommitPlan(
	plan AutomationRouteDurableCommitPlan,
) (AutomationRouteDurableCommitResult, error) {
	if store == nil {
		return AutomationRouteDurableCommitResult{}, ErrInvalidAutomationRouteDurableCommit
	}
	if automationRouteDurableCommitPlanIsNoOp(plan) {
		return AutomationRouteDurableCommitResult{}, nil
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()
	return store.applyAutomationRouteDurableCommitPlanLocked(plan)
}

func (store *InMemoryStore) CommittedAutomationRouteDurableRecord(
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

	record, ok := store.routeDurableRecords[routeID]
	if !ok {
		return AutomationRouteDurableRecord{}, false, nil
	}
	if err := validateAutomationRouteDurableRecordForRoute(record, routeID); err != nil {
		return AutomationRouteDurableRecord{}, false, err
	}
	return cloneAutomationRouteDurableRecord(record), true, nil
}

func (store *InMemoryStore) CommittedAutomationRouteDurableRecordByReference(
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

	record, ok := store.routeDurableReferences[referenceKey]
	if !ok {
		return AutomationRouteDurableRecord{}, false, nil
	}
	if err := validateAutomationRouteDurableRecordForReference(record, referenceKey, store.routeDurableRecords); err != nil {
		return AutomationRouteDurableRecord{}, false, err
	}
	return cloneAutomationRouteDurableRecord(record), true, nil
}

func (store *InMemoryStore) CommittedAutomationRouteDurableRecordsForOwner(
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

	if err := validateAutomationRouteDurableRecordMap(store.routeDurableRecords); err != nil {
		return nil, err
	}
	return automationRouteDurableRecordsForOwner(store.routeDurableRecords, playerID), nil
}

func (store *InMemoryStore) AutomationRouteDurableRecords() []AutomationRouteDurableRecord {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	return automationRouteDurableRecordsFromMap(store.routeDurableRecords)
}

func (store *InMemoryStore) applyAutomationRouteDurableCommitPlanLocked(
	plan AutomationRouteDurableCommitPlan,
) (AutomationRouteDurableCommitResult, error) {
	result, err := applyAutomationRouteDurableCommitPlanToMaps(
		&store.routeDurableRecords,
		&store.routeDurableReferences,
		plan,
	)
	if err != nil {
		return AutomationRouteDurableCommitResult{}, err
	}
	if !result.Duplicate && plan.SourceProductionState != nil {
		store.states[plan.SourceProductionState.PlanetID] = cloneProductionState(*plan.SourceProductionState)
	}
	return result, nil
}

func (store *InMemoryStore) commitRouteDurableMutationLocked(
	route AutomationRoute,
	sourceProductionState *PlanetProductionState,
	referenceKey foundation.IdempotencyKey,
	recordedAt time.Time,
) error {
	if referenceKey.IsZero() {
		return nil
	}
	expectedRevision := uint64(0)
	if record, ok := store.routeDurableRecords[route.RouteID]; ok {
		expectedRevision = record.Revision
	}
	_, err := store.applyAutomationRouteDurableCommitPlanLocked(AutomationRouteDurableCommitPlan{
		Route:                 route,
		SourceProductionState: sourceProductionState,
		ReferenceKey:          referenceKey,
		ExpectedRevision:      expectedRevision,
		RecordedAt:            recordedAt,
	})
	return err
}

func (store *InMemoryStore) committedAutomationRouteDurableRecordByReferenceLocked(
	referenceKey foundation.IdempotencyKey,
) (AutomationRouteDurableRecord, bool, error) {
	record, ok := store.routeDurableReferences[referenceKey]
	if !ok {
		return AutomationRouteDurableRecord{}, false, nil
	}
	if err := validateAutomationRouteDurableRecordForReference(record, referenceKey, store.routeDurableRecords); err != nil {
		return AutomationRouteDurableRecord{}, false, err
	}
	return cloneAutomationRouteDurableRecord(record), true, nil
}

func ensureAutomationRouteDurableMaps(
	records *map[foundation.RouteID]AutomationRouteDurableRecord,
	references *map[foundation.IdempotencyKey]AutomationRouteDurableRecord,
) {
	if *records == nil {
		*records = make(map[foundation.RouteID]AutomationRouteDurableRecord)
	}
	if *references == nil {
		*references = make(map[foundation.IdempotencyKey]AutomationRouteDurableRecord)
	}
}

func applyAutomationRouteDurableCommitPlanToMaps(
	records *map[foundation.RouteID]AutomationRouteDurableRecord,
	references *map[foundation.IdempotencyKey]AutomationRouteDurableRecord,
	plan AutomationRouteDurableCommitPlan,
) (AutomationRouteDurableCommitResult, error) {
	if records == nil || references == nil {
		return AutomationRouteDurableCommitResult{}, ErrInvalidAutomationRouteDurableCommit
	}
	if automationRouteDurableCommitPlanIsNoOp(plan) {
		return AutomationRouteDurableCommitResult{}, nil
	}
	if err := plan.Validate(); err != nil {
		return AutomationRouteDurableCommitResult{}, err
	}
	normalized := normalizeAutomationRouteDurableCommitPlan(plan)
	ensureAutomationRouteDurableMaps(records, references)

	if record, ok := (*references)[normalized.ReferenceKey]; ok {
		if record.Route.RouteID != normalized.Route.RouteID ||
			!automationRouteDurableRecordsEqual(record, automationRouteDurableRecordFromPlan(normalized, record.Revision)) {
			return AutomationRouteDurableCommitResult{}, fmt.Errorf("reference_conflict: %w", ErrInvalidAutomationRouteDurableCommit)
		}
		return AutomationRouteDurableCommitResult{
			Record:    cloneAutomationRouteDurableRecord(record),
			Duplicate: true,
		}, nil
	}

	existing, exists := (*records)[normalized.Route.RouteID]
	switch {
	case !exists && normalized.ExpectedRevision != 0:
		return AutomationRouteDurableCommitResult{}, fmt.Errorf("route %q expected revision %d: %w", normalized.Route.RouteID, normalized.ExpectedRevision, ErrStaleAutomationRouteDurableCommit)
	case exists && existing.Revision != normalized.ExpectedRevision:
		return AutomationRouteDurableCommitResult{}, fmt.Errorf("route %q expected revision %d current %d: %w", normalized.Route.RouteID, normalized.ExpectedRevision, existing.Revision, ErrStaleAutomationRouteDurableCommit)
	}

	record := automationRouteDurableRecordFromPlan(normalized, normalized.ExpectedRevision+1)
	(*records)[record.Route.RouteID] = cloneAutomationRouteDurableRecord(record)
	(*references)[record.ReferenceKey] = cloneAutomationRouteDurableRecord(record)
	return AutomationRouteDurableCommitResult{Record: cloneAutomationRouteDurableRecord(record)}, nil
}

func automationRouteDurableRecordsForOwner(
	records map[foundation.RouteID]AutomationRouteDurableRecord,
	playerID foundation.PlayerID,
) []AutomationRouteDurableRecord {
	routeIDs := make([]foundation.RouteID, 0, len(records))
	for routeID, record := range records {
		if record.Route.OwnerPlayerID == playerID {
			routeIDs = append(routeIDs, routeID)
		}
	}
	sort.Slice(routeIDs, func(i, j int) bool {
		return routeIDs[i] < routeIDs[j]
	})
	ownerRecords := make([]AutomationRouteDurableRecord, 0, len(routeIDs))
	for _, routeID := range routeIDs {
		ownerRecords = append(ownerRecords, cloneAutomationRouteDurableRecord(records[routeID]))
	}
	return ownerRecords
}

func automationRouteDurableRecordsFromMap(
	records map[foundation.RouteID]AutomationRouteDurableRecord,
) []AutomationRouteDurableRecord {
	routeIDs := make([]foundation.RouteID, 0, len(records))
	for routeID := range records {
		routeIDs = append(routeIDs, routeID)
	}
	sort.Slice(routeIDs, func(i, j int) bool {
		return routeIDs[i] < routeIDs[j]
	})
	durableRecords := make([]AutomationRouteDurableRecord, 0, len(routeIDs))
	for _, routeID := range routeIDs {
		durableRecords = append(durableRecords, cloneAutomationRouteDurableRecord(records[routeID]))
	}
	return durableRecords
}

func validateAutomationRouteDurableRecordMap(records map[foundation.RouteID]AutomationRouteDurableRecord) error {
	for routeID, record := range records {
		if err := validateAutomationRouteDurableRecordForRoute(record, routeID); err != nil {
			return err
		}
	}
	return nil
}

func validateAutomationRouteDurableRecordForRoute(record AutomationRouteDurableRecord, routeID foundation.RouteID) error {
	if record.Route.RouteID != routeID {
		return fmt.Errorf("route_id: %w", ErrInvalidAutomationRouteDurableCommit)
	}
	return validateAutomationRouteDurableRecord(record)
}

func validateAutomationRouteDurableRecordForReference(
	record AutomationRouteDurableRecord,
	referenceKey foundation.IdempotencyKey,
	records map[foundation.RouteID]AutomationRouteDurableRecord,
) error {
	if record.ReferenceKey != referenceKey {
		return fmt.Errorf("reference_key: %w", ErrInvalidAutomationRouteDurableCommit)
	}
	if err := validateAutomationRouteDurableRecord(record); err != nil {
		return err
	}
	if err := validateAutomationRouteDurableReferenceKeyMatchesRecord(referenceKey, record); err != nil {
		return err
	}
	routeRecord, ok := records[record.Route.RouteID]
	if !ok {
		return fmt.Errorf("route_record: %w", ErrInvalidAutomationRouteDurableCommit)
	}
	if err := validateAutomationRouteDurableRecordForRoute(routeRecord, record.Route.RouteID); err != nil {
		return err
	}
	if !automationRouteDurableImmutableIdentityMatches(routeRecord.Route, record.Route) {
		return fmt.Errorf("route_record.identity: %w", ErrInvalidAutomationRouteDurableCommit)
	}
	return nil
}

func validateAutomationRouteDurableReferenceKeyMatchesRecord(
	referenceKey foundation.IdempotencyKey,
	record AutomationRouteDurableRecord,
) error {
	parts := strings.Split(referenceKey.String(), ":")
	if len(parts) < 3 {
		return fmt.Errorf("reference_key: %w", ErrInvalidAutomationRouteDurableCommit)
	}
	switch parts[0] {
	case "route_create":
		if len(parts) != 3 || parts[1] != record.Route.OwnerPlayerID.String() || parts[2] != record.Route.RouteID.String() {
			return fmt.Errorf("reference_key: %w", ErrInvalidAutomationRouteDurableCommit)
		}
	case "route_update", "route_enable", "route_disable":
		if len(parts) != 4 || parts[1] != record.Route.OwnerPlayerID.String() || parts[2] != record.Route.RouteID.String() {
			return fmt.Errorf("reference_key: %w", ErrInvalidAutomationRouteDurableCommit)
		}
	case "route_settlement":
		if len(parts) != 3 || parts[1] != record.Route.RouteID.String() {
			return fmt.Errorf("reference_key: %w", ErrInvalidAutomationRouteDurableCommit)
		}
	default:
		return fmt.Errorf("reference_key: %w", ErrInvalidAutomationRouteDurableCommit)
	}
	return nil
}

func validateAutomationRouteDurableRecord(record AutomationRouteDurableRecord) error {
	if err := record.Route.Validate(); err != nil {
		return fmt.Errorf("route: %w: %v", ErrInvalidAutomationRouteDurableCommit, err)
	}
	if err := record.ReferenceKey.Validate(); err != nil {
		return fmt.Errorf("reference_key: %w: %v", ErrInvalidAutomationRouteDurableCommit, err)
	}
	if record.Revision == 0 {
		return fmt.Errorf("revision: %w", ErrInvalidAutomationRouteDurableCommit)
	}
	if record.RecordedAt.IsZero() {
		return fmt.Errorf("recorded_at: %w", ErrInvalidAutomationRouteDurableCommit)
	}
	if record.SourceProductionState != nil {
		if err := record.SourceProductionState.Validate(); err != nil {
			return fmt.Errorf("source_production_state: %w: %v", ErrInvalidAutomationRouteDurableCommit, err)
		}
		if record.SourceProductionState.PlanetID != record.Route.SourcePlanetID {
			return fmt.Errorf("source_production_state: %w", ErrInvalidAutomationRouteDurableCommit)
		}
		if record.Route.Enabled && record.SourceProductionState.EnergyReservedPerHour < record.Route.EnergyCostPerHour {
			return fmt.Errorf("source_production_state.energy: %w", ErrInvalidAutomationRouteDurableCommit)
		}
		if !record.SourceProductionState.UpdatedAt.Equal(record.RecordedAt) {
			return fmt.Errorf("source_production_state.updated_at: %w", ErrInvalidAutomationRouteDurableCommit)
		}
	}
	return nil
}

func automationRouteDurableImmutableIdentityMatches(current AutomationRoute, reference AutomationRoute) bool {
	return current.RouteID == reference.RouteID &&
		current.OwnerPlayerID == reference.OwnerPlayerID &&
		current.SourcePlanetID == reference.SourcePlanetID &&
		current.SourceMapID == reference.SourceMapID &&
		current.CreatedAt.Equal(reference.CreatedAt)
}

func automationRouteDurableRecordFromPlan(
	plan AutomationRouteDurableCommitPlan,
	revision uint64,
) AutomationRouteDurableRecord {
	return AutomationRouteDurableRecord{
		Route:                 cloneAutomationRoute(plan.Route),
		SourceProductionState: cloneProductionStatePointer(plan.SourceProductionState),
		ReferenceKey:          plan.ReferenceKey,
		Revision:              revision,
		RecordedAt:            plan.RecordedAt.UTC(),
	}
}

func automationRouteDurableCommitPlanIsNoOp(plan AutomationRouteDurableCommitPlan) bool {
	return reflect.DeepEqual(plan, AutomationRouteDurableCommitPlan{})
}

func normalizeAutomationRouteDurableCommitPlan(plan AutomationRouteDurableCommitPlan) AutomationRouteDurableCommitPlan {
	plan.Route = cloneAutomationRoute(plan.Route)
	plan.SourceProductionState = cloneProductionStatePointer(plan.SourceProductionState)
	plan.RecordedAt = plan.RecordedAt.UTC()
	return plan
}

func automationRouteDurableRecordsEqual(left AutomationRouteDurableRecord, right AutomationRouteDurableRecord) bool {
	return reflect.DeepEqual(cloneAutomationRouteDurableRecord(left), cloneAutomationRouteDurableRecord(right))
}

func cloneAutomationRouteDurableRecord(record AutomationRouteDurableRecord) AutomationRouteDurableRecord {
	record.Route = cloneAutomationRoute(record.Route)
	record.SourceProductionState = cloneProductionStatePointer(record.SourceProductionState)
	record.RecordedAt = record.RecordedAt.UTC()
	return record
}

func cloneAutomationRouteDurableRecordPointer(
	record *AutomationRouteDurableRecord,
) *AutomationRouteDurableRecord {
	if record == nil {
		return nil
	}
	cloned := cloneAutomationRouteDurableRecord(*record)
	return &cloned
}
