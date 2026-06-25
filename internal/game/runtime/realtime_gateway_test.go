package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/realtime"
)

func TestRealtimeCommandGatewayWritesStructuredJSONCommandLog(t *testing.T) {
	var output bytes.Buffer
	logger, err := observability.NewJSONCommandLogger(&output)
	if err != nil {
		t.Fatalf("NewJSONCommandLogger() error = %v", err)
	}
	metrics := observability.NewMetricRecorder()
	gateway := NewRealtimeCommandGateway(RealtimeCommandGatewayConfig{
		Clock:   &runtimeStepClock{now: time.Date(2026, 6, 18, 15, 0, 0, 0, time.UTC), step: 13 * time.Millisecond},
		Logger:  logger,
		Metrics: metrics,
		Handlers: map[realtime.Operation]realtime.CommandHandler{
			realtime.OperationMoveTo: func(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
				if ctx.PlayerID != "player-1" {
					t.Fatalf("handler PlayerID = %q, want server-resolved player-1", ctx.PlayerID)
				}
				return json.RawMessage(`{"accepted":true,"server_debug":"not logged"}`), nil
			},
		},
	})
	request := realtime.NewRequestEnvelope(
		"request-1",
		realtime.OperationMoveTo,
		json.RawMessage(`{"x":10,"y":20,"client_secret":"not logged"}`),
		7,
	)

	payload, err := gateway.Handle(validRuntimeCommandContext(), request)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if got := string(payload); got != `{"accepted":true,"server_debug":"not logged"}` {
		t.Fatalf("payload = %s, want handler payload", got)
	}

	logLine := output.String()
	for _, want := range []string{
		`"request_id":"request-1"`,
		`"player_id":"player-1"`,
		`"session_id":"session-1"`,
		`"op":"move_to"`,
		`"idempotency_key":"loot_pickup:drop-1"`,
		`"ref_ids":["loot_pickup:drop-1"]`,
		`"result":"ok"`,
		`"duration_ms":13`,
	} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("log line %s missing %s", logLine, want)
		}
	}
	for _, leaked := range []string{
		`"world_id":`,
		`"zone_id":`,
		`"operation":`,
		`"reference_id":`,
		`"status":`,
		`"duration":`,
		"client_secret",
		"accepted",
		"server_debug",
		"not logged",
	} {
		if strings.Contains(logLine, leaked) {
			t.Fatalf("log line leaked %q in %s", leaked, logLine)
		}
	}

	counter := requireRuntimeCounter(t, metrics.Snapshot(), observability.MetricCommandsPerSecond)
	if counter.Value != 1 {
		t.Fatalf("command counter = %d, want 1", counter.Value)
	}
	requireRuntimeLabel(t, counter.Labels, "op", string(realtime.OperationMoveTo))
}

func TestRealtimeCommandGatewayLogsMissingHandlerAsError(t *testing.T) {
	logger := observability.NewMemoryCommandLogger()
	metrics := observability.NewMetricRecorder()
	gateway := NewRealtimeCommandGateway(RealtimeCommandGatewayConfig{
		Clock:   &runtimeStepClock{now: time.Date(2026, 6, 18, 15, 1, 0, 0, time.UTC), step: time.Millisecond},
		Logger:  logger,
		Metrics: metrics,
	})
	request := realtime.NewRequestEnvelope("request-2", realtime.OperationStop, json.RawMessage(`{}`), 8)

	_, err := gateway.Handle(validRuntimeCommandContext(), request)
	if !errors.Is(err, ErrMissingRealtimeCommandHandler) {
		t.Fatalf("Handle() error = %v, want ErrMissingRealtimeCommandHandler", err)
	}
	if !foundation.IsCode(err, foundation.CodeInternal) {
		t.Fatalf("Handle() code = %v, want %s", err, foundation.CodeInternal)
	}

	entries := logger.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	if entries[0].Status != observability.CommandStatusError || entries[0].ErrorCode != foundation.CodeInternal {
		t.Fatalf("log status/code = %q/%q, want error/%s", entries[0].Status, entries[0].ErrorCode, foundation.CodeInternal)
	}
	errorCounter := requireRuntimeCounterWithLabel(t, metrics.Snapshot(), observability.MetricErrorsByCode, "code", foundation.CodeInternal.String())
	if errorCounter.Value != 1 {
		t.Fatalf("error counter = %d, want 1", errorCounter.Value)
	}
}

func validRuntimeCommandContext() realtime.CommandContext {
	return realtime.CommandContext{
		SessionID:   realtime.SessionID("session-1"),
		PlayerID:    foundation.PlayerID("player-1"),
		WorldID:     foundation.WorldID("world-1"),
		ZoneID:      foundation.ZoneID("zone-1"),
		ReferenceID: foundation.IdempotencyKey("loot_pickup:drop-1"),
	}
}

type runtimeStepClock struct {
	now  time.Time
	step time.Duration
}

func (clock *runtimeStepClock) Now() time.Time {
	now := clock.now
	clock.now = clock.now.Add(clock.step)
	return now
}

func requireRuntimeCounter(t *testing.T, snapshot observability.MetricSnapshot, name string) observability.CounterSnapshot {
	t.Helper()
	for _, counter := range snapshot.Counters {
		if counter.Name == name {
			return counter
		}
	}
	t.Fatalf("missing counter %q in %#v", name, snapshot.Counters)
	return observability.CounterSnapshot{}
}

func requireRuntimeCounterWithLabel(
	t *testing.T,
	snapshot observability.MetricSnapshot,
	name string,
	labelName string,
	labelValue string,
) observability.CounterSnapshot {
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

func requireRuntimeLabel(t *testing.T, labels []observability.Label, name string, value string) {
	t.Helper()
	for _, label := range labels {
		if label.Name == name && label.Value == value {
			return
		}
	}
	t.Fatalf("missing label %s=%s in %#v", name, value, labels)
}
