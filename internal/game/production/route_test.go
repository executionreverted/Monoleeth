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

func testRouteNow() time.Time {
	return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
}

var _ RouteCreatePolicyProvider = (*fakeRoutePolicyProvider)(nil)
var _ foundation.Clock = fixedRouteClock{}
