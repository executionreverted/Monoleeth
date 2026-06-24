package server

import (
	"fmt"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/ships"
)

type runtimeModuleItemMover struct {
	inventory   *economy.InventoryService
	itemCatalog map[foundation.ItemID]economy.ItemDefinition
}

func (mover runtimeModuleItemMover) MoveModuleItemLocations(moves []modules.ModuleItemLocationMove) ([]modules.ModuleItemLocationMoveResult, error) {
	inputs := make([]economy.MoveItemInput, 0, len(moves))
	for _, move := range moves {
		definition, ok := mover.itemCatalog[move.ItemID]
		if !ok {
			return nil, fmt.Errorf("module item %q: %w", move.ItemID, modules.ErrUnknownModuleItem)
		}
		inputs = append(inputs, economy.MoveItemInput{
			PlayerID: move.PlayerID,
			ItemRef: economy.MoveItemRef{
				Definition:     definition,
				ItemInstanceID: move.ItemInstanceID,
			},
			FromLocation: move.FromLocation,
			ToLocation:   move.ToLocation,
			Quantity:     1,
			Reason:       move.LedgerReason,
			ReferenceKey: move.ReferenceKey,
		})
	}

	results, err := mover.inventory.SystemMoveItemsWithoutEvents(inputs)
	if err != nil {
		return nil, err
	}
	payloads := make([]modules.ModuleItemLocationMoveResult, 0, len(results))
	for _, result := range results {
		payloads = append(payloads, modules.ModuleItemLocationMoveResult{
			LedgerEntries: result.LedgerEntries,
			Duplicate:     result.Duplicate,
		})
	}
	return payloads, nil
}

type runtimeShipSlotLayoutProvider struct {
	shipCatalog ships.Catalog
}

func (provider runtimeShipSlotLayoutProvider) SlotLayoutForShip(shipID foundation.ShipID) (modules.ShipSlotLayout, error) {
	if err := shipID.Validate(); err != nil {
		return modules.ShipSlotLayout{}, err
	}
	definition, ok := provider.shipCatalog.Get(shipID)
	if !ok {
		return modules.ShipSlotLayout{}, fmt.Errorf("ship %q: %w", shipID, modules.ErrUnknownShipSlotLayout)
	}
	layout := modules.ShipSlotLayout{
		Offensive: definition.Slots.Offensive,
		Defensive: definition.Slots.Defensive,
		Utility:   definition.Slots.Utility,
	}
	if err := layout.Validate(); err != nil {
		return modules.ShipSlotLayout{}, err
	}
	return layout, nil
}

func (runtime *Runtime) shipSlotLayoutForLoadout(shipID foundation.ShipID) (modules.ShipSlotLayout, error) {
	return runtimeShipSlotLayoutProvider{shipCatalog: runtime.ShipCatalog}.SlotLayoutForShip(shipID)
}

type runtimeShipRankProvider struct {
	progression *progression.ProgressionService
}

func (provider runtimeShipRankProvider) RankForPlayer(playerID foundation.PlayerID) (int, error) {
	snapshot, err := provider.progression.GetProgressionSnapshot(playerID)
	if err != nil {
		return 0, err
	}
	return snapshot.Player.Rank, nil
}

type runtimeLoadoutProgressionProvider struct {
	progression *progression.ProgressionService
}

func (provider runtimeLoadoutProgressionProvider) ProgressionForPlayer(playerID foundation.PlayerID) (modules.PilotProgression, error) {
	snapshot, err := provider.progression.GetProgressionSnapshot(playerID)
	if err != nil {
		return modules.PilotProgression{}, err
	}
	roleLevels := make(map[modules.PilotRole]int)
	for _, roleLevel := range snapshot.RoleLevels() {
		roleLevels[modules.PilotRole(roleLevel.Role.String())] = roleLevel.Level
	}
	return modules.PilotProgression{
		Rank:       snapshot.Player.Rank,
		RoleLevels: roleLevels,
	}, nil
}
