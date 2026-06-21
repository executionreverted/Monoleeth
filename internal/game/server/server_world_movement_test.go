package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
)

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
	if snapshot.Sector.SectorKey != "1-1" {
		t.Fatalf("sector key = %q, want current public map key", snapshot.Sector.SectorKey)
	}
	if snapshot.Map.MapKey != "1-1" || snapshot.Map.PublicMapKey != "1-1" || snapshot.Map.Bounds != worldmaps.ExactPlayableBounds() {
		t.Fatalf("map projection = %+v, want public 1-1 exact-bounds projection", snapshot.Map)
	}
	if len(snapshot.Map.VisiblePortals) != 1 || snapshot.Map.VisiblePortals[0].PortalID == "" {
		t.Fatalf("map visible portals = %+v, want client-safe visible portal summary", snapshot.Map.VisiblePortals)
	}
	rawSnapshot := string(mustJSON(t, snapshot))
	for _, forbidden := range []string{
		"internal_map_id",
		"map_id",
		"zone_id",
		"worker_id",
		"map_worker_id",
		"destination_map_id",
		"destination_spawn_id",
		"map_1_1",
		"map_1_2",
		"gameplay_seed",
		"procedural_seed",
		"enemy_pool",
	} {
		if strings.Contains(rawSnapshot, forbidden) {
			t.Fatalf("world snapshot leaked %q in %s", forbidden, rawSnapshot)
		}
	}
	if snapshot.Minimap.RadarRange != defaultRadarRange || snapshot.Minimap.ProjectionWindowSize != defaultRadarRange*2 {
		t.Fatalf("minimap projection = range %v window %v, want %v/%v", snapshot.Minimap.RadarRange, snapshot.Minimap.ProjectionWindowSize, defaultRadarRange, defaultRadarRange*2)
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
		Accepted     bool                          `json:"accepted"`
		PublicMapKey string                        `json:"public_map_key"`
		Map          worldmaps.ClientMapProjection `json:"map"`
		Entities     []aoi.EntityPayload           `json:"entities"`
		Minimap      minimapPayload                `json:"minimap"`
	}
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode move payload: %v", err)
	}
	if !payload.Accepted {
		t.Fatal("move accepted = false, want true")
	}
	if payload.PublicMapKey != "1-1" || payload.Map.MapKey != "1-1" || payload.Map.Bounds != worldmaps.ExactPlayableBounds() {
		t.Fatalf("move map projection = key %q map %+v, want current public 1-1 map", payload.PublicMapKey, payload.Map)
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

	for _, tc := range []struct {
		name   string
		target world.Vec2
	}{
		{name: "x below bounds", target: world.Vec2{X: -1, Y: 0}},
		{name: "y below bounds", target: world.Vec2{X: 0, Y: -1}},
		{name: "x above bounds", target: world.Vec2{X: worldmaps.PlayableMaxCoordinate + 1, Y: 0}},
		{name: "y above bounds", target: world.Vec2{X: 0, Y: worldmaps.PlayableMaxCoordinate + 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, httpServer := newTestServer(t, false)
			defer httpServer.Close()
			conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
			defer conn.CloseNow()
			readBootstrapEvents(t, conn)

			writeText(t, conn, fmt.Sprintf(`{"request_id":"request-move-%s","op":"move_to","payload":{"target":{"x":%v,"y":%v}},"client_seq":1,"v":1}`, strings.ReplaceAll(tc.name, " ", "-"), tc.target.X, tc.target.Y))
			got := readError(t, conn)
			if got.Error.Code != foundation.CodeOutOfRange {
				t.Fatalf("out-of-bounds move error = %+v, want %s", got.Error, foundation.CodeOutOfRange)
			}
		})
	}

	t.Run("trusted map payload", func(t *testing.T) {
		_, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)

		writeText(t, conn, `{"request_id":"request-move-map-spoof","op":"move_to","payload":{"target":{"x":100,"y":0},"map_id":"map_1_2"},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeInvalidPayload {
			t.Fatalf("map spoof move error = %+v, want %s", got.Error, foundation.CodeInvalidPayload)
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
