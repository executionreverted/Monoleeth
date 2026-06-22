package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

// EnemySpawnRecord is server-only map-worker state for one live or known NPC
// spawn row. It is intentionally not part of any client wire projection.
type EnemySpawnRecord struct {
	EntityID       world.EntityID
	EnemyPoolID    worldmaps.EnemyPoolID
	EventSpawnID   worldmaps.NPCEventSpawnID
	SpawnAreaID    worldmaps.SpawnAreaID
	NPCType        string
	Level          int
	StatTemplateID worldmaps.NPCStatTemplateID
	DropProfileID  worldmaps.NPCDropProfileID
	AggroProfileID worldmaps.NPCAggroProfileID
	LeashProfileID worldmaps.NPCLeashProfileID
	Position       world.Vec2
	LeashOrigin    world.Vec2
	Alive          bool
	SpawnedAt      time.Time
	DeadAt         time.Time
	NextRespawnAt  time.Time

	AggroTargetEntityID   world.EntityID
	AggroAcquiredAt       time.Time
	AggroTargetLastSeenAt time.Time
	LastAggroTickAt       time.Time
}

// EnemySpawnSnapshot is a deterministic server-only copy of worker spawner
// state. It must not be serialized to clients.
type EnemySpawnSnapshot struct {
	Records          []EnemySpawnRecord
	PoolAliveCounts  map[worldmaps.EnemyPoolID]int
	InitializedPools []worldmaps.EnemyPoolID
	MapAliveCount    int
}

// InitializeEnemyPoolsCommand performs the Phase08B deterministic initial fill
// from enabled map enemy pools.
type InitializeEnemyPoolsCommand struct {
	Definition        worldmaps.MapDefinition
	EntityIDOverrides map[worldmaps.EnemyPoolID][]world.EntityID
}

func (command InitializeEnemyPoolsCommand) apply(worker *Worker) error {
	err := worker.initializeEnemyPools(command.Definition, command.EntityIDOverrides)
	if err != nil {
		worker.recordEnemySpawnerCommandRejection(EnemyTelemetryStageInitialFill, err)
	}
	return err
}

// MarkEnemyKilledCommand records a spawner-backed NPC death in worker-owned
// lifecycle state and removes the live world entity if it is still present.
type MarkEnemyKilledCommand struct {
	Definition  worldmaps.MapDefinition
	NPCEntityID world.EntityID
	KilledAt    time.Time
}

func (command MarkEnemyKilledCommand) apply(worker *Worker) error {
	err := worker.markEnemyKilled(command.Definition, command.NPCEntityID, command.KilledAt)
	if err != nil {
		worker.recordEnemySpawnerCommandRejection(EnemyTelemetryStageCommand, err)
	}
	return err
}

// TriggerEnemyEventSpawnCommand is a server-owned hook for due boss/event
// spawns. It is not a client command and exposes no event internals to clients.
type TriggerEnemyEventSpawnCommand struct {
	Definition   worldmaps.MapDefinition
	EventSpawnID worldmaps.NPCEventSpawnID
	TriggeredAt  time.Time
}

func (command TriggerEnemyEventSpawnCommand) apply(worker *Worker) error {
	err := worker.triggerEnemyEventSpawn(command.Definition, command.EventSpawnID, command.TriggeredAt)
	if err != nil {
		worker.recordEnemySpawnerCommandRejection(EnemyTelemetryStageEventSpawn, err)
	}
	return err
}

// EnemySpawnSnapshot returns clone-safe server-only spawner state.
func (worker *Worker) EnemySpawnSnapshot() EnemySpawnSnapshot {
	if worker.enemySpawner == nil {
		return EnemySpawnSnapshot{
			Records:          nil,
			PoolAliveCounts:  map[worldmaps.EnemyPoolID]int{},
			InitializedPools: nil,
			MapAliveCount:    0,
		}
	}
	return worker.enemySpawner.snapshot()
}

// EnemySpawnRecord returns a clone-safe server-only copy for entityID.
func (worker *Worker) EnemySpawnRecord(entityID world.EntityID) (EnemySpawnRecord, bool) {
	if worker.enemySpawner == nil {
		return EnemySpawnRecord{}, false
	}
	return worker.enemySpawner.record(entityID)
}

func (worker *Worker) initializeEnemyPools(definition worldmaps.MapDefinition, overrides map[worldmaps.EnemyPoolID][]world.EntityID) error {
	if err := worker.validateEnemyPoolDefinitionOwnership(definition); err != nil {
		return err
	}
	if worker.enemySpawner == nil {
		worker.enemySpawner = newEnemySpawnerState()
	}
	now := worker.clock.Now()
	worker.enemySpawner.configureTicks(definition, now)
	if len(definition.EnemyPools) == 0 {
		return nil
	}

	spawnAreas := spawnAreasByID(definition.SpawnAreas)
	statTemplates := statTemplatesByID(definition.NPCStatTemplates)
	mapAliveCount := worker.enemySpawner.mapAliveCount()
	mapAliveCap, hasMapAliveCap := initialEnemyMapAliveCap(definition.EnemyPools)

	for _, pool := range definition.EnemyPools {
		if worker.enemySpawner.poolInitialized(pool.EnemyPoolID) {
			continue
		}
		if !pool.Enabled || pool.SpawnMode == worldmaps.SpawnModeDisabled || pool.SpawnMode == worldmaps.SpawnModeEventScheduled {
			reason := EnemyTelemetryReasonDisabled
			if pool.Enabled && pool.SpawnMode == worldmaps.SpawnModeEventScheduled {
				reason = EnemyTelemetryReasonEventScheduled
			}
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindSpawn,
				EnemyTelemetryStageInitialFill,
				EnemyTelemetryResultSkipped,
				reason,
				pool.NPCType,
				string(pool.SpawnMode),
			)
			worker.enemySpawner.markPoolInitialized(pool.EnemyPoolID)
			continue
		}
		statTemplate, ok := statTemplates[pool.StatTemplateID]
		if !ok {
			return fmt.Errorf("enemy pool %q stat template %q: %w", pool.EnemyPoolID, pool.StatTemplateID, worldmaps.ErrInvalidCatalog)
		}

		poolAliveCount := worker.enemySpawner.poolAliveCount(pool.EnemyPoolID)
		for spawnIndex := 0; spawnIndex < pool.InitialAlive; spawnIndex++ {
			if poolAliveCount >= pool.PoolMaxAlive {
				worker.recordEnemyTelemetry(
					EnemyTelemetryKindSpawn,
					EnemyTelemetryStageInitialFill,
					EnemyTelemetryResultSkipped,
					EnemyTelemetryReasonPoolCap,
					pool.NPCType,
					string(pool.SpawnMode),
				)
				break
			}
			if hasMapAliveCap && mapAliveCount >= mapAliveCap {
				worker.recordEnemyTelemetry(
					EnemyTelemetryKindSpawn,
					EnemyTelemetryStageInitialFill,
					EnemyTelemetryResultSkipped,
					EnemyTelemetryReasonMapCap,
					pool.NPCType,
					string(pool.SpawnMode),
				)
				break
			}
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindSpawn,
				EnemyTelemetryStageInitialFill,
				EnemyTelemetryResultAttempted,
				EnemyTelemetryStageInitialFill,
				pool.NPCType,
				string(pool.SpawnMode),
			)
			area, ok, err := selectInitialSpawnArea(definition, spawnAreas, pool, spawnIndex)
			if err != nil {
				return err
			}
			if !ok {
				worker.recordEnemyTelemetry(
					EnemyTelemetryKindSpawn,
					EnemyTelemetryStageInitialFill,
					EnemyTelemetryResultSkipped,
					EnemyTelemetryReasonForbiddenCandidate,
					pool.NPCType,
					string(pool.SpawnMode),
				)
				continue
			}

			entityID := enemySpawnEntityID(definition.InternalMapID, pool.EnemyPoolID, spawnIndex, overrides)
			if _, exists := worker.entities[entityID]; exists {
				return fmt.Errorf("enemy spawn entity %q: %w", entityID, ErrEntityAlreadyExists)
			}
			record, err := worker.newEnemySpawnRecord(definition, pool, statTemplate, area, entityID, now)
			if err != nil {
				return err
			}
			worker.enemySpawner.add(record)
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindSpawn,
				EnemyTelemetryStageInitialFill,
				EnemyTelemetryResultSpawned,
				EnemyTelemetryStageInitialFill,
				pool.NPCType,
				string(pool.SpawnMode),
			)
			poolAliveCount++
			mapAliveCount++
		}
		worker.enemySpawner.markPoolInitialized(pool.EnemyPoolID)
	}
	return nil
}

func (worker *Worker) markEnemyKilled(definition worldmaps.MapDefinition, entityID world.EntityID, killedAt time.Time) error {
	if err := worker.validateEnemyPoolDefinitionOwnership(definition); err != nil {
		return err
	}
	if worker.enemySpawner == nil {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindDeath,
			EnemyTelemetryStageCommand,
			EnemyTelemetryResultUnknown,
			EnemyTelemetryReasonUnknownEntity,
			"",
			"",
		)
		return fmt.Errorf("enemy spawn entity %q: %w", entityID, ErrUnknownEntity)
	}

	index, ok := worker.enemySpawner.byEntityID[entityID]
	if !ok {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindDeath,
			EnemyTelemetryStageCommand,
			EnemyTelemetryResultUnknown,
			EnemyTelemetryReasonUnknownEntity,
			"",
			"",
		)
		return fmt.Errorf("enemy spawn entity %q: %w", entityID, ErrUnknownEntity)
	}
	record := worker.enemySpawner.rows[index]
	if !record.Alive {
		worker.removeEntity(entityID)
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindDeath,
			EnemyTelemetryStageCommand,
			EnemyTelemetryResultDuplicate,
			EnemyTelemetryReasonAlreadyDead,
			record.NPCType,
			"",
		)
		return nil
	}

	pools := enemyPoolsByID(definition.EnemyPools)
	pool, ok := pools[record.EnemyPoolID]
	if !ok {
		return fmt.Errorf("enemy pool %q: %w", record.EnemyPoolID, worldmaps.ErrInvalidCatalog)
	}
	if killedAt.IsZero() {
		killedAt = worker.clock.Now()
	}
	if entity, ok := worker.entities[entityID]; ok {
		record.Position = entity.Position
	}
	record.Alive = false
	record.DeadAt = killedAt
	if pool.SpawnMode == worldmaps.SpawnModeEventScheduled {
		record.NextRespawnAt = time.Time{}
	} else {
		record.NextRespawnAt = killedAt.Add(pool.KillRespawnDelay + deterministicSpawnJitter(pool.SpawnJitter, definition.InternalMapID.String(), pool.EnemyPoolID.String(), entityID.String()))
	}
	if !record.AggroTargetEntityID.IsZero() {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindAggro,
			EnemyTelemetryStageTargeting,
			EnemyTelemetryResultReset,
			EnemyTelemetryReasonDeath,
			record.NPCType,
			string(pool.SpawnMode),
		)
	}
	clearEnemyAggroState(&record)
	worker.enemySpawner.rows[index] = record

	if worker.enemySpawner.aliveByPool[record.EnemyPoolID] > 0 {
		worker.enemySpawner.aliveByPool[record.EnemyPoolID]--
	} else {
		worker.enemySpawner.aliveByPool[record.EnemyPoolID] = 0
	}
	worker.removeEntity(entityID)
	worker.recordEnemyTelemetry(
		EnemyTelemetryKindDeath,
		EnemyTelemetryStageCommand,
		EnemyTelemetryResultAccepted,
		EnemyTelemetryReasonNone,
		record.NPCType,
		string(pool.SpawnMode),
	)
	return nil
}

func (worker *Worker) tickEnemySpawner() []CommandError {
	if worker.enemySpawner == nil || !worker.enemySpawner.hasTickDefinition {
		return nil
	}
	if err := worker.tickEnemySpawnerDefinition(worker.enemySpawner.tickDefinition); err != nil {
		return []CommandError{{Index: -1, Err: err}}
	}
	return nil
}

func (worker *Worker) tickEnemySpawnerDefinition(definition worldmaps.MapDefinition) error {
	if err := worker.validateEnemyPoolDefinitionOwnership(definition); err != nil {
		worker.recordEnemySpawnerRejection(EnemyTelemetryStageTickSpawner, EnemyTelemetryReasonOwnership)
		return err
	}
	if len(definition.EnemyPools) == 0 {
		return nil
	}

	spawnAreas := spawnAreasByID(definition.SpawnAreas)
	pools := enemyPoolsByID(definition.EnemyPools)
	statTemplates := statTemplatesByID(definition.NPCStatTemplates)
	now := worker.clock.Now()
	mapAliveCap, hasMapAliveCap := initialEnemyMapAliveCap(definition.EnemyPools)
	mapAliveCount := worker.enemySpawner.mapAliveCount()

	for index := range worker.enemySpawner.rows {
		record := worker.enemySpawner.rows[index]
		if record.Alive || record.NextRespawnAt.IsZero() || now.Before(record.NextRespawnAt) {
			continue
		}
		pool, ok := pools[record.EnemyPoolID]
		if !ok {
			return fmt.Errorf("enemy pool %q: %w", record.EnemyPoolID, worldmaps.ErrInvalidCatalog)
		}
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindRespawn,
			EnemyTelemetryStageKillDelay,
			EnemyTelemetryResultDue,
			EnemyTelemetryStageKillDelay,
			record.NPCType,
			string(pool.SpawnMode),
		)
		if !pool.Enabled || pool.SpawnMode == worldmaps.SpawnModeDisabled || pool.SpawnMode == worldmaps.SpawnModeEventScheduled {
			reason := EnemyTelemetryReasonDisabled
			if pool.Enabled && pool.SpawnMode == worldmaps.SpawnModeEventScheduled {
				reason = EnemyTelemetryReasonEventScheduled
			}
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindRespawn,
				EnemyTelemetryStageKillDelay,
				EnemyTelemetryResultSkipped,
				reason,
				record.NPCType,
				string(pool.SpawnMode),
			)
			continue
		}
		if worker.enemySpawner.poolAliveCount(record.EnemyPoolID) >= pool.PoolMaxAlive {
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindRespawn,
				EnemyTelemetryStageKillDelay,
				EnemyTelemetryResultSkipped,
				EnemyTelemetryReasonPoolCap,
				record.NPCType,
				string(pool.SpawnMode),
			)
			continue
		}
		if hasMapAliveCap && mapAliveCount >= mapAliveCap {
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindRespawn,
				EnemyTelemetryStageKillDelay,
				EnemyTelemetryResultSkipped,
				EnemyTelemetryReasonMapCap,
				record.NPCType,
				string(pool.SpawnMode),
			)
			continue
		}
		statTemplate, ok := statTemplates[record.StatTemplateID]
		if !ok {
			return fmt.Errorf("enemy pool %q stat template %q: %w", record.EnemyPoolID, record.StatTemplateID, worldmaps.ErrInvalidCatalog)
		}
		area, ok := spawnAreas[record.SpawnAreaID]
		if !ok {
			return fmt.Errorf("enemy pool %q spawn area %q: %w", record.EnemyPoolID, record.SpawnAreaID, worldmaps.ErrInvalidCatalog)
		}
		if !validSpawnCandidate(definition, area, area.Center) {
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindRespawn,
				EnemyTelemetryStageKillDelay,
				EnemyTelemetryResultSkipped,
				EnemyTelemetryReasonForbiddenCandidate,
				record.NPCType,
				string(pool.SpawnMode),
			)
			continue
		}
		if err := worker.respawnEnemyRecord(index, definition, statTemplate, area, now); err != nil {
			return err
		}
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindRespawn,
			EnemyTelemetryStageKillDelay,
			EnemyTelemetryResultRestored,
			EnemyTelemetryStageKillDelay,
			record.NPCType,
			string(pool.SpawnMode),
		)
		mapAliveCount++
	}

	mapReservedCount := worker.enemySpawner.mapReservedCount()
	for _, pool := range definition.EnemyPools {
		if !pool.Enabled || pool.SpawnMode != worldmaps.SpawnModePeriodic {
			continue
		}
		lastFillAt := worker.enemySpawner.lastPeriodicFillAt(pool.EnemyPoolID)
		nextFillAt := lastFillAt.Add(pool.SpawnInterval + deterministicSpawnJitter(pool.SpawnJitter, definition.InternalMapID.String(), pool.EnemyPoolID.String(), "periodic"))
		if now.Before(nextFillAt) {
			continue
		}
		if worker.enemySpawner.poolReservedCount(pool.EnemyPoolID) >= pool.PoolMaxAlive {
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindSpawn,
				EnemyTelemetryStagePeriodicFill,
				EnemyTelemetryResultSkipped,
				EnemyTelemetryReasonPoolCap,
				pool.NPCType,
				string(pool.SpawnMode),
			)
			continue
		}
		if hasMapAliveCap && mapReservedCount >= mapAliveCap {
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindSpawn,
				EnemyTelemetryStagePeriodicFill,
				EnemyTelemetryResultSkipped,
				EnemyTelemetryReasonMapCap,
				pool.NPCType,
				string(pool.SpawnMode),
			)
			continue
		}
		worker.enemySpawner.setLastPeriodicFillAt(pool.EnemyPoolID, now)
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindSpawn,
			EnemyTelemetryStagePeriodicFill,
			EnemyTelemetryResultAttempted,
			EnemyTelemetryStagePeriodicFill,
			pool.NPCType,
			string(pool.SpawnMode),
		)

		statTemplate, ok := statTemplates[pool.StatTemplateID]
		if !ok {
			return fmt.Errorf("enemy pool %q stat template %q: %w", pool.EnemyPoolID, pool.StatTemplateID, worldmaps.ErrInvalidCatalog)
		}
		spawnIndex := worker.enemySpawner.rowCountForPool(pool.EnemyPoolID)
		area, ok, err := selectSpawnArea(definition, spawnAreas, pool, spawnIndex)
		if err != nil {
			return err
		}
		if !ok {
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindSpawn,
				EnemyTelemetryStagePeriodicFill,
				EnemyTelemetryResultSkipped,
				EnemyTelemetryReasonForbiddenCandidate,
				pool.NPCType,
				string(pool.SpawnMode),
			)
			continue
		}
		entityID := enemySpawnEntityID(definition.InternalMapID, pool.EnemyPoolID, spawnIndex, nil)
		if _, exists := worker.enemySpawner.byEntityID[entityID]; exists {
			return fmt.Errorf("enemy spawn entity %q: %w", entityID, ErrEntityAlreadyExists)
		}
		record, err := worker.newEnemySpawnRecord(definition, pool, statTemplate, area, entityID, now)
		if err != nil {
			return err
		}
		worker.enemySpawner.add(record)
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindSpawn,
			EnemyTelemetryStagePeriodicFill,
			EnemyTelemetryResultSpawned,
			EnemyTelemetryStagePeriodicFill,
			pool.NPCType,
			string(pool.SpawnMode),
		)
		mapAliveCount++
		mapReservedCount++
	}
	return nil
}

func (worker *Worker) newEnemySpawnRecord(
	definition worldmaps.MapDefinition,
	pool worldmaps.MapEnemyPoolDefinition,
	statTemplate worldmaps.NPCStatTemplate,
	area worldmaps.MapSpawnAreaDefinition,
	entityID world.EntityID,
	spawnedAt time.Time,
) (EnemySpawnRecord, error) {
	entity, err := world.NewEntity(definition.WorldID, definition.ZoneID, entityID, world.EntityTypeNPC, area.Center)
	if err != nil {
		return EnemySpawnRecord{}, err
	}
	if err := worker.insertEntity(entity, statTemplate.Speed); err != nil {
		return EnemySpawnRecord{}, err
	}

	return EnemySpawnRecord{
		EntityID:       entityID,
		EnemyPoolID:    pool.EnemyPoolID,
		SpawnAreaID:    area.SpawnAreaID,
		NPCType:        pool.NPCType,
		Level:          pool.MinLevel,
		StatTemplateID: pool.StatTemplateID,
		DropProfileID:  pool.DropProfileID,
		AggroProfileID: pool.AggroProfileID,
		LeashProfileID: pool.LeashProfileID,
		Position:       area.Center,
		LeashOrigin:    area.Center,
		Alive:          true,
		SpawnedAt:      spawnedAt,
	}, nil
}

func (worker *Worker) triggerEnemyEventSpawn(definition worldmaps.MapDefinition, eventSpawnID worldmaps.NPCEventSpawnID, triggeredAt time.Time) error {
	if err := worker.validateEnemyPoolDefinitionOwnership(definition); err != nil {
		return err
	}
	if worker.enemySpawner == nil {
		worker.enemySpawner = newEnemySpawnerState()
	}
	now := triggeredAt
	if now.IsZero() {
		now = worker.clock.Now()
	}
	if !worker.enemySpawner.hasTickDefinition {
		worker.enemySpawner.configureTicks(definition, now)
	}

	eventSpawns := eventSpawnsByID(definition.NPCEventSpawns)
	eventSpawn, ok := eventSpawns[eventSpawnID]
	if !ok {
		return fmt.Errorf("npc event spawn %q: %w", eventSpawnID, worldmaps.ErrInvalidCatalog)
	}
	if !eventSpawn.Enabled {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindSpawn,
			EnemyTelemetryStageEventSpawn,
			EnemyTelemetryResultSkipped,
			EnemyTelemetryReasonDisabled,
			"",
			string(worldmaps.SpawnModeEventScheduled),
		)
		return nil
	}
	if eventSpawn.MapPolicy != worldmaps.NPCEventSpawnMapPolicyCurrentMapOnly {
		return fmt.Errorf("npc event spawn %q map policy %q: %w", eventSpawn.EventSpawnID, eventSpawn.MapPolicy, worldmaps.ErrInvalidMapDefinition)
	}
	dueAt := worker.enemySpawner.ensureEventDueAt(eventSpawn, now)
	if now.Before(dueAt) {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindSpawn,
			EnemyTelemetryStageEventSpawn,
			EnemyTelemetryResultSkipped,
			EnemyTelemetryStageKillDelay,
			"",
			string(worldmaps.SpawnModeEventScheduled),
		)
		return nil
	}

	pools := enemyPoolsByID(definition.EnemyPools)
	pool, ok := pools[eventSpawn.EnemyPoolID]
	if !ok {
		return fmt.Errorf("npc event spawn %q enemy pool %q: %w", eventSpawn.EventSpawnID, eventSpawn.EnemyPoolID, worldmaps.ErrInvalidCatalog)
	}
	if !pool.Enabled {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindSpawn,
			EnemyTelemetryStageEventSpawn,
			EnemyTelemetryResultSkipped,
			EnemyTelemetryReasonDisabled,
			pool.NPCType,
			string(pool.SpawnMode),
		)
		return nil
	}
	if pool.SpawnMode != worldmaps.SpawnModeEventScheduled {
		return fmt.Errorf("npc event spawn %q enemy pool %q spawn mode %q: %w", eventSpawn.EventSpawnID, pool.EnemyPoolID, pool.SpawnMode, worldmaps.ErrInvalidCatalog)
	}
	if worker.enemySpawner.eventAliveCount(eventSpawn.EventSpawnID) >= eventSpawn.MaxAlive {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindSpawn,
			EnemyTelemetryStageEventSpawn,
			EnemyTelemetryResultSkipped,
			EnemyTelemetryReasonEventCap,
			pool.NPCType,
			string(pool.SpawnMode),
		)
		return nil
	}
	if worker.enemySpawner.poolAliveCount(pool.EnemyPoolID) >= pool.PoolMaxAlive {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindSpawn,
			EnemyTelemetryStageEventSpawn,
			EnemyTelemetryResultSkipped,
			EnemyTelemetryReasonPoolCap,
			pool.NPCType,
			string(pool.SpawnMode),
		)
		return nil
	}
	if mapAliveCap, hasMapAliveCap := enabledEnemyMapAliveCap(definition.EnemyPools); hasMapAliveCap && worker.enemySpawner.mapAliveCount() >= mapAliveCap {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindSpawn,
			EnemyTelemetryStageEventSpawn,
			EnemyTelemetryResultSkipped,
			EnemyTelemetryReasonMapCap,
			pool.NPCType,
			string(pool.SpawnMode),
		)
		return nil
	}

	spawnAreas := spawnAreasByID(definition.SpawnAreas)
	statTemplates := statTemplatesByID(definition.NPCStatTemplates)
	dropProfiles := dropProfilesByID(definition.NPCDropProfiles)
	statTemplate, ok := statTemplates[pool.StatTemplateID]
	if !ok {
		return fmt.Errorf("enemy pool %q stat template %q: %w", pool.EnemyPoolID, pool.StatTemplateID, worldmaps.ErrInvalidCatalog)
	}
	if !statTemplateCompatibleWithPool(pool, statTemplate) {
		return fmt.Errorf("npc event spawn %q stat template %q incompatible with pool %q: %w", eventSpawn.EventSpawnID, statTemplate.StatTemplateID, pool.EnemyPoolID, worldmaps.ErrInvalidCatalog)
	}
	dropProfile, ok := dropProfiles[eventSpawn.DropProfileID]
	if !ok {
		return fmt.Errorf("npc event spawn %q drop profile %q: %w", eventSpawn.EventSpawnID, eventSpawn.DropProfileID, worldmaps.ErrInvalidCatalog)
	}
	if !dropProfileCompatibleWithPool(definition, pool, dropProfile) {
		return fmt.Errorf("npc event spawn %q drop profile %q incompatible with pool %q: %w", eventSpawn.EventSpawnID, dropProfile.DropProfileID, pool.EnemyPoolID, worldmaps.ErrInvalidCatalog)
	}
	spawnIndex := worker.enemySpawner.rowCountForPool(pool.EnemyPoolID)
	worker.recordEnemyTelemetry(
		EnemyTelemetryKindSpawn,
		EnemyTelemetryStageEventSpawn,
		EnemyTelemetryResultAttempted,
		EnemyTelemetryStageEventSpawn,
		pool.NPCType,
		string(pool.SpawnMode),
	)
	area, ok, err := selectSpawnArea(definition, spawnAreas, pool, spawnIndex)
	if err != nil {
		return err
	}
	if !ok {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindSpawn,
			EnemyTelemetryStageEventSpawn,
			EnemyTelemetryResultSkipped,
			EnemyTelemetryReasonForbiddenCandidate,
			pool.NPCType,
			string(pool.SpawnMode),
		)
		return nil
	}

	entityID := enemyEventSpawnEntityID(definition.InternalMapID, pool.EnemyPoolID, eventSpawn.EventSpawnID, spawnIndex)
	if _, exists := worker.enemySpawner.byEntityID[entityID]; exists {
		return fmt.Errorf("enemy spawn entity %q: %w", entityID, ErrEntityAlreadyExists)
	}
	record, err := worker.newEnemySpawnRecord(definition, pool, statTemplate, area, entityID, now)
	if err != nil {
		return err
	}
	record.EventSpawnID = eventSpawn.EventSpawnID
	record.DropProfileID = eventSpawn.DropProfileID
	worker.enemySpawner.add(record)
	worker.recordEnemyTelemetry(
		EnemyTelemetryKindSpawn,
		EnemyTelemetryStageEventSpawn,
		EnemyTelemetryResultSpawned,
		EnemyTelemetryStageEventSpawn,
		pool.NPCType,
		string(pool.SpawnMode),
	)
	return nil
}

func (worker *Worker) respawnEnemyRecord(index int, definition worldmaps.MapDefinition, statTemplate worldmaps.NPCStatTemplate, area worldmaps.MapSpawnAreaDefinition, spawnedAt time.Time) error {
	record := worker.enemySpawner.rows[index]
	entity, err := world.NewEntity(definition.WorldID, definition.ZoneID, record.EntityID, world.EntityTypeNPC, area.Center)
	if err != nil {
		return err
	}
	if err := worker.insertEntity(entity, statTemplate.Speed); err != nil {
		return err
	}
	record.Position = area.Center
	record.LeashOrigin = area.Center
	record.Alive = true
	record.SpawnedAt = spawnedAt
	record.DeadAt = time.Time{}
	record.NextRespawnAt = time.Time{}
	clearEnemyAggroState(&record)
	worker.enemySpawner.rows[index] = record
	worker.enemySpawner.aliveByPool[record.EnemyPoolID]++
	return nil
}

func initialEnemyMapAliveCap(pools []worldmaps.MapEnemyPoolDefinition) (int, bool) {
	cap := 0
	hasCap := false
	for _, pool := range pools {
		if !pool.Enabled || pool.SpawnMode == worldmaps.SpawnModeDisabled || pool.SpawnMode == worldmaps.SpawnModeEventScheduled {
			continue
		}
		if !hasCap || pool.MapMaxAlive < cap {
			cap = pool.MapMaxAlive
			hasCap = true
		}
	}
	return cap, hasCap
}

func enabledEnemyMapAliveCap(pools []worldmaps.MapEnemyPoolDefinition) (int, bool) {
	cap := 0
	hasCap := false
	for _, pool := range pools {
		if !pool.Enabled || pool.SpawnMode == worldmaps.SpawnModeDisabled {
			continue
		}
		if !hasCap || pool.MapMaxAlive < cap {
			cap = pool.MapMaxAlive
			hasCap = true
		}
	}
	return cap, hasCap
}

func (worker *Worker) validateEnemyPoolDefinitionOwnership(definition worldmaps.MapDefinition) error {
	if definition.WorldID != worker.worldID {
		return fmt.Errorf("enemy pools map %q world %q not owned by worker world %q: %w", definition.InternalMapID, definition.WorldID, worker.worldID, ErrInvalidWorkerConfig)
	}
	if err := definition.InternalMapID.Validate(); err != nil {
		return err
	}
	if definition.ZoneID != definition.InternalMapID.ZoneID() {
		return fmt.Errorf("enemy pools map %q zone %q does not match internal map zone %q: %w", definition.InternalMapID, definition.ZoneID, definition.InternalMapID.ZoneID(), ErrInvalidWorkerConfig)
	}
	if definition.ZoneID != worker.zoneID {
		return fmt.Errorf("enemy pools map %q zone %q not owned by worker zone %q: %w", definition.InternalMapID, definition.ZoneID, worker.zoneID, ErrInvalidWorkerConfig)
	}
	if err := definition.Bounds.ValidateExactPlayable(); err != nil {
		return err
	}
	return nil
}

func spawnAreasByID(areas []worldmaps.MapSpawnAreaDefinition) map[worldmaps.SpawnAreaID]worldmaps.MapSpawnAreaDefinition {
	byID := make(map[worldmaps.SpawnAreaID]worldmaps.MapSpawnAreaDefinition, len(areas))
	for _, area := range areas {
		byID[area.SpawnAreaID] = area
	}
	return byID
}

func enemyPoolsByID(pools []worldmaps.MapEnemyPoolDefinition) map[worldmaps.EnemyPoolID]worldmaps.MapEnemyPoolDefinition {
	byID := make(map[worldmaps.EnemyPoolID]worldmaps.MapEnemyPoolDefinition, len(pools))
	for _, pool := range pools {
		byID[pool.EnemyPoolID] = pool
	}
	return byID
}

func statTemplatesByID(templates []worldmaps.NPCStatTemplate) map[worldmaps.NPCStatTemplateID]worldmaps.NPCStatTemplate {
	byID := make(map[worldmaps.NPCStatTemplateID]worldmaps.NPCStatTemplate, len(templates))
	for _, template := range templates {
		byID[template.StatTemplateID] = template
	}
	return byID
}

func dropProfilesByID(profiles []worldmaps.NPCDropProfile) map[worldmaps.NPCDropProfileID]worldmaps.NPCDropProfile {
	byID := make(map[worldmaps.NPCDropProfileID]worldmaps.NPCDropProfile, len(profiles))
	for _, profile := range profiles {
		byID[profile.DropProfileID] = profile
	}
	return byID
}

func statTemplateCompatibleWithPool(pool worldmaps.MapEnemyPoolDefinition, template worldmaps.NPCStatTemplate) bool {
	if template.NPCType != pool.NPCType {
		return false
	}
	return template.MinLevel <= pool.MinLevel && template.MaxLevel >= pool.MaxLevel
}

func dropProfileCompatibleWithPool(definition worldmaps.MapDefinition, pool worldmaps.MapEnemyPoolDefinition, profile worldmaps.NPCDropProfile) bool {
	if profile.NPCType != pool.NPCType {
		return false
	}
	if pool.MinLevel < profile.MinLevel || pool.MaxLevel > profile.MaxLevel {
		return false
	}
	return profile.RiskBand == definition.RiskBand
}

func eventSpawnsByID(eventSpawns []worldmaps.NPCEventSpawnDefinition) map[worldmaps.NPCEventSpawnID]worldmaps.NPCEventSpawnDefinition {
	byID := make(map[worldmaps.NPCEventSpawnID]worldmaps.NPCEventSpawnDefinition, len(eventSpawns))
	for _, eventSpawn := range eventSpawns {
		byID[eventSpawn.EventSpawnID] = eventSpawn
	}
	return byID
}

func selectSpawnArea(
	definition worldmaps.MapDefinition,
	spawnAreas map[worldmaps.SpawnAreaID]worldmaps.MapSpawnAreaDefinition,
	pool worldmaps.MapEnemyPoolDefinition,
	spawnIndex int,
) (worldmaps.MapSpawnAreaDefinition, bool, error) {
	if len(pool.SpawnAreaIDs) == 0 {
		return worldmaps.MapSpawnAreaDefinition{}, false, fmt.Errorf("enemy pool %q spawn areas: %w", pool.EnemyPoolID, worldmaps.ErrInvalidMapDefinition)
	}
	for offset := 0; offset < len(pool.SpawnAreaIDs); offset++ {
		areaID := pool.SpawnAreaIDs[(spawnIndex+offset)%len(pool.SpawnAreaIDs)]
		area, ok := spawnAreas[areaID]
		if !ok {
			return worldmaps.MapSpawnAreaDefinition{}, false, fmt.Errorf("enemy pool %q spawn area %q: %w", pool.EnemyPoolID, areaID, worldmaps.ErrInvalidCatalog)
		}
		// Phase08B/08E deterministic MVP: use the area center as the spawn
		// candidate. Richer RNG placement remains deferred.
		if validSpawnCandidate(definition, area, area.Center) {
			return area, true, nil
		}
	}
	return worldmaps.MapSpawnAreaDefinition{}, false, nil
}

func selectInitialSpawnArea(
	definition worldmaps.MapDefinition,
	spawnAreas map[worldmaps.SpawnAreaID]worldmaps.MapSpawnAreaDefinition,
	pool worldmaps.MapEnemyPoolDefinition,
	spawnIndex int,
) (worldmaps.MapSpawnAreaDefinition, bool, error) {
	return selectSpawnArea(definition, spawnAreas, pool, spawnIndex)
}

func validSpawnCandidate(definition worldmaps.MapDefinition, area worldmaps.MapSpawnAreaDefinition, position world.Vec2) bool {
	if err := position.Validate(); err != nil {
		return false
	}
	if !definition.Bounds.Contains(position) {
		return false
	}
	if area.Shape == worldmaps.SpawnAreaShapeCircle && position.DistanceSquared(area.Center) > area.Radius*area.Radius {
		return false
	}
	if area.SafeZoneExcluded {
		if _, ok := definition.PVPBlockingSafeZoneAt(position); ok {
			return false
		}
	}
	if area.PortalExclusionRadius > 0 {
		exclusionRadiusSquared := area.PortalExclusionRadius * area.PortalExclusionRadius
		for _, portal := range definition.Portals {
			if portal.Visible && position.DistanceSquared(portal.SourcePosition) <= exclusionRadiusSquared {
				return false
			}
		}
	}
	return true
}

func validInitialSpawnCandidate(definition worldmaps.MapDefinition, area worldmaps.MapSpawnAreaDefinition, position world.Vec2) bool {
	return validSpawnCandidate(definition, area, position)
}

func enemySpawnEntityID(mapID worldmaps.MapID, poolID worldmaps.EnemyPoolID, spawnIndex int, overrides map[worldmaps.EnemyPoolID][]world.EntityID) world.EntityID {
	if len(overrides) > 0 {
		if ids := overrides[poolID]; spawnIndex < len(ids) && !ids[spawnIndex].IsZero() {
			return ids[spawnIndex]
		}
	}
	return world.EntityID(fmt.Sprintf(
		"entity_npc_%s_%03d",
		entityIDOpaqueHashSuffix(mapID.String(), poolID.String()),
		spawnIndex+1,
	))
}

func enemyEventSpawnEntityID(mapID worldmaps.MapID, poolID worldmaps.EnemyPoolID, eventSpawnID worldmaps.NPCEventSpawnID, spawnIndex int) world.EntityID {
	return world.EntityID(fmt.Sprintf(
		"entity_npc_evt_%s_%03d",
		entityIDOpaqueHashSuffix(mapID.String(), poolID.String(), eventSpawnID.String()),
		spawnIndex+1,
	))
}

func entityIDOpaqueHashSuffix(parts ...string) string {
	var raw strings.Builder
	for _, part := range parts {
		fmt.Fprintf(&raw, "%d:", len(part))
		raw.WriteString(part)
		raw.WriteByte('|')
	}
	sum := sha256.Sum256([]byte(raw.String()))
	return hex.EncodeToString(sum[:16])
}

type enemySpawnerState struct {
	rows              []EnemySpawnRecord
	byEntityID        map[world.EntityID]int
	aliveByPool       map[worldmaps.EnemyPoolID]int
	initializedPools  map[worldmaps.EnemyPoolID]struct{}
	hasTickDefinition bool
	tickDefinition    worldmaps.MapDefinition
	lastFillByPool    map[worldmaps.EnemyPoolID]time.Time
	eventDueByID      map[worldmaps.NPCEventSpawnID]time.Time
}

func newEnemySpawnerState() *enemySpawnerState {
	return &enemySpawnerState{
		rows:             make([]EnemySpawnRecord, 0),
		byEntityID:       make(map[world.EntityID]int),
		aliveByPool:      make(map[worldmaps.EnemyPoolID]int),
		initializedPools: make(map[worldmaps.EnemyPoolID]struct{}),
		lastFillByPool:   make(map[worldmaps.EnemyPoolID]time.Time),
		eventDueByID:     make(map[worldmaps.NPCEventSpawnID]time.Time),
	}
}

func (spawner *enemySpawnerState) add(record EnemySpawnRecord) {
	spawner.byEntityID[record.EntityID] = len(spawner.rows)
	spawner.rows = append(spawner.rows, record)
	if record.Alive {
		spawner.aliveByPool[record.EnemyPoolID]++
	}
}

func (spawner *enemySpawnerState) record(entityID world.EntityID) (EnemySpawnRecord, bool) {
	index, ok := spawner.byEntityID[entityID]
	if !ok {
		return EnemySpawnRecord{}, false
	}
	return spawner.rows[index], true
}

func (spawner *enemySpawnerState) snapshot() EnemySpawnSnapshot {
	records := append([]EnemySpawnRecord(nil), spawner.rows...)
	sort.Slice(records, func(i, j int) bool {
		return records[i].EntityID < records[j].EntityID
	})
	aliveCounts := make(map[worldmaps.EnemyPoolID]int, len(spawner.aliveByPool))
	for poolID, count := range spawner.aliveByPool {
		aliveCounts[poolID] = count
	}
	initializedPools := make([]worldmaps.EnemyPoolID, 0, len(spawner.initializedPools))
	for poolID := range spawner.initializedPools {
		initializedPools = append(initializedPools, poolID)
	}
	sort.Slice(initializedPools, func(i, j int) bool {
		return initializedPools[i] < initializedPools[j]
	})
	return EnemySpawnSnapshot{
		Records:          records,
		PoolAliveCounts:  aliveCounts,
		InitializedPools: initializedPools,
		MapAliveCount:    spawner.mapAliveCount(),
	}
}

func (spawner *enemySpawnerState) poolAliveCount(poolID worldmaps.EnemyPoolID) int {
	return spawner.aliveByPool[poolID]
}

func (spawner *enemySpawnerState) eventAliveCount(eventSpawnID worldmaps.NPCEventSpawnID) int {
	count := 0
	for _, record := range spawner.rows {
		if record.EventSpawnID == eventSpawnID && record.Alive {
			count++
		}
	}
	return count
}

func (spawner *enemySpawnerState) poolReservedCount(poolID worldmaps.EnemyPoolID) int {
	count := 0
	for _, record := range spawner.rows {
		if record.EnemyPoolID != poolID {
			continue
		}
		if record.Alive || !record.NextRespawnAt.IsZero() {
			count++
		}
	}
	return count
}

func (spawner *enemySpawnerState) mapAliveCount() int {
	count := 0
	for _, record := range spawner.rows {
		if record.Alive {
			count++
		}
	}
	return count
}

func (spawner *enemySpawnerState) mapReservedCount() int {
	count := 0
	for _, record := range spawner.rows {
		if record.Alive || !record.NextRespawnAt.IsZero() {
			count++
		}
	}
	return count
}

func (spawner *enemySpawnerState) poolInitialized(poolID worldmaps.EnemyPoolID) bool {
	_, ok := spawner.initializedPools[poolID]
	return ok
}

func (spawner *enemySpawnerState) markPoolInitialized(poolID worldmaps.EnemyPoolID) {
	spawner.initializedPools[poolID] = struct{}{}
}

func (spawner *enemySpawnerState) configureTicks(definition worldmaps.MapDefinition, now time.Time) {
	spawner.tickDefinition = cloneEnemySpawnerTickDefinition(definition)
	spawner.hasTickDefinition = true
	for _, pool := range definition.EnemyPools {
		if _, exists := spawner.lastFillByPool[pool.EnemyPoolID]; !exists {
			spawner.lastFillByPool[pool.EnemyPoolID] = now
		}
	}
	for _, eventSpawn := range definition.NPCEventSpawns {
		if _, exists := spawner.eventDueByID[eventSpawn.EventSpawnID]; !exists {
			spawner.eventDueByID[eventSpawn.EventSpawnID] = now.Add(eventSpawn.StartsAfter)
		}
	}
}

func (spawner *enemySpawnerState) lastPeriodicFillAt(poolID worldmaps.EnemyPoolID) time.Time {
	if last, ok := spawner.lastFillByPool[poolID]; ok {
		return last
	}
	return time.Time{}
}

func (spawner *enemySpawnerState) setLastPeriodicFillAt(poolID worldmaps.EnemyPoolID, at time.Time) {
	spawner.lastFillByPool[poolID] = at
}

func (spawner *enemySpawnerState) ensureEventDueAt(eventSpawn worldmaps.NPCEventSpawnDefinition, firstSeenAt time.Time) time.Time {
	if dueAt, ok := spawner.eventDueByID[eventSpawn.EventSpawnID]; ok {
		return dueAt
	}
	dueAt := firstSeenAt.Add(eventSpawn.StartsAfter)
	spawner.eventDueByID[eventSpawn.EventSpawnID] = dueAt
	return dueAt
}

func (spawner *enemySpawnerState) rowCountForPool(poolID worldmaps.EnemyPoolID) int {
	count := 0
	for _, record := range spawner.rows {
		if record.EnemyPoolID == poolID {
			count++
		}
	}
	return count
}

func cloneEnemySpawnerTickDefinition(definition worldmaps.MapDefinition) worldmaps.MapDefinition {
	cloned := definition
	cloned.SpawnPoints = append([]worldmaps.SpawnPointDefinition(nil), definition.SpawnPoints...)
	cloned.Portals = append([]worldmaps.PortalDefinition(nil), definition.Portals...)
	cloned.SafeZones = append([]worldmaps.SafeZoneDefinition(nil), definition.SafeZones...)
	cloned.SpawnAreas = append([]worldmaps.MapSpawnAreaDefinition(nil), definition.SpawnAreas...)
	cloned.EnemyPools = append([]worldmaps.MapEnemyPoolDefinition(nil), definition.EnemyPools...)
	for index := range cloned.EnemyPools {
		cloned.EnemyPools[index].SpawnAreaIDs = append([]worldmaps.SpawnAreaID(nil), definition.EnemyPools[index].SpawnAreaIDs...)
	}
	cloned.NPCEventSpawns = append([]worldmaps.NPCEventSpawnDefinition(nil), definition.NPCEventSpawns...)
	cloned.NPCStatTemplates = append([]worldmaps.NPCStatTemplate(nil), definition.NPCStatTemplates...)
	cloned.NPCDropProfiles = append([]worldmaps.NPCDropProfile(nil), definition.NPCDropProfiles...)
	cloned.NPCAggroProfiles = append([]worldmaps.NPCAggroProfile(nil), definition.NPCAggroProfiles...)
	cloned.NPCLeashProfiles = append([]worldmaps.NPCLeashProfile(nil), definition.NPCLeashProfiles...)
	return cloned
}

func deterministicSpawnJitter(limit time.Duration, parts ...string) time.Duration {
	if limit <= 0 {
		return 0
	}
	hasher := fnv.New64a()
	for _, part := range parts {
		fmt.Fprintf(hasher, "%d:%s|", len(part), part)
	}
	return time.Duration(hasher.Sum64() % uint64(limit+1))
}
