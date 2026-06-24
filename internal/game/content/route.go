package content

import (
	"fmt"

	"gameproject/internal/game/foundation"
)

const (
	DefaultRouteCreateMaxRoutesPerPlayer       = 3
	DefaultRouteMaxDistanceUnits         int64 = 25_000
	DefaultRouteCrossMapPenaltyUnits     int64 = 1_000
	DefaultRouteEnergyBasePerHour        int64 = 1
	DefaultRouteEnergyAmountDivisor      int64 = 20
	DefaultRouteEndpointStorageUnits     int64 = 1_000
	DefaultRouteableResourceItemID             = foundation.ItemID("refined_alloy")
)

type RouteContent struct {
	RouteableItemIDs             []foundation.ItemID
	MaxRoutesPerPlayer           int
	MaxDistanceUnits             float64
	CrossMapDistancePenalty      float64
	EnergyBasePerHour            int64
	EnergyAmountDivisor          int64
	MinLossPercent               float64
	MaxLossPercent               float64
	EndpointStorageCapacityUnits int64
}

func DefaultRouteContent() RouteContent {
	return RouteContent{
		RouteableItemIDs: []foundation.ItemID{
			DefaultRouteableResourceItemID,
		},
		MaxRoutesPerPlayer:           DefaultRouteCreateMaxRoutesPerPlayer,
		MaxDistanceUnits:             float64(DefaultRouteMaxDistanceUnits),
		CrossMapDistancePenalty:      float64(DefaultRouteCrossMapPenaltyUnits),
		EnergyBasePerHour:            DefaultRouteEnergyBasePerHour,
		EnergyAmountDivisor:          DefaultRouteEnergyAmountDivisor,
		MinLossPercent:               0,
		MaxLossPercent:               0,
		EndpointStorageCapacityUnits: DefaultRouteEndpointStorageUnits,
	}
}

func (content RouteContent) Validate(bundle GameplayContent) error {
	if content.MaxRoutesPerPlayer <= 0 ||
		content.MaxDistanceUnits <= 0 ||
		content.CrossMapDistancePenalty < 0 ||
		content.EnergyBasePerHour < 0 ||
		content.EnergyAmountDivisor <= 0 ||
		content.EndpointStorageCapacityUnits <= 0 ||
		content.MinLossPercent < 0 ||
		content.MaxLossPercent < content.MinLossPercent {
		return fmt.Errorf("route content: %w", ErrInvalidRouteContent)
	}
	if len(content.RouteableItemIDs) == 0 {
		return fmt.Errorf("routeable items: %w", ErrInvalidRouteContent)
	}
	seen := make(map[foundation.ItemID]struct{}, len(content.RouteableItemIDs))
	for _, itemID := range content.RouteableItemIDs {
		if _, exists := seen[itemID]; exists {
			return fmt.Errorf("routeable item %q duplicate: %w", itemID, ErrInvalidRouteContent)
		}
		seen[itemID] = struct{}{}
		if err := validateKnownItem(bundle, "routeable resource", itemID); err != nil {
			return err
		}
	}
	return nil
}

func (content RouteContent) ResourceRouteable(itemID foundation.ItemID) bool {
	for _, routeableID := range content.RouteableItemIDs {
		if routeableID == itemID {
			return true
		}
	}
	return false
}

func (content RouteContent) EnergyCostPerHour(amountPerHour int64) int64 {
	return content.EnergyBasePerHour + amountPerHour/content.EnergyAmountDivisor
}
