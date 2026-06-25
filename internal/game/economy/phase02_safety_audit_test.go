package economy

import (
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestPhase02SafetyReserveItemsRejectsNegativeQuantityWithoutLedger(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	input := validReserveItemsInput(t)
	input.Requirements[0].Quantity = -1

	_, err := reservations.ReserveItems(input)
	if !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("ReserveItems error = %v, want foundation.ErrNonPositiveAmount", err)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 0 {
		t.Fatalf("ledger entries len = %d, want 0", got)
	}
	if got := len(reservations.reservations); got != 0 {
		t.Fatalf("reservations len = %d, want 0", got)
	}
}

func TestPhase02ReserveItemsRollbackRestoresPartialMoveFailure(t *testing.T) {
	inventory := newTestInventoryService()
	definition := validStackableDefinition(t)
	sourceLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 9, sourceLocation, "loot_pickup:reserve-rollback-seed")

	input := validReserveItemsInput(t)
	input.ReservationID = "craft-reservation-reserve-rollback"
	input.ReservedLocationID = "craft-job-reserve-rollback"
	input.ReferenceKey = validReferenceKey(t, "craft_complete:reserve-rollback")
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:   definition,
			Quantity:     4,
			FromLocation: sourceLocation,
		},
		{
			Definition:   definition,
			Quantity:     3,
			FromLocation: sourceLocation,
		},
	}
	validated, err := input.validate()
	if err != nil {
		t.Fatalf("validate reserve input: %v", err)
	}
	if len(validated.moveInputs) != 2 {
		t.Fatalf("move inputs len = %d, want 2", len(validated.moveInputs))
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	firstReference := inventoryReferenceKey{
		playerID:     input.PlayerID,
		operation:    moveItemOperation,
		referenceKey: validated.moveInputs[0].ReferenceKey,
	}
	secondReference := inventoryReferenceKey{
		playerID:     input.PlayerID,
		operation:    moveItemOperation,
		referenceKey: validated.moveInputs[1].ReferenceKey,
	}

	inventory.mu.Lock()
	inventory.moveItemReferences[secondReference] = MoveItemResult{}
	_, err = inventory.moveReservationItemsWithRollbackLocked("reserve line", validated.moveInputs, validated.quantities, inventory.clock.Now())
	if !errors.Is(err, ErrReservationMoveReferenceExists) {
		inventory.mu.Unlock()
		t.Fatalf("moveReservationItemsWithRollbackLocked error = %v, want ErrReservationMoveReferenceExists", err)
	}
	if got := inventory.totalItemQuantityLocked(input.PlayerID, definition.ItemID, sourceLocation); got != 9 {
		inventory.mu.Unlock()
		t.Fatalf("source quantity after restore = %d, want 9", got)
	}
	if got := inventory.totalItemQuantityLocked(input.PlayerID, definition.ItemID, reservedLocation); got != 0 {
		inventory.mu.Unlock()
		t.Fatalf("reserved quantity after restore = %d, want 0", got)
	}
	if got := len(inventory.itemLedgerEntries); got != 1 {
		inventory.mu.Unlock()
		t.Fatalf("ledger entries after restore = %d, want 1", got)
	}
	if _, ok := inventory.moveItemReferences[firstReference]; ok {
		inventory.mu.Unlock()
		t.Fatalf("first reserve reference %q survived rollback restore", firstReference.referenceKey)
	}
	if _, ok := inventory.moveItemReferences[secondReference]; !ok {
		inventory.mu.Unlock()
		t.Fatalf("pre-existing second reserve reference %q was removed by rollback restore", secondReference.referenceKey)
	}
	delete(inventory.moveItemReferences, secondReference)
	inventory.mu.Unlock()

	reservations := NewReservationService(inventory)
	result, err := reservations.ReserveItems(input)
	if err != nil {
		t.Fatalf("ReserveItems after rollback restore: %v", err)
	}
	if result.Duplicate {
		t.Fatal("ReserveItems Duplicate = true, want false")
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, sourceLocation); got != 2 {
		t.Fatalf("source TotalItemQuantity() = %d, want 2", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 7 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 7", got)
	}
}

func TestPhase02RollbackSnapshotRestoresReservationReleaseAndCommitMutations(t *testing.T) {
	cases := []struct {
		name       string
		moveInputs func(*ReservationService, Reservation) ([]MoveItemInput, []foundation.Quantity, error)
		finish     func(*testing.T, *ReservationService, ReservationID) int
		assertDone func(*testing.T, *InventoryService, foundation.PlayerID, foundation.ItemID, ItemLocation, ItemLocation, ReservationID)
	}{
		{
			name: "release",
			moveInputs: func(service *ReservationService, reservation Reservation) ([]MoveItemInput, []foundation.Quantity, error) {
				return service.releaseMoveInputsLocked(reservation)
			},
			finish: func(t *testing.T, service *ReservationService, reservationID ReservationID) int {
				t.Helper()
				result, err := service.ReleaseReservation(reservationID)
				if err != nil {
					t.Fatalf("ReleaseReservation after rollback restore: %v", err)
				}
				if result.Duplicate {
					t.Fatal("ReleaseReservation Duplicate = true, want false")
				}
				if result.Reservation.State != ReservationStateReleased {
					t.Fatalf("reservation state = %q, want %q", result.Reservation.State, ReservationStateReleased)
				}
				return len(result.Moves) * 2
			},
			assertDone: func(t *testing.T, inventory *InventoryService, playerID foundation.PlayerID, itemID foundation.ItemID, sourceLocation ItemLocation, reservedLocation ItemLocation, reservationID ReservationID) {
				t.Helper()
				if got := inventory.TotalItemQuantity(playerID, itemID, sourceLocation); got != 9 {
					t.Fatalf("source TotalItemQuantity() = %d, want 9", got)
				}
				if got := inventory.TotalItemQuantity(playerID, itemID, reservedLocation); got != 0 {
					t.Fatalf("reserved TotalItemQuantity() = %d, want 0", got)
				}
			},
		},
		{
			name: "commit",
			moveInputs: func(service *ReservationService, reservation Reservation) ([]MoveItemInput, []foundation.Quantity, error) {
				return service.commitMoveInputsLocked(reservation)
			},
			finish: func(t *testing.T, service *ReservationService, reservationID ReservationID) int {
				t.Helper()
				result, err := service.CommitReservation(reservationID)
				if err != nil {
					t.Fatalf("CommitReservation after rollback restore: %v", err)
				}
				if result.Duplicate {
					t.Fatal("CommitReservation Duplicate = true, want false")
				}
				if result.Reservation.State != ReservationStateCommitted {
					t.Fatalf("reservation state = %q, want %q", result.Reservation.State, ReservationStateCommitted)
				}
				return len(result.Moves) * 2
			},
			assertDone: func(t *testing.T, inventory *InventoryService, playerID foundation.PlayerID, itemID foundation.ItemID, sourceLocation ItemLocation, reservedLocation ItemLocation, reservationID ReservationID) {
				t.Helper()
				systemSink := validLocationKind(t, LocationKindSystemSink, reservationID.String())
				if got := inventory.TotalItemQuantity(playerID, itemID, sourceLocation); got != 2 {
					t.Fatalf("source TotalItemQuantity() = %d, want 2", got)
				}
				if got := inventory.TotalItemQuantity(playerID, itemID, reservedLocation); got != 0 {
					t.Fatalf("reserved TotalItemQuantity() = %d, want 0", got)
				}
				if got := inventory.TotalItemQuantity(playerID, itemID, systemSink); got != 7 {
					t.Fatalf("system sink TotalItemQuantity() = %d, want 7", got)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inventory := newTestInventoryService()
			reservations := NewReservationService(inventory)
			definition := validStackableDefinition(t)
			sourceLocation := validLocation(t)
			addStackableItems(t, inventory, definition, 9, sourceLocation, "loot_pickup:rollback-seed-"+tc.name)

			input := validReserveItemsInput(t)
			input.ReservationID = ReservationID("craft-reservation-rollback-" + tc.name)
			input.ReservedLocationID = LocationID("craft-job-rollback-" + tc.name)
			input.ReferenceKey = validReferenceKey(t, "craft_complete:rollback-"+tc.name)
			input.Requirements = []ReserveItemRequirement{
				{
					Definition:   definition,
					Quantity:     4,
					FromLocation: sourceLocation,
				},
				{
					Definition:   definition,
					Quantity:     3,
					FromLocation: sourceLocation,
				},
			}
			if _, err := reservations.ReserveItems(input); err != nil {
				t.Fatalf("ReserveItems: %v", err)
			}

			reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
			ledgerCountBefore := len(inventory.ItemLedgerEntries())
			sourceBefore := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, sourceLocation)
			reservedBefore := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation)

			reservation := reservations.reservations[input.ReservationID]
			moveInputs, quantities, err := tc.moveInputs(reservations, reservation)
			if err != nil {
				t.Fatalf("build %s move inputs: %v", tc.name, err)
			}
			if len(moveInputs) != 2 {
				t.Fatalf("move inputs len = %d, want 2", len(moveInputs))
			}

			inventory.mu.Lock()
			snapshot := inventory.snapshotReservationMutationLocked()
			now := inventory.clock.Now()
			firstMove, err := inventory.moveItemValidatedLocked(moveInputs[0], quantities[0], now, true)
			if err != nil {
				inventory.mu.Unlock()
				t.Fatalf("first simulated %s move: %v", tc.name, err)
			}
			if firstMove.Duplicate {
				inventory.mu.Unlock()
				t.Fatalf("first simulated %s move Duplicate = true, want false", tc.name)
			}
			if got := len(inventory.itemLedgerEntries); got != ledgerCountBefore+2 {
				inventory.mu.Unlock()
				t.Fatalf("ledger entries after simulated %s line = %d, want %d", tc.name, got, ledgerCountBefore+2)
			}

			inventory.restoreReservationMutationLocked(snapshot)
			if got := len(inventory.itemLedgerEntries); got != ledgerCountBefore {
				inventory.mu.Unlock()
				t.Fatalf("ledger entries after restore = %d, want %d", got, ledgerCountBefore)
			}
			if got := inventory.totalItemQuantityLocked(input.PlayerID, definition.ItemID, sourceLocation); got != sourceBefore {
				inventory.mu.Unlock()
				t.Fatalf("source quantity after restore = %d, want %d", got, sourceBefore)
			}
			if got := inventory.totalItemQuantityLocked(input.PlayerID, definition.ItemID, reservedLocation); got != reservedBefore {
				inventory.mu.Unlock()
				t.Fatalf("reserved quantity after restore = %d, want %d", got, reservedBefore)
			}
			reference := inventoryReferenceKey{
				playerID:     input.PlayerID,
				operation:    moveItemOperation,
				referenceKey: moveInputs[0].ReferenceKey,
			}
			if _, ok := inventory.moveItemReferences[reference]; ok {
				inventory.mu.Unlock()
				t.Fatalf("%s move reference %q survived rollback restore", tc.name, moveInputs[0].ReferenceKey)
			}
			inventory.mu.Unlock()

			ledgerEntriesAdded := tc.finish(t, reservations, input.ReservationID)
			if got := len(inventory.ItemLedgerEntries()); got != ledgerCountBefore+ledgerEntriesAdded {
				t.Fatalf("ledger entries after %s = %d, want %d", tc.name, got, ledgerCountBefore+ledgerEntriesAdded)
			}
			tc.assertDone(t, inventory, input.PlayerID, definition.ItemID, sourceLocation, reservedLocation, input.ReservationID)
		})
	}
}

func TestPhase02OverflowRejectedByCreditWalletAndTransferCurrency(t *testing.T) {
	const maxInt64 = int64(1<<63 - 1)

	t.Run("credit wallet", func(t *testing.T) {
		service := newTestWalletService()
		setWalletBalanceForPhase02Audit(t, service, "player-1", CurrencyBucketCredits, maxInt64-1)
		input := validCreditWalletInput(t, "quest_reward:overflow-credit")
		input.Amount = 2

		_, err := service.CreditWallet(input)
		if !errors.Is(err, ErrWalletBalanceOverflow) {
			t.Fatalf("CreditWallet error = %v, want ErrWalletBalanceOverflow", err)
		}
		if got := service.Balance("player-1", CurrencyBucketCredits); got != maxInt64-1 {
			t.Fatalf("Balance() = %d, want %d", got, maxInt64-1)
		}
		if got := len(service.CurrencyLedgerEntries()); got != 0 {
			t.Fatalf("ledger entries len = %d, want 0", got)
		}
	})

	t.Run("transfer currency", func(t *testing.T) {
		service := newTestWalletService()
		setWalletBalanceForPhase02Audit(t, service, "player-1", CurrencyBucketCredits, 10)
		setWalletBalanceForPhase02Audit(t, service, "player-2", CurrencyBucketCredits, maxInt64-1)
		input := validTransferCurrencyInput(t, "quest_reward:overflow-transfer")
		input.Amount = 2

		_, err := service.TransferCurrency(input)
		if !errors.Is(err, ErrWalletBalanceOverflow) {
			t.Fatalf("TransferCurrency error = %v, want ErrWalletBalanceOverflow", err)
		}
		if got := service.Balance("player-1", CurrencyBucketCredits); got != 10 {
			t.Fatalf("from Balance() = %d, want 10", got)
		}
		if got := service.Balance("player-2", CurrencyBucketCredits); got != maxInt64-1 {
			t.Fatalf("to Balance() = %d, want %d", got, maxInt64-1)
		}
		if got := len(service.CurrencyLedgerEntries()); got != 0 {
			t.Fatalf("ledger entries len = %d, want 0", got)
		}
	})
}

func setWalletBalanceForPhase02Audit(
	t *testing.T,
	service *WalletService,
	playerID foundation.PlayerID,
	currency CurrencyBucket,
	amount int64,
) {
	t.Helper()

	balance := WalletBalance{
		PlayerID:  playerID,
		Currency:  currency,
		Balance:   amount,
		UpdatedAt: testWalletNow,
	}
	if err := balance.Validate(); err != nil {
		t.Fatalf("test wallet balance: %v", err)
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	service.balances[walletBalanceKey{playerID: playerID, currency: currency}] = balance
}
