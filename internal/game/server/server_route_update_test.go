package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestRouteUpdateChangesOwnedRouteTermsThroughGateway(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "route-update-owner@example.com", "Route Update Owner")
	other := createResolvedRuntimeSession(t, gameServer, "route-update-other@example.com", "Route Update Other")
	sourcePlanetID := foundation.PlanetID("planet-route-update-source")
	oldDestinationPlanetID := foundation.PlanetID("planet-route-update-old-destination")
	newDestinationPlanetID := foundation.PlanetID("planet-route-update-new-destination")
	routeID := foundation.RouteID("route-update-owned")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-update-source")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, oldDestinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-update-old-destination")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, newDestinationPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 2100, Y: 2400}, "candidate-route-update-new-destination")
	seedAutomationRouteForTestWithResource(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, oldDestinationPlanetID, "map_1_1", "map_1_2", "raw_ore", 40)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-update-owned","op":"route.update","payload":{"route_id":"`+routeID.String()+`","destination_planet_id":"`+newDestinationPlanetID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":75},"client_seq":1,"v":1}`),
	)
	assertRouteUpdateResponse(t, response, routeID, sourcePlanetID, newDestinationPlanetID, "1-1", "1-1", "refined_alloy", 75, true, "route.update")

	stored, ok, err := gameServer.runtime.Production.AutomationRoute(routeID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok=%v err=%v, want stored route", routeID, ok, err)
	}
	if stored.OwnerPlayerID != owner.PlayerID ||
		stored.SourcePlanetID != sourcePlanetID ||
		stored.Destination.ID != production.RouteDestinationID(newDestinationPlanetID.String()) ||
		stored.ResourceItemID != "refined_alloy" ||
		stored.AmountPerHour != 75 ||
		stored.SourceMapID != "map_1_1" ||
		stored.DestinationMapID != "map_1_1" {
		t.Fatalf("stored route after update = %+v, want server-owned updated terms", stored)
	}
	assertStoredRouteEnergyReserved(t, gameServer, sourcePlanetID, stored.EnergyCostPerHour)

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteUpdate, owner.PlayerID)
	if err != nil {
		t.Fatalf("post route.update events: %v", err)
	}
	if _, leaked := eventsBySession[other.SessionID]; leaked {
		t.Fatalf("route.update events leaked to non-owner session: %+v", eventsBySession[other.SessionID])
	}
	assertRouteUpdateEvents(t, eventsBySession[owner.SessionID], routeID, newDestinationPlanetID, "1-1", "1-1", "refined_alloy", 75, true, "route.update")
	assertRouteListAndSnapshotMapKeysWithRequestSuffix(t, gameServer, owner, routeID, "1-1", "1-1", "update-owned")
}

func TestRouteUpdateSettlesElapsedStorageAndQueuesActiveMapSnapshots(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-update-settle-owner@example.com", "Route Update Settle Owner")
	other := createResolvedRuntimeSession(t, gameServer, "route-update-settle-other@example.com", "Route Update Settle Other")
	sourcePlanetID := foundation.PlanetID("planet-route-update-settle-source")
	oldDestinationPlanetID := foundation.PlanetID("planet-route-update-settle-old-destination")
	newDestinationPlanetID := foundation.PlanetID("planet-route-update-settle-new-destination")
	routeID := foundation.RouteID("route-update-settle")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-update-settle-source")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, oldDestinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-update-settle-old-destination")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, newDestinationPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 2100, Y: 2400}, "candidate-route-update-settle-new-destination")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, oldDestinationPlanetID, "map_1_1", "map_1_2")
	clock.Advance(time.Hour)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-update-settle","op":"route.update","payload":{"route_id":"`+routeID.String()+`","destination_planet_id":"`+newDestinationPlanetID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":55},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("route.update settlement response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsInternalMapIdentity(t, "route.update settlement response", response.Response.Payload)
	var payload struct {
		Route      routePayload                      `json:"route"`
		Routes     routeListPayload                  `json:"routes"`
		Production planetProductionCollectionPayload `json:"production"`
		Storage    planetStorageCollectionPayload    `json:"storage"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode route.update settlement payload: %v", err)
	}
	assertRoutePayloadMapKeys(t, payload.Route, routeID, "1-1", "1-1")
	if payload.Route.Destination.ID != newDestinationPlanetID.String() || payload.Route.AmountPerHour != 55 || !payload.Route.Enabled {
		t.Fatalf("route.update settlement route = %+v, want new destination/rate and enabled", payload.Route)
	}
	assertRouteUpdateActiveMapSnapshots(t, payload.Production, payload.Storage, sourcePlanetID, 60)
	assertStoredRouteEnergyReserved(t, gameServer, sourcePlanetID, payload.Route.EnergyCostPerHour)
	assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 60)
	assertStoredRouteStorageQuantity(t, gameServer, oldDestinationPlanetID, "refined_alloy", 40)
	assertStoredRouteStorageQuantity(t, gameServer, newDestinationPlanetID, "refined_alloy", 0)

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteUpdate, owner.PlayerID)
	if err != nil {
		t.Fatalf("post route.update settlement events: %v", err)
	}
	if _, leaked := eventsBySession[other.SessionID]; leaked {
		t.Fatalf("route.update settlement events leaked to non-owner session: %+v", eventsBySession[other.SessionID])
	}
	assertRouteUpdateSettlementEvents(t, eventsBySession[owner.SessionID], routeID, newDestinationPlanetID, "1-1", "1-1", sourcePlanetID, 60)
}

func TestRouteUpdateRejectsWrongOwnerWithoutMutationOrEvents(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "route-update-owner-safe@example.com", "Route Owner")
	other := createResolvedRuntimeSession(t, gameServer, "route-update-wrong-owner@example.com", "Route Other")
	sourcePlanetID := foundation.PlanetID("planet-route-update-wrong-owner-source")
	oldDestinationPlanetID := foundation.PlanetID("planet-route-update-wrong-owner-old-destination")
	newDestinationPlanetID := foundation.PlanetID("planet-route-update-wrong-owner-new-destination")
	routeID := foundation.RouteID("route-update-wrong-owner")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-update-wrong-owner-source")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, oldDestinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-update-wrong-owner-old-destination")
	seedOwnedProductionPlanetForTest(t, gameServer, other.PlayerID, newDestinationPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 2100, Y: 2400}, "candidate-route-update-wrong-owner-new-destination")
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, oldDestinationPlanetID, "map_1_1", "map_1_2")

	beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(other.SessionID.String()),
		[]byte(`{"request_id":"request-route-update-wrong-owner","op":"route.update","payload":{"route_id":"`+routeID.String()+`","destination_planet_id":"`+newDestinationPlanetID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":55},"client_seq":1,"v":1}`),
	)
	if !response.HasError || (response.Error.Error.Code != foundation.CodeNotFound && response.Error.Error.Code != foundation.CodeForbidden) {
		t.Fatalf("route.update wrong-owner response = %+v, want safe not-found/forbidden", response)
	}
	assertRoutesUnchanged(t, gameServer, beforeRoutes, "route.update wrong-owner")
	assertNoQueuedEventsForSessions(t, gameServer, owner.SessionID, other.SessionID)
}

func TestRouteUpdateRejectsSpoofedServerOwnedFieldsBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "route-update-spoof@example.com", "Route Update Spoof")
	sourcePlanetID := foundation.PlanetID("planet-route-update-spoof-source")
	oldDestinationPlanetID := foundation.PlanetID("planet-route-update-spoof-old-destination")
	newDestinationPlanetID := foundation.PlanetID("planet-route-update-spoof-new-destination")
	routeID := foundation.RouteID("route-update-spoof")

	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-update-spoof-source")
	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, oldDestinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-update-spoof-old-destination")
	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, newDestinationPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 2100, Y: 2400}, "candidate-route-update-spoof-new-destination")
	seedAutomationRouteForTest(t, gameServer, resolved.PlayerID, routeID, sourcePlanetID, oldDestinationPlanetID, "map_1_1", "map_1_2")

	tests := []struct {
		name  string
		field string
	}{
		{name: "owner", field: `"owner":{"player_id":"spoofed-player"}`},
		{name: "player", field: `"player":{"id":"spoofed-player"}`},
		{name: "session", field: `"session":{"id":"spoofed-session"}`},
		{name: "route payload", field: `"route":{"route_id":"route-spoofed"}`},
		{name: "route list payload", field: `"routes":[]`},
		{name: "source planet", field: `"source_planet_id":"planet-spoofed"`},
		{name: "source map", field: `"source":{"source_map_id":"map_1_1"}`},
		{name: "destination object", field: `"destination":{"type":"planet","id":"planet-spoofed"}`},
		{name: "destination id alias", field: `"destination_id":"planet-spoofed"`},
		{name: "destination map", field: `"destination_map_id":"map_1_2"`},
		{name: "public map key", field: `"config":{"destination":{"public_map_key":"1-2"}}`},
		{name: "enabled", field: `"enabled":false`},
		{name: "settlement", field: `"settlement":{"delivered_amount":999}`},
		{name: "timestamp", field: `"last_calculated_at":999999999`},
		{name: "storage", field: `"storage":{"refined_alloy":999}`},
		{name: "energy", field: `"energy_cost_per_hour":0`},
		{name: "cost", field: `"cost":{"credits":1}`},
		{name: "risk", field: `"route_risk":{"loss_chance":0}`},
		{name: "loss", field: `"loss":{"lost_amount":0}`},
		{name: "cooldown", field: `"cooldown":0`},
		{name: "position", field: `"position":{"x":0,"y":0}`},
		{name: "coordinate scalar", field: `"x":0`},
		{name: "amount alias", field: `"amount":999`},
		{name: "rate alias", field: `"rate_per_hour":999`},
		{name: "resource alias", field: `"resource_id":"x_core"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(resolved.SessionID.String()),
				[]byte(`{"request_id":"request-route-update-spoof-`+strings.ReplaceAll(tt.name, " ", "-")+`","op":"route.update","payload":{"route_id":"`+routeID.String()+`","destination_planet_id":"`+newDestinationPlanetID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":55,`+tt.field+`},"client_seq":1,"v":1}`),
			)
			if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
				t.Fatalf("route.update spoof response = %+v, want invalid payload", response)
			}
			assertRoutesUnchanged(t, gameServer, beforeRoutes, tt.name)
			assertNoQueuedEventsForSessions(t, gameServer, resolved.SessionID)
		})
	}
}

func TestRouteUpdateRejectsXCoreResourceBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "route-update-xcore@example.com", "Route Update XCore")
	sourcePlanetID := foundation.PlanetID("planet-route-update-xcore-source")
	oldDestinationPlanetID := foundation.PlanetID("planet-route-update-xcore-old-destination")
	newDestinationPlanetID := foundation.PlanetID("planet-route-update-xcore-new-destination")
	routeID := foundation.RouteID("route-update-xcore")

	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-update-xcore-source")
	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, oldDestinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-update-xcore-old-destination")
	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, newDestinationPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 2100, Y: 2400}, "candidate-route-update-xcore-new-destination")
	seedAutomationRouteForTest(t, gameServer, resolved.PlayerID, routeID, sourcePlanetID, oldDestinationPlanetID, "map_1_1", "map_1_2")

	beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-route-update-xcore","op":"route.update","payload":{"route_id":"`+routeID.String()+`","destination_planet_id":"`+newDestinationPlanetID.String()+`","resource_item_id":"x_core","amount_per_hour":1},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("route.update x_core response = %+v, want forbidden", response)
	}
	assertRoutesUnchanged(t, gameServer, beforeRoutes, "route.update x_core")
	assertNoQueuedEventsForSessions(t, gameServer, resolved.SessionID)
}

func TestRouteUpdateMapsStoredRouteConfigErrorsToInternal(t *testing.T) {
	for _, err := range []error{
		production.ErrInvalidRouteDestinationID,
		production.ErrInvalidRouteDestinationType,
		production.ErrInvalidRouteEnergyCost,
		production.ErrInvalidRouteRisk,
		production.ErrInvalidRouteDistance,
		production.ErrInvalidRouteMapID,
		production.ErrZeroProductionTimestamp,
		production.ErrInvalidRouteCreateConfig,
		production.ErrInvalidRouteSettlementConfig,
		foundation.ErrEmptyID,
		foundation.ErrInvalidID,
	} {
		t.Run(err.Error(), func(t *testing.T) {
			mapped := domainErrorForRouteUpdate(fmt.Errorf("stored route/config: %w", err))
			domainErr, ok := mapped.(*foundation.DomainError)
			if !ok {
				t.Fatalf("domainErrorForRouteUpdate(%v) = %T, want DomainError", err, mapped)
			}
			if domainErr.Code != foundation.CodeInternal {
				t.Fatalf("domainErrorForRouteUpdate(%v) code = %s, want %s", err, domainErr.Code, foundation.CodeInternal)
			}
		})
	}
}

func TestRouteUpdateMapsInvalidRateToInvalidPayload(t *testing.T) {
	mapped := domainErrorForRouteUpdate(fmt.Errorf("client route update rate: %w", production.ErrInvalidRouteRate))
	domainErr, ok := mapped.(*foundation.DomainError)
	if !ok {
		t.Fatalf("domainErrorForRouteUpdate(invalid rate) = %T, want DomainError", mapped)
	}
	if domainErr.Code != foundation.CodeInvalidPayload {
		t.Fatalf("domainErrorForRouteUpdate(invalid rate) code = %s, want %s", domainErr.Code, foundation.CodeInvalidPayload)
	}
}

func assertRouteUpdateResponse(
	t *testing.T,
	response realtime.CachedResponse,
	routeID foundation.RouteID,
	sourcePlanetID foundation.PlanetID,
	destinationPlanetID foundation.PlanetID,
	fromPublicMapKey string,
	toPublicMapKey string,
	resourceItemID foundation.ItemID,
	amountPerHour int64,
	enabled bool,
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
	assertRouteUpdatePayload(t, payload.Route, routeID, sourcePlanetID, destinationPlanetID, fromPublicMapKey, toPublicMapKey, resourceItemID, amountPerHour, enabled, label)
	if len(payload.Routes.Routes) != 1 {
		t.Fatalf("%s route list = %+v, want one route", label, payload.Routes.Routes)
	}
	assertRouteUpdatePayload(t, payload.Routes.Routes[0], routeID, sourcePlanetID, destinationPlanetID, fromPublicMapKey, toPublicMapKey, resourceItemID, amountPerHour, enabled, label+" list")
}

func assertRouteUpdatePayload(
	t *testing.T,
	payload routePayload,
	routeID foundation.RouteID,
	sourcePlanetID foundation.PlanetID,
	destinationPlanetID foundation.PlanetID,
	fromPublicMapKey string,
	toPublicMapKey string,
	resourceItemID foundation.ItemID,
	amountPerHour int64,
	enabled bool,
	label string,
) {
	t.Helper()
	assertRoutePayloadMapKeys(t, payload, routeID, fromPublicMapKey, toPublicMapKey)
	if payload.SourcePlanetID != sourcePlanetID.String() ||
		payload.Destination.Type != production.RouteDestinationTypePlanet.String() ||
		payload.Destination.ID != destinationPlanetID.String() ||
		payload.ResourceItemID != resourceItemID.String() ||
		payload.AmountPerHour != amountPerHour ||
		payload.Enabled != enabled {
		t.Fatalf("%s route payload = %+v, want source %q destination %q resource %q amount %d enabled %v", label, payload, sourcePlanetID, destinationPlanetID, resourceItemID, amountPerHour, enabled)
	}
}

func assertRouteUpdateEvents(
	t *testing.T,
	events []realtime.EventEnvelope,
	routeID foundation.RouteID,
	destinationPlanetID foundation.PlanetID,
	fromPublicMapKey string,
	toPublicMapKey string,
	resourceItemID foundation.ItemID,
	amountPerHour int64,
	enabled bool,
	label string,
) {
	t.Helper()
	requireEventTypeForTest(t, events, realtime.EventRouteUpdated)
	requireEventTypeForTest(t, events, realtime.EventRouteList)
	requireEventTypeForTest(t, events, realtime.EventRouteSnapshot)
	if len(events) != 3 {
		t.Fatalf("%s events = %+v, want only route.updated, route.snapshot, and route.list", label, events)
	}
	for _, event := range events {
		assertPayloadOmitsInternalMapIdentity(t, string(event.Type)+" event", event.Payload)
		switch event.Type {
		case realtime.EventRouteUpdated, realtime.EventRouteSnapshot:
			var payload struct {
				Route routePayload `json:"route"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode %s event payload: %v", event.Type, err)
			}
			if payload.Route.Destination.ID != destinationPlanetID.String() ||
				payload.Route.ResourceItemID != resourceItemID.String() ||
				payload.Route.AmountPerHour != amountPerHour ||
				payload.Route.Enabled != enabled {
				t.Fatalf("%s %s route = %+v, want updated terms", label, event.Type, payload.Route)
			}
			assertRoutePayloadMapKeys(t, payload.Route, routeID, fromPublicMapKey, toPublicMapKey)
		case realtime.EventRouteList:
			var payload struct {
				Routes routeListPayload `json:"routes"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode route.list event payload: %v", err)
			}
			if len(payload.Routes.Routes) != 1 {
				t.Fatalf("%s route.list event routes = %+v, want one route", label, payload.Routes.Routes)
			}
			listRoute := payload.Routes.Routes[0]
			if listRoute.Destination.ID != destinationPlanetID.String() ||
				listRoute.ResourceItemID != resourceItemID.String() ||
				listRoute.AmountPerHour != amountPerHour ||
				listRoute.Enabled != enabled {
				t.Fatalf("%s route.list route = %+v, want updated terms", label, listRoute)
			}
			assertRoutePayloadMapKeys(t, listRoute, routeID, fromPublicMapKey, toPublicMapKey)
		}
	}
}

func assertRouteUpdateSettlementEvents(
	t *testing.T,
	events []realtime.EventEnvelope,
	routeID foundation.RouteID,
	destinationPlanetID foundation.PlanetID,
	fromPublicMapKey string,
	toPublicMapKey string,
	activeMapPlanetID foundation.PlanetID,
	wantQuantity int64,
) {
	t.Helper()
	requireEventTypeForTest(t, events, realtime.EventRouteUpdated)
	requireEventTypeForTest(t, events, realtime.EventRouteList)
	requireEventTypeForTest(t, events, realtime.EventRouteSnapshot)
	requireEventTypeForTest(t, events, realtime.EventProductionSummary)
	requireEventTypeForTest(t, events, realtime.EventPlanetStorage)
	if len(events) != 5 {
		t.Fatalf("route.update settlement events = %+v, want route + production/storage snapshots only", events)
	}
	for _, event := range events {
		assertPayloadOmitsInternalMapIdentity(t, string(event.Type)+" event", event.Payload)
		switch event.Type {
		case realtime.EventRouteUpdated, realtime.EventRouteSnapshot:
			var payload struct {
				Route routePayload `json:"route"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode %s event payload: %v", event.Type, err)
			}
			assertRoutePayloadMapKeys(t, payload.Route, routeID, fromPublicMapKey, toPublicMapKey)
			if payload.Route.Destination.ID != destinationPlanetID.String() || !payload.Route.Enabled {
				t.Fatalf("%s route = %+v, want updated enabled route", event.Type, payload.Route)
			}
		case realtime.EventRouteList:
			var payload struct {
				Routes routeListPayload `json:"routes"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode route.list event payload: %v", err)
			}
			if len(payload.Routes.Routes) != 1 {
				t.Fatalf("route.list event routes = %+v, want one route", payload.Routes.Routes)
			}
			assertRoutePayloadMapKeys(t, payload.Routes.Routes[0], routeID, fromPublicMapKey, toPublicMapKey)
			if payload.Routes.Routes[0].Destination.ID != destinationPlanetID.String() || !payload.Routes.Routes[0].Enabled {
				t.Fatalf("route.list route = %+v, want updated enabled route", payload.Routes.Routes[0])
			}
		case realtime.EventProductionSummary:
			var payload planetProductionCollectionPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode production summary event payload: %v", err)
			}
			assertRouteUpdateProductionSnapshot(t, payload, activeMapPlanetID, wantQuantity)
		case realtime.EventPlanetStorage:
			var payload planetStorageCollectionPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode planet storage event payload: %v", err)
			}
			assertRouteUpdateStorageSnapshot(t, payload, activeMapPlanetID, wantQuantity)
		}
	}
}

func assertRouteUpdateActiveMapSnapshots(t *testing.T, productionPayload planetProductionCollectionPayload, storagePayload planetStorageCollectionPayload, planetID foundation.PlanetID, wantQuantity int64) {
	t.Helper()
	assertRouteUpdateProductionSnapshot(t, productionPayload, planetID, wantQuantity)
	assertRouteUpdateStorageSnapshot(t, storagePayload, planetID, wantQuantity)
}

func assertRouteUpdateProductionSnapshot(t *testing.T, payload planetProductionCollectionPayload, planetID foundation.PlanetID, wantQuantity int64) {
	t.Helper()
	for _, planet := range payload.Planets {
		if planet.PlanetID != planetID.String() {
			continue
		}
		if planet.PublicMapKey != "1-1" {
			t.Fatalf("production planet = %+v, want public map 1-1", planet)
		}
		if got := routeControlStorageQuantity(planet.Storage, "refined_alloy"); got != wantQuantity {
			t.Fatalf("production storage refined_alloy = %d, want %d", got, wantQuantity)
		}
		return
	}
	t.Fatalf("production planets = %+v, want active-map planet %q", payload.Planets, planetID)
}

func assertRouteUpdateStorageSnapshot(t *testing.T, payload planetStorageCollectionPayload, planetID foundation.PlanetID, wantQuantity int64) {
	t.Helper()
	for _, storage := range payload.Planets {
		if storage.PlanetID != planetID.String() {
			continue
		}
		if storage.PublicMapKey != "1-1" {
			t.Fatalf("storage planet = %+v, want public map 1-1", storage)
		}
		if got := routeControlStorageQuantity(storage, "refined_alloy"); got != wantQuantity {
			t.Fatalf("storage refined_alloy = %d, want %d", got, wantQuantity)
		}
		return
	}
	t.Fatalf("storage planets = %+v, want active-map planet %q", payload.Planets, planetID)
}

func seedAutomationRouteForTestWithResource(
	t *testing.T,
	gameServer *Server,
	ownerID foundation.PlayerID,
	routeID foundation.RouteID,
	sourcePlanetID foundation.PlanetID,
	destinationPlanetID foundation.PlanetID,
	sourceMapID production.RouteMapID,
	destinationMapID production.RouteMapID,
	resourceItemID foundation.ItemID,
	amountPerHour int64,
) {
	t.Helper()
	service, err := production.NewAutomationRouteService(production.AutomationRouteServiceConfig{
		Store:  gameServer.runtime.Production,
		Clock:  gameServer.runtime.clock,
		Policy: mapAwareRoutePolicyForTest{sourceMapID: sourceMapID, destinationMapID: destinationMapID},
	})
	if err != nil {
		t.Fatalf("NewAutomationRouteService() error = %v, want nil", err)
	}
	destination, err := production.NewPlanetRouteDestination(destinationPlanetID)
	if err != nil {
		t.Fatalf("NewPlanetRouteDestination(%q) error = %v, want nil", destinationPlanetID, err)
	}
	result, err := service.CreateRoute(production.CreateRouteInput{
		RouteID:        routeID,
		OwnerPlayerID:  ownerID,
		SourcePlanetID: sourcePlanetID,
		Destination:    destination,
		ResourceItemID: resourceItemID,
		AmountPerHour:  amountPerHour,
	})
	if err != nil {
		t.Fatalf("CreateRoute(%q) error = %v, want nil", routeID, err)
	}
	if result.Route.SourceMapID != sourceMapID || result.Route.DestinationMapID != destinationMapID {
		t.Fatalf("seeded route map ids = %q/%q, want %q/%q", result.Route.SourceMapID, result.Route.DestinationMapID, sourceMapID, destinationMapID)
	}
}
