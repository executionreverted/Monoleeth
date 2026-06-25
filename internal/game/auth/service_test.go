package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestRegisterCreatesAccountPlayerAndHashedSessionToken(t *testing.T) {
	service, store, _ := newTestAuthService(t)

	result, err := service.Register(context.Background(), RegisterInput{
		Email:    " Pilot@Example.COM ",
		Password: "correct-password",
		Callsign: "Frontier-01",
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	if result.Token == "" {
		t.Fatal("Register() token is empty")
	}
	if !result.Response.Authenticated || result.Response.Account.Email != "pilot@example.com" ||
		result.Response.Account.Admin || result.Response.Player.Callsign != "Frontier-01" {
		t.Fatalf("public response = %+v, want normalized non-admin account/player", result.Response)
	}
	account, player, err := store.AccountByEmail(context.Background(), "pilot@example.com")
	if err != nil {
		t.Fatalf("AccountByEmail() error = %v, want nil", err)
	}
	if account.ID.IsZero() || player.ID.IsZero() || player.AccountID != account.ID {
		t.Fatalf("account/player = %+v / %+v, want linked server ids", account, player)
	}
	if account.PasswordHash == "" || string(account.PasswordHash) == "correct-password" {
		t.Fatalf("password hash = %q, want non-plaintext verifier", account.PasswordHash)
	}
	session, err := store.SessionByID(context.Background(), result.Session.SessionID)
	if err != nil {
		t.Fatalf("SessionByID() error = %v, want nil", err)
	}
	if session.TokenHash == "" || session.TokenHash == result.Token {
		t.Fatalf("stored token hash = %q raw = %q, want hashed-at-rest token", session.TokenHash, result.Token)
	}
	wantHash, err := tokenHash(result.Token)
	if err != nil {
		t.Fatalf("tokenHash() error = %v, want nil", err)
	}
	if session.TokenHash != wantHash {
		t.Fatalf("stored token hash = %q, want %q", session.TokenHash, wantHash)
	}
}

func TestDuplicateRegisterUsesGenericPublicError(t *testing.T) {
	service, _, _ := newTestAuthService(t)
	input := RegisterInput{Email: "pilot@example.com", Password: "correct-password", Callsign: "Frontier-01"}
	if _, err := service.Register(context.Background(), input); err != nil {
		t.Fatalf("first Register() error = %v, want nil", err)
	}

	publicErr := publicErrorFor(t, func() error {
		_, err := service.Register(context.Background(), input)
		return err
	})

	if publicErr.Code != foundation.CodeInvalidPayload || publicErr.Message != "Registration could not be completed." {
		t.Fatalf("duplicate Register() public error = %+v, want generic invalid payload", publicErr)
	}
}

func TestRepeatedDuplicateRegisterDoesNotRevealAccountExistence(t *testing.T) {
	service, _, _ := newTestAuthService(t)
	input := RegisterInput{Email: "pilot@example.com", Password: "correct-password", Callsign: "Frontier-01"}
	if _, err := service.Register(context.Background(), input); err != nil {
		t.Fatalf("first Register() error = %v, want nil", err)
	}

	for attempt := 1; attempt <= defaultAuthAttemptMaxFailures; attempt++ {
		publicErr := publicErrorFor(t, func() error {
			_, err := service.Register(context.Background(), input)
			return err
		})
		message := strings.ToLower(publicErr.Message)
		if strings.Contains(message, "registered") || strings.Contains(message, "exists") || strings.Contains(message, "pilot@example.com") {
			t.Fatalf("duplicate Register() public error leaked account existence: %+v", publicErr)
		}
	}
}

func TestLoginCreatesFreshSession(t *testing.T) {
	service, _, _ := newTestAuthService(t)
	registered, err := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "correct-password",
		Callsign: "Frontier-01",
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	loggedIn, err := service.Login(context.Background(), LoginInput{Email: "PILOT@example.com", Password: "correct-password"})
	if err != nil {
		t.Fatalf("Login() error = %v, want nil", err)
	}
	if loggedIn.Token == registered.Token || loggedIn.Session.SessionID == registered.Session.SessionID {
		t.Fatalf("login reused session token/id, want rotation")
	}
}

func TestFailedLoginExistingAndMissingEmailSharePublicCodeMessage(t *testing.T) {
	service, _, _ := newTestAuthService(t)
	if _, err := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "correct-password",
		Callsign: "Frontier-01",
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	wrongPassword := publicErrorFor(t, func() error {
		_, err := service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "wrong-password"})
		return err
	})
	wrongEmail := publicErrorFor(t, func() error {
		_, err := service.Login(context.Background(), LoginInput{Email: "missing@example.com", Password: "wrong-password"})
		return err
	})
	if wrongPassword != wrongEmail || wrongPassword.Code != foundation.CodeUnauthenticated {
		t.Fatalf("invalid credential errors = %+v / %+v, want same unauthenticated public shape", wrongPassword, wrongEmail)
	}
}

func TestRepeatedFailedLoginTriggersLockout(t *testing.T) {
	service, _, _ := newTestAuthService(t)
	if _, err := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "correct-password",
		Callsign: "Frontier-01",
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	for attempt := 1; attempt < defaultAuthAttemptMaxFailures; attempt++ {
		_, err := service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "wrong-password"})
		if !foundation.IsCode(err, foundation.CodeUnauthenticated) {
			t.Fatalf("Login(wrong password) attempt %d error = %v, want %s", attempt, err, foundation.CodeUnauthenticated)
		}
	}
	_, err := service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "wrong-password"})
	if !foundation.IsCode(err, foundation.CodeRateLimited) {
		t.Fatalf("Login(wrong password) lockout error = %v, want %s", err, foundation.CodeRateLimited)
	}
}

func TestLoginLockoutExistingAndMissingEmailSharePublicCodeMessage(t *testing.T) {
	service, _, _ := newTestAuthService(t)
	if _, err := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "correct-password",
		Callsign: "Frontier-01",
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	existing := lockedLoginPublicError(t, service, "pilot@example.com")
	missing := lockedLoginPublicError(t, service, "missing@example.com")

	if existing != missing || existing.Code != foundation.CodeRateLimited {
		t.Fatalf("lockout errors = %+v / %+v, want same rate-limited public shape", existing, missing)
	}
}

func TestLockoutLoginDoesNotCreateSession(t *testing.T) {
	service, store, _ := newTestAuthService(t)
	if _, err := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "correct-password",
		Callsign: "Frontier-01",
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	before := sessionCount(store)
	for attempt := 1; attempt <= defaultAuthAttemptMaxFailures; attempt++ {
		_, _ = service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "wrong-password"})
	}

	_, err := service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "correct-password"})

	if !foundation.IsCode(err, foundation.CodeRateLimited) {
		t.Fatalf("Login(throttled correct password) error = %v, want %s", err, foundation.CodeRateLimited)
	}
	if after := sessionCount(store); after != before {
		t.Fatalf("session count after throttled login = %d, want unchanged %d", after, before)
	}
}

func TestLoginLockoutExpires(t *testing.T) {
	service, _, clock := newTestAuthService(t)
	if _, err := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "correct-password",
		Callsign: "Frontier-01",
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	for attempt := 1; attempt <= defaultAuthAttemptMaxFailures; attempt++ {
		_, _ = service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "wrong-password"})
	}

	clock.now = clock.now.Add(defaultAuthAttemptLockout + time.Nanosecond)
	_, err := service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "correct-password"})

	if err != nil {
		t.Fatalf("Login(after lockout expiry) error = %v, want nil", err)
	}
}

func TestLoginSuccessResetsFailedAttempts(t *testing.T) {
	service, _, _ := newTestAuthService(t)
	if _, err := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "correct-password",
		Callsign: "Frontier-01",
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	for attempt := 1; attempt < defaultAuthAttemptMaxFailures; attempt++ {
		_, err := service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "wrong-password"})
		if !foundation.IsCode(err, foundation.CodeUnauthenticated) {
			t.Fatalf("Login(wrong password) before reset error = %v, want %s", err, foundation.CodeUnauthenticated)
		}
	}
	if _, err := service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "correct-password"}); err != nil {
		t.Fatalf("Login(correct password) error = %v, want nil", err)
	}

	_, err := service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "wrong-password"})

	if !foundation.IsCode(err, foundation.CodeUnauthenticated) {
		t.Fatalf("Login(wrong password) after reset error = %v, want %s", err, foundation.CodeUnauthenticated)
	}
}

func TestLogoutRevokesSessionAndExpiredSessionIsRejected(t *testing.T) {
	service, _, clock := newTestAuthService(t)
	result, err := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "correct-password",
		Callsign: "Frontier-01",
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	if err := service.LogoutByToken(context.Background(), result.Token); err != nil {
		t.Fatalf("LogoutByToken() error = %v, want nil", err)
	}
	if _, err := service.ResolveToken(context.Background(), result.Token); !foundation.IsCode(err, foundation.CodeSessionRevoked) {
		t.Fatalf("ResolveToken(revoked) error = %v, want %s", err, foundation.CodeSessionRevoked)
	}

	fresh, err := service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "correct-password"})
	if err != nil {
		t.Fatalf("Login() error = %v, want nil", err)
	}
	clock.now = clock.now.Add(2 * time.Hour)
	if _, err := service.ResolveSessionID(context.Background(), fresh.Session.SessionID); !foundation.IsCode(err, foundation.CodeSessionExpired) {
		t.Fatalf("ResolveSessionID(expired) error = %v, want %s", err, foundation.CodeSessionExpired)
	}
}

func TestAdminSeedCreatesAndUpdatesAdminWithoutDefaults(t *testing.T) {
	service, store, _ := newTestAuthService(t)

	if _, err := service.SeedAdmin(context.Background(), AdminSeedInput{Enabled: true, Email: "admin@example.com"}); !errors.Is(err, ErrMissingAdminSeedInput) {
		t.Fatalf("SeedAdmin(missing password) error = %v, want ErrMissingAdminSeedInput", err)
	}
	created, err := service.SeedAdmin(context.Background(), AdminSeedInput{
		Enabled:  true,
		Email:    "ADMIN@example.com",
		Password: "initial-password",
		Callsign: "Admin-01",
	})
	if err != nil {
		t.Fatalf("SeedAdmin(create) error = %v, want nil", err)
	}
	if !created.Applied || !created.Created || created.Email != "admin@example.com" {
		t.Fatalf("created seed result = %+v, want created normalized admin", created)
	}
	account, _, err := store.AccountByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("AccountByEmail(admin) error = %v, want nil", err)
	}
	if !hasRole(account.Roles, RoleAdmin) {
		t.Fatalf("admin account roles = %+v, want admin", account.Roles)
	}
	if _, err := service.Login(context.Background(), LoginInput{Email: "admin@example.com", Password: "initial-password"}); err != nil {
		t.Fatalf("Login(initial admin password) error = %v, want nil", err)
	}

	updated, err := service.SeedAdmin(context.Background(), AdminSeedInput{
		Enabled:  true,
		Email:    "admin@example.com",
		Password: "rotated-password",
		Callsign: "Ignored-For-Existing",
	})
	if err != nil {
		t.Fatalf("SeedAdmin(update) error = %v, want nil", err)
	}
	if !updated.Applied || updated.Created {
		t.Fatalf("updated seed result = %+v, want update", updated)
	}
	if _, err := service.Login(context.Background(), LoginInput{Email: "admin@example.com", Password: "rotated-password"}); err != nil {
		t.Fatalf("Login(rotated admin password) error = %v, want nil", err)
	}
}

func publicErrorFor(t *testing.T, run func() error) foundation.PublicError {
	t.Helper()
	err := run()
	var domainErr *foundation.DomainError
	if !errors.As(err, &domainErr) {
		t.Fatalf("error = %v, want DomainError", err)
	}
	return domainErr.Public()
}

func lockedLoginPublicError(t *testing.T, service *Service, email string) foundation.PublicError {
	t.Helper()
	for attempt := 1; attempt < defaultAuthAttemptMaxFailures; attempt++ {
		_, err := service.Login(context.Background(), LoginInput{Email: email, Password: "wrong-password"})
		if !foundation.IsCode(err, foundation.CodeUnauthenticated) {
			t.Fatalf("Login(lock setup %d) error = %v, want %s", attempt, err, foundation.CodeUnauthenticated)
		}
	}
	return publicErrorFor(t, func() error {
		_, err := service.Login(context.Background(), LoginInput{Email: email, Password: "wrong-password"})
		return err
	})
}

func newTestAuthService(t *testing.T) (*Service, *InMemoryStore, *testClock) {
	t.Helper()
	store := NewInMemoryStore()
	clock := &testClock{now: time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)}
	service, err := NewService(ServiceConfig{
		Store:          store,
		Clock:          clock,
		PasswordHasher: PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
		TokenGenerator: &testTokenGenerator{},
		SessionTTL:     time.Hour,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v, want nil", err)
	}
	return service, store, clock
}

func sessionCount(store *InMemoryStore) int {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return len(store.sessionsByID)
}

type testClock struct {
	now time.Time
}

func (clock *testClock) Now() time.Time {
	return clock.now
}

type testTokenGenerator struct {
	next int
}

func (generator *testTokenGenerator) NewSessionToken() (string, error) {
	generator.next++
	return "raw_token_" + time.Unix(int64(generator.next), 0).UTC().Format("20060102150405"), nil
}

func (generator *testTokenGenerator) NewID(prefix string) (string, error) {
	generator.next++
	return prefix + "_test_" + time.Unix(int64(generator.next), 0).UTC().Format("20060102150405"), nil
}
