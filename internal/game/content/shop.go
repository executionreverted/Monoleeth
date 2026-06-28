package content

import (
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/ships"
)

const (
	ShopCategoryShips            = "ships"
	ShopCategoryWeapons          = "weapons"
	ShopCategoryAmmo             = "ammo"
	ShopCategoryLaunchers        = "launchers"
	ShopCategoryShieldGenerators = "shield_generators"
	ShopCategorySpeedGenerators  = "speed_generators"
	ShopCategoryExtrasModules    = "extras_modules"
	ShopCategoryScannerRadar     = "scanner_radar"
	ShopCategoryStealth          = "stealth"
	ShopCategoryCargoUtility     = "cargo_utility"
	ShopCategoryBoosters         = "boosters"
	ShopCategoryResources        = "resources"
)

func DefaultShopContent(
	items map[foundation.ItemID]economy.ItemDefinition,
	moduleCatalog modules.Catalog,
	shipCatalog ships.Catalog,
) (catalog.ContentRegistry, error) {
	products := make([]catalog.ShopProductDefinition, 0, 16)
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
	if _, ok := items["laser_lens"]; ok {
		products = append(products, materialShopProduct("product_laser_lens", "laser_lens", "Laser Lens", "Focusing glass for entry weapon fabrication.", "item.laser_lens", 2, 160, 930))
	}
	if _, ok := items["energy_cell"]; ok {
		products = append(products, materialShopProduct("product_energy_cell", "energy_cell", "Energy Cell", "Starter power cell used in weapon and utility module recipes.", "item.energy_cell", 2, 130, 940))
	}
	if _, ok := items["scanner_circuit"]; ok {
		products = append(products, materialShopProduct("product_scanner_circuit", "scanner_circuit", "Scanner Circuit", "Low-tier circuit board for scout hull and scanner fabrication.", "item.scanner_circuit", 2, 180, 950))
	}
	if _, ok := items["warp_coil"]; ok {
		products = append(products, materialShopProduct("product_warp_coil", "warp_coil", "Warp Coil", "Compact navigation coil for early scout hull unlocks.", "item.warp_coil", 1, 220, 960))
	}
	registry, err := catalog.NewContentRegistry(catalog.ContentRegistryVersion, runtimeShopCategories(), products)
	if err != nil {
		return catalog.ContentRegistry{}, err
	}
	if err := registry.ValidateReferences(shopReferenceResolver(items, moduleCatalog, shipCatalog)); err != nil {
		return catalog.ContentRegistry{}, err
	}
	return registry, nil
}

func materialShopProduct(
	productID catalog.ShopProductID,
	itemID foundation.ItemID,
	displayName string,
	description string,
	artKey string,
	quantity int64,
	price int64,
	sortOrder int,
) catalog.ShopProductDefinition {
	return catalog.ShopProductDefinition{
		ProductID:   productID,
		ProductType: catalog.ShopProductTypeItem,
		Display: catalog.DisplayMetadata{
			DisplayName: displayName,
			Description: description,
			Category:    ShopCategoryResources,
			Subcategory: "Material",
			ArtKey:      artKey,
			Rarity:      economy.ItemRarityCommon.String(),
			Tier:        1,
			SortOrder:   sortOrder,
		},
		GrantTarget: catalog.GrantTarget{Kind: catalog.GrantTargetKindItem, RefID: itemID.String(), Quantity: quantity},
		Price:       catalog.PricePolicy{Currency: catalog.PriceCurrencyCredits, Amount: price, Fixed: true},
		Stock:       catalog.StockPolicy{Kind: catalog.StockPolicyUnlimited},
		Availability: catalog.AvailabilityRule{
			Available: true,
		},
	}
}

func shopReferenceResolver(
	items map[foundation.ItemID]economy.ItemDefinition,
	moduleCatalog modules.Catalog,
	shipCatalog ships.Catalog,
) catalog.ReferenceResolver {
	return catalog.ReferenceResolver{
		HasItem: func(id string) bool {
			_, ok := items[foundation.ItemID(id)]
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
	}
}

func DefaultShopCategories() []catalog.ContentCategory {
	return runtimeShopCategories()
}

func runtimeShopCategories() []catalog.ContentCategory {
	return []catalog.ContentCategory{
		{ID: ShopCategoryShips, DisplayName: "Ships", SortOrder: 10},
		{ID: ShopCategoryWeapons, DisplayName: "Weapons", SortOrder: 20},
		{ID: ShopCategoryAmmo, DisplayName: "Ammo", SortOrder: 30},
		{ID: ShopCategoryLaunchers, DisplayName: "Launchers", SortOrder: 40},
		{ID: ShopCategoryShieldGenerators, DisplayName: "Shield Generators", SortOrder: 50},
		{ID: ShopCategorySpeedGenerators, DisplayName: "Speed Generators", SortOrder: 60},
		{ID: ShopCategoryExtrasModules, DisplayName: "Extras/Modules", SortOrder: 70},
		{ID: ShopCategoryScannerRadar, DisplayName: "Scanner/Radar", SortOrder: 80},
		{ID: ShopCategoryStealth, DisplayName: "Stealth", SortOrder: 90},
		{ID: ShopCategoryCargoUtility, DisplayName: "Cargo/Utility", SortOrder: 100},
		{ID: ShopCategoryBoosters, DisplayName: "Boosters", SortOrder: 110},
		{ID: ShopCategoryResources, DisplayName: "Resources", SortOrder: 120},
	}
}

func runtimeShipShopProduct(definition ships.ShipDefinition) (catalog.ShopProductDefinition, bool) {
	if definition.ShipID == ships.ShipIDStarter {
		return catalog.ShopProductDefinition{}, false
	}
	metadata := map[foundation.ShipID]catalog.DisplayMetadata{
		ships.ShipIDFighterT1: {
			DisplayName: "Goliath K2",
			Description: "Combat chassis using the temporary legacy Goliath K2 balance.",
			Category:    ShopCategoryShips,
			Subcategory: "Fighter",
			ArtKey:      "ship.goliath_k2",
			Rarity:      economy.ItemRarityUncommon.String(),
			Tier:        definition.Tier,
			SortOrder:   100,
		},
		ships.ShipIDScoutT1: {
			DisplayName: "Vengeance",
			Description: "Fast scout hull using the temporary legacy Vengeance balance.",
			Category:    ShopCategoryShips,
			Subcategory: "Scout",
			ArtKey:      "ship.vengeance",
			Rarity:      economy.ItemRarityUncommon.String(),
			Tier:        definition.Tier,
			SortOrder:   110,
		},
		ships.ShipIDHaulerT1: {
			DisplayName: "Bigboy",
			Description: "Cargo-heavy hull using the temporary legacy Bigboy balance.",
			Category:    ShopCategoryShips,
			Subcategory: "Hauler",
			ArtKey:      "ship.bigboy",
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
			Available:    true,
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
			Available:    true,
			RequiredRank: definition.RequiredRank,
		},
	}, nil
}

func runtimeModuleShopDisplay(definition modules.ModuleDefinition) (catalog.DisplayMetadata, int64, bool) {
	switch definition.ItemID {
	case "laser_alpha_t1":
		return catalog.DisplayMetadata{
			DisplayName: "LF-1",
			Description: "Entry laser cannon using the legacy LF-1 damage baseline.",
			Category:    ShopCategoryWeapons,
			Subcategory: "Laser",
			ArtKey:      "module.lf_1",
			Rarity:      definition.Rarity.String(),
			Tier:        definition.Tier,
			SortOrder:   200,
		}, 10_000, true
	case "shield_generator_t1":
		return catalog.DisplayMetadata{
			DisplayName: "SG3N-A01",
			Description: "Starter shield generator using the legacy SG3N-A01 baseline.",
			Category:    ShopCategoryShieldGenerators,
			Subcategory: "Shield",
			ArtKey:      "module.sg3n_a01",
			Rarity:      definition.Rarity.String(),
			Tier:        definition.Tier,
			SortOrder:   500,
		}, 8_000, true
	case "scanner_t1":
		return catalog.DisplayMetadata{
			DisplayName: "Horizon Scanner",
			Description: "Utility scanner for planet discovery, hidden-signal sweeps, and playtest intel.",
			Category:    ShopCategoryScannerRadar,
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
			Category:    ShopCategoryScannerRadar,
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
			Category:    ShopCategoryCargoUtility,
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
