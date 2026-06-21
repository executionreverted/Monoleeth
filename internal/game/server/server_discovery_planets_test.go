package server

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
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
	rawScan := string(scanResponse.Payload)
	for _, forbidden := range []string{
		"candidate_key",
		"planet_candidate",
		"procedural_seed",
		"world_seed",
		"detection_roll",
		"scan_cell",
		`"world_id"`,
		`"zone_id"`,
		`"internal_map_id"`,
		`"map_id"`,
		`"coordinates"`,
		`"x"`,
		`"y"`,
	} {
		if strings.Contains(rawScan, forbidden) {
			t.Fatalf("scan response leaked %q in %s", forbidden, rawScan)
		}
	}
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
		if raw := string(mustJSON(t, event)); strings.Contains(raw, "candidate_key") || strings.Contains(raw, "procedural_seed") || strings.Contains(raw, "detection_roll") || strings.Contains(raw, `"world_id"`) || strings.Contains(raw, `"zone_id"`) || strings.Contains(raw, `"internal_map_id"`) || strings.Contains(raw, `"map_id"`) {
			t.Fatalf("scan event leaked hidden scanner truth: %s", raw)
		}
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

	writeText(t, conn, `{"request_id":"request-scan-low-cap","op":"scan.pulse","payload":{"energy":999,"capacitor":999},"client_seq":1,"v":1}`)
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

	assertProductionSummaryPlanetIDs(t, gameServer, resolved.PlayerID, "", []foundation.PlanetID{starterPlanetID})
	assertStorageSummaryPlanetIDs(t, gameServer, resolved.PlayerID, "", []foundation.PlanetID{starterPlanetID})
	assertProductionSummaryPlanetIDs(t, gameServer, resolved.PlayerID, starterPlanetID, []foundation.PlanetID{starterPlanetID})
	assertStorageSummaryPlanetIDs(t, gameServer, resolved.PlayerID, starterPlanetID, []foundation.PlanetID{starterPlanetID})
	assertProductionSummaryPlanetIDs(t, gameServer, resolved.PlayerID, mapTwoPlanetID, nil)
	assertStorageSummaryPlanetIDs(t, gameServer, resolved.PlayerID, mapTwoPlanetID, nil)

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
}
