package worker

import (
	"errors"
	"strings"

	worldmaps "gameproject/internal/game/world/maps"
)

const (
	EnemyTelemetryCategorySpawn            = "spawn"
	EnemyTelemetryCategoryRespawn          = "respawn"
	EnemyTelemetryCategoryDeath            = "death"
	EnemyTelemetryCategoryAggro            = "aggro"
	EnemyTelemetryCategorySpawnerRejection = "spawner_rejection"

	EnemyTelemetryKindSpawn            = EnemyTelemetryCategorySpawn
	EnemyTelemetryKindRespawn          = EnemyTelemetryCategoryRespawn
	EnemyTelemetryKindDeath            = EnemyTelemetryCategoryDeath
	EnemyTelemetryKindAggro            = EnemyTelemetryCategoryAggro
	EnemyTelemetryKindSpawnerRejection = EnemyTelemetryCategorySpawnerRejection
)

const (
	enemyTelemetryResultAttempted = "attempted"
	enemyTelemetryResultSpawned   = "spawned"
	enemyTelemetryResultSkipped   = "skipped"
	enemyTelemetryResultRestored  = "restored"
	enemyTelemetryResultAccepted  = "accepted"
	enemyTelemetryResultDuplicate = "duplicate"
	enemyTelemetryResultUnknown   = "unknown"
	enemyTelemetryResultAcquired  = "acquired"
	enemyTelemetryResultCleared   = "cleared"
	enemyTelemetryResultReset     = "reset"
	enemyTelemetryResultRejected  = "rejected"
	enemyTelemetryResultDue       = "due"

	EnemyTelemetryResultAttempted = enemyTelemetryResultAttempted
	EnemyTelemetryResultSpawned   = enemyTelemetryResultSpawned
	EnemyTelemetryResultSkipped   = enemyTelemetryResultSkipped
	EnemyTelemetryResultRestored  = enemyTelemetryResultRestored
	EnemyTelemetryResultAccepted  = enemyTelemetryResultAccepted
	EnemyTelemetryResultDuplicate = enemyTelemetryResultDuplicate
	EnemyTelemetryResultUnknown   = enemyTelemetryResultUnknown
	EnemyTelemetryResultAcquired  = enemyTelemetryResultAcquired
	EnemyTelemetryResultCleared   = enemyTelemetryResultCleared
	EnemyTelemetryResultReset     = enemyTelemetryResultReset
	EnemyTelemetryResultRejected  = enemyTelemetryResultRejected
	EnemyTelemetryResultDue       = enemyTelemetryResultDue
)

const (
	enemyTelemetryStageInitialFill  = "initial_fill"
	enemyTelemetryStagePeriodicFill = "periodic_fill"
	enemyTelemetryStageKillRespawn  = "kill_respawn"
	enemyTelemetryStageEventSpawn   = "event_spawn"
	enemyTelemetryStageDeath        = "death"
	enemyTelemetryStageAggroTick    = "aggro_tick"
	enemyTelemetryStageTickSpawner  = "tick_spawner"
	enemyTelemetryStageTickAggro    = "tick_aggro"

	EnemyTelemetryStageInitialFill  = enemyTelemetryStageInitialFill
	EnemyTelemetryStagePeriodicFill = enemyTelemetryStagePeriodicFill
	EnemyTelemetryStageKillRespawn  = enemyTelemetryStageKillRespawn
	EnemyTelemetryStageEventSpawn   = enemyTelemetryStageEventSpawn
	EnemyTelemetryStageDeath        = enemyTelemetryStageDeath
	EnemyTelemetryStageAggroTick    = enemyTelemetryStageAggroTick
	EnemyTelemetryStageTickSpawner  = enemyTelemetryStageTickSpawner
	EnemyTelemetryStageTickAggro    = enemyTelemetryStageTickAggro
	EnemyTelemetryStageCommand      = "command"
	EnemyTelemetryStageTargeting    = "targeting"
	EnemyTelemetryStageKillDelay    = "kill_delay"
)

const (
	enemyTelemetryReasonNone                = "none"
	enemyTelemetryReasonRequested           = "requested"
	enemyTelemetryReasonCap                 = "cap"
	enemyTelemetryReasonForbiddenCandidate  = "forbidden_candidate"
	enemyTelemetryReasonDisabled            = "disabled"
	enemyTelemetryReasonNotDue              = "not_due"
	enemyTelemetryReasonSpawnMode           = "spawn_mode"
	enemyTelemetryReasonOwnership           = "ownership"
	enemyTelemetryReasonUnknownEntity       = "unknown_entity"
	enemyTelemetryReasonTargetInRange       = "target_in_range"
	enemyTelemetryReasonNoTarget            = "no_target"
	enemyTelemetryReasonPassiveProfile      = "passive_profile"
	enemyTelemetryReasonSafeZone            = "safe_zone"
	enemyTelemetryReasonTargetUnavailable   = "target_unavailable"
	enemyTelemetryReasonLeashBreak          = "leash_break"
	enemyTelemetryReasonTargetMemoryExpired = "target_memory_expired"

	EnemyTelemetryReasonNone                = enemyTelemetryReasonNone
	EnemyTelemetryReasonRequested           = enemyTelemetryReasonRequested
	EnemyTelemetryReasonCap                 = enemyTelemetryReasonCap
	EnemyTelemetryReasonForbiddenCandidate  = enemyTelemetryReasonForbiddenCandidate
	EnemyTelemetryReasonDisabled            = enemyTelemetryReasonDisabled
	EnemyTelemetryReasonNotDue              = enemyTelemetryReasonNotDue
	EnemyTelemetryReasonSpawnMode           = enemyTelemetryReasonSpawnMode
	EnemyTelemetryReasonOwnership           = enemyTelemetryReasonOwnership
	EnemyTelemetryReasonUnknownEntity       = enemyTelemetryReasonUnknownEntity
	EnemyTelemetryReasonTargetInRange       = enemyTelemetryReasonTargetInRange
	EnemyTelemetryReasonNoTarget            = enemyTelemetryReasonNoTarget
	EnemyTelemetryReasonPassiveProfile      = enemyTelemetryReasonPassiveProfile
	EnemyTelemetryReasonSafeZone            = enemyTelemetryReasonSafeZone
	EnemyTelemetryReasonTargetUnavailable   = enemyTelemetryReasonTargetUnavailable
	EnemyTelemetryReasonLeashBreak          = enemyTelemetryReasonLeashBreak
	EnemyTelemetryReasonTargetMemoryExpired = enemyTelemetryReasonTargetMemoryExpired
	EnemyTelemetryReasonPoolCap             = "pool_cap"
	EnemyTelemetryReasonMapCap              = "map_cap"
	EnemyTelemetryReasonEventCap            = "event_cap"
	EnemyTelemetryReasonEventScheduled      = "event_scheduled"
	EnemyTelemetryReasonAlreadyDead         = "already_dead"
	EnemyTelemetryReasonDeath               = "death"
	EnemyTelemetryReasonInvalid             = "invalid"
	EnemyTelemetryReasonPassive             = enemyTelemetryReasonPassiveProfile
	EnemyTelemetryReasonInRange             = enemyTelemetryReasonTargetInRange
	EnemyTelemetryReasonTargetMissing       = "target_missing"
	EnemyTelemetryReasonTargetIneligible    = "target_ineligible"
)

const enemyTelemetryUnknown = "unknown"

// EnemyLifecycleTelemetry is server-only worker telemetry for safe lifecycle
// counters. It intentionally carries only enum-like labels and public content
// categories, never pool ids, spawn area ids, entity ids, player ids, positions,
// RNG seed material, or loot table/profile ids.
type EnemyLifecycleTelemetry struct {
	Category  string
	Stage     string
	Result    string
	Reason    string
	NPCType   string
	SpawnMode string
}

func (worker *Worker) resetEnemyLifecycleTelemetry() {
	if len(worker.enemyTelemetry) == 0 {
		return
	}
	clear(worker.enemyTelemetry)
	worker.enemyTelemetry = worker.enemyTelemetry[:0]
}

func (worker *Worker) enemyLifecycleTelemetrySnapshot() []EnemyLifecycleTelemetry {
	if len(worker.enemyTelemetry) == 0 {
		return nil
	}
	return append([]EnemyLifecycleTelemetry(nil), worker.enemyTelemetry...)
}

func (worker *Worker) recordEnemySpawnDecision(pool worldmaps.MapEnemyPoolDefinition, stage string, result string, reason string) {
	worker.recordEnemyLifecycleTelemetry(EnemyLifecycleTelemetry{
		Category:  EnemyTelemetryCategorySpawn,
		Stage:     stage,
		Result:    result,
		Reason:    reason,
		NPCType:   pool.NPCType,
		SpawnMode: string(pool.SpawnMode),
	})
}

func (worker *Worker) recordEnemyRespawnDecision(record EnemySpawnRecord, spawnMode worldmaps.SpawnMode, result string, reason string) {
	worker.recordEnemyLifecycleTelemetry(EnemyLifecycleTelemetry{
		Category:  EnemyTelemetryCategoryRespawn,
		Stage:     enemyTelemetryStageKillRespawn,
		Result:    result,
		Reason:    reason,
		NPCType:   record.NPCType,
		SpawnMode: string(spawnMode),
	})
}

func (worker *Worker) recordEnemyDeathDecision(record EnemySpawnRecord, spawnMode worldmaps.SpawnMode, result string, reason string) {
	worker.recordEnemyLifecycleTelemetry(EnemyLifecycleTelemetry{
		Category:  EnemyTelemetryCategoryDeath,
		Stage:     enemyTelemetryStageDeath,
		Result:    result,
		Reason:    reason,
		NPCType:   record.NPCType,
		SpawnMode: string(spawnMode),
	})
}

func (worker *Worker) recordUnknownEnemyDeathDecision() {
	worker.recordEnemyLifecycleTelemetry(EnemyLifecycleTelemetry{
		Category: EnemyTelemetryCategoryDeath,
		Stage:    enemyTelemetryStageDeath,
		Result:   enemyTelemetryResultUnknown,
		Reason:   enemyTelemetryReasonUnknownEntity,
	})
}

func (worker *Worker) recordEnemyAggroDecision(record EnemySpawnRecord, result string, reason string) {
	worker.recordEnemyLifecycleTelemetry(EnemyLifecycleTelemetry{
		Category: EnemyTelemetryCategoryAggro,
		Stage:    enemyTelemetryStageAggroTick,
		Result:   result,
		Reason:   reason,
		NPCType:  record.NPCType,
	})
}

func (worker *Worker) recordEnemySpawnerRejection(stage string, reason string) {
	worker.recordEnemyLifecycleTelemetry(EnemyLifecycleTelemetry{
		Category: EnemyTelemetryCategorySpawnerRejection,
		Stage:    stage,
		Result:   enemyTelemetryResultRejected,
		Reason:   reason,
	})
}

func (worker *Worker) recordEnemyTelemetry(category string, stage string, result string, reason string, npcType string, spawnMode string) {
	worker.recordEnemyLifecycleTelemetry(EnemyLifecycleTelemetry{
		Category:  category,
		Stage:     stage,
		Result:    result,
		Reason:    normalizeEnemyTelemetryReason(stage, result, reason),
		NPCType:   npcType,
		SpawnMode: spawnMode,
	})
}

func (worker *Worker) recordEnemySpawnerCommandRejection(stage string, err error) {
	if err == nil {
		return
	}
	reason := EnemyTelemetryReasonInvalid
	if errors.Is(err, ErrInvalidWorkerConfig) {
		reason = enemyTelemetryReasonOwnership
	}
	worker.recordEnemySpawnerRejection(stage, reason)
}

func (worker *Worker) recordEnemyLifecycleTelemetry(event EnemyLifecycleTelemetry) {
	event.Stage = enemyTelemetryValueOrUnknown(event.Stage)
	event.Result = enemyTelemetryValueOrUnknown(event.Result)
	event.Reason = enemyTelemetryValueOrNone(event.Reason)
	event.NPCType = enemyTelemetryValueOrUnknown(event.NPCType)
	event.SpawnMode = enemyTelemetryValueOrUnknown(event.SpawnMode)
	worker.enemyTelemetry = append(worker.enemyTelemetry, event)
}

func normalizeEnemyTelemetryReason(stage string, result string, reason string) string {
	reason = strings.TrimSpace(reason)
	switch result {
	case enemyTelemetryResultAttempted:
		if reason == "" || reason == stage {
			return enemyTelemetryReasonRequested
		}
	case enemyTelemetryResultSpawned, enemyTelemetryResultRestored, enemyTelemetryResultAccepted, enemyTelemetryResultDue:
		if reason == "" || reason == stage {
			return enemyTelemetryReasonNone
		}
	case enemyTelemetryResultSkipped:
		if reason == EnemyTelemetryStageKillDelay {
			return enemyTelemetryReasonNotDue
		}
	}
	return reason
}

func enemyTelemetryValueOrUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return enemyTelemetryUnknown
	}
	return value
}

func enemyTelemetryValueOrNone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return enemyTelemetryReasonNone
	}
	return value
}
