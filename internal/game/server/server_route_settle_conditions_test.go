package server

import (
	"encoding/json"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestRouteSettleSourceEmptyReturnsSafePayloadAndDurableRows(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-source-empty@example.com", "Route Settle Source Empty")
	sourcePlanetID := foundation.PlanetID("planet-route-settle-source-empty-a")
	destinationPlanetID := foundation.PlanetID("planet-route-settle-source-empty-b")
	routeID := foundation.RouteID("route-settle-source-empty")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-source-empty-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-source-empty-b")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, nil)
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	settledAt := clock.Advance(time.Hour)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-source-empty","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("route.settle source-empty response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsInternalMapIdentity(t, "route.settle source-empty response", response.Response.Payload)
	assertPayloadOmitsPlayerOwner(t, "route.settle source-empty response", response.Response.Payload, owner.PlayerID)
	var payload struct {
		Route      routePayload           `json:"route"`
		Settlement routeSettlementPayload `json:"settlement"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode route.settle source-empty payload: %v", err)
	}
	assertRoutePayloadMapKeys(t, payload.Route, routeID, "1-1", "1-2")
	assertRouteSettlementPayload(t, payload.Settlement, routeID, "refined_alloy", 3_600_000, 40, 0, 0, 0, 0, false, "source-empty route.settle response")
	if !payload.Settlement.SourceEmpty {
		t.Fatalf("source-empty route.settle source_empty = false, want true")
	}
	if payload.Settlement.DestinationFull || payload.Settlement.LossApplied {
		t.Fatalf("source-empty route.settle flags = %+v, want no destination_full/loss_applied", payload.Settlement)
	}
	assertRouteSettlementPayloadOmitsServerTruth(t, payload.Settlement, owner.PlayerID, "source-empty route.settle settlement")
	assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 0)
	assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "refined_alloy", 0)
	assertStoredRouteCursor(t, gameServer, routeID, settledAt)
	assertRouteDurableSettlementRows(t, gameServer, []foundation.RouteID{routeID}, 0)
	assertRouteSettlementOutboxEventTypes(t, gameServer, production.EventRouteSourceEmpty, production.EventRouteTransferSettled)

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteSettle, owner.PlayerID)
	if err != nil {
		t.Fatalf("post route.settle source-empty events: %v", err)
	}
	settled := requireEventTypeForTest(t, eventsBySession[owner.SessionID], realtime.EventRouteSettled)
	var eventPayload struct {
		Settlement routeSettlementPayload `json:"settlement"`
	}
	if err := json.Unmarshal(settled.Payload, &eventPayload); err != nil {
		t.Fatalf("decode source-empty route.settled event payload: %v", err)
	}
	if !eventPayload.Settlement.SourceEmpty || eventPayload.Settlement.DestinationFull || eventPayload.Settlement.LossApplied {
		t.Fatalf("source-empty route.settled event settlement = %+v, want source_empty only", eventPayload.Settlement)
	}
	assertRouteSettlementPayloadOmitsServerTruth(t, eventPayload.Settlement, owner.PlayerID, "source-empty route.settled event settlement")
}

func assertRouteSettlementOutboxEventTypes(t *testing.T, gameServer *Server, want ...string) {
	t.Helper()
	outbox := gameServer.runtime.Settlements.OutboxRecords()
	if len(outbox) != len(want) {
		t.Fatalf("route settlement outbox rows = %+v, want event types %+v", outbox, want)
	}
	for index, record := range outbox {
		if record.Event.Type != want[index] {
			t.Fatalf("route settlement outbox[%d] type = %q, want %q; rows = %+v", index, record.Event.Type, want[index], outbox)
		}
		if record.ReferenceKey.IsZero() || record.SettlementWindow == "" {
			t.Fatalf("route settlement outbox[%d] missing settlement evidence: %+v", index, record)
		}
	}
}
