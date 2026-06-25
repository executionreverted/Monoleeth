package ships

import (
	"sort"
	"sync"

	"gameproject/internal/game/foundation"
)

// HangarStore persists player ship ownership and active pointers for one
// player hangar. Implementations must apply UpdatePlayerHangar atomically per
// player.
type HangarStore interface {
	UpdatePlayerHangar(playerID foundation.PlayerID, update func(*HangarRecord) error) error
	ViewPlayerHangar(playerID foundation.PlayerID, view func(HangarRecord) error) error
}

var _ HangarStore = (*InMemoryHangarStore)(nil)

// InMemoryHangarStore stores player ship ownership and active pointers for
// tests and early server slices. Updates are copy-on-write per player.
type InMemoryHangarStore struct {
	mu      sync.Mutex
	players map[foundation.PlayerID]HangarRecord
}

// HangarRecord is one player's ship ownership rows and active ship pointer.
type HangarRecord struct {
	ships  map[foundation.ShipID]PlayerShipState
	active *ActiveShipState
}

// NewInMemoryHangarStore returns an empty in-memory hangar store.
func NewInMemoryHangarStore() *InMemoryHangarStore {
	return &InMemoryHangarStore{
		players: make(map[foundation.PlayerID]HangarRecord),
	}
}

// PutPlayerShip seeds or replaces a player ship row.
func (store *InMemoryHangarStore) PutPlayerShip(playerShip PlayerShipState) error {
	if err := playerShip.Validate(); err != nil {
		return err
	}
	return store.UpdatePlayerHangar(playerShip.PlayerID, func(record *HangarRecord) error {
		record.putShip(playerShip)
		return nil
	})
}

// PutActiveShip seeds or replaces a player's active ship pointer.
func (store *InMemoryHangarStore) PutActiveShip(activeShip ActiveShipState) error {
	if err := activeShip.Validate(); err != nil {
		return err
	}
	return store.UpdatePlayerHangar(activeShip.PlayerID, func(record *HangarRecord) error {
		record.putActiveShip(activeShip)
		return nil
	})
}

// PlayerShip returns one player ship row, if present.
func (store *InMemoryHangarStore) PlayerShip(playerID foundation.PlayerID, shipID foundation.ShipID) (PlayerShipState, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	record := store.players[playerID]
	return record.ship(shipID)
}

// ActiveShip returns the player's active ship pointer, if present.
func (store *InMemoryHangarStore) ActiveShip(playerID foundation.PlayerID) (ActiveShipState, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	record := store.players[playerID]
	return record.activeShip()
}

// UpdatePlayerHangar applies one atomic per-player hangar mutation.
func (store *InMemoryHangarStore) UpdatePlayerHangar(playerID foundation.PlayerID, update func(*HangarRecord) error) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.players == nil {
		store.players = make(map[foundation.PlayerID]HangarRecord)
	}

	working := cloneHangarRecord(store.players[playerID])
	if working.ships == nil {
		working.ships = make(map[foundation.ShipID]PlayerShipState)
	}
	if err := update(&working); err != nil {
		return err
	}

	store.players[playerID] = cloneHangarRecord(working)
	return nil
}

// ViewPlayerHangar returns one cloned per-player hangar view.
func (store *InMemoryHangarStore) ViewPlayerHangar(playerID foundation.PlayerID, view func(HangarRecord) error) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	return view(cloneHangarRecord(store.players[playerID]))
}

// PlayerShip returns one ship row from the working hangar record.
func (record HangarRecord) PlayerShip(shipID foundation.ShipID) (PlayerShipState, bool) {
	return record.ship(shipID)
}

// PutPlayerShip stores one ship row in the working hangar record.
func (record *HangarRecord) PutPlayerShip(playerShip PlayerShipState) {
	record.putShip(playerShip)
}

// ActiveShip returns the active ship pointer from the working hangar record.
func (record HangarRecord) ActiveShip() (ActiveShipState, bool) {
	return record.activeShip()
}

// PutActiveShip stores the active ship pointer in the working hangar record.
func (record *HangarRecord) PutActiveShip(activeShip ActiveShipState) {
	record.putActiveShip(activeShip)
}

// PlayerShips returns player ship rows in stable ship-id order.
func (record HangarRecord) PlayerShips() []PlayerShipState {
	return record.sortedShips()
}

func (record HangarRecord) ship(shipID foundation.ShipID) (PlayerShipState, bool) {
	playerShip, ok := record.ships[shipID]
	if !ok {
		return PlayerShipState{}, false
	}
	return clonePlayerShipState(playerShip), true
}

func (record *HangarRecord) putShip(playerShip PlayerShipState) {
	if record.ships == nil {
		record.ships = make(map[foundation.ShipID]PlayerShipState)
	}
	record.ships[playerShip.ShipID] = clonePlayerShipState(playerShip)
}

func (record HangarRecord) activeShip() (ActiveShipState, bool) {
	if record.active == nil {
		return ActiveShipState{}, false
	}
	return *record.active, true
}

func (record *HangarRecord) putActiveShip(activeShip ActiveShipState) {
	activeCopy := activeShip
	record.active = &activeCopy
}

func (record HangarRecord) sortedShips() []PlayerShipState {
	shipIDs := make([]foundation.ShipID, 0, len(record.ships))
	for shipID := range record.ships {
		shipIDs = append(shipIDs, shipID)
	}
	sort.Slice(shipIDs, func(i, j int) bool {
		return shipIDs[i].String() < shipIDs[j].String()
	})

	ships := make([]PlayerShipState, 0, len(shipIDs))
	for _, shipID := range shipIDs {
		ships = append(ships, clonePlayerShipState(record.ships[shipID]))
	}
	return ships
}

func cloneHangarRecord(record HangarRecord) HangarRecord {
	clone := HangarRecord{}
	if len(record.ships) > 0 {
		clone.ships = make(map[foundation.ShipID]PlayerShipState, len(record.ships))
		for shipID, playerShip := range record.ships {
			clone.ships[shipID] = clonePlayerShipState(playerShip)
		}
	}
	if record.active != nil {
		activeCopy := *record.active
		clone.active = &activeCopy
	}
	return clone
}

func clonePlayerShipState(playerShip PlayerShipState) PlayerShipState {
	clone := playerShip
	if playerShip.DisabledAt != nil {
		disabledAt := *playerShip.DisabledAt
		clone.DisabledAt = &disabledAt
	}
	if playerShip.LastRepairedAt != nil {
		lastRepairedAt := *playerShip.LastRepairedAt
		clone.LastRepairedAt = &lastRepairedAt
	}
	if len(playerShip.MetadataJSON) > 0 {
		clone.MetadataJSON = append([]byte(nil), playerShip.MetadataJSON...)
	}
	return clone
}
