package auth

import (
	"context"
	"errors"
	"time"

	"gameproject/internal/game/foundation"
)

const defaultSessionTTL = 24 * time.Hour

// ServiceConfig wires the auth service.
type ServiceConfig struct {
	Store          Store
	Clock          foundation.Clock
	PasswordHasher PasswordHasher
	TokenGenerator TokenGenerator
	SessionTTL     time.Duration
	AttemptTracker AuthAttemptTracker
	AttemptPolicy  AuthAttemptPolicy
}

// Service owns account, password, and session lifecycle rules.
type Service struct {
	store      Store
	clock      foundation.Clock
	passwords  PasswordHasher
	tokens     TokenGenerator
	sessionTTL time.Duration
	attempts   AuthAttemptTracker
}

// RegisterInput is the browser-supplied registration payload.
type RegisterInput struct {
	Email    string
	Password string
	Callsign string
}

// LoginInput is the browser-supplied login payload.
type LoginInput struct {
	Email    string
	Password string
}

// AuthResult contains a newly-created raw cookie token plus public session
// state. The token is deliberately not JSON-tagged and must only be written as
// an HttpOnly cookie.
type AuthResult struct {
	Token    string
	Session  ResolvedSession
	Response PublicSessionResponse
}

// NewService returns an auth service.
func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil {
		return nil, ErrNilAuthStore
	}
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	passwords := config.PasswordHasher
	if passwords == nil {
		passwords = PBKDF2PasswordHasher{}
	}
	tokens := config.TokenGenerator
	if tokens == nil {
		tokens = RandomTokenGenerator{}
	}
	ttl := config.SessionTTL
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	attempts := config.AttemptTracker
	if attempts == nil {
		attempts = NewInMemoryAuthAttemptTracker(config.AttemptPolicy)
	}
	return &Service{
		store:      config.Store,
		clock:      clock,
		passwords:  passwords,
		tokens:     tokens,
		sessionTTL: ttl,
		attempts:   attempts,
	}, nil
}

// Register creates an account, player profile, and first login session.
func (service *Service) Register(ctx context.Context, input RegisterInput) (AuthResult, error) {
	if service == nil {
		return AuthResult{}, ErrNilAuthService
	}
	attemptSubject := authAttemptSubject(input.Email)
	if err := service.requireAuthAttemptAllowed(AuthAttemptRegister, attemptSubject); err != nil {
		return AuthResult{}, err
	}
	email, err := NormalizeEmail(input.Email)
	if err != nil {
		return AuthResult{}, service.recordAuthAttemptFailure(AuthAttemptRegister, attemptSubject, invalidAuthPayload("Email is invalid.", err))
	}
	callsign, err := ValidateCallsign(input.Callsign)
	if err != nil {
		return AuthResult{}, service.recordAuthAttemptFailure(AuthAttemptRegister, attemptSubject, invalidAuthPayload("Callsign is invalid.", err))
	}
	passwordHash, err := service.passwords.HashPassword(input.Password)
	if err != nil {
		return AuthResult{}, service.recordAuthAttemptFailure(AuthAttemptRegister, attemptSubject, invalidAuthPayload("Password is invalid.", err))
	}
	now := service.now()
	accountID, playerID, err := service.newAccountIDs()
	if err != nil {
		return AuthResult{}, err
	}
	account := Account{
		ID:           accountID,
		Email:        email,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	player := PlayerProfile{
		ID:        playerID,
		AccountID: accountID,
		Callsign:  callsign,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := service.store.InsertAccount(ctx, account, player); err != nil {
		if errors.Is(err, ErrDuplicateEmail) {
			return AuthResult{}, service.recordAuthAttemptFailure(AuthAttemptRegister, attemptSubject, registrationRejected(err))
		}
		return AuthResult{}, err
	}
	if err := service.resetAuthAttempts(AuthAttemptRegister, attemptSubject); err != nil {
		return AuthResult{}, err
	}
	return service.createSession(ctx, account, player)
}

// Login verifies a password and creates a fresh session token.
func (service *Service) Login(ctx context.Context, input LoginInput) (AuthResult, error) {
	if service == nil {
		return AuthResult{}, ErrNilAuthService
	}
	attemptSubject := authAttemptSubject(input.Email)
	if err := service.requireAuthAttemptAllowed(AuthAttemptLogin, attemptSubject); err != nil {
		return AuthResult{}, err
	}
	email, err := NormalizeEmail(input.Email)
	if err != nil {
		return AuthResult{}, service.recordAuthAttemptFailure(AuthAttemptLogin, attemptSubject, invalidCredentials(err))
	}
	account, player, err := service.store.AccountByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return AuthResult{}, service.recordAuthAttemptFailure(AuthAttemptLogin, attemptSubject, invalidCredentials(err))
		}
		return AuthResult{}, err
	}
	ok, err := service.passwords.VerifyPassword(input.Password, account.PasswordHash)
	if err != nil || !ok {
		if err == nil {
			err = ErrInvalidPassword
		}
		return AuthResult{}, service.recordAuthAttemptFailure(AuthAttemptLogin, attemptSubject, invalidCredentials(err))
	}
	if err := service.resetAuthAttempts(AuthAttemptLogin, attemptSubject); err != nil {
		return AuthResult{}, err
	}
	return service.createSession(ctx, account, player)
}

func (service *Service) newAccountIDs() (foundation.AccountID, foundation.PlayerID, error) {
	accountRaw, err := service.tokens.NewID("acc")
	if err != nil {
		return "", "", err
	}
	playerRaw, err := service.tokens.NewID("player")
	if err != nil {
		return "", "", err
	}
	accountID, err := foundation.ParseAccountID(accountRaw)
	if err != nil {
		return "", "", err
	}
	playerID, err := foundation.ParsePlayerID(playerRaw)
	if err != nil {
		return "", "", err
	}
	return accountID, playerID, nil
}

func (service *Service) now() time.Time {
	if service == nil || service.clock == nil {
		return foundation.RealClock{}.Now().UTC()
	}
	return service.clock.Now().UTC()
}

func invalidAuthPayload(message string, cause error) *foundation.DomainError {
	return foundation.NewDomainError(foundation.CodeInvalidPayload, message, foundation.WithCause(cause))
}

func registrationRejected(cause error) *foundation.DomainError {
	return foundation.NewDomainError(
		foundation.CodeInvalidPayload,
		"Registration could not be completed.",
		foundation.WithCause(cause),
	)
}

func invalidCredentials(cause error) *foundation.DomainError {
	return foundation.NewDomainError(
		foundation.CodeUnauthenticated,
		"Email or password is invalid.",
		foundation.WithCause(cause),
	)
}

func authRateLimited(cause error) *foundation.DomainError {
	opts := make([]foundation.DomainErrorOption, 0, 1)
	if cause != nil {
		opts = append(opts, foundation.WithCause(cause))
	}
	return foundation.NewDomainError(
		foundation.CodeRateLimited,
		"Too many auth attempts. Try again later.",
		opts...,
	)
}

func (service *Service) requireAuthAttemptAllowed(operation AuthAttemptOperation, subject string) error {
	if service.attempts == nil {
		return nil
	}
	decision, err := service.attempts.Check(operation, subject, service.now())
	if err != nil {
		return err
	}
	if decision.Limited {
		return authRateLimited(nil)
	}
	return nil
}

func (service *Service) recordAuthAttemptFailure(operation AuthAttemptOperation, subject string, publicErr error) error {
	if service.attempts == nil {
		return publicErr
	}
	decision, err := service.attempts.RecordFailure(operation, subject, service.now())
	if err != nil {
		return err
	}
	if decision.Limited {
		return authRateLimited(publicErr)
	}
	return publicErr
}

func (service *Service) resetAuthAttempts(operation AuthAttemptOperation, subject string) error {
	if service.attempts == nil {
		return nil
	}
	return service.attempts.Reset(operation, subject, service.now())
}

func authRequired(cause error) *foundation.DomainError {
	return foundation.NewDomainError(
		foundation.CodeAuthRequired,
		"Authenticated session is required.",
		foundation.WithCause(cause),
	)
}
