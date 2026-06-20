package discovery

import (
	"errors"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
)

const (
	defaultScannerDiscoveryHorizon = 200_000
	defaultScannerSpawnBudget      = 3
	defaultScannerXPAmount         = 25
)

var (
	ErrInvalidScannerConfig     = errors.New("invalid scanner config")
	ErrInvalidScanPulse         = errors.New("invalid scan pulse")
	ErrScannerUnavailable       = errors.New("scanner unavailable")
	ErrScannerEnergyUnavailable = errors.New("scanner energy unavailable")
	ErrScanMovementRestricted   = errors.New("scan movement restricted")
	ErrScanCooldownActive       = errors.New("scanner cooldown active")
	ErrScanPulseNotFound        = errors.New("scan pulse not found")
	ErrScanPulseNotReady        = errors.New("scan pulse not ready")
	ErrInvalidScannerStats      = errors.New("invalid scanner stats")
	ErrInvalidScannerXPGrant    = errors.New("invalid scanner xp grant")
)

// ScanPulseReference identifies one server-scheduled scanner pulse.
type ScanPulseReference string

// ScanPulseStatus is the client-safe scanner pulse state/result family.
type ScanPulseStatus string

const (
	ScanPulseStatusStarted          ScanPulseStatus = "started"
	ScanPulseStatusNoSignal         ScanPulseStatus = "no_signal"
	ScanPulseStatusPlanetDiscovered ScanPulseStatus = "planet_discovered"
	ScanPulseStatusPlayerRevealed   ScanPulseStatus = "player_revealed"
)

// ScannerEventType names local scanner event records.
type ScannerEventType string

const (
	ScannerEventPulseStarted     ScannerEventType = "scan.pulse_started"
	ScannerEventPulseResolved    ScannerEventType = "scan.pulse_resolved"
	ScannerEventPlanetDiscovered ScannerEventType = "scan.planet_discovered"
	ScannerEventPlayerRevealed   ScannerEventType = "scan.player_revealed"
)

// StartScanPulseInput is the service-level command for scheduling a local MVP
// scanner pulse. Player, world, zone, ship, position, modules, cooldown, and
// stats are all validated server-side before the pulse is recorded.
type StartScanPulseInput struct {
	PlayerID       foundation.PlayerID `json:"player_id"`
	WorldID        foundation.WorldID  `json:"world_id"`
	ZoneID         foundation.ZoneID   `json:"zone_id"`
	ShipID         foundation.ShipID   `json:"ship_id"`
	PulseReference ScanPulseReference  `json:"pulse_reference"`
}

// StartScanPulseResult is safe to serialize to a client. It contains no hidden
// candidate truth or procedural generation data.
type StartScanPulseResult struct {
	PulseReference ScanPulseReference `json:"pulse_reference"`
	Status         ScanPulseStatus    `json:"status"`
	ResolveAfter   time.Time          `json:"resolve_after"`
}

// ResolveScanPulseInput resolves one server-owned pulse reference.
type ResolveScanPulseInput struct {
	PlayerID       foundation.PlayerID `json:"player_id"`
	WorldID        foundation.WorldID  `json:"world_id"`
	ZoneID         foundation.ZoneID   `json:"zone_id"`
	PulseReference ScanPulseReference  `json:"pulse_reference"`
}

// ResolveScanPulseResult is the client-safe scanner outcome. It intentionally
// omits candidate keys, exact coordinates, procedural seeds, and detection rolls.
type ResolveScanPulseResult struct {
	PulseReference ScanPulseReference         `json:"pulse_reference"`
	Status         ScanPulseStatus            `json:"status"`
	Message        string                     `json:"message,omitempty"`
	Signal         *CandidateSignalProjection `json:"signal,omitempty"`
	PlanetID       foundation.PlanetID        `json:"planet_id,omitempty"`
	XPGranted      bool                       `json:"xp_granted,omitempty"`
	Duplicate      bool                       `json:"duplicate,omitempty"`
}

// ScannerEventRecord is a local event/outbox-shaped record for scanner flows.
// It is deliberately coarse and does not include hidden coordinates or rolls.
type ScannerEventRecord struct {
	EventID        foundation.EventID  `json:"event_id"`
	Type           ScannerEventType    `json:"type"`
	PlayerID       foundation.PlayerID `json:"player_id"`
	WorldID        foundation.WorldID  `json:"world_id"`
	ZoneID         foundation.ZoneID   `json:"zone_id"`
	PulseReference ScanPulseReference  `json:"pulse_reference"`
	PlanetID       foundation.PlanetID `json:"planet_id,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
}

// ScannerPosition is authoritative zone-owned position state for a player.
type ScannerPosition struct {
	WorldID  foundation.WorldID  `json:"world_id"`
	ZoneID   foundation.ZoneID   `json:"zone_id"`
	Position world.Vec2          `json:"position"`
	Movement world.MovementState `json:"movement,omitempty"`
}

// ScannerModuleInput asks the module/loadout boundary whether a scanner is equipped.
type ScannerModuleInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	ShipID   foundation.ShipID   `json:"ship_id"`
}

// ScannerStatsInput asks for the current server stat snapshot.
type ScannerStatsInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	ShipID   foundation.ShipID   `json:"ship_id"`
}

// ScannerPositionInput asks the world/zone boundary for authoritative position.
type ScannerPositionInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	WorldID  foundation.WorldID  `json:"world_id"`
	ZoneID   foundation.ZoneID   `json:"zone_id"`
}

// ScannerCooldownInput starts a server-owned scanner cooldown if one is ready.
type ScannerCooldownInput struct {
	PlayerID       foundation.PlayerID `json:"player_id"`
	ShipID         foundation.ShipID   `json:"ship_id"`
	WorldID        foundation.WorldID  `json:"world_id"`
	ZoneID         foundation.ZoneID   `json:"zone_id"`
	PulseReference ScanPulseReference  `json:"pulse_reference"`
	StartedAt      time.Time           `json:"started_at"`
	Duration       time.Duration       `json:"duration"`
}

// ScannerCooldownResult reports whether a scanner cooldown was accepted.
type ScannerCooldownResult struct {
	Accepted bool      `json:"accepted"`
	ReadyAt  time.Time `json:"ready_at"`
}

// ScannerEnergyInput asks the ship/energy authority whether this scan has the
// capacitor/energy needed for one server-owned pulse.
//
// This is a read-only availability check. Durable spend/reserve behavior must
// happen in a runtime transaction that also owns cooldown and pulse creation.
type ScannerEnergyInput struct {
	PlayerID       foundation.PlayerID  `json:"player_id"`
	ShipID         foundation.ShipID    `json:"ship_id"`
	WorldID        foundation.WorldID   `json:"world_id"`
	ZoneID         foundation.ZoneID    `json:"zone_id"`
	PulseReference ScanPulseReference   `json:"pulse_reference"`
	CheckedAt      time.Time            `json:"checked_at"`
	Stats          stats.EffectiveStats `json:"stats"`
}

// ScannerEnergyResult reports whether scanner energy was available.
type ScannerEnergyResult struct {
	Accepted bool `json:"accepted"`
}

// ScannerPlayerRevealInput asks a runtime-owned scanner bridge to reveal one
// hidden live player, if server rules allow it. The result must not expose the
// target identity to the scanner service or client payload.
type ScannerPlayerRevealInput struct {
	PlayerID       foundation.PlayerID  `json:"player_id"`
	ShipID         foundation.ShipID    `json:"ship_id"`
	WorldID        foundation.WorldID   `json:"world_id"`
	ZoneID         foundation.ZoneID    `json:"zone_id"`
	PulseReference ScanPulseReference   `json:"pulse_reference"`
	Position       world.Vec2           `json:"position"`
	Stats          stats.EffectiveStats `json:"stats"`
	RevealedAt     time.Time            `json:"revealed_at"`
}

// ScannerPlayerRevealResult is intentionally targetless. Hidden target ids,
// expiry, scan rolls, and coordinates stay inside the runtime visibility bridge.
type ScannerPlayerRevealResult struct {
	Revealed bool `json:"revealed"`
	NoSignal bool `json:"no_signal,omitempty"`
}

// ScanXPGrantInput is the narrow discovery-to-progression handoff.
type ScanXPGrantInput struct {
	PlayerID       foundation.PlayerID          `json:"player_id"`
	Amount         int64                        `json:"amount"`
	SourceType     progression.XPSourceType     `json:"source_type"`
	SourceID       progression.XPSourceID       `json:"source_id"`
	IdempotencyKey progression.XPIdempotencyKey `json:"idempotency_key"`
	Authority      progression.XPGrantAuthority `json:"-"`
	RoleXP         []progression.RoleXPGrant    `json:"role_xp,omitempty"`
}

// ScanXPGrantResult reports duplicate-safe XP handling.
type ScanXPGrantResult struct {
	Duplicate bool `json:"duplicate"`
}

type ScannerModuleProvider interface {
	HasEquippedScannerModule(input ScannerModuleInput) (bool, error)
}

type ScannerStatsProvider interface {
	ScanStats(input ScannerStatsInput) (stats.StatSnapshot, error)
}

type ScannerPositionProvider interface {
	PlayerScanPosition(input ScannerPositionInput) (ScannerPosition, error)
}

type ScannerCooldownProvider interface {
	StartScanCooldown(input ScannerCooldownInput) (ScannerCooldownResult, error)
}

// ScannerEnergyProvider checks scanner energy availability without mutating
// durable ship energy/capacitor state.
type ScannerEnergyProvider interface {
	CheckScanEnergy(input ScannerEnergyInput) (ScannerEnergyResult, error)
}

type ScannerPlayerRevealProvider interface {
	RevealHiddenPlayer(input ScannerPlayerRevealInput) (ScannerPlayerRevealResult, error)
}

type ScanXPGrantProvider interface {
	GrantScanXP(input ScanXPGrantInput) (ScanXPGrantResult, error)
}

// ScannerServiceConfig wires scanner service boundaries without depending on
// concrete module, stats, world, cooldown, or progression packages.
type ScannerServiceConfig struct {
	Store     *InMemoryStore
	WorldSeed WorldSeed
	Clock     foundation.Clock
	Modules   ScannerModuleProvider
	Stats     ScannerStatsProvider
	Positions ScannerPositionProvider
	Cooldowns ScannerCooldownProvider
	Energy    ScannerEnergyProvider
	Reveals   ScannerPlayerRevealProvider
	XP        ScanXPGrantProvider

	CandidateOptions  CandidateGenerationOptions
	ScanCellSize      float64
	ChunkSize         float64
	RadarLevelUnit    float64
	DiscoveryXPAmount int64
}

// ScannerService owns local MVP scanner pulse state and discovery resolution.
type ScannerService struct {
	mu sync.Mutex

	store     *InMemoryStore
	seed      WorldSeed
	clock     foundation.Clock
	modules   ScannerModuleProvider
	stats     ScannerStatsProvider
	positions ScannerPositionProvider
	cooldowns ScannerCooldownProvider
	energy    ScannerEnergyProvider
	reveals   ScannerPlayerRevealProvider
	xp        ScanXPGrantProvider

	candidateOptions  CandidateGenerationOptions
	scanCellSize      float64
	chunkSize         float64
	radarLevelUnit    float64
	discoveryXPAmount int64

	pulses  map[ScanPulseReference]scanPulse
	results map[ScanPulseReference]ResolveScanPulseResult
	events  []ScannerEventRecord
}

type scanPulse struct {
	reference    ScanPulseReference
	playerID     foundation.PlayerID
	worldID      foundation.WorldID
	zoneID       foundation.ZoneID
	shipID       foundation.ShipID
	position     world.Vec2
	cell         ScanCellCoord
	stats        stats.EffectiveStats
	startedAt    time.Time
	resolveAfter time.Time
}
