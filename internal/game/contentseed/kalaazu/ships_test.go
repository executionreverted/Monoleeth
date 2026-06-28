package kalaazu

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/content"
	"gameproject/internal/game/ships"
)

func TestBuildStarterShipRowsMapsKalaazuShipsAndStats(t *testing.T) {
	rows, err := BuildStarterShipRows(DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildStarterShipRows() error = %v, want nil", err)
	}
	if got, want := len(rows), 13; got != want {
		t.Fatalf("ship rows = %d, want %d", got, want)
	}

	phoenix := requireShipDefinitionForTest(t, rows, "ship_phoenix")
	if phoenix.Name != "Phoenix" ||
		phoenix.BaseStats.HP != 4000 ||
		phoenix.BaseStats.Speed != 320 ||
		phoenix.BaseStats.CargoCapacity != 100 ||
		phoenix.Slots != (ships.SlotLayout{Offensive: 1, Defensive: 1, Utility: 1}) {
		t.Fatalf("phoenix = %+v, want Kalaazu starter ship stats", phoenix)
	}

	goliath := requireShipDefinitionForTest(t, rows, "ship_goliath")
	if goliath.BaseStats.HP != 256000 ||
		goliath.BaseStats.Speed != 300 ||
		goliath.BaseStats.CargoCapacity != 1000 ||
		goliath.Slots.Offensive != 15 ||
		goliath.Slots.Defensive != 15 ||
		goliath.PremiumPrice != 80000 ||
		goliath.CreditPrice != 0 {
		t.Fatalf("goliath = %+v, want Kalaazu stats and elite price", goliath)
	}
}

func requireShipDefinitionForTest(t *testing.T, rows []content.SnapshotRow, contentID content.ContentID) ships.ShipDefinition {
	t.Helper()
	for _, row := range rows {
		if row.ContentID != contentID {
			continue
		}
		var definition ships.ShipDefinition
		if err := json.Unmarshal(row.DataJSON, &definition); err != nil {
			t.Fatalf("ship row %q json error = %v", row.ContentID, err)
		}
		if err := definition.Validate(); err != nil {
			t.Fatalf("ship row %q Validate() error = %v", row.ContentID, err)
		}
		return definition
	}
	t.Fatalf("ship row %q missing", contentID)
	return ships.ShipDefinition{}
}
