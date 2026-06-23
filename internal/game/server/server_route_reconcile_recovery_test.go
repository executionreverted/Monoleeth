package server

import (
	"encoding/json"
	"testing"
	"time"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
)

func TestRouteSettleOwnerReconcileUsesDurableRoutesAfterLiveReadModelLoss(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-reconcile-recovery@example.com", "Route Reconcile Recovery")
	sourcePlanetID := foundation.PlanetID("planet-route-reconcile-recovery-source")
	destination := production.RouteDestination{
		Type: production.RouteDestinationTypeStorage,
		ID:   "storage-route-reconcile-recovery-destination",
	}
	destinationStorageID := foundation.PlanetID(destination.ID)
	routeID := foundation.RouteID("route-reconcile-recovery")

	seedOwnedProductionPlanetForTest(
		t,
		gameServer,
		owner.PlayerID,
		sourcePlanetID,
		gameServer.runtime.zoneID,
		world.Vec2{X: 1300, Y: 1400},
		discovery.PlanetMaterializationKey("candidate-route-reconcile-recovery-source"),
	)
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	saveRouteControlStorage(t, gameServer, destinationStorageID, nil)
	seedAutomationRouteToDestinationForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destination, "map_1_1", "map_1_1")
	clock.Advance(time.Hour)

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-reconcile-recovery-first","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if first.HasError {
		t.Fatalf("first route.settle response error = %+v, want success", first.Error)
	}
	if err := gameServer.runtime.Production.DropAutomationRouteReadModel(routeID); err != nil {
		t.Fatalf("DropAutomationRouteReadModel() error = %v, want nil", err)
	}
	if _, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteSettle, owner.PlayerID); err != nil {
		t.Fatalf("drain first route.settle events: %v", err)
	}

	reconcile := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-reconcile-recovery-all","op":"route.settle","payload":{},"client_seq":2,"v":1}`),
	)
	if reconcile.HasError {
		t.Fatalf("owner reconcile response error = %+v, want durable fallback success", reconcile.Error)
	}
	assertPayloadOmitsRouteEndpointID(t, "route.settle reconcile response", reconcile.Response.Payload, destination.ID.String())

	var payload struct {
		Routes      routeListPayload         `json:"routes"`
		Settlements []routeSettlementPayload `json:"settlements"`
	}
	if err := json.Unmarshal(reconcile.Response.Payload, &payload); err != nil {
		t.Fatalf("decode owner reconcile payload: %v", err)
	}
	if len(payload.Routes.Routes) != 1 || payload.Routes.Routes[0].RouteID != routeID.String() {
		t.Fatalf("owner reconcile route list = %+v, want recovered durable route %q", payload.Routes.Routes, routeID)
	}
	if len(payload.Settlements) != 1 {
		t.Fatalf("owner reconcile settlements = %+v, want one no-op durable route settlement", payload.Settlements)
	}
	assertRouteSettlementPayload(t, payload.Settlements[0], routeID, "refined_alloy", 0, 0, 0, 0, 0, 0, true, "owner reconcile settlement")
	assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 60)
	assertStoredRouteStorageQuantity(t, gameServer, destinationStorageID, "refined_alloy", 40)

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteSettle, owner.PlayerID)
	if err != nil {
		t.Fatalf("post owner reconcile events: %v", err)
	}
	assertRouteReconcileStorageNoOpEvents(t, routeSettleEventSuffixForTest(t, eventsBySession[owner.SessionID], 4), routeID, owner.PlayerID)
}

func assertRouteReconcileStorageNoOpEvents(
	t *testing.T,
	events []realtime.EventEnvelope,
	routeID foundation.RouteID,
	ownerID foundation.PlayerID,
) {
	t.Helper()
	requireEventTypeForTest(t, events, realtime.EventRouteSettled)
	requireEventTypeForTest(t, events, realtime.EventRouteUpdated)
	requireEventTypeForTest(t, events, realtime.EventRouteSnapshot)
	requireEventTypeForTest(t, events, realtime.EventRouteList)
	if len(events) != 4 {
		t.Fatalf("owner reconcile events = %+v, want route.settled/updated/snapshot/list only", events)
	}
	for _, event := range events {
		assertPayloadOmitsInternalMapIdentity(t, string(event.Type)+" event", event.Payload)
		assertPayloadOmitsPlayerOwner(t, string(event.Type)+" event", event.Payload, ownerID)
	}

	settled := requireEventTypeForTest(t, events, realtime.EventRouteSettled)
	var payload struct {
		Route      routePayload           `json:"route"`
		Settlement routeSettlementPayload `json:"settlement"`
	}
	if err := json.Unmarshal(settled.Payload, &payload); err != nil {
		t.Fatalf("decode owner reconcile route.settled event: %v", err)
	}
	assertRoutePayloadMapKeys(t, payload.Route, routeID, "1-1", "1-1")
	if payload.Route.Destination.Type != production.RouteDestinationTypeStorage.String() || payload.Route.Destination.ID != "" {
		t.Fatalf("owner reconcile route destination = %+v, want public storage type without internal id", payload.Route.Destination)
	}
	assertRouteSettlementPayload(t, payload.Settlement, routeID, "refined_alloy", 0, 0, 0, 0, 0, 0, true, "owner reconcile route.settled event")

	list := requireEventTypeForTest(t, events, realtime.EventRouteList)
	var listPayload struct {
		Routes routeListPayload `json:"routes"`
	}
	if err := json.Unmarshal(list.Payload, &listPayload); err != nil {
		t.Fatalf("decode owner reconcile route.list event: %v", err)
	}
	if len(listPayload.Routes.Routes) != 1 || listPayload.Routes.Routes[0].RouteID != routeID.String() {
		t.Fatalf("owner reconcile route.list event routes = %+v, want route %q", listPayload.Routes.Routes, routeID)
	}
}
