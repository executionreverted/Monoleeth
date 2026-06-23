package server

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestPhase07DiscoveryProductionRouteQueriesUseServerState(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-scan-pulse","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`)
	scanResponse := readResponse(t, conn)
	if !scanResponse.OK {
		t.Fatalf("scan response = %+v, want success", scanResponse)
	}
	assertPayloadOmitsScannerNoFogTruth(t, "scan response", scanResponse.Payload, `"coordinates"`, `"x"`, `"y"`)
	var scanPayload struct {
		Scan         scanPulsePayload           `json:"scan"`
		KnownPlanets knownPlanetsPayload        `json:"known_planets"`
		Progression  progressionSnapshotPayload `json:"progression"`
	}
	if err := json.Unmarshal(scanResponse.Payload, &scanPayload); err != nil {
		t.Fatalf("decode scan payload: %v", err)
	}
	if scanPayload.Scan.Status != string(discovery.ScanPulseStatusPlanetDiscovered) || scanPayload.Scan.PlanetID == "" || scanPayload.Scan.Signal == nil {
		t.Fatalf("scan payload = %+v, want discovered planet with safe signal", scanPayload.Scan)
	}
	if !scanPayload.Scan.XPGranted || scanPayload.Progression.MainXP < 25 {
		t.Fatalf("scan progression = %+v scan=%+v, want scanner XP grant", scanPayload.Progression, scanPayload.Scan)
	}
	if len(scanPayload.KnownPlanets.Planets) != 1 || scanPayload.KnownPlanets.Counts.Known != 1 {
		t.Fatalf("known planets = %+v, want one server-authored intel row", scanPayload.KnownPlanets)
	}
	if scanPayload.KnownPlanets.Planets[0].PublicMapKey != "1-1" {
		t.Fatalf("known planet public map key = %q, want 1-1", scanPayload.KnownPlanets.Planets[0].PublicMapKey)
	}
	planetID := scanPayload.Scan.PlanetID

	seen := map[realtime.ClientEventType]bool{}
	var knownEventPayload struct {
		Planets []knownPlanetPayload `json:"planets"`
		Counts  planetIntelCounts    `json:"counts"`
		Minimap minimapPayload       `json:"minimap"`
	}
	for attempts := 0; attempts < 6 && (!seen[realtime.EventScanPulseStarted] || !seen[realtime.EventScanPulseResolved] || !seen[realtime.EventScanPlanetDiscovered] || !seen[realtime.EventKnownPlanets]); attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
		assertPayloadOmitsScannerNoFogTruth(t, string(event.Type)+" event", mustJSON(t, event))
		if event.Type == realtime.EventKnownPlanets {
			if err := json.Unmarshal(event.Payload, &knownEventPayload); err != nil {
				t.Fatalf("decode known planets event: %v", err)
			}
		}
	}
	for _, want := range []realtime.ClientEventType{realtime.EventScanPulseStarted, realtime.EventScanPulseResolved, realtime.EventScanPlanetDiscovered, realtime.EventKnownPlanets} {
		if !seen[want] {
			t.Fatalf("scan events seen = %#v, missing %s", seen, want)
		}
	}
	if len(knownEventPayload.Minimap.Remembered) != 1 {
		t.Fatalf("known planets event remembered minimap = %+v, want discovered planet memory without manual world.snapshot", knownEventPayload.Minimap.Remembered)
	}
	eventMemory := knownEventPayload.Minimap.Remembered[0]
	if eventMemory.Kind != "known_planet" || eventMemory.PlanetID != planetID || eventMemory.DetailID != planetID {
		t.Fatalf("known planets event memory = %+v, want known planet %s", eventMemory, planetID)
	}
	if eventMemory.PublicMapKey != "1-1" {
		t.Fatalf("known planets event memory public map key = %q, want 1-1", eventMemory.PublicMapKey)
	}
	if eventMemory.Position.X == 0 && eventMemory.Position.Y == 0 {
		t.Fatalf("known planets event memory position = %+v, want client-safe discovered coordinates", eventMemory.Position)
	}

	writeText(t, conn, `{"request_id":"request-known-planets","op":"discovery.known_planets","payload":{},"client_seq":2,"v":1}`)
	knownResponse := readResponse(t, conn)
	if !knownResponse.OK {
		t.Fatalf("known planets response = %+v, want success", knownResponse)
	}
	var knownPayload struct {
		KnownPlanets knownPlanetsPayload `json:"known_planets"`
		Minimap      minimapPayload      `json:"minimap"`
	}
	if err := json.Unmarshal(knownResponse.Payload, &knownPayload); err != nil {
		t.Fatalf("decode known planets: %v", err)
	}
	if len(knownPayload.KnownPlanets.Planets) != 1 || knownPayload.KnownPlanets.Planets[0].PlanetID != planetID {
		t.Fatalf("known planets response = %+v, want discovered planet %s", knownPayload.KnownPlanets, planetID)
	}
	if knownPayload.KnownPlanets.Planets[0].PublicMapKey != "1-1" {
		t.Fatalf("known planets response public map key = %q, want 1-1", knownPayload.KnownPlanets.Planets[0].PublicMapKey)
	}
	if len(knownPayload.Minimap.Remembered) != 1 || knownPayload.Minimap.Remembered[0].PlanetID != planetID {
		t.Fatalf("known planets remembered minimap = %+v, want discovered planet %s", knownPayload.Minimap.Remembered, planetID)
	}
	if knownPayload.Minimap.Remembered[0].PublicMapKey != "1-1" {
		t.Fatalf("known planets remembered public map key = %q, want 1-1", knownPayload.Minimap.Remembered[0].PublicMapKey)
	}

	writeText(t, conn, `{"request_id":"request-planet-detail","op":"discovery.planet_detail","payload":{"planet_id":"`+planetID+`"},"client_seq":3,"v":1}`)
	detailResponse := readResponse(t, conn)
	if !detailResponse.OK {
		t.Fatalf("planet detail response = %+v, want success", detailResponse)
	}
	if raw := string(detailResponse.Payload); strings.Contains(raw, "owner_player_id") || strings.Contains(raw, "player_id") || strings.Contains(raw, "candidate_key") {
		t.Fatalf("planet detail leaked hidden/server-owned fields: %s", raw)
	}
	var detailPayload struct {
		PlanetDetail planetDetailPayload `json:"planet_detail"`
	}
	if err := json.Unmarshal(detailResponse.Payload, &detailPayload); err != nil {
		t.Fatalf("decode planet detail: %v", err)
	}
	if detailPayload.PlanetDetail.PlanetID != planetID || !detailPayload.PlanetDetail.ProductionLocked {
		t.Fatalf("planet detail = %+v, want discovered unclaimed planet with locked production", detailPayload.PlanetDetail)
	}
	if detailPayload.PlanetDetail.Coordinates.X == 0 && detailPayload.PlanetDetail.Coordinates.Y == 0 {
		t.Fatalf("planet detail coordinates = %+v, want discovered intel coordinates", detailPayload.PlanetDetail.Coordinates)
	}

	writeText(t, conn, `{"request_id":"request-world-snapshot-fog","op":"world.snapshot","payload":{},"client_seq":4,"v":1}`)
	worldResponse := readResponse(t, conn)
	if !worldResponse.OK {
		t.Fatalf("world snapshot response = %+v, want success", worldResponse)
	}
	assertPayloadOmitsScannerNoFogTruth(t, "world snapshot response", worldResponse.Payload)
	var worldPayload worldSnapshotPayload
	if err := json.Unmarshal(worldResponse.Payload, &worldPayload); err != nil {
		t.Fatalf("decode world snapshot: %v", err)
	}
	if len(worldPayload.Minimap.Remembered) != 1 {
		t.Fatalf("remembered minimap = %+v, want one known planet memory", worldPayload.Minimap.Remembered)
	}
	memory := worldPayload.Minimap.Remembered[0]
	if memory.Kind != "known_planet" || memory.Label == "" || memory.Freshness == "" {
		t.Fatalf("remembered minimap memory = %+v, want client-safe known planet summary", memory)
	}
	if memory.PlanetID != planetID || memory.DetailID != planetID {
		t.Fatalf("remembered minimap memory ids = %+v, want planet/detail %s", memory, planetID)
	}
	if memory.PublicMapKey != "1-1" {
		t.Fatalf("remembered minimap public map key = %q, want 1-1", memory.PublicMapKey)
	}
	if memory.Position.X != detailPayload.PlanetDetail.Coordinates.X || memory.Position.Y != detailPayload.PlanetDetail.Coordinates.Y {
		t.Fatalf("remembered minimap position = %+v, want detail coordinates %+v", memory.Position, detailPayload.PlanetDetail.Coordinates)
	}
	for _, contact := range worldPayload.Minimap.LiveContacts {
		if contact.EntityID == "entity_hidden_planet_signal" {
			t.Fatalf("hidden entity leaked into minimap contact %+v", contact)
		}
	}

	for _, request := range []struct {
		name string
		body string
	}{
		{name: "production", body: `{"request_id":"request-production-summary","op":"planet.production_summary","payload":{},"client_seq":5,"v":1}`},
		{name: "storage", body: `{"request_id":"request-storage-summary","op":"planet.storage_summary","payload":{},"client_seq":6,"v":1}`},
		{name: "routes", body: `{"request_id":"request-route-list","op":"route.list","payload":{},"client_seq":7,"v":1}`},
	} {
		t.Run(request.name, func(t *testing.T) {
			writeText(t, conn, request.body)
			response := readResponse(t, conn)
			if !response.OK {
				t.Fatalf("%s response = %+v, want success", request.name, response)
			}
			raw := string(response.Payload)
			for _, forbidden := range []string{"owner_player_id", "player_id", "world_id", "zone_id", "procedural_seed", "candidate_key"} {
				if strings.Contains(raw, forbidden) {
					t.Fatalf("%s response leaked %q in %s", request.name, forbidden, raw)
				}
			}
		})
	}

	writeText(t, conn, `{"request_id":"request-scan-spoof","op":"scan.pulse","payload":{"player_id":"spoofed-player","candidate_key":"forced","procedural_seed":"leak","scan_result":"planet"},"client_seq":8,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed scan error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}
}

func TestScanPulseUsesActiveDestinationMapScope(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "scan-map-two@example.com", "Scan Map Two", "map_1_2", "west_gate")

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-scan-map-two","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("scan.pulse response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsScannerNoFogTruth(t, "map_1_2 scan response", response.Response.Payload)
	assertPayloadOmitsActiveMapInternalTruth(t, "map_1_2 scan response", response.Response.Payload)

	var scanPayload struct {
		Scan         scanPulsePayload           `json:"scan"`
		KnownPlanets knownPlanetsPayload        `json:"known_planets"`
		Progression  progressionSnapshotPayload `json:"progression"`
	}
	if err := json.Unmarshal(response.Response.Payload, &scanPayload); err != nil {
		t.Fatalf("decode map_1_2 scan response: %v", err)
	}

	events, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationScanPulse, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post map_1_2 scan events: %v", err)
	}
	for _, event := range events {
		assertPayloadOmitsScannerNoFogTruth(t, string(event.Type)+" map_1_2 event", mustJSON(t, event))
		assertPayloadOmitsActiveMapInternalTruth(t, string(event.Type)+" map_1_2 event", mustJSON(t, event))
	}

	switch scanPayload.Scan.Status {
	case string(discovery.ScanPulseStatusPlanetDiscovered):
		if scanPayload.Scan.PlanetID == "" || scanPayload.Scan.Signal == nil {
			t.Fatalf("scan payload = %+v, want discovered planet with safe signal", scanPayload.Scan)
		}
		if len(scanPayload.KnownPlanets.Planets) != 1 || scanPayload.KnownPlanets.Planets[0].PlanetID != scanPayload.Scan.PlanetID {
			t.Fatalf("known planets = %+v, want only discovered planet %s", scanPayload.KnownPlanets, scanPayload.Scan.PlanetID)
		}
		if scanPayload.KnownPlanets.Planets[0].PublicMapKey != "1-2" {
			t.Fatalf("known planet public map key = %q, want 1-2", scanPayload.KnownPlanets.Planets[0].PublicMapKey)
		}
		assertScanMapTwoKnownEvent(t, events, scanPayload.Scan.PlanetID)
		assertScanMapTwoKnownPlanetsQuery(t, gameServer, resolved, scanPayload.Scan.PlanetID)
		assertScanMapTwoPlanetDetail(t, gameServer, resolved, scanPayload.Scan.PlanetID)
		assertScanMapTwoWorldSnapshot(t, gameServer, resolved, scanPayload.Scan.PlanetID)
	case string(discovery.ScanPulseStatusNoSignal):
		if scanPayload.Scan.PlanetID != "" || scanPayload.Scan.Signal != nil || scanPayload.Scan.XPGranted {
			t.Fatalf("no-signal scan payload = %+v, want no planet/intel/xp truth", scanPayload.Scan)
		}
		if len(scanPayload.KnownPlanets.Planets) != 0 || scanPayload.KnownPlanets.Counts.Known != 0 {
			t.Fatalf("no-signal known planets = %+v, want none", scanPayload.KnownPlanets)
		}
		if planets := gameServer.runtime.Discovery.Planets(); len(planets) != 0 {
			t.Fatalf("no-signal scan planets = %d, want no discovery mutation", len(planets))
		}
		intel, err := gameServer.runtime.Discovery.PlayerPlanetIntelRecords(resolved.PlayerID)
		if err != nil {
			t.Fatalf("PlayerPlanetIntelRecords() error = %v, want nil", err)
		}
		if len(intel) != 0 {
			t.Fatalf("no-signal scan intel records = %d, want none", len(intel))
		}
		assertScanMapTwoWorldSnapshot(t, gameServer, resolved, "")
	default:
		t.Fatalf("scan status = %q, want planet_discovered or no_signal", scanPayload.Scan.Status)
	}
}

func TestPlanetDetailRejectsHiddenPlanetWithSafeError(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "hidden-detail@example.com", "Hidden Detail")
	planetID := foundation.PlanetID("planet-hidden-detail")
	if _, err := gameServer.runtime.Discovery.MaterializePlanet(discovery.MaterializePlanetInput{
		CandidateKey: "candidate-hidden-detail",
		Planet: discovery.Planet{
			ID:           planetID,
			WorldID:      gameServer.runtime.worldID,
			ZoneID:       gameServer.runtime.zoneID,
			Coordinates:  world.Vec2{X: 1400, Y: 1500},
			Biome:        discovery.PlanetBiomeOriginBelt,
			Type:         discovery.PlanetTypeTerrestrial,
			Rarity:       discovery.PlanetRarityCommon,
			Level:        1,
			DiscoveredAt: gameServer.runtime.clock.Now().UTC(),
			DiscoveredBy: "player-other",
		},
	}); err != nil {
		t.Fatalf("MaterializePlanet(hidden): %v", err)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-hidden-planet-detail","op":"discovery.planet_detail","payload":{"planet_id":"`+planetID.String()+`"},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("hidden planet detail response = %+v, want safe not found", response)
	}
	intelRows, err := gameServer.runtime.Discovery.PlayerPlanetIntelRecords(resolved.PlayerID)
	if err != nil {
		t.Fatalf("PlayerPlanetIntelRecords() error = %v, want nil", err)
	}
	if len(intelRows) != 0 {
		t.Fatalf("hidden detail intel rows = %+v, want no mutation", intelRows)
	}
	gameServer.runtime.mu.Lock()
	queuedEvents := len(gameServer.runtime.queuedEvents[resolved.SessionID])
	gameServer.runtime.mu.Unlock()
	if queuedEvents != 0 {
		t.Fatalf("hidden detail queued events = %d, want none", queuedEvents)
	}
}

func assertScanMapTwoKnownEvent(t *testing.T, events []realtime.EventEnvelope, planetID string) {
	t.Helper()
	for _, event := range events {
		if event.Type != realtime.EventKnownPlanets {
			continue
		}
		var payload struct {
			Planets []knownPlanetPayload `json:"planets"`
			Counts  planetIntelCounts    `json:"counts"`
			Minimap minimapPayload       `json:"minimap"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode known planets event: %v", err)
		}
		if len(payload.Planets) != 1 || payload.Planets[0].PlanetID != planetID || payload.Planets[0].PublicMapKey != "1-2" {
			t.Fatalf("known planets event = %+v, want planet %s on public map 1-2", payload.Planets, planetID)
		}
		if len(payload.Minimap.Remembered) != 1 || payload.Minimap.Remembered[0].PlanetID != planetID || payload.Minimap.Remembered[0].PublicMapKey != "1-2" {
			t.Fatalf("known planets event minimap = %+v, want planet %s on public map 1-2", payload.Minimap.Remembered, planetID)
		}
		return
	}
	t.Fatalf("events = %+v, missing known planets event", events)
}

func assertScanMapTwoKnownPlanetsQuery(t *testing.T, gameServer *Server, resolved auth.ResolvedSession, planetID string) {
	t.Helper()
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-known-planets-map-two","op":"discovery.known_planets","payload":{},"client_seq":2,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("discovery.known_planets response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsScannerNoFogTruth(t, "map_1_2 known planets response", response.Response.Payload)
	assertPayloadOmitsActiveMapInternalTruth(t, "map_1_2 known planets response", response.Response.Payload)
	var payload struct {
		KnownPlanets knownPlanetsPayload `json:"known_planets"`
		Minimap      minimapPayload      `json:"minimap"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode known planets response: %v", err)
	}
	if len(payload.KnownPlanets.Planets) != 1 || payload.KnownPlanets.Planets[0].PlanetID != planetID || payload.KnownPlanets.Planets[0].PublicMapKey != "1-2" {
		t.Fatalf("known planets response = %+v, want planet %s on public map 1-2", payload.KnownPlanets.Planets, planetID)
	}
	if len(payload.Minimap.Remembered) != 1 || payload.Minimap.Remembered[0].PlanetID != planetID || payload.Minimap.Remembered[0].PublicMapKey != "1-2" {
		t.Fatalf("known planets minimap = %+v, want planet %s on public map 1-2", payload.Minimap.Remembered, planetID)
	}
}

func assertScanMapTwoPlanetDetail(t *testing.T, gameServer *Server, resolved auth.ResolvedSession, planetID string) {
	t.Helper()
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-planet-detail-map-two","op":"discovery.planet_detail","payload":{"planet_id":"`+planetID+`"},"client_seq":3,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("discovery.planet_detail response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsScannerNoFogTruth(t, "map_1_2 planet detail response", response.Response.Payload)
	assertPayloadOmitsActiveMapInternalTruth(t, "map_1_2 planet detail response", response.Response.Payload)
	var payload struct {
		PlanetDetail planetDetailPayload `json:"planet_detail"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode planet detail response: %v", err)
	}
	if payload.PlanetDetail.PlanetID != planetID || payload.PlanetDetail.PublicMapKey != "1-2" {
		t.Fatalf("planet detail = %+v, want planet %s on public map 1-2", payload.PlanetDetail, planetID)
	}
	if payload.PlanetDetail.Coordinates.X == 0 && payload.PlanetDetail.Coordinates.Y == 0 {
		t.Fatalf("planet detail coordinates = %+v, want discovered map-local coordinates", payload.PlanetDetail.Coordinates)
	}
}

func assertScanMapTwoWorldSnapshot(t *testing.T, gameServer *Server, resolved auth.ResolvedSession, planetID string) {
	t.Helper()
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-world-snapshot-map-two","op":"world.snapshot","payload":{},"client_seq":4,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("world.snapshot response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsScannerNoFogTruth(t, "map_1_2 world snapshot response", response.Response.Payload)
	assertPayloadOmitsActiveMapInternalTruth(t, "map_1_2 world snapshot response", response.Response.Payload)
	var snapshot worldSnapshotPayload
	if err := json.Unmarshal(response.Response.Payload, &snapshot); err != nil {
		t.Fatalf("decode world snapshot response: %v", err)
	}
	if snapshot.Map.PublicMapKey != "1-2" {
		t.Fatalf("world snapshot map = %+v, want public map 1-2", snapshot.Map)
	}
	if planetID == "" {
		if len(snapshot.Minimap.Remembered) != 0 {
			t.Fatalf("no-signal remembered minimap = %+v, want none", snapshot.Minimap.Remembered)
		}
		return
	}
	if len(snapshot.Minimap.Remembered) != 1 {
		t.Fatalf("remembered minimap = %+v, want only discovered destination-map planet", snapshot.Minimap.Remembered)
	}
	memory := snapshot.Minimap.Remembered[0]
	if memory.PlanetID != planetID || memory.DetailID != planetID || memory.PublicMapKey != "1-2" || memory.SectorKey == "1-1" {
		t.Fatalf("remembered minimap memory = %+v, want planet %s only on public map 1-2", memory, planetID)
	}
}

func TestScanPulseSpendsStarterShipCapacitorOnce(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)

	writeText(t, conn, `{"request_id":"request-scan-spend","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("scan response = %+v, want success", response)
	}
	if capacitor := testShipCapacitor(gameServer, resolved.PlayerID); capacitor != 100-starterScannerEnergyCost {
		t.Fatalf("scan capacitor = %d, want one spend to %d", capacitor, 100-starterScannerEnergyCost)
	}
}
func TestScanPulseRejectsInsufficientCapacitorBeforeDiscoveryMutation(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	setTestShipCapacitor(gameServer, resolved.PlayerID, starterScannerEnergyCost-1)

	writeText(t, conn, `{"request_id":"request-scan-low-cap","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeNotEnoughEnergy {
		t.Fatalf("low-capacitor scan error = %+v, want %s", got.Error, foundation.CodeNotEnoughEnergy)
	}
	if capacitor := testShipCapacitor(gameServer, resolved.PlayerID); capacitor != starterScannerEnergyCost-1 {
		t.Fatalf("low-capacitor scan capacitor = %d, want unchanged", capacitor)
	}
	if planets := gameServer.runtime.Discovery.Planets(); len(planets) != 0 {
		t.Fatalf("low-capacitor scan planets = %d, want no discovery mutation", len(planets))
	}
	intel, err := gameServer.runtime.Discovery.PlayerPlanetIntelRecords(resolved.PlayerID)
	if err != nil {
		t.Fatalf("PlayerPlanetIntelRecords() error = %v, want nil", err)
	}
	if len(intel) != 0 {
		t.Fatalf("low-capacitor scan intel records = %d, want none", len(intel))
	}
	if events := gameServer.runtime.Scanner.Events(); len(events) != 0 {
		t.Fatalf("low-capacitor scan events = %d, want none", len(events))
	}
}

func TestScanPulseRejectsMovingPlayerBeforeDiscoveryMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "scan-moving@example.com", "Scan Moving")
	initialCapacitor := testShipCapacitor(gameServer, resolved.PlayerID)

	move := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-scan-moving-move","op":"move_to","payload":{"target":{"x":100,"y":0}},"client_seq":1,"v":1}`),
	)
	if move.HasError {
		t.Fatalf("move_to response error = %+v, want success", move.Error)
	}
	gameServer.runtime.mu.Lock()
	instance, _, err := gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("activeMapInstance() error = %v, want nil", err)
	}
	entity, ok := instance.Worker.PlayerEntity(resolved.PlayerID)
	gameServer.runtime.mu.Unlock()
	if !ok || !entity.Movement.Moving {
		t.Fatalf("player entity = %+v ok=%v, want moving before scan", entity, ok)
	}

	scan := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-scan-moving","op":"scan.pulse","payload":{},"client_seq":2,"v":1}`),
	)
	if !scan.HasError || scan.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("moving scan response = %+v, want forbidden", scan)
	}
	if capacitor := testShipCapacitor(gameServer, resolved.PlayerID); capacitor != initialCapacitor {
		t.Fatalf("moving scan capacitor = %d, want unchanged %d", capacitor, initialCapacitor)
	}
	if planets := gameServer.runtime.Discovery.Planets(); len(planets) != 0 {
		t.Fatalf("moving scan planets = %d, want no discovery mutation", len(planets))
	}
	intel, err := gameServer.runtime.Discovery.PlayerPlanetIntelRecords(resolved.PlayerID)
	if err != nil {
		t.Fatalf("PlayerPlanetIntelRecords() error = %v, want nil", err)
	}
	if len(intel) != 0 {
		t.Fatalf("moving scan intel records = %d, want none", len(intel))
	}
	if events := gameServer.runtime.Scanner.Events(); len(events) != 0 {
		t.Fatalf("moving scan events = %d, want none", len(events))
	}
}

func TestScanPulseRejectsTrustedClientScannerFields(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		payload string
	}{
		{name: "map", payload: `{"map_id":"map_1_2"}`},
		{name: "position", payload: `{"position":{"x":1,"y":1}}`},
		{name: "candidate", payload: `{"candidate_key":"forced"}`},
		{name: "radar", payload: `{"radar_range":9999}`},
		{name: "cooldown", payload: `{"cooldown":0}`},
		{name: "energy", payload: `{"energy":999,"capacitor":999}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			assertScanPulsePayloadRejectedBeforeMutation(t, "trusted-"+fixture.name, fixture.payload)
		})
	}
}

func TestScanPulseRejectsEmptyIntentAliasFields(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		payload string
	}{
		{name: "x", payload: `{"x":100}`},
		{name: "y", payload: `{"y":200}`},
		{name: "target", payload: `{"target":{"x":100,"y":200}}`},
		{name: "planet_id", payload: `{"planet_id":"planet-forced"}`},
		{name: "radar", payload: `{"radar":{"range":9999}}`},
		{name: "scanner_power", payload: `{"scanner_power":9999}`},
		{name: "discovery_result", payload: `{"discovery_result":{"planet_id":"planet-forced"}}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			assertScanPulsePayloadRejectedBeforeMutation(t, "alias-"+fixture.name, fixture.payload)
		})
	}
}

func TestScanPulseRejectsArbitraryUnknownFields(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		payload string
	}{
		{name: "scalar", payload: `{"client_note":"scan now"}`},
		{name: "nested_object", payload: `{"unknown":{"nested":true}}`},
		{name: "nested_array_object", payload: `{"items":[{"unknown":1}]}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			assertScanPulsePayloadRejectedBeforeMutation(t, "unknown-"+fixture.name, fixture.payload)
		})
	}
}

func assertScanPulsePayloadRejectedBeforeMutation(t *testing.T, requestSuffix string, payload string) {
	t.Helper()
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	beforeProgression, err := gameServer.runtime.Progression.GetProgressionSnapshot(resolved.PlayerID)
	if err != nil {
		t.Fatalf("GetProgressionSnapshot(before) error = %v, want nil", err)
	}
	beforeCapacitor := testShipCapacitor(gameServer, resolved.PlayerID)

	writeText(t, conn, `{"request_id":"request-scan-`+requestSuffix+`","op":"scan.pulse","payload":`+payload+`,"client_seq":1,"v":1}`)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("scan payload %s error = %+v, want %s", payload, got.Error, foundation.CodeInvalidPayload)
	}
	if capacitor := testShipCapacitor(gameServer, resolved.PlayerID); capacitor != beforeCapacitor {
		t.Fatalf("scan payload %s capacitor = %d, want unchanged %d", payload, capacitor, beforeCapacitor)
	}
	gameServer.runtime.mu.Lock()
	cooldowns := len(gameServer.runtime.scanCooldowns)
	capacitorSpends := len(gameServer.runtime.scanCapacitorSpends)
	queuedEvents := len(gameServer.runtime.queuedEvents[resolved.SessionID])
	gameServer.runtime.mu.Unlock()
	if cooldowns != 0 || capacitorSpends != 0 {
		t.Fatalf("scan payload %s cooldowns=%d capacitor_spends=%d, want none", payload, cooldowns, capacitorSpends)
	}
	if queuedEvents != 0 {
		t.Fatalf("scan payload %s queued events = %d, want none", payload, queuedEvents)
	}
	if planets := gameServer.runtime.Discovery.Planets(); len(planets) != 0 {
		t.Fatalf("scan payload %s planets = %d, want no mutation", payload, len(planets))
	}
	intel, err := gameServer.runtime.Discovery.PlayerPlanetIntelRecords(resolved.PlayerID)
	if err != nil {
		t.Fatalf("PlayerPlanetIntelRecords() error = %v, want nil", err)
	}
	if len(intel) != 0 {
		t.Fatalf("scan payload %s intel records = %d, want none", payload, len(intel))
	}
	afterProgression, err := gameServer.runtime.Progression.GetProgressionSnapshot(resolved.PlayerID)
	if err != nil {
		t.Fatalf("GetProgressionSnapshot(after) error = %v, want nil", err)
	}
	if afterProgression.Player.MainXP != beforeProgression.Player.MainXP {
		t.Fatalf("scan payload %s main XP = %d, want unchanged %d", payload, afterProgression.Player.MainXP, beforeProgression.Player.MainXP)
	}
	if events := gameServer.runtime.Scanner.Events(); len(events) != 0 {
		t.Fatalf("scan payload %s scanner events = %d, want none", payload, len(events))
	}
}

func TestScanPulseDuplicateRequestIDDoesNotDoubleSpend(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	request := `{"request_id":"request-scan-duplicate","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`

	writeText(t, conn, request)
	first := readResponse(t, conn)
	if !first.OK {
		t.Fatalf("first scan response = %+v, want success", first)
	}
	drainEventTypes(t, conn, realtime.EventScanPulseStarted, realtime.EventScanPulseResolved, realtime.EventScanPlanetDiscovered, realtime.EventKnownPlanets)
	afterFirst := testShipCapacitor(gameServer, resolved.PlayerID)
	if afterFirst != 100-starterScannerEnergyCost {
		t.Fatalf("first scan capacitor = %d, want one spend to %d", afterFirst, 100-starterScannerEnergyCost)
	}

	writeText(t, conn, request)
	second := readResponse(t, conn)
	if !second.OK {
		t.Fatalf("duplicate scan response = %+v, want success", second)
	}
	if afterSecond := testShipCapacitor(gameServer, resolved.PlayerID); afterSecond != afterFirst {
		t.Fatalf("duplicate scan capacitor = %d, want unchanged from %d", afterSecond, afterFirst)
	}
	if planets := gameServer.runtime.Discovery.Planets(); len(planets) != 1 {
		t.Fatalf("duplicate scan planets = %d, want one materialized planet", len(planets))
	}
}
func TestProductionAndStorageSummariesAreFilteredToActiveMap(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "production-map-filter@example.com", "Production Map")
	starterPlanetID := foundation.PlanetID("planet-production-map-1-1")
	mapTwoPlanetID := foundation.PlanetID("planet-production-map-1-2")

	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, starterPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-production-map-1-1")
	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, mapTwoPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-production-map-1-2")
	routeID := foundation.RouteID("route-production-map-1-1-to-1-2")
	seedAutomationRouteForTest(t, gameServer, resolved.PlayerID, routeID, starterPlanetID, mapTwoPlanetID, "map_1_1", "map_1_2")

	assertProductionSummaryPlanetIDs(t, gameServer, resolved.PlayerID, "", []foundation.PlanetID{starterPlanetID})
	assertStorageSummaryPlanetIDs(t, gameServer, resolved.PlayerID, "", []foundation.PlanetID{starterPlanetID})
	assertProductionSummaryPlanetIDs(t, gameServer, resolved.PlayerID, starterPlanetID, []foundation.PlanetID{starterPlanetID})
	assertStorageSummaryPlanetIDs(t, gameServer, resolved.PlayerID, starterPlanetID, []foundation.PlanetID{starterPlanetID})
	assertProductionSummaryPlanetIDs(t, gameServer, resolved.PlayerID, mapTwoPlanetID, nil)
	assertStorageSummaryPlanetIDs(t, gameServer, resolved.PlayerID, mapTwoPlanetID, nil)
	assertProductionAndStorageMapKeys(t, gameServer, resolved.PlayerID, starterPlanetID, "1-1")
	assertRouteListAndSnapshotMapKeys(t, gameServer, resolved, routeID, "1-1", "1-2")

	gameServer.runtime.mu.Lock()
	if _, err := gameServer.runtime.mapRouter.SetActiveLocationFromSpawn(resolved.PlayerID, "map_1_2", "west_gate"); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("SetActiveLocationFromSpawn(map_1_2) error = %v, want nil", err)
	}
	gameServer.runtime.mu.Unlock()
	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("ensurePlayerSession(map_1_2) error = %v, want nil", err)
	}

	assertProductionSummaryPlanetIDs(t, gameServer, resolved.PlayerID, "", []foundation.PlanetID{mapTwoPlanetID})
	assertStorageSummaryPlanetIDs(t, gameServer, resolved.PlayerID, "", []foundation.PlanetID{mapTwoPlanetID})
	assertProductionSummaryPlanetIDs(t, gameServer, resolved.PlayerID, mapTwoPlanetID, []foundation.PlanetID{mapTwoPlanetID})
	assertStorageSummaryPlanetIDs(t, gameServer, resolved.PlayerID, mapTwoPlanetID, []foundation.PlanetID{mapTwoPlanetID})
	assertProductionSummaryPlanetIDs(t, gameServer, resolved.PlayerID, starterPlanetID, nil)
	assertStorageSummaryPlanetIDs(t, gameServer, resolved.PlayerID, starterPlanetID, nil)
	assertProductionAndStorageMapKeys(t, gameServer, resolved.PlayerID, mapTwoPlanetID, "1-2")

	other := createResolvedRuntimeSession(t, gameServer, "route-not-owner@example.com", "Route Not Owner")
	notOwner := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(other.SessionID.String()),
		[]byte(`{"request_id":"request-route-snapshot-non-owner","op":"route.snapshot","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if !notOwner.HasError || notOwner.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("non-owner route snapshot = %+v, want safe not-found", notOwner)
	}
}

func TestRouteCreateCreatesOwnedPlanetRouteThroughGateway(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "route-create-owner@example.com", "Route Create Owner")
	other := createResolvedRuntimeSession(t, gameServer, "route-create-other@example.com", "Route Create Other")
	sourcePlanetID := foundation.PlanetID("planet-route-create-source")
	destinationPlanetID := foundation.PlanetID("planet-route-create-destination")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-create-source")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-create-destination")

	requestID := foundation.RequestID("request-route-create-owned")
	wantRouteID := foundation.RouteID("route-" + requestID.String())
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"`+requestID.String()+`","op":"route.create","payload":{"source_planet_id":"`+sourcePlanetID.String()+`","destination_planet_id":"`+destinationPlanetID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":40},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("route.create response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsInternalMapIdentity(t, "route.create response", response.Response.Payload)
	var payload struct {
		Route  routePayload     `json:"route"`
		Routes routeListPayload `json:"routes"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode route.create payload: %v", err)
	}
	assertRoutePayloadMapKeys(t, payload.Route, wantRouteID, "1-1", "1-2")
	if payload.Route.SourcePlanetID != sourcePlanetID.String() ||
		payload.Route.Destination.Type != production.RouteDestinationTypePlanet.String() ||
		payload.Route.Destination.ID != destinationPlanetID.String() ||
		payload.Route.ResourceItemID != "refined_alloy" ||
		payload.Route.AmountPerHour != 40 ||
		!payload.Route.Enabled {
		t.Fatalf("route.create route payload = %+v, want safe created route", payload.Route)
	}
	if len(payload.Routes.Routes) != 1 {
		t.Fatalf("route.create route list = %+v, want one route", payload.Routes.Routes)
	}
	assertRoutePayloadMapKeys(t, payload.Routes.Routes[0], wantRouteID, "1-1", "1-2")

	stored, ok, err := gameServer.runtime.Production.AutomationRoute(wantRouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok=%v err=%v, want stored route", wantRouteID, ok, err)
	}
	if stored.OwnerPlayerID != owner.PlayerID {
		t.Fatalf("stored route owner = %q, want server-resolved %q", stored.OwnerPlayerID, owner.PlayerID)
	}
	if stored.SourceMapID != "map_1_1" || stored.DestinationMapID != "map_1_2" {
		t.Fatalf("stored route map ids = %q/%q, want server-derived map_1_1/map_1_2", stored.SourceMapID, stored.DestinationMapID)
	}

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteCreate, owner.PlayerID)
	if err != nil {
		t.Fatalf("post route.create events: %v", err)
	}
	if _, leaked := eventsBySession[other.SessionID]; leaked {
		t.Fatalf("route.create events leaked to non-owner session: %+v", eventsBySession[other.SessionID])
	}
	ownerEvents := eventsBySession[owner.SessionID]
	requireEventTypeForTest(t, ownerEvents, realtime.EventRouteUpdated)
	requireEventTypeForTest(t, ownerEvents, realtime.EventRouteList)
	requireEventTypeForTest(t, ownerEvents, realtime.EventRouteSnapshot)
	for _, event := range ownerEvents {
		assertPayloadOmitsInternalMapIdentity(t, string(event.Type)+" event", event.Payload)
	}
	assertRouteCreateEventPayloads(t, ownerEvents, wantRouteID, "1-1", "1-2")

	assertRouteListAndSnapshotMapKeysWithRequestSuffix(t, gameServer, owner, wantRouteID, "1-1", "1-2", "create-owned")
}

func assertRouteCreateEventPayloads(t *testing.T, events []realtime.EventEnvelope, routeID foundation.RouteID, fromPublicMapKey string, toPublicMapKey string) {
	t.Helper()
	for _, event := range events {
		switch event.Type {
		case realtime.EventRouteUpdated, realtime.EventRouteSnapshot:
			var payload struct {
				Route routePayload `json:"route"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode %s event payload: %v", event.Type, err)
			}
			assertRoutePayloadMapKeys(t, payload.Route, routeID, fromPublicMapKey, toPublicMapKey)
		case realtime.EventRouteList:
			var payload struct {
				Routes routeListPayload `json:"routes"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode route.list event payload: %v", err)
			}
			if len(payload.Routes.Routes) != 1 {
				t.Fatalf("route.list event routes = %+v, want one route", payload.Routes.Routes)
			}
			assertRoutePayloadMapKeys(t, payload.Routes.Routes[0], routeID, fromPublicMapKey, toPublicMapKey)
		}
	}
}

func TestRouteCreateRejectsSpoofedServerOwnedFieldsBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "route-create-spoof@example.com", "Route Create Spoof")
	sourcePlanetID := foundation.PlanetID("planet-route-create-spoof-source")
	destinationPlanetID := foundation.PlanetID("planet-route-create-spoof-destination")

	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-create-spoof-source")
	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-create-spoof-destination")

	tests := []struct {
		name  string
		field string
	}{
		{name: "owner player", field: `"owner_player_id":"spoofed-player"`},
		{name: "source map", field: `"source_map_id":"map_1_1"`},
		{name: "destination map", field: `"destination_map_id":"map_1_2"`},
		{name: "route id", field: `"route_id":"route-spoofed"`},
		{name: "energy cost", field: `"energy_cost_per_hour":1`},
		{name: "risk", field: `"risk":{"loss_chance":0.99}`},
		{name: "loss chance", field: `"loss_chance":0.99`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(resolved.SessionID.String()),
				[]byte(`{"request_id":"request-route-create-spoof-`+strings.ReplaceAll(tt.name, " ", "-")+`","op":"route.create","payload":{"source_planet_id":"`+sourcePlanetID.String()+`","destination_planet_id":"`+destinationPlanetID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":40,`+tt.field+`},"client_seq":1,"v":1}`),
			)
			if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
				t.Fatalf("route.create spoof response = %+v, want invalid payload", response)
			}
			if routes := gameServer.runtime.Production.AutomationRoutes(); len(routes) != 0 {
				t.Fatalf("routes after spoofed %s = %+v, want no mutation", tt.name, routes)
			}
			gameServer.runtime.mu.Lock()
			queuedEvents := len(gameServer.runtime.queuedEvents[resolved.SessionID])
			gameServer.runtime.mu.Unlock()
			if queuedEvents != 0 {
				t.Fatalf("spoofed %s queued events = %d, want none", tt.name, queuedEvents)
			}
		})
	}
}

func TestRouteCreateRejectsXCoreResourceBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "route-create-xcore@example.com", "Route Create XCore")
	sourcePlanetID := foundation.PlanetID("planet-route-create-xcore-source")
	destinationPlanetID := foundation.PlanetID("planet-route-create-xcore-destination")

	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-create-xcore-source")
	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-create-xcore-destination")

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-route-create-xcore","op":"route.create","payload":{"source_planet_id":"`+sourcePlanetID.String()+`","destination_planet_id":"`+destinationPlanetID.String()+`","resource_item_id":"x_core","amount_per_hour":1},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("route.create x_core response = %+v, want forbidden", response)
	}
	if routes := gameServer.runtime.Production.AutomationRoutes(); len(routes) != 0 {
		t.Fatalf("routes after x_core route.create = %+v, want no mutation", routes)
	}
	gameServer.runtime.mu.Lock()
	queuedEvents := len(gameServer.runtime.queuedEvents[resolved.SessionID])
	gameServer.runtime.mu.Unlock()
	if queuedEvents != 0 {
		t.Fatalf("x_core route.create queued events = %d, want none", queuedEvents)
	}
}

func TestRouteCreateRejectsRouteCapacityBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "route-create-capacity@example.com", "Route Create Capacity")
	sourcePlanetID := foundation.PlanetID("planet-route-create-capacity-source")
	destinationPlanetID := foundation.PlanetID("planet-route-create-capacity-destination")

	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-create-capacity-source")
	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-create-capacity-destination")

	for index := 0; index < runtimeRouteCreateMaxRoutesPerPlayer; index++ {
		requestID := foundation.RequestID("request-route-create-capacity-" + strconv.Itoa(index))
		response := gameServer.runtime.Gateway.HandleRequest(
			realtime.SessionID(resolved.SessionID.String()),
			[]byte(`{"request_id":"`+requestID.String()+`","op":"route.create","payload":{"source_planet_id":"`+sourcePlanetID.String()+`","destination_planet_id":"`+destinationPlanetID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":40},"client_seq":1,"v":1}`),
		)
		if response.HasError {
			t.Fatalf("route.create capacity seed %d response error = %+v, want success", index, response.Error)
		}
		if _, err := gameServer.runtime.postCommandEventsBySession(resolved.SessionID, realtime.OperationRouteCreate, resolved.PlayerID); err != nil {
			t.Fatalf("post route.create capacity seed %d events: %v", index, err)
		}
	}

	beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
	if len(beforeRoutes) != runtimeRouteCreateMaxRoutesPerPlayer {
		t.Fatalf("seeded routes = %+v, want %d", beforeRoutes, runtimeRouteCreateMaxRoutesPerPlayer)
	}
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-route-create-capacity-overflow","op":"route.create","payload":{"source_planet_id":"`+sourcePlanetID.String()+`","destination_planet_id":"`+destinationPlanetID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":40},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("route.create capacity overflow response = %+v, want forbidden", response)
	}
	assertRoutesUnchanged(t, gameServer, beforeRoutes, "route.create capacity overflow")
	assertNoQueuedEventsForSessions(t, gameServer, resolved.SessionID)
}

func assertProductionAndStorageMapKeys(t *testing.T, gameServer *Server, playerID foundation.PlayerID, planetID foundation.PlanetID, publicMapKey string) {
	t.Helper()
	productionPayload, err := gameServer.runtime.productionSummaryPayload(playerID, planetID)
	if err != nil {
		t.Fatalf("productionSummaryPayload(%q) error = %v, want nil", planetID, err)
	}
	if len(productionPayload.Planets) != 1 {
		t.Fatalf("productionSummaryPayload(%q) planets = %+v, want one planet", planetID, productionPayload.Planets)
	}
	productionPlanet := productionPayload.Planets[0]
	if productionPlanet.PublicMapKey != publicMapKey || productionPlanet.Storage.PublicMapKey != publicMapKey {
		t.Fatalf("production/storage public map keys = %q/%q, want %q", productionPlanet.PublicMapKey, productionPlanet.Storage.PublicMapKey, publicMapKey)
	}
	assertPayloadOmitsInternalMapIdentity(t, "production payload", productionPayload)

	storagePayload, err := gameServer.runtime.storageSummaryPayload(playerID, planetID)
	if err != nil {
		t.Fatalf("storageSummaryPayload(%q) error = %v, want nil", planetID, err)
	}
	if len(storagePayload.Planets) != 1 || storagePayload.Planets[0].PublicMapKey != publicMapKey {
		t.Fatalf("storageSummaryPayload(%q) = %+v, want public map key %q", planetID, storagePayload.Planets, publicMapKey)
	}
	assertPayloadOmitsInternalMapIdentity(t, "storage payload", storagePayload)
}

func assertRouteListAndSnapshotMapKeys(t *testing.T, gameServer *Server, resolved auth.ResolvedSession, routeID foundation.RouteID, fromPublicMapKey string, toPublicMapKey string) {
	t.Helper()
	assertRouteListAndSnapshotMapKeysWithRequestSuffix(t, gameServer, resolved, routeID, fromPublicMapKey, toPublicMapKey, "map-keys")
}

func assertRouteListAndSnapshotMapKeysWithRequestSuffix(t *testing.T, gameServer *Server, resolved auth.ResolvedSession, routeID foundation.RouteID, fromPublicMapKey string, toPublicMapKey string, requestSuffix string) {
	t.Helper()
	list := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-route-list-`+requestSuffix+`","op":"route.list","payload":{},"client_seq":1,"v":1}`),
	)
	if list.HasError {
		t.Fatalf("route.list error = %+v, want success", list.Error)
	}
	assertPayloadOmitsInternalMapIdentity(t, "route.list payload", list.Response.Payload)
	var listPayload struct {
		Routes routeListPayload `json:"routes"`
	}
	if err := json.Unmarshal(list.Response.Payload, &listPayload); err != nil {
		t.Fatalf("decode route.list payload: %v", err)
	}
	if len(listPayload.Routes.Routes) != 1 {
		t.Fatalf("route.list routes = %+v, want one route", listPayload.Routes.Routes)
	}
	assertRoutePayloadMapKeys(t, listPayload.Routes.Routes[0], routeID, fromPublicMapKey, toPublicMapKey)

	snapshot := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-route-snapshot-`+requestSuffix+`","op":"route.snapshot","payload":{"route_id":"`+routeID.String()+`"},"client_seq":2,"v":1}`),
	)
	if snapshot.HasError {
		t.Fatalf("route.snapshot error = %+v, want success", snapshot.Error)
	}
	assertPayloadOmitsInternalMapIdentity(t, "route.snapshot payload", snapshot.Response.Payload)
	var snapshotPayload struct {
		Route routePayload `json:"route"`
	}
	if err := json.Unmarshal(snapshot.Response.Payload, &snapshotPayload); err != nil {
		t.Fatalf("decode route.snapshot payload: %v", err)
	}
	assertRoutePayloadMapKeys(t, snapshotPayload.Route, routeID, fromPublicMapKey, toPublicMapKey)
}

func assertRoutePayloadMapKeys(t *testing.T, payload routePayload, routeID foundation.RouteID, fromPublicMapKey string, toPublicMapKey string) {
	t.Helper()
	if payload.RouteID != routeID.String() || payload.FromPublicMapKey != fromPublicMapKey || payload.ToPublicMapKey != toPublicMapKey {
		t.Fatalf("route payload = %+v, want route %q public map keys %q/%q", payload, routeID, fromPublicMapKey, toPublicMapKey)
	}
}

func assertPayloadOmitsInternalMapIdentity(t *testing.T, label string, payload any) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s: %v", label, err)
	}
	raw := string(data)
	for _, forbidden := range []string{
		`"internal_map_id"`,
		`"source_map_id"`,
		`"destination_map_id"`,
		`"world_id"`,
		`"zone_id"`,
		`"map_id"`,
		`"map_1_1"`,
		`"map_1_2"`,
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}

func assertPayloadOmitsScannerNoFogTruth(t *testing.T, label string, payload []byte, extraForbidden ...string) {
	t.Helper()
	raw := string(payload)
	forbidden := append([]string{
		"candidate_key",
		"planet_candidate",
		"procedural_seed",
		"world_seed",
		"detection_roll",
		"scan_roll",
		"scan_cell",
		"scan_candidate",
		"scan_candidates",
		"candidate_data",
		`"world_id"`,
		`"zone_id"`,
		`"internal_map_id"`,
		`"map_id"`,
		"fog",
		"fog_wave",
		"fog_memory",
		"fog-of-war",
		"fog_of_war",
		"fogOfWar",
	}, extraForbidden...)
	for _, fragment := range forbidden {
		if strings.Contains(raw, fragment) {
			t.Fatalf("%s leaked %q in %s", label, fragment, raw)
		}
	}
}

func assertPayloadOmitsActiveMapInternalTruth(t *testing.T, label string, payload []byte) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range []string{
		"map_1_1",
		"map_1_2",
		"world_id",
		"zone_id",
		"candidate_key",
		"procedural_seed",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}

func seedAutomationRouteForTest(
	t *testing.T,
	gameServer *Server,
	ownerID foundation.PlayerID,
	routeID foundation.RouteID,
	sourcePlanetID foundation.PlanetID,
	destinationPlanetID foundation.PlanetID,
	sourceMapID production.RouteMapID,
	destinationMapID production.RouteMapID,
) {
	t.Helper()
	service, err := production.NewAutomationRouteService(production.AutomationRouteServiceConfig{
		Store:  gameServer.runtime.Production,
		Clock:  gameServer.runtime.clock,
		Policy: mapAwareRoutePolicyForTest{sourceMapID: sourceMapID, destinationMapID: destinationMapID},
	})
	if err != nil {
		t.Fatalf("NewAutomationRouteService() error = %v, want nil", err)
	}
	destination, err := production.NewPlanetRouteDestination(destinationPlanetID)
	if err != nil {
		t.Fatalf("NewPlanetRouteDestination(%q) error = %v, want nil", destinationPlanetID, err)
	}
	result, err := service.CreateRoute(production.CreateRouteInput{
		RouteID:        routeID,
		OwnerPlayerID:  ownerID,
		SourcePlanetID: sourcePlanetID,
		Destination:    destination,
		ResourceItemID: "refined_alloy",
		AmountPerHour:  40,
	})
	if err != nil {
		t.Fatalf("CreateRoute(%q) error = %v, want nil", routeID, err)
	}
	if result.Route.SourceMapID != sourceMapID || result.Route.DestinationMapID != destinationMapID {
		t.Fatalf("seeded route map ids = %q/%q, want %q/%q", result.Route.SourceMapID, result.Route.DestinationMapID, sourceMapID, destinationMapID)
	}
}

type mapAwareRoutePolicyForTest struct {
	sourceMapID      production.RouteMapID
	destinationMapID production.RouteMapID
}

func (policy mapAwareRoutePolicyForTest) RouteCreatePolicy(input production.RouteCreatePolicyInput) (production.RouteCreatePolicy, error) {
	if err := input.Validate(); err != nil {
		return production.RouteCreatePolicy{}, err
	}
	return production.RouteCreatePolicy{
		SourcePlanetOwned:     true,
		DestinationAccessible: true,
		ResourceRouteable:     true,
		RequirementsMet:       true,
		SourceMapID:           policy.sourceMapID,
		DestinationMapID:      policy.destinationMapID,
		DistanceUnits:         100,
		MaxDistanceUnits:      1_000,
		MinLossPercent:        0,
		MaxLossPercent:        0,
		EnergyCostPerHour:     4,
	}, nil
}
