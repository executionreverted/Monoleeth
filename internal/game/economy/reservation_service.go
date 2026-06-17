package economy

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

const reserveItemsOperation = "reserve_items"

var (
	ErrMissingInventoryService        = errors.New("missing inventory service")
	ErrReservationAlreadyExists       = errors.New("reservation already exists")
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

// ReservationService coordinates in-memory item reservations.
type ReservationService struct {
	mu        sync.Mutex
	inventory *InventoryService

	reservations           map[ReservationID]Reservation
	reserveItemsReferences map[reservationReferenceKey]ReserveItemsResult
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
	itemLines     []ReservationItemLine
}

// NewReservationService returns an in-memory reservation coordinator.
func NewReservationService(inventory *InventoryService) *ReservationService {
	return &ReservationService{
		inventory:              inventory,
		reservations:           make(map[ReservationID]Reservation),
		reserveItemsReferences: make(map[reservationReferenceKey]ReserveItemsResult),
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

	service.mu.Lock()
	defer service.mu.Unlock()
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
	service.reserveItemsReferences[reference] = cloneReserveItemsResult(result)

	return cloneReserveItemsResult(result), nil
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
		itemLines:     itemLines,
	}, nil
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
	if service.reserveItemsReferences == nil {
		service.reserveItemsReferences = make(map[reservationReferenceKey]ReserveItemsResult)
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
	if lineCount == 1 {
		return referenceKey, nil
	}

	parts := strings.Split(referenceKey.String(), ":")
	if len(parts) == 0 {
		return "", foundation.ErrInvalidIdempotencyKey
	}
	parts[len(parts)-1] = fmt.Sprintf("%s-reserve-line-%d", parts[len(parts)-1], lineIndex+1)
	return foundation.ParseIdempotencyKey(strings.Join(parts, ":"))
}

func cloneReserveItemsResult(result ReserveItemsResult) ReserveItemsResult {
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

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
