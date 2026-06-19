package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
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

func testCargoUsed(gameServer *Server, playerID foundation.PlayerID) int64 {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	return gameServer.runtime.players[playerID].Cargo.Used
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
