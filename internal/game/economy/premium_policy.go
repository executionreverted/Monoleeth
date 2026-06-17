package economy

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidPremiumEligibilityPolicy  = errors.New("invalid premium eligibility policy")
	ErrPaidPremiumRequired              = errors.New("paid premium required")
	ErrEarnedPremiumNotEligible         = errors.New("earned premium not eligible")
	ErrMarketAcquiredPremiumNotEligible = errors.New("market-acquired premium not eligible")
)

// PremiumEligibilityPolicy declares which premium buckets a premium use accepts.
type PremiumEligibilityPolicy string

const (
	PremiumEligibilityPaidOnly             PremiumEligibilityPolicy = "paid_only"
	PremiumEligibilityPaidOrMarketAcquired PremiumEligibilityPolicy = "paid_or_market_acquired"
)

// String returns the stable premium eligibility policy representation.
func (policy PremiumEligibilityPolicy) String() string {
	return string(policy)
}

// Validate reports whether policy is supported.
func (policy PremiumEligibilityPolicy) Validate() error {
	switch policy {
	case PremiumEligibilityPaidOnly,
		PremiumEligibilityPaidOrMarketAcquired:
		return nil
	default:
		return fmt.Errorf("premium eligibility policy %q: %w", policy, ErrInvalidPremiumEligibilityPolicy)
	}
}

// ValidatePremiumEligibility reports whether a currency bucket can satisfy a
// premium use policy. Market-acquired premium is intentionally separate from
// paid premium so later market rules must opt into it explicitly.
func ValidatePremiumEligibility(currency CurrencyBucket, policy PremiumEligibilityPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	switch currency {
	case CurrencyBucketPremiumPaid:
		return nil
	case CurrencyBucketPremiumEarned:
		return fmt.Errorf("currency bucket %q: %w", currency, ErrEarnedPremiumNotEligible)
	case CurrencyBucketPremiumMarketAcquired:
		if policy == PremiumEligibilityPaidOrMarketAcquired {
			return nil
		}
		return fmt.Errorf("currency bucket %q: %w", currency, ErrMarketAcquiredPremiumNotEligible)
	default:
		if err := currency.Validate(); err != nil {
			return err
		}
		return fmt.Errorf("currency bucket %q: %w", currency, ErrPaidPremiumRequired)
	}
}
