package server

import (
	"encoding/json"
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
)

func TestCraftingCancelRefundsReservedServerOwnedState(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	resolved := createResolvedRuntimeSession(t, gameServer, "crafting-cancel@example.com", "Craft Cancel")
	seedCraftingCancelInputs(t, gameServer, resolved.PlayerID)

	start := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-cancel-start","op":"crafting.start","payload":{"recipe_id":"refined_alloy_batch"},"client_seq":1,"v":1}`),
	)
	if start.HasError {
		t.Fatalf("crafting.start response error = %+v, want success", start.Error)
	}
	var startPayload struct {
		Crafting  craftingSnapshotPayload  `json:"crafting"`
		Inventory inventorySnapshotPayload `json:"inventory"`
		Wallet    walletSnapshotPayload    `json:"wallet"`
	}
	if err := json.Unmarshal(start.Response.Payload, &startPayload); err != nil {
		t.Fatalf("decode crafting.start payload: %v", err)
	}
	if len(startPayload.Crafting.ActiveJobs) != 1 {
		t.Fatalf("active jobs after start = %+v, want one", startPayload.Crafting.ActiveJobs)
	}
	jobID := startPayload.Crafting.ActiveJobs[0].JobID
	if startPayload.Wallet.Credits != starterWalletCredits-100 {
		t.Fatalf("wallet after start = %d, want %d", startPayload.Wallet.Credits, starterWalletCredits-100)
	}
	if got := inventoryStackQuantity(startPayload.Inventory, "iron_ore", economy.LocationKindCraftingReserved.String()); got != 20 {
		t.Fatalf("reserved iron_ore after start = %d, want 20", got)
	}

	cancel := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-cancel","op":"crafting.cancel","payload":{"job_id":"`+jobID+`"},"client_seq":2,"v":1}`),
	)
	if cancel.HasError {
		t.Fatalf("crafting.cancel response error = %+v, want success", cancel.Error)
	}
	assertCraftingPayloadOmitsInternals(t, "crafting.cancel response", cancel.Response.Payload)
	var cancelPayload struct {
		Crafting  craftingSnapshotPayload  `json:"crafting"`
		Inventory inventorySnapshotPayload `json:"inventory"`
		Wallet    walletSnapshotPayload    `json:"wallet"`
	}
	if err := json.Unmarshal(cancel.Response.Payload, &cancelPayload); err != nil {
		t.Fatalf("decode crafting.cancel payload: %v", err)
	}
	if len(cancelPayload.Crafting.ActiveJobs) != 0 {
		t.Fatalf("active jobs after cancel = %+v, want none", cancelPayload.Crafting.ActiveJobs)
	}
	if cancelPayload.Wallet.Credits != starterWalletCredits {
		t.Fatalf("wallet after cancel = %d, want refunded %d", cancelPayload.Wallet.Credits, starterWalletCredits)
	}
	if got := inventoryStackQuantity(cancelPayload.Inventory, "iron_ore", economy.LocationKindAccountInventory.String()); got != 20 {
		t.Fatalf("account iron_ore after cancel = %d, want 20", got)
	}
	if got := inventoryStackQuantity(cancelPayload.Inventory, "carbon_shards", economy.LocationKindAccountInventory.String()); got != 5 {
		t.Fatalf("account carbon_shards after cancel = %d, want 5", got)
	}
	if got := inventoryStackQuantity(cancelPayload.Inventory, "iron_ore", economy.LocationKindCraftingReserved.String()); got != 0 {
		t.Fatalf("reserved iron_ore after cancel = %d, want 0", got)
	}

	duplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-cancel-duplicate","op":"crafting.cancel","payload":{"job_id":"`+jobID+`"},"client_seq":3,"v":1}`),
	)
	if duplicate.HasError {
		t.Fatalf("duplicate crafting.cancel response error = %+v, want success", duplicate.Error)
	}
	var duplicatePayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
		Wallet    walletSnapshotPayload    `json:"wallet"`
	}
	if err := json.Unmarshal(duplicate.Response.Payload, &duplicatePayload); err != nil {
		t.Fatalf("decode duplicate crafting.cancel payload: %v", err)
	}
	if duplicatePayload.Wallet.Credits != starterWalletCredits {
		t.Fatalf("wallet after duplicate cancel = %d, want %d", duplicatePayload.Wallet.Credits, starterWalletCredits)
	}
	if got := inventoryStackQuantity(duplicatePayload.Inventory, "iron_ore", economy.LocationKindAccountInventory.String()); got != 20 {
		t.Fatalf("account iron_ore after duplicate cancel = %d, want 20", got)
	}

	complete := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-cancel-complete","op":"crafting.complete","payload":{"job_id":"`+jobID+`"},"client_seq":4,"v":1}`),
	)
	if !complete.HasError || complete.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("crafting.complete after cancel = %+v, want forbidden", complete)
	}
}

func seedCraftingCancelInputs(t *testing.T, gameServer *Server, playerID foundation.PlayerID) {
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
		addTestInventoryStack(t, gameServer, playerID, definition, seed.quantity, accountLocation, "crafting-cancel-"+seed.itemID.String())
	}
}
