package server

import (
	"gameproject/internal/game/combat"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/quests"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

func (runtime *Runtime) recordCurrencyLedgerMetric(entry economy.CurrencyLedgerEntry) {
	if runtime == nil || runtime.Metrics == nil || entry.LedgerID.IsZero() {
		return
	}
	_ = runtime.Metrics.RecordWalletDelta(
		entry.Reason.String(),
		entry.Currency.String(),
		entry.Action.String(),
		entry.Amount.Int64(),
	)
}

func (runtime *Runtime) recordItemLedgerMetrics(entries []economy.ItemLedgerEntry) {
	if runtime == nil || runtime.Metrics == nil {
		return
	}
	for _, entry := range entries {
		if entry.LedgerID.IsZero() {
			continue
		}
		_ = runtime.Metrics.RecordItemDelta(
			entry.Reason.String(),
			entry.ItemID,
			entry.Action.String(),
			entry.Quantity.Int64(),
		)
	}
}

func (runtime *Runtime) recordQuestRewardMetrics(result quests.ClaimRewardResult) {
	if runtime == nil || runtime.Metrics == nil || result.Duplicate {
		return
	}
	for _, grant := range result.Quest.RewardPayload.Grants {
		_ = runtime.Metrics.RecordQuestReward(grant.Kind.String())
	}
	itemReason := runtimeQuestRewardLedgerReason
	if result.Credits != nil {
		itemReason = result.Credits.LedgerEntry.Reason
		runtime.recordCurrencyLedgerMetric(result.Credits.LedgerEntry)
	}
	if result.Items != nil {
		for _, item := range result.Items.Items {
			_ = runtime.Metrics.RecordItemDelta(itemReason.String(), item.ItemID, economy.LedgerActionIncrease.String(), item.Quantity)
		}
	}
}

func (runtime *Runtime) submitWorkerCommandAndRecordMetricsLocked(instance *mapInstance, command worker.Command) error {
	if instance == nil || instance.Worker == nil {
		return errMapInstanceNotFound
	}
	if err := instance.Worker.Submit(command); err != nil {
		return err
	}
	result := instance.Worker.Tick()
	runtime.recordEnemyTelemetryLocked(instance, result)
	return commandErrors(result)
}

func (runtime *Runtime) recordEnemyTelemetryLocked(instance *mapInstance, result worker.TickResult) {
	if runtime == nil || runtime.Metrics == nil || instance == nil {
		return
	}
	for _, event := range result.EnemyTelemetry {
		runtime.recordEnemyLifecycleMetricLocked(instance.Definition, event)
	}
}

func (runtime *Runtime) recordEnemyLifecycleMetricLocked(definition worldmaps.MapDefinition, event worker.EnemyLifecycleTelemetry) {
	if runtime == nil || runtime.Metrics == nil {
		return
	}
	switch event.Category {
	case worker.EnemyTelemetryKindSpawn:
		_ = runtime.Metrics.RecordEnemySpawnDecision(
			definition.WorldID,
			definition.ZoneID,
			definition.PublicMapKey.String(),
			definition.RiskBand,
			event.Stage,
			event.Result,
			event.Reason,
			event.NPCType,
			event.SpawnMode,
		)
	case worker.EnemyTelemetryKindRespawn:
		_ = runtime.Metrics.RecordEnemyRespawnDecision(
			definition.WorldID,
			definition.ZoneID,
			definition.PublicMapKey.String(),
			definition.RiskBand,
			event.Stage,
			event.Result,
			event.Reason,
			event.NPCType,
			event.SpawnMode,
		)
	case worker.EnemyTelemetryKindDeath:
		_ = runtime.Metrics.RecordEnemyDeathAccounting(
			definition.WorldID,
			definition.ZoneID,
			definition.PublicMapKey.String(),
			definition.RiskBand,
			event.Stage,
			event.Result,
			event.Reason,
			event.NPCType,
		)
	case worker.EnemyTelemetryKindAggro:
		_ = runtime.Metrics.RecordEnemyAggroDecision(
			definition.WorldID,
			definition.ZoneID,
			definition.PublicMapKey.String(),
			definition.RiskBand,
			event.Stage,
			event.Result,
			event.Reason,
			event.NPCType,
		)
	case worker.EnemyTelemetryKindSpawnerRejection:
		_ = runtime.Metrics.RecordEnemySpawnerCommandRejection(
			definition.WorldID,
			definition.ZoneID,
			definition.PublicMapKey.String(),
			definition.RiskBand,
			event.Stage,
			event.Result,
			event.Reason,
		)
	}
}

func (runtime *Runtime) recordNPCLootSelectorMetricLocked(instance *mapInstance, event combat.NPCKilledEvent, stage, result, reason string) {
	if runtime == nil || runtime.Metrics == nil {
		return
	}
	worldID := event.WorldID
	zoneID := event.ZoneID
	mapKey := "unknown"
	riskBand := "unknown"
	if instance != nil {
		worldID = instance.Definition.WorldID
		zoneID = instance.Definition.ZoneID
		mapKey = instance.Definition.PublicMapKey.String()
		riskBand = instance.Definition.RiskBand
	}
	_ = runtime.Metrics.RecordNPCLootSelectorDecision(
		worldID,
		zoneID,
		mapKey,
		riskBand,
		stage,
		result,
		reason,
		event.NPCType,
	)
}
