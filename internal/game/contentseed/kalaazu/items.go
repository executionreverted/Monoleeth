package kalaazu

import (
	"fmt"
	"io/fs"
	"sort"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

const (
	kalaazuItemCatalogVersion catalog.Version = "kalaazu_item_seed_v1"
	defaultStackMax                           = 1_000_000
	defaultItemWeight                         = 1
)

func BuildStarterItemRows(filesystem fs.FS) ([]content.SnapshotRow, error) {
	itemRows, err := LoadDumpRows(filesystem, "testdata/items.sql")
	if err != nil {
		return nil, err
	}
	return mapStarterItemRows(itemRows)
}

func mapStarterItemRows(itemRows []DumpRow) ([]content.SnapshotRow, error) {
	sources := make([]kalaazuItemSource, 0, len(itemRows))
	for _, row := range itemRows {
		source, err := decodeKalaazuItem(row)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].KalaazuID < sources[j].KalaazuID })

	seen := make(map[foundation.ItemID]struct{}, len(sources))
	rows := make([]content.SnapshotRow, 0, len(sources))
	for _, source := range sources {
		itemID := uniqueItemID(source, seen)
		definition, err := itemDefinitionWithID(source, itemID)
		if err != nil {
			return nil, err
		}
		seen[definition.ItemID] = struct{}{}
		row, err := snapshotRow(definition.ItemID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func itemDefinition(source kalaazuItemSource) (economy.ItemDefinition, error) {
	return itemDefinitionWithID(source, foundation.ItemID(source.LootID))
}

func itemDefinitionWithID(source kalaazuItemSource, itemID foundation.ItemID) (economy.ItemDefinition, error) {
	sourceRow, err := catalog.NewVersionedDefinitionFromStrings(itemID.String(), kalaazuItemCatalogVersion.String())
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	maxStack := int64(defaultStackMax)
	itemType := economy.ItemTypeStackable
	if isKalaazuInstanceItem(source) {
		itemType = economy.ItemTypeInstance
		maxStack = 1
	}
	maxStackQuantity, err := foundation.NewQuantity(maxStack)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weightQuantity, err := foundation.NewQuantity(defaultItemWeight)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	definition, err := economy.NewItemDefinition(
		sourceRow,
		itemID,
		source.Name,
		itemType,
		itemRarity(source),
		maxStackQuantity,
		weightQuantity,
		itemTradeFlags(source),
		nil,
		nil,
	)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return definition, nil
}

func uniqueItemID(source kalaazuItemSource, seen map[foundation.ItemID]struct{}) foundation.ItemID {
	base := foundation.ItemID(source.LootID)
	if _, exists := seen[base]; !exists {
		return base
	}
	for suffix := 1; ; suffix++ {
		candidate := foundation.ItemID(fmt.Sprintf("%s_kalaazu_%d", base, source.KalaazuID))
		if suffix > 1 {
			candidate = foundation.ItemID(fmt.Sprintf("%s_kalaazu_%d_%d", base, source.KalaazuID, suffix))
		}
		if _, exists := seen[candidate]; !exists {
			return candidate
		}
	}
}

func isKalaazuInstanceItem(source kalaazuItemSource) bool {
	return source.Category == 1 || source.Type == 30
}

func itemRarity(source kalaazuItemSource) economy.ItemRarity {
	switch {
	case source.IsEvent:
		return economy.ItemRarityEvent
	case source.IsElite:
		return economy.ItemRarityRare
	case source.Price > 0:
		return economy.ItemRarityUncommon
	default:
		return economy.ItemRarityCommon
	}
}

func itemTradeFlags(source kalaazuItemSource) []economy.TradeFlag {
	if !source.IsBuyable {
		return nil
	}
	return []economy.TradeFlag{economy.TradeFlagTradeable}
}
