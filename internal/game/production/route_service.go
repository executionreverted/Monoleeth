package production

import (
	"fmt"

	"gameproject/internal/game/foundation"
)

// AutomationRouteServiceConfig wires route operations to explicit storage,
// clock, policy, and loss-roll dependencies.
type AutomationRouteServiceConfig struct {
	Store                 *InMemoryStore
	CreateTransaction     RouteCreateTransactionStore
	SettlementTransaction RouteSettlementTransactionStore
	Clock                 foundation.Clock
	Policy                RouteCreatePolicyProvider
	LossRoller            RouteLossRoller
}

// AutomationRouteService owns the Phase 09 route creation and settlement boundary.
type AutomationRouteService struct {
	store                 *InMemoryStore
	createTransaction     RouteCreateTransactionStore
	settlementTransaction RouteSettlementTransactionStore
	clock                 foundation.Clock
	policy                RouteCreatePolicyProvider
	lossRoller            RouteLossRoller
}

// NewAutomationRouteService returns a route service backed by in-memory route
// storage and server-owned policy facts.
func NewAutomationRouteService(config AutomationRouteServiceConfig) (*AutomationRouteService, error) {
	if config.Store == nil {
		config.Store = NewInMemoryStore()
	}
	if config.CreateTransaction == nil {
		config.CreateTransaction = config.Store
	}
	if config.SettlementTransaction == nil {
		config.SettlementTransaction = config.Store
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
		store:                 config.Store,
		createTransaction:     config.CreateTransaction,
		settlementTransaction: config.SettlementTransaction,
		clock:                 config.Clock,
		policy:                config.Policy,
		lossRoller:            config.LossRoller,
	}, nil
}

// CreateRoute validates player intent against server-owned policy facts,
// initializes timestamps from the server clock, and stores the route once.
func (service *AutomationRouteService) CreateRoute(input CreateRouteInput) (CreateRouteResult, error) {
	if service == nil || service.createTransaction == nil || service.clock == nil || service.policy == nil {
		return CreateRouteResult{}, ErrInvalidRouteCreateConfig
	}
	if err := input.Validate(); err != nil {
		return CreateRouteResult{}, err
	}
	if replay, ok, err := service.committedRouteCreateReplay(input); err != nil || ok {
		if err != nil {
			return CreateRouteResult{}, err
		}
		return CreateRouteResult{Route: replay, Created: false}, nil
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
	stored, err := service.createTransaction.ApplyRouteCreateTransaction(RouteCreateTransactionInput{
		Route:         route,
		MaxRouteCount: policy.MaxRouteCount,
	})
	if err != nil {
		return CreateRouteResult{}, err
	}
	return CreateRouteResult{
		Route:   cloneAutomationRoute(stored.Route),
		Created: stored.Created,
	}, nil
}

func (service *AutomationRouteService) committedRouteCreateReplay(input CreateRouteInput) (AutomationRoute, bool, error) {
	referenceKey, err := foundation.RouteCreateIdempotencyKey(input.OwnerPlayerID, input.RouteID)
	if err != nil {
		return AutomationRoute{}, false, err
	}
	reader := service.routeCreateReplayReader()
	if reader == nil {
		return AutomationRoute{}, false, nil
	}
	record, ok, err := reader.CommittedAutomationRouteDurableRecordByReference(referenceKey)
	if err != nil || !ok {
		return AutomationRoute{}, ok, err
	}
	if !routeCreateInputMatchesCommittedRoute(input, record.Route) {
		return AutomationRoute{}, false, nil
	}
	return cloneAutomationRoute(record.Route), true, nil
}

func (service *AutomationRouteService) routeCreateReplayReader() AutomationRouteDurableReader {
	if reader, ok := service.createTransaction.(AutomationRouteDurableReader); ok {
		return reader
	}
	if service.store != nil {
		return service.store
	}
	return nil
}

func routeCreateInputMatchesCommittedRoute(input CreateRouteInput, route AutomationRoute) bool {
	return input.RouteID == route.RouteID &&
		input.OwnerPlayerID == route.OwnerPlayerID &&
		input.SourcePlanetID == route.SourcePlanetID &&
		input.Destination == route.Destination &&
		input.ResourceItemID == route.ResourceItemID &&
		input.AmountPerHour == route.AmountPerHour
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
	if service == nil || service.settlementTransaction == nil || service.clock == nil || service.lossRoller == nil {
		return RouteSettlementResult{}, ErrInvalidRouteSettlementConfig
	}
	result, err := service.settlementTransaction.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: ownerPlayerID,
		RouteID:       routeID,
		SettledAt:     service.clock.Now(),
		LossRoller:    service.lossRoller,
	})
	if err != nil {
		return RouteSettlementResult{}, err
	}
	return result.Settlement, nil
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
	return service.DisableRouteForOwnerWithRequest(ownerPlayerID, routeID, "")
}

func (service *AutomationRouteService) DisableRouteForOwnerWithRequest(
	ownerPlayerID foundation.PlayerID,
	routeID foundation.RouteID,
	requestID foundation.RequestID,
) (RouteControlResult, error) {
	if service == nil || service.store == nil || service.clock == nil || service.lossRoller == nil {
		return RouteControlResult{}, ErrInvalidRouteSettlementConfig
	}
	return service.store.DisableRouteForOwnerWithRequest(ownerPlayerID, routeID, service.clock.Now(), service.lossRoller, requestID)
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
	return service.EnableRouteForOwnerWithRequest(ownerPlayerID, routeID, "")
}

func (service *AutomationRouteService) EnableRouteForOwnerWithRequest(
	ownerPlayerID foundation.PlayerID,
	routeID foundation.RouteID,
	requestID foundation.RequestID,
) (RouteControlResult, error) {
	if service == nil || service.store == nil || service.clock == nil {
		return RouteControlResult{}, ErrInvalidRouteSettlementConfig
	}
	return service.store.EnableRouteForOwnerWithRequest(ownerPlayerID, routeID, service.clock.Now(), requestID)
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
	if replay, ok, err := service.committedRouteUpdateReplay(input); err != nil || ok {
		if err != nil {
			return UpdateRouteResult{}, err
		}
		return UpdateRouteResult{Route: replay}, nil
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

func (service *AutomationRouteService) committedRouteUpdateReplay(input UpdateRouteInput) (AutomationRoute, bool, error) {
	if input.RequestID.IsZero() {
		return AutomationRoute{}, false, nil
	}
	if service.store == nil {
		return AutomationRoute{}, false, nil
	}
	referenceKey, err := foundation.RouteUpdateIdempotencyKey(input.OwnerPlayerID, input.RouteID, input.RequestID)
	if err != nil {
		return AutomationRoute{}, false, err
	}
	record, ok, err := service.store.CommittedAutomationRouteDurableRecordByReference(referenceKey)
	if err != nil || !ok {
		return AutomationRoute{}, ok, err
	}
	if !routeUpdateReplayMatches(input, record.Route) {
		return AutomationRoute{}, false, ErrInvalidAutomationRouteDurableCommit
	}
	return cloneAutomationRoute(record.Route), true, nil
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

// UpdateRouteForOwnerWithRequest updates a route and records the server
// request id as part of the durable route mutation idempotency key.
func (service *AutomationRouteService) UpdateRouteForOwnerWithRequest(
	ownerPlayerID foundation.PlayerID,
	input UpdateRouteInput,
	requestID foundation.RequestID,
) (UpdateRouteResult, error) {
	input.OwnerPlayerID = ownerPlayerID
	input.RequestID = requestID
	return service.UpdateRoute(input)
}
