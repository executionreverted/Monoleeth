package economy

import (
	"errors"
	"slices"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestMoveItemRejectsZeroAndNegativeQuantity(t *testing.T) {
	cases := []struct {
		name     string
		quantity int64
	}{
		{name: "zero", quantity: 0},
		{name: "negative", quantity: -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestInventoryService()
			input := validMoveItemInput(t)
			input.Quantity = tc.quantity

			_, err := service.MoveItem(input)
			if !errors.Is(err, foundation.ErrNonPositiveAmount) {
				t.Fatalf("MoveItem error = %v, want foundation.ErrNonPositiveAmount", err)
			}
			if got := len(service.ItemLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestMoveItemRejectsMissingRequiredInputs(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*MoveItemInput)
		wantErr error
	}{
		{
			name: "blank player",
			mutate: func(input *MoveItemInput) {
				input.PlayerID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "blank from location",
			mutate: func(input *MoveItemInput) {
				input.FromLocation = ItemLocation{}
			},
			wantErr: ErrInvalidLocationKind,
		},
		{
			name: "blank to location",
			mutate: func(input *MoveItemInput) {
				input.ToLocation = ItemLocation{}
			},
			wantErr: ErrInvalidLocationKind,
		},
		{
			name: "blank reason",
			mutate: func(input *MoveItemInput) {
				input.Reason = ""
			},
			wantErr: ErrEmptyLedgerReason,
		},
		{
			name: "blank reference",
			mutate: func(input *MoveItemInput) {
				input.ReferenceKey = ""
			},
			wantErr: foundation.ErrEmptyIdempotencyKey,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestInventoryService()
			input := validMoveItemInput(t)
			tc.mutate(&input)

			_, err := service.MoveItem(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("MoveItem error = %v, want %v", err, tc.wantErr)
			}
			if got := len(service.ItemLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestMoveItemMovesStackableQuantityAndWritesLedgerEntries(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	toLocation := validStationStorageLocation(t)
	addStackableItems(t, service, definition, 75, fromLocation, "loot_pickup:drop-1")

	input := validMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.ToLocation = toLocation
	input.Quantity = 30
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:move-1")

	result, err := service.MoveItem(input)
	if err != nil {
		t.Fatalf("MoveItem: %v", err)
	}

	if result.Duplicate {
		t.Fatal("MoveItem Duplicate = true, want false")
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 45 {
		t.Fatalf("source TotalItemQuantity() = %d, want 45", got)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, toLocation); got != 30 {
		t.Fatalf("destination TotalItemQuantity() = %d, want 30", got)
	}
	if got := len(result.LedgerEntries); got != 2 {
		t.Fatalf("result ledger entries len = %d, want 2", got)
	}

	entries := service.ItemLedgerEntries()
	if len(entries) != 3 {
		t.Fatalf("ledger entries len = %d, want 3", len(entries))
	}
	sourceEntry := entries[1]
	if sourceEntry.Action != LedgerActionDecrease {
		t.Fatalf("source ledger action = %q, want %q", sourceEntry.Action, LedgerActionDecrease)
	}
	if got := sourceEntry.Quantity.Int64(); got != 30 {
		t.Fatalf("source ledger quantity = %d, want 30", got)
	}
	if sourceEntry.BalanceAfter != 45 {
		t.Fatalf("source ledger balance after = %d, want 45", sourceEntry.BalanceAfter)
	}
	if sourceEntry.Location != fromLocation {
		t.Fatalf("source ledger location = %v, want %v", sourceEntry.Location, fromLocation)
	}

	destinationEntry := entries[2]
	if destinationEntry.Action != LedgerActionIncrease {
		t.Fatalf("destination ledger action = %q, want %q", destinationEntry.Action, LedgerActionIncrease)
	}
	if got := destinationEntry.Quantity.Int64(); got != 30 {
		t.Fatalf("destination ledger quantity = %d, want 30", got)
	}
	if destinationEntry.BalanceAfter != 30 {
		t.Fatalf("destination ledger balance after = %d, want 30", destinationEntry.BalanceAfter)
	}
	if destinationEntry.Location != toLocation {
		t.Fatalf("destination ledger location = %v, want %v", destinationEntry.Location, toLocation)
	}
	for _, entry := range []ItemLedgerEntry{sourceEntry, destinationEntry} {
		if entry.Reason != input.Reason {
			t.Fatalf("ledger reason = %q, want %q", entry.Reason, input.Reason)
		}
		if entry.ReferenceKey != input.ReferenceKey {
			t.Fatalf("ledger reference = %q, want %q", entry.ReferenceKey, input.ReferenceKey)
		}
		if entry.CreatedAt != testInventoryNow {
			t.Fatalf("ledger created at = %s, want %s", entry.CreatedAt, testInventoryNow)
		}
	}
}

func TestMoveItemDuplicateReferenceDoesNotMoveOrLedgerTwice(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	toLocation := validStationStorageLocation(t)
	addStackableItems(t, service, definition, 75, fromLocation, "loot_pickup:drop-1")

	input := validMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.ToLocation = toLocation
	input.Quantity = 30
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:move-1")

	first, err := service.MoveItem(input)
	if err != nil {
		t.Fatalf("first MoveItem: %v", err)
	}
	second, err := service.MoveItem(input)
	if err != nil {
		t.Fatalf("duplicate MoveItem: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first MoveItem Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate MoveItem Duplicate = false, want true")
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 45 {
		t.Fatalf("source TotalItemQuantity() = %d, want 45", got)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, toLocation); got != 30 {
		t.Fatalf("destination TotalItemQuantity() = %d, want 30", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 3 {
		t.Fatalf("ledger entries len = %d, want 3", got)
	}
	if len(second.LedgerEntries) != 2 {
		t.Fatalf("duplicate result ledger entries len = %d, want 2", len(second.LedgerEntries))
	}
	if second.LedgerEntries[0].LedgerID != first.LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate source LedgerID = %q, want %q", second.LedgerEntries[0].LedgerID, first.LedgerEntries[0].LedgerID)
	}
	if second.LedgerEntries[1].LedgerID != first.LedgerEntries[1].LedgerID {
		t.Fatalf("duplicate destination LedgerID = %q, want %q", second.LedgerEntries[1].LedgerID, first.LedgerEntries[1].LedgerID)
	}
}

func TestMoveItemStackMergeRespectsMaxStack(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	toLocation := validStationStorageLocation(t)
	addStackableItems(t, service, definition, 80, fromLocation, "loot_pickup:drop-1")
	addStackableItems(t, service, definition, 90, toLocation, "loot_pickup:drop-2")

	input := validMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.ToLocation = toLocation
	input.Quantity = 20
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:move-1")

	if _, err := service.MoveItem(input); err != nil {
		t.Fatalf("MoveItem: %v", err)
	}

	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 60 {
		t.Fatalf("source TotalItemQuantity() = %d, want 60", got)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, toLocation); got != 110 {
		t.Fatalf("destination TotalItemQuantity() = %d, want 110", got)
	}

	var destinationQuantities []int64
	for _, item := range service.StackableItems() {
		if item.OwnerPlayerID == input.PlayerID && item.ItemID == definition.ItemID && item.Location == toLocation {
			destinationQuantities = append(destinationQuantities, item.Quantity.Int64())
			if item.Quantity.Int64() > definition.MaxStack.Int64() {
				t.Fatalf("destination stack quantity = %d, max stack = %d", item.Quantity.Int64(), definition.MaxStack.Int64())
			}
		}
	}
	slices.Sort(destinationQuantities)
	want := []int64{10, 100}
	if !slices.Equal(destinationQuantities, want) {
		t.Fatalf("destination stack quantities = %v, want %v", destinationQuantities, want)
	}
}

func TestMoveItemInstanceQuantityAboveOneRejectedAndQuantityOneMovesInstance(t *testing.T) {
	service := newTestInventoryService()
	definition := validInstanceDefinition(t)
	fromLocation := validLocation(t)
	toLocation := validStationStorageLocation(t)
	addResult := addInstanceItems(t, service, definition, 1, fromLocation, "loot_pickup:drop-1")
	instanceID := addResult.InstanceItems[0].ItemInstanceID

	invalidInput := validMoveItemInput(t)
	invalidInput.ItemRef = MoveItemRef{
		Definition:     definition,
		ItemInstanceID: instanceID,
	}
	invalidInput.FromLocation = fromLocation
	invalidInput.ToLocation = toLocation
	invalidInput.Quantity = 2
	invalidInput.ReferenceKey = validReferenceKey(t, "loot_pickup:move-1")

	if _, err := service.MoveItem(invalidInput); !errors.Is(err, ErrInvalidInstanceQuantity) {
		t.Fatalf("MoveItem quantity above one error = %v, want ErrInvalidInstanceQuantity", err)
	}
	if got := len(service.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries after rejected move len = %d, want 1", got)
	}

	validInput := invalidInput
	validInput.Quantity = 1
	validInput.ReferenceKey = validReferenceKey(t, "loot_pickup:move-2")
	result, err := service.MoveItem(validInput)
	if err != nil {
		t.Fatalf("MoveItem quantity one: %v", err)
	}

	if got := service.TotalItemQuantity(validInput.PlayerID, definition.ItemID, fromLocation); got != 0 {
		t.Fatalf("source TotalItemQuantity() = %d, want 0", got)
	}
	if got := service.TotalItemQuantity(validInput.PlayerID, definition.ItemID, toLocation); got != 1 {
		t.Fatalf("destination TotalItemQuantity() = %d, want 1", got)
	}
	items := service.InstanceItems()
	if len(items) != 1 {
		t.Fatalf("instance items len = %d, want 1", len(items))
	}
	if items[0].ItemInstanceID != instanceID {
		t.Fatalf("moved instance id = %q, want %q", items[0].ItemInstanceID, instanceID)
	}
	if items[0].Location != toLocation {
		t.Fatalf("moved instance location = %v, want %v", items[0].Location, toLocation)
	}
	if len(result.LedgerEntries) != 2 {
		t.Fatalf("result ledger entries len = %d, want 2", len(result.LedgerEntries))
	}
	for _, entry := range result.LedgerEntries {
		if entry.ItemInstanceID != instanceID {
			t.Fatalf("ledger item instance id = %q, want %q", entry.ItemInstanceID, instanceID)
		}
	}
}
