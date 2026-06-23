package server

import (
	"strings"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
)

type routeDestinationIntent struct {
	DestinationPlanetID string `json:"destination_planet_id"`
	DestinationType     string `json:"destination_type"`
	DestinationID       string `json:"destination_id"`
}

func parseRouteDestinationIntent(intent routeDestinationIntent) (production.RouteDestination, error) {
	hasLegacyPlanet := strings.TrimSpace(intent.DestinationPlanetID) != ""
	hasTypedDestination := strings.TrimSpace(intent.DestinationType) != "" || strings.TrimSpace(intent.DestinationID) != ""
	if hasLegacyPlanet && hasTypedDestination {
		return production.RouteDestination{}, invalidPayload("Route destination is ambiguous.", nil)
	}
	if !hasTypedDestination {
		destinationPlanetID, err := foundation.ParsePlanetID(intent.DestinationPlanetID)
		if err != nil {
			return production.RouteDestination{}, invalidPayload("Destination planet is invalid.", err)
		}
		destination, err := production.NewPlanetRouteDestination(destinationPlanetID)
		if err != nil {
			return production.RouteDestination{}, invalidPayload("Route destination is invalid.", err)
		}
		return destination, nil
	}

	destinationType := production.RouteDestinationType(strings.ToLower(strings.TrimSpace(intent.DestinationType)))
	destinationID := production.RouteDestinationID(intent.DestinationID)
	destination := production.RouteDestination{
		Type: destinationType,
		ID:   destinationID,
	}
	if destinationType == production.RouteDestinationTypePlanet {
		planetID, err := foundation.ParsePlanetID(destinationID.String())
		if err != nil {
			return production.RouteDestination{}, invalidPayload("Destination planet is invalid.", err)
		}
		return production.NewPlanetRouteDestination(planetID)
	}
	if err := destination.Validate(); err != nil {
		return production.RouteDestination{}, invalidPayload("Route destination is invalid.", err)
	}
	return destination, nil
}
