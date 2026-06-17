package ships

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
)

var (
	ErrInvalidShipState    = errors.New("invalid ship state")
	ErrInvalidMetadataJSON = errors.New("invalid ship metadata json")
)

// ShipState records the hangar state of one player-owned ship.
type ShipState string

const (
	ShipStateAvailable ShipState = "available"
	ShipStateActive    ShipState = "active"
	ShipStateDisabled  ShipState = "disabled"
	ShipStateRepairing ShipState = "repairing"
	ShipStateLocked    ShipState = "locked"
)

// PlayerShipState records a player's ownership row for one ship type.
type PlayerShipState struct {
	PlayerID       foundation.PlayerID `json:"player_id"`
	ShipID         foundation.ShipID   `json:"ship_id"`
	UnlockedAt     time.Time           `json:"unlocked_at"`
	State          ShipState           `json:"state"`
	DisabledReason string              `json:"disabled_reason,omitempty"`
	DisabledAt     *time.Time          `json:"disabled_at,omitempty"`
	LastRepairedAt *time.Time          `json:"last_repaired_at,omitempty"`
	MetadataJSON   json.RawMessage     `json:"metadata_json,omitempty"`
}

// ActiveShipState records the authoritative active ship pointer for a player.
type ActiveShipState struct {
	PlayerID    foundation.PlayerID `json:"player_id"`
	ShipID      foundation.ShipID   `json:"ship_id"`
	ActivatedAt time.Time           `json:"activated_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// NewPlayerShipState validates and returns player ship state.
func NewPlayerShipState(playerID foundation.PlayerID, shipID foundation.ShipID, state ShipState) (PlayerShipState, error) {
	playerShip := PlayerShipState{
		PlayerID: playerID,
		ShipID:   shipID,
		State:    state,
	}
	if err := playerShip.Validate(); err != nil {
		return PlayerShipState{}, err
	}
	return playerShip, nil
}

// NewActiveShipState validates and returns active ship state.
func NewActiveShipState(playerID foundation.PlayerID, shipID foundation.ShipID) (ActiveShipState, error) {
	activeShip := ActiveShipState{
		PlayerID: playerID,
		ShipID:   shipID,
	}
	if err := activeShip.Validate(); err != nil {
		return ActiveShipState{}, err
	}
	return activeShip, nil
}

// SupportedShipStates returns all supported player ship state values.
func SupportedShipStates() []ShipState {
	return []ShipState{
		ShipStateAvailable,
		ShipStateActive,
		ShipStateDisabled,
		ShipStateRepairing,
		ShipStateLocked,
	}
}

// String returns the stable state representation.
func (state ShipState) String() string {
	return string(state)
}

// Validate reports whether state is supported.
func (state ShipState) Validate() error {
	switch state {
	case ShipStateAvailable,
		ShipStateActive,
		ShipStateDisabled,
		ShipStateRepairing,
		ShipStateLocked:
		return nil
	default:
		return fmt.Errorf("ship state %q: %w", state, ErrInvalidShipState)
	}
}

// Validate reports whether playerShip has valid ids, state, and metadata JSON.
func (playerShip PlayerShipState) Validate() error {
	if err := playerShip.PlayerID.Validate(); err != nil {
		return err
	}
	if err := playerShip.ShipID.Validate(); err != nil {
		return err
	}
	if err := playerShip.State.Validate(); err != nil {
		return err
	}
	if err := validateRawJSON("metadata json", playerShip.MetadataJSON); err != nil {
		return err
	}
	return nil
}

// Validate reports whether activeShip has valid ids.
func (activeShip ActiveShipState) Validate() error {
	if err := activeShip.PlayerID.Validate(); err != nil {
		return err
	}
	if err := activeShip.ShipID.Validate(); err != nil {
		return err
	}
	return nil
}

func validateRawJSON(name string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if !json.Valid(raw) {
		return fmt.Errorf("%s: %w", name, ErrInvalidMetadataJSON)
	}
	return nil
}
