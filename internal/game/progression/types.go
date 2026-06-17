package progression

import (
	"time"

	"gameproject/internal/game/foundation"
)

// RoleXPGrant describes one role-specific XP mutation inside a GrantXP call.
type RoleXPGrant struct {
	Role   RoleType `json:"role"`
	Amount int64    `json:"amount"`
}

// GrantXPInput describes one authoritative XP grant from a validated source.
type GrantXPInput struct {
	PlayerID       foundation.PlayerID `json:"player_id"`
	Amount         int64               `json:"amount"`
	SourceType     XPSourceType        `json:"source_type"`
	SourceID       XPSourceID          `json:"source_id"`
	IdempotencyKey XPIdempotencyKey    `json:"idempotency_key"`
	RoleXP         []RoleXPGrant       `json:"role_xp,omitempty"`
}

// GrantRoleXPInput describes a role-only XP grant with the same duplicate
// safety fields used by GrantXP.
type GrantRoleXPInput struct {
	PlayerID       foundation.PlayerID `json:"player_id"`
	Role           RoleType            `json:"role,omitempty"`
	Amount         int64               `json:"amount"`
	SourceType     XPSourceType        `json:"source_type"`
	SourceID       XPSourceID          `json:"source_id"`
	IdempotencyKey XPIdempotencyKey    `json:"idempotency_key"`
}

// MainLevelChange reports a deterministic main level increase caused by XP.
type MainLevelChange struct {
	OldLevel int `json:"old_level"`
	NewLevel int `json:"new_level"`
}

// RoleLevelChange reports a deterministic role level increase caused by XP.
type RoleLevelChange struct {
	Role     RoleType `json:"role"`
	OldLevel int      `json:"old_level"`
	NewLevel int      `json:"new_level"`
}

// GrantXPResult reports the current progression state after an XP grant.
type GrantXPResult struct {
	Snapshot                  ProgressionSnapshot      `json:"snapshot"`
	MainLevelUp               *MainLevelChange         `json:"main_level_up,omitempty"`
	RoleLevelUps              []RoleLevelChange        `json:"role_level_ups,omitempty"`
	StatInvalidationSignals   []StatInvalidationSignal `json:"stat_invalidation_signals,omitempty"`
	Duplicate                 bool                     `json:"duplicate"`
	RecordedXPGrantSourceType XPSourceType             `json:"recorded_xp_grant_source_type,omitempty"`
	RecordedXPGrantSourceID   XPSourceID               `json:"recorded_xp_grant_source_id,omitempty"`
}

// TryRankUpInput describes one attempt to advance a player by one rank.
type TryRankUpInput struct {
	PlayerID       foundation.PlayerID `json:"player_id"`
	TargetRank     int                 `json:"target_rank,omitempty"`
	Reason         string              `json:"reason,omitempty"`
	IdempotencyKey XPIdempotencyKey    `json:"idempotency_key,omitempty"`
}

// TryRankUpResult reports the current progression state after a rank attempt.
type TryRankUpResult struct {
	Snapshot                ProgressionSnapshot      `json:"snapshot"`
	RankedUp                bool                     `json:"ranked_up"`
	Duplicate               bool                     `json:"duplicate"`
	AlreadyAtRank           bool                     `json:"already_at_rank"`
	MissingRequirements     []string                 `json:"missing_requirements,omitempty"`
	RankHistoryEntry        *RankHistoryEntry        `json:"rank_history_entry,omitempty"`
	SkillPointsGranted      int                      `json:"skill_points_granted,omitempty"`
	StatInvalidationSignals []StatInvalidationSignal `json:"stat_invalidation_signals,omitempty"`
}

// UnlockPilotSkillInput describes one passive skill unlock attempt.
type UnlockPilotSkillInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	NodeID   SkillNodeID         `json:"node_id"`
}

// UnlockPilotSkillResult reports the current progression state after an unlock.
type UnlockPilotSkillResult struct {
	Snapshot                ProgressionSnapshot      `json:"snapshot"`
	Node                    PilotSkillDefinition     `json:"node"`
	Unlocked                bool                     `json:"unlocked"`
	Duplicate               bool                     `json:"duplicate"`
	StatInvalidationSignals []StatInvalidationSignal `json:"stat_invalidation_signals,omitempty"`
}

// RespecPilotSkillsInput describes a deterministic passive skill reset.
type RespecPilotSkillsInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
}

// RespecPilotSkillsResult reports the current progression state after respec.
type RespecPilotSkillsResult struct {
	Snapshot                ProgressionSnapshot      `json:"snapshot"`
	Respecced               bool                     `json:"respecced"`
	RefundedPoints          int                      `json:"refunded_points"`
	ClearedNodeIDs          []SkillNodeID            `json:"cleared_node_ids,omitempty"`
	StatInvalidationSignals []StatInvalidationSignal `json:"stat_invalidation_signals,omitempty"`
}

// RankHistoryID identifies one rank transition audit row.
type RankHistoryID string

// RankHistoryEntry records a server-owned rank transition.
type RankHistoryEntry struct {
	ID        RankHistoryID       `json:"id"`
	PlayerID  foundation.PlayerID `json:"player_id"`
	OldRank   int                 `json:"old_rank"`
	NewRank   int                 `json:"new_rank"`
	Reason    string              `json:"reason"`
	CreatedAt time.Time           `json:"created_at"`
}

// StatInvalidationReason names progression changes that make effective stats stale.
type StatInvalidationReason string

const (
	StatInvalidationReasonPlayerRoleLevelUp    StatInvalidationReason = "player.role_level_up"
	StatInvalidationReasonPlayerRankUp         StatInvalidationReason = "player.rank_up"
	StatInvalidationReasonPilotSkillUnlocked   StatInvalidationReason = "pilot.skill_unlocked"
	StatInvalidationReasonPilotSkillsRespecced StatInvalidationReason = "pilot.skills_respecced"
)

// StatInvalidationSignal is recorded locally by progression without wiring to
// the stats package.
type StatInvalidationSignal struct {
	PlayerID              foundation.PlayerID    `json:"player_id"`
	Reason                StatInvalidationReason `json:"reason"`
	Role                  RoleType               `json:"role,omitempty"`
	OldLevel              int                    `json:"old_level,omitempty"`
	NewLevel              int                    `json:"new_level,omitempty"`
	OldRank               int                    `json:"old_rank,omitempty"`
	NewRank               int                    `json:"new_rank,omitempty"`
	NodeID                SkillNodeID            `json:"node_id,omitempty"`
	RefundedSkillPoints   int                    `json:"refunded_skill_points,omitempty"`
	ClearedSkillNodeCount int                    `json:"cleared_skill_node_count,omitempty"`
	SourceType            XPSourceType           `json:"source_type,omitempty"`
	SourceID              XPSourceID             `json:"source_id,omitempty"`
	CreatedAt             time.Time              `json:"created_at"`
}

// XPGrantRecord records one accepted XP source for duplicate safety and audit.
type XPGrantRecord struct {
	PlayerID       foundation.PlayerID `json:"player_id"`
	Amount         int64               `json:"amount"`
	SourceType     XPSourceType        `json:"source_type"`
	SourceID       XPSourceID          `json:"source_id"`
	IdempotencyKey XPIdempotencyKey    `json:"idempotency_key"`
	RoleXP         []RoleXPGrant       `json:"role_xp,omitempty"`
	GrantedAt      time.Time           `json:"granted_at"`
}
