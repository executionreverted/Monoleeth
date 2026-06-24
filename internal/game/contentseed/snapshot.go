package contentseed

import (
	"encoding/json"
	"fmt"
	"sort"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const MVPSnapshotVersion = "content_mvp_seed_v1"

// BuildMVPSnapshot compiles the current validated static gameplay bundle into
// deterministic CMS snapshot rows.
func BuildMVPSnapshot(worldID world.WorldID) (content.Snapshot, error) {
	bundle, err := content.DefaultGameplayContent(worldID)
	if err != nil {
		return content.Snapshot{}, err
	}

	snapshot := content.Snapshot{Version: MVPSnapshotVersion}
	if err := appendCoreRows(&snapshot, bundle); err != nil {
		return content.Snapshot{}, err
	}
	if err := appendQuestRows(&snapshot); err != nil {
		return content.Snapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return content.Snapshot{}, err
	}
	return snapshot, nil
}

func appendCoreRows(snapshot *content.Snapshot, bundle content.GameplayContent) error {
	var err error
	if snapshot.Items, err = itemRows(bundle); err != nil {
		return err
	}
	if snapshot.Modules, err = moduleRows(bundle); err != nil {
		return err
	}
	if snapshot.Ships, err = shipRows(bundle); err != nil {
		return err
	}
	if snapshot.ShopProducts, err = shopProductRows(bundle); err != nil {
		return err
	}
	if snapshot.LootTables, err = lootTableRows(bundle); err != nil {
		return err
	}
	if snapshot.CraftRecipes, err = craftRecipeRows(bundle); err != nil {
		return err
	}
	if snapshot.ProductionBuildings, err = productionBuildingRows(bundle); err != nil {
		return err
	}
	if err := appendMapNPCRows(snapshot, bundle); err != nil {
		return err
	}
	return nil
}

func itemRows(bundle content.GameplayContent) ([]content.SnapshotRow, error) {
	itemIDs := make([]foundation.ItemID, 0, len(bundle.Items))
	for itemID := range bundle.Items {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Slice(itemIDs, func(i, j int) bool { return itemIDs[i] < itemIDs[j] })

	rows := make([]content.SnapshotRow, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		row, err := newSnapshotRow(itemID.String(), bundle.Items[itemID])
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func moduleRows(bundle content.GameplayContent) ([]content.SnapshotRow, error) {
	definitions := bundle.Modules.Definitions()
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].ItemID < definitions[j].ItemID })

	rows := make([]content.SnapshotRow, 0, len(definitions))
	for _, definition := range definitions {
		row, err := newSnapshotRow(definition.ItemID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func shipRows(bundle content.GameplayContent) ([]content.SnapshotRow, error) {
	definitions := bundle.Ships.All()
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].ShipID < definitions[j].ShipID })

	rows := make([]content.SnapshotRow, 0, len(definitions))
	for _, definition := range definitions {
		row, err := newSnapshotRow(definition.ShipID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func shopProductRows(bundle content.GameplayContent) ([]content.SnapshotRow, error) {
	products := bundle.Shop.SortedShopProducts()
	rows := make([]content.SnapshotRow, 0, len(products))
	for _, product := range products {
		row, err := newSnapshotRow(string(product.ProductID), product)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func lootTableRows(bundle content.GameplayContent) ([]content.SnapshotRow, error) {
	tableIDs := make([]string, 0, len(bundle.LootTables))
	for tableID := range bundle.LootTables {
		tableIDs = append(tableIDs, tableID)
	}
	sort.Strings(tableIDs)

	rows := make([]content.SnapshotRow, 0, len(tableIDs))
	for _, tableID := range tableIDs {
		row, err := newSnapshotRow(tableID, bundle.LootTables[tableID])
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func craftRecipeRows(bundle content.GameplayContent) ([]content.SnapshotRow, error) {
	definitions := bundle.Recipes.Definitions()
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].RecipeID < definitions[j].RecipeID })

	rows := make([]content.SnapshotRow, 0, len(definitions))
	for _, definition := range definitions {
		row, err := newSnapshotRow(definition.RecipeID.String(), craftRecipeSnapshotData(definition))
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func productionBuildingRows(bundle content.GameplayContent) ([]content.SnapshotRow, error) {
	definitions := bundle.Production.Definitions()
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].DefinitionID < definitions[j].DefinitionID })

	rows := make([]content.SnapshotRow, 0, len(definitions))
	for _, definition := range definitions {
		row, err := newSnapshotRow(definition.DefinitionID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func appendMapNPCRows(snapshot *content.Snapshot, bundle content.GameplayContent) error {
	if bundle.Maps == nil {
		return nil
	}
	for _, definition := range bundle.Maps.Definitions() {
		if err := appendMapDefinitionNPCRows(snapshot, definition); err != nil {
			return err
		}
	}
	return nil
}

func appendMapDefinitionNPCRows(snapshot *content.Snapshot, definition worldmaps.MapDefinition) error {
	for _, template := range definition.NPCStatTemplates {
		row, err := newSnapshotRow(template.StatTemplateID.String(), npcTemplateRowData(definition.InternalMapID, template.NPCType))
		if err != nil {
			return err
		}
		snapshot.NPCTemplates = append(snapshot.NPCTemplates, row)
	}
	for _, area := range definition.SpawnAreas {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, area.SpawnAreaID.String()), map[string]any{
			"map_id":        definition.InternalMapID,
			"spawn_area_id": area.SpawnAreaID,
		})
		if err != nil {
			return err
		}
		snapshot.SpawnAreas = append(snapshot.SpawnAreas, row)
	}
	for _, pool := range definition.EnemyPools {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, pool.EnemyPoolID.String()), map[string]any{
			"map_id":        definition.InternalMapID,
			"enemy_pool_id": pool.EnemyPoolID,
			"npc_type":      pool.NPCType,
		})
		if err != nil {
			return err
		}
		snapshot.EnemyPools = append(snapshot.EnemyPools, row)
	}
	for _, profile := range definition.NPCDropProfiles {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, profile.DropProfileID.String()), map[string]any{
			"map_id":          definition.InternalMapID,
			"drop_profile_id": profile.DropProfileID,
			"npc_type":        profile.NPCType,
			"loot_table_id":   profile.LootTableID,
		})
		if err != nil {
			return err
		}
		snapshot.NPCDropProfiles = append(snapshot.NPCDropProfiles, row)
	}
	for _, profile := range definition.NPCAggroProfiles {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, profile.AggroProfileID.String()), map[string]any{
			"map_id":           definition.InternalMapID,
			"aggro_profile_id": profile.AggroProfileID,
		})
		if err != nil {
			return err
		}
		snapshot.NPCAggroProfiles = append(snapshot.NPCAggroProfiles, row)
	}
	for _, profile := range definition.NPCLeashProfiles {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, profile.LeashProfileID.String()), map[string]any{
			"map_id":           definition.InternalMapID,
			"leash_profile_id": profile.LeashProfileID,
		})
		if err != nil {
			return err
		}
		snapshot.NPCLeashProfiles = append(snapshot.NPCLeashProfiles, row)
	}
	for _, eventSpawn := range definition.NPCEventSpawns {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, eventSpawn.EventSpawnID.String()), map[string]any{
			"map_id":         definition.InternalMapID,
			"event_spawn_id": eventSpawn.EventSpawnID,
		})
		if err != nil {
			return err
		}
		snapshot.NPCEventSpawns = append(snapshot.NPCEventSpawns, row)
	}
	return nil
}

func npcTemplateRowData(mapID worldmaps.MapID, npcType string) map[string]any {
	data := map[string]any{
		"npc_type": npcType,
	}
	if mapID != "" {
		data["map_id"] = mapID
	}
	return data
}

func qualifiedMapContentID(mapID worldmaps.MapID, id string) string {
	return fmt.Sprintf("%s.%s", mapID, id)
}

func newSnapshotRow(contentID string, data any) (content.SnapshotRow, error) {
	if err := content.ValidateContentID("content row", contentID); err != nil {
		return content.SnapshotRow{}, err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return content.SnapshotRow{}, err
	}
	return content.SnapshotRow{
		ContentID: content.ContentID(contentID),
		Enabled:   true,
		DataJSON:  raw,
	}, nil
}

func appendQuestRows(snapshot *content.Snapshot) error {
	templateRows, err := QuestTemplateRows(quests.MustMVPQuestCatalog())
	if err != nil {
		return err
	}
	rewardRows, err := QuestRewardTableRows(quests.MustMVPQuestCatalog())
	if err != nil {
		return err
	}
	snapshot.QuestTemplates = append(snapshot.QuestTemplates, templateRows...)
	snapshot.QuestRewardTables = append(snapshot.QuestRewardTables, rewardRows...)
	return nil
}

func mustQuestSource(definitionID catalog.DefinitionID) (catalog.VersionedDefinition, error) {
	return catalog.NewQuestSource(definitionID.String(), quests.QuestCatalogVersion.String())
}
