package server

import (
	"context"
	"fmt"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/ships"
)

type runtimeLoadoutStore interface {
	modules.LoadoutStore
	SetActiveShip(foundation.PlayerID, foundation.ShipID) error
	PutModuleItem(economy.InstanceItem) error
}

type runtimeDurableLoadoutStore struct {
	modules.LoadoutStore
}

func (store runtimeDurableLoadoutStore) SetActiveShip(playerID foundation.PlayerID, shipID foundation.ShipID) error {
	if err := playerID.Validate(); err != nil {
		return err
	}
	if err := shipID.Validate(); err != nil {
		return err
	}
	activeShipID, err := store.ActiveShipID(playerID)
	if err != nil {
		return err
	}
	if activeShipID != shipID {
		return fmt.Errorf("active ship %q target %q: %w", activeShipID, shipID, modules.ErrLoadoutShipMismatch)
	}
	return nil
}

func (store runtimeDurableLoadoutStore) PutModuleItem(item economy.InstanceItem) error {
	if err := item.Validate(); err != nil {
		return err
	}
	stored, err := store.ModuleItem(item.ItemInstanceID)
	if err != nil {
		return err
	}
	if stored.OwnerPlayerID != item.OwnerPlayerID || stored.ItemID != item.ItemID {
		return fmt.Errorf("module item %q durable row mismatch: %w", item.ItemInstanceID, modules.ErrModuleItemInstanceMismatch)
	}
	return nil
}

type runtimeModuleItemMover struct {
	inventory   *economy.InventoryService
	itemCatalog map[foundation.ItemID]economy.ItemDefinition
}

func (mover runtimeModuleItemMover) MoveModuleItemLocations(moves []modules.ModuleItemLocationMove) ([]modules.ModuleItemLocationMoveResult, error) {
	inputs, err := runtimeModuleMoveInputs(moves, mover.itemCatalog)
	if err != nil {
		return nil, err
	}
	results, err := mover.inventory.SystemMoveItemsWithoutEvents(inputs)
	if err != nil {
		return nil, err
	}
	return runtimeModuleMoveResults(results), nil
}

type runtimeDurableModuleItemMover struct {
	inventory   *economy.InventoryService
	itemCatalog map[foundation.ItemID]economy.ItemDefinition
	repository  *contentdb.InventoryStore
}

func (mover runtimeDurableModuleItemMover) MoveModuleItemLocations(moves []modules.ModuleItemLocationMove) ([]modules.ModuleItemLocationMoveResult, error) {
	if mover.inventory == nil || mover.repository == nil {
		return nil, contentdb.ErrNilDatabase
	}
	inputs, err := runtimeModuleMoveInputs(moves, mover.itemCatalog)
	if err != nil {
		return nil, err
	}
	results, err := mover.inventory.SystemMoveItemsWithoutEvents(inputs)
	if err != nil {
		return nil, err
	}
	for _, result := range results {
		for _, item := range result.InstanceItems {
			if err := mover.repository.UpsertInstanceItem(context.Background(), item); err != nil {
				return nil, err
			}
		}
	}
	return runtimeModuleMoveResults(results), nil
}

func runtimeModuleMoveInputs(
	moves []modules.ModuleItemLocationMove,
	itemCatalog map[foundation.ItemID]economy.ItemDefinition,
) ([]economy.MoveItemInput, error) {
	inputs := make([]economy.MoveItemInput, 0, len(moves))
	for _, move := range moves {
		definition, ok := itemCatalog[move.ItemID]
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
	return inputs, nil
}

func runtimeModuleMoveResults(results []economy.MoveItemResult) []modules.ModuleItemLocationMoveResult {
	payloads := make([]modules.ModuleItemLocationMoveResult, 0, len(results))
	for _, result := range results {
		payloads = append(payloads, modules.ModuleItemLocationMoveResult{
			LedgerEntries: result.LedgerEntries,
			Duplicate:     result.Duplicate,
		})
	}
	return payloads
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
