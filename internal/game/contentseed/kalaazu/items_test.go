package kalaazu

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/content"
	"gameproject/internal/game/economy"
)

func TestBuildStarterItemRowsMapsKalaazuItems(t *testing.T) {
	dumpRows, err := LoadDumpRows(DefaultSeedFS(), "testdata/items.sql")
	if err != nil {
		t.Fatalf("LoadDumpRows(items) error = %v, want nil", err)
	}
	rows, err := BuildStarterItemRows(DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildStarterItemRows() error = %v, want nil", err)
	}
	if got, want := len(rows), len(dumpRows); got != want {
		t.Fatalf("item rows = %d, want dump row count %d", got, want)
	}

	phoenix := requireItemDefinitionForTest(t, rows, "ship_phoenix")
	if phoenix.Name != "Phoenix" || phoenix.Type != economy.ItemTypeInstance || phoenix.MaxStack.Int64() != 1 {
		t.Fatalf("phoenix item = %+v, want instance ship item", phoenix)
	}

	ammo := requireItemDefinitionForTest(t, rows, "ammunition_laser_lcb_10")
	if ammo.Name != "LCB-10" || ammo.Type != economy.ItemTypeStackable || ammo.MaxStack.Int64() != defaultStackMax {
		t.Fatalf("lcb ammo item = %+v, want stackable ammo", ammo)
	}
}

func requireItemDefinitionForTest(t *testing.T, rows []content.SnapshotRow, contentID content.ContentID) economy.ItemDefinition {
	t.Helper()
	for _, row := range rows {
		if row.ContentID != contentID {
			continue
		}
		var definition economy.ItemDefinition
		if err := json.Unmarshal(row.DataJSON, &definition); err != nil {
			t.Fatalf("item row %q json error = %v", row.ContentID, err)
		}
		if err := definition.Validate(); err != nil {
			t.Fatalf("item row %q Validate() error = %v", row.ContentID, err)
		}
		return definition
	}
	t.Fatalf("item row %q missing", contentID)
	return economy.ItemDefinition{}
}
