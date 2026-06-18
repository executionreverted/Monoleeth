package progression

import (
	"fmt"
	"sort"
	"strings"

	"gameproject/internal/game/foundation"
)

// ProgressionService owns Phase 03 progression mutations.
type ProgressionService struct {
	clock foundation.Clock
	store *InMemoryProgressionStore
}

// NewProgressionService returns a progression service backed by store.
func NewProgressionService(clock foundation.Clock, store *InMemoryProgressionStore) *ProgressionService {
	if clock == nil {
		clock = foundation.RealClock{}
	}
	if store == nil {
		store = NewInMemoryProgressionStore()
	}
	return &ProgressionService{
		clock: clock,
		store: store,
	}
}

// GrantXP applies main and role XP once for a player/source/idempotency tuple.
func (service *ProgressionService) GrantXP(input GrantXPInput) (GrantXPResult, error) {
	roleGrants, err := input.validate()
	if err != nil {
		return GrantXPResult{}, err
	}

	service.store.mu.Lock()
	defer service.store.mu.Unlock()

	now := service.clock.Now()
	if err := service.store.ensurePlayerLocked(input.PlayerID, now); err != nil {
		return GrantXPResult{}, err
	}

	sourceKey := xpSourceKey{
		playerID:   input.PlayerID,
		sourceType: input.SourceType,
		sourceID:   input.SourceID,
	}
	idempotencyKey := xpIdempotencyKey{
		playerID:       input.PlayerID,
		idempotencyKey: input.IdempotencyKey,
	}
	if service.store.hasXPGrantLocked(sourceKey, idempotencyKey) {
		snapshot, err := service.store.snapshotLocked(input.PlayerID)
		if err != nil {
			return GrantXPResult{}, err
		}
		return GrantXPResult{
			Snapshot:  snapshot,
			Duplicate: true,
		}, nil
	}

	player := service.store.players[input.PlayerID]
	oldMainLevel := player.MainLevel
	player.MainXP += input.Amount
	newMainLevel, err := MainLevelForXP(player.MainXP)
	if err != nil {
		return GrantXPResult{}, err
	}
	player.MainLevel = newMainLevel
	if input.Amount > 0 {
		player.UpdatedAt = now
	}
	service.store.players[input.PlayerID] = player

	var mainLevelUp *MainLevelChange
	if newMainLevel > oldMainLevel {
		mainLevelUp = &MainLevelChange{OldLevel: oldMainLevel, NewLevel: newMainLevel}
	}

	roleLevelUps, signals, err := service.store.applyRoleXPGrantsLocked(input, roleGrants, now)
	if err != nil {
		return GrantXPResult{}, err
	}

	service.store.xpSources[sourceKey] = struct{}{}
	service.store.xpIdempotencyKeys[idempotencyKey] = struct{}{}
	service.store.xpGrantRecords = append(service.store.xpGrantRecords, XPGrantRecord{
		PlayerID:       input.PlayerID,
		Amount:         input.Amount,
		SourceType:     input.SourceType,
		SourceID:       input.SourceID,
		IdempotencyKey: input.IdempotencyKey,
		Authority:      input.Authority,
		RoleXP:         cloneRoleXPGrants(roleGrants),
		GrantedAt:      now,
	})
	service.store.appendStatInvalidationSignalsLocked(input.PlayerID, signals)

	snapshot, err := service.store.snapshotLocked(input.PlayerID)
	if err != nil {
		return GrantXPResult{}, err
	}
	return GrantXPResult{
		Snapshot:                  snapshot,
		MainLevelUp:               mainLevelUp,
		RoleLevelUps:              append([]RoleLevelChange(nil), roleLevelUps...),
		StatInvalidationSignals:   append([]StatInvalidationSignal(nil), signals...),
		RecordedXPGrantSourceType: input.SourceType,
		RecordedXPGrantSourceID:   input.SourceID,
	}, nil
}

// GrantRoleXP applies role-only XP once for a player/source/idempotency tuple.
func (service *ProgressionService) GrantRoleXP(input GrantRoleXPInput) (GrantXPResult, error) {
	if err := input.validate(); err != nil {
		return GrantXPResult{}, err
	}
	return service.GrantXP(GrantXPInput{
		PlayerID:       input.PlayerID,
		Amount:         0,
		SourceType:     input.SourceType,
		SourceID:       input.SourceID,
		IdempotencyKey: input.IdempotencyKey,
		Authority:      input.Authority,
		RoleXP: []RoleXPGrant{
			{Role: input.Role, Amount: input.Amount},
		},
	})
}

// TryRankUp attempts one server-owned rank transition.
func (service *ProgressionService) TryRankUp(input TryRankUpInput) (TryRankUpResult, error) {
	if err := input.validate(); err != nil {
		return TryRankUpResult{}, err
	}

	service.store.mu.Lock()
	defer service.store.mu.Unlock()

	now := service.clock.Now()
	if err := service.store.ensurePlayerLocked(input.PlayerID, now); err != nil {
		return TryRankUpResult{}, err
	}

	key := rankUpIdempotencyKey{
		playerID:       input.PlayerID,
		idempotencyKey: input.IdempotencyKey,
	}
	if !input.IdempotencyKey.IsZero() {
		if _, ok := service.store.rankUpKeys[key]; ok {
			snapshot, err := service.store.snapshotLocked(input.PlayerID)
			if err != nil {
				return TryRankUpResult{}, err
			}
			return TryRankUpResult{
				Snapshot:  snapshot,
				Duplicate: true,
			}, nil
		}
	}

	snapshot, err := service.store.snapshotLocked(input.PlayerID)
	if err != nil {
		return TryRankUpResult{}, err
	}
	currentRank := snapshot.Player.Rank
	targetRank := input.TargetRank
	if targetRank == 0 {
		if currentRank >= MaxMVPRank {
			return TryRankUpResult{
				Snapshot:      snapshot,
				AlreadyAtRank: true,
			}, nil
		}
		targetRank = currentRank + 1
	}
	if targetRank <= currentRank {
		return TryRankUpResult{
			Snapshot:      snapshot,
			Duplicate:     true,
			AlreadyAtRank: true,
		}, nil
	}
	if targetRank != currentRank+1 {
		return TryRankUpResult{}, fmt.Errorf("target rank %d from current rank %d: %w", targetRank, currentRank, ErrInvalidRankTarget)
	}

	requirement, err := RankRequirementFor(targetRank)
	if err != nil {
		return TryRankUpResult{}, err
	}
	missing := requirement.missingFor(snapshot, service.store.rankMilestoneStateLocked(input.PlayerID))
	if len(missing) > 0 {
		return TryRankUpResult{
			Snapshot:            snapshot,
			MissingRequirements: append([]string(nil), missing...),
		}, nil
	}

	player := service.store.players[input.PlayerID]
	player.Rank = targetRank
	player.UpdatedAt = now
	service.store.players[input.PlayerID] = player

	skillPoints := service.store.skillPoints[input.PlayerID]
	skillPoints.TotalPoints++
	skillPoints.UpdatedAt = now
	service.store.skillPoints[input.PlayerID] = skillPoints

	history := RankHistoryEntry{
		ID:        service.store.nextRankHistoryIDLocked(),
		PlayerID:  input.PlayerID,
		OldRank:   currentRank,
		NewRank:   targetRank,
		Reason:    rankUpReason(input.Reason),
		CreatedAt: now,
	}
	service.store.rankHistory[input.PlayerID] = append(service.store.rankHistory[input.PlayerID], history)
	if !input.IdempotencyKey.IsZero() {
		service.store.rankUpKeys[key] = struct{}{}
	}

	signal := StatInvalidationSignal{
		PlayerID:  input.PlayerID,
		Reason:    StatInvalidationReasonPlayerRankUp,
		OldRank:   currentRank,
		NewRank:   targetRank,
		CreatedAt: now,
	}
	service.store.appendStatInvalidationSignalsLocked(input.PlayerID, []StatInvalidationSignal{signal})

	snapshot, err = service.store.snapshotLocked(input.PlayerID)
	if err != nil {
		return TryRankUpResult{}, err
	}
	return TryRankUpResult{
		Snapshot:                snapshot,
		RankedUp:                true,
		RankHistoryEntry:        cloneRankHistoryEntryPtr(history),
		SkillPointsGranted:      1,
		StatInvalidationSignals: []StatInvalidationSignal{signal},
	}, nil
}

// UnlockPilotSkill unlocks one server-owned passive pilot skill node.
func (service *ProgressionService) UnlockPilotSkill(input UnlockPilotSkillInput) (UnlockPilotSkillResult, error) {
	if err := input.validate(); err != nil {
		return UnlockPilotSkillResult{}, err
	}
	definition, err := PilotSkillDefinitionFor(input.NodeID)
	if err != nil {
		return UnlockPilotSkillResult{}, err
	}
	if err := definition.Validate(); err != nil {
		return UnlockPilotSkillResult{}, err
	}

	service.store.mu.Lock()
	defer service.store.mu.Unlock()

	now := service.clock.Now()
	if err := service.store.ensurePlayerLocked(input.PlayerID, now); err != nil {
		return UnlockPilotSkillResult{}, err
	}

	if _, ok := service.store.unlockedNodes[input.PlayerID][input.NodeID]; ok {
		snapshot, err := service.store.snapshotLocked(input.PlayerID)
		if err != nil {
			return UnlockPilotSkillResult{}, err
		}
		return UnlockPilotSkillResult{
			Snapshot:  snapshot,
			Node:      definition,
			Duplicate: true,
		}, nil
	}

	snapshot, err := service.store.snapshotLocked(input.PlayerID)
	if err != nil {
		return UnlockPilotSkillResult{}, err
	}
	if err := validatePilotSkillUnlock(snapshot, definition); err != nil {
		return UnlockPilotSkillResult{}, err
	}

	skillPoints := service.store.skillPoints[input.PlayerID]
	skillPoints.SpentPoints += definition.CostPoints
	skillPoints.UpdatedAt = now
	service.store.skillPoints[input.PlayerID] = skillPoints
	service.store.unlockedNodes[input.PlayerID][input.NodeID] = UnlockedSkillNodeState{
		PlayerID:   input.PlayerID,
		NodeID:     input.NodeID,
		UnlockedAt: now,
	}

	signal := StatInvalidationSignal{
		PlayerID:  input.PlayerID,
		Reason:    StatInvalidationReasonPilotSkillUnlocked,
		NodeID:    input.NodeID,
		CreatedAt: now,
	}
	service.store.appendStatInvalidationSignalsLocked(input.PlayerID, []StatInvalidationSignal{signal})

	snapshot, err = service.store.snapshotLocked(input.PlayerID)
	if err != nil {
		return UnlockPilotSkillResult{}, err
	}
	return UnlockPilotSkillResult{
		Snapshot:                snapshot,
		Node:                    definition,
		Unlocked:                true,
		StatInvalidationSignals: []StatInvalidationSignal{signal},
	}, nil
}

// RespecPilotSkills clears unlocked passive nodes and refunds spent points.
func (service *ProgressionService) RespecPilotSkills(input RespecPilotSkillsInput) (RespecPilotSkillsResult, error) {
	if err := input.validate(); err != nil {
		return RespecPilotSkillsResult{}, err
	}

	service.store.mu.Lock()
	defer service.store.mu.Unlock()

	now := service.clock.Now()
	if err := service.store.ensurePlayerLocked(input.PlayerID, now); err != nil {
		return RespecPilotSkillsResult{}, err
	}

	clearedNodeIDs := make([]SkillNodeID, 0, len(service.store.unlockedNodes[input.PlayerID]))
	for nodeID := range service.store.unlockedNodes[input.PlayerID] {
		clearedNodeIDs = append(clearedNodeIDs, nodeID)
	}
	sort.Slice(clearedNodeIDs, func(i, j int) bool {
		return clearedNodeIDs[i] < clearedNodeIDs[j]
	})

	skillPoints := service.store.skillPoints[input.PlayerID]
	refundedPoints := skillPoints.SpentPoints
	if refundedPoints == 0 && len(clearedNodeIDs) == 0 {
		snapshot, err := service.store.snapshotLocked(input.PlayerID)
		if err != nil {
			return RespecPilotSkillsResult{}, err
		}
		return RespecPilotSkillsResult{
			Snapshot: snapshot,
		}, nil
	}
	skillPoints.SpentPoints = 0
	skillPoints.UpdatedAt = now
	service.store.skillPoints[input.PlayerID] = skillPoints
	service.store.unlockedNodes[input.PlayerID] = make(map[SkillNodeID]UnlockedSkillNodeState)

	signal := StatInvalidationSignal{
		PlayerID:              input.PlayerID,
		Reason:                StatInvalidationReasonPilotSkillsRespecced,
		RefundedSkillPoints:   refundedPoints,
		ClearedSkillNodeCount: len(clearedNodeIDs),
		CreatedAt:             now,
	}
	service.store.appendStatInvalidationSignalsLocked(input.PlayerID, []StatInvalidationSignal{signal})

	snapshot, err := service.store.snapshotLocked(input.PlayerID)
	if err != nil {
		return RespecPilotSkillsResult{}, err
	}
	return RespecPilotSkillsResult{
		Snapshot:                snapshot,
		Respecced:               true,
		RefundedPoints:          refundedPoints,
		ClearedNodeIDs:          append([]SkillNodeID(nil), clearedNodeIDs...),
		StatInvalidationSignals: []StatInvalidationSignal{signal},
	}, nil
}

// GetProgressionSnapshot returns the current player progression snapshot.
func (service *ProgressionService) GetProgressionSnapshot(playerID foundation.PlayerID) (ProgressionSnapshot, error) {
	if err := playerID.Validate(); err != nil {
		return ProgressionSnapshot{}, err
	}

	service.store.mu.Lock()
	defer service.store.mu.Unlock()

	now := service.clock.Now()
	if err := service.store.ensurePlayerLocked(playerID, now); err != nil {
		return ProgressionSnapshot{}, err
	}
	return service.store.snapshotLocked(playerID)
}

func (input GrantXPInput) validate() ([]RoleXPGrant, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return nil, err
	}
	if input.Amount < 0 {
		return nil, fmt.Errorf("main xp amount %d: %w", input.Amount, ErrInvalidXPGrantAmount)
	}
	if err := input.SourceType.Validate(); err != nil {
		return nil, err
	}
	if err := input.Authority.ValidateForSource(input.SourceType); err != nil {
		return nil, err
	}
	if err := input.SourceID.Validate(); err != nil {
		return nil, err
	}
	if err := input.IdempotencyKey.Validate(); err != nil {
		return nil, err
	}

	totalRoleXP := int64(0)
	for _, grant := range input.RoleXP {
		if err := grant.validate(); err != nil {
			return nil, err
		}
		totalRoleXP += grant.Amount
	}
	if input.Amount == 0 && totalRoleXP == 0 {
		return nil, ErrEmptyXPGrant
	}
	roleGrants := consolidateRoleXPGrants(input.RoleXP)
	return roleGrants, nil
}

func (input GrantRoleXPInput) validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if err := input.Role.Validate(); err != nil {
		return err
	}
	if input.Amount <= 0 {
		return fmt.Errorf("role xp amount %d: %w", input.Amount, ErrInvalidXPGrantAmount)
	}
	if err := input.SourceType.Validate(); err != nil {
		return err
	}
	if err := input.Authority.ValidateForSource(input.SourceType); err != nil {
		return err
	}
	if err := input.SourceID.Validate(); err != nil {
		return err
	}
	if err := input.IdempotencyKey.Validate(); err != nil {
		return err
	}
	return nil
}

func (input TryRankUpInput) validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if input.TargetRank != 0 {
		if err := ValidateRank(input.TargetRank); err != nil {
			return err
		}
	}
	if input.IdempotencyKey.IsZero() {
		return ErrMissingRankUpIdempotencyKey
	}
	if err := input.IdempotencyKey.Validate(); err != nil {
		return err
	}
	return nil
}

func (input UnlockPilotSkillInput) validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if err := input.NodeID.Validate(); err != nil {
		return err
	}
	return nil
}

func (input RespecPilotSkillsInput) validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	return nil
}

func (grant RoleXPGrant) validate() error {
	if err := grant.Role.Validate(); err != nil {
		return err
	}
	if grant.Amount < 0 {
		return fmt.Errorf("role %q xp amount %d: %w", grant.Role, grant.Amount, ErrInvalidXPGrantAmount)
	}
	return nil
}

func validatePilotSkillUnlock(snapshot ProgressionSnapshot, definition PilotSkillDefinition) error {
	for _, prerequisite := range definition.PrerequisiteNodes {
		if !snapshot.HasUnlockedSkillNode(prerequisite) {
			return fmt.Errorf("node %q prerequisite %q: %w", definition.NodeID, prerequisite, ErrMissingSkillPrerequisite)
		}
	}
	if snapshot.Player.Rank < definition.RankRequirement {
		return fmt.Errorf("node %q rank %d requires %d: %w", definition.NodeID, snapshot.Player.Rank, definition.RankRequirement, ErrRankRequirementNotMet)
	}
	if !definition.RoleRequirement.IsZero() {
		roleLevel := MinProgressionLevel
		if state, ok := snapshot.RoleLevel(definition.RoleRequirement); ok {
			roleLevel = state.Level
		}
		if roleLevel < definition.RoleLevelRequirement {
			return fmt.Errorf("node %q role %q level %d requires %d: %w", definition.NodeID, definition.RoleRequirement, roleLevel, definition.RoleLevelRequirement, ErrRoleRequirementNotMet)
		}
	}
	if snapshot.SkillPoints.AvailablePoints() < definition.CostPoints {
		return fmt.Errorf("node %q available points %d requires %d: %w", definition.NodeID, snapshot.SkillPoints.AvailablePoints(), definition.CostPoints, ErrNotEnoughSkillPoints)
	}
	return nil
}

func consolidateRoleXPGrants(grants []RoleXPGrant) []RoleXPGrant {
	if len(grants) == 0 {
		return nil
	}

	amountsByRole := make(map[RoleType]int64, len(grants))
	for _, grant := range grants {
		amountsByRole[grant.Role] += grant.Amount
	}

	roles := make([]RoleType, 0, len(amountsByRole))
	for role := range amountsByRole {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool {
		return roles[i] < roles[j]
	})

	consolidated := make([]RoleXPGrant, 0, len(roles))
	for _, role := range roles {
		consolidated = append(consolidated, RoleXPGrant{Role: role, Amount: amountsByRole[role]})
	}
	return consolidated
}

func rankUpReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return defaultRankUpReason
	}
	return trimmed
}

func cloneRoleXPGrants(grants []RoleXPGrant) []RoleXPGrant {
	return append([]RoleXPGrant(nil), grants...)
}

func cloneXPGrantRecord(record XPGrantRecord) XPGrantRecord {
	record.RoleXP = cloneRoleXPGrants(record.RoleXP)
	return record
}

func cloneRankHistoryEntryPtr(entry RankHistoryEntry) *RankHistoryEntry {
	cloned := entry
	return &cloned
}
