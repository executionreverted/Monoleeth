package economy

import (
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestReserveItemsMovesStackableRequirementsToReservedLocationByKind(t *testing.T) {
	cases := []struct {
		name                 string
		kind                 ReservationKind
		reservationID        ReservationID
		reservedLocationID   LocationID
		reference            string
		wantReservedLocation LocationKind
	}{
		{
			name:                 "craft",
			kind:                 ReservationKindCraft,
			reservationID:        "craft-reservation-1",
			reservedLocationID:   "craft-job-1",
			reference:            "craft_complete:job-1",
			wantReservedLocation: LocationKindCraftingReserved,
		},
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
			addStackableItems(t, inventory, definition, 12, fromLocation, "loot_pickup:drop-1")

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

			result, err := reservations.ReserveItems(input)
			if err != nil {
				t.Fatalf("ReserveItems: %v", err)
			}

			reservedLocation := validLocationKind(t, tc.wantReservedLocation, tc.reservedLocationID.String())
			if result.Duplicate {
				t.Fatal("ReserveItems Duplicate = true, want false")
			}
			if result.Reservation.ReservationID != input.ReservationID {
				t.Fatalf("reservation id = %q, want %q", result.Reservation.ReservationID, input.ReservationID)
			}
			if result.Reservation.Kind != input.Kind {
				t.Fatalf("reservation kind = %q, want %q", result.Reservation.Kind, input.Kind)
			}
			if result.Reservation.State != ReservationStateActive {
				t.Fatalf("reservation state = %q, want %q", result.Reservation.State, ReservationStateActive)
			}
			if result.Reservation.CreatedAt != testInventoryNow {
				t.Fatalf("reservation created at = %s, want %s", result.Reservation.CreatedAt, testInventoryNow)
			}
			if len(result.Reservation.ItemLines) != 1 {
				t.Fatalf("reservation item lines len = %d, want 1", len(result.Reservation.ItemLines))
			}
			line := result.Reservation.ItemLines[0]
			if line.ItemID != definition.ItemID {
				t.Fatalf("reservation item id = %q, want %q", line.ItemID, definition.ItemID)
			}
			if got := line.Quantity.Int64(); got != 5 {
				t.Fatalf("reservation line quantity = %d, want 5", got)
			}
			if line.FromLocation != fromLocation {
				t.Fatalf("reservation from location = %v, want %v", line.FromLocation, fromLocation)
			}
			if line.ReservedLocation != reservedLocation {
				t.Fatalf("reservation reserved location = %v, want %v", line.ReservedLocation, reservedLocation)
			}
			if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 7 {
				t.Fatalf("source TotalItemQuantity() = %d, want 7", got)
			}
			if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 5 {
				t.Fatalf("reserved TotalItemQuantity() = %d, want 5", got)
			}
			if got := len(result.Moves); got != 1 {
				t.Fatalf("moves len = %d, want 1", got)
			}
			if got := len(result.Moves[0].LedgerEntries); got != 2 {
				t.Fatalf("move ledger entries len = %d, want 2", got)
			}

			entries := inventory.ItemLedgerEntries()
			if len(entries) != 3 {
				t.Fatalf("ledger entries len = %d, want 3", len(entries))
			}
			sourceEntry := entries[1]
			if sourceEntry.Action != LedgerActionDecrease {
				t.Fatalf("source action = %q, want %q", sourceEntry.Action, LedgerActionDecrease)
			}
			if sourceEntry.Location != fromLocation {
				t.Fatalf("source ledger location = %v, want %v", sourceEntry.Location, fromLocation)
			}
			if sourceEntry.ReferenceKey != input.ReferenceKey {
				t.Fatalf("source ledger reference = %q, want %q", sourceEntry.ReferenceKey, input.ReferenceKey)
			}
			reservedEntry := entries[2]
			if reservedEntry.Action != LedgerActionIncrease {
				t.Fatalf("reserved action = %q, want %q", reservedEntry.Action, LedgerActionIncrease)
			}
			if reservedEntry.Location != reservedLocation {
				t.Fatalf("reserved ledger location = %v, want %v", reservedEntry.Location, reservedLocation)
			}
			if reservedEntry.ReferenceKey != input.ReferenceKey {
				t.Fatalf("reserved ledger reference = %q, want %q", reservedEntry.ReferenceKey, input.ReferenceKey)
			}
		})
	}
}

func TestReserveItemsDuplicateReferenceDoesNotReserveOrLedgerTwice(t *testing.T) {
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

	first, err := reservations.ReserveItems(input)
	if err != nil {
		t.Fatalf("first ReserveItems: %v", err)
	}
	second, err := reservations.ReserveItems(input)
	if err != nil {
		t.Fatalf("duplicate ReserveItems: %v", err)
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	if first.Duplicate {
		t.Fatal("first ReserveItems Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate ReserveItems Duplicate = false, want true")
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 6 {
		t.Fatalf("source TotalItemQuantity() = %d, want 6", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 4 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 4", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 3 {
		t.Fatalf("ledger entries len = %d, want 3", got)
	}
	if got := len(reservations.reservations); got != 1 {
		t.Fatalf("reservations len = %d, want 1", got)
	}
	if second.Reservation.ReservationID != first.Reservation.ReservationID {
		t.Fatalf("duplicate reservation id = %q, want %q", second.Reservation.ReservationID, first.Reservation.ReservationID)
	}
	if second.Moves[0].LedgerEntries[0].LedgerID != first.Moves[0].LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate source ledger id = %q, want %q", second.Moves[0].LedgerEntries[0].LedgerID, first.Moves[0].LedgerEntries[0].LedgerID)
	}
}

func TestReserveItemsInsufficientQuantityDoesNotCreateReservationOrLedgerOnlyState(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 5, fromLocation, "loot_pickup:drop-1")

	input := validReserveItemsInput(t)
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:   definition,
			Quantity:     6,
			FromLocation: fromLocation,
		},
	}

	_, err := reservations.ReserveItems(input)
	if !errors.Is(err, ErrInsufficientItemQuantity) {
		t.Fatalf("ReserveItems error = %v, want ErrInsufficientItemQuantity", err)
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 5 {
		t.Fatalf("source TotalItemQuantity() = %d, want 5", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 0 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
	if got := len(reservations.reservations); got != 0 {
		t.Fatalf("reservations len = %d, want 0", got)
	}
}

func TestReserveItemsAggregatesStackableRequirementsBeforeMoving(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 10, fromLocation, "loot_pickup:drop-1")

	input := validReserveItemsInput(t)
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:   definition,
			Quantity:     6,
			FromLocation: fromLocation,
		},
		{
			Definition:   definition,
			Quantity:     5,
			FromLocation: fromLocation,
		},
	}

	_, err := reservations.ReserveItems(input)
	if !errors.Is(err, ErrInsufficientItemQuantity) {
		t.Fatalf("ReserveItems error = %v, want ErrInsufficientItemQuantity", err)
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 10 {
		t.Fatalf("source TotalItemQuantity() = %d, want 10", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 0 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
	if got := len(reservations.reservations); got != 0 {
		t.Fatalf("reservations len = %d, want 0", got)
	}
}

func TestReserveItemsMovesMultipleStackableLinesWithDerivedMoveReferences(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	addStackableItems(t, inventory, definition, 10, fromLocation, "loot_pickup:drop-1")

	input := validReserveItemsInput(t)
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:   definition,
			Quantity:     6,
			FromLocation: fromLocation,
		},
		{
			Definition:   definition,
			Quantity:     4,
			FromLocation: fromLocation,
		},
	}

	result, err := reservations.ReserveItems(input)
	if err != nil {
		t.Fatalf("ReserveItems: %v", err)
	}

	reservedLocation := validLocationKind(t, LocationKindCraftingReserved, input.ReservedLocationID.String())
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 0 {
		t.Fatalf("source TotalItemQuantity() = %d, want 0", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 10 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 10", got)
	}
	if got := len(result.Moves); got != 2 {
		t.Fatalf("moves len = %d, want 2", got)
	}
	if result.Moves[0].LedgerEntries[0].ReferenceKey == result.Moves[1].LedgerEntries[0].ReferenceKey {
		t.Fatalf("line move references both = %q, want distinct", result.Moves[0].LedgerEntries[0].ReferenceKey)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 5 {
		t.Fatalf("ledger entries len = %d, want 5", got)
	}
}

func TestReserveItemsMovesInstanceRequirement(t *testing.T) {
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

	result, err := reservations.ReserveItems(input)
	if err != nil {
		t.Fatalf("ReserveItems: %v", err)
	}

	reservedLocation := validLocationKind(t, LocationKindAuctionEscrow, "auction-1")
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 0 {
		t.Fatalf("source TotalItemQuantity() = %d, want 0", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 1 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 1", got)
	}
	items := inventory.InstanceItems()
	if len(items) != 1 {
		t.Fatalf("instance items len = %d, want 1", len(items))
	}
	if items[0].Location != reservedLocation {
		t.Fatalf("instance location = %v, want %v", items[0].Location, reservedLocation)
	}
	if result.Reservation.ItemLines[0].ItemInstanceID != instanceID {
		t.Fatalf("reservation instance id = %q, want %q", result.Reservation.ItemLines[0].ItemInstanceID, instanceID)
	}
	for _, entry := range result.Moves[0].LedgerEntries {
		if entry.ItemInstanceID != instanceID {
			t.Fatalf("ledger instance id = %q, want %q", entry.ItemInstanceID, instanceID)
		}
	}
}

func TestReserveItemsRejectsReservedSourceWithoutMutation(t *testing.T) {
	inventory := newTestInventoryService()
	reservations := NewReservationService(inventory)
	definition := validStackableDefinition(t)
	fromLocation := validLocationKind(t, LocationKindCraftingReserved, "craft-job-1")
	addStackableItems(t, inventory, definition, 5, fromLocation, "loot_pickup:drop-1")

	input := validReserveItemsInput(t)
	input.Kind = ReservationKindMarket
	input.ReservationID = "market-reservation-1"
	input.ReservedLocationID = "listing-1"
	input.ReferenceKey = validReferenceKey(t, "market_buy:listing-1:player-1:request-1")
	input.Requirements = []ReserveItemRequirement{
		{
			Definition:   definition,
			Quantity:     1,
			FromLocation: fromLocation,
		},
	}

	_, err := reservations.ReserveItems(input)
	if !errors.Is(err, ErrBlockedGenericMoveSource) {
		t.Fatalf("ReserveItems error = %v, want ErrBlockedGenericMoveSource", err)
	}

	reservedLocation := validLocationKind(t, LocationKindMarketEscrow, "listing-1")
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 5 {
		t.Fatalf("source TotalItemQuantity() = %d, want 5", got)
	}
	if got := inventory.TotalItemQuantity(input.PlayerID, definition.ItemID, reservedLocation); got != 0 {
		t.Fatalf("reserved TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(inventory.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
	if got := len(reservations.reservations); got != 0 {
		t.Fatalf("reservations len = %d, want 0", got)
	}
}

func TestReserveItemsRejectsZeroQuantityAndMissingRequiredInputs(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*ReserveItemsInput)
		wantErr error
	}{
		{
			name: "zero quantity",
			mutate: func(input *ReserveItemsInput) {
				input.Requirements[0].Quantity = 0
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "blank player",
			mutate: func(input *ReserveItemsInput) {
				input.PlayerID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "blank reference",
			mutate: func(input *ReserveItemsInput) {
				input.ReferenceKey = ""
			},
			wantErr: foundation.ErrEmptyIdempotencyKey,
		},
		{
			name: "empty requirements",
			mutate: func(input *ReserveItemsInput) {
				input.Requirements = nil
			},
			wantErr: ErrEmptyReservationAssets,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inventory := newTestInventoryService()
			reservations := NewReservationService(inventory)
			input := validReserveItemsInput(t)
			tc.mutate(&input)

			_, err := reservations.ReserveItems(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ReserveItems error = %v, want %v", err, tc.wantErr)
			}
			if got := len(inventory.ItemLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

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

func validReserveItemsInput(t *testing.T) ReserveItemsInput {
	t.Helper()

	return ReserveItemsInput{
		ReservationID:      "craft-reservation-1",
		Kind:               ReservationKindCraft,
		PlayerID:           "player-1",
		ReservedLocationID: "craft-job-1",
		Reason:             "reserve_items",
		ReferenceKey:       validReferenceKey(t, "craft_complete:job-1"),
		Requirements: []ReserveItemRequirement{
			{
				Definition:   validStackableDefinition(t),
				Quantity:     1,
				FromLocation: validLocation(t),
			},
		},
	}
}
