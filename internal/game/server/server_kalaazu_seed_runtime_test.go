package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/contentseed"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestRuntimeDefaultPublishedSeedSpawnsVisibleKalaazuMapThreeNPCs(t *testing.T) {
	bundle := runtimeBundleFromDefaultPublishedSeed(t)
	runtime, err := NewRuntime(RuntimeConfig{
		WorldID:           foundation.WorldID("world-1"),
		ContentRepository: &fakeRuntimeRepository{bundle: bundle},
		Passwords:         auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("NewRuntime(default published seed) error = %v, want nil", err)
	}

	result, err := runtime.Auth.Register(context.Background(), auth.RegisterInput{
		Email:    "kalaazu-map-three-runtime@example.com",
		Password: "correct-password",
		Callsign: "Kalaazu Runtime",
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	if err := runtime.ensurePlayerSession(result.Session); err != nil {
		t.Fatalf("ensurePlayerSession() error = %v, want nil", err)
	}
	runtime.mu.Lock()
	starterState := runtime.players[result.Session.PlayerID]
	runtime.mu.Unlock()
	if starterState.Ship.ActiveShipID != "starter" || starterState.Ship.Hull != 4000 {
		t.Fatalf("starter ship = %+v, want starter contract with Kalaazu Phoenix hull 4000", starterState.Ship)
	}

	runtime.mu.Lock()
	instance, err := runtime.mapInstanceLocked(worldmaps.MapID("map_1_3"))
	if err != nil {
		runtime.mu.Unlock()
		t.Fatalf("map_1_3 instance: %v", err)
	}
	if len(instance.Definition.SpawnPoints) == 0 {
		runtime.mu.Unlock()
		t.Fatal("map_1_3 spawn points empty")
	}
	location, err := runtime.mapRouter.SetActiveLocationFromSpawn(result.Session.PlayerID, worldmaps.MapID("map_1_3"), instance.Definition.SpawnPoints[0].SpawnID)
	if err != nil {
		runtime.mu.Unlock()
		t.Fatalf("SetActiveLocationFromSpawn(map_1_3) error = %v, want nil", err)
	}
	runtime.mu.Unlock()
	if err := runtime.ensurePlayerSession(result.Session); err != nil {
		t.Fatalf("ensurePlayerSession(map_1_3) error = %v, want nil", err)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	instance, err = runtime.mapInstanceLocked(worldmaps.MapID("map_1_3"))
	if err != nil {
		t.Fatalf("map_1_3 instance: %v", err)
	}
	spawn := instance.Worker.EnemySpawnSnapshot()
	if len(instance.Definition.EnemyPools) != 6 {
		t.Fatalf("map_1_3 enemy pools = %d, want six Kalaazu NPC pools", len(instance.Definition.EnemyPools))
	}
	if got, want := spawn.MapAliveCount, 24; got != want {
		t.Fatalf("map_1_3 live NPC count = %d, want %d from six pools x four initial alive; snapshot=%+v", got, want, spawn)
	}

	npcCenter := world.Vec2{X: 5200, Y: 4096}
	entity, ok := instance.Worker.PlayerEntity(result.Session.PlayerID)
	if !ok {
		t.Fatal("map_1_3 player entity missing")
	}
	entity.Position = npcCenter
	entity.Movement = world.MovementState{}
	if err := instance.Worker.UpdateEntity(entity); err != nil {
		t.Fatalf("UpdateEntity(player at NPC center) error = %v, want nil", err)
	}
	state := runtime.players[result.Session.PlayerID]
	state.Stats.RadarRange = 420
	runtime.players[result.Session.PlayerID] = state

	snapshot, err := runtime.worldSnapshotForSessionLocked(result.Session.PlayerID, result.Session.SessionID)
	if err != nil {
		t.Fatalf("worldSnapshotForSessionLocked() error = %v, want nil", err)
	}
	if snapshot.Map.PublicMapKey != "1-3" || location.InternalMapID != "map_1_3" {
		t.Fatalf("map projection = %q/%q, want 1-3/map_1_3", snapshot.Map.PublicMapKey, location.InternalMapID)
	}
	if got := countSnapshotEntitiesOfType(snapshot.Entities, "npc"); got == 0 {
		t.Fatalf("map_1_3 AOI entities = %+v, want visible Kalaazu NPC near %+v", snapshot.Entities, npcCenter)
	}
}

func TestRuntimeDefaultPublishedSeedKillsKalaazuNPCAndPicksUpLoot(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		Clock:             clock,
		ContentRepository: &fakeRuntimeRepository{bundle: runtimeBundleFromDefaultPublishedSeed(t)},
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New(default published seed) error = %v, want nil", err)
	}
	resolved := createResolvedRuntimeSession(t, gameServer, "kalaazu-kill-loot-runtime@example.com", "Kalaazu Loot")
	targetID := firstLiveNPCEntityIDForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, targetID, world.Vec2{X: 80})

	startPayload := gatewayJSON(t, gameServer, resolved, "kalaazu-default-start-attack", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": targetID.String(),
	}, 1)
	var start struct {
		Accepted bool `json:"accepted"`
	}
	if err := json.Unmarshal(startPayload, &start); err != nil {
		t.Fatalf("decode start attack payload: %v", err)
	}
	if !start.Accepted {
		t.Fatalf("combat.start_attack payload = %+v, want accepted", start)
	}
	postEvents, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post combat.start_attack events: %v", err)
	}
	events := append([]realtime.EventEnvelope(nil), postEvents...)

	var drop kalaazuLootDropForTest
	for tick := 0; tick < 80 && drop.DropID == ""; tick++ {
		tickEvents := gameServer.runtime.tickAndCollectAOIEvents()[resolved.SessionID]
		events = append(events, tickEvents...)
		drop = firstKalaazuLootDropForTest(t, tickEvents)
		if drop.DropID != "" {
			break
		}
		clock.Advance(1250 * time.Millisecond)
	}
	if drop.DropID == "" {
		t.Fatalf("default seed combat events missing loot.created after 80 ticks; seen=%v", eventTypeSetForTest(events))
	}
	for _, want := range []realtime.ClientEventType{
		realtime.EventCombatNPCKilled,
		realtime.EventLootCreated,
		realtime.EventCombatAttackStopped,
	} {
		if !eventTypesContain(events, want) {
			t.Fatalf("default seed combat events = %v, missing %s", eventTypeSetForTest(events), want)
		}
	}

	pickupPayload := gatewayJSON(t, gameServer, resolved, "kalaazu-default-loot-pickup", realtime.OperationLootPickup, map[string]any{
		"drop_id": drop.DropID,
	}, 2)
	var pickup struct {
		Accepted bool `json:"accepted"`
		Cargo    struct {
			Items []struct {
				ItemID   string `json:"item_id"`
				Quantity int64  `json:"quantity"`
			} `json:"items"`
		} `json:"cargo"`
	}
	if err := json.Unmarshal(pickupPayload, &pickup); err != nil {
		t.Fatalf("decode pickup payload: %v", err)
	}
	if !pickup.Accepted || !cargoItemsContainForTest(pickup.Cargo.Items, drop.ItemID, drop.Quantity) {
		t.Fatalf("pickup payload = %+v, want accepted cargo with %s x%d", pickup, drop.ItemID, drop.Quantity)
	}
	pickupEvents, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationLootPickup, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post loot.pickup events: %v", err)
	}
	for _, want := range []realtime.ClientEventType{
		realtime.EventLootPickedUp,
		realtime.EventLootRemoved,
		realtime.EventCargoSnapshot,
		realtime.EventInventorySnapshot,
	} {
		if !eventTypesContain(pickupEvents, want) {
			t.Fatalf("default seed pickup events = %v, missing %s", eventTypeSetForTest(pickupEvents), want)
		}
	}
}

func runtimeBundleFromDefaultPublishedSeed(t *testing.T) gamecontent.GameplayContent {
	t.Helper()
	snapshot, err := contentseed.BuildDefaultSnapshot(foundation.WorldID("world-1"))
	if err != nil {
		t.Fatalf("BuildDefaultSnapshot() error = %v, want nil", err)
	}
	bundle, err := contentdb.MapSnapshotContent(snapshot, foundation.WorldID("world-1"))
	if err != nil {
		t.Fatalf("MapSnapshotContent(default seed) error = %v, want nil", err)
	}
	return bundle
}

func countSnapshotEntitiesOfType(entities []aoi.EntityPayload, entityType world.EntityType) int {
	count := 0
	for _, entity := range entities {
		if entity.Type == entityType {
			count++
		}
	}
	return count
}

type kalaazuLootDropForTest struct {
	DropID   string `json:"drop_id"`
	ItemID   string `json:"item_id"`
	Quantity int64  `json:"quantity"`
}

func firstKalaazuLootDropForTest(t *testing.T, events []realtime.EventEnvelope) kalaazuLootDropForTest {
	t.Helper()
	for _, event := range events {
		if event.Type != realtime.EventLootCreated {
			continue
		}
		var drop kalaazuLootDropForTest
		if err := json.Unmarshal(event.Payload, &drop); err != nil {
			t.Fatalf("decode loot.created payload: %v", err)
		}
		if drop.DropID == "" || drop.ItemID == "" || drop.Quantity <= 0 {
			t.Fatalf("loot.created payload = %+v, want drop/item/positive quantity", drop)
		}
		return drop
	}
	return kalaazuLootDropForTest{}
}

func cargoItemsContainForTest(items []struct {
	ItemID   string `json:"item_id"`
	Quantity int64  `json:"quantity"`
}, itemID string, quantity int64) bool {
	for _, item := range items {
		if item.ItemID == itemID && item.Quantity >= quantity {
			return true
		}
	}
	return false
}

func eventTypeSetForTest(events []realtime.EventEnvelope) []realtime.ClientEventType {
	seen := make(map[realtime.ClientEventType]struct{}, len(events))
	for _, event := range events {
		seen[event.Type] = struct{}{}
	}
	types := make([]realtime.ClientEventType, 0, len(seen))
	for eventType := range seen {
		types = append(types, eventType)
	}
	return types
}
