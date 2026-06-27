package realtime

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

var ErrRealtimeRateLimitExceeded = errors.New("realtime rate limit exceeded")

// RateLimiter gates realtime requests before command handlers mutate gameplay
// state.
type RateLimiter interface {
	AllowRealtimeRequest(RateLimitRequest) error
}

// RealtimeRateLimitBucket defines one in-process token bucket.
type RealtimeRateLimitBucket struct {
	Burst       int
	RefillEvery time.Duration
}

// InMemoryRealtimeLimiterOptions configures the process-local realtime limiter.
type InMemoryRealtimeLimiterOptions struct {
	Clock            foundation.Clock
	DefaultBucket    RealtimeRateLimitBucket
	PostureBuckets   map[RateLimitPosture]RealtimeRateLimitBucket
	OperationBuckets map[Operation]RealtimeRateLimitBucket
}

// InMemoryRealtimeLimiter is a process-local token bucket limiter keyed by
// server-resolved session, player, op, and rate-limit posture.
type InMemoryRealtimeLimiter struct {
	mu               sync.Mutex
	clock            foundation.Clock
	defaultBucket    RealtimeRateLimitBucket
	postureBuckets   map[RateLimitPosture]RealtimeRateLimitBucket
	operationBuckets map[Operation]RealtimeRateLimitBucket
	buckets          map[realtimeRateLimitKey]realtimeTokenBucket
}

// RateLimitRequest is the server-owned identity and operation posture supplied
// to realtime abuse protection.
type RateLimitRequest struct {
	SessionID        SessionID
	PlayerID         foundation.PlayerID
	WorldID          foundation.WorldID
	ZoneID           foundation.ZoneID
	Operation        Operation
	RequestID        foundation.RequestID
	RateLimitPosture RateLimitPosture
}

type realtimeRateLimitKey struct {
	sessionID SessionID
	playerID  foundation.PlayerID
	operation Operation
	posture   RateLimitPosture
}

type realtimeTokenBucket struct {
	tokens     int
	refilledAt time.Time
}

// NewInMemoryRealtimeLimiter returns a concrete process-local limiter suitable
// for a single gateway process. Callers needing cross-process limits must wrap
// the gateway with shared storage in a later operational slice.
func NewInMemoryRealtimeLimiter(options InMemoryRealtimeLimiterOptions) *InMemoryRealtimeLimiter {
	clock := options.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	defaultBucket := normalizeRealtimeRateLimitBucket(options.DefaultBucket, RealtimeRateLimitBucket{
		Burst:       30,
		RefillEvery: 100 * time.Millisecond,
	})
	postureBuckets := clonePostureRateLimitBuckets(defaultRealtimePostureBuckets())
	for posture, bucket := range options.PostureBuckets {
		postureBuckets[posture] = normalizeRealtimeRateLimitBucket(bucket, defaultBucket)
	}
	operationBuckets := cloneOperationRateLimitBuckets(defaultRealtimeOperationBuckets())
	for operation, bucket := range options.OperationBuckets {
		operationBuckets[operation] = normalizeRealtimeRateLimitBucket(bucket, defaultBucket)
	}
	return &InMemoryRealtimeLimiter{
		clock:            clock,
		defaultBucket:    defaultBucket,
		postureBuckets:   postureBuckets,
		operationBuckets: operationBuckets,
		buckets:          make(map[realtimeRateLimitKey]realtimeTokenBucket),
	}
}

// AllowRealtimeRequest consumes one token for the request bucket.
func (limiter *InMemoryRealtimeLimiter) AllowRealtimeRequest(request RateLimitRequest) error {
	if limiter == nil {
		return nil
	}
	limit := limiter.bucketFor(request)
	now := limiter.clock.Now()
	key := realtimeRateLimitKey{
		sessionID: request.SessionID,
		playerID:  request.PlayerID,
		operation: request.Operation,
		posture:   request.RateLimitPosture,
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	bucket, ok := limiter.buckets[key]
	if !ok {
		bucket = realtimeTokenBucket{
			tokens:     limit.Burst,
			refilledAt: now,
		}
	}
	bucket = refillRealtimeBucket(bucket, limit, now)
	if bucket.tokens <= 0 {
		limiter.buckets[key] = bucket
		return fmt.Errorf("%w: op=%s posture=%s", ErrRealtimeRateLimitExceeded, request.Operation, request.RateLimitPosture)
	}
	bucket.tokens--
	limiter.buckets[key] = bucket
	return nil
}

func (limiter *InMemoryRealtimeLimiter) bucketFor(request RateLimitRequest) RealtimeRateLimitBucket {
	if bucket, ok := limiter.operationBuckets[request.Operation]; ok {
		return bucket
	}
	if bucket, ok := limiter.postureBuckets[request.RateLimitPosture]; ok {
		return bucket
	}
	return limiter.defaultBucket
}

func refillRealtimeBucket(bucket realtimeTokenBucket, limit RealtimeRateLimitBucket, now time.Time) realtimeTokenBucket {
	if bucket.refilledAt.IsZero() || now.Before(bucket.refilledAt) {
		bucket.refilledAt = now
		return bucket
	}
	if bucket.tokens >= limit.Burst {
		bucket.refilledAt = now
		return bucket
	}
	elapsed := now.Sub(bucket.refilledAt)
	if elapsed < limit.RefillEvery {
		return bucket
	}
	refillTokens := int(elapsed / limit.RefillEvery)
	if refillTokens <= 0 {
		return bucket
	}
	bucket.tokens += refillTokens
	if bucket.tokens > limit.Burst {
		bucket.tokens = limit.Burst
	}
	bucket.refilledAt = bucket.refilledAt.Add(time.Duration(refillTokens) * limit.RefillEvery)
	return bucket
}

func normalizeRealtimeRateLimitBucket(bucket RealtimeRateLimitBucket, fallback RealtimeRateLimitBucket) RealtimeRateLimitBucket {
	if bucket.Burst < 1 {
		bucket.Burst = fallback.Burst
	}
	if bucket.RefillEvery <= 0 {
		bucket.RefillEvery = fallback.RefillEvery
	}
	return bucket
}

func defaultRealtimePostureBuckets() map[RateLimitPosture]RealtimeRateLimitBucket {
	return map[RateLimitPosture]RealtimeRateLimitBucket{
		RateLimitPostureIntentBurst: {
			Burst:       30,
			RefillEvery: 100 * time.Millisecond,
		},
		RateLimitPostureDebugOnly: {
			Burst:       5,
			RefillEvery: time.Second,
		},
		RateLimitPostureUnspecified: {
			Burst:       10,
			RefillEvery: time.Second,
		},
	}
}

func defaultRealtimeOperationBuckets() map[Operation]RealtimeRateLimitBucket {
	return map[Operation]RealtimeRateLimitBucket{
		OperationCombatUseSkill: {
			Burst:       8,
			RefillEvery: 250 * time.Millisecond,
		},
		OperationLootPickup: {
			Burst:       6,
			RefillEvery: 500 * time.Millisecond,
		},
		OperationScanPulse: {
			Burst:       2,
			RefillEvery: 5 * time.Second,
		},
		OperationMarketSearch: {
			Burst:       5,
			RefillEvery: 2 * time.Second,
		},
		OperationChatSend: {
			Burst:       3,
			RefillEvery: time.Second,
		},
		OperationPartyInvite: {
			Burst:       4,
			RefillEvery: 5 * time.Second,
		},
		OperationPartyAccept: {
			Burst:       4,
			RefillEvery: 5 * time.Second,
		},
		OperationPartyLeave: {
			Burst:       4,
			RefillEvery: 5 * time.Second,
		},
		OperationQuestReroll: {
			Burst:       2,
			RefillEvery: 30 * time.Second,
		},
		Operation("inventory.move"): {
			Burst:       6,
			RefillEvery: time.Second,
		},
		OperationProgressionUnlockSkill: {
			Burst:       4,
			RefillEvery: 2 * time.Second,
		},
	}
}

func clonePostureRateLimitBuckets(buckets map[RateLimitPosture]RealtimeRateLimitBucket) map[RateLimitPosture]RealtimeRateLimitBucket {
	cloned := make(map[RateLimitPosture]RealtimeRateLimitBucket, len(buckets))
	for posture, bucket := range buckets {
		cloned[posture] = bucket
	}
	return cloned
}

func cloneOperationRateLimitBuckets(buckets map[Operation]RealtimeRateLimitBucket) map[Operation]RealtimeRateLimitBucket {
	cloned := make(map[Operation]RealtimeRateLimitBucket, len(buckets))
	for operation, bucket := range buckets {
		cloned[operation] = bucket
	}
	return cloned
}

func rateLimitedError(cause error) *foundation.DomainError {
	return foundation.NewDomainError(
		foundation.CodeRateLimited,
		"Too many requests. Slow down and try again.",
		foundation.WithCause(cause),
	)
}
