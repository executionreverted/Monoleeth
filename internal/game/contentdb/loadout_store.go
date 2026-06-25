package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
)

var ErrLoadoutItemMoverRequired = errors.New("loadout module item mover required to persist item locations")

type LoadoutStore struct {
	store     *Store
	itemMover modules.ModuleItemLocationMover
}

var (
	_ modules.LoadoutStore         = (*LoadoutStore)(nil)
	_ modules.LoadoutRepository    = (*LoadoutStore)(nil)
	_ modules.ActiveShipReader     = (*LoadoutStore)(nil)
	_ modules.ModuleItemReader     = (*LoadoutStore)(nil)
	_ modules.EquippedModuleReader = (*LoadoutStore)(nil)
	_ modules.ModuleItemMutator    = (*LoadoutStore)(nil)
)

func NewLoadoutStore(store *Store) (*LoadoutStore, error) {
	return newLoadoutStore(store, nil)
}

func NewLoadoutStoreWithItemMover(store *Store, mover modules.ModuleItemLocationMover) (*LoadoutStore, error) {
	return newLoadoutStore(store, mover)
}

func newLoadoutStore(store *Store, mover modules.ModuleItemLocationMover) (*LoadoutStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &LoadoutStore{store: store, itemMover: mover}, nil
}

func (store *LoadoutStore) SaveLoadout(loadout modules.Loadout) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if err := loadout.Validate(); err != nil {
		return err
	}
	assignmentsJSON, err := marshalSlotAssignments(loadout.SlotAssignments)
	if err != nil {
		return err
	}
	_, err = store.store.db.ExecContext(context.Background(), `
		INSERT INTO player_loadouts(player_id, loadout_id, ship_id, name, slot_assignments_json, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
		ON CONFLICT (player_id, loadout_id) DO UPDATE
		SET ship_id = EXCLUDED.ship_id,
			name = EXCLUDED.name,
			slot_assignments_json = EXCLUDED.slot_assignments_json,
			updated_at = EXCLUDED.updated_at
	`, loadout.PlayerID.String(), loadout.LoadoutID.String(), loadout.ShipID.String(), loadout.Name, string(assignmentsJSON), loadout.CreatedAt.UTC(), loadout.UpdatedAt.UTC())
	return err
}

func (store *LoadoutStore) Loadout(playerID foundation.PlayerID, loadoutID modules.LoadoutID) (modules.Loadout, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return modules.Loadout{}, ErrNilDatabase
	}
	if err := playerID.Validate(); err != nil {
		return modules.Loadout{}, err
	}
	if err := loadoutID.Validate(); err != nil {
		return modules.Loadout{}, err
	}
	var loadout modules.Loadout
	var storedPlayerID string
	var storedLoadoutID string
	var shipID string
	var slotAssignmentsJSON []byte
	err := store.store.db.QueryRowContext(context.Background(), `
		SELECT player_id, loadout_id, ship_id, name, slot_assignments_json, created_at, updated_at
		FROM player_loadouts
		WHERE player_id = $1
			AND loadout_id = $2
	`, playerID.String(), loadoutID.String()).Scan(&storedPlayerID, &storedLoadoutID, &shipID, &loadout.Name, &slotAssignmentsJSON, &loadout.CreatedAt, &loadout.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return modules.Loadout{}, fmt.Errorf("loadout %q: %w", loadoutID, modules.ErrUnknownLoadout)
	}
	if err != nil {
		return modules.Loadout{}, err
	}
	loadout.PlayerID = foundation.PlayerID(storedPlayerID)
	loadout.LoadoutID = modules.LoadoutID(storedLoadoutID)
	loadout.ShipID = foundation.ShipID(shipID)
	loadout.SlotAssignments, err = parseSlotAssignments(slotAssignmentsJSON)
	if err != nil {
		return modules.Loadout{}, err
	}
	loadout.CreatedAt = loadout.CreatedAt.UTC()
	loadout.UpdatedAt = loadout.UpdatedAt.UTC()
	if loadout.PlayerID != playerID {
		return modules.Loadout{}, fmt.Errorf("loadout %q owner %q player %q: %w", loadoutID, loadout.PlayerID, playerID, modules.ErrLoadoutOwnerMismatch)
	}
	if err := loadout.Validate(); err != nil {
		return modules.Loadout{}, err
	}
	return cloneContentLoadout(loadout), nil
}

func (store *LoadoutStore) ActiveShipID(playerID foundation.PlayerID) (foundation.ShipID, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return "", ErrNilDatabase
	}
	if err := playerID.Validate(); err != nil {
		return "", err
	}
	record, err := loadPlayerHangarRecord(context.Background(), store.store.db, playerID)
	if err != nil {
		return "", err
	}
	active, ok := record.ActiveShip()
	if !ok {
		return "", fmt.Errorf("player %q: %w", playerID, modules.ErrActiveShipNotFound)
	}
	return active.ShipID, nil
}

func (store *LoadoutStore) ModuleItem(itemInstanceID foundation.ItemID) (economy.InstanceItem, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return economy.InstanceItem{}, ErrNilDatabase
	}
	item, ok, err := loadInventoryInstanceItem(context.Background(), store.store.db, itemInstanceID)
	if err != nil {
		return economy.InstanceItem{}, err
	}
	if !ok {
		return economy.InstanceItem{}, fmt.Errorf("item %q: %w", itemInstanceID, modules.ErrUnknownModuleItem)
	}
	return cloneContentInstanceItem(item), nil
}

func (store *LoadoutStore) EquippedModules(playerID foundation.PlayerID, shipID foundation.ShipID) ([]modules.EquippedModule, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	if err := playerID.Validate(); err != nil {
		return nil, err
	}
	if err := shipID.Validate(); err != nil {
		return nil, err
	}
	return loadEquippedModules(context.Background(), store.store.db, playerID, shipID)
}

func (store *LoadoutStore) EquippedModuleByItem(itemInstanceID foundation.ItemID) (modules.EquippedModule, bool, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return modules.EquippedModule{}, false, ErrNilDatabase
	}
	if err := itemInstanceID.Validate(); err != nil {
		return modules.EquippedModule{}, false, err
	}
	equipped, err := scanEquippedModule(store.store.db.QueryRowContext(context.Background(), `
		SELECT player_id, ship_id, slot_id, item_instance_id, equipped_at
		FROM player_equipped_modules
		WHERE item_instance_id = $1
	`, itemInstanceID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return modules.EquippedModule{}, false, nil
	}
	if err != nil {
		return modules.EquippedModule{}, false, err
	}
	return equipped, true, nil
}

func (store *LoadoutStore) ReplaceEquippedModules(input modules.ReplaceEquippedModulesInput) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if err := input.ShipID.Validate(); err != nil {
		return err
	}
	nextBySlot, err := validateContentNextEquipped(input)
	if err != nil {
		return err
	}
	current, err := store.EquippedModules(input.PlayerID, input.ShipID)
	if err != nil {
		return err
	}
	items, err := store.loadReplaceModuleItems(input.PlayerID, input.ShipID, current, nextBySlot)
	if err != nil {
		return err
	}
	moves, err := buildContentModuleItemLocationMoves(input, current, nextBySlot, items)
	if err != nil {
		return err
	}
	if len(moves) > 0 {
		if store.itemMover == nil {
			return fmt.Errorf("%w: player %q ship %q", ErrLoadoutItemMoverRequired, input.PlayerID, input.ShipID)
		}
		if _, err := store.itemMover.MoveModuleItemLocations(moves); err != nil {
			return err
		}
	}

	tx, err := store.store.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(context.Background(), `DELETE FROM player_equipped_modules WHERE player_id = $1 AND ship_id = $2`, input.PlayerID.String(), input.ShipID.String()); err != nil {
		return err
	}
	next := equippedMapValues(nextBySlot)
	sortContentEquippedModules(next)
	for _, equipped := range next {
		if _, err = tx.ExecContext(context.Background(), `
			INSERT INTO player_equipped_modules(player_id, ship_id, slot_id, item_instance_id, equipped_at)
			VALUES ($1, $2, $3, $4, $5)
		`, equipped.PlayerID.String(), equipped.ShipID.String(), equipped.SlotID.String(), equipped.ItemInstanceID.String(), equipped.EquippedAt.UTC()); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

func (store *LoadoutStore) MarkEquippedModuleBroken(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	itemInstanceID foundation.ItemID,
) (modules.EquippedModule, bool, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return modules.EquippedModule{}, false, ErrNilDatabase
	}
	if err := playerID.Validate(); err != nil {
		return modules.EquippedModule{}, false, err
	}
	if err := shipID.Validate(); err != nil {
		return modules.EquippedModule{}, false, err
	}
	if err := itemInstanceID.Validate(); err != nil {
		return modules.EquippedModule{}, false, err
	}

	tx, err := store.store.db.BeginTx(context.Background(), nil)
	if err != nil {
		return modules.EquippedModule{}, false, err
	}
	defer tx.Rollback()

	item, ok, err := loadInventoryInstanceItem(context.Background(), tx, itemInstanceID)
	if err != nil {
		return modules.EquippedModule{}, false, err
	}
	if !ok {
		return modules.EquippedModule{}, false, fmt.Errorf("item %q: %w", itemInstanceID, modules.ErrUnknownModuleItem)
	}
	if item.OwnerPlayerID != playerID {
		return modules.EquippedModule{}, false, fmt.Errorf("item %q owner %q player %q: %w", itemInstanceID, item.OwnerPlayerID, playerID, modules.ErrModuleItemNotOwned)
	}
	equipped, err := scanEquippedModule(tx.QueryRowContext(context.Background(), `
		SELECT player_id, ship_id, slot_id, item_instance_id, equipped_at
		FROM player_equipped_modules
		WHERE item_instance_id = $1
	`, itemInstanceID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return modules.EquippedModule{}, false, fmt.Errorf("item %q: %w", itemInstanceID, modules.ErrModuleItemNotEquipped)
	}
	if err != nil {
		return modules.EquippedModule{}, false, err
	}
	if equipped.PlayerID != playerID || equipped.ShipID != shipID {
		return modules.EquippedModule{}, false, fmt.Errorf("item %q equipped by player %q ship %q target player %q ship %q: %w", itemInstanceID, equipped.PlayerID, equipped.ShipID, playerID, shipID, modules.ErrModuleItemNotEquipped)
	}
	if item.DurabilityCurrent <= 0 {
		err = tx.Commit()
		return equipped, false, err
	}
	if _, err = tx.ExecContext(context.Background(), `
		UPDATE player_inventory_instance_items
		SET durability_current = 0,
			updated_at = now()
		WHERE item_instance_id = $1
	`, itemInstanceID.String()); err != nil {
		return modules.EquippedModule{}, false, err
	}
	err = tx.Commit()
	return equipped, true, err
}

func loadEquippedModules(ctx context.Context, reader hangarReader, playerID foundation.PlayerID, shipID foundation.ShipID) ([]modules.EquippedModule, error) {
	rows, err := reader.QueryContext(ctx, `
		SELECT player_id, ship_id, slot_id, item_instance_id, equipped_at
		FROM player_equipped_modules
		WHERE player_id = $1
			AND ship_id = $2
		ORDER BY slot_id
	`, playerID.String(), shipID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	equipped := make([]modules.EquippedModule, 0)
	for rows.Next() {
		module, err := scanEquippedModule(rows)
		if err != nil {
			return nil, err
		}
		equipped = append(equipped, module)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortContentEquippedModules(equipped)
	return equipped, nil
}

func (store *LoadoutStore) loadReplaceModuleItems(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	current []modules.EquippedModule,
	nextBySlot map[modules.ModuleSlotID]modules.EquippedModule,
) (map[foundation.ItemID]economy.InstanceItem, error) {
	items := make(map[foundation.ItemID]economy.InstanceItem)
	key := contentPlayerShipKey(playerID, shipID)
	for _, module := range equippedMapValues(nextBySlot) {
		item, err := store.ModuleItem(module.ItemInstanceID)
		if err != nil {
			return nil, err
		}
		if item.OwnerPlayerID != playerID {
			return nil, fmt.Errorf("item %q owner %q player %q: %w", module.ItemInstanceID, item.OwnerPlayerID, playerID, modules.ErrModuleItemNotOwned)
		}
		existing, ok, err := store.EquippedModuleByItem(module.ItemInstanceID)
		if err != nil {
			return nil, err
		}
		if ok && contentPlayerShipKey(existing.PlayerID, existing.ShipID) != key {
			return nil, fmt.Errorf("item %q equipped by player %q ship %q: %w", module.ItemInstanceID, existing.PlayerID, existing.ShipID, modules.ErrModuleItemAlreadyEquipped)
		}
		items[module.ItemInstanceID] = item
	}
	for _, module := range current {
		item, err := store.ModuleItem(module.ItemInstanceID)
		if err != nil {
			return nil, err
		}
		items[module.ItemInstanceID] = item
	}
	return items, nil
}

func validateContentNextEquipped(input modules.ReplaceEquippedModulesInput) (map[modules.ModuleSlotID]modules.EquippedModule, error) {
	nextBySlot := make(map[modules.ModuleSlotID]modules.EquippedModule, len(input.Equipped))
	nextItems := make(map[foundation.ItemID]modules.ModuleSlotID, len(input.Equipped))
	for _, module := range input.Equipped {
		if err := module.Validate(); err != nil {
			return nil, err
		}
		if module.PlayerID != input.PlayerID || module.ShipID != input.ShipID {
			return nil, fmt.Errorf("equipped player %q ship %q target player %q ship %q: %w", module.PlayerID, module.ShipID, input.PlayerID, input.ShipID, modules.ErrLoadoutShipMismatch)
		}
		if _, ok := nextBySlot[module.SlotID]; ok {
			return nil, fmt.Errorf("slot %q: %w", module.SlotID, modules.ErrDuplicateModuleAssignment)
		}
		if firstSlot, ok := nextItems[module.ItemInstanceID]; ok {
			return nil, fmt.Errorf("item %q in slots %q and %q: %w", module.ItemInstanceID, firstSlot, module.SlotID, modules.ErrDuplicateModuleAssignment)
		}
		nextBySlot[module.SlotID] = module
		nextItems[module.ItemInstanceID] = module.SlotID
	}
	return nextBySlot, nil
}

func buildContentModuleItemLocationMoves(
	input modules.ReplaceEquippedModulesInput,
	current []modules.EquippedModule,
	next map[modules.ModuleSlotID]modules.EquippedModule,
	items map[foundation.ItemID]economy.InstanceItem,
) ([]modules.ModuleItemLocationMove, error) {
	currentItems := make(map[foundation.ItemID]modules.EquippedModule, len(current))
	for _, module := range current {
		currentItems[module.ItemInstanceID] = module
	}
	nextItems := make(map[foundation.ItemID]modules.EquippedModule, len(next))
	nextEquipped := equippedMapValues(next)
	for _, module := range nextEquipped {
		nextItems[module.ItemInstanceID] = module
	}
	sortContentEquippedModules(current)
	sortContentEquippedModules(nextEquipped)

	moveCount := 0
	for itemInstanceID := range currentItems {
		if _, kept := nextItems[itemInstanceID]; !kept {
			moveCount++
		}
	}
	for itemInstanceID := range nextItems {
		if _, kept := currentItems[itemInstanceID]; !kept {
			moveCount++
		}
	}
	if moveCount == 0 {
		return nil, nil
	}
	if err := input.RequestID.Validate(); err != nil {
		return nil, err
	}

	accountLocation := economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID(input.PlayerID.String())}
	equippedLocation := economy.ItemLocation{Kind: economy.LocationKindShipEquipped, ID: economy.LocationID(input.ShipID.String())}
	moves := make([]modules.ModuleItemLocationMove, 0, moveCount)
	for _, old := range current {
		if _, kept := nextItems[old.ItemInstanceID]; kept {
			continue
		}
		item := items[old.ItemInstanceID]
		if item.Location != equippedLocation {
			return nil, fmt.Errorf("item %q location %s: %w", old.ItemInstanceID, item.Location.String(), modules.ErrInvalidModuleItemLocation)
		}
		reference, err := foundation.ModuleUnequipIdempotencyKey(input.PlayerID, input.ShipID, old.ItemInstanceID, input.RequestID)
		if err != nil {
			return nil, err
		}
		moves = append(moves, modules.ModuleItemLocationMove{
			PlayerID:       input.PlayerID,
			ShipID:         input.ShipID,
			SlotID:         old.SlotID,
			ItemID:         item.ItemID,
			ItemInstanceID: old.ItemInstanceID,
			FromLocation:   equippedLocation,
			ToLocation:     accountLocation,
			Direction:      modules.ModuleItemMoveUnequip,
			RequestID:      input.RequestID,
			LedgerReason:   modules.LedgerReasonModuleUnequip,
			ReferenceKey:   reference,
		})
	}
	for _, module := range nextEquipped {
		if _, kept := currentItems[module.ItemInstanceID]; kept {
			continue
		}
		item := items[module.ItemInstanceID]
		if item.Location != accountLocation {
			return nil, fmt.Errorf("item %q location %s: %w", module.ItemInstanceID, item.Location.String(), modules.ErrInvalidModuleItemLocation)
		}
		reference, err := foundation.ModuleEquipIdempotencyKey(input.PlayerID, input.ShipID, module.ItemInstanceID, input.RequestID)
		if err != nil {
			return nil, err
		}
		moves = append(moves, modules.ModuleItemLocationMove{
			PlayerID:       input.PlayerID,
			ShipID:         input.ShipID,
			SlotID:         module.SlotID,
			ItemID:         item.ItemID,
			ItemInstanceID: module.ItemInstanceID,
			FromLocation:   accountLocation,
			ToLocation:     equippedLocation,
			Direction:      modules.ModuleItemMoveEquip,
			RequestID:      input.RequestID,
			LedgerReason:   modules.LedgerReasonModuleEquip,
			ReferenceKey:   reference,
		})
	}
	return moves, nil
}

func marshalSlotAssignments(assignments modules.SlotAssignments) ([]byte, error) {
	out := make(map[string]string, len(assignments))
	for slotID, itemInstanceID := range assignments {
		out[slotID.String()] = itemInstanceID.String()
	}
	return json.Marshal(out)
}

func parseSlotAssignments(raw []byte) (modules.SlotAssignments, error) {
	values := make(map[string]string)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
	}
	if len(values) == 0 {
		return nil, nil
	}
	assignments := make(modules.SlotAssignments, len(values))
	for slotID, itemInstanceID := range values {
		assignments[modules.ModuleSlotID(slotID)] = foundation.ItemID(itemInstanceID)
	}
	if err := assignments.Validate(); err != nil {
		return nil, err
	}
	return assignments, nil
}

func scanEquippedModule(scanner interface{ Scan(dest ...any) error }) (modules.EquippedModule, error) {
	var playerID string
	var shipID string
	var slotID string
	var itemInstanceID string
	var equipped modules.EquippedModule
	if err := scanner.Scan(&playerID, &shipID, &slotID, &itemInstanceID, &equipped.EquippedAt); err != nil {
		return modules.EquippedModule{}, err
	}
	equipped.PlayerID = foundation.PlayerID(playerID)
	equipped.ShipID = foundation.ShipID(shipID)
	equipped.SlotID = modules.ModuleSlotID(slotID)
	equipped.ItemInstanceID = foundation.ItemID(itemInstanceID)
	equipped.EquippedAt = equipped.EquippedAt.UTC()
	if err := equipped.Validate(); err != nil {
		return modules.EquippedModule{}, err
	}
	return equipped, nil
}

func equippedMapValues(input map[modules.ModuleSlotID]modules.EquippedModule) []modules.EquippedModule {
	out := make([]modules.EquippedModule, 0, len(input))
	for _, equipped := range input {
		out = append(out, equipped)
	}
	return out
}

func sortContentEquippedModules(equipped []modules.EquippedModule) {
	sort.Slice(equipped, func(i, j int) bool {
		return equipped[i].SlotID.String() < equipped[j].SlotID.String()
	})
}

func cloneContentLoadout(loadout modules.Loadout) modules.Loadout {
	loadout.SlotAssignments = loadout.SlotAssignments.Clone()
	return loadout
}

func cloneContentInstanceItem(item economy.InstanceItem) economy.InstanceItem {
	item.MetadataJSON = append(item.MetadataJSON[:0:0], item.MetadataJSON...)
	return item
}

func contentPlayerShipKey(playerID foundation.PlayerID, shipID foundation.ShipID) string {
	return playerID.String() + "\x00" + shipID.String()
}
