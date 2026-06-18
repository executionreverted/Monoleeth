package market

import (
	"errors"
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
	sellerID       foundation.PlayerID
	buyerID        foundation.PlayerID
	otherBuyerID   foundation.PlayerID
	sourceLocation economy.ItemLocation
	buyerLocation  economy.ItemLocation
	definition     economy.ItemDefinition
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

func TestCreateListingRejectsDuplicateListingIDWithoutMutation(t *testing.T) {
	fixture := newMarketFixture(t)
	fixture.seedSellerItems(t, 20, "seed-duplicate")
	first := fixture.createListing(t, "listing-duplicate", 5, 20)

	_, err := fixture.service.CreateListing(fixture.createListingInput("listing-duplicate", 5, 20))
	if !errors.Is(err, ErrDuplicateListingID) {
		t.Fatalf("duplicate CreateListing error = %v, want ErrDuplicateListingID", err)
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
	fixture.service.suspiciousPolicy = SuspiciousTradePolicy{HighValueSaleThreshold: 100}
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
	})
	if err != nil {
		t.Fatalf("CancelListing: %v", err)
	}
	itemLedgerAfterFirst := len(fixture.inventory.ItemLedgerEntries())
	second, err := fixture.service.CancelListing(CancelListingInput{
		SellerPlayerID: fixture.sellerID,
		ListingID:      create.Listing.ListingID,
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
	t.Helper()

	clock := testutil.NewFakeClock(marketTestNow)
	inventory := economy.NewInventoryService(clock)
	wallet := economy.NewWalletService(clock)
	service, err := NewMarketService(MarketServiceConfig{
		Clock:     clock,
		Inventory: inventory,
		Wallet:    wallet,
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
		sellerID:       sellerID,
		buyerID:        buyerID,
		otherBuyerID:   "buyer-2",
		sourceLocation: sourceLocation,
		buyerLocation:  buyerLocation,
		definition:     marketStackableDefinition(t, "raw-ore", []economy.TradeFlag{economy.TradeFlagMarketTradeable}),
	}
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
