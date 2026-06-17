package economy

import (
	"errors"
	"fmt"
	"sync"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

var (
	ErrNilInventoryService        = errors.New("nil inventory service")
	ErrInvalidCargoLocation       = errors.New("invalid cargo location")
	ErrNegativeCargoCapacity      = errors.New("negative cargo capacity")
	ErrCargoCapacityExceeded      = errors.New("cargo capacity exceeded")
	ErrUnknownCargoItemDefinition = errors.New("unknown cargo item definition")
	ErrCargoWeightOverflow        = errors.New("cargo weight overflow")
)

// CargoAddItemInput describes one server-authoritative cargo add attempt.
type CargoAddItemInput struct {
	PlayerID           foundation.PlayerID       `json:"player_id"`
	ActiveCargo        ItemLocation              `json:"active_cargo_location"`
	ItemDefinition     ItemDefinition            `json:"item_definition"`
	Quantity           int64                     `json:"quantity"`
	CargoCapacityUnits int64                     `json:"cargo_capacity_units"`
	Reason             LedgerReason              `json:"reason"`
	ReferenceKey       foundation.IdempotencyKey `json:"reference_id"`
}

// CargoMoveItemInput describes one server-authoritative cargo move attempt.
type CargoMoveItemInput struct {
	PlayerID           foundation.PlayerID       `json:"player_id"`
	ItemRef            MoveItemRef               `json:"item_ref"`
	FromLocation       ItemLocation              `json:"from_location"`
	ActiveCargo        ItemLocation              `json:"active_cargo_location"`
	Quantity           int64                     `json:"quantity"`
	CargoCapacityUnits int64                     `json:"cargo_capacity_units"`
	Reason             LedgerReason              `json:"reason"`
	ReferenceKey       foundation.IdempotencyKey `json:"reference_id"`
}

// CargoService validates ship cargo capacity before delegating item mutation to InventoryService.
type CargoService struct {
	mu        sync.Mutex
	inventory *InventoryService

	definitions map[cargoDefinitionKey]ItemDefinition

	emitter           EventEmitter
	nextEventSequence uint64
}

type cargoDefinitionKey struct {
	source catalog.VersionedDefinition
	itemID foundation.ItemID
}

// NewCargoService returns a capacity-checking wrapper around inventory item adds.
func NewCargoService(inventory *InventoryService) *CargoService {
	return &CargoService{
		inventory:   inventory,
		definitions: make(map[cargoDefinitionKey]ItemDefinition),
	}
}

// RegisterItemDefinition makes an existing cargo row's weight available for capacity checks.
func (service *CargoService) RegisterItemDefinition(definition ItemDefinition) error {
	if err := definition.Validate(); err != nil {
		return err
	}
	if service == nil {
		return ErrNilInventoryService
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	service.definitions[cargoKeyForDefinition(definition)] = definition
	return nil
}

// AddItem validates active cargo capacity, then uses InventoryService.AddItem for the mutation.
func (service *CargoService) AddItem(input CargoAddItemInput) (AddItemResult, error) {
	quantity, err := input.validate()
	if err != nil {
		return AddItemResult{}, err
	}
	if service == nil || service.inventory == nil {
		return AddItemResult{}, ErrNilInventoryService
	}

	var inventoryEmitted []events.EventEnvelope
	var inventoryEmitter EventEmitter
	var cargoEmitted []events.EventEnvelope
	var cargoEmitter EventEmitter
	service.mu.Lock()
	inventory := service.inventory
	inventory.mu.Lock()

	if result, ok := inventory.duplicateAddItemResultLocked(input.PlayerID, input.ReferenceKey); ok {
		inventory.mu.Unlock()
		service.mu.Unlock()
		return result, nil
	}

	incomingUnits, err := checkedCargoUnits(input.ItemDefinition.WeightUnits.Int64(), quantity.Int64())
	if err != nil {
		inventory.mu.Unlock()
		service.mu.Unlock()
		return AddItemResult{}, err
	}

	usedUnits, err := service.usedCargoUnitsFromInventoryLocked(input.PlayerID, input.ActiveCargo, input.ItemDefinition)
	if err != nil {
		inventory.mu.Unlock()
		service.mu.Unlock()
		return AddItemResult{}, err
	}
	if usedUnits > input.CargoCapacityUnits || incomingUnits > input.CargoCapacityUnits-usedUnits {
		inventory.mu.Unlock()
		service.mu.Unlock()
		return AddItemResult{}, fmt.Errorf(
			"used %d incoming %d capacity %d: %w",
			usedUnits,
			incomingUnits,
			input.CargoCapacityUnits,
			ErrCargoCapacityExceeded,
		)
	}

	now := inventory.clock.Now()
	addInput := AddItemInput{
		PlayerID:       input.PlayerID,
		ItemDefinition: input.ItemDefinition,
		Quantity:       input.Quantity,
		Location:       input.ActiveCargo,
		Reason:         input.Reason,
		ReferenceKey:   input.ReferenceKey,
	}
	result, err := inventory.addItemValidatedLocked(addInput, quantity, now)
	if err != nil {
		inventory.mu.Unlock()
		service.mu.Unlock()
		return AddItemResult{}, err
	}

	service.definitions[cargoKeyForDefinition(input.ItemDefinition)] = input.ItemDefinition
	inventoryEmitter = inventory.emitter
	cargoEmitter = service.emitter
	if inventoryEmitter != nil && !result.Duplicate {
		inventoryEmitted = inventory.addItemEventsLocked(addInput, result, now)
	}
	if cargoEmitter != nil && !result.Duplicate {
		cargoEmitted = []events.EventEnvelope{
			service.cargoUpdatedEventLocked(input, result, result.LedgerEntry.CreatedAt),
		}
	}

	inventory.mu.Unlock()
	service.mu.Unlock()
	emitEvents(inventoryEmitter, inventoryEmitted)
	emitEvents(cargoEmitter, cargoEmitted)
	return result, nil
}

// MoveItem validates active cargo capacity, then moves existing player-owned items into cargo.
func (service *CargoService) MoveItem(input CargoMoveItemInput) (MoveItemResult, error) {
	quantity, err := input.validate()
	if err != nil {
		return MoveItemResult{}, err
	}
	if service == nil || service.inventory == nil {
		return MoveItemResult{}, ErrNilInventoryService
	}

	var inventoryEmitted []events.EventEnvelope
	var inventoryEmitter EventEmitter
	var cargoEmitted []events.EventEnvelope
	var cargoEmitter EventEmitter
	service.mu.Lock()
	inventory := service.inventory
	inventory.mu.Lock()

	moveInput := input.toMoveItemInput()
	if result, ok := inventory.duplicateMoveItemResultLocked(input.PlayerID, input.ReferenceKey); ok {
		inventory.mu.Unlock()
		service.mu.Unlock()
		return result, nil
	}

	incomingUnits, err := checkedCargoUnits(input.ItemRef.Definition.WeightUnits.Int64(), quantity.Int64())
	if err != nil {
		inventory.mu.Unlock()
		service.mu.Unlock()
		return MoveItemResult{}, err
	}
	usedUnits, err := service.usedCargoUnitsFromInventoryLocked(input.PlayerID, input.ActiveCargo, input.ItemRef.Definition)
	if err != nil {
		inventory.mu.Unlock()
		service.mu.Unlock()
		return MoveItemResult{}, err
	}
	if usedUnits > input.CargoCapacityUnits || incomingUnits > input.CargoCapacityUnits-usedUnits {
		inventory.mu.Unlock()
		service.mu.Unlock()
		return MoveItemResult{}, fmt.Errorf(
			"used %d incoming %d capacity %d: %w",
			usedUnits,
			incomingUnits,
			input.CargoCapacityUnits,
			ErrCargoCapacityExceeded,
		)
	}

	now := inventory.clock.Now()
	result, err := inventory.moveItemValidatedLocked(moveInput, quantity, now)
	if err != nil {
		inventory.mu.Unlock()
		service.mu.Unlock()
		return MoveItemResult{}, err
	}

	service.definitions[cargoKeyForDefinition(input.ItemRef.Definition)] = input.ItemRef.Definition
	inventoryEmitter = inventory.emitter
	cargoEmitter = service.emitter
	if inventoryEmitter != nil && !result.Duplicate {
		inventoryEmitted = inventory.moveItemEventsLocked(moveInput, result, now)
	}
	if cargoEmitter != nil && !result.Duplicate {
		cargoEmitted = []events.EventEnvelope{
			service.cargoUpdatedMoveEventLocked(input, result, now),
		}
	}

	inventory.mu.Unlock()
	service.mu.Unlock()
	emitEvents(inventoryEmitter, inventoryEmitted)
	emitEvents(cargoEmitter, cargoEmitted)
	return result, nil
}

func (service *InventoryService) duplicateAddItemResultLocked(playerID foundation.PlayerID, referenceKey foundation.IdempotencyKey) (AddItemResult, bool) {
	reference := inventoryReferenceKey{
		playerID:     playerID,
		operation:    addItemOperation,
		referenceKey: referenceKey,
	}
	previous, ok := service.addItemReferences[reference]
	if !ok {
		return AddItemResult{}, false
	}
	result := cloneAddItemResult(previous)
	result.Duplicate = true
	return result, true
}

func (service *InventoryService) duplicateMoveItemResultLocked(playerID foundation.PlayerID, referenceKey foundation.IdempotencyKey) (MoveItemResult, bool) {
	reference := inventoryReferenceKey{
		playerID:     playerID,
		operation:    moveItemOperation,
		referenceKey: referenceKey,
	}
	previous, ok := service.moveItemReferences[reference]
	if !ok {
		return MoveItemResult{}, false
	}
	result := cloneMoveItemResult(previous)
	result.Duplicate = true
	return result, true
}

func (input CargoAddItemInput) validate() (foundation.Quantity, error) {
	quantity, err := AddItemInput{
		PlayerID:       input.PlayerID,
		ItemDefinition: input.ItemDefinition,
		Quantity:       input.Quantity,
		Location:       input.ActiveCargo,
		Reason:         input.Reason,
		ReferenceKey:   input.ReferenceKey,
	}.validateCargoAdd()
	if err != nil {
		return foundation.Quantity{}, err
	}
	if input.ActiveCargo.Kind != LocationKindShipCargo {
		return foundation.Quantity{}, fmt.Errorf("location %q: %w", input.ActiveCargo.Kind, ErrInvalidCargoLocation)
	}
	if input.CargoCapacityUnits < 0 {
		return foundation.Quantity{}, fmt.Errorf("capacity %d: %w", input.CargoCapacityUnits, ErrNegativeCargoCapacity)
	}
	return quantity, nil
}

func (input CargoMoveItemInput) validate() (foundation.Quantity, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.ItemRef.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.FromLocation.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.ActiveCargo.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if input.ActiveCargo.Kind != LocationKindShipCargo {
		return foundation.Quantity{}, fmt.Errorf("location %q: %w", input.ActiveCargo.Kind, ErrInvalidCargoLocation)
	}
	quantity, err := input.toMoveItemInput().validateCargoMove()
	if err != nil {
		return foundation.Quantity{}, err
	}
	if input.CargoCapacityUnits < 0 {
		return foundation.Quantity{}, fmt.Errorf("capacity %d: %w", input.CargoCapacityUnits, ErrNegativeCargoCapacity)
	}
	if err := input.Reason.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.ReferenceKey.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	return quantity, nil
}

func (input CargoMoveItemInput) toMoveItemInput() MoveItemInput {
	return MoveItemInput{
		PlayerID:     input.PlayerID,
		ItemRef:      input.ItemRef,
		FromLocation: input.FromLocation,
		ToLocation:   input.ActiveCargo,
		Quantity:     input.Quantity,
		Reason:       input.Reason,
		ReferenceKey: input.ReferenceKey,
	}
}

func (service *CargoService) usedCargoUnitsLocked(input CargoAddItemInput) (int64, error) {
	service.inventory.mu.Lock()
	defer service.inventory.mu.Unlock()

	return service.usedCargoUnitsFromInventoryLocked(input.PlayerID, input.ActiveCargo, input.ItemDefinition)
}

func (service *CargoService) usedCargoUnitsFromInventoryLocked(playerID foundation.PlayerID, activeCargo ItemLocation, incomingDefinition ItemDefinition) (int64, error) {
	var usedUnits int64

	for _, item := range service.inventory.stackableItems {
		if item.OwnerPlayerID != playerID || item.Location != activeCargo {
			continue
		}
		itemUnits, err := service.itemCargoUnitsLocked(item.Source, item.ItemID, item.Quantity.Int64(), incomingDefinition)
		if err != nil {
			return 0, err
		}
		usedUnits, err = checkedCargoUnitSum(usedUnits, itemUnits)
		if err != nil {
			return 0, err
		}
	}

	for _, item := range service.inventory.instanceItems {
		if item.OwnerPlayerID != playerID || item.Location != activeCargo {
			continue
		}
		itemUnits, err := service.itemCargoUnitsLocked(item.Source, item.ItemID, item.Quantity.Int64(), incomingDefinition)
		if err != nil {
			return 0, err
		}
		usedUnits, err = checkedCargoUnitSum(usedUnits, itemUnits)
		if err != nil {
			return 0, err
		}
	}

	return usedUnits, nil
}

func (service *CargoService) itemCargoUnitsLocked(
	source catalog.VersionedDefinition,
	itemID foundation.ItemID,
	quantity int64,
	incomingDefinition ItemDefinition,
) (int64, error) {
	weightUnits, err := service.weightUnitsLocked(source, itemID, incomingDefinition)
	if err != nil {
		return 0, err
	}
	return checkedCargoUnits(weightUnits, quantity)
}

func (service *CargoService) weightUnitsLocked(
	source catalog.VersionedDefinition,
	itemID foundation.ItemID,
	incomingDefinition ItemDefinition,
) (int64, error) {
	key := cargoDefinitionKey{source: source, itemID: itemID}
	if key == cargoKeyForDefinition(incomingDefinition) {
		return incomingDefinition.WeightUnits.Int64(), nil
	}
	if definition, ok := service.definitions[key]; ok {
		return definition.WeightUnits.Int64(), nil
	}
	return 0, fmt.Errorf("item %q source %q: %w", itemID, source.Version, ErrUnknownCargoItemDefinition)
}

func cargoKeyForDefinition(definition ItemDefinition) cargoDefinitionKey {
	return cargoDefinitionKey{
		source: definition.Source,
		itemID: definition.ItemID,
	}
}

func checkedCargoUnits(weightUnits int64, quantity int64) (int64, error) {
	const maxInt64 = int64(1<<63 - 1)
	if weightUnits > 0 && quantity > maxInt64/weightUnits {
		return 0, ErrCargoWeightOverflow
	}
	return weightUnits * quantity, nil
}

func checkedCargoUnitSum(left int64, right int64) (int64, error) {
	const maxInt64 = int64(1<<63 - 1)
	if right > maxInt64-left {
		return 0, ErrCargoWeightOverflow
	}
	return left + right, nil
}
