package discovery

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

func TestPlayerPlanetIntelValidateRequiresServerAuthoredFields(t *testing.T) {
	intel := testIntel("player-1", "planet-1", testTime(0), IntelStateFresh, 100, "scan-1")

	if err := intel.Validate(); err != nil {
		t.Fatalf("valid intel Validate() error = %v, want nil", err)
	}

	intel.LastSeenAt = time.Time{}
	if err := intel.Validate(); !errors.Is(err, ErrZeroIntelTimestamp) {
		t.Fatalf("zero last_seen_at Validate() error = %v, want %v", err, ErrZeroIntelTimestamp)
	}
}

func TestPlayerPlanetIntelValidateRejectsInvalidConfidence(t *testing.T) {
	intel := testIntel("player-1", "planet-1", testTime(0), IntelStateFresh, 101, "scan-1")

	if err := intel.Validate(); !errors.Is(err, ErrInvalidIntelConfidence) {
		t.Fatalf("confidence Validate() error = %v, want %v", err, ErrInvalidIntelConfidence)
	}
}

func testIntel(
	playerID foundation.PlayerID,
	planetID foundation.PlanetID,
	lastSeenAt time.Time,
	state IntelState,
	confidence int,
	sourceReference string,
) PlayerPlanetIntel {
	return PlayerPlanetIntel{
		PlayerID:        playerID,
		PlanetID:        planetID,
		Coordinates:     world.Vec2{X: 1200, Y: -450},
		State:           state,
		Confidence:      confidence,
		LastSeenAt:      lastSeenAt,
		SourceType:      IntelSourceScanSuccess,
		SourceReference: sourceReference,
	}
}
