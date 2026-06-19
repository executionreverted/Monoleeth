package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

func TestResolveCookieValidatesOriginAndCookieBeforeWebSocketState(t *testing.T) {
	service, _, _ := newTestAuthService(t)
	result, err := service.Register(nil, RegisterInput{Email: "pilot@example.com", Password: "correct-password", Callsign: "Frontier-01"})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	policy := OriginPolicy{AllowMissingOrigin: false}
	request := httptest.NewRequest(http.MethodGet, "/ws", nil)
	request.Header.Set("Origin", "http://example.com")
	request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: result.Token})

	resolved, err := ResolveCookie(request.Context(), service, "", policy, request)
	if err != nil {
		t.Fatalf("ResolveCookie() error = %v, want nil", err)
	}
	if resolved.PlayerID != result.Session.PlayerID || resolved.AccountID != result.Session.AccountID {
		t.Fatalf("resolved = %+v, want server-owned account/player from session", resolved)
	}

	crossSite := httptest.NewRequest(http.MethodGet, "/ws", nil)
	crossSite.Header.Set("Origin", "http://evil.example")
	crossSite.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: result.Token})
	if _, err := ResolveCookie(crossSite.Context(), service, "", policy, crossSite); !foundation.IsCode(err, foundation.CodeOriginDenied) {
		t.Fatalf("ResolveCookie(cross-site) error = %v, want %s", err, foundation.CodeOriginDenied)
	}

	missingCookie := httptest.NewRequest(http.MethodGet, "/ws", nil)
	missingCookie.Header.Set("Origin", "http://example.com")
	if _, err := ResolveCookie(missingCookie.Context(), service, "", policy, missingCookie); !foundation.IsCode(err, foundation.CodeAuthRequired) {
		t.Fatalf("ResolveCookie(missing cookie) error = %v, want %s", err, foundation.CodeAuthRequired)
	}
}

func TestStaticRealtimeSessionResolverMapsSessionIDToServerOwnedPlayer(t *testing.T) {
	service, _, _ := newTestAuthService(t)
	result, err := service.Register(nil, RegisterInput{Email: "pilot@example.com", Password: "correct-password", Callsign: "Frontier-01"})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	resolver := StaticRealtimeSessionResolver{
		Service: service,
		WorldID: "world-1",
		ZoneID:  "zone-1",
	}

	ctx, err := resolver.ResolveSession(realtime.SessionID(result.Session.SessionID.String()))
	if err != nil {
		t.Fatalf("ResolveSession() error = %v, want nil", err)
	}
	if ctx.SessionID != realtime.SessionID(result.Session.SessionID.String()) ||
		ctx.PlayerID != result.Session.PlayerID || ctx.WorldID != "world-1" || ctx.ZoneID != "zone-1" {
		t.Fatalf("command context = %+v, want server-owned session/player/world/zone", ctx)
	}

	if err := service.LogoutByToken(nil, result.Token); err != nil {
		t.Fatalf("LogoutByToken() error = %v, want nil", err)
	}
	if _, err := resolver.ResolveSession(realtime.SessionID(result.Session.SessionID.String())); !foundation.IsCode(err, foundation.CodeSessionRevoked) {
		t.Fatalf("ResolveSession(revoked) error = %v, want %s", err, foundation.CodeSessionRevoked)
	}
}
