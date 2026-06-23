package server

import (
	"fmt"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	worldmaps "gameproject/internal/game/world/maps"
)

func knownPlanetPayloadFromIntel(playerID foundation.PlayerID, planet discovery.Planet, intel discovery.PlayerPlanetIntel, publicMapKey string) knownPlanetPayload {
	return knownPlanetPayload{
		PlanetID:     planet.ID.String(),
		PublicMapKey: publicMapKey,
		Biome:        string(planet.Biome),
		PlanetType:   string(planet.Type),
		Rarity:       string(planet.Rarity),
		Level:        planet.Level,
		IntelState:   string(intel.State),
		Confidence:   intel.Confidence,
		LastSeenAt:   intel.LastSeenAt.UTC().UnixMilli(),
		OwnerStatus:  publicPlanetOwnerStatus(playerID, planet),
		DiscoveredAt: planet.DiscoveredAt.UTC().UnixMilli(),
	}
}

func publicPlanetOwnerStatus(playerID foundation.PlayerID, planet discovery.Planet) string {
	if planet.OwnerPlayerID.IsZero() {
		return "unclaimed"
	}
	if planet.OwnerPlayerID == playerID {
		return "owned_by_you"
	}
	return "claimed"
}

func scanPulsePayloadFromStart(start discovery.StartScanPulseResult) scanPulsePayload {
	return scanPulsePayload{
		PulseReference: string(start.PulseReference),
		Status:         string(start.Status),
		ResolveAfter:   start.ResolveAfter.UTC().UnixMilli(),
	}
}

func scanPulsePayloadFromResult(result discovery.ResolveScanPulseResult) scanPulsePayload {
	return scanPulsePayload{
		PulseReference: string(result.PulseReference),
		Status:         string(result.Status),
		Message:        result.Message,
		Signal:         result.Signal,
		PlanetID:       result.PlanetID.String(),
		XPGranted:      result.XPGranted,
		Duplicate:      result.Duplicate,
	}
}

func (runtime *Runtime) publicMapKeyForPlanet(planet discovery.Planet) (string, error) {
	if runtime == nil || runtime.mapCatalog == nil {
		return "", fmt.Errorf("map catalog unavailable")
	}
	mapID := worldmaps.MapID(planet.ZoneID.String())
	definition, ok := runtime.mapCatalog.Get(mapID)
	if !ok {
		return "", fmt.Errorf("planet %q map %q: %w", planet.ID, mapID, worldmaps.ErrMapNotFound)
	}
	if definition.WorldID != planet.WorldID || definition.ZoneID != planet.ZoneID {
		return "", fmt.Errorf("planet %q map %q world/zone mismatch", planet.ID, mapID)
	}
	return publicMapKeyFromProjection(definition.ClientProjection()), nil
}

func (runtime *Runtime) publicMapKeyForRouteMapID(routeMapID production.RouteMapID) (string, error) {
	if runtime == nil || runtime.mapCatalog == nil {
		return "", fmt.Errorf("map catalog unavailable")
	}
	if err := routeMapID.Validate(); err != nil {
		return "", err
	}
	projection, err := runtime.mapCatalog.ClientProjection(worldmaps.MapID(routeMapID.String()))
	if err != nil {
		return "", err
	}
	return publicMapKeyFromProjection(projection), nil
}

func planetProductionPayloadFromSnapshot(snapshot production.PlanetProductionSnapshot, publicMapKey string) planetProductionPayload {
	buildings := make([]planetBuildingPayload, 0, len(snapshot.Buildings))
	for _, building := range snapshot.Buildings {
		buildings = append(buildings, planetBuildingPayload{
			PlanetID:     building.PlanetID.String(),
			PublicMapKey: publicMapKey,
			BuildingID:   building.BuildingID.String(),
			BuildingType: building.BuildingType.String(),
			Category:     productionDefinitionCategory(building),
			Level:        building.Level,
			State:        building.State.String(),
			UpdatedAt:    building.UpdatedAt.UTC().UnixMilli(),
		})
	}
	return planetProductionPayload{
		PlanetID:              snapshot.State.PlanetID.String(),
		PublicMapKey:          publicMapKey,
		ProductionEnabled:     snapshot.State.ProductionEnabled,
		LastCalculatedAt:      snapshot.State.LastCalculatedAt.UTC().UnixMilli(),
		EnergyCapacityPerHour: snapshot.State.EnergyCapacityPerHour,
		EnergyReservedPerHour: snapshot.State.EnergyReservedPerHour,
		Storage:               planetStoragePayloadFromStorage(snapshot.Storage, publicMapKey),
		Buildings:             buildings,
	}
}

func planetStoragePayloadFromStorage(storage production.PlanetStorage, publicMapKey string) planetStoragePayload {
	items := make([]planetStorageItem, 0, len(storage.Items))
	for _, item := range storage.Items {
		items = append(items, planetStorageItem{
			ItemID:   item.ItemID.String(),
			Quantity: item.Quantity,
		})
	}
	return planetStoragePayload{
		PlanetID:      storage.PlanetID.String(),
		PublicMapKey:  publicMapKey,
		UsedUnits:     storage.UsedUnits(),
		FreeUnits:     storage.FreeUnits(),
		CapacityUnits: storage.CapacityUnits,
		UpdatedAt:     storage.UpdatedAt.UTC().UnixMilli(),
		Items:         items,
	}
}

func productionDefinitionCategory(building production.PlanetBuilding) string {
	switch building.BuildingType {
	case production.BuildingTypeIronExtractor:
		return production.BuildingCategoryExtractor.String()
	case production.BuildingTypeAlloyFoundry:
		return production.BuildingCategoryRefinery.String()
	default:
		return ""
	}
}

func (runtime *Runtime) routePayloadFromRoute(route production.AutomationRoute) (routePayload, error) {
	if err := route.Validate(); err != nil {
		return routePayload{}, err
	}
	fromPublicMapKey, err := runtime.publicMapKeyForRouteMapID(route.SourceMapID)
	if err != nil {
		return routePayload{}, err
	}
	toPublicMapKey, err := runtime.publicMapKeyForRouteMapID(route.DestinationMapID)
	if err != nil {
		return routePayload{}, err
	}
	destinationPayload := routeDestinationPayloadFromRouteDestination(route.Destination)
	return routePayload{
		RouteID:           route.RouteID.String(),
		SourcePlanetID:    route.SourcePlanetID.String(),
		FromPublicMapKey:  fromPublicMapKey,
		ToPublicMapKey:    toPublicMapKey,
		Destination:       destinationPayload,
		ResourceItemID:    route.ResourceItemID.String(),
		AmountPerHour:     route.AmountPerHour,
		EnergyCostPerHour: route.EnergyCostPerHour,
		Enabled:           route.Enabled,
		Risk: routeRiskPayload{
			LossChance:     route.Risk.LossChance,
			MinLossPercent: route.Risk.MinLossPercent,
			MaxLossPercent: route.Risk.MaxLossPercent,
		},
		LastCalculatedAt: route.LastCalculatedAt.UTC().UnixMilli(),
		UpdatedAt:        route.UpdatedAt.UTC().UnixMilli(),
	}, nil
}

func routeDestinationPayloadFromRouteDestination(destination production.RouteDestination) routeDestinationPayload {
	payload := routeDestinationPayload{
		Type: destination.Type.String(),
	}
	if destination.Type == production.RouteDestinationTypePlanet {
		payload.ID = destination.ID.String()
	}
	return payload
}
