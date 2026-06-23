package server

import (
	"encoding/json"
	"errors"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world/maps"
)

type routeControlIntent struct {
	RouteID string `json:"route_id"`
}

type routeControlAction func(*production.AutomationRouteService, foundation.PlayerID, foundation.RouteID, foundation.RequestID) (production.RouteControlResult, error)

var routeControlServerOwnedPayloadKeys = []string{
	"owner",
	"owner_player_id",
	"player",
	"session",
	"route",
	"routes",
	"source",
	"source_id",
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
	"settlement",
	"settlement_result",
	"settled",
	"timestamp",
	"created_at",
	"updated_at",
	"last_calculated_at",
	"last_settlement_at",
	"storage",
	"storage_truth",
	"energy",
	"energy_cost",
	"energy_cost_per_hour",
	"risk",
	"route_risk",
	"loss",
	"loss_chance",
	"loss_min_percent",
	"loss_max_percent",
	"min_loss_percent",
	"max_loss_percent",
	"cost",
	"rate",
	"amount",
	"amount_per_hour",
	"resource",
	"resource_id",
	"resource_item_id",
	"cooldown",
	"position",
	"coordinates",
	"x",
	"y",
}

func (runtime *Runtime) handleRouteEnable(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	return runtime.handleRouteControl(ctx, request, (*production.AutomationRouteService).EnableRouteForOwnerWithRequest)
}

func (runtime *Runtime) handleRouteDisable(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	return runtime.handleRouteControl(ctx, request, (*production.AutomationRouteService).DisableRouteForOwnerWithRequest)
}

func (runtime *Runtime) handleRouteControl(
	ctx realtime.CommandContext,
	request realtime.RequestEnvelope,
	control routeControlAction,
) (json.RawMessage, error) {
	if err := ctx.PlayerID.Validate(); err != nil {
		return nil, foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated player is required.", foundation.WithCause(err))
	}
	if err := rejectTrustedPayloadWithAdditional(request.Payload, routeControlServerOwnedPayloadKeys...); err != nil {
		return nil, err
	}
	var intent routeControlIntent
	if err := decodeStrict(request.Payload, &intent); err != nil {
		return nil, err
	}
	routeID, err := foundation.ParseRouteID(intent.RouteID)
	if err != nil {
		return nil, invalidPayload("Route id is invalid.", err)
	}

	service, err := production.NewAutomationRouteService(production.AutomationRouteServiceConfig{
		Store:  runtime.Production,
		Clock:  runtime.clock,
		Policy: runtimeRouteCreatePolicyProvider{runtime: runtime},
	})
	if err != nil {
		return nil, domainErrorForRouteControl(err)
	}
	result, err := control(service, ctx.PlayerID, routeID, request.RequestID)
	if err != nil {
		return nil, domainErrorForRouteControl(err)
	}

	responsePayload, err := runtime.routeMutationResponseAndEvents(ctx.PlayerID, result.Route, result.Settlement)
	if err != nil {
		return nil, domainErrorForRouteControl(err)
	}

	return marshalPayload(responsePayload)
}

func (runtime *Runtime) routeMutationResponseAndEvents(
	playerID foundation.PlayerID,
	route production.AutomationRoute,
	settlement production.RouteSettlementResult,
) (map[string]any, error) {
	routePayload, err := runtime.routePayloadFromRoute(route)
	if err != nil {
		return nil, err
	}
	routes, err := runtime.routeListPayload(playerID)
	if err != nil {
		return nil, err
	}

	responsePayload := map[string]any{
		"route":  routePayload,
		"routes": routes,
	}
	includeSettlementSnapshots := routeSettlementTouchedStorage(settlement)
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

	runtime.mu.Lock()
	runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventRouteUpdated, map[string]any{"route": routePayload})
	runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventRouteSnapshot, map[string]any{"route": routePayload})
	runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventRouteList, map[string]any{"routes": routes})
	if includeSettlementSnapshots {
		runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventProductionSummary, productionPayload)
		runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventPlanetStorage, storagePayload)
	}
	runtime.mu.Unlock()

	return responsePayload, nil
}

func routeSettlementTouchedStorage(settlement production.RouteSettlementResult) bool {
	if settlement.NoOp || settlement.RouteID.IsZero() {
		return false
	}
	return settlement.TakenAmount > 0 ||
		settlement.AddedAmount > 0 ||
		settlement.LostAmount > 0 ||
		settlement.SourceEmpty ||
		settlement.DestinationFull
}

func domainErrorForRouteControl(err error) error {
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
	case errors.Is(err, production.ErrRouteEnergyUnavailable):
		return foundation.NewDomainError(foundation.CodeForbidden, "Route requirements are not met.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Route control failed.", foundation.WithCause(err))
	}
}
