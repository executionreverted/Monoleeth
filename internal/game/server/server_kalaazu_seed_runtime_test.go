package server

import (
	"context"
	"testing"

	"gameproject/internal/game/auth"
	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/contentseed"
	"gameproject/internal/game/foundation"
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
