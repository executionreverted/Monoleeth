package kalaazu

import (
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
)

const kalaazuProductionCatalogVersion catalog.Version = "kalaazu_production_seed_v1"

func mapProductionBuildingRows(itemRows []content.SnapshotRow) ([]content.SnapshotRow, error) {
	if err := requireProductionBuildingSourceRows(itemRows); err != nil {
		return nil, err
	}
	definitions := []production.BuildingProductionDefinition{
		mustDefaultProductionDefinition(
			production.ProductionDefinitionIDIronExtractorL1,
			production.BuildingTypeIronExtractor,
			production.BuildingCategoryExtractor,
			1,
			nil,
			[]production.ItemRate{
				mustDefaultItemRate("iron_ore", 30),
			},
			4,
		),
		mustDefaultProductionDefinition(
			production.ProductionDefinitionIDAlloyFoundryL1,
			production.BuildingTypeAlloyFoundry,
			production.BuildingCategoryRefinery,
			1,
			[]production.ItemRate{
				mustDefaultItemRate("iron_ore", 30),
			},
			[]production.ItemRate{
				mustDefaultItemRate("refined_alloy", 10),
			},
			5,
		),
		mustDefaultProductionDefinition(
			production.ProductionDefinitionIDIronExtractorL2,
			production.BuildingTypeIronExtractor,
			production.BuildingCategoryExtractor,
			2,
			nil,
			[]production.ItemRate{
				mustDefaultItemRate("iron_ore", 60),
			},
			8,
		),
	}
	if _, err := production.NewCatalog(definitions); err != nil {
		return nil, err
	}
	rows := make([]content.SnapshotRow, 0, len(definitions))
	for _, definition := range definitions {
		row, err := snapshotRow(definition.DefinitionID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func mustDefaultProductionDefinition(
	definitionID catalog.DefinitionID,
	buildingType production.BuildingType,
	category production.BuildingCategory,
	level int,
	inputs []production.ItemRate,
	outputs []production.ItemRate,
	energyCostPerHour int64,
) production.BuildingProductionDefinition {
	source, err := catalog.NewVersionedDefinitionFromStrings(definitionID.String(), kalaazuProductionCatalogVersion.String())
	if err != nil {
		panic(err)
	}
	definition := production.BuildingProductionDefinition{
		Source:            source,
		DefinitionID:      definitionID,
		BuildingType:      buildingType,
		Category:          category,
		Level:             level,
		Inputs:            append([]production.ItemRate(nil), inputs...),
		Outputs:           append([]production.ItemRate(nil), outputs...),
		EnergyCostPerHour: energyCostPerHour,
	}
	if err := definition.Validate(); err != nil {
		panic(err)
	}
	return definition
}

func mustDefaultItemRate(itemID foundation.ItemID, amountPerHour int64) production.ItemRate {
	rate := production.ItemRate{
		ItemID:        itemID,
		AmountPerHour: amountPerHour,
	}
	if err := rate.Validate(); err != nil {
		panic(err)
	}
	return rate
}

func requireProductionBuildingSourceRows(itemRows []content.SnapshotRow) error {
	for _, contentID := range []content.ContentID{
		"iron_ore",
		"refined_alloy",
	} {
		if !snapshotRowsContain(itemRows, contentID) {
			return fmt.Errorf("production building item source %q missing", contentID)
		}
	}
	return nil
}
