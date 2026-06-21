package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"gameproject/internal/game/auction"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/market"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/premium"
	"gameproject/internal/game/realtime"
)

func TestPhase08MarketAuctionPremiumUseServerEconomyState(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-wallet-snapshot","op":"wallet.snapshot","payload":{},"client_seq":1,"v":1}`)
	walletResponse := readResponse(t, conn)
	if !walletResponse.OK {
		t.Fatalf("wallet response = %+v, want success", walletResponse)
	}
	var walletPayload struct {
		Wallet walletSnapshotPayload `json:"wallet"`
	}
	if err := json.Unmarshal(walletResponse.Payload, &walletPayload); err != nil {
		t.Fatalf("decode wallet: %v", err)
	}
	if walletPayload.Wallet.Credits != starterWalletCredits || walletPayload.Wallet.PremiumPaid != starterWalletPremiumPaid {
		t.Fatalf("wallet = %+v, want deterministic starter balances", walletPayload.Wallet)
	}

	writeText(t, conn, `{"request_id":"request-market-search","op":"market.search","payload":{},"client_seq":2,"v":1}`)
	marketResponse := readResponse(t, conn)
	if !marketResponse.OK {
		t.Fatalf("market response = %+v, want success", marketResponse)
	}
	assertNoEconomyLeak(t, "market search", marketResponse.Payload)
	var marketPayload struct {
		Market marketSearchPayload `json:"market"`
	}
	if err := json.Unmarshal(marketResponse.Payload, &marketPayload); err != nil {
		t.Fatalf("decode market: %v", err)
	}
	if len(marketPayload.Market.Listings) != 1 || marketPayload.Market.Listings[0].ListingID != seedMarketListingID.String() {
		t.Fatalf("market listings = %+v, want seeded listing", marketPayload.Market.Listings)
	}
	if !marketPayload.Market.Listings[0].FinalPricePending {
		t.Fatalf("market listing = %+v, want final price pending marker", marketPayload.Market.Listings[0])
	}

	writeText(t, conn, `{"request_id":"request-market-spoof","op":"market.buy","payload":{"listing_id":"`+seedMarketListingID.String()+`","quantity":1,"total_amount":1},"client_seq":3,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed market buy error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, conn, `{"request_id":"request-market-buy","op":"market.buy","payload":{"listing_id":"`+seedMarketListingID.String()+`","quantity":2},"client_seq":4,"v":1}`)
	buyResponse := readResponse(t, conn)
	if !buyResponse.OK {
		t.Fatalf("market buy response = %+v, want success", buyResponse)
	}
	assertNoEconomyLeak(t, "market buy", buyResponse.Payload)
	var buyPayload marketMutationPayload
	if err := json.Unmarshal(buyResponse.Payload, &buyPayload); err != nil {
		t.Fatalf("decode market buy: %v", err)
	}
	if buyPayload.ServerTotal != 50 || buyPayload.Wallet.Credits != starterWalletCredits-50 {
		t.Fatalf("market buy = %+v, want server-calculated total and debited wallet", buyPayload)
	}
	if len(buyPayload.Inventory.Stackable) != 1 ||
		buyPayload.Inventory.Stackable[0].ItemID != "raw_ore" ||
		buyPayload.Inventory.Stackable[0].Quantity != 2 ||
		buyPayload.Inventory.Stackable[0].Location != economy.LocationKindAccountInventory.String() {
		t.Fatalf("market buy inventory = %+v, want purchased raw ore in account inventory", buyPayload.Inventory)
	}
	drainEventTypes(t, conn, realtime.EventMarketSaleCompleted, realtime.EventWalletSnapshot, realtime.EventInventorySnapshot)

	writeText(t, conn, `{"request_id":"request-auction-search","op":"auction.search","payload":{},"client_seq":5,"v":1}`)
	auctionResponse := readResponse(t, conn)
	if !auctionResponse.OK {
		t.Fatalf("auction search response = %+v, want success", auctionResponse)
	}
	assertNoEconomyLeak(t, "auction search", auctionResponse.Payload)
	var auctionPayload struct {
		Auction auctionSearchPayload `json:"auction"`
	}
	if err := json.Unmarshal(auctionResponse.Payload, &auctionPayload); err != nil {
		t.Fatalf("decode auction search: %v", err)
	}
	if len(auctionPayload.Auction.Lots) != 1 || auctionPayload.Auction.Lots[0].AuctionID != seedAuctionID.String() {
		t.Fatalf("auction lots = %+v, want seeded lot", auctionPayload.Auction.Lots)
	}

	writeText(t, conn, `{"request_id":"request-auction-bid","op":"auction.bid","payload":{"auction_id":"`+seedAuctionID.String()+`","amount":300},"client_seq":6,"v":1}`)
	bidResponse := readResponse(t, conn)
	if !bidResponse.OK {
		t.Fatalf("auction bid response = %+v, want success", bidResponse)
	}
	var bidPayload auctionMutationPayload
	if err := json.Unmarshal(bidResponse.Payload, &bidPayload); err != nil {
		t.Fatalf("decode auction bid: %v", err)
	}
	if bidPayload.Amount != 300 || bidPayload.Wallet.Credits != starterWalletCredits-50-300 || !bidPayload.Lot.Leading {
		t.Fatalf("auction bid = %+v, want debited leading bid", bidPayload)
	}
	drainEventTypes(t, conn, realtime.EventAuctionBidPlaced, realtime.EventAuctionLotUpdated, realtime.EventWalletSnapshot)

	writeText(t, conn, `{"request_id":"request-auction-buy-now","op":"auction.buy_now","payload":{"auction_id":"`+seedAuctionID.String()+`"},"client_seq":7,"v":1}`)
	buyNowResponse := readResponse(t, conn)
	if !buyNowResponse.OK {
		t.Fatalf("auction buy-now response = %+v, want success", buyNowResponse)
	}
	var buyNowPayload auctionMutationPayload
	if err := json.Unmarshal(buyNowResponse.Payload, &buyNowPayload); err != nil {
		t.Fatalf("decode auction buy-now: %v", err)
	}
	if buyNowPayload.Price != 650 || buyNowPayload.Grant == nil || buyNowPayload.Wallet.Credits != 500 {
		t.Fatalf("auction buy-now = %+v, want server price, grant, and refunded current bid", buyNowPayload)
	}
	drainEventTypes(t, conn, realtime.EventAuctionClosed, realtime.EventAuctionLotUpdated, realtime.EventWalletSnapshot)

	writeText(t, conn, `{"request_id":"request-auction-grants","op":"auction.grants","payload":{},"client_seq":8,"v":1}`)
	grantsResponse := readResponse(t, conn)
	if !grantsResponse.OK {
		t.Fatalf("auction grant response = %+v, want success", grantsResponse)
	}
	var grantsPayload struct {
		Auction auctionSearchPayload `json:"auction"`
	}
	if err := json.Unmarshal(grantsResponse.Payload, &grantsPayload); err != nil {
		t.Fatalf("decode auction grants: %v", err)
	}
	if len(grantsPayload.Auction.Grants) != 1 || grantsPayload.Auction.Grants[0].AuctionID != seedAuctionID.String() {
		t.Fatalf("auction grants = %+v, want player grant snapshot", grantsPayload.Auction.Grants)
	}

	writeText(t, conn, `{"request_id":"request-premium-entitlements","op":"premium.entitlements","payload":{},"client_seq":9,"v":1}`)
	premiumResponse := readResponse(t, conn)
	if !premiumResponse.OK {
		t.Fatalf("premium response = %+v, want success", premiumResponse)
	}
	assertNoEconomyLeak(t, "premium entitlements", premiumResponse.Payload)
	var premiumPayload struct {
		Premium premiumSummaryPayload `json:"premium"`
	}
	if err := json.Unmarshal(premiumResponse.Payload, &premiumPayload); err != nil {
		t.Fatalf("decode premium: %v", err)
	}
	if len(premiumPayload.Premium.Entitlements) != 1 || premiumPayload.Premium.Entitlements[0].State != premium.EntitlementStatePending.String() {
		t.Fatalf("premium entitlements = %+v, want one pending entitlement", premiumPayload.Premium.Entitlements)
	}
	entitlementID := premiumPayload.Premium.Entitlements[0].EntitlementID

	writeText(t, conn, `{"request_id":"request-premium-claim","op":"premium.claim","payload":{"entitlement_id":"`+entitlementID+`"},"client_seq":10,"v":1}`)
	claimResponse := readResponse(t, conn)
	if !claimResponse.OK {
		t.Fatalf("premium claim response = %+v, want success", claimResponse)
	}
	var claimPayload premiumMutationPayload
	if err := json.Unmarshal(claimResponse.Payload, &claimPayload); err != nil {
		t.Fatalf("decode premium claim: %v", err)
	}
	if claimPayload.Wallet.PremiumEarned != 50 || claimPayload.Premium.Entitlements[0].State != premium.EntitlementStateClaimed.String() {
		t.Fatalf("premium claim = %+v, want earned premium credit and claimed state", claimPayload)
	}
	drainEventTypes(t, conn, realtime.EventPremiumClaimed, realtime.EventWalletSnapshot)

	writeText(t, conn, `{"request_id":"request-weekly-xcore-empty","op":"premium.purchase_weekly_xcore","payload":{},"client_seq":11,"v":1}`)
	emptyStockIntent := readError(t, conn)
	if emptyStockIntent.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("empty weekly xcore intent error = %+v, want %s", emptyStockIntent.Error, foundation.CodeInvalidPayload)
	}

	premiumPeriod := gameServer.runtime.currentPremiumPeriodKey()
	writeText(t, conn, `{"request_id":"request-weekly-xcore","op":"premium.purchase_weekly_xcore","payload":{"product_id":"weekly_xcore","period_key":"`+premiumPeriod+`"},"client_seq":12,"v":1}`)
	xcoreResponse := readResponse(t, conn)
	if !xcoreResponse.OK {
		t.Fatalf("weekly xcore response = %+v, want success", xcoreResponse)
	}
	var xcorePayload premiumMutationPayload
	if err := json.Unmarshal(xcoreResponse.Payload, &xcorePayload); err != nil {
		t.Fatalf("decode weekly xcore: %v", err)
	}
	if xcorePayload.Wallet.PremiumPaid != starterWalletPremiumPaid-weeklyXCorePremiumPrice || len(xcorePayload.Premium.Purchases) != 1 {
		t.Fatalf("weekly xcore = %+v, want paid premium debit and purchase row", xcorePayload)
	}
	if len(xcorePayload.Premium.Stock) != 1 || xcorePayload.Premium.Stock[0].StockRemaining != weeklyXCoreStockTotal-1 {
		t.Fatalf("weekly xcore stock = %+v, want stock decrement", xcorePayload.Premium.Stock)
	}
	drainEventTypes(t, conn, realtime.EventPremiumStockConsumed, realtime.EventWalletSnapshot)

	writeText(t, conn, `{"request_id":"request-weekly-xcore-again","op":"premium.purchase_weekly_xcore","payload":{"product_id":"weekly_xcore","period_key":"`+premiumPeriod+`"},"client_seq":13,"v":1}`)
	limit := readError(t, conn)
	if limit.Error.Code != foundation.CodeForbidden {
		t.Fatalf("second weekly xcore error = %+v, want %s", limit.Error, foundation.CodeForbidden)
	}

	writeText(t, conn, `{"request_id":"request-admin-economy","op":"admin.economy_dashboard","payload":{},"client_seq":14,"v":1}`)
	admin := readError(t, conn)
	if admin.Error.Code != foundation.CodeForbidden {
		t.Fatalf("non-admin dashboard error = %+v, want %s", admin.Error, foundation.CodeForbidden)
	}

	snapshot := gameServer.runtime.Metrics.Snapshot()
	requireMetricCounter(t, snapshot, observability.MetricMarketVolume, 50, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "raw_ore"},
	})
	requireMetricCounter(t, snapshot, observability.MetricMarketQuantity, 2, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "raw_ore"},
	})
	requireMetricCounter(t, snapshot, observability.MetricMarketSales, 1, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "raw_ore"},
	})
	requireMetricCounter(t, snapshot, observability.MetricAuctionVolume, 300, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
	})
	requireMetricCounter(t, snapshot, observability.MetricAuctionClearingVolume, 650, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "x_core_fragment_bundle"},
	})
	requireMetricCounter(t, snapshot, observability.MetricAuctionClearingQuantity, 2, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "x_core_fragment_bundle"},
	})
	requireMetricCounter(t, snapshot, observability.MetricAuctionClears, 1, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "x_core_fragment_bundle"},
	})
	requireMetricCounter(t, snapshot, observability.MetricWalletDeltaByReason, 50, []observability.Label{
		{Name: "action", Value: economy.LedgerActionIncrease.String()},
		{Name: "currency_type", Value: economy.CurrencyBucketPremiumEarned.String()},
		{Name: "reason", Value: premium.LedgerReasonPremiumEntitlementClaim.String()},
	})
	requireMetricCounter(t, snapshot, observability.MetricWalletDeltaByReason, weeklyXCorePremiumPrice, []observability.Label{
		{Name: "action", Value: economy.LedgerActionDecrease.String()},
		{Name: "currency_type", Value: economy.CurrencyBucketPremiumPaid.String()},
		{Name: "reason", Value: premium.LedgerReasonPremiumWeeklyXCore.String()},
	})
}
func TestPhase07EconomyTrustedPayloadsRejectedBeforeMarketMutation(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	entitlementID := ""
	for _, entitlement := range gameServer.runtime.Premium.Entitlements() {
		if entitlement.PlayerID == resolved.PlayerID {
			entitlementID = entitlement.ID.String()
			break
		}
	}
	if entitlementID == "" {
		t.Fatal("seed premium entitlement missing")
	}

	beforeListing, ok := gameServer.runtime.Market.Listing(seedMarketListingID)
	if !ok {
		t.Fatalf("seed listing %s missing", seedMarketListingID)
	}
	beforeListingCount := len(gameServer.runtime.Market.Listings())
	beforeWalletCredits := gameServer.runtime.Wallet.Balance(resolved.PlayerID, economy.CurrencyBucketCredits)
	beforeWalletLedger := len(gameServer.runtime.Wallet.CurrencyLedgerEntries())
	beforeItemLedger := len(gameServer.runtime.Inventory.ItemLedgerEntries())
	premiumPeriod := gameServer.runtime.currentPremiumPeriodKey()

	tests := []struct {
		name    string
		op      realtime.Operation
		payload string
	}{
		{
			name:    "shop catalog stock/provider spoof",
			op:      realtime.OperationShopCatalog,
			payload: `{"category_id":"weapons","stock_remaining":99,"provider_reference":"client-stock"}`,
		},
		{
			name:    "shop buy product total stock balance spoof",
			op:      realtime.OperationShopBuyProduct,
			payload: `{"product_id":"product_module_laser_alpha_t1","quantity":1,"server_total":1,"stock_remaining":99,"balance":999999}`,
		},
		{
			name: "market buy total fee escrow identity spoof",
			op:   realtime.OperationMarketBuy,
			payload: fmt.Sprintf(
				`{"listing_id":%q,"quantity":1,"total_amount":1,"server_total":1,"fee_amount":0,"server_fee":0,"escrow_location":"market_escrow/spoof","buyer_player_id":"player-buyer-spoof","seller_player_id":"player-seller-spoof"}`,
				seedMarketListingID.String(),
			),
		},
		{
			name:    "market create listing seller escrow spoof",
			op:      realtime.OperationMarketCreateListing,
			payload: `{"item_id":"raw_ore","quantity":1,"unit_price":10,"seller_player_id":"player-seller-spoof","escrow_location":"market_escrow/spoof","source_return_location":"account_inventory/spoof"}`,
		},
		{
			name: "market cancel seller escrow spoof",
			op:   realtime.OperationMarketCancel,
			payload: fmt.Sprintf(
				`{"listing_id":%q,"seller_player_id":"player-seller-spoof","escrow_location":"market_escrow/spoof","source_return_location":"account_inventory/spoof"}`,
				seedMarketListingID.String(),
			),
		},
		{
			name: "auction bid bidder current bid balance spoof",
			op:   realtime.OperationAuctionBid,
			payload: fmt.Sprintf(
				`{"auction_id":%q,"amount":300,"bidder_player_id":"player-bidder-spoof","current_bid":999,"balance":999999}`,
				seedAuctionID.String(),
			),
		},
		{
			name: "auction buy now buyer winner total spoof",
			op:   realtime.OperationAuctionBuyNow,
			payload: fmt.Sprintf(
				`{"auction_id":%q,"buyer_player_id":"player-buyer-spoof","winning_player_id":"player-winner-spoof","server_total":1}`,
				seedAuctionID.String(),
			),
		},
		{
			name: "premium claim provider state balance spoof",
			op:   realtime.OperationPremiumClaim,
			payload: fmt.Sprintf(
				`{"entitlement_id":%q,"provider_reference":"provider-spoof","entitlement_state":"claimed","balance":999999}`,
				entitlementID,
			),
		},
		{
			name: "premium weekly stock provider balance spoof",
			op:   realtime.OperationPremiumWeeklyXCore,
			payload: fmt.Sprintf(
				`{"product_id":"weekly_xcore","period_key":%q,"stock_remaining":99,"provider_reference":"provider-spoof","balance":999999}`,
				premiumPeriod,
			),
		},
	}

	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := fmt.Sprintf(
				`{"request_id":"request-economy-trusted-%d","op":%q,"payload":%s,"client_seq":%d,"v":1}`,
				index,
				tc.op,
				tc.payload,
				index+1,
			)
			writeText(t, conn, request)
			got := readError(t, conn)
			if got.Error.Code != foundation.CodeInvalidPayload {
				t.Fatalf("%s error = %+v, want %s", tc.name, got.Error, foundation.CodeInvalidPayload)
			}
		})
	}

	afterListing, ok := gameServer.runtime.Market.Listing(seedMarketListingID)
	if !ok {
		t.Fatalf("seed listing %s missing after rejected payloads", seedMarketListingID)
	}
	if afterListing.RemainingQuantity != beforeListing.RemainingQuantity || afterListing.Status != beforeListing.Status {
		t.Fatalf("market listing mutated after rejected payloads: before=%+v after=%+v", beforeListing, afterListing)
	}
	if got := len(gameServer.runtime.Market.Listings()); got != beforeListingCount {
		t.Fatalf("market listing count after rejected payloads = %d, want %d", got, beforeListingCount)
	}
	if got := gameServer.runtime.Wallet.Balance(resolved.PlayerID, economy.CurrencyBucketCredits); got != beforeWalletCredits {
		t.Fatalf("wallet credits after rejected payloads = %d, want %d", got, beforeWalletCredits)
	}
	if got := len(gameServer.runtime.Wallet.CurrencyLedgerEntries()); got != beforeWalletLedger {
		t.Fatalf("wallet ledger entries after rejected payloads = %d, want %d", got, beforeWalletLedger)
	}
	if got := len(gameServer.runtime.Inventory.ItemLedgerEntries()); got != beforeItemLedger {
		t.Fatalf("item ledger entries after rejected payloads = %d, want %d", got, beforeItemLedger)
	}
}
func TestMarketCreateListingDuplicateRequestIDReturnsCachedResponse(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-market-create-inventory","op":"inventory.snapshot","payload":{},"client_seq":1,"v":1}`)
	inventoryResponse := readResponse(t, conn)
	if !inventoryResponse.OK {
		t.Fatalf("inventory response = %+v, want success", inventoryResponse)
	}
	var inventoryPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
	}
	if err := json.Unmarshal(inventoryResponse.Payload, &inventoryPayload); err != nil {
		t.Fatalf("decode inventory snapshot: %v", err)
	}
	laserID := requireInventoryInstance(t, inventoryPayload.Inventory, "laser_alpha_t1", economy.LocationKindAccountInventory.String())
	beforeListings := len(gameServer.runtime.Market.Listings())
	beforeLedger := len(gameServer.runtime.Inventory.ItemLedgerEntries())

	request := `{"request_id":"request-market-create-listing-dup","op":"market.create_listing","payload":{"item_id":"laser_alpha_t1","item_instance_id":"` + laserID + `","quantity":1,"unit_price":75},"client_seq":2,"v":1}`
	writeText(t, conn, request)
	firstRaw := readRawText(t, conn)
	first := decodeRawResponse(t, firstRaw)
	if !first.OK {
		t.Fatalf("market create response = %+v, want success", first)
	}
	var firstPayload marketMutationPayload
	if err := json.Unmarshal(first.Payload, &firstPayload); err != nil {
		t.Fatalf("decode market create: %v", err)
	}
	if !firstPayload.Accepted || firstPayload.Listing.ListingID != "listing-request-market-create-listing-dup" {
		t.Fatalf("market create payload = %+v, want accepted listing from request id", firstPayload)
	}
	if got := len(gameServer.runtime.Market.Listings()); got != beforeListings+1 {
		t.Fatalf("listings after create = %d, want %d", got, beforeListings+1)
	}
	if got := len(gameServer.runtime.Inventory.ItemLedgerEntries()); got != beforeLedger+2 {
		t.Fatalf("item ledger entries after create = %d, want %d", got, beforeLedger+2)
	}
	drainEventTypes(t, conn, realtime.EventMarketListingCreated, realtime.EventInventorySnapshot)

	writeText(t, conn, request)
	secondRaw := readRawText(t, conn)
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatalf("duplicate market create response changed:\nfirst=%s\nsecond=%s", firstRaw, secondRaw)
	}
	second := decodeRawResponse(t, secondRaw)
	if !second.OK {
		t.Fatalf("duplicate market create response = %+v, want cached success", second)
	}
	if got := len(gameServer.runtime.Market.Listings()); got != beforeListings+1 {
		t.Fatalf("listings after duplicate = %d, want %d", got, beforeListings+1)
	}
	if got := len(gameServer.runtime.Inventory.ItemLedgerEntries()); got != beforeLedger+2 {
		t.Fatalf("item ledger entries after duplicate = %d, want %d", got, beforeLedger+2)
	}
}
func TestMarketPassiveFanoutNotifiesSellerBuyerAndViewer(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	sellerConn := dialWebSocket(t, httpServer, registerPilotWithIdentity(t, httpServer, "seller@example.com", "Seller-01"))
	defer sellerConn.CloseNow()
	buyerConn := dialWebSocket(t, httpServer, registerPilotWithIdentity(t, httpServer, "buyer@example.com", "Buyer-01"))
	defer buyerConn.CloseNow()
	passiveConn := dialWebSocket(t, httpServer, registerPilotWithIdentity(t, httpServer, "viewer@example.com", "Viewer-01"))
	defer passiveConn.CloseNow()
	readBootstrapEvents(t, sellerConn)
	readBootstrapEvents(t, buyerConn)
	readBootstrapEvents(t, passiveConn)

	writeText(t, sellerConn, `{"request_id":"request-market-fanout-seller-inventory","op":"inventory.snapshot","payload":{},"client_seq":1,"v":1}`)
	inventoryResponse := readResponse(t, sellerConn)
	if !inventoryResponse.OK {
		t.Fatalf("seller inventory response = %+v, want success", inventoryResponse)
	}
	var inventoryPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
	}
	if err := json.Unmarshal(inventoryResponse.Payload, &inventoryPayload); err != nil {
		t.Fatalf("decode seller inventory: %v", err)
	}
	laserID := requireInventoryInstance(t, inventoryPayload.Inventory, "laser_alpha_t1", economy.LocationKindAccountInventory.String())

	createRequest := `{"request_id":"request-market-fanout-create","op":"market.create_listing","payload":{"item_id":"laser_alpha_t1","item_instance_id":"` + laserID + `","quantity":1,"unit_price":90},"client_seq":2,"v":1}`
	writeText(t, sellerConn, createRequest)
	createResponse := readResponse(t, sellerConn)
	if !createResponse.OK {
		t.Fatalf("market create response = %+v, want success", createResponse)
	}
	var createPayload marketMutationPayload
	if err := json.Unmarshal(createResponse.Payload, &createPayload); err != nil {
		t.Fatalf("decode market create: %v", err)
	}
	if createPayload.Listing.ListingID == "" || !createPayload.Listing.OwnedByYou {
		t.Fatalf("create listing payload = %+v, want seller-owned listing", createPayload.Listing)
	}
	sellerCreated := readEvent(t, sellerConn)
	if sellerCreated.Type != realtime.EventMarketListingCreated {
		t.Fatalf("seller create event type = %s, want %s", sellerCreated.Type, realtime.EventMarketListingCreated)
	}
	var sellerCreatedListing marketListingPayload
	if err := json.Unmarshal(sellerCreated.Payload, &sellerCreatedListing); err != nil {
		t.Fatalf("decode seller created event: %v", err)
	}
	if !sellerCreatedListing.OwnedByYou {
		t.Fatalf("seller created listing = %+v, want owned_by_you", sellerCreatedListing)
	}
	sellerInventory := readEvent(t, sellerConn)
	if sellerInventory.Type != realtime.EventInventorySnapshot {
		t.Fatalf("seller create refresh = %s, want %s", sellerInventory.Type, realtime.EventInventorySnapshot)
	}

	buyerCreated := readEvent(t, buyerConn)
	assertPassiveMarketEvent(t, "buyer passive create", buyerCreated, realtime.EventMarketListingCreated)
	passiveCreated := readEvent(t, passiveConn)
	createdListing := assertPassiveMarketEvent(t, "viewer passive create", passiveCreated, realtime.EventMarketListingCreated)
	if createdListing.OwnedByYou || createdListing.ListingID != createPayload.Listing.ListingID {
		t.Fatalf("passive created listing = %+v, want public non-owned listing %q", createdListing, createPayload.Listing.ListingID)
	}

	writeText(t, buyerConn, `{"request_id":"request-market-fanout-buy","op":"market.buy","payload":{"listing_id":"`+createPayload.Listing.ListingID+`","quantity":1},"client_seq":3,"v":1}`)
	buyResponse := readResponse(t, buyerConn)
	if !buyResponse.OK {
		t.Fatalf("market buy response = %+v, want success", buyResponse)
	}

	buyerSale := readEvent(t, buyerConn)
	if buyerSale.Type != realtime.EventMarketSaleCompleted {
		t.Fatalf("buyer sale event type = %s, want %s", buyerSale.Type, realtime.EventMarketSaleCompleted)
	}
	assertNoEconomyLeak(t, "buyer sale event", buyerSale.Payload)
	buyerWallet := readEvent(t, buyerConn)
	if buyerWallet.Type != realtime.EventWalletSnapshot {
		t.Fatalf("buyer wallet event type = %s, want %s", buyerWallet.Type, realtime.EventWalletSnapshot)
	}
	buyerInventory := readEvent(t, buyerConn)
	if buyerInventory.Type != realtime.EventInventorySnapshot {
		t.Fatalf("buyer inventory event type = %s, want %s", buyerInventory.Type, realtime.EventInventorySnapshot)
	}

	sellerSale := readEventOfTypeSkipping(t, sellerConn, realtime.EventMarketSaleCompleted)
	assertNoEconomyLeak(t, "seller passive sale event", sellerSale.Payload)
	var sellerSalePayload struct {
		Listing marketListingPayload `json:"listing"`
	}
	if err := json.Unmarshal(sellerSale.Payload, &sellerSalePayload); err != nil {
		t.Fatalf("decode seller sale event: %v", err)
	}
	if !sellerSalePayload.Listing.OwnedByYou {
		t.Fatalf("seller sale listing = %+v, want owned_by_you", sellerSalePayload.Listing)
	}
	sellerWallet := readEvent(t, sellerConn)
	if sellerWallet.Type != realtime.EventWalletSnapshot {
		t.Fatalf("seller passive wallet event type = %s, want %s", sellerWallet.Type, realtime.EventWalletSnapshot)
	}

	passiveUpdated := readEvent(t, passiveConn)
	updatedListing := assertPassiveMarketEvent(t, "viewer passive buy update", passiveUpdated, realtime.EventMarketListingUpdated)
	if updatedListing.Status != "sold" || updatedListing.RemainingQuantity != 0 {
		t.Fatalf("passive updated listing = %+v, want sold empty listing", updatedListing)
	}
	if passiveUpdated.Sequence <= passiveCreated.Sequence || sellerSale.Sequence <= sellerCreated.Sequence {
		t.Fatalf("event seq did not advance: passive %d->%d seller %d->%d", passiveCreated.Sequence, passiveUpdated.Sequence, sellerCreated.Sequence, sellerSale.Sequence)
	}
}
func TestMarketPassiveFanoutUsesOwnerAwarePrivateAndPublicEvents(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	sellerCookie := registerPilotWithIdentity(t, httpServer, "seller@example.com", "Seller")
	buyerCookie := registerPilotWithIdentity(t, httpServer, "buyer@example.com", "Buyer")
	passiveCookie := registerPilotWithIdentity(t, httpServer, "passive@example.com", "Passive")

	sellerConn := dialWebSocket(t, httpServer, sellerCookie)
	defer sellerConn.CloseNow()
	buyerConn := dialWebSocket(t, httpServer, buyerCookie)
	defer buyerConn.CloseNow()
	passiveConn := dialWebSocket(t, httpServer, passiveCookie)
	defer passiveConn.CloseNow()
	sellerBootstrap := readBootstrapEvents(t, sellerConn)
	buyerBootstrap := readBootstrapEvents(t, buyerConn)
	passiveBootstrap := readBootstrapEvents(t, passiveConn)
	sellerSeq := sellerBootstrap[len(sellerBootstrap)-1].Sequence
	buyerSeq := buyerBootstrap[len(buyerBootstrap)-1].Sequence
	passiveSeq := passiveBootstrap[len(passiveBootstrap)-1].Sequence

	writeText(t, sellerConn, `{"request_id":"request-market-passive-inventory","op":"inventory.snapshot","payload":{},"client_seq":1,"v":1}`)
	inventoryResponse := readResponse(t, sellerConn)
	if !inventoryResponse.OK {
		t.Fatalf("seller inventory response = %+v, want success", inventoryResponse)
	}
	var inventoryPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
	}
	if err := json.Unmarshal(inventoryResponse.Payload, &inventoryPayload); err != nil {
		t.Fatalf("decode seller inventory: %v", err)
	}
	laserID := requireInventoryInstance(t, inventoryPayload.Inventory, "laser_alpha_t1", economy.LocationKindAccountInventory.String())
	shieldID := requireInventoryInstance(t, inventoryPayload.Inventory, "shield_generator_t1", economy.LocationKindAccountInventory.String())

	createRequest := `{"request_id":"request-market-passive-create","op":"market.create_listing","payload":{"item_id":"laser_alpha_t1","item_instance_id":"` + laserID + `","quantity":1,"unit_price":75},"client_seq":2,"v":1}`
	writeText(t, sellerConn, createRequest)
	createResponse := readResponse(t, sellerConn)
	if !createResponse.OK {
		t.Fatalf("market create response = %+v, want success", createResponse)
	}
	var createPayload marketMutationPayload
	if err := json.Unmarshal(createResponse.Payload, &createPayload); err != nil {
		t.Fatalf("decode market create: %v", err)
	}
	listingID := createPayload.Listing.ListingID

	sellerCreated := readEvent(t, sellerConn)
	sellerInventory := readEvent(t, sellerConn)
	if sellerCreated.Type != realtime.EventMarketListingCreated || sellerInventory.Type != realtime.EventInventorySnapshot {
		t.Fatalf("seller create events = %s/%s, want listing created/inventory", sellerCreated.Type, sellerInventory.Type)
	}
	if sellerCreated.Sequence != sellerSeq+1 || sellerInventory.Sequence != sellerSeq+2 {
		t.Fatalf("seller create seq = %d/%d after %d, want contiguous", sellerCreated.Sequence, sellerInventory.Sequence, sellerSeq)
	}
	sellerSeq = sellerInventory.Sequence

	buyerCreated := readEvent(t, buyerConn)
	if buyerCreated.Type != realtime.EventMarketListingCreated || buyerCreated.Sequence != buyerSeq+1 {
		t.Fatalf("buyer passive create event = %+v, want listing created next seq", buyerCreated)
	}
	buyerSeq = buyerCreated.Sequence
	assertPassiveMarketEvent(t, "buyer passive create", buyerCreated, realtime.EventMarketListingCreated)

	passiveCreated := readEvent(t, passiveConn)
	if passiveCreated.Type != realtime.EventMarketListingCreated || passiveCreated.Sequence != passiveSeq+1 {
		t.Fatalf("passive create event = %+v, want listing created next seq", passiveCreated)
	}
	passiveSeq = passiveCreated.Sequence
	passiveCreatedListing := assertPassiveMarketEvent(t, "passive create", passiveCreated, realtime.EventMarketListingCreated)
	if passiveCreatedListing.ListingID != listingID || passiveCreatedListing.OwnedByYou {
		t.Fatalf("passive create listing = %+v, want public non-owned listing %s", passiveCreatedListing, listingID)
	}

	writeText(t, buyerConn, `{"request_id":"request-market-passive-buy","op":"market.buy","payload":{"listing_id":"`+listingID+`","quantity":1},"client_seq":3,"v":1}`)
	buyResponse := readResponse(t, buyerConn)
	if !buyResponse.OK {
		t.Fatalf("market buy response = %+v, want success", buyResponse)
	}
	var buyPayload marketMutationPayload
	if err := json.Unmarshal(buyResponse.Payload, &buyPayload); err != nil {
		t.Fatalf("decode market buy: %v", err)
	}
	if buyPayload.Wallet.Credits != starterWalletCredits-75 || !inventorySnapshotHasInstance(buyPayload.Inventory, "laser_alpha_t1") {
		t.Fatalf("buyer market buy = %+v, want wallet debit and laser inventory", buyPayload)
	}
	buyerSale := readEvent(t, buyerConn)
	buyerWallet := readEvent(t, buyerConn)
	buyerInventory := readEvent(t, buyerConn)
	if buyerSale.Type != realtime.EventMarketSaleCompleted || buyerWallet.Type != realtime.EventWalletSnapshot || buyerInventory.Type != realtime.EventInventorySnapshot {
		t.Fatalf("buyer buy events = %s/%s/%s, want sale/wallet/inventory", buyerSale.Type, buyerWallet.Type, buyerInventory.Type)
	}
	if buyerSale.Sequence != buyerSeq+1 || buyerWallet.Sequence != buyerSeq+2 || buyerInventory.Sequence != buyerSeq+3 {
		t.Fatalf("buyer buy seq = %d/%d/%d after %d, want contiguous", buyerSale.Sequence, buyerWallet.Sequence, buyerInventory.Sequence, buyerSeq)
	}
	buyerSeq = buyerInventory.Sequence

	sellerSale := readEvent(t, sellerConn)
	sellerWallet := readEvent(t, sellerConn)
	if sellerSale.Type != realtime.EventMarketSaleCompleted || sellerWallet.Type != realtime.EventWalletSnapshot {
		t.Fatalf("seller passive sale events = %s/%s, want sale/wallet", sellerSale.Type, sellerWallet.Type)
	}
	if sellerSale.Sequence != sellerSeq+1 || sellerWallet.Sequence != sellerSeq+2 {
		t.Fatalf("seller passive sale seq = %d/%d after %d, want contiguous", sellerSale.Sequence, sellerWallet.Sequence, sellerSeq)
	}
	sellerSeq = sellerWallet.Sequence
	assertNoEconomyLeak(t, "seller sale fanout", sellerSale.Payload)

	passiveUpdated := readEvent(t, passiveConn)
	if passiveUpdated.Type != realtime.EventMarketListingUpdated || passiveUpdated.Sequence != passiveSeq+1 {
		t.Fatalf("passive update event = %+v, want listing updated next seq", passiveUpdated)
	}
	passiveSeq = passiveUpdated.Sequence
	passiveUpdatedListing := assertPassiveMarketEvent(t, "passive update", passiveUpdated, realtime.EventMarketListingUpdated)
	if passiveUpdatedListing.ListingID != listingID || passiveUpdatedListing.Status != market.ListingStatusSold.String() || passiveUpdatedListing.OwnedByYou {
		t.Fatalf("passive update listing = %+v, want public sold listing %s", passiveUpdatedListing, listingID)
	}

	cancelCreateRequest := `{"request_id":"request-market-passive-cancel-create","op":"market.create_listing","payload":{"item_id":"shield_generator_t1","item_instance_id":"` + shieldID + `","quantity":1,"unit_price":90},"client_seq":4,"v":1}`
	writeText(t, sellerConn, cancelCreateRequest)
	cancelCreateResponse := readResponse(t, sellerConn)
	if !cancelCreateResponse.OK {
		t.Fatalf("market cancel fixture create response = %+v, want success", cancelCreateResponse)
	}
	var cancelCreatePayload marketMutationPayload
	if err := json.Unmarshal(cancelCreateResponse.Payload, &cancelCreatePayload); err != nil {
		t.Fatalf("decode cancel fixture create: %v", err)
	}
	cancelListingID := cancelCreatePayload.Listing.ListingID
	drainEventTypes(t, sellerConn, realtime.EventMarketListingCreated, realtime.EventInventorySnapshot)
	sellerSeq += 2
	buyerCancelCreated := readEvent(t, buyerConn)
	passiveCancelCreated := readEvent(t, passiveConn)
	if buyerCancelCreated.Type != realtime.EventMarketListingCreated || buyerCancelCreated.Sequence != buyerSeq+1 {
		t.Fatalf("buyer second passive create = %+v, want listing created next seq", buyerCancelCreated)
	}
	buyerSeq = buyerCancelCreated.Sequence
	if passiveCancelCreated.Type != realtime.EventMarketListingCreated || passiveCancelCreated.Sequence != passiveSeq+1 {
		t.Fatalf("passive second create = %+v, want listing created next seq", passiveCancelCreated)
	}
	passiveSeq = passiveCancelCreated.Sequence
	assertPassiveMarketEvent(t, "passive second create", passiveCancelCreated, realtime.EventMarketListingCreated)

	writeText(t, sellerConn, `{"request_id":"request-market-passive-cancel","op":"market.cancel","payload":{"listing_id":"`+cancelListingID+`"},"client_seq":5,"v":1}`)
	cancelResponse := readResponse(t, sellerConn)
	if !cancelResponse.OK {
		t.Fatalf("market cancel response = %+v, want success", cancelResponse)
	}
	sellerCanceled := readEvent(t, sellerConn)
	sellerCancelInventory := readEvent(t, sellerConn)
	if sellerCanceled.Type != realtime.EventMarketListingCanceled || sellerCancelInventory.Type != realtime.EventInventorySnapshot {
		t.Fatalf("seller cancel events = %s/%s, want listing cancelled/inventory", sellerCanceled.Type, sellerCancelInventory.Type)
	}
	if sellerCanceled.Sequence != sellerSeq+1 || sellerCancelInventory.Sequence != sellerSeq+2 {
		t.Fatalf("seller cancel seq = %d/%d after %d, want contiguous", sellerCanceled.Sequence, sellerCancelInventory.Sequence, sellerSeq)
	}

	buyerCanceled := readEvent(t, buyerConn)
	if buyerCanceled.Type != realtime.EventMarketListingCanceled || buyerCanceled.Sequence != buyerSeq+1 {
		t.Fatalf("buyer passive cancel event = %+v, want listing cancelled next seq", buyerCanceled)
	}
	assertPassiveMarketEvent(t, "buyer passive cancel", buyerCanceled, realtime.EventMarketListingCanceled)

	passiveCanceled := readEvent(t, passiveConn)
	if passiveCanceled.Type != realtime.EventMarketListingCanceled || passiveCanceled.Sequence != passiveSeq+1 {
		t.Fatalf("passive cancel event = %+v, want listing cancelled next seq", passiveCanceled)
	}
	assertPassiveMarketEvent(t, "passive cancel", passiveCanceled, realtime.EventMarketListingCanceled)
}
func TestAuctionBidPassiveFanoutNotifiesBidderPreviousBidderAndViewer(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	previousCookie := registerPilotWithIdentity(t, httpServer, "previous-bidder@example.com", "PrevBidder")
	bidderCookie := registerPilotWithIdentity(t, httpServer, "new-bidder@example.com", "NewBidder")
	viewerCookie := registerPilotWithIdentity(t, httpServer, "auction-viewer@example.com", "AuctionViewer")

	previousConn := dialWebSocket(t, httpServer, previousCookie)
	defer previousConn.CloseNow()
	bidderConn := dialWebSocket(t, httpServer, bidderCookie)
	defer bidderConn.CloseNow()
	viewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer viewerConn.CloseNow()
	previousBootstrap := readBootstrapEvents(t, previousConn)
	bidderBootstrap := readBootstrapEvents(t, bidderConn)
	viewerBootstrap := readBootstrapEvents(t, viewerConn)
	previousSeq := previousBootstrap[len(previousBootstrap)-1].Sequence
	bidderSeq := bidderBootstrap[len(bidderBootstrap)-1].Sequence
	viewerSeq := viewerBootstrap[len(viewerBootstrap)-1].Sequence

	previousBidRequest := `{"request_id":"request-auction-passive-previous-bid","op":"auction.bid","payload":{"auction_id":"` + seedAuctionID.String() + `","amount":300},"client_seq":1,"v":1}`
	writeText(t, previousConn, previousBidRequest)
	previousBidResponse := readResponse(t, previousConn)
	if !previousBidResponse.OK {
		t.Fatalf("previous bid response = %+v, want success", previousBidResponse)
	}
	previousBidPlaced := assertAuctionLotEvent(t, "previous bidder bid placed", readEvent(t, previousConn), realtime.EventAuctionBidPlaced)
	previousLotUpdated := assertAuctionLotEvent(t, "previous bidder lot updated", readEvent(t, previousConn), realtime.EventAuctionLotUpdated)
	previousWallet := assertWalletSnapshotEvent(t, "previous bidder wallet", readEvent(t, previousConn))
	if !previousBidPlaced.Leading || !previousLotUpdated.Leading || previousWallet.Credits != starterWalletCredits-300 {
		t.Fatalf("previous bid events = %+v/%+v wallet=%+v, want leading with debit", previousBidPlaced, previousLotUpdated, previousWallet)
	}
	if previousBidPlaced.CurrentBid != 300 || previousBidPlaced.Status != auction.LotStatusActive.String() {
		t.Fatalf("previous bid lot = %+v, want active 300 bid", previousBidPlaced)
	}
	if previousBidPlaced.Sequence != previousSeq+1 || previousLotUpdated.Sequence != previousSeq+2 {
		t.Fatalf("previous bid seq = %d/%d after %d, want contiguous", previousBidPlaced.Sequence, previousLotUpdated.Sequence, previousSeq)
	}
	previousSeq += 3

	bidderPassive := assertAuctionLotEvent(t, "new bidder passive first bid", readEvent(t, bidderConn), realtime.EventAuctionLotUpdated)
	if bidderPassive.Leading || bidderPassive.CurrentBid != 300 {
		t.Fatalf("new bidder passive lot = %+v, want public non-leading 300 bid", bidderPassive)
	}
	if bidderPassive.Sequence != bidderSeq+1 {
		t.Fatalf("new bidder passive seq = %d after %d, want contiguous", bidderPassive.Sequence, bidderSeq)
	}
	bidderSeq = bidderPassive.Sequence

	viewerPassive := assertAuctionLotEvent(t, "viewer passive first bid", readEvent(t, viewerConn), realtime.EventAuctionLotUpdated)
	if viewerPassive.Leading || viewerPassive.CurrentBid != 300 {
		t.Fatalf("viewer passive lot = %+v, want public non-leading 300 bid", viewerPassive)
	}
	if viewerPassive.Sequence != viewerSeq+1 {
		t.Fatalf("viewer passive seq = %d after %d, want contiguous", viewerPassive.Sequence, viewerSeq)
	}
	viewerSeq = viewerPassive.Sequence

	newBidRequest := `{"request_id":"request-auction-passive-new-bid","op":"auction.bid","payload":{"auction_id":"` + seedAuctionID.String() + `","amount":450},"client_seq":2,"v":1}`
	writeText(t, bidderConn, newBidRequest)
	newBidResponse := readResponse(t, bidderConn)
	if !newBidResponse.OK {
		t.Fatalf("new bid response = %+v, want success", newBidResponse)
	}
	var newBidPayload auctionMutationPayload
	if err := json.Unmarshal(newBidResponse.Payload, &newBidPayload); err != nil {
		t.Fatalf("decode new bid response: %v", err)
	}
	if newBidPayload.Wallet.Credits != starterWalletCredits-450 || !newBidPayload.Lot.Leading {
		t.Fatalf("new bid response payload = %+v, want bidder leading with wallet debit", newBidPayload)
	}

	newBidPlaced := assertAuctionLotEvent(t, "new bidder bid placed", readEvent(t, bidderConn), realtime.EventAuctionBidPlaced)
	newBidUpdated := assertAuctionLotEvent(t, "new bidder lot updated", readEvent(t, bidderConn), realtime.EventAuctionLotUpdated)
	newBidWallet := assertWalletSnapshotEvent(t, "new bidder wallet", readEvent(t, bidderConn))
	if !newBidPlaced.Leading || !newBidUpdated.Leading || newBidWallet.Credits != starterWalletCredits-450 {
		t.Fatalf("new bidder events = %+v/%+v wallet=%+v, want leading with wallet debit", newBidPlaced, newBidUpdated, newBidWallet)
	}
	if newBidPlaced.CurrentBid != 450 || newBidUpdated.CurrentBid != 450 {
		t.Fatalf("new bidder lot events = %+v/%+v, want current bid 450", newBidPlaced, newBidUpdated)
	}
	if newBidPlaced.Sequence != bidderSeq+1 || newBidUpdated.Sequence != bidderSeq+2 || newBidWallet.Sequence != bidderSeq+3 {
		t.Fatalf("new bidder seq = %d/%d/%d after %d, want contiguous", newBidPlaced.Sequence, newBidUpdated.Sequence, newBidWallet.Sequence, bidderSeq)
	}
	bidderSeq = newBidWallet.Sequence

	previousOutbid := assertAuctionLotEvent(t, "previous bidder outbid", readEvent(t, previousConn), realtime.EventAuctionLotUpdated)
	previousRefundWallet := assertWalletSnapshotEvent(t, "previous bidder refund wallet", readEvent(t, previousConn))
	if previousOutbid.Leading || previousOutbid.CurrentBid != 450 || previousRefundWallet.Credits != starterWalletCredits {
		t.Fatalf("previous outbid = %+v wallet=%+v, want non-leading refund", previousOutbid, previousRefundWallet)
	}
	if previousOutbid.Sequence != previousSeq+1 || previousRefundWallet.Sequence != previousSeq+2 {
		t.Fatalf("previous refund seq = %d/%d after %d, want contiguous", previousOutbid.Sequence, previousRefundWallet.Sequence, previousSeq)
	}

	viewerOutbid := assertAuctionLotEvent(t, "viewer passive outbid", readEvent(t, viewerConn), realtime.EventAuctionLotUpdated)
	if viewerOutbid.Leading || viewerOutbid.CurrentBid != 450 {
		t.Fatalf("viewer outbid lot = %+v, want public non-leading 450 bid", viewerOutbid)
	}
	if viewerOutbid.Sequence != viewerSeq+1 {
		t.Fatalf("viewer outbid seq = %d after %d, want contiguous", viewerOutbid.Sequence, viewerSeq)
	}

	writeText(t, bidderConn, newBidRequest)
	duplicateBidResponse := readResponse(t, bidderConn)
	if !duplicateBidResponse.OK {
		t.Fatalf("duplicate bid response = %+v, want cached success", duplicateBidResponse)
	}
	assertNoRealtimeMessageWithin(t, "duplicate bid bidder fanout", bidderConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate bid previous fanout", previousConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate bid viewer fanout", viewerConn, 100*time.Millisecond)
}
func TestAuctionBuyNowPassiveFanoutKeepsGrantPrivate(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	bidderCookie := registerPilotWithIdentity(t, httpServer, "current-bidder@example.com", "CurrentBidder")
	buyerCookie := registerPilotWithIdentity(t, httpServer, "buy-now-buyer@example.com", "BuyNowBuyer")
	viewerCookie := registerPilotWithIdentity(t, httpServer, "buy-now-viewer@example.com", "BuyNowViewer")

	bidderConn := dialWebSocket(t, httpServer, bidderCookie)
	defer bidderConn.CloseNow()
	buyerConn := dialWebSocket(t, httpServer, buyerCookie)
	defer buyerConn.CloseNow()
	viewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer viewerConn.CloseNow()
	bidderBootstrap := readBootstrapEvents(t, bidderConn)
	buyerBootstrap := readBootstrapEvents(t, buyerConn)
	viewerBootstrap := readBootstrapEvents(t, viewerConn)
	bidderSeq := bidderBootstrap[len(bidderBootstrap)-1].Sequence
	buyerSeq := buyerBootstrap[len(buyerBootstrap)-1].Sequence
	viewerSeq := viewerBootstrap[len(viewerBootstrap)-1].Sequence

	writeText(t, bidderConn, `{"request_id":"request-auction-buy-now-current-bid","op":"auction.bid","payload":{"auction_id":"`+seedAuctionID.String()+`","amount":300},"client_seq":1,"v":1}`)
	currentBidResponse := readResponse(t, bidderConn)
	if !currentBidResponse.OK {
		t.Fatalf("current bid response = %+v, want success", currentBidResponse)
	}
	currentBidPlaced := assertAuctionLotEvent(t, "current bidder bid placed", readEvent(t, bidderConn), realtime.EventAuctionBidPlaced)
	currentBidUpdated := assertAuctionLotEvent(t, "current bidder lot updated", readEvent(t, bidderConn), realtime.EventAuctionLotUpdated)
	currentBidWallet := assertWalletSnapshotEvent(t, "current bidder bid wallet", readEvent(t, bidderConn))
	if !currentBidPlaced.Leading || !currentBidUpdated.Leading || currentBidWallet.Credits != starterWalletCredits-300 {
		t.Fatalf("current bid events = %+v/%+v wallet=%+v, want leading with debit", currentBidPlaced, currentBidUpdated, currentBidWallet)
	}
	bidderSeq += 3
	buyerBidView := assertAuctionLotEvent(t, "buyer passive bid", readEvent(t, buyerConn), realtime.EventAuctionLotUpdated)
	if buyerBidView.Leading || buyerBidView.CurrentBid != 300 {
		t.Fatalf("buyer passive bid lot = %+v, want public non-leading 300 bid", buyerBidView)
	}
	buyerSeq = buyerBidView.Sequence
	viewerBidView := assertAuctionLotEvent(t, "viewer passive bid", readEvent(t, viewerConn), realtime.EventAuctionLotUpdated)
	if viewerBidView.Leading || viewerBidView.CurrentBid != 300 {
		t.Fatalf("viewer passive bid lot = %+v, want public non-leading 300 bid", viewerBidView)
	}
	viewerSeq = viewerBidView.Sequence

	buyNowRequest := `{"request_id":"request-auction-passive-buy-now","op":"auction.buy_now","payload":{"auction_id":"` + seedAuctionID.String() + `"},"client_seq":2,"v":1}`
	writeText(t, buyerConn, buyNowRequest)
	buyNowResponse := readResponse(t, buyerConn)
	if !buyNowResponse.OK {
		t.Fatalf("buy now response = %+v, want success", buyNowResponse)
	}
	var buyNowPayload auctionMutationPayload
	if err := json.Unmarshal(buyNowResponse.Payload, &buyNowPayload); err != nil {
		t.Fatalf("decode buy now response: %v", err)
	}
	if buyNowPayload.Price != 650 || buyNowPayload.Grant == nil || buyNowPayload.Wallet.Credits != starterWalletCredits-650 {
		t.Fatalf("buy now payload = %+v, want private grant and buyer debit", buyNowPayload)
	}

	buyerClosed := assertAuctionClosedEvent(t, "buyer closed", readEvent(t, buyerConn))
	buyerLotUpdated := assertAuctionLotEvent(t, "buyer lot updated", readEvent(t, buyerConn), realtime.EventAuctionLotUpdated)
	buyerWallet := assertWalletSnapshotEvent(t, "buyer wallet", readEvent(t, buyerConn))
	if buyerClosed.Grant == nil || buyerClosed.Lot.Status != auction.LotStatusClosed.String() || buyerClosed.Lot.Leading {
		t.Fatalf("buyer closed event = %+v, want closed private grant without leading", buyerClosed)
	}
	if buyerLotUpdated.Status != auction.LotStatusClosed.String() || buyerLotUpdated.Leading || buyerWallet.Credits != starterWalletCredits-650 {
		t.Fatalf("buyer lot/wallet = %+v/%+v, want closed non-leading with debit", buyerLotUpdated, buyerWallet)
	}
	if buyerClosed.Sequence != buyerSeq+1 || buyerLotUpdated.Sequence != buyerSeq+2 || buyerWallet.Sequence != buyerSeq+3 {
		t.Fatalf("buyer buy-now seq = %d/%d/%d after %d, want contiguous", buyerClosed.Sequence, buyerLotUpdated.Sequence, buyerWallet.Sequence, buyerSeq)
	}

	refundedLot := assertAuctionLotEvent(t, "refunded bidder lot", readEvent(t, bidderConn), realtime.EventAuctionLotUpdated)
	refundedWallet := assertWalletSnapshotEvent(t, "refunded bidder wallet", readEvent(t, bidderConn))
	if refundedLot.Status != auction.LotStatusClosed.String() || refundedLot.Leading || refundedWallet.Credits != starterWalletCredits {
		t.Fatalf("refunded bidder events = %+v wallet=%+v, want public closed lot and refund", refundedLot, refundedWallet)
	}
	if refundedLot.Sequence != bidderSeq+1 || refundedWallet.Sequence != bidderSeq+2 {
		t.Fatalf("refunded bidder seq = %d/%d after %d, want contiguous", refundedLot.Sequence, refundedWallet.Sequence, bidderSeq)
	}

	viewerClosedLot := assertAuctionLotEvent(t, "passive viewer closed lot", readEvent(t, viewerConn), realtime.EventAuctionLotUpdated)
	if viewerClosedLot.Status != auction.LotStatusClosed.String() || viewerClosedLot.Leading {
		t.Fatalf("viewer closed lot = %+v, want public closed non-leading lot", viewerClosedLot)
	}
	if viewerClosedLot.Sequence != viewerSeq+1 {
		t.Fatalf("viewer closed seq = %d after %d, want contiguous", viewerClosedLot.Sequence, viewerSeq)
	}

	writeText(t, buyerConn, buyNowRequest)
	duplicateBuyNowResponse := readResponse(t, buyerConn)
	if !duplicateBuyNowResponse.OK {
		t.Fatalf("duplicate buy-now response = %+v, want cached success", duplicateBuyNowResponse)
	}
	assertNoRealtimeMessageWithin(t, "duplicate buy-now buyer fanout", buyerConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate buy-now bidder fanout", bidderConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate buy-now viewer fanout", viewerConn, 100*time.Millisecond)
}
func TestPremiumClaimPassiveFanoutNotifiesOwnerSessionsOnly(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	ownerEmail := "premium-owner@example.com"
	ownerCookie := registerPilotWithIdentity(t, httpServer, ownerEmail, "PremiumOwner")
	ownerSecondCookie := loginPilot(t, httpServer, ownerEmail, "correct-password")
	viewerCookie := registerPilotWithIdentity(t, httpServer, "premium-viewer@example.com", "PremiumViewer")

	ownerConn := dialWebSocket(t, httpServer, ownerCookie)
	defer ownerConn.CloseNow()
	ownerSecondConn := dialWebSocket(t, httpServer, ownerSecondCookie)
	defer ownerSecondConn.CloseNow()
	viewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer viewerConn.CloseNow()
	readBootstrapEvents(t, ownerConn)
	readBootstrapEvents(t, ownerSecondConn)
	readBootstrapEvents(t, viewerConn)

	writeText(t, ownerConn, `{"request_id":"request-premium-owner-entitlements","op":"premium.entitlements","payload":{},"client_seq":1,"v":1}`)
	entitlementsResponse := readResponse(t, ownerConn)
	if !entitlementsResponse.OK {
		t.Fatalf("premium entitlements response = %+v, want success", entitlementsResponse)
	}
	var entitlementsPayload struct {
		Premium premiumSummaryPayload `json:"premium"`
	}
	if err := json.Unmarshal(entitlementsResponse.Payload, &entitlementsPayload); err != nil {
		t.Fatalf("decode premium entitlements: %v", err)
	}
	if len(entitlementsPayload.Premium.Entitlements) != 1 {
		t.Fatalf("premium entitlements = %+v, want one owner entitlement", entitlementsPayload.Premium.Entitlements)
	}
	entitlementID := entitlementsPayload.Premium.Entitlements[0].EntitlementID

	claimRequest := `{"request_id":"request-premium-passive-claim","op":"premium.claim","payload":{"entitlement_id":"` + entitlementID + `"},"client_seq":2,"v":1}`
	writeText(t, ownerConn, claimRequest)
	claimResponse := readResponse(t, ownerConn)
	if !claimResponse.OK {
		t.Fatalf("premium claim response = %+v, want success", claimResponse)
	}
	var claimPayload premiumMutationPayload
	if err := json.Unmarshal(claimResponse.Payload, &claimPayload); err != nil {
		t.Fatalf("decode premium claim: %v", err)
	}
	if claimPayload.Wallet.PremiumEarned != 50 || claimPayload.Premium.Entitlements[0].State != premium.EntitlementStateClaimed.String() {
		t.Fatalf("premium claim payload = %+v, want claimed entitlement and earned premium wallet", claimPayload)
	}

	ownerClaim := assertPremiumClaimedEvent(t, "owner claim", readEvent(t, ownerConn), entitlementID)
	ownerWallet := assertWalletSnapshotEvent(t, "owner claim wallet", readEvent(t, ownerConn))
	if ownerWallet.PremiumEarned != 50 {
		t.Fatalf("owner claim wallet = %+v, want earned premium 50", ownerWallet)
	}
	ownerSecondClaim := assertPremiumClaimedEvent(t, "second owner claim", readEvent(t, ownerSecondConn), entitlementID)
	ownerSecondWallet := assertWalletSnapshotEvent(t, "second owner claim wallet", readEvent(t, ownerSecondConn))
	if ownerSecondWallet.PremiumEarned != 50 {
		t.Fatalf("second owner claim wallet = %+v, want earned premium 50", ownerSecondWallet)
	}
	if ownerClaim.State != premium.EntitlementStateClaimed.String() || ownerSecondClaim.State != premium.EntitlementStateClaimed.String() {
		t.Fatalf("claim fanout states = %s/%s, want claimed", ownerClaim.State, ownerSecondClaim.State)
	}
	assertNoRealtimeMessageWithin(t, "unrelated premium claim fanout", viewerConn, 100*time.Millisecond)

	duplicateViewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer duplicateViewerConn.CloseNow()
	readBootstrapEvents(t, duplicateViewerConn)

	writeText(t, ownerConn, claimRequest)
	duplicateClaimResponse := readResponse(t, ownerConn)
	if !duplicateClaimResponse.OK {
		t.Fatalf("duplicate premium claim response = %+v, want cached success", duplicateClaimResponse)
	}
	assertNoRealtimeMessageWithin(t, "duplicate claim owner fanout", ownerConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate claim second owner fanout", ownerSecondConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate claim unrelated fanout", duplicateViewerConn, 100*time.Millisecond)
}
func TestPremiumWeeklyXCorePassiveFanoutKeepsViewerPayloadPublic(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	purchaserEmail := "premium-purchaser@example.com"
	purchaserCookie := registerPilotWithIdentity(t, httpServer, purchaserEmail, "PremiumBuyer")
	purchaserSecondCookie := loginPilot(t, httpServer, purchaserEmail, "correct-password")
	viewerCookie := registerPilotWithIdentity(t, httpServer, "premium-stock-viewer@example.com", "StockViewer")

	purchaserConn := dialWebSocket(t, httpServer, purchaserCookie)
	defer purchaserConn.CloseNow()
	purchaserSecondConn := dialWebSocket(t, httpServer, purchaserSecondCookie)
	defer purchaserSecondConn.CloseNow()
	viewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer viewerConn.CloseNow()
	readBootstrapEvents(t, purchaserConn)
	readBootstrapEvents(t, purchaserSecondConn)
	readBootstrapEvents(t, viewerConn)

	premiumPeriod := gameServer.runtime.currentPremiumPeriodKey()
	purchaseRequest := `{"request_id":"request-premium-passive-weekly-xcore","op":"premium.purchase_weekly_xcore","payload":{"product_id":"weekly_xcore","period_key":"` + premiumPeriod + `"},"client_seq":1,"v":1}`
	writeText(t, purchaserConn, purchaseRequest)
	purchaseResponse := readResponse(t, purchaserConn)
	if !purchaseResponse.OK {
		t.Fatalf("premium weekly xcore response = %+v, want success", purchaseResponse)
	}
	var purchasePayload premiumMutationPayload
	if err := json.Unmarshal(purchaseResponse.Payload, &purchasePayload); err != nil {
		t.Fatalf("decode weekly xcore: %v", err)
	}
	if purchasePayload.Wallet.PremiumPaid != starterWalletPremiumPaid-weeklyXCorePremiumPrice || len(purchasePayload.Premium.Purchases) != 1 {
		t.Fatalf("weekly xcore payload = %+v, want purchaser debit and purchase row", purchasePayload)
	}

	purchaserStock := assertPremiumStockConsumedEvent(t, "purchaser stock", readEvent(t, purchaserConn))
	purchaserWallet := assertWalletSnapshotEvent(t, "purchaser weekly xcore wallet", readEvent(t, purchaserConn))
	if purchaserStock.StockRemaining != weeklyXCoreStockTotal-1 || purchaserWallet.PremiumPaid != starterWalletPremiumPaid-weeklyXCorePremiumPrice {
		t.Fatalf("purchaser fanout stock/wallet = %+v/%+v, want stock decrement and wallet debit", purchaserStock, purchaserWallet)
	}
	purchaserSecondStock := assertPremiumStockConsumedEvent(t, "second purchaser stock", readEvent(t, purchaserSecondConn))
	purchaserSecondWallet := assertWalletSnapshotEvent(t, "second purchaser weekly xcore wallet", readEvent(t, purchaserSecondConn))
	if purchaserSecondStock.StockRemaining != weeklyXCoreStockTotal-1 || purchaserSecondWallet.PremiumPaid != starterWalletPremiumPaid-weeklyXCorePremiumPrice {
		t.Fatalf("second purchaser fanout stock/wallet = %+v/%+v, want stock decrement and wallet debit", purchaserSecondStock, purchaserSecondWallet)
	}
	viewerStock := assertPremiumStockConsumedEvent(t, "passive stock viewer", readEvent(t, viewerConn))
	if viewerStock.StockRemaining != weeklyXCoreStockTotal-1 || viewerStock.PeriodKey != premiumPeriod {
		t.Fatalf("passive stock payload = %+v, want public decremented stock for %s", viewerStock, premiumPeriod)
	}
	assertNoRealtimeMessageWithin(t, "passive stock viewer wallet fanout", viewerConn, 100*time.Millisecond)

	duplicateViewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer duplicateViewerConn.CloseNow()
	readBootstrapEvents(t, duplicateViewerConn)

	writeText(t, purchaserConn, purchaseRequest)
	duplicatePurchaseResponse := readResponse(t, purchaserConn)
	if !duplicatePurchaseResponse.OK {
		t.Fatalf("duplicate weekly xcore response = %+v, want cached success", duplicatePurchaseResponse)
	}
	assertNoRealtimeMessageWithin(t, "duplicate weekly xcore purchaser fanout", purchaserConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate weekly xcore second purchaser fanout", purchaserSecondConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate weekly xcore viewer fanout", duplicateViewerConn, 100*time.Millisecond)
}
