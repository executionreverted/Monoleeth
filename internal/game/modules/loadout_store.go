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

	loadouts       map[LoadoutID]Loadout
	activeShips    map[foundation.PlayerID]foundation.ShipID
	items          map[foundation.ItemID]economy.InstanceItem
	equippedByKey  map[string]map[ModuleSlotID]EquippedModule
	equippedByItem map[foundation.ItemID]EquippedModule
}

// NewInMemoryLoadoutStore returns an empty in-memory loadout store.
func NewInMemoryLoadoutStore() *InMemoryLoadoutStore {
	return &InMemoryLoadoutStore{
		loadouts:       make(map[LoadoutID]Loadout),
		activeShips:    make(map[foundation.PlayerID]foundation.ShipID),
		items:          make(map[foundation.ItemID]economy.InstanceItem),
		equippedByKey:  make(map[string]map[ModuleSlotID]EquippedModule),
		equippedByItem: make(map[foundation.ItemID]EquippedModule),
	}
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
	if existing, ok := store.loadouts[loadout.LoadoutID]; ok && existing.PlayerID != loadout.PlayerID {
		return fmt.Errorf("loadout %q owner %q player %q: %w", loadout.LoadoutID, existing.PlayerID, loadout.PlayerID, ErrLoadoutOwnerMismatch)
	}
	store.loadouts[loadout.LoadoutID] = cloneLoadout(loadout)
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
	loadout, ok := store.loadouts[loadoutID]
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
func (store *InMemoryLoadoutStore) ReplaceEquippedModules(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	equipped []EquippedModule,
) error {
	if err := playerID.Validate(); err != nil {
		return err
	}
	if err := shipID.Validate(); err != nil {
		return err
	}
	nextBySlot := make(map[ModuleSlotID]EquippedModule, len(equipped))
	nextItems := make(map[foundation.ItemID]ModuleSlotID, len(equipped))
	for _, module := range equipped {
		if err := module.Validate(); err != nil {
			return err
		}
		if module.PlayerID != playerID || module.ShipID != shipID {
			return fmt.Errorf("equipped player %q ship %q target player %q ship %q: %w", module.PlayerID, module.ShipID, playerID, shipID, ErrLoadoutShipMismatch)
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
	key := playerShipStoreKey(playerID, shipID)
	for _, module := range nextBySlot {
		if existing, ok := store.equippedByItem[module.ItemInstanceID]; ok {
			existingKey := playerShipStoreKey(existing.PlayerID, existing.ShipID)
			if existingKey != key {
				return fmt.Errorf("item %q equipped by player %q ship %q: %w", module.ItemInstanceID, existing.PlayerID, existing.ShipID, ErrModuleItemAlreadyEquipped)
			}
		}
	}
	for _, old := range store.equippedByKey[key] {
		delete(store.equippedByItem, old.ItemInstanceID)
	}
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

func cloneInstanceItem(item economy.InstanceItem) economy.InstanceItem {
	item.MetadataJSON = append(item.MetadataJSON[:0:0], item.MetadataJSON...)
	return item
}
