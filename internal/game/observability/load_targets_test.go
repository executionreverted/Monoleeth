package observability

import (
	"fmt"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	"gameproject/internal/game/world/visibility"
)

func TestPhase12LoadTestTargetsCoverExpectedThroughput(t *testing.T) {
	targets := Phase12LoadTestTargets()
	if len(targets) != 1 {
		t.Fatalf("load targets = %d, want 1", len(targets))
	}
	target := targets[0]
	if target.Key != LoadTestTargetWorldRealtime {
		t.Fatalf("load target key = %q, want %q", target.Key, LoadTestTargetWorldRealtime)
	}
	if target.MinOnlinePlayers != 1500 || target.MaxOnlinePlayers != 2000 {
		t.Fatalf("online players = %d-%d, want 1500-2000", target.MinOnlinePlayers, target.MaxOnlinePlayers)
	}
	if target.MinVisibleEntitiesPerPlayer != 50 || target.MaxVisibleEntitiesPerPlayer != 100 {
		t.Fatalf("visible entities/player = %d-%d, want 50-100", target.MinVisibleEntitiesPerPlayer, target.MaxVisibleEntitiesPerPlayer)
	}
	if target.MinSnapshotHz != 5 || target.MaxSnapshotHz != 10 {
		t.Fatalf("snapshot Hz = %d-%d, want 5-10", target.MinSnapshotHz, target.MaxSnapshotHz)
	}
	if target.CombatTickHz != 20 {
		t.Fatalf("combat tick Hz = %d, want 20", target.CombatTickHz)
	}
	assertStringSet(t, target.Metrics, []string{
		MetricZoneTickMS,
		LoadMetricWebSocketOutboundBytesPerPlayer,
		LoadMetricCommandLatencyMS,
		LoadMetricDBTransactionLatencyMS,
		LoadMetricRedisHitRate,
		LoadMetricNATSEventLagMS,
		LoadMetricGCPauseMS,
		LoadMetricCPUPerZoneWorker,
		LoadMetricMemoryPerZoneWorker,
	})
}

func TestPhase12LoadTestTargetsAreCloneSafe(t *testing.T) {
	targets := Phase12LoadTestTargets()
	targets[0].Key = LoadTestTargetKey("mutated")
	targets[0].Metrics[0] = "mutated"

	next := Phase12LoadTestTargets()
	if next[0].Key != LoadTestTargetWorldRealtime {
		t.Fatalf("load target key mutated through returned slice: got %q", next[0].Key)
	}
	if next[0].Metrics[0] == "mutated" {
		t.Fatal("load target metrics mutated through returned slice")
	}
}

func TestPhase12WorldRealtimeLoadSmokeCoversExpectedThroughput(t *testing.T) {
	target := requirePhase12LoadTarget(t, LoadTestTargetWorldRealtime)
	states := phase12WorldRealtimeStates(target)
	recorder := NewMetricRecorder()

	totalSnapshots := 0
	totalVisibleEntities := 0
	var lastViewer visibility.Viewer
	for playerIndex := 0; playerIndex < target.MinOnlinePlayers; playerIndex++ {
		viewer := phase12WorldRealtimeViewer(target, playerIndex)
		lastViewer = viewer
		for snapshotIndex := 0; snapshotIndex < target.MinSnapshotHz; snapshotIndex++ {
			startedAt := time.Now()
			snapshot := aoi.BuildVisibleSnapshot(viewer, states)
			if err := recorder.RecordZoneTickDuration(viewer.WorldID, viewer.ZoneID, time.Since(startedAt)); err != nil {
				t.Fatalf("record zone tick duration: %v", err)
			}
			if got, want := len(snapshot.Entities), target.MinVisibleEntitiesPerPlayer; got != want {
				t.Fatalf("visible entities for player %d snapshot %d = %d, want %d", playerIndex, snapshotIndex, got, want)
			}
			totalSnapshots++
			totalVisibleEntities += len(snapshot.Entities)
		}
	}

	wantSnapshots := target.MinOnlinePlayers * target.MinSnapshotHz
	if totalSnapshots != wantSnapshots {
		t.Fatalf("snapshots = %d, want %d", totalSnapshots, wantSnapshots)
	}
	wantVisibleEntities := wantSnapshots * target.MinVisibleEntitiesPerPlayer
	if totalVisibleEntities != wantVisibleEntities {
		t.Fatalf("visible entity payloads = %d, want %d", totalVisibleEntities, wantVisibleEntities)
	}
	if err := recordPhase12LoadSmokeMetrics(recorder, target, lastViewer, totalSnapshots, totalVisibleEntities); err != nil {
		t.Fatalf("record load smoke metrics: %v", err)
	}
	assertPhase12LoadSmokeMetrics(t, recorder.Snapshot(), target)
}

func requirePhase12LoadTarget(t *testing.T, key LoadTestTargetKey) LoadTestTarget {
	t.Helper()
	for _, target := range Phase12LoadTestTargets() {
		if target.Key == key {
			return target
		}
	}
	t.Fatalf("load target %q not found", key)
	return LoadTestTarget{}
}

func phase12WorldRealtimeViewer(target LoadTestTarget, playerIndex int) visibility.Viewer {
	snapshot := stats.NewStatSnapshot(
		foundation.PlayerID(fmt.Sprintf("player-phase12-load-%04d", playerIndex)),
		"ship-phase12-load",
		1,
		stats.EffectiveStats{
			Exploration: stats.ExplorationStats{
				RadarRange: float64(target.MinVisibleEntitiesPerPlayer + 200),
			},
		},
		time.Unix(1, 0).UTC(),
	)
	return visibility.Viewer{
		WorldID:    "world-1",
		ZoneID:     "zone-1",
		Position:   world.Vec2{X: float64(playerIndex % 10), Y: float64((playerIndex / 10) % 10)},
		RadarRange: visibility.RadarRangeFromStatSnapshot(snapshot),
	}
}

func phase12WorldRealtimeStates(target LoadTestTarget) []aoi.EntityState {
	states := make([]aoi.EntityState, 0, target.MinVisibleEntitiesPerPlayer+2)
	for index := 0; index < target.MinVisibleEntitiesPerPlayer; index++ {
		states = append(states, aoi.EntityState{
			Entity: world.Entity{
				WorldID:  "world-1",
				ZoneID:   "zone-1",
				ID:       world.EntityID(fmt.Sprintf("entity-visible-%04d", index)),
				Type:     world.EntityTypeNPCPlaceholder,
				Position: world.Vec2{X: float64(index + 1), Y: float64(index % 5)},
			},
			Signature:         visibility.EntitySignature(1),
			PublicStatusFlags: []aoi.StatusFlag{"load_smoke"},
			InternalMetadata:  map[string]string{"server_only": "hidden"},
			GameplaySeed:      "server-seed",
			FutureSpawnData:   []string{"future-spawn-candidate"},
		})
	}
	states = append(states,
		aoi.EntityState{
			Entity: world.Entity{
				WorldID:  "world-1",
				ZoneID:   "zone-1",
				ID:       "entity-hidden",
				Type:     world.EntityTypePlanetSignalPlaceholder,
				Position: world.Vec2{X: 1},
			},
			Signature: visibility.EntitySignature(1),
			Hidden:    true,
		},
		aoi.EntityState{
			Entity: world.Entity{
				WorldID:  "world-1",
				ZoneID:   "zone-1",
				ID:       "entity-out-of-range",
				Type:     world.EntityTypeNPCPlaceholder,
				Position: world.Vec2{X: float64(target.MinVisibleEntitiesPerPlayer + 1000)},
			},
			Signature: visibility.EntitySignature(1),
		},
	)
	return states
}

func recordPhase12LoadSmokeMetrics(
	recorder *MetricRecorder,
	target LoadTestTarget,
	viewer visibility.Viewer,
	totalSnapshots int,
	totalVisibleEntities int,
) error {
	labels := Labels{"target": string(target.Key)}
	if err := recorder.RecordVisibleEntityCount(viewer.WorldID, viewer.ZoneID, int64(target.MinVisibleEntitiesPerPlayer)); err != nil {
		return err
	}
	if err := recorder.AddCounter(MetricCommandsPerSecond, Labels{"op": "snapshot"}, int64(totalSnapshots)); err != nil {
		return err
	}
	if err := recorder.SetGauge(LoadMetricWebSocketOutboundBytesPerPlayer, labels, int64(totalVisibleEntities/target.MinOnlinePlayers*64)); err != nil {
		return err
	}
	if err := recorder.ObserveDuration(LoadMetricCommandLatencyMS, labels, time.Millisecond); err != nil {
		return err
	}
	if err := recorder.ObserveDuration(LoadMetricDBTransactionLatencyMS, labels, time.Millisecond); err != nil {
		return err
	}
	if err := recorder.SetGauge(LoadMetricRedisHitRate, labels, 100); err != nil {
		return err
	}
	if err := recorder.ObserveDuration(LoadMetricNATSEventLagMS, labels, time.Millisecond); err != nil {
		return err
	}
	if err := recorder.ObserveDuration(LoadMetricGCPauseMS, labels, time.Millisecond); err != nil {
		return err
	}
	if err := recorder.SetGauge(LoadMetricCPUPerZoneWorker, labels, 1); err != nil {
		return err
	}
	return recorder.SetGauge(LoadMetricMemoryPerZoneWorker, labels, 1)
}

func assertPhase12LoadSmokeMetrics(t *testing.T, snapshot MetricSnapshot, target LoadTestTarget) {
	t.Helper()
	for _, metric := range target.Metrics {
		if !metricSnapshotHasSeries(snapshot, metric) {
			t.Fatalf("load smoke metric %q has no recorded series", metric)
		}
	}
	if !metricSnapshotHasSeries(snapshot, MetricVisibleEntityCount) {
		t.Fatalf("load smoke metric %q has no recorded series", MetricVisibleEntityCount)
	}
	if !metricSnapshotHasSeries(snapshot, MetricCommandsPerSecond) {
		t.Fatalf("load smoke metric %q has no recorded series", MetricCommandsPerSecond)
	}
}

func metricSnapshotHasSeries(snapshot MetricSnapshot, name string) bool {
	for _, counter := range snapshot.Counters {
		if counter.Name == name {
			return true
		}
	}
	for _, gauge := range snapshot.Gauges {
		if gauge.Name == name {
			return true
		}
	}
	for _, duration := range snapshot.Durations {
		if duration.Name == name {
			return true
		}
	}
	return false
}
