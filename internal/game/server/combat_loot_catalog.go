package server

import (
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/modules"
)

func runtimeLootCatalog() (loot.LootTable, map[foundation.ItemID]economy.ItemDefinition, error) {
	rawOre, err := runtimeRawOreDefinition()
	if err != nil {
		return loot.LootTable{}, nil, err
	}
	source, err := catalog.NewLootTableSource("training_drone_salvage", "v1")
	if err != nil {
		return loot.LootTable{}, nil, err
	}
	table := loot.LootTable{
		Source: source,
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
	for _, itemID := range []foundation.ItemID{
		"iron_ore",
		"carbon_shards",
		"energy_cell",
		"scanner_circuit",
		"refined_alloy",
		"helium_dust",
	} {
		definition, err := runtimeStackableDefinition(itemID, itemID.String())
		if err != nil {
			return loot.LootTable{}, nil, err
		}
		itemCatalog[definition.ItemID] = definition
	}
	return table, itemCatalog, nil
}

func runtimeRawOreDefinition() (economy.ItemDefinition, error) {
	return runtimeStackableDefinition("raw_ore", "Raw Ore")
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
