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
	defaultCoordinateScrollQuantity     int64                = 1
	defaultCoordinateScrollCreateReason economy.LedgerReason = "coordinate_scroll_create"
	defaultCoordinateScrollUseReason    economy.LedgerReason = "coordinate_scroll_use"
)

var (
	ErrInvalidCoordinateScrollConfig     = errors.New("invalid coordinate scroll config")
	ErrInvalidCoordinateScroll           = errors.New("invalid coordinate scroll")
	ErrCoordinateScrollReferenceConflict = errors.New("coordinate scroll reference conflict")
	ErrCoordinateScrollRequiresIntel     = errors.New("coordinate scroll requires player intel")
	ErrCoordinateScrollIntelInvalidated  = errors.New("coordinate scroll intel invalidated")
	ErrUnknownCoordinateScroll           = errors.New("unknown coordinate scroll")
	ErrCoordinateScrollAlreadyUsed       = errors.New("coordinate scroll already used")
	ErrInvalidCoordinateScrollItem       = errors.New("invalid coordinate scroll item")
	ErrInvalidCoordinateScrollSource     = errors.New("invalid coordinate scroll source")
)

// CoordinateScrollReference identifies one domain create or use transition.
// It is separate from transport request ids and must stay stable across retries.
type CoordinateScrollReference string

// CreateCoordinateScrollInput creates one itemized planet-coordinate record
// from existing server-side player intel. It intentionally contains no client
// coordinate or intel payload fields.
type CreateCoordinateScrollInput struct {
	PlayerID        foundation.PlayerID       `json:"player_id"`
	PlanetID        foundation.PlanetID       `json:"planet_id"`
	CreateReference CoordinateScrollReference `json:"create_reference"`
}

// CreateCoordinateScrollResult reports the item instance and metadata stored
// by the server for a coordinate scroll.
type CreateCoordinateScrollResult struct {
	ScrollItemInstanceID foundation.ItemID        `json:"scroll_item_instance_id"`
	Metadata             CoordinateScrollMetadata `json:"metadata"`
	Created              bool                     `json:"created"`
	Duplicate            bool                     `json:"duplicate,omitempty"`
}

// UseCoordinateScrollInput consumes one coordinate scroll and writes personal
// intel for the receiver. It intentionally contains no client coordinate or
// intel payload fields.
type UseCoordinateScrollInput struct {
	PlayerID             foundation.PlayerID       `json:"player_id"`
	ScrollItemInstanceID foundation.ItemID         `json:"scroll_item_instance_id"`
	UseReference         CoordinateScrollReference `json:"use_reference"`
}

// UseCoordinateScrollResult reports the authoritative intel outcome from a
// coordinate scroll use.
type UseCoordinateScrollResult struct {
	Metadata     CoordinateScrollMetadata `json:"metadata"`
	Intel        PlayerPlanetIntel        `json:"intel"`
	Used         bool                     `json:"used"`
	IntelUpdated bool                     `json:"intel_updated"`
	Duplicate    bool                     `json:"duplicate,omitempty"`
}

// CoordinateScrollMetadata is the server-side payload for one scroll item
// instance. Clients may refer to the item instance id, but do not author this.
type CoordinateScrollMetadata struct {
	ScrollItemInstanceID foundation.ItemID         `json:"scroll_item_instance_id"`
	PlanetID             foundation.PlanetID       `json:"planet_id"`
	Coordinates          world.Vec2                `json:"coordinates"`
	PlanetLevelKnown     int                       `json:"planet_level_known"`
	PlanetTypeKnown      PlanetType                `json:"planet_type_known"`
	OwnerPlayerID        foundation.PlayerID       `json:"owner_player_id,omitempty"`
	State                IntelState                `json:"stale_state"`
	Confidence           int                       `json:"confidence"`
	LastVerifiedAt       time.Time                 `json:"last_verified_at"`
	CreatedAt            time.Time                 `json:"created_at"`
	CreatedBy            foundation.PlayerID       `json:"created_by"`
	SourceReference      string                    `json:"source_reference"`
	UsedAt               *time.Time                `json:"used_at,omitempty"`
	UsedBy               foundation.PlayerID       `json:"used_by,omitempty"`
	UseReference         CoordinateScrollReference `json:"use_reference,omitempty"`
}

// CoordinateScrollItemCreateInput is shaped for an economy item grant boundary
// without depending on a concrete inventory service.
type CoordinateScrollItemCreateInput struct {
	PlayerID       foundation.PlayerID       `json:"player_id"`
	ItemDefinition economy.ItemDefinition    `json:"item_definition"`
	TargetLocation economy.ItemLocation      `json:"target_location"`
	Quantity       int64                     `json:"quantity"`
	Reason         economy.LedgerReason      `json:"reason"`
	Reference      CoordinateScrollReference `json:"reference"`
}

// CoordinateScrollItemCreateResult reports the created item instance id.
type CoordinateScrollItemCreateResult struct {
	ItemInstanceID foundation.ItemID `json:"item_instance_id"`
	Duplicate      bool              `json:"duplicate,omitempty"`
}

// CoordinateScrollConsumeInput is intentionally shaped like economy.RemoveItem
// input while keeping discovery decoupled from the concrete inventory service.
type CoordinateScrollConsumeInput struct {
	PlayerID             foundation.PlayerID       `json:"player_id"`
	ScrollItemInstanceID foundation.ItemID         `json:"scroll_item_instance_id"`
	ItemRef              economy.RemoveItemRef     `json:"item_ref"`
	SourceLocation       economy.ItemLocation      `json:"source_location"`
	Quantity             int64                     `json:"quantity"`
	Reason               economy.LedgerReason      `json:"reason"`
	Reference            CoordinateScrollReference `json:"reference"`
}

// CoordinateScrollConsumeResult reports duplicate-safe scroll consumption.
type CoordinateScrollConsumeResult struct {
	Duplicate bool `json:"duplicate,omitempty"`
}

// CoordinateScrollSourceInput asks a server-side boundary where a scroll should
// be consumed from for this player and item instance.
type CoordinateScrollSourceInput struct {
	PlayerID             foundation.PlayerID `json:"player_id"`
	ScrollItemInstanceID foundation.ItemID   `json:"scroll_item_instance_id"`
}

type CoordinateScrollItemCreator interface {
	CreateCoordinateScrollItem(input CoordinateScrollItemCreateInput) (CoordinateScrollItemCreateResult, error)
}

type CoordinateScrollConsumer interface {
	ConsumeCoordinateScroll(input CoordinateScrollConsumeInput) (CoordinateScrollConsumeResult, error)
}

type CoordinateScrollSourceProvider interface {
	CoordinateScrollSourceLocation(input CoordinateScrollSourceInput) (economy.ItemLocation, error)
}

// CoordinateScrollServiceConfig wires coordinate scroll boundaries.
type CoordinateScrollServiceConfig struct {
	Store                          *InMemoryStore
	Clock                          foundation.Clock
	ItemCreator                    CoordinateScrollItemCreator
	ItemConsumer                   CoordinateScrollConsumer
	ScrollSources                  CoordinateScrollSourceProvider
	CoordinateScrollItemDefinition economy.ItemDefinition
	CreateReason                   economy.LedgerReason
	UseReason                      economy.LedgerReason
}

// CoordinateScrollService owns local MVP coordinate scroll idempotency and
// server-side metadata storage.
type CoordinateScrollService struct {
	mu sync.Mutex

	store          *InMemoryStore
	clock          foundation.Clock
	itemCreator    CoordinateScrollItemCreator
	itemConsumer   CoordinateScrollConsumer
	scrollSources  CoordinateScrollSourceProvider
	itemDefinition economy.ItemDefinition
	createReason   economy.LedgerReason
	useReason      economy.LedgerReason

	scrolls          map[foundation.ItemID]CoordinateScrollMetadata
	createReferences map[CoordinateScrollReference]createCoordinateScrollRecord
	useReferences    map[CoordinateScrollReference]useCoordinateScrollRecord
}

type createCoordinateScrollRecord struct {
	input  CreateCoordinateScrollInput
	result CreateCoordinateScrollResult
}

type useCoordinateScrollRecord struct {
	input  UseCoordinateScrollInput
	result UseCoordinateScrollResult
}

type defaultCoordinateScrollSourceProvider struct{}

// NewCoordinateScrollService returns a coordinate scroll service backed by
// server-side discovery state.
func NewCoordinateScrollService(config CoordinateScrollServiceConfig) (*CoordinateScrollService, error) {
	normalized, err := normalizeCoordinateScrollConfig(config)
	if err != nil {
		return nil, err
	}
	return &CoordinateScrollService{
		store:            normalized.Store,
		clock:            normalized.Clock,
		itemCreator:      normalized.ItemCreator,
		itemConsumer:     normalized.ItemConsumer,
		scrollSources:    normalized.ScrollSources,
		itemDefinition:   normalized.CoordinateScrollItemDefinition,
		createReason:     normalized.CreateReason,
		useReason:        normalized.UseReason,
		scrolls:          make(map[foundation.ItemID]CoordinateScrollMetadata),
		createReferences: make(map[CoordinateScrollReference]createCoordinateScrollRecord),
		useReferences:    make(map[CoordinateScrollReference]useCoordinateScrollRecord),
	}, nil
}

// CreateCoordinateScroll creates one coordinate scroll from server-side player
// intel and stores its server-authored metadata by item instance id.
func (service *CoordinateScrollService) CreateCoordinateScroll(input CreateCoordinateScrollInput) (CreateCoordinateScrollResult, error) {
	if err := input.Validate(); err != nil {
		return CreateCoordinateScrollResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if record, ok := service.createReferences[input.CreateReference]; ok {
		if !createCoordinateScrollRecordMatchesInput(record, input) {
			return CreateCoordinateScrollResult{}, ErrCoordinateScrollReferenceConflict
		}
		duplicate := cloneCreateCoordinateScrollResult(record.result)
		duplicate.Duplicate = true
		return duplicate, nil
	}

	sourceIntel, err := service.sourceIntelForCreate(input)
	if err != nil {
		return CreateCoordinateScrollResult{}, err
	}
	planet, ok, err := service.store.Planet(input.PlanetID)
	if err != nil {
		return CreateCoordinateScrollResult{}, err
	}
	if !ok {
		return CreateCoordinateScrollResult{}, fmt.Errorf("planet %q: %w", input.PlanetID, ErrUnknownPlanet)
	}

	targetLocation, err := coordinateScrollAccountInventoryLocation(input.PlayerID)
	if err != nil {
		return CreateCoordinateScrollResult{}, err
	}
	createItemInput := CoordinateScrollItemCreateInput{
		PlayerID:       input.PlayerID,
		ItemDefinition: service.itemDefinition,
		TargetLocation: targetLocation,
		Quantity:       defaultCoordinateScrollQuantity,
		Reason:         service.createReason,
		Reference:      input.CreateReference,
	}
	if err := createItemInput.Validate(); err != nil {
		return CreateCoordinateScrollResult{}, err
	}
	itemResult, err := service.itemCreator.CreateCoordinateScrollItem(createItemInput)
	if err != nil {
		return CreateCoordinateScrollResult{}, err
	}
	if err := itemResult.Validate(); err != nil {
		return CreateCoordinateScrollResult{}, err
	}

	metadata := newCoordinateScrollMetadata(itemResult.ItemInstanceID, input.PlayerID, sourceIntel, planet, service.clock.Now())
	if err := metadata.Validate(); err != nil {
		return CreateCoordinateScrollResult{}, err
	}
	if existing, ok := service.scrolls[itemResult.ItemInstanceID]; ok && !coordinateScrollMetadataMatchesIdentity(existing, metadata) {
		return CreateCoordinateScrollResult{}, ErrInvalidCoordinateScrollItem
	}
	service.scrolls[itemResult.ItemInstanceID] = cloneCoordinateScrollMetadata(metadata)

	result := CreateCoordinateScrollResult{
		ScrollItemInstanceID: itemResult.ItemInstanceID,
		Metadata:             metadata,
		Created:              true,
		Duplicate:            itemResult.Duplicate,
	}
	service.createReferences[input.CreateReference] = createCoordinateScrollRecord{
		input:  input,
		result: cloneCreateCoordinateScrollResult(result),
	}
	return cloneCreateCoordinateScrollResult(result), nil
}

// UseCoordinateScroll consumes one coordinate scroll and upserts receiver intel
// from the stored server-side metadata.
func (service *CoordinateScrollService) UseCoordinateScroll(input UseCoordinateScrollInput) (UseCoordinateScrollResult, error) {
	if err := input.Validate(); err != nil {
		return UseCoordinateScrollResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if record, ok := service.useReferences[input.UseReference]; ok {
		if !useCoordinateScrollRecordMatchesInput(record, input) {
			return UseCoordinateScrollResult{}, ErrCoordinateScrollReferenceConflict
		}
		duplicate := cloneUseCoordinateScrollResult(record.result)
		duplicate.Duplicate = true
		return duplicate, nil
	}

	metadata, ok := service.scrolls[input.ScrollItemInstanceID]
	if !ok {
		return UseCoordinateScrollResult{}, fmt.Errorf("scroll %q: %w", input.ScrollItemInstanceID, ErrUnknownCoordinateScroll)
	}
	if metadata.UsedAt != nil {
		return UseCoordinateScrollResult{}, fmt.Errorf("scroll %q: %w", input.ScrollItemInstanceID, ErrCoordinateScrollAlreadyUsed)
	}

	incomingIntel := playerIntelFromCoordinateScroll(input.PlayerID, metadata, input.UseReference)
	if err := incomingIntel.Validate(); err != nil {
		return UseCoordinateScrollResult{}, err
	}

	sourceLocation, err := service.scrollSourceLocation(input)
	if err != nil {
		return UseCoordinateScrollResult{}, err
	}
	consumeInput := CoordinateScrollConsumeInput{
		PlayerID:             input.PlayerID,
		ScrollItemInstanceID: input.ScrollItemInstanceID,
		ItemRef: economy.RemoveItemRef{
			Definition:     service.itemDefinition,
			ItemInstanceID: input.ScrollItemInstanceID,
		},
		SourceLocation: sourceLocation,
		Quantity:       defaultCoordinateScrollQuantity,
		Reason:         service.useReason,
		Reference:      input.UseReference,
	}
	if err := consumeInput.Validate(); err != nil {
		return UseCoordinateScrollResult{}, err
	}
	consumeResult, err := service.itemConsumer.ConsumeCoordinateScroll(consumeInput)
	if err != nil {
		return UseCoordinateScrollResult{}, err
	}

	storedIntel, updated, err := service.store.UpsertPlayerPlanetIntel(incomingIntel)
	if err != nil {
		return UseCoordinateScrollResult{}, err
	}

	usedAt := service.clock.Now().UTC()
	metadata.UsedAt = &usedAt
	metadata.UsedBy = input.PlayerID
	metadata.UseReference = input.UseReference
	if err := metadata.Validate(); err != nil {
		return UseCoordinateScrollResult{}, err
	}
	service.scrolls[input.ScrollItemInstanceID] = cloneCoordinateScrollMetadata(metadata)

	result := UseCoordinateScrollResult{
		Metadata:     metadata,
		Intel:        storedIntel,
		Used:         true,
		IntelUpdated: updated,
		Duplicate:    consumeResult.Duplicate,
	}
	service.useReferences[input.UseReference] = useCoordinateScrollRecord{
		input:  input,
		result: cloneUseCoordinateScrollResult(result),
	}
	return cloneUseCoordinateScrollResult(result), nil
}

// coordinateScrollMetadata returns the server-side metadata for one scroll item.
// It is intentionally unexported because coordinate payload lookup needs a
// player/item authorization boundary before becoming a public query.
func (service *CoordinateScrollService) coordinateScrollMetadata(scrollItemInstanceID foundation.ItemID) (CoordinateScrollMetadata, bool, error) {
	if err := scrollItemInstanceID.Validate(); err != nil {
		return CoordinateScrollMetadata{}, false, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	metadata, ok := service.scrolls[scrollItemInstanceID]
	if !ok {
		return CoordinateScrollMetadata{}, false, nil
	}
	return cloneCoordinateScrollMetadata(metadata), true, nil
}

// Validate reports whether reference is a non-empty server-generated scroll reference.
func (ref CoordinateScrollReference) Validate() error {
	if !validDiscoveryToken(string(ref)) {
		return fmt.Errorf("coordinate_scroll_reference %q: %w", ref, ErrInvalidCoordinateScroll)
	}
	return nil
}

// Validate reports whether input has the required server-resolved identity.
func (input CreateCoordinateScrollInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := input.CreateReference.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether input has the required server-resolved identity.
func (input UseCoordinateScrollInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.ScrollItemInstanceID.Validate(); err != nil {
		return fmt.Errorf("scroll_item_instance_id: %w", err)
	}
	if err := input.UseReference.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether metadata is a complete server-authored scroll payload.
func (metadata CoordinateScrollMetadata) Validate() error {
	if err := metadata.ScrollItemInstanceID.Validate(); err != nil {
		return fmt.Errorf("scroll_item_instance_id: %w", err)
	}
	if err := metadata.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := metadata.Coordinates.Validate(); err != nil {
		return err
	}
	if metadata.PlanetLevelKnown <= 0 {
		return fmt.Errorf("planet_level_known %d: %w", metadata.PlanetLevelKnown, ErrInvalidCoordinateScroll)
	}
	if err := metadata.PlanetTypeKnown.Validate(); err != nil {
		return err
	}
	if !metadata.OwnerPlayerID.IsZero() {
		if err := metadata.OwnerPlayerID.Validate(); err != nil {
			return fmt.Errorf("owner_player_id: %w", err)
		}
	}
	if err := metadata.State.Validate(); err != nil {
		return err
	}
	if metadata.State == IntelStateInvalidated {
		return ErrCoordinateScrollIntelInvalidated
	}
	if metadata.Confidence < 0 || metadata.Confidence > 100 {
		return fmt.Errorf("confidence %d: %w", metadata.Confidence, ErrInvalidIntelConfidence)
	}
	if metadata.LastVerifiedAt.IsZero() {
		return fmt.Errorf("last_verified_at: %w", ErrZeroIntelTimestamp)
	}
	if metadata.CreatedAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrZeroIntelTimestamp)
	}
	if err := metadata.CreatedBy.Validate(); err != nil {
		return fmt.Errorf("created_by: %w", err)
	}
	if !validDiscoveryToken(metadata.SourceReference) {
		return fmt.Errorf("source_reference %q: %w", metadata.SourceReference, ErrEmptyIntelSourceRef)
	}
	if metadata.UsedAt != nil {
		if metadata.UsedAt.IsZero() {
			return fmt.Errorf("used_at: %w", ErrZeroIntelTimestamp)
		}
		if err := metadata.UsedBy.Validate(); err != nil {
			return fmt.Errorf("used_by: %w", err)
		}
		if err := metadata.UseReference.Validate(); err != nil {
			return err
		}
	}
	if metadata.UsedAt == nil && (!metadata.UsedBy.IsZero() || metadata.UseReference != "") {
		return fmt.Errorf("used state: %w", ErrInvalidCoordinateScroll)
	}
	return nil
}

// Validate reports whether input can be handed to an economy item creation boundary.
func (input CoordinateScrollItemCreateInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := validateCoordinateScrollItemDefinition(input.ItemDefinition); err != nil {
		return err
	}
	if err := input.TargetLocation.Validate(); err != nil {
		return err
	}
	if input.Quantity != defaultCoordinateScrollQuantity {
		return fmt.Errorf("quantity %d: %w", input.Quantity, ErrInvalidCoordinateScrollItem)
	}
	if err := input.Reason.Validate(); err != nil {
		return err
	}
	if err := input.Reference.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether result contains a usable item instance id.
func (result CoordinateScrollItemCreateResult) Validate() error {
	if err := result.ItemInstanceID.Validate(); err != nil {
		return fmt.Errorf("item_instance_id: %w", err)
	}
	return nil
}

// Validate reports whether input can be handed to an economy remove-item boundary.
func (input CoordinateScrollConsumeInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.ScrollItemInstanceID.Validate(); err != nil {
		return fmt.Errorf("scroll_item_instance_id: %w", err)
	}
	if err := input.ItemRef.Validate(); err != nil {
		return err
	}
	if input.ItemRef.ItemInstanceID != input.ScrollItemInstanceID {
		return fmt.Errorf("item ref instance %q scroll %q: %w", input.ItemRef.ItemInstanceID, input.ScrollItemInstanceID, ErrInvalidCoordinateScrollItem)
	}
	if err := input.SourceLocation.Validate(); err != nil {
		return err
	}
	if input.Quantity != defaultCoordinateScrollQuantity {
		return fmt.Errorf("quantity %d: %w", input.Quantity, ErrInvalidCoordinateScrollItem)
	}
	if err := input.Reason.Validate(); err != nil {
		return err
	}
	if err := input.Reference.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether input identifies one scroll source lookup.
func (input CoordinateScrollSourceInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.ScrollItemInstanceID.Validate(); err != nil {
		return fmt.Errorf("scroll_item_instance_id: %w", err)
	}
	return nil
}

func normalizeCoordinateScrollConfig(config CoordinateScrollServiceConfig) (CoordinateScrollServiceConfig, error) {
	if config.Store == nil {
		config.Store = NewInMemoryStore()
	}
	if config.Clock == nil {
		config.Clock = foundation.RealClock{}
	}
	if config.ItemCreator == nil {
		return CoordinateScrollServiceConfig{}, fmt.Errorf("item_creator: %w", ErrInvalidCoordinateScrollConfig)
	}
	if config.ItemConsumer == nil {
		return CoordinateScrollServiceConfig{}, fmt.Errorf("item_consumer: %w", ErrInvalidCoordinateScrollConfig)
	}
	if config.ScrollSources == nil {
		config.ScrollSources = defaultCoordinateScrollSourceProvider{}
	}
	if err := validateCoordinateScrollItemDefinition(config.CoordinateScrollItemDefinition); err != nil {
		return CoordinateScrollServiceConfig{}, fmt.Errorf("coordinate_scroll_item_definition: %w", err)
	}
	if config.CreateReason.IsZero() {
		config.CreateReason = defaultCoordinateScrollCreateReason
	}
	if err := config.CreateReason.Validate(); err != nil {
		return CoordinateScrollServiceConfig{}, fmt.Errorf("create_reason: %w", err)
	}
	if config.UseReason.IsZero() {
		config.UseReason = defaultCoordinateScrollUseReason
	}
	if err := config.UseReason.Validate(); err != nil {
		return CoordinateScrollServiceConfig{}, fmt.Errorf("use_reason: %w", err)
	}
	return config, nil
}

func validateCoordinateScrollItemDefinition(definition economy.ItemDefinition) error {
	if err := definition.Validate(); err != nil {
		return err
	}
	if definition.Type != economy.ItemTypeInstance {
		return fmt.Errorf("item type %q: %w", definition.Type, ErrInvalidCoordinateScrollItem)
	}
	if definition.MaxStack.Int64() != 1 {
		return fmt.Errorf("max stack %d: %w", definition.MaxStack.Int64(), ErrInvalidCoordinateScrollItem)
	}
	return nil
}

func (service *CoordinateScrollService) sourceIntelForCreate(input CreateCoordinateScrollInput) (PlayerPlanetIntel, error) {
	sourceIntel, ok, err := service.store.PlayerPlanetIntel(input.PlayerID, input.PlanetID)
	if err != nil {
		return PlayerPlanetIntel{}, err
	}
	if !ok {
		return PlayerPlanetIntel{}, fmt.Errorf("player %q planet %q: %w", input.PlayerID, input.PlanetID, ErrCoordinateScrollRequiresIntel)
	}
	if sourceIntel.State == IntelStateInvalidated {
		return PlayerPlanetIntel{}, fmt.Errorf("player %q planet %q: %w", input.PlayerID, input.PlanetID, ErrCoordinateScrollIntelInvalidated)
	}
	return sourceIntel, nil
}

func (service *CoordinateScrollService) scrollSourceLocation(input UseCoordinateScrollInput) (economy.ItemLocation, error) {
	sourceInput := CoordinateScrollSourceInput{
		PlayerID:             input.PlayerID,
		ScrollItemInstanceID: input.ScrollItemInstanceID,
	}
	if err := sourceInput.Validate(); err != nil {
		return economy.ItemLocation{}, err
	}
	sourceLocation, err := service.scrollSources.CoordinateScrollSourceLocation(sourceInput)
	if err != nil {
		return economy.ItemLocation{}, err
	}
	if err := sourceLocation.Validate(); err != nil {
		return economy.ItemLocation{}, fmt.Errorf("%w: %v", ErrInvalidCoordinateScrollSource, err)
	}
	return sourceLocation, nil
}

func (defaultCoordinateScrollSourceProvider) CoordinateScrollSourceLocation(input CoordinateScrollSourceInput) (economy.ItemLocation, error) {
	if err := input.Validate(); err != nil {
		return economy.ItemLocation{}, err
	}
	location, err := coordinateScrollAccountInventoryLocation(input.PlayerID)
	if err != nil {
		return economy.ItemLocation{}, fmt.Errorf("%w: %v", ErrInvalidCoordinateScrollSource, err)
	}
	return location, nil
}

func coordinateScrollAccountInventoryLocation(playerID foundation.PlayerID) (economy.ItemLocation, error) {
	if err := playerID.Validate(); err != nil {
		return economy.ItemLocation{}, err
	}
	return economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
}

func newCoordinateScrollMetadata(
	scrollItemInstanceID foundation.ItemID,
	createdBy foundation.PlayerID,
	sourceIntel PlayerPlanetIntel,
	planet Planet,
	createdAt time.Time,
) CoordinateScrollMetadata {
	return CoordinateScrollMetadata{
		ScrollItemInstanceID: scrollItemInstanceID,
		PlanetID:             sourceIntel.PlanetID,
		Coordinates:          sourceIntel.Coordinates,
		PlanetLevelKnown:     planet.Level,
		PlanetTypeKnown:      planet.Type,
		OwnerPlayerID:        planet.OwnerPlayerID,
		State:                sourceIntel.State,
		Confidence:           sourceIntel.Confidence,
		LastVerifiedAt:       sourceIntel.LastSeenAt.UTC(),
		CreatedAt:            createdAt.UTC(),
		CreatedBy:            createdBy,
		SourceReference:      sourceIntel.SourceReference,
	}
}

func playerIntelFromCoordinateScroll(
	playerID foundation.PlayerID,
	metadata CoordinateScrollMetadata,
	useReference CoordinateScrollReference,
) PlayerPlanetIntel {
	return PlayerPlanetIntel{
		PlayerID:        playerID,
		PlanetID:        metadata.PlanetID,
		Coordinates:     metadata.Coordinates,
		State:           metadata.State,
		Confidence:      metadata.Confidence,
		LastSeenAt:      metadata.LastVerifiedAt.UTC(),
		SourceType:      IntelSourceCoordinateScrollUsed,
		SourceReference: string(useReference),
	}
}

func coordinateScrollMetadataMatchesIdentity(a CoordinateScrollMetadata, b CoordinateScrollMetadata) bool {
	return a.ScrollItemInstanceID == b.ScrollItemInstanceID &&
		a.PlanetID == b.PlanetID &&
		a.Coordinates == b.Coordinates &&
		a.LastVerifiedAt.Equal(b.LastVerifiedAt) &&
		a.Confidence == b.Confidence &&
		a.State == b.State
}

func createCoordinateScrollRecordMatchesInput(record createCoordinateScrollRecord, input CreateCoordinateScrollInput) bool {
	return record.input.PlayerID == input.PlayerID &&
		record.input.PlanetID == input.PlanetID &&
		record.input.CreateReference == input.CreateReference
}

func useCoordinateScrollRecordMatchesInput(record useCoordinateScrollRecord, input UseCoordinateScrollInput) bool {
	return record.input.PlayerID == input.PlayerID &&
		record.input.ScrollItemInstanceID == input.ScrollItemInstanceID &&
		record.input.UseReference == input.UseReference
}

func cloneCoordinateScrollMetadata(metadata CoordinateScrollMetadata) CoordinateScrollMetadata {
	clone := metadata
	clone.UsedAt = cloneTimePtr(metadata.UsedAt)
	return clone
}

func cloneCreateCoordinateScrollResult(result CreateCoordinateScrollResult) CreateCoordinateScrollResult {
	result.Metadata = cloneCoordinateScrollMetadata(result.Metadata)
	return result
}

func cloneUseCoordinateScrollResult(result UseCoordinateScrollResult) UseCoordinateScrollResult {
	result.Metadata = cloneCoordinateScrollMetadata(result.Metadata)
	result.Intel = clonePlayerPlanetIntel(result.Intel)
	return result
}
