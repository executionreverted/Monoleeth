package economy

import (
	"errors"
	"fmt"
)

var (
	ErrItemNotMarketTradeable            = errors.New("item is not market tradeable")
	ErrItemNotAuctionTradeable           = errors.New("item is not auction tradeable")
	ErrItemNotDroppable                  = errors.New("item is not droppable")
	ErrItemNotDestroyable                = errors.New("item is not destroyable")
	ErrBlockedEquippedItem               = errors.New("equipped item is blocked")
	ErrBlockedPlayerTradeOrEquipLocation = errors.New("blocked player trade or equip location")
)

// ValidateMarketListingTradeFlags reports whether flags allow market listing.
func ValidateMarketListingTradeFlags(flags []TradeFlag) error {
	return validateTradeFlagRequirement(
		flags,
		ErrItemNotMarketTradeable,
		TradeFlagMarketTradeable,
		TradeFlagTradeable,
	)
}

// ValidateAuctionListingTradeFlags reports whether flags allow auction listing.
func ValidateAuctionListingTradeFlags(flags []TradeFlag) error {
	return validateTradeFlagRequirement(
		flags,
		ErrItemNotAuctionTradeable,
		TradeFlagAuctionTradeable,
		TradeFlagTradeable,
	)
}

// ValidateDroppableTradeFlags reports whether flags allow dropping an item.
func ValidateDroppableTradeFlags(flags []TradeFlag) error {
	return validateTradeFlagRequirement(flags, ErrItemNotDroppable, TradeFlagDroppable)
}

// ValidateDestroyableTradeFlags reports whether flags allow destroying an item.
func ValidateDestroyableTradeFlags(flags []TradeFlag) error {
	return validateTradeFlagRequirement(flags, ErrItemNotDestroyable, TradeFlagDestroyable)
}

// ValidatePlayerTradeOrEquipLocation blocks locations that player trade/equip
// style flows must not use directly. Callers still validate ownership and the
// exact source locations allowed by their specific flow.
func ValidatePlayerTradeOrEquipLocation(location ItemLocation, equipped bool) error {
	if err := location.Validate(); err != nil {
		return err
	}
	if equipped {
		return ErrBlockedEquippedItem
	}
	if IsBlockedPlayerTradeOrEquipLocationKind(location.Kind) {
		return fmt.Errorf("location kind %q: %w", location.Kind, ErrBlockedPlayerTradeOrEquipLocation)
	}
	return nil
}

// IsBlockedPlayerTradeOrEquipLocationKind reports whether kind is currently
// modeled as escrow, reserved, or system-owned for player trade/equip flows.
func IsBlockedPlayerTradeOrEquipLocationKind(kind LocationKind) bool {
	switch kind {
	case LocationKindMarketEscrow,
		LocationKindAuctionEscrow,
		LocationKindCraftingReserved,
		LocationKindSystemSink:
		return true
	default:
		return false
	}
}

func validateTradeFlagRequirement(flags []TradeFlag, missing error, allowed ...TradeFlag) error {
	matched := false
	for _, flag := range flags {
		if err := flag.Validate(); err != nil {
			return err
		}
		for _, allowedFlag := range allowed {
			if flag == allowedFlag {
				matched = true
			}
		}
	}
	if !matched {
		return missing
	}
	return nil
}
