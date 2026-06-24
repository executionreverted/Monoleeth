package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
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
		actor.HP != 34 ||
		actor.Stats.Stats.Core.HPMax != 34 ||
		actor.Shield != 4 ||
		actor.Stats.Stats.Core.ShieldMax != 4 ||
		actor.Energy != 6 ||
		actor.Stats.Stats.Core.EnergyMax != 6 ||
		actor.Stats.Stats.Combat.WeaponRange != 120 ||
		actor.Stats.Stats.Combat.Accuracy != 0.7 ||
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
		mapTwoActor.HP != 44 ||
		mapTwoActor.Stats.Stats.Core.HPMax != 44 ||
		mapTwoActor.Shield != 8 ||
		mapTwoActor.Stats.Stats.Core.ShieldMax != 8 ||
		mapTwoActor.Energy != 4 ||
		mapTwoActor.Stats.Stats.Core.EnergyMax != 4 ||
		mapTwoActor.Stats.Stats.Combat.WeaponRange != 140 ||
		mapTwoActor.Stats.Stats.Combat.Accuracy != 0.72 ||
		mapTwoActor.Signature != wantSignature ||
		mapTwoActor.Stats.Stats.Exploration.SignatureRadius != wantSignature.Units() {
		t.Fatalf("map_1_2 combat actor = %+v, want outer ring scout template projection", mapTwoActor)
	}

	mapThree, err := gameServer.runtime.mapInstanceLocked("map_1_3")
	if err != nil {
		t.Fatalf("map_1_3 instance: %v", err)
	}
	mapThreeSnapshot := mapThree.Worker.EnemySpawnSnapshot()
	if len(mapThreeSnapshot.Records) != 1 || mapThreeSnapshot.MapAliveCount != 1 {
		t.Fatalf("map_1_3 spawn snapshot = %+v, want one border raider", mapThreeSnapshot)
	}
	mapThreeRecord := mapThreeSnapshot.Records[0]
	if mapThreeRecord.EnemyPoolID != "border_raider_drone_pool" ||
		mapThreeRecord.SpawnAreaID != "border_raider_drone_area" ||
		mapThreeRecord.NPCType != "border_raider_drone" ||
		mapThreeRecord.Level != 2 ||
		mapThreeRecord.StatTemplateID != "border_raider_drone_level_2" ||
		mapThreeRecord.DropProfileID != "border_raider_drone_salvage" ||
		mapThreeRecord.AggroProfileID != "border_raider_drone_hunter" ||
		mapThreeRecord.LeashProfileID != "border_raider_drone_patrol" ||
		!mapThreeRecord.Alive {
		t.Fatalf("map_1_3 spawn record = %+v, want border raider pool row", mapThreeRecord)
	}
	if mapThreeRecord.Position != (world.Vec2{X: 5400, Y: 5200}) {
		t.Fatalf("map_1_3 spawn position = %+v, want deterministic center", mapThreeRecord.Position)
	}
	if _, ok := starter.Worker.Entity(mapThreeRecord.EntityID); ok {
		t.Fatalf("starter worker contains map_1_3 NPC %q", mapThreeRecord.EntityID)
	}
	if _, ok := mapTwo.Worker.Entity(mapThreeRecord.EntityID); ok {
		t.Fatalf("map_1_2 worker contains map_1_3 NPC %q", mapThreeRecord.EntityID)
	}
	if npcCount := countWorkerEntitiesOfType(mapThree.Worker.Snapshot(), world.EntityTypeNPC); npcCount != 1 {
		t.Fatalf("map_1_3 NPC count = %d, want one", npcCount)
	}
	mapThreeEntity, ok := mapThree.Worker.Entity(mapThreeRecord.EntityID)
	if !ok {
		t.Fatalf("map_1_3 worker missing spawned NPC %q", mapThreeRecord.EntityID)
	}
	if mapThreeEntity.Type != world.EntityTypeNPC || mapThreeEntity.Position != mapThreeRecord.Position {
		t.Fatalf("map_1_3 entity = %+v, want spawner-created NPC at %+v", mapThreeEntity, mapThreeRecord.Position)
	}
	mapThreeActor, ok := gameServer.runtime.Combat.Actor(mapThreeRecord.EntityID)
	if !ok {
		t.Fatalf("map_1_3 combat actor missing %q after seed", mapThreeRecord.EntityID)
	}
	if mapThreeActor.Type != world.EntityTypeNPC ||
		mapThreeActor.NPCType != "border_raider_drone" ||
		mapThreeActor.WorldID != mapThree.Definition.WorldID ||
		mapThreeActor.ZoneID != mapThree.Definition.ZoneID ||
		mapThreeActor.HP != 72 ||
		mapThreeActor.Stats.Stats.Core.HPMax != 72 ||
		mapThreeActor.Shield != 22 ||
		mapThreeActor.Stats.Stats.Core.ShieldMax != 22 ||
		mapThreeActor.Energy != 8 ||
		mapThreeActor.Stats.Stats.Core.EnergyMax != 8 ||
		mapThreeActor.Stats.Stats.Combat.WeaponRange != 180 ||
		mapThreeActor.Stats.Stats.Combat.Accuracy != 0.82 ||
		mapThreeActor.Signature != wantSignature ||
		mapThreeActor.Stats.Stats.Exploration.SignatureRadius != wantSignature.Units() {
		t.Fatalf("map_1_3 combat actor = %+v, want border raider template projection", mapThreeActor)
	}
}

func TestRuntimeMapTwoEnemyLifecycleRespawnsThroughMapInstance(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		Clock:             clock,
		ContentRepository: staticContentRepositoryForTest(),
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		t.Fatalf("starter map instance: %v", err)
	}
	mapTwo, err := gameServer.runtime.mapInstanceLocked("map_1_2")
	if err != nil {
		t.Fatalf("map_1_2 instance: %v", err)
	}
	if len(mapTwo.Definition.EnemyPools) != 1 {
		t.Fatalf("map_1_2 enemy pools = %+v, want one deterministic destination pool", mapTwo.Definition.EnemyPools)
	}
	pool := mapTwo.Definition.EnemyPools[0]
	initial := mapTwo.Worker.EnemySpawnSnapshot()
	if len(initial.Records) != 1 ||
		initial.MapAliveCount != 1 ||
		initial.PoolAliveCounts[pool.EnemyPoolID] != 1 {
		t.Fatalf("initial map_1_2 spawn snapshot = %+v, want one live row at cap-safe counts", initial)
	}
	record := initial.Records[0]
	if record.EnemyPoolID != pool.EnemyPoolID ||
		record.EntityID == "entity_training_npc" ||
		record.NPCType != "outer_ring_scout_drone" ||
		!record.Alive {
		t.Fatalf("initial map_1_2 spawn record = %+v, want destination scout row isolated from starter seed", record)
	}
	if _, ok := starter.Worker.Entity(record.EntityID); ok {
		t.Fatalf("starter worker contains destination NPC %q", record.EntityID)
	}
	if _, ok := mapTwo.Worker.Entity("entity_training_npc"); ok {
		t.Fatal("map_1_2 worker contains starter training NPC")
	}
	if actor, ok := gameServer.runtime.Combat.Actor(record.EntityID); !ok ||
		actor.Type != world.EntityTypeNPC ||
		actor.NPCType != record.NPCType ||
		actor.HP != 44 ||
		actor.Shield != 8 {
		t.Fatalf("initial map_1_2 combat actor = %+v ok=%v, want live destination projection", actor, ok)
	}

	killedAt := clock.Now()
	deadActor, ok := gameServer.runtime.Combat.Actor(record.EntityID)
	if !ok {
		t.Fatalf("combat actor %q missing before death mark", record.EntityID)
	}
	deadActor.HP = 0
	deadActor.Shield = 0
	deadActor.Dead = true
	deadActor.DiedAt = &killedAt
	if err := gameServer.runtime.Combat.UpsertActor(deadActor); err != nil {
		t.Fatalf("UpsertActor(dead destination NPC) error = %v, want nil", err)
	}
	if err := gameServer.runtime.submitWorkerCommandAndRecordMetricsLocked(mapTwo, worker.MarkEnemyKilledCommand{
		Definition:  mapTwo.Definition,
		NPCEntityID: record.EntityID,
		KilledAt:    killedAt,
	}); err != nil {
		t.Fatalf("MarkEnemyKilledCommand(map_1_2) error = %v, want nil", err)
	}
	mapTwo.HiddenEntities[record.EntityID] = true

	dead, ok := mapTwo.Worker.EnemySpawnRecord(record.EntityID)
	if !ok {
		t.Fatalf("map_1_2 spawn row %q missing after death mark", record.EntityID)
	}
	if dead.Alive ||
		!dead.DeadAt.Equal(killedAt) ||
		!dead.NextRespawnAt.Equal(killedAt.Add(pool.KillRespawnDelay)) ||
		dead.EntityID != record.EntityID {
		t.Fatalf("dead map_1_2 record = %+v, want same row dead until KillRespawnDelay", dead)
	}
	deadSnapshot := mapTwo.Worker.EnemySpawnSnapshot()
	if len(deadSnapshot.Records) != 1 ||
		deadSnapshot.MapAliveCount != 0 ||
		deadSnapshot.PoolAliveCounts[pool.EnemyPoolID] != 0 {
		t.Fatalf("dead map_1_2 snapshot = %+v, want one pending row and no live destination NPCs", deadSnapshot)
	}
	if _, ok := mapTwo.Worker.Entity(record.EntityID); ok {
		t.Fatalf("map_1_2 entity %q still exists after death mark", record.EntityID)
	}

	clock.Advance(pool.KillRespawnDelay - time.Nanosecond)
	if err := commandErrors(mapTwo.Worker.Tick()); err != nil {
		t.Fatalf("map_1_2 tick before respawn due error = %v, want nil", err)
	}
	beforeDue, ok := mapTwo.Worker.EnemySpawnRecord(record.EntityID)
	if !ok || beforeDue.Alive {
		t.Fatalf("record before respawn due = %+v ok=%v, want same dead row", beforeDue, ok)
	}

	respawnedAt := clock.Advance(time.Nanosecond)
	if err := commandErrors(mapTwo.Worker.Tick()); err != nil {
		t.Fatalf("map_1_2 tick at respawn due error = %v, want nil", err)
	}
	if err := gameServer.runtime.syncAliveNPCCombatActorProjectionsLocked(mapTwo); err != nil {
		t.Fatalf("syncAliveNPCCombatActorProjectionsLocked(map_1_2) error = %v, want nil", err)
	}

	respawned, ok := mapTwo.Worker.EnemySpawnRecord(record.EntityID)
	if !ok {
		t.Fatalf("map_1_2 spawn row %q missing after respawn", record.EntityID)
	}
	if !respawned.Alive ||
		respawned.EntityID != record.EntityID ||
		!respawned.SpawnedAt.Equal(respawnedAt) ||
		!respawned.DeadAt.IsZero() ||
		!respawned.NextRespawnAt.IsZero() {
		t.Fatalf("respawned map_1_2 record = %+v, want same row alive with cleared death timing", respawned)
	}
	if entity, ok := mapTwo.Worker.Entity(record.EntityID); !ok ||
		entity.Type != world.EntityTypeNPC ||
		entity.Position != respawned.Position {
		t.Fatalf("respawned map_1_2 entity = %+v ok=%v, want restored NPC at %+v", entity, ok, respawned.Position)
	}
	respawnedSnapshot := mapTwo.Worker.EnemySpawnSnapshot()
	if len(respawnedSnapshot.Records) < 1 ||
		respawnedSnapshot.MapAliveCount < 1 ||
		respawnedSnapshot.PoolAliveCounts[pool.EnemyPoolID] < 1 ||
		respawnedSnapshot.MapAliveCount > pool.MapMaxAlive ||
		respawnedSnapshot.PoolAliveCounts[pool.EnemyPoolID] > pool.PoolMaxAlive {
		t.Fatalf("respawned map_1_2 snapshot = %+v, want stable cap-safe destination rows", respawnedSnapshot)
	}
	seenRespawnedID := false
	seenEntityIDs := map[world.EntityID]bool{}
	for _, row := range respawnedSnapshot.Records {
		if seenEntityIDs[row.EntityID] {
			t.Fatalf("respawned map_1_2 snapshot = %+v, duplicate entity id %q", respawnedSnapshot, row.EntityID)
		}
		seenEntityIDs[row.EntityID] = true
		if row.EntityID == record.EntityID {
			seenRespawnedID = true
		}
	}
	if !seenRespawnedID {
		t.Fatalf("respawned map_1_2 snapshot = %+v, missing reused entity id %q", respawnedSnapshot, record.EntityID)
	}
	restoredActor, ok := gameServer.runtime.Combat.Actor(record.EntityID)
	if !ok ||
		restoredActor.Dead ||
		restoredActor.DiedAt != nil ||
		restoredActor.HP != 44 ||
		restoredActor.Shield != 8 ||
		restoredActor.NPCType != record.NPCType ||
		restoredActor.Position != respawned.Position ||
		restoredActor.Hidden {
		t.Fatalf("restored map_1_2 combat actor = %+v ok=%v, want live respawn projection", restoredActor, ok)
	}
	if mapTwo.HiddenEntities[record.EntityID] {
		t.Fatalf("map_1_2 hidden state for %q still set after projection sync", record.EntityID)
	}
	starterSnapshot := starter.Worker.EnemySpawnSnapshot()
	if len(starterSnapshot.Records) != 1 ||
		starterSnapshot.MapAliveCount != 1 ||
		starterSnapshot.Records[0].EntityID != "entity_training_npc" {
		t.Fatalf("starter snapshot after map_1_2 respawn = %+v, want starter seed uncontaminated", starterSnapshot)
	}
	if _, ok := starter.Worker.Entity("entity_training_npc"); !ok {
		t.Fatal("starter training NPC missing after map_1_2 lifecycle")
	}
	if _, ok := starter.Worker.Entity(record.EntityID); ok {
		t.Fatalf("starter worker contains respawned destination NPC %q", record.EntityID)
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
