package contentdb

import (
	"fmt"

	"gameproject/internal/game/content"
	"gameproject/internal/game/ships"
)

func mapShipRows(snapshot content.Snapshot) (ships.Catalog, error) {
	definitions := make([]ships.ShipDefinition, 0, len(snapshot.Ships))
	version := publishedVersion(snapshot)
	for _, row := range snapshot.Ships {
		if !row.Enabled {
			continue
		}
		var definition ships.ShipDefinition
		if err := decodeSnapshotRow(content.ContentTypeShip, row, &definition); err != nil {
			return ships.Catalog{}, err
		}
		if err := requireRowID(content.ContentTypeShip, row, definition.ShipID.String()); err != nil {
			return ships.Catalog{}, err
		}
		definition.Source = forceSourceVersion(definition.Source, version)
		definitions = append(definitions, definition)
	}
	catalogRows, err := ships.NewCatalog(definitions)
	if err != nil {
		return ships.Catalog{}, fmt.Errorf("ships: %w", err)
	}
	return catalogRows, nil
}
