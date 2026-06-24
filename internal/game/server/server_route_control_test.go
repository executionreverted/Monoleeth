package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestRouteControlDisableThenEnableThroughGatewayQueuesSafeOwnerEvents(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "route-control-owner@example.com", "Route Control Owner")
	other := createResolvedRuntimeSession(t, gameServer, "route-control-other@example.com", "Route Control Other")
	sourcePlanetID := foundation.PlanetID("planet-route-control-source")
	destinationPlanetID := foundation.PlanetID("planet-route-control-destination")
	routeID := foundation.RouteID("route-control-map-1-1-to-1-2")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-control-source")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-control-destination")
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")

	disableResponse := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-disable-owned","op":"route.disable","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	assertRouteControlResponse(t, disableResponse, routeID, "1-1", "1-2", false, "route.disable")
	assertStoredRouteEnabled(t, gameServer, routeID, false)
	assertStoredRouteEnergyReserved(t, gameServer, sourcePlanetID, 0)

	disableEventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteDisable, owner.PlayerID)
	if err != nil {
		t.Fatalf("post route.disable events: %v", err)
	}
	if _, leaked := disableEventsBySession[other.SessionID]; leaked {
		t.Fatalf("route.disable events leaked to non-owner session: %+v", disableEventsBySession[other.SessionID])
	}
	assertRouteControlEvents(t, disableEventsBySession[owner.SessionID], routeID, "1-1", "1-2", false, "route.disable")

	enableResponse := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-enable-owned","op":"route.enable","payload":{"route_id":"`+routeID.String()+`"},"client_seq":2,"v":1}`),
	)
	assertRouteControlResponse(t, enableResponse, routeID, "1-1", "1-2", true, "route.enable")
	assertStoredRouteEnabled(t, gameServer, routeID, true)
	assertStoredRouteEnergyReserved(t, gameServer, sourcePlanetID, 4)

	enableEventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteEnable, owner.PlayerID)
	if err != nil {
		t.Fatalf("post route.enable events: %v", err)
	}
	if _, leaked := enableEventsBySession[other.SessionID]; leaked {
		t.Fatalf("route.enable events leaked to non-owner session: %+v", enableEventsBySession[other.SessionID])
	}
	assertRouteControlEvents(t, enableEventsBySession[owner.SessionID], routeID, "1-1", "1-2", true, "route.enable")
}

func TestRouteDisableSettlesStorageAndQueuesSafeProductionStorageSnapshots(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-disable-settle-owner@example.com", "Route Settle Owner")
	other := createResolvedRuntimeSession(t, gameServer, "route-disable-settle-other@example.com", "Route Settle Other")
	sourcePlanetID := foundation.PlanetID("planet-route-disable-settle-source")
	destinationPlanetID := foundation.PlanetID("planet-route-disable-settle-destination")
	routeID := foundation.RouteID("route-disable-settle")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-disable-settle-source")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-disable-settle-destination")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	clock.Advance(time.Hour)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-disable-settle","op":"route.disable","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("route.disable settlement response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsInternalMapIdentity(t, "route.disable settlement response", response.Response.Payload)
	var payload struct {
		Route      routePayload                      `json:"route"`
		Routes     routeListPayload                  `json:"routes"`
		Production planetProductionCollectionPayload `json:"production"`
		Storage    planetStorageCollectionPayload    `json:"storage"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode route.disable settlement payload: %v", err)
	}
	assertRoutePayloadMapKeys(t, payload.Route, routeID, "1-1", "1-2")
	if payload.Route.Enabled {
		t.Fatalf("route.disable settlement route enabled = true, want false")
	}
	assertRouteControlActiveMapSnapshots(t, payload.Production, payload.Storage, sourcePlanetID, 60)
	assertStoredRouteEnabled(t, gameServer, routeID, false)
	assertStoredRouteEnergyReserved(t, gameServer, sourcePlanetID, 0)
	assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 60)
	assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "refined_alloy", 40)

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteDisable, owner.PlayerID)
	if err != nil {
		t.Fatalf("post route.disable settlement events: %v", err)
	}
	if _, leaked := eventsBySession[other.SessionID]; leaked {
		t.Fatalf("route.disable settlement events leaked to non-owner session: %+v", eventsBySession[other.SessionID])
	}
	assertRouteDisableSettlementEvents(t, eventsBySession[owner.SessionID], routeID, "1-1", "1-2", sourcePlanetID, 60)
}

func TestRouteControlRejectsWrongOwnerWithoutMutationOrEvents(t *testing.T) {
	tests := []struct {
		name             string
		op               string
		prepareDisabled  bool
		wantEnabledAfter bool
	}{
		{name: "enable", op: "route.enable", prepareDisabled: true, wantEnabledAfter: false},
		{name: "disable", op: "route.disable", wantEnabledAfter: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gameServer, _ := newTestServer(t, false)
			owner := createResolvedRuntimeSession(t, gameServer, "route-control-owner-"+tt.name+"@example.com", "Route Owner")
			other := createResolvedRuntimeSession(t, gameServer, "route-control-wrong-owner-"+tt.name+"@example.com", "Route Other")
			sourcePlanetID := foundation.PlanetID("planet-route-control-wrong-owner-source-" + tt.name)
			destinationPlanetID := foundation.PlanetID("planet-route-control-wrong-owner-destination-" + tt.name)
			routeID := foundation.RouteID("route-control-wrong-owner-" + tt.name)

			seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, discovery.PlanetMaterializationKey("candidate-route-control-wrong-owner-source-"+tt.name))
			seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, discovery.PlanetMaterializationKey("candidate-route-control-wrong-owner-destination-"+tt.name))
			seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
			if tt.prepareDisabled {
				service := routeServiceForTest(t, gameServer)
				if _, err := service.DisableRouteForOwner(owner.PlayerID, routeID); err != nil {
					t.Fatalf("DisableRouteForOwner setup error = %v, want nil", err)
				}
			}

			beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(other.SessionID.String()),
				[]byte(`{"request_id":"request-route-control-wrong-owner-`+tt.name+`","op":"`+tt.op+`","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
			)
			if !response.HasError || (response.Error.Error.Code != foundation.CodeNotFound && response.Error.Error.Code != foundation.CodeForbidden) {
				t.Fatalf("%s wrong-owner response = %+v, want safe not-found/forbidden", tt.op, response)
			}
			assertRoutesUnchanged(t, gameServer, beforeRoutes, tt.op)
			assertStoredRouteEnabled(t, gameServer, routeID, tt.wantEnabledAfter)
			assertNoQueuedEventsForSessions(t, gameServer, owner.SessionID, other.SessionID)
		})
	}
}

func TestRouteControlRejectsSpoofedServerOwnedFieldsBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "route-control-spoof@example.com", "Route Control Spoof")
	sourcePlanetID := foundation.PlanetID("planet-route-control-spoof-source")
	destinationPlanetID := foundation.PlanetID("planet-route-control-spoof-destination")
	routeID := foundation.RouteID("route-control-spoof")

	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-control-spoof-source")
	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-control-spoof-destination")
	seedAutomationRouteForTest(t, gameServer, resolved.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")

	tests := []struct {
		name  string
		op    string
		field string
	}{
		{name: "owner player", op: "route.enable", field: `"owner_player_id":"spoofed-player"`},
		{name: "session", op: "route.disable", field: `"session":{"id":"spoofed-session"}`},
		{name: "enabled", op: "route.enable", field: `"enabled":true`},
		{name: "settlement", op: "route.disable", field: `"settlement":{"delivered_amount":999}`},
		{name: "source planet", op: "route.enable", field: `"source_planet_id":"planet-spoofed"`},
		{name: "destination fact", op: "route.disable", field: `"destination":{"type":"planet","id":"planet-spoofed"}`},
		{name: "timestamp", op: "route.enable", field: `"last_calculated_at":999999999`},
		{name: "storage", op: "route.disable", field: `"storage":{"refined_alloy":999}`},
		{name: "energy", op: "route.enable", field: `"energy_cost_per_hour":0`},
		{name: "risk", op: "route.disable", field: `"route_risk":{"loss_chance":0}`},
		{name: "rate", op: "route.enable", field: `"amount_per_hour":999999`},
		{name: "resource", op: "route.disable", field: `"resource_item_id":"x_core"`},
		{name: "cooldown", op: "route.enable", field: `"cooldown":0`},
		{name: "position", op: "route.disable", field: `"position":{"x":0,"y":0}`},
		{name: "nested internal map", op: "route.enable", field: `"config":{"source_map_id":"map_1_1"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(resolved.SessionID.String()),
				[]byte(`{"request_id":"request-route-control-spoof-`+strings.ReplaceAll(tt.name, " ", "-")+`","op":"`+tt.op+`","payload":{"route_id":"`+routeID.String()+`",`+tt.field+`},"client_seq":1,"v":1}`),
			)
			if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
				t.Fatalf("%s spoof response = %+v, want invalid payload", tt.op, response)
			}
			assertRoutesUnchanged(t, gameServer, beforeRoutes, tt.name)
			assertNoQueuedEventsForSessions(t, gameServer, resolved.SessionID)
		})
	}
}

func TestRouteControlMapsStoredRouteConfigErrorsToInternal(t *testing.T) {
	for _, err := range []error{
		production.ErrInvalidRouteCreateConfig,
		production.ErrInvalidRouteSettlementConfig,
		production.ErrInvalidRouteDestinationID,
		production.ErrInvalidRouteDestinationType,
		production.ErrInvalidRouteEnergyCost,
		production.ErrInvalidRouteRisk,
		production.ErrInvalidRouteDistance,
		production.ErrInvalidRouteMapID,
		production.ErrZeroProductionTimestamp,
		foundation.ErrEmptyID,
		foundation.ErrInvalidID,
	} {
		t.Run(err.Error(), func(t *testing.T) {
			mapped := domainErrorForRouteControl(fmt.Errorf("stored route/config: %w", err))
			domainErr, ok := mapped.(*foundation.DomainError)
			if !ok {
				t.Fatalf("domainErrorForRouteControl(%v) = %T, want DomainError", err, mapped)
			}
			if domainErr.Code != foundation.CodeInternal {
				t.Fatalf("domainErrorForRouteControl(%v) code = %s, want %s", err, domainErr.Code, foundation.CodeInternal)
			}
		})
	}
}

func assertRouteControlResponse(t *testing.T, response realtime.CachedResponse, routeID foundation.RouteID, fromPublicMapKey string, toPublicMapKey string, enabled bool, label string) {
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
	if payload.Route.Enabled != enabled {
		t.Fatalf("%s route enabled = %v, want %v", label, payload.Route.Enabled, enabled)
	}
	if len(payload.Routes.Routes) != 1 {
		t.Fatalf("%s route list = %+v, want one route", label, payload.Routes.Routes)
	}
	assertRoutePayloadMapKeys(t, payload.Routes.Routes[0], routeID, fromPublicMapKey, toPublicMapKey)
	if payload.Routes.Routes[0].Enabled != enabled {
		t.Fatalf("%s route list enabled = %v, want %v", label, payload.Routes.Routes[0].Enabled, enabled)
	}
}

func assertRouteControlEvents(t *testing.T, events []realtime.EventEnvelope, routeID foundation.RouteID, fromPublicMapKey string, toPublicMapKey string, enabled bool, label string) {
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
			assertRoutePayloadMapKeys(t, payload.Route, routeID, fromPublicMapKey, toPublicMapKey)
			if payload.Route.Enabled != enabled {
				t.Fatalf("%s %s route enabled = %v, want %v", label, event.Type, payload.Route.Enabled, enabled)
			}
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
			assertRoutePayloadMapKeys(t, payload.Routes.Routes[0], routeID, fromPublicMapKey, toPublicMapKey)
			if payload.Routes.Routes[0].Enabled != enabled {
				t.Fatalf("%s route.list enabled = %v, want %v", label, payload.Routes.Routes[0].Enabled, enabled)
			}
		}
	}
}

func assertRouteDisableSettlementEvents(t *testing.T, events []realtime.EventEnvelope, routeID foundation.RouteID, fromPublicMapKey string, toPublicMapKey string, planetID foundation.PlanetID, wantQuantity int64) {
	t.Helper()
	requireEventTypeForTest(t, events, realtime.EventRouteUpdated)
	requireEventTypeForTest(t, events, realtime.EventRouteList)
	requireEventTypeForTest(t, events, realtime.EventRouteSnapshot)
	requireEventTypeForTest(t, events, realtime.EventProductionSummary)
	requireEventTypeForTest(t, events, realtime.EventPlanetStorage)
	if len(events) != 5 {
		t.Fatalf("route.disable settlement events = %+v, want route + production/storage snapshots only", events)
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
			if payload.Route.Enabled {
				t.Fatalf("%s route enabled = true, want false", event.Type)
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
			if payload.Routes.Routes[0].Enabled {
				t.Fatalf("route.list event enabled = true, want false")
			}
		case realtime.EventProductionSummary:
			var payload planetProductionCollectionPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode production summary event payload: %v", err)
			}
			assertRouteControlProductionSnapshot(t, payload, planetID, wantQuantity)
		case realtime.EventPlanetStorage:
			var payload planetStorageCollectionPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode planet storage event payload: %v", err)
			}
			assertRouteControlStorageSnapshot(t, payload, planetID, wantQuantity)
		}
	}
}

func assertRouteControlActiveMapSnapshots(t *testing.T, productionPayload planetProductionCollectionPayload, storagePayload planetStorageCollectionPayload, planetID foundation.PlanetID, wantQuantity int64) {
	t.Helper()
	assertRouteControlProductionSnapshot(t, productionPayload, planetID, wantQuantity)
	assertRouteControlStorageSnapshot(t, storagePayload, planetID, wantQuantity)
}

func assertRouteControlProductionSnapshot(t *testing.T, payload planetProductionCollectionPayload, planetID foundation.PlanetID, wantQuantity int64) {
	t.Helper()
	if len(payload.Planets) != 1 {
		t.Fatalf("production planets = %+v, want one active-map planet", payload.Planets)
	}
	planet := payload.Planets[0]
	if planet.PlanetID != planetID.String() || planet.PublicMapKey != "1-1" {
		t.Fatalf("production planet = %+v, want %q on public map 1-1", planet, planetID)
	}
	if got := routeControlStorageQuantity(planet.Storage, "refined_alloy"); got != wantQuantity {
		t.Fatalf("production storage refined_alloy = %d, want %d", got, wantQuantity)
	}
}

func assertRouteControlStorageSnapshot(t *testing.T, payload planetStorageCollectionPayload, planetID foundation.PlanetID, wantQuantity int64) {
	t.Helper()
	if len(payload.Planets) != 1 {
		t.Fatalf("storage planets = %+v, want one active-map planet", payload.Planets)
	}
	storage := payload.Planets[0]
	if storage.PlanetID != planetID.String() || storage.PublicMapKey != "1-1" {
		t.Fatalf("storage planet = %+v, want %q on public map 1-1", storage, planetID)
	}
	if got := routeControlStorageQuantity(storage, "refined_alloy"); got != wantQuantity {
		t.Fatalf("storage refined_alloy = %d, want %d", got, wantQuantity)
	}
}

func assertStoredRouteEnabled(t *testing.T, gameServer *Server, routeID foundation.RouteID, enabled bool) {
	t.Helper()
	stored, ok, err := gameServer.runtime.Production.AutomationRoute(routeID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok=%v err=%v, want stored route", routeID, ok, err)
	}
	if stored.Enabled != enabled {
		t.Fatalf("stored route enabled = %v, want %v", stored.Enabled, enabled)
	}
}

func assertStoredRouteEnergyReserved(t *testing.T, gameServer *Server, planetID foundation.PlanetID, want int64) {
	t.Helper()
	state, ok, err := gameServer.runtime.Production.ProductionState(planetID)
	if err != nil || !ok {
		t.Fatalf("ProductionState(%q) ok=%v err=%v, want stored state", planetID, ok, err)
	}
	if state.EnergyReservedPerHour != want {
		t.Fatalf("ProductionState(%q) EnergyReservedPerHour = %d, want %d", planetID, state.EnergyReservedPerHour, want)
	}
}

func assertRoutesUnchanged(t *testing.T, gameServer *Server, beforeRoutes []production.AutomationRoute, label string) {
	t.Helper()
	afterRoutes := gameServer.runtime.Production.AutomationRoutes()
	if len(afterRoutes) != len(beforeRoutes) {
		t.Fatalf("%s routes after rejection = %+v, want unchanged count %d", label, afterRoutes, len(beforeRoutes))
	}
	for index := range afterRoutes {
		if afterRoutes[index] != beforeRoutes[index] {
			t.Fatalf("%s routes after rejection = %+v, want unchanged %+v", label, afterRoutes, beforeRoutes)
		}
	}
}

func assertNoQueuedEventsForSessions(t *testing.T, gameServer *Server, sessionIDs ...auth.SessionID) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	for _, sessionID := range sessionIDs {
		if queued := len(gameServer.runtime.queuedEvents[sessionID]); queued != 0 {
			t.Fatalf("session %q queued events = %d, want none", sessionID, queued)
		}
	}
}

func assertStoredRouteStorageQuantity(t *testing.T, gameServer *Server, planetID foundation.PlanetID, itemID foundation.ItemID, want int64) {
	t.Helper()
	storage, ok, err := gameServer.runtime.Production.PlanetStorage(planetID)
	if err != nil || !ok {
		t.Fatalf("PlanetStorage(%q) ok=%v err=%v, want stored storage", planetID, ok, err)
	}
	if got := storage.QuantityOf(itemID); got != want {
		t.Fatalf("stored storage %q %q quantity = %d, want %d", planetID, itemID, got, want)
	}
}

func routeControlStorageQuantity(storage planetStoragePayload, itemID string) int64 {
	for _, item := range storage.Items {
		if item.ItemID == itemID {
			return item.Quantity
		}
	}
	return 0
}

func saveRouteControlStorage(t *testing.T, gameServer *Server, planetID foundation.PlanetID, items []production.StoredItem) {
	t.Helper()
	storage, err := production.NewPlanetStorage(planetID, 250, items, gameServer.runtime.clock.Now().UTC())
	if err != nil {
		t.Fatalf("NewPlanetStorage(%q) error = %v, want nil", planetID, err)
	}
	if err := gameServer.runtime.Production.SavePlanetStorage(storage); err != nil {
		t.Fatalf("SavePlanetStorage(%q) error = %v, want nil", planetID, err)
	}
}

func routeServiceForTest(t *testing.T, gameServer *Server) *production.AutomationRouteService {
	t.Helper()
	service, err := production.NewAutomationRouteService(production.AutomationRouteServiceConfig{
		Store:  gameServer.runtime.Production,
		Clock:  gameServer.runtime.clock,
		Policy: mapAwareRoutePolicyForTest{sourceMapID: "map_1_1", destinationMapID: "map_1_2"},
	})
	if err != nil {
		t.Fatalf("NewAutomationRouteService() error = %v, want nil", err)
	}
	return service
}

func newRouteControlTestServer(t *testing.T, clock foundation.Clock) *Server {
	t.Helper()
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		SessionTTL:        24 * time.Hour,
		TickDelta:         50 * time.Millisecond,
		ContentRepository: staticContentRepositoryForTest(),
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
		Clock:             clock,
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	return gameServer
}
