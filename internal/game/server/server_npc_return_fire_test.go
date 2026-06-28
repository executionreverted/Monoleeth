package server

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/worker"
)

func TestNPCReturnFireAggressiveMapDamagesVisiblePlayer(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "npc-return-fire@example.com", "NPC Return Fire", "map_1_3", "west_gate")
	npcID := firstLiveNPCEntityIDForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, npcID, world.Vec2{X: 80})
	forceNPCHitForTest(t, gameServer, npcID)
	before := testShipShieldHullForTest(t, gameServer, resolved.PlayerID)

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	events := eventsBySession[resolved.SessionID]

	requireEventTypeForTest(t, events, realtime.EventCombatShotStarted)
	requireEventTypeForTest(t, events, realtime.EventCombatDamage)
	requireEventTypeForTest(t, events, realtime.EventCombatShotResolved)
	requireEventTypeForTest(t, events, realtime.EventShipSnapshot)
	requireEventTypeForTest(t, events, realtime.EventPlayerSnapshot)
	after := testShipShieldHullForTest(t, gameServer, resolved.PlayerID)
	if after.Shield >= before.Shield && after.Hull >= before.Hull {
		t.Fatalf("ship shield/hull after NPC return fire = %+v before %+v, want damage", after, before)
	}
	assertNPCCombatEventsSafeForTest(t, events)
}

func TestNPCReturnFirePassiveMapsDoNotDamagePlayer(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "npc-passive-map@example.com", "NPC Passive Map", "map_1_2", "west_gate")
	npcID := firstLiveNPCEntityIDForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, npcID, world.Vec2{})
	forceNPCHitForTest(t, gameServer, npcID)
	before := testShipShieldHullForTest(t, gameServer, resolved.PlayerID)

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	events := eventsBySession[resolved.SessionID]

	if eventTypesContain(events, realtime.EventCombatDamage) || eventTypesContain(events, realtime.EventCombatShotStarted) {
		t.Fatalf("passive map NPC return-fire events = %+v, want none", events)
	}
	after := testShipShieldHullForTest(t, gameServer, resolved.PlayerID)
	if after != before {
		t.Fatalf("passive map ship = %+v before %+v, want unchanged", after, before)
	}
}

func TestNPCReturnFireRespectsPlayerProtection(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "npc-protected@example.com", "NPC Protected", "map_1_3", "west_gate")
	npcID := firstLiveNPCEntityIDForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, npcID, world.Vec2{X: 80})
	forceNPCHitForTest(t, gameServer, npcID)
	gameServer.runtime.mu.Lock()
	if _, err := gameServer.runtime.startPlayerProtectionLocked(resolved.PlayerID, "map_1_3", protectionReasonPortal); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("startPlayerProtectionLocked() error = %v, want nil", err)
	}
	gameServer.runtime.mu.Unlock()
	before := testShipShieldHullForTest(t, gameServer, resolved.PlayerID)

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	events := eventsBySession[resolved.SessionID]

	if eventTypesContain(events, realtime.EventCombatDamage) || eventTypesContain(events, realtime.EventCombatShotStarted) {
		t.Fatalf("protected player NPC return-fire events = %+v, want none", events)
	}
	after := testShipShieldHullForTest(t, gameServer, resolved.PlayerID)
	if after != before {
		t.Fatalf("protected player ship = %+v before %+v, want unchanged", after, before)
	}
}

func TestNPCReturnFireSkipsHiddenIneligiblePlayer(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "npc-hidden@example.com", "NPC Hidden", "map_1_3", "west_gate")
	npcID := firstLiveNPCEntityIDForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, npcID, world.Vec2{X: 80})
	forceNPCHitForTest(t, gameServer, npcID)
	gameServer.runtime.mu.Lock()
	instance, _, err := gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("activeMapInstanceLocked() error = %v, want nil", err)
	}
	instance.HiddenPlayers[resolved.PlayerID] = true
	if err := gameServer.runtime.submitWorkerCommandAndRecordMetricsLocked(instance, worker.SetPlayerAggroEligibilityCommand{
		PlayerID: resolved.PlayerID,
		Eligible: false,
	}); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("SetPlayerAggroEligibilityCommand() error = %v, want nil", err)
	}
	gameServer.runtime.mu.Unlock()
	before := testShipShieldHullForTest(t, gameServer, resolved.PlayerID)

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	events := eventsBySession[resolved.SessionID]

	if eventTypesContain(events, realtime.EventCombatDamage) || eventTypesContain(events, realtime.EventCombatShotStarted) {
		t.Fatalf("hidden player NPC return-fire events = %+v, want none", events)
	}
	after := testShipShieldHullForTest(t, gameServer, resolved.PlayerID)
	if after != before {
		t.Fatalf("hidden player ship = %+v before %+v, want unchanged", after, before)
	}
}

type shipShieldHullForTest struct {
	Shield int
	Hull   int
}

func firstLiveNPCEntityIDForTest(t *testing.T, gameServer *Server, playerID foundation.PlayerID) world.EntityID {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	instance, _, err := gameServer.runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		t.Fatalf("activeMapInstanceLocked() error = %v, want nil", err)
	}
	for _, record := range instance.Worker.EnemySpawnSnapshot().Records {
		if record.Alive {
			return record.EntityID
		}
	}
	t.Fatalf("active map %q has no live NPC", instance.Definition.InternalMapID)
	return ""
}

func forceNPCHitForTest(t *testing.T, gameServer *Server, npcID world.EntityID) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	for _, instance := range gameServer.runtime.mapInstances {
		record, ok := instance.Worker.EnemySpawnRecord(npcID)
		if !ok {
			continue
		}
		for index, template := range instance.Definition.NPCStatTemplates {
			if template.StatTemplateID != record.StatTemplateID {
				continue
			}
			template.Accuracy = 1
			instance.Definition.NPCStatTemplates[index] = template
			if err := gameServer.runtime.syncAliveNPCCombatActorProjectionsLocked(instance); err != nil {
				t.Fatalf("syncAliveNPCCombatActorProjectionsLocked() error = %v, want nil", err)
			}
			return
		}
	}
	t.Fatalf("NPC stat template for %q missing", npcID)
}

func testShipShieldHullForTest(t *testing.T, gameServer *Server, playerID foundation.PlayerID) shipShieldHullForTest {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state := gameServer.runtime.players[playerID]
	return shipShieldHullForTest{Shield: state.Ship.Shield, Hull: state.Ship.Hull}
}

func assertNPCCombatEventsSafeForTest(t *testing.T, events []realtime.EventEnvelope) {
	t.Helper()
	for _, event := range events {
		if event.Type != realtime.EventCombatDamage &&
			event.Type != realtime.EventCombatShotStarted &&
			event.Type != realtime.EventCombatShotResolved &&
			event.Type != realtime.EventShipSnapshot &&
			event.Type != realtime.EventPlayerSnapshot {
			continue
		}
		raw := string(event.Payload)
		for _, forbidden := range []string{
			"aggro_profile",
			"leash",
			"enemy_pool",
			"pool_id",
			"spawn_area",
			"hidden_target",
			"target_player_id",
			"gameplay_seed",
		} {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("NPC combat event %s leaked %q in %s", event.Type, forbidden, raw)
			}
		}
		var decoded any
		if err := json.Unmarshal(event.Payload, &decoded); err != nil {
			t.Fatalf("decode NPC combat event %s payload: %v", event.Type, err)
		}
	}
}
