package economy

import (
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
)

var (
	ErrInvalidCurrencyBucket = errors.New("invalid currency bucket")
	ErrNegativeWalletBalance = errors.New("negative wallet balance")
)

// CurrencyBucket identifies a wallet balance bucket.
type CurrencyBucket string

const (
	CurrencyBucketCredits               CurrencyBucket = "credits"
	CurrencyBucketPremiumPaid           CurrencyBucket = "premium_paid"
	CurrencyBucketPremiumEarned         CurrencyBucket = "premium_earned"
	CurrencyBucketPremiumMarketAcquired CurrencyBucket = "premium_market_acquired"
	CurrencyBucketEventToken            CurrencyBucket = "event_token"
	CurrencyBucketReputationToken       CurrencyBucket = "reputation_token"
)

// WalletBalance models a player's balance for one currency bucket.
//
// Balances may be zero. State-changing amounts use foundation.Money and must be
// strictly positive.
type WalletBalance struct {
	PlayerID  foundation.PlayerID `json:"player_id"`
	Currency  CurrencyBucket      `json:"currency_type"`
	Balance   int64               `json:"balance"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// SupportedCurrencyBuckets returns all roadmap-supported currency buckets.
func SupportedCurrencyBuckets() []CurrencyBucket {
	return []CurrencyBucket{
		CurrencyBucketCredits,
		CurrencyBucketPremiumPaid,
		CurrencyBucketPremiumEarned,
		CurrencyBucketPremiumMarketAcquired,
		CurrencyBucketEventToken,
		CurrencyBucketReputationToken,
	}
}

// NewWalletBalance validates and returns a wallet balance model.
func NewWalletBalance(playerID foundation.PlayerID, currency CurrencyBucket, balance int64) (WalletBalance, error) {
	wallet := WalletBalance{
		PlayerID: playerID,
		Currency: currency,
		Balance:  balance,
	}
	if err := wallet.Validate(); err != nil {
		return WalletBalance{}, err
	}
	return wallet, nil
}

// String returns the stable currency bucket representation.
func (currency CurrencyBucket) String() string {
	return string(currency)
}

// Validate reports whether currency is supported.
func (currency CurrencyBucket) Validate() error {
	switch currency {
	case CurrencyBucketCredits,
		CurrencyBucketPremiumPaid,
		CurrencyBucketPremiumEarned,
		CurrencyBucketPremiumMarketAcquired,
		CurrencyBucketEventToken,
		CurrencyBucketReputationToken:
		return nil
	default:
		return fmt.Errorf("currency bucket %q: %w", currency, ErrInvalidCurrencyBucket)
	}
}

// IsZero reports whether currency is the zero value.
func (currency CurrencyBucket) IsZero() bool {
	return currency == ""
}

// Validate reports whether wallet has a valid owner, currency, and balance.
func (wallet WalletBalance) Validate() error {
	if err := wallet.PlayerID.Validate(); err != nil {
		return err
	}
	if err := wallet.Currency.Validate(); err != nil {
		return err
	}
	if wallet.Balance < 0 {
		return fmt.Errorf("wallet balance %d: %w", wallet.Balance, ErrNegativeWalletBalance)
	}
	return nil
}
