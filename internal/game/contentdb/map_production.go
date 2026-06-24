package contentdb

import (
	"fmt"

	"gameproject/internal/game/content"
	"gameproject/internal/game/production"
)

func mapProductionRows(snapshot content.Snapshot) (production.Catalog, error) {
	definitions := make([]production.BuildingProductionDefinition, 0, len(snapshot.ProductionBuildings))
	version := publishedVersion(snapshot)
	for _, row := range snapshot.ProductionBuildings {
		if !row.Enabled {
			continue
		}
		var definition production.BuildingProductionDefinition
		if err := decodeSnapshotRow(content.ContentTypeProductionBuilding, row, &definition); err != nil {
			return production.Catalog{}, err
		}
		if err := requireRowID(content.ContentTypeProductionBuilding, row, definition.DefinitionID.String()); err != nil {
			return production.Catalog{}, err
		}
		definition.Source = forceSourceVersion(definition.Source, version)
		definitions = append(definitions, definition)
	}
	catalogRows, err := production.NewCatalog(definitions)
	if err != nil {
		return production.Catalog{}, fmt.Errorf("production: %w", err)
	}
	return catalogRows, nil
}
