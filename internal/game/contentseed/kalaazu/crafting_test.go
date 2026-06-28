package kalaazu

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/crafting"
)

func TestBuildDefaultRowsMapsCraftRecipes(t *testing.T) {
	rows, err := BuildDefaultRows(DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildDefaultRows() error = %v, want nil", err)
	}
	if len(rows.CraftRecipeRows) != 3 {
		t.Fatalf("craft recipe rows = %d, want 3", len(rows.CraftRecipeRows))
	}

	refined := requireCraftRecipeForTest(t, rows.CraftRecipeRows, crafting.RecipeIDRefinedAlloy)
	if refined.Output.ItemID != "refined_alloy" ||
		refined.Output.Quantity != 5 ||
		!craftRecipeHasInput(refined, "iron_ore", 20) ||
		!craftRecipeHasInput(refined, "carbon_shards", 5) {
		t.Fatalf("refined alloy recipe = %+v, want default material recipe over Kalaazu-projected resources", refined)
	}

	laser := requireCraftRecipeForTest(t, rows.CraftRecipeRows, crafting.RecipeIDLaserAlphaT1)
	if laser.Output.ItemID != "laser_alpha_t1" ||
		laser.RequiredCredits != 650 ||
		laser.CraftDurationMS != 20*60*1000 ||
		!craftRecipeHasInput(laser, "refined_alloy", 18) ||
		!craftRecipeHasInput(laser, "laser_lens", 3) ||
		!craftRecipeHasInput(laser, "energy_cell", 2) {
		t.Fatalf("laser recipe = %+v, want Kalaazu/default starter-balance recipe", laser)
	}

	scout := requireCraftRecipeForTest(t, rows.CraftRecipeRows, crafting.RecipeIDScoutT1)
	if scout.Output.ShipID != "scout_t1" ||
		scout.Output.Kind != crafting.RecipeOutputKindShipUnlock ||
		scout.RequiredCredits != 2_200 ||
		scout.CraftDurationMS != 90*60*1000 ||
		!craftRecipeHasInput(scout, "refined_alloy", 80) ||
		!craftRecipeHasInput(scout, "scanner_circuit", 12) ||
		!craftRecipeHasInput(scout, "warp_coil", 4) {
		t.Fatalf("scout recipe = %+v, want Kalaazu/default scout unlock recipe", scout)
	}
}

func requireCraftRecipeForTest(t *testing.T, rows []content.SnapshotRow, recipeID catalog.DefinitionID) craftRecipeRowData {
	t.Helper()
	for _, row := range rows {
		if row.ContentID != content.ContentID(recipeID.String()) {
			continue
		}
		var recipe craftRecipeRowData
		if err := json.Unmarshal(row.DataJSON, &recipe); err != nil {
			t.Fatalf("recipe row %q json error = %v", row.ContentID, err)
		}
		if recipe.Source.DefinitionID != recipe.RecipeID {
			t.Fatalf("recipe row %q source id = %q, want recipe id", row.ContentID, recipe.Source.DefinitionID)
		}
		return recipe
	}
	t.Fatalf("recipe row %q missing", recipeID)
	return craftRecipeRowData{}
}

func craftRecipeHasInput(recipe craftRecipeRowData, itemID content.ContentID, quantity int64) bool {
	for _, input := range recipe.Inputs {
		if content.ContentID(input.ItemID.String()) == itemID && input.Quantity == quantity {
			return true
		}
	}
	return false
}
