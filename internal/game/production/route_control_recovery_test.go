package production

import (
	"errors"
	"testing"
	"time"
)

func TestEnableRouteFutureRequestRestoresDurableRouteWithoutLiveRouteRow(t *testing.T) {
	last := testRouteNow()
	enableAt := last.Add(time.Hour)
	route := validSettlementRoute(last)
	route.Enabled = false
	store := newRouteSettlementStore(t, route, 100, []StoredItem{{ItemID: "refined_alloy", Quantity: 100}}, 100, nil)
	delete(store.routes, route.RouteID)

	service := newTestRouteSettlementService(t, store, enableAt, nil)
	result, err := service.EnableRouteForOwnerWithRequest("player-1", route.RouteID, "request-enable-future")
	if err != nil {
		t.Fatalf("EnableRoute(future) error = %v, want durable route repair", err)
	}
	if !result.Changed || !result.Route.Enabled || !result.Route.LastCalculatedAt.Equal(enableAt) {
		t.Fatalf("EnableRoute(future) = %+v, want changed enabled route at %s", result, enableAt)
	}
	assertRouteEnergyReserved(t, store, route.SourcePlanetID, 12)
	assertRouteDurableRecord(t, store, route.RouteID, "route_enable:player-1:route-1:request-enable-future", 2, result.Route)
}

func TestDisableRouteFutureRequestRestoresDurableRouteWithoutLiveRouteRow(t *testing.T) {
	last := testRouteNow()
	disableAt := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(t, route, 100, []StoredItem{{ItemID: "refined_alloy", Quantity: 100}}, 100, nil)
	delete(store.routes, route.RouteID)

	service := newTestRouteSettlementService(t, store, disableAt, nil)
	result, err := service.DisableRouteForOwnerWithRequest("player-1", route.RouteID, "request-disable-future")
	if err != nil {
		t.Fatalf("DisableRoute(future) error = %v, want durable route repair", err)
	}
	if !result.Changed || result.Route.Enabled || result.Settlement.AddedAmount != 40 {
		t.Fatalf("DisableRoute(future) = %+v, want disabled route and one settled window", result)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, disableAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, disableAt)
	assertRouteDurableRecord(t, store, route.RouteID, "route_disable:player-1:route-1:request-disable-future", 3, result.Route)
}

func TestUpdateRouteFutureRequestRestoresDurableRouteWithoutLiveRouteRow(t *testing.T) {
	last := testRouteNow()
	updateAt := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(t, route, 1_000, []StoredItem{{ItemID: "refined_alloy", Quantity: 1_000}}, 1_000, nil)
	delete(store.routes, route.RouteID)

	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	provider.policy.DestinationMapID = "map_1_2"
	input := validUpdateRouteInput()
	input.AmountPerHour = 80
	input.RequestID = "request-update-future"

	service := newTestRouteService(t, store, provider, updateAt)
	result, err := service.UpdateRoute(input)
	if err != nil {
		t.Fatalf("UpdateRoute(future) error = %v, want durable route repair", err)
	}
	if !result.Updated || result.Route.AmountPerHour != 80 || result.Settlement.AddedAmount != 40 {
		t.Fatalf("UpdateRoute(future) = %+v, want updated route and one settled window", result)
	}
	if provider.calls != 1 {
		t.Fatalf("policy calls = %d, want one", provider.calls)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 960, updateAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, updateAt)
	assertRouteDurableRecord(t, store, route.RouteID, "route_update:player-1:route-1:request-update-future", 3, result.Route)
}

func TestRouteControlDurableRestoreRejectsWrongOwner(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(t, route, 100, []StoredItem{{ItemID: "refined_alloy", Quantity: 100}}, 100, nil)
	delete(store.routes, route.RouteID)

	service := newTestRouteSettlementService(t, store, now, nil)
	_, err := service.DisableRouteForOwnerWithRequest("player-2", route.RouteID, "request-disable-wrong-owner")
	if !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("DisableRoute(wrong owner) error = %v, want ErrRouteOwnerMismatch", err)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}
