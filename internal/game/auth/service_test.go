package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
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

func TestRegisterBackoffRepeatedDuplicateRegisterTriggersLockout(t *testing.T) {
	service, _, _ := newTestAuthService(t)
	input := RegisterInput{Email: "pilot@example.com", Password: "correct-password", Callsign: "Frontier-01"}
	if _, err := service.Register(context.Background(), input); err != nil {
		t.Fatalf("first Register() error = %v, want nil", err)
	}

	for attempt := 1; attempt < defaultAuthAttemptMaxFailures; attempt++ {
		_, err := service.Register(context.Background(), input)
		if !foundation.IsCode(err, foundation.CodeInvalidPayload) {
			t.Fatalf("duplicate Register() attempt %d error = %v, want %s", attempt, err, foundation.CodeInvalidPayload)
		}
	}
	_, err := service.Register(context.Background(), input)

	if !foundation.IsCode(err, foundation.CodeRateLimited) {
		t.Fatalf("duplicate Register() lockout error = %v, want %s", err, foundation.CodeRateLimited)
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

func TestAuthRegisterLoginStructuredLogSuccessNoSecrets(t *testing.T) {
	logger := observability.NewMemoryAuthTransitionLogger()
	service, _, _ := newTestAuthServiceWithTransitionLogger(t, logger)

	registered, err := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "register-secret-password",
		Callsign: "Frontier-01",
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	loggedIn, err := service.Login(context.Background(), LoginInput{
		Email:    "pilot@example.com",
		Password: "register-secret-password",
	})
	if err != nil {
		t.Fatalf("Login() error = %v, want nil", err)
	}

	entries := logger.Snapshot()
	if len(entries) != 2 {
		t.Fatalf("auth transition log entries = %d, want 2", len(entries))
	}
	assertAuthTransitionSuccess(t, requireAuthTransitionEntry(t, entries, AuthAttemptRegister), AuthAttemptRegister, registered)
	assertAuthTransitionSuccess(t, requireAuthTransitionEntry(t, entries, AuthAttemptLogin), AuthAttemptLogin, loggedIn)
	assertAuthTransitionLogsNoSecrets(t, entries,
		"register-secret-password",
		"pilot@example.com",
		registered.Token,
		loggedIn.Token,
		"password",
		"password_hash",
		"raw_token",
		"session_token",
		"cookie",
		"hash",
	)
}

func TestAuthRegisterLoginStructuredLogFailureNoSecrets(t *testing.T) {
	service, _, _ := newTestAuthService(t)
	if _, err := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "initial-secret-password",
		Callsign: "Frontier-01",
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	logger := observability.NewMemoryAuthTransitionLogger()
	service.transitionLogger = logger

	_, registerErr := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "duplicate-secret-password",
		Callsign: "Frontier-02",
	})
	if !foundation.IsCode(registerErr, foundation.CodeInvalidPayload) {
		t.Fatalf("duplicate Register() error = %v, want %s", registerErr, foundation.CodeInvalidPayload)
	}
	_, loginErr := service.Login(context.Background(), LoginInput{
		Email:    "pilot@example.com",
		Password: "wrong-secret-password",
	})
	if !foundation.IsCode(loginErr, foundation.CodeUnauthenticated) {
		t.Fatalf("Login(wrong password) error = %v, want %s", loginErr, foundation.CodeUnauthenticated)
	}

	entries := logger.Snapshot()
	if len(entries) != 2 {
		t.Fatalf("auth transition log entries = %d, want 2", len(entries))
	}
	assertAuthTransitionFailure(t, requireAuthTransitionEntry(t, entries, AuthAttemptRegister), AuthAttemptRegister, foundation.CodeInvalidPayload)
	assertAuthTransitionFailure(t, requireAuthTransitionEntry(t, entries, AuthAttemptLogin), AuthAttemptLogin, foundation.CodeUnauthenticated)
	assertAuthTransitionLogsNoSecrets(t, entries,
		"initial-secret-password",
		"duplicate-secret-password",
		"wrong-secret-password",
		"pilot@example.com",
		"password",
		"password_hash",
		"raw_token",
		"session_token",
		"cookie",
		"hash",
	)
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

func TestDisableAttemptsSkipsLoginLockout(t *testing.T) {
	store := NewInMemoryStore()
	service, err := NewService(ServiceConfig{
		Store:           store,
		Clock:           &testClock{now: time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)},
		PasswordHasher:  PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
		TokenGenerator:  &testTokenGenerator{},
		SessionTTL:      time.Hour,
		DisableAttempts: true,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v, want nil", err)
	}
	if _, err := service.Register(context.Background(), RegisterInput{
		Email:    "pilot@example.com",
		Password: "correct-password",
		Callsign: "Frontier-01",
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	for attempt := 1; attempt <= defaultAuthAttemptMaxFailures+2; attempt++ {
		_, err := service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "wrong-password"})
		if !foundation.IsCode(err, foundation.CodeUnauthenticated) {
			t.Fatalf("Login(wrong password) attempt %d error = %v, want %s", attempt, err, foundation.CodeUnauthenticated)
		}
	}
	if _, err := service.Login(context.Background(), LoginInput{Email: "pilot@example.com", Password: "correct-password"}); err != nil {
		t.Fatalf("Login(correct after disabled attempts) error = %v, want nil", err)
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
	return newTestAuthServiceWithTransitionLogger(t, nil)
}

func newTestAuthServiceWithTransitionLogger(t *testing.T, logger observability.AuthTransitionLogger) (*Service, *InMemoryStore, *testClock) {
	t.Helper()
	store := NewInMemoryStore()
	clock := &testClock{now: time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)}
	service, err := NewService(ServiceConfig{
		Store:            store,
		Clock:            clock,
		PasswordHasher:   PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
		TokenGenerator:   &testTokenGenerator{},
		SessionTTL:       time.Hour,
		TransitionLogger: logger,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v, want nil", err)
	}
	return service, store, clock
}

func requireAuthTransitionEntry(t *testing.T, entries []observability.AuthTransitionLogEntry, operation AuthAttemptOperation) observability.AuthTransitionLogEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.Request.Operation == observability.Operation(operation) {
			return entry
		}
	}
	t.Fatalf("auth transition log missing operation %q in %+v", operation, entries)
	return observability.AuthTransitionLogEntry{}
}

func assertAuthTransitionSuccess(t *testing.T, entry observability.AuthTransitionLogEntry, operation AuthAttemptOperation, result AuthResult) {
	t.Helper()
	if entry.Request.Operation != observability.Operation(operation) || entry.Status != observability.CommandStatusOK || !entry.ErrorCode.IsZero() {
		t.Fatalf("auth transition entry = %+v, want %s ok without error code", entry, operation)
	}
	if entry.PlayerID != result.Session.PlayerID || entry.SessionID != observability.SessionID(result.Session.SessionID.String()) {
		t.Fatalf("auth transition identity = %+v, want player/session from result %+v", entry, result.Session)
	}
	assertAuthTransitionPublicFields(t, entry, operation)
}

func assertAuthTransitionFailure(t *testing.T, entry observability.AuthTransitionLogEntry, operation AuthAttemptOperation, code foundation.Code) {
	t.Helper()
	if entry.Request.Operation != observability.Operation(operation) || entry.Status != observability.CommandStatusError || entry.ErrorCode != code {
		t.Fatalf("auth transition entry = %+v, want %s error %s", entry, operation, code)
	}
	if !entry.PlayerID.IsZero() || !entry.SessionID.IsZero() {
		t.Fatalf("auth failure transition identity = %+v, want no player/session enumeration", entry)
	}
	assertAuthTransitionPublicFields(t, entry, operation)
}

func assertAuthTransitionPublicFields(t *testing.T, entry observability.AuthTransitionLogEntry, operation AuthAttemptOperation) {
	t.Helper()
	payload, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal auth transition log: %v", err)
	}
	var public map[string]any
	if err := json.Unmarshal(payload, &public); err != nil {
		t.Fatalf("decode auth transition log: %v", err)
	}
	request, ok := public["request"].(map[string]any)
	if !ok || request["op"] != string(operation) {
		t.Fatalf("auth transition request = %#v, want op %q in %s", public["request"], operation, payload)
	}
	for _, field := range []string{"request", "op", "result", "error_code", "duration_ms", "timestamp"} {
		if _, ok := public[field]; !ok {
			t.Fatalf("auth transition log missing field %q in %s", field, payload)
		}
	}
	if got, ok := public["duration_ms"].(float64); !ok || got < 0 {
		t.Fatalf("auth transition duration_ms = %#v, want non-negative number in %s", public["duration_ms"], payload)
	}
}

func assertAuthTransitionLogsNoSecrets(t *testing.T, entries []observability.AuthTransitionLogEntry, forbidden ...string) {
	t.Helper()
	for _, entry := range entries {
		payload, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("marshal auth transition log: %v", err)
		}
		for _, leaked := range forbidden {
			if leaked == "" {
				continue
			}
			if strings.Contains(string(payload), leaked) {
				t.Fatalf("auth transition log leaked %q in %s", leaked, payload)
			}
		}
	}
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
