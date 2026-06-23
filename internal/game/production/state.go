package production

import (
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
)

// PlanetProductionState records per-planet production control state.
type PlanetProductionState struct {
	PlanetID              foundation.PlanetID `json:"planet_id"`
	LastCalculatedAt      time.Time           `json:"last_calculated_at"`
	ProductionEnabled     bool                `json:"production_enabled"`
	EnergyCapacityPerHour int64               `json:"energy_capacity_per_hour"`
	EnergyReservedPerHour int64               `json:"energy_reserved_per_hour"`
	BuildingPriority      []BuildingID        `json:"building_priority,omitempty"`
	UpdatedAt             time.Time           `json:"updated_at"`
}

// PlanetProductionSnapshot returns a detached aggregate view for one planet.
type PlanetProductionSnapshot struct {
	State     PlanetProductionState `json:"state"`
	Storage   PlanetStorage         `json:"storage"`
	Buildings []PlanetBuilding      `json:"buildings,omitempty"`
}

// NewPlanetProductionState validates and returns a production state row.
func NewPlanetProductionState(
	planetID foundation.PlanetID,
	lastCalculatedAt time.Time,
	energyCapacityPerHour int64,
	updatedAt time.Time,
) (PlanetProductionState, error) {
	state := PlanetProductionState{
		PlanetID:              planetID,
		LastCalculatedAt:      lastCalculatedAt,
		ProductionEnabled:     true,
		EnergyCapacityPerHour: energyCapacityPerHour,
		UpdatedAt:             updatedAt,
	}
	if err := state.Validate(); err != nil {
		return PlanetProductionState{}, err
	}
	return cloneProductionState(state), nil
}

// Validate reports whether state has valid timestamps and energy budget fields.
func (state PlanetProductionState) Validate() error {
	if err := state.PlanetID.Validate(); err != nil {
		return err
	}
	if state.LastCalculatedAt.IsZero() {
		return fmt.Errorf("last_calculated_at: %w", ErrZeroProductionTimestamp)
	}
	if state.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at: %w", ErrZeroProductionTimestamp)
	}
	if err := validateNonNegativeBoundedAmount("energy capacity per hour", state.EnergyCapacityPerHour, ErrInvalidEnergyRate); err != nil {
		return err
	}
	if err := validateNonNegativeBoundedAmount("energy reserved per hour", state.EnergyReservedPerHour, ErrInvalidEnergyRate); err != nil {
		return err
	}
	if state.EnergyReservedPerHour > state.EnergyCapacityPerHour {
		return fmt.Errorf("reserved %d capacity %d: %w", state.EnergyReservedPerHour, state.EnergyCapacityPerHour, ErrInvalidEnergyRate)
	}
	seen := make(map[BuildingID]struct{}, len(state.BuildingPriority))
	for _, buildingID := range state.BuildingPriority {
		if err := buildingID.Validate(); err != nil {
			return err
		}
		if _, ok := seen[buildingID]; ok {
			return fmt.Errorf("building priority %q: %w", buildingID, ErrInvalidProductionState)
		}
		seen[buildingID] = struct{}{}
	}
	return nil
}

// Validate reports whether snapshot contains matching validated state, storage, and buildings.
func (snapshot PlanetProductionSnapshot) Validate() error {
	if err := snapshot.State.Validate(); err != nil {
		return err
	}
	if err := snapshot.Storage.Validate(); err != nil {
		return err
	}
	if snapshot.State.PlanetID != snapshot.Storage.PlanetID {
		return fmt.Errorf("state planet %q storage planet %q: %w", snapshot.State.PlanetID, snapshot.Storage.PlanetID, ErrProductionSnapshotIncomplete)
	}
	seen := make(map[BuildingID]struct{}, len(snapshot.Buildings))
	for _, building := range snapshot.Buildings {
		if err := building.Validate(); err != nil {
			return err
		}
		if building.PlanetID != snapshot.State.PlanetID {
			return fmt.Errorf("building %q planet %q state planet %q: %w", building.BuildingID, building.PlanetID, snapshot.State.PlanetID, ErrProductionSnapshotIncomplete)
		}
		if _, ok := seen[building.BuildingID]; ok {
			return fmt.Errorf("building %q: %w", building.BuildingID, ErrInvalidProductionState)
		}
		seen[building.BuildingID] = struct{}{}
	}
	return nil
}

// Clone returns a detached state copy.
func (state PlanetProductionState) Clone() PlanetProductionState {
	return cloneProductionState(state)
}

// Clone returns a detached snapshot copy.
func (snapshot PlanetProductionSnapshot) Clone() PlanetProductionSnapshot {
	return cloneProductionSnapshot(snapshot)
}

func cloneProductionState(state PlanetProductionState) PlanetProductionState {
	state.LastCalculatedAt = state.LastCalculatedAt.UTC()
	state.UpdatedAt = state.UpdatedAt.UTC()
	state.BuildingPriority = append([]BuildingID(nil), state.BuildingPriority...)
	return state
}

func cloneProductionStatePointer(state *PlanetProductionState) *PlanetProductionState {
	if state == nil {
		return nil
	}
	cloned := cloneProductionState(*state)
	return &cloned
}

func cloneProductionSnapshot(snapshot PlanetProductionSnapshot) PlanetProductionSnapshot {
	snapshot.State = cloneProductionState(snapshot.State)
	snapshot.Storage = clonePlanetStorage(snapshot.Storage)
	snapshot.Buildings = clonePlanetBuildings(snapshot.Buildings)
	return snapshot
}

func clonePlanetBuildings(buildings []PlanetBuilding) []PlanetBuilding {
	if len(buildings) == 0 {
		return nil
	}
	cloned := make([]PlanetBuilding, 0, len(buildings))
	for _, building := range buildings {
		cloned = append(cloned, clonePlanetBuilding(building))
	}
	sortPlanetBuildings(cloned)
	return cloned
}
