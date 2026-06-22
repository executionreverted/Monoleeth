package server

import (
	"encoding/json"
	"fmt"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const seededPVPMapID = worldmaps.MapID("map_1_3")

func TestPvPBlockedByMapPolicyBeforeCombatMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	attacker := createResolvedRuntimeSession(t, gameServer, "pvp-policy-attacker@example.com", "Policy Attacker")
	target := createResolvedRuntimeSession(t, gameServer, "pvp-policy-target@example.com", "Policy Target")
	moveTestPlayerEntity(gameServer, attacker.PlayerID, world.Vec2{X: 500, Y: 500})
	moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: 520, Y: 500})

	response := requestPlayerAttackForTest(t, gameServer, attacker, target)

	if !response.HasError || response.Error.Error.Code != foundation.CodePVPBlocked {
		t.Fatalf("pvp policy response = %+v, want %s", response, foundation.CodePVPBlocked)
	}
	assertPvPBlockedNoMutationForTest(t, gameServer, attacker.PlayerID, target.PlayerID)
}

func TestSeededPVPMapSafeZoneBlocksPvPBeforeCombatMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	attacker := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-safe-attacker@example.com", "Safe Attacker", seededPVPMapID, "west_gate")
	target := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-safe-target@example.com", "Safe Target", seededPVPMapID, "west_gate")
	moveTestPlayerEntity(gameServer, attacker.PlayerID, world.Vec2{X: 400, Y: 5000})
	moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: 410, Y: 5000})

	response := requestPlayerAttackForTest(t, gameServer, attacker, target)

	if !response.HasError || response.Error.Error.Code != foundation.CodePVPBlocked {
		t.Fatalf("safe-zone pvp response = %+v, want %s", response, foundation.CodePVPBlocked)
	}
	assertPvPBlockedNoMutationForTest(t, gameServer, attacker.PlayerID, target.PlayerID)
}

func TestSeededPVPMapProtectionBlocksBeforeCombatMutationAndInitiationBreaksProtection(t *testing.T) {
	t.Run("target protection blocks incoming pvp", func(t *testing.T) {
		gameServer, _ := newTestServer(t, false)
		attacker := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-target-protection-attacker@example.com", "Target Protection Attacker", seededPVPMapID, "west_gate")
		target := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-target-protection-target@example.com", "Target Protection Target", seededPVPMapID, "west_gate")
		moveTestPlayerEntity(gameServer, attacker.PlayerID, world.Vec2{X: 500, Y: 500})
		moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: 520, Y: 500})
		startTestProtection(t, gameServer, target.PlayerID, seededPVPMapID)

		response := requestPlayerAttackForTest(t, gameServer, attacker, target)

		if !response.HasError || response.Error.Error.Code != foundation.CodePVPBlocked {
			t.Fatalf("target-protected pvp response = %+v, want %s", response, foundation.CodePVPBlocked)
		}
		assertPvPBlockedNoMutationForTest(t, gameServer, attacker.PlayerID, target.PlayerID)
		assertTestProtectionActive(t, gameServer, target.PlayerID, true)
	})

	t.Run("attacker protection breaks on pvp initiation", func(t *testing.T) {
		gameServer, _ := newTestServer(t, false)
		attacker := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-attacker-protection-attacker@example.com", "Attacker Protection Attacker", seededPVPMapID, "west_gate")
		target := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-attacker-protection-target@example.com", "Attacker Protection Target", seededPVPMapID, "west_gate")
		moveTestPlayerEntity(gameServer, attacker.PlayerID, world.Vec2{X: 500, Y: 500})
		moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: 520, Y: 500})
		startTestProtection(t, gameServer, attacker.PlayerID, seededPVPMapID)

		response := requestPlayerAttackForTest(t, gameServer, attacker, target)

		if !response.HasError || response.Error.Error.Code != foundation.CodePVPBlocked {
			t.Fatalf("attacker-protected pvp response = %+v, want %s", response, foundation.CodePVPBlocked)
		}
		assertPvPBlockedNoMutationForTest(t, gameServer, attacker.PlayerID, target.PlayerID)
		assertTestProtectionActive(t, gameServer, attacker.PlayerID, false)
		gameServer.runtime.mu.Lock()
		events := gameServer.runtime.drainQueuedEventsLocked(attacker.SessionID)
		gameServer.runtime.mu.Unlock()
		protectionEvent := requireEventTypeForTest(t, events, realtime.EventPlayerProtection)
		var payload playerProtectionUpdatedPayload
		if err := json.Unmarshal(protectionEvent.Payload, &payload); err != nil {
			t.Fatalf("decode protection cleared event: %v", err)
		}
		if payload.Reason != protectionReasonPVPAction || payload.PublicMapKey != "1-3" || payload.BlocksPVP || payload.BreakOnPVPAction {
			t.Fatalf("protection cleared payload = %+v, want inactive pvp_action on public map", payload)
		}
	})
}

func TestSeededPVPMapOutsideSafeZoneAllowsPvPPersistsTargetPlayerStateAndEvents(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	attacker := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-allowed-attacker@example.com", "Allowed Attacker", seededPVPMapID, "west_gate")
	target := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-allowed-target@example.com", "Allowed Target", seededPVPMapID, "west_gate")
	moveTestPlayerEntity(gameServer, attacker.PlayerID, world.Vec2{X: 500, Y: 500})
	moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: 520, Y: 500})

	response := requestPlayerAttackForTest(t, gameServer, attacker, target)

	if response.HasError {
		t.Fatalf("allowed pvp response = %+v, want success", response.Error)
	}
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	gameServer.runtime.mu.Lock()
	targetState := gameServer.runtime.players[target.PlayerID]
	targetActor, actorOK := gameServer.runtime.Combat.Actor(targetEntityID)
	targetEvents := gameServer.runtime.drainQueuedEventsLocked(target.SessionID)
	gameServer.runtime.mu.Unlock()
	if !actorOK {
		t.Fatalf("target combat actor %q missing after allowed pvp", targetEntityID)
	}
	if targetState.Ship.Shield != roundCombatValue(targetActor.Shield) ||
		targetState.Ship.Hull != roundCombatValue(targetActor.HP) ||
		targetState.Ship.Capacitor != roundCombatValue(targetActor.Energy) {
		t.Fatalf("target runtime ship = %+v, actor = %+v, want persisted actor state", targetState.Ship, targetActor)
	}
	if targetState.Ship.Shield >= 100 && targetState.Ship.Hull >= 100 {
		t.Fatalf("target runtime ship = %+v, want damage persisted", targetState.Ship)
	}
	shipEvent := requireEventTypeForTest(t, targetEvents, realtime.EventShipSnapshot)
	playerEvent := requireEventTypeForTest(t, targetEvents, realtime.EventPlayerSnapshot)
	requireEventTypeForTest(t, targetEvents, realtime.EventCombatDamage)
	requireEventTypeForTest(t, targetEvents, realtime.EventTargetUpdated)
	var shipPayload shipSnapshotPayload
	if err := json.Unmarshal(shipEvent.Payload, &shipPayload); err != nil {
		t.Fatalf("decode target ship event: %v", err)
	}
	if shipPayload.Hull != targetState.Ship.Hull || shipPayload.Shield != targetState.Ship.Shield || shipPayload.Capacitor != targetState.Ship.Capacitor {
		t.Fatalf("target ship event = %+v, want runtime ship %+v", shipPayload, targetState.Ship)
	}
	var playerPayload playerSnapshotPayload
	if err := json.Unmarshal(playerEvent.Payload, &playerPayload); err != nil {
		t.Fatalf("decode target player event: %v", err)
	}
	if playerPayload.HP != targetState.Ship.Hull || playerPayload.Shield != targetState.Ship.Shield || playerPayload.Energy != targetState.Ship.Capacitor {
		t.Fatalf("target player event = %+v, want runtime ship %+v", playerPayload, targetState.Ship)
	}
}

func TestPvEAllowedInSafeAndPVEMap(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "pve-safe-map@example.com", "PvE Safe")
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-pve-safe-policy","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`),
	)

	if response.HasError {
		t.Fatalf("pve response error = %+v, want success", response.Error)
	}
	if capacitor := testShipCapacitor(gameServer, resolved.PlayerID); capacitor != 100-runtimeBasicLaserEnergyCost {
		t.Fatalf("pve capacitor = %d, want %d", capacitor, 100-runtimeBasicLaserEnergyCost)
	}
}

func requestPlayerAttackForTest(t *testing.T, gameServer *Server, attacker auth.ResolvedSession, target auth.ResolvedSession) realtime.CachedResponse {
	t.Helper()
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	return gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(attacker.SessionID.String()),
		[]byte(fmt.Sprintf(
			`{"request_id":"request-pvp-%s","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":%q},"client_seq":1,"v":1}`,
			attacker.PlayerID,
			targetEntityID.String(),
		)),
	)
}

func assertPvPBlockedNoMutationForTest(t *testing.T, gameServer *Server, attackerID foundation.PlayerID, targetID foundation.PlayerID) {
	t.Helper()
	if capacitor := testShipCapacitor(gameServer, attackerID); capacitor != 100 {
		t.Fatalf("blocked pvp attacker capacitor = %d, want unchanged 100", capacitor)
	}
	attackerEntityID := testPlayerEntityID(t, gameServer, attackerID)
	targetEntityID := testPlayerEntityID(t, gameServer, targetID)
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	now := gameServer.runtime.clock.Now()
	attacker, attackerOK := gameServer.runtime.Combat.Actor(attackerEntityID)
	target, targetOK := gameServer.runtime.Combat.Actor(targetEntityID)
	if !attackerOK || !attacker.Cooldowns.Ready(combat.BasicLaserCooldownKey, now) {
		t.Fatalf("blocked pvp attacker actor = %+v ok=%v, want no cooldown", attacker, attackerOK)
	}
	targetState := gameServer.runtime.players[targetID]
	if !targetOK || target.HP != float64(targetState.Ship.Hull) || target.Shield != float64(targetState.Ship.Shield) {
		t.Fatalf("blocked pvp target actor = %+v ok=%v, want unchanged hull/shield", target, targetOK)
	}
}

func startTestProtection(t *testing.T, gameServer *Server, playerID foundation.PlayerID, mapID worldmaps.MapID) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	if _, err := gameServer.runtime.startPortalProtectionLocked(playerID, mapID); err != nil {
		t.Fatalf("startPortalProtectionLocked() error = %v, want nil", err)
	}
}

func assertTestProtectionActive(t *testing.T, gameServer *Server, playerID foundation.PlayerID, want bool) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	_, got := gameServer.runtime.activeProtectionLocked(playerID)
	if got != want {
		t.Fatalf("active protection for %q = %v, want %v", playerID, got, want)
	}
}
