package production

import (
	"errors"
	"testing"
	"time"
)

func TestDisableRouteSettlesOldRouteBeforeDisabling(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.DisableRoute(route.RouteID)
	if err != nil {
		t.Fatalf("DisableRoute() error = %v, want nil", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	if result.Route.Enabled {
		t.Fatal("route Enabled = true, want false")
	}
	assertRouteMapIdentity(t, result.Route, route.SourceMapID, route.DestinationMapID)
	assertRouteMapIdentity(t, result.Settlement.BeforeRoute, route.SourceMapID, route.DestinationMapID)
	assertRouteMapIdentity(t, result.Settlement.AfterRoute, route.SourceMapID, route.DestinationMapID)
	if result.Settlement.AddedAmount != 40 || result.Settlement.TakenAmount != 40 {
		t.Fatalf("settlement taken/added = %d/%d, want 40/40", result.Settlement.TakenAmount, result.Settlement.AddedAmount)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, now)
	assertRouteEnabled(t, store, route.RouteID, false)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, now)
}

func TestDisableRouteForOwnerRejectsWrongOwnerWithoutMutation(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	_, err := service.DisableRouteForOwner("player-2", route.RouteID)
	if !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("DisableRouteForOwner() error = %v, want ErrRouteOwnerMismatch", err)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteEnabled(t, store, route.RouteID, true)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
	assertNoRouteEvents(t, store)
}

func TestEnableRouteResetsLastCalculatedAtSoDisabledElapsedDoesNotTransfer(t *testing.T) {
	last := testRouteNow()
	enableAt := last.Add(10 * time.Hour)
	settleAt := enableAt.Add(time.Hour)
	route := validSettlementRoute(last)
	route.Enabled = false
	store := newRouteSettlementStore(
		t,
		route,
		1_000,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 1_000}},
		1_000,
		nil,
	)
	enableService := newTestRouteSettlementService(t, store, enableAt, nil)

	enableResult, err := enableService.EnableRoute(route.RouteID)
	if err != nil {
		t.Fatalf("EnableRoute() error = %v, want nil", err)
	}
	if !enableResult.Changed || !enableResult.Route.Enabled {
		t.Fatalf("enable Changed/Enabled = %v/%v, want true/true", enableResult.Changed, enableResult.Route.Enabled)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, enableAt)

	settleService := newTestRouteSettlementService(t, store, settleAt, nil)
	settleResult, err := settleService.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if settleResult.AddedAmount != 40 || settleResult.ElapsedApplied != time.Hour {
		t.Fatalf("settlement added/applied = %d/%s, want 40/1h", settleResult.AddedAmount, settleResult.ElapsedApplied)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 960, settleAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, settleAt)
}

func TestEnableRouteForOwnerRejectsWrongOwnerWithoutMutation(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	route.Enabled = false
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	_, err := service.EnableRouteForOwner("player-2", route.RouteID)
	if !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("EnableRouteForOwner() error = %v, want ErrRouteOwnerMismatch", err)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteEnabled(t, store, route.RouteID, false)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
	assertNoRouteEvents(t, store)
}

func TestEnableRouteDoesNotMoveFutureTimestampBackward(t *testing.T) {
	now := testRouteNow()
	futureLast := now.Add(time.Hour)
	route := validSettlementRoute(now)
	route.Enabled = false
	route.LastCalculatedAt = futureLast
	route.UpdatedAt = futureLast
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.EnableRoute(route.RouteID)
	if err != nil {
		t.Fatalf("EnableRoute() error = %v, want nil", err)
	}
	if !result.Changed || !result.Route.Enabled {
		t.Fatalf("enable Changed/Enabled = %v/%v, want true/true", result.Changed, result.Route.Enabled)
	}
	stored, ok, err := store.AutomationRoute(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", route.RouteID, ok, err)
	}
	if !stored.LastCalculatedAt.Equal(futureLast) {
		t.Fatalf("LastCalculatedAt = %s, want unchanged future %s", stored.LastCalculatedAt, futureLast)
	}
	if !stored.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %s, want enable time %s", stored.UpdatedAt, now)
	}
}

func TestUpdateRouteSettlesOldAmountBeforeApplyingNewAmount(t *testing.T) {
	last := testRouteNow()
	updateAt := last.Add(time.Hour)
	settleAt := updateAt.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		1_000,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 1_000}},
		1_000,
		nil,
	)
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	provider.policy.DestinationMapID = "map_1_2"
	service := newTestRouteService(t, store, provider, updateAt)
	input := validUpdateRouteInput()
	input.AmountPerHour = 80

	result, err := service.UpdateRoute(input)
	if err != nil {
		t.Fatalf("UpdateRoute() error = %v, want nil", err)
	}
	if !result.Updated {
		t.Fatal("Updated = false, want true")
	}
	if result.Settlement.AddedAmount != 40 || result.Settlement.WantedAmount != 40 {
		t.Fatalf("settlement wanted/added = %d/%d, want 40/40", result.Settlement.WantedAmount, result.Settlement.AddedAmount)
	}
	if result.Route.AmountPerHour != 80 || result.Route.EnergyCostPerHour != provider.policy.EnergyCostPerHour {
		t.Fatalf("updated amount/energy = %d/%d, want 80/%d", result.Route.AmountPerHour, result.Route.EnergyCostPerHour, provider.policy.EnergyCostPerHour)
	}
	if result.Route.SourcePlanetID != route.SourcePlanetID || result.Route.OwnerPlayerID != route.OwnerPlayerID || !result.Route.CreatedAt.Equal(route.CreatedAt) {
		t.Fatalf("source/owner/created changed: %+v, want source %q owner %q created %s", result.Route, route.SourcePlanetID, route.OwnerPlayerID, route.CreatedAt)
	}
	assertRouteMapIdentity(t, result.Route, route.SourceMapID, provider.policy.DestinationMapID)
	if provider.calls != 1 || provider.lastInput.SourcePlanetID != route.SourcePlanetID || provider.lastInput.AmountPerHour != 80 {
		t.Fatalf("policy calls/input = %d/%+v, want one call with source %q amount 80", provider.calls, provider.lastInput, route.SourcePlanetID)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, updateAt)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 960, updateAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, updateAt)
	storedAfterUpdate, ok, err := store.AutomationRoute(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", route.RouteID, ok, err)
	}
	assertRouteMapIdentity(t, storedAfterUpdate, route.SourceMapID, provider.policy.DestinationMapID)

	settleService := newTestRouteSettlementService(t, store, settleAt, nil)
	settleResult, err := settleService.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if settleResult.AddedAmount != 80 || settleResult.WantedAmount != 80 {
		t.Fatalf("post-update wanted/added = %d/%d, want 80/80", settleResult.WantedAmount, settleResult.AddedAmount)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 880, settleAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 120, settleAt)
	settledRoute, ok, err := store.AutomationRoute(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", route.RouteID, ok, err)
	}
	assertRouteMapIdentity(t, settledRoute, route.SourceMapID, provider.policy.DestinationMapID)
}

func TestUpdateRouteRejectsSourceMapPolicyMismatchWithoutMutation(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	provider.policy.SourceMapID = "map_1_2"
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.AmountPerHour = 80

	_, err := service.UpdateRoute(input)
	if !errors.Is(err, ErrInvalidRouteMapID) {
		t.Fatalf("UpdateRoute() error = %v, want ErrInvalidRouteMapID", err)
	}
	assertRouteAmountAndTime(t, store, route.RouteID, 40, last)
	stored, ok, lookupErr := store.AutomationRoute(route.RouteID)
	if lookupErr != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", route.RouteID, ok, lookupErr)
	}
	assertRouteMapIdentity(t, stored, route.SourceMapID, route.DestinationMapID)
	assertNoRouteEvents(t, store)
}

func TestUpdateRouteRejectsOwnerMismatchWithoutMutation(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.OwnerPlayerID = "player-2"
	input.AmountPerHour = 80

	_, err := service.UpdateRoute(input)
	if !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("UpdateRoute() error = %v, want ErrRouteOwnerMismatch", err)
	}
	if provider.calls != 0 {
		t.Fatalf("policy calls = %d, want 0", provider.calls)
	}
	assertRouteAmountAndTime(t, store, route.RouteID, 40, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

func TestUpdateRouteForOwnerRejectsWrongOwnerWithoutMutation(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.OwnerPlayerID = "player-1"
	input.AmountPerHour = 80

	_, err := service.UpdateRouteForOwner("player-2", input)
	if !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("UpdateRouteForOwner() error = %v, want ErrRouteOwnerMismatch", err)
	}
	if provider.calls != 0 {
		t.Fatalf("policy calls = %d, want 0", provider.calls)
	}
	assertRouteAmountAndTime(t, store, route.RouteID, 40, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
	assertNoRouteEvents(t, store)
}

func TestUpdateRouteUnsupportedDestinationFailsBeforePolicyLookup(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.Destination = RouteDestination{Type: RouteDestinationTypeStorage, ID: "storage-1"}

	_, err := service.UpdateRoute(input)
	if !errors.Is(err, ErrUnsupportedRouteDestination) {
		t.Fatalf("UpdateRoute() error = %v, want ErrUnsupportedRouteDestination", err)
	}
	if provider.calls != 0 {
		t.Fatalf("policy calls = %d, want 0", provider.calls)
	}
	assertRouteAmountAndTime(t, store, route.RouteID, 40, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

func TestUpdateRouteRejectsInvalidPolicyWithoutMutation(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	policy := noLossRoutePolicy()
	policy.ResourceRouteable = false
	provider := &fakeRoutePolicyProvider{policy: policy}
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.AmountPerHour = 80

	_, err := service.UpdateRoute(input)
	if !errors.Is(err, ErrRouteResourceNotRouteable) {
		t.Fatalf("UpdateRoute() error = %v, want ErrRouteResourceNotRouteable", err)
	}
	if provider.calls != 1 {
		t.Fatalf("policy calls = %d, want 1", provider.calls)
	}
	assertRouteAmountAndTime(t, store, route.RouteID, 40, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

func TestDisableRouteDuplicateIsIdempotent(t *testing.T) {
	last := testRouteNow()
	firstDisableAt := last.Add(time.Hour)
	secondDisableAt := firstDisableAt.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)

	firstService := newTestRouteSettlementService(t, store, firstDisableAt, nil)
	first, err := firstService.DisableRoute(route.RouteID)
	if err != nil {
		t.Fatalf("DisableRoute(first) error = %v, want nil", err)
	}
	if !first.Changed || first.Settlement.AddedAmount != 40 {
		t.Fatalf("first Changed/added = %v/%d, want true/40", first.Changed, first.Settlement.AddedAmount)
	}

	secondService := newTestRouteSettlementService(t, store, secondDisableAt, nil)
	second, err := secondService.DisableRoute(route.RouteID)
	if err != nil {
		t.Fatalf("DisableRoute(second) error = %v, want nil", err)
	}
	if second.Changed {
		t.Fatal("second Changed = true, want false")
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, firstDisableAt)
	assertRouteEnabled(t, store, route.RouteID, false)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, firstDisableAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, firstDisableAt)
}

func TestUpdateRouteFutureTimestampSafe(t *testing.T) {
	now := testRouteNow()
	futureLast := now.Add(time.Hour)
	route := validSettlementRoute(now)
	route.LastCalculatedAt = futureLast
	route.UpdatedAt = futureLast
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.AmountPerHour = 80

	result, err := service.UpdateRoute(input)
	if err != nil {
		t.Fatalf("UpdateRoute() error = %v, want nil", err)
	}
	if !result.Settlement.NoOp || result.Settlement.ElapsedApplied != 0 {
		t.Fatalf("settlement NoOp/applied = %v/%s, want true/0", result.Settlement.NoOp, result.Settlement.ElapsedApplied)
	}
	if result.Route.AmountPerHour != 80 {
		t.Fatalf("AmountPerHour = %d, want 80", result.Route.AmountPerHour)
	}
	stored, ok, err := store.AutomationRoute(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", route.RouteID, ok, err)
	}
	if !stored.LastCalculatedAt.Equal(futureLast) {
		t.Fatalf("LastCalculatedAt = %s, want unchanged future %s", stored.LastCalculatedAt, futureLast)
	}
	if !stored.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %s, want update time %s", stored.UpdatedAt, now)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, futureLast)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, futureLast)
}
