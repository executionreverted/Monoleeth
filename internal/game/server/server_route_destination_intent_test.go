package server

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
)

func TestRouteCreateAcceptsNonPlanetDestinationIntentThroughGateway(t *testing.T) {
	for _, tc := range nonPlanetRouteDestinationCases() {
		t.Run(tc.name, func(t *testing.T) {
			gameServer, _ := newTestServer(t, false)
			owner := createResolvedRuntimeSession(t, gameServer, "route-create-"+tc.name+"@example.com", "Route Create "+tc.name)
			sourcePlanetID := foundation.PlanetID("planet-route-create-" + tc.name + "-source")
			destinationStorageID := foundation.PlanetID(runtimeRouteEndpointID(owner.PlayerID, tc.destinationType))

			seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, discovery.PlanetMaterializationKey("candidate-route-create-"+tc.name+"-source"))

			requestID := foundation.RequestID("request-route-create-" + tc.name + "-destination")
			wantRouteID := foundation.RouteID("route-" + requestID.String())
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(owner.SessionID.String()),
				[]byte(`{"request_id":"`+requestID.String()+`","op":"route.create","payload":{"source_planet_id":"`+sourcePlanetID.String()+`","destination_type":"`+tc.destinationType.String()+`","destination_id":"`+destinationStorageID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":40},"client_seq":1,"v":1}`),
			)
			if response.HasError {
				t.Fatalf("route.create %s response error = %+v, want success", tc.name, response.Error)
			}
			assertPayloadOmitsRouteEndpointID(t, "route.create "+tc.name+" response", response.Response.Payload, destinationStorageID.String())

			var payload struct {
				Route routePayload `json:"route"`
			}
			if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
				t.Fatalf("decode route.create %s payload: %v", tc.name, err)
			}
			if payload.Route.Destination.Type != tc.destinationType.String() || payload.Route.Destination.ID != "" {
				t.Fatalf("route.create %s public destination = %+v, want masked %q destination", tc.name, payload.Route.Destination, tc.destinationType)
			}
			assertRoutePayloadMapKeys(t, payload.Route, wantRouteID, "1-1", "1-1")

			stored, ok, err := gameServer.runtime.Production.AutomationRoute(wantRouteID)
			if err != nil || !ok {
				t.Fatalf("AutomationRoute(%q) ok=%v err=%v, want stored route", wantRouteID, ok, err)
			}
			if stored.Destination.Type != tc.destinationType || stored.Destination.ID.String() != destinationStorageID.String() {
				t.Fatalf("stored route destination = %+v, want %s/%s", stored.Destination, tc.destinationType, destinationStorageID)
			}
			if _, ok, err := gameServer.runtime.Production.PlanetStorage(destinationStorageID); err != nil || !ok {
				t.Fatalf("PlanetStorage(%q) ok=%v err=%v, want ensured endpoint storage", destinationStorageID, ok, err)
			}

			eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteCreate, owner.PlayerID)
			if err != nil {
				t.Fatalf("post route.create %s events: %v", tc.name, err)
			}
			for _, event := range eventsBySession[owner.SessionID] {
				assertPayloadOmitsRouteEndpointID(t, string(event.Type)+" "+tc.name+" event", event.Payload, destinationStorageID.String())
			}
		})
	}
}

func TestRouteUpdateAcceptsNonPlanetDestinationIntentThroughGateway(t *testing.T) {
	for _, tc := range nonPlanetRouteDestinationCases() {
		t.Run(tc.name, func(t *testing.T) {
			gameServer, _ := newTestServer(t, false)
			owner := createResolvedRuntimeSession(t, gameServer, "route-update-"+tc.name+"@example.com", "Route Update "+tc.name)
			sourcePlanetID := foundation.PlanetID("planet-route-update-" + tc.name + "-source")
			oldDestinationPlanetID := foundation.PlanetID("planet-route-update-" + tc.name + "-old-destination")
			destinationStorageID := foundation.PlanetID(runtimeRouteEndpointID(owner.PlayerID, tc.destinationType))
			routeID := foundation.RouteID("route-update-" + tc.name + "-destination")

			seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, discovery.PlanetMaterializationKey("candidate-route-update-"+tc.name+"-source"))
			seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, oldDestinationPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1700, Y: 1900}, discovery.PlanetMaterializationKey("candidate-route-update-"+tc.name+"-old-destination"))
			seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, oldDestinationPlanetID, "map_1_1", "map_1_1")

			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(owner.SessionID.String()),
				[]byte(`{"request_id":"request-route-update-`+tc.name+`-destination","op":"route.update","payload":{"route_id":"`+routeID.String()+`","destination_type":"`+tc.destinationType.String()+`","destination_id":"`+destinationStorageID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":55},"client_seq":1,"v":1}`),
			)
			if response.HasError {
				t.Fatalf("route.update %s response error = %+v, want success", tc.name, response.Error)
			}
			assertPayloadOmitsRouteEndpointID(t, "route.update "+tc.name+" response", response.Response.Payload, destinationStorageID.String())

			var payload struct {
				Route routePayload `json:"route"`
			}
			if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
				t.Fatalf("decode route.update %s payload: %v", tc.name, err)
			}
			if payload.Route.Destination.Type != tc.destinationType.String() || payload.Route.Destination.ID != "" || payload.Route.AmountPerHour != 55 {
				t.Fatalf("route.update %s public route = %+v, want masked destination/rate", tc.name, payload.Route)
			}
			assertRoutePayloadMapKeys(t, payload.Route, routeID, "1-1", "1-1")

			stored, ok, err := gameServer.runtime.Production.AutomationRoute(routeID)
			if err != nil || !ok {
				t.Fatalf("AutomationRoute(%q) ok=%v err=%v, want stored route", routeID, ok, err)
			}
			if stored.Destination.Type != tc.destinationType || stored.Destination.ID.String() != destinationStorageID.String() || stored.AmountPerHour != 55 {
				t.Fatalf("stored route after update = %+v, want %s/%s amount 55", stored, tc.destinationType, destinationStorageID)
			}
			if _, ok, err := gameServer.runtime.Production.PlanetStorage(destinationStorageID); err != nil || !ok {
				t.Fatalf("PlanetStorage(%q) ok=%v err=%v, want ensured endpoint storage", destinationStorageID, ok, err)
			}
		})
	}
}

func TestPlanetDetailIncludesOwnerRouteEndpointCatalog(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "route-endpoint-catalog@example.com", "Route Endpoint Catalog")
	other := createResolvedRuntimeSession(t, gameServer, "route-endpoint-catalog-other@example.com", "Route Endpoint Other")
	planetID := foundation.PlanetID("planet-route-endpoint-catalog")
	coordinates := world.Vec2{X: 1300, Y: 1400}

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, planetID, gameServer.runtime.zoneID, coordinates, discovery.PlanetMaterializationKey("candidate-route-endpoint-catalog"))
	if _, _, err := gameServer.runtime.Discovery.UpsertPlayerPlanetIntel(discovery.PlayerPlanetIntel{
		PlayerID:        owner.PlayerID,
		PlanetID:        planetID,
		WorldID:         gameServer.runtime.worldID,
		ZoneID:          gameServer.runtime.zoneID,
		Coordinates:     coordinates,
		State:           discovery.IntelStateVerified,
		Confidence:      100,
		LastSeenAt:      gameServer.runtime.clock.Now().UTC(),
		SourceType:      discovery.IntelSourceAdmin,
		SourceReference: "route-endpoint-catalog",
	}); err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel(%q) error = %v, want nil", planetID, err)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-endpoint-catalog","op":"discovery.planet_detail","payload":{"planet_id":"`+planetID.String()+`"},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("planet_detail endpoint catalog response error = %+v, want success", response.Error)
	}
	var payload struct {
		PlanetDetail planetDetailPayload `json:"planet_detail"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode planet_detail endpoint catalog payload: %v", err)
	}
	if len(payload.PlanetDetail.RouteEndpoints) != 2 {
		t.Fatalf("route endpoints = %+v, want storage and station", payload.PlanetDetail.RouteEndpoints)
	}
	for _, endpoint := range payload.PlanetDetail.RouteEndpoints {
		if endpoint.ID == "" || endpoint.Label == "" {
			t.Fatalf("route endpoint = %+v, want public id and label", endpoint)
		}
		if endpoint.Type == production.RouteDestinationTypeStorage.String() && endpoint.ID != runtimeRouteEndpointID(owner.PlayerID, production.RouteDestinationTypeStorage).String() {
			t.Fatalf("storage endpoint id = %q, want owner-scoped id", endpoint.ID)
		}
		if endpoint.Type == production.RouteDestinationTypeStation.String() && endpoint.ID != runtimeRouteEndpointID(owner.PlayerID, production.RouteDestinationTypeStation).String() {
			t.Fatalf("station endpoint id = %q, want owner-scoped id", endpoint.ID)
		}
		if endpoint.ID == runtimeRouteEndpointID(other.PlayerID, production.RouteDestinationTypeStorage).String() ||
			endpoint.ID == runtimeRouteEndpointID(other.PlayerID, production.RouteDestinationTypeStation).String() {
			t.Fatalf("route endpoint = %+v leaked other player endpoint id", endpoint)
		}
	}
}

func TestRouteCreateRejectsUncatalogedNonPlanetDestinationIntent(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "route-create-uncataloged@example.com", "Route Create Uncataloged")
	sourcePlanetID := foundation.PlanetID("planet-route-create-uncataloged-source")
	uncatalogedID := foundation.PlanetID("route-storage-uncataloged")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, discovery.PlanetMaterializationKey("candidate-route-create-uncataloged-source"))
	saveRouteControlStorage(t, gameServer, uncatalogedID, nil)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-create-uncataloged","op":"route.create","payload":{"source_planet_id":"`+sourcePlanetID.String()+`","destination_type":"storage","destination_id":"`+uncatalogedID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":40},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("uncataloged route.create response = %+v, want safe not-found", response)
	}
	if routes := gameServer.runtime.Production.AutomationRoutes(); len(routes) != 0 {
		t.Fatalf("routes after uncataloged endpoint = %+v, want no mutation", routes)
	}
}

func nonPlanetRouteDestinationCases() []struct {
	name            string
	destinationType production.RouteDestinationType
} {
	return []struct {
		name            string
		destinationType production.RouteDestinationType
	}{
		{name: "storage", destinationType: production.RouteDestinationTypeStorage},
		{name: "station", destinationType: production.RouteDestinationTypeStation},
	}
}
