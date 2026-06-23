package intel

import (
	"fmt"
	"sync"

	"gameproject/internal/game/foundation"
)

// Service owns server-authoritative in-memory planet intel and coordinate item state.
type Service struct {
	mu    sync.Mutex
	clock foundation.Clock

	intel map[playerPlanetKey]PlayerPlanetIntel
	items map[foundation.ItemID]CoordinateItem

	shareReferences  map[foundation.IdempotencyKey]shareRecord
	createReferences map[foundation.IdempotencyKey]createRecord
	useReferences    map[foundation.IdempotencyKey]useRecord
}

type playerPlanetKey struct {
	playerID foundation.PlayerID
	planetID foundation.PlanetID
}

type shareRecord struct {
	input  SharePlanetIntelInput
	result SharePlanetIntelResult
}

type createRecord struct {
	input  CreateCoordinateItemInput
	result CreateCoordinateItemResult
}

type useRecord struct {
	input  UseCoordinateItemInput
	result UseCoordinateItemResult
}

// NewService returns an empty in-memory intel service.
func NewService(clock foundation.Clock) *Service {
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &Service{
		clock:            clock,
		intel:            make(map[playerPlanetKey]PlayerPlanetIntel),
		items:            make(map[foundation.ItemID]CoordinateItem),
		shareReferences:  make(map[foundation.IdempotencyKey]shareRecord),
		createReferences: make(map[foundation.IdempotencyKey]createRecord),
		useReferences:    make(map[foundation.IdempotencyKey]useRecord),
	}
}

// UpsertPlayerPlanetIntel records server-authored planet memory.
func (service *Service) UpsertPlayerPlanetIntel(incoming PlayerPlanetIntel) (PlayerPlanetIntel, bool, error) {
	if err := incoming.Validate(); err != nil {
		return PlayerPlanetIntel{}, false, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	stored, updated := service.upsertPlayerPlanetIntelLocked(incoming)
	return clonePlayerPlanetIntel(stored), updated, nil
}

// PlayerPlanetIntel returns one player's server-owned intel record.
func (service *Service) PlayerPlanetIntel(playerID foundation.PlayerID, planetID foundation.PlanetID) (PlayerPlanetIntel, bool, error) {
	if err := playerID.Validate(); err != nil {
		return PlayerPlanetIntel{}, false, err
	}
	if err := planetID.Validate(); err != nil {
		return PlayerPlanetIntel{}, false, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	intel, ok := service.intel[newPlayerPlanetKey(playerID, planetID)]
	if !ok {
		return PlayerPlanetIntel{}, false, nil
	}
	return clonePlayerPlanetIntel(intel), true, nil
}

// CoordinateItem returns one server-owned coordinate item payload.
func (service *Service) CoordinateItem(itemInstanceID foundation.ItemID) (CoordinateItem, bool, error) {
	if err := itemInstanceID.Validate(); err != nil {
		return CoordinateItem{}, false, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	item, ok := service.items[itemInstanceID]
	if !ok {
		return CoordinateItem{}, false, nil
	}
	return cloneCoordinateItem(item), true, nil
}

// SharePlanetIntel copies one known, non-invalidated planet intel row to another player.
func (service *Service) SharePlanetIntel(input SharePlanetIntelInput) (SharePlanetIntelResult, error) {
	if err := input.Validate(); err != nil {
		return SharePlanetIntelResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if record, ok := service.shareReferences[input.Reference]; ok {
		if !shareInputMatches(record.input, input) {
			return SharePlanetIntelResult{}, ErrReferenceConflict
		}
		result := cloneSharePlanetIntelResult(record.result)
		result.Duplicate = true
		return result, nil
	}

	source, err := service.knownSourceIntelLocked(input.FromPlayerID, input.PlanetID)
	if err != nil {
		return SharePlanetIntelResult{}, err
	}
	receiver := clonePlayerPlanetIntel(source)
	receiver.PlayerID = input.ToPlayerID
	receiver.SourceType = IntelSourceShareReceived
	receiver.SourceReference = input.Reference.String()

	storedReceiver, updated := service.upsertPlayerPlanetIntelLocked(receiver)
	result := SharePlanetIntelResult{
		SourceIntel:     source,
		ReceiverIntel:   storedReceiver,
		Shared:          true,
		ReceiverUpdated: updated,
	}
	service.shareReferences[input.Reference] = shareRecord{input: input, result: cloneSharePlanetIntelResult(result)}
	return cloneSharePlanetIntelResult(result), nil
}

// CreateCoordinateItem creates a coordinate item payload from stored player intel.
func (service *Service) CreateCoordinateItem(input CreateCoordinateItemInput) (CreateCoordinateItemResult, error) {
	if err := input.Validate(); err != nil {
		return CreateCoordinateItemResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if record, ok := service.createReferences[input.Reference]; ok {
		if !createInputMatches(record.input, input) {
			return CreateCoordinateItemResult{}, ErrReferenceConflict
		}
		result := cloneCreateCoordinateItemResult(record.result)
		result.Duplicate = true
		return result, nil
	}
	if _, exists := service.items[input.ItemInstanceID]; exists {
		return CreateCoordinateItemResult{}, fmt.Errorf("item %q: %w", input.ItemInstanceID, ErrCoordinateItemAlreadyExists)
	}

	source, err := service.knownSourceIntelLocked(input.PlayerID, input.PlanetID)
	if err != nil {
		return CreateCoordinateItemResult{}, err
	}
	now := service.clock.Now().UTC()
	item := CoordinateItem{
		ItemInstanceID:       input.ItemInstanceID,
		OwnerPlayerID:        input.PlayerID,
		PlanetID:             source.PlanetID,
		WorldID:              source.WorldID,
		ZoneID:               source.ZoneID,
		Coordinates:          source.Coordinates,
		State:                source.State,
		Confidence:           source.Confidence,
		LastVerifiedAt:       source.LastSeenAt,
		CreatedAt:            now,
		CreatedBy:            input.PlayerID,
		CreateReference:      input.Reference,
		SourceIntelReference: source.SourceReference,
	}
	if err := item.Validate(); err != nil {
		return CreateCoordinateItemResult{}, err
	}
	service.items[input.ItemInstanceID] = cloneCoordinateItem(item)

	result := CreateCoordinateItemResult{
		Item:    item,
		Created: true,
	}
	service.createReferences[input.Reference] = createRecord{input: input, result: cloneCreateCoordinateItemResult(result)}
	return cloneCreateCoordinateItemResult(result), nil
}

// UseCoordinateItem marks one owned coordinate item used and writes player intel.
func (service *Service) UseCoordinateItem(input UseCoordinateItemInput) (UseCoordinateItemResult, error) {
	if err := input.Validate(); err != nil {
		return UseCoordinateItemResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if record, ok := service.useReferences[input.Reference]; ok {
		if !useInputMatches(record.input, input) {
			return UseCoordinateItemResult{}, ErrReferenceConflict
		}
		result := cloneUseCoordinateItemResult(record.result)
		result.Duplicate = true
		return result, nil
	}

	item, ok := service.items[input.ItemInstanceID]
	if !ok {
		return UseCoordinateItemResult{}, fmt.Errorf("item %q: %w", input.ItemInstanceID, ErrCoordinateItemNotFound)
	}
	if item.OwnerPlayerID != input.PlayerID {
		return UseCoordinateItemResult{}, fmt.Errorf("item %q owner %q player %q: %w", input.ItemInstanceID, item.OwnerPlayerID, input.PlayerID, ErrCoordinateItemNotOwned)
	}
	if item.UsedAt != nil {
		return UseCoordinateItemResult{}, fmt.Errorf("item %q: %w", input.ItemInstanceID, ErrCoordinateItemAlreadyUsed)
	}

	incoming := PlayerPlanetIntel{
		PlayerID:        input.PlayerID,
		PlanetID:        item.PlanetID,
		WorldID:         item.WorldID,
		ZoneID:          item.ZoneID,
		Coordinates:     item.Coordinates,
		State:           item.State,
		Confidence:      item.Confidence,
		LastSeenAt:      item.LastVerifiedAt,
		SourceType:      IntelSourceCoordinateItemUsed,
		SourceReference: input.Reference.String(),
	}
	if err := incoming.Validate(); err != nil {
		return UseCoordinateItemResult{}, err
	}
	storedIntel, updated := service.upsertPlayerPlanetIntelLocked(incoming)

	usedAt := service.clock.Now().UTC()
	item.UsedAt = &usedAt
	item.UsedBy = input.PlayerID
	item.UseReference = input.Reference
	if err := item.Validate(); err != nil {
		return UseCoordinateItemResult{}, err
	}
	service.items[input.ItemInstanceID] = cloneCoordinateItem(item)

	result := UseCoordinateItemResult{
		Item:         item,
		Intel:        storedIntel,
		Used:         true,
		IntelUpdated: updated,
	}
	service.useReferences[input.Reference] = useRecord{input: input, result: cloneUseCoordinateItemResult(result)}
	return cloneUseCoordinateItemResult(result), nil
}

func (service *Service) knownSourceIntelLocked(playerID foundation.PlayerID, planetID foundation.PlanetID) (PlayerPlanetIntel, error) {
	source, ok := service.intel[newPlayerPlanetKey(playerID, planetID)]
	if !ok {
		return PlayerPlanetIntel{}, fmt.Errorf("player %q planet %q: %w", playerID, planetID, ErrPlanetIntelNotKnown)
	}
	if source.State == IntelStateInvalidated {
		return PlayerPlanetIntel{}, fmt.Errorf("player %q planet %q: %w", playerID, planetID, ErrPlanetIntelInvalidated)
	}
	if !source.State.Known() {
		return PlayerPlanetIntel{}, fmt.Errorf("player %q planet %q: %w", playerID, planetID, ErrPlanetIntelNotKnown)
	}
	return clonePlayerPlanetIntel(source), nil
}

func (service *Service) upsertPlayerPlanetIntelLocked(incoming PlayerPlanetIntel) (PlayerPlanetIntel, bool) {
	key := newPlayerPlanetKey(incoming.PlayerID, incoming.PlanetID)
	if existing, ok := service.intel[key]; ok && !shouldReplaceIntel(existing, incoming) {
		return clonePlayerPlanetIntel(existing), false
	}
	next := clonePlayerPlanetIntel(incoming)
	next.LastSeenAt = next.LastSeenAt.UTC()
	service.intel[key] = clonePlayerPlanetIntel(next)
	return clonePlayerPlanetIntel(next), true
}

func shouldReplaceIntel(existing PlayerPlanetIntel, incoming PlayerPlanetIntel) bool {
	if incoming.LastSeenAt.After(existing.LastSeenAt) {
		return true
	}
	if incoming.LastSeenAt.Before(existing.LastSeenAt) {
		return false
	}
	if incoming.Confidence > existing.Confidence {
		return true
	}
	if incoming.Confidence < existing.Confidence {
		return false
	}
	return intelStateRank(incoming.State) > intelStateRank(existing.State)
}

func intelStateRank(state IntelState) int {
	switch state {
	case IntelStateInvalidated:
		return 0
	case IntelStateStale:
		return 1
	case IntelStateColonizedByOther:
		return 2
	case IntelStateFresh:
		return 3
	case IntelStateVerified:
		return 4
	default:
		return -1
	}
}

func newPlayerPlanetKey(playerID foundation.PlayerID, planetID foundation.PlanetID) playerPlanetKey {
	return playerPlanetKey{playerID: playerID, planetID: planetID}
}

func shareInputMatches(a SharePlanetIntelInput, b SharePlanetIntelInput) bool {
	return a.FromPlayerID == b.FromPlayerID &&
		a.ToPlayerID == b.ToPlayerID &&
		a.PlanetID == b.PlanetID &&
		a.Reference == b.Reference
}

func createInputMatches(a CreateCoordinateItemInput, b CreateCoordinateItemInput) bool {
	return a.PlayerID == b.PlayerID &&
		a.PlanetID == b.PlanetID &&
		a.ItemInstanceID == b.ItemInstanceID &&
		a.Reference == b.Reference
}

func useInputMatches(a UseCoordinateItemInput, b UseCoordinateItemInput) bool {
	return a.PlayerID == b.PlayerID &&
		a.ItemInstanceID == b.ItemInstanceID &&
		a.Reference == b.Reference
}

func clonePlayerPlanetIntel(intel PlayerPlanetIntel) PlayerPlanetIntel {
	return intel
}

func cloneCoordinateItem(item CoordinateItem) CoordinateItem {
	clone := item
	if item.UsedAt != nil {
		usedAt := item.UsedAt.UTC()
		clone.UsedAt = &usedAt
	}
	return clone
}

func cloneSharePlanetIntelResult(result SharePlanetIntelResult) SharePlanetIntelResult {
	result.SourceIntel = clonePlayerPlanetIntel(result.SourceIntel)
	result.ReceiverIntel = clonePlayerPlanetIntel(result.ReceiverIntel)
	return result
}

func cloneCreateCoordinateItemResult(result CreateCoordinateItemResult) CreateCoordinateItemResult {
	result.Item = cloneCoordinateItem(result.Item)
	return result
}

func cloneUseCoordinateItemResult(result UseCoordinateItemResult) UseCoordinateItemResult {
	result.Item = cloneCoordinateItem(result.Item)
	result.Intel = clonePlayerPlanetIntel(result.Intel)
	return result
}
