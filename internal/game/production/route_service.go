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
