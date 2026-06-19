package quests

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

func TestInvalidQuestStateRejected(t *testing.T) {
	if err := QuestState("started").Validate(); !errors.Is(err, ErrInvalidQuestState) {
		t.Fatalf("invalid state error = %v, want ErrInvalidQuestState", err)
	}

	quest := validPlayerQuest(t, QuestStateAccepted)
	quest.State = QuestState("started")
	if err := quest.ValidateAgainst(validObjectiveSchema(t)); !errors.Is(err, ErrInvalidQuestState) {
		t.Fatalf("player quest invalid state error = %v, want ErrInvalidQuestState", err)
	}
}

func TestInvalidObjectiveSchemasRejected(t *testing.T) {
	tests := []struct {
		name   string
		schema ObjectiveSchema
		want   error
	}{
		{
			name:   "empty schema",
			schema: ObjectiveSchema{},
			want:   ErrInvalidObjectiveKind,
		},
		{
			name: "missing objective id",
			schema: ObjectiveSchema{Objectives: []Objective{{
				Kind: ObjectiveKindKill,
				Kill: &KillObjective{TargetNPCType: "pirate", RequiredCount: qty(t, 3)},
			}}},
			want: ErrEmptyObjectiveSchema,
		},
		{
			name: "duplicate objective id",
			schema: ObjectiveSchema{Objectives: []Objective{
				{
					ID:   "kill_1",
					Kind: ObjectiveKindKill,
					Kill: &KillObjective{TargetNPCType: "pirate", RequiredCount: qty(t, 3)},
				},
				{
					ID:   "kill_1",
					Kind: ObjectiveKindKill,
					Kill: &KillObjective{TargetNPCType: "raider", RequiredCount: qty(t, 2)},
				},
			}},
			want: ErrDuplicateObjectiveID,
		},
		{
			name: "unknown kind",
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:   "travel_1",
				Kind: ObjectiveKind("travel"),
			}}},
			want: ErrInvalidObjectiveKind,
		},
		{
			name: "multiple details",
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:      "bad_1",
				Kind:    ObjectiveKindKill,
				Kill:    &KillObjective{TargetNPCType: "pirate", RequiredCount: qty(t, 3)},
				Collect: &CollectObjective{ItemID: "iron_ore", Quantity: qty(t, 5)},
			}}},
			want: ErrEmptyObjectiveSchema,
		},
		{
			name: "legacy single objective mismatched detail",
			schema: ObjectiveSchema{
				Kind:    ObjectiveKindKill,
				Collect: &CollectObjectiveDetails{ItemID: "iron_ore", RequiredQuantity: 5},
			},
			want: ErrInvalidObjectiveSchema,
		},
		{
			name: "kill missing target",
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:   "kill_1",
				Kind: ObjectiveKindKill,
				Kill: &KillObjective{RequiredCount: qty(t, 3)},
			}}},
			want: ErrEmptyObjectiveTarget,
		},
		{
			name: "collect zero quantity",
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:      "collect_1",
				Kind:    ObjectiveKindCollect,
				Collect: &CollectObjective{ItemID: "iron_ore"},
			}}},
			want: ErrInvalidObjectiveRequired,
		},
		{
			name: "craft missing target",
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:    "craft_1",
				Kind:  ObjectiveKindCraft,
				Craft: &CraftObjective{Quantity: qty(t, 1)},
			}}},
			want: ErrEmptyObjectiveTarget,
		},
		{
			name: "scan zero count",
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:   "scan_1",
				Kind: ObjectiveKindScan,
				Scan: &ScanObjective{},
			}}},
			want: ErrInvalidObjectiveRequired,
		},
		{
			name: "build missing target",
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:    "build_1",
				Kind:  ObjectiveKindBuild,
				Build: &BuildObjective{RequiredCount: qty(t, 1)},
			}}},
			want: ErrEmptyObjectiveTarget,
		},
		{
			name: "deliver missing destination",
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:      "deliver_1",
				Kind:    ObjectiveKindDeliver,
				Deliver: &DeliverObjective{ItemID: "iron_ore", Quantity: qty(t, 10)},
			}}},
			want: ErrEmptyObjectiveTarget,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.schema.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("Validate() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestMVPObjectiveSchemasValidate(t *testing.T) {
	tests := []struct {
		name   string
		kind   ObjectiveKind
		schema ObjectiveSchema
	}{
		{
			name: "kill",
			kind: ObjectiveKindKill,
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:   "kill_1",
				Kind: ObjectiveKindKill,
				Kill: &KillObjective{TargetNPCType: "pirate", RequiredCount: qty(t, 3)},
			}}},
		},
		{
			name: "collect",
			kind: ObjectiveKindCollect,
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:      "collect_1",
				Kind:    ObjectiveKindCollect,
				Collect: &CollectObjective{ItemID: "iron_ore", Quantity: qty(t, 25)},
			}}},
		},
		{
			name: "craft",
			kind: ObjectiveKindCraft,
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:    "craft_1",
				Kind:  ObjectiveKindCraft,
				Craft: &CraftObjective{RecipeID: "laser_beta_t2", Quantity: qty(t, 1)},
			}}},
		},
		{
			name: "scan",
			kind: ObjectiveKindScan,
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:   "scan_1",
				Kind: ObjectiveKindScan,
				Scan: &ScanObjective{TargetSignalType: "planet_signal", RequiredCount: qty(t, 2)},
			}}},
		},
		{
			name: "build",
			kind: ObjectiveKindBuild,
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:    "build_1",
				Kind:  ObjectiveKindBuild,
				Build: &BuildObjective{BuildingType: "extractor_t1", RequiredCount: qty(t, 1)},
			}}},
		},
		{
			name: "deliver",
			kind: ObjectiveKindDeliver,
			schema: ObjectiveSchema{Objectives: []Objective{{
				ID:      "deliver_1",
				Kind:    ObjectiveKindDeliver,
				Deliver: &DeliverObjective{ItemID: "iron_ore", Quantity: qty(t, 40), DestinationType: "station"},
			}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if len(test.schema.Objectives) != 1 {
				t.Fatalf("schema objectives count = %d, want 1", len(test.schema.Objectives))
			}
			if schemaUsesLegacySingleObjectiveFields(test.schema) {
				t.Fatalf("MVP schema uses legacy single-objective fields: %#v", test.schema)
			}
			if got := test.schema.Objectives[0].Kind; got != test.kind {
				t.Fatalf("objective kind = %q, want %q", got, test.kind)
			}
			if err := test.schema.Validate(); err != nil {
				t.Fatalf("schema Validate() = %v, want nil", err)
			}
		})
	}
}

func TestLegacySingleObjectiveSchemaShapeRemainsBackcompatOnly(t *testing.T) {
	tests := []struct {
		name         string
		schema       ObjectiveSchema
		wantProgress ObjectiveProgress
	}{
		{
			name: "kill",
			schema: ObjectiveSchema{
				Kind: ObjectiveKindKill,
				Kill: &KillObjectiveDetails{NPCType: "pirate", RequiredCount: 3},
			},
			wantProgress: ObjectiveProgress{ObjectiveID: "kill", Required: 3},
		},
		{
			name: "collect",
			schema: ObjectiveSchema{
				Kind:    ObjectiveKindCollect,
				Collect: &CollectObjectiveDetails{ItemID: "iron_ore", RequiredQuantity: 25},
			},
			wantProgress: ObjectiveProgress{ObjectiveID: "collect", Required: 25},
		},
		{
			name: "craft",
			schema: ObjectiveSchema{
				Kind:  ObjectiveKindCraft,
				Craft: &CraftObjectiveDetails{RecipeID: "laser_beta_t2", RequiredCount: 1},
			},
			wantProgress: ObjectiveProgress{ObjectiveID: "craft", Required: 1},
		},
		{
			name: "scan",
			schema: ObjectiveSchema{
				Kind: ObjectiveKindScan,
				Scan: &ScanObjectiveDetails{TargetKind: ScanTargetSignal, RequiredCount: 2},
			},
			wantProgress: ObjectiveProgress{ObjectiveID: "scan", Required: 2},
		},
		{
			name: "build",
			schema: ObjectiveSchema{
				Kind:  ObjectiveKindBuild,
				Build: &BuildObjectiveDetails{BuildingID: "extractor_t1", RequiredCount: 1},
			},
			wantProgress: ObjectiveProgress{ObjectiveID: "build", Required: 1},
		},
		{
			name: "deliver",
			schema: ObjectiveSchema{
				Kind: ObjectiveKindDeliver,
				Deliver: &DeliverObjectiveDetails{
					ItemID:           "iron_ore",
					RequiredQuantity: 40,
					DestinationKind:  DeliveryTargetStation,
				},
			},
			wantProgress: ObjectiveProgress{ObjectiveID: "deliver", Required: 40},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if len(test.schema.Objectives) != 0 {
				t.Fatalf("legacy schema objectives count = %d, want 0", len(test.schema.Objectives))
			}
			if !schemaUsesLegacySingleObjectiveFields(test.schema) {
				t.Fatalf("schema does not use legacy single-objective fields: %#v", test.schema)
			}
			if err := test.schema.Validate(); err != nil {
				t.Fatalf("legacy schema Validate() = %v, want nil", err)
			}
			progress, err := NewQuestProgressFromSchema(test.schema)
			if err != nil {
				t.Fatalf("NewQuestProgressFromSchema() = %v, want nil", err)
			}
			if len(progress.Objectives) != 1 {
				t.Fatalf("progress objectives count = %d, want 1", len(progress.Objectives))
			}
			got := progress.Objectives[0]
			if got.ObjectiveID != test.wantProgress.ObjectiveID ||
				got.Required != test.wantProgress.Required ||
				got.Current != 0 ||
				got.Completed {
				t.Fatalf("progress objective = %#v, want id %q required %d current 0 incomplete",
					got, test.wantProgress.ObjectiveID, test.wantProgress.Required)
			}
		})
	}
}

func TestRewardPayloadRejectsInvalidGrantAndHookShapes(t *testing.T) {
	validGrant := RewardGrant{
		Kind:     RewardKindCredits,
		Amount:   100,
		Currency: economy.CurrencyBucketCredits,
	}
	tests := []struct {
		name    string
		payload RewardPayload
		want    error
	}{
		{
			name: "unknown kind",
			payload: RewardPayload{Grants: []RewardGrant{{
				Kind:   RewardKind("ship"),
				Amount: 1,
			}}},
			want: ErrInvalidRewardKind,
		},
		{
			name: "zero amount",
			payload: RewardPayload{Grants: []RewardGrant{{
				Kind:     RewardKindCredits,
				Currency: economy.CurrencyBucketCredits,
			}}},
			want: ErrInvalidRewardAmount,
		},
		{
			name: "negative amount",
			payload: RewardPayload{Grants: []RewardGrant{{
				Kind:   RewardKindMainXP,
				Amount: -1,
			}}},
			want: ErrInvalidRewardAmount,
		},
		{
			name: "bad currency",
			payload: RewardPayload{Grants: []RewardGrant{{
				Kind:     RewardKindCredits,
				Amount:   100,
				Currency: economy.CurrencyBucketPremiumPaid,
			}}},
			want: ErrInvalidRewardCurrency,
		},
		{
			name: "bad item",
			payload: RewardPayload{Grants: []RewardGrant{{
				Kind:   RewardKindItem,
				Amount: 3,
			}}},
			want: ErrInvalidRewardItem,
		},
		{
			name: "bad role",
			payload: RewardPayload{Grants: []RewardGrant{{
				Kind:   RewardKindRoleXP,
				Amount: 20,
				Role:   progression.RoleType("trader"),
			}}},
			want: ErrInvalidRewardRole,
		},
		{
			name: "bad hook kind",
			payload: RewardPayload{
				Grants: []RewardGrant{validGrant},
				Hooks:  []RewardHook{{Kind: RewardHookKind("weekly_unknown"), Key: "rare"}},
			},
			want: ErrInvalidRewardHook,
		},
		{
			name: "bad hook limit",
			payload: RewardPayload{
				Grants: []RewardGrant{validGrant},
				Hooks:  []RewardHook{{Kind: RewardHookXCore}},
			},
			want: ErrInvalidRewardHook,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.payload.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("Validate() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestGeneratedOfferStoresGeneratedAndRewardPayloads(t *testing.T) {
	createdAt := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	generated := GeneratedPayload{
		Seed:         99,
		Difficulty:   2,
		TargetRegion: "frontier",
		Data:         json.RawMessage(`{"npc_type":"pirate"}`),
	}
	reward := validRewardPayload()
	template := validQuestTemplate(t)

	offer, err := NewGeneratedBoardOffer(
		"offer_1",
		"player_1",
		template,
		generated,
		reward,
		createdAt,
		createdAt.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("NewGeneratedBoardOffer() = %v, want nil", err)
	}
	generated.Objective = template.ObjectiveSchema
	var roadmapOffer QuestOffer = offer
	if !reflect.DeepEqual(roadmapOffer.GeneratedPayload, generated) {
		t.Fatalf("offer generated payload = %#v, want %#v", roadmapOffer.GeneratedPayload, generated)
	}
	if !reflect.DeepEqual(roadmapOffer.RewardPayload, reward) {
		t.Fatalf("offer reward payload = %#v, want %#v", roadmapOffer.RewardPayload, reward)
	}
}

func TestAcceptedQuestCanValidateFromGeneratedPayload(t *testing.T) {
	createdAt := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	template := validQuestTemplate(t)
	offer, err := NewGeneratedBoardOffer(
		"offer_1",
		"player_1",
		template,
		GeneratedPayload{Seed: 99},
		validRewardPayload(),
		createdAt,
		createdAt.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("NewGeneratedBoardOffer() = %v, want nil", err)
	}
	accepted, err := NewAcceptedPlayerQuest("player_quest_1", offer, ObjectiveSchema{}, createdAt.Add(time.Minute), nil)
	if err != nil {
		t.Fatalf("NewAcceptedPlayerQuest() = %v, want nil", err)
	}
	if err := accepted.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestGeneratedOfferRejectsObjectivePayloadMismatch(t *testing.T) {
	createdAt := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	mismatched := GeneratedPayload{
		Seed: 99,
		Objective: ObjectiveSchema{Objectives: []Objective{{
			ID:      "collect_1",
			Kind:    ObjectiveKindCollect,
			Collect: &CollectObjective{ItemID: "iron_ore", Quantity: qty(t, 3)},
		}}},
	}
	_, err := NewGeneratedBoardOffer(
		"offer_1",
		"player_1",
		validQuestTemplate(t),
		mismatched,
		validRewardPayload(),
		createdAt,
		createdAt.Add(24*time.Hour),
	)
	if !errors.Is(err, ErrObjectivePayloadMismatch) {
		t.Fatalf("NewGeneratedBoardOffer() error = %v, want ErrObjectivePayloadMismatch", err)
	}
}

func TestInvalidPlayerQuestStateTransitionsRejected(t *testing.T) {
	tests := []struct {
		from QuestState
		to   QuestState
	}{
		{QuestStateOffered, QuestStateClaimed},
		{QuestStateAccepted, QuestStateClaimed},
		{QuestStateCompleted, QuestStateAccepted},
		{QuestStateClaimed, QuestStateCompleted},
		{QuestStateExpired, QuestStateAccepted},
		{QuestStateAbandoned, QuestStateAccepted},
	}

	for _, test := range tests {
		t.Run(test.from.String()+"_to_"+test.to.String(), func(t *testing.T) {
			if err := test.from.ValidateTransition(test.to); !errors.Is(err, ErrInvalidQuestStateTransition) {
				t.Fatalf("ValidateTransition() error = %v, want ErrInvalidQuestStateTransition", err)
			}
		})
	}
}

func TestAcceptedCompletedClaimedTimestampsValidated(t *testing.T) {
	acceptedAt := time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC)
	completedAt := acceptedAt.Add(time.Hour)

	t.Run("accepted requires accepted_at", func(t *testing.T) {
		quest := validPlayerQuest(t, QuestStateAccepted)
		quest.AcceptedAt = time.Time{}
		if err := quest.ValidateAgainst(validObjectiveSchema(t)); !errors.Is(err, ErrZeroQuestTime) {
			t.Fatalf("ValidateAgainst() error = %v, want ErrZeroQuestTime", err)
		}
	})

	t.Run("accepted rejects completed_at", func(t *testing.T) {
		quest := validPlayerQuest(t, QuestStateAccepted)
		quest.CompletedAt = &completedAt
		if err := quest.ValidateAgainst(validObjectiveSchema(t)); !errors.Is(err, ErrInvalidQuestTime) {
			t.Fatalf("ValidateAgainst() error = %v, want ErrInvalidQuestTime", err)
		}
	})

	t.Run("completed requires completed_at", func(t *testing.T) {
		quest := validPlayerQuest(t, QuestStateCompleted)
		quest.CompletedAt = nil
		if err := quest.ValidateAgainst(validObjectiveSchema(t)); !errors.Is(err, ErrZeroQuestTime) {
			t.Fatalf("ValidateAgainst() error = %v, want ErrZeroQuestTime", err)
		}
	})

	t.Run("completed rejects timestamp before acceptance", func(t *testing.T) {
		quest := validPlayerQuest(t, QuestStateCompleted)
		beforeAccepted := acceptedAt.Add(-time.Second)
		quest.AcceptedAt = acceptedAt
		quest.CompletedAt = &beforeAccepted
		if err := quest.ValidateAgainst(validObjectiveSchema(t)); !errors.Is(err, ErrInvalidQuestTime) {
			t.Fatalf("ValidateAgainst() error = %v, want ErrInvalidQuestTime", err)
		}
	})

	t.Run("completed requires complete progress", func(t *testing.T) {
		quest := validPlayerQuest(t, QuestStateCompleted)
		quest.Progress.Objectives[0].Current = quest.Progress.Objectives[0].Required - 1
		quest.Progress.Objectives[0].Completed = false
		if err := quest.ValidateAgainst(validObjectiveSchema(t)); !errors.Is(err, ErrInvalidQuestProgress) {
			t.Fatalf("ValidateAgainst() error = %v, want ErrInvalidQuestProgress", err)
		}
	})

	t.Run("claimed requires claimed_at", func(t *testing.T) {
		quest := validPlayerQuest(t, QuestStateClaimed)
		quest.ClaimedAt = nil
		quest.RewardClaimedAt = nil
		if err := quest.ValidateAgainst(validObjectiveSchema(t)); !errors.Is(err, ErrZeroQuestTime) {
			t.Fatalf("ValidateAgainst() error = %v, want ErrZeroQuestTime", err)
		}
	})

	t.Run("claimed rejects timestamp before completion", func(t *testing.T) {
		quest := validPlayerQuest(t, QuestStateClaimed)
		beforeCompleted := completedAt.Add(-time.Second)
		quest.CompletedAt = &completedAt
		quest.ClaimedAt = &beforeCompleted
		if err := quest.ValidateAgainst(validObjectiveSchema(t)); !errors.Is(err, ErrInvalidQuestTime) {
			t.Fatalf("ValidateAgainst() error = %v, want ErrInvalidQuestTime", err)
		}
	})

	for _, state := range []QuestState{QuestStateAccepted, QuestStateCompleted, QuestStateClaimed} {
		t.Run("valid_"+state.String(), func(t *testing.T) {
			quest := validPlayerQuest(t, state)
			if err := quest.ValidateAgainst(validObjectiveSchema(t)); err != nil {
				t.Fatalf("ValidateAgainst() = %v, want nil", err)
			}
		})
	}
}

func validQuestTemplate(t *testing.T) QuestTemplate {
	t.Helper()
	return QuestTemplate{
		Source:          validQuestSource(t),
		TemplateID:      "quest_kill_pirates",
		Type:            QuestTypeKill,
		TitleKey:        "quest.kill_pirates.title",
		DescriptionKey:  "quest.kill_pirates.description",
		ObjectiveSchema: validObjectiveSchema(t),
	}
}

func validQuestSource(t *testing.T) catalog.VersionedDefinition {
	t.Helper()
	source, err := catalog.NewQuestSource("quest_kill_pirates", "v1")
	if err != nil {
		t.Fatalf("NewQuestSource() = %v", err)
	}
	return source
}

func validObjectiveSchema(t *testing.T) ObjectiveSchema {
	t.Helper()
	return ObjectiveSchema{Objectives: []Objective{{
		ID:   "kill_1",
		Kind: ObjectiveKindKill,
		Kill: &KillObjective{
			TargetNPCType: "pirate",
			RequiredCount: qty(t, 3),
		},
	}}}
}

func schemaUsesLegacySingleObjectiveFields(schema ObjectiveSchema) bool {
	return schema.Kind != "" ||
		schema.Kill != nil ||
		schema.Collect != nil ||
		schema.Craft != nil ||
		schema.Scan != nil ||
		schema.Build != nil ||
		schema.Deliver != nil
}

func validGeneratedPayload() GeneratedPayload {
	return GeneratedPayload{
		Seed:       42,
		Difficulty: 1,
	}
}

func validRewardPayload() RewardPayload {
	return RewardPayload{
		Grants: []RewardGrant{
			{
				Kind:     RewardKindCredits,
				Amount:   100,
				Currency: economy.CurrencyBucketCredits,
			},
			{
				Kind:   RewardKindItem,
				Amount: 5,
				ItemID: "iron_ore",
			},
			{
				Kind:   RewardKindMainXP,
				Amount: 25,
			},
			{
				Kind:   RewardKindRoleXP,
				Amount: 30,
				Role:   progression.RoleTypeCombat,
			},
		},
		Hooks: []RewardHook{{
			Kind:  RewardHookRareCap,
			Key:   "weekly_x_core_fragment",
			Limit: 1,
		}},
	}
}

func validPlayerQuest(t *testing.T, state QuestState) PlayerQuest {
	t.Helper()
	acceptedAt := time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC)
	completedAt := acceptedAt.Add(time.Hour)
	claimedAt := completedAt.Add(time.Minute)
	progress, err := NewQuestProgressFromSchema(validObjectiveSchema(t))
	if err != nil {
		t.Fatalf("NewQuestProgressFromSchema() = %v", err)
	}
	if state == QuestStateCompleted || state == QuestStateClaimed {
		progress.Objectives[0].Current = progress.Objectives[0].Required
		progress.Objectives[0].Completed = true
	}

	quest := PlayerQuest{
		PlayerQuestID:     foundation.QuestID("player_quest_1"),
		PlayerID:          foundation.PlayerID("player_1"),
		TemplateSource:    validQuestSource(t),
		TemplateID:        catalog.DefinitionID("quest_kill_pirates"),
		Type:              QuestTypeKill,
		GeneratedPayload:  validGeneratedPayload(),
		RewardPayload:     validRewardPayload(),
		State:             state,
		Progress:          progress,
		RewardReferenceID: RewardReferenceForPlayerQuest("player_quest_1"),
	}
	if state != QuestStateOffered {
		quest.AcceptedAt = acceptedAt
	}
	if state == QuestStateCompleted || state == QuestStateClaimed {
		quest.CompletedAt = &completedAt
	}
	if state == QuestStateClaimed {
		quest.ClaimedAt = &claimedAt
	}
	return quest
}

func qty(t *testing.T, amount int64) foundation.Quantity {
	t.Helper()
	quantity, err := foundation.NewQuantity(amount)
	if err != nil {
		t.Fatalf("NewQuantity(%d) = %v", amount, err)
	}
	return quantity
}
