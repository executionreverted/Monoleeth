package economy

import (
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestRemoveItemRejectsZeroAndNegativeQuantity(t *testing.T) {
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
			input := validRemoveItemInput(t)
			input.Quantity = tc.quantity

			_, err := service.RemoveItem(input)
			if !errors.Is(err, foundation.ErrNonPositiveAmount) {
				t.Fatalf("RemoveItem error = %v, want foundation.ErrNonPositiveAmount", err)
			}
			if got := len(service.ItemLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestRemoveItemRejectsMissingRequiredInputs(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*RemoveItemInput)
		wantErr error
	}{
		{
			name: "blank player",
			mutate: func(input *RemoveItemInput) {
				input.PlayerID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "blank item",
			mutate: func(input *RemoveItemInput) {
				input.ItemRef.Definition.ItemID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "blank source location",
			mutate: func(input *RemoveItemInput) {
				input.SourceLocation = ItemLocation{}
			},
			wantErr: ErrInvalidLocationKind,
		},
		{
			name: "blank reason",
			mutate: func(input *RemoveItemInput) {
				input.Reason = ""
			},
			wantErr: ErrEmptyLedgerReason,
		},
		{
			name: "blank reference",
			mutate: func(input *RemoveItemInput) {
				input.ReferenceKey = ""
			},
			wantErr: foundation.ErrEmptyIdempotencyKey,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestInventoryService()
			input := validRemoveItemInput(t)
			tc.mutate(&input)

			_, err := service.RemoveItem(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("RemoveItem error = %v, want %v", err, tc.wantErr)
			}
			if got := len(service.ItemLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestRemoveItemDecreasesStackableQuantityAndWritesLedgerEntry(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	sourceLocation := validLocation(t)
	otherLocation := validStationStorageLocation(t)
	addStackableItems(t, service, definition, 75, sourceLocation, "loot_pickup:drop-1")
	addStackableItems(t, service, definition, 20, otherLocation, "loot_pickup:drop-2")

	input := validRemoveItemInput(t)
	input.ItemRef.Definition = definition
	input.SourceLocation = sourceLocation
	input.Quantity = 30
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:remove-1")

	result, err := service.RemoveItem(input)
	if err != nil {
		t.Fatalf("RemoveItem: %v", err)
	}

	if result.Duplicate {
		t.Fatal("RemoveItem Duplicate = true, want false")
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, sourceLocation); got != 45 {
		t.Fatalf("source TotalItemQuantity() = %d, want 45", got)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, otherLocation); got != 20 {
		t.Fatalf("other location TotalItemQuantity() = %d, want 20", got)
	}
	if got := len(result.LedgerEntries); got != 1 {
		t.Fatalf("result ledger entries len = %d, want 1", got)
	}

	entries := service.ItemLedgerEntries()
	if len(entries) != 3 {
		t.Fatalf("ledger entries len = %d, want 3", len(entries))
	}
	entry := entries[2]
	if entry.Action != LedgerActionDecrease {
		t.Fatalf("ledger action = %q, want %q", entry.Action, LedgerActionDecrease)
	}
	if got := entry.Quantity.Int64(); got != 30 {
		t.Fatalf("ledger quantity = %d, want 30", got)
	}
	if entry.BalanceAfter != 45 {
		t.Fatalf("ledger balance after = %d, want 45", entry.BalanceAfter)
	}
	if entry.Location != sourceLocation {
		t.Fatalf("ledger location = %v, want %v", entry.Location, sourceLocation)
	}
	if entry.Reason != input.Reason {
		t.Fatalf("ledger reason = %q, want %q", entry.Reason, input.Reason)
	}
	if entry.ReferenceKey != input.ReferenceKey {
		t.Fatalf("ledger reference = %q, want %q", entry.ReferenceKey, input.ReferenceKey)
	}
	if entry.CreatedAt != testInventoryNow {
		t.Fatalf("ledger created at = %s, want %s", entry.CreatedAt, testInventoryNow)
	}
	if result.LedgerEntries[0] != entry {
		t.Fatalf("result ledger entry = %#v, want %#v", result.LedgerEntries[0], entry)
	}
}

func TestRemoveItemDuplicateReferenceDoesNotRemoveOrLedgerTwice(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	sourceLocation := validLocation(t)
	addStackableItems(t, service, definition, 75, sourceLocation, "loot_pickup:drop-1")

	input := validRemoveItemInput(t)
	input.ItemRef.Definition = definition
	input.SourceLocation = sourceLocation
	input.Quantity = 30
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:remove-1")

	first, err := service.RemoveItem(input)
	if err != nil {
		t.Fatalf("first RemoveItem: %v", err)
	}
	second, err := service.RemoveItem(input)
	if err != nil {
		t.Fatalf("duplicate RemoveItem: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first RemoveItem Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate RemoveItem Duplicate = false, want true")
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, sourceLocation); got != 45 {
		t.Fatalf("source TotalItemQuantity() = %d, want 45", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 2 {
		t.Fatalf("ledger entries len = %d, want 2", got)
	}
	if len(second.LedgerEntries) != 1 {
		t.Fatalf("duplicate result ledger entries len = %d, want 1", len(second.LedgerEntries))
	}
	if second.LedgerEntries[0].LedgerID != first.LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate LedgerID = %q, want %q", second.LedgerEntries[0].LedgerID, first.LedgerEntries[0].LedgerID)
	}
}

func TestRemoveItemRejectsInsufficientQuantityWithoutMutation(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	sourceLocation := validLocation(t)
	addStackableItems(t, service, definition, 10, sourceLocation, "loot_pickup:drop-1")

	input := validRemoveItemInput(t)
	input.ItemRef.Definition = definition
	input.SourceLocation = sourceLocation
	input.Quantity = 11
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:remove-1")

	_, err := service.RemoveItem(input)
	if !errors.Is(err, ErrInsufficientItemQuantity) {
		t.Fatalf("RemoveItem error = %v, want ErrInsufficientItemQuantity", err)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, sourceLocation); got != 10 {
		t.Fatalf("source TotalItemQuantity() = %d, want 10", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestRemoveItemInstanceQuantityAboveOneRejectedAndQuantityOneRemovesExactInstance(t *testing.T) {
	service := newTestInventoryService()
	definition := validInstanceDefinition(t)
	sourceLocation := validLocation(t)
	addResult := addInstanceItems(t, service, definition, 1, sourceLocation, "loot_pickup:drop-1")
	instanceID := addResult.InstanceItems[0].ItemInstanceID

	invalidInput := validRemoveItemInput(t)
	invalidInput.ItemRef = RemoveItemRef{
		Definition:     definition,
		ItemInstanceID: instanceID,
	}
	invalidInput.SourceLocation = sourceLocation
	invalidInput.Quantity = 2
	invalidInput.ReferenceKey = validReferenceKey(t, "loot_pickup:remove-1")

	if _, err := service.RemoveItem(invalidInput); !errors.Is(err, ErrInvalidInstanceQuantity) {
		t.Fatalf("RemoveItem quantity above one error = %v, want ErrInvalidInstanceQuantity", err)
	}
	if got := len(service.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries after rejected remove len = %d, want 1", got)
	}

	validInput := invalidInput
	validInput.Quantity = 1
	validInput.ReferenceKey = validReferenceKey(t, "loot_pickup:remove-2")
	result, err := service.RemoveItem(validInput)
	if err != nil {
		t.Fatalf("RemoveItem quantity one: %v", err)
	}

	if got := service.TotalItemQuantity(validInput.PlayerID, definition.ItemID, sourceLocation); got != 0 {
		t.Fatalf("source TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(service.InstanceItems()); got != 0 {
		t.Fatalf("instance items len = %d, want 0", got)
	}
	if len(result.InstanceItems) != 1 {
		t.Fatalf("result instance items len = %d, want 1", len(result.InstanceItems))
	}
	if result.InstanceItems[0].ItemInstanceID != instanceID {
		t.Fatalf("removed instance id = %q, want %q", result.InstanceItems[0].ItemInstanceID, instanceID)
	}
	if len(result.LedgerEntries) != 1 {
		t.Fatalf("result ledger entries len = %d, want 1", len(result.LedgerEntries))
	}
	if result.LedgerEntries[0].ItemInstanceID != instanceID {
		t.Fatalf("ledger item instance id = %q, want %q", result.LedgerEntries[0].ItemInstanceID, instanceID)
	}
	if result.LedgerEntries[0].BalanceAfter != 0 {
		t.Fatalf("ledger balance after = %d, want 0", result.LedgerEntries[0].BalanceAfter)
	}
}

func TestRemoveItemRejectsGenericRemoveFromEscrowReservedAndSystemLocations(t *testing.T) {
	cases := []struct {
		name string
		kind LocationKind
	}{
		{name: "market escrow", kind: LocationKindMarketEscrow},
		{name: "auction escrow", kind: LocationKindAuctionEscrow},
		{name: "crafting reserved", kind: LocationKindCraftingReserved},
		{name: "system sink", kind: LocationKindSystemSink},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestInventoryService()
			definition := validStackableDefinition(t)
			sourceLocation := validLocationKind(t, tc.kind, "reserved-1")
			addStackableItems(t, service, definition, 5, sourceLocation, "loot_pickup:drop-1")

			input := validRemoveItemInput(t)
			input.ItemRef.Definition = definition
			input.SourceLocation = sourceLocation
			input.Quantity = 1
			input.ReferenceKey = validReferenceKey(t, "loot_pickup:remove-1")

			_, err := service.RemoveItem(input)
			if !errors.Is(err, ErrBlockedGenericRemoveSource) {
				t.Fatalf("RemoveItem error = %v, want ErrBlockedGenericRemoveSource", err)
			}
			if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, sourceLocation); got != 5 {
				t.Fatalf("source TotalItemQuantity() = %d, want 5", got)
			}
			if got := len(service.ItemLedgerEntries()); got != 1 {
				t.Fatalf("ledger entries len = %d, want 1", got)
			}
		})
	}
}

func validRemoveItemInput(t *testing.T) RemoveItemInput {
	t.Helper()

	return RemoveItemInput{
		PlayerID: "player-1",
		ItemRef: RemoveItemRef{
			Definition: validStackableDefinition(t),
		},
		SourceLocation: validLocation(t),
		Quantity:       1,
		Reason:         "inventory_remove",
		ReferenceKey:   validReferenceKey(t, "loot_pickup:remove-1"),
	}
}
