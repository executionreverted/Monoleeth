package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	"gameproject/internal/game/world/worker"
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
	if snapshot.Minimap.RadarRange != runtimeLiveProjectionHalfExtent || snapshot.Minimap.ProjectionWindowSize != runtimeLiveProjectionDiameter {
		t.Fatalf("minimap projection = range %v window %v, want %v/%v", snapshot.Minimap.RadarRange, snapshot.Minimap.ProjectionWindowSize, runtimeLiveProjectionHalfExtent, runtimeLiveProjectionDiameter)
	}
	if len(snapshot.Minimap.LiveContacts) != len(snapshot.Entities) {
		t.Fatalf("minimap contacts = %d, entities = %d", len(snapshot.Minimap.LiveContacts), len(snapshot.Entities))
	}
	entitiesByID := make(map[string]aoi.EntityPayload, len(snapshot.Entities))
	selfCount := 0
	npcCombatCount := 0
	for _, entity := range snapshot.Entities {
		entitiesByID[entity.ID.String()] = entity
		if entity.ProjectionSource != runtimeProjectionSourceWorker {
			t.Fatalf("entity %s projection source = %q, want %q", entity.ID, entity.ProjectionSource, runtimeProjectionSourceWorker)
		}
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
		if entity.Type == "npc" {
			npcCombatCount++
			if entity.Combat == nil || entity.Combat.HP <= 0 || entity.Combat.MaxHP <= 0 || entity.Combat.Status == "" {
				t.Fatalf("npc entity = %+v, want public combat status in initial snapshot", entity)
			}
		}
	}
	if selfCount != 1 {
		t.Fatalf("self entity count = %d, want 1", selfCount)
	}
	if npcCombatCount == 0 {
		t.Fatalf("world snapshot missing visible npc for combat contract test")
	}
	for _, contact := range snapshot.Minimap.LiveContacts {
		if contact.EntityID == "" || contact.EntityType == "" {
			t.Fatalf("minimap contact missing stable identity: %+v", contact)
		}
		if contact.EntityID == "entity_hidden_planet_signal" {
			t.Fatalf("hidden entity leaked into minimap contact %+v", contact)
		}
		entity, ok := entitiesByID[contact.EntityID]
		if !ok {
			t.Fatalf("minimap contact %+v missing matching snapshot entity", contact)
		}
		if contact.EntityType != entity.Type || contact.Position != entity.Position {
			t.Fatalf("minimap contact %+v does not mirror snapshot entity %+v", contact, entity)
		}
		if contact.ProjectionSource != entity.ProjectionSource {
			t.Fatalf("minimap contact %+v projection source does not mirror snapshot entity %+v", contact, entity)
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
		Accepted bool                `json:"accepted"`
		Entities []aoi.EntityPayload `json:"entities"`
		Minimap  minimapPayload      `json:"minimap"`
	}
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode move payload: %v", err)
	}
	if !payload.Accepted {
		t.Fatal("move accepted = false, want true")
	}
	assertMinimapMirrorsEntities(t, "move response", payload.Entities, payload.Minimap)
	var playerX float64
	var playerMovement *aoi.EntityMovementStatus
	for _, entity := range payload.Entities {
		if entity.Type == world.EntityTypePlayer {
			playerX = entity.Position.X
			playerMovement = entity.Movement
		}
	}
	if playerX <= 0 || playerX >= 100 {
		t.Fatalf("player x after one server tick = %v, want server-derived movement between 0 and target", playerX)
	}
	if playerMovement == nil {
		t.Fatal("player movement = nil, want server-timed movement route")
	}
	if playerMovement.Origin.X != 0 || playerMovement.Target.X != 100 || playerMovement.Speed != defaultPlayerSpeed {
		t.Fatalf("player movement = %+v, want origin 0 target 100 speed %d", playerMovement, defaultPlayerSpeed)
	}
	if playerMovement.ArriveAtMS <= playerMovement.StartedAtMS {
		t.Fatalf("player movement timing = %+v, want arrival after start", playerMovement)
	}

	event := readEvent(t, conn)
	if event.Type != realtime.EventPositionCorrected || event.Sequence == 0 {
		t.Fatalf("post-move event = %+v, want position.corrected with seq", event)
	}
	var correction struct {
		EntityID string                  `json:"entity_id"`
		Position world.Vec2              `json:"position"`
		Movement *movementPayloadForTest `json:"movement"`
	}
	if err := json.Unmarshal(event.Payload, &correction); err != nil {
		t.Fatalf("decode correction payload: %v", err)
	}
	if correction.Movement == nil || correction.Movement.Target.X != 100 {
		t.Fatalf("correction movement = %+v, want target 100", correction.Movement)
	}
	update := readEvent(t, conn)
	if update.Type != realtime.EventAOIEntityUpdated {
		t.Fatalf("post-move AOI event = %+v, want entity updated", update)
	}
	var aoiUpdate struct {
		EntityID string                  `json:"entity_id"`
		Type     string                  `json:"entity_type"`
		Movement *movementPayloadForTest `json:"movement"`
	}
	if err := json.Unmarshal(update.Payload, &aoiUpdate); err != nil {
		t.Fatalf("decode AOI update payload: %v", err)
	}
	if aoiUpdate.Type == "player" && aoiUpdate.Movement == nil {
		t.Fatalf("AOI update movement = nil, want movement timing")
	}

	time.Sleep(minMoveCommandInterval)
	writeText(t, conn, `{"request_id":"request-move-2","op":"move_to","payload":{"target":{"x":0,"y":100}},"client_seq":2,"v":1}`)
	response = readResponse(t, conn)
	if !response.OK {
		t.Fatalf("second move response = %+v, want success", response)
	}
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode second move payload: %v", err)
	}
	assertMinimapMirrorsEntities(t, "second move response", payload.Entities, payload.Minimap)
	var secondMovement *aoi.EntityMovementStatus
	for _, entity := range payload.Entities {
		if entity.Type == world.EntityTypePlayer {
			secondMovement = entity.Movement
		}
	}
	if secondMovement == nil {
		t.Fatal("second movement = nil, want server-timed movement route")
	}
	if secondMovement.Origin.X < playerX || secondMovement.Origin.X >= 100 {
		t.Fatalf("second movement origin x = %v, want between first server position %v and first target 100", secondMovement.Origin.X, playerX)
	}
	if secondMovement.Target.X != 0 || secondMovement.Target.Y != 100 {
		t.Fatalf("second movement target = %+v, want 0,100", secondMovement.Target)
	}
}

func TestMoveToRateLimitsSpamWithoutChangingAuthoritativeRoute(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)

	writeText(t, conn, `{"request_id":"request-move-rate-1","op":"move_to","payload":{"target":{"x":100,"y":0}},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("first move response = %+v, want success", response)
	}
	drainEventTypes(t, conn, realtime.EventPositionCorrected, realtime.EventAOIEntityUpdated)

	gameServer.runtime.mu.Lock()
	before, ok := gameServer.runtime.Worker.PlayerEntity(resolved.PlayerID)
	gameServer.runtime.mu.Unlock()
	if !ok || !before.Movement.Moving || before.Movement.Target != (world.Vec2{X: 100, Y: 0}) {
		t.Fatalf("authoritative movement before spam = %+v, ok=%v; want target 100,0", before.Movement, ok)
	}

	writeText(t, conn, `{"request_id":"request-move-rate-2","op":"move_to","payload":{"target":{"x":0,"y":100}},"client_seq":2,"v":1}`)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeRateLimited {
		t.Fatalf("spam move error = %+v, want %s", got.Error, foundation.CodeRateLimited)
	}

	gameServer.runtime.mu.Lock()
	after, ok := gameServer.runtime.Worker.PlayerEntity(resolved.PlayerID)
	gameServer.runtime.mu.Unlock()
	if !ok || !after.Movement.Moving || after.Movement.Target != before.Movement.Target {
		t.Fatalf("authoritative movement after spam = %+v, ok=%v; want unchanged target %+v", after.Movement, ok, before.Movement.Target)
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
		disabled := readEvent(t, conn)
		if disabled.Type != realtime.EventDeathShipDisabled {
			t.Fatalf("disabled move event = %s, want %s", disabled.Type, realtime.EventDeathShipDisabled)
		}
		var payload struct {
			ShipID         string              `json:"ship_id"`
			DisabledReason string              `json:"disabled_reason"`
			Ship           shipSnapshotPayload `json:"ship"`
			RepairQuote    repairQuotePayload  `json:"repair_quote"`
		}
		if err := json.Unmarshal(disabled.Payload, &payload); err != nil {
			t.Fatalf("decode disabled event: %v", err)
		}
		if payload.ShipID != starterShipID.String() || payload.DisabledReason != "death" || !payload.Ship.Disabled || !payload.RepairQuote.Disabled {
			t.Fatalf("disabled payload = %+v, want disabled starter ship with repair quote", payload)
		}
		drainEventTypes(t, conn, realtime.EventShipSnapshot, realtime.EventPlayerSnapshot)
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
	response := readResponseSkippingEvents(t, conn)
	if !response.OK {
		t.Fatalf("stop response = %+v, want success", response)
	}
	var stopPayload struct {
		Accepted bool                `json:"accepted"`
		Entities []aoi.EntityPayload `json:"entities"`
		Minimap  minimapPayload      `json:"minimap"`
	}
	if err := json.Unmarshal(response.Payload, &stopPayload); err != nil {
		t.Fatalf("decode stop payload: %v", err)
	}
	if !stopPayload.Accepted {
		t.Fatalf("stop accepted = false, want true")
	}
	assertMinimapMirrorsEntities(t, "stop response", stopPayload.Entities, stopPayload.Minimap)
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
				ItemID       string `json:"item_id"`
				DisplayName  string `json:"display_name"`
				Category     string `json:"category"`
				ArtKey       string `json:"art_key"`
				Quantity     int64  `json:"quantity"`
				UnitWeight   int64  `json:"unit_weight"`
				UsedUnits    int64  `json:"used_units"`
				Location     string `json:"location"`
				MoveEligible bool   `json:"move_eligible"`
				LockedReason string `json:"locked_reason"`
			} `json:"items"`
		} `json:"cargo"`
	}
	if err := json.Unmarshal(pickup.Payload, &pickupPayload); err != nil {
		t.Fatalf("decode pickup response: %v", err)
	}
	if !pickupPayload.Accepted || pickupPayload.Cargo.Used != 6 || len(pickupPayload.Cargo.Items) != 1 || pickupPayload.Cargo.Items[0].Quantity != 3 {
		t.Fatalf("pickup payload = %+v, want cargo with three raw ore", pickupPayload)
	}
	rawOreCargo := pickupPayload.Cargo.Items[0]
	if rawOreCargo.ItemID != "raw_ore" ||
		rawOreCargo.DisplayName != "Raw Ore" ||
		rawOreCargo.Category != "resource" ||
		rawOreCargo.ArtKey != "item.raw_ore" ||
		rawOreCargo.UnitWeight != 2 ||
		rawOreCargo.UsedUnits != 6 ||
		rawOreCargo.Location != economy.LocationKindShipCargo.String() ||
		rawOreCargo.MoveEligible ||
		rawOreCargo.LockedReason != "cargo_transfer_unavailable" {
		t.Fatalf("cargo metadata = %+v, want server-owned raw ore metadata and move locked", rawOreCargo)
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
				if len(payload.Inventory.Stackable) != 0 ||
					len(payload.Inventory.Instances) != 3 ||
					payload.Inventory.Counts.EquippedInstances != 1 ||
					payload.Cargo.Capacity != 60 {
					t.Fatalf("inventory payload = %+v cargo=%+v, want starter modules and cargo capacity", payload.Inventory, payload.Cargo)
				}
			case "hangar":
				var payload struct {
					Hangar hangarSnapshotPayload `json:"hangar"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode hangar snapshot: %v", err)
				}
				if payload.Hangar.ActiveShipID != starterShipID.String() || len(payload.Hangar.Ships) != 1 {
					t.Fatalf("hangar payload = %+v, want active starter ship", payload.Hangar)
				}
			case "loadout":
				var payload struct {
					Loadout loadoutSnapshotPayload `json:"loadout"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode loadout snapshot: %v", err)
				}
				if payload.Loadout.ActiveShipID != starterShipID.String() || len(payload.Loadout.Slots) != 3 {
					t.Fatalf("loadout payload = %+v, want starter slot snapshot", payload.Loadout)
				}
			case "stats":
				var payload struct {
					Stats statSnapshotPayload `json:"stats"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode stats snapshot: %v", err)
				}
				if payload.Stats.RadarRange != defaultRadarRange ||
					payload.Stats.CargoCapacity != 60 ||
					payload.Stats.LootPickupRange != runtimeLootPickupRange ||
					payload.Stats.BasicLaserEnergyCost != runtimeBasicLaserEnergyCost ||
					payload.Stats.BasicLaserCooldownMS != runtimeBasicLaserCooldownMS {
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

func TestLoadoutEquipAndUnequipMutateServerOwnedInventory(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-loadout-inventory","op":"inventory.snapshot","payload":{},"client_seq":1,"v":1}`)
	inventoryResponse := readResponse(t, conn)
	if !inventoryResponse.OK {
		t.Fatalf("inventory response = %+v, want success", inventoryResponse)
	}
	var inventoryPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
	}
	if err := json.Unmarshal(inventoryResponse.Payload, &inventoryPayload); err != nil {
		t.Fatalf("decode inventory snapshot: %v", err)
	}
	laserID := requireInventoryInstance(t, inventoryPayload.Inventory, "laser_alpha_t1", economy.LocationKindAccountInventory.String())
	shieldID := requireInventoryInstance(t, inventoryPayload.Inventory, "shield_generator_t1", economy.LocationKindAccountInventory.String())

	equipRequest := `{"request_id":"request-loadout-equip-laser","op":"loadout.equip_module","payload":{"slot_id":"offensive_1","item_instance_id":"` + laserID + `"},"client_seq":2,"v":1}`
	writeText(t, conn, equipRequest)
	equipRaw := readRawText(t, conn)
	equipResponse := decodeRawResponse(t, equipRaw)
	if !equipResponse.OK {
		t.Fatalf("equip response = %+v, want success", equipResponse)
	}
	var equipPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
		Loadout   loadoutSnapshotPayload   `json:"loadout"`
	}
	if err := json.Unmarshal(equipResponse.Payload, &equipPayload); err != nil {
		t.Fatalf("decode equip payload: %v", err)
	}
	offensive := requireLoadoutSlot(t, equipPayload.Loadout, "offensive_1")
	if offensive.ItemInstanceID != laserID || offensive.ModuleItemID != "laser_alpha_t1" || offensive.Durability != 100 {
		t.Fatalf("offensive slot = %+v, want equipped laser %s", offensive, laserID)
	}
	requireInventoryInstanceLocation(t, equipPayload.Inventory, laserID, economy.LocationKindShipEquipped.String())
	drainEventTypes(t, conn, realtime.EventInventorySnapshot, realtime.EventLoadoutSnapshot, realtime.EventStatsUpdated)
	writeText(t, conn, equipRequest)
	duplicateEquipRaw := readRawText(t, conn)
	if !bytes.Equal(equipRaw, duplicateEquipRaw) {
		t.Fatalf("duplicate equip response changed:\nfirst=%s\nsecond=%s", equipRaw, duplicateEquipRaw)
	}

	writeText(t, conn, `{"request_id":"request-loadout-spoof","op":"loadout.equip_module","payload":{"slot_id":"offensive_1","item_instance_id":"`+laserID+`","player_id":"spoof"},"client_seq":3,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed loadout error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, conn, `{"request_id":"request-loadout-wrong-slot","op":"loadout.equip_module","payload":{"slot_id":"offensive_1","item_instance_id":"`+shieldID+`"},"client_seq":4,"v":1}`)
	wrongSlot := readError(t, conn)
	if wrongSlot.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("wrong slot error = %+v, want %s", wrongSlot.Error, foundation.CodeInvalidPayload)
	}

	unequipRequest := `{"request_id":"request-loadout-unequip-laser","op":"loadout.unequip_module","payload":{"slot_id":"offensive_1"},"client_seq":5,"v":1}`
	writeText(t, conn, unequipRequest)
	unequipRaw := readRawText(t, conn)
	unequipResponse := decodeRawResponse(t, unequipRaw)
	if !unequipResponse.OK {
		t.Fatalf("unequip response = %+v, want success", unequipResponse)
	}
	var unequipPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
		Loadout   loadoutSnapshotPayload   `json:"loadout"`
	}
	if err := json.Unmarshal(unequipResponse.Payload, &unequipPayload); err != nil {
		t.Fatalf("decode unequip payload: %v", err)
	}
	offensive = requireLoadoutSlot(t, unequipPayload.Loadout, "offensive_1")
	if offensive.ItemInstanceID != "" || offensive.ModuleItemID != "" {
		t.Fatalf("offensive slot after unequip = %+v, want empty", offensive)
	}
	requireInventoryInstanceLocation(t, unequipPayload.Inventory, laserID, economy.LocationKindAccountInventory.String())
	drainEventTypes(t, conn, realtime.EventInventorySnapshot, realtime.EventLoadoutSnapshot, realtime.EventStatsUpdated)
	writeText(t, conn, unequipRequest)
	duplicateUnequipRaw := readRawText(t, conn)
	if !bytes.Equal(unequipRaw, duplicateUnequipRaw) {
		t.Fatalf("duplicate unequip response changed:\nfirst=%s\nsecond=%s", unequipRaw, duplicateUnequipRaw)
	}
}

func TestHangarActivateShipUsesServerOwnedHangarState(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-hangar-snapshot","op":"hangar.snapshot","payload":{},"client_seq":1,"v":1}`)
	snapshotResponse := readResponse(t, conn)
	if !snapshotResponse.OK {
		t.Fatalf("hangar snapshot response = %+v, want success", snapshotResponse)
	}
	var snapshotPayload struct {
		Hangar hangarSnapshotPayload `json:"hangar"`
	}
	if err := json.Unmarshal(snapshotResponse.Payload, &snapshotPayload); err != nil {
		t.Fatalf("decode hangar snapshot: %v", err)
	}
	if snapshotPayload.Hangar.ActiveShipID != starterShipID.String() || len(snapshotPayload.Hangar.Ships) != 1 {
		t.Fatalf("hangar snapshot = %+v, want active starter row", snapshotPayload.Hangar)
	}
	starter := snapshotPayload.Hangar.Ships[0]
	if starter.ShipID != starterShipID.String() || !starter.Active || starter.CargoCapacity <= 0 || starter.SlotUtility != 1 {
		t.Fatalf("starter hangar row = %+v, want server catalog stats and active flag", starter)
	}

	activateRequest := `{"request_id":"request-hangar-activate-starter","op":"hangar.activate_ship","payload":{"ship_id":"` + starterShipID.String() + `"},"client_seq":2,"v":1}`
	writeText(t, conn, activateRequest)
	activateRaw := readRawText(t, conn)
	activateResponse := decodeRawResponse(t, activateRaw)
	if !activateResponse.OK {
		t.Fatalf("hangar activate response = %+v, want success", activateResponse)
	}
	var activatePayload struct {
		Hangar  hangarSnapshotPayload  `json:"hangar"`
		Ship    shipSnapshotPayload    `json:"ship"`
		Stats   statSnapshotPayload    `json:"stats"`
		Cargo   cargoSnapshotPayload   `json:"cargo"`
		Loadout loadoutSnapshotPayload `json:"loadout"`
	}
	if err := json.Unmarshal(activateResponse.Payload, &activatePayload); err != nil {
		t.Fatalf("decode hangar activate payload: %v", err)
	}
	if activatePayload.Hangar.ActiveShipID != starterShipID.String() ||
		activatePayload.Ship.ActiveShipID != starterShipID.String() ||
		activatePayload.Loadout.ActiveShipID != starterShipID.String() {
		t.Fatalf("activate payload = %+v, want starter snapshots", activatePayload)
	}
	drainEventTypes(t, conn, realtime.EventHangarSnapshot, realtime.EventShipSnapshot, realtime.EventStatsUpdated, realtime.EventCargoSnapshot, realtime.EventLoadoutSnapshot)
	writeText(t, conn, activateRequest)
	duplicateActivateRaw := readRawText(t, conn)
	if !bytes.Equal(activateRaw, duplicateActivateRaw) {
		t.Fatalf("duplicate hangar activate response changed:\nfirst=%s\nsecond=%s", activateRaw, duplicateActivateRaw)
	}

	writeText(t, conn, `{"request_id":"request-hangar-spoof","op":"hangar.activate_ship","payload":{"ship_id":"`+starterShipID.String()+`","player_id":"spoof"},"client_seq":3,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed hangar error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, conn, `{"request_id":"request-hangar-unknown","op":"hangar.activate_ship","payload":{"ship_id":"missing_ship"},"client_seq":4,"v":1}`)
	unknown := readError(t, conn)
	if unknown.Error.Code != foundation.CodeNotFound {
		t.Fatalf("unknown hangar error = %+v, want %s", unknown.Error, foundation.CodeNotFound)
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
	var knownEventPayload struct {
		Planets []knownPlanetPayload `json:"planets"`
		Counts  planetIntelCounts    `json:"counts"`
		Minimap minimapPayload       `json:"minimap"`
	}
	for attempts := 0; attempts < 6 && (!seen[realtime.EventScanPulseStarted] || !seen[realtime.EventScanPulseResolved] || !seen[realtime.EventScanPlanetDiscovered] || !seen[realtime.EventKnownPlanets]); attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
		if raw := string(mustJSON(t, event)); strings.Contains(raw, "candidate_key") || strings.Contains(raw, "procedural_seed") || strings.Contains(raw, "detection_roll") {
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
	if len(knownPayload.Minimap.Remembered) != 1 || knownPayload.Minimap.Remembered[0].PlanetID != planetID {
		t.Fatalf("known planets remembered minimap = %+v, want discovered planet %s", knownPayload.Minimap.Remembered, planetID)
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
	if !marketPayload.Market.Listings[0].FinalPricePending {
		t.Fatalf("market listing = %+v, want final price pending marker", marketPayload.Market.Listings[0])
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

	writeText(t, conn, `{"request_id":"request-auction-grants","op":"auction.grants","payload":{},"client_seq":8,"v":1}`)
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

	writeText(t, conn, `{"request_id":"request-weekly-xcore-empty","op":"premium.purchase_weekly_xcore","payload":{},"client_seq":11,"v":1}`)
	emptyStockIntent := readError(t, conn)
	if emptyStockIntent.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("empty weekly xcore intent error = %+v, want %s", emptyStockIntent.Error, foundation.CodeInvalidPayload)
	}

	premiumPeriod := gameServer.runtime.currentPremiumPeriodKey()
	writeText(t, conn, `{"request_id":"request-weekly-xcore","op":"premium.purchase_weekly_xcore","payload":{"product_id":"weekly_xcore","period_key":"`+premiumPeriod+`"},"client_seq":12,"v":1}`)
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

	writeText(t, conn, `{"request_id":"request-weekly-xcore-again","op":"premium.purchase_weekly_xcore","payload":{"product_id":"weekly_xcore","period_key":"`+premiumPeriod+`"},"client_seq":13,"v":1}`)
	limit := readError(t, conn)
	if limit.Error.Code != foundation.CodeForbidden {
		t.Fatalf("second weekly xcore error = %+v, want %s", limit.Error, foundation.CodeForbidden)
	}

	writeText(t, conn, `{"request_id":"request-admin-economy","op":"admin.economy_dashboard","payload":{},"client_seq":14,"v":1}`)
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

func TestMarketCreateListingDuplicateRequestIDReturnsCachedResponse(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-market-create-inventory","op":"inventory.snapshot","payload":{},"client_seq":1,"v":1}`)
	inventoryResponse := readResponse(t, conn)
	if !inventoryResponse.OK {
		t.Fatalf("inventory response = %+v, want success", inventoryResponse)
	}
	var inventoryPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
	}
	if err := json.Unmarshal(inventoryResponse.Payload, &inventoryPayload); err != nil {
		t.Fatalf("decode inventory snapshot: %v", err)
	}
	laserID := requireInventoryInstance(t, inventoryPayload.Inventory, "laser_alpha_t1", economy.LocationKindAccountInventory.String())
	beforeListings := len(gameServer.runtime.Market.Listings())
	beforeLedger := len(gameServer.runtime.Inventory.ItemLedgerEntries())

	request := `{"request_id":"request-market-create-listing-dup","op":"market.create_listing","payload":{"item_id":"laser_alpha_t1","item_instance_id":"` + laserID + `","quantity":1,"unit_price":75},"client_seq":2,"v":1}`
	writeText(t, conn, request)
	firstRaw := readRawText(t, conn)
	first := decodeRawResponse(t, firstRaw)
	if !first.OK {
		t.Fatalf("market create response = %+v, want success", first)
	}
	var firstPayload marketMutationPayload
	if err := json.Unmarshal(first.Payload, &firstPayload); err != nil {
		t.Fatalf("decode market create: %v", err)
	}
	if !firstPayload.Accepted || firstPayload.Listing.ListingID != "listing-request-market-create-listing-dup" {
		t.Fatalf("market create payload = %+v, want accepted listing from request id", firstPayload)
	}
	if got := len(gameServer.runtime.Market.Listings()); got != beforeListings+1 {
		t.Fatalf("listings after create = %d, want %d", got, beforeListings+1)
	}
	if got := len(gameServer.runtime.Inventory.ItemLedgerEntries()); got != beforeLedger+2 {
		t.Fatalf("item ledger entries after create = %d, want %d", got, beforeLedger+2)
	}
	drainEventTypes(t, conn, realtime.EventMarketListingCreated, realtime.EventInventorySnapshot)

	writeText(t, conn, request)
	secondRaw := readRawText(t, conn)
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatalf("duplicate market create response changed:\nfirst=%s\nsecond=%s", firstRaw, secondRaw)
	}
	second := decodeRawResponse(t, secondRaw)
	if !second.OK {
		t.Fatalf("duplicate market create response = %+v, want cached success", second)
	}
	if got := len(gameServer.runtime.Market.Listings()); got != beforeListings+1 {
		t.Fatalf("listings after duplicate = %d, want %d", got, beforeListings+1)
	}
	if got := len(gameServer.runtime.Inventory.ItemLedgerEntries()); got != beforeLedger+2 {
		t.Fatalf("item ledger entries after duplicate = %d, want %d", got, beforeLedger+2)
	}
}

func TestShopCatalogUsesServerOwnedGameCatalog(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-shop-catalog","op":"shop.catalog","payload":{},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("shop catalog response = %+v, want success", response)
	}
	assertNoEconomyLeak(t, "shop catalog", response.Payload)
	rawPayload := string(response.Payload)
	for _, forbidden := range []string{"listing-raw-ore-1", "server_recalculates", "server recalculates", "server-owned"} {
		if strings.Contains(rawPayload, forbidden) {
			t.Fatalf("shop catalog leaked %q in %s", forbidden, rawPayload)
		}
	}

	var payload shopCatalogResponsePayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode shop catalog: %v", err)
	}
	if payload.Shop.CatalogVersion != catalog.ContentRegistryVersion.String() {
		t.Fatalf("catalog version = %q, want %q", payload.Shop.CatalogVersion, catalog.ContentRegistryVersion)
	}
	categoryNames := make(map[string]string, len(payload.Shop.Categories))
	for _, category := range payload.Shop.Categories {
		categoryNames[category.CategoryID] = category.DisplayName
	}
	for id, displayName := range map[string]string{
		shopCategoryShips:            "Ships",
		shopCategoryWeapons:          "Weapons",
		shopCategoryShieldGenerators: "Shield Generators",
		shopCategoryScannerRadar:     "Scanner/Radar",
		shopCategoryCargoUtility:     "Cargo/Utility",
		shopCategoryResources:        "Resources",
	} {
		if categoryNames[id] != displayName {
			t.Fatalf("category %q = %q, want %q in %+v", id, categoryNames[id], displayName, payload.Shop.Categories)
		}
	}
	if len(payload.Shop.Products) < 6 {
		t.Fatalf("shop products = %+v, want broad seed coverage", payload.Shop.Products)
	}
	seenPrism := false
	for _, product := range payload.Shop.Products {
		if product.DisplayName == "" || product.DisplayName == product.ProductID || strings.Contains(product.DisplayName, "_") {
			t.Fatalf("shop product has raw display name: %+v", product)
		}
		if product.CategoryID == "" || product.ArtKey == "" || product.Price.Currency == "" {
			t.Fatalf("shop product missing display/category/price metadata: %+v", product)
		}
		if product.DisplayName == "Prism Lance I" {
			seenPrism = true
		}
	}
	if !seenPrism {
		t.Fatalf("shop products = %+v, missing Prism Lance I weapon product", payload.Shop.Products)
	}

	writeText(t, conn, `{"request_id":"request-shop-catalog-spoof","op":"shop.catalog","payload":{"stock_remaining":99},"client_seq":2,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed shop catalog error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}
}

func TestShopBuyProductDebitsWalletAndGrantsServerCatalogProduct(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	beforeCount := countInventoryInstances(gameServer.runtime.Inventory.InstanceItems(), "laser_alpha_t1")
	writeText(t, conn, `{"request_id":"request-shop-buy-laser","op":"shop.buy_product","payload":{"product_id":"product_module_laser_alpha_t1","quantity":1},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("shop buy response = %+v, want success", response)
	}
	assertNoEconomyLeak(t, "shop buy", response.Payload)
	var payload shopBuyProductResponsePayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode shop buy: %v", err)
	}
	if !payload.Accepted || payload.Product.ProductID != "product_module_laser_alpha_t1" || payload.Quantity != 1 || payload.ServerTotal != 450 {
		t.Fatalf("shop buy payload = %+v, want accepted laser at server price", payload)
	}
	if payload.Wallet.Credits != starterWalletCredits-450 {
		t.Fatalf("wallet credits = %d, want %d", payload.Wallet.Credits, starterWalletCredits-450)
	}
	if payload.Inventory == nil {
		t.Fatalf("shop buy inventory snapshot missing")
	}
	afterCount := countInventoryInstances(gameServer.runtime.Inventory.InstanceItems(), "laser_alpha_t1")
	if afterCount != beforeCount+1 {
		t.Fatalf("laser instances = %d, want %d after shop buy", afterCount, beforeCount+1)
	}
	if !inventorySnapshotHasInstance(*payload.Inventory, "laser_alpha_t1") {
		t.Fatalf("inventory snapshot missing laser_alpha_t1 instance: %+v", payload.Inventory.Instances)
	}

	writeText(t, conn, `{"request_id":"request-shop-buy-spoof","op":"shop.buy_product","payload":{"product_id":"product_module_laser_alpha_t1","quantity":1,"price":{"amount":1},"stock_remaining":99},"client_seq":2,"v":1}`)
	spoof := readErrorSkippingEvents(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed shop buy error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}
}

func TestShopBuyProductRejectsInsufficientFundsBeforeGrant(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	for index := 1; index <= 2; index++ {
		writeText(t, conn, fmt.Sprintf(`{"request_id":"request-shop-buy-laser-%d","op":"shop.buy_product","payload":{"product_id":"product_module_laser_alpha_t1","quantity":1},"client_seq":%d,"v":1}`, index, index))
		response := readResponseSkippingEvents(t, conn)
		if !response.OK {
			t.Fatalf("shop buy %d response = %+v, want success", index, response)
		}
	}
	beforeCount := countInventoryInstances(gameServer.runtime.Inventory.InstanceItems(), "laser_alpha_t1")
	writeText(t, conn, `{"request_id":"request-shop-buy-laser-no-funds","op":"shop.buy_product","payload":{"product_id":"product_module_laser_alpha_t1","quantity":1},"client_seq":3,"v":1}`)
	rejected := readErrorSkippingEvents(t, conn)
	if rejected.Error.Code != foundation.CodeNotEnoughFunds {
		t.Fatalf("insufficient funds error = %+v, want %s", rejected.Error, foundation.CodeNotEnoughFunds)
	}
	afterCount := countInventoryInstances(gameServer.runtime.Inventory.InstanceItems(), "laser_alpha_t1")
	if afterCount != beforeCount {
		t.Fatalf("laser instances after failed buy = %d, want unchanged %d", afterCount, beforeCount)
	}
	if credits := runtimeWalletCredits(t, gameServer.runtime); credits != 300 {
		t.Fatalf("wallet credits after failed buy = %d, want 300", credits)
	}
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
	if !boardPayload.QuestBoard.CanReroll || boardPayload.QuestBoard.ResetAt <= boardPayload.QuestBoard.GeneratedAt || boardPayload.QuestBoard.Revision <= 0 || !boardPayload.QuestBoard.Offers[0].CanAccept {
		t.Fatalf("quest board action state = %+v first offer %+v, want server-owned reroll/accept/reset state", boardPayload.QuestBoard, boardPayload.QuestBoard.Offers[0])
	}
	for _, offer := range boardPayload.QuestBoard.Offers {
		for _, objective := range offer.Objectives {
			if objective.DisplayName == "" || objective.DisplayName == objective.Target {
				t.Fatalf("quest objective display metadata = %+v, want safe display name separate from raw target", objective)
			}
		}
		for _, reward := range offer.Rewards {
			if reward.DisplayName == "" || reward.DisplayName == reward.ItemID || reward.DisplayName == reward.Currency || reward.DisplayName == reward.Role {
				t.Fatalf("quest reward display metadata = %+v, want safe display name separate from raw ids", reward)
			}
		}
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
	if accepted.Quest.AcceptedOfferID != offer.OfferID || accepted.QuestBoard.Counts.Offers != quests.BoardOfferCount-1 {
		t.Fatalf("accepted quest offer reconciliation = quest %+v board counts %+v, want accepted offer removed", accepted.Quest, accepted.QuestBoard.Counts)
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
		{"admin inspect target", fmt.Sprintf(`{"request_id":"request-non-admin-inspect-target","op":"admin.inspect_player","payload":{"target_player_id":"%s"},"client_seq":9,"v":1}`, resolved.PlayerID.String())},
		{"admin repair", `{"request_id":"request-non-admin-repair","op":"admin.repair_craft_job","payload":{"job_id":"job-missing"},"client_seq":10,"v":1}`},
		{"command log", `{"request_id":"request-non-admin-log","op":"observability.command_log","payload":{},"client_seq":11,"v":1}`},
		{"metrics", `{"request_id":"request-non-admin-metrics","op":"observability.metrics","payload":{},"client_seq":12,"v":1}`},
		{"release gate", `{"request_id":"request-non-admin-gate","op":"observability.release_gate","payload":{},"client_seq":13,"v":1}`},
		{"abuse coverage", `{"request_id":"request-non-admin-abuse","op":"observability.abuse_coverage","payload":{},"client_seq":14,"v":1}`},
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
			name: "inspect target",
			body: fmt.Sprintf(`{"request_id":"request-admin-inspect-target","op":"admin.inspect_player","payload":{"target_player_id":"%s"},"client_seq":2,"v":1}`, resolved.PlayerID.String()),
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Admin adminPlayerInspectionPayload `json:"admin"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode admin inspect target: %v", err)
				}
				if payload.Admin.Target != "requested" || len(payload.Admin.Wallet.Balances) == 0 {
					t.Fatalf("admin inspect target = %+v, want requested wallet balances", payload.Admin)
				}
			},
		},
		{
			name: "economy",
			body: `{"request_id":"request-admin-economy-ok","op":"admin.economy_dashboard","payload":{},"client_seq":3,"v":1}`,
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
			body: `{"request_id":"request-admin-repair","op":"admin.repair_craft_job","payload":{"job_id":"job-missing"},"client_seq":4,"v":1}`,
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
			body: `{"request_id":"request-admin-command-log","op":"observability.command_log","payload":{},"client_seq":5,"v":1}`,
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
			body: `{"request_id":"request-admin-metrics","op":"observability.metrics","payload":{},"client_seq":6,"v":1}`,
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
			body: `{"request_id":"request-admin-release-gate","op":"observability.release_gate","payload":{},"client_seq":7,"v":1}`,
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
			body: `{"request_id":"request-admin-abuse","op":"observability.abuse_coverage","payload":{},"client_seq":8,"v":1}`,
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

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	_ = decodeWorldSnapshotForTest(t, events)
	insertTestWorldEntity(t, gameServer, "entity_projection_hidden", world.EntityTypeNPC, world.Vec2{X: 880, Y: 0}, true)
	insertTestWorldEntity(t, gameServer, "entity_projection_visible", world.EntityTypeNPC, world.Vec2{X: 900, Y: 0}, false)

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	sessionEvents := eventsBySession[resolved.SessionID]
	if len(sessionEvents) == 0 || sessionEvents[0].Type != realtime.EventAOIEntityEntered {
		t.Fatalf("AOI events after visible entity insert = %+v, want entity entered", sessionEvents)
	}
	var entered aoiEntityPayloadForTest
	if err := json.Unmarshal(sessionEvents[0].Payload, &entered); err != nil {
		t.Fatalf("decode entered event: %v", err)
	}
	if entered.EntityID != "entity_projection_visible" || entered.EntityType != "npc" {
		t.Fatalf("entered entity = %+v, want visible projection npc", entered)
	}
	for _, event := range sessionEvents {
		if strings.Contains(string(event.Payload), "entity_projection_hidden") {
			t.Fatalf("hidden entity leaked into AOI event %+v", event)
		}
	}
}

func TestHiddenPlayerWitnessVisibilityIsViewerSpecificAndExpires(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		SessionTTL:     time.Hour,
		TickDelta:      50 * time.Millisecond,
		Clock:          clock,
		PasswordHasher: auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	target := createResolvedRuntimeSession(t, gameServer, "hidden-target@example.com", "Hidden")
	viewer := createResolvedRuntimeSession(t, gameServer, "scanner-viewer@example.com", "Scanner")
	other := createResolvedRuntimeSession(t, gameServer, "other-viewer@example.com", "Other")
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	setTestHiddenPlayer(gameServer, target.PlayerID, true)

	targetEvents, err := gameServer.runtime.bootstrapEvents(target)
	if err != nil {
		t.Fatalf("target bootstrap events: %v", err)
	}
	targetSnapshot := decodeWorldSnapshotForTest(t, targetEvents)
	if !hasEntityID(targetSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("hidden target self snapshot missing self entity %s: %+v", targetEntityID, targetSnapshot.Entities)
	}

	viewerEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("viewer bootstrap events: %v", err)
	}
	viewerSnapshot := decodeWorldSnapshotForTest(t, viewerEvents)
	if hasEntityID(viewerSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw hidden target without witness: %+v", viewerSnapshot.Entities)
	}

	otherEvents, err := gameServer.runtime.bootstrapEvents(other)
	if err != nil {
		t.Fatalf("other bootstrap events: %v", err)
	}
	otherSnapshot := decodeWorldSnapshotForTest(t, otherEvents)
	if hasEntityID(otherSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("other viewer saw hidden target without witness: %+v", otherSnapshot.Entities)
	}

	setTestHiddenPlayerWitness(gameServer, viewer.PlayerID, target.PlayerID, clock.Now().Add(runtimeHiddenPlayerWitnessDuration))
	witnessedEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("witnessed bootstrap events: %v", err)
	}
	witnessedSnapshot := decodeWorldSnapshotForTest(t, witnessedEvents)
	witnessedTarget, ok := entityPayloadByID(witnessedSnapshot.Entities, targetEntityID.String())
	if !ok {
		t.Fatalf("witnessed viewer snapshot missing hidden target %s: %+v", targetEntityID, witnessedSnapshot.Entities)
	}
	if !hasStatusFlag(witnessedTarget.StatusFlags, "scan_revealed") {
		t.Fatalf("witnessed target flags = %+v, want scan_revealed", witnessedTarget.StatusFlags)
	}
	rawWitnessed := string(mustJSON(t, witnessedSnapshot))
	for _, forbidden := range []string{"hidden", "target_player_id", "witness_expires_at", "player_id", target.PlayerID.String(), viewer.PlayerID.String()} {
		if strings.Contains(rawWitnessed, forbidden) {
			t.Fatalf("witnessed snapshot leaked %q in %s", forbidden, rawWitnessed)
		}
	}

	otherAfterWitnessEvents, err := gameServer.runtime.bootstrapEvents(other)
	if err != nil {
		t.Fatalf("other after witness bootstrap events: %v", err)
	}
	otherAfterWitness := decodeWorldSnapshotForTest(t, otherAfterWitnessEvents)
	if hasEntityID(otherAfterWitness.Entities, targetEntityID.String()) {
		t.Fatalf("unrelated viewer saw witnessed hidden target: %+v", otherAfterWitness.Entities)
	}

	clock.Advance(runtimeHiddenPlayerWitnessDuration)
	expiredEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("expired witness bootstrap events: %v", err)
	}
	expiredSnapshot := decodeWorldSnapshotForTest(t, expiredEvents)
	if hasEntityID(expiredSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw hidden target after witness expiry: %+v", expiredSnapshot.Entities)
	}
}

func TestScanPulseRevealsHiddenPlayerWithoutPlanetIntelOrXP(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		SessionTTL:     time.Hour,
		TickDelta:      50 * time.Millisecond,
		Clock:          clock,
		PasswordHasher: auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	target := createResolvedRuntimeSession(t, gameServer, "hidden-scan-target@example.com", "Hidden Scan")
	viewer := createResolvedRuntimeSession(t, gameServer, "hidden-scan-viewer@example.com", "Scanner Scan")
	other := createResolvedRuntimeSession(t, gameServer, "hidden-scan-other@example.com", "Other Scan")
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	setTestHiddenPlayer(gameServer, target.PlayerID, true)

	viewerEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("viewer bootstrap events: %v", err)
	}
	viewerSnapshot := decodeWorldSnapshotForTest(t, viewerEvents)
	if hasEntityID(viewerSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw hidden target before scan: %+v", viewerSnapshot.Entities)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(viewer.SessionID.String()),
		[]byte(`{"request_id":"request-scan-player-reveal","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("scan hidden player response error = %+v, want success", response.Error)
	}
	rawResponse := string(response.Response.Payload)
	for _, forbidden := range []string{
		"known_planets",
		"progression",
		"planet_id",
		"target_player_id",
		"witness_expires_at",
		"witness_expiry",
		"hidden",
		"detection_roll",
		"scan_candidate",
		"candidate_key",
		"procedural_seed",
		target.PlayerID.String(),
		viewer.PlayerID.String(),
	} {
		if strings.Contains(rawResponse, forbidden) {
			t.Fatalf("scan hidden player response leaked %q in %s", forbidden, rawResponse)
		}
	}
	var payload struct {
		Scan         scanPulsePayload            `json:"scan"`
		KnownPlanets *knownPlanetsPayload        `json:"known_planets,omitempty"`
		Progression  *progressionSnapshotPayload `json:"progression,omitempty"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode scan hidden player payload: %v", err)
	}
	if payload.Scan.Status != string(discovery.ScanPulseStatusPlayerRevealed) {
		t.Fatalf("scan status = %q, want %q", payload.Scan.Status, discovery.ScanPulseStatusPlayerRevealed)
	}
	if payload.Scan.PlanetID != "" || payload.Scan.Signal != nil || payload.Scan.XPGranted || payload.KnownPlanets != nil || payload.Progression != nil {
		t.Fatalf("scan hidden player payload = %+v, want no planet/intel/progression", payload)
	}

	events, err := gameServer.runtime.postCommandEvents(viewer.SessionID, realtime.OperationScanPulse, viewer.PlayerID)
	if err != nil {
		t.Fatalf("post scan events: %v", err)
	}
	seenResolved := false
	seenEntered := false
	for _, event := range events {
		rawEvent := string(mustJSON(t, event))
		for _, forbidden := range []string{"target_player_id", "witness_expires_at", "hidden", "detection_roll", "candidate_key", "procedural_seed", target.PlayerID.String()} {
			if strings.Contains(rawEvent, forbidden) {
				t.Fatalf("scan hidden player event leaked %q in %s", forbidden, rawEvent)
			}
		}
		if event.Type == realtime.EventScanPulseResolved {
			seenResolved = true
			if !strings.Contains(rawEvent, string(discovery.ScanPulseStatusPlayerRevealed)) {
				t.Fatalf("scan resolved event = %s, want player_revealed", rawEvent)
			}
		}
		if event.Type == realtime.EventAOIEntityEntered && strings.Contains(rawEvent, targetEntityID.String()) {
			seenEntered = true
			if !strings.Contains(rawEvent, "scan_revealed") {
				t.Fatalf("aoi entered event = %s, want scan_revealed", rawEvent)
			}
		}
	}
	if !seenResolved || !seenEntered {
		t.Fatalf("post scan events = %+v, want scan resolved and AOI entered hidden target", events)
	}

	otherEvents, err := gameServer.runtime.bootstrapEvents(other)
	if err != nil {
		t.Fatalf("other bootstrap events: %v", err)
	}
	otherSnapshot := decodeWorldSnapshotForTest(t, otherEvents)
	if hasEntityID(otherSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("unrelated viewer saw scan-revealed hidden target: %+v", otherSnapshot.Entities)
	}

	clock.Advance(runtimeHiddenPlayerWitnessDuration)
	expiredEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("expired bootstrap events: %v", err)
	}
	expiredSnapshot := decodeWorldSnapshotForTest(t, expiredEvents)
	if hasEntityID(expiredSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw scan-revealed target after expiry: %+v", expiredSnapshot.Entities)
	}
}

func TestScanPulseDoesNotRevealHiddenPlayerOutsideProjectionWindow(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		SessionTTL:     time.Hour,
		TickDelta:      50 * time.Millisecond,
		Clock:          clock,
		PasswordHasher: auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	target := createResolvedRuntimeSession(t, gameServer, "hidden-scan-far-target@example.com", "Hidden Far")
	viewer := createResolvedRuntimeSession(t, gameServer, "hidden-scan-far-viewer@example.com", "Scanner Far")
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: runtimeLiveProjectionHalfExtent + 250, Y: 0})
	setTestHiddenPlayer(gameServer, target.PlayerID, true)

	viewerEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("viewer bootstrap events: %v", err)
	}
	viewerSnapshot := decodeWorldSnapshotForTest(t, viewerEvents)
	if hasEntityID(viewerSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw out-of-projection hidden target before scan: %+v", viewerSnapshot.Entities)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(viewer.SessionID.String()),
		[]byte(`{"request_id":"request-scan-player-projection-miss","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("scan projection miss response error = %+v, want success", response.Error)
	}
	rawResponse := string(response.Response.Payload)
	for _, forbidden := range []string{targetEntityID.String(), target.PlayerID.String(), "target_player_id", "witness_expires_at", "hidden"} {
		if strings.Contains(rawResponse, forbidden) {
			t.Fatalf("scan projection miss response leaked %q in %s", forbidden, rawResponse)
		}
	}
	var payload struct {
		Scan scanPulsePayload `json:"scan"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode scan projection miss payload: %v", err)
	}
	if payload.Scan.Status != string(discovery.ScanPulseStatusNoSignal) {
		t.Fatalf("scan status = %q, want %q for hidden-player reveal outside projection", payload.Scan.Status, discovery.ScanPulseStatusNoSignal)
	}

	gameServer.runtime.mu.Lock()
	witnessed := gameServer.runtime.hiddenPlayerWitnessActiveLocked(viewer.PlayerID, target.PlayerID, clock.Now())
	gameServer.runtime.mu.Unlock()
	if witnessed {
		t.Fatal("hidden player witness active outside projection, want none")
	}

	events, err := gameServer.runtime.postCommandEvents(viewer.SessionID, realtime.OperationScanPulse, viewer.PlayerID)
	if err != nil {
		t.Fatalf("post scan projection miss events: %v", err)
	}
	for _, event := range events {
		rawEvent := string(mustJSON(t, event))
		if strings.Contains(rawEvent, targetEntityID.String()) || strings.Contains(rawEvent, target.PlayerID.String()) {
			t.Fatalf("scan projection miss event leaked target in %s", rawEvent)
		}
	}

	afterEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("viewer after projection miss bootstrap events: %v", err)
	}
	afterSnapshot := decodeWorldSnapshotForTest(t, afterEvents)
	if hasEntityID(afterSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw out-of-projection hidden target after scan: %+v", afterSnapshot.Entities)
	}
}

func TestRuntimePlayerStealthAppliesSpeedPenaltyWithoutStackingAndRecalculatesRoute(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		SessionTTL:     time.Hour,
		TickDelta:      50 * time.Millisecond,
		Clock:          clock,
		PasswordHasher: auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	resolved := createResolvedRuntimeSession(t, gameServer, "stealth-speed@example.com", "Stealth")

	gameServer.runtime.mu.Lock()
	if err := gameServer.runtime.Worker.Submit(worker.MoveToCommand{
		PlayerID: resolved.PlayerID,
		Intent:   mustMovementIntentForServerTest(t, world.Vec2{X: 1_000, Y: 0}),
	}); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("Submit(move) error = %v, want nil", err)
	}
	moveResult := gameServer.runtime.Worker.Tick()
	gameServer.runtime.mu.Unlock()
	if len(moveResult.CommandErrors) > 0 {
		t.Fatalf("move command errors = %+v, want none", moveResult.CommandErrors)
	}

	clock.Advance(500 * time.Millisecond)
	if err := gameServer.runtime.setPlayerStealth(resolved.PlayerID, true); err != nil {
		t.Fatalf("set stealth true error = %v, want nil", err)
	}

	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[resolved.PlayerID]
	entity, ok := gameServer.runtime.Worker.PlayerEntity(resolved.PlayerID)
	hidden := gameServer.runtime.hiddenPlayers[resolved.PlayerID]
	speed, speedOK := gameServer.runtime.Worker.EntitySpeed(state.EntityID)
	gameServer.runtime.mu.Unlock()
	if !ok || !speedOK || !hidden {
		t.Fatalf("runtime stealth state entity=%v speed=%v hidden=%v, want all true", ok, speedOK, hidden)
	}
	assertServerFloatNear(t, state.Stats.Speed, defaultPlayerSpeed*runtimeStealthSpeedMultiplier)
	assertServerFloatNear(t, speed, state.Stats.Speed)
	assertServerVecNear(t, entity.Movement.Origin, world.Vec2{X: 90, Y: 0})
	if entity.Movement.Target != (world.Vec2{X: 1_000, Y: 0}) {
		t.Fatalf("stealth movement target = %+v, want original target", entity.Movement.Target)
	}
	assertServerFloatNear(t, entity.Movement.Speed, defaultPlayerSpeed*runtimeStealthSpeedMultiplier)

	if err := gameServer.runtime.setPlayerStealth(resolved.PlayerID, true); err != nil {
		t.Fatalf("set stealth true duplicate error = %v, want nil", err)
	}
	gameServer.runtime.mu.Lock()
	duplicateState := gameServer.runtime.players[resolved.PlayerID]
	duplicateSpeed, _ := gameServer.runtime.Worker.EntitySpeed(duplicateState.EntityID)
	gameServer.runtime.mu.Unlock()
	assertServerFloatNear(t, duplicateState.Stats.Speed, defaultPlayerSpeed*runtimeStealthSpeedMultiplier)
	assertServerFloatNear(t, duplicateSpeed, duplicateState.Stats.Speed)

	if err := gameServer.runtime.setPlayerStealth(resolved.PlayerID, false); err != nil {
		t.Fatalf("set stealth false error = %v, want nil", err)
	}
	gameServer.runtime.mu.Lock()
	restoredState := gameServer.runtime.players[resolved.PlayerID]
	restoredHidden := gameServer.runtime.hiddenPlayers[resolved.PlayerID]
	restoredSpeed, _ := gameServer.runtime.Worker.EntitySpeed(restoredState.EntityID)
	gameServer.runtime.mu.Unlock()
	if restoredHidden {
		t.Fatal("hiddenPlayers[player] = true after disable, want false")
	}
	if restoredState.Stats.Speed != defaultPlayerSpeed || restoredSpeed != defaultPlayerSpeed {
		t.Fatalf("restored speed state=%v worker=%v, want %v", restoredState.Stats.Speed, restoredSpeed, defaultPlayerSpeed)
	}
}

func TestRuntimePlayerStealthRestoresServerEffectiveSpeed(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "stealth-effective-speed@example.com", "Stealth Effective")
	const baseSpeed = 260.0

	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[resolved.PlayerID]
	state.Stats.Speed = baseSpeed
	gameServer.runtime.players[resolved.PlayerID] = state
	if err := gameServer.runtime.Worker.Submit(worker.SetPlayerSpeedCommand{PlayerID: resolved.PlayerID, Speed: baseSpeed}); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("Submit(base speed) error = %v, want nil", err)
	}
	result := gameServer.runtime.Worker.Tick()
	gameServer.runtime.mu.Unlock()
	if len(result.CommandErrors) > 0 {
		t.Fatalf("base speed command errors = %+v, want none", result.CommandErrors)
	}

	if err := gameServer.runtime.setPlayerStealth(resolved.PlayerID, true); err != nil {
		t.Fatalf("set stealth true error = %v, want nil", err)
	}
	gameServer.runtime.mu.Lock()
	hiddenState := gameServer.runtime.players[resolved.PlayerID]
	hiddenSpeed, _ := gameServer.runtime.Worker.EntitySpeed(hiddenState.EntityID)
	gameServer.runtime.mu.Unlock()
	assertServerFloatNear(t, hiddenState.Stats.Speed, baseSpeed*runtimeStealthSpeedMultiplier)
	assertServerFloatNear(t, hiddenSpeed, hiddenState.Stats.Speed)

	if err := gameServer.runtime.setPlayerStealth(resolved.PlayerID, false); err != nil {
		t.Fatalf("set stealth false error = %v, want nil", err)
	}
	gameServer.runtime.mu.Lock()
	restoredState := gameServer.runtime.players[resolved.PlayerID]
	restoredSpeed, _ := gameServer.runtime.Worker.EntitySpeed(restoredState.EntityID)
	gameServer.runtime.mu.Unlock()
	if restoredState.Stats.Speed != baseSpeed || restoredSpeed != baseSpeed {
		t.Fatalf("restored speed state=%v worker=%v, want %v", restoredState.Stats.Speed, restoredSpeed, baseSpeed)
	}
}

func TestStealthToggleCommandUsesServerOwnedStateAndSafePayload(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		SessionTTL:     time.Hour,
		TickDelta:      50 * time.Millisecond,
		Clock:          clock,
		PasswordHasher: auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	resolved := createResolvedRuntimeSession(t, gameServer, "stealth-command@example.com", "Stealth Command")

	bootstrapEvents, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	initialSnapshot := decodeWorldSnapshotForTest(t, bootstrapEvents)
	selfEntityID := testPlayerEntityID(t, gameServer, resolved.PlayerID).String()
	if entity, ok := entityPayloadByID(initialSnapshot.Entities, selfEntityID); !ok || hasStatusFlag(entity.StatusFlags, "stealthed") {
		t.Fatalf("initial self entity = %+v ok=%v, want visible without stealthed", entity, ok)
	}

	forged := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-stealth-forged","op":"stealth.toggle","payload":{"enabled":true,"hidden":true},"client_seq":1,"v":1}`),
	)
	if !forged.HasError || forged.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("forged stealth response = %+v, want invalid payload", forged)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-stealth-enable","op":"stealth.toggle","payload":{"enabled":true},"client_seq":2,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("stealth enable response error = %+v, want success", response.Error)
	}
	rawResponse := string(response.Response.Payload)
	for _, forbidden := range []string{"hidden", "player_id", resolved.PlayerID.String()} {
		if strings.Contains(rawResponse, forbidden) {
			t.Fatalf("stealth response leaked %q in %s", forbidden, rawResponse)
		}
	}
	var payload struct {
		Accepted bool `json:"accepted"`
		Stealth  struct {
			Enabled bool `json:"enabled"`
		} `json:"stealth"`
		Stats statSnapshotPayload `json:"stats"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode stealth response: %v", err)
	}
	if !payload.Accepted || !payload.Stealth.Enabled {
		t.Fatalf("stealth payload = %+v, want accepted enabled", payload)
	}
	assertServerFloatNear(t, payload.Stats.Speed, defaultPlayerSpeed*runtimeStealthSpeedMultiplier)

	events, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationStealthToggle, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post stealth events: %v", err)
	}
	seenStats := false
	seenSelfUpdate := false
	for _, event := range events {
		rawEvent := string(mustJSON(t, event))
		for _, forbidden := range []string{"hidden", "player_id", resolved.PlayerID.String()} {
			if strings.Contains(rawEvent, forbidden) {
				t.Fatalf("stealth event leaked %q in %s", forbidden, rawEvent)
			}
		}
		switch event.Type {
		case realtime.EventStatsUpdated:
			seenStats = true
		case realtime.EventAOIEntityUpdated, realtime.EventAOIEntityEntered:
			var entity aoi.EntityPayload
			if err := json.Unmarshal(event.Payload, &entity); err != nil {
				t.Fatalf("decode stealth AOI entity: %v", err)
			}
			if entity.ID.String() == selfEntityID && hasStatusFlag(entity.StatusFlags, "stealthed") {
				seenSelfUpdate = true
			}
		}
	}
	if !seenStats || !seenSelfUpdate {
		t.Fatalf("stealth post events = %+v, want stats update and self AOI stealthed update", events)
	}

	disable := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-stealth-disable","op":"stealth.toggle","payload":{"enabled":false},"client_seq":3,"v":1}`),
	)
	if disable.HasError {
		t.Fatalf("stealth disable response error = %+v, want success", disable.Error)
	}
	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[resolved.PlayerID]
	hidden := gameServer.runtime.hiddenPlayers[resolved.PlayerID]
	gameServer.runtime.mu.Unlock()
	if hidden || state.Stats.Speed != defaultPlayerSpeed {
		t.Fatalf("disable hidden=%v speed=%v, want false/%v", hidden, state.Stats.Speed, defaultPlayerSpeed)
	}
}

func TestWorldSnapshotProjectionPolicyIsServerOwnedAndSeparateFromRadarStat(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "projection@example.com", "Projection")
	other := createResolvedRuntimeSession(t, gameServer, "projection-other@example.com", "Projection Other")
	moveTestPlayerEntity(gameServer, other.PlayerID, world.Vec2{X: -750, Y: 100})
	insertTestWorldEntity(t, gameServer, "entity_projection_corner", world.EntityTypeNPC, world.Vec2{X: runtimeLiveProjectionHalfExtent, Y: runtimeLiveProjectionHalfExtent}, false)
	insertTestWorldEntity(t, gameServer, "entity_projection_loot", world.EntityTypeLoot, world.Vec2{X: 640, Y: -120}, false)
	insertTestWorldEntity(t, gameServer, "entity_projection_outside", world.EntityTypeNPC, world.Vec2{X: runtimeLiveProjectionHalfExtent + 1, Y: 0}, false)
	insertTestWorldEntity(t, gameServer, "entity_projection_hidden_inside", world.EntityTypeNPC, world.Vec2{X: 100, Y: 100}, true)

	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[resolved.PlayerID]
	state.Stats.RadarRange = 10
	gameServer.runtime.players[resolved.PlayerID] = state
	gameServer.runtime.mu.Unlock()

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	snapshot := decodeWorldSnapshotForTest(t, events)

	if snapshot.Minimap.RadarRange != runtimeLiveProjectionHalfExtent || snapshot.Minimap.ProjectionWindowSize != runtimeLiveProjectionDiameter {
		t.Fatalf("projection payload = %+v, want half extent/window", snapshot.Minimap)
	}
	otherEntityID := testPlayerEntityID(t, gameServer, other.PlayerID)
	for _, want := range []string{"entity_projection_corner", "entity_projection_loot", otherEntityID.String()} {
		if !hasEntityID(snapshot.Entities, want) {
			t.Fatalf("projection snapshot missing %s: %+v", want, snapshot.Entities)
		}
		contact, ok := minimapContactByID(snapshot.Minimap.LiveContacts, want)
		if !ok || contact.ProjectionSource != runtimeProjectionSourceWorker {
			t.Fatalf("projection contact %s = %+v, want source %q", want, contact, runtimeProjectionSourceWorker)
		}
	}
	for _, forbidden := range []string{"entity_projection_outside", "entity_projection_hidden_inside"} {
		if hasEntityID(snapshot.Entities, forbidden) {
			t.Fatalf("projection snapshot included %s: %+v", forbidden, snapshot.Entities)
		}
		for _, contact := range snapshot.Minimap.LiveContacts {
			if contact.EntityID == forbidden {
				t.Fatalf("projection minimap leaked %s: %+v", forbidden, snapshot.Minimap.LiveContacts)
			}
		}
	}
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	if got := gameServer.runtime.players[resolved.PlayerID].Stats.RadarRange; got != 10 {
		t.Fatalf("player stat radar range = %v, want unchanged 10", got)
	}
}

func TestWorldSnapshotFarRememberedPlanetStaysMemoryNotLiveContact(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "projection-memory@example.com", "Projection Memory")
	now := gameServer.runtime.clock.Now().UTC()
	planetID := foundation.PlanetID("planet-far-memory")
	coordinates := world.Vec2{X: 5200, Y: -3800}

	if _, err := gameServer.runtime.Discovery.MaterializePlanet(discovery.MaterializePlanetInput{
		CandidateKey: "candidate-far-memory",
		Planet: discovery.Planet{
			ID:           planetID,
			WorldID:      gameServer.runtime.worldID,
			ZoneID:       gameServer.runtime.zoneID,
			Coordinates:  coordinates,
			Biome:        discovery.PlanetBiomeOuterDrift,
			Type:         discovery.PlanetTypeIce,
			Rarity:       discovery.PlanetRarityUncommon,
			Level:        2,
			DiscoveredAt: now,
			DiscoveredBy: resolved.PlayerID,
		},
	}); err != nil {
		t.Fatalf("MaterializePlanet() error = %v, want nil", err)
	}
	if _, _, err := gameServer.runtime.Discovery.UpsertPlayerPlanetIntel(discovery.PlayerPlanetIntel{
		PlayerID:        resolved.PlayerID,
		PlanetID:        planetID,
		Coordinates:     coordinates,
		State:           discovery.IntelStateFresh,
		Confidence:      100,
		LastSeenAt:      now,
		SourceType:      discovery.IntelSourceAdmin,
		SourceReference: "far-memory-fixture",
	}); err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel() error = %v, want nil", err)
	}

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	snapshot := decodeWorldSnapshotForTest(t, events)

	for _, contact := range snapshot.Minimap.LiveContacts {
		if contact.EntityID == planetID.String() {
			t.Fatalf("far remembered planet became live radar contact: %+v", snapshot.Minimap.LiveContacts)
		}
	}
	if len(snapshot.Minimap.Remembered) != 1 {
		t.Fatalf("remembered minimap = %+v, want one far planet memory", snapshot.Minimap.Remembered)
	}
	memory := snapshot.Minimap.Remembered[0]
	if memory.PlanetID != planetID.String() || memory.DetailID != planetID.String() {
		t.Fatalf("far memory ids = %+v, want planet/detail %s", memory, planetID)
	}
	if memory.Position != coordinates {
		t.Fatalf("far memory position = %+v, want unclipped coordinates %+v", memory.Position, coordinates)
	}
	if memory.Freshness != string(discovery.IntelStateFresh) {
		t.Fatalf("far memory freshness = %q, want fresh", memory.Freshness)
	}
	if memory.SectorKey != runtimeSectorKey || memory.ProjectionSource != runtimeProjectionSourceKnownIntel {
		t.Fatalf("far memory sector/source = %+v, want %s/%s", memory, runtimeSectorKey, runtimeProjectionSourceKnownIntel)
	}
}

func TestWorldProjectionSourcesReconcileAfterServerOwnedMovement(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "projection-move@example.com", "Projection Move")
	insertTestWorldEntity(t, gameServer, "entity_projection_departing", world.EntityTypeNPC, world.Vec2{X: -900, Y: 0}, false)
	insertTestWorldEntity(t, gameServer, "entity_projection_arriving", world.EntityTypeLoot, world.Vec2{X: 1500, Y: 0}, false)
	insertTestWorldEntity(t, gameServer, "entity_projection_hidden_arriving", world.EntityTypeNPC, world.Vec2{X: 1500, Y: 10}, true)

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	initial := decodeWorldSnapshotForTest(t, events)
	if !hasEntityID(initial.Entities, "entity_projection_departing") || hasEntityID(initial.Entities, "entity_projection_arriving") {
		t.Fatalf("initial projection entities = %+v, want departing visible and arriving outside", initial.Entities)
	}

	moveTestPlayerEntity(gameServer, resolved.PlayerID, world.Vec2{X: 600, Y: 0})
	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	sessionEvents := eventsBySession[resolved.SessionID]
	if len(sessionEvents) == 0 {
		t.Fatalf("movement projection events = none, want entered/left")
	}
	seenEntered := false
	seenLeft := false
	for _, event := range sessionEvents {
		raw := string(event.Payload)
		if strings.Contains(raw, "entity_projection_hidden_arriving") {
			t.Fatalf("hidden arriving entity leaked into movement AOI event %s", raw)
		}
		switch event.Type {
		case realtime.EventAOIEntityEntered:
			var entered aoi.EntityPayload
			if err := json.Unmarshal(event.Payload, &entered); err != nil {
				t.Fatalf("decode entered entity: %v", err)
			}
			if entered.ID == "entity_projection_arriving" {
				seenEntered = true
				if entered.ProjectionSource != runtimeProjectionSourceWorker {
					t.Fatalf("entered projection source = %q, want %q", entered.ProjectionSource, runtimeProjectionSourceWorker)
				}
			}
		case realtime.EventAOIEntityLeft:
			var left map[string]string
			if err := json.Unmarshal(event.Payload, &left); err != nil {
				t.Fatalf("decode left entity: %v", err)
			}
			if left["entity_id"] == "entity_projection_departing" {
				seenLeft = true
			}
		}
	}
	if !seenEntered || !seenLeft {
		t.Fatalf("movement projection events = %+v, want arriving entered and departing left", sessionEvents)
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

func TestProductionWebSocketForbidsDebugOperationsAndKeepsSessionSnapshotPublic(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-session-snapshot","op":"session.snapshot","payload":{},"client_seq":1,"v":1}`)
	session := readResponse(t, conn)
	if !session.OK {
		t.Fatalf("session snapshot response = %+v, want success", session)
	}
	if session.RequestID != foundation.RequestID("request-session-snapshot") || session.Version != realtime.CurrentVersion {
		t.Fatalf("session envelope = %+v, want request id/version", session)
	}
	var sessionPayload sessionReadyPayload
	if err := json.Unmarshal(session.Payload, &sessionPayload); err != nil {
		t.Fatalf("decode session snapshot: %v", err)
	}
	if !sessionPayload.Authenticated || sessionPayload.Account == nil || sessionPayload.Account.Email != "pilot@example.com" || sessionPayload.Account.Admin {
		t.Fatalf("session account payload = %+v, want public authenticated pilot account", sessionPayload)
	}
	if sessionPayload.Player == nil || sessionPayload.Player.Callsign != "Frontier-01" || sessionPayload.ProtocolVersion != realtime.CurrentVersion {
		t.Fatalf("session player/protocol payload = %+v, want public player and protocol version", sessionPayload)
	}
	rawSession := string(session.Payload)
	for _, forbidden := range []string{"session_id", "account_id", "player_id", "password"} {
		if strings.Contains(rawSession, forbidden) {
			t.Fatalf("session snapshot leaked %q in %s", forbidden, rawSession)
		}
	}

	for index, body := range []string{
		`{"request_id":"request-debug-snapshot","op":"debug_snapshot","payload":{},"client_seq":2,"v":1}`,
		`{"request_id":"request-debug-spawn","op":"debug_spawn_npc","payload":{"entity_id":"debug_npc","position":{"x":1,"y":2}},"client_seq":3,"v":1}`,
		`{"request_id":"request-debug-spawn-spoof","op":"debug_spawn_npc","payload":{"entity_id":"debug_npc","position":{"x":1,"y":2},"player_id":"spoof"},"client_seq":4,"v":1}`,
	} {
		writeText(t, conn, body)
		response := readError(t, conn)
		if response.Error.Code != foundation.CodeForbidden {
			t.Fatalf("debug response %d = %+v, want %s", index, response.Error, foundation.CodeForbidden)
		}
		if response.Error.Retryable {
			t.Fatalf("debug response %d retryable = true, want false", index)
		}
		message := strings.ToLower(response.Error.Message)
		if strings.Contains(message, "dev") || strings.Contains(message, "internal") {
			t.Fatalf("debug response leaked internal mode copy: %+v", response.Error)
		}
	}

	writeText(t, conn, `{"request_id":"request-world-after-debug","op":"world.snapshot","payload":{},"client_seq":5,"v":1}`)
	world := readResponse(t, conn)
	if !world.OK {
		t.Fatalf("world snapshot after debug forbids = %+v, want live socket", world)
	}
}

func TestRejectTrustedPayloadSharedSensitiveFieldsAndAdminException(t *testing.T) {
	for _, field := range []string{
		"account_id",
		"client_player_id",
		"player_id",
		"session_id",
		"world_id",
		"zone_id",
		"speed",
		"hidden",
		"internal_metadata",
		"gameplay_seed",
		"procedural_seed",
		"world_seed",
		"future_spawn_data",
		"candidate_key",
		"detection_roll",
		"scan_roll",
		"scan_cell",
		"scan_result",
		"scan_candidates",
		"target_player_id",
		"witness_expires_at",
		"hidden_target_metadata",
		"provider",
		"provider_reference",
		"source_return_location",
		"seller_player_id",
		"buyer_player_id",
		"bidder_player_id",
		"winning_player_id",
		"generated_payload",
		"generated_seed",
		"loot_roll",
		"password",
		"password_hash",
		"token",
		"session_token",
		"reset_secret",
		"auth_header",
		"cookie",
	} {
		payload := json.RawMessage(fmt.Sprintf(`{"outer":[{%q:"spoof"}]}`, field))
		err := rejectTrustedPayload(payload)
		if !foundation.IsCode(err, foundation.CodeInvalidPayload) || !strings.Contains(err.Error(), field) {
			t.Fatalf("rejectTrustedPayload(%s) error = %v, want invalid payload naming field", field, err)
		}
	}

	if err := rejectTrustedPayloadAllowing(json.RawMessage(`{"target_player_id":"player-admin-target"}`), "target_player_id"); err != nil {
		t.Fatalf("admin target exception rejected: %v", err)
	}
	err := rejectTrustedPayloadAllowing(json.RawMessage(`{"nested":{"target_player_id":"player-admin-target"}}`), "target_player_id")
	if !foundation.IsCode(err, foundation.CodeInvalidPayload) || !strings.Contains(err.Error(), "target_player_id") {
		t.Fatalf("admin target exception nested target_player_id error = %v, want invalid payload", err)
	}
	err = rejectTrustedPayloadAllowing(json.RawMessage(`{"target_player_id":"player-admin-target","nested":{"player_id":"spoof"}}`), "target_player_id")
	if !foundation.IsCode(err, foundation.CodeInvalidPayload) || !strings.Contains(err.Error(), "player_id") {
		t.Fatalf("admin target exception nested player_id error = %v, want invalid payload", err)
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

func TestRuntimeDetachSettlesMovementBeforeReconnectSnapshot(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		SessionTTL:     time.Hour,
		TickDelta:      50 * time.Millisecond,
		PasswordHasher: auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
		Clock:          clock,
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	resolved := createResolvedRuntimeSession(t, gameServer, "settle-reconnect@example.com", "Settle")
	if _, err := gameServer.runtime.bootstrapEvents(resolved); err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-detach-move","op":"move_to","payload":{"target":{"x":100,"y":0}},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("move response error = %+v, want success", response.Error)
	}

	clock.Advance(250 * time.Millisecond)
	gameServer.runtime.detachSession(resolved.SessionID)
	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("re-ensure session: %v", err)
	}
	reconnectEvents, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("reconnect bootstrap events: %v", err)
	}
	snapshot := decodeWorldSnapshotForTest(t, reconnectEvents)

	var self *aoi.EntityPayload
	for index := range snapshot.Entities {
		if hasStatusFlag(snapshot.Entities[index].StatusFlags, "self") {
			self = &snapshot.Entities[index]
			break
		}
	}
	if self == nil {
		t.Fatalf("reconnect snapshot entities = %+v, missing self", snapshot.Entities)
	}
	if self.Movement != nil {
		t.Fatalf("reconnect self movement = %+v, want settled/stopped", self.Movement)
	}
	if self.Position.X <= defaultPlayerSpeed*0.05 || self.Position.X >= 100 {
		t.Fatalf("reconnect self x = %v, want settled intermediate position after disconnect", self.Position.X)
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

func setTestHiddenPlayer(gameServer *Server, playerID foundation.PlayerID, hidden bool) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	gameServer.runtime.hiddenPlayers[playerID] = hidden
}

func setTestHiddenPlayerWitness(gameServer *Server, viewerID foundation.PlayerID, targetID foundation.PlayerID, expiresAt time.Time) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	gameServer.runtime.hiddenPlayerWitnesses[hiddenPlayerWitnessKey{
		ViewerPlayerID: viewerID,
		TargetPlayerID: targetID,
	}] = expiresAt
}

func testPlayerEntityID(t *testing.T, gameServer *Server, playerID foundation.PlayerID) world.EntityID {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state, ok := gameServer.runtime.players[playerID]
	if !ok {
		t.Fatalf("player %q missing runtime state", playerID)
	}
	return state.EntityID
}

func insertTestWorldEntity(t *testing.T, gameServer *Server, entityID world.EntityID, entityType world.EntityType, position world.Vec2, hidden bool) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	entity, err := world.NewEntity(gameServer.runtime.worldID, gameServer.runtime.zoneID, entityID, entityType, position)
	if err != nil {
		t.Fatalf("NewEntity(%q) = %v, want nil", entityID, err)
	}
	if err := gameServer.runtime.Worker.InsertEntity(entity, 0); err != nil {
		t.Fatalf("InsertEntity(%q) = %v, want nil", entityID, err)
	}
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

func requireInventoryInstance(t *testing.T, inventory inventorySnapshotPayload, itemID string, location string) string {
	t.Helper()
	for _, item := range inventory.Instances {
		if item.ItemID == itemID && item.Location == location {
			if item.DurabilityCurrent <= 0 {
				t.Fatalf("inventory instance = %+v, want positive durability", item)
			}
			return item.ItemInstanceID
		}
	}
	t.Fatalf("inventory = %+v, missing instance %s at %s", inventory, itemID, location)
	return ""
}

func requireInventoryInstanceLocation(t *testing.T, inventory inventorySnapshotPayload, itemInstanceID string, location string) {
	t.Helper()
	for _, item := range inventory.Instances {
		if item.ItemInstanceID == itemInstanceID {
			if item.Location != location {
				t.Fatalf("inventory instance = %+v, want location %s", item, location)
			}
			return
		}
	}
	t.Fatalf("inventory = %+v, missing instance %s", inventory, itemInstanceID)
}

func requireLoadoutSlot(t *testing.T, loadout loadoutSnapshotPayload, slotID string) loadoutSlotPayload {
	t.Helper()
	for _, slot := range loadout.Slots {
		if slot.SlotID == slotID {
			return slot
		}
	}
	t.Fatalf("loadout = %+v, missing slot %s", loadout, slotID)
	return loadoutSlotPayload{}
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

func assertMinimapMirrorsEntities(t *testing.T, label string, entities []aoi.EntityPayload, minimap minimapPayload) {
	t.Helper()
	if minimap.RadarRange != runtimeLiveProjectionHalfExtent || minimap.ProjectionWindowSize != runtimeLiveProjectionDiameter {
		t.Fatalf("%s minimap projection = range %v window %v, want %v/%v", label, minimap.RadarRange, minimap.ProjectionWindowSize, runtimeLiveProjectionHalfExtent, runtimeLiveProjectionDiameter)
	}
	if len(minimap.LiveContacts) != len(entities) {
		t.Fatalf("%s minimap contacts = %d, entities = %d", label, len(minimap.LiveContacts), len(entities))
	}
	entitiesByID := make(map[string]aoi.EntityPayload, len(entities))
	for _, entity := range entities {
		entitiesByID[entity.ID.String()] = entity
	}
	for _, contact := range minimap.LiveContacts {
		if contact.EntityID == "" || contact.EntityType == "" {
			t.Fatalf("%s minimap contact missing stable identity: %+v", label, contact)
		}
		entity, ok := entitiesByID[contact.EntityID]
		if !ok {
			t.Fatalf("%s minimap contact %+v missing matching entity", label, contact)
		}
		if contact.EntityType != entity.Type || contact.Position != entity.Position {
			t.Fatalf("%s minimap contact %+v does not mirror entity %+v", label, contact, entity)
		}
		if contact.ProjectionSource != entity.ProjectionSource {
			t.Fatalf("%s minimap contact %+v projection source does not mirror entity %+v", label, contact, entity)
		}
	}
}

func minimapContactByID(contacts []minimapContactPayload, want string) (minimapContactPayload, bool) {
	for _, contact := range contacts {
		if contact.EntityID == want {
			return contact, true
		}
	}
	return minimapContactPayload{}, false
}

func entityPayloadByID(entities []aoi.EntityPayload, want string) (aoi.EntityPayload, bool) {
	for _, entity := range entities {
		if entity.ID.String() == want {
			return entity, true
		}
	}
	return aoi.EntityPayload{}, false
}

func mustMovementIntentForServerTest(t *testing.T, target world.Vec2) world.MovementIntent {
	t.Helper()
	intent, err := world.NewMovementIntent(target)
	if err != nil {
		t.Fatalf("NewMovementIntent(%+v) error = %v, want nil", target, err)
	}
	return intent
}

func assertServerVecNear(t *testing.T, got world.Vec2, want world.Vec2) {
	t.Helper()
	if math.Abs(got.X-want.X) > 0.05 || math.Abs(got.Y-want.Y) > 0.05 {
		t.Fatalf("vector = %+v, want near %+v", got, want)
	}
}

func assertServerFloatNear(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("float = %v, want near %v", got, want)
	}
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

type movementPayloadForTest struct {
	Moving      bool       `json:"moving"`
	Origin      world.Vec2 `json:"origin"`
	Target      world.Vec2 `json:"target"`
	Speed       float64    `json:"speed"`
	StartedAtMS int64      `json:"started_at_ms"`
	ArriveAtMS  int64      `json:"arrive_at_ms"`
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
		"server_recalculates",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}

func countInventoryInstances(items []economy.InstanceItem, itemID string) int {
	count := 0
	for _, item := range items {
		if item.ItemID.String() == itemID {
			count++
		}
	}
	return count
}

func inventorySnapshotHasInstance(snapshot inventorySnapshotPayload, itemID string) bool {
	for _, item := range snapshot.Instances {
		if item.ItemID == itemID {
			return true
		}
	}
	return false
}

func runtimeWalletCredits(t *testing.T, runtime *Runtime) int64 {
	t.Helper()
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	for _, playerID := range runtime.sessions {
		return runtime.walletSnapshotLocked(playerID).Credits
	}
	t.Fatal("runtime has no authenticated player session")
	return 0
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
	return decodeRawResponse(t, readRawText(t, conn))
}

func readResponseSkippingEvents(t *testing.T, conn *websocket.Conn) realtime.ResponseEnvelope {
	t.Helper()
	for range 8 {
		data := readRawText(t, conn)
		if !rawRealtimeMessageIsResponse(data) {
			continue
		}
		return decodeRawResponse(t, data)
	}
	t.Fatal("no response received before event skip limit")
	return realtime.ResponseEnvelope{}
}

func readErrorSkippingEvents(t *testing.T, conn *websocket.Conn) realtime.ErrorEnvelope {
	t.Helper()
	for range 8 {
		data := readRawText(t, conn)
		if !rawRealtimeMessageIsResponse(data) {
			continue
		}
		var response realtime.ErrorEnvelope
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatalf("decode error %s: %v", data, err)
		}
		if response.OK {
			t.Fatalf("error response %s had ok=true", data)
		}
		return response
	}
	t.Fatal("no error response received before event skip limit")
	return realtime.ErrorEnvelope{}
}

func rawRealtimeMessageIsResponse(data []byte) bool {
	var probe struct {
		OK *bool `json:"ok"`
	}
	return json.Unmarshal(data, &probe) == nil && probe.OK != nil
}

func decodeRawResponse(t *testing.T, data []byte) realtime.ResponseEnvelope {
	t.Helper()
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
