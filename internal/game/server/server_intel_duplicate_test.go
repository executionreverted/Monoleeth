package server

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestIntelShareDuplicateRequestIDRejectsChangedPayloadWithoutSecondReceiverMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	sender := createResolvedRuntimeSession(t, gameServer, "intel-share-duplicate-sender@example.com", "Intel Duplicate Sender")
	receiverOne := createResolvedRuntimeSession(t, gameServer, "intel-share-duplicate-one@example.com", "Intel Duplicate One")
	receiverTwo := createResolvedRuntimeSession(t, gameServer, "intel-share-duplicate-two@example.com", "Intel Duplicate Two")
	planetOneID := foundation.PlanetID("planet-intel-share-duplicate-one")
	planetTwoID := foundation.PlanetID("planet-intel-share-duplicate-two")
	requestID := "request-intel-share-duplicate"

	seedKnownClaimPlanetForTest(t, gameServer, sender.PlayerID, planetOneID, worldmaps.StarterMapID, world.Vec2{X: 1200, Y: 1300}, 2)
	seedKnownClaimPlanetForTest(t, gameServer, sender.PlayerID, planetTwoID, worldmaps.StarterMapID, world.Vec2{X: 1400, Y: 1500}, 2)

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(sender.SessionID.String()),
		[]byte(`{"request_id":"`+requestID+`","op":"intel.share","payload":{"planet_id":"`+planetOneID.String()+`","to_player_id":"`+receiverOne.PlayerID.String()+`"},"client_seq":1,"v":1}`),
	)
	assertIntelShareDuplicateResponse(t, first, planetOneID, receiverOne.PlayerID, true, "first intel.share")
	if _, ok, err := gameServer.runtime.Discovery.PlayerPlanetIntel(receiverOne.PlayerID, planetOneID); err != nil || !ok {
		t.Fatalf("receiver one discovery intel ok=%v err=%v, want first share", ok, err)
	}
	if _, ok, err := gameServer.runtime.Intel.PlayerPlanetIntel(receiverOne.PlayerID, planetOneID); err != nil || !ok {
		t.Fatalf("receiver one runtime intel ok=%v err=%v, want first share", ok, err)
	}
	if _, err := gameServer.runtime.postCommandEventsBySession(sender.SessionID, realtime.OperationIntelShare, sender.PlayerID); err != nil {
		t.Fatalf("drain first intel.share events: %v", err)
	}

	duplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(sender.SessionID.String()),
		[]byte(`{"request_id":"`+requestID+`","op":"intel.share","payload":{"planet_id":"`+planetTwoID.String()+`","to_player_id":"`+receiverTwo.PlayerID.String()+`"},"client_seq":2,"v":1}`),
	)
	assertGatewayReplayMismatchForTest(t, duplicate, "duplicate changed-payload intel.share")
	if _, ok, err := gameServer.runtime.Discovery.PlayerPlanetIntel(receiverTwo.PlayerID, planetTwoID); err != nil || ok {
		t.Fatalf("receiver two discovery intel after duplicate ok=%v err=%v, want no changed-payload mutation", ok, err)
	}
	if _, ok, err := gameServer.runtime.Intel.PlayerPlanetIntel(receiverTwo.PlayerID, planetTwoID); err != nil || ok {
		t.Fatalf("receiver two runtime intel after duplicate ok=%v err=%v, want no changed-payload mutation", ok, err)
	}
	if _, ok, err := gameServer.runtime.Discovery.PlayerPlanetIntel(receiverTwo.PlayerID, planetOneID); err != nil || ok {
		t.Fatalf("receiver two discovery planet-one intel after duplicate ok=%v err=%v, want no mutation", ok, err)
	}
	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(sender.SessionID, realtime.OperationIntelShare, sender.PlayerID)
	if err != nil {
		t.Fatalf("post duplicate intel.share events: %v", err)
	}
	if events := eventsBySession[receiverTwo.SessionID]; len(events) != 0 {
		t.Fatalf("duplicate intel.share receiver two events = %+v, want none", events)
	}
	if events := eventsBySession[receiverOne.SessionID]; len(events) != 0 {
		t.Fatalf("duplicate intel.share receiver one events = %+v, want cached replay without events", events)
	}
}

func TestCoordinateItemCreateDuplicateRequestIDRejectsChangedPlanetWithoutSecondScroll(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "coordinate-create-duplicate@example.com", "Coordinate Create Duplicate")
	planetOneID := foundation.PlanetID("planet-coordinate-create-duplicate-one")
	planetTwoID := foundation.PlanetID("planet-coordinate-create-duplicate-two")
	requestID := "request-coordinate-create-duplicate"

	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetOneID, worldmaps.StarterMapID, world.Vec2{X: 1500, Y: 1600}, 3)
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetTwoID, worldmaps.StarterMapID, world.Vec2{X: 1700, Y: 1800}, 3)

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"`+requestID+`","op":"intel.coordinate_item.create","payload":{"planet_id":"`+planetOneID.String()+`"},"client_seq":1,"v":1}`),
	)
	firstPayload := assertCoordinateItemCreateDuplicateResponse(t, first, planetOneID, "first coordinate create")
	if !inventorySnapshotHasInstanceID(firstPayload.Inventory, firstPayload.CoordinateItem.ItemInstanceID, coordinateScrollItemID.String(), economy.LocationKindAccountInventory.String()) {
		t.Fatalf("first coordinate create inventory = %+v, want coordinate scroll %s", firstPayload.Inventory, firstPayload.CoordinateItem.ItemInstanceID)
	}
	if got := countInventoryInstances(gameServer.runtime.Inventory.InstanceItems(), coordinateScrollItemID.String()); got != 1 {
		t.Fatalf("coordinate scroll count after first create = %d, want 1", got)
	}
	assertCoordinateItemLedgerCount(t, gameServer, owner.PlayerID, firstPayload.CoordinateItem.ItemInstanceID, economy.LedgerActionIncrease, intelCoordinateItemCreateLedgerReason, 1)
	if _, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationIntelCoordinateCreate, owner.PlayerID); err != nil {
		t.Fatalf("drain first coordinate create events: %v", err)
	}

	duplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"`+requestID+`","op":"intel.coordinate_item.create","payload":{"planet_id":"`+planetTwoID.String()+`"},"client_seq":2,"v":1}`),
	)
	assertGatewayReplayMismatchForTest(t, duplicate, "duplicate changed-planet coordinate create")
	if got := countInventoryInstances(gameServer.runtime.Inventory.InstanceItems(), coordinateScrollItemID.String()); got != 1 {
		t.Fatalf("coordinate scroll count after duplicate changed-planet create = %d, want 1", got)
	}
	assertCoordinateItemLedgerCount(t, gameServer, owner.PlayerID, firstPayload.CoordinateItem.ItemInstanceID, economy.LedgerActionIncrease, intelCoordinateItemCreateLedgerReason, 1)
	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationIntelCoordinateCreate, owner.PlayerID)
	if err != nil {
		t.Fatalf("post duplicate coordinate create events: %v", err)
	}
	if events := eventsBySession[owner.SessionID]; len(events) != 0 {
		t.Fatalf("duplicate coordinate create events = %+v, want cached replay without events", events)
	}
}

func assertIntelShareDuplicateResponse(
	t *testing.T,
	response realtime.CachedResponse,
	planetID foundation.PlanetID,
	toPlayerID foundation.PlayerID,
	receiverUpdated bool,
	label string,
) {
	t.Helper()
	if response.HasError {
		t.Fatalf("%s response error = %+v, want success", label, response.Error)
	}
	assertIntelPayloadSafe(t, label+" response", response.Response.Payload)
	assertIntelPayloadOmitsCoordinates(t, label+" response", response.Response.Payload)
	var payload struct {
		Share intelSharePayload `json:"share"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode %s payload: %v", label, err)
	}
	if payload.Share.PlanetID != planetID.String() ||
		payload.Share.ToPlayerID != toPlayerID.String() ||
		!payload.Share.Shared ||
		payload.Share.ReceiverUpdated != receiverUpdated {
		t.Fatalf("%s share payload = %+v, want planet %q receiver %q updated %v", label, payload.Share, planetID, toPlayerID, receiverUpdated)
	}
}

func assertCoordinateItemCreateDuplicateResponse(
	t *testing.T,
	response realtime.CachedResponse,
	planetID foundation.PlanetID,
	label string,
) struct {
	CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
	Inventory      inventorySnapshotPayload   `json:"inventory"`
	Created        bool                       `json:"created"`
	Duplicate      bool                       `json:"duplicate"`
} {
	t.Helper()
	if response.HasError {
		t.Fatalf("%s response error = %+v, want success", label, response.Error)
	}
	assertIntelPayloadSafe(t, label+" response", response.Response.Payload)
	assertIntelPayloadOmitsCoordinates(t, label+" response", response.Response.Payload)
	var payload struct {
		CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
		Inventory      inventorySnapshotPayload   `json:"inventory"`
		Created        bool                       `json:"created"`
		Duplicate      bool                       `json:"duplicate"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode %s payload: %v", label, err)
	}
	if !payload.Created ||
		payload.Duplicate ||
		payload.CoordinateItem.PlanetID != planetID.String() ||
		payload.CoordinateItem.ItemInstanceID == "" ||
		payload.CoordinateItem.Used {
		t.Fatalf("%s coordinate item payload = %+v, want created unused item for planet %q", label, payload, planetID)
	}
	return payload
}
