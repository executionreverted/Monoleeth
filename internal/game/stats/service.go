package stats

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

var (
	ErrNilStatInputProvider = errors.New("nil stat input provider")
	ErrUnknownStatSubject   = errors.New("unknown stat subject")
)

// ModuleModifier describes one equipped module's stat contribution plus the
// minimum metadata needed for stat aggregation to ignore broken modules.
type ModuleModifier struct {
	SourceID string       `json:"source_id"`
	Broken   bool         `json:"broken"`
	Flat     FlatStats    `json:"flat"`
	Percent  PercentStats `json:"percent"`
}

// StatBuildInput is the service boundary used by ship, module, progression,
// role, and temporary-effect systems once they own those records. It does not
// carry player or ship ids; those come from the StatSubject requested by
// gameplay systems.
type StatBuildInput struct {
	BaseShip EffectiveStats `json:"base_ship"`

	Modules         []ModuleModifier  `json:"modules"`
	FlatPassives    []FlatModifier    `json:"flat_passives"`
	RoleBonuses     []FlatModifier    `json:"role_bonuses"`
	PercentPassives []PercentModifier `json:"percent_passives"`

	TemporaryModifiers []TemporaryModifier `json:"temporary_modifiers"`
}

// AggregationInput returns the pure aggregation payload after removing broken
// equipped modules.
func (input StatBuildInput) AggregationInput() AggregationInput {
	flatModules := make([]FlatModifier, 0, len(input.Modules))
	percentModules := make([]PercentModifier, 0, len(input.Modules))
	for _, module := range input.Modules {
		if module.Broken {
			continue
		}
		flatModules = append(flatModules, FlatModifier{
			Source:   ModifierSourceModule,
			SourceID: module.SourceID,
			Stats:    module.Flat,
		})
		percentModules = append(percentModules, PercentModifier{
			Source:   ModifierSourceModule,
			SourceID: module.SourceID,
			Stats:    module.Percent,
		})
	}

	return AggregationInput{
		BaseShip:           input.BaseShip,
		FlatModules:        flatModules,
		FlatPassives:       append([]FlatModifier(nil), input.FlatPassives...),
		RoleBonuses:        append([]FlatModifier(nil), input.RoleBonuses...),
		PercentModules:     percentModules,
		PercentPassives:    append([]PercentModifier(nil), input.PercentPassives...),
		TemporaryModifiers: append([]TemporaryModifier(nil), input.TemporaryModifiers...),
	}
}

// StatSubject identifies one player ship whose effective stats are requested.
type StatSubject struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	ShipID   foundation.ShipID   `json:"ship_id"`
}

// NewStatSubject returns a stat subject key.
func NewStatSubject(playerID foundation.PlayerID, shipID foundation.ShipID) StatSubject {
	return StatSubject{PlayerID: playerID, ShipID: shipID}
}

func (subject StatSubject) validate() error {
	if err := subject.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := subject.ShipID.Validate(); err != nil {
		return fmt.Errorf("ship_id: %w", err)
	}
	return nil
}

// StatInputProvider builds authoritative aggregation inputs for a stat subject.
type StatInputProvider interface {
	BuildStatsInput(subject StatSubject) (StatBuildInput, error)
}

// StaticStatInputProvider is a deterministic provider for tests and early
// single-process slices.
type StaticStatInputProvider map[StatSubject]StatBuildInput

// InvalidateStatsInput marks the active snapshot for one player ship stale.
type InvalidateStatsInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	ShipID   foundation.ShipID   `json:"ship_id"`
	Reason   InvalidationReason  `json:"reason"`
}

func (input InvalidateStatsInput) validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.ShipID.Validate(); err != nil {
		return fmt.Errorf("ship_id: %w", err)
	}
	if input.Reason == "" {
		return fmt.Errorf("reason: %w", foundation.ErrEmptyID)
	}
	return nil
}

// StatSnapshotKey identifies one immutable snapshot version.
type StatSnapshotKey struct {
	PlayerID foundation.PlayerID
	ShipID   foundation.ShipID
	Version  SnapshotVersion
}

// NewStatSnapshotKey returns the cache/store key for snapshot.
func NewStatSnapshotKey(snapshot StatSnapshot) StatSnapshotKey {
	return StatSnapshotKey{
		PlayerID: snapshot.PlayerID,
		ShipID:   snapshot.ShipID,
		Version:  snapshot.Version,
	}
}

type statSubjectKey struct {
	playerID foundation.PlayerID
	shipID   foundation.ShipID
}

func newStatSubjectKey(playerID foundation.PlayerID, shipID foundation.ShipID) statSubjectKey {
	return statSubjectKey{playerID: playerID, shipID: shipID}
}

// StatSnapshotStore stores durable snapshot and invalidation metadata.
type StatSnapshotStore interface {
	GetSnapshot(key StatSnapshotKey) (StatSnapshot, bool)
	SaveSnapshot(snapshot StatSnapshot) error
	GetInvalidationState(playerID foundation.PlayerID, shipID foundation.ShipID) (InvalidationState, bool)
	SaveInvalidationState(playerID foundation.PlayerID, shipID foundation.ShipID, state InvalidationState) error
}

// ActiveStatCache stores active-session snapshots keyed by player, ship, and version.
type ActiveStatCache interface {
	Get(key StatSnapshotKey) (StatSnapshot, bool)
	Put(snapshot StatSnapshot)
}

// StatService owns server-side effective stat snapshot lookup and recalculation.
type StatService struct {
	mu        sync.Mutex
	clock     foundation.Clock
	snapshots StatSnapshotStore
	cache     ActiveStatCache
	inputs    StatInputProvider
}

// NewStatService returns a stat service with explicit snapshot storage and
// active-session cache dependencies.
func NewStatService(
	clock foundation.Clock,
	snapshots StatSnapshotStore,
	cache ActiveStatCache,
	inputs StatInputProvider,
) (*StatService, error) {
	if inputs == nil {
		return nil, ErrNilStatInputProvider
	}
	if clock == nil {
		clock = foundation.RealClock{}
	}
	if snapshots == nil {
		snapshots = NewInMemoryStatSnapshotStore()
	}
	if cache == nil {
		cache = NewInMemoryActiveStatCache()
	}
	return &StatService{
		clock:     clock,
		snapshots: snapshots,
		cache:     cache,
		inputs:    inputs,
	}, nil
}

// GetEffectiveStats returns the current valid effective stat snapshot. If the
// stored snapshot or invalidation state says the current version is stale, the
// service recalculates from the authoritative input provider and advances the version.
func (service *StatService) GetEffectiveStats(subject StatSubject) (StatSnapshot, error) {
	if err := subject.validate(); err != nil {
		return StatSnapshot{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	state, hasState := service.snapshots.GetInvalidationState(subject.PlayerID, subject.ShipID)
	if hasState && !state.Invalidated && state.CurrentVersion > 0 {
		key := StatSnapshotKey{
			PlayerID: subject.PlayerID,
			ShipID:   subject.ShipID,
			Version:  state.CurrentVersion,
		}
		if snapshot, ok := service.snapshots.GetSnapshot(key); ok && !snapshot.IsInvalidated() {
			if cached, ok := service.cache.Get(key); ok && !cached.IsInvalidated() {
				return cached, nil
			}
			service.cache.Put(snapshot)
			return snapshot, nil
		}
	}

	input, err := service.inputs.BuildStatsInput(subject)
	if err != nil {
		return StatSnapshot{}, err
	}
	now := service.clock.Now()
	version := SnapshotVersion(1)
	if hasState && state.CurrentVersion > 0 {
		version = state.CurrentVersion.Next()
	}
	snapshot := NewStatSnapshot(
		subject.PlayerID,
		subject.ShipID,
		version,
		AggregateStats(input.AggregationInput()),
		now,
	)
	if err := service.snapshots.SaveSnapshot(snapshot); err != nil {
		return StatSnapshot{}, err
	}
	state = state.MarkRecalculated(version, now)
	if err := service.snapshots.SaveInvalidationState(subject.PlayerID, subject.ShipID, state); err != nil {
		return StatSnapshot{}, err
	}
	service.cache.Put(snapshot)
	return snapshot, nil
}

// InvalidateStats marks the current snapshot stale so the next GetEffectiveStats
// call recalculates and writes a new snapshot version.
func (service *StatService) InvalidateStats(input InvalidateStatsInput) error {
	if err := input.validate(); err != nil {
		return err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	now := service.clock.Now()
	state, hasState := service.snapshots.GetInvalidationState(input.PlayerID, input.ShipID)
	if hasState && state.CurrentVersion > 0 {
		key := StatSnapshotKey{
			PlayerID: input.PlayerID,
			ShipID:   input.ShipID,
			Version:  state.CurrentVersion,
		}
		if snapshot, ok := service.snapshots.GetSnapshot(key); ok && !snapshot.IsInvalidated() {
			if err := service.snapshots.SaveSnapshot(snapshot.Invalidate(now)); err != nil {
				return err
			}
		}
	}

	state = state.Invalidate(input.Reason, now)
	return service.snapshots.SaveInvalidationState(input.PlayerID, input.ShipID, state)
}

// BuildStatsInput returns a cloned static stat build input for subject.
func (provider StaticStatInputProvider) BuildStatsInput(subject StatSubject) (StatBuildInput, error) {
	if err := subject.validate(); err != nil {
		return StatBuildInput{}, err
	}
	input, ok := provider[subject]
	if !ok {
		return StatBuildInput{}, fmt.Errorf("%s/%s: %w", subject.PlayerID, subject.ShipID, ErrUnknownStatSubject)
	}
	return cloneStatBuildInput(input), nil
}

// InMemoryStatSnapshotStore is a deterministic store implementation for tests
// and early in-process gameplay slices.
type InMemoryStatSnapshotStore struct {
	mu            sync.RWMutex
	snapshots     map[StatSnapshotKey]StatSnapshot
	invalidations map[statSubjectKey]InvalidationState
}

// NewInMemoryStatSnapshotStore returns an empty in-memory snapshot store.
func NewInMemoryStatSnapshotStore() *InMemoryStatSnapshotStore {
	return &InMemoryStatSnapshotStore{
		snapshots:     make(map[StatSnapshotKey]StatSnapshot),
		invalidations: make(map[statSubjectKey]InvalidationState),
	}
}

// GetSnapshot returns a snapshot by player, ship, and version.
func (store *InMemoryStatSnapshotStore) GetSnapshot(key StatSnapshotKey) (StatSnapshot, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	snapshot, ok := store.snapshots[key]
	return cloneStatSnapshot(snapshot), ok
}

// SaveSnapshot stores a snapshot by player, ship, and version.
func (store *InMemoryStatSnapshotStore) SaveSnapshot(snapshot StatSnapshot) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.snapshots[NewStatSnapshotKey(snapshot)] = cloneStatSnapshot(snapshot)
	return nil
}

// GetInvalidationState returns invalidation metadata for one player ship.
func (store *InMemoryStatSnapshotStore) GetInvalidationState(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
) (InvalidationState, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	state, ok := store.invalidations[newStatSubjectKey(playerID, shipID)]
	return cloneInvalidationState(state), ok
}

// SaveInvalidationState stores invalidation metadata for one player ship.
func (store *InMemoryStatSnapshotStore) SaveInvalidationState(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	state InvalidationState,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.invalidations[newStatSubjectKey(playerID, shipID)] = cloneInvalidationState(state)
	return nil
}

// InMemoryActiveStatCache is an active-session cache keyed by player, ship, and version.
type InMemoryActiveStatCache struct {
	mu        sync.RWMutex
	snapshots map[StatSnapshotKey]StatSnapshot
}

// NewInMemoryActiveStatCache returns an empty active-session stat cache.
func NewInMemoryActiveStatCache() *InMemoryActiveStatCache {
	return &InMemoryActiveStatCache{
		snapshots: make(map[StatSnapshotKey]StatSnapshot),
	}
}

// Get returns a cached snapshot by player, ship, and version.
func (cache *InMemoryActiveStatCache) Get(key StatSnapshotKey) (StatSnapshot, bool) {
	cache.mu.RLock()
	defer cache.mu.RUnlock()

	snapshot, ok := cache.snapshots[key]
	return cloneStatSnapshot(snapshot), ok
}

// Put stores a snapshot in the active-session cache.
func (cache *InMemoryActiveStatCache) Put(snapshot StatSnapshot) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.snapshots[NewStatSnapshotKey(snapshot)] = cloneStatSnapshot(snapshot)
}

func cloneStatBuildInput(input StatBuildInput) StatBuildInput {
	input.Modules = append([]ModuleModifier(nil), input.Modules...)
	input.FlatPassives = append([]FlatModifier(nil), input.FlatPassives...)
	input.RoleBonuses = append([]FlatModifier(nil), input.RoleBonuses...)
	input.PercentPassives = append([]PercentModifier(nil), input.PercentPassives...)
	input.TemporaryModifiers = append([]TemporaryModifier(nil), input.TemporaryModifiers...)
	return input
}

func cloneStatSnapshot(snapshot StatSnapshot) StatSnapshot {
	snapshot.InvalidatedAt = cloneTimePtr(snapshot.InvalidatedAt)
	return snapshot
}

func cloneInvalidationState(state InvalidationState) InvalidationState {
	state.InvalidatedAt = cloneTimePtr(state.InvalidatedAt)
	state.LastRecalculatedAt = cloneTimePtr(state.LastRecalculatedAt)
	return state
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
