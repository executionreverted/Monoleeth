package economy

import (
	"errors"
	"testing"
)

func TestItemTradePolicyValidatesMarketAuctionDropAndDestroyFlags(t *testing.T) {
	cases := []struct {
		name     string
		validate func([]TradeFlag) error
		flags    []TradeFlag
		wantErr  error
	}{
		{
			name:     "market allows market specific flag",
			validate: ValidateMarketListingTradeFlags,
			flags:    []TradeFlag{TradeFlagMarketTradeable},
		},
		{
			name:     "market allows generic tradeable flag",
			validate: ValidateMarketListingTradeFlags,
			flags:    []TradeFlag{TradeFlagTradeable},
		},
		{
			name:     "market rejects auction-only flag",
			validate: ValidateMarketListingTradeFlags,
			flags:    []TradeFlag{TradeFlagAuctionTradeable},
			wantErr:  ErrItemNotMarketTradeable,
		},
		{
			name:     "auction allows auction specific flag",
			validate: ValidateAuctionListingTradeFlags,
			flags:    []TradeFlag{TradeFlagAuctionTradeable},
		},
		{
			name:     "auction allows generic tradeable flag",
			validate: ValidateAuctionListingTradeFlags,
			flags:    []TradeFlag{TradeFlagTradeable},
		},
		{
			name:     "auction rejects market-only flag",
			validate: ValidateAuctionListingTradeFlags,
			flags:    []TradeFlag{TradeFlagMarketTradeable},
			wantErr:  ErrItemNotAuctionTradeable,
		},
		{
			name:     "drop requires droppable flag",
			validate: ValidateDroppableTradeFlags,
			flags:    []TradeFlag{TradeFlagDroppable},
		},
		{
			name:     "drop rejects generic tradeable flag",
			validate: ValidateDroppableTradeFlags,
			flags:    []TradeFlag{TradeFlagTradeable},
			wantErr:  ErrItemNotDroppable,
		},
		{
			name:     "destroy requires destroyable flag",
			validate: ValidateDestroyableTradeFlags,
			flags:    []TradeFlag{TradeFlagDestroyable},
		},
		{
			name:     "destroy rejects generic tradeable flag",
			validate: ValidateDestroyableTradeFlags,
			flags:    []TradeFlag{TradeFlagTradeable},
			wantErr:  ErrItemNotDestroyable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.validate(tc.flags)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("validate trade flags error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("validate trade flags error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestItemTradePolicyRejectsInvalidTradeFlag(t *testing.T) {
	err := ValidateMarketListingTradeFlags([]TradeFlag{
		TradeFlagTradeable,
		TradeFlag("bad_flag"),
	})
	if !errors.Is(err, ErrInvalidTradeFlag) {
		t.Fatalf("ValidateMarketListingTradeFlags invalid flag error = %v, want ErrInvalidTradeFlag", err)
	}
}

func TestPlayerTradeOrEquipPolicyBlocksEquippedEscrowReservedAndSystemLocations(t *testing.T) {
	accountLocation := validLocation(t)
	if err := ValidatePlayerTradeOrEquipLocation(accountLocation, true); !errors.Is(err, ErrBlockedEquippedItem) {
		t.Fatalf("equipped location error = %v, want ErrBlockedEquippedItem", err)
	}

	blockedKinds := []LocationKind{
		LocationKindMarketEscrow,
		LocationKindAuctionEscrow,
		LocationKindCraftingReserved,
		LocationKindSystemSink,
	}
	for _, kind := range blockedKinds {
		t.Run(kind.String(), func(t *testing.T) {
			location := validLocationKind(t, kind, "blocked-1")
			err := ValidatePlayerTradeOrEquipLocation(location, false)
			if !errors.Is(err, ErrBlockedPlayerTradeOrEquipLocation) {
				t.Fatalf("blocked location error = %v, want ErrBlockedPlayerTradeOrEquipLocation", err)
			}
			if !IsBlockedPlayerTradeOrEquipLocationKind(kind) {
				t.Fatalf("IsBlockedPlayerTradeOrEquipLocationKind(%q) = false, want true", kind)
			}
		})
	}
}

func TestPlayerTradeOrEquipPolicyAllowsUnblockedModeledLocations(t *testing.T) {
	allowedKinds := []LocationKind{
		LocationKindAccountInventory,
		LocationKindShipCargo,
		LocationKindPlanetStorage,
		LocationKindStationStorage,
		LocationKindWorldDrop,
	}
	for _, kind := range allowedKinds {
		t.Run(kind.String(), func(t *testing.T) {
			location := validLocationKind(t, kind, "container-1")
			if err := ValidatePlayerTradeOrEquipLocation(location, false); err != nil {
				t.Fatalf("ValidatePlayerTradeOrEquipLocation(%q) error = %v, want nil", kind, err)
			}
			if IsBlockedPlayerTradeOrEquipLocationKind(kind) {
				t.Fatalf("IsBlockedPlayerTradeOrEquipLocationKind(%q) = true, want false", kind)
			}
		})
	}
}

func TestPlayerTradeOrEquipPolicyRejectsInvalidLocation(t *testing.T) {
	err := ValidatePlayerTradeOrEquipLocation(ItemLocation{}, false)
	if !errors.Is(err, ErrInvalidLocationKind) {
		t.Fatalf("invalid location error = %v, want ErrInvalidLocationKind", err)
	}
}
