package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestPortalEnterRejectsTrustedInternalFieldsWithoutMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "portal-internals@example.com", "Portal Internals")
	moveTestPlayerEntity(gameServer, resolved.PlayerID, world.Vec2{X: 9800, Y: 5000})

	for _, field := range []string{
		"map_id",
		"internal_map_id",
		"zone_id",
		"worker_id",
		"map_worker_id",
		"transfer_id",
		"transfer_token",
		"destination_worker",
		"origin_worker",
		"destination_map_id",
		"destination_spawn_id",
	} {
		response := gameServer.runtime.Gateway.HandleRequest(
			realtime.SessionID(resolved.SessionID.String()),
			[]byte(fmt.Sprintf(
				`{"request_id":"request-portal-internal-%s","op":"portal.enter","payload":{"portal_id":"east_gate",%q:"client-authored"},"client_seq":1,"v":1}`,
				strings.ReplaceAll(field, "_", "-"),
				field,
			)),
		)
		if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
			t.Fatalf("portal.enter with %s response = %+v, want invalid payload", field, response)
		}
		assertPlayerOnlyInMapForTest(t, gameServer, resolved.PlayerID, worldmaps.StarterMapID)
		if len(gameServer.runtime.portalCooldowns) != 0 {
			t.Fatalf("portal cooldowns = %+v, want none after trusted-field rejection", gameServer.runtime.portalCooldowns)
		}
	}
}

func TestPortalEnterOutOfRangeAndCooldownAreNonMutating(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "portal-range@example.com", "Portal Range")

	outOfRange := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-portal-range","op":"portal.enter","payload":{"portal_id":"east_gate"},"client_seq":1,"v":1}`),
	)
	if !outOfRange.HasError || outOfRange.Error.Error.Code != foundation.CodeOutOfRange {
		t.Fatalf("out-of-range portal response = %+v, want out of range", outOfRange)
	}
	assertPlayerOnlyInMapForTest(t, gameServer, resolved.PlayerID, worldmaps.StarterMapID)
	if len(gameServer.runtime.portalCooldowns) != 0 {
		t.Fatalf("portal cooldowns = %+v, want none after failed range validation", gameServer.runtime.portalCooldowns)
	}

	cooldownPlayer := createResolvedRuntimeSession(t, gameServer, "portal-cooldown@example.com", "Portal Cooldown")
	moveTestPlayerEntity(gameServer, cooldownPlayer.PlayerID, world.Vec2{X: 9800, Y: 5000})
	gameServer.runtime.mu.Lock()
	gameServer.runtime.portalCooldowns[portalCooldownKey{
		PlayerID:    cooldownPlayer.PlayerID,
		SourceMapID: worldmaps.StarterMapID,
		PortalID:    "east_gate",
	}] = gameServer.runtime.clock.Now().Add(runtimePortalCooldown)
	gameServer.runtime.mu.Unlock()

	cooldown := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(cooldownPlayer.SessionID.String()),
		[]byte(`{"request_id":"request-portal-cooldown","op":"portal.enter","payload":{"portal_id":"east_gate"},"client_seq":1,"v":1}`),
	)
	if !cooldown.HasError || cooldown.Error.Error.Code != foundation.CodeCooldown {
		t.Fatalf("cooldown portal response = %+v, want cooldown", cooldown)
	}
	assertPlayerOnlyInMapForTest(t, gameServer, cooldownPlayer.PlayerID, worldmaps.StarterMapID)
}

func TestPortalEnterTransfersPlayerAndAllActiveSessions(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "portal-transfer@example.com", "Portal Transfer")
	secondLogin, err := gameServer.runtime.Auth.Login(context.Background(), auth.LoginInput{
		Email:    resolved.Email.String(),
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("second login error = %v, want nil", err)
	}
	if err := gameServer.runtime.ensurePlayerSession(secondLogin.Session); err != nil {
		t.Fatalf("ensure second session: %v", err)
	}
	moveTestPlayerEntity(gameServer, resolved.PlayerID, world.Vec2{X: 9800, Y: 5000})

	gameServer.runtime.mu.Lock()
	oldEpoch := gameServer.runtime.sessionMapEpochLocked(resolved.SessionID)
	gameServer.runtime.mu.Unlock()
	requestedAt := gameServer.runtime.clock.Now()

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-portal-transfer","op":"portal.enter","payload":{"portal_id":"east_gate"},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("portal transfer response error = %+v, want success", response.Error)
	}
	var responsePayload struct {
		Accepted             bool                 `json:"accepted"`
		ToPublicMapKey       string               `json:"to_public_map_key"`
		MapSubscriptionEpoch uint64               `json:"map_subscription_epoch"`
		Snapshot             worldSnapshotPayload `json:"snapshot"`
	}
	if err := json.Unmarshal(response.Response.Payload, &responsePayload); err != nil {
		t.Fatalf("decode portal response: %v", err)
	}
	if !responsePayload.Accepted || responsePayload.ToPublicMapKey != "1-2" || responsePayload.Snapshot.Map.PublicMapKey != "1-2" {
		t.Fatalf("portal response = %+v, want destination 1-2 snapshot", responsePayload)
	}
	if responsePayload.Snapshot.Map.Protection == nil ||
		responsePayload.Snapshot.Map.Protection.Reason != protectionReasonPortal ||
		responsePayload.Snapshot.Map.Protection.ExpiresAt < requestedAt.Add(9*time.Second).UTC().UnixMilli() ||
		!responsePayload.Snapshot.Map.Protection.BlocksPVP ||
		!responsePayload.Snapshot.Map.Protection.BreakOnPVPAction {
		t.Fatalf("portal response protection = %+v, want 10s server-owned portal protection", responsePayload.Snapshot.Map.Protection)
	}
	if responsePayload.Snapshot.Map.SafeZone == nil ||
		!responsePayload.Snapshot.Map.SafeZone.Inside ||
		!responsePayload.Snapshot.Map.SafeZone.BlocksPVP ||
		responsePayload.Snapshot.Map.SafeZone.ProtectionExpiresAt != responsePayload.Snapshot.Map.Protection.ExpiresAt {
		t.Fatalf("portal response safe-zone summary = %+v, protection = %+v", responsePayload.Snapshot.Map.SafeZone, responsePayload.Snapshot.Map.Protection)
	}
	if responsePayload.MapSubscriptionEpoch == 0 || responsePayload.MapSubscriptionEpoch <= oldEpoch || responsePayload.Snapshot.MapSubscriptionEpoch != responsePayload.MapSubscriptionEpoch {
		t.Fatalf("portal response epoch old=%d payload=%d snapshot=%d", oldEpoch, responsePayload.MapSubscriptionEpoch, responsePayload.Snapshot.MapSubscriptionEpoch)
	}

	gameServer.runtime.mu.Lock()
	starter, _ := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	destination, _ := gameServer.runtime.mapInstanceLocked("map_1_2")
	_, sourceHasPlayer := starter.Worker.PlayerEntity(resolved.PlayerID)
	entity, destinationHasPlayer := destination.Worker.PlayerEntity(resolved.PlayerID)
	for _, sessionID := range []auth.SessionID{resolved.SessionID, secondLogin.Session.SessionID} {
		if starter.ActiveSessions[sessionID] != "" {
			t.Fatalf("starter active session %q remained after transfer: %+v", sessionID, starter.ActiveSessions)
		}
		if _, ok := starter.LastAOI[sessionID]; ok {
			t.Fatalf("starter LastAOI retained transferred session %q", sessionID)
		}
		if destination.ActiveSessions[sessionID] != resolved.PlayerID {
			t.Fatalf("destination session %q = %q, want %q", sessionID, destination.ActiveSessions[sessionID], resolved.PlayerID)
		}
		if _, ok := destination.LastAOI[sessionID]; !ok {
			t.Fatalf("destination LastAOI missing session %q", sessionID)
		}
		if gameServer.runtime.sessionLocations[sessionID] != "map_1_2" {
			t.Fatalf("session %q location = %q, want map_1_2", sessionID, gameServer.runtime.sessionLocations[sessionID])
		}
	}
	gameServer.runtime.mu.Unlock()
	if sourceHasPlayer {
		t.Fatalf("starter worker still has player %q after transfer", resolved.PlayerID)
	}
	if !destinationHasPlayer || entity.Position != (world.Vec2{X: 400, Y: 5000}) {
		t.Fatalf("destination player entity = %+v ok=%v, want west gate spawn", entity, destinationHasPlayer)
	}

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(resolved.SessionID, realtime.OperationPortalEnter, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post portal events: %v", err)
	}
	for _, sessionID := range []auth.SessionID{resolved.SessionID, secondLogin.Session.SessionID} {
		events := eventsBySession[sessionID]
		started := requireEventTypeForTest(t, events, realtime.EventMapTransferStarted)
		protectionUpdated := requireEventTypeForTest(t, events, realtime.EventPlayerProtection)
		completed := requireEventTypeForTest(t, events, realtime.EventMapTransferCompleted)
		if epochForEventForTest(t, started) == 0 || epochForEventForTest(t, protectionUpdated) == 0 || epochForEventForTest(t, completed) == 0 {
			t.Fatalf("transfer events for %q missing epochs: %+v", sessionID, events)
		}
		var protectionPayload playerProtectionUpdatedPayload
		if err := json.Unmarshal(protectionUpdated.Payload, &protectionPayload); err != nil {
			t.Fatalf("decode protection payload: %v", err)
		}
		if protectionPayload.Reason != protectionReasonPortal ||
			protectionPayload.PublicMapKey != "1-2" ||
			protectionPayload.ExpiresAt != responsePayload.Snapshot.Map.Protection.ExpiresAt ||
			!protectionPayload.BlocksPVP ||
			!protectionPayload.BreakOnPVPAction {
			t.Fatalf("protection payload = %+v, want client-safe destination protection", protectionPayload)
		}
		for _, forbidden := range []string{"internal_map_id", "map_1_2", "spawn", "worker"} {
			if strings.Contains(string(protectionUpdated.Payload), forbidden) {
				t.Fatalf("protection event leaked %q in %s", forbidden, protectionUpdated.Payload)
			}
		}
		var completedPayload mapTransferCompletedPayload
		if err := json.Unmarshal(completed.Payload, &completedPayload); err != nil {
			t.Fatalf("decode completed payload: %v", err)
		}
		if completedPayload.Snapshot.Map.PublicMapKey != "1-2" || completedPayload.Snapshot.MapSubscriptionEpoch == 0 {
			t.Fatalf("completed payload = %+v, want destination snapshot with epoch", completedPayload)
		}
		if completedPayload.Snapshot.Map.Protection == nil ||
			completedPayload.Snapshot.Map.Protection.ExpiresAt != responsePayload.Snapshot.Map.Protection.ExpiresAt {
			t.Fatalf("completed protection = %+v, want response protection expiry", completedPayload.Snapshot.Map.Protection)
		}
	}
}

func TestWorldSnapshotIgnoresAndClearsExpiredProtection(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "expired-protection@example.com", "Expired Protection", "map_1_2", "west_gate")

	gameServer.runtime.mu.Lock()
	protection, err := gameServer.runtime.startPortalProtectionLocked(resolved.PlayerID, "map_1_2")
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("startPortalProtectionLocked() error = %v, want nil", err)
	}
	protection.ExpiresAt = gameServer.runtime.clock.Now().Add(-time.Second)
	gameServer.runtime.playerProtections[protectionKey{PlayerID: resolved.PlayerID, MapID: "map_1_2"}] = protection
	snapshot, err := gameServer.runtime.worldSnapshotForSessionLocked(resolved.PlayerID, resolved.SessionID)
	_, stillStored := gameServer.runtime.playerProtections[protectionKey{PlayerID: resolved.PlayerID, MapID: "map_1_2"}]
	gameServer.runtime.mu.Unlock()
	if err != nil {
		t.Fatalf("worldSnapshotForSessionLocked() error = %v, want nil", err)
	}
	if snapshot.Map.Protection != nil {
		t.Fatalf("expired protection projected in snapshot: %+v", snapshot.Map.Protection)
	}
	if stillStored {
		t.Fatalf("expired protection remained stored")
	}
}

func TestPortalEnterRollbackCleansDestinationAfterSessionAttachFailure(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "portal-rollback@example.com", "Portal Rollback")
	secondLogin, err := gameServer.runtime.Auth.Login(context.Background(), auth.LoginInput{
		Email:    resolved.Email.String(),
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("second login error = %v, want nil", err)
	}
	if err := gameServer.runtime.ensurePlayerSession(secondLogin.Session); err != nil {
		t.Fatalf("ensure second session: %v", err)
	}
	moveTestPlayerEntity(gameServer, resolved.PlayerID, world.Vec2{X: 9800, Y: 5000})

	previousHook := portalTransferInterleaveTestHook
	defer func() { portalTransferInterleaveTestHook = previousHook }()
	var hookRan bool
	var hookContext portalTransferInterleaveContext
	portalTransferInterleaveTestHook = func(stage portalTransferInterleaveStage, runtime *Runtime, context portalTransferInterleaveContext) error {
		if stage != portalTransferAfterDestinationSessionAttach || hookRan {
			return nil
		}
		hookRan = true
		hookContext = context
		return errors.New("forced destination session attach failure")
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-portal-rollback","op":"portal.enter","payload":{"portal_id":"east_gate"},"client_seq":1,"v":1}`),
	)
	if !hookRan {
		t.Fatalf("portal rollback hook did not run")
	}
	if hookContext.PlayerID != resolved.PlayerID || hookContext.DestinationMapID != "map_1_2" || len(hookContext.SessionIDs) != 2 {
		t.Fatalf("hook context = %+v, want destination map_1_2 and two sessions", hookContext)
	}
	if !response.HasError || response.Error.Error.Code != foundation.CodeInternal {
		t.Fatalf("portal rollback response = %+v, want internal error", response)
	}
	assertPlayerOnlyInMapForTest(t, gameServer, resolved.PlayerID, worldmaps.StarterMapID)

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	starter, _ := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	destination, _ := gameServer.runtime.mapInstanceLocked("map_1_2")
	if _, ok := destination.Worker.PlayerEntity(resolved.PlayerID); ok {
		t.Fatalf("destination retained player entity after rollback")
	}
	for _, sessionID := range []auth.SessionID{resolved.SessionID, secondLogin.Session.SessionID} {
		if attachedPlayer, ok := destination.Worker.AttachedPlayer(realtime.SessionID(sessionID.String())); ok {
			t.Fatalf("destination worker retained session %q for player %q after rollback", sessionID, attachedPlayer)
		}
		if destination.ActiveSessions[sessionID] != "" {
			t.Fatalf("destination active session %q remained after rollback: %+v", sessionID, destination.ActiveSessions)
		}
		if _, ok := destination.LastAOI[sessionID]; ok {
			t.Fatalf("destination LastAOI retained session %q after rollback", sessionID)
		}
		if starter.ActiveSessions[sessionID] != resolved.PlayerID {
			t.Fatalf("source active session %q = %q, want %q", sessionID, starter.ActiveSessions[sessionID], resolved.PlayerID)
		}
		if attachedPlayer, ok := starter.Worker.AttachedPlayer(realtime.SessionID(sessionID.String())); !ok || attachedPlayer != resolved.PlayerID {
			t.Fatalf("source worker session %q = %q ok=%v, want %q", sessionID, attachedPlayer, ok, resolved.PlayerID)
		}
		if gameServer.runtime.sessionLocations[sessionID] != worldmaps.StarterMapID {
			t.Fatalf("session %q location = %q, want starter map", sessionID, gameServer.runtime.sessionLocations[sessionID])
		}
	}
	if len(gameServer.runtime.portalCooldowns) != 0 {
		t.Fatalf("portal cooldowns = %+v, want none after rollback", gameServer.runtime.portalCooldowns)
	}
	if len(gameServer.runtime.portalAttempts) != 0 {
		t.Fatalf("portal attempts = %+v, want none for failed transfer", gameServer.runtime.portalAttempts)
	}
	if len(gameServer.runtime.activeTransfers) != 0 {
		t.Fatalf("active transfers = %+v, want cleared after rollback", gameServer.runtime.activeTransfers)
	}
}

func TestPortalEnterDuplicateAndOldEpochQueuedEventsDoNotDuplicateOrLeak(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	traveler := createResolvedRuntimeSession(t, gameServer, "portal-epoch-traveler@example.com", "Epoch Traveler")
	sourceViewer := createResolvedRuntimeSession(t, gameServer, "portal-epoch-source@example.com", "Epoch Source")
	if _, err := gameServer.runtime.bootstrapEvents(traveler); err != nil {
		t.Fatalf("bootstrap traveler: %v", err)
	}
	if _, err := gameServer.runtime.bootstrapEvents(sourceViewer); err != nil {
		t.Fatalf("bootstrap source viewer: %v", err)
	}
	moveTestPlayerEntity(gameServer, traveler.PlayerID, world.Vec2{X: 9800, Y: 5000})

	gameServer.runtime.mu.Lock()
	oldEpoch := gameServer.runtime.sessionMapEpochLocked(traveler.SessionID)
	gameServer.runtime.queueEventLocked(traveler.SessionID, realtime.EventAOIEntityEntered, map[string]any{
		"entity_id":   "origin-queued-before-transfer",
		"entity_type": "npc",
		"position":    world.Vec2{X: 1, Y: 1},
	})
	gameServer.runtime.mu.Unlock()

	request := []byte(`{"request_id":"request-portal-idempotent","op":"portal.enter","payload":{"portal_id":"east_gate"},"client_seq":1,"v":1}`)
	first := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(traveler.SessionID.String()), request)
	if first.HasError {
		t.Fatalf("first portal response error = %+v, want success", first.Error)
	}
	duplicate := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(traveler.SessionID.String()), request)
	if duplicate.HasError {
		t.Fatalf("duplicate portal response error = %+v, want same success", duplicate.Error)
	}
	if string(first.Response.Payload) != string(duplicate.Response.Payload) {
		t.Fatalf("duplicate portal payload changed:\nfirst=%s\nduplicate=%s", first.Response.Payload, duplicate.Response.Payload)
	}

	gameServer.runtime.mu.Lock()
	newEpoch := gameServer.runtime.sessionMapEpochLocked(traveler.SessionID)
	gameServer.runtime.mu.Unlock()
	if newEpoch <= oldEpoch {
		t.Fatalf("epoch old=%d new=%d, want rebind increment", oldEpoch, newEpoch)
	}

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(traveler.SessionID, realtime.OperationPortalEnter, traveler.PlayerID)
	if err != nil {
		t.Fatalf("post transfer events: %v", err)
	}
	for _, event := range eventsBySession[traveler.SessionID] {
		if strings.Contains(string(event.Payload), "origin-queued-before-transfer") {
			t.Fatalf("old epoch origin event leaked after transfer: %+v", eventsBySession[traveler.SessionID])
		}
	}
	requireEventTypeForTest(t, eventsBySession[traveler.SessionID], realtime.EventMapTransferCompleted)
	assertPlayerOnlyInMapForTest(t, gameServer, traveler.PlayerID, "map_1_2")

	gameServer.runtime.mu.Lock()
	insertTestWorldEntityInMapLocked(t, gameServer, worldmaps.StarterMapID, "entity_source_after_transfer", world.EntityTypeNPC, world.Vec2{X: 10, Y: 0}, false)
	insertTestWorldEntityInMapLocked(t, gameServer, "map_1_2", "entity_destination_after_transfer", world.EntityTypeNPC, world.Vec2{X: 410, Y: 5000}, false)
	gameServer.runtime.mu.Unlock()
	tickEvents := gameServer.runtime.tickAndCollectAOIEvents()
	assertEventsContainEntityOnly(t, tickEvents[sourceViewer.SessionID], "entity_source_after_transfer", "entity_destination_after_transfer")
	assertEventsContainEntityOnly(t, tickEvents[traveler.SessionID], "entity_destination_after_transfer", "entity_source_after_transfer")
}

func TestWorldSnapshotBootstrapIncludesMapSubscriptionEpoch(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "snapshot-epoch@example.com", "Snapshot Epoch")

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	snapshot := decodeWorldSnapshotForTest(t, events)
	if snapshot.Map.PublicMapKey != "1-1" || snapshot.MapSubscriptionEpoch == 0 {
		t.Fatalf("bootstrap snapshot = %+v, want current map metadata and nonzero epoch", snapshot)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-world-snapshot-epoch","op":"world.snapshot","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("world snapshot response error = %+v, want success", response.Error)
	}
	var responseSnapshot worldSnapshotPayload
	if err := json.Unmarshal(response.Response.Payload, &responseSnapshot); err != nil {
		t.Fatalf("decode world snapshot response: %v", err)
	}
	if responseSnapshot.Map.PublicMapKey != "1-1" || responseSnapshot.MapSubscriptionEpoch != snapshot.MapSubscriptionEpoch {
		t.Fatalf("response snapshot = %+v, want same active map epoch %d", responseSnapshot, snapshot.MapSubscriptionEpoch)
	}
}

func TestLiveCommandsRejectWhileTransferActive(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "transfer-guard@example.com", "Transfer Guard")

	gameServer.runtime.mu.Lock()
	gameServer.runtime.activeTransfers[resolved.PlayerID] = portalTransferState{
		PlayerID:    resolved.PlayerID,
		SourceMapID: worldmaps.StarterMapID,
		PortalID:    "east_gate",
		RequestID:   "request-active-transfer",
	}
	gameServer.runtime.mu.Unlock()

	requests := []string{
		`{"request_id":"request-transfer-move","op":"move_to","payload":{"target":{"x":10,"y":10}},"client_seq":1,"v":1}`,
		`{"request_id":"request-transfer-stop","op":"stop","payload":{},"client_seq":2,"v":1}`,
		`{"request_id":"request-transfer-combat","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"missing-target"},"client_seq":3,"v":1}`,
		`{"request_id":"request-transfer-loot","op":"loot.pickup","payload":{"drop_id":"missing-drop"},"client_seq":4,"v":1}`,
		`{"request_id":"request-transfer-scan","op":"scan.pulse","payload":{},"client_seq":5,"v":1}`,
		`{"request_id":"request-transfer-stealth","op":"stealth.toggle","payload":{"enabled":true},"client_seq":6,"v":1}`,
		`{"request_id":"request-transfer-portal","op":"portal.enter","payload":{"portal_id":"east_gate"},"client_seq":7,"v":1}`,
	}
	for _, request := range requests {
		response := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), []byte(request))
		if !response.HasError || response.Error.Error.Code != foundation.CodeForbidden {
			t.Fatalf("active-transfer request %s response = %+v, want forbidden", request, response)
		}
	}
	assertPlayerOnlyInMapForTest(t, gameServer, resolved.PlayerID, worldmaps.StarterMapID)
}

func TestScanPulseRejectsPortalTransferInterleavingBeforeQueue(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "scan-transfer-race@example.com", "Scan Transfer Race")
	secondLogin, err := gameServer.runtime.Auth.Login(context.Background(), auth.LoginInput{
		Email:    resolved.Email.String(),
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("second login error = %v, want nil", err)
	}
	if err := gameServer.runtime.ensurePlayerSession(secondLogin.Session); err != nil {
		t.Fatalf("ensure second session: %v", err)
	}
	moveTestPlayerEntity(gameServer, resolved.PlayerID, world.Vec2{X: 9800, Y: 5000})

	gameServer.runtime.mu.Lock()
	sourceEpoch := gameServer.runtime.sessionMapEpochLocked(resolved.SessionID)
	gameServer.runtime.mu.Unlock()

	previousHook := scanPulseInterleaveTestHook
	defer func() { scanPulseInterleaveTestHook = previousHook }()
	var portalAttempted bool
	var portalResponse realtime.CachedResponse
	scanPulseInterleaveTestHook = func(stage scanPulseInterleaveStage, runtime *Runtime, guard scanPulseMapGuard) {
		if stage != scanPulseInterleaveBeforeQueue || portalAttempted {
			return
		}
		portalAttempted = true
		portalResponse = runtime.Gateway.HandleRequest(
			realtime.SessionID(secondLogin.Session.SessionID.String()),
			[]byte(`{"request_id":"request-portal-during-scan","op":"portal.enter","payload":{"portal_id":"east_gate"},"client_seq":1,"v":1}`),
		)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-scan-transfer-race","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("scan response error = %+v, want success with portal attempt rejected", response.Error)
	}
	if !portalAttempted {
		t.Fatalf("portal interleave hook did not run")
	}
	if !portalResponse.HasError || portalResponse.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("portal during scan response = %+v, want forbidden", portalResponse)
	}
	assertPlayerOnlyInMapForTest(t, gameServer, resolved.PlayerID, worldmaps.StarterMapID)

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(resolved.SessionID, realtime.OperationScanPulse, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post scan events: %v", err)
	}
	for _, event := range eventsBySession[resolved.SessionID] {
		if event.Type == realtime.EventMapTransferStarted || event.Type == realtime.EventMapTransferCompleted {
			t.Fatalf("scan interleave emitted transfer event after portal rejection: %+v", eventsBySession[resolved.SessionID])
		}
		if epoch, ok := eventMapSubscriptionEpoch(event); ok && epoch != sourceEpoch {
			t.Fatalf("scan event %s epoch = %d, want source epoch %d: %s", event.Type, epoch, sourceEpoch, event.Payload)
		}
		if strings.Contains(string(event.Payload), `"public_map_key":"1-2"`) || strings.Contains(string(event.Payload), `"map_key":"1-2"`) {
			t.Fatalf("scan event leaked destination map payload: %+v", event)
		}
	}
}

func TestScanPulseAbortsWhenMapEpochChangesBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "scan-epoch-abort@example.com", "Scan Epoch Abort")

	gameServer.runtime.mu.Lock()
	startCapacitor := gameServer.runtime.players[resolved.PlayerID].Ship.Capacitor
	startEpoch := gameServer.runtime.sessionMapEpochLocked(resolved.SessionID)
	gameServer.runtime.mu.Unlock()

	previousHook := scanPulseInterleaveTestHook
	defer func() { scanPulseInterleaveTestHook = previousHook }()
	var hookRan bool
	var hookErr error
	scanPulseInterleaveTestHook = func(stage scanPulseInterleaveStage, runtime *Runtime, guard scanPulseMapGuard) {
		if stage != scanPulseInterleaveBeforeMutation || hookRan {
			return
		}
		hookRan = true
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		if _, hookErr = runtime.mapRouter.SetActiveLocationFromSpawn(guard.PlayerID, "map_1_2", "west_gate"); hookErr != nil {
			return
		}
		destination, err := runtime.mapInstanceLocked("map_1_2")
		if err != nil {
			hookErr = err
			return
		}
		runtime.attachSessionToInstanceLocked(destination, guard.SessionID, guard.PlayerID)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-scan-epoch-abort","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`),
	)
	if hookErr != nil {
		t.Fatalf("hook error = %v", hookErr)
	}
	if !hookRan {
		t.Fatalf("map epoch interleave hook did not run")
	}
	if !response.HasError || response.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("scan response = %+v, want forbidden after map epoch change", response)
	}

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	if len(gameServer.runtime.activeScanPulses) != 0 {
		t.Fatalf("active scan guards = %+v, want cleared", gameServer.runtime.activeScanPulses)
	}
	if len(gameServer.runtime.scanCooldowns) != 0 || len(gameServer.runtime.scanCapacitorSpends) != 0 {
		t.Fatalf("scan mutation occurred after epoch change: cooldowns=%+v capacitor_spends=%+v", gameServer.runtime.scanCooldowns, gameServer.runtime.scanCapacitorSpends)
	}
	if got := gameServer.runtime.players[resolved.PlayerID].Ship.Capacitor; got != startCapacitor {
		t.Fatalf("ship capacitor = %d, want unchanged %d", got, startCapacitor)
	}
	if len(gameServer.runtime.queuedEvents[resolved.SessionID]) != 0 {
		t.Fatalf("queued scan events = %+v, want none", gameServer.runtime.queuedEvents[resolved.SessionID])
	}
	if got := gameServer.runtime.sessionMapEpochLocked(resolved.SessionID); got <= startEpoch {
		t.Fatalf("test hook epoch = %d, want > start epoch %d", got, startEpoch)
	}
}

func assertPlayerOnlyInMapForTest(t *testing.T, gameServer *Server, playerID foundation.PlayerID, wantMapID worldmaps.MapID) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	for mapID, instance := range gameServer.runtime.mapInstances {
		_, hasPlayer := instance.Worker.PlayerEntity(playerID)
		if mapID == wantMapID {
			if !hasPlayer {
				t.Fatalf("map %q missing player %q", mapID, playerID)
			}
			continue
		}
		if hasPlayer {
			t.Fatalf("map %q unexpectedly has player %q", mapID, playerID)
		}
	}
	location, err := gameServer.runtime.mapRouter.ActiveLocation(playerID)
	if err != nil {
		t.Fatalf("active location: %v", err)
	}
	if location.InternalMapID != wantMapID {
		t.Fatalf("active map = %q, want %q", location.InternalMapID, wantMapID)
	}
}

func requireEventTypeForTest(t *testing.T, events []realtime.EventEnvelope, eventType realtime.ClientEventType) realtime.EventEnvelope {
	t.Helper()
	for _, event := range events {
		if event.Type == eventType {
			return event
		}
	}
	t.Fatalf("events = %+v, missing %s", events, eventType)
	return realtime.EventEnvelope{}
}

func epochForEventForTest(t *testing.T, event realtime.EventEnvelope) uint64 {
	t.Helper()
	var payload struct {
		MapSubscriptionEpoch uint64 `json:"map_subscription_epoch"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode event epoch: %v", err)
	}
	return payload.MapSubscriptionEpoch
}
