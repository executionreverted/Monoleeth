package economy

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gameproject/internal/game/foundation"
)

var (
	ErrEmptyReservationID      = errors.New("empty reservation id")
	ErrInvalidReservationKind  = errors.New("invalid reservation kind")
	ErrInvalidReservationState = errors.New("invalid reservation state")
	ErrEmptyReservationAssets  = errors.New("empty reservation assets")
)

// ReservationID identifies one item or currency reservation.
type ReservationID string

// ReservationKind identifies the owning system for a reservation.
type ReservationKind string

const (
	ReservationKindCraft   ReservationKind = "craft"
	ReservationKindMarket  ReservationKind = "market"
	ReservationKindAuction ReservationKind = "auction"
)

// ReservationState identifies the lifecycle state of a reservation row.
type ReservationState string

const (
	ReservationStateActive    ReservationState = "active"
	ReservationStateReleased  ReservationState = "released"
	ReservationStateCommitted ReservationState = "committed"
	ReservationStateExpired   ReservationState = "expired"
)

// ReservationItemLine models one item quantity held by a reservation.
type ReservationItemLine struct {
	ItemID           foundation.ItemID   `json:"item_id"`
	ItemInstanceID   foundation.ItemID   `json:"item_instance_id,omitempty"`
	Quantity         foundation.Quantity `json:"quantity"`
	FromLocation     ItemLocation        `json:"from_location"`
	ReservedLocation ItemLocation        `json:"reserved_location"`
}

// ReservationCurrencyLine models one currency amount held by a reservation.
type ReservationCurrencyLine struct {
	Currency CurrencyBucket   `json:"currency_type"`
	Amount   foundation.Money `json:"amount"`
}

// Reservation records assets reserved for craft, market, or auction flows.
type Reservation struct {
	ReservationID ReservationID             `json:"reservation_id"`
	Kind          ReservationKind           `json:"reservation_kind"`
	State         ReservationState          `json:"state"`
	PlayerID      foundation.PlayerID       `json:"player_id"`
	ReferenceKey  foundation.IdempotencyKey `json:"reference_id"`
	ItemLines     []ReservationItemLine     `json:"item_lines,omitempty"`
	CurrencyLines []ReservationCurrencyLine `json:"currency_lines,omitempty"`
	CreatedAt     time.Time                 `json:"created_at"`
	ExpiresAt     *time.Time                `json:"expires_at,omitempty"`
}

// SupportedReservationKinds returns all Phase 02-supported reservation kinds.
func SupportedReservationKinds() []ReservationKind {
	return []ReservationKind{
		ReservationKindCraft,
		ReservationKindMarket,
		ReservationKindAuction,
	}
}

// NewReservation validates and returns a reservation model.
func NewReservation(
	reservationID ReservationID,
	kind ReservationKind,
	playerID foundation.PlayerID,
	referenceKey foundation.IdempotencyKey,
	itemLines []ReservationItemLine,
	currencyLines []ReservationCurrencyLine,
) (Reservation, error) {
	reservation := Reservation{
		ReservationID: reservationID,
		Kind:          kind,
		State:         ReservationStateActive,
		PlayerID:      playerID,
		ReferenceKey:  referenceKey,
		ItemLines:     append([]ReservationItemLine(nil), itemLines...),
		CurrencyLines: append([]ReservationCurrencyLine(nil), currencyLines...),
	}
	if err := reservation.Validate(); err != nil {
		return Reservation{}, err
	}
	return reservation, nil
}

// String returns the stable reservation id representation.
func (id ReservationID) String() string {
	return string(id)
}

// Validate reports whether id is non-blank.
func (id ReservationID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrEmptyReservationID
	}
	return nil
}

// IsZero reports whether id is the zero value.
func (id ReservationID) IsZero() bool {
	return id == ""
}

// String returns the stable reservation kind representation.
func (kind ReservationKind) String() string {
	return string(kind)
}

// Validate reports whether kind is supported.
func (kind ReservationKind) Validate() error {
	switch kind {
	case ReservationKindCraft, ReservationKindMarket, ReservationKindAuction:
		return nil
	default:
		return fmt.Errorf("reservation kind %q: %w", kind, ErrInvalidReservationKind)
	}
}

// ReservedLocationKind returns the default reserved location kind for kind.
func (kind ReservationKind) ReservedLocationKind() (LocationKind, error) {
	if err := kind.Validate(); err != nil {
		return "", err
	}
	switch kind {
	case ReservationKindCraft:
		return LocationKindCraftingReserved, nil
	case ReservationKindMarket:
		return LocationKindMarketEscrow, nil
	case ReservationKindAuction:
		return LocationKindAuctionEscrow, nil
	default:
		return "", fmt.Errorf("reservation kind %q: %w", kind, ErrInvalidReservationKind)
	}
}

// IsZero reports whether kind is the zero value.
func (kind ReservationKind) IsZero() bool {
	return kind == ""
}

// String returns the stable reservation state representation.
func (state ReservationState) String() string {
	return string(state)
}

// Validate reports whether state is supported.
func (state ReservationState) Validate() error {
	switch state {
	case ReservationStateActive,
		ReservationStateReleased,
		ReservationStateCommitted,
		ReservationStateExpired:
		return nil
	default:
		return fmt.Errorf("reservation state %q: %w", state, ErrInvalidReservationState)
	}
}

// IsZero reports whether state is the zero value.
func (state ReservationState) IsZero() bool {
	return state == ""
}

// Validate reports whether line has valid item ids, quantity, and locations.
func (line ReservationItemLine) Validate() error {
	if err := line.ItemID.Validate(); err != nil {
		return err
	}
	if !line.ItemInstanceID.IsZero() {
		if err := line.ItemInstanceID.Validate(); err != nil {
			return err
		}
	}
	if err := line.Quantity.Validate(); err != nil {
		return err
	}
	if err := line.FromLocation.Validate(); err != nil {
		return err
	}
	if err := line.ReservedLocation.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether line has a valid currency and positive amount.
func (line ReservationCurrencyLine) Validate() error {
	if err := line.Currency.Validate(); err != nil {
		return err
	}
	if err := line.Amount.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether reservation has valid identity, ownership, reference, and assets.
func (reservation Reservation) Validate() error {
	if err := reservation.ReservationID.Validate(); err != nil {
		return err
	}
	if err := reservation.Kind.Validate(); err != nil {
		return err
	}
	if err := reservation.State.Validate(); err != nil {
		return err
	}
	if err := reservation.PlayerID.Validate(); err != nil {
		return err
	}
	if err := reservation.ReferenceKey.Validate(); err != nil {
		return err
	}
	if len(reservation.ItemLines) == 0 && len(reservation.CurrencyLines) == 0 {
		return ErrEmptyReservationAssets
	}
	for _, line := range reservation.ItemLines {
		if err := line.Validate(); err != nil {
			return err
		}
	}
	for _, line := range reservation.CurrencyLines {
		if err := line.Validate(); err != nil {
			return err
		}
	}
	return nil
}
