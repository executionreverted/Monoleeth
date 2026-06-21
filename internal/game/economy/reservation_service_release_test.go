package economy

import (
	"errors"
	"testing"
)

func TestReleaseReservationMovesStackableItemsBackToOriginalLocationAndWritesLedger(t *testing.T) {
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

	result, err := reservations.ReleaseReservation(input.ReservationID)
	if err != nil {
		t.Fatalf("ReleaseReservation: %v", err)
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	if result.Duplicate {
		t.Fatal("ReleaseReservation Duplicate = true, want false")
	}
	if result.Reservation.State != ReservationStateReleased {
		t.Fatalf("reservation state = %q, want %q", result.Reservation.State, ReservationStateReleased)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 12 {
		t.Fatalf("source TotalItemQuantity() = %d, want 12", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 0 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(result.Moves); got != 1 {
		t.Fatalf("moves len = %d, want 1", got)
	}
	if got := len(result.Moves[0].LedgerEntries); got != 2 {
		t.Fatalf("release move ledger entries len = %d, want 2", got)
	}

	entries := inventory.ItemLedgerEntries()
	if len(entries) != 5 {
		t.Fatalf("ledger entries len = %d, want 5", len(entries))
	}
	releaseReference := validReferenceKey(t, "craft_complete:job-1-release")
	reservedEntry := entries[3]
	if reservedEntry.Action != LedgerActionDecrease {
		t.Fatalf("release reserved action = %q, want %q", reservedEntry.Action, LedgerActionDecrease)
	}
	if reservedEntry.Location != reservedLocation {
		t.Fatalf("release reserved location = %v, want %v", reservedEntry.Location, reservedLocation)
	}
	if reservedEntry.BalanceAfter != 0 {
		t.Fatalf("release reserved balance after = %d, want 0", reservedEntry.BalanceAfter)
	}
	if reservedEntry.Reason != releaseReservationReason {
		t.Fatalf("release reserved reason = %q, want %q", reservedEntry.Reason, releaseReservationReason)
	}
	if reservedEntry.ReferenceKey != releaseReference {
		t.Fatalf("release reserved reference = %q, want %q", reservedEntry.ReferenceKey, releaseReference)
	}
	sourceEntry := entries[4]
	if sourceEntry.Action != LedgerActionIncrease {
		t.Fatalf("release source action = %q, want %q", sourceEntry.Action, LedgerActionIncrease)
	}
	if sourceEntry.Location != fromLocation {
		t.Fatalf("release source location = %v, want %v", sourceEntry.Location, fromLocation)
	}
	if sourceEntry.BalanceAfter != 12 {
		t.Fatalf("release source balance after = %d, want 12", sourceEntry.BalanceAfter)
	}
	if sourceEntry.ReferenceKey != releaseReference {
		t.Fatalf("release source reference = %q, want %q", sourceEntry.ReferenceKey, releaseReference)
	}
}

func TestReleaseReservationMovesInstanceItemsBackToOriginalLocation(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validInstanceDefinition(t)
	fromLocation := validLocation(t)
	addResult := addInstanceItems(t, inventory, definition, 1, fromLocation, "loot_pickup:drop-1")
	instanceID := addResult.InstanceItems[0].ItemInstanceID

	input := validReserveItemsInput(t)
	input.Kind = ReservationKindAuction
	input.ReservationID = "auction-reservation-1"
	input.ReservedLocationID = "auction-1"
	input.ReferenceKey = validReferenceKey(t, "auction_close:auction-1")
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:     definition,
			ItemInstanceID: instanceID,
			Quantity:       1,
			FromLocation:   fromLocation,
		},
	}
	if _, err := reservations.ReserveItems(input); err != nil {
		t.Fatalf("ReserveItems: %v", err)
	}

	result, err := reservations.ReleaseReservation(input.ReservationID)
	if err != nil {
		t.Fatalf("ReleaseReservation: %v", err)
	}

	reservedLocation := validLocationKind(t, LocationKindAuctionEscrow, "auction-1")
	if result.Reservation.State != ReservationStateReleased {
		t.Fatalf("reservation state = %q, want %q", result.Reservation.State, ReservationStateReleased)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 1 {
		t.Fatalf("source TotalItemQuantity() = %d, want 1", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 0 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 0", got)
	}
	items := inventory.InstanceItems()
	if len(items) != 1 {
		t.Fatalf("instance items len = %d, want 1", len(items))
	}
	if items[0].Location != fromLocation {
		t.Fatalf("instance location = %v, want %v", items[0].Location, fromLocation)
	}
	for _, entry := range result.Moves[0].LedgerEntries {
		if entry.ItemInstanceID != instanceID {
			t.Fatalf("release ledger instance id = %q, want %q", entry.ItemInstanceID, instanceID)
		}
	}
}

func TestReleaseReservationDuplicateDoesNotMoveOrLedgerTwice(t *testing.T) {
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

	first, err := reservations.ReleaseReservation(input.ReservationID)
	if err != nil {
		t.Fatalf("first ReleaseReservation: %v", err)
	}
	second, err := reservations.ReleaseReservation(input.ReservationID)
	if err != nil {
		t.Fatalf("duplicate ReleaseReservation: %v", err)
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	if first.Duplicate {
		t.Fatal("first ReleaseReservation Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate ReleaseReservation Duplicate = false, want true")
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 10 {
		t.Fatalf("source TotalItemQuantity() = %d, want 10", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 0 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 5 {
		t.Fatalf("ledger entries len = %d, want 5", got)
	}
	if second.Moves[0].LedgerEntries[0].LedgerID != first.Moves[0].LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate release ledger id = %q, want %q", second.Moves[0].LedgerEntries[0].LedgerID, first.Moves[0].LedgerEntries[0].LedgerID)
	}
}

func TestReleaseReservationMissingReturnsClearError(t *testing.T) {
	reservations := NewReservationService(newTestInventoryService())

	_, err := reservations.ReleaseReservation("missing-reservation")
	if !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("ReleaseReservation error = %v, want ErrReservationNotFound", err)
	}
}

func TestReleaseReservationCommittedAndExpiredDoNotReleaseAssets(t *testing.T) {
	cases := []struct {
		name  string
		state ReservationState
	}{
		{name: "committed", state: ReservationStateCommitted},
		{name: "expired", state: ReservationStateExpired},
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
			reservation := reservations.reservations[input.ReservationID]
			reservation.State = tc.state
			reservations.reservations[input.ReservationID] = reservation

			_, err := reservations.ReleaseReservation(input.ReservationID)
			if !errors.Is(err, ErrReservationNotActive) {
				t.Fatalf("ReleaseReservation error = %v, want ErrReservationNotActive", err)
			}

			reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
			if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 5 {
				t.Fatalf("source TotalItemQuantity() = %d, want 5", got)
			}
			if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 5 {
				t.Fatalf("reserved TotalItemQuantity() = %d, want 5", got)
			}
			if got := len(inventory.ItemLedgerEntries()); got != 3 {
				t.Fatalf("ledger entries len = %d, want 3", got)
			}
		})
	}
}

func TestReleaseReservationInsufficientReservedQuantityLeavesReservationAndLedgerUnchanged(t *testing.T) {
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

	_, err := reservations.ReleaseReservation(input.ReservationID)
	if !errors.Is(err, ErrInsufficientItemQuantity) {
		t.Fatalf("ReleaseReservation error = %v, want ErrInsufficientItemQuantity", err)
	}

	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 3 {
		t.Fatalf("source TotalItemQuantity() = %d, want 3", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 4 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 4", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != ledgerCountBefore {
		t.Fatalf("ledger entries len = %d, want %d", got, ledgerCountBefore)
	}
	if got := reservations.reservations[input.ReservationID].State; got != ReservationStateActive {
		t.Fatalf("reservation state = %q, want %q", got, ReservationStateActive)
	}
}
