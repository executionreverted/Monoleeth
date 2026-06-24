package contentseed

import (
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/foundation"
)

type craftRecipeSnapshotRowData struct {
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

func craftRecipeSnapshotData(definition crafting.RecipeDefinition) craftRecipeSnapshotRowData {
	inputs := make([]craftRecipeInputRowData, 0, len(definition.Inputs))
	for _, input := range definition.Inputs {
		inputs = append(inputs, craftRecipeInputRowData{
			ItemID:   input.ItemID,
			Quantity: input.Quantity.Int64(),
		})
	}
	return craftRecipeSnapshotRowData{
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
