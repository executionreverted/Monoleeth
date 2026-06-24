package contentdb

import (
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
)

type lootTableRowData struct {
	Source catalog.VersionedDefinition `json:"Source"`
	Rows   []lootRowData               `json:"Rows"`
}

type lootRowData struct {
	ItemDefinition lootItemReference `json:"ItemDefinition"`
	MinQuantity    int64             `json:"MinQuantity"`
	MaxQuantity    int64             `json:"MaxQuantity"`
	Chance         float64           `json:"Chance"`
}

type lootItemReference struct {
	ItemID foundation.ItemID `json:"item_id"`
}

func mapLootTableRows(
	snapshot content.Snapshot,
	items map[foundation.ItemID]economy.ItemDefinition,
) (map[string]loot.LootTable, error) {
	tables := make(map[string]loot.LootTable, len(snapshot.LootTables))
	version := publishedVersion(snapshot)
	for _, row := range snapshot.LootTables {
		if !row.Enabled {
			continue
		}
		var data lootTableRowData
		if err := decodeSnapshotRow(content.ContentTypeLootTable, row, &data); err != nil {
			return nil, err
		}
		tableID := data.Source.DefinitionID.String()
		if err := requireRowID(content.ContentTypeLootTable, row, tableID); err != nil {
			return nil, err
		}
		table := loot.LootTable{
			Source: forceSourceVersion(data.Source, version),
			Rows:   make([]loot.LootRow, 0, len(data.Rows)),
		}
		for index, lootRow := range data.Rows {
			item, ok := items[lootRow.ItemDefinition.ItemID]
			if !ok {
				return nil, fmt.Errorf("loot table %q row %d item %q: %w", tableID, index, lootRow.ItemDefinition.ItemID, content.ErrUnknownContentItem)
			}
			table.Rows = append(table.Rows, loot.LootRow{
				ItemDefinition: item,
				MinQuantity:    lootRow.MinQuantity,
				MaxQuantity:    lootRow.MaxQuantity,
				Chance:         lootRow.Chance,
			})
		}
		if _, exists := tables[tableID]; exists {
			return nil, fmt.Errorf("loot table %q: %w", tableID, content.ErrDuplicateContentID)
		}
		tables[tableID] = table
	}
	return tables, nil
}
