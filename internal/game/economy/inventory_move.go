package economy

import (
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

const moveItemOperation = "move_item"

var (
	ErrItemNotOwned                = errors.New("item not owned by player at location")
	ErrInsufficientItemQuantity    = errors.New("insufficient item quantity")
	ErrBlockedGenericMoveSource    = errors.New("blocked generic move source location")
	ErrBlockedGenericMoveTarget    = errors.New("blocked generic move target location")
	ErrBlockedGenericOwnerTransfer = errors.New("blocked generic owner transfer")
	ErrMoveItemSameSourceAndTarget = errors.New("move item source and target are the same")
)

// MoveItemRef identifies the item being moved.
type MoveItemRef struct {
	Definition     ItemDefinition    `json:"item_definition"`
	ItemInstanceID foundation.ItemID `json:"item_instance_id,omitempty"`
}

// MoveItemInput describes one authoritative player item movement.
type MoveItemInput struct {
	PlayerID     foundation.PlayerID       `json:"player_id"`
	ToPlayerID   foundation.PlayerID       `json:"to_player_id,omitempty"`
	ItemRef      MoveItemRef               `json:"item_ref"`
	FromLocation ItemLocation              `json:"from_location"`
	ToLocation   ItemLocation              `json:"to_location"`
	Quantity     int64                     `json:"quantity"`
	Reason       LedgerReason              `json:"reason"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
}

// MoveItemResult reports item rows touched and ledger rows written by MoveItem.
type MoveItemResult struct {
	StackableItems []StackableItem   `json:"stackable_items,omitempty"`
	InstanceItems  []InstanceItem    `json:"instance_items,omitempty"`
	LedgerEntries  []ItemLedgerEntry `json:"ledger_entries"`
	Duplicate      bool              `json:"duplicate"`
}

// MoveItem moves player-owned item quantity between inventory locations once for a player/reference pair.
func (service *InventoryService) MoveItem(input MoveItemInput) (MoveItemResult, error) {
	quantity, err := input.validate()
	if err != nil {
		return MoveItemResult{}, err
	}

	return service.moveItemWithValidatedQuantity(input, quantity)
}

// SystemMoveItem moves player-owned items for trusted server-side economy flows.
// It bypasses generic player-facing source/target location blocking while
// preserving all other validation, idempotency, ledger writes, and events.
func (service *InventoryService) SystemMoveItem(input MoveItemInput) (MoveItemResult, error) {
	quantity, err := input.validateSystemMove()
	if err != nil {
		return MoveItemResult{}, err
	}

	return service.moveItemWithValidatedQuantity(input, quantity)
}

// SystemMoveItems moves several player-owned items as one in-memory
// transaction for trusted server-side economy flows.
func (service *InventoryService) SystemMoveItems(inputs []MoveItemInput) ([]MoveItemResult, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	quantities := make([]foundation.Quantity, len(inputs))
	for index, input := range inputs {
		quantity, err := input.validateSystemMove()
		if err != nil {
			return nil, err
		}
		quantities[index] = quantity
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()
	emitter = service.emitter

	if err := service.preflightMoveItemsLocked(inputs, quantities); err != nil {
		return nil, err
	}

	now := service.clock.Now()
	results := make([]MoveItemResult, 0, len(inputs))
	for index, input := range inputs {
		result, err := service.moveItemValidatedLocked(input, quantities[index], now)
		if err != nil {
			return nil, err
		}
		if emitter != nil && !result.Duplicate {
			emitted = append(emitted, service.moveItemEventsLocked(input, result, now)...)
		}
		results = append(results, result)
	}
	return cloneMoveItemResults(results), nil
}

func (service *InventoryService) moveItemWithValidatedQuantity(input MoveItemInput, quantity foundation.Quantity) (MoveItemResult, error) {
	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()
	emitter = service.emitter

	now := service.clock.Now()
	result, err := service.moveItemValidatedLocked(input, quantity, now)
	if err != nil {
		return MoveItemResult{}, err
	}
	if emitter != nil && !result.Duplicate {
		emitted = service.moveItemEventsLocked(input, result, now)
	}
	return result, nil
}

func (service *InventoryService) moveItemValidatedLocked(input MoveItemInput, quantity foundation.Quantity, now time.Time) (MoveItemResult, error) {
	reference := inventoryReferenceKey{
		playerID:     input.PlayerID,
		operation:    moveItemOperation,
		referenceKey: input.ReferenceKey,
	}
	if previous, ok := service.moveItemReferences[reference]; ok {
		result := cloneMoveItemResult(previous)
		result.Duplicate = true
		return result, nil
	}

	var result MoveItemResult
	var err error
	switch input.ItemRef.Definition.Type {
	case ItemTypeStackable:
		result, err = service.moveStackableItemLocked(input, quantity, now)
	case ItemTypeInstance:
		result, err = service.moveInstanceItemLocked(input, quantity, now)
	default:
		err = input.ItemRef.Definition.Type.Validate()
	}
	if err != nil {
		return MoveItemResult{}, err
	}

	service.itemLedgerEntries = append(service.itemLedgerEntries, result.LedgerEntries...)
	service.moveItemReferences[reference] = cloneMoveItemResult(result)
	return cloneMoveItemResult(result), nil
}

type stackableMovePreflightKey struct {
	playerID foundation.PlayerID
	itemID   foundation.ItemID
	source   catalogSourceKey
	location ItemLocation
}

type catalogSourceKey struct {
	definitionID string
	version      string
}

func (service *InventoryService) preflightMoveItemsLocked(inputs []MoveItemInput, quantities []foundation.Quantity) error {
	reservedStackable := make(map[stackableMovePreflightKey]int64)
	reservedInstances := make(map[foundation.ItemID]struct{})
	pendingReferences := make(map[inventoryReferenceKey]struct{}, len(inputs))

	for index, input := range inputs {
		reference := inventoryReferenceKey{
			playerID:     input.PlayerID,
			operation:    moveItemOperation,
			referenceKey: input.ReferenceKey,
		}
		if _, ok := service.moveItemReferences[reference]; ok {
			continue
		}
		if _, ok := pendingReferences[reference]; ok {
			return fmt.Errorf("move reference %q: %w", input.ReferenceKey, foundation.ErrInvalidIdempotencyKey)
		}
		pendingReferences[reference] = struct{}{}

		quantity := quantities[index]
		switch input.ItemRef.Definition.Type {
		case ItemTypeStackable:
			key := stackableMovePreflightKey{
				playerID: input.PlayerID,
				itemID:   input.ItemRef.Definition.ItemID,
				source: catalogSourceKey{
					definitionID: input.ItemRef.Definition.Source.DefinitionID.String(),
					version:      input.ItemRef.Definition.Source.Version.String(),
				},
				location: input.FromLocation,
			}
			available := service.stackableQuantityForDefinitionLocked(input.PlayerID, input.ItemRef.Definition, input.FromLocation) - reservedStackable[key]
			if available <= 0 {
				return ErrItemNotOwned
			}
			if available < quantity.Int64() {
				return fmt.Errorf("have %d need %d: %w", available, quantity.Int64(), ErrInsufficientItemQuantity)
			}
			reservedStackable[key] += quantity.Int64()
		case ItemTypeInstance:
			if _, ok := reservedInstances[input.ItemRef.ItemInstanceID]; ok {
				return fmt.Errorf("item %q: %w", input.ItemRef.ItemInstanceID, ErrItemNotOwned)
			}
			if service.instanceItemIndexLocked(input) < 0 {
				return ErrItemNotOwned
			}
			reservedInstances[input.ItemRef.ItemInstanceID] = struct{}{}
		default:
			return input.ItemRef.Definition.Type.Validate()
		}
	}
	return nil
}

func (input MoveItemInput) validate() (foundation.Quantity, error) {
	return input.validateWithLocationPolicies(true, true)
}

func (input MoveItemInput) validateSystemMove() (foundation.Quantity, error) {
	return input.validateWithLocationPolicies(false, false)
}

func (input MoveItemInput) validateCargoMove() (foundation.Quantity, error) {
	return input.validateGenericSourceMove()
}

func (input MoveItemInput) validateGenericSourceMove() (foundation.Quantity, error) {
	return input.validateWithLocationPolicies(true, false)
}

func (input MoveItemInput) validateWithLocationPolicies(validateSourcePolicy bool, validateTargetPolicy bool) (foundation.Quantity, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if !input.ToPlayerID.IsZero() {
		if validateSourcePolicy || validateTargetPolicy {
			return foundation.Quantity{}, ErrBlockedGenericOwnerTransfer
		}
		if err := input.ToPlayerID.Validate(); err != nil {
			return foundation.Quantity{}, err
		}
	}
	if err := input.ItemRef.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.FromLocation.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if err := input.ToLocation.Validate(); err != nil {
		return foundation.Quantity{}, err
	}
	if input.FromLocation == input.ToLocation {
		return foundation.Quantity{}, ErrMoveItemSameSourceAndTarget
	}
	if validateSourcePolicy {
		if err := validateGenericMoveSourceLocation(input.FromLocation); err != nil {
			return foundation.Quantity{}, err
		}
	}
	if validateTargetPolicy {
		if err := validateGenericMoveTargetLocation(input.ToLocation); err != nil {
			return foundation.Quantity{}, err
		}
	}
	quantity, err := foundation.NewQuantity(input.Quantity)
	if err != nil {
		return foundation.Quantity{}, err
	}
	if input.ItemRef.Definition.Type == ItemTypeInstance && quantity.Int64() != 1 {
		return foundation.Quantity{}, fmt.Errorf("instance move quantity %d: %w", quantity.Int64(), ErrInvalidInstanceQuantity)
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
func (ref MoveItemRef) Validate() error {
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

func (service *InventoryService) moveStackableItemLocked(input MoveItemInput, quantity foundation.Quantity, now time.Time) (MoveItemResult, error) {
	sourceAvailable := service.stackableQuantityForDefinitionLocked(input.PlayerID, input.ItemRef.Definition, input.FromLocation)
	if sourceAvailable <= 0 {
		return MoveItemResult{}, ErrItemNotOwned
	}
	if sourceAvailable < quantity.Int64() {
		return MoveItemResult{}, fmt.Errorf("have %d need %d: %w", sourceAvailable, quantity.Int64(), ErrInsufficientItemQuantity)
	}

	newDestinationItems, err := service.buildStackableMoveDestinationItemsLocked(input, quantity, now)
	if err != nil {
		return MoveItemResult{}, err
	}
	ledgerEntries, err := service.buildMoveLedgerEntriesLocked(input, quantity, "", now)
	if err != nil {
		return MoveItemResult{}, err
	}

	stackableItems, err := service.movedStackableRowsLocked(input, quantity, now, newDestinationItems)
	if err != nil {
		return MoveItemResult{}, err
	}

	return MoveItemResult{
		StackableItems: stackableItems,
		LedgerEntries:  ledgerEntries,
	}, nil
}

func (service *InventoryService) moveInstanceItemLocked(input MoveItemInput, quantity foundation.Quantity, now time.Time) (MoveItemResult, error) {
	index := service.instanceItemIndexLocked(input)
	if index < 0 {
		return MoveItemResult{}, ErrItemNotOwned
	}

	ledgerEntries, err := service.buildMoveLedgerEntriesLocked(input, quantity, input.ItemRef.ItemInstanceID, now)
	if err != nil {
		return MoveItemResult{}, err
	}

	item := service.instanceItems[index]
	item.OwnerPlayerID = input.destinationPlayerID()
	item.Location = input.ToLocation
	item.UpdatedAt = now
	service.instanceItems[index] = item

	return MoveItemResult{
		InstanceItems: []InstanceItem{item},
		LedgerEntries: ledgerEntries,
	}, nil
}

func (service *InventoryService) buildMoveLedgerEntriesLocked(
	input MoveItemInput,
	quantity foundation.Quantity,
	itemInstanceID foundation.ItemID,
	now time.Time,
) ([]ItemLedgerEntry, error) {
	itemID := input.ItemRef.Definition.ItemID
	destinationPlayerID := input.destinationPlayerID()
	sourceBalanceAfter := service.totalItemQuantityLocked(input.PlayerID, itemID, input.FromLocation) - quantity.Int64()
	destinationBalanceAfter := service.totalItemQuantityLocked(destinationPlayerID, itemID, input.ToLocation) + quantity.Int64()

	sourceEntry, err := NewItemLedgerEntry(
		service.nextLedgerID(),
		input.PlayerID,
		itemID,
		itemInstanceID,
		quantity,
		LedgerActionDecrease,
		sourceBalanceAfter,
		input.FromLocation,
		input.Reason,
		input.ReferenceKey,
	)
	if err != nil {
		return nil, err
	}
	sourceEntry.CreatedAt = now

	destinationEntry, err := NewItemLedgerEntry(
		service.nextLedgerID(),
		destinationPlayerID,
		itemID,
		itemInstanceID,
		quantity,
		LedgerActionIncrease,
		destinationBalanceAfter,
		input.ToLocation,
		input.Reason,
		input.ReferenceKey,
	)
	if err != nil {
		return nil, err
	}
	destinationEntry.CreatedAt = now

	return []ItemLedgerEntry{sourceEntry, destinationEntry}, nil
}

func (service *InventoryService) buildStackableMoveDestinationItemsLocked(
	input MoveItemInput,
	quantity foundation.Quantity,
	now time.Time,
) ([]StackableItem, error) {
	remaining := quantity.Int64()
	maxStack := input.ItemRef.Definition.MaxStack.Int64()
	destinationPlayerID := input.destinationPlayerID()
	for _, item := range service.stackableItems {
		if !matchesStackableDefinitionLocation(item, destinationPlayerID, input.ItemRef.Definition, input.ToLocation) {
			continue
		}
		available := maxStack - item.Quantity.Int64()
		if available <= 0 {
			continue
		}
		if remaining <= available {
			return nil, nil
		}
		remaining -= available
	}

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
			input.ItemRef.Definition.Source,
			service.nextItemInstanceID(input.ItemRef.Definition.ItemID, "stack"),
			input.ItemRef.Definition.ItemID,
			destinationPlayerID,
			input.ToLocation,
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

func (service *InventoryService) movedStackableRowsLocked(
	input MoveItemInput,
	quantity foundation.Quantity,
	now time.Time,
	newDestinationItems []StackableItem,
) ([]StackableItem, error) {
	remainingSource := quantity.Int64()
	updatedItems := make([]StackableItem, 0, len(service.stackableItems)+len(newDestinationItems))
	for _, item := range service.stackableItems {
		if remainingSource > 0 && matchesStackableDefinitionLocation(item, input.PlayerID, input.ItemRef.Definition, input.FromLocation) {
			taken := item.Quantity.Int64()
			if taken > remainingSource {
				taken = remainingSource
			}
			remainingSource -= taken
			newAmount := item.Quantity.Int64() - taken
			if newAmount == 0 {
				continue
			}
			quantity, err := foundation.NewQuantity(newAmount)
			if err != nil {
				return nil, err
			}
			item.Quantity = quantity
			item.UpdatedAt = now
		}
		updatedItems = append(updatedItems, item)
	}

	remainingDestination := quantity.Int64()
	maxStack := input.ItemRef.Definition.MaxStack.Int64()
	destinationPlayerID := input.destinationPlayerID()
	for index := range updatedItems {
		if remainingDestination == 0 {
			break
		}
		if !matchesStackableDefinitionLocation(updatedItems[index], destinationPlayerID, input.ItemRef.Definition, input.ToLocation) {
			continue
		}
		available := maxStack - updatedItems[index].Quantity.Int64()
		if available <= 0 {
			continue
		}
		added := available
		if added > remainingDestination {
			added = remainingDestination
		}
		quantity, err := foundation.NewQuantity(updatedItems[index].Quantity.Int64() + added)
		if err != nil {
			return nil, err
		}
		updatedItems[index].Quantity = quantity
		updatedItems[index].UpdatedAt = now
		remainingDestination -= added
	}
	updatedItems = append(updatedItems, newDestinationItems...)

	service.stackableItems = updatedItems
	return service.stackableItemsForMoveLocked(input), nil
}

func (service *InventoryService) instanceItemIndexLocked(input MoveItemInput) int {
	for index, item := range service.instanceItems {
		if item.OwnerPlayerID == input.PlayerID &&
			item.ItemID == input.ItemRef.Definition.ItemID &&
			item.ItemInstanceID == input.ItemRef.ItemInstanceID &&
			item.Location == input.FromLocation {
			return index
		}
	}
	return -1
}

func (service *InventoryService) stackableQuantityForDefinitionLocked(
	playerID foundation.PlayerID,
	definition ItemDefinition,
	location ItemLocation,
) int64 {
	var total int64
	for _, item := range service.stackableItems {
		if matchesStackableDefinitionLocation(item, playerID, definition, location) {
			total += item.Quantity.Int64()
		}
	}
	return total
}

func (service *InventoryService) stackableItemsForMoveLocked(input MoveItemInput) []StackableItem {
	items := make([]StackableItem, 0)
	destinationPlayerID := input.destinationPlayerID()
	for _, item := range service.stackableItems {
		if item.ItemID != input.ItemRef.Definition.ItemID {
			continue
		}
		if (item.OwnerPlayerID == input.PlayerID && item.Location == input.FromLocation) ||
			(item.OwnerPlayerID == destinationPlayerID && item.Location == input.ToLocation) {
			items = append(items, item)
		}
	}
	return items
}

func (input MoveItemInput) destinationPlayerID() foundation.PlayerID {
	if input.ToPlayerID.IsZero() {
		return input.PlayerID
	}
	return input.ToPlayerID
}

func matchesStackableDefinitionLocation(
	item StackableItem,
	playerID foundation.PlayerID,
	definition ItemDefinition,
	location ItemLocation,
) bool {
	return item.OwnerPlayerID == playerID &&
		item.ItemID == definition.ItemID &&
		item.Source == definition.Source &&
		item.Location == location
}

func validateGenericMoveSourceLocation(location ItemLocation) error {
	if isBlockedGenericMoveSourceLocation(location.Kind) {
		return fmt.Errorf("source location %q: %w", location.Kind, ErrBlockedGenericMoveSource)
	}
	return nil
}

func validateGenericMoveTargetLocation(location ItemLocation) error {
	if location.Kind == LocationKindShipCargo || isBlockedGenericMoveSourceLocation(location.Kind) {
		return fmt.Errorf("target location %q: %w", location.Kind, ErrBlockedGenericMoveTarget)
	}
	return nil
}

func validateAddItemTargetLocation(location ItemLocation) error {
	if location.Kind == LocationKindShipCargo || location.Kind == LocationKindShipEquipped {
		return fmt.Errorf("target location %q: %w", location.Kind, ErrBlockedGenericMoveTarget)
	}
	return nil
}

func isBlockedGenericMoveSourceLocation(kind LocationKind) bool {
	switch kind {
	case LocationKindShipEquipped,
		LocationKindMarketEscrow,
		LocationKindAuctionEscrow,
		LocationKindCraftingReserved,
		LocationKindSystemSink:
		return true
	default:
		return false
	}
}

func cloneMoveItemResult(result MoveItemResult) MoveItemResult {
	result.StackableItems = append([]StackableItem(nil), result.StackableItems...)
	result.InstanceItems = append([]InstanceItem(nil), result.InstanceItems...)
	result.LedgerEntries = append([]ItemLedgerEntry(nil), result.LedgerEntries...)
	return result
}

func cloneMoveItemResults(results []MoveItemResult) []MoveItemResult {
	if len(results) == 0 {
		return nil
	}
	cloned := make([]MoveItemResult, len(results))
	for index, result := range results {
		cloned[index] = cloneMoveItemResult(result)
	}
	return cloned
}
