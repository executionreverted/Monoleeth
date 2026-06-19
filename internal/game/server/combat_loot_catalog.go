package server

import (
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
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
	return table, map[foundation.ItemID]economy.ItemDefinition{
		rawOre.ItemID: rawOre,
	}, nil
}

func runtimeRawOreDefinition() (economy.ItemDefinition, error) {
	source, err := catalog.NewVersionedDefinitionFromStrings("raw_ore", "v1")
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
		"raw_ore",
		"Raw Ore",
		economy.ItemTypeStackable,
		economy.ItemRarityCommon,
		maxStack,
		weight,
		[]economy.TradeFlag{economy.TradeFlagDroppable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
}
