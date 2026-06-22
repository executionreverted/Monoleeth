package worker

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

// EnemySpawnRecord is server-only map-worker state for one live or known NPC
// spawn row. It is intentionally not part of any client wire projection.
type EnemySpawnRecord struct {
	EntityID       world.EntityID
	EnemyPoolID    worldmaps.EnemyPoolID
	SpawnAreaID    worldmaps.SpawnAreaID
	NPCType        string
	Level          int
	StatTemplateID worldmaps.NPCStatTemplateID
	DropProfileID  worldmaps.NPCDropProfileID
	AggroProfileID worldmaps.NPCAggroProfileID
	LeashProfileID worldmaps.NPCLeashProfileID
	Position       world.Vec2
	Alive          bool
	SpawnedAt      time.Time
	DeadAt         time.Time
	NextRespawnAt  time.Time
}

// EnemySpawnSnapshot is a deterministic server-only copy of worker spawner
// state. It must not be serialized to clients.
type EnemySpawnSnapshot struct {
	Records          []EnemySpawnRecord
	PoolAliveCounts  map[worldmaps.EnemyPoolID]int
	InitializedPools []worldmaps.EnemyPoolID
	MapAliveCount    int
}

// InitializeEnemyPoolsCommand performs the Phase08B deterministic initial fill
// from enabled map enemy pools.
type InitializeEnemyPoolsCommand struct {
	Definition        worldmaps.MapDefinition
	EntityIDOverrides map[worldmaps.EnemyPoolID][]world.EntityID
}

func (command InitializeEnemyPoolsCommand) apply(worker *Worker) error {
	return worker.initializeEnemyPools(command.Definition, command.EntityIDOverrides)
}

// MarkEnemyKilledCommand records a spawner-backed NPC death in worker-owned
// lifecycle state and removes the live world entity if it is still present.
type MarkEnemyKilledCommand struct {
	Definition  worldmaps.MapDefinition
	NPCEntityID world.EntityID
	KilledAt    time.Time
}

func (command MarkEnemyKilledCommand) apply(worker *Worker) error {
	return worker.markEnemyKilled(command.Definition, command.NPCEntityID, command.KilledAt)
}

// EnemySpawnSnapshot returns clone-safe server-only spawner state.
func (worker *Worker) EnemySpawnSnapshot() EnemySpawnSnapshot {
	if worker.enemySpawner == nil {
		return EnemySpawnSnapshot{
			Records:          nil,
			PoolAliveCounts:  map[worldmaps.EnemyPoolID]int{},
			InitializedPools: nil,
			MapAliveCount:    0,
		}
	}
	return worker.enemySpawner.snapshot()
}

// EnemySpawnRecord returns a clone-safe server-only copy for entityID.
func (worker *Worker) EnemySpawnRecord(entityID world.EntityID) (EnemySpawnRecord, bool) {
	if worker.enemySpawner == nil {
		return EnemySpawnRecord{}, false
	}
	return worker.enemySpawner.record(entityID)
}

func (worker *Worker) initializeEnemyPools(definition worldmaps.MapDefinition, overrides map[worldmaps.EnemyPoolID][]world.EntityID) error {
	if err := worker.validateEnemyPoolDefinitionOwnership(definition); err != nil {
		return err
	}
	if worker.enemySpawner == nil {
		worker.enemySpawner = newEnemySpawnerState()
	}
	if len(definition.EnemyPools) == 0 {
		return nil
	}

	spawnAreas := spawnAreasByID(definition.SpawnAreas)
	statTemplates := statTemplatesByID(definition.NPCStatTemplates)
	now := worker.clock.Now()
	mapAliveCount := worker.enemySpawner.mapAliveCount()
	mapAliveCap, hasMapAliveCap := initialEnemyMapAliveCap(definition.EnemyPools)

	for _, pool := range definition.EnemyPools {
		if worker.enemySpawner.poolInitialized(pool.EnemyPoolID) {
			continue
		}
		if !pool.Enabled || pool.SpawnMode == worldmaps.SpawnModeDisabled {
			worker.enemySpawner.markPoolInitialized(pool.EnemyPoolID)
			continue
		}
		statTemplate, ok := statTemplates[pool.StatTemplateID]
		if !ok {
			return fmt.Errorf("enemy pool %q stat template %q: %w", pool.EnemyPoolID, pool.StatTemplateID, worldmaps.ErrInvalidCatalog)
		}

		poolAliveCount := worker.enemySpawner.poolAliveCount(pool.EnemyPoolID)
		for spawnIndex := 0; spawnIndex < pool.InitialAlive; spawnIndex++ {
			if poolAliveCount >= pool.PoolMaxAlive || (hasMapAliveCap && mapAliveCount >= mapAliveCap) {
				break
			}
			area, ok, err := selectInitialSpawnArea(definition, spawnAreas, pool, spawnIndex)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}

			entityID := enemySpawnEntityID(definition.InternalMapID, pool.EnemyPoolID, spawnIndex, overrides)
			if _, exists := worker.entities[entityID]; exists {
				return fmt.Errorf("enemy spawn entity %q: %w", entityID, ErrEntityAlreadyExists)
			}
			entity, err := world.NewEntity(definition.WorldID, definition.ZoneID, entityID, world.EntityTypeNPC, area.Center)
			if err != nil {
				return err
			}
			if err := worker.insertEntity(entity, statTemplate.Speed); err != nil {
				return err
			}

			record := EnemySpawnRecord{
				EntityID:       entityID,
				EnemyPoolID:    pool.EnemyPoolID,
				SpawnAreaID:    area.SpawnAreaID,
				NPCType:        pool.NPCType,
				Level:          pool.MinLevel,
				StatTemplateID: pool.StatTemplateID,
				DropProfileID:  pool.DropProfileID,
				AggroProfileID: pool.AggroProfileID,
				LeashProfileID: pool.LeashProfileID,
				Position:       area.Center,
				Alive:          true,
				SpawnedAt:      now,
			}
			worker.enemySpawner.add(record)
			poolAliveCount++
			mapAliveCount++
		}
		worker.enemySpawner.markPoolInitialized(pool.EnemyPoolID)
	}
	return nil
}

func (worker *Worker) markEnemyKilled(definition worldmaps.MapDefinition, entityID world.EntityID, killedAt time.Time) error {
	if err := worker.validateEnemyPoolDefinitionOwnership(definition); err != nil {
		return err
	}
	if worker.enemySpawner == nil {
		return fmt.Errorf("enemy spawn entity %q: %w", entityID, ErrUnknownEntity)
	}

	index, ok := worker.enemySpawner.byEntityID[entityID]
	if !ok {
		return fmt.Errorf("enemy spawn entity %q: %w", entityID, ErrUnknownEntity)
	}
	record := worker.enemySpawner.rows[index]
	if !record.Alive {
		worker.removeEntity(entityID)
		return nil
	}

	pools := enemyPoolsByID(definition.EnemyPools)
	pool, ok := pools[record.EnemyPoolID]
	if !ok {
		return fmt.Errorf("enemy pool %q: %w", record.EnemyPoolID, worldmaps.ErrInvalidCatalog)
	}
	if killedAt.IsZero() {
		killedAt = worker.clock.Now()
	}
	if entity, ok := worker.entities[entityID]; ok {
		record.Position = entity.Position
	}
	record.Alive = false
	record.DeadAt = killedAt
	record.NextRespawnAt = killedAt.Add(pool.KillRespawnDelay)
	worker.enemySpawner.rows[index] = record

	if worker.enemySpawner.aliveByPool[record.EnemyPoolID] > 0 {
		worker.enemySpawner.aliveByPool[record.EnemyPoolID]--
	} else {
		worker.enemySpawner.aliveByPool[record.EnemyPoolID] = 0
	}
	worker.removeEntity(entityID)
	return nil
}

func initialEnemyMapAliveCap(pools []worldmaps.MapEnemyPoolDefinition) (int, bool) {
	cap := 0
	hasCap := false
	for _, pool := range pools {
		if !pool.Enabled || pool.SpawnMode == worldmaps.SpawnModeDisabled {
			continue
		}
		if !hasCap || pool.MapMaxAlive < cap {
			cap = pool.MapMaxAlive
			hasCap = true
		}
	}
	return cap, hasCap
}

func (worker *Worker) validateEnemyPoolDefinitionOwnership(definition worldmaps.MapDefinition) error {
	if definition.WorldID != worker.worldID {
		return fmt.Errorf("enemy pools map %q world %q not owned by worker world %q: %w", definition.InternalMapID, definition.WorldID, worker.worldID, ErrInvalidWorkerConfig)
	}
	if definition.ZoneID != worker.zoneID {
		return fmt.Errorf("enemy pools map %q zone %q not owned by worker zone %q: %w", definition.InternalMapID, definition.ZoneID, worker.zoneID, ErrInvalidWorkerConfig)
	}
	if err := definition.InternalMapID.Validate(); err != nil {
		return err
	}
	if err := definition.Bounds.ValidateExactPlayable(); err != nil {
		return err
	}
	return nil
}

func spawnAreasByID(areas []worldmaps.MapSpawnAreaDefinition) map[worldmaps.SpawnAreaID]worldmaps.MapSpawnAreaDefinition {
	byID := make(map[worldmaps.SpawnAreaID]worldmaps.MapSpawnAreaDefinition, len(areas))
	for _, area := range areas {
		byID[area.SpawnAreaID] = area
	}
	return byID
}

func enemyPoolsByID(pools []worldmaps.MapEnemyPoolDefinition) map[worldmaps.EnemyPoolID]worldmaps.MapEnemyPoolDefinition {
	byID := make(map[worldmaps.EnemyPoolID]worldmaps.MapEnemyPoolDefinition, len(pools))
	for _, pool := range pools {
		byID[pool.EnemyPoolID] = pool
	}
	return byID
}

func statTemplatesByID(templates []worldmaps.NPCStatTemplate) map[worldmaps.NPCStatTemplateID]worldmaps.NPCStatTemplate {
	byID := make(map[worldmaps.NPCStatTemplateID]worldmaps.NPCStatTemplate, len(templates))
	for _, template := range templates {
		byID[template.StatTemplateID] = template
	}
	return byID
}

func selectInitialSpawnArea(
	definition worldmaps.MapDefinition,
	spawnAreas map[worldmaps.SpawnAreaID]worldmaps.MapSpawnAreaDefinition,
	pool worldmaps.MapEnemyPoolDefinition,
	spawnIndex int,
) (worldmaps.MapSpawnAreaDefinition, bool, error) {
	if len(pool.SpawnAreaIDs) == 0 {
		return worldmaps.MapSpawnAreaDefinition{}, false, fmt.Errorf("enemy pool %q spawn areas: %w", pool.EnemyPoolID, worldmaps.ErrInvalidMapDefinition)
	}
	for offset := 0; offset < len(pool.SpawnAreaIDs); offset++ {
		areaID := pool.SpawnAreaIDs[(spawnIndex+offset)%len(pool.SpawnAreaIDs)]
		area, ok := spawnAreas[areaID]
		if !ok {
			return worldmaps.MapSpawnAreaDefinition{}, false, fmt.Errorf("enemy pool %q spawn area %q: %w", pool.EnemyPoolID, areaID, worldmaps.ErrInvalidCatalog)
		}
		// Phase08B deterministic MVP: use the area center as the initial-fill
		// candidate. RNG/jittered candidate generation remains deferred.
		if validInitialSpawnCandidate(definition, area, area.Center) {
			return area, true, nil
		}
	}
	return worldmaps.MapSpawnAreaDefinition{}, false, nil
}

func validInitialSpawnCandidate(definition worldmaps.MapDefinition, area worldmaps.MapSpawnAreaDefinition, position world.Vec2) bool {
	if err := position.Validate(); err != nil {
		return false
	}
	if !definition.Bounds.Contains(position) {
		return false
	}
	if area.Shape == worldmaps.SpawnAreaShapeCircle && position.DistanceSquared(area.Center) > area.Radius*area.Radius {
		return false
	}
	if area.SafeZoneExcluded {
		if _, ok := definition.PVPBlockingSafeZoneAt(position); ok {
			return false
		}
	}
	if area.PortalExclusionRadius > 0 {
		exclusionRadiusSquared := area.PortalExclusionRadius * area.PortalExclusionRadius
		for _, portal := range definition.Portals {
			if portal.Visible && position.DistanceSquared(portal.SourcePosition) <= exclusionRadiusSquared {
				return false
			}
		}
	}
	return true
}

func enemySpawnEntityID(mapID worldmaps.MapID, poolID worldmaps.EnemyPoolID, spawnIndex int, overrides map[worldmaps.EnemyPoolID][]world.EntityID) world.EntityID {
	if len(overrides) > 0 {
		if ids := overrides[poolID]; spawnIndex < len(ids) && !ids[spawnIndex].IsZero() {
			return ids[spawnIndex]
		}
	}
	return world.EntityID(fmt.Sprintf(
		"entity_npc_%s_%s_%s_%03d",
		sanitizeEntityIDPart(mapID.String()),
		sanitizeEntityIDPart(poolID.String()),
		entityIDRawPartSuffix(mapID.String(), poolID.String()),
		spawnIndex+1,
	))
}

func sanitizeEntityIDPart(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer(" ", "_", ":", "_", "/", "_", "\\", "_")
	return replacer.Replace(value)
}

func entityIDRawPartSuffix(parts ...string) string {
	var raw strings.Builder
	for _, part := range parts {
		fmt.Fprintf(&raw, "%d:", len(part))
		raw.WriteString(part)
		raw.WriteByte('|')
	}
	return hex.EncodeToString([]byte(raw.String()))
}

type enemySpawnerState struct {
	rows             []EnemySpawnRecord
	byEntityID       map[world.EntityID]int
	aliveByPool      map[worldmaps.EnemyPoolID]int
	initializedPools map[worldmaps.EnemyPoolID]struct{}
}

func newEnemySpawnerState() *enemySpawnerState {
	return &enemySpawnerState{
		rows:             make([]EnemySpawnRecord, 0),
		byEntityID:       make(map[world.EntityID]int),
		aliveByPool:      make(map[worldmaps.EnemyPoolID]int),
		initializedPools: make(map[worldmaps.EnemyPoolID]struct{}),
	}
}

func (spawner *enemySpawnerState) add(record EnemySpawnRecord) {
	spawner.byEntityID[record.EntityID] = len(spawner.rows)
	spawner.rows = append(spawner.rows, record)
	if record.Alive {
		spawner.aliveByPool[record.EnemyPoolID]++
	}
}

func (spawner *enemySpawnerState) record(entityID world.EntityID) (EnemySpawnRecord, bool) {
	index, ok := spawner.byEntityID[entityID]
	if !ok {
		return EnemySpawnRecord{}, false
	}
	return spawner.rows[index], true
}

func (spawner *enemySpawnerState) snapshot() EnemySpawnSnapshot {
	records := append([]EnemySpawnRecord(nil), spawner.rows...)
	sort.Slice(records, func(i, j int) bool {
		return records[i].EntityID < records[j].EntityID
	})
	aliveCounts := make(map[worldmaps.EnemyPoolID]int, len(spawner.aliveByPool))
	for poolID, count := range spawner.aliveByPool {
		aliveCounts[poolID] = count
	}
	initializedPools := make([]worldmaps.EnemyPoolID, 0, len(spawner.initializedPools))
	for poolID := range spawner.initializedPools {
		initializedPools = append(initializedPools, poolID)
	}
	sort.Slice(initializedPools, func(i, j int) bool {
		return initializedPools[i] < initializedPools[j]
	})
	return EnemySpawnSnapshot{
		Records:          records,
		PoolAliveCounts:  aliveCounts,
		InitializedPools: initializedPools,
		MapAliveCount:    spawner.mapAliveCount(),
	}
}

func (spawner *enemySpawnerState) poolAliveCount(poolID worldmaps.EnemyPoolID) int {
	return spawner.aliveByPool[poolID]
}

func (spawner *enemySpawnerState) mapAliveCount() int {
	count := 0
	for _, record := range spawner.rows {
		if record.Alive {
			count++
		}
	}
	return count
}

func (spawner *enemySpawnerState) poolInitialized(poolID worldmaps.EnemyPoolID) bool {
	_, ok := spawner.initializedPools[poolID]
	return ok
}

func (spawner *enemySpawnerState) markPoolInitialized(poolID worldmaps.EnemyPoolID) {
	spawner.initializedPools[poolID] = struct{}{}
}
