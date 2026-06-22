package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/visibility"
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
	actor, ok := gameServer.runtime.Combat.Actor("entity_training_npc")
	if !ok {
		t.Fatal("starter combat actor missing entity_training_npc after seed")
	}
	wantSignature := visibility.SignatureForEntityType(world.EntityTypeNPC)
	if actor.Type != world.EntityTypeNPC ||
		actor.NPCType != trainingNPCType ||
		actor.HP != 30 ||
		actor.Stats.Stats.Core.HPMax != 30 ||
		actor.Shield != 0 ||
		actor.Stats.Stats.Core.ShieldMax != 0 ||
		actor.Energy != 1 ||
		actor.Stats.Stats.Core.EnergyMax != 1 ||
		actor.Stats.Stats.Combat.WeaponRange != 1 ||
		actor.Stats.Stats.Combat.Accuracy != 1 ||
		actor.Signature != wantSignature ||
		actor.Stats.Stats.Exploration.SignatureRadius != wantSignature.Units() {
		t.Fatalf("starter combat actor = %+v, want starter template projection with signature %v", actor, wantSignature)
	}

	mapTwo, err := gameServer.runtime.mapInstanceLocked("map_1_2")
	if err != nil {
		t.Fatalf("map_1_2 instance: %v", err)
	}
	mapTwoSnapshot := mapTwo.Worker.EnemySpawnSnapshot()
	if len(mapTwoSnapshot.Records) != 1 || mapTwoSnapshot.MapAliveCount != 1 {
		t.Fatalf("map_1_2 spawn snapshot = %+v, want one outer ring scout", mapTwoSnapshot)
	}
	mapTwoRecord := mapTwoSnapshot.Records[0]
	if mapTwoRecord.EnemyPoolID != "outer_ring_scout_drone_pool" ||
		mapTwoRecord.SpawnAreaID != "outer_ring_scout_drone_area" ||
		mapTwoRecord.NPCType != "outer_ring_scout_drone" ||
		mapTwoRecord.Level != 1 ||
		mapTwoRecord.StatTemplateID != "outer_ring_scout_drone_level_1" ||
		mapTwoRecord.DropProfileID != "outer_ring_scout_drone_salvage" ||
		mapTwoRecord.AggroProfileID != "outer_ring_scout_drone_cautious" ||
		mapTwoRecord.LeashProfileID != "outer_ring_scout_drone_patrol" ||
		!mapTwoRecord.Alive {
		t.Fatalf("map_1_2 spawn record = %+v, want outer ring scout pool row", mapTwoRecord)
	}
	if mapTwoRecord.Position != (world.Vec2{X: 1800, Y: 5400}) {
		t.Fatalf("map_1_2 spawn position = %+v, want deterministic center", mapTwoRecord.Position)
	}
	if npcCount := countWorkerEntitiesOfType(mapTwo.Worker.Snapshot(), world.EntityTypeNPC); npcCount != 1 {
		t.Fatalf("map_1_2 NPC count = %d, want one", npcCount)
	}
	if signalCount := countWorkerEntitiesOfType(mapTwo.Worker.Snapshot(), world.EntityTypePlanetSignal); signalCount != 1 {
		t.Fatalf("map_1_2 planet signal count = %d, want hidden planet signal seeding unchanged", signalCount)
	}
	mapTwoEntity, ok := mapTwo.Worker.Entity(mapTwoRecord.EntityID)
	if !ok {
		t.Fatalf("map_1_2 worker missing spawned NPC %q", mapTwoRecord.EntityID)
	}
	if mapTwoEntity.Type != world.EntityTypeNPC || mapTwoEntity.Position != mapTwoRecord.Position {
		t.Fatalf("map_1_2 entity = %+v, want spawner-created NPC at %+v", mapTwoEntity, mapTwoRecord.Position)
	}
	mapTwoActor, ok := gameServer.runtime.Combat.Actor(mapTwoRecord.EntityID)
	if !ok {
		t.Fatalf("map_1_2 combat actor missing %q after seed", mapTwoRecord.EntityID)
	}
	if mapTwoActor.Type != world.EntityTypeNPC ||
		mapTwoActor.NPCType != "outer_ring_scout_drone" ||
		mapTwoActor.WorldID != mapTwo.Definition.WorldID ||
		mapTwoActor.ZoneID != mapTwo.Definition.ZoneID ||
		mapTwoActor.HP != 36 ||
		mapTwoActor.Stats.Stats.Core.HPMax != 36 ||
		mapTwoActor.Shield != 4 ||
		mapTwoActor.Stats.Stats.Core.ShieldMax != 4 ||
		mapTwoActor.Energy != 2 ||
		mapTwoActor.Stats.Stats.Core.EnergyMax != 2 ||
		mapTwoActor.Stats.Stats.Combat.WeaponRange != 1 ||
		mapTwoActor.Stats.Stats.Combat.Accuracy != 1 ||
		mapTwoActor.Signature != wantSignature ||
		mapTwoActor.Stats.Stats.Exploration.SignatureRadius != wantSignature.Units() {
		t.Fatalf("map_1_2 combat actor = %+v, want outer ring scout template projection", mapTwoActor)
	}
}

func TestNPCActorProjectionRefreshesTemplateBackedStats(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		t.Fatalf("starter map instance: %v", err)
	}
	if len(starter.Definition.NPCStatTemplates) == 0 {
		t.Fatal("starter map missing NPC stat templates")
	}
	template := starter.Definition.NPCStatTemplates[0]
	template.HPMax = 44
	template.WeaponRange = 8
	template.RadarSignature = 123
	starter.Definition.NPCStatTemplates[0] = template
	entity, ok := starter.Worker.Entity("entity_training_npc")
	if !ok {
		t.Fatal("starter worker missing entity_training_npc")
	}

	gameServer.runtime.Combat = combat.NewService(gameServer.runtime.clock, nil)
	actor, err := gameServer.runtime.upsertNPCCombatActorProjectionLocked(starter, entity)
	if err != nil {
		t.Fatalf("upsertNPCCombatActorProjectionLocked() error = %v, want nil", err)
	}
	if actor.HP != 44 ||
		actor.Stats.Stats.Core.HPMax != 44 ||
		actor.Stats.Stats.Combat.WeaponRange != 8 ||
		actor.Signature != visibility.EntitySignature(123) ||
		actor.Stats.Stats.Exploration.SignatureRadius != 123 {
		t.Fatalf("actor projection = %+v, want updated HP/range/signature from template", actor)
	}
}

func TestNPCActorProjectionPreservesMutableCombatStateAcrossResync(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "npc-preserve@example.com", "NPC Preserve")

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		t.Fatalf("starter map instance: %v", err)
	}
	entity, ok := starter.Worker.Entity("entity_training_npc")
	if !ok {
		t.Fatal("starter worker missing entity_training_npc")
	}
	actor, ok := gameServer.runtime.Combat.Actor(entity.ID)
	if !ok {
		t.Fatal("starter combat actor missing entity_training_npc")
	}
	readyAt := gameServer.runtime.clock.Now().Add(time.Minute)
	contributor := foundation.PlayerID("player_projection_contributor")
	actor.HP = 9
	actor.Energy = 0.25
	actor.Cooldowns = combat.CooldownState{combat.BasicLaserCooldownKey: readyAt}
	actor.Contributions = map[foundation.PlayerID]float64{contributor: 7}
	if err := gameServer.runtime.Combat.UpsertActor(actor); err != nil {
		t.Fatalf("UpsertActor(mutated npc) error = %v, want nil", err)
	}

	newPosition := world.Vec2{X: entity.Position.X + 12, Y: entity.Position.Y + 6}
	entity.Position = newPosition
	if err := starter.Worker.UpdateEntity(entity); err != nil {
		t.Fatalf("UpdateEntity(entity_training_npc) error = %v, want nil", err)
	}
	starter.HiddenEntities[entity.ID] = true

	if err := gameServer.runtime.syncWorldCombatActorLocked(resolved.PlayerID, entity.ID); err != nil {
		t.Fatalf("syncWorldCombatActorLocked(npc) error = %v, want nil", err)
	}
	refreshed, ok := gameServer.runtime.Combat.Actor(entity.ID)
	if !ok {
		t.Fatal("refreshed combat actor missing entity_training_npc")
	}
	if refreshed.HP != 9 ||
		refreshed.Energy != 0.25 ||
		!refreshed.Cooldowns[combat.BasicLaserCooldownKey].Equal(readyAt) ||
		refreshed.Contributions[contributor] != 7 {
		t.Fatalf("refreshed actor mutable state = %+v, want HP/energy/cooldown/contribution preserved", refreshed)
	}
	if refreshed.Position != newPosition || !refreshed.Hidden || refreshed.StealthScore <= 0 {
		t.Fatalf("refreshed actor projection = %+v, want position %v and hidden state refreshed", refreshed, newPosition)
	}
}

func TestBootstrapProjectionDoesNotLeakEnemyPoolOrDropProfileInternals(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "spawner-leak@example.com", "Spawner Leak")
	mapTwoResolved := createResolvedRuntimeSessionOnMap(t, gameServer, "spawner-leak-map-two@example.com", "Spawner Leak Two", "map_1_2", "west_gate")
	gameServer.runtime.mu.Lock()
	mapTwo, err := gameServer.runtime.mapInstanceLocked("map_1_2")
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("map_1_2 instance: %v", err)
	}
	mapTwoSpawnSnapshot := mapTwo.Worker.EnemySpawnSnapshot()
	if len(mapTwoSpawnSnapshot.Records) != 1 {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("map_1_2 spawn snapshot = %+v, want one destination NPC", mapTwoSpawnSnapshot)
	}
	mapTwoNPCEntityID := mapTwoSpawnSnapshot.Records[0].EntityID
	gameServer.runtime.mu.Unlock()
	moveTestPlayerNearEntity(t, gameServer, mapTwoResolved.PlayerID, mapTwoNPCEntityID, world.Vec2{})

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	mapTwoEvents, err := gameServer.runtime.bootstrapEvents(mapTwoResolved)
	if err != nil {
		t.Fatalf("map_1_2 bootstrap events: %v", err)
	}
	mapTwoSnapshot := decodeWorldSnapshotForTest(t, mapTwoEvents)
	mapTwoNPCVisible := false
	for _, entity := range mapTwoSnapshot.Entities {
		if entity.ID == mapTwoNPCEntityID {
			mapTwoNPCVisible = true
			break
		}
	}
	if !mapTwoNPCVisible {
		t.Fatalf("map_1_2 bootstrap entities = %+v, missing visible destination NPC %q", mapTwoSnapshot.Entities, mapTwoNPCEntityID)
	}
	rawBytes, err := json.Marshal(append(events, mapTwoEvents...))
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
		"map_1_2",
		"outer_ring_scout_drone_pool",
		"outer_ring_scout_drone_area",
		"outer_ring_scout_drone_level_1",
		"outer_ring_scout_drone_salvage",
		"outer_ring_scout_drone_cautious",
		"outer_ring_scout_drone_patrol",
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
