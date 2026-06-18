package auction

import (
	"encoding/json"
	"fmt"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

// LotStatus records the system auction lifecycle.
type LotStatus string

const (
	LotStatusUpcoming LotStatus = "upcoming"
	LotStatusActive   LotStatus = "active"
	LotStatusClosed   LotStatus = "closed"
	LotStatusExpired  LotStatus = "expired"
)

// LotPayloadType describes the grant adapter that will receive the lot payload.
type LotPayloadType string

const (
	LotPayloadTypeShipUnlock          LotPayloadType = "ship_unlock"
	LotPayloadTypeModuleBlueprint     LotPayloadType = "module_blueprint"
	LotPayloadTypeXCore               LotPayloadType = "x_core"
	LotPayloadTypeXCoreFragmentBundle LotPayloadType = "x_core_fragment_bundle"
	LotPayloadTypeRareMaterialBundle  LotPayloadType = "rare_material_bundle"
	LotPayloadTypeCosmetic            LotPayloadType = "cosmetic"
	LotPayloadTypeIntelCache          LotPayloadType = "intel_cache"
	LotPayloadTypeBuildingBlueprint   LotPayloadType = "building_blueprint"
)

// CloseReason records why a lot reached a terminal state.
type CloseReason string

const (
	CloseReasonEnded  CloseReason = "ended"
	CloseReasonBuyNow CloseReason = "buy_now"
	CloseReasonNoBids CloseReason = "no_bids"
)

// LotPayload records the server-catalog payload for a system-created lot.
type LotPayload struct {
	Type     LotPayloadType              `json:"type"`
	Source   catalog.VersionedDefinition `json:"source"`
	Quantity int64                       `json:"quantity"`
	Metadata json.RawMessage             `json:"metadata,omitempty"`
}

// Lot records one server-generated auction lot.
type Lot struct {
	AuctionID       foundation.AuctionID   `json:"auction_id"`
	WorldID         foundation.WorldID     `json:"world_id"`
	Payload         LotPayload             `json:"payload"`
	Currency        economy.CurrencyBucket `json:"currency_type"`
	StartPrice      int64                  `json:"start_price"`
	BuyNowPrice     *int64                 `json:"buy_now_price,omitempty"`
	CurrentBid      int64                  `json:"current_bid"`
	CurrentBidderID foundation.PlayerID    `json:"current_bidder_id,omitempty"`
	Status          LotStatus              `json:"status"`
	StartsAt        time.Time              `json:"starts_at"`
	EndsAt          time.Time              `json:"ends_at"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	ClosedAt        *time.Time             `json:"closed_at,omitempty"`
	WinningPlayerID foundation.PlayerID    `json:"winning_player_id,omitempty"`
	CloseReason     CloseReason            `json:"close_reason,omitempty"`
}

// Grant records the skeleton payload grant created by close or buy-now.
type Grant struct {
	AuctionID foundation.AuctionID `json:"auction_id"`
	PlayerID  foundation.PlayerID  `json:"player_id"`
	Payload   LotPayload           `json:"payload"`
	Reason    CloseReason          `json:"reason"`
	GrantedAt time.Time            `json:"granted_at"`
}

// CreateLotInput describes a server-authored auction lot creation.
type CreateLotInput struct {
	AuctionID   foundation.AuctionID   `json:"auction_id"`
	WorldID     foundation.WorldID     `json:"world_id"`
	Payload     LotPayload             `json:"payload"`
	Currency    economy.CurrencyBucket `json:"currency_type"`
	StartPrice  int64                  `json:"start_price"`
	BuyNowPrice *int64                 `json:"buy_now_price,omitempty"`
	StartsAt    time.Time              `json:"starts_at"`
	EndsAt      time.Time              `json:"ends_at"`
}

// PlaceBidInput describes one bidder intent.
type PlaceBidInput struct {
	AuctionID      foundation.AuctionID `json:"auction_id"`
	BidderPlayerID foundation.PlayerID  `json:"bidder_player_id"`
	Amount         int64                `json:"amount"`
	RequestID      foundation.RequestID `json:"request_id"`
}

// BuyNowInput describes one buy-now intent.
type BuyNowInput struct {
	AuctionID     foundation.AuctionID `json:"auction_id"`
	BuyerPlayerID foundation.PlayerID  `json:"buyer_player_id"`
	RequestID     foundation.RequestID `json:"request_id"`
}

// CloseAuctionInput describes a server close worker or command.
type CloseAuctionInput struct {
	AuctionID foundation.AuctionID `json:"auction_id"`
	Force     bool                 `json:"force"`
}

// CreateLotResult reports the created lot.
type CreateLotResult struct {
	Lot Lot `json:"lot"`
}

// PlaceBidResult reports bid debit, optional outbid refund, and the lot snapshot.
type PlaceBidResult struct {
	Lot             Lot                         `json:"lot"`
	Amount          int64                       `json:"amount"`
	BidderDebit     economy.DebitWalletResult   `json:"bidder_debit"`
	PreviousRefund  *economy.CreditWalletResult `json:"previous_refund,omitempty"`
	ReferenceKey    foundation.IdempotencyKey   `json:"reference_id"`
	RefundReference foundation.IdempotencyKey   `json:"refund_reference_id,omitempty"`
	Duplicate       bool                        `json:"duplicate"`
}

// BuyNowResult reports the closing debit, optional refund, and skeleton grant.
type BuyNowResult struct {
	Lot             Lot                         `json:"lot"`
	Price           int64                       `json:"price"`
	BuyerDebit      economy.DebitWalletResult   `json:"buyer_debit"`
	CurrentRefund   *economy.CreditWalletResult `json:"current_refund,omitempty"`
	Grant           Grant                       `json:"grant"`
	ReferenceKey    foundation.IdempotencyKey   `json:"reference_id"`
	RefundReference foundation.IdempotencyKey   `json:"refund_reference_id,omitempty"`
	Duplicate       bool                        `json:"duplicate"`
}

// CloseAuctionResult reports the terminal lot state and optional winner grant.
type CloseAuctionResult struct {
	Lot          Lot                       `json:"lot"`
	Grant        *Grant                    `json:"grant,omitempty"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
	Duplicate    bool                      `json:"duplicate"`
}

// String returns the stable status representation.
func (status LotStatus) String() string {
	return string(status)
}

// Validate reports whether status is part of the auction state machine.
func (status LotStatus) Validate() error {
	switch status {
	case LotStatusUpcoming, LotStatusActive, LotStatusClosed, LotStatusExpired:
		return nil
	default:
		return fmt.Errorf("auction lot status %q: %w", status, ErrInvalidLotStatus)
	}
}

// CanTransitionTo reports whether the auction state machine allows next.
func (status LotStatus) CanTransitionTo(next LotStatus) bool {
	switch status {
	case LotStatusUpcoming:
		return next == LotStatusActive || next == LotStatusClosed || next == LotStatusExpired || next == LotStatusUpcoming
	case LotStatusActive:
		return next == LotStatusClosed || next == LotStatusExpired || next == LotStatusActive
	default:
		return status == next
	}
}

// IsTerminal reports whether the lot can no longer accept value-changing input.
func (status LotStatus) IsTerminal() bool {
	return status == LotStatusClosed || status == LotStatusExpired
}

// String returns the stable payload type representation.
func (payloadType LotPayloadType) String() string {
	return string(payloadType)
}

// Validate reports whether payloadType is supported.
func (payloadType LotPayloadType) Validate() error {
	switch payloadType {
	case LotPayloadTypeShipUnlock,
		LotPayloadTypeModuleBlueprint,
		LotPayloadTypeXCore,
		LotPayloadTypeXCoreFragmentBundle,
		LotPayloadTypeRareMaterialBundle,
		LotPayloadTypeCosmetic,
		LotPayloadTypeIntelCache,
		LotPayloadTypeBuildingBlueprint:
		return nil
	default:
		return fmt.Errorf("auction payload type %q: %w", payloadType, ErrInvalidLotPayloadType)
	}
}

func (payload LotPayload) validate() error {
	if err := payload.Type.Validate(); err != nil {
		return err
	}
	if err := payload.Source.Validate(); err != nil {
		return err
	}
	if _, err := foundation.NewQuantity(payload.Quantity); err != nil {
		return fmt.Errorf("payload quantity: %w", err)
	}
	if len(payload.Metadata) > 0 && !json.Valid(payload.Metadata) {
		return fmt.Errorf("payload metadata: %w", ErrInvalidLotPayload)
	}
	return nil
}

func cloneLot(lot Lot) Lot {
	lot.Payload = clonePayload(lot.Payload)
	if lot.BuyNowPrice != nil {
		buyNowPrice := *lot.BuyNowPrice
		lot.BuyNowPrice = &buyNowPrice
	}
	if lot.ClosedAt != nil {
		closedAt := *lot.ClosedAt
		lot.ClosedAt = &closedAt
	}
	return lot
}

func clonePayload(payload LotPayload) LotPayload {
	if len(payload.Metadata) > 0 {
		payload.Metadata = append(json.RawMessage(nil), payload.Metadata...)
	}
	return payload
}

func cloneGrant(grant Grant) Grant {
	grant.Payload = clonePayload(grant.Payload)
	return grant
}
