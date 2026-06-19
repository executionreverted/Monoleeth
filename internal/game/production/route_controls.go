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

	return store.enableRouteLocked(routeID, now)
}

// EnableRouteForOwner enables a disabled route only after matching the
// server-resolved player id against the durable route owner.
func (store *InMemoryStore) EnableRouteForOwner(
	ownerPlayerID foundation.PlayerID,
	routeID foundation.RouteID,
	now time.Time,
) (RouteControlResult, error) {
	if err := ownerPlayerID.Validate(); err != nil {
		return RouteControlResult{}, err
	}
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

	if err := store.requireRouteOwnerLocked(ownerPlayerID, routeID); err != nil {
		return RouteControlResult{}, err
	}
	return store.enableRouteLocked(routeID, now)
}

func (store *InMemoryStore) enableRouteLocked(routeID foundation.RouteID, now time.Time) (RouteControlResult, error) {
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

	return store.disableRouteLocked(routeID, now, lossRoller)
}

// DisableRouteForOwner settles and disables an enabled route only after
// matching the server-resolved player id against the durable route owner.
func (store *InMemoryStore) DisableRouteForOwner(
	ownerPlayerID foundation.PlayerID,
	routeID foundation.RouteID,
	now time.Time,
	lossRoller RouteLossRoller,
) (RouteControlResult, error) {
	if err := ownerPlayerID.Validate(); err != nil {
		return RouteControlResult{}, err
	}
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

	if err := store.requireRouteOwnerLocked(ownerPlayerID, routeID); err != nil {
		return RouteControlResult{}, err
	}
	return store.disableRouteLocked(routeID, now, lossRoller)
}

func (store *InMemoryStore) disableRouteLocked(
	routeID foundation.RouteID,
	now time.Time,
	lossRoller RouteLossRoller,
) (RouteControlResult, error) {
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

// SettleRouteForOwner settles one route only after matching the server-resolved
// player id against the durable route owner.
func (store *InMemoryStore) SettleRouteForOwner(
	ownerPlayerID foundation.PlayerID,
	routeID foundation.RouteID,
	now time.Time,
	lossRoller RouteLossRoller,
) (RouteSettlementResult, error) {
	if err := ownerPlayerID.Validate(); err != nil {
		return RouteSettlementResult{}, err
	}
	if err := routeID.Validate(); err != nil {
		return RouteSettlementResult{}, err
	}
	if now.IsZero() {
		return RouteSettlementResult{}, fmt.Errorf("now: %w", ErrZeroProductionTimestamp)
	}
	if lossRoller == nil {
		lossRoller = defaultRouteLossRoller{}
	}
	now = now.UTC()

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if err := store.requireRouteOwnerLocked(ownerPlayerID, routeID); err != nil {
		return RouteSettlementResult{}, err
	}
	return store.settleRouteLocked(routeID, now, lossRoller)
}

func (store *InMemoryStore) requireRouteOwnerLocked(
	ownerPlayerID foundation.PlayerID,
	routeID foundation.RouteID,
) error {
	route, ok := store.routes[routeID]
	if !ok {
		return fmt.Errorf("route %q: %w", routeID, ErrRouteNotFound)
	}
	if err := route.Validate(); err != nil {
		return err
	}
	if route.OwnerPlayerID != ownerPlayerID {
		return fmt.Errorf("route %q owner %q: %w", routeID, ownerPlayerID, ErrRouteOwnerMismatch)
	}
	return nil
}

func maxRouteTimestamp(left, right time.Time) time.Time {
	left = left.UTC()
	right = right.UTC()
	if left.After(right) {
		return left
	}
	return right
}
