package foundation

import (
	"errors"
	"fmt"
	"strings"
)

// ErrEmptyIdempotencyKey reports a missing or blank domain idempotency key.
var ErrEmptyIdempotencyKey = errors.New("empty idempotency key")

// ErrEmptyIdempotencyPart reports a missing or blank part for a domain key.
var ErrEmptyIdempotencyPart = errors.New("empty idempotency key part")

// ErrInvalidIdempotencyKey reports a malformed domain idempotency key.
var ErrInvalidIdempotencyKey = errors.New("invalid idempotency key")

// ErrInvalidIdempotencyPart reports a malformed part for a domain key.
var ErrInvalidIdempotencyPart = errors.New("invalid idempotency key part")

// IdempotencyKey identifies one domain state transition for duplicate safety.
//
// RequestID is transport retry identity. IdempotencyKey is a service-level
// domain reference used by ledgers, jobs, webhooks, and settlement flows.
type IdempotencyKey string

const (
	idempotencyQuestReward       = "quest_reward"
	idempotencyQuestReroll       = "quest_reroll"
	idempotencyCraftStart        = "craft_start"
	idempotencyCraftComplete     = "craft_complete"
	idempotencyDeathCargoDrop    = "death_cargo_drop"
	idempotencyLootPickup        = "loot_pickup"
	idempotencyAuctionClose      = "auction_close"
	idempotencyPremiumWebhook    = "premium_webhook"
	idempotencyOfflineSettlement = "offline_settlement"
	idempotencyMarketBuy         = "market_buy"
	idempotencyShipRepair        = "ship_repair"
)

// ParseIdempotencyKey validates value and returns an IdempotencyKey.
func ParseIdempotencyKey(value string) (IdempotencyKey, error) {
	if err := validateIdempotencyKey(value); err != nil {
		return "", err
	}
	return IdempotencyKey(value), nil
}

// QuestRewardIdempotencyKey returns quest_reward:<player_quest_id>.
func QuestRewardIdempotencyKey(playerQuestID QuestID) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyQuestReward, playerQuestID.String())
}

// QuestRerollIdempotencyKey returns quest_reroll:<player_id>:<reroll_reference>.
func QuestRerollIdempotencyKey(playerID PlayerID, rerollReference string) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyQuestReroll, playerID.String(), rerollReference)
}

// CraftStartIdempotencyKey returns craft_start:<start_reference>.
func CraftStartIdempotencyKey(startReference string) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyCraftStart, startReference)
}

// CraftCompleteIdempotencyKey returns craft_complete:<job_id>.
func CraftCompleteIdempotencyKey(jobID string) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyCraftComplete, jobID)
}

// DeathCargoDropIdempotencyKey returns death_cargo_drop:<death_id>:<stack_id>.
func DeathCargoDropIdempotencyKey(deathID EventID, stackID ItemID) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyDeathCargoDrop, deathID.String(), stackID.String())
}

// LootPickupIdempotencyKey returns loot_pickup:<drop_id>.
func LootPickupIdempotencyKey(dropID string) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyLootPickup, dropID)
}

// AuctionCloseIdempotencyKey returns auction_close:<auction_id>.
func AuctionCloseIdempotencyKey(auctionID AuctionID) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyAuctionClose, auctionID.String())
}

// PremiumWebhookIdempotencyKey returns premium_webhook:<provider_event_id>.
func PremiumWebhookIdempotencyKey(providerEventID string) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyPremiumWebhook, providerEventID)
}

// OfflineSettlementIdempotencyKey returns offline_settlement:<planet_id>:<settlement_window>.
func OfflineSettlementIdempotencyKey(planetID PlanetID, settlementWindow string) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyOfflineSettlement, planetID.String(), settlementWindow)
}

// MarketBuyIdempotencyKey returns market_buy:<listing_id>:<buyer_id>:<request_id>.
//
// The request ID is only one domain reference part here. The returned value is
// still an IdempotencyKey and must not be modeled on request envelopes.
func MarketBuyIdempotencyKey(listingID ListingID, buyerID PlayerID, requestID RequestID) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyMarketBuy, listingID.String(), buyerID.String(), requestID.String())
}

// ShipRepairIdempotencyKey returns ship_repair:<ship_id>:<repair_reference>.
func ShipRepairIdempotencyKey(shipID ShipID, repairReference string) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyShipRepair, shipID.String(), repairReference)
}

// ShipRepairShipID returns the ship id encoded in a ship repair idempotency key.
func ShipRepairShipID(key IdempotencyKey) (ShipID, error) {
	if err := key.Validate(); err != nil {
		return "", err
	}
	parts := strings.Split(key.String(), ":")
	if len(parts) != 3 || parts[0] != idempotencyShipRepair {
		return "", fmt.Errorf("idempotency key %q: %w", key, ErrInvalidIdempotencyKey)
	}
	return ParseShipID(parts[1])
}

// String returns the stable key representation.
func (key IdempotencyKey) String() string {
	return string(key)
}

// Validate reports whether key is non-blank.
func (key IdempotencyKey) Validate() error {
	return validateIdempotencyKey(string(key))
}

// IsZero reports whether key is the zero value.
func (key IdempotencyKey) IsZero() bool {
	return key == ""
}

func buildIdempotencyKey(operation string, parts ...string) (IdempotencyKey, error) {
	if err := validateIdempotencyPart("operation", operation); err != nil {
		return "", err
	}

	values := make([]string, 0, 1+len(parts))
	values = append(values, operation)
	for index, part := range parts {
		if err := validateIdempotencyPart(fmt.Sprintf("part %d", index+1), part); err != nil {
			return "", err
		}
		values = append(values, part)
	}

	return IdempotencyKey(strings.Join(values, ":")), nil
}

func validateIdempotencyKey(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("idempotency key: %w", ErrEmptyIdempotencyKey)
	}
	parts := strings.Split(value, ":")
	if len(parts) == 0 {
		return fmt.Errorf("idempotency key: %w", ErrInvalidIdempotencyKey)
	}
	if err := validateIdempotencyPart("operation", parts[0]); err != nil {
		return err
	}
	expectedParts, ok := idempotencyPartCount(parts[0])
	if !ok {
		return fmt.Errorf("operation %q: %w", parts[0], ErrInvalidIdempotencyKey)
	}
	if len(parts)-1 != expectedParts {
		return fmt.Errorf("idempotency key %q: %w", value, ErrInvalidIdempotencyKey)
	}
	for index, part := range parts[1:] {
		if err := validateIdempotencyPart(fmt.Sprintf("part %d", index+1), part); err != nil {
			return err
		}
	}
	return nil
}

func validateIdempotencyPart(kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s: %w", kind, ErrEmptyIdempotencyPart)
	}
	if value != strings.TrimSpace(value) || strings.Contains(value, ":") {
		return fmt.Errorf("%s %q: %w", kind, value, ErrInvalidIdempotencyPart)
	}
	return nil
}

func idempotencyPartCount(operation string) (int, bool) {
	switch operation {
	case idempotencyQuestReward,
		idempotencyCraftStart,
		idempotencyCraftComplete,
		idempotencyLootPickup,
		idempotencyAuctionClose,
		idempotencyPremiumWebhook:
		return 1, true
	case idempotencyOfflineSettlement:
		return 2, true
	case idempotencyQuestReroll:
		return 2, true
	case idempotencyMarketBuy:
		return 3, true
	case idempotencyShipRepair,
		idempotencyDeathCargoDrop:
		return 2, true
	default:
		return 0, false
	}
}
