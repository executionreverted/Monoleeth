package economy

import (
	"encoding/json"
	"errors"
	"testing"

	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestInventoryServiceEmitsItemAddedAndLedgerEvents(t *testing.T) {
	service := newTestInventoryService()
	recorder := testutil.NewEventRecorder()
	service.SetEventEmitter(recorder)
	input := validAddItemInput(t)

	result, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	recorded := recorder.Events()
	testutil.AssertEventTypes(t, recorded, EventInventoryItemAdded, EventLedgerEntryCreated)
	assertEventClock(t, recorded[0], testInventoryNow.UnixMilli(), 1)
	assertEventClock(t, recorded[1], testInventoryNow.UnixMilli(), 2)

	added := decodeEventPayload[InventoryItemAddedPayload](t, recorded[0])
	if added.PlayerID != input.PlayerID {
		t.Fatalf("added player = %q, want %q", added.PlayerID, input.PlayerID)
	}
	if added.ItemID != input.ItemDefinition.ItemID {
		t.Fatalf("added item = %q, want %q", added.ItemID, input.ItemDefinition.ItemID)
	}
	if got := added.Quantity; got != input.Quantity {
		t.Fatalf("added quantity = %d, want %d", got, input.Quantity)
	}
	if added.Location != input.Location {
		t.Fatalf("added location = %v, want %v", added.Location, input.Location)
	}
	if added.Reason != input.Reason {
		t.Fatalf("added reason = %q, want %q", added.Reason, input.Reason)
	}
	if added.ReferenceKey != input.ReferenceKey {
		t.Fatalf("added reference = %q, want %q", added.ReferenceKey, input.ReferenceKey)
	}
	if added.LedgerID != result.LedgerEntry.LedgerID {
		t.Fatalf("added ledger = %q, want %q", added.LedgerID, result.LedgerEntry.LedgerID)
	}
	if len(added.ItemInstanceIDs) != len(result.StackableItems) {
		t.Fatalf("added item instance ids len = %d, want %d", len(added.ItemInstanceIDs), len(result.StackableItems))
	}
	if added.ItemInstanceIDs[0] != result.StackableItems[0].ItemInstanceID {
		t.Fatalf("added item instance id = %q, want %q", added.ItemInstanceIDs[0], result.StackableItems[0].ItemInstanceID)
	}

	ledger := decodeEventPayload[LedgerEntryCreatedPayload](t, recorded[1])
	assertItemLedgerPayload(t, ledger, result.LedgerEntry)
}

func TestInventoryServiceEmitsItemMovedRemovedAndLedgerEvents(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	toLocation := validStationStorageLocation(t)
	addStackableItems(t, service, definition, 75, fromLocation, "loot_pickup:event-seed")

	recorder := testutil.NewEventRecorder()
	service.SetEventEmitter(recorder)

	moveInput := validMoveItemInput(t)
	moveInput.ItemRef.Definition = definition
	moveInput.FromLocation = fromLocation
	moveInput.ToLocation = toLocation
	moveInput.Quantity = 30
	moveInput.ReferenceKey = validReferenceKey(t, "loot_pickup:event-move")

	moveResult, err := service.MoveItem(moveInput)
	if err != nil {
		t.Fatalf("MoveItem: %v", err)
	}

	recorded := recorder.Events()
	testutil.AssertEventTypes(t, recorded, EventInventoryItemMoved, EventLedgerEntryCreated, EventLedgerEntryCreated)
	moved := decodeEventPayload[InventoryItemMovedPayload](t, recorded[0])
	if moved.PlayerID != moveInput.PlayerID || moved.ItemID != definition.ItemID {
		t.Fatalf("moved asset = (%q,%q), want (%q,%q)", moved.PlayerID, moved.ItemID, moveInput.PlayerID, definition.ItemID)
	}
	if moved.Quantity != moveInput.Quantity {
		t.Fatalf("moved quantity = %d, want %d", moved.Quantity, moveInput.Quantity)
	}
	if moved.FromLocation != fromLocation || moved.ToLocation != toLocation {
		t.Fatalf("moved locations = %v -> %v, want %v -> %v", moved.FromLocation, moved.ToLocation, fromLocation, toLocation)
	}
	if moved.Reason != moveInput.Reason || moved.ReferenceKey != moveInput.ReferenceKey {
		t.Fatalf("moved reason/reference = %q/%q, want %q/%q", moved.Reason, moved.ReferenceKey, moveInput.Reason, moveInput.ReferenceKey)
	}
	if len(moved.LedgerIDs) != len(moveResult.LedgerEntries) {
		t.Fatalf("moved ledger ids len = %d, want %d", len(moved.LedgerIDs), len(moveResult.LedgerEntries))
	}
	for index, entry := range moveResult.LedgerEntries {
		if moved.LedgerIDs[index] != entry.LedgerID {
			t.Fatalf("moved ledger id %d = %q, want %q", index, moved.LedgerIDs[index], entry.LedgerID)
		}
		assertItemLedgerPayload(t, decodeEventPayload[LedgerEntryCreatedPayload](t, recorded[index+1]), entry)
	}

	recorder.Reset()
	removeInput := validRemoveItemInput(t)
	removeInput.ItemRef.Definition = definition
	removeInput.SourceLocation = toLocation
	removeInput.Quantity = 10
	removeInput.ReferenceKey = validReferenceKey(t, "loot_pickup:event-remove")

	removeResult, err := service.RemoveItem(removeInput)
	if err != nil {
		t.Fatalf("RemoveItem: %v", err)
	}

	recorded = recorder.Events()
	testutil.AssertEventTypes(t, recorded, EventInventoryItemRemoved, EventLedgerEntryCreated)
	removed := decodeEventPayload[InventoryItemRemovedPayload](t, recorded[0])
	if removed.PlayerID != removeInput.PlayerID || removed.ItemID != definition.ItemID {
		t.Fatalf("removed asset = (%q,%q), want (%q,%q)", removed.PlayerID, removed.ItemID, removeInput.PlayerID, definition.ItemID)
	}
	if removed.Quantity != removeInput.Quantity {
		t.Fatalf("removed quantity = %d, want %d", removed.Quantity, removeInput.Quantity)
	}
	if removed.Location != toLocation {
		t.Fatalf("removed location = %v, want %v", removed.Location, toLocation)
	}
	if removed.Reason != removeInput.Reason || removed.ReferenceKey != removeInput.ReferenceKey {
		t.Fatalf("removed reason/reference = %q/%q, want %q/%q", removed.Reason, removed.ReferenceKey, removeInput.Reason, removeInput.ReferenceKey)
	}
	if removed.LedgerID != removeResult.LedgerEntries[0].LedgerID {
		t.Fatalf("removed ledger = %q, want %q", removed.LedgerID, removeResult.LedgerEntries[0].LedgerID)
	}
	assertItemLedgerPayload(t, decodeEventPayload[LedgerEntryCreatedPayload](t, recorded[1]), removeResult.LedgerEntries[0])
}

func TestInventoryServiceDoesNotEmitOnValidationFailureOrDuplicate(t *testing.T) {
	service := newTestInventoryService()
	recorder := testutil.NewEventRecorder()
	service.SetEventEmitter(recorder)

	invalid := validAddItemInput(t)
	invalid.Quantity = 0
	if _, err := service.AddItem(invalid); !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("AddItem invalid error = %v, want foundation.ErrNonPositiveAmount", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder)

	input := validAddItemInput(t)
	if _, err := service.AddItem(input); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder, EventInventoryItemAdded, EventLedgerEntryCreated)

	recorder.Reset()
	duplicate, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("duplicate AddItem: %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate AddItem Duplicate = false, want true")
	}
	testutil.AssertRecordedEventTypes(t, recorder)

	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	toLocation := validStationStorageLocation(t)
	addStackableItems(t, service, definition, 10, fromLocation, "loot_pickup:event-no-emit-move-seed")
	recorder.Reset()

	moveInput := validMoveItemInput(t)
	moveInput.ItemRef.Definition = definition
	moveInput.FromLocation = fromLocation
	moveInput.ToLocation = toLocation
	moveInput.Quantity = 4
	moveInput.ReferenceKey = validReferenceKey(t, "loot_pickup:event-no-emit-move")
	if _, err := service.MoveItem(moveInput); err != nil {
		t.Fatalf("MoveItem: %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder, EventInventoryItemMoved, EventLedgerEntryCreated, EventLedgerEntryCreated)

	recorder.Reset()
	duplicateMove, err := service.MoveItem(moveInput)
	if err != nil {
		t.Fatalf("duplicate MoveItem: %v", err)
	}
	if !duplicateMove.Duplicate {
		t.Fatal("duplicate MoveItem Duplicate = false, want true")
	}
	testutil.AssertRecordedEventTypes(t, recorder)

	recorder.Reset()
	invalidMove := moveInput
	invalidMove.ReferenceKey = validReferenceKey(t, "loot_pickup:event-no-emit-move-invalid")
	invalidMove.Quantity = 100
	if _, err := service.MoveItem(invalidMove); !errors.Is(err, ErrInsufficientItemQuantity) {
		t.Fatalf("invalid MoveItem error = %v, want ErrInsufficientItemQuantity", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder)

	removeInput := validRemoveItemInput(t)
	removeInput.ItemRef.Definition = definition
	removeInput.SourceLocation = toLocation
	removeInput.Quantity = 2
	removeInput.ReferenceKey = validReferenceKey(t, "loot_pickup:event-no-emit-remove")
	if _, err := service.RemoveItem(removeInput); err != nil {
		t.Fatalf("RemoveItem: %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder, EventInventoryItemRemoved, EventLedgerEntryCreated)

	recorder.Reset()
	duplicateRemove, err := service.RemoveItem(removeInput)
	if err != nil {
		t.Fatalf("duplicate RemoveItem: %v", err)
	}
	if !duplicateRemove.Duplicate {
		t.Fatal("duplicate RemoveItem Duplicate = false, want true")
	}
	testutil.AssertRecordedEventTypes(t, recorder)

	recorder.Reset()
	invalidRemove := removeInput
	invalidRemove.ReferenceKey = validReferenceKey(t, "loot_pickup:event-no-emit-remove-invalid")
	invalidRemove.Quantity = 100
	if _, err := service.RemoveItem(invalidRemove); !errors.Is(err, ErrInsufficientItemQuantity) {
		t.Fatalf("invalid RemoveItem error = %v, want ErrInsufficientItemQuantity", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder)
}

func TestCargoServiceEmitsCargoUpdatedAfterSuccessfulAdd(t *testing.T) {
	inventory := newTestInventoryService()
	service := NewCargoService(inventory)
	recorder := testutil.NewEventRecorder()
	inventory.SetEventEmitter(recorder)
	service.SetEventEmitter(recorder)
	input := validCargoAddItemInput(t)

	result, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	recorded := recorder.Events()
	testutil.AssertEventTypes(t, recorded, EventInventoryItemAdded, EventLedgerEntryCreated, EventCargoUpdated)
	updated := decodeEventPayload[CargoUpdatedPayload](t, recorded[2])
	if updated.PlayerID != input.PlayerID || updated.ItemID != input.ItemDefinition.ItemID {
		t.Fatalf("cargo asset = (%q,%q), want (%q,%q)", updated.PlayerID, updated.ItemID, input.PlayerID, input.ItemDefinition.ItemID)
	}
	if updated.Quantity != input.Quantity {
		t.Fatalf("cargo quantity = %d, want %d", updated.Quantity, input.Quantity)
	}
	if updated.Location != input.ActiveCargo {
		t.Fatalf("cargo location = %v, want %v", updated.Location, input.ActiveCargo)
	}
	if updated.Reason != input.Reason || updated.ReferenceKey != input.ReferenceKey {
		t.Fatalf("cargo reason/reference = %q/%q, want %q/%q", updated.Reason, updated.ReferenceKey, input.Reason, input.ReferenceKey)
	}
	if updated.LedgerID != result.LedgerEntry.LedgerID {
		t.Fatalf("cargo ledger = %q, want %q", updated.LedgerID, result.LedgerEntry.LedgerID)
	}

	recorder.Reset()
	duplicate, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("duplicate AddItem: %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate cargo AddItem Duplicate = false, want true")
	}
	testutil.AssertRecordedEventTypes(t, recorder)

	overCapacity := input
	overCapacity.ReferenceKey = validReferenceKey(t, "loot_pickup:cargo-over-capacity")
	overCapacity.CargoCapacityUnits = 1
	if _, err := service.AddItem(overCapacity); !errors.Is(err, ErrCargoCapacityExceeded) {
		t.Fatalf("over capacity AddItem error = %v, want ErrCargoCapacityExceeded", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder)
}

func TestWalletServiceEmitsCreditDebitTransferAndLedgerEvents(t *testing.T) {
	t.Run("credit", func(t *testing.T) {
		service := newTestWalletService()
		recorder := testutil.NewEventRecorder()
		service.SetEventEmitter(recorder)
		input := validCreditWalletInput(t, "quest_reward:event-wallet-credit")

		result, err := service.CreditWallet(input)
		if err != nil {
			t.Fatalf("CreditWallet: %v", err)
		}

		recorded := recorder.Events()
		testutil.AssertEventTypes(t, recorded, EventWalletCredited, EventLedgerEntryCreated)
		credited := decodeEventPayload[WalletMutationPayload](t, recorded[0])
		assertWalletMutationPayload(t, credited, input.PlayerID, input.Currency, input.Amount, result.Balance.Balance, input.Reason, input.ReferenceKey, result.LedgerEntry.LedgerID)
		assertCurrencyLedgerPayload(t, decodeEventPayload[LedgerEntryCreatedPayload](t, recorded[1]), result.LedgerEntry)

		recorder.Reset()
		invalid := input
		invalid.ReferenceKey = validReferenceKey(t, "quest_reward:event-wallet-credit-invalid")
		invalid.Amount = 0
		if _, err := service.CreditWallet(invalid); !errors.Is(err, foundation.ErrNonPositiveAmount) {
			t.Fatalf("invalid CreditWallet error = %v, want foundation.ErrNonPositiveAmount", err)
		}
		testutil.AssertRecordedEventTypes(t, recorder)

		duplicate, err := service.CreditWallet(input)
		if err != nil {
			t.Fatalf("duplicate CreditWallet: %v", err)
		}
		if !duplicate.Duplicate {
			t.Fatal("duplicate CreditWallet Duplicate = false, want true")
		}
		testutil.AssertRecordedEventTypes(t, recorder)
	})

	t.Run("debit", func(t *testing.T) {
		service := newTestWalletService()
		creditWalletForTest(t, service, "player-1", CurrencyBucketCredits, 500, "quest_reward:event-wallet-debit-seed")
		recorder := testutil.NewEventRecorder()
		service.SetEventEmitter(recorder)
		input := validDebitWalletInput(t, "quest_reward:event-wallet-debit")

		result, err := service.DebitWallet(input)
		if err != nil {
			t.Fatalf("DebitWallet: %v", err)
		}

		recorded := recorder.Events()
		testutil.AssertEventTypes(t, recorded, EventWalletDebited, EventLedgerEntryCreated)
		debited := decodeEventPayload[WalletMutationPayload](t, recorded[0])
		assertWalletMutationPayload(t, debited, input.PlayerID, input.Currency, input.Amount, result.Balance.Balance, input.Reason, input.ReferenceKey, result.LedgerEntry.LedgerID)
		assertCurrencyLedgerPayload(t, decodeEventPayload[LedgerEntryCreatedPayload](t, recorded[1]), result.LedgerEntry)

		recorder.Reset()
		invalid := input
		invalid.ReferenceKey = validReferenceKey(t, "quest_reward:event-wallet-debit-invalid")
		invalid.Amount = 0
		if _, err := service.DebitWallet(invalid); !errors.Is(err, foundation.ErrNonPositiveAmount) {
			t.Fatalf("invalid DebitWallet error = %v, want foundation.ErrNonPositiveAmount", err)
		}
		testutil.AssertRecordedEventTypes(t, recorder)

		recorder.Reset()
		duplicate, err := service.DebitWallet(input)
		if err != nil {
			t.Fatalf("duplicate DebitWallet: %v", err)
		}
		if !duplicate.Duplicate {
			t.Fatal("duplicate DebitWallet Duplicate = false, want true")
		}
		testutil.AssertRecordedEventTypes(t, recorder)
	})

	t.Run("transfer", func(t *testing.T) {
		service := newTestWalletService()
		creditWalletForTest(t, service, "player-1", CurrencyBucketCredits, 500, "quest_reward:event-wallet-transfer-seed")
		recorder := testutil.NewEventRecorder()
		service.SetEventEmitter(recorder)
		input := validTransferCurrencyInput(t, "quest_reward:event-wallet-transfer")

		result, err := service.TransferCurrency(input)
		if err != nil {
			t.Fatalf("TransferCurrency: %v", err)
		}

		recorded := recorder.Events()
		testutil.AssertEventTypes(t, recorded, EventWalletDebited, EventLedgerEntryCreated, EventWalletCredited, EventLedgerEntryCreated)
		debited := decodeEventPayload[WalletMutationPayload](t, recorded[0])
		assertWalletMutationPayload(t, debited, input.FromPlayerID, input.Currency, input.Amount, result.FromBalance.Balance, input.Reason, input.ReferenceKey, result.LedgerEntries[0].LedgerID)
		assertCurrencyLedgerPayload(t, decodeEventPayload[LedgerEntryCreatedPayload](t, recorded[1]), result.LedgerEntries[0])
		credited := decodeEventPayload[WalletMutationPayload](t, recorded[2])
		assertWalletMutationPayload(t, credited, input.ToPlayerID, input.Currency, input.Amount, result.ToBalance.Balance, input.Reason, input.ReferenceKey, result.LedgerEntries[1].LedgerID)
		assertCurrencyLedgerPayload(t, decodeEventPayload[LedgerEntryCreatedPayload](t, recorded[3]), result.LedgerEntries[1])

		recorder.Reset()
		invalid := input
		invalid.ReferenceKey = validReferenceKey(t, "quest_reward:event-wallet-transfer-invalid")
		invalid.ToPlayerID = invalid.FromPlayerID
		if _, err := service.TransferCurrency(invalid); !errors.Is(err, ErrWalletSelfTransfer) {
			t.Fatalf("invalid TransferCurrency error = %v, want ErrWalletSelfTransfer", err)
		}
		testutil.AssertRecordedEventTypes(t, recorder)

		recorder.Reset()
		duplicate, err := service.TransferCurrency(input)
		if err != nil {
			t.Fatalf("duplicate TransferCurrency: %v", err)
		}
		if !duplicate.Duplicate {
			t.Fatal("duplicate TransferCurrency Duplicate = false, want true")
		}
		testutil.AssertRecordedEventTypes(t, recorder)
	})
}

func TestReservationServiceEmitsInventoryMoveEvents(t *testing.T) {
	t.Run("reserve and release", func(t *testing.T) {
		inventory := newTestInventoryService()
		definition := validStackableDefinition(t)
		fromLocation := validLocation(t)
		addStackableItems(t, inventory, definition, 10, fromLocation, "loot_pickup:event-reservation-seed")
		reservations := NewReservationService(inventory)
		recorder := testutil.NewEventRecorder()
		inventory.SetEventEmitter(recorder)

		input := validReserveItemsInput(t)
		input.Requirements[0].Definition = definition
		input.Requirements[0].FromLocation = fromLocation
		if _, err := reservations.ReserveItems(input); err != nil {
			t.Fatalf("ReserveItems: %v", err)
		}
		testutil.AssertRecordedEventTypes(t, recorder, EventInventoryItemMoved, EventLedgerEntryCreated, EventLedgerEntryCreated)

		recorder.Reset()
		duplicate, err := reservations.ReserveItems(input)
		if err != nil {
			t.Fatalf("duplicate ReserveItems: %v", err)
		}
		if !duplicate.Duplicate {
			t.Fatal("duplicate ReserveItems Duplicate = false, want true")
		}
		testutil.AssertRecordedEventTypes(t, recorder)

		recorder.Reset()
		if _, err := reservations.ReleaseReservation(input.ReservationID); err != nil {
			t.Fatalf("ReleaseReservation: %v", err)
		}
		testutil.AssertRecordedEventTypes(t, recorder, EventInventoryItemMoved, EventLedgerEntryCreated, EventLedgerEntryCreated)

		recorder.Reset()
		duplicateRelease, err := reservations.ReleaseReservation(input.ReservationID)
		if err != nil {
			t.Fatalf("duplicate ReleaseReservation: %v", err)
		}
		if !duplicateRelease.Duplicate {
			t.Fatal("duplicate ReleaseReservation Duplicate = false, want true")
		}
		testutil.AssertRecordedEventTypes(t, recorder)
	})

	t.Run("commit", func(t *testing.T) {
		inventory := newTestInventoryService()
		definition := validStackableDefinition(t)
		fromLocation := validLocation(t)
		addStackableItems(t, inventory, definition, 10, fromLocation, "loot_pickup:event-reservation-commit-seed")
		reservations := NewReservationService(inventory)

		input := validReserveItemsInput(t)
		input.Requirements[0].Definition = definition
		input.Requirements[0].FromLocation = fromLocation
		if _, err := reservations.ReserveItems(input); err != nil {
			t.Fatalf("ReserveItems: %v", err)
		}

		recorder := testutil.NewEventRecorder()
		inventory.SetEventEmitter(recorder)
		if _, err := reservations.CommitReservation(input.ReservationID); err != nil {
			t.Fatalf("CommitReservation: %v", err)
		}
		testutil.AssertRecordedEventTypes(t, recorder, EventInventoryItemMoved, EventLedgerEntryCreated, EventLedgerEntryCreated)

		recorder.Reset()
		duplicateCommit, err := reservations.CommitReservation(input.ReservationID)
		if err != nil {
			t.Fatalf("duplicate CommitReservation: %v", err)
		}
		if !duplicateCommit.Duplicate {
			t.Fatal("duplicate CommitReservation Duplicate = false, want true")
		}
		testutil.AssertRecordedEventTypes(t, recorder)

		recorder.Reset()
		if _, err := reservations.CommitReservation("missing-reservation"); !errors.Is(err, ErrReservationNotFound) {
			t.Fatalf("missing CommitReservation error = %v, want ErrReservationNotFound", err)
		}
		testutil.AssertRecordedEventTypes(t, recorder)
	})
}

func decodeEventPayload[T any](t *testing.T, event events.EventEnvelope) T {
	t.Helper()

	var payload T
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal %s payload %s: %v", event.Type, event.Payload, err)
	}
	return payload
}

func assertEventClock(t *testing.T, event events.EventEnvelope, wantServerTime int64, wantSequence uint64) {
	t.Helper()

	if event.ServerTime != wantServerTime {
		t.Fatalf("%s server time = %d, want %d", event.Type, event.ServerTime, wantServerTime)
	}
	if event.Sequence != wantSequence {
		t.Fatalf("%s sequence = %d, want %d", event.Type, event.Sequence, wantSequence)
	}
}

func assertItemLedgerPayload(t *testing.T, payload LedgerEntryCreatedPayload, entry ItemLedgerEntry) {
	t.Helper()

	if payload.LedgerID != entry.LedgerID {
		t.Fatalf("ledger payload id = %q, want %q", payload.LedgerID, entry.LedgerID)
	}
	if payload.AssetType != ledgerAssetTypeItem {
		t.Fatalf("ledger payload asset type = %q, want %q", payload.AssetType, ledgerAssetTypeItem)
	}
	if payload.PlayerID != entry.PlayerID {
		t.Fatalf("ledger payload player = %q, want %q", payload.PlayerID, entry.PlayerID)
	}
	if payload.ItemID != entry.ItemID {
		t.Fatalf("ledger payload item = %q, want %q", payload.ItemID, entry.ItemID)
	}
	if payload.ItemInstanceID != entry.ItemInstanceID {
		t.Fatalf("ledger payload item instance = %q, want %q", payload.ItemInstanceID, entry.ItemInstanceID)
	}
	if payload.Quantity != entry.Quantity.Int64() {
		t.Fatalf("ledger payload quantity = %d, want %d", payload.Quantity, entry.Quantity.Int64())
	}
	if payload.Action != entry.Action {
		t.Fatalf("ledger payload action = %q, want %q", payload.Action, entry.Action)
	}
	if payload.BalanceAfter != entry.BalanceAfter {
		t.Fatalf("ledger payload balance after = %d, want %d", payload.BalanceAfter, entry.BalanceAfter)
	}
	if payload.Location == nil || *payload.Location != entry.Location {
		t.Fatalf("ledger payload location = %v, want %v", payload.Location, entry.Location)
	}
	if payload.Reason != entry.Reason {
		t.Fatalf("ledger payload reason = %q, want %q", payload.Reason, entry.Reason)
	}
	if payload.ReferenceKey != entry.ReferenceKey {
		t.Fatalf("ledger payload reference = %q, want %q", payload.ReferenceKey, entry.ReferenceKey)
	}
}

func assertCurrencyLedgerPayload(t *testing.T, payload LedgerEntryCreatedPayload, entry CurrencyLedgerEntry) {
	t.Helper()

	if payload.LedgerID != entry.LedgerID {
		t.Fatalf("ledger payload id = %q, want %q", payload.LedgerID, entry.LedgerID)
	}
	if payload.AssetType != ledgerAssetTypeCurrency {
		t.Fatalf("ledger payload asset type = %q, want %q", payload.AssetType, ledgerAssetTypeCurrency)
	}
	if payload.PlayerID != entry.PlayerID {
		t.Fatalf("ledger payload player = %q, want %q", payload.PlayerID, entry.PlayerID)
	}
	if payload.Currency != entry.Currency {
		t.Fatalf("ledger payload currency = %q, want %q", payload.Currency, entry.Currency)
	}
	if payload.Amount != entry.Amount.Int64() {
		t.Fatalf("ledger payload amount = %d, want %d", payload.Amount, entry.Amount.Int64())
	}
	if payload.Action != entry.Action {
		t.Fatalf("ledger payload action = %q, want %q", payload.Action, entry.Action)
	}
	if payload.BalanceAfter != entry.BalanceAfter {
		t.Fatalf("ledger payload balance after = %d, want %d", payload.BalanceAfter, entry.BalanceAfter)
	}
	if payload.Location != nil {
		t.Fatalf("currency ledger location = %v, want nil", payload.Location)
	}
	if payload.Reason != entry.Reason {
		t.Fatalf("ledger payload reason = %q, want %q", payload.Reason, entry.Reason)
	}
	if payload.ReferenceKey != entry.ReferenceKey {
		t.Fatalf("ledger payload reference = %q, want %q", payload.ReferenceKey, entry.ReferenceKey)
	}
}

func assertWalletMutationPayload(
	t *testing.T,
	payload WalletMutationPayload,
	playerID foundation.PlayerID,
	currency CurrencyBucket,
	amount int64,
	balanceAfter int64,
	reason LedgerReason,
	referenceKey foundation.IdempotencyKey,
	ledgerID LedgerID,
) {
	t.Helper()

	if payload.PlayerID != playerID {
		t.Fatalf("wallet payload player = %q, want %q", payload.PlayerID, playerID)
	}
	if payload.Currency != currency {
		t.Fatalf("wallet payload currency = %q, want %q", payload.Currency, currency)
	}
	if payload.Amount != amount {
		t.Fatalf("wallet payload amount = %d, want %d", payload.Amount, amount)
	}
	if payload.BalanceAfter != balanceAfter {
		t.Fatalf("wallet payload balance after = %d, want %d", payload.BalanceAfter, balanceAfter)
	}
	if payload.Reason != reason {
		t.Fatalf("wallet payload reason = %q, want %q", payload.Reason, reason)
	}
	if payload.ReferenceKey != referenceKey {
		t.Fatalf("wallet payload reference = %q, want %q", payload.ReferenceKey, referenceKey)
	}
	if payload.LedgerID != ledgerID {
		t.Fatalf("wallet payload ledger = %q, want %q", payload.LedgerID, ledgerID)
	}
}
