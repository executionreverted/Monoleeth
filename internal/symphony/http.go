package symphony

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type HTTPServer struct {
	server       *http.Server
	orchestrator *Orchestrator
	logger       *Logger
}

func NewHTTPServer(host string, port int, orchestrator *Orchestrator, logger *Logger) *HTTPServer {
	mux := http.NewServeMux()
	s := &HTTPServer{orchestrator: orchestrator, logger: logger}
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/api/v1/state", s.handleState)
	mux.HandleFunc("/api/v1/refresh", s.handleRefresh)
	mux.HandleFunc("/api/v1/", s.handleIssue)
	s.server = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", host, port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *HTTPServer) Start() {
	go func() {
		s.logger.Info("starting Symphony HTTP API", "addr", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP API stopped with error", "error", err)
		}
	}()
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *HTTPServer) handleRoot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<html><head><title>Symphony Go</title></head><body><h1>Symphony Go</h1><p>Use <a href="/api/v1/state">/api/v1/state</a> for runtime state.</p></body></html>`))
}

func (s *HTTPServer) handleState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.orchestrator.Snapshot())
}

func (s *HTTPServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.orchestrator.ForcePoll()
	writeJSON(w, map[string]any{"ok": true})
}

func (s *HTTPServer) handleIssue(w http.ResponseWriter, r *http.Request) {
	identifier := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	if identifier == "" {
		http.NotFound(w, r)
		return
	}
	running, blocked, ok := s.orchestrator.IssueSnapshot(identifier)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if blocked != nil {
		writeJSON(w, blocked)
		return
	}
	writeJSON(w, running)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}
