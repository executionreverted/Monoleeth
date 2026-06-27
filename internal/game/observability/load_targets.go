package observability

// LoadTestTargetKey is a stable identifier for one local load-test target.
type LoadTestTargetKey string

const (
	LoadTestTargetWorldRealtime LoadTestTargetKey = "world_realtime"
)

const (
	LoadMetricWebSocketOutboundBytesPerPlayer = "websocket_outbound_bytes_per_player_sec"
	LoadMetricCommandLatencyMS                = "command_latency_ms"
	LoadMetricDBTransactionLatencyMS          = "db_transaction_latency_ms"
	LoadMetricRedisHitRate                    = "redis_hit_rate"
	LoadMetricNATSEventLagMS                  = "nats_event_lag_ms"
	LoadMetricGCPauseMS                       = "gc_pause_ms"
	LoadMetricCPUPerZoneWorker                = "cpu_per_zone_worker"
	LoadMetricMemoryPerZoneWorker             = "memory_per_zone_worker"
	LoadMetricAOIEntityPayloadsPerSnapshot    = "aoi_entity_payloads_per_snapshot"
	LoadMetricAggroCandidateChecksPerNPCTick  = "aggro_candidate_checks_per_npc_tick"
)

// LoadTestTarget records the expected Phase 12 throughput envelope for one
// executable local or production load test.
type LoadTestTarget struct {
	Key                         LoadTestTargetKey `json:"key"`
	Name                        string            `json:"name"`
	MinOnlinePlayers            int               `json:"min_online_players"`
	MaxOnlinePlayers            int               `json:"max_online_players"`
	MinVisibleEntitiesPerPlayer int               `json:"min_visible_entities_per_player"`
	MaxVisibleEntitiesPerPlayer int               `json:"max_visible_entities_per_player"`
	MinSnapshotHz               int               `json:"min_snapshot_hz"`
	MaxSnapshotHz               int               `json:"max_snapshot_hz"`
	CombatTickHz                int               `json:"combat_tick_hz"`
	Metrics                     []string          `json:"metrics"`
}

var phase12LoadTestTargets = []LoadTestTarget{
	{
		Key:                         LoadTestTargetWorldRealtime,
		Name:                        "World realtime target",
		MinOnlinePlayers:            1500,
		MaxOnlinePlayers:            2000,
		MinVisibleEntitiesPerPlayer: 50,
		MaxVisibleEntitiesPerPlayer: 100,
		MinSnapshotHz:               5,
		MaxSnapshotHz:               10,
		CombatTickHz:                20,
		Metrics: []string{
			MetricZoneTickMS,
			LoadMetricWebSocketOutboundBytesPerPlayer,
			LoadMetricCommandLatencyMS,
			LoadMetricDBTransactionLatencyMS,
			LoadMetricRedisHitRate,
			LoadMetricNATSEventLagMS,
			LoadMetricGCPauseMS,
			LoadMetricCPUPerZoneWorker,
			LoadMetricMemoryPerZoneWorker,
			LoadMetricAOIEntityPayloadsPerSnapshot,
			LoadMetricAggroCandidateChecksPerNPCTick,
		},
	},
}

// Phase12LoadTestTargets returns deterministic clones of local load targets.
func Phase12LoadTestTargets() []LoadTestTarget {
	targets := make([]LoadTestTarget, len(phase12LoadTestTargets))
	for index, target := range phase12LoadTestTargets {
		targets[index] = cloneLoadTestTarget(target)
	}
	return targets
}

func cloneLoadTestTarget(target LoadTestTarget) LoadTestTarget {
	cloned := target
	if len(target.Metrics) > 0 {
		cloned.Metrics = make([]string, len(target.Metrics))
		copy(cloned.Metrics, target.Metrics)
	}
	return cloned
}
