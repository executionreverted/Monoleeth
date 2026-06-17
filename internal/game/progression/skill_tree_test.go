package progression

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestPilotSkillDefinitionsExposeThreeBranchesWithChains(t *testing.T) {
	definitions := PilotSkillDefinitions()
	branches := make(map[PilotSkillBranch]int)
	hasPrerequisiteByBranch := make(map[PilotSkillBranch]bool)
	seen := make(map[SkillNodeID]struct{})

	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			t.Fatalf("definition %q Validate() = %v, want nil", definition.NodeID, err)
		}
		branches[definition.Branch]++
		seen[definition.NodeID] = struct{}{}
		if len(definition.PrerequisiteNodes) > 0 {
			hasPrerequisiteByBranch[definition.Branch] = true
		}
	}

	if len(branches) < 3 {
		t.Fatalf("branch count = %d, want at least 3", len(branches))
	}
	for branch := range branches {
		if !hasPrerequisiteByBranch[branch] {
			t.Fatalf("branch %q has no prerequisite chain", branch)
		}
	}
	if _, ok := seen["combat_weapon_calibration"]; !ok {
		t.Fatal("combat root skill missing")
	}
	if _, ok := seen["scout_signal_tuning"]; !ok {
		t.Fatal("scout root skill missing")
	}
	if _, ok := seen["industry_cargo_protocols"]; !ok {
		t.Fatal("industry root skill missing")
	}

	definitions[0].NodeID = "tampered"
	definition, err := PilotSkillDefinitionFor("combat_weapon_calibration")
	if err != nil {
		t.Fatalf("PilotSkillDefinitionFor() = %v, want nil", err)
	}
	if definition.NodeID != "combat_weapon_calibration" {
		t.Fatal("PilotSkillDefinitions returned mutable internal definitions")
	}
}

func TestUnlockPilotSkillValidatesLockedNodesAndConsumesPointOnce(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 17, 0, 0, 0, time.UTC))
	store := NewInMemoryProgressionStore()
	service := NewProgressionService(clock, store)

	if _, err := service.UnlockPilotSkill(UnlockPilotSkillInput{
		PlayerID: "player-1",
		NodeID:   "missing_node",
	}); !errors.Is(err, ErrUnknownSkillNode) {
		t.Fatalf("unknown node UnlockPilotSkill() error = %v, want ErrUnknownSkillNode", err)
	}

	grantXPAndRankForSkillTest(t, service, "player-1", 100, []RoleXPGrant{{Role: RoleTypeCombat, Amount: 75}}, 2)

	if _, err := service.UnlockPilotSkill(UnlockPilotSkillInput{
		PlayerID: "player-1",
		NodeID:   "combat_heat_control",
	}); !errors.Is(err, ErrMissingSkillPrerequisite) {
		t.Fatalf("locked prerequisite UnlockPilotSkill() error = %v, want ErrMissingSkillPrerequisite", err)
	}

	firstUnlock, err := service.UnlockPilotSkill(UnlockPilotSkillInput{
		PlayerID: "player-1",
		NodeID:   "combat_weapon_calibration",
	})
	if err != nil {
		t.Fatalf("UnlockPilotSkill(root) = %v, want nil", err)
	}
	if !firstUnlock.Unlocked || firstUnlock.Duplicate {
		t.Fatalf("first unlock flags = unlocked %t duplicate %t, want true/false", firstUnlock.Unlocked, firstUnlock.Duplicate)
	}
	if firstUnlock.Snapshot.SkillPoints.SpentPoints != 1 || firstUnlock.Snapshot.SkillPoints.AvailablePoints() != 0 {
		t.Fatalf("skill points after unlock = spent %d available %d, want spent 1 available 0", firstUnlock.Snapshot.SkillPoints.SpentPoints, firstUnlock.Snapshot.SkillPoints.AvailablePoints())
	}
	if !firstUnlock.Snapshot.HasUnlockedSkillNode("combat_weapon_calibration") {
		t.Fatal("combat root node was not recorded as unlocked")
	}
	if len(firstUnlock.StatInvalidationSignals) != 1 ||
		firstUnlock.StatInvalidationSignals[0].Reason != StatInvalidationReasonPilotSkillUnlocked ||
		firstUnlock.StatInvalidationSignals[0].NodeID != "combat_weapon_calibration" {
		t.Fatalf("unlock stat invalidation = %+v, want one pilot skill unlocked signal", firstUnlock.StatInvalidationSignals)
	}

	duplicateUnlock, err := service.UnlockPilotSkill(UnlockPilotSkillInput{
		PlayerID: "player-1",
		NodeID:   "combat_weapon_calibration",
	})
	if err != nil {
		t.Fatalf("duplicate UnlockPilotSkill() = %v, want nil", err)
	}
	if !duplicateUnlock.Duplicate || duplicateUnlock.Unlocked {
		t.Fatalf("duplicate flags = duplicate %t unlocked %t, want true/false", duplicateUnlock.Duplicate, duplicateUnlock.Unlocked)
	}
	if duplicateUnlock.Snapshot.SkillPoints.SpentPoints != 1 || duplicateUnlock.Snapshot.SkillPoints.AvailablePoints() != 0 {
		t.Fatalf("skill points after duplicate = spent %d available %d, want spent 1 available 0", duplicateUnlock.Snapshot.SkillPoints.SpentPoints, duplicateUnlock.Snapshot.SkillPoints.AvailablePoints())
	}
	if len(duplicateUnlock.StatInvalidationSignals) != 0 {
		t.Fatalf("duplicate stat invalidation = %+v, want none", duplicateUnlock.StatInvalidationSignals)
	}

	if _, err := service.UnlockPilotSkill(UnlockPilotSkillInput{
		PlayerID: "player-1",
		NodeID:   "scout_signal_tuning",
	}); !errors.Is(err, ErrNotEnoughSkillPoints) {
		t.Fatalf("no-points UnlockPilotSkill() error = %v, want ErrNotEnoughSkillPoints", err)
	}
}

func TestUnlockPilotSkillChecksRankAndRoleRequirements(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 17, 30, 0, 0, time.UTC))
	store := NewInMemoryProgressionStore()
	service := NewProgressionService(clock, store)

	seedSkillPointsForSkillTest(t, store, "player-1", 1)
	if _, err := service.UnlockPilotSkill(UnlockPilotSkillInput{
		PlayerID: "player-1",
		NodeID:   "combat_weapon_calibration",
	}); !errors.Is(err, ErrRankRequirementNotMet) {
		t.Fatalf("rank-low UnlockPilotSkill() error = %v, want ErrRankRequirementNotMet", err)
	}

	grantXPAndRankForSkillTest(t, service, "player-2", 100, nil, 2)
	seedSkillPointsForSkillTest(t, store, "player-2", 2)
	if _, err := service.UnlockPilotSkill(UnlockPilotSkillInput{
		PlayerID: "player-2",
		NodeID:   "combat_weapon_calibration",
	}); err != nil {
		t.Fatalf("UnlockPilotSkill(root) = %v, want nil", err)
	}
	if _, err := service.UnlockPilotSkill(UnlockPilotSkillInput{
		PlayerID: "player-2",
		NodeID:   "combat_heat_control",
	}); !errors.Is(err, ErrRoleRequirementNotMet) {
		t.Fatalf("role-low UnlockPilotSkill() error = %v, want ErrRoleRequirementNotMet", err)
	}
}

func TestRespecPilotSkillsClearsUnlocksRefundsPointsAndInvalidatesStats(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 18, 0, 0, 0, time.UTC))
	store := NewInMemoryProgressionStore()
	service := NewProgressionService(clock, store)

	grantXPAndRankForSkillTest(t, service, "player-1", 300, []RoleXPGrant{{Role: RoleTypeCombat, Amount: 225}}, 3)
	for _, nodeID := range []SkillNodeID{"combat_weapon_calibration", "combat_heat_control"} {
		if _, err := service.UnlockPilotSkill(UnlockPilotSkillInput{
			PlayerID: "player-1",
			NodeID:   nodeID,
		}); err != nil {
			t.Fatalf("UnlockPilotSkill(%q) = %v, want nil", nodeID, err)
		}
	}

	result, err := service.RespecPilotSkills(RespecPilotSkillsInput{
		PlayerID: "player-1",
	})
	if err != nil {
		t.Fatalf("RespecPilotSkills() = %v, want nil", err)
	}
	if !result.Respecced {
		t.Fatal("Respecced = false, want true")
	}
	if result.RefundedPoints != 2 {
		t.Fatalf("RefundedPoints = %d, want 2", result.RefundedPoints)
	}
	if result.Snapshot.SkillPoints.SpentPoints != 0 || result.Snapshot.SkillPoints.AvailablePoints() != 2 {
		t.Fatalf("skill points after respec = spent %d available %d, want spent 0 available 2", result.Snapshot.SkillPoints.SpentPoints, result.Snapshot.SkillPoints.AvailablePoints())
	}
	if got := result.Snapshot.UnlockedSkillNodes(); len(got) != 0 {
		t.Fatalf("unlocked nodes after respec = %+v, want none", got)
	}
	wantCleared := []SkillNodeID{"combat_heat_control", "combat_weapon_calibration"}
	if !reflect.DeepEqual(result.ClearedNodeIDs, wantCleared) {
		t.Fatalf("ClearedNodeIDs = %+v, want %+v", result.ClearedNodeIDs, wantCleared)
	}
	if len(result.StatInvalidationSignals) != 1 ||
		result.StatInvalidationSignals[0].Reason != StatInvalidationReasonPilotSkillsRespecced ||
		result.StatInvalidationSignals[0].RefundedSkillPoints != 2 ||
		result.StatInvalidationSignals[0].ClearedSkillNodeCount != 2 {
		t.Fatalf("respec stat invalidation = %+v, want one respec signal", result.StatInvalidationSignals)
	}
}

func grantXPAndRankForSkillTest(t *testing.T, service *ProgressionService, playerID string, mainXP int64, roleXP []RoleXPGrant, targetRank int) {
	t.Helper()

	if mainXP > 0 || len(roleXP) > 0 {
		if _, err := service.GrantXP(GrantXPInput{
			PlayerID:       foundationPlayerIDForTest(playerID),
			Amount:         mainXP,
			SourceType:     XPSourceTypeQuest,
			SourceID:       XPSourceID("skill-test-xp-" + playerID),
			IdempotencyKey: XPIdempotencyKey("skill-test-xp-" + playerID),
			RoleXP:         roleXP,
		}); err != nil {
			t.Fatalf("GrantXP(%q) = %v, want nil", playerID, err)
		}
	}
	for rank := 2; rank <= targetRank; rank++ {
		if _, err := service.TryRankUp(TryRankUpInput{
			PlayerID:       foundationPlayerIDForTest(playerID),
			TargetRank:     rank,
			IdempotencyKey: XPIdempotencyKey("skill-test-rank-" + playerID + "-" + string(rune('0'+rank))),
		}); err != nil {
			t.Fatalf("TryRankUp(%q, rank %d) = %v, want nil", playerID, rank, err)
		}
	}
}

func seedSkillPointsForSkillTest(t *testing.T, store *InMemoryProgressionStore, playerID string, totalPoints int) {
	t.Helper()

	now := time.Date(2026, 6, 17, 17, 45, 0, 0, time.UTC)
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.ensurePlayerLocked(foundationPlayerIDForTest(playerID), now); err != nil {
		t.Fatalf("ensurePlayerLocked(%q) = %v, want nil", playerID, err)
	}
	state := store.skillPoints[foundationPlayerIDForTest(playerID)]
	state.TotalPoints = totalPoints
	state.UpdatedAt = now
	store.skillPoints[foundationPlayerIDForTest(playerID)] = state
}

func foundationPlayerIDForTest(playerID string) foundation.PlayerID {
	return foundation.PlayerID(playerID)
}
