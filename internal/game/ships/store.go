package ships

import (
	"sort"
	"sync"

	"gameproject/internal/game/foundation"
)

// InMemoryHangarStore stores player ship ownership and active pointers for
// tests and early server slices. Updates are copy-on-write per player.
type InMemoryHangarStore struct {
	mu      sync.Mutex
	players map[foundation.PlayerID]hangarRecord
}

type hangarRecord struct {
	ships  map[foundation.ShipID]PlayerShipState
	active *ActiveShipState
}

// NewInMemoryHangarStore returns an empty in-memory hangar store.
func NewInMemoryHangarStore() *InMemoryHangarStore {
	return &InMemoryHangarStore{
		players: make(map[foundation.PlayerID]hangarRecord),
	}
}

// PutPlayerShip seeds or replaces a player ship row.
func (store *InMemoryHangarStore) PutPlayerShip(playerShip PlayerShipState) error {
	if err := playerShip.Validate(); err != nil {
		return err
	}
	return store.updatePlayerHangar(playerShip.PlayerID, func(record *hangarRecord) error {
		record.putShip(playerShip)
		return nil
	})
}

// PutActiveShip seeds or replaces a player's active ship pointer.
func (store *InMemoryHangarStore) PutActiveShip(activeShip ActiveShipState) error {
	if err := activeShip.Validate(); err != nil {
		return err
	}
	return store.updatePlayerHangar(activeShip.PlayerID, func(record *hangarRecord) error {
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

func (store *InMemoryHangarStore) updatePlayerHangar(playerID foundation.PlayerID, update func(*hangarRecord) error) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.players == nil {
		store.players = make(map[foundation.PlayerID]hangarRecord)
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

func (store *InMemoryHangarStore) viewPlayerHangar(playerID foundation.PlayerID, view func(hangarRecord) error) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	return view(cloneHangarRecord(store.players[playerID]))
}

func (record hangarRecord) ship(shipID foundation.ShipID) (PlayerShipState, bool) {
	playerShip, ok := record.ships[shipID]
	if !ok {
		return PlayerShipState{}, false
	}
	return clonePlayerShipState(playerShip), true
}

func (record *hangarRecord) putShip(playerShip PlayerShipState) {
	if record.ships == nil {
		record.ships = make(map[foundation.ShipID]PlayerShipState)
	}
	record.ships[playerShip.ShipID] = clonePlayerShipState(playerShip)
}

func (record hangarRecord) activeShip() (ActiveShipState, bool) {
	if record.active == nil {
		return ActiveShipState{}, false
	}
	return *record.active, true
}

func (record *hangarRecord) putActiveShip(activeShip ActiveShipState) {
	activeCopy := activeShip
	record.active = &activeCopy
}

func (record hangarRecord) sortedShips() []PlayerShipState {
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

func cloneHangarRecord(record hangarRecord) hangarRecord {
	clone := hangarRecord{}
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
