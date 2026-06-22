package server

import (
	"strings"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

const (
	protectionReasonPortal    = "portal"
	protectionReasonRespawn   = "respawn"
	protectionReasonPVPAction = "pvp_action"
)

type protectionKey struct {
	PlayerID foundation.PlayerID
	MapID    worldmaps.MapID
}

type playerProtectionState struct {
	PlayerID         foundation.PlayerID
	InternalMapID    worldmaps.MapID
	PublicMapKey     worldmaps.PublicMapKey
	Reason           string
	ExpiresAt        time.Time
	BlocksPVP        bool
	BreakOnPVPAction bool
}

type playerProtectionUpdatedPayload struct {
	Reason           string `json:"reason"`
	PublicMapKey     string `json:"public_map_key"`
	ExpiresAt        int64  `json:"expires_at"`
	BlocksPVP        bool   `json:"blocks_pvp"`
	BreakOnPVPAction bool   `json:"break_on_pvp_action"`
}

func (runtime *Runtime) startPortalProtectionLocked(playerID foundation.PlayerID, mapID worldmaps.MapID) (playerProtectionState, error) {
	return runtime.startPlayerProtectionLocked(playerID, mapID, protectionReasonPortal)
}

func (runtime *Runtime) startPlayerProtectionLocked(playerID foundation.PlayerID, mapID worldmaps.MapID, reason string) (playerProtectionState, error) {
	definition, ok := runtime.mapCatalog.Get(mapID)
	if !ok {
		return playerProtectionState{}, worldmaps.ErrMapNotFound
	}
	runtime.clearPlayerProtectionsExceptLocked(playerID, mapID)
	state := playerProtectionState{
		PlayerID:         playerID,
		InternalMapID:    mapID,
		PublicMapKey:     definition.PublicMapKey,
		Reason:           reason,
		ExpiresAt:        runtime.clock.Now().Add(runtimePortalProtectionDuration).UTC(),
		BlocksPVP:        true,
		BreakOnPVPAction: true,
	}
	runtime.playerProtections[protectionKey{PlayerID: playerID, MapID: mapID}] = state
	return state, nil
}

func (runtime *Runtime) clearPlayerProtectionsExceptLocked(playerID foundation.PlayerID, keepMapID worldmaps.MapID) {
	for key := range runtime.playerProtections {
		if key.PlayerID == playerID && key.MapID != keepMapID {
			delete(runtime.playerProtections, key)
		}
	}
}

func (runtime *Runtime) clearProtectionLocked(playerID foundation.PlayerID, mapID worldmaps.MapID) {
	delete(runtime.playerProtections, protectionKey{PlayerID: playerID, MapID: mapID})
}

func (runtime *Runtime) activeProtectionLocked(playerID foundation.PlayerID) (playerProtectionState, bool) {
	location, err := runtime.mapRouter.ActiveLocation(playerID)
	if err != nil {
		return playerProtectionState{}, false
	}
	return runtime.protectionForMapLocked(playerID, location.InternalMapID)
}

func (runtime *Runtime) protectionForMapLocked(playerID foundation.PlayerID, mapID worldmaps.MapID) (playerProtectionState, bool) {
	key := protectionKey{PlayerID: playerID, MapID: mapID}
	state, ok := runtime.playerProtections[key]
	if !ok {
		return playerProtectionState{}, false
	}
	if !state.ExpiresAt.After(runtime.clock.Now()) {
		delete(runtime.playerProtections, key)
		return playerProtectionState{}, false
	}
	return state, true
}

func (runtime *Runtime) queueProtectionUpdatedLocked(sessionIDs []auth.SessionID, state playerProtectionState) {
	payload := state.eventPayload()
	for _, sessionID := range sessionIDs {
		runtime.queueEventLocked(sessionID, realtime.EventPlayerProtection, payload)
	}
}

func (runtime *Runtime) queueProtectionClearedLocked(playerID foundation.PlayerID, state playerProtectionState, reason string) {
	state.Reason = reason
	state.ExpiresAt = runtime.clock.Now().UTC()
	state.BlocksPVP = false
	state.BreakOnPVPAction = false
	runtime.queueProtectionUpdatedLocked(runtime.sessionIDsForPlayerLocked(playerID, ""), state)
}

func (state playerProtectionState) eventPayload() playerProtectionUpdatedPayload {
	return playerProtectionUpdatedPayload{
		Reason:           state.Reason,
		PublicMapKey:     state.PublicMapKey.String(),
		ExpiresAt:        state.ExpiresAt.UTC().UnixMilli(),
		BlocksPVP:        state.BlocksPVP,
		BreakOnPVPAction: state.BreakOnPVPAction,
	}
}

func (state playerProtectionState) clientSummary() worldmaps.ClientProtectionSummary {
	return worldmaps.ClientProtectionSummary{
		Reason:           state.Reason,
		ExpiresAt:        state.ExpiresAt.UTC().UnixMilli(),
		BlocksPVP:        state.BlocksPVP,
		BreakOnPVPAction: state.BreakOnPVPAction,
	}
}

func (runtime *Runtime) mapProjectionWithViewerPolicyLocked(playerID foundation.PlayerID, projection worldmaps.ClientMapProjection) worldmaps.ClientMapProjection {
	instance, _, err := runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return projection
	}
	entity, ok := instance.Worker.PlayerEntity(playerID)
	if !ok {
		return projection
	}
	var protectionExpiresAt int64
	if protection, ok := runtime.activeProtectionLocked(playerID); ok {
		projection.Protection = ptr(protection.clientSummary())
		protectionExpiresAt = protection.ExpiresAt.UTC().UnixMilli()
	}
	if safeZone, ok := instance.Definition.PVPBlockingSafeZoneAt(entity.Position); ok {
		projection.SafeZone = &worldmaps.ClientSafeZoneSummary{
			Inside:              true,
			BlocksPVP:           safeZone.BlocksPVP,
			ProtectionExpiresAt: protectionExpiresAt,
		}
	}
	return projection
}

func (runtime *Runtime) basicAttackPolicyLocked() combat.AttackPolicy {
	return combat.AttackPolicyFunc(func(input combat.AttackPolicyInput) error {
		return runtime.validateBasicAttackPolicyLocked(input)
	})
}

func (runtime *Runtime) validateBasicAttackPolicyLocked(input combat.AttackPolicyInput) error {
	if input.Attacker.Type != world.EntityTypePlayer || input.Target.Type != world.EntityTypePlayer {
		return nil
	}
	if input.Attacker.PlayerID.IsZero() || input.Target.PlayerID.IsZero() {
		return combat.ErrPVPBlocked
	}
	if input.Attacker.PlayerID == input.Target.PlayerID {
		return combat.ErrPVPBlocked
	}
	attackerLocation, err := runtime.mapRouter.ActiveLocation(input.Attacker.PlayerID)
	if err != nil {
		return combat.ErrPVPBlocked
	}
	targetLocation, err := runtime.mapRouter.ActiveLocation(input.Target.PlayerID)
	if err != nil {
		return combat.ErrPVPBlocked
	}
	if attackerLocation.InternalMapID != targetLocation.InternalMapID {
		return combat.ErrPVPBlocked
	}
	instance, err := runtime.mapInstanceForLocationLocked(attackerLocation)
	if err != nil {
		return combat.ErrPVPBlocked
	}
	if attackerLocation.WorldID != input.Attacker.WorldID ||
		attackerLocation.ZoneID != input.Attacker.ZoneID ||
		targetLocation.WorldID != input.Target.WorldID ||
		targetLocation.ZoneID != input.Target.ZoneID {
		return combat.ErrPVPBlocked
	}

	if protection, ok := runtime.protectionForMapLocked(input.Attacker.PlayerID, attackerLocation.InternalMapID); ok {
		if protection.BreakOnPVPAction {
			runtime.clearProtectionLocked(input.Attacker.PlayerID, attackerLocation.InternalMapID)
			runtime.queueProtectionClearedLocked(input.Attacker.PlayerID, protection, protectionReasonPVPAction)
		}
		return combat.ErrPVPBlocked
	}
	if _, ok := runtime.protectionForMapLocked(input.Target.PlayerID, targetLocation.InternalMapID); ok {
		return combat.ErrPVPBlocked
	}
	if _, ok := instance.Definition.PVPBlockingSafeZoneAt(input.Attacker.Position); ok {
		return combat.ErrPVPBlocked
	}
	if _, ok := instance.Definition.PVPBlockingSafeZoneAt(input.Target.Position); ok {
		return combat.ErrPVPBlocked
	}
	if !mapPolicyAllowsPVP(instance.Definition.PVPPolicy) {
		return combat.ErrPVPBlocked
	}
	return nil
}

func mapPolicyAllowsPVP(policy string) bool {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "pvp", "contested", "enabled":
		return true
	default:
		return false
	}
}

func (runtime *Runtime) syncPlayerTargetCombatActorLocked(entity world.Entity) error {
	targetPlayerID, state, ok := runtime.playerByEntityLocked(entity.ID)
	if !ok {
		return worker.ErrUnknownPlayer
	}
	hidden := false
	if instance, _, err := runtime.activeMapInstanceLocked(targetPlayerID); err == nil {
		hidden = instance.HiddenPlayers[targetPlayerID] || instance.HiddenEntities[entity.ID]
	}
	signature, stealthScore, jammerStrength := runtime.visibilityInputsForEntityLocked(entity, targetPlayerID, hidden)
	actor := combat.ActorState{
		EntityID:       entity.ID,
		Type:           world.EntityTypePlayer,
		PlayerID:       targetPlayerID,
		WorldID:        entity.WorldID,
		ZoneID:         entity.ZoneID,
		Position:       entity.Position,
		Signature:      signature,
		StealthScore:   stealthScore,
		JammerStrength: jammerStrength,
		Hidden:         hidden,
		Stats:          runtime.playerCombatStatsLocked(targetPlayerID, state),
		HP:             float64(state.Ship.Hull),
		Shield:         float64(state.Ship.Shield),
		Energy:         float64(state.Ship.Capacitor),
		Cooldowns:      combat.CooldownState{},
		Contributions:  make(map[foundation.PlayerID]float64),
	}
	if existing, ok := runtime.Combat.Actor(entity.ID); ok {
		actor.Cooldowns = existing.Cooldowns
		actor.Contributions = existing.Contributions
	}
	if state.Ship.Disabled {
		actor.Dead = true
		actor.HP = 0
		now := runtime.clock.Now()
		actor.DiedAt = &now
	}
	return runtime.Combat.UpsertActor(actor)
}

func ptr[T any](value T) *T {
	return &value
}
