package progression

import (
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
)

// PlayerProgressionState models the durable main progression row for a player.
type PlayerProgressionState struct {
	PlayerID  foundation.PlayerID `json:"player_id"`
	MainLevel int                 `json:"main_level"`
	MainXP    int64               `json:"main_xp"`
	Rank      int                 `json:"rank"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// RoleLevelState models one role-specific XP track for a player.
type RoleLevelState struct {
	PlayerID  foundation.PlayerID `json:"player_id"`
	Role      RoleType            `json:"role_type"`
	Level     int                 `json:"level"`
	XP        int64               `json:"xp"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// SkillPointState records total and spent pilot passive skill points.
type SkillPointState struct {
	PlayerID    foundation.PlayerID `json:"player_id"`
	TotalPoints int                 `json:"total_points"`
	SpentPoints int                 `json:"spent_points"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// UnlockedSkillNodeState records one unlocked pilot passive skill node.
type UnlockedSkillNodeState struct {
	PlayerID   foundation.PlayerID `json:"player_id"`
	NodeID     SkillNodeID         `json:"node_id"`
	UnlockedAt time.Time           `json:"unlocked_at"`
}

// NewPlayerProgressionState validates and returns a player progression state
// with MainLevel computed from mainXP.
func NewPlayerProgressionState(playerID foundation.PlayerID, mainXP int64, rank int) (PlayerProgressionState, error) {
	level, err := MainLevelForXP(mainXP)
	if err != nil {
		return PlayerProgressionState{}, err
	}
	state := PlayerProgressionState{
		PlayerID:  playerID,
		MainLevel: level,
		MainXP:    mainXP,
		Rank:      rank,
	}
	if err := state.Validate(); err != nil {
		return PlayerProgressionState{}, err
	}
	return state, nil
}

// NewRoleLevelState validates and returns a role level state with Level
// computed from xp.
func NewRoleLevelState(playerID foundation.PlayerID, role RoleType, xp int64) (RoleLevelState, error) {
	level, err := RoleLevelForXP(xp)
	if err != nil {
		return RoleLevelState{}, err
	}
	state := RoleLevelState{
		PlayerID: playerID,
		Role:     role,
		Level:    level,
		XP:       xp,
	}
	if err := state.Validate(); err != nil {
		return RoleLevelState{}, err
	}
	return state, nil
}

// AvailablePoints returns unspent pilot passive skill points.
func (state SkillPointState) AvailablePoints() int {
	return state.TotalPoints - state.SpentPoints
}

// Validate reports whether state has a valid owner, rank, XP, and derived level.
func (state PlayerProgressionState) Validate() error {
	if err := state.PlayerID.Validate(); err != nil {
		return err
	}
	if err := ValidateMainLevel(state.MainLevel); err != nil {
		return err
	}
	if state.MainXP < 0 {
		return fmt.Errorf("main xp %d: %w", state.MainXP, ErrNegativeXP)
	}
	level, err := MainLevelForXP(state.MainXP)
	if err != nil {
		return err
	}
	if state.MainLevel != level {
		return fmt.Errorf("main level %d for xp %d want %d: %w", state.MainLevel, state.MainXP, level, ErrLevelXPMismatch)
	}
	if err := ValidateRank(state.Rank); err != nil {
		return err
	}
	return nil
}

// Validate reports whether state has a valid owner, role, XP, and derived level.
func (state RoleLevelState) Validate() error {
	if err := state.PlayerID.Validate(); err != nil {
		return err
	}
	if err := state.Role.Validate(); err != nil {
		return err
	}
	if err := ValidateRoleLevel(state.Level); err != nil {
		return err
	}
	if state.XP < 0 {
		return fmt.Errorf("role xp %d: %w", state.XP, ErrNegativeXP)
	}
	level, err := RoleLevelForXP(state.XP)
	if err != nil {
		return err
	}
	if state.Level != level {
		return fmt.Errorf("role level %d for xp %d want %d: %w", state.Level, state.XP, level, ErrLevelXPMismatch)
	}
	return nil
}

// Validate reports whether state has valid point counters for one player.
func (state SkillPointState) Validate() error {
	if err := state.PlayerID.Validate(); err != nil {
		return err
	}
	if state.TotalPoints < 0 {
		return fmt.Errorf("total skill points %d: %w", state.TotalPoints, ErrNegativeSkillPoints)
	}
	if state.SpentPoints < 0 {
		return fmt.Errorf("spent skill points %d: %w", state.SpentPoints, ErrNegativeSkillPoints)
	}
	if state.SpentPoints > state.TotalPoints {
		return fmt.Errorf("spent %d total %d: %w", state.SpentPoints, state.TotalPoints, ErrSpentSkillPointsExceedTotal)
	}
	return nil
}

// Validate reports whether state has a valid owner and node id.
func (state UnlockedSkillNodeState) Validate() error {
	if err := state.PlayerID.Validate(); err != nil {
		return err
	}
	if err := state.NodeID.Validate(); err != nil {
		return err
	}
	return nil
}
