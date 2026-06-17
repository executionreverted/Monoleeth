package economy

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestSupportedCurrencyBucketsValidateAndStringify(t *testing.T) {
	want := []CurrencyBucket{
		CurrencyBucketCredits,
		CurrencyBucketPremiumPaid,
		CurrencyBucketPremiumEarned,
		CurrencyBucketPremiumMarketAcquired,
		CurrencyBucketEventToken,
		CurrencyBucketReputationToken,
	}

	got := SupportedCurrencyBuckets()
	if len(got) != len(want) {
		t.Fatalf("SupportedCurrencyBuckets len = %d, want %d", len(got), len(want))
	}
	for i, currency := range want {
		if got[i] != currency {
			t.Fatalf("SupportedCurrencyBuckets[%d] = %q, want %q", i, got[i], currency)
		}
		if err := currency.Validate(); err != nil {
			t.Fatalf("%q Validate() = %v, want nil", currency, err)
		}
		if currency.String() != string(currency) {
			t.Fatalf("%q String() = %q, want %q", currency, currency.String(), string(currency))
		}
	}
}

func TestWalletBalanceValidatesOwnerCurrencyAndBalance(t *testing.T) {
	if _, err := NewWalletBalance("player-1", CurrencyBucketCredits, 0); err != nil {
		t.Fatalf("zero wallet balance error = %v, want nil", err)
	}
	if _, err := NewWalletBalance("", CurrencyBucketCredits, 0); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank player id error = %v, want foundation.ErrEmptyID", err)
	}
	if _, err := NewWalletBalance("player-1", CurrencyBucket("gold"), 0); !errors.Is(err, ErrInvalidCurrencyBucket) {
		t.Fatalf("invalid currency error = %v, want ErrInvalidCurrencyBucket", err)
	}
	if _, err := NewWalletBalance("player-1", CurrencyBucketCredits, -1); !errors.Is(err, ErrNegativeWalletBalance) {
		t.Fatalf("negative balance error = %v, want ErrNegativeWalletBalance", err)
	}
}

func TestWalletBalanceJSONBehaviorIsStable(t *testing.T) {
	wallet, err := NewWalletBalance("player-1", CurrencyBucketCredits, 0)
	if err != nil {
		t.Fatalf("NewWalletBalance valid value: %v", err)
	}
	wallet.UpdatedAt = time.Date(2026, 6, 17, 14, 0, 0, 0, time.UTC)

	payload, err := json.Marshal(wallet)
	if err != nil {
		t.Fatalf("json marshal wallet balance: %v", err)
	}
	want := `{"player_id":"player-1","currency_type":"credits","balance":0,"updated_at":"2026-06-17T14:00:00Z"}`
	if got := string(payload); got != want {
		t.Fatalf("wallet JSON = %s, want %s", got, want)
	}
}
