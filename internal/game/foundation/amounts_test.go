package foundation

import (
	"errors"
	"testing"
)

type positiveAmountCase struct {
	name     string
	newValue func(int64) (positiveAmount, error)
	parse    func(string) (positiveAmount, error)
	zero     func() positiveAmount
}

type positiveAmount interface {
	Int64() int64
	String() string
	Validate() error
	IsZero() bool
}

func TestValidatePositiveAmountRejectsZeroAndNegativeValues(t *testing.T) {
	for _, value := range []int64{0, -1, -500} {
		err := ValidatePositiveAmount(value)
		if !errors.Is(err, ErrNonPositiveAmount) {
			t.Fatalf("ValidatePositiveAmount(%d) error = %v, want ErrNonPositiveAmount", value, err)
		}
	}
}

func TestValidatePositiveAmountAcceptsPositiveValues(t *testing.T) {
	for _, value := range []int64{1, 42, 1_000_000} {
		if err := ValidatePositiveAmount(value); err != nil {
			t.Fatalf("ValidatePositiveAmount(%d) = %v, want nil", value, err)
		}
	}
}

func TestPositiveAmountTypesAcceptValidValuesAndConvertToString(t *testing.T) {
	for _, tc := range positiveAmountCases() {
		t.Run(tc.name, func(t *testing.T) {
			amount, err := tc.newValue(123)
			if err != nil {
				t.Fatalf("new positive amount: %v", err)
			}

			if got := amount.Int64(); got != 123 {
				t.Fatalf("Int64() = %d, want 123", got)
			}
			if got := amount.String(); got != "123" {
				t.Fatalf("String() = %q, want %q", got, "123")
			}
			if err := amount.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if amount.IsZero() {
				t.Fatal("IsZero() = true, want false")
			}
		})
	}
}

func TestPositiveAmountTypesParseValidStrings(t *testing.T) {
	for _, tc := range positiveAmountCases() {
		t.Run(tc.name, func(t *testing.T) {
			amount, err := tc.parse("987")
			if err != nil {
				t.Fatalf("parse positive amount: %v", err)
			}

			if got := amount.Int64(); got != 987 {
				t.Fatalf("Int64() = %d, want 987", got)
			}
			if got := amount.String(); got != "987" {
				t.Fatalf("String() = %q, want %q", got, "987")
			}
		})
	}
}

func TestPositiveAmountTypesRejectZeroAndNegativeValues(t *testing.T) {
	invalidValues := []int64{0, -1}

	for _, tc := range positiveAmountCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, value := range invalidValues {
				_, err := tc.newValue(value)
				if !errors.Is(err, ErrNonPositiveAmount) {
					t.Fatalf("new %d error = %v, want ErrNonPositiveAmount", value, err)
				}
			}
		})
	}
}

func TestPositiveAmountTypesRejectInvalidStrings(t *testing.T) {
	for _, tc := range positiveAmountCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, value := range []string{"", "one", "1.5"} {
				_, err := tc.parse(value)
				if !errors.Is(err, ErrInvalidAmount) {
					t.Fatalf("parse %q error = %v, want ErrInvalidAmount", value, err)
				}
			}
			for _, value := range []string{"0", "-1"} {
				_, err := tc.parse(value)
				if !errors.Is(err, ErrNonPositiveAmount) {
					t.Fatalf("parse %q error = %v, want ErrNonPositiveAmount", value, err)
				}
			}
		})
	}
}

func TestPositiveAmountZeroValuesAreInvalid(t *testing.T) {
	for _, tc := range positiveAmountCases() {
		t.Run(tc.name, func(t *testing.T) {
			amount := tc.zero()

			if !amount.IsZero() {
				t.Fatal("IsZero() = false, want true")
			}
			if err := amount.Validate(); !errors.Is(err, ErrNonPositiveAmount) {
				t.Fatalf("Validate() = %v, want ErrNonPositiveAmount", err)
			}
			if got := amount.Int64(); got != 0 {
				t.Fatalf("Int64() = %d, want 0", got)
			}
			if got := amount.String(); got != "0" {
				t.Fatalf("String() = %q, want %q", got, "0")
			}
		})
	}
}

func positiveAmountCases() []positiveAmountCase {
	return []positiveAmountCase{
		{
			name:     "Money",
			newValue: func(value int64) (positiveAmount, error) { return NewMoney(value) },
			parse:    func(value string) (positiveAmount, error) { return ParseMoney(value) },
			zero:     func() positiveAmount { return Money{} },
		},
		{
			name:     "Quantity",
			newValue: func(value int64) (positiveAmount, error) { return NewQuantity(value) },
			parse:    func(value string) (positiveAmount, error) { return ParseQuantity(value) },
			zero:     func() positiveAmount { return Quantity{} },
		},
	}
}
