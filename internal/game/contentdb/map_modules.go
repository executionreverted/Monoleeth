package contentdb

import (
	"fmt"

	"gameproject/internal/game/content"
	"gameproject/internal/game/modules"
)

func mapModuleRows(snapshot content.Snapshot) (modules.Catalog, error) {
	definitions := make([]modules.ModuleDefinition, 0, len(snapshot.Modules))
	version := publishedVersion(snapshot)
	for _, row := range snapshot.Modules {
		if !row.Enabled {
			continue
		}
		var definition modules.ModuleDefinition
		if err := decodeSnapshotRow(content.ContentTypeModule, row, &definition); err != nil {
			return modules.Catalog{}, err
		}
		if err := requireRowID(content.ContentTypeModule, row, definition.ItemID.String()); err != nil {
			return modules.Catalog{}, err
		}
		definition.Source = forceSourceVersion(definition.Source, version)
		definitions = append(definitions, definition)
	}
	catalogRows, err := modules.NewCatalog(definitions)
	if err != nil {
		return modules.Catalog{}, fmt.Errorf("modules: %w", err)
	}
	return catalogRows, nil
}
