package market

import (
	"encoding/json"
	"fmt"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

// ListingStatus records the fixed-price listing lifecycle.
type ListingStatus string

const (
	ListingStatusActive    ListingStatus = "active"
	ListingStatusSold      ListingStatus = "sold"
	ListingStatusCancelled ListingStatus = "cancelled"
	ListingStatusExpired   ListingStatus = "expired"
	ListingStatusStale     ListingStatus = "stale"
	ListingStatusLocked    ListingStatus = "locked"
)

// FeePolicy controls server-side sale fee calculation in basis points.
type FeePolicy struct {
	SaleFeeBasisPoints int64 `json:"sale_fee_basis_points"`
}

// Listing records one fixed-price sell listing and its escrow location.
type Listing struct {
	ListingID            foundation.ListingID   `json:"listing_id"`
	SellerPlayerID       foundation.PlayerID    `json:"seller_player_id"`
	ItemDefinition       economy.ItemDefinition `json:"item_definition"`
	ItemInstanceID       foundation.ItemID      `json:"item_instance_id,omitempty"`
	ItemID               foundation.ItemID      `json:"item_id"`
	OriginalQuantity     int64                  `json:"original_quantity"`
	RemainingQuantity    int64                  `json:"remaining_quantity"`
	UnitPrice            int64                  `json:"unit_price"`
	Currency             economy.CurrencyBucket `json:"currency_type"`
	Status               ListingStatus          `json:"status"`
	SourceReturnLocation economy.ItemLocation   `json:"source_return_location"`
	EscrowLocation       economy.ItemLocation   `json:"escrow_location"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
	ExpiresAt            *time.Time             `json:"expires_at,omitempty"`
	StaleAt              *time.Time             `json:"stale_at,omitempty"`
	StaleReason          string                 `json:"stale_reason,omitempty"`
}

// CreateListingInput describes one server-authoritative fixed-price listing creation.
type CreateListingInput struct {
	ListingID      foundation.ListingID   `json:"listing_id"`
	SellerPlayerID foundation.PlayerID    `json:"seller_player_id"`
	ItemRef        economy.MoveItemRef    `json:"item_ref"`
	SourceLocation economy.ItemLocation   `json:"source_location"`
	Quantity       int64                  `json:"quantity"`
	UnitPrice      int64                  `json:"unit_price"`
	Currency       economy.CurrencyBucket `json:"currency_type"`
	ExpiresAt      *time.Time             `json:"expires_at,omitempty"`
}

// CreateListingResult reports the listing and escrow movement.
type CreateListingResult struct {
	Listing      Listing                   `json:"listing"`
	EscrowMove   economy.MoveItemResult    `json:"escrow_move"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
}

// BuyListingInput describes one fixed-price listing purchase attempt.
type BuyListingInput struct {
	BuyerPlayerID foundation.PlayerID  `json:"buyer_player_id"`
	ListingID     foundation.ListingID `json:"listing_id"`
	Quantity      int64                `json:"quantity"`
	RequestID     foundation.RequestID `json:"request_id"`
}

// BuyListingResult reports all server-calculated item and currency movements.
type BuyListingResult struct {
	Listing        Listing                     `json:"listing"`
	Quantity       int64                       `json:"quantity"`
	TotalAmount    int64                       `json:"total_amount"`
	FeeAmount      int64                       `json:"fee_amount"`
	SellerProceeds int64                       `json:"seller_proceeds"`
	BuyerDebit     economy.DebitWalletResult   `json:"buyer_debit"`
	SellerCredit   economy.CreditWalletResult  `json:"seller_credit"`
	FeeCredit      *economy.CreditWalletResult `json:"fee_credit,omitempty"`
	ItemMove       economy.MoveItemResult      `json:"item_move"`
	ReferenceKey   foundation.IdempotencyKey   `json:"reference_id"`
	SaleReference  foundation.IdempotencyKey   `json:"sale_reference_id"`
	FeeReference   foundation.IdempotencyKey   `json:"fee_reference_id,omitempty"`
	Duplicate      bool                        `json:"duplicate"`
}

// CancelListingInput describes a seller cancellation request.
type CancelListingInput struct {
	SellerPlayerID foundation.PlayerID  `json:"seller_player_id"`
	ListingID      foundation.ListingID `json:"listing_id"`
}

// ExpireListingInput describes a server expiration command for one listing.
type ExpireListingInput struct {
	ListingID foundation.ListingID `json:"listing_id"`
}

// MarkListingStaleInput describes an intel/coordinate invalidation marker.
type MarkListingStaleInput struct {
	ListingID foundation.ListingID `json:"listing_id"`
	Reason    string               `json:"reason"`
}

// CancelListingResult reports returned escrow quantity.
type CancelListingResult struct {
	Listing          Listing                   `json:"listing"`
	ReturnedQuantity int64                     `json:"returned_quantity"`
	ReturnMove       economy.MoveItemResult    `json:"return_move"`
	ReferenceKey     foundation.IdempotencyKey `json:"reference_id"`
	Duplicate        bool                      `json:"duplicate"`
}

// ExpireListingResult reports escrow returned by the expiration command.
type ExpireListingResult struct {
	Listing          Listing                   `json:"listing"`
	ReturnedQuantity int64                     `json:"returned_quantity"`
	ReturnMove       economy.MoveItemResult    `json:"return_move"`
	ReferenceKey     foundation.IdempotencyKey `json:"reference_id"`
	Duplicate        bool                      `json:"duplicate"`
}

// MarkListingStaleResult reports the stale listing snapshot.
type MarkListingStaleResult struct {
	Listing   Listing `json:"listing"`
	Duplicate bool    `json:"duplicate"`
}

// String returns the stable status representation.
func (status ListingStatus) String() string {
	return string(status)
}

// Validate reports whether status is part of the listing state machine.
func (status ListingStatus) Validate() error {
	switch status {
	case ListingStatusActive,
		ListingStatusSold,
		ListingStatusCancelled,
		ListingStatusExpired,
		ListingStatusStale,
		ListingStatusLocked:
		return nil
	default:
		return fmt.Errorf("listing status %q: %w", status, ErrInvalidListingStatus)
	}
}

// CanTransitionTo reports whether the listing state machine allows next.
func (status ListingStatus) CanTransitionTo(next ListingStatus) bool {
	switch status {
	case ListingStatusActive:
		switch next {
		case ListingStatusSold, ListingStatusCancelled, ListingStatusExpired, ListingStatusStale, ListingStatusLocked:
			return true
		}
	case ListingStatusStale:
		switch next {
		case ListingStatusCancelled, ListingStatusExpired, ListingStatusLocked:
			return true
		}
	case ListingStatusLocked:
		switch next {
		case ListingStatusActive, ListingStatusSold, ListingStatusCancelled, ListingStatusExpired:
			return true
		}
	}
	return status == next
}

// IsTerminal reports whether no further item movement should happen for status.
func (status ListingStatus) IsTerminal() bool {
	switch status {
	case ListingStatusSold, ListingStatusCancelled, ListingStatusExpired:
		return true
	default:
		return false
	}
}

// DefaultFeePolicy returns the MVP market fee policy.
func DefaultFeePolicy() FeePolicy {
	return FeePolicy{SaleFeeBasisPoints: 500}
}

func cloneListing(listing Listing) Listing {
	listing.ItemDefinition = cloneItemDefinition(listing.ItemDefinition)
	if listing.ExpiresAt != nil {
		expiresAt := *listing.ExpiresAt
		listing.ExpiresAt = &expiresAt
	}
	if listing.StaleAt != nil {
		staleAt := *listing.StaleAt
		listing.StaleAt = &staleAt
	}
	return listing
}

func cloneItemDefinition(definition economy.ItemDefinition) economy.ItemDefinition {
	definition.TradeFlags = append([]economy.TradeFlag(nil), definition.TradeFlags...)
	definition.BindRules = append([]economy.BindRule(nil), definition.BindRules...)
	if len(definition.MetadataSchema) > 0 {
		definition.MetadataSchema = append(json.RawMessage(nil), definition.MetadataSchema...)
	}
	return definition
}
