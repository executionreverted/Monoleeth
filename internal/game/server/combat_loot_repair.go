package server

import (
	"encoding/json"
	"fmt"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
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
	PublicMapKey string
	Position     world.Vec2
	Protection   worldmaps.ClientProtectionSummary
	Repaired     bool
	RepairCost   int64
}

type repairQuotePayload struct {
	ShipID   string `json:"ship_id"`
	Currency string `json:"currency"`
	Cost     int64  `json:"cost"`
	Disabled bool   `json:"disabled"`
}

func runtimeShipRepairIdempotencyKey(playerID foundation.PlayerID, shipID foundation.ShipID, requestID foundation.RequestID) (foundation.IdempotencyKey, error) {
	if err := playerID.Validate(); err != nil {
		return "", err
	}
	if err := requestID.Validate(); err != nil {
		return "", err
	}
	repairReference := fmt.Sprintf("player%d.%s.request%d.%s", len(playerID.String()), playerID.String(), len(requestID.String()), requestID.String())
	return foundation.ShipRepairIdempotencyKey(shipID, repairReference)
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
	attackerBefore := attacker
	targetBefore, ok := runtime.Combat.Actor(intent.TargetID)
	if !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownEntity)
	}
	restoreAttackActors := func() {
		_ = runtime.Combat.UpsertActor(attackerBefore)
		_ = runtime.Combat.UpsertActor(targetBefore)
	}

	result, err := runtime.Combat.ExecuteBasicAttack(combat.BasicAttackInput{
		AttackerID: attacker.EntityID,
		TargetID:   intent.TargetID,
		Viewer:     &viewer,
		Policy:     runtime.basicAttackPolicyLocked(),
	})
	if err != nil {
		return nil, domainErrorForCombat(err)
	}

	var drops []loot.Drop
	lethalPlayerDeath := isLethalPlayerCombatResult(targetBefore, result)
	if lethalPlayerDeath {
		playerDeathDrops, err := runtime.processLethalPVPDeathLocked(request.RequestID, result.Attacker, result.Target)
		if err != nil {
			return nil, domainErrorForRuntime(err)
		}
		drops = append(drops, playerDeathDrops...)
	}
	var progressionSnapshot *progressionSnapshotPayload
	var questUpdates []quests.PlayerQuest
	if result.KillEvent != nil {
		instance, _, err := runtime.activeMapInstanceLocked(ctx.PlayerID)
		if err != nil {
			restoreAttackActors()
			return nil, domainErrorForRuntime(err)
		}
		lootTable, err := runtime.selectNPCKillLootTableForInstanceLocked(instance, *result.KillEvent)
		if err != nil {
			restoreAttackActors()
			return nil, domainErrorForRuntime(err)
		}
		if err := runtime.submitWorkerCommandAndRecordMetricsLocked(instance, worker.MarkEnemyKilledCommand{
			Definition:  instance.Definition,
			NPCEntityID: result.KillEvent.NPCEntityID,
			KilledAt:    result.KillEvent.KilledAt,
		}); err != nil {
			restoreAttackActors()
			return nil, domainErrorForRuntime(err)
		}
		instance.HiddenEntities[result.KillEvent.NPCEntityID] = true

		if updated, err := runtime.Quest.ConsumeCombatNPCKilled(quests.CombatNPCKilledInput{
			EventID:          foundation.EventID("quest-combat-" + request.RequestID.String()),
			ProgressEventKey: quests.QuestProgressEventKey("combat.npc_killed:" + result.KillEvent.NPCEntityID.String()),
			PlayerID:         ctx.PlayerID,
			NPCType:          result.KillEvent.NPCType,
		}); err != nil {
			return nil, domainErrorForQuest(err)
		} else {
			questUpdates = updated
		}
		if xpResult, err := runtime.combatXP.GrantNPCKillXP(*result.KillEvent); err == nil {
			payload := progressionPayload(xpResult.Snapshot)
			progressionSnapshot = &payload
		}
		created, err := runtime.Loot.CreateDropsForNPCKill(*result.KillEvent, lootTable)
		if err != nil {
			return nil, domainErrorForRuntime(err)
		}
		drops = created.Drops
		for _, drop := range drops {
			if err := runtime.insertLootDropEntityLocked(drop); err != nil {
				return nil, domainErrorForRuntime(err)
			}
		}
	}

	state, ok := runtime.applyCombatActorToPlayerShipLocked(ctx.PlayerID, result.Attacker)
	if !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}

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
	if result.Target.Type == world.EntityTypePlayer && !result.Target.PlayerID.IsZero() && result.Target.PlayerID != ctx.PlayerID {
		if result.Hit {
			runtime.queueEventToPlayerSessionsLocked(result.Target.PlayerID, realtime.EventCombatDamage, map[string]any{
				"target_id":     result.Target.EntityID.String(),
				"amount":        roundCombatValue(result.Damage),
				"shield_amount": roundCombatValue(result.ShieldDamage),
				"hull_amount":   roundCombatValue(result.HPDamage),
			})
		} else {
			runtime.queueEventToPlayerSessionsLocked(result.Target.PlayerID, realtime.EventCombatMiss, map[string]any{
				"target_id": result.Target.EntityID.String(),
			})
		}
		if !lethalPlayerDeath {
			targetState, ok := runtime.applyCombatActorToPlayerShipLocked(result.Target.PlayerID, result.Target)
			if !ok {
				return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
			}
			runtime.queueEventToPlayerSessionsLocked(result.Target.PlayerID, realtime.EventPlayerSnapshot, targetState.playerSnapshot())
			runtime.queueEventToPlayerSessionsLocked(result.Target.PlayerID, realtime.EventShipSnapshot, targetState.Ship)
		}
		runtime.queueTargetUpdatedToPlayerSessionsLocked(result.Target.PlayerID, result.Target)
	}
	if result.KillEvent != nil {
		runtime.queueEventLocked(sessionID, realtime.EventCombatNPCKilled, map[string]any{
			"entity_id": result.KillEvent.NPCEntityID.String(),
			"npc_type":  result.KillEvent.NPCType,
		})
		runtime.queueQuestProgressEventsLocked(sessionID, questUpdates)
		if progressionSnapshot != nil {
			runtime.queueEventLocked(sessionID, realtime.EventProgressionSnapshot, *progressionSnapshot)
		}
		for _, drop := range drops {
			runtime.queueEventLocked(sessionID, realtime.EventLootCreated, lootDropPayload(drop, runtime.clock.Now()))
		}
	}
	if lethalPlayerDeath {
		for _, drop := range drops {
			runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventLootCreated, lootDropPayload(drop, runtime.clock.Now()))
		}
	}
	targetKilled := result.Killed || lethalPlayerDeath

	response := map[string]any{
		"accepted":             true,
		"skill_id":             intent.SkillID,
		"target_id":            intent.TargetID.String(),
		"hit":                  result.Hit,
		"amount":               roundCombatValue(result.Damage),
		"killed":               targetKilled,
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
	instance, _, err := runtime.activeMapInstanceLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	instance.Worker.RemoveEntity(intent.DropID)
	instance.HiddenEntities[intent.DropID] = true

	state := runtime.players[ctx.PlayerID]
	state.Cargo = runtime.cargoSnapshotFromInventoryLocked(ctx.PlayerID)
	runtime.players[ctx.PlayerID] = state

	sessionID := authSessionID(ctx.SessionID)
	runtime.queueEventLocked(sessionID, realtime.EventLootPickedUp, map[string]any{
		"drop_id":  result.Drop.ID.String(),
		"item_id":  result.Drop.ItemDefinition.ItemID.String(),
		"quantity": result.Drop.Quantity,
	})
	quantity, err := foundation.NewQuantity(result.Drop.Quantity)
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	if updated, err := runtime.Quest.ConsumeLootPickedUp(quests.LootPickedUpInput{
		EventID:          foundation.EventID("quest-loot-" + request.RequestID.String()),
		ProgressEventKey: quests.QuestProgressEventKey("loot.picked_up:" + result.Drop.ID.String()),
		PlayerID:         ctx.PlayerID,
		ItemID:           result.Drop.ItemDefinition.ItemID,
		Quantity:         quantity,
	}); err != nil {
		return nil, domainErrorForQuest(err)
	} else {
		runtime.queueQuestProgressEventsLocked(sessionID, updated)
	}
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
	referenceKey, err := runtimeShipRepairIdempotencyKey(ctx.PlayerID, foundation.ShipID(state.Ship.ActiveShipID), request.RequestID)
	if err != nil {
		return nil, invalidPayload("Repair reference is invalid.", err)
	}
	if previous, ok := runtime.repairAttempts[referenceKey]; ok {
		return marshalPayload(map[string]any{
			"accepted":       true,
			"duplicate":      true,
			"repaired":       previous.Repaired,
			"repair_cost":    previous.RepairCost,
			"ship":           previous.Ship,
			"wallet":         previous.Wallet,
			"public_map_key": previous.PublicMapKey,
			"position":       previous.Position,
			"protection":     previous.Protection,
		})
	}
	if !state.Ship.Disabled {
		return nil, foundation.NewDomainError(foundation.CodeShipDisabled, "Ship is not disabled.")
	}

	quote := runtime.repairQuoteLocked(state)
	wallet := runtime.walletSnapshotLocked(ctx.PlayerID)
	if wallet.Credits < quote.Cost {
		return nil, foundation.NewDomainError(foundation.CodeNotEnoughFunds, "Not enough credits.")
	}
	if quote.Cost > 0 {
		if _, err := runtime.Wallet.DebitWallet(economy.DebitWalletInput{
			PlayerID:     ctx.PlayerID,
			Currency:     economy.CurrencyBucketCredits,
			Amount:       quote.Cost,
			Reason:       "ship_repair",
			ReferenceKey: referenceKey,
		}); err != nil {
			return nil, domainErrorForEconomy(err)
		}
	}
	state.Wallet = runtime.walletSnapshotLocked(ctx.PlayerID)
	state.Ship.Disabled = false
	state.Ship.RepairState = "ready"
	state.Ship.Hull = state.Ship.MaxHull
	state.Ship.Shield = state.Ship.MaxShield
	state.Ship.Capacitor = state.Ship.MaxCapacitor
	runtime.players[ctx.PlayerID] = state
	respawn, err := runtime.repairRespawnPlayerLocked(ctx.PlayerID, authSessionID(ctx.SessionID))
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	state = runtime.players[ctx.PlayerID]

	record := repairAttemptRecord{
		ReferenceKey: referenceKey,
		Ship:         state.Ship,
		Wallet:       state.Wallet,
		PublicMapKey: respawn.PublicMapKey,
		Position:     respawn.Position,
		Protection:   respawn.Protection,
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
	runtime.queueEventLocked(sessionID, realtime.EventWalletSnapshot, runtime.walletSnapshotLocked(ctx.PlayerID))

	return marshalPayload(map[string]any{
		"accepted":       true,
		"repaired":       true,
		"repair_cost":    quote.Cost,
		"ship":           state.Ship,
		"wallet":         runtime.walletSnapshotLocked(ctx.PlayerID),
		"public_map_key": respawn.PublicMapKey,
		"position":       respawn.Position,
		"protection":     respawn.Protection,
	})
}
