package economy

import (
	"errors"
	"testing"
)

func TestMoveItemCargoTransferGuardBlocksShipCargoMoveWithoutMutation(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	fromLocation := validShipCargoLocation(t)
	toLocation := validStationStorageLocation(t)
	seedStackableItem(t, service, definition, 5, fromLocation)
	guardErr := errors.New("cargo locked")
	service.SetCargoTransferGuard(&recordingCargoTransferGuard{err: guardErr})

	input := validMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.ToLocation = toLocation
	input.Quantity = 2
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:guarded-move-from-cargo")

	_, err := service.MoveItem(input)
	if !errors.Is(err, guardErr) {
		t.Fatalf("MoveItem error = %v, want guard error", err)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 5 {
		t.Fatalf("source TotalItemQuantity() = %d, want 5", got)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, toLocation); got != 0 {
		t.Fatalf("destination TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 0 {
		t.Fatalf("ledger entries len = %d, want 0", got)
	}
}

func TestMoveItemCargoTransferGuardAllowsDuplicateMoveRetry(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	fromLocation := validShipCargoLocation(t)
	toLocation := validStationStorageLocation(t)
	seedStackableItem(t, service, definition, 5, fromLocation)

	input := validMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.ToLocation = toLocation
	input.Quantity = 2
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:guarded-duplicate-move")

	first, err := service.MoveItem(input)
	if err != nil {
		t.Fatalf("first MoveItem error = %v", err)
	}
	guard := &recordingCargoTransferGuard{err: errors.New("cargo locked")}
	service.SetCargoTransferGuard(guard)

	second, err := service.MoveItem(input)
	if err != nil {
		t.Fatalf("duplicate MoveItem error = %v, want nil", err)
	}
	if !second.Duplicate {
		t.Fatal("duplicate MoveItem Duplicate = false, want true")
	}
	if second.LedgerEntries[0].LedgerID != first.LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate source LedgerID = %q, want %q", second.LedgerEntries[0].LedgerID, first.LedgerEntries[0].LedgerID)
	}
	if got := len(guard.inputs); got != 0 {
		t.Fatalf("guard calls for duplicate = %d, want 0", got)
	}
}

func TestMoveItemCargoTransferGuardIgnoresNonCargoMove(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	toLocation := validStationStorageLocation(t)
	addStackableItems(t, service, definition, 5, fromLocation, "loot_pickup:non-cargo-guard-seed")
	guard := &recordingCargoTransferGuard{err: errors.New("cargo locked")}
	service.SetCargoTransferGuard(guard)

	input := validMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.ToLocation = toLocation
	input.Quantity = 2
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:non-cargo-guard-move")

	if _, err := service.MoveItem(input); err != nil {
		t.Fatalf("MoveItem non-cargo error = %v, want nil", err)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 3 {
		t.Fatalf("source TotalItemQuantity() = %d, want 3", got)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, toLocation); got != 2 {
		t.Fatalf("destination TotalItemQuantity() = %d, want 2", got)
	}
	if got := len(guard.inputs); got != 0 {
		t.Fatalf("guard calls for non-cargo move = %d, want 0", got)
	}
}

func TestSystemMoveItemBypassesCargoTransferGuard(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	fromLocation := validShipCargoLocation(t)
	toLocation := validStationStorageLocation(t)
	seedStackableItem(t, service, definition, 5, fromLocation)
	service.SetCargoTransferGuard(&recordingCargoTransferGuard{err: errors.New("cargo locked")})

	input := validMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.ToLocation = toLocation
	input.Quantity = 2
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:system-move-from-cargo")

	if _, err := service.SystemMoveItem(input); err != nil {
		t.Fatalf("SystemMoveItem error = %v, want nil", err)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 3 {
		t.Fatalf("source TotalItemQuantity() = %d, want 3", got)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, toLocation); got != 2 {
		t.Fatalf("destination TotalItemQuantity() = %d, want 2", got)
	}
}
