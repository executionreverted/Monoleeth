package contentseed

import (
	"fmt"
	"sort"

	"gameproject/internal/game/content"
	"gameproject/internal/game/contentseed/kalaazu"
	"gameproject/internal/game/world"
)

// LegacyBridgeRow documents default-snapshot rows that still come from local
// legacy content because the selected Kalaazu dumps do not model that system yet.
type LegacyBridgeRow struct {
	ContentType content.ContentType `json:"content_type"`
	ContentID   content.ContentID   `json:"content_id"`
	Reason      string              `json:"reason"`
}

// DefaultSnapshotLegacyBridgeReport returns the explicit temporary bridge rows
// left in the Kalaazu-derived default snapshot.
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
	add(content.ContentTypeNPCTemplate, rows.NPCTemplateRows)
	add(content.ContentTypeSpawnArea, rows.SpawnAreaRows)
	add(content.ContentTypeEnemyPool, rows.EnemyPoolRows)
	add(content.ContentTypeNPCDropProfile, rows.NPCDropRows)
	add(content.ContentTypeNPCAggroProfile, rows.NPCAggroRows)
	add(content.ContentTypeNPCLeashProfile, rows.NPCLeashRows)
	add(content.ContentTypeScannerConfig, rows.ScannerConfigRows)
	add(content.ContentTypeStarterConfig, rows.StarterConfigRows)
	add(content.ContentTypeRoutePolicy, rows.RoutePolicyRows)
	return out
}

var explicitLegacyBridgeReasons = map[content.ContentType]map[content.ContentID]string{
	content.ContentTypeCraftRecipe: {
		"laser_alpha_t1":      "Local crafting recipe bridge; selected Kalaazu dumps do not include craft recipe rows.",
		"refined_alloy_batch": "Local crafting recipe bridge; selected Kalaazu dumps do not include craft recipe rows.",
		"scout_t1_unlock":     "Local crafting recipe bridge; selected Kalaazu dumps do not include craft recipe rows.",
	},
	content.ContentTypeProductionBuilding: {
		"alloy_foundry_l1":  "Local production building bridge; selected Kalaazu dumps do not include planet production rows.",
		"iron_extractor_l1": "Local production building bridge; selected Kalaazu dumps do not include planet production rows.",
		"iron_extractor_l2": "Local production building bridge; selected Kalaazu dumps do not include planet production rows.",
	},
	content.ContentTypeQuestTemplate: {
		"quest_build_extractor_r1":       "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_build_storage_r1":         "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_collect_carbon_shards_r1": "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_collect_iron_ore_r1":      "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_craft_energy_cells_r1":    "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_craft_laser_alpha_r2":     "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_craft_refined_alloy_r1":   "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_deliver_energy_cells_r1":  "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_deliver_iron_ore_r1":      "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_kill_pirates_r1":          "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_kill_raiders_r1":          "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_kill_void_raiders_r3":     "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_scan_planets_r1":          "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
		"quest_scan_signals_r1":          "Local quest template bridge; selected Kalaazu dumps do not include quest rows.",
	},
	content.ContentTypeQuestRewardTable: {
		"quest_rewards.quest_build_extractor_r1":       "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_build_storage_r1":         "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_collect_carbon_shards_r1": "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_collect_iron_ore_r1":      "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_craft_energy_cells_r1":    "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_craft_laser_alpha_r2":     "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_craft_refined_alloy_r1":   "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_deliver_energy_cells_r1":  "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_deliver_iron_ore_r1":      "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_kill_pirates_r1":          "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_kill_raiders_r1":          "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_kill_void_raiders_r3":     "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_scan_planets_r1":          "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
		"quest_rewards.quest_scan_signals_r1":          "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.",
	},
	content.ContentTypeProductionRules: {
		"production_rules": "Local production rules bridge; selected Kalaazu dumps do not include production rule rows.",
	},
	content.ContentTypeCombatRules: {
		"combat_rules": "Local combat rules bridge; selected Kalaazu dumps do not include player combat-rule rows.",
	},
}

func legacyBridgeReason(contentType content.ContentType, contentID content.ContentID) (string, bool) {
	byID, ok := explicitLegacyBridgeReasons[contentType]
	if !ok {
		return "", false
	}
	reason, ok := byID[contentID]
	return reason, ok
}
