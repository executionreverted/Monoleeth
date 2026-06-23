package server

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
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
		Created        bool                       `json:"created"`
		Duplicate      bool                       `json:"duplicate"`
	}
	if err := json.Unmarshal(create.Response.Payload, &createPayload); err != nil {
		t.Fatalf("decode coordinate create payload: %v", err)
	}
	if !createPayload.Created || createPayload.CoordinateItem.ItemInstanceID == "" || createPayload.CoordinateItem.Used {
		t.Fatalf("create payload = %+v, want fresh server-owned coordinate item", createPayload)
	}

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
		Used           bool                       `json:"used"`
	}
	if err := json.Unmarshal(use.Response.Payload, &usePayload); err != nil {
		t.Fatalf("decode coordinate use payload: %v", err)
	}
	if !usePayload.Used || !usePayload.CoordinateItem.Used || usePayload.PlanetDetail.PlanetID != planetID.String() || len(usePayload.KnownPlanets.Planets) != 1 {
		t.Fatalf("use payload = %+v, want consumed item and refreshed discovery", usePayload)
	}

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationIntelCoordinateUse, owner.PlayerID)
	if err != nil {
		t.Fatalf("post coordinate use events: %v", err)
	}
	requireEventTypeForTest(t, eventsBySession[owner.SessionID], realtime.EventKnownPlanets)
	requireEventTypeForTest(t, eventsBySession[owner.SessionID], realtime.EventPlanetDetail)

	secondUse := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-coordinate-use-second","op":"intel.coordinate_item.use","payload":{"item_instance_id":"`+createPayload.CoordinateItem.ItemInstanceID+`"},"client_seq":3,"v":1}`),
	)
	if !secondUse.HasError || secondUse.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("second coordinate use response = %+v, want forbidden", secondUse)
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
