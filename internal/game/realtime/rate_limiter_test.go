package realtime

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestGatewayInMemoryRateLimiterBurstOverLimitReturnsRateLimited(t *testing.T) {
	clock := newManualRateLimitClock()
	limiter := newTestInMemoryRateLimiter(clock, OperationCombatUseSkill)
	calls := 0
	gateway := newTestGatewayWithLimiter(t, validSessionResolver(), limiter, map[Operation]CommandHandler{
		OperationCombatUseSkill: func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`{"accepted":true}`), nil
		},
	})

	allowed := gateway.HandleRequest("session-1", combatUseSkillRequest("request-bucket-allowed"))
	throttled := gateway.HandleRequest("session-1", combatUseSkillRequest("request-bucket-throttled"))

	if allowed.HasError {
		t.Fatalf("first response = %+v, want success", allowed)
	}
	if !throttled.HasError || throttled.Error.Error.Code != foundation.CodeRateLimited {
		t.Fatalf("second response = %+v, want %s", throttled, foundation.CodeRateLimited)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want first request only", calls)
	}
}

func TestGatewayInMemoryRateLimiterRetryAfterRefillExecutes(t *testing.T) {
	clock := newManualRateLimitClock()
	limiter := newTestInMemoryRateLimiter(clock, OperationCombatUseSkill)
	calls := 0
	gateway := newTestGatewayWithLimiter(t, validSessionResolver(), limiter, map[Operation]CommandHandler{
		OperationCombatUseSkill: func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`{"accepted":true}`), nil
		},
	})
	retryRequest := combatUseSkillRequest("request-bucket-retry")

	_ = gateway.HandleRequest("session-1", combatUseSkillRequest("request-bucket-primer"))
	denied := gateway.HandleRequest("session-1", retryRequest)
	clock.Advance(time.Second)
	retried := gateway.HandleRequest("session-1", retryRequest)

	if !denied.HasError || denied.Error.Error.Code != foundation.CodeRateLimited {
		t.Fatalf("denied response = %+v, want %s", denied, foundation.CodeRateLimited)
	}
	if retried.HasError {
		t.Fatalf("retried response = %+v, want success after refill", retried)
	}
	if calls != 2 {
		t.Fatalf("handler calls = %d, want primer plus retried request", calls)
	}
}

func TestInMemoryRateLimiterIsolatesOperationBuckets(t *testing.T) {
	clock := newManualRateLimitClock()
	limiter := newTestInMemoryRateLimiter(clock, OperationCombatUseSkill, OperationLootPickup)

	if err := limiter.AllowRealtimeRequest(rateLimitRequest(OperationCombatUseSkill, "session-1", "player-1")); err != nil {
		t.Fatalf("first combat request error = %v, want nil", err)
	}
	if err := limiter.AllowRealtimeRequest(rateLimitRequest(OperationCombatUseSkill, "session-1", "player-1")); err == nil {
		t.Fatal("second combat request error = nil, want rate limited")
	}
	if err := limiter.AllowRealtimeRequest(rateLimitRequest(OperationLootPickup, "session-1", "player-1")); err != nil {
		t.Fatalf("loot request error = %v, want isolated op bucket", err)
	}
}

func TestInMemoryRateLimiterIsolatesSessionBuckets(t *testing.T) {
	clock := newManualRateLimitClock()
	limiter := newTestInMemoryRateLimiter(clock, OperationCombatUseSkill)

	if err := limiter.AllowRealtimeRequest(rateLimitRequest(OperationCombatUseSkill, "session-1", "player-1")); err != nil {
		t.Fatalf("session-1 first request error = %v, want nil", err)
	}
	if err := limiter.AllowRealtimeRequest(rateLimitRequest(OperationCombatUseSkill, "session-1", "player-1")); err == nil {
		t.Fatal("session-1 second request error = nil, want rate limited")
	}
	if err := limiter.AllowRealtimeRequest(rateLimitRequest(OperationCombatUseSkill, "session-2", "player-1")); err != nil {
		t.Fatalf("session-2 request error = %v, want isolated session bucket", err)
	}
}

func TestInMemoryRateLimiterThrottlesEveryRegisteredOperation(t *testing.T) {
	clock := newManualRateLimitClock()
	limiter := NewInMemoryRealtimeLimiter(InMemoryRealtimeLimiterOptions{
		Clock: clock,
	})

	for operation, spec := range OperationRegistry() {
		t.Run(string(operation), func(t *testing.T) {
			if spec.Operation != operation {
				t.Fatalf("registry spec operation = %q, want %q", spec.Operation, operation)
			}
			if spec.RateLimitPosture == "" {
				t.Fatal("registry spec posture is empty")
			}
			request := rateLimitRequest(operation, "session-registered", "player-registered")
			request.RateLimitPosture = spec.RateLimitPosture
			limit := limiter.bucketFor(request)

			for attempt := 0; attempt < limit.Burst; attempt++ {
				if err := limiter.AllowRealtimeRequest(request); err != nil {
					t.Fatalf("allowed request %d/%d error = %v, want nil", attempt+1, limit.Burst, err)
				}
			}
			if err := limiter.AllowRealtimeRequest(request); !errors.Is(err, ErrRealtimeRateLimitExceeded) {
				t.Fatalf("over-limit request error = %v, want %v", err, ErrRealtimeRateLimitExceeded)
			}
		})
	}
}

func TestDefaultOperationBucketsCoverPhase04NamedRealtimeOperations(t *testing.T) {
	buckets := defaultRealtimeOperationBuckets()
	for _, operation := range []Operation{
		OperationCombatUseSkill,
		OperationLootPickup,
		OperationScanPulse,
		OperationMarketSearch,
		OperationQuestReroll,
		OperationChatSend,
		OperationPartyInvite,
		OperationPartyAccept,
		OperationPartyLeave,
	} {
		bucket, ok := buckets[operation]
		if !ok {
			t.Fatalf("default operation bucket missing for %q", operation)
		}
		if bucket.Burst < 1 || bucket.RefillEvery <= 0 {
			t.Fatalf("default operation bucket for %q = %+v, want positive burst/refill", operation, bucket)
		}
	}
	if bucket, ok := buckets[Operation("inventory.move")]; !ok || bucket.Burst < 1 || bucket.RefillEvery <= 0 {
		t.Fatalf("default operation bucket for inventory.move = %+v, present %v; want predeclared positive bucket", bucket, ok)
	}

	registry := OperationRegistry()
	for _, operation := range []Operation{OperationChatSend, OperationPartyInvite, OperationPartyAccept, OperationPartyLeave} {
		if _, ok := registry[operation]; !ok {
			t.Fatalf("operation %q missing from registry", operation)
		}
	}
}

func newTestInMemoryRateLimiter(clock *manualRateLimitClock, operations ...Operation) *InMemoryRealtimeLimiter {
	buckets := make(map[Operation]RealtimeRateLimitBucket, len(operations))
	for _, operation := range operations {
		buckets[operation] = RealtimeRateLimitBucket{
			Burst:       1,
			RefillEvery: time.Second,
		}
	}
	return NewInMemoryRealtimeLimiter(InMemoryRealtimeLimiterOptions{
		Clock:            clock,
		OperationBuckets: buckets,
	})
}

func rateLimitRequest(operation Operation, sessionID SessionID, playerID foundation.PlayerID) RateLimitRequest {
	return RateLimitRequest{
		SessionID:        sessionID,
		PlayerID:         playerID,
		WorldID:          "world-1",
		ZoneID:           "zone-1",
		Operation:        operation,
		RequestID:        foundation.RequestID("request-rate-limit"),
		RateLimitPosture: RateLimitPostureIntentBurst,
	}
}

type manualRateLimitClock struct {
	now time.Time
}

func newManualRateLimitClock() *manualRateLimitClock {
	return &manualRateLimitClock{now: time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)}
}

func (clock *manualRateLimitClock) Now() time.Time {
	return clock.now
}

func (clock *manualRateLimitClock) Advance(duration time.Duration) {
	clock.now = clock.now.Add(duration)
}
