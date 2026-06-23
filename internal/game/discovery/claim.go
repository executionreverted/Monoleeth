package discovery

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

const (
	defaultClaimXCoreQuantity int64                = 1
	defaultClaimReason        economy.LedgerReason = "planet_claim"
)

var (
	ErrInvalidClaimConfig           = errors.New("invalid planet claim config")
	ErrInvalidPlanetClaim           = errors.New("invalid planet claim")
	ErrInvalidClaimProductionInit   = errors.New("invalid claim production initialization")
	ErrPlanetClaimReferenceConflict = errors.New("planet claim reference conflict")
	ErrPlanetClaimRequiresIntel     = errors.New("planet claim requires player intel")
	ErrPlanetClaimIntelInvalidated  = errors.New("planet claim intel invalidated")
	ErrPlanetAlreadyOwned           = errors.New("planet already owned")
	ErrPlanetClaimProximity         = errors.New("planet claim proximity not met")
	ErrPlanetClaimRankTooLow        = errors.New("planet claim rank too low")
	ErrInvalidPlanetClaimRank       = errors.New("invalid planet claim rank")
	ErrInvalidClaimXCoreConsume     = errors.New("invalid claim x core consume")
	ErrInvalidClaimXCoreSource      = errors.New("invalid claim x core source")
)

// PlanetClaimReference identifies one domain claim attempt. It is not a
// transport request id; it must remain stable across retries of the same claim.
type PlanetClaimReference string

// ClaimEventType names local discovery claim event records.
type ClaimEventType string

const (
	ClaimEventPlanetClaimed ClaimEventType = "planet.claimed"
)

// ClaimPlanetInput is the service command for claiming a materialized planet.
type ClaimPlanetInput struct {
	PlayerID       foundation.PlayerID  `json:"player_id"`
	PlanetID       foundation.PlanetID  `json:"planet_id"`
	ClaimReference PlanetClaimReference `json:"claim_reference"`
}

// ClaimPlanetResult reports authoritative owner state after a claim attempt.
type ClaimPlanetResult struct {
	Planet            Planet `json:"planet"`
	Claimed           bool   `json:"claimed"`
	AlreadyOwned      bool   `json:"already_owned,omitempty"`
	Duplicate         bool   `json:"duplicate,omitempty"`
	StaleIntelCount   int    `json:"stale_intel_count,omitempty"`
	StaleListingCount int    `json:"stale_listing_count,omitempty"`
}

// ClaimProductionInitializeInput is the narrow discovery-to-production handoff
// after a planet claim has become authoritative.
type ClaimProductionInitializeInput struct {
	PlayerID       foundation.PlayerID  `json:"player_id"`
	PlanetID       foundation.PlanetID  `json:"planet_id"`
	PlanetLevel    int                  `json:"planet_level"`
	ClaimedAt      time.Time            `json:"claimed_at"`
	ClaimReference PlanetClaimReference `json:"claim_reference"`
}

// ClaimProductionInitializeResult reports whether production rows were created
// or already existed for this claimed planet.
type ClaimProductionInitializeResult struct {
	Created            bool `json:"created"`
	AlreadyInitialized bool `json:"already_initialized,omitempty"`
}

// ClaimEventRecord is a local event/outbox-shaped record for planet claims.
type ClaimEventRecord struct {
	EventID        foundation.EventID   `json:"event_id"`
	Type           ClaimEventType       `json:"type"`
	PlayerID       foundation.PlayerID  `json:"player_id"`
	PlanetID       foundation.PlanetID  `json:"planet_id"`
	ClaimReference PlanetClaimReference `json:"claim_reference"`
	CreatedAt      time.Time            `json:"created_at"`
}

// ClaimListedIntelStaleInput is the discovery-to-market/intel invalidation hook
// fired after a planet owner change. It carries no hidden coordinates.
type ClaimListedIntelStaleInput struct {
	PlayerID        foundation.PlayerID  `json:"player_id"`
	PlanetID        foundation.PlanetID  `json:"planet_id"`
	ClaimReference  PlanetClaimReference `json:"claim_reference"`
	Reason          string               `json:"reason"`
	ClaimedAt       time.Time            `json:"claimed_at"`
	SourceReference string               `json:"source_reference"`
}

// ClaimListedIntelStaleResult reports stale listed intel markers.
type ClaimListedIntelStaleResult struct {
	MarkedCount int  `json:"marked_count"`
	Duplicate   bool `json:"duplicate,omitempty"`
}

// ClaimRankInput asks progression for the server-owned player rank.
type ClaimRankInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	PlanetID foundation.PlanetID `json:"planet_id"`
}

// ClaimRankResult reports the current player rank relevant to planet claims.
type ClaimRankResult struct {
	Rank int `json:"rank"`
}

// ClaimProximityInput asks the world/zone boundary whether the player is close
// enough to interact with the planet now.
type ClaimProximityInput struct {
	PlayerID          foundation.PlayerID `json:"player_id"`
	PlanetID          foundation.PlanetID `json:"planet_id"`
	WorldID           foundation.WorldID  `json:"world_id"`
	ZoneID            foundation.ZoneID   `json:"zone_id"`
	PlanetCoordinates world.Vec2          `json:"planet_coordinates"`
}

// ClaimProximityResult reports whether the authoritative range check passed.
type ClaimProximityResult struct {
	WithinRange bool `json:"within_range"`
}

// ClaimXCoreSourceInput asks a server-side boundary where claim X Core should
// be consumed from for this player and planet.
type ClaimXCoreSourceInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	PlanetID foundation.PlanetID `json:"planet_id"`
}

// ClaimXCoreConsumeInput is intentionally shaped like economy.RemoveItem input
// while keeping discovery decoupled from a concrete inventory service.
type ClaimXCoreConsumeInput struct {
	PlayerID       foundation.PlayerID   `json:"player_id"`
	PlanetID       foundation.PlanetID   `json:"planet_id"`
	ItemRef        economy.RemoveItemRef `json:"item_ref"`
	SourceLocation economy.ItemLocation  `json:"source_location"`
	Quantity       int64                 `json:"quantity"`
	Reason         economy.LedgerReason  `json:"reason"`
	Reference      PlanetClaimReference  `json:"reference"`
}

// ClaimXCoreConsumeResult reports duplicate-safe X Core consumption.
type ClaimXCoreConsumeResult struct {
	Duplicate bool `json:"duplicate"`
}

type ClaimRankProvider interface {
	PlayerClaimRank(input ClaimRankInput) (ClaimRankResult, error)
}

type ClaimProximityProvider interface {
	PlayerCanClaimPlanet(input ClaimProximityInput) (ClaimProximityResult, error)
}

type ClaimXCoreSourceProvider interface {
	ClaimXCoreSourceLocation(input ClaimXCoreSourceInput) (economy.ItemLocation, error)
}

type ClaimXCoreConsumer interface {
	ConsumeClaimXCore(input ClaimXCoreConsumeInput) (ClaimXCoreConsumeResult, error)
}

type ClaimProductionInitializer interface {
	InitializeClaimProduction(input ClaimProductionInitializeInput) (ClaimProductionInitializeResult, error)
}

type ClaimListedIntelStaleMarker interface {
	MarkClaimedPlanetListingsStale(input ClaimListedIntelStaleInput) (ClaimListedIntelStaleResult, error)
}

// ClaimServiceConfig wires claim service boundaries without depending on
// concrete progression, world, or inventory services.
type ClaimServiceConfig struct {
	Store                  *InMemoryStore
	ClaimBoundaries        ClaimBoundaryStore
	XCoreOwnerBoundary     ClaimXCoreOwnerBoundary
	Clock                  foundation.Clock
	Ranks                  ClaimRankProvider
	Proximity              ClaimProximityProvider
	XCoreSources           ClaimXCoreSourceProvider
	XCoreConsumer          ClaimXCoreConsumer
	ProductionInitializer  ClaimProductionInitializer
	ListedIntelStaleMarker ClaimListedIntelStaleMarker
	XCoreItemDefinition    economy.ItemDefinition
	ClaimReason            economy.LedgerReason
}

// ClaimService owns local MVP planet claim idempotency and event records.
type ClaimService struct {
	mu sync.Mutex

	store                  *InMemoryStore
	claimBoundaries        ClaimBoundaryStore
	xCoreOwnerBoundary     ClaimXCoreOwnerBoundary
	clock                  foundation.Clock
	ranks                  ClaimRankProvider
	proximity              ClaimProximityProvider
	xCoreSources           ClaimXCoreSourceProvider
	xCoreConsumer          ClaimXCoreConsumer
	productionInitializer  ClaimProductionInitializer
	listedIntelStaleMarker ClaimListedIntelStaleMarker
	xCoreItemDefinition    economy.ItemDefinition
	claimReason            economy.LedgerReason

	claims                    map[PlanetClaimReference]claimRecord
	events                    []ClaimEventRecord
	references                map[PlanetClaimReference]ClaimReferenceRecord
	recoveries                []ClaimRecoveryRecord
	outbox                    []ClaimOutboxRecord
	nextOutboxSequence        uint64
	xCoreConsumptions         map[PlanetClaimReference]ClaimXCoreConsumptionRecord
	productionInitializations map[PlanetClaimReference]ClaimProductionInitializationRecord
}

type claimRecord struct {
	input  ClaimPlanetInput
	result ClaimPlanetResult
}

type defaultClaimXCoreSourceProvider struct{}

// NewClaimService returns a planet claim service backed by InMemoryStore.
func NewClaimService(config ClaimServiceConfig) (*ClaimService, error) {
	normalized, err := normalizeClaimConfig(config)
	if err != nil {
		return nil, err
	}
	return &ClaimService{
		store:                     normalized.Store,
		claimBoundaries:           normalized.ClaimBoundaries,
		xCoreOwnerBoundary:        normalized.XCoreOwnerBoundary,
		clock:                     normalized.Clock,
		ranks:                     normalized.Ranks,
		proximity:                 normalized.Proximity,
		xCoreSources:              normalized.XCoreSources,
		xCoreConsumer:             normalized.XCoreConsumer,
		productionInitializer:     normalized.ProductionInitializer,
		listedIntelStaleMarker:    normalized.ListedIntelStaleMarker,
		xCoreItemDefinition:       normalized.XCoreItemDefinition,
		claimReason:               normalized.ClaimReason,
		claims:                    make(map[PlanetClaimReference]claimRecord),
		references:                make(map[PlanetClaimReference]ClaimReferenceRecord),
		xCoreConsumptions:         make(map[PlanetClaimReference]ClaimXCoreConsumptionRecord),
		productionInitializations: make(map[PlanetClaimReference]ClaimProductionInitializationRecord),
	}, nil
}

// ClaimPlanet validates personal intel, materialized planet state, proximity,
// rank, and X Core consumption before recording global ownership.
func (service *ClaimService) ClaimPlanet(input ClaimPlanetInput) (ClaimPlanetResult, error) {
	if err := input.Validate(); err != nil {
		return ClaimPlanetResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if record, ok := service.claims[input.ClaimReference]; ok {
		if !claimRecordMatchesInput(record, input) {
			return ClaimPlanetResult{}, ErrPlanetClaimReferenceConflict
		}
		duplicate := cloneClaimPlanetResult(record.result)
		duplicate.Duplicate = true
		return duplicate, nil
	}

	if err := service.validatePlayerIntel(input); err != nil {
		return ClaimPlanetResult{}, err
	}

	planet, ok, err := service.store.Planet(input.PlanetID)
	if err != nil {
		return ClaimPlanetResult{}, err
	}
	if !ok {
		return ClaimPlanetResult{}, fmt.Errorf("planet %q: %w", input.PlanetID, ErrUnknownPlanet)
	}

	if planet.OwnerPlayerID == input.PlayerID {
		claimedAt := service.clock.Now().UTC()
		if planet.OwnerChangedAt != nil {
			claimedAt = planet.OwnerChangedAt.UTC()
		}
		boundary, hasBoundary, err := service.claimBoundaryForInput(input)
		if err != nil {
			return ClaimPlanetResult{}, err
		}
		if hasBoundary && boundary.Status == ClaimBoundaryStatusComplete {
			if err := service.completeClaimBoundary(input, claimedAt, boundary.StaleListingCount); err != nil {
				return ClaimPlanetResult{}, err
			}
			result := ClaimPlanetResult{
				Planet:            clonePlanet(planet),
				Claimed:           true,
				StaleIntelCount:   boundary.StaleIntelCount,
				StaleListingCount: boundary.StaleListingCount,
			}
			service.claims[input.ClaimReference] = claimRecord{input: input, result: cloneClaimPlanetResult(result)}
			duplicate := cloneClaimPlanetResult(result)
			duplicate.Duplicate = true
			return duplicate, nil
		}
		if err := service.initializeProduction(input, planet, claimedAt); err != nil {
			return ClaimPlanetResult{}, err
		}
		staleListings, err := service.markListedIntelStale(input, claimedAt)
		if err != nil {
			return ClaimPlanetResult{}, err
		}
		if hasBoundary && boundary.Status == ClaimBoundaryStatusPendingSideEffects {
			if err := service.completeClaimBoundary(input, claimedAt, staleListings.MarkedCount); err != nil {
				return ClaimPlanetResult{}, err
			}
			result := ClaimPlanetResult{
				Planet:            clonePlanet(planet),
				Claimed:           true,
				StaleIntelCount:   boundary.StaleIntelCount,
				StaleListingCount: staleListings.MarkedCount,
			}
			service.claims[input.ClaimReference] = claimRecord{input: input, result: cloneClaimPlanetResult(result)}
			return result, nil
		}
		result := ClaimPlanetResult{
			Planet:            clonePlanet(planet),
			Claimed:           true,
			AlreadyOwned:      true,
			StaleListingCount: staleListings.MarkedCount,
		}
		service.claims[input.ClaimReference] = claimRecord{input: input, result: cloneClaimPlanetResult(result)}
		recordedAt := service.clock.Now().UTC()
		service.recordClaimReferenceLocked(input, claimedAt, recordedAt, true, "")
		service.appendClaimRecoveryLocked(input, claimedAt, recordedAt, ClaimRecoveryReasonAlreadyOwnedRepair)
		return result, nil
	}
	if !planet.OwnerPlayerID.IsZero() {
		return ClaimPlanetResult{}, fmt.Errorf("planet %q: %w", planet.ID, ErrPlanetAlreadyOwned)
	}
	if err := service.validateClaimBoundaryPreflight(input); err != nil {
		return ClaimPlanetResult{}, err
	}

	if err := service.validateProximity(input.PlayerID, planet); err != nil {
		return ClaimPlanetResult{}, err
	}
	if err := service.validateRank(input.PlayerID, planet); err != nil {
		return ClaimPlanetResult{}, err
	}

	now := service.clock.Now().UTC()
	event := newClaimEvent(ClaimEventPlanetClaimed, input, now)
	claimBoundary, err := service.beginClaimWithXCore(input, planet, now, event)
	if err != nil {
		return ClaimPlanetResult{}, err
	}
	if err := service.initializeProduction(input, claimBoundary.Planet, now); err != nil {
		return ClaimPlanetResult{}, err
	}
	staleListings, err := service.markListedIntelStale(input, now)
	if err != nil {
		return ClaimPlanetResult{}, err
	}
	if err := service.completeClaimBoundary(input, now, staleListings.MarkedCount); err != nil {
		return ClaimPlanetResult{}, err
	}

	result := ClaimPlanetResult{
		Planet:            claimBoundary.Planet,
		Claimed:           claimBoundary.Planet.OwnerPlayerID == input.PlayerID,
		StaleIntelCount:   len(claimBoundary.StaleIntel),
		StaleListingCount: staleListings.MarkedCount,
	}
	service.claims[input.ClaimReference] = claimRecord{input: input, result: cloneClaimPlanetResult(result)}
	return result, nil
}

// Events returns claim event records in append order.
func (service *ClaimService) Events() []ClaimEventRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	storeEvents := service.store.ClaimEvents()
	events := make([]ClaimEventRecord, 0, len(storeEvents)+len(service.events))
	events = append(events, storeEvents...)
	for _, event := range service.events {
		events = append(events, cloneClaimEventRecord(event))
	}
	return events
}

// Validate reports whether reference is a non-empty server-generated claim reference.
func (ref PlanetClaimReference) Validate() error {
	if !validDiscoveryToken(string(ref)) {
		return fmt.Errorf("claim_reference %q: %w", ref, ErrInvalidPlanetClaim)
	}
	return nil
}

// IdempotencyKey returns typed durable-boundary evidence for the canonical
// planet claim key matching this player and planet. Legacy local references
// remain valid but carry no typed evidence.
func (ref PlanetClaimReference) IdempotencyKey(playerID foundation.PlayerID, planetID foundation.PlanetID) (foundation.IdempotencyKey, bool) {
	key, err := foundation.PlanetClaimIdempotencyKey(playerID, planetID)
	if err != nil {
		return "", false
	}
	if ref != PlanetClaimReference(key.String()) {
		return "", false
	}
	return key, true
}

// Validate reports whether input has the required server-resolved identity.
func (input ClaimPlanetInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := input.ClaimReference.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether input can initialize production for a completed claim.
func (input ClaimProductionInitializeInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if input.PlanetLevel <= 0 {
		return fmt.Errorf("planet_level %d: %w", input.PlanetLevel, ErrInvalidClaimProductionInit)
	}
	if input.ClaimedAt.IsZero() {
		return fmt.Errorf("claimed_at: %w", ErrInvalidClaimProductionInit)
	}
	if err := input.ClaimReference.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether input can be handed to an intel listing stale marker.
func (input ClaimListedIntelStaleInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := input.ClaimReference.Validate(); err != nil {
		return err
	}
	if !validDiscoveryToken(input.Reason) {
		return fmt.Errorf("reason %q: %w", input.Reason, ErrInvalidPlanetClaim)
	}
	if input.ClaimedAt.IsZero() {
		return fmt.Errorf("claimed_at: %w", ErrInvalidPlanetClaim)
	}
	if input.SourceReference == "" {
		return fmt.Errorf("source_reference: %w", ErrInvalidPlanetClaim)
	}
	return nil
}

// Validate reports whether input identifies one rank lookup.
func (input ClaimRankInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	return nil
}

// Validate reports whether result contains a positive rank.
func (result ClaimRankResult) Validate() error {
	if result.Rank <= 0 {
		return fmt.Errorf("rank %d: %w", result.Rank, ErrInvalidPlanetClaimRank)
	}
	return nil
}

// Validate reports whether input identifies one proximity lookup.
func (input ClaimProximityInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := input.WorldID.Validate(); err != nil {
		return fmt.Errorf("world_id: %w", err)
	}
	if err := input.ZoneID.Validate(); err != nil {
		return fmt.Errorf("zone_id: %w", err)
	}
	if err := input.PlanetCoordinates.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether input identifies one X Core source lookup.
func (input ClaimXCoreSourceInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	return nil
}

// Validate reports whether input can be handed to an economy remove-item boundary.
func (input ClaimXCoreConsumeInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := input.ItemRef.Validate(); err != nil {
		return err
	}
	if err := input.SourceLocation.Validate(); err != nil {
		return err
	}
	if input.Quantity != defaultClaimXCoreQuantity {
		return fmt.Errorf("quantity %d: %w", input.Quantity, ErrInvalidClaimXCoreConsume)
	}
	if err := input.Reason.Validate(); err != nil {
		return err
	}
	if err := input.Reference.Validate(); err != nil {
		return err
	}
	return nil
}

func normalizeClaimConfig(config ClaimServiceConfig) (ClaimServiceConfig, error) {
	if config.Store == nil {
		config.Store = NewInMemoryStore()
	}
	if config.ClaimBoundaries == nil {
		config.ClaimBoundaries = config.Store
	}
	if config.Clock == nil {
		config.Clock = foundation.RealClock{}
	}
	if config.Ranks == nil {
		return ClaimServiceConfig{}, fmt.Errorf("ranks: %w", ErrInvalidClaimConfig)
	}
	if config.Proximity == nil {
		return ClaimServiceConfig{}, fmt.Errorf("proximity: %w", ErrInvalidClaimConfig)
	}
	if config.XCoreConsumer == nil {
		return ClaimServiceConfig{}, fmt.Errorf("x_core_consumer: %w", ErrInvalidClaimConfig)
	}
	if config.XCoreSources == nil {
		config.XCoreSources = defaultClaimXCoreSourceProvider{}
	}
	if config.XCoreOwnerBoundary == nil {
		config.XCoreOwnerBoundary = composedClaimXCoreOwnerBoundary{
			Consumer:   config.XCoreConsumer,
			Boundaries: config.ClaimBoundaries,
		}
	}
	if err := config.XCoreItemDefinition.Validate(); err != nil {
		return ClaimServiceConfig{}, fmt.Errorf("x_core_item_definition: %w", err)
	}
	if config.ClaimReason.IsZero() {
		config.ClaimReason = defaultClaimReason
	}
	if err := config.ClaimReason.Validate(); err != nil {
		return ClaimServiceConfig{}, fmt.Errorf("claim_reason: %w", err)
	}
	return config, nil
}

func (service *ClaimService) validatePlayerIntel(input ClaimPlanetInput) error {
	intel, ok, err := service.store.PlayerPlanetIntel(input.PlayerID, input.PlanetID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("player %q planet %q: %w", input.PlayerID, input.PlanetID, ErrPlanetClaimRequiresIntel)
	}
	if intel.State == IntelStateInvalidated {
		return fmt.Errorf("player %q planet %q: %w", input.PlayerID, input.PlanetID, ErrPlanetClaimIntelInvalidated)
	}
	return nil
}

func (service *ClaimService) validateProximity(playerID foundation.PlayerID, planet Planet) error {
	input := ClaimProximityInput{
		PlayerID:          playerID,
		PlanetID:          planet.ID,
		WorldID:           planet.WorldID,
		ZoneID:            planet.ZoneID,
		PlanetCoordinates: planet.Coordinates,
	}
	if err := input.Validate(); err != nil {
		return err
	}
	result, err := service.proximity.PlayerCanClaimPlanet(input)
	if err != nil {
		return err
	}
	if !result.WithinRange {
		return fmt.Errorf("planet %q: %w", planet.ID, ErrPlanetClaimProximity)
	}
	return nil
}

func (service *ClaimService) validateRank(playerID foundation.PlayerID, planet Planet) error {
	input := ClaimRankInput{
		PlayerID: playerID,
		PlanetID: planet.ID,
	}
	if err := input.Validate(); err != nil {
		return err
	}
	result, err := service.ranks.PlayerClaimRank(input)
	if err != nil {
		return err
	}
	if err := result.Validate(); err != nil {
		return err
	}
	if result.Rank < planet.Level {
		return fmt.Errorf("rank %d planet level %d: %w", result.Rank, planet.Level, ErrPlanetClaimRankTooLow)
	}
	return nil
}

func (service *ClaimService) validateClaimBoundaryPreflight(input ClaimPlanetInput) error {
	boundary, ok, err := service.claimBoundaries.ClaimBoundary(input.ClaimReference)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if boundary.PlayerID != input.PlayerID || boundary.PlanetID != input.PlanetID {
		return ErrPlanetClaimReferenceConflict
	}
	return nil
}

func (service *ClaimService) claimBoundaryForInput(input ClaimPlanetInput) (ClaimBoundaryRecord, bool, error) {
	boundary, ok, err := service.claimBoundaries.ClaimBoundary(input.ClaimReference)
	if err != nil || !ok {
		return ClaimBoundaryRecord{}, ok, err
	}
	if boundary.PlayerID != input.PlayerID || boundary.PlanetID != input.PlanetID {
		return ClaimBoundaryRecord{}, false, ErrPlanetClaimReferenceConflict
	}
	return boundary, true, nil
}

func (service *ClaimService) beginClaimWithXCore(
	input ClaimPlanetInput,
	planet Planet,
	claimedAt time.Time,
	event ClaimEventRecord,
) (BeginPlanetClaimBoundaryResult, error) {
	beginInput := BeginPlanetClaimBoundaryInput{
		ClaimReference:  input.ClaimReference,
		PlayerID:        input.PlayerID,
		PlanetID:        planet.ID,
		ClaimedAt:       claimedAt,
		EventID:         event.EventID,
		SourceReference: claimOwnerChangeSourceReference(event),
	}
	if consumed, err := service.claimXCoreAlreadyConsumedLocked(input); err != nil {
		return BeginPlanetClaimBoundaryResult{}, err
	} else if consumed {
		return service.claimBoundaries.BeginPlanetClaimBoundary(beginInput)
	}
	consumeInput, err := service.claimXCoreConsumeInput(input, planet)
	if err != nil {
		return BeginPlanetClaimBoundaryResult{}, err
	}
	result, err := service.xCoreOwnerBoundary.BeginPlanetClaimWithXCore(BeginPlanetClaimWithXCoreInput{
		XCore:      consumeInput,
		Boundary:   beginInput,
		ConsumedAt: service.clock.Now().UTC(),
	})
	if result.XCoreConsumption.ClaimReference != "" {
		if result.XCoreConsumption.ClaimReference != input.ClaimReference ||
			result.XCoreConsumption.PlayerID != input.PlayerID ||
			result.XCoreConsumption.PlanetID != input.PlanetID {
			return BeginPlanetClaimBoundaryResult{}, ErrPlanetClaimReferenceConflict
		}
		service.recordClaimXCoreConsumptionRecordLocked(result.XCoreConsumption)
	}
	if err != nil {
		return BeginPlanetClaimBoundaryResult{}, err
	}
	return result.Boundary, nil
}

func (service *ClaimService) claimXCoreConsumeInput(input ClaimPlanetInput, planet Planet) (ClaimXCoreConsumeInput, error) {
	sourceInput := ClaimXCoreSourceInput{
		PlayerID: input.PlayerID,
		PlanetID: planet.ID,
	}
	if err := sourceInput.Validate(); err != nil {
		return ClaimXCoreConsumeInput{}, err
	}
	sourceLocation, err := service.xCoreSources.ClaimXCoreSourceLocation(sourceInput)
	if err != nil {
		return ClaimXCoreConsumeInput{}, err
	}
	if err := sourceLocation.Validate(); err != nil {
		return ClaimXCoreConsumeInput{}, fmt.Errorf("x core source: %w", err)
	}

	consumeInput := ClaimXCoreConsumeInput{
		PlayerID: input.PlayerID,
		PlanetID: planet.ID,
		ItemRef: economy.RemoveItemRef{
			Definition: service.xCoreItemDefinition,
		},
		SourceLocation: sourceLocation,
		Quantity:       defaultClaimXCoreQuantity,
		Reason:         service.claimReason,
		Reference:      input.ClaimReference,
	}
	if err := consumeInput.Validate(); err != nil {
		return ClaimXCoreConsumeInput{}, err
	}
	return consumeInput, nil
}

func (service *ClaimService) completeClaimBoundary(input ClaimPlanetInput, completedAt time.Time, staleListingCount int) error {
	_, err := service.claimBoundaries.CompletePlanetClaimBoundary(CompletePlanetClaimBoundaryInput{
		ClaimReference:    input.ClaimReference,
		PlayerID:          input.PlayerID,
		PlanetID:          input.PlanetID,
		CompletedAt:       completedAt,
		StaleListingCount: staleListingCount,
	})
	return err
}

func (service *ClaimService) initializeProduction(input ClaimPlanetInput, planet Planet, claimedAt time.Time) error {
	if service.productionInitializer == nil {
		return nil
	}
	if initialized, err := service.claimProductionAlreadyInitializedLocked(input); err != nil || initialized {
		return err
	}
	initInput := ClaimProductionInitializeInput{
		PlayerID:       input.PlayerID,
		PlanetID:       planet.ID,
		PlanetLevel:    planet.Level,
		ClaimedAt:      claimedAt,
		ClaimReference: input.ClaimReference,
	}
	if err := initInput.Validate(); err != nil {
		return err
	}
	result, err := service.productionInitializer.InitializeClaimProduction(initInput)
	if err != nil {
		return err
	}
	service.recordClaimProductionInitializationLocked(initInput, result, service.clock.Now().UTC())
	return nil
}

func (service *ClaimService) markListedIntelStale(input ClaimPlanetInput, claimedAt time.Time) (ClaimListedIntelStaleResult, error) {
	if service.listedIntelStaleMarker == nil {
		return ClaimListedIntelStaleResult{}, nil
	}
	markerInput := ClaimListedIntelStaleInput{
		PlayerID:        input.PlayerID,
		PlanetID:        input.PlanetID,
		ClaimReference:  input.ClaimReference,
		Reason:          "planet_claimed",
		ClaimedAt:       claimedAt,
		SourceReference: fmt.Sprintf("%s:%s", ClaimEventPlanetClaimed, input.ClaimReference),
	}
	if err := markerInput.Validate(); err != nil {
		return ClaimListedIntelStaleResult{}, err
	}
	return service.listedIntelStaleMarker.MarkClaimedPlanetListingsStale(markerInput)
}

func (defaultClaimXCoreSourceProvider) ClaimXCoreSourceLocation(input ClaimXCoreSourceInput) (economy.ItemLocation, error) {
	if err := input.Validate(); err != nil {
		return economy.ItemLocation{}, err
	}
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, input.PlayerID.String())
	if err != nil {
		return economy.ItemLocation{}, fmt.Errorf("%w: %v", ErrInvalidClaimXCoreSource, err)
	}
	return location, nil
}

func claimRecordMatchesInput(record claimRecord, input ClaimPlanetInput) bool {
	return record.input.PlayerID == input.PlayerID &&
		record.input.PlanetID == input.PlanetID &&
		record.input.ClaimReference == input.ClaimReference
}

func newClaimEvent(eventType ClaimEventType, input ClaimPlanetInput, createdAt time.Time) ClaimEventRecord {
	digest := scannerDigest("claim_event", string(eventType), input.PlayerID.String(), input.PlanetID.String(), string(input.ClaimReference))
	return ClaimEventRecord{
		EventID:        foundation.EventID("event_" + fmt.Sprintf("%x", digest[:10])),
		Type:           eventType,
		PlayerID:       input.PlayerID,
		PlanetID:       input.PlanetID,
		ClaimReference: input.ClaimReference,
		CreatedAt:      createdAt.UTC(),
	}
}

func claimOwnerChangeSourceReference(event ClaimEventRecord) string {
	return string(event.Type) + ":" + event.EventID.String()
}

func cloneClaimPlanetResult(result ClaimPlanetResult) ClaimPlanetResult {
	result.Planet = clonePlanet(result.Planet)
	return result
}
