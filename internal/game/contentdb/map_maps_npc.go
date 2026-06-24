package contentdb

import (
	"fmt"
	"time"

	"gameproject/internal/game/content"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

type npcTemplateMapRow struct {
	MapID          worldmaps.MapID             `json:"map_id"`
	StatTemplateID worldmaps.NPCStatTemplateID `json:"stat_template_id"`
	NPCType        string                      `json:"npc_type"`
	MinLevel       int                         `json:"min_level"`
	MaxLevel       int                         `json:"max_level"`
	LabelKey       string                      `json:"label_key"`
	HPMax          float64                     `json:"hp_max"`
	ShieldMax      float64                     `json:"shield_max"`
	EnergyMax      float64                     `json:"energy_max"`
	WeaponRange    float64                     `json:"weapon_range"`
	WeaponDamage   float64                     `json:"weapon_damage"`
	WeaponCooldown time.Duration               `json:"weapon_cooldown"`
	Accuracy       float64                     `json:"accuracy"`
	RadarSignature float64                     `json:"radar_signature"`
	Speed          float64                     `json:"speed"`
	XPValue        int64                       `json:"xp_value"`
}

type spawnAreaMapRow struct {
	MapID                 worldmaps.MapID          `json:"map_id"`
	SpawnAreaID           worldmaps.SpawnAreaID    `json:"spawn_area_id"`
	Shape                 worldmaps.SpawnAreaShape `json:"shape"`
	Center                world.Vec2               `json:"center"`
	Radius                float64                  `json:"radius"`
	SafeZoneExcluded      bool                     `json:"safe_zone_excluded"`
	PortalExclusionRadius float64                  `json:"portal_exclusion_radius"`
}

type enemyPoolMapRow struct {
	MapID            worldmaps.MapID             `json:"map_id"`
	EnemyPoolID      worldmaps.EnemyPoolID       `json:"enemy_pool_id"`
	NPCType          string                      `json:"npc_type"`
	MinLevel         int                         `json:"min_level"`
	MaxLevel         int                         `json:"max_level"`
	SpawnAreaIDs     []worldmaps.SpawnAreaID     `json:"spawn_area_ids"`
	MapMaxAlive      int                         `json:"map_max_alive"`
	PoolMaxAlive     int                         `json:"pool_max_alive"`
	InitialAlive     int                         `json:"initial_alive"`
	SpawnInterval    time.Duration               `json:"spawn_interval"`
	KillRespawnDelay time.Duration               `json:"kill_respawn_delay"`
	SpawnJitter      time.Duration               `json:"spawn_jitter"`
	SpawnMode        worldmaps.SpawnMode         `json:"spawn_mode"`
	StatTemplateID   worldmaps.NPCStatTemplateID `json:"stat_template_id"`
	DropProfileID    worldmaps.NPCDropProfileID  `json:"drop_profile_id"`
	AggroProfileID   worldmaps.NPCAggroProfileID `json:"aggro_profile_id"`
	LeashProfileID   worldmaps.NPCLeashProfileID `json:"leash_profile_id"`
	Enabled          bool                        `json:"enabled"`
}

type npcDropProfileMapRow struct {
	MapID         worldmaps.MapID            `json:"map_id"`
	DropProfileID worldmaps.NPCDropProfileID `json:"drop_profile_id"`
	NPCType       string                     `json:"npc_type"`
	MinLevel      int                        `json:"min_level"`
	MaxLevel      int                        `json:"max_level"`
	RiskBand      string                     `json:"risk_band"`
	LootTableID   string                     `json:"loot_table_id"`
}

type npcAggroProfileMapRow struct {
	MapID                worldmaps.MapID             `json:"map_id"`
	AggroProfileID       worldmaps.NPCAggroProfileID `json:"aggro_profile_id"`
	AggroRadius          float64                     `json:"aggro_radius"`
	AssistRadius         float64                     `json:"assist_radius"`
	TargetMemory         time.Duration               `json:"target_memory"`
	SafeZoneAttackPolicy string                      `json:"safe_zone_attack_policy"`
}

type npcLeashProfileMapRow struct {
	MapID          worldmaps.MapID             `json:"map_id"`
	LeashProfileID worldmaps.NPCLeashProfileID `json:"leash_profile_id"`
	LeashDistance  float64                     `json:"leash_distance"`
	ResetOnBreak   bool                        `json:"reset_on_break"`
}

type npcEventSpawnMapRow struct {
	MapID         worldmaps.MapID                  `json:"map_id"`
	EventSpawnID  worldmaps.NPCEventSpawnID        `json:"event_spawn_id"`
	EnemyPoolID   worldmaps.EnemyPoolID            `json:"enemy_pool_id"`
	DropProfileID worldmaps.NPCDropProfileID       `json:"drop_profile_id"`
	Enabled       bool                             `json:"enabled"`
	StartsAfter   time.Duration                    `json:"starts_after"`
	MaxAlive      int                              `json:"max_alive"`
	MapPolicy     worldmaps.NPCEventSpawnMapPolicy `json:"map_policy"`
}

func mapAndVerifyWorldMaps(snapshot content.Snapshot, worldID world.WorldID) (*worldmaps.Catalog, error) {
	definitions, err := mapWorldMapDefinitions(snapshot, worldID)
	if err != nil {
		return nil, err
	}
	mapCatalog, err := worldmaps.NewCatalog(definitions, worldmaps.StarterMapID, worldmaps.StarterSpawnID)
	if err != nil {
		return nil, fmt.Errorf("cms map catalog: %w", err)
	}
	return mapCatalog, nil
}

func mapWorldMapDefinitions(snapshot content.Snapshot, worldID world.WorldID) ([]worldmaps.MapDefinition, error) {
	definitions := mapShellDefinitions(worldID)
	byMapID := make(map[worldmaps.MapID]*worldmaps.MapDefinition, len(definitions))
	for index := range definitions {
		byMapID[definitions[index].InternalMapID] = &definitions[index]
	}
	if err := mapNPCStatTemplateRows(snapshot.NPCTemplates, byMapID); err != nil {
		return nil, err
	}
	if err := mapSpawnAreaRows(snapshot.SpawnAreas, byMapID); err != nil {
		return nil, err
	}
	if err := mapEnemyPoolRows(snapshot.EnemyPools, byMapID); err != nil {
		return nil, err
	}
	if err := mapNPCDropProfileRows(snapshot.NPCDropProfiles, byMapID); err != nil {
		return nil, err
	}
	if err := mapNPCAggroProfileRows(snapshot.NPCAggroProfiles, byMapID); err != nil {
		return nil, err
	}
	if err := mapNPCLeashProfileRows(snapshot.NPCLeashProfiles, byMapID); err != nil {
		return nil, err
	}
	if err := mapNPCEventSpawnRows(snapshot.NPCEventSpawns, byMapID); err != nil {
		return nil, err
	}
	return definitions, nil
}

func mapNPCStatTemplateRows(rows []content.SnapshotRow, byMapID map[worldmaps.MapID]*worldmaps.MapDefinition) error {
	return mapMapRowGroup(content.ContentTypeNPCTemplate, rows, func(row content.SnapshotRow, decoded npcTemplateMapRow) error {
		if err := requireRowID(content.ContentTypeNPCTemplate, row, decoded.StatTemplateID.String()); err != nil {
			return err
		}
		definition, err := requireMapDefinition(content.ContentTypeNPCTemplate, row, byMapID, decoded.MapID)
		if err != nil {
			return err
		}
		definition.NPCStatTemplates = append(definition.NPCStatTemplates, worldmaps.NPCStatTemplate{
			StatTemplateID: decoded.StatTemplateID,
			NPCType:        decoded.NPCType,
			MinLevel:       decoded.MinLevel,
			MaxLevel:       decoded.MaxLevel,
			LabelKey:       decoded.LabelKey,
			HPMax:          decoded.HPMax,
			ShieldMax:      decoded.ShieldMax,
			EnergyMax:      decoded.EnergyMax,
			WeaponRange:    decoded.WeaponRange,
			WeaponDamage:   decoded.WeaponDamage,
			WeaponCooldown: decoded.WeaponCooldown,
			Accuracy:       decoded.Accuracy,
			RadarSignature: decoded.RadarSignature,
			Speed:          decoded.Speed,
			XPValue:        decoded.XPValue,
		})
		return nil
	})
}

func mapSpawnAreaRows(rows []content.SnapshotRow, byMapID map[worldmaps.MapID]*worldmaps.MapDefinition) error {
	return mapMapRowGroup(content.ContentTypeSpawnArea, rows, func(row content.SnapshotRow, decoded spawnAreaMapRow) error {
		if err := requireRowID(content.ContentTypeSpawnArea, row, qualifiedMapContentID(decoded.MapID, decoded.SpawnAreaID.String())); err != nil {
			return err
		}
		definition, err := requireMapDefinition(content.ContentTypeSpawnArea, row, byMapID, decoded.MapID)
		if err != nil {
			return err
		}
		definition.SpawnAreas = append(definition.SpawnAreas, worldmaps.MapSpawnAreaDefinition{
			SpawnAreaID:           decoded.SpawnAreaID,
			Shape:                 decoded.Shape,
			Center:                decoded.Center,
			Radius:                decoded.Radius,
			SafeZoneExcluded:      decoded.SafeZoneExcluded,
			PortalExclusionRadius: decoded.PortalExclusionRadius,
		})
		return nil
	})
}

func mapEnemyPoolRows(rows []content.SnapshotRow, byMapID map[worldmaps.MapID]*worldmaps.MapDefinition) error {
	return mapMapRowGroup(content.ContentTypeEnemyPool, rows, func(row content.SnapshotRow, decoded enemyPoolMapRow) error {
		if err := requireRowID(content.ContentTypeEnemyPool, row, qualifiedMapContentID(decoded.MapID, decoded.EnemyPoolID.String())); err != nil {
			return err
		}
		definition, err := requireMapDefinition(content.ContentTypeEnemyPool, row, byMapID, decoded.MapID)
		if err != nil {
			return err
		}
		definition.EnemyPools = append(definition.EnemyPools, worldmaps.MapEnemyPoolDefinition{
			EnemyPoolID:      decoded.EnemyPoolID,
			NPCType:          decoded.NPCType,
			MinLevel:         decoded.MinLevel,
			MaxLevel:         decoded.MaxLevel,
			SpawnAreaIDs:     append([]worldmaps.SpawnAreaID(nil), decoded.SpawnAreaIDs...),
			MapMaxAlive:      decoded.MapMaxAlive,
			PoolMaxAlive:     decoded.PoolMaxAlive,
			InitialAlive:     decoded.InitialAlive,
			SpawnInterval:    decoded.SpawnInterval,
			KillRespawnDelay: decoded.KillRespawnDelay,
			SpawnJitter:      decoded.SpawnJitter,
			SpawnMode:        decoded.SpawnMode,
			StatTemplateID:   decoded.StatTemplateID,
			DropProfileID:    decoded.DropProfileID,
			AggroProfileID:   decoded.AggroProfileID,
			LeashProfileID:   decoded.LeashProfileID,
			Enabled:          decoded.Enabled,
		})
		return nil
	})
}

func mapNPCDropProfileRows(rows []content.SnapshotRow, byMapID map[worldmaps.MapID]*worldmaps.MapDefinition) error {
	return mapMapRowGroup(content.ContentTypeNPCDropProfile, rows, func(row content.SnapshotRow, decoded npcDropProfileMapRow) error {
		if err := requireRowID(content.ContentTypeNPCDropProfile, row, qualifiedMapContentID(decoded.MapID, decoded.DropProfileID.String())); err != nil {
			return err
		}
		definition, err := requireMapDefinition(content.ContentTypeNPCDropProfile, row, byMapID, decoded.MapID)
		if err != nil {
			return err
		}
		definition.NPCDropProfiles = append(definition.NPCDropProfiles, worldmaps.NPCDropProfile{
			DropProfileID: decoded.DropProfileID,
			NPCType:       decoded.NPCType,
			MinLevel:      decoded.MinLevel,
			MaxLevel:      decoded.MaxLevel,
			RiskBand:      decoded.RiskBand,
			LootTableID:   decoded.LootTableID,
		})
		return nil
	})
}

func mapNPCAggroProfileRows(rows []content.SnapshotRow, byMapID map[worldmaps.MapID]*worldmaps.MapDefinition) error {
	return mapMapRowGroup(content.ContentTypeNPCAggroProfile, rows, func(row content.SnapshotRow, decoded npcAggroProfileMapRow) error {
		if err := requireRowID(content.ContentTypeNPCAggroProfile, row, qualifiedMapContentID(decoded.MapID, decoded.AggroProfileID.String())); err != nil {
			return err
		}
		definition, err := requireMapDefinition(content.ContentTypeNPCAggroProfile, row, byMapID, decoded.MapID)
		if err != nil {
			return err
		}
		definition.NPCAggroProfiles = append(definition.NPCAggroProfiles, worldmaps.NPCAggroProfile{
			AggroProfileID:       decoded.AggroProfileID,
			AggroRadius:          decoded.AggroRadius,
			AssistRadius:         decoded.AssistRadius,
			TargetMemory:         decoded.TargetMemory,
			SafeZoneAttackPolicy: decoded.SafeZoneAttackPolicy,
		})
		return nil
	})
}

func mapNPCLeashProfileRows(rows []content.SnapshotRow, byMapID map[worldmaps.MapID]*worldmaps.MapDefinition) error {
	return mapMapRowGroup(content.ContentTypeNPCLeashProfile, rows, func(row content.SnapshotRow, decoded npcLeashProfileMapRow) error {
		if err := requireRowID(content.ContentTypeNPCLeashProfile, row, qualifiedMapContentID(decoded.MapID, decoded.LeashProfileID.String())); err != nil {
			return err
		}
		definition, err := requireMapDefinition(content.ContentTypeNPCLeashProfile, row, byMapID, decoded.MapID)
		if err != nil {
			return err
		}
		definition.NPCLeashProfiles = append(definition.NPCLeashProfiles, worldmaps.NPCLeashProfile{
			LeashProfileID: decoded.LeashProfileID,
			LeashDistance:  decoded.LeashDistance,
			ResetOnBreak:   decoded.ResetOnBreak,
		})
		return nil
	})
}

func mapNPCEventSpawnRows(rows []content.SnapshotRow, byMapID map[worldmaps.MapID]*worldmaps.MapDefinition) error {
	return mapMapRowGroup(content.ContentTypeNPCEventSpawn, rows, func(row content.SnapshotRow, decoded npcEventSpawnMapRow) error {
		if err := requireRowID(content.ContentTypeNPCEventSpawn, row, qualifiedMapContentID(decoded.MapID, decoded.EventSpawnID.String())); err != nil {
			return err
		}
		definition, err := requireMapDefinition(content.ContentTypeNPCEventSpawn, row, byMapID, decoded.MapID)
		if err != nil {
			return err
		}
		definition.NPCEventSpawns = append(definition.NPCEventSpawns, worldmaps.NPCEventSpawnDefinition{
			EventSpawnID:  decoded.EventSpawnID,
			EnemyPoolID:   decoded.EnemyPoolID,
			DropProfileID: decoded.DropProfileID,
			Enabled:       decoded.Enabled,
			StartsAfter:   decoded.StartsAfter,
			MaxAlive:      decoded.MaxAlive,
			MapPolicy:     decoded.MapPolicy,
		})
		return nil
	})
}

func mapMapRowGroup[T any](contentType content.ContentType, rows []content.SnapshotRow, apply func(content.SnapshotRow, T) error) error {
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		var decoded T
		if err := decodeSnapshotRow(contentType, row, &decoded); err != nil {
			return err
		}
		if err := apply(row, decoded); err != nil {
			return err
		}
	}
	return nil
}

func requireMapDefinition(
	contentType content.ContentType,
	row content.SnapshotRow,
	byMapID map[worldmaps.MapID]*worldmaps.MapDefinition,
	mapID worldmaps.MapID,
) (*worldmaps.MapDefinition, error) {
	definition, ok := byMapID[mapID]
	if !ok {
		return nil, fmt.Errorf("%s row %q unknown map %q: %w", contentType, row.ContentID, mapID, content.ErrInvalidContentSnapshot)
	}
	return definition, nil
}

func mapShellDefinitions(worldID world.WorldID) []worldmaps.MapDefinition {
	bounds := worldmaps.ExactPlayableBounds()
	return []worldmaps.MapDefinition{
		{
			InternalMapID:  worldmaps.StarterMapID,
			PublicMapKey:   "1-1",
			WorldID:        worldID,
			ZoneID:         worldmaps.StarterMapID.ZoneID(),
			DisplayName:    "Origin Fringe",
			Region:         "Origin Belt",
			RiskBand:       "low",
			PVPPolicy:      "safe",
			VisualThemeKey: "starter-blue",
			Bounds:         bounds,
			SpawnPoints: []worldmaps.SpawnPointDefinition{
				{SpawnID: worldmaps.StarterSpawnID, Position: world.Vec2{X: 0, Y: 0}, Label: "Starter Dock"},
				{SpawnID: "east_gate", Position: world.Vec2{X: 9600, Y: 5000}, Label: "East Gate"},
			},
			SafeZones: []worldmaps.SafeZoneDefinition{
				{SafeZoneID: "starter_dock", Center: world.Vec2{X: 0, Y: 0}, Radius: 250, DisplayName: "Starter Dock", BlocksPVP: true, HangarActions: true},
				{SafeZoneID: "east_gate", Center: world.Vec2{X: 9600, Y: 5000}, Radius: 260, DisplayName: "East Gate", BlocksPVP: true, HangarActions: true},
			},
			Portals: []worldmaps.PortalDefinition{{
				PortalID:           "east_gate",
				SourceMapID:        worldmaps.StarterMapID,
				SourcePosition:     world.Vec2{X: 9800, Y: 5000},
				InteractionRadius:  180,
				DestinationMapID:   "map_1_2",
				DestinationSpawnID: "west_gate",
				DisplayName:        "East Gate",
				Visible:            true,
			}},
		},
		{
			InternalMapID:  "map_1_2",
			PublicMapKey:   "1-2",
			WorldID:        worldID,
			ZoneID:         worldmaps.MapID("map_1_2").ZoneID(),
			DisplayName:    "Outer Ring",
			Region:         "Origin Belt",
			RiskBand:       "low",
			PVPPolicy:      "pve",
			VisualThemeKey: "starter-violet",
			Bounds:         bounds,
			SpawnPoints: []worldmaps.SpawnPointDefinition{
				{SpawnID: "west_gate", Position: world.Vec2{X: 400, Y: 5000}, Label: "West Gate"},
			},
			SafeZones: []worldmaps.SafeZoneDefinition{
				{SafeZoneID: "west_gate", Center: world.Vec2{X: 400, Y: 5000}, Radius: 260, DisplayName: "West Gate", BlocksPVP: true, HangarActions: true},
			},
			Portals: []worldmaps.PortalDefinition{
				{
					PortalID:           "west_gate",
					SourceMapID:        "map_1_2",
					SourcePosition:     world.Vec2{X: 200, Y: 5000},
					InteractionRadius:  180,
					DestinationMapID:   worldmaps.StarterMapID,
					DestinationSpawnID: worldmaps.StarterSpawnID,
					DisplayName:        "West Gate",
					Visible:            true,
				},
				{
					PortalID:           "skirmish_gate",
					SourceMapID:        "map_1_2",
					SourcePosition:     world.Vec2{X: 9800, Y: 5000},
					InteractionRadius:  180,
					DestinationMapID:   "map_1_3",
					DestinationSpawnID: "west_gate",
					DisplayName:        "Skirmish Gate",
					Visible:            true,
				},
			},
		},
		{
			InternalMapID:  "map_1_3",
			PublicMapKey:   "1-3",
			WorldID:        worldID,
			ZoneID:         worldmaps.MapID("map_1_3").ZoneID(),
			DisplayName:    "Border Skirmish",
			Region:         "Origin Belt",
			RiskBand:       "medium",
			PVPPolicy:      "pvp",
			VisualThemeKey: "border-amber",
			Bounds:         bounds,
			SpawnPoints: []worldmaps.SpawnPointDefinition{
				{SpawnID: "west_gate", Position: world.Vec2{X: 400, Y: 5000}, Label: "West Gate"},
			},
			SafeZones: []worldmaps.SafeZoneDefinition{
				{SafeZoneID: "west_gate", Center: world.Vec2{X: 400, Y: 5000}, Radius: 260, DisplayName: "West Gate", BlocksPVP: true, HangarActions: true},
			},
			Portals: []worldmaps.PortalDefinition{{
				PortalID:           "west_gate",
				SourceMapID:        "map_1_3",
				SourcePosition:     world.Vec2{X: 200, Y: 5000},
				InteractionRadius:  180,
				DestinationMapID:   "map_1_2",
				DestinationSpawnID: "west_gate",
				DisplayName:        "West Gate",
				Visible:            true,
			}},
		},
	}
}

func qualifiedMapContentID(mapID worldmaps.MapID, id string) string {
	return fmt.Sprintf("%s.%s", mapID, id)
}
