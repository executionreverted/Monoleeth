package contentseed

import (
	"encoding/json"
	"strings"
	"testing"

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

func TestBuildMVPSnapshotRuntimeRowsAvoidOriginalReferenceTerms(t *testing.T) {
	snapshot, err := BuildMVPSnapshot(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("BuildMVPSnapshot() error = %v, want nil", err)
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	text := strings.ToLower(string(raw))
	for _, forbidden := range forbiddenSnapshotReferenceTerms() {
		if strings.Contains(text, forbidden) {
			t.Fatalf("snapshot contains forbidden reference term %q", forbidden)
		}
	}
}

func forbiddenSnapshotReferenceTerms() []string {
	return []string{
		"darkorbit",
		"dark orbit",
		"streuner",
		"lordakia",
		"mordon",
		"saimon",
		"devolarium",
		"sibelon",
		"kristallon",
		"cubikon",
		"protegit",
		"phoenix",
		"yamato",
		"nostromo",
		"leonov",
		"piranha",
		"goliath",
		"vengeance",
		"bigboy",
		"citadel",
		"aegis",
		"iris",
		"flax",
		"lf-1",
		"lf-2",
		"lf-3",
		"lf-4",
		"mp-1",
		"bo-1",
		"bo-2",
		"g3n",
	}
}
