package economy

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gameproject/internal/game/foundation"
)

var (
	ErrEmptyLedgerID       = errors.New("empty ledger id")
	ErrEmptyLedgerReason   = errors.New("empty ledger reason")
	ErrInvalidLedgerAction = errors.New("invalid ledger action")
	ErrNegativeBalance     = errors.New("negative ledger balance")
)

// LedgerID identifies one durable ledger row.
type LedgerID string

// LedgerReason records the business reason for an auditable value movement.
type LedgerReason string

// LedgerAction declares whether the entry increases or decreases the tracked value.
type LedgerAction string

const (
	LedgerActionIncrease LedgerAction = "increase"
	LedgerActionDecrease LedgerAction = "decrease"
)

// ItemLedgerEntry records an auditable item quantity movement.
type ItemLedgerEntry struct {
	LedgerID       LedgerID                  `json:"ledger_id"`
	PlayerID       foundation.PlayerID       `json:"player_id"`
	ItemID         foundation.ItemID         `json:"item_id"`
	ItemInstanceID foundation.ItemID         `json:"item_instance_id,omitempty"`
	Quantity       foundation.Quantity       `json:"quantity"`
	Action         LedgerAction              `json:"action"`
	BalanceAfter   int64                     `json:"balance_after"`
	Location       ItemLocation              `json:"location"`
	Reason         LedgerReason              `json:"reason"`
	ReferenceKey   foundation.IdempotencyKey `json:"reference_id"`
	CreatedAt      time.Time                 `json:"created_at"`
}

// CurrencyLedgerEntry records an auditable currency movement.
type CurrencyLedgerEntry struct {
	LedgerID     LedgerID                  `json:"ledger_id"`
	PlayerID     foundation.PlayerID       `json:"player_id"`
	Currency     CurrencyBucket            `json:"currency_type"`
	Amount       foundation.Money          `json:"amount"`
	Action       LedgerAction              `json:"action"`
	BalanceAfter int64                     `json:"balance_after"`
	Reason       LedgerReason              `json:"reason"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
	CreatedAt    time.Time                 `json:"created_at"`
}

// NewItemLedgerEntry validates and returns an item ledger entry.
func NewItemLedgerEntry(
	ledgerID LedgerID,
	playerID foundation.PlayerID,
	itemID foundation.ItemID,
	itemInstanceID foundation.ItemID,
	quantity foundation.Quantity,
	action LedgerAction,
	balanceAfter int64,
	location ItemLocation,
	reason LedgerReason,
	referenceKey foundation.IdempotencyKey,
) (ItemLedgerEntry, error) {
	entry := ItemLedgerEntry{
		LedgerID:       ledgerID,
		PlayerID:       playerID,
		ItemID:         itemID,
		ItemInstanceID: itemInstanceID,
		Quantity:       quantity,
		Action:         action,
		BalanceAfter:   balanceAfter,
		Location:       location,
		Reason:         reason,
		ReferenceKey:   referenceKey,
	}
	if err := entry.Validate(); err != nil {
		return ItemLedgerEntry{}, err
	}
	return entry, nil
}

// NewCurrencyLedgerEntry validates and returns a currency ledger entry.
func NewCurrencyLedgerEntry(
	ledgerID LedgerID,
	playerID foundation.PlayerID,
	currency CurrencyBucket,
	amount foundation.Money,
	action LedgerAction,
	balanceAfter int64,
	reason LedgerReason,
	referenceKey foundation.IdempotencyKey,
) (CurrencyLedgerEntry, error) {
	entry := CurrencyLedgerEntry{
		LedgerID:     ledgerID,
		PlayerID:     playerID,
		Currency:     currency,
		Amount:       amount,
		Action:       action,
		BalanceAfter: balanceAfter,
		Reason:       reason,
		ReferenceKey: referenceKey,
	}
	if err := entry.Validate(); err != nil {
		return CurrencyLedgerEntry{}, err
	}
	return entry, nil
}

// String returns the stable ledger id representation.
func (id LedgerID) String() string {
	return string(id)
}

// Validate reports whether id is non-blank.
func (id LedgerID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrEmptyLedgerID
	}
	return nil
}

// IsZero reports whether id is the zero value.
func (id LedgerID) IsZero() bool {
	return id == ""
}

// String returns the stable ledger reason representation.
func (reason LedgerReason) String() string {
	return string(reason)
}

// Validate reports whether reason is non-blank.
func (reason LedgerReason) Validate() error {
	if strings.TrimSpace(string(reason)) == "" {
		return ErrEmptyLedgerReason
	}
	return nil
}

// IsZero reports whether reason is the zero value.
func (reason LedgerReason) IsZero() bool {
	return reason == ""
}

// String returns the stable ledger action representation.
func (action LedgerAction) String() string {
	return string(action)
}

// Validate reports whether action is supported.
func (action LedgerAction) Validate() error {
	switch action {
	case LedgerActionIncrease, LedgerActionDecrease:
		return nil
	default:
		return fmt.Errorf("ledger action %q: %w", action, ErrInvalidLedgerAction)
	}
}

// IsZero reports whether action is the zero value.
func (action LedgerAction) IsZero() bool {
	return action == ""
}

// Validate reports whether entry has valid ids, amount, location, reason, and reference.
func (entry ItemLedgerEntry) Validate() error {
	if err := entry.LedgerID.Validate(); err != nil {
		return err
	}
	if err := entry.PlayerID.Validate(); err != nil {
		return err
	}
	if err := entry.ItemID.Validate(); err != nil {
		return err
	}
	if !entry.ItemInstanceID.IsZero() {
		if err := entry.ItemInstanceID.Validate(); err != nil {
			return err
		}
	}
	if err := entry.Quantity.Validate(); err != nil {
		return err
	}
	if err := entry.Action.Validate(); err != nil {
		return err
	}
	if err := validateLedgerBalanceAfter(entry.BalanceAfter); err != nil {
		return err
	}
	if err := entry.Location.Validate(); err != nil {
		return err
	}
	if err := entry.Reason.Validate(); err != nil {
		return err
	}
	if err := entry.ReferenceKey.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether entry has valid ids, amount, reason, and reference.
func (entry CurrencyLedgerEntry) Validate() error {
	if err := entry.LedgerID.Validate(); err != nil {
		return err
	}
	if err := entry.PlayerID.Validate(); err != nil {
		return err
	}
	if err := entry.Currency.Validate(); err != nil {
		return err
	}
	if err := entry.Amount.Validate(); err != nil {
		return err
	}
	if err := entry.Action.Validate(); err != nil {
		return err
	}
	if err := validateLedgerBalanceAfter(entry.BalanceAfter); err != nil {
		return err
	}
	if err := entry.Reason.Validate(); err != nil {
		return err
	}
	if err := entry.ReferenceKey.Validate(); err != nil {
		return err
	}
	return nil
}

func validateLedgerBalanceAfter(balanceAfter int64) error {
	if balanceAfter < 0 {
		return fmt.Errorf("balance after %d: %w", balanceAfter, ErrNegativeBalance)
	}
	return nil
}
