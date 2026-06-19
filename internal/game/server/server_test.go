package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/premium"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
)

const testOrigin = "http://example.com"

func TestServerAuthRoutesAndWebSocketBootstrap(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()

	events := readBootstrapEvents(t, conn)
	gotTypes := make(map[realtime.ClientEventType]struct{}, len(events))
	for _, event := range events {
		gotTypes[event.Type] = struct{}{}
		raw := string(mustJSON(t, event))
		for _, forbidden := range []string{
			"account_id",
			"player_id",
			"session_id",
			"world_id",
			"zone_id",
			"entity_hidden_planet_signal",
			"npc_placeholder",
			"loot_placeholder",
			"planet_signal_placeholder",
			"gameplay_seed",
			"future_spawn",
		} {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("bootstrap event leaked %q in %s", forbidden, raw)
			}
		}
	}
	for _, want := range []realtime.ClientEventType{
		realtime.EventSessionReady,
		realtime.EventPlayerSnapshot,
		realtime.EventShipSnapshot,
		realtime.EventStatsUpdated,
		realtime.EventWalletSnapshot,
		realtime.EventCargoSnapshot,
		realtime.EventProgressionSnapshot,
		realtime.EventWorldSnapshot,
	} {
		if _, ok := gotTypes[want]; !ok {
			t.Fatalf("missing bootstrap event %q in %#v", want, gotTypes)
		}
	}
	if events[0].Sequence != 1 || events[len(events)-1].Sequence != uint64(len(events)) {
		t.Fatalf("bootstrap seq range = %d..%d, want 1..%d", events[0].Sequence, events[len(events)-1].Sequence, len(events))
	}
	_ = gameServer
}

func TestWorldSnapshotCarriesSectorMinimapAndPublicEntityContract(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	events := readBootstrapEvents(t, conn)

	var snapshot worldSnapshotPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &snapshot); err != nil {
		t.Fatalf("decode world snapshot: %v", err)
	}
	if snapshot.Sector.Name != "Origin Fringe" || snapshot.Sector.Region != "Origin Belt" || snapshot.Sector.Danger == "" {
		t.Fatalf("sector payload = %+v, want client-safe sector summary", snapshot.Sector)
	}
	if snapshot.Minimap.RadarRange != defaultRadarRange {
		t.Fatalf("minimap radar = %v, want %v", snapshot.Minimap.RadarRange, defaultRadarRange)
	}
	if len(snapshot.Minimap.LiveContacts) != len(snapshot.Entities) {
		t.Fatalf("minimap contacts = %d, entities = %d", len(snapshot.Minimap.LiveContacts), len(snapshot.Entities))
	}
	selfCount := 0
	for _, entity := range snapshot.Entities {
		if strings.Contains(entity.Type.String(), "placeholder") {
			t.Fatalf("entity type %q still uses placeholder contract", entity.Type)
		}
		if entity.Display == nil || entity.Display.Label == "" || entity.Display.Disposition == "" {
			t.Fatalf("entity %+v missing public display metadata", entity)
		}
		if hasStatusFlag(entity.StatusFlags, "self") {
			selfCount++
			if entity.Type != "player" || entity.Display.Disposition != "self" {
				t.Fatalf("self entity = %+v, want player/self", entity)
			}
		}
	}
	if selfCount != 1 {
		t.Fatalf("self entity count = %d, want 1", selfCount)
	}
	for _, contact := range snapshot.Minimap.LiveContacts {
		if contact.EntityID == "entity_hidden_planet_signal" {
			t.Fatalf("hidden entity leaked into minimap contact %+v", contact)
		}
	}
}

func TestWebSocketUnauthenticatedConnectionRejectedBeforeUpgrade(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, wsURL(httpServer), &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{testOrigin}},
	})
	if err == nil {
		t.Fatal("websocket dial without cookie succeeded, want rejection")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("dial response = %#v, want 401", resp)
	}
}

func TestMoveToThroughWebSocketUsesGatewayAndServerPosition(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-move-1","op":"move_to","payload":{"target":{"x":100,"y":0}},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("move response = %+v, want success", response)
	}
	var payload struct {
		Accepted bool `json:"accepted"`
		Entities []struct {
			EntityID string `json:"entity_id"`
			Type     string `json:"entity_type"`
			Position struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"position"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode move payload: %v", err)
	}
	if !payload.Accepted {
		t.Fatal("move accepted = false, want true")
	}
	var playerX float64
	for _, entity := range payload.Entities {
		if entity.Type == "player" {
			playerX = entity.Position.X
		}
	}
	if playerX <= 0 || playerX >= 100 {
		t.Fatalf("player x after one server tick = %v, want server-derived movement between 0 and target", playerX)
	}

	event := readEvent(t, conn)
	if event.Type != realtime.EventPositionCorrected || event.Sequence == 0 {
		t.Fatalf("post-move event = %+v, want position.corrected with seq", event)
	}
	update := readEvent(t, conn)
	if update.Type != realtime.EventAOIEntityUpdated {
		t.Fatalf("post-move AOI event = %+v, want entity updated", update)
	}
}

func TestMoveToRejectsExcessivePathAndDisabledShip(t *testing.T) {
	t.Run("non finite coordinate", func(t *testing.T) {
		_, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)

		writeText(t, conn, `{"request_id":"request-move-non-finite","op":"move_to","payload":{"target":{"x":1e999,"y":0}},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeInvalidPayload {
			t.Fatalf("non-finite move error = %+v, want %s", got.Error, foundation.CodeInvalidPayload)
		}
	})

	t.Run("excessive path", func(t *testing.T) {
		_, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)

		writeText(t, conn, `{"request_id":"request-move-far","op":"move_to","payload":{"target":{"x":5000,"y":0}},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeOutOfRange {
			t.Fatalf("far move error = %+v, want %s", got.Error, foundation.CodeOutOfRange)
		}
	})

	t.Run("disabled ship", func(t *testing.T) {
		gameServer, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		cookie := registerPilot(t, httpServer)
		conn := dialWebSocket(t, httpServer, cookie)
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)
		resolved := resolvedSessionForCookie(t, gameServer, cookie)
		setTestShipDisabled(gameServer, resolved.PlayerID, true)

		writeText(t, conn, `{"request_id":"request-move-disabled","op":"move_to","payload":{"target":{"x":10,"y":0}},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeShipDisabled {
			t.Fatalf("disabled move error = %+v, want %s", got.Error, foundation.CodeShipDisabled)
		}
	})
}

func TestStopClearsMovementTargetServerSide(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)

	writeText(t, conn, `{"request_id":"request-stop-move","op":"move_to","payload":{"target":{"x":100,"y":0}},"client_seq":1,"v":1}`)
	if response := readResponse(t, conn); !response.OK {
		t.Fatalf("move response = %+v, want success", response)
	}
	readEvent(t, conn)
	readEvent(t, conn)
	writeText(t, conn, `{"request_id":"request-stop-1","op":"stop","payload":{},"client_seq":2,"v":1}`)
	if response := readResponse(t, conn); !response.OK {
		t.Fatalf("stop response = %+v, want success", response)
	}
	correction := readEvent(t, conn)
	stopped := readEvent(t, conn)
	if correction.Type != realtime.EventPositionCorrected || stopped.Type != realtime.EventMovementStopped {
		t.Fatalf("stop events = %q/%q, want correction/stopped", correction.Type, stopped.Type)
	}

	gameServer.runtime.mu.Lock()
	entity, ok := gameServer.runtime.Worker.PlayerEntity(resolved.PlayerID)
	gameServer.runtime.mu.Unlock()
	if !ok {
		t.Fatal("player entity missing after stop")
	}
	if entity.Movement.Moving {
		t.Fatalf("movement state = %+v, want stopped", entity.Movement)
	}
}

func TestCombatKillCreatesLootAndPickupUpdatesCargo(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-combat-1","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("combat response = %+v, want success", response)
	}
	var combatPayload struct {
		Accepted bool `json:"accepted"`
		Killed   bool `json:"killed"`
		Amount   int  `json:"amount"`
		Ship     struct {
			Capacitor int `json:"capacitor"`
		} `json:"ship"`
	}
	if err := json.Unmarshal(response.Payload, &combatPayload); err != nil {
		t.Fatalf("decode combat response: %v", err)
	}
	if !combatPayload.Accepted || !combatPayload.Killed || combatPayload.Amount <= 0 || combatPayload.Ship.Capacitor >= 100 {
		t.Fatalf("combat payload = %+v, want accepted killed with energy spent", combatPayload)
	}

	var dropID string
	seen := map[realtime.ClientEventType]bool{}
	for attempts := 0; attempts < 12 && (dropID == "" || !seen[realtime.EventAOIEntityEntered] || !seen[realtime.EventAOIEntityLeft]); attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
		raw := string(event.Payload)
		for _, forbidden := range []string{"player_id", "damage", "loot_table", "gameplay_seed"} {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("combat event leaked %q in %s", forbidden, raw)
			}
		}
		if event.Type == realtime.EventLootCreated {
			var payload struct {
				DropID   string `json:"drop_id"`
				EntityID string `json:"entity_id"`
				ItemID   string `json:"item_id"`
				Quantity int64  `json:"quantity"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode loot.created: %v", err)
			}
			if payload.DropID == "" || payload.EntityID != payload.DropID || payload.ItemID != "raw_ore" || payload.Quantity != 3 {
				t.Fatalf("loot.created payload = %+v, want raw ore drop", payload)
			}
			dropID = payload.DropID
		}
	}
	for _, want := range []realtime.ClientEventType{
		realtime.EventCombatDamage,
		realtime.EventCombatCooldownStarted,
		realtime.EventTargetUpdated,
		realtime.EventCombatNPCKilled,
		realtime.EventProgressionSnapshot,
		realtime.EventLootCreated,
	} {
		if !seen[want] {
			t.Fatalf("combat events seen = %#v, missing %s", seen, want)
		}
	}

	request := `{"request_id":"request-loot-1","op":"loot.pickup","payload":{"drop_id":"` + dropID + `"},"client_seq":2,"v":1}`
	writeText(t, conn, request)
	pickup := readResponse(t, conn)
	if !pickup.OK {
		t.Fatalf("pickup response = %+v, want success", pickup)
	}
	var pickupPayload struct {
		Accepted bool `json:"accepted"`
		Cargo    struct {
			Used     int64 `json:"used"`
			Capacity int64 `json:"capacity"`
			Items    []struct {
				ItemID   string `json:"item_id"`
				Quantity int64  `json:"quantity"`
			} `json:"items"`
		} `json:"cargo"`
	}
	if err := json.Unmarshal(pickup.Payload, &pickupPayload); err != nil {
		t.Fatalf("decode pickup response: %v", err)
	}
	if !pickupPayload.Accepted || pickupPayload.Cargo.Used != 6 || len(pickupPayload.Cargo.Items) != 1 || pickupPayload.Cargo.Items[0].Quantity != 3 {
		t.Fatalf("pickup payload = %+v, want cargo with three raw ore", pickupPayload)
	}

	seen = map[realtime.ClientEventType]bool{}
	for attempts := 0; attempts < 8 && !seen[realtime.EventAOIEntityLeft]; attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
	}
	for _, want := range []realtime.ClientEventType{realtime.EventLootPickedUp, realtime.EventLootRemoved, realtime.EventCargoSnapshot, realtime.EventProgressionSnapshot, realtime.EventAOIEntityLeft} {
		if !seen[want] {
			t.Fatalf("pickup events seen = %#v, missing %s", seen, want)
		}
	}
	writeText(t, conn, request)
	duplicatePickup := readResponse(t, conn)
	if !bytes.Equal(duplicatePickup.Payload, pickup.Payload) {
		t.Fatalf("duplicate pickup payload changed:\nfirst=%s\nsecond=%s", pickup.Payload, duplicatePickup.Payload)
	}
}

func TestPhase06SnapshotQueriesUseServerResolvedState(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	requests := []struct {
		name string
		body string
	}{
		{
			name: "progression",
			body: `{"request_id":"request-progression-snapshot","op":"progression.snapshot","payload":{},"client_seq":1,"v":1}`,
		},
		{
			name: "inventory",
			body: `{"request_id":"request-inventory-snapshot","op":"inventory.snapshot","payload":{},"client_seq":2,"v":1}`,
		},
		{
			name: "hangar",
			body: `{"request_id":"request-hangar-snapshot","op":"hangar.snapshot","payload":{},"client_seq":3,"v":1}`,
		},
		{
			name: "loadout",
			body: `{"request_id":"request-loadout-snapshot","op":"loadout.snapshot","payload":{},"client_seq":4,"v":1}`,
		},
		{
			name: "stats",
			body: `{"request_id":"request-stats-snapshot","op":"stats.snapshot","payload":{},"client_seq":5,"v":1}`,
		},
		{
			name: "crafting",
			body: `{"request_id":"request-crafting-recipes","op":"crafting.recipes","payload":{},"client_seq":6,"v":1}`,
		},
	}

	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			writeText(t, conn, request.body)
			response := readResponse(t, conn)
			if !response.OK {
				t.Fatalf("%s response = %+v, want success", request.name, response)
			}
			raw := string(response.Payload)
			for _, forbidden := range []string{"account_id", "player_id", "session_id", "world_id", "zone_id", "gameplay_seed", "loot_table"} {
				if strings.Contains(raw, forbidden) {
					t.Fatalf("%s response leaked %q in %s", request.name, forbidden, raw)
				}
			}

			switch request.name {
			case "progression":
				var payload struct {
					Progression progressionSnapshotPayload `json:"progression"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode progression snapshot: %v", err)
				}
				if payload.Progression.Rank != 1 || payload.Progression.MainLevel < 1 {
					t.Fatalf("progression payload = %+v, want starter server snapshot", payload.Progression)
				}
			case "inventory":
				var payload struct {
					Inventory inventorySnapshotPayload `json:"inventory"`
					Cargo     cargoSnapshotPayload     `json:"cargo"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode inventory snapshot: %v", err)
				}
				if len(payload.Inventory.Stackable) != 0 || payload.Cargo.Capacity != 60 {
					t.Fatalf("inventory payload = %+v cargo=%+v, want empty starter inventory and cargo capacity", payload.Inventory, payload.Cargo)
				}
			case "hangar":
				var payload struct {
					Hangar hangarSnapshotPayload `json:"hangar"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode hangar snapshot: %v", err)
				}
				if payload.Hangar.ActiveShipID != "starter_ship" || len(payload.Hangar.Ships) != 1 {
					t.Fatalf("hangar payload = %+v, want active starter ship", payload.Hangar)
				}
			case "loadout":
				var payload struct {
					Loadout loadoutSnapshotPayload `json:"loadout"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode loadout snapshot: %v", err)
				}
				if payload.Loadout.ActiveShipID != "starter_ship" || len(payload.Loadout.Slots) != 3 {
					t.Fatalf("loadout payload = %+v, want starter slot snapshot", payload.Loadout)
				}
			case "stats":
				var payload struct {
					Stats statSnapshotPayload `json:"stats"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode stats snapshot: %v", err)
				}
				if payload.Stats.RadarRange != defaultRadarRange || payload.Stats.CargoCapacity != 60 {
					t.Fatalf("stats payload = %+v, want starter effective stats", payload.Stats)
				}
			case "crafting":
				var payload struct {
					Crafting craftingSnapshotPayload `json:"crafting"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode crafting recipes: %v", err)
				}
				if len(payload.Crafting.Recipes) < 3 || payload.Crafting.Recipes[0].CraftDurationMS <= 0 {
					t.Fatalf("crafting payload = %+v, want MVP recipes with millisecond durations", payload.Crafting)
				}
			}
		})
	}

	writeText(t, conn, `{"request_id":"request-progression-spoof","op":"progression.snapshot","payload":{"xp":999,"rank":99},"client_seq":7,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed progression error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	dropID := killTrainingNPCForDrop(t, conn)
	writeText(t, conn, `{"request_id":"request-phase06-loot","op":"loot.pickup","payload":{"drop_id":"`+dropID+`"},"client_seq":8,"v":1}`)
	pickup := readResponse(t, conn)
	if !pickup.OK {
		t.Fatalf("phase06 pickup response = %+v, want success", pickup)
	}
	var pickupPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
	}
	if err := json.Unmarshal(pickup.Payload, &pickupPayload); err != nil {
		t.Fatalf("decode pickup inventory: %v", err)
	}
	if len(pickupPayload.Inventory.Stackable) != 1 ||
		pickupPayload.Inventory.Stackable[0].ItemID != "raw_ore" ||
		pickupPayload.Inventory.Stackable[0].Quantity != 3 ||
		pickupPayload.Inventory.Stackable[0].Location != economy.LocationKindShipCargo.String() {
		t.Fatalf("pickup inventory = %+v, want real raw ore in ship cargo", pickupPayload.Inventory)
	}
}

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
	planetID := scanPayload.Scan.PlanetID

	seen := map[realtime.ClientEventType]bool{}
	for attempts := 0; attempts < 6 && (!seen[realtime.EventScanPulseStarted] || !seen[realtime.EventScanPulseResolved] || !seen[realtime.EventScanPlanetDiscovered] || !seen[realtime.EventKnownPlanets]); attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
		if raw := string(mustJSON(t, event)); strings.Contains(raw, "candidate_key") || strings.Contains(raw, "procedural_seed") || strings.Contains(raw, "detection_roll") {
			t.Fatalf("scan event leaked hidden scanner truth: %s", raw)
		}
	}
	for _, want := range []realtime.ClientEventType{realtime.EventScanPulseStarted, realtime.EventScanPulseResolved, realtime.EventScanPlanetDiscovered, realtime.EventKnownPlanets} {
		if !seen[want] {
			t.Fatalf("scan events seen = %#v, missing %s", seen, want)
		}
	}

	writeText(t, conn, `{"request_id":"request-known-planets","op":"discovery.known_planets","payload":{},"client_seq":2,"v":1}`)
	knownResponse := readResponse(t, conn)
	if !knownResponse.OK {
		t.Fatalf("known planets response = %+v, want success", knownResponse)
	}
	var knownPayload struct {
		KnownPlanets knownPlanetsPayload `json:"known_planets"`
	}
	if err := json.Unmarshal(knownResponse.Payload, &knownPayload); err != nil {
		t.Fatalf("decode known planets: %v", err)
	}
	if len(knownPayload.KnownPlanets.Planets) != 1 || knownPayload.KnownPlanets.Planets[0].PlanetID != planetID {
		t.Fatalf("known planets response = %+v, want discovered planet %s", knownPayload.KnownPlanets, planetID)
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

	for _, request := range []struct {
		name string
		body string
	}{
		{name: "production", body: `{"request_id":"request-production-summary","op":"planet.production_summary","payload":{},"client_seq":4,"v":1}`},
		{name: "storage", body: `{"request_id":"request-storage-summary","op":"planet.storage_summary","payload":{},"client_seq":5,"v":1}`},
		{name: "routes", body: `{"request_id":"request-route-list","op":"route.list","payload":{},"client_seq":6,"v":1}`},
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

	writeText(t, conn, `{"request_id":"request-scan-spoof","op":"scan.pulse","payload":{"player_id":"spoofed-player","candidate_key":"forced","procedural_seed":"leak","scan_result":"planet"},"client_seq":7,"v":1}`)
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

func TestPhase08MarketAuctionPremiumUseServerEconomyState(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-wallet-snapshot","op":"wallet.snapshot","payload":{},"client_seq":1,"v":1}`)
	walletResponse := readResponse(t, conn)
	if !walletResponse.OK {
		t.Fatalf("wallet response = %+v, want success", walletResponse)
	}
	var walletPayload struct {
		Wallet walletSnapshotPayload `json:"wallet"`
	}
	if err := json.Unmarshal(walletResponse.Payload, &walletPayload); err != nil {
		t.Fatalf("decode wallet: %v", err)
	}
	if walletPayload.Wallet.Credits != starterWalletCredits || walletPayload.Wallet.PremiumPaid != starterWalletPremiumPaid {
		t.Fatalf("wallet = %+v, want deterministic starter balances", walletPayload.Wallet)
	}

	writeText(t, conn, `{"request_id":"request-market-search","op":"market.search","payload":{},"client_seq":2,"v":1}`)
	marketResponse := readResponse(t, conn)
	if !marketResponse.OK {
		t.Fatalf("market response = %+v, want success", marketResponse)
	}
	assertNoEconomyLeak(t, "market search", marketResponse.Payload)
	var marketPayload struct {
		Market marketSearchPayload `json:"market"`
	}
	if err := json.Unmarshal(marketResponse.Payload, &marketPayload); err != nil {
		t.Fatalf("decode market: %v", err)
	}
	if len(marketPayload.Market.Listings) != 1 || marketPayload.Market.Listings[0].ListingID != seedMarketListingID.String() {
		t.Fatalf("market listings = %+v, want seeded listing", marketPayload.Market.Listings)
	}
	if !marketPayload.Market.Listings[0].ServerRecalculates {
		t.Fatalf("market listing = %+v, want server recalculation marker", marketPayload.Market.Listings[0])
	}

	writeText(t, conn, `{"request_id":"request-market-spoof","op":"market.buy","payload":{"listing_id":"`+seedMarketListingID.String()+`","quantity":1,"total_amount":1},"client_seq":3,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed market buy error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, conn, `{"request_id":"request-market-buy","op":"market.buy","payload":{"listing_id":"`+seedMarketListingID.String()+`","quantity":2},"client_seq":4,"v":1}`)
	buyResponse := readResponse(t, conn)
	if !buyResponse.OK {
		t.Fatalf("market buy response = %+v, want success", buyResponse)
	}
	assertNoEconomyLeak(t, "market buy", buyResponse.Payload)
	var buyPayload marketMutationPayload
	if err := json.Unmarshal(buyResponse.Payload, &buyPayload); err != nil {
		t.Fatalf("decode market buy: %v", err)
	}
	if buyPayload.ServerTotal != 50 || buyPayload.Wallet.Credits != starterWalletCredits-50 {
		t.Fatalf("market buy = %+v, want server-calculated total and debited wallet", buyPayload)
	}
	if len(buyPayload.Inventory.Stackable) != 1 ||
		buyPayload.Inventory.Stackable[0].ItemID != "raw_ore" ||
		buyPayload.Inventory.Stackable[0].Quantity != 2 ||
		buyPayload.Inventory.Stackable[0].Location != economy.LocationKindAccountInventory.String() {
		t.Fatalf("market buy inventory = %+v, want purchased raw ore in account inventory", buyPayload.Inventory)
	}
	drainEventTypes(t, conn, realtime.EventMarketSaleCompleted, realtime.EventWalletSnapshot, realtime.EventInventorySnapshot)

	writeText(t, conn, `{"request_id":"request-auction-search","op":"auction.search","payload":{},"client_seq":5,"v":1}`)
	auctionResponse := readResponse(t, conn)
	if !auctionResponse.OK {
		t.Fatalf("auction search response = %+v, want success", auctionResponse)
	}
	assertNoEconomyLeak(t, "auction search", auctionResponse.Payload)
	var auctionPayload struct {
		Auction auctionSearchPayload `json:"auction"`
	}
	if err := json.Unmarshal(auctionResponse.Payload, &auctionPayload); err != nil {
		t.Fatalf("decode auction search: %v", err)
	}
	if len(auctionPayload.Auction.Lots) != 1 || auctionPayload.Auction.Lots[0].AuctionID != seedAuctionID.String() {
		t.Fatalf("auction lots = %+v, want seeded lot", auctionPayload.Auction.Lots)
	}

	writeText(t, conn, `{"request_id":"request-auction-bid","op":"auction.bid","payload":{"auction_id":"`+seedAuctionID.String()+`","amount":300},"client_seq":6,"v":1}`)
	bidResponse := readResponse(t, conn)
	if !bidResponse.OK {
		t.Fatalf("auction bid response = %+v, want success", bidResponse)
	}
	var bidPayload auctionMutationPayload
	if err := json.Unmarshal(bidResponse.Payload, &bidPayload); err != nil {
		t.Fatalf("decode auction bid: %v", err)
	}
	if bidPayload.Amount != 300 || bidPayload.Wallet.Credits != starterWalletCredits-50-300 || !bidPayload.Lot.Leading {
		t.Fatalf("auction bid = %+v, want debited leading bid", bidPayload)
	}
	drainEventTypes(t, conn, realtime.EventAuctionBidPlaced, realtime.EventAuctionLotUpdated, realtime.EventWalletSnapshot)

	writeText(t, conn, `{"request_id":"request-auction-buy-now","op":"auction.buy_now","payload":{"auction_id":"`+seedAuctionID.String()+`"},"client_seq":7,"v":1}`)
	buyNowResponse := readResponse(t, conn)
	if !buyNowResponse.OK {
		t.Fatalf("auction buy-now response = %+v, want success", buyNowResponse)
	}
	var buyNowPayload auctionMutationPayload
	if err := json.Unmarshal(buyNowResponse.Payload, &buyNowPayload); err != nil {
		t.Fatalf("decode auction buy-now: %v", err)
	}
	if buyNowPayload.Price != 650 || buyNowPayload.Grant == nil || buyNowPayload.Wallet.Credits != 500 {
		t.Fatalf("auction buy-now = %+v, want server price, grant, and refunded current bid", buyNowPayload)
	}
	drainEventTypes(t, conn, realtime.EventAuctionClosed, realtime.EventAuctionLotUpdated, realtime.EventWalletSnapshot)

	writeText(t, conn, `{"request_id":"request-auction-grants","op":"auction.claim_grant","payload":{},"client_seq":8,"v":1}`)
	grantsResponse := readResponse(t, conn)
	if !grantsResponse.OK {
		t.Fatalf("auction grant response = %+v, want success", grantsResponse)
	}
	var grantsPayload struct {
		Auction auctionSearchPayload `json:"auction"`
	}
	if err := json.Unmarshal(grantsResponse.Payload, &grantsPayload); err != nil {
		t.Fatalf("decode auction grants: %v", err)
	}
	if len(grantsPayload.Auction.Grants) != 1 || grantsPayload.Auction.Grants[0].AuctionID != seedAuctionID.String() {
		t.Fatalf("auction grants = %+v, want player grant snapshot", grantsPayload.Auction.Grants)
	}

	writeText(t, conn, `{"request_id":"request-premium-entitlements","op":"premium.entitlements","payload":{},"client_seq":9,"v":1}`)
	premiumResponse := readResponse(t, conn)
	if !premiumResponse.OK {
		t.Fatalf("premium response = %+v, want success", premiumResponse)
	}
	assertNoEconomyLeak(t, "premium entitlements", premiumResponse.Payload)
	var premiumPayload struct {
		Premium premiumSummaryPayload `json:"premium"`
	}
	if err := json.Unmarshal(premiumResponse.Payload, &premiumPayload); err != nil {
		t.Fatalf("decode premium: %v", err)
	}
	if len(premiumPayload.Premium.Entitlements) != 1 || premiumPayload.Premium.Entitlements[0].State != premium.EntitlementStatePending.String() {
		t.Fatalf("premium entitlements = %+v, want one pending entitlement", premiumPayload.Premium.Entitlements)
	}
	entitlementID := premiumPayload.Premium.Entitlements[0].EntitlementID

	writeText(t, conn, `{"request_id":"request-premium-claim","op":"premium.claim","payload":{"entitlement_id":"`+entitlementID+`"},"client_seq":10,"v":1}`)
	claimResponse := readResponse(t, conn)
	if !claimResponse.OK {
		t.Fatalf("premium claim response = %+v, want success", claimResponse)
	}
	var claimPayload premiumMutationPayload
	if err := json.Unmarshal(claimResponse.Payload, &claimPayload); err != nil {
		t.Fatalf("decode premium claim: %v", err)
	}
	if claimPayload.Wallet.PremiumEarned != 50 || claimPayload.Premium.Entitlements[0].State != premium.EntitlementStateClaimed.String() {
		t.Fatalf("premium claim = %+v, want earned premium credit and claimed state", claimPayload)
	}
	drainEventTypes(t, conn, realtime.EventPremiumClaimed, realtime.EventWalletSnapshot)

	writeText(t, conn, `{"request_id":"request-weekly-xcore","op":"premium.purchase_weekly_xcore","payload":{},"client_seq":11,"v":1}`)
	xcoreResponse := readResponse(t, conn)
	if !xcoreResponse.OK {
		t.Fatalf("weekly xcore response = %+v, want success", xcoreResponse)
	}
	var xcorePayload premiumMutationPayload
	if err := json.Unmarshal(xcoreResponse.Payload, &xcorePayload); err != nil {
		t.Fatalf("decode weekly xcore: %v", err)
	}
	if xcorePayload.Wallet.PremiumPaid != starterWalletPremiumPaid-weeklyXCorePremiumPrice || len(xcorePayload.Premium.Purchases) != 1 {
		t.Fatalf("weekly xcore = %+v, want paid premium debit and purchase row", xcorePayload)
	}
	if len(xcorePayload.Premium.Stock) != 1 || xcorePayload.Premium.Stock[0].StockRemaining != weeklyXCoreStockTotal-1 {
		t.Fatalf("weekly xcore stock = %+v, want stock decrement", xcorePayload.Premium.Stock)
	}
	drainEventTypes(t, conn, realtime.EventPremiumStockConsumed, realtime.EventWalletSnapshot)

	writeText(t, conn, `{"request_id":"request-weekly-xcore-again","op":"premium.purchase_weekly_xcore","payload":{},"client_seq":12,"v":1}`)
	limit := readError(t, conn)
	if limit.Error.Code != foundation.CodeForbidden {
		t.Fatalf("second weekly xcore error = %+v, want %s", limit.Error, foundation.CodeForbidden)
	}

	writeText(t, conn, `{"request_id":"request-admin-economy","op":"admin.economy_dashboard","payload":{},"client_seq":13,"v":1}`)
	admin := readError(t, conn)
	if admin.Error.Code != foundation.CodeForbidden {
		t.Fatalf("non-admin dashboard error = %+v, want %s", admin.Error, foundation.CodeForbidden)
	}

	snapshot := gameServer.runtime.Metrics.Snapshot()
	requireMetricCounter(t, snapshot, observability.MetricMarketVolume, 50, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "raw_ore"},
	})
	requireMetricCounter(t, snapshot, observability.MetricMarketQuantity, 2, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "raw_ore"},
	})
	requireMetricCounter(t, snapshot, observability.MetricMarketSales, 1, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "raw_ore"},
	})
	requireMetricCounter(t, snapshot, observability.MetricAuctionVolume, 300, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
	})
	requireMetricCounter(t, snapshot, observability.MetricAuctionClearingVolume, 650, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "x_core_fragment_bundle"},
	})
	requireMetricCounter(t, snapshot, observability.MetricAuctionClearingQuantity, 2, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "x_core_fragment_bundle"},
	})
	requireMetricCounter(t, snapshot, observability.MetricAuctionClears, 1, []observability.Label{
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "item_id", Value: "x_core_fragment_bundle"},
	})
	requireMetricCounter(t, snapshot, observability.MetricWalletDeltaByReason, 50, []observability.Label{
		{Name: "action", Value: economy.LedgerActionIncrease.String()},
		{Name: "currency_type", Value: economy.CurrencyBucketPremiumEarned.String()},
		{Name: "reason", Value: premium.LedgerReasonPremiumEntitlementClaim.String()},
	})
	requireMetricCounter(t, snapshot, observability.MetricWalletDeltaByReason, weeklyXCorePremiumPrice, []observability.Label{
		{Name: "action", Value: economy.LedgerActionDecrease.String()},
		{Name: "currency_type", Value: economy.CurrencyBucketPremiumPaid.String()},
		{Name: "reason", Value: premium.LedgerReasonPremiumWeeklyXCore.String()},
	})
}

func TestPhase09QuestAdminObservabilityUseServerState(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	if _, err := gameServer.runtime.Auth.SeedAdmin(context.Background(), auth.AdminSeedInput{
		Enabled:  true,
		Email:    "admin@example.com",
		Password: "admin-password",
		Callsign: "Ops-Admin",
	}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)

	writeText(t, conn, `{"request_id":"request-quest-board","op":"quest.board","payload":{},"client_seq":1,"v":1}`)
	boardResponse := readResponse(t, conn)
	if !boardResponse.OK {
		t.Fatalf("quest board response = %+v, want success", boardResponse)
	}
	assertNoPhase09Leak(t, "quest board", boardResponse.Payload)
	var boardPayload struct {
		QuestBoard questBoardSummaryPayload `json:"quest_board"`
	}
	if err := json.Unmarshal(boardResponse.Payload, &boardPayload); err != nil {
		t.Fatalf("decode quest board: %v", err)
	}
	if len(boardPayload.QuestBoard.Offers) != quests.BoardOfferCount || boardPayload.QuestBoard.RerollCost.Amount <= 0 {
		t.Fatalf("quest board = %+v, want ten server offers and reroll cost", boardPayload.QuestBoard)
	}
	drainEventTypes(t, conn, realtime.EventQuestBoardGenerated)

	writeText(t, conn, `{"request_id":"request-quest-progress-spoof","op":"quest.progress","payload":{"progress":{"current":999,"completed":true}},"client_seq":2,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("quest progress spoof error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	offer := progressableQuestOfferWithItemReward(t, boardPayload.QuestBoard.Offers)
	itemReward := questItemReward(t, offer.Rewards)
	writeText(t, conn, `{"request_id":"request-quest-accept","op":"quest.accept","payload":{"offer_id":"`+offer.OfferID+`"},"client_seq":3,"v":1}`)
	acceptResponse := readResponse(t, conn)
	if !acceptResponse.OK {
		t.Fatalf("quest accept response = %+v, want success", acceptResponse)
	}
	var accepted questMutationPayload
	if err := json.Unmarshal(acceptResponse.Payload, &accepted); err != nil {
		t.Fatalf("decode quest accept: %v", err)
	}
	if accepted.Quest == nil || accepted.Quest.QuestID == "" || accepted.Quest.State != quests.QuestStateAccepted.String() {
		t.Fatalf("accepted quest = %+v, want accepted quest", accepted.Quest)
	}
	drainEventTypes(t, conn, realtime.EventQuestAccepted)

	completeQuestWithServerEvents(t, gameServer, resolved.PlayerID, *accepted.Quest)
	writeText(t, conn, `{"request_id":"request-quest-progress","op":"quest.progress","payload":{},"client_seq":4,"v":1}`)
	progressResponse := readResponse(t, conn)
	if !progressResponse.OK {
		t.Fatalf("quest progress response = %+v, want success", progressResponse)
	}
	var progressPayload struct {
		QuestBoard questBoardSummaryPayload `json:"quest_board"`
	}
	if err := json.Unmarshal(progressResponse.Payload, &progressPayload); err != nil {
		t.Fatalf("decode quest progress: %v", err)
	}
	if progressPayload.QuestBoard.Counts.Claimable != 1 {
		t.Fatalf("quest counts = %+v, want one claimable quest", progressPayload.QuestBoard.Counts)
	}

	writeText(t, conn, `{"request_id":"request-quest-claim","op":"quest.claim_reward","payload":{"quest_id":"`+accepted.Quest.QuestID+`"},"client_seq":5,"v":1}`)
	claimResponse := readResponse(t, conn)
	if !claimResponse.OK {
		t.Fatalf("quest claim response = %+v, want success", claimResponse)
	}
	var claim questMutationPayload
	if err := json.Unmarshal(claimResponse.Payload, &claim); err != nil {
		t.Fatalf("decode quest claim: %v", err)
	}
	if claim.Quest == nil || claim.Quest.State != quests.QuestStateClaimed.String() || claim.Wallet.Credits <= starterWalletCredits || claim.Progression == nil || claim.Progression.MainXP == 0 {
		t.Fatalf("quest claim = %+v, want claimed quest, credits, and XP", claim)
	}
	if claim.Inventory == nil || len(claim.Inventory.Stackable) == 0 {
		t.Fatalf("quest claim inventory = %+v, want reward item grant", claim.Inventory)
	}
	assertQuestRewardInventorySnapshot(t, *claim.Inventory, itemReward)
	questRewardReference, err := foundation.QuestRewardIdempotencyKey(foundation.QuestID(accepted.Quest.QuestID))
	if err != nil {
		t.Fatalf("quest reward reference: %v", err)
	}
	questRewardLedger := questRewardItemLedgerEntries(gameServer, resolved.PlayerID, questRewardReference)
	if len(questRewardLedger) != 1 {
		t.Fatalf("quest reward item ledger entries = %+v, want one AddItem ledger entry for %s", questRewardLedger, questRewardReference)
	}
	assertQuestRewardLedgerEntry(t, questRewardLedger[0], itemReward, questRewardReference)
	drainEventTypes(t, conn, realtime.EventQuestRewardClaimed, realtime.EventWalletSnapshot, realtime.EventInventorySnapshot, realtime.EventProgressionSnapshot)

	writeText(t, conn, `{"request_id":"request-quest-claim-duplicate","op":"quest.claim_reward","payload":{"quest_id":"`+accepted.Quest.QuestID+`"},"client_seq":6,"v":1}`)
	duplicateClaimResponse := readResponse(t, conn)
	if !duplicateClaimResponse.OK {
		t.Fatalf("duplicate quest claim response = %+v, want success", duplicateClaimResponse)
	}
	var duplicateClaim questMutationPayload
	if err := json.Unmarshal(duplicateClaimResponse.Payload, &duplicateClaim); err != nil {
		t.Fatalf("decode duplicate quest claim: %v", err)
	}
	if !duplicateClaim.Duplicate || duplicateClaim.Quest == nil || duplicateClaim.Quest.State != quests.QuestStateClaimed.String() {
		t.Fatalf("duplicate quest claim = %+v, want duplicate claimed result", duplicateClaim)
	}
	if got := questRewardItemLedgerEntries(gameServer, resolved.PlayerID, questRewardReference); len(got) != len(questRewardLedger) {
		t.Fatalf("duplicate quest claim ledger entries = %+v, want unchanged from %+v", got, questRewardLedger)
	}
	drainEventTypes(t, conn, realtime.EventQuestRewardClaimed, realtime.EventWalletSnapshot, realtime.EventInventorySnapshot, realtime.EventProgressionSnapshot)

	writeText(t, conn, `{"request_id":"request-quest-reroll","op":"quest.reroll","payload":{},"client_seq":7,"v":1}`)
	rerollResponse := readResponse(t, conn)
	if !rerollResponse.OK {
		t.Fatalf("quest reroll response = %+v, want success", rerollResponse)
	}
	var reroll questMutationPayload
	if err := json.Unmarshal(rerollResponse.Payload, &reroll); err != nil {
		t.Fatalf("decode quest reroll: %v", err)
	}
	if len(reroll.QuestBoard.Offers) != quests.BoardOfferCount || reroll.Wallet.Credits >= claim.Wallet.Credits {
		t.Fatalf("quest reroll = %+v, want fresh board and wallet debit", reroll)
	}
	drainEventTypes(t, conn, realtime.EventQuestBoardRerolled, realtime.EventWalletSnapshot)

	for _, request := range []struct {
		name string
		body string
	}{
		{"admin inspect", `{"request_id":"request-non-admin-inspect","op":"admin.inspect_player","payload":{},"client_seq":8,"v":1}`},
		{"admin repair", `{"request_id":"request-non-admin-repair","op":"admin.repair_craft_job","payload":{"job_id":"job-missing"},"client_seq":9,"v":1}`},
		{"command log", `{"request_id":"request-non-admin-log","op":"observability.command_log","payload":{},"client_seq":10,"v":1}`},
		{"metrics", `{"request_id":"request-non-admin-metrics","op":"observability.metrics","payload":{},"client_seq":11,"v":1}`},
		{"release gate", `{"request_id":"request-non-admin-gate","op":"observability.release_gate","payload":{},"client_seq":12,"v":1}`},
		{"abuse coverage", `{"request_id":"request-non-admin-abuse","op":"observability.abuse_coverage","payload":{},"client_seq":13,"v":1}`},
	} {
		t.Run("non-admin "+request.name, func(t *testing.T) {
			writeText(t, conn, request.body)
			got := readError(t, conn)
			if got.Error.Code != foundation.CodeForbidden {
				t.Fatalf("%s error = %+v, want %s", request.name, got.Error, foundation.CodeForbidden)
			}
		})
	}

	adminCookie := loginPilot(t, httpServer, "admin@example.com", "admin-password")
	adminConn := dialWebSocket(t, httpServer, adminCookie)
	defer adminConn.CloseNow()
	readBootstrapEvents(t, adminConn)

	adminRequests := []struct {
		name       string
		body       string
		decode     func(*testing.T, json.RawMessage)
		eventTypes []realtime.ClientEventType
	}{
		{
			name: "inspect",
			body: `{"request_id":"request-admin-inspect","op":"admin.inspect_player","payload":{},"client_seq":1,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Admin adminPlayerInspectionPayload `json:"admin"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode admin inspect: %v", err)
				}
				if payload.Admin.Target != "self" || len(payload.Admin.Wallet.Balances) == 0 {
					t.Fatalf("admin inspect = %+v, want self wallet balances", payload.Admin)
				}
			},
		},
		{
			name: "economy",
			body: `{"request_id":"request-admin-economy-ok","op":"admin.economy_dashboard","payload":{},"client_seq":2,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Economy economyDashboardPayload `json:"economy"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode admin economy: %v", err)
				}
				if payload.Economy.Wallets.Credits == 0 {
					t.Fatalf("admin economy = %+v, want wallet totals", payload.Economy)
				}
			},
		},
		{
			name: "repair",
			body: `{"request_id":"request-admin-repair","op":"admin.repair_craft_job","payload":{"job_id":"job-missing"},"client_seq":3,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Repair adminRepairCraftJobPayload `json:"admin_repair"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode admin repair: %v", err)
				}
				if payload.Repair.Status != "unavailable" {
					t.Fatalf("admin repair = %+v, want unavailable runtime status", payload.Repair)
				}
			},
			eventTypes: []realtime.ClientEventType{realtime.EventAdminActionCompleted},
		},
		{
			name: "command log",
			body: `{"request_id":"request-admin-command-log","op":"observability.command_log","payload":{},"client_seq":4,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					CommandLog commandLogSummaryPayload `json:"command_log"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode command log: %v", err)
				}
				if payload.CommandLog.Total == 0 || len(payload.CommandLog.Entries) == 0 {
					t.Fatalf("command log = %+v, want recorded commands", payload.CommandLog)
				}
			},
		},
		{
			name: "metrics",
			body: `{"request_id":"request-admin-metrics","op":"observability.metrics","payload":{},"client_seq":5,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Metrics metricsSummaryPayload `json:"metrics"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode metrics: %v", err)
				}
				if len(payload.Metrics.Snapshot.Counters) == 0 {
					t.Fatalf("metrics = %+v, want command counters", payload.Metrics)
				}
			},
			eventTypes: []realtime.ClientEventType{realtime.EventObservabilityMetric},
		},
		{
			name: "release gate",
			body: `{"request_id":"request-admin-release-gate","op":"observability.release_gate","payload":{},"client_seq":6,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					ReleaseGate releaseGatePayload `json:"release_gate"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode release gate: %v", err)
				}
				if !payload.ReleaseGate.Report.Passed || len(payload.ReleaseGate.Coverage) == 0 {
					t.Fatalf("release gate = %+v, want passing coverage", payload.ReleaseGate.Report)
				}
			},
			eventTypes: []realtime.ClientEventType{realtime.EventReleaseGateUpdated},
		},
		{
			name: "abuse",
			body: `{"request_id":"request-admin-abuse","op":"observability.abuse_coverage","payload":{},"client_seq":7,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Abuse abuseCoveragePayload `json:"abuse_coverage"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode abuse coverage: %v", err)
				}
				if !payload.Abuse.Report.Passed || len(payload.Abuse.Coverage) == 0 {
					t.Fatalf("abuse coverage = %+v, want passing coverage", payload.Abuse.Report)
				}
			},
		},
	}

	for _, request := range adminRequests {
		t.Run("admin "+request.name, func(t *testing.T) {
			writeText(t, adminConn, request.body)
			response := readResponse(t, adminConn)
			if !response.OK {
				t.Fatalf("%s response = %+v, want success", request.name, response)
			}
			assertNoPhase09Leak(t, request.name, response.Payload)
			request.decode(t, response.Payload)
			if len(request.eventTypes) > 0 {
				drainEventTypes(t, adminConn, request.eventTypes...)
			}
		})
	}

	snapshot := gameServer.runtime.Metrics.Snapshot()
	requireMetricCounter(t, snapshot, observability.MetricQuestRewardsClaimed, 1, []observability.Label{
		{Name: "reward_type", Value: quests.RewardKindCredits.String()},
	})
	requireMetricCounter(t, snapshot, observability.MetricQuestRewardsClaimed, 1, []observability.Label{
		{Name: "reward_type", Value: quests.RewardKindMainXP.String()},
	})
	requireMetricCounter(t, snapshot, observability.MetricQuestRewardsClaimed, 1, []observability.Label{
		{Name: "reward_type", Value: quests.RewardKindItem.String()},
	})
	requireMetricCounter(t, snapshot, observability.MetricWalletDeltaByReason, claim.Wallet.Credits-starterWalletCredits, []observability.Label{
		{Name: "action", Value: economy.LedgerActionIncrease.String()},
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "reason", Value: runtimeQuestRewardLedgerReason.String()},
	})
}

func TestCombatRejectsClientAuthoredGameplayTruth(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-combat-spoof","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc","player_id":"spoofed-player","damage":9999},"client_seq":1,"v":1}`)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed combat error = %+v, want %s", got.Error, foundation.CodeInvalidPayload)
	}
}

func TestCombatRejectsHiddenOutOfRangeAndDisabledWithoutEnergySpend(t *testing.T) {
	t.Run("hidden target", func(t *testing.T) {
		gameServer, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		cookie := registerPilot(t, httpServer)
		conn := dialWebSocket(t, httpServer, cookie)
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)
		resolved := resolvedSessionForCookie(t, gameServer, cookie)
		setTestHidden(gameServer, "entity_training_npc", true)

		writeText(t, conn, `{"request_id":"request-combat-hidden","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeNotVisible {
			t.Fatalf("hidden combat error = %+v, want %s", got.Error, foundation.CodeNotVisible)
		}
		if capacitor := testShipCapacitor(gameServer, resolved.PlayerID); capacitor != 100 {
			t.Fatalf("hidden combat capacitor = %d, want unchanged", capacitor)
		}
	})

	t.Run("out of range", func(t *testing.T) {
		gameServer, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		cookie := registerPilot(t, httpServer)
		conn := dialWebSocket(t, httpServer, cookie)
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)
		resolved := resolvedSessionForCookie(t, gameServer, cookie)
		setTestWeaponRange(gameServer, resolved.PlayerID, 10)

		writeText(t, conn, `{"request_id":"request-combat-range","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeOutOfRange {
			t.Fatalf("out-of-range combat error = %+v, want %s", got.Error, foundation.CodeOutOfRange)
		}
		if capacitor := testShipCapacitor(gameServer, resolved.PlayerID); capacitor != 100 {
			t.Fatalf("out-of-range combat capacitor = %d, want unchanged", capacitor)
		}
	})

	t.Run("disabled ship", func(t *testing.T) {
		gameServer, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		cookie := registerPilot(t, httpServer)
		conn := dialWebSocket(t, httpServer, cookie)
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)
		resolved := resolvedSessionForCookie(t, gameServer, cookie)
		setTestShipDisabled(gameServer, resolved.PlayerID, true)

		writeText(t, conn, `{"request_id":"request-combat-disabled","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeShipDisabled {
			t.Fatalf("disabled combat error = %+v, want %s", got.Error, foundation.CodeShipDisabled)
		}
		if capacitor := testShipCapacitor(gameServer, resolved.PlayerID); capacitor != 100 {
			t.Fatalf("disabled combat capacitor = %d, want unchanged", capacitor)
		}
	})
}

func TestLootPickupRejectsOutOfRangeDropWithoutCargoMutation(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	dropID := killTrainingNPCForDrop(t, conn)
	moveTestPlayerEntity(gameServer, resolved.PlayerID, world.Vec2{X: 1000, Y: 0})
	setTestRadarRange(gameServer, resolved.PlayerID, 2000)

	writeText(t, conn, `{"request_id":"request-loot-far","op":"loot.pickup","payload":{"drop_id":"`+dropID+`"},"client_seq":2,"v":1}`)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeOutOfRange {
		t.Fatalf("out-of-range pickup error = %+v, want %s", got.Error, foundation.CodeOutOfRange)
	}
	if used := testCargoUsed(gameServer, resolved.PlayerID); used != 0 {
		t.Fatalf("out-of-range pickup cargo used = %d, want unchanged", used)
	}
}

func TestRepairQuoteAndRepairUseServerOwnedActiveShip(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	setTestShipDisabled(gameServer, resolved.PlayerID, true)

	writeText(t, conn, `{"request_id":"request-repair-quote","op":"death.repair_quote","payload":{},"client_seq":1,"v":1}`)
	quoteResponse := readResponse(t, conn)
	if !quoteResponse.OK {
		t.Fatalf("repair quote response = %+v, want success", quoteResponse)
	}
	var quote repairQuotePayload
	if err := json.Unmarshal(quoteResponse.Payload, &quote); err != nil {
		t.Fatalf("decode quote: %v", err)
	}
	if !quote.Disabled || quote.ShipID != starterShipID.String() || quote.Cost != 0 {
		t.Fatalf("repair quote = %+v, want disabled free starter repair", quote)
	}

	writeText(t, conn, `{"request_id":"request-repair-ship","op":"death.repair_ship","payload":{},"client_seq":2,"v":1}`)
	repairResponse := readResponse(t, conn)
	if !repairResponse.OK {
		t.Fatalf("repair response = %+v, want success", repairResponse)
	}
	var repaired struct {
		Repaired bool `json:"repaired"`
		Ship     struct {
			Disabled bool `json:"disabled"`
			Hull     int  `json:"hull"`
		} `json:"ship"`
	}
	if err := json.Unmarshal(repairResponse.Payload, &repaired); err != nil {
		t.Fatalf("decode repair response: %v", err)
	}
	if !repaired.Repaired || repaired.Ship.Disabled || repaired.Ship.Hull != 100 {
		t.Fatalf("repair payload = %+v, want restored ship", repaired)
	}
	seen := map[realtime.ClientEventType]bool{}
	for attempts := 0; attempts < 6 &&
		(!seen[realtime.EventDeathRepaired] ||
			!seen[realtime.EventShipSnapshot] ||
			!seen[realtime.EventPlayerSnapshot] ||
			!seen[realtime.EventWalletSnapshot]); attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
	}
	for _, want := range []realtime.ClientEventType{realtime.EventDeathRepaired, realtime.EventShipSnapshot, realtime.EventPlayerSnapshot, realtime.EventWalletSnapshot} {
		if !seen[want] {
			t.Fatalf("repair events seen = %#v, missing %s", seen, want)
		}
	}
}

func TestAOIDiffEventsAreFilteredPerSession(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "aoi-filter@example.com", "AOI-Filter")

	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[resolved.PlayerID]
	state.Stats.RadarRange = 10
	gameServer.runtime.players[resolved.PlayerID] = state
	gameServer.runtime.mu.Unlock()

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	var initial worldSnapshotPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &initial); err != nil {
		t.Fatalf("decode initial world snapshot: %v", err)
	}
	if len(initial.Entities) != 1 || !hasStatusFlag(initial.Entities[0].StatusFlags, "self") {
		t.Fatalf("initial filtered entities = %+v, want only self", initial.Entities)
	}

	gameServer.runtime.mu.Lock()
	state = gameServer.runtime.players[resolved.PlayerID]
	state.Stats.RadarRange = defaultRadarRange
	gameServer.runtime.players[resolved.PlayerID] = state
	gameServer.runtime.mu.Unlock()

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	sessionEvents := eventsBySession[resolved.SessionID]
	if len(sessionEvents) == 0 || sessionEvents[0].Type != realtime.EventAOIEntityEntered {
		t.Fatalf("AOI events after radar increase = %+v, want entity entered", sessionEvents)
	}
	var entered aoiEntityPayloadForTest
	if err := json.Unmarshal(sessionEvents[0].Payload, &entered); err != nil {
		t.Fatalf("decode entered event: %v", err)
	}
	if entered.EntityID != "entity_training_npc" || entered.EntityType != "npc" {
		t.Fatalf("entered entity = %+v, want training npc", entered)
	}
}

func TestTwoPlayersWithDifferentRadarReceiveDifferentFilteredSnapshots(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	limited := createResolvedRuntimeSession(t, gameServer, "limited-radar@example.com", "Limited")
	defaultRadar := createResolvedRuntimeSession(t, gameServer, "default-radar@example.com", "Default")

	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[limited.PlayerID]
	state.Stats.RadarRange = 10
	gameServer.runtime.players[limited.PlayerID] = state
	gameServer.runtime.mu.Unlock()

	limitedEvents, err := gameServer.runtime.bootstrapEvents(limited)
	if err != nil {
		t.Fatalf("limited bootstrap events: %v", err)
	}
	defaultEvents, err := gameServer.runtime.bootstrapEvents(defaultRadar)
	if err != nil {
		t.Fatalf("default bootstrap events: %v", err)
	}
	limitedSnapshot := decodeWorldSnapshotForTest(t, limitedEvents)
	defaultSnapshot := decodeWorldSnapshotForTest(t, defaultEvents)

	if hasEntityID(limitedSnapshot.Entities, "entity_training_npc") {
		t.Fatalf("limited radar snapshot included training npc: %+v", limitedSnapshot.Entities)
	}
	if !hasEntityID(defaultSnapshot.Entities, "entity_training_npc") {
		t.Fatalf("default radar snapshot missing training npc: %+v", defaultSnapshot.Entities)
	}
}

func TestMultiTabAttachDoesNotDuplicatePlayerEntity(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)

	first := dialWebSocket(t, httpServer, cookie)
	defer first.CloseNow()
	firstEvents := readBootstrapEvents(t, first)
	second := dialWebSocket(t, httpServer, cookie)
	defer second.CloseNow()
	secondEvents := readBootstrapEvents(t, second)

	for _, events := range [][]realtime.EventEnvelope{firstEvents, secondEvents} {
		var snapshot worldSnapshotPayload
		if err := json.Unmarshal(events[len(events)-1].Payload, &snapshot); err != nil {
			t.Fatalf("decode world snapshot: %v", err)
		}
		playerCount := 0
		for _, entity := range snapshot.Entities {
			if entity.Type == "player" {
				playerCount++
			}
		}
		if playerCount != 1 {
			t.Fatalf("player entities = %d in %+v, want 1", playerCount, snapshot.Entities)
		}
	}
}

func TestDuplicateRequestIDReturnsCachedWebSocketResponse(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	request := `{"request_id":"request-snapshot-1","op":"world.snapshot","payload":{},"client_seq":1,"v":1}`
	writeText(t, conn, request)
	first := readRawText(t, conn)
	writeText(t, conn, request)
	second := readRawText(t, conn)

	if !bytes.Equal(first, second) {
		t.Fatalf("duplicate response changed:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestBadPayloadReturnsSafeErrorAndLogoutRejectsFurtherCommands(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-bad-1","op":"move_to","payload":{"target":{"x":"bad","y":0}},"client_seq":1,"v":1}`)
	bad := readError(t, conn)
	if bad.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("bad payload error = %+v, want %s", bad.Error, foundation.CodeInvalidPayload)
	}

	logoutPilot(t, httpServer, cookie)
	writeText(t, conn, `{"request_id":"request-after-logout","op":"world.snapshot","payload":{},"client_seq":2,"v":1}`)
	revoked := readError(t, conn)
	if revoked.Error.Code != foundation.CodeSessionRevoked {
		t.Fatalf("after logout error = %+v, want %s", revoked.Error, foundation.CodeSessionRevoked)
	}
}

func TestBadJSONDoesNotCrashSocketLoop(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `not-json`)
	bad := readError(t, conn)
	if bad.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("bad JSON error = %+v, want %s", bad.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, conn, `{"request_id":"request-after-bad-json","op":"world.snapshot","payload":{},"client_seq":2,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("response after bad JSON = %+v, want success", response)
	}
}

func TestReconnectBootstrapCarriesSnapshotCursor(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)

	firstConn := dialWebSocket(t, httpServer, cookie)
	firstEvents := readBootstrapEvents(t, firstConn)
	firstConn.CloseNow()

	secondConn := dialWebSocket(t, httpServer, cookie)
	defer secondConn.CloseNow()
	secondEvents := readBootstrapEvents(t, secondConn)

	if secondEvents[0].Sequence <= firstEvents[len(firstEvents)-1].Sequence {
		t.Fatalf("reconnect first seq = %d, want after %d", secondEvents[0].Sequence, firstEvents[len(firstEvents)-1].Sequence)
	}
	var ready sessionReadyPayload
	if err := json.Unmarshal(secondEvents[0].Payload, &ready); err != nil {
		t.Fatalf("decode reconnect session.ready: %v", err)
	}
	if ready.ReconnectCursor != firstEvents[len(firstEvents)-1].Sequence {
		t.Fatalf("reconnect cursor = %d, want %d", ready.ReconnectCursor, firstEvents[len(firstEvents)-1].Sequence)
	}
}

func TestShutdownClosesActiveWebSocket(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	readBootstrapEvents(t, conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := gameServer.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}
	readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
	defer readCancel()
	_, _, err := conn.Read(readCtx)
	if err == nil {
		t.Fatal("Read() after Shutdown succeeded, want closed connection")
	}
}

func newTestServer(t *testing.T, devMode bool) (*Server, *httptest.Server) {
	t.Helper()
	gameServer, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		DevMode:        devMode,
		SessionTTL:     time.Hour,
		TickDelta:      50 * time.Millisecond,
		PasswordHasher: auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	httpServer := httptest.NewServer(gameServer.Handler())
	return gameServer, httpServer
}

func registerPilot(t *testing.T, httpServer *httptest.Server) *http.Cookie {
	t.Helper()
	body := strings.NewReader(`{"email":"pilot@example.com","password":"correct-password","callsign":"Frontier-01"}`)
	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/auth/register", body)
	if err != nil {
		t.Fatalf("new register request: %v", err)
	}
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("register request error = %v, want nil", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d, want 201", resp.StatusCode)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == auth.DefaultSessionCookieName {
			return cookie
		}
	}
	t.Fatal("register response missing session cookie")
	return nil
}

func loginPilot(t *testing.T, httpServer *httptest.Server, email string, password string) *http.Cookie {
	t.Helper()
	body := strings.NewReader(`{"email":"` + email + `","password":"` + password + `"}`)
	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/auth/login", body)
	if err != nil {
		t.Fatalf("new login request: %v", err)
	}
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login request error = %v, want nil", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == auth.DefaultSessionCookieName {
			return cookie
		}
	}
	t.Fatal("login response missing session cookie")
	return nil
}

func resolvedSessionForCookie(t *testing.T, gameServer *Server, cookie *http.Cookie) auth.ResolvedSession {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	if err != nil {
		t.Fatalf("new resolve request: %v", err)
	}
	req.Header.Set("Origin", testOrigin)
	req.AddCookie(cookie)
	resolved, err := auth.ResolveCookie(context.Background(), gameServer.runtime.Auth, auth.DefaultSessionCookieName, gameServer.config.originPolicy(), req)
	if err != nil {
		t.Fatalf("resolve test cookie: %v", err)
	}
	return resolved
}

func createResolvedRuntimeSession(t *testing.T, gameServer *Server, email string, callsign string) auth.ResolvedSession {
	t.Helper()
	result, err := gameServer.runtime.Auth.Register(context.Background(), auth.RegisterInput{
		Email:    email,
		Password: "correct-password",
		Callsign: callsign,
	})
	if err != nil {
		t.Fatalf("register runtime session: %v", err)
	}
	if err := gameServer.runtime.ensurePlayerSession(result.Session); err != nil {
		t.Fatalf("ensure runtime player session: %v", err)
	}
	return result.Session
}

func setTestShipDisabled(gameServer *Server, playerID foundation.PlayerID, disabled bool) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state := gameServer.runtime.players[playerID]
	state.Ship.Disabled = disabled
	if disabled {
		state.Ship.RepairState = "disabled"
	} else {
		state.Ship.RepairState = "ready"
	}
	gameServer.runtime.players[playerID] = state
}

func setTestHidden(gameServer *Server, entityID world.EntityID, hidden bool) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	gameServer.runtime.hidden[entityID] = hidden
}

func setTestWeaponRange(gameServer *Server, playerID foundation.PlayerID, weaponRange float64) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state := gameServer.runtime.players[playerID]
	state.Stats.WeaponRange = weaponRange
	gameServer.runtime.players[playerID] = state
}

func setTestRadarRange(gameServer *Server, playerID foundation.PlayerID, radarRange float64) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state := gameServer.runtime.players[playerID]
	state.Stats.RadarRange = radarRange
	gameServer.runtime.players[playerID] = state
}

func moveTestPlayerEntity(gameServer *Server, playerID foundation.PlayerID, position world.Vec2) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	entity, ok := gameServer.runtime.Worker.PlayerEntity(playerID)
	if !ok {
		return
	}
	entity.Position = position
	entity.Movement = world.MovementState{}
	_ = gameServer.runtime.Worker.UpdateEntity(entity)
}

func testShipCapacitor(gameServer *Server, playerID foundation.PlayerID) int {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	return gameServer.runtime.players[playerID].Ship.Capacitor
}

func setTestShipCapacitor(gameServer *Server, playerID foundation.PlayerID, capacitor int) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state := gameServer.runtime.players[playerID]
	state.Ship.Capacitor = capacitor
	gameServer.runtime.players[playerID] = state
}

func testCargoUsed(gameServer *Server, playerID foundation.PlayerID) int64 {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	return gameServer.runtime.players[playerID].Cargo.Used
}

func requireMetricCounter(t *testing.T, snapshot observability.MetricSnapshot, name string, value int64, labels []observability.Label) {
	t.Helper()
	for _, counter := range snapshot.Counters {
		if counter.Name != name || !sameMetricLabels(counter.Labels, labels) {
			continue
		}
		if counter.Value != value {
			t.Fatalf("metric %s labels %+v value = %d, want %d", name, labels, counter.Value, value)
		}
		return
	}
	t.Fatalf("missing metric %s labels %+v in snapshot %+v", name, labels, snapshot)
}

func sameMetricLabels(got, want []observability.Label) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func progressableQuestOfferWithItemReward(t *testing.T, offers []questOfferPayload) questOfferPayload {
	t.Helper()
	for _, offer := range offers {
		if questItemRewardCount(offer.Rewards) != 1 {
			continue
		}
		for _, objective := range offer.Objectives {
			switch objective.Kind {
			case quests.ObjectiveKindKill.String(), quests.ObjectiveKindCollect.String(), quests.ObjectiveKindCraft.String():
				if objective.Required > 0 {
					return offer
				}
			}
		}
	}
	t.Fatalf("no progressable kill/collect/craft offer with one item reward in %+v", offers)
	return questOfferPayload{}
}

func questItemRewardCount(rewards []questRewardPayload) int {
	count := 0
	for _, reward := range rewards {
		if reward.Kind == quests.RewardKindItem.String() {
			count++
		}
	}
	return count
}

func questItemReward(t *testing.T, rewards []questRewardPayload) questRewardPayload {
	t.Helper()
	var itemReward questRewardPayload
	for _, reward := range rewards {
		if reward.Kind != quests.RewardKindItem.String() {
			continue
		}
		if itemReward.Kind != "" {
			t.Fatalf("quest rewards = %+v, want exactly one item reward", rewards)
		}
		itemReward = reward
	}
	if itemReward.Kind == "" || itemReward.ItemID == "" || itemReward.Amount <= 0 {
		t.Fatalf("quest rewards = %+v, want positive item reward", rewards)
	}
	return itemReward
}

func assertQuestRewardInventorySnapshot(t *testing.T, inventory inventorySnapshotPayload, reward questRewardPayload) {
	t.Helper()
	for _, stack := range inventory.Stackable {
		if stack.ItemID == reward.ItemID && stack.Location == economy.LocationKindAccountInventory.String() {
			if stack.Quantity != reward.Amount {
				t.Fatalf("quest reward inventory stack = %+v, want quantity %d", stack, reward.Amount)
			}
			return
		}
	}
	t.Fatalf("quest reward inventory = %+v, missing %s x%d in account inventory", inventory, reward.ItemID, reward.Amount)
}

func questRewardItemLedgerEntries(gameServer *Server, playerID foundation.PlayerID, referenceKey foundation.IdempotencyKey) []economy.ItemLedgerEntry {
	entries := gameServer.runtime.Inventory.ItemLedgerEntries()
	matched := make([]economy.ItemLedgerEntry, 0, 1)
	for _, entry := range entries {
		if entry.PlayerID == playerID &&
			entry.Action == economy.LedgerActionIncrease &&
			entry.Reason == runtimeQuestRewardLedgerReason &&
			entry.ReferenceKey == referenceKey {
			matched = append(matched, entry)
		}
	}
	return matched
}

func assertQuestRewardLedgerEntry(t *testing.T, entry economy.ItemLedgerEntry, reward questRewardPayload, referenceKey foundation.IdempotencyKey) {
	t.Helper()
	if entry.ItemID != foundation.ItemID(reward.ItemID) ||
		entry.Quantity.Int64() != reward.Amount ||
		entry.Location.Kind != economy.LocationKindAccountInventory ||
		entry.Action != economy.LedgerActionIncrease ||
		entry.Reason != runtimeQuestRewardLedgerReason ||
		entry.ReferenceKey != referenceKey {
		t.Fatalf("quest reward item ledger = %+v, want %s x%d in account inventory with reference %s", entry, reward.ItemID, reward.Amount, referenceKey)
	}
}

func completeQuestWithServerEvents(t *testing.T, gameServer *Server, playerID foundation.PlayerID, quest questPayload) {
	t.Helper()
	for _, objective := range quest.Objectives {
		switch objective.Kind {
		case quests.ObjectiveKindKill.String():
			for index := int64(0); index < objective.Required; index++ {
				updated, err := gameServer.runtime.Quest.ConsumeCombatNPCKilled(quests.CombatNPCKilledInput{
					EventID:          foundation.EventID(fmt.Sprintf("event-quest-test-kill-%d", index)),
					ProgressEventKey: quests.QuestProgressEventKey(fmt.Sprintf("test.quest.kill:%s:%d", quest.QuestID, index)),
					PlayerID:         playerID,
					NPCType:          objective.Target,
				})
				if err != nil {
					t.Fatalf("complete kill quest: %v", err)
				}
				if index == objective.Required-1 && len(updated) == 0 {
					t.Fatalf("complete kill quest updated no quests on final event")
				}
			}
		case quests.ObjectiveKindCollect.String():
			quantity, err := foundation.NewQuantity(objective.Required)
			if err != nil {
				t.Fatalf("collect quantity: %v", err)
			}
			updated, err := gameServer.runtime.Quest.ConsumeLootPickedUp(quests.LootPickedUpInput{
				EventID:          "event-quest-test-collect",
				ProgressEventKey: quests.QuestProgressEventKey("test.quest.collect:" + quest.QuestID),
				PlayerID:         playerID,
				ItemID:           foundation.ItemID(objective.Target),
				Quantity:         quantity,
			})
			if err != nil {
				t.Fatalf("complete collect quest: %v", err)
			}
			if len(updated) == 0 {
				t.Fatalf("complete collect quest updated no quests")
			}
		case quests.ObjectiveKindCraft.String():
			quantity, err := foundation.NewQuantity(objective.Required)
			if err != nil {
				t.Fatalf("craft quantity: %v", err)
			}
			updated, err := gameServer.runtime.Quest.ConsumeCraftJobCompleted(quests.CraftJobCompletedInput{
				EventID:          "event-quest-test-craft",
				ProgressEventKey: quests.QuestProgressEventKey("test.quest.craft:" + quest.QuestID),
				PlayerID:         playerID,
				RecipeID:         catalog.DefinitionID(objective.Target),
				Quantity:         quantity,
			})
			if err != nil {
				t.Fatalf("complete craft quest: %v", err)
			}
			if len(updated) == 0 {
				t.Fatalf("complete craft quest updated no quests")
			}
		default:
			t.Fatalf("unsupported test quest objective %+v", objective)
		}
	}
}

func killTrainingNPCForDrop(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	writeText(t, conn, `{"request_id":"request-combat-drop","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("combat-for-drop response = %+v, want success", response)
	}

	var dropID string
	seen := map[realtime.ClientEventType]bool{}
	for attempts := 0; attempts < 12 && (!seen[realtime.EventAOIEntityEntered] || !seen[realtime.EventAOIEntityLeft] || dropID == ""); attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
		if event.Type != realtime.EventLootCreated {
			continue
		}
		var payload struct {
			DropID string `json:"drop_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode loot.created: %v", err)
		}
		dropID = payload.DropID
	}
	if dropID == "" {
		t.Fatalf("combat-for-drop events seen = %#v, missing loot.created", seen)
	}
	return dropID
}

func hasStatusFlag(flags []aoi.StatusFlag, want aoi.StatusFlag) bool {
	for _, flag := range flags {
		if flag == want {
			return true
		}
	}
	return false
}

func hasEntityID(entities []aoi.EntityPayload, want string) bool {
	for _, entity := range entities {
		if entity.ID.String() == want {
			return true
		}
	}
	return false
}

func decodeWorldSnapshotForTest(t *testing.T, events []realtime.EventEnvelope) worldSnapshotPayload {
	t.Helper()
	var snapshot worldSnapshotPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &snapshot); err != nil {
		t.Fatalf("decode world snapshot: %v", err)
	}
	return snapshot
}

type aoiEntityPayloadForTest struct {
	EntityID   string `json:"entity_id"`
	EntityType string `json:"entity_type"`
}

func logoutPilot(t *testing.T, httpServer *httptest.Server, cookie *http.Cookie) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/auth/logout", nil)
	if err != nil {
		t.Fatalf("new logout request: %v", err)
	}
	req.Header.Set("Origin", testOrigin)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("logout request error = %v, want nil", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", resp.StatusCode)
	}
}

func dialWebSocket(t *testing.T, httpServer *httptest.Server, cookie *http.Cookie) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(httpServer), &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{testOrigin},
			"Cookie": []string{cookie.String()},
		},
	})
	if err != nil {
		t.Fatalf("websocket dial error = %v, want nil", err)
	}
	return conn
}

func wsURL(httpServer *httptest.Server) string {
	return "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
}

func readBootstrapEvents(t *testing.T, conn *websocket.Conn) []realtime.EventEnvelope {
	t.Helper()
	events := make([]realtime.EventEnvelope, 0, 8)
	for len(events) < 8 {
		events = append(events, readEvent(t, conn))
	}
	return events
}

func writeText(t *testing.T, conn *websocket.Conn, payload string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, []byte(payload)); err != nil {
		t.Fatalf("websocket Write() error = %v, want nil", err)
	}
}

func readRawText(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("websocket Read() error = %v, want nil", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("message type = %v, want text", messageType)
	}
	return data
}

func readEvent(t *testing.T, conn *websocket.Conn) realtime.EventEnvelope {
	t.Helper()
	data := readRawText(t, conn)
	var event realtime.EventEnvelope
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("decode event %s: %v", data, err)
	}
	if event.Type == "" || event.Payload == nil {
		t.Fatalf("message %s is not an event", data)
	}
	return event
}

func drainEventTypes(t *testing.T, conn *websocket.Conn, wants ...realtime.ClientEventType) {
	t.Helper()
	seen := make(map[realtime.ClientEventType]bool, len(wants))
	for range wants {
		event := readEvent(t, conn)
		seen[event.Type] = true
	}
	for _, want := range wants {
		if !seen[want] {
			t.Fatalf("events seen = %#v, missing %s", seen, want)
		}
	}
}

func assertNoEconomyLeak(t *testing.T, label string, payload json.RawMessage) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range []string{
		"seller_player_id",
		"buyer_player_id",
		"bidder_player_id",
		"current_bidder_id",
		"winning_player_id",
		"provider_reference",
		"provider",
		"escrow_location",
		"source_return_location",
		"world_id",
		"zone_id",
		"account_id",
		"session_id",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}

func assertNoPhase09Leak(t *testing.T, label string, payload json.RawMessage) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range []string{
		"account_id",
		"player_id",
		"session_id",
		"password",
		"password_hash",
		"token",
		"cookie",
		"provider_reference",
		"reference_id",
		"generated_payload",
		"generated_seed",
		"reward_payload",
		"rare_cap",
		"world_seed",
		"gameplay_seed",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}

func readResponse(t *testing.T, conn *websocket.Conn) realtime.ResponseEnvelope {
	t.Helper()
	data := readRawText(t, conn)
	var response realtime.ResponseEnvelope
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode response %s: %v", data, err)
	}
	return response
}

func readError(t *testing.T, conn *websocket.Conn) realtime.ErrorEnvelope {
	t.Helper()
	data := readRawText(t, conn)
	var response realtime.ErrorEnvelope
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode error %s: %v", data, err)
	}
	if response.OK {
		t.Fatalf("error response %s had ok=true", data)
	}
	return response
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal(%T) error = %v", value, err)
	}
	return data
}
