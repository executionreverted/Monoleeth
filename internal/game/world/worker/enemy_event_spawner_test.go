package worker

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestEnemySpawnerStarterDisabledEventHookDoesNotSpawnDuringInitializationOrTick(t *testing.T) {
	catalog, err := worldmaps.StarterCatalog("world-1")
	if err != nil {
		t.Fatalf("StarterCatalog() error = %v, want nil", err)
	}
	definition, ok := catalog.Get(worldmaps.StarterMapID)
	if !ok {
		t.Fatal("starter map definition missing")
	}
	if len(definition.NPCEventSpawns) != 1 || definition.NPCEventSpawns[0].Enabled {
		t.Fatalf("starter event spawns = %+v, want one disabled hook", definition.NPCEventSpawns)
	}
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	clock.Advance(5 * time.Minute)
	assertNoCommandErrors(t, zoneWorker.Tick())
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, TriggerEnemyEventSpawnCommand{
		Definition:   definition,
		EventSpawnID: definition.NPCEventSpawns[0].EventSpawnID,
		TriggeredAt:  clock.Now(),
	}))

	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 1 || snapshot.MapAliveCount != 1 {
		t.Fatalf("snapshot = %+v, want only starter training NPC after disabled event hook", snapshot)
	}
	if snapshot.Records[0].EventSpawnID != "" || snapshot.Records[0].EnemyPoolID != "starter_training_drone_pool" {
		t.Fatalf("record = %+v, want normal starter pool record only", snapshot.Records[0])
	}
	if len(zoneWorker.Snapshot().Entities) != 1 {
		t.Fatalf("worker entities = %+v, want one starter NPC only", zoneWorker.Snapshot().Entities)
	}
}

func TestEnemySpawnerTriggerEventSpawnCreatesDueEventNPC(t *testing.T) {
	definition := testEventEnemyMapDefinition()
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	if snapshot := zoneWorker.EnemySpawnSnapshot(); len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 {
		t.Fatalf("snapshot after initialization = %+v, want no event-scheduled initial fill", snapshot)
	}
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, TriggerEnemyEventSpawnCommand{
		Definition:   definition,
		EventSpawnID: "test_event_spawn",
		TriggeredAt:  clock.Now(),
	}))

	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 1 || snapshot.MapAliveCount != 1 || snapshot.PoolAliveCounts["test_event_pool"] != 1 {
		t.Fatalf("snapshot after event trigger = %+v, want one alive event NPC", snapshot)
	}
	record := snapshot.Records[0]
	if record.EventSpawnID != "test_event_spawn" ||
		record.EnemyPoolID != "test_event_pool" ||
		record.SpawnAreaID != "test_area" ||
		record.NPCType != "event_boss" ||
		record.Level != 3 ||
		record.StatTemplateID != "event_stat" ||
		record.DropProfileID != "event_drop" ||
		record.AggroProfileID != "test_aggro" ||
		record.LeashProfileID != "test_leash" ||
		!record.Alive ||
		!record.SpawnedAt.Equal(clock.Now()) ||
		record.Position != definition.SpawnAreas[0].Center ||
		record.LeashOrigin != definition.SpawnAreas[0].Center {
		t.Fatalf("event record = %+v, want event pool/stat/drop/aggro/leash metadata", record)
	}
	assertEventEntityIDOpaque(t, record.EntityID, definition.InternalMapID.String(), "test_event_pool", "test_event_spawn")
	entity, ok := zoneWorker.Entity(record.EntityID)
	if !ok || entity.Type != world.EntityTypeNPC || entity.Position != record.Position {
		t.Fatalf("event entity = %+v ok=%v, want NPC at record position", entity, ok)
	}
	if speed, ok := zoneWorker.EntitySpeed(record.EntityID); !ok || speed != definition.NPCStatTemplates[0].Speed {
		t.Fatalf("event speed = %v ok=%v, want stat template speed %v", speed, ok, definition.NPCStatTemplates[0].Speed)
	}
}

func TestEnemySpawnerTriggerEventSpawnRejectsInternalMapZoneMismatchBeforeMutation(t *testing.T) {
	definition := testEventEnemyMapDefinition()
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)
	malformed := definition
	malformed.InternalMapID = "map_1_2"

	result := tickSubmitted(t, zoneWorker, TriggerEnemyEventSpawnCommand{
		Definition:   malformed,
		EventSpawnID: "test_event_spawn",
		TriggeredAt:  clock.Now(),
	})
	assertEventSpawnCommandError(t, result, ErrInvalidWorkerConfig)
	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 || len(zoneWorker.Snapshot().Entities) != 0 {
		t.Fatalf("snapshot=%+v entities=%+v, want ownership rejection before event spawn mutation", snapshot, zoneWorker.Snapshot().Entities)
	}
}

func TestEnemySpawnerTriggerEventSpawnInitializesMissingDueCacheBeforeSpawn(t *testing.T) {
	definition := testEventEnemyMapDefinition()
	definition.NPCEventSpawns[0].StartsAfter = time.Minute
	configuredDefinition := definition
	configuredDefinition.NPCEventSpawns = nil
	zoneWorker := newWorkerForMapDefinition(t, configuredDefinition)
	clock := fakeClockForWorker(t, zoneWorker)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: configuredDefinition}))
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, TriggerEnemyEventSpawnCommand{
		Definition:   definition,
		EventSpawnID: "test_event_spawn",
		TriggeredAt:  clock.Now(),
	}))
	if snapshot := zoneWorker.EnemySpawnSnapshot(); len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 {
		t.Fatalf("snapshot before newly observed event due = %+v, want no event spawn", snapshot)
	}

	spawnedAt := clock.Advance(time.Minute)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, TriggerEnemyEventSpawnCommand{
		Definition:   definition,
		EventSpawnID: "test_event_spawn",
		TriggeredAt:  spawnedAt,
	}))
	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 1 || !snapshot.Records[0].SpawnedAt.Equal(spawnedAt) {
		t.Fatalf("snapshot after newly observed event due = %+v, want one event spawn at %s", snapshot, spawnedAt)
	}
}

func TestEnemySpawnerTriggerEventSpawnUsesCompatibleEventDropProfileOverride(t *testing.T) {
	definition := testEventEnemyMapDefinition()
	overrideProfile := definition.NPCDropProfiles[0]
	overrideProfile.DropProfileID = "event_bonus_drop"
	overrideProfile.LootTableID = "event_bonus_loot"
	definition.NPCEventSpawns[0].DropProfileID = overrideProfile.DropProfileID
	definition.NPCDropProfiles = append(definition.NPCDropProfiles, overrideProfile)
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, TriggerEnemyEventSpawnCommand{
		Definition:   definition,
		EventSpawnID: "test_event_spawn",
		TriggeredAt:  clock.Now(),
	}))

	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 1 || snapshot.Records[0].DropProfileID != overrideProfile.DropProfileID {
		t.Fatalf("snapshot = %+v, want event record to use compatible override drop profile %q", snapshot, overrideProfile.DropProfileID)
	}
}

func TestEnemySpawnerTriggerEventSpawnRejectsRuntimeInvalidStatTemplateOrMapPolicy(t *testing.T) {
	tests := []struct {
		name string
		edit func(*worldmaps.MapDefinition)
		want error
	}{
		{
			name: "stat template npc type mismatch",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.NPCStatTemplates[0].NPCType = "wrong_event_boss"
			},
			want: worldmaps.ErrInvalidCatalog,
		},
		{
			name: "stat template level band does not cover pool",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.NPCStatTemplates[0].MinLevel = 4
				definition.NPCStatTemplates[0].MaxLevel = 4
			},
			want: worldmaps.ErrInvalidCatalog,
		},
		{
			name: "invalid map policy",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.NPCEventSpawns[0].MapPolicy = "all_maps"
			},
			want: worldmaps.ErrInvalidMapDefinition,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			definition := testEventEnemyMapDefinition()
			tc.edit(&definition)
			zoneWorker := newWorkerForMapDefinition(t, definition)
			clock := fakeClockForWorker(t, zoneWorker)
			assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

			result := tickSubmitted(t, zoneWorker, TriggerEnemyEventSpawnCommand{
				Definition:   definition,
				EventSpawnID: "test_event_spawn",
				TriggeredAt:  clock.Now(),
			})
			assertEventSpawnCommandError(t, result, tc.want)
			if snapshot := zoneWorker.EnemySpawnSnapshot(); len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 || len(zoneWorker.Snapshot().Entities) != 0 {
				t.Fatalf("snapshot=%+v entities=%+v, want invalid runtime event guard to insert nothing", snapshot, zoneWorker.Snapshot().Entities)
			}
		})
	}
}

func TestEnemySpawnerTriggerEventSpawnRejectsRuntimeInvalidDropProfileCompatibility(t *testing.T) {
	tests := []struct {
		name string
		edit func(*worldmaps.MapDefinition)
	}{
		{
			name: "drop profile npc type mismatch",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.NPCDropProfiles[0].NPCType = "wrong_event_boss"
			},
		},
		{
			name: "drop profile level band does not cover pool",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.NPCDropProfiles[0].MinLevel = 4
				definition.NPCDropProfiles[0].MaxLevel = 4
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			definition := testEventEnemyMapDefinition()
			tc.edit(&definition)
			zoneWorker := newWorkerForMapDefinition(t, definition)
			clock := fakeClockForWorker(t, zoneWorker)
			assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

			result := tickSubmitted(t, zoneWorker, TriggerEnemyEventSpawnCommand{
				Definition:   definition,
				EventSpawnID: "test_event_spawn",
				TriggeredAt:  clock.Now(),
			})
			assertEventSpawnCommandError(t, result, worldmaps.ErrInvalidCatalog)
			if snapshot := zoneWorker.EnemySpawnSnapshot(); len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 || len(zoneWorker.Snapshot().Entities) != 0 {
				t.Fatalf("snapshot=%+v entities=%+v, want invalid drop profile guard to insert nothing", snapshot, zoneWorker.Snapshot().Entities)
			}
		})
	}
}

func TestEnemySpawnerTriggerEventSpawnNoOpsDisabledOrNotDueWithoutEntityLeak(t *testing.T) {
	tests := []struct {
		name string
		edit func(*worldmaps.MapDefinition)
	}{
		{
			name: "disabled hook",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.NPCEventSpawns[0].Enabled = false
			},
		},
		{
			name: "not yet due",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.NPCEventSpawns[0].StartsAfter = time.Minute
			},
		},
		{
			name: "disabled pool",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.EnemyPools[0].Enabled = false
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			definition := testEventEnemyMapDefinition()
			tc.edit(&definition)
			zoneWorker := newWorkerForMapDefinition(t, definition)
			clock := fakeClockForWorker(t, zoneWorker)

			assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
			assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, TriggerEnemyEventSpawnCommand{
				Definition:   definition,
				EventSpawnID: "test_event_spawn",
				TriggeredAt:  clock.Now(),
			}))

			snapshot := zoneWorker.EnemySpawnSnapshot()
			if len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 || len(zoneWorker.Snapshot().Entities) != 0 {
				t.Fatalf("snapshot=%+v entities=%+v, want no event entity leak", snapshot, zoneWorker.Snapshot().Entities)
			}
		})
	}
}

func TestEnemySpawnerTriggerEventSpawnRespectsEventPoolAndMapCaps(t *testing.T) {
	tests := []struct {
		name          string
		edit          func(*worldmaps.MapDefinition)
		triggerIDs    []worldmaps.NPCEventSpawnID
		wantAlive     int
		wantPoolAlive int
		wantTotalRows int
	}{
		{
			name: "event cap",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.NPCEventSpawns[0].MaxAlive = 1
				definition.EnemyPools[0].PoolMaxAlive = 3
				definition.EnemyPools[0].MapMaxAlive = 3
			},
			triggerIDs:    []worldmaps.NPCEventSpawnID{"test_event_spawn", "test_event_spawn", "test_event_spawn"},
			wantAlive:     1,
			wantPoolAlive: 1,
			wantTotalRows: 1,
		},
		{
			name: "pool cap",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.NPCEventSpawns[0].MaxAlive = 1
				definition.EnemyPools[0].PoolMaxAlive = 1
				definition.EnemyPools[0].MapMaxAlive = 3
				second := definition.NPCEventSpawns[0]
				second.EventSpawnID = "test_event_spawn_two"
				definition.NPCEventSpawns = append(definition.NPCEventSpawns, second)
			},
			triggerIDs:    []worldmaps.NPCEventSpawnID{"test_event_spawn", "test_event_spawn_two"},
			wantAlive:     1,
			wantPoolAlive: 1,
			wantTotalRows: 1,
		},
		{
			name: "map cap",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.NPCEventSpawns[0].MaxAlive = 3
				definition.EnemyPools[0].PoolMaxAlive = 3
				definition.EnemyPools[0].MapMaxAlive = 3
				normalPool, statTemplate, dropProfile := testMapCapNormalEnemyContent("map_cap", 3)
				definition.EnemyPools = append([]worldmaps.MapEnemyPoolDefinition{normalPool}, definition.EnemyPools...)
				definition.NPCStatTemplates = append(definition.NPCStatTemplates, statTemplate)
				definition.NPCDropProfiles = append(definition.NPCDropProfiles, dropProfile)
			},
			triggerIDs:    []worldmaps.NPCEventSpawnID{"test_event_spawn", "test_event_spawn"},
			wantAlive:     3,
			wantPoolAlive: 1,
			wantTotalRows: 3,
		},
		{
			name: "lower enabled map cap from another pool",
			edit: func(definition *worldmaps.MapDefinition) {
				definition.NPCEventSpawns[0].MaxAlive = 3
				definition.EnemyPools[0].PoolMaxAlive = 3
				definition.EnemyPools[0].MapMaxAlive = 5
				normalPool, statTemplate, dropProfile := testMapCapNormalEnemyContent("lower_map_cap", 2)
				definition.EnemyPools = append([]worldmaps.MapEnemyPoolDefinition{normalPool}, definition.EnemyPools...)
				definition.NPCStatTemplates = append(definition.NPCStatTemplates, statTemplate)
				definition.NPCDropProfiles = append(definition.NPCDropProfiles, dropProfile)
			},
			triggerIDs:    []worldmaps.NPCEventSpawnID{"test_event_spawn"},
			wantAlive:     2,
			wantPoolAlive: 0,
			wantTotalRows: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			definition := testEventEnemyMapDefinition()
			tc.edit(&definition)
			zoneWorker := newWorkerForMapDefinition(t, definition)
			clock := fakeClockForWorker(t, zoneWorker)
			assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))

			for _, triggerID := range tc.triggerIDs {
				assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, TriggerEnemyEventSpawnCommand{
					Definition:   definition,
					EventSpawnID: triggerID,
					TriggeredAt:  clock.Now(),
				}))
			}

			snapshot := zoneWorker.EnemySpawnSnapshot()
			if len(snapshot.Records) != tc.wantTotalRows ||
				snapshot.MapAliveCount != tc.wantAlive ||
				snapshot.PoolAliveCounts["test_event_pool"] != tc.wantPoolAlive ||
				len(zoneWorker.Snapshot().Entities) != tc.wantAlive {
				t.Fatalf("snapshot=%+v entities=%+v, want rows=%d map alive=%d pool alive=%d",
					snapshot, zoneWorker.Snapshot().Entities, tc.wantTotalRows, tc.wantAlive, tc.wantPoolAlive)
			}
		})
	}
}

func TestEnemySpawnerTriggerEventSpawnSkipsForbiddenCandidatesWithoutEntityLeak(t *testing.T) {
	definition := testEventEnemyMapDefinition()
	definition.SpawnAreas[0].Center = world.Vec2{X: 500, Y: 500}
	definition.SpawnAreas[0].PortalExclusionRadius = 200
	definition.Portals = []worldmaps.PortalDefinition{{
		PortalID:          "event_gate",
		SourceMapID:       definition.InternalMapID,
		SourcePosition:    world.Vec2{X: 500, Y: 500},
		InteractionRadius: 180,
		Visible:           true,
	}}
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition}))
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, TriggerEnemyEventSpawnCommand{
		Definition:   definition,
		EventSpawnID: "test_event_spawn",
		TriggeredAt:  clock.Now(),
	}))

	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 || len(zoneWorker.Snapshot().Entities) != 0 {
		t.Fatalf("snapshot=%+v entities=%+v, want forbidden event candidate skipped without entity leak", snapshot, zoneWorker.Snapshot().Entities)
	}
}

func assertEventEntityIDOpaque(t *testing.T, entityID world.EntityID, forbidden ...string) {
	t.Helper()

	rawID := entityID.String()
	for _, value := range forbidden {
		if strings.Contains(rawID, value) {
			t.Fatalf("event entity id %q leaked raw identifier %q", entityID, value)
		}
	}
	parts := strings.FieldsFunc(rawID, func(r rune) bool {
		return r == '_' || r == '-'
	})
	for _, part := range parts {
		if len(part) == 0 || len(part)%2 != 0 {
			continue
		}
		decoded, err := hex.DecodeString(part)
		if err != nil {
			continue
		}
		decodedText := string(decoded)
		for _, value := range forbidden {
			if strings.Contains(decodedText, value) {
				t.Fatalf("event entity id %q hex part %q decoded to %q and leaked %q", entityID, part, decodedText, value)
			}
		}
	}
}

func assertEventSpawnCommandError(t *testing.T, result TickResult, want error) {
	t.Helper()

	if len(result.CommandErrors) != 1 || !errors.Is(result.CommandErrors[0].Err, want) {
		t.Fatalf("command errors = %+v, want one wrapping %v", result.CommandErrors, want)
	}
}

func testMapCapNormalEnemyContent(suffix string, mapMaxAlive int) (worldmaps.MapEnemyPoolDefinition, worldmaps.NPCStatTemplate, worldmaps.NPCDropProfile) {
	poolID := worldmaps.EnemyPoolID(suffix + "_normal_pool")
	statID := worldmaps.NPCStatTemplateID(suffix + "_normal_stat")
	dropID := worldmaps.NPCDropProfileID(suffix + "_normal_drop")
	npcType := suffix + "_drone"
	return worldmaps.MapEnemyPoolDefinition{
			EnemyPoolID:      poolID,
			NPCType:          npcType,
			MinLevel:         1,
			MaxLevel:         1,
			SpawnAreaIDs:     []worldmaps.SpawnAreaID{"test_area"},
			MapMaxAlive:      mapMaxAlive,
			PoolMaxAlive:     2,
			InitialAlive:     2,
			SpawnInterval:    30 * time.Second,
			KillRespawnDelay: 30 * time.Second,
			SpawnJitter:      0,
			SpawnMode:        worldmaps.SpawnModePeriodic,
			StatTemplateID:   statID,
			DropProfileID:    dropID,
			AggroProfileID:   "test_aggro",
			LeashProfileID:   "test_leash",
			Enabled:          true,
		}, worldmaps.NPCStatTemplate{
			StatTemplateID: statID,
			NPCType:        npcType,
			MinLevel:       1,
			MaxLevel:       1,
			LabelKey:       "npc." + npcType,
			HPMax:          10,
			WeaponRange:    1,
			WeaponCooldown: time.Second,
			Accuracy:       1,
		}, worldmaps.NPCDropProfile{
			DropProfileID: dropID,
			NPCType:       npcType,
			MinLevel:      1,
			MaxLevel:      1,
			RiskBand:      "low",
			LootTableID:   suffix + "_normal_loot",
		}
}

func testEventEnemyMapDefinition() worldmaps.MapDefinition {
	definition := testEnemyMapDefinition()
	definition.EnemyPools = []worldmaps.MapEnemyPoolDefinition{{
		EnemyPoolID:      "test_event_pool",
		NPCType:          "event_boss",
		MinLevel:         3,
		MaxLevel:         3,
		SpawnAreaIDs:     []worldmaps.SpawnAreaID{"test_area"},
		MapMaxAlive:      3,
		PoolMaxAlive:     3,
		InitialAlive:     0,
		SpawnInterval:    30 * time.Second,
		KillRespawnDelay: 30 * time.Second,
		SpawnJitter:      0,
		SpawnMode:        worldmaps.SpawnModeEventScheduled,
		StatTemplateID:   "event_stat",
		DropProfileID:    "event_drop",
		AggroProfileID:   "test_aggro",
		LeashProfileID:   "test_leash",
		Enabled:          true,
	}}
	definition.NPCStatTemplates = []worldmaps.NPCStatTemplate{{
		StatTemplateID: "event_stat",
		NPCType:        "event_boss",
		MinLevel:       3,
		MaxLevel:       3,
		LabelKey:       "npc.event_boss",
		HPMax:          500,
		ShieldMax:      100,
		EnergyMax:      50,
		WeaponRange:    200,
		WeaponDamage:   20,
		WeaponCooldown: 2 * time.Second,
		Accuracy:       0.9,
		Speed:          12,
		XPValue:        100,
	}}
	definition.NPCDropProfiles = []worldmaps.NPCDropProfile{{
		DropProfileID: "event_drop",
		NPCType:       "event_boss",
		MinLevel:      3,
		MaxLevel:      3,
		RiskBand:      "low",
		LootTableID:   "event_loot",
	}}
	definition.NPCEventSpawns = []worldmaps.NPCEventSpawnDefinition{{
		EventSpawnID:  "test_event_spawn",
		EnemyPoolID:   "test_event_pool",
		DropProfileID: "event_drop",
		Enabled:       true,
		StartsAfter:   0,
		MaxAlive:      3,
		MapPolicy:     worldmaps.NPCEventSpawnMapPolicyCurrentMapOnly,
	}}
	return definition
}
