package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
)

type routeEndpointPayload struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Label string `json:"label"`
}

func (runtime *Runtime) routeEndpointPayloadsForOwner(playerID foundation.PlayerID) []routeEndpointPayload {
	return []routeEndpointPayload{
		runtimeRouteEndpointPayload(playerID, production.RouteDestinationTypeStorage),
		runtimeRouteEndpointPayload(playerID, production.RouteDestinationTypeStation),
	}
}

func runtimeRouteEndpointPayload(playerID foundation.PlayerID, destinationType production.RouteDestinationType) routeEndpointPayload {
	return routeEndpointPayload{
		Type:  destinationType.String(),
		ID:    runtimeRouteEndpointID(playerID, destinationType).String(),
		Label: runtimeRouteEndpointLabel(destinationType),
	}
}

func runtimeRouteEndpointID(playerID foundation.PlayerID, destinationType production.RouteDestinationType) production.RouteDestinationID {
	sum := sha256.Sum256([]byte("route-endpoint:" + destinationType.String() + ":" + playerID.String()))
	return production.RouteDestinationID(fmt.Sprintf("route-%s-%s", destinationType.String(), hex.EncodeToString(sum[:])[:16]))
}

func runtimeRouteEndpointLabel(destinationType production.RouteDestinationType) string {
	switch destinationType {
	case production.RouteDestinationTypeStorage:
		return "Storage"
	case production.RouteDestinationTypeStation:
		return "Station"
	default:
		return destinationType.String()
	}
}

func runtimeRouteDestinationMatchesPlayerEndpoint(playerID foundation.PlayerID, destination production.RouteDestination) bool {
	switch destination.Type {
	case production.RouteDestinationTypeStorage, production.RouteDestinationTypeStation:
		return destination.ID == runtimeRouteEndpointID(playerID, destination.Type)
	default:
		return false
	}
}

func (runtime *Runtime) ensurePlayerRouteEndpointStorage(playerID foundation.PlayerID, destination production.RouteDestination) error {
	if destination.Type == production.RouteDestinationTypePlanet {
		return nil
	}
	if !runtimeRouteDestinationMatchesPlayerEndpoint(playerID, destination) {
		return production.ErrRouteDestinationNotAccessible
	}
	storageID := foundation.PlanetID(destination.ID)
	if _, ok, err := runtime.Production.PlanetStorage(storageID); err != nil || ok {
		return err
	}
	storage, err := production.NewPlanetStorage(storageID, runtime.routeContent.EndpointStorageCapacityUnits, nil, runtime.clock.Now().UTC())
	if err != nil {
		return err
	}
	return runtime.Production.SavePlanetStorage(storage)
}
