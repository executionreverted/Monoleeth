package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
)

func TestShieldRepairTickRequiresEquippedShieldRepairModule(t *testing.T) {
	gameServer, httpServer, _ := newTestServerWithFakeClock(t)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	setTestShipCombatValues(t, gameServer, resolved.PlayerID, 70, 20, 100)

	writeText(t, conn, `{"request_id":"request-shield-repair-no-module","op":"repair.shield_tick","payload":{},"client_seq":1,"v":1}`)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeForbidden {
		t.Fatalf("shield repair without module error = %+v, want %s", got.Error, foundation.CodeForbidden)
	}
	state := testPlayerState(t, gameServer, resolved.PlayerID)
	if state.Ship.Shield != 20 || state.Ship.Hull != 70 {
		t.Fatalf("state after no-module repair = %+v, want unchanged hull/shield", state.Ship)
	}
}

func TestShieldRepairTickRespectsCombatLockAndRepairsOnlyShield(t *testing.T) {
	gameServer, httpServer, clock := newTestServerWithFakeClock(t)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	equipShieldRepairModule(t, conn)
	setTestShipCombatValues(t, gameServer, resolved.PlayerID, 45, 20, 100)
	setTestCombatLock(t, gameServer, resolved.PlayerID, clock.Now().Add(shieldRepairCombatLockDuration))

	writeText(t, conn, `{"request_id":"request-shield-repair-locked","op":"repair.shield_tick","payload":{},"client_seq":3,"v":1}`)
	locked := readErrorSkippingEvents(t, conn)
	if locked.Error.Code != foundation.CodeCooldown {
		t.Fatalf("shield repair during combat lock error = %+v, want %s", locked.Error, foundation.CodeCooldown)
	}
	if state := testPlayerState(t, gameServer, resolved.PlayerID); state.Ship.Shield != 20 || state.Ship.Hull != 45 {
		t.Fatalf("state during combat lock = %+v, want unchanged damaged ship", state.Ship)
	}

	clock.Advance(shieldRepairCombatLockDuration + shieldRepairMinTickInterval)
	writeText(t, conn, `{"request_id":"request-shield-repair-ok","op":"repair.shield_tick","payload":{},"client_seq":4,"v":1}`)
	response := readResponseSkippingEvents(t, conn)
	if !response.OK {
		t.Fatalf("shield repair response = %+v, want success", response)
	}
	var payload shieldRepairTickPayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode shield repair response: %v", err)
	}
	if !payload.Accepted || !payload.Repaired || payload.ShieldBefore != 20 || payload.ShieldAfter != 24 || payload.MaxShield != 100 || payload.RepairRate != 4 {
		t.Fatalf("shield repair payload = %+v, want +4 shield from equipped module", payload)
	}
	state := testPlayerState(t, gameServer, resolved.PlayerID)
	if state.Ship.Hull != 45 || state.Ship.Shield != 24 {
		t.Fatalf("state after shield repair = %+v, want hull unchanged and shield repaired", state.Ship)
	}
	drainEventTypes(t, conn, realtime.EventShipSnapshot, realtime.EventPlayerSnapshot)
}

func TestCombatUseSkillRefreshesShieldRepairCombatLock(t *testing.T) {
	gameServer, httpServer, _ := newTestServerWithFakeClock(t)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})
	gameServer.runtime.tickAndCollectAOIEvents()

	writeText(t, conn, `{"request_id":"request-shield-repair-combat-lock","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
	combatResponse := readResponseSkippingEvents(t, conn)
	if !combatResponse.OK {
		t.Fatalf("combat response = %+v, want success", combatResponse)
	}
	assertTestCombatLocked(t, gameServer, resolved.PlayerID)
}

func assertTestCombatLocked(t *testing.T, gameServer *Server, playerID foundation.PlayerID) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	if lockUntil, ok := gameServer.runtime.combatLocks[playerID]; !ok || !lockUntil.After(gameServer.runtime.clock.Now()) {
		t.Fatalf("combat lock for %q = %s ok=%v, want active", playerID, lockUntil, ok)
	}
}

func setTestCombatLock(t *testing.T, gameServer *Server, playerID foundation.PlayerID, lockUntil time.Time) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	gameServer.runtime.combatLocks[playerID] = lockUntil
}

func TestShieldRepairTickRejectsClientAuthoredRepairTruth(t *testing.T) {
	gameServer, httpServer, clock := newTestServerWithFakeClock(t)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	equipShieldRepairModule(t, conn)
	setTestShipCombatValues(t, gameServer, resolved.PlayerID, 70, 20, 100)
	clock.Advance(shieldRepairCombatLockDuration + shieldRepairMinTickInterval)

	writeText(t, conn, `{"request_id":"request-shield-repair-spoof","op":"repair.shield_tick","payload":{"shield":100,"repair_rate":999,"elapsed_ms":60000},"client_seq":1,"v":1}`)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed shield repair error = %+v, want %s", got.Error, foundation.CodeInvalidPayload)
	}
	state := testPlayerState(t, gameServer, resolved.PlayerID)
	if state.Ship.Shield != 20 || state.Ship.Hull != 70 {
		t.Fatalf("state after spoofed shield repair = %+v, want unchanged", state.Ship)
	}
}

func newTestServerWithFakeClock(t *testing.T) (*Server, *httptest.Server, *testutil.FakeClock) {
	t.Helper()
	clock := testutil.NewFakeClock(time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		DevMode:           false,
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		Clock:             clock,
		ContentRepository: staticContentRepositoryForTest(),
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	return gameServer, httptest.NewServer(gameServer.Handler()), clock
}

func equipShieldRepairModule(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	writeText(t, conn, `{"request_id":"request-shield-repair-inventory","op":"inventory.snapshot","payload":{},"client_seq":1,"v":1}`)
	inventoryResponse := readResponse(t, conn)
	if !inventoryResponse.OK {
		t.Fatalf("inventory response = %+v, want success", inventoryResponse)
	}
	var inventoryPayload struct {
		Inventory inventorySnapshotPayload `json:"inventory"`
	}
	if err := json.Unmarshal(inventoryResponse.Payload, &inventoryPayload); err != nil {
		t.Fatalf("decode inventory snapshot: %v", err)
	}
	shieldID := requireInventoryInstance(t, inventoryPayload.Inventory, "shield_generator_t1", economy.LocationKindAccountInventory.String())
	writeText(t, conn, `{"request_id":"request-shield-repair-equip","op":"loadout.equip_module","payload":{"slot_id":"defensive_1","item_instance_id":"`+shieldID+`"},"client_seq":2,"v":1}`)
	equipResponse := readResponseSkippingEvents(t, conn)
	if !equipResponse.OK {
		t.Fatalf("shield equip response = %+v, want success", equipResponse)
	}
	drainEventTypes(t, conn, realtime.EventInventorySnapshot, realtime.EventLoadoutSnapshot, realtime.EventStatsUpdated)
}

func setTestShipCombatValues(t *testing.T, gameServer *Server, playerID foundation.PlayerID, hull int, shield int, capacitor int) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state, ok := gameServer.runtime.players[playerID]
	if !ok {
		t.Fatalf("player %q missing runtime state", playerID)
	}
	state.Ship.Hull = hull
	state.Ship.Shield = shield
	state.Ship.Capacitor = capacitor
	gameServer.runtime.players[playerID] = state
}

func testPlayerState(t *testing.T, gameServer *Server, playerID foundation.PlayerID) playerRuntimeState {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state, ok := gameServer.runtime.players[playerID]
	if !ok {
		t.Fatalf("player %q missing runtime state", playerID)
	}
	return state
}
