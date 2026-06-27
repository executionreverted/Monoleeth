package worker

import (
	"fmt"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestSeededPassiveEnemyAggroProfilesDoNotAcquireOrMoveAcrossMaps(t *testing.T) {
	catalog, err := worldmaps.StarterCatalog("world-1")
	if err != nil {
		t.Fatalf("StarterCatalog() error = %v, want nil", err)
	}

	for _, mapID := range []worldmaps.MapID{worldmaps.StarterMapID, "map_1_2"} {
		definition, ok := catalog.Get(mapID)
		if !ok {
			t.Fatalf("map definition %q missing", mapID)
		}
		t.Run(string(definition.PublicMapKey), func(t *testing.T) {
			if len(definition.SpawnAreas) == 0 {
				t.Fatalf("map %s has no seeded spawn areas", definition.PublicMapKey)
			}
			zoneWorker := newWorkerForMapDefinition(t, definition)
			spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", definition.SpawnAreas[0].Center, 100)

			assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

			snapshot := zoneWorker.EnemySpawnSnapshot()
			if len(snapshot.Records) != 1 || snapshot.MapAliveCount != 1 {
				t.Fatalf("EnemySpawnSnapshot() = %+v, want one initial seeded NPC row", snapshot)
			}
			record := snapshot.Records[0]
			if !record.AggroTargetEntityID.IsZero() || !record.AggroAcquiredAt.IsZero() || !record.AggroTargetLastSeenAt.IsZero() {
				t.Fatalf("%s passive aggro state = %+v, want no acquired target", definition.PublicMapKey, record)
			}
			entity, ok := zoneWorker.Entity(record.EntityID)
			if !ok {
				t.Fatalf("Entity(%q) missing", record.EntityID)
			}
			if entity.Movement.Moving || entity.Position != record.LeashOrigin || entity.Position != record.Position {
				t.Fatalf("%s passive entity = %+v record=%+v, want stationary at leash origin/record position", definition.PublicMapKey, entity, record)
			}
		})
	}
}

func TestSeededBorderSkirmishAggressiveEnemyAggroLeashUsesCatalogSeed(t *testing.T) {
	catalog, err := worldmaps.StarterCatalog("world-1")
	if err != nil {
		t.Fatalf("StarterCatalog() error = %v, want nil", err)
	}
	definition, ok := catalog.Get("map_1_3")
	if !ok {
		t.Fatalf("map_1_3 definition missing")
	}
	if definition.PublicMapKey != "1-3" {
		t.Fatalf("map_1_3 public key = %q, want 1-3", definition.PublicMapKey)
	}
	if len(definition.EnemyPools) != 1 || len(definition.SpawnAreas) != 1 ||
		len(definition.NPCStatTemplates) != 1 || len(definition.NPCAggroProfiles) != 1 ||
		len(definition.NPCLeashProfiles) != 1 {
		t.Fatalf("map 1-3 seed rows = pools:%+v areas:%+v stats:%+v aggro:%+v leash:%+v, want one border raider seed row",
			definition.EnemyPools,
			definition.SpawnAreas,
			definition.NPCStatTemplates,
			definition.NPCAggroProfiles,
			definition.NPCLeashProfiles,
		)
	}

	pool := definition.EnemyPools[0]
	spawnArea := definition.SpawnAreas[0]
	statTemplate := definition.NPCStatTemplates[0]
	aggroProfile := definition.NPCAggroProfiles[0]
	leashProfile := definition.NPCLeashProfiles[0]
	if pool.NPCType != "border_raider_drone" || pool.StatTemplateID != statTemplate.StatTemplateID ||
		pool.AggroProfileID != aggroProfile.AggroProfileID || pool.LeashProfileID != leashProfile.LeashProfileID {
		t.Fatalf("map 1-3 enemy seed refs pool=%+v stat=%+v aggro=%+v leash=%+v, want border raider seed wiring",
			pool,
			statTemplate,
			aggroProfile,
			leashProfile,
		)
	}
	if aggroProfile.AggroRadius != 520 || aggroProfile.TargetMemory != 8*time.Second ||
		aggroProfile.SafeZoneAttackPolicy != "never" || leashProfile.LeashDistance != 900 ||
		!leashProfile.ResetOnBreak || statTemplate.Speed != 90 {
		t.Fatalf("map 1-3 aggro/leash/stat seed = aggro:%+v leash:%+v stat:%+v, want seeded border raider hunter behavior",
			aggroProfile,
			leashProfile,
			statTemplate,
		)
	}

	zoneWorker := newWorkerForMapDefinition(t, definition)
	spawnCenter := spawnArea.Center
	targetInsideAggro := world.Vec2{X: spawnCenter.X + 300, Y: spawnCenter.Y}
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", targetInsideAggro, 100)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

	acquired := onlyEnemyAggroRecord(t, zoneWorker)
	if acquired.EnemyPoolID != pool.EnemyPoolID || acquired.NPCType != pool.NPCType ||
		acquired.StatTemplateID != statTemplate.StatTemplateID ||
		acquired.AggroProfileID != aggroProfile.AggroProfileID ||
		acquired.LeashProfileID != leashProfile.LeashProfileID ||
		acquired.LeashOrigin != spawnCenter {
		t.Fatalf("seeded border raider record = %+v, want catalog-owned pool/stat/aggro/leash at %+v", acquired, spawnCenter)
	}
	if got, want := acquired.AggroTargetEntityID, world.EntityID("entity-player-1"); got != want {
		t.Fatalf("aggro target = %q, want seeded border raider to acquire %q", got, want)
	}
	if acquired.AggroAcquiredAt.IsZero() || acquired.AggroTargetLastSeenAt.IsZero() || acquired.LastAggroTickAt.IsZero() {
		t.Fatalf("aggro timing = %+v, want acquired/seen/ticked timestamps", acquired)
	}
	entity, ok := zoneWorker.Entity(acquired.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", acquired.EntityID)
	}
	if !entity.Movement.Moving || entity.Movement.Target != targetInsideAggro || entity.Movement.Speed != statTemplate.Speed {
		t.Fatalf("movement after acquire = %+v, want chase target %+v at seeded speed %v", entity.Movement, targetInsideAggro, statTemplate.Speed)
	}

	npcAwayFromOrigin := world.Vec2{X: spawnCenter.X + 200, Y: spawnCenter.Y}
	targetBeyondLeash := world.Vec2{X: spawnCenter.X + leashProfile.LeashDistance + 1, Y: spawnCenter.Y}
	setWorkerEntityPosition(t, zoneWorker, acquired.EntityID, npcAwayFromOrigin)
	setWorkerEntityPosition(t, zoneWorker, "entity-player-1", targetBeyondLeash)

	assertNoCommandErrors(t, zoneWorker.Tick())

	reset := onlyEnemyAggroRecord(t, zoneWorker)
	if !reset.AggroTargetEntityID.IsZero() || !reset.AggroAcquiredAt.IsZero() || !reset.AggroTargetLastSeenAt.IsZero() {
		t.Fatalf("aggro after seeded leash break = %+v, want target memory reset", reset)
	}
	entity, ok = zoneWorker.Entity(reset.EntityID)
	if !ok {
		t.Fatalf("Entity(%q) missing", reset.EntityID)
	}
	if reset.LeashOrigin != spawnCenter || !entity.Movement.Moving ||
		entity.Movement.Target != spawnCenter || entity.Movement.Speed != statTemplate.Speed {
		t.Fatalf("movement after seeded leash break = %+v record=%+v, want return to seed origin %+v at speed %v",
			entity.Movement,
			reset,
			spawnCenter,
			statTemplate.Speed,
		)
	}
	if entity.Movement.Target == targetBeyondLeash {
		t.Fatalf("movement after seeded leash break still chases target beyond leash: %+v", entity.Movement)
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

func TestEnemyAggroUsesSpatialPlayerIndexForCandidateChecks(t *testing.T) {
	definition := aggressiveEnemyMapDefinition()
	definition.NPCAggroProfiles[0].AggroRadius = 250
	zoneWorker := newWorkerForMapDefinition(t, definition)
	for index := 0; index < 64; index++ {
		spawnPlayer(
			t,
			zoneWorker,
			foundation.PlayerID(fmt.Sprintf("player-far-%02d", index)),
			world.EntityID(fmt.Sprintf("entity-player-far-%02d", index)),
			world.Vec2{X: 2000 + float64(index*25), Y: 2000},
			100,
		)
	}
	spawnPlayer(t, zoneWorker, "player-near", "entity-player-near", world.Vec2{X: 550, Y: 500}, 100)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

	record := onlyEnemyAggroRecord(t, zoneWorker)
	if got, want := record.AggroTargetEntityID, world.EntityID("entity-player-near"); got != want {
		t.Fatalf("aggro target = %q, want nearest %q", got, want)
	}
	if got, want := lastEnemyAggroCandidateChecks(t, zoneWorker), 1; got != want {
		t.Fatalf("aggro candidate checks = %d, want %d from player spatial radius query", got, want)
	}
}

func TestEnemyAggroSpatialPlayerIndexTracksMovementIntoRadius(t *testing.T) {
	definition := aggressiveEnemyMapDefinition()
	definition.NPCAggroProfiles[0].AggroRadius = 250
	definition.NPCLeashProfiles[0].LeashDistance = 1000
	zoneWorker := newWorkerForMapDefinition(t, definition)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{X: 900, Y: 500}, 1000)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	before := onlyEnemyAggroRecord(t, zoneWorker)
	if !before.AggroTargetEntityID.IsZero() {
		t.Fatalf("initial aggro target = %q, want none outside radius", before.AggroTargetEntityID)
	}

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MoveToCommand{
		PlayerID: "player-1",
		Intent:   mustMovementIntent(t, world.Vec2{X: 550, Y: 500}),
	}))

	after := onlyEnemyAggroRecord(t, zoneWorker)
	if got, want := after.AggroTargetEntityID, world.EntityID("entity-player-1"); got != want {
		t.Fatalf("aggro target after movement = %q, want moved player %q", got, want)
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

func lastEnemyAggroCandidateChecks(t *testing.T, zoneWorker *Worker) int {
	t.Helper()

	zoneWorker.mu.RLock()
	defer zoneWorker.mu.RUnlock()
	return zoneWorker.enemyAggroCandidateChecks
}
