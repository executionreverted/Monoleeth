package contentseed

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/content"
	"gameproject/internal/game/world"
)

func TestSeedCraftRecipeRowsEmitCraftDurationMSOnly(t *testing.T) {
	snapshot, err := BuildMVPSnapshot(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("BuildMVPSnapshot() error = %v, want nil", err)
	}
	if len(snapshot.CraftRecipes) == 0 {
		t.Fatal("seed snapshot has no craft recipe rows")
	}
	for _, row := range snapshot.CraftRecipes {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(row.DataJSON, &fields); err != nil {
			t.Fatalf("unmarshal craft row %q: %v", row.ContentID, err)
		}
		if _, ok := fields["craft_duration"]; ok {
			t.Fatalf("craft row %q emitted raw craft_duration: %s", row.ContentID, row.DataJSON)
		}
		rawDuration, ok := fields["craft_duration_ms"]
		if !ok {
			t.Fatalf("craft row %q missing craft_duration_ms: %s", row.ContentID, row.DataJSON)
		}
		var durationMS int64
		if err := json.Unmarshal(rawDuration, &durationMS); err != nil {
			t.Fatalf("craft row %q craft_duration_ms decode: %v", row.ContentID, err)
		}
		if durationMS <= 0 {
			t.Fatalf("craft row %q craft_duration_ms = %d, want positive", row.ContentID, durationMS)
		}
	}
}

func TestBuildMVPSnapshotRuntimeRowsCarryLegacyBalanceTerms(t *testing.T) {
	snapshot, err := BuildMVPSnapshot(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("BuildMVPSnapshot() error = %v, want nil", err)
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	text := strings.ToLower(string(raw))
	for _, expected := range []string{
		"content_kalaazu_starter_seed_v1",
		"phoenix",
		"lf-1",
		"sg3n-a01",
		"prometium",
		"terbium",
		"endurium",
		"prometid",
		"duranium",
		"promerium",
		"xenomit",
		"lordakia",
		"saimon",
		"streuner",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("snapshot missing expected legacy balance term %q", expected)
		}
	}
}

func TestBuildMVPSnapshotIncludesServerRuleRows(t *testing.T) {
	snapshot, err := BuildMVPSnapshot(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("BuildMVPSnapshot() error = %v, want nil", err)
	}
	requireSingleSeedRow(t, "scanner config", snapshot.ScannerConfigs, "scanner_config")
	requireSingleSeedRow(t, "starter config", snapshot.StarterConfigs, "starter_config")
	requireSingleSeedRow(t, "route policy", snapshot.RoutePolicies, "route_policy")
	requireSingleSeedRow(t, "production rules", snapshot.ProductionRules, "production_rules")
	requireSingleSeedRow(t, "combat rules", snapshot.CombatRules, "combat_rules")
}

func requireSingleSeedRow(t *testing.T, label string, rows []content.SnapshotRow, contentID string) {
	t.Helper()
	if len(rows) != 1 {
		t.Fatalf("%s rows = %d, want 1", label, len(rows))
	}
	if string(rows[0].ContentID) != contentID {
		t.Fatalf("%s content_id = %q, want %q", label, rows[0].ContentID, contentID)
	}
	if len(rows[0].DataJSON) == 0 {
		t.Fatalf("%s data_json empty", label)
	}
}
