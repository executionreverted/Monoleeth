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

// IdempotencyKey identifies one domain state transition for duplicate safety.
//
// RequestID is transport retry identity. IdempotencyKey is a service-level
// domain reference used by ledgers, jobs, webhooks, and settlement flows.
type IdempotencyKey string

const (
	idempotencyQuestReward       = "quest_reward"
	idempotencyCraftComplete     = "craft_complete"
	idempotencyLootPickup        = "loot_pickup"
	idempotencyAuctionClose      = "auction_close"
	idempotencyPremiumWebhook    = "premium_webhook"
	idempotencyOfflineSettlement = "offline_settlement"
	idempotencyMarketBuy         = "market_buy"
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

// CraftCompleteIdempotencyKey returns craft_complete:<job_id>.
func CraftCompleteIdempotencyKey(jobID string) (IdempotencyKey, error) {
	return buildIdempotencyKey(idempotencyCraftComplete, jobID)
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
	return nil
}

func validateIdempotencyPart(kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s: %w", kind, ErrEmptyIdempotencyPart)
	}
	return nil
}
