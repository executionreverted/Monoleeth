package server

import (
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

func TestCombatEngagementStartStoresPlayerTargetAndSkill(t *testing.T) {
	runtime := &Runtime{}
	now := time.UnixMilli(1_827_360_000_000)

	state := runtime.startCombatEngagementLocked("player-1", "npc-1", "basic_laser", now)

	if state.PlayerID != "player-1" || state.TargetID != "npc-1" || state.SkillID != "basic_laser" {
		t.Fatalf("combat engagement state = %+v, want player target and skill", state)
	}
	if !state.StartedAt.Equal(now) || !state.NextFireAt.Equal(now) {
		t.Fatalf("combat engagement timing = started %s next %s, want %s", state.StartedAt, state.NextFireAt, now)
	}
	snapshot := runtime.combatEngagementSnapshotLocked("player-1", now)
	if !snapshot.Active || snapshot.TargetID != "npc-1" || snapshot.SkillID != "basic_laser" {
		t.Fatalf("combat engagement snapshot = %+v, want active npc-1 basic_laser", snapshot)
	}
}

func TestCombatEngagementStartDefaultsSkillAndIsIdempotentForSameTarget(t *testing.T) {
	runtime := &Runtime{}
	now := time.UnixMilli(1_827_360_000_000)
	later := now.Add(10 * time.Second)

	first := runtime.startCombatEngagementLocked("player-1", "npc-1", "", now)
	second := runtime.startCombatEngagementLocked("player-1", "npc-1", defaultCombatEngagementSkillID, later)

	if first.SkillID != defaultCombatEngagementSkillID {
		t.Fatalf("default skill = %q, want %q", first.SkillID, defaultCombatEngagementSkillID)
	}
	if !second.StartedAt.Equal(now) || !second.NextFireAt.Equal(now) {
		t.Fatalf("duplicate start changed timing: %+v, want original %s", second, now)
	}
}

func TestCombatEngagementStartReplacesDifferentTarget(t *testing.T) {
	runtime := &Runtime{}
	now := time.UnixMilli(1_827_360_000_000)
	later := now.Add(10 * time.Second)

	runtime.startCombatEngagementLocked("player-1", "npc-1", "basic_laser", now)
	replaced := runtime.startCombatEngagementLocked("player-1", "npc-2", "basic_laser", later)

	if replaced.TargetID != "npc-2" {
		t.Fatalf("target = %q, want npc-2", replaced.TargetID)
	}
	if !replaced.StartedAt.Equal(later) || !replaced.NextFireAt.Equal(later) {
		t.Fatalf("replacement timing = started %s next %s, want %s", replaced.StartedAt, replaced.NextFireAt, later)
	}
}

func TestCombatEngagementStopRemovesStateAndRecordsReason(t *testing.T) {
	runtime := &Runtime{}
	now := time.UnixMilli(1_827_360_000_000)

	runtime.startCombatEngagementLocked("player-1", "npc-1", "basic_laser", now)
	snapshot := runtime.stopCombatEngagementLocked("player-1", combatStopReasonManual, now.Add(time.Second))

	if snapshot.Active {
		t.Fatalf("snapshot active = true, want stopped")
	}
	if snapshot.LastStopReason != string(combatStopReasonManual) {
		t.Fatalf("stop reason = %q, want %q", snapshot.LastStopReason, combatStopReasonManual)
	}
	if _, ok := runtime.activeCombatEngagements["player-1"]; ok {
		t.Fatal("combat engagement still active after stop")
	}
}

func TestCombatEngagementClearForTargetEntity(t *testing.T) {
	runtime := &Runtime{}
	now := time.UnixMilli(1_827_360_000_000)

	runtime.startCombatEngagementLocked("player-1", "npc-1", "basic_laser", now)
	runtime.startCombatEngagementLocked("player-2", "npc-2", "basic_laser", now)

	stopped := runtime.clearCombatEngagementsForEntityLocked("npc-1", combatStopReasonTargetDestroyed, now.Add(time.Second))

	if len(stopped) != 1 || stopped[0].PlayerID != "player-1" || stopped[0].LastStopReason != string(combatStopReasonTargetDestroyed) {
		t.Fatalf("stopped engagements = %+v, want only player-1 target_destroyed", stopped)
	}
	if _, ok := runtime.activeCombatEngagements["player-1"]; ok {
		t.Fatal("player-1 engagement still active after target clear")
	}
	if _, ok := runtime.activeCombatEngagements["player-2"]; !ok {
		t.Fatal("player-2 engagement removed for unrelated target")
	}
}

func TestCombatEngagementClearForPlayerEntity(t *testing.T) {
	runtime := &Runtime{
		players: map[foundation.PlayerID]playerRuntimeState{
			"player-1": {EntityID: world.EntityID("entity-player-1")},
			"player-2": {EntityID: world.EntityID("entity-player-2")},
		},
	}
	now := time.UnixMilli(1_827_360_000_000)

	runtime.startCombatEngagementLocked("player-1", "npc-1", "basic_laser", now)
	runtime.startCombatEngagementLocked("player-2", "npc-2", "basic_laser", now)

	stopped := runtime.clearCombatEngagementsForEntityLocked("entity-player-1", combatStopReasonMapChanged, now.Add(time.Second))

	if len(stopped) != 1 || stopped[0].PlayerID != "player-1" || stopped[0].LastStopReason != string(combatStopReasonMapChanged) {
		t.Fatalf("stopped engagements = %+v, want only player-1 map_changed", stopped)
	}
	if _, ok := runtime.activeCombatEngagements["player-1"]; ok {
		t.Fatal("player-1 engagement still active after player entity clear")
	}
	if _, ok := runtime.activeCombatEngagements["player-2"]; !ok {
		t.Fatal("player-2 engagement removed for unrelated player entity")
	}
}
