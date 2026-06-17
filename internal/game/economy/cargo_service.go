package economy

import (
	"errors"
	"fmt"
	"sync"

	"gameproject/internal/game/catalog"
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

// CargoService validates ship cargo capacity before delegating item mutation to InventoryService.
type CargoService struct {
	mu        sync.Mutex
	inventory *InventoryService

	definitions map[cargoDefinitionKey]ItemDefinition
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

// AddItem validates active cargo capacity, then uses InventoryService.AddItem for the mutation.
func (service *CargoService) AddItem(input CargoAddItemInput) (AddItemResult, error) {
	quantity, err := input.validate()
	if err != nil {
		return AddItemResult{}, err
	}
	if service == nil || service.inventory == nil {
		return AddItemResult{}, ErrNilInventoryService
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if result, ok := service.duplicateAddItemResultLocked(input); ok {
		return result, nil
	}

	incomingUnits, err := checkedCargoUnits(input.ItemDefinition.WeightUnits.Int64(), quantity.Int64())
	if err != nil {
		return AddItemResult{}, err
	}

	usedUnits, err := service.usedCargoUnitsLocked(input)
	if err != nil {
		return AddItemResult{}, err
	}
	if usedUnits > input.CargoCapacityUnits || incomingUnits > input.CargoCapacityUnits-usedUnits {
		return AddItemResult{}, fmt.Errorf(
			"used %d incoming %d capacity %d: %w",
			usedUnits,
			incomingUnits,
			input.CargoCapacityUnits,
			ErrCargoCapacityExceeded,
		)
	}

	result, err := service.inventory.AddItem(AddItemInput{
		PlayerID:       input.PlayerID,
		ItemDefinition: input.ItemDefinition,
		Quantity:       input.Quantity,
		Location:       input.ActiveCargo,
		Reason:         input.Reason,
		ReferenceKey:   input.ReferenceKey,
	})
	if err != nil {
		return AddItemResult{}, err
	}

	service.definitions[cargoKeyForDefinition(input.ItemDefinition)] = input.ItemDefinition
	return result, nil
}

func (service *CargoService) duplicateAddItemResultLocked(input CargoAddItemInput) (AddItemResult, bool) {
	service.inventory.mu.Lock()
	defer service.inventory.mu.Unlock()

	reference := inventoryReferenceKey{
		playerID:     input.PlayerID,
		operation:    addItemOperation,
		referenceKey: input.ReferenceKey,
	}
	previous, ok := service.inventory.addItemReferences[reference]
	if !ok {
		return AddItemResult{}, false
	}
	result := cloneAddItemResult(previous)
	result.Duplicate = true
	return result, true
}

func (input CargoAddItemInput) validate() (foundation.Quantity, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.ActiveCargo.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if input.ActiveCargo.Kind != LocationKindShipCargo {
		return foundation.Quantity{}, fmt.Errorf("location %q: %w", input.ActiveCargo.Kind, ErrInvalidCargoLocation)
	}
	if err := input.ItemDefinition.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	quantity, err := foundation.NewQuantity(input.Quantity)
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

func (service *CargoService) usedCargoUnitsLocked(input CargoAddItemInput) (int64, error) {
	var usedUnits int64

	for _, item := range service.inventory.StackableItems() {
		if item.OwnerPlayerID != input.PlayerID || item.Location != input.ActiveCargo {
			continue
		}
		itemUnits, err := service.itemCargoUnitsLocked(item.Source, item.ItemID, item.Quantity.Int64(), input.ItemDefinition)
		if err != nil {
			return 0, err
		}
		usedUnits, err = checkedCargoUnitSum(usedUnits, itemUnits)
		if err != nil {
			return 0, err
		}
	}

	for _, item := range service.inventory.InstanceItems() {
		if item.OwnerPlayerID != input.PlayerID || item.Location != input.ActiveCargo {
			continue
		}
		itemUnits, err := service.itemCargoUnitsLocked(item.Source, item.ItemID, item.Quantity.Int64(), input.ItemDefinition)
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
