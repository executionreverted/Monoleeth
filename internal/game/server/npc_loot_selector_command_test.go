package server

import (
	"testing"
	"time"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
)

func TestCombatUseSkillSelectorFailureDoesNotLeakQueuedEventsOrKillSpawner(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilotWithIdentity(t, httpServer, "selector-command@example.com", "Selector Command")
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	attackerEntityID := testPlayerEntityID(t, gameServer, resolved.PlayerID)

	targetID := world.EntityID("entity_training_npc")
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, targetID, world.Vec2{})
	_ = gameServer.runtime.tickAndCollectAOIEvents()

	var hpBefore, shieldBefore float64
	gameServer.runtime.mu.Lock()
	starter, _, err := gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("active map instance: %v", err)
	}
	if len(starter.Definition.NPCDropProfiles) == 0 {
		gameServer.runtime.mu.Unlock()
		t.Fatal("starter map has no NPC drop profiles")
	}
	recordBefore, ok := starter.Worker.EnemySpawnRecord(targetID)
	if !ok || !recordBefore.Alive {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("starter spawn record before command = %+v ok=%v, want alive", recordBefore, ok)
	}
	actorBefore, ok := gameServer.runtime.Combat.Actor(targetID)
	if !ok {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("combat actor %q missing before command", targetID)
	}
	gameServer.runtime.mu.Unlock()

	writeText(t, conn, `{"request_id":"request-combat-selector-prime","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
	prime := readResponseSkippingEvents(t, conn)
	if !prime.OK {
		t.Fatalf("selector prime response = %+v, want success", prime)
	}

	gameServer.runtime.mu.Lock()
	starter, _, err = gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("active map instance after prime: %v", err)
	}
	actorBefore, ok = gameServer.runtime.Combat.Actor(targetID)
	if !ok {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("combat actor %q missing after prime", targetID)
	}
	hpBefore = actorBefore.HP
	shieldBefore = actorBefore.Shield
	attacker, ok := gameServer.runtime.Combat.Actor(attackerEntityID)
	if !ok {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("combat actor %q missing after prime", attackerEntityID)
	}
	delete(attacker.Cooldowns, combat.BasicLaserCooldownKey)
	if err := gameServer.runtime.Combat.UpsertActor(attacker); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("clear selector prime cooldown: %v", err)
	}
	starter.Definition.NPCDropProfiles[0].LootTableID = "missing_selector_table"
	gameServer.runtime.mu.Unlock()

	writeText(t, conn, `{"request_id":"request-combat-selector-fail","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":2,"v":1}`)
	got := readErrorSkippingEvents(t, conn)
	if got.Error.Code != foundation.CodeInternal {
		t.Fatalf("selector failure error = %+v, want %s", got.Error, foundation.CodeInternal)
	}

	writeText(t, conn, `{"request_id":"request-selector-fail-drain","op":"death.repair_quote","payload":{},"client_seq":3,"v":1}`)
	drainResponse := readResponse(t, conn)
	if !drainResponse.OK {
		t.Fatalf("drain command response = %+v, want success", drainResponse)
	}
	assertNoRealtimeMessageWithin(t, "selector failure stale post-command events", conn, 100*time.Millisecond)

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	starter, _, err = gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if err != nil {
		t.Fatalf("active map instance after failure: %v", err)
	}
	recordAfter, ok := starter.Worker.EnemySpawnRecord(targetID)
	if !ok {
		t.Fatalf("starter spawn record %q missing after selector failure", targetID)
	}
	if !recordAfter.Alive || !recordAfter.DeadAt.IsZero() || !recordAfter.NextRespawnAt.IsZero() {
		t.Fatalf("starter spawn record after selector failure = %+v, want still alive", recordAfter)
	}
	if _, ok := starter.Worker.Entity(targetID); !ok {
		t.Fatalf("target entity %q removed from worker after selector failure", targetID)
	}
	if starter.HiddenEntities[targetID] {
		t.Fatalf("target entity %q hidden after selector failure", targetID)
	}
	actorAfter, ok := gameServer.runtime.Combat.Actor(targetID)
	if !ok {
		t.Fatalf("combat actor %q missing after selector failure", targetID)
	}
	if actorAfter.Dead || actorAfter.HP != hpBefore || actorAfter.Shield != shieldBefore {
		t.Fatalf("combat actor after selector failure = %+v, want live HP %.2f shield %.2f", actorAfter, hpBefore, shieldBefore)
	}
	if drop, ok := gameServer.runtime.Loot.Drop("drop_1"); ok {
		t.Fatalf("selector failure created fallback drop %+v; want no drop", drop)
	}
	for _, queued := range gameServer.runtime.queuedEvents {
		for _, event := range queued {
			switch event.Type {
			case realtime.EventCombatDamage, realtime.EventCombatCooldownStarted, realtime.EventCombatNPCKilled:
				t.Fatalf("selector failure left queued stale event %+v", event)
			}
		}
	}
}
