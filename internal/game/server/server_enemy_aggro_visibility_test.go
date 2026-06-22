package server

import (
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

func TestRuntimeStealthSyncPreventsNPCAggroMovementTargetLeak(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	target := createResolvedRuntimeSession(t, gameServer, "aggro-hidden-target@example.com", "Aggro Hidden")
	viewer := createResolvedRuntimeSession(t, gameServer, "aggro-viewer@example.com", "Aggro Viewer")
	targetPosition := world.Vec2{X: 850, Y: 400}

	gameServer.runtime.mu.Lock()
	npcID := installAggressiveStarterNPCForAggroVisibilityTestLocked(t, gameServer, target.PlayerID, targetPosition, viewer.PlayerID, world.Vec2{X: 800, Y: 150})
	before, _, _, err := gameServer.runtime.aoiSnapshotForPlayerLocked(viewer.PlayerID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("before aoiSnapshotForPlayerLocked() error = %v, want nil", err)
	}
	targetEntityID := testPlayerEntityIDLocked(t, gameServer, target.PlayerID)
	gameServer.runtime.mu.Unlock()

	if !hasEntityID(before.Entities, targetEntityID.String()) {
		t.Fatalf("before stealth snapshot entities = %+v, want visible target player", before.Entities)
	}
	beforeNPC, ok := entityPayloadByID(before.Entities, npcID.String())
	if !ok {
		t.Fatalf("before stealth snapshot entities = %+v, want visible NPC %q", before.Entities, npcID)
	}
	if beforeNPC.Movement == nil || beforeNPC.Movement.Target != targetPosition {
		t.Fatalf("before stealth NPC movement = %+v, want public chase target %+v for visible player", beforeNPC.Movement, targetPosition)
	}

	if err := gameServer.runtime.setPlayerStealth(target.PlayerID, true); err != nil {
		t.Fatalf("setPlayerStealth(true) error = %v, want nil", err)
	}

	gameServer.runtime.mu.Lock()
	after, _, _, err := gameServer.runtime.aoiSnapshotForPlayerLocked(viewer.PlayerID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("after aoiSnapshotForPlayerLocked() error = %v, want nil", err)
	}
	record, _ := gameServer.runtime.Worker.EnemySpawnRecord(npcID)
	npcEntity, _ := gameServer.runtime.Worker.Entity(npcID)
	gameServer.runtime.mu.Unlock()

	if hasEntityID(after.Entities, targetEntityID.String()) {
		t.Fatalf("after stealth snapshot entities = %+v, want hidden target omitted", after.Entities)
	}
	afterNPC, ok := entityPayloadByID(after.Entities, npcID.String())
	if !ok {
		t.Fatalf("after stealth snapshot entities = %+v, want visible NPC %q", after.Entities, npcID)
	}
	if afterNPC.Movement != nil && afterNPC.Movement.Target == targetPosition {
		t.Fatalf("after stealth NPC public movement = %+v, leaked hidden target coordinate %+v", afterNPC.Movement, targetPosition)
	}
	if !record.AggroTargetEntityID.IsZero() {
		t.Fatalf("worker aggro target = %q, want cleared for hidden target", record.AggroTargetEntityID)
	}
	if npcEntity.Movement.Moving && npcEntity.Movement.Target == targetPosition {
		t.Fatalf("worker NPC movement = %+v, want no retained hidden target coordinate", npcEntity.Movement)
	}
}

func installAggressiveStarterNPCForAggroVisibilityTestLocked(
	t *testing.T,
	gameServer *Server,
	targetPlayerID foundation.PlayerID,
	targetPosition world.Vec2,
	viewerPlayerID foundation.PlayerID,
	viewerPosition world.Vec2,
) world.EntityID {
	t.Helper()

	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		t.Fatalf("starter map instance: %v", err)
	}
	definition := aggressiveStarterDefinitionForAggroVisibilityTest(starter.Definition)
	replacement, err := worker.NewWorker(worker.Config{
		WorldID:   definition.WorldID,
		ZoneID:    definition.ZoneID,
		TickDelta: gameServer.runtime.Worker.TickDelta(),
		Clock:     gameServer.runtime.clock,
	})
	if err != nil {
		t.Fatalf("NewWorker(aggressive starter) error = %v, want nil", err)
	}
	starter.Definition = definition
	starter.Worker = replacement
	gameServer.runtime.Worker = replacement

	overrides := map[worldmaps.EnemyPoolID][]world.EntityID{
		"starter_training_drone_pool": {"entity_training_npc"},
	}
	if err := commandErrorsFromSubmitAndTick(replacement, worker.InitializeEnemyPoolsCommand{
		Definition:        definition,
		EntityIDOverrides: overrides,
	}); err != nil {
		t.Fatalf("InitializeEnemyPoolsCommand error = %v, want nil", err)
	}
	targetState := gameServer.runtime.players[targetPlayerID]
	if err := commandErrorsFromSubmitAndTick(replacement, worker.SpawnPlayerCommand{
		PlayerID: targetPlayerID,
		EntityID: targetState.EntityID,
		Position: targetPosition,
		Speed:    defaultPlayerSpeed,
	}); err != nil {
		t.Fatalf("SpawnPlayerCommand(target) error = %v, want nil", err)
	}
	viewerState := gameServer.runtime.players[viewerPlayerID]
	if err := commandErrorsFromSubmitAndTick(replacement, worker.SpawnPlayerCommand{
		PlayerID: viewerPlayerID,
		EntityID: viewerState.EntityID,
		Position: viewerPosition,
		Speed:    defaultPlayerSpeed,
	}); err != nil {
		t.Fatalf("SpawnPlayerCommand(viewer) error = %v, want nil", err)
	}
	if err := gameServer.runtime.syncAliveNPCCombatActorProjectionsLocked(starter); err != nil {
		t.Fatalf("syncAliveNPCCombatActorProjectionsLocked() error = %v, want nil", err)
	}
	record, ok := replacement.EnemySpawnRecord("entity_training_npc")
	if !ok {
		t.Fatalf("EnemySpawnRecord(entity_training_npc) missing; snapshot=%+v", replacement.EnemySpawnSnapshot())
	}
	if record.AggroTargetEntityID != targetState.EntityID {
		t.Fatalf("initial aggro record = %+v, want target entity %q", record, targetState.EntityID)
	}
	return record.EntityID
}

func aggressiveStarterDefinitionForAggroVisibilityTest(definition worldmaps.MapDefinition) worldmaps.MapDefinition {
	definition.NPCStatTemplates[0].Speed = 100
	definition.NPCAggroProfiles[0].AggroRadius = 120
	definition.NPCAggroProfiles[0].TargetMemory = 5 * time.Second
	definition.NPCAggroProfiles[0].SafeZoneAttackPolicy = "never"
	definition.NPCLeashProfiles[0].LeashDistance = 1000
	definition.NPCLeashProfiles[0].ResetOnBreak = true
	return definition
}
