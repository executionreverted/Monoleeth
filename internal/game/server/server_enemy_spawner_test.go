package server

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

func TestRuntimeSeedWorldInitializesStarterEnemyPoolThroughSpawner(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		t.Fatalf("starter map instance: %v", err)
	}
	record, ok := starter.Worker.EnemySpawnRecord("entity_training_npc")
	if !ok {
		t.Fatalf("starter spawner missing entity_training_npc; snapshot=%+v", starter.Worker.EnemySpawnSnapshot())
	}
	if record.EnemyPoolID != "starter_training_drone_pool" ||
		record.SpawnAreaID != "starter_training_drone_area" ||
		record.NPCType != trainingNPCType ||
		!record.Alive {
		t.Fatalf("starter spawn record = %+v, want training drone pool row", record)
	}
	entity, ok := starter.Worker.Entity("entity_training_npc")
	if !ok {
		t.Fatal("starter worker missing entity_training_npc")
	}
	if entity.Type != world.EntityTypeNPC || entity.Position != record.Position {
		t.Fatalf("starter entity = %+v, want spawner-created NPC at %+v", entity, record.Position)
	}

	mapTwo, err := gameServer.runtime.mapInstanceLocked("map_1_2")
	if err != nil {
		t.Fatalf("map_1_2 instance: %v", err)
	}
	if snapshot := mapTwo.Worker.EnemySpawnSnapshot(); len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 {
		t.Fatalf("map_1_2 spawn snapshot = %+v, want no default NPCs without enemy pools", snapshot)
	}
	if npcCount := countWorkerEntitiesOfType(mapTwo.Worker.Snapshot(), world.EntityTypeNPC); npcCount != 0 {
		t.Fatalf("map_1_2 NPC count = %d, want none", npcCount)
	}
	if signalCount := countWorkerEntitiesOfType(mapTwo.Worker.Snapshot(), world.EntityTypePlanetSignal); signalCount != 1 {
		t.Fatalf("map_1_2 planet signal count = %d, want hidden planet signal seeding unchanged", signalCount)
	}
}

func TestBootstrapProjectionDoesNotLeakEnemyPoolOrDropProfileInternals(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "spawner-leak@example.com", "Spawner Leak")

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	rawBytes, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal bootstrap events: %v", err)
	}
	raw := string(rawBytes)
	for _, forbidden := range []string{
		"starter_training_drone_pool",
		"starter_training_drone_area",
		"training_drone_salvage",
		"drop_profile",
		"stat_template",
		"aggro_profile",
		"leash_profile",
		"loot_table",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("bootstrap projection leaked %q in %s", forbidden, raw)
		}
	}
}

func countWorkerEntitiesOfType(snapshot worker.Snapshot, entityType world.EntityType) int {
	count := 0
	for _, entity := range snapshot.Entities {
		if entity.Type == entityType {
			count++
		}
	}
	return count
}
