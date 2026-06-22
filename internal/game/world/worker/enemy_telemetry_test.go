package worker

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEnemyLifecycleTelemetryReportsSpawnDeathRespawnWithSafeLabels(t *testing.T) {
	definition := testEnemyMapDefinition()
	zoneWorker := newWorkerForMapDefinition(t, definition)
	clock := fakeClockForWorker(t, zoneWorker)

	initial := tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition})
	assertNoCommandErrors(t, initial)
	requireEnemyTelemetry(t, initial.EnemyTelemetry, EnemyTelemetryKindSpawn, EnemyTelemetryStageInitialFill, EnemyTelemetryResultAttempted, EnemyTelemetryReasonRequested, "test_drone", "periodic")
	requireEnemyTelemetry(t, initial.EnemyTelemetry, EnemyTelemetryKindSpawn, EnemyTelemetryStageInitialFill, EnemyTelemetryResultSpawned, EnemyTelemetryReasonNone, "test_drone", "periodic")
	assertEnemyTelemetrySafe(t, initial.EnemyTelemetry)

	record := zoneWorker.EnemySpawnSnapshot().Records[0]
	killedAt := clock.Now().Add(time.Second)
	killed := tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    killedAt,
	})
	assertNoCommandErrors(t, killed)
	requireEnemyTelemetry(t, killed.EnemyTelemetry, EnemyTelemetryKindDeath, EnemyTelemetryStageCommand, EnemyTelemetryResultAccepted, EnemyTelemetryReasonNone, "test_drone", "periodic")
	assertEnemyTelemetrySafe(t, killed.EnemyTelemetry)

	duplicate := tickSubmitted(t, zoneWorker, MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    killedAt.Add(time.Second),
	})
	assertNoCommandErrors(t, duplicate)
	requireEnemyTelemetry(t, duplicate.EnemyTelemetry, EnemyTelemetryKindDeath, EnemyTelemetryStageCommand, EnemyTelemetryResultDuplicate, EnemyTelemetryReasonAlreadyDead, "test_drone", "unknown")
	assertEnemyTelemetrySafe(t, duplicate.EnemyTelemetry)

	clock.Advance(definition.EnemyPools[0].KillRespawnDelay + time.Second)
	respawned := zoneWorker.Tick()
	assertNoCommandErrors(t, respawned)
	requireEnemyTelemetry(t, respawned.EnemyTelemetry, EnemyTelemetryKindRespawn, EnemyTelemetryStageKillDelay, EnemyTelemetryResultDue, EnemyTelemetryReasonNone, "test_drone", "periodic")
	requireEnemyTelemetry(t, respawned.EnemyTelemetry, EnemyTelemetryKindRespawn, EnemyTelemetryStageKillDelay, EnemyTelemetryResultRestored, EnemyTelemetryReasonNone, "test_drone", "periodic")
	assertEnemyTelemetrySafe(t, respawned.EnemyTelemetry)
}

func TestEnemyLifecycleTelemetryReportsAggroAcquireAndClearWithSafeLabels(t *testing.T) {
	definition := aggressiveEnemyMapDefinition()
	zoneWorker := newWorkerForMapDefinition(t, definition)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", definition.SpawnAreas[0].Center, 100)

	initial := tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition})
	assertNoCommandErrors(t, initial)
	requireEnemyTelemetry(t, initial.EnemyTelemetry, EnemyTelemetryKindAggro, EnemyTelemetryStageTargeting, EnemyTelemetryResultAcquired, EnemyTelemetryReasonInRange, "test_drone", "unknown")
	assertEnemyTelemetrySafe(t, initial.EnemyTelemetry)

	zoneWorker.playerAggroIneligible["player-1"] = true
	cleared := zoneWorker.Tick()
	assertNoCommandErrors(t, cleared)
	requireEnemyTelemetry(t, cleared.EnemyTelemetry, EnemyTelemetryKindAggro, EnemyTelemetryStageTargeting, EnemyTelemetryResultCleared, EnemyTelemetryReasonTargetIneligible, "test_drone", "unknown")
	assertEnemyTelemetrySafe(t, cleared.EnemyTelemetry)
}

func requireEnemyTelemetry(t *testing.T, events []EnemyLifecycleTelemetry, category, stage, result, reason, npcType, spawnMode string) {
	t.Helper()
	for _, event := range events {
		if event.Category == category &&
			event.Stage == stage &&
			event.Result == result &&
			event.Reason == reason &&
			event.NPCType == npcType &&
			event.SpawnMode == spawnMode {
			return
		}
	}
	t.Fatalf("missing enemy telemetry category=%q stage=%q result=%q reason=%q npc=%q mode=%q in %+v", category, stage, result, reason, npcType, spawnMode, events)
}

func assertEnemyTelemetrySafe(t *testing.T, events []EnemyLifecycleTelemetry) {
	t.Helper()
	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal telemetry: %v", err)
	}
	raw := string(payload)
	for _, forbidden := range []string{
		"test_pool",
		"test_area",
		"test_stat",
		"test_drop",
		"test_loot",
		"entity_npc",
		"entity-player",
		"player-1",
		"pool_id",
		"spawn_area_id",
		"stat_template_id",
		"drop_profile_id",
		"loot_table_id",
		"rng",
		"seed",
		"roll",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("enemy telemetry leaked %q in %s", forbidden, raw)
		}
	}
}
