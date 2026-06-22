package server

import (
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/modules"
)

const (
	trainingDroneSalvageLootTableID = "training_drone_salvage"
	borderRaiderSalvageLootTableID  = "border_raider_salvage"
)

func runtimeLootCatalog() (map[string]loot.LootTable, map[foundation.ItemID]economy.ItemDefinition, error) {
	rawOre, err := runtimeRawOreDefinition()
	if err != nil {
		return nil, nil, err
	}
	trainingSource, err := catalog.NewLootTableSource(trainingDroneSalvageLootTableID, "v1")
	if err != nil {
		return nil, nil, err
	}
	trainingTable := loot.LootTable{
		Source: trainingSource,
		Rows: []loot.LootRow{{
			ItemDefinition: rawOre,
			MinQuantity:    3,
			MaxQuantity:    3,
			Chance:         1,
		}},
	}
	itemCatalog := map[foundation.ItemID]economy.ItemDefinition{
		rawOre.ItemID: rawOre,
	}
	xCore, err := runtimeXCoreDefinition()
	if err != nil {
		return nil, nil, err
	}
	itemCatalog[xCore.ItemID] = xCore
	carbonShards, err := runtimeStackableDefinition("carbon_shards", "carbon_shards")
	if err != nil {
		return nil, nil, err
	}
	itemCatalog[carbonShards.ItemID] = carbonShards
	for _, itemID := range []foundation.ItemID{
		"iron_ore",
		"energy_cell",
		"scanner_circuit",
		"refined_alloy",
		"helium_dust",
	} {
		definition, err := runtimeStackableDefinition(itemID, itemID.String())
		if err != nil {
			return nil, nil, err
		}
		itemCatalog[definition.ItemID] = definition
	}
	borderSource, err := catalog.NewLootTableSource(borderRaiderSalvageLootTableID, "v1")
	if err != nil {
		return nil, nil, err
	}
	borderTable := loot.LootTable{
		Source: borderSource,
		Rows: []loot.LootRow{{
			ItemDefinition: carbonShards,
			MinQuantity:    2,
			MaxQuantity:    2,
			Chance:         1,
		}},
	}
	return map[string]loot.LootTable{
		trainingDroneSalvageLootTableID: trainingTable,
		borderRaiderSalvageLootTableID:  borderTable,
	}, itemCatalog, nil
}

func runtimeRawOreDefinition() (economy.ItemDefinition, error) {
	return runtimeStackableDefinition("raw_ore", "Raw Ore")
}

func runtimeXCoreDefinition() (economy.ItemDefinition, error) {
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

func runtimeStackableDefinition(itemID foundation.ItemID, name string) (economy.ItemDefinition, error) {
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

func appendRuntimeModuleItems(itemCatalog map[foundation.ItemID]economy.ItemDefinition, moduleCatalog modules.Catalog) error {
	for _, module := range moduleCatalog.Definitions() {
		definition, err := runtimeModuleItemDefinition(module)
		if err != nil {
			return err
		}
		itemCatalog[definition.ItemID] = definition
	}
	return nil
}

func runtimeModuleItemDefinition(module modules.ModuleDefinition) (economy.ItemDefinition, error) {
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
