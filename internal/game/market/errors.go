package market

import "errors"

var (
	ErrMissingInventoryService        = errors.New("missing inventory service")
	ErrMissingWalletService           = errors.New("missing wallet service")
	ErrInvalidFeePolicy               = errors.New("invalid market fee policy")
	ErrInvalidListingStatus           = errors.New("invalid market listing status")
	ErrInvalidListingTransition       = errors.New("invalid market listing transition")
	ErrDuplicateListingID             = errors.New("duplicate market listing id")
	ErrCreateListingReferenceMismatch = errors.New("market create listing reference mismatch")
	ErrListingNotFound                = errors.New("market listing not found")
	ErrListingNotActive               = errors.New("market listing is not active")
	ErrListingExpired                 = errors.New("market listing expired")
	ErrListingNotExpired              = errors.New("market listing is not expired")
	ErrInvalidStaleReason             = errors.New("invalid market stale reason")
	ErrListingSourceLocation          = errors.New("market listing source location is not allowed")
	ErrSellerCannotBuyOwnListing      = errors.New("seller cannot buy own listing")
	ErrListingOwnership               = errors.New("market listing seller mismatch")
	ErrBuyReferenceMismatch           = errors.New("market buy reference mismatch")
	ErrMarketBuyInProgress            = errors.New("market buy idempotency key in progress")
	ErrMarketBuyIdempotencyResult     = errors.New("market buy idempotency result unavailable")
	ErrMarketAmountOverflow           = errors.New("market amount overflow")
	ErrMarketEscrowQuantityMissing    = errors.New("market escrow quantity missing")
)
