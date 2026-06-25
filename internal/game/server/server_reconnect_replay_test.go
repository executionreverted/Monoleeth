package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

func TestReconnectWithLastSeqReplaysMissedEventsInOrder(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilotWithIdentity(t, httpServer, "replay-order@example.com", "Replay Order")
	resolved := resolvedSessionForCookie(t, gameServer, cookie)

	firstConn := dialWebSocket(t, httpServer, cookie)
	defer firstConn.CloseNow()
	initial := readBootstrapEvents(t, firstConn)
	lastSeq := initial[len(initial)-1].Sequence

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(
		resolved.SessionID,
		realtime.OperationStop,
		resolved.PlayerID,
	)
	if err != nil {
		t.Fatalf("post stop events: %v", err)
	}
	missed := eventsBySession[resolved.SessionID]
	if len(missed) < 2 {
		t.Fatalf("missed events = %+v, want at least two ordered events", missed)
	}

	reconnect := dialWebSocketWithLastSeq(t, httpServer, cookie, lastSeq)
	defer reconnect.CloseNow()
	for index, want := range missed {
		got := readEvent(t, reconnect)
		if got.Sequence != want.Sequence || got.Type != want.Type {
			t.Fatalf("replay event %d = %s/%d, want %s/%d", index, got.Type, got.Sequence, want.Type, want.Sequence)
		}
	}
	lastMissed := missed[len(missed)-1]
	bootstrap := readBootstrapEvents(t, reconnect)
	if bootstrap[0].Type != realtime.EventSessionReady || bootstrap[0].Sequence <= lastMissed.Sequence {
		t.Fatalf("post-replay bootstrap first = %s/%d, want session.ready after seq %d", bootstrap[0].Type, bootstrap[0].Sequence, lastMissed.Sequence)
	}
}

func TestReconnectTooOldCursorSkipsReplayAndStillGetsSnapshot(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilotWithIdentity(t, httpServer, "replay-too-old@example.com", "Replay Too Old")
	resolved := resolvedSessionForCookie(t, gameServer, cookie)

	firstConn := dialWebSocket(t, httpServer, cookie)
	defer firstConn.CloseNow()
	readBootstrapEvents(t, firstConn)

	gameServer.runtime.mu.Lock()
	for i := 0; i < sessionEventRingCapacity+1; i++ {
		event := gameServer.runtime.eventLocked(resolved.SessionID, realtime.EventAOIEntityEntered, map[string]any{
			"entity_id": fmt.Sprintf("hidden_replay_%d", i),
			"hidden":    true,
		})
		gameServer.runtime.recordReplayEventsLocked(resolved.SessionID, []realtime.EventEnvelope{event})
	}
	gameServer.runtime.mu.Unlock()

	reconnect := dialWebSocketWithLastSeq(t, httpServer, cookie, 1)
	defer reconnect.CloseNow()
	events := readBootstrapEvents(t, reconnect)
	if events[0].Type != realtime.EventSessionReady {
		t.Fatalf("first reconnect event = %s, want snapshot bootstrap without replay", events[0].Type)
	}
	for _, event := range events {
		if strings.Contains(string(mustJSON(t, event)), "hidden_replay_") {
			t.Fatalf("too-old cursor replay leaked hidden event: %+v", event)
		}
	}
	_ = decodeWorldSnapshotForTest(t, events)
}

func TestEventRingBoundedCapacityEvictsOldEvents(t *testing.T) {
	ring := newSessionEventRing(3)
	for seq := uint64(1); seq <= 5; seq++ {
		ring.append(replayRingTestEvent(seq))
	}

	if _, ok := ring.replayAfter(1); ok {
		t.Fatal("replayAfter(1) ok = true, want false after eviction")
	}
	events, ok := ring.replayAfter(2)
	if !ok {
		t.Fatal("replayAfter(2) ok = false, want true")
	}
	if len(events) != 3 {
		t.Fatalf("replay event count = %d, want 3", len(events))
	}
	for index, event := range events {
		wantSeq := uint64(index + 3)
		if event.Sequence != wantSeq {
			t.Fatalf("replay event %d seq = %d, want %d", index, event.Sequence, wantSeq)
		}
	}
}

func dialWebSocketWithLastSeq(t *testing.T, httpServer *httptest.Server, cookie *http.Cookie, lastSeq uint64) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(httpServer)+"?last_seq="+strconv.FormatUint(lastSeq, 10), &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{testOrigin},
			"Cookie": []string{cookie.String()},
		},
	})
	if err != nil {
		t.Fatalf("websocket dial last_seq=%d error = %v, want nil", lastSeq, err)
	}
	return conn
}

func replayRingTestEvent(seq uint64) realtime.EventEnvelope {
	return realtime.NewEventEnvelope(
		foundation.EventID(fmt.Sprintf("event_%d", seq)),
		realtime.EventStatsUpdated,
		json.RawMessage(`{}`),
		0,
		seq,
	)
}
