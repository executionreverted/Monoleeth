package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/combat"
	deathdomain "gameproject/internal/game/death"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/social"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

const repairQuoteTTL = 2 * time.Minute

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

type repairQuoteRecord struct {
	PlayerID  foundation.PlayerID
	Quote     repairQuotePayload
	ExpiresAt time.Time
}

type repairQuotePayload struct {
	ShipID      string `json:"ship_id"`
	Currency    string `json:"currency"`
	Cost        int64  `json:"cost"`
	Disabled    bool   `json:"disabled"`
	QuoteID     string `json:"quote_id"`
	IssuedAtMS  int64  `json:"issued_at_ms"`
	ExpiresAtMS int64  `json:"expires_at_ms"`
}

type deathRepairShipIntent = repairQuotePayload

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

type runtimeBasicLaserInput struct {
	RequestID          foundation.RequestID
	PlayerID           foundation.PlayerID
	TargetID           world.EntityID
	SkillID            string
	SessionIDs         []auth.SessionID
	QueueShotLifecycle bool
}

type runtimeBasicLaserResult struct {
	Combat              combat.BasicAttackResult
	PlayerState         playerRuntimeState
	Drops               []loot.Drop
	ProgressionSnapshot *progressionSnapshotPayload
	TargetKilled        bool
}

func (runtime *Runtime) executeBasicLaserLocked(input runtimeBasicLaserInput) (runtimeBasicLaserResult, error) {
	if input.SkillID == "" {
		input.SkillID = runtime.combatRules.BasicLaserSkillID
	}
	if input.SkillID != runtime.combatRules.BasicLaserSkillID {
		return runtimeBasicLaserResult{}, foundation.NewDomainError(foundation.CodeInvalidPayload, "Unsupported combat skill.")
	}
	if len(input.SessionIDs) == 0 {
		input.SessionIDs = runtime.sessionIDsForPlayerLocked(input.PlayerID, "")
	}
	if err := runtime.validateShipCanActLocked(input.PlayerID); err != nil {
		return runtimeBasicLaserResult{}, err
	}
	attacker, err := runtime.syncPlayerCombatActorLocked(input.PlayerID)
	if err != nil {
		return runtimeBasicLaserResult{}, domainErrorForRuntime(err)
	}
	if err := runtime.syncWorldCombatActorLocked(input.PlayerID, input.TargetID); err != nil {
		return runtimeBasicLaserResult{}, domainErrorForRuntime(err)
	}
	viewer, err := runtime.viewerForPlayerLocked(input.PlayerID)
	if err != nil {
		return runtimeBasicLaserResult{}, domainErrorForRuntime(err)
	}
	if !runtime.entityVisibleToPlayerLocked(input.PlayerID, input.TargetID) {
		return runtimeBasicLaserResult{}, foundation.NewDomainError(foundation.CodeNotVisible, "Target is not visible.")
	}
	attackerBefore := attacker
	targetBefore, ok := runtime.Combat.Actor(input.TargetID)
	if !ok {
		return runtimeBasicLaserResult{}, domainErrorForRuntime(worker.ErrUnknownEntity)
	}
	restoreAttackActors := func() {
		_ = runtime.Combat.UpsertActor(attackerBefore)
		_ = runtime.Combat.UpsertActor(targetBefore)
	}

	result, err := runtime.Combat.ExecuteBasicAttack(combat.BasicAttackInput{
		AttackerID: attacker.EntityID,
		TargetID:   input.TargetID,
		Viewer:     &viewer,
		Policy:     runtime.basicAttackPolicyLocked(),
	})
	if err != nil {
		return runtimeBasicLaserResult{}, domainErrorForCombat(err)
	}
	if input.QueueShotLifecycle {
		for _, sessionID := range input.SessionIDs {
			runtime.queueEventLocked(sessionID, realtime.EventCombatShotStarted, map[string]any{
				"skill_id":  input.SkillID,
				"target_id": input.TargetID.String(),
			})
		}
	}
	combatLockedAt := runtime.clock.Now()
	runtime.refreshCombatLockForActorLocked(input.PlayerID, combatLockedAt)
	runtime.refreshCombatLockForActorLocked(result.Target.PlayerID, combatLockedAt)

	var drops []loot.Drop
	lethalPlayerDeath := isLethalPlayerCombatResult(targetBefore, result)
	if lethalPlayerDeath {
		playerDeathDrops, err := runtime.processLethalPVPDeathLocked(input.RequestID, result.Attacker, result.Target)
		if err != nil {
			return runtimeBasicLaserResult{}, domainErrorForRuntime(err)
		}
		drops = append(drops, playerDeathDrops...)
	}
	var progressionSnapshot *progressionSnapshotPayload
	var questUpdates []quests.PlayerQuest
	var socialContributionSnapshots []social.ContributionSnapshot
	if result.KillEvent != nil {
		instance, _, err := runtime.activeMapInstanceLocked(input.PlayerID)
		if err != nil {
			restoreAttackActors()
			return runtimeBasicLaserResult{}, domainErrorForRuntime(err)
		}
		lootTable, err := runtime.selectNPCKillLootTableForInstanceLocked(instance, *result.KillEvent)
		if err != nil {
			restoreAttackActors()
			return runtimeBasicLaserResult{}, domainErrorForRuntime(err)
		}
		if err := runtime.submitWorkerCommandAndRecordMetricsLocked(instance, worker.MarkEnemyKilledCommand{
			Definition:  instance.Definition,
			NPCEntityID: result.KillEvent.NPCEntityID,
			KilledAt:    result.KillEvent.KilledAt,
		}); err != nil {
			restoreAttackActors()
			return runtimeBasicLaserResult{}, domainErrorForRuntime(err)
		}
		instance.HiddenEntities[result.KillEvent.NPCEntityID] = true

		if updated, err := runtime.Quest.ConsumeCombatNPCKilled(quests.CombatNPCKilledInput{
			EventID:          foundation.EventID("quest-combat-" + input.RequestID.String()),
			ProgressEventKey: quests.QuestProgressEventKey("combat.npc_killed:" + result.KillEvent.NPCEntityID.String()),
			PlayerID:         input.PlayerID,
			NPCType:          result.KillEvent.NPCType,
		}); err != nil {
			return runtimeBasicLaserResult{}, domainErrorForQuest(err)
		} else {
			questUpdates = updated
		}
		if xpResult, err := runtime.combatXP.GrantNPCKillXP(*result.KillEvent); err == nil {
			payload := progressionPayload(xpResult.Snapshot)
			progressionSnapshot = &payload
		}
		created, err := runtime.Loot.CreateDropsForNPCKill(*result.KillEvent, lootTable)
		if err != nil {
			return runtimeBasicLaserResult{}, domainErrorForRuntime(err)
		}
		drops = created.Drops
		for _, drop := range drops {
			if err := runtime.insertLootDropEntityLocked(drop); err != nil {
				return runtimeBasicLaserResult{}, domainErrorForRuntime(err)
			}
		}
		snapshots, err := runtime.recordSocialNPCKillContributionsLocked(*result.KillEvent, result.Target.Contributions)
		if err != nil {
			return runtimeBasicLaserResult{}, domainErrorForRuntime(err)
		}
		socialContributionSnapshots = snapshots
	}

	state, ok := runtime.applyCombatActorToPlayerShipLocked(input.PlayerID, result.Attacker)
	if !ok {
		return runtimeBasicLaserResult{}, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}

	for _, sessionID := range input.SessionIDs {
		runtime.queueEventLocked(sessionID, realtime.EventPlayerSnapshot, state.playerSnapshot())
		runtime.queueEventLocked(sessionID, realtime.EventShipSnapshot, state.Ship)
		runtime.queueEventLocked(sessionID, realtime.EventCombatCooldownStarted, map[string]any{
			"skill_id":             input.SkillID,
			"target_id":            input.TargetID.String(),
			"cooldown_ready_at_ms": result.CooldownReadyAt.UTC().UnixMilli(),
		})
		if result.Hit {
			runtime.queueEventLocked(sessionID, realtime.EventCombatDamage, map[string]any{
				"target_id":     input.TargetID.String(),
				"amount":        roundCombatValue(result.Damage),
				"shield_amount": roundCombatValue(result.ShieldDamage),
				"hull_amount":   roundCombatValue(result.HPDamage),
			})
		} else {
			runtime.queueEventLocked(sessionID, realtime.EventCombatMiss, map[string]any{
				"target_id": input.TargetID.String(),
			})
		}
		runtime.queueTargetUpdatedLocked(sessionID, result.Target)
	}
	if result.Target.Type == world.EntityTypePlayer && !result.Target.PlayerID.IsZero() && result.Target.PlayerID != input.PlayerID {
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
				return runtimeBasicLaserResult{}, domainErrorForRuntime(worker.ErrUnknownPlayer)
			}
			runtime.queueEventToPlayerSessionsLocked(result.Target.PlayerID, realtime.EventPlayerSnapshot, targetState.playerSnapshot())
			runtime.queueEventToPlayerSessionsLocked(result.Target.PlayerID, realtime.EventShipSnapshot, targetState.Ship)
		}
		runtime.queueTargetUpdatedToPlayerSessionsLocked(result.Target.PlayerID, result.Target)
	}
	if result.KillEvent != nil {
		for _, sessionID := range input.SessionIDs {
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
		runtime.queueSocialContributionSnapshotsLocked(socialContributionSnapshots)
	}
	if lethalPlayerDeath {
		for _, drop := range drops {
			runtime.queueEventToPlayerSessionsLocked(input.PlayerID, realtime.EventLootCreated, lootDropPayload(drop, runtime.clock.Now()))
		}
	}
	targetKilled := result.Killed || lethalPlayerDeath
	if input.QueueShotLifecycle {
		for _, sessionID := range input.SessionIDs {
			runtime.queueEventLocked(sessionID, realtime.EventCombatShotResolved, map[string]any{
				"skill_id":             input.SkillID,
				"target_id":            input.TargetID.String(),
				"hit":                  result.Hit,
				"amount":               roundCombatValue(result.Damage),
				"killed":               targetKilled,
				"cooldown_ready_at_ms": result.CooldownReadyAt.UTC().UnixMilli(),
			})
		}
	}
	return runtimeBasicLaserResult{
		Combat:              result,
		PlayerState:         state,
		Drops:               drops,
		ProgressionSnapshot: progressionSnapshot,
		TargetKilled:        targetKilled,
	}, nil
}

func (runtime *Runtime) handleCombatUseSkill(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	intent, err := decodeCombatUseSkillIntent(request.Payload)
	if err != nil {
		return nil, err
	}
	if intent.SkillID != runtime.combatRules.BasicLaserSkillID {
		return nil, foundation.NewDomainError(foundation.CodeInvalidPayload, "Unsupported combat skill.")
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	execution, err := runtime.executeBasicLaserLocked(runtimeBasicLaserInput{
		RequestID:  request.RequestID,
		PlayerID:   ctx.PlayerID,
		TargetID:   intent.TargetID,
		SkillID:    intent.SkillID,
		SessionIDs: []auth.SessionID{authSessionID(ctx.SessionID)},
	})
	if err != nil {
		return nil, err
	}

	response := map[string]any{
		"accepted":             true,
		"skill_id":             intent.SkillID,
		"target_id":            intent.TargetID.String(),
		"hit":                  execution.Combat.Hit,
		"amount":               roundCombatValue(execution.Combat.Damage),
		"killed":               execution.TargetKilled,
		"cooldown_ready_at_ms": execution.Combat.CooldownReadyAt.UTC().UnixMilli(),
		"ship":                 execution.PlayerState.Ship,
		"player":               execution.PlayerState.playerSnapshot(),
	}
	if targetStatus := combatStatusFromActor(execution.Combat.Target); targetStatus != nil {
		response["target"] = targetStatus
	}
	if len(execution.Drops) > 0 {
		response["drops"] = lootDropPayloads(execution.Drops, runtime.clock.Now())
	}
	if execution.ProgressionSnapshot != nil {
		response["progression"] = *execution.ProgressionSnapshot
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
	if err := runtime.submitWorkerCommandAndRecordMetricsLocked(instance, worker.RemoveEntityCommand{EntityID: intent.DropID}); err != nil && !errors.Is(err, worker.ErrUnknownEntity) {
		return nil, domainErrorForRuntime(err)
	}
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
	return marshalPayload(runtime.issueRepairQuoteLocked(ctx.PlayerID, state))
}

func (runtime *Runtime) handleDeathRepairShip(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	intent, err := decodeDeathRepairShipIntent(request.Payload)
	if err != nil {
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

	quote, err := runtime.validateRepairQuoteLocked(ctx.PlayerID, state, intent)
	if err != nil {
		return nil, err
	}
	shipID := foundation.ShipID(state.Ship.ActiveShipID)
	if err := runtime.ensureHangarShipDisabledForRepairLocked(ctx.PlayerID, shipID); err != nil {
		return nil, err
	}
	if quote.Cost > 0 {
		if _, err := runtime.Wallet.DebitWallet(economy.DebitWalletInput{
			PlayerID:     ctx.PlayerID,
			Currency:     economy.CurrencyBucketCredits,
			Amount:       quote.Cost,
			Reason:       deathdomain.LedgerReasonShipRepair,
			ReferenceKey: referenceKey,
		}); err != nil {
			state.Wallet = runtime.walletSnapshotLocked(ctx.PlayerID)
			runtime.players[ctx.PlayerID] = state
			return nil, domainErrorForEconomy(err)
		}
		state.Wallet = runtime.walletSnapshotLocked(ctx.PlayerID)
		runtime.players[ctx.PlayerID] = state
	}
	if err := runtime.repairHangarShipAfterDebitLocked(ctx.PlayerID, shipID, quote.Cost, referenceKey); err != nil {
		state.Wallet = runtime.walletSnapshotLocked(ctx.PlayerID)
		runtime.players[ctx.PlayerID] = state
		return nil, err
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
	delete(runtime.repairQuotes, ctx.PlayerID)

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
		"currency":    runtime.combatRules.RepairCurrency.String(),
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

func (runtime *Runtime) ensureHangarShipDisabledForRepairLocked(playerID foundation.PlayerID, shipID foundation.ShipID) error {
	hangar, err := runtime.Hangar.GetHangar(playerID)
	if err != nil {
		return domainErrorForHangar(err)
	}
	for _, playerShip := range hangar.Ships {
		if playerShip.ShipID != shipID {
			continue
		}
		if playerShip.State != ships.ShipStateDisabled {
			disabled, err := runtime.Hangar.DisableActiveShipForDeath(ships.DisableActiveShipForDeathInput{PlayerID: playerID})
			if err != nil {
				return domainErrorForHangar(err)
			}
			if disabled.PlayerShip.ShipID != shipID || disabled.PlayerShip.State != ships.ShipStateDisabled {
				return foundation.NewDomainError(foundation.CodeShipDisabled, "Ship is not disabled.", foundation.WithCause(ships.ErrShipNotDisabled))
			}
		}
		return nil
	}
	return domainErrorForHangar(fmt.Errorf("ship %q: %w", shipID, ships.ErrShipNotUnlocked))
}

func (runtime *Runtime) repairHangarShipAfterDebitLocked(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	repairCost int64,
	referenceKey foundation.IdempotencyKey,
) error {
	if _, err := runtime.Hangar.RepairShip(ships.RepairShipInput{
		PlayerID: playerID,
		ShipID:   shipID,
	}); err != nil {
		if repairCost > 0 {
			if _, refundErr := runtime.Wallet.CreditWallet(economy.CreditWalletInput{
				PlayerID:     playerID,
				Currency:     economy.CurrencyBucketCredits,
				Amount:       repairCost,
				Reason:       deathdomain.LedgerReasonShipRepairRefund,
				ReferenceKey: referenceKey,
			}); refundErr != nil {
				return foundation.NewDomainError(
					foundation.CodeInternal,
					"Ship repair failed after wallet debit.",
					foundation.WithCause(fmt.Errorf("%w; repair refund failed: %v", err, refundErr)),
				)
			}
		}
		return domainErrorForHangar(err)
	}
	return nil
}

func decodeDeathRepairShipIntent(raw json.RawMessage) (deathRepairShipIntent, error) {
	var intent deathRepairShipIntent
	if err := decodeStrict(raw, &intent); err != nil {
		return deathRepairShipIntent{}, invalidPayload("Repair quote is invalid.", err)
	}
	if intent.QuoteID == "" {
		return deathRepairShipIntent{}, invalidPayload("Repair quote is required.", nil)
	}
	return intent, nil
}

func (runtime *Runtime) validateRepairQuoteLocked(
	playerID foundation.PlayerID,
	state playerRuntimeState,
	intent deathRepairShipIntent,
) (repairQuotePayload, error) {
	record, ok := runtime.repairQuotes[playerID]
	if !ok || record.PlayerID != playerID {
		return repairQuotePayload{}, invalidPayload("Repair quote is required.", nil)
	}
	now := runtime.clock.Now().UTC()
	if !record.ExpiresAt.After(now) {
		delete(runtime.repairQuotes, playerID)
		return repairQuotePayload{}, invalidPayload("Repair quote is stale.", nil)
	}
	if !sameRepairQuotePayload(record.Quote, intent) {
		return repairQuotePayload{}, invalidPayload("Repair quote was tampered.", nil)
	}
	current := runtime.repairQuotePayloadLocked(
		state,
		record.Quote.QuoteID,
		time.UnixMilli(record.Quote.IssuedAtMS).UTC(),
		record.ExpiresAt,
	)
	if current.ShipID != record.Quote.ShipID ||
		current.Currency != record.Quote.Currency ||
		current.Cost != record.Quote.Cost ||
		current.Disabled != record.Quote.Disabled {
		return repairQuotePayload{}, invalidPayload("Repair quote is no longer valid.", nil)
	}
	return record.Quote, nil
}

func sameRepairQuotePayload(a repairQuotePayload, b repairQuotePayload) bool {
	return a.ShipID == b.ShipID &&
		a.Currency == b.Currency &&
		a.Cost == b.Cost &&
		a.Disabled == b.Disabled &&
		a.QuoteID == b.QuoteID &&
		a.IssuedAtMS == b.IssuedAtMS &&
		a.ExpiresAtMS == b.ExpiresAtMS
}
