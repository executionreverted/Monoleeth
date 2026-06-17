package discovery

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

func TestPlanetValidateRequiresPersistentDiscoveryFields(t *testing.T) {
	planet := testPlanet("planet-1", testTime(0))

	if err := planet.Validate(); err != nil {
		t.Fatalf("valid planet Validate() error = %v, want nil", err)
	}

	planet.DiscoveredAt = time.Time{}
	if err := planet.Validate(); !errors.Is(err, ErrZeroPlanetTimestamp) {
		t.Fatalf("zero discovered_at Validate() error = %v, want %v", err, ErrZeroPlanetTimestamp)
	}
}

func TestPlanetValidateRequiresConsistentOwnerFields(t *testing.T) {
	planet := testPlanet("planet-1", testTime(0))
	planet.OwnerPlayerID = "player-owner"

	if err := planet.Validate(); !errors.Is(err, ErrZeroPlanetTimestamp) {
		t.Fatalf("owner without timestamp Validate() error = %v, want %v", err, ErrZeroPlanetTimestamp)
	}

	changedAt := testTime(1)
	planet.OwnerChangedAt = &changedAt
	if err := planet.Validate(); err != nil {
		t.Fatalf("owned planet Validate() error = %v, want nil", err)
	}
}

func testPlanet(id foundation.PlanetID, discoveredAt time.Time) Planet {
	return Planet{
		ID:           id,
		WorldID:      "world-1",
		ZoneID:       "zone-1",
		Coordinates:  world.Vec2{X: 1200, Y: -450},
		Biome:        PlanetBiomeOuterDrift,
		Type:         PlanetTypeTerrestrial,
		Rarity:       PlanetRarityRare,
		Level:        7,
		DiscoveredAt: discoveredAt,
		DiscoveredBy: "player-scout",
	}
}

func testTime(offsetMinutes int) time.Time {
	return time.Date(2026, 6, 17, 12, offsetMinutes, 0, 0, time.UTC)
}
