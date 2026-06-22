package worker

import (
	"testing"
	"time"

	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestEnemyAggroPassiveStarterProfileDoesNotAcquireOrMove(t *testing.T) {
	catalog, err := worldmaps.StarterCatalog("world-1")
	if err != nil {
		t.Fatalf("StarterCatalog() error = %v, want nil", err)
	}
	definition, ok := catalog.Get(worldmaps.StarterMapID)
	if !ok {
		t.Fatal("starter map definition missing")
	}
	zoneWorker := newWorkerForMapDefinition(t, definition)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{X: 820, Y: 400}, 100)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

	record := onlyEnemyAggroRecord(t, zoneWorker)
	if !record.AggroTargetEntityID.IsZero() || !record.AggroAcquiredAt.IsZero() || !record.AggroTargetLastSeenAt.IsZero() {
		t.Fatalf("starter passive aggro state = %+v, want no acquired target", record)
	}
	entity, ok := zoneWorker.Entity(record.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", record.EntityID)
	}
	if entity.Movement.Moving || entity.Position != record.LeashOrigin {
		t.Fatalf("starter passive entity = %+v record=%+v, want stationary at leash origin", entity, record)
	}
}

func TestEnemyAggroAcquiresNearestPlayerAndStartsChase(t *testing.T) {
	definition := aggressiveEnemyMapDefinition()
	zoneWorker := newWorkerForMapDefinition(t, definition)
	spawnPlayer(t, zoneWorker, "player-far", "entity-player-far", world.Vec2{X: 610, Y: 500}, 100)
	spawnPlayer(t, zoneWorker, "player-near", "entity-player-near", world.Vec2{X: 550, Y: 500}, 100)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

	record := onlyEnemyAggroRecord(t, zoneWorker)
	if got, want := record.AggroTargetEntityID, world.EntityID("entity-player-near"); got != want {
		t.Fatalf("aggro target = %q, want nearest %q", got, want)
	}
	if record.AggroAcquiredAt.IsZero() || record.AggroTargetLastSeenAt.IsZero() || record.LastAggroTickAt.IsZero() {
		t.Fatalf("aggro timing = %+v, want acquired/seen/ticked timestamps", record)
	}
	entity, ok := zoneWorker.Entity(record.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", record.EntityID)
	}
	if !entity.Movement.Moving {
		t.Fatalf("entity movement = %+v, want chase movement", entity.Movement)
	}
	if entity.Movement.Target != (world.Vec2{X: 550, Y: 500}) || entity.Movement.Speed != definition.NPCStatTemplates[0].Speed {
		t.Fatalf("entity movement = %+v, want target nearest player with server speed %v", entity.Movement, definition.NPCStatTemplates[0].Speed)
	}
}

func TestEnemyAggroSkipsIneligiblePlayerAndAcquiresVisiblePlayer(t *testing.T) {
	definition := aggressiveEnemyMapDefinition()
	zoneWorker := newWorkerForMapDefinition(t, definition)
	spawnPlayer(t, zoneWorker, "player-hidden", "entity-player-hidden", world.Vec2{X: 520, Y: 500}, 100)
	spawnPlayer(t, zoneWorker, "player-visible", "entity-player-visible", world.Vec2{X: 590, Y: 500}, 100)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, SetPlayerAggroEligibilityCommand{
		PlayerID: "player-hidden",
		Eligible: false,
	}))

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

	record := onlyEnemyAggroRecord(t, zoneWorker)
	if got, want := record.AggroTargetEntityID, world.EntityID("entity-player-visible"); got != want {
		t.Fatalf("aggro target = %q, want visible eligible player %q", got, want)
	}
	entity, ok := zoneWorker.Entity(record.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", record.EntityID)
	}
	if !entity.Movement.Moving || entity.Movement.Target != (world.Vec2{X: 590, Y: 500}) {
		t.Fatalf("entity movement = %+v, want chase toward visible player", entity.Movement)
	}
}

func TestEnemyAggroDoesNotAcquireOutOfRadiusPlayer(t *testing.T) {
	definition := aggressiveEnemyMapDefinition()
	definition.NPCAggroProfiles[0].AggroRadius = 50
	zoneWorker := newWorkerForMapDefinition(t, definition)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{X: 600, Y: 500}, 100)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

	record := onlyEnemyAggroRecord(t, zoneWorker)
	if !record.AggroTargetEntityID.IsZero() {
		t.Fatalf("aggro target = %q, want none for out-of-radius player", record.AggroTargetEntityID)
	}
	entity, ok := zoneWorker.Entity(record.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", record.EntityID)
	}
	if entity.Movement.Moving {
		t.Fatalf("entity movement = %+v, want no chase for out-of-radius player", entity.Movement)
	}
}

func TestEnemyAggroClearsTargetWhenPlayerBecomesIneligible(t *testing.T) {
	definition := aggressiveEnemyMapDefinition()
	definition.NPCLeashProfiles[0].LeashDistance = 1000
	zoneWorker := newWorkerForMapDefinition(t, definition)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{X: 550, Y: 500}, 100)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	acquired := onlyEnemyAggroRecord(t, zoneWorker)
	if acquired.AggroTargetEntityID != "entity-player-1" {
		t.Fatalf("initial aggro record = %+v, want player acquired", acquired)
	}
	entity, ok := zoneWorker.Entity(acquired.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", acquired.EntityID)
	}
	if !entity.Movement.Moving || entity.Movement.Target != (world.Vec2{X: 550, Y: 500}) {
		t.Fatalf("initial movement = %+v, want chase toward player", entity.Movement)
	}
	entity.Position = world.Vec2{X: 530, Y: 500}
	if err := zoneWorker.UpdateEntity(entity); err != nil {
		t.Fatalf("UpdateEntity(%q) error = %v, want nil", entity.ID, err)
	}

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, SetPlayerAggroEligibilityCommand{
		PlayerID: "player-1",
		Eligible: false,
	}))

	cleared := onlyEnemyAggroRecord(t, zoneWorker)
	if !cleared.AggroTargetEntityID.IsZero() || !cleared.AggroAcquiredAt.IsZero() || !cleared.AggroTargetLastSeenAt.IsZero() {
		t.Fatalf("aggro after target became ineligible = %+v, want cleared", cleared)
	}
	entity, ok = zoneWorker.Entity(cleared.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", cleared.EntityID)
	}
	if entity.Position != (world.Vec2{X: 530, Y: 500}) {
		t.Fatalf("entity position = %+v, want no movement advance toward hidden player", entity.Position)
	}
	if !entity.Movement.Moving || entity.Movement.Target != cleared.LeashOrigin {
		t.Fatalf("movement after target became ineligible = %+v, want return toward leash origin %+v", entity.Movement, cleared.LeashOrigin)
	}
	if entity.Movement.Target == (world.Vec2{X: 550, Y: 500}) {
		t.Fatalf("movement target still points at hidden player coordinate: %+v", entity.Movement)
	}
}

func TestEnemyAggroTargetMemoryExpiresAfterTargetLeavesRadius(t *testing.T) {
	definition := aggressiveEnemyMapDefinition()
	definition.NPCAggroProfiles[0].AggroRadius = 100
	definition.NPCAggroProfiles[0].TargetMemory = 2 * time.Second
	definition.NPCLeashProfiles[0].LeashDistance = 1000
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{X: 550, Y: 500}, 100)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	acquired := onlyEnemyAggroRecord(t, zoneWorker)
	if acquired.AggroTargetEntityID != "entity-player-1" {
		t.Fatalf("initial aggro record = %+v, want player acquired", acquired)
	}

	setWorkerEntityPosition(t, zoneWorker, "entity-player-1", world.Vec2{X: 900, Y: 500})
	clock.Advance(time.Second)
	assertNoCommandErrors(t, zoneWorker.Tick())
	remembered := onlyEnemyAggroRecord(t, zoneWorker)
	if remembered.AggroTargetEntityID != "entity-player-1" {
		t.Fatalf("aggro before memory expiry = %+v, want target remembered", remembered)
	}
	entity, ok := zoneWorker.Entity(remembered.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", remembered.EntityID)
	}
	if !entity.Movement.Moving || entity.Movement.Target != (world.Vec2{X: 900, Y: 500}) {
		t.Fatalf("movement before memory expiry = %+v, want continued chase toward last target", entity.Movement)
	}

	clock.Advance(time.Second)
	assertNoCommandErrors(t, zoneWorker.Tick())
	expired := onlyEnemyAggroRecord(t, zoneWorker)
	if !expired.AggroTargetEntityID.IsZero() || !expired.AggroAcquiredAt.IsZero() || !expired.AggroTargetLastSeenAt.IsZero() {
		t.Fatalf("aggro after memory expiry = %+v, want reset", expired)
	}
	entity, ok = zoneWorker.Entity(expired.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", expired.EntityID)
	}
	if !entity.Movement.Moving || entity.Movement.Target != expired.LeashOrigin {
		t.Fatalf("movement after memory expiry = %+v, want return toward leash origin %+v", entity.Movement, expired.LeashOrigin)
	}
}

func TestEnemyAggroSafeZoneResetReturnsToLeashOrigin(t *testing.T) {
	definition := aggressiveEnemyMapDefinition()
	definition.NPCLeashProfiles[0].LeashDistance = 1000
	definition.SafeZones = []worldmaps.SafeZoneDefinition{{
		SafeZoneID: "target_safe_zone",
		Center:     world.Vec2{X: 700, Y: 500},
		Radius:     40,
		BlocksPVP:  true,
	}}
	zoneWorker := newWorkerForMapDefinition(t, definition)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{X: 550, Y: 500}, 100)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	acquired := onlyEnemyAggroRecord(t, zoneWorker)
	if acquired.AggroTargetEntityID != "entity-player-1" {
		t.Fatalf("initial aggro record = %+v, want player acquired", acquired)
	}

	setWorkerEntityPosition(t, zoneWorker, "entity-player-1", world.Vec2{X: 700, Y: 500})
	assertNoCommandErrors(t, zoneWorker.Tick())

	reset := onlyEnemyAggroRecord(t, zoneWorker)
	if !reset.AggroTargetEntityID.IsZero() || !reset.AggroAcquiredAt.IsZero() || !reset.AggroTargetLastSeenAt.IsZero() {
		t.Fatalf("aggro after target safe-zone entry = %+v, want reset", reset)
	}
	entity, ok := zoneWorker.Entity(reset.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", reset.EntityID)
	}
	if !entity.Movement.Moving || entity.Movement.Target != reset.LeashOrigin {
		t.Fatalf("movement after safe-zone reset = %+v, want return toward leash origin %+v", entity.Movement, reset.LeashOrigin)
	}
}

func TestEnemyAggroLeashBreakResetsAndReturnsToOrigin(t *testing.T) {
	definition := aggressiveEnemyMapDefinition()
	definition.NPCAggroProfiles[0].AggroRadius = 1000
	definition.NPCLeashProfiles[0].LeashDistance = 120
	zoneWorker := newWorkerForMapDefinition(t, definition)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{X: 550, Y: 500}, 100)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	acquired := onlyEnemyAggroRecord(t, zoneWorker)
	if acquired.AggroTargetEntityID != "entity-player-1" {
		t.Fatalf("initial aggro record = %+v, want player acquired", acquired)
	}

	setWorkerEntityPosition(t, zoneWorker, "entity-player-1", world.Vec2{X: 700, Y: 500})
	assertNoCommandErrors(t, zoneWorker.Tick())

	reset := onlyEnemyAggroRecord(t, zoneWorker)
	if !reset.AggroTargetEntityID.IsZero() {
		t.Fatalf("aggro target after leash break = %q, want reset", reset.AggroTargetEntityID)
	}
	entity, ok := zoneWorker.Entity(reset.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", reset.EntityID)
	}
	if !entity.Movement.Moving || entity.Movement.Target != reset.LeashOrigin {
		t.Fatalf("movement after leash break = %+v, want return toward leash origin %+v", entity.Movement, reset.LeashOrigin)
	}
}

func TestEnemyAggroDeadRowsDoNotAcquireAndRespawnClearsStaleState(t *testing.T) {
	definition := aggressiveEnemyMapDefinition()
	definition.EnemyPools[0].PoolMaxAlive = 1
	definition.EnemyPools[0].MapMaxAlive = 1
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{X: 550, Y: 500}, 100)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	record := onlyEnemyAggroRecord(t, zoneWorker)
	if record.AggroTargetEntityID != "entity-player-1" {
		t.Fatalf("initial aggro record = %+v, want player acquired", record)
	}

	killedAt := clock.Now()
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    killedAt,
	}))
	dead := onlyEnemyAggroRecord(t, zoneWorker)
	if dead.Alive || !dead.AggroTargetEntityID.IsZero() || !dead.AggroAcquiredAt.IsZero() || !dead.LastAggroTickAt.IsZero() {
		t.Fatalf("dead record = %+v, want dead with cleared aggro", dead)
	}

	clock.Advance(time.Second)
	assertNoCommandErrors(t, zoneWorker.Tick())
	stillDead := onlyEnemyAggroRecord(t, zoneWorker)
	if stillDead.Alive || !stillDead.AggroTargetEntityID.IsZero() {
		t.Fatalf("dead record after nearby-player tick = %+v, want no acquisition while dead", stillDead)
	}

	setWorkerEntityPosition(t, zoneWorker, "entity-player-1", world.Vec2{X: 2000, Y: 2000})
	zoneWorker.enemySpawner.rows[0].AggroTargetEntityID = "entity-player-1"
	zoneWorker.enemySpawner.rows[0].AggroAcquiredAt = clock.Now()
	zoneWorker.enemySpawner.rows[0].AggroTargetLastSeenAt = clock.Now()
	zoneWorker.enemySpawner.rows[0].LastAggroTickAt = clock.Now()
	respawnedAt := clock.Advance(definition.EnemyPools[0].KillRespawnDelay - time.Second)
	assertNoCommandErrors(t, zoneWorker.Tick())

	respawned := onlyEnemyAggroRecord(t, zoneWorker)
	if !respawned.Alive || !respawned.SpawnedAt.Equal(respawnedAt) {
		t.Fatalf("respawned record = %+v, want alive at %s", respawned, respawnedAt)
	}
	if !respawned.AggroTargetEntityID.IsZero() || !respawned.AggroAcquiredAt.IsZero() || !respawned.AggroTargetLastSeenAt.IsZero() {
		t.Fatalf("respawned aggro state = %+v, want stale aggro cleared", respawned)
	}
	if respawned.LeashOrigin != definition.SpawnAreas[0].Center || respawned.Position != definition.SpawnAreas[0].Center {
		t.Fatalf("respawned origin/position = %+v, want spawn center %+v", respawned, definition.SpawnAreas[0].Center)
	}
}

func aggressiveEnemyMapDefinition() worldmaps.MapDefinition {
	definition := testEnemyMapDefinition()
	definition.NPCStatTemplates[0].Speed = 100
	definition.NPCAggroProfiles[0].AggroRadius = 250
	definition.NPCAggroProfiles[0].TargetMemory = 5 * time.Second
	definition.NPCAggroProfiles[0].SafeZoneAttackPolicy = "never"
	definition.NPCLeashProfiles[0].LeashDistance = 1000
	definition.NPCLeashProfiles[0].ResetOnBreak = true
	return definition
}

func onlyEnemyAggroRecord(t *testing.T, zoneWorker *Worker) EnemySpawnRecord {
	t.Helper()

	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 1 {
		t.Fatalf("EnemySpawnSnapshot().Records = %+v, want exactly one record", snapshot.Records)
	}
	return snapshot.Records[0]
}

func setWorkerEntityPosition(t *testing.T, zoneWorker *Worker, entityID world.EntityID, position world.Vec2) {
	t.Helper()

	entity, ok := zoneWorker.Entity(entityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", entityID)
	}
	entity.Position = position
	entity.Movement = world.MovementState{}
	if err := zoneWorker.UpdateEntity(entity); err != nil {
		t.Fatalf("UpdateEntity(%q) error = %v, want nil", entityID, err)
	}
}
