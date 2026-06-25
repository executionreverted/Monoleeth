package server

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

func TestNewRuntimeDefaultRateLimitLimiterThrottlesGatewayBurst(t *testing.T) {
	runtime := newRateLimitRuntimeForTest(t, RuntimeConfig{})
	resolved := createRateLimitRuntimeSession(t, runtime, "runtime-rate-limit@example.com", "RuntimeLimit")
	sessionID := realtime.SessionID(resolved.SessionID.String())

	for index := 1; index <= 5; index++ {
		response := runtime.Gateway.HandleRequest(
			sessionID,
			debugSpawnNPCRequestForRateLimit(index, fmt.Sprintf("runtime_allowed_npc_%d", index)),
		)
		if response.HasError {
			t.Fatalf("allowed response %d = %+v, want success", index, response.Error)
		}
	}
	throttled := runtime.Gateway.HandleRequest(
		sessionID,
		debugSpawnNPCRequestForRateLimit(6, "runtime_throttled_npc"),
	)

	if !throttled.HasError || throttled.Error.Error.Code != foundation.CodeRateLimited {
		t.Fatalf("throttled response = %+v, want %s", throttled, foundation.CodeRateLimited)
	}
}

func TestServerWebSocketDefaultRateLimitReturnsRateLimitedAndSkipsMutation(t *testing.T) {
	gameServer, httpServer := newTestServer(t, true)
	defer httpServer.Close()
	cookie := registerPilotWithIdentity(t, httpServer, "ws-rate-limit@example.com", "WSLimit")
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	for index := 1; index <= 5; index++ {
		entityID := fmt.Sprintf("ws_allowed_npc_%d", index)
		writeText(t, conn, string(debugSpawnNPCRequestForRateLimit(index, entityID)))
		response := readResponseSkippingEvents(t, conn)
		if !response.OK {
			t.Fatalf("allowed websocket response %d = %+v, want success", index, response)
		}
	}
	if !runtimeHasEntityForRateLimit(t, gameServer, resolved.PlayerID, "ws_allowed_npc_1") {
		t.Fatal("allowed debug_spawn_npc did not create comparison entity")
	}

	writeText(t, conn, string(debugSpawnNPCRequestForRateLimit(6, "ws_throttled_npc")))
	throttled := readErrorSkippingEvents(t, conn)

	if throttled.Error.Code != foundation.CodeRateLimited {
		t.Fatalf("throttled websocket error = %+v, want %s", throttled.Error, foundation.CodeRateLimited)
	}
	if runtimeHasEntityForRateLimit(t, gameServer, resolved.PlayerID, "ws_throttled_npc") {
		t.Fatal("rate-limited debug_spawn_npc mutated world state")
	}
}

func TestServerConfigRateLimitLimiterSeamCanDisableDefault(t *testing.T) {
	gameServer := newRateLimitServerForTest(t, Config{
		DevMode:                true,
		disableRealtimeLimiter: true,
	})
	resolved := createResolvedRuntimeSession(t, gameServer, "disable-rate-limit@example.com", "DisableLimit")
	sessionID := realtime.SessionID(resolved.SessionID.String())

	for index := 1; index <= 6; index++ {
		response := gameServer.runtime.Gateway.HandleRequest(
			sessionID,
			debugSpawnNPCRequestForRateLimit(index, fmt.Sprintf("disabled_limiter_npc_%d", index)),
		)
		if response.HasError {
			t.Fatalf("disabled limiter response %d = %+v, want success", index, response.Error)
		}
	}
}

func TestServerConfigRateLimitLimiterSeamCanReplaceDefault(t *testing.T) {
	gameServer := newRateLimitServerForTest(t, Config{
		DevMode:         true,
		realtimeLimiter: denyRealtimeLimiter{},
	})
	resolved := createResolvedRuntimeSession(t, gameServer, "replace-rate-limit@example.com", "ReplaceLimit")

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		debugSpawnNPCRequestForRateLimit(1, "replaced_limiter_npc"),
	)

	if !response.HasError || response.Error.Error.Code != foundation.CodeRateLimited {
		t.Fatalf("replacement limiter response = %+v, want %s", response, foundation.CodeRateLimited)
	}
	if runtimeHasEntityForRateLimit(t, gameServer, resolved.PlayerID, "replaced_limiter_npc") {
		t.Fatal("replacement limiter denial mutated world state")
	}
}

func newRateLimitRuntimeForTest(t *testing.T, config RuntimeConfig) *Runtime {
	t.Helper()
	config.DevMode = true
	if config.SessionTTL <= 0 {
		config.SessionTTL = time.Hour
	}
	if config.TickDelta <= 0 {
		config.TickDelta = 50 * time.Millisecond
	}
	if config.WorldID == "" {
		config.WorldID = "world-1"
	}
	if config.ContentRepository == nil {
		config.ContentRepository = &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)}
	}
	if config.Passwords == nil {
		config.Passwords = auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16}
	}
	runtime, err := NewRuntime(config)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Fatalf("runtime Close() error = %v, want nil", err)
		}
	})
	return runtime
}

func newRateLimitServerForTest(t *testing.T, config Config) *Server {
	t.Helper()
	config.AllowedOrigins = []string{testOrigin}
	config.SessionTTL = time.Hour
	config.TickDelta = 50 * time.Millisecond
	config.ContentRepository = &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)}
	config.PasswordHasher = auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16}
	gameServer, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		if err := gameServer.Shutdown(context.Background()); err != nil {
			t.Fatalf("server Shutdown() error = %v, want nil", err)
		}
	})
	return gameServer
}

func createRateLimitRuntimeSession(t *testing.T, runtime *Runtime, email string, callsign string) auth.ResolvedSession {
	t.Helper()
	result, err := runtime.Auth.Register(context.Background(), auth.RegisterInput{
		Email:    email,
		Password: "correct-password",
		Callsign: callsign,
	})
	if err != nil {
		t.Fatalf("register runtime session: %v", err)
	}
	if err := runtime.ensurePlayerSession(result.Session); err != nil {
		t.Fatalf("ensure runtime player session: %v", err)
	}
	return result.Session
}

func debugSpawnNPCRequestForRateLimit(index int, entityID string) []byte {
	return []byte(fmt.Sprintf(
		`{"request_id":"request-rate-limit-%d","op":"debug_spawn_npc","payload":{"entity_id":%q,"position":{"x":%d,"y":0}},"client_seq":%d,"v":1}`,
		index,
		entityID,
		index,
		index,
	))
}

func runtimeHasEntityForRateLimit(t *testing.T, gameServer *Server, playerID foundation.PlayerID, entityID string) bool {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	instance, _, err := gameServer.runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		t.Fatalf("activeMapInstanceLocked() error = %v, want nil", err)
	}
	_, ok := instance.Worker.Entity(foundation.EntityID(entityID))
	return ok
}

type denyRealtimeLimiter struct{}

func (denyRealtimeLimiter) AllowRealtimeRequest(realtime.RateLimitRequest) error {
	return errors.New("test realtime limiter denied")
}
