package server

import (
	"fmt"
	"sort"
	"strings"

	"gameproject/internal/game/combat"
	deathdomain "gameproject/internal/game/death"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const (
	runtimeDefaultPVPDeathCargoDropPercent = 0.50
	runtimeSeededPVPDeathCargoDropPercent  = 1.00
)

func isLethalPlayerCombatResult(before combat.ActorState, result combat.BasicAttackResult) bool {
	return result.Target.Type == world.EntityTypePlayer &&
		!result.Target.PlayerID.IsZero() &&
		!before.Dead &&
		before.HP > 0 &&
		(result.Target.Dead || result.Target.HP <= 0)
}

func (runtime *Runtime) processLethalPVPDeathLocked(
	requestID foundation.RequestID,
	attacker combat.ActorState,
	target combat.ActorState,
) ([]loot.Drop, error) {
	if runtime.Death == nil {
		return nil, fmt.Errorf("runtime death service missing")
	}
	input, err := runtime.processDeathInputForLethalPVPLocked(requestID, attacker, target)
	if err != nil {
		return nil, err
	}
	result, err := runtime.Death.ProcessDeath(input)
	if err != nil {
		return nil, err
	}
	if result.Duplicate {
		return nil, nil
	}
	if err := runtime.applyProcessDeathResultLocked(result); err != nil {
		return nil, err
	}
	return append([]loot.Drop(nil), result.LootDrops...), nil
}

func (runtime *Runtime) processDeathInputForLethalPVPLocked(
	requestID foundation.RequestID,
	attacker combat.ActorState,
	target combat.ActorState,
) (deathdomain.ProcessDeathInput, error) {
	lethalEventID, err := lethalPVPDeathEventID(requestID, attacker, target)
	if err != nil {
		return deathdomain.ProcessDeathInput{}, err
	}
	policy, err := runtime.pvpDeathCargoDropPolicyLocked(target.ZoneID)
	if err != nil {
		return deathdomain.ProcessDeathInput{}, err
	}
	respawnLocationID, err := runtime.respawnLocationIDForDeathLocked(target.PlayerID, target.ZoneID)
	if err != nil {
		return deathdomain.ProcessDeathInput{}, err
	}
	return deathdomain.ProcessDeathInput{
		LethalEventID:   lethalEventID,
		PlayerID:        target.PlayerID,
		WorldID:         target.WorldID,
		ZoneID:          target.ZoneID,
		Position:        target.Position,
		KillerEntityID:  attacker.EntityID,
		Reason:          deathdomain.DeathReasonCombat,
		CargoDropPolicy: policy,
		CargoSnapshot: func() ([]deathdomain.CargoStack, error) {
			return runtime.deathCargoStacksForPlayerLocked(target.PlayerID)
		},
		DropOwnerPlayerID: attacker.PlayerID,
		RespawnLocationID: respawnLocationID,
	}, nil
}

func lethalPVPDeathEventID(requestID foundation.RequestID, attacker combat.ActorState, target combat.ActorState) (foundation.EventID, error) {
	if err := requestID.Validate(); err != nil {
		return "", err
	}
	if err := attacker.PlayerID.Validate(); err != nil {
		return "", fmt.Errorf("pvp death attacker player: %w", err)
	}
	if err := attacker.EntityID.Validate(); err != nil {
		return "", fmt.Errorf("pvp death attacker entity: %w", err)
	}
	if err := target.PlayerID.Validate(); err != nil {
		return "", fmt.Errorf("pvp death target player: %w", err)
	}
	if err := target.EntityID.Validate(); err != nil {
		return "", fmt.Errorf("pvp death target entity: %w", err)
	}
	if err := target.WorldID.Validate(); err != nil {
		return "", fmt.Errorf("pvp death target world: %w", err)
	}
	if err := target.ZoneID.Validate(); err != nil {
		return "", fmt.Errorf("pvp death target zone: %w", err)
	}
	components := []string{
		"lethal",
		"pvp",
		eventIDComponent("world", target.WorldID.String()),
		eventIDComponent("zone", target.ZoneID.String()),
		eventIDComponent("attacker-player", attacker.PlayerID.String()),
		eventIDComponent("target-player", target.PlayerID.String()),
		eventIDComponent("target-entity", target.EntityID.String()),
		eventIDComponent("attacker-entity", attacker.EntityID.String()),
		eventIDComponent("request", requestID.String()),
	}
	eventID := foundation.EventID(strings.Join(components, "-"))
	if err := eventID.Validate(); err != nil {
		return "", err
	}
	return eventID, nil
}

func eventIDComponent(label, value string) string {
	return fmt.Sprintf("%s%d-%s", label, len(value), value)
}

func (runtime *Runtime) pvpDeathCargoDropPolicyLocked(zoneID foundation.ZoneID) (deathdomain.ZoneCargoDropPolicy, error) {
	percent := runtimeDefaultPVPDeathCargoDropPercent
	if zoneID == worldmaps.MapID("map_1_3").ZoneID() {
		percent = runtimeSeededPVPDeathCargoDropPercent
	}
	return deathdomain.NewZoneCargoDropPolicy(zoneID, percent, percent)
}

func (runtime *Runtime) respawnLocationIDForDeathLocked(playerID foundation.PlayerID, zoneID foundation.ZoneID) (deathdomain.RespawnLocationID, error) {
	if location, err := runtime.mapRouter.ActiveLocation(playerID); err == nil &&
		location.ZoneID == zoneID &&
		location.SpawnID != "" {
		return deathdomain.RespawnLocationID(location.SpawnID.String()), nil
	}
	if definition, ok := runtime.mapCatalog.Get(worldmaps.MapID(zoneID.String())); ok && len(definition.SpawnPoints) > 0 {
		return deathdomain.RespawnLocationID(definition.SpawnPoints[0].SpawnID.String()), nil
	}
	return deathdomain.RespawnLocationID(worldmaps.StarterSpawnID.String()), nil
}

func (runtime *Runtime) deathCargoStacksForPlayerLocked(playerID foundation.PlayerID) ([]deathdomain.CargoStack, error) {
	location := runtime.activeCargoLocationLocked(playerID)
	stacks := make([]deathdomain.CargoStack, 0)
	for _, item := range runtime.Inventory.StackableItems() {
		if item.OwnerPlayerID != playerID || item.Location != location {
			continue
		}
		definition, ok := runtime.itemCatalog[item.ItemID]
		if !ok {
			return nil, fmt.Errorf("death cargo item %q: %w", item.ItemID, economy.ErrUnknownCargoItemDefinition)
		}
		stacks = append(stacks, deathdomain.CargoStack{
			StackID:           item.ItemInstanceID,
			SourceStackID:     item.ItemInstanceID,
			Definition:        deathCargoItemDefinition(definition),
			EconomyDefinition: definition,
			OwnerPlayerID:     item.OwnerPlayerID,
			Location:          item.Location,
			Quantity:          item.Quantity.Int64(),
			BoundState:        economy.BoundStateUnbound,
		})
	}
	for _, item := range runtime.Inventory.InstanceItems() {
		if item.OwnerPlayerID != playerID || item.Location != location {
			continue
		}
		definition, ok := runtime.itemCatalog[item.ItemID]
		if !ok {
			return nil, fmt.Errorf("death cargo item %q: %w", item.ItemID, economy.ErrUnknownCargoItemDefinition)
		}
		stacks = append(stacks, deathdomain.CargoStack{
			StackID:           item.ItemInstanceID,
			ItemInstanceID:    item.ItemInstanceID,
			SourceStackID:     item.ItemInstanceID,
			Definition:        deathCargoItemDefinition(definition),
			EconomyDefinition: definition,
			OwnerPlayerID:     item.OwnerPlayerID,
			Location:          item.Location,
			Quantity:          item.Quantity.Int64(),
			BoundState:        item.BoundState,
		})
	}
	sort.Slice(stacks, func(i, j int) bool {
		return stacks[i].SourceStackID.String() < stacks[j].SourceStackID.String()
	})
	return stacks, nil
}

func deathCargoItemDefinition(definition economy.ItemDefinition) deathdomain.CargoItemDefinition {
	return deathdomain.CargoItemDefinition{
		Source:            definition.Source,
		ItemID:            definition.ItemID,
		Type:              definition.Type,
		TradeFlags:        append([]economy.TradeFlag(nil), definition.TradeFlags...),
		BindRules:         append([]economy.BindRule(nil), definition.BindRules...),
		CargoUnitsPerItem: definition.WeightUnits.Int64(),
	}
}

func (runtime *Runtime) applyProcessDeathResultLocked(result deathdomain.ProcessDeathResult) error {
	if result.Duplicate {
		return nil
	}
	record := result.Record
	if record.PlayerID.IsZero() {
		return nil
	}
	playerShip := result.ShipDisableResult.PlayerShip
	disabledAt := record.CreatedAt
	if playerShip.DisabledAt != nil {
		disabledAt = *playerShip.DisabledAt
	}
	disabledReason := playerShip.DisabledReason
	if disabledReason == "" {
		disabledReason = ships.DisabledReasonDeath
	}
	runtime.applyShipDisabledDomainEventLocked(deathdomain.ShipDisabledEvent{
		DeathID:           record.DeathID,
		LethalEventKey:    record.LethalEventKey,
		PlayerID:          record.PlayerID,
		ShipID:            playerShip.ShipID,
		DisabledReason:    disabledReason,
		DisabledAt:        disabledAt,
		StatInvalidation:  result.ShipDisableResult.StatInvalidation,
		RespawnLocationID: record.RespawnLocationID,
	})
	if state, ok := runtime.players[record.PlayerID]; ok {
		state.Cargo = runtime.cargoSnapshotFromInventoryLocked(record.PlayerID)
		runtime.players[record.PlayerID] = state
		runtime.queueEventToPlayerSessionsLocked(record.PlayerID, realtime.EventCargoSnapshot, state.Cargo)
		runtime.queueEventToPlayerSessionsLocked(record.PlayerID, realtime.EventInventorySnapshot, runtime.inventorySnapshotLocked(record.PlayerID))
	}
	for _, drop := range result.LootDrops {
		if err := runtime.insertLootDropEntityLocked(drop); err != nil {
			return err
		}
	}
	return nil
}
