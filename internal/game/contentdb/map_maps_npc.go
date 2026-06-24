package contentdb

import (
	"fmt"

	"gameproject/internal/game/content"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

type npcTemplateMapRow struct {
	MapID   worldmaps.MapID `json:"map_id"`
	NPCType string          `json:"npc_type"`
}

type spawnAreaMapRow struct {
	MapID       worldmaps.MapID       `json:"map_id"`
	SpawnAreaID worldmaps.SpawnAreaID `json:"spawn_area_id"`
}

type enemyPoolMapRow struct {
	MapID       worldmaps.MapID       `json:"map_id"`
	EnemyPoolID worldmaps.EnemyPoolID `json:"enemy_pool_id"`
	NPCType     string                `json:"npc_type"`
}

type npcDropProfileMapRow struct {
	MapID         worldmaps.MapID            `json:"map_id"`
	DropProfileID worldmaps.NPCDropProfileID `json:"drop_profile_id"`
	NPCType       string                     `json:"npc_type"`
	LootTableID   string                     `json:"loot_table_id"`
}

type npcAggroProfileMapRow struct {
	MapID          worldmaps.MapID             `json:"map_id"`
	AggroProfileID worldmaps.NPCAggroProfileID `json:"aggro_profile_id"`
}

type npcLeashProfileMapRow struct {
	MapID          worldmaps.MapID             `json:"map_id"`
	LeashProfileID worldmaps.NPCLeashProfileID `json:"leash_profile_id"`
}

type npcEventSpawnMapRow struct {
	MapID        worldmaps.MapID           `json:"map_id"`
	EventSpawnID worldmaps.NPCEventSpawnID `json:"event_spawn_id"`
}

func mapAndVerifyWorldMaps(snapshot content.Snapshot, worldID world.WorldID) (*worldmaps.Catalog, error) {
	mapCatalog, err := worldmaps.StarterCatalog(worldID)
	if err != nil {
		return nil, fmt.Errorf("starter map catalog: %w", err)
	}
	if err := verifyMapNPCSnapshotRows(snapshot, mapCatalog); err != nil {
		return nil, err
	}
	return mapCatalog, nil
}

func verifyMapNPCSnapshotRows(snapshot content.Snapshot, mapCatalog *worldmaps.Catalog) error {
	definitions := mapCatalog.Definitions()
	if err := verifyNPCStatTemplateRows(snapshot.NPCTemplates, definitions); err != nil {
		return err
	}
	if err := verifySpawnAreaRows(snapshot.SpawnAreas, definitions); err != nil {
		return err
	}
	if err := verifyEnemyPoolRows(snapshot.EnemyPools, definitions); err != nil {
		return err
	}
	if err := verifyNPCDropProfileRows(snapshot.NPCDropProfiles, definitions); err != nil {
		return err
	}
	if err := verifyNPCAggroProfileRows(snapshot.NPCAggroProfiles, definitions); err != nil {
		return err
	}
	if err := verifyNPCLeashProfileRows(snapshot.NPCLeashProfiles, definitions); err != nil {
		return err
	}
	return verifyNPCEventSpawnRows(snapshot.NPCEventSpawns, definitions)
}

func verifyNPCStatTemplateRows(rows []content.SnapshotRow, definitions []worldmaps.MapDefinition) error {
	expected := make(map[string]npcTemplateMapRow)
	for _, definition := range definitions {
		for _, template := range definition.NPCStatTemplates {
			expected[template.StatTemplateID.String()] = npcTemplateMapRow{
				MapID:   definition.InternalMapID,
				NPCType: template.NPCType,
			}
		}
	}
	return verifyMapRowGroup(content.ContentTypeNPCTemplate, rows, expected, nil)
}

func verifySpawnAreaRows(rows []content.SnapshotRow, definitions []worldmaps.MapDefinition) error {
	expected := make(map[string]spawnAreaMapRow)
	for _, definition := range definitions {
		for _, area := range definition.SpawnAreas {
			expected[qualifiedMapContentID(definition.InternalMapID, area.SpawnAreaID.String())] = spawnAreaMapRow{
				MapID:       definition.InternalMapID,
				SpawnAreaID: area.SpawnAreaID,
			}
		}
	}
	return verifyMapRowGroup(content.ContentTypeSpawnArea, rows, expected, func(row spawnAreaMapRow) string {
		return qualifiedMapContentID(row.MapID, row.SpawnAreaID.String())
	})
}

func verifyEnemyPoolRows(rows []content.SnapshotRow, definitions []worldmaps.MapDefinition) error {
	expected := make(map[string]enemyPoolMapRow)
	for _, definition := range definitions {
		for _, pool := range definition.EnemyPools {
			expected[qualifiedMapContentID(definition.InternalMapID, pool.EnemyPoolID.String())] = enemyPoolMapRow{
				MapID:       definition.InternalMapID,
				EnemyPoolID: pool.EnemyPoolID,
				NPCType:     pool.NPCType,
			}
		}
	}
	return verifyMapRowGroup(content.ContentTypeEnemyPool, rows, expected, func(row enemyPoolMapRow) string {
		return qualifiedMapContentID(row.MapID, row.EnemyPoolID.String())
	})
}

func verifyNPCDropProfileRows(rows []content.SnapshotRow, definitions []worldmaps.MapDefinition) error {
	expected := make(map[string]npcDropProfileMapRow)
	for _, definition := range definitions {
		for _, profile := range definition.NPCDropProfiles {
			expected[qualifiedMapContentID(definition.InternalMapID, profile.DropProfileID.String())] = npcDropProfileMapRow{
				MapID:         definition.InternalMapID,
				DropProfileID: profile.DropProfileID,
				NPCType:       profile.NPCType,
				LootTableID:   profile.LootTableID,
			}
		}
	}
	return verifyMapRowGroup(content.ContentTypeNPCDropProfile, rows, expected, func(row npcDropProfileMapRow) string {
		return qualifiedMapContentID(row.MapID, row.DropProfileID.String())
	})
}

func verifyNPCAggroProfileRows(rows []content.SnapshotRow, definitions []worldmaps.MapDefinition) error {
	expected := make(map[string]npcAggroProfileMapRow)
	for _, definition := range definitions {
		for _, profile := range definition.NPCAggroProfiles {
			expected[qualifiedMapContentID(definition.InternalMapID, profile.AggroProfileID.String())] = npcAggroProfileMapRow{
				MapID:          definition.InternalMapID,
				AggroProfileID: profile.AggroProfileID,
			}
		}
	}
	return verifyMapRowGroup(content.ContentTypeNPCAggroProfile, rows, expected, func(row npcAggroProfileMapRow) string {
		return qualifiedMapContentID(row.MapID, row.AggroProfileID.String())
	})
}

func verifyNPCLeashProfileRows(rows []content.SnapshotRow, definitions []worldmaps.MapDefinition) error {
	expected := make(map[string]npcLeashProfileMapRow)
	for _, definition := range definitions {
		for _, profile := range definition.NPCLeashProfiles {
			expected[qualifiedMapContentID(definition.InternalMapID, profile.LeashProfileID.String())] = npcLeashProfileMapRow{
				MapID:          definition.InternalMapID,
				LeashProfileID: profile.LeashProfileID,
			}
		}
	}
	return verifyMapRowGroup(content.ContentTypeNPCLeashProfile, rows, expected, func(row npcLeashProfileMapRow) string {
		return qualifiedMapContentID(row.MapID, row.LeashProfileID.String())
	})
}

func verifyNPCEventSpawnRows(rows []content.SnapshotRow, definitions []worldmaps.MapDefinition) error {
	expected := make(map[string]npcEventSpawnMapRow)
	for _, definition := range definitions {
		for _, eventSpawn := range definition.NPCEventSpawns {
			expected[qualifiedMapContentID(definition.InternalMapID, eventSpawn.EventSpawnID.String())] = npcEventSpawnMapRow{
				MapID:        definition.InternalMapID,
				EventSpawnID: eventSpawn.EventSpawnID,
			}
		}
	}
	return verifyMapRowGroup(content.ContentTypeNPCEventSpawn, rows, expected, func(row npcEventSpawnMapRow) string {
		return qualifiedMapContentID(row.MapID, row.EventSpawnID.String())
	})
}

func verifyMapRowGroup[T comparable](
	contentType content.ContentType,
	rows []content.SnapshotRow,
	expected map[string]T,
	decodedID func(T) string,
) error {
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		var decoded T
		if err := decodeSnapshotRow(contentType, row, &decoded); err != nil {
			return err
		}
		if decodedID != nil {
			if err := requireRowID(contentType, row, decodedID(decoded)); err != nil {
				return err
			}
		}
		contentID := string(row.ContentID)
		want, ok := expected[contentID]
		if !ok {
			return fmt.Errorf("%s row %q extra: %w", contentType, row.ContentID, content.ErrInvalidContentSnapshot)
		}
		if decoded != want {
			return fmt.Errorf("%s row %q mismatch: got %+v want %+v: %w", contentType, row.ContentID, decoded, want, content.ErrInvalidContentSnapshot)
		}
		seen[contentID] = struct{}{}
	}
	for contentID := range expected {
		if _, ok := seen[contentID]; !ok {
			return fmt.Errorf("%s row %q missing: %w", contentType, contentID, content.ErrInvalidContentSnapshot)
		}
	}
	return nil
}

func qualifiedMapContentID(mapID worldmaps.MapID, id string) string {
	return fmt.Sprintf("%s.%s", mapID, id)
}
