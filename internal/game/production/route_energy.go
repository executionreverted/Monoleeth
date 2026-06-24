package production

import (
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
)

func (store *InMemoryStore) prepareRouteEnergyReservationLocked(
	sourcePlanetID foundation.PlanetID,
	oldEnergyCostPerHour int64,
	newEnergyCostPerHour int64,
	now time.Time,
) (PlanetProductionState, bool, error) {
	if oldEnergyCostPerHour < 0 || newEnergyCostPerHour < 0 {
		return PlanetProductionState{}, false, ErrInvalidRouteEnergyCost
	}
	before, ok := store.snapshotLocked(sourcePlanetID)
	if !ok {
		if oldEnergyCostPerHour == 0 && newEnergyCostPerHour == 0 {
			return PlanetProductionState{}, false, nil
		}
		return PlanetProductionState{}, false, fmt.Errorf("source planet %q: %w", sourcePlanetID, ErrRouteSourceProductionMissing)
	}
	if err := before.Validate(); err != nil {
		return PlanetProductionState{}, false, err
	}
	state := before.State
	if oldEnergyCostPerHour == newEnergyCostPerHour {
		if state.EnergyReservedPerHour < oldEnergyCostPerHour {
			return PlanetProductionState{}, false, fmt.Errorf("reserved %d existing route %d: %w", state.EnergyReservedPerHour, oldEnergyCostPerHour, ErrInvalidEnergyRate)
		}
		return PlanetProductionState{}, false, nil
	}

	updated := cloneProductionState(state)
	availableReserved := updated.EnergyReservedPerHour - oldEnergyCostPerHour
	if availableReserved < 0 {
		return PlanetProductionState{}, false, fmt.Errorf("reserved %d release %d: %w", updated.EnergyReservedPerHour, oldEnergyCostPerHour, ErrInvalidEnergyRate)
	}
	if availableReserved+newEnergyCostPerHour > updated.EnergyCapacityPerHour {
		return PlanetProductionState{}, false, fmt.Errorf("reserved %d route %d capacity %d: %w", availableReserved, newEnergyCostPerHour, updated.EnergyCapacityPerHour, ErrRouteEnergyUnavailable)
	}

	catalogRows, err := store.catalogLocked()
	if err != nil {
		return PlanetProductionState{}, false, err
	}
	if _, err := store.settlePlanetProductionLocked(sourcePlanetID, now.UTC(), catalogRows, false); err != nil {
		return PlanetProductionState{}, false, err
	}
	state, ok = store.states[sourcePlanetID]
	if !ok {
		return PlanetProductionState{}, false, fmt.Errorf("source planet %q: %w", sourcePlanetID, ErrRouteSourceProductionMissing)
	}
	updated = cloneProductionState(state)
	availableReserved = updated.EnergyReservedPerHour - oldEnergyCostPerHour
	if availableReserved < 0 {
		return PlanetProductionState{}, false, fmt.Errorf("reserved %d release %d: %w", updated.EnergyReservedPerHour, oldEnergyCostPerHour, ErrInvalidEnergyRate)
	}
	updated.EnergyReservedPerHour = availableReserved + newEnergyCostPerHour
	updated.UpdatedAt = now.UTC()
	if err := updated.Validate(); err != nil {
		return PlanetProductionState{}, false, err
	}
	return cloneProductionState(updated), true, nil
}

func routeReservedEnergyCost(route AutomationRoute) int64 {
	if !route.Enabled {
		return 0
	}
	return route.EnergyCostPerHour
}

func optionalRouteSourceProductionState(state PlanetProductionState, ok bool) *PlanetProductionState {
	if !ok {
		return nil
	}
	return cloneProductionStatePointer(&state)
}
