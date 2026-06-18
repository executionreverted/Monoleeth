package production

import (
	"fmt"

	"gameproject/internal/game/crafting"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
)

// CraftPlanetStore is the discovery ownership boundary used by planet crafting.
type CraftPlanetStore interface {
	Planet(foundation.PlanetID) (discovery.Planet, bool, error)
}

// CraftProductionStore is the production/storage boundary used by planet
// crafting location authorization.
type CraftProductionStore interface {
	Snapshot(foundation.PlanetID) (PlanetProductionSnapshot, bool, error)
	Building(foundation.PlanetID, BuildingID) (PlanetBuilding, bool, error)
}

// CraftLocationAuthorizerConfig wires planet crafting authorization to
// authoritative discovery ownership and production building state.
type CraftLocationAuthorizerConfig struct {
	Planets    CraftPlanetStore
	Production CraftProductionStore
}

// CraftLocationAuthorizer validates non-station craft locations against
// server-owned planet ownership, storage, and building rows.
type CraftLocationAuthorizer struct {
	planets    CraftPlanetStore
	production CraftProductionStore
}

// NewCraftLocationAuthorizer returns a crafting authorizer backed by discovery
// and production state.
func NewCraftLocationAuthorizer(config CraftLocationAuthorizerConfig) (*CraftLocationAuthorizer, error) {
	if config.Planets == nil {
		return nil, fmt.Errorf("planets: %w", ErrInvalidCraftLocationAuthorizerConfig)
	}
	if config.Production == nil {
		return nil, fmt.Errorf("production: %w", ErrInvalidCraftLocationAuthorizerConfig)
	}
	return &CraftLocationAuthorizer{
		planets:    config.Planets,
		production: config.Production,
	}, nil
}

// AuthorizeCraftLocation implements crafting.CraftLocationAuthorizer.
func (authorizer *CraftLocationAuthorizer) AuthorizeCraftLocation(input crafting.CraftLocationAuthorizationInput) error {
	if authorizer == nil || authorizer.planets == nil || authorizer.production == nil {
		return ErrInvalidCraftLocationAuthorizerConfig
	}
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if err := input.Recipe.Validate(); err != nil {
		return err
	}
	if err := input.Recipe.ValidateLocationRequirement(input.Location); err != nil {
		return err
	}

	switch input.Location.Type {
	case crafting.CraftLocationStation, crafting.CraftLocationSpecialEventStation:
		return nil
	case crafting.CraftLocationOwnedPlanet:
		return authorizer.authorizeOwnedPlanet(input.PlayerID, foundation.PlanetID(input.Location.ID))
	case crafting.CraftLocationPlanetBuilding:
		return authorizer.authorizePlanetBuilding(input.PlayerID, input.Location.PlanetID, BuildingID(input.Location.ID))
	default:
		return input.Location.Type.Validate()
	}
}

func (authorizer *CraftLocationAuthorizer) authorizePlanetBuilding(
	playerID foundation.PlayerID,
	planetID foundation.PlanetID,
	buildingID BuildingID,
) error {
	if err := authorizer.authorizeOwnedPlanet(playerID, planetID); err != nil {
		return err
	}
	building, ok, err := authorizer.production.Building(planetID, buildingID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("building %q planet %q: %w", buildingID, planetID, ErrCraftBuildingNotFound)
	}
	if building.State != BuildingStateActive {
		return fmt.Errorf("building %q planet %q state %q: %w", buildingID, planetID, building.State, ErrCraftBuildingInactive)
	}
	return nil
}

func (authorizer *CraftLocationAuthorizer) authorizeOwnedPlanet(
	playerID foundation.PlayerID,
	planetID foundation.PlanetID,
) error {
	if err := planetID.Validate(); err != nil {
		return err
	}
	planet, ok, err := authorizer.planets.Planet(planetID)
	if err != nil {
		return err
	}
	if !ok || planet.OwnerPlayerID != playerID {
		return fmt.Errorf("planet %q: %w", planetID, ErrCraftPlanetNotOwned)
	}
	if _, ok, err := authorizer.production.Snapshot(planetID); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("planet %q: %w", planetID, ErrCraftPlanetProductionMissing)
	}
	return nil
}
