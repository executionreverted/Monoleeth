package content

import (
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/production"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const (
	TrainingDroneSalvageLootTableID = "training_drone_salvage"
	BorderRaiderSalvageLootTableID  = "border_raider_salvage"
	CoordinateScrollItemID          = foundation.ItemID("planet_coordinate_scroll")
)

// GameplayContent is the canonical static content bundle used by the playtest
// runtime. The same shape can later be loaded from a published DB/CMS revision.
type GameplayContent struct {
	Items      map[foundation.ItemID]economy.ItemDefinition
	LootTables map[string]loot.LootTable
	Modules    modules.Catalog
	Ships      ships.Catalog
	Recipes    crafting.RecipeCatalog
	Production production.Catalog
	Quests     quests.QuestCatalog
	Maps       *worldmaps.Catalog
	Scanner    ScannerContent
	Starter    StarterContent
	Shop       catalog.ContentRegistry
	Route      RouteContent
	Rules      ProductionRulesContent
	Combat     CombatRulesContent
}

// DefaultGameplayContent returns the current static playtest content bundle.
func DefaultGameplayContent(worldID world.WorldID) (GameplayContent, error) {
	moduleCatalog := modules.MustMVPCatalog()
	shipCatalog, err := ships.MVPShipCatalog()
	if err != nil {
		return GameplayContent{}, err
	}
	recipeCatalog, err := crafting.MVPRecipeCatalog()
	if err != nil {
		return GameplayContent{}, err
	}
	productionCatalog, err := production.MVPCatalog()
	if err != nil {
		return GameplayContent{}, err
	}
	questCatalog := quests.MustMVPQuestCatalog()
	mapCatalog, err := worldmaps.StarterCatalog(worldID)
	if err != nil {
		return GameplayContent{}, err
	}
	items, lootTables, err := defaultItemAndLootContent(moduleCatalog)
	if err != nil {
		return GameplayContent{}, err
	}
	shop, err := DefaultShopContent(items, moduleCatalog, shipCatalog)
	if err != nil {
		return GameplayContent{}, err
	}
	bundle := GameplayContent{
		Items:      items,
		LootTables: lootTables,
		Modules:    moduleCatalog,
		Ships:      shipCatalog,
		Recipes:    recipeCatalog,
		Production: productionCatalog,
		Quests:     questCatalog,
		Maps:       mapCatalog,
		Scanner:    DefaultScannerContent(),
		Starter:    DefaultStarterContent(),
		Shop:       shop,
		Route:      DefaultRouteContent(),
		Rules:      DefaultProductionRulesContent(),
		Combat:     DefaultCombatRulesContent(),
	}
	if err := bundle.Validate(); err != nil {
		return GameplayContent{}, err
	}
	return bundle, nil
}

func defaultItemAndLootContent(moduleCatalog modules.Catalog) (map[foundation.ItemID]economy.ItemDefinition, map[string]loot.LootTable, error) {
	rawOre, err := StackableItemDefinition("raw_ore", "Raw Ore")
	if err != nil {
		return nil, nil, err
	}
	xCore, err := XCoreDefinition()
	if err != nil {
		return nil, nil, err
	}
	coordinateScroll, err := CoordinateScrollDefinition()
	if err != nil {
		return nil, nil, err
	}
	carbonShards, err := StackableItemDefinition("carbon_shards", "Carbon Shards")
	if err != nil {
		return nil, nil, err
	}
	items := map[foundation.ItemID]economy.ItemDefinition{
		rawOre.ItemID:           rawOre,
		xCore.ItemID:            xCore,
		coordinateScroll.ItemID: coordinateScroll,
		carbonShards.ItemID:     carbonShards,
	}
	for _, itemID := range []foundation.ItemID{
		"iron_ore",
		"laser_lens",
		"energy_cell",
		"scanner_circuit",
		"refined_alloy",
		"warp_coil",
		"helium_dust",
	} {
		definition, err := StackableItemDefinition(itemID, itemID.String())
		if err != nil {
			return nil, nil, err
		}
		items[definition.ItemID] = definition
	}
	for _, module := range moduleCatalog.Definitions() {
		definition, err := ModuleItemDefinition(module)
		if err != nil {
			return nil, nil, err
		}
		items[definition.ItemID] = definition
	}

	trainingSource, err := catalog.NewLootTableSource(TrainingDroneSalvageLootTableID, "v1")
	if err != nil {
		return nil, nil, err
	}
	borderSource, err := catalog.NewLootTableSource(BorderRaiderSalvageLootTableID, "v1")
	if err != nil {
		return nil, nil, err
	}
	lootTables := map[string]loot.LootTable{
		TrainingDroneSalvageLootTableID: {
			Source: trainingSource,
			Rows: []loot.LootRow{{
				ItemDefinition: rawOre,
				MinQuantity:    3,
				MaxQuantity:    3,
				Chance:         1,
			}},
		},
		BorderRaiderSalvageLootTableID: {
			Source: borderSource,
			Rows: []loot.LootRow{{
				ItemDefinition: carbonShards,
				MinQuantity:    2,
				MaxQuantity:    2,
				Chance:         1,
			}},
		},
	}
	return items, lootTables, nil
}

func CoordinateScrollDefinition() (economy.ItemDefinition, error) {
	source, err := catalog.NewVersionedDefinitionFromStrings(CoordinateScrollItemID.String(), "v1")
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	maxStack, err := foundation.NewQuantity(1)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return economy.NewItemDefinition(
		source,
		CoordinateScrollItemID,
		"Planet Coordinate Scroll",
		economy.ItemTypeInstance,
		economy.ItemRarityRare,
		maxStack,
		weight,
		[]economy.TradeFlag{economy.TradeFlagTradeable, economy.TradeFlagMarketTradeable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
}

func XCoreDefinition() (economy.ItemDefinition, error) {
	source, err := catalog.NewVersionedDefinitionFromStrings("x_core", "v1")
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	maxStack, err := foundation.NewQuantity(99)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return economy.NewItemDefinition(
		source,
		"x_core",
		"X Core",
		economy.ItemTypeStackable,
		economy.ItemRarityRare,
		maxStack,
		weight,
		[]economy.TradeFlag{economy.TradeFlagTradeable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
}

func StackableItemDefinition(itemID foundation.ItemID, name string) (economy.ItemDefinition, error) {
	source, err := catalog.NewVersionedDefinitionFromStrings(itemID.String(), "v1")
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	maxStack, err := foundation.NewQuantity(999)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weight, err := foundation.NewQuantity(2)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return economy.NewItemDefinition(
		source,
		itemID,
		name,
		economy.ItemTypeStackable,
		economy.ItemRarityCommon,
		maxStack,
		weight,
		[]economy.TradeFlag{economy.TradeFlagDroppable, economy.TradeFlagMarketTradeable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
}

func ModuleItemDefinition(module modules.ModuleDefinition) (economy.ItemDefinition, error) {
	maxStack, err := foundation.NewQuantity(1)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weight, err := foundation.NewQuantity(6)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return economy.NewItemDefinition(
		module.Source,
		module.ItemID,
		module.Name,
		economy.ItemTypeInstance,
		module.Rarity,
		maxStack,
		weight,
		module.TradeFlags,
		module.BindRules,
		nil,
	)
}

func cloneItems(items map[foundation.ItemID]economy.ItemDefinition) map[foundation.ItemID]economy.ItemDefinition {
	cloned := make(map[foundation.ItemID]economy.ItemDefinition, len(items))
	for itemID, definition := range items {
		cloned[itemID] = definition
	}
	return cloned
}

func cloneLootTables(tables map[string]loot.LootTable) map[string]loot.LootTable {
	cloned := make(map[string]loot.LootTable, len(tables))
	for tableID, table := range tables {
		rows := append([]loot.LootRow(nil), table.Rows...)
		table.Rows = rows
		cloned[tableID] = table
	}
	return cloned
}

func (bundle GameplayContent) RuntimeItemsAndLootTables() (map[foundation.ItemID]economy.ItemDefinition, map[string]loot.LootTable, error) {
	if err := bundle.Validate(); err != nil {
		return nil, nil, err
	}
	if bundle.Maps == nil {
		return nil, nil, fmt.Errorf("maps: %w", ErrInvalidContentBundle)
	}
	return cloneItems(bundle.Items), cloneLootTables(bundle.LootTables), nil
}
