package contentseed

import (
	"fmt"
	"sort"

	"gameproject/internal/game/content"
	"gameproject/internal/game/contentseed/kalaazu"
	"gameproject/internal/game/world"
)

// LegacyBridgeRow documents default-snapshot rows that are temporarily allowed
// to come from local legacy content instead of the Kalaazu default row set.
type LegacyBridgeRow struct {
	ContentType content.ContentType `json:"content_type"`
	ContentID   content.ContentID   `json:"content_id"`
	Reason      string              `json:"reason"`
}

// DefaultSnapshotLegacyBridgeReport returns explicit temporary bridge rows left
// in the Kalaazu-derived default snapshot. A complete default seed returns none.
func DefaultSnapshotLegacyBridgeReport(worldID world.WorldID) ([]LegacyBridgeRow, error) {
	snapshot, err := BuildDefaultSnapshot(worldID)
	if err != nil {
		return nil, err
	}
	kalaazuRows, err := kalaazu.BuildDefaultRows(kalaazu.DefaultSeedFS())
	if err != nil {
		return nil, err
	}
	kalaazuIDs := kalaazuDefaultRowIDs(kalaazuRows)

	bridges := make([]LegacyBridgeRow, 0)
	for _, group := range snapshot.Groups() {
		for _, row := range group.Rows {
			if _, ok := kalaazuIDs[group.Type][row.ContentID]; ok {
				continue
			}
			reason, ok := legacyBridgeReason(group.Type, row.ContentID)
			if !ok {
				return nil, fmt.Errorf("default snapshot bridge row %s/%s has no explicit reason", group.Type, row.ContentID)
			}
			bridges = append(bridges, LegacyBridgeRow{
				ContentType: group.Type,
				ContentID:   row.ContentID,
				Reason:      reason,
			})
		}
	}
	sort.Slice(bridges, func(i, j int) bool {
		if bridges[i].ContentType == bridges[j].ContentType {
			return bridges[i].ContentID < bridges[j].ContentID
		}
		return bridges[i].ContentType < bridges[j].ContentType
	})
	return bridges, nil
}

func kalaazuDefaultRowIDs(rows kalaazu.DefaultRows) map[content.ContentType]map[content.ContentID]struct{} {
	out := make(map[content.ContentType]map[content.ContentID]struct{})
	add := func(contentType content.ContentType, rows []content.SnapshotRow) {
		if _, ok := out[contentType]; !ok {
			out[contentType] = make(map[content.ContentID]struct{}, len(rows))
		}
		for _, row := range rows {
			out[contentType][row.ContentID] = struct{}{}
		}
	}
	add(content.ContentTypeMap, rows.MapRows)
	add(content.ContentTypeMapPortal, rows.MapPortalRows)
	add(content.ContentTypeItem, rows.ItemRows)
	add(content.ContentTypeModule, rows.ModuleRows)
	add(content.ContentTypeShip, rows.ShipRows)
	add(content.ContentTypeShopProduct, rows.ShopProductRows)
	add(content.ContentTypeLootTable, rows.LootTableRows)
	add(content.ContentTypeCraftRecipe, rows.CraftRecipeRows)
	add(content.ContentTypeProductionBuilding, rows.ProductionBuildingRows)
	add(content.ContentTypeNPCTemplate, rows.NPCTemplateRows)
	add(content.ContentTypeSpawnArea, rows.SpawnAreaRows)
	add(content.ContentTypeEnemyPool, rows.EnemyPoolRows)
	add(content.ContentTypeNPCDropProfile, rows.NPCDropRows)
	add(content.ContentTypeNPCAggroProfile, rows.NPCAggroRows)
	add(content.ContentTypeNPCLeashProfile, rows.NPCLeashRows)
	add(content.ContentTypeScannerConfig, rows.ScannerConfigRows)
	add(content.ContentTypeStarterConfig, rows.StarterConfigRows)
	add(content.ContentTypeRoutePolicy, rows.RoutePolicyRows)
	add(content.ContentTypeProductionRules, rows.ProductionRuleRows)
	add(content.ContentTypeCombatRules, rows.CombatRuleRows)
	add(content.ContentTypeQuestTemplate, rows.QuestTemplateRows)
	add(content.ContentTypeQuestRewardTable, rows.QuestRewardRows)
	return out
}

var explicitLegacyBridgeReasons = map[content.ContentType]map[content.ContentID]string{}

func legacyBridgeReason(contentType content.ContentType, contentID content.ContentID) (string, bool) {
	byID, ok := explicitLegacyBridgeReasons[contentType]
	if !ok {
		return "", false
	}
	reason, ok := byID[contentID]
	return reason, ok
}
