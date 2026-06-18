package runtime

import (
	"encoding/json"
	"errors"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

var (
	ErrMissingRealtimeCommandHandler = errors.New("missing realtime command handler")
)

// RealtimeCommandGatewayConfig wires runtime realtime commands through
// observability before dispatching to operation handlers.
type RealtimeCommandGatewayConfig struct {
	Clock    foundation.Clock
	Logger   realtime.CommandLogger
	Metrics  realtime.CommandMetricRecorder
	Handlers map[realtime.Operation]realtime.CommandHandler
}

// RealtimeCommandGateway is the single-process runtime command boundary used
// by local server slices before a concrete WebSocket transport is attached.
type RealtimeCommandGateway struct {
	executor realtime.ObservedCommandExecutor
	handlers map[realtime.Operation]realtime.CommandHandler
}

// NewRealtimeCommandGateway returns an observed realtime command dispatcher.
func NewRealtimeCommandGateway(config RealtimeCommandGatewayConfig) *RealtimeCommandGateway {
	handlers := make(map[realtime.Operation]realtime.CommandHandler, len(config.Handlers))
	for operation, handler := range config.Handlers {
		handlers[operation] = handler
	}
	return &RealtimeCommandGateway{
		executor: realtime.ObservedCommandExecutor{
			Clock:   config.Clock,
			Logger:  config.Logger,
			Metrics: config.Metrics,
		},
		handlers: handlers,
	}
}

// Handle executes one server-resolved realtime command through the observed
// command boundary. The context must be built from auth/session state, not from
// the client request payload.
func (gateway *RealtimeCommandGateway) Handle(
	ctx realtime.CommandContext,
	request realtime.RequestEnvelope,
) (json.RawMessage, error) {
	executor := realtime.ObservedCommandExecutor{}
	var handler realtime.CommandHandler
	if gateway != nil {
		executor = gateway.executor
		handler = gateway.handlers[request.Op]
	}
	if handler == nil {
		handler = missingRealtimeCommandHandler
	}
	return executor.Execute(ctx, request, handler)
}

func missingRealtimeCommandHandler(realtime.CommandContext, realtime.RequestEnvelope) (json.RawMessage, error) {
	return nil, foundation.NewDomainError(
		foundation.CodeInternal,
		"Command handler unavailable.",
		foundation.WithCause(ErrMissingRealtimeCommandHandler),
	)
}
