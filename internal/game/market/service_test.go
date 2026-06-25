package market

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

var marketTestNow = time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

type marketFixture struct {
	clock          *testutil.FakeClock
	inventory      *economy.InventoryService
	wallet         *economy.WalletService
	service        *MarketService
	listingStore   *fakeMarketListingRepository
	sellerID       foundation.PlayerID
	buyerID        foundation.PlayerID
	otherBuyerID   foundation.PlayerID
	sourceLocation economy.ItemLocation
	buyerLocation  economy.ItemLocation
	definition     economy.ItemDefinition
	economyStore   *memoryMarketEconomyStore
}

func TestCreateListingMovesStackableItemsIntoEscrow(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 20, "seed-create")

	result := fixture.createListing(t, "listing-create", 8, 25)

	if result.Listing.Status != ListingStatusActive {
		t.Fatalf("listing status = %q, want %q", result.Listing.Status, ListingStatusActive)
	}
	if result.Listing.OriginalQuantity != 8 || result.Listing.RemainingQuantity != 8 {
		t.Fatalf("listing quantities = original %d remaining %d, want 8/8", result.Listing.OriginalQuantity, result.Listing.RemainingQuantity)
	}
	if result.Listing.UnitPrice != 25 {
		t.Fatalf("unit price = %d, want 25", result.Listing.UnitPrice)
	}
	if result.Listing.Currency != economy.CurrencyBucketCredits {
		t.Fatalf("currency = %q, want credits", result.Listing.Currency)
	}
	if result.Listing.SourceReturnLocation != fixture.sourceLocation {
		t.Fatalf("source return location = %v, want %v", result.Listing.SourceReturnLocation, fixture.sourceLocation)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, fixture.sourceLocation); got != 12 {
		t.Fatalf("source quantity = %d, want 12", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, result.Listing.EscrowLocation); got != 8 {
		t.Fatalf("escrow quantity = %d, want 8", got)
	}
	if len(result.EscrowMove.LedgerEntries) != 2 {
		t.Fatalf("escrow move ledger entries len = %d, want 2", len(result.EscrowMove.LedgerEntries))
	}
}

func TestCreateListingRejectsInvalidInputsWithoutMutation(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, *marketFixture, *CreateListingInput)
		wantErr error
	}{
		{
			name: "invalid quantity",
			prepare: func(t *testing.T, fixture *marketFixture, input *CreateListingInput) {
				input.Quantity = 0
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "invalid price",
			prepare: func(t *testing.T, fixture *marketFixture, input *CreateListingInput) {
				input.UnitPrice = 0
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "non-tradeable definition",
			prepare: func(t *testing.T, fixture *marketFixture, input *CreateListingInput) {
				input.ItemRef.Definition = marketStackableDefinition(t, "bound-ore", nil)
			},
			wantErr: economy.ErrItemNotMarketTradeable,
		},
		{
			name: "insufficient quantity",
			prepare: func(t *testing.T, fixture *marketFixture, input *CreateListingInput) {
				input.Quantity = 11
			},
			wantErr: economy.ErrInsufficientItemQuantity,
		},
		{
			name: "equipped source",
			prepare: func(t *testing.T, fixture *marketFixture, input *CreateListingInput) {
				input.SourceLocation = mustLocation(t, economy.LocationKindShipEquipped, "ship-1")
			},
			wantErr: economy.ErrBlockedEquippedItem,
		},
		{
			name: "reserved source",
			prepare: func(t *testing.T, fixture *marketFixture, input *CreateListingInput) {
				input.SourceLocation = mustLocation(t, economy.LocationKindCraftingReserved, "craft-job-1")
			},
			wantErr: economy.ErrBlockedPlayerTradeOrEquipLocation,
		},
		{
			name: "market escrow source",
			prepare: func(t *testing.T, fixture *marketFixture, input *CreateListingInput) {
				input.SourceLocation = mustLocation(t, economy.LocationKindMarketEscrow, "listing-other")
			},
			wantErr: economy.ErrBlockedPlayerTradeOrEquipLocation,
		},
		{
			name: "auction escrow source",
			prepare: func(t *testing.T, fixture *marketFixture, input *CreateListingInput) {
				input.SourceLocation = mustLocation(t, economy.LocationKindAuctionEscrow, "auction-1")
			},
			wantErr: economy.ErrBlockedPlayerTradeOrEquipLocation,
		},
		{
			name: "system sink source",
			prepare: func(t *testing.T, fixture *marketFixture, input *CreateListingInput) {
				input.SourceLocation = mustLocation(t, economy.LocationKindSystemSink, "sink-1")
			},
			wantErr: economy.ErrBlockedPlayerTradeOrEquipLocation,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newMarketFixture(t)
			fixture.seedSellerItems(t, 10, "seed-"+tc.name)
			input := fixture.createListingInput("listing-"+tc.name, 5, 20)
			tc.prepare(t, fixture, &input)

			_, err := fixture.service.CreateListing(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CreateListing error = %v, want %v", err, tc.wantErr)
			}
			if got := len(fixture.service.Listings()); got != 0 {
				t.Fatalf("listings len = %d, want 0", got)
			}
			if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, fixture.sourceLocation); got != 10 {
				t.Fatalf("source quantity = %d, want 10", got)
			}
		})
	}
}

func TestCreateListingDuplicateReferenceReturnsCachedResultWithoutMutation(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 20, "seed-duplicate")
	input := fixture.createListingInput("listing-duplicate", 5, 20)
	first, err := fixture.service.CreateListing(input)
	if err != nil {
		t.Fatalf("first CreateListing: %v", err)
	}
	ledgerCount := len(fixture.inventory.ItemLedgerEntries())

	second, err := fixture.service.CreateListing(input)
	if err != nil {
		t.Fatalf("duplicate CreateListing: %v", err)
	}
	if first.Duplicate {
		t.Fatal("first CreateListing Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate CreateListing Duplicate = false, want true")
	}
	if second.Listing.ListingID != first.Listing.ListingID || second.ReferenceKey != first.ReferenceKey {
		t.Fatalf("duplicate result = %+v, want cached listing/reference from %+v", second, first)
	}
	if second.EscrowMove.LedgerEntries[0].LedgerID != first.EscrowMove.LedgerEntries[0].LedgerID ||
		second.EscrowMove.LedgerEntries[1].LedgerID != first.EscrowMove.LedgerEntries[1].LedgerID {
		t.Fatalf("duplicate escrow ledger ids = %+v, want cached ids %+v", second.EscrowMove.LedgerEntries, first.EscrowMove.LedgerEntries)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, fixture.sourceLocation); got != 15 {
		t.Fatalf("source quantity after duplicate = %d, want 15", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, first.Listing.EscrowLocation); got != 5 {
		t.Fatalf("escrow quantity after duplicate = %d, want 5", got)
	}
	if got := len(fixture.service.Listings()); got != 1 {
		t.Fatalf("listings len = %d, want 1", got)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != ledgerCount {
		t.Fatalf("item ledger entries after duplicate = %d, want %d", got, ledgerCount)
	}

	mismatch := input
	mismatch.Quantity = 6
	_, err = fixture.service.CreateListing(mismatch)
	if !errors.Is(err, ErrCreateListingReferenceMismatch) {
		t.Fatalf("mismatched duplicate CreateListing error = %v, want ErrCreateListingReferenceMismatch", err)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != ledgerCount {
		t.Fatalf("item ledger entries after mismatch = %d, want %d", got, ledgerCount)
	}
}

func TestCreateListingPersistsRepositorySnapshotWhenConfigured(t *testing.T) {
	fixture := newMarketFixtureWithListingRepository(t)
	fixture.seedSellerItems(t, 20, "seed-repository-create")

	result := fixture.createListing(t, "listing-repository-create", 8, 25)

	saved, ok := fixture.listingStore.saved(result.Listing.ListingID)
	if !ok {
		t.Fatalf("saved listing %q ok = false, want true", result.Listing.ListingID)
	}
	if saved.Status != ListingStatusActive || saved.OriginalQuantity != 8 || saved.RemainingQuantity != 8 {
		t.Fatalf("saved listing = status %q original %d remaining %d, want active 8/8", saved.Status, saved.OriginalQuantity, saved.RemainingQuantity)
	}
	if saved.SourceReturnLocation != fixture.sourceLocation || saved.EscrowLocation != result.Listing.EscrowLocation {
		t.Fatalf("saved locations = source %v escrow %v, want source %v escrow %v", saved.SourceReturnLocation, saved.EscrowLocation, fixture.sourceLocation, result.Listing.EscrowLocation)
	}
}

func TestCreateListingNewReferenceForSameItemValidatesCurrentSourceQuantity(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 20, "seed-new-reference")
	_ = fixture.createListing(t, "listing-first-reference", 5, 20)
	ledgerCount := len(fixture.inventory.ItemLedgerEntries())

	_, err := fixture.service.CreateListing(fixture.createListingInput("listing-new-reference", 16, 20))
	if !errors.Is(err, economy.ErrInsufficientItemQuantity) {
		t.Fatalf("new reference CreateListing error = %v, want economy.ErrInsufficientItemQuantity", err)
	}
	if got := len(fixture.service.Listings()); got != 1 {
		t.Fatalf("listings len = %d, want 1", got)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != ledgerCount {
		t.Fatalf("item ledger entries after failed new reference = %d, want %d", got, ledgerCount)
	}
}

func TestBuyListingPersistsRepositorySnapshotWhenConfigured(t *testing.T) {
	fixture := newMarketFixtureWithListingRepository(t)
	fixture.seedSellerItems(t, 5, "seed-repository-buy")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-repository-buy")
	create := fixture.createListing(t, "listing-repository-buy", 5, 10)

	partial, err := fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      2,
		RequestID:     "buy-repository-partial",
	})
	if err != nil {
		t.Fatalf("partial BuyListing: %v", err)
	}
	saved, ok := fixture.listingStore.saved(create.Listing.ListingID)
	if !ok {
		t.Fatalf("saved listing %q ok = false, want true", create.Listing.ListingID)
	}
	if saved.Status != ListingStatusActive || saved.RemainingQuantity != 3 {
		t.Fatalf("partial saved listing status/remaining = %q/%d, want active/3", saved.Status, saved.RemainingQuantity)
	}
	if saved.UpdatedAt != partial.Listing.UpdatedAt {
		t.Fatalf("partial saved updated_at = %s, want %s", saved.UpdatedAt, partial.Listing.UpdatedAt)
	}

	final, err := fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      3,
		RequestID:     "buy-repository-final",
	})
	if err != nil {
		t.Fatalf("final BuyListing: %v", err)
	}
	saved, ok = fixture.listingStore.saved(create.Listing.ListingID)
	if !ok {
		t.Fatalf("saved listing %q after final ok = false, want true", create.Listing.ListingID)
	}
	if saved.Status != ListingStatusSold || saved.RemainingQuantity != 0 || final.Listing.Status != ListingStatusSold {
		t.Fatalf("final saved listing = status %q remaining %d, result status %q, want sold/0/sold", saved.Status, saved.RemainingQuantity, final.Listing.Status)
	}
}

func TestBuyListingRepositoryTransactionSeamCommitsSettlementRows(t *testing.T) {
	fixture, listingStore := newMarketFixtureWithListingTransactionRepository(t)
	fixture.seedSellerItems(t, 5, "seed-transaction-buy")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-transaction-buy")
	create := fixture.createListing(t, "listing-transaction-buy", 5, 10)

	result, err := fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      2,
		RequestID:     "buy-transaction",
	})
	if err != nil {
		t.Fatalf("BuyListing: %v", err)
	}

	if got := listingStore.transactionCount(); got != 1 {
		t.Fatalf("market transaction count = %d, want 1", got)
	}
	if got := listingStore.lockedListingCount(); got != 1 {
		t.Fatal("market transaction did not lock listing")
	}
	if got := len(listingStore.walletCommitsSnapshot()); got != 3 {
		t.Fatalf("wallet commits = %d, want buyer debit, seller credit, fee credit", got)
	}
	if got := len(listingStore.inventoryCommitsSnapshot()); got != 1 {
		t.Fatalf("inventory commits = %d, want 1 escrow-to-buyer move", got)
	}
	row, ok := listingStore.idempotencyRow(economy.IdempotencyScopeEconomy, result.ReferenceKey)
	if !ok || row.Status != economy.IdempotencyStatusCompleted {
		t.Fatalf("buy idempotency row = %+v ok %v, want completed", row, ok)
	}
	if _, ok := listingStore.outboxRow("market_buy:" + result.ReferenceKey.String()); !ok {
		t.Fatalf("buy outbox row missing for reference %q", result.ReferenceKey)
	}
	saved, ok := listingStore.saved(create.Listing.ListingID)
	if !ok || saved.RemainingQuantity != 3 || saved.Status != ListingStatusActive {
		t.Fatalf("saved listing after buy = %+v ok %v, want active remaining 3", saved, ok)
	}
}

func TestBuyListingTransfersItemsCurrencyAndRecordsTotals(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 10, "seed-buy")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-buy")
	create := fixture.createListing(t, "listing-buy", 5, 10)

	result, err := fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      2,
		RequestID:     "buy-1",
	})
	if err != nil {
		t.Fatalf("BuyListing: %v", err)
	}

	if result.TotalAmount != 20 || result.FeeAmount != 1 || result.SellerProceeds != 19 {
		t.Fatalf("settlement totals = total %d fee %d proceeds %d, want 20/1/19", result.TotalAmount, result.FeeAmount, result.SellerProceeds)
	}
	if got := fixture.wallet.Balance(fixture.buyerID, economy.CurrencyBucketCredits); got != 980 {
		t.Fatalf("buyer balance = %d, want 980", got)
	}
	if got := fixture.wallet.Balance(fixture.sellerID, economy.CurrencyBucketCredits); got != 19 {
		t.Fatalf("seller balance = %d, want 19", got)
	}
	if got := fixture.wallet.Balance(defaultSystemFeePlayerID, economy.CurrencyBucketCredits); got != 1 {
		t.Fatalf("fee balance = %d, want 1", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.buyerID, fixture.definition.ItemID, fixture.buyerLocation); got != 2 {
		t.Fatalf("buyer item quantity = %d, want 2", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 3 {
		t.Fatalf("escrow quantity = %d, want 3", got)
	}
	if result.Listing.RemainingQuantity != 3 || result.Listing.Status != ListingStatusActive {
		t.Fatalf("listing after buy = remaining %d status %q, want 3 active", result.Listing.RemainingQuantity, result.Listing.Status)
	}
	if result.FeeCredit == nil {
		t.Fatal("FeeCredit = nil, want fee credit result")
	}
}

func TestBuyListingHighValueSaleRecordsSuspiciousTradeLog(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 10, "seed-suspicious")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-suspicious")
	create := fixture.createListing(t, "listing-suspicious", 5, 50)

	result, err := fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      2,
		RequestID:     "buy-suspicious",
	})
	if err != nil {
		t.Fatalf("BuyListing suspicious: %v", err)
	}

	logs := fixture.service.SuspiciousTradeLogs()
	if len(logs) != 1 {
		t.Fatalf("suspicious logs len = %d, want 1", len(logs))
	}
	log := logs[0]
	if log.ListingID != create.Listing.ListingID || log.SellerPlayerID != fixture.sellerID || log.BuyerPlayerID != fixture.buyerID {
		t.Fatalf("suspicious log identity = %+v, want listing/seller/buyer", log)
	}
	if log.TotalAmount != result.TotalAmount || log.Quantity != 2 || log.UnitPrice != 50 {
		t.Fatalf("suspicious log totals = %+v, want total %d quantity 2 unit 50", log, result.TotalAmount)
	}
	if log.Reason != "high_value_market_sale" || log.ReferenceKey != result.ReferenceKey {
		t.Fatalf("suspicious log reason/reference = %q/%q, want high_value_market_sale/%q", log.Reason, log.ReferenceKey, result.ReferenceKey)
	}
}

func TestBuyListingFullQuantityMarksSoldAndClearsEscrow(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-full-buy")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-full-buy")
	create := fixture.createListing(t, "listing-full-buy", 5, 10)

	result, err := fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      5,
		RequestID:     "buy-full",
	})
	if err != nil {
		t.Fatalf("BuyListing full: %v", err)
	}

	if result.Listing.Status != ListingStatusSold {
		t.Fatalf("listing status = %q, want sold", result.Listing.Status)
	}
	if result.Listing.RemainingQuantity != 0 {
		t.Fatalf("remaining quantity = %d, want 0", result.Listing.RemainingQuantity)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 0 {
		t.Fatalf("escrow quantity = %d, want 0", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.buyerID, fixture.definition.ItemID, fixture.buyerLocation); got != 5 {
		t.Fatalf("buyer item quantity = %d, want 5", got)
	}
}

func TestBuyListingDuplicateRetryReturnsPreviousResultWithoutDuplication(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-duplicate-buy")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-duplicate-buy")
	create := fixture.createListing(t, "listing-duplicate-buy", 5, 10)
	input := BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      2,
		RequestID:     "buy-duplicate",
	}

	first, err := fixture.service.BuyListing(input)
	if err != nil {
		t.Fatalf("first BuyListing: %v", err)
	}
	walletLedgerAfterFirst := len(fixture.wallet.CurrencyLedgerEntries())
	itemLedgerAfterFirst := len(fixture.inventory.ItemLedgerEntries())
	outboxRowsAfterFirst := len(fixture.service.MarketBuyOutboxRows())
	second, err := fixture.service.BuyListing(input)
	if err != nil {
		t.Fatalf("duplicate BuyListing: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate Duplicate = false, want true")
	}
	if second.ReferenceKey != first.ReferenceKey {
		t.Fatalf("duplicate reference = %q, want %q", second.ReferenceKey, first.ReferenceKey)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.buyerID, fixture.definition.ItemID, fixture.buyerLocation); got != 2 {
		t.Fatalf("buyer item quantity = %d, want 2", got)
	}
	if got := fixture.wallet.Balance(fixture.buyerID, economy.CurrencyBucketCredits); got != 980 {
		t.Fatalf("buyer balance = %d, want 980", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != walletLedgerAfterFirst {
		t.Fatalf("wallet ledger entries len = %d, want %d", got, walletLedgerAfterFirst)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != itemLedgerAfterFirst {
		t.Fatalf("item ledger entries len = %d, want %d", got, itemLedgerAfterFirst)
	}
	if got := len(fixture.service.MarketBuyOutboxRows()); got != outboxRowsAfterFirst {
		t.Fatalf("market buy outbox rows len = %d, want %d", got, outboxRowsAfterFirst)
	}
}

func TestBuyListingItemMoveFailureRollsBackWalletMutations(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-item-failure")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-item-failure")
	create := fixture.createListing(t, "listing-item-failure", 5, 10)
	injectedErr := errors.New("injected market buy item move failure")
	fixture.service.inventory = failingMarketBuyInventory{
		delegate:   fixture.inventory,
		failReason: marketBuyReason,
		err:        injectedErr,
	}
	walletLedgerBefore := len(fixture.wallet.CurrencyLedgerEntries())
	itemLedgerBefore := len(fixture.inventory.ItemLedgerEntries())
	outboxRowsBefore := len(fixture.service.MarketBuyOutboxRows())

	_, err := fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      2,
		RequestID:     "buy-item-failure",
	})
	if !errors.Is(err, injectedErr) {
		t.Fatalf("BuyListing error = %v, want injected item move failure", err)
	}
	listing, ok := fixture.service.Listing(create.Listing.ListingID)
	if !ok {
		t.Fatal("listing missing")
	}
	if listing.RemainingQuantity != 5 || listing.Status != ListingStatusActive {
		t.Fatalf("listing after failed buy = remaining %d status %q, want 5 active", listing.RemainingQuantity, listing.Status)
	}
	if got := fixture.wallet.Balance(fixture.buyerID, economy.CurrencyBucketCredits); got != 1_000 {
		t.Fatalf("buyer balance after rollback = %d, want 1000", got)
	}
	if got := fixture.wallet.Balance(fixture.sellerID, economy.CurrencyBucketCredits); got != 0 {
		t.Fatalf("seller balance after rollback = %d, want 0", got)
	}
	if got := fixture.wallet.Balance(defaultSystemFeePlayerID, economy.CurrencyBucketCredits); got != 0 {
		t.Fatalf("fee balance after rollback = %d, want 0", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 5 {
		t.Fatalf("escrow quantity after failed buy = %d, want 5", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.buyerID, fixture.definition.ItemID, fixture.buyerLocation); got != 0 {
		t.Fatalf("buyer item quantity after failed buy = %d, want 0", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != walletLedgerBefore {
		t.Fatalf("wallet ledger entries after failed buy = %d, want %d", got, walletLedgerBefore)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != itemLedgerBefore {
		t.Fatalf("item ledger entries after failed buy = %d, want %d", got, itemLedgerBefore)
	}
	if got := len(fixture.service.MarketBuyOutboxRows()); got != outboxRowsBefore {
		t.Fatalf("market buy outbox rows after failed buy = %d, want %d", got, outboxRowsBefore)
	}
}

func TestBuyListingRepositoryTransactionRollbackLeavesNoPartialRows(t *testing.T) {
	fixture, listingStore := newMarketFixtureWithListingTransactionRepository(t)
	fixture.seedSellerItems(t, 5, "seed-transaction-rollback")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-transaction-rollback")
	create := fixture.createListing(t, "listing-transaction-rollback", 5, 10)
	injectedErr := errors.New("injected market transaction outbox failure")
	listingStore.failInsertOutbox = injectedErr

	_, err := fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      2,
		RequestID:     "buy-transaction-rollback",
	})
	if !errors.Is(err, injectedErr) {
		t.Fatalf("BuyListing error = %v, want injected transaction failure", err)
	}
	saved, ok := listingStore.saved(create.Listing.ListingID)
	if !ok || saved.RemainingQuantity != 5 || saved.Status != ListingStatusActive {
		t.Fatalf("repository listing after rollback = %+v ok %v, want original active remaining 5", saved, ok)
	}
	if got := len(listingStore.walletCommitsSnapshot()); got != 0 {
		t.Fatalf("wallet commits after rollback = %d, want 0", got)
	}
	if got := len(listingStore.inventoryCommitsSnapshot()); got != 0 {
		t.Fatalf("inventory commits after rollback = %d, want 0", got)
	}
	referenceKey, keyErr := foundation.MarketBuyIdempotencyKey(create.Listing.ListingID, fixture.buyerID, "buy-transaction-rollback")
	if keyErr != nil {
		t.Fatalf("MarketBuyIdempotencyKey: %v", keyErr)
	}
	if row, ok := listingStore.idempotencyRow(economy.IdempotencyScopeEconomy, referenceKey); ok {
		t.Fatalf("idempotency row after rollback = %+v, want none", row)
	}
	if _, ok := listingStore.outboxRow("market_buy:" + referenceKey.String()); ok {
		t.Fatalf("outbox row after rollback exists for reference %q, want none", referenceKey)
	}
	listing, ok := fixture.service.Listing(create.Listing.ListingID)
	if !ok || listing.RemainingQuantity != 5 || listing.Status != ListingStatusActive {
		t.Fatalf("service listing after rollback = %+v ok %v, want original active remaining 5", listing, ok)
	}
	if got := fixture.wallet.Balance(fixture.buyerID, economy.CurrencyBucketCredits); got != 1_000 {
		t.Fatalf("buyer balance after rollback = %d, want 1000", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.buyerID, fixture.definition.ItemID, fixture.buyerLocation); got != 0 {
		t.Fatalf("buyer item quantity after rollback = %d, want 0", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 5 {
		t.Fatalf("escrow quantity after rollback = %d, want 5", got)
	}
}

func TestBuyListingInsufficientFundsLeavesListingAndEscrowUnchanged(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-insufficient")
	fixture.seedCredits(t, fixture.buyerID, 10, "buyer-insufficient")
	create := fixture.createListing(t, "listing-insufficient", 5, 10)

	_, err := fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      2,
		RequestID:     "buy-insufficient",
	})
	if !errors.Is(err, economy.ErrInsufficientWalletFunds) {
		t.Fatalf("BuyListing error = %v, want ErrInsufficientWalletFunds", err)
	}
	listing, ok := fixture.service.Listing(create.Listing.ListingID)
	if !ok {
		t.Fatal("listing missing")
	}
	if listing.RemainingQuantity != 5 || listing.Status != ListingStatusActive {
		t.Fatalf("listing after insufficient funds = remaining %d status %q, want 5 active", listing.RemainingQuantity, listing.Status)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 5 {
		t.Fatalf("escrow quantity = %d, want 5", got)
	}
	if got := fixture.wallet.Balance(fixture.buyerID, economy.CurrencyBucketCredits); got != 10 {
		t.Fatalf("buyer balance = %d, want 10", got)
	}
}

func TestCancelListingReturnsRemainingEscrowAndDuplicateDoesNotReturnTwice(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 10, "seed-cancel")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-cancel")
	create := fixture.createListing(t, "listing-cancel", 5, 10)
	if _, err := fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      2,
		RequestID:     "buy-before-cancel",
	}); err != nil {
		t.Fatalf("partial BuyListing before cancel: %v", err)
	}

	first, err := fixture.service.CancelListing(CancelListingInput{
		SellerPlayerID: fixture.sellerID,
		ListingID:      create.Listing.ListingID,
		RequestID:      "cancel-1",
	})
	if err != nil {
		t.Fatalf("CancelListing: %v", err)
	}
	itemLedgerAfterFirst := len(fixture.inventory.ItemLedgerEntries())
	second, err := fixture.service.CancelListing(CancelListingInput{
		SellerPlayerID: fixture.sellerID,
		ListingID:      create.Listing.ListingID,
		RequestID:      "cancel-2",
	})
	if err != nil {
		t.Fatalf("duplicate CancelListing: %v", err)
	}

	if first.ReturnedQuantity != 3 {
		t.Fatalf("returned quantity = %d, want 3", first.ReturnedQuantity)
	}
	if first.Listing.Status != ListingStatusCancelled {
		t.Fatalf("listing status = %q, want cancelled", first.Listing.Status)
	}
	if !second.Duplicate {
		t.Fatal("duplicate cancel Duplicate = false, want true")
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, fixture.sourceLocation); got != 8 {
		t.Fatalf("seller source quantity = %d, want 8", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 0 {
		t.Fatalf("escrow quantity = %d, want 0", got)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != itemLedgerAfterFirst {
		t.Fatalf("item ledger entries len = %d, want %d", got, itemLedgerAfterFirst)
	}
}

func TestCancelListingPersistsRepositorySnapshotWhenConfigured(t *testing.T) {
	fixture := newMarketFixtureWithListingRepository(t)
	fixture.seedSellerItems(t, 5, "seed-repository-cancel")
	create := fixture.createListing(t, "listing-repository-cancel", 5, 10)

	result, err := fixture.service.CancelListing(CancelListingInput{
		SellerPlayerID: fixture.sellerID,
		ListingID:      create.Listing.ListingID,
		RequestID:      "cancel-repository",
	})
	if err != nil {
		t.Fatalf("CancelListing: %v", err)
	}

	saved, ok := fixture.listingStore.saved(create.Listing.ListingID)
	if !ok {
		t.Fatalf("saved listing %q ok = false, want true", create.Listing.ListingID)
	}
	if saved.Status != ListingStatusCancelled || saved.RemainingQuantity != 5 || result.ReturnedQuantity != 5 {
		t.Fatalf("saved cancel listing = status %q remaining %d returned %d, want cancelled/5/5", saved.Status, saved.RemainingQuantity, result.ReturnedQuantity)
	}
}

func TestCancelListingRepositoryTransactionSeamCommitsSettlementRows(t *testing.T) {
	fixture, listingStore := newMarketFixtureWithListingTransactionRepository(t)
	fixture.seedSellerItems(t, 5, "seed-transaction-cancel")
	create := fixture.createListing(t, "listing-transaction-cancel", 5, 10)

	result, err := fixture.service.CancelListing(CancelListingInput{
		SellerPlayerID: fixture.sellerID,
		ListingID:      create.Listing.ListingID,
		RequestID:      "cancel-transaction",
	})
	if err != nil {
		t.Fatalf("CancelListing: %v", err)
	}

	if got := listingStore.transactionCount(); got != 1 {
		t.Fatalf("market transaction count = %d, want 1", got)
	}
	if got := listingStore.lockedListingCount(); got != 1 {
		t.Fatal("market transaction did not lock listing")
	}
	if got := len(listingStore.walletCommitsSnapshot()); got != 0 {
		t.Fatalf("wallet commits = %d, want 0 for cancel", got)
	}
	if got := len(listingStore.inventoryCommitsSnapshot()); got != 1 {
		t.Fatalf("inventory commits = %d, want 1 escrow return move", got)
	}
	row, ok := listingStore.idempotencyRow(economy.IdempotencyScopeEconomy, result.ReferenceKey)
	if !ok || row.Status != economy.IdempotencyStatusCompleted {
		t.Fatalf("cancel idempotency row = %+v ok %v, want completed", row, ok)
	}
	if _, ok := listingStore.outboxRow("market_cancel:" + result.ReferenceKey.String()); !ok {
		t.Fatalf("cancel outbox row missing for reference %q", result.ReferenceKey)
	}
	saved, ok := listingStore.saved(create.Listing.ListingID)
	if !ok || saved.Status != ListingStatusCancelled {
		t.Fatalf("saved listing after cancel = %+v ok %v, want cancelled", saved, ok)
	}
}

func TestCancelListingIdempotencyCompletionFailureRollsBackReturnMove(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-cancel-rollback")
	create := fixture.createListing(t, "listing-cancel-rollback", 5, 10)
	injectedErr := errors.New("injected market cancel idempotency completion failure")
	fixture.service.idempotencyStore = failingCompleteMarketEconomyStore{
		delegate:  fixture.economyStore,
		operation: marketCancelOperation,
		status:    economy.IdempotencyStatusCompleted,
		err:       injectedErr,
	}
	itemLedgerBefore := len(fixture.inventory.ItemLedgerEntries())

	_, err := fixture.service.CancelListing(CancelListingInput{
		SellerPlayerID: fixture.sellerID,
		ListingID:      create.Listing.ListingID,
		RequestID:      "cancel-rollback",
	})
	if !errors.Is(err, injectedErr) {
		t.Fatalf("CancelListing error = %v, want injected idempotency failure", err)
	}
	listing, ok := fixture.service.Listing(create.Listing.ListingID)
	if !ok {
		t.Fatal("listing missing")
	}
	if listing.RemainingQuantity != 5 || listing.Status != ListingStatusActive {
		t.Fatalf("listing after failed cancel = remaining %d status %q, want 5 active", listing.RemainingQuantity, listing.Status)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, fixture.sourceLocation); got != 0 {
		t.Fatalf("source quantity after failed cancel = %d, want 0", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 5 {
		t.Fatalf("escrow quantity after failed cancel = %d, want 5", got)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != itemLedgerBefore {
		t.Fatalf("item ledger entries after failed cancel = %d, want %d", got, itemLedgerBefore)
	}
}

func TestCancelListingIdempotencyRetryDoesNotReturnEscrowTwice(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-cancel-idempotency")
	create := fixture.createListing(t, "listing-cancel-idempotency", 5, 10)

	first, err := fixture.service.CancelListing(CancelListingInput{
		SellerPlayerID: fixture.sellerID,
		ListingID:      create.Listing.ListingID,
		RequestID:      "cancel-idempotency-1",
	})
	if err != nil {
		t.Fatalf("CancelListing first: %v", err)
	}
	itemLedgerAfterFirst := len(fixture.inventory.ItemLedgerEntries())
	sourceAfterFirst := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, fixture.sourceLocation)
	fixture.service.mu.Lock()
	delete(fixture.service.cancelResults, create.Listing.ListingID)
	fixture.service.mu.Unlock()

	second, err := fixture.service.CancelListing(CancelListingInput{
		SellerPlayerID: fixture.sellerID,
		ListingID:      create.Listing.ListingID,
		RequestID:      "cancel-idempotency-2",
	})
	if err != nil {
		t.Fatalf("CancelListing idempotency retry: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("idempotency retry Duplicate = false, want true")
	}
	if second.ReferenceKey != first.ReferenceKey {
		t.Fatalf("retry reference = %q, want %q", second.ReferenceKey, first.ReferenceKey)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != itemLedgerAfterFirst {
		t.Fatalf("item ledger entries after retry = %d, want %d", got, itemLedgerAfterFirst)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, fixture.sourceLocation); got != sourceAfterFirst {
		t.Fatalf("seller source quantity after retry = %d, want %d", got, sourceAfterFirst)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 0 {
		t.Fatalf("escrow quantity after retry = %d, want 0", got)
	}
}

func TestBuyListingRejectsSellerSelfBuy(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-self-buy")
	fixture.seedCredits(t, fixture.sellerID, 1_000, "seller-self-buy")
	create := fixture.createListing(t, "listing-self-buy", 5, 10)

	_, err := fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.sellerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      1,
		RequestID:     "buy-self",
	})
	if !errors.Is(err, ErrSellerCannotBuyOwnListing) {
		t.Fatalf("self BuyListing error = %v, want ErrSellerCannotBuyOwnListing", err)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 5 {
		t.Fatalf("escrow quantity = %d, want 5", got)
	}
}

func TestBuyListingRejectsExpiredListingWithoutMutatingEscrow(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-expired")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-expired")
	expiresAt := fixture.clock.Now().Add(time.Minute)
	input := fixture.createListingInput("listing-expired", 5, 10)
	input.ExpiresAt = &expiresAt
	create, err := fixture.service.CreateListing(input)
	if err != nil {
		t.Fatalf("CreateListing expiring: %v", err)
	}
	fixture.clock.Advance(2 * time.Minute)

	_, err = fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      1,
		RequestID:     "buy-expired",
	})
	if !errors.Is(err, ErrListingExpired) {
		t.Fatalf("expired BuyListing error = %v, want ErrListingExpired", err)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 5 {
		t.Fatalf("escrow quantity = %d, want 5", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.buyerID, fixture.definition.ItemID, fixture.buyerLocation); got != 0 {
		t.Fatalf("buyer item quantity = %d, want 0", got)
	}
}

func TestExpireListingReturnsEscrowAndDuplicateDoesNotReturnTwice(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-expire-command")
	expiresAt := fixture.clock.Now().Add(time.Minute)
	input := fixture.createListingInput("listing-expire-command", 5, 10)
	input.ExpiresAt = &expiresAt
	create, err := fixture.service.CreateListing(input)
	if err != nil {
		t.Fatalf("CreateListing expiring: %v", err)
	}
	fixture.clock.Advance(2 * time.Minute)

	first, err := fixture.service.ExpireListing(ExpireListingInput{ListingID: create.Listing.ListingID})
	if err != nil {
		t.Fatalf("ExpireListing: %v", err)
	}
	itemLedgerAfterFirst := len(fixture.inventory.ItemLedgerEntries())
	second, err := fixture.service.ExpireListing(ExpireListingInput{ListingID: create.Listing.ListingID})
	if err != nil {
		t.Fatalf("duplicate ExpireListing: %v", err)
	}

	if first.Listing.Status != ListingStatusExpired {
		t.Fatalf("expired listing status = %q, want expired", first.Listing.Status)
	}
	if first.ReturnedQuantity != 5 {
		t.Fatalf("returned quantity = %d, want 5", first.ReturnedQuantity)
	}
	if !second.Duplicate {
		t.Fatal("duplicate expire Duplicate = false, want true")
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, fixture.sourceLocation); got != 5 {
		t.Fatalf("seller source quantity = %d, want 5", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 0 {
		t.Fatalf("escrow quantity = %d, want 0", got)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != itemLedgerAfterFirst {
		t.Fatalf("item ledger entries len = %d, want %d", got, itemLedgerAfterFirst)
	}
}

func TestExpireListingRejectsNotExpiredWithoutMutation(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-not-expired")
	expiresAt := fixture.clock.Now().Add(time.Minute)
	input := fixture.createListingInput("listing-not-expired", 5, 10)
	input.ExpiresAt = &expiresAt
	create, err := fixture.service.CreateListing(input)
	if err != nil {
		t.Fatalf("CreateListing expiring: %v", err)
	}

	_, err = fixture.service.ExpireListing(ExpireListingInput{ListingID: create.Listing.ListingID})
	if !errors.Is(err, ErrListingNotExpired) {
		t.Fatalf("ExpireListing not expired error = %v, want ErrListingNotExpired", err)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, fixture.sourceLocation); got != 0 {
		t.Fatalf("seller source quantity = %d, want 0", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 5 {
		t.Fatalf("escrow quantity = %d, want 5", got)
	}
}

func TestMarkListingStaleBlocksBuyAndAllowsCancel(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-stale")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-stale")
	create := fixture.createListing(t, "listing-stale", 5, 10)

	first, err := fixture.service.MarkListingStale(MarkListingStaleInput{
		ListingID: create.Listing.ListingID,
		Reason:    "planet_claimed",
	})
	if err != nil {
		t.Fatalf("MarkListingStale: %v", err)
	}
	second, err := fixture.service.MarkListingStale(MarkListingStaleInput{
		ListingID: create.Listing.ListingID,
		Reason:    "planet_claimed",
	})
	if err != nil {
		t.Fatalf("duplicate MarkListingStale: %v", err)
	}
	if first.Listing.Status != ListingStatusStale {
		t.Fatalf("listing status = %q, want stale", first.Listing.Status)
	}
	if first.Listing.StaleAt == nil || first.Listing.StaleReason != "planet_claimed" {
		t.Fatalf("stale fields = at %v reason %q, want set/planet_claimed", first.Listing.StaleAt, first.Listing.StaleReason)
	}
	if !second.Duplicate {
		t.Fatal("duplicate stale Duplicate = false, want true")
	}

	_, err = fixture.service.BuyListing(BuyListingInput{
		BuyerPlayerID: fixture.buyerID,
		ListingID:     create.Listing.ListingID,
		Quantity:      1,
		RequestID:     "buy-stale",
	})
	if !errors.Is(err, ErrListingNotActive) {
		t.Fatalf("BuyListing stale error = %v, want ErrListingNotActive", err)
	}

	cancel, err := fixture.service.CancelListing(CancelListingInput{
		SellerPlayerID: fixture.sellerID,
		ListingID:      create.Listing.ListingID,
		RequestID:      "cancel-stale",
	})
	if err != nil {
		t.Fatalf("CancelListing stale: %v", err)
	}
	if cancel.Listing.Status != ListingStatusCancelled {
		t.Fatalf("cancelled stale listing status = %q, want cancelled", cancel.Listing.Status)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, fixture.sourceLocation); got != 5 {
		t.Fatalf("seller source quantity = %d, want 5", got)
	}
	if got := fixture.wallet.Balance(fixture.buyerID, economy.CurrencyBucketCredits); got != 1_000 {
		t.Fatalf("buyer balance = %d, want unchanged 1000", got)
	}
}

func TestListedItemCannotBeEquippedFromEscrow(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 5, "seed-listed-equip")
	create := fixture.createListing(t, "listing-equip-block", 5, 10)
	equippedLocation := mustLocation(t, economy.LocationKindShipEquipped, "ship-1")

	_, err := fixture.inventory.MoveItem(economy.MoveItemInput{
		PlayerID:     fixture.sellerID,
		ItemRef:      economy.MoveItemRef{Definition: fixture.definition},
		FromLocation: create.Listing.EscrowLocation,
		ToLocation:   equippedLocation,
		Quantity:     1,
		Reason:       "test_equip",
		ReferenceKey: mustLootPickupKey(t, "equip-listed"),
	})
	if !errors.Is(err, economy.ErrBlockedGenericMoveSource) {
		t.Fatalf("MoveItem listed escrow error = %v, want ErrBlockedGenericMoveSource", err)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 5 {
		t.Fatalf("escrow quantity = %d, want 5", got)
	}
}

func TestConcurrentFinalBuysCannotOversell(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 1, "seed-concurrent-buy")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-one-concurrent")
	fixture.seedCredits(t, fixture.otherBuyerID, 1_000, "buyer-two-concurrent")
	create := fixture.createListing(t, "listing-concurrent-buy", 1, 10)

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, buyerID := range []foundation.PlayerID{fixture.buyerID, fixture.otherBuyerID} {
		wg.Add(1)
		go func(buyerID foundation.PlayerID) {
			defer wg.Done()
			<-start
			_, err := fixture.service.BuyListing(BuyListingInput{
				BuyerPlayerID: buyerID,
				ListingID:     create.Listing.ListingID,
				Quantity:      1,
				RequestID:     foundation.RequestID("buy-" + buyerID.String()),
			})
			results <- err
		}(buyerID)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrListingNotActive) {
			t.Fatalf("concurrent buy loser error = %v, want ErrListingNotActive", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful buys = %d, want 1", successes)
	}
	buyerOneQuantity := fixture.inventory.TotalItemQuantity(fixture.buyerID, fixture.definition.ItemID, fixture.buyerLocation)
	otherBuyerLocation := mustLocation(t, economy.LocationKindAccountInventory, fixture.otherBuyerID.String())
	buyerTwoQuantity := fixture.inventory.TotalItemQuantity(fixture.otherBuyerID, fixture.definition.ItemID, otherBuyerLocation)
	if buyerOneQuantity+buyerTwoQuantity != 1 {
		t.Fatalf("total buyer quantity = %d, want 1", buyerOneQuantity+buyerTwoQuantity)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation); got != 0 {
		t.Fatalf("escrow quantity = %d, want 0", got)
	}
}

func TestBuyRacingCancelCannotDuplicateItems(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 1, "seed-buy-cancel-race")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "buyer-buy-cancel-race")
	create := fixture.createListing(t, "listing-buy-cancel-race", 1, 10)

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := fixture.service.BuyListing(BuyListingInput{
			BuyerPlayerID: fixture.buyerID,
			ListingID:     create.Listing.ListingID,
			Quantity:      1,
			RequestID:     "buy-cancel-race",
		})
		errs <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err := fixture.service.CancelListing(CancelListingInput{
			SellerPlayerID: fixture.sellerID,
			ListingID:      create.Listing.ListingID,
			RequestID:      "cancel-race",
		})
		errs <- err
	}()
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrListingNotActive) {
			t.Fatalf("buy/cancel loser error = %v, want ErrListingNotActive", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful operations = %d, want 1", successes)
	}
	sourceQuantity := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, fixture.sourceLocation)
	escrowQuantity := fixture.inventory.TotalItemQuantity(fixture.sellerID, fixture.definition.ItemID, create.Listing.EscrowLocation)
	buyerQuantity := fixture.inventory.TotalItemQuantity(fixture.buyerID, fixture.definition.ItemID, fixture.buyerLocation)
	if sourceQuantity+escrowQuantity+buyerQuantity != 1 {
		t.Fatalf("total item quantity = %d, want 1", sourceQuantity+escrowQuantity+buyerQuantity)
	}
	if escrowQuantity != 0 {
		t.Fatalf("escrow quantity = %d, want 0", escrowQuantity)
	}
}

func newMarketFixture(t *testing.T) *marketFixture {
	return newMarketFixtureWithListingStore(t, nil)
}

func newMarketFixtureWithListingRepository(t *testing.T) *marketFixture {
	return newMarketFixtureWithListingStore(t, newFakeMarketListingRepository())
}

func newMarketFixtureWithListingTransactionRepository(t *testing.T) (*marketFixture, *fakeMarketListingTransactionRepository) {
	t.Helper()

	clock := testutil.NewFakeClock(marketTestNow)
	inventory := economy.NewInventoryService(clock)
	wallet := economy.NewWalletService(clock)
	economyStore := newMemoryMarketEconomyStore()
	listingRepository := newFakeMarketListingTransactionRepository()
	service, err := NewMarketService(MarketServiceConfig{
		Clock:             clock,
		Inventory:         inventory,
		Wallet:            wallet,
		ListingRepository: listingRepository,
		IdempotencyStore:  economyStore,
		OutboxStore:       economyStore,
	})
	if err != nil {
		t.Fatalf("NewMarketService(transaction repository): %v", err)
	}

	sellerID := foundation.PlayerID("seller-1")
	buyerID := foundation.PlayerID("buyer-1")
	sourceLocation := mustLocation(t, economy.LocationKindAccountInventory, sellerID.String())
	buyerLocation := mustLocation(t, economy.LocationKindAccountInventory, buyerID.String())

	return &marketFixture{
		clock:          clock,
		inventory:      inventory,
		wallet:         wallet,
		service:        service,
		sellerID:       sellerID,
		buyerID:        buyerID,
		otherBuyerID:   "buyer-2",
		sourceLocation: sourceLocation,
		buyerLocation:  buyerLocation,
		definition:     marketStackableDefinition(t, "raw-ore", []economy.TradeFlag{economy.TradeFlagMarketTradeable}),
		economyStore:   economyStore,
	}, listingRepository
}

func newMarketFixtureWithListingStore(t *testing.T, listingStore *fakeMarketListingRepository) *marketFixture {
	t.Helper()

	clock := testutil.NewFakeClock(marketTestNow)
	inventory := economy.NewInventoryService(clock)
	wallet := economy.NewWalletService(clock)
	economyStore := newMemoryMarketEconomyStore()
	var listingRepository MarketListingRepository
	if listingStore != nil {
		listingRepository = listingStore
	}
	service, err := NewMarketService(MarketServiceConfig{
		Clock:             clock,
		Inventory:         inventory,
		Wallet:            wallet,
		ListingRepository: listingRepository,
		IdempotencyStore:  economyStore,
		OutboxStore:       economyStore,
	})
	if err != nil {
		t.Fatalf("NewMarketService: %v", err)
	}

	sellerID := foundation.PlayerID("seller-1")
	buyerID := foundation.PlayerID("buyer-1")
	sourceLocation := mustLocation(t, economy.LocationKindAccountInventory, sellerID.String())
	buyerLocation := mustLocation(t, economy.LocationKindAccountInventory, buyerID.String())

	return &marketFixture{
		clock:          clock,
		inventory:      inventory,
		wallet:         wallet,
		service:        service,
		listingStore:   listingStore,
		sellerID:       sellerID,
		buyerID:        buyerID,
		otherBuyerID:   "buyer-2",
		sourceLocation: sourceLocation,
		buyerLocation:  buyerLocation,
		definition:     marketStackableDefinition(t, "raw-ore", []economy.TradeFlag{economy.TradeFlagMarketTradeable}),
		economyStore:   economyStore,
	}
}

type fakeMarketListingRepository struct {
	mu       sync.Mutex
	listings map[foundation.ListingID]Listing
}

func newFakeMarketListingRepository() *fakeMarketListingRepository {
	return &fakeMarketListingRepository{listings: make(map[foundation.ListingID]Listing)}
}

func (repository *fakeMarketListingRepository) SaveMarketListing(ctx context.Context, listing Listing) error {
	if repository == nil {
		return errors.New("nil fake market listing repository")
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.listings[listing.ListingID] = cloneListing(listing)
	return nil
}

func (repository *fakeMarketListingRepository) saved(listingID foundation.ListingID) (Listing, bool) {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	listing, ok := repository.listings[listingID]
	if !ok {
		return Listing{}, false
	}
	return cloneListing(listing), true
}

type fakeMarketListingTransactionRepository struct {
	mu               sync.Mutex
	listings         map[foundation.ListingID]Listing
	txCount          int
	lockedListings   []foundation.ListingID
	walletCommits    []economy.WalletMutationCommit
	inventoryCommits []economy.InventoryMoveItemCommit
	idempotency      map[string]economy.IdempotencyKeyRow
	outbox           map[string]economy.OutboxRow
	failInsertOutbox error
}

func newFakeMarketListingTransactionRepository() *fakeMarketListingTransactionRepository {
	return &fakeMarketListingTransactionRepository{
		listings:    make(map[foundation.ListingID]Listing),
		idempotency: make(map[string]economy.IdempotencyKeyRow),
		outbox:      make(map[string]economy.OutboxRow),
	}
}

func (repository *fakeMarketListingTransactionRepository) SaveMarketListing(ctx context.Context, listing Listing) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.listings[listing.ListingID] = cloneListing(listing)
	return nil
}

func (repository *fakeMarketListingTransactionRepository) WithMarketListingTransaction(
	ctx context.Context,
	fn func(MarketListingTransaction) error,
) error {
	repository.mu.Lock()
	tx := &fakeMarketListingTransaction{
		repository:       repository,
		listings:         cloneListingMap(repository.listings),
		walletCommits:    cloneWalletMutationCommits(repository.walletCommits),
		inventoryCommits: cloneInventoryMoveItemCommits(repository.inventoryCommits),
		idempotency:      cloneIdempotencyKeyRowStringMap(repository.idempotency),
		outbox:           cloneOutboxRowMap(repository.outbox),
		failInsertOutbox: repository.failInsertOutbox,
	}
	repository.mu.Unlock()

	err := fn(tx)

	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.txCount++
	repository.lockedListings = append(repository.lockedListings, tx.lockedListings...)
	if err != nil {
		return err
	}
	repository.listings = cloneListingMap(tx.listings)
	repository.walletCommits = cloneWalletMutationCommits(tx.walletCommits)
	repository.inventoryCommits = cloneInventoryMoveItemCommits(tx.inventoryCommits)
	repository.idempotency = cloneIdempotencyKeyRowStringMap(tx.idempotency)
	repository.outbox = cloneOutboxRowMap(tx.outbox)
	return nil
}

func (repository *fakeMarketListingTransactionRepository) saved(listingID foundation.ListingID) (Listing, bool) {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	listing, ok := repository.listings[listingID]
	if !ok {
		return Listing{}, false
	}
	return cloneListing(listing), true
}

func (repository *fakeMarketListingTransactionRepository) transactionCount() int {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	return repository.txCount
}

func (repository *fakeMarketListingTransactionRepository) lockedListingCount() int {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	return len(repository.lockedListings)
}

func (repository *fakeMarketListingTransactionRepository) walletCommitsSnapshot() []economy.WalletMutationCommit {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	return cloneWalletMutationCommits(repository.walletCommits)
}

func (repository *fakeMarketListingTransactionRepository) inventoryCommitsSnapshot() []economy.InventoryMoveItemCommit {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	return cloneInventoryMoveItemCommits(repository.inventoryCommits)
}

func (repository *fakeMarketListingTransactionRepository) idempotencyRow(scope string, key foundation.IdempotencyKey) (economy.IdempotencyKeyRow, bool) {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	row, ok := repository.idempotency[memoryMarketIdempotencyKey(scope, key)]
	return row.Clone(), ok
}

func (repository *fakeMarketListingTransactionRepository) outboxRow(outboxID string) (economy.OutboxRow, bool) {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	row, ok := repository.outbox[outboxID]
	return row.Clone(), ok
}

type fakeMarketListingTransaction struct {
	repository       *fakeMarketListingTransactionRepository
	listings         map[foundation.ListingID]Listing
	lockedListings   []foundation.ListingID
	walletCommits    []economy.WalletMutationCommit
	inventoryCommits []economy.InventoryMoveItemCommit
	idempotency      map[string]economy.IdempotencyKeyRow
	outbox           map[string]economy.OutboxRow
	failInsertOutbox error
}

func (tx *fakeMarketListingTransaction) LoadMarketListingForUpdate(ctx context.Context, listingID foundation.ListingID) (Listing, bool, error) {
	tx.lockedListings = append(tx.lockedListings, listingID)
	listing, ok := tx.listings[listingID]
	if !ok {
		return Listing{}, false, nil
	}
	return cloneListing(listing), true, nil
}

func (tx *fakeMarketListingTransaction) SaveMarketListing(ctx context.Context, listing Listing) error {
	tx.listings[listing.ListingID] = cloneListing(listing)
	return nil
}

func (tx *fakeMarketListingTransaction) CommitWalletMutation(ctx context.Context, commit economy.WalletMutationCommit) error {
	if err := commit.Validate(); err != nil {
		return err
	}
	tx.walletCommits = append(tx.walletCommits, cloneWalletMutationCommit(commit))
	return nil
}

func (tx *fakeMarketListingTransaction) CommitInventoryMoveItem(ctx context.Context, commit economy.InventoryMoveItemCommit) error {
	if err := commit.Validate(); err != nil {
		return err
	}
	tx.inventoryCommits = append(tx.inventoryCommits, cloneInventoryMoveItemCommit(commit))
	return nil
}

func (tx *fakeMarketListingTransaction) ClaimIdempotencyKey(
	ctx context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyClaimResult, error) {
	key := memoryMarketIdempotencyKey(row.Scope, row.Key)
	if existing, ok := tx.idempotency[key]; ok {
		return economy.ResolveIdempotencyClaim(&existing, row)
	}
	claim, err := economy.ResolveIdempotencyClaim(nil, row)
	if err != nil {
		return economy.IdempotencyClaimResult{}, err
	}
	tx.idempotency[key] = claim.Row.Clone()
	return claim, nil
}

func (tx *fakeMarketListingTransaction) CompleteIdempotencyKey(
	ctx context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, error) {
	if err := row.Validate(); err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	tx.idempotency[memoryMarketIdempotencyKey(row.Scope, row.Key)] = row.Clone()
	return row.Clone(), nil
}

func (tx *fakeMarketListingTransaction) InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error {
	if tx.failInsertOutbox != nil {
		return tx.failInsertOutbox
	}
	inserted, err := economy.NewOutboxRow(row)
	if err != nil {
		return err
	}
	if _, exists := tx.outbox[inserted.OutboxID]; exists {
		return fmt.Errorf("outbox %q: %w", inserted.OutboxID, economy.ErrInvalidOutboxRow)
	}
	tx.outbox[inserted.OutboxID] = inserted.Clone()
	return nil
}

func cloneWalletMutationCommits(commits []economy.WalletMutationCommit) []economy.WalletMutationCommit {
	cloned := make([]economy.WalletMutationCommit, 0, len(commits))
	for _, commit := range commits {
		cloned = append(cloned, cloneWalletMutationCommit(commit))
	}
	return cloned
}

func cloneWalletMutationCommit(commit economy.WalletMutationCommit) economy.WalletMutationCommit {
	commit.Balances = append([]economy.WalletBalance(nil), commit.Balances...)
	commit.LedgerEntries = append([]economy.CurrencyLedgerEntry(nil), commit.LedgerEntries...)
	commit.Reference.LedgerEntries = append([]economy.CurrencyLedgerEntry(nil), commit.Reference.LedgerEntries...)
	return commit
}

func cloneInventoryMoveItemCommits(commits []economy.InventoryMoveItemCommit) []economy.InventoryMoveItemCommit {
	cloned := make([]economy.InventoryMoveItemCommit, 0, len(commits))
	for _, commit := range commits {
		cloned = append(cloned, cloneInventoryMoveItemCommit(commit))
	}
	return cloned
}

func cloneInventoryMoveItemCommit(commit economy.InventoryMoveItemCommit) economy.InventoryMoveItemCommit {
	commit.StackableItems = append([]economy.StackableItem(nil), commit.StackableItems...)
	commit.DeletedStackableItems = append([]economy.StackableItem(nil), commit.DeletedStackableItems...)
	commit.InstanceItems = append([]economy.InstanceItem(nil), commit.InstanceItems...)
	commit.LedgerEntries = append([]economy.ItemLedgerEntry(nil), commit.LedgerEntries...)
	commit.Reference.Result = cloneMoveItemResult(commit.Reference.Result)
	return commit
}

func cloneIdempotencyKeyRowStringMap(rows map[string]economy.IdempotencyKeyRow) map[string]economy.IdempotencyKeyRow {
	cloned := make(map[string]economy.IdempotencyKeyRow, len(rows))
	for key, row := range rows {
		cloned[key] = row.Clone()
	}
	return cloned
}

type failingMarketBuyInventory struct {
	delegate   *economy.InventoryService
	failReason economy.LedgerReason
	err        error
}

func (inventory failingMarketBuyInventory) SystemMoveItem(input economy.MoveItemInput) (economy.MoveItemResult, error) {
	if input.Reason == inventory.failReason {
		return economy.MoveItemResult{}, inventory.err
	}
	return inventory.delegate.SystemMoveItem(input)
}

func (inventory failingMarketBuyInventory) TotalItemQuantity(
	playerID foundation.PlayerID,
	itemID foundation.ItemID,
	location economy.ItemLocation,
) int64 {
	return inventory.delegate.TotalItemQuantity(playerID, itemID, location)
}

func (inventory failingMarketBuyInventory) SnapshotMutationState() economy.InventoryMutationSnapshot {
	return inventory.delegate.SnapshotMutationState()
}

func (inventory failingMarketBuyInventory) RestoreMutationState(snapshot economy.InventoryMutationSnapshot) {
	inventory.delegate.RestoreMutationState(snapshot)
}

type failingCompleteMarketEconomyStore struct {
	delegate  *memoryMarketEconomyStore
	operation string
	status    economy.IdempotencyStatus
	err       error
}

func (store failingCompleteMarketEconomyStore) ClaimIdempotencyKey(
	ctx context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyClaimResult, error) {
	return store.delegate.ClaimIdempotencyKey(ctx, row)
}

func (store failingCompleteMarketEconomyStore) CompleteIdempotencyKey(
	ctx context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, error) {
	if row.Operation == store.operation && row.Status == store.status {
		return economy.IdempotencyKeyRow{}, store.err
	}
	return store.delegate.CompleteIdempotencyKey(ctx, row)
}

type memoryMarketEconomyStore struct {
	mu          sync.Mutex
	idempotency map[string]economy.IdempotencyKeyRow
	outbox      map[string]economy.OutboxRow
}

func newMemoryMarketEconomyStore() *memoryMarketEconomyStore {
	return &memoryMarketEconomyStore{
		idempotency: make(map[string]economy.IdempotencyKeyRow),
		outbox:      make(map[string]economy.OutboxRow),
	}
}

func (store *memoryMarketEconomyStore) ClaimIdempotencyKey(
	ctx context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyClaimResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	key := memoryMarketIdempotencyKey(row.Scope, row.Key)
	if existing, ok := store.idempotency[key]; ok {
		return economy.ResolveIdempotencyClaim(&existing, row)
	}
	result, err := economy.ResolveIdempotencyClaim(nil, row)
	if err != nil {
		return economy.IdempotencyClaimResult{}, err
	}
	store.idempotency[key] = result.Row.Clone()
	return result, nil
}

func (store *memoryMarketEconomyStore) CompleteIdempotencyKey(
	ctx context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, error) {
	if err := row.Validate(); err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	key := memoryMarketIdempotencyKey(row.Scope, row.Key)
	store.idempotency[key] = row.Clone()
	return row.Clone(), nil
}

func (store *memoryMarketEconomyStore) InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error {
	inserted, err := economy.NewOutboxRow(row)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	if _, exists := store.outbox[inserted.OutboxID]; exists {
		return fmt.Errorf("outbox %q: %w", inserted.OutboxID, economy.ErrInvalidOutboxRow)
	}
	store.outbox[inserted.OutboxID] = inserted.Clone()
	return nil
}

func (store *memoryMarketEconomyStore) LoadOutboxRow(ctx context.Context, outboxID string) (economy.OutboxRow, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	row, ok := store.outbox[outboxID]
	return row.Clone(), ok, nil
}

func (store *memoryMarketEconomyStore) LoadDueOutboxRows(
	ctx context.Context,
	query economy.OutboxDueRowsQuery,
) ([]economy.OutboxRow, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	rows := make([]economy.OutboxRow, 0, query.Limit)
	for _, row := range store.outbox {
		if row.Status != economy.OutboxStatusPending && row.Status != economy.OutboxStatusFailed {
			continue
		}
		if row.AvailableAt.After(query.Now) {
			continue
		}
		rows = append(rows, row.Clone())
	}
	sort.Slice(rows, func(left int, right int) bool {
		if !rows[left].AvailableAt.Equal(rows[right].AvailableAt) {
			return rows[left].AvailableAt.Before(rows[right].AvailableAt)
		}
		if !rows[left].CreatedAt.Equal(rows[right].CreatedAt) {
			return rows[left].CreatedAt.Before(rows[right].CreatedAt)
		}
		return rows[left].OutboxID < rows[right].OutboxID
	})
	if len(rows) > query.Limit {
		rows = rows[:query.Limit]
	}
	return rows, nil
}

func (store *memoryMarketEconomyStore) LeaseOutboxRow(
	ctx context.Context,
	input economy.OutboxLeaseInput,
) (economy.OutboxRow, bool, error) {
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	row, ok := store.outbox[input.OutboxID]
	if !ok || !memoryMarketOutboxLeaseEligible(row, input.Now) {
		return economy.OutboxRow{}, false, nil
	}
	row.Status = economy.OutboxStatusLeased
	row.LeaseOwner = input.LeaseOwner
	row.LeasedUntil = input.LeasedUntil
	row.UpdatedAt = input.Now
	if err := row.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.outbox[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func (store *memoryMarketEconomyStore) MarkOutboxPublished(
	ctx context.Context,
	input economy.OutboxPublishInput,
) (economy.OutboxRow, bool, error) {
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	row, ok := store.outbox[input.OutboxID]
	if !ok || !memoryMarketOutboxLeaseMatches(row, input.LeaseOwner, input.Now) {
		return economy.OutboxRow{}, false, nil
	}
	row.Status = economy.OutboxStatusPublished
	row.LeaseOwner = ""
	row.LeasedUntil = time.Time{}
	row.AttemptCount++
	row.LastError = ""
	row.UpdatedAt = input.Now
	row.PublishedAt = input.Now
	if err := row.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.outbox[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func (store *memoryMarketEconomyStore) MarkOutboxFailed(
	ctx context.Context,
	input economy.OutboxFailureInput,
) (economy.OutboxRow, bool, error) {
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	row, ok := store.outbox[input.OutboxID]
	if !ok || !memoryMarketOutboxLeaseMatches(row, input.LeaseOwner, input.Now) {
		return economy.OutboxRow{}, false, nil
	}
	row.AttemptCount++
	if row.AttemptCount >= row.MaxAttempts {
		row.Status = economy.OutboxStatusDead
	} else {
		row.Status = economy.OutboxStatusFailed
	}
	row.AvailableAt = input.AvailableAt
	row.LeaseOwner = ""
	row.LeasedUntil = time.Time{}
	row.LastError = input.LastError
	row.UpdatedAt = input.Now
	if err := row.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.outbox[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func memoryMarketOutboxLeaseEligible(row economy.OutboxRow, now time.Time) bool {
	if (row.Status == economy.OutboxStatusPending || row.Status == economy.OutboxStatusFailed) && !row.AvailableAt.After(now) {
		return true
	}
	return row.Status == economy.OutboxStatusLeased && !row.LeasedUntil.IsZero() && !row.LeasedUntil.After(now)
}

func memoryMarketOutboxLeaseMatches(row economy.OutboxRow, owner string, now time.Time) bool {
	return row.Status == economy.OutboxStatusLeased &&
		row.LeaseOwner == owner &&
		!row.LeasedUntil.IsZero() &&
		row.LeasedUntil.After(now)
}

func memoryMarketIdempotencyKey(scope string, key foundation.IdempotencyKey) string {
	return scope + "\x00" + key.String()
}

func (fixture *marketFixture) createListingInput(listingID string, quantity int64, unitPrice int64) CreateListingInput {
	return CreateListingInput{
		ListingID:      foundation.ListingID(listingID),
		SellerPlayerID: fixture.sellerID,
		ItemRef:        economy.MoveItemRef{Definition: fixture.definition},
		SourceLocation: fixture.sourceLocation,
		Quantity:       quantity,
		UnitPrice:      unitPrice,
		Currency:       economy.CurrencyBucketCredits,
	}
}

func (fixture *marketFixture) createListing(t *testing.T, listingID string, quantity int64, unitPrice int64) CreateListingResult {
	t.Helper()

	result, err := fixture.service.CreateListing(fixture.createListingInput(listingID, quantity, unitPrice))
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	return result
}

func (fixture *marketFixture) seedSellerItems(t *testing.T, quantity int64, reference string) {
	t.Helper()

	_, err := fixture.inventory.AddItem(economy.AddItemInput{
		PlayerID:       fixture.sellerID,
		ItemDefinition: fixture.definition,
		Quantity:       quantity,
		Location:       fixture.sourceLocation,
		Reason:         "test_seed",
		ReferenceKey:   mustLootPickupKey(t, reference),
	})
	if err != nil {
		t.Fatalf("seed seller items: %v", err)
	}
}

func (fixture *marketFixture) seedCredits(t *testing.T, playerID foundation.PlayerID, amount int64, reference string) {
	t.Helper()

	_, err := fixture.wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       amount,
		Reason:       "test_seed",
		ReferenceKey: mustQuestRewardKey(t, reference),
	})
	if err != nil {
		t.Fatalf("seed credits: %v", err)
	}
}

func marketStackableDefinition(t *testing.T, itemID string, flags []economy.TradeFlag) economy.ItemDefinition {
	t.Helper()

	source, err := catalog.NewVersionedDefinitionFromStrings(itemID, "v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings: %v", err)
	}
	maxStack, err := foundation.NewQuantity(100)
	if err != nil {
		t.Fatalf("NewQuantity max stack: %v", err)
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity weight: %v", err)
	}
	definition, err := economy.NewItemDefinition(
		source,
		foundation.ItemID(itemID),
		"Test "+itemID,
		economy.ItemTypeStackable,
		economy.ItemRarityCommon,
		maxStack,
		weight,
		flags,
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition: %v", err)
	}
	return definition
}

func mustLocation(t *testing.T, kind economy.LocationKind, id string) economy.ItemLocation {
	t.Helper()

	location, err := economy.NewItemLocation(kind, id)
	if err != nil {
		t.Fatalf("NewItemLocation: %v", err)
	}
	return location
}

func mustLootPickupKey(t *testing.T, reference string) foundation.IdempotencyKey {
	t.Helper()

	key, err := foundation.LootPickupIdempotencyKey(reference)
	if err != nil {
		t.Fatalf("LootPickupIdempotencyKey: %v", err)
	}
	return key
}

func mustQuestRewardKey(t *testing.T, reference string) foundation.IdempotencyKey {
	t.Helper()

	key, err := foundation.QuestRewardIdempotencyKey(foundation.QuestID(reference))
	if err != nil {
		t.Fatalf("QuestRewardIdempotencyKey: %v", err)
	}
	return key
}
