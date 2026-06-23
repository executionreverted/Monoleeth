package server

import (
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

func TestPlaytestSeedGrantsClaimAndRouteOnboardingState(t *testing.T) {
	gameServer, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		PlaytestSeed:   true,
	})
	if err != nil {
		t.Fatalf("New(playtest seed) error = %v, want nil", err)
	}

	first := createResolvedRuntimeSession(t, gameServer, "playtest-seed-one@example.com", "Playtest One")
	second := createResolvedRuntimeSession(t, gameServer, "playtest-seed-two@example.com", "Playtest Two")

	assertPlaytestSeedState(t, gameServer, first.PlayerID)
	assertPlaytestSeedState(t, gameServer, second.PlayerID)
	if playtestRoutePlanetID(first.PlayerID, "source") == playtestRoutePlanetID(second.PlayerID, "source") {
		t.Fatalf("playtest route source ids matched for different players")
	}

	if err := gameServer.runtime.ensurePlayerSession(first); err != nil {
		t.Fatalf("ensurePlayerSession retry error = %v, want nil", err)
	}
	assertPlaytestSeedState(t, gameServer, first.PlayerID)
}

func TestPlaytestSeedIgnoresE2EClaimCoreMatrixQuantity(t *testing.T) {
	gameServer, err := New(Config{
		AllowedOrigins:      []string{testOrigin},
		PlaytestSeed:        true,
		E2EPlanetClaimCores: 2,
	})
	if err != nil {
		t.Fatalf("New(playtest seed) error = %v, want nil", err)
	}

	resolved := createResolvedRuntimeSession(t, gameServer, "playtest-seed-cores@example.com", "Playtest Cores")
	if got := inventoryStackQuantityForTest(gameServer, resolved.PlayerID, "x_core"); got != 1 {
		t.Fatalf("x_core quantity = %d, want playtest default 1", got)
	}
}

func assertPlaytestSeedState(t *testing.T, gameServer *Server, playerID foundation.PlayerID) {
	t.Helper()

	if got := inventoryStackQuantityForTest(gameServer, playerID, "x_core"); got != 1 {
		t.Fatalf("x_core quantity = %d, want 1 with playtest seed", got)
	}
	snapshot, err := gameServer.runtime.Progression.GetProgressionSnapshot(playerID)
	if err != nil {
		t.Fatalf("GetProgressionSnapshot() error = %v, want nil", err)
	}
	if snapshot.Player.Rank != progression.MaxMVPRank {
		t.Fatalf("progression rank = %d, want %d with playtest seed", snapshot.Player.Rank, progression.MaxMVPRank)
	}

	sourceID := playtestRoutePlanetID(playerID, "source")
	destinationID := playtestRoutePlanetID(playerID, "destination")
	for _, planetID := range []foundation.PlanetID{sourceID, destinationID} {
		planet, ok, err := gameServer.runtime.Discovery.Planet(planetID)
		if err != nil || !ok {
			t.Fatalf("Discovery.Planet(%q) ok=%v err=%v, want playtest seeded", planetID, ok, err)
		}
		if planet.OwnerPlayerID != playerID {
			t.Fatalf("planet %q owner = %q, want %q", planetID, planet.OwnerPlayerID, playerID)
		}
	}
	storage, ok, err := gameServer.runtime.Production.PlanetStorage(sourceID)
	if err != nil || !ok {
		t.Fatalf("PlanetStorage(%q) ok=%v err=%v, want playtest seeded", sourceID, ok, err)
	}
	if got := storage.QuantityOf("refined_alloy"); got != 160 {
		t.Fatalf("refined_alloy quantity = %d, want 160 with playtest seed", got)
	}
}
