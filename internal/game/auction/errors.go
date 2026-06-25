package auction

import "errors"

var (
	ErrMissingWalletService    = errors.New("missing wallet service")
	ErrDuplicateLotID          = errors.New("duplicate auction lot id")
	ErrLotNotFound             = errors.New("auction lot not found")
	ErrInvalidLotStatus        = errors.New("invalid auction lot status")
	ErrInvalidLotTransition    = errors.New("invalid auction lot transition")
	ErrInvalidLotPayloadType   = errors.New("invalid auction lot payload type")
	ErrInvalidLotPayload       = errors.New("invalid auction lot payload")
	ErrInvalidLotTiming        = errors.New("invalid auction lot timing")
	ErrLotNotActive            = errors.New("auction lot is not active")
	ErrLotNotStarted           = errors.New("auction lot has not started")
	ErrLotEnded                = errors.New("auction lot ended")
	ErrBidTooLow               = errors.New("auction bid too low")
	ErrBidReachesBuyNow        = errors.New("auction bid reaches buy-now price")
	ErrCurrentWinningBidder    = errors.New("bidder is already winning auction")
	ErrBuyNowUnavailable       = errors.New("auction buy-now unavailable")
	ErrBuyNowInProgress        = errors.New("auction buy-now idempotency key in progress")
	ErrBuyNowIdempotencyResult = errors.New("auction buy-now idempotency result unavailable")
	ErrCloseTooEarly           = errors.New("auction close too early")
	ErrAuctionAmountOverflow   = errors.New("auction amount overflow")
	ErrBidReferenceMismatch    = errors.New("auction bid reference mismatch")
	ErrBuyNowReferenceMismatch = errors.New("auction buy-now reference mismatch")
	ErrCloseReferenceMismatch  = errors.New("auction close reference mismatch")
)
