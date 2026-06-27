package worker

import (
	"fmt"
	"strings"
	"time"

	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const npcSafeZoneAttackPolicyNever = "never"

func (worker *Worker) tickEnemyAggro() []CommandError {
	worker.enemyAggroCandidateChecks = 0
	if worker.enemySpawner == nil || !worker.enemySpawner.hasTickDefinition {
		return nil
	}
	if err := worker.tickEnemyAggroDefinition(worker.enemySpawner.tickDefinition); err != nil {
		return []CommandError{{Index: -1, Err: err}}
	}
	return nil
}

func (worker *Worker) tickEnemyAggroDefinition(definition worldmaps.MapDefinition) error {
	if err := worker.validateEnemyPoolDefinitionOwnership(definition); err != nil {
		worker.recordEnemySpawnerRejection(EnemyTelemetryStageTickAggro, EnemyTelemetryReasonOwnership)
		return err
	}
	if len(worker.enemySpawner.rows) == 0 {
		return nil
	}

	aggroProfiles := aggroProfilesByID(definition.NPCAggroProfiles)
	leashProfiles := leashProfilesByID(definition.NPCLeashProfiles)
	now := worker.clock.Now()
	for index := range worker.enemySpawner.rows {
		if err := worker.tickEnemyAggroRow(index, definition, aggroProfiles, leashProfiles, now); err != nil {
			return err
		}
	}
	return nil
}

func (worker *Worker) tickEnemyAggroRow(
	index int,
	definition worldmaps.MapDefinition,
	aggroProfiles map[worldmaps.NPCAggroProfileID]worldmaps.NPCAggroProfile,
	leashProfiles map[worldmaps.NPCLeashProfileID]worldmaps.NPCLeashProfile,
	now time.Time,
) error {
	record := worker.enemySpawner.rows[index]
	if !record.Alive {
		return nil
	}
	aggroProfile, ok := aggroProfiles[record.AggroProfileID]
	if !ok {
		return fmt.Errorf("enemy spawn entity %q aggro profile %q: %w", record.EntityID, record.AggroProfileID, worldmaps.ErrInvalidCatalog)
	}
	leashProfile, ok := leashProfiles[record.LeashProfileID]
	if !ok {
		return fmt.Errorf("enemy spawn entity %q leash profile %q: %w", record.EntityID, record.LeashProfileID, worldmaps.ErrInvalidCatalog)
	}

	entity, ok := worker.entities[record.EntityID]
	if !ok {
		return fmt.Errorf("enemy spawn entity %q: %w", record.EntityID, ErrUnknownEntity)
	}
	record.LastAggroTickAt = now
	record.Position = entity.Position

	if aggroProfile.AggroRadius <= 0 {
		if !record.AggroTargetEntityID.IsZero() {
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindAggro,
				EnemyTelemetryStageTargeting,
				EnemyTelemetryResultCleared,
				EnemyTelemetryReasonPassive,
				record.NPCType,
				"",
			)
		}
		clearEnemyTargetMemory(&record)
		entity = worker.stopEnemyMovement(entity)
		worker.enemySpawner.rows[index] = record
		return nil
	}

	if safeZoneAttackNever(aggroProfile) && definitionPositionInPVPBlockingSafeZone(definition, entity.Position) {
		if !record.AggroTargetEntityID.IsZero() {
			worker.recordEnemyTelemetry(
				EnemyTelemetryKindAggro,
				EnemyTelemetryStageTargeting,
				EnemyTelemetryResultReset,
				EnemyTelemetryReasonSafeZone,
				record.NPCType,
				"",
			)
		}
		entity, err := worker.resetEnemyTargetAndMaybeReturn(entity, &record, leashProfile, now)
		if err != nil {
			return err
		}
		record.Position = entity.Position
		worker.enemySpawner.rows[index] = record
		return nil
	}

	if record.AggroTargetEntityID.IsZero() {
		target, acquired, err := worker.nearestAggroTarget(definition, entity, record.LeashOrigin, aggroProfile, leashProfile)
		if err != nil {
			return err
		}
		if !acquired {
			entity, err := worker.resetEnemyTargetAndMaybeReturn(entity, &record, leashProfile, now)
			if err != nil {
				return err
			}
			record.Position = entity.Position
			worker.enemySpawner.rows[index] = record
			return nil
		}
		record.AggroTargetEntityID = target.ID
		record.AggroAcquiredAt = now
		record.AggroTargetLastSeenAt = now
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindAggro,
			EnemyTelemetryStageTargeting,
			EnemyTelemetryResultAcquired,
			EnemyTelemetryReasonInRange,
			record.NPCType,
			"",
		)
	}

	target, keep, lostReason := worker.currentAggroTarget(record.AggroTargetEntityID)
	if !keep {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindAggro,
			EnemyTelemetryStageTargeting,
			EnemyTelemetryResultCleared,
			lostReason,
			record.NPCType,
			"",
		)
		entity, err := worker.resetEnemyTargetAndMaybeReturn(entity, &record, leashProfile, now)
		if err != nil {
			return err
		}
		record.Position = entity.Position
		worker.enemySpawner.rows[index] = record
		return nil
	}

	if safeZoneAttackNever(aggroProfile) && (definitionPositionInPVPBlockingSafeZone(definition, entity.Position) || definitionPositionInPVPBlockingSafeZone(definition, target.Position)) {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindAggro,
			EnemyTelemetryStageTargeting,
			EnemyTelemetryResultReset,
			EnemyTelemetryReasonSafeZone,
			record.NPCType,
			"",
		)
		entity, err := worker.resetEnemyTargetAndMaybeReturn(entity, &record, leashProfile, now)
		if err != nil {
			return err
		}
		record.Position = entity.Position
		worker.enemySpawner.rows[index] = record
		return nil
	}

	if leashProfile.ResetOnBreak && enemyLeashBroken(record.LeashOrigin, leashProfile.LeashDistance, entity.Position, target.Position) {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindAggro,
			EnemyTelemetryStageTargeting,
			EnemyTelemetryResultReset,
			EnemyTelemetryReasonLeashBreak,
			record.NPCType,
			"",
		)
		entity, err := worker.resetEnemyTargetAndMaybeReturn(entity, &record, leashProfile, now)
		if err != nil {
			return err
		}
		record.Position = entity.Position
		worker.enemySpawner.rows[index] = record
		return nil
	}

	if entity.Position.DistanceSquared(target.Position) <= aggroProfile.AggroRadius*aggroProfile.AggroRadius {
		record.AggroTargetLastSeenAt = now
	} else if aggroTargetMemoryExpired(record, aggroProfile, now) {
		worker.recordEnemyTelemetry(
			EnemyTelemetryKindAggro,
			EnemyTelemetryStageTargeting,
			EnemyTelemetryResultReset,
			EnemyTelemetryReasonTargetMemoryExpired,
			record.NPCType,
			"",
		)
		entity, err := worker.resetEnemyTargetAndMaybeReturn(entity, &record, leashProfile, now)
		if err != nil {
			return err
		}
		record.Position = entity.Position
		worker.enemySpawner.rows[index] = record
		return nil
	}

	entity, err := worker.moveEnemyToward(entity, target.Position, now)
	if err != nil {
		return err
	}
	record.Position = entity.Position
	worker.enemySpawner.rows[index] = record
	return nil
}

func clearEnemyAggroState(record *EnemySpawnRecord) {
	clearEnemyTargetMemory(record)
	record.LastAggroTickAt = time.Time{}
}

func clearEnemyTargetMemory(record *EnemySpawnRecord) {
	record.AggroTargetEntityID = ""
	record.AggroAcquiredAt = time.Time{}
	record.AggroTargetLastSeenAt = time.Time{}
}

func aggroProfilesByID(profiles []worldmaps.NPCAggroProfile) map[worldmaps.NPCAggroProfileID]worldmaps.NPCAggroProfile {
	byID := make(map[worldmaps.NPCAggroProfileID]worldmaps.NPCAggroProfile, len(profiles))
	for _, profile := range profiles {
		byID[profile.AggroProfileID] = profile
	}
	return byID
}

func leashProfilesByID(profiles []worldmaps.NPCLeashProfile) map[worldmaps.NPCLeashProfileID]worldmaps.NPCLeashProfile {
	byID := make(map[worldmaps.NPCLeashProfileID]worldmaps.NPCLeashProfile, len(profiles))
	for _, profile := range profiles {
		byID[profile.LeashProfileID] = profile
	}
	return byID
}

func (worker *Worker) nearestAggroTarget(
	definition worldmaps.MapDefinition,
	npc world.Entity,
	leashOrigin world.Vec2,
	aggroProfile worldmaps.NPCAggroProfile,
	leashProfile worldmaps.NPCLeashProfile,
) (world.Entity, bool, error) {
	if safeZoneAttackNever(aggroProfile) && definitionPositionInPVPBlockingSafeZone(definition, npc.Position) {
		return world.Entity{}, false, nil
	}
	candidates, err := worker.playerIndex.QueryRadius(spatialPosition(npc.Position), aggroProfile.AggroRadius)
	if err != nil {
		return world.Entity{}, false, err
	}
	worker.enemyAggroCandidateChecks += len(candidates)

	var best world.Entity
	var bestDistanceSquared float64
	found := false
	aggroRadiusSquared := aggroProfile.AggroRadius * aggroProfile.AggroRadius
	for _, result := range candidates {
		candidate, ok := worker.entities[world.EntityID(result.ID)]
		if !ok || candidate.Type != world.EntityTypePlayer || candidate.WorldID != npc.WorldID || candidate.ZoneID != npc.ZoneID {
			continue
		}
		if !worker.playerAggroEligibleByEntityID(candidate.ID) {
			continue
		}
		if safeZoneAttackNever(aggroProfile) && definitionPositionInPVPBlockingSafeZone(definition, candidate.Position) {
			continue
		}
		if leashProfile.ResetOnBreak && enemyLeashBroken(leashOrigin, leashProfile.LeashDistance, npc.Position, candidate.Position) {
			continue
		}
		distanceSquared := npc.Position.DistanceSquared(candidate.Position)
		if distanceSquared > aggroRadiusSquared {
			continue
		}
		if !found || distanceSquared < bestDistanceSquared || (distanceSquared == bestDistanceSquared && candidate.ID < best.ID) {
			best = candidate
			bestDistanceSquared = distanceSquared
			found = true
		}
	}
	return best, found, nil
}

func (worker *Worker) currentAggroTarget(entityID world.EntityID) (world.Entity, bool, string) {
	target, ok := worker.entities[entityID]
	if !ok || target.Type != world.EntityTypePlayer {
		return world.Entity{}, false, EnemyTelemetryReasonTargetMissing
	}
	if !worker.playerAggroEligibleByEntityID(target.ID) {
		return world.Entity{}, false, EnemyTelemetryReasonTargetIneligible
	}
	return target, true, ""
}

func (worker *Worker) playerAggroEligibleByEntityID(entityID world.EntityID) bool {
	playerID, ok := worker.entityPlayers[entityID]
	if !ok {
		return false
	}
	return !worker.playerAggroIneligible[playerID]
}

func (worker *Worker) moveEnemyToward(entity world.Entity, target world.Vec2, now time.Time) (world.Entity, error) {
	speed := worker.entitySpeeds[entity.ID]
	if speed <= 0 || entity.Position.DistanceSquared(target) == 0 {
		entity = worker.stopEnemyMovement(entity)
		return entity, nil
	}
	movement, err := world.NewTimedMovementState(entity.Position, target, speed, now)
	if err != nil {
		return world.Entity{}, err
	}
	entity.Movement = movement
	worker.entities[entity.ID] = entity
	return entity, nil
}

func (worker *Worker) resetEnemyTargetAndMaybeReturn(entity world.Entity, record *EnemySpawnRecord, leashProfile worldmaps.NPCLeashProfile, now time.Time) (world.Entity, error) {
	clearEnemyTargetMemory(record)
	if !leashProfile.ResetOnBreak {
		return worker.stopEnemyMovement(entity), nil
	}
	return worker.returnEnemyToLeashOrigin(entity, record.LeashOrigin, now)
}

func (worker *Worker) returnEnemyToLeashOrigin(entity world.Entity, leashOrigin world.Vec2, now time.Time) (world.Entity, error) {
	return worker.moveEnemyToward(entity, leashOrigin, now)
}

func (worker *Worker) stopEnemyMovement(entity world.Entity) world.Entity {
	entity.Movement = world.MovementState{}
	worker.entities[entity.ID] = entity
	return entity
}

func aggroTargetMemoryExpired(record EnemySpawnRecord, aggroProfile worldmaps.NPCAggroProfile, now time.Time) bool {
	if record.AggroTargetLastSeenAt.IsZero() {
		return true
	}
	if aggroProfile.TargetMemory <= 0 {
		return true
	}
	return !record.AggroTargetLastSeenAt.Add(aggroProfile.TargetMemory).After(now)
}

func enemyLeashBroken(origin world.Vec2, leashDistance float64, npcPosition world.Vec2, targetPosition world.Vec2) bool {
	leashDistanceSquared := leashDistance * leashDistance
	return npcPosition.DistanceSquared(origin) > leashDistanceSquared || targetPosition.DistanceSquared(origin) > leashDistanceSquared
}

func safeZoneAttackNever(profile worldmaps.NPCAggroProfile) bool {
	return strings.EqualFold(strings.TrimSpace(profile.SafeZoneAttackPolicy), npcSafeZoneAttackPolicyNever)
}

func definitionPositionInPVPBlockingSafeZone(definition worldmaps.MapDefinition, position world.Vec2) bool {
	_, ok := definition.PVPBlockingSafeZoneAt(position)
	return ok
}
