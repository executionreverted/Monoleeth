package server

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/intel"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestIntelShareUsesServerOwnedKnownPlanetAndQueuesReceiverIntel(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	sender := createResolvedRuntimeSession(t, gameServer, "intel-share-sender@example.com", "Intel Sender")
	receiver := createResolvedRuntimeSession(t, gameServer, "intel-share-receiver@example.com", "Intel Receiver")
	planetID := foundation.PlanetID("planet-intel-share")
	seedKnownClaimPlanetForTest(t, gameServer, sender.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1200, Y: 1300}, 2)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(sender.SessionID.String()),
		[]byte(`{"request_id":"request-intel-share","op":"intel.share","payload":{"planet_id":"`+planetID.String()+`","to_player_id":"`+receiver.PlayerID.String()+`"},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("intel.share response error = %+v, want success", response.Error)
	}
	assertIntelPayloadSafe(t, "intel.share response", response.Response.Payload)
	assertIntelPayloadOmitsCoordinates(t, "intel.share response", response.Response.Payload)
	var payload struct {
		Share intelSharePayload `json:"share"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode intel.share payload: %v", err)
	}
	if payload.Share.PlanetID != planetID.String() || payload.Share.ToPlayerID != receiver.PlayerID.String() || !payload.Share.Shared || !payload.Share.ReceiverUpdated {
		t.Fatalf("share payload = %+v, want receiver update for %s", payload.Share, planetID)
	}
	stored, ok, err := gameServer.runtime.Discovery.PlayerPlanetIntel(receiver.PlayerID, planetID)
	if err != nil || !ok {
		t.Fatalf("receiver PlayerPlanetIntel ok=%v err=%v, want stored intel", ok, err)
	}
	if stored.SourceType != discovery.IntelSourceShareReceived || stored.Coordinates.X != 1200 || stored.Coordinates.Y != 1300 {
		t.Fatalf("receiver intel = %+v, want share source and server-owned coordinates", stored)
	}

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(sender.SessionID, realtime.OperationIntelShare, sender.PlayerID)
	if err != nil {
		t.Fatalf("post intel.share events: %v", err)
	}
	if _, leaked := eventsBySession[sender.SessionID]; leaked {
		t.Fatalf("intel.share queued actor events = %+v, want receiver-only refresh", eventsBySession[sender.SessionID])
	}
	receiverEvents := eventsBySession[receiver.SessionID]
	requireEventTypeForTest(t, receiverEvents, realtime.EventKnownPlanets)
	for _, event := range receiverEvents {
		assertIntelPayloadSafe(t, string(event.Type)+" event", event.Payload)
	}
}

func TestIntelShareRejectsSpoofedServerOwnedFieldsBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	sender := createResolvedRuntimeSession(t, gameServer, "intel-share-spoof@example.com", "Intel Spoof")
	receiver := createResolvedRuntimeSession(t, gameServer, "intel-share-spoof-receiver@example.com", "Intel Spoof Receiver")
	planetID := foundation.PlanetID("planet-intel-share-spoof")
	seedKnownClaimPlanetForTest(t, gameServer, sender.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1200, Y: 1300}, 2)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(sender.SessionID.String()),
		[]byte(`{"request_id":"request-intel-share-spoof","op":"intel.share","payload":{"planet_id":"`+planetID.String()+`","to_player_id":"`+receiver.PlayerID.String()+`","coordinates":{"x":1,"y":2}},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed intel.share response = %+v, want invalid payload", response)
	}
	if _, ok, err := gameServer.runtime.Discovery.PlayerPlanetIntel(receiver.PlayerID, planetID); err != nil || ok {
		t.Fatalf("receiver intel after spoof ok=%v err=%v, want no mutation", ok, err)
	}
}

func TestIntelShareRejectsUnsafeSourceStateBeforeReceiverMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	sender := createResolvedRuntimeSession(t, gameServer, "intel-share-stale@example.com", "Intel Share Stale")
	receiver := createResolvedRuntimeSession(t, gameServer, "intel-share-stale-receiver@example.com", "Intel Share Stale Receiver")
	planetID := foundation.PlanetID("planet-intel-share-stale")
	if _, _, err := gameServer.runtime.Discovery.UpsertPlayerPlanetIntel(discovery.PlayerPlanetIntel{
		PlayerID:        sender.PlayerID,
		PlanetID:        planetID,
		WorldID:         gameServer.runtime.worldID,
		ZoneID:          gameServer.runtime.zoneID,
		Coordinates:     world.Vec2{X: 1200, Y: 1300},
		State:           discovery.IntelStateStale,
		Confidence:      30,
		LastSeenAt:      gameServer.runtime.clock.Now().UTC(),
		SourceType:      discovery.IntelSourceScanSuccess,
		SourceReference: "scan:stale-share",
	}); err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel(stale source): %v", err)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(sender.SessionID.String()),
		[]byte(`{"request_id":"request-intel-share-stale","op":"intel.share","payload":{"planet_id":"`+planetID.String()+`","to_player_id":"`+receiver.PlayerID.String()+`"},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("stale intel.share response = %+v, want safe not found", response)
	}
	if _, ok, err := gameServer.runtime.Discovery.PlayerPlanetIntel(receiver.PlayerID, planetID); err != nil || ok {
		t.Fatalf("receiver intel after stale share ok=%v err=%v, want no mutation", ok, err)
	}
	gameServer.runtime.mu.Lock()
	receiverEvents := len(gameServer.runtime.queuedEvents[receiver.SessionID])
	gameServer.runtime.mu.Unlock()
	if receiverEvents != 0 {
		t.Fatalf("stale share receiver queued events = %d, want none", receiverEvents)
	}
}

func TestIntelShareRejectsUnknownReceiverBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	sender := createResolvedRuntimeSession(t, gameServer, "intel-share-unknown-receiver@example.com", "Intel Share Unknown")
	planetID := foundation.PlanetID("planet-intel-share-unknown-receiver")
	unknownReceiverID := foundation.PlayerID("player_unknown_receiver")
	seedKnownClaimPlanetForTest(t, gameServer, sender.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1200, Y: 1300}, 2)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(sender.SessionID.String()),
		[]byte(`{"request_id":"request-intel-share-unknown-receiver","op":"intel.share","payload":{"planet_id":"`+planetID.String()+`","to_player_id":"`+unknownReceiverID.String()+`"},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("unknown receiver intel.share response = %+v, want not found", response)
	}
	if _, ok, err := gameServer.runtime.Discovery.PlayerPlanetIntel(unknownReceiverID, planetID); err != nil || ok {
		t.Fatalf("unknown receiver intel after rejected share ok=%v err=%v, want no mutation", ok, err)
	}
	if _, ok, err := gameServer.runtime.Intel.PlayerPlanetIntel(unknownReceiverID, planetID); err != nil || ok {
		t.Fatalf("unknown receiver runtime intel after rejected share ok=%v err=%v, want no mutation", ok, err)
	}
}

func TestCoordinateItemCreateAndUseConsumeOnceAndRefreshDiscovery(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "coordinate-owner@example.com", "Coordinate Owner")
	planetID := foundation.PlanetID("planet-coordinate-use")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1500, Y: 1600}, 3)

	create := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-create","op":"intel.coordinate_item.create","payload":{"planet_id":"`+planetID.String()+`"},"client_seq":1,"v":1}`),
	)
	if create.HasError {
		t.Fatalf("coordinate create response error = %+v, want success", create.Error)
	}
	assertIntelPayloadSafe(t, "coordinate create response", create.Response.Payload)
	assertIntelPayloadOmitsCoordinates(t, "coordinate create response", create.Response.Payload)
	var createPayload struct {
		CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
		Inventory      inventorySnapshotPayload   `json:"inventory"`
		Created        bool                       `json:"created"`
		Duplicate      bool                       `json:"duplicate"`
	}
	if err := json.Unmarshal(create.Response.Payload, &createPayload); err != nil {
		t.Fatalf("decode coordinate create payload: %v", err)
	}
	if !createPayload.Created || createPayload.CoordinateItem.ItemInstanceID == "" || createPayload.CoordinateItem.Used {
		t.Fatalf("create payload = %+v, want fresh server-owned coordinate item", createPayload)
	}
	if !inventorySnapshotHasInstanceID(createPayload.Inventory, createPayload.CoordinateItem.ItemInstanceID, coordinateScrollItemID.String(), economy.LocationKindAccountInventory.String()) {
		t.Fatalf("create inventory = %+v, want coordinate scroll instance %s", createPayload.Inventory, createPayload.CoordinateItem.ItemInstanceID)
	}
	assertCoordinateItemLedgerCount(t, gameServer, owner.PlayerID, createPayload.CoordinateItem.ItemInstanceID, economy.LedgerActionIncrease, intelCoordinateItemCreateLedgerReason, 1)
	createEventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationIntelCoordinateCreate, owner.PlayerID)
	if err != nil {
		t.Fatalf("post coordinate create events: %v", err)
	}
	requireEventTypeForTest(t, createEventsBySession[owner.SessionID], realtime.EventInventorySnapshot)

	duplicateCreate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-create","op":"intel.coordinate_item.create","payload":{"planet_id":"`+planetID.String()+`"},"client_seq":1,"v":1}`),
	)
	if duplicateCreate.HasError {
		t.Fatalf("duplicate coordinate create response error = %+v, want cached success", duplicateCreate.Error)
	}
	if got := countInventoryInstances(gameServer.runtime.Inventory.InstanceItems(), coordinateScrollItemID.String()); got != 1 {
		t.Fatalf("coordinate scroll inventory instances = %d, want one after duplicate create", got)
	}
	assertCoordinateItemLedgerCount(t, gameServer, owner.PlayerID, createPayload.CoordinateItem.ItemInstanceID, economy.LedgerActionIncrease, intelCoordinateItemCreateLedgerReason, 1)

	use := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-use","op":"intel.coordinate_item.use","payload":{"item_instance_id":"`+createPayload.CoordinateItem.ItemInstanceID+`"},"client_seq":2,"v":1}`),
	)
	if use.HasError {
		t.Fatalf("coordinate use response error = %+v, want success", use.Error)
	}
	assertIntelPayloadSafe(t, "coordinate use response", use.Response.Payload)
	var usePayload struct {
		CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
		KnownPlanets   knownPlanetsPayload        `json:"known_planets"`
		PlanetDetail   planetDetailPayload        `json:"planet_detail"`
		Inventory      inventorySnapshotPayload   `json:"inventory"`
		Used           bool                       `json:"used"`
	}
	if err := json.Unmarshal(use.Response.Payload, &usePayload); err != nil {
		t.Fatalf("decode coordinate use payload: %v", err)
	}
	if !usePayload.Used || !usePayload.CoordinateItem.Used || usePayload.PlanetDetail.PlanetID != planetID.String() || len(usePayload.KnownPlanets.Planets) != 1 {
		t.Fatalf("use payload = %+v, want consumed item and refreshed discovery", usePayload)
	}
	if inventorySnapshotHasInstanceID(usePayload.Inventory, createPayload.CoordinateItem.ItemInstanceID, coordinateScrollItemID.String(), economy.LocationKindAccountInventory.String()) {
		t.Fatalf("use inventory = %+v, want coordinate scroll consumed", usePayload.Inventory)
	}
	assertCoordinateItemLedgerCount(t, gameServer, owner.PlayerID, createPayload.CoordinateItem.ItemInstanceID, economy.LedgerActionDecrease, intelCoordinateItemUseLedgerReason, 1)

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationIntelCoordinateUse, owner.PlayerID)
	if err != nil {
		t.Fatalf("post coordinate use events: %v", err)
	}
	requireEventTypeForTest(t, eventsBySession[owner.SessionID], realtime.EventKnownPlanets)
	requireEventTypeForTest(t, eventsBySession[owner.SessionID], realtime.EventPlanetDetail)
	requireEventTypeForTest(t, eventsBySession[owner.SessionID], realtime.EventInventorySnapshot)

	secondUse := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-use-second","op":"intel.coordinate_item.use","payload":{"item_instance_id":"`+createPayload.CoordinateItem.ItemInstanceID+`"},"client_seq":3,"v":1}`),
	)
	if !secondUse.HasError || secondUse.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("second coordinate use response = %+v, want forbidden", secondUse)
	}
}

func TestCoordinateItemUseDuplicateReplayBypassesTransportCache(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "coordinate-use-replay@example.com", "Coordinate Use Replay")
	planetID := foundation.PlanetID("planet-coordinate-use-replay")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1500, Y: 1600}, 3)

	createPayload := createCoordinateItemForTest(t, gameServer, owner, planetID, "request-coordinate-use-replay-create")
	request := coordinateItemUseRequestForTest("request-coordinate-use-replay", createPayload.CoordinateItem.ItemInstanceID, 2)
	ctx := commandContextForResolvedSessionForTest(gameServer, owner)

	firstRaw, err := gameServer.runtime.handleIntelCoordinateItemUse(ctx, request)
	if err != nil {
		t.Fatalf("first direct coordinate use error = %v, want nil", err)
	}
	var first struct {
		Duplicate bool `json:"duplicate"`
	}
	if err := json.Unmarshal(firstRaw, &first); err != nil {
		t.Fatalf("decode first direct coordinate use payload: %v", err)
	}
	if first.Duplicate {
		t.Fatalf("first direct coordinate use duplicate = true, want false")
	}

	replayRaw, err := gameServer.runtime.handleIntelCoordinateItemUse(ctx, request)
	if err != nil {
		t.Fatalf("replayed direct coordinate use error = %v, want cached domain success", err)
	}
	var replay struct {
		CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
		Inventory      inventorySnapshotPayload   `json:"inventory"`
		Duplicate      bool                       `json:"duplicate"`
	}
	if err := json.Unmarshal(replayRaw, &replay); err != nil {
		t.Fatalf("decode replay coordinate use payload: %v", err)
	}
	if !replay.Duplicate || !replay.CoordinateItem.Used {
		t.Fatalf("replay payload = %+v, want duplicate used coordinate item", replay)
	}
	if inventorySnapshotHasInstanceID(replay.Inventory, createPayload.CoordinateItem.ItemInstanceID, coordinateScrollItemID.String(), economy.LocationKindAccountInventory.String()) {
		t.Fatalf("replay inventory = %+v, want coordinate scroll still consumed", replay.Inventory)
	}
	assertCoordinateItemLedgerCount(t, gameServer, owner.PlayerID, createPayload.CoordinateItem.ItemInstanceID, economy.LedgerActionDecrease, intelCoordinateItemUseLedgerReason, 1)
}

func TestCoordinateItemUseRestoresInventoryAfterPostConsumeFailureAndRetryCleansRepair(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "coordinate-use-compensation@example.com", "Coordinate Use Compensation")
	planetID := foundation.PlanetID("planet-coordinate-use-compensation")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1500, Y: 1600}, 3)
	createPayload := createCoordinateItemForTest(t, gameServer, owner, planetID, "request-coordinate-use-compensation-create")
	request := coordinateItemUseRequestForTest("request-coordinate-use-compensation", createPayload.CoordinateItem.ItemInstanceID, 2)
	ctx := commandContextForResolvedSessionForTest(gameServer, owner)

	forcedErr := errors.New("forced coordinate use failure")
	previousHook := coordinateItemUseInterleaveTestHook
	defer func() { coordinateItemUseInterleaveTestHook = previousHook }()
	var hookRan bool
	coordinateItemUseInterleaveTestHook = func(stage coordinateItemUseInterleaveStage, _ *Runtime, playerID foundation.PlayerID, itemID foundation.ItemID) error {
		if stage != coordinateItemUseAfterInventoryConsume || hookRan {
			return nil
		}
		hookRan = true
		if playerID != owner.PlayerID || itemID.String() != createPayload.CoordinateItem.ItemInstanceID {
			t.Fatalf("hook player/item = %s/%s, want %s/%s", playerID, itemID, owner.PlayerID, createPayload.CoordinateItem.ItemInstanceID)
		}
		return forcedErr
	}

	_, err := gameServer.runtime.handleIntelCoordinateItemUse(ctx, request)
	if !errors.Is(err, forcedErr) {
		t.Fatalf("forced coordinate use error = %v, want forcedErr", err)
	}
	if !hookRan {
		t.Fatalf("coordinate use interleave hook did not run")
	}
	itemID := foundation.ItemID(createPayload.CoordinateItem.ItemInstanceID)
	item, ok, lookupErr := gameServer.runtime.Intel.CoordinateItem(itemID)
	if lookupErr != nil || !ok {
		t.Fatalf("CoordinateItem after failed use ok=%v err=%v, want item", ok, lookupErr)
	}
	if item.UsedAt != nil {
		t.Fatalf("coordinate item used_at after failed use = %v, want unconsumed", item.UsedAt)
	}
	if !inventorySnapshotHasInstanceID(gameServer.runtime.inventorySnapshotForPlayer(owner.PlayerID), createPayload.CoordinateItem.ItemInstanceID, coordinateScrollItemID.String(), economy.LocationKindAccountInventory.String()) {
		t.Fatalf("inventory after failed coordinate use missing restored scroll %s", createPayload.CoordinateItem.ItemInstanceID)
	}
	assertCoordinateItemLedgerCount(t, gameServer, owner.PlayerID, createPayload.CoordinateItem.ItemInstanceID, economy.LedgerActionDecrease, intelCoordinateItemUseLedgerReason, 1)
	assertCoordinateItemLedgerCount(t, gameServer, owner.PlayerID, createPayload.CoordinateItem.ItemInstanceID, economy.LedgerActionIncrease, intelCoordinateItemUseRepairReason, 1)

	retryRaw, err := gameServer.runtime.handleIntelCoordinateItemUse(ctx, request)
	if err != nil {
		t.Fatalf("retry coordinate use error = %v, want repaired success", err)
	}
	var retry struct {
		CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
		Inventory      inventorySnapshotPayload   `json:"inventory"`
	}
	if err := json.Unmarshal(retryRaw, &retry); err != nil {
		t.Fatalf("decode retry coordinate use payload: %v", err)
	}
	if !retry.CoordinateItem.Used {
		t.Fatalf("retry coordinate item = %+v, want used", retry.CoordinateItem)
	}
	if inventorySnapshotHasInstanceID(retry.Inventory, createPayload.CoordinateItem.ItemInstanceID, coordinateScrollItemID.String(), economy.LocationKindAccountInventory.String()) {
		t.Fatalf("retry inventory = %+v, want restored scroll cleaned up", retry.Inventory)
	}
	assertCoordinateItemLedgerCount(t, gameServer, owner.PlayerID, createPayload.CoordinateItem.ItemInstanceID, economy.LedgerActionDecrease, intelCoordinateItemUseLedgerReason, 1)
	assertCoordinateItemLedgerCount(t, gameServer, owner.PlayerID, createPayload.CoordinateItem.ItemInstanceID, economy.LedgerActionDecrease, intelCoordinateItemUseRepairReason, 1)
}

func TestCoordinateItemUseGatewayRetryAfterPostConsumeFailureReexecutes(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "coordinate-use-gateway-retry@example.com", "Coordinate Gateway Retry")
	planetID := foundation.PlanetID("planet-coordinate-use-gateway-retry")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1500, Y: 1600}, 3)
	createPayload := createCoordinateItemForTest(t, gameServer, owner, planetID, "request-coordinate-use-gateway-retry-create")
	request := []byte(`{"request_id":"request-coordinate-use-gateway-retry","op":"intel.coordinate_item.use","payload":{"item_instance_id":"` + createPayload.CoordinateItem.ItemInstanceID + `"},"client_seq":2,"v":1}`)

	forcedErr := errors.New("forced coordinate gateway retry failure")
	previousHook := coordinateItemUseInterleaveTestHook
	defer func() { coordinateItemUseInterleaveTestHook = previousHook }()
	var hookCalls int
	coordinateItemUseInterleaveTestHook = func(stage coordinateItemUseInterleaveStage, _ *Runtime, playerID foundation.PlayerID, itemID foundation.ItemID) error {
		if stage != coordinateItemUseAfterInventoryConsume {
			return nil
		}
		hookCalls++
		if playerID != owner.PlayerID || itemID.String() != createPayload.CoordinateItem.ItemInstanceID {
			t.Fatalf("hook player/item = %s/%s, want %s/%s", playerID, itemID, owner.PlayerID, createPayload.CoordinateItem.ItemInstanceID)
		}
		if hookCalls == 1 {
			return forcedErr
		}
		return nil
	}

	first := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(owner.SessionID.String()), request)
	if !first.HasError || first.Error.Error.Code != foundation.CodeInternal || !first.Error.Error.Retryable {
		t.Fatalf("first gateway coordinate use response = %+v, want retryable internal error", first)
	}
	if !inventorySnapshotHasInstanceID(gameServer.runtime.inventorySnapshotForPlayer(owner.PlayerID), createPayload.CoordinateItem.ItemInstanceID, coordinateScrollItemID.String(), economy.LocationKindAccountInventory.String()) {
		t.Fatalf("inventory after first gateway failure missing restored scroll %s", createPayload.CoordinateItem.ItemInstanceID)
	}

	retry := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(owner.SessionID.String()), request)
	if retry.HasError {
		t.Fatalf("retry gateway coordinate use response error = %+v, want success after re-execution", retry.Error)
	}
	var payload struct {
		CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
		Inventory      inventorySnapshotPayload   `json:"inventory"`
		Duplicate      bool                       `json:"duplicate"`
	}
	if err := json.Unmarshal(retry.Response.Payload, &payload); err != nil {
		t.Fatalf("decode retry gateway coordinate use payload: %v", err)
	}
	if !payload.CoordinateItem.Used || payload.Duplicate {
		t.Fatalf("retry payload = %+v, want fresh successful use", payload)
	}
	if inventorySnapshotHasInstanceID(payload.Inventory, createPayload.CoordinateItem.ItemInstanceID, coordinateScrollItemID.String(), economy.LocationKindAccountInventory.String()) {
		t.Fatalf("retry inventory = %+v, want coordinate scroll consumed", payload.Inventory)
	}
	if hookCalls != 2 {
		t.Fatalf("coordinate use hook calls = %d, want first failure plus retry execution", hookCalls)
	}
}

func TestCoordinateItemMarketPurchaseTransfersUseAuthority(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	seller := createResolvedRuntimeSession(t, gameServer, "coordinate-market-seller@example.com", "Coordinate Seller")
	buyer := createResolvedRuntimeSession(t, gameServer, "coordinate-market-buyer@example.com", "Coordinate Buyer")
	planetID := foundation.PlanetID("planet-coordinate-market-purchase")
	seedKnownClaimPlanetForTest(t, gameServer, seller.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1500, Y: 1600}, 3)

	create := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(seller.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-market-create","op":"intel.coordinate_item.create","payload":{"planet_id":"`+planetID.String()+`"},"client_seq":1,"v":1}`),
	)
	if create.HasError {
		t.Fatalf("coordinate create response error = %+v, want success", create.Error)
	}
	var createPayload struct {
		CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
	}
	if err := json.Unmarshal(create.Response.Payload, &createPayload); err != nil {
		t.Fatalf("decode coordinate create payload: %v", err)
	}

	list := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(seller.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-market-list","op":"market.create_listing","payload":{"item_id":"planet_coordinate_scroll","item_instance_id":"`+createPayload.CoordinateItem.ItemInstanceID+`","quantity":1,"unit_price":75},"client_seq":2,"v":1}`),
	)
	if list.HasError {
		t.Fatalf("market create listing response error = %+v, want success", list.Error)
	}
	var listPayload marketMutationPayload
	if err := json.Unmarshal(list.Response.Payload, &listPayload); err != nil {
		t.Fatalf("decode market create listing payload: %v", err)
	}

	buyRequest := `{"request_id":"request-coordinate-market-buy","op":"market.buy","payload":{"listing_id":"` + listPayload.Listing.ListingID + `","quantity":1},"client_seq":1,"v":1}`
	buy := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(buyer.SessionID.String()), []byte(buyRequest))
	if buy.HasError {
		t.Fatalf("market buy response error = %+v, want success", buy.Error)
	}
	var buyPayload marketMutationPayload
	if err := json.Unmarshal(buy.Response.Payload, &buyPayload); err != nil {
		t.Fatalf("decode market buy payload: %v", err)
	}
	if !inventorySnapshotHasInstanceID(buyPayload.Inventory, createPayload.CoordinateItem.ItemInstanceID, coordinateScrollItemID.String(), economy.LocationKindAccountInventory.String()) {
		t.Fatalf("buyer inventory = %+v, want bought coordinate scroll instance", buyPayload.Inventory)
	}
	duplicateBuy := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(buyer.SessionID.String()), []byte(buyRequest))
	if duplicateBuy.HasError {
		t.Fatalf("duplicate market buy response error = %+v, want cached success", duplicateBuy.Error)
	}

	item, ok, err := gameServer.runtime.Intel.CoordinateItem(foundation.ItemID(createPayload.CoordinateItem.ItemInstanceID))
	if err != nil || !ok {
		t.Fatalf("coordinate item lookup ok=%v err=%v, want item", ok, err)
	}
	if item.OwnerPlayerID != buyer.PlayerID {
		t.Fatalf("coordinate item owner = %q, want buyer %q", item.OwnerPlayerID, buyer.PlayerID)
	}

	sellerUse := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(seller.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-market-seller-use","op":"intel.coordinate_item.use","payload":{"item_instance_id":"`+createPayload.CoordinateItem.ItemInstanceID+`"},"client_seq":3,"v":1}`),
	)
	if !sellerUse.HasError || sellerUse.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("seller use after market buy = %+v, want forbidden", sellerUse)
	}

	use := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(buyer.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-market-use","op":"intel.coordinate_item.use","payload":{"item_instance_id":"`+createPayload.CoordinateItem.ItemInstanceID+`"},"client_seq":2,"v":1}`),
	)
	if use.HasError {
		t.Fatalf("buyer coordinate use response error = %+v, want success", use.Error)
	}
	var usePayload struct {
		CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
		KnownPlanets   knownPlanetsPayload        `json:"known_planets"`
		PlanetDetail   planetDetailPayload        `json:"planet_detail"`
	}
	if err := json.Unmarshal(use.Response.Payload, &usePayload); err != nil {
		t.Fatalf("decode coordinate use payload: %v", err)
	}
	if !usePayload.CoordinateItem.Used || usePayload.PlanetDetail.PlanetID != planetID.String() || len(usePayload.KnownPlanets.Planets) != 1 {
		t.Fatalf("buyer use payload = %+v, want bought coordinate reveal", usePayload)
	}
}

func TestCoordinateItemCreateRequiresKnownPlanet(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "coordinate-missing@example.com", "Coordinate Missing")

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-missing","op":"intel.coordinate_item.create","payload":{"planet_id":"planet-coordinate-missing"},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("missing coordinate create response = %+v, want not found", response)
	}
}

func TestCoordinateItemCreateRejectsSpoofedServerOwnedFieldsBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "coordinate-create-spoof@example.com", "Coordinate Create Spoof")
	planetID := foundation.PlanetID("planet-coordinate-create-spoof")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1500, Y: 1600}, 3)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-create-spoof","op":"intel.coordinate_item.create","payload":{"planet_id":"`+planetID.String()+`","item_instance_id":"coord-forged"},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed coordinate create response = %+v, want invalid payload", response)
	}
}

func TestCoordinateItemUseRejectsSpoofedServerOwnedFieldsBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "coordinate-use-spoof@example.com", "Coordinate Use Spoof")

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-use-spoof","op":"intel.coordinate_item.use","payload":{"item_instance_id":"coord-item","planet_id":"planet-forged"},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed coordinate use response = %+v, want invalid payload", response)
	}
}

func TestCoordinateItemUseRejectsWrongOwner(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "coordinate-owner-only@example.com", "Coordinate Owner Only")
	other := createResolvedRuntimeSession(t, gameServer, "coordinate-wrong-owner@example.com", "Coordinate Wrong Owner")
	planetID := foundation.PlanetID("planet-coordinate-wrong-owner")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1500, Y: 1600}, 3)

	create := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-wrong-owner-create","op":"intel.coordinate_item.create","payload":{"planet_id":"`+planetID.String()+`"},"client_seq":1,"v":1}`),
	)
	if create.HasError {
		t.Fatalf("coordinate create response error = %+v, want success", create.Error)
	}
	var payload struct {
		CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
	}
	if err := json.Unmarshal(create.Response.Payload, &payload); err != nil {
		t.Fatalf("decode coordinate create payload: %v", err)
	}

	use := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(other.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-wrong-owner-use","op":"intel.coordinate_item.use","payload":{"item_instance_id":"`+payload.CoordinateItem.ItemInstanceID+`"},"client_seq":1,"v":1}`),
	)
	if !use.HasError || use.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("wrong-owner coordinate use response = %+v, want forbidden", use)
	}
}

func TestCoordinateItemUseRequiresMatchingInventoryInstance(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "coordinate-no-inventory@example.com", "Coordinate No Inventory")
	planetID := foundation.PlanetID("planet-coordinate-no-inventory")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1500, Y: 1600}, 3)

	if _, err := gameServer.runtime.syncIntelFromDiscovery(owner.PlayerID, planetID); err != nil {
		t.Fatalf("syncIntelFromDiscovery: %v", err)
	}
	itemID := foundation.ItemID("coord-missing-inventory")
	reference, err := foundation.CoordinateItemCreateIdempotencyKey(owner.PlayerID, planetID, itemID)
	if err != nil {
		t.Fatalf("CoordinateItemCreateIdempotencyKey: %v", err)
	}
	if _, err := gameServer.runtime.Intel.CreateCoordinateItem(intel.CreateCoordinateItemInput{
		PlayerID:       owner.PlayerID,
		PlanetID:       planetID,
		ItemInstanceID: itemID,
		Reference:      reference,
	}); err != nil {
		t.Fatalf("CreateCoordinateItem without inventory: %v", err)
	}

	use := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-no-inventory-use","op":"intel.coordinate_item.use","payload":{"item_instance_id":"`+itemID.String()+`"},"client_seq":1,"v":1}`),
	)
	if !use.HasError || use.Error.Error.Code != foundation.CodeNotEnoughCargo {
		t.Fatalf("missing inventory coordinate use response = %+v, want not enough cargo", use)
	}
	item, ok, err := gameServer.runtime.Intel.CoordinateItem(itemID)
	if err != nil || !ok {
		t.Fatalf("CoordinateItem ok=%v err=%v, want item still present", ok, err)
	}
	if item.UsedAt != nil {
		t.Fatalf("coordinate item used_at = %v, want unconsumed after missing inventory", item.UsedAt)
	}
}

func assertIntelPayloadSafe(t *testing.T, label string, payload json.RawMessage) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range []string{
		"owner_player_id",
		"created_by",
		"world_id",
		"zone_id",
		"source_type",
		"source_reference",
		"create_reference",
		"use_reference",
		"source_intel_reference",
	} {
		if strings.Contains(raw, `"`+forbidden+`"`) {
			t.Fatalf("%s leaked %s: %s", label, forbidden, raw)
		}
	}
}

func assertIntelPayloadOmitsCoordinates(t *testing.T, label string, payload json.RawMessage) {
	t.Helper()
	raw := string(payload)
	if strings.Contains(raw, `"coordinates"`) {
		t.Fatalf("%s leaked coordinates: %s", label, raw)
	}
}

func inventorySnapshotHasInstanceID(snapshot inventorySnapshotPayload, itemInstanceID string, itemID string, location string) bool {
	for _, item := range snapshot.Instances {
		if item.ItemInstanceID == itemInstanceID && item.ItemID == itemID && item.Location == location {
			return true
		}
	}
	return false
}

func createCoordinateItemForTest(t *testing.T, gameServer *Server, owner auth.ResolvedSession, planetID foundation.PlanetID, requestID string) struct {
	CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
	Inventory      inventorySnapshotPayload   `json:"inventory"`
	Created        bool                       `json:"created"`
	Duplicate      bool                       `json:"duplicate"`
} {
	t.Helper()
	create := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"`+requestID+`","op":"intel.coordinate_item.create","payload":{"planet_id":"`+planetID.String()+`"},"client_seq":1,"v":1}`),
	)
	if create.HasError {
		t.Fatalf("coordinate create response error = %+v, want success", create.Error)
	}
	var payload struct {
		CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
		Inventory      inventorySnapshotPayload   `json:"inventory"`
		Created        bool                       `json:"created"`
		Duplicate      bool                       `json:"duplicate"`
	}
	if err := json.Unmarshal(create.Response.Payload, &payload); err != nil {
		t.Fatalf("decode coordinate create payload: %v", err)
	}
	if !payload.Created || payload.CoordinateItem.ItemInstanceID == "" {
		t.Fatalf("coordinate create payload = %+v, want created item", payload)
	}
	return payload
}

func coordinateItemUseRequestForTest(requestID string, itemInstanceID string, clientSeq uint64) realtime.RequestEnvelope {
	return realtime.RequestEnvelope{
		RequestID: foundation.RequestID(requestID),
		Op:        realtime.OperationIntelCoordinateUse,
		Payload:   json.RawMessage(`{"item_instance_id":"` + itemInstanceID + `"}`),
		ClientSeq: clientSeq,
		Version:   1,
	}
}

func commandContextForResolvedSessionForTest(gameServer *Server, resolved auth.ResolvedSession) realtime.CommandContext {
	return realtime.CommandContext{
		SessionID: realtime.SessionID(resolved.SessionID.String()),
		PlayerID:  resolved.PlayerID,
		WorldID:   gameServer.runtime.worldID,
		ZoneID:    gameServer.runtime.zoneID,
	}
}

func assertCoordinateItemLedgerCount(t *testing.T, gameServer *Server, playerID foundation.PlayerID, itemInstanceID string, action economy.LedgerAction, reason economy.LedgerReason, want int) {
	t.Helper()
	got := 0
	for _, entry := range gameServer.runtime.Inventory.ItemLedgerEntries() {
		if entry.PlayerID == playerID &&
			entry.ItemID == coordinateScrollItemID &&
			entry.ItemInstanceID.String() == itemInstanceID &&
			entry.Action == action &&
			entry.Reason == reason {
			got++
		}
	}
	if got != want {
		t.Fatalf("coordinate item %s ledger %s count = %d, want %d", itemInstanceID, action, got, want)
	}
}
