package server

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/visibility"
	"gameproject/internal/game/world/worker"
)

func decodeCombatUseSkillIntent(raw json.RawMessage) (combatUseSkillIntent, error) {
	var intent combatUseSkillIntent
	if err := decodeStrict(raw, &intent); err != nil {
		return combatUseSkillIntent{}, err
	}
	if intent.SkillID == "" {
		return combatUseSkillIntent{}, invalidPayload("skill_id is required.", nil)
	}
	if err := intent.TargetID.Validate(); err != nil {
		return combatUseSkillIntent{}, invalidPayload("target_id is required.", err)
	}
	return intent, nil
}

func decodeLootPickupIntent(raw json.RawMessage) (lootPickupIntent, error) {
	var intent lootPickupIntent
	if err := decodeStrict(raw, &intent); err != nil {
		return lootPickupIntent{}, err
	}
	if err := intent.DropID.Validate(); err != nil {
		return lootPickupIntent{}, invalidPayload("drop_id is required.", err)
	}
	return intent, nil
}

func (runtime *Runtime) validateShipCanActLocked(playerID foundation.PlayerID) error {
	return runtime.validateShipCanMoveLocked(playerID)
}

func (runtime *Runtime) syncPlayerCombatActorLocked(playerID foundation.PlayerID) (combat.ActorState, error) {
	state, ok := runtime.players[playerID]
	if !ok {
		return combat.ActorState{}, worker.ErrUnknownPlayer
	}
	instance, _, err := runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return combat.ActorState{}, err
	}
	entity, ok := instance.Worker.PlayerEntity(playerID)
	if !ok {
		return combat.ActorState{}, worker.ErrUnknownPlayer
	}
	hidden := instance.HiddenPlayers[playerID] || instance.HiddenEntities[entity.ID]
	signature, stealthScore, jammerStrength := runtime.visibilityInputsForEntityLocked(entity, playerID, hidden)
	actor := combat.ActorState{
		EntityID:       entity.ID,
		Type:           world.EntityTypePlayer,
		PlayerID:       playerID,
		WorldID:        entity.WorldID,
		ZoneID:         entity.ZoneID,
		Position:       entity.Position,
		Signature:      signature,
		StealthScore:   stealthScore,
		JammerStrength: jammerStrength,
		Hidden:         hidden,
		Stats:          runtime.playerCombatStatsLocked(playerID, state),
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
	if err := runtime.Combat.UpsertActor(actor); err != nil {
		return combat.ActorState{}, err
	}
	return actor, nil
}

func (runtime *Runtime) syncWorldCombatActorLocked(playerID foundation.PlayerID, entityID world.EntityID) error {
	instance, _, err := runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return err
	}
	entity, ok := instance.Worker.Entity(entityID)
	if !ok {
		return worker.ErrUnknownEntity
	}
	if entity.Type == world.EntityTypePlayer {
		return runtime.syncPlayerTargetCombatActorLocked(entity)
	}
	if entity.Type != world.EntityTypeNPC {
		return foundation.NewDomainError(foundation.CodeInvalidPayload, "Target is not a hostile entity.")
	}
	_, err = runtime.upsertNPCCombatActorProjectionLocked(instance, entity)
	return err
}

func (runtime *Runtime) playerCombatStatsLocked(playerID foundation.PlayerID, state playerRuntimeState) stats.StatSnapshot {
	exploration := runtime.explorationStatsForPlayerStateLocked(state)
	return stats.NewStatSnapshot(playerID, starterShipID, 1, stats.EffectiveStats{
		Core: stats.CoreStats{
			HPMax:         float64(state.Ship.MaxHull),
			ShieldMax:     float64(state.Ship.MaxShield),
			EnergyMax:     float64(state.Ship.MaxCapacitor),
			EnergyRegen:   4,
			Speed:         state.Stats.Speed,
			CargoCapacity: float64(state.Stats.CargoCapacity),
		},
		Combat: stats.CombatStats{
			WeaponDamage:     35,
			WeaponRange:      state.Stats.WeaponRange,
			WeaponCooldown:   float64(runtime.combatRules.BasicLaserCooldownMS) / 1000,
			WeaponEnergyCost: float64(runtime.combatRules.BasicLaserEnergyCost),
			Accuracy:         1,
		},
		Exploration: exploration,
	}, runtime.clock.Now())
}

func (runtime *Runtime) viewerForPlayerLocked(playerID foundation.PlayerID) (visibility.Viewer, error) {
	instance, location, err := runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return visibility.Viewer{}, err
	}
	entity, ok := instance.Worker.PlayerEntity(playerID)
	if !ok {
		return visibility.Viewer{}, worker.ErrUnknownPlayer
	}
	now := runtime.clock.Now()
	statSnapshot := runtime.visibilityStatSnapshotLocked(playerID, now)
	return visibility.Viewer{
		PlayerID:       playerID,
		WorldID:        location.WorldID,
		ZoneID:         location.ZoneID,
		Position:       entity.Position,
		RadarRange:     visibility.RadarRangeFromStatSnapshot(statSnapshot),
		DetectionStats: visibility.DetectionStatsFromStatSnapshot(statSnapshot),
		Witnesses:      runtime.hiddenPlayerWitnessesForViewerLocked(instance, playerID, now),
		ObservedAt:     now,
	}, nil
}

func (runtime *Runtime) entityVisibleToPlayerLocked(playerID foundation.PlayerID, entityID world.EntityID) bool {
	snapshot, _, _, err := runtime.aoiSnapshotForPlayerLocked(playerID)
	if err != nil {
		return false
	}
	for _, entity := range snapshot.Entities {
		if entity.ID == entityID {
			return true
		}
	}
	return false
}

func (runtime *Runtime) queueTargetUpdatedLocked(sessionID auth.SessionID, actor combat.ActorState) {
	status := combatStatusFromActor(actor)
	if status == nil {
		return
	}
	runtime.queueEventLocked(sessionID, realtime.EventTargetUpdated, map[string]any{
		"entity_id": actor.EntityID.String(),
		"combat":    status,
	})
}

func (runtime *Runtime) queueTargetUpdatedToPlayerSessionsLocked(playerID foundation.PlayerID, actor combat.ActorState) {
	for _, sessionID := range runtime.sessionIDsForPlayerLocked(playerID, "") {
		runtime.queueTargetUpdatedLocked(sessionID, actor)
	}
}

func (runtime *Runtime) applyCombatActorToPlayerShipLocked(playerID foundation.PlayerID, actor combat.ActorState) (playerRuntimeState, bool) {
	state, ok := runtime.players[playerID]
	if !ok {
		return playerRuntimeState{}, false
	}
	state.Ship.Capacitor = roundCombatValue(actor.Energy)
	state.Ship.Hull = roundCombatValue(actor.HP)
	state.Ship.Shield = roundCombatValue(actor.Shield)
	runtime.players[playerID] = state
	return state, true
}

func (runtime *Runtime) entityCombatStatusLocked(entityID world.EntityID) *aoi.EntityCombatStatus {
	actor, ok := runtime.Combat.Actor(entityID)
	if !ok {
		return nil
	}
	return combatStatusFromActor(actor)
}

func combatStatusFromActor(actor combat.ActorState) *aoi.EntityCombatStatus {
	if actor.EntityID.IsZero() {
		return nil
	}
	status := "active"
	if actor.Dead || actor.HP <= 0 {
		status = "destroyed"
	}
	return &aoi.EntityCombatStatus{
		HP:        roundCombatValue(actor.HP),
		MaxHP:     roundCombatValue(actor.Stats.Stats.Core.HPMax),
		Shield:    roundCombatValue(actor.Shield),
		MaxShield: roundCombatValue(actor.Stats.Stats.Core.ShieldMax),
		Status:    status,
	}
}

func (runtime *Runtime) insertLootDropEntityLocked(drop loot.Drop) error {
	entity, err := world.NewEntity(drop.WorldID, drop.ZoneID, drop.ID, world.EntityTypeLoot, drop.Position)
	if err != nil {
		return err
	}
	instance, err := runtime.mapInstanceLocked(worldmaps.MapID(drop.ZoneID.String()))
	if err != nil {
		return err
	}
	return instance.Worker.InsertEntity(entity, 0)
}

func (runtime *Runtime) activeCargoLocationLocked(playerID foundation.PlayerID) economy.ItemLocation {
	state := runtime.players[playerID]
	locationID := state.Ship.ActiveShipID
	if locationID == "" {
		locationID = starterShipID.String()
	}
	return economy.ItemLocation{
		Kind: economy.LocationKindShipCargo,
		ID:   economy.LocationID(locationID),
	}
}

func (runtime *Runtime) cargoSnapshotFromInventoryLocked(playerID foundation.PlayerID) cargoSnapshotPayload {
	state := runtime.players[playerID]
	location := runtime.activeCargoLocationLocked(playerID)
	itemsByID := make(map[string]int64)
	var used int64
	for _, item := range runtime.Inventory.StackableItems() {
		if item.OwnerPlayerID != playerID || item.Location != location {
			continue
		}
		quantity := item.Quantity.Int64()
		itemsByID[item.ItemID.String()] += quantity
		if definition, ok := runtime.itemCatalog[item.ItemID]; ok {
			used += definition.WeightUnits.Int64() * quantity
		}
	}
	items := make([]cargoItemStack, 0, len(itemsByID))
	for itemID, quantity := range itemsByID {
		items = append(items, runtime.cargoItemStackPayloadLocked(foundation.ItemID(itemID), quantity, location))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ItemID < items[j].ItemID
	})
	return cargoSnapshotPayload{
		Used:     used,
		Capacity: state.Cargo.Capacity,
		Items:    items,
	}
}

func (runtime *Runtime) cargoItemStackPayloadLocked(itemID foundation.ItemID, quantity int64, location economy.ItemLocation) cargoItemStack {
	payload := cargoItemStack{
		ItemID:       itemID.String(),
		DisplayName:  itemID.String(),
		Category:     "unknown",
		ArtKey:       "item." + itemID.String(),
		Quantity:     quantity,
		Location:     location.Kind.String(),
		MoveEligible: false,
		LockedReason: "cargo_transfer_unavailable",
	}
	if definition, ok := runtime.itemCatalog[itemID]; ok {
		unitWeight := definition.WeightUnits.Int64()
		payload.DisplayName = definition.Name
		payload.Category = cargoItemCategory(definition)
		payload.ArtKey = "item." + definition.ItemID.String()
		payload.Rarity = definition.Rarity.String()
		payload.UnitWeight = unitWeight
		payload.UsedUnits = unitWeight * quantity
	}
	return payload
}

func cargoItemCategory(definition economy.ItemDefinition) string {
	switch definition.Type {
	case economy.ItemTypeStackable:
		return "resource"
	case economy.ItemTypeInstance:
		return "module"
	default:
		return definition.Type.String()
	}
}

func (runtime *Runtime) repairQuoteLocked(state playerRuntimeState) repairQuotePayload {
	cost := int64(0)
	if state.Ship.ActiveShipID != starterShipID.String() {
		cost = runtime.combatRules.NonStarterShipRepairFee
	}
	return repairQuotePayload{
		ShipID:   state.Ship.ActiveShipID,
		Currency: runtime.combatRules.RepairCurrency.String(),
		Cost:     cost,
		Disabled: state.Ship.Disabled,
	}
}

func (runtime *Runtime) shipDisabledRefreshEvents(sessionID auth.SessionID, playerID foundation.PlayerID) ([]realtime.EventEnvelope, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	state, ok := runtime.players[playerID]
	if !ok {
		return nil, worker.ErrUnknownPlayer
	}
	if !state.Ship.Disabled && state.Ship.RepairState != "disabled" {
		return nil, nil
	}
	if state.Ship.RepairState == "" {
		state.Ship.RepairState = "disabled"
		runtime.players[playerID] = state
	}
	payload := map[string]any{
		"ship_id":         state.Ship.ActiveShipID,
		"disabled_reason": shipDisabledReason(state.Ship),
		"ship":            state.Ship,
		"repair_quote":    runtime.repairQuoteLocked(state),
	}
	return []realtime.EventEnvelope{
		runtime.eventLocked(sessionID, realtime.EventDeathShipDisabled, payload),
		runtime.eventLocked(sessionID, realtime.EventShipSnapshot, state.Ship),
		runtime.eventLocked(sessionID, realtime.EventPlayerSnapshot, state.playerSnapshot()),
	}, nil
}

func shipDisabledReason(ship shipSnapshotPayload) string {
	if ship.RepairState != "" && ship.RepairState != "ready" && ship.RepairState != "disabled" {
		return ship.RepairState
	}
	return "death"
}

func (runtime *Runtime) queueEventLocked(sessionID auth.SessionID, eventType realtime.ClientEventType, payload any) {
	runtime.queuedEvents[sessionID] = append(runtime.queuedEvents[sessionID], runtime.eventLocked(sessionID, eventType, payload))
}

func (runtime *Runtime) drainQueuedEventsLocked(sessionID auth.SessionID) []realtime.EventEnvelope {
	events := runtime.queuedEvents[sessionID]
	delete(runtime.queuedEvents, sessionID)
	return runtime.filterEventsForActiveEpochLocked(sessionID, append([]realtime.EventEnvelope(nil), events...))
}

func (runtime *Runtime) drainQueuedEventsBySessionLocked() map[auth.SessionID][]realtime.EventEnvelope {
	if len(runtime.queuedEvents) == 0 {
		return nil
	}
	eventsBySession := make(map[auth.SessionID][]realtime.EventEnvelope, len(runtime.queuedEvents))
	for sessionID, events := range runtime.queuedEvents {
		delete(runtime.queuedEvents, sessionID)
		if len(events) == 0 {
			continue
		}
		filtered := runtime.filterEventsForActiveEpochLocked(sessionID, append([]realtime.EventEnvelope(nil), events...))
		if len(filtered) == 0 {
			continue
		}
		eventsBySession[sessionID] = filtered
	}
	return eventsBySession
}

func lootDropPayload(drop loot.Drop, now time.Time) map[string]any {
	return map[string]any{
		"drop_id":    drop.ID.String(),
		"entity_id":  drop.ID.String(),
		"position":   drop.Position,
		"item_id":    drop.ItemDefinition.ItemID.String(),
		"quantity":   drop.Quantity,
		"state":      string(drop.State(now)),
		"expires_at": drop.ExpiresAt.UTC().UnixMilli(),
	}
}

func lootDropPayloads(drops []loot.Drop, now time.Time) []map[string]any {
	payloads := make([]map[string]any, 0, len(drops))
	for _, drop := range drops {
		payloads = append(payloads, lootDropPayload(drop, now))
	}
	return payloads
}

func progressionPayload(snapshot progression.ProgressionSnapshot) progressionSnapshotPayload {
	payload := progressionSnapshotPayload{
		MainLevel: snapshot.Player.MainLevel,
		MainXP:    snapshot.Player.MainXP,
		Rank:      snapshot.Player.Rank,
	}
	if combatRole, ok := snapshot.RoleLevel(progression.RoleTypeCombat); ok {
		payload.CombatLevel = combatRole.Level
		payload.CombatXP = combatRole.XP
	}
	return payload
}

func roundCombatValue(value float64) int {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return int(math.Round(value))
}

func domainErrorForCombat(err error) error {
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, combat.ErrCooldownNotReady):
		return foundation.NewDomainError(foundation.CodeCooldown, "Skill is on cooldown.", foundation.WithCause(err))
	case errors.Is(err, combat.ErrNotEnoughEnergy):
		return foundation.NewDomainError(foundation.CodeNotEnoughEnergy, "Not enough energy.", foundation.WithCause(err))
	case errors.Is(err, combat.ErrOutOfRange):
		return foundation.NewDomainError(foundation.CodeOutOfRange, "Target is out of range.", foundation.WithCause(err))
	case errors.Is(err, combat.ErrPVPBlocked):
		return foundation.NewDomainError(foundation.CodePVPBlocked, "PvP is blocked here.", foundation.WithCause(err))
	case errors.Is(err, combat.ErrTargetNotVisible), errors.Is(err, combat.ErrDifferentWorldZone):
		return foundation.NewDomainError(foundation.CodeNotVisible, "Target is not visible.", foundation.WithCause(err))
	case errors.Is(err, combat.ErrUnknownActor), errors.Is(err, combat.ErrTargetDead), errors.Is(err, combat.ErrAttackerDead):
		return foundation.NewDomainError(foundation.CodeNotFound, "Target is not available.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Combat command failed.", foundation.WithCause(err))
	}
}

func domainErrorForLoot(err error) error {
	switch {
	case errors.Is(err, loot.ErrUnknownDrop), errors.Is(err, loot.ErrDropClaimed), errors.Is(err, loot.ErrDropExpired):
		return foundation.NewDomainError(foundation.CodeNotFound, "Drop is not available.", foundation.WithCause(err))
	case errors.Is(err, loot.ErrDropOwnerLocked), errors.Is(err, loot.ErrPickupNotVisible):
		return foundation.NewDomainError(foundation.CodeNotVisible, "Drop is not visible.", foundation.WithCause(err))
	case errors.Is(err, loot.ErrPickupOutOfRange):
		return foundation.NewDomainError(foundation.CodeOutOfRange, "Drop is out of range.", foundation.WithCause(err))
	case errors.Is(err, economy.ErrCargoCapacityExceeded):
		return foundation.NewDomainError(foundation.CodeNotEnoughCargo, "Cargo capacity is full.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Loot command failed.", foundation.WithCause(err))
	}
}
