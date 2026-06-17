package economy

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

const (
	reserveItemsOperation    = "reserve_items"
	commitReservationReason  = LedgerReason("commit_reservation")
	releaseReservationReason = LedgerReason("release_reservation")
)

var (
	ErrMissingInventoryService        = errors.New("missing inventory service")
	ErrReservationAlreadyExists       = errors.New("reservation already exists")
	ErrReservationNotFound            = errors.New("reservation not found")
	ErrReservationNotActive           = errors.New("reservation is not active")
	ErrReservationItemDefinition      = errors.New("reservation item definition missing")
	ErrReservationMoveReferenceExists = errors.New("reservation move reference already exists")
	ErrInvalidReservationLocation     = errors.New("invalid reservation location")
	ErrDuplicateInstanceReservation   = errors.New("duplicate instance reservation")
)

// ReserveItemRequirement describes one player-owned item quantity to reserve.
type ReserveItemRequirement struct {
	Definition     ItemDefinition    `json:"item_definition"`
	ItemInstanceID foundation.ItemID `json:"item_instance_id,omitempty"`
	Quantity       int64             `json:"quantity"`
	FromLocation   ItemLocation      `json:"from_location"`
}

// ReserveItemsInput describes one item reservation request for craft, market, or auction.
type ReserveItemsInput struct {
	ReservationID      ReservationID             `json:"reservation_id"`
	Kind               ReservationKind           `json:"reservation_kind"`
	PlayerID           foundation.PlayerID       `json:"player_id"`
	Requirements       []ReserveItemRequirement  `json:"requirements"`
	ReservedLocationID LocationID                `json:"reserved_location_id,omitempty"`
	Reason             LedgerReason              `json:"reason"`
	ReferenceKey       foundation.IdempotencyKey `json:"reference_id"`
	ExpiresAt          *time.Time                `json:"expires_at,omitempty"`
}

// ReserveItemsResult reports the reservation and item moves created by ReserveItems.
type ReserveItemsResult struct {
	Reservation Reservation      `json:"reservation"`
	Moves       []MoveItemResult `json:"moves"`
	Duplicate   bool             `json:"duplicate"`
}

// ReleaseReservationResult reports the reservation and item moves released by ReleaseReservation.
type ReleaseReservationResult struct {
	Reservation Reservation      `json:"reservation"`
	Moves       []MoveItemResult `json:"moves"`
	Duplicate   bool             `json:"duplicate"`
}

// CommitReservationResult reports the reservation and item moves consumed by CommitReservation.
type CommitReservationResult struct {
	Reservation Reservation      `json:"reservation"`
	Moves       []MoveItemResult `json:"moves"`
	Duplicate   bool             `json:"duplicate"`
}

// ReservationService coordinates in-memory item reservations.
type ReservationService struct {
	mu        sync.Mutex
	inventory *InventoryService

	reservations               map[ReservationID]Reservation
	reservationItemDefinitions map[ReservationID][]ItemDefinition
	reserveItemsReferences     map[reservationReferenceKey]ReserveItemsResult
	releaseReservationResults  map[ReservationID]ReleaseReservationResult
	commitReservationResults   map[ReservationID]CommitReservationResult
}

type reservationReferenceKey struct {
	playerID     foundation.PlayerID
	operation    string
	referenceKey foundation.IdempotencyKey
}

type validatedReserveItemsInput struct {
	reservationID ReservationID
	kind          ReservationKind
	playerID      foundation.PlayerID
	referenceKey  foundation.IdempotencyKey
	expiresAt     *time.Time
	moveInputs    []MoveItemInput
	quantities    []foundation.Quantity
	definitions   []ItemDefinition
	itemLines     []ReservationItemLine
}

// NewReservationService returns an in-memory reservation coordinator.
func NewReservationService(inventory *InventoryService) *ReservationService {
	return &ReservationService{
		inventory:                  inventory,
		reservations:               make(map[ReservationID]Reservation),
		reservationItemDefinitions: make(map[ReservationID][]ItemDefinition),
		reserveItemsReferences:     make(map[reservationReferenceKey]ReserveItemsResult),
		releaseReservationResults:  make(map[ReservationID]ReleaseReservationResult),
		commitReservationResults:   make(map[ReservationID]CommitReservationResult),
	}
}

// ReserveItems moves player-owned item requirements into the reservation kind's reserved location.
func (service *ReservationService) ReserveItems(input ReserveItemsInput) (ReserveItemsResult, error) {
	validated, err := input.validate()
	if err != nil {
		return ReserveItemsResult{}, err
	}
	if service == nil || service.inventory == nil {
		return ReserveItemsResult{}, ErrMissingInventoryService
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()
	service.ensureMapsLocked()

	reference := reservationReferenceKey{
		playerID:     validated.playerID,
		operation:    reserveItemsOperation,
		referenceKey: validated.referenceKey,
	}
	if previous, ok := service.reserveItemsReferences[reference]; ok {
		result := cloneReserveItemsResult(previous)
		result.Duplicate = true
		return result, nil
	}
	if _, ok := service.reservations[validated.reservationID]; ok {
		return ReserveItemsResult{}, fmt.Errorf("reservation %q: %w", validated.reservationID, ErrReservationAlreadyExists)
	}

	inventory := service.inventory
	inventory.mu.Lock()
	defer inventory.mu.Unlock()

	if err := inventory.validateReserveItemsAvailableLocked(validated.moveInputs, validated.quantities); err != nil {
		return ReserveItemsResult{}, err
	}

	now := inventory.clock.Now()
	reservation, err := NewReservation(
		validated.reservationID,
		validated.kind,
		validated.playerID,
		validated.referenceKey,
		validated.itemLines,
		nil,
	)
	if err != nil {
		return ReserveItemsResult{}, err
	}
	reservation.CreatedAt = now
	reservation.ExpiresAt = cloneTimePointer(validated.expiresAt)

	moves := make([]MoveItemResult, 0, len(validated.moveInputs))
	for index, moveInput := range validated.moveInputs {
		moveResult, err := inventory.moveItemValidatedLocked(moveInput, validated.quantities[index], now)
		if err != nil {
			return ReserveItemsResult{}, err
		}
		if moveResult.Duplicate {
			return ReserveItemsResult{}, fmt.Errorf("reference %q: %w", moveInput.ReferenceKey, ErrReservationMoveReferenceExists)
		}
		moves = append(moves, moveResult)
	}

	result := ReserveItemsResult{
		Reservation: reservation,
		Moves:       moves,
	}
	service.reservations[reservation.ReservationID] = cloneReservation(reservation)
	service.reservationItemDefinitions[reservation.ReservationID] = cloneItemDefinitions(validated.definitions)
	service.reserveItemsReferences[reference] = cloneReserveItemsResult(result)
	emitter = inventory.emitter
	if emitter != nil {
		emitted = inventory.moveResultsEventsLocked(validated.moveInputs, moves, now)
	}

	return cloneReserveItemsResult(result), nil
}

// ReleaseReservation releases an active reservation's items back to their original locations.
func (service *ReservationService) ReleaseReservation(reservationID ReservationID) (ReleaseReservationResult, error) {
	if err := reservationID.Validate(); err != nil {
		return ReleaseReservationResult{}, err
	}
	if service == nil || service.inventory == nil {
		return ReleaseReservationResult{}, ErrMissingInventoryService
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()
	service.ensureMapsLocked()

	reservation, ok := service.reservations[reservationID]
	if !ok {
		return ReleaseReservationResult{}, fmt.Errorf("reservation %q: %w", reservationID, ErrReservationNotFound)
	}
	if reservation.State == ReservationStateReleased {
		if previous, ok := service.releaseReservationResults[reservationID]; ok {
			result := cloneReleaseReservationResult(previous)
			result.Duplicate = true
			return result, nil
		}
		return ReleaseReservationResult{
			Reservation: cloneReservation(reservation),
			Duplicate:   true,
		}, nil
	}
	if reservation.State != ReservationStateActive {
		return ReleaseReservationResult{}, fmt.Errorf("reservation %q state %q: %w", reservationID, reservation.State, ErrReservationNotActive)
	}

	moveInputs, quantities, err := service.releaseMoveInputsLocked(reservation)
	if err != nil {
		return ReleaseReservationResult{}, err
	}

	inventory := service.inventory
	inventory.mu.Lock()
	defer inventory.mu.Unlock()

	if err := inventory.validateReleaseReservationAvailableLocked(moveInputs, quantities); err != nil {
		return ReleaseReservationResult{}, err
	}

	snapshot := inventory.snapshotReservationMutationLocked()
	now := inventory.clock.Now()
	moves := make([]MoveItemResult, 0, len(moveInputs))
	for index, moveInput := range moveInputs {
		moveResult, err := inventory.moveItemValidatedLocked(moveInput, quantities[index], now)
		if err != nil {
			inventory.restoreReservationMutationLocked(snapshot)
			return ReleaseReservationResult{}, err
		}
		if moveResult.Duplicate {
			inventory.restoreReservationMutationLocked(snapshot)
			return ReleaseReservationResult{}, fmt.Errorf("release line %d reference %q: %w", index, moveInput.ReferenceKey, ErrReservationMoveReferenceExists)
		}
		moves = append(moves, moveResult)
	}

	reservation.State = ReservationStateReleased
	service.reservations[reservationID] = cloneReservation(reservation)
	result := ReleaseReservationResult{
		Reservation: reservation,
		Moves:       moves,
	}
	service.releaseReservationResults[reservationID] = cloneReleaseReservationResult(result)
	emitter = inventory.emitter
	if emitter != nil {
		emitted = inventory.moveResultsEventsLocked(moveInputs, moves, now)
	}

	return cloneReleaseReservationResult(result), nil
}

// CommitReservation finalizes an active reservation without assigning downstream buyers or outputs.
func (service *ReservationService) CommitReservation(reservationID ReservationID) (CommitReservationResult, error) {
	if err := reservationID.Validate(); err != nil {
		return CommitReservationResult{}, err
	}
	if service == nil || service.inventory == nil {
		return CommitReservationResult{}, ErrMissingInventoryService
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()
	service.ensureMapsLocked()

	reservation, ok := service.reservations[reservationID]
	if !ok {
		return CommitReservationResult{}, fmt.Errorf("reservation %q: %w", reservationID, ErrReservationNotFound)
	}
	if reservation.State == ReservationStateCommitted {
		if previous, ok := service.commitReservationResults[reservationID]; ok {
			result := cloneCommitReservationResult(previous)
			result.Duplicate = true
			return result, nil
		}
		return CommitReservationResult{
			Reservation: cloneReservation(reservation),
			Duplicate:   true,
		}, nil
	}
	if reservation.State != ReservationStateActive {
		return CommitReservationResult{}, fmt.Errorf("reservation %q state %q: %w", reservationID, reservation.State, ErrReservationNotActive)
	}

	var moves []MoveItemResult
	var moveInputs []MoveItemInput
	var inventory *InventoryService
	var now time.Time
	switch reservation.Kind {
	case ReservationKindCraft:
		var quantities []foundation.Quantity
		var err error
		moveInputs, quantities, err = service.commitMoveInputsLocked(reservation)
		if err != nil {
			return CommitReservationResult{}, err
		}

		inventory = service.inventory
		inventory.mu.Lock()
		defer inventory.mu.Unlock()

		if err := inventory.validateCommitReservationAvailableLocked(moveInputs, quantities); err != nil {
			return CommitReservationResult{}, err
		}

		snapshot := inventory.snapshotReservationMutationLocked()
		now = inventory.clock.Now()
		moves = make([]MoveItemResult, 0, len(moveInputs))
		for index, moveInput := range moveInputs {
			moveResult, err := inventory.moveItemValidatedLocked(moveInput, quantities[index], now)
			if err != nil {
				inventory.restoreReservationMutationLocked(snapshot)
				return CommitReservationResult{}, err
			}
			if moveResult.Duplicate {
				inventory.restoreReservationMutationLocked(snapshot)
				return CommitReservationResult{}, fmt.Errorf("commit line %d reference %q: %w", index, moveInput.ReferenceKey, ErrReservationMoveReferenceExists)
			}
			moves = append(moves, moveResult)
		}
	case ReservationKindMarket, ReservationKindAuction:
		moves = nil
	default:
		return CommitReservationResult{}, reservation.Kind.Validate()
	}

	reservation.State = ReservationStateCommitted
	service.reservations[reservationID] = cloneReservation(reservation)
	result := CommitReservationResult{
		Reservation: reservation,
		Moves:       moves,
	}
	service.commitReservationResults[reservationID] = cloneCommitReservationResult(result)
	if inventory != nil {
		emitter = inventory.emitter
		if emitter != nil {
			emitted = inventory.moveResultsEventsLocked(moveInputs, moves, now)
		}
	}

	return cloneCommitReservationResult(result), nil
}

func (input ReserveItemsInput) validate() (validatedReserveItemsInput, error) {
	if err := input.ReservationID.Validate(); err != nil {
		return validatedReserveItemsInput{}, err
	}
	if err := input.Kind.Validate(); err != nil {
		return validatedReserveItemsInput{}, err
	}
	if err := input.PlayerID.Validate(); err != nil {
		return validatedReserveItemsInput{}, err
	}
	if len(input.Requirements) == 0 {
		return validatedReserveItemsInput{}, ErrEmptyReservationAssets
	}
	if err := input.Reason.Validate(); err != nil {
		return validatedReserveItemsInput{}, err
	}
	if err := input.ReferenceKey.Validate(); err != nil {
		return validatedReserveItemsInput{}, err
	}

	reservedLocation, err := input.reservedLocation()
	if err != nil {
		return validatedReserveItemsInput{}, err
	}

	moveInputs := make([]MoveItemInput, 0, len(input.Requirements))
	quantities := make([]foundation.Quantity, 0, len(input.Requirements))
	definitions := make([]ItemDefinition, 0, len(input.Requirements))
	itemLines := make([]ReservationItemLine, 0, len(input.Requirements))
	for index, requirement := range input.Requirements {
		referenceKey, err := reserveItemMoveReference(input.ReferenceKey, index, len(input.Requirements))
		if err != nil {
			return validatedReserveItemsInput{}, err
		}
		moveInput := MoveItemInput{
			PlayerID: input.PlayerID,
			ItemRef: MoveItemRef{
				Definition:     requirement.Definition,
				ItemInstanceID: requirement.ItemInstanceID,
			},
			FromLocation: requirement.FromLocation,
			ToLocation:   reservedLocation,
			Quantity:     requirement.Quantity,
			Reason:       input.Reason,
			ReferenceKey: referenceKey,
		}
		quantity, err := moveInput.validate()
		if err != nil {
			return validatedReserveItemsInput{}, fmt.Errorf("requirement %d: %w", index, err)
		}
		itemLine := ReservationItemLine{
			ItemID:           requirement.Definition.ItemID,
			ItemInstanceID:   requirement.ItemInstanceID,
			Quantity:         quantity,
			FromLocation:     requirement.FromLocation,
			ReservedLocation: reservedLocation,
		}
		if err := itemLine.Validate(); err != nil {
			return validatedReserveItemsInput{}, fmt.Errorf("requirement %d: %w", index, err)
		}

		moveInputs = append(moveInputs, moveInput)
		quantities = append(quantities, quantity)
		definitions = append(definitions, requirement.Definition)
		itemLines = append(itemLines, itemLine)
	}

	return validatedReserveItemsInput{
		reservationID: input.ReservationID,
		kind:          input.Kind,
		playerID:      input.PlayerID,
		referenceKey:  input.ReferenceKey,
		expiresAt:     cloneTimePointer(input.ExpiresAt),
		moveInputs:    moveInputs,
		quantities:    quantities,
		definitions:   definitions,
		itemLines:     itemLines,
	}, nil
}

func (service *ReservationService) releaseMoveInputsLocked(reservation Reservation) ([]MoveItemInput, []foundation.Quantity, error) {
	if len(reservation.ItemLines) == 0 {
		return nil, nil, nil
	}

	definitions, ok := service.reservationItemDefinitions[reservation.ReservationID]
	if !ok || len(definitions) != len(reservation.ItemLines) {
		return nil, nil, fmt.Errorf("reservation %q: %w", reservation.ReservationID, ErrReservationItemDefinition)
	}
	expectedReservedKind, err := reservation.Kind.ReservedLocationKind()
	if err != nil {
		return nil, nil, err
	}

	moveInputs := make([]MoveItemInput, 0, len(reservation.ItemLines))
	quantities := make([]foundation.Quantity, 0, len(reservation.ItemLines))
	for index, line := range reservation.ItemLines {
		if line.ReservedLocation.Kind != expectedReservedKind {
			return nil, nil, fmt.Errorf("reservation %q line %d reserved location %q: %w", reservation.ReservationID, index, line.ReservedLocation.Kind, ErrInvalidReservationLocation)
		}
		definition := definitions[index]
		if definition.ItemID != line.ItemID {
			return nil, nil, fmt.Errorf("reservation %q line %d: %w", reservation.ReservationID, index, ErrReservationItemDefinition)
		}
		referenceKey, err := releaseReservationMoveReference(reservation.ReferenceKey, index, len(reservation.ItemLines))
		if err != nil {
			return nil, nil, err
		}
		moveInput := MoveItemInput{
			PlayerID: reservation.PlayerID,
			ItemRef: MoveItemRef{
				Definition:     definition,
				ItemInstanceID: line.ItemInstanceID,
			},
			FromLocation: line.ReservedLocation,
			ToLocation:   line.FromLocation,
			Quantity:     line.Quantity.Int64(),
			Reason:       releaseReservationReason,
			ReferenceKey: referenceKey,
		}
		quantity, err := moveInput.validateSystemMove()
		if err != nil {
			return nil, nil, fmt.Errorf("release line %d: %w", index, err)
		}

		moveInputs = append(moveInputs, moveInput)
		quantities = append(quantities, quantity)
	}

	return moveInputs, quantities, nil
}

func (service *ReservationService) commitMoveInputsLocked(reservation Reservation) ([]MoveItemInput, []foundation.Quantity, error) {
	if len(reservation.ItemLines) == 0 {
		return nil, nil, nil
	}

	definitions, ok := service.reservationItemDefinitions[reservation.ReservationID]
	if !ok || len(definitions) != len(reservation.ItemLines) {
		return nil, nil, fmt.Errorf("reservation %q: %w", reservation.ReservationID, ErrReservationItemDefinition)
	}
	expectedReservedKind, err := reservation.Kind.ReservedLocationKind()
	if err != nil {
		return nil, nil, err
	}
	systemSink, err := commitSystemSinkLocation(reservation.ReservationID)
	if err != nil {
		return nil, nil, err
	}

	moveInputs := make([]MoveItemInput, 0, len(reservation.ItemLines))
	quantities := make([]foundation.Quantity, 0, len(reservation.ItemLines))
	for index, line := range reservation.ItemLines {
		if line.ReservedLocation.Kind != expectedReservedKind {
			return nil, nil, fmt.Errorf("reservation %q line %d reserved location %q: %w", reservation.ReservationID, index, line.ReservedLocation.Kind, ErrInvalidReservationLocation)
		}
		definition := definitions[index]
		if definition.ItemID != line.ItemID {
			return nil, nil, fmt.Errorf("reservation %q line %d: %w", reservation.ReservationID, index, ErrReservationItemDefinition)
		}
		referenceKey, err := commitReservationMoveReference(reservation.ReferenceKey, index, len(reservation.ItemLines))
		if err != nil {
			return nil, nil, err
		}
		moveInput := MoveItemInput{
			PlayerID: reservation.PlayerID,
			ItemRef: MoveItemRef{
				Definition:     definition,
				ItemInstanceID: line.ItemInstanceID,
			},
			FromLocation: line.ReservedLocation,
			ToLocation:   systemSink,
			Quantity:     line.Quantity.Int64(),
			Reason:       commitReservationReason,
			ReferenceKey: referenceKey,
		}
		quantity, err := moveInput.validateSystemMove()
		if err != nil {
			return nil, nil, fmt.Errorf("commit line %d: %w", index, err)
		}

		moveInputs = append(moveInputs, moveInput)
		quantities = append(quantities, quantity)
	}

	return moveInputs, quantities, nil
}

func (input ReserveItemsInput) reservedLocation() (ItemLocation, error) {
	locationKind, err := input.Kind.ReservedLocationKind()
	if err != nil {
		return ItemLocation{}, err
	}

	locationID := input.ReservedLocationID
	if locationID.IsZero() {
		locationID = LocationID(input.ReservationID.String())
	}
	location, err := NewItemLocation(locationKind, locationID.String())
	if err != nil {
		return ItemLocation{}, fmt.Errorf("%w: %v", ErrInvalidReservationLocation, err)
	}
	return location, nil
}

func (service *ReservationService) ensureMapsLocked() {
	if service.reservations == nil {
		service.reservations = make(map[ReservationID]Reservation)
	}
	if service.reservationItemDefinitions == nil {
		service.reservationItemDefinitions = make(map[ReservationID][]ItemDefinition)
	}
	if service.reserveItemsReferences == nil {
		service.reserveItemsReferences = make(map[reservationReferenceKey]ReserveItemsResult)
	}
	if service.releaseReservationResults == nil {
		service.releaseReservationResults = make(map[ReservationID]ReleaseReservationResult)
	}
	if service.commitReservationResults == nil {
		service.commitReservationResults = make(map[ReservationID]CommitReservationResult)
	}
}

func (service *InventoryService) validateReserveItemsAvailableLocked(moveInputs []MoveItemInput, quantities []foundation.Quantity) error {
	stackRequirements := make(map[reserveStackKey]int64)
	instanceRequirements := make(map[reserveInstanceKey]struct{})

	for index, moveInput := range moveInputs {
		reference := inventoryReferenceKey{
			playerID:     moveInput.PlayerID,
			operation:    moveItemOperation,
			referenceKey: moveInput.ReferenceKey,
		}
		if _, ok := service.moveItemReferences[reference]; ok {
			return fmt.Errorf("requirement %d reference %q: %w", index, moveInput.ReferenceKey, ErrReservationMoveReferenceExists)
		}

		switch moveInput.ItemRef.Definition.Type {
		case ItemTypeStackable:
			key := newReserveStackKey(moveInput)
			stackRequirements[key] += quantities[index].Int64()
			available := service.stackableQuantityForDefinitionLocked(moveInput.PlayerID, moveInput.ItemRef.Definition, moveInput.FromLocation)
			if available <= 0 {
				return fmt.Errorf("requirement %d: %w", index, ErrItemNotOwned)
			}
			if stackRequirements[key] > available {
				return fmt.Errorf("requirement %d have %d need %d: %w", index, available, stackRequirements[key], ErrInsufficientItemQuantity)
			}
		case ItemTypeInstance:
			key := newReserveInstanceKey(moveInput)
			if _, ok := instanceRequirements[key]; ok {
				return fmt.Errorf("requirement %d instance %q: %w", index, moveInput.ItemRef.ItemInstanceID, ErrDuplicateInstanceReservation)
			}
			if service.instanceItemIndexLocked(moveInput) < 0 {
				return fmt.Errorf("requirement %d: %w", index, ErrItemNotOwned)
			}
			instanceRequirements[key] = struct{}{}
		default:
			return fmt.Errorf("requirement %d: %w", index, moveInput.ItemRef.Definition.Type.Validate())
		}
	}

	return nil
}

func (service *InventoryService) validateReleaseReservationAvailableLocked(moveInputs []MoveItemInput, quantities []foundation.Quantity) error {
	return service.validateReservationMovesAvailableLocked("release line", moveInputs, quantities)
}

func (service *InventoryService) validateCommitReservationAvailableLocked(moveInputs []MoveItemInput, quantities []foundation.Quantity) error {
	return service.validateReservationMovesAvailableLocked("commit line", moveInputs, quantities)
}

func (service *InventoryService) validateReservationMovesAvailableLocked(lineLabel string, moveInputs []MoveItemInput, quantities []foundation.Quantity) error {
	stackRequirements := make(map[reserveStackKey]int64)
	instanceRequirements := make(map[reserveInstanceKey]struct{})

	for index, moveInput := range moveInputs {
		reference := inventoryReferenceKey{
			playerID:     moveInput.PlayerID,
			operation:    moveItemOperation,
			referenceKey: moveInput.ReferenceKey,
		}
		if _, ok := service.moveItemReferences[reference]; ok {
			return fmt.Errorf("%s %d reference %q: %w", lineLabel, index, moveInput.ReferenceKey, ErrReservationMoveReferenceExists)
		}

		switch moveInput.ItemRef.Definition.Type {
		case ItemTypeStackable:
			key := newReserveStackKey(moveInput)
			stackRequirements[key] += quantities[index].Int64()
			available := service.stackableQuantityForDefinitionLocked(moveInput.PlayerID, moveInput.ItemRef.Definition, moveInput.FromLocation)
			if available <= 0 {
				return fmt.Errorf("%s %d: %w", lineLabel, index, ErrItemNotOwned)
			}
			if stackRequirements[key] > available {
				return fmt.Errorf("%s %d have %d need %d: %w", lineLabel, index, available, stackRequirements[key], ErrInsufficientItemQuantity)
			}
		case ItemTypeInstance:
			key := newReserveInstanceKey(moveInput)
			if _, ok := instanceRequirements[key]; ok {
				return fmt.Errorf("%s %d instance %q: %w", lineLabel, index, moveInput.ItemRef.ItemInstanceID, ErrDuplicateInstanceReservation)
			}
			if service.instanceItemIndexLocked(moveInput) < 0 {
				return fmt.Errorf("%s %d: %w", lineLabel, index, ErrItemNotOwned)
			}
			instanceRequirements[key] = struct{}{}
		default:
			return fmt.Errorf("%s %d: %w", lineLabel, index, moveInput.ItemRef.Definition.Type.Validate())
		}
	}

	return nil
}

type reserveStackKey struct {
	playerID           foundation.PlayerID
	itemID             foundation.ItemID
	sourceDefinitionID string
	sourceVersion      string
	location           ItemLocation
}

type reserveInstanceKey struct {
	playerID       foundation.PlayerID
	itemID         foundation.ItemID
	itemInstanceID foundation.ItemID
	location       ItemLocation
}

func newReserveStackKey(input MoveItemInput) reserveStackKey {
	return reserveStackKey{
		playerID:           input.PlayerID,
		itemID:             input.ItemRef.Definition.ItemID,
		sourceDefinitionID: input.ItemRef.Definition.Source.DefinitionID.String(),
		sourceVersion:      input.ItemRef.Definition.Source.Version.String(),
		location:           input.FromLocation,
	}
}

func newReserveInstanceKey(input MoveItemInput) reserveInstanceKey {
	return reserveInstanceKey{
		playerID:       input.PlayerID,
		itemID:         input.ItemRef.Definition.ItemID,
		itemInstanceID: input.ItemRef.ItemInstanceID,
		location:       input.FromLocation,
	}
}

func reserveItemMoveReference(referenceKey foundation.IdempotencyKey, lineIndex int, lineCount int) (foundation.IdempotencyKey, error) {
	parts := strings.Split(referenceKey.String(), ":")
	if len(parts) == 0 {
		return "", foundation.ErrInvalidIdempotencyKey
	}
	if lineCount == 1 {
		parts[len(parts)-1] = fmt.Sprintf("%s-reserve", parts[len(parts)-1])
	} else {
		parts[len(parts)-1] = fmt.Sprintf("%s-reserve-line-%d", parts[len(parts)-1], lineIndex+1)
	}
	return foundation.ParseIdempotencyKey(strings.Join(parts, ":"))
}

func releaseReservationMoveReference(referenceKey foundation.IdempotencyKey, lineIndex int, lineCount int) (foundation.IdempotencyKey, error) {
	parts := strings.Split(referenceKey.String(), ":")
	if len(parts) == 0 {
		return "", foundation.ErrInvalidIdempotencyKey
	}
	if lineCount == 1 {
		parts[len(parts)-1] = fmt.Sprintf("%s-release", parts[len(parts)-1])
	} else {
		parts[len(parts)-1] = fmt.Sprintf("%s-release-line-%d", parts[len(parts)-1], lineIndex+1)
	}
	return foundation.ParseIdempotencyKey(strings.Join(parts, ":"))
}

func commitReservationMoveReference(referenceKey foundation.IdempotencyKey, lineIndex int, lineCount int) (foundation.IdempotencyKey, error) {
	parts := strings.Split(referenceKey.String(), ":")
	if len(parts) == 0 {
		return "", foundation.ErrInvalidIdempotencyKey
	}
	if lineCount == 1 {
		parts[len(parts)-1] = fmt.Sprintf("%s-commit", parts[len(parts)-1])
	} else {
		parts[len(parts)-1] = fmt.Sprintf("%s-commit-line-%d", parts[len(parts)-1], lineIndex+1)
	}
	return foundation.ParseIdempotencyKey(strings.Join(parts, ":"))
}

func commitSystemSinkLocation(reservationID ReservationID) (ItemLocation, error) {
	location, err := NewItemLocation(LocationKindSystemSink, reservationID.String())
	if err != nil {
		return ItemLocation{}, fmt.Errorf("%w: %v", ErrInvalidReservationLocation, err)
	}
	return location, nil
}

func cloneReserveItemsResult(result ReserveItemsResult) ReserveItemsResult {
	result.Reservation = cloneReservation(result.Reservation)
	result.Moves = append([]MoveItemResult(nil), result.Moves...)
	for index := range result.Moves {
		result.Moves[index] = cloneMoveItemResult(result.Moves[index])
	}
	return result
}

func cloneReleaseReservationResult(result ReleaseReservationResult) ReleaseReservationResult {
	result.Reservation = cloneReservation(result.Reservation)
	result.Moves = append([]MoveItemResult(nil), result.Moves...)
	for index := range result.Moves {
		result.Moves[index] = cloneMoveItemResult(result.Moves[index])
	}
	return result
}

func cloneCommitReservationResult(result CommitReservationResult) CommitReservationResult {
	result.Reservation = cloneReservation(result.Reservation)
	result.Moves = append([]MoveItemResult(nil), result.Moves...)
	for index := range result.Moves {
		result.Moves[index] = cloneMoveItemResult(result.Moves[index])
	}
	return result
}

func cloneReservation(reservation Reservation) Reservation {
	reservation.ItemLines = append([]ReservationItemLine(nil), reservation.ItemLines...)
	reservation.CurrencyLines = append([]ReservationCurrencyLine(nil), reservation.CurrencyLines...)
	reservation.ExpiresAt = cloneTimePointer(reservation.ExpiresAt)
	return reservation
}

func cloneItemDefinitions(definitions []ItemDefinition) []ItemDefinition {
	cloned := make([]ItemDefinition, len(definitions))
	for index, definition := range definitions {
		cloned[index] = cloneItemDefinition(definition)
	}
	return cloned
}

func cloneItemDefinition(definition ItemDefinition) ItemDefinition {
	definition.TradeFlags = append([]TradeFlag(nil), definition.TradeFlags...)
	definition.BindRules = append([]BindRule(nil), definition.BindRules...)
	definition.MetadataSchema = cloneRawJSON(definition.MetadataSchema)
	return definition
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

type inventoryReservationMutationSnapshot struct {
	nextItemSequence   int64
	nextLedgerSequence int64
	stackableItems     []StackableItem
	instanceItems      []InstanceItem
	itemLedgerEntries  []ItemLedgerEntry
	moveItemReferences map[inventoryReferenceKey]MoveItemResult
}

func (service *InventoryService) snapshotReservationMutationLocked() inventoryReservationMutationSnapshot {
	return inventoryReservationMutationSnapshot{
		nextItemSequence:   service.nextItemSequence,
		nextLedgerSequence: service.nextLedgerSequence,
		stackableItems:     append([]StackableItem(nil), service.stackableItems...),
		instanceItems:      append([]InstanceItem(nil), service.instanceItems...),
		itemLedgerEntries:  append([]ItemLedgerEntry(nil), service.itemLedgerEntries...),
		moveItemReferences: cloneMoveItemReferences(service.moveItemReferences),
	}
}

func (service *InventoryService) restoreReservationMutationLocked(snapshot inventoryReservationMutationSnapshot) {
	service.nextItemSequence = snapshot.nextItemSequence
	service.nextLedgerSequence = snapshot.nextLedgerSequence
	service.stackableItems = append([]StackableItem(nil), snapshot.stackableItems...)
	service.instanceItems = append([]InstanceItem(nil), snapshot.instanceItems...)
	service.itemLedgerEntries = append([]ItemLedgerEntry(nil), snapshot.itemLedgerEntries...)
	service.moveItemReferences = cloneMoveItemReferences(snapshot.moveItemReferences)
}

func cloneMoveItemReferences(references map[inventoryReferenceKey]MoveItemResult) map[inventoryReferenceKey]MoveItemResult {
	if references == nil {
		return nil
	}
	cloned := make(map[inventoryReferenceKey]MoveItemResult, len(references))
	for key, result := range references {
		cloned[key] = cloneMoveItemResult(result)
	}
	return cloned
}
