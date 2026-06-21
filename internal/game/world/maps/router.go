package maps

import (
	"errors"
	"fmt"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

var ErrLocationNotFound = errors.New("player map location not found")

// PlayerMapLocation is server-owned active map membership for one player.
type PlayerMapLocation struct {
	PlayerID      foundation.PlayerID
	WorldID       world.WorldID
	InternalMapID MapID
	PublicMapKey  PublicMapKey
	ZoneID        world.ZoneID
	SpawnID       SpawnID
	Position      world.Vec2
}

// Router resolves active map state without trusting client payload fields.
type Router struct {
	catalog   *Catalog
	locations map[foundation.PlayerID]PlayerMapLocation
}

func NewRouter(catalog *Catalog) (*Router, error) {
	if catalog == nil {
		return nil, ErrInvalidCatalog
	}
	if _, _, err := catalog.StarterDefinition(); err != nil {
		return nil, err
	}
	return &Router{
		catalog:   catalog,
		locations: make(map[foundation.PlayerID]PlayerMapLocation),
	}, nil
}

func (router *Router) Catalog() *Catalog {
	if router == nil {
		return nil
	}
	return router.catalog
}

// ActiveLocation returns the player's active map without creating or mutating it.
func (router *Router) ActiveLocation(playerID foundation.PlayerID) (PlayerMapLocation, error) {
	if router == nil || router.catalog == nil {
		return PlayerMapLocation{}, ErrInvalidCatalog
	}
	if err := playerID.Validate(); err != nil {
		return PlayerMapLocation{}, err
	}
	location, ok := router.locations[playerID]
	if !ok {
		return PlayerMapLocation{}, fmt.Errorf("player %q: %w", playerID, ErrLocationNotFound)
	}
	return location, nil
}

// EnsureStarterLocation creates starter location only when no active location
// exists. Reconnects and multitab bootstraps preserve server-owned map.
func (router *Router) EnsureStarterLocation(playerID foundation.PlayerID) (PlayerMapLocation, error) {
	if router == nil || router.catalog == nil {
		return PlayerMapLocation{}, ErrInvalidCatalog
	}
	if err := playerID.Validate(); err != nil {
		return PlayerMapLocation{}, err
	}
	if location, ok := router.locations[playerID]; ok {
		return location, nil
	}
	definition, spawn, err := router.catalog.StarterDefinition()
	if err != nil {
		return PlayerMapLocation{}, err
	}
	location := locationFromSpawn(playerID, definition, spawn)
	router.locations[playerID] = location
	return location, nil
}

// SetActiveLocationFromSpawn is a server-owned mutation hook for future portal
// transfer/persistence paths. It validates map, spawn, and bounded position.
func (router *Router) SetActiveLocationFromSpawn(playerID foundation.PlayerID, mapID MapID, spawnID SpawnID) (PlayerMapLocation, error) {
	if router == nil || router.catalog == nil {
		return PlayerMapLocation{}, ErrInvalidCatalog
	}
	if err := playerID.Validate(); err != nil {
		return PlayerMapLocation{}, err
	}
	definition, ok := router.catalog.Get(mapID)
	if !ok {
		return PlayerMapLocation{}, fmt.Errorf("map %q: %w", mapID, ErrMapNotFound)
	}
	spawn, ok := router.catalog.Spawn(mapID, spawnID)
	if !ok {
		return PlayerMapLocation{}, fmt.Errorf("spawn %q: %w", spawnID, ErrSpawnNotFound)
	}
	location := locationFromSpawn(playerID, definition, spawn)
	router.locations[playerID] = location
	return location, nil
}

func (router *Router) ClientProjection(playerID foundation.PlayerID) (ClientMapProjection, error) {
	location, err := router.ActiveLocation(playerID)
	if err != nil {
		return ClientMapProjection{}, err
	}
	projection, err := router.catalog.ClientProjection(location.InternalMapID)
	if err != nil {
		return ClientMapProjection{}, fmt.Errorf("active map %q: %w", location.InternalMapID, err)
	}
	return projection, nil
}

func (router *Router) ValidateActivePosition(playerID foundation.PlayerID, position world.Vec2) error {
	location, err := router.ActiveLocation(playerID)
	if err != nil {
		return err
	}
	return router.catalog.ValidatePosition(location.InternalMapID, position)
}

func locationFromSpawn(playerID foundation.PlayerID, definition MapDefinition, spawn SpawnPointDefinition) PlayerMapLocation {
	return PlayerMapLocation{
		PlayerID:      playerID,
		WorldID:       definition.WorldID,
		InternalMapID: definition.InternalMapID,
		PublicMapKey:  definition.PublicMapKey,
		ZoneID:        definition.ZoneID,
		SpawnID:       spawn.SpawnID,
		Position:      spawn.Position,
	}
}
