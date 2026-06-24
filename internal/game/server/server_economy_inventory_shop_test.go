package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
)

func TestPhase06SnapshotQueriesUseServerResolvedState(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	requests := []struct {
		name string
		body string
	}{
		{
			name: "progression",
			body: `{"request_id":"request-progression-snapshot","op":"progression.snapshot","payload":{},"client_seq":1,"v":1}`,
		},
		{
			name: "inventory",
			body: `{"request_id":"request-inventory-snapshot","op":"inventory.snapshot","payload":{},"client_seq":2,"v":1}`,
		},
		{
			name: "hangar",
			body: `{"request_id":"request-hangar-snapshot","op":"hangar.snapshot","payload":{},"client_seq":3,"v":1}`,
		},
		{
			name: "loadout",
			body: `{"request_id":"request-loadout-snapshot","op":"loadout.snapshot","payload":{},"client_seq":4,"v":1}`,
		},
		{
			name: "stats",
			body: `{"request_id":"request-stats-snapshot","op":"stats.snapshot","payload":{},"client_seq":5,"v":1}`,
		},
		{
			name: "crafting",
			body: `{"request_id":"request-crafting-recipes","op":"crafting.recipes","payload":{},"client_seq":6,"v":1}`,
		},
	}

	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			writeText(t, conn, request.body)
			response := readResponse(t, conn)
			if !response.OK {
				t.Fatalf("%s response = %+v, want success", request.name, response)
			}
			raw := string(response.Payload)
			for _, forbidden := range []string{"account_id", "player_id", "session_id", "world_id", "zone_id", "gameplay_seed", "loot_table"} {
				if strings.Contains(raw, forbidden) {
					t.Fatalf("%s response leaked %q in %s", request.name, forbidden, raw)
				}
			}

			switch request.name {
			case "progression":
				var payload struct {
					Progression progressionSnapshotPayload `json:"progression"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode progression snapshot: %v", err)
				}
				if payload.Progression.Rank != 1 || payload.Progression.MainLevel < 1 {
					t.Fatalf("progression payload = %+v, want starter server snapshot", payload.Progression)
				}
			case "inventory":
				var payload struct {
					Inventory inventorySnapshotPayload `json:"inventory"`
					Cargo     cargoSnapshotPayload     `json:"cargo"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode inventory snapshot: %v", err)
				}
				if len(payload.Inventory.Stackable) != 0 ||
					len(payload.Inventory.Instances) != 3 ||
					payload.Inventory.Counts.EquippedInstances != 1 ||
					payload.Cargo.Capacity != 60 {
					t.Fatalf("inventory payload = %+v cargo=%+v, want starter modules and cargo capacity", payload.Inventory, payload.Cargo)
				}
			case "hangar":
				var payload struct {
					Hangar hangarSnapshotPayload `json:"hangar"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode hangar snapshot: %v", err)
				}
				if payload.Hangar.ActiveShipID != starterShipID.String() || len(payload.Hangar.Ships) != 1 {
					t.Fatalf("hangar payload = %+v, want active starter ship", payload.Hangar)
				}
			case "loadout":
				var payload struct {
					Loadout loadoutSnapshotPayload `json:"loadout"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode loadout snapshot: %v", err)
				}
				if payload.Loadout.ActiveShipID != starterShipID.String() || len(payload.Loadout.Slots) != 3 {
					t.Fatalf("loadout payload = %+v, want starter slot snapshot", payload.Loadout)
				}
			case "stats":
				var payload struct {
					Stats statSnapshotPayload `json:"stats"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode stats snapshot: %v", err)
				}
				if payload.Stats.RadarRange != defaultRadarRange ||
					payload.Stats.CargoCapacity != 60 ||
					payload.Stats.LootPickupRange != runtimeLootPickupRange ||
					payload.Stats.BasicLaserEnergyCost != runtimeBasicLaserEnergyCost ||
					payload.Stats.BasicLaserCooldownMS != runtimeBasicLaserCooldownMS {
					t.Fatalf("stats payload = %+v, want starter effective stats", payload.Stats)
				}
			case "crafting":
				var payload struct {
					Crafting craftingSnapshotPayload `json:"crafting"`
				}
				if err := json.Unmarshal(response.Payload, &payload); err != nil {
					t.Fatalf("decode crafting recipes: %v", err)
				}
				if len(payload.Crafting.Recipes) < 3 || payload.Crafting.Recipes[0].CraftDurationMS <= 0 {
					t.Fatalf("crafting payload = %+v, want MVP recipes with millisecond durations", payload.Crafting)
				}
			}
		})
	}

	writeText(t, conn, `{"request_id":"request-progression-spoof","op":"progression.snapshot","payload":{"xp":999,"rank":99},"client_seq":7,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed progression error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})
	gameServer.runtime.tickAndCollectAOIEvents()
	dropID := killTrainingNPCForDrop(t, conn)
	writeText(t, conn, `{"request_id":"request-phase06-loot","op":"loot.pickup","payload":{"drop_id":"`+dropID+`"},"client_seq":8,"v":1}`)
	pickup := readResponseSkippingEvents(t, conn)
	if !pickup.OK {
		t.Fatalf("phase06 pickup response = %+v, want success", pickup)
	}
	var pickupPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
	}
	if err := json.Unmarshal(pickup.Payload, &pickupPayload); err != nil {
		t.Fatalf("decode pickup inventory: %v", err)
	}
	if len(pickupPayload.Inventory.Stackable) != 1 ||
		pickupPayload.Inventory.Stackable[0].ItemID != "raw_ore" ||
		pickupPayload.Inventory.Stackable[0].Quantity != 3 ||
		pickupPayload.Inventory.Stackable[0].Location != economy.LocationKindShipCargo.String() {
		t.Fatalf("pickup inventory = %+v, want real raw ore in ship cargo", pickupPayload.Inventory)
	}
}

func TestCraftingStartAndCompleteUseServerOwnedEconomyState(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	resolved := createResolvedRuntimeSession(t, gameServer, "crafting-owner@example.com", "Crafting Owner")
	accountLocation, err := economy.NewItemLocation(economy.LocationKindAccountInventory, resolved.PlayerID.String())
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
		addTestInventoryStack(t, gameServer, resolved.PlayerID, definition, seed.quantity, accountLocation, "crafting-"+seed.itemID.String())
	}

	spoof := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-spoof","op":"crafting.start","payload":{"recipe_id":"refined_alloy_batch","player_id":"spoof","materials":{"iron_ore":999},"wallet":{"credits":999999}},"client_seq":1,"v":1}`),
	)
	if !spoof.HasError || spoof.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed crafting.start = %+v, want invalid payload", spoof)
	}
	if jobs := gameServer.runtime.Crafting.Jobs(); len(jobs) != 0 {
		t.Fatalf("jobs after spoofed crafting.start = %+v, want none", jobs)
	}

	start := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-start","op":"crafting.start","payload":{"recipe_id":"refined_alloy_batch"},"client_seq":2,"v":1}`),
	)
	if start.HasError {
		t.Fatalf("crafting.start response error = %+v, want success", start.Error)
	}
	assertCraftingPayloadOmitsInternals(t, "crafting.start response", start.Response.Payload)
	var startPayload struct {
		Crafting  craftingSnapshotPayload  `json:"crafting"`
		Inventory inventorySnapshotPayload `json:"inventory"`
		Wallet    walletSnapshotPayload    `json:"wallet"`
	}
	if err := json.Unmarshal(start.Response.Payload, &startPayload); err != nil {
		t.Fatalf("decode crafting.start payload: %v", err)
	}
	if len(startPayload.Crafting.ActiveJobs) != 1 {
		t.Fatalf("active jobs after start = %+v, want one running job", startPayload.Crafting.ActiveJobs)
	}
	jobID := startPayload.Crafting.ActiveJobs[0].JobID
	if startPayload.Crafting.ActiveJobs[0].RecipeID != "refined_alloy_batch" ||
		startPayload.Crafting.ActiveJobs[0].State != "running" ||
		startPayload.Crafting.ActiveJobs[0].CompletesAt <= startPayload.Crafting.ActiveJobs[0].StartedAt {
		t.Fatalf("started craft job = %+v, want server-timed refined alloy job", startPayload.Crafting.ActiveJobs[0])
	}
	if startPayload.Wallet.Credits != starterWalletCredits-100 {
		t.Fatalf("wallet credits after craft start = %d, want %d", startPayload.Wallet.Credits, starterWalletCredits-100)
	}
	if got := inventoryStackQuantity(startPayload.Inventory, "iron_ore", economy.LocationKindCraftingReserved.String()); got != 20 {
		t.Fatalf("reserved iron_ore = %d, want 20", got)
	}
	if got := inventoryStackQuantity(startPayload.Inventory, "carbon_shards", economy.LocationKindCraftingReserved.String()); got != 5 {
		t.Fatalf("reserved carbon_shards = %d, want 5", got)
	}

	duplicateStart := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-start","op":"crafting.start","payload":{"recipe_id":"refined_alloy_batch"},"client_seq":3,"v":1}`),
	)
	if duplicateStart.HasError || !bytes.Equal(start.Response.Payload, duplicateStart.Response.Payload) {
		t.Fatalf("duplicate crafting.start = %+v, want cached identical success", duplicateStart)
	}

	early := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-complete-early","op":"crafting.complete","payload":{"job_id":"`+jobID+`"},"client_seq":4,"v":1}`),
	)
	if !early.HasError || early.Error.Error.Code != foundation.CodeCooldown {
		t.Fatalf("early crafting.complete = %+v, want cooldown", early)
	}

	clock.Advance(5*time.Minute + time.Millisecond)
	complete := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-complete","op":"crafting.complete","payload":{"job_id":"`+jobID+`"},"client_seq":5,"v":1}`),
	)
	if complete.HasError {
		t.Fatalf("crafting.complete response error = %+v, want success", complete.Error)
	}
	assertCraftingPayloadOmitsInternals(t, "crafting.complete response", complete.Response.Payload)
	var completePayload struct {
		Crafting    craftingSnapshotPayload    `json:"crafting"`
		Inventory   inventorySnapshotPayload   `json:"inventory"`
		Wallet      walletSnapshotPayload      `json:"wallet"`
		Progression progressionSnapshotPayload `json:"progression"`
	}
	if err := json.Unmarshal(complete.Response.Payload, &completePayload); err != nil {
		t.Fatalf("decode crafting.complete payload: %v", err)
	}
	if len(completePayload.Crafting.ActiveJobs) != 0 {
		t.Fatalf("active jobs after complete = %+v, want none", completePayload.Crafting.ActiveJobs)
	}
	if got := inventoryStackQuantity(completePayload.Inventory, "refined_alloy", economy.LocationKindAccountInventory.String()); got != 5 {
		t.Fatalf("refined_alloy output = %d, want 5", got)
	}
	if completePayload.Wallet.Credits != starterWalletCredits-100 {
		t.Fatalf("wallet credits after complete = %d, want unchanged %d", completePayload.Wallet.Credits, starterWalletCredits-100)
	}
	if completePayload.Progression.MainXP != 40 {
		t.Fatalf("main XP after craft complete = %d, want 40", completePayload.Progression.MainXP)
	}

	duplicateComplete := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-crafting-complete-duplicate","op":"crafting.complete","payload":{"job_id":"`+jobID+`"},"client_seq":6,"v":1}`),
	)
	if duplicateComplete.HasError {
		t.Fatalf("duplicate crafting.complete response error = %+v, want success", duplicateComplete.Error)
	}
	var duplicatePayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
	}
	if err := json.Unmarshal(duplicateComplete.Response.Payload, &duplicatePayload); err != nil {
		t.Fatalf("decode duplicate crafting.complete payload: %v", err)
	}
	if got := inventoryStackQuantity(duplicatePayload.Inventory, "refined_alloy", economy.LocationKindAccountInventory.String()); got != 5 {
		t.Fatalf("duplicate refined_alloy output = %d, want unchanged 5", got)
	}
}

func TestLoadoutEquipAndUnequipMutateServerOwnedInventory(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-loadout-inventory","op":"inventory.snapshot","payload":{},"client_seq":1,"v":1}`)
	inventoryResponse := readResponse(t, conn)
	if !inventoryResponse.OK {
		t.Fatalf("inventory response = %+v, want success", inventoryResponse)
	}
	var inventoryPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
	}
	if err := json.Unmarshal(inventoryResponse.Payload, &inventoryPayload); err != nil {
		t.Fatalf("decode inventory snapshot: %v", err)
	}
	laserID := requireInventoryInstance(t, inventoryPayload.Inventory, "laser_alpha_t1", economy.LocationKindAccountInventory.String())
	shieldID := requireInventoryInstance(t, inventoryPayload.Inventory, "shield_generator_t1", economy.LocationKindAccountInventory.String())

	equipRequest := `{"request_id":"request-loadout-equip-laser","op":"loadout.equip_module","payload":{"slot_id":"offensive_1","item_instance_id":"` + laserID + `"},"client_seq":2,"v":1}`
	writeText(t, conn, equipRequest)
	equipRaw := readRawText(t, conn)
	equipResponse := decodeRawResponse(t, equipRaw)
	if !equipResponse.OK {
		t.Fatalf("equip response = %+v, want success", equipResponse)
	}
	var equipPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
		Loadout   loadoutSnapshotPayload   `json:"loadout"`
	}
	if err := json.Unmarshal(equipResponse.Payload, &equipPayload); err != nil {
		t.Fatalf("decode equip payload: %v", err)
	}
	offensive := requireLoadoutSlot(t, equipPayload.Loadout, "offensive_1")
	if offensive.ItemInstanceID != laserID || offensive.ModuleItemID != "laser_alpha_t1" || offensive.Durability != 100 {
		t.Fatalf("offensive slot = %+v, want equipped laser %s", offensive, laserID)
	}
	requireInventoryInstanceLocation(t, equipPayload.Inventory, laserID, economy.LocationKindShipEquipped.String())
	drainEventTypes(t, conn, realtime.EventInventorySnapshot, realtime.EventLoadoutSnapshot, realtime.EventStatsUpdated)
	writeText(t, conn, equipRequest)
	duplicateEquipRaw := readRawText(t, conn)
	if !bytes.Equal(equipRaw, duplicateEquipRaw) {
		t.Fatalf("duplicate equip response changed:\nfirst=%s\nsecond=%s", equipRaw, duplicateEquipRaw)
	}

	writeText(t, conn, `{"request_id":"request-loadout-spoof","op":"loadout.equip_module","payload":{"slot_id":"offensive_1","item_instance_id":"`+laserID+`","player_id":"spoof"},"client_seq":3,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed loadout error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, conn, `{"request_id":"request-loadout-wrong-slot","op":"loadout.equip_module","payload":{"slot_id":"offensive_1","item_instance_id":"`+shieldID+`"},"client_seq":4,"v":1}`)
	wrongSlot := readError(t, conn)
	if wrongSlot.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("wrong slot error = %+v, want %s", wrongSlot.Error, foundation.CodeInvalidPayload)
	}

	unequipRequest := `{"request_id":"request-loadout-unequip-laser","op":"loadout.unequip_module","payload":{"slot_id":"offensive_1"},"client_seq":5,"v":1}`
	writeText(t, conn, unequipRequest)
	unequipRaw := readRawText(t, conn)
	unequipResponse := decodeRawResponse(t, unequipRaw)
	if !unequipResponse.OK {
		t.Fatalf("unequip response = %+v, want success", unequipResponse)
	}
	var unequipPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
		Loadout   loadoutSnapshotPayload   `json:"loadout"`
	}
	if err := json.Unmarshal(unequipResponse.Payload, &unequipPayload); err != nil {
		t.Fatalf("decode unequip payload: %v", err)
	}
	offensive = requireLoadoutSlot(t, unequipPayload.Loadout, "offensive_1")
	if offensive.ItemInstanceID != "" || offensive.ModuleItemID != "" {
		t.Fatalf("offensive slot after unequip = %+v, want empty", offensive)
	}
	requireInventoryInstanceLocation(t, unequipPayload.Inventory, laserID, economy.LocationKindAccountInventory.String())
	drainEventTypes(t, conn, realtime.EventInventorySnapshot, realtime.EventLoadoutSnapshot, realtime.EventStatsUpdated)
	writeText(t, conn, unequipRequest)
	duplicateUnequipRaw := readRawText(t, conn)
	if !bytes.Equal(unequipRaw, duplicateUnequipRaw) {
		t.Fatalf("duplicate unequip response changed:\nfirst=%s\nsecond=%s", unequipRaw, duplicateUnequipRaw)
	}
}
func TestHangarActivateShipUsesServerOwnedHangarState(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-hangar-snapshot","op":"hangar.snapshot","payload":{},"client_seq":1,"v":1}`)
	snapshotResponse := readResponse(t, conn)
	if !snapshotResponse.OK {
		t.Fatalf("hangar snapshot response = %+v, want success", snapshotResponse)
	}
	var snapshotPayload struct {
		Hangar hangarSnapshotPayload `json:"hangar"`
	}
	if err := json.Unmarshal(snapshotResponse.Payload, &snapshotPayload); err != nil {
		t.Fatalf("decode hangar snapshot: %v", err)
	}
	if snapshotPayload.Hangar.ActiveShipID != starterShipID.String() || len(snapshotPayload.Hangar.Ships) != 1 {
		t.Fatalf("hangar snapshot = %+v, want active starter row", snapshotPayload.Hangar)
	}
	starter := snapshotPayload.Hangar.Ships[0]
	if starter.ShipID != starterShipID.String() || !starter.Active || starter.CargoCapacity <= 0 || starter.SlotUtility != 1 {
		t.Fatalf("starter hangar row = %+v, want server catalog stats and active flag", starter)
	}

	activateRequest := `{"request_id":"request-hangar-activate-starter","op":"hangar.activate_ship","payload":{"ship_id":"` + starterShipID.String() + `"},"client_seq":2,"v":1}`
	writeText(t, conn, activateRequest)
	activateRaw := readRawText(t, conn)
	activateResponse := decodeRawResponse(t, activateRaw)
	if !activateResponse.OK {
		t.Fatalf("hangar activate response = %+v, want success", activateResponse)
	}
	var activatePayload struct {
		Hangar  hangarSnapshotPayload  `json:"hangar"`
		Ship    shipSnapshotPayload    `json:"ship"`
		Stats   statSnapshotPayload    `json:"stats"`
		Cargo   cargoSnapshotPayload   `json:"cargo"`
		Loadout loadoutSnapshotPayload `json:"loadout"`
	}
	if err := json.Unmarshal(activateResponse.Payload, &activatePayload); err != nil {
		t.Fatalf("decode hangar activate payload: %v", err)
	}
	if activatePayload.Hangar.ActiveShipID != starterShipID.String() ||
		activatePayload.Ship.ActiveShipID != starterShipID.String() ||
		activatePayload.Loadout.ActiveShipID != starterShipID.String() {
		t.Fatalf("activate payload = %+v, want starter snapshots", activatePayload)
	}
	drainEventTypes(t, conn, realtime.EventHangarSnapshot, realtime.EventShipSnapshot, realtime.EventStatsUpdated, realtime.EventCargoSnapshot, realtime.EventLoadoutSnapshot)
	writeText(t, conn, activateRequest)
	duplicateActivateRaw := readRawText(t, conn)
	if !bytes.Equal(activateRaw, duplicateActivateRaw) {
		t.Fatalf("duplicate hangar activate response changed:\nfirst=%s\nsecond=%s", activateRaw, duplicateActivateRaw)
	}

	writeText(t, conn, `{"request_id":"request-hangar-spoof","op":"hangar.activate_ship","payload":{"ship_id":"`+starterShipID.String()+`","player_id":"spoof"},"client_seq":3,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed hangar error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, conn, `{"request_id":"request-hangar-unknown","op":"hangar.activate_ship","payload":{"ship_id":"missing_ship"},"client_seq":4,"v":1}`)
	unknown := readError(t, conn)
	if unknown.Error.Code != foundation.CodeNotFound {
		t.Fatalf("unknown hangar error = %+v, want %s", unknown.Error, foundation.CodeNotFound)
	}
}
func TestInventorySnapshotCarriesServerOwnedMarketListEligibility(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-market-buy-for-sell-eligibility","op":"market.buy","payload":{"listing_id":"`+seedMarketListingID.String()+`","quantity":1},"client_seq":1,"v":1}`)
	buyResponse := readResponse(t, conn)
	if !buyResponse.OK {
		t.Fatalf("market buy response = %+v, want success", buyResponse)
	}
	var buyPayload marketMutationPayload
	if err := json.Unmarshal(buyResponse.Payload, &buyPayload); err != nil {
		t.Fatalf("decode market buy: %v", err)
	}
	rawOre := requireInventoryStack(t, buyPayload.Inventory, "raw_ore", economy.LocationKindAccountInventory.String())
	if !rawOre.ListEligible || rawOre.LockedReason != "" {
		t.Fatalf("raw ore market eligibility = %+v, want eligible without locked reason", rawOre)
	}
	drainEventTypes(t, conn, realtime.EventMarketSaleCompleted, realtime.EventWalletSnapshot, realtime.EventInventorySnapshot)

	accountLocation, err := economy.NewItemLocation(economy.LocationKindAccountInventory, resolved.PlayerID.String())
	if err != nil {
		t.Fatalf("account location: %v", err)
	}
	blockedLocation, err := economy.NewItemLocation(economy.LocationKindCraftingReserved, "craft-reserved-sell-test")
	if err != nil {
		t.Fatalf("blocked location: %v", err)
	}
	unlistedDefinition := testStackableDefinition(t, "bound_scrap", "Bound Scrap", []economy.TradeFlag{economy.TradeFlagDroppable})

	gameServer.runtime.mu.Lock()
	marketDefinition, ok := gameServer.runtime.itemCatalog["raw_ore"]
	if !ok {
		gameServer.runtime.mu.Unlock()
		t.Fatal("raw_ore definition missing")
	}
	gameServer.runtime.itemCatalog[unlistedDefinition.ItemID] = unlistedDefinition
	gameServer.runtime.mu.Unlock()

	addTestInventoryStack(t, gameServer, resolved.PlayerID, unlistedDefinition, 2, accountLocation, "sell-eligibility-non-market")
	addTestInventoryStack(t, gameServer, resolved.PlayerID, marketDefinition, 2, blockedLocation, "sell-eligibility-blocked-location")

	writeText(t, conn, `{"request_id":"request-inventory-sell-eligibility","op":"inventory.snapshot","payload":{},"client_seq":2,"v":1}`)
	inventoryResponse := readResponse(t, conn)
	if !inventoryResponse.OK {
		t.Fatalf("inventory response = %+v, want success", inventoryResponse)
	}
	var inventoryPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
	}
	if err := json.Unmarshal(inventoryResponse.Payload, &inventoryPayload); err != nil {
		t.Fatalf("decode inventory snapshot: %v", err)
	}

	accountRawOre := requireInventoryStack(t, inventoryPayload.Inventory, "raw_ore", economy.LocationKindAccountInventory.String())
	if !accountRawOre.ListEligible || accountRawOre.LockedReason != "" {
		t.Fatalf("account raw ore eligibility = %+v, want eligible without locked reason", accountRawOre)
	}
	boundScrap := requireInventoryStack(t, inventoryPayload.Inventory, "bound_scrap", economy.LocationKindAccountInventory.String())
	if boundScrap.ListEligible || boundScrap.LockedReason != "Item cannot be listed" {
		t.Fatalf("bound scrap eligibility = %+v, want item cannot be listed", boundScrap)
	}
	reservedRawOre := requireInventoryStack(t, inventoryPayload.Inventory, "raw_ore", economy.LocationKindCraftingReserved.String())
	if reservedRawOre.ListEligible || reservedRawOre.LockedReason != "Move item to inventory first" {
		t.Fatalf("reserved raw ore eligibility = %+v, want move-to-inventory lock", reservedRawOre)
	}
}
func TestShopCatalogUsesServerOwnedGameCatalog(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-shop-catalog","op":"shop.catalog","payload":{},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("shop catalog response = %+v, want success", response)
	}
	assertNoEconomyLeak(t, "shop catalog", response.Payload)
	rawPayload := string(response.Payload)
	for _, forbidden := range []string{"listing-raw-ore-1", "server_recalculates", "server recalculates", "server-owned"} {
		if strings.Contains(rawPayload, forbidden) {
			t.Fatalf("shop catalog leaked %q in %s", forbidden, rawPayload)
		}
	}

	var payload shopCatalogResponsePayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode shop catalog: %v", err)
	}
	if payload.Shop.CatalogVersion != catalog.ContentRegistryVersion.String() {
		t.Fatalf("catalog version = %q, want %q", payload.Shop.CatalogVersion, catalog.ContentRegistryVersion)
	}
	categoryNames := make(map[string]string, len(payload.Shop.Categories))
	for _, category := range payload.Shop.Categories {
		categoryNames[category.CategoryID] = category.DisplayName
	}
	for id, displayName := range map[string]string{
		gamecontent.ShopCategoryShips:            "Ships",
		gamecontent.ShopCategoryWeapons:          "Weapons",
		gamecontent.ShopCategoryShieldGenerators: "Shield Generators",
		gamecontent.ShopCategoryScannerRadar:     "Scanner/Radar",
		gamecontent.ShopCategoryCargoUtility:     "Cargo/Utility",
		gamecontent.ShopCategoryResources:        "Resources",
	} {
		if categoryNames[id] != displayName {
			t.Fatalf("category %q = %q, want %q in %+v", id, categoryNames[id], displayName, payload.Shop.Categories)
		}
	}
	if len(payload.Shop.Products) < 6 {
		t.Fatalf("shop products = %+v, want broad seed coverage", payload.Shop.Products)
	}
	seenPrism := false
	for _, product := range payload.Shop.Products {
		if product.DisplayName == "" || product.DisplayName == product.ProductID || strings.Contains(product.DisplayName, "_") {
			t.Fatalf("shop product has raw display name: %+v", product)
		}
		if product.CategoryID == "" || product.ArtKey == "" || product.Price.Currency == "" {
			t.Fatalf("shop product missing display/category/price metadata: %+v", product)
		}
		if product.DisplayName == "Prism Lance I" {
			seenPrism = true
		}
	}
	if !seenPrism {
		t.Fatalf("shop products = %+v, missing Prism Lance I weapon product", payload.Shop.Products)
	}

	writeText(t, conn, `{"request_id":"request-shop-catalog-spoof","op":"shop.catalog","payload":{"stock_remaining":99},"client_seq":2,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed shop catalog error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}
}
func TestShopBuyProductDebitsWalletAndGrantsServerCatalogProduct(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	beforeCount := countInventoryInstances(gameServer.runtime.Inventory.InstanceItems(), "laser_alpha_t1")
	writeText(t, conn, `{"request_id":"request-shop-buy-laser","op":"shop.buy_product","payload":{"product_id":"product_module_laser_alpha_t1","quantity":1},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("shop buy response = %+v, want success", response)
	}
	assertNoEconomyLeak(t, "shop buy", response.Payload)
	var payload shopBuyProductResponsePayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode shop buy: %v", err)
	}
	if !payload.Accepted || payload.Product.ProductID != "product_module_laser_alpha_t1" || payload.Quantity != 1 || payload.ServerTotal != 450 {
		t.Fatalf("shop buy payload = %+v, want accepted laser at server price", payload)
	}
	if payload.Wallet.Credits != starterWalletCredits-450 {
		t.Fatalf("wallet credits = %d, want %d", payload.Wallet.Credits, starterWalletCredits-450)
	}
	if payload.Inventory == nil {
		t.Fatalf("shop buy inventory snapshot missing")
	}
	afterCount := countInventoryInstances(gameServer.runtime.Inventory.InstanceItems(), "laser_alpha_t1")
	if afterCount != beforeCount+1 {
		t.Fatalf("laser instances = %d, want %d after shop buy", afterCount, beforeCount+1)
	}
	if !inventorySnapshotHasInstance(*payload.Inventory, "laser_alpha_t1") {
		t.Fatalf("inventory snapshot missing laser_alpha_t1 instance: %+v", payload.Inventory.Instances)
	}

	writeText(t, conn, `{"request_id":"request-shop-buy-spoof","op":"shop.buy_product","payload":{"product_id":"product_module_laser_alpha_t1","quantity":1,"price":{"amount":1},"stock_remaining":99},"client_seq":2,"v":1}`)
	spoof := readErrorSkippingEvents(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed shop buy error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}
}
func TestShopBuyProductRejectsInsufficientFundsBeforeGrant(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	for index := 1; index <= 2; index++ {
		writeText(t, conn, fmt.Sprintf(`{"request_id":"request-shop-buy-laser-%d","op":"shop.buy_product","payload":{"product_id":"product_module_laser_alpha_t1","quantity":1},"client_seq":%d,"v":1}`, index, index))
		response := readResponseSkippingEvents(t, conn)
		if !response.OK {
			t.Fatalf("shop buy %d response = %+v, want success", index, response)
		}
	}
	beforeCount := countInventoryInstances(gameServer.runtime.Inventory.InstanceItems(), "laser_alpha_t1")
	writeText(t, conn, `{"request_id":"request-shop-buy-laser-no-funds","op":"shop.buy_product","payload":{"product_id":"product_module_laser_alpha_t1","quantity":1},"client_seq":3,"v":1}`)
	rejected := readErrorSkippingEvents(t, conn)
	if rejected.Error.Code != foundation.CodeNotEnoughFunds {
		t.Fatalf("insufficient funds error = %+v, want %s", rejected.Error, foundation.CodeNotEnoughFunds)
	}
	afterCount := countInventoryInstances(gameServer.runtime.Inventory.InstanceItems(), "laser_alpha_t1")
	if afterCount != beforeCount {
		t.Fatalf("laser instances after failed buy = %d, want unchanged %d", afterCount, beforeCount)
	}
	if credits := runtimeWalletCredits(t, gameServer.runtime); credits != 300 {
		t.Fatalf("wallet credits after failed buy = %d, want 300", credits)
	}
}

func inventoryStackQuantity(inventory inventorySnapshotPayload, itemID string, location string) int64 {
	for _, stack := range inventory.Stackable {
		if stack.ItemID == itemID && stack.Location == location {
			return stack.Quantity
		}
	}
	return 0
}

func assertCraftingPayloadOmitsInternals(t *testing.T, label string, payload []byte) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range []string{
		"account_id",
		"player_id",
		"session_id",
		"reference_id",
		"reservation_id",
		"source_location",
		"wallet_debit",
		"reservation_commit",
		"ledger_id",
		"materials",
		"wallet_amount",
		"balance_after",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}
