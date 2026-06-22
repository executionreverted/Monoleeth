package server

import (
	"encoding/json"
	"errors"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world/maps"
)

type routeUpdateIntent struct {
	RouteID             string `json:"route_id"`
	DestinationPlanetID string `json:"destination_planet_id"`
	ResourceItemID      string `json:"resource_item_id"`
	AmountPerHour       int64  `json:"amount_per_hour"`
}

var routeUpdateServerOwnedPayloadKeys = []string{
	"owner",
	"owner_player_id",
	"player",
	"session",
	"route",
	"routes",
	"source",
	"source_id",
	"source_planet",
	"source_planet_id",
	"source_map",
	"source_map_id",
	"source_map_key",
	"source_public_map_key",
	"from_public_map_key",
	"destination",
	"destination_id",
	"destination_type",
	"destination_map",
	"destination_map_id",
	"destination_map_key",
	"destination_public_map_key",
	"to_public_map_key",
	"enabled",
	"settlement",
	"settlement_result",
	"settled",
	"settled_at",
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
	"cost",
	"risk",
	"route_risk",
	"loss",
	"loss_chance",
	"loss_min_percent",
	"loss_max_percent",
	"min_loss_percent",
	"max_loss_percent",
	"cooldown",
	"position",
	"coordinates",
	"x",
	"y",
	"amount",
	"amount_hourly",
	"amount_per_minute",
	"amount_per_second",
	"hourly_amount",
	"per_hour",
	"rate",
	"rate_per_hour",
	"resource",
	"resource_id",
}

func (runtime *Runtime) handleRouteUpdate(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := ctx.PlayerID.Validate(); err != nil {
		return nil, foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated player is required.", foundation.WithCause(err))
	}
	if err := rejectTrustedPayloadWithAdditional(request.Payload, routeUpdateServerOwnedPayloadKeys...); err != nil {
		return nil, err
	}
	var intent routeUpdateIntent
	if err := decodeStrict(request.Payload, &intent); err != nil {
		return nil, err
	}
	routeID, err := foundation.ParseRouteID(intent.RouteID)
	if err != nil {
		return nil, invalidPayload("Route id is invalid.", err)
	}
	destinationPlanetID, err := foundation.ParsePlanetID(intent.DestinationPlanetID)
	if err != nil {
		return nil, invalidPayload("Destination planet is invalid.", err)
	}
	resourceItemID, err := foundation.ParseItemID(intent.ResourceItemID)
	if err != nil {
		return nil, invalidPayload("Route resource is invalid.", err)
	}
	destination, err := production.NewPlanetRouteDestination(destinationPlanetID)
	if err != nil {
		return nil, invalidPayload("Route destination is invalid.", err)
	}

	service, err := production.NewAutomationRouteService(production.AutomationRouteServiceConfig{
		Store:  runtime.Production,
		Clock:  runtime.clock,
		Policy: runtimeRouteCreatePolicyProvider{runtime: runtime},
	})
	if err != nil {
		return nil, domainErrorForRouteUpdate(err)
	}
	result, err := service.UpdateRouteForOwner(ctx.PlayerID, production.UpdateRouteInput{
		RouteID:        routeID,
		Destination:    destination,
		ResourceItemID: resourceItemID,
		AmountPerHour:  intent.AmountPerHour,
	})
	if err != nil {
		return nil, domainErrorForRouteUpdate(err)
	}

	responsePayload, err := runtime.routeMutationResponseAndEvents(ctx.PlayerID, result.Route, result.Settlement)
	if err != nil {
		return nil, domainErrorForRouteUpdate(err)
	}
	return marshalPayload(responsePayload)
}

func domainErrorForRouteUpdate(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, production.ErrInvalidRouteRate):
		return invalidPayload("Route update payload is invalid.", err)
	case errors.Is(err, production.ErrRouteNotFound),
		errors.Is(err, production.ErrRouteOwnerMismatch):
		return foundation.NewDomainError(foundation.CodeNotFound, "Route was not found.", foundation.WithCause(err))
	case errors.Is(err, production.ErrRouteSourceNotOwned):
		return foundation.NewDomainError(foundation.CodeForbidden, "Route source planet is not accessible.", foundation.WithCause(err))
	case errors.Is(err, production.ErrRouteDestinationNotAccessible),
		errors.Is(err, production.ErrRouteSourceStorageMissing),
		errors.Is(err, production.ErrRouteDestinationStorageMissing),
		errors.Is(err, production.ErrUnsupportedRouteDestination),
		errors.Is(err, maps.ErrMapNotFound):
		return foundation.NewDomainError(foundation.CodeNotFound, "Route endpoint was not found.", foundation.WithCause(err))
	case errors.Is(err, production.ErrRouteResourceNotRouteable):
		return foundation.NewDomainError(foundation.CodeForbidden, "Route resource is not routeable.", foundation.WithCause(err))
	case errors.Is(err, production.ErrRouteRequirementNotMet),
		errors.Is(err, production.ErrRouteDistanceTooFar):
		return foundation.NewDomainError(foundation.CodeForbidden, "Route requirements are not met.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Route update failed.", foundation.WithCause(err))
	}
}
