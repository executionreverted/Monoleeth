package discovery

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

// MaterializePlanetInput is the repository command for committing one
// server-confirmed procedural planet candidate into persistent state.
type MaterializePlanetInput struct {
	CandidateKey PlanetMaterializationKey
	Planet       Planet
}

// MaterializePlanetResult reports whether a planet row was newly stored.
type MaterializePlanetResult struct {
	Planet  Planet
	Created bool
}

// PlanetOwnerChangeInput records a global owner update from a future claim flow.
type PlanetOwnerChangeInput struct {
	PlanetID         foundation.PlanetID
	NewOwnerPlayerID foundation.PlayerID
	ChangedAt        time.Time
	SourceReference  string
}

// PlanetOwnerChangeResult reports owner state and intel records marked stale.
type PlanetOwnerChangeResult struct {
	Planet     Planet
	Changed    bool
	StaleIntel []PlayerPlanetIntel
}

// InMemoryStore is a mutex-protected discovery repository for early server slices.
type InMemoryStore struct {
	mu sync.RWMutex

	planets                 map[foundation.PlanetID]Planet
	planetIDsByCandidateKey map[PlanetMaterializationKey]foundation.PlanetID
	intel                   map[playerPlanetIntelKey]PlayerPlanetIntel
	intelPlayersByPlanet    map[foundation.PlanetID]map[foundation.PlayerID]struct{}
	claimBoundaries         map[PlanetClaimReference]ClaimBoundaryRecord
}

type playerPlanetIntelKey struct {
	playerID foundation.PlayerID
	planetID foundation.PlanetID
}

// NewInMemoryStore returns an empty discovery repository.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		planets:                 make(map[foundation.PlanetID]Planet),
		planetIDsByCandidateKey: make(map[PlanetMaterializationKey]foundation.PlanetID),
		intel:                   make(map[playerPlanetIntelKey]PlayerPlanetIntel),
		intelPlayersByPlanet:    make(map[foundation.PlanetID]map[foundation.PlayerID]struct{}),
		claimBoundaries:         make(map[PlanetClaimReference]ClaimBoundaryRecord),
	}
}

// MaterializePlanet stores a discovered planet once. If the candidate key or
// planet id was already materialized, the existing planet is returned unchanged.
func (store *InMemoryStore) MaterializePlanet(input MaterializePlanetInput) (MaterializePlanetResult, error) {
	if err := input.Validate(); err != nil {
		return MaterializePlanetResult{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if existing, ok := store.planets[input.Planet.ID]; ok {
		return MaterializePlanetResult{Planet: clonePlanet(existing)}, nil
	}
	if !input.CandidateKey.IsZero() {
		if planetID, ok := store.planetIDsByCandidateKey[input.CandidateKey]; ok {
			if existing, ok := store.planets[planetID]; ok {
				return MaterializePlanetResult{Planet: clonePlanet(existing)}, nil
			}
		}
	}

	planet := clonePlanet(input.Planet)
	planet.DiscoveredAt = planet.DiscoveredAt.UTC()
	if planet.OwnerChangedAt != nil {
		ownerChangedAt := planet.OwnerChangedAt.UTC()
		planet.OwnerChangedAt = &ownerChangedAt
	}
	store.planets[planet.ID] = clonePlanet(planet)
	if !input.CandidateKey.IsZero() {
		store.planetIDsByCandidateKey[input.CandidateKey] = planet.ID
	}
	return MaterializePlanetResult{Planet: clonePlanet(planet), Created: true}, nil
}

// Planet returns one materialized planet by id.
func (store *InMemoryStore) Planet(planetID foundation.PlanetID) (Planet, bool, error) {
	if err := planetID.Validate(); err != nil {
		return Planet{}, false, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	planet, ok := store.planets[planetID]
	if !ok {
		return Planet{}, false, nil
	}
	return clonePlanet(planet), true, nil
}

// PlanetByCandidateKey returns the planet already materialized for key.
func (store *InMemoryStore) PlanetByCandidateKey(key PlanetMaterializationKey) (Planet, bool, error) {
	if err := key.Validate(); err != nil {
		return Planet{}, false, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	planetID, ok := store.planetIDsByCandidateKey[key]
	if !ok {
		return Planet{}, false, nil
	}
	planet, ok := store.planets[planetID]
	if !ok {
		return Planet{}, false, nil
	}
	return clonePlanet(planet), true, nil
}

// Planets returns all materialized planets in deterministic planet id order.
func (store *InMemoryStore) Planets() []Planet {
	store.mu.RLock()
	defer store.mu.RUnlock()

	planetIDs := make([]foundation.PlanetID, 0, len(store.planets))
	for planetID := range store.planets {
		planetIDs = append(planetIDs, planetID)
	}
	sort.Slice(planetIDs, func(i, j int) bool {
		return planetIDs[i] < planetIDs[j]
	})

	planets := make([]Planet, 0, len(planetIDs))
	for _, planetID := range planetIDs {
		planets = append(planets, clonePlanet(store.planets[planetID]))
	}
	return planets
}

// UpsertPlayerPlanetIntel writes personal planet memory unless an existing row
// is fresher or more trustworthy for the same last_seen_at.
func (store *InMemoryStore) UpsertPlayerPlanetIntel(incoming PlayerPlanetIntel) (PlayerPlanetIntel, bool, error) {
	if err := incoming.Validate(); err != nil {
		return PlayerPlanetIntel{}, false, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	key := newPlayerPlanetIntelKey(incoming.PlayerID, incoming.PlanetID)
	if existing, ok := store.intel[key]; ok {
		if !shouldReplacePlayerPlanetIntel(existing, incoming) {
			return clonePlayerPlanetIntel(existing), false, nil
		}
	}

	next := clonePlayerPlanetIntel(incoming)
	next.LastSeenAt = next.LastSeenAt.UTC()
	store.intel[key] = clonePlayerPlanetIntel(next)
	store.addPlanetIntelPlayerLocked(next.PlanetID, next.PlayerID)
	return clonePlayerPlanetIntel(next), true, nil
}

// PlayerPlanetIntel returns one player's known intel for a planet.
func (store *InMemoryStore) PlayerPlanetIntel(playerID foundation.PlayerID, planetID foundation.PlanetID) (PlayerPlanetIntel, bool, error) {
	if err := playerID.Validate(); err != nil {
		return PlayerPlanetIntel{}, false, err
	}
	if err := planetID.Validate(); err != nil {
		return PlayerPlanetIntel{}, false, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	intel, ok := store.intel[newPlayerPlanetIntelKey(playerID, planetID)]
	if !ok {
		return PlayerPlanetIntel{}, false, nil
	}
	return clonePlayerPlanetIntel(intel), true, nil
}

// PlayerPlanetIntelRecords returns all intel rows for playerID in planet id order.
func (store *InMemoryStore) PlayerPlanetIntelRecords(playerID foundation.PlayerID) ([]PlayerPlanetIntel, error) {
	if err := playerID.Validate(); err != nil {
		return nil, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	planetIDs := make([]foundation.PlanetID, 0)
	for key := range store.intel {
		if key.playerID == playerID {
			planetIDs = append(planetIDs, key.planetID)
		}
	}
	sort.Slice(planetIDs, func(i, j int) bool {
		return planetIDs[i] < planetIDs[j]
	})

	records := make([]PlayerPlanetIntel, 0, len(planetIDs))
	for _, planetID := range planetIDs {
		records = append(records, clonePlayerPlanetIntel(store.intel[newPlayerPlanetIntelKey(playerID, planetID)]))
	}
	return records, nil
}

// RecordPlanetOwnerChange updates a planet owner and marks older personal intel
// stale. It is a repository hook for the future claim/event path, not a claim service.
func (store *InMemoryStore) RecordPlanetOwnerChange(input PlanetOwnerChangeInput) (PlanetOwnerChangeResult, error) {
	if err := input.Validate(); err != nil {
		return PlanetOwnerChangeResult{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	planet, ok := store.planets[input.PlanetID]
	if !ok {
		return PlanetOwnerChangeResult{}, fmt.Errorf("planet %q: %w", input.PlanetID, ErrUnknownPlanet)
	}
	if planet.OwnerPlayerID == input.NewOwnerPlayerID {
		return PlanetOwnerChangeResult{Planet: clonePlanet(planet)}, nil
	}
	if !planet.OwnerPlayerID.IsZero() {
		return PlanetOwnerChangeResult{}, fmt.Errorf("planet %q: %w", input.PlanetID, ErrPlanetAlreadyOwned)
	}
	changedAt := input.ChangedAt.UTC()
	if planet.OwnerChangedAt != nil && !changedAt.After(*planet.OwnerChangedAt) {
		return PlanetOwnerChangeResult{Planet: clonePlanet(planet)}, nil
	}

	planet.OwnerPlayerID = input.NewOwnerPlayerID
	planet.OwnerChangedAt = &changedAt
	if err := planet.Validate(); err != nil {
		return PlanetOwnerChangeResult{}, err
	}
	store.planets[input.PlanetID] = clonePlanet(planet)

	staleIntel := store.markPlanetIntelStaleLocked(input.PlanetID, changedAt, input.SourceReference)
	return PlanetOwnerChangeResult{
		Planet:     clonePlanet(planet),
		Changed:    true,
		StaleIntel: staleIntel,
	}, nil
}

// Validate reports whether input is a valid materialization command.
func (input MaterializePlanetInput) Validate() error {
	if !input.CandidateKey.IsZero() {
		if err := input.CandidateKey.Validate(); err != nil {
			return err
		}
	}
	if err := input.Planet.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether input is a valid owner-change event.
func (input PlanetOwnerChangeInput) Validate() error {
	if err := input.PlanetID.Validate(); err != nil {
		return err
	}
	if err := input.NewOwnerPlayerID.Validate(); err != nil {
		return err
	}
	if input.ChangedAt.IsZero() {
		return fmt.Errorf("changed_at: %w", ErrZeroPlanetTimestamp)
	}
	if !validDiscoveryToken(input.SourceReference) {
		return fmt.Errorf("source_reference %q: %w", input.SourceReference, ErrEmptyOwnerChangeReference)
	}
	return nil
}

// IsZero reports whether key is omitted.
func (key PlanetMaterializationKey) IsZero() bool {
	return key == ""
}

func (store *InMemoryStore) ensureMapsLocked() {
	if store.planets == nil {
		store.planets = make(map[foundation.PlanetID]Planet)
	}
	if store.planetIDsByCandidateKey == nil {
		store.planetIDsByCandidateKey = make(map[PlanetMaterializationKey]foundation.PlanetID)
	}
	if store.intel == nil {
		store.intel = make(map[playerPlanetIntelKey]PlayerPlanetIntel)
	}
	if store.intelPlayersByPlanet == nil {
		store.intelPlayersByPlanet = make(map[foundation.PlanetID]map[foundation.PlayerID]struct{})
	}
	if store.claimBoundaries == nil {
		store.claimBoundaries = make(map[PlanetClaimReference]ClaimBoundaryRecord)
	}
}

func (store *InMemoryStore) addPlanetIntelPlayerLocked(planetID foundation.PlanetID, playerID foundation.PlayerID) {
	if store.intelPlayersByPlanet[planetID] == nil {
		store.intelPlayersByPlanet[planetID] = make(map[foundation.PlayerID]struct{})
	}
	store.intelPlayersByPlanet[planetID][playerID] = struct{}{}
}

func (store *InMemoryStore) markPlanetIntelStaleLocked(
	planetID foundation.PlanetID,
	changedAt time.Time,
	sourceReference string,
) []PlayerPlanetIntel {
	playerIDs := make([]foundation.PlayerID, 0, len(store.intelPlayersByPlanet[planetID]))
	for playerID := range store.intelPlayersByPlanet[planetID] {
		playerIDs = append(playerIDs, playerID)
	}
	sort.Slice(playerIDs, func(i, j int) bool {
		return playerIDs[i] < playerIDs[j]
	})

	staleIntel := make([]PlayerPlanetIntel, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		key := newPlayerPlanetIntelKey(playerID, planetID)
		existing, ok := store.intel[key]
		if !ok || !existing.LastSeenAt.Before(changedAt) {
			continue
		}
		next := staleMarkedIntel(existing, changedAt, sourceReference)
		store.intel[key] = clonePlayerPlanetIntel(next)
		staleIntel = append(staleIntel, clonePlayerPlanetIntel(next))
	}
	return staleIntel
}

func newPlayerPlanetIntelKey(playerID foundation.PlayerID, planetID foundation.PlanetID) playerPlanetIntelKey {
	return playerPlanetIntelKey{playerID: playerID, planetID: planetID}
}
