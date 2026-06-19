package server

import (
	"fmt"
	"time"

	"gameproject/internal/game/auction"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/market"
	"gameproject/internal/game/modules"
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
	if err := runtime.seedStarterModulesAndLoadout(playerID); err != nil {
		return err
	}
	state := runtime.players[playerID]
	state.Wallet = runtime.walletSnapshotLocked(playerID)
	state.Cargo = runtime.cargoSnapshotFromInventoryLocked(playerID)
	runtime.players[playerID] = state
	return nil
}

func (runtime *Runtime) seedStarterModulesAndLoadout(playerID foundation.PlayerID) error {
	if err := runtime.LoadoutStore.SetActiveShip(playerID, starterShipID); err != nil {
		return err
	}
	items, scannerCreated, err := runtime.seedStarterModuleInventory(playerID)
	if err != nil {
		return err
	}
	current, err := runtime.LoadoutStore.EquippedModules(playerID, starterShipID)
	if err != nil {
		return err
	}
	scanner := items[foundation.ItemID(starterScannerItemID)]
	if scannerCreated && len(current) == 0 {
		return runtime.LoadoutStore.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
			PlayerID:  playerID,
			ShipID:    starterShipID,
			RequestID: foundation.RequestID("starter-loadout-" + playerID.String()),
			Equipped: []modules.EquippedModule{{
				PlayerID:       playerID,
				ShipID:         starterShipID,
				SlotID:         modules.ModuleSlotUtility1,
				ItemInstanceID: scanner.ItemInstanceID,
				EquippedAt:     runtime.clock.Now(),
			}},
		})
	}
	return nil
}

func (runtime *Runtime) seedStarterModuleInventory(playerID foundation.PlayerID) (map[foundation.ItemID]economy.InstanceItem, bool, error) {
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		return nil, false, err
	}
	itemIDs := []foundation.ItemID{
		"scanner_t1",
		"laser_alpha_t1",
		"shield_generator_t1",
	}
	items := make(map[foundation.ItemID]economy.InstanceItem, len(itemIDs))
	scannerCreated := false
	for _, itemID := range itemIDs {
		definition, ok := runtime.itemCatalog[itemID]
		if !ok {
			return nil, false, fmt.Errorf("starter module item %q definition missing", itemID)
		}
		moduleDefinition, ok := runtime.ModuleCatalog.Lookup(itemID)
		if !ok {
			return nil, false, fmt.Errorf("starter module item %q: %w", itemID, modules.ErrUnknownModuleDefinition)
		}
		seedRef, err := foundation.AdminCompensationIdempotencyKey(playerID.String(), "starter-module-"+itemID.String())
		if err != nil {
			return nil, false, err
		}
		result, err := runtime.Inventory.AddItem(economy.AddItemInput{
			PlayerID:       playerID,
			ItemDefinition: definition,
			Quantity:       1,
			Location:       location,
			Reason:         runtimeSeedLedgerReason,
			ReferenceKey:   seedRef,
		})
		if err != nil {
			return nil, false, err
		}
		if len(result.InstanceItems) != 1 {
			return nil, false, fmt.Errorf("starter module item %q grant returned %d instances", itemID, len(result.InstanceItems))
		}
		instanceID := result.InstanceItems[0].ItemInstanceID
		item, err := runtime.Inventory.SystemSetInstanceDurability(playerID, instanceID, moduleDefinition.Durability.Max)
		if err != nil {
			return nil, false, err
		}
		if err := runtime.LoadoutStore.PutModuleItem(item); err != nil {
			return nil, false, err
		}
		items[itemID] = item
		if itemID == foundation.ItemID(starterScannerItemID) && !result.Duplicate {
			scannerCreated = true
		}
	}
	return items, scannerCreated, nil
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
