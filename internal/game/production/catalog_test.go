package production

import (
	"errors"
	"testing"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

func TestMVPCatalogContainsExtractorAndRefineryDefinitions(t *testing.T) {
	catalogRows := MustMVPCatalog()
	definitions := catalogRows.Definitions()

	if got, want := len(definitions), 3; got != want {
		t.Fatalf("MVP definitions count = %d, want %d", got, want)
	}

	extractor, err := catalogRows.MustGet(ProductionDefinitionIDIronExtractorL1)
	if err != nil {
		t.Fatalf("MustGet(%q) error = %v, want nil", ProductionDefinitionIDIronExtractorL1, err)
	}
	assertDefinitionBasics(t, extractor, ProductionDefinitionIDIronExtractorL1, BuildingTypeIronExtractor, BuildingCategoryExtractor)
	if len(extractor.Inputs) != 0 {
		t.Fatalf("extractor Inputs len = %d, want 0", len(extractor.Inputs))
	}
	assertRate(t, extractor.Outputs, "iron_ore", 30)
	if extractor.EnergyCostPerHour != 4 {
		t.Fatalf("extractor EnergyCostPerHour = %d, want 4", extractor.EnergyCostPerHour)
	}

	refinery, err := catalogRows.MustGet(ProductionDefinitionIDAlloyFoundryL1)
	if err != nil {
		t.Fatalf("MustGet(%q) error = %v, want nil", ProductionDefinitionIDAlloyFoundryL1, err)
	}
	assertDefinitionBasics(t, refinery, ProductionDefinitionIDAlloyFoundryL1, BuildingTypeAlloyFoundry, BuildingCategoryRefinery)
	assertRate(t, refinery.Inputs, "iron_ore", 30)
	assertRate(t, refinery.Outputs, "refined_alloy", 10)
	if refinery.EnergyCostPerHour != 5 {
		t.Fatalf("refinery EnergyCostPerHour = %d, want 5", refinery.EnergyCostPerHour)
	}

	extractorL2, err := catalogRows.MustGet(ProductionDefinitionIDIronExtractorL2)
	if err != nil {
		t.Fatalf("MustGet(%q) error = %v, want nil", ProductionDefinitionIDIronExtractorL2, err)
	}
	if err := extractorL2.Validate(); err != nil {
		t.Fatalf("extractor L2 Validate() = %v, want nil", err)
	}
	if extractorL2.Source.Version != ProductionCatalogVersion {
		t.Fatalf("extractor L2 Source.Version = %q, want %q", extractorL2.Source.Version, ProductionCatalogVersion)
	}
	if extractorL2.DefinitionID != ProductionDefinitionIDIronExtractorL2 {
		t.Fatalf("extractor L2 DefinitionID = %q, want %q", extractorL2.DefinitionID, ProductionDefinitionIDIronExtractorL2)
	}
	if extractorL2.BuildingType != BuildingTypeIronExtractor {
		t.Fatalf("extractor L2 BuildingType = %q, want %q", extractorL2.BuildingType, BuildingTypeIronExtractor)
	}
	if extractorL2.Category != BuildingCategoryExtractor {
		t.Fatalf("extractor L2 Category = %q, want %q", extractorL2.Category, BuildingCategoryExtractor)
	}
	if extractorL2.Level != 2 {
		t.Fatalf("extractor L2 level = %d, want 2", extractorL2.Level)
	}
	if len(extractorL2.Inputs) != 0 {
		t.Fatalf("extractor L2 Inputs len = %d, want 0", len(extractorL2.Inputs))
	}
	assertRate(t, extractorL2.Outputs, "iron_ore", 60)
	if extractorL2.EnergyCostPerHour != 8 {
		t.Fatalf("extractor L2 EnergyCostPerHour = %d, want 8", extractorL2.EnergyCostPerHour)
	}

	byBuilding, ok := catalogRows.GetBuilding(BuildingTypeAlloyFoundry, 1)
	if !ok {
		t.Fatalf("GetBuilding(%q, 1) = false, want true", BuildingTypeAlloyFoundry)
	}
	if byBuilding.DefinitionID != ProductionDefinitionIDAlloyFoundryL1 {
		t.Fatalf("GetBuilding DefinitionID = %q, want %q", byBuilding.DefinitionID, ProductionDefinitionIDAlloyFoundryL1)
	}
	byBuilding, ok = catalogRows.GetBuilding(BuildingTypeIronExtractor, 2)
	if !ok {
		t.Fatalf("GetBuilding(%q, 2) = false, want true", BuildingTypeIronExtractor)
	}
	if byBuilding.DefinitionID != ProductionDefinitionIDIronExtractorL2 {
		t.Fatalf("GetBuilding DefinitionID = %q, want %q", byBuilding.DefinitionID, ProductionDefinitionIDIronExtractorL2)
	}
}

func TestProductionDefinitionRejectsInvalidRows(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*BuildingProductionDefinition)
		wantErr error
	}{
		{
			name: "source mismatch",
			mutate: func(definition *BuildingProductionDefinition) {
				definition.Source.DefinitionID = "other_production"
			},
			wantErr: ErrProductionSourceMismatch,
		},
		{
			name: "invalid building type",
			mutate: func(definition *BuildingProductionDefinition) {
				definition.BuildingType = "quantum_fountain"
			},
			wantErr: ErrInvalidBuildingType,
		},
		{
			name: "invalid category",
			mutate: func(definition *BuildingProductionDefinition) {
				definition.Category = "alchemy"
			},
			wantErr: ErrInvalidBuildingCategory,
		},
		{
			name: "zero building level",
			mutate: func(definition *BuildingProductionDefinition) {
				definition.Level = 0
			},
			wantErr: ErrInvalidBuildingLevel,
		},
		{
			name: "empty outputs",
			mutate: func(definition *BuildingProductionDefinition) {
				definition.Outputs = nil
			},
			wantErr: ErrEmptyProductionOutputs,
		},
		{
			name: "zero output rate",
			mutate: func(definition *BuildingProductionDefinition) {
				definition.Outputs[0].AmountPerHour = 0
			},
			wantErr: ErrInvalidProductionRate,
		},
		{
			name: "duplicate input",
			mutate: func(definition *BuildingProductionDefinition) {
				definition.Inputs = append(definition.Inputs, definition.Inputs[0])
			},
			wantErr: ErrDuplicateProductionInput,
		},
		{
			name: "duplicate output",
			mutate: func(definition *BuildingProductionDefinition) {
				definition.Outputs = append(definition.Outputs, definition.Outputs[0])
			},
			wantErr: ErrDuplicateProductionOutput,
		},
		{
			name: "zero energy cost",
			mutate: func(definition *BuildingProductionDefinition) {
				definition.EnergyCostPerHour = 0
			},
			wantErr: ErrInvalidEnergyCost,
		},
		{
			name: "extractor with inputs",
			mutate: func(definition *BuildingProductionDefinition) {
				definition.Category = BuildingCategoryExtractor
				definition.BuildingType = BuildingTypeIronExtractor
			},
			wantErr: ErrUnexpectedExtractorInputs,
		},
		{
			name: "refinery without inputs",
			mutate: func(definition *BuildingProductionDefinition) {
				definition.Inputs = nil
			},
			wantErr: ErrEmptyRefineryInputs,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			definition := validProductionDefinition()
			tc.mutate(&definition)
			if _, err := NewCatalog([]BuildingProductionDefinition{definition}); !errors.Is(err, tc.wantErr) {
				t.Fatalf("NewCatalog() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestProductionCatalogRejectsDuplicateRowsAndUnknownDefinitions(t *testing.T) {
	definition := validProductionDefinition()
	if _, err := NewCatalog([]BuildingProductionDefinition{definition, definition}); !errors.Is(err, ErrDuplicateProductionDefinition) {
		t.Fatalf("duplicate definition error = %v, want %v", err, ErrDuplicateProductionDefinition)
	}

	sameBuilding := validProductionDefinition()
	sameBuilding.DefinitionID = "alloy_foundry_l1_variant"
	sameBuilding.Source = mustProductionSource(sameBuilding.DefinitionID)
	if _, err := NewCatalog([]BuildingProductionDefinition{definition, sameBuilding}); !errors.Is(err, ErrDuplicateBuildingDefinition) {
		t.Fatalf("duplicate building error = %v, want %v", err, ErrDuplicateBuildingDefinition)
	}

	catalogRows := MustMVPCatalog()
	if _, err := catalogRows.MustGet("missing_production"); !errors.Is(err, ErrUnknownProductionDefinition) {
		t.Fatalf("unknown definition error = %v, want %v", err, ErrUnknownProductionDefinition)
	}
}

func TestProductionCatalogClonesReturnedDefinitions(t *testing.T) {
	catalogRows := MustMVPCatalog()
	definitions := catalogRows.Definitions()
	definitions[0].Outputs[0].AmountPerHour = 999

	extractor, ok := catalogRows.Get(ProductionDefinitionIDIronExtractorL1)
	if !ok {
		t.Fatalf("Get(%q) = false, want true", ProductionDefinitionIDIronExtractorL1)
	}
	if extractor.Outputs[0].AmountPerHour == 999 {
		t.Fatal("catalog definition mutated through returned Definitions slice")
	}

	extractor.Outputs[0].AmountPerHour = 999
	extractorAgain, ok := catalogRows.Get(ProductionDefinitionIDIronExtractorL1)
	if !ok {
		t.Fatalf("Get(%q) = false, want true", ProductionDefinitionIDIronExtractorL1)
	}
	if extractorAgain.Outputs[0].AmountPerHour == 999 {
		t.Fatal("catalog definition mutated through returned Get value")
	}
}

func TestBuildingStateCompatibilityAllowsActiveAndDisabledOnly(t *testing.T) {
	definition := validProductionDefinition()

	produces, err := definition.ProducesInState(BuildingStateActive)
	if err != nil {
		t.Fatalf("ProducesInState(active) error = %v, want nil", err)
	}
	if !produces {
		t.Fatal("ProducesInState(active) = false, want true")
	}

	produces, err = definition.ProducesInState(BuildingStateDisabled)
	if err != nil {
		t.Fatalf("ProducesInState(disabled) error = %v, want nil", err)
	}
	if produces {
		t.Fatal("ProducesInState(disabled) = true, want false")
	}

	if err := definition.ValidateStateCompatibility("constructing"); !errors.Is(err, ErrInvalidBuildingState) {
		t.Fatalf("ValidateStateCompatibility(constructing) error = %v, want %v", err, ErrInvalidBuildingState)
	}
}

func assertDefinitionBasics(
	t *testing.T,
	definition BuildingProductionDefinition,
	definitionID catalog.DefinitionID,
	buildingType BuildingType,
	category BuildingCategory,
) {
	t.Helper()
	if err := definition.Validate(); err != nil {
		t.Fatalf("definition Validate() = %v, want nil", err)
	}
	if definition.Source.Version != ProductionCatalogVersion {
		t.Fatalf("Source.Version = %q, want %q", definition.Source.Version, ProductionCatalogVersion)
	}
	if definition.DefinitionID != definitionID {
		t.Fatalf("DefinitionID = %q, want %q", definition.DefinitionID, definitionID)
	}
	if definition.BuildingType != buildingType {
		t.Fatalf("BuildingType = %q, want %q", definition.BuildingType, buildingType)
	}
	if definition.Category != category {
		t.Fatalf("Category = %q, want %q", definition.Category, category)
	}
	if definition.Level != 1 {
		t.Fatalf("Level = %d, want 1", definition.Level)
	}
}

func assertRate(t *testing.T, rates []ItemRate, itemID foundation.ItemID, amountPerHour int64) {
	t.Helper()
	for _, rate := range rates {
		if rate.ItemID == itemID {
			if rate.AmountPerHour != amountPerHour {
				t.Fatalf("rate for %q = %d, want %d", itemID, rate.AmountPerHour, amountPerHour)
			}
			return
		}
	}
	t.Fatalf("rates missing item %q", itemID)
}

func validProductionDefinition() BuildingProductionDefinition {
	return newMVPDefinition(
		ProductionDefinitionIDAlloyFoundryL1,
		BuildingTypeAlloyFoundry,
		BuildingCategoryRefinery,
		1,
		[]ItemRate{mustItemRate("iron_ore", 30)},
		[]ItemRate{mustItemRate("refined_alloy", 10)},
		5,
	)
}

func mustProductionSource(definitionID catalog.DefinitionID) catalog.VersionedDefinition {
	source, err := catalog.NewVersionedDefinitionFromStrings(definitionID.String(), ProductionCatalogVersion.String())
	if err != nil {
		panic(err)
	}
	return source
}
