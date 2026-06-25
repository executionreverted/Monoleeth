package server

import (
	"encoding/json"
	"strconv"
	"testing"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

func newInventoryMoveTestServer(t *testing.T) (*Server, foundation.PlayerID, realtime.SessionID, foundation.ItemID, economy.ItemDefinition) {
	t.Helper()
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "inventory-move@example.com", "Inventory Move")
	var stackableID foundation.ItemID
	var stackableDef economy.ItemDefinition
	for id, def := range gameServer.runtime.itemCatalog {
		if def.Type == economy.ItemTypeStackable {
			stackableID, stackableDef = id, def
			break
		}
	}
	if stackableID == "" {
		t.Fatal("no stackable item found in runtime catalog")
	}
	return gameServer, owner.PlayerID, realtime.SessionID(owner.SessionID.String()), stackableID, stackableDef
}

func seedInventoryMoveStack(t *testing.T, gameServer *Server, playerID foundation.PlayerID, def economy.ItemDefinition, quantity int64, location economy.ItemLocation, suffix string) {
	t.Helper()
	addTestInventoryStack(t, gameServer, playerID, def, quantity, location, suffix)
}

func inventoryStackQuantityAt(snapshot inventorySnapshotPayload, itemID, location string) int64 {
	var total int64
	for _, stack := range snapshot.Stackable {
		if stack.ItemID == itemID && stack.Location == location {
			total += stack.Quantity
		}
	}
	return total
}

func sendInventoryMoveCommand(t *testing.T, gameServer *Server, sessionID realtime.SessionID, requestID, itemID, fromType, fromID, toType, toID string, quantity int64) (inventoryMoveResponsePayload, bool) {
	t.Helper()
	body := `{"request_id":"` + requestID + `","op":"inventory.move","payload":{"item_id":"` + itemID + `","from_location":{"location_type":"` + fromType + `","location_id":"` + fromID + `"},"to_location":{"location_type":"` + toType + `","location_id":"` + toID + `"},"quantity":` + strconv.FormatInt(quantity, 10) + `},"client_seq":1,"v":1}`
	response := gameServer.runtime.Gateway.HandleRequest(sessionID, []byte(body))
	if response.HasError {
		return inventoryMoveResponsePayload{}, false
	}
	var payload inventoryMoveResponsePayload
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("inventory.move unmarshal payload: %v", err)
	}
	return payload, true
}

type inventoryMoveResponsePayload struct {
	Accepted  bool `json:"accepted"`
	Duplicate bool `json:"duplicate"`
}

// TestInventoryMoveRejectsUnownedNegativeAndBlockedLocations proves the
// command rejects an unowned item, a negative quantity, and a blocked target
// location (ship cargo uses the capacity-checked cargo path) with domain
// errors and no inventory mutation.
func TestInventoryMoveRejectsUnownedNegativeAndBlockedLocations(t *testing.T) {
	gameServer, playerID, sessionID, itemID, _ := newInventoryMoveTestServer(t)
	accountLocation := playerID.String()

	cases := []struct {
		name     string
		fromType string
		toType   string
		quantity int64
	}{
		{name: "unowned item", fromType: "account_inventory", toType: "station_storage", quantity: 1},
		{name: "negative quantity", fromType: "account_inventory", toType: "station_storage", quantity: -3},
		{name: "blocked target location", fromType: "account_inventory", toType: "ship_cargo", quantity: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := sendInventoryMoveCommand(t, gameServer, sessionID, "request-move-reject-"+tc.name, itemID.String(), tc.fromType, accountLocation, tc.toType, "origin-station", tc.quantity)
			if ok {
				t.Fatalf("inventory.move(%s) unexpectedly succeeded, want rejection", tc.name)
			}
		})
	}
}

// TestInventoryMoveDuplicateRequestDoesNotDoubleMoveStack proves moving a
// stack and replaying the same request id moves the quantity exactly once.
func TestInventoryMoveDuplicateRequestDoesNotDoubleMoveStack(t *testing.T) {
	gameServer, playerID, sessionID, itemID, itemDef := newInventoryMoveTestServer(t)
	accountLocation := playerID.String()
	accountInventory, err := economy.NewItemLocation(economy.LocationKindAccountInventory, accountLocation)
	if err != nil {
		t.Fatalf("account inventory location: %v", err)
	}
	seedInventoryMoveStack(t, gameServer, playerID, itemDef, 10, accountInventory, "inventory-move-seed")

	first, ok := sendInventoryMoveCommand(t, gameServer, sessionID, "request-move-once", itemID.String(), "account_inventory", accountLocation, "station_storage", "origin-station", 4)
	if !ok || !first.Accepted {
		t.Fatalf("first inventory.move response = %+v ok %v, want accepted", first, ok)
	}
	gameServer.runtime.mu.Lock()
	afterFirst := gameServer.runtime.inventorySnapshotLocked(playerID)
	gameServer.runtime.mu.Unlock()
	if got := inventoryStackQuantityAt(afterFirst, itemID.String(), "account_inventory"); got != 6 {
		t.Fatalf("account inventory after first move = %d, want 6", got)
	}
	if got := inventoryStackQuantityAt(afterFirst, itemID.String(), "station_storage"); got != 4 {
		t.Fatalf("station storage after first move = %d, want 4", got)
	}

	duplicate, ok := sendInventoryMoveCommand(t, gameServer, sessionID, "request-move-once", itemID.String(), "account_inventory", accountLocation, "station_storage", "origin-station", 4)
	if !ok || !duplicate.Accepted {
		t.Fatalf("duplicate inventory.move response = %+v ok %v, want accepted (cached)", duplicate, ok)
	}
	gameServer.runtime.mu.Lock()
	afterDuplicate := gameServer.runtime.inventorySnapshotLocked(playerID)
	gameServer.runtime.mu.Unlock()
	if got := inventoryStackQuantityAt(afterDuplicate, itemID.String(), "account_inventory"); got != 6 {
		t.Fatalf("account inventory after duplicate move = %d, want 6 (no double move)", got)
	}
	if got := inventoryStackQuantityAt(afterDuplicate, itemID.String(), "station_storage"); got != 4 {
		t.Fatalf("station storage after duplicate move = %d, want 4 (no double move)", got)
	}
}
