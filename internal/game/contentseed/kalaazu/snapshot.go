package kalaazu

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strconv"

	"gameproject/internal/game/content"
	worldmaps "gameproject/internal/game/world/maps"
)

type ImportReport struct {
	SourceRows       map[string]int              `json:"source_rows"`
	ImportedRows     map[content.ContentType]int `json:"imported_rows"`
	UnsupportedItems map[string]int              `json:"unsupported_items"`
}

type DefaultRows struct {
	MapRows                []content.SnapshotRow
	MapPortalRows          []content.SnapshotRow
	ItemRows               []content.SnapshotRow
	ModuleRows             []content.SnapshotRow
	ShipRows               []content.SnapshotRow
	ShopProductRows        []content.SnapshotRow
	LootTableRows          []content.SnapshotRow
	CraftRecipeRows        []content.SnapshotRow
	ProductionBuildingRows []content.SnapshotRow
	ProductionRuleRows     []content.SnapshotRow
	NPCTemplateRows        []content.SnapshotRow
	SpawnAreaRows          []content.SnapshotRow
	EnemyPoolRows          []content.SnapshotRow
	NPCDropRows            []content.SnapshotRow
	NPCAggroRows           []content.SnapshotRow
	NPCLeashRows           []content.SnapshotRow
	ScannerConfigRows      []content.SnapshotRow
	StarterConfigRows      []content.SnapshotRow
	RoutePolicyRows        []content.SnapshotRow
	Report                 ImportReport
}

func BuildDefaultRows(filesystem fs.FS) (DefaultRows, error) {
	source, err := loadDefaultSourceRows(filesystem)
	if err != nil {
		return DefaultRows{}, err
	}
	mapRows, err := mapStarterMapRows(source.Maps, source.MapPortals)
	if err != nil {
		return DefaultRows{}, err
	}
	itemRows, err := mapStarterItemRows(source.Items)
	if err != nil {
		return DefaultRows{}, err
	}
	moduleRows, err := mapStarterModuleRows(source.Items)
	if err != nil {
		return DefaultRows{}, err
	}
	shipRows, err := mapStarterShipRows(source.Items, source.Ships)
	if err != nil {
		return DefaultRows{}, err
	}
	shopRows, err := mapStarterShopRows(source.Items, shipRows, moduleRows)
	if err != nil {
		return DefaultRows{}, err
	}
	lootRows, err := mapStarterLootTableRows()
	if err != nil {
		return DefaultRows{}, err
	}
	if err := requireCraftRecipeSourceRows(itemRows, shipRows); err != nil {
		return DefaultRows{}, err
	}
	craftRecipeRows, err := mapCraftRecipeRows()
	if err != nil {
		return DefaultRows{}, err
	}
	productionBuildingRows, err := mapProductionBuildingRows(itemRows)
	if err != nil {
		return DefaultRows{}, err
	}
	productionRuleRows, err := mapProductionRuleRows(itemRows, productionBuildingRows)
	if err != nil {
		return DefaultRows{}, err
	}
	npcRows, err := mapStarterNPCRows(source.Maps, source.MapNPCs, source.NPCs)
	if err != nil {
		return DefaultRows{}, err
	}
	scannerConfigRows, err := mapScannerConfigRows(mapRows.MapRows)
	if err != nil {
		return DefaultRows{}, err
	}
	starterConfigRows, err := mapStarterConfigRows(npcRows.EnemyPools)
	if err != nil {
		return DefaultRows{}, err
	}
	routePolicyRows, err := mapRoutePolicyRows()
	if err != nil {
		return DefaultRows{}, err
	}
	rows := DefaultRows{
		MapRows:                mapRows.MapRows,
		MapPortalRows:          mapRows.PortalRows,
		ItemRows:               itemRows,
		ModuleRows:             moduleRows,
		ShipRows:               shipRows,
		ShopProductRows:        shopRows,
		LootTableRows:          lootRows,
		CraftRecipeRows:        craftRecipeRows,
		ProductionBuildingRows: productionBuildingRows,
		ProductionRuleRows:     productionRuleRows,
		NPCTemplateRows:        npcRows.NPCTemplates,
		SpawnAreaRows:          npcRows.SpawnAreas,
		EnemyPoolRows:          npcRows.EnemyPools,
		NPCDropRows:            npcRows.NPCDropProfiles,
		NPCAggroRows:           npcRows.NPCAggroProfiles,
		NPCLeashRows:           npcRows.NPCLeashProfiles,
		ScannerConfigRows:      scannerConfigRows,
		StarterConfigRows:      starterConfigRows,
		RoutePolicyRows:        routePolicyRows,
	}
	rows.Report = buildImportReport(source, rows)
	return rows, nil
}

func mapRoutePolicyRows() ([]content.SnapshotRow, error) {
	route := content.DefaultRouteContent()
	row, err := snapshotRow("route_policy", route)
	if err != nil {
		return nil, err
	}
	return []content.SnapshotRow{row}, nil
}

func mapScannerConfigRows(mapRows []content.SnapshotRow) ([]content.SnapshotRow, error) {
	if len(mapRows) == 0 {
		return nil, fmt.Errorf("scanner config map rows empty: %w", ErrMalformedDumpSQL)
	}
	maps := make([]mapDefinitionSnapshotData, 0, len(mapRows))
	for _, row := range mapRows {
		var mapped mapDefinitionSnapshotData
		if err := json.Unmarshal(row.DataJSON, &mapped); err != nil {
			return nil, fmt.Errorf("scanner config map row %q: %w", row.ContentID, err)
		}
		if mapped.MapID == "" {
			return nil, fmt.Errorf("scanner config map row %q missing map id: %w", row.ContentID, ErrMalformedDumpSQL)
		}
		maps = append(maps, mapped)
	}
	scanner := content.DefaultScannerContent()
	scanner.StaticSeed = []byte("kalaazu_scanner_seed_v1")
	scanner.CandidateOptions.ProfileVersion = "kalaazu_scanner_default_v1"
	scanner.MapProfiles = make([]content.ScannerMapProfile, 0, len(maps))
	for index, mapped := range maps {
		levelMax := 4
		if index == 0 {
			levelMax = 3
		}
		scanner.MapProfiles = append(scanner.MapProfiles, content.ScannerMapProfile{
			MapID:          mapped.MapID,
			ProfileVersion: fmt.Sprintf("kalaazu_scanner_%s_v1", normalizeIdentifier(mapped.PublicMapKey.String())),
			LevelMin:       1,
			LevelMax:       levelMax,
			Density:        1,
			SpawnBudget:    4 + index,
		})
	}
	row, err := snapshotRow("scanner_config", scanner)
	if err != nil {
		return nil, err
	}
	return []content.SnapshotRow{row}, nil
}

func mapStarterConfigRows(enemyPoolRows []content.SnapshotRow) ([]content.SnapshotRow, error) {
	if len(enemyPoolRows) == 0 {
		return nil, fmt.Errorf("starter config enemy pools empty: %w", ErrMalformedDumpSQL)
	}
	var pool struct {
		MapID       worldmaps.MapID       `json:"map_id"`
		EnemyPoolID worldmaps.EnemyPoolID `json:"enemy_pool_id"`
	}
	if err := json.Unmarshal(enemyPoolRows[0].DataJSON, &pool); err != nil {
		return nil, fmt.Errorf("starter config enemy pool row: %w", err)
	}
	if pool.MapID == "" || pool.EnemyPoolID == "" {
		return nil, fmt.Errorf("starter config enemy pool row missing map or pool id: %w", ErrMalformedDumpSQL)
	}
	starter := content.DefaultStarterContent()
	starter.BalanceProfileID = "kalaazu_default_seed_v1"
	starter.BalanceProfileNote = "Default content DB seed derived from Kalaazu starter reference rows; legacy starter ship id keeps existing loadout contracts while ship stats use Kalaazu Phoenix values."
	starter.WorldSeeds = []content.WorldSeedContent{{
		MapID:       pool.MapID,
		EnemyPoolID: pool.EnemyPoolID,
	}}
	row, err := snapshotRow("starter_config", starter)
	if err != nil {
		return nil, err
	}
	return []content.SnapshotRow{row}, nil
}

type defaultSourceRows struct {
	Maps       []DumpRow
	MapNPCs    []DumpRow
	NPCs       []DumpRow
	Items      []DumpRow
	Ships      []DumpRow
	MapPortals []DumpRow
}

func loadDefaultSourceRows(filesystem fs.FS) (defaultSourceRows, error) {
	maps, err := LoadDumpRows(filesystem, "testdata/maps.sql")
	if err != nil {
		return defaultSourceRows{}, err
	}
	mapNPCs, err := LoadDumpRows(filesystem, "testdata/maps_npcs.sql")
	if err != nil {
		return defaultSourceRows{}, err
	}
	npcs, err := LoadDumpRows(filesystem, "testdata/npcs.sql")
	if err != nil {
		return defaultSourceRows{}, err
	}
	items, err := LoadDumpRows(filesystem, "testdata/items.sql")
	if err != nil {
		return defaultSourceRows{}, err
	}
	shipsRows, err := LoadDumpRows(filesystem, "testdata/ships.sql")
	if err != nil {
		return defaultSourceRows{}, err
	}
	mapPortals, err := LoadDumpRows(filesystem, "testdata/maps_portals.sql")
	if err != nil {
		return defaultSourceRows{}, err
	}
	return defaultSourceRows{
		Maps:       maps,
		MapNPCs:    mapNPCs,
		NPCs:       npcs,
		Items:      items,
		Ships:      shipsRows,
		MapPortals: mapPortals,
	}, nil
}

func buildImportReport(source defaultSourceRows, rows DefaultRows) ImportReport {
	report := ImportReport{
		SourceRows: map[string]int{
			"maps":         len(source.Maps),
			"maps_npcs":    len(source.MapNPCs),
			"npcs":         len(source.NPCs),
			"items":        len(source.Items),
			"ships":        len(source.Ships),
			"maps_portals": len(source.MapPortals),
		},
		ImportedRows: map[content.ContentType]int{
			content.ContentTypeMap:                len(rows.MapRows),
			content.ContentTypeMapPortal:          len(rows.MapPortalRows),
			content.ContentTypeItem:               len(rows.ItemRows),
			content.ContentTypeModule:             len(rows.ModuleRows),
			content.ContentTypeShip:               len(rows.ShipRows),
			content.ContentTypeShopProduct:        len(rows.ShopProductRows),
			content.ContentTypeLootTable:          len(rows.LootTableRows),
			content.ContentTypeCraftRecipe:        len(rows.CraftRecipeRows),
			content.ContentTypeProductionBuilding: len(rows.ProductionBuildingRows),
			content.ContentTypeProductionRules:    len(rows.ProductionRuleRows),
			content.ContentTypeNPCTemplate:        len(rows.NPCTemplateRows),
			content.ContentTypeSpawnArea:          len(rows.SpawnAreaRows),
			content.ContentTypeEnemyPool:          len(rows.EnemyPoolRows),
			content.ContentTypeNPCDropProfile:     len(rows.NPCDropRows),
			content.ContentTypeNPCAggroProfile:    len(rows.NPCAggroRows),
			content.ContentTypeNPCLeashProfile:    len(rows.NPCLeashRows),
			content.ContentTypeScannerConfig:      len(rows.ScannerConfigRows),
			content.ContentTypeStarterConfig:      len(rows.StarterConfigRows),
			content.ContentTypeRoutePolicy:        len(rows.RoutePolicyRows),
		},
		UnsupportedItems: unsupportedItemCounts(source.Items),
	}
	return report
}

func unsupportedItemCounts(itemRows []DumpRow) map[string]int {
	counts := make(map[string]int)
	for _, row := range itemRows {
		source, err := decodeKalaazuItem(row)
		if err != nil {
			continue
		}
		if source.Category == 4 && source.Type != 14 && source.Type != 15 && source.Type != 16 && source.Type != 30 {
			counts["category_"+strconv.Itoa(source.Category)+"_type_"+strconv.Itoa(source.Type)]++
		}
	}
	return counts
}
