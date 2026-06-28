package contentseed

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/content"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestBuildMVPSnapshotUsesKalaazuStarterMapAndNPCRows(t *testing.T) {
	snapshot, err := BuildMVPSnapshot(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("BuildMVPSnapshot() error = %v, want nil", err)
	}

	if got, want := len(snapshot.Maps), 3; got != want {
		t.Fatalf("map rows = %d, want %d", got, want)
	}
	if got, want := len(snapshot.NPCTemplates), 11; got != want {
		t.Fatalf("npc template rows = %d, want %d", got, want)
	}
	if got, want := len(snapshot.EnemyPools), 11; got != want {
		t.Fatalf("enemy pool rows = %d, want %d", got, want)
	}
	if len(snapshot.MapPortals) == 0 {
		t.Fatal("map portal rows empty, want Kalaazu portal graph")
	}
	if !hasSeedRow(snapshot.Ships, "ship_goliath") {
		t.Fatal("ship_goliath row missing, want Kalaazu ship seed appended")
	}

	starter := requireSeedMapRow(t, snapshot.Maps, "map_1_1")
	if starter.PublicMapKey != "1-1" || starter.Bounds.MaxX != 20800 || starter.Bounds.MaxY != 12800 {
		t.Fatalf("starter map = %+v, want Kalaazu 1-1 bounds", starter)
	}

	streuner := requireSeedNPCTemplateRow(t, snapshot.NPCTemplates, "map_1_1", "streuner")
	if streuner.HPMax != 800 || streuner.ShieldMax != 400 || streuner.WeaponDamage != 15 {
		t.Fatalf("streuner template = %+v, want Kalaazu stats", streuner)
	}
	saimonPool := requireSeedEnemyPoolRow(t, snapshot.EnemyPools, "map_1_3", "saimon")
	if saimonPool.MapMaxAlive != 30 || saimonPool.PoolMaxAlive != 12 || saimonPool.InitialAlive != 4 {
		t.Fatalf("saimon pool = %+v, want Kalaazu amount 30 scaled to 12/4", saimonPool)
	}

	goliath := requireSeedShipRow(t, snapshot.Ships, "ship_goliath")
	if goliath.BaseStats.HP != 256000 || goliath.Slots.Offensive != 15 || goliath.Slots.Defensive != 15 {
		t.Fatalf("goliath = %+v, want Kalaazu ship stats", goliath)
	}
}

func requireSeedMapRow(t *testing.T, rows []content.SnapshotRow, contentID content.ContentID) seedMapRow {
	t.Helper()
	for _, row := range rows {
		if row.ContentID != contentID {
			continue
		}
		var decoded seedMapRow
		if err := json.Unmarshal(row.DataJSON, &decoded); err != nil {
			t.Fatalf("map row %q json error = %v", row.ContentID, err)
		}
		return decoded
	}
	t.Fatalf("map row %q missing", contentID)
	return seedMapRow{}
}

func requireSeedNPCTemplateRow(t *testing.T, rows []content.SnapshotRow, mapID worldmaps.MapID, npcType string) seedNPCTemplateRow {
	t.Helper()
	for _, row := range rows {
		var decoded seedNPCTemplateRow
		if err := json.Unmarshal(row.DataJSON, &decoded); err != nil {
			t.Fatalf("npc template row %q json error = %v", row.ContentID, err)
		}
		if decoded.MapID == mapID && decoded.NPCType == npcType {
			return decoded
		}
	}
	t.Fatalf("npc template %s/%s missing", mapID, npcType)
	return seedNPCTemplateRow{}
}

func requireSeedEnemyPoolRow(t *testing.T, rows []content.SnapshotRow, mapID worldmaps.MapID, npcType string) seedEnemyPoolRow {
	t.Helper()
	for _, row := range rows {
		var decoded seedEnemyPoolRow
		if err := json.Unmarshal(row.DataJSON, &decoded); err != nil {
			t.Fatalf("enemy pool row %q json error = %v", row.ContentID, err)
		}
		if decoded.MapID == mapID && decoded.NPCType == npcType {
			return decoded
		}
	}
	t.Fatalf("enemy pool %s/%s missing", mapID, npcType)
	return seedEnemyPoolRow{}
}

func requireSeedShipRow(t *testing.T, rows []content.SnapshotRow, contentID content.ContentID) ships.ShipDefinition {
	t.Helper()
	for _, row := range rows {
		if row.ContentID != contentID {
			continue
		}
		var definition ships.ShipDefinition
		if err := json.Unmarshal(row.DataJSON, &definition); err != nil {
			t.Fatalf("ship row %q json error = %v", row.ContentID, err)
		}
		return definition
	}
	t.Fatalf("ship row %q missing", contentID)
	return ships.ShipDefinition{}
}

func hasSeedRow(rows []content.SnapshotRow, contentID content.ContentID) bool {
	for _, row := range rows {
		if row.ContentID == contentID {
			return true
		}
	}
	return false
}

type seedMapRow struct {
	MapID        worldmaps.MapID        `json:"map_id"`
	PublicMapKey worldmaps.PublicMapKey `json:"public_map_key"`
	Bounds       worldmaps.Bounds       `json:"bounds"`
}

type seedNPCTemplateRow struct {
	MapID        worldmaps.MapID `json:"map_id"`
	NPCType      string          `json:"npc_type"`
	HPMax        float64         `json:"hp_max"`
	ShieldMax    float64         `json:"shield_max"`
	WeaponDamage float64         `json:"weapon_damage"`
}

type seedEnemyPoolRow struct {
	MapID        worldmaps.MapID `json:"map_id"`
	NPCType      string          `json:"npc_type"`
	MapMaxAlive  int             `json:"map_max_alive"`
	PoolMaxAlive int             `json:"pool_max_alive"`
	InitialAlive int             `json:"initial_alive"`
}
