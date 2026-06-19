package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world/aoi"
)

const testOrigin = "http://example.com"

func TestServerAuthRoutesAndWebSocketBootstrap(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()

	events := readBootstrapEvents(t, conn)
	gotTypes := make(map[realtime.ClientEventType]struct{}, len(events))
	for _, event := range events {
		gotTypes[event.Type] = struct{}{}
		raw := string(mustJSON(t, event))
		for _, forbidden := range []string{
			"account_id",
			"player_id",
			"session_id",
			"world_id",
			"zone_id",
			"entity_hidden_planet_signal",
			"npc_placeholder",
			"loot_placeholder",
			"planet_signal_placeholder",
			"gameplay_seed",
			"future_spawn",
		} {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("bootstrap event leaked %q in %s", forbidden, raw)
			}
		}
	}
	for _, want := range []realtime.ClientEventType{
		realtime.EventSessionReady,
		realtime.EventPlayerSnapshot,
		realtime.EventShipSnapshot,
		realtime.EventStatsUpdated,
		realtime.EventWalletSnapshot,
		realtime.EventCargoSnapshot,
		realtime.EventWorldSnapshot,
	} {
		if _, ok := gotTypes[want]; !ok {
			t.Fatalf("missing bootstrap event %q in %#v", want, gotTypes)
		}
	}
	if events[0].Sequence != 1 || events[len(events)-1].Sequence != uint64(len(events)) {
		t.Fatalf("bootstrap seq range = %d..%d, want 1..%d", events[0].Sequence, events[len(events)-1].Sequence, len(events))
	}
	_ = gameServer
}

func TestWorldSnapshotCarriesSectorMinimapAndPublicEntityContract(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	events := readBootstrapEvents(t, conn)

	var snapshot worldSnapshotPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &snapshot); err != nil {
		t.Fatalf("decode world snapshot: %v", err)
	}
	if snapshot.Sector.Name != "Origin Fringe" || snapshot.Sector.Region != "Origin Belt" || snapshot.Sector.Danger == "" {
		t.Fatalf("sector payload = %+v, want client-safe sector summary", snapshot.Sector)
	}
	if snapshot.Minimap.RadarRange != defaultRadarRange {
		t.Fatalf("minimap radar = %v, want %v", snapshot.Minimap.RadarRange, defaultRadarRange)
	}
	if len(snapshot.Minimap.LiveContacts) != len(snapshot.Entities) {
		t.Fatalf("minimap contacts = %d, entities = %d", len(snapshot.Minimap.LiveContacts), len(snapshot.Entities))
	}
	selfCount := 0
	for _, entity := range snapshot.Entities {
		if strings.Contains(entity.Type.String(), "placeholder") {
			t.Fatalf("entity type %q still uses placeholder contract", entity.Type)
		}
		if entity.Display == nil || entity.Display.Label == "" || entity.Display.Disposition == "" {
			t.Fatalf("entity %+v missing public display metadata", entity)
		}
		if hasStatusFlag(entity.StatusFlags, "self") {
			selfCount++
			if entity.Type != "player" || entity.Display.Disposition != "self" {
				t.Fatalf("self entity = %+v, want player/self", entity)
			}
		}
	}
	if selfCount != 1 {
		t.Fatalf("self entity count = %d, want 1", selfCount)
	}
	for _, contact := range snapshot.Minimap.LiveContacts {
		if contact.EntityID == "entity_hidden_planet_signal" {
			t.Fatalf("hidden entity leaked into minimap contact %+v", contact)
		}
	}
}

func TestWebSocketUnauthenticatedConnectionRejectedBeforeUpgrade(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, wsURL(httpServer), &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{testOrigin}},
	})
	if err == nil {
		t.Fatal("websocket dial without cookie succeeded, want rejection")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("dial response = %#v, want 401", resp)
	}
}

func TestMoveToThroughWebSocketUsesGatewayAndServerPosition(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-move-1","op":"move_to","payload":{"target":{"x":100,"y":0}},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("move response = %+v, want success", response)
	}
	var payload struct {
		Accepted bool `json:"accepted"`
		Entities []struct {
			EntityID string `json:"entity_id"`
			Type     string `json:"entity_type"`
			Position struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"position"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode move payload: %v", err)
	}
	if !payload.Accepted {
		t.Fatal("move accepted = false, want true")
	}
	var playerX float64
	for _, entity := range payload.Entities {
		if entity.Type == "player" {
			playerX = entity.Position.X
		}
	}
	if playerX <= 0 || playerX >= 100 {
		t.Fatalf("player x after one server tick = %v, want server-derived movement between 0 and target", playerX)
	}

	event := readEvent(t, conn)
	if event.Type != realtime.EventPositionCorrected || event.Sequence == 0 {
		t.Fatalf("post-move event = %+v, want position.corrected with seq", event)
	}
	update := readEvent(t, conn)
	if update.Type != realtime.EventAOIEntityUpdated {
		t.Fatalf("post-move AOI event = %+v, want entity updated", update)
	}
}

func TestMoveToRejectsExcessivePathAndDisabledShip(t *testing.T) {
	t.Run("non finite coordinate", func(t *testing.T) {
		_, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)

		writeText(t, conn, `{"request_id":"request-move-non-finite","op":"move_to","payload":{"target":{"x":1e999,"y":0}},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeInvalidPayload {
			t.Fatalf("non-finite move error = %+v, want %s", got.Error, foundation.CodeInvalidPayload)
		}
	})

	t.Run("excessive path", func(t *testing.T) {
		_, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)

		writeText(t, conn, `{"request_id":"request-move-far","op":"move_to","payload":{"target":{"x":5000,"y":0}},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeOutOfRange {
			t.Fatalf("far move error = %+v, want %s", got.Error, foundation.CodeOutOfRange)
		}
	})

	t.Run("disabled ship", func(t *testing.T) {
		gameServer, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		cookie := registerPilot(t, httpServer)
		conn := dialWebSocket(t, httpServer, cookie)
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)
		resolved := resolvedSessionForCookie(t, gameServer, cookie)
		setTestShipDisabled(gameServer, resolved.PlayerID, true)

		writeText(t, conn, `{"request_id":"request-move-disabled","op":"move_to","payload":{"target":{"x":10,"y":0}},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeShipDisabled {
			t.Fatalf("disabled move error = %+v, want %s", got.Error, foundation.CodeShipDisabled)
		}
	})
}

func TestStopClearsMovementTargetServerSide(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)

	writeText(t, conn, `{"request_id":"request-stop-move","op":"move_to","payload":{"target":{"x":100,"y":0}},"client_seq":1,"v":1}`)
	if response := readResponse(t, conn); !response.OK {
		t.Fatalf("move response = %+v, want success", response)
	}
	readEvent(t, conn)
	readEvent(t, conn)
	writeText(t, conn, `{"request_id":"request-stop-1","op":"stop","payload":{},"client_seq":2,"v":1}`)
	if response := readResponse(t, conn); !response.OK {
		t.Fatalf("stop response = %+v, want success", response)
	}
	correction := readEvent(t, conn)
	stopped := readEvent(t, conn)
	if correction.Type != realtime.EventPositionCorrected || stopped.Type != realtime.EventMovementStopped {
		t.Fatalf("stop events = %q/%q, want correction/stopped", correction.Type, stopped.Type)
	}

	gameServer.runtime.mu.Lock()
	entity, ok := gameServer.runtime.Worker.PlayerEntity(resolved.PlayerID)
	gameServer.runtime.mu.Unlock()
	if !ok {
		t.Fatal("player entity missing after stop")
	}
	if entity.Movement.Moving {
		t.Fatalf("movement state = %+v, want stopped", entity.Movement)
	}
}

func TestAOIDiffEventsAreFilteredPerSession(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "aoi-filter@example.com", "AOI-Filter")

	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[resolved.PlayerID]
	state.Stats.RadarRange = 10
	gameServer.runtime.players[resolved.PlayerID] = state
	gameServer.runtime.mu.Unlock()

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	var initial worldSnapshotPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &initial); err != nil {
		t.Fatalf("decode initial world snapshot: %v", err)
	}
	if len(initial.Entities) != 1 || !hasStatusFlag(initial.Entities[0].StatusFlags, "self") {
		t.Fatalf("initial filtered entities = %+v, want only self", initial.Entities)
	}

	gameServer.runtime.mu.Lock()
	state = gameServer.runtime.players[resolved.PlayerID]
	state.Stats.RadarRange = defaultRadarRange
	gameServer.runtime.players[resolved.PlayerID] = state
	gameServer.runtime.mu.Unlock()

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	sessionEvents := eventsBySession[resolved.SessionID]
	if len(sessionEvents) == 0 || sessionEvents[0].Type != realtime.EventAOIEntityEntered {
		t.Fatalf("AOI events after radar increase = %+v, want entity entered", sessionEvents)
	}
	var entered aoiEntityPayloadForTest
	if err := json.Unmarshal(sessionEvents[0].Payload, &entered); err != nil {
		t.Fatalf("decode entered event: %v", err)
	}
	if entered.EntityID != "entity_training_npc" || entered.EntityType != "npc" {
		t.Fatalf("entered entity = %+v, want training npc", entered)
	}
}

func TestTwoPlayersWithDifferentRadarReceiveDifferentFilteredSnapshots(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	limited := createResolvedRuntimeSession(t, gameServer, "limited-radar@example.com", "Limited")
	defaultRadar := createResolvedRuntimeSession(t, gameServer, "default-radar@example.com", "Default")

	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[limited.PlayerID]
	state.Stats.RadarRange = 10
	gameServer.runtime.players[limited.PlayerID] = state
	gameServer.runtime.mu.Unlock()

	limitedEvents, err := gameServer.runtime.bootstrapEvents(limited)
	if err != nil {
		t.Fatalf("limited bootstrap events: %v", err)
	}
	defaultEvents, err := gameServer.runtime.bootstrapEvents(defaultRadar)
	if err != nil {
		t.Fatalf("default bootstrap events: %v", err)
	}
	limitedSnapshot := decodeWorldSnapshotForTest(t, limitedEvents)
	defaultSnapshot := decodeWorldSnapshotForTest(t, defaultEvents)

	if hasEntityID(limitedSnapshot.Entities, "entity_training_npc") {
		t.Fatalf("limited radar snapshot included training npc: %+v", limitedSnapshot.Entities)
	}
	if !hasEntityID(defaultSnapshot.Entities, "entity_training_npc") {
		t.Fatalf("default radar snapshot missing training npc: %+v", defaultSnapshot.Entities)
	}
}

func TestMultiTabAttachDoesNotDuplicatePlayerEntity(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)

	first := dialWebSocket(t, httpServer, cookie)
	defer first.CloseNow()
	firstEvents := readBootstrapEvents(t, first)
	second := dialWebSocket(t, httpServer, cookie)
	defer second.CloseNow()
	secondEvents := readBootstrapEvents(t, second)

	for _, events := range [][]realtime.EventEnvelope{firstEvents, secondEvents} {
		var snapshot worldSnapshotPayload
		if err := json.Unmarshal(events[len(events)-1].Payload, &snapshot); err != nil {
			t.Fatalf("decode world snapshot: %v", err)
		}
		playerCount := 0
		for _, entity := range snapshot.Entities {
			if entity.Type == "player" {
				playerCount++
			}
		}
		if playerCount != 1 {
			t.Fatalf("player entities = %d in %+v, want 1", playerCount, snapshot.Entities)
		}
	}
}

func TestDuplicateRequestIDReturnsCachedWebSocketResponse(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	request := `{"request_id":"request-snapshot-1","op":"world.snapshot","payload":{},"client_seq":1,"v":1}`
	writeText(t, conn, request)
	first := readRawText(t, conn)
	writeText(t, conn, request)
	second := readRawText(t, conn)

	if !bytes.Equal(first, second) {
		t.Fatalf("duplicate response changed:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestBadPayloadReturnsSafeErrorAndLogoutRejectsFurtherCommands(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-bad-1","op":"move_to","payload":{"target":{"x":"bad","y":0}},"client_seq":1,"v":1}`)
	bad := readError(t, conn)
	if bad.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("bad payload error = %+v, want %s", bad.Error, foundation.CodeInvalidPayload)
	}

	logoutPilot(t, httpServer, cookie)
	writeText(t, conn, `{"request_id":"request-after-logout","op":"world.snapshot","payload":{},"client_seq":2,"v":1}`)
	revoked := readError(t, conn)
	if revoked.Error.Code != foundation.CodeSessionRevoked {
		t.Fatalf("after logout error = %+v, want %s", revoked.Error, foundation.CodeSessionRevoked)
	}
}

func TestBadJSONDoesNotCrashSocketLoop(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `not-json`)
	bad := readError(t, conn)
	if bad.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("bad JSON error = %+v, want %s", bad.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, conn, `{"request_id":"request-after-bad-json","op":"world.snapshot","payload":{},"client_seq":2,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("response after bad JSON = %+v, want success", response)
	}
}

func TestReconnectBootstrapCarriesSnapshotCursor(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)

	firstConn := dialWebSocket(t, httpServer, cookie)
	firstEvents := readBootstrapEvents(t, firstConn)
	firstConn.CloseNow()

	secondConn := dialWebSocket(t, httpServer, cookie)
	defer secondConn.CloseNow()
	secondEvents := readBootstrapEvents(t, secondConn)

	if secondEvents[0].Sequence <= firstEvents[len(firstEvents)-1].Sequence {
		t.Fatalf("reconnect first seq = %d, want after %d", secondEvents[0].Sequence, firstEvents[len(firstEvents)-1].Sequence)
	}
	var ready sessionReadyPayload
	if err := json.Unmarshal(secondEvents[0].Payload, &ready); err != nil {
		t.Fatalf("decode reconnect session.ready: %v", err)
	}
	if ready.ReconnectCursor != firstEvents[len(firstEvents)-1].Sequence {
		t.Fatalf("reconnect cursor = %d, want %d", ready.ReconnectCursor, firstEvents[len(firstEvents)-1].Sequence)
	}
}

func TestShutdownClosesActiveWebSocket(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	readBootstrapEvents(t, conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := gameServer.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}
	readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
	defer readCancel()
	_, _, err := conn.Read(readCtx)
	if err == nil {
		t.Fatal("Read() after Shutdown succeeded, want closed connection")
	}
}

func newTestServer(t *testing.T, devMode bool) (*Server, *httptest.Server) {
	t.Helper()
	gameServer, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		DevMode:        devMode,
		SessionTTL:     time.Hour,
		TickDelta:      50 * time.Millisecond,
		PasswordHasher: auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	httpServer := httptest.NewServer(gameServer.Handler())
	return gameServer, httpServer
}

func registerPilot(t *testing.T, httpServer *httptest.Server) *http.Cookie {
	t.Helper()
	body := strings.NewReader(`{"email":"pilot@example.com","password":"correct-password","callsign":"Frontier-01"}`)
	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/auth/register", body)
	if err != nil {
		t.Fatalf("new register request: %v", err)
	}
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("register request error = %v, want nil", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d, want 201", resp.StatusCode)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == auth.DefaultSessionCookieName {
			return cookie
		}
	}
	t.Fatal("register response missing session cookie")
	return nil
}

func resolvedSessionForCookie(t *testing.T, gameServer *Server, cookie *http.Cookie) auth.ResolvedSession {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	if err != nil {
		t.Fatalf("new resolve request: %v", err)
	}
	req.Header.Set("Origin", testOrigin)
	req.AddCookie(cookie)
	resolved, err := auth.ResolveCookie(context.Background(), gameServer.runtime.Auth, auth.DefaultSessionCookieName, gameServer.config.originPolicy(), req)
	if err != nil {
		t.Fatalf("resolve test cookie: %v", err)
	}
	return resolved
}

func createResolvedRuntimeSession(t *testing.T, gameServer *Server, email string, callsign string) auth.ResolvedSession {
	t.Helper()
	result, err := gameServer.runtime.Auth.Register(context.Background(), auth.RegisterInput{
		Email:    email,
		Password: "correct-password",
		Callsign: callsign,
	})
	if err != nil {
		t.Fatalf("register runtime session: %v", err)
	}
	if err := gameServer.runtime.ensurePlayerSession(result.Session); err != nil {
		t.Fatalf("ensure runtime player session: %v", err)
	}
	return result.Session
}

func setTestShipDisabled(gameServer *Server, playerID foundation.PlayerID, disabled bool) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state := gameServer.runtime.players[playerID]
	state.Ship.Disabled = disabled
	if disabled {
		state.Ship.RepairState = "disabled"
	} else {
		state.Ship.RepairState = "ready"
	}
	gameServer.runtime.players[playerID] = state
}

func hasStatusFlag(flags []aoi.StatusFlag, want aoi.StatusFlag) bool {
	for _, flag := range flags {
		if flag == want {
			return true
		}
	}
	return false
}

func hasEntityID(entities []aoi.EntityPayload, want string) bool {
	for _, entity := range entities {
		if entity.ID.String() == want {
			return true
		}
	}
	return false
}

func decodeWorldSnapshotForTest(t *testing.T, events []realtime.EventEnvelope) worldSnapshotPayload {
	t.Helper()
	var snapshot worldSnapshotPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &snapshot); err != nil {
		t.Fatalf("decode world snapshot: %v", err)
	}
	return snapshot
}

type aoiEntityPayloadForTest struct {
	EntityID   string `json:"entity_id"`
	EntityType string `json:"entity_type"`
}

func logoutPilot(t *testing.T, httpServer *httptest.Server, cookie *http.Cookie) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/auth/logout", nil)
	if err != nil {
		t.Fatalf("new logout request: %v", err)
	}
	req.Header.Set("Origin", testOrigin)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("logout request error = %v, want nil", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", resp.StatusCode)
	}
}

func dialWebSocket(t *testing.T, httpServer *httptest.Server, cookie *http.Cookie) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(httpServer), &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{testOrigin},
			"Cookie": []string{cookie.String()},
		},
	})
	if err != nil {
		t.Fatalf("websocket dial error = %v, want nil", err)
	}
	return conn
}

func wsURL(httpServer *httptest.Server) string {
	return "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
}

func readBootstrapEvents(t *testing.T, conn *websocket.Conn) []realtime.EventEnvelope {
	t.Helper()
	events := make([]realtime.EventEnvelope, 0, 7)
	for len(events) < 7 {
		events = append(events, readEvent(t, conn))
	}
	return events
}

func writeText(t *testing.T, conn *websocket.Conn, payload string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, []byte(payload)); err != nil {
		t.Fatalf("websocket Write() error = %v, want nil", err)
	}
}

func readRawText(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("websocket Read() error = %v, want nil", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("message type = %v, want text", messageType)
	}
	return data
}

func readEvent(t *testing.T, conn *websocket.Conn) realtime.EventEnvelope {
	t.Helper()
	data := readRawText(t, conn)
	var event realtime.EventEnvelope
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("decode event %s: %v", data, err)
	}
	if event.Type == "" || event.Payload == nil {
		t.Fatalf("message %s is not an event", data)
	}
	return event
}

func readResponse(t *testing.T, conn *websocket.Conn) realtime.ResponseEnvelope {
	t.Helper()
	data := readRawText(t, conn)
	var response realtime.ResponseEnvelope
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode response %s: %v", data, err)
	}
	return response
}

func readError(t *testing.T, conn *websocket.Conn) realtime.ErrorEnvelope {
	t.Helper()
	data := readRawText(t, conn)
	var response realtime.ErrorEnvelope
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode error %s: %v", data, err)
	}
	if response.OK {
		t.Fatalf("error response %s had ok=true", data)
	}
	return response
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal(%T) error = %v", value, err)
	}
	return data
}
