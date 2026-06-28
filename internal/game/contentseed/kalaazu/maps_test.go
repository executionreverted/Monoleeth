package kalaazu

import (
	"encoding/json"
	"os"
	"testing"

	"gameproject/internal/game/content"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestBuildStarterMapRowsMapsKalaazuStarterSectors(t *testing.T) {
	result, err := BuildStarterMapRows(os.DirFS("."))
	if err != nil {
		t.Fatalf("BuildStarterMapRows() error = %v, want nil", err)
	}
	if len(result.MapRows) != 3 {
		t.Fatalf("map rows = %d, want 3", len(result.MapRows))
	}
	if len(result.PortalRows) != 4 {
		t.Fatalf("portal rows = %d, want 4", len(result.PortalRows))
	}

	first := decodeMapRowForTest(t, result.MapRows[0])
	if first.MapID != worldmaps.StarterMapID || first.PublicMapKey != "1-1" {
		t.Fatalf("first map = %s/%s, want %s/1-1", first.MapID, first.PublicMapKey, worldmaps.StarterMapID)
	}
	if first.Bounds.MaxX != 20800 || first.Bounds.MaxY != 12800 {
		t.Fatalf("first bounds = %+v, want Kalaazu 20800x12800", first.Bounds)
	}
	if first.PVPPolicy != "safe" || first.RiskBand != "low" {
		t.Fatalf("first risk/pvp = %s/%s, want low/safe", first.RiskBand, first.PVPPolicy)
	}
	if !hasSpawn(first.SpawnPoints, worldmaps.StarterSpawnID) {
		t.Fatalf("first spawns = %+v, want starter spawn", first.SpawnPoints)
	}

	third := decodeMapRowForTest(t, result.MapRows[2])
	if third.MapID != "map_1_3" || third.PublicMapKey != "1-3" {
		t.Fatalf("third map = %s/%s, want map_1_3/1-3", third.MapID, third.PublicMapKey)
	}
	if third.RiskBand != "medium" || third.PVPPolicy != "pve" {
		t.Fatalf("third risk/pvp = %s/%s, want medium/pve from source flags", third.RiskBand, third.PVPPolicy)
	}
}

func TestBuildStarterMapRowsMapsPortalDestinationsServerSide(t *testing.T) {
	result, err := BuildStarterMapRows(os.DirFS("."))
	if err != nil {
		t.Fatalf("BuildStarterMapRows() error = %v, want nil", err)
	}

	var found bool
	for _, row := range result.PortalRows {
		var portal mapPortalSnapshotData
		if err := json.Unmarshal(row.DataJSON, &portal); err != nil {
			t.Fatalf("portal row %q json error = %v", row.ContentID, err)
		}
		if portal.SourceMapID == worldmaps.StarterMapID && portal.DestinationMapID == "map_1_2" {
			found = true
			if portal.SourcePosition.X != 18500 || portal.SourcePosition.Y != 11500 {
				t.Fatalf("source position = %+v, want Kalaazu gate position", portal.SourcePosition)
			}
			if portal.DestinationSpawnID == "" || portal.InteractionRadius <= 0 || !portal.Visible {
				t.Fatalf("portal = %+v, want destination spawn, radius, visible", portal)
			}
		}
	}
	if !found {
		t.Fatalf("portal rows = %+v, want 1-1 -> 1-2 portal", result.PortalRows)
	}
}

func decodeMapRowForTest(t *testing.T, row content.SnapshotRow) mapDefinitionSnapshotData {
	t.Helper()
	var decoded mapDefinitionSnapshotData
	if err := json.Unmarshal(row.DataJSON, &decoded); err != nil {
		t.Fatalf("map row json error = %v", err)
	}
	return decoded
}

func hasSpawn(spawns []spawnPointSnapshotData, spawnID worldmaps.SpawnID) bool {
	for _, spawn := range spawns {
		if spawn.SpawnID == spawnID {
			return true
		}
	}
	return false
}
