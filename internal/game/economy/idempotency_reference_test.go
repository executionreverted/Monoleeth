package economy

import (
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestInventoryReferencesAreScopedByOperation(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	account := validLocation(t)
	cargo := validShipCargoLocation(t)
	reference := validReferenceKey(t, "loot_pickup:shared-inventory-reference")

	addResult, err := service.AddItem(AddItemInput{
		PlayerID:       "player-1",
		ItemDefinition: definition,
		Quantity:       10,
		Location:       account,
		Reason:         "loot_pickup",
		ReferenceKey:   reference,
	})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	moveResult, err := service.MoveItem(MoveItemInput{
		PlayerID: "player-1",
		ItemRef: MoveItemRef{
			Definition: definition,
		},
		FromLocation: account,
		ToLocation:   cargo,
		Quantity:     4,
		Reason:       "inventory_move",
		ReferenceKey: reference,
	})
	if err != nil {
		t.Fatalf("MoveItem with AddItem reference: %v", err)
	}
	removeResult, err := service.RemoveItem(RemoveItemInput{
		PlayerID: "player-1",
		ItemRef: RemoveItemRef{
			Definition: definition,
		},
		SourceLocation: account,
		Quantity:       2,
		Reason:         "inventory_remove",
		ReferenceKey:   reference,
	})
	if err != nil {
		t.Fatalf("RemoveItem with AddItem/MoveItem reference: %v", err)
	}

	if addResult.Duplicate || moveResult.Duplicate || removeResult.Duplicate {
		t.Fatalf("operation-scoped references reported duplicate: add=%v move=%v remove=%v", addResult.Duplicate, moveResult.Duplicate, removeResult.Duplicate)
	}
	if got := service.TotalItemQuantity("player-1", definition.ItemID, account); got != 4 {
		t.Fatalf("account TotalItemQuantity() = %d, want 4", got)
	}
	if got := service.TotalItemQuantity("player-1", definition.ItemID, cargo); got != 4 {
		t.Fatalf("cargo TotalItemQuantity() = %d, want 4", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 4 {
		t.Fatalf("ledger entries len = %d, want 4", got)
	}
}

func TestWalletReferencesAreScopedByOperation(t *testing.T) {
	service := newTestWalletService()
	reference := "quest_reward:shared-wallet-reference"

	creditInput := validCreditWalletInput(t, reference)
	creditResult, err := service.CreditWallet(creditInput)
	if err != nil {
		t.Fatalf("CreditWallet: %v", err)
	}

	debitInput := validDebitWalletInput(t, reference)
	debitResult, err := service.DebitWallet(debitInput)
	if err != nil {
		t.Fatalf("DebitWallet with CreditWallet reference: %v", err)
	}

	transferInput := validTransferCurrencyInput(t, reference)
	transferInput.Amount = 50
	transferResult, err := service.TransferCurrency(transferInput)
	if err != nil {
		t.Fatalf("TransferCurrency with CreditWallet/DebitWallet reference: %v", err)
	}

	if creditResult.Duplicate || debitResult.Duplicate || transferResult.Duplicate {
		t.Fatalf("operation-scoped references reported duplicate: credit=%v debit=%v transfer=%v", creditResult.Duplicate, debitResult.Duplicate, transferResult.Duplicate)
	}
	if got := service.Balance("player-1", CurrencyBucketCredits); got != 50 {
		t.Fatalf("player-1 Balance() = %d, want 50", got)
	}
	if got := service.Balance("player-2", CurrencyBucketCredits); got != 50 {
		t.Fatalf("player-2 Balance() = %d, want 50", got)
	}
	if got := len(service.CurrencyLedgerEntries()); got != 4 {
		t.Fatalf("ledger entries len = %d, want 4", got)
	}
}

func TestInventoryFailedMutationsDoNotReserveReferenceKeys(t *testing.T) {
	t.Run("add", func(t *testing.T) {
		service := newTestInventoryService()
		input := validAddItemInput(t)
		input.ReferenceKey = validReferenceKey(t, "loot_pickup:retry-add")
		input.Quantity = 0

		_, err := service.AddItem(input)
		if !errors.Is(err, foundation.ErrNonPositiveAmount) {
			t.Fatalf("AddItem error = %v, want foundation.ErrNonPositiveAmount", err)
		}

		input.Quantity = 5
		result, err := service.AddItem(input)
		if err != nil {
			t.Fatalf("retry AddItem with same reference: %v", err)
		}
		if result.Duplicate {
			t.Fatal("retry AddItem Duplicate = true, want false")
		}
		if got := len(service.ItemLedgerEntries()); got != 1 {
			t.Fatalf("ledger entries len = %d, want 1", got)
		}
	})

	t.Run("move", func(t *testing.T) {
		service := newTestInventoryService()
		definition := validStackableDefinition(t)
		fromLocation := validLocation(t)
		toLocation := validShipCargoLocation(t)
		addStackableItems(t, service, definition, 2, fromLocation, "loot_pickup:retry-move-seed")

		input := validMoveItemInput(t)
		input.ItemRef.Definition = definition
		input.FromLocation = fromLocation
		input.ToLocation = toLocation
		input.ReferenceKey = validReferenceKey(t, "loot_pickup:retry-move")
		input.Quantity = 3

		_, err := service.MoveItem(input)
		if !errors.Is(err, ErrInsufficientItemQuantity) {
			t.Fatalf("MoveItem error = %v, want ErrInsufficientItemQuantity", err)
		}

		input.Quantity = 2
		result, err := service.MoveItem(input)
		if err != nil {
			t.Fatalf("retry MoveItem with same reference: %v", err)
		}
		if result.Duplicate {
			t.Fatal("retry MoveItem Duplicate = true, want false")
		}
		if got := len(service.ItemLedgerEntries()); got != 3 {
			t.Fatalf("ledger entries len = %d, want 3", got)
		}
	})

	t.Run("remove", func(t *testing.T) {
		service := newTestInventoryService()
		definition := validStackableDefinition(t)
		sourceLocation := validLocation(t)
		addStackableItems(t, service, definition, 2, sourceLocation, "loot_pickup:retry-remove-seed")

		input := validRemoveItemInput(t)
		input.ItemRef.Definition = definition
		input.SourceLocation = sourceLocation
		input.ReferenceKey = validReferenceKey(t, "loot_pickup:retry-remove")
		input.Quantity = 3

		_, err := service.RemoveItem(input)
		if !errors.Is(err, ErrInsufficientItemQuantity) {
			t.Fatalf("RemoveItem error = %v, want ErrInsufficientItemQuantity", err)
		}

		input.Quantity = 2
		result, err := service.RemoveItem(input)
		if err != nil {
			t.Fatalf("retry RemoveItem with same reference: %v", err)
		}
		if result.Duplicate {
			t.Fatal("retry RemoveItem Duplicate = true, want false")
		}
		if got := len(service.ItemLedgerEntries()); got != 2 {
			t.Fatalf("ledger entries len = %d, want 2", got)
		}
	})
}

func TestWalletFailedMutationsDoNotReserveReferenceKeys(t *testing.T) {
	t.Run("credit", func(t *testing.T) {
		service := newTestWalletService()
		input := validCreditWalletInput(t, "quest_reward:retry-credit")
		input.Amount = 0

		_, err := service.CreditWallet(input)
		if !errors.Is(err, foundation.ErrNonPositiveAmount) {
			t.Fatalf("CreditWallet error = %v, want foundation.ErrNonPositiveAmount", err)
		}

		input.Amount = 250
		result, err := service.CreditWallet(input)
		if err != nil {
			t.Fatalf("retry CreditWallet with same reference: %v", err)
		}
		if result.Duplicate {
			t.Fatal("retry CreditWallet Duplicate = true, want false")
		}
		if got := len(service.CurrencyLedgerEntries()); got != 1 {
			t.Fatalf("ledger entries len = %d, want 1", got)
		}
	})

	t.Run("debit", func(t *testing.T) {
		service := newTestWalletService()
		input := validDebitWalletInput(t, "quest_reward:retry-debit")

		_, err := service.DebitWallet(input)
		if !errors.Is(err, ErrInsufficientWalletFunds) {
			t.Fatalf("DebitWallet error = %v, want ErrInsufficientWalletFunds", err)
		}

		creditWalletForTest(t, service, "player-1", CurrencyBucketCredits, 150, "quest_reward:retry-debit-seed")
		result, err := service.DebitWallet(input)
		if err != nil {
			t.Fatalf("retry DebitWallet with same reference: %v", err)
		}
		if result.Duplicate {
			t.Fatal("retry DebitWallet Duplicate = true, want false")
		}
		if got := len(service.CurrencyLedgerEntries()); got != 2 {
			t.Fatalf("ledger entries len = %d, want 2", got)
		}
	})

	t.Run("transfer", func(t *testing.T) {
		service := newTestWalletService()
		input := validTransferCurrencyInput(t, "quest_reward:retry-transfer")

		_, err := service.TransferCurrency(input)
		if !errors.Is(err, ErrInsufficientWalletFunds) {
			t.Fatalf("TransferCurrency error = %v, want ErrInsufficientWalletFunds", err)
		}

		creditWalletForTest(t, service, "player-1", CurrencyBucketCredits, 200, "quest_reward:retry-transfer-seed")
		result, err := service.TransferCurrency(input)
		if err != nil {
			t.Fatalf("retry TransferCurrency with same reference: %v", err)
		}
		if result.Duplicate {
			t.Fatal("retry TransferCurrency Duplicate = true, want false")
		}
		if got := len(service.CurrencyLedgerEntries()); got != 3 {
			t.Fatalf("ledger entries len = %d, want 3", got)
		}
	})
}

func TestCargoFailedAddDoesNotReserveReferenceKey(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	input := validCargoAddItemInput(t)
	input.ItemDefinition = validWeightedStackableDefinition(t, 3)
	input.Quantity = 2
	input.CargoCapacityUnits = 5
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:retry-cargo")

	_, err := service.AddItem(input)
	if !errors.Is(err, ErrCargoCapacityExceeded) {
		t.Fatalf("Cargo AddItem error = %v, want ErrCargoCapacityExceeded", err)
	}

	input.CargoCapacityUnits = 6
	result, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("retry Cargo AddItem with same reference: %v", err)
	}
	if result.Duplicate {
		t.Fatal("retry Cargo AddItem Duplicate = true, want false")
	}
	if got := len(inventory.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestReserveItemsFailedAttemptDoesNotReserveReferenceOrReservationID(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 2, fromLocation, "loot_pickup:retry-reserve-seed-1")

	input := validReserveItemsInput(t)
	input.ReservationID = "craft-reservation-retry"
	input.ReservedLocationID = "craft-job-retry"
	input.ReferenceKey = validReferenceKey(t, "craft_complete:retry-reserve")
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:   definition,
			Quantity:     3,
			FromLocation: fromLocation,
		},
	}

	_, err := reservations.ReserveItems(input)
	if !errors.Is(err, ErrInsufficientItemQuantity) {
		t.Fatalf("ReserveItems error = %v, want ErrInsufficientItemQuantity", err)
	}
	if got := len(reservations.reservations); got != 0 {
		t.Fatalf("reservations len after failure = %d, want 0", got)
	}

	addStackableItems(t, inventory, definition, 1, fromLocation, "loot_pickup:retry-reserve-seed-2")
	result, err := reservations.ReserveItems(input)
	if err != nil {
		t.Fatalf("retry ReserveItems with same reference: %v", err)
	}
	if result.Duplicate {
		t.Fatal("retry ReserveItems Duplicate = true, want false")
	}
	if got := len(reservations.reservations); got != 1 {
		t.Fatalf("reservations len = %d, want 1", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 4 {
		t.Fatalf("ledger entries len = %d, want 4", got)
	}
}

func TestReserveItemsReferenceDoesNotCollideWithPriorMoveReference(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	account := validLocation(t)
	cargo := validShipCargoLocation(t)
	sharedReference := validReferenceKey(t, "loot_pickup:shared-reservation-reference")
	addStackableItems(t, inventory, definition, 10, account, "loot_pickup:shared-reservation-seed")

	moveResult, err := inventory.MoveItem(MoveItemInput{
		PlayerID: "player-1",
		ItemRef: MoveItemRef{
			Definition: definition,
		},
		FromLocation: account,
		ToLocation:   cargo,
		Quantity:     2,
		Reason:       "inventory_move",
		ReferenceKey: sharedReference,
	})
	if err != nil {
		t.Fatalf("MoveItem setup: %v", err)
	}
	if moveResult.Duplicate {
		t.Fatal("MoveItem setup Duplicate = true, want false")
	}

	input := validReserveItemsInput(t)
	input.ReservationID = "craft-reservation-shared-reference"
	input.ReservedLocationID = "craft-job-shared-reference"
	input.ReferenceKey = sharedReference
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:   definition,
			Quantity:     3,
			FromLocation: account,
		},
	}
	result, err := reservations.ReserveItems(input)
	if err != nil {
		t.Fatalf("ReserveItems with prior MoveItem reference: %v", err)
	}

	reserveReference, err := reserveItemMoveReference(sharedReference, 0, 1)
	if err != nil {
		t.Fatalf("reserveItemMoveReference: %v", err)
	}
	if result.Duplicate {
		t.Fatal("ReserveItems Duplicate = true, want false")
	}
	if got := result.Moves[0].LedgerEntries[0].ReferenceKey; got != reserveReference {
		t.Fatalf("reserve move reference = %q, want %q", got, reserveReference)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 5 {
		t.Fatalf("ledger entries len = %d, want 5", got)
	}
}

func TestReleaseReservationFailedAttemptCanSucceedAfterStateCorrection(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 5, fromLocation, "loot_pickup:retry-release-seed")

	input := validReserveItemsInput(t)
	input.ReservationID = "craft-reservation-release-retry"
	input.ReservedLocationID = "craft-job-release-retry"
	input.ReferenceKey = validReferenceKey(t, "craft_complete:retry-release")
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:   definition,
			Quantity:     5,
			FromLocation: fromLocation,
		},
	}
	if _, err := reservations.ReserveItems(input); err != nil {
		t.Fatalf("ReserveItems: %v", err)
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	setStackableQuantityForTest(t, inventory, input.PlayerID, definition, reservedLocation, 4)
	ledgerCountBefore := len(inventory.ItemLedgerEntries())
	_, err := reservations.ReleaseReservation(input.ReservationID)
	if !errors.Is(err, ErrInsufficientItemQuantity) {
		t.Fatalf("ReleaseReservation error = %v, want ErrInsufficientItemQuantity", err)
	}
	if got := reservations.reservations[input.ReservationID].State; got != ReservationStateActive {
		t.Fatalf("reservation state after failed release = %q, want %q", got, ReservationStateActive)
	}
	if got := len(inventory.ItemLedgerEntries()); got != ledgerCountBefore {
		t.Fatalf("ledger entries after failed release = %d, want %d", got, ledgerCountBefore)
	}

	setStackableQuantityForTest(t, inventory, input.PlayerID, definition, reservedLocation, 5)
	result, err := reservations.ReleaseReservation(input.ReservationID)
	if err != nil {
		t.Fatalf("retry ReleaseReservation after state correction: %v", err)
	}
	if result.Duplicate {
		t.Fatal("retry ReleaseReservation Duplicate = true, want false")
	}
	if result.Reservation.State != ReservationStateReleased {
		t.Fatalf("reservation state = %q, want %q", result.Reservation.State, ReservationStateReleased)
	}
	if got := len(inventory.ItemLedgerEntries()); got != ledgerCountBefore+2 {
		t.Fatalf("ledger entries after successful release = %d, want %d", got, ledgerCountBefore+2)
	}
}

func TestCommitReservationFailedAttemptCanSucceedAfterStateCorrection(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 5, fromLocation, "loot_pickup:retry-commit-seed")

	input := validReserveItemsInput(t)
	input.ReservationID = "craft-reservation-commit-retry"
	input.ReservedLocationID = "craft-job-commit-retry"
	input.ReferenceKey = validReferenceKey(t, "craft_complete:retry-commit")
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:   definition,
			Quantity:     5,
			FromLocation: fromLocation,
		},
	}
	if _, err := reservations.ReserveItems(input); err != nil {
		t.Fatalf("ReserveItems: %v", err)
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	setStackableQuantityForTest(t, inventory, input.PlayerID, definition, reservedLocation, 4)
	ledgerCountBefore := len(inventory.ItemLedgerEntries())
	_, err := reservations.CommitReservation(input.ReservationID)
	if !errors.Is(err, ErrInsufficientItemQuantity) {
		t.Fatalf("CommitReservation error = %v, want ErrInsufficientItemQuantity", err)
	}
	if got := reservations.reservations[input.ReservationID].State; got != ReservationStateActive {
		t.Fatalf("reservation state after failed commit = %q, want %q", got, ReservationStateActive)
	}
	if got := len(inventory.ItemLedgerEntries()); got != ledgerCountBefore {
		t.Fatalf("ledger entries after failed commit = %d, want %d", got, ledgerCountBefore)
	}

	setStackableQuantityForTest(t, inventory, input.PlayerID, definition, reservedLocation, 5)
	result, err := reservations.CommitReservation(input.ReservationID)
	if err != nil {
		t.Fatalf("retry CommitReservation after state correction: %v", err)
	}
	if result.Duplicate {
		t.Fatal("retry CommitReservation Duplicate = true, want false")
	}
	if result.Reservation.State != ReservationStateCommitted {
		t.Fatalf("reservation state = %q, want %q", result.Reservation.State, ReservationStateCommitted)
	}
	systemSink := validLocationKind(t, LocationKindSystemSink, input.ReservationID.String())
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, systemSink); got != 5 {
		t.Fatalf("system sink TotalItemQuantity() = %d, want 5", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != ledgerCountBefore+2 {
		t.Fatalf("ledger entries after successful commit = %d, want %d", got, ledgerCountBefore+2)
	}
}

func setStackableQuantityForTest(
	t *testing.T,
	service *InventoryService,
	playerID foundation.PlayerID,
	definition ItemDefinition,
	location ItemLocation,
	quantity int64,
) {
	t.Helper()

	quantityValue := validQuantity(t, quantity)
	service.mu.Lock()
	defer service.mu.Unlock()
	for index := range service.stackableItems {
		if matchesStackableDefinitionLocation(service.stackableItems[index], playerID, definition, location) {
			service.stackableItems[index].Quantity = quantityValue
			return
		}
	}
	t.Fatalf("stackable item %q at %v not found", definition.ItemID, location)
}
