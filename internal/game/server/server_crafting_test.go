package server

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

func TestCraftingStartCreatesServerOwnedJobAndSnapshots(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "craft-start-owner@example.com", "Craft Start")
	seedCraftingStartInputs(t, gameServer, resolved.PlayerID, 40, 10)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-craft-start","op":"crafting.start","payload":{"recipe_id":"refined_alloy_batch"},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("crafting.start response error = %+v, want success", response.Error)
	}
	assertCraftingPayloadIsClientSafe(t, response.Response.Payload)

	var payload struct {
		Crafting  craftingSnapshotPayload  `json:"crafting"`
		Job       craftingJobPayload       `json:"job"`
		Wallet    walletSnapshotPayload    `json:"wallet"`
		Inventory inventorySnapshotPayload `json:"inventory"`
		Duplicate bool                     `json:"duplicate"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode crafting.start payload: %v", err)
	}
	if payload.Duplicate {
		t.Fatalf("duplicate = true, want first mutation")
	}
	if payload.Job.JobID == "" || payload.Job.RecipeID != "refined_alloy_batch" || payload.Job.State != "running" {
		t.Fatalf("job payload = %+v, want running refined alloy job", payload.Job)
	}
	if payload.Job.StartedAt <= 0 || payload.Job.CompletesAt <= payload.Job.StartedAt {
		t.Fatalf("job timestamps = %+v, want server timed completion", payload.Job)
	}
	if len(payload.Crafting.ActiveJobs) != 1 || payload.Crafting.ActiveJobs[0].JobID != payload.Job.JobID {
		t.Fatalf("active jobs = %+v, want started job in crafting snapshot", payload.Crafting.ActiveJobs)
	}
	if payload.Wallet.Credits >= starterWalletCredits+500 {
		t.Fatalf("wallet credits = %d, want craft fee debited", payload.Wallet.Credits)
	}
	if got := craftingStackQuantity(t, gameServer, resolved.PlayerID, economy.LocationKindAccountInventory, "iron_ore"); got != 20 {
		t.Fatalf("account iron_ore = %d, want 20 after reserve", got)
	}
	if got := craftingStackQuantity(t, gameServer, resolved.PlayerID, economy.LocationKindCraftingReserved, "iron_ore"); got != 20 {
		t.Fatalf("reserved iron_ore = %d, want 20", got)
	}

	snapshot := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-craft-snapshot","op":"crafting.recipes","payload":{},"client_seq":2,"v":1}`),
	)
	if snapshot.HasError {
		t.Fatalf("crafting.recipes response error = %+v, want success", snapshot.Error)
	}
	var snapshotPayload struct {
		Crafting craftingSnapshotPayload `json:"crafting"`
	}
	if err := json.Unmarshal(snapshot.Response.Payload, &snapshotPayload); err != nil {
		t.Fatalf("decode crafting snapshot: %v", err)
	}
	if len(snapshotPayload.Crafting.ActiveJobs) != 1 || snapshotPayload.Crafting.ActiveJobs[0].JobID != payload.Job.JobID {
		t.Fatalf("snapshot active jobs = %+v, want started job", snapshotPayload.Crafting.ActiveJobs)
	}
}

func TestCraftingStartRejectsSpoofedServerOwnedFields(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "craft-spoof-owner@example.com", "Craft Spoof")
	seedCraftingStartInputs(t, gameServer, resolved.PlayerID, 40, 10)

	tests := []struct {
		name  string
		field string
	}{
		{name: "player", field: `"player_id":"spoofed-player"`},
		{name: "job", field: `"job_id":"craft-job-spoofed"`},
		{name: "wallet", field: `"wallet_debit":{"credits":0}`},
		{name: "source", field: `"source_location":"account_inventory:spoofed"`},
		{name: "duration", field: `"duration":1`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(resolved.SessionID.String()),
				[]byte(`{"request_id":"request-craft-spoof-`+tt.name+`","op":"crafting.start","payload":{"recipe_id":"refined_alloy_batch",`+tt.field+`},"client_seq":1,"v":1}`),
			)
			if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
				t.Fatalf("spoof response = %+v, want invalid payload", response)
			}
			if got := len(gameServer.runtime.Crafting.Jobs()); got != 0 {
				t.Fatalf("craft jobs = %d, want none after spoofed payload", got)
			}
		})
	}
}

func TestCraftingStartDuplicateRequestDoesNotReserveTwice(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "craft-duplicate-owner@example.com", "Craft Duplicate")
	seedCraftingStartInputs(t, gameServer, resolved.PlayerID, 40, 10)

	body := []byte(`{"request_id":"request-craft-duplicate","op":"crafting.start","payload":{"recipe_id":"refined_alloy_batch"},"client_seq":1,"v":1}`)
	first := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), body)
	if first.HasError {
		t.Fatalf("first crafting.start response error = %+v, want success", first.Error)
	}
	second := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), body)
	if second.HasError {
		t.Fatalf("duplicate crafting.start response error = %+v, want cached success", second.Error)
	}
	if len(gameServer.runtime.Crafting.Jobs()) != 1 {
		t.Fatalf("craft jobs = %d, want one after duplicate request", len(gameServer.runtime.Crafting.Jobs()))
	}
	if got := craftingStackQuantity(t, gameServer, resolved.PlayerID, economy.LocationKindAccountInventory, "iron_ore"); got != 20 {
		t.Fatalf("account iron_ore = %d, want one reservation only", got)
	}
	if got := craftingStackQuantity(t, gameServer, resolved.PlayerID, economy.LocationKindCraftingReserved, "iron_ore"); got != 20 {
		t.Fatalf("reserved iron_ore = %d, want one reservation only", got)
	}
}

func seedCraftingStartInputs(t *testing.T, gameServer *Server, playerID foundation.PlayerID, ore int64, carbon int64) {
	t.Helper()
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		t.Fatalf("craft account location: %v", err)
	}
	seedCraftingItem(t, gameServer, playerID, location, "iron_ore", ore, "ore")
	seedCraftingItem(t, gameServer, playerID, location, "carbon_shards", carbon, "carbon")
	seedCraftingCredits(t, gameServer, playerID, 500)
}

func seedCraftingItem(t *testing.T, gameServer *Server, playerID foundation.PlayerID, location economy.ItemLocation, itemID foundation.ItemID, quantity int64, suffix string) {
	t.Helper()
	definition, ok := gameServer.runtime.itemCatalog[itemID]
	if !ok {
		t.Fatalf("item definition %q missing", itemID)
	}
	reference, err := foundation.LootPickupIdempotencyKey("craft-seed-" + playerID.String() + "-" + suffix)
	if err != nil {
		t.Fatalf("craft seed item reference: %v", err)
	}
	if _, err := gameServer.runtime.Inventory.AddItem(economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: definition,
		Quantity:       quantity,
		Location:       location,
		Reason:         economy.LedgerReason("test_craft_seed"),
		ReferenceKey:   reference,
	}); err != nil {
		t.Fatalf("seed crafting item %q: %v", itemID, err)
	}
}

func seedCraftingCredits(t *testing.T, gameServer *Server, playerID foundation.PlayerID, amount int64) {
	t.Helper()
	reference, err := foundation.QuestRewardIdempotencyKey(foundation.QuestID("craft-seed-" + playerID.String()))
	if err != nil {
		t.Fatalf("craft seed credits reference: %v", err)
	}
	if _, err := gameServer.runtime.Wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       amount,
		Reason:       economy.LedgerReason("test_craft_seed"),
		ReferenceKey: reference,
	}); err != nil {
		t.Fatalf("seed crafting credits: %v", err)
	}
}

func craftingStackQuantity(t *testing.T, gameServer *Server, playerID foundation.PlayerID, kind economy.LocationKind, itemID foundation.ItemID) int64 {
	t.Helper()
	var total int64
	for _, stack := range gameServer.runtime.Inventory.StackableItems() {
		if stack.OwnerPlayerID == playerID && stack.ItemID == itemID && stack.Location.Kind == kind {
			total += stack.Quantity.Int64()
		}
	}
	return total
}

func assertCraftingPayloadIsClientSafe(t *testing.T, payload json.RawMessage) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range []string{"player_id", "reservation_id", "reference_id", "source_location", "output_location", "wallet_debit"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("crafting payload leaked %q in %s", forbidden, raw)
		}
	}
}
