package visibility

import (
	"errors"
	"math"
	"time"

	"gameproject/internal/game/foundation"
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
	PlayerID   foundation.PlayerID
	WorldID    world.WorldID
	ZoneID     world.ZoneID
	Position   world.Vec2
	RadarRange ServerRadarRange
	Witnesses  []Witness
	ObservedAt time.Time
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
	PlayerID  foundation.PlayerID
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

// Witness is a viewer-specific temporary scanner reveal for one hidden player.
// It is an internal visibility input only and must not be serialized.
type Witness struct {
	TargetPlayerID foundation.PlayerID
	ExpiresAt      time.Time
}

// CanSendEntityToClient reports whether entity may be serialized to viewer.
//
// Hidden entities are rejected unless they are the viewer's own player entity
// or a hidden player covered by an active viewer-specific witness. Normal
// entities must be in the same world and zone, have valid server-owned
// visibility inputs, and be within the viewer radar range.
func CanSendEntityToClient(viewer Viewer, entity Entity) bool {
	if !viewer.valid() || !entity.valid() {
		return false
	}
	if viewer.WorldID != entity.WorldID || viewer.ZoneID != entity.ZoneID {
		return false
	}

	radarRange := viewer.RadarRange.Units()
	if viewer.Position.DistanceSquared(entity.Position) > radarRange*radarRange {
		return false
	}
	if entity.Hidden && !hiddenEntityAllowed(viewer, entity) {
		return false
	}
	return true
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

func hiddenEntityAllowed(viewer Viewer, entity Entity) bool {
	if entity.PlayerID.IsZero() {
		return false
	}
	if viewer.PlayerID == entity.PlayerID {
		return true
	}
	for _, witness := range viewer.Witnesses {
		if witness.TargetPlayerID != entity.PlayerID {
			continue
		}
		if witness.ExpiresAt.IsZero() {
			continue
		}
		if !viewer.ObservedAt.IsZero() && !witness.ExpiresAt.After(viewer.ObservedAt) {
			continue
		}
		return true
	}
	return false
}
