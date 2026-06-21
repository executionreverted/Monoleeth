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

// ServerDetectionStats are server-calculated detection inputs. They are grouped
// so callers cannot accidentally replace the stat snapshot boundary with
// client-provided gameplay floats.
type ServerDetectionStats struct {
	detectionPower        float64
	scannerBonus          float64
	jammerResistance      float64
	stealthDetectionBonus float64
}

// DetectionStatsFromStatSnapshot extracts radar detection inputs from
// authoritative server stats.
func DetectionStatsFromStatSnapshot(snapshot stats.StatSnapshot) ServerDetectionStats {
	exploration := snapshot.Stats.Exploration
	return ServerDetectionStats{
		detectionPower:        exploration.DetectionPower,
		scannerBonus:          exploration.ScanPower,
		jammerResistance:      exploration.JammerResistance,
		stealthDetectionBonus: exploration.StealthDetectionBonus,
	}
}

// DetectionPower returns the server-owned detection strength.
func (detection ServerDetectionStats) DetectionPower() float64 {
	return detection.detectionPower
}

// ScannerBonus returns the server-owned scan/reveal detection bonus.
func (detection ServerDetectionStats) ScannerBonus() float64 {
	return detection.scannerBonus
}

// JammerResistance returns the server-owned counter-jammer strength.
func (detection ServerDetectionStats) JammerResistance() float64 {
	return detection.jammerResistance
}

// StealthDetectionBonus returns server-owned stealth-specific detection bonus.
func (detection ServerDetectionStats) StealthDetectionBonus() float64 {
	return detection.stealthDetectionBonus
}

func (detection ServerDetectionStats) valid() bool {
	return finiteNonNegative(detection.detectionPower) &&
		finiteNonNegative(detection.scannerBonus) &&
		finiteNonNegative(detection.jammerResistance) &&
		finiteNonNegative(detection.stealthDetectionBonus)
}

func (detection ServerDetectionStats) activeDetectionPower() float64 {
	return detection.detectionPower + detection.scannerBonus + detection.stealthDetectionBonus
}

// EntitySignature is the server-owned detection signature of a live entity.
type EntitySignature float64

const (
	EntitySignaturePlayer EntitySignature = 100
	EntitySignatureNPC    EntitySignature = 75
	EntitySignatureLoot   EntitySignature = 25
	EntitySignatureSignal EntitySignature = 50
)

const (
	hiddenDetectionThreshold       = 1
	detectionPenaltyAtRadarEdge    = 10
	defaultHiddenStealthOverSignal = 1
)

// Units returns the entity signature in world units.
func (signature EntitySignature) Units() float64 {
	return float64(signature)
}

func (signature EntitySignature) valid() bool {
	return finiteNonNegative(float64(signature))
}

// SignatureForEntityType returns a server-owned content signature for live AOI
// and interaction candidates. It is internal truth only and must not be copied
// into client payloads.
func SignatureForEntityType(entityType world.EntityType) EntitySignature {
	switch entityType {
	case world.EntityTypePlayer:
		return EntitySignaturePlayer
	case world.EntityTypeNPC:
		return EntitySignatureNPC
	case world.EntityTypeLoot:
		return EntitySignatureLoot
	case world.EntityTypePlanetSignal:
		return EntitySignatureSignal
	default:
		return 0
	}
}

// PlayerSignatureFromStatSnapshot prefers a ship/module signature stat when one
// exists and otherwise falls back to the content default for players.
func PlayerSignatureFromStatSnapshot(snapshot stats.StatSnapshot) EntitySignature {
	signature := snapshot.Stats.Exploration.SignatureRadius
	if signature > 0 && finiteNonNegative(signature) {
		return EntitySignature(signature)
	}
	return EntitySignaturePlayer
}

// Viewer is the server-owned visibility state for a player.
type Viewer struct {
	PlayerID       foundation.PlayerID
	WorldID        world.WorldID
	ZoneID         world.ZoneID
	Position       world.Vec2
	RadarRange     ServerRadarRange
	DetectionStats ServerDetectionStats
	Witnesses      []Witness
	ObservedAt     time.Time
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
	return viewer.RadarRange.valid() && viewer.DetectionStats.valid()
}

// Entity is the server-owned visibility state for a live world entity.
//
// This is an internal decision input, not a client payload shape.
type Entity struct {
	PlayerID       foundation.PlayerID
	WorldID        world.WorldID
	ZoneID         world.ZoneID
	ID             world.EntityID
	Position       world.Vec2
	Signature      EntitySignature
	StealthScore   float64
	JammerStrength float64
	Hidden         bool
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
	return entity.Signature.valid() &&
		finiteNonNegative(entity.StealthScore) &&
		finiteNonNegative(entity.JammerStrength)
}

// Witness is a viewer-specific temporary scanner reveal for one hidden player.
// It is an internal visibility input only and must not be serialized.
type Witness struct {
	TargetPlayerID foundation.PlayerID
	ExpiresAt      time.Time
}

// CanSendEntityToClient reports whether entity may be serialized to viewer.
//
// Hidden entities are rejected unless they are the viewer's own player entity,
// a hidden player covered by an active viewer-specific witness, or a target
// detected through server-owned radar/scanner stats. Normal entities must be in
// the same current map (represented by world/zone during migration), have valid
// server-owned visibility inputs, and be within the viewer radar range.
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
	if entity.Hidden {
		if hiddenEntityAllowed(viewer, entity) {
			return true
		}
		return DetectionForEntity(viewer, entity).Passed
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

// DetectionResult is an internal, testable detection-score result. It must not
// be serialized to gameplay clients.
type DetectionResult struct {
	Score     float64
	Threshold float64
	Passed    bool
}

// DetectionForEntity computes the hidden/stealthed entity detection result from
// server-owned viewer and target inputs.
func DetectionForEntity(viewer Viewer, entity Entity) DetectionResult {
	result := DetectionResult{Threshold: hiddenDetectionThreshold}
	if !viewer.valid() || !entity.valid() {
		return result
	}
	if viewer.WorldID != entity.WorldID || viewer.ZoneID != entity.ZoneID {
		return result
	}

	radarRange := viewer.RadarRange.Units()
	if radarRange <= 0 || viewer.Position.DistanceSquared(entity.Position) > radarRange*radarRange {
		return result
	}
	if viewer.DetectionStats.activeDetectionPower() <= 0 {
		return result
	}

	effectiveJammer := entity.JammerStrength - viewer.DetectionStats.JammerResistance()
	if effectiveJammer < 0 {
		effectiveJammer = 0
	}
	distancePenalty := detectionDistancePenalty(viewer, entity, radarRange)
	result.Score = viewer.DetectionStats.DetectionPower() +
		viewer.DetectionStats.ScannerBonus() +
		viewer.DetectionStats.StealthDetectionBonus() +
		entity.Signature.Units() -
		effectiveStealthScore(entity) -
		effectiveJammer -
		distancePenalty
	result.Passed = result.Score >= result.Threshold
	return result
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func effectiveStealthScore(entity Entity) float64 {
	if entity.StealthScore > 0 {
		return entity.StealthScore
	}
	if entity.Hidden {
		return entity.Signature.Units() + defaultHiddenStealthOverSignal
	}
	return 0
}

func detectionDistancePenalty(viewer Viewer, entity Entity, radarRange float64) float64 {
	if radarRange <= 0 {
		return 0
	}
	distance := viewer.Position.Distance(entity.Position)
	if distance <= 0 {
		return 0
	}
	return (distance / radarRange) * detectionPenaltyAtRadarEdge
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
