package kalaazu

import (
	"fmt"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

const kalaazuRecipeCatalogVersion catalog.Version = "kalaazu_recipe_seed_v1"

type craftRecipeRowData struct {
	Source               catalog.VersionedDefinition `json:"source"`
	RecipeID             catalog.DefinitionID        `json:"recipe_id"`
	Category             crafting.RecipeCategory     `json:"category"`
	Output               craftRecipeOutputRowData    `json:"output"`
	Inputs               []craftRecipeInputRowData   `json:"inputs"`
	RequiredCredits      int64                       `json:"required_credits"`
	RequiredRank         int                         `json:"required_rank"`
	RequiredRoleLevels   []crafting.RoleRequirement  `json:"required_role_levels,omitempty"`
	RequiredLocationType crafting.CraftLocationType  `json:"required_location_type"`
	CraftDurationMS      int64                       `json:"craft_duration_ms"`
	Repeatable           bool                        `json:"repeatable"`
}

type craftRecipeInputRowData struct {
	ItemID   foundation.ItemID `json:"item_id"`
	Quantity int64             `json:"quantity"`
}

type craftRecipeOutputRowData struct {
	Kind      crafting.RecipeOutputKind `json:"kind"`
	ItemID    foundation.ItemID         `json:"item_id,omitempty"`
	ShipID    foundation.ShipID         `json:"ship_id,omitempty"`
	Quantity  int64                     `json:"quantity"`
	Tradeable bool                      `json:"tradeable"`
}

func mapCraftRecipeRows() ([]content.SnapshotRow, error) {
	definitions := []crafting.RecipeDefinition{
		mustDefaultItemRecipe(
			crafting.RecipeIDRefinedAlloy,
			crafting.RecipeCategoryProcessedMaterial,
			"refined_alloy",
			5,
			[]crafting.RecipeInput{
				mustDefaultRecipeInput("iron_ore", 20),
				mustDefaultRecipeInput("carbon_shards", 5),
			},
			100,
			1,
			[]crafting.RoleRequirement{{Role: progression.RoleTypeCrafting, Level: 1}},
			5*time.Minute,
			true,
		),
		mustDefaultItemRecipe(
			crafting.RecipeIDLaserAlphaT1,
			crafting.RecipeCategoryModule,
			"laser_alpha_t1",
			1,
			[]crafting.RecipeInput{
				mustDefaultRecipeInput("refined_alloy", 18),
				mustDefaultRecipeInput("laser_lens", 3),
				mustDefaultRecipeInput("energy_cell", 2),
			},
			650,
			1,
			[]crafting.RoleRequirement{{Role: progression.RoleTypeCrafting, Level: 1}},
			20*time.Minute,
			true,
		),
		mustDefaultShipUnlockRecipe(
			crafting.RecipeIDScoutT1,
			"scout_t1",
			[]crafting.RecipeInput{
				mustDefaultRecipeInput("refined_alloy", 80),
				mustDefaultRecipeInput("scanner_circuit", 12),
				mustDefaultRecipeInput("warp_coil", 4),
			},
			2_200,
			2,
			[]crafting.RoleRequirement{{Role: progression.RoleTypeCrafting, Level: 2}},
			90*time.Minute,
		),
	}
	if _, err := crafting.NewRecipeCatalog(definitions); err != nil {
		return nil, err
	}
	rows := make([]content.SnapshotRow, 0, len(definitions))
	for _, definition := range definitions {
		row, err := snapshotRow(definition.RecipeID.String(), craftRecipeSnapshotData(definition))
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func mustDefaultItemRecipe(
	recipeID catalog.DefinitionID,
	category crafting.RecipeCategory,
	outputItemID foundation.ItemID,
	outputQuantity int64,
	inputs []crafting.RecipeInput,
	requiredCredits int64,
	requiredRank int,
	requiredRoleLevels []crafting.RoleRequirement,
	duration time.Duration,
	repeatable bool,
) crafting.RecipeDefinition {
	return crafting.RecipeDefinition{
		Source:               mustDefaultRecipeSource(recipeID),
		RecipeID:             recipeID,
		Category:             category,
		Output:               crafting.RecipeOutput{Kind: crafting.RecipeOutputKindItem, ItemID: outputItemID, Quantity: mustDefaultQuantity(outputQuantity), Tradeable: true},
		Inputs:               append([]crafting.RecipeInput(nil), inputs...),
		RequiredCredits:      mustDefaultMoney(requiredCredits),
		RequiredRank:         requiredRank,
		RequiredRoleLevels:   append([]crafting.RoleRequirement(nil), requiredRoleLevels...),
		RequiredLocationType: crafting.CraftLocationStation,
		CraftDuration:        duration,
		Repeatable:           repeatable,
	}
}

func mustDefaultShipUnlockRecipe(
	recipeID catalog.DefinitionID,
	outputShipID foundation.ShipID,
	inputs []crafting.RecipeInput,
	requiredCredits int64,
	requiredRank int,
	requiredRoleLevels []crafting.RoleRequirement,
	duration time.Duration,
) crafting.RecipeDefinition {
	return crafting.RecipeDefinition{
		Source:               mustDefaultRecipeSource(recipeID),
		RecipeID:             recipeID,
		Category:             crafting.RecipeCategoryShipUnlock,
		Output:               crafting.RecipeOutput{Kind: crafting.RecipeOutputKindShipUnlock, ShipID: outputShipID, Quantity: mustDefaultQuantity(1), Tradeable: false},
		Inputs:               append([]crafting.RecipeInput(nil), inputs...),
		RequiredCredits:      mustDefaultMoney(requiredCredits),
		RequiredRank:         requiredRank,
		RequiredRoleLevels:   append([]crafting.RoleRequirement(nil), requiredRoleLevels...),
		RequiredLocationType: crafting.CraftLocationStation,
		CraftDuration:        duration,
		Repeatable:           false,
	}
}

func craftRecipeSnapshotData(definition crafting.RecipeDefinition) craftRecipeRowData {
	inputs := make([]craftRecipeInputRowData, 0, len(definition.Inputs))
	for _, input := range definition.Inputs {
		inputs = append(inputs, craftRecipeInputRowData{
			ItemID:   input.ItemID,
			Quantity: input.Quantity.Int64(),
		})
	}
	return craftRecipeRowData{
		Source:   definition.Source,
		RecipeID: definition.RecipeID,
		Category: definition.Category,
		Output: craftRecipeOutputRowData{
			Kind:      definition.Output.Kind,
			ItemID:    definition.Output.ItemID,
			ShipID:    definition.Output.ShipID,
			Quantity:  definition.Output.Quantity.Int64(),
			Tradeable: definition.Output.Tradeable,
		},
		Inputs:               inputs,
		RequiredCredits:      definition.RequiredCredits.Int64(),
		RequiredRank:         definition.RequiredRank,
		RequiredRoleLevels:   append([]crafting.RoleRequirement(nil), definition.RequiredRoleLevels...),
		RequiredLocationType: definition.RequiredLocationType,
		CraftDurationMS:      definition.CraftDuration.Milliseconds(),
		Repeatable:           definition.Repeatable,
	}
}

func mustDefaultRecipeInput(itemID foundation.ItemID, quantity int64) crafting.RecipeInput {
	return crafting.RecipeInput{
		ItemID:   itemID,
		Quantity: mustDefaultQuantity(quantity),
	}
}

func mustDefaultRecipeSource(recipeID catalog.DefinitionID) catalog.VersionedDefinition {
	source, err := catalog.NewRecipeSource(recipeID.String(), kalaazuRecipeCatalogVersion.String())
	if err != nil {
		panic(err)
	}
	return source
}

func mustDefaultQuantity(amount int64) foundation.Quantity {
	quantity, err := foundation.NewQuantity(amount)
	if err != nil {
		panic(err)
	}
	return quantity
}

func mustDefaultMoney(amount int64) foundation.Money {
	money, err := foundation.NewMoney(amount)
	if err != nil {
		panic(err)
	}
	return money
}

func requireCraftRecipeSourceRows(itemRows []content.SnapshotRow, shipRows []content.SnapshotRow) error {
	requiredItems := []content.ContentID{
		"iron_ore",
		"carbon_shards",
		"refined_alloy",
		"laser_lens",
		"energy_cell",
		"scanner_circuit",
		"warp_coil",
		"laser_alpha_t1",
	}
	requiredShips := []content.ContentID{"scout_t1"}
	for _, contentID := range requiredItems {
		if !snapshotRowsContain(itemRows, contentID) {
			return fmt.Errorf("craft recipe item source %q missing", contentID)
		}
	}
	for _, contentID := range requiredShips {
		if !snapshotRowsContain(shipRows, contentID) {
			return fmt.Errorf("craft recipe ship source %q missing", contentID)
		}
	}
	return nil
}

func snapshotRowsContain(rows []content.SnapshotRow, contentID content.ContentID) bool {
	for _, row := range rows {
		if row.ContentID == contentID {
			return true
		}
	}
	return false
}
