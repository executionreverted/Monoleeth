package server

import (
	"fmt"
	"time"

	"gameproject/internal/game/auction"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/market"
	"gameproject/internal/game/premium"
)

const (
	systemMarketSellerID foundation.PlayerID  = "system-market-seller"
	seedMarketListingID  foundation.ListingID = "listing-raw-ore-1"
	seedAuctionID        foundation.AuctionID = "auction-xcore-fragments-1"
)

const runtimeSeedLedgerReason economy.LedgerReason = "runtime_seed"

func (runtime *Runtime) seedSharedEconomy() error {
	if err := runtime.seedMarketFixture(); err != nil {
		return err
	}
	if err := runtime.seedAuctionFixture(); err != nil {
		return err
	}
	return runtime.seedPremiumStock()
}

func (runtime *Runtime) seedMarketFixture() error {
	rawOre, ok := runtime.itemCatalog["raw_ore"]
	if !ok {
		return fmt.Errorf("raw_ore definition missing")
	}
	sourceLocation, err := economy.NewItemLocation(economy.LocationKindAccountInventory, systemMarketSellerID.String())
	if err != nil {
		return err
	}
	seedRef, err := foundation.AdminCompensationIdempotencyKey(systemMarketSellerID.String(), "phase08-market-raw-ore")
	if err != nil {
		return err
	}
	if _, err := runtime.Inventory.AddItem(economy.AddItemInput{
		PlayerID:       systemMarketSellerID,
		ItemDefinition: rawOre,
		Quantity:       24,
		Location:       sourceLocation,
		Reason:         runtimeSeedLedgerReason,
		ReferenceKey:   seedRef,
	}); err != nil {
		return err
	}
	_, err = runtime.Market.CreateListing(market.CreateListingInput{
		ListingID:      seedMarketListingID,
		SellerPlayerID: systemMarketSellerID,
		ItemRef:        economy.MoveItemRef{Definition: rawOre},
		SourceLocation: sourceLocation,
		Quantity:       12,
		UnitPrice:      25,
		Currency:       economy.CurrencyBucketCredits,
	})
	return err
}

func (runtime *Runtime) seedAuctionFixture() error {
	source, err := catalog.NewVersionedDefinitionFromStrings("x_core_fragment_bundle", "v1")
	if err != nil {
		return err
	}
	buyNow := int64(650)
	now := runtime.clock.Now()
	_, err = runtime.Auction.CreateLot(auction.CreateLotInput{
		AuctionID: seedAuctionID,
		WorldID:   runtime.worldID,
		Payload: auction.LotPayload{
			Type:     auction.LotPayloadTypeXCoreFragmentBundle,
			Source:   source,
			Quantity: 2,
		},
		Currency:    economy.CurrencyBucketCredits,
		StartPrice:  250,
		BuyNowPrice: &buyNow,
		StartsAt:    now.Add(-time.Minute),
		EndsAt:      now.Add(24 * time.Hour),
	})
	return err
}

func (runtime *Runtime) seedPremiumStock() error {
	_, err := runtime.Premium.ConfigureWeeklyXCoreStock(premium.ConfigureWeeklyXCoreStockInput{
		WorldID:    runtime.worldID,
		PeriodKey:  runtime.currentPremiumPeriodKey(),
		StockTotal: weeklyXCoreStockTotal,
	})
	return err
}

func (runtime *Runtime) ensurePlayerEconomyLocked(playerID foundation.PlayerID) error {
	if err := runtime.seedStarterWallet(playerID); err != nil {
		return err
	}
	if err := runtime.seedPremiumEntitlement(playerID); err != nil {
		return err
	}
	state := runtime.players[playerID]
	state.Wallet = runtime.walletSnapshotLocked(playerID)
	runtime.players[playerID] = state
	return nil
}

func (runtime *Runtime) seedStarterWallet(playerID foundation.PlayerID) error {
	creditsRef, err := foundation.AdminCompensationIdempotencyKey(playerID.String(), "starter-credits")
	if err != nil {
		return err
	}
	if _, err := runtime.Wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       starterWalletCredits,
		Reason:       runtimeSeedLedgerReason,
		ReferenceKey: creditsRef,
	}); err != nil {
		return err
	}
	premiumRef, err := foundation.AdminCompensationIdempotencyKey(playerID.String(), "starter-premium-paid")
	if err != nil {
		return err
	}
	_, err = runtime.Wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketPremiumPaid,
		Amount:       starterWalletPremiumPaid,
		Reason:       runtimeSeedLedgerReason,
		ReferenceKey: premiumRef,
	})
	return err
}

func (runtime *Runtime) seedPremiumEntitlement(playerID foundation.PlayerID) error {
	now := runtime.clock.Now()
	_, err := runtime.Premium.CreateEntitlement(premium.CreateEntitlementInput{
		EntitlementID: premium.EntitlementID("entitlement-starter-premium-" + playerID.String()),
		PlayerID:      playerID,
		Type:          premium.EntitlementTypePremiumCurrencyPack,
		Provider: premium.ProviderReference{
			Source:    "dev_seed",
			Reference: "starter-premium-" + playerID.String(),
		},
		Payload: premium.EntitlementGrantPayload{
			CurrencyBucket: economy.CurrencyBucketPremiumEarned,
			Amount:         50,
		},
		CreatedAt:           now,
		ProviderConfirmedAt: now,
	})
	return err
}

func (runtime *Runtime) walletSnapshotLocked(playerID foundation.PlayerID) walletSnapshotPayload {
	return walletSnapshotPayload{
		Credits:       runtime.Wallet.Balance(playerID, economy.CurrencyBucketCredits),
		PremiumPaid:   runtime.Wallet.Balance(playerID, economy.CurrencyBucketPremiumPaid),
		PremiumEarned: runtime.Wallet.Balance(playerID, economy.CurrencyBucketPremiumEarned),
	}
}

func (runtime *Runtime) currentPremiumPeriodKey() string {
	year, week := runtime.clock.Now().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}
