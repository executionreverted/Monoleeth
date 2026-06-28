package kalaazu

import (
	"io/fs"
	"strconv"

	"gameproject/internal/game/content"
)

type ImportReport struct {
	SourceRows       map[string]int              `json:"source_rows"`
	ImportedRows     map[content.ContentType]int `json:"imported_rows"`
	UnsupportedItems map[string]int              `json:"unsupported_items"`
}

type DefaultRows struct {
	MapRows         []content.SnapshotRow
	MapPortalRows   []content.SnapshotRow
	ItemRows        []content.SnapshotRow
	ModuleRows      []content.SnapshotRow
	ShipRows        []content.SnapshotRow
	ShopProductRows []content.SnapshotRow
	NPCTemplateRows []content.SnapshotRow
	SpawnAreaRows   []content.SnapshotRow
	EnemyPoolRows   []content.SnapshotRow
	NPCDropRows     []content.SnapshotRow
	NPCAggroRows    []content.SnapshotRow
	NPCLeashRows    []content.SnapshotRow
	Report          ImportReport
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
	npcRows, err := mapStarterNPCRows(source.Maps, source.MapNPCs, source.NPCs)
	if err != nil {
		return DefaultRows{}, err
	}
	rows := DefaultRows{
		MapRows:         mapRows.MapRows,
		MapPortalRows:   mapRows.PortalRows,
		ItemRows:        itemRows,
		ModuleRows:      moduleRows,
		ShipRows:        shipRows,
		ShopProductRows: shopRows,
		NPCTemplateRows: npcRows.NPCTemplates,
		SpawnAreaRows:   npcRows.SpawnAreas,
		EnemyPoolRows:   npcRows.EnemyPools,
		NPCDropRows:     npcRows.NPCDropProfiles,
		NPCAggroRows:    npcRows.NPCAggroProfiles,
		NPCLeashRows:    npcRows.NPCLeashProfiles,
	}
	rows.Report = buildImportReport(source, rows)
	return rows, nil
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
			content.ContentTypeMap:             len(rows.MapRows),
			content.ContentTypeMapPortal:       len(rows.MapPortalRows),
			content.ContentTypeItem:            len(rows.ItemRows),
			content.ContentTypeModule:          len(rows.ModuleRows),
			content.ContentTypeShip:            len(rows.ShipRows),
			content.ContentTypeShopProduct:     len(rows.ShopProductRows),
			content.ContentTypeNPCTemplate:     len(rows.NPCTemplateRows),
			content.ContentTypeSpawnArea:       len(rows.SpawnAreaRows),
			content.ContentTypeEnemyPool:       len(rows.EnemyPoolRows),
			content.ContentTypeNPCDropProfile:  len(rows.NPCDropRows),
			content.ContentTypeNPCAggroProfile: len(rows.NPCAggroRows),
			content.ContentTypeNPCLeashProfile: len(rows.NPCLeashRows),
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
		if source.Category == 4 && source.Type != 14 && source.Type != 16 && source.Type != 30 {
			counts["category_"+strconv.Itoa(source.Category)+"_type_"+strconv.Itoa(source.Type)]++
		}
	}
	return counts
}
