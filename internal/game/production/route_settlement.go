package production

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"gameproject/internal/game/foundation"
)

const DefaultMaxRouteOfflineSettlementDuration = 72 * time.Hour

// RouteLossRoller supplies route-loss random rolls. Tests can inject a
// deterministic implementation to control both chance and percent rolls.
type RouteLossRoller interface {
	Float64() float64
}

type defaultRouteLossRoller struct{}

func (defaultRouteLossRoller) Float64() float64 {
	return rand.Float64()
}

type routeLossResult struct {
	lost        int64
	delivered   int64
	applied     bool
	lossPercent float64
}

// RouteSettlementResult summarizes one server-timed automation route settlement.
type RouteSettlementResult struct {
	RouteID                           foundation.RouteID `json:"route_id"`
	SettledAt                         time.Time          `json:"settled_at"`
	MaxRouteOfflineSettlementDuration time.Duration      `json:"max_route_offline_settlement_duration"`
	ElapsedRequested                  time.Duration      `json:"elapsed_requested"`
	ElapsedApplied                    time.Duration      `json:"elapsed_applied"`
	BeforeRoute                       AutomationRoute    `json:"before_route"`
	AfterRoute                        AutomationRoute    `json:"after_route"`
	WantedAmount                      int64              `json:"wanted_amount"`
	TakenAmount                       int64              `json:"taken_amount"`
	LostAmount                        int64              `json:"lost_amount"`
	DeliveredAmount                   int64              `json:"delivered_amount"`
	AddedAmount                       int64              `json:"added_amount"`
	LossPercent                       float64            `json:"loss_percent"`
	SourceEmpty                       bool               `json:"source_empty"`
	DestinationFull                   bool               `json:"destination_full"`
	LossApplied                       bool               `json:"loss_applied"`
	NoOp                              bool               `json:"no_op"`
}

// SettleRoute applies one atomic virtual transfer for an automation route using
// the supplied server timestamp.
func (store *InMemoryStore) SettleRoute(
	routeID foundation.RouteID,
	now time.Time,
	lossRoller RouteLossRoller,
) (RouteSettlementResult, error) {
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

	return store.settleRouteLocked(routeID, now, lossRoller)
}

func (store *InMemoryStore) settleRouteLocked(
	routeID foundation.RouteID,
	now time.Time,
	lossRoller RouteLossRoller,
) (RouteSettlementResult, error) {
	route, ok := store.routes[routeID]
	if !ok {
		return RouteSettlementResult{}, fmt.Errorf("route %q: %w", routeID, ErrRouteNotFound)
	}
	if err := route.Validate(); err != nil {
		return RouteSettlementResult{}, err
	}

	result := newRouteSettlementResult(route, now)
	result.ElapsedRequested = now.Sub(route.LastCalculatedAt)
	if result.ElapsedRequested <= 0 {
		result.NoOp = true
		return result, nil
	}
	if !route.Enabled {
		result.NoOp = true
		return result, nil
	}
	if route.Destination.Type != RouteDestinationTypePlanet {
		return RouteSettlementResult{}, fmt.Errorf("route %q destination %q: %w", routeID, route.Destination.Type, ErrUnsupportedRouteDestination)
	}

	result.ElapsedApplied = minDuration(result.ElapsedRequested, DefaultMaxRouteOfflineSettlementDuration)
	result.WantedAmount = wholeUnitsForElapsed(route.AmountPerHour, result.ElapsedApplied)
	if result.WantedAmount < 1 {
		route.LastCalculatedAt = now
		route.UpdatedAt = now
		if err := route.Validate(); err != nil {
			return RouteSettlementResult{}, err
		}
		store.routes[routeID] = cloneAutomationRoute(route)
		result.AfterRoute = cloneAutomationRoute(route)
		if err := store.appendRouteSettlementEventsLocked(result); err != nil {
			return RouteSettlementResult{}, err
		}
		return result, nil
	}

	sourcePlanetID := route.SourcePlanetID
	destinationPlanetID := foundation.PlanetID(route.Destination.ID)
	if err := destinationPlanetID.Validate(); err != nil {
		return RouteSettlementResult{}, fmt.Errorf("route %q destination planet %q: %w", routeID, route.Destination.ID, err)
	}

	source, ok := store.storage[sourcePlanetID]
	if !ok {
		return RouteSettlementResult{}, fmt.Errorf("route %q source planet %q: %w", routeID, sourcePlanetID, ErrRouteSourceStorageMissing)
	}
	if err := source.Validate(); err != nil {
		return RouteSettlementResult{}, err
	}
	source = clonePlanetStorage(source)

	if sourcePlanetID == destinationPlanetID {
		if err := settleRouteWithinSinglePlanetStorage(&source, route, now, lossRoller, &result); err != nil {
			return RouteSettlementResult{}, err
		}
		store.storage[sourcePlanetID] = clonePlanetStorage(source)
	} else {
		destination, ok := store.storage[destinationPlanetID]
		if !ok {
			return RouteSettlementResult{}, fmt.Errorf("route %q destination planet %q: %w", routeID, destinationPlanetID, ErrRouteDestinationStorageMissing)
		}
		if err := destination.Validate(); err != nil {
			return RouteSettlementResult{}, err
		}
		destination = clonePlanetStorage(destination)
		if err := settleRouteBetweenPlanetStorage(&source, &destination, route, now, lossRoller, &result); err != nil {
			return RouteSettlementResult{}, err
		}
		store.storage[sourcePlanetID] = clonePlanetStorage(source)
		store.storage[destinationPlanetID] = clonePlanetStorage(destination)
	}

	route.LastCalculatedAt = now
	route.UpdatedAt = now
	if err := route.Validate(); err != nil {
		return RouteSettlementResult{}, err
	}
	store.routes[routeID] = cloneAutomationRoute(route)
	result.AfterRoute = cloneAutomationRoute(route)
	if err := store.appendRouteSettlementEventsLocked(result); err != nil {
		return RouteSettlementResult{}, err
	}
	return result, nil
}

func (store *InMemoryStore) appendRouteSettlementEventsLocked(result RouteSettlementResult) error {
	if result.NoOp {
		return nil
	}
	payload, err := NewRouteSettlementPayload(result)
	if err != nil {
		return err
	}
	if result.LossApplied {
		if err := store.appendProductionEventLocked(EventType(EventRouteTransferLost), payload, result.SettledAt); err != nil {
			return err
		}
	}
	if result.SourceEmpty {
		if err := store.appendProductionEventLocked(EventType(EventRouteSourceEmpty), payload, result.SettledAt); err != nil {
			return err
		}
	}
	if result.DestinationFull {
		if err := store.appendProductionEventLocked(EventType(EventRouteDestinationFull), payload, result.SettledAt); err != nil {
			return err
		}
	}
	return store.appendProductionEventLocked(EventType(EventRouteTransferSettled), payload, result.SettledAt)
}

func newRouteSettlementResult(route AutomationRoute, now time.Time) RouteSettlementResult {
	route = cloneAutomationRoute(route)
	return RouteSettlementResult{
		RouteID:                           route.RouteID,
		SettledAt:                         now,
		MaxRouteOfflineSettlementDuration: DefaultMaxRouteOfflineSettlementDuration,
		BeforeRoute:                       route,
		AfterRoute:                        route,
	}
}

func settleRouteBetweenPlanetStorage(
	source *PlanetStorage,
	destination *PlanetStorage,
	route AutomationRoute,
	now time.Time,
	lossRoller RouteLossRoller,
	result *RouteSettlementResult,
) error {
	taken, err := source.RemoveUpTo(route.ResourceItemID, result.WantedAmount, now)
	if err != nil {
		return err
	}
	result.TakenAmount = taken
	loss, err := rollRouteLoss(taken, route.Risk, lossRoller)
	if err != nil {
		return err
	}
	applyRouteLossResult(loss, result)
	if result.DeliveredAmount > 0 {
		added, err := destination.AddUpToCapacity(route.ResourceItemID, result.DeliveredAmount, now)
		if err != nil {
			return err
		}
		result.AddedAmount = added
	}
	source.UpdatedAt = now
	destination.UpdatedAt = now
	if err := source.Validate(); err != nil {
		return err
	}
	if err := destination.Validate(); err != nil {
		return err
	}
	result.SourceEmpty = source.QuantityOf(route.ResourceItemID) == 0
	result.DestinationFull = result.DeliveredAmount > result.AddedAmount
	return nil
}

func settleRouteWithinSinglePlanetStorage(
	storage *PlanetStorage,
	route AutomationRoute,
	now time.Time,
	lossRoller RouteLossRoller,
	result *RouteSettlementResult,
) error {
	taken, err := storage.RemoveUpTo(route.ResourceItemID, result.WantedAmount, now)
	if err != nil {
		return err
	}
	result.TakenAmount = taken
	loss, err := rollRouteLoss(taken, route.Risk, lossRoller)
	if err != nil {
		return err
	}
	applyRouteLossResult(loss, result)
	if result.DeliveredAmount > 0 {
		added, err := storage.AddUpToCapacity(route.ResourceItemID, result.DeliveredAmount, now)
		if err != nil {
			return err
		}
		result.AddedAmount = added
	}
	storage.UpdatedAt = now
	if err := storage.Validate(); err != nil {
		return err
	}
	result.SourceEmpty = storage.QuantityOf(route.ResourceItemID) == 0
	result.DestinationFull = result.DeliveredAmount > result.AddedAmount
	return nil
}

func applyRouteLossResult(loss routeLossResult, result *RouteSettlementResult) {
	result.LostAmount = loss.lost
	result.DeliveredAmount = loss.delivered
	result.LossApplied = loss.applied
	result.LossPercent = loss.lossPercent
}

func rollRouteLoss(amount int64, risk RouteRisk, lossRoller RouteLossRoller) (routeLossResult, error) {
	if amount <= 0 {
		return routeLossResult{}, nil
	}
	if err := risk.Validate(); err != nil {
		return routeLossResult{}, err
	}
	if lossRoller == nil {
		lossRoller = defaultRouteLossRoller{}
	}
	if risk.LossChance <= 0 {
		return routeLossResult{delivered: amount}, nil
	}
	chanceRoll := lossRoller.Float64()
	if err := validateRouteRoll("loss chance roll", chanceRoll); err != nil {
		return routeLossResult{}, err
	}
	if chanceRoll > risk.LossChance {
		return routeLossResult{delivered: amount}, nil
	}

	lossPercent := risk.MinLossPercent
	if risk.MaxLossPercent > risk.MinLossPercent {
		percentRoll := lossRoller.Float64()
		if err := validateRouteRoll("loss percent roll", percentRoll); err != nil {
			return routeLossResult{}, err
		}
		lossPercent += percentRoll * (risk.MaxLossPercent - risk.MinLossPercent)
	}
	if err := validateFiniteRouteFloat("loss percent", lossPercent, ErrInvalidRouteLossRoll); err != nil {
		return routeLossResult{}, err
	}
	if lossPercent < risk.MinLossPercent || lossPercent > risk.MaxLossPercent {
		return routeLossResult{}, fmt.Errorf("loss percent %.4f outside %.4f..%.4f: %w", lossPercent, risk.MinLossPercent, risk.MaxLossPercent, ErrInvalidRouteLossRoll)
	}

	lost := int64(math.Floor(float64(amount) * lossPercent))
	if lost < 0 {
		lost = 0
	}
	if lost == 0 && lossPercent > 0 {
		lost = 1
	}
	if lost > amount {
		lost = amount
	}
	return routeLossResult{
		lost:        lost,
		delivered:   amount - lost,
		applied:     true,
		lossPercent: lossPercent,
	}, nil
}

func validateRouteRoll(name string, value float64) error {
	if err := validateFiniteRouteFloat(name, value, ErrInvalidRouteLossRoll); err != nil {
		return err
	}
	if value < 0 || value >= 1 {
		return fmt.Errorf("%s %.4f: %w", name, value, ErrInvalidRouteLossRoll)
	}
	return nil
}
