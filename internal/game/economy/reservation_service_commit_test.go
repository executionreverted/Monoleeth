package economy

import (
	"errors"
	"testing"
)

func TestCommitReservationCraftMovesReservedItemsToSystemSinkAndWritesLedger(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 12, fromLocation, "loot_pickup:drop-1")

	input := validReserveItemsInput(t)
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

	result, err := reservations.CommitReservation(input.ReservationID)
	if err != nil {
		t.Fatalf("CommitReservation: %v", err)
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	systemSink := validLocationKind(t, LocationKindSystemSink, input.ReservationID.String())
	if result.Duplicate {
		t.Fatal("CommitReservation Duplicate = true, want false")
	}
	if result.Reservation.State != ReservationStateCommitted {
		t.Fatalf("reservation state = %q, want %q", result.Reservation.State, ReservationStateCommitted)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 7 {
		t.Fatalf("source TotalItemQuantity() = %d, want 7", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 0 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 0", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, systemSink); got != 5 {
		t.Fatalf("system sink TotalItemQuantity() = %d, want 5", got)
	}
	if got := len(result.Moves); got != 1 {
		t.Fatalf("moves len = %d, want 1", got)
	}
	if got := len(result.Moves[0].LedgerEntries); got != 2 {
		t.Fatalf("commit move ledger entries len = %d, want 2", got)
	}

	entries := inventory.ItemLedgerEntries()
	if len(entries) != 5 {
		t.Fatalf("ledger entries len = %d, want 5", len(entries))
	}
	commitReference := validReferenceKey(t, "craft_complete:job-1-commit")
	reservedEntry := entries[3]
	if reservedEntry.Action != LedgerActionDecrease {
		t.Fatalf("commit reserved action = %q, want %q", reservedEntry.Action, LedgerActionDecrease)
	}
	if reservedEntry.Location != reservedLocation {
		t.Fatalf("commit reserved location = %v, want %v", reservedEntry.Location, reservedLocation)
	}
	if reservedEntry.BalanceAfter != 0 {
		t.Fatalf("commit reserved balance after = %d, want 0", reservedEntry.BalanceAfter)
	}
	if reservedEntry.Reason != commitReservationReason {
		t.Fatalf("commit reserved reason = %q, want %q", reservedEntry.Reason, commitReservationReason)
	}
	if reservedEntry.ReferenceKey != commitReference {
		t.Fatalf("commit reserved reference = %q, want %q", reservedEntry.ReferenceKey, commitReference)
	}
	sinkEntry := entries[4]
	if sinkEntry.Action != LedgerActionIncrease {
		t.Fatalf("commit sink action = %q, want %q", sinkEntry.Action, LedgerActionIncrease)
	}
	if sinkEntry.Location != systemSink {
		t.Fatalf("commit sink location = %v, want %v", sinkEntry.Location, systemSink)
	}
	if sinkEntry.BalanceAfter != 5 {
		t.Fatalf("commit sink balance after = %d, want 5", sinkEntry.BalanceAfter)
	}
	if sinkEntry.ReferenceKey != commitReference {
		t.Fatalf("commit sink reference = %q, want %q", sinkEntry.ReferenceKey, commitReference)
	}
}

func TestCommitReservationDuplicateDoesNotMoveOrLedgerTwiceAndBlocksLaterRelease(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 10, fromLocation, "loot_pickup:drop-1")

	input := validReserveItemsInput(t)
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:   definition,
			Quantity:     4,
			FromLocation: fromLocation,
		},
	}
	if _, err := reservations.ReserveItems(input); err != nil {
		t.Fatalf("ReserveItems: %v", err)
	}

	first, err := reservations.CommitReservation(input.ReservationID)
	if err != nil {
		t.Fatalf("first CommitReservation: %v", err)
	}
	second, err := reservations.CommitReservation(input.ReservationID)
	if err != nil {
		t.Fatalf("duplicate CommitReservation: %v", err)
	}
	_, releaseErr := reservations.ReleaseReservation(input.ReservationID)
	if !errors.Is(releaseErr, ErrReservationNotActive) {
		t.Fatalf("ReleaseReservation after commit error = %v, want ErrReservationNotActive", releaseErr)
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	systemSink := validLocationKind(t, LocationKindSystemSink, input.ReservationID.String())
	if first.Duplicate {
		t.Fatal("first CommitReservation Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate CommitReservation Duplicate = false, want true")
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 6 {
		t.Fatalf("source TotalItemQuantity() = %d, want 6", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 0 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 0", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, systemSink); got != 4 {
		t.Fatalf("system sink TotalItemQuantity() = %d, want 4", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 5 {
		t.Fatalf("ledger entries len = %d, want 5", got)
	}
	if second.Moves[0].LedgerEntries[0].LedgerID != first.Moves[0].LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate commit ledger id = %q, want %q", second.Moves[0].LedgerEntries[0].LedgerID, first.Moves[0].LedgerEntries[0].LedgerID)
	}
	if got := reservations.reservations[input.ReservationID].State; got != ReservationStateCommitted {
		t.Fatalf("reservation state = %q, want %q", got, ReservationStateCommitted)
	}
}

func TestCommitReservationMarketAndAuctionMarkCommittedWithoutEscrowMovement(t *testing.T) {
	cases := []struct {
		name                 string
		kind                 ReservationKind
		reservationID        ReservationID
		reservedLocationID   LocationID
		reference            string
		wantReservedLocation LocationKind
	}{
		{
			name:                 "market",
			kind:                 ReservationKindMarket,
			reservationID:        "market-reservation-1",
			reservedLocationID:   "listing-1",
			reference:            "market_buy:listing-1:player-1:request-1",
			wantReservedLocation: LocationKindMarketEscrow,
		},
		{
			name:                 "auction",
			kind:                 ReservationKindAuction,
			reservationID:        "auction-reservation-1",
			reservedLocationID:   "auction-1",
			reference:            "auction_close:auction-1",
			wantReservedLocation: LocationKindAuctionEscrow,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inventory := newTestInventoryService()
			reservations := NewReservationService(inventory)
			definition := validStackableDefinition(t)
			fromLocation := validLocation(t)
			addStackableItems(t, inventory, definition, 10, fromLocation, "loot_pickup:drop-1")

			input := validReserveItemsInput(t)
			input.Kind = tc.kind
			input.ReservationID = tc.reservationID
			input.ReservedLocationID = tc.reservedLocationID
			input.ReferenceKey = validReferenceKey(t, tc.reference)
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
			ledgerCountBefore := len(inventory.ItemLedgerEntries())

			result, err := reservations.CommitReservation(input.ReservationID)
			if err != nil {
				t.Fatalf("CommitReservation: %v", err)
			}

			reservedLocation := validLocationKind(t, tc.wantReservedLocation, tc.reservedLocationID.String())
			systemSink := validLocationKind(t, LocationKindSystemSink, input.ReservationID.String())
			if result.Reservation.State != ReservationStateCommitted {
				t.Fatalf("reservation state = %q, want %q", result.Reservation.State, ReservationStateCommitted)
			}
			if len(result.Moves) != 0 {
				t.Fatalf("moves len = %d, want 0", len(result.Moves))
			}
			if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 5 {
				t.Fatalf("source TotalItemQuantity() = %d, want 5", got)
			}
			if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 5 {
				t.Fatalf("reserved TotalItemQuantity() = %d, want 5", got)
			}
			if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, systemSink); got != 0 {
				t.Fatalf("system sink TotalItemQuantity() = %d, want 0", got)
			}
			if got := len(inventory.ItemLedgerEntries()); got != ledgerCountBefore {
				t.Fatalf("ledger entries len = %d, want %d", got, ledgerCountBefore)
			}
		})
	}
}

func TestCommitReservationMissingReturnsClearError(t *testing.T) {
	reservations := NewReservationService(newTestInventoryService())

	_, err := reservations.CommitReservation("missing-reservation")
	if !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("CommitReservation error = %v, want ErrReservationNotFound", err)
	}
}

func TestCommitReservationReleasedAndExpiredDoNotCommit(t *testing.T) {
	cases := []struct {
		name          string
		prepareState  func(*testing.T, *ReservationService, ReservationID)
		wantReserved  int64
		wantSource    int64
		wantLedgerLen int
	}{
		{
			name: "released",
			prepareState: func(t *testing.T, reservations *ReservationService, reservationID ReservationID) {
				t.Helper()
				if _, err := reservations.ReleaseReservation(reservationID); err != nil {
					t.Fatalf("ReleaseReservation: %v", err)
				}
			},
			wantReserved:  0,
			wantSource:    10,
			wantLedgerLen: 5,
		},
		{
			name: "expired",
			prepareState: func(t *testing.T, reservations *ReservationService, reservationID ReservationID) {
				t.Helper()
				reservation := reservations.reservations[reservationID]
				reservation.State = ReservationStateExpired
				reservations.reservations[reservationID] = reservation
			},
			wantReserved:  5,
			wantSource:    5,
			wantLedgerLen: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inventory := newTestInventoryService()
			reservations := NewReservationService(inventory)
			definition := validStackableDefinition(t)
			fromLocation := validLocation(t)
			addStackableItems(t, inventory, definition, 10, fromLocation, "loot_pickup:drop-1")

			input := validReserveItemsInput(t)
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
			tc.prepareState(t, reservations, input.ReservationID)

			_, err := reservations.CommitReservation(input.ReservationID)
			if !errors.Is(err, ErrReservationNotActive) {
				t.Fatalf("CommitReservation error = %v, want ErrReservationNotActive", err)
			}

			reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
			systemSink := validLocationKind(t, LocationKindSystemSink, input.ReservationID.String())
			if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != tc.wantSource {
				t.Fatalf("source TotalItemQuantity() = %d, want %d", got, tc.wantSource)
			}
			if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != tc.wantReserved {
				t.Fatalf("reserved TotalItemQuantity() = %d, want %d", got, tc.wantReserved)
			}
			if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, systemSink); got != 0 {
				t.Fatalf("system sink TotalItemQuantity() = %d, want 0", got)
			}
			if got := len(inventory.ItemLedgerEntries()); got != tc.wantLedgerLen {
				t.Fatalf("ledger entries len = %d, want %d", got, tc.wantLedgerLen)
			}
		})
	}
}

func TestCommitReservationInsufficientReservedQuantityLeavesReservationAndLedgerUnchanged(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 10, fromLocation, "loot_pickup:drop-1")

	input := validReserveItemsInput(t)
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:   definition,
			Quantity:     4,
			FromLocation: fromLocation,
		},
		{
			Definition:   definition,
			Quantity:     3,
			FromLocation: fromLocation,
		},
	}
	if _, err := reservations.ReserveItems(input); err != nil {
		t.Fatalf("ReserveItems: %v", err)
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	systemSink := validLocationKind(t, LocationKindSystemSink, input.ReservationID.String())
	corruptedQuantity := validQuantity(t, 4)
	corrupted := false
	inventory.mu.Lock()
	for index := range inventory.stackableItems {
		if matchesStackableDefinitionLocation(inventory.stackableItems[index], input.PlayerID, definition, reservedLocation) {
			inventory.stackableItems[index].Quantity = corruptedQuantity
			corrupted = true
			break
		}
	}
	inventory.mu.Unlock()
	if !corrupted {
		t.Fatal("failed to corrupt reserved stack quantity for test setup")
	}
	ledgerCountBefore := len(inventory.ItemLedgerEntries())

	_, err := reservations.CommitReservation(input.ReservationID)
	if !errors.Is(err, ErrInsufficientItemQuantity) {
		t.Fatalf("CommitReservation error = %v, want ErrInsufficientItemQuantity", err)
	}

	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 3 {
		t.Fatalf("source TotalItemQuantity() = %d, want 3", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 4 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 4", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, systemSink); got != 0 {
		t.Fatalf("system sink TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != ledgerCountBefore {
		t.Fatalf("ledger entries len = %d, want %d", got, ledgerCountBefore)
	}
	if got := reservations.reservations[input.ReservationID].State; got != ReservationStateActive {
		t.Fatalf("reservation state = %q, want %q", got, ReservationStateActive)
	}
}

func TestCommitReservationUsesInternalMoveWhileGenericMoveStillBlocksReservedSource(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 5, fromLocation, "loot_pickup:drop-1")

	input := validReserveItemsInput(t)
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
	systemSink := validLocationKind(t, LocationKindSystemSink, input.ReservationID.String())
	_, moveErr := inventory.MoveItem(MoveItemInput{
		PlayerID: input.PlayerID,
		ItemRef: MoveItemRef{
			Definition: definition,
		},
		FromLocation: reservedLocation,
		ToLocation:   systemSink,
		Quantity:     1,
		Reason:       "player_move_attempt",
		ReferenceKey: validReferenceKey(t, "craft_complete:job-1-player-move"),
	})
	if !errors.Is(moveErr, ErrBlockedGenericMoveSource) {
		t.Fatalf("MoveItem error = %v, want ErrBlockedGenericMoveSource", moveErr)
	}

	if _, err := reservations.CommitReservation(input.ReservationID); err != nil {
		t.Fatalf("CommitReservation: %v", err)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 0 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 0", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, systemSink); got != 5 {
		t.Fatalf("system sink TotalItemQuantity() = %d, want 5", got)
	}
}
