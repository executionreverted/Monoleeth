package server

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestRouteSnapshotReplaysDurableRouteAfterLiveRouteRowLoss(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "route-snapshot-owner@example.com", "Route Snapshot Owner")
	other := createResolvedRuntimeSession(t, gameServer, "route-snapshot-other@example.com", "Route Snapshot Other")
	sourcePlanetID := foundation.PlanetID("planet-route-snapshot-source")
	destinationPlanetID := foundation.PlanetID("planet-route-snapshot-destination")
	routeID := foundation.RouteID("route-snapshot-durable-replay")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-snapshot-source")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-snapshot-destination")
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	if err := gameServer.runtime.Production.DropAutomationRouteReadModel(routeID); err != nil {
		t.Fatalf("DropAutomationRouteReadModel() error = %v, want nil", err)
	}
	if _, ok, err := gameServer.runtime.Production.AutomationRoute(routeID); err != nil || ok {
		t.Fatalf("live AutomationRoute(%q) ok=%v err=%v, want dropped live row", routeID, ok, err)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-snapshot-durable-replay","op":"route.snapshot","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("route.snapshot durable replay response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsInternalMapIdentity(t, "route.snapshot durable replay", response.Response.Payload)
	var payload struct {
		Route routePayload `json:"route"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode route.snapshot durable replay payload: %v", err)
	}
	assertRoutePayloadMapKeys(t, payload.Route, routeID, "1-1", "1-2")

	notOwner := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(other.SessionID.String()),
		[]byte(`{"request_id":"request-route-snapshot-durable-replay-other","op":"route.snapshot","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if !notOwner.HasError || notOwner.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("non-owner durable route.snapshot response = %+v, want safe not-found", notOwner)
	}
}
