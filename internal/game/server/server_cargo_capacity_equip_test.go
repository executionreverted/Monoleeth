package server

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

// TestEquipCargoModuleIncreasesServerAndVisibleCargoCapacity proves equipping
// a cargo module raises the authoritative server cargo capacity and the
// visible stats payload by the module's flat bonus, reconciled on equip.
func TestEquipCargoModuleIncreasesServerAndVisibleCargoCapacity(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "cargo-equip@example.com", "Cargo Equip")
	sessionID := realtime.SessionID(owner.SessionID.String())

	gameServer.runtime.mu.Lock()
	beforeCargo := gameServer.runtime.players[owner.PlayerID].Cargo
	gameServer.runtime.mu.Unlock()
	if beforeCargo.Capacity != 100 {
		t.Fatalf("base cargo capacity = %d, want 100 (catalog authoritative)", beforeCargo.Capacity)
	}

	itemInstance := "cargo_expander_t1-instance-equip-test"
	seedModuleInstance(t, gameServer, owner.PlayerID, "cargo_expander_t1", itemInstance)

	unequipResponse := gameServer.runtime.Gateway.HandleRequest(sessionID, []byte(
		`{"request_id":"request-unequip-scanner","op":"loadout.unequip_module","payload":{"slot_id":"utility_1"},"client_seq":1,"v":1}`,
	))
	if unequipResponse.HasError {
		t.Fatalf("unequip scanner response error = %+v", unequipResponse.Error)
	}

	response := gameServer.runtime.Gateway.HandleRequest(sessionID, []byte(
		`{"request_id":"request-equip-cargo","op":"loadout.equip_module","payload":{"slot_id":"utility_1","item_instance_id":"`+itemInstance+`"},"client_seq":2,"v":1}`,
	))
	if response.HasError {
		t.Fatalf("equip cargo module response error = %+v", response.Error)
	}
	var equipPayload struct {
		Stats statsResponsePayload `json:"stats"`
	}
	if err := json.Unmarshal(response.Response.Payload, &equipPayload); err != nil {
		t.Fatalf("decode equip response: %v", err)
	}
	if equipPayload.Stats.CargoCapacity != 140 {
		t.Fatalf("visible stats cargo capacity after equip = %d, want 140 (100 base + 40 module)", equipPayload.Stats.CargoCapacity)
	}

	gameServer.runtime.mu.Lock()
	afterStats := gameServer.runtime.players[owner.PlayerID].Stats
	afterCargo := gameServer.runtime.players[owner.PlayerID].Cargo
	gameServer.runtime.mu.Unlock()
	if afterStats.CargoCapacity != 140 {
		t.Fatalf("server stats cargo capacity after equip = %d, want 140", afterStats.CargoCapacity)
	}
	if afterCargo.Capacity != 140 {
		t.Fatalf("server cargo capacity after equip = %d, want 140 (authoritative)", afterCargo.Capacity)
	}
}

type statsResponsePayload struct {
	CargoCapacity int64 `json:"cargo_capacity"`
}

func seedModuleInstance(t *testing.T, gameServer *Server, playerID foundation.PlayerID, itemID, instanceID string) {
	t.Helper()
	definition, ok := gameServer.runtime.itemCatalog[foundation.ItemID(itemID)]
	if !ok {
		t.Fatalf("module %q not in catalog", itemID)
	}
	accountInventory, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		t.Fatalf("account inventory location: %v", err)
	}
	if _, err := gameServer.runtime.Inventory.AddItem(economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: definition,
		ItemInstanceID: foundation.ItemID(instanceID),
		Quantity:       1,
		Location:       accountInventory,
		Reason:         runtimeSeedLedgerReason,
		ReferenceKey:   foundation.IdempotencyKey("admin_compensation:" + playerID.String() + ":cargo-equip-seed"),
	}); err != nil {
		t.Fatalf("seed module instance %q: %v", itemID, err)
	}
	if _, err := gameServer.runtime.Inventory.SystemSetInstanceDurability(playerID, foundation.ItemID(instanceID), 100); err != nil {
		t.Fatalf("seed module durability %q: %v", itemID, err)
	}
}
