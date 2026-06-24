package contentdb

import (
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/ships"
	worldmaps "gameproject/internal/game/world/maps"
)

func mapQuestRows(
	snapshot content.Snapshot,
	items map[foundation.ItemID]economy.ItemDefinition,
	shipCatalog ships.Catalog,
	recipeCatalog crafting.RecipeCatalog,
	productionCatalog production.Catalog,
	mapCatalog *worldmaps.Catalog,
) (quests.QuestCatalog, error) {
	resolver := questReferenceResolver(items, shipCatalog, recipeCatalog, productionCatalog, mapCatalog)
	if err := content.ValidateQuestContentRows(snapshot.QuestTemplates, snapshot.QuestRewardTables, resolver); err != nil {
		return quests.QuestCatalog{}, err
	}
	rewardPayloads, err := mapQuestRewardPayloadRows(snapshot)
	if err != nil {
		return quests.QuestCatalog{}, err
	}
	version := publishedVersion(snapshot)
	templates := make([]quests.QuestTemplate, 0, len(snapshot.QuestTemplates))
	for _, row := range snapshot.QuestTemplates {
		if !row.Enabled {
			continue
		}
		var data content.QuestTemplateRow
		if err := decodeSnapshotRow(content.ContentTypeQuestTemplate, row, &data); err != nil {
			return quests.QuestCatalog{}, err
		}
		if err := requireRowID(content.ContentTypeQuestTemplate, row, data.TemplateID.String()); err != nil {
			return quests.QuestCatalog{}, err
		}
		data.Source = forceSourceVersion(data.Source, version)
		template, err := data.Template()
		if err != nil {
			return quests.QuestCatalog{}, fmt.Errorf("quest template %q: %w", row.ContentID, err)
		}
		rewardPayload, ok := rewardPayloads[template.TemplateID]
		if !ok {
			return quests.QuestCatalog{}, fmt.Errorf("quest template %q reward table: %w", template.TemplateID, content.ErrInvalidQuestRewardTable)
		}
		template.RewardPayload = &rewardPayload
		templates = append(templates, template)
	}
	questCatalog, err := quests.NewQuestCatalog(templates)
	if err != nil {
		return quests.QuestCatalog{}, fmt.Errorf("quests: %w", err)
	}
	return questCatalog, nil
}

func mapQuestRewardPayloadRows(snapshot content.Snapshot) (map[catalog.DefinitionID]quests.RewardPayload, error) {
	rewards := make(map[catalog.DefinitionID]quests.RewardPayload, len(snapshot.QuestRewardTables))
	for _, row := range snapshot.QuestRewardTables {
		if !row.Enabled {
			continue
		}
		var data content.QuestRewardTableRow
		if err := decodeSnapshotRow(content.ContentTypeQuestRewardTable, row, &data); err != nil {
			return nil, err
		}
		if err := requireRowID(content.ContentTypeQuestRewardTable, row, data.RewardTableID.String()); err != nil {
			return nil, err
		}
		if _, exists := rewards[data.TemplateID]; exists {
			return nil, fmt.Errorf("quest template %q reward table: %w", data.TemplateID, content.ErrDuplicateQuestRewardTable)
		}
		rewards[data.TemplateID] = data.RewardPayload
	}
	return rewards, nil
}

func questReferenceResolver(
	items map[foundation.ItemID]economy.ItemDefinition,
	shipCatalog ships.Catalog,
	recipeCatalog crafting.RecipeCatalog,
	productionCatalog production.Catalog,
	mapCatalog *worldmaps.Catalog,
) content.QuestReferenceResolver {
	return content.QuestReferenceResolver{
		HasItem: func(itemID foundation.ItemID) bool {
			_, ok := items[itemID]
			return ok
		},
		HasShip: func(shipID foundation.ShipID) bool {
			_, ok := shipCatalog.Get(shipID)
			return ok
		},
		HasRecipe: func(recipeID catalog.DefinitionID) bool {
			_, ok := recipeCatalog.Get(recipeID)
			return ok
		},
		HasProduction: func(definitionID catalog.DefinitionID) bool {
			_, ok := productionCatalog.Get(definitionID)
			return ok
		},
		HasBuilding: func(buildingID string) bool {
			for _, definition := range productionCatalog.Definitions() {
				if definition.BuildingType.String() == buildingID {
					return true
				}
			}
			return false
		},
		HasNPC: func(npcType string) bool {
			if mapCatalog == nil {
				return false
			}
			for _, definition := range mapCatalog.Definitions() {
				for _, template := range definition.NPCStatTemplates {
					if template.NPCType == npcType {
						return true
					}
				}
			}
			return false
		},
	}
}
