package production

import (
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

// ProductionCatalogVersion identifies the first static planet production catalog slice.
const ProductionCatalogVersion catalog.Version = "production_catalog_mvp_v1"

// MVP production definition ids.
const (
	ProductionDefinitionIDIronExtractorL1 catalog.DefinitionID = "iron_extractor_l1"
	ProductionDefinitionIDAlloyFoundryL1  catalog.DefinitionID = "alloy_foundry_l1"
)

// Catalog indexes static building production definitions.
type Catalog struct {
	definitions    []BuildingProductionDefinition
	byDefinitionID map[catalog.DefinitionID]BuildingProductionDefinition
	byBuilding     map[buildingLevelKey]BuildingProductionDefinition
}

// NewCatalog validates and indexes production definitions.
func NewCatalog(definitions []BuildingProductionDefinition) (Catalog, error) {
	byDefinitionID := make(map[catalog.DefinitionID]BuildingProductionDefinition, len(definitions))
	byBuilding := make(map[buildingLevelKey]BuildingProductionDefinition, len(definitions))
	cloned := make([]BuildingProductionDefinition, 0, len(definitions))

	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return Catalog{}, err
		}
		if _, ok := byDefinitionID[definition.DefinitionID]; ok {
			return Catalog{}, fmt.Errorf("production %q: %w", definition.DefinitionID, ErrDuplicateProductionDefinition)
		}
		key := buildingLevelKey{
			buildingType: definition.BuildingType,
			level:        definition.Level,
		}
		if _, ok := byBuilding[key]; ok {
			return Catalog{}, fmt.Errorf("building %q level %d: %w", definition.BuildingType, definition.Level, ErrDuplicateBuildingDefinition)
		}

		clonedDefinition := cloneDefinition(definition)
		byDefinitionID[clonedDefinition.DefinitionID] = clonedDefinition
		byBuilding[key] = clonedDefinition
		cloned = append(cloned, clonedDefinition)
	}

	return Catalog{
		definitions:    cloned,
		byDefinitionID: byDefinitionID,
		byBuilding:     byBuilding,
	}, nil
}

// MVPCatalog returns the validated MVP production catalog.
func MVPCatalog() (Catalog, error) {
	return NewCatalog(MVPProductionDefinitions())
}

// MustMVPCatalog returns the validated MVP production catalog or panics if
// checked-in catalog data is invalid.
func MustMVPCatalog() Catalog {
	catalogRows, err := MVPCatalog()
	if err != nil {
		panic(err)
	}
	return catalogRows
}

// Definitions returns all definitions in deterministic catalog order.
func (catalogRows Catalog) Definitions() []BuildingProductionDefinition {
	definitions := make([]BuildingProductionDefinition, 0, len(catalogRows.definitions))
	for _, definition := range catalogRows.definitions {
		definitions = append(definitions, cloneDefinition(definition))
	}
	return definitions
}

// Get returns one production definition by catalog definition id.
func (catalogRows Catalog) Get(definitionID catalog.DefinitionID) (BuildingProductionDefinition, bool) {
	definition, ok := catalogRows.byDefinitionID[definitionID]
	if !ok {
		return BuildingProductionDefinition{}, false
	}
	return cloneDefinition(definition), true
}

// MustGet returns one production definition by id or an unknown-definition error.
func (catalogRows Catalog) MustGet(definitionID catalog.DefinitionID) (BuildingProductionDefinition, error) {
	definition, ok := catalogRows.Get(definitionID)
	if !ok {
		return BuildingProductionDefinition{}, fmt.Errorf("production %q: %w", definitionID, ErrUnknownProductionDefinition)
	}
	return definition, nil
}

// GetBuilding returns the production definition for a building type and level.
func (catalogRows Catalog) GetBuilding(buildingType BuildingType, level int) (BuildingProductionDefinition, bool) {
	definition, ok := catalogRows.byBuilding[buildingLevelKey{buildingType: buildingType, level: level}]
	if !ok {
		return BuildingProductionDefinition{}, false
	}
	return cloneDefinition(definition), true
}

// MVPProductionDefinitions returns the initial planet production rows for
// Phase 09 catalog work. The rates are whole item units per server hour.
func MVPProductionDefinitions() []BuildingProductionDefinition {
	return []BuildingProductionDefinition{
		newMVPDefinition(
			ProductionDefinitionIDIronExtractorL1,
			BuildingTypeIronExtractor,
			BuildingCategoryExtractor,
			1,
			nil,
			[]ItemRate{
				mustItemRate("iron_ore", 30),
			},
			4,
		),
		newMVPDefinition(
			ProductionDefinitionIDAlloyFoundryL1,
			BuildingTypeAlloyFoundry,
			BuildingCategoryRefinery,
			1,
			[]ItemRate{
				mustItemRate("iron_ore", 30),
			},
			[]ItemRate{
				mustItemRate("refined_alloy", 10),
			},
			5,
		),
	}
}

func newMVPDefinition(
	definitionID catalog.DefinitionID,
	buildingType BuildingType,
	category BuildingCategory,
	level int,
	inputs []ItemRate,
	outputs []ItemRate,
	energyCostPerHour int64,
) BuildingProductionDefinition {
	source, err := catalog.NewVersionedDefinitionFromStrings(definitionID.String(), ProductionCatalogVersion.String())
	if err != nil {
		panic(err)
	}
	definition := BuildingProductionDefinition{
		Source:            source,
		DefinitionID:      definitionID,
		BuildingType:      buildingType,
		Category:          category,
		Level:             level,
		Inputs:            append([]ItemRate(nil), inputs...),
		Outputs:           append([]ItemRate(nil), outputs...),
		EnergyCostPerHour: energyCostPerHour,
	}
	if err := definition.Validate(); err != nil {
		panic(err)
	}
	return definition
}

func mustItemRate(itemID foundation.ItemID, amountPerHour int64) ItemRate {
	rate := ItemRate{
		ItemID:        itemID,
		AmountPerHour: amountPerHour,
	}
	if err := rate.Validate(); err != nil {
		panic(err)
	}
	return rate
}

func cloneDefinition(definition BuildingProductionDefinition) BuildingProductionDefinition {
	definition.Inputs = append([]ItemRate(nil), definition.Inputs...)
	definition.Outputs = append([]ItemRate(nil), definition.Outputs...)
	return definition
}
