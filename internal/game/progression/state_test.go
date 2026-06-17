package progression

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestSupportedRoleTypesValidate(t *testing.T) {
	roles := SupportedRoleTypes()
	if len(roles) != 4 {
		t.Fatalf("SupportedRoleTypes len = %d, want 4", len(roles))
	}
	roles[0] = "tampered"
	if SupportedRoleTypes()[0] != RoleTypeCombat {
		t.Fatal("SupportedRoleTypes returned mutable internal slice")
	}
	for _, role := range SupportedRoleTypes() {
		if err := role.Validate(); err != nil {
			t.Fatalf("%q Validate() = %v, want nil", role, err)
		}
	}
	if err := RoleType("trading").Validate(); !errors.Is(err, ErrInvalidRoleType) {
		t.Fatalf("invalid role Validate() error = %v, want ErrInvalidRoleType", err)
	}
}

func TestPlayerProgressionStateValidationUsesXPAsLevelSourceOfTruth(t *testing.T) {
	state, err := NewPlayerProgressionState("player-1", 300, 2)
	if err != nil {
		t.Fatalf("NewPlayerProgressionState() = %v, want nil", err)
	}
	if state.MainLevel != 3 {
		t.Fatalf("MainLevel = %d, want 3", state.MainLevel)
	}

	state.MainLevel = 2
	if err := state.Validate(); !errors.Is(err, ErrLevelXPMismatch) {
		t.Fatalf("mismatched level Validate() error = %v, want ErrLevelXPMismatch", err)
	}

	state.MainLevel = 3
	state.MainXP = -1
	if err := state.Validate(); !errors.Is(err, ErrNegativeXP) {
		t.Fatalf("negative xp Validate() error = %v, want ErrNegativeXP", err)
	}

	state.MainXP = 300
	state.Rank = 0
	if err := state.Validate(); !errors.Is(err, ErrInvalidRank) {
		t.Fatalf("invalid rank Validate() error = %v, want ErrInvalidRank", err)
	}
}

func TestRoleLevelStateValidationUsesRoleXPAsLevelSourceOfTruth(t *testing.T) {
	state, err := NewRoleLevelState("player-1", RoleTypeScout, 500)
	if err != nil {
		t.Fatalf("NewRoleLevelState() = %v, want nil", err)
	}
	if state.Level != 4 {
		t.Fatalf("Level = %d, want 4", state.Level)
	}

	state.Level = 3
	if err := state.Validate(); !errors.Is(err, ErrLevelXPMismatch) {
		t.Fatalf("mismatched role level Validate() error = %v, want ErrLevelXPMismatch", err)
	}

	state.Level = 4
	state.Role = "trading"
	if err := state.Validate(); !errors.Is(err, ErrInvalidRoleType) {
		t.Fatalf("invalid role Validate() error = %v, want ErrInvalidRoleType", err)
	}
}

func TestSkillPointStateValidationAndAvailablePoints(t *testing.T) {
	state := SkillPointState{
		PlayerID:    "player-1",
		TotalPoints: 4,
		SpentPoints: 2,
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if got := state.AvailablePoints(); got != 2 {
		t.Fatalf("AvailablePoints() = %d, want 2", got)
	}

	state.SpentPoints = 5
	if err := state.Validate(); !errors.Is(err, ErrSpentSkillPointsExceedTotal) {
		t.Fatalf("overspent Validate() error = %v, want ErrSpentSkillPointsExceedTotal", err)
	}

	state.SpentPoints = -1
	if err := state.Validate(); !errors.Is(err, ErrNegativeSkillPoints) {
		t.Fatalf("negative spent Validate() error = %v, want ErrNegativeSkillPoints", err)
	}
}

func TestUnlockedSkillNodeStateValidation(t *testing.T) {
	state := UnlockedSkillNodeState{
		PlayerID:   foundation.PlayerID("player-1"),
		NodeID:     SkillNodeID("combat_damage_1"),
		UnlockedAt: time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	state.NodeID = " "
	if err := state.Validate(); !errors.Is(err, ErrEmptySkillNodeID) {
		t.Fatalf("blank node Validate() error = %v, want ErrEmptySkillNodeID", err)
	}
}
