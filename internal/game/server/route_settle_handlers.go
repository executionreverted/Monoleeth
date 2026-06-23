package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world/maps"
)

type routeSettleIntent struct {
	RouteID string `json:"route_id,omitempty"`
}

type routeSettlementPayload struct {
	RouteID          string `json:"route_id"`
	ResourceItemID   string `json:"resource_item_id"`
	SettledAt        int64  `json:"settled_at"`
	ElapsedAppliedMS int64  `json:"elapsed_applied_ms"`
	WantedAmount     int64  `json:"wanted_amount"`
	TakenAmount      int64  `json:"taken_amount"`
	LostAmount       int64  `json:"lost_amount"`
	DeliveredAmount  int64  `json:"delivered_amount"`
	AddedAmount      int64  `json:"added_amount"`
	SourceEmpty      bool   `json:"source_empty"`
	DestinationFull  bool   `json:"destination_full"`
	LossApplied      bool   `json:"loss_applied"`
	NoOp             bool   `json:"no_op"`
}

var routeSettleServerOwnedPayloadKeys = []string{
	"owner",
	"owner_player_id",
	"player",
	"session",
	"route",
	"routes",
	"route_list",
	"result",
	"results",
	"settlement",
	"settlements",
	"settlement_result",
	"source",
	"source_id",
	"source_planet",
	"source_planet_id",
	"destination",
	"destination_id",
	"destination_planet_id",
	"destination_type",
	"source_map_id",
	"destination_map_id",
	"from_public_map_key",
	"to_public_map_key",
	"source_public_map_key",
	"destination_public_map_key",
	"enabled",
	"settled",
	"settled_at",
	"timestamp",
	"created_at",
	"updated_at",
	"last_calculated_at",
	"last_settlement_at",
	"elapsed",
	"elapsed_ms",
	"elapsed_applied",
	"elapsed_applied_ms",
	"elapsed_requested",
	"window",
	"settlement_window",
	"storage",
	"storage_truth",
	"energy",
	"energy_cost",
	"energy_cost_per_hour",
	"risk",
	"route_risk",
	"loss",
	"loss_chance",
	"loss_percent",
	"loss_min_percent",
	"loss_max_percent",
	"min_loss_percent",
	"max_loss_percent",
	"cost",
	"wanted_amount",
	"taken_amount",
	"lost_amount",
	"delivered_amount",
	"added_amount",
	"amount",
	"amount_per_hour",
	"rate",
	"resource",
	"resource_id",
	"resource_item_id",
	"cooldown",
	"position",
	"coordinates",
	"x",
	"y",
}

func (runtime *Runtime) handleRouteSettle(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := ctx.PlayerID.Validate(); err != nil {
		return nil, foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated player is required.", foundation.WithCause(err))
	}
	if err := rejectTrustedPayloadWithAdditional(request.Payload, routeSettleServerOwnedPayloadKeys...); err != nil {
		return nil, err
	}
	hasRouteID, err := routeSettleHasExactRouteID(request.Payload)
	if err != nil {
		return nil, err
	}
	var intent routeSettleIntent
	if err := decodeStrict(request.Payload, &intent); err != nil {
		return nil, err
	}
	now := runtime.clock.Now().UTC()

	if hasRouteID {
		routeID, err := foundation.ParseRouteID(intent.RouteID)
		if err != nil {
			return nil, invalidPayload("Route id is invalid.", err)
		}
		route, err := runtime.routeSettleRouteForOwner(ctx.PlayerID, routeID)
		if err != nil {
			return nil, domainErrorForRouteSettle(err)
		}
		if _, err := runtime.routePayloadFromRoute(route); err != nil {
			return nil, domainErrorForRouteSettle(err)
		}
		if err := runtime.preflightRouteSettleReadModelsAndStorage(ctx.PlayerID, []production.AutomationRoute{route}, now); err != nil {
			return nil, domainErrorForRouteSettle(err)
		}
		result, err := runtime.applyRouteSettlement(ctx.PlayerID, routeID, now)
		if err != nil {
			return nil, domainErrorForRouteSettle(err)
		}
		responsePayload, err := runtime.routeSettleResponseAndEvents(ctx.PlayerID, []production.RouteSettlementResult{result.Settlement}, true)
		if err != nil {
			return nil, domainErrorForRouteSettle(err)
		}
		return marshalPayload(responsePayload)
	}

	ownerRoutes, err := runtime.ownerAutomationRoutes(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRouteSettle(err)
	}
	if err := runtime.preflightRouteSettleReadModelsAndStorage(ctx.PlayerID, ownerRoutes, now); err != nil {
		return nil, domainErrorForRouteSettle(err)
	}
	settlements := make([]production.RouteSettlementResult, 0, len(ownerRoutes))
	for _, route := range ownerRoutes {
		result, err := runtime.applyRouteSettlement(ctx.PlayerID, route.RouteID, now)
		if err != nil {
			return nil, domainErrorForRouteSettle(err)
		}
		settlements = append(settlements, result.Settlement)
	}
	responsePayload, err := runtime.routeSettleResponseAndEvents(ctx.PlayerID, settlements, false)
	if err != nil {
		return nil, domainErrorForRouteSettle(err)
	}
	return marshalPayload(responsePayload)
}

func (runtime *Runtime) routeSettleRouteForOwner(
	playerID foundation.PlayerID,
	routeID foundation.RouteID,
) (production.AutomationRoute, error) {
	route, ok, err := runtime.Production.AutomationRoute(routeID)
	if err != nil {
		return production.AutomationRoute{}, err
	}
	if !ok {
		record, durableOK, durableErr := runtime.Production.CommittedAutomationRouteDurableRecord(routeID)
		if durableErr != nil {
			return production.AutomationRoute{}, durableErr
		}
		if !durableOK {
			return production.AutomationRoute{}, fmt.Errorf("route %q: %w", routeID, production.ErrRouteNotFound)
		}
		route = record.Route
	}
	if err := route.Validate(); err != nil {
		return production.AutomationRoute{}, err
	}
	if route.OwnerPlayerID != playerID {
		return production.AutomationRoute{}, fmt.Errorf("route %q owner %q: %w", routeID, playerID, production.ErrRouteOwnerMismatch)
	}
	return route, nil
}

func (runtime *Runtime) applyRouteSettlement(
	playerID foundation.PlayerID,
	routeID foundation.RouteID,
	now time.Time,
) (production.RouteSettlementTransactionResult, error) {
	result, err := runtime.Production.ApplyRouteSettlementTransaction(production.RouteSettlementTransactionInput{
		OwnerPlayerID: playerID,
		RouteID:       routeID,
		SettledAt:     now,
	})
	if err != nil {
		return production.RouteSettlementTransactionResult{}, err
	}
	if _, err := result.ApplyDurableCommit(runtime.Settlements); err != nil {
		return production.RouteSettlementTransactionResult{}, err
	}
	return result, nil
}

func (runtime *Runtime) preflightRouteSettleReadModelsAndStorage(
	playerID foundation.PlayerID,
	routes []production.AutomationRoute,
	now time.Time,
) error {
	if _, err := runtime.routeListPayload(playerID); err != nil {
		return err
	}
	includeSettlementSnapshots := false
	for _, route := range routes {
		if route.OwnerPlayerID != playerID {
			return fmt.Errorf("route %q owner %q: %w", route.RouteID, playerID, production.ErrRouteOwnerMismatch)
		}
		touchesStorage, err := runtime.preflightRouteSettlementPrerequisites(route, now)
		if err != nil {
			return err
		}
		includeSettlementSnapshots = includeSettlementSnapshots || touchesStorage
	}
	if includeSettlementSnapshots {
		if _, err := runtime.productionSummaryPayload(playerID, ""); err != nil {
			return err
		}
		if _, err := runtime.storageSummaryPayload(playerID, ""); err != nil {
			return err
		}
	}
	return nil
}

func (runtime *Runtime) preflightRouteSettlementPrerequisites(route production.AutomationRoute, now time.Time) (bool, error) {
	if err := route.Validate(); err != nil {
		return false, err
	}
	if now.IsZero() {
		return false, fmt.Errorf("now: %w", production.ErrZeroProductionTimestamp)
	}
	elapsedRequested := now.UTC().Sub(route.LastCalculatedAt.UTC())
	if elapsedRequested <= 0 || !route.Enabled {
		return false, nil
	}
	elapsedApplied := elapsedRequested
	if elapsedApplied > production.DefaultMaxRouteOfflineSettlementDuration {
		elapsedApplied = production.DefaultMaxRouteOfflineSettlementDuration
	}
	wantedAmount := routeSettleWholeUnitsForElapsed(route.AmountPerHour, elapsedApplied)

	source, ok, err := runtime.Production.PlanetStorage(route.SourcePlanetID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("route %q source planet %q: %w", route.RouteID, route.SourcePlanetID, production.ErrRouteSourceStorageMissing)
	}
	if err := source.Validate(); err != nil {
		return false, err
	}

	destinationStorageID, err := production.RouteSettlementDestinationStorageID(route.Destination)
	if err != nil {
		return false, fmt.Errorf("route %q destination %q: %w", route.RouteID, route.Destination.Type, err)
	}
	if route.SourcePlanetID == destinationStorageID {
		return wantedAmount >= 1, nil
	}
	destination, ok, err := runtime.Production.PlanetStorage(destinationStorageID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("route %q destination storage %q: %w", route.RouteID, destinationStorageID, production.ErrRouteDestinationStorageMissing)
	}
	if err := destination.Validate(); err != nil {
		return false, err
	}
	return wantedAmount >= 1, nil
}

func routeSettleWholeUnitsForElapsed(amountPerHour int64, elapsed time.Duration) int64 {
	if amountPerHour <= 0 || elapsed <= 0 {
		return 0
	}
	quantity := math.Floor(elapsed.Hours() * float64(amountPerHour))
	if quantity < 1 {
		return 0
	}
	if quantity > float64(foundation.MaxAmount) {
		return foundation.MaxAmount
	}
	return int64(quantity)
}

func routeSettleHasExactRouteID(payload json.RawMessage) (bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return false, invalidPayload("Invalid payload.", err)
	}
	if fields == nil {
		return false, invalidPayload("Invalid payload.", nil)
	}
	if len(fields) == 0 {
		return false, nil
	}
	if len(fields) == 1 {
		if _, ok := fields["route_id"]; ok {
			return true, nil
		}
	}
	return false, invalidPayload("Payload must be empty or contain only route_id.", nil)
}

func (runtime *Runtime) ownerAutomationRoutes(playerID foundation.PlayerID) ([]production.AutomationRoute, error) {
	routesByID := make(map[foundation.RouteID]production.AutomationRoute)
	for _, route := range runtime.Production.AutomationRoutes() {
		if route.OwnerPlayerID == playerID {
			routesByID[route.RouteID] = route
		}
	}
	durableRecords, err := runtime.Production.CommittedAutomationRouteDurableRecordsForOwner(playerID)
	if err != nil {
		return nil, err
	}
	for _, record := range durableRecords {
		if _, ok := routesByID[record.Route.RouteID]; !ok {
			routesByID[record.Route.RouteID] = record.Route
		}
	}
	routeIDs := make([]foundation.RouteID, 0, len(routesByID))
	for routeID := range routesByID {
		routeIDs = append(routeIDs, routeID)
	}
	sort.Slice(routeIDs, func(i, j int) bool { return routeIDs[i] < routeIDs[j] })
	owned := make([]production.AutomationRoute, 0, len(routeIDs))
	for _, routeID := range routeIDs {
		owned = append(owned, routesByID[routeID])
	}
	return owned, nil
}

func (runtime *Runtime) routeSettleResponseAndEvents(
	playerID foundation.PlayerID,
	settlements []production.RouteSettlementResult,
	singleRoute bool,
) (map[string]any, error) {
	routes, err := runtime.routeListPayload(playerID)
	if err != nil {
		return nil, err
	}

	type routeSettlementEventPayload struct {
		route      routePayload
		settlement routeSettlementPayload
	}
	eventPayloads := make([]routeSettlementEventPayload, 0, len(settlements))
	settlementPayloads := make([]routeSettlementPayload, 0, len(settlements))
	includeSettlementSnapshots := false
	responsePayload := map[string]any{
		"routes": routes,
	}

	for index, settlement := range settlements {
		routePayload, err := runtime.routePayloadFromRoute(settlement.AfterRoute)
		if err != nil {
			return nil, err
		}
		settlementPayload, err := routeSettlementPayloadFromResult(settlement)
		if err != nil {
			return nil, err
		}
		if singleRoute && index == 0 {
			responsePayload["route"] = routePayload
			responsePayload["settlement"] = settlementPayload
		}
		settlementPayloads = append(settlementPayloads, settlementPayload)
		eventPayloads = append(eventPayloads, routeSettlementEventPayload{
			route:      routePayload,
			settlement: settlementPayload,
		})
		includeSettlementSnapshots = includeSettlementSnapshots || routeSettlementTouchedStorage(settlement)
	}
	if singleRoute && len(settlements) != 1 {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Route settlement response was incomplete.")
	}
	if !singleRoute {
		responsePayload["settlements"] = settlementPayloads
	}

	var productionPayload planetProductionCollectionPayload
	var storagePayload planetStorageCollectionPayload
	if includeSettlementSnapshots {
		productionPayload, err = runtime.productionSummaryPayload(playerID, "")
		if err != nil {
			return nil, err
		}
		storagePayload, err = runtime.storageSummaryPayload(playerID, "")
		if err != nil {
			return nil, err
		}
		responsePayload["production"] = productionPayload
		responsePayload["storage"] = storagePayload
	}

	if len(eventPayloads) > 0 || includeSettlementSnapshots {
		runtime.mu.Lock()
		for _, payload := range eventPayloads {
			runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventRouteSettled, map[string]any{
				"route":      payload.route,
				"settlement": payload.settlement,
			})
			runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventRouteUpdated, map[string]any{"route": payload.route})
			runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventRouteSnapshot, map[string]any{"route": payload.route})
		}
		if len(eventPayloads) > 0 {
			runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventRouteList, map[string]any{"routes": routes})
		}
		if includeSettlementSnapshots {
			runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventProductionSummary, productionPayload)
			runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventPlanetStorage, storagePayload)
		}
		runtime.mu.Unlock()
	}

	return responsePayload, nil
}

func routeSettlementPayloadFromResult(result production.RouteSettlementResult) (routeSettlementPayload, error) {
	payload, err := production.NewRouteSettlementPayload(result)
	if err != nil {
		return routeSettlementPayload{}, err
	}
	return routeSettlementPayload{
		RouteID:          payload.RouteID.String(),
		ResourceItemID:   payload.ResourceItemID.String(),
		SettledAt:        payload.SettledAt.UTC().UnixMilli(),
		ElapsedAppliedMS: payload.ElapsedApplied.Milliseconds(),
		WantedAmount:     payload.WantedAmount,
		TakenAmount:      payload.TakenAmount,
		LostAmount:       payload.LostAmount,
		DeliveredAmount:  payload.DeliveredAmount,
		AddedAmount:      payload.AddedAmount,
		SourceEmpty:      payload.SourceEmpty,
		DestinationFull:  payload.DestinationFull,
		LossApplied:      payload.LossApplied,
		NoOp:             payload.NoOp,
	}, nil
}

func domainErrorForRouteSettle(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, production.ErrRouteNotFound),
		errors.Is(err, production.ErrRouteOwnerMismatch):
		return foundation.NewDomainError(foundation.CodeNotFound, "Route was not found.", foundation.WithCause(err))
	case errors.Is(err, production.ErrRouteSourceStorageMissing),
		errors.Is(err, production.ErrRouteDestinationStorageMissing),
		errors.Is(err, production.ErrUnsupportedRouteDestination),
		errors.Is(err, maps.ErrMapNotFound):
		return foundation.NewDomainError(foundation.CodeNotFound, "Route endpoint was not found.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Route settlement failed.", foundation.WithCause(err))
	}
}
