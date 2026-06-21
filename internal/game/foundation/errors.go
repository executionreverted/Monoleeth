package foundation

import (
	"encoding/json"
	"errors"
)

// Code is a stable, client-safe error code.
type Code string

const (
	CodeUnauthenticated Code = "ERR_UNAUTHENTICATED"
	CodeForbidden       Code = "ERR_FORBIDDEN"
	CodeNotFound        Code = "ERR_NOT_FOUND"
	CodeInvalidPayload  Code = "ERR_INVALID_PAYLOAD"
	CodeRateLimited     Code = "ERR_RATE_LIMITED"
	CodeInternal        Code = "ERR_INTERNAL"
	CodeAuthRequired    Code = "ERR_AUTH_REQUIRED"
	CodeSessionExpired  Code = "ERR_SESSION_EXPIRED"
	CodeSessionRevoked  Code = "ERR_SESSION_REVOKED"
	CodeOriginDenied    Code = "ERR_ORIGIN_DENIED"

	CodeOutOfRange       Code = "ERR_OUT_OF_RANGE"
	CodeNotVisible       Code = "ERR_NOT_VISIBLE"
	CodeCooldown         Code = "ERR_COOLDOWN"
	CodeNotEnoughEnergy  Code = "ERR_NOT_ENOUGH_ENERGY"
	CodeNotEnoughCargo   Code = "ERR_NOT_ENOUGH_CARGO"
	CodeNotEnoughFunds   Code = "ERR_NOT_ENOUGH_FUNDS"
	CodeRankTooLow       Code = "ERR_RANK_TOO_LOW"
	CodeItemNotTradeable Code = "ERR_ITEM_NOT_TRADEABLE"
	CodeShipDisabled     Code = "ERR_SHIP_DISABLED"
	CodeStorageFull      Code = "ERR_STORAGE_FULL"
	CodePVPBlocked       Code = "ERR_PVP_BLOCKED"
)

// String returns the wire representation of the code.
func (c Code) String() string {
	return string(c)
}

// MarshalText returns the stable text representation used by encoders.
func (c Code) MarshalText() ([]byte, error) {
	return []byte(c), nil
}

// IsZero reports whether c is the zero value.
func (c Code) IsZero() bool {
	return c == ""
}

// DomainError is a gameplay error with a stable public code and safe message.
type DomainError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`

	detail string
	cause  error
}

// PublicError is the default client-safe representation of a DomainError.
type PublicError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

// DomainErrorOption configures a DomainError without expanding its public shape.
type DomainErrorOption func(*DomainError)

// NewDomainError returns a DomainError. Message must be safe to expose to clients.
func NewDomainError(code Code, message string, opts ...DomainErrorOption) *DomainError {
	err := &DomainError{
		Code:    code,
		Message: message,
	}
	for _, opt := range opts {
		opt(err)
	}
	return err
}

// WithDetail attaches internal diagnostic detail that is not public by default.
func WithDetail(detail string) DomainErrorOption {
	return func(err *DomainError) {
		err.detail = detail
	}
}

// WithCause attaches an underlying error for unwrapping.
func WithCause(cause error) DomainErrorOption {
	return func(err *DomainError) {
		err.cause = cause
	}
}

// Error returns only the public code and safe message.
func (err *DomainError) Error() string {
	if err == nil {
		return "<nil>"
	}
	if err.Message == "" {
		return err.Code.String()
	}
	if err.Code.IsZero() {
		return err.Message
	}
	return err.Code.String() + ": " + err.Message
}

// Unwrap returns the internal cause, when one exists.
func (err *DomainError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

// Public returns the default client-safe payload for err.
func (err *DomainError) Public() PublicError {
	if err == nil {
		return PublicError{}
	}
	return PublicError{
		Code:    err.Code,
		Message: err.Message,
	}
}

// InternalDetail returns internal diagnostics for server logs and tests.
func (err *DomainError) InternalDetail() string {
	if err == nil {
		return ""
	}
	return err.detail
}

// MarshalJSON encodes only the default public representation.
func (err DomainError) MarshalJSON() ([]byte, error) {
	return json.Marshal(err.Public())
}

// IsCode reports whether err contains a DomainError with code.
func IsCode(err error, code Code) bool {
	got, ok := CodeOf(err)
	return ok && got == code
}

// CodeOf returns the first DomainError code found in err's chain.
func CodeOf(err error) (Code, bool) {
	var domainErr *DomainError
	if !errors.As(err, &domainErr) {
		return "", false
	}
	return domainErr.Code, true
}
