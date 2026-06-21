package economy

import (
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestMoveItemRejectsGenericMoveFromEscrowReservedAndSystemLocations(t *testing.T) {
	cases := []struct {
		name string
		kind LocationKind
	}{
		{name: "market escrow", kind: LocationKindMarketEscrow},
		{name: "auction escrow", kind: LocationKindAuctionEscrow},
		{name: "crafting reserved", kind: LocationKindCraftingReserved},
		{name: "system sink", kind: LocationKindSystemSink},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestInventoryService()
			definition := validStackableDefinition(t)
			fromLocation := validLocationKind(t, tc.kind, "reserved-1")
			toLocation := validLocation(t)
			addStackableItems(t, service, definition, 5, fromLocation, "loot_pickup:drop-1")

			input := validMoveItemInput(t)
			input.ItemRef.Definition = definition
			input.FromLocation = fromLocation
			input.ToLocation = toLocation
			input.Quantity = 1
			input.ReferenceKey = validReferenceKey(t, "loot_pickup:move-1")

			_, err := service.MoveItem(input)
			if !errors.Is(err, ErrBlockedGenericMoveSource) {
				t.Fatalf("MoveItem error = %v, want ErrBlockedGenericMoveSource", err)
			}
			if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 5 {
				t.Fatalf("source TotalItemQuantity() = %d, want 5", got)
			}
			if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, toLocation); got != 0 {
				t.Fatalf("destination TotalItemQuantity() = %d, want 0", got)
			}
			if got := len(service.ItemLedgerEntries()); got != 1 {
				t.Fatalf("ledger entries len = %d, want 1", got)
			}
		})
	}
}

func TestMoveItemRejectsGenericMoveFromShipEquipped(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	fromLocation := validShipEquippedLocation(t)
	toLocation := validLocation(t)
	seedStackableItem(t, service, definition, 5, fromLocation)

	input := validMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.ToLocation = toLocation
	input.Quantity = 1
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:move-from-equipped")

	_, err := service.MoveItem(input)
	if !errors.Is(err, ErrBlockedGenericMoveSource) {
		t.Fatalf("MoveItem error = %v, want ErrBlockedGenericMoveSource", err)
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

func TestMoveItemRejectsGenericMoveToBlockedTargets(t *testing.T) {
	cases := []struct {
		name     string
		location ItemLocation
	}{
		{name: "ship cargo", location: validShipCargoLocation(t)},
		{name: "ship equipped", location: validShipEquippedLocation(t)},
		{name: "market escrow", location: validLocationKind(t, LocationKindMarketEscrow, "listing-1")},
		{name: "auction escrow", location: validLocationKind(t, LocationKindAuctionEscrow, "auction-1")},
		{name: "crafting reserved", location: validLocationKind(t, LocationKindCraftingReserved, "craft-job-1")},
		{name: "system sink", location: validLocationKind(t, LocationKindSystemSink, "sink-1")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestInventoryService()
			definition := validStackableDefinition(t)
			fromLocation := validLocation(t)
			toLocation := tc.location
			addStackableItems(t, service, definition, 5, fromLocation, "loot_pickup:drop-1")

			input := validMoveItemInput(t)
			input.ItemRef.Definition = definition
			input.FromLocation = fromLocation
			input.ToLocation = toLocation
			input.Quantity = 1
			input.ReferenceKey = validReferenceKey(t, "loot_pickup:move-to-blocked-target")

			_, err := service.MoveItem(input)
			if !errors.Is(err, ErrBlockedGenericMoveTarget) {
				t.Fatalf("MoveItem error = %v, want ErrBlockedGenericMoveTarget", err)
			}
			if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 5 {
				t.Fatalf("source TotalItemQuantity() = %d, want 5", got)
			}
			if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, toLocation); got != 0 {
				t.Fatalf("destination TotalItemQuantity() = %d, want 0", got)
			}
			if got := len(service.ItemLedgerEntries()); got != 1 {
				t.Fatalf("ledger entries len = %d, want 1", got)
			}
		})
	}
}

func TestMoveItemRejectsGenericOwnerTransfer(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	toLocation := validStationStorageLocation(t)
	addStackableItems(t, service, definition, 5, fromLocation, "loot_pickup:generic-owner-transfer-seed")

	input := validMoveItemInput(t)
	input.ToPlayerID = "player-2"
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.ToLocation = toLocation
	input.Quantity = 1
	input.ReferenceKey = validReferenceKey(t, "market_buy:listing-1:player-2:generic-transfer")

	_, err := service.MoveItem(input)
	if !errors.Is(err, ErrBlockedGenericOwnerTransfer) {
		t.Fatalf("MoveItem owner transfer error = %v, want ErrBlockedGenericOwnerTransfer", err)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 5 {
		t.Fatalf("source TotalItemQuantity() = %d, want 5", got)
	}
	if got := service.TotalItemQuantity(input.ToPlayerID, definition.ItemID, toLocation); got != 0 {
		t.Fatalf("destination owner TotalItemQuantity() = %d, want 0", got)
	}
}

func TestSystemMoveItemMovesStackableToMarketEscrowAndBack(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	account := validLocation(t)
	escrow := validLocationKind(t, LocationKindMarketEscrow, "listing-1")
	addStackableItems(t, service, definition, 12, account, "loot_pickup:market-system-seed")

	toEscrow := validMoveItemInput(t)
	toEscrow.ItemRef.Definition = definition
	toEscrow.FromLocation = account
	toEscrow.ToLocation = escrow
	toEscrow.Quantity = 7
	toEscrow.Reason = "market_listing"
	toEscrow.ReferenceKey = validReferenceKey(t, "market_buy:listing-1:player-1:escrow-1")

	toEscrowResult, err := service.SystemMoveItem(toEscrow)
	if err != nil {
		t.Fatalf("SystemMoveItem to escrow: %v", err)
	}
	if toEscrowResult.Duplicate {
		t.Fatal("SystemMoveItem to escrow Duplicate = true, want false")
	}
	if got := service.TotalItemQuantity(toEscrow.PlayerID, definition.ItemID, account); got != 5 {
		t.Fatalf("account TotalItemQuantity() after escrow = %d, want 5", got)
	}
	if got := service.TotalItemQuantity(toEscrow.PlayerID, definition.ItemID, escrow); got != 7 {
		t.Fatalf("escrow TotalItemQuantity() after escrow = %d, want 7", got)
	}
	if got := len(toEscrowResult.LedgerEntries); got != 2 {
		t.Fatalf("to escrow ledger entries len = %d, want 2", got)
	}
	if toEscrowResult.LedgerEntries[0].Action != LedgerActionDecrease || toEscrowResult.LedgerEntries[0].Location != account {
		t.Fatalf("to escrow source ledger = %s at %v, want decrease at %v", toEscrowResult.LedgerEntries[0].Action, toEscrowResult.LedgerEntries[0].Location, account)
	}
	if toEscrowResult.LedgerEntries[1].Action != LedgerActionIncrease || toEscrowResult.LedgerEntries[1].Location != escrow {
		t.Fatalf("to escrow destination ledger = %s at %v, want increase at %v", toEscrowResult.LedgerEntries[1].Action, toEscrowResult.LedgerEntries[1].Location, escrow)
	}

	fromEscrow := toEscrow
	fromEscrow.FromLocation = escrow
	fromEscrow.ToLocation = account
	fromEscrow.Reason = "market_cancel"
	fromEscrow.ReferenceKey = validReferenceKey(t, "market_buy:listing-1:player-1:cancel-1")

	fromEscrowResult, err := service.SystemMoveItem(fromEscrow)
	if err != nil {
		t.Fatalf("SystemMoveItem from escrow: %v", err)
	}
	if got := service.TotalItemQuantity(fromEscrow.PlayerID, definition.ItemID, account); got != 12 {
		t.Fatalf("account TotalItemQuantity() after return = %d, want 12", got)
	}
	if got := service.TotalItemQuantity(fromEscrow.PlayerID, definition.ItemID, escrow); got != 0 {
		t.Fatalf("escrow TotalItemQuantity() after return = %d, want 0", got)
	}
	if got := len(fromEscrowResult.LedgerEntries); got != 2 {
		t.Fatalf("from escrow ledger entries len = %d, want 2", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 5 {
		t.Fatalf("ledger entries len = %d, want 5", got)
	}
}

func TestSystemMoveItemTransfersStackableOwnershipFromEscrowToBuyer(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	sellerAccount := validLocation(t)
	escrow := validLocationKind(t, LocationKindMarketEscrow, "listing-owner-transfer")
	buyerAccount := validLocationKind(t, LocationKindAccountInventory, "player-2")
	addStackableItems(t, service, definition, 5, sellerAccount, "loot_pickup:owner-transfer-seed")

	toEscrow := validMoveItemInput(t)
	toEscrow.ItemRef.Definition = definition
	toEscrow.FromLocation = sellerAccount
	toEscrow.ToLocation = escrow
	toEscrow.Quantity = 5
	toEscrow.Reason = "market_listing"
	toEscrow.ReferenceKey = validReferenceKey(t, "market_listing:listing-owner-transfer")
	if _, err := service.SystemMoveItem(toEscrow); err != nil {
		t.Fatalf("SystemMoveItem to escrow: %v", err)
	}

	toBuyer := validMoveItemInput(t)
	toBuyer.ToPlayerID = "player-2"
	toBuyer.ItemRef.Definition = definition
	toBuyer.FromLocation = escrow
	toBuyer.ToLocation = buyerAccount
	toBuyer.Quantity = 2
	toBuyer.Reason = "market_buy"
	toBuyer.ReferenceKey = validReferenceKey(t, "market_buy:listing-owner-transfer:player-2:buy-1")

	result, err := service.SystemMoveItem(toBuyer)
	if err != nil {
		t.Fatalf("SystemMoveItem to buyer: %v", err)
	}
	if got := service.TotalItemQuantity(toBuyer.PlayerID, definition.ItemID, escrow); got != 3 {
		t.Fatalf("seller escrow TotalItemQuantity() = %d, want 3", got)
	}
	if got := service.TotalItemQuantity(toBuyer.ToPlayerID, definition.ItemID, buyerAccount); got != 2 {
		t.Fatalf("buyer account TotalItemQuantity() = %d, want 2", got)
	}
	if result.LedgerEntries[0].PlayerID != toBuyer.PlayerID {
		t.Fatalf("source ledger player = %q, want %q", result.LedgerEntries[0].PlayerID, toBuyer.PlayerID)
	}
	if result.LedgerEntries[1].PlayerID != toBuyer.ToPlayerID {
		t.Fatalf("destination ledger player = %q, want %q", result.LedgerEntries[1].PlayerID, toBuyer.ToPlayerID)
	}
}

func TestSystemMoveItemMovesInstanceToMarketEscrow(t *testing.T) {
	service := newTestInventoryService()
	definition := validInstanceDefinition(t)
	account := validLocation(t)
	escrow := validLocationKind(t, LocationKindMarketEscrow, "listing-1")
	addResult := addInstanceItems(t, service, definition, 1, account, "loot_pickup:market-instance-seed")
	instanceID := addResult.InstanceItems[0].ItemInstanceID

	input := validMoveItemInput(t)
	input.ItemRef = MoveItemRef{
		Definition:     definition,
		ItemInstanceID: instanceID,
	}
	input.FromLocation = account
	input.ToLocation = escrow
	input.Quantity = 1
	input.Reason = "market_listing"
	input.ReferenceKey = validReferenceKey(t, "market_buy:listing-1:player-1:instance-escrow")

	result, err := service.SystemMoveItem(input)
	if err != nil {
		t.Fatalf("SystemMoveItem instance to escrow: %v", err)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, account); got != 0 {
		t.Fatalf("account TotalItemQuantity() = %d, want 0", got)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, escrow); got != 1 {
		t.Fatalf("escrow TotalItemQuantity() = %d, want 1", got)
	}
	items := service.InstanceItems()
	if len(items) != 1 {
		t.Fatalf("instance items len = %d, want 1", len(items))
	}
	if items[0].ItemInstanceID != instanceID || items[0].Location != escrow {
		t.Fatalf("moved instance = %q at %v, want %q at %v", items[0].ItemInstanceID, items[0].Location, instanceID, escrow)
	}
	if got := len(result.LedgerEntries); got != 2 {
		t.Fatalf("result ledger entries len = %d, want 2", got)
	}
	for _, entry := range result.LedgerEntries {
		if entry.ItemInstanceID != instanceID {
			t.Fatalf("ledger item instance id = %q, want %q", entry.ItemInstanceID, instanceID)
		}
	}
}

func TestSystemMoveItemRejectsInstanceQuantityAboveOneWithoutMutation(t *testing.T) {
	service := newTestInventoryService()
	definition := validInstanceDefinition(t)
	account := validLocation(t)
	escrow := validLocationKind(t, LocationKindMarketEscrow, "listing-1")
	addResult := addInstanceItems(t, service, definition, 1, account, "loot_pickup:market-instance-invalid-seed")
	instanceID := addResult.InstanceItems[0].ItemInstanceID

	input := validMoveItemInput(t)
	input.ItemRef = MoveItemRef{
		Definition:     definition,
		ItemInstanceID: instanceID,
	}
	input.FromLocation = account
	input.ToLocation = escrow
	input.Quantity = 2
	input.Reason = "market_listing"
	input.ReferenceKey = validReferenceKey(t, "market_buy:listing-1:player-1:instance-invalid")

	_, err := service.SystemMoveItem(input)
	if !errors.Is(err, ErrInvalidInstanceQuantity) {
		t.Fatalf("SystemMoveItem error = %v, want ErrInvalidInstanceQuantity", err)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, account); got != 1 {
		t.Fatalf("account TotalItemQuantity() = %d, want 1", got)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, escrow); got != 0 {
		t.Fatalf("escrow TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestSystemMoveItemDuplicateReferenceDoesNotMoveOrLedgerTwice(t *testing.T) {
	service := newTestInventoryService()
	definition := validStackableDefinition(t)
	account := validLocation(t)
	escrow := validLocationKind(t, LocationKindMarketEscrow, "listing-1")
	addStackableItems(t, service, definition, 8, account, "loot_pickup:market-duplicate-seed")

	input := validMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = account
	input.ToLocation = escrow
	input.Quantity = 3
	input.Reason = "market_listing"
	input.ReferenceKey = validReferenceKey(t, "market_buy:listing-1:player-1:duplicate")

	first, err := service.SystemMoveItem(input)
	if err != nil {
		t.Fatalf("first SystemMoveItem: %v", err)
	}
	second, err := service.SystemMoveItem(input)
	if err != nil {
		t.Fatalf("duplicate SystemMoveItem: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first SystemMoveItem Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate SystemMoveItem Duplicate = false, want true")
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, account); got != 5 {
		t.Fatalf("account TotalItemQuantity() = %d, want 5", got)
	}
	if got := service.TotalItemQuantity(input.PlayerID, definition.ItemID, escrow); got != 3 {
		t.Fatalf("escrow TotalItemQuantity() = %d, want 3", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 3 {
		t.Fatalf("ledger entries len = %d, want 3", got)
	}
	if second.LedgerEntries[0].LedgerID != first.LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate source LedgerID = %q, want %q", second.LedgerEntries[0].LedgerID, first.LedgerEntries[0].LedgerID)
	}
	if second.LedgerEntries[1].LedgerID != first.LedgerEntries[1].LedgerID {
		t.Fatalf("duplicate destination LedgerID = %q, want %q", second.LedgerEntries[1].LedgerID, first.LedgerEntries[1].LedgerID)
	}
}

func TestSystemMoveItemRejectsInvalidInputsWithoutMutation(t *testing.T) {
	cases := []struct {
		name      string
		reference string
		mutate    func(*MoveItemInput)
		wantErr   error
	}{
		{
			name:      "zero quantity",
			reference: "invalid-zero",
			mutate: func(input *MoveItemInput) {
				input.Quantity = 0
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name:      "same source and target",
			reference: "invalid-same",
			mutate: func(input *MoveItemInput) {
				input.ToLocation = input.FromLocation
			},
			wantErr: ErrMoveItemSameSourceAndTarget,
		},
		{
			name:      "insufficient source quantity",
			reference: "invalid-insufficient",
			mutate: func(input *MoveItemInput) {
				input.Quantity = 6
			},
			wantErr: ErrInsufficientItemQuantity,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestInventoryService()
			definition := validStackableDefinition(t)
			account := validLocation(t)
			escrow := validLocationKind(t, LocationKindMarketEscrow, "listing-1")
			addStackableItems(t, service, definition, 5, account, "loot_pickup:market-invalid-seed")

			input := validMoveItemInput(t)
			input.ItemRef.Definition = definition
			input.FromLocation = account
			input.ToLocation = escrow
			input.Quantity = 1
			input.Reason = "market_listing"
			input.ReferenceKey = validReferenceKey(t, "market_buy:listing-1:player-1:"+tc.reference)
			tc.mutate(&input)

			_, err := service.SystemMoveItem(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("SystemMoveItem error = %v, want %v", err, tc.wantErr)
			}
			if got := service.TotalItemQuantity("player-1", definition.ItemID, account); got != 5 {
				t.Fatalf("account TotalItemQuantity() = %d, want 5", got)
			}
			if got := service.TotalItemQuantity("player-1", definition.ItemID, escrow); got != 0 {
				t.Fatalf("escrow TotalItemQuantity() = %d, want 0", got)
			}
			if got := len(service.ItemLedgerEntries()); got != 1 {
				t.Fatalf("ledger entries len = %d, want 1", got)
			}
		})
	}
}
