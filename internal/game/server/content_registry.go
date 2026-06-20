package server

import (
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/ships"
)

const (
	shopCategoryShips            = "ships"
	shopCategoryWeapons          = "weapons"
	shopCategoryAmmo             = "ammo"
	shopCategoryLaunchers        = "launchers"
	shopCategoryShieldGenerators = "shield_generators"
	shopCategorySpeedGenerators  = "speed_generators"
	shopCategoryExtrasModules    = "extras_modules"
	shopCategoryScannerRadar     = "scanner_radar"
	shopCategoryStealth          = "stealth"
	shopCategoryCargoUtility     = "cargo_utility"
	shopCategoryBoosters         = "boosters"
	shopCategoryResources        = "resources"
)

func buildRuntimeContentRegistry(
	itemCatalog map[foundation.ItemID]economy.ItemDefinition,
	moduleCatalog modules.Catalog,
	shipCatalog ships.Catalog,
) (catalog.ContentRegistry, error) {
	products := make([]catalog.ShopProductDefinition, 0, 10)
	for _, definition := range shipCatalog.All() {
		product, ok := runtimeShipShopProduct(definition)
		if ok {
			products = append(products, product)
		}
	}
	for _, definition := range moduleCatalog.Definitions() {
		product, err := runtimeModuleShopProduct(definition)
		if err != nil {
			return catalog.ContentRegistry{}, err
		}
		products = append(products, product)
	}
	if _, ok := itemCatalog["raw_ore"]; ok {
		products = append(products, catalog.ShopProductDefinition{
			ProductID:   "product_ferrite_ore",
			ProductType: catalog.ShopProductTypeItem,
			Display: catalog.DisplayMetadata{
				DisplayName: "Ferrite Ore",
				Description: "Dense starter ore used for ship repairs, starter crafting, and early trade loops.",
				Category:    shopCategoryResources,
				Subcategory: "Material",
				ArtKey:      "item.ferrite_ore",
				Rarity:      economy.ItemRarityCommon.String(),
				Tier:        1,
				SortOrder:   900,
			},
			GrantTarget: catalog.GrantTarget{Kind: catalog.GrantTargetKindItem, RefID: "raw_ore", Quantity: 10},
			Price:       catalog.PricePolicy{Currency: catalog.PriceCurrencyCredits, Amount: 40, Fixed: true},
			Stock:       catalog.StockPolicy{Kind: catalog.StockPolicyLimited, Remaining: 250, Total: 250},
			Availability: catalog.AvailabilityRule{
				Available:    false,
				LockedReason: "Purchase window unavailable in this playtest.",
			},
		})
	}
	registry, err := catalog.NewContentRegistry(catalog.ContentRegistryVersion, runtimeShopCategories(), products)
	if err != nil {
		return catalog.ContentRegistry{}, err
	}
	if err := registry.ValidateReferences(catalog.ReferenceResolver{
		HasItem: func(id string) bool {
			_, ok := itemCatalog[foundation.ItemID(id)]
			return ok
		},
		HasModule: func(id string) bool {
			_, ok := moduleCatalog.Lookup(foundation.ItemID(id))
			return ok
		},
		HasShip: func(id string) bool {
			_, ok := shipCatalog.Get(foundation.ShipID(id))
			return ok
		},
		HasPremium: func(id string) bool {
			return id == "weekly_xcore_stock"
		},
	}); err != nil {
		return catalog.ContentRegistry{}, err
	}
	return registry, nil
}

func runtimeShopCategories() []catalog.ContentCategory {
	return []catalog.ContentCategory{
		{ID: shopCategoryShips, DisplayName: "Ships", SortOrder: 10},
		{ID: shopCategoryWeapons, DisplayName: "Weapons", SortOrder: 20},
		{ID: shopCategoryAmmo, DisplayName: "Ammo", SortOrder: 30},
		{ID: shopCategoryLaunchers, DisplayName: "Launchers", SortOrder: 40},
		{ID: shopCategoryShieldGenerators, DisplayName: "Shield Generators", SortOrder: 50},
		{ID: shopCategorySpeedGenerators, DisplayName: "Speed Generators", SortOrder: 60},
		{ID: shopCategoryExtrasModules, DisplayName: "Extras/Modules", SortOrder: 70},
		{ID: shopCategoryScannerRadar, DisplayName: "Scanner/Radar", SortOrder: 80},
		{ID: shopCategoryStealth, DisplayName: "Stealth", SortOrder: 90},
		{ID: shopCategoryCargoUtility, DisplayName: "Cargo/Utility", SortOrder: 100},
		{ID: shopCategoryBoosters, DisplayName: "Boosters", SortOrder: 110},
		{ID: shopCategoryResources, DisplayName: "Resources", SortOrder: 120},
	}
}

func runtimeShipShopProduct(definition ships.ShipDefinition) (catalog.ShopProductDefinition, bool) {
	if definition.ShipID == ships.ShipIDStarter {
		return catalog.ShopProductDefinition{}, false
	}
	metadata := map[foundation.ShipID]catalog.DisplayMetadata{
		ships.ShipIDFighterT1: {
			DisplayName: "Helion Lance",
			Description: "Combat chassis with extra weapon slots and balanced shield reserves.",
			Category:    shopCategoryShips,
			Subcategory: "Fighter",
			ArtKey:      "ship.helion_lance",
			Rarity:      economy.ItemRarityUncommon.String(),
			Tier:        definition.Tier,
			SortOrder:   100,
		},
		ships.ShipIDScoutT1: {
			DisplayName: "Vesper Dart",
			Description: "Fast scout hull built for radar coverage, scanner duty, and quick escapes.",
			Category:    shopCategoryShips,
			Subcategory: "Scout",
			ArtKey:      "ship.vesper_dart",
			Rarity:      economy.ItemRarityUncommon.String(),
			Tier:        definition.Tier,
			SortOrder:   110,
		},
		ships.ShipIDHaulerT1: {
			DisplayName: "Aegis Courier",
			Description: "Cargo-heavy hull with stronger plating for resource runs and route staging.",
			Category:    shopCategoryShips,
			Subcategory: "Hauler",
			ArtKey:      "ship.aegis_courier",
			Rarity:      economy.ItemRarityUncommon.String(),
			Tier:        definition.Tier,
			SortOrder:   120,
		},
	}
	display, ok := metadata[definition.ShipID]
	if !ok {
		return catalog.ShopProductDefinition{}, false
	}
	price := definition.CreditPrice
	if price <= 0 {
		price = 1
	}
	return catalog.ShopProductDefinition{
		ProductID:   catalog.ShopProductID("product_ship_" + definition.ShipID.String()),
		ProductType: catalog.ShopProductTypeShip,
		Display:     display,
		GrantTarget: catalog.GrantTarget{Kind: catalog.GrantTargetKindShip, RefID: definition.ShipID.String(), Quantity: 1},
		Price:       catalog.PricePolicy{Currency: catalog.PriceCurrencyCredits, Amount: price, Fixed: true},
		Stock:       catalog.StockPolicy{Kind: catalog.StockPolicyUnlimited},
		Availability: catalog.AvailabilityRule{
			Available:    false,
			LockedReason: "Ship purchase unavailable in this playtest.",
			RequiredRank: definition.RankRequirement,
		},
	}, true
}

func runtimeModuleShopProduct(definition modules.ModuleDefinition) (catalog.ShopProductDefinition, error) {
	display, price, ok := runtimeModuleShopDisplay(definition)
	if !ok {
		return catalog.ShopProductDefinition{}, fmt.Errorf("module %q shop metadata missing", definition.ItemID)
	}
	return catalog.ShopProductDefinition{
		ProductID:   catalog.ShopProductID("product_module_" + definition.ItemID.String()),
		ProductType: catalog.ShopProductTypeModule,
		Display:     display,
		GrantTarget: catalog.GrantTarget{Kind: catalog.GrantTargetKindModule, RefID: definition.ItemID.String(), Quantity: 1},
		Price:       catalog.PricePolicy{Currency: catalog.PriceCurrencyCredits, Amount: price, Fixed: true},
		Stock:       catalog.StockPolicy{Kind: catalog.StockPolicyUnlimited},
		Availability: catalog.AvailabilityRule{
			Available:    false,
			LockedReason: "Module purchase unavailable in this playtest.",
			RequiredRank: definition.RequiredRank,
		},
	}, nil
}

func runtimeModuleShopDisplay(definition modules.ModuleDefinition) (catalog.DisplayMetadata, int64, bool) {
	switch definition.ItemID {
	case "laser_alpha_t1":
		return catalog.DisplayMetadata{
			DisplayName: "Prism Lance I",
			Description: "Entry laser array for reliable basic-fire pressure against small hostiles.",
			Category:    shopCategoryWeapons,
			Subcategory: "Laser",
			ArtKey:      "module.prism_lance_1",
			Rarity:      definition.Rarity.String(),
			Tier:        definition.Tier,
			SortOrder:   200,
		}, 450, true
	case "shield_generator_t1":
		return catalog.DisplayMetadata{
			DisplayName: "Aurora Shield Cell",
			Description: "Compact shield generator that increases maximum shield and passive recovery.",
			Category:    shopCategoryShieldGenerators,
			Subcategory: "Shield",
			ArtKey:      "module.aurora_shield_cell",
			Rarity:      definition.Rarity.String(),
			Tier:        definition.Tier,
			SortOrder:   500,
		}, 420, true
	case "scanner_t1":
		return catalog.DisplayMetadata{
			DisplayName: "Horizon Scanner",
			Description: "Utility scanner for planet discovery, hidden-signal sweeps, and playtest intel.",
			Category:    shopCategoryScannerRadar,
			Subcategory: "Scanner",
			ArtKey:      "module.horizon_scanner",
			Rarity:      definition.Rarity.String(),
			Tier:        definition.Tier,
			SortOrder:   800,
		}, 360, true
	case "radar_t1":
		return catalog.DisplayMetadata{
			DisplayName: "Warden Radar I",
			Description: "Radar extender that improves sector awareness and contact projection.",
			Category:    shopCategoryScannerRadar,
			Subcategory: "Radar",
			ArtKey:      "module.warden_radar_1",
			Rarity:      definition.Rarity.String(),
			Tier:        definition.Tier,
			SortOrder:   810,
		}, 380, true
	case "cargo_expander_t1":
		return catalog.DisplayMetadata{
			DisplayName: "Cargo Spine I",
			Description: "Utility frame that expands cargo capacity for longer salvage routes.",
			Category:    shopCategoryCargoUtility,
			Subcategory: "Cargo",
			ArtKey:      "module.cargo_spine_1",
			Rarity:      definition.Rarity.String(),
			Tier:        definition.Tier,
			SortOrder:   1000,
		}, 320, true
	default:
		return catalog.DisplayMetadata{}, 0, false
	}
}
