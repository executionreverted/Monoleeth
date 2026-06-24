package contentdb

import (
	"fmt"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/foundation"
)

type recipeRowData struct {
	Source               catalog.VersionedDefinition `json:"source"`
	RecipeID             catalog.DefinitionID        `json:"recipe_id"`
	Category             crafting.RecipeCategory     `json:"category"`
	Output               recipeOutputRowData         `json:"output"`
	Inputs               []recipeInputRowData        `json:"inputs"`
	RequiredCredits      int64                       `json:"required_credits"`
	RequiredRank         int                         `json:"required_rank"`
	RequiredRoleLevels   []crafting.RoleRequirement  `json:"required_role_levels,omitempty"`
	RequiredLocationType crafting.CraftLocationType  `json:"required_location_type"`
	CraftDuration        time.Duration               `json:"craft_duration"`
	Repeatable           bool                        `json:"repeatable"`
}

type recipeInputRowData struct {
	ItemID   foundation.ItemID `json:"item_id"`
	Quantity int64             `json:"quantity"`
}

type recipeOutputRowData struct {
	Kind      crafting.RecipeOutputKind `json:"kind"`
	ItemID    foundation.ItemID         `json:"item_id,omitempty"`
	ShipID    foundation.ShipID         `json:"ship_id,omitempty"`
	Quantity  int64                     `json:"quantity"`
	Tradeable bool                      `json:"tradeable"`
}

func mapCraftRecipeRows(snapshot content.Snapshot) (crafting.RecipeCatalog, error) {
	definitions := make([]crafting.RecipeDefinition, 0, len(snapshot.CraftRecipes))
	version := publishedVersion(snapshot)
	for _, row := range snapshot.CraftRecipes {
		if !row.Enabled {
			continue
		}
		var data recipeRowData
		if err := decodeSnapshotRow(content.ContentTypeCraftRecipe, row, &data); err != nil {
			return crafting.RecipeCatalog{}, err
		}
		if err := requireRowID(content.ContentTypeCraftRecipe, row, data.RecipeID.String()); err != nil {
			return crafting.RecipeCatalog{}, err
		}
		definition, err := data.toDefinition(version)
		if err != nil {
			return crafting.RecipeCatalog{}, fmt.Errorf("recipe %q: %w", row.ContentID, err)
		}
		definitions = append(definitions, definition)
	}
	catalogRows, err := crafting.NewRecipeCatalog(definitions)
	if err != nil {
		return crafting.RecipeCatalog{}, fmt.Errorf("recipes: %w", err)
	}
	return catalogRows, nil
}

func (data recipeRowData) toDefinition(version catalog.Version) (crafting.RecipeDefinition, error) {
	outputQuantity, err := foundation.NewQuantity(data.Output.Quantity)
	if err != nil {
		return crafting.RecipeDefinition{}, fmt.Errorf("output quantity: %w", err)
	}
	inputs := make([]crafting.RecipeInput, 0, len(data.Inputs))
	for _, input := range data.Inputs {
		quantity, err := foundation.NewQuantity(input.Quantity)
		if err != nil {
			return crafting.RecipeDefinition{}, fmt.Errorf("input %q quantity: %w", input.ItemID, err)
		}
		inputs = append(inputs, crafting.RecipeInput{
			ItemID:   input.ItemID,
			Quantity: quantity,
		})
	}
	requiredCredits, err := foundation.NewMoney(data.RequiredCredits)
	if err != nil {
		return crafting.RecipeDefinition{}, fmt.Errorf("required credits: %w", err)
	}
	return crafting.RecipeDefinition{
		Source:   forceSourceVersion(data.Source, version),
		RecipeID: data.RecipeID,
		Category: data.Category,
		Output: crafting.RecipeOutput{
			Kind:      data.Output.Kind,
			ItemID:    data.Output.ItemID,
			ShipID:    data.Output.ShipID,
			Quantity:  outputQuantity,
			Tradeable: data.Output.Tradeable,
		},
		Inputs:               inputs,
		RequiredCredits:      requiredCredits,
		RequiredRank:         data.RequiredRank,
		RequiredRoleLevels:   append([]crafting.RoleRequirement(nil), data.RequiredRoleLevels...),
		RequiredLocationType: data.RequiredLocationType,
		CraftDuration:        data.CraftDuration,
		Repeatable:           data.Repeatable,
	}, nil
}
