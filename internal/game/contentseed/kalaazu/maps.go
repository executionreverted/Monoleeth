package kalaazu

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"gameproject/internal/game/content"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const (
	starterRegion        = "Origin Belt"
	defaultPortalRadius  = 180
	defaultSafeZoneRange = 260
)

var starterPublicMapKeys = map[string]struct{}{
	"1-1": {},
	"1-2": {},
	"1-3": {},
}

type MapRowsResult struct {
	MapRows    []content.SnapshotRow
	PortalRows []content.SnapshotRow
}

func BuildStarterMapRows(filesystem fs.FS) (MapRowsResult, error) {
	mapRows, err := LoadDumpRows(filesystem, "testdata/maps.sql")
	if err != nil {
		return MapRowsResult{}, err
	}
	portalRows, err := LoadDumpRows(filesystem, "testdata/maps_portals.sql")
	if err != nil {
		return MapRowsResult{}, err
	}
	return mapStarterMapRows(mapRows, portalRows)
}

func mapStarterMapRows(mapRows []DumpRow, portalRows []DumpRow) (MapRowsResult, error) {
	mapsByKalaazuID := make(map[int]kalaazuMapSource)
	for _, row := range mapRows {
		source, err := decodeKalaazuMap(row)
		if err != nil {
			return MapRowsResult{}, err
		}
		if _, ok := starterPublicMapKeys[source.PublicKey]; !ok {
			continue
		}
		mapsByKalaazuID[source.KalaazuID] = source
	}
	if len(mapsByKalaazuID) != len(starterPublicMapKeys) {
		return MapRowsResult{}, fmt.Errorf("starter map rows=%d want=%d: %w", len(mapsByKalaazuID), len(starterPublicMapKeys), ErrMalformedDumpSQL)
	}

	spawnsByMap := map[worldmaps.MapID]map[worldmaps.SpawnID]spawnPointSnapshotData{}
	portals := make([]content.SnapshotRow, 0)
	for _, row := range portalRows {
		source, err := decodeKalaazuPortal(row)
		if err != nil {
			return MapRowsResult{}, err
		}
		sourceMap, ok := mapsByKalaazuID[source.SourceMapID]
		if !ok {
			continue
		}
		destinationMap, ok := mapsByKalaazuID[source.TargetMapID]
		if !ok {
			continue
		}
		destinationSpawnID := spawnIDForPosition(source.TargetPosition)
		ensureSpawn(spawnsByMap, destinationMap.MapID, destinationSpawnID, source.TargetPosition, "Portal Arrival")
		portalID := worldmaps.PortalID(fmt.Sprintf("portal_to_%s_%d_%d", strings.ReplaceAll(destinationMap.PublicKey, "-", "_"), int(source.Position.X), int(source.Position.Y)))
		rowData := mapPortalSnapshotData{
			PortalID:           portalID,
			SourceMapID:        sourceMap.MapID,
			SourcePosition:     source.Position,
			InteractionRadius:  defaultPortalRadius,
			DestinationMapID:   destinationMap.MapID,
			DestinationSpawnID: destinationSpawnID,
			DisplayName:        "Gate to " + destinationMap.PublicKey,
			Visible:            true,
		}
		row, err := snapshotRow(qualifiedMapContentID(sourceMap.MapID, portalID.String()), rowData)
		if err != nil {
			return MapRowsResult{}, err
		}
		portals = append(portals, row)
	}

	mapIDs := make([]int, 0, len(mapsByKalaazuID))
	for id := range mapsByKalaazuID {
		mapIDs = append(mapIDs, id)
	}
	sort.Ints(mapIDs)

	maps := make([]content.SnapshotRow, 0, len(mapIDs))
	for _, id := range mapIDs {
		source := mapsByKalaazuID[id]
		if source.MapID == worldmaps.StarterMapID {
			ensureSpawn(spawnsByMap, source.MapID, worldmaps.StarterSpawnID, world.Vec2{X: 2000, Y: 2000}, "Starter Dock")
		}
		rowData := mapDefinitionSnapshotData{
			MapID:          source.MapID,
			PublicMapKey:   worldmaps.PublicMapKey(source.PublicKey),
			DisplayName:    source.PublicKey,
			Region:         starterRegion,
			RiskBand:       source.RiskBand(),
			PVPPolicy:      source.PVPPolicy(),
			VisualThemeKey: source.VisualThemeKey(),
			Bounds:         source.Bounds,
			SpawnPoints:    sortedSpawnPoints(spawnsByMap[source.MapID]),
			SafeZones:      safeZonesForSpawns(spawnsByMap[source.MapID]),
		}
		row, err := snapshotRow(source.MapID.String(), rowData)
		if err != nil {
			return MapRowsResult{}, err
		}
		maps = append(maps, row)
	}
	sort.Slice(portals, func(i, j int) bool { return portals[i].ContentID < portals[j].ContentID })
	return MapRowsResult{MapRows: maps, PortalRows: portals}, nil
}

type kalaazuMapSource struct {
	KalaazuID int
	PublicKey string
	MapID     worldmaps.MapID
	IsPVP     bool
	IsStarter bool
	Bounds    worldmaps.Bounds
}

func (source kalaazuMapSource) RiskBand() string {
	if source.IsPVP {
		return "high"
	}
	if source.IsStarter {
		return "low"
	}
	return "medium"
}

func (source kalaazuMapSource) PVPPolicy() string {
	if source.IsPVP {
		return "pvp"
	}
	if source.PublicKey == "1-1" {
		return "safe"
	}
	return "pve"
}

func (source kalaazuMapSource) VisualThemeKey() string {
	switch source.PublicKey {
	case "1-1":
		return "starter-blue"
	case "1-2":
		return "starter-violet"
	case "1-3":
		return "border-amber"
	default:
		return "space-default"
	}
}

func decodeKalaazuMap(row DumpRow) (kalaazuMapSource, error) {
	id, err := row.Int("id")
	if err != nil {
		return kalaazuMapSource{}, err
	}
	name, err := row.String("name")
	if err != nil {
		return kalaazuMapSource{}, err
	}
	isPVP, err := row.Bool("is_pvp")
	if err != nil {
		return kalaazuMapSource{}, err
	}
	isStarter, err := row.Bool("is_starter")
	if err != nil {
		return kalaazuMapSource{}, err
	}
	limits, err := row.String("limits")
	if err != nil {
		return kalaazuMapSource{}, err
	}
	bounds, err := parseBounds(limits)
	if err != nil {
		return kalaazuMapSource{}, err
	}
	return kalaazuMapSource{
		KalaazuID: id,
		PublicKey: name,
		MapID:     publicMapKeyToMapID(name),
		IsPVP:     isPVP,
		IsStarter: isStarter,
		Bounds:    bounds,
	}, nil
}

type kalaazuPortalSource struct {
	SourceMapID    int
	Position       world.Vec2
	TargetPosition world.Vec2
	TargetMapID    int
}

func decodeKalaazuPortal(row DumpRow) (kalaazuPortalSource, error) {
	sourceMapID, err := row.Int("maps_id")
	if err != nil {
		return kalaazuPortalSource{}, err
	}
	position, err := parseVec2Column(row, "position")
	if err != nil {
		return kalaazuPortalSource{}, err
	}
	targetPosition, err := parseVec2Column(row, "target_position")
	if err != nil {
		return kalaazuPortalSource{}, err
	}
	targetMapID, err := row.Int("target_maps_id")
	if err != nil {
		return kalaazuPortalSource{}, err
	}
	return kalaazuPortalSource{
		SourceMapID:    sourceMapID,
		Position:       position,
		TargetPosition: targetPosition,
		TargetMapID:    targetMapID,
	}, nil
}

func parseVec2Column(row DumpRow, column string) (world.Vec2, error) {
	raw, err := row.String(column)
	if err != nil {
		return world.Vec2{}, err
	}
	return parseVec2(raw)
}

func parseBounds(raw string) (worldmaps.Bounds, error) {
	parts := strings.Split(raw, "|")
	if len(parts) != 2 {
		return worldmaps.Bounds{}, fmt.Errorf("bounds %q: %w", raw, ErrMalformedDumpSQL)
	}
	minimum, err := parseVec2(parts[0])
	if err != nil {
		return worldmaps.Bounds{}, err
	}
	maximum, err := parseVec2(parts[1])
	if err != nil {
		return worldmaps.Bounds{}, err
	}
	return worldmaps.Bounds{MinX: minimum.X, MinY: minimum.Y, MaxX: maximum.X, MaxY: maximum.Y}, nil
}

func parseVec2(raw string) (world.Vec2, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		return world.Vec2{}, fmt.Errorf("position %q: %w", raw, ErrMalformedDumpSQL)
	}
	x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return world.Vec2{}, fmt.Errorf("position %q x: %w", raw, err)
	}
	y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return world.Vec2{}, fmt.Errorf("position %q y: %w", raw, err)
	}
	return world.Vec2{X: x, Y: y}, nil
}

func publicMapKeyToMapID(key string) worldmaps.MapID {
	return worldmaps.MapID("map_" + strings.ReplaceAll(key, "-", "_"))
}

func spawnIDForPosition(position world.Vec2) worldmaps.SpawnID {
	return worldmaps.SpawnID(fmt.Sprintf("spawn_%d_%d", int(position.X), int(position.Y)))
}

func ensureSpawn(spawnsByMap map[worldmaps.MapID]map[worldmaps.SpawnID]spawnPointSnapshotData, mapID worldmaps.MapID, spawnID worldmaps.SpawnID, position world.Vec2, label string) {
	spawns := spawnsByMap[mapID]
	if spawns == nil {
		spawns = make(map[worldmaps.SpawnID]spawnPointSnapshotData)
		spawnsByMap[mapID] = spawns
	}
	if _, ok := spawns[spawnID]; ok {
		return
	}
	spawns[spawnID] = spawnPointSnapshotData{SpawnID: spawnID, Position: position, Label: label}
}

func sortedSpawnPoints(spawns map[worldmaps.SpawnID]spawnPointSnapshotData) []spawnPointSnapshotData {
	keys := make([]worldmaps.SpawnID, 0, len(spawns))
	for key := range spawns {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	result := make([]spawnPointSnapshotData, 0, len(keys))
	for _, key := range keys {
		result = append(result, spawns[key])
	}
	return result
}

func safeZonesForSpawns(spawns map[worldmaps.SpawnID]spawnPointSnapshotData) []safeZoneSnapshotData {
	points := sortedSpawnPoints(spawns)
	result := make([]safeZoneSnapshotData, 0, len(points))
	for _, spawn := range points {
		result = append(result, safeZoneSnapshotData{
			SafeZoneID:    worldmaps.SafeZoneID(spawn.SpawnID),
			Center:        spawn.Position,
			Radius:        defaultSafeZoneRange,
			DisplayName:   spawn.Label,
			BlocksPVP:     true,
			HangarActions: true,
		})
	}
	return result
}

func snapshotRow(contentID string, data any) (content.SnapshotRow, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return content.SnapshotRow{}, err
	}
	return content.SnapshotRow{ContentID: content.ContentID(contentID), Enabled: true, DataJSON: raw}, nil
}

func qualifiedMapContentID(mapID worldmaps.MapID, id string) string {
	return fmt.Sprintf("%s.%s", mapID, id)
}

type mapDefinitionSnapshotData struct {
	MapID          worldmaps.MapID          `json:"map_id"`
	PublicMapKey   worldmaps.PublicMapKey   `json:"public_map_key"`
	DisplayName    string                   `json:"display_name"`
	Region         string                   `json:"region"`
	RiskBand       string                   `json:"risk_band"`
	PVPPolicy      string                   `json:"pvp_policy"`
	VisualThemeKey string                   `json:"visual_theme_key"`
	Bounds         worldmaps.Bounds         `json:"bounds"`
	SpawnPoints    []spawnPointSnapshotData `json:"spawn_points"`
	SafeZones      []safeZoneSnapshotData   `json:"safe_zones"`
}

type spawnPointSnapshotData struct {
	SpawnID  worldmaps.SpawnID `json:"spawn_id"`
	Position world.Vec2        `json:"position"`
	Label    string            `json:"label"`
}

type safeZoneSnapshotData struct {
	SafeZoneID    worldmaps.SafeZoneID `json:"safe_zone_id"`
	Center        world.Vec2           `json:"center"`
	Radius        float64              `json:"radius"`
	DisplayName   string               `json:"display_name"`
	BlocksPVP     bool                 `json:"blocks_pvp"`
	HangarActions bool                 `json:"hangar_actions"`
}

type mapPortalSnapshotData struct {
	PortalID           worldmaps.PortalID `json:"portal_id"`
	SourceMapID        worldmaps.MapID    `json:"source_map_id"`
	SourcePosition     world.Vec2         `json:"source_position"`
	InteractionRadius  float64            `json:"interaction_radius"`
	DestinationMapID   worldmaps.MapID    `json:"destination_map_id"`
	DestinationSpawnID worldmaps.SpawnID  `json:"destination_spawn_id"`
	DisplayName        string             `json:"display_name"`
	Visible            bool               `json:"visible"`
}
