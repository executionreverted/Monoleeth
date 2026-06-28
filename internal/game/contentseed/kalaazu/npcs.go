package kalaazu

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"time"

	"gameproject/internal/game/content"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/visibility"
)

const (
	maxPoolAliveFromKalaazu = 12
	maxInitialAlive         = 4
	defaultWeaponRange      = 650
	defaultWeaponCooldown   = 2 * time.Second
	defaultAccuracy         = 0.78
	defaultLeashDistance    = 1800
)

type NPCRowsResult struct {
	NPCTemplates     []content.SnapshotRow
	SpawnAreas       []content.SnapshotRow
	EnemyPools       []content.SnapshotRow
	NPCDropProfiles  []content.SnapshotRow
	NPCAggroProfiles []content.SnapshotRow
	NPCLeashProfiles []content.SnapshotRow
}

func BuildStarterNPCRows(filesystem fs.FS) (NPCRowsResult, error) {
	mapRows, err := LoadDumpRows(filesystem, "testdata/maps.sql")
	if err != nil {
		return NPCRowsResult{}, err
	}
	mapsNPCsRows, err := LoadDumpRows(filesystem, "testdata/maps_npcs.sql")
	if err != nil {
		return NPCRowsResult{}, err
	}
	npcRows, err := LoadDumpRows(filesystem, "testdata/npcs.sql")
	if err != nil {
		return NPCRowsResult{}, err
	}
	return mapStarterNPCRows(mapRows, mapsNPCsRows, npcRows)
}

func mapStarterNPCRows(mapRows []DumpRow, mapsNPCsRows []DumpRow, npcRows []DumpRow) (NPCRowsResult, error) {
	mapsByKalaazuID := make(map[int]kalaazuMapSource)
	for _, row := range mapRows {
		source, err := decodeKalaazuMap(row)
		if err != nil {
			return NPCRowsResult{}, err
		}
		if _, ok := starterPublicMapKeys[source.PublicKey]; ok {
			mapsByKalaazuID[source.KalaazuID] = source
		}
	}
	npcsByID := make(map[int]kalaazuNPCSource)
	for _, row := range npcRows {
		source, err := decodeKalaazuNPC(row)
		if err != nil {
			return NPCRowsResult{}, err
		}
		npcsByID[source.KalaazuID] = source
	}

	assignments := make([]kalaazuMapNPCSource, 0)
	for _, row := range mapsNPCsRows {
		assignment, err := decodeKalaazuMapNPC(row)
		if err != nil {
			return NPCRowsResult{}, err
		}
		if _, ok := mapsByKalaazuID[assignment.MapID]; !ok {
			continue
		}
		assignments = append(assignments, assignment)
	}
	sort.Slice(assignments, func(i, j int) bool {
		if assignments[i].MapID == assignments[j].MapID {
			return assignments[i].NPCID < assignments[j].NPCID
		}
		return assignments[i].MapID < assignments[j].MapID
	})

	var result NPCRowsResult
	perMapIndex := make(map[worldmaps.MapID]int)
	for _, assignment := range assignments {
		mapSource := mapsByKalaazuID[assignment.MapID]
		npcSource, ok := npcsByID[assignment.NPCID]
		if !ok {
			return NPCRowsResult{}, fmt.Errorf("maps_npcs npc %d: %w", assignment.NPCID, ErrMalformedDumpSQL)
		}
		rowSet, err := mapNPCRowSet(mapSource, npcSource, assignment.Amount, perMapIndex[mapSource.MapID])
		if err != nil {
			return NPCRowsResult{}, err
		}
		perMapIndex[mapSource.MapID]++
		result.NPCTemplates = append(result.NPCTemplates, rowSet.Template)
		result.SpawnAreas = append(result.SpawnAreas, rowSet.SpawnArea)
		result.EnemyPools = append(result.EnemyPools, rowSet.EnemyPool)
		result.NPCDropProfiles = append(result.NPCDropProfiles, rowSet.DropProfile)
		result.NPCAggroProfiles = append(result.NPCAggroProfiles, rowSet.AggroProfile)
		result.NPCLeashProfiles = append(result.NPCLeashProfiles, rowSet.LeashProfile)
	}
	return result, nil
}

type kalaazuNPCSource struct {
	KalaazuID int
	Name      string
	NPCType   string
	Health    int
	Shield    int
	Damage    int
	Speed     int
	AI        int
}

func decodeKalaazuNPC(row DumpRow) (kalaazuNPCSource, error) {
	id, err := row.Int("id")
	if err != nil {
		return kalaazuNPCSource{}, err
	}
	name, err := row.String("name")
	if err != nil {
		return kalaazuNPCSource{}, err
	}
	health, err := row.Int("health")
	if err != nil {
		return kalaazuNPCSource{}, err
	}
	shield, err := row.Int("shield")
	if err != nil {
		return kalaazuNPCSource{}, err
	}
	damage, err := row.Int("damage")
	if err != nil {
		return kalaazuNPCSource{}, err
	}
	speed, err := row.Int("speed")
	if err != nil {
		return kalaazuNPCSource{}, err
	}
	ai, err := row.Int("ai")
	if err != nil {
		return kalaazuNPCSource{}, err
	}
	return kalaazuNPCSource{
		KalaazuID: id,
		Name:      name,
		NPCType:   normalizeNPCType(name),
		Health:    health,
		Shield:    shield,
		Damage:    damage,
		Speed:     speed,
		AI:        ai,
	}, nil
}

type kalaazuMapNPCSource struct {
	MapID  int
	NPCID  int
	Amount int
}

func decodeKalaazuMapNPC(row DumpRow) (kalaazuMapNPCSource, error) {
	mapID, err := row.Int("maps_id")
	if err != nil {
		return kalaazuMapNPCSource{}, err
	}
	npcID, err := row.Int("npcs_id")
	if err != nil {
		return kalaazuMapNPCSource{}, err
	}
	amount, err := row.Int("amount")
	if err != nil {
		return kalaazuMapNPCSource{}, err
	}
	return kalaazuMapNPCSource{MapID: mapID, NPCID: npcID, Amount: amount}, nil
}

type npcRowSet struct {
	Template     content.SnapshotRow
	SpawnArea    content.SnapshotRow
	EnemyPool    content.SnapshotRow
	DropProfile  content.SnapshotRow
	AggroProfile content.SnapshotRow
	LeashProfile content.SnapshotRow
}

func mapNPCRowSet(mapSource kalaazuMapSource, npcSource kalaazuNPCSource, amount int, index int) (npcRowSet, error) {
	prefix := fmt.Sprintf("%s.%s_%d", mapSource.MapID, npcSource.NPCType, npcSource.KalaazuID)
	templateID := worldmaps.NPCStatTemplateID(prefix + "_template")
	spawnAreaID := worldmaps.SpawnAreaID(prefix + "_area")
	poolID := worldmaps.EnemyPoolID(prefix + "_pool")
	dropProfileID := worldmaps.NPCDropProfileID(prefix + "_drop")
	aggroProfileID := worldmaps.NPCAggroProfileID(prefix + "_aggro")
	leashProfileID := worldmaps.NPCLeashProfileID(prefix + "_leash")

	template := npcTemplateSnapshotData{
		MapID:          mapSource.MapID,
		StatTemplateID: templateID,
		NPCType:        npcSource.NPCType,
		MinLevel:       1,
		MaxLevel:       1,
		LabelKey:       "npc." + npcSource.NPCType,
		HPMax:          float64(npcSource.Health),
		ShieldMax:      float64(npcSource.Shield),
		EnergyMax:      float64(maxInt(1, npcSource.Damage/4)),
		WeaponRange:    defaultWeaponRange,
		WeaponDamage:   float64(npcSource.Damage),
		WeaponCooldown: defaultWeaponCooldown,
		Accuracy:       defaultAccuracy,
		RadarSignature: visibility.SignatureForEntityType(world.EntityTypeNPC).Units(),
		Speed:          float64(npcSource.Speed),
		XPValue:        int64(maxInt(1, (npcSource.Health+npcSource.Shield)/10)),
	}
	area := spawnAreaSnapshotData{
		MapID:                 mapSource.MapID,
		SpawnAreaID:           spawnAreaID,
		Shape:                 worldmaps.SpawnAreaShapeCircle,
		Center:                spawnCenter(mapSource.Bounds, index),
		Radius:                spawnRadiusForAmount(amount),
		SafeZoneExcluded:      true,
		PortalExclusionRadius: 650,
	}
	pool := enemyPoolSnapshotData{
		MapID:            mapSource.MapID,
		EnemyPoolID:      poolID,
		NPCType:          npcSource.NPCType,
		MinLevel:         1,
		MaxLevel:         1,
		SpawnAreaIDs:     []worldmaps.SpawnAreaID{spawnAreaID},
		MapMaxAlive:      maxInt(1, amount),
		PoolMaxAlive:     minInt(maxInt(1, amount), maxPoolAliveFromKalaazu),
		InitialAlive:     minInt(minInt(maxInt(1, amount), maxPoolAliveFromKalaazu), maxInitialAlive),
		SpawnInterval:    20 * time.Second,
		KillRespawnDelay: 20 * time.Second,
		SpawnJitter:      2 * time.Second,
		SpawnMode:        worldmaps.SpawnModePeriodic,
		StatTemplateID:   templateID,
		DropProfileID:    dropProfileID,
		AggroProfileID:   aggroProfileID,
		LeashProfileID:   leashProfileID,
		Enabled:          true,
	}
	drop := npcDropProfileSnapshotData{
		MapID:         mapSource.MapID,
		DropProfileID: dropProfileID,
		NPCType:       npcSource.NPCType,
		MinLevel:      1,
		MaxLevel:      1,
		RiskBand:      mapSource.RiskBand(),
		LootTableID:   lootTableForRisk(mapSource.RiskBand()),
	}
	aggro := npcAggroProfileSnapshotData{
		MapID:                mapSource.MapID,
		AggroProfileID:       aggroProfileID,
		AggroRadius:          aggroRadiusForAI(npcSource.AI),
		AssistRadius:         assistRadiusForAI(npcSource.AI),
		TargetMemory:         targetMemoryForAI(npcSource.AI),
		SafeZoneAttackPolicy: "never",
	}
	leash := npcLeashProfileSnapshotData{
		MapID:          mapSource.MapID,
		LeashProfileID: leashProfileID,
		LeashDistance:  defaultLeashDistance,
		ResetOnBreak:   true,
	}

	templateRow, err := snapshotRow(templateID.String(), template)
	if err != nil {
		return npcRowSet{}, err
	}
	areaRow, err := snapshotRow(qualifiedMapContentID(mapSource.MapID, spawnAreaID.String()), area)
	if err != nil {
		return npcRowSet{}, err
	}
	poolRow, err := snapshotRow(qualifiedMapContentID(mapSource.MapID, poolID.String()), pool)
	if err != nil {
		return npcRowSet{}, err
	}
	dropRow, err := snapshotRow(qualifiedMapContentID(mapSource.MapID, dropProfileID.String()), drop)
	if err != nil {
		return npcRowSet{}, err
	}
	aggroRow, err := snapshotRow(qualifiedMapContentID(mapSource.MapID, aggroProfileID.String()), aggro)
	if err != nil {
		return npcRowSet{}, err
	}
	leashRow, err := snapshotRow(qualifiedMapContentID(mapSource.MapID, leashProfileID.String()), leash)
	if err != nil {
		return npcRowSet{}, err
	}
	return npcRowSet{Template: templateRow, SpawnArea: areaRow, EnemyPool: poolRow, DropProfile: dropRow, AggroProfile: aggroRow, LeashProfile: leashRow}, nil
}

var npcTypePattern = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeNPCType(name string) string {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	trimmed = strings.ReplaceAll(trimmed, "boss", "boss ")
	trimmed = strings.ReplaceAll(trimmed, "uber", "uber ")
	normalized := npcTypePattern.ReplaceAllString(trimmed, "_")
	normalized = strings.Trim(normalized, "_")
	if normalized == "" {
		return "npc"
	}
	return normalized
}

func spawnCenter(bounds worldmaps.Bounds, index int) world.Vec2 {
	width := bounds.MaxX - bounds.MinX
	height := bounds.MaxY - bounds.MinY
	columns := []float64{0.25, 0.5, 0.75}
	rows := []float64{0.32, 0.52, 0.72, 0.42}
	return world.Vec2{
		X: bounds.MinX + width*columns[index%len(columns)],
		Y: bounds.MinY + height*rows[(index/len(columns))%len(rows)],
	}
}

func spawnRadiusForAmount(amount int) float64 {
	switch {
	case amount >= 60:
		return 900
	case amount >= 30:
		return 760
	default:
		return 620
	}
}

func aggroRadiusForAI(ai int) float64 {
	if ai >= 2 {
		return 700
	}
	return 0
}

func assistRadiusForAI(ai int) float64 {
	if ai >= 2 {
		return 220
	}
	return 0
}

func targetMemoryForAI(ai int) time.Duration {
	if ai >= 2 {
		return 8 * time.Second
	}
	return 0
}

func lootTableForRisk(riskBand string) string {
	if riskBand == "medium" || riskBand == "high" {
		return "border_raider_salvage"
	}
	return "training_drone_salvage"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type npcTemplateSnapshotData struct {
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

type spawnAreaSnapshotData struct {
	MapID                 worldmaps.MapID          `json:"map_id"`
	SpawnAreaID           worldmaps.SpawnAreaID    `json:"spawn_area_id"`
	Shape                 worldmaps.SpawnAreaShape `json:"shape"`
	Center                world.Vec2               `json:"center"`
	Radius                float64                  `json:"radius"`
	SafeZoneExcluded      bool                     `json:"safe_zone_excluded"`
	PortalExclusionRadius float64                  `json:"portal_exclusion_radius"`
}

type enemyPoolSnapshotData struct {
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

type npcDropProfileSnapshotData struct {
	MapID         worldmaps.MapID            `json:"map_id"`
	DropProfileID worldmaps.NPCDropProfileID `json:"drop_profile_id"`
	NPCType       string                     `json:"npc_type"`
	MinLevel      int                        `json:"min_level"`
	MaxLevel      int                        `json:"max_level"`
	RiskBand      string                     `json:"risk_band"`
	LootTableID   string                     `json:"loot_table_id"`
}

type npcAggroProfileSnapshotData struct {
	MapID                worldmaps.MapID             `json:"map_id"`
	AggroProfileID       worldmaps.NPCAggroProfileID `json:"aggro_profile_id"`
	AggroRadius          float64                     `json:"aggro_radius"`
	AssistRadius         float64                     `json:"assist_radius"`
	TargetMemory         time.Duration               `json:"target_memory"`
	SafeZoneAttackPolicy string                      `json:"safe_zone_attack_policy"`
}

type npcLeashProfileSnapshotData struct {
	MapID          worldmaps.MapID             `json:"map_id"`
	LeashProfileID worldmaps.NPCLeashProfileID `json:"leash_profile_id"`
	LeashDistance  float64                     `json:"leash_distance"`
	ResetOnBreak   bool                        `json:"reset_on_break"`
}
