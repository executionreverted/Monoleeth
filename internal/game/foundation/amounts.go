package foundation

import (
	"errors"
	"fmt"
	"strconv"
)

// ErrInvalidAmount reports an amount string that cannot be parsed as an int64.
var ErrInvalidAmount = errors.New("invalid amount")

// ErrNonPositiveAmount reports an amount that is zero or negative.
var ErrNonPositiveAmount = errors.New("non-positive amount")

// Money stores currency in whole minor units. It intentionally avoids floating point.
type Money struct {
	amount int64
}

// Quantity stores item or resource counts as whole units.
type Quantity struct {
	amount int64
}

// ValidatePositiveAmount reports whether amount is strictly greater than zero.
func ValidatePositiveAmount(amount int64) error {
	return validatePositiveAmount("amount", amount)
}

// NewMoney validates amount and returns Money.
func NewMoney(amount int64) (Money, error) {
	if err := validatePositiveAmount("money", amount); err != nil {
		return Money{}, err
	}
	return Money{amount: amount}, nil
}

// ParseMoney parses a base-10 int64 string and returns Money.
func ParseMoney(value string) (Money, error) {
	amount, err := parsePositiveAmount("money", value)
	if err != nil {
		return Money{}, err
	}
	return Money{amount: amount}, nil
}

// Int64 returns the underlying whole-unit amount.
func (m Money) Int64() int64 { return m.amount }

// String returns the base-10 representation of the amount.
func (m Money) String() string { return strconv.FormatInt(m.amount, 10) }

// MarshalJSON returns the stable whole-unit numeric representation.
func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(m.String()), nil
}

// Validate reports whether Money is strictly positive.
func (m Money) Validate() error { return validatePositiveAmount("money", m.amount) }

// IsZero reports whether Money is the zero value.
func (m Money) IsZero() bool { return m.amount == 0 }

// NewQuantity validates amount and returns Quantity.
func NewQuantity(amount int64) (Quantity, error) {
	if err := validatePositiveAmount("quantity", amount); err != nil {
		return Quantity{}, err
	}
	return Quantity{amount: amount}, nil
}

// ParseQuantity parses a base-10 int64 string and returns Quantity.
func ParseQuantity(value string) (Quantity, error) {
	amount, err := parsePositiveAmount("quantity", value)
	if err != nil {
		return Quantity{}, err
	}
	return Quantity{amount: amount}, nil
}

// Int64 returns the underlying whole-unit amount.
func (q Quantity) Int64() int64 { return q.amount }

// String returns the base-10 representation of the amount.
func (q Quantity) String() string { return strconv.FormatInt(q.amount, 10) }

// MarshalJSON returns the stable whole-unit numeric representation.
func (q Quantity) MarshalJSON() ([]byte, error) {
	return []byte(q.String()), nil
}

// Validate reports whether Quantity is strictly positive.
func (q Quantity) Validate() error { return validatePositiveAmount("quantity", q.amount) }

// IsZero reports whether Quantity is the zero value.
func (q Quantity) IsZero() bool { return q.amount == 0 }

func parsePositiveAmount(kind, value string) (int64, error) {
	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", kind, value, ErrInvalidAmount)
	}
	if err := validatePositiveAmount(kind, amount); err != nil {
		return 0, err
	}
	return amount, nil
}

func validatePositiveAmount(kind string, amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("%s must be positive: %w", kind, ErrNonPositiveAmount)
	}
	return nil
}
