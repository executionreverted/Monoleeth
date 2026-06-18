package runtime

import (
	"errors"
	"fmt"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
)

var (
	ErrNilInventoryService        = errors.New("nil inventory service")
	ErrInvalidModuleMoveDirection = errors.New("invalid module move direction")
)

// ModuleInventoryLedgerAdapter translates loadout equip/unequip transitions
// into authoritative inventory moves and item ledger rows.
type ModuleInventoryLedgerAdapter struct {
	inventory *economy.InventoryService
	modules   modules.Catalog
}

// NewModuleInventoryLedgerAdapter returns the runtime inventory bridge used by
// module loadout stores.
func NewModuleInventoryLedgerAdapter(
	inventory *economy.InventoryService,
	moduleCatalog modules.Catalog,
) (*ModuleInventoryLedgerAdapter, error) {
	if inventory == nil {
		return nil, ErrNilInventoryService
	}
	return &ModuleInventoryLedgerAdapter{
		inventory: inventory,
		modules:   moduleCatalog,
	}, nil
}

// MoveModuleItemLocations moves module items through InventoryService so the
// equip/unequip transitions have ledger evidence and idempotent retry behavior.
func (adapter *ModuleInventoryLedgerAdapter) MoveModuleItemLocations(moves []modules.ModuleItemLocationMove) ([]modules.ModuleItemLocationMoveResult, error) {
	if len(moves) == 0 {
		return nil, nil
	}

	inputs := make([]economy.MoveItemInput, 0, len(moves))
	for _, move := range moves {
		if err := validateModuleItemLocationMove(move); err != nil {
			return nil, err
		}
		definition, ok := adapter.modules.Lookup(move.ItemID)
		if !ok {
			return nil, fmt.Errorf("module item %q: %w", move.ItemID, modules.ErrUnknownModuleDefinition)
		}
		itemDefinition, err := moduleItemDefinition(definition)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, economy.MoveItemInput{
			PlayerID: move.PlayerID,
			ItemRef: economy.MoveItemRef{
				Definition:     itemDefinition,
				ItemInstanceID: move.ItemInstanceID,
			},
			FromLocation: move.FromLocation,
			ToLocation:   move.ToLocation,
			Quantity:     1,
			Reason:       move.LedgerReason,
			ReferenceKey: move.ReferenceKey,
		})
	}

	results, err := adapter.inventory.SystemMoveItemsWithoutEvents(inputs)
	if err != nil {
		return nil, err
	}
	moveResults := make([]modules.ModuleItemLocationMoveResult, 0, len(results))
	for _, result := range results {
		moveResults = append(moveResults, modules.ModuleItemLocationMoveResult{
			LedgerEntries: result.LedgerEntries,
			Duplicate:     result.Duplicate,
		})
	}
	return moveResults, nil
}

func validateModuleItemLocationMove(move modules.ModuleItemLocationMove) error {
	if err := move.PlayerID.Validate(); err != nil {
		return err
	}
	if err := move.ShipID.Validate(); err != nil {
		return err
	}
	if err := move.SlotID.Validate(); err != nil {
		return err
	}
	if err := move.ItemID.Validate(); err != nil {
		return err
	}
	if err := move.ItemInstanceID.Validate(); err != nil {
		return err
	}
	if err := move.FromLocation.Validate(); err != nil {
		return err
	}
	if err := move.ToLocation.Validate(); err != nil {
		return err
	}
	if err := move.RequestID.Validate(); err != nil {
		return err
	}
	if err := move.LedgerReason.Validate(); err != nil {
		return err
	}
	if err := move.ReferenceKey.Validate(); err != nil {
		return err
	}
	switch move.Direction {
	case modules.ModuleItemMoveEquip:
		if move.LedgerReason != modules.LedgerReasonModuleEquip {
			return fmt.Errorf("ledger reason %q: %w", move.LedgerReason, modules.ErrInvalidModuleItemMove)
		}
		if err := validateModuleItemLocations(
			move,
			economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID(move.PlayerID.String())},
			economy.ItemLocation{Kind: economy.LocationKindShipEquipped, ID: economy.LocationID(move.ShipID.String())},
		); err != nil {
			return err
		}
		expectedReference, err := foundation.ModuleEquipIdempotencyKey(move.PlayerID, move.ShipID, move.ItemInstanceID, move.RequestID)
		if err != nil {
			return err
		}
		if move.ReferenceKey != expectedReference {
			return fmt.Errorf("reference %q expected %q: %w", move.ReferenceKey, expectedReference, modules.ErrInvalidModuleItemMove)
		}
	case modules.ModuleItemMoveUnequip:
		if move.LedgerReason != modules.LedgerReasonModuleUnequip {
			return fmt.Errorf("ledger reason %q: %w", move.LedgerReason, modules.ErrInvalidModuleItemMove)
		}
		if err := validateModuleItemLocations(
			move,
			economy.ItemLocation{Kind: economy.LocationKindShipEquipped, ID: economy.LocationID(move.ShipID.String())},
			economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID(move.PlayerID.String())},
		); err != nil {
			return err
		}
		expectedReference, err := foundation.ModuleUnequipIdempotencyKey(move.PlayerID, move.ShipID, move.ItemInstanceID, move.RequestID)
		if err != nil {
			return err
		}
		if move.ReferenceKey != expectedReference {
			return fmt.Errorf("reference %q expected %q: %w", move.ReferenceKey, expectedReference, modules.ErrInvalidModuleItemMove)
		}
	default:
		return fmt.Errorf("direction %q: %w", move.Direction, ErrInvalidModuleMoveDirection)
	}
	if move.FromLocation == move.ToLocation {
		return economy.ErrMoveItemSameSourceAndTarget
	}
	return nil
}

func validateModuleItemLocations(
	move modules.ModuleItemLocationMove,
	wantFrom economy.ItemLocation,
	wantTo economy.ItemLocation,
) error {
	if move.FromLocation != wantFrom {
		return fmt.Errorf("from location %s expected %s: %w", move.FromLocation.String(), wantFrom.String(), modules.ErrInvalidModuleItemMove)
	}
	if move.ToLocation != wantTo {
		return fmt.Errorf("to location %s expected %s: %w", move.ToLocation.String(), wantTo.String(), modules.ErrInvalidModuleItemMove)
	}
	return nil
}

func moduleItemDefinition(definition modules.ModuleDefinition) (economy.ItemDefinition, error) {
	one, err := foundation.NewQuantity(1)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return economy.NewItemDefinition(
		definition.Source,
		definition.ItemID,
		definition.Name,
		economy.ItemTypeInstance,
		definition.Rarity,
		one,
		one,
		definition.TradeFlags,
		definition.BindRules,
		nil,
	)
}
