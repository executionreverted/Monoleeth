package premium

import "gameproject/internal/game/economy"

// ValidatePaidPremiumUse reports whether a currency bucket may satisfy a
// paid-only premium use. Earned and market-acquired premium stay separate.
func ValidatePaidPremiumUse(currency economy.CurrencyBucket) error {
	return economy.ValidatePremiumEligibility(currency, economy.PremiumEligibilityPaidOnly)
}

// ValidatePremiumCurrencyListing reports whether a premium currency bucket may
// be listed or traded through future wallet-currency market flows.
func ValidatePremiumCurrencyListing(currency economy.CurrencyBucket) error {
	return ValidatePaidPremiumUse(currency)
}
