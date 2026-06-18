package production

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestCreateRouteValidatesSourceOwnership(t *testing.T) {
	store := NewInMemoryStore()
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	provider.policy.SourcePlanetOwned = false
	service := newTestRouteService(t, store, provider, testRouteNow())

	_, err := service.CreateRoute(validCreateRouteInput())
	if !errors.Is(err, ErrRouteSourceNotOwned) {
		t.Fatalf("CreateRoute() error = %v, want ErrRouteSourceNotOwned", err)
	}
}

func TestCreateRouteUnauthorizedDestinationFails(t *testing.T) {
	store := NewInMemoryStore()
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	provider.policy.DestinationAccessible = false
	service := newTestRouteService(t, store, provider, testRouteNow())

	_, err := service.CreateRoute(validCreateRouteInput())
	if !errors.Is(err, ErrRouteDestinationNotAccessible) {
		t.Fatalf("CreateRoute() error = %v, want ErrRouteDestinationNotAccessible", err)
	}
}

func TestCreateRouteNonRouteableResourceFails(t *testing.T) {
	store := NewInMemoryStore()
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	provider.policy.ResourceRouteable = false
	service := newTestRouteService(t, store, provider, testRouteNow())

	_, err := service.CreateRoute(validCreateRouteInput())
	if !errors.Is(err, ErrRouteResourceNotRouteable) {
		t.Fatalf("CreateRoute() error = %v, want ErrRouteResourceNotRouteable", err)
	}
}

func TestCreateRouteNonPositiveRateFailsBeforePolicyLookup(t *testing.T) {
	store := NewInMemoryStore()
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	service := newTestRouteService(t, store, provider, testRouteNow())
	input := validCreateRouteInput()
	input.AmountPerHour = 0

	_, err := service.CreateRoute(input)
	if !errors.Is(err, ErrInvalidRouteRate) {
		t.Fatalf("CreateRoute() error = %v, want ErrInvalidRouteRate", err)
	}
	if provider.calls != 0 {
		t.Fatalf("policy calls = %d, want 0", provider.calls)
	}
}

func TestCreateRouteRequirementFailureFails(t *testing.T) {
	store := NewInMemoryStore()
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	provider.policy.RequirementsMet = false
	service := newTestRouteService(t, store, provider, testRouteNow())

	_, err := service.CreateRoute(validCreateRouteInput())
	if !errors.Is(err, ErrRouteRequirementNotMet) {
		t.Fatalf("CreateRoute() error = %v, want ErrRouteRequirementNotMet", err)
	}
}

func TestCreateRouteDistanceAndRiskCalculation(t *testing.T) {
	t.Run("distance over max fails", func(t *testing.T) {
		store := NewInMemoryStore()
		provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
		provider.policy.DistanceUnits = 1_001
		provider.policy.MaxDistanceUnits = 1_000
		service := newTestRouteService(t, store, provider, testRouteNow())

		_, err := service.CreateRoute(validCreateRouteInput())
		if !errors.Is(err, ErrRouteDistanceTooFar) {
			t.Fatalf("CreateRoute() error = %v, want ErrRouteDistanceTooFar", err)
		}
	})

	t.Run("loss chance and loss range clamp", func(t *testing.T) {
		store := NewInMemoryStore()
		provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
		provider.policy.DistanceUnits = 9_000
		provider.policy.MaxDistanceUnits = 10_000
		provider.policy.BaseLossChance = 0.39
		provider.policy.DistanceLossChancePerUnit = 0.001
		provider.policy.MinLossPercent = -0.25
		provider.policy.MaxLossPercent = 1.25
		service := newTestRouteService(t, store, provider, testRouteNow())

		result, err := service.CreateRoute(validCreateRouteInput())
		if err != nil {
			t.Fatalf("CreateRoute() error = %v, want nil", err)
		}
		if result.Route.Risk.LossChance != MaxRouteLossChance {
			t.Fatalf("LossChance = %.4f, want %.4f", result.Route.Risk.LossChance, MaxRouteLossChance)
		}
		if result.Route.Risk.MinLossPercent != 0 || result.Route.Risk.MaxLossPercent != 1 {
			t.Fatalf("loss range = %.4f..%.4f, want 0..1", result.Route.Risk.MinLossPercent, result.Route.Risk.MaxLossPercent)
		}
	})
}

func TestCreateRouteStoresDetachedEnabledRoute(t *testing.T) {
	store := NewInMemoryStore()
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	now := time.Date(2026, 6, 18, 15, 30, 0, 0, time.FixedZone("UTC+3", 3*60*60))
	service := newTestRouteService(t, store, provider, now)
	input := validCreateRouteInput()

	result, err := service.CreateRoute(input)
	if err != nil {
		t.Fatalf("CreateRoute() error = %v, want nil", err)
	}
	if !result.Created {
		t.Fatal("Created = false, want true")
	}
	if provider.calls != 1 || provider.lastInput.OwnerPlayerID != input.OwnerPlayerID || provider.lastInput.AmountPerHour != input.AmountPerHour {
		t.Fatalf("policy calls/input = %d/%+v, want one matching input", provider.calls, provider.lastInput)
	}
	wantTime := now.UTC()
	if !result.Route.Enabled {
		t.Fatal("route Enabled = false, want true")
	}
	if !result.Route.LastCalculatedAt.Equal(wantTime) || !result.Route.CreatedAt.Equal(wantTime) || !result.Route.UpdatedAt.Equal(wantTime) {
		t.Fatalf("route timestamps = %s/%s/%s, want %s", result.Route.LastCalculatedAt, result.Route.CreatedAt, result.Route.UpdatedAt, wantTime)
	}
	if result.Route.LastCalculatedAt.Location() != time.UTC || result.Route.CreatedAt.Location() != time.UTC || result.Route.UpdatedAt.Location() != time.UTC {
		t.Fatalf("timestamp locations = %v/%v/%v, want UTC", result.Route.LastCalculatedAt.Location(), result.Route.CreatedAt.Location(), result.Route.UpdatedAt.Location())
	}
	if result.Route.EnergyCostPerHour != provider.policy.EnergyCostPerHour {
		t.Fatalf("EnergyCostPerHour = %d, want %d", result.Route.EnergyCostPerHour, provider.policy.EnergyCostPerHour)
	}

	result.Route.Enabled = false
	stored, ok, err := store.AutomationRoute(input.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute() ok = %v err = %v, want true nil", ok, err)
	}
	if !stored.Enabled {
		t.Fatal("mutating result changed stored route Enabled to false")
	}
	stored.AmountPerHour = 999
	storedAgain, ok, err := store.AutomationRoute(input.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(second) ok = %v err = %v, want true nil", ok, err)
	}
	if storedAgain.AmountPerHour != input.AmountPerHour {
		t.Fatalf("stored AmountPerHour = %d, want %d", storedAgain.AmountPerHour, input.AmountPerHour)
	}
}

func TestCreateRouteDuplicateRouteIDFailsWithoutOverwrite(t *testing.T) {
	store := NewInMemoryStore()
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	service := newTestRouteService(t, store, provider, testRouteNow())
	input := validCreateRouteInput()

	first, err := service.CreateRoute(input)
	if err != nil {
		t.Fatalf("CreateRoute(first) error = %v, want nil", err)
	}
	provider.policy.EnergyCostPerHour = 99
	input.AmountPerHour = 80

	_, err = service.CreateRoute(input)
	if !errors.Is(err, ErrDuplicateRoute) {
		t.Fatalf("CreateRoute(second) error = %v, want ErrDuplicateRoute", err)
	}
	stored, ok, err := store.AutomationRoute(first.Route.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute() ok = %v err = %v, want true nil", ok, err)
	}
	if stored.AmountPerHour != first.Route.AmountPerHour || stored.EnergyCostPerHour != first.Route.EnergyCostPerHour {
		t.Fatalf("stored route = %+v, want original amount %d energy %d", stored, first.Route.AmountPerHour, first.Route.EnergyCostPerHour)
	}
}

func TestSettleRouteMissingRouteReturnsClearError(t *testing.T) {
	store := NewInMemoryStore()
	service := newTestRouteSettlementService(t, store, testRouteNow(), nil)

	_, err := service.SettleRoute("route-missing")
	if !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("SettleRoute() error = %v, want ErrRouteNotFound", err)
	}
}

func TestSettleRouteEmptySourceTransfersZeroAndUpdatesTimestamps(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(t, route, 100, nil, 100, nil)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if result.NoOp {
		t.Fatal("NoOp = true, want false because route timestamp advances")
	}
	if result.WantedAmount != 40 || result.TakenAmount != 0 || result.LostAmount != 0 || result.DeliveredAmount != 0 || result.AddedAmount != 0 {
		t.Fatalf("amounts = wanted %d taken %d lost %d delivered %d added %d, want 40/0/0/0/0",
			result.WantedAmount, result.TakenAmount, result.LostAmount, result.DeliveredAmount, result.AddedAmount)
	}
	if !result.SourceEmpty {
		t.Fatal("SourceEmpty = false, want true")
	}
	if result.DestinationFull || result.LossApplied {
		t.Fatalf("DestinationFull/LossApplied = %v/%v, want false/false", result.DestinationFull, result.LossApplied)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, now)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 0, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, now)
}

func TestSettleRouteFullDestinationClampsDelivery(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		10,
		[]StoredItem{{ItemID: "void_salt", Quantity: 10}},
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if result.WantedAmount != 40 || result.TakenAmount != 40 || result.DeliveredAmount != 40 || result.AddedAmount != 0 {
		t.Fatalf("amounts = wanted %d taken %d delivered %d added %d, want 40/40/40/0",
			result.WantedAmount, result.TakenAmount, result.DeliveredAmount, result.AddedAmount)
	}
	if !result.DestinationFull {
		t.Fatal("DestinationFull = false, want true")
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, now)
}

func TestSettleRouteLossChanceAppliesInConfiguredRange(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	route.Risk = RouteRisk{
		LossChance:     MaxRouteLossChance,
		MinLossPercent: 0.25,
		MaxLossPercent: 0.75,
	}
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, testutil.NewFakeRNG(nil, []float64{0.10, 0.50}))

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if !result.LossApplied {
		t.Fatal("LossApplied = false, want true")
	}
	if result.LossPercent != 0.50 {
		t.Fatalf("LossPercent = %.2f, want 0.50", result.LossPercent)
	}
	if result.LostAmount != 20 || result.DeliveredAmount != 20 || result.AddedAmount != 20 {
		t.Fatalf("loss amounts = lost %d delivered %d added %d, want 20/20/20",
			result.LostAmount, result.DeliveredAmount, result.AddedAmount)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 20, now)
}

func TestSettleRouteDoubleSettlementDoesNotDuplicateTransfer(t *testing.T) {
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

	first, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute(first) error = %v, want nil", err)
	}
	second, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute(second) error = %v, want nil", err)
	}
	if first.AddedAmount != 40 {
		t.Fatalf("first AddedAmount = %d, want 40", first.AddedAmount)
	}
	if !second.NoOp || second.AddedAmount != 0 || second.ElapsedApplied != 0 {
		t.Fatalf("second NoOp/added/applied = %v/%d/%s, want true/0/0", second.NoOp, second.AddedAmount, second.ElapsedApplied)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, now)
}

func TestSettleRouteFutureTimestampSafe(t *testing.T) {
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
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if !result.NoOp || result.ElapsedApplied != 0 {
		t.Fatalf("NoOp/applied = %v/%s, want true/0", result.NoOp, result.ElapsedApplied)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, futureLast)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, futureLast)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, futureLast)
}

func TestSettleRouteDisabledRouteNoOpPreservesTimestamp(t *testing.T) {
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

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if !result.NoOp || result.ElapsedRequested != time.Hour || result.ElapsedApplied != 0 {
		t.Fatalf("NoOp/requested/applied = %v/%s/%s, want true/1h/0", result.NoOp, result.ElapsedRequested, result.ElapsedApplied)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

func TestSettleRouteUnsupportedDestinationReturnsErrorWithoutMutation(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	route.Destination = RouteDestination{Type: RouteDestinationTypeStorage, ID: "storage-1"}
	store := NewInMemoryStore()
	insertRouteSettlementRoute(t, store, route)
	saveRouteSettlementStorage(t, store, "planet-1", 100, []StoredItem{{ItemID: "refined_alloy", Quantity: 100}}, last)
	service := newTestRouteSettlementService(t, store, now, nil)

	_, err := service.SettleRoute(route.RouteID)
	if !errors.Is(err, ErrUnsupportedRouteDestination) {
		t.Fatalf("SettleRoute() error = %v, want ErrUnsupportedRouteDestination", err)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
}

func TestSettleRouteOfflineCapApplies(t *testing.T) {
	now := testRouteNow()
	last := now.Add(-100 * time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		5_000,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 5_000}},
		5_000,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if result.ElapsedRequested != 100*time.Hour {
		t.Fatalf("ElapsedRequested = %s, want 100h", result.ElapsedRequested)
	}
	if result.ElapsedApplied != DefaultMaxRouteOfflineSettlementDuration {
		t.Fatalf("ElapsedApplied = %s, want %s", result.ElapsedApplied, DefaultMaxRouteOfflineSettlementDuration)
	}
	if result.WantedAmount != 2_880 || result.AddedAmount != 2_880 {
		t.Fatalf("wanted/added = %d/%d, want 2880/2880", result.WantedAmount, result.AddedAmount)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, now)
}

func TestSettleRouteSubUnitWantedAdvancesRouteTimestampWithoutTransfer(t *testing.T) {
	last := testRouteNow()
	now := last.Add(30 * time.Minute)
	route := validSettlementRoute(last)
	route.AmountPerHour = 1
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 10}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if result.NoOp || result.WantedAmount != 0 || result.TakenAmount != 0 || result.AddedAmount != 0 {
		t.Fatalf("NoOp/wanted/taken/added = %v/%d/%d/%d, want false/0/0/0",
			result.NoOp, result.WantedAmount, result.TakenAmount, result.AddedAmount)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, now)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 10, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

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
	if result.Settlement.AddedAmount != 40 || result.Settlement.TakenAmount != 40 {
		t.Fatalf("settlement taken/added = %d/%d, want 40/40", result.Settlement.TakenAmount, result.Settlement.AddedAmount)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, now)
	assertRouteEnabled(t, store, route.RouteID, false)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, now)
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
	if provider.calls != 1 || provider.lastInput.SourcePlanetID != route.SourcePlanetID || provider.lastInput.AmountPerHour != 80 {
		t.Fatalf("policy calls/input = %d/%+v, want one call with source %q amount 80", provider.calls, provider.lastInput, route.SourcePlanetID)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, updateAt)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 960, updateAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, updateAt)

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

type fakeRoutePolicyProvider struct {
	policy    RouteCreatePolicy
	err       error
	calls     int
	lastInput RouteCreatePolicyInput
}

func (provider *fakeRoutePolicyProvider) RouteCreatePolicy(input RouteCreatePolicyInput) (RouteCreatePolicy, error) {
	provider.calls++
	provider.lastInput = input
	if provider.err != nil {
		return RouteCreatePolicy{}, provider.err
	}
	return provider.policy, nil
}

type fixedRouteClock struct {
	now time.Time
}

func (clock fixedRouteClock) Now() time.Time {
	return clock.now
}

func newTestRouteService(
	t *testing.T,
	store *InMemoryStore,
	provider RouteCreatePolicyProvider,
	now time.Time,
) *AutomationRouteService {
	t.Helper()
	service, err := NewAutomationRouteService(AutomationRouteServiceConfig{
		Store:  store,
		Clock:  fixedRouteClock{now: now},
		Policy: provider,
	})
	if err != nil {
		t.Fatalf("NewAutomationRouteService() error = %v, want nil", err)
	}
	return service
}

func newTestRouteSettlementService(
	t *testing.T,
	store *InMemoryStore,
	now time.Time,
	lossRoller RouteLossRoller,
) *AutomationRouteService {
	t.Helper()
	service, err := NewAutomationRouteService(AutomationRouteServiceConfig{
		Store:      store,
		Clock:      fixedRouteClock{now: now},
		Policy:     &fakeRoutePolicyProvider{policy: validRoutePolicy()},
		LossRoller: lossRoller,
	})
	if err != nil {
		t.Fatalf("NewAutomationRouteService() error = %v, want nil", err)
	}
	return service
}

func validCreateRouteInput() CreateRouteInput {
	return CreateRouteInput{
		RouteID:        "route-1",
		OwnerPlayerID:  "player-1",
		SourcePlanetID: "planet-1",
		Destination: RouteDestination{
			Type: RouteDestinationTypePlanet,
			ID:   "planet-2",
		},
		ResourceItemID: "refined_alloy",
		AmountPerHour:  40,
	}
}

func validUpdateRouteInput() UpdateRouteInput {
	return UpdateRouteInput{
		RouteID:       "route-1",
		OwnerPlayerID: "player-1",
		Destination: RouteDestination{
			Type: RouteDestinationTypePlanet,
			ID:   "planet-2",
		},
		ResourceItemID: "refined_alloy",
		AmountPerHour:  40,
	}
}

func validRoutePolicy() RouteCreatePolicy {
	return RouteCreatePolicy{
		SourcePlanetOwned:         true,
		DestinationAccessible:     true,
		ResourceRouteable:         true,
		RequirementsMet:           true,
		DistanceUnits:             100,
		MaxDistanceUnits:          1_000,
		BaseLossChance:            0.02,
		DistanceLossChancePerUnit: 0.0001,
		SourceRegionRisk:          0.02,
		DestinationRegionRisk:     0.01,
		DeepSpaceRisk:             0,
		PlayerLossReduction:       0.005,
		RouteSecurityReduction:    0.005,
		MinLossPercent:            0.25,
		MaxLossPercent:            0.75,
		EnergyCostPerHour:         12,
	}
}

func noLossRoutePolicy() RouteCreatePolicy {
	policy := validRoutePolicy()
	policy.BaseLossChance = 0
	policy.DistanceLossChancePerUnit = 0
	policy.SourceRegionRisk = 0
	policy.DestinationRegionRisk = 0
	policy.DeepSpaceRisk = 0
	policy.PlayerLossReduction = 0
	policy.RouteSecurityReduction = 0
	policy.MinLossPercent = 0
	policy.MaxLossPercent = 0
	policy.EnergyCostPerHour = 24
	return policy
}

func validSettlementRoute(lastCalculatedAt time.Time) AutomationRoute {
	return AutomationRoute{
		RouteID:        "route-1",
		OwnerPlayerID:  "player-1",
		SourcePlanetID: "planet-1",
		Destination: RouteDestination{
			Type: RouteDestinationTypePlanet,
			ID:   "planet-2",
		},
		ResourceItemID:    "refined_alloy",
		AmountPerHour:     40,
		EnergyCostPerHour: 12,
		Risk: RouteRisk{
			LossChance:     0,
			MinLossPercent: 0.25,
			MaxLossPercent: 0.75,
		},
		Enabled:          true,
		LastCalculatedAt: lastCalculatedAt,
		CreatedAt:        lastCalculatedAt,
		UpdatedAt:        lastCalculatedAt,
	}
}

func assertRouteEnabled(
	t *testing.T,
	store *InMemoryStore,
	routeID foundation.RouteID,
	enabled bool,
) {
	t.Helper()
	route, ok, err := store.AutomationRoute(routeID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", routeID, ok, err)
	}
	if route.Enabled != enabled {
		t.Fatalf("route Enabled = %v, want %v", route.Enabled, enabled)
	}
}

func assertRouteAmountAndTime(
	t *testing.T,
	store *InMemoryStore,
	routeID foundation.RouteID,
	amountPerHour int64,
	lastCalculatedAt time.Time,
) {
	t.Helper()
	route, ok, err := store.AutomationRoute(routeID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", routeID, ok, err)
	}
	if route.AmountPerHour != amountPerHour {
		t.Fatalf("route AmountPerHour = %d, want %d", route.AmountPerHour, amountPerHour)
	}
	if !route.LastCalculatedAt.Equal(lastCalculatedAt) || !route.UpdatedAt.Equal(lastCalculatedAt) {
		t.Fatalf("route timestamps = %s/%s, want %s", route.LastCalculatedAt, route.UpdatedAt, lastCalculatedAt)
	}
}

func newRouteSettlementStore(
	t *testing.T,
	route AutomationRoute,
	sourceCapacity int64,
	sourceItems []StoredItem,
	destinationCapacity int64,
	destinationItems []StoredItem,
) *InMemoryStore {
	t.Helper()
	store := NewInMemoryStore()
	insertRouteSettlementRoute(t, store, route)
	saveRouteSettlementStorage(t, store, route.SourcePlanetID, sourceCapacity, sourceItems, route.UpdatedAt)
	if route.Destination.Type == RouteDestinationTypePlanet {
		saveRouteSettlementStorage(t, store, foundation.PlanetID(route.Destination.ID), destinationCapacity, destinationItems, route.UpdatedAt)
	}
	return store
}

func insertRouteSettlementRoute(t *testing.T, store *InMemoryStore, route AutomationRoute) {
	t.Helper()
	if _, err := store.insertAutomationRoute(route); err != nil {
		t.Fatalf("insertAutomationRoute() error = %v, want nil", err)
	}
}

func saveRouteSettlementStorage(
	t *testing.T,
	store *InMemoryStore,
	planetID foundation.PlanetID,
	capacity int64,
	items []StoredItem,
	updatedAt time.Time,
) {
	t.Helper()
	storage, err := NewPlanetStorage(planetID, capacity, items, updatedAt)
	if err != nil {
		t.Fatalf("NewPlanetStorage(%q) error = %v, want nil", planetID, err)
	}
	if err := store.SavePlanetStorage(storage); err != nil {
		t.Fatalf("SavePlanetStorage(%q) error = %v, want nil", planetID, err)
	}
}

func assertRouteSettlementRouteTime(
	t *testing.T,
	store *InMemoryStore,
	routeID foundation.RouteID,
	lastCalculatedAt time.Time,
) {
	t.Helper()
	route, ok, err := store.AutomationRoute(routeID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", routeID, ok, err)
	}
	if !route.LastCalculatedAt.Equal(lastCalculatedAt) || !route.UpdatedAt.Equal(lastCalculatedAt) {
		t.Fatalf("route timestamps = %s/%s, want %s", route.LastCalculatedAt, route.UpdatedAt, lastCalculatedAt)
	}
}

func assertRouteSettlementStorage(
	t *testing.T,
	store *InMemoryStore,
	planetID foundation.PlanetID,
	itemID foundation.ItemID,
	quantity int64,
	updatedAt time.Time,
) {
	t.Helper()
	storage, ok, err := store.PlanetStorage(planetID)
	if err != nil || !ok {
		t.Fatalf("PlanetStorage(%q) ok = %v err = %v, want true nil", planetID, ok, err)
	}
	if got := storage.QuantityOf(itemID); got != quantity {
		t.Fatalf("storage %q item %q = %d, want %d", planetID, itemID, got, quantity)
	}
	if !storage.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("storage %q UpdatedAt = %s, want %s", planetID, storage.UpdatedAt, updatedAt)
	}
}

func testRouteNow() time.Time {
	return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
}

var _ RouteCreatePolicyProvider = (*fakeRoutePolicyProvider)(nil)
var _ foundation.Clock = fixedRouteClock{}
