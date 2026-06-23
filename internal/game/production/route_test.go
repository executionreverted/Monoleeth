package production

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
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

func TestCreateRouteUnsupportedDestinationFailsBeforePolicyLookup(t *testing.T) {
	store := NewInMemoryStore()
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	service := newTestRouteService(t, store, provider, testRouteNow())
	input := validCreateRouteInput()
	input.Destination = RouteDestination{Type: RouteDestinationTypeStation, ID: "station-1"}

	_, err := service.CreateRoute(input)
	if !errors.Is(err, ErrUnsupportedRouteDestination) {
		t.Fatalf("CreateRoute() error = %v, want ErrUnsupportedRouteDestination", err)
	}
	if provider.calls != 0 {
		t.Fatalf("policy calls = %d, want 0", provider.calls)
	}
}

func TestCreateRouteStorageDestinationUsesPolicyAndPersistsRoute(t *testing.T) {
	store := NewInMemoryStore()
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	provider.policy.DestinationMapID = "map_storage"
	service := newTestRouteService(t, store, provider, testRouteNow())
	input := validCreateRouteInput()
	input.Destination = RouteDestination{Type: RouteDestinationTypeStorage, ID: "storage-1"}

	result, err := service.CreateRoute(input)
	if err != nil {
		t.Fatalf("CreateRoute(storage destination) error = %v, want nil", err)
	}
	if provider.calls != 1 || provider.lastInput.Destination != input.Destination {
		t.Fatalf("policy calls/input = %d/%+v, want storage destination policy lookup", provider.calls, provider.lastInput)
	}
	if result.Route.Destination != input.Destination {
		t.Fatalf("created route destination = %+v, want %+v", result.Route.Destination, input.Destination)
	}
	assertRouteMapIdentity(t, result.Route, "map_1_1", "map_storage")
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

func TestCreateRouteCapacityExceededFailsBeforeMutation(t *testing.T) {
	store := NewInMemoryStore()
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	provider.policy.CurrentRouteCount = 3
	provider.policy.MaxRouteCount = 3
	service := newTestRouteService(t, store, provider, testRouteNow())

	_, err := service.CreateRoute(validCreateRouteInput())
	if !errors.Is(err, ErrRouteCapacityExceeded) {
		t.Fatalf("CreateRoute() error = %v, want ErrRouteCapacityExceeded", err)
	}
	if routes := store.AutomationRoutes(); len(routes) != 0 {
		t.Fatalf("routes after capacity failure = %+v, want none", routes)
	}
}

func TestCreateRouteReservesSourceEnergy(t *testing.T) {
	store := NewInMemoryStore()
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	provider.policy.EnergyCostPerHour = 12
	service := newTestRouteService(t, store, provider, testRouteNow())

	result, err := service.CreateRoute(validCreateRouteInput())
	if err != nil {
		t.Fatalf("CreateRoute() error = %v, want nil", err)
	}
	assertRouteEnergyReserved(t, store, result.Route.SourcePlanetID, 12)
}

func TestCreateRouteSettlesSourceProductionBeforeReservingEnergy(t *testing.T) {
	store := NewInMemoryStore()
	start := testTime(0)
	now := start.Add(time.Hour)
	if _, err := store.InitializePlanetProduction(InitializePlanetProductionInput{
		PlanetID:              "planet-1",
		LastCalculatedAt:      start,
		StorageCapacityUnits:  100,
		EnergyCapacityPerHour: 16,
		UpdatedAt:             start,
	}); err != nil {
		t.Fatalf("InitializePlanetProduction() error = %v, want nil", err)
	}
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	provider.policy.EnergyCostPerHour = 12
	service := newTestRouteService(t, store, provider, now)

	result, err := service.CreateRoute(validCreateRouteInput())
	if err != nil {
		t.Fatalf("CreateRoute() error = %v, want nil", err)
	}
	snapshot, ok, err := store.Snapshot(result.Route.SourcePlanetID)
	if err != nil || !ok {
		t.Fatalf("Snapshot(%q) ok=%v err=%v, want true nil", result.Route.SourcePlanetID, ok, err)
	}
	if got := snapshot.Storage.QuantityOf("iron_ore"); got != 30 {
		t.Fatalf("source iron_ore after route create = %d, want 30 from pre-route production window", got)
	}
	if !snapshot.State.LastCalculatedAt.Equal(now.UTC()) {
		t.Fatalf("LastCalculatedAt = %s, want %s", snapshot.State.LastCalculatedAt, now.UTC())
	}
	if snapshot.State.EnergyReservedPerHour != result.Route.EnergyCostPerHour {
		t.Fatalf("EnergyReservedPerHour = %d, want route cost %d", snapshot.State.EnergyReservedPerHour, result.Route.EnergyCostPerHour)
	}
}

func TestCreateRouteRejectsEnergyReservationOverCapacityWithoutMutation(t *testing.T) {
	store := NewInMemoryStore()
	now := testRouteNow()
	setRouteEnergyStateForTest(t, store, "planet-1", 40, 35, now)
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	provider.policy.EnergyCostPerHour = 12
	service := newTestRouteService(t, store, provider, now)

	_, err := service.CreateRoute(validCreateRouteInput())
	if !errors.Is(err, ErrRouteEnergyUnavailable) {
		t.Fatalf("CreateRoute() error = %v, want ErrRouteEnergyUnavailable", err)
	}
	if routes := store.AutomationRoutes(); len(routes) != 0 {
		t.Fatalf("routes after energy failure = %+v, want none", routes)
	}
	if _, ok, readErr := store.CommittedAutomationRouteDurableRecord("route-1"); readErr != nil || ok {
		t.Fatalf("CommittedAutomationRouteDurableRecord(route-1) ok=%v err=%v, want false nil", ok, readErr)
	}
	assertRouteEnergyReserved(t, store, "planet-1", 35)
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

func TestCreateRouteValidatesPolicyMapIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(RouteCreatePolicy) RouteCreatePolicy
	}{
		{
			name: "missing source map",
			edit: func(policy RouteCreatePolicy) RouteCreatePolicy {
				policy.SourceMapID = ""
				return policy
			},
		},
		{
			name: "missing destination map",
			edit: func(policy RouteCreatePolicy) RouteCreatePolicy {
				policy.DestinationMapID = ""
				return policy
			},
		},
		{
			name: "invalid source map",
			edit: func(policy RouteCreatePolicy) RouteCreatePolicy {
				policy.SourceMapID = " map_1_1"
				return policy
			},
		},
		{
			name: "invalid destination map",
			edit: func(policy RouteCreatePolicy) RouteCreatePolicy {
				policy.DestinationMapID = "map:1:1"
				return policy
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := NewInMemoryStore()
			provider := &fakeRoutePolicyProvider{policy: tc.edit(validRoutePolicy())}
			service := newTestRouteService(t, store, provider, testRouteNow())

			_, err := service.CreateRoute(validCreateRouteInput())
			if !errors.Is(err, ErrInvalidRouteMapID) {
				t.Fatalf("CreateRoute() error = %v, want ErrInvalidRouteMapID", err)
			}
		})
	}
}

func TestAutomationRouteValidateRequiresMapIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(AutomationRoute) AutomationRoute
	}{
		{
			name: "missing source map",
			edit: func(route AutomationRoute) AutomationRoute {
				route.SourceMapID = ""
				return route
			},
		},
		{
			name: "missing destination map",
			edit: func(route AutomationRoute) AutomationRoute {
				route.DestinationMapID = ""
				return route
			},
		},
		{
			name: "invalid source map",
			edit: func(route AutomationRoute) AutomationRoute {
				route.SourceMapID = "map_1_1\n"
				return route
			},
		},
		{
			name: "invalid destination map",
			edit: func(route AutomationRoute) AutomationRoute {
				route.DestinationMapID = " map_1_1"
				return route
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.edit(validSettlementRoute(testRouteNow())).Validate()
			if !errors.Is(err, ErrInvalidRouteMapID) {
				t.Fatalf("Validate() error = %v, want ErrInvalidRouteMapID", err)
			}
		})
	}
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
	assertRouteMapIdentity(t, result.Route, provider.policy.SourceMapID, provider.policy.DestinationMapID)

	result.Route.Enabled = false
	result.Route.SourceMapID = "map_mutated"
	result.Route.DestinationMapID = "map_mutated"
	stored, ok, err := store.AutomationRoute(input.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute() ok = %v err = %v, want true nil", ok, err)
	}
	if !stored.Enabled {
		t.Fatal("mutating result changed stored route Enabled to false")
	}
	assertRouteMapIdentity(t, stored, provider.policy.SourceMapID, provider.policy.DestinationMapID)
	stored.AmountPerHour = 999
	stored.SourceMapID = "map_mutated_again"
	storedAgain, ok, err := store.AutomationRoute(input.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(second) ok = %v err = %v, want true nil", ok, err)
	}
	if storedAgain.AmountPerHour != input.AmountPerHour {
		t.Fatalf("stored AmountPerHour = %d, want %d", storedAgain.AmountPerHour, input.AmountPerHour)
	}
	assertRouteMapIdentity(t, storedAgain, provider.policy.SourceMapID, provider.policy.DestinationMapID)
	assertRouteDurableRecord(t, store, input.RouteID, foundation.IdempotencyKey("route_create:player-1:route-1"), 1, storedAgain)
	assertRouteDurableRecordSourceEnergy(t, store, input.RouteID, input.SourcePlanetID, storedAgain.EnergyCostPerHour)
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

func TestCreateRouteRechecksCapacityInsideTransaction(t *testing.T) {
	now := testRouteNow()
	store := NewInMemoryStore()
	ensureRouteProductionStateForTest(t, store, "planet-1", 100, now)
	existing := validSettlementRoute(now)
	existing.RouteID = "route-existing"
	if _, err := store.insertAutomationRoute(existing); err != nil {
		t.Fatalf("insertAutomationRoute(existing) error = %v, want nil", err)
	}

	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	provider.policy.CurrentRouteCount = 0
	provider.policy.MaxRouteCount = 1
	service := newTestRouteService(t, store, provider, now.Add(time.Minute))
	input := validCreateRouteInput()
	input.RouteID = "route-over-cap"

	_, err := service.CreateRoute(input)
	if !errors.Is(err, ErrRouteCapacityExceeded) {
		t.Fatalf("CreateRoute(over cap) error = %v, want ErrRouteCapacityExceeded", err)
	}
	if _, ok, err := store.AutomationRoute(input.RouteID); err != nil || ok {
		t.Fatalf("AutomationRoute(over cap) ok=%v err=%v, want missing nil", ok, err)
	}
	if _, ok, err := store.CommittedAutomationRouteDurableRecord(input.RouteID); err != nil || ok {
		t.Fatalf("CommittedAutomationRouteDurableRecord(over cap) ok=%v err=%v, want missing nil", ok, err)
	}
	assertRouteEnergyReserved(t, store, existing.SourcePlanetID, existing.EnergyCostPerHour)
}
