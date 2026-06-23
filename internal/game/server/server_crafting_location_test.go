package server

import (
	"encoding/json"
	"testing"
	"time"

	"gameproject/internal/game/crafting"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
)

func TestCraftingStartAcceptsExplicitStationLocationIntent(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	resolved := createResolvedRuntimeSession(t, gameServer, "crafting-location@example.com", "Crafting Location")
	seedCraftingLocationInputs(t, gameServer, resolved.PlayerID, "explicit-station")

	start := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-location-station","op":"crafting.start","payload":{"recipe_id":"refined_alloy_batch","location_type":"station","location_id":"origin-station"},"client_seq":1,"v":1}`),
	)
	if start.HasError {
		t.Fatalf("crafting.start response error = %+v, want success", start.Error)
	}

	var payload struct {
		Crafting craftingSnapshotPayload `json:"crafting"`
	}
	if err := json.Unmarshal(start.Response.Payload, &payload); err != nil {
		t.Fatalf("decode crafting.start payload: %v", err)
	}
	if len(payload.Crafting.ActiveJobs) != 1 {
		t.Fatalf("active jobs = %+v, want one", payload.Crafting.ActiveJobs)
	}
	jobs := gameServer.runtime.Crafting.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("craft jobs = %+v, want one", jobs)
	}
	if jobs[0].Location.Type != crafting.CraftLocationStation || jobs[0].Location.ID != runtimeDefaultCraftStationID {
		t.Fatalf("job location = %+v, want explicit origin station", jobs[0].Location)
	}
}

func TestCraftingStartRejectsNonStationLocationForStationRecipeWithoutMutation(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	resolved := createResolvedRuntimeSession(t, gameServer, "crafting-location-reject@example.com", "Crafting Reject")
	seedCraftingLocationInputs(t, gameServer, resolved.PlayerID, "reject-owned-planet")

	before := craftingLocationEconomyState(t, gameServer, resolved.PlayerID)
	rejected := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-location-owned-planet","op":"crafting.start","payload":{"recipe_id":"refined_alloy_batch","location_type":"owned_planet","location_id":"planet-crafting-location"},"client_seq":1,"v":1}`),
	)
	if !rejected.HasError || rejected.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("crafting.start owned_planet = %+v, want forbidden", rejected)
	}
	if jobs := gameServer.runtime.Crafting.Jobs(); len(jobs) != 0 {
		t.Fatalf("craft jobs after rejected start = %+v, want none", jobs)
	}
	after := craftingLocationEconomyState(t, gameServer, resolved.PlayerID)
	if after.credits != before.credits {
		t.Fatalf("wallet credits after rejected start = %d, want %d", after.credits, before.credits)
	}
	if after.accountIronOre != before.accountIronOre || after.reservedIronOre != 0 {
		t.Fatalf("iron ore after rejected start = account %d reserved %d, want account %d reserved 0", after.accountIronOre, after.reservedIronOre, before.accountIronOre)
	}
	if after.accountCarbonShards != before.accountCarbonShards || after.reservedCarbonShards != 0 {
		t.Fatalf("carbon shards after rejected start = account %d reserved %d, want account %d reserved 0", after.accountCarbonShards, after.reservedCarbonShards, before.accountCarbonShards)
	}
}

func seedCraftingLocationInputs(t *testing.T, gameServer *Server, playerID foundation.PlayerID, suffix string) {
	t.Helper()
	accountLocation, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		t.Fatalf("account location: %v", err)
	}
	for _, seed := range []struct {
		itemID   foundation.ItemID
		quantity int64
	}{
		{itemID: "iron_ore", quantity: 20},
		{itemID: "carbon_shards", quantity: 5},
	} {
		definition, ok := gameServer.runtime.itemCatalog[seed.itemID]
		if !ok {
			t.Fatalf("runtime item %q missing", seed.itemID)
		}
		addTestInventoryStack(t, gameServer, playerID, definition, seed.quantity, accountLocation, "crafting-location-"+suffix+"-"+seed.itemID.String())
	}
}

type craftingLocationEconomySnapshot struct {
	credits              int64
	accountIronOre       int64
	reservedIronOre      int64
	accountCarbonShards  int64
	reservedCarbonShards int64
}

func craftingLocationEconomyState(t *testing.T, gameServer *Server, playerID foundation.PlayerID) craftingLocationEconomySnapshot {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	inventory := gameServer.runtime.inventorySnapshotLocked(playerID)
	return craftingLocationEconomySnapshot{
		credits:              gameServer.runtime.walletSnapshotLocked(playerID).Credits,
		accountIronOre:       inventoryStackQuantity(inventory, "iron_ore", economy.LocationKindAccountInventory.String()),
		reservedIronOre:      inventoryStackQuantity(inventory, "iron_ore", economy.LocationKindCraftingReserved.String()),
		accountCarbonShards:  inventoryStackQuantity(inventory, "carbon_shards", economy.LocationKindAccountInventory.String()),
		reservedCarbonShards: inventoryStackQuantity(inventory, "carbon_shards", economy.LocationKindCraftingReserved.String()),
	}
}
