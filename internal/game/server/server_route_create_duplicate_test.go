package server

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestRouteCreateDuplicateRequestIDRejectsChangedPayloadWithoutSecondRoute(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "route-create-duplicate-owner@example.com", "Route Create Duplicate")
	sourcePlanetID := foundation.PlanetID("planet-route-create-duplicate-source")
	firstDestinationPlanetID := foundation.PlanetID("planet-route-create-duplicate-first-destination")
	spoofedDestinationPlanetID := foundation.PlanetID("planet-route-create-duplicate-spoofed-destination")
	requestID := foundation.RequestID("request-route-create-duplicate")
	routeID := foundation.RouteID("route-" + requestID.String())

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-create-duplicate-source")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, firstDestinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-create-duplicate-first-destination")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, spoofedDestinationPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 2100, Y: 2400}, "candidate-route-create-duplicate-spoofed-destination")

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"`+requestID.String()+`","op":"route.create","payload":{"source_planet_id":"`+sourcePlanetID.String()+`","destination_planet_id":"`+firstDestinationPlanetID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":40},"client_seq":1,"v":1}`),
	)
	assertRouteCreateDuplicateResponse(t, first, routeID, sourcePlanetID, firstDestinationPlanetID, "1-1", "1-2", 40, "first route.create")
	stored, ok, err := gameServer.runtime.Production.AutomationRoute(routeID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok=%v err=%v, want stored route", routeID, ok, err)
	}
	assertStoredRouteEnergyReserved(t, gameServer, sourcePlanetID, stored.EnergyCostPerHour)
	if routes := gameServer.runtime.Production.AutomationRoutes(); len(routes) != 1 {
		t.Fatalf("routes after first route.create = %+v, want one route", routes)
	}
	if records, err := gameServer.runtime.Production.CommittedAutomationRouteDurableRecordsForOwner(owner.PlayerID); err != nil || len(records) != 1 {
		t.Fatalf("durable route records after first route.create = %+v/%v, want one", records, err)
	}
	if _, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteCreate, owner.PlayerID); err != nil {
		t.Fatalf("drain first route.create events: %v", err)
	}

	duplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"`+requestID.String()+`","op":"route.create","payload":{"source_planet_id":"`+sourcePlanetID.String()+`","destination_planet_id":"`+spoofedDestinationPlanetID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":90},"client_seq":2,"v":1}`),
	)
	assertGatewayReplayMismatchForTest(t, duplicate, "duplicate changed-payload route.create")
	assertStoredRouteEnergyReserved(t, gameServer, sourcePlanetID, stored.EnergyCostPerHour)
	routes := gameServer.runtime.Production.AutomationRoutes()
	if len(routes) != 1 {
		t.Fatalf("routes after duplicate route.create = %+v, want one route", routes)
	}
	if routes[0].Destination.ID != production.RouteDestinationID(firstDestinationPlanetID.String()) || routes[0].AmountPerHour != 40 {
		t.Fatalf("route after duplicate route.create = %+v, want original destination/rate", routes[0])
	}
	records, err := gameServer.runtime.Production.CommittedAutomationRouteDurableRecordsForOwner(owner.PlayerID)
	if err != nil || len(records) != 1 || records[0].Route.Destination.ID != production.RouteDestinationID(firstDestinationPlanetID.String()) {
		t.Fatalf("durable route records after duplicate route.create = %+v/%v, want original single record", records, err)
	}
	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteCreate, owner.PlayerID)
	if err != nil {
		t.Fatalf("post duplicate route.create events: %v", err)
	}
	if events := eventsBySession[owner.SessionID]; len(events) != 0 {
		t.Fatalf("duplicate route.create queued events = %+v, want none from cached replay", events)
	}
}

func assertRouteCreateDuplicateResponse(
	t *testing.T,
	response realtime.CachedResponse,
	routeID foundation.RouteID,
	sourcePlanetID foundation.PlanetID,
	destinationPlanetID foundation.PlanetID,
	fromPublicMapKey string,
	toPublicMapKey string,
	amountPerHour int64,
	label string,
) {
	t.Helper()
	if response.HasError {
		t.Fatalf("%s response error = %+v, want success", label, response.Error)
	}
	assertPayloadOmitsInternalMapIdentity(t, label+" response", response.Response.Payload)
	var payload struct {
		Route  routePayload     `json:"route"`
		Routes routeListPayload `json:"routes"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode %s payload: %v", label, err)
	}
	assertRoutePayloadMapKeys(t, payload.Route, routeID, fromPublicMapKey, toPublicMapKey)
	if payload.Route.SourcePlanetID != sourcePlanetID.String() ||
		payload.Route.Destination.Type != production.RouteDestinationTypePlanet.String() ||
		payload.Route.Destination.ID != destinationPlanetID.String() ||
		payload.Route.ResourceItemID != "refined_alloy" ||
		payload.Route.AmountPerHour != amountPerHour ||
		!payload.Route.Enabled {
		t.Fatalf("%s route payload = %+v, want original safe created route", label, payload.Route)
	}
	if len(payload.Routes.Routes) != 1 {
		t.Fatalf("%s route list = %+v, want one route", label, payload.Routes.Routes)
	}
}
