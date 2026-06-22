package server

import (
	"encoding/json"
	"fmt"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
)

type lethalPVPPayloadForTest struct {
	Accepted bool `json:"accepted"`
	Killed   bool `json:"killed"`
	Drops    []struct {
		DropID   string `json:"drop_id"`
		EntityID string `json:"entity_id"`
		ItemID   string `json:"item_id"`
		Quantity int64  `json:"quantity"`
	} `json:"drops"`
}

func TestSeededPVPMapSameClientRequestIDDoesNotCollideAcrossDeaths(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	attackerOne := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-shared-request-attacker-1@example.com", "Shared Request Attacker 1", seededPVPMapID, "west_gate")
	targetOne := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-shared-request-target-1@example.com", "Shared Request Target 1", seededPVPMapID, "west_gate")
	attackerTwo := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-shared-request-attacker-2@example.com", "Shared Request Attacker 2", seededPVPMapID, "west_gate")
	targetTwo := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-shared-request-target-2@example.com", "Shared Request Target 2", seededPVPMapID, "west_gate")
	moveTestPlayerEntity(gameServer, attackerOne.PlayerID, world.Vec2{X: 500, Y: 500})
	moveTestPlayerEntity(gameServer, targetOne.PlayerID, world.Vec2{X: 520, Y: 500})
	moveTestPlayerEntity(gameServer, attackerTwo.PlayerID, world.Vec2{X: 700, Y: 500})
	moveTestPlayerEntity(gameServer, targetTwo.PlayerID, world.Vec2{X: 720, Y: 500})
	setTestPlayerShipCombatValues(t, gameServer, targetOne.PlayerID, 1, 0, 100)
	setTestPlayerShipCombatValues(t, gameServer, targetTwo.PlayerID, 1, 0, 100)
	addTestCargoStack(t, gameServer, targetOne.PlayerID, "raw_ore", 3, "shared-request-target-one-raw-ore")
	addTestCargoStack(t, gameServer, targetTwo.PlayerID, "raw_ore", 4, "shared-request-target-two-raw-ore")

	sharedRequestID := foundation.RequestID("request-pvp-death-shared")
	first := performLethalPVPAttackForTest(t, gameServer, attackerOne, targetOne, sharedRequestID)
	second := performLethalPVPAttackForTest(t, gameServer, attackerTwo, targetTwo, sharedRequestID)

	firstDropID := world.EntityID(first.Drops[0].DropID)
	secondDropID := world.EntityID(second.Drops[0].DropID)
	if firstDropID == secondDropID {
		t.Fatalf("shared request deaths reused drop id %q, want independent death keys", firstDropID)
	}

	gameServer.runtime.mu.Lock()
	firstState := gameServer.runtime.players[targetOne.PlayerID]
	secondState := gameServer.runtime.players[targetTwo.PlayerID]
	instance, instanceErr := gameServer.runtime.mapInstanceLocked(seededPVPMapID)
	var firstWorkerDropOK bool
	var secondWorkerDropOK bool
	if instanceErr == nil {
		_, firstWorkerDropOK = instance.Worker.Entity(firstDropID)
		_, secondWorkerDropOK = instance.Worker.Entity(secondDropID)
	}
	gameServer.runtime.mu.Unlock()
	if instanceErr != nil {
		t.Fatalf("pvp map instance: %v", instanceErr)
	}
	if !firstState.Ship.Disabled || !secondState.Ship.Disabled {
		t.Fatalf("shared request target ships disabled = %v/%v, want both disabled", firstState.Ship.Disabled, secondState.Ship.Disabled)
	}
	if !firstWorkerDropOK || !secondWorkerDropOK {
		t.Fatalf("shared request worker drops present = %v/%v, want both", firstWorkerDropOK, secondWorkerDropOK)
	}
	assertStoredPVPDeathDropForTest(t, gameServer, firstDropID, attackerOne.PlayerID, "raw_ore", 3)
	assertStoredPVPDeathDropForTest(t, gameServer, secondDropID, attackerTwo.PlayerID, "raw_ore", 4)
}

func TestSeededPVPMapDuplicateDeathResultDoesNotReapplyLootOrEvents(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	attacker := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-duplicate-death-attacker@example.com", "Duplicate Death Attacker", seededPVPMapID, "west_gate")
	target := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-duplicate-death-target@example.com", "Duplicate Death Target", seededPVPMapID, "west_gate")
	moveTestPlayerEntity(gameServer, attacker.PlayerID, world.Vec2{X: 500, Y: 500})
	moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: 520, Y: 500})
	setTestPlayerShipCombatValues(t, gameServer, target.PlayerID, 1, 0, 100)
	addTestCargoStack(t, gameServer, target.PlayerID, "raw_ore", 3, "duplicate-death-raw-ore")

	requestID := foundation.RequestID("request-pvp-death-duplicate-runtime")
	payload := performLethalPVPAttackForTest(t, gameServer, attacker, target, requestID)
	dropID := world.EntityID(payload.Drops[0].DropID)

	var duplicateErr error
	var duplicateDropCount int
	var attackerEventCount int
	var targetEventCount int
	var workerDropOK bool
	func() {
		gameServer.runtime.mu.Lock()
		defer gameServer.runtime.mu.Unlock()
		gameServer.runtime.drainQueuedEventsLocked(attacker.SessionID)
		gameServer.runtime.drainQueuedEventsLocked(target.SessionID)

		attackerActor, attackerOK := gameServer.runtime.Combat.Actor(gameServer.runtime.players[attacker.PlayerID].EntityID)
		targetActor, targetOK := gameServer.runtime.Combat.Actor(gameServer.runtime.players[target.PlayerID].EntityID)
		if !attackerOK || !targetOK {
			t.Fatalf("combat actors after first death present = %v/%v, want both", attackerOK, targetOK)
		}
		duplicateDrops, err := gameServer.runtime.processLethalPVPDeathLocked(requestID, attackerActor, targetActor)
		duplicateErr = err
		duplicateDropCount = len(duplicateDrops)
		attackerEventCount = len(gameServer.runtime.drainQueuedEventsLocked(attacker.SessionID))
		targetEventCount = len(gameServer.runtime.drainQueuedEventsLocked(target.SessionID))
		instance, instanceErr := gameServer.runtime.mapInstanceLocked(seededPVPMapID)
		if instanceErr != nil {
			t.Fatalf("pvp map instance: %v", instanceErr)
		}
		_, workerDropOK = instance.Worker.Entity(dropID)
	}()

	if duplicateErr != nil {
		t.Fatalf("duplicate ProcessDeath runtime path error = %v, want nil no-op", duplicateErr)
	}
	if duplicateDropCount != 0 {
		t.Fatalf("duplicate ProcessDeath runtime drops = %d, want 0 newly-created drops", duplicateDropCount)
	}
	if attackerEventCount != 0 || targetEventCount != 0 {
		t.Fatalf("duplicate ProcessDeath queued events attacker=%d target=%d, want none", attackerEventCount, targetEventCount)
	}
	if !workerDropOK {
		t.Fatalf("original player-death drop %q missing after duplicate no-op", dropID)
	}
}

func performLethalPVPAttackForTest(
	t *testing.T,
	gameServer *Server,
	attacker auth.ResolvedSession,
	target auth.ResolvedSession,
	requestID foundation.RequestID,
) lethalPVPPayloadForTest {
	t.Helper()
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(attacker.SessionID.String()),
		[]byte(fmt.Sprintf(
			`{"request_id":%q,"op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":%q},"client_seq":1,"v":1}`,
			requestID.String(),
			targetEntityID.String(),
		)),
	)
	if response.HasError {
		t.Fatalf("lethal pvp response = %+v, want success", response.Error)
	}
	var payload lethalPVPPayloadForTest
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode lethal pvp response: %v", err)
	}
	if !payload.Accepted || !payload.Killed || len(payload.Drops) != 1 {
		t.Fatalf("lethal pvp payload = %+v, want accepted killed with one cargo drop", payload)
	}
	if payload.Drops[0].DropID == "" || payload.Drops[0].EntityID != payload.Drops[0].DropID {
		t.Fatalf("lethal pvp drop identity = %+v, want public drop/entity id", payload.Drops[0])
	}
	return payload
}

func assertStoredPVPDeathDropForTest(
	t *testing.T,
	gameServer *Server,
	dropID world.EntityID,
	ownerPlayerID foundation.PlayerID,
	itemID foundation.ItemID,
	quantity int64,
) {
	t.Helper()
	storedDrop, ok := gameServer.runtime.Loot.Drop(dropID)
	if !ok {
		t.Fatalf("loot drop %q missing from loot service", dropID)
	}
	if storedDrop.SourceType != loot.DropSourcePlayerDeath ||
		storedDrop.OwnerPlayerID != ownerPlayerID ||
		storedDrop.ZoneID != seededPVPMapID.ZoneID() ||
		storedDrop.ItemDefinition.ItemID != itemID ||
		storedDrop.Quantity != quantity {
		t.Fatalf("stored player-death drop = %+v, want owner %q %s x%d in 1-3", storedDrop, ownerPlayerID, itemID, quantity)
	}
}
