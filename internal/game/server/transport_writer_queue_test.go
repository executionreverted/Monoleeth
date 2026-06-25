package server

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/realtime"
)

func TestWriterQueueSlowClientWriteDoesNotBlockOtherConnection(t *testing.T) {
	server := &Server{}
	slowSessionID := auth.SessionID("session-writer-queue-slow")
	fastSessionID := auth.SessionID("session-writer-queue-fast")
	slowConn := newFakeWebSocketWriter(true)
	fastConn := newFakeWebSocketWriter(false)
	slowClient := newClientConnection(slowConn, slowSessionID, 2)
	fastClient := newClientConnection(fastConn, fastSessionID, 2)
	slowClient.startWriter(time.Second)
	fastClient.startWriter(time.Second)
	defer closeTestClient(slowClient)
	defer closeTestClient(fastClient)
	server.conns.Store(slowClient, struct{}{})
	server.conns.Store(fastClient, struct{}{})
	defer server.conns.Delete(slowClient)
	defer server.conns.Delete(fastClient)

	if !server.writeText(slowClient, []byte(`{"held":true}`)) {
		t.Fatal("slow client initial enqueue = false, want true")
	}
	select {
	case <-slowConn.writeStarted:
	case <-time.After(time.Second):
		t.Fatal("slow client writer did not enter blocked write")
	}

	written := make(chan bool, 1)
	go func() {
		server.writeEventsToSession(slowSessionID, []realtime.EventEnvelope{writerQueueEvent(1)})
		written <- server.writeEventsToSession(fastSessionID, []realtime.EventEnvelope{writerQueueEvent(2)})
	}()

	select {
	case ok := <-written:
		if !ok {
			t.Fatal("writeEventsToSession returned false, want true")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("writeEventsToSession blocked behind slow client")
	}
	select {
	case payload := <-fastConn.writes:
		got := decodeWriterQueueEvent(t, payload)
		if got.Type != realtime.EventServerNotice {
			t.Fatalf("fast client event type = %q, want %q", got.Type, realtime.EventServerNotice)
		}
	case <-time.After(time.Second):
		t.Fatal("fast client received no queued event")
	}
}

func TestWriterQueueOverflowClosesOnlySlowClient(t *testing.T) {
	server := &Server{}
	slowConn := newFakeWebSocketWriter(false)
	otherConn := newFakeWebSocketWriter(false)
	slowClient := newClientConnection(slowConn, auth.SessionID("session-overflow-slow"), 1)
	otherClient := newClientConnection(otherConn, auth.SessionID("session-overflow-other"), 1)

	if !server.writeEvents(slowClient, []realtime.EventEnvelope{writerQueueEvent(1)}) {
		t.Fatal("first slow client enqueue = false, want true")
	}
	if server.writeEvents(slowClient, []realtime.EventEnvelope{writerQueueEvent(2)}) {
		t.Fatal("overflow enqueue = true, want false")
	}
	if !slowConn.isClosed() {
		t.Fatal("overflow client closed = false, want true")
	}
	if otherConn.isClosed() {
		t.Fatal("other client closed = true, want false")
	}
	if !server.writeEvents(otherClient, []realtime.EventEnvelope{writerQueueEvent(3)}) {
		t.Fatal("other client enqueue = false, want true")
	}
}

func TestWriterQueueOverflowRecordsTelemetryQueueDropCounter(t *testing.T) {
	server, metrics := newTransportTelemetryServer()
	slowClient := newClientConnection(newFakeWebSocketWriter(false), auth.SessionID("session-overflow-telemetry"), 1)

	if !server.writeEvents(slowClient, []realtime.EventEnvelope{writerQueueEvent(1)}) {
		t.Fatal("first slow client enqueue = false, want true")
	}
	if server.writeEvents(slowClient, []realtime.EventEnvelope{writerQueueEvent(2)}) {
		t.Fatal("overflow enqueue = true, want false")
	}

	snapshot := metrics.Snapshot()
	requireMetricCounter(t, snapshot, observability.MetricTelemetryErrors, 1, []observability.Label{
		{Name: "reason", Value: observability.TelemetryErrorQueueDrop.String()},
	})
}

func TestWriterQueueOverflowRecordsTelemetrySlowClientDisconnectCounter(t *testing.T) {
	server, metrics := newTransportTelemetryServer()
	slowClient := newClientConnection(newFakeWebSocketWriter(false), auth.SessionID("session-overflow-slow-client-telemetry"), 1)

	if !server.writeEvents(slowClient, []realtime.EventEnvelope{writerQueueEvent(1)}) {
		t.Fatal("first slow client enqueue = false, want true")
	}
	if server.writeEvents(slowClient, []realtime.EventEnvelope{writerQueueEvent(2)}) {
		t.Fatal("overflow enqueue = true, want false")
	}

	requireMetricCounter(t, metrics.Snapshot(), observability.MetricTelemetryErrors, 1, []observability.Label{
		{Name: "reason", Value: observability.TelemetryErrorSlowClientDisconnect.String()},
	})
}

func TestWriteEventsRecordsTelemetryEncodeCounterOnEventEncodeError(t *testing.T) {
	server, metrics := newTransportTelemetryServer()
	conn := newFakeWebSocketWriter(false)
	client := newClientConnection(conn, auth.SessionID("session-event-encode-telemetry"), 1)
	event := realtime.NewEventEnvelope(
		foundation.EventID("event-bad-json"),
		realtime.EventServerNotice,
		json.RawMessage(`{"bad"`),
		123,
		1,
	)

	if server.writeEvents(client, []realtime.EventEnvelope{event}) {
		t.Fatal("writeEvents() = true, want false for event encode failure")
	}

	requireMetricCounter(t, metrics.Snapshot(), observability.MetricTelemetryErrors, 1, []observability.Label{
		{Name: "reason", Value: observability.TelemetryErrorEventEncode.String()},
	})
}

func TestWriterQueueNormalClientReceivesQueuedEvent(t *testing.T) {
	server := &Server{}
	conn := newFakeWebSocketWriter(false)
	client := newClientConnection(conn, auth.SessionID("session-writer-queue-normal"), 1)
	client.startWriter(time.Second)
	defer closeTestClient(client)

	if !server.writeEvents(client, []realtime.EventEnvelope{writerQueueEvent(1)}) {
		t.Fatal("normal client enqueue = false, want true")
	}
	select {
	case payload := <-conn.writes:
		got := decodeWriterQueueEvent(t, payload)
		if got.Type != realtime.EventServerNotice {
			t.Fatalf("normal client event type = %q, want %q", got.Type, realtime.EventServerNotice)
		}
	case <-time.After(time.Second):
		t.Fatal("normal client received no queued event")
	}
}

func newTransportTelemetryServer() (*Server, *observability.MetricRecorder) {
	metrics := observability.NewMetricRecorder()
	return &Server{runtime: &Runtime{Metrics: metrics}}, metrics
}

type fakeWebSocketWriter struct {
	blockWrites  bool
	writeStarted chan struct{}
	writes       chan []byte
	closed       chan struct{}
	closeOnce    sync.Once
	unblockOnce  sync.Once
	startOnce    sync.Once
	mu           sync.Mutex
	status       websocket.StatusCode
	reason       string
}

func newFakeWebSocketWriter(blockWrites bool) *fakeWebSocketWriter {
	return &fakeWebSocketWriter{
		blockWrites:  blockWrites,
		writeStarted: make(chan struct{}),
		writes:       make(chan []byte, 8),
		closed:       make(chan struct{}),
	}
}

func (conn *fakeWebSocketWriter) Write(ctx context.Context, messageType websocket.MessageType, payload []byte) error {
	if messageType != websocket.MessageText {
		return errors.New("unexpected non-text websocket message")
	}
	if conn.blockWrites {
		conn.startOnce.Do(func() {
			close(conn.writeStarted)
		})
		select {
		case <-conn.closed:
			return errors.New("connection closed")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	copied := append([]byte(nil), payload...)
	select {
	case conn.writes <- copied:
		return nil
	case <-conn.closed:
		return errors.New("connection closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (conn *fakeWebSocketWriter) Close(status websocket.StatusCode, reason string) error {
	conn.mu.Lock()
	conn.status = status
	conn.reason = reason
	conn.mu.Unlock()
	conn.closeOnce.Do(func() {
		close(conn.closed)
	})
	return nil
}

func (conn *fakeWebSocketWriter) CloseNow() error {
	return conn.Close(websocket.StatusNormalClosure, "closed now")
}

func (conn *fakeWebSocketWriter) isClosed() bool {
	select {
	case <-conn.closed:
		return true
	default:
		return false
	}
}

func writerQueueEvent(sequence uint64) realtime.EventEnvelope {
	return realtime.NewEventEnvelope(
		foundation.EventID("event-writer-queue"),
		realtime.EventServerNotice,
		json.RawMessage(`{"message":"queued"}`),
		123,
		sequence,
	)
}

func decodeWriterQueueEvent(t *testing.T, payload []byte) realtime.EventEnvelope {
	t.Helper()
	var event realtime.EventEnvelope
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decode queued event %s: %v", payload, err)
	}
	return event
}

func closeTestClient(client *clientConnection) {
	client.closeNow()
	client.waitForWriter(time.Second)
}
