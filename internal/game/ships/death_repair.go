package ships

import (
	"fmt"

	"gameproject/internal/game/foundation"
)

const (
	// DisabledReasonDeath records that the ship was disabled by lethal combat.
	DisabledReasonDeath = "death"
)

// DisableActiveShipForDeathInput describes a server-authoritative death disable.
type DisableActiveShipForDeathInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
}

// DisableActiveShipForDeathResult reports whether the active ship was disabled.
type DisableActiveShipForDeathResult struct {
	PlayerShip       PlayerShipState         `json:"player_ship"`
	ActiveShip       ActiveShipState         `json:"active_ship"`
	Disabled         bool                    `json:"disabled"`
	Duplicate        bool                    `json:"duplicate"`
	StatInvalidation *StatInvalidationSignal `json:"stat_invalidation,omitempty"`
}

// RepairShipInput describes a wallet-free ship repair state transition.
type RepairShipInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	ShipID   foundation.ShipID   `json:"ship_id"`
}

// RepairShipResult reports a wallet-free ship repair state transition.
type RepairShipResult struct {
	PlayerShip       PlayerShipState         `json:"player_ship"`
	Repaired         bool                    `json:"repaired"`
	StatInvalidation *StatInvalidationSignal `json:"stat_invalidation,omitempty"`
}

// DisableActiveShipForDeath marks the player's active ship disabled. Duplicate
// death handling in the death service can call this safely after the first
// disable because an already-disabled active ship is returned as a no-op.
func (service *HangarService) DisableActiveShipForDeath(input DisableActiveShipForDeathInput) (DisableActiveShipForDeathResult, error) {
	if err := input.validate(); err != nil {
		return DisableActiveShipForDeathResult{}, err
	}

	now := service.clock.Now()
	var result DisableActiveShipForDeathResult
	err := service.store.updatePlayerHangar(input.PlayerID, func(record *hangarRecord) error {
		activeShip, hasActive := record.activeShip()
		if !hasActive {
			return ErrNoActiveShip
		}
		playerShip, ok := record.ship(activeShip.ShipID)
		if !ok {
			return fmt.Errorf("active ship %q: %w", activeShip.ShipID, ErrShipNotUnlocked)
		}
		if playerShip.State == ShipStateDisabled {
			result = DisableActiveShipForDeathResult{
				PlayerShip: playerShip,
				ActiveShip: activeShip,
				Duplicate:  true,
			}
			return nil
		}
		if !isUsableShipState(playerShip.State) {
			return fmt.Errorf("active ship state %q: %w", playerShip.State, ErrShipUnavailable)
		}

		disabledAt := now
		playerShip.State = ShipStateDisabled
		playerShip.DisabledReason = DisabledReasonDeath
		playerShip.DisabledAt = &disabledAt
		record.putShip(playerShip)

		activeShip.UpdatedAt = now
		record.putActiveShip(activeShip)

		result = DisableActiveShipForDeathResult{
			PlayerShip: playerShip,
			ActiveShip: activeShip,
			Disabled:   true,
			StatInvalidation: newStatInvalidationSignalWithReason(
				input.PlayerID,
				"",
				activeShip.ShipID,
				StatInvalidationReasonActiveShipStateChanged,
				now,
			),
		}
		return nil
	})
	if err != nil {
		return DisableActiveShipForDeathResult{}, err
	}
	return result, nil
}

// RepairShip restores a disabled ship without debiting wallet state. The Phase
// 06 RepairService will own payment before calling this primitive.
func (service *HangarService) RepairShip(input RepairShipInput) (RepairShipResult, error) {
	if err := input.validate(); err != nil {
		return RepairShipResult{}, err
	}
	if _, err := service.catalog.MustGet(input.ShipID); err != nil {
		return RepairShipResult{}, err
	}

	now := service.clock.Now()
	var result RepairShipResult
	err := service.store.updatePlayerHangar(input.PlayerID, func(record *hangarRecord) error {
		playerShip, ok := record.ship(input.ShipID)
		if !ok {
			return fmt.Errorf("ship %q: %w", input.ShipID, ErrShipNotUnlocked)
		}
		if playerShip.State != ShipStateDisabled {
			return ErrShipNotDisabled
		}

		activeShip, isActiveShip := record.activeShip()
		isActiveShip = isActiveShip && activeShip.ShipID == input.ShipID

		repairedAt := now
		playerShip.State = ShipStateAvailable
		if isActiveShip {
			playerShip.State = ShipStateActive
			activeShip.UpdatedAt = now
			record.putActiveShip(activeShip)
		}
		playerShip.DisabledReason = ""
		playerShip.DisabledAt = nil
		playerShip.LastRepairedAt = &repairedAt
		record.putShip(playerShip)

		result = RepairShipResult{
			PlayerShip: playerShip,
			Repaired:   true,
		}
		if isActiveShip {
			result.StatInvalidation = newStatInvalidationSignalWithReason(
				input.PlayerID,
				"",
				input.ShipID,
				StatInvalidationReasonActiveShipStateChanged,
				now,
			)
		}
		return nil
	})
	if err != nil {
		return RepairShipResult{}, err
	}
	return result, nil
}

func (input DisableActiveShipForDeathInput) validate() error {
	return input.PlayerID.Validate()
}

func (input RepairShipInput) validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	return input.ShipID.Validate()
}
