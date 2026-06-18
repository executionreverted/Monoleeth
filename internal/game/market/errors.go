package market

import "errors"

var (
	ErrMissingInventoryService     = errors.New("missing inventory service")
	ErrMissingWalletService        = errors.New("missing wallet service")
	ErrInvalidFeePolicy            = errors.New("invalid market fee policy")
	ErrInvalidListingStatus        = errors.New("invalid market listing status")
	ErrInvalidListingTransition    = errors.New("invalid market listing transition")
	ErrDuplicateListingID          = errors.New("duplicate market listing id")
	ErrListingNotFound             = errors.New("market listing not found")
	ErrListingNotActive            = errors.New("market listing is not active")
	ErrListingExpired              = errors.New("market listing expired")
	ErrListingSourceLocation       = errors.New("market listing source location is not allowed")
	ErrSellerCannotBuyOwnListing   = errors.New("seller cannot buy own listing")
	ErrListingOwnership            = errors.New("market listing seller mismatch")
	ErrBuyReferenceMismatch        = errors.New("market buy reference mismatch")
	ErrMarketAmountOverflow        = errors.New("market amount overflow")
	ErrMarketEscrowQuantityMissing = errors.New("market escrow quantity missing")
)
