package production

import (
	"fmt"

	"gameproject/internal/game/foundation"
)

// RouteCreateTransactionStore is the durable adapter boundary for route
// creation. DB implementations should enforce owner route quota, route-id
// uniqueness, source energy reservation, and durable route row commit in one
// transaction.
type RouteCreateTransactionStore interface {
	ApplyRouteCreateTransaction(RouteCreateTransactionInput) (RouteCreateTransactionResult, error)
}

// RouteCreateTransactionInput carries the server-built route plus the
// server-owned route slot cap that must be rechecked at insert time.
type RouteCreateTransactionInput struct {
	Route         AutomationRoute
	MaxRouteCount int
}

// RouteCreateTransactionResult reports the route and durable row accepted by
// the create transaction.
type RouteCreateTransactionResult struct {
	Route        AutomationRoute
	ReferenceKey foundation.IdempotencyKey
	RouteRow     *AutomationRouteDurableRecord
	Created      bool
}

// ApplyRouteCreateTransaction inserts one route under the store lock while
// rechecking quota against the current authoritative route rows.
func (store *InMemoryStore) ApplyRouteCreateTransaction(
	input RouteCreateTransactionInput,
) (RouteCreateTransactionResult, error) {
	if store == nil {
		return RouteCreateTransactionResult{}, ErrInvalidRouteCreateConfig
	}
	if err := input.Validate(); err != nil {
		return RouteCreateTransactionResult{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	referenceKey, err := foundation.RouteCreateIdempotencyKey(input.Route.OwnerPlayerID, input.Route.RouteID)
	if err != nil {
		return RouteCreateTransactionResult{}, err
	}
	if _, ok := store.routes[input.Route.RouteID]; ok {
		record, replay, err := store.committedAutomationRouteDurableRecordByReferenceLocked(referenceKey)
		if err != nil {
			return RouteCreateTransactionResult{}, err
		}
		if replay && routeCreateReplayMatches(record.Route, input.Route) {
			clonedRecord := cloneAutomationRouteDurableRecord(record)
			return RouteCreateTransactionResult{
				Route:        cloneAutomationRoute(record.Route),
				ReferenceKey: referenceKey,
				RouteRow:     &clonedRecord,
				Created:      false,
			}, nil
		}
		return RouteCreateTransactionResult{}, fmt.Errorf("route %q: %w", input.Route.RouteID, ErrDuplicateRoute)
	}
	if input.MaxRouteCount > 0 && store.countOwnerRoutesLocked(input.Route.OwnerPlayerID) >= input.MaxRouteCount {
		return RouteCreateTransactionResult{}, ErrRouteCapacityExceeded
	}
	updatedSourceState, updateSourceState, err := store.prepareRouteEnergyReservationLocked(
		input.Route.SourcePlanetID,
		0,
		routeReservedEnergyCost(input.Route),
		input.Route.CreatedAt,
	)
	if err != nil {
		return RouteCreateTransactionResult{}, err
	}
	commit, err := store.applyAutomationRouteDurableCommitPlanLocked(AutomationRouteDurableCommitPlan{
		Route:                 input.Route,
		SourceProductionState: optionalRouteSourceProductionState(updatedSourceState, updateSourceState),
		ReferenceKey:          referenceKey,
		ExpectedRevision:      0,
		RecordedAt:            input.Route.CreatedAt,
	})
	if err != nil {
		return RouteCreateTransactionResult{}, err
	}
	store.routes[input.Route.RouteID] = cloneAutomationRoute(input.Route)
	if updateSourceState {
		store.states[input.Route.SourcePlanetID] = cloneProductionState(updatedSourceState)
	}
	record := cloneAutomationRouteDurableRecord(commit.Record)
	return RouteCreateTransactionResult{
		Route:        cloneAutomationRoute(input.Route),
		ReferenceKey: referenceKey,
		RouteRow:     &record,
		Created:      true,
	}, nil
}

func (input RouteCreateTransactionInput) Validate() error {
	if err := input.Route.Validate(); err != nil {
		return err
	}
	if input.MaxRouteCount < 0 {
		return ErrInvalidRouteCreateConfig
	}
	return nil
}

func routeCreateReplayMatches(committed AutomationRoute, incoming AutomationRoute) bool {
	return committed.RouteID == incoming.RouteID &&
		committed.OwnerPlayerID == incoming.OwnerPlayerID &&
		committed.SourcePlanetID == incoming.SourcePlanetID &&
		committed.SourceMapID == incoming.SourceMapID &&
		committed.Destination == incoming.Destination &&
		committed.DestinationMapID == incoming.DestinationMapID &&
		committed.ResourceItemID == incoming.ResourceItemID &&
		committed.AmountPerHour == incoming.AmountPerHour &&
		committed.EnergyCostPerHour == incoming.EnergyCostPerHour &&
		committed.Risk == incoming.Risk &&
		committed.Enabled == incoming.Enabled
}

func (store *InMemoryStore) countOwnerRoutesLocked(playerID foundation.PlayerID) int {
	count := 0
	for _, route := range store.routes {
		if route.OwnerPlayerID == playerID {
			count++
		}
	}
	return count
}
