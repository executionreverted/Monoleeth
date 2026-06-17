package visibility

import (
	"errors"
	"sort"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

var (
	// ErrInvalidPlanetIntel reports a malformed fog-memory planet summary.
	ErrInvalidPlanetIntel = errors.New("invalid planet intel summary")
)

// IntelSource names the server-side source that created a fog-memory summary.
type IntelSource string

const (
	IntelSourceScanner   IntelSource = "scanner"
	IntelSourceShared    IntelSource = "shared_intel"
	IntelSourceNarrative IntelSource = "narrative"
)

// PlanetIntelSummary is safe fog memory for a previously known planet.
//
// It deliberately omits hidden/live-only state such as current occupants,
// current resources, scan roll data, and procedural gameplay seeds.
type PlanetIntelSummary struct {
	PlanetID          foundation.PlanetID
	WorldID           world.WorldID
	ZoneID            world.ZoneID
	LastKnownPosition world.Vec2
	DisplayName       string
	Source            IntelSource
	UpdatedAt         time.Time
}

func (summary PlanetIntelSummary) valid() bool {
	if err := summary.PlanetID.Validate(); err != nil {
		return false
	}
	if err := summary.WorldID.Validate(); err != nil {
		return false
	}
	if err := summary.ZoneID.Validate(); err != nil {
		return false
	}
	if err := summary.LastKnownPosition.Validate(); err != nil {
		return false
	}
	if summary.Source == "" || summary.UpdatedAt.IsZero() {
		return false
	}
	return true
}

// FogMemory stores player-known summaries. It does not grant live visibility or
// interaction permission; callers must still use CanSendEntityToClient and
// CanInteract against live entity state.
type FogMemory struct {
	planets map[foundation.PlanetID]PlanetIntelSummary
}

// NewFogMemory returns an empty fog-memory store.
func NewFogMemory() *FogMemory {
	return &FogMemory{
		planets: make(map[foundation.PlanetID]PlanetIntelSummary),
	}
}

// RememberPlanet stores or replaces a known planet summary.
func (memory *FogMemory) RememberPlanet(summary PlanetIntelSummary) error {
	if memory == nil || !summary.valid() {
		return ErrInvalidPlanetIntel
	}
	if memory.planets == nil {
		memory.planets = make(map[foundation.PlanetID]PlanetIntelSummary)
	}
	memory.planets[summary.PlanetID] = summary
	return nil
}

// KnownPlanet returns a stored planet summary.
func (memory *FogMemory) KnownPlanet(planetID foundation.PlanetID) (PlanetIntelSummary, bool) {
	if memory == nil || memory.planets == nil {
		return PlanetIntelSummary{}, false
	}
	summary, ok := memory.planets[planetID]
	return summary, ok
}

// KnownPlanets returns known planet summaries in deterministic planet ID order.
func (memory *FogMemory) KnownPlanets() []PlanetIntelSummary {
	if memory == nil || len(memory.planets) == 0 {
		return nil
	}

	planetIDs := make([]foundation.PlanetID, 0, len(memory.planets))
	for planetID := range memory.planets {
		planetIDs = append(planetIDs, planetID)
	}
	sort.Slice(planetIDs, func(i, j int) bool {
		return planetIDs[i] < planetIDs[j]
	})

	summaries := make([]PlanetIntelSummary, 0, len(planetIDs))
	for _, planetID := range planetIDs {
		summaries = append(summaries, memory.planets[planetID])
	}
	return summaries
}

// ScannerBridgeEventType names scanner events that can update visibility-side
// memory. The skeleton defines event contracts only; scan roll mechanics and
// planet-generation truth belong outside this package.
type ScannerBridgeEventType string

const (
	ScannerEventPulseStarted     ScannerBridgeEventType = "scanner.pulse_started"
	ScannerEventPulseResolved    ScannerBridgeEventType = "scanner.pulse_resolved"
	ScannerEventSignalDetected   ScannerBridgeEventType = "scanner.signal_detected"
	ScannerEventPlanetDiscovered ScannerBridgeEventType = "scanner.planet_discovered"
)

// ScannerBridgeEvent is the visibility-facing scanner event shell.
type ScannerBridgeEvent struct {
	Type       ScannerBridgeEventType
	PlayerID   foundation.PlayerID
	WorldID    world.WorldID
	ZoneID     world.ZoneID
	PlanetID   foundation.PlanetID
	Position   world.Vec2
	OccurredAt time.Time
}
