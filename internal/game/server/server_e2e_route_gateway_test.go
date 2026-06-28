package server

import (
	"encoding/json"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
)

func TestE2ERouteSeedSupportsGatewayCreateAndSettleLoop(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		DevMode:           true,
		E2ERouteSeed:      true,
		Clock:             clock,
		ContentRepository: staticContentRepositoryForTest(),
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	resolved := createResolvedRuntimeSession(t, gameServer, "route-seed-gateway@example.com", "Route Seed Gateway")
	sourceID := e2eRoutePlanetID(resolved.PlayerID, "source")
	destinationID := e2eRoutePlanetID(resolved.PlayerID, "destination")
	createRequestID := foundation.RequestID("request-e2e-route-create")
	routeID := foundation.RouteID("route-" + createRequestID.String())

	create := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"`+createRequestID.String()+`","op":"route.create","payload":{"source_planet_id":"`+sourceID.String()+`","destination_planet_id":"`+destinationID.String()+`","resource_item_id":"refined_alloy","amount_per_hour":40},"client_seq":1,"v":1}`),
	)
	if create.HasError {
		t.Fatalf("route.create response error = %+v, want success", create.Error)
	}
	var createPayload struct {
		Route routePayload `json:"route"`
	}
	if err := json.Unmarshal(create.Response.Payload, &createPayload); err != nil {
		t.Fatalf("decode route.create payload: %v", err)
	}
	assertRoutePayloadMapKeys(t, createPayload.Route, routeID, "1-1", "1-1")
	if createPayload.Route.Destination.Type != production.RouteDestinationTypePlanet.String() ||
		createPayload.Route.Destination.ID != destinationID.String() ||
		createPayload.Route.ResourceItemID != "refined_alloy" ||
		createPayload.Route.AmountPerHour != 40 ||
		!createPayload.Route.Enabled {
		t.Fatalf("route.create payload = %+v, want enabled e2e route", createPayload.Route)
	}

	clock.Advance(time.Hour)
	settle := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-e2e-route-settle","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":2,"v":1}`),
	)
	if settle.HasError {
		t.Fatalf("route.settle response error = %+v, want success", settle.Error)
	}
	var settlePayload struct {
		Route      routePayload           `json:"route"`
		Settlement routeSettlementPayload `json:"settlement"`
	}
	if err := json.Unmarshal(settle.Response.Payload, &settlePayload); err != nil {
		t.Fatalf("decode route.settle payload: %v", err)
	}
	assertRoutePayloadMapKeys(t, settlePayload.Route, routeID, "1-1", "1-1")
	assertRouteSettlementPayload(t, settlePayload.Settlement, routeID, "refined_alloy", 3_600_000, 40, 40, 0, 40, 40, false, "e2e route.settle response")
	assertStoredRouteStorageQuantity(t, gameServer, sourceID, "refined_alloy", 120)
	assertStoredRouteStorageQuantity(t, gameServer, destinationID, "refined_alloy", 40)
}
