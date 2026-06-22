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
