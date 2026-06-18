package progression

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/testutil"
)

func TestGrantXPAppliesMainAndRoleXPOncePerSource(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 14, 0, 0, 0, time.UTC))
	store := NewInMemoryProgressionStore()
	service := NewProgressionService(clock, store)

	input := GrantXPInput{
		PlayerID:       "player-1",
		Amount:         100,
		SourceType:     XPSourceTypeQuest,
		SourceID:       "quest-reward-1",
		IdempotencyKey: "xp-quest-reward-1",
		Authority:      XPGrantAuthorityQuestService,
		RoleXP: []RoleXPGrant{
			{Role: RoleTypeCombat, Amount: 75},
		},
	}
	result, err := service.GrantXP(input)
	if err != nil {
		t.Fatalf("GrantXP() = %v, want nil", err)
	}
	if result.Duplicate {
		t.Fatal("first GrantXP Duplicate = true, want false")
	}
	if result.Snapshot.Player.MainXP != 100 || result.Snapshot.Player.MainLevel != 2 {
		t.Fatalf("player after GrantXP = xp %d level %d, want xp 100 level 2", result.Snapshot.Player.MainXP, result.Snapshot.Player.MainLevel)
	}
	if result.MainLevelUp == nil || result.MainLevelUp.OldLevel != 1 || result.MainLevelUp.NewLevel != 2 {
		t.Fatalf("MainLevelUp = %+v, want 1 -> 2", result.MainLevelUp)
	}
	roleLevel, ok := result.Snapshot.RoleLevel(RoleTypeCombat)
	if !ok {
		t.Fatal("combat role level missing after role XP grant")
	}
	if roleLevel.XP != 75 || roleLevel.Level != 2 {
		t.Fatalf("combat role after GrantXP = xp %d level %d, want xp 75 level 2", roleLevel.XP, roleLevel.Level)
	}
	if len(result.RoleLevelUps) != 1 || result.RoleLevelUps[0].Role != RoleTypeCombat {
		t.Fatalf("RoleLevelUps = %+v, want one combat role level up", result.RoleLevelUps)
	}
	assertSignalReasons(t, result.StatInvalidationSignals, []StatInvalidationReason{StatInvalidationReasonPlayerRoleLevelUp})

	duplicate := input
	duplicate.Amount = 1_000
	duplicate.RoleXP = []RoleXPGrant{{Role: RoleTypeCombat, Amount: 1_000}}
	duplicateResult, err := service.GrantXP(duplicate)
	if err != nil {
		t.Fatalf("duplicate GrantXP() = %v, want nil", err)
	}
	if !duplicateResult.Duplicate {
		t.Fatal("duplicate GrantXP Duplicate = false, want true")
	}
	if duplicateResult.Snapshot.Player.MainXP != 100 || duplicateResult.Snapshot.Player.MainLevel != 2 {
		t.Fatalf("player after duplicate GrantXP = xp %d level %d, want xp 100 level 2", duplicateResult.Snapshot.Player.MainXP, duplicateResult.Snapshot.Player.MainLevel)
	}
	duplicateRole, ok := duplicateResult.Snapshot.RoleLevel(RoleTypeCombat)
	if !ok {
		t.Fatal("combat role level missing after duplicate")
	}
	if duplicateRole.XP != 75 || duplicateRole.Level != 2 {
		t.Fatalf("combat role after duplicate = xp %d level %d, want xp 75 level 2", duplicateRole.XP, duplicateRole.Level)
	}
	if len(duplicateResult.StatInvalidationSignals) != 0 {
		t.Fatalf("duplicate StatInvalidationSignals = %+v, want none", duplicateResult.StatInvalidationSignals)
	}
	if got := len(store.XPGrantRecords("player-1")); got != 1 {
		t.Fatalf("XPGrantRecords len = %d, want 1", got)
	}
	assertSignalReasons(t, store.StatInvalidationSignals("player-1"), []StatInvalidationReason{StatInvalidationReasonPlayerRoleLevelUp})

	idempotentRetry := input
	idempotentRetry.SourceID = "quest-reward-2"
	idempotentRetry.Amount = 500
	idempotentRetryResult, err := service.GrantXP(idempotentRetry)
	if err != nil {
		t.Fatalf("idempotency retry GrantXP() = %v, want nil", err)
	}
	if !idempotentRetryResult.Duplicate {
		t.Fatal("idempotency retry Duplicate = false, want true")
	}
	if idempotentRetryResult.Snapshot.Player.MainXP != 100 {
		t.Fatalf("player xp after idempotency retry = %d, want 100", idempotentRetryResult.Snapshot.Player.MainXP)
	}
	if got := len(store.XPGrantRecords("player-1")); got != 1 {
		t.Fatalf("XPGrantRecords len after idempotency retry = %d, want 1", got)
	}
}

func TestGrantRoleXPUsesExplicitRoleInputAndInvalidatesStats(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 14, 30, 0, 0, time.UTC))
	service := NewProgressionService(clock, nil)

	result, err := service.GrantRoleXP(GrantRoleXPInput{
		PlayerID:       "player-1",
		Role:           RoleTypeScout,
		Amount:         75,
		SourceType:     XPSourceTypeScan,
		SourceID:       "scan-pulse-1",
		IdempotencyKey: "xp-scan-pulse-1",
		Authority:      XPGrantAuthorityScannerService,
	})
	if err != nil {
		t.Fatalf("GrantRoleXP() = %v, want nil", err)
	}
	if result.Snapshot.Player.MainXP != 0 || result.Snapshot.Player.MainLevel != 1 {
		t.Fatalf("main progression after GrantRoleXP = xp %d level %d, want xp 0 level 1", result.Snapshot.Player.MainXP, result.Snapshot.Player.MainLevel)
	}
	roleLevel, ok := result.Snapshot.RoleLevel(RoleTypeScout)
	if !ok {
		t.Fatal("scout role level missing")
	}
	if roleLevel.XP != 75 || roleLevel.Level != 2 {
		t.Fatalf("scout role after GrantRoleXP = xp %d level %d, want xp 75 level 2", roleLevel.XP, roleLevel.Level)
	}
	if result.MainLevelUp != nil {
		t.Fatalf("MainLevelUp = %+v, want nil for role-only grant", result.MainLevelUp)
	}
	assertSignalReasons(t, result.StatInvalidationSignals, []StatInvalidationReason{StatInvalidationReasonPlayerRoleLevelUp})
}

func TestTryRankUpGrantsHistorySkillPointAndInvalidationOnce(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 15, 0, 0, 0, time.UTC))
	store := NewInMemoryProgressionStore()
	service := NewProgressionService(clock, store)

	_, err := service.GrantXP(GrantXPInput{
		PlayerID:       "player-1",
		Amount:         100,
		SourceType:     XPSourceTypeCombat,
		SourceID:       "npc-kill-1",
		IdempotencyKey: "xp-npc-kill-1",
		Authority:      XPGrantAuthorityCombatService,
	})
	if err != nil {
		t.Fatalf("GrantXP() = %v, want nil", err)
	}

	input := TryRankUpInput{
		PlayerID:       "player-1",
		TargetRank:     2,
		Reason:         "main_level_2",
		IdempotencyKey: "rank-up-player-1-rank-2",
	}
	result, err := service.TryRankUp(input)
	if err != nil {
		t.Fatalf("TryRankUp() = %v, want nil", err)
	}
	if !result.RankedUp {
		t.Fatal("RankedUp = false, want true")
	}
	if result.Snapshot.Player.Rank != 2 {
		t.Fatalf("rank after TryRankUp = %d, want 2", result.Snapshot.Player.Rank)
	}
	if result.Snapshot.SkillPoints.TotalPoints != 1 || result.Snapshot.SkillPoints.AvailablePoints() != 1 {
		t.Fatalf("skill points after TryRankUp = total %d available %d, want total 1 available 1", result.Snapshot.SkillPoints.TotalPoints, result.Snapshot.SkillPoints.AvailablePoints())
	}
	if result.RankHistoryEntry == nil || result.RankHistoryEntry.OldRank != 1 || result.RankHistoryEntry.NewRank != 2 {
		t.Fatalf("RankHistoryEntry = %+v, want 1 -> 2", result.RankHistoryEntry)
	}
	assertSignalReasons(t, result.StatInvalidationSignals, []StatInvalidationReason{StatInvalidationReasonPlayerRankUp})

	duplicateRankUp, err := service.TryRankUp(input)
	if err != nil {
		t.Fatalf("duplicate TryRankUp() = %v, want nil", err)
	}
	if !duplicateRankUp.Duplicate {
		t.Fatal("duplicate TryRankUp Duplicate = false, want true")
	}
	if duplicateRankUp.Snapshot.Player.Rank != 2 || duplicateRankUp.Snapshot.SkillPoints.TotalPoints != 1 {
		t.Fatalf("state after duplicate TryRankUp = rank %d skill points %d, want rank 2 skill points 1", duplicateRankUp.Snapshot.Player.Rank, duplicateRankUp.Snapshot.SkillPoints.TotalPoints)
	}

	staleTargetInput := input
	staleTargetInput.IdempotencyKey = ""
	if _, err := service.TryRankUp(staleTargetInput); !errors.Is(err, ErrMissingRankUpIdempotencyKey) {
		t.Fatalf("missing idempotency TryRankUp() error = %v, want ErrMissingRankUpIdempotencyKey", err)
	}
	if got := len(store.RankHistory("player-1")); got != 1 {
		t.Fatalf("RankHistory len = %d, want 1", got)
	}

	duplicateGrant, err := service.GrantXP(GrantXPInput{
		PlayerID:       "player-1",
		Amount:         10_000,
		SourceType:     XPSourceTypeCombat,
		SourceID:       "npc-kill-1",
		IdempotencyKey: "xp-npc-kill-1-different-key",
		Authority:      XPGrantAuthorityCombatService,
	})
	if err != nil {
		t.Fatalf("duplicate source GrantXP() = %v, want nil", err)
	}
	if !duplicateGrant.Duplicate {
		t.Fatal("duplicate source GrantXP Duplicate = false, want true")
	}
	if duplicateGrant.Snapshot.Player.Rank != 2 || duplicateGrant.Snapshot.SkillPoints.TotalPoints != 1 {
		t.Fatalf("current state from duplicate source GrantXP = rank %d skill points %d, want rank 2 skill points 1", duplicateGrant.Snapshot.Player.Rank, duplicateGrant.Snapshot.SkillPoints.TotalPoints)
	}
	if got := len(store.RankHistory("player-1")); got != 1 {
		t.Fatalf("RankHistory len after duplicate source GrantXP = %d, want 1", got)
	}
	assertSignalReasons(t, store.StatInvalidationSignals("player-1"), []StatInvalidationReason{StatInvalidationReasonPlayerRankUp})
}

func TestTryRankUpReportsMissingRequirementsWithoutMutation(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 16, 0, 0, 0, time.UTC))
	store := NewInMemoryProgressionStore()
	service := NewProgressionService(clock, store)

	result, err := service.TryRankUp(TryRankUpInput{
		PlayerID:       "player-1",
		TargetRank:     2,
		IdempotencyKey: "rank-up-player-1-rank-2",
	})
	if err != nil {
		t.Fatalf("TryRankUp() = %v, want nil", err)
	}
	if result.RankedUp {
		t.Fatal("RankedUp = true, want false")
	}
	if !reflect.DeepEqual(result.MissingRequirements, []string{"main_level:2"}) {
		t.Fatalf("MissingRequirements = %+v, want [main_level:2]", result.MissingRequirements)
	}
	if result.Snapshot.Player.Rank != 1 {
		t.Fatalf("rank after missing requirements = %d, want 1", result.Snapshot.Player.Rank)
	}
	if result.Snapshot.SkillPoints.TotalPoints != 0 {
		t.Fatalf("skill points after missing requirements = %d, want 0", result.Snapshot.SkillPoints.TotalPoints)
	}
	if got := len(store.RankHistory("player-1")); got != 0 {
		t.Fatalf("RankHistory len = %d, want 0", got)
	}
	if got := len(store.StatInvalidationSignals("player-1")); got != 0 {
		t.Fatalf("StatInvalidationSignals len = %d, want 0", got)
	}
}

func TestRespecPilotSkillsNoopDoesNotInvalidateStats(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 16, 30, 0, 0, time.UTC))
	store := NewInMemoryProgressionStore()
	service := NewProgressionService(clock, store)

	result, err := service.RespecPilotSkills(RespecPilotSkillsInput{
		PlayerID: "player-1",
	})
	if err != nil {
		t.Fatalf("RespecPilotSkills() = %v, want nil", err)
	}
	if result.Respecced {
		t.Fatal("Respecced = true, want false")
	}
	if result.RefundedPoints != 0 || len(result.ClearedNodeIDs) != 0 {
		t.Fatalf("noop respec refunded/cleared = %d/%+v, want zero/nil", result.RefundedPoints, result.ClearedNodeIDs)
	}
	if len(result.StatInvalidationSignals) != 0 {
		t.Fatalf("noop respec invalidations = %+v, want none", result.StatInvalidationSignals)
	}
	if got := len(store.StatInvalidationSignals("player-1")); got != 0 {
		t.Fatalf("stored invalidations len = %d, want 0", got)
	}
}

func TestGrantXPValidationRejectsBadSourceAndAmounts(t *testing.T) {
	service := NewProgressionService(nil, nil)

	if _, err := service.GrantXP(GrantXPInput{
		PlayerID:       "player-1",
		Amount:         -1,
		SourceType:     XPSourceTypeQuest,
		SourceID:       "quest-1",
		IdempotencyKey: "xp-quest-1",
		Authority:      XPGrantAuthorityQuestService,
	}); !errors.Is(err, ErrInvalidXPGrantAmount) {
		t.Fatalf("negative GrantXP error = %v, want ErrInvalidXPGrantAmount", err)
	}
	if _, err := service.GrantXP(GrantXPInput{
		PlayerID:       "player-1",
		Amount:         1,
		SourceType:     "trading",
		SourceID:       "trade-1",
		IdempotencyKey: "xp-trade-1",
		Authority:      XPGrantAuthorityQuestService,
	}); !errors.Is(err, ErrInvalidXPSourceType) {
		t.Fatalf("invalid source GrantXP error = %v, want ErrInvalidXPSourceType", err)
	}
	if _, err := service.GrantXP(GrantXPInput{
		PlayerID:       "player-1",
		SourceType:     XPSourceTypeQuest,
		SourceID:       "quest-1",
		IdempotencyKey: "xp-quest-empty",
		Authority:      XPGrantAuthorityQuestService,
	}); !errors.Is(err, ErrEmptyXPGrant) {
		t.Fatalf("empty GrantXP error = %v, want ErrEmptyXPGrant", err)
	}
}

func TestGrantXPRequiresMatchingServerAuthority(t *testing.T) {
	service := NewProgressionService(testutil.NewFakeClock(time.Date(2026, 6, 17, 19, 0, 0, 0, time.UTC)), nil)

	if _, err := service.GrantXP(GrantXPInput{
		PlayerID:       "player-1",
		Amount:         100,
		SourceType:     XPSourceTypeQuest,
		SourceID:       "quest-1",
		IdempotencyKey: "quest_reward:quest-1",
	}); !errors.Is(err, ErrInvalidXPGrantAuthority) {
		t.Fatalf("missing authority GrantXP error = %v, want ErrInvalidXPGrantAuthority", err)
	}
	if records := service.store.XPGrantRecords("player-1"); len(records) != 0 {
		t.Fatalf("records after missing authority = %+v, want none", records)
	}

	if _, err := service.GrantXP(GrantXPInput{
		PlayerID:       "player-1",
		Amount:         100,
		SourceType:     XPSourceTypeQuest,
		SourceID:       "quest-1",
		IdempotencyKey: "quest_reward:quest-1",
		Authority:      XPGrantAuthorityLootService,
	}); !errors.Is(err, ErrUnauthorizedXPSource) {
		t.Fatalf("wrong authority GrantXP error = %v, want ErrUnauthorizedXPSource", err)
	}
	if records := service.store.XPGrantRecords("player-1"); len(records) != 0 {
		t.Fatalf("records after wrong authority = %+v, want none", records)
	}

	result, err := service.GrantXP(GrantXPInput{
		PlayerID:       "player-1",
		Amount:         100,
		SourceType:     XPSourceTypeQuest,
		SourceID:       "quest-1",
		IdempotencyKey: "quest_reward:quest-1",
		Authority:      XPGrantAuthorityQuestService,
	})
	if err != nil {
		t.Fatalf("authorized GrantXP error = %v, want nil", err)
	}
	if result.Snapshot.Player.MainXP != 100 {
		t.Fatalf("authorized GrantXP main XP = %d, want 100", result.Snapshot.Player.MainXP)
	}
	records := service.store.XPGrantRecords("player-1")
	if len(records) != 1 || records[0].Authority != XPGrantAuthorityQuestService {
		t.Fatalf("records after authorized GrantXP = %+v, want one quest-authorized record", records)
	}
}

func TestSupportedXPSourceTypesHaveRequiredAuthorities(t *testing.T) {
	want := map[XPSourceType]XPGrantAuthority{
		XPSourceTypeCombat:          XPGrantAuthorityCombatService,
		XPSourceTypeQuest:           XPGrantAuthorityQuestService,
		XPSourceTypeLoot:            XPGrantAuthorityLootService,
		XPSourceTypeScan:            XPGrantAuthorityScannerService,
		XPSourceTypeCraft:           XPGrantAuthorityCraftingService,
		XPSourceTypeConstruction:    XPGrantAuthorityProductionService,
		XPSourceTypeRoute:           XPGrantAuthorityRouteService,
		XPSourceTypeEvent:           XPGrantAuthorityEventService,
		XPSourceTypeAdminAdjustment: XPGrantAuthorityAdminService,
	}

	for _, sourceType := range SupportedXPSourceTypes() {
		required, err := RequiredXPGrantAuthorityForSource(sourceType)
		if err != nil {
			t.Fatalf("RequiredXPGrantAuthorityForSource(%q) error = %v", sourceType, err)
		}
		if required != want[sourceType] {
			t.Fatalf("RequiredXPGrantAuthorityForSource(%q) = %q, want %q", sourceType, required, want[sourceType])
		}
		if err := required.ValidateForSource(sourceType); err != nil {
			t.Fatalf("ValidateForSource(%q, %q) error = %v", required, sourceType, err)
		}
	}
}

func assertSignalReasons(t *testing.T, signals []StatInvalidationSignal, want []StatInvalidationReason) {
	t.Helper()

	got := make([]StatInvalidationReason, 0, len(signals))
	for _, signal := range signals {
		got = append(got, signal.Reason)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("signal reasons = %+v, want %+v", got, want)
	}
}
