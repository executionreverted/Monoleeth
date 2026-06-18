package production

import (
	"errors"
	"testing"
)

func TestNewPlanetBuildingUsesDefinitionAndValidatesState(t *testing.T) {
	definition, err := MustMVPCatalog().MustGet(ProductionDefinitionIDIronExtractorL1)
	if err != nil {
		t.Fatalf("MustGet() error = %v, want nil", err)
	}

	building, err := NewPlanetBuilding("building-1", "planet-1", definition, BuildingStateActive, testTime(0), testTime(1))
	if err != nil {
		t.Fatalf("NewPlanetBuilding() error = %v, want nil", err)
	}
	if building.Source != definition.Source || building.BuildingType != definition.BuildingType || building.Level != definition.Level {
		t.Fatalf("building = %+v, want definition fields %+v", building, definition)
	}

	_, err = NewPlanetBuilding("building-2", "planet-1", definition, BuildingState("paused"), testTime(0), testTime(1))
	if !errors.Is(err, ErrInvalidBuildingState) {
		t.Fatalf("invalid state error = %v, want ErrInvalidBuildingState", err)
	}
}

func TestPlanetBuildingValidationRejectsInvalidRows(t *testing.T) {
	definition, err := MustMVPCatalog().MustGet(ProductionDefinitionIDAlloyFoundryL1)
	if err != nil {
		t.Fatalf("MustGet() error = %v, want nil", err)
	}
	building, err := NewPlanetBuilding("building-1", "planet-1", definition, BuildingStateActive, testTime(0), testTime(1))
	if err != nil {
		t.Fatalf("NewPlanetBuilding() error = %v, want nil", err)
	}

	building.UpdatedAt = testTime(-1)
	if err := building.Validate(); !errors.Is(err, ErrInvalidProductionState) {
		t.Fatalf("updated before created error = %v, want ErrInvalidProductionState", err)
	}

	building = building.Clone()
	building.BuildingID = " "
	if err := building.Validate(); !errors.Is(err, ErrInvalidBuildingID) {
		t.Fatalf("invalid building id error = %v, want ErrInvalidBuildingID", err)
	}
}
