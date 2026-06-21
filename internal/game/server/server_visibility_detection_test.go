package server

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

func TestRuntimeVisibilityCandidatesUseContentDrivenSignatures(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	viewer := createResolvedRuntimeSession(t, gameServer, "signature-viewer@example.com", "Signature Viewer")
	other := createResolvedRuntimeSession(t, gameServer, "signature-other@example.com", "Signature Other")
	insertTestWorldEntity(t, gameServer, "entity_signature_npc", world.EntityTypeNPC, world.Vec2{X: 10, Y: 0}, false)
	insertTestWorldEntity(t, gameServer, "entity_signature_loot", world.EntityTypeLoot, world.Vec2{X: 12, Y: 0}, false)
	insertTestWorldEntity(t, gameServer, "entity_signature_signal", world.EntityTypePlanetSignal, world.Vec2{X: 14, Y: 0}, false)

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	instance, _, err := gameServer.runtime.activeMapInstanceLocked(viewer.PlayerID)
	if err != nil {
		t.Fatalf("activeMapInstanceLocked() error = %v, want nil", err)
	}
	if _, err := gameServer.runtime.syncPlayerCombatActorLocked(viewer.PlayerID); err != nil {
		t.Fatalf("syncPlayerCombatActorLocked() error = %v, want nil", err)
	}
	viewerActor, ok := gameServer.runtime.Combat.Actor(testPlayerEntityIDLocked(t, gameServer, viewer.PlayerID))
	if !ok {
		t.Fatal("viewer combat actor missing")
	}
	if viewerActor.Signature == visibility.EntitySignature(1) || viewerActor.Signature != gameServer.runtime.playerSignatureLocked(viewer.PlayerID) {
		t.Fatalf("player actor signature = %v, want ship/content signature", viewerActor.Signature)
	}

	npc, ok := instance.Worker.Entity("entity_signature_npc")
	if !ok {
		t.Fatal("npc entity missing")
	}
	if err := gameServer.runtime.syncWorldCombatActorLocked(viewer.PlayerID, npc.ID); err != nil {
		t.Fatalf("syncWorldCombatActorLocked(npc) error = %v, want nil", err)
	}
	npcActor, ok := gameServer.runtime.Combat.Actor(npc.ID)
	if !ok {
		t.Fatal("npc combat actor missing")
	}
	if npcActor.Signature != visibility.SignatureForEntityType(world.EntityTypeNPC) || npcActor.Signature == visibility.EntitySignature(1) {
		t.Fatalf("npc actor signature = %v, want content-driven NPC signature", npcActor.Signature)
	}

	otherEntity, ok := instance.Worker.PlayerEntity(other.PlayerID)
	if !ok {
		t.Fatal("other player entity missing")
	}
	if err := gameServer.runtime.syncWorldCombatActorLocked(viewer.PlayerID, otherEntity.ID); err != nil {
		t.Fatalf("syncWorldCombatActorLocked(player target) error = %v, want nil", err)
	}
	otherActor, ok := gameServer.runtime.Combat.Actor(otherEntity.ID)
	if !ok {
		t.Fatal("other player combat actor missing")
	}
	if otherActor.Signature == visibility.EntitySignature(1) || otherActor.Signature != gameServer.runtime.playerSignatureLocked(other.PlayerID) {
		t.Fatalf("target player actor signature = %v, want ship/content signature", otherActor.Signature)
	}

	for _, test := range []struct {
		entityID world.EntityID
		want     visibility.EntitySignature
	}{
		{entityID: "entity_signature_loot", want: visibility.SignatureForEntityType(world.EntityTypeLoot)},
		{entityID: "entity_signature_signal", want: visibility.SignatureForEntityType(world.EntityTypePlanetSignal)},
	} {
		entity, ok := instance.Worker.Entity(test.entityID)
		if !ok {
			t.Fatalf("%s entity missing", test.entityID)
		}
		signature, _, _ := gameServer.runtime.visibilityInputsForEntityLocked(entity, "", false)
		if signature != test.want || signature == visibility.EntitySignature(1) {
			t.Fatalf("%s visibility signature = %v, want %v", test.entityID, signature, test.want)
		}
	}
}

func TestWorldSnapshotHiddenDetectionUsesServerStatsAndSafePayload(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	viewer := createResolvedRuntimeSession(t, gameServer, "detection-viewer@example.com", "Detection Viewer")
	target := createResolvedRuntimeSession(t, gameServer, "detection-target@example.com", "Detection Target")
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: 10, Y: 0})
	setTestHiddenPlayer(gameServer, target.PlayerID, true)

	initialEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("bootstrapEvents() error = %v, want nil", err)
	}
	initialSnapshot := decodeWorldSnapshotForTest(t, initialEvents)
	if hasEntityID(initialSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw hidden target without server detection stats: %+v", initialSnapshot.Entities)
	}

	forged := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(viewer.SessionID.String()),
		[]byte(`{"request_id":"request-forged-detection","op":"world.snapshot","payload":{"detection_power":999,"stealth_score":0},"client_seq":1,"v":1}`),
	)
	if !forged.HasError || forged.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("forged detection response = %+v, want invalid payload", forged)
	}

	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[viewer.PlayerID]
	state.Stats.DetectionPower = 40
	gameServer.runtime.players[viewer.PlayerID] = state
	gameServer.runtime.mu.Unlock()

	detectedEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("detected bootstrapEvents() error = %v, want nil", err)
	}
	detectedSnapshot := decodeWorldSnapshotForTest(t, detectedEvents)
	if !hasEntityID(detectedSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer did not see hidden target with server detection stats: %+v", detectedSnapshot.Entities)
	}
	rawSnapshot := string(mustJSON(t, detectedSnapshot))
	for _, forbidden := range []string{
		"signature",
		"stealth_score",
		"jammer_strength",
		"detection_score",
		"detection_threshold",
		"target_player_id",
		"witness_expires_at",
		target.PlayerID.String(),
		viewer.PlayerID.String(),
	} {
		if strings.Contains(rawSnapshot, forbidden) {
			t.Fatalf("detected snapshot leaked %q in %s", forbidden, rawSnapshot)
		}
	}
}

func TestScanWitnessAllowsCombatAgainstHiddenPlayerWhileActive(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	attacker := createResolvedRuntimeSession(t, gameServer, "witness-combat-attacker@example.com", "Witness Attacker")
	target := createResolvedRuntimeSession(t, gameServer, "witness-combat-target@example.com", "Witness Target")
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	setTestActiveMapPVPPolicy(t, gameServer, attacker.PlayerID, "pvp")
	moveTestPlayerEntity(gameServer, attacker.PlayerID, world.Vec2{X: 500, Y: 500})
	moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: 520, Y: 500})
	setTestHiddenPlayer(gameServer, target.PlayerID, true)

	beforeScan := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(attacker.SessionID.String()),
		[]byte(`{"request_id":"request-witness-combat-before","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"`+targetEntityID.String()+`"},"client_seq":1,"v":1}`),
	)
	if !beforeScan.HasError || beforeScan.Error.Error.Code != foundation.CodeNotVisible {
		t.Fatalf("combat before scan response = %+v, want %s", beforeScan, foundation.CodeNotVisible)
	}

	scanResponse := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(attacker.SessionID.String()),
		[]byte(`{"request_id":"request-witness-combat-scan","op":"scan.pulse","payload":{},"client_seq":2,"v":1}`),
	)
	if scanResponse.HasError {
		t.Fatalf("scan response error = %+v, want success", scanResponse.Error)
	}
	var scanPayload struct {
		Scan scanPulsePayload `json:"scan"`
	}
	if err := json.Unmarshal(scanResponse.Response.Payload, &scanPayload); err != nil {
		t.Fatalf("decode scan payload: %v", err)
	}
	if scanPayload.Scan.Status != string(discovery.ScanPulseStatusPlayerRevealed) {
		t.Fatalf("scan status = %q, want %q", scanPayload.Scan.Status, discovery.ScanPulseStatusPlayerRevealed)
	}
	gameServer.runtime.mu.Lock()
	instance, _, instanceErr := gameServer.runtime.activeMapInstanceLocked(attacker.PlayerID)
	witnessed := instanceErr == nil && gameServer.runtime.hiddenPlayerWitnessActiveLocked(instance, attacker.PlayerID, target.PlayerID, gameServer.runtime.clock.Now())
	gameServer.runtime.mu.Unlock()
	if instanceErr != nil {
		t.Fatalf("activeMapInstanceLocked() error = %v, want nil", instanceErr)
	}
	if !witnessed {
		t.Fatal("hidden target witness inactive after scan, want active witness")
	}

	afterScan := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(attacker.SessionID.String()),
		[]byte(`{"request_id":"request-witness-combat-after","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"`+targetEntityID.String()+`"},"client_seq":3,"v":1}`),
	)
	if afterScan.HasError {
		t.Fatalf("combat after scan response error = %+v, want success", afterScan.Error)
	}
	gameServer.runtime.mu.Lock()
	targetState := gameServer.runtime.players[target.PlayerID]
	gameServer.runtime.mu.Unlock()
	if targetState.Ship.Shield >= 100 && targetState.Ship.Hull >= 100 {
		t.Fatalf("target ship after witnessed combat = %+v, want damage", targetState.Ship)
	}
}

func testPlayerEntityIDLocked(t *testing.T, gameServer *Server, playerID foundation.PlayerID) world.EntityID {
	t.Helper()
	state, ok := gameServer.runtime.players[playerID]
	if !ok {
		t.Fatalf("player %q missing runtime state", playerID)
	}
	return state.EntityID
}
