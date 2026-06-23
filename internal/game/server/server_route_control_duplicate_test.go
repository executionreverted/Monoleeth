package server

import (
	"encoding/json"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestRouteDisableDuplicateRequestIDIgnoresChangedRouteWithoutSecondMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "route-control-duplicate-owner@example.com", "Route Control Duplicate")
	sourceOneID := foundation.PlanetID("planet-route-control-duplicate-source-one")
	destinationOneID := foundation.PlanetID("planet-route-control-duplicate-destination-one")
	sourceTwoID := foundation.PlanetID("planet-route-control-duplicate-source-two")
	destinationTwoID := foundation.PlanetID("planet-route-control-duplicate-destination-two")
	routeOneID := foundation.RouteID("route-control-duplicate-one")
	routeTwoID := foundation.RouteID("route-control-duplicate-two")
	requestID := "request-route-control-duplicate"

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourceOneID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-control-duplicate-source-one")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationOneID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-control-duplicate-destination-one")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourceTwoID, gameServer.runtime.zoneID, world.Vec2{X: 2100, Y: 2400}, "candidate-route-control-duplicate-source-two")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationTwoID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 2500, Y: 5600}, "candidate-route-control-duplicate-destination-two")
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeOneID, sourceOneID, destinationOneID, "map_1_1", "map_1_2")
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeTwoID, sourceTwoID, destinationTwoID, "map_1_1", "map_1_2")
	routeTwoBefore := storedRouteForDuplicateControlTest(t, gameServer, routeTwoID)

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"`+requestID+`","op":"route.disable","payload":{"route_id":"`+routeOneID.String()+`"},"client_seq":1,"v":1}`),
	)
	assertRouteControlDuplicateResponse(t, first, routeOneID, "1-1", "1-2", false, "first route.disable")
	assertStoredRouteEnabled(t, gameServer, routeOneID, false)
	assertStoredRouteEnergyReserved(t, gameServer, sourceOneID, 0)
	assertStoredRouteEnabled(t, gameServer, routeTwoID, true)
	assertStoredRouteEnergyReserved(t, gameServer, sourceTwoID, routeTwoBefore.EnergyCostPerHour)
	if _, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteDisable, owner.PlayerID); err != nil {
		t.Fatalf("drain first route.disable events: %v", err)
	}

	duplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"`+requestID+`","op":"route.disable","payload":{"route_id":"`+routeTwoID.String()+`"},"client_seq":2,"v":1}`),
	)
	assertRouteControlDuplicateResponse(t, duplicate, routeOneID, "1-1", "1-2", false, "duplicate route.disable")
	assertStoredRouteEnabled(t, gameServer, routeOneID, false)
	assertStoredRouteEnergyReserved(t, gameServer, sourceOneID, 0)
	assertStoredRouteEnabled(t, gameServer, routeTwoID, true)
	assertStoredRouteEnergyReserved(t, gameServer, sourceTwoID, routeTwoBefore.EnergyCostPerHour)
	routeTwoAfter := storedRouteForDuplicateControlTest(t, gameServer, routeTwoID)
	if !routeTwoAfter.UpdatedAt.Equal(routeTwoBefore.UpdatedAt) || routeTwoAfter.Enabled != routeTwoBefore.Enabled {
		t.Fatalf("route two after duplicate route.disable = %+v, want unchanged %+v", routeTwoAfter, routeTwoBefore)
	}
	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteDisable, owner.PlayerID)
	if err != nil {
		t.Fatalf("post duplicate route.disable events: %v", err)
	}
	if events := eventsBySession[owner.SessionID]; len(events) != 0 {
		t.Fatalf("duplicate route.disable queued events = %+v, want none from cached replay", events)
	}
}

func assertRouteControlDuplicateResponse(
	t *testing.T,
	response realtime.CachedResponse,
	routeID foundation.RouteID,
	fromPublicMapKey string,
	toPublicMapKey string,
	enabled bool,
	label string,
) {
	t.Helper()
	if response.HasError {
		t.Fatalf("%s response error = %+v, want success", label, response.Error)
	}
	assertPayloadOmitsInternalMapIdentity(t, label+" response", response.Response.Payload)
	var payload struct {
		Route routePayload `json:"route"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode %s payload: %v", label, err)
	}
	assertRoutePayloadMapKeys(t, payload.Route, routeID, fromPublicMapKey, toPublicMapKey)
	if payload.Route.Enabled != enabled {
		t.Fatalf("%s route enabled = %v, want %v", label, payload.Route.Enabled, enabled)
	}
}

func storedRouteForDuplicateControlTest(t *testing.T, gameServer *Server, routeID foundation.RouteID) productionRouteSnapshotForDuplicateControl {
	t.Helper()
	route, ok, err := gameServer.runtime.Production.AutomationRoute(routeID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok=%v err=%v, want stored route", routeID, ok, err)
	}
	return productionRouteSnapshotForDuplicateControl{
		Enabled:           route.Enabled,
		EnergyCostPerHour: route.EnergyCostPerHour,
		UpdatedAt:         route.UpdatedAt,
	}
}

type productionRouteSnapshotForDuplicateControl struct {
	Enabled           bool
	EnergyCostPerHour int64
	UpdatedAt         time.Time
}
