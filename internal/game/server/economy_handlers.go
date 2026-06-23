package server

import (
	"context"
	"encoding/json"
	"errors"

	"gameproject/internal/game/auction"
	"gameproject/internal/game/auth"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/intel"
	"gameproject/internal/game/market"
	"gameproject/internal/game/premium"
	"gameproject/internal/game/realtime"
)

const premiumWeeklyXCoreProductID = "weekly_xcore"

type marketSearchPayload struct {
	Listings []marketListingPayload `json:"listings"`
	Counts   marketCountsPayload    `json:"counts"`
}

type marketCountsPayload struct {
	Active int `json:"active"`
	Mine   int `json:"mine"`
}

type marketListingPayload struct {
	ListingID             string                `json:"listing_id"`
	ItemID                string                `json:"item_id"`
	DisplayName           string                `json:"display_name"`
	Rarity                string                `json:"rarity"`
	RemainingQuantity     int64                 `json:"remaining_quantity"`
	UnitPrice             int64                 `json:"unit_price"`
	Currency              string                `json:"currency_type"`
	Status                string                `json:"status"`
	ExpiresAt             int64                 `json:"expires_at,omitempty"`
	OwnedByYou            bool                  `json:"owned_by_you"`
	FinalPricePending     bool                  `json:"final_price_pending"`
	EstimatedUnitPurchase marketEstimatePayload `json:"estimated_unit_purchase"`
}

type marketEstimatePayload struct {
	Quantity int64  `json:"quantity"`
	Subtotal int64  `json:"subtotal"`
	Currency string `json:"currency_type"`
	Pending  bool   `json:"pending"`
}

type marketMutationPayload struct {
	Accepted          bool                     `json:"accepted"`
	Duplicate         bool                     `json:"duplicate,omitempty"`
	Listing           marketListingPayload     `json:"listing"`
	Quantity          int64                    `json:"quantity,omitempty"`
	ServerTotal       int64                    `json:"server_total,omitempty"`
	ServerFee         int64                    `json:"server_fee,omitempty"`
	FinalPricePending bool                     `json:"final_price_pending"`
	Market            marketSearchPayload      `json:"market"`
	Wallet            walletSnapshotPayload    `json:"wallet"`
	Inventory         inventorySnapshotPayload `json:"inventory"`
}

type auctionSearchPayload struct {
	Lots   []auctionLotPayload   `json:"lots"`
	Grants []auctionGrantPayload `json:"grants"`
}

type auctionLotPayload struct {
	AuctionID         string `json:"auction_id"`
	PayloadType       string `json:"payload_type"`
	DefinitionID      string `json:"definition_id"`
	Quantity          int64  `json:"quantity"`
	Currency          string `json:"currency_type"`
	StartPrice        int64  `json:"start_price"`
	CurrentBid        int64  `json:"current_bid"`
	HasBid            bool   `json:"has_bid"`
	Leading           bool   `json:"leading"`
	BuyNowPrice       *int64 `json:"buy_now_price,omitempty"`
	Status            string `json:"status"`
	StartsAt          int64  `json:"starts_at"`
	EndsAt            int64  `json:"ends_at"`
	FinalPricePending bool   `json:"final_price_pending"`
}

type auctionGrantPayload struct {
	AuctionID    string `json:"auction_id"`
	PayloadType  string `json:"payload_type"`
	DefinitionID string `json:"definition_id"`
	Quantity     int64  `json:"quantity"`
	Reason       string `json:"reason"`
	GrantedAt    int64  `json:"granted_at"`
}

type auctionMutationPayload struct {
	Accepted          bool                  `json:"accepted"`
	Duplicate         bool                  `json:"duplicate,omitempty"`
	Lot               auctionLotPayload     `json:"lot"`
	Amount            int64                 `json:"amount,omitempty"`
	Price             int64                 `json:"price,omitempty"`
	Grant             *auctionGrantPayload  `json:"grant,omitempty"`
	Auction           auctionSearchPayload  `json:"auction"`
	Wallet            walletSnapshotPayload `json:"wallet"`
	FinalPricePending bool                  `json:"final_price_pending"`
}

type premiumSummaryPayload struct {
	Entitlements []premiumEntitlementPayload `json:"entitlements"`
	Stock        []premiumStockPayload       `json:"stock"`
	Purchases    []premiumPurchasePayload    `json:"purchases"`
}

type premiumEntitlementPayload struct {
	EntitlementID string                      `json:"entitlement_id"`
	Type          string                      `json:"type"`
	State         string                      `json:"state"`
	Payload       premiumEntitlementGrantInfo `json:"payload"`
	CreatedAt     int64                       `json:"created_at"`
	ClaimedAt     int64                       `json:"claimed_at,omitempty"`
}

type premiumEntitlementGrantInfo struct {
	CurrencyBucket   string `json:"currency_bucket,omitempty"`
	Amount           int64  `json:"amount,omitempty"`
	LoadoutSlotScope string `json:"loadout_slot_scope,omitempty"`
	LoadoutSlotCount int64  `json:"loadout_slot_count,omitempty"`
	PeriodKey        string `json:"period_key,omitempty"`
	CosmeticID       string `json:"cosmetic_id,omitempty"`
	BadgeID          string `json:"badge_id,omitempty"`
}

type premiumStockPayload struct {
	PeriodKey       string `json:"period_key"`
	StockTotal      int64  `json:"stock_total"`
	StockRemaining  int64  `json:"stock_remaining"`
	PriceAmount     int64  `json:"price_amount"`
	PaymentCurrency string `json:"payment_currency"`
}

type premiumPurchasePayload struct {
	PeriodKey       string `json:"period_key"`
	PaymentCurrency string `json:"payment_currency"`
	GrantedAt       int64  `json:"granted_at"`
}

type premiumMutationPayload struct {
	Accepted  bool                  `json:"accepted"`
	Duplicate bool                  `json:"duplicate,omitempty"`
	Premium   premiumSummaryPayload `json:"premium"`
	Wallet    walletSnapshotPayload `json:"wallet"`
}

type economyDashboardPayload struct {
	Wallets   economyDashboardWallets `json:"wallets"`
	Market    economyDashboardMarket  `json:"market"`
	Auction   economyDashboardAuction `json:"auction"`
	Premium   economyDashboardPremium `json:"premium"`
	Generated int64                   `json:"generated_at"`
}

type economyDashboardWallets struct {
	Credits       int64 `json:"credits"`
	PremiumPaid   int64 `json:"premium_paid"`
	PremiumEarned int64 `json:"premium_earned"`
}

type economyDashboardMarket struct {
	ActiveListings int   `json:"active_listings"`
	SoldListings   int   `json:"sold_listings"`
	VolumeCredits  int64 `json:"volume_credits"`
}

type economyDashboardAuction struct {
	ActiveLots int `json:"active_lots"`
	ClosedLots int `json:"closed_lots"`
	Grants     int `json:"grants"`
}

type economyDashboardPremium struct {
	PendingEntitlements  int   `json:"pending_entitlements"`
	ClaimedEntitlements  int   `json:"claimed_entitlements"`
	WeeklyStockRemaining int64 `json:"weekly_stock_remaining"`
}

func (runtime *Runtime) handleWalletSnapshot(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return marshalPayload(map[string]any{"wallet": runtime.walletSnapshotLocked(ctx.PlayerID)})
}

func (runtime *Runtime) handleMarketSearch(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var filter struct {
		ItemID string `json:"item_id,omitempty"`
	}
	if err := decodeStrict(request.Payload, &filter); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return marshalPayload(map[string]any{"market": runtime.marketSearchPayload(ctx.PlayerID, filter.ItemID)})
}

func (runtime *Runtime) handleMarketCreateListing(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		ItemID         string `json:"item_id"`
		ItemInstanceID string `json:"item_instance_id,omitempty"`
		SourceLocation string `json:"source_location,omitempty"`
		Quantity       int64  `json:"quantity"`
		UnitPrice      int64  `json:"unit_price"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	itemID, err := foundation.ParseItemID(payload.ItemID)
	if err != nil {
		return nil, invalidPayload("Market item is invalid.", err)
	}
	listingID, err := foundation.ParseListingID("listing-" + request.RequestID.String())
	if err != nil {
		return nil, invalidPayload("Market listing reference is invalid.", err)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	definition, ok := runtime.itemCatalog[itemID]
	if !ok {
		return nil, foundation.NewDomainError(foundation.CodeNotFound, "Item was not found.")
	}
	sourceLocation, err := runtime.resolveMarketSourceLocationLocked(ctx.PlayerID, payload.SourceLocation)
	if err != nil {
		return nil, invalidPayload("Market source location is invalid.", err)
	}
	var instanceID foundation.ItemID
	if payload.ItemInstanceID != "" {
		instanceID, err = foundation.ParseItemID(payload.ItemInstanceID)
		if err != nil {
			return nil, invalidPayload("Market item instance is invalid.", err)
		}
	}
	result, err := runtime.Market.CreateListing(market.CreateListingInput{
		ListingID:      listingID,
		SellerPlayerID: ctx.PlayerID,
		ItemRef:        economy.MoveItemRef{Definition: definition, ItemInstanceID: instanceID},
		SourceLocation: sourceLocation,
		Quantity:       payload.Quantity,
		UnitPrice:      payload.UnitPrice,
		Currency:       economy.CurrencyBucketCredits,
	})
	if err != nil {
		return nil, domainErrorForEconomy(err)
	}
	if !result.Duplicate {
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventMarketListingCreated, marketListingPayloadFromListing(result.Listing, ctx.PlayerID))
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventInventorySnapshot, runtime.inventorySnapshotLocked(ctx.PlayerID))
		runtime.queuePassiveMarketListingEventLocked(realtime.EventMarketListingCreated, result.Listing, ctx.PlayerID)
	}
	return marshalPayload(runtime.marketMutationResponseLocked(ctx.PlayerID, result.Listing, 0, 0, 0, result.Duplicate))
}

func (runtime *Runtime) handleMarketBuy(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		ListingID string `json:"listing_id"`
		Quantity  int64  `json:"quantity"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	listingID, err := foundation.ParseListingID(payload.ListingID)
	if err != nil {
		return nil, invalidPayload("Market listing is invalid.", err)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.coordinateListingBuyBlockedByClaimLocked(listingID) {
		return nil, domainErrorForEconomy(market.ErrListingNotActive)
	}
	if err := runtime.validateCoordinateListingBuyPreflightLocked(listingID); err != nil {
		return nil, intelDomainError(err)
	}
	result, err := runtime.Market.BuyListing(market.BuyListingInput{
		BuyerPlayerID: ctx.PlayerID,
		ListingID:     listingID,
		Quantity:      payload.Quantity,
		RequestID:     request.RequestID,
	})
	if err != nil {
		return nil, domainErrorForEconomy(err)
	}
	if err := runtime.transferBoughtCoordinateItemLocked(result, ctx.PlayerID); err != nil {
		return nil, intelDomainError(err)
	}
	if !result.Duplicate && runtime.Metrics != nil {
		_ = runtime.Metrics.RecordMarketSale(result.Listing.Currency.String(), result.Listing.ItemID, result.Quantity, result.TotalAmount)
		runtime.recordCurrencyLedgerMetric(result.BuyerDebit.LedgerEntry)
		runtime.recordCurrencyLedgerMetric(result.SellerCredit.LedgerEntry)
		if result.FeeCredit != nil {
			runtime.recordCurrencyLedgerMetric(result.FeeCredit.LedgerEntry)
		}
		runtime.recordItemLedgerMetrics(result.ItemMove.LedgerEntries)
	}
	wallet := runtime.walletSnapshotLocked(ctx.PlayerID)
	state := runtime.players[ctx.PlayerID]
	state.Wallet = wallet
	runtime.players[ctx.PlayerID] = state
	if !result.Duplicate {
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventMarketSaleCompleted, marketSaleEventPayload(result.Listing, ctx.PlayerID, result.Quantity, result.TotalAmount, result.FeeAmount))
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventWalletSnapshot, wallet)
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventInventorySnapshot, runtime.inventorySnapshotLocked(ctx.PlayerID))

		sellerID := result.Listing.SellerPlayerID
		sellerWallet := runtime.walletSnapshotLocked(sellerID)
		if sellerState, ok := runtime.players[sellerID]; ok {
			sellerState.Wallet = sellerWallet
			runtime.players[sellerID] = sellerState
		}
		runtime.queueEventToPlayerSessionsLocked(sellerID, realtime.EventMarketSaleCompleted, marketSaleEventPayload(result.Listing, sellerID, result.Quantity, result.TotalAmount, result.FeeAmount))
		runtime.queueEventToPlayerSessionsLocked(sellerID, realtime.EventWalletSnapshot, sellerWallet)
		runtime.queuePassiveMarketListingEventLocked(realtime.EventMarketListingUpdated, result.Listing, ctx.PlayerID, sellerID)
	}
	return marshalPayload(runtime.marketMutationResponseLocked(ctx.PlayerID, result.Listing, result.Quantity, result.TotalAmount, result.FeeAmount, result.Duplicate))
}

func (runtime *Runtime) handleMarketCancel(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		ListingID string `json:"listing_id"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	listingID, err := foundation.ParseListingID(payload.ListingID)
	if err != nil {
		return nil, invalidPayload("Market listing is invalid.", err)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	result, err := runtime.Market.CancelListing(market.CancelListingInput{
		SellerPlayerID: ctx.PlayerID,
		ListingID:      listingID,
	})
	if err != nil {
		return nil, domainErrorForEconomy(err)
	}
	if !result.Duplicate {
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventMarketListingCanceled, marketListingPayloadFromListing(result.Listing, ctx.PlayerID))
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventInventorySnapshot, runtime.inventorySnapshotLocked(ctx.PlayerID))
		runtime.queuePassiveMarketListingEventLocked(realtime.EventMarketListingCanceled, result.Listing, ctx.PlayerID)
	}
	return marshalPayload(runtime.marketMutationResponseLocked(ctx.PlayerID, result.Listing, result.ReturnedQuantity, 0, 0, result.Duplicate))
}

func (runtime *Runtime) handleAuctionSearch(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return marshalPayload(map[string]any{"auction": runtime.auctionSearchPayload(ctx.PlayerID)})
}

func (runtime *Runtime) handleAuctionBid(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		AuctionID string `json:"auction_id"`
		Amount    int64  `json:"amount"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	auctionID, err := foundation.ParseAuctionID(payload.AuctionID)
	if err != nil {
		return nil, invalidPayload("Auction lot is invalid.", err)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	result, err := runtime.Auction.PlaceBid(auction.PlaceBidInput{
		AuctionID:      auctionID,
		BidderPlayerID: ctx.PlayerID,
		Amount:         payload.Amount,
		RequestID:      request.RequestID,
	})
	if err != nil {
		return nil, domainErrorForEconomy(err)
	}
	if !result.Duplicate && runtime.Metrics != nil {
		_ = runtime.Metrics.RecordAuctionBid(result.Lot.Currency.String(), result.Amount)
		runtime.recordCurrencyLedgerMetric(result.BidderDebit.LedgerEntry)
		if result.PreviousRefund != nil {
			runtime.recordCurrencyLedgerMetric(result.PreviousRefund.LedgerEntry)
		}
	}
	if !result.Duplicate {
		bidderWallet := runtime.walletSnapshotLocked(ctx.PlayerID)
		runtime.updatePlayerWalletCacheLocked(ctx.PlayerID, bidderWallet)
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventAuctionBidPlaced, auctionLotPayloadFromLot(result.Lot, ctx.PlayerID))
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventAuctionLotUpdated, auctionLotPayloadFromLot(result.Lot, ctx.PlayerID))
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventWalletSnapshot, bidderWallet)

		excludedPlayers := []foundation.PlayerID{ctx.PlayerID}
		if result.PreviousRefund != nil {
			previousBidderID := result.PreviousRefund.Balance.PlayerID
			previousWallet := runtime.walletSnapshotLocked(previousBidderID)
			runtime.updatePlayerWalletCacheLocked(previousBidderID, previousWallet)
			runtime.queueEventToPlayerSessionsLocked(previousBidderID, realtime.EventAuctionLotUpdated, auctionLotPayloadFromLot(result.Lot, previousBidderID))
			runtime.queueEventToPlayerSessionsLocked(previousBidderID, realtime.EventWalletSnapshot, previousWallet)
			excludedPlayers = append(excludedPlayers, previousBidderID)
		}
		runtime.queuePassiveAuctionLotUpdatedLocked(result.Lot, excludedPlayers...)
	}
	return marshalPayload(runtime.auctionMutationResponseLocked(ctx.PlayerID, result.Lot, result.Amount, 0, nil, result.Duplicate))
}

func (runtime *Runtime) handleAuctionBuyNow(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		AuctionID string `json:"auction_id"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	auctionID, err := foundation.ParseAuctionID(payload.AuctionID)
	if err != nil {
		return nil, invalidPayload("Auction lot is invalid.", err)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	result, err := runtime.Auction.BuyNow(auction.BuyNowInput{
		AuctionID:     auctionID,
		BuyerPlayerID: ctx.PlayerID,
		RequestID:     request.RequestID,
	})
	if err != nil {
		return nil, domainErrorForEconomy(err)
	}
	if !result.Duplicate && runtime.Metrics != nil {
		_ = runtime.Metrics.RecordAuctionClearing(result.Lot.Currency.String(), foundation.ItemID(result.Grant.Payload.Source.DefinitionID), result.Grant.Payload.Quantity, result.Price)
		runtime.recordCurrencyLedgerMetric(result.BuyerDebit.LedgerEntry)
		if result.CurrentRefund != nil {
			runtime.recordCurrencyLedgerMetric(result.CurrentRefund.LedgerEntry)
		}
	}
	grant := auctionGrantPayloadFromGrant(result.Grant)
	if !result.Duplicate {
		buyerWallet := runtime.walletSnapshotLocked(ctx.PlayerID)
		runtime.updatePlayerWalletCacheLocked(ctx.PlayerID, buyerWallet)
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventAuctionClosed, map[string]any{"lot": auctionLotPayloadFromLot(result.Lot, ctx.PlayerID), "grant": grant})
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventAuctionLotUpdated, auctionLotPayloadFromLot(result.Lot, ctx.PlayerID))
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventWalletSnapshot, buyerWallet)

		excludedPlayers := []foundation.PlayerID{ctx.PlayerID}
		if result.CurrentRefund != nil {
			refundedBidderID := result.CurrentRefund.Balance.PlayerID
			if refundedBidderID != ctx.PlayerID {
				refundedWallet := runtime.walletSnapshotLocked(refundedBidderID)
				runtime.updatePlayerWalletCacheLocked(refundedBidderID, refundedWallet)
				runtime.queueEventToPlayerSessionsLocked(refundedBidderID, realtime.EventAuctionLotUpdated, auctionLotPayloadFromLot(result.Lot, refundedBidderID))
				runtime.queueEventToPlayerSessionsLocked(refundedBidderID, realtime.EventWalletSnapshot, refundedWallet)
				excludedPlayers = append(excludedPlayers, refundedBidderID)
			}
		}
		runtime.queuePassiveAuctionLotUpdatedLocked(result.Lot, excludedPlayers...)
	}
	return marshalPayload(runtime.auctionMutationResponseLocked(ctx.PlayerID, result.Lot, 0, result.Price, &grant, result.Duplicate))
}

func (runtime *Runtime) handleAuctionGrants(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return marshalPayload(map[string]any{"auction": runtime.auctionSearchPayload(ctx.PlayerID)})
}

func (runtime *Runtime) handlePremiumEntitlements(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return marshalPayload(map[string]any{"premium": runtime.premiumSummaryPayload(ctx.PlayerID)})
}

func (runtime *Runtime) handlePremiumClaim(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		EntitlementID string `json:"entitlement_id"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	entitlementID := premium.EntitlementID(payload.EntitlementID)
	if err := entitlementID.Validate(); err != nil {
		return nil, invalidPayload("Premium entitlement is invalid.", err)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	result, err := runtime.Premium.ClaimEntitlement(premium.ClaimEntitlementInput{
		EntitlementID:    entitlementID,
		PlayerID:         ctx.PlayerID,
		RequestReference: request.RequestID.String(),
	})
	if err != nil {
		return nil, domainErrorForEconomy(err)
	}
	if !result.Duplicate && result.WalletCredit != nil {
		runtime.recordCurrencyLedgerMetric(result.WalletCredit.LedgerEntry)
	}
	wallet := runtime.walletSnapshotLocked(ctx.PlayerID)
	runtime.updatePlayerWalletCacheLocked(ctx.PlayerID, wallet)
	if !result.Duplicate {
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventPremiumClaimed, premiumEntitlementPayloadFromEntitlement(result.Entitlement))
		if result.WalletCredit != nil {
			runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventWalletSnapshot, wallet)
		}
	}
	return marshalPayload(premiumMutationPayload{
		Accepted:  true,
		Duplicate: result.Duplicate,
		Premium:   runtime.premiumSummaryPayload(ctx.PlayerID),
		Wallet:    wallet,
	})
}

func (runtime *Runtime) handlePremiumWeeklyXCore(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		ProductID string `json:"product_id"`
		PeriodKey string `json:"period_key"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if payload.ProductID != premiumWeeklyXCoreProductID {
		return nil, invalidPayload("Premium product is invalid.", nil)
	}
	if payload.PeriodKey == "" || payload.PeriodKey != runtime.currentPremiumPeriodKey() {
		return nil, invalidPayload("Premium stock period is not available.", nil)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	result, err := runtime.Premium.PurchaseWeeklyXCore(premium.PurchaseWeeklyXCoreInput{
		PlayerID:          ctx.PlayerID,
		WorldID:           runtime.worldID,
		PeriodKey:         payload.PeriodKey,
		PurchaseReference: request.RequestID.String(),
		PaymentCurrency:   economy.CurrencyBucketPremiumPaid,
		PriceAmount:       runtime.starterContent.WeeklyXCore.PremiumPrice,
	})
	if err != nil {
		return nil, domainErrorForEconomy(err)
	}
	if !result.Duplicate && result.WalletDebit != nil {
		runtime.recordCurrencyLedgerMetric(result.WalletDebit.LedgerEntry)
	}
	wallet := runtime.walletSnapshotLocked(ctx.PlayerID)
	runtime.updatePlayerWalletCacheLocked(ctx.PlayerID, wallet)
	if !result.Duplicate {
		stockPayload := runtime.premiumStockPayloadFromRecord(result.Stock)
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventPremiumStockConsumed, stockPayload)
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventWalletSnapshot, wallet)
		runtime.queuePassivePremiumStockConsumedLocked(ctx.PlayerID, stockPayload)
	}
	return marshalPayload(premiumMutationPayload{
		Accepted:  true,
		Duplicate: result.Duplicate,
		Premium:   runtime.premiumSummaryPayload(ctx.PlayerID),
		Wallet:    wallet,
	})
}

func (runtime *Runtime) handleAdminEconomyDashboard(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	resolved, err := runtime.Auth.ResolveSessionID(context.Background(), authSessionID(ctx.SessionID))
	if err != nil {
		return nil, err
	}
	if !hasRole(resolved.Roles, auth.RoleAdmin) {
		return nil, foundation.NewDomainError(foundation.CodeForbidden, "Admin economy dashboard is restricted.")
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return marshalPayload(map[string]any{"economy": runtime.economyDashboardPayload()})
}

func (runtime *Runtime) marketMutationResponseLocked(playerID foundation.PlayerID, listing market.Listing, quantity int64, serverTotal int64, serverFee int64, duplicate bool) marketMutationPayload {
	return marketMutationPayload{
		Accepted:          true,
		Duplicate:         duplicate,
		Listing:           marketListingPayloadFromListing(listing, playerID),
		Quantity:          quantity,
		ServerTotal:       serverTotal,
		ServerFee:         serverFee,
		FinalPricePending: true,
		Market:            runtime.marketSearchPayload(playerID, ""),
		Wallet:            runtime.walletSnapshotLocked(playerID),
		Inventory:         runtime.inventorySnapshotLocked(playerID),
	}
}

func (runtime *Runtime) queueEventToPlayerSessionsLocked(playerID foundation.PlayerID, eventType realtime.ClientEventType, payload any) {
	for sessionID, sessionPlayerID := range runtime.sessions {
		if sessionPlayerID != playerID {
			continue
		}
		runtime.queueEventLocked(sessionID, eventType, payload)
	}
}

func (runtime *Runtime) updatePlayerWalletCacheLocked(playerID foundation.PlayerID, wallet walletSnapshotPayload) {
	state, ok := runtime.players[playerID]
	if !ok {
		return
	}
	state.Wallet = wallet
	runtime.players[playerID] = state
}

func (runtime *Runtime) queuePassiveMarketListingEventLocked(eventType realtime.ClientEventType, listing market.Listing, excludedPlayers ...foundation.PlayerID) {
	excluded := make(map[foundation.PlayerID]struct{}, len(excludedPlayers))
	for _, playerID := range excludedPlayers {
		excluded[playerID] = struct{}{}
	}
	for sessionID, playerID := range runtime.sessions {
		if _, skip := excluded[playerID]; skip {
			continue
		}
		runtime.queueEventLocked(sessionID, eventType, marketListingPayloadFromListing(listing, playerID))
	}
}

func (runtime *Runtime) queueClaimStaleMarketListingEventsLocked(planetID foundation.PlanetID) {
	for _, listing := range runtime.Market.Listings() {
		if !runtime.marketListingMatchesCoordinatePlanetLocked(listing, planetID) ||
			listing.Status != market.ListingStatusStale ||
			listing.StaleReason != "planet_claimed" {
			continue
		}
		runtime.queuePassiveMarketListingEventLocked(realtime.EventMarketListingUpdated, listing)
	}
}

func (runtime *Runtime) coordinateListingBuyBlockedByClaimLocked(listingID foundation.ListingID) bool {
	listing, ok := runtime.Market.Listing(listingID)
	if !ok || listing.Status != market.ListingStatusActive {
		return false
	}
	planetID, ok := runtime.coordinatePlanetForMarketListingLocked(listing)
	if !ok {
		return false
	}
	if runtime.activePlanetClaims[planetID] > 0 {
		return true
	}
	planet, ok, err := runtime.Discovery.Planet(planetID)
	if err != nil || !ok {
		return false
	}
	return !planet.OwnerPlayerID.IsZero()
}

func (runtime *Runtime) validateCoordinateListingBuyPreflightLocked(listingID foundation.ListingID) error {
	listing, ok := runtime.Market.Listing(listingID)
	if !ok || listing.Status != market.ListingStatusActive {
		return nil
	}
	if listing.ItemID != coordinateScrollItemID || listing.ItemInstanceID.IsZero() {
		return nil
	}
	item, ok, err := runtime.Intel.CoordinateItem(listing.ItemInstanceID)
	if err != nil {
		return err
	}
	if !ok {
		return intel.ErrCoordinateItemNotFound
	}
	if item.UsedAt != nil {
		return intel.ErrCoordinateItemAlreadyUsed
	}
	if item.OwnerPlayerID != listing.SellerPlayerID {
		return intel.ErrCoordinateItemNotOwned
	}
	return nil
}

func (runtime *Runtime) transferBoughtCoordinateItemLocked(result market.BuyListingResult, buyerID foundation.PlayerID) error {
	if result.Listing.ItemID != coordinateScrollItemID || result.Listing.ItemInstanceID.IsZero() {
		return nil
	}
	_, err := runtime.Intel.TransferCoordinateItemOwner(intel.TransferCoordinateItemOwnerInput{
		FromPlayerID:   result.Listing.SellerPlayerID,
		ToPlayerID:     buyerID,
		ItemInstanceID: result.Listing.ItemInstanceID,
		Reference:      result.ReferenceKey,
	})
	return err
}

func (runtime *Runtime) marketListingMatchesCoordinatePlanetLocked(listing market.Listing, planetID foundation.PlanetID) bool {
	listingPlanetID, ok := runtime.coordinatePlanetForMarketListingLocked(listing)
	return ok && listingPlanetID == planetID
}

func (runtime *Runtime) coordinatePlanetForMarketListingLocked(listing market.Listing) (foundation.PlanetID, bool) {
	if listing.ItemID != coordinateScrollItemID || listing.ItemInstanceID.IsZero() {
		return "", false
	}
	item, ok, err := runtime.Intel.CoordinateItem(listing.ItemInstanceID)
	if err != nil || !ok || item.UsedAt != nil {
		return "", false
	}
	return item.PlanetID, true
}

func (runtime *Runtime) queuePassiveAuctionLotUpdatedLocked(lot auction.Lot, excludedPlayers ...foundation.PlayerID) {
	excluded := make(map[foundation.PlayerID]struct{}, len(excludedPlayers))
	for _, playerID := range excludedPlayers {
		excluded[playerID] = struct{}{}
	}
	for sessionID, playerID := range runtime.sessions {
		if _, skip := excluded[playerID]; skip {
			continue
		}
		runtime.queueEventLocked(sessionID, realtime.EventAuctionLotUpdated, auctionLotPayloadFromLot(lot, playerID))
	}
}

func (runtime *Runtime) queuePassivePremiumStockConsumedLocked(excludedPlayer foundation.PlayerID, payload premiumStockPayload) {
	for sessionID, playerID := range runtime.sessions {
		if playerID == excludedPlayer {
			continue
		}
		runtime.queueEventLocked(sessionID, realtime.EventPremiumStockConsumed, payload)
	}
}

func marketSaleEventPayload(listing market.Listing, viewerID foundation.PlayerID, quantity int64, serverTotal int64, serverFee int64) map[string]any {
	return map[string]any{
		"listing":      marketListingPayloadFromListing(listing, viewerID),
		"quantity":     quantity,
		"server_total": serverTotal,
		"server_fee":   serverFee,
	}
}

func (runtime *Runtime) auctionMutationResponseLocked(playerID foundation.PlayerID, lot auction.Lot, amount int64, price int64, grant *auctionGrantPayload, duplicate bool) auctionMutationPayload {
	return auctionMutationPayload{
		Accepted:          true,
		Duplicate:         duplicate,
		Lot:               auctionLotPayloadFromLot(lot, playerID),
		Amount:            amount,
		Price:             price,
		Grant:             grant,
		Auction:           runtime.auctionSearchPayload(playerID),
		Wallet:            runtime.walletSnapshotLocked(playerID),
		FinalPricePending: true,
	}
}

func (runtime *Runtime) marketSearchPayload(playerID foundation.PlayerID, itemFilter string) marketSearchPayload {
	listings := runtime.Market.Listings()
	payload := make([]marketListingPayload, 0, len(listings))
	counts := marketCountsPayload{}
	for _, listing := range listings {
		if itemFilter != "" && listing.ItemID.String() != itemFilter {
			continue
		}
		if listing.Status == market.ListingStatusActive {
			counts.Active++
		}
		if listing.SellerPlayerID == playerID {
			counts.Mine++
		}
		payload = append(payload, marketListingPayloadFromListing(listing, playerID))
	}
	return marketSearchPayload{Listings: payload, Counts: counts}
}

func marketListingPayloadFromListing(listing market.Listing, viewerID foundation.PlayerID) marketListingPayload {
	estimate := marketEstimatePayload{
		Quantity: 1,
		Subtotal: listing.UnitPrice,
		Currency: listing.Currency.String(),
		Pending:  true,
	}
	payload := marketListingPayload{
		ListingID:             listing.ListingID.String(),
		ItemID:                listing.ItemID.String(),
		DisplayName:           listing.ItemDefinition.Name,
		Rarity:                listing.ItemDefinition.Rarity.String(),
		RemainingQuantity:     listing.RemainingQuantity,
		UnitPrice:             listing.UnitPrice,
		Currency:              listing.Currency.String(),
		Status:                listing.Status.String(),
		OwnedByYou:            listing.SellerPlayerID == viewerID,
		FinalPricePending:     true,
		EstimatedUnitPurchase: estimate,
	}
	if listing.ExpiresAt != nil {
		payload.ExpiresAt = listing.ExpiresAt.UTC().UnixMilli()
	}
	return payload
}

func (runtime *Runtime) auctionSearchPayload(playerID foundation.PlayerID) auctionSearchPayload {
	lots := runtime.Auction.Lots()
	payload := make([]auctionLotPayload, 0, len(lots))
	for _, lot := range lots {
		payload = append(payload, auctionLotPayloadFromLot(lot, playerID))
	}
	grants := runtime.playerAuctionGrants(playerID)
	return auctionSearchPayload{Lots: payload, Grants: grants}
}

func auctionLotPayloadFromLot(lot auction.Lot, viewerID foundation.PlayerID) auctionLotPayload {
	return auctionLotPayload{
		AuctionID:         lot.AuctionID.String(),
		PayloadType:       lot.Payload.Type.String(),
		DefinitionID:      lot.Payload.Source.DefinitionID.String(),
		Quantity:          lot.Payload.Quantity,
		Currency:          lot.Currency.String(),
		StartPrice:        lot.StartPrice,
		CurrentBid:        lot.CurrentBid,
		HasBid:            !lot.CurrentBidderID.IsZero(),
		Leading:           lot.Status == auction.LotStatusActive && lot.CurrentBidderID == viewerID,
		BuyNowPrice:       cloneInt64(lot.BuyNowPrice),
		Status:            lot.Status.String(),
		StartsAt:          lot.StartsAt.UTC().UnixMilli(),
		EndsAt:            lot.EndsAt.UTC().UnixMilli(),
		FinalPricePending: true,
	}
}

func (runtime *Runtime) playerAuctionGrants(playerID foundation.PlayerID) []auctionGrantPayload {
	grants := runtime.Auction.Grants()
	payload := make([]auctionGrantPayload, 0)
	for _, grant := range grants {
		if grant.PlayerID != playerID {
			continue
		}
		payload = append(payload, auctionGrantPayloadFromGrant(grant))
	}
	return payload
}

func auctionGrantPayloadFromGrant(grant auction.Grant) auctionGrantPayload {
	return auctionGrantPayload{
		AuctionID:    grant.AuctionID.String(),
		PayloadType:  grant.Payload.Type.String(),
		DefinitionID: grant.Payload.Source.DefinitionID.String(),
		Quantity:     grant.Payload.Quantity,
		Reason:       string(grant.Reason),
		GrantedAt:    grant.GrantedAt.UTC().UnixMilli(),
	}
}

func (runtime *Runtime) premiumSummaryPayload(playerID foundation.PlayerID) premiumSummaryPayload {
	entitlements := runtime.Premium.Entitlements()
	entitlementPayloads := make([]premiumEntitlementPayload, 0)
	for _, entitlement := range entitlements {
		if entitlement.PlayerID != playerID {
			continue
		}
		entitlementPayloads = append(entitlementPayloads, premiumEntitlementPayloadFromEntitlement(entitlement))
	}
	stockRecords := runtime.Premium.WeeklyXCoreStockRecords()
	stocks := make([]premiumStockPayload, 0, len(stockRecords))
	for _, record := range stockRecords {
		stocks = append(stocks, runtime.premiumStockPayloadFromRecord(record))
	}
	purchases := runtime.Premium.WeeklyXCorePurchases()
	purchasePayloads := make([]premiumPurchasePayload, 0)
	for _, purchase := range purchases {
		if purchase.PlayerID != playerID {
			continue
		}
		purchasePayloads = append(purchasePayloads, premiumPurchasePayload{
			PeriodKey:       purchase.PeriodKey,
			PaymentCurrency: purchase.PaymentCurrency.String(),
			GrantedAt:       purchase.GrantedAt.UTC().UnixMilli(),
		})
	}
	return premiumSummaryPayload{
		Entitlements: entitlementPayloads,
		Stock:        stocks,
		Purchases:    purchasePayloads,
	}
}

func premiumEntitlementPayloadFromEntitlement(entitlement premium.Entitlement) premiumEntitlementPayload {
	payload := premiumEntitlementPayload{
		EntitlementID: entitlement.ID.String(),
		Type:          entitlement.Type.String(),
		State:         entitlement.State.String(),
		Payload: premiumEntitlementGrantInfo{
			CurrencyBucket:   entitlement.Payload.CurrencyBucket.String(),
			Amount:           entitlement.Payload.Amount,
			LoadoutSlotScope: entitlement.Payload.LoadoutSlotScope,
			LoadoutSlotCount: entitlement.Payload.LoadoutSlotCount,
			PeriodKey:        entitlement.Payload.PeriodKey,
			CosmeticID:       entitlement.Payload.CosmeticID,
			BadgeID:          entitlement.Payload.BadgeID,
		},
		CreatedAt: entitlement.CreatedAt.UTC().UnixMilli(),
	}
	if !entitlement.ClaimedAt.IsZero() {
		payload.ClaimedAt = entitlement.ClaimedAt.UTC().UnixMilli()
	}
	return payload
}

func (runtime *Runtime) premiumStockPayloadFromRecord(record premium.WeeklyXCoreStockRecord) premiumStockPayload {
	return premiumStockPayload{
		PeriodKey:       record.PeriodKey,
		StockTotal:      record.StockTotal,
		StockRemaining:  record.StockRemaining,
		PriceAmount:     runtime.starterContent.WeeklyXCore.PremiumPrice,
		PaymentCurrency: economy.CurrencyBucketPremiumPaid.String(),
	}
}

func (runtime *Runtime) economyDashboardPayload() economyDashboardPayload {
	dashboard := economyDashboardPayload{Generated: runtime.clock.Now().UTC().UnixMilli()}
	for _, balance := range runtime.Wallet.WalletBalances() {
		switch balance.Currency {
		case economy.CurrencyBucketCredits:
			dashboard.Wallets.Credits += balance.Balance
		case economy.CurrencyBucketPremiumPaid:
			dashboard.Wallets.PremiumPaid += balance.Balance
		case economy.CurrencyBucketPremiumEarned:
			dashboard.Wallets.PremiumEarned += balance.Balance
		}
	}
	for _, listing := range runtime.Market.Listings() {
		switch listing.Status {
		case market.ListingStatusActive:
			dashboard.Market.ActiveListings++
		case market.ListingStatusSold:
			dashboard.Market.SoldListings++
		}
		sold := listing.OriginalQuantity - listing.RemainingQuantity
		if sold > 0 && listing.Currency == economy.CurrencyBucketCredits {
			dashboard.Market.VolumeCredits += sold * listing.UnitPrice
		}
	}
	for _, lot := range runtime.Auction.Lots() {
		if lot.Status == auction.LotStatusActive {
			dashboard.Auction.ActiveLots++
		}
		if lot.Status == auction.LotStatusClosed {
			dashboard.Auction.ClosedLots++
		}
	}
	dashboard.Auction.Grants = len(runtime.Auction.Grants())
	for _, entitlement := range runtime.Premium.Entitlements() {
		switch entitlement.State {
		case premium.EntitlementStatePending:
			dashboard.Premium.PendingEntitlements++
		case premium.EntitlementStateClaimed:
			dashboard.Premium.ClaimedEntitlements++
		}
	}
	for _, stock := range runtime.Premium.WeeklyXCoreStockRecords() {
		dashboard.Premium.WeeklyStockRemaining += stock.StockRemaining
	}
	return dashboard
}

func (runtime *Runtime) resolveMarketSourceLocationLocked(playerID foundation.PlayerID, publicLocation string) (economy.ItemLocation, error) {
	switch publicLocation {
	case "", economy.LocationKindAccountInventory.String():
		return economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	case economy.LocationKindShipCargo.String():
		return runtime.activeCargoLocationLocked(playerID), nil
	default:
		return economy.ItemLocation{}, economy.ErrInvalidLocationKind
	}
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func domainErrorForEconomy(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, economy.ErrInsufficientWalletFunds):
		return foundation.NewDomainError(foundation.CodeNotEnoughFunds, "Not enough funds.", foundation.WithCause(err))
	case errors.Is(err, economy.ErrInsufficientItemQuantity), errors.Is(err, economy.ErrItemNotOwned), errors.Is(err, market.ErrMarketEscrowQuantityMissing):
		return foundation.NewDomainError(foundation.CodeNotEnoughCargo, "Not enough item quantity.", foundation.WithCause(err))
	case errors.Is(err, economy.ErrBlockedGenericMoveSource), errors.Is(err, market.ErrListingSourceLocation):
		return foundation.NewDomainError(foundation.CodeForbidden, "Item cannot be listed from that location.", foundation.WithCause(err))
	case errors.Is(err, market.ErrListingOwnership), errors.Is(err, premium.ErrEntitlementWrongPlayer):
		return foundation.NewDomainError(foundation.CodeForbidden, "Economy record is not owned by this player.", foundation.WithCause(err))
	case errors.Is(err, market.ErrListingNotFound), errors.Is(err, auction.ErrLotNotFound), errors.Is(err, premium.ErrEntitlementNotFound):
		return foundation.NewDomainError(foundation.CodeNotFound, "Economy record was not found.", foundation.WithCause(err))
	case errors.Is(err, market.ErrListingNotActive), errors.Is(err, market.ErrListingExpired), errors.Is(err, auction.ErrLotNotActive), errors.Is(err, auction.ErrLotEnded), errors.Is(err, auction.ErrLotNotStarted):
		return foundation.NewDomainError(foundation.CodeForbidden, "Economy record is not active.", foundation.WithCause(err))
	case errors.Is(err, market.ErrSellerCannotBuyOwnListing), errors.Is(err, auction.ErrCurrentWinningBidder), errors.Is(err, premium.ErrWeeklyLimitReached):
		return foundation.NewDomainError(foundation.CodeForbidden, "Economy action is not allowed.", foundation.WithCause(err))
	case errors.Is(err, market.ErrCreateListingReferenceMismatch):
		return foundation.NewDomainError(foundation.CodeInvalidPayload, "Economy request was already used with different details.", foundation.WithCause(err))
	case errors.Is(err, foundation.ErrNonPositiveAmount), errors.Is(err, auction.ErrBidTooLow), errors.Is(err, auction.ErrBidReachesBuyNow), errors.Is(err, auction.ErrBuyNowUnavailable), errors.Is(err, premium.ErrWeeklyStockSoldOut), errors.Is(err, premium.ErrWeeklyStockNotSet):
		return foundation.NewDomainError(foundation.CodeInvalidPayload, "Economy amount is not valid.", foundation.WithCause(err))
	case errors.Is(err, economy.ErrInvalidTradeFlag):
		return foundation.NewDomainError(foundation.CodeItemNotTradeable, "Item is not tradeable.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Economy command failed.", foundation.WithCause(err))
	}
}
