package premium

import "errors"

var (
	ErrNilWalletService = errors.New("nil wallet service")

	ErrEmptyEntitlementID        = errors.New("empty entitlement id")
	ErrInvalidEntitlementID      = errors.New("invalid entitlement id")
	ErrDuplicateEntitlementID    = errors.New("duplicate entitlement id")
	ErrEntitlementNotFound       = errors.New("entitlement not found")
	ErrEntitlementWrongPlayer    = errors.New("entitlement belongs to another player")
	ErrEntitlementAlreadyClaimed = errors.New("entitlement already claimed")
	ErrEntitlementNotPending     = errors.New("entitlement not pending")

	ErrInvalidEntitlementType  = errors.New("invalid entitlement type")
	ErrInvalidEntitlementState = errors.New("invalid entitlement state")
	ErrInvalidEntitlementGrant = errors.New("invalid entitlement grant")

	ErrEmptyProviderSource      = errors.New("empty provider source")
	ErrInvalidProviderSource    = errors.New("invalid provider source")
	ErrEmptyProviderReference   = errors.New("empty provider reference")
	ErrInvalidProviderReference = errors.New("invalid provider reference")

	ErrEmptyRequestReference    = errors.New("empty request reference")
	ErrInvalidRequestReference  = errors.New("invalid request reference")
	ErrEmptyPurchaseReference   = errors.New("empty purchase reference")
	ErrInvalidPurchaseReference = errors.New("invalid purchase reference")

	ErrInvalidTimestamp = errors.New("invalid timestamp")

	ErrEmptyPeriodKey     = errors.New("empty period key")
	ErrInvalidPeriodKey   = errors.New("invalid period key")
	ErrInvalidWeeklyStock = errors.New("invalid weekly stock")
	ErrWeeklyStockNotSet  = errors.New("weekly stock not set")
	ErrWeeklyStockSoldOut = errors.New("weekly stock sold out")
	ErrWeeklyLimitReached = errors.New("weekly limit reached")

	ErrEmptySuspiciousTradeReason      = errors.New("empty suspicious trade reason")
	ErrInvalidSuspiciousTradeReason    = errors.New("invalid suspicious trade reason")
	ErrEmptySuspiciousTradeReference   = errors.New("empty suspicious trade reference")
	ErrInvalidSuspiciousTradeReference = errors.New("invalid suspicious trade reference")

	ErrEmptyProviderRiskReason       = errors.New("empty provider risk reason")
	ErrInvalidProviderRiskReason     = errors.New("invalid provider risk reason")
	ErrEmptyProviderRiskReference    = errors.New("empty provider risk reference")
	ErrInvalidProviderRiskReference  = errors.New("invalid provider risk reference")
	ErrProviderRiskReferenceConflict = errors.New("provider risk reference conflict")
)
