package economy

import (
	"errors"
	"testing"
)

func TestPremiumEligibilityPolicyValidatesAndStringifies(t *testing.T) {
	policies := []PremiumEligibilityPolicy{
		PremiumEligibilityPaidOnly,
		PremiumEligibilityPaidOrMarketAcquired,
	}

	for _, policy := range policies {
		if err := policy.Validate(); err != nil {
			t.Fatalf("%q Validate() = %v, want nil", policy, err)
		}
		if policy.String() != string(policy) {
			t.Fatalf("%q String() = %q, want %q", policy, policy.String(), string(policy))
		}
	}

	if err := PremiumEligibilityPolicy("earned_allowed").Validate(); !errors.Is(err, ErrInvalidPremiumEligibilityPolicy) {
		t.Fatalf("invalid policy error = %v, want ErrInvalidPremiumEligibilityPolicy", err)
	}
}

func TestPremiumEligibilityPaidOnlyAllowsPaidAndRejectsEarned(t *testing.T) {
	if err := ValidatePremiumEligibility(CurrencyBucketPremiumPaid, PremiumEligibilityPaidOnly); err != nil {
		t.Fatalf("paid premium eligibility error = %v, want nil", err)
	}

	err := ValidatePremiumEligibility(CurrencyBucketPremiumEarned, PremiumEligibilityPaidOnly)
	if !errors.Is(err, ErrEarnedPremiumNotEligible) {
		t.Fatalf("earned premium paid-only error = %v, want ErrEarnedPremiumNotEligible", err)
	}
}

func TestPremiumEligibilityHandlesMarketAcquiredExplicitly(t *testing.T) {
	err := ValidatePremiumEligibility(CurrencyBucketPremiumMarketAcquired, PremiumEligibilityPaidOnly)
	if !errors.Is(err, ErrMarketAcquiredPremiumNotEligible) {
		t.Fatalf("market-acquired paid-only error = %v, want ErrMarketAcquiredPremiumNotEligible", err)
	}

	if err := ValidatePremiumEligibility(CurrencyBucketPremiumMarketAcquired, PremiumEligibilityPaidOrMarketAcquired); err != nil {
		t.Fatalf("market-acquired explicit eligibility error = %v, want nil", err)
	}
}

func TestPremiumEligibilityRejectsNonPremiumAndInvalidBuckets(t *testing.T) {
	err := ValidatePremiumEligibility(CurrencyBucketCredits, PremiumEligibilityPaidOnly)
	if !errors.Is(err, ErrPaidPremiumRequired) {
		t.Fatalf("credits paid-only error = %v, want ErrPaidPremiumRequired", err)
	}

	err = ValidatePremiumEligibility(CurrencyBucket("gold"), PremiumEligibilityPaidOnly)
	if !errors.Is(err, ErrInvalidCurrencyBucket) {
		t.Fatalf("invalid currency error = %v, want ErrInvalidCurrencyBucket", err)
	}
}
