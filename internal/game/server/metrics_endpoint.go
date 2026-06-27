package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"gameproject/internal/game/observability"
)

func (server *Server) serveMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if server == nil || !server.metricsRequestAuthorized(r) {
		http.Error(w, "metrics unavailable", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", observability.PrometheusContentType)
	w.WriteHeader(http.StatusOK)
	if server.runtime == nil || server.runtime.Metrics == nil {
		return
	}
	_, _ = w.Write([]byte(observability.PrometheusText(server.runtime.Metrics.Snapshot())))
}

func (server *Server) metricsRequestAuthorized(r *http.Request) bool {
	token := server.config.MetricsToken
	if token == "" {
		return true
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}
