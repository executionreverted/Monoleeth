package economy

import (
	"errors"
	"sync"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestCargoServiceAllowsCapacitySafeAdd(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	input := validCargoAddItemInput(t)

	result, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if result.Duplicate {
		t.Fatal("Duplicate = true, want false")
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.ActiveCargo); got != input.Quantity {
		t.Fatalf("TotalItemQuantity() = %d, want %d", got, input.Quantity)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestCargoServiceBlocksOverCapacityAddWithoutMutationOrLedger(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	input := validCargoAddItemInput(t)
	input.ItemDefinition = validWeightedStackableDefinition(t, 3)
	input.Quantity = 2
	input.CargoCapacityUnits = 5

	_, err := service.AddItem(input)
	if !errors.Is(err, ErrCargoCapacityExceeded) {
		t.Fatalf("AddItem error = %v, want ErrCargoCapacityExceeded", err)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.ActiveCargo); got != 0 {
		t.Fatalf("TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 0 {
		t.Fatalf("ledger entries len = %d, want 0", got)
	}
}

func TestCargoServiceComputesUsedCargoFromInventoryState(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	input := validCargoAddItemInput(t)
	input.ItemDefinition = validWeightedStackableDefinition(t, 2)
	input.Quantity = 2
	input.CargoCapacityUnits = 4

	if _, err := inventory.AddItem(AddItemInput{
		PlayerID:       input.PlayerID,
		ItemDefinition: input.ItemDefinition,
		Quantity:       input.Quantity,
		Location:       input.ActiveCargo,
		Reason:         "loot_pickup",
		ReferenceKey:   validReferenceKey(t, "loot_pickup:seed-cargo-1"),
	}); err != nil {
		t.Fatalf("seed AddItem: %v", err)
	}

	input.Quantity = 1
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:drop-2")
	_, err := service.AddItem(input)
	if !errors.Is(err, ErrCargoCapacityExceeded) {
		t.Fatalf("AddItem error = %v, want ErrCargoCapacityExceeded", err)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.ActiveCargo); got != 2 {
		t.Fatalf("TotalItemQuantity() = %d, want 2", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestCargoServiceDuplicateReferenceReturnsPreviousResultWhenCargoIsFull(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	input := validCargoAddItemInput(t)
	input.ItemDefinition = validWeightedStackableDefinition(t, 5)
	input.Quantity = 2
	input.CargoCapacityUnits = 10

	first, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("first AddItem: %v", err)
	}
	second, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("duplicate AddItem: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("second Duplicate = false, want true")
	}
	if second.LedgerEntry.LedgerID != first.LedgerEntry.LedgerID {
		t.Fatalf("duplicate LedgerID = %q, want %q", second.LedgerEntry.LedgerID, first.LedgerEntry.LedgerID)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.ActiveCargo); got != 2 {
		t.Fatalf("TotalItemQuantity() = %d, want 2", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestCargoServiceConcurrentPickupOnlyAllowsCapacitySafeResult(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	input := validCargoAddItemInput(t)
	input.ItemDefinition = validWeightedStackableDefinition(t, 6)
	input.Quantity = 1
	input.CargoCapacityUnits = 10

	const attempts = 2
	referenceKeys := []foundation.IdempotencyKey{
		validReferenceKey(t, "loot_pickup:concurrent-drop-a"),
		validReferenceKey(t, "loot_pickup:concurrent-drop-b"),
	}
	errs := make(chan error, attempts)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(attempts)

	for i := 0; i < attempts; i++ {
		go func(index int) {
			defer wg.Done()
			<-start
			attempt := input
			attempt.ReferenceKey = referenceKeys[index]
			_, err := service.AddItem(attempt)
			errs <- err
		}(i)
	}

	close(start)
	wg.Wait()
	close(errs)

	var successes int
	var capacityErrors int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrCargoCapacityExceeded):
			capacityErrors++
		default:
			t.Fatalf("unexpected AddItem error: %v", err)
		}
	}

	if successes != 1 {
		t.Fatalf("successful pickups = %d, want 1", successes)
	}
	if capacityErrors != 1 {
		t.Fatalf("capacity errors = %d, want 1", capacityErrors)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.ActiveCargo); got != 1 {
		t.Fatalf("TotalItemQuantity() = %d, want 1", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestCargoServiceValidatesCargoInputs(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*CargoAddItemInput)
		wantErr error
	}{
		{
			name: "account inventory location",
			mutate: func(input *CargoAddItemInput) {
				input.ActiveCargo = validLocation(t)
			},
			wantErr: ErrInvalidCargoLocation,
		},
		{
			name: "negative capacity",
			mutate: func(input *CargoAddItemInput) {
				input.CargoCapacityUnits = -1
			},
			wantErr: ErrNegativeCargoCapacity,
		},
		{
			name: "negative quantity",
			mutate: func(input *CargoAddItemInput) {
				input.Quantity = -1
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inventory := newTestInventoryService()
			service := NewCargoService(inventory)
			input := validCargoAddItemInput(t)
			tc.mutate(&input)

			_, err := service.AddItem(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("AddItem error = %v, want %v", err, tc.wantErr)
			}
			if got := len(inventory.ItemLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func validCargoAddItemInput(t *testing.T) CargoAddItemInput {
	t.Helper()

	return CargoAddItemInput{
		PlayerID:           "player-1",
		ActiveCargo:        validShipCargoLocation(t),
		ItemDefinition:     validStackableDefinition(t),
		Quantity:           5,
		CargoCapacityUnits: 10,
		Reason:             "loot_pickup",
		ReferenceKey:       validReferenceKey(t, "loot_pickup:drop-1"),
	}
}

func validWeightedStackableDefinition(t *testing.T, weightUnits int64) ItemDefinition {
	t.Helper()

	definition := validStackableDefinition(t)
	definition.WeightUnits = validQuantity(t, weightUnits)
	return definition
}
