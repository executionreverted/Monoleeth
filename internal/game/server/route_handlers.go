package server

import (
	"encoding/json"
	"errors"
	"fmt"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	worldmaps "gameproject/internal/game/world/maps"
)

const runtimeRouteCreateMaxRoutesPerPlayer = 3

type routeCreateIntent struct {
	SourcePlanetID      string `json:"source_planet_id"`
	DestinationPlanetID string `json:"destination_planet_id"`
	ResourceItemID      string `json:"resource_item_id"`
	AmountPerHour       int64  `json:"amount_per_hour"`
}

func (runtime *Runtime) handleRouteCreate(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := ctx.PlayerID.Validate(); err != nil {
		return nil, foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated player is required.", foundation.WithCause(err))
	}
	if err := rejectTrustedPayloadWithAdditional(
		request.Payload,
		"owner",
		"owner_player_id",
		"player",
		"session",
		"route_id",
		"source_map_id",
		"destination",
		"destination_id",
		"destination_type",
		"destination_map_id",
		"from_public_map_key",
		"to_public_map_key",
		"source_public_map_key",
		"destination_public_map_key",
		"energy",
		"energy_cost",
		"energy_cost_per_hour",
		"risk",
		"route_risk",
		"loss_chance",
		"cost",
		"enabled",
		"timestamp",
		"created_at",
		"updated_at",
		"last_calculated_at",
		"position",
		"coordinates",
		"cooldown",
		"capacity",
		"route_capacity",
		"route_count",
		"current_route_count",
		"max_route_count",
		"storage",
		"storage_truth",
	); err != nil {
		return nil, err
	}
	var intent routeCreateIntent
	if err := decodeStrict(request.Payload, &intent); err != nil {
		return nil, err
	}

	sourcePlanetID, err := foundation.ParsePlanetID(intent.SourcePlanetID)
	if err != nil {
		return nil, invalidPayload("Source planet is invalid.", err)
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
	routeID, err := foundation.ParseRouteID("route-" + request.RequestID.String())
	if err != nil {
		return nil, invalidPayload("Route request is invalid.", err)
	}

	service, err := production.NewAutomationRouteService(production.AutomationRouteServiceConfig{
		Store:  runtime.Production,
		Clock:  runtime.clock,
		Policy: runtimeRouteCreatePolicyProvider{runtime: runtime},
	})
	if err != nil {
		return nil, domainErrorForRouteCreate(err)
	}
	result, err := service.CreateRoute(production.CreateRouteInput{
		RouteID:        routeID,
		OwnerPlayerID:  ctx.PlayerID,
		SourcePlanetID: sourcePlanetID,
		Destination:    destination,
		ResourceItemID: resourceItemID,
		AmountPerHour:  intent.AmountPerHour,
	})
	if err != nil {
		return nil, domainErrorForRouteCreate(err)
	}

	routePayload, err := runtime.routePayloadFromRoute(result.Route)
	if err != nil {
		return nil, domainErrorForRouteCreate(err)
	}
	routes, err := runtime.routeListPayload(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRouteCreate(err)
	}

	runtime.mu.Lock()
	runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventRouteUpdated, map[string]any{"route": routePayload})
	runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventRouteSnapshot, map[string]any{"route": routePayload})
	runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventRouteList, map[string]any{"routes": routes})
	runtime.mu.Unlock()

	return marshalPayload(map[string]any{
		"route":  routePayload,
		"routes": routes,
	})
}

type runtimeRouteCreatePolicyProvider struct {
	runtime *Runtime
}

func (provider runtimeRouteCreatePolicyProvider) RouteCreatePolicy(input production.RouteCreatePolicyInput) (production.RouteCreatePolicy, error) {
	if err := input.Validate(); err != nil {
		return production.RouteCreatePolicy{}, err
	}
	if provider.runtime == nil || provider.runtime.Discovery == nil || provider.runtime.Production == nil || provider.runtime.mapCatalog == nil {
		return production.RouteCreatePolicy{}, production.ErrInvalidRouteCreateConfig
	}
	source, ok, err := provider.runtime.Discovery.Planet(input.SourcePlanetID)
	if err != nil {
		return production.RouteCreatePolicy{}, err
	}
	if !ok || source.OwnerPlayerID != input.OwnerPlayerID {
		return production.RouteCreatePolicy{}, production.ErrRouteSourceNotOwned
	}
	if input.Destination.Type != production.RouteDestinationTypePlanet {
		return production.RouteCreatePolicy{}, production.ErrRouteDestinationNotAccessible
	}
	destinationPlanetID, err := foundation.ParsePlanetID(input.Destination.ID.String())
	if err != nil {
		return production.RouteCreatePolicy{}, production.ErrRouteDestinationNotAccessible
	}
	destination, ok, err := provider.runtime.Discovery.Planet(destinationPlanetID)
	if err != nil {
		return production.RouteCreatePolicy{}, err
	}
	if !ok || destination.OwnerPlayerID != input.OwnerPlayerID {
		return production.RouteCreatePolicy{}, production.ErrRouteDestinationNotAccessible
	}
	if source.WorldID != destination.WorldID {
		return production.RouteCreatePolicy{}, production.ErrRouteDestinationNotAccessible
	}

	sourceMapID, err := routeMapIDForPlanet(provider.runtime.mapCatalog, source.WorldID, source.ZoneID)
	if err != nil {
		return production.RouteCreatePolicy{}, err
	}
	destinationMapID, err := routeMapIDForPlanet(provider.runtime.mapCatalog, destination.WorldID, destination.ZoneID)
	if err != nil {
		return production.RouteCreatePolicy{}, err
	}
	if !provider.runtime.routeResourceAvailable(input.ResourceItemID) {
		return production.RouteCreatePolicy{}, production.ErrRouteResourceNotRouteable
	}
	ownerRoutes, err := provider.runtime.ownerAutomationRoutes(input.OwnerPlayerID)
	if err != nil {
		return production.RouteCreatePolicy{}, err
	}

	distance := source.Coordinates.Distance(destination.Coordinates)
	if sourceMapID != destinationMapID {
		distance += 1000
	}
	return production.RouteCreatePolicy{
		SourcePlanetOwned:     true,
		DestinationAccessible: true,
		ResourceRouteable:     true,
		RequirementsMet:       true,
		SourceMapID:           sourceMapID,
		DestinationMapID:      destinationMapID,
		DistanceUnits:         distance,
		MaxDistanceUnits:      25_000,
		CurrentRouteCount:     len(ownerRoutes),
		MaxRouteCount:         runtimeRouteCreateMaxRoutesPerPlayer,
		EnergyCostPerHour:     1 + input.AmountPerHour/20,
		MinLossPercent:        0,
		MaxLossPercent:        0,
	}, nil
}

func routeMapIDForPlanet(catalog *worldmaps.Catalog, worldID foundation.WorldID, zoneID foundation.ZoneID) (production.RouteMapID, error) {
	mapID := worldmaps.MapID(zoneID.String())
	definition, ok := catalog.Get(mapID)
	if !ok {
		return "", worldmaps.ErrMapNotFound
	}
	if definition.WorldID != worldID || definition.ZoneID != zoneID {
		return "", fmt.Errorf("planet map %q world/zone mismatch", mapID)
	}
	routeMapID := production.RouteMapID(definition.InternalMapID.String())
	if err := routeMapID.Validate(); err != nil {
		return "", err
	}
	return routeMapID, nil
}

func (runtime *Runtime) routeResourceAvailable(itemID foundation.ItemID) bool {
	if runtime == nil {
		return false
	}
	if !isRouteCreateMVPRouteableResource(itemID) {
		return false
	}
	_, ok := runtime.itemCatalog[itemID]
	return ok
}

func isRouteCreateMVPRouteableResource(itemID foundation.ItemID) bool {
	switch itemID {
	case "refined_alloy":
		return true
	default:
		return false
	}
}

func domainErrorForRouteCreate(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, production.ErrInvalidRouteRate),
		errors.Is(err, production.ErrInvalidRouteDestinationID),
		errors.Is(err, production.ErrInvalidRouteDestinationType),
		errors.Is(err, production.ErrInvalidRouteEnergyCost),
		errors.Is(err, production.ErrInvalidRouteRisk),
		errors.Is(err, production.ErrInvalidRouteDistance),
		errors.Is(err, production.ErrInvalidRouteMapID),
		errors.Is(err, foundation.ErrEmptyID),
		errors.Is(err, foundation.ErrInvalidID):
		return invalidPayload("Route create payload is invalid.", err)
	case errors.Is(err, production.ErrRouteSourceNotOwned):
		return foundation.NewDomainError(foundation.CodeForbidden, "Route source planet is not accessible.", foundation.WithCause(err))
	case errors.Is(err, production.ErrRouteDestinationNotAccessible):
		return foundation.NewDomainError(foundation.CodeNotFound, "Route destination planet was not found.", foundation.WithCause(err))
	case errors.Is(err, production.ErrRouteResourceNotRouteable):
		return foundation.NewDomainError(foundation.CodeForbidden, "Route resource is not routeable.", foundation.WithCause(err))
	case errors.Is(err, production.ErrRouteRequirementNotMet),
		errors.Is(err, production.ErrRouteCapacityExceeded),
		errors.Is(err, production.ErrRouteEnergyUnavailable),
		errors.Is(err, production.ErrRouteDistanceTooFar):
		return foundation.NewDomainError(foundation.CodeForbidden, "Route requirements are not met.", foundation.WithCause(err))
	case errors.Is(err, production.ErrDuplicateRoute):
		return foundation.NewDomainError(foundation.CodeForbidden, "Route already exists.", foundation.WithCause(err))
	default:
		if errors.Is(err, worldmaps.ErrMapNotFound) {
			return foundation.NewDomainError(foundation.CodeNotFound, "Route endpoint was not found.", foundation.WithCause(err))
		}
		return foundation.NewDomainError(foundation.CodeInternal, "Route create failed.", foundation.WithCause(err))
	}
}
