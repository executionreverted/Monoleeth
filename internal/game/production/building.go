package production

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

// BuildingID identifies one constructed building on a planet.
type BuildingID string

// PlanetBuilding records durable state for one building installed on a planet.
type PlanetBuilding struct {
	BuildingID     BuildingID                  `json:"building_id"`
	PlanetID       foundation.PlanetID         `json:"planet_id"`
	Source         catalog.VersionedDefinition `json:"source"`
	BuildingType   BuildingType                `json:"building_type"`
	Level          int                         `json:"level"`
	State          BuildingState               `json:"state"`
	DisabledReason string                      `json:"disabled_reason,omitempty"`
	CreatedAt      time.Time                   `json:"created_at"`
	UpdatedAt      time.Time                   `json:"updated_at"`
}

// NewPlanetBuilding validates and returns a durable building row from a
// production catalog definition.
func NewPlanetBuilding(
	buildingID BuildingID,
	planetID foundation.PlanetID,
	definition BuildingProductionDefinition,
	state BuildingState,
	createdAt time.Time,
	updatedAt time.Time,
) (PlanetBuilding, error) {
	if err := definition.Validate(); err != nil {
		return PlanetBuilding{}, err
	}
	building := PlanetBuilding{
		BuildingID:   buildingID,
		PlanetID:     planetID,
		Source:       definition.Source,
		BuildingType: definition.BuildingType,
		Level:        definition.Level,
		State:        state,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
	if err := building.Validate(); err != nil {
		return PlanetBuilding{}, err
	}
	return clonePlanetBuilding(building), nil
}

// Validate reports whether id is a non-blank building id.
func (id BuildingID) Validate() error {
	if strings.TrimSpace(string(id)) == "" || string(id) != strings.TrimSpace(string(id)) {
		return fmt.Errorf("building id %q: %w", id, ErrInvalidBuildingID)
	}
	return nil
}

// String returns the stable building id representation.
func (id BuildingID) String() string { return string(id) }

// Validate reports whether building state is complete and consistent.
func (building PlanetBuilding) Validate() error {
	if err := building.BuildingID.Validate(); err != nil {
		return err
	}
	if err := building.PlanetID.Validate(); err != nil {
		return err
	}
	if err := building.Source.Validate(); err != nil {
		return err
	}
	if err := building.BuildingType.Validate(); err != nil {
		return err
	}
	if building.Level <= 0 {
		return fmt.Errorf("building level %d: %w", building.Level, ErrInvalidBuildingLevel)
	}
	if err := building.State.Validate(); err != nil {
		return err
	}
	if building.CreatedAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrZeroProductionTimestamp)
	}
	if building.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at: %w", ErrZeroProductionTimestamp)
	}
	if building.UpdatedAt.Before(building.CreatedAt) {
		return fmt.Errorf("updated_at before created_at: %w", ErrInvalidProductionState)
	}
	return nil
}

// Clone returns a detached building copy.
func (building PlanetBuilding) Clone() PlanetBuilding {
	return clonePlanetBuilding(building)
}

func clonePlanetBuilding(building PlanetBuilding) PlanetBuilding {
	building.CreatedAt = building.CreatedAt.UTC()
	building.UpdatedAt = building.UpdatedAt.UTC()
	return building
}

func sortPlanetBuildings(buildings []PlanetBuilding) {
	sort.Slice(buildings, func(i, j int) bool {
		return buildings[i].BuildingID < buildings[j].BuildingID
	})
}
