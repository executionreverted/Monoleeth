package server

import (
	"strings"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestE2ERouteSeedAbsentByDefault(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	resolved := createResolvedRuntimeSession(t, gameServer, "route-seed-default@example.com", "RouteSeedDefault")

	sourceID := e2eRoutePlanetID(resolved.PlayerID, "source")
	if _, ok, err := gameServer.runtime.Discovery.Planet(sourceID); err != nil || ok {
		t.Fatalf("E2E route source planet ok=%v err=%v, want absent by default", ok, err)
	}
}

func TestE2ERouteSeedCreatesOwnedPlanetsAndStorageIdempotently(t *testing.T) {
	gameServer, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		DevMode:        true,
		E2ERouteSeed:   true,
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	resolved := createResolvedRuntimeSession(t, gameServer, "route-seed-enabled@example.com", "RouteSeedEnabled")
	assertE2ERouteSeedState(t, gameServer, resolved.PlayerID, 160)

	sourceID := e2eRoutePlanetID(resolved.PlayerID, "source")
	storage, ok, err := gameServer.runtime.Production.PlanetStorage(sourceID)
	if err != nil || !ok {
		t.Fatalf("PlanetStorage(%q) ok=%v err=%v, want seeded", sourceID, ok, err)
	}
	if removed, err := storage.RemoveUpTo("refined_alloy", 25, gameServer.runtime.clock.Now().UTC()); err != nil || removed != 25 {
		t.Fatalf("RemoveUpTo(refined_alloy) removed=%d err=%v, want 25 nil", removed, err)
	}
	if err := gameServer.runtime.Production.SavePlanetStorage(storage); err != nil {
		t.Fatalf("SavePlanetStorage(mutated source) error = %v, want nil", err)
	}

	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("ensurePlayerSession second call error = %v, want nil", err)
	}
	assertE2ERouteSeedState(t, gameServer, resolved.PlayerID, 135)
}

func assertE2ERouteSeedState(t *testing.T, gameServer *Server, playerID foundation.PlayerID, expectedSourceAlloy int64) {
	t.Helper()

	sourceID := e2eRoutePlanetID(playerID, "source")
	destinationID := e2eRoutePlanetID(playerID, "destination")
	for _, planetID := range []foundation.PlanetID{sourceID, destinationID} {
		assertE2ERoutePlanetIDClientSafe(t, planetID, playerID)
		planet, ok, err := gameServer.runtime.Discovery.Planet(planetID)
		if err != nil || !ok {
			t.Fatalf("Discovery.Planet(%q) ok=%v err=%v, want seeded", planetID, ok, err)
		}
		if planet.OwnerPlayerID != playerID {
			t.Fatalf("planet %q owner = %q, want %q", planetID, planet.OwnerPlayerID, playerID)
		}
		intel, ok, err := gameServer.runtime.Discovery.PlayerPlanetIntel(playerID, planetID)
		if err != nil || !ok {
			t.Fatalf("PlayerPlanetIntel(%q) ok=%v err=%v, want seeded", planetID, ok, err)
		}
		if intel.PlayerID != playerID || intel.PlanetID != planetID {
			t.Fatalf("intel = %+v, want player %q planet %q", intel, playerID, planetID)
		}
	}
	storage, ok, err := gameServer.runtime.Production.PlanetStorage(sourceID)
	if err != nil || !ok {
		t.Fatalf("PlanetStorage(%q) ok=%v err=%v, want seeded", sourceID, ok, err)
	}
	if got := storage.QuantityOf("refined_alloy"); got != expectedSourceAlloy {
		t.Fatalf("refined_alloy quantity = %d, want %d", got, expectedSourceAlloy)
	}
}

func assertE2ERoutePlanetIDClientSafe(t *testing.T, planetID foundation.PlanetID, playerID foundation.PlayerID) {
	t.Helper()

	planetIDRaw := planetID.String()
	playerIDRaw := playerID.String()
	if strings.Contains(planetIDRaw, playerIDRaw) {
		t.Fatalf("route planet id %q leaked raw player id %q", planetIDRaw, playerIDRaw)
	}
	if strings.Contains(playerIDRaw, "player-") && strings.Contains(planetIDRaw, "player-") {
		t.Fatalf("route planet id %q leaked raw player id prefix from %q", planetIDRaw, playerIDRaw)
	}
}
