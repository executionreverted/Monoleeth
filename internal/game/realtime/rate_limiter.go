package realtime

import "gameproject/internal/game/foundation"

// RateLimiter gates realtime requests before command handlers mutate gameplay
// state.
type RateLimiter interface {
	AllowRealtimeRequest(RateLimitRequest) error
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

func rateLimitedError(cause error) *foundation.DomainError {
	return foundation.NewDomainError(
		foundation.CodeRateLimited,
		"Too many requests. Slow down and try again.",
		foundation.WithCause(cause),
	)
}
