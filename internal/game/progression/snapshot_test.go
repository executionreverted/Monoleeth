package progression

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestProgressionSnapshotHelpersReturnDefensiveCopies(t *testing.T) {
	now := time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)
	player, err := NewPlayerProgressionState("player-1", 20_000, 3)
	if err != nil {
		t.Fatalf("NewPlayerProgressionState() = %v, want nil", err)
	}
	skillPoints := SkillPointState{PlayerID: "player-1", TotalPoints: 3, SpentPoints: 1}
	roles := []RoleLevelState{
		{PlayerID: "player-1", Role: RoleTypeScout, Level: 2, XP: 75},
		{PlayerID: "player-1", Role: RoleTypeCombat, Level: 3, XP: 225},
	}
	nodes := []UnlockedSkillNodeState{
		{PlayerID: "player-1", NodeID: "scout_scan_1", UnlockedAt: now},
		{PlayerID: "player-1", NodeID: "combat_damage_1", UnlockedAt: now},
	}

	snapshot, err := NewProgressionSnapshot(player, roles, skillPoints, nodes)
	if err != nil {
		t.Fatalf("NewProgressionSnapshot() = %v, want nil", err)
	}

	roles[0].XP = 999
	nodes[0].NodeID = "tampered"
	if roleLevel, ok := snapshot.RoleLevel(RoleTypeScout); !ok || roleLevel.XP != 75 {
		t.Fatalf("RoleLevel(%q) = %+v, %t; want xp 75 and ok", RoleTypeScout, roleLevel, ok)
	}
	if !snapshot.HasUnlockedSkillNode("scout_scan_1") {
		t.Fatal("HasUnlockedSkillNode(scout_scan_1) = false, want true")
	}
	if snapshot.HasUnlockedSkillNode("tampered") {
		t.Fatal("snapshot picked up source node mutation")
	}

	roleLevels := snapshot.RoleLevels()
	roleLevels[0].XP = 999
	if got := snapshot.RoleLevels()[0].XP; got == 999 {
		t.Fatal("RoleLevels returned mutable internal slice")
	}

	unlockedNodes := snapshot.UnlockedSkillNodes()
	unlockedNodes[0].NodeID = "tampered"
	if snapshot.HasUnlockedSkillNode("tampered") {
		t.Fatal("UnlockedSkillNodes returned mutable internal slice")
	}

	roleMap := snapshot.RoleLevelMap()
	roleMap[RoleTypeCombat] = RoleLevelState{}
	if roleLevel, ok := snapshot.RoleLevel(RoleTypeCombat); !ok || roleLevel.XP != 225 {
		t.Fatalf("RoleLevel(%q) after map mutation = %+v, %t; want xp 225 and ok", RoleTypeCombat, roleLevel, ok)
	}

	nodeIDs := snapshot.UnlockedSkillNodeIDs()
	delete(nodeIDs, "combat_damage_1")
	if !snapshot.HasUnlockedSkillNode("combat_damage_1") {
		t.Fatal("UnlockedSkillNodeIDs returned mutable internal map")
	}

	cloned := snapshot.Clone()
	cloned.RoleLevelMap()[RoleTypeScout] = RoleLevelState{}
	if roleLevel, ok := snapshot.RoleLevel(RoleTypeScout); !ok || roleLevel.XP != 75 {
		t.Fatalf("RoleLevel(%q) after clone map mutation = %+v, %t; want xp 75 and ok", RoleTypeScout, roleLevel, ok)
	}
}

func TestProgressionSnapshotValidationRejectsInconsistentState(t *testing.T) {
	player, err := NewPlayerProgressionState("player-1", 0, 1)
	if err != nil {
		t.Fatalf("NewPlayerProgressionState() = %v, want nil", err)
	}
	skillPoints := SkillPointState{PlayerID: "player-1"}
	roleLevel := RoleLevelState{PlayerID: "player-1", Role: RoleTypeCombat, Level: 1, XP: 0}
	node := UnlockedSkillNodeState{PlayerID: "player-1", NodeID: "combat_damage_1"}

	if _, err := NewProgressionSnapshot(player, []RoleLevelState{roleLevel}, SkillPointState{PlayerID: "other-player"}, []UnlockedSkillNodeState{node}); !errors.Is(err, ErrSnapshotPlayerMismatch) {
		t.Fatalf("skill point player mismatch error = %v, want ErrSnapshotPlayerMismatch", err)
	}

	duplicateRole := roleLevel
	if _, err := NewProgressionSnapshot(player, []RoleLevelState{roleLevel, duplicateRole}, skillPoints, []UnlockedSkillNodeState{node}); !errors.Is(err, ErrDuplicateRoleLevel) {
		t.Fatalf("duplicate role error = %v, want ErrDuplicateRoleLevel", err)
	}

	duplicateNode := node
	if _, err := NewProgressionSnapshot(player, []RoleLevelState{roleLevel}, skillPoints, []UnlockedSkillNodeState{node, duplicateNode}); !errors.Is(err, ErrDuplicateSkillNodeUnlock) {
		t.Fatalf("duplicate node error = %v, want ErrDuplicateSkillNodeUnlock", err)
	}

	otherPlayerRole := roleLevel
	otherPlayerRole.PlayerID = "other-player"
	if _, err := NewProgressionSnapshot(player, []RoleLevelState{otherPlayerRole}, skillPoints, []UnlockedSkillNodeState{node}); !errors.Is(err, ErrSnapshotPlayerMismatch) {
		t.Fatalf("role player mismatch error = %v, want ErrSnapshotPlayerMismatch", err)
	}

	otherPlayerNode := node
	otherPlayerNode.PlayerID = "other-player"
	if _, err := NewProgressionSnapshot(player, []RoleLevelState{roleLevel}, skillPoints, []UnlockedSkillNodeState{otherPlayerNode}); !errors.Is(err, ErrSnapshotPlayerMismatch) {
		t.Fatalf("node player mismatch error = %v, want ErrSnapshotPlayerMismatch", err)
	}
}

func TestProgressionSnapshotMarshalJSONIncludesCopiedSlices(t *testing.T) {
	player, err := NewPlayerProgressionState("player-1", 0, 1)
	if err != nil {
		t.Fatalf("NewPlayerProgressionState() = %v, want nil", err)
	}
	snapshot, err := NewProgressionSnapshot(
		player,
		[]RoleLevelState{{PlayerID: "player-1", Role: RoleTypeCombat, Level: 1, XP: 0}},
		SkillPointState{PlayerID: "player-1"},
		[]UnlockedSkillNodeState{{PlayerID: "player-1", NodeID: "combat_damage_1"}},
	)
	if err != nil {
		t.Fatalf("NewProgressionSnapshot() = %v, want nil", err)
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("json.Marshal(snapshot) = %v, want nil", err)
	}

	var decoded struct {
		RoleLevels         []RoleLevelState         `json:"role_levels"`
		UnlockedSkillNodes []UnlockedSkillNodeState `json:"unlocked_skill_nodes"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(snapshot) = %v, want nil", err)
	}
	if len(decoded.RoleLevels) != 1 {
		t.Fatalf("decoded role levels len = %d, want 1", len(decoded.RoleLevels))
	}
	if len(decoded.UnlockedSkillNodes) != 1 {
		t.Fatalf("decoded unlocked skill nodes len = %d, want 1", len(decoded.UnlockedSkillNodes))
	}
}
