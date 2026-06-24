package contentdb

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"gameproject/internal/game/content"
	"gameproject/internal/game/crafting"
)

func TestMapCraftRecipeRowsDecodesCraftDurationMS(t *testing.T) {
	snapshot := craftRecipeTestSnapshot(t, `"craft_duration_ms":45000`)

	catalogRows, err := mapCraftRecipeRows(snapshot)
	if err != nil {
		t.Fatalf("mapCraftRecipeRows() error = %v, want nil", err)
	}
	recipe, ok := catalogRows.Get("recipe_test")
	if !ok {
		t.Fatal("recipe_test missing from mapped catalog")
	}
	if got, want := recipe.CraftDuration, 45*time.Second; got != want {
		t.Fatalf("craft duration = %s, want %s", got, want)
	}
}

func TestMapCraftRecipeRowsRejectsInvalidCraftDurationMS(t *testing.T) {
	tests := []struct {
		name       string
		durationMS int64
	}{
		{name: "zero", durationMS: 0},
		{name: "negative", durationMS: -1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := craftRecipeTestSnapshot(t, `"craft_duration_ms":`+int64String(test.durationMS))

			_, err := mapCraftRecipeRows(snapshot)
			if !errors.Is(err, crafting.ErrInvalidCraftDuration) {
				t.Fatalf("mapCraftRecipeRows() error = %v, want %v", err, crafting.ErrInvalidCraftDuration)
			}
		})
	}
}

func TestMapCraftRecipeRowsAcceptsLegacyCraftDurationFallback(t *testing.T) {
	snapshot := craftRecipeTestSnapshot(t, `"craft_duration":45000000000`)

	catalogRows, err := mapCraftRecipeRows(snapshot)
	if err != nil {
		t.Fatalf("mapCraftRecipeRows() legacy error = %v, want nil", err)
	}
	recipe, ok := catalogRows.Get("recipe_test")
	if !ok {
		t.Fatal("recipe_test missing from mapped catalog")
	}
	if got, want := recipe.CraftDuration, 45*time.Second; got != want {
		t.Fatalf("legacy craft duration = %s, want %s", got, want)
	}
}

func craftRecipeTestSnapshot(t *testing.T, durationField string) content.Snapshot {
	t.Helper()
	return content.Snapshot{
		Version: "content_test_v1",
		CraftRecipes: []content.SnapshotRow{{
			ContentID: "recipe_test",
			Enabled:   true,
			DataJSON: []byte(`{
				"source":{"definition_id":"recipe_test","catalog_version":"recipe_seed_v1"},
				"recipe_id":"recipe_test",
				"category":"processed_material",
				"output":{"kind":"item","item_id":"refined_alloy","quantity":1,"tradeable":true},
				"inputs":[{"item_id":"iron_ore","quantity":2}],
				"required_credits":1,
				"required_rank":1,
				"required_location_type":"station",
				` + durationField + `,
				"repeatable":true
			}`),
		}},
	}
}

func int64String(value int64) string {
	return strconv.FormatInt(value, 10)
}
