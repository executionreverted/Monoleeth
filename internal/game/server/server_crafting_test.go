package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
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

func TestCraftingCompleteRejectsEarlyCompletionWithoutOutput(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	resolved := createResolvedRuntimeSession(t, gameServer, "craft-complete-early@example.com", "Craft Early")
	seedCraftingStartInputs(t, gameServer, resolved.PlayerID, 40, 10)
	job := startRefinedAlloyCraftForTest(t, gameServer, realtime.SessionID(resolved.SessionID.String()), "request-craft-early-start")

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-craft-early-complete","op":"crafting.complete","payload":{"job_id":"`+job.JobID+`"},"client_seq":2,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeCooldown {
		t.Fatalf("early crafting.complete response = %+v, want cooldown", response)
	}
	if got := craftingStackQuantity(t, gameServer, resolved.PlayerID, economy.LocationKindAccountInventory, "refined_alloy"); got != 0 {
		t.Fatalf("refined_alloy output = %d, want 0 before craft is ready", got)
	}
}

func TestCraftingCompleteGrantsOutputAndDuplicateDoesNotGrantTwice(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	resolved := createResolvedRuntimeSession(t, gameServer, "craft-complete-ready@example.com", "Craft Ready")
	seedCraftingStartInputs(t, gameServer, resolved.PlayerID, 40, 10)
	job := startRefinedAlloyCraftForTest(t, gameServer, realtime.SessionID(resolved.SessionID.String()), "request-craft-ready-start")
	clock.Advance(5 * time.Minute)

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-craft-ready-complete","op":"crafting.complete","payload":{"job_id":"`+job.JobID+`"},"client_seq":2,"v":1}`),
	)
	if first.HasError {
		t.Fatalf("crafting.complete response error = %+v, want success", first.Error)
	}
	assertCraftingPayloadIsClientSafe(t, first.Response.Payload)
	var payload struct {
		Crafting    craftingSnapshotPayload    `json:"crafting"`
		Job         craftingJobPayload         `json:"job"`
		Inventory   inventorySnapshotPayload   `json:"inventory"`
		Progression progressionSnapshotPayload `json:"progression"`
		Duplicate   bool                       `json:"duplicate"`
	}
	if err := json.Unmarshal(first.Response.Payload, &payload); err != nil {
		t.Fatalf("decode crafting.complete payload: %v", err)
	}
	if payload.Duplicate || payload.Job.JobID != job.JobID || payload.Job.State != "completed" {
		t.Fatalf("complete payload = %+v, want first completed job", payload)
	}
	if payload.Progression.MainXP <= 0 {
		t.Fatalf("progression payload = %+v, want XP snapshot after craft completion", payload.Progression)
	}
	if got := craftingStackQuantity(t, gameServer, resolved.PlayerID, economy.LocationKindAccountInventory, "refined_alloy"); got != 5 {
		t.Fatalf("refined_alloy output = %d, want 5 after completion", got)
	}

	duplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-craft-ready-complete-retry","op":"crafting.complete","payload":{"job_id":"`+job.JobID+`"},"client_seq":3,"v":1}`),
	)
	if duplicate.HasError {
		t.Fatalf("duplicate crafting.complete response error = %+v, want success", duplicate.Error)
	}
	var duplicatePayload struct {
		Duplicate bool `json:"duplicate"`
	}
	if err := json.Unmarshal(duplicate.Response.Payload, &duplicatePayload); err != nil {
		t.Fatalf("decode duplicate crafting.complete payload: %v", err)
	}
	if !duplicatePayload.Duplicate {
		t.Fatalf("duplicate = false, want true for second complete request")
	}
	if got := craftingStackQuantity(t, gameServer, resolved.PlayerID, economy.LocationKindAccountInventory, "refined_alloy"); got != 5 {
		t.Fatalf("refined_alloy output after duplicate = %d, want still 5", got)
	}
}

func TestCraftingCancelReleasesReservationAndDuplicateDoesNotReleaseTwice(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "craft-cancel@example.com", "Craft Cancel")
	seedCraftingStartInputs(t, gameServer, resolved.PlayerID, 40, 10)
	job := startRefinedAlloyCraftForTest(t, gameServer, realtime.SessionID(resolved.SessionID.String()), "request-craft-cancel-start")

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-craft-cancel","op":"crafting.cancel","payload":{"job_id":"`+job.JobID+`"},"client_seq":2,"v":1}`),
	)
	if first.HasError {
		t.Fatalf("crafting.cancel response error = %+v, want success", first.Error)
	}
	assertCraftingPayloadIsClientSafe(t, first.Response.Payload)
	var payload struct {
		Crafting  craftingSnapshotPayload  `json:"crafting"`
		Job       craftingJobPayload       `json:"job"`
		Inventory inventorySnapshotPayload `json:"inventory"`
		Wallet    walletSnapshotPayload    `json:"wallet"`
		Duplicate bool                     `json:"duplicate"`
	}
	if err := json.Unmarshal(first.Response.Payload, &payload); err != nil {
		t.Fatalf("decode crafting.cancel payload: %v", err)
	}
	if payload.Duplicate || payload.Job.JobID != job.JobID || payload.Job.State != "cancelled" {
		t.Fatalf("cancel payload = %+v, want first cancelled job", payload)
	}
	if got := craftingStackQuantity(t, gameServer, resolved.PlayerID, economy.LocationKindAccountInventory, "iron_ore"); got != 40 {
		t.Fatalf("account iron_ore after cancel = %d, want released 40", got)
	}
	if got := craftingStackQuantity(t, gameServer, resolved.PlayerID, economy.LocationKindCraftingReserved, "iron_ore"); got != 0 {
		t.Fatalf("reserved iron_ore after cancel = %d, want 0", got)
	}
	cancelledCredits := payload.Wallet.Credits

	duplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-craft-cancel-retry","op":"crafting.cancel","payload":{"job_id":"`+job.JobID+`"},"client_seq":3,"v":1}`),
	)
	if duplicate.HasError {
		t.Fatalf("duplicate crafting.cancel response error = %+v, want success", duplicate.Error)
	}
	var duplicatePayload struct {
		Wallet    walletSnapshotPayload `json:"wallet"`
		Duplicate bool                  `json:"duplicate"`
	}
	if err := json.Unmarshal(duplicate.Response.Payload, &duplicatePayload); err != nil {
		t.Fatalf("decode duplicate crafting.cancel payload: %v", err)
	}
	if !duplicatePayload.Duplicate {
		t.Fatal("duplicate = false, want true for second cancel request")
	}
	if duplicatePayload.Wallet.Credits != cancelledCredits {
		t.Fatalf("duplicate cancel credits = %d, want %d", duplicatePayload.Wallet.Credits, cancelledCredits)
	}
	if got := craftingStackQuantity(t, gameServer, resolved.PlayerID, economy.LocationKindAccountInventory, "iron_ore"); got != 40 {
		t.Fatalf("account iron_ore after duplicate cancel = %d, want still 40", got)
	}
}

func TestCraftingCancelRejectsSpoofedFieldsAndWrongOwner(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	owner := createResolvedRuntimeSession(t, gameServer, "craft-cancel-owner@example.com", "Craft Cancel Owner")
	other := createResolvedRuntimeSession(t, gameServer, "craft-cancel-other@example.com", "Craft Cancel Other")
	seedCraftingStartInputs(t, gameServer, owner.PlayerID, 40, 10)
	job := startRefinedAlloyCraftForTest(t, gameServer, realtime.SessionID(owner.SessionID.String()), "request-craft-cancel-sec-start")

	spoof := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-craft-cancel-spoof","op":"crafting.cancel","payload":{"job_id":"`+job.JobID+`","reservation_release":{"state":"released"}},"client_seq":2,"v":1}`),
	)
	if !spoof.HasError || spoof.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoof crafting.cancel response = %+v, want invalid payload", spoof)
	}
	if got := craftingStackQuantity(t, gameServer, owner.PlayerID, economy.LocationKindCraftingReserved, "iron_ore"); got != 20 {
		t.Fatalf("reserved iron_ore after spoof = %d, want still 20", got)
	}

	wrongOwner := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(other.SessionID.String()),
		[]byte(`{"request_id":"request-craft-cancel-wrong-owner","op":"crafting.cancel","payload":{"job_id":"`+job.JobID+`"},"client_seq":2,"v":1}`),
	)
	if !wrongOwner.HasError || wrongOwner.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("wrong-owner crafting.cancel response = %+v, want forbidden", wrongOwner)
	}
	if got := craftingStackQuantity(t, gameServer, owner.PlayerID, economy.LocationKindCraftingReserved, "iron_ore"); got != 20 {
		t.Fatalf("reserved iron_ore after wrong owner = %d, want still 20", got)
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

func startRefinedAlloyCraftForTest(t *testing.T, gameServer *Server, sessionID realtime.SessionID, requestID string) craftingJobPayload {
	t.Helper()
	response := gameServer.runtime.Gateway.HandleRequest(
		sessionID,
		[]byte(`{"request_id":"`+requestID+`","op":"crafting.start","payload":{"recipe_id":"refined_alloy_batch"},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("crafting.start response error = %+v, want success", response.Error)
	}
	var payload struct {
		Job craftingJobPayload `json:"job"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode crafting.start helper payload: %v", err)
	}
	if payload.Job.JobID == "" {
		t.Fatalf("crafting.start helper job is empty")
	}
	return payload.Job
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
