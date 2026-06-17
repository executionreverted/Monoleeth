package progression

import (
	"encoding/json"
	"fmt"
	"sort"

	"gameproject/internal/game/foundation"
)

// ProgressionSnapshot is a read-only aggregate view of player progression.
type ProgressionSnapshot struct {
	Player      PlayerProgressionState `json:"player"`
	SkillPoints SkillPointState        `json:"skill_points"`

	roleLevels        []RoleLevelState
	roleLevelsByType  map[RoleType]RoleLevelState
	unlockedNodes     []UnlockedSkillNodeState
	unlockedNodeIDSet map[SkillNodeID]struct{}
}

type progressionSnapshotJSON struct {
	Player             PlayerProgressionState   `json:"player"`
	RoleLevels         []RoleLevelState         `json:"role_levels"`
	SkillPoints        SkillPointState          `json:"skill_points"`
	UnlockedSkillNodes []UnlockedSkillNodeState `json:"unlocked_skill_nodes"`
}

// NewProgressionSnapshot validates and copies all state into a stable snapshot.
func NewProgressionSnapshot(
	player PlayerProgressionState,
	roleLevels []RoleLevelState,
	skillPoints SkillPointState,
	unlockedNodes []UnlockedSkillNodeState,
) (ProgressionSnapshot, error) {
	snapshot := ProgressionSnapshot{
		Player:        player,
		SkillPoints:   skillPoints,
		roleLevels:    append([]RoleLevelState(nil), roleLevels...),
		unlockedNodes: append([]UnlockedSkillNodeState(nil), unlockedNodes...),
	}
	sortRoleLevels(snapshot.roleLevels)
	sortUnlockedNodes(snapshot.unlockedNodes)

	if err := snapshot.Validate(); err != nil {
		return ProgressionSnapshot{}, err
	}
	snapshot.rebuildLookups()
	return snapshot, nil
}

// Validate reports whether the snapshot is internally consistent.
func (snapshot ProgressionSnapshot) Validate() error {
	if err := snapshot.Player.Validate(); err != nil {
		return err
	}
	if err := snapshot.SkillPoints.Validate(); err != nil {
		return err
	}
	if err := validateSnapshotPlayer(snapshot.Player.PlayerID, snapshot.SkillPoints.PlayerID, "skill points"); err != nil {
		return err
	}

	roleLevelsByType := make(map[RoleType]RoleLevelState, len(snapshot.roleLevels))
	for _, roleLevel := range snapshot.roleLevels {
		if err := roleLevel.Validate(); err != nil {
			return err
		}
		if err := validateSnapshotPlayer(snapshot.Player.PlayerID, roleLevel.PlayerID, "role level"); err != nil {
			return err
		}
		if _, exists := roleLevelsByType[roleLevel.Role]; exists {
			return fmt.Errorf("role %q: %w", roleLevel.Role, ErrDuplicateRoleLevel)
		}
		roleLevelsByType[roleLevel.Role] = roleLevel
	}

	unlockedNodeIDSet := make(map[SkillNodeID]struct{}, len(snapshot.unlockedNodes))
	for _, node := range snapshot.unlockedNodes {
		if err := node.Validate(); err != nil {
			return err
		}
		if err := validateSnapshotPlayer(snapshot.Player.PlayerID, node.PlayerID, "skill node unlock"); err != nil {
			return err
		}
		if _, exists := unlockedNodeIDSet[node.NodeID]; exists {
			return fmt.Errorf("node %q: %w", node.NodeID, ErrDuplicateSkillNodeUnlock)
		}
		unlockedNodeIDSet[node.NodeID] = struct{}{}
	}
	return nil
}

// Clone returns a defensive copy of the snapshot.
func (snapshot ProgressionSnapshot) Clone() ProgressionSnapshot {
	cloned := ProgressionSnapshot{
		Player:            snapshot.Player,
		SkillPoints:       snapshot.SkillPoints,
		roleLevels:        append([]RoleLevelState(nil), snapshot.roleLevels...),
		roleLevelsByType:  cloneRoleLevelMap(snapshot.roleLevelsByType),
		unlockedNodes:     append([]UnlockedSkillNodeState(nil), snapshot.unlockedNodes...),
		unlockedNodeIDSet: cloneSkillNodeIDSet(snapshot.unlockedNodeIDSet),
	}
	return cloned
}

// MarshalJSON includes copied slice fields without exposing mutable internals.
func (snapshot ProgressionSnapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(progressionSnapshotJSON{
		Player:             snapshot.Player,
		RoleLevels:         snapshot.RoleLevels(),
		SkillPoints:        snapshot.SkillPoints,
		UnlockedSkillNodes: snapshot.UnlockedSkillNodes(),
	})
}

// RoleLevels returns a defensive copy of role level states.
func (snapshot ProgressionSnapshot) RoleLevels() []RoleLevelState {
	return append([]RoleLevelState(nil), snapshot.roleLevels...)
}

// RoleLevelMap returns a defensive copy keyed by role type.
func (snapshot ProgressionSnapshot) RoleLevelMap() map[RoleType]RoleLevelState {
	return cloneRoleLevelMap(snapshot.roleLevelsByType)
}

// RoleLevel returns the state for role when present.
func (snapshot ProgressionSnapshot) RoleLevel(role RoleType) (RoleLevelState, bool) {
	roleLevel, ok := snapshot.roleLevelsByType[role]
	return roleLevel, ok
}

// UnlockedSkillNodes returns a defensive copy of unlocked node states.
func (snapshot ProgressionSnapshot) UnlockedSkillNodes() []UnlockedSkillNodeState {
	return append([]UnlockedSkillNodeState(nil), snapshot.unlockedNodes...)
}

// UnlockedSkillNodeIDs returns a defensive copy of unlocked node ids.
func (snapshot ProgressionSnapshot) UnlockedSkillNodeIDs() map[SkillNodeID]struct{} {
	return cloneSkillNodeIDSet(snapshot.unlockedNodeIDSet)
}

// HasUnlockedSkillNode reports whether nodeID is unlocked in the snapshot.
func (snapshot ProgressionSnapshot) HasUnlockedSkillNode(nodeID SkillNodeID) bool {
	_, ok := snapshot.unlockedNodeIDSet[nodeID]
	return ok
}

func (snapshot *ProgressionSnapshot) rebuildLookups() {
	snapshot.roleLevelsByType = make(map[RoleType]RoleLevelState, len(snapshot.roleLevels))
	for _, roleLevel := range snapshot.roleLevels {
		snapshot.roleLevelsByType[roleLevel.Role] = roleLevel
	}
	snapshot.unlockedNodeIDSet = make(map[SkillNodeID]struct{}, len(snapshot.unlockedNodes))
	for _, node := range snapshot.unlockedNodes {
		snapshot.unlockedNodeIDSet[node.NodeID] = struct{}{}
	}
}

func validateSnapshotPlayer(want foundation.PlayerID, got foundation.PlayerID, kind string) error {
	if want != got {
		return fmt.Errorf("%s player %q want %q: %w", kind, got, want, ErrSnapshotPlayerMismatch)
	}
	return nil
}

func sortRoleLevels(roleLevels []RoleLevelState) {
	sort.Slice(roleLevels, func(i, j int) bool {
		return roleLevels[i].Role < roleLevels[j].Role
	})
}

func sortUnlockedNodes(nodes []UnlockedSkillNodeState) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NodeID < nodes[j].NodeID
	})
}

func cloneRoleLevelMap(source map[RoleType]RoleLevelState) map[RoleType]RoleLevelState {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[RoleType]RoleLevelState, len(source))
	for role, roleLevel := range source {
		cloned[role] = roleLevel
	}
	return cloned
}

func cloneSkillNodeIDSet(source map[SkillNodeID]struct{}) map[SkillNodeID]struct{} {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[SkillNodeID]struct{}, len(source))
	for nodeID := range source {
		cloned[nodeID] = struct{}{}
	}
	return cloned
}
