package realtime

import (
	"context"
	"encoding/json"
	"fmt"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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

type telemetryErrorRecorder interface {
	RecordTelemetryError(reason observability.TelemetryErrorReason) error
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
	Tracer  trace.Tracer
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
	_, span := executor.startCommandSpan(ctx, request)
	defer span.End()
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
		span.RecordError(err)
		span.SetStatus(codes.Error, code.String())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	op := observability.Operation(request.Op)
	referenceID := commandLogReferenceID(ctx, request, payload)
	span.SetAttributes(
		attribute.String("game.command.result", status.String()),
		attribute.String("game.command.error_code", code.String()),
		attribute.String("game.command.idempotency_key", referenceID.String()),
	)
	recordCommandMetric(executor.Metrics, op, code)
	recordCommandLog(executor.Logger, observability.CommandLogEntry{
		RequestID:   request.RequestID,
		PlayerID:    ctx.PlayerID,
		SessionID:   observability.SessionID(ctx.SessionID),
		WorldID:     ctx.WorldID,
		ZoneID:      ctx.ZoneID,
		Operation:   op,
		ErrorCode:   code,
		ReferenceID: referenceID,
		Duration:    duration,
		Status:      status,
		Timestamp:   startedAt,
	})

	return cloneRawMessage(payload), err
}

func (executor ObservedCommandExecutor) startCommandSpan(ctx CommandContext, request RequestEnvelope) (context.Context, trace.Span) {
	if executor.Tracer == nil {
		return context.Background(), trace.SpanFromContext(context.Background())
	}
	return executor.Tracer.Start(context.Background(), "game.command",
		trace.WithAttributes(
			attribute.String("game.command.op", string(request.Op)),
			attribute.String("game.command.request_id", request.RequestID.String()),
			attribute.String("game.session_id", ctx.SessionID.String()),
			attribute.String("game.player_id", ctx.PlayerID.String()),
			attribute.String("game.world_id", ctx.WorldID.String()),
			attribute.String("game.zone_id", ctx.ZoneID.String()),
		),
	)
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
	if ctx.WorldID.IsZero() {
		return foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated world is required.")
	}
	if err := ctx.WorldID.Validate(); err != nil {
		return foundation.NewDomainError(foundation.CodeInvalidPayload, "World is invalid.", foundation.WithCause(err))
	}
	if ctx.ZoneID.IsZero() {
		return foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated zone is required.")
	}
	if err := ctx.ZoneID.Validate(); err != nil {
		return foundation.NewDomainError(foundation.CodeInvalidPayload, "Zone is invalid.", foundation.WithCause(err))
	}
	if !ctx.ReferenceID.IsZero() {
		if err := ctx.ReferenceID.Validate(); err != nil {
			return foundation.NewDomainError(foundation.CodeInvalidPayload, "Reference is invalid.", foundation.WithCause(err))
		}
	}
	return nil
}

func unavailableCommandHandler(CommandContext, RequestEnvelope) (json.RawMessage, error) {
	return nil, foundation.NewDomainError(foundation.CodeNotFound, "Command is unavailable.")
}

func recordCommandMetric(recorder CommandMetricRecorder, op observability.Operation, code foundation.Code) {
	if recorder == nil {
		return
	}
	if err := recorder.RecordCommandCount(op); err != nil {
		recordTelemetryMetricError(recorder)
	}
	if !code.IsZero() {
		if err := recorder.RecordCommandError(op, code); err != nil {
			recordTelemetryMetricError(recorder)
		}
	}
}

func recordTelemetryMetricError(recorder CommandMetricRecorder) {
	telemetryRecorder, ok := recorder.(telemetryErrorRecorder)
	if !ok {
		return
	}
	_ = telemetryRecorder.RecordTelemetryError(observability.TelemetryErrorMetricWrite)
}

func recordCommandLog(logger CommandLogger, entry observability.CommandLogEntry) {
	if logger == nil {
		return
	}
	_ = logger.Record(entry)
}

func commandLogReferenceID(ctx CommandContext, request RequestEnvelope, response json.RawMessage) foundation.IdempotencyKey {
	if !ctx.ReferenceID.IsZero() {
		return ctx.ReferenceID
	}

	switch request.Op {
	case OperationShopBuyProduct:
		referenceID, err := foundation.ShopPurchaseIdempotencyKey(ctx.PlayerID, request.RequestID)
		if err == nil {
			return referenceID
		}
	case OperationMarketBuy:
		listingID, ok := commandLogListingID(request.Payload)
		if !ok {
			return ""
		}
		referenceID, err := foundation.MarketBuyIdempotencyKey(listingID, ctx.PlayerID, request.RequestID)
		if err == nil {
			return referenceID
		}
	case OperationMarketCancel:
		listingID, ok := commandLogListingID(request.Payload)
		if !ok {
			return ""
		}
		referenceID, err := foundation.MarketCancelIdempotencyKey(listingID)
		if err == nil {
			return referenceID
		}
	case OperationDeathRepairShip:
		shipID, ok := commandLogRepairShipID(request.Payload)
		if !ok {
			return ""
		}
		referenceID, err := commandLogShipRepairID(ctx.PlayerID, shipID, request.RequestID)
		if err == nil {
			return referenceID
		}
	case OperationPortalEnter:
		portalID, ok := commandLogPortalID(request.Payload)
		if !ok {
			return ""
		}
		referenceID, err := foundation.PortalTransferIdempotencyKey(ctx.PlayerID, portalID, request.RequestID)
		if err == nil {
			return referenceID
		}
	case OperationAdminContentPublish:
		if referenceID, ok := commandLogContentResponseReferenceID(response, "content_publish"); ok {
			return referenceID
		}
		referenceID, err := foundation.ContentPublishIdempotencyKey(request.RequestID.String())
		if err == nil {
			return referenceID
		}
	case OperationAdminContentRollback:
		if referenceID, ok := commandLogContentResponseReferenceID(response, "content_rollback"); ok {
			return referenceID
		}
		targetVersionID, ok := commandLogTargetVersionID(request.Payload)
		if !ok {
			return ""
		}
		referenceID, err := foundation.ContentRollbackIdempotencyKey(targetVersionID, request.RequestID.String())
		if err == nil {
			return referenceID
		}
	}
	return ""
}

func commandLogListingID(payload json.RawMessage) (foundation.ListingID, bool) {
	var body struct {
		ListingID string `json:"listing_id"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", false
	}
	listingID, err := foundation.ParseListingID(body.ListingID)
	if err != nil {
		return "", false
	}
	return listingID, true
}

func commandLogRepairShipID(payload json.RawMessage) (foundation.ShipID, bool) {
	var body struct {
		ShipID string `json:"ship_id"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", false
	}
	shipID, err := foundation.ParseShipID(body.ShipID)
	if err != nil {
		return "", false
	}
	return shipID, true
}

func commandLogPortalID(payload json.RawMessage) (string, bool) {
	var body struct {
		PortalID string `json:"portal_id"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", false
	}
	if body.PortalID == "" {
		return "", false
	}
	return body.PortalID, true
}

func commandLogTargetVersionID(payload json.RawMessage) (string, bool) {
	var body struct {
		TargetVersionID string `json:"target_version_id"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", false
	}
	if body.TargetVersionID == "" {
		return "", false
	}
	return body.TargetVersionID, true
}

func commandLogContentResponseReferenceID(response json.RawMessage, key string) (foundation.IdempotencyKey, bool) {
	if len(response) == 0 {
		return "", false
	}
	var body map[string]struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.Unmarshal(response, &body); err != nil {
		return "", false
	}
	section, ok := body[key]
	if !ok || section.IdempotencyKey == "" {
		return "", false
	}
	referenceID, err := foundation.ParseIdempotencyKey(section.IdempotencyKey)
	if err != nil {
		return "", false
	}
	return referenceID, true
}

func commandLogShipRepairID(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	requestID foundation.RequestID,
) (foundation.IdempotencyKey, error) {
	if err := playerID.Validate(); err != nil {
		return "", err
	}
	if err := requestID.Validate(); err != nil {
		return "", err
	}
	reference := fmt.Sprintf(
		"player%d.%s.request%d.%s",
		len(playerID.String()),
		playerID.String(),
		len(requestID.String()),
		requestID.String(),
	)
	return foundation.ShipRepairIdempotencyKey(shipID, reference)
}
