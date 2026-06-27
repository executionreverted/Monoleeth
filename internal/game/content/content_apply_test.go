package content

import (
	"encoding/json"
	"testing"
)

func applyTestRow(contentID string, enabled bool, dataJSON string, display ...json.RawMessage) SnapshotRow {
	row := SnapshotRow{
		ContentID: ContentID(contentID),
		Enabled:   enabled,
		DataJSON:  json.RawMessage(dataJSON),
	}
	if len(display) > 0 {
		row.DisplayJSON = display[0]
	}
	return row
}

func TestPlanRuntimeApplyClassifiesIdenticalSnapshotAsSafeReload(t *testing.T) {
	base := Snapshot{Version: "v1", Items: []SnapshotRow{applyTestRow("raw_ore", true, `{"stackable":true}`)}}

	plan := PlanRuntimeApply(base, base)

	if plan.Class != ApplyClassSafeReload {
		t.Fatalf("class = %q, want %q", plan.Class, ApplyClassSafeReload)
	}
	if len(plan.ChangedContentTypes) != 0 {
		t.Fatalf("changed types = %v, want empty", plan.ChangedContentTypes)
	}
}

func TestPlanRuntimeApplyClassifiesItemDisplayChangeAsRestartRequired(t *testing.T) {
	base := Snapshot{Version: "v1", Items: []SnapshotRow{applyTestRow("raw_ore", true, `{"stackable":true}`)}}
	next := Snapshot{
		Version: "v2",
		Items:   []SnapshotRow{applyTestRow("raw_ore", true, `{"stackable":true}`, displayJSON("Auric Ore"))},
	}

	plan := PlanRuntimeApply(base, next)

	if plan.Class != ApplyClassRestartRequired {
		t.Fatalf("class = %q, want %q (item catalog is boot-wired)", plan.Class, ApplyClassRestartRequired)
	}
	if !containsContentType(plan.ChangedContentTypes, ContentTypeItem) {
		t.Fatalf("changed types = %v, want item", plan.ChangedContentTypes)
	}
}

func TestPlanRuntimeApplyClassifiesShopProductChangeAsRestartRequired(t *testing.T) {
	base := Snapshot{Version: "v1", ShopProducts: []SnapshotRow{applyTestRow("product_ferrite_ore", true, `{"price":10}`)}}
	next := Snapshot{
		Version:      "v2",
		ShopProducts: []SnapshotRow{applyTestRow("product_ferrite_ore", true, `{"price":12}`)},
	}

	plan := PlanRuntimeApply(base, next)

	if plan.Class != ApplyClassRestartRequired {
		t.Fatalf("class = %q, want %q (shop product catalog is boot-wired)", plan.Class, ApplyClassRestartRequired)
	}
}

func TestPlanRuntimeApplyClassifiesCraftRecipeChangeAsRestartRequired(t *testing.T) {
	base := Snapshot{Version: "v1", CraftRecipes: []SnapshotRow{applyTestRow("recipe_laser", true, `{"output":"laser_alpha_t1"}`)}}
	next := Snapshot{
		Version:      "v2",
		CraftRecipes: []SnapshotRow{applyTestRow("recipe_laser", true, `{"output":"laser_alpha_t2"}`)},
	}

	plan := PlanRuntimeApply(base, next)

	if plan.Class != ApplyClassRestartRequired {
		t.Fatalf("class = %q, want %q (craft recipe is boot-wired)", plan.Class, ApplyClassRestartRequired)
	}
	if !containsContentType(plan.ChangedContentTypes, ContentTypeCraftRecipe) {
		t.Fatalf("changed types = %v, want craft_recipe", plan.ChangedContentTypes)
	}
}

func TestPlanRuntimeApplyClassifiesShipChangeAsRestartRequired(t *testing.T) {
	base := Snapshot{Version: "v1", Ships: []SnapshotRow{applyTestRow("starter", true, `{"slots":4}`)}}
	next := Snapshot{
		Version: "v2",
		Ships:   []SnapshotRow{applyTestRow("starter", true, `{"slots":6}`)},
	}

	plan := PlanRuntimeApply(base, next)

	if plan.Class != ApplyClassRestartRequired {
		t.Fatalf("class = %q, want %q (ship catalog is boot-wired)", plan.Class, ApplyClassRestartRequired)
	}
}

func TestPlanRuntimeApplyClassifiesMixedSafeAndRestartChangeAsRestartRequired(t *testing.T) {
	base := Snapshot{
		Version: "v1",
		Items:   []SnapshotRow{applyTestRow("raw_ore", true, `{"stackable":true}`)},
		Ships:   []SnapshotRow{applyTestRow("starter", true, `{"slots":4}`)},
	}
	next := Snapshot{
		Version: "v2",
		Items:   []SnapshotRow{applyTestRow("raw_ore", true, `{"stackable":true}`, displayJSON("Renamed Ore"))},
		Ships:   []SnapshotRow{applyTestRow("starter", true, `{"slots":6}`)},
	}

	plan := PlanRuntimeApply(base, next)

	if plan.Class != ApplyClassRestartRequired {
		t.Fatalf("class = %q, want %q (ship change forces restart even with safe item change)", plan.Class, ApplyClassRestartRequired)
	}
}

func displayJSON(name string) json.RawMessage {
	return json.RawMessage(`{"display_name":"` + name + `"}`)
}

func containsContentType(types []ContentType, want ContentType) bool {
	for _, got := range types {
		if got == want {
			return true
		}
	}
	return false
}
