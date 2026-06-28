package server

import (
	"errors"
	"sort"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
)

const npcBasicLaserSkillID = "npc_basic_laser"

func (runtime *Runtime) tickNPCCombatLocked(now time.Time) {
	for _, instance := range runtime.sortedMapInstancesLocked() {
		records := instance.Worker.EnemySpawnSnapshot().Records
		sort.Slice(records, func(i, j int) bool {
			return records[i].EntityID < records[j].EntityID
		})
		for _, record := range records {
			if !record.Alive || record.AggroTargetEntityID.IsZero() {
				continue
			}
			npcEntity, ok := instance.Worker.Entity(record.EntityID)
			if !ok || npcEntity.Type != world.EntityTypeNPC {
				continue
			}
			targetPlayerID, _, ok := runtime.playerByEntityLocked(record.AggroTargetEntityID)
			if !ok || !runtime.npcMayAttackPlayerLocked(instance, npcEntity, targetPlayerID, record.AggroTargetEntityID) {
				continue
			}
			runtime.executeNPCBasicLaserLocked(now, instance, npcEntity, targetPlayerID, record.AggroTargetEntityID)
		}
	}
}

func (runtime *Runtime) npcMayAttackPlayerLocked(instance *mapInstance, npcEntity world.Entity, targetPlayerID foundation.PlayerID, targetEntityID world.EntityID) bool {
	if instance == nil || instance.Worker == nil || targetPlayerID.IsZero() || targetEntityID.IsZero() {
		return false
	}
	if instance.HiddenPlayers[targetPlayerID] || instance.HiddenEntities[targetEntityID] {
		return false
	}
	if _, ok := runtime.protectionForMapLocked(targetPlayerID, instance.Definition.InternalMapID); ok {
		return false
	}
	targetEntity, ok := instance.Worker.Entity(targetEntityID)
	if !ok || targetEntity.Type != world.EntityTypePlayer {
		return false
	}
	if _, ok := instance.Definition.PVPBlockingSafeZoneAt(npcEntity.Position); ok {
		return false
	}
	if _, ok := instance.Definition.PVPBlockingSafeZoneAt(targetEntity.Position); ok {
		return false
	}
	return true
}

func (runtime *Runtime) executeNPCBasicLaserLocked(now time.Time, instance *mapInstance, npcEntity world.Entity, targetPlayerID foundation.PlayerID, targetEntityID world.EntityID) {
	attacker, err := runtime.upsertNPCCombatActorProjectionLocked(instance, npcEntity)
	if err != nil {
		return
	}
	target, err := runtime.syncPlayerCombatActorLocked(targetPlayerID)
	if err != nil || target.EntityID != targetEntityID {
		return
	}
	result, err := runtime.Combat.ExecuteBasicAttack(combat.BasicAttackInput{
		AttackerID: attacker.EntityID,
		TargetID:   target.EntityID,
		Policy: combat.AttackPolicyFunc(func(input combat.AttackPolicyInput) error {
			if runtime.npcMayAttackPlayerLocked(instance, npcEntity, targetPlayerID, targetEntityID) {
				return nil
			}
			return combat.ErrPVPBlocked
		}),
	})
	if err != nil {
		if errors.Is(err, combat.ErrCooldownNotReady) ||
			errors.Is(err, combat.ErrOutOfRange) ||
			errors.Is(err, combat.ErrTargetNotVisible) ||
			errors.Is(err, combat.ErrPVPBlocked) ||
			errors.Is(err, combat.ErrNotEnoughEnergy) ||
			errors.Is(err, combat.ErrAttackerDead) ||
			errors.Is(err, combat.ErrTargetDead) {
			return
		}
		return
	}
	targetState, ok := runtime.applyCombatActorToPlayerShipLocked(targetPlayerID, result.Target)
	if !ok {
		return
	}
	runtime.refreshCombatLockForActorLocked(targetPlayerID, now)
	runtime.queueNPCCombatEventsLocked(targetPlayerID, result, targetState)
}

func (runtime *Runtime) queueNPCCombatEventsLocked(playerID foundation.PlayerID, result combat.BasicAttackResult, targetState playerRuntimeState) {
	sessionIDs := runtime.sessionIDsForPlayerLocked(playerID, "")
	for _, sessionID := range sessionIDs {
		runtime.queueNPCCombatEventsForSessionLocked(sessionID, result, targetState)
	}
}

func (runtime *Runtime) queueNPCCombatEventsForSessionLocked(sessionID auth.SessionID, result combat.BasicAttackResult, targetState playerRuntimeState) {
	runtime.queueEventLocked(sessionID, realtime.EventCombatShotStarted, map[string]any{
		"source_id": result.Attacker.EntityID.String(),
		"target_id": result.Target.EntityID.String(),
		"skill_id":  npcBasicLaserSkillID,
	})
	if result.Hit {
		runtime.queueEventLocked(sessionID, realtime.EventCombatDamage, map[string]any{
			"source_id":     result.Attacker.EntityID.String(),
			"target_id":     result.Target.EntityID.String(),
			"amount":        roundCombatValue(result.Damage),
			"shield_amount": roundCombatValue(result.ShieldDamage),
			"hull_amount":   roundCombatValue(result.HPDamage),
		})
	} else {
		runtime.queueEventLocked(sessionID, realtime.EventCombatMiss, map[string]any{
			"source_id": result.Attacker.EntityID.String(),
			"target_id": result.Target.EntityID.String(),
		})
	}
	runtime.queueEventLocked(sessionID, realtime.EventPlayerSnapshot, targetState.playerSnapshot())
	runtime.queueEventLocked(sessionID, realtime.EventShipSnapshot, targetState.Ship)
	runtime.queueEventLocked(sessionID, realtime.EventCombatShotResolved, map[string]any{
		"source_id":            result.Attacker.EntityID.String(),
		"target_id":            result.Target.EntityID.String(),
		"skill_id":             npcBasicLaserSkillID,
		"hit":                  result.Hit,
		"amount":               roundCombatValue(result.Damage),
		"killed":               result.Target.Dead || result.Target.HP <= 0,
		"cooldown_ready_at_ms": result.CooldownReadyAt.UTC().UnixMilli(),
	})
}
