package server

import (
	"fmt"
	"sort"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
)

const defaultCombatEngagementSkillID = "basic_laser"

type combatStopReason string

const (
	combatStopReasonManual           combatStopReason = "manual"
	combatStopReasonTargetLost       combatStopReason = "target_lost"
	combatStopReasonTargetNotVisible combatStopReason = "target_not_visible"
	combatStopReasonOutOfRange       combatStopReason = "out_of_range"
	combatStopReasonCooldown         combatStopReason = "cooldown"
	combatStopReasonNotEnoughEnergy  combatStopReason = "not_enough_energy"
	combatStopReasonNotEnoughAmmo    combatStopReason = "not_enough_ammo"
	combatStopReasonShipDisabled     combatStopReason = "ship_disabled"
	combatStopReasonTargetDestroyed  combatStopReason = "target_destroyed"
	combatStopReasonMapChanged       combatStopReason = "map_changed"
	combatStopReasonPolicyBlocked    combatStopReason = "policy_blocked"
)

type combatEngagementState struct {
	PlayerID       foundation.PlayerID
	TargetID       world.EntityID
	SkillID        string
	StartedAt      time.Time
	NextFireAt     time.Time
	LastStopReason string
}

type combatEngagementSnapshot struct {
	Active         bool
	PlayerID       foundation.PlayerID
	TargetID       world.EntityID
	SkillID        string
	StartedAt      time.Time
	NextFireAt     time.Time
	LastStopReason string
}

func (runtime *Runtime) startCombatEngagementLocked(playerID foundation.PlayerID, targetID world.EntityID, skillID string, now time.Time) combatEngagementState {
	runtime.ensureCombatEngagementMapsLocked()
	if skillID == "" {
		skillID = defaultCombatEngagementSkillID
	}
	if existing, ok := runtime.activeCombatEngagements[playerID]; ok && existing.TargetID == targetID && existing.SkillID == skillID {
		return existing
	}
	state := combatEngagementState{
		PlayerID:   playerID,
		TargetID:   targetID,
		SkillID:    skillID,
		StartedAt:  now,
		NextFireAt: now,
	}
	runtime.activeCombatEngagements[playerID] = state
	delete(runtime.lastCombatStopReasons, playerID)
	return state
}

func (runtime *Runtime) stopCombatEngagementLocked(playerID foundation.PlayerID, reason combatStopReason, now time.Time) combatEngagementSnapshot {
	runtime.ensureCombatEngagementMapsLocked()
	if reason == "" {
		reason = combatStopReasonManual
	}
	delete(runtime.activeCombatEngagements, playerID)
	runtime.lastCombatStopReasons[playerID] = reason
	return runtime.combatEngagementSnapshotLocked(playerID, now)
}

func (runtime *Runtime) combatEngagementSnapshotLocked(playerID foundation.PlayerID, now time.Time) combatEngagementSnapshot {
	runtime.ensureCombatEngagementMapsLocked()
	if active, ok := runtime.activeCombatEngagements[playerID]; ok {
		return combatEngagementSnapshot{
			Active:         true,
			PlayerID:       active.PlayerID,
			TargetID:       active.TargetID,
			SkillID:        active.SkillID,
			StartedAt:      active.StartedAt,
			NextFireAt:     active.NextFireAt,
			LastStopReason: active.LastStopReason,
		}
	}
	return combatEngagementSnapshot{
		Active:         false,
		PlayerID:       playerID,
		LastStopReason: string(runtime.lastCombatStopReasons[playerID]),
	}
}

func (runtime *Runtime) clearCombatEngagementsForEntityLocked(entityID world.EntityID, reason combatStopReason, now time.Time) []combatEngagementSnapshot {
	runtime.ensureCombatEngagementMapsLocked()
	stopped := make([]combatEngagementSnapshot, 0)
	for playerID, state := range runtime.activeCombatEngagements {
		if state.TargetID != entityID && runtime.playerEntityIDLocked(playerID) != entityID {
			continue
		}
		stopped = append(stopped, runtime.stopCombatEngagementLocked(playerID, reason, now))
	}
	return stopped
}

func (runtime *Runtime) ensureCombatEngagementMapsLocked() {
	if runtime.activeCombatEngagements == nil {
		runtime.activeCombatEngagements = make(map[foundation.PlayerID]combatEngagementState)
	}
	if runtime.lastCombatStopReasons == nil {
		runtime.lastCombatStopReasons = make(map[foundation.PlayerID]combatStopReason)
	}
}

func (runtime *Runtime) playerEntityIDLocked(playerID foundation.PlayerID) world.EntityID {
	if runtime.players == nil {
		return ""
	}
	return runtime.players[playerID].EntityID
}

func (runtime *Runtime) tickCombatEngagementsLocked(now time.Time) {
	runtime.ensureCombatEngagementMapsLocked()
	playerIDs := make([]foundation.PlayerID, 0, len(runtime.activeCombatEngagements))
	for playerID := range runtime.activeCombatEngagements {
		playerIDs = append(playerIDs, playerID)
	}
	sort.Slice(playerIDs, func(i, j int) bool {
		return playerIDs[i] < playerIDs[j]
	})
	for _, playerID := range playerIDs {
		state, ok := runtime.activeCombatEngagements[playerID]
		if !ok || now.Before(state.NextFireAt) {
			continue
		}
		sessionIDs := runtime.sessionIDsForPlayerLocked(playerID, "")
		if len(sessionIDs) == 0 {
			continue
		}
		execution, err := runtime.executeBasicLaserLocked(runtimeBasicLaserInput{
			RequestID:          combatEngagementTickRequestID(playerID, state.TargetID, now),
			PlayerID:           playerID,
			TargetID:           state.TargetID,
			SkillID:            state.SkillID,
			SessionIDs:         sessionIDs,
			QueueShotLifecycle: true,
		})
		if err != nil {
			if reason, stop := combatEngagementStopReasonForError(err); stop {
				runtime.queueCombatEngagementStoppedLocked(playerID, sessionIDs, reason, now)
			}
			continue
		}
		if execution.TargetKilled {
			runtime.queueCombatEngagementStoppedLocked(playerID, sessionIDs, combatStopReasonTargetDestroyed, now)
			continue
		}
		state.NextFireAt = execution.Combat.CooldownReadyAt
		state.LastStopReason = ""
		runtime.activeCombatEngagements[playerID] = state
		payload := runtime.combatEngagementPayloadForStateLocked(state, "")
		for _, sessionID := range sessionIDs {
			runtime.queueEventLocked(sessionID, realtime.EventCombatStateSnapshot, payload)
		}
	}
}

func (runtime *Runtime) queueCombatEngagementStoppedLocked(playerID foundation.PlayerID, sessionIDs []auth.SessionID, reason combatStopReason, now time.Time) {
	snapshot := runtime.stopCombatEngagementLocked(playerID, reason, now)
	payload := runtime.combatEngagementPayloadFromSnapshotLocked(snapshot)
	for _, sessionID := range sessionIDs {
		runtime.queueEventLocked(sessionID, realtime.EventCombatAttackStopped, payload)
		runtime.queueEventLocked(sessionID, realtime.EventCombatStateSnapshot, payload)
	}
}

func combatEngagementTickRequestID(playerID foundation.PlayerID, targetID world.EntityID, now time.Time) foundation.RequestID {
	return foundation.RequestID(fmt.Sprintf("combat-tick-%s-%s-%d", playerID.String(), targetID.String(), now.UTC().UnixNano()))
}

func combatEngagementStopReasonForError(err error) (combatStopReason, bool) {
	if err == nil {
		return "", false
	}
	if foundation.IsCode(err, foundation.CodeCooldown) {
		return combatStopReasonCooldown, false
	}
	if foundation.IsCode(err, foundation.CodeNotVisible) {
		return combatStopReasonTargetNotVisible, true
	}
	if foundation.IsCode(err, foundation.CodeOutOfRange) {
		return combatStopReasonOutOfRange, true
	}
	if foundation.IsCode(err, foundation.CodeNotEnoughEnergy) {
		return combatStopReasonNotEnoughEnergy, true
	}
	if foundation.IsCode(err, foundation.CodeNotEnoughAmmo) {
		return combatStopReasonNotEnoughAmmo, true
	}
	if foundation.IsCode(err, foundation.CodeShipDisabled) {
		return combatStopReasonShipDisabled, true
	}
	if foundation.IsCode(err, foundation.CodePVPBlocked) || foundation.IsCode(err, foundation.CodeForbidden) {
		return combatStopReasonPolicyBlocked, true
	}
	if foundation.IsCode(err, foundation.CodeNotFound) {
		return combatStopReasonTargetLost, true
	}
	return combatStopReasonTargetLost, true
}

func combatEngagementPayloadFromState(state combatEngagementState, lastStopReason string) map[string]any {
	return map[string]any{
		"active":           true,
		"target_id":        state.TargetID.String(),
		"skill_id":         state.SkillID,
		"started_at_ms":    state.StartedAt.UTC().UnixMilli(),
		"next_fire_at_ms":  state.NextFireAt.UTC().UnixMilli(),
		"last_stop_reason": lastStopReason,
	}
}

func combatEngagementPayloadFromSnapshot(snapshot combatEngagementSnapshot) map[string]any {
	if !snapshot.Active {
		return map[string]any{
			"active":           false,
			"target_id":        "",
			"skill_id":         "",
			"started_at_ms":    int64(0),
			"next_fire_at_ms":  int64(0),
			"last_stop_reason": snapshot.LastStopReason,
		}
	}
	return combatEngagementPayloadFromState(combatEngagementState{
		PlayerID:       snapshot.PlayerID,
		TargetID:       snapshot.TargetID,
		SkillID:        snapshot.SkillID,
		StartedAt:      snapshot.StartedAt,
		NextFireAt:     snapshot.NextFireAt,
		LastStopReason: snapshot.LastStopReason,
	}, snapshot.LastStopReason)
}

func (runtime *Runtime) combatEngagementPayloadLocked(playerID foundation.PlayerID, now time.Time) map[string]any {
	return runtime.combatEngagementPayloadFromSnapshotLocked(runtime.combatEngagementSnapshotLocked(playerID, now))
}

func (runtime *Runtime) combatEngagementPayloadForStateLocked(state combatEngagementState, lastStopReason string) map[string]any {
	payload := combatEngagementPayloadFromState(state, lastStopReason)
	payload["active_ammo"] = runtime.combatAmmoStatePayloadLocked(state.PlayerID)
	return payload
}

func (runtime *Runtime) combatEngagementPayloadFromSnapshotLocked(snapshot combatEngagementSnapshot) map[string]any {
	payload := combatEngagementPayloadFromSnapshot(snapshot)
	payload["active_ammo"] = runtime.combatAmmoStatePayloadLocked(snapshot.PlayerID)
	return payload
}
