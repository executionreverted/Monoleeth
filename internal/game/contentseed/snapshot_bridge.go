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
	return out
}

func legacyBridgeReason(contentType content.ContentType, contentID content.ContentID) (string, bool) {
	switch contentType {
	case content.ContentTypeItem:
		return "Legacy item definition retained until loot tables, crafting, production, quests, and starter grants are fully Kalaazu-derived.", true
	case content.ContentTypeModule:
		return "Legacy module definition retained for starter loadout and compatibility where no Kalaazu row shares this module id.", true
	case content.ContentTypeShip:
		if contentID == content.ContentID(content.DefaultStarterShipID.String()) {
			return "Legacy starter ship id contract retained for loadout/session compatibility; stats are overwritten from Kalaazu Phoenix.", true
		}
		return "Legacy ship definition retained for shop/hangar compatibility where no Kalaazu row shares this ship id.", true
	case content.ContentTypeCraftRecipe:
		return "Local crafting recipe bridge; selected Kalaazu dumps do not include craft recipe rows.", true
	case content.ContentTypeProductionBuilding:
		return "Local production building bridge; selected Kalaazu dumps do not include planet production rows.", true
	case content.ContentTypeQuestTemplate:
		return "Local quest template bridge; selected Kalaazu dumps do not include quest rows.", true
	case content.ContentTypeQuestRewardTable:
		return "Local quest reward bridge; selected Kalaazu dumps do not include quest reward rows.", true
	case content.ContentTypeScannerConfig:
		return "Local scanner config bridge; selected Kalaazu dumps do not include scanner tuning rows.", true
	case content.ContentTypeStarterConfig:
		return "Local starter config bridge with Kalaazu starter pool and Phoenix display projected into existing account/session contracts.", true
	case content.ContentTypeRoutePolicy:
		return "Local route policy bridge; selected Kalaazu dumps do not include automation route rules.", true
	case content.ContentTypeProductionRules:
		return "Local production rules bridge; selected Kalaazu dumps do not include production rule rows.", true
	case content.ContentTypeCombatRules:
		return "Local combat rules bridge; selected Kalaazu dumps do not include player combat-rule rows.", true
	default:
		return "", false
	}
}
