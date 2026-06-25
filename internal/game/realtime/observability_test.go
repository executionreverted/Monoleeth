package realtime

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
)

func TestObservedCommandExecutorRecordsSafeLogAndCommandMetric(t *testing.T) {
	logger := observability.NewMemoryCommandLogger()
	metrics := observability.NewMetricRecorder()
	clock := &steppingClock{now: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC), step: 25 * time.Millisecond}
	executor := ObservedCommandExecutor{Clock: clock, Logger: logger, Metrics: metrics}
	request := NewRequestEnvelope("request-1", OperationMoveTo, json.RawMessage(`{"x":10,"y":20,"client_only":"ignored"}`), 7)
	ctx := validCommandContext()

	payload, err := executor.Execute(ctx, request, func(gotCtx CommandContext, gotRequest RequestEnvelope) (json.RawMessage, error) {
		if gotCtx.PlayerID != ctx.PlayerID || gotRequest.RequestID != request.RequestID {
			t.Fatalf("handler context/request = %+v/%+v, want %+v/%+v", gotCtx, gotRequest, ctx, request)
		}
		return json.RawMessage(`{"accepted":true,"internal":"not logged"}`), nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := string(payload); got != `{"accepted":true,"internal":"not logged"}` {
		t.Fatalf("payload = %s", got)
	}

	entries := logger.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.RequestID != request.RequestID || entry.PlayerID != ctx.PlayerID || entry.SessionID != observability.SessionID(ctx.SessionID) {
		t.Fatalf("log identity = %+v, want request/player/session from server context", entry)
	}
	if entry.Operation != observability.Operation(OperationMoveTo) || entry.Status != observability.CommandStatusOK {
		t.Fatalf("log op/status = %q/%q, want move_to/ok", entry.Operation, entry.Status)
	}
	if entry.Duration != 25*time.Millisecond {
		t.Fatalf("duration = %s, want 25ms", entry.Duration)
	}
	rawLog, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal log: %v", err)
	}
	for _, leaked := range []string{"client_only", "accepted", "internal", "not logged"} {
		if strings.Contains(string(rawLog), leaked) {
			t.Fatalf("command log leaked %q in %s", leaked, rawLog)
		}
	}

	counter := requireRealtimeCounter(t, metrics.Snapshot(), observability.MetricCommandsPerSecond)
	if counter.Value != 1 {
		t.Fatalf("command count = %d, want 1", counter.Value)
	}
	assertRealtimeLabels(t, counter.Labels, []observability.Label{{Name: "op", Value: string(OperationMoveTo)}})
}

func TestObservedCommandExecutorStructuredLogForLootPickupIncludesIdempotencyRequestFieldsNoSecrets(t *testing.T) {
	logger := observability.NewMemoryCommandLogger()
	clock := &steppingClock{now: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC), step: 17 * time.Millisecond}
	executor := ObservedCommandExecutor{Clock: clock, Logger: logger}
	request := NewRequestEnvelope(
		"request-loot-pickup-structured-log",
		OperationLootPickup,
		json.RawMessage(`{"drop_id":"drop-structured-log","password":"redacted","token":"redacted","cookie":"redacted","hash":"redacted"}`),
		10,
	)
	ctx := validCommandContext()
	wantReferenceID, err := foundation.LootPickupIdempotencyKey("drop-structured-log")
	if err != nil {
		t.Fatalf("LootPickupIdempotencyKey() error = %v", err)
	}
	ctx.ReferenceID = wantReferenceID

	_, err = executor.Execute(ctx, request, func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
		return json.RawMessage(`{"accepted":true,"token":"not_logged"}`), nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	entries := logger.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.ReferenceID != wantReferenceID {
		t.Fatalf("reference id = %q, want %q", entry.ReferenceID, wantReferenceID)
	}

	rawLog, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal log: %v", err)
	}
	var public map[string]any
	if err := json.Unmarshal(rawLog, &public); err != nil {
		t.Fatalf("decode public command log: %v", err)
	}

	want := map[string]string{
		"player_id":       ctx.PlayerID.String(),
		"session_id":      ctx.SessionID.String(),
		"request_id":      request.RequestID.String(),
		"op":              string(OperationLootPickup),
		"result":          observability.CommandStatusOK.String(),
		"error_code":      "",
		"idempotency_key": wantReferenceID.String(),
	}
	for field, value := range want {
		if public[field] != value {
			t.Fatalf("command log field %s = %#v, want %q in %s", field, public[field], value, rawLog)
		}
	}
	if public["duration_ms"] != float64(17) {
		t.Fatalf("duration_ms = %#v, want 17 in %s", public["duration_ms"], rawLog)
	}
	refIDs, ok := public["ref_ids"].([]any)
	if !ok || len(refIDs) != 1 || refIDs[0] != wantReferenceID.String() {
		t.Fatalf("ref_ids = %#v, want [%q] in %s", public["ref_ids"], wantReferenceID, rawLog)
	}
	for _, field := range []string{"password", "token", "cookie", "hash", "payload"} {
		if _, ok := public[field]; ok {
			t.Fatalf("command log exposed secret/payload field %q in %s", field, rawLog)
		}
		if strings.Contains(string(rawLog), field) {
			t.Fatalf("command log leaked secret/payload marker %q in %s", field, rawLog)
		}
	}
}

func TestObservedCommandExecutorMarketBuyStructuredLogIncludesSettlementIdempotencyAndNoSecrets(t *testing.T) {
	logger := observability.NewMemoryCommandLogger()
	clock := &steppingClock{now: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC), step: 19 * time.Millisecond}
	executor := ObservedCommandExecutor{Clock: clock, Logger: logger}
	request := NewRequestEnvelope(
		"request-market-buy-structured-log",
		OperationMarketBuy,
		json.RawMessage(`{"listing_id":"listing-structured-log","quantity":1,"password":"redacted","token":"redacted","cookie":"redacted","hash":"redacted"}`),
		11,
	)
	ctx := validCommandContext()
	ctx.ReferenceID = ""
	wantReferenceID, err := foundation.MarketBuyIdempotencyKey("listing-structured-log", ctx.PlayerID, request.RequestID)
	if err != nil {
		t.Fatalf("MarketBuyIdempotencyKey() error = %v", err)
	}

	_, err = executor.Execute(ctx, request, func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
		return json.RawMessage(`{"settled":true,"session_token":"not_logged"}`), nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	entry := requireSingleCommandLogEntry(t, logger)
	rawLog, public := decodePublicCommandLog(t, entry)
	want := map[string]string{
		"player_id":       ctx.PlayerID.String(),
		"session_id":      ctx.SessionID.String(),
		"request_id":      request.RequestID.String(),
		"op":              string(OperationMarketBuy),
		"result":          observability.CommandStatusOK.String(),
		"error_code":      "",
		"idempotency_key": wantReferenceID.String(),
	}
	assertPublicCommandLogStringFields(t, public, want, rawLog)
	assertPublicCommandLogDuration(t, public, 19, rawLog)
	assertPublicCommandLogRefIDs(t, public, wantReferenceID, rawLog)
	assertPublicCommandLogNoSecrets(t, public, rawLog)
}

func TestObservedCommandExecutorMarketCancelStructuredLogIncludesSettlementIdempotencyAndNoSecrets(t *testing.T) {
	logger := observability.NewMemoryCommandLogger()
	clock := &steppingClock{now: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC), step: 23 * time.Millisecond}
	executor := ObservedCommandExecutor{Clock: clock, Logger: logger}
	request := NewRequestEnvelope(
		"request-market-cancel-structured-log",
		OperationMarketCancel,
		json.RawMessage(`{"listing_id":"listing-cancel-structured-log","password":"redacted","token":"redacted","cookie":"redacted","hash":"redacted"}`),
		12,
	)
	ctx := validCommandContext()
	ctx.ReferenceID = ""
	wantReferenceID, err := foundation.MarketCancelIdempotencyKey("listing-cancel-structured-log")
	if err != nil {
		t.Fatalf("MarketCancelIdempotencyKey() error = %v", err)
	}

	_, err = executor.Execute(ctx, request, func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
		return nil, foundation.NewDomainError(foundation.CodeNotFound, "Market listing was not found.", foundation.WithDetail("listing internal detail"))
	})
	if !foundation.IsCode(err, foundation.CodeNotFound) {
		t.Fatalf("Execute() error = %v, want %s", err, foundation.CodeNotFound)
	}

	entry := requireSingleCommandLogEntry(t, logger)
	rawLog, public := decodePublicCommandLog(t, entry)
	want := map[string]string{
		"player_id":       ctx.PlayerID.String(),
		"session_id":      ctx.SessionID.String(),
		"request_id":      request.RequestID.String(),
		"op":              string(OperationMarketCancel),
		"result":          observability.CommandStatusError.String(),
		"error_code":      foundation.CodeNotFound.String(),
		"idempotency_key": wantReferenceID.String(),
	}
	assertPublicCommandLogStringFields(t, public, want, rawLog)
	assertPublicCommandLogDuration(t, public, 23, rawLog)
	assertPublicCommandLogRefIDs(t, public, wantReferenceID, rawLog)
	assertPublicCommandLogNoSecrets(t, public, rawLog)
	for _, leaked := range []string{"Market listing was not found.", "listing internal detail"} {
		if strings.Contains(string(rawLog), leaked) {
			t.Fatalf("command log leaked error detail %q in %s", leaked, rawLog)
		}
	}
}

func TestObservedCommandExecutorRecordsErrorCodeMetricWithoutLeakingDetails(t *testing.T) {
	logger := observability.NewMemoryCommandLogger()
	metrics := observability.NewMetricRecorder()
	executor := ObservedCommandExecutor{
		Clock:   &steppingClock{now: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC), step: time.Millisecond},
		Logger:  logger,
		Metrics: metrics,
	}
	request := NewRequestEnvelope("request-2", OperationDebugSnapshot, json.RawMessage(`{"target_id":"hidden-planet-9"}`), 8)
	hiddenErr := foundation.NewDomainError(
		foundation.CodeNotVisible,
		"No valid signal found.",
		foundation.WithDetail("hidden planet at x=10 y=20"),
	)

	_, err := executor.Execute(validCommandContext(), request, func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
		return nil, hiddenErr
	})
	if !foundation.IsCode(err, foundation.CodeNotVisible) {
		t.Fatalf("Execute() error = %v, want %s", err, foundation.CodeNotVisible)
	}

	entries := logger.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Status != observability.CommandStatusError || entry.ErrorCode != foundation.CodeNotVisible {
		t.Fatalf("log status/code = %q/%q, want error/%s", entry.Status, entry.ErrorCode, foundation.CodeNotVisible)
	}
	rawLog, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal log: %v", err)
	}
	for _, leaked := range []string{"hidden planet", "x=10", "y=20", "No valid signal found"} {
		if strings.Contains(string(rawLog), leaked) {
			t.Fatalf("command log leaked %q in %s", leaked, rawLog)
		}
	}

	errorCounter := requireCounterWithLabel(t, metrics.Snapshot(), observability.MetricErrorsByCode, "code", foundation.CodeNotVisible.String())
	if errorCounter.Value != 1 {
		t.Fatalf("error counter = %d, want 1", errorCounter.Value)
	}
}

func TestObservedCommandExecutorRecordsTelemetryErrorWhenMetricWriteFails(t *testing.T) {
	telemetry := observability.NewMetricRecorder()
	metrics := failingCommandMetricRecorder{telemetry: telemetry}
	executor := ObservedCommandExecutor{Metrics: metrics}
	request := NewRequestEnvelope("request-metric-write-failure", OperationMoveTo, json.RawMessage(`{"x":10,"y":20}`), 9)

	_, err := executor.Execute(validCommandContext(), request, func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
		return json.RawMessage(`{"accepted":true}`), nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	counter := requireCounterWithLabel(t, telemetry.Snapshot(), observability.MetricTelemetryErrors, "reason", observability.TelemetryErrorMetricWrite.String())
	if counter.Value != 1 {
		t.Fatalf("telemetry metric write error counter = %d, want 1", counter.Value)
	}
}

func TestObservedCommandExecutorRequiresServerResolvedIdentity(t *testing.T) {
	executor := ObservedCommandExecutor{}
	request := NewRequestEnvelope("request-3", OperationMoveTo, json.RawMessage(`{"x":10,"y":20}`), 9)
	var called bool

	_, err := executor.Execute(CommandContext{SessionID: "session-1"}, request, func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
		called = true
		return nil, nil
	})
	if !foundation.IsCode(err, foundation.CodeUnauthenticated) {
		t.Fatalf("Execute() error = %v, want %s", err, foundation.CodeUnauthenticated)
	}
	if called {
		t.Fatal("handler called without server-resolved player identity")
	}
}

func TestObservedCommandExecutorRequiresServerResolvedWorldAndZone(t *testing.T) {
	tests := []struct {
		name string
		ctx  CommandContext
	}{
		{
			name: "missing world",
			ctx: CommandContext{
				SessionID: SessionID("session-1"),
				PlayerID:  foundation.PlayerID("player-1"),
				ZoneID:    foundation.ZoneID("zone-1"),
			},
		},
		{
			name: "missing zone",
			ctx: CommandContext{
				SessionID: SessionID("session-1"),
				PlayerID:  foundation.PlayerID("player-1"),
				WorldID:   foundation.WorldID("world-1"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := ObservedCommandExecutor{}
			request := NewRequestEnvelope("request-4", OperationMoveTo, json.RawMessage(`{"x":10,"y":20}`), 9)
			var called bool

			_, err := executor.Execute(tt.ctx, request, func(CommandContext, RequestEnvelope) (json.RawMessage, error) {
				called = true
				return nil, nil
			})

			if !foundation.IsCode(err, foundation.CodeUnauthenticated) {
				t.Fatalf("Execute() error = %v, want %s", err, foundation.CodeUnauthenticated)
			}
			if called {
				t.Fatal("handler called without server-resolved world/zone identity")
			}
		})
	}
}

func validCommandContext() CommandContext {
	return CommandContext{
		SessionID:   SessionID("session-1"),
		PlayerID:    foundation.PlayerID("player-1"),
		WorldID:     foundation.WorldID("world-1"),
		ZoneID:      foundation.ZoneID("zone-1"),
		ReferenceID: foundation.IdempotencyKey("loot_pickup:drop-1"),
	}
}

func requireSingleCommandLogEntry(t *testing.T, logger *observability.MemoryCommandLogger) observability.CommandLogEntry {
	t.Helper()
	entries := logger.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	return entries[0]
}

func decodePublicCommandLog(t *testing.T, entry observability.CommandLogEntry) ([]byte, map[string]any) {
	t.Helper()
	rawLog, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal log: %v", err)
	}
	var public map[string]any
	if err := json.Unmarshal(rawLog, &public); err != nil {
		t.Fatalf("decode public command log: %v", err)
	}
	return rawLog, public
}

func assertPublicCommandLogStringFields(t *testing.T, public map[string]any, want map[string]string, rawLog []byte) {
	t.Helper()
	for field, value := range want {
		if public[field] != value {
			t.Fatalf("command log field %s = %#v, want %q in %s", field, public[field], value, rawLog)
		}
	}
}

func assertPublicCommandLogDuration(t *testing.T, public map[string]any, want int64, rawLog []byte) {
	t.Helper()
	if public["duration_ms"] != float64(want) {
		t.Fatalf("duration_ms = %#v, want %d in %s", public["duration_ms"], want, rawLog)
	}
}

func assertPublicCommandLogRefIDs(t *testing.T, public map[string]any, wantReferenceID foundation.IdempotencyKey, rawLog []byte) {
	t.Helper()
	refIDs, ok := public["ref_ids"].([]any)
	if !ok || len(refIDs) != 1 || refIDs[0] != wantReferenceID.String() {
		t.Fatalf("ref_ids = %#v, want [%q] in %s", public["ref_ids"], wantReferenceID, rawLog)
	}
}

func assertPublicCommandLogNoSecrets(t *testing.T, public map[string]any, rawLog []byte) {
	t.Helper()
	for _, field := range []string{"password", "password_hash", "session_token", "token", "cookie", "hash", "payload"} {
		if _, ok := public[field]; ok {
			t.Fatalf("command log exposed secret/payload field %q in %s", field, rawLog)
		}
		if strings.Contains(string(rawLog), field) {
			t.Fatalf("command log leaked secret/payload marker %q in %s", field, rawLog)
		}
	}
}

type steppingClock struct {
	now  time.Time
	step time.Duration
}

func (clock *steppingClock) Now() time.Time {
	now := clock.now
	clock.now = clock.now.Add(clock.step)
	return now
}

type failingCommandMetricRecorder struct {
	telemetry *observability.MetricRecorder
}

func (recorder failingCommandMetricRecorder) RecordCommandCount(observability.Operation) error {
	return errors.New("metric write failed")
}

func (recorder failingCommandMetricRecorder) RecordCommandError(observability.Operation, foundation.Code) error {
	return errors.New("metric write failed")
}

func (recorder failingCommandMetricRecorder) RecordTelemetryError(reason observability.TelemetryErrorReason) error {
	return recorder.telemetry.RecordTelemetryError(reason)
}

func requireRealtimeCounter(t *testing.T, snapshot observability.MetricSnapshot, name string) observability.CounterSnapshot {
	t.Helper()
	for _, counter := range snapshot.Counters {
		if counter.Name == name {
			return counter
		}
	}
	t.Fatalf("missing counter %q in %#v", name, snapshot.Counters)
	return observability.CounterSnapshot{}
}

func requireCounterWithLabel(t *testing.T, snapshot observability.MetricSnapshot, name string, labelName string, labelValue string) observability.CounterSnapshot {
	t.Helper()
	for _, counter := range snapshot.Counters {
		if counter.Name != name {
			continue
		}
		for _, label := range counter.Labels {
			if label.Name == labelName && label.Value == labelValue {
				return counter
			}
		}
	}
	t.Fatalf("missing counter %q with %s=%s in %#v", name, labelName, labelValue, snapshot.Counters)
	return observability.CounterSnapshot{}
}

func assertRealtimeLabels(t *testing.T, got, want []observability.Label) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("labels = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("label[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
