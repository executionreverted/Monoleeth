package worker

import (
	"errors"
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

func TestEnemySpawnerMarkKilledUpdatesDeathAccountingAndRemovesEntity(t *testing.T) {
	definition := testEnemyMapDefinition()
	zoneWorker := newWorkerForMapDefinition(t, definition)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	record := zoneWorker.EnemySpawnSnapshot().Records[0]

	entity, ok := zoneWorker.Entity(record.EntityID)
	if !ok {
		t.Fatalf("spawned entity %q missing before kill", record.EntityID)
	}
	entity.Position = world.Vec2{X: 575, Y: 525}
	if err := zoneWorker.UpdateEntity(entity); err != nil {
		t.Fatalf("UpdateEntity(%q) error = %v, want nil", record.EntityID, err)
	}
	killedAt := time.Date(2026, 6, 17, 12, 3, 0, 0, time.UTC)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    killedAt,
	}))

	got, ok := zoneWorker.EnemySpawnRecord(record.EntityID)
	if !ok {
		t.Fatalf("EnemySpawnRecord(%q) missing after kill", record.EntityID)
	}
	if got.Alive {
		t.Fatalf("record = %+v, want dead", got)
	}
	if !got.DeadAt.Equal(killedAt) {
		t.Fatalf("DeadAt = %s, want %s", got.DeadAt, killedAt)
	}
	wantRespawn := killedAt.Add(definition.EnemyPools[0].KillRespawnDelay)
	if !got.NextRespawnAt.Equal(wantRespawn) {
		t.Fatalf("NextRespawnAt = %s, want %s", got.NextRespawnAt, wantRespawn)
	}
	if got.Position != entity.Position {
		t.Fatalf("record position = %+v, want current entity position %+v", got.Position, entity.Position)
	}
	snapshot := zoneWorker.EnemySpawnSnapshot()
	if snapshot.PoolAliveCounts[record.EnemyPoolID] != 0 || snapshot.MapAliveCount != 0 {
		t.Fatalf("alive counts = pool %+v map %d, want zero", snapshot.PoolAliveCounts, snapshot.MapAliveCount)
	}
	if _, ok := zoneWorker.Entity(record.EntityID); ok {
		t.Fatalf("Entity(%q) still present after kill", record.EntityID)
	}
}

func TestEnemySpawnerMarkKilledDuplicateIsIdempotent(t *testing.T) {
	definition := testEnemyMapDefinition()
	zoneWorker := newWorkerForMapDefinition(t, definition)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	record := zoneWorker.EnemySpawnSnapshot().Records[0]
	firstKilledAt := time.Date(2026, 6, 17, 12, 4, 0, 0, time.UTC)
	secondKilledAt := firstKilledAt.Add(5 * time.Minute)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    firstKilledAt,
	}))
	first, ok := zoneWorker.EnemySpawnRecord(record.EntityID)
	if !ok {
		t.Fatalf("EnemySpawnRecord(%q) missing after first kill", record.EntityID)
	}
	mustInsertWorkerEntity(t, zoneWorker, record.EntityID, world.EntityTypeNPC, world.Vec2{X: 650, Y: 650})

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    secondKilledAt,
	}))

	duplicate, ok := zoneWorker.EnemySpawnRecord(record.EntityID)
	if !ok {
		t.Fatalf("EnemySpawnRecord(%q) missing after duplicate kill", record.EntityID)
	}
	if duplicate.Alive {
		t.Fatalf("record = %+v, want still dead after duplicate kill", duplicate)
	}
	if !duplicate.DeadAt.Equal(first.DeadAt) || !duplicate.NextRespawnAt.Equal(first.NextRespawnAt) {
		t.Fatalf("duplicate timing = dead %s respawn %s, want original dead %s respawn %s",
			duplicate.DeadAt, duplicate.NextRespawnAt, first.DeadAt, first.NextRespawnAt)
	}
	if duplicate.Position != first.Position {
		t.Fatalf("duplicate position = %+v, want original death position %+v", duplicate.Position, first.Position)
	}
	snapshot := zoneWorker.EnemySpawnSnapshot()
	if snapshot.PoolAliveCounts[record.EnemyPoolID] != 0 || snapshot.MapAliveCount != 0 {
		t.Fatalf("alive counts after duplicate = pool %+v map %d, want zero", snapshot.PoolAliveCounts, snapshot.MapAliveCount)
	}
	if _, ok := zoneWorker.Entity(record.EntityID); ok {
		t.Fatalf("leftover Entity(%q) still present after duplicate kill", record.EntityID)
	}
}

func TestEnemySpawnerRespawnWaitsForDelayAndReusesDeadRow(t *testing.T) {
	definition := testEnemyMapDefinition()
	definition.EnemyPools[0].PoolMaxAlive = 1
	definition.EnemyPools[0].MapMaxAlive = 1
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	record := zoneWorker.EnemySpawnSnapshot().Records[0]
	killedAt := clock.Now()

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    killedAt,
	}))
	clock.Advance(definition.EnemyPools[0].KillRespawnDelay - time.Nanosecond)
	assertNoCommandErrors(t, zoneWorker.Tick())
	beforeDue, ok := zoneWorker.EnemySpawnRecord(record.EntityID)
	if !ok {
		t.Fatalf("EnemySpawnRecord(%q) missing before respawn", record.EntityID)
	}
	if beforeDue.Alive || beforeDue.DeadAt.IsZero() || beforeDue.NextRespawnAt.IsZero() {
		t.Fatalf("record before due = %+v, want still dead with respawn timing", beforeDue)
	}
	if _, ok := zoneWorker.Entity(record.EntityID); ok {
		t.Fatalf("Entity(%q) exists before respawn delay elapsed", record.EntityID)
	}

	respawnedAt := clock.Advance(time.Nanosecond)
	assertNoCommandErrors(t, zoneWorker.Tick())
	respawned, ok := zoneWorker.EnemySpawnRecord(record.EntityID)
	if !ok {
		t.Fatalf("EnemySpawnRecord(%q) missing after respawn", record.EntityID)
	}
	if !respawned.Alive ||
		!respawned.SpawnedAt.Equal(respawnedAt) ||
		!respawned.DeadAt.IsZero() ||
		!respawned.NextRespawnAt.IsZero() ||
		respawned.EntityID != record.EntityID ||
		respawned.Position != definition.SpawnAreas[0].Center {
		t.Fatalf("respawned record = %+v, want same row alive at spawn center with cleared death timing", respawned)
	}
	entity, ok := zoneWorker.Entity(record.EntityID)
	if !ok || entity.Type != world.EntityTypeNPC || entity.Position != respawned.Position {
		t.Fatalf("respawned entity = %+v ok=%v, want NPC at %+v", entity, ok, respawned.Position)
	}
	if speed, ok := zoneWorker.EntitySpeed(record.EntityID); !ok || speed != definition.NPCStatTemplates[0].Speed {
		t.Fatalf("respawned speed = %v ok=%v, want %v", speed, ok, definition.NPCStatTemplates[0].Speed)
	}
	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 1 || snapshot.MapAliveCount != 1 || snapshot.PoolAliveCounts[record.EnemyPoolID] != 1 {
		t.Fatalf("snapshot after respawn = %+v, want one alive reused row", snapshot)
	}
}

func TestEnemySpawnerPeriodicFillReservesPendingRespawnCapacity(t *testing.T) {
	definition := testEnemyMapDefinition()
	definition.EnemyPools[0].InitialAlive = 1
	definition.EnemyPools[0].PoolMaxAlive = 1
	definition.EnemyPools[0].MapMaxAlive = 1
	definition.EnemyPools[0].SpawnInterval = time.Second
	definition.EnemyPools[0].KillRespawnDelay = 5 * time.Second
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	record := zoneWorker.EnemySpawnSnapshot().Records[0]
	killedAt := clock.Now()
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    killedAt,
	}))

	clock.Advance(definition.EnemyPools[0].SpawnInterval)
	assertNoCommandErrors(t, zoneWorker.Tick())
	pending := zoneWorker.EnemySpawnSnapshot()
	if len(pending.Records) != 1 || pending.MapAliveCount != 0 || pending.PoolAliveCounts[record.EnemyPoolID] != 0 {
		t.Fatalf("snapshot after periodic interval = %+v, want one pending dead row and no periodic replacement", pending)
	}
	if pending.Records[0].EntityID != record.EntityID || pending.Records[0].Alive {
		t.Fatalf("record after periodic interval = %+v, want original entity still pending respawn", pending.Records[0])
	}
	if _, ok := zoneWorker.Entity(record.EntityID); ok {
		t.Fatalf("Entity(%q) exists before kill respawn delay elapsed", record.EntityID)
	}

	respawnedAt := clock.Advance(definition.EnemyPools[0].KillRespawnDelay - definition.EnemyPools[0].SpawnInterval)
	assertNoCommandErrors(t, zoneWorker.Tick())
	respawned := zoneWorker.EnemySpawnSnapshot()
	if len(respawned.Records) != 1 || respawned.MapAliveCount != 1 || respawned.PoolAliveCounts[record.EnemyPoolID] != 1 {
		t.Fatalf("snapshot after kill delay = %+v, want same row respawned at cap", respawned)
	}
	if respawned.Records[0].EntityID != record.EntityID ||
		!respawned.Records[0].Alive ||
		!respawned.Records[0].SpawnedAt.Equal(respawnedAt) ||
		!respawned.Records[0].DeadAt.IsZero() ||
		!respawned.Records[0].NextRespawnAt.IsZero() {
		t.Fatalf("record after kill delay = %+v, want original entity id respawned", respawned.Records[0])
	}
	if _, ok := zoneWorker.Entity(record.EntityID); !ok {
		t.Fatalf("Entity(%q) missing after kill respawn delay elapsed", record.EntityID)
	}
}

func TestEnemySpawnerDuplicateDeathThenRespawnDoesNotDuplicateOrDriftCaps(t *testing.T) {
	definition := testEnemyMapDefinition()
	definition.EnemyPools[0].PoolMaxAlive = 1
	definition.EnemyPools[0].MapMaxAlive = 1
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	record := zoneWorker.EnemySpawnSnapshot().Records[0]
	killedAt := clock.Now()

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    killedAt,
	}))
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    killedAt.Add(time.Second),
	}))
	clock.Advance(definition.EnemyPools[0].KillRespawnDelay)
	assertNoCommandErrors(t, zoneWorker.Tick())

	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 1 || snapshot.MapAliveCount != 1 || snapshot.PoolAliveCounts[record.EnemyPoolID] != 1 {
		t.Fatalf("snapshot after duplicate death respawn = %+v, want exactly one alive row", snapshot)
	}
	if len(zoneWorker.Snapshot().Entities) != 1 || !hasWorkerSnapshotEntityID(zoneWorker.Snapshot(), record.EntityID) {
		t.Fatalf("worker entities after duplicate death respawn = %+v, want only %q", zoneWorker.Snapshot().Entities, record.EntityID)
	}
	respawned, ok := zoneWorker.EnemySpawnRecord(record.EntityID)
	if !ok || !respawned.Alive || !respawned.DeadAt.IsZero() || !respawned.NextRespawnAt.IsZero() {
		t.Fatalf("record after duplicate death respawn = %+v ok=%v, want alive with cleared death timing", respawned, ok)
	}
}

func TestEnemySpawnerMarkKilledUnknownOrNonSpawnerEntityReturnsUnknown(t *testing.T) {
	definition := testEnemyMapDefinition()

	t.Run("unknown entity", func(t *testing.T) {
		zoneWorker := newWorkerForMapDefinition(t, definition)

		result := tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
			Definition:  definition,
			NPCEntityID: "entity_missing",
			KilledAt:    time.Date(2026, 6, 17, 12, 5, 0, 0, time.UTC),
		})
		if len(result.CommandErrors) != 1 || !errors.Is(result.CommandErrors[0].Err, ErrUnknownEntity) {
			t.Fatalf("command errors = %+v, want ErrUnknownEntity", result.CommandErrors)
		}
	})

	t.Run("non spawner entity", func(t *testing.T) {
		zoneWorker := newWorkerForMapDefinition(t, definition)
		assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
		mustInsertWorkerEntity(t, zoneWorker, "entity_non_spawner_npc", world.EntityTypeNPC, world.Vec2{X: 700, Y: 700})

		result := tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
			Definition:  definition,
			NPCEntityID: "entity_non_spawner_npc",
			KilledAt:    time.Date(2026, 6, 17, 12, 6, 0, 0, time.UTC),
		})
		if len(result.CommandErrors) != 1 || !errors.Is(result.CommandErrors[0].Err, ErrUnknownEntity) {
			t.Fatalf("command errors = %+v, want ErrUnknownEntity", result.CommandErrors)
		}
		if _, ok := zoneWorker.Entity("entity_non_spawner_npc"); !ok {
			t.Fatal("non-spawner entity was removed despite ErrUnknownEntity")
		}
	})
}

func TestEnemySpawnerPeriodicFillWaitsForIntervalAndRespectsCaps(t *testing.T) {
	definition := testEnemyMapDefinition()
	definition.EnemyPools[0].InitialAlive = 0
	definition.EnemyPools[0].PoolMaxAlive = 2
	definition.EnemyPools[0].MapMaxAlive = 2
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	clock.Advance(definition.EnemyPools[0].SpawnInterval - time.Nanosecond)
	assertNoCommandErrors(t, zoneWorker.Tick())
	if snapshot := zoneWorker.EnemySpawnSnapshot(); len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 {
		t.Fatalf("snapshot before periodic interval = %+v, want no fill", snapshot)
	}

	clock.Advance(time.Nanosecond)
	assertNoCommandErrors(t, zoneWorker.Tick())
	first := zoneWorker.EnemySpawnSnapshot()
	if len(first.Records) != 1 || first.MapAliveCount != 1 || first.PoolAliveCounts["test_pool"] != 1 {
		t.Fatalf("snapshot after first periodic fill = %+v, want one alive", first)
	}
	firstID := first.Records[0].EntityID

	clock.Advance(definition.EnemyPools[0].SpawnInterval)
	assertNoCommandErrors(t, zoneWorker.Tick())
	second := zoneWorker.EnemySpawnSnapshot()
	if len(second.Records) != 2 || second.MapAliveCount != 2 || second.PoolAliveCounts["test_pool"] != 2 {
		t.Fatalf("snapshot after second periodic fill = %+v, want two alive capped by pool/map", second)
	}
	if second.Records[0].EntityID == second.Records[1].EntityID {
		t.Fatalf("periodic fill records = %+v, want distinct entity ids", second.Records)
	}
	if !hasWorkerSnapshotEntityID(zoneWorker.Snapshot(), firstID) {
		t.Fatalf("worker snapshot missing first periodic entity %q", firstID)
	}

	clock.Advance(definition.EnemyPools[0].SpawnInterval)
	assertNoCommandErrors(t, zoneWorker.Tick())
	capped := zoneWorker.EnemySpawnSnapshot()
	if len(capped.Records) != 2 || capped.MapAliveCount != 2 || len(zoneWorker.Snapshot().Entities) != 2 {
		t.Fatalf("snapshot after capped periodic tick = %+v entities=%+v, want no third spawn", capped, zoneWorker.Snapshot().Entities)
	}
}

func TestEnemySpawnerKillReplacementAndDisabledModesStayInertForPeriodicFill(t *testing.T) {
	t.Run("kill replacement only respawns killed rows", func(t *testing.T) {
		definition := testEnemyMapDefinition()
		definition.EnemyPools[0].SpawnMode = worldmaps.SpawnModeKillReplacement
		definition.EnemyPools[0].InitialAlive = 1
		definition.EnemyPools[0].PoolMaxAlive = 3
		definition.EnemyPools[0].MapMaxAlive = 3
		zoneWorker := newWorkerForMapDefinition(t, definition)
		clock := fakeClockForWorker(t, zoneWorker)

		assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
		record := zoneWorker.EnemySpawnSnapshot().Records[0]
		clock.Advance(definition.EnemyPools[0].SpawnInterval)
		assertNoCommandErrors(t, zoneWorker.Tick())
		if snapshot := zoneWorker.EnemySpawnSnapshot(); len(snapshot.Records) != 1 || snapshot.MapAliveCount != 1 {
			t.Fatalf("kill replacement snapshot after periodic interval = %+v, want no periodic fill", snapshot)
		}

		assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
			Definition:  definition,
			NPCEntityID: record.EntityID,
			KilledAt:    clock.Now(),
		}))
		clock.Advance(definition.EnemyPools[0].KillRespawnDelay)
		assertNoCommandErrors(t, zoneWorker.Tick())
		respawned := zoneWorker.EnemySpawnSnapshot()
		if len(respawned.Records) != 1 || respawned.MapAliveCount != 1 || respawned.Records[0].EntityID != record.EntityID || !respawned.Records[0].Alive {
			t.Fatalf("kill replacement snapshot after respawn = %+v, want same killed row restored only", respawned)
		}
	})

	t.Run("disabled stays inert after ticks", func(t *testing.T) {
		definition := testEnemyMapDefinition()
		definition.EnemyPools[0].SpawnMode = worldmaps.SpawnModeDisabled
		definition.EnemyPools[0].InitialAlive = 1
		zoneWorker := newWorkerForMapDefinition(t, definition)
		clock := fakeClockForWorker(t, zoneWorker)

		assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
		clock.Advance(definition.EnemyPools[0].SpawnInterval)
		assertNoCommandErrors(t, zoneWorker.Tick())
		if snapshot := zoneWorker.EnemySpawnSnapshot(); len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 || len(zoneWorker.Snapshot().Entities) != 0 {
			t.Fatalf("disabled snapshot=%+v entities=%+v, want no spawn after tick", snapshot, zoneWorker.Snapshot().Entities)
		}
	})
}

func TestEnemySpawnerPeriodicFillSkipsForbiddenCandidatesWithoutEntityLeak(t *testing.T) {
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
					SafeZoneID: "safe_periodic_block",
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
					PortalID:          "periodic_gate",
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
			definition.EnemyPools[0].InitialAlive = 0
			definition.EnemyPools[0].PoolMaxAlive = 1
			definition.EnemyPools[0].MapMaxAlive = 1
			tc.edit(&definition)
			zoneWorker := newWorkerForMapDefinition(t, definition)
			clock := fakeClockForWorker(t, zoneWorker)

			assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
			clock.Advance(definition.EnemyPools[0].SpawnInterval)
			assertNoCommandErrors(t, zoneWorker.Tick())

			snapshot := zoneWorker.EnemySpawnSnapshot()
			if len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 || len(zoneWorker.Snapshot().Entities) != 0 {
				t.Fatalf("snapshot=%+v entities=%+v, want forbidden periodic candidate skipped without entity leak", snapshot, zoneWorker.Snapshot().Entities)
			}
		})
	}
}

func TestEnemySpawnerPeriodicFillUsesDeterministicSpawnJitter(t *testing.T) {
	definition := testEnemyMapDefinition()
	definition.EnemyPools[0].EnemyPoolID = "test_pool_periodic_jitter"
	definition.EnemyPools[0].InitialAlive = 0
	definition.EnemyPools[0].PoolMaxAlive = 1
	definition.EnemyPools[0].MapMaxAlive = 1
	definition.EnemyPools[0].SpawnJitter = 5 * time.Second
	pool := definition.EnemyPools[0]
	jitter := deterministicSpawnJitter(pool.SpawnJitter, definition.InternalMapID.String(), pool.EnemyPoolID.String(), "periodic")
	if jitter <= 0 {
		t.Fatalf("deterministic periodic jitter = %s, want non-zero for coverage", jitter)
	}
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	clock.Advance(pool.SpawnInterval)
	assertNoCommandErrors(t, zoneWorker.Tick())
	if snapshot := zoneWorker.EnemySpawnSnapshot(); len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 {
		t.Fatalf("snapshot at base interval = %+v, want jitter to delay periodic fill", snapshot)
	}

	spawnedAt := clock.Advance(jitter)
	assertNoCommandErrors(t, zoneWorker.Tick())
	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 1 || snapshot.MapAliveCount != 1 || !snapshot.Records[0].Alive || !snapshot.Records[0].SpawnedAt.Equal(spawnedAt) {
		t.Fatalf("snapshot at deterministic jitter due time = %+v, want one periodic spawn at %s", snapshot, spawnedAt)
	}
}

func TestEnemySpawnerRespawnUsesDeterministicSpawnJitter(t *testing.T) {
	definition := testEnemyMapDefinition()
	definition.EnemyPools[0].EnemyPoolID = "test_pool_respawn_jitter"
	definition.EnemyPools[0].SpawnMode = worldmaps.SpawnModeKillReplacement
	definition.EnemyPools[0].PoolMaxAlive = 1
	definition.EnemyPools[0].MapMaxAlive = 1
	definition.EnemyPools[0].SpawnJitter = 5 * time.Second
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	record := zoneWorker.EnemySpawnSnapshot().Records[0]
	pool := definition.EnemyPools[0]
	jitter := deterministicSpawnJitter(pool.SpawnJitter, definition.InternalMapID.String(), pool.EnemyPoolID.String(), record.EntityID.String())
	if jitter <= 0 {
		t.Fatalf("deterministic respawn jitter = %s, want non-zero for coverage", jitter)
	}
	killedAt := clock.Now()
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    killedAt,
	}))
	dead, ok := zoneWorker.EnemySpawnRecord(record.EntityID)
	if !ok || !dead.NextRespawnAt.Equal(killedAt.Add(pool.KillRespawnDelay+jitter)) {
		t.Fatalf("dead record = %+v ok=%v, want deterministic jitter respawn at %s", dead, ok, killedAt.Add(pool.KillRespawnDelay+jitter))
	}

	clock.Advance(pool.KillRespawnDelay)
	assertNoCommandErrors(t, zoneWorker.Tick())
	beforeJitterDue, ok := zoneWorker.EnemySpawnRecord(record.EntityID)
	if !ok || beforeJitterDue.Alive {
		t.Fatalf("record at base kill delay = %+v ok=%v, want jitter to delay respawn", beforeJitterDue, ok)
	}

	respawnedAt := clock.Advance(jitter)
	assertNoCommandErrors(t, zoneWorker.Tick())
	respawned, ok := zoneWorker.EnemySpawnRecord(record.EntityID)
	if !ok || !respawned.Alive || respawned.EntityID != record.EntityID || !respawned.SpawnedAt.Equal(respawnedAt) {
		t.Fatalf("record at deterministic jitter due time = %+v ok=%v, want same entity respawned at %s", respawned, ok, respawnedAt)
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

func fakeClockForWorker(t *testing.T, zoneWorker *Worker) *testutil.FakeClock {
	t.Helper()

	clock, ok := zoneWorker.clock.(*testutil.FakeClock)
	if !ok {
		t.Fatalf("worker clock = %T, want *testutil.FakeClock", zoneWorker.clock)
	}
	return clock
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
