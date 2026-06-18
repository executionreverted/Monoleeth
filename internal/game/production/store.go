package production

import (
	"fmt"
	"sort"
	"sync"
	"time"

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

	states    map[foundation.PlanetID]PlanetProductionState
	storage   map[foundation.PlanetID]PlanetStorage
	buildings map[foundation.PlanetID]map[BuildingID]PlanetBuilding
}

// NewInMemoryStore returns an empty production repository.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		states:    make(map[foundation.PlanetID]PlanetProductionState),
		storage:   make(map[foundation.PlanetID]PlanetStorage),
		buildings: make(map[foundation.PlanetID]map[BuildingID]PlanetBuilding),
	}
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
