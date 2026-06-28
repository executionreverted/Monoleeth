package server

import (
	"bytes"
	"encoding/json"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
)

func TestCombatStartAttackRejectsClientAuthoredGameplayTruth(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-start-spoof@example.com", "Combat Start Spoof")

	response := combatEngagementGatewayRequest(t, gameServer, resolved, "request-combat-start-spoof", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
		"player_id": "spoofed-player",
		"damage":    9999,
		"cooldown":  0,
		"position":  map[string]any{"x": 10, "y": 20},
	}, 1)

	if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed combat.start_attack response = %+v, want %s", response, foundation.CodeInvalidPayload)
	}
	assertNoActiveCombatEngagementForTest(t, gameServer, resolved.PlayerID)
}

func TestCombatStartAttackRejectsMissingTargetID(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-start-missing@example.com", "Combat Start Missing")

	response := combatEngagementGatewayRequest(t, gameServer, resolved, "request-combat-start-missing", realtime.OperationCombatStartAttack, map[string]any{}, 1)

	if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("missing target combat.start_attack response = %+v, want %s", response, foundation.CodeInvalidPayload)
	}
	assertNoActiveCombatEngagementForTest(t, gameServer, resolved.PlayerID)
}

func TestCombatStartAttackRejectsHiddenAndOutOfRangeBeforeStateMutation(t *testing.T) {
	t.Run("hidden target", func(t *testing.T) {
		gameServer, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		resolved := createResolvedRuntimeSession(t, gameServer, "combat-start-hidden@example.com", "Combat Start Hidden")
		equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
		setTestHidden(gameServer, "entity_training_npc", true)

		response := combatEngagementGatewayRequest(t, gameServer, resolved, "request-combat-start-hidden", realtime.OperationCombatStartAttack, map[string]any{
			"target_id": "entity_training_npc",
		}, 1)

		if !response.HasError || response.Error.Error.Code != foundation.CodeNotVisible {
			t.Fatalf("hidden combat.start_attack response = %+v, want %s", response, foundation.CodeNotVisible)
		}
		assertNoActiveCombatEngagementForTest(t, gameServer, resolved.PlayerID)
	})

	t.Run("out of range", func(t *testing.T) {
		gameServer, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		resolved := createResolvedRuntimeSession(t, gameServer, "combat-start-range@example.com", "Combat Start Range")
		equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
		moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{X: -700})
		setTestRadarRange(gameServer, resolved.PlayerID, 1000)

		response := combatEngagementGatewayRequest(t, gameServer, resolved, "request-combat-start-range", realtime.OperationCombatStartAttack, map[string]any{
			"target_id": "entity_training_npc",
		}, 1)

		if !response.HasError || response.Error.Error.Code != foundation.CodeOutOfRange {
			t.Fatalf("out-of-range combat.start_attack response = %+v, want %s", response, foundation.CodeOutOfRange)
		}
		assertNoActiveCombatEngagementForTest(t, gameServer, resolved.PlayerID)
	})
}

func TestCombatStartAttackQueuesStartedAndStateSnapshot(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-start-valid@example.com", "Combat Start Valid")
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})

	response := combatEngagementGatewayRequest(t, gameServer, resolved, "request-combat-start-valid", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 1)
	if response.HasError {
		t.Fatalf("valid combat.start_attack response error = %+v, want success", response.Error)
	}
	var payload struct {
		Accepted bool   `json:"accepted"`
		TargetID string `json:"target_id"`
		SkillID  string `json:"skill_id"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode combat.start_attack payload: %v", err)
	}
	if !payload.Accepted || payload.TargetID != "entity_training_npc" || payload.SkillID != defaultCombatEngagementSkillID {
		t.Fatalf("combat.start_attack payload = %+v, want accepted target basic laser", payload)
	}

	events, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post combat.start_attack events: %v", err)
	}
	started := requireEventTypeForTest(t, events, realtime.EventCombatAttackStarted)
	snapshot := requireEventTypeForTest(t, events, realtime.EventCombatStateSnapshot)
	assertCombatEngagementPayloadForTest(t, "attack_started", started.Payload, true, "entity_training_npc", "")
	assertCombatEngagementPayloadForTest(t, "state_snapshot", snapshot.Payload, true, "entity_training_npc", "")
}

func TestCombatStopAttackQueuesStoppedAndStateSnapshot(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-stop-valid@example.com", "Combat Stop Valid")
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})
	_ = gatewayJSON(t, gameServer, resolved, "request-combat-stop-prime", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 1)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID); err != nil {
		t.Fatalf("post prime start events: %v", err)
	}

	response := combatEngagementGatewayRequest(t, gameServer, resolved, "request-combat-stop-valid", realtime.OperationCombatStopAttack, map[string]any{}, 2)
	if response.HasError {
		t.Fatalf("valid combat.stop_attack response error = %+v, want success", response.Error)
	}

	events, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStopAttack, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post combat.stop_attack events: %v", err)
	}
	stopped := requireEventTypeForTest(t, events, realtime.EventCombatAttackStopped)
	snapshot := requireEventTypeForTest(t, events, realtime.EventCombatStateSnapshot)
	assertCombatEngagementPayloadForTest(t, "attack_stopped", stopped.Payload, false, "", string(combatStopReasonManual))
	assertCombatEngagementPayloadForTest(t, "stopped state_snapshot", snapshot.Payload, false, "", string(combatStopReasonManual))
}

func TestCombatStateReturnsCurrentPublicSnapshot(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-state@example.com", "Combat State")
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})
	_ = gatewayJSON(t, gameServer, resolved, "request-combat-state-prime", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 1)

	payload := gatewayJSON(t, gameServer, resolved, "request-combat-state", realtime.OperationCombatState, map[string]any{}, 2)
	assertCombatEngagementPayloadForTest(t, "combat.state response", payload, true, "entity_training_npc", "")
}

func TestCombatSelectAmmoRejectsClientAuthoredGameplayTruth(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-ammo-spoof@example.com", "Combat Ammo Spoof")

	response := combatEngagementGatewayRequest(t, gameServer, resolved, "request-combat-ammo-spoof", realtime.OperationCombatSelectAmmo, map[string]any{
		"family":            "laser",
		"item_id":           "ammunition_laser_mcb_50",
		"quantity":          999,
		"damage_multiplier": 99,
	}, 1)

	if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed combat.select_ammo response = %+v, want %s", response, foundation.CodeInvalidPayload)
	}
}

func TestCombatSelectAmmoRequiresOwnedSelectableAmmo(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-ammo-owned@example.com", "Combat Ammo Owned")
	gameServer.runtime.itemCatalog["ammunition_laser_mcb_50"] = testStackableDefinition(t, "ammunition_laser_mcb_50", "MCB-50", []economy.TradeFlag{economy.TradeFlagTradeable})

	response := combatEngagementGatewayRequest(t, gameServer, resolved, "request-combat-ammo-missing", realtime.OperationCombatSelectAmmo, map[string]any{
		"family":  "laser",
		"item_id": "ammunition_laser_mcb_50",
	}, 1)

	if !response.HasError || response.Error.Error.Code != foundation.CodeNotEnoughAmmo {
		t.Fatalf("missing combat.select_ammo response = %+v, want %s", response, foundation.CodeNotEnoughAmmo)
	}
}

func TestCombatSelectAmmoStoresServerOwnedSelectionAndQueuesState(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-ammo-select@example.com", "Combat Ammo Select")
	definition := testStackableDefinition(t, "ammunition_laser_mcb_50", "MCB-50", []economy.TradeFlag{economy.TradeFlagTradeable})
	gameServer.runtime.itemCatalog["ammunition_laser_mcb_50"] = definition
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, resolved.PlayerID.String())
	if err != nil {
		t.Fatalf("account inventory location: %v", err)
	}
	addTestInventoryStack(t, gameServer, resolved.PlayerID, definition, 25, location, "combat-ammo-select")

	response := combatEngagementGatewayRequest(t, gameServer, resolved, "request-combat-ammo-select", realtime.OperationCombatSelectAmmo, map[string]any{
		"family":  "laser",
		"item_id": "ammunition_laser_mcb_50",
	}, 1)
	if response.HasError {
		t.Fatalf("valid combat.select_ammo response error = %+v, want success", response.Error)
	}
	assertCombatAmmoSelectionPayloadForTest(t, "combat.select_ammo response", response.Response.Payload, "laser", "ammunition_laser_mcb_50", 25, 3)

	events, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatSelectAmmo, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post combat.select_ammo events: %v", err)
	}
	snapshot := requireEventTypeForTest(t, events, realtime.EventCombatStateSnapshot)
	assertCombatAmmoSelectionPayloadForTest(t, "combat.select_ammo state snapshot", snapshot.Payload, "laser", "ammunition_laser_mcb_50", 25, 3)
}

func TestBootstrapIncludesActiveCombatStateSnapshot(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-bootstrap@example.com", "Combat Bootstrap")
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})
	_ = gatewayJSON(t, gameServer, resolved, "request-combat-bootstrap-start", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 1)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID); err != nil {
		t.Fatalf("post combat.start_attack events: %v", err)
	}

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrapEvents(active combat) error = %v, want nil", err)
	}
	snapshot := requireEventTypeForTest(t, events, realtime.EventCombatStateSnapshot)
	assertCombatEngagementPayloadForTest(t, "bootstrap combat state", snapshot.Payload, true, "entity_training_npc", "")
}

func TestCombatStartAttackDuplicateRequestIDReplaysWithoutDuplicateEvents(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-start-duplicate@example.com", "Combat Start Duplicate")
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})

	request := combatEngagementRequestBodyForTest(t, "request-combat-start-duplicate", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 1)
	first := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), request)
	if first.HasError {
		t.Fatalf("first combat.start_attack response error = %+v, want success", first.Error)
	}
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID); err != nil {
		t.Fatalf("post first combat.start_attack events: %v", err)
	}
	replay := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), request)
	if replay.HasError {
		t.Fatalf("duplicate combat.start_attack response error = %+v, want cached success", replay.Error)
	}
	if !bytes.Equal(replay.Response.Payload, first.Response.Payload) {
		t.Fatalf("duplicate combat.start_attack payload changed:\nfirst=%s\nreplay=%s", first.Response.Payload, replay.Response.Payload)
	}
	events, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post duplicate combat.start_attack events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("duplicate combat.start_attack queued events = %+v, want none from cached replay", events)
	}
}

func combatEngagementGatewayRequest(t *testing.T, gameServer *Server, resolved auth.ResolvedSession, requestID string, op realtime.Operation, payload map[string]any, clientSeq uint64) realtime.CachedResponse {
	t.Helper()
	return gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), combatEngagementRequestBodyForTest(t, requestID, op, payload, clientSeq))
}

func combatEngagementRequestBodyForTest(t *testing.T, requestID string, op realtime.Operation, payload map[string]any, clientSeq uint64) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"request_id": requestID,
		"op":         op,
		"payload":    payload,
		"client_seq": clientSeq,
		"v":          1,
	})
	if err != nil {
		t.Fatalf("marshal combat engagement request %s: %v", requestID, err)
	}
	return body
}

func assertNoActiveCombatEngagementForTest(t *testing.T, gameServer *Server, playerID foundation.PlayerID) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	if snapshot := gameServer.runtime.combatEngagementSnapshotLocked(playerID, gameServer.runtime.clock.Now()); snapshot.Active {
		t.Fatalf("combat engagement snapshot = %+v, want inactive", snapshot)
	}
}

func assertCombatEngagementPayloadForTest(t *testing.T, label string, raw json.RawMessage, wantActive bool, wantTargetID string, wantStopReason string) {
	t.Helper()
	var payload struct {
		Active         bool   `json:"active"`
		TargetID       string `json:"target_id"`
		SkillID        string `json:"skill_id"`
		StartedAtMS    int64  `json:"started_at_ms"`
		NextFireAtMS   int64  `json:"next_fire_at_ms"`
		LastStopReason string `json:"last_stop_reason"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode %s combat engagement payload: %v", label, err)
	}
	if payload.Active != wantActive || payload.TargetID != wantTargetID || payload.LastStopReason != wantStopReason {
		t.Fatalf("%s payload = %+v, want active=%t target=%q reason=%q", label, payload, wantActive, wantTargetID, wantStopReason)
	}
	if wantActive {
		if payload.SkillID != defaultCombatEngagementSkillID || payload.StartedAtMS == 0 || payload.NextFireAtMS == 0 {
			t.Fatalf("%s active payload = %+v, want skill/timing", label, payload)
		}
	}
}

func assertCombatAmmoSelectionPayloadForTest(t *testing.T, label string, raw json.RawMessage, family string, itemID string, quantity int64, multiplier float64) {
	t.Helper()
	var payload struct {
		ActiveAmmo map[string]struct {
			ItemID           string  `json:"item_id"`
			Quantity         int64   `json:"quantity"`
			DamageMultiplier float64 `json:"power_multiplier"`
		} `json:"active_ammo"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode %s ammo payload: %v", label, err)
	}
	selected, ok := payload.ActiveAmmo[family]
	if !ok {
		t.Fatalf("%s active_ammo = %+v, missing family %q", label, payload.ActiveAmmo, family)
	}
	if selected.ItemID != itemID || selected.Quantity != quantity || selected.DamageMultiplier != multiplier {
		t.Fatalf("%s selected ammo = %+v, want item=%q quantity=%d multiplier=%.1f", label, selected, itemID, quantity, multiplier)
	}
}
