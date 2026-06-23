package production

import (
	"fmt"
	"sort"
	"sync"
	"time"

	gameevents "gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

// InitializePlanetProductionInput creates the primitive state rows after a
// server-side planet claim has already been accepted by the discovery domain.
type InitializePlanetProductionInput struct {
	PlanetID              foundation.PlanetID
	LastCalculatedAt      time.Time
	StorageCapacityUnits  int64
	EnergyCapacityPerHour int64
	UpdatedAt             time.Time
}

// InitializePlanetProductionResult reports whether rows were newly stored.
type InitializePlanetProductionResult struct {
	Snapshot PlanetProductionSnapshot
	Created  bool
}

// InMemoryStore is a mutex-protected repository for Phase 09 production state.
type InMemoryStore struct {
	mu sync.RWMutex

	states                     map[foundation.PlanetID]PlanetProductionState
	storage                    map[foundation.PlanetID]PlanetStorage
	buildings                  map[foundation.PlanetID]map[BuildingID]PlanetBuilding
	routes                     map[foundation.RouteID]AutomationRoute
	routeDurableRecords        map[foundation.RouteID]AutomationRouteDurableRecord
	routeDurableReferences     map[foundation.IdempotencyKey]AutomationRouteDurableRecord
	events                     []gameevents.EventEnvelope
	nextEventSequence          uint64
	references                 map[foundation.IdempotencyKey]SettlementReferenceRecord
	buildingReferences         map[foundation.IdempotencyKey]BuildingMutationReferenceRecord
	buildingMaterialLedger     []BuildingMaterialLedgerEntry
	nextBuildingLedgerSequence uint64
	routeStorageLedger         []RouteStorageLedgerEntry
	nextRouteLedgerSequence    uint64
	outbox                     []ProductionOutboxRecord
	nextOutboxSequence         uint64
}

// NewInMemoryStore returns an empty production repository.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		states:                 make(map[foundation.PlanetID]PlanetProductionState),
		storage:                make(map[foundation.PlanetID]PlanetStorage),
		buildings:              make(map[foundation.PlanetID]map[BuildingID]PlanetBuilding),
		routes:                 make(map[foundation.RouteID]AutomationRoute),
		routeDurableRecords:    make(map[foundation.RouteID]AutomationRouteDurableRecord),
		routeDurableReferences: make(map[foundation.IdempotencyKey]AutomationRouteDurableRecord),
		references:             make(map[foundation.IdempotencyKey]SettlementReferenceRecord),
		buildingReferences:     make(map[foundation.IdempotencyKey]BuildingMutationReferenceRecord),
	}
}

// Clone returns a detached copy of all production, storage, building, and route
// state. It is intended for read-side dry-runs and reconciliation tools.
func (store *InMemoryStore) Clone() *InMemoryStore {
	cloned := NewInMemoryStore()
	if store == nil {
		return cloned
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	for planetID, state := range store.states {
		cloned.states[planetID] = cloneProductionState(state)
	}
	for planetID, storage := range store.storage {
		cloned.storage[planetID] = clonePlanetStorage(storage)
	}
	for planetID, buildings := range store.buildings {
		if len(buildings) == 0 {
			continue
		}
		cloned.buildings[planetID] = make(map[BuildingID]PlanetBuilding, len(buildings))
		for buildingID, building := range buildings {
			cloned.buildings[planetID][buildingID] = clonePlanetBuilding(building)
		}
	}
	for routeID, route := range store.routes {
		cloned.routes[routeID] = cloneAutomationRoute(route)
	}
	for routeID, record := range store.routeDurableRecords {
		cloned.routeDurableRecords[routeID] = cloneAutomationRouteDurableRecord(record)
	}
	for referenceKey, record := range store.routeDurableReferences {
		cloned.routeDurableReferences[referenceKey] = cloneAutomationRouteDurableRecord(record)
	}
	cloned.events = cloneProductionEventEnvelopes(store.events)
	cloned.nextEventSequence = store.nextEventSequence
	for referenceKey, reference := range store.references {
		cloned.references[referenceKey] = cloneSettlementReferenceRecord(reference)
	}
	for referenceKey, reference := range store.buildingReferences {
		cloned.buildingReferences[referenceKey] = cloneBuildingMutationReferenceRecord(reference)
	}
	cloned.buildingMaterialLedger = cloneBuildingMaterialLedgerEntries(store.buildingMaterialLedger)
	cloned.nextBuildingLedgerSequence = store.nextBuildingLedgerSequence
	cloned.routeStorageLedger = cloneRouteStorageLedgerEntries(store.routeStorageLedger)
	cloned.nextRouteLedgerSequence = store.nextRouteLedgerSequence
	cloned.outbox = cloneProductionOutboxRecords(store.outbox)
	cloned.nextOutboxSequence = store.nextOutboxSequence
	return cloned
}

// InitializePlanetProduction stores default state and empty storage once.
func (store *InMemoryStore) InitializePlanetProduction(input InitializePlanetProductionInput) (InitializePlanetProductionResult, error) {
	if err := input.Validate(); err != nil {
		return InitializePlanetProductionResult{}, err
	}
	state, err := NewPlanetProductionState(input.PlanetID, input.LastCalculatedAt, input.EnergyCapacityPerHour, input.UpdatedAt)
	if err != nil {
		return InitializePlanetProductionResult{}, err
	}
	storage, err := NewPlanetStorage(input.PlanetID, input.StorageCapacityUnits, nil, input.UpdatedAt)
	if err != nil {
		return InitializePlanetProductionResult{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if existing, ok := store.snapshotLocked(input.PlanetID); ok {
		return InitializePlanetProductionResult{Snapshot: existing}, nil
	}
	store.states[input.PlanetID] = cloneProductionState(state)
	store.storage[input.PlanetID] = clonePlanetStorage(storage)
	snapshot, _ := store.snapshotLocked(input.PlanetID)
	return InitializePlanetProductionResult{Snapshot: snapshot, Created: true}, nil
}

// SaveProductionState validates and replaces one planet production state row.
func (store *InMemoryStore) SaveProductionState(state PlanetProductionState) error {
	if err := state.Validate(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	store.states[state.PlanetID] = cloneProductionState(state)
	return nil
}

// ProductionState returns one production state by planet id.
func (store *InMemoryStore) ProductionState(planetID foundation.PlanetID) (PlanetProductionState, bool, error) {
	if err := planetID.Validate(); err != nil {
		return PlanetProductionState{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	state, ok := store.states[planetID]
	if !ok {
		return PlanetProductionState{}, false, nil
	}
	return cloneProductionState(state), true, nil
}

// SavePlanetStorage validates and replaces one planet storage aggregate.
func (store *InMemoryStore) SavePlanetStorage(storage PlanetStorage) error {
	if err := storage.Validate(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	store.storage[storage.PlanetID] = clonePlanetStorage(storage)
	return nil
}

// PlanetStorage returns one storage aggregate by planet id.
func (store *InMemoryStore) PlanetStorage(planetID foundation.PlanetID) (PlanetStorage, bool, error) {
	if err := planetID.Validate(); err != nil {
		return PlanetStorage{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	storage, ok := store.storage[planetID]
	if !ok {
		return PlanetStorage{}, false, nil
	}
	return clonePlanetStorage(storage), true, nil
}

// UpsertBuilding validates and stores one planet building row.
func (store *InMemoryStore) UpsertBuilding(building PlanetBuilding) (PlanetBuilding, bool, error) {
	if err := building.Validate(); err != nil {
		return PlanetBuilding{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if store.buildings[building.PlanetID] == nil {
		store.buildings[building.PlanetID] = make(map[BuildingID]PlanetBuilding)
	}
	_, existed := store.buildings[building.PlanetID][building.BuildingID]
	store.buildings[building.PlanetID][building.BuildingID] = clonePlanetBuilding(building)
	return clonePlanetBuilding(building), !existed, nil
}

// Building returns one planet building by id.
func (store *InMemoryStore) Building(planetID foundation.PlanetID, buildingID BuildingID) (PlanetBuilding, bool, error) {
	if err := planetID.Validate(); err != nil {
		return PlanetBuilding{}, false, err
	}
	if err := buildingID.Validate(); err != nil {
		return PlanetBuilding{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	building, ok := store.buildings[planetID][buildingID]
	if !ok {
		return PlanetBuilding{}, false, nil
	}
	return clonePlanetBuilding(building), true, nil
}

// Buildings returns all buildings for a planet in deterministic building id order.
func (store *InMemoryStore) Buildings(planetID foundation.PlanetID) ([]PlanetBuilding, error) {
	if err := planetID.Validate(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	return buildingsFromMap(store.buildings[planetID], func(PlanetBuilding) bool { return true }), nil
}

// ActiveBuildings returns active buildings for a planet in deterministic order.
func (store *InMemoryStore) ActiveBuildings(planetID foundation.PlanetID) ([]PlanetBuilding, error) {
	if err := planetID.Validate(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	return buildingsFromMap(store.buildings[planetID], func(building PlanetBuilding) bool {
		return building.State == BuildingStateActive
	}), nil
}

// AutomationRoute returns one route by id.
func (store *InMemoryStore) AutomationRoute(routeID foundation.RouteID) (AutomationRoute, bool, error) {
	if err := routeID.Validate(); err != nil {
		return AutomationRoute{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	route, ok := store.routes[routeID]
	if !ok {
		return AutomationRoute{}, false, nil
	}
	return cloneAutomationRoute(route), true, nil
}

// AutomationRoutes returns all routes in deterministic route id order.
func (store *InMemoryStore) AutomationRoutes() []AutomationRoute {
	store.mu.RLock()
	defer store.mu.RUnlock()

	return routesFromMap(store.routes)
}

// Events returns production event envelopes in append order.
func (store *InMemoryStore) Events() []gameevents.EventEnvelope {
	store.mu.RLock()
	defer store.mu.RUnlock()

	return cloneProductionEventEnvelopes(store.events)
}

// Snapshot returns one validated aggregate snapshot by planet id.
func (store *InMemoryStore) Snapshot(planetID foundation.PlanetID) (PlanetProductionSnapshot, bool, error) {
	if err := planetID.Validate(); err != nil {
		return PlanetProductionSnapshot{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	snapshot, ok := store.snapshotLocked(planetID)
	if !ok {
		return PlanetProductionSnapshot{}, false, nil
	}
	return snapshot, true, nil
}

// Snapshots returns all complete planet production snapshots in planet id order.
func (store *InMemoryStore) Snapshots() []PlanetProductionSnapshot {
	store.mu.RLock()
	defer store.mu.RUnlock()

	planetIDs := make([]foundation.PlanetID, 0, len(store.states))
	for planetID := range store.states {
		planetIDs = append(planetIDs, planetID)
	}
	sort.Slice(planetIDs, func(i, j int) bool {
		return planetIDs[i] < planetIDs[j]
	})

	snapshots := make([]PlanetProductionSnapshot, 0, len(planetIDs))
	for _, planetID := range planetIDs {
		if snapshot, ok := store.snapshotLocked(planetID); ok {
			snapshots = append(snapshots, snapshot)
		}
	}
	return snapshots
}

// Validate reports whether input can initialize primitive production rows.
func (input InitializePlanetProductionInput) Validate() error {
	if err := input.PlanetID.Validate(); err != nil {
		return err
	}
	if input.LastCalculatedAt.IsZero() {
		return fmt.Errorf("last_calculated_at: %w", ErrZeroProductionTimestamp)
	}
	if err := validatePositiveBoundedAmount("storage capacity", input.StorageCapacityUnits, ErrInvalidStorageCapacity); err != nil {
		return err
	}
	if err := validateNonNegativeBoundedAmount("energy capacity per hour", input.EnergyCapacityPerHour, ErrInvalidEnergyRate); err != nil {
		return err
	}
	if input.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at: %w", ErrZeroProductionTimestamp)
	}
	return nil
}

func (store *InMemoryStore) ensureMapsLocked() {
	if store.states == nil {
		store.states = make(map[foundation.PlanetID]PlanetProductionState)
	}
	if store.storage == nil {
		store.storage = make(map[foundation.PlanetID]PlanetStorage)
	}
	if store.buildings == nil {
		store.buildings = make(map[foundation.PlanetID]map[BuildingID]PlanetBuilding)
	}
	if store.routes == nil {
		store.routes = make(map[foundation.RouteID]AutomationRoute)
	}
	ensureAutomationRouteDurableMaps(&store.routeDurableRecords, &store.routeDurableReferences)
	if store.references == nil {
		store.references = make(map[foundation.IdempotencyKey]SettlementReferenceRecord)
	}
	if store.buildingReferences == nil {
		store.buildingReferences = make(map[foundation.IdempotencyKey]BuildingMutationReferenceRecord)
	}
}

func (store *InMemoryStore) insertAutomationRoute(route AutomationRoute) (AutomationRoute, error) {
	result, err := store.ApplyRouteCreateTransaction(RouteCreateTransactionInput{
		Route: route,
	})
	if err != nil {
		return AutomationRoute{}, err
	}
	return cloneAutomationRoute(result.Route), nil
}

func (store *InMemoryStore) appendProductionEventLocked(eventType EventType, payload any, occurredAt time.Time) (gameevents.EventEnvelope, error) {
	return store.appendProductionEventWithOutboxEvidenceLocked(eventType, payload, occurredAt, "", "")
}

func (store *InMemoryStore) appendProductionEventWithOutboxEvidenceLocked(
	eventType EventType,
	payload any,
	occurredAt time.Time,
	referenceKey foundation.IdempotencyKey,
	settlementWindow string,
) (gameevents.EventEnvelope, error) {
	store.nextEventSequence++
	sequence := store.nextEventSequence
	eventID := foundation.EventID(fmt.Sprintf("production-event-%d", sequence))
	event, err := NewProductionEvent(eventID, eventType, payload, occurredAt, sequence)
	if err != nil {
		store.nextEventSequence--
		return gameevents.EventEnvelope{}, err
	}
	store.events = append(store.events, event)
	store.appendOutboxRecordLocked(event, payload, referenceKey, settlementWindow)
	return event, nil
}

func (store *InMemoryStore) snapshotLocked(planetID foundation.PlanetID) (PlanetProductionSnapshot, bool) {
	state, hasState := store.states[planetID]
	storage, hasStorage := store.storage[planetID]
	if !hasState || !hasStorage {
		return PlanetProductionSnapshot{}, false
	}
	snapshot := PlanetProductionSnapshot{
		State:     cloneProductionState(state),
		Storage:   clonePlanetStorage(storage),
		Buildings: buildingsFromMap(store.buildings[planetID], func(PlanetBuilding) bool { return true }),
	}
	return cloneProductionSnapshot(snapshot), true
}

func buildingsFromMap(buildings map[BuildingID]PlanetBuilding, include func(PlanetBuilding) bool) []PlanetBuilding {
	if len(buildings) == 0 {
		return nil
	}
	cloned := make([]PlanetBuilding, 0, len(buildings))
	for _, building := range buildings {
		if include(building) {
			cloned = append(cloned, clonePlanetBuilding(building))
		}
	}
	sortPlanetBuildings(cloned)
	return cloned
}

func routesFromMap(routes map[foundation.RouteID]AutomationRoute) []AutomationRoute {
	if len(routes) == 0 {
		return nil
	}
	cloned := make([]AutomationRoute, 0, len(routes))
	for _, route := range routes {
		cloned = append(cloned, cloneAutomationRoute(route))
	}
	sort.Slice(cloned, func(i, j int) bool {
		return cloned[i].RouteID < cloned[j].RouteID
	})
	return cloned
}

func cloneProductionEventEnvelopes(envelopes []gameevents.EventEnvelope) []gameevents.EventEnvelope {
	if len(envelopes) == 0 {
		return nil
	}
	cloned := make([]gameevents.EventEnvelope, len(envelopes))
	for index, envelope := range envelopes {
		cloned[index] = cloneProductionEventEnvelope(envelope)
	}
	return cloned
}

func cloneProductionEventEnvelope(envelope gameevents.EventEnvelope) gameevents.EventEnvelope {
	cloned := envelope
	cloned.Payload = append([]byte(nil), envelope.Payload...)
	return cloned
}

func validatePositiveBoundedAmount(name string, amount int64, sentinel error) error {
	if amount <= 0 {
		return fmt.Errorf("%s %d: %w", name, amount, sentinel)
	}
	if amount > foundation.MaxAmount {
		return fmt.Errorf("%s %d exceeds max %d: %w", name, amount, foundation.MaxAmount, sentinel)
	}
	return nil
}

func validateNonNegativeBoundedAmount(name string, amount int64, sentinel error) error {
	if amount < 0 {
		return fmt.Errorf("%s %d: %w", name, amount, sentinel)
	}
	if amount > foundation.MaxAmount {
		return fmt.Errorf("%s %d exceeds max %d: %w", name, amount, foundation.MaxAmount, sentinel)
	}
	return nil
}
