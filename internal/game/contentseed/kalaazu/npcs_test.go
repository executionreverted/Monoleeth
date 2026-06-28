package kalaazu

import (
	"encoding/json"
	"os"
	"testing"

	"gameproject/internal/game/content"
)

func TestBuildStarterNPCRowsMapsDensityAndStats(t *testing.T) {
	result, err := BuildStarterNPCRows(os.DirFS("."))
	if err != nil {
		t.Fatalf("BuildStarterNPCRows() error = %v, want nil", err)
	}
	if len(result.NPCTemplates) != 11 ||
		len(result.SpawnAreas) != 11 ||
		len(result.EnemyPools) != 11 ||
		len(result.NPCDropProfiles) != 11 ||
		len(result.NPCAggroProfiles) != 11 ||
		len(result.NPCLeashProfiles) != 11 {
		t.Fatalf("row counts = templates:%d areas:%d pools:%d drops:%d aggro:%d leash:%d, want 11 each",
			len(result.NPCTemplates),
			len(result.SpawnAreas),
			len(result.EnemyPools),
			len(result.NPCDropProfiles),
			len(result.NPCAggroProfiles),
			len(result.NPCLeashProfiles))
	}

	starterPool := decodeEnemyPoolForTest(t, result.EnemyPools[0])
	if starterPool.MapID != "map_1_1" || starterPool.MapMaxAlive != 80 || starterPool.PoolMaxAlive != 12 || starterPool.InitialAlive != 4 {
		t.Fatalf("starter pool = %+v, want Kalaazu amount 80 scaled to 12/4", starterPool)
	}
	mapThreePool := decodeEnemyPoolForTest(t, result.EnemyPools[5])
	if mapThreePool.MapID != "map_1_3" || mapThreePool.MapMaxAlive != 98 {
		t.Fatalf("map_1_3 pool = %+v, want shared map cap from total Kalaazu amount 98", mapThreePool)
	}

	lordakia := findTemplateForTest(t, result.NPCTemplates, "map_1_2", "lordakia")
	if lordakia.HPMax != 2000 || lordakia.ShieldMax != 2000 || lordakia.WeaponDamage != 90 || lordakia.Speed != 300 {
		t.Fatalf("lordakia template = %+v, want Kalaazu stats", lordakia)
	}
}

func TestBuildStarterNPCRowsMapsAggroFromAI(t *testing.T) {
	result, err := BuildStarterNPCRows(os.DirFS("."))
	if err != nil {
		t.Fatalf("BuildStarterNPCRows() error = %v, want nil", err)
	}

	var aggressive bool
	for _, row := range result.NPCAggroProfiles {
		var profile npcAggroProfileSnapshotData
		if err := json.Unmarshal(row.DataJSON, &profile); err != nil {
			t.Fatalf("aggro row %q json error = %v", row.ContentID, err)
		}
		if profile.MapID == "map_1_3" && profile.AggroRadius > 0 && profile.TargetMemory > 0 {
			aggressive = true
		}
	}
	if !aggressive {
		t.Fatalf("aggro rows = %+v, want map_1_3 aggressive NPC profile", result.NPCAggroProfiles)
	}
}

func decodeEnemyPoolForTest(t *testing.T, row content.SnapshotRow) enemyPoolSnapshotData {
	t.Helper()
	var decoded enemyPoolSnapshotData
	if err := json.Unmarshal(row.DataJSON, &decoded); err != nil {
		t.Fatalf("enemy pool json error = %v", err)
	}
	return decoded
}

func findTemplateForTest(t *testing.T, rows []content.SnapshotRow, mapID string, npcType string) npcTemplateSnapshotData {
	t.Helper()
	for _, row := range rows {
		var template npcTemplateSnapshotData
		if err := json.Unmarshal(row.DataJSON, &template); err != nil {
			t.Fatalf("template row %q json error = %v", row.ContentID, err)
		}
		if template.MapID.String() == mapID && template.NPCType == npcType {
			return template
		}
	}
	t.Fatalf("template %s/%s missing", mapID, npcType)
	return npcTemplateSnapshotData{}
}
