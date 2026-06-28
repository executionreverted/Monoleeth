package contentseed

import (
	"encoding/json"
	"fmt"
	"sort"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/contentseed/kalaazu"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const MVPSnapshotVersion = "content_kalaazu_starter_seed_v1"

// BuildMVPSnapshot compiles the current validated seed bundle into deterministic
// CMS snapshot rows for first-run contentdb publishing.
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
	if err := applyKalaazuStarterRows(snapshot); err != nil {
		return err
	}
	if err := appendServerRuleRows(snapshot, bundle); err != nil {
		return err
	}
	return nil
}

func applyKalaazuStarterRows(snapshot *content.Snapshot) error {
	mapRows, err := kalaazu.BuildStarterMapRows(kalaazu.DefaultSeedFS())
	if err != nil {
		return err
	}
	itemRows, err := kalaazu.BuildStarterItemRows(kalaazu.DefaultSeedFS())
	if err != nil {
		return err
	}
	moduleRows, err := kalaazu.BuildStarterModuleRows(kalaazu.DefaultSeedFS())
	if err != nil {
		return err
	}
	npcRows, err := kalaazu.BuildStarterNPCRows(kalaazu.DefaultSeedFS())
	if err != nil {
		return err
	}
	shipRows, err := kalaazu.BuildStarterShipRows(kalaazu.DefaultSeedFS())
	if err != nil {
		return err
	}
	shopRows, err := kalaazu.BuildStarterShopRows(kalaazu.DefaultSeedFS())
	if err != nil {
		return err
	}
	snapshot.Maps = mapRows.MapRows
	snapshot.MapPortals = mapRows.PortalRows
	snapshot.Items = appendMissingSnapshotRows(snapshot.Items, itemRows)
	snapshot.Modules = appendMissingSnapshotRows(snapshot.Modules, moduleRows)
	snapshot.Ships = appendMissingSnapshotRows(snapshot.Ships, shipRows)
	snapshot.ShopProducts = appendMissingSnapshotRows(snapshot.ShopProducts, shopRows)
	snapshot.NPCTemplates = npcRows.NPCTemplates
	snapshot.SpawnAreas = npcRows.SpawnAreas
	snapshot.EnemyPools = npcRows.EnemyPools
	snapshot.NPCDropProfiles = npcRows.NPCDropProfiles
	snapshot.NPCAggroProfiles = npcRows.NPCAggroProfiles
	snapshot.NPCLeashProfiles = npcRows.NPCLeashProfiles
	snapshot.NPCEventSpawns = nil
	return nil
}

func appendMissingSnapshotRows(existing []content.SnapshotRow, candidates []content.SnapshotRow) []content.SnapshotRow {
	seen := make(map[content.ContentID]struct{}, len(existing)+len(candidates))
	for _, row := range existing {
		seen[row.ContentID] = struct{}{}
	}
	out := append([]content.SnapshotRow(nil), existing...)
	for _, row := range candidates {
		if _, exists := seen[row.ContentID]; exists {
			continue
		}
		seen[row.ContentID] = struct{}{}
		out = append(out, row)
	}
	return out
}

func appendServerRuleRows(snapshot *content.Snapshot, bundle content.GameplayContent) error {
	var err error
	if snapshot.ScannerConfigs, err = singletonRow("scanner_config", bundle.Scanner); err != nil {
		return err
	}
	if snapshot.StarterConfigs, err = singletonRow("starter_config", bundle.Starter); err != nil {
		return err
	}
	if snapshot.RoutePolicies, err = singletonRow("route_policy", bundle.Route); err != nil {
		return err
	}
	if snapshot.ProductionRules, err = singletonRow("production_rules", bundle.Rules); err != nil {
		return err
	}
	if snapshot.CombatRules, err = singletonRow("combat_rules", bundle.Combat); err != nil {
		return err
	}
	return nil
}

func singletonRow(contentID string, data any) ([]content.SnapshotRow, error) {
	row, err := newSnapshotRow(contentID, data)
	if err != nil {
		return nil, err
	}
	return []content.SnapshotRow{row}, nil
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
		row, err := newSnapshotRow(tableID, content.SnapshotDataForLootTable(bundle.LootTables[tableID]))
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
		row, err := newSnapshotRow(template.StatTemplateID.String(), npcTemplateRowData(definition.InternalMapID, template))
		if err != nil {
			return err
		}
		snapshot.NPCTemplates = append(snapshot.NPCTemplates, row)
	}
	for _, area := range definition.SpawnAreas {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, area.SpawnAreaID.String()), spawnAreaRowData(definition.InternalMapID, area))
		if err != nil {
			return err
		}
		snapshot.SpawnAreas = append(snapshot.SpawnAreas, row)
	}
	for _, pool := range definition.EnemyPools {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, pool.EnemyPoolID.String()), enemyPoolRowData(definition.InternalMapID, pool))
		if err != nil {
			return err
		}
		snapshot.EnemyPools = append(snapshot.EnemyPools, row)
	}
	for _, profile := range definition.NPCDropProfiles {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, profile.DropProfileID.String()), npcDropProfileRowData(definition.InternalMapID, profile))
		if err != nil {
			return err
		}
		snapshot.NPCDropProfiles = append(snapshot.NPCDropProfiles, row)
	}
	for _, profile := range definition.NPCAggroProfiles {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, profile.AggroProfileID.String()), npcAggroProfileRowData(definition.InternalMapID, profile))
		if err != nil {
			return err
		}
		snapshot.NPCAggroProfiles = append(snapshot.NPCAggroProfiles, row)
	}
	for _, profile := range definition.NPCLeashProfiles {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, profile.LeashProfileID.String()), npcLeashProfileRowData(definition.InternalMapID, profile))
		if err != nil {
			return err
		}
		snapshot.NPCLeashProfiles = append(snapshot.NPCLeashProfiles, row)
	}
	for _, eventSpawn := range definition.NPCEventSpawns {
		row, err := newSnapshotRow(qualifiedMapContentID(definition.InternalMapID, eventSpawn.EventSpawnID.String()), npcEventSpawnRowData(definition.InternalMapID, eventSpawn))
		if err != nil {
			return err
		}
		snapshot.NPCEventSpawns = append(snapshot.NPCEventSpawns, row)
	}
	return nil
}

func npcTemplateRowData(mapID worldmaps.MapID, template worldmaps.NPCStatTemplate) map[string]any {
	return map[string]any{
		"map_id":           mapID,
		"stat_template_id": template.StatTemplateID,
		"npc_type":         template.NPCType,
		"min_level":        template.MinLevel,
		"max_level":        template.MaxLevel,
		"label_key":        template.LabelKey,
		"hp_max":           template.HPMax,
		"shield_max":       template.ShieldMax,
		"energy_max":       template.EnergyMax,
		"weapon_range":     template.WeaponRange,
		"weapon_damage":    template.WeaponDamage,
		"weapon_cooldown":  template.WeaponCooldown,
		"accuracy":         template.Accuracy,
		"radar_signature":  template.RadarSignature,
		"speed":            template.Speed,
		"xp_value":         template.XPValue,
	}
}

func spawnAreaRowData(mapID worldmaps.MapID, area worldmaps.MapSpawnAreaDefinition) map[string]any {
	return map[string]any{
		"map_id":                  mapID,
		"spawn_area_id":           area.SpawnAreaID,
		"shape":                   area.Shape,
		"center":                  area.Center,
		"radius":                  area.Radius,
		"safe_zone_excluded":      area.SafeZoneExcluded,
		"portal_exclusion_radius": area.PortalExclusionRadius,
	}
}

func enemyPoolRowData(mapID worldmaps.MapID, pool worldmaps.MapEnemyPoolDefinition) map[string]any {
	return map[string]any{
		"map_id":             mapID,
		"enemy_pool_id":      pool.EnemyPoolID,
		"npc_type":           pool.NPCType,
		"min_level":          pool.MinLevel,
		"max_level":          pool.MaxLevel,
		"spawn_area_ids":     pool.SpawnAreaIDs,
		"map_max_alive":      pool.MapMaxAlive,
		"pool_max_alive":     pool.PoolMaxAlive,
		"initial_alive":      pool.InitialAlive,
		"spawn_interval":     pool.SpawnInterval,
		"kill_respawn_delay": pool.KillRespawnDelay,
		"spawn_jitter":       pool.SpawnJitter,
		"spawn_mode":         pool.SpawnMode,
		"stat_template_id":   pool.StatTemplateID,
		"drop_profile_id":    pool.DropProfileID,
		"aggro_profile_id":   pool.AggroProfileID,
		"leash_profile_id":   pool.LeashProfileID,
		"enabled":            pool.Enabled,
	}
}

func npcDropProfileRowData(mapID worldmaps.MapID, profile worldmaps.NPCDropProfile) map[string]any {
	return map[string]any{
		"map_id":          mapID,
		"drop_profile_id": profile.DropProfileID,
		"npc_type":        profile.NPCType,
		"min_level":       profile.MinLevel,
		"max_level":       profile.MaxLevel,
		"risk_band":       profile.RiskBand,
		"loot_table_id":   profile.LootTableID,
	}
}

func npcAggroProfileRowData(mapID worldmaps.MapID, profile worldmaps.NPCAggroProfile) map[string]any {
	return map[string]any{
		"map_id":                  mapID,
		"aggro_profile_id":        profile.AggroProfileID,
		"aggro_radius":            profile.AggroRadius,
		"assist_radius":           profile.AssistRadius,
		"target_memory":           profile.TargetMemory,
		"safe_zone_attack_policy": profile.SafeZoneAttackPolicy,
	}
}

func npcLeashProfileRowData(mapID worldmaps.MapID, profile worldmaps.NPCLeashProfile) map[string]any {
	return map[string]any{
		"map_id":           mapID,
		"leash_profile_id": profile.LeashProfileID,
		"leash_distance":   profile.LeashDistance,
		"reset_on_break":   profile.ResetOnBreak,
	}
}

func npcEventSpawnRowData(mapID worldmaps.MapID, eventSpawn worldmaps.NPCEventSpawnDefinition) map[string]any {
	return map[string]any{
		"map_id":          mapID,
		"event_spawn_id":  eventSpawn.EventSpawnID,
		"enemy_pool_id":   eventSpawn.EnemyPoolID,
		"drop_profile_id": eventSpawn.DropProfileID,
		"enabled":         eventSpawn.Enabled,
		"starts_after":    eventSpawn.StartsAfter,
		"max_alive":       eventSpawn.MaxAlive,
		"map_policy":      eventSpawn.MapPolicy,
	}
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
