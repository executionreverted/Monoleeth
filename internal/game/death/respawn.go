package death

import (
	"errors"
	"fmt"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

var (
	ErrInvalidRespawnLocationKind = errors.New("invalid respawn location kind")
	ErrInvalidRespawnLocation     = errors.New("invalid respawn location")
	ErrRespawnOriginRequired      = errors.New("respawn origin required")
)

// RespawnLocationKind identifies the server-owned category used when choosing
// where a player reappears after death.
type RespawnLocationKind string

const (
	RespawnLocationKindCheckpoint    RespawnLocationKind = "checkpoint"
	RespawnLocationKindOwnedPlanet   RespawnLocationKind = "owned_planet"
	RespawnLocationKindSafeStation   RespawnLocationKind = "safe_station"
	RespawnLocationKindOriginStation RespawnLocationKind = "origin_station"
)

// RespawnLocation is one server-known respawn target.
type RespawnLocation struct {
	ID       RespawnLocationID   `json:"id"`
	Kind     RespawnLocationKind `json:"kind"`
	Position world.Vec2          `json:"position"`
}

// SelectRespawnLocationInput is the authoritative context used by RespawnService.
type SelectRespawnLocationInput struct {
	PlayerID       foundation.PlayerID `json:"player_id"`
	DeathPosition  world.Vec2          `json:"death_position"`
	LastCheckpoint *RespawnLocation    `json:"last_checkpoint,omitempty"`
	OwnedPlanets   []RespawnLocation   `json:"owned_planets,omitempty"`
	SafeStations   []RespawnLocation   `json:"safe_stations,omitempty"`
	Origin         RespawnLocation     `json:"origin"`
}

// RespawnSelection reports the chosen target and why it won.
type RespawnSelection struct {
	Location       RespawnLocation `json:"location"`
	PriorityIndex  int             `json:"priority_index"`
	OriginFallback bool            `json:"origin_fallback"`
}

// RespawnService owns Phase 06 respawn location selection.
type RespawnService struct {
	origin RespawnLocation
}

// RespawnConfig configures the origin fallback for RespawnService.
type RespawnConfig struct {
	Origin RespawnLocation
}

// NewRespawnService returns a server-owned respawn selector.
func NewRespawnService(config RespawnConfig) (*RespawnService, error) {
	if err := validateRespawnLocation(config.Origin, RespawnLocationKindOriginStation); err != nil {
		return nil, fmt.Errorf("origin: %w", err)
	}
	return &RespawnService{origin: config.Origin}, nil
}

// SelectLocation chooses the highest-priority available respawn location.
func (service *RespawnService) SelectLocation(input SelectRespawnLocationInput) (RespawnSelection, error) {
	if service == nil {
		return RespawnSelection{}, ErrRespawnOriginRequired
	}
	input.Origin = service.origin
	return SelectRespawnLocation(input)
}

// DefaultRespawnPriority returns the documented respawn priority order.
func DefaultRespawnPriority() []RespawnLocationKind {
	return []RespawnLocationKind{
		RespawnLocationKindCheckpoint,
		RespawnLocationKindOwnedPlanet,
		RespawnLocationKindSafeStation,
		RespawnLocationKindOriginStation,
	}
}

// SelectRespawnLocation chooses a respawn target using the Phase 06 priority:
// last checkpoint, nearest owned planet, nearest safe station, then origin.
func SelectRespawnLocation(input SelectRespawnLocationInput) (RespawnSelection, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return RespawnSelection{}, err
	}
	if err := input.DeathPosition.Validate(); err != nil {
		return RespawnSelection{}, err
	}
	if err := validateRespawnLocation(input.Origin, RespawnLocationKindOriginStation); err != nil {
		return RespawnSelection{}, fmt.Errorf("origin: %w", err)
	}

	if input.LastCheckpoint != nil {
		if err := validateRespawnLocation(*input.LastCheckpoint, RespawnLocationKindCheckpoint); err != nil {
			return RespawnSelection{}, fmt.Errorf("last_checkpoint: %w", err)
		}
		return RespawnSelection{Location: *input.LastCheckpoint, PriorityIndex: 0}, nil
	}

	if location, ok, err := nearestRespawnLocation(input.DeathPosition, input.OwnedPlanets, RespawnLocationKindOwnedPlanet); err != nil {
		return RespawnSelection{}, err
	} else if ok {
		return RespawnSelection{Location: location, PriorityIndex: 1}, nil
	}

	if location, ok, err := nearestRespawnLocation(input.DeathPosition, input.SafeStations, RespawnLocationKindSafeStation); err != nil {
		return RespawnSelection{}, err
	} else if ok {
		return RespawnSelection{Location: location, PriorityIndex: 2}, nil
	}

	return RespawnSelection{
		Location:       input.Origin,
		PriorityIndex:  3,
		OriginFallback: true,
	}, nil
}

func nearestRespawnLocation(deathPosition world.Vec2, locations []RespawnLocation, kind RespawnLocationKind) (RespawnLocation, bool, error) {
	var selected RespawnLocation
	var selectedDistance float64
	for index, location := range locations {
		if err := validateRespawnLocation(location, kind); err != nil {
			return RespawnLocation{}, false, fmt.Errorf("%s[%d]: %w", kind, index, err)
		}
		distance := deathPosition.DistanceSquared(location.Position)
		if selected.ID.IsZero() ||
			distance < selectedDistance ||
			(distance == selectedDistance && location.ID.String() < selected.ID.String()) {
			selected = location
			selectedDistance = distance
		}
	}
	if selected.ID.IsZero() {
		return RespawnLocation{}, false, nil
	}
	return selected, true, nil
}

func validateRespawnLocation(location RespawnLocation, kind RespawnLocationKind) error {
	if err := location.ID.Validate(); err != nil {
		return fmt.Errorf("id: %w", err)
	}
	if err := location.Kind.Validate(); err != nil {
		return err
	}
	if location.Kind != kind {
		return fmt.Errorf("kind %q want %q: %w", location.Kind, kind, ErrInvalidRespawnLocation)
	}
	if err := location.Position.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether kind is one of the supported respawn categories.
func (kind RespawnLocationKind) Validate() error {
	switch kind {
	case RespawnLocationKindCheckpoint,
		RespawnLocationKindOwnedPlanet,
		RespawnLocationKindSafeStation,
		RespawnLocationKindOriginStation:
		return nil
	default:
		return fmt.Errorf("respawn location kind %q: %w", kind, ErrInvalidRespawnLocationKind)
	}
}
