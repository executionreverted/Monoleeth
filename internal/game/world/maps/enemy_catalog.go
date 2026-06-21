package maps

import (
	"fmt"
	"strings"
	"time"

	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

// EnemyPoolID identifies a hostile NPC pool scoped by map.
type EnemyPoolID string

// SpawnAreaID identifies an NPC spawn area scoped by map.
type SpawnAreaID string

// NPCStatTemplateID identifies an NPC stat template scoped by map.
type NPCStatTemplateID string

// NPCDropProfileID identifies an NPC drop profile scoped by map.
type NPCDropProfileID string

// NPCAggroProfileID identifies an NPC aggro profile scoped by map.
type NPCAggroProfileID string

// NPCLeashProfileID identifies an NPC leash profile scoped by map.
type NPCLeashProfileID string

func (id EnemyPoolID) String() string       { return string(id) }
func (id SpawnAreaID) String() string       { return string(id) }
func (id NPCStatTemplateID) String() string { return string(id) }
func (id NPCDropProfileID) String() string  { return string(id) }
func (id NPCAggroProfileID) String() string { return string(id) }
func (id NPCLeashProfileID) String() string { return string(id) }

func (id EnemyPoolID) Validate() error { return validateCatalogID("enemy pool", string(id)) }
func (id SpawnAreaID) Validate() error { return validateCatalogID("spawn area", string(id)) }
func (id NPCStatTemplateID) Validate() error {
	return validateCatalogID("npc stat template", string(id))
}
func (id NPCDropProfileID) Validate() error { return validateCatalogID("npc drop profile", string(id)) }
func (id NPCAggroProfileID) Validate() error {
	return validateCatalogID("npc aggro profile", string(id))
}
func (id NPCLeashProfileID) Validate() error {
	return validateCatalogID("npc leash profile", string(id))
}

// SpawnMode defines how a map enemy pool is replenished.
type SpawnMode string

const (
	SpawnModePeriodic        SpawnMode = "periodic"
	SpawnModeKillReplacement SpawnMode = "kill_replacement"
	SpawnModeDisabled        SpawnMode = "disabled"
)

func (mode SpawnMode) Validate() error {
	switch mode {
	case SpawnModePeriodic, SpawnModeKillReplacement, SpawnModeDisabled:
		return nil
	default:
		return fmt.Errorf("spawn mode %q: %w", mode, ErrInvalidMapDefinition)
	}
}

// SpawnAreaShape defines the supported geometry for a spawn area.
type SpawnAreaShape string

const (
	SpawnAreaShapeCircle SpawnAreaShape = "circle"
)

func (shape SpawnAreaShape) Validate() error {
	switch shape {
	case SpawnAreaShapeCircle:
		return nil
	default:
		return fmt.Errorf("spawn area shape %q: %w", shape, ErrInvalidMapDefinition)
	}
}

// MapSpawnAreaDefinition is server-owned catalog data for NPC spawn placement.
type MapSpawnAreaDefinition struct {
	SpawnAreaID           SpawnAreaID    `json:"-"`
	Shape                 SpawnAreaShape `json:"-"`
	Center                world.Vec2     `json:"-"`
	Radius                float64        `json:"-"`
	SafeZoneExcluded      bool           `json:"-"`
	PortalExclusionRadius float64        `json:"-"`
}

// MapEnemyPoolDefinition is server-owned catalog data for map hostile pools.
type MapEnemyPoolDefinition struct {
	EnemyPoolID      EnemyPoolID       `json:"-"`
	NPCType          string            `json:"-"`
	MinLevel         int               `json:"-"`
	MaxLevel         int               `json:"-"`
	SpawnAreaIDs     []SpawnAreaID     `json:"-"`
	MapMaxAlive      int               `json:"-"`
	PoolMaxAlive     int               `json:"-"`
	InitialAlive     int               `json:"-"`
	SpawnInterval    time.Duration     `json:"-"`
	KillRespawnDelay time.Duration     `json:"-"`
	SpawnJitter      time.Duration     `json:"-"`
	SpawnMode        SpawnMode         `json:"-"`
	StatTemplateID   NPCStatTemplateID `json:"-"`
	DropProfileID    NPCDropProfileID  `json:"-"`
	AggroProfileID   NPCAggroProfileID `json:"-"`
	LeashProfileID   NPCLeashProfileID `json:"-"`
	Enabled          bool              `json:"-"`
}

// NPCStatTemplate is server-owned catalog data for NPC combat projection.
type NPCStatTemplate struct {
	StatTemplateID NPCStatTemplateID `json:"-"`
	NPCType        string            `json:"-"`
	MinLevel       int               `json:"-"`
	MaxLevel       int               `json:"-"`
	LabelKey       string            `json:"-"`
	HPMax          float64           `json:"-"`
	ShieldMax      float64           `json:"-"`
	EnergyMax      float64           `json:"-"`
	WeaponRange    float64           `json:"-"`
	WeaponDamage   float64           `json:"-"`
	WeaponCooldown time.Duration     `json:"-"`
	Accuracy       float64           `json:"-"`
	RadarSignature float64           `json:"-"`
	Speed          float64           `json:"-"`
	XPValue        int64             `json:"-"`
}

// NPCDropProfile is server-owned catalog data for map/risk loot table choice.
type NPCDropProfile struct {
	DropProfileID NPCDropProfileID `json:"-"`
	NPCType       string           `json:"-"`
	MinLevel      int              `json:"-"`
	MaxLevel      int              `json:"-"`
	RiskBand      string           `json:"-"`
	LootTableID   string           `json:"-"`
}

// NPCAggroProfile is server-owned catalog data for NPC target acquisition.
type NPCAggroProfile struct {
	AggroProfileID       NPCAggroProfileID `json:"-"`
	AggroRadius          float64           `json:"-"`
	AssistRadius         float64           `json:"-"`
	TargetMemory         time.Duration     `json:"-"`
	SafeZoneAttackPolicy string            `json:"-"`
}

// NPCLeashProfile is server-owned catalog data for NPC leash/reset behavior.
type NPCLeashProfile struct {
	LeashProfileID NPCLeashProfileID `json:"-"`
	LeashDistance  float64           `json:"-"`
	ResetOnBreak   bool              `json:"-"`
}

func validateEnemyContent(definition MapDefinition) error {
	spawnAreas := make(map[SpawnAreaID]struct{}, len(definition.SpawnAreas))
	for _, area := range definition.SpawnAreas {
		if err := area.SpawnAreaID.Validate(); err != nil {
			return fmt.Errorf("map %q spawn area: %w", definition.InternalMapID, err)
		}
		if _, exists := spawnAreas[area.SpawnAreaID]; exists {
			return fmt.Errorf("map %q duplicate spawn area %q: %w", definition.InternalMapID, area.SpawnAreaID, ErrInvalidCatalog)
		}
		spawnAreas[area.SpawnAreaID] = struct{}{}
		if err := validateSpawnAreaDefinition(definition, area); err != nil {
			return fmt.Errorf("map %q spawn area %q: %w", definition.InternalMapID, area.SpawnAreaID, err)
		}
	}

	statTemplates := make(map[NPCStatTemplateID]NPCStatTemplate, len(definition.NPCStatTemplates))
	for _, template := range definition.NPCStatTemplates {
		if err := template.StatTemplateID.Validate(); err != nil {
			return fmt.Errorf("map %q npc stat template: %w", definition.InternalMapID, err)
		}
		if _, exists := statTemplates[template.StatTemplateID]; exists {
			return fmt.Errorf("map %q duplicate npc stat template %q: %w", definition.InternalMapID, template.StatTemplateID, ErrInvalidCatalog)
		}
		statTemplates[template.StatTemplateID] = template
		if err := validateNPCStatTemplate(template); err != nil {
			return fmt.Errorf("map %q npc stat template %q: %w", definition.InternalMapID, template.StatTemplateID, err)
		}
	}

	dropProfiles := make(map[NPCDropProfileID]NPCDropProfile, len(definition.NPCDropProfiles))
	for _, profile := range definition.NPCDropProfiles {
		if err := profile.DropProfileID.Validate(); err != nil {
			return fmt.Errorf("map %q npc drop profile: %w", definition.InternalMapID, err)
		}
		if _, exists := dropProfiles[profile.DropProfileID]; exists {
			return fmt.Errorf("map %q duplicate npc drop profile %q: %w", definition.InternalMapID, profile.DropProfileID, ErrInvalidCatalog)
		}
		dropProfiles[profile.DropProfileID] = profile
		if err := validateNPCDropProfile(profile); err != nil {
			return fmt.Errorf("map %q npc drop profile %q: %w", definition.InternalMapID, profile.DropProfileID, err)
		}
	}

	aggroProfiles := make(map[NPCAggroProfileID]struct{}, len(definition.NPCAggroProfiles))
	for _, profile := range definition.NPCAggroProfiles {
		if err := profile.AggroProfileID.Validate(); err != nil {
			return fmt.Errorf("map %q npc aggro profile: %w", definition.InternalMapID, err)
		}
		if _, exists := aggroProfiles[profile.AggroProfileID]; exists {
			return fmt.Errorf("map %q duplicate npc aggro profile %q: %w", definition.InternalMapID, profile.AggroProfileID, ErrInvalidCatalog)
		}
		aggroProfiles[profile.AggroProfileID] = struct{}{}
		if err := validateNPCAggroProfile(profile); err != nil {
			return fmt.Errorf("map %q npc aggro profile %q: %w", definition.InternalMapID, profile.AggroProfileID, err)
		}
	}

	leashProfiles := make(map[NPCLeashProfileID]struct{}, len(definition.NPCLeashProfiles))
	for _, profile := range definition.NPCLeashProfiles {
		if err := profile.LeashProfileID.Validate(); err != nil {
			return fmt.Errorf("map %q npc leash profile: %w", definition.InternalMapID, err)
		}
		if _, exists := leashProfiles[profile.LeashProfileID]; exists {
			return fmt.Errorf("map %q duplicate npc leash profile %q: %w", definition.InternalMapID, profile.LeashProfileID, ErrInvalidCatalog)
		}
		leashProfiles[profile.LeashProfileID] = struct{}{}
		if err := validateNPCLeashProfile(profile); err != nil {
			return fmt.Errorf("map %q npc leash profile %q: %w", definition.InternalMapID, profile.LeashProfileID, err)
		}
	}

	poolIDs := make(map[EnemyPoolID]struct{}, len(definition.EnemyPools))
	for _, pool := range definition.EnemyPools {
		if err := pool.EnemyPoolID.Validate(); err != nil {
			return fmt.Errorf("map %q enemy pool: %w", definition.InternalMapID, err)
		}
		if _, exists := poolIDs[pool.EnemyPoolID]; exists {
			return fmt.Errorf("map %q duplicate enemy pool %q: %w", definition.InternalMapID, pool.EnemyPoolID, ErrInvalidCatalog)
		}
		poolIDs[pool.EnemyPoolID] = struct{}{}
		if err := validateEnemyPoolDefinition(pool, definition.RiskBand, spawnAreas, statTemplates, dropProfiles, aggroProfiles, leashProfiles); err != nil {
			return fmt.Errorf("map %q enemy pool %q: %w", definition.InternalMapID, pool.EnemyPoolID, err)
		}
	}
	return nil
}

func validateSpawnAreaDefinition(definition MapDefinition, area MapSpawnAreaDefinition) error {
	if err := area.Shape.Validate(); err != nil {
		return err
	}
	if err := area.Center.Validate(); err != nil {
		return err
	}
	if area.Radius <= 0 || !isFinite(area.Radius) {
		return fmt.Errorf("radius %v: %w", area.Radius, ErrInvalidMapDefinition)
	}
	if !circleInsideBounds(definition.Bounds, area.Center, area.Radius) {
		return fmt.Errorf("circle center %+v radius %v: %w", area.Center, area.Radius, ErrPositionOutOfBounds)
	}
	if area.PortalExclusionRadius < 0 || !isFinite(area.PortalExclusionRadius) {
		return fmt.Errorf("portal exclusion radius %v: %w", area.PortalExclusionRadius, ErrInvalidMapDefinition)
	}
	if area.SafeZoneExcluded {
		for _, safeZone := range definition.SafeZones {
			if safeZone.BlocksPVP && circlesOverlap(area.Center, area.Radius, safeZone.Center, safeZone.Radius) {
				return fmt.Errorf("overlaps pvp-blocking safe zone %q: %w", safeZone.SafeZoneID, ErrInvalidMapDefinition)
			}
		}
	}
	return nil
}

func validateNPCStatTemplate(template NPCStatTemplate) error {
	if strings.TrimSpace(template.NPCType) == "" {
		return fmt.Errorf("npc type: %w", ErrInvalidMapDefinition)
	}
	if strings.TrimSpace(template.LabelKey) == "" {
		return fmt.Errorf("label key: %w", ErrInvalidMapDefinition)
	}
	if err := validateLevelBand(template.MinLevel, template.MaxLevel); err != nil {
		return err
	}
	if template.HPMax <= 0 || !isFinite(template.HPMax) {
		return fmt.Errorf("hp max %v: %w", template.HPMax, ErrInvalidMapDefinition)
	}
	if !finiteNonNegative(template.ShieldMax) || !finiteNonNegative(template.EnergyMax) || !finiteNonNegative(template.WeaponDamage) {
		return ErrInvalidMapDefinition
	}
	if template.WeaponRange <= 0 || !isFinite(template.WeaponRange) {
		return fmt.Errorf("weapon range %v: %w", template.WeaponRange, ErrInvalidMapDefinition)
	}
	if template.WeaponCooldown <= 0 {
		return fmt.Errorf("weapon cooldown %s: %w", template.WeaponCooldown, ErrInvalidMapDefinition)
	}
	if template.Accuracy <= 0 || template.Accuracy > 1 || !isFinite(template.Accuracy) {
		return fmt.Errorf("accuracy %v: %w", template.Accuracy, ErrInvalidMapDefinition)
	}
	if !finiteNonNegative(template.RadarSignature) || !finiteNonNegative(template.Speed) || template.XPValue < 0 {
		return ErrInvalidMapDefinition
	}
	return nil
}

func validateNPCDropProfile(profile NPCDropProfile) error {
	if strings.TrimSpace(profile.NPCType) == "" {
		return fmt.Errorf("npc type: %w", ErrInvalidMapDefinition)
	}
	if err := validateLevelBand(profile.MinLevel, profile.MaxLevel); err != nil {
		return err
	}
	if strings.TrimSpace(profile.RiskBand) == "" {
		return fmt.Errorf("risk band: %w", ErrInvalidMapDefinition)
	}
	if strings.TrimSpace(profile.LootTableID) == "" {
		return fmt.Errorf("loot table id: %w", ErrInvalidMapDefinition)
	}
	return nil
}

func validateNPCAggroProfile(profile NPCAggroProfile) error {
	if !finiteNonNegative(profile.AggroRadius) || !finiteNonNegative(profile.AssistRadius) {
		return ErrInvalidMapDefinition
	}
	if profile.TargetMemory < 0 {
		return fmt.Errorf("target memory %s: %w", profile.TargetMemory, ErrInvalidMapDefinition)
	}
	if strings.TrimSpace(profile.SafeZoneAttackPolicy) == "" {
		return fmt.Errorf("safe zone attack policy: %w", ErrInvalidMapDefinition)
	}
	return nil
}

func validateNPCLeashProfile(profile NPCLeashProfile) error {
	if profile.LeashDistance <= 0 || !isFinite(profile.LeashDistance) {
		return fmt.Errorf("leash distance %v: %w", profile.LeashDistance, ErrInvalidMapDefinition)
	}
	return nil
}

func validateEnemyPoolDefinition(
	pool MapEnemyPoolDefinition,
	mapRiskBand string,
	spawnAreas map[SpawnAreaID]struct{},
	statTemplates map[NPCStatTemplateID]NPCStatTemplate,
	dropProfiles map[NPCDropProfileID]NPCDropProfile,
	aggroProfiles map[NPCAggroProfileID]struct{},
	leashProfiles map[NPCLeashProfileID]struct{},
) error {
	if strings.TrimSpace(pool.NPCType) == "" {
		return fmt.Errorf("npc type: %w", ErrInvalidMapDefinition)
	}
	if err := validateLevelBand(pool.MinLevel, pool.MaxLevel); err != nil {
		return err
	}
	if len(pool.SpawnAreaIDs) == 0 {
		return fmt.Errorf("spawn area refs: %w", ErrInvalidMapDefinition)
	}
	for _, spawnAreaID := range pool.SpawnAreaIDs {
		if err := spawnAreaID.Validate(); err != nil {
			return err
		}
		if _, exists := spawnAreas[spawnAreaID]; !exists {
			return fmt.Errorf("spawn area %q: %w", spawnAreaID, ErrInvalidCatalog)
		}
	}
	if pool.MapMaxAlive <= 0 || pool.PoolMaxAlive <= 0 || pool.InitialAlive < 0 ||
		pool.InitialAlive > pool.PoolMaxAlive || pool.PoolMaxAlive > pool.MapMaxAlive {
		return fmt.Errorf("caps map=%d pool=%d initial=%d: %w", pool.MapMaxAlive, pool.PoolMaxAlive, pool.InitialAlive, ErrInvalidMapDefinition)
	}
	if pool.SpawnInterval <= 0 || pool.KillRespawnDelay <= 0 {
		return fmt.Errorf("spawn timing interval=%s kill_delay=%s: %w", pool.SpawnInterval, pool.KillRespawnDelay, ErrInvalidMapDefinition)
	}
	if pool.SpawnJitter < 0 || pool.SpawnJitter > pool.SpawnInterval {
		return fmt.Errorf("spawn jitter %s interval=%s: %w", pool.SpawnJitter, pool.SpawnInterval, ErrInvalidMapDefinition)
	}
	if err := pool.SpawnMode.Validate(); err != nil {
		return err
	}
	statTemplate, exists := statTemplates[pool.StatTemplateID]
	if !exists {
		return fmt.Errorf("stat template %q: %w", pool.StatTemplateID, ErrInvalidCatalog)
	}
	if err := validatePoolStatTemplateCompatibility(pool, statTemplate); err != nil {
		return err
	}
	dropProfile, exists := dropProfiles[pool.DropProfileID]
	if !exists {
		return fmt.Errorf("drop profile %q: %w", pool.DropProfileID, ErrInvalidCatalog)
	}
	if err := validatePoolDropProfileCompatibility(pool, dropProfile, mapRiskBand); err != nil {
		return err
	}
	if _, exists := aggroProfiles[pool.AggroProfileID]; !exists {
		return fmt.Errorf("aggro profile %q: %w", pool.AggroProfileID, ErrInvalidCatalog)
	}
	if _, exists := leashProfiles[pool.LeashProfileID]; !exists {
		return fmt.Errorf("leash profile %q: %w", pool.LeashProfileID, ErrInvalidCatalog)
	}
	return nil
}

func validatePoolStatTemplateCompatibility(pool MapEnemyPoolDefinition, template NPCStatTemplate) error {
	if template.NPCType != pool.NPCType {
		return fmt.Errorf("stat template %q npc type %q does not match pool npc type %q: %w", template.StatTemplateID, template.NPCType, pool.NPCType, ErrInvalidCatalog)
	}
	if !levelBandCovers(template.MinLevel, template.MaxLevel, pool.MinLevel, pool.MaxLevel) {
		return fmt.Errorf("stat template %q level band %d..%d does not cover pool level band %d..%d: %w", template.StatTemplateID, template.MinLevel, template.MaxLevel, pool.MinLevel, pool.MaxLevel, ErrInvalidCatalog)
	}
	return nil
}

func validatePoolDropProfileCompatibility(pool MapEnemyPoolDefinition, profile NPCDropProfile, mapRiskBand string) error {
	if profile.NPCType != pool.NPCType {
		return fmt.Errorf("drop profile %q npc type %q does not match pool npc type %q: %w", profile.DropProfileID, profile.NPCType, pool.NPCType, ErrInvalidCatalog)
	}
	if !levelBandCovers(profile.MinLevel, profile.MaxLevel, pool.MinLevel, pool.MaxLevel) {
		return fmt.Errorf("drop profile %q level band %d..%d does not cover pool level band %d..%d: %w", profile.DropProfileID, profile.MinLevel, profile.MaxLevel, pool.MinLevel, pool.MaxLevel, ErrInvalidCatalog)
	}
	if profile.RiskBand != mapRiskBand {
		return fmt.Errorf("drop profile %q risk band %q does not match map risk band %q: %w", profile.DropProfileID, profile.RiskBand, mapRiskBand, ErrInvalidCatalog)
	}
	return nil
}

func validateLevelBand(minLevel int, maxLevel int) error {
	if minLevel <= 0 || maxLevel <= 0 || minLevel > maxLevel {
		return fmt.Errorf("level band %d..%d: %w", minLevel, maxLevel, ErrInvalidMapDefinition)
	}
	return nil
}

func levelBandCovers(outerMin int, outerMax int, innerMin int, innerMax int) bool {
	return outerMin <= innerMin && outerMax >= innerMax
}

func circleInsideBounds(bounds Bounds, center world.Vec2, radius float64) bool {
	return center.X-radius >= bounds.MinX &&
		center.Y-radius >= bounds.MinY &&
		center.X+radius <= bounds.MaxX &&
		center.Y+radius <= bounds.MaxY
}

func circlesOverlap(a world.Vec2, aRadius float64, b world.Vec2, bRadius float64) bool {
	radius := aRadius + bRadius
	return a.DistanceSquared(b) <= radius*radius
}

func finiteNonNegative(value float64) bool {
	return isFinite(value) && value >= 0
}

func cloneEnemyPoolDefinitions(pools []MapEnemyPoolDefinition) []MapEnemyPoolDefinition {
	if len(pools) == 0 {
		return nil
	}
	cloned := make([]MapEnemyPoolDefinition, len(pools))
	copy(cloned, pools)
	for i := range cloned {
		cloned[i].SpawnAreaIDs = append([]SpawnAreaID(nil), cloned[i].SpawnAreaIDs...)
	}
	return cloned
}

func starterMapSpawnAreas() []MapSpawnAreaDefinition {
	return []MapSpawnAreaDefinition{{
		SpawnAreaID:           "starter_training_drone_area",
		Shape:                 SpawnAreaShapeCircle,
		Center:                world.Vec2{X: 800, Y: 400},
		Radius:                180,
		SafeZoneExcluded:      true,
		PortalExclusionRadius: 180,
	}}
}

func starterMapEnemyPools() []MapEnemyPoolDefinition {
	return []MapEnemyPoolDefinition{{
		EnemyPoolID:      "starter_training_drone_pool",
		NPCType:          "training_drone",
		MinLevel:         1,
		MaxLevel:         1,
		SpawnAreaIDs:     []SpawnAreaID{"starter_training_drone_area"},
		MapMaxAlive:      3,
		PoolMaxAlive:     1,
		InitialAlive:     1,
		SpawnInterval:    30 * time.Second,
		KillRespawnDelay: 30 * time.Second,
		SpawnJitter:      0,
		SpawnMode:        SpawnModePeriodic,
		StatTemplateID:   "training_drone_level_1",
		DropProfileID:    "training_drone_salvage",
		AggroProfileID:   "training_drone_passive",
		LeashProfileID:   "training_drone_stationary",
		Enabled:          true,
	}}
}

func starterMapNPCStatTemplates() []NPCStatTemplate {
	return []NPCStatTemplate{{
		StatTemplateID: "training_drone_level_1",
		NPCType:        "training_drone",
		MinLevel:       1,
		MaxLevel:       1,
		LabelKey:       "npc.training_drone",
		HPMax:          30,
		ShieldMax:      0,
		EnergyMax:      1,
		WeaponRange:    1,
		WeaponDamage:   0,
		WeaponCooldown: time.Second,
		Accuracy:       1,
		RadarSignature: visibility.SignatureForEntityType(world.EntityTypeNPC).Units(),
		Speed:          0,
		XPValue:        0,
	}}
}

func starterMapNPCDropProfiles() []NPCDropProfile {
	return []NPCDropProfile{{
		DropProfileID: "training_drone_salvage",
		NPCType:       "training_drone",
		MinLevel:      1,
		MaxLevel:      1,
		RiskBand:      "low",
		LootTableID:   "training_drone_salvage",
	}}
}

func starterMapNPCAggroProfiles() []NPCAggroProfile {
	return []NPCAggroProfile{{
		AggroProfileID:       "training_drone_passive",
		AggroRadius:          0,
		AssistRadius:         0,
		TargetMemory:         0,
		SafeZoneAttackPolicy: "never",
	}}
}

func starterMapNPCLeashProfiles() []NPCLeashProfile {
	return []NPCLeashProfile{{
		LeashProfileID: "training_drone_stationary",
		LeashDistance:  1,
		ResetOnBreak:   true,
	}}
}
