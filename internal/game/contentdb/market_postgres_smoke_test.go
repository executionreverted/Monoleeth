package contentdb_test

import (
	"context"
	"testing"
	"time"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/market"
)

func TestPostgresMarketStorePersistsListingEscrowAcrossStoreReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	sellerID := foundation.PlayerID("player-postgres-market-seller")
	seedPostgresWalletPlayer(t, ctx, store, sellerID)
	marketStore, err := contentdb.NewMarketListingStore(store)
	if err != nil {
		t.Fatalf("NewMarketListingStore() error = %v, want nil", err)
	}
	listingID := foundation.ListingID("listing-postgres-market-escrow")
	expiresAt := time.Date(2026, 6, 25, 21, 0, 0, 0, time.UTC)
	listing := market.Listing{
		ListingID:         listingID,
		SellerPlayerID:    sellerID,
		ItemDefinition:    postgresStackableDefinitionForTest(t),
		OriginalQuantity:  12,
		RemainingQuantity: 7,
		UnitPrice:         42,
		Currency:          economy.CurrencyBucketCredits,
		Status:            market.ListingStatusActive,
		SourceReturnLocation: economy.ItemLocation{
			Kind: economy.LocationKindAccountInventory,
			ID:   economy.LocationID("account"),
		},
		EscrowLocation: economy.ItemLocation{
			Kind: economy.LocationKindMarketEscrow,
			ID:   economy.LocationID(listingID.String()),
		},
		CreatedAt: time.Date(2026, 6, 25, 20, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 25, 20, 1, 0, 0, time.UTC),
		ExpiresAt: &expiresAt,
	}
	listing.ItemID = listing.ItemDefinition.ItemID
	if err := marketStore.UpsertMarketListing(ctx, listing); err != nil {
		t.Fatalf("UpsertMarketListing() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewMarketListingStore(store)
	if err != nil {
		t.Fatalf("NewMarketListingStore(reopen) error = %v, want nil", err)
	}
	loaded, ok, err := reopened.LoadMarketListing(ctx, listingID)
	if err != nil {
		t.Fatalf("LoadMarketListing() error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("LoadMarketListing(%q) ok = false, want true", listingID)
	}
	if loaded.ListingID != listing.ListingID ||
		loaded.Status != market.ListingStatusActive ||
		loaded.RemainingQuantity != listing.RemainingQuantity ||
		loaded.SourceReturnLocation != listing.SourceReturnLocation ||
		loaded.EscrowLocation != listing.EscrowLocation ||
		loaded.ItemDefinition.ItemID != listing.ItemDefinition.ItemID {
		t.Fatalf("loaded listing = %+v, want persisted status/quantity/escrow/source/item from %+v", loaded, listing)
	}
}

func TestPostgresMarketStoreTransactionLocksListingForUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	sellerID := foundation.PlayerID("player-postgres-market-lock-seller")
	seedPostgresWalletPlayer(t, ctx, store, sellerID)
	marketStore, err := contentdb.NewMarketStore(store)
	if err != nil {
		t.Fatalf("NewMarketStore() error = %v, want nil", err)
	}
	listingID := foundation.ListingID("listing-postgres-market-lock")
	listing := market.Listing{
		ListingID:         listingID,
		SellerPlayerID:    sellerID,
		ItemDefinition:    postgresStackableDefinitionForTest(t),
		OriginalQuantity:  9,
		RemainingQuantity: 9,
		UnitPrice:         25,
		Currency:          economy.CurrencyBucketCredits,
		Status:            market.ListingStatusActive,
		SourceReturnLocation: economy.ItemLocation{
			Kind: economy.LocationKindAccountInventory,
			ID:   economy.LocationID("account"),
		},
		EscrowLocation: economy.ItemLocation{
			Kind: economy.LocationKindMarketEscrow,
			ID:   economy.LocationID(listingID.String()),
		},
		CreatedAt: time.Date(2026, 6, 25, 21, 10, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 25, 21, 10, 0, 0, time.UTC),
	}
	listing.ItemID = listing.ItemDefinition.ItemID
	if err := marketStore.SaveMarketListing(ctx, listing); err != nil {
		t.Fatalf("SaveMarketListing() error = %v, want nil", err)
	}

	if err := marketStore.WithTransaction(ctx, func(tx *contentdb.MarketListingTx) error {
		locked, ok, err := tx.LoadMarketListingForUpdate(ctx, listingID)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatalf("LoadMarketListingForUpdate(%q) ok = false, want true", listingID)
		}
		locked.RemainingQuantity = 4
		locked.UpdatedAt = time.Date(2026, 6, 25, 21, 11, 0, 0, time.UTC)
		return tx.UpsertMarketListing(ctx, locked)
	}); err != nil {
		t.Fatalf("WithTransaction(LoadMarketListingForUpdate) error = %v, want nil", err)
	}

	loaded, ok, err := marketStore.LoadMarketListing(ctx, listingID)
	if err != nil {
		t.Fatalf("LoadMarketListing(after tx) error = %v, want nil", err)
	}
	if !ok || loaded.RemainingQuantity != 4 {
		t.Fatalf("listing after locked transaction = %+v ok %v, want remaining quantity 4", loaded, ok)
	}
}
