package economy

import (
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

const removeItemOperation = "remove_item"

// ErrBlockedGenericRemoveSource reports a generic player remove from a reserved/system location.
var ErrBlockedGenericRemoveSource = errors.New("blocked generic remove source location")

// RemoveItemRef identifies the item being removed.
type RemoveItemRef struct {
	Definition     ItemDefinition    `json:"item_definition"`
	ItemInstanceID foundation.ItemID `json:"item_instance_id,omitempty"`
}

// RemoveItemInput describes one authoritative player item removal.
type RemoveItemInput struct {
	PlayerID       foundation.PlayerID       `json:"player_id"`
	ItemRef        RemoveItemRef             `json:"item_ref"`
	SourceLocation ItemLocation              `json:"source_location"`
	Quantity       int64                     `json:"quantity"`
	Reason         LedgerReason              `json:"reason"`
	ReferenceKey   foundation.IdempotencyKey `json:"reference_id"`
}

// RemoveItemResult reports item rows touched and ledger rows written by RemoveItem.
type RemoveItemResult struct {
	StackableItems        []StackableItem   `json:"stackable_items,omitempty"`
	DeletedStackableItems []StackableItem   `json:"deleted_stackable_items,omitempty"`
	InstanceItems         []InstanceItem    `json:"instance_items,omitempty"`
	LedgerEntries         []ItemLedgerEntry `json:"ledger_entries"`
	Duplicate             bool              `json:"duplicate"`
}

// RemoveItem removes player-owned item quantity from one source location once for a player/reference pair.
func (service *InventoryService) RemoveItem(input RemoveItemInput) (RemoveItemResult, error) {
	quantity, err := input.validate()
	if err != nil {
		return RemoveItemResult{}, err
	}

	return service.removeItemWithValidatedQuantity(input, quantity)
}

// SystemRemoveItem removes player-owned item quantity for trusted server-side
// repair and economy flows. It bypasses generic player-facing source location
// blocking while preserving validation, idempotency, ledger writes, and events.
func (service *InventoryService) SystemRemoveItem(input RemoveItemInput) (RemoveItemResult, error) {
	quantity, err := input.validateSystemRemove()
	if err != nil {
		return RemoveItemResult{}, err
	}

	return service.removeItemWithValidatedQuantity(input, quantity)
}

func (service *InventoryService) removeItemWithValidatedQuantity(input RemoveItemInput, quantity foundation.Quantity) (RemoveItemResult, error) {
	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()
	emitter = service.emitter

	reference := inventoryReferenceKey{
		playerID:     input.PlayerID,
		operation:    removeItemOperation,
		referenceKey: input.ReferenceKey,
	}
	if previous, ok := service.removeItemReferences[reference]; ok {
		result := cloneRemoveItemResult(previous)
		result.Duplicate = true
		return result, nil
	}

	now := service.clock.Now()
	var result RemoveItemResult
	var err error
	switch input.ItemRef.Definition.Type {
	case ItemTypeStackable:
		result, err = service.removeStackableItemLocked(input, quantity, now)
	case ItemTypeInstance:
		result, err = service.removeInstanceItemLocked(input, quantity, now)
	default:
		err = input.ItemRef.Definition.Type.Validate()
	}
	if err != nil {
		return RemoveItemResult{}, err
	}

	service.itemLedgerEntries = append(service.itemLedgerEntries, result.LedgerEntries...)
	service.removeItemReferences[reference] = cloneRemoveItemResult(result)
	if emitter != nil {
		emitted = service.removeItemEventsLocked(input, result, now)
	}
	return cloneRemoveItemResult(result), nil
}

func (input RemoveItemInput) validate() (foundation.Quantity, error) {
	return input.validateWithSourcePolicy(true)
}

func (input RemoveItemInput) validateSystemRemove() (foundation.Quantity, error) {
	return input.validateWithSourcePolicy(false)
}

func (input RemoveItemInput) validateWithSourcePolicy(validateSourcePolicy bool) (foundation.Quantity, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.ItemRef.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.SourceLocation.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if validateSourcePolicy {
		if err := validateGenericRemoveSourceLocation(input.SourceLocation); err != nil {
			return foundation.Quantity{}, err
		}
	}
	quantity, err := foundation.NewQuantity(input.Quantity)
	if err != nil {
		return foundation.Quantity{}, err
	}
	if input.ItemRef.Definition.Type == ItemTypeInstance && quantity.Int64() != 1 {
		return foundation.Quantity{}, fmt.Errorf("instance remove quantity %d: %w", quantity.Int64(), ErrInvalidInstanceQuantity)
	}
	if err := input.Reason.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.ReferenceKey.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	return quantity, nil
}

// Validate reports whether ref contains a valid item definition and required instance id.
func (ref RemoveItemRef) Validate() error {
	if err := ref.Definition.Validate(); err != nil {
		return err
	}
	if !ref.ItemInstanceID.IsZero() {
		if err := ref.ItemInstanceID.Validate(); err != nil {
			return err
		}
	}
	if ref.Definition.Type == ItemTypeInstance && ref.ItemInstanceID.IsZero() {
		return foundation.ErrEmptyID
	}
	return nil
}

func (service *InventoryService) removeStackableItemLocked(input RemoveItemInput, quantity foundation.Quantity, now time.Time) (RemoveItemResult, error) {
	sourceAvailable := service.stackableQuantityForDefinitionLocked(input.PlayerID, input.ItemRef.Definition, input.SourceLocation)
	if sourceAvailable <= 0 {
		return RemoveItemResult{}, ErrItemNotOwned
	}
	if sourceAvailable < quantity.Int64() {
		return RemoveItemResult{}, fmt.Errorf("have %d need %d: %w", sourceAvailable, quantity.Int64(), ErrInsufficientItemQuantity)
	}

	ledgerEntry, err := service.buildRemoveLedgerEntryLocked(input, quantity, "", now)
	if err != nil {
		return RemoveItemResult{}, err
	}

	stackableItems, deletedStackableItems, err := service.removedStackableRowsLocked(input, quantity, now)
	if err != nil {
		return RemoveItemResult{}, err
	}

	return RemoveItemResult{
		StackableItems:        stackableItems,
		DeletedStackableItems: deletedStackableItems,
		LedgerEntries:         []ItemLedgerEntry{ledgerEntry},
	}, nil
}

func (service *InventoryService) removeInstanceItemLocked(input RemoveItemInput, quantity foundation.Quantity, now time.Time) (RemoveItemResult, error) {
	index := service.removeInstanceItemIndexLocked(input)
	if index < 0 {
		return RemoveItemResult{}, ErrItemNotOwned
	}

	ledgerEntry, err := service.buildRemoveLedgerEntryLocked(input, quantity, input.ItemRef.ItemInstanceID, now)
	if err != nil {
		return RemoveItemResult{}, err
	}

	item := service.instanceItems[index]
	service.instanceItems = append(service.instanceItems[:index], service.instanceItems[index+1:]...)

	return RemoveItemResult{
		InstanceItems: []InstanceItem{item},
		LedgerEntries: []ItemLedgerEntry{ledgerEntry},
	}, nil
}

func (service *InventoryService) buildRemoveLedgerEntryLocked(
	input RemoveItemInput,
	quantity foundation.Quantity,
	itemInstanceID foundation.ItemID,
	now time.Time,
) (ItemLedgerEntry, error) {
	itemID := input.ItemRef.Definition.ItemID
	balanceAfter := service.totalItemQuantityLocked(input.PlayerID, itemID, input.SourceLocation) - quantity.Int64()
	entry, err := NewItemLedgerEntry(
		service.nextLedgerID(),
		input.PlayerID,
		itemID,
		itemInstanceID,
		quantity,
		LedgerActionDecrease,
		balanceAfter,
		input.SourceLocation,
		input.Reason,
		input.ReferenceKey,
	)
	if err != nil {
		return ItemLedgerEntry{}, err
	}
	entry.CreatedAt = now
	return entry, nil
}

func (service *InventoryService) removedStackableRowsLocked(
	input RemoveItemInput,
	quantity foundation.Quantity,
	now time.Time,
) ([]StackableItem, []StackableItem, error) {
	remainingSource := quantity.Int64()
	updatedItems := make([]StackableItem, 0, len(service.stackableItems))
	deletedItems := make([]StackableItem, 0)
	for _, item := range service.stackableItems {
		if remainingSource > 0 && matchesStackableDefinitionLocation(item, input.PlayerID, input.ItemRef.Definition, input.SourceLocation) {
			taken := item.Quantity.Int64()
			if taken > remainingSource {
				taken = remainingSource
			}
			remainingSource -= taken
			newAmount := item.Quantity.Int64() - taken
			if newAmount == 0 {
				deletedItems = append(deletedItems, item)
				continue
			}
			quantity, err := foundation.NewQuantity(newAmount)
			if err != nil {
				return nil, nil, err
			}
			item.Quantity = quantity
			item.UpdatedAt = now
		}
		updatedItems = append(updatedItems, item)
	}

	service.stackableItems = updatedItems
	return service.stackableItemsForRemoveLocked(input), deletedItems, nil
}

func (service *InventoryService) removeInstanceItemIndexLocked(input RemoveItemInput) int {
	for index, item := range service.instanceItems {
		if item.OwnerPlayerID == input.PlayerID &&
			item.ItemID == input.ItemRef.Definition.ItemID &&
			item.ItemInstanceID == input.ItemRef.ItemInstanceID &&
			item.Location == input.SourceLocation {
			return index
		}
	}
	return -1
}

func (service *InventoryService) stackableItemsForRemoveLocked(input RemoveItemInput) []StackableItem {
	items := make([]StackableItem, 0)
	for _, item := range service.stackableItems {
		if item.OwnerPlayerID == input.PlayerID &&
			item.ItemID == input.ItemRef.Definition.ItemID &&
			item.Location == input.SourceLocation {
			items = append(items, item)
		}
	}
	return items
}

func validateGenericRemoveSourceLocation(location ItemLocation) error {
	if isBlockedGenericMoveSourceLocation(location.Kind) {
		return fmt.Errorf("source location %q: %w", location.Kind, ErrBlockedGenericRemoveSource)
	}
	return nil
}

func cloneRemoveItemResult(result RemoveItemResult) RemoveItemResult {
	result.StackableItems = append([]StackableItem(nil), result.StackableItems...)
	result.DeletedStackableItems = append([]StackableItem(nil), result.DeletedStackableItems...)
	result.InstanceItems = append([]InstanceItem(nil), result.InstanceItems...)
	result.LedgerEntries = append([]ItemLedgerEntry(nil), result.LedgerEntries...)
	return result
}
