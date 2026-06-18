package observability

import "testing"

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
