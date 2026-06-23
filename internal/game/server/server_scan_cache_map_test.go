package server

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestScanPulseReplayAfterMapTransferDoesNotReturnPreviousMapPayload(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "scan-cache-map@example.com", "Scan Cache Map", "map_1_2", "west_gate")
	request := []byte(`{"request_id":"request-scan-cache-map","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`)

	first := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), request)
	if first.HasError {
		t.Fatalf("first scan response error = %+v, want success", first.Error)
	}
	assertScanKnownPlanetMapKey(t, first, "1-2")

	gameServer.runtime.mu.Lock()
	if _, err := gameServer.runtime.mapRouter.SetActiveLocationFromSpawn(resolved.PlayerID, worldmaps.StarterMapID, worldmaps.StarterSpawnID); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("SetActiveLocationFromSpawn(%q) error = %v, want nil", worldmaps.StarterMapID, err)
	}
	gameServer.runtime.mu.Unlock()
	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("ensurePlayerSession(%q) error = %v, want nil", worldmaps.StarterMapID, err)
	}

	replay := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), request)
	if !replay.HasError {
		t.Fatalf("scan replay after map transfer response = %s, want safe rejection instead of cached previous-map payload", replay.Response.Payload)
	}
	if replay.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("scan replay after map transfer error = %+v, want %s", replay.Error.Error, foundation.CodeNotFound)
	}
}

func assertScanKnownPlanetMapKey(t *testing.T, response realtime.CachedResponse, want string) {
	t.Helper()
	var payload struct {
		KnownPlanets knownPlanetsPayload `json:"known_planets"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode scan payload: %v", err)
	}
	if len(payload.KnownPlanets.Planets) == 0 {
		t.Fatalf("known planets = %+v, want discovered planet in map %s", payload.KnownPlanets, want)
	}
	if got := payload.KnownPlanets.Planets[0].PublicMapKey; got != want {
		t.Fatalf("known planet public map key = %q, want %q", got, want)
	}
}
