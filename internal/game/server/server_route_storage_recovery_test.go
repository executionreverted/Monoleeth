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

func TestRouteSettleRestoresMissingStorageReadModelsFromDurableRows(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-storage-repair@example.com", "Route Storage Repair")
	sourcePlanetID := foundation.PlanetID("planet-route-storage-repair-source")
	destinationPlanetID := foundation.PlanetID("planet-route-storage-repair-destination")
	routeID := foundation.RouteID("route-storage-repair")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-storage-repair-source")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-storage-repair-destination")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	clock.Advance(time.Hour)

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-storage-repair-first","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if first.HasError {
		t.Fatalf("first route.settle response error = %+v, want success", first.Error)
	}
	if err := gameServer.runtime.Production.DropPlanetStorageReadModel(sourcePlanetID); err != nil {
		t.Fatalf("DropPlanetStorageReadModel(source) error = %v, want nil", err)
	}
	if err := gameServer.runtime.Production.DropPlanetStorageReadModel(destinationPlanetID); err != nil {
		t.Fatalf("DropPlanetStorageReadModel(destination) error = %v, want nil", err)
	}
	clock.Advance(time.Hour)

	repaired := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-storage-repair-second","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":2,"v":1}`),
	)
	if repaired.HasError {
		t.Fatalf("repaired route.settle response error = %+v, want durable storage recovery success", repaired.Error)
	}
	var payload struct {
		Settlement routeSettlementPayload            `json:"settlement"`
		Storage    planetStorageCollectionPayload    `json:"storage"`
		Production planetProductionCollectionPayload `json:"production"`
	}
	if err := json.Unmarshal(repaired.Response.Payload, &payload); err != nil {
		t.Fatalf("decode repaired route.settle payload: %v", err)
	}
	assertRouteSettlementPayload(t, payload.Settlement, routeID, "refined_alloy", 3_600_000, 40, 40, 0, 40, 40, false, "repaired route.settle response")
	assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 20)
	assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "refined_alloy", 80)
	assertRouteControlActiveMapSnapshots(t, payload.Production, payload.Storage, sourcePlanetID, 20)
}

func TestRouteSettleRestoresMissingRouteAndStorageReadModelsForFutureWindow(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "route-full-repair@example.com", "Route Full Repair")
	sourcePlanetID := foundation.PlanetID("planet-route-full-repair-source")
	destinationPlanetID := foundation.PlanetID("planet-route-full-repair-destination")
	routeID := foundation.RouteID("route-full-repair")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, sourcePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-route-full-repair-source")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-route-full-repair-destination")
	saveRouteControlStorage(t, gameServer, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, gameServer, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	clock.Advance(time.Hour)

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-full-repair-first","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":1,"v":1}`),
	)
	if first.HasError {
		t.Fatalf("first route.settle response error = %+v, want success", first.Error)
	}
	if err := gameServer.runtime.Production.DropAutomationRouteReadModel(routeID); err != nil {
		t.Fatalf("DropAutomationRouteReadModel() error = %v, want nil", err)
	}
	if err := gameServer.runtime.Production.DropPlanetStorageReadModel(sourcePlanetID); err != nil {
		t.Fatalf("DropPlanetStorageReadModel(source) error = %v, want nil", err)
	}
	if err := gameServer.runtime.Production.DropPlanetStorageReadModel(destinationPlanetID); err != nil {
		t.Fatalf("DropPlanetStorageReadModel(destination) error = %v, want nil", err)
	}
	clock.Advance(time.Hour)

	repaired := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-route-full-repair-second","op":"route.settle","payload":{"route_id":"`+routeID.String()+`"},"client_seq":2,"v":1}`),
	)
	if repaired.HasError {
		t.Fatalf("repaired route.settle response error = %+v, want durable route/storage recovery success", repaired.Error)
	}
	var payload struct {
		Settlement routeSettlementPayload         `json:"settlement"`
		Routes     routeListPayload               `json:"routes"`
		Storage    planetStorageCollectionPayload `json:"storage"`
	}
	if err := json.Unmarshal(repaired.Response.Payload, &payload); err != nil {
		t.Fatalf("decode repaired route.settle payload: %v", err)
	}
	assertRouteSettlementPayload(t, payload.Settlement, routeID, "refined_alloy", 3_600_000, 40, 40, 0, 40, 40, false, "full repaired route.settle response")
	if len(payload.Routes.Routes) != 1 || payload.Routes.Routes[0].RouteID != routeID.String() {
		t.Fatalf("repaired route list = %+v, want route %q", payload.Routes.Routes, routeID)
	}
	assertStoredRouteStorageQuantity(t, gameServer, sourcePlanetID, "refined_alloy", 20)
	assertStoredRouteStorageQuantity(t, gameServer, destinationPlanetID, "refined_alloy", 80)
}
