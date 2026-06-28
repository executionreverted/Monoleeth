package kalaazu

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/content"
)

func TestBuildDefaultRowsMapsKalaazuLootTables(t *testing.T) {
	rows, err := BuildDefaultRows(DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildDefaultRows() error = %v, want nil", err)
	}
	if got, want := len(rows.LootTableRows), 2; got != want {
		t.Fatalf("loot table rows = %d, want %d", got, want)
	}
	if got, want := rows.Report.ImportedRows[content.ContentTypeLootTable], 2; got != want {
		t.Fatalf("loot table import report rows = %d, want %d", got, want)
	}

	itemIDs := make(map[string]struct{}, len(rows.ItemRows))
	for _, row := range rows.ItemRows {
		itemIDs[string(row.ContentID)] = struct{}{}
	}

	training := requireLootTableForTest(t, rows.LootTableRows, "training_drone_salvage")
	if got, want := training.Source.Version.String(), "kalaazu_default_seed_v1"; got != want {
		t.Fatalf("training loot source version = %q, want %q", got, want)
	}
	if got, want := len(training.Rows), 3; got != want {
		t.Fatalf("training loot row count = %d, want %d", got, want)
	}

	border := requireLootTableForTest(t, rows.LootTableRows, "border_raider_salvage")
	if got, want := len(border.Rows), 6; got != want {
		t.Fatalf("border loot row count = %d, want %d", got, want)
	}
	for _, table := range []content.LootTableSnapshotData{training, border} {
		for _, lootRow := range table.Rows {
			itemID := lootRow.ItemDefinition.ItemID.String()
			if _, ok := itemIDs[itemID]; !ok {
				t.Fatalf("loot table %s references missing Kalaazu item %q", table.Source.DefinitionID, itemID)
			}
		}
	}
}

func requireLootTableForTest(t *testing.T, rows []content.SnapshotRow, contentID content.ContentID) content.LootTableSnapshotData {
	t.Helper()
	for _, row := range rows {
		if row.ContentID != contentID {
			continue
		}
		var table content.LootTableSnapshotData
		if err := json.Unmarshal(row.DataJSON, &table); err != nil {
			t.Fatalf("loot table row %q json error = %v", row.ContentID, err)
		}
		return table
	}
	t.Fatalf("loot table row %q missing", contentID)
	return content.LootTableSnapshotData{}
}
