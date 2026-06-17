package visibility

import (
	"errors"
	"math"

	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
)

var (
	// ErrNotVisible is safe to return for hidden, out-of-range, or unknown
	// targets when the client must not learn which case occurred.
	ErrNotVisible = errors.New("no visible entity found")
)

// ServerRadarRange is a radar range sourced from a server-calculated stat
// snapshot. It intentionally cannot be populated with an arbitrary float field.
type ServerRadarRange struct {
	units float64
}

// RadarRangeFromStatSnapshot extracts the viewer radar range from authoritative
// server stats. Clients may display stat snapshots, but they must not submit
// radar range as gameplay truth.
func RadarRangeFromStatSnapshot(snapshot stats.StatSnapshot) ServerRadarRange {
	return ServerRadarRange{units: snapshot.Stats.Exploration.RadarRange}
}

// Units returns the radar range in world units.
func (radarRange ServerRadarRange) Units() float64 {
	return radarRange.units
}

func (radarRange ServerRadarRange) valid() bool {
	return finiteNonNegative(radarRange.units)
}

// EntitySignature is the server-owned detection signature of a live entity.
//
// The Phase 04 skeleton validates the value but does not implement stealth,
// jammer, scan roll, or signature-vs-radar mechanics yet.
type EntitySignature float64

// Units returns the entity signature in world units.
func (signature EntitySignature) Units() float64 {
	return float64(signature)
}

func (signature EntitySignature) valid() bool {
	return finiteNonNegative(float64(signature))
}

// Viewer is the server-owned visibility state for a player.
type Viewer struct {
	WorldID    world.WorldID
	ZoneID     world.ZoneID
	Position   world.Vec2
	RadarRange ServerRadarRange
}

func (viewer Viewer) valid() bool {
	if err := viewer.WorldID.Validate(); err != nil {
		return false
	}
	if err := viewer.ZoneID.Validate(); err != nil {
		return false
	}
	if err := viewer.Position.Validate(); err != nil {
		return false
	}
	return viewer.RadarRange.valid()
}

// Entity is the server-owned visibility state for a live world entity.
//
// This is an internal decision input, not a client payload shape.
type Entity struct {
	WorldID   world.WorldID
	ZoneID    world.ZoneID
	ID        world.EntityID
	Position  world.Vec2
	Signature EntitySignature
	Hidden    bool
}

func (entity Entity) valid() bool {
	if err := entity.WorldID.Validate(); err != nil {
		return false
	}
	if err := entity.ZoneID.Validate(); err != nil {
		return false
	}
	if err := entity.ID.Validate(); err != nil {
		return false
	}
	if err := entity.Position.Validate(); err != nil {
		return false
	}
	return entity.Signature.valid()
}

// CanSendEntityToClient reports whether entity may be serialized to viewer.
//
// Hidden entities are always rejected. Normal entities must be in the same
// world and zone, have valid server-owned visibility inputs, and be within the
// viewer radar range.
func CanSendEntityToClient(viewer Viewer, entity Entity) bool {
	if !viewer.valid() || !entity.valid() {
		return false
	}
	if entity.Hidden {
		return false
	}
	if viewer.WorldID != entity.WorldID || viewer.ZoneID != entity.ZoneID {
		return false
	}

	radarRange := viewer.RadarRange.Units()
	return viewer.Position.DistanceSquared(entity.Position) <= radarRange*radarRange
}

// CanInteract reports whether viewer may issue a live interaction against
// entity. It intentionally returns the same generic error for hidden,
// out-of-range, cross-zone, and invalid targets to avoid hidden-data oracles.
func CanInteract(viewer Viewer, entity Entity) error {
	if !CanSendEntityToClient(viewer, entity) {
		return ErrNotVisible
	}
	return nil
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}
