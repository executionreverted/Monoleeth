package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

type steppingRouteSettleClock struct {
	next time.Time
	step time.Duration
}

func (clock *steppingRouteSettleClock) Now() time.Time {
	now := clock.next
	clock.next = clock.next.Add(clock.step)
	return now
}

func TestRouteSettleTransfersStorageAndQueuesSafeOwnerEvents(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-owner@example.com", "Route Settle Owner")
	other := createResolvedRuntimeSession(t, gameServer, "route-settle-other@example.com", "Route Settle Other")
	sourcePlanetID := foundation.PlanetID("planet-route-settle-a")
	destinationPlanetID := foundation.PlanetID("planet-route-settle-b")
	routeID := foundation.RouteID("route-settle-owned")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-b")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	clock.Advance(time.Hour)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-owned","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("route.settle response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsInternalMapIdentity(t, "route.settle response", response.Response.Payload)
	assertPayloadOmitsPlayerOwner(t, "route.settle response", response.Response.Payload, owner.PlayerID)
	var payload struct {
		Route      routePayload                      `json:"route"`
		Routes     routeListPayload                  `json:"routes"`
		Settlement routeSettlementPayload            `json:"settlement"`
		Production planetProductionCollectionPayload `json:"production"`
		Storage    planetStorageCollectionPayload    `json:"storage"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode route.settle payload: %v", err)
	}
	assertRoutePayloadMapKeys(t, payload.Route, routeID, "1-1", "1-2")
	if len(payload.Routes.Routes) != 1 {
		t.Fatalf("route.settle routes = %+v, want one route", payload.Routes.Routes)
	}
	assertRoutePayloadMapKeys(t, payload.Routes.Routes[0], routeID, "1-1", "1-2")
	assertRouteSettlementPayload(t, payload.Settlement, routeID, "refined_alloy", 3_600_000, 40, 40, 0, 40, 40, false, "route.settle response")
	assertRouteSettlementPayloadOmitsServerTruth(t, payload.Settlement, owner.PlayerID, "route.settle settlement")
	assertRouteControlActiveMapSnapshots(t, payload.Production, payload.Storage, sourcePlanetID, 60)
	assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 60)
	assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "refined_alloy", 40)
	assertStoredRouteCursor(t, gameServer, routeID, clock.Now())
	assertRouteDurableSettlementRows(t, gameServer, []foundation.RouteID{routeID}, 2)

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteSettle, owner.PlayerID)
	if err != nil {
		t.Fatalf("post route.settle events: %v", err)
	}
	if _, leaked := eventsBySession[other.SessionID]; leaked {
		t.Fatalf("route.settle events leaked to non-owner session: %+v", eventsBySession[other.SessionID])
	}
	assertRouteSettleEvents(t, eventsBySession[owner.SessionID], routeID, sourcePlanetID, "1-1", "1-2", owner.PlayerID, 60, 1)
}

func TestRouteSettleNonPlanetDestinationRoutesThroughGateway(t *testing.T) {
	cases := []struct {
		name        string
		destination production.RouteDestination
	}{
		{
			name: "storage",
			destination: production.RouteDestination{
				Type: production.RouteDestinationTypeStorage,
				ID:   "storage-route-settle-destination",
			},
		},
		{
			name: "station",
			destination: production.RouteDestination{
				Type: production.RouteDestinationTypeStation,
				ID:   "station-route-settle-destination",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
			gameServer := newRouteControlTestServer(t, clock)
			owner := createResolvedRuntimeSession(t, gameServer, "route-settle-"+tc.name+"-owner@example.com", "Route Settle "+tc.name+" Owner")
			other := createResolvedRuntimeSession(t, gameServer, "route-settle-"+tc.name+"-other@example.com", "Route Settle "+tc.name+" Other")
			sourcePlanetID := foundation.PlanetID("planet-route-settle-" + tc.name + "-source")
			destinationStorageID := foundation.PlanetID(tc.destination.ID)
			routeID := foundation.RouteID("route-settle-" + tc.name + "-destination")

			seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, discovery.PlanetMaterializationKey("candidate-route-settle-"+tc.name+"-source"))
			saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
			saveRouteControlStorage(t, gameServer, destinationStorageID, nil)
			seedAutomationRouteToDestinationForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, tc.destination, "map_1_1", "map_1_1")
			clock.Advance(time.Hour)

			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(owner.SessionID.String()),
				[]byte(`{"request_id":"request-route-settle-`+tc.name+`-destination","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
			)
			if response.HasError {
				t.Fatalf("route.settle %s destination response error = %+v, want success", tc.name, response.Error)
			}
			assertPayloadOmitsInternalMapIdentity(t, "route.settle "+tc.name+" response", response.Response.Payload)
			assertPayloadOmitsPlayerOwner(t, "route.settle "+tc.name+" response", response.Response.Payload, owner.PlayerID)
			var payload struct {
				Route      routePayload                      `json:"route"`
				Routes     routeListPayload                  `json:"routes"`
				Settlement routeSettlementPayload            `json:"settlement"`
				Production planetProductionCollectionPayload `json:"production"`
				Storage    planetStorageCollectionPayload    `json:"storage"`
			}
			if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
				t.Fatalf("decode route.settle %s payload: %v", tc.name, err)
			}
			assertRoutePayloadMapKeys(t, payload.Route, routeID, "1-1", "1-1")
			if payload.Route.Destination.Type != tc.destination.Type.String() || payload.Route.Destination.ID != "" {
				t.Fatalf("route.settle %s route destination = %+v, want public %q destination without internal id", tc.name, payload.Route.Destination, tc.destination.Type)
			}
			assertPayloadOmitsRouteEndpointID(t, "route.settle "+tc.name+" response", response.Response.Payload, tc.destination.ID.String())
			if len(payload.Routes.Routes) != 1 ||
				payload.Routes.Routes[0].Destination.Type != tc.destination.Type.String() ||
				payload.Routes.Routes[0].Destination.ID != "" {
				t.Fatalf("route.settle %s route list = %+v, want one public %q destination without internal id", tc.name, payload.Routes.Routes, tc.destination.Type)
			}
			assertRouteSettlementPayload(t, payload.Settlement, routeID, "refined_alloy", 3_600_000, 40, 40, 0, 40, 40, false, "route.settle "+tc.name+" response")
			assertRouteSettlementPayloadOmitsServerTruth(t, payload.Settlement, owner.PlayerID, "route.settle "+tc.name+" settlement")
			assertRouteControlActiveMapSnapshots(t, payload.Production, payload.Storage, sourcePlanetID, 60)
			assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 60)
			assertStoredRouteStorageQuantity(t, gameServer, destinationStorageID, "refined_alloy", 40)
			assertRouteDurableSettlementRows(t, gameServer, []foundation.RouteID{routeID}, 2)

			eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteSettle, owner.PlayerID)
			if err != nil {
				t.Fatalf("post route.settle %s events: %v", tc.name, err)
			}
			if _, leaked := eventsBySession[other.SessionID]; leaked {
				t.Fatalf("route.settle %s events leaked to non-owner session: %+v", tc.name, eventsBySession[other.SessionID])
			}
			for _, event := range eventsBySession[owner.SessionID] {
				assertPayloadOmitsRouteEndpointID(t, string(event.Type)+" "+tc.name+" event", event.Payload, tc.destination.ID.String())
			}
			assertRouteSettleEvents(t, eventsBySession[owner.SessionID], routeID, sourcePlanetID, "1-1", "1-1", owner.PlayerID, 60, 1)
		})
	}
}

func TestRouteSettleClampsDestinationStorageCapacity(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-capacity@example.com", "Route Settle Capacity")
	sourcePlanetID := foundation.PlanetID("planet-route-settle-capacity-a")
	destinationPlanetID := foundation.PlanetID("planet-route-settle-capacity-b")
	routeID := foundation.RouteID("route-settle-capacity")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-capacity-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-capacity-b")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	saveRouteControlStorage(t, gameServer, destinationPlanetID, []production.StoredItem{{ItemID: "iron_ore", Quantity: 250}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	clock.Advance(time.Hour)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-capacity","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("route.settle capacity response error = %+v, want success", response.Error)
	}
	var payload struct {
		Settlement routeSettlementPayload `json:"settlement"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode route.settle capacity payload: %v", err)
	}
	assertRouteSettlementPayload(t, payload.Settlement, routeID, "refined_alloy", 3_600_000, 40, 40, 0, 40, 0, false, "capacity route.settle response")
	if !payload.Settlement.DestinationFull {
		t.Fatalf("capacity route.settle destination_full = false, want true")
	}
	assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 60)
	assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "refined_alloy", 0)
	assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "iron_ore", 250)
	assertStoredPlanetStorageWithinCapacity(t, gameServer, destinationPlanetID)
}

func TestRouteSettleImmediateDuplicateReturnsNoOpWithoutDuplicateTransfer(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-duplicate@example.com", "Route Settle Duplicate")
	sourcePlanetID := foundation.PlanetID("planet-route-settle-dup-a")
	destinationPlanetID := foundation.PlanetID("planet-route-settle-dup-b")
	routeID := foundation.RouteID("route-settle-duplicate")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-dup-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-dup-b")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	clock.Advance(time.Hour)

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-duplicate-first","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if first.HasError {
		t.Fatalf("first route.settle response error = %+v, want success", first.Error)
	}
	durableOutboxAfterFirst := len(gameServer.runtime.Settlements.OutboxRecords())
	durableLedgerAfterFirst := len(gameServer.runtime.Settlements.RouteStorageLedgerEntries())
	if _, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteSettle, owner.PlayerID); err != nil {
		t.Fatalf("drain first route.settle events: %v", err)
	}

	second := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-duplicate-second","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":2,"v":1}`),
	)
	if second.HasError {
		t.Fatalf("duplicate route.settle response error = %+v, want success", second.Error)
	}
	var payload struct {
		Settlement routeSettlementPayload `json:"settlement"`
	}
	if err := json.Unmarshal(second.Response.Payload, &payload); err != nil {
		t.Fatalf("decode duplicate route.settle payload: %v", err)
	}
	assertRouteSettlementPayload(t, payload.Settlement, routeID, "refined_alloy", 0, 0, 0, 0, 0, 0, true, "duplicate route.settle response")
	assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 60)
	assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "refined_alloy", 40)
	assertRouteDurableSettlementRows(t, gameServer, []foundation.RouteID{routeID}, durableLedgerAfterFirst)
	if got := len(gameServer.runtime.Settlements.OutboxRecords()); got != durableOutboxAfterFirst {
		t.Fatalf("duplicate route.settle durable outbox rows = %d, want %d", got, durableOutboxAfterFirst)
	}

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteSettle, owner.PlayerID)
	if err != nil {
		t.Fatalf("post duplicate route.settle events: %v", err)
	}
	assertRouteSettleNoOpEvents(t, eventsBySession[owner.SessionID], routeID)
}

func TestRouteSettleDuplicateReplaysDurableHandoffAfterLiveRouteRowLoss(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-replay@example.com", "Route Settle Replay")
	sourcePlanetID := foundation.PlanetID("planet-route-settle-replay-a")
	destinationPlanetID := foundation.PlanetID("planet-route-settle-replay-b")
	routeID := foundation.RouteID("route-settle-replay")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-replay-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-replay-b")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	clock.Advance(time.Hour)

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-replay-first","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if first.HasError {
		t.Fatalf("first route.settle response error = %+v, want success", first.Error)
	}
	durableOutboxAfterFirst := len(gameServer.runtime.Settlements.OutboxRecords())
	durableLedgerAfterFirst := len(gameServer.runtime.Settlements.RouteStorageLedgerEntries())
	if err := gameServer.runtime.Production.DropAutomationRouteReadModel(routeID); err != nil {
		t.Fatalf("DropAutomationRouteReadModel() error = %v, want nil", err)
	}

	second := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-replay-second","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":2,"v":1}`),
	)
	if second.HasError {
		t.Fatalf("duplicate route.settle replay response error = %+v, want success", second.Error)
	}
	var payload struct {
		Settlement routeSettlementPayload `json:"settlement"`
		Routes     routeListPayload       `json:"routes"`
	}
	if err := json.Unmarshal(second.Response.Payload, &payload); err != nil {
		t.Fatalf("decode duplicate replay route.settle payload: %v", err)
	}
	assertRouteSettlementPayload(t, payload.Settlement, routeID, "refined_alloy", 0, 0, 0, 0, 0, 0, true, "duplicate replay route.settle response")
	if len(payload.Routes.Routes) != 1 || payload.Routes.Routes[0].RouteID != routeID.String() {
		t.Fatalf("duplicate replay route list = %+v, want recovered durable route %q", payload.Routes.Routes, routeID)
	}
	assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 60)
	assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "refined_alloy", 40)
	if got := len(gameServer.runtime.Settlements.OutboxRecords()); got != durableOutboxAfterFirst {
		t.Fatalf("duplicate replay durable outbox rows = %d, want %d", got, durableOutboxAfterFirst)
	}
	if got := len(gameServer.runtime.Settlements.RouteStorageLedgerEntries()); got != durableLedgerAfterFirst {
		t.Fatalf("duplicate replay durable ledger rows = %d, want %d", got, durableLedgerAfterFirst)
	}
	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteSettle, owner.PlayerID)
	if err != nil {
		t.Fatalf("post duplicate replay route.settle events: %v", err)
	}
	assertRouteSettleNoOpEvents(t, routeSettleEventSuffixForTest(t, eventsBySession[owner.SessionID], 4), routeID)
}

func TestRouteSettleEmptyPayloadReconcilesOwnedRoutesOnly(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-all-owner@example.com", "Route Settle All Owner")
	other := createResolvedRuntimeSession(t, gameServer, "route-settle-all-other@example.com", "Route Settle All Other")

	sourceOneID := foundation.PlanetID("planet-route-settle-all-a")
	destinationOneID := foundation.PlanetID("planet-route-settle-all-b")
	sourceTwoID := foundation.PlanetID("planet-route-settle-all-c")
	destinationTwoID := foundation.PlanetID("planet-route-settle-all-d")
	otherSourceID := foundation.PlanetID("planet-route-settle-other-a")
	otherDestinationID := foundation.PlanetID("planet-route-settle-other-b")
	routeOneID := foundation.RouteID("route-settle-all-one")
	routeTwoID := foundation.RouteID("route-settle-all-two")
	otherRouteID := foundation.RouteID("route-settle-all-other")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourceOneID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-all-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationOneID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-all-b")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourceTwoID, gameServer.runtime.zoneID, world.Vec2{X: 2300, Y: 2400}, "candidate-route-settle-all-c")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationTwoID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 2700, Y: 5200}, "candidate-route-settle-all-d")
	seedOwnedProductionPlanetForTest(t, gameServer, other.PlayerID, otherSourceID, gameServer.runtime.zoneID, world.Vec2{X: 3300, Y: 3400}, "candidate-route-settle-other-a")
	seedOwnedProductionPlanetForTest(t, gameServer, other.PlayerID, otherDestinationID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 3700, Y: 5200}, "candidate-route-settle-other-b")
	saveRouteControlStorage(t, gameServer, sourceOneID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	saveRouteControlStorage(t, gameServer, sourceTwoID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 80}})
	saveRouteControlStorage(t, gameServer, otherSourceID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 50}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeOneID, sourceOneID, destinationOneID, "map_1_1", "map_1_2")
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeTwoID, sourceTwoID, destinationTwoID, "map_1_1", "map_1_2")
	seedAutomationRouteForTest(t, gameServer, other.PlayerID, otherRouteID, otherSourceID, otherDestinationID, "map_1_1", "map_1_2")
	clock.Advance(time.Hour)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-all-owned","op":"route.settle","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("route.settle all response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsInternalMapIdentity(t, "route.settle all response", response.Response.Payload)
	assertPayloadOmitsPlayerOwner(t, "route.settle all response", response.Response.Payload, owner.PlayerID)
	assertPayloadOmitsPlayerOwner(t, "route.settle all response", response.Response.Payload, other.PlayerID)
	var payload struct {
		Routes      routeListPayload                  `json:"routes"`
		Settlements []routeSettlementPayload          `json:"settlements"`
		Production  planetProductionCollectionPayload `json:"production"`
		Storage     planetStorageCollectionPayload    `json:"storage"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode route.settle all payload: %v", err)
	}
	if len(payload.Routes.Routes) != 2 {
		t.Fatalf("route.settle all routes = %+v, want two owned routes", payload.Routes.Routes)
	}
	if len(payload.Settlements) != 2 {
		t.Fatalf("route.settle all settlements = %+v, want two owned settlements", payload.Settlements)
	}
	assertRouteSettlementIDs(t, payload.Settlements, routeOneID, routeTwoID)
	assertRouteUpdateProductionSnapshot(t, payload.Production, sourceOneID, 60)
	assertRouteUpdateProductionSnapshot(t, payload.Production, sourceTwoID, 40)
	assertRouteUpdateStorageSnapshot(t, payload.Storage, sourceOneID, 60)
	assertRouteUpdateStorageSnapshot(t, payload.Storage, sourceTwoID, 40)
	assertStoredRouteStorageQuantity(t, gameServer, sourceOneID, "refined_alloy", 60)
	assertStoredRouteStorageQuantity(t, gameServer, destinationOneID, "refined_alloy", 40)
	assertStoredRouteStorageQuantity(t, gameServer, sourceTwoID, "refined_alloy", 40)
	assertStoredRouteStorageQuantity(t, gameServer, destinationTwoID, "refined_alloy", 40)
	assertStoredRouteStorageQuantity(t, gameServer, otherSourceID, "refined_alloy", 50)
	assertStoredRouteStorageQuantity(t, gameServer, otherDestinationID, "refined_alloy", 0)
	assertRouteDurableSettlementRows(t, gameServer, []foundation.RouteID{routeOneID, routeTwoID}, 4)

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationRouteSettle, owner.PlayerID)
	if err != nil {
		t.Fatalf("post route.settle all events: %v", err)
	}
	if _, leaked := eventsBySession[other.SessionID]; leaked {
		t.Fatalf("route.settle all events leaked to non-owner session: %+v", eventsBySession[other.SessionID])
	}
	assertRouteSettleReconcileEvents(t, eventsBySession[owner.SessionID], []foundation.RouteID{routeOneID, routeTwoID}, owner.PlayerID)
}

func TestRouteSettleRejectsWrongOwnerWithoutMutationOrEvents(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-safe-owner@example.com", "Route Settle Safe Owner")
	other := createResolvedRuntimeSession(t, gameServer, "route-settle-wrong-owner@example.com", "Route Settle Wrong Owner")
	sourcePlanetID := foundation.PlanetID("planet-route-settle-wrong-a")
	destinationPlanetID := foundation.PlanetID("planet-route-settle-wrong-b")
	routeID := foundation.RouteID("route-settle-wrong-owner")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-wrong-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-wrong-b")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	clock.Advance(time.Hour)

	beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(other.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-wrong-owner","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("route.settle wrong-owner response = %+v, want safe not-found", response)
	}
	assertRoutesUnchanged(t, gameServer, beforeRoutes, "route.settle wrong-owner")
	assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 100)
	assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "refined_alloy", 0)
	assertNoQueuedEventsForSessions(t, gameServer, owner.SessionID, other.SessionID)
}

func TestRouteSettleRejectsCaseVariantRouteIDKeysWithoutMutationOrEvents(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-case-key@example.com", "Route Settle Case Key")
	sourcePlanetID := foundation.PlanetID("planet-route-settle-case-a")
	destinationPlanetID := foundation.PlanetID("planet-route-settle-case-b")
	routeID := foundation.RouteID("route-settle-case-key")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-case-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-case-b")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	clock.Advance(time.Hour)

	for _, key := range []string{"route_ID", "RouteID", "routeId"} {
		t.Run(key, func(t *testing.T) {
			beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(owner.SessionID.String()),
				[]byte(`{"request_id":"request-route-settle-case-`+key+`","op":"route.settle","payload":{"`+key+`":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
			)
			if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
				t.Fatalf("route.settle case-variant key response = %+v, want invalid payload", response)
			}
			assertRoutesUnchanged(t, gameServer, beforeRoutes, key)
			assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 100)
			assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "refined_alloy", 0)
			assertNoQueuedEventsForSessions(t, gameServer, owner.SessionID)
		})
	}
}

func TestRouteSettleReconcilePreflightsAllOwnedRoutesBeforeMutation(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-preflight-all@example.com", "Route Settle Preflight All")
	validSourceID := foundation.PlanetID("planet-route-settle-preflight-valid-a")
	validDestinationID := foundation.PlanetID("planet-route-settle-preflight-valid-b")
	brokenSourceID := foundation.PlanetID("planet-route-settle-preflight-broken-a")
	missingDestinationID := foundation.PlanetID("planet-route-settle-preflight-missing-b")
	validRouteID := foundation.RouteID("route-settle-preflight-a-valid")
	brokenRouteID := foundation.RouteID("route-settle-preflight-z-missing-storage")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, validSourceID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-preflight-valid-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, validDestinationID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-preflight-valid-b")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, brokenSourceID, gameServer.runtime.zoneID, world.Vec2{X: 1500, Y: 1450}, "candidate-route-settle-preflight-broken-a")
	saveRouteControlStorage(t, gameServer, validSourceID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	saveRouteControlStorage(t, gameServer, brokenSourceID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 80}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, validRouteID, validSourceID, validDestinationID, "map_1_1", "map_1_2")
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, brokenRouteID, brokenSourceID, missingDestinationID, "map_1_1", "map_1_2")
	validBefore, ok, err := gameServer.runtime.Production.AutomationRoute(validRouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok=%v err=%v, want valid route", validRouteID, ok, err)
	}
	clock.Advance(time.Hour)

	beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-preflight-all","op":"route.settle","payload":{},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("route.settle preflight all response = %+v, want safe not-found", response)
	}
	assertRoutesUnchanged(t, gameServer, beforeRoutes, "route.settle preflight all")
	assertStoredRouteCursor(t, gameServer, validRouteID, validBefore.LastCalculatedAt)
	assertStoredRouteStorageQuantity(t, gameServer, validSourceID, "refined_alloy", 100)
	assertStoredRouteStorageQuantity(t, gameServer, validDestinationID, "refined_alloy", 0)
	assertNoQueuedEventsForSessions(t, gameServer, owner.SessionID)
}

func TestRouteSettleReconcilePreflightsClockDriftStorageBeforeMutation(t *testing.T) {
	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	requestNow := base.Add(time.Hour)
	setupClock := testutil.NewFakeClock(base)
	gameServer := newRouteControlTestServer(t, setupClock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-preflight-drift@example.com", "Route Settle Preflight Drift")
	validSourceID := foundation.PlanetID("planet-route-settle-drift-valid-a")
	validDestinationID := foundation.PlanetID("planet-route-settle-drift-valid-b")
	brokenSourceID := foundation.PlanetID("planet-route-settle-drift-broken-a")
	missingDestinationID := foundation.PlanetID("planet-route-settle-drift-missing-b")
	validRouteID := foundation.RouteID("route-settle-drift-a-valid")
	brokenRouteID := foundation.RouteID("route-settle-drift-z-missing-storage")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, validSourceID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-drift-valid-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, validDestinationID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-drift-valid-b")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, brokenSourceID, gameServer.runtime.zoneID, world.Vec2{X: 1500, Y: 1450}, "candidate-route-settle-drift-broken-a")
	saveRouteControlStorage(t, gameServer, validSourceID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	saveRouteControlStorage(t, gameServer, brokenSourceID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 80}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, validRouteID, validSourceID, validDestinationID, "map_1_1", "map_1_2")
	setupClock.Advance(time.Hour - 89*time.Second)
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, brokenRouteID, brokenSourceID, missingDestinationID, "map_1_1", "map_1_2")
	gameServer.runtime.clock = &steppingRouteSettleClock{next: requestNow, step: 4 * time.Second}
	validBefore, ok, err := gameServer.runtime.Production.AutomationRoute(validRouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok=%v err=%v, want valid route", validRouteID, ok, err)
	}

	beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-preflight-drift","op":"route.settle","payload":{},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("route.settle preflight drift response = %+v, want safe not-found", response)
	}
	assertRoutesUnchanged(t, gameServer, beforeRoutes, "route.settle preflight drift")
	assertStoredRouteCursor(t, gameServer, validRouteID, validBefore.LastCalculatedAt)
	assertStoredRouteStorageQuantity(t, gameServer, validSourceID, "refined_alloy", 100)
	assertStoredRouteStorageQuantity(t, gameServer, validDestinationID, "refined_alloy", 0)
	assertNoQueuedEventsForSessions(t, gameServer, owner.SessionID)
}

func TestRouteSettleReconcileUsesRequestScopedClockForSettlements(t *testing.T) {
	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	requestNow := base.Add(time.Hour)
	setupClock := testutil.NewFakeClock(base)
	gameServer := newRouteControlTestServer(t, setupClock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-fixed-clock@example.com", "Route Settle Fixed Clock")
	sourceOneID := foundation.PlanetID("planet-route-settle-fixed-clock-a")
	destinationOneID := foundation.PlanetID("planet-route-settle-fixed-clock-b")
	sourceTwoID := foundation.PlanetID("planet-route-settle-fixed-clock-c")
	destinationTwoID := foundation.PlanetID("planet-route-settle-fixed-clock-d")
	routeOneID := foundation.RouteID("route-settle-fixed-clock-a")
	routeTwoID := foundation.RouteID("route-settle-fixed-clock-b")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourceOneID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-fixed-clock-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationOneID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-fixed-clock-b")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourceTwoID, gameServer.runtime.zoneID, world.Vec2{X: 1500, Y: 1450}, "candidate-route-settle-fixed-clock-c")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationTwoID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1800, Y: 5300}, "candidate-route-settle-fixed-clock-d")
	saveRouteControlStorage(t, gameServer, sourceOneID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	saveRouteControlStorage(t, gameServer, sourceTwoID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeOneID, sourceOneID, destinationOneID, "map_1_1", "map_1_2")
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeTwoID, sourceTwoID, destinationTwoID, "map_1_1", "map_1_2")
	gameServer.runtime.clock = &steppingRouteSettleClock{next: requestNow, step: 5 * time.Second}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-fixed-clock","op":"route.settle","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("route.settle fixed-clock response error = %+v, want success", response.Error)
	}
	assertStoredRouteCursor(t, gameServer, routeOneID, requestNow)
	assertStoredRouteCursor(t, gameServer, routeTwoID, requestNow)
	assertStoredRouteStorageQuantity(t, gameServer, sourceOneID, "refined_alloy", 60)
	assertStoredRouteStorageQuantity(t, gameServer, sourceTwoID, "refined_alloy", 60)
}

func TestRouteSettleSinglePreflightsOwnerRouteListBeforeMutation(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-settle-preflight-single@example.com", "Route Settle Preflight Single")
	validSourceID := foundation.PlanetID("planet-route-settle-preflight-single-a")
	validDestinationID := foundation.PlanetID("planet-route-settle-preflight-single-b")
	brokenSourceID := foundation.PlanetID("planet-route-settle-preflight-single-broken-a")
	brokenDestinationID := foundation.PlanetID("planet-route-settle-preflight-single-broken-b")
	validRouteID := foundation.RouteID("route-settle-preflight-single-valid")
	brokenRouteID := foundation.RouteID("route-settle-preflight-single-broken-map")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, validSourceID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-preflight-single-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, validDestinationID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-preflight-single-b")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, brokenSourceID, gameServer.runtime.zoneID, world.Vec2{X: 1500, Y: 1450}, "candidate-route-settle-preflight-single-broken-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, brokenDestinationID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1800, Y: 5300}, "candidate-route-settle-preflight-single-broken-b")
	saveRouteControlStorage(t, gameServer, validSourceID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, validRouteID, validSourceID, validDestinationID, "map_1_1", "map_1_2")
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, brokenRouteID, brokenSourceID, brokenDestinationID, "map_1_1", "map_missing")
	validBefore, ok, err := gameServer.runtime.Production.AutomationRoute(validRouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok=%v err=%v, want valid route", validRouteID, ok, err)
	}
	clock.Advance(time.Hour)

	beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-settle-preflight-single","op":"route.settle","payload":{"route_id":"`+validRouteID.String()+`"},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("route.settle preflight single response = %+v, want safe not-found", response)
	}
	assertRoutesUnchanged(t, gameServer, beforeRoutes, "route.settle preflight single")
	assertStoredRouteCursor(t, gameServer, validRouteID, validBefore.LastCalculatedAt)
	assertStoredRouteStorageQuantity(t, gameServer, validSourceID, "refined_alloy", 100)
	assertStoredRouteStorageQuantity(t, gameServer, validDestinationID, "refined_alloy", 0)
	assertNoQueuedEventsForSessions(t, gameServer, owner.SessionID)
}

func TestRouteSettleRejectsSpoofedServerOwnedFieldsBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "route-settle-spoof@example.com", "Route Settle Spoof")
	sourcePlanetID := foundation.PlanetID("planet-route-settle-spoof-a")
	destinationPlanetID := foundation.PlanetID("planet-route-settle-spoof-b")
	routeID := foundation.RouteID("route-settle-spoof")

	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-settle-spoof-a")
	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-settle-spoof-b")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, resolved.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")

	tests := []struct {
		name  string
		field string
	}{
		{name: "owner", field: `"owner_player_id":"spoofed-player"`},
		{name: "session", field: `"session":{"id":"spoofed-session"}`},
		{name: "route payload", field: `"route":{"route_id":"route-spoofed"}`},
		{name: "route list payload", field: `"routes":[]`},
		{name: "settlement", field: `"settlement":{"taken_amount":999}`},
		{name: "settled at", field: `"settled_at":999999999`},
		{name: "elapsed", field: `"elapsed_applied_ms":3600000`},
		{name: "window", field: `"settlement_window":"forced"`},
		{name: "source planet", field: `"source_planet_id":"planet-spoofed"`},
		{name: "destination fact", field: `"destination":{"type":"planet","id":"planet-spoofed"}`},
		{name: "enabled", field: `"enabled":false`},
		{name: "storage", field: `"storage":{"refined_alloy":999}`},
		{name: "energy", field: `"energy_cost_per_hour":0`},
		{name: "risk", field: `"route_risk":{"loss_chance":0}`},
		{name: "wanted amount", field: `"wanted_amount":999`},
		{name: "delivered amount", field: `"delivered_amount":999`},
		{name: "rate", field: `"amount_per_hour":999999`},
		{name: "resource", field: `"resource_item_id":"x_core"`},
		{name: "cooldown", field: `"cooldown":0`},
		{name: "position", field: `"position":{"x":0,"y":0}`},
		{name: "nested internal map", field: `"config":{"source_map_id":"map_1_1"}`},
		{name: "unknown field", field: `"unexpected":true`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeRoutes := gameServer.runtime.Production.AutomationRoutes()
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(resolved.SessionID.String()),
				[]byte(`{"request_id":"request-route-settle-spoof-`+strings.ReplaceAll(tt.name, " ", "-")+`","op":"route.settle","payload":{"route_id":"`+routeID.String()+`",`+tt.field+`},"client_seq":1,"v":1}`),
			)
			if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
				t.Fatalf("route.settle spoof response = %+v, want invalid payload", response)
			}
			assertRoutesUnchanged(t, gameServer, beforeRoutes, tt.name)
			assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 100)
			assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "refined_alloy", 0)
			assertNoQueuedEventsForSessions(t, gameServer, resolved.SessionID)
		})
	}
}

func TestRouteSettleMapsErrorsSafely(t *testing.T) {
	for _, err := range []error{
		production.ErrInvalidRouteCreateConfig,
		production.ErrInvalidRouteSettlementConfig,
		production.ErrInvalidRouteDestinationID,
		production.ErrInvalidRouteDestinationType,
		production.ErrInvalidRouteEnergyCost,
		production.ErrInvalidRouteRisk,
		production.ErrInvalidRouteDistance,
		production.ErrInvalidRouteMapID,
		production.ErrInvalidProductionEvent,
		production.ErrZeroProductionTimestamp,
		foundation.ErrEmptyID,
		foundation.ErrInvalidID,
	} {
		t.Run(err.Error(), func(t *testing.T) {
			mapped := domainErrorForRouteSettle(fmt.Errorf("stored route/config: %w", err))
			domainErr, ok := mapped.(*foundation.DomainError)
			if !ok {
				t.Fatalf("domainErrorForRouteSettle(%v) = %T, want DomainError", err, mapped)
			}
			if domainErr.Code != foundation.CodeInternal {
				t.Fatalf("domainErrorForRouteSettle(%v) code = %s, want %s", err, domainErr.Code, foundation.CodeInternal)
			}
		})
	}

	for _, err := range []error{
		production.ErrRouteNotFound,
		production.ErrRouteOwnerMismatch,
		production.ErrRouteSourceStorageMissing,
		production.ErrRouteDestinationStorageMissing,
		production.ErrUnsupportedRouteDestination,
		worldmaps.ErrMapNotFound,
	} {
		t.Run("not-found-"+err.Error(), func(t *testing.T) {
			mapped := domainErrorForRouteSettle(fmt.Errorf("route settle: %w", err))
			domainErr, ok := mapped.(*foundation.DomainError)
			if !ok {
				t.Fatalf("domainErrorForRouteSettle(%v) = %T, want DomainError", err, mapped)
			}
			if domainErr.Code != foundation.CodeNotFound {
				t.Fatalf("domainErrorForRouteSettle(%v) code = %s, want %s", err, domainErr.Code, foundation.CodeNotFound)
			}
		})
	}
}

func TestRouteSettleRejectsInvalidRouteID(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "route-settle-invalid@example.com", "Route Settle Invalid")

	for _, tc := range []struct {
		name    string
		payload string
	}{
		{name: "malformed", payload: `{"route_id":"bad:id"}`},
		{name: "empty", payload: `{"route_id":""}`},
		{name: "null", payload: `{"route_id":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(resolved.SessionID.String()),
				[]byte(`{"request_id":"request-route-settle-invalid-`+tc.name+`","op":"route.settle","payload":`+tc.payload+`,"client_seq":1,"v":1}`),
			)
			if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
				t.Fatalf("route.settle invalid route id response = %+v, want invalid payload", response)
			}
		})
	}
	assertNoQueuedEventsForSessions(t, gameServer, resolved.SessionID)
}

func assertRouteSettlementPayload(
	t *testing.T,
	payload routeSettlementPayload,
	routeID foundation.RouteID,
	resourceItemID foundation.ItemID,
	elapsedAppliedMS int64,
	wantedAmount int64,
	takenAmount int64,
	lostAmount int64,
	deliveredAmount int64,
	addedAmount int64,
	noOp bool,
	label string,
) {
	t.Helper()
	if payload.RouteID != routeID.String() ||
		payload.ResourceItemID != resourceItemID.String() ||
		payload.ElapsedAppliedMS != elapsedAppliedMS ||
		payload.WantedAmount != wantedAmount ||
		payload.TakenAmount != takenAmount ||
		payload.LostAmount != lostAmount ||
		payload.DeliveredAmount != deliveredAmount ||
		payload.AddedAmount != addedAmount ||
		payload.NoOp != noOp {
		t.Fatalf("%s settlement = %+v, want route %q resource %q elapsed %d wanted/taken/lost/delivered/added %d/%d/%d/%d/%d no_op %v", label, payload, routeID, resourceItemID, elapsedAppliedMS, wantedAmount, takenAmount, lostAmount, deliveredAmount, addedAmount, noOp)
	}
	if payload.SettledAt <= 0 {
		t.Fatalf("%s settled_at = %d, want server timestamp", label, payload.SettledAt)
	}
}

func assertStoredPlanetStorageWithinCapacity(t *testing.T, gameServer *Server, planetID foundation.PlanetID) {
	t.Helper()
	storage, ok, err := gameServer.runtime.Production.PlanetStorage(planetID)
	if err != nil || !ok {
		t.Fatalf("PlanetStorage(%q) ok=%v err=%v, want stored", planetID, ok, err)
	}
	if used, capacity := storage.UsedUnits(), storage.CapacityUnits; used > capacity {
		t.Fatalf("storage %q used/capacity = %d/%d, want within capacity", planetID, used, capacity)
	}
}

func assertRouteSettleEvents(
	t *testing.T,
	events []realtime.EventEnvelope,
	routeID foundation.RouteID,
	activeMapPlanetID foundation.PlanetID,
	fromPublicMapKey string,
	toPublicMapKey string,
	ownerID foundation.PlayerID,
	wantQuantity int64,
	wantRouteListEvents int,
) {
	t.Helper()
	requireEventTypeForTest(t, events, realtime.EventRouteSettled)
	requireEventTypeForTest(t, events, realtime.EventRouteUpdated)
	requireEventTypeForTest(t, events, realtime.EventRouteSnapshot)
	requireEventTypeForTest(t, events, realtime.EventRouteList)
	requireEventTypeForTest(t, events, realtime.EventProductionSummary)
	requireEventTypeForTest(t, events, realtime.EventPlanetStorage)
	if countEventType(events, realtime.EventRouteList) != wantRouteListEvents {
		t.Fatalf("route.list events = %d, want %d in %+v", countEventType(events, realtime.EventRouteList), wantRouteListEvents, events)
	}
	if len(events) != 6 {
		t.Fatalf("route.settle events = %+v, want route.settled/updated/snapshot/list plus production/storage only", events)
	}
	for _, event := range events {
		assertPayloadOmitsInternalMapIdentity(t, string(event.Type)+" event", event.Payload)
		assertPayloadOmitsPlayerOwner(t, string(event.Type)+" event", event.Payload, ownerID)
		switch event.Type {
		case realtime.EventRouteSettled:
			var payload struct {
				Route      routePayload           `json:"route"`
				Settlement routeSettlementPayload `json:"settlement"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode route.settled event payload: %v", err)
			}
			assertRoutePayloadMapKeys(t, payload.Route, routeID, fromPublicMapKey, toPublicMapKey)
			assertRouteSettlementPayload(t, payload.Settlement, routeID, "refined_alloy", 3_600_000, 40, 40, 0, 40, 40, false, "route.settled event")
			assertRouteSettlementPayloadOmitsServerTruth(t, payload.Settlement, ownerID, "route.settled settlement")
		case realtime.EventRouteUpdated, realtime.EventRouteSnapshot:
			var payload struct {
				Route routePayload `json:"route"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode %s event payload: %v", event.Type, err)
			}
			assertRoutePayloadMapKeys(t, payload.Route, routeID, fromPublicMapKey, toPublicMapKey)
		case realtime.EventRouteList:
			var payload struct {
				Routes routeListPayload `json:"routes"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode route.list event payload: %v", err)
			}
			if len(payload.Routes.Routes) == 0 {
				t.Fatalf("route.list event routes = %+v, want at least one route", payload.Routes.Routes)
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
		case realtime.EventAOIEntityEntered, realtime.EventAOIEntityUpdated, realtime.EventAOIEntityLeft:
			t.Fatalf("route.settle emitted AOI diff event: %+v", event)
		}
	}
}

func assertRouteSettleNoOpEvents(t *testing.T, events []realtime.EventEnvelope, routeID foundation.RouteID) {
	t.Helper()
	requireEventTypeForTest(t, events, realtime.EventRouteSettled)
	requireEventTypeForTest(t, events, realtime.EventRouteUpdated)
	requireEventTypeForTest(t, events, realtime.EventRouteSnapshot)
	requireEventTypeForTest(t, events, realtime.EventRouteList)
	if len(events) != 4 {
		t.Fatalf("duplicate route.settle events = %+v, want route.settled/updated/snapshot/list only", events)
	}
	settled := requireEventTypeForTest(t, events, realtime.EventRouteSettled)
	var payload struct {
		Route      routePayload           `json:"route"`
		Settlement routeSettlementPayload `json:"settlement"`
	}
	if err := json.Unmarshal(settled.Payload, &payload); err != nil {
		t.Fatalf("decode no-op route.settled event payload: %v", err)
	}
	assertRoutePayloadMapKeys(t, payload.Route, routeID, "1-1", "1-2")
	assertRouteSettlementPayload(t, payload.Settlement, routeID, "refined_alloy", 0, 0, 0, 0, 0, 0, true, "no-op route.settled event")
	list := requireEventTypeForTest(t, events, realtime.EventRouteList)
	var listPayload struct {
		Routes routeListPayload `json:"routes"`
	}
	if err := json.Unmarshal(list.Payload, &listPayload); err != nil {
		t.Fatalf("decode no-op route.list event payload: %v", err)
	}
	if len(listPayload.Routes.Routes) != 1 || listPayload.Routes.Routes[0].RouteID != routeID.String() {
		t.Fatalf("no-op route.list event routes = %+v, want route %q", listPayload.Routes.Routes, routeID)
	}
}

func routeSettleEventSuffixForTest(t *testing.T, events []realtime.EventEnvelope, count int) []realtime.EventEnvelope {
	t.Helper()
	if len(events) < count {
		t.Fatalf("route.settle events = %+v, want at least %d events", events, count)
	}
	return events[len(events)-count:]
}

func assertRouteSettleReconcileEvents(t *testing.T, events []realtime.EventEnvelope, routeIDs []foundation.RouteID, ownerID foundation.PlayerID) {
	t.Helper()
	if countEventType(events, realtime.EventRouteSettled) != len(routeIDs) ||
		countEventType(events, realtime.EventRouteUpdated) != len(routeIDs) ||
		countEventType(events, realtime.EventRouteSnapshot) != len(routeIDs) ||
		countEventType(events, realtime.EventRouteList) != 1 ||
		countEventType(events, realtime.EventProductionSummary) != 1 ||
		countEventType(events, realtime.EventPlanetStorage) != 1 {
		t.Fatalf("route.settle reconcile events = %+v, want per-route settled/updated/snapshot, one list, and one production/storage pair", events)
	}
	for _, event := range events {
		assertPayloadOmitsInternalMapIdentity(t, string(event.Type)+" event", event.Payload)
		assertPayloadOmitsPlayerOwner(t, string(event.Type)+" event", event.Payload, ownerID)
		if event.Type == realtime.EventAOIEntityEntered || event.Type == realtime.EventAOIEntityUpdated || event.Type == realtime.EventAOIEntityLeft {
			t.Fatalf("route.settle reconcile emitted AOI diff event: %+v", event)
		}
	}
	seenSettlements := make(map[string]struct{}, len(routeIDs))
	for _, event := range events {
		if event.Type != realtime.EventRouteSettled {
			continue
		}
		var payload struct {
			Settlement routeSettlementPayload `json:"settlement"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode reconcile route.settled event payload: %v", err)
		}
		seenSettlements[payload.Settlement.RouteID] = struct{}{}
	}
	for _, routeID := range routeIDs {
		if _, ok := seenSettlements[routeID.String()]; !ok {
			t.Fatalf("route.settle reconcile events missing settlement for %q in %+v", routeID, seenSettlements)
		}
	}
}

func assertRouteDurableSettlementRows(t *testing.T, gameServer *Server, routeIDs []foundation.RouteID, wantLedgerRows int) {
	t.Helper()

	references := gameServer.runtime.Settlements.SettlementReferences()
	if len(references) != len(routeIDs) {
		t.Fatalf("durable route settlement references = %+v, want %d", references, len(routeIDs))
	}
	wantRoutes := make(map[foundation.RouteID]struct{}, len(routeIDs))
	for _, routeID := range routeIDs {
		wantRoutes[routeID] = struct{}{}
	}
	for _, reference := range references {
		if reference.Kind != production.SettlementKindRoute {
			t.Fatalf("durable route settlement reference = %+v, want route kind", reference)
		}
		if !reference.PlanetID.IsZero() {
			t.Fatalf("durable route settlement reference planet_id = %q, want empty", reference.PlanetID)
		}
		if _, ok := wantRoutes[reference.RouteID]; !ok {
			t.Fatalf("durable route settlement reference = %+v, want one of %+v", reference, routeIDs)
		}
		routeRecord, ok, err := gameServer.runtime.Production.CommittedAutomationRouteDurableRecordByReference(reference.ReferenceKey)
		if err != nil || !ok {
			t.Fatalf("durable route row for settlement reference %q ok = %v err = %v, want true nil", reference.ReferenceKey, ok, err)
		}
		if routeRecord.Route.RouteID != reference.RouteID || routeRecord.ReferenceKey != reference.ReferenceKey {
			t.Fatalf("durable route row = %+v, want route/reference %+v", routeRecord, reference)
		}
	}

	outbox := gameServer.runtime.Settlements.OutboxRecords()
	if len(outbox) == 0 {
		t.Fatal("durable route settlement outbox rows = 0, want pending rows")
	}
	hasReferenceEvidence := false
	for _, record := range outbox {
		if record.Status != production.ProductionOutboxStatusPending {
			t.Fatalf("durable route settlement outbox row = %+v, want pending status", record)
		}
		hasReferenceEvidence = hasReferenceEvidence || (!record.ReferenceKey.IsZero() && record.SettlementWindow != "")
	}
	if !hasReferenceEvidence {
		t.Fatalf("durable route settlement outbox rows = %+v, want settlement reference evidence", outbox)
	}

	ledger := gameServer.runtime.Settlements.RouteStorageLedgerEntries()
	if len(ledger) != wantLedgerRows {
		t.Fatalf("durable route settlement ledger rows = %+v, want %d", ledger, wantLedgerRows)
	}
	for _, row := range ledger {
		if _, ok := wantRoutes[row.RouteID]; !ok {
			t.Fatalf("durable route settlement ledger row = %+v, want route in %+v", row, routeIDs)
		}
		if row.ReferenceKey.IsZero() || row.SettlementWindow == "" {
			t.Fatalf("durable route settlement ledger row missing reference evidence: %+v", row)
		}
	}
}

func seedAutomationRouteToDestinationForTest(
	t *testing.T,
	gameServer *Server,
	ownerID foundation.PlayerID,
	routeID foundation.RouteID,
	sourcePlanetID foundation.PlanetID,
	destination production.RouteDestination,
	sourceMapID production.RouteMapID,
	destinationMapID production.RouteMapID,
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
	result, err := service.CreateRoute(production.CreateRouteInput{
		RouteID:        routeID,
		OwnerPlayerID:  ownerID,
		SourcePlanetID: sourcePlanetID,
		Destination:    destination,
		ResourceItemID: "refined_alloy",
		AmountPerHour:  40,
	})
	if err != nil {
		t.Fatalf("CreateRoute(%q) error = %v, want nil", routeID, err)
	}
	if result.Route.Destination != destination {
		t.Fatalf("seeded route destination = %+v, want %+v", result.Route.Destination, destination)
	}
	if result.Route.SourceMapID != sourceMapID || result.Route.DestinationMapID != destinationMapID {
		t.Fatalf("seeded route map ids = %q/%q, want %q/%q", result.Route.SourceMapID, result.Route.DestinationMapID, sourceMapID, destinationMapID)
	}
}

func assertRouteSettlementIDs(t *testing.T, settlements []routeSettlementPayload, routeIDs ...foundation.RouteID) {
	t.Helper()
	got := make(map[string]struct{}, len(settlements))
	for _, settlement := range settlements {
		got[settlement.RouteID] = struct{}{}
	}
	for _, routeID := range routeIDs {
		if _, ok := got[routeID.String()]; !ok {
			t.Fatalf("settlements = %+v, missing route %q", settlements, routeID)
		}
	}
	if len(got) != len(routeIDs) {
		t.Fatalf("settlements = %+v, want only %d settlements", settlements, len(routeIDs))
	}
}

func assertRouteSettlementPayloadOmitsServerTruth(t *testing.T, payload routeSettlementPayload, ownerID foundation.PlayerID, label string) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s: %v", label, err)
	}
	raw := string(data)
	for _, forbidden := range []string{
		`"owner`,
		ownerID.String(),
		`"source_planet_id"`,
		`"source_map_id"`,
		`"destination":`,
		`"destination_id"`,
		`"destination_planet_id"`,
		`"destination_map_id"`,
		`"loss_percent"`,
		`"internal`,
		"map_",
		`"world`,
		`"zone`,
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}

func assertPayloadOmitsPlayerOwner(t *testing.T, label string, payload any, ownerID foundation.PlayerID) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s: %v", label, err)
	}
	raw := string(data)
	for _, forbidden := range []string{`"owner_player_id"`, ownerID.String()} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}

func assertPayloadOmitsRouteEndpointID(t *testing.T, label string, payload any, endpointID string) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s: %v", label, err)
	}
	if strings.Contains(string(data), endpointID) {
		t.Fatalf("%s leaked route endpoint id %q in %s", label, endpointID, string(data))
	}
}

func assertStoredRouteCursor(t *testing.T, gameServer *Server, routeID foundation.RouteID, want time.Time) {
	t.Helper()
	stored, ok, err := gameServer.runtime.Production.AutomationRoute(routeID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok=%v err=%v, want stored route", routeID, ok, err)
	}
	if !stored.LastCalculatedAt.Equal(want.UTC()) {
		t.Fatalf("route %q LastCalculatedAt = %s, want %s", routeID, stored.LastCalculatedAt, want.UTC())
	}
}

func countEventType(events []realtime.EventEnvelope, eventType realtime.ClientEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}
