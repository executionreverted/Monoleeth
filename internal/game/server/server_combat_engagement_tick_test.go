package server

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
)

func TestCombatEngagementTickFiresWithoutSecondClientAttackCommand(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-tick-fire@example.com", "Combat Tick Fire")
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})

	before, ok := gameServer.runtime.Combat.Actor("entity_training_npc")
	if !ok {
		t.Fatal("target actor missing before combat engagement tick")
	}
	_ = gatewayJSON(t, gameServer, resolved, "request-combat-tick-start", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 1)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID); err != nil {
		t.Fatalf("post combat.start_attack events: %v", err)
	}

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	events := eventsBySession[resolved.SessionID]

	requireEventTypeForTest(t, events, realtime.EventCombatShotStarted)
	requireEventTypeForTest(t, events, realtime.EventCombatCooldownStarted)
	requireEventTypeForTest(t, events, realtime.EventCombatShotResolved)
	requireEventTypeForTest(t, events, realtime.EventCombatStateSnapshot)
	if !eventTypesContain(events, realtime.EventCombatDamage) && !eventTypesContain(events, realtime.EventCombatMiss) {
		t.Fatalf("combat engagement tick events = %+v, want damage or miss", events)
	}
	after, ok := gameServer.runtime.Combat.Actor("entity_training_npc")
	if !ok {
		t.Fatal("target actor missing after combat engagement tick")
	}
	if after.HP == before.HP && after.Shield == before.Shield {
		t.Fatalf("target hp/shield unchanged after tick attack: before=%v/%v after=%v/%v", before.HP, before.Shield, after.HP, after.Shield)
	}
	assertCombatEngagementStillActiveForTest(t, gameServer, resolved.PlayerID, "entity_training_npc")
}

func TestCombatEngagementTickKeepsAttackActiveWhilePlayerMoves(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-tick-move@example.com", "Combat Tick Move")
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})

	_ = gatewayJSON(t, gameServer, resolved, "request-combat-tick-move-start", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 1)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID); err != nil {
		t.Fatalf("post combat.start_attack events: %v", err)
	}
	_ = gatewayJSON(t, gameServer, resolved, "request-combat-tick-move", realtime.OperationMoveTo, map[string]any{
		"target": map[string]any{"x": 25, "y": 0},
	}, 2)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationMoveTo, resolved.PlayerID); err != nil {
		t.Fatalf("post move_to events: %v", err)
	}

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	events := eventsBySession[resolved.SessionID]

	requireEventTypeForTest(t, events, realtime.EventCombatShotStarted)
	assertCombatEngagementStillActiveForTest(t, gameServer, resolved.PlayerID, "entity_training_npc")
}

func TestCombatEngagementTickConsumesSelectedLaserAmmo(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-tick-ammo@example.com", "Combat Tick Ammo")
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})
	mcbDefinition := testStackableDefinition(t, "ammunition_laser_mcb_50", "MCB-50", []economy.TradeFlag{economy.TradeFlagTradeable})
	gameServer.runtime.itemCatalog["ammunition_laser_mcb_50"] = mcbDefinition
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, resolved.PlayerID.String())
	if err != nil {
		t.Fatalf("account inventory location: %v", err)
	}
	addTestInventoryStack(t, gameServer, resolved.PlayerID, mcbDefinition, 2, location, "combat-tick-mcb")

	_ = gatewayJSON(t, gameServer, resolved, "request-combat-tick-select-mcb", realtime.OperationCombatSelectAmmo, map[string]any{
		"family":  "laser",
		"item_id": "ammunition_laser_mcb_50",
	}, 1)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatSelectAmmo, resolved.PlayerID); err != nil {
		t.Fatalf("post combat.select_ammo events: %v", err)
	}
	_ = gatewayJSON(t, gameServer, resolved, "request-combat-tick-ammo-start", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 2)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID); err != nil {
		t.Fatalf("post combat.start_attack events: %v", err)
	}

	events := gameServer.runtime.tickAndCollectAOIEvents()[resolved.SessionID]
	started := requireEventTypeForTest(t, events, realtime.EventCombatShotStarted)
	assertCombatShotAmmoPayloadForTest(t, "selected ammo shot", started.Payload, "ammunition_laser_mcb_50", false, 2, 1)
	if got := gameServer.runtime.Inventory.TotalItemQuantity(resolved.PlayerID, "ammunition_laser_mcb_50", location); got != 1 {
		t.Fatalf("mcb quantity after shot = %d, want 1", got)
	}
}

func TestCombatEngagementTickFallsBackToLCBWhenSelectedAmmoEmpty(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-tick-ammo-fallback@example.com", "Combat Tick Ammo Fallback")
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})
	gameServer.runtime.mu.Lock()
	gameServer.runtime.setActiveCombatAmmoLocked(resolved.PlayerID, "laser", "ammunition_laser_mcb_50")
	gameServer.runtime.mu.Unlock()

	_ = gatewayJSON(t, gameServer, resolved, "request-combat-tick-fallback-start", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 1)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID); err != nil {
		t.Fatalf("post combat.start_attack events: %v", err)
	}

	events := gameServer.runtime.tickAndCollectAOIEvents()[resolved.SessionID]
	started := requireEventTypeForTest(t, events, realtime.EventCombatShotStarted)
	assertCombatShotAmmoPayloadForTest(t, "fallback ammo shot", started.Payload, "ammunition_laser_lcb_10", true, 100, 99)
}

func TestCombatEngagementTickStopsWhenNoLaserAmmoAvailable(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-tick-no-ammo@example.com", "Combat Tick No Ammo")
	equipStarterLaserWithoutAmmoForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})

	_ = gatewayJSON(t, gameServer, resolved, "request-combat-tick-no-ammo-start", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 1)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID); err != nil {
		t.Fatalf("post combat.start_attack events: %v", err)
	}

	events := gameServer.runtime.tickAndCollectAOIEvents()[resolved.SessionID]
	stopped := requireEventTypeForTest(t, events, realtime.EventCombatAttackStopped)
	assertCombatEngagementPayloadForTest(t, "no-ammo tick stop", stopped.Payload, false, "", string(combatStopReasonNotEnoughAmmo))
	assertNoActiveCombatEngagementForTest(t, gameServer, resolved.PlayerID)
}

func TestCombatEngagementTickStopsWhenTargetOutOfRange(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-tick-range@example.com", "Combat Tick Range")
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})

	_ = gatewayJSON(t, gameServer, resolved, "request-combat-tick-range-start", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 1)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID); err != nil {
		t.Fatalf("post combat.start_attack events: %v", err)
	}
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{X: -700})
	setTestRadarRange(gameServer, resolved.PlayerID, 1000)

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	events := eventsBySession[resolved.SessionID]

	stopped := requireEventTypeForTest(t, events, realtime.EventCombatAttackStopped)
	assertCombatEngagementPayloadForTest(t, "out-of-range tick stop", stopped.Payload, false, "", string(combatStopReasonOutOfRange))
	assertNoActiveCombatEngagementForTest(t, gameServer, resolved.PlayerID)
}

func TestCombatEngagementTickStopsWhenTargetKilledAndCreatesLoot(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "combat-tick-kill@example.com", "Combat Tick Kill")
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})
	primeTrainingNPCForOneShot(t, gameServer)

	_ = gatewayJSON(t, gameServer, resolved, "request-combat-tick-kill-start", realtime.OperationCombatStartAttack, map[string]any{
		"target_id": "entity_training_npc",
	}, 1)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatStartAttack, resolved.PlayerID); err != nil {
		t.Fatalf("post combat.start_attack events: %v", err)
	}

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	events := eventsBySession[resolved.SessionID]

	requireEventTypeForTest(t, events, realtime.EventCombatNPCKilled)
	requireEventTypeForTest(t, events, realtime.EventLootCreated)
	stopped := requireEventTypeForTest(t, events, realtime.EventCombatAttackStopped)
	assertCombatEngagementPayloadForTest(t, "target-killed tick stop", stopped.Payload, false, "", string(combatStopReasonTargetDestroyed))
	assertNoActiveCombatEngagementForTest(t, gameServer, resolved.PlayerID)
}

func eventTypesContain(events []realtime.EventEnvelope, want realtime.ClientEventType) bool {
	for _, event := range events {
		if event.Type == want {
			return true
		}
	}
	return false
}

func assertCombatEngagementStillActiveForTest(t *testing.T, gameServer *Server, playerID foundation.PlayerID, targetID string) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	snapshot := gameServer.runtime.combatEngagementSnapshotLocked(playerID, gameServer.runtime.clock.Now())
	if !snapshot.Active || snapshot.TargetID.String() != targetID {
		raw, _ := json.Marshal(snapshot)
		t.Fatalf("combat engagement snapshot = %s, want active target %q", raw, targetID)
	}
}

func assertCombatShotAmmoPayloadForTest(t *testing.T, label string, raw json.RawMessage, itemID string, fallback bool, quantityBefore int64, quantityAfter int64) {
	t.Helper()
	var payload struct {
		Ammo struct {
			ItemID         string `json:"item_id"`
			Fallback       bool   `json:"fallback"`
			QuantityBefore int64  `json:"quantity_before"`
			QuantityAfter  int64  `json:"quantity_after"`
		} `json:"ammo"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode %s shot ammo payload: %v", label, err)
	}
	if payload.Ammo.ItemID != itemID || payload.Ammo.Fallback != fallback || payload.Ammo.QuantityBefore != quantityBefore || payload.Ammo.QuantityAfter != quantityAfter {
		t.Fatalf("%s ammo = %+v, want item=%q fallback=%t before=%d after=%d", label, payload.Ammo, itemID, fallback, quantityBefore, quantityAfter)
	}
}
