package server

import (
	"testing"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestCoordinateItemUseRejectsWrongActiveMapWithoutMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSessionOnMap(t, gameServer, "coordinate-wrong-map@example.com", "Coordinate Wrong Map", "map_1_2", "west_gate")
	planetID := foundation.PlanetID("planet-coordinate-wrong-map")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, "map_1_2", world.Vec2{X: 1500, Y: 1600}, 3)
	createPayload := createCoordinateItemForTest(t, gameServer, owner, planetID, "request-coordinate-wrong-map-create")

	gameServer.runtime.mu.Lock()
	if _, err := gameServer.runtime.mapRouter.SetActiveLocationFromSpawn(owner.PlayerID, worldmaps.StarterMapID, worldmaps.StarterSpawnID); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("SetActiveLocationFromSpawn(%q) error = %v, want nil", worldmaps.StarterMapID, err)
	}
	gameServer.runtime.mu.Unlock()
	if err := gameServer.runtime.ensurePlayerSession(owner); err != nil {
		t.Fatalf("ensurePlayerSession(%q) error = %v, want nil", worldmaps.StarterMapID, err)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-wrong-map-use","op":"intel.coordinate_item.use","payload":{"item_instance_id":"`+createPayload.CoordinateItem.ItemInstanceID+`"},"client_seq":2,"v":1}`),
	)
	if !response.HasError {
		t.Fatalf("coordinate use response = %+v, want wrong active-map rejection", response.Response)
	}

	itemID := foundation.ItemID(createPayload.CoordinateItem.ItemInstanceID)
	item, ok, err := gameServer.runtime.Intel.CoordinateItem(itemID)
	if err != nil || !ok {
		t.Fatalf("CoordinateItem(%s) ok=%v err=%v, want existing item", itemID, ok, err)
	}
	if item.UsedAt != nil {
		t.Fatalf("coordinate item used_at = %v, want unused after wrong active-map rejection", item.UsedAt)
	}
	if !inventorySnapshotHasInstanceID(gameServer.runtime.inventorySnapshotForPlayer(owner.PlayerID), createPayload.CoordinateItem.ItemInstanceID, coordinateScrollItemID.String(), economy.LocationKindAccountInventory.String()) {
		t.Fatalf("inventory missing coordinate scroll %s after wrong active-map rejection", createPayload.CoordinateItem.ItemInstanceID)
	}
	assertCoordinateItemLedgerCount(t, gameServer, owner.PlayerID, createPayload.CoordinateItem.ItemInstanceID, economy.LedgerActionDecrease, intelCoordinateItemUseLedgerReason, 0)
}
