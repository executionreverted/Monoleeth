package server

import (
	"encoding/json"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/worker"
)

const (
	basicLaserSkillID = "basic_laser"
	trainingNPCType   = "training_drone"
	repairCurrency    = "credits"
)

type combatUseSkillIntent struct {
	SkillID  string         `json:"skill_id"`
	TargetID world.EntityID `json:"target_id"`
}

type lootPickupIntent struct {
	DropID world.EntityID `json:"drop_id"`
}

type repairAttemptRecord struct {
	ReferenceKey foundation.IdempotencyKey
	Ship         shipSnapshotPayload
	Wallet       walletSnapshotPayload
	Repaired     bool
	RepairCost   int64
}

type repairQuotePayload struct {
	ShipID   string `json:"ship_id"`
	Currency string `json:"currency"`
	Cost     int64  `json:"cost"`
	Disabled bool   `json:"disabled"`
}

func (runtime *Runtime) handleCombatUseSkill(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	intent, err := decodeCombatUseSkillIntent(request.Payload)
	if err != nil {
		return nil, err
	}
	if intent.SkillID != basicLaserSkillID {
		return nil, foundation.NewDomainError(foundation.CodeInvalidPayload, "Unsupported combat skill.")
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
	if err := runtime.syncWorldCombatActorLocked(intent.TargetID); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if !runtime.entityVisibleToPlayerLocked(ctx.PlayerID, intent.TargetID) {
		return nil, foundation.NewDomainError(foundation.CodeNotVisible, "Target is not visible.")
	}

	result, err := runtime.Combat.ExecuteBasicAttack(combat.BasicAttackInput{
		AttackerID: attacker.EntityID,
		TargetID:   intent.TargetID,
	})
	if err != nil {
		return nil, domainErrorForCombat(err)
	}

	state := runtime.players[ctx.PlayerID]
	state.Ship.Capacitor = roundCombatValue(result.Attacker.Energy)
	state.Ship.Hull = roundCombatValue(result.Attacker.HP)
	state.Ship.Shield = roundCombatValue(result.Attacker.Shield)
	runtime.players[ctx.PlayerID] = state

	sessionID := authSessionID(ctx.SessionID)
	runtime.queueEventLocked(sessionID, realtime.EventPlayerSnapshot, state.playerSnapshot())
	runtime.queueEventLocked(sessionID, realtime.EventShipSnapshot, state.Ship)
	runtime.queueEventLocked(sessionID, realtime.EventCombatCooldownStarted, map[string]any{
		"skill_id":             intent.SkillID,
		"target_id":            intent.TargetID.String(),
		"cooldown_ready_at_ms": result.CooldownReadyAt.UTC().UnixMilli(),
	})
	if result.Hit {
		runtime.queueEventLocked(sessionID, realtime.EventCombatDamage, map[string]any{
			"target_id":     intent.TargetID.String(),
			"amount":        roundCombatValue(result.Damage),
			"shield_amount": roundCombatValue(result.ShieldDamage),
			"hull_amount":   roundCombatValue(result.HPDamage),
		})
	} else {
		runtime.queueEventLocked(sessionID, realtime.EventCombatMiss, map[string]any{
			"target_id": intent.TargetID.String(),
		})
	}
	runtime.queueTargetUpdatedLocked(sessionID, result.Target)

	var drops []loot.Drop
	var progressionSnapshot *progressionSnapshotPayload
	if result.KillEvent != nil {
		runtime.queueEventLocked(sessionID, realtime.EventCombatNPCKilled, map[string]any{
			"entity_id": result.KillEvent.NPCEntityID.String(),
			"npc_type":  result.KillEvent.NPCType,
		})
		if xpResult, err := runtime.combatXP.GrantNPCKillXP(*result.KillEvent); err == nil {
			payload := progressionPayload(xpResult.Snapshot)
			progressionSnapshot = &payload
			runtime.queueEventLocked(sessionID, realtime.EventProgressionSnapshot, payload)
		}
		created, err := runtime.Loot.CreateDropsForNPCKill(*result.KillEvent, runtime.lootTable)
		if err != nil {
			return nil, domainErrorForRuntime(err)
		}
		drops = created.Drops
		for _, drop := range drops {
			if err := runtime.insertLootDropEntityLocked(drop); err != nil {
				return nil, domainErrorForRuntime(err)
			}
			runtime.queueEventLocked(sessionID, realtime.EventLootCreated, lootDropPayload(drop, runtime.clock.Now()))
		}
		if !runtime.Worker.RemoveEntity(intent.TargetID) {
			return nil, domainErrorForRuntime(worker.ErrUnknownEntity)
		}
		runtime.hidden[intent.TargetID] = true
	}

	response := map[string]any{
		"accepted":             true,
		"skill_id":             intent.SkillID,
		"target_id":            intent.TargetID.String(),
		"hit":                  result.Hit,
		"amount":               roundCombatValue(result.Damage),
		"killed":               result.Killed,
		"cooldown_ready_at_ms": result.CooldownReadyAt.UTC().UnixMilli(),
		"ship":                 state.Ship,
		"player":               state.playerSnapshot(),
	}
	if targetStatus := combatStatusFromActor(result.Target); targetStatus != nil {
		response["target"] = targetStatus
	}
	if len(drops) > 0 {
		response["drops"] = lootDropPayloads(drops, runtime.clock.Now())
	}
	if progressionSnapshot != nil {
		response["progression"] = *progressionSnapshot
	}
	return marshalPayload(response)
}

func (runtime *Runtime) handleLootPickup(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	intent, err := decodeLootPickupIntent(request.Payload)
	if err != nil {
		return nil, err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if err := runtime.validateShipCanActLocked(ctx.PlayerID); err != nil {
		return nil, err
	}
	viewer, err := runtime.viewerForPlayerLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if !runtime.entityVisibleToPlayerLocked(ctx.PlayerID, intent.DropID) {
		return nil, foundation.NewDomainError(foundation.CodeNotVisible, "Drop is not visible.")
	}

	result, err := runtime.Loot.PickupDrop(loot.PickupInput{
		PlayerID:           ctx.PlayerID,
		DropID:             intent.DropID,
		Viewer:             viewer,
		ActiveCargo:        runtime.activeCargoLocationLocked(ctx.PlayerID),
		CargoCapacityUnits: runtime.players[ctx.PlayerID].Cargo.Capacity,
	})
	if err != nil {
		return nil, domainErrorForLoot(err)
	}
	runtime.Worker.RemoveEntity(intent.DropID)
	runtime.hidden[intent.DropID] = true

	state := runtime.players[ctx.PlayerID]
	state.Cargo = runtime.cargoSnapshotFromInventoryLocked(ctx.PlayerID)
	runtime.players[ctx.PlayerID] = state

	sessionID := authSessionID(ctx.SessionID)
	runtime.queueEventLocked(sessionID, realtime.EventLootPickedUp, map[string]any{
		"drop_id":  result.Drop.ID.String(),
		"item_id":  result.Drop.ItemDefinition.ItemID.String(),
		"quantity": result.Drop.Quantity,
	})
	runtime.queueEventLocked(sessionID, realtime.EventLootRemoved, map[string]any{
		"entity_id": result.Drop.ID.String(),
	})
	runtime.queueEventLocked(sessionID, realtime.EventCargoSnapshot, state.Cargo)
	runtime.queueEventLocked(sessionID, realtime.EventInventorySnapshot, runtime.inventorySnapshotLocked(ctx.PlayerID))
	if result.XPResult != nil {
		payload := progressionPayload(result.XPResult.Snapshot)
		runtime.queueEventLocked(sessionID, realtime.EventProgressionSnapshot, payload)
	}

	return marshalPayload(map[string]any{
		"accepted":  true,
		"drop_id":   result.Drop.ID.String(),
		"cargo":     state.Cargo,
		"inventory": runtime.inventorySnapshotLocked(ctx.PlayerID),
	})
}

func (runtime *Runtime) handleDeathRepairQuote(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	state, ok := runtime.players[ctx.PlayerID]
	if !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	return marshalPayload(runtime.repairQuoteLocked(state))
}

func (runtime *Runtime) handleDeathRepairShip(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	state, ok := runtime.players[ctx.PlayerID]
	if !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	if !state.Ship.Disabled {
		return nil, foundation.NewDomainError(foundation.CodeShipDisabled, "Ship is not disabled.")
	}
	referenceKey, err := foundation.ShipRepairIdempotencyKey(foundation.ShipID(state.Ship.ActiveShipID), request.RequestID.String())
	if err != nil {
		return nil, invalidPayload("Repair reference is invalid.", err)
	}
	if previous, ok := runtime.repairAttempts[referenceKey]; ok {
		return marshalPayload(map[string]any{
			"accepted":    true,
			"duplicate":   true,
			"repaired":    previous.Repaired,
			"repair_cost": previous.RepairCost,
			"ship":        previous.Ship,
			"wallet":      previous.Wallet,
		})
	}

	quote := runtime.repairQuoteLocked(state)
	if state.Wallet.Credits < quote.Cost {
		return nil, foundation.NewDomainError(foundation.CodeNotEnoughFunds, "Not enough credits.")
	}
	state.Wallet.Credits -= quote.Cost
	state.Ship.Disabled = false
	state.Ship.RepairState = "ready"
	state.Ship.Hull = state.Ship.MaxHull
	state.Ship.Shield = state.Ship.MaxShield
	state.Ship.Capacitor = state.Ship.MaxCapacitor
	runtime.players[ctx.PlayerID] = state
	if _, err := runtime.syncPlayerCombatActorLocked(ctx.PlayerID); err != nil {
		return nil, domainErrorForRuntime(err)
	}

	record := repairAttemptRecord{
		ReferenceKey: referenceKey,
		Ship:         state.Ship,
		Wallet:       state.Wallet,
		Repaired:     true,
		RepairCost:   quote.Cost,
	}
	runtime.repairAttempts[referenceKey] = record

	sessionID := authSessionID(ctx.SessionID)
	runtime.queueEventLocked(sessionID, realtime.EventDeathRepaired, map[string]any{
		"ship_id":     state.Ship.ActiveShipID,
		"repair_cost": quote.Cost,
		"currency":    repairCurrency,
	})
	runtime.queueEventLocked(sessionID, realtime.EventShipSnapshot, state.Ship)
	runtime.queueEventLocked(sessionID, realtime.EventPlayerSnapshot, state.playerSnapshot())
	runtime.queueEventLocked(sessionID, realtime.EventWalletSnapshot, state.Wallet)

	return marshalPayload(map[string]any{
		"accepted":    true,
		"repaired":    true,
		"repair_cost": quote.Cost,
		"ship":        state.Ship,
		"wallet":      state.Wallet,
	})
}
