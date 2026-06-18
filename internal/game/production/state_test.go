package production

import (
	"errors"
	"testing"
)

func TestProductionStateValidationRejectsInvalidEnergyAndPriority(t *testing.T) {
	state, err := NewPlanetProductionState("planet-1", testTime(0), 10, testTime(0))
	if err != nil {
		t.Fatalf("NewPlanetProductionState() error = %v, want nil", err)
	}

	state.EnergyReservedPerHour = 11
	if err := state.Validate(); !errors.Is(err, ErrInvalidEnergyRate) {
		t.Fatalf("reserved over capacity error = %v, want ErrInvalidEnergyRate", err)
	}

	state.EnergyReservedPerHour = 0
	state.BuildingPriority = []BuildingID{"building-1", "building-1"}
	if err := state.Validate(); !errors.Is(err, ErrInvalidProductionState) {
		t.Fatalf("duplicate priority error = %v, want ErrInvalidProductionState", err)
	}
}

func TestProductionSnapshotValidationRequiresMatchingPlanet(t *testing.T) {
	state, err := NewPlanetProductionState("planet-1", testTime(0), 10, testTime(0))
	if err != nil {
		t.Fatalf("NewPlanetProductionState() error = %v, want nil", err)
	}
	storage, err := NewPlanetStorage("planet-2", 10, nil, testTime(0))
	if err != nil {
		t.Fatalf("NewPlanetStorage() error = %v, want nil", err)
	}

	err = (PlanetProductionSnapshot{State: state, Storage: storage}).Validate()
	if !errors.Is(err, ErrProductionSnapshotIncomplete) {
		t.Fatalf("mismatched snapshot error = %v, want ErrProductionSnapshotIncomplete", err)
	}
}
