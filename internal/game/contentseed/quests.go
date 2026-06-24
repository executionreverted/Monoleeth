package contentseed

import (
	"encoding/json"
	"fmt"
	"sort"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/quests"
)

const questRewardTablePrefix = "quest_rewards."

// QuestTemplateRows compiles runtime MVP quest templates into CMS rows.
func QuestTemplateRows(questCatalog quests.QuestCatalog) ([]content.SnapshotRow, error) {
	templates := questCatalog.Templates()
	sort.Slice(templates, func(i, j int) bool { return templates[i].TemplateID < templates[j].TemplateID })

	rows := make([]content.SnapshotRow, 0, len(templates))
	for _, template := range templates {
		data, err := questTemplateRow(template)
		if err != nil {
			return nil, err
		}
		row, err := newSnapshotRow(data.TemplateID.String(), data)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// QuestRewardTableRows compiles deterministic MVP reward tables for templates.
func QuestRewardTableRows(questCatalog quests.QuestCatalog) ([]content.SnapshotRow, error) {
	templates := questCatalog.Templates()
	sort.Slice(templates, func(i, j int) bool { return templates[i].TemplateID < templates[j].TemplateID })

	rows := make([]content.SnapshotRow, 0, len(templates))
	for _, template := range templates {
		data, err := questRewardTableRow(template)
		if err != nil {
			return nil, err
		}
		row, err := newSnapshotRow(data.RewardTableID.String(), data)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func questTemplateRow(template quests.QuestTemplate) (content.QuestTemplateRow, error) {
	objectiveSchema, err := questObjectiveSchemaRow(template.ObjectiveSchema)
	if err != nil {
		return content.QuestTemplateRow{}, err
	}
	return content.QuestTemplateRow{
		Source:          template.Source,
		TemplateID:      template.TemplateID,
		Type:            template.Type,
		TitleKey:        template.TitleKey,
		DescriptionKey:  template.DescriptionKey,
		DifficultyRules: cloneRawJSON(template.DifficultyRules),
		ObjectiveSchema: objectiveSchema,
		RewardRules:     cloneRawJSON(template.RewardRules),
		ExpirationRules: cloneRawJSON(template.ExpirationRules),
		Requirements:    append([]quests.QuestRequirement(nil), template.Requirements...),
		BoardWeight:     100,
	}, nil
}

func questRewardTableRow(template quests.QuestTemplate) (content.QuestRewardTableRow, error) {
	rewardTableID := catalog.DefinitionID(questRewardTablePrefix + template.TemplateID.String())
	source, err := mustQuestSource(rewardTableID)
	if err != nil {
		return content.QuestRewardTableRow{}, err
	}
	payload := deterministicRewardPayload(template)
	if err := payload.Validate(); err != nil {
		return content.QuestRewardTableRow{}, err
	}
	return content.QuestRewardTableRow{
		Source:        source,
		RewardTableID: rewardTableID,
		TemplateID:    template.TemplateID,
		RewardPayload: payload,
		Weight:        100,
		Probability:   1,
	}, nil
}

func questObjectiveSchemaRow(schema quests.ObjectiveSchema) (content.QuestObjectiveSchemaRow, error) {
	if len(schema.Objectives) > 0 {
		objectives := make([]content.QuestObjectiveRow, 0, len(schema.Objectives))
		for _, objective := range schema.Objectives {
			row, err := questObjectiveRow(objective)
			if err != nil {
				return content.QuestObjectiveSchemaRow{}, err
			}
			objectives = append(objectives, row)
		}
		return content.QuestObjectiveSchemaRow{Objectives: objectives}, nil
	}
	row := content.QuestObjectiveSchemaRow{
		Kind:    schema.Kind,
		Kill:    clonePtr(schema.Kill),
		Collect: clonePtr(schema.Collect),
		Craft:   clonePtr(schema.Craft),
		Scan:    clonePtr(schema.Scan),
		Build:   clonePtr(schema.Build),
		Deliver: clonePtr(schema.Deliver),
	}
	if row.Kill != nil {
		row.Kill.NPCType = seededQuestNPCType(row.Kill.NPCType)
	}
	if row.Craft != nil {
		row.Craft.RecipeID, row.Craft.ItemID = seededQuestCraftTarget(row.Craft.RecipeID, row.Craft.ItemID)
	}
	if row.Build != nil {
		row.Build.BuildingID = seededQuestBuildingType(row.Build.BuildingID)
	}
	return row, nil
}

func questObjectiveRow(objective quests.Objective) (content.QuestObjectiveRow, error) {
	row := content.QuestObjectiveRow{
		ID:   objective.ID,
		Kind: objective.Kind,
	}
	switch objective.Kind {
	case quests.ObjectiveKindKill:
		if objective.Kill == nil {
			return content.QuestObjectiveRow{}, fmt.Errorf("objective %q missing kill detail", objective.ID)
		}
		row.Kill = &content.QuestKillObjectiveRow{
			TargetNPCType: seededQuestNPCType(objective.Kill.TargetNPCType),
			RequiredCount: objective.Kill.RequiredCount.Int64(),
		}
	case quests.ObjectiveKindCollect:
		if objective.Collect == nil {
			return content.QuestObjectiveRow{}, fmt.Errorf("objective %q missing collect detail", objective.ID)
		}
		row.Collect = &content.QuestCollectObjectiveRow{
			ItemID:   objective.Collect.ItemID,
			Quantity: objective.Collect.Quantity.Int64(),
		}
	case quests.ObjectiveKindCraft:
		if objective.Craft == nil {
			return content.QuestObjectiveRow{}, fmt.Errorf("objective %q missing craft detail", objective.ID)
		}
		recipeID, itemID := seededQuestCraftTarget(objective.Craft.RecipeID, objective.Craft.ItemID)
		row.Craft = &content.QuestCraftObjectiveRow{
			RecipeID: recipeID,
			ItemID:   itemID,
			Quantity: objective.Craft.Quantity.Int64(),
		}
	case quests.ObjectiveKindScan:
		if objective.Scan == nil {
			return content.QuestObjectiveRow{}, fmt.Errorf("objective %q missing scan detail", objective.ID)
		}
		row.Scan = &content.QuestScanObjectiveRow{
			TargetSignalType: objective.Scan.TargetSignalType,
			RequiredCount:    objective.Scan.RequiredCount.Int64(),
		}
	case quests.ObjectiveKindBuild:
		if objective.Build == nil {
			return content.QuestObjectiveRow{}, fmt.Errorf("objective %q missing build detail", objective.ID)
		}
		row.Build = &content.QuestBuildObjectiveRow{
			BuildingType:  seededQuestBuildingType(objective.Build.BuildingType),
			RequiredCount: objective.Build.RequiredCount.Int64(),
		}
	case quests.ObjectiveKindDeliver:
		if objective.Deliver == nil {
			return content.QuestObjectiveRow{}, fmt.Errorf("objective %q missing deliver detail", objective.ID)
		}
		row.Deliver = &content.QuestDeliverObjectiveRow{
			ItemID:          objective.Deliver.ItemID,
			Quantity:        objective.Deliver.Quantity.Int64(),
			DestinationType: objective.Deliver.DestinationType,
			DestinationID:   objective.Deliver.DestinationID,
		}
	default:
		return content.QuestObjectiveRow{}, fmt.Errorf("objective %q kind %q", objective.ID, objective.Kind)
	}
	return row, nil
}

func seededQuestNPCType(npcType string) string {
	switch npcType {
	case "pirate":
		return "training_drone"
	case "raider":
		return "border_raider_drone"
	case "void_raider":
		return "outer_ring_scout_drone"
	default:
		return npcType
	}
}

func seededQuestCraftTarget(recipeID catalog.DefinitionID, itemID foundation.ItemID) (catalog.DefinitionID, foundation.ItemID) {
	switch recipeID {
	case "energy_cell_batch":
		return crafting.RecipeIDRefinedAlloy, ""
	default:
		return recipeID, itemID
	}
}

func seededQuestBuildingType(buildingType string) string {
	switch buildingType {
	case "extractor_t1":
		return production.BuildingTypeIronExtractor.String()
	case "storage_t1":
		return production.BuildingTypeAlloyFoundry.String()
	default:
		return buildingType
	}
}

func deterministicRewardPayload(template quests.QuestTemplate) quests.RewardPayload {
	rank := minimumTemplateRank(template.Requirements)
	difficulty := int64(rank)
	grants := []quests.RewardGrant{
		{
			Kind:     quests.RewardKindCredits,
			Currency: economy.CurrencyBucketCredits,
			Amount:   100 + difficulty*75 + int64(rank)*50,
		},
		{
			Kind:   quests.RewardKindMainXP,
			Amount: 20 + difficulty*20 + int64(rank)*5,
		},
		{
			Kind:   quests.RewardKindRoleXP,
			Role:   rewardRoleForQuestType(template.Type),
			Amount: 15 + difficulty*15,
		},
	}
	if itemID := rewardItemForQuestType(template.Type); !itemID.IsZero() {
		grants = append(grants, quests.RewardGrant{
			Kind:   quests.RewardKindItem,
			ItemID: itemID,
			Amount: difficulty,
		})
	}
	return quests.RewardPayload{Grants: grants}
}

func minimumTemplateRank(requirements []quests.QuestRequirement) int {
	rank := 1
	for _, requirement := range requirements {
		if requirement.MinRank > rank {
			rank = requirement.MinRank
		}
	}
	if rank > progression.MaxMVPRank {
		return progression.MaxMVPRank
	}
	return rank
}

func rewardRoleForQuestType(questType quests.QuestType) progression.RoleType {
	switch questType {
	case quests.QuestTypeScan:
		return progression.RoleTypeScout
	case quests.QuestTypeCraft:
		return progression.RoleTypeCrafting
	case quests.QuestTypeBuild, quests.QuestTypeDeliver:
		return progression.RoleTypeConstruction
	default:
		return progression.RoleTypeCombat
	}
}

func rewardItemForQuestType(questType quests.QuestType) foundation.ItemID {
	switch questType {
	case quests.QuestTypeKill:
		return "iron_ore"
	case quests.QuestTypeCollect:
		return "carbon_shards"
	case quests.QuestTypeCraft:
		return "energy_cell"
	case quests.QuestTypeScan:
		return "scanner_circuit"
	case quests.QuestTypeBuild:
		return "refined_alloy"
	case quests.QuestTypeDeliver:
		return "helium_dust"
	default:
		return ""
	}
}

func cloneRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func clonePtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
