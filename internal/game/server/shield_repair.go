package server

import (
	"encoding/json"
	"math"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world/worker"
)

const (
	shieldRepairCombatLockDuration = 8 * time.Second
	shieldRepairMinTickInterval    = time.Second
)

type shieldRepairTickPayload struct {
	Accepted     bool    `json:"accepted"`
	Repaired     bool    `json:"repaired"`
	ShieldBefore int     `json:"shield_before"`
	ShieldAfter  int     `json:"shield_after"`
	MaxShield    int     `json:"max_shield"`
	RepairRate   float64 `json:"repair_rate"`
	ElapsedMS    int64   `json:"elapsed_ms"`
	NextTickAtMS int64   `json:"next_tick_at_ms"`
}

func (runtime *Runtime) handleShieldRepairTick(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectEmptyIntentPayload(request.Payload, "shield", "max_shield", "repair_rate", "elapsed_ms", "combat_lock_until", "combat_locked"); err != nil {
		return nil, err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	state, ok := runtime.players[ctx.PlayerID]
	if !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	if state.Ship.Disabled || state.Ship.Hull <= 0 {
		return nil, foundation.NewDomainError(foundation.CodeShipDisabled, "Ship is disabled.")
	}
	if state.Ship.Shield >= state.Ship.MaxShield {
		now := runtime.clock.Now()
		return marshalPayload(shieldRepairTickPayload{
			Accepted:     true,
			Repaired:     false,
			ShieldBefore: state.Ship.Shield,
			ShieldAfter:  state.Ship.Shield,
			MaxShield:    state.Ship.MaxShield,
			NextTickAtMS: now.Add(shieldRepairMinTickInterval).UTC().UnixMilli(),
		})
	}

	now := runtime.clock.Now()
	if _, locked := runtime.playerCombatLockLocked(ctx.PlayerID, now); locked {
		return nil, foundation.NewDomainError(
			foundation.CodeCooldown,
			"Shield repair is locked during combat.",
		)
	}

	repairRate, err := runtime.equippedShieldRepairRateLocked(ctx.PlayerID, foundation.ShipID(state.Ship.ActiveShipID))
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if repairRate <= 0 {
		return nil, foundation.NewDomainError(foundation.CodeForbidden, "No equipped shield repair module.")
	}

	lastTick, hasLastTick := runtime.shieldRepairTicks[ctx.PlayerID]
	if hasLastTick && now.Sub(lastTick) < shieldRepairMinTickInterval {
		return nil, foundation.NewDomainError(
			foundation.CodeCooldown,
			"Shield repair tick is cooling down.",
		)
	}

	elapsed := shieldRepairMinTickInterval
	if hasLastTick {
		elapsed = now.Sub(lastTick)
	}
	if elapsed < shieldRepairMinTickInterval {
		elapsed = shieldRepairMinTickInterval
	}
	repairAmount := int(math.Floor(repairRate * elapsed.Seconds()))
	if repairAmount < 1 {
		repairAmount = 1
	}

	before := state.Ship.Shield
	state.Ship.Shield = min(before+repairAmount, state.Ship.MaxShield)
	runtime.players[ctx.PlayerID] = state
	runtime.shieldRepairTicks[ctx.PlayerID] = now
	if actor, ok := runtime.Combat.Actor(state.EntityID); ok {
		actor.Shield = float64(state.Ship.Shield)
		_ = runtime.Combat.UpsertActor(actor)
	}

	sessionID := authSessionID(ctx.SessionID)
	runtime.queueEventLocked(sessionID, realtime.EventShipSnapshot, state.Ship)
	runtime.queueEventLocked(sessionID, realtime.EventPlayerSnapshot, state.playerSnapshot())

	return marshalPayload(shieldRepairTickPayload{
		Accepted:     true,
		Repaired:     state.Ship.Shield > before,
		ShieldBefore: before,
		ShieldAfter:  state.Ship.Shield,
		MaxShield:    state.Ship.MaxShield,
		RepairRate:   repairRate,
		ElapsedMS:    elapsed.Milliseconds(),
		NextTickAtMS: now.Add(shieldRepairMinTickInterval).UTC().UnixMilli(),
	})
}

func (runtime *Runtime) refreshCombatLockForActorLocked(actorPlayerID foundation.PlayerID, now time.Time) {
	if actorPlayerID.IsZero() {
		return
	}
	runtime.combatLocks[actorPlayerID] = now.Add(shieldRepairCombatLockDuration)
}

func (runtime *Runtime) playerCombatLockLocked(playerID foundation.PlayerID, now time.Time) (time.Time, bool) {
	lockUntil, ok := runtime.combatLocks[playerID]
	if !ok {
		return time.Time{}, false
	}
	if !lockUntil.After(now) {
		delete(runtime.combatLocks, playerID)
		return time.Time{}, false
	}
	return lockUntil, true
}

func (runtime *Runtime) equippedShieldRepairRateLocked(playerID foundation.PlayerID, shipID foundation.ShipID) (float64, error) {
	equipped, err := runtime.LoadoutStore.EquippedModules(playerID, shipID)
	if err != nil {
		return 0, err
	}
	rate := 0.0
	for _, equippedModule := range equipped {
		item, err := runtime.LoadoutStore.ModuleItem(equippedModule.ItemInstanceID)
		if err != nil {
			return 0, err
		}
		if item.DurabilityCurrent <= 0 {
			continue
		}
		definition, ok := runtime.ModuleCatalog.Lookup(item.ItemID)
		if !ok {
			return 0, modules.ErrUnknownModuleDefinition
		}
		for _, modifier := range definition.StatModifiers {
			if modifier.Stat != modules.StatShieldRegen {
				continue
			}
			switch modifier.Kind {
			case modules.StatModifierFlat:
				rate += float64(modifier.Value)
			case modules.StatModifierPercent:
				rate *= 1 + float64(modifier.Value)/10_000
			}
		}
	}
	return rate, nil
}
