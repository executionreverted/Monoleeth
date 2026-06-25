package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
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

type repairShipResponseForTest struct {
	Accepted     bool                              `json:"accepted"`
	Repaired     bool                              `json:"repaired"`
	Duplicate    bool                              `json:"duplicate"`
	PublicMapKey string                            `json:"public_map_key"`
	Position     world.Vec2                        `json:"position"`
	Protection   worldmaps.ClientProtectionSummary `json:"protection"`
	Ship         shipSnapshotPayload               `json:"ship"`
	Wallet       walletSnapshotPayload             `json:"wallet"`
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

func TestSeededPVPMapDeathRepairIdempotencyIsPlayerScopedForSharedShipAndRequestID(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	attackerOne := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-shared-repair-attacker-1@example.com", "Shared Repair Attacker 1", seededPVPMapID, "west_gate")
	targetOne := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-shared-repair-target-1@example.com", "Shared Repair Target 1", seededPVPMapID, "west_gate")
	attackerTwo := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-shared-repair-attacker-2@example.com", "Shared Repair Attacker 2", seededPVPMapID, "west_gate")
	targetTwo := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-shared-repair-target-2@example.com", "Shared Repair Target 2", seededPVPMapID, "west_gate")
	moveTestPlayerEntity(gameServer, attackerOne.PlayerID, world.Vec2{X: 500, Y: 500})
	moveTestPlayerEntity(gameServer, targetOne.PlayerID, world.Vec2{X: 520, Y: 500})
	moveTestPlayerEntity(gameServer, attackerTwo.PlayerID, world.Vec2{X: 700, Y: 500})
	moveTestPlayerEntity(gameServer, targetTwo.PlayerID, world.Vec2{X: 720, Y: 500})
	setTestPlayerShipCombatValues(t, gameServer, targetOne.PlayerID, 1, 0, 100)
	setTestPlayerShipCombatValues(t, gameServer, targetTwo.PlayerID, 1, 0, 100)
	addTestCargoStack(t, gameServer, targetOne.PlayerID, "raw_ore", 3, "shared-repair-target-one-raw-ore")
	addTestCargoStack(t, gameServer, targetTwo.PlayerID, "raw_ore", 4, "shared-repair-target-two-raw-ore")

	targetOneEntityID := testPlayerEntityID(t, gameServer, targetOne.PlayerID)
	targetTwoEntityID := testPlayerEntityID(t, gameServer, targetTwo.PlayerID)
	gameServer.runtime.mu.Lock()
	targetOneShipID := gameServer.runtime.players[targetOne.PlayerID].Ship.ActiveShipID
	targetTwoShipID := gameServer.runtime.players[targetTwo.PlayerID].Ship.ActiveShipID
	gameServer.runtime.mu.Unlock()
	if targetOneShipID != starterShipID.String() || targetTwoShipID != starterShipID.String() || targetOneShipID != targetTwoShipID {
		t.Fatalf("shared repair active ships = %q/%q, want same starter ship", targetOneShipID, targetTwoShipID)
	}

	_ = performLethalPVPAttackForTest(t, gameServer, attackerOne, targetOne, foundation.RequestID("request-pvp-shared-repair-death-one"))
	_ = performLethalPVPAttackForTest(t, gameServer, attackerTwo, targetTwo, foundation.RequestID("request-pvp-shared-repair-death-two"))

	westGate, ok := gameServer.runtime.mapCatalog.Spawn(seededPVPMapID, "west_gate")
	if !ok {
		t.Fatalf("seeded pvp west_gate spawn missing")
	}
	gameServer.runtime.mu.Lock()
	pendingOne, pendingOneOK := gameServer.runtime.pendingRespawns[targetOne.PlayerID]
	pendingTwo, pendingTwoOK := gameServer.runtime.pendingRespawns[targetTwo.PlayerID]
	gameServer.runtime.drainQueuedEventsLocked(attackerOne.SessionID)
	gameServer.runtime.drainQueuedEventsLocked(targetOne.SessionID)
	gameServer.runtime.drainQueuedEventsLocked(attackerTwo.SessionID)
	gameServer.runtime.drainQueuedEventsLocked(targetTwo.SessionID)
	gameServer.runtime.mu.Unlock()
	if !pendingOneOK || pendingOne.MapID != seededPVPMapID || pendingOne.SpawnID != "west_gate" {
		t.Fatalf("first pending respawn = %+v ok=%v, want server-selected map_1_3 west_gate", pendingOne, pendingOneOK)
	}
	if !pendingTwoOK || pendingTwo.MapID != seededPVPMapID || pendingTwo.SpawnID != "west_gate" {
		t.Fatalf("second pending respawn = %+v ok=%v, want server-selected map_1_3 west_gate", pendingTwo, pendingTwoOK)
	}

	sharedRepairRequestID := foundation.RequestID("request-pvp-shared-repair-ship")
	rawFirstRepair, firstRepair := repairShipForTest(t, gameServer, targetOne, sharedRepairRequestID)
	rawSecondRepair, secondRepair := repairShipForTest(t, gameServer, targetTwo, sharedRepairRequestID)
	assertRepairResponseAtCheckpointForTest(t, "first", firstRepair, westGate.Position)
	assertRepairResponseAtCheckpointForTest(t, "second", secondRepair, westGate.Position)
	assertRepairPayloadClientSafeForTest(t, rawFirstRepair, attackerOne, targetOne, attackerTwo, targetTwo)
	assertRepairPayloadClientSafeForTest(t, rawSecondRepair, attackerOne, targetOne, attackerTwo, targetTwo)

	gameServer.runtime.mu.Lock()
	firstRepairEvents := gameServer.runtime.drainQueuedEventsLocked(targetOne.SessionID)
	secondRepairEvents := gameServer.runtime.drainQueuedEventsLocked(targetTwo.SessionID)
	firstState := gameServer.runtime.players[targetOne.PlayerID]
	secondState := gameServer.runtime.players[targetTwo.PlayerID]
	instance, instanceErr := gameServer.runtime.mapInstanceLocked(seededPVPMapID)
	var firstEntity, secondEntity world.Entity
	var firstEntityOK, secondEntityOK bool
	if instanceErr == nil {
		firstEntity, firstEntityOK = instance.Worker.PlayerEntity(targetOne.PlayerID)
		secondEntity, secondEntityOK = instance.Worker.PlayerEntity(targetTwo.PlayerID)
	}
	firstActor, firstActorOK := gameServer.runtime.Combat.Actor(targetOneEntityID)
	secondActor, secondActorOK := gameServer.runtime.Combat.Actor(targetTwoEntityID)
	firstProtection, firstProtectionOK := gameServer.runtime.activeProtectionLocked(targetOne.PlayerID)
	secondProtection, secondProtectionOK := gameServer.runtime.activeProtectionLocked(targetTwo.PlayerID)
	_, firstPendingStillPresent := gameServer.runtime.pendingRespawns[targetOne.PlayerID]
	_, secondPendingStillPresent := gameServer.runtime.pendingRespawns[targetTwo.PlayerID]
	repairAttemptCount := len(gameServer.runtime.repairAttempts)
	gameServer.runtime.mu.Unlock()
	if instanceErr != nil {
		t.Fatalf("pvp map instance: %v", instanceErr)
	}
	for _, item := range []struct {
		label        string
		state        playerRuntimeState
		entity       world.Entity
		entityOK     bool
		actor        combat.ActorState
		actorOK      bool
		protection   playerProtectionState
		protectionOK bool
		pending      bool
	}{
		{
			label:        "first",
			state:        firstState,
			entity:       firstEntity,
			entityOK:     firstEntityOK,
			actor:        firstActor,
			actorOK:      firstActorOK,
			protection:   firstProtection,
			protectionOK: firstProtectionOK,
			pending:      firstPendingStillPresent,
		},
		{
			label:        "second",
			state:        secondState,
			entity:       secondEntity,
			entityOK:     secondEntityOK,
			actor:        secondActor,
			actorOK:      secondActorOK,
			protection:   secondProtection,
			protectionOK: secondProtectionOK,
			pending:      secondPendingStillPresent,
		},
	} {
		assertRepairRuntimeStateAtCheckpointForTest(t, item.label, item.state, item.entity, item.entityOK, item.actor, item.actorOK, item.protection, item.protectionOK, item.pending, westGate.Position)
	}
	if repairAttemptCount != 2 {
		t.Fatalf("repair attempt count = %d, want two player-scoped records", repairAttemptCount)
	}
	requireEventTypeForTest(t, firstRepairEvents, realtime.EventDeathRepaired)
	requireEventTypeForTest(t, firstRepairEvents, realtime.EventPlayerProtection)
	requireEventTypeForTest(t, firstRepairEvents, realtime.EventPositionCorrected)
	requireEventTypeForTest(t, secondRepairEvents, realtime.EventDeathRepaired)
	requireEventTypeForTest(t, secondRepairEvents, realtime.EventPlayerProtection)
	requireEventTypeForTest(t, secondRepairEvents, realtime.EventPositionCorrected)
	assertRepairRespawnEventsClientSafeForTest(t, firstRepairEvents, targetOne.PlayerID, targetOne.SessionID)
	assertRepairRespawnEventsClientSafeForTest(t, secondRepairEvents, targetTwo.PlayerID, targetTwo.SessionID)
}

func TestSeededPVPMapDeathRepairRespawnsAtCheckpointWithProtectionAndDuplicateRepairIsIdempotent(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	attacker := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-repair-respawn-attacker@example.com", "Repair Respawn Attacker", seededPVPMapID, "west_gate")
	target := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-repair-respawn-target@example.com", "Repair Respawn Target", seededPVPMapID, "west_gate")
	moveTestPlayerEntity(gameServer, attacker.PlayerID, world.Vec2{X: 500, Y: 500})
	moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: 520, Y: 500})
	startTestPlayerMovement(t, gameServer, target.PlayerID, world.Vec2{X: 900, Y: 500})
	setTestPlayerShipCombatValues(t, gameServer, target.PlayerID, 1, 0, 100)
	addTestCargoStack(t, gameServer, target.PlayerID, "raw_ore", 3, "repair-respawn-raw-ore")

	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	_ = performLethalPVPAttackForTest(t, gameServer, attacker, target, foundation.RequestID("request-pvp-death-repair-respawn"))

	westGate, ok := gameServer.runtime.mapCatalog.Spawn(seededPVPMapID, "west_gate")
	if !ok {
		t.Fatalf("seeded pvp west_gate spawn missing")
	}
	gameServer.runtime.mu.Lock()
	pending, pendingOK := gameServer.runtime.pendingRespawns[target.PlayerID]
	attackerDeathEvents := gameServer.runtime.drainQueuedEventsLocked(attacker.SessionID)
	targetDeathEvents := gameServer.runtime.drainQueuedEventsLocked(target.SessionID)
	gameServer.runtime.mu.Unlock()
	if !pendingOK || pending.MapID != seededPVPMapID || pending.SpawnID != "west_gate" {
		t.Fatalf("pending respawn = %+v ok=%v, want map_1_3 west_gate stored server-side", pending, pendingOK)
	}
	assertNoDeathInternalsInQueuedEventsForTest(t, attackerDeathEvents)
	assertNoDeathInternalsInQueuedEventsForTest(t, targetDeathEvents)

	quote := issueRepairQuoteForTest(t, gameServer, target.PlayerID)
	repairRequest := realtime.RequestEnvelope{
		RequestID: foundation.RequestID("request-pvp-death-repair-ship"),
		Op:        realtime.OperationDeathRepairShip,
		Payload:   repairShipPayloadForTest(t, quote),
		Version:   realtime.CurrentVersion,
	}
	repairContext := realtime.CommandContext{
		SessionID: realtime.SessionID(target.SessionID.String()),
		PlayerID:  target.PlayerID,
	}
	rawRepair, err := gameServer.runtime.handleDeathRepairShip(repairContext, repairRequest)
	if err != nil {
		t.Fatalf("repair after pvp death error = %v, want nil", err)
	}
	var repairPayload struct {
		Accepted     bool                              `json:"accepted"`
		Repaired     bool                              `json:"repaired"`
		Duplicate    bool                              `json:"duplicate"`
		PublicMapKey string                            `json:"public_map_key"`
		Position     world.Vec2                        `json:"position"`
		Protection   worldmaps.ClientProtectionSummary `json:"protection"`
		Ship         shipSnapshotPayload               `json:"ship"`
	}
	if err := json.Unmarshal(rawRepair, &repairPayload); err != nil {
		t.Fatalf("decode repair payload: %v", err)
	}
	if !repairPayload.Accepted ||
		!repairPayload.Repaired ||
		repairPayload.Duplicate ||
		repairPayload.PublicMapKey != "1-3" ||
		repairPayload.Position != westGate.Position ||
		repairPayload.Protection.Reason != protectionReasonRespawn ||
		repairPayload.Ship.Disabled ||
		repairPayload.Ship.RepairState != "ready" {
		t.Fatalf("repair payload = %+v, want repaired ship at public 1-3 spawn with respawn protection", repairPayload)
	}

	gameServer.runtime.mu.Lock()
	repairEvents := gameServer.runtime.drainQueuedEventsLocked(target.SessionID)
	targetState := gameServer.runtime.players[target.PlayerID]
	instance, instanceErr := gameServer.runtime.mapInstanceLocked(seededPVPMapID)
	targetEntity, entityOK := instance.Worker.PlayerEntity(target.PlayerID)
	targetActor, actorOK := gameServer.runtime.Combat.Actor(targetEntityID)
	protection, protectionOK := gameServer.runtime.activeProtectionLocked(target.PlayerID)
	_, pendingStillPresent := gameServer.runtime.pendingRespawns[target.PlayerID]
	gameServer.runtime.mu.Unlock()
	if instanceErr != nil {
		t.Fatalf("pvp map instance: %v", instanceErr)
	}
	if !entityOK || targetEntity.Position != westGate.Position || targetEntity.Movement.Moving {
		t.Fatalf("repaired target entity = %+v ok=%v, want snapped stopped at west_gate", targetEntity, entityOK)
	}
	if targetState.Ship.Disabled ||
		targetState.Ship.RepairState != "ready" ||
		targetState.Ship.Hull != targetState.Ship.MaxHull ||
		targetState.Ship.Shield != targetState.Ship.MaxShield ||
		targetState.Ship.Capacitor != targetState.Ship.MaxCapacitor {
		t.Fatalf("repaired target ship = %+v, want ready and restored", targetState.Ship)
	}
	if !actorOK ||
		targetActor.Dead ||
		targetActor.DiedAt != nil ||
		targetActor.Position != westGate.Position ||
		targetActor.HP != float64(targetState.Ship.MaxHull) ||
		targetActor.Shield != float64(targetState.Ship.MaxShield) ||
		targetActor.Energy != float64(targetState.Ship.MaxCapacitor) ||
		len(targetActor.Cooldowns) != 0 ||
		len(targetActor.Contributions) != 0 {
		t.Fatalf("repaired target actor = %+v ok=%v, want restored live actor at west_gate with cleared combat state", targetActor, actorOK)
	}
	if !protectionOK ||
		protection.Reason != protectionReasonRespawn ||
		protection.InternalMapID != seededPVPMapID ||
		protection.PublicMapKey != "1-3" ||
		!protection.BlocksPVP ||
		!protection.BreakOnPVPAction {
		t.Fatalf("respawn protection = %+v ok=%v, want active pvp-blocking respawn protection on public 1-3", protection, protectionOK)
	}
	if pendingStillPresent {
		t.Fatalf("pending respawn for %q still present after successful repair", target.PlayerID)
	}
	protectionEvent := requireEventTypeForTest(t, repairEvents, realtime.EventPlayerProtection)
	positionEvent := requireEventTypeForTest(t, repairEvents, realtime.EventPositionCorrected)
	requireEventTypeForTest(t, repairEvents, realtime.EventDeathRepaired)
	assertRepairRespawnEventsClientSafeForTest(t, repairEvents, target.PlayerID, target.SessionID)

	var protectionPayload playerProtectionUpdatedPayload
	if err := json.Unmarshal(protectionEvent.Payload, &protectionPayload); err != nil {
		t.Fatalf("decode protection event: %v", err)
	}
	if protectionPayload.Reason != protectionReasonRespawn ||
		protectionPayload.PublicMapKey != "1-3" ||
		!protectionPayload.BlocksPVP ||
		!protectionPayload.BreakOnPVPAction {
		t.Fatalf("protection event = %+v, want public respawn protection", protectionPayload)
	}
	var positionPayload struct {
		EntityID string     `json:"entity_id"`
		Position world.Vec2 `json:"position"`
		Movement any        `json:"movement,omitempty"`
		MapEpoch uint64     `json:"map_subscription_epoch"`
	}
	if err := json.Unmarshal(positionEvent.Payload, &positionPayload); err != nil {
		t.Fatalf("decode position correction event: %v", err)
	}
	if positionPayload.EntityID != targetEntityID.String() ||
		positionPayload.Position != westGate.Position ||
		positionPayload.Movement != nil ||
		positionPayload.MapEpoch == 0 {
		t.Fatalf("position correction = %+v, want stopped public position at west_gate", positionPayload)
	}

	movedAfterRepair := world.Vec2{X: 740, Y: 5000}
	moveTestPlayerEntity(gameServer, target.PlayerID, movedAfterRepair)
	rawDuplicate, err := gameServer.runtime.handleDeathRepairShip(repairContext, repairRequest)
	if err != nil {
		t.Fatalf("duplicate repair error = %v, want nil duplicate response", err)
	}
	var duplicatePayload struct {
		Accepted  bool       `json:"accepted"`
		Duplicate bool       `json:"duplicate"`
		Position  world.Vec2 `json:"position"`
	}
	if err := json.Unmarshal(rawDuplicate, &duplicatePayload); err != nil {
		t.Fatalf("decode duplicate repair payload: %v", err)
	}
	gameServer.runtime.mu.Lock()
	duplicateEvents := gameServer.runtime.drainQueuedEventsLocked(target.SessionID)
	afterDuplicate, duplicateEntityOK := instance.Worker.PlayerEntity(target.PlayerID)
	afterDuplicateProtection, afterDuplicateProtectionOK := gameServer.runtime.activeProtectionLocked(target.PlayerID)
	gameServer.runtime.mu.Unlock()
	if !duplicatePayload.Accepted || !duplicatePayload.Duplicate || duplicatePayload.Position != westGate.Position {
		t.Fatalf("duplicate repair payload = %+v, want stored duplicate response", duplicatePayload)
	}
	if len(duplicateEvents) != 0 {
		t.Fatalf("duplicate repair queued events = %+v, want none", duplicateEvents)
	}
	if !duplicateEntityOK || afterDuplicate.Position != movedAfterRepair {
		t.Fatalf("duplicate repair entity = %+v ok=%v, want no second respawn move from %+v", afterDuplicate, duplicateEntityOK, movedAfterRepair)
	}
	if !afterDuplicateProtectionOK || !afterDuplicateProtection.ExpiresAt.Equal(protection.ExpiresAt) {
		t.Fatalf("duplicate repair protection = %+v ok=%v, want unchanged protection expiry %+v", afterDuplicateProtection, afterDuplicateProtectionOK, protection.ExpiresAt)
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
	equipStarterLaserForTest(t, gameServer, attacker.PlayerID)
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

func repairShipForTest(
	t *testing.T,
	gameServer *Server,
	resolved auth.ResolvedSession,
	requestID foundation.RequestID,
) ([]byte, repairShipResponseForTest) {
	t.Helper()
	quote := issueRepairQuoteForTest(t, gameServer, resolved.PlayerID)
	request := realtime.RequestEnvelope{
		RequestID: requestID,
		Op:        realtime.OperationDeathRepairShip,
		Payload:   repairShipPayloadForTest(t, quote),
		Version:   realtime.CurrentVersion,
	}
	context := realtime.CommandContext{
		SessionID: realtime.SessionID(resolved.SessionID.String()),
		PlayerID:  resolved.PlayerID,
	}
	raw, err := gameServer.runtime.handleDeathRepairShip(context, request)
	if err != nil {
		t.Fatalf("repair ship for %q error = %v, want nil", resolved.PlayerID, err)
	}
	var payload repairShipResponseForTest
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode repair payload for %q: %v", resolved.PlayerID, err)
	}
	return raw, payload
}

func assertRepairResponseAtCheckpointForTest(t *testing.T, label string, payload repairShipResponseForTest, position world.Vec2) {
	t.Helper()
	if !payload.Accepted ||
		!payload.Repaired ||
		payload.Duplicate ||
		payload.PublicMapKey != "1-3" ||
		payload.Position != position ||
		payload.Protection.Reason != protectionReasonRespawn ||
		!payload.Protection.BlocksPVP ||
		!payload.Protection.BreakOnPVPAction ||
		payload.Ship.Disabled ||
		payload.Ship.RepairState != "ready" {
		t.Fatalf("%s repair payload = %+v, want independent repair at public 1-3 checkpoint with respawn protection", label, payload)
	}
}

func assertRepairRuntimeStateAtCheckpointForTest(
	t *testing.T,
	label string,
	state playerRuntimeState,
	entity world.Entity,
	entityOK bool,
	actor combat.ActorState,
	actorOK bool,
	protection playerProtectionState,
	protectionOK bool,
	pendingStillPresent bool,
	position world.Vec2,
) {
	t.Helper()
	if state.Ship.Disabled ||
		state.Ship.RepairState != "ready" ||
		state.Ship.Hull != state.Ship.MaxHull ||
		state.Ship.Shield != state.Ship.MaxShield ||
		state.Ship.Capacitor != state.Ship.MaxCapacitor {
		t.Fatalf("%s repaired ship = %+v, want ready and restored", label, state.Ship)
	}
	if !entityOK || entity.Position != position || entity.Movement.Moving {
		t.Fatalf("%s repaired entity = %+v ok=%v, want snapped stopped at checkpoint", label, entity, entityOK)
	}
	if !actorOK ||
		actor.Dead ||
		actor.DiedAt != nil ||
		actor.Position != position ||
		actor.HP != float64(state.Ship.MaxHull) ||
		actor.Shield != float64(state.Ship.MaxShield) ||
		actor.Energy != float64(state.Ship.MaxCapacitor) {
		t.Fatalf("%s repaired actor = %+v ok=%v, want live actor at checkpoint", label, actor, actorOK)
	}
	if !protectionOK ||
		protection.Reason != protectionReasonRespawn ||
		protection.InternalMapID != seededPVPMapID ||
		protection.PublicMapKey != "1-3" ||
		!protection.BlocksPVP ||
		!protection.BreakOnPVPAction {
		t.Fatalf("%s respawn protection = %+v ok=%v, want active pvp-blocking respawn protection", label, protection, protectionOK)
	}
	if pendingStillPresent {
		t.Fatalf("%s pending respawn still present after successful repair", label)
	}
}

func startTestPlayerMovement(t *testing.T, gameServer *Server, playerID foundation.PlayerID, target world.Vec2) {
	t.Helper()
	intent, err := world.NewMovementIntent(target)
	if err != nil {
		t.Fatalf("movement intent: %v", err)
	}
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	instance, _, err := gameServer.runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		t.Fatalf("active map instance: %v", err)
	}
	if err := instance.Worker.Submit(worker.MoveToCommand{PlayerID: playerID, Intent: intent}); err != nil {
		t.Fatalf("submit move: %v", err)
	}
	result := instance.Worker.Tick()
	gameServer.runtime.recordEnemyTelemetryLocked(instance, result)
	if err := commandErrors(result); err != nil {
		t.Fatalf("tick move: %v", err)
	}
}

func assertRepairPayloadClientSafeForTest(t *testing.T, raw []byte, sessions ...auth.ResolvedSession) {
	t.Helper()
	payload := string(raw)
	for _, session := range sessions {
		for _, forbidden := range []string{session.PlayerID.String(), session.SessionID.String()} {
			if strings.Contains(payload, forbidden) {
				t.Fatalf("repair payload leaked %q in %s", forbidden, payload)
			}
		}
	}
	for _, forbidden := range []string{
		"player_id",
		"session_id",
		"death_id",
		"lethal_event_key",
		"respawn_location_id",
		"map_1_3",
		"west_gate",
	} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("repair payload leaked %q in %s", forbidden, payload)
		}
	}
}

func assertRepairRespawnEventsClientSafeForTest(t *testing.T, events []realtime.EventEnvelope, playerID foundation.PlayerID, sessionID auth.SessionID) {
	t.Helper()
	for _, event := range events {
		rawEvent := string(mustJSON(t, event))
		for _, forbidden := range []string{
			playerID.String(),
			sessionID.String(),
			"player_id",
			"session_id",
			"death_id",
			"lethal_event_key",
			"respawn_location_id",
			"map_1_3",
			"west_gate",
		} {
			if strings.Contains(rawEvent, forbidden) {
				t.Fatalf("repair respawn event %s leaked %q in %s", event.Type, forbidden, rawEvent)
			}
		}
	}
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
