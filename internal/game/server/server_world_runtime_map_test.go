package server

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

func TestWorldProjectionSourcesReconcileAfterServerOwnedMovement(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "projection-move@example.com", "Projection Move")
	arrivingPosition := world.Vec2{X: defaultRadarRange + 200, Y: 0}
	movePosition := world.Vec2{X: defaultRadarRange + 80, Y: 0}
	insertTestWorldEntity(t, gameServer, "entity_projection_departing", world.EntityTypeNPC, world.Vec2{X: 0, Y: 0}, false)
	insertTestWorldEntity(t, gameServer, "entity_projection_arriving", world.EntityTypeLoot, arrivingPosition, false)
	insertTestWorldEntity(t, gameServer, "entity_projection_hidden_arriving", world.EntityTypeNPC, world.Vec2{X: arrivingPosition.X, Y: 10}, true)

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	initial := decodeWorldSnapshotForTest(t, events)
	if !hasEntityID(initial.Entities, "entity_projection_departing") || hasEntityID(initial.Entities, "entity_projection_arriving") {
		t.Fatalf("initial projection entities = %+v, want departing visible and arriving outside", initial.Entities)
	}

	moveTestPlayerEntity(gameServer, resolved.PlayerID, movePosition)
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
			var left struct {
				EntityID             string `json:"entity_id"`
				MapSubscriptionEpoch uint64 `json:"map_subscription_epoch"`
			}
			if err := json.Unmarshal(event.Payload, &left); err != nil {
				t.Fatalf("decode left entity: %v", err)
			}
			if left.EntityID == "entity_projection_departing" {
				seenLeft = true
				if left.MapSubscriptionEpoch == 0 {
					t.Fatalf("left event missing map_subscription_epoch: %s", event.Payload)
				}
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
func TestRuntimeConstructsWorkerPerConfiguredMapDefinition(t *testing.T) {
	gameServer, _ := newTestServer(t, false)

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	definitions := gameServer.runtime.mapCatalog.Definitions()
	if len(gameServer.runtime.mapInstances) != len(definitions) {
		t.Fatalf("map instances = %d, definitions = %d", len(gameServer.runtime.mapInstances), len(definitions))
	}
	for _, definition := range definitions {
		instance, err := gameServer.runtime.mapInstanceLocked(definition.InternalMapID)
		if err != nil {
			t.Fatalf("mapInstanceLocked(%q) = %v, want nil", definition.InternalMapID, err)
		}
		if instance.Worker.WorldID() != definition.WorldID {
			t.Fatalf("worker %q world = %q, want %q", definition.InternalMapID, instance.Worker.WorldID(), definition.WorldID)
		}
		if instance.Worker.ZoneID() != definition.InternalMapID.ZoneID() {
			t.Fatalf("worker %q zone = %q, want internal map id", definition.InternalMapID, instance.Worker.ZoneID())
		}
	}
	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		t.Fatalf("starter map instance: %v", err)
	}
	if gameServer.runtime.Worker != starter.Worker {
		t.Fatal("runtime compatibility worker is not the starter map worker")
	}
}
func TestEnsurePlayerSessionPreservesExistingActiveMap(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "router-preserve@example.com", "Router Preserve")

	gameServer.runtime.mu.Lock()
	want, err := gameServer.runtime.mapRouter.SetActiveLocationFromSpawn(resolved.PlayerID, "map_1_2", "west_gate")
	gameServer.runtime.mu.Unlock()
	if err != nil {
		t.Fatalf("SetActiveLocationFromSpawn() error = %v, want nil", err)
	}

	login, err := gameServer.runtime.Auth.Login(context.Background(), auth.LoginInput{
		Email:    resolved.Email.String(),
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("Login() error = %v, want nil", err)
	}
	if err := gameServer.runtime.ensurePlayerSession(login.Session); err != nil {
		t.Fatalf("ensurePlayerSession(reconnect) error = %v, want nil", err)
	}

	gameServer.runtime.mu.Lock()
	got, err := gameServer.runtime.mapRouter.ActiveLocation(resolved.PlayerID)
	instance, _, instanceErr := gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	_, starterHasPlayer := gameServer.runtime.Worker.PlayerEntity(resolved.PlayerID)
	gameServer.runtime.mu.Unlock()
	if err != nil {
		t.Fatalf("ActiveLocation() error = %v, want nil", err)
	}
	if got != want {
		t.Fatalf("active location after reconnect = %+v, want preserved %+v", got, want)
	}
	if instanceErr != nil {
		t.Fatalf("active instance error = %v, want nil", instanceErr)
	}
	if instance.Definition.InternalMapID != "map_1_2" || instance.Worker.ZoneID() != world.ZoneID("map_1_2") {
		t.Fatalf("active instance = %+v zone %q, want map_1_2 worker", instance.Definition, instance.Worker.ZoneID())
	}
	if _, ok := instance.Worker.PlayerEntity(resolved.PlayerID); !ok {
		t.Fatalf("active map_1_2 worker missing player %q", resolved.PlayerID)
	}
	if starterHasPlayer {
		t.Fatalf("starter worker still has player %q after active map_1_2 reconnect", resolved.PlayerID)
	}
}
func TestSessionReconnectMovesMembershipAndAOICursorToActiveMap(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "runtime-membership@example.com", "Runtime Membership")

	starterEvents, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("starter bootstrap events: %v", err)
	}
	_ = decodeWorldSnapshotForTest(t, starterEvents)

	gameServer.runtime.mu.Lock()
	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("starter instance: %v", err)
	}
	if starter.ActiveSessions[resolved.SessionID] != resolved.PlayerID {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("starter active sessions = %+v, want session attached", starter.ActiveSessions)
	}
	if _, ok := starter.LastAOI[resolved.SessionID]; !ok {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("starter LastAOI missing session %q", resolved.SessionID)
	}
	if _, err := gameServer.runtime.mapRouter.SetActiveLocationFromSpawn(resolved.PlayerID, "map_1_2", "west_gate"); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("SetActiveLocationFromSpawn() error = %v, want nil", err)
	}
	gameServer.runtime.mu.Unlock()

	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("ensurePlayerSession(map_1_2) error = %v, want nil", err)
	}
	mapTwoEvents, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("map_1_2 bootstrap events: %v", err)
	}
	mapTwoSnapshot := decodeWorldSnapshotForTest(t, mapTwoEvents)
	if mapTwoSnapshot.Map.PublicMapKey != "1-2" {
		t.Fatalf("reconnect snapshot map = %+v, want 1-2", mapTwoSnapshot.Map)
	}

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	mapTwo, err := gameServer.runtime.mapInstanceLocked("map_1_2")
	if err != nil {
		t.Fatalf("map_1_2 instance: %v", err)
	}
	if _, ok := starter.ActiveSessions[resolved.SessionID]; ok {
		t.Fatalf("starter active sessions still contains %q: %+v", resolved.SessionID, starter.ActiveSessions)
	}
	if _, ok := starter.LastAOI[resolved.SessionID]; ok {
		t.Fatalf("starter LastAOI still contains %q", resolved.SessionID)
	}
	if _, ok := starter.Worker.PlayerEntity(resolved.PlayerID); ok {
		t.Fatalf("starter worker still has player %q after active map switch", resolved.PlayerID)
	}
	if mapTwo.ActiveSessions[resolved.SessionID] != resolved.PlayerID {
		t.Fatalf("map_1_2 active sessions = %+v, want session attached", mapTwo.ActiveSessions)
	}
	if _, ok := mapTwo.LastAOI[resolved.SessionID]; !ok {
		t.Fatalf("map_1_2 LastAOI missing session %q", resolved.SessionID)
	}
	if gameServer.runtime.sessionLocations[resolved.SessionID] != "map_1_2" {
		t.Fatalf("session location = %q, want map_1_2", gameServer.runtime.sessionLocations[resolved.SessionID])
	}
}
func TestReconnectMovesAllPlayerSessionsToActiveMap(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "runtime-multisession@example.com", "Runtime Multi")
	secondLogin, err := gameServer.runtime.Auth.Login(context.Background(), auth.LoginInput{
		Email:    "runtime-multisession@example.com",
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("second login error = %v, want nil", err)
	}
	if err := gameServer.runtime.ensurePlayerSession(secondLogin.Session); err != nil {
		t.Fatalf("ensure second session: %v", err)
	}
	if _, err := gameServer.runtime.bootstrapEvents(resolved); err != nil {
		t.Fatalf("bootstrap first session: %v", err)
	}
	if _, err := gameServer.runtime.bootstrapEvents(secondLogin.Session); err != nil {
		t.Fatalf("bootstrap second session: %v", err)
	}

	gameServer.runtime.mu.Lock()
	if _, err := gameServer.runtime.mapRouter.SetActiveLocationFromSpawn(resolved.PlayerID, "map_1_2", "west_gate"); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("SetActiveLocationFromSpawn() error = %v, want nil", err)
	}
	gameServer.runtime.mu.Unlock()
	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("ensure first session after map switch: %v", err)
	}

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	starter, _ := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	mapTwo, _ := gameServer.runtime.mapInstanceLocked("map_1_2")
	for _, sessionID := range []auth.SessionID{resolved.SessionID, secondLogin.Session.SessionID} {
		if _, ok := starter.ActiveSessions[sessionID]; ok {
			t.Fatalf("starter active sessions still contains %q: %+v", sessionID, starter.ActiveSessions)
		}
		if _, ok := starter.LastAOI[sessionID]; ok {
			t.Fatalf("starter LastAOI still contains %q", sessionID)
		}
		if mapTwo.ActiveSessions[sessionID] != resolved.PlayerID {
			t.Fatalf("map_1_2 active session %q = %q, want %q", sessionID, mapTwo.ActiveSessions[sessionID], resolved.PlayerID)
		}
		if gameServer.runtime.sessionLocations[sessionID] != "map_1_2" {
			t.Fatalf("session %q location = %q, want map_1_2", sessionID, gameServer.runtime.sessionLocations[sessionID])
		}
	}
	if _, ok := starter.Worker.PlayerEntity(resolved.PlayerID); ok {
		t.Fatalf("starter worker still has player %q after multi-session map switch", resolved.PlayerID)
	}
	if _, ok := mapTwo.Worker.PlayerEntity(resolved.PlayerID); !ok {
		t.Fatalf("map_1_2 worker missing player %q after multi-session map switch", resolved.PlayerID)
	}
}
func TestActiveMapSnapshotUsesActiveMapWorker(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "router-snapshot@example.com", "Router Snapshot")

	gameServer.runtime.mu.Lock()
	if _, err := gameServer.runtime.mapRouter.SetActiveLocationFromSpawn(resolved.PlayerID, "map_1_2", "west_gate"); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("SetActiveLocationFromSpawn() error = %v, want nil", err)
	}
	gameServer.runtime.mu.Unlock()
	login, err := gameServer.runtime.Auth.Login(context.Background(), auth.LoginInput{
		Email:    resolved.Email.String(),
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("Login() error = %v, want nil", err)
	}
	if err := gameServer.runtime.ensurePlayerSession(login.Session); err != nil {
		t.Fatalf("ensurePlayerSession(map_1_2) error = %v, want nil", err)
	}

	events, err := gameServer.runtime.bootstrapEvents(login.Session)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	snapshot := decodeWorldSnapshotForTest(t, events)
	if snapshot.Map.PublicMapKey != "1-2" || snapshot.Sector.SectorKey != "1-2" {
		t.Fatalf("snapshot map/sector = %+v/%+v, want active public 1-2", snapshot.Map, snapshot.Sector)
	}
	selfCount := 0
	for _, entity := range snapshot.Entities {
		if hasStatusFlag(entity.StatusFlags, "self") {
			selfCount++
			if entity.Position != (world.Vec2{X: 400, Y: 5000}) {
				t.Fatalf("self position = %+v, want map_1_2 west gate spawn", entity.Position)
			}
		}
	}
	if selfCount != 1 {
		t.Fatalf("map_1_2 snapshot self count = %d in %+v, want 1", selfCount, snapshot.Entities)
	}
}
func TestWorldSnapshotUsesActiveMapEntitiesAndStoresInstanceAOI(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "snapshot-active-map@example.com", "Snapshot Active", "map_1_2", "west_gate")

	gameServer.runtime.mu.Lock()
	insertTestWorldEntityInMapLocked(t, gameServer, worldmaps.StarterMapID, "entity_snapshot_starter_only", world.EntityTypeNPC, world.Vec2{X: 410, Y: 5000}, false)
	insertTestWorldEntityInMapLocked(t, gameServer, "map_1_2", "entity_snapshot_map_two", world.EntityTypeNPC, world.Vec2{X: 410, Y: 5000}, false)
	gameServer.runtime.mu.Unlock()

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-active-map-snapshot","op":"world.snapshot","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("world snapshot response error = %+v, want success", response.Error)
	}
	var snapshot worldSnapshotPayload
	if err := json.Unmarshal(response.Response.Payload, &snapshot); err != nil {
		t.Fatalf("decode world snapshot: %v", err)
	}
	if snapshot.Map.PublicMapKey != "1-2" {
		t.Fatalf("snapshot map = %+v, want 1-2", snapshot.Map)
	}
	if !hasEntityID(snapshot.Entities, "entity_snapshot_map_two") {
		t.Fatalf("snapshot entities = %+v, missing map_1_2 entity", snapshot.Entities)
	}
	if hasEntityID(snapshot.Entities, "entity_snapshot_starter_only") {
		t.Fatalf("snapshot leaked starter map entity: %+v", snapshot.Entities)
	}

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	starter, _ := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	mapTwo, _ := gameServer.runtime.mapInstanceLocked("map_1_2")
	if _, ok := starter.LastAOI[resolved.SessionID]; ok {
		t.Fatalf("starter LastAOI contains active map_1_2 session")
	}
	if _, ok := mapTwo.LastAOI[resolved.SessionID]; !ok {
		t.Fatalf("map_1_2 LastAOI missing active session")
	}
}

func TestWorldSnapshotStoresImmutableAOICursorProjectionCopy(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "snapshot-immutable@example.com", "Snapshot Immutable")

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	payload, err := gameServer.runtime.worldSnapshotForSessionLocked(resolved.PlayerID, resolved.SessionID)
	if err != nil {
		t.Fatalf("worldSnapshotForSessionLocked() error = %v, want nil", err)
	}
	instance, _, err := gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if err != nil {
		t.Fatalf("activeMapInstanceLocked() error = %v, want nil", err)
	}
	instance.LastAOI[resolved.SessionID] = aoi.Snapshot{Entities: cloneAOIEntities(payload.Entities)}

	playerEntityID := gameServer.runtime.players[resolved.PlayerID].EntityID.String()
	before, ok := entityPayloadByID(instance.LastAOI[resolved.SessionID].Entities, playerEntityID)
	if !ok {
		t.Fatalf("LastAOI missing player entity %q", playerEntityID)
	}
	payloadIndex := -1
	for index := range payload.Entities {
		if payload.Entities[index].ID.String() == playerEntityID {
			payloadIndex = index
			break
		}
	}
	if payloadIndex == -1 {
		t.Fatalf("world snapshot payload missing player entity %q", playerEntityID)
	}
	if len(payload.Entities[payloadIndex].StatusFlags) == 0 || payload.Entities[payloadIndex].Display == nil {
		t.Fatalf("world snapshot payload entity missing cloneable public fields: %+v", payload.Entities[payloadIndex])
	}

	payload.Entities[payloadIndex].Position = world.Vec2{X: 900, Y: 900}
	payload.Entities[payloadIndex].StatusFlags[0] = "torn_payload"
	payload.Entities[payloadIndex].Display.Label = "torn payload"

	after, ok := entityPayloadByID(instance.LastAOI[resolved.SessionID].Entities, playerEntityID)
	if !ok {
		t.Fatalf("LastAOI missing player entity %q after payload mutation", playerEntityID)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("LastAOI entity mutated through world snapshot payload: before=%+v after=%+v", before, after)
	}
}

func TestTickLoopEmitsAOIOnlyToSessionsAttachedToSameMap(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	starterPlayer := createResolvedRuntimeSession(t, gameServer, "tick-map-one@example.com", "Tick One")
	mapTwoPlayer := createResolvedRuntimeSessionOnMap(t, gameServer, "tick-map-two@example.com", "Tick Two", "map_1_2", "west_gate")
	moveTestPlayerEntity(gameServer, starterPlayer.PlayerID, world.Vec2{X: 100, Y: 100})
	moveTestPlayerEntity(gameServer, mapTwoPlayer.PlayerID, world.Vec2{X: 100, Y: 100})

	if _, err := gameServer.runtime.bootstrapEvents(starterPlayer); err != nil {
		t.Fatalf("starter bootstrap events: %v", err)
	}
	if _, err := gameServer.runtime.bootstrapEvents(mapTwoPlayer); err != nil {
		t.Fatalf("map_1_2 bootstrap events: %v", err)
	}

	gameServer.runtime.mu.Lock()
	insertTestWorldEntityInMapLocked(t, gameServer, worldmaps.StarterMapID, "entity_tick_map_one", world.EntityTypeNPC, world.Vec2{X: 120, Y: 100}, false)
	insertTestWorldEntityInMapLocked(t, gameServer, "map_1_2", "entity_tick_map_two", world.EntityTypeNPC, world.Vec2{X: 120, Y: 100}, false)
	gameServer.runtime.mu.Unlock()

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	assertEventsContainEntityOnly(t, eventsBySession[starterPlayer.SessionID], "entity_tick_map_one", "entity_tick_map_two")
	assertEventsContainEntityOnly(t, eventsBySession[mapTwoPlayer.SessionID], "entity_tick_map_two", "entity_tick_map_one")
}

func TestRuntimeTickCollectionReachesOtherMapWhileRuntimeMutexHeld(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	mapTwoTicked := make(chan struct{})

	gameServer.runtime.mu.Lock()
	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("starter instance: %v", err)
	}
	mapTwo, err := gameServer.runtime.mapInstanceLocked("map_1_2")
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("map_1_2 instance: %v", err)
	}
	starter.Worker = newRuntimeTickProbeWorker(t, starter.Definition, nil)
	mapTwo.Worker = newRuntimeTickProbeWorker(t, mapTwo.Definition, mapTwoTicked)
	gameServer.runtime.Worker = starter.Worker
	gameServer.runtime.mu.Unlock()

	runtimeMuHeld := make(chan struct{})
	releaseRuntimeMu := make(chan struct{})
	holderDone := make(chan struct{})
	go func() {
		gameServer.runtime.mu.Lock()
		close(runtimeMuHeld)
		<-releaseRuntimeMu
		gameServer.runtime.mu.Unlock()
		close(holderDone)
	}()
	<-runtimeMuHeld

	tickDone := make(chan map[auth.SessionID][]realtime.EventEnvelope, 1)
	go func() {
		tickDone <- gameServer.runtime.tickAndCollectAOIEvents()
	}()

	select {
	case <-mapTwoTicked:
	case <-tickDone:
		t.Fatal("tickAndCollectAOIEvents completed while Runtime.mu was held; want worker tick collection before Runtime-owned AOI phase")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("map_1_2 worker tick waited for Runtime.mu")
	}

	close(releaseRuntimeMu)
	select {
	case <-tickDone:
	case <-time.After(time.Second):
		t.Fatal("tickAndCollectAOIEvents did not finish after Runtime.mu release")
	}
	<-holderDone
}

func TestCommandOnMapADoesNotBlockCommandOnMapBTiming(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	mapAPlayer := createResolvedRuntimeSession(t, gameServer, "command-map-a@example.com", "Command Map A")
	mapBPlayer := createResolvedRuntimeSessionOnMap(t, gameServer, "command-map-b@example.com", "Command Map B", "map_1_2", "west_gate")

	blockingMailbox := newBlockingRuntimeCommandMailbox()
	gameServer.runtime.mu.Lock()
	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("starter instance: %v", err)
	}
	mapAEntity, ok := starter.Worker.PlayerEntity(mapAPlayer.PlayerID)
	if !ok {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("starter worker missing player %q", mapAPlayer.PlayerID)
	}
	blockingWorker := newRuntimeBlockingCommandWorker(t, starter.Definition, blockingMailbox)
	gameServer.runtime.mu.Unlock()

	if err := blockingWorker.Submit(worker.SpawnPlayerCommand{
		PlayerID:  mapAPlayer.PlayerID,
		EntityID:  mapAEntity.ID,
		Position:  mapAEntity.Position,
		Speed:     defaultPlayerSpeed,
		SessionID: realtime.SessionID(mapAPlayer.SessionID.String()),
	}); err != nil {
		t.Fatalf("Submit(spawn map A player) error = %v, want nil", err)
	}
	if result := blockingWorker.Tick(); len(result.CommandErrors) != 0 {
		t.Fatalf("seed blocking worker command errors = %+v, want none", result.CommandErrors)
	}

	blockingMailbox.EnableBlock()
	gameServer.runtime.mu.Lock()
	starter, err = gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("starter instance after seed: %v", err)
	}
	starter.Worker = blockingWorker
	gameServer.runtime.Worker = blockingWorker
	gameServer.runtime.mu.Unlock()

	mapADone := make(chan realtime.CachedResponse, 1)
	go func() {
		mapADone <- gameServer.runtime.Gateway.HandleRequest(
			realtime.SessionID(mapAPlayer.SessionID.String()),
			[]byte(`{"request_id":"request-map-a-blocking-stop","op":"stop","payload":{},"client_seq":1,"v":1}`),
		)
	}()

	select {
	case <-blockingMailbox.DrainEntered():
	case <-time.After(time.Second):
		t.Fatal("map A command did not reach blocking worker tick")
	}

	mapBDone := make(chan realtime.CachedResponse, 1)
	go func() {
		mapBDone <- gameServer.runtime.Gateway.HandleRequest(
			realtime.SessionID(mapBPlayer.SessionID.String()),
			[]byte(`{"request_id":"request-map-b-independent-stop","op":"stop","payload":{},"client_seq":1,"v":1}`),
		)
	}()

	select {
	case response := <-mapBDone:
		if response.HasError {
			t.Fatalf("map B stop response error = %+v, want success", response.Error)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("map B command waited behind blocked map A worker command")
	}

	blockingMailbox.Release()
	select {
	case response := <-mapADone:
		if response.HasError {
			t.Fatalf("map A stop response error = %+v, want success", response.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("map A command did not finish after blocking worker release")
	}
}

type runtimeTickProbeMailbox struct {
	ticked chan struct{}
	once   sync.Once
}

func (mailbox *runtimeTickProbeMailbox) Submit(command worker.Command) error {
	return nil
}

func (mailbox *runtimeTickProbeMailbox) Drain() []worker.Command {
	if mailbox.ticked != nil {
		mailbox.once.Do(func() {
			close(mailbox.ticked)
		})
	}
	return nil
}

func newRuntimeTickProbeWorker(t *testing.T, definition worldmaps.MapDefinition, ticked chan struct{}) *worker.Worker {
	t.Helper()
	zoneWorker, err := worker.NewWorker(worker.Config{
		WorldID:   definition.WorldID,
		ZoneID:    definition.ZoneID,
		TickDelta: 50 * time.Millisecond,
		Mailbox:   &runtimeTickProbeMailbox{ticked: ticked},
	})
	if err != nil {
		t.Fatalf("new tick probe worker for %q: %v", definition.InternalMapID, err)
	}
	return zoneWorker
}

type blockingRuntimeCommandMailbox struct {
	mu           sync.Mutex
	commands     []worker.Command
	block        bool
	drainEntered chan struct{}
	release      chan struct{}
	enterOnce    sync.Once
	releaseOnce  sync.Once
}

func newBlockingRuntimeCommandMailbox() *blockingRuntimeCommandMailbox {
	return &blockingRuntimeCommandMailbox{
		commands:     make([]worker.Command, 0),
		drainEntered: make(chan struct{}),
		release:      make(chan struct{}),
	}
}

func (mailbox *blockingRuntimeCommandMailbox) Submit(command worker.Command) error {
	if command == nil {
		return worker.ErrNilCommand
	}
	mailbox.mu.Lock()
	defer mailbox.mu.Unlock()
	mailbox.commands = append(mailbox.commands, command)
	return nil
}

func (mailbox *blockingRuntimeCommandMailbox) Drain() []worker.Command {
	mailbox.mu.Lock()
	commands := append([]worker.Command(nil), mailbox.commands...)
	clear(mailbox.commands)
	mailbox.commands = mailbox.commands[:0]
	shouldBlock := mailbox.block
	mailbox.mu.Unlock()

	if shouldBlock {
		mailbox.enterOnce.Do(func() {
			close(mailbox.drainEntered)
		})
		<-mailbox.release
	}
	return commands
}

func (mailbox *blockingRuntimeCommandMailbox) EnableBlock() {
	mailbox.mu.Lock()
	defer mailbox.mu.Unlock()
	mailbox.block = true
}

func (mailbox *blockingRuntimeCommandMailbox) DrainEntered() <-chan struct{} {
	return mailbox.drainEntered
}

func (mailbox *blockingRuntimeCommandMailbox) Release() {
	mailbox.releaseOnce.Do(func() {
		close(mailbox.release)
	})
}

func newRuntimeBlockingCommandWorker(t *testing.T, definition worldmaps.MapDefinition, mailbox worker.Mailbox) *worker.Worker {
	t.Helper()
	zoneWorker, err := worker.NewWorker(worker.Config{
		WorldID:   definition.WorldID,
		ZoneID:    definition.ZoneID,
		TickDelta: 50 * time.Millisecond,
		Mailbox:   mailbox,
	})
	if err != nil {
		t.Fatalf("new blocking command worker for %q: %v", definition.InternalMapID, err)
	}
	return zoneWorker
}

func TestMoveToAndStopMutateOnlyActiveMapWorker(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "move-active-map@example.com", "Move Active", "map_1_2", "west_gate")

	move := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-active-map-move","op":"move_to","payload":{"target":{"x":450,"y":5000}},"client_seq":1,"v":1}`),
	)
	if move.HasError {
		t.Fatalf("move response error = %+v, want success", move.Error)
	}

	gameServer.runtime.mu.Lock()
	starter, _ := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	mapTwo, _ := gameServer.runtime.mapInstanceLocked("map_1_2")
	_, starterHasPlayer := starter.Worker.PlayerEntity(resolved.PlayerID)
	entity, mapTwoHasPlayer := mapTwo.Worker.PlayerEntity(resolved.PlayerID)
	gameServer.runtime.mu.Unlock()
	if starterHasPlayer {
		t.Fatalf("starter worker has active map_1_2 player %q", resolved.PlayerID)
	}
	if !mapTwoHasPlayer || !entity.Movement.Moving || entity.Movement.Target != (world.Vec2{X: 450, Y: 5000}) {
		t.Fatalf("map_1_2 movement entity = %+v ok=%v, want moving to 450,5000", entity, mapTwoHasPlayer)
	}

	outOfBounds := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-active-map-move-oob","op":"move_to","payload":{"target":{"x":10001,"y":5000}},"client_seq":2,"v":1}`),
	)
	if !outOfBounds.HasError || outOfBounds.Error.Error.Code != foundation.CodeOutOfRange {
		t.Fatalf("out-of-bounds move response = %+v, want out of range", outOfBounds)
	}
	gameServer.runtime.mu.Lock()
	afterOutOfBounds, afterOutOfBoundsOK := mapTwo.Worker.PlayerEntity(resolved.PlayerID)
	gameServer.runtime.mu.Unlock()
	if !afterOutOfBoundsOK ||
		!afterOutOfBounds.Movement.Moving ||
		afterOutOfBounds.Movement.Target != (world.Vec2{X: 450, Y: 5000}) ||
		afterOutOfBounds.Position != entity.Position {
		t.Fatalf("after out-of-bounds move entity = %+v ok=%v, want unchanged active target/state", afterOutOfBounds, afterOutOfBoundsOK)
	}

	stop := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-active-map-stop","op":"stop","payload":{},"client_seq":3,"v":1}`),
	)
	if stop.HasError {
		t.Fatalf("stop response error = %+v, want success", stop.Error)
	}
	gameServer.runtime.mu.Lock()
	stopped, ok := mapTwo.Worker.PlayerEntity(resolved.PlayerID)
	gameServer.runtime.mu.Unlock()
	if !ok || stopped.Movement.Moving {
		t.Fatalf("stopped entity = %+v ok=%v, want active map movement stopped", stopped, ok)
	}
}
func TestSafeHangarClassificationUsesMapSafeZoneDefinition(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "safe-zone-map@example.com", "Safe Zone", "map_1_2", "west_gate")

	gameServer.runtime.mu.Lock()
	if !gameServer.runtime.playerInSafeHangarAreaLocked(resolved.PlayerID) {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("player at map_1_2 west gate spawn is not in map-defined hangar safe zone")
	}
	instance, _, err := gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("active instance: %v", err)
	}
	entity, ok := instance.Worker.PlayerEntity(resolved.PlayerID)
	if !ok {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("active map missing player")
	}
	entity.Position = world.Vec2{X: 0, Y: 0}
	entity.Movement = world.MovementState{}
	if err := instance.Worker.UpdateEntity(entity); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("UpdateEntity(origin) error = %v, want nil", err)
	}
	if gameServer.runtime.playerInSafeHangarAreaLocked(resolved.PlayerID) {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("map_1_2 origin classified safe; want only map-defined west gate safe zone")
	}
	gameServer.runtime.mu.Unlock()
}
func TestSectorPayloadFromMapUsesProjectionKey(t *testing.T) {
	sector := sectorPayloadFromMap(worldmaps.ClientMapProjection{
		MapKey:       "1-2",
		PublicMapKey: "1-2",
		DisplayName:  "Outer Ring",
		Region:       "Origin Belt",
		RiskBand:     "low",
		PVPPolicy:    "pve",
	})
	if sector.SectorKey != "1-2" || sector.Name != "Outer Ring" || sector.Contested {
		t.Fatalf("sector from map = %+v, want 1-2 projection", sector)
	}
	fallback := sectorPayloadFromMap(worldmaps.ClientMapProjection{})
	if fallback.SectorKey != runtimeSectorKey {
		t.Fatalf("empty map sector key = %q, want fallback %q", fallback.SectorKey, runtimeSectorKey)
	}
}
