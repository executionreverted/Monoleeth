package content

import (
	"encoding/json"
	"errors"
	"testing"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/quests"
)

func TestQuestContentRowsValidateTemplateAndRewardTable(t *testing.T) {
	err := ValidateQuestContentRows(
		[]SnapshotRow{validQuestTemplateSnapshotRow(t)},
		[]SnapshotRow{validQuestRewardTableSnapshotRow(t)},
		validQuestReferenceResolver(),
	)

	if err != nil {
		t.Fatalf("ValidateQuestContentRows() error = %v, want nil", err)
	}
}

func TestQuestTemplateRowsRejectUnknownObjectiveReferences(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*QuestTemplateRow)
		wantErr error
	}{
		{
			name: "item",
			mutate: func(row *QuestTemplateRow) {
				row.ObjectiveSchema = collectQuestObjectiveRow("collect_1", "missing_item", 2)
			},
			wantErr: ErrUnknownContentItem,
		},
		{
			name: "recipe",
			mutate: func(row *QuestTemplateRow) {
				row.Type = quests.QuestTypeCraft
				row.ObjectiveSchema = craftQuestObjectiveRow("craft_1", "missing_recipe", "", 1)
			},
			wantErr: ErrUnknownContentRecipe,
		},
		{
			name: "npc",
			mutate: func(row *QuestTemplateRow) {
				row.Type = quests.QuestTypeKill
				row.ObjectiveSchema = killQuestObjectiveRow("kill_1", "missing_npc", 1)
			},
			wantErr: ErrUnknownContentNPC,
		},
		{
			name: "building",
			mutate: func(row *QuestTemplateRow) {
				row.Type = quests.QuestTypeBuild
				row.ObjectiveSchema = buildQuestObjectiveRow("build_1", "missing_building", 1)
			},
			wantErr: ErrUnknownContentBuilding,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := validQuestTemplateRow(t)
			tt.mutate(&data)

			err := ValidateQuestTemplateRows(
				[]SnapshotRow{questTemplateSnapshotRow(t, data.TemplateID, data)},
				validQuestReferenceResolver(),
			)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateQuestTemplateRows() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestQuestRowsRejectDuplicateTemplateAndRewardTableIDs(t *testing.T) {
	t.Run("template", func(t *testing.T) {
		row := validQuestTemplateSnapshotRow(t)

		err := ValidateQuestTemplateRows([]SnapshotRow{row, row}, validQuestReferenceResolver())

		if !errors.Is(err, ErrDuplicateContentID) {
			t.Fatalf("ValidateQuestTemplateRows() error = %v, want %v", err, ErrDuplicateContentID)
		}
	})

	t.Run("reward table", func(t *testing.T) {
		row := validQuestRewardTableSnapshotRow(t)

		err := ValidateQuestRewardTableRows([]SnapshotRow{row, row}, validQuestReferenceResolver())

		if !errors.Is(err, ErrDuplicateContentID) {
			t.Fatalf("ValidateQuestRewardTableRows() error = %v, want %v", err, ErrDuplicateContentID)
		}
	})
}

func TestQuestRewardTableRowsAllowMultipleTablesForOneTemplate(t *testing.T) {
	first := validQuestRewardTableRow(t)
	second := validQuestRewardTableRow(t)
	second.RewardTableID = "quest_rewards_collect_iron_ore_r1_rare"
	second.Source = mustQuestContentSource(t, second.RewardTableID)
	second.Weight = 10

	err := ValidateQuestRewardTableRows(
		[]SnapshotRow{
			questRewardTableSnapshotRow(t, first.RewardTableID, first),
			questRewardTableSnapshotRow(t, second.RewardTableID, second),
		},
		validQuestReferenceResolver(),
	)

	if err != nil {
		t.Fatalf("ValidateQuestRewardTableRows() error = %v, want nil", err)
	}
}

func TestQuestRowsRejectInvalidRewardPayloadAndRankGate(t *testing.T) {
	t.Run("reward payload", func(t *testing.T) {
		data := validQuestRewardTableRow(t)
		data.RewardPayload.Grants[0].Amount = 0

		err := ValidateQuestRewardTableRows(
			[]SnapshotRow{questRewardTableSnapshotRow(t, data.RewardTableID, data)},
			validQuestReferenceResolver(),
		)

		if !errors.Is(err, quests.ErrInvalidRewardAmount) {
			t.Fatalf("ValidateQuestRewardTableRows() error = %v, want %v", err, quests.ErrInvalidRewardAmount)
		}
	})

	t.Run("rank gate", func(t *testing.T) {
		data := validQuestTemplateRow(t)
		data.Requirements = []quests.QuestRequirement{{MinRank: progression.MaxMVPRank + 1}}

		err := ValidateQuestTemplateRows(
			[]SnapshotRow{questTemplateSnapshotRow(t, data.TemplateID, data)},
			validQuestReferenceResolver(),
		)

		if !errors.Is(err, quests.ErrInvalidQuestRequirement) {
			t.Fatalf("ValidateQuestTemplateRows() error = %v, want %v", err, quests.ErrInvalidQuestRequirement)
		}
	})
}

func validQuestTemplateSnapshotRow(t *testing.T) SnapshotRow {
	t.Helper()
	data := validQuestTemplateRow(t)
	return questTemplateSnapshotRow(t, data.TemplateID, data)
}

func validQuestRewardTableSnapshotRow(t *testing.T) SnapshotRow {
	t.Helper()
	data := validQuestRewardTableRow(t)
	return questRewardTableSnapshotRow(t, data.RewardTableID, data)
}

func questTemplateSnapshotRow(t *testing.T, templateID catalog.DefinitionID, data QuestTemplateRow) SnapshotRow {
	t.Helper()
	return SnapshotRow{
		ContentID: ContentID(templateID.String()),
		Enabled:   true,
		DataJSON:  mustQuestContentJSON(t, data),
	}
}

func questRewardTableSnapshotRow(t *testing.T, tableID catalog.DefinitionID, data QuestRewardTableRow) SnapshotRow {
	t.Helper()
	return SnapshotRow{
		ContentID: ContentID(tableID.String()),
		Enabled:   true,
		DataJSON:  mustQuestContentJSON(t, data),
	}
}

func validQuestTemplateRow(t *testing.T) QuestTemplateRow {
	t.Helper()
	templateID := catalog.DefinitionID("quest_collect_iron_ore_r1")
	return QuestTemplateRow{
		Source:          mustQuestContentSource(t, templateID),
		TemplateID:      templateID,
		Type:            quests.QuestTypeCollect,
		TitleKey:        "quest.collect_iron_ore.title",
		DescriptionKey:  "quest.collect_iron_ore.description",
		ObjectiveSchema: collectQuestObjectiveRow("collect_1", "iron_ore", 2),
		Requirements:    []quests.QuestRequirement{{MinRank: 1, MaxRank: 3}},
		BoardWeight:     100,
	}
}

func validQuestRewardTableRow(t *testing.T) QuestRewardTableRow {
	t.Helper()
	tableID := catalog.DefinitionID("quest_rewards_collect_iron_ore_r1")
	return QuestRewardTableRow{
		Source:        mustQuestContentSource(t, tableID),
		RewardTableID: tableID,
		TemplateID:    "quest_collect_iron_ore_r1",
		RewardPayload: quests.RewardPayload{Grants: []quests.RewardGrant{
			{
				Kind:     quests.RewardKindCredits,
				Currency: economy.CurrencyBucketCredits,
				Amount:   100,
			},
			{
				Kind:   quests.RewardKindItem,
				ItemID: "iron_ore",
				Amount: 2,
			},
		}},
		Weight:      100,
		Probability: 1,
	}
}

func collectQuestObjectiveRow(id string, itemID foundation.ItemID, amount int64) QuestObjectiveSchemaRow {
	return QuestObjectiveSchemaRow{Objectives: []QuestObjectiveRow{{
		ID:   id,
		Kind: quests.ObjectiveKindCollect,
		Collect: &QuestCollectObjectiveRow{
			ItemID:   itemID,
			Quantity: amount,
		},
	}}}
}

func craftQuestObjectiveRow(id string, recipeID catalog.DefinitionID, itemID foundation.ItemID, amount int64) QuestObjectiveSchemaRow {
	return QuestObjectiveSchemaRow{Objectives: []QuestObjectiveRow{{
		ID:   id,
		Kind: quests.ObjectiveKindCraft,
		Craft: &QuestCraftObjectiveRow{
			RecipeID: recipeID,
			ItemID:   itemID,
			Quantity: amount,
		},
	}}}
}

func killQuestObjectiveRow(id string, npcType string, amount int64) QuestObjectiveSchemaRow {
	return QuestObjectiveSchemaRow{Objectives: []QuestObjectiveRow{{
		ID:   id,
		Kind: quests.ObjectiveKindKill,
		Kill: &QuestKillObjectiveRow{
			TargetNPCType: npcType,
			RequiredCount: amount,
		},
	}}}
}

func buildQuestObjectiveRow(id string, buildingType string, amount int64) QuestObjectiveSchemaRow {
	return QuestObjectiveSchemaRow{Objectives: []QuestObjectiveRow{{
		ID:   id,
		Kind: quests.ObjectiveKindBuild,
		Build: &QuestBuildObjectiveRow{
			BuildingType:  buildingType,
			RequiredCount: amount,
		},
	}}}
}

func mustQuestContentSource(t *testing.T, id catalog.DefinitionID) catalog.VersionedDefinition {
	t.Helper()
	source, err := catalog.NewQuestSource(id.String(), "quest_content_test_v1")
	if err != nil {
		t.Fatalf("NewQuestSource(%q) error = %v", id, err)
	}
	return source
}

func mustQuestContentJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return raw
}

func validQuestReferenceResolver() QuestReferenceResolver {
	items := map[foundation.ItemID]struct{}{
		"carbon_shards": {},
		"energy_cell":   {},
		"iron_ore":      {},
	}
	recipes := map[catalog.DefinitionID]struct{}{
		"refined_alloy_batch": {},
	}
	buildings := map[string]struct{}{
		"iron_extractor": {},
	}
	npcs := map[string]struct{}{
		"raider": {},
	}
	return QuestReferenceResolver{
		HasItem: func(id foundation.ItemID) bool {
			_, ok := items[id]
			return ok
		},
		HasShip: func(id foundation.ShipID) bool {
			return id == "starter"
		},
		HasRecipe: func(id catalog.DefinitionID) bool {
			_, ok := recipes[id]
			return ok
		},
		HasProduction: func(id catalog.DefinitionID) bool {
			return id == "iron_extractor_l1"
		},
		HasBuilding: func(id string) bool {
			_, ok := buildings[id]
			return ok
		},
		HasNPC: func(id string) bool {
			_, ok := npcs[id]
			return ok
		},
	}
}
