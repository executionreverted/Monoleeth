package worker

import (
	"testing"
	"time"

	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestEnemySpawnerInitialFillSpawnsStarterPoolNPC(t *testing.T) {
	catalog, err := worldmaps.StarterCatalog("world-1")
	if err != nil {
		t.Fatalf("StarterCatalog() error = %v, want nil", err)
	}
	definition, ok := catalog.Get(worldmaps.StarterMapID)
	if !ok {
		t.Fatal("starter map definition missing")
	}
	zoneWorker := newWorkerForMapDefinition(t, definition)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 1 {
		t.Fatalf("spawn records = %+v, want one starter NPC", snapshot.Records)
	}
	record := snapshot.Records[0]
	if record.EnemyPoolID != "starter_training_drone_pool" ||
		record.SpawnAreaID != "starter_training_drone_area" ||
		record.NPCType != "training_drone" ||
		record.Level != 1 ||
		record.StatTemplateID != "training_drone_level_1" ||
		record.DropProfileID != "training_drone_salvage" ||
		record.AggroProfileID != "training_drone_passive" ||
		record.LeashProfileID != "training_drone_stationary" ||
		!record.Alive ||
		record.SpawnedAt.IsZero() {
		t.Fatalf("spawn record = %+v, want starter pool/profile metadata", record)
	}
	area := definition.SpawnAreas[0]
	if record.Position.DistanceSquared(area.Center) > area.Radius*area.Radius {
		t.Fatalf("spawn position = %+v outside area %+v", record.Position, area)
	}
	entity, ok := zoneWorker.Entity(record.EntityID)
	if !ok {
		t.Fatalf("spawned entity %q missing from worker", record.EntityID)
	}
	if entity.Type != world.EntityTypeNPC || entity.Position != record.Position {
		t.Fatalf("spawned entity = %+v, want NPC at record position", entity)
	}
	speed, ok := zoneWorker.EntitySpeed(record.EntityID)
	if !ok || speed != definition.NPCStatTemplates[0].Speed {
		t.Fatalf("spawned speed = %v ok=%v, want stat template speed %v", speed, ok, definition.NPCStatTemplates[0].Speed)
	}
	if snapshot.PoolAliveCounts[record.EnemyPoolID] != 1 || snapshot.MapAliveCount != 1 {
		t.Fatalf("alive counts = pool %+v map %d, want one alive", snapshot.PoolAliveCounts, snapshot.MapAliveCount)
	}
}

func TestEnemySpawnerInitialFillRespectsPoolAndMapCaps(t *testing.T) {
	t.Run("pool cap", func(t *testing.T) {
		definition := testEnemyMapDefinition()
		definition.EnemyPools[0].InitialAlive = 3
		definition.EnemyPools[0].PoolMaxAlive = 2
		definition.EnemyPools[0].MapMaxAlive = 5
		zoneWorker := newWorkerForMapDefinition(t, definition)

		assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

		snapshot := zoneWorker.EnemySpawnSnapshot()
		if len(snapshot.Records) != 2 || snapshot.PoolAliveCounts["test_pool"] != 2 || snapshot.MapAliveCount != 2 {
			t.Fatalf("snapshot = %+v, want two rows capped by pool max", snapshot)
		}
	})

	t.Run("map cap", func(t *testing.T) {
		definition := testEnemyMapDefinition()
		definition.EnemyPools[0].InitialAlive = 3
		definition.EnemyPools[0].PoolMaxAlive = 3
		definition.EnemyPools[0].MapMaxAlive = 2
		zoneWorker := newWorkerForMapDefinition(t, definition)

		assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

		snapshot := zoneWorker.EnemySpawnSnapshot()
		if len(snapshot.Records) != 2 || snapshot.PoolAliveCounts["test_pool"] != 2 || snapshot.MapAliveCount != 2 {
			t.Fatalf("snapshot = %+v, want two rows capped by map max", snapshot)
		}
	})
}

func TestEnemySpawnerInitialFillUsesStrictestMapCapAcrossEnabledPools(t *testing.T) {
	tests := []struct {
		name      string
		highFirst bool
	}{
		{name: "lower cap first"},
		{name: "lower cap second", highFirst: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			definition := testEnemyMapDefinition()
			lowCapPool := definition.EnemyPools[0]
			lowCapPool.EnemyPoolID = "pool_low_map_cap"
			lowCapPool.InitialAlive = 3
			lowCapPool.PoolMaxAlive = 3
			lowCapPool.MapMaxAlive = 2
			highCapPool := definition.EnemyPools[0]
			highCapPool.EnemyPoolID = "pool_high_map_cap"
			highCapPool.InitialAlive = 3
			highCapPool.PoolMaxAlive = 3
			highCapPool.MapMaxAlive = 5
			definition.EnemyPools = []worldmaps.MapEnemyPoolDefinition{lowCapPool, highCapPool}
			if tc.highFirst {
				definition.EnemyPools = []worldmaps.MapEnemyPoolDefinition{highCapPool, lowCapPool}
			}
			zoneWorker := newWorkerForMapDefinition(t, definition)

			assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

			snapshot := zoneWorker.EnemySpawnSnapshot()
			if len(snapshot.Records) != 2 || snapshot.MapAliveCount != 2 {
				t.Fatalf("snapshot = %+v, want strictest map cap of two regardless of pool order", snapshot)
			}
		})
	}
}

func TestEnemySpawnerGeneratedEntityIDsAreDistinctForSanitizedPoolCollisions(t *testing.T) {
	definition := testEnemyMapDefinition()
	slashPool := definition.EnemyPools[0]
	slashPool.EnemyPoolID = "pool/a"
	slashPool.InitialAlive = 1
	slashPool.PoolMaxAlive = 1
	slashPool.MapMaxAlive = 2
	underscorePool := definition.EnemyPools[0]
	underscorePool.EnemyPoolID = "pool_a"
	underscorePool.InitialAlive = 1
	underscorePool.PoolMaxAlive = 1
	underscorePool.MapMaxAlive = 2
	definition.EnemyPools = []worldmaps.MapEnemyPoolDefinition{slashPool, underscorePool}
	zoneWorker := newWorkerForMapDefinition(t, definition)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 2 || snapshot.MapAliveCount != 2 {
		t.Fatalf("snapshot = %+v, want both sanitized-collision pools spawned", snapshot)
	}
	entityIDs := make(map[world.EntityID]struct{}, len(snapshot.Records))
	poolIDs := make(map[worldmaps.EnemyPoolID]struct{}, len(snapshot.Records))
	for _, record := range snapshot.Records {
		if _, exists := entityIDs[record.EntityID]; exists {
			t.Fatalf("duplicate entity id %q in snapshot %+v", record.EntityID, snapshot)
		}
		entityIDs[record.EntityID] = struct{}{}
		poolIDs[record.EnemyPoolID] = struct{}{}
		if _, ok := zoneWorker.Entity(record.EntityID); !ok {
			t.Fatalf("spawned entity %q missing from worker", record.EntityID)
		}
	}
	if _, ok := poolIDs["pool/a"]; !ok {
		t.Fatalf("snapshot = %+v, missing slash pool", snapshot)
	}
	if _, ok := poolIDs["pool_a"]; !ok {
		t.Fatalf("snapshot = %+v, missing underscore pool", snapshot)
	}
}

func TestEnemySpawnerDuplicateInitializationDoesNotDuplicateRowsOrEntities(t *testing.T) {
	definition := testEnemyMapDefinition()
	definition.EnemyPools[0].InitialAlive = 2
	definition.EnemyPools[0].PoolMaxAlive = 2
	definition.EnemyPools[0].MapMaxAlive = 2
	zoneWorker := newWorkerForMapDefinition(t, definition)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	first := zoneWorker.EnemySpawnSnapshot()
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	second := zoneWorker.EnemySpawnSnapshot()

	if len(first.Records) != 2 || len(second.Records) != 2 || second.MapAliveCount != first.MapAliveCount {
		t.Fatalf("first=%+v second=%+v, want duplicate command to preserve row count", first, second)
	}
	if len(zoneWorker.Snapshot().Entities) != 2 {
		t.Fatalf("worker entities = %+v, want exactly two spawned NPCs", zoneWorker.Snapshot().Entities)
	}
}

func TestEnemySpawnerSkipsDisabledPools(t *testing.T) {
	definition := testEnemyMapDefinition()
	disabled := definition.EnemyPools[0]
	disabled.EnemyPoolID = "disabled_pool"
	disabled.Enabled = false
	modeDisabled := definition.EnemyPools[0]
	modeDisabled.EnemyPoolID = "spawn_mode_disabled_pool"
	modeDisabled.SpawnMode = worldmaps.SpawnModeDisabled
	definition.EnemyPools = []worldmaps.MapEnemyPoolDefinition{disabled, modeDisabled}
	zoneWorker := newWorkerForMapDefinition(t, definition)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 || len(zoneWorker.Snapshot().Entities) != 0 {
		t.Fatalf("snapshot=%+v entities=%+v, want no NPCs from disabled pools", snapshot, zoneWorker.Snapshot().Entities)
	}
}

func TestEnemySpawnerSkipsForbiddenInitialCandidates(t *testing.T) {
	tests := []struct {
		name string
		edit func(*worldmaps.MapDefinition)
	}{
		{
			name: "pvp blocking safe zone",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.SpawnAreas[0].Center = world.Vec2{X: 100, Y: 100}
				definition.SpawnAreas[0].SafeZoneExcluded = true
				definition.SafeZones = []worldmaps.SafeZoneDefinition{{
					SafeZoneID: "safe_spawn_block",
					Center:     world.Vec2{X: 100, Y: 100},
					Radius:     50,
					BlocksPVP:  true,
				}}
			},
		},
		{
			name: "visible portal exclusion",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.SpawnAreas[0].Center = world.Vec2{X: 500, Y: 500}
				definition.SpawnAreas[0].PortalExclusionRadius = 200
				definition.Portals = []worldmaps.PortalDefinition{{
					PortalID:          "test_gate",
					SourceMapID:       definition.InternalMapID,
					SourcePosition:    world.Vec2{X: 500, Y: 500},
					InteractionRadius: 180,
					Visible:           true,
				}}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			definition := testEnemyMapDefinition()
			tc.edit(&definition)
			zoneWorker := newWorkerForMapDefinition(t, definition)

			assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

			snapshot := zoneWorker.EnemySpawnSnapshot()
			if len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 || len(zoneWorker.Snapshot().Entities) != 0 {
				t.Fatalf("snapshot=%+v entities=%+v, want forbidden candidate skipped without entity leak", snapshot, zoneWorker.Snapshot().Entities)
			}
		})
	}
}

func newWorkerForMapDefinition(t *testing.T, definition worldmaps.MapDefinition) *Worker {
	t.Helper()

	zoneWorker, err := NewWorker(Config{
		WorldID:   definition.WorldID,
		ZoneID:    definition.ZoneID,
		TickDelta: time.Second,
		Clock:     testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	return zoneWorker
}

func testEnemyMapDefinition() worldmaps.MapDefinition {
	return worldmaps.MapDefinition{
		InternalMapID:  "zone-1",
		PublicMapKey:   "test-map",
		WorldID:        "world-1",
		ZoneID:         "zone-1",
		DisplayName:    "Test Map",
		Region:         "Test",
		RiskBand:       "low",
		PVPPolicy:      "pve",
		VisualThemeKey: "test",
		Bounds:         worldmaps.ExactPlayableBounds(),
		SpawnAreas: []worldmaps.MapSpawnAreaDefinition{{
			SpawnAreaID:           "test_area",
			Shape:                 worldmaps.SpawnAreaShapeCircle,
			Center:                world.Vec2{X: 500, Y: 500},
			Radius:                100,
			SafeZoneExcluded:      true,
			PortalExclusionRadius: 0,
		}},
		EnemyPools: []worldmaps.MapEnemyPoolDefinition{{
			EnemyPoolID:      "test_pool",
			NPCType:          "test_drone",
			MinLevel:         2,
			MaxLevel:         2,
			SpawnAreaIDs:     []worldmaps.SpawnAreaID{"test_area"},
			MapMaxAlive:      3,
			PoolMaxAlive:     3,
			InitialAlive:     1,
			SpawnInterval:    30 * time.Second,
			KillRespawnDelay: 30 * time.Second,
			SpawnJitter:      0,
			SpawnMode:        worldmaps.SpawnModePeriodic,
			StatTemplateID:   "test_stat",
			DropProfileID:    "test_drop",
			AggroProfileID:   "test_aggro",
			LeashProfileID:   "test_leash",
			Enabled:          true,
		}},
		NPCStatTemplates: []worldmaps.NPCStatTemplate{{
			StatTemplateID: "test_stat",
			NPCType:        "test_drone",
			MinLevel:       2,
			MaxLevel:       2,
			LabelKey:       "npc.test_drone",
			HPMax:          10,
			WeaponRange:    1,
			WeaponCooldown: time.Second,
			Accuracy:       1,
			Speed:          42,
		}},
		NPCDropProfiles: []worldmaps.NPCDropProfile{{
			DropProfileID: "test_drop",
			NPCType:       "test_drone",
			MinLevel:      2,
			MaxLevel:      2,
			RiskBand:      "low",
			LootTableID:   "test_loot",
		}},
		NPCAggroProfiles: []worldmaps.NPCAggroProfile{{
			AggroProfileID:       "test_aggro",
			SafeZoneAttackPolicy: "never",
		}},
		NPCLeashProfiles: []worldmaps.NPCLeashProfile{{
			LeashProfileID: "test_leash",
			LeashDistance:  1,
			ResetOnBreak:   true,
		}},
	}
}
