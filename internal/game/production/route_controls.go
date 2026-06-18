package production

import (
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
)

// EnableRoute enables a disabled route and resets its settlement clock so
// disabled elapsed time cannot accrue a free transfer.
func (store *InMemoryStore) EnableRoute(routeID foundation.RouteID, now time.Time) (RouteControlResult, error) {
	if err := routeID.Validate(); err != nil {
		return RouteControlResult{}, err
	}
	if now.IsZero() {
		return RouteControlResult{}, fmt.Errorf("now: %w", ErrZeroProductionTimestamp)
	}
	now = now.UTC()

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	route, ok := store.routes[routeID]
	if !ok {
		return RouteControlResult{}, fmt.Errorf("route %q: %w", routeID, ErrRouteNotFound)
	}
	if err := route.Validate(); err != nil {
		return RouteControlResult{}, err
	}
	if route.Enabled {
		return RouteControlResult{Route: cloneAutomationRoute(route)}, nil
	}

	route.Enabled = true
	route.LastCalculatedAt = maxRouteTimestamp(route.LastCalculatedAt, now)
	route.UpdatedAt = now
	if err := route.Validate(); err != nil {
		return RouteControlResult{}, err
	}
	store.routes[routeID] = cloneAutomationRoute(route)
	return RouteControlResult{Route: cloneAutomationRoute(route), Changed: true}, nil
}

// DisableRoute settles an enabled route through the current server timestamp,
// then disables it without advancing already-disabled routes.
func (store *InMemoryStore) DisableRoute(
	routeID foundation.RouteID,
	now time.Time,
	lossRoller RouteLossRoller,
) (RouteControlResult, error) {
	if err := routeID.Validate(); err != nil {
		return RouteControlResult{}, err
	}
	if now.IsZero() {
		return RouteControlResult{}, fmt.Errorf("now: %w", ErrZeroProductionTimestamp)
	}
	if lossRoller == nil {
		lossRoller = defaultRouteLossRoller{}
	}
	now = now.UTC()

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	route, ok := store.routes[routeID]
	if !ok {
		return RouteControlResult{}, fmt.Errorf("route %q: %w", routeID, ErrRouteNotFound)
	}
	if err := route.Validate(); err != nil {
		return RouteControlResult{}, err
	}
	if !route.Enabled {
		return RouteControlResult{Route: cloneAutomationRoute(route)}, nil
	}

	disabledRoute := cloneAutomationRoute(route)
	disabledRoute.Enabled = false
	disabledRoute.UpdatedAt = now
	if err := disabledRoute.Validate(); err != nil {
		return RouteControlResult{}, err
	}

	settlement, err := store.settleRouteLocked(routeID, now, lossRoller)
	if err != nil {
		return RouteControlResult{}, err
	}

	route = store.routes[routeID]
	route.Enabled = false
	route.UpdatedAt = now
	if err := route.Validate(); err != nil {
		return RouteControlResult{}, err
	}
	store.routes[routeID] = cloneAutomationRoute(route)
	return RouteControlResult{
		Route:      cloneAutomationRoute(route),
		Settlement: settlement,
		Changed:    true,
	}, nil
}

// UpdateRoute settles the old route terms before atomically replacing mutable
// route terms with server-policy-derived energy and risk.
func (store *InMemoryStore) UpdateRoute(
	input UpdateRouteInput,
	policy RouteCreatePolicy,
	now time.Time,
	lossRoller RouteLossRoller,
) (UpdateRouteResult, error) {
	if err := input.Validate(); err != nil {
		return UpdateRouteResult{}, err
	}
	if now.IsZero() {
		return UpdateRouteResult{}, fmt.Errorf("now: %w", ErrZeroProductionTimestamp)
	}
	if lossRoller == nil {
		lossRoller = defaultRouteLossRoller{}
	}
	now = now.UTC()

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	route, ok := store.routes[input.RouteID]
	if !ok {
		return UpdateRouteResult{}, fmt.Errorf("route %q: %w", input.RouteID, ErrRouteNotFound)
	}
	if err := route.Validate(); err != nil {
		return UpdateRouteResult{}, err
	}
	if route.OwnerPlayerID != input.OwnerPlayerID {
		return UpdateRouteResult{}, fmt.Errorf("route %q owner %q: %w", input.RouteID, input.OwnerPlayerID, ErrRouteOwnerMismatch)
	}

	risk, err := policy.CalculateRisk()
	if err != nil {
		return UpdateRouteResult{}, err
	}

	updatedRoute := cloneAutomationRoute(route)
	updatedRoute.Destination = input.Destination
	updatedRoute.ResourceItemID = input.ResourceItemID
	updatedRoute.AmountPerHour = input.AmountPerHour
	updatedRoute.EnergyCostPerHour = policy.EnergyCostPerHour
	updatedRoute.Risk = risk
	updatedRoute.LastCalculatedAt = maxRouteTimestamp(route.LastCalculatedAt, now)
	updatedRoute.UpdatedAt = now
	if err := updatedRoute.Validate(); err != nil {
		return UpdateRouteResult{}, err
	}

	settlement, err := store.settleRouteLocked(input.RouteID, now, lossRoller)
	if err != nil {
		return UpdateRouteResult{}, err
	}

	settledRoute := store.routes[input.RouteID]
	updatedRoute.OwnerPlayerID = settledRoute.OwnerPlayerID
	updatedRoute.SourcePlanetID = settledRoute.SourcePlanetID
	updatedRoute.CreatedAt = settledRoute.CreatedAt
	updatedRoute.Enabled = route.Enabled
	updatedRoute.LastCalculatedAt = maxRouteTimestamp(settledRoute.LastCalculatedAt, now)
	updatedRoute.UpdatedAt = now
	if err := updatedRoute.Validate(); err != nil {
		return UpdateRouteResult{}, err
	}
	store.routes[input.RouteID] = cloneAutomationRoute(updatedRoute)
	return UpdateRouteResult{
		Route:      cloneAutomationRoute(updatedRoute),
		Settlement: settlement,
		Updated:    true,
	}, nil
}

func maxRouteTimestamp(left, right time.Time) time.Time {
	left = left.UTC()
	right = right.UTC()
	if left.After(right) {
		return left
	}
	return right
}
