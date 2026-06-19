package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestHTTPRegisterLoginSessionAndLogoutUseSecurePublicCookieFlow(t *testing.T) {
	handler, service := newTestHTTPHandler(t)

	register := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"pilot@example.com","password":"correct-password","callsign":"Frontier-01"}`))
	register.Header.Set("Origin", "http://example.com")
	register.Header.Set("Content-Type", "application/json")
	registerRecorder := httptest.NewRecorder()
	handler.ServeHTTP(registerRecorder, register)

	if registerRecorder.Code != http.StatusCreated {
		t.Fatalf("register status = %d body = %s, want 201", registerRecorder.Code, registerRecorder.Body.String())
	}
	body := registerRecorder.Body.String()
	for _, forbidden := range []string{"correct-password", "password_hash", "session_id", "account_id", "player_id", "raw_token"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Fatalf("register response body leaked %q: %s", forbidden, body)
		}
	}
	registerCookie := sessionCookie(t, registerRecorder.Result())
	if !registerCookie.HttpOnly || registerCookie.SameSite != http.SameSiteLaxMode || registerCookie.Value == "" {
		t.Fatalf("register cookie = %+v, want HttpOnly SameSite=Lax non-empty value", registerCookie)
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	sessionReq.AddCookie(registerCookie)
	sessionRecorder := httptest.NewRecorder()
	handler.ServeHTTP(sessionRecorder, sessionReq)
	if sessionRecorder.Code != http.StatusOK {
		t.Fatalf("session status = %d body = %s, want 200", sessionRecorder.Code, sessionRecorder.Body.String())
	}
	var session PublicSessionResponse
	if err := json.Unmarshal(sessionRecorder.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if !session.Authenticated || session.Account.Email != "pilot@example.com" || session.Player.Callsign != "Frontier-01" {
		t.Fatalf("session response = %+v, want authenticated public account/player", session)
	}

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"pilot@example.com","password":"correct-password"}`))
	login.Header.Set("Origin", "http://example.com")
	loginRecorder := httptest.NewRecorder()
	handler.ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("login status = %d body = %s, want 200", loginRecorder.Code, loginRecorder.Body.String())
	}
	loginCookie := sessionCookie(t, loginRecorder.Result())
	if loginCookie.Value == registerCookie.Value {
		t.Fatal("login cookie reused register token, want rotated token")
	}

	logout := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logout.Header.Set("Origin", "http://example.com")
	logout.AddCookie(loginCookie)
	logoutRecorder := httptest.NewRecorder()
	handler.ServeHTTP(logoutRecorder, logout)
	if logoutRecorder.Code != http.StatusOK {
		t.Fatalf("logout status = %d body = %s, want 200", logoutRecorder.Code, logoutRecorder.Body.String())
	}
	clearedCookie := sessionCookie(t, logoutRecorder.Result())
	if clearedCookie.MaxAge >= 0 {
		t.Fatalf("logout cookie max age = %d, want deletion", clearedCookie.MaxAge)
	}
	if _, err := service.ResolveToken(logout.Context(), loginCookie.Value); !foundation.IsCode(err, foundation.CodeSessionRevoked) {
		t.Fatalf("ResolveToken(logged out) error = %v, want revoked", err)
	}
}

func TestHTTPCrossSiteLogoutIsRejectedWithoutRevokingSession(t *testing.T) {
	handler, service := newTestHTTPHandler(t)
	result, err := service.Register(nil, RegisterInput{Email: "pilot@example.com", Password: "correct-password", Callsign: "Frontier-01"})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	logout := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logout.Header.Set("Origin", "http://evil.example")
	logout.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: result.Token})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, logout)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-site logout status = %d body = %s, want 403", recorder.Code, recorder.Body.String())
	}
	if _, err := service.ResolveToken(logout.Context(), result.Token); err != nil {
		t.Fatalf("ResolveToken(after rejected logout) error = %v, want nil", err)
	}
}

func TestHTTPCredentialFailuresSharePublicShape(t *testing.T) {
	handler, _ := newTestHTTPHandler(t)
	register := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"pilot@example.com","password":"correct-password","callsign":"Frontier-01"}`))
	register.Header.Set("Origin", "http://example.com")
	handler.ServeHTTP(httptest.NewRecorder(), register)

	wrongPassword := authHTTPError(t, handler, `{"email":"pilot@example.com","password":"wrong-password"}`)
	wrongEmail := authHTTPError(t, handler, `{"email":"missing@example.com","password":"wrong-password"}`)
	if wrongPassword != wrongEmail || wrongPassword.Code != foundation.CodeUnauthenticated {
		t.Fatalf("login errors = %+v / %+v, want same unauthenticated public shape", wrongPassword, wrongEmail)
	}
}

func TestHTTPRegisterRejectsClientAuthoredAdminRole(t *testing.T) {
	handler, _ := newTestHTTPHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"pilot@example.com","password":"correct-password","callsign":"Frontier-01","admin":true}`))
	req.Header.Set("Origin", "http://example.com")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("register with admin field status = %d body = %s, want 400", recorder.Code, recorder.Body.String())
	}
}

func TestHTTPAllowedOriginCORSIsCredentialedAndNotWildcard(t *testing.T) {
	handler, _ := newTestHTTPHandler(t)
	req := httptest.NewRequest(http.MethodOptions, "/api/auth/login", nil)
	req.Header.Set("Origin", "http://client.example")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://client.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want allowed origin", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Fatal("Access-Control-Allow-Origin used wildcard with credentials")
	}
}

func authHTTPError(t *testing.T, handler http.Handler, payload string) foundation.PublicError {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(payload))
	req.Header.Set("Origin", "http://example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	var body struct {
		Error foundation.PublicError `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode auth error response: %v", err)
	}
	return body.Error
}

func newTestHTTPHandler(t *testing.T) (http.Handler, *Service) {
	t.Helper()
	service, _, _ := newTestAuthService(t)
	handler, err := NewHTTPHandler(service, HTTPConfig{
		OriginPolicy: OriginPolicy{
			AllowedOrigins:     []string{"http://client.example"},
			AllowMissingOrigin: false,
		},
	})
	if err != nil {
		t.Fatalf("NewHTTPHandler() error = %v, want nil", err)
	}
	return handler.Handler(), service
}

func sessionCookie(t *testing.T, response *http.Response) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if cookie.Name == DefaultSessionCookieName {
			return cookie
		}
	}
	t.Fatalf("session cookie not found in response cookies %+v", response.Cookies())
	return nil
}
