package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestServerRejectsNonStarterRuntimeZone(t *testing.T) {
	_, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		WorldID:           "world-1",
		ZoneID:            "zone-1",
		ContentRepository: staticContentRepositoryForTest(),
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err == nil || !strings.Contains(err.Error(), string(worldmaps.StarterMapID)) {
		t.Fatalf("New(non-starter zone) error = %v, want starter map zone validation", err)
	}
}
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
			"map_id",
			"internal_map_id",
			"destination_map_id",
			"destination_spawn_id",
			"worker_id",
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
		realtime.EventProgressionSnapshot,
		realtime.EventWorldSnapshot,
		realtime.EventCombatStateSnapshot,
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
func TestDebugSpawnNPCRejectsOutOfBoundsPosition(t *testing.T) {
	_, httpServer := newTestServer(t, true)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-debug-spawn-oob","op":"debug_spawn_npc","payload":{"entity_id":"debug_npc_oob","position":{"x":10001,"y":0}},"client_seq":1,"v":1}`)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeOutOfRange {
		t.Fatalf("debug spawn out-of-bounds error = %+v, want %s", got.Error, foundation.CodeOutOfRange)
	}
}

func TestProductionRuntimeOmitsDebugCommandHandlers(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	handlers := gameServer.runtime.commandHandlers()

	for _, op := range []realtime.Operation{realtime.OperationDebugSnapshot, realtime.OperationDebugSpawnNPC} {
		if _, ok := handlers[op]; ok {
			t.Fatalf("production handler %s registered, want omitted", op)
		}
	}
}

func TestDevModeRuntimeRegistersDebugCommandHandlers(t *testing.T) {
	gameServer, httpServer := newTestServer(t, true)
	defer httpServer.Close()
	handlers := gameServer.runtime.commandHandlers()

	for _, op := range []realtime.Operation{realtime.OperationDebugSnapshot, realtime.OperationDebugSpawnNPC} {
		if handlers[op] == nil {
			t.Fatalf("dev mode handler %s missing, want registered", op)
		}
	}
}

func TestDevModeRuntimeRecordsDevModeMetric(t *testing.T) {
	gameServer, httpServer := newTestServer(t, true)
	defer httpServer.Close()

	snapshot := gameServer.runtime.Metrics.Snapshot()
	for _, gauge := range snapshot.Gauges {
		if gauge.Name != observability.MetricDevModeEnabled || len(gauge.Labels) != 0 {
			continue
		}
		if gauge.Value != 1 {
			t.Fatalf("dev mode metric value = %d, want 1", gauge.Value)
		}
		return
	}
	t.Fatalf("missing dev mode metric %s in %+v", observability.MetricDevModeEnabled, snapshot.Gauges)
}

func TestProductionWebSocketRejectsUnavailableDebugOperationsAndKeepsSessionSnapshotPublic(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-session-snapshot","op":"session.snapshot","payload":{},"client_seq":1,"v":1}`)
	session := readResponse(t, conn)
	if !session.OK {
		t.Fatalf("session snapshot response = %+v, want success", session)
	}
	if session.RequestID != foundation.RequestID("request-session-snapshot") || session.Version != realtime.CurrentVersion {
		t.Fatalf("session envelope = %+v, want request id/version", session)
	}
	var sessionPayload sessionReadyPayload
	if err := json.Unmarshal(session.Payload, &sessionPayload); err != nil {
		t.Fatalf("decode session snapshot: %v", err)
	}
	if !sessionPayload.Authenticated || sessionPayload.Account == nil || sessionPayload.Account.Email != "pilot@example.com" || sessionPayload.Account.Admin {
		t.Fatalf("session account payload = %+v, want public authenticated pilot account", sessionPayload)
	}
	if sessionPayload.Player == nil || sessionPayload.Player.Callsign != "Frontier-01" || sessionPayload.ProtocolVersion != realtime.CurrentVersion {
		t.Fatalf("session player/protocol payload = %+v, want public player and protocol version", sessionPayload)
	}
	rawSession := string(session.Payload)
	for _, forbidden := range []string{"session_id", "account_id", "player_id", "password"} {
		if strings.Contains(rawSession, forbidden) {
			t.Fatalf("session snapshot leaked %q in %s", forbidden, rawSession)
		}
	}
	assertNoForbiddenLeakCanary(t, "session snapshot", session.Payload)

	for index, body := range []string{
		`{"request_id":"request-debug-snapshot","op":"debug_snapshot","payload":{},"client_seq":2,"v":1}`,
		`{"request_id":"request-debug-spawn","op":"debug_spawn_npc","payload":{"entity_id":"debug_npc","position":{"x":1,"y":2}},"client_seq":3,"v":1}`,
		`{"request_id":"request-debug-spawn-spoof","op":"debug_spawn_npc","payload":{"entity_id":"debug_npc_spoof","position":{"x":1,"y":2},"player_id":"spoof"},"client_seq":4,"v":1}`,
	} {
		writeText(t, conn, body)
		response := readError(t, conn)
		if response.Error.Code != foundation.CodeNotFound {
			t.Fatalf("debug response %d = %+v, want %s", index, response.Error, foundation.CodeNotFound)
		}
		if response.Error.Retryable {
			t.Fatalf("debug response %d retryable = true, want false", index)
		}
		assertNoForbiddenLeakCanary(t, fmt.Sprintf("debug response %d", index), mustJSON(t, response))
		message := strings.ToLower(response.Error.Message)
		if strings.Contains(message, "dev") || strings.Contains(message, "internal") {
			t.Fatalf("debug response leaked internal mode copy: %+v", response.Error)
		}
	}

	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	gameServer.runtime.mu.Lock()
	instance, _, instanceErr := gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if instanceErr != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("activeMapInstanceLocked() error = %v, want nil", instanceErr)
	}
	for _, forbiddenEntity := range []foundation.EntityID{"debug_npc", "debug_npc_spoof"} {
		if _, spawned := instance.Worker.Entity(forbiddenEntity); spawned {
			gameServer.runtime.mu.Unlock()
			t.Fatalf("debug_spawn_npc inserted %s in production mode", forbiddenEntity)
		}
	}
	gameServer.runtime.mu.Unlock()

	writeText(t, conn, `{"request_id":"request-world-after-debug","op":"world.snapshot","payload":{},"client_seq":5,"v":1}`)
	world := readResponse(t, conn)
	if !world.OK {
		t.Fatalf("world snapshot after debug forbids = %+v, want live socket", world)
	}
	var worldPayload worldSnapshotPayload
	if err := json.Unmarshal(world.Payload, &worldPayload); err != nil {
		t.Fatalf("decode world snapshot after debug forbids: %v", err)
	}
	for _, forbiddenEntity := range []string{"debug_npc", "debug_npc_spoof"} {
		if hasEntityID(worldPayload.Entities, forbiddenEntity) {
			t.Fatalf("world snapshot after debug forbids includes %q: %+v", forbiddenEntity, worldPayload.Entities)
		}
	}
}

func TestDebugSnapshotLeakCanaryKeepsWorldProjectionSafe(t *testing.T) {
	_, httpServer := newTestServer(t, true)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-debug-snapshot-canary","op":"debug_snapshot","payload":{},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("debug snapshot response = %+v, want success", response)
	}
	assertNoForbiddenLeakCanary(t, "debug snapshot", response.Payload)

	var snapshot worldSnapshotPayload
	if err := json.Unmarshal(response.Payload, &snapshot); err != nil {
		t.Fatalf("decode debug snapshot: %v", err)
	}
	if snapshot.Map.MapKey == "" || snapshot.Sector.Name == "" {
		t.Fatalf("debug snapshot = %+v, want public world projection", snapshot)
	}
}
func TestRejectTrustedPayloadSharedSensitiveFieldsAndAdminException(t *testing.T) {
	for _, field := range []string{
		"account_id",
		"client_player_id",
		"player_id",
		"session_id",
		"world_id",
		"zone_id",
		"map_id",
		"internal_map_id",
		"public_map_key",
		"map_key",
		"map",
		"source_map_id",
		"source_map_key",
		"source_map",
		"source_spawn_id",
		"destination_map_id",
		"destination_map_key",
		"destination_map",
		"destination_spawn_id",
		"spawn_id",
		"worker",
		"worker_id",
		"map_worker_id",
		"worker_topology",
		"speed",
		"hidden",
		"internal_metadata",
		"gameplay_seed",
		"procedural_seed",
		"world_seed",
		"future_spawn_data",
		"candidate_key",
		"detection_roll",
		"scan_roll",
		"scan_cell",
		"scan_result",
		"scan_candidates",
		"target_player_id",
		"witness_expires_at",
		"hidden_target_metadata",
		"provider",
		"provider_reference",
		"source_return_location",
		"seller_player_id",
		"buyer_player_id",
		"bidder_player_id",
		"winning_player_id",
		"server_total",
		"server_fee",
		"generated_payload",
		"generated_seed",
		"loot_roll",
		"password",
		"password_hash",
		"token",
		"session_token",
		"reset_secret",
		"auth_header",
		"cookie",
	} {
		payload := json.RawMessage(fmt.Sprintf(`{"outer":[{%q:"spoof"}]}`, field))
		err := rejectTrustedPayload(payload)
		if !foundation.IsCode(err, foundation.CodeInvalidPayload) || !strings.Contains(err.Error(), field) {
			t.Fatalf("rejectTrustedPayload(%s) error = %v, want invalid payload naming field", field, err)
		}
	}

	if err := rejectTrustedPayloadAllowing(json.RawMessage(`{"target_player_id":"player-admin-target"}`), "target_player_id"); err != nil {
		t.Fatalf("admin target exception rejected: %v", err)
	}
	err := rejectTrustedPayloadAllowing(json.RawMessage(`{"nested":{"target_player_id":"player-admin-target"}}`), "target_player_id")
	if !foundation.IsCode(err, foundation.CodeInvalidPayload) || !strings.Contains(err.Error(), "target_player_id") {
		t.Fatalf("admin target exception nested target_player_id error = %v, want invalid payload", err)
	}
	err = rejectTrustedPayloadAllowing(json.RawMessage(`{"target_player_id":"player-admin-target","nested":{"player_id":"spoof"}}`), "target_player_id")
	if !foundation.IsCode(err, foundation.CodeInvalidPayload) || !strings.Contains(err.Error(), "player_id") {
		t.Fatalf("admin target exception nested player_id error = %v, want invalid payload", err)
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
func TestRuntimeDetachSettlesMovementBeforeReconnectSnapshot(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		ContentRepository: staticContentRepositoryForTest(),
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
		Clock:             clock,
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	resolved := createResolvedRuntimeSession(t, gameServer, "settle-reconnect@example.com", "Settle")
	if _, err := gameServer.runtime.bootstrapEvents(resolved); err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-detach-move","op":"move_to","payload":{"target":{"x":100,"y":0}},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("move response error = %+v, want success", response.Error)
	}

	clock.Advance(250 * time.Millisecond)
	gameServer.runtime.detachSession(resolved.SessionID)
	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("re-ensure session: %v", err)
	}
	reconnectEvents, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("reconnect bootstrap events: %v", err)
	}
	snapshot := decodeWorldSnapshotForTest(t, reconnectEvents)

	var self *aoi.EntityPayload
	for index := range snapshot.Entities {
		if hasStatusFlag(snapshot.Entities[index].StatusFlags, "self") {
			self = &snapshot.Entities[index]
			break
		}
	}
	if self == nil {
		t.Fatalf("reconnect snapshot entities = %+v, missing self", snapshot.Entities)
	}
	if self.Movement != nil {
		t.Fatalf("reconnect self movement = %+v, want settled/stopped", self.Movement)
	}
	if self.Position.X <= defaultPlayerSpeed*0.05 || self.Position.X >= 100 {
		t.Fatalf("reconnect self x = %v, want settled intermediate position after disconnect", self.Position.X)
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
