package economy

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestAddItemRejectsNegativeQuantity(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)
	input.Quantity = -1

	_, err := service.AddItem(input)
	if !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("AddItem negative quantity error = %v, want foundation.ErrNonPositiveAmount", err)
	}
	if got := service.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.Location); got != 0 {
		t.Fatalf("TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 0 {
		t.Fatalf("ledger entries len = %d, want 0", got)
	}
}

func TestAddItemRejectsZeroQuantity(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)
	input.Quantity = 0

	_, err := service.AddItem(input)
	if !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("AddItem zero quantity error = %v, want foundation.ErrNonPositiveAmount", err)
	}
	if got := service.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.Location); got != 0 {
		t.Fatalf("TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 0 {
		t.Fatalf("ledger entries len = %d, want 0", got)
	}
}

func TestAddItemValidatesRequiredInputs(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*AddItemInput)
		wantErr error
	}{
		{
			name: "blank player",
			mutate: func(input *AddItemInput) {
				input.PlayerID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "blank item",
			mutate: func(input *AddItemInput) {
				input.ItemDefinition.ItemID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "blank location",
			mutate: func(input *AddItemInput) {
				input.Location = ItemLocation{}
			},
			wantErr: ErrInvalidLocationKind,
		},
		{
			name: "blank reason",
			mutate: func(input *AddItemInput) {
				input.Reason = ""
			},
			wantErr: ErrEmptyLedgerReason,
		},
		{
			name: "blank reference",
			mutate: func(input *AddItemInput) {
				input.ReferenceKey = ""
			},
			wantErr: foundation.ErrEmptyIdempotencyKey,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestInventoryService()
			input := validAddItemInput(t)
			tc.mutate(&input)

			_, err := service.AddItem(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("AddItem error = %v, want %v", err, tc.wantErr)
			}
			if got := len(service.ItemLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestAddItemRejectsGenericCargoAndEquippedTargets(t *testing.T) {
	cases := []ItemLocation{
		validShipCargoLocation(t),
		{Kind: LocationKindShipEquipped, ID: "ship-1"},
	}

	for _, location := range cases {
		t.Run(location.Kind.String(), func(t *testing.T) {
			service := newTestInventoryService()
			input := validAddItemInput(t)
			input.Location = location

			_, err := service.AddItem(input)
			if !errors.Is(err, ErrBlockedGenericMoveTarget) {
				t.Fatalf("AddItem error = %v, want ErrBlockedGenericMoveTarget", err)
			}
			if got := service.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.Location); got != 0 {
				t.Fatalf("TotalItemQuantity() = %d, want 0", got)
			}
			if got := len(service.ItemLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestAddItemDuplicateReferenceDoesNotDuplicateGrant(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)

	first, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("first AddItem: %v", err)
	}
	second, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("duplicate AddItem: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first AddItem Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate AddItem Duplicate = false, want true")
	}
	if got := service.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.Location); got != input.Quantity {
		t.Fatalf("TotalItemQuantity() = %d, want %d", got, input.Quantity)
	}
	if got := len(service.StackableItems()); got != 1 {
		t.Fatalf("stackable items len = %d, want 1", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
	if second.LedgerEntry.LedgerID != first.LedgerEntry.LedgerID {
		t.Fatalf("duplicate LedgerID = %q, want %q", second.LedgerEntry.LedgerID, first.LedgerEntry.LedgerID)
	}
}

func TestAddItemWritesItemLedgerEntryWithReasonAndReference(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)

	result, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	entries := service.ItemLedgerEntries()
	if len(entries) != 1 {
		t.Fatalf("ledger entries len = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.LedgerID.IsZero() {
		t.Fatal("ledger id is zero")
	}
	if entry.PlayerID != input.PlayerID {
		t.Fatalf("ledger player = %q, want %q", entry.PlayerID, input.PlayerID)
	}
	if entry.ItemID != input.ItemDefinition.ItemID {
		t.Fatalf("ledger item = %q, want %q", entry.ItemID, input.ItemDefinition.ItemID)
	}
	if got := entry.Quantity.Int64(); got != input.Quantity {
		t.Fatalf("ledger quantity = %d, want %d", got, input.Quantity)
	}
	if entry.Action != LedgerActionIncrease {
		t.Fatalf("ledger action = %q, want %q", entry.Action, LedgerActionIncrease)
	}
	if entry.BalanceAfter != input.Quantity {
		t.Fatalf("ledger balance after = %d, want %d", entry.BalanceAfter, input.Quantity)
	}
	if entry.Location != input.Location {
		t.Fatalf("ledger location = %v, want %v", entry.Location, input.Location)
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
	if result.LedgerEntry != entry {
		t.Fatalf("result ledger entry = %#v, want %#v", result.LedgerEntry, entry)
	}
}

func TestAddItemSplitsStackableRowsByDefinitionMaxStack(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)
	input.Quantity = 250

	result, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if got := len(result.StackableItems); got != 3 {
		t.Fatalf("stackable result len = %d, want 3", got)
	}
	wantQuantities := []int64{100, 100, 50}
	for index, item := range result.StackableItems {
		if got := item.Quantity.Int64(); got != wantQuantities[index] {
			t.Fatalf("stack %d quantity = %d, want %d", index, got, wantQuantities[index])
		}
	}
	if got := result.LedgerEntry.Quantity.Int64(); got != input.Quantity {
		t.Fatalf("ledger quantity = %d, want %d", got, input.Quantity)
	}
	if got := result.LedgerEntry.BalanceAfter; got != input.Quantity {
		t.Fatalf("ledger balance after = %d, want %d", got, input.Quantity)
	}
}

var testInventoryNow = time.Date(2026, 6, 17, 15, 0, 0, 0, time.UTC)

func newTestInventoryService() *InventoryService {
	return NewInventoryService(testutil.NewFakeClock(testInventoryNow))
}

func validAddItemInput(t *testing.T) AddItemInput {
	t.Helper()

	return AddItemInput{
		PlayerID:       "player-1",
		ItemDefinition: validStackableDefinition(t),
		Quantity:       5,
		Location:       validLocation(t),
		Reason:         "loot_pickup",
		ReferenceKey:   validReferenceKey(t, "loot_pickup:drop-1"),
	}
}
