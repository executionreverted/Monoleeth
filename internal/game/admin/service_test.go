package admin_test

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/admin"
	"gameproject/internal/game/auction"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/market"
	"gameproject/internal/game/production"
	"gameproject/internal/game/testutil"
)

func TestAdminInspectsPlayerInventoryAndLedgers(t *testing.T) {
	clock := testutil.NewFakeClock(testAdminNow)
	inventory := economy.NewInventoryService(clock)
	wallet := economy.NewWalletService(clock)
	service := admin.NewService(admin.ServiceConfig{Inventory: inventory, Wallet: wallet, Clock: clock})
	definition := testItemDefinition(t, "iron_ore", economy.ItemTypeStackable)
	playerLocation := testAccountLocation(t, "player-1")
	otherLocation := testAccountLocation(t, "player-2")

	addItem(t, inventory, "player-1", definition, 3, playerLocation, "player-1-item")
	addItem(t, inventory, "player-2", definition, 5, otherLocation, "player-2-item")
	creditWallet(t, wallet, "player-1", economy.CurrencyBucketCredits, 100, "player-1-wallet")
	creditWallet(t, wallet, "player-2", economy.CurrencyBucketCredits, 50, "player-2-wallet")

	inventoryReport, err := service.InspectPlayerInventory("player-1")
	if err != nil {
		t.Fatalf("InspectPlayerInventory() error = %v", err)
	}
	if len(inventoryReport.StackableItems) != 1 || inventoryReport.StackableItems[0].Quantity.Int64() != 3 {
		t.Fatalf("stackable items = %+v, want one quantity 3", inventoryReport.StackableItems)
	}
	if len(inventoryReport.InstanceItems) != 0 {
		t.Fatalf("instance items = %+v, want none", inventoryReport.InstanceItems)
	}

	walletReport, err := service.InspectPlayerWalletLedger("player-1")
	if err != nil {
		t.Fatalf("InspectPlayerWalletLedger() error = %v", err)
	}
	if len(walletReport.Balances) != 1 || walletReport.Balances[0].Balance != 100 {
		t.Fatalf("balances = %+v, want one 100-credit balance", walletReport.Balances)
	}
	if len(walletReport.LedgerEntries) != 1 || walletReport.LedgerEntries[0].PlayerID != "player-1" {
		t.Fatalf("currency ledger = %+v, want one player-1 row", walletReport.LedgerEntries)
	}

	itemLedgerReport, err := service.InspectPlayerItemLedger("player-1")
	if err != nil {
		t.Fatalf("InspectPlayerItemLedger() error = %v", err)
	}
	if len(itemLedgerReport.LedgerEntries) != 1 || itemLedgerReport.LedgerEntries[0].Quantity.Int64() != 3 {
		t.Fatalf("item ledger = %+v, want one quantity 3 row", itemLedgerReport.LedgerEntries)
	}
}

func TestAdminWritesCompensatingCurrencyAndItemEntries(t *testing.T) {
	clock := testutil.NewFakeClock(testAdminNow)
	inventory := economy.NewInventoryService(clock)
	wallet := economy.NewWalletService(clock)
	service := admin.NewService(admin.ServiceConfig{Inventory: inventory, Wallet: wallet, Clock: clock})
	definition := testItemDefinition(t, "iron_ore", economy.ItemTypeStackable)
	location := testAccountLocation(t, "player-1")

	creditWallet(t, wallet, "player-1", economy.CurrencyBucketCredits, 100, "wallet-seed")
	debit, err := wallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     "player-1",
		Currency:     economy.CurrencyBucketCredits,
		Amount:       40,
		Reason:       "test_bad_debit",
		ReferenceKey: mustAdminKey(t, "wallet-debit", "seed"),
	})
	if err != nil {
		t.Fatalf("DebitWallet() error = %v", err)
	}

	currencyRepair, err := service.CompensateCurrencyLedgerEntry(admin.CompensateCurrencyInput{
		LedgerEntry:     debit.LedgerEntry,
		RepairReference: "ticket-1",
	})
	if err != nil {
		t.Fatalf("CompensateCurrencyLedgerEntry() error = %v", err)
	}
	if currencyRepair.Credit == nil || currencyRepair.Credit.LedgerEntry.Reason != admin.ReasonAdminCompensation {
		t.Fatalf("currency repair = %+v, want admin compensation credit", currencyRepair)
	}
	if got := wallet.Balance("player-1", economy.CurrencyBucketCredits); got != 100 {
		t.Fatalf("wallet balance = %d, want 100", got)
	}

	addItem(t, inventory, "player-1", definition, 5, location, "item-seed")
	remove, err := inventory.RemoveItem(economy.RemoveItemInput{
		PlayerID:       "player-1",
		ItemRef:        economy.RemoveItemRef{Definition: definition},
		SourceLocation: location,
		Quantity:       2,
		Reason:         "test_bad_remove",
		ReferenceKey:   mustAdminKey(t, "item-remove", "seed"),
	})
	if err != nil {
		t.Fatalf("RemoveItem() error = %v", err)
	}

	itemRepair, err := service.CompensateItemLedgerEntry(admin.CompensateItemInput{
		LedgerEntry:     remove.LedgerEntries[0],
		ItemDefinition:  definition,
		RepairReference: "ticket-2",
	})
	if err != nil {
		t.Fatalf("CompensateItemLedgerEntry() error = %v", err)
	}
	if itemRepair.Add == nil || itemRepair.Add.LedgerEntry.Reason != admin.ReasonAdminCompensation {
		t.Fatalf("item repair = %+v, want admin compensation add", itemRepair)
	}
	if got := inventory.TotalItemQuantity("player-1", "iron_ore", location); got != 5 {
		t.Fatalf("item quantity = %d, want 5", got)
	}

	escrowLocation, err := economy.NewItemLocation(economy.LocationKindMarketEscrow, "listing-admin-1")
	if err != nil {
		t.Fatalf("NewItemLocation() error = %v", err)
	}
	badEscrowGrant := addItem(t, inventory, "player-1", definition, 4, escrowLocation, "item-bad-grant")
	grantRepair, err := service.CompensateItemLedgerEntry(admin.CompensateItemInput{
		LedgerEntry:     badEscrowGrant.LedgerEntry,
		ItemDefinition:  definition,
		RepairReference: "ticket-3",
	})
	if err != nil {
		t.Fatalf("CompensateItemLedgerEntry(bad grant) error = %v", err)
	}
	if grantRepair.Remove == nil || grantRepair.Remove.LedgerEntries[0].Reason != admin.ReasonAdminCompensation {
		t.Fatalf("grant repair = %+v, want admin compensation remove", grantRepair)
	}
	if got := inventory.TotalItemQuantity("player-1", "iron_ore", escrowLocation); got != 0 {
		t.Fatalf("escrow quantity = %d, want 0", got)
	}
}

func TestAdminDisablesSuspiciousListingAndMarksIntelStale(t *testing.T) {
	clock := testutil.NewFakeClock(testAdminNow)
	inventory := economy.NewInventoryService(clock)
	wallet := economy.NewWalletService(clock)
	marketService := newTestMarketService(t, clock, inventory, wallet)
	service := admin.NewService(admin.ServiceConfig{Inventory: inventory, Wallet: wallet, Market: marketService, Clock: clock})
	definition := testItemDefinition(t, "intel_cache", economy.ItemTypeStackable)
	sourceLocation := testAccountLocation(t, "seller-1")
	addItem(t, inventory, "seller-1", definition, 2, sourceLocation, "listing-seed")

	created, err := marketService.CreateListing(market.CreateListingInput{
		ListingID:      "listing-1",
		SellerPlayerID: "seller-1",
		ItemRef:        economy.MoveItemRef{Definition: definition},
		SourceLocation: sourceLocation,
		Quantity:       2,
		UnitPrice:      100,
		Currency:       economy.CurrencyBucketCredits,
	})
	if err != nil {
		t.Fatalf("CreateListing() error = %v", err)
	}

	stale, err := service.MarkIntelListingStale(admin.MarkIntelListingStaleInput{
		ListingID: "listing-1",
		Reason:    "coordinate revoked",
	})
	if err != nil {
		t.Fatalf("MarkIntelListingStale() error = %v", err)
	}
	if stale.Listing.Status != market.ListingStatusStale {
		t.Fatalf("stale status = %q, want stale", stale.Listing.Status)
	}

	disabled, err := service.DisableSuspiciousMarketListing(admin.DisableMarketListingInput{
		ListingID: "listing-1",
		Reason:    "fraud review",
	})
	if err != nil {
		t.Fatalf("DisableSuspiciousMarketListing() error = %v", err)
	}
	if disabled.Cancel.Listing.Status != market.ListingStatusCancelled || disabled.Cancel.ReturnedQuantity != 2 {
		t.Fatalf("cancel result = %+v, want cancelled return of 2", disabled.Cancel)
	}
	if got := inventory.TotalItemQuantity("seller-1", "intel_cache", sourceLocation); got != 2 {
		t.Fatalf("seller returned quantity = %d, want 2", got)
	}
	if got := inventory.TotalItemQuantity("seller-1", "intel_cache", created.Listing.EscrowLocation); got != 0 {
		t.Fatalf("escrow quantity = %d, want 0", got)
	}
}

func TestAdminRefundsAuctionBidThroughLedgerAndBlocksActiveCurrentBid(t *testing.T) {
	clock := testutil.NewFakeClock(testAdminNow)
	wallet := economy.NewWalletService(clock)
	auctionService := newTestAuctionService(t, clock, wallet)
	service := admin.NewService(admin.ServiceConfig{Wallet: wallet, Auction: auctionService, Clock: clock})
	creditWallet(t, wallet, "bidder-1", economy.CurrencyBucketCredits, 500, "bidder-seed")
	lot := createAuctionLot(t, auctionService, "auction-1", 100, nil)

	bid, err := auctionService.PlaceBid(auction.PlaceBidInput{
		AuctionID:      lot.AuctionID,
		BidderPlayerID: "bidder-1",
		Amount:         120,
		RequestID:      "request-1",
	})
	if err != nil {
		t.Fatalf("PlaceBid() error = %v", err)
	}

	_, err = service.RefundAuctionBid(admin.RefundAuctionBidInput{
		AuctionID:       lot.AuctionID,
		BidLedgerEntry:  bid.BidderDebit.LedgerEntry,
		RepairReference: "ticket-active",
	})
	if !errors.Is(err, admin.ErrUnsafeActiveAuctionBidRefund) {
		t.Fatalf("RefundAuctionBid(active) error = %v, want ErrUnsafeActiveAuctionBidRefund", err)
	}

	missingAuctionAdmin := admin.NewService(admin.ServiceConfig{Wallet: wallet, Clock: clock})
	_, err = missingAuctionAdmin.RefundAuctionBid(admin.RefundAuctionBidInput{
		AuctionID:       lot.AuctionID,
		BidLedgerEntry:  bid.BidderDebit.LedgerEntry,
		RepairReference: "ticket-missing-auction",
	})
	if !errors.Is(err, admin.ErrMissingAuctionService) {
		t.Fatalf("RefundAuctionBid(missing auction) error = %v, want ErrMissingAuctionService", err)
	}

	creditWallet(t, wallet, "bidder-2", economy.CurrencyBucketCredits, 500, "outbidder-seed")
	_, err = auctionService.PlaceBid(auction.PlaceBidInput{
		AuctionID:      lot.AuctionID,
		BidderPlayerID: "bidder-2",
		Amount:         150,
		RequestID:      "request-2",
	})
	if err != nil {
		t.Fatalf("PlaceBid(outbidder) error = %v", err)
	}
	if got := wallet.Balance("bidder-1", economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("outbid bidder balance = %d, want normal refund to restore 500", got)
	}
	_, err = service.RefundAuctionBid(admin.RefundAuctionBidInput{
		AuctionID:       lot.AuctionID,
		BidLedgerEntry:  bid.BidderDebit.LedgerEntry,
		RepairReference: "ticket-already-refunded",
	})
	if !errors.Is(err, admin.ErrAuctionBidAlreadyRefunded) {
		t.Fatalf("RefundAuctionBid(already refunded) error = %v, want ErrAuctionBidAlreadyRefunded", err)
	}
	if got := wallet.Balance("bidder-1", economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("already-refunded bidder balance = %d, want 500", got)
	}

	creditWallet(t, wallet, "bidder-3", economy.CurrencyBucketCredits, 500, "stale-bidder-seed")
	staleBidKey, err := foundation.AuctionBidIdempotencyKey(lot.AuctionID, "bidder-3", "request-stale")
	if err != nil {
		t.Fatalf("AuctionBidIdempotencyKey() error = %v", err)
	}
	staleBid, err := wallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     "bidder-3",
		Currency:     economy.CurrencyBucketCredits,
		Amount:       80,
		Reason:       "auction_bid",
		ReferenceKey: staleBidKey,
	})
	if err != nil {
		t.Fatalf("DebitWallet(stale bid) error = %v", err)
	}

	refund, err := service.RefundAuctionBid(admin.RefundAuctionBidInput{
		AuctionID:       lot.AuctionID,
		BidLedgerEntry:  staleBid.LedgerEntry,
		RepairReference: "ticket-refund",
	})
	if err != nil {
		t.Fatalf("RefundAuctionBid(stale bid) error = %v", err)
	}
	if refund.Credit.LedgerEntry.Reason != admin.ReasonAdminAuctionRefund {
		t.Fatalf("refund reason = %q, want admin auction refund", refund.Credit.LedgerEntry.Reason)
	}
	if got := wallet.Balance("bidder-3", economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("bidder balance = %d, want restored 500", got)
	}
	duplicateRefund, err := service.RefundAuctionBid(admin.RefundAuctionBidInput{
		AuctionID:       lot.AuctionID,
		BidLedgerEntry:  staleBid.LedgerEntry,
		RepairReference: "ticket-refund-second-operator",
	})
	if err != nil {
		t.Fatalf("RefundAuctionBid(duplicate admin refund) error = %v", err)
	}
	if !duplicateRefund.Credit.Duplicate {
		t.Fatalf("duplicate refund = %+v, want duplicate credit", duplicateRefund.Credit)
	}
	if got := wallet.Balance("bidder-3", economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("bidder balance after duplicate = %d, want 500", got)
	}
}

func TestAdminRepairsReadyCraftJobThroughCompletion(t *testing.T) {
	now := testAdminNow.Add(2 * time.Hour)
	job := testCraftJob(t, "craft-job-1", "player-1", now.Add(-time.Hour))
	completed := job
	completed.State = crafting.CraftJobStateCompleted
	completedAt := now
	completed.CompletedAt = &completedAt
	completed.ReservationCommittedAt = &completedAt
	completed.OutputGrantedAt = &completedAt
	completed.XPGrantedAt = &completedAt
	fake := &fakeCraftingService{
		job: job,
		completion: crafting.CompleteCraftResult{
			Job: completed,
		},
	}
	service := admin.NewService(admin.ServiceConfig{
		Crafting: fake,
		Clock:    testutil.NewFakeClock(now),
	})

	result, err := service.RepairStuckCraftJob(admin.RepairCraftJobInput{JobID: job.JobID, Now: now})
	if err != nil {
		t.Fatalf("RepairStuckCraftJob() error = %v", err)
	}
	if result.Completion == nil || result.Job.State != crafting.CraftJobStateCompleted {
		t.Fatalf("repair result = %+v, want completed job", result)
	}
	if fake.completeInput.PlayerID != job.PlayerID {
		t.Fatalf("CompleteCraft player = %q, want job player %q", fake.completeInput.PlayerID, job.PlayerID)
	}
}

func TestAdminDryRunsProductionAndRouteSettlementWithoutMutatingStore(t *testing.T) {
	store := production.NewInMemoryStore()
	start := testAdminNow
	settledAt := start.Add(time.Hour)
	initializeProductionPlanet(t, store, "planet-1", start)
	addProductionBuilding(t, store, "planet-1", "extractor-1", production.ProductionDefinitionIDIronExtractorL1, start)
	createProductionRoute(t, store, start, "route-1", "planet-2", "planet-3")
	service := admin.NewService(admin.ServiceConfig{Production: store, Clock: testutil.NewFakeClock(start)})

	planetResult, err := service.DryRunPlanetSettlement(admin.DryRunPlanetSettlementInput{
		PlanetID:  "planet-1",
		SettledAt: settledAt,
	})
	if err != nil {
		t.Fatalf("DryRunPlanetSettlement() error = %v", err)
	}
	if planetResult.NoOp || planetResult.After.Storage.QuantityOf("iron_ore") != 30 {
		t.Fatalf("planet dry-run result = %+v, want 30 iron ore", planetResult)
	}
	originalSnapshot, ok, err := store.Snapshot("planet-1")
	if err != nil || !ok {
		t.Fatalf("Snapshot() ok=%v err=%v, want true nil", ok, err)
	}
	if !originalSnapshot.State.LastCalculatedAt.Equal(start) || originalSnapshot.Storage.QuantityOf("iron_ore") != 0 {
		t.Fatalf("original planet mutated: state=%+v storage=%+v", originalSnapshot.State, originalSnapshot.Storage)
	}

	routeResult, err := service.DryRunRouteSettlement(admin.DryRunRouteSettlementInput{
		RouteID:   "route-1",
		SettledAt: settledAt,
	})
	if err != nil {
		t.Fatalf("DryRunRouteSettlement() error = %v", err)
	}
	if routeResult.NoOp || routeResult.TakenAmount != 40 || routeResult.AddedAmount != 40 {
		t.Fatalf("route dry-run result = %+v, want transfer of 40", routeResult)
	}
	source, ok, err := store.PlanetStorage("planet-2")
	if err != nil || !ok {
		t.Fatalf("source storage ok=%v err=%v, want true nil", ok, err)
	}
	destination, ok, err := store.PlanetStorage("planet-3")
	if err != nil || !ok {
		t.Fatalf("destination storage ok=%v err=%v, want true nil", ok, err)
	}
	if source.QuantityOf("refined_alloy") != 100 || destination.QuantityOf("refined_alloy") != 0 {
		t.Fatalf("original route storage mutated: source=%+v destination=%+v", source, destination)
	}
}

func addItem(
	t *testing.T,
	inventory *economy.InventoryService,
	playerID foundation.PlayerID,
	definition economy.ItemDefinition,
	quantity int64,
	location economy.ItemLocation,
	reference string,
) economy.AddItemResult {
	t.Helper()
	result, err := inventory.AddItem(economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: definition,
		Quantity:       quantity,
		Location:       location,
		Reason:         "test_seed",
		ReferenceKey:   mustAdminKey(t, reference, "add"),
	})
	if err != nil {
		t.Fatalf("AddItem() error = %v", err)
	}
	return result
}

func creditWallet(
	t *testing.T,
	wallet *economy.WalletService,
	playerID foundation.PlayerID,
	currency economy.CurrencyBucket,
	amount int64,
	reference string,
) economy.CreditWalletResult {
	t.Helper()
	result, err := wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     currency,
		Amount:       amount,
		Reason:       "test_seed",
		ReferenceKey: mustAdminKey(t, reference, "credit"),
	})
	if err != nil {
		t.Fatalf("CreditWallet() error = %v", err)
	}
	return result
}

func testItemDefinition(t *testing.T, itemID foundation.ItemID, itemType economy.ItemType) economy.ItemDefinition {
	t.Helper()
	source, err := catalog.NewVersionedDefinitionFromStrings(itemID.String(), "item_catalog_test_v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings() error = %v", err)
	}
	maxStack := int64(99)
	if itemType == economy.ItemTypeInstance {
		maxStack = 1
	}
	maxQuantity, err := foundation.NewQuantity(maxStack)
	if err != nil {
		t.Fatalf("NewQuantity(max) error = %v", err)
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(weight) error = %v", err)
	}
	definition, err := economy.NewItemDefinition(
		source,
		itemID,
		itemID.String(),
		itemType,
		economy.ItemRarityCommon,
		maxQuantity,
		weight,
		[]economy.TradeFlag{economy.TradeFlagTradeable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition() error = %v", err)
	}
	return definition
}

func testAccountLocation(t *testing.T, playerID foundation.PlayerID) economy.ItemLocation {
	t.Helper()
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		t.Fatalf("NewItemLocation() error = %v", err)
	}
	return location
}

func mustAdminKey(t *testing.T, subjectID string, repairReference string) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.AdminCompensationIdempotencyKey(subjectID, repairReference)
	if err != nil {
		t.Fatalf("AdminCompensationIdempotencyKey() error = %v", err)
	}
	return key
}

func newTestMarketService(
	t *testing.T,
	clock *testutil.FakeClock,
	inventory *economy.InventoryService,
	wallet *economy.WalletService,
) *market.MarketService {
	t.Helper()
	service, err := market.NewMarketService(market.MarketServiceConfig{
		Clock:     clock,
		Inventory: inventory,
		Wallet:    wallet,
	})
	if err != nil {
		t.Fatalf("NewMarketService() error = %v", err)
	}
	return service
}

func newTestAuctionService(t *testing.T, clock *testutil.FakeClock, wallet *economy.WalletService) *auction.Service {
	t.Helper()
	service, err := auction.NewService(auction.ServiceConfig{
		Clock:  clock,
		Wallet: wallet,
	})
	if err != nil {
		t.Fatalf("NewAuctionService() error = %v", err)
	}
	return service
}

func createAuctionLot(
	t *testing.T,
	service *auction.Service,
	auctionID foundation.AuctionID,
	startPrice int64,
	buyNowPrice *int64,
) auction.Lot {
	t.Helper()
	source, err := catalog.NewVersionedDefinitionFromStrings("x_core", "auction_catalog_test_v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings() error = %v", err)
	}
	result, err := service.CreateLot(auction.CreateLotInput{
		AuctionID: auctionID,
		WorldID:   "world-1",
		Payload: auction.LotPayload{
			Type:     auction.LotPayloadTypeXCore,
			Source:   source,
			Quantity: 1,
		},
		Currency:    economy.CurrencyBucketCredits,
		StartPrice:  startPrice,
		BuyNowPrice: buyNowPrice,
		StartsAt:    testAdminNow.Add(-time.Minute),
		EndsAt:      testAdminNow.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateLot() error = %v", err)
	}
	return result.Lot
}

type fakeCraftingService struct {
	job           crafting.CraftJob
	completion    crafting.CompleteCraftResult
	completeInput crafting.CompleteCraftInput
}

func (service *fakeCraftingService) Job(jobID crafting.CraftJobID) (crafting.CraftJob, bool) {
	if jobID != service.job.JobID {
		return crafting.CraftJob{}, false
	}
	return service.job, true
}

func (service *fakeCraftingService) CompleteCraft(input crafting.CompleteCraftInput) (crafting.CompleteCraftResult, error) {
	service.completeInput = input
	return service.completion, nil
}

func testCraftJob(t *testing.T, jobID crafting.CraftJobID, playerID foundation.PlayerID, completesAt time.Time) crafting.CraftJob {
	t.Helper()
	source, err := catalog.NewVersionedDefinitionFromStrings("alloy_recipe", "recipe_catalog_test_v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings() error = %v", err)
	}
	return crafting.CraftJob{
		JobID:         jobID,
		PlayerID:      playerID,
		RecipeSource:  source,
		ReservationID: economy.ReservationID(jobID.String()),
		Location: crafting.CraftLocation{
			Type: crafting.CraftLocationStation,
			ID:   "station-1",
		},
		State:       crafting.CraftJobStateRunning,
		StartedAt:   completesAt.Add(-time.Hour),
		CompletesAt: completesAt,
	}
}

func initializeProductionPlanet(t *testing.T, store *production.InMemoryStore, planetID foundation.PlanetID, start time.Time) {
	t.Helper()
	_, err := store.InitializePlanetProduction(production.InitializePlanetProductionInput{
		PlanetID:              planetID,
		LastCalculatedAt:      start,
		StorageCapacityUnits:  1_000,
		EnergyCapacityPerHour: 16,
		UpdatedAt:             start,
	})
	if err != nil {
		t.Fatalf("InitializePlanetProduction() error = %v", err)
	}
}

func addProductionBuilding(
	t *testing.T,
	store *production.InMemoryStore,
	planetID foundation.PlanetID,
	buildingID production.BuildingID,
	definitionID catalog.DefinitionID,
	start time.Time,
) {
	t.Helper()
	definition, err := production.MustMVPCatalog().MustGet(definitionID)
	if err != nil {
		t.Fatalf("MustGet() error = %v", err)
	}
	building, err := production.NewPlanetBuilding(buildingID, planetID, definition, production.BuildingStateActive, start, start)
	if err != nil {
		t.Fatalf("NewPlanetBuilding() error = %v", err)
	}
	if _, _, err := store.UpsertBuilding(building); err != nil {
		t.Fatalf("UpsertBuilding() error = %v", err)
	}
}

func createProductionRoute(
	t *testing.T,
	store *production.InMemoryStore,
	start time.Time,
	routeID foundation.RouteID,
	sourcePlanetID foundation.PlanetID,
	destinationPlanetID foundation.PlanetID,
) {
	t.Helper()
	saveProductionStorage(t, store, sourcePlanetID, 100, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}}, start)
	saveProductionStorage(t, store, destinationPlanetID, 100, nil, start)
	clock := testutil.NewFakeClock(start)
	service, err := production.NewAutomationRouteService(production.AutomationRouteServiceConfig{
		Store:  store,
		Clock:  clock,
		Policy: testRoutePolicy{},
	})
	if err != nil {
		t.Fatalf("NewAutomationRouteService() error = %v", err)
	}
	destination, err := production.NewPlanetRouteDestination(destinationPlanetID)
	if err != nil {
		t.Fatalf("NewPlanetRouteDestination() error = %v", err)
	}
	if _, err := service.CreateRoute(production.CreateRouteInput{
		RouteID:        routeID,
		OwnerPlayerID:  "route-owner",
		SourcePlanetID: sourcePlanetID,
		Destination:    destination,
		ResourceItemID: "refined_alloy",
		AmountPerHour:  40,
	}); err != nil {
		t.Fatalf("CreateRoute() error = %v", err)
	}
}

func saveProductionStorage(
	t *testing.T,
	store *production.InMemoryStore,
	planetID foundation.PlanetID,
	capacity int64,
	items []production.StoredItem,
	updatedAt time.Time,
) {
	t.Helper()
	storage, err := production.NewPlanetStorage(planetID, capacity, items, updatedAt)
	if err != nil {
		t.Fatalf("NewPlanetStorage() error = %v", err)
	}
	if err := store.SavePlanetStorage(storage); err != nil {
		t.Fatalf("SavePlanetStorage() error = %v", err)
	}
}

type testRoutePolicy struct{}

func (testRoutePolicy) RouteCreatePolicy(input production.RouteCreatePolicyInput) (production.RouteCreatePolicy, error) {
	if err := input.Validate(); err != nil {
		return production.RouteCreatePolicy{}, err
	}
	return production.RouteCreatePolicy{
		SourcePlanetOwned:     true,
		DestinationAccessible: true,
		ResourceRouteable:     true,
		RequirementsMet:       true,
		SourceMapID:           "map_1_1",
		DestinationMapID:      "map_1_1",
		DistanceUnits:         10,
		MaxDistanceUnits:      100,
		EnergyCostPerHour:     1,
	}, nil
}

var testAdminNow = time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
