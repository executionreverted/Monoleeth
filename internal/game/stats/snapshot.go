package stats

import (
	"time"

	"gameproject/internal/game/foundation"
)

// SnapshotVersion changes whenever a player's effective stats are recalculated.
type SnapshotVersion uint64

// Next returns the next monotonically increasing snapshot version.
func (version SnapshotVersion) Next() SnapshotVersion {
	return version + 1
}

// InvalidationReason names the domain event that made a snapshot stale.
type InvalidationReason string

const (
	InvalidationReasonActiveShipChanged    InvalidationReason = "ship.active_changed"
	InvalidationReasonModuleEquipped       InvalidationReason = "module.equipped"
	InvalidationReasonModuleUnequipped     InvalidationReason = "module.unequipped"
	InvalidationReasonModuleBroken         InvalidationReason = "module.durability_changed_to_broken"
	InvalidationReasonPilotSkillUnlocked   InvalidationReason = "pilot.skill_unlocked"
	InvalidationReasonPilotSkillsRespecced InvalidationReason = "pilot.skills_respecced"
	InvalidationReasonPlayerRoleLevelUp    InvalidationReason = "player.role_level_up"
	InvalidationReasonPlayerRankUp         InvalidationReason = "player.rank_up"
	InvalidationReasonBuffApplied          InvalidationReason = "buff.applied"
	InvalidationReasonBuffExpired          InvalidationReason = "buff.expired"
	InvalidationReasonDebuffApplied        InvalidationReason = "debuff.applied"
	InvalidationReasonDebuffExpired        InvalidationReason = "debuff.expired"
)

// StatSnapshot is the durable, versioned effective stat view for a player ship.
type StatSnapshot struct {
	PlayerID      foundation.PlayerID `json:"player_id"`
	ShipID        foundation.ShipID   `json:"ship_id"`
	Version       SnapshotVersion     `json:"version"`
	Stats         EffectiveStats      `json:"stats"`
	CreatedAt     time.Time           `json:"created_at"`
	InvalidatedAt *time.Time          `json:"invalidated_at,omitempty"`
}

// NewStatSnapshot returns a fresh valid snapshot.
func NewStatSnapshot(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	version SnapshotVersion,
	stats EffectiveStats,
	createdAt time.Time,
) StatSnapshot {
	return StatSnapshot{
		PlayerID:  playerID,
		ShipID:    shipID,
		Version:   version,
		Stats:     stats,
		CreatedAt: createdAt,
	}
}

// IsInvalidated reports whether the snapshot has been marked stale.
func (snapshot StatSnapshot) IsInvalidated() bool {
	return snapshot.InvalidatedAt != nil
}

// Invalidate returns a copy of snapshot marked stale at invalidatedAt.
func (snapshot StatSnapshot) Invalidate(invalidatedAt time.Time) StatSnapshot {
	snapshot.InvalidatedAt = &invalidatedAt
	return snapshot
}

// InvalidationState tracks whether a player's cached stat snapshot is stale.
type InvalidationState struct {
	CurrentVersion     SnapshotVersion    `json:"current_version"`
	Invalidated        bool               `json:"invalidated"`
	InvalidatedVersion SnapshotVersion    `json:"invalidated_version,omitempty"`
	Reason             InvalidationReason `json:"reason,omitempty"`
	InvalidatedAt      *time.Time         `json:"invalidated_at,omitempty"`
	LastRecalculatedAt *time.Time         `json:"last_recalculated_at,omitempty"`
}

// NewInvalidationState returns a valid state for a calculated snapshot version.
func NewInvalidationState(currentVersion SnapshotVersion, recalculatedAt time.Time) InvalidationState {
	return InvalidationState{
		CurrentVersion:     currentVersion,
		LastRecalculatedAt: &recalculatedAt,
	}
}

// Invalidate returns a copy of state marked stale for reason.
func (state InvalidationState) Invalidate(reason InvalidationReason, invalidatedAt time.Time) InvalidationState {
	state.Invalidated = true
	state.InvalidatedVersion = state.CurrentVersion
	state.Reason = reason
	state.InvalidatedAt = &invalidatedAt
	return state
}

// MarkRecalculated returns a copy of state made valid by a new snapshot version.
func (state InvalidationState) MarkRecalculated(version SnapshotVersion, recalculatedAt time.Time) InvalidationState {
	state.CurrentVersion = version
	state.Invalidated = false
	state.InvalidatedVersion = 0
	state.Reason = ""
	state.InvalidatedAt = nil
	state.LastRecalculatedAt = &recalculatedAt
	return state
}
