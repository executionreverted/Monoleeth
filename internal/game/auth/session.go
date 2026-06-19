package auth

import (
	"context"
	"errors"
	"fmt"

	"gameproject/internal/game/foundation"
)

// ResolveToken resolves an opaque cookie token to server-owned session context.
func (service *Service) ResolveToken(ctx context.Context, rawToken string) (ResolvedSession, error) {
	if service == nil {
		return ResolvedSession{}, ErrNilAuthService
	}
	hash, err := tokenHash(rawToken)
	if err != nil {
		return ResolvedSession{}, authRequired(err)
	}
	session, err := service.store.SessionByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return ResolvedSession{}, authRequired(err)
		}
		return ResolvedSession{}, err
	}
	return service.resolveStoredSession(ctx, session)
}

// ResolveSessionID resolves a server-owned session id and re-checks revocation
// and expiry. Future WebSocket loops can call this before each command.
func (service *Service) ResolveSessionID(ctx context.Context, sessionID SessionID) (ResolvedSession, error) {
	if service == nil {
		return ResolvedSession{}, ErrNilAuthService
	}
	if err := sessionID.Validate(); err != nil {
		return ResolvedSession{}, authRequired(err)
	}
	session, err := service.store.SessionByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return ResolvedSession{}, authRequired(err)
		}
		return ResolvedSession{}, err
	}
	return service.resolveStoredSession(ctx, session)
}

// LogoutByToken revokes the session addressed by rawToken.
func (service *Service) LogoutByToken(ctx context.Context, rawToken string) error {
	if service == nil {
		return ErrNilAuthService
	}
	hash, err := tokenHash(rawToken)
	if err != nil {
		return authRequired(err)
	}
	session, err := service.store.SessionByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return authRequired(err)
		}
		return err
	}
	if session.RevokedAt != nil {
		return nil
	}
	return service.store.RevokeSession(ctx, session.ID, service.now())
}

// PublicSessionByToken returns the public session shape or an unauthenticated
// shape for missing/invalid/expired sessions.
func (service *Service) PublicSessionByToken(ctx context.Context, rawToken string) (PublicSessionResponse, error) {
	if service == nil {
		return PublicSessionResponse{}, ErrNilAuthService
	}
	resolved, err := service.ResolveToken(ctx, rawToken)
	if err != nil {
		return unauthenticatedSession(service.now()), nil
	}
	return publicSession(resolved, service.now()), nil
}

func (service *Service) createSession(ctx context.Context, account Account, player PlayerProfile) (AuthResult, error) {
	now := service.now()
	for attempt := 0; attempt < 3; attempt++ {
		rawToken, err := service.tokens.NewSessionToken()
		if err != nil {
			return AuthResult{}, err
		}
		hash, err := tokenHash(rawToken)
		if err != nil {
			return AuthResult{}, err
		}
		id, err := service.tokens.NewID("sess")
		if err != nil {
			return AuthResult{}, err
		}
		session := Session{
			ID:        SessionID(id),
			AccountID: account.ID,
			PlayerID:  player.ID,
			TokenHash: hash,
			Roles:     cloneRoles(account.Roles),
			CreatedAt: now,
			ExpiresAt: now.Add(service.sessionTTL),
		}
		if err := service.store.InsertSession(ctx, session); err != nil {
			if errors.Is(err, ErrDuplicateSession) {
				continue
			}
			return AuthResult{}, err
		}
		resolved := ResolvedSession{
			SessionID: session.ID,
			AccountID: account.ID,
			PlayerID:  player.ID,
			Email:     account.Email,
			Callsign:  player.Callsign,
			Roles:     cloneRoles(account.Roles),
			ExpiresAt: session.ExpiresAt,
		}
		return AuthResult{
			Token:    rawToken,
			Session:  resolved,
			Response: publicSession(resolved, now),
		}, nil
	}
	return AuthResult{}, fmt.Errorf("create session: %w", ErrDuplicateSession)
}

func (service *Service) resolveStoredSession(ctx context.Context, session Session) (ResolvedSession, error) {
	if session.RevokedAt != nil {
		return ResolvedSession{}, foundation.NewDomainError(
			foundation.CodeSessionRevoked,
			"Session has been revoked.",
			foundation.WithCause(ErrSessionRevoked),
		)
	}
	if !service.now().Before(session.ExpiresAt) {
		return ResolvedSession{}, foundation.NewDomainError(
			foundation.CodeSessionExpired,
			"Session has expired.",
			foundation.WithCause(ErrSessionExpired),
		)
	}
	account, player, err := service.store.AccountByID(ctx, session.AccountID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return ResolvedSession{}, authRequired(err)
		}
		return ResolvedSession{}, err
	}
	if player.ID != session.PlayerID {
		return ResolvedSession{}, foundation.NewDomainError(
			foundation.CodeInternal,
			"Session account is invalid.",
			foundation.WithDetail("session player does not match account profile"),
		)
	}
	return ResolvedSession{
		SessionID: session.ID,
		AccountID: account.ID,
		PlayerID:  player.ID,
		Email:     account.Email,
		Callsign:  player.Callsign,
		Roles:     cloneRoles(account.Roles),
		ExpiresAt: session.ExpiresAt,
	}, nil
}
