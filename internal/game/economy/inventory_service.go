package economy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

const addItemOperation = "add_item"

// ErrItemInstanceAlreadyExists reports an explicit instance id collision.
var ErrItemInstanceAlreadyExists = errors.New("item instance already exists")

// AddItemInput describes one authoritative item grant/add mutation.
type AddItemInput struct {
	PlayerID       foundation.PlayerID       `json:"player_id"`
	ItemDefinition ItemDefinition            `json:"item_definition"`
	ItemInstanceID foundation.ItemID         `json:"item_instance_id,omitempty"`
	Quantity       int64                     `json:"quantity"`
	Location       ItemLocation              `json:"location"`
	Reason         LedgerReason              `json:"reason"`
	ReferenceKey   foundation.IdempotencyKey `json:"reference_id"`
}

// AddItemResult reports the item and ledger rows created by AddItem.
type AddItemResult struct {
	StackableItems []StackableItem `json:"stackable_items,omitempty"`
	InstanceItems  []InstanceItem  `json:"instance_items,omitempty"`
	LedgerEntry    ItemLedgerEntry `json:"ledger_entry"`
	Duplicate      bool            `json:"duplicate"`
}

// InventoryRepository is the durable persistence boundary for inventory items.
type InventoryRepository interface {
	LoadStackableItems(ctx context.Context) ([]StackableItem, error)
	LoadInstanceItems(ctx context.Context) ([]InstanceItem, error)
	UpsertStackableItem(ctx context.Context, item StackableItem) error
	UpsertInstanceItem(ctx context.Context, item InstanceItem) error
}

// InventoryService is an in-memory Phase 02 mutation service.
type InventoryService struct {
	mu    sync.Mutex
	clock foundation.Clock

	nextItemSequence   int64
	nextLedgerSequence int64

	stackableItems       []StackableItem
	instanceItems        []InstanceItem
	itemLedgerEntries    []ItemLedgerEntry
	addItemReferences    map[inventoryReferenceKey]AddItemResult
	moveItemReferences   map[inventoryReferenceKey]MoveItemResult
	removeItemReferences map[inventoryReferenceKey]RemoveItemResult

	repository InventoryRepository

	emitter           EventEmitter
	cargoGuard        CargoTransferGuard
	nextEventSequence uint64
}

type inventoryReferenceKey struct {
	playerID     foundation.PlayerID
	operation    string
	referenceKey foundation.IdempotencyKey
}

// NewInventoryService returns an in-memory inventory mutation service.
func NewInventoryService(clock foundation.Clock) *InventoryService {
	service, err := NewInventoryServiceWithRepository(clock, nil)
	if err != nil {
		panic(err)
	}
	return service
}

func NewInventoryServiceWithRepository(clock foundation.Clock, repository InventoryRepository) (*InventoryService, error) {
	if clock == nil {
		clock = foundation.RealClock{}
	}
	service := &InventoryService{
		clock:                clock,
		addItemReferences:    make(map[inventoryReferenceKey]AddItemResult),
		moveItemReferences:   make(map[inventoryReferenceKey]MoveItemResult),
		removeItemReferences: make(map[inventoryReferenceKey]RemoveItemResult),
		repository:           repository,
	}
	if repository != nil {
		stackables, err := repository.LoadStackableItems(context.Background())
		if err != nil {
			return nil, err
		}
		for _, item := range stackables {
			if err := item.Validate(); err != nil {
				return nil, err
			}
			service.stackableItems = append(service.stackableItems, item)
		}
		instances, err := repository.LoadInstanceItems(context.Background())
		if err != nil {
			return nil, err
		}
		for _, item := range instances {
			if err := item.Validate(); err != nil {
				return nil, err
			}
			service.instanceItems = append(service.instanceItems, item)
		}
	}
	return service, nil
}

// SetCargoTransferGuard configures an optional guard for player-facing cargo moves.
func (service *InventoryService) SetCargoTransferGuard(guard CargoTransferGuard) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.cargoGuard = guard
}

// AddItem grants item quantity once for a player/reference pair and writes an item ledger row.
func (service *InventoryService) AddItem(input AddItemInput) (AddItemResult, error) {
	quantity, err := input.validate()
	if err != nil {
		return AddItemResult{}, err
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()
	emitter = service.emitter

	now := service.clock.Now()
	result, err := service.addItemValidatedLocked(input, quantity, now)
	if err != nil {
		return AddItemResult{}, err
	}
	if emitter != nil && !result.Duplicate {
		emitted = service.addItemEventsLocked(input, result, now)
	}
	return result, nil
}

func (service *InventoryService) addItemValidatedLocked(input AddItemInput, quantity foundation.Quantity, now time.Time) (AddItemResult, error) {
	reference := inventoryReferenceKey{
		playerID:     input.PlayerID,
		operation:    addItemOperation,
		referenceKey: input.ReferenceKey,
	}
	if previous, ok := service.addItemReferences[reference]; ok {
		result := cloneAddItemResult(previous)
		result.Duplicate = true
		return result, nil
	}
	if !input.ItemInstanceID.IsZero() && service.itemInstanceExistsLocked(input.ItemInstanceID) {
		return AddItemResult{}, fmt.Errorf("item instance %q: %w", input.ItemInstanceID, ErrItemInstanceAlreadyExists)
	}

	stackableItems, instanceItems, err := service.buildAddedItems(input, quantity, now)
	if err != nil {
		return AddItemResult{}, err
	}

	balanceAfter := service.totalItemQuantityLocked(input.PlayerID, input.ItemDefinition.ItemID, input.Location) + quantity.Int64()
	ledgerEntry, err := NewItemLedgerEntry(
		service.nextLedgerID(),
		input.PlayerID,
		input.ItemDefinition.ItemID,
		singleAddedItemInstanceID(stackableItems, instanceItems),
		quantity,
		LedgerActionIncrease,
		balanceAfter,
		input.Location,
		input.Reason,
		input.ReferenceKey,
	)
	if err != nil {
		return AddItemResult{}, err
	}
	ledgerEntry.CreatedAt = now

	for _, item := range stackableItems {
		if err := service.persistStackableItemLocked(item); err != nil {
			return AddItemResult{}, err
		}
	}
	for _, item := range instanceItems {
		if err := service.persistInstanceItemLocked(item); err != nil {
			return AddItemResult{}, err
		}
	}
	service.stackableItems = append(service.stackableItems, stackableItems...)
	service.instanceItems = append(service.instanceItems, instanceItems...)
	service.itemLedgerEntries = append(service.itemLedgerEntries, ledgerEntry)

	result := AddItemResult{
		StackableItems: stackableItems,
		InstanceItems:  instanceItems,
		LedgerEntry:    ledgerEntry,
	}
	service.addItemReferences[reference] = cloneAddItemResult(result)
	return cloneAddItemResult(result), nil
}

// StackableItems returns a snapshot of in-memory stack rows.
func (service *InventoryService) StackableItems() []StackableItem {
	service.mu.Lock()
	defer service.mu.Unlock()

	return append([]StackableItem(nil), service.stackableItems...)
}

// InstanceItems returns a snapshot of in-memory instance rows.
func (service *InventoryService) InstanceItems() []InstanceItem {
	service.mu.Lock()
	defer service.mu.Unlock()

	return append([]InstanceItem(nil), service.instanceItems...)
}

// ItemLedgerEntries returns a snapshot of in-memory item ledger rows.
func (service *InventoryService) ItemLedgerEntries() []ItemLedgerEntry {
	service.mu.Lock()
	defer service.mu.Unlock()

	return append([]ItemLedgerEntry(nil), service.itemLedgerEntries...)
}

// TotalItemQuantity returns the current item quantity for a player/item/location tuple.
func (service *InventoryService) TotalItemQuantity(playerID foundation.PlayerID, itemID foundation.ItemID, location ItemLocation) int64 {
	service.mu.Lock()
	defer service.mu.Unlock()

	return service.totalItemQuantityLocked(playerID, itemID, location)
}

func (input AddItemInput) validate() (foundation.Quantity, error) {
	return input.validateWithCargoTargetPolicy(true)
}

func (input AddItemInput) validateCargoAdd() (foundation.Quantity, error) {
	return input.validateWithCargoTargetPolicy(false)
}

func (input AddItemInput) validateWithCargoTargetPolicy(blockCargoTarget bool) (foundation.Quantity, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.ItemDefinition.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	quantity, err := foundation.NewQuantity(input.Quantity)
	if err != nil {
		return foundation.Quantity{}, err
	}
	if !input.ItemInstanceID.IsZero() {
		if err := input.ItemInstanceID.Validate(); err != nil {
			return foundation.Quantity{}, err
		}
		if input.ItemDefinition.Type != ItemTypeInstance || quantity.Int64() != 1 {
			return foundation.Quantity{}, fmt.Errorf("explicit item instance id %q: %w", input.ItemInstanceID, ErrInvalidInstanceQuantity)
		}
	}
	if err := input.Location.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if blockCargoTarget {
		if err := validateAddItemTargetLocation(input.Location); err != nil {
			return foundation.Quantity{}, err
		}
	}
	if err := input.Reason.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.ReferenceKey.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	return quantity, nil
}

func (service *InventoryService) buildAddedItems(input AddItemInput, quantity foundation.Quantity, now time.Time) ([]StackableItem, []InstanceItem, error) {
	switch input.ItemDefinition.Type {
	case ItemTypeStackable:
		stackableItems, err := service.buildStackableAddItems(input, quantity, now)
		return stackableItems, nil, err
	case ItemTypeInstance:
		instanceItems, err := service.buildInstanceAddItems(input, quantity, now)
		return nil, instanceItems, err
	default:
		return nil, nil, input.ItemDefinition.Type.Validate()
	}
}

func (service *InventoryService) buildStackableAddItems(input AddItemInput, quantity foundation.Quantity, now time.Time) ([]StackableItem, error) {
	remaining := quantity.Int64()
	maxStack := input.ItemDefinition.MaxStack.Int64()
	items := make([]StackableItem, 0, (remaining+maxStack-1)/maxStack)

	for remaining > 0 {
		stackAmount := remaining
		if stackAmount > maxStack {
			stackAmount = maxStack
		}
		stackQuantity, err := foundation.NewQuantity(stackAmount)
		if err != nil {
			return nil, err
		}
		item, err := NewStackableItem(
			input.ItemDefinition.Source,
			service.nextItemInstanceID(input.ItemDefinition.ItemID, "stack"),
			input.ItemDefinition.ItemID,
			input.PlayerID,
			input.Location,
			stackQuantity,
		)
		if err != nil {
			return nil, err
		}
		item.CreatedAt = now
		item.UpdatedAt = now
		items = append(items, item)
		remaining -= stackAmount
	}

	return items, nil
}

func (service *InventoryService) buildInstanceAddItems(input AddItemInput, quantity foundation.Quantity, now time.Time) ([]InstanceItem, error) {
	items := make([]InstanceItem, 0, quantity.Int64())
	one, err := foundation.NewQuantity(1)
	if err != nil {
		return nil, err
	}

	for range quantity.Int64() {
		itemInstanceID := input.ItemInstanceID
		if input.ItemInstanceID.IsZero() {
			itemInstanceID = service.nextItemInstanceID(input.ItemDefinition.ItemID, "instance")
		}
		item, err := NewInstanceItem(
			input.ItemDefinition.Source,
			itemInstanceID,
			input.ItemDefinition.ItemID,
			input.PlayerID,
			input.Location,
			one,
		)
		if err != nil {
			return nil, err
		}
		item.CreatedAt = now
		item.UpdatedAt = now
		items = append(items, item)
	}

	return items, nil
}

func (service *InventoryService) persistStackableItemLocked(item StackableItem) error {
	if service.repository == nil {
		return nil
	}
	return service.repository.UpsertStackableItem(context.Background(), item)
}

func (service *InventoryService) persistInstanceItemLocked(item InstanceItem) error {
	if service.repository == nil {
		return nil
	}
	return service.repository.UpsertInstanceItem(context.Background(), item)
}

func (service *InventoryService) nextItemInstanceID(itemID foundation.ItemID, kind string) foundation.ItemID {
	for {
		service.nextItemSequence++
		candidate := foundation.ItemID(fmt.Sprintf("%s-%s-%d", itemID, kind, service.nextItemSequence))
		if !service.itemInstanceExistsLocked(candidate) {
			return candidate
		}
	}
}

func (service *InventoryService) nextLedgerID() LedgerID {
	service.nextLedgerSequence++
	return LedgerID(fmt.Sprintf("item-ledger-%d", service.nextLedgerSequence))
}

func (service *InventoryService) totalItemQuantityLocked(playerID foundation.PlayerID, itemID foundation.ItemID, location ItemLocation) int64 {
	var total int64
	for _, item := range service.stackableItems {
		if item.OwnerPlayerID == playerID && item.ItemID == itemID && item.Location == location {
			total += item.Quantity.Int64()
		}
	}
	for _, item := range service.instanceItems {
		if item.OwnerPlayerID == playerID && item.ItemID == itemID && item.Location == location {
			total += item.Quantity.Int64()
		}
	}
	return total
}

func (service *InventoryService) itemInstanceExistsLocked(itemInstanceID foundation.ItemID) bool {
	for _, item := range service.instanceItems {
		if item.ItemInstanceID == itemInstanceID {
			return true
		}
	}
	return false
}

func singleAddedItemInstanceID(stackableItems []StackableItem, instanceItems []InstanceItem) foundation.ItemID {
	if len(stackableItems)+len(instanceItems) != 1 {
		return ""
	}
	if len(stackableItems) == 1 {
		return stackableItems[0].ItemInstanceID
	}
	return instanceItems[0].ItemInstanceID
}

func cloneAddItemResult(result AddItemResult) AddItemResult {
	result.StackableItems = append([]StackableItem(nil), result.StackableItems...)
	result.InstanceItems = append([]InstanceItem(nil), result.InstanceItems...)
	return result
}
