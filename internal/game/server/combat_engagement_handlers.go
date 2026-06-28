package server

import (
	"encoding/json"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/worker"
)

type combatStartAttackIntent struct {
	TargetID world.EntityID `json:"target_id"`
}

func decodeCombatStartAttackIntent(raw json.RawMessage) (combatStartAttackIntent, error) {
	var intent combatStartAttackIntent
	if err := decodeStrict(raw, &intent); err != nil {
		return combatStartAttackIntent{}, err
	}
	if err := intent.TargetID.Validate(); err != nil {
		return combatStartAttackIntent{}, invalidPayload("target_id is required.", err)
	}
	return intent, nil
}

func (runtime *Runtime) handleCombatStartAttack(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	intent, err := decodeCombatStartAttackIntent(request.Payload)
	if err != nil {
		return nil, err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if err := runtime.validateShipCanActLocked(ctx.PlayerID); err != nil {
		return nil, err
	}
	attacker, err := runtime.syncPlayerCombatActorLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if err := runtime.syncWorldCombatActorLocked(ctx.PlayerID, intent.TargetID); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	viewer, err := runtime.viewerForPlayerLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if !runtime.entityVisibleToPlayerLocked(ctx.PlayerID, intent.TargetID) {
		return nil, foundation.NewDomainError(foundation.CodeNotVisible, "Target is not visible.")
	}
	if _, ok := runtime.Combat.Actor(intent.TargetID); !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownEntity)
	}
	if err := runtime.Combat.CanBasicAttack(combat.BasicAttackInput{
		AttackerID: attacker.EntityID,
		TargetID:   intent.TargetID,
		Viewer:     &viewer,
		Policy:     runtime.basicAttackPolicyLocked(),
	}); err != nil {
		return nil, domainErrorForCombat(err)
	}

	now := runtime.clock.Now()
	state := runtime.startCombatEngagementLocked(ctx.PlayerID, intent.TargetID, defaultCombatEngagementSkillID, now)
	payload := runtime.combatEngagementPayloadForStateLocked(state, "")
	sessionID := authSessionID(ctx.SessionID)
	runtime.queueEventLocked(sessionID, realtime.EventCombatAttackStarted, payload)
	runtime.queueEventLocked(sessionID, realtime.EventCombatStateSnapshot, payload)

	return marshalPayload(map[string]any{
		"accepted":  true,
		"target_id": state.TargetID.String(),
		"skill_id":  state.SkillID,
	})
}

func (runtime *Runtime) handleCombatStopAttack(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if err := decodeStrict(request.Payload, &struct{}{}); err != nil {
		return nil, err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	now := runtime.clock.Now()
	snapshot := runtime.stopCombatEngagementLocked(ctx.PlayerID, combatStopReasonManual, now)
	payload := runtime.combatEngagementPayloadFromSnapshotLocked(snapshot)
	sessionID := authSessionID(ctx.SessionID)
	runtime.queueEventLocked(sessionID, realtime.EventCombatAttackStopped, payload)
	runtime.queueEventLocked(sessionID, realtime.EventCombatStateSnapshot, payload)

	return marshalPayload(map[string]any{
		"accepted": true,
		"reason":   string(combatStopReasonManual),
	})
}

func (runtime *Runtime) handleCombatState(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if err := decodeStrict(request.Payload, &struct{}{}); err != nil {
		return nil, err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return marshalPayload(runtime.combatEngagementPayloadLocked(ctx.PlayerID, runtime.clock.Now()))
}
