package auth

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"gameproject/internal/game/foundation"
)

// Email is a normalized login email address.
type Email string

// Role is a server-owned account role.
type Role string

const (
	RoleAdmin Role = "admin"
)

// SessionID identifies an authenticated account session.
type SessionID string

// Account stores server-owned login and role state.
type Account struct {
	ID           foundation.AccountID
	Email        Email
	PasswordHash PasswordHash
	Roles        []Role
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PlayerProfile stores the public player identity created with an account.
type PlayerProfile struct {
	ID        foundation.PlayerID
	AccountID foundation.AccountID
	Callsign  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Session stores server-owned session state. TokenHash is a hash of the opaque
// cookie token; the raw token is never stored here.
type Session struct {
	ID        SessionID
	AccountID foundation.AccountID
	PlayerID  foundation.PlayerID
	TokenHash string
	Roles     []Role
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// ResolvedSession is the server-owned auth context used by HTTP and future
// WebSocket transports.
type ResolvedSession struct {
	SessionID SessionID
	AccountID foundation.AccountID
	PlayerID  foundation.PlayerID
	Email     Email
	Callsign  string
	Roles     []Role
	ExpiresAt time.Time
}

// PublicAccount is safe to return to browsers.
type PublicAccount struct {
	Email string `json:"email"`
	Admin bool   `json:"admin"`
}

// PublicPlayer is safe to return to browsers.
type PublicPlayer struct {
	Callsign string `json:"callsign"`
}

// PublicSessionResponse is the JSON shape for auth endpoints.
type PublicSessionResponse struct {
	Authenticated bool           `json:"authenticated"`
	Account       *PublicAccount `json:"account,omitempty"`
	Player        *PublicPlayer  `json:"player,omitempty"`
	Roles         []string       `json:"roles,omitempty"`
	ExpiresAt     int64          `json:"expires_at,omitempty"`
	ServerTime    int64          `json:"server_time"`
}

// String returns the stable session id representation.
func (id SessionID) String() string {
	return string(id)
}

// Validate reports whether id can identify a server-side auth session.
func (id SessionID) Validate() error {
	value := string(id)
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || strings.Contains(value, ":") {
		return fmt.Errorf("session id %q: %w", id, ErrInvalidSessionToken)
	}
	return nil
}

// IsZero reports whether id is empty.
func (id SessionID) IsZero() bool {
	return id == ""
}

// NormalizeEmail trims and lowercases a login email address.
func NormalizeEmail(raw string) (Email, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" || len(normalized) > 254 {
		return "", ErrInvalidEmail
	}
	if strings.ContainsAny(normalized, "\r\n\t ") {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(normalized)
	if err != nil || addr.Address != normalized || addr.Name != "" {
		return "", ErrInvalidEmail
	}
	local, domain, ok := strings.Cut(normalized, "@")
	if !ok || local == "" || domain == "" || strings.Contains(domain, "..") {
		return "", ErrInvalidEmail
	}
	return Email(normalized), nil
}

// String returns the normalized email value.
func (email Email) String() string {
	return string(email)
}

// ValidateCallsign normalizes and validates a player callsign.
func ValidateCallsign(raw string) (string, error) {
	callsign := strings.TrimSpace(raw)
	if len(callsign) < 2 || len(callsign) > 32 {
		return "", ErrInvalidCallsign
	}
	for _, r := range callsign {
		if unicode.IsControl(r) {
			return "", ErrInvalidCallsign
		}
	}
	return callsign, nil
}

func publicSession(resolved ResolvedSession, serverTime time.Time) PublicSessionResponse {
	roles := roleStrings(resolved.Roles)
	return PublicSessionResponse{
		Authenticated: true,
		Account: &PublicAccount{
			Email: resolved.Email.String(),
			Admin: hasRole(resolved.Roles, RoleAdmin),
		},
		Player: &PublicPlayer{
			Callsign: resolved.Callsign,
		},
		Roles:      roles,
		ExpiresAt:  resolved.ExpiresAt.UTC().UnixMilli(),
		ServerTime: serverTime.UTC().UnixMilli(),
	}
}

func unauthenticatedSession(serverTime time.Time) PublicSessionResponse {
	return PublicSessionResponse{
		Authenticated: false,
		ServerTime:    serverTime.UTC().UnixMilli(),
	}
}

func hasRole(roles []Role, want Role) bool {
	for _, role := range roles {
		if role == want {
			return true
		}
	}
	return false
}

func roleStrings(roles []Role) []string {
	if len(roles) == 0 {
		return nil
	}
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, string(role))
	}
	return out
}

func mergeRoles(existing []Role, additions ...Role) []Role {
	seen := make(map[Role]struct{}, len(existing)+len(additions))
	merged := make([]Role, 0, len(existing)+len(additions))
	for _, role := range existing {
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		merged = append(merged, role)
	}
	for _, role := range additions {
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		merged = append(merged, role)
	}
	return merged
}

func cloneRoles(roles []Role) []Role {
	if len(roles) == 0 {
		return nil
	}
	cloned := make([]Role, len(roles))
	copy(cloned, roles)
	return cloned
}
