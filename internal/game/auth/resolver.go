package auth

import (
	"context"
	"fmt"
	"net/http"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

// StaticRealtimeSessionResolver adapts auth sessions to the existing realtime
// gateway interface. Phase 02 can replace the static route fields with a
// route-aware world resolver while keeping the auth lookup unchanged.
type StaticRealtimeSessionResolver struct {
	Service *Service
	WorldID foundation.WorldID
	ZoneID  foundation.ZoneID
}

// ResolveSession maps a server-side session id to command context. Player
// identity comes only from auth state.
func (resolver StaticRealtimeSessionResolver) ResolveSession(sessionID realtime.SessionID) (realtime.CommandContext, error) {
	if resolver.Service == nil {
		return realtime.CommandContext{}, ErrNilAuthService
	}
	authSessionID := SessionID(sessionID.String())
	resolved, err := resolver.Service.ResolveSessionID(context.Background(), authSessionID)
	if err != nil {
		return realtime.CommandContext{}, err
	}
	if resolver.WorldID.IsZero() || resolver.ZoneID.IsZero() {
		return realtime.CommandContext{}, foundation.NewDomainError(
			foundation.CodeUnauthenticated,
			"Authenticated world is required.",
			foundation.WithDetail("auth realtime resolver missing world or zone route"),
		)
	}
	return realtime.CommandContext{
		SessionID: realtime.SessionID(resolved.SessionID.String()),
		PlayerID:  resolved.PlayerID,
		WorldID:   resolver.WorldID,
		ZoneID:    resolver.ZoneID,
	}, nil
}

// ResolveCookie resolves the auth cookie from an HTTP upgrade request and
// validates origin before any gameplay state is sent.
func ResolveCookie(ctx context.Context, service *Service, cookieName string, originPolicy OriginPolicy, r *http.Request) (ResolvedSession, error) {
	if service == nil {
		return ResolvedSession{}, ErrNilAuthService
	}
	if err := originPolicy.CheckRequest(r); err != nil {
		return ResolvedSession{}, err
	}
	if cookieName == "" {
		cookieName = DefaultSessionCookieName
	}
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return ResolvedSession{}, authRequired(fmt.Errorf("cookie %q: %w", cookieName, err))
	}
	return service.ResolveToken(ctx, cookie.Value)
}
