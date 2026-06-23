package server

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/realtime"
)

func TestE2EScanNoPlanetSeedReturnsNoSignalWithoutPlanetMutation(t *testing.T) {
	gameServer, err := New(Config{
		AllowedOrigins:      []string{testOrigin},
		DevMode:             true,
		E2EScanNoPlanetSeed: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	resolved := createResolvedRuntimeSession(t, gameServer, "scan-no-planet@example.com", "Scan No Planet")

	raw := gatewayJSON(t, gameServer, resolved, "request-scan-no-planet", realtime.OperationScanPulse, map[string]any{}, 1)
	assertPayloadOmitsScannerNoFogTruth(t, "scan no-planet response", raw)

	var payload struct {
		Scan         scanPulsePayload    `json:"scan"`
		KnownPlanets knownPlanetsPayload `json:"known_planets"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode scan no-planet response: %v", err)
	}
	if payload.Scan.Status != string(discovery.ScanPulseStatusNoSignal) {
		t.Fatalf("scan status = %q, want %q", payload.Scan.Status, discovery.ScanPulseStatusNoSignal)
	}
	if payload.Scan.PlanetID != "" || payload.Scan.Signal != nil || payload.Scan.XPGranted {
		t.Fatalf("scan payload = %+v, want no planet, no signal, no XP", payload.Scan)
	}
	if payload.KnownPlanets.Counts.Known != 0 || len(payload.KnownPlanets.Planets) != 0 {
		t.Fatalf("known planets = %+v, want empty no-planet result", payload.KnownPlanets)
	}
	if planets := gameServer.runtime.Discovery.Planets(); len(planets) != 0 {
		t.Fatalf("materialized planets = %+v, want none for no-planet seed", planets)
	}

	events, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationScanPulse, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post scan no-planet events: %v", err)
	}
	for _, event := range events {
		assertPayloadOmitsScannerNoFogTruth(t, string(event.Type)+" no-planet event", mustJSON(t, event))
		if event.Type == realtime.EventScanPlanetDiscovered {
			t.Fatalf("unexpected planet discovered event for no-planet scan: %+v", event)
		}
	}
}
