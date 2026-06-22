package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/realtime"
)

// Server owns HTTP routing and the concrete WebSocket transport.
type Server struct {
	config            Config
	runtime           *Runtime
	handler           http.Handler
	server            *http.Server
	conns             sync.Map
	connMu            sync.Mutex
	sessionConnCounts map[auth.SessionID]int
}

type clientConnection struct {
	conn      *websocket.Conn
	sessionID auth.SessionID
	mu        sync.Mutex
}

// New returns a concrete game server.
func New(config Config) (*Server, error) {
	config = config.withDefaults()
	if config.E2EPlanetClaimSeed && !config.DevMode {
		return nil, fmt.Errorf("%s requires %s=true", EnvE2EPlanetClaimSeed, EnvDevMode)
	}
	if config.E2ERouteSeed && !config.DevMode {
		return nil, fmt.Errorf("%s requires %s=true", EnvE2ERouteSeed, EnvDevMode)
	}
	runtime, err := NewRuntime(RuntimeConfig{
		Clock:              config.Clock,
		SessionTTL:         config.SessionTTL,
		TickDelta:          config.TickDelta,
		WorldID:            config.WorldID,
		ZoneID:             config.ZoneID,
		DevMode:            config.DevMode,
		E2EPlanetClaimSeed: config.E2EPlanetClaimSeed,
		E2ERouteSeed:       config.E2ERouteSeed,
		AdminSeed:          config.AdminSeed,
		Passwords:          config.PasswordHasher,
	})
	if err != nil {
		return nil, err
	}
	authHandler, err := auth.NewHTTPHandler(runtime.Auth, auth.HTTPConfig{
		CookieSecure: config.CookieSecure,
		OriginPolicy: config.originPolicy(),
	})
	if err != nil {
		return nil, err
	}
	gameServer := &Server{
		config:            config,
		runtime:           runtime,
		sessionConnCounts: make(map[auth.SessionID]int),
	}
	mux := http.NewServeMux()
	authHandler.RegisterRoutes(mux)
	mux.HandleFunc("/ws", gameServer.serveWebSocket)
	mux.HandleFunc("/healthz", gameServer.serveHealth)
	gameServer.handler = mux
	gameServer.server = &http.Server{
		Addr:              config.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return gameServer, nil
}

// Handler returns the configured HTTP handler for tests or embedding.
func (server *Server) Handler() http.Handler {
	return server.handler
}

// Runtime returns the composed single-process runtime.
func (server *Server) Runtime() *Runtime {
	return server.runtime
}

// Run starts the worker tick lifecycle and HTTP server until ctx is canceled.
func (server *Server) Run(ctx context.Context) error {
	if server == nil || server.server == nil {
		return errors.New("nil game server")
	}
	server.runtime.StartWithEventSink(ctx, func(sessionID auth.SessionID, events []realtime.EventEnvelope) {
		server.writeEventsToSession(sessionID, events)
	})
	errc := make(chan error, 1)
	go func() {
		err := server.server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errc <- err
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

// Shutdown closes the HTTP server and active WebSocket sessions.
func (server *Server) Shutdown(ctx context.Context) error {
	if server == nil {
		return nil
	}
	server.conns.Range(func(key, _ any) bool {
		if client, ok := key.(*clientConnection); ok {
			_ = client.conn.Close(websocket.StatusGoingAway, "server shutdown")
		}
		return true
	})
	if server.server == nil {
		return nil
	}
	return server.server.Shutdown(ctx)
}

func (server *Server) serveHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}
