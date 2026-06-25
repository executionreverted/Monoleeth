package realtime

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
)

func TestGatewayResolvesPlayerFromSessionNotPayload(t *testing.T) {
	resolver := staticSessionResolver{
		"session-1": {
			PlayerID: foundation.PlayerID("server-player"),
			WorldID:  foundation.WorldID("world-1"),
			ZoneID:   foundation.ZoneID("zone-1"),
		},
	}
	var handlerContext CommandContext
	gateway := newTestGateway(t, resolver, map[Operation]CommandHandler{
		OperationMoveTo: func(ctx CommandContext, request RequestEnvelope) (json.RawMessage, error) {
			handlerContext = ctx
			var payload struct {
				PlayerID string  `json:"player_id"`
				WorldID  string  `json:"world_id"`
				ZoneID   string  `json:"zone_id"`
				X        float64 `json:"x"`
				Y        float64 `json:"y"`
			}
			if err := json.Unmarshal(request.Payload, &payload); err != nil {
				t.Fatalf("unmarshal request payload: %v", err)
			}
			if payload.PlayerID != "spoofed-player" {
				t.Fatalf("payload player_id = %q, want spoofed-player", payload.PlayerID)
			}
			if payload.WorldID != "spoofed-world" || payload.ZoneID != "spoofed-zone" {
				t.Fatalf("payload world/zone = %q/%q, want spoofed payload identity", payload.WorldID, payload.ZoneID)
			}
			return json.RawMessage(`{"accepted":true}`), nil
		},
	})

	response := gateway.HandleRequest("session-1", []byte(`{"request_id":"request-1","op":"move_to","payload":{"player_id":"spoofed-player","world_id":"spoofed-world","zone_id":"spoofed-zone","x":10,"y":20},"client_seq":7,"v":1}`))

	if response.HasError {
		t.Fatalf("HandleRequest() error response = %+v, want success", response.Error)
	}
	if handlerContext.SessionID != "session-1" || handlerContext.PlayerID != "server-player" ||
		handlerContext.WorldID != "world-1" || handlerContext.ZoneID != "zone-1" {
		t.Fatalf("handler context = %+v, want server-resolved session/player/world/zone", handlerContext)
	}
	if got := string(response.Response.Payload); got != `{"accepted":true}` {
		t.Fatalf("response payload = %s, want accepted true", got)
	}
}

func TestGatewayRejectsUnknownSessionBeforeHandler(t *testing.T) {
	var called bool
	gateway := newTestGateway(t, staticSessionResolver{}, map[Operation]CommandHandler{
		OperationMoveTo: func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
			called = true
			return json.RawMessage(`{"accepted":true}`), nil
		},
	})

	response := gateway.HandleRequest("missing-session", []byte(`{"request_id":"request-2","op":"move_to","payload":{"x":10,"y":20},"client_seq":8,"v":1}`))

	if !response.HasError {
		t.Fatalf("HandleRequest() HasError = false, want true")
	}
	if response.Error.Error.Code != foundation.CodeUnauthenticated {
		t.Fatalf("error code = %s, want %s", response.Error.Error.Code, foundation.CodeUnauthenticated)
	}
	if called {
		t.Fatal("handler called before authenticated session resolution")
	}
}

func TestGatewayLimiterDenialReturnsRateLimited(t *testing.T) {
	gateway := newTestGatewayWithLimiter(t, validSessionResolver(), &recordingRateLimiter{deny: true}, map[Operation]CommandHandler{
		OperationCombatUseSkill: func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
			return json.RawMessage(`{"accepted":true}`), nil
		},
	})

	response := gateway.HandleRequest("session-1", combatUseSkillRequest("request-limiter-denied"))

	if !response.HasError || response.Error.Error.Code != foundation.CodeRateLimited {
		t.Fatalf("HandleRequest() = %+v, want %s", response, foundation.CodeRateLimited)
	}
}

func TestGatewayLimiterDenialSkipsHandler(t *testing.T) {
	calls := 0
	gateway := newTestGatewayWithLimiter(t, validSessionResolver(), &recordingRateLimiter{deny: true}, map[Operation]CommandHandler{
		OperationCombatUseSkill: func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`{"accepted":true}`), nil
		},
	})

	_ = gateway.HandleRequest("session-1", combatUseSkillRequest("request-limiter-skip-handler"))

	if calls != 0 {
		t.Fatalf("handler calls = %d, want 0", calls)
	}
}

func TestGatewayLimiterReceivesResolvedRequestContextAndMutationPosture(t *testing.T) {
	limiter := &recordingRateLimiter{}
	gateway := newTestGatewayWithLimiter(t, validSessionResolver(), limiter, map[Operation]CommandHandler{
		OperationCombatUseSkill: func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
			return json.RawMessage(`{"accepted":true}`), nil
		},
	})

	_ = gateway.HandleRequest("session-1", combatUseSkillRequest("request-limiter-posture"))

	want := RateLimitRequest{
		SessionID:        "session-1",
		PlayerID:         "player-1",
		WorldID:          "world-1",
		ZoneID:           "zone-1",
		Operation:        OperationCombatUseSkill,
		RequestID:        "request-limiter-posture",
		RateLimitPosture: RateLimitPostureIntentBurst,
	}
	if len(limiter.requests) != 1 || limiter.requests[0] != want {
		t.Fatalf("limiter requests = %+v, want [%+v]", limiter.requests, want)
	}
}

func TestGatewayWithoutLimiterExecutesHandler(t *testing.T) {
	calls := 0
	gateway := newTestGateway(t, validSessionResolver(), map[Operation]CommandHandler{
		OperationCombatUseSkill: func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`{"accepted":true}`), nil
		},
	})

	_ = gateway.HandleRequest("session-1", combatUseSkillRequest("request-no-limiter"))

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestGatewayLimiterDeniedRetryCanExecuteAfterAllow(t *testing.T) {
	calls := 0
	limiter := &recordingRateLimiter{deny: true}
	gateway := newTestGatewayWithLimiter(t, validSessionResolver(), limiter, map[Operation]CommandHandler{
		OperationCombatUseSkill: func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`{"accepted":true}`), nil
		},
	})
	request := combatUseSkillRequest("request-limiter-retry")

	denied := gateway.HandleRequest("session-1", request)
	limiter.deny = false
	allowed := gateway.HandleRequest("session-1", request)

	if !denied.HasError || allowed.HasError || calls != 1 {
		t.Fatalf("denied/allowed/calls = %+v / %+v / %d, want retry execute once after allow", denied, allowed, calls)
	}
}

func TestGatewayCachesDuplicateRequestPerSession(t *testing.T) {
	resolver := staticSessionResolver{
		"session-1": {PlayerID: "player-1", WorldID: "world-1", ZoneID: "zone-1"},
	}
	calls := 0
	gateway := newTestGateway(t, resolver, map[Operation]CommandHandler{
		OperationDebugSnapshot: func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`{"snapshot":1}`), nil
		},
	})
	request := []byte(`{"request_id":"request-3","op":"debug_snapshot","payload":{},"client_seq":9,"v":1}`)

	first := gateway.HandleRequest("session-1", request)
	second := gateway.HandleRequest("session-1", request)

	if first.HasError || second.HasError {
		t.Fatalf("responses = %+v / %+v, want successes", first, second)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	if string(second.Response.Payload) != `{"snapshot":1}` {
		t.Fatalf("duplicate payload = %s, want cached snapshot", second.Response.Payload)
	}
}

func TestGatewaySameRequestIDDifferentOpReturnsReplayMismatch(t *testing.T) {
	var calls int
	gateway := newReplayMismatchGateway(t, &calls)

	first := gateway.HandleRequest("session-1", []byte(`{"request_id":"request-replay-op","op":"debug_snapshot","payload":{},"client_seq":1,"v":1}`))
	mismatch := gateway.HandleRequest("session-1", []byte(`{"request_id":"request-replay-op","op":"world.snapshot","payload":{},"client_seq":2,"v":1}`))

	if first.HasError {
		t.Fatalf("first response = %+v, want success", first)
	}
	requireReplayMismatch(t, mismatch)
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestGatewaySameRequestIDDifferentPayloadReturnsReplayMismatch(t *testing.T) {
	var calls int
	gateway := newReplayMismatchGateway(t, &calls)

	first := gateway.HandleRequest("session-1", []byte(`{"request_id":"request-replay-payload","op":"debug_snapshot","payload":{"page":1},"client_seq":1,"v":1}`))
	mismatch := gateway.HandleRequest("session-1", []byte(`{"request_id":"request-replay-payload","op":"debug_snapshot","payload":{"page":2},"client_seq":2,"v":1}`))

	if first.HasError {
		t.Fatalf("first response = %+v, want success", first)
	}
	requireReplayMismatch(t, mismatch)
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestGatewaySameRequestIDDifferentVersionReturnsReplayMismatch(t *testing.T) {
	var calls int
	gateway := newReplayMismatchGateway(t, &calls)

	first := gateway.HandleRequest("session-1", []byte(`{"request_id":"request-replay-version","op":"debug_snapshot","payload":{},"client_seq":1}`))
	mismatch := gateway.HandleRequest("session-1", []byte(`{"request_id":"request-replay-version","op":"debug_snapshot","payload":{},"client_seq":2,"v":1}`))

	if first.HasError {
		t.Fatalf("first response = %+v, want success", first)
	}
	requireReplayMismatch(t, mismatch)
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestGatewayRejectsMissingWorldOrZoneBeforeHandler(t *testing.T) {
	tests := []struct {
		name    string
		context CommandContext
	}{
		{
			name:    "missing world",
			context: CommandContext{PlayerID: "player-1", ZoneID: "zone-1"},
		},
		{
			name:    "missing zone",
			context: CommandContext{PlayerID: "player-1", WorldID: "world-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			gateway := newTestGateway(t, staticSessionResolver{"session-1": tt.context}, map[Operation]CommandHandler{
				OperationMoveTo: func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
					called = true
					return json.RawMessage(`{"accepted":true}`), nil
				},
			})

			response := gateway.HandleRequest("session-1", []byte(`{"request_id":"request-missing-route","op":"move_to","payload":{"x":10,"y":20},"client_seq":10,"v":1}`))

			if !response.HasError || response.Error.Error.Code != foundation.CodeUnauthenticated {
				t.Fatalf("HandleRequest() = %+v, want unauthenticated missing route identity", response)
			}
			if called {
				t.Fatal("handler called without server-resolved world/zone identity")
			}
		})
	}
}

func TestGatewayRejectsResolverSessionMismatch(t *testing.T) {
	gateway := newTestGateway(t, staticSessionResolver{
		"session-1": {SessionID: "other-session", PlayerID: "player-1", WorldID: "world-1", ZoneID: "zone-1"},
	}, map[Operation]CommandHandler{
		OperationDebugSnapshot: func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
			return json.RawMessage(`{"snapshot":1}`), nil
		},
	})

	response := gateway.HandleRequest("session-1", []byte(`{"request_id":"request-4","op":"debug_snapshot","payload":{},"client_seq":10,"v":1}`))

	if !response.HasError || response.Error.Error.Code != foundation.CodeUnauthenticated {
		t.Fatalf("HandleRequest() = %+v, want unauthenticated session mismatch", response)
	}
}

func TestGatewayConstructorsRejectNilDependencies(t *testing.T) {
	_, err := NewGateway(GatewayOptions{})
	if !errors.Is(err, ErrNilSessionResolver) {
		t.Fatalf("NewGateway(nil resolver) error = %v, want ErrNilSessionResolver", err)
	}
}

func newTestGateway(t *testing.T, resolver SessionResolver, handlers map[Operation]CommandHandler) *Gateway {
	t.Helper()
	return newTestGatewayWithLimiter(t, resolver, nil, handlers)
}

func newTestGatewayWithLimiter(t *testing.T, resolver SessionResolver, limiter RateLimiter, handlers map[Operation]CommandHandler) *Gateway {
	t.Helper()
	gateway, err := NewGateway(GatewayOptions{
		Clock:    &steppingClock{now: time.Date(2026, 6, 18, 18, 0, 0, 0, time.UTC), step: time.Millisecond},
		Sessions: resolver,
		Executor: ObservedCommandExecutor{
			Logger:  observability.NewMemoryCommandLogger(),
			Metrics: observability.NewMetricRecorder(),
		},
		Limiter:  limiter,
		Handlers: handlers,
	})
	if err != nil {
		t.Fatalf("NewGateway() error = %v, want nil", err)
	}
	return gateway
}

func newReplayMismatchGateway(t *testing.T, calls *int) *Gateway {
	t.Helper()
	resolver := staticSessionResolver{
		"session-1": {PlayerID: "player-1", WorldID: "world-1", ZoneID: "zone-1"},
	}
	handler := func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
		*calls = *calls + 1
		return json.RawMessage(`{"snapshot":1}`), nil
	}
	return newTestGateway(t, resolver, map[Operation]CommandHandler{
		OperationDebugSnapshot: handler,
		OperationWorldSnapshot: handler,
	})
}

func requireReplayMismatch(t *testing.T, response CachedResponse) {
	t.Helper()
	if !response.HasError {
		t.Fatalf("response = %+v, want replay mismatch error", response)
	}
	if response.Error.Error.Code != foundation.CodeRequestReplayMismatch {
		t.Fatalf("error code = %s, want %s", response.Error.Error.Code, foundation.CodeRequestReplayMismatch)
	}
	if response.Error.Error.Retryable {
		t.Fatal("replay mismatch error was retryable")
	}
}

type staticSessionResolver map[SessionID]CommandContext

func (resolver staticSessionResolver) ResolveSession(sessionID SessionID) (CommandContext, error) {
	ctx, ok := resolver[sessionID]
	if !ok {
		return CommandContext{}, foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated session is required.")
	}
	return ctx, nil
}

type recordingRateLimiter struct {
	deny     bool
	requests []RateLimitRequest
}

func (limiter *recordingRateLimiter) AllowRealtimeRequest(request RateLimitRequest) error {
	limiter.requests = append(limiter.requests, request)
	if limiter.deny {
		return errors.New("rate limit exceeded")
	}
	return nil
}

func validSessionResolver() staticSessionResolver {
	return staticSessionResolver{
		"session-1": {
			PlayerID: "player-1",
			WorldID:  "world-1",
			ZoneID:   "zone-1",
		},
	}
}

func combatUseSkillRequest(requestID foundation.RequestID) []byte {
	return []byte(`{"request_id":"` + string(requestID) + `","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"npc-1"},"client_seq":11,"v":1}`)
}
