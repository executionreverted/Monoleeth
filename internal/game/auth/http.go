package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"gameproject/internal/game/foundation"
)

const (
	DefaultSessionCookieName = "game_session"
	maxAuthRequestBytes      = 32 * 1024
)

// AuthRateLimitPosture documents the intended abuse posture for auth routes.
type AuthRateLimitPosture string

const (
	AuthRateLimitStrict AuthRateLimitPosture = "strict_auth_attempts"
)

// HTTPRouteSpec documents one auth HTTP operation.
type HTTPRouteSpec struct {
	Method           string
	Path             string
	RateLimitPosture AuthRateLimitPosture
}

// HTTPRouteSpecs returns the auth route metadata. The MVP documents rate-limit
// posture here; concrete enforcement can be wired at the server boundary.
func HTTPRouteSpecs() []HTTPRouteSpec {
	return []HTTPRouteSpec{
		{Method: http.MethodPost, Path: "/api/auth/register", RateLimitPosture: AuthRateLimitStrict},
		{Method: http.MethodPost, Path: "/api/auth/login", RateLimitPosture: AuthRateLimitStrict},
		{Method: http.MethodPost, Path: "/api/auth/logout", RateLimitPosture: AuthRateLimitStrict},
		{Method: http.MethodGet, Path: "/api/session", RateLimitPosture: AuthRateLimitStrict},
	}
}

// HTTPConfig configures cookie and browser-origin behavior.
type HTTPConfig struct {
	CookieName   string
	CookieSecure bool
	CookiePath   string
	OriginPolicy OriginPolicy
}

// HTTPHandler exposes auth endpoints.
type HTTPHandler struct {
	service *Service
	config  HTTPConfig
}

// NewHTTPHandler returns an auth HTTP handler.
func NewHTTPHandler(service *Service, config HTTPConfig) (*HTTPHandler, error) {
	if service == nil {
		return nil, ErrNilAuthService
	}
	if config.CookieName == "" {
		config.CookieName = DefaultSessionCookieName
	}
	if config.CookiePath == "" {
		config.CookiePath = "/"
	}
	return &HTTPHandler{service: service, config: config}, nil
}

// Handler returns a mux with the Phase 01 auth routes.
func (handler *HTTPHandler) Handler() http.Handler {
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

// RegisterRoutes registers auth routes on mux.
func (handler *HTTPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/register", handler.handleRegister)
	mux.HandleFunc("/api/auth/login", handler.handleLogin)
	mux.HandleFunc("/api/auth/logout", handler.handleLogout)
	mux.HandleFunc("/api/session", handler.handleSession)
}

func (handler *HTTPHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !handler.prepare(w, r, http.MethodPost, true) {
		return
	}
	var payload RegisterInput
	if !decodeJSON(w, r, &payload) {
		return
	}
	result, err := handler.service.Register(r.Context(), payload)
	if err != nil {
		writeError(w, err)
		return
	}
	handler.setSessionCookie(w, result.Token, result.Session.ExpiresAt)
	writeJSON(w, http.StatusCreated, result.Response)
}

func (handler *HTTPHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !handler.prepare(w, r, http.MethodPost, true) {
		return
	}
	var payload LoginInput
	if !decodeJSON(w, r, &payload) {
		return
	}
	result, err := handler.service.Login(r.Context(), payload)
	if err != nil {
		writeError(w, err)
		return
	}
	handler.setSessionCookie(w, result.Token, result.Session.ExpiresAt)
	writeJSON(w, http.StatusOK, result.Response)
}

func (handler *HTTPHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !handler.prepare(w, r, http.MethodPost, true) {
		return
	}
	if cookie, err := r.Cookie(handler.config.CookieName); err == nil {
		err = handler.service.LogoutByToken(r.Context(), cookie.Value)
		if err != nil && !foundation.IsCode(err, foundation.CodeAuthRequired) {
			writeError(w, err)
			return
		}
	}
	handler.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, unauthenticatedSession(handler.service.now()))
}

func (handler *HTTPHandler) handleSession(w http.ResponseWriter, r *http.Request) {
	if !handler.prepare(w, r, http.MethodGet, false) {
		return
	}
	response := unauthenticatedSession(handler.service.now())
	if cookie, err := r.Cookie(handler.config.CookieName); err == nil {
		resolved, err := handler.service.ResolveToken(r.Context(), cookie.Value)
		if err == nil {
			response = publicSession(resolved, handler.service.now())
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (handler *HTTPHandler) prepare(w http.ResponseWriter, r *http.Request, method string, requireOrigin bool) bool {
	handler.config.OriginPolicy.ApplyCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	if r.Method != method {
		writeError(w, foundation.NewDomainError(foundation.CodeInvalidPayload, "Method is not allowed."))
		return false
	}
	if requireOrigin {
		if err := handler.config.OriginPolicy.CheckRequest(r); err != nil {
			writeError(w, err)
			return false
		}
	}
	return true
}

func (handler *HTTPHandler) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	maxAge := int(expiresAt.Sub(handler.service.now()).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     handler.config.CookieName,
		Value:    token,
		Path:     handler.config.CookiePath,
		Expires:  expiresAt.UTC(),
		MaxAge:   maxAge,
		Secure:   handler.config.CookieSecure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (handler *HTTPHandler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     handler.config.CookieName,
		Value:    "",
		Path:     handler.config.CookiePath,
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		Secure:   handler.config.CookieSecure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, foundation.NewDomainError(
			foundation.CodeInvalidPayload,
			"Invalid JSON payload.",
			foundation.WithCause(err),
		))
		return false
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, foundation.NewDomainError(foundation.CodeInvalidPayload, "Invalid JSON payload."))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, err error) {
	var domainErr *foundation.DomainError
	if !errors.As(err, &domainErr) {
		domainErr = foundation.NewDomainError(foundation.CodeInternal, "Request failed.", foundation.WithCause(err))
	}
	status := statusForCode(domainErr.Code)
	writeJSON(w, status, struct {
		Error foundation.PublicError `json:"error"`
	}{
		Error: domainErr.Public(),
	})
}

func statusForCode(code foundation.Code) int {
	switch code {
	case foundation.CodeUnauthenticated, foundation.CodeAuthRequired, foundation.CodeSessionExpired, foundation.CodeSessionRevoked:
		return http.StatusUnauthorized
	case foundation.CodeForbidden, foundation.CodeOriginDenied:
		return http.StatusForbidden
	case foundation.CodeNotFound:
		return http.StatusNotFound
	case foundation.CodeInvalidPayload:
		return http.StatusBadRequest
	case foundation.CodeRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// CookieTokenFromRequest returns the raw cookie token from r.
func (handler *HTTPHandler) CookieTokenFromRequest(r *http.Request) (string, error) {
	if handler == nil {
		return "", ErrNilAuthService
	}
	cookie, err := r.Cookie(handler.config.CookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", authRequired(ErrSessionNotFound)
	}
	return cookie.Value, nil
}
