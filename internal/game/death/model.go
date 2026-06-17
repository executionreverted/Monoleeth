package death

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

const lethalEventKeyOperation = "player_death"

var (
	ErrInvalidDeathReason     = errors.New("invalid death reason")
	ErrEmptyRespawnLocationID = errors.New("empty respawn location id")
	ErrEmptyLethalEventKey    = errors.New("empty lethal event key")
	ErrInvalidLethalEventKey  = errors.New("invalid lethal event key")
	ErrInvalidDeathRecord     = errors.New("invalid death record")
)

// DeathReason records why a player death was processed.
type DeathReason string

const (
	DeathReasonCombat      DeathReason = "combat"
	DeathReasonEnvironment DeathReason = "environment"
	DeathReasonSystem      DeathReason = "system"
)

// RespawnLocationID identifies the selected respawn target for a death record.
type RespawnLocationID string

// LethalEventIdempotencyKey is the death-domain idempotency key derived from
// one lethal combat/world event. It is stable across request retries and
// duplicate event delivery.
type LethalEventIdempotencyKey string

// LethalEventKey is kept as a shorter alias for record fields.
type LethalEventKey = LethalEventIdempotencyKey

// DeathRecord is the durable death state inserted after the death transaction
// has selected cargo loss and before post-commit broadcasts.
type DeathRecord struct {
	DeathID           foundation.EventID  `json:"death_id"`
	LethalEventKey    LethalEventKey      `json:"lethal_event_key"`
	PlayerID          foundation.PlayerID `json:"player_id"`
	WorldID           world.WorldID       `json:"world_id"`
	ZoneID            world.ZoneID        `json:"zone_id"`
	Position          world.Vec2          `json:"position"`
	KillerEntityID    world.EntityID      `json:"killer_entity_id,omitempty"`
	Reason            DeathReason         `json:"death_reason"`
	CargoDropPercent  float64             `json:"cargo_drop_percent"`
	ActiveShipID      foundation.ShipID   `json:"active_ship_id"`
	RespawnLocationID RespawnLocationID   `json:"respawn_location_id"`
	CreatedAt         time.Time           `json:"created_at"`
}

// NewLethalEventIdempotencyKey returns player_death:<lethal_event_id>.
func NewLethalEventIdempotencyKey(lethalEventID foundation.EventID) (LethalEventIdempotencyKey, error) {
	if err := lethalEventID.Validate(); err != nil {
		return "", err
	}
	return LethalEventIdempotencyKey(lethalEventKeyOperation + ":" + lethalEventID.String()), nil
}

// ParseLethalEventIdempotencyKey validates value and returns a key.
func ParseLethalEventIdempotencyKey(value string) (LethalEventIdempotencyKey, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("lethal event idempotency key: %w", ErrEmptyLethalEventKey)
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("lethal event idempotency key %q: %w", value, ErrInvalidLethalEventKey)
	}
	if parts[0] != lethalEventKeyOperation {
		return "", fmt.Errorf("lethal event idempotency operation %q: %w", parts[0], ErrInvalidLethalEventKey)
	}
	if _, err := foundation.ParseEventID(parts[1]); err != nil {
		return "", err
	}
	return LethalEventIdempotencyKey(value), nil
}

// NewLethalEventKey returns player_death:<lethal_event_id>.
func NewLethalEventKey(lethalEventID foundation.EventID) (LethalEventKey, error) {
	return NewLethalEventIdempotencyKey(lethalEventID)
}

// ParseLethalEventKey validates value and returns a LethalEventKey.
func ParseLethalEventKey(value string) (LethalEventKey, error) {
	return ParseLethalEventIdempotencyKey(value)
}

// String returns the stable key representation.
func (key LethalEventIdempotencyKey) String() string {
	return string(key)
}

// Validate reports whether key has the player_death:<lethal_event_id> shape.
func (key LethalEventIdempotencyKey) Validate() error {
	_, err := ParseLethalEventIdempotencyKey(string(key))
	return err
}

// EventID returns the lethal source event id encoded in the key.
func (key LethalEventIdempotencyKey) EventID() (foundation.EventID, error) {
	if err := key.Validate(); err != nil {
		return "", err
	}
	parts := strings.Split(string(key), ":")
	return foundation.ParseEventID(parts[1])
}

// IsZero reports whether key is the zero value.
func (key LethalEventIdempotencyKey) IsZero() bool {
	return key == ""
}

// String returns the stable reason representation.
func (reason DeathReason) String() string {
	return string(reason)
}

// Validate reports whether reason is supported.
func (reason DeathReason) Validate() error {
	switch reason {
	case DeathReasonCombat, DeathReasonEnvironment, DeathReasonSystem:
		return nil
	default:
		return fmt.Errorf("death reason %q: %w", reason, ErrInvalidDeathReason)
	}
}

// String returns the stable respawn location representation.
func (id RespawnLocationID) String() string {
	return string(id)
}

// Validate reports whether id is non-blank.
func (id RespawnLocationID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrEmptyRespawnLocationID
	}
	return nil
}

// IsZero reports whether id is the zero value.
func (id RespawnLocationID) IsZero() bool {
	return id == ""
}

// Validate reports whether record contains a complete death record shape.
func (record DeathRecord) Validate() error {
	if err := record.DeathID.Validate(); err != nil {
		return err
	}
	if err := record.LethalEventKey.Validate(); err != nil {
		return err
	}
	if err := record.PlayerID.Validate(); err != nil {
		return err
	}
	if err := record.WorldID.Validate(); err != nil {
		return err
	}
	if err := record.ZoneID.Validate(); err != nil {
		return err
	}
	if err := record.Position.Validate(); err != nil {
		return err
	}
	if !record.KillerEntityID.IsZero() {
		if err := record.KillerEntityID.Validate(); err != nil {
			return err
		}
	}
	if err := record.Reason.Validate(); err != nil {
		return err
	}
	if err := validateDropPercent("cargo drop percent", record.CargoDropPercent); err != nil {
		return err
	}
	if err := record.ActiveShipID.Validate(); err != nil {
		return err
	}
	if err := record.RespawnLocationID.Validate(); err != nil {
		return err
	}
	if record.CreatedAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrInvalidDeathRecord)
	}
	return nil
}
