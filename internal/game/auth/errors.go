package auth

import "errors"

var (
	ErrDuplicateEmail        = errors.New("duplicate email")
	ErrAccountNotFound       = errors.New("account not found")
	ErrSessionNotFound       = errors.New("session not found")
	ErrSessionExpired        = errors.New("session expired")
	ErrSessionRevoked        = errors.New("session revoked")
	ErrInvalidEmail          = errors.New("invalid email")
	ErrInvalidPassword       = errors.New("invalid password")
	ErrInvalidPasswordHash   = errors.New("invalid password hash")
	ErrInvalidCallsign       = errors.New("invalid callsign")
	ErrInvalidSessionToken   = errors.New("invalid session token")
	ErrDuplicateSession      = errors.New("duplicate session")
	ErrMissingAdminSeedInput = errors.New("missing admin seed input")
	ErrOriginDenied          = errors.New("origin denied")
	ErrNilAuthService        = errors.New("nil auth service")
	ErrNilAuthStore          = errors.New("nil auth store")
)
