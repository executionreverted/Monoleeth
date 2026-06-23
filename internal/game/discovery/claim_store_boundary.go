package discovery

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"gameproject/internal/game/foundation"
)

var ErrClaimBoundaryNotFound = errors.New("claim boundary not found")

type ClaimBoundaryStatus string

const (
	ClaimBoundaryStatusPendingSideEffects ClaimBoundaryStatus = "pending_side_effects"
	ClaimBoundaryStatusComplete           ClaimBoundaryStatus = "complete"
)

type BeginPlanetClaimBoundaryInput struct {
	ClaimReference  PlanetClaimReference
	PlayerID        foundation.PlayerID
	PlanetID        foundation.PlanetID
	ClaimedAt       time.Time
	EventID         foundation.EventID
	SourceReference string
}

type BeginPlanetClaimBoundaryResult struct {
	Planet     Planet
	Boundary   ClaimBoundaryRecord
	StaleIntel []PlayerPlanetIntel
	Duplicate  bool
}

type CompletePlanetClaimBoundaryInput struct {
	ClaimReference    PlanetClaimReference
	PlayerID          foundation.PlayerID
	PlanetID          foundation.PlanetID
	CompletedAt       time.Time
	StaleListingCount int
}

type CompletePlanetClaimBoundaryResult struct {
	Boundary  ClaimBoundaryRecord
	Duplicate bool
}

// ClaimBoundaryRecord is the store-owned transaction marker for a planet claim
// whose owner CAS has committed but whose post-owner side effects may still be
// pending. It mirrors the shape a durable DB row should use later.
type ClaimBoundaryRecord struct {
	ClaimReference    PlanetClaimReference
	ReferenceKey      foundation.IdempotencyKey
	PlayerID          foundation.PlayerID
	PlanetID          foundation.PlanetID
	Status            ClaimBoundaryStatus
	EventID           foundation.EventID
	ClaimedAt         time.Time
	RecordedAt        time.Time
	CompletedAt       time.Time
	StaleIntelCount   int
	StaleListingCount int
}

func (store *InMemoryStore) BeginPlanetClaimBoundary(input BeginPlanetClaimBoundaryInput) (BeginPlanetClaimBoundaryResult, error) {
	if err := input.Validate(); err != nil {
		return BeginPlanetClaimBoundaryResult{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if existing, ok := store.claimBoundaries[input.ClaimReference]; ok {
		if !claimBoundaryMatches(existing, input.PlayerID, input.PlanetID) {
			return BeginPlanetClaimBoundaryResult{}, ErrPlanetClaimReferenceConflict
		}
		planet, ok := store.planets[input.PlanetID]
		if !ok {
			return BeginPlanetClaimBoundaryResult{}, fmt.Errorf("planet %q: %w", input.PlanetID, ErrUnknownPlanet)
		}
		return BeginPlanetClaimBoundaryResult{
			Planet:    clonePlanet(planet),
			Boundary:  cloneClaimBoundaryRecord(existing),
			Duplicate: true,
		}, nil
	}

	planet, ok := store.planets[input.PlanetID]
	if !ok {
		return BeginPlanetClaimBoundaryResult{}, fmt.Errorf("planet %q: %w", input.PlanetID, ErrUnknownPlanet)
	}
	if planet.OwnerPlayerID == input.PlayerID {
		return BeginPlanetClaimBoundaryResult{}, fmt.Errorf("planet %q: %w", input.PlanetID, ErrPlanetAlreadyOwned)
	}
	if !planet.OwnerPlayerID.IsZero() {
		return BeginPlanetClaimBoundaryResult{}, fmt.Errorf("planet %q: %w", input.PlanetID, ErrPlanetAlreadyOwned)
	}

	claimedAt := input.ClaimedAt.UTC()
	planet.OwnerPlayerID = input.PlayerID
	planet.OwnerChangedAt = &claimedAt
	if err := planet.Validate(); err != nil {
		return BeginPlanetClaimBoundaryResult{}, err
	}
	store.planets[input.PlanetID] = clonePlanet(planet)
	staleIntel := store.markPlanetIntelStaleLocked(input.PlanetID, claimedAt, input.SourceReference)

	referenceKey, _ := input.ClaimReference.IdempotencyKey(input.PlayerID, input.PlanetID)
	record := ClaimBoundaryRecord{
		ClaimReference:  input.ClaimReference,
		ReferenceKey:    referenceKey,
		PlayerID:        input.PlayerID,
		PlanetID:        input.PlanetID,
		Status:          ClaimBoundaryStatusPendingSideEffects,
		EventID:         input.EventID,
		ClaimedAt:       claimedAt,
		RecordedAt:      claimedAt,
		StaleIntelCount: len(staleIntel),
	}
	store.claimBoundaries[input.ClaimReference] = cloneClaimBoundaryRecord(record)
	return BeginPlanetClaimBoundaryResult{
		Planet:     clonePlanet(planet),
		Boundary:   cloneClaimBoundaryRecord(record),
		StaleIntel: cloneClaimBoundaryStaleIntel(staleIntel),
	}, nil
}

func (store *InMemoryStore) CompletePlanetClaimBoundary(input CompletePlanetClaimBoundaryInput) (CompletePlanetClaimBoundaryResult, error) {
	if err := input.Validate(); err != nil {
		return CompletePlanetClaimBoundaryResult{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	record, ok := store.claimBoundaries[input.ClaimReference]
	if !ok {
		return CompletePlanetClaimBoundaryResult{}, ErrClaimBoundaryNotFound
	}
	if !claimBoundaryMatches(record, input.PlayerID, input.PlanetID) {
		return CompletePlanetClaimBoundaryResult{}, ErrPlanetClaimReferenceConflict
	}
	if record.Status == ClaimBoundaryStatusComplete {
		completed := cloneClaimBoundaryRecord(record)
		return CompletePlanetClaimBoundaryResult{Boundary: completed, Duplicate: true}, nil
	}
	record.Status = ClaimBoundaryStatusComplete
	record.CompletedAt = input.CompletedAt.UTC()
	record.StaleListingCount = input.StaleListingCount
	store.claimBoundaries[input.ClaimReference] = cloneClaimBoundaryRecord(record)
	return CompletePlanetClaimBoundaryResult{Boundary: cloneClaimBoundaryRecord(record)}, nil
}

func (store *InMemoryStore) ClaimBoundary(ref PlanetClaimReference) (ClaimBoundaryRecord, bool, error) {
	if err := ref.Validate(); err != nil {
		return ClaimBoundaryRecord{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	record, ok := store.claimBoundaries[ref]
	if !ok {
		return ClaimBoundaryRecord{}, false, nil
	}
	return cloneClaimBoundaryRecord(record), true, nil
}

func (store *InMemoryStore) ClaimBoundaries() []ClaimBoundaryRecord {
	store.mu.RLock()
	defer store.mu.RUnlock()

	if len(store.claimBoundaries) == 0 {
		return nil
	}
	refs := make([]PlanetClaimReference, 0, len(store.claimBoundaries))
	for ref := range store.claimBoundaries {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i] < refs[j]
	})
	records := make([]ClaimBoundaryRecord, 0, len(refs))
	for _, ref := range refs {
		records = append(records, cloneClaimBoundaryRecord(store.claimBoundaries[ref]))
	}
	return records
}

func (input BeginPlanetClaimBoundaryInput) Validate() error {
	if err := input.ClaimReference.Validate(); err != nil {
		return err
	}
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if input.ClaimedAt.IsZero() {
		return fmt.Errorf("claimed_at: %w", ErrInvalidPlanetClaim)
	}
	if err := input.EventID.Validate(); err != nil {
		return fmt.Errorf("event_id: %w", err)
	}
	if !validDiscoveryToken(input.SourceReference) {
		return fmt.Errorf("source_reference %q: %w", input.SourceReference, ErrInvalidPlanetClaim)
	}
	return nil
}

func (input CompletePlanetClaimBoundaryInput) Validate() error {
	if err := input.ClaimReference.Validate(); err != nil {
		return err
	}
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if input.CompletedAt.IsZero() {
		return fmt.Errorf("completed_at: %w", ErrInvalidPlanetClaim)
	}
	if input.StaleListingCount < 0 {
		return fmt.Errorf("stale_listing_count %d: %w", input.StaleListingCount, ErrInvalidPlanetClaim)
	}
	return nil
}

func claimBoundaryMatches(record ClaimBoundaryRecord, playerID foundation.PlayerID, planetID foundation.PlanetID) bool {
	return record.PlayerID == playerID && record.PlanetID == planetID
}

func cloneClaimBoundaryRecord(record ClaimBoundaryRecord) ClaimBoundaryRecord {
	record.ClaimedAt = record.ClaimedAt.UTC()
	record.RecordedAt = record.RecordedAt.UTC()
	record.CompletedAt = record.CompletedAt.UTC()
	return record
}

func cloneClaimBoundaryRecords(records []ClaimBoundaryRecord) []ClaimBoundaryRecord {
	if len(records) == 0 {
		return nil
	}
	cloned := make([]ClaimBoundaryRecord, len(records))
	for index, record := range records {
		cloned[index] = cloneClaimBoundaryRecord(record)
	}
	return cloned
}

func cloneClaimBoundaryStaleIntel(records []PlayerPlanetIntel) []PlayerPlanetIntel {
	if len(records) == 0 {
		return nil
	}
	cloned := make([]PlayerPlanetIntel, len(records))
	for index, record := range records {
		cloned[index] = clonePlayerPlanetIntel(record)
	}
	return cloned
}
