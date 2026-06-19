package world

import (
	"errors"
	"fmt"
	"math"

	"gameproject/internal/game/foundation"
)

// WorldID identifies a persistent world.
type WorldID = foundation.WorldID

// ZoneID identifies the authoritative zone worker that owns live simulation.
type ZoneID = foundation.ZoneID

// EntityID identifies a live world entity.
type EntityID = foundation.EntityID

var (
	// ErrInvalidEntityType reports an unsupported live world entity kind.
	ErrInvalidEntityType = errors.New("invalid entity type")

	// ErrInvalidPosition reports a non-finite world coordinate.
	ErrInvalidPosition = errors.New("invalid world position")
)

// EntityType identifies the kind of entity owned by a zone worker.
type EntityType string

const (
	EntityTypePlayer       EntityType = "player"
	EntityTypeNPC          EntityType = "npc"
	EntityTypeLoot         EntityType = "loot"
	EntityTypePlanetSignal EntityType = "planet_signal"

	// Deprecated aliases keep older domain code compiling while the Phase 04
	// public client contract uses the non-placeholder entity type names above.
	EntityTypeNPCPlaceholder          EntityType = EntityTypeNPC
	EntityTypeLootPlaceholder         EntityType = EntityTypeLoot
	EntityTypePlanetSignalPlaceholder EntityType = EntityTypePlanetSignal
)

// String returns the stable entity type representation.
func (entityType EntityType) String() string {
	return string(entityType)
}

// Validate reports whether entityType is supported by the Phase 04 MVP world.
func (entityType EntityType) Validate() error {
	switch entityType {
	case EntityTypePlayer,
		EntityTypeNPC,
		EntityTypeLoot,
		EntityTypePlanetSignal:
		return nil
	default:
		return fmt.Errorf("entity type %q: %w", entityType, ErrInvalidEntityType)
	}
}

// Vec2 is a position or offset in world units.
type Vec2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Validate reports whether pos contains finite coordinates.
func (pos Vec2) Validate() error {
	if !isFinite(pos.X) || !isFinite(pos.Y) {
		return fmt.Errorf("position (%v, %v): %w", pos.X, pos.Y, ErrInvalidPosition)
	}
	return nil
}

// DistanceSquared returns the squared distance from pos to other.
func (pos Vec2) DistanceSquared(other Vec2) float64 {
	return DistanceSquared(pos, other)
}

// Distance returns the Euclidean distance from pos to other.
func (pos Vec2) Distance(other Vec2) float64 {
	return Distance(pos, other)
}

// DistanceSquared returns the squared distance between two positions.
func DistanceSquared(a Vec2, b Vec2) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	return dx*dx + dy*dy
}

// Distance returns the Euclidean distance between two positions.
func Distance(a Vec2, b Vec2) float64 {
	return math.Sqrt(DistanceSquared(a, b))
}

// Entity is a live world object owned by a zone worker.
type Entity struct {
	WorldID  WorldID       `json:"world_id"`
	ZoneID   ZoneID        `json:"zone_id"`
	ID       EntityID      `json:"entity_id"`
	Type     EntityType    `json:"entity_type"`
	Position Vec2          `json:"position"`
	Movement MovementState `json:"movement"`
}

// NewEntity validates and returns a live world entity.
func NewEntity(worldID WorldID, zoneID ZoneID, id EntityID, entityType EntityType, position Vec2) (Entity, error) {
	entity := Entity{
		WorldID:  worldID,
		ZoneID:   zoneID,
		ID:       id,
		Type:     entityType,
		Position: position,
	}
	if err := entity.Validate(); err != nil {
		return Entity{}, err
	}
	return entity, nil
}

// Validate reports whether entity has a valid identity, type, position, and movement state.
func (entity Entity) Validate() error {
	if err := entity.WorldID.Validate(); err != nil {
		return err
	}
	if err := entity.ZoneID.Validate(); err != nil {
		return err
	}
	if err := entity.ID.Validate(); err != nil {
		return err
	}
	if err := entity.Type.Validate(); err != nil {
		return err
	}
	if err := entity.Position.Validate(); err != nil {
		return err
	}
	if err := entity.Movement.Validate(); err != nil {
		return err
	}
	return nil
}

// MovementState records the server-owned target for an entity's current movement.
type MovementState struct {
	Moving bool `json:"moving"`
	Target Vec2 `json:"target"`
}

// NewMovementState validates and returns an active movement state.
func NewMovementState(target Vec2) (MovementState, error) {
	state := MovementState{
		Moving: true,
		Target: target,
	}
	if err := state.Validate(); err != nil {
		return MovementState{}, err
	}
	return state, nil
}

// Validate reports whether state contains a valid server-owned movement target.
func (state MovementState) Validate() error {
	return state.Target.Validate()
}

// MovementIntent is the client request shape for movement.
//
// It intentionally contains only the requested target. The server supplies
// current position, speed, tick delta, and the resulting authoritative position.
type MovementIntent struct {
	Target Vec2 `json:"target"`
}

// NewMovementIntent validates and returns a movement intent.
func NewMovementIntent(target Vec2) (MovementIntent, error) {
	intent := MovementIntent{Target: target}
	if err := intent.Validate(); err != nil {
		return MovementIntent{}, err
	}
	return intent, nil
}

// Validate reports whether intent contains a valid target.
func (intent MovementIntent) Validate() error {
	return intent.Target.Validate()
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
