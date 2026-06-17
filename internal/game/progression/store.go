package progression

import (
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

// InMemoryProgressionStore is a mutex-protected Phase 03 store suitable for
// service tests and later repository replacement.
type InMemoryProgressionStore struct {
	mu sync.Mutex

	players       map[foundation.PlayerID]PlayerProgressionState
	roleLevels    map[foundation.PlayerID]map[RoleType]RoleLevelState
	skillPoints   map[foundation.PlayerID]SkillPointState
	unlockedNodes map[foundation.PlayerID]map[SkillNodeID]UnlockedSkillNodeState

	xpSources          map[xpSourceKey]struct{}
	xpIdempotencyKeys  map[xpIdempotencyKey]struct{}
	xpGrantRecords     []XPGrantRecord
	rankUpKeys         map[rankUpIdempotencyKey]struct{}
	rankHistory        map[foundation.PlayerID][]RankHistoryEntry
	statInvalidations  map[foundation.PlayerID][]StatInvalidationSignal
	nextRankHistorySeq int64
}

type xpSourceKey struct {
	playerID   foundation.PlayerID
	sourceType XPSourceType
	sourceID   XPSourceID
}

type xpIdempotencyKey struct {
	playerID       foundation.PlayerID
	idempotencyKey XPIdempotencyKey
}

type rankUpIdempotencyKey struct {
	playerID       foundation.PlayerID
	idempotencyKey XPIdempotencyKey
}

// NewInMemoryProgressionStore returns an empty in-memory progression store.
func NewInMemoryProgressionStore() *InMemoryProgressionStore {
	return &InMemoryProgressionStore{
		players:           make(map[foundation.PlayerID]PlayerProgressionState),
		roleLevels:        make(map[foundation.PlayerID]map[RoleType]RoleLevelState),
		skillPoints:       make(map[foundation.PlayerID]SkillPointState),
		unlockedNodes:     make(map[foundation.PlayerID]map[SkillNodeID]UnlockedSkillNodeState),
		xpSources:         make(map[xpSourceKey]struct{}),
		xpIdempotencyKeys: make(map[xpIdempotencyKey]struct{}),
		rankUpKeys:        make(map[rankUpIdempotencyKey]struct{}),
		rankHistory:       make(map[foundation.PlayerID][]RankHistoryEntry),
		statInvalidations: make(map[foundation.PlayerID][]StatInvalidationSignal),
	}
}

// XPGrantRecords returns accepted XP grant records in insertion order.
func (store *InMemoryProgressionStore) XPGrantRecords(playerID foundation.PlayerID) []XPGrantRecord {
	store.mu.Lock()
	defer store.mu.Unlock()

	records := make([]XPGrantRecord, 0)
	for _, record := range store.xpGrantRecords {
		if record.PlayerID == playerID {
			records = append(records, cloneXPGrantRecord(record))
		}
	}
	return records
}

// RankHistory returns rank transition history in insertion order.
func (store *InMemoryProgressionStore) RankHistory(playerID foundation.PlayerID) []RankHistoryEntry {
	store.mu.Lock()
	defer store.mu.Unlock()

	return append([]RankHistoryEntry(nil), store.rankHistory[playerID]...)
}

// StatInvalidationSignals returns recorded progression stat invalidation signals.
func (store *InMemoryProgressionStore) StatInvalidationSignals(playerID foundation.PlayerID) []StatInvalidationSignal {
	store.mu.Lock()
	defer store.mu.Unlock()

	return append([]StatInvalidationSignal(nil), store.statInvalidations[playerID]...)
}

func (store *InMemoryProgressionStore) hasXPGrantLocked(sourceKey xpSourceKey, idempotencyKey xpIdempotencyKey) bool {
	if _, ok := store.xpSources[sourceKey]; ok {
		return true
	}
	if _, ok := store.xpIdempotencyKeys[idempotencyKey]; ok {
		return true
	}
	return false
}

func (store *InMemoryProgressionStore) applyRoleXPGrantsLocked(input GrantXPInput, roleGrants []RoleXPGrant, now time.Time) ([]RoleLevelChange, []StatInvalidationSignal, error) {
	roleLevels := store.roleLevels[input.PlayerID]
	roleLevelUps := make([]RoleLevelChange, 0)
	signals := make([]StatInvalidationSignal, 0)
	for _, grant := range roleGrants {
		if grant.Amount == 0 {
			continue
		}

		roleLevel, ok := roleLevels[grant.Role]
		if !ok {
			var err error
			roleLevel, err = NewRoleLevelState(input.PlayerID, grant.Role, 0)
			if err != nil {
				return nil, nil, err
			}
		}

		oldLevel := roleLevel.Level
		roleLevel.XP += grant.Amount
		newLevel, err := RoleLevelForXP(roleLevel.XP)
		if err != nil {
			return nil, nil, err
		}
		roleLevel.Level = newLevel
		roleLevel.UpdatedAt = now
		roleLevels[grant.Role] = roleLevel

		if newLevel > oldLevel {
			change := RoleLevelChange{
				Role:     grant.Role,
				OldLevel: oldLevel,
				NewLevel: newLevel,
			}
			roleLevelUps = append(roleLevelUps, change)
			signals = append(signals, StatInvalidationSignal{
				PlayerID:   input.PlayerID,
				Reason:     StatInvalidationReasonPlayerRoleLevelUp,
				Role:       grant.Role,
				OldLevel:   oldLevel,
				NewLevel:   newLevel,
				SourceType: input.SourceType,
				SourceID:   input.SourceID,
				CreatedAt:  now,
			})
		}
	}
	return roleLevelUps, signals, nil
}

func (store *InMemoryProgressionStore) ensurePlayerLocked(playerID foundation.PlayerID, now time.Time) error {
	if err := playerID.Validate(); err != nil {
		return err
	}
	if _, ok := store.players[playerID]; !ok {
		player, err := NewPlayerProgressionState(playerID, 0, MinRank)
		if err != nil {
			return err
		}
		player.CreatedAt = now
		player.UpdatedAt = now
		store.players[playerID] = player
	}
	if _, ok := store.roleLevels[playerID]; !ok {
		store.roleLevels[playerID] = make(map[RoleType]RoleLevelState)
	}
	if _, ok := store.skillPoints[playerID]; !ok {
		store.skillPoints[playerID] = SkillPointState{
			PlayerID:  playerID,
			UpdatedAt: now,
		}
	}
	if _, ok := store.unlockedNodes[playerID]; !ok {
		store.unlockedNodes[playerID] = make(map[SkillNodeID]UnlockedSkillNodeState)
	}
	return nil
}

func (store *InMemoryProgressionStore) snapshotLocked(playerID foundation.PlayerID) (ProgressionSnapshot, error) {
	roleLevels := make([]RoleLevelState, 0, len(store.roleLevels[playerID]))
	for _, roleLevel := range store.roleLevels[playerID] {
		roleLevels = append(roleLevels, roleLevel)
	}
	unlockedNodes := make([]UnlockedSkillNodeState, 0, len(store.unlockedNodes[playerID]))
	for _, node := range store.unlockedNodes[playerID] {
		unlockedNodes = append(unlockedNodes, node)
	}
	return NewProgressionSnapshot(
		store.players[playerID],
		roleLevels,
		store.skillPoints[playerID],
		unlockedNodes,
	)
}

func (store *InMemoryProgressionStore) appendStatInvalidationSignalsLocked(playerID foundation.PlayerID, signals []StatInvalidationSignal) {
	if len(signals) == 0 {
		return
	}
	store.statInvalidations[playerID] = append(store.statInvalidations[playerID], signals...)
}

func (store *InMemoryProgressionStore) nextRankHistoryIDLocked() RankHistoryID {
	store.nextRankHistorySeq++
	return RankHistoryID(fmt.Sprintf("rank-history-%d", store.nextRankHistorySeq))
}
