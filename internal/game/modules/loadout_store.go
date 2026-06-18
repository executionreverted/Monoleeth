package modules

import (
	"fmt"
	"sync"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

// InMemoryLoadoutStore is a deterministic LoadoutStore implementation for
// tests and early vertical slices.
type InMemoryLoadoutStore struct {
	mu sync.RWMutex

	loadouts       map[string]Loadout
	activeShips    map[foundation.PlayerID]foundation.ShipID
	items          map[foundation.ItemID]economy.InstanceItem
	equippedByKey  map[string]map[ModuleSlotID]EquippedModule
	equippedByItem map[foundation.ItemID]EquippedModule
	itemMover      ModuleItemLocationMover
}

// NewInMemoryLoadoutStore returns an empty in-memory loadout store.
func NewInMemoryLoadoutStore() *InMemoryLoadoutStore {
	return &InMemoryLoadoutStore{
		loadouts:       make(map[string]Loadout),
		activeShips:    make(map[foundation.PlayerID]foundation.ShipID),
		items:          make(map[foundation.ItemID]economy.InstanceItem),
		equippedByKey:  make(map[string]map[ModuleSlotID]EquippedModule),
		equippedByItem: make(map[foundation.ItemID]EquippedModule),
	}
}

// NewInMemoryLoadoutStoreWithItemMover returns an in-memory loadout store that
// ledgers module item location changes through mover before committing them.
func NewInMemoryLoadoutStoreWithItemMover(mover ModuleItemLocationMover) *InMemoryLoadoutStore {
	store := NewInMemoryLoadoutStore()
	store.itemMover = mover
	return store
}

// SetActiveShip sets the explicit active ship pointer used by ApplyLoadout.
func (store *InMemoryLoadoutStore) SetActiveShip(playerID foundation.PlayerID, shipID foundation.ShipID) error {
	if err := playerID.Validate(); err != nil {
		return err
	}
	if err := shipID.Validate(); err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.activeShips[playerID] = shipID
	return nil
}

// PutModuleItem records an item snapshot for validation.
func (store *InMemoryLoadoutStore) PutModuleItem(item economy.InstanceItem) error {
	if err := item.Validate(); err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.items[item.ItemInstanceID] = cloneInstanceItem(item)
	return nil
}

// SaveLoadout stores a validated loadout.
func (store *InMemoryLoadoutStore) SaveLoadout(loadout Loadout) error {
	if err := loadout.Validate(); err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	key := playerLoadoutStoreKey(loadout.PlayerID, loadout.LoadoutID)
	if existing, ok := store.loadouts[key]; ok && existing.PlayerID != loadout.PlayerID {
		return fmt.Errorf("loadout %q owner %q player %q: %w", loadout.LoadoutID, existing.PlayerID, loadout.PlayerID, ErrLoadoutOwnerMismatch)
	}
	store.loadouts[key] = cloneLoadout(loadout)
	return nil
}

// Loadout returns a saved loadout owned by playerID.
func (store *InMemoryLoadoutStore) Loadout(playerID foundation.PlayerID, loadoutID LoadoutID) (Loadout, error) {
	if err := playerID.Validate(); err != nil {
		return Loadout{}, err
	}
	if err := loadoutID.Validate(); err != nil {
		return Loadout{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	loadout, ok := store.loadouts[playerLoadoutStoreKey(playerID, loadoutID)]
	if !ok {
		return Loadout{}, fmt.Errorf("loadout %q: %w", loadoutID, ErrUnknownLoadout)
	}
	if loadout.PlayerID != playerID {
		return Loadout{}, fmt.Errorf("loadout %q owner %q player %q: %w", loadoutID, loadout.PlayerID, playerID, ErrLoadoutOwnerMismatch)
	}
	return cloneLoadout(loadout), nil
}

// ActiveShipID returns the explicit active ship pointer for playerID.
func (store *InMemoryLoadoutStore) ActiveShipID(playerID foundation.PlayerID) (foundation.ShipID, error) {
	if err := playerID.Validate(); err != nil {
		return "", err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	shipID, ok := store.activeShips[playerID]
	if !ok {
		return "", fmt.Errorf("player %q: %w", playerID, ErrActiveShipNotFound)
	}
	return shipID, nil
}

// ModuleItem returns one item instance snapshot.
func (store *InMemoryLoadoutStore) ModuleItem(itemInstanceID foundation.ItemID) (economy.InstanceItem, error) {
	if err := itemInstanceID.Validate(); err != nil {
		return economy.InstanceItem{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	item, ok := store.items[itemInstanceID]
	if !ok {
		return economy.InstanceItem{}, fmt.Errorf("item %q: %w", itemInstanceID, ErrUnknownModuleItem)
	}
	return cloneInstanceItem(item), nil
}

// EquippedModules returns modules equipped on a player ship.
func (store *InMemoryLoadoutStore) EquippedModules(playerID foundation.PlayerID, shipID foundation.ShipID) ([]EquippedModule, error) {
	if err := playerID.Validate(); err != nil {
		return nil, err
	}
	if err := shipID.Validate(); err != nil {
		return nil, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	bySlot := store.equippedByKey[playerShipStoreKey(playerID, shipID)]
	equipped := make([]EquippedModule, 0, len(bySlot))
	for _, module := range bySlot {
		equipped = append(equipped, module)
	}
	sortEquippedModules(equipped)
	return equipped, nil
}

// EquippedModuleByItem returns the current equipped row for an item, if any.
func (store *InMemoryLoadoutStore) EquippedModuleByItem(itemInstanceID foundation.ItemID) (EquippedModule, bool, error) {
	if err := itemInstanceID.Validate(); err != nil {
		return EquippedModule{}, false, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	equipped, ok := store.equippedByItem[itemInstanceID]
	return equipped, ok, nil
}

// ReplaceEquippedModules replaces all equipped modules for one player ship.
func (store *InMemoryLoadoutStore) ReplaceEquippedModules(input ReplaceEquippedModulesInput) error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if err := input.ShipID.Validate(); err != nil {
		return err
	}
	nextBySlot := make(map[ModuleSlotID]EquippedModule, len(input.Equipped))
	nextItems := make(map[foundation.ItemID]ModuleSlotID, len(input.Equipped))
	for _, module := range input.Equipped {
		if err := module.Validate(); err != nil {
			return err
		}
		if module.PlayerID != input.PlayerID || module.ShipID != input.ShipID {
			return fmt.Errorf("equipped player %q ship %q target player %q ship %q: %w", module.PlayerID, module.ShipID, input.PlayerID, input.ShipID, ErrLoadoutShipMismatch)
		}
		if _, ok := nextBySlot[module.SlotID]; ok {
			return fmt.Errorf("slot %q: %w", module.SlotID, ErrDuplicateModuleAssignment)
		}
		if firstSlot, ok := nextItems[module.ItemInstanceID]; ok {
			return fmt.Errorf("item %q in slots %q and %q: %w", module.ItemInstanceID, firstSlot, module.SlotID, ErrDuplicateModuleAssignment)
		}
		nextBySlot[module.SlotID] = module
		nextItems[module.ItemInstanceID] = module.SlotID
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	key := playerShipStoreKey(input.PlayerID, input.ShipID)
	for _, module := range nextBySlot {
		item, ok := store.items[module.ItemInstanceID]
		if !ok {
			return fmt.Errorf("item %q: %w", module.ItemInstanceID, ErrUnknownModuleItem)
		}
		if item.OwnerPlayerID != input.PlayerID {
			return fmt.Errorf("item %q owner %q player %q: %w", module.ItemInstanceID, item.OwnerPlayerID, input.PlayerID, ErrModuleItemNotOwned)
		}
		if existing, ok := store.equippedByItem[module.ItemInstanceID]; ok {
			existingKey := playerShipStoreKey(existing.PlayerID, existing.ShipID)
			if existingKey != key {
				return fmt.Errorf("item %q equipped by player %q ship %q: %w", module.ItemInstanceID, existing.PlayerID, existing.ShipID, ErrModuleItemAlreadyEquipped)
			}
		}
	}
	for _, old := range store.equippedByKey[key] {
		if _, ok := store.items[old.ItemInstanceID]; !ok {
			return fmt.Errorf("item %q: %w", old.ItemInstanceID, ErrUnknownModuleItem)
		}
	}
	if store.itemMover != nil {
		moves, err := store.moduleItemLocationMovesLocked(input, store.equippedByKey[key], nextBySlot)
		if err != nil {
			return err
		}
		if _, err := store.itemMover.MoveModuleItemLocations(moves); err != nil {
			return err
		}
	}
	for _, old := range store.equippedByKey[key] {
		delete(store.equippedByItem, old.ItemInstanceID)
	}
	store.moveReplacedModuleItemsLocked(input.PlayerID, input.ShipID, store.equippedByKey[key], nextBySlot)
	if len(nextBySlot) == 0 {
		delete(store.equippedByKey, key)
		return nil
	}
	store.equippedByKey[key] = nextBySlot
	for _, module := range nextBySlot {
		store.equippedByItem[module.ItemInstanceID] = module
	}
	return nil
}

func (store *InMemoryLoadoutStore) moduleItemLocationMovesLocked(
	input ReplaceEquippedModulesInput,
	current map[ModuleSlotID]EquippedModule,
	next map[ModuleSlotID]EquippedModule,
) ([]ModuleItemLocationMove, error) {
	currentItems := make(map[foundation.ItemID]EquippedModule, len(current))
	currentEquipped := make([]EquippedModule, 0, len(current))
	for _, module := range current {
		currentItems[module.ItemInstanceID] = module
		currentEquipped = append(currentEquipped, module)
	}
	nextItems := make(map[foundation.ItemID]EquippedModule, len(next))
	nextEquipped := make([]EquippedModule, 0, len(next))
	for _, module := range next {
		nextItems[module.ItemInstanceID] = module
		nextEquipped = append(nextEquipped, module)
	}
	sortEquippedModules(currentEquipped)
	sortEquippedModules(nextEquipped)

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

	moves := make([]ModuleItemLocationMove, 0, moveCount)
	accountLocation := economy.ItemLocation{
		Kind: economy.LocationKindAccountInventory,
		ID:   economy.LocationID(input.PlayerID.String()),
	}
	equippedLocation := economy.ItemLocation{
		Kind: economy.LocationKindShipEquipped,
		ID:   economy.LocationID(input.ShipID.String()),
	}

	for _, old := range currentEquipped {
		if _, kept := nextItems[old.ItemInstanceID]; kept {
			continue
		}
		item := store.items[old.ItemInstanceID]
		if item.Location != equippedLocation {
			return nil, fmt.Errorf("item %q location %s: %w", item.ItemInstanceID, item.Location.String(), ErrInvalidModuleItemLocation)
		}
		reference, err := foundation.ModuleUnequipIdempotencyKey(input.PlayerID, input.ShipID, old.ItemInstanceID, input.RequestID)
		if err != nil {
			return nil, err
		}
		moves = append(moves, ModuleItemLocationMove{
			PlayerID:       input.PlayerID,
			ShipID:         input.ShipID,
			SlotID:         old.SlotID,
			ItemID:         item.ItemID,
			ItemInstanceID: old.ItemInstanceID,
			FromLocation:   equippedLocation,
			ToLocation:     accountLocation,
			Direction:      ModuleItemMoveUnequip,
			RequestID:      input.RequestID,
			LedgerReason:   LedgerReasonModuleUnequip,
			ReferenceKey:   reference,
		})
	}
	for _, module := range nextEquipped {
		if _, kept := currentItems[module.ItemInstanceID]; kept {
			continue
		}
		item := store.items[module.ItemInstanceID]
		if item.Location.Kind != economy.LocationKindAccountInventory {
			return nil, fmt.Errorf("item %q location %s: %w", item.ItemInstanceID, item.Location.String(), ErrInvalidModuleItemLocation)
		}
		reference, err := foundation.ModuleEquipIdempotencyKey(input.PlayerID, input.ShipID, module.ItemInstanceID, input.RequestID)
		if err != nil {
			return nil, err
		}
		moves = append(moves, ModuleItemLocationMove{
			PlayerID:       input.PlayerID,
			ShipID:         input.ShipID,
			SlotID:         module.SlotID,
			ItemID:         item.ItemID,
			ItemInstanceID: module.ItemInstanceID,
			FromLocation:   item.Location,
			ToLocation:     equippedLocation,
			Direction:      ModuleItemMoveEquip,
			RequestID:      input.RequestID,
			LedgerReason:   LedgerReasonModuleEquip,
			ReferenceKey:   reference,
		})
	}
	return moves, nil
}

func (store *InMemoryLoadoutStore) moveReplacedModuleItemsLocked(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	current map[ModuleSlotID]EquippedModule,
	next map[ModuleSlotID]EquippedModule,
) {
	nextItems := make(map[foundation.ItemID]struct{}, len(next))
	for _, module := range next {
		nextItems[module.ItemInstanceID] = struct{}{}
	}

	accountLocation := economy.ItemLocation{
		Kind: economy.LocationKindAccountInventory,
		ID:   economy.LocationID(playerID.String()),
	}
	for _, old := range current {
		if _, kept := nextItems[old.ItemInstanceID]; kept {
			continue
		}
		item := store.items[old.ItemInstanceID]
		item.Location = accountLocation
		store.items[old.ItemInstanceID] = cloneInstanceItem(item)
	}

	equippedLocation := economy.ItemLocation{
		Kind: economy.LocationKindShipEquipped,
		ID:   economy.LocationID(shipID.String()),
	}
	for _, module := range next {
		item := store.items[module.ItemInstanceID]
		item.Location = equippedLocation
		store.items[module.ItemInstanceID] = cloneInstanceItem(item)
	}
}

// MarkEquippedModuleBroken records a positive-to-zero durability transition for
// an item that is currently equipped by playerID on shipID.
func (store *InMemoryLoadoutStore) MarkEquippedModuleBroken(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	itemInstanceID foundation.ItemID,
) (EquippedModule, bool, error) {
	if err := playerID.Validate(); err != nil {
		return EquippedModule{}, false, err
	}
	if err := shipID.Validate(); err != nil {
		return EquippedModule{}, false, err
	}
	if err := itemInstanceID.Validate(); err != nil {
		return EquippedModule{}, false, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	item, ok := store.items[itemInstanceID]
	if !ok {
		return EquippedModule{}, false, fmt.Errorf("item %q: %w", itemInstanceID, ErrUnknownModuleItem)
	}
	if item.OwnerPlayerID != playerID {
		return EquippedModule{}, false, fmt.Errorf("item %q owner %q player %q: %w", itemInstanceID, item.OwnerPlayerID, playerID, ErrModuleItemNotOwned)
	}

	equipped, ok := store.equippedByItem[itemInstanceID]
	if !ok {
		return EquippedModule{}, false, fmt.Errorf("item %q: %w", itemInstanceID, ErrModuleItemNotEquipped)
	}
	if equipped.PlayerID != playerID || equipped.ShipID != shipID {
		return EquippedModule{}, false, fmt.Errorf("item %q equipped by player %q ship %q target player %q ship %q: %w", itemInstanceID, equipped.PlayerID, equipped.ShipID, playerID, shipID, ErrModuleItemNotEquipped)
	}
	if item.DurabilityCurrent <= 0 {
		return equipped, false, nil
	}

	item.DurabilityCurrent = 0
	store.items[itemInstanceID] = cloneInstanceItem(item)
	return equipped, true, nil
}

func playerShipStoreKey(playerID foundation.PlayerID, shipID foundation.ShipID) string {
	return playerID.String() + "\x00" + shipID.String()
}

func playerLoadoutStoreKey(playerID foundation.PlayerID, loadoutID LoadoutID) string {
	return playerID.String() + "\x00" + loadoutID.String()
}

func cloneInstanceItem(item economy.InstanceItem) economy.InstanceItem {
	item.MetadataJSON = append(item.MetadataJSON[:0:0], item.MetadataJSON...)
	return item
}
