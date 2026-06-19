package auth

import (
	"net/http"
	"net/url"
	"strings"

	"gameproject/internal/game/foundation"
)

// OriginPolicy controls browser credential safety for cookie-authenticated
// requests and WebSocket upgrades.
type OriginPolicy struct {
	AllowedOrigins     []string
	AllowMissingOrigin bool
}

// CheckRequest validates the Origin header against same-origin or configured
// first-party origins.
func (policy OriginPolicy) CheckRequest(r *http.Request) error {
	if r == nil {
		return originDenied("missing request")
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		if policy.AllowMissingOrigin {
			return nil
		}
		return originDenied("missing origin")
	}
	normalized, err := normalizeOrigin(origin)
	if err != nil {
		return originDenied("invalid origin")
	}
	if normalized == requestOrigin(r) {
		return nil
	}
	for _, allowed := range policy.AllowedOrigins {
		if normalizedAllowed, err := normalizeOrigin(allowed); err == nil && normalizedAllowed == normalized {
			return nil
		}
	}
	return originDenied("origin not allowed")
}

// ApplyCORS writes credentialed CORS headers only for allowed first-party
// origins. It never emits a wildcard origin.
func (policy OriginPolicy) ApplyCORS(w http.ResponseWriter, r *http.Request) {
	if w == nil || r == nil {
		return
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return
	}
	if policy.CheckRequest(r) != nil {
		return
	}
	w.Header().Add("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Headers", "content-type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
}

func normalizeOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrOriginDenied
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrOriginDenied
	}
	return scheme + "://" + strings.ToLower(parsed.Host), nil
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + strings.ToLower(r.Host)
}

func originDenied(detail string) *foundation.DomainError {
	return foundation.NewDomainError(
		foundation.CodeOriginDenied,
		"Origin is not allowed.",
		foundation.WithCause(ErrOriginDenied),
		foundation.WithDetail(detail),
	)
}
