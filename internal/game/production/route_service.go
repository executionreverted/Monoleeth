package production

import (
	"fmt"

	"gameproject/internal/game/foundation"
)

// AutomationRouteServiceConfig wires route operations to explicit storage,
// clock, policy, and loss-roll dependencies.
type AutomationRouteServiceConfig struct {
	Store      *InMemoryStore
	Clock      foundation.Clock
	Policy     RouteCreatePolicyProvider
	LossRoller RouteLossRoller
}

// AutomationRouteService owns the Phase 09 route creation and settlement boundary.
type AutomationRouteService struct {
	store      *InMemoryStore
	clock      foundation.Clock
	policy     RouteCreatePolicyProvider
	lossRoller RouteLossRoller
}

// NewAutomationRouteService returns a route service backed by in-memory route
// storage and server-owned policy facts.
func NewAutomationRouteService(config AutomationRouteServiceConfig) (*AutomationRouteService, error) {
	if config.Store == nil {
		config.Store = NewInMemoryStore()
	}
	if config.Clock == nil {
		config.Clock = foundation.RealClock{}
	}
	if config.Policy == nil {
		return nil, fmt.Errorf("policy: %w", ErrInvalidRouteCreateConfig)
	}
	if config.LossRoller == nil {
		config.LossRoller = defaultRouteLossRoller{}
	}
	return &AutomationRouteService{
		store:      config.Store,
		clock:      config.Clock,
		policy:     config.Policy,
		lossRoller: config.LossRoller,
	}, nil
}

// CreateRoute validates player intent against server-owned policy facts,
// initializes timestamps from the server clock, and stores the route once.
func (service *AutomationRouteService) CreateRoute(input CreateRouteInput) (CreateRouteResult, error) {
	if service == nil || service.store == nil || service.clock == nil || service.policy == nil {
		return CreateRouteResult{}, ErrInvalidRouteCreateConfig
	}
	if err := input.Validate(); err != nil {
		return CreateRouteResult{}, err
	}
	policyInput := RouteCreatePolicyInput{
		OwnerPlayerID:  input.OwnerPlayerID,
		SourcePlanetID: input.SourcePlanetID,
		Destination:    input.Destination,
		ResourceItemID: input.ResourceItemID,
		AmountPerHour:  input.AmountPerHour,
	}
	if err := policyInput.Validate(); err != nil {
		return CreateRouteResult{}, err
	}
	policy, err := service.policy.RouteCreatePolicy(policyInput)
	if err != nil {
		return CreateRouteResult{}, err
	}
	route, err := newAutomationRoute(input, policy, service.clock.Now())
	if err != nil {
		return CreateRouteResult{}, err
	}
	stored, err := service.store.insertAutomationRoute(route)
	if err != nil {
		return CreateRouteResult{}, err
	}
	return CreateRouteResult{
		Route:   cloneAutomationRoute(stored),
		Created: true,
	}, nil
}

// SettleRoute settles one route using server-owned time and deterministic
// service-configured loss rolls.
func (service *AutomationRouteService) SettleRoute(routeID foundation.RouteID) (RouteSettlementResult, error) {
	if service == nil || service.store == nil || service.clock == nil || service.lossRoller == nil {
		return RouteSettlementResult{}, ErrInvalidRouteSettlementConfig
	}
	return service.store.SettleRoute(routeID, service.clock.Now(), service.lossRoller)
}

// SettleRouteForOwner settles a route only after matching the server-resolved
// player id against the durable route owner.
func (service *AutomationRouteService) SettleRouteForOwner(
	ownerPlayerID foundation.PlayerID,
	routeID foundation.RouteID,
) (RouteSettlementResult, error) {
	if service == nil || service.store == nil || service.clock == nil || service.lossRoller == nil {
		return RouteSettlementResult{}, ErrInvalidRouteSettlementConfig
	}
	return service.store.SettleRouteForOwner(ownerPlayerID, routeID, service.clock.Now(), service.lossRoller)
}

// DisableRoute settles the currently enabled route period, then disables the
// route using server-owned time and loss rolls.
func (service *AutomationRouteService) DisableRoute(routeID foundation.RouteID) (RouteControlResult, error) {
	if service == nil || service.store == nil || service.clock == nil || service.lossRoller == nil {
		return RouteControlResult{}, ErrInvalidRouteSettlementConfig
	}
	return service.store.DisableRoute(routeID, service.clock.Now(), service.lossRoller)
}

// DisableRouteForOwner disables a route only after matching the server-resolved
// player id against the durable route owner.
func (service *AutomationRouteService) DisableRouteForOwner(
	ownerPlayerID foundation.PlayerID,
	routeID foundation.RouteID,
) (RouteControlResult, error) {
	if service == nil || service.store == nil || service.clock == nil || service.lossRoller == nil {
		return RouteControlResult{}, ErrInvalidRouteSettlementConfig
	}
	return service.store.DisableRouteForOwner(ownerPlayerID, routeID, service.clock.Now(), service.lossRoller)
}

// EnableRoute re-enables a disabled route and starts a fresh settlement period
// at the server timestamp.
func (service *AutomationRouteService) EnableRoute(routeID foundation.RouteID) (RouteControlResult, error) {
	if service == nil || service.store == nil || service.clock == nil {
		return RouteControlResult{}, ErrInvalidRouteSettlementConfig
	}
	return service.store.EnableRoute(routeID, service.clock.Now())
}

// EnableRouteForOwner enables a route only after matching the server-resolved
// player id against the durable route owner.
func (service *AutomationRouteService) EnableRouteForOwner(
	ownerPlayerID foundation.PlayerID,
	routeID foundation.RouteID,
) (RouteControlResult, error) {
	if service == nil || service.store == nil || service.clock == nil {
		return RouteControlResult{}, ErrInvalidRouteSettlementConfig
	}
	return service.store.EnableRouteForOwner(ownerPlayerID, routeID, service.clock.Now())
}

// UpdateRoute settles old route terms first, then replaces mutable terms using
// server-owned policy facts for distance, energy cost, and risk.
func (service *AutomationRouteService) UpdateRoute(input UpdateRouteInput) (UpdateRouteResult, error) {
	if service == nil || service.store == nil || service.clock == nil || service.policy == nil || service.lossRoller == nil {
		return UpdateRouteResult{}, ErrInvalidRouteCreateConfig
	}
	if err := input.Validate(); err != nil {
		return UpdateRouteResult{}, err
	}

	route, ok, err := service.store.AutomationRoute(input.RouteID)
	if err != nil {
		return UpdateRouteResult{}, err
	}
	if !ok {
		return UpdateRouteResult{}, fmt.Errorf("route %q: %w", input.RouteID, ErrRouteNotFound)
	}
	if route.OwnerPlayerID != input.OwnerPlayerID {
		return UpdateRouteResult{}, fmt.Errorf("route %q owner %q: %w", input.RouteID, input.OwnerPlayerID, ErrRouteOwnerMismatch)
	}

	policyInput := input.policyInput(route.SourcePlanetID)
	if err := policyInput.Validate(); err != nil {
		return UpdateRouteResult{}, err
	}
	policy, err := service.policy.RouteCreatePolicy(policyInput)
	if err != nil {
		return UpdateRouteResult{}, err
	}
	if _, err := policy.CalculateRisk(); err != nil {
		return UpdateRouteResult{}, err
	}

	return service.store.UpdateRoute(input, policy, service.clock.Now(), service.lossRoller)
}

// UpdateRouteForOwner updates a route using only the server-resolved player id
// for ownership. Any client-supplied owner field on input is ignored.
func (service *AutomationRouteService) UpdateRouteForOwner(
	ownerPlayerID foundation.PlayerID,
	input UpdateRouteInput,
) (UpdateRouteResult, error) {
	input.OwnerPlayerID = ownerPlayerID
	return service.UpdateRoute(input)
}
