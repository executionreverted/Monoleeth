package server

import (
	"errors"
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

const (
	runtimeSeedLedgerReason economy.LedgerReason = "runtime_seed"
)

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
		StockTotal: runtime.starterContent.WeeklyXCore.StockTotal,
	})
	return err
}

func (runtime *Runtime) ensurePlayerEconomyLocked(playerID foundation.PlayerID) error {
	if err := runtime.seedStarterWallet(playerID); err != nil {
		return fmt.Errorf("seed starter wallet: %w", err)
	}
	if err := runtime.seedPremiumEntitlement(playerID); err != nil {
		return fmt.Errorf("seed premium entitlement: %w", err)
	}
	if err := runtime.seedStarterModulesAndLoadout(playerID); err != nil {
		return fmt.Errorf("seed starter modules and loadout: %w", err)
	}
	if err := runtime.seedStarterAmmoInventory(playerID); err != nil {
		return fmt.Errorf("seed starter ammo inventory: %w", err)
	}
	if err := runtime.refreshPlayerCombatStatsPayloadLocked(playerID); err != nil {
		return fmt.Errorf("refresh player combat stats: %w", err)
	}
	if err := runtime.seedE2EPlanetClaimProof(playerID); err != nil {
		return fmt.Errorf("seed e2e planet claim proof: %w", err)
	}
	if err := runtime.seedE2ERouteProof(playerID); err != nil {
		return fmt.Errorf("seed e2e route proof: %w", err)
	}
	state := runtime.players[playerID]
	state.Wallet = runtime.walletSnapshotLocked(playerID)
	state.Cargo = runtime.cargoSnapshotFromInventoryLocked(playerID)
	runtime.players[playerID] = state
	return nil
}

func (runtime *Runtime) seedStarterModulesAndLoadout(playerID foundation.PlayerID) error {
	if err := runtime.LoadoutStore.SetActiveShip(playerID, runtime.starterContent.ShipID); err != nil {
		return err
	}
	current, err := runtime.LoadoutStore.EquippedModules(playerID, runtime.starterContent.ShipID)
	if err != nil {
		return err
	}
	if starterLoadoutHasRequiredSlots(current) {
		return nil
	}
	items, err := runtime.seedStarterModuleInventory(playerID)
	if err != nil {
		return err
	}
	if starterModuleItemsAlreadyEquipped(items) {
		return nil
	}
	equipped, changed, err := runtime.starterLoadoutEquippedModules(playerID, items, current)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return runtime.LoadoutStore.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID:  playerID,
		ShipID:    runtime.starterContent.ShipID,
		RequestID: foundation.RequestID("starter-loadout-" + playerID.String()),
		Equipped:  equipped,
	})
}

func starterLoadoutHasRequiredSlots(current []modules.EquippedModule) bool {
	hasOffensive := false
	hasUtility := false
	for _, equipped := range current {
		switch equipped.SlotID {
		case modules.ModuleSlotOffensive1:
			hasOffensive = true
		case modules.ModuleSlotUtility1:
			hasUtility = true
		}
	}
	return hasOffensive && hasUtility
}

func starterModuleItemsAlreadyEquipped(items map[foundation.ItemID]economy.InstanceItem) bool {
	for _, item := range items {
		if item.Location.Kind == economy.LocationKindShipEquipped {
			return true
		}
	}
	return false
}

func (runtime *Runtime) seedStarterAmmoInventory(playerID foundation.PlayerID) error {
	const starterAmmoQuantity int64 = 10000
	definition, ok := runtime.itemCatalog["ammunition_laser_lcb_10"]
	if !ok {
		return nil
	}
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		return err
	}
	if runtime.Inventory.TotalItemQuantity(playerID, definition.ItemID, location) > 0 {
		return nil
	}
	seedRef, err := foundation.AdminCompensationIdempotencyKey(playerID.String(), "starter-ammo-ammunition_laser_lcb_10")
	if err != nil {
		return err
	}
	_, err = runtime.Inventory.AddItem(economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: definition,
		Quantity:       starterAmmoQuantity,
		Location:       location,
		Reason:         runtimeSeedLedgerReason,
		ReferenceKey:   seedRef,
	})
	return err
}

func (runtime *Runtime) starterLoadoutEquippedModules(
	playerID foundation.PlayerID,
	items map[foundation.ItemID]economy.InstanceItem,
	current []modules.EquippedModule,
) ([]modules.EquippedModule, bool, error) {
	scanner, ok := items[runtime.starterContent.ScannerItemID]
	if !ok {
		return nil, false, fmt.Errorf("starter scanner item %q missing from granted modules", runtime.starterContent.ScannerItemID)
	}
	offensive, ok := runtime.firstStarterModuleItemForSlotType(items, modules.ModuleSlotTypeOffensive)
	if !ok {
		return nil, false, fmt.Errorf("starter offensive module missing from granted modules")
	}

	assignments := make(modules.SlotAssignments, len(current)+2)
	for _, equipped := range current {
		assignments[equipped.SlotID] = equipped.ItemInstanceID
	}

	changed := false
	if _, ok := assignments[modules.ModuleSlotOffensive1]; !ok {
		removeAssignedItem(assignments, offensive.ItemInstanceID)
		assignments[modules.ModuleSlotOffensive1] = offensive.ItemInstanceID
		changed = true
	}
	if _, ok := assignments[modules.ModuleSlotUtility1]; !ok {
		removeAssignedItem(assignments, scanner.ItemInstanceID)
		assignments[modules.ModuleSlotUtility1] = scanner.ItemInstanceID
		changed = true
	}
	if !changed {
		return nil, false, nil
	}
	return runtimeTargetEquippedModules(playerID, runtime.starterContent.ShipID, assignments, current, runtime.clock.Now()), true, nil
}

func removeAssignedItem(assignments modules.SlotAssignments, itemInstanceID foundation.ItemID) {
	for slotID, assignedItemID := range assignments {
		if assignedItemID == itemInstanceID {
			delete(assignments, slotID)
		}
	}
}

func (runtime *Runtime) firstStarterModuleItemForSlotType(items map[foundation.ItemID]economy.InstanceItem, slotType modules.ModuleSlotType) (economy.InstanceItem, bool) {
	for _, itemID := range runtime.starterContent.ModuleItemIDs {
		definition, ok := runtime.ModuleCatalog.Lookup(itemID)
		if !ok || definition.SlotType != slotType {
			continue
		}
		item, ok := items[itemID]
		return item, ok
	}
	return economy.InstanceItem{}, false
}

func (runtime *Runtime) seedStarterModuleInventory(playerID foundation.PlayerID) (map[foundation.ItemID]economy.InstanceItem, error) {
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		return nil, err
	}
	itemIDs := runtime.starterContent.ModuleItemIDs
	items := make(map[foundation.ItemID]economy.InstanceItem, len(itemIDs))
	for _, itemID := range itemIDs {
		definition, ok := runtime.itemCatalog[itemID]
		if !ok {
			return nil, fmt.Errorf("starter module item %q definition missing", itemID)
		}
		moduleDefinition, ok := runtime.ModuleCatalog.Lookup(itemID)
		if !ok {
			return nil, fmt.Errorf("starter module item %q: %w", itemID, modules.ErrUnknownModuleDefinition)
		}
		if existing, ok := runtime.existingStarterModuleItem(playerID, itemID, location); ok {
			if existing.Location.Kind == economy.LocationKindShipEquipped {
				items[itemID] = existing
				continue
			}
			item, err := runtime.Inventory.SystemSetInstanceDurability(playerID, existing.ItemInstanceID, moduleDefinition.Durability.Max)
			if err != nil {
				return nil, err
			}
			if err := runtime.LoadoutStore.PutModuleItem(item); err != nil {
				return nil, err
			}
			items[itemID] = item
			continue
		}
		seedRef, err := foundation.AdminCompensationIdempotencyKey(playerID.String(), "starter-module-"+itemID.String())
		if err != nil {
			return nil, err
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
			return nil, err
		}
		if len(result.InstanceItems) != 1 {
			return nil, fmt.Errorf("starter module item %q grant returned %d instances", itemID, len(result.InstanceItems))
		}
		instanceID := result.InstanceItems[0].ItemInstanceID
		item, err := runtime.Inventory.SystemSetInstanceDurability(playerID, instanceID, moduleDefinition.Durability.Max)
		if err != nil {
			return nil, err
		}
		if err := runtime.LoadoutStore.PutModuleItem(item); err != nil {
			return nil, err
		}
		items[itemID] = item
	}
	return items, nil
}

func (runtime *Runtime) existingStarterModuleItem(playerID foundation.PlayerID, itemID foundation.ItemID, location economy.ItemLocation) (economy.InstanceItem, bool) {
	for _, item := range runtime.Inventory.InstanceItems() {
		if item.OwnerPlayerID == playerID && item.ItemID == itemID && (item.Location == location || item.Location.Kind == economy.LocationKindShipEquipped) {
			return item, true
		}
	}
	return economy.InstanceItem{}, false
}

func (runtime *Runtime) seedStarterWallet(playerID foundation.PlayerID) error {
	creditsRef, err := foundation.AdminCompensationIdempotencyKey(playerID.String(), "starter-credits")
	if err != nil {
		return err
	}
	if current := runtime.Wallet.Balance(playerID, economy.CurrencyBucketCredits); current < runtime.starterContent.WalletCredits {
		if _, err := runtime.Wallet.CreditWallet(economy.CreditWalletInput{
			PlayerID:     playerID,
			Currency:     economy.CurrencyBucketCredits,
			Amount:       runtime.starterContent.WalletCredits - current,
			Reason:       runtimeSeedLedgerReason,
			ReferenceKey: creditsRef,
		}); err != nil {
			return err
		}
	}
	premiumRef, err := foundation.AdminCompensationIdempotencyKey(playerID.String(), "starter-premium-paid")
	if err != nil {
		return err
	}
	if current := runtime.Wallet.Balance(playerID, economy.CurrencyBucketPremiumPaid); current < runtime.starterContent.WalletPremiumPaid {
		_, err = runtime.Wallet.CreditWallet(economy.CreditWalletInput{
			PlayerID:     playerID,
			Currency:     economy.CurrencyBucketPremiumPaid,
			Amount:       runtime.starterContent.WalletPremiumPaid - current,
			Reason:       runtimeSeedLedgerReason,
			ReferenceKey: premiumRef,
		})
		return err
	}
	return nil
}

func (runtime *Runtime) seedPremiumEntitlement(playerID foundation.PlayerID) error {
	entitlementID := premium.EntitlementID("entitlement-starter-premium-" + playerID.String())
	for _, entitlement := range runtime.Premium.Entitlements() {
		if entitlement.ID == entitlementID && entitlement.PlayerID == playerID {
			return nil
		}
	}
	now := runtime.clock.Now()
	_, err := runtime.Premium.CreateEntitlement(premium.CreateEntitlementInput{
		EntitlementID: entitlementID,
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
	if errors.Is(err, economy.ErrIdempotencyKeyConflict) {
		return nil
	}
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
