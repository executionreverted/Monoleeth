package economy

import (
	"errors"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/events"
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
	seedDefinition := validWeightedDefinition(t, "cargo_seed", 2)
	input.ItemDefinition = validWeightedDefinition(t, "cargo_incoming", 1)
	input.Quantity = 1
	input.CargoCapacityUnits = 4

	seedCargoRowForTest(t, inventory, input.PlayerID, seedDefinition, 2, input.ActiveCargo, "loot_pickup:seed-cargo-1")
	if err := service.RegisterItemDefinition(seedDefinition); err != nil {
		t.Fatalf("RegisterItemDefinition: %v", err)
	}

	input.ReferenceKey = validReferenceKey(t, "loot_pickup:drop-2")
	_, err := service.AddItem(input)
	if !errors.Is(err, ErrCargoCapacityExceeded) {
		t.Fatalf("AddItem error = %v, want ErrCargoCapacityExceeded", err)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, seedDefinition.ItemID, input.ActiveCargo); got != 2 {
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

func TestCargoServiceMoveItemRespectsCapacityAndWritesLedger(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	definition := validWeightedStackableDefinition(t, 2)
	fromLocation := validLocation(t)
	activeCargo := validShipCargoLocation(t)
	addStackableItems(t, inventory, definition, 5, fromLocation, "loot_pickup:cargo-move-seed")

	input := validCargoMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.ActiveCargo = activeCargo
	input.Quantity = 3
	input.CargoCapacityUnits = 6
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:cargo-move")

	result, err := service.MoveItem(input)
	if err != nil {
		t.Fatalf("MoveItem: %v", err)
	}
	if result.Duplicate {
		t.Fatal("MoveItem Duplicate = true, want false")
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 2 {
		t.Fatalf("source TotalItemQuantity() = %d, want 2", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, activeCargo); got != 3 {
		t.Fatalf("cargo TotalItemQuantity() = %d, want 3", got)
	}
	if got := len(result.LedgerEntries); got != 2 {
		t.Fatalf("result ledger entries len = %d, want 2", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 3 {
		t.Fatalf("ledger entries len = %d, want 3", got)
	}
}

func TestCargoServiceMoveItemDuplicateReferenceReturnsPreviousResultWhenCargoIsFull(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	definition := validWeightedStackableDefinition(t, 5)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 3, fromLocation, "loot_pickup:cargo-move-duplicate-seed")

	input := validCargoMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.Quantity = 2
	input.CargoCapacityUnits = 10
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:cargo-move-duplicate")

	first, err := service.MoveItem(input)
	if err != nil {
		t.Fatalf("first MoveItem: %v", err)
	}
	second, err := service.MoveItem(input)
	if err != nil {
		t.Fatalf("duplicate MoveItem: %v", err)
	}
	if first.Duplicate {
		t.Fatal("first Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("second Duplicate = false, want true")
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, input.ActiveCargo); got != 2 {
		t.Fatalf("cargo TotalItemQuantity() = %d, want 2", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 3 {
		t.Fatalf("ledger entries len = %d, want 3", got)
	}
	if second.LedgerEntries[0].LedgerID != first.LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate source LedgerID = %q, want %q", second.LedgerEntries[0].LedgerID, first.LedgerEntries[0].LedgerID)
	}
}

func TestCargoServiceMoveItemBlocksOverCapacityWithoutMutationOrLedger(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	definition := validWeightedStackableDefinition(t, 4)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 2, fromLocation, "loot_pickup:cargo-move-over-capacity-seed")

	input := validCargoMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.Quantity = 2
	input.CargoCapacityUnits = 7
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:cargo-move-over-capacity")

	_, err := service.MoveItem(input)
	if !errors.Is(err, ErrCargoCapacityExceeded) {
		t.Fatalf("MoveItem error = %v, want ErrCargoCapacityExceeded", err)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 2 {
		t.Fatalf("source TotalItemQuantity() = %d, want 2", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, input.ActiveCargo); got != 0 {
		t.Fatalf("cargo TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestCargoServiceAddItemDoesNotDeadlockWhenInventoryEmitterReentersCargo(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	input := validCargoAddItemInput(t)
	inventory.SetEventEmitter(reentrantCargoEmitter{
		t:          t,
		service:    service,
		definition: input.ItemDefinition,
	})

	done := make(chan error, 1)
	go func() {
		_, err := service.AddItem(input)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AddItem: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("AddItem deadlocked while inventory emitter re-entered CargoService")
	}
}

func TestCargoServiceBlocksDirectInventoryCargoBypass(t *testing.T) {
	inventory := newTestInventoryService()
	definition := validWeightedStackableDefinition(t, 1)
	fromLocation := validLocation(t)
	activeCargo := validShipCargoLocation(t)
	addStackableItems(t, inventory, definition, 2, fromLocation, "loot_pickup:direct-cargo-bypass-seed")

	if _, err := inventory.AddItem(AddItemInput{
		PlayerID:       "player-1",
		ItemDefinition: definition,
		Quantity:       1,
		Location:       activeCargo,
		Reason:         "loot_pickup",
		ReferenceKey:   validReferenceKey(t, "loot_pickup:direct-cargo-add"),
	}); !errors.Is(err, ErrBlockedGenericMoveTarget) {
		t.Fatalf("direct AddItem cargo error = %v, want ErrBlockedGenericMoveTarget", err)
	}

	_, err := inventory.MoveItem(MoveItemInput{
		PlayerID:     "player-1",
		ItemRef:      MoveItemRef{Definition: definition},
		FromLocation: fromLocation,
		ToLocation:   activeCargo,
		Quantity:     1,
		Reason:       "inventory_move",
		ReferenceKey: validReferenceKey(t, "loot_pickup:direct-cargo-move"),
	})
	if !errors.Is(err, ErrBlockedGenericMoveTarget) {
		t.Fatalf("direct MoveItem cargo error = %v, want ErrBlockedGenericMoveTarget", err)
	}
	if got := inventory.TotalItemQuantity("player-1", definition.ItemID, activeCargo); got != 0 {
		t.Fatalf("cargo TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestCargoServiceGuardBlocksAddAndMoveWithoutMutation(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	guardErr := errors.New("cargo locked")
	guard := &recordingCargoTransferGuard{err: guardErr}
	service.SetCargoTransferGuard(guard)

	addInput := validCargoAddItemInput(t)
	if _, err := service.AddItem(addInput); !errors.Is(err, guardErr) {
		t.Fatalf("AddItem error = %v, want guard error", err)
	}
	if got := inventory.TotalItemQuantity(addInput.PlayerID, addInput.ItemDefinition.ItemID, addInput.ActiveCargo); got != 0 {
		t.Fatalf("cargo TotalItemQuantity after guarded add = %d, want 0", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 0 {
		t.Fatalf("ledger entries after guarded add = %d, want 0", got)
	}

	definition := validWeightedStackableDefinition(t, 1)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 3, fromLocation, "loot_pickup:guarded-cargo-move-seed")
	moveInput := validCargoMoveItemInput(t)
	moveInput.ItemRef.Definition = definition
	moveInput.FromLocation = fromLocation
	moveInput.Quantity = 2
	moveInput.ReferenceKey = validReferenceKey(t, "loot_pickup:guarded-cargo-move")

	if _, err := service.MoveItem(moveInput); !errors.Is(err, guardErr) {
		t.Fatalf("MoveItem error = %v, want guard error", err)
	}
	if got := inventory.TotalItemQuantity(moveInput.PlayerID, definition.ItemID, fromLocation); got != 3 {
		t.Fatalf("source TotalItemQuantity after guarded move = %d, want 3", got)
	}
	if got := inventory.TotalItemQuantity(moveInput.PlayerID, definition.ItemID, moveInput.ActiveCargo); got != 0 {
		t.Fatalf("cargo TotalItemQuantity after guarded move = %d, want 0", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries after guarded move = %d, want seed ledger only", got)
	}
	if got, want := len(guard.inputs), 2; got != want {
		t.Fatalf("guard calls = %d, want %d", got, want)
	}
	if !guard.inputs[0].InvolvesShipCargo() || !guard.inputs[1].InvolvesShipCargo() {
		t.Fatalf("guard inputs = %+v, want ship cargo involvement", guard.inputs)
	}
}

func TestCargoServiceGuardAllowsDuplicateRetries(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)

	addInput := validCargoAddItemInput(t)
	firstAdd, err := service.AddItem(addInput)
	if err != nil {
		t.Fatalf("first AddItem error = %v", err)
	}
	addGuard := &recordingCargoTransferGuard{err: errors.New("cargo locked")}
	service.SetCargoTransferGuard(addGuard)
	secondAdd, err := service.AddItem(addInput)
	if err != nil {
		t.Fatalf("duplicate AddItem error = %v, want nil", err)
	}
	if !secondAdd.Duplicate {
		t.Fatal("duplicate AddItem Duplicate = false, want true")
	}
	if secondAdd.LedgerEntry.LedgerID != firstAdd.LedgerEntry.LedgerID {
		t.Fatalf("duplicate add LedgerID = %q, want %q", secondAdd.LedgerEntry.LedgerID, firstAdd.LedgerEntry.LedgerID)
	}
	if got := len(addGuard.inputs); got != 0 {
		t.Fatalf("add guard calls for duplicate = %d, want 0", got)
	}

	moveGuard := &recordingCargoTransferGuard{err: errors.New("cargo locked")}
	service.SetCargoTransferGuard(nil)
	definition := validWeightedStackableDefinition(t, 1)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 3, fromLocation, "loot_pickup:guarded-duplicate-cargo-move-seed")
	moveInput := validCargoMoveItemInput(t)
	moveInput.ItemRef.Definition = definition
	moveInput.FromLocation = fromLocation
	moveInput.Quantity = 2
	moveInput.ReferenceKey = validReferenceKey(t, "loot_pickup:guarded-duplicate-cargo-move")

	firstMove, err := service.MoveItem(moveInput)
	if err != nil {
		t.Fatalf("first MoveItem error = %v", err)
	}
	service.SetCargoTransferGuard(moveGuard)
	secondMove, err := service.MoveItem(moveInput)
	if err != nil {
		t.Fatalf("duplicate MoveItem error = %v, want nil", err)
	}
	if !secondMove.Duplicate {
		t.Fatal("duplicate MoveItem Duplicate = false, want true")
	}
	if secondMove.LedgerEntries[0].LedgerID != firstMove.LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate move LedgerID = %q, want %q", secondMove.LedgerEntries[0].LedgerID, firstMove.LedgerEntries[0].LedgerID)
	}
	if got := len(moveGuard.inputs); got != 0 {
		t.Fatalf("move guard calls for duplicate = %d, want 0", got)
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

func validCargoMoveItemInput(t *testing.T) CargoMoveItemInput {
	t.Helper()

	return CargoMoveItemInput{
		PlayerID: "player-1",
		ItemRef: MoveItemRef{
			Definition: validStackableDefinition(t),
		},
		FromLocation:       validLocation(t),
		ActiveCargo:        validShipCargoLocation(t),
		Quantity:           1,
		CargoCapacityUnits: 10,
		Reason:             "inventory_move",
		ReferenceKey:       validReferenceKey(t, "loot_pickup:cargo-move"),
	}
}

func validWeightedStackableDefinition(t *testing.T, weightUnits int64) ItemDefinition {
	t.Helper()

	definition := validStackableDefinition(t)
	definition.WeightUnits = validQuantity(t, weightUnits)
	return definition
}

func validWeightedDefinition(t *testing.T, itemID string, weightUnits int64) ItemDefinition {
	t.Helper()

	source := validItemSource(t, itemID)
	maxStack := validQuantity(t, 100)
	weight := validQuantity(t, weightUnits)
	definition, err := NewItemDefinition(
		source,
		foundation.ItemID(itemID),
		itemID,
		ItemTypeStackable,
		ItemRarityCommon,
		maxStack,
		weight,
		[]TradeFlag{TradeFlagTradeable},
		[]BindRule{BindRuleNone},
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition(%q): %v", itemID, err)
	}
	return definition
}

func seedCargoRowForTest(
	t *testing.T,
	inventory *InventoryService,
	playerID foundation.PlayerID,
	definition ItemDefinition,
	quantity int64,
	location ItemLocation,
	reference string,
) AddItemResult {
	t.Helper()

	input := AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: definition,
		Quantity:       quantity,
		Location:       location,
		Reason:         "loot_pickup",
		ReferenceKey:   validReferenceKey(t, reference),
	}
	amount, err := input.validateCargoAdd()
	if err != nil {
		t.Fatalf("validate cargo seed: %v", err)
	}
	inventory.mu.Lock()
	defer inventory.mu.Unlock()

	result, err := inventory.addItemValidatedLocked(input, amount, inventory.clock.Now())
	if err != nil {
		t.Fatalf("seed cargo add: %v", err)
	}
	return result
}

type reentrantCargoEmitter struct {
	t          *testing.T
	service    *CargoService
	definition ItemDefinition
}

func (emitter reentrantCargoEmitter) Record(_ events.EventEnvelope) {
	emitter.t.Helper()
	if err := emitter.service.RegisterItemDefinition(emitter.definition); err != nil {
		emitter.t.Errorf("RegisterItemDefinition from emitter: %v", err)
	}
}

type recordingCargoTransferGuard struct {
	err    error
	inputs []CargoTransferGuardInput
	active int
}

func (guard *recordingCargoTransferGuard) BeginCargoTransfer(input CargoTransferGuardInput) (CargoTransferLease, error) {
	guard.inputs = append(guard.inputs, input)
	if guard.err != nil {
		return nil, guard.err
	}
	guard.active++
	return CargoTransferLeaseFunc(func() {
		guard.active--
	}), nil
}
