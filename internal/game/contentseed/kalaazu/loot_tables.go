package kalaazu

import (
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
)

const (
	trainingLootTableID = "training_drone_salvage"
	borderLootTableID   = "border_raider_salvage"
)

func mapStarterLootTableRows() ([]content.SnapshotRow, error) {
	rows := []struct {
		tableID string
		items   []content.LootRowSnapshotData
	}{
		{
			tableID: trainingLootTableID,
			items: []content.LootRowSnapshotData{
				lootRow("resource_ore_prometium", 20, 20, 1),
				lootRow("resource_ore_terbium", 20, 20, 1),
				lootRow("resource_ore_endurium", 20, 20, 1),
			},
		},
		{
			tableID: borderLootTableID,
			items: []content.LootRowSnapshotData{
				lootRow("resource_ore_prometium", 40, 40, 1),
				lootRow("resource_ore_terbium", 40, 40, 1),
				lootRow("resource_ore_endurium", 40, 40, 1),
				lootRow("resource_ore_prometid", 2, 2, 1),
				lootRow("resource_ore_duranium", 2, 2, 1),
				lootRow("resource_ore_xenomit", 1, 1, 0.25),
			},
		},
	}
	out := make([]content.SnapshotRow, 0, len(rows))
	for _, row := range rows {
		source, err := catalog.NewLootTableSource(row.tableID, "kalaazu_default_seed_v1")
		if err != nil {
			return nil, err
		}
		data := content.LootTableSnapshotData{
			Source: source,
			Rows:   row.items,
		}
		snapshotRow, err := snapshotRow(row.tableID, data)
		if err != nil {
			return nil, err
		}
		out = append(out, snapshotRow)
	}
	return out, nil
}

func lootRow(itemID foundation.ItemID, minQuantity int64, maxQuantity int64, chance float64) content.LootRowSnapshotData {
	return content.LootRowSnapshotData{
		ItemDefinition: content.LootItemSnapshotReference{ItemID: itemID},
		MinQuantity:    minQuantity,
		MaxQuantity:    maxQuantity,
		Chance:         chance,
	}
}
