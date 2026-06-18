package realtime

import (
	"encoding/json"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
)

// CommandLogger is the structured command log sink used by the realtime command boundary.
type CommandLogger interface {
	Record(observability.CommandLogEntry) error
}

// CommandMetricRecorder is the command metric sink used by the realtime command boundary.
type CommandMetricRecorder interface {
	RecordCommandCount(op observability.Operation) error
	RecordCommandError(op observability.Operation, code foundation.Code) error
}

// CommandContext is server-resolved command identity. It must come from auth and
// authoritative routing, never from the client payload.
type CommandContext struct {
	SessionID   SessionID
	PlayerID    foundation.PlayerID
	WorldID     foundation.WorldID
	ZoneID      foundation.ZoneID
	ReferenceID foundation.IdempotencyKey
}

// CommandHandler executes one already-decoded realtime command.
type CommandHandler func(CommandContext, RequestEnvelope) (json.RawMessage, error)

// ObservedCommandExecutor records safe logs and metrics around command execution.
type ObservedCommandExecutor struct {
	Clock   foundation.Clock
	Logger  CommandLogger
	Metrics CommandMetricRecorder
}

// Execute runs handler and records command observability using server-resolved context.
func (executor ObservedCommandExecutor) Execute(ctx CommandContext, request RequestEnvelope, handler CommandHandler) (json.RawMessage, error) {
	if err := ctx.Validate(); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if handler == nil {
		handler = unavailableCommandHandler
	}

	clock := executor.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	startedAt := clock.Now().UTC()
	payload, err := handler(ctx, request)
	finishedAt := clock.Now().UTC()
	duration := finishedAt.Sub(startedAt)
	if duration < 0 {
		duration = 0
	}

	status := observability.CommandStatusOK
	var code foundation.Code
	if err != nil {
		status = observability.CommandStatusError
		code = foundation.CodeInternal
		if domainCode, ok := foundation.CodeOf(err); ok {
			code = domainCode
		}
	}

	op := observability.Operation(request.Op)
	recordCommandMetric(executor.Metrics, op, code)
	recordCommandLog(executor.Logger, observability.CommandLogEntry{
		RequestID:   request.RequestID,
		PlayerID:    ctx.PlayerID,
		SessionID:   observability.SessionID(ctx.SessionID),
		WorldID:     ctx.WorldID,
		ZoneID:      ctx.ZoneID,
		Operation:   op,
		ErrorCode:   code,
		ReferenceID: ctx.ReferenceID,
		Duration:    duration,
		Status:      status,
		Timestamp:   startedAt,
	})

	return cloneRawMessage(payload), err
}

// Validate reports whether context contains server-resolved command identity.
func (ctx CommandContext) Validate() error {
	if err := ctx.SessionID.Validate(); err != nil {
		return foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated session is required.", foundation.WithCause(err))
	}
	if ctx.PlayerID.IsZero() {
		return foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated player is required.")
	}
	if err := ctx.PlayerID.Validate(); err != nil {
		return foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated player is invalid.", foundation.WithCause(err))
	}
	if !ctx.WorldID.IsZero() {
		if err := ctx.WorldID.Validate(); err != nil {
			return foundation.NewDomainError(foundation.CodeInvalidPayload, "World is invalid.", foundation.WithCause(err))
		}
	}
	if !ctx.ZoneID.IsZero() {
		if err := ctx.ZoneID.Validate(); err != nil {
			return foundation.NewDomainError(foundation.CodeInvalidPayload, "Zone is invalid.", foundation.WithCause(err))
		}
	}
	if !ctx.ReferenceID.IsZero() {
		if err := ctx.ReferenceID.Validate(); err != nil {
			return foundation.NewDomainError(foundation.CodeInvalidPayload, "Reference is invalid.", foundation.WithCause(err))
		}
	}
	return nil
}

func unavailableCommandHandler(CommandContext, RequestEnvelope) (json.RawMessage, error) {
	return nil, foundation.NewDomainError(foundation.CodeInternal, "Command handler unavailable.")
}

func recordCommandMetric(recorder CommandMetricRecorder, op observability.Operation, code foundation.Code) {
	if recorder == nil {
		return
	}
	_ = recorder.RecordCommandCount(op)
	if !code.IsZero() {
		_ = recorder.RecordCommandError(op, code)
	}
}

func recordCommandLog(logger CommandLogger, entry observability.CommandLogEntry) {
	if logger == nil {
		return
	}
	_ = logger.Record(entry)
}
