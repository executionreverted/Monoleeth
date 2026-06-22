package server

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gameproject/internal/game/observability"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

func TestRuntimeRecordsEnemySpawnDeathAndRespawnMetricsWithSafeLabels(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	gameServer.runtime.mu.Lock()
	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("starter map instance: %v", err)
	}
	definition := starter.Definition
	definition.EnemyPools[0].KillRespawnDelay = 0
	starter.Definition = definition
	if err := gameServer.runtime.submitWorkerCommandAndRecordMetricsLocked(starter, worker.InitializeEnemyPoolsCommand{Definition: definition}); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("refresh enemy tick definition: %v", err)
	}
	record, ok := starter.Worker.EnemySpawnRecord("entity_training_npc")
	if !ok {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("starter spawn record missing; snapshot=%+v", starter.Worker.EnemySpawnSnapshot())
	}
	if err := gameServer.runtime.submitWorkerCommandAndRecordMetricsLocked(starter, worker.MarkEnemyKilledCommand{
		Definition:  definition,
		NPCEntityID: record.EntityID,
		KilledAt:    gameServer.runtime.clock.Now(),
	}); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("MarkEnemyKilledCommand() error = %v, want nil", err)
	}
	gameServer.runtime.mu.Unlock()

	gameServer.runtime.tickAndCollectAOIEvents()

	snapshot := gameServer.runtime.Metrics.Snapshot()
	requireMetricCounter(t, snapshot, observability.MetricEnemySpawnDecisions, 1, []observability.Label{
		{Name: "map_key", Value: "1-1"},
		{Name: "npc_type", Value: "training_drone"},
		{Name: "reason", Value: "none"},
		{Name: "result", Value: "spawned"},
		{Name: "risk_band", Value: "low"},
		{Name: "spawn_mode", Value: "periodic"},
		{Name: "stage", Value: "initial_fill"},
		{Name: "world_id", Value: "world-1"},
		{Name: "zone_id", Value: "map_1_1"},
	})
	requireMetricCounter(t, snapshot, observability.MetricEnemyDeathAccounting, 1, []observability.Label{
		{Name: "map_key", Value: "1-1"},
		{Name: "npc_type", Value: "training_drone"},
		{Name: "reason", Value: "none"},
		{Name: "result", Value: "accepted"},
		{Name: "risk_band", Value: "low"},
		{Name: "stage", Value: "command"},
		{Name: "world_id", Value: "world-1"},
		{Name: "zone_id", Value: "map_1_1"},
	})
	requireMetricCounter(t, snapshot, observability.MetricEnemyRespawnDecisions, 1, []observability.Label{
		{Name: "map_key", Value: "1-1"},
		{Name: "npc_type", Value: "training_drone"},
		{Name: "reason", Value: "none"},
		{Name: "result", Value: "restored"},
		{Name: "risk_band", Value: "low"},
		{Name: "spawn_mode", Value: "periodic"},
		{Name: "stage", Value: "kill_delay"},
		{Name: "world_id", Value: "world-1"},
		{Name: "zone_id", Value: "map_1_1"},
	})
	assertMetricSnapshotDoesNotLeakEnemyHiddenIDs(t, snapshot)
}

func TestRuntimeRecordsAggroAndOwnershipRejectionMetricsWithSafeLabels(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	target := createResolvedRuntimeSession(t, gameServer, "aggro-metric-target@example.com", "Aggro Metric")
	viewer := createResolvedRuntimeSession(t, gameServer, "aggro-metric-viewer@example.com", "Aggro Metric Viewer")

	gameServer.runtime.mu.Lock()
	installAggressiveStarterNPCForAggroVisibilityTestLocked(t, gameServer, target.PlayerID, world.Vec2{X: 850, Y: 400}, viewer.PlayerID, world.Vec2{X: 800, Y: 150})
	gameServer.runtime.mu.Unlock()

	if err := gameServer.runtime.setPlayerStealth(target.PlayerID, true); err != nil {
		t.Fatalf("setPlayerStealth(true) error = %v, want nil", err)
	}

	gameServer.runtime.mu.Lock()
	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("starter map instance: %v", err)
	}
	badDefinition := starter.Definition
	badDefinition.InternalMapID = "map_1_2"
	err = gameServer.runtime.submitWorkerCommandAndRecordMetricsLocked(starter, worker.InitializeEnemyPoolsCommand{Definition: badDefinition})
	gameServer.runtime.mu.Unlock()
	if !errors.Is(err, worker.ErrInvalidWorkerConfig) {
		t.Fatalf("InitializeEnemyPoolsCommand ownership error = %v, want %v", err, worker.ErrInvalidWorkerConfig)
	}

	snapshot := gameServer.runtime.Metrics.Snapshot()
	requireMetricCounter(t, snapshot, observability.MetricEnemyAggroDecisions, 1, []observability.Label{
		{Name: "map_key", Value: "1-1"},
		{Name: "npc_type", Value: "training_drone"},
		{Name: "reason", Value: "target_in_range"},
		{Name: "result", Value: "acquired"},
		{Name: "risk_band", Value: "low"},
		{Name: "stage", Value: "targeting"},
		{Name: "world_id", Value: "world-1"},
		{Name: "zone_id", Value: "map_1_1"},
	})
	requireMetricCounter(t, snapshot, observability.MetricEnemySpawnerCommandRejections, 1, []observability.Label{
		{Name: "map_key", Value: "1-1"},
		{Name: "reason", Value: "ownership"},
		{Name: "result", Value: "rejected"},
		{Name: "risk_band", Value: "low"},
		{Name: "stage", Value: "initial_fill"},
		{Name: "world_id", Value: "world-1"},
		{Name: "zone_id", Value: "map_1_1"},
	})
	assertMetricSnapshotDoesNotLeakEnemyHiddenIDs(t, snapshot)
}

func assertMetricSnapshotDoesNotLeakEnemyHiddenIDs(t *testing.T, snapshot observability.MetricSnapshot) {
	t.Helper()

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal metric snapshot: %v", err)
	}
	payload := string(raw)
	for _, forbidden := range []string{
		"starter_training_drone_pool",
		"starter_training_drone_area",
		"training_drone_level_1",
		"training_drone_salvage",
		"entity_training_npc",
		"aggro-metric-target",
		"aggro-metric-viewer",
		"player_id",
		"session_id",
		"loot_table_id",
		"drop_profile_id",
		"stat_template_id",
		"spawn_area_id",
		"event_spawn_id",
		"seed",
		"roll",
	} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("metric snapshot leaked %q in %s", forbidden, payload)
		}
	}
}
