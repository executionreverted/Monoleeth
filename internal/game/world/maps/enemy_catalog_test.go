package maps

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

func TestStarterCatalogContainsStarterEnemyCatalogData(t *testing.T) {
	catalog, err := StarterCatalog("world-1")
	if err != nil {
		t.Fatalf("StarterCatalog() error = %v, want nil", err)
	}

	starter, ok := catalog.Get(StarterMapID)
	if !ok {
		t.Fatalf("starter map missing")
	}
	if len(starter.SpawnAreas) != 1 || starter.SpawnAreas[0].SpawnAreaID != "starter_training_drone_area" {
		t.Fatalf("starter spawn areas = %+v, want training drone area", starter.SpawnAreas)
	}
	area := starter.SpawnAreas[0]
	if area.Shape != SpawnAreaShapeCircle || area.Center != (world.Vec2{X: 800, Y: 400}) || area.Radius != 180 {
		t.Fatalf("starter spawn area = %+v, want circle at 800,400 radius 180", area)
	}
	if !area.SafeZoneExcluded {
		t.Fatalf("starter spawn area SafeZoneExcluded = false, want true")
	}

	if len(starter.EnemyPools) != 2 {
		t.Fatalf("starter enemy pools = %+v, want training drone pool plus disabled event pool", starter.EnemyPools)
	}
	pool := starter.EnemyPools[0]
	if pool.EnemyPoolID != "starter_training_drone_pool" || pool.NPCType != "training_drone" {
		t.Fatalf("starter pool = %+v, want training_drone pool", pool)
	}
	if pool.MapMaxAlive != 3 || pool.PoolMaxAlive != 1 || pool.InitialAlive != 1 {
		t.Fatalf("starter pool caps = %+v, want map=3 pool=1 initial=1", pool)
	}
	if pool.SpawnInterval != 30*time.Second || pool.KillRespawnDelay != 30*time.Second || pool.SpawnJitter != 0 {
		t.Fatalf("starter pool timing = %+v, want 30s/30s/0", pool)
	}
	if pool.SpawnMode != SpawnModePeriodic || !pool.Enabled {
		t.Fatalf("starter pool mode/enabled = %s/%v, want periodic enabled", pool.SpawnMode, pool.Enabled)
	}
	if len(pool.SpawnAreaIDs) != 1 || pool.SpawnAreaIDs[0] != area.SpawnAreaID {
		t.Fatalf("starter pool spawn refs = %+v, want %q", pool.SpawnAreaIDs, area.SpawnAreaID)
	}
	eventPool := starter.EnemyPools[1]
	if eventPool.EnemyPoolID != "starter_event_overseer_pool" ||
		eventPool.NPCType != "training_overseer" ||
		eventPool.SpawnMode != SpawnModeEventScheduled ||
		eventPool.InitialAlive != 0 ||
		!eventPool.Enabled {
		t.Fatalf("starter event pool = %+v, want enabled event-scheduled overseer pool with no initial fill", eventPool)
	}

	if len(starter.NPCStatTemplates) != 2 {
		t.Fatalf("starter stat templates = %+v, want training plus event templates", starter.NPCStatTemplates)
	}
	template := starter.NPCStatTemplates[0]
	if pool.StatTemplateID != template.StatTemplateID || template.NPCType != "training_drone" {
		t.Fatalf("starter stat template = %+v pool=%+v, want referenced training_drone template", template, pool)
	}
	if template.MinLevel != 1 || template.MaxLevel != 1 || template.HPMax != 30 ||
		template.ShieldMax != 0 || template.EnergyMax != 1 || template.WeaponRange != 1 ||
		template.Accuracy != 1 || template.RadarSignature != visibility.SignatureForEntityType(world.EntityTypeNPC).Units() ||
		template.Speed != 0 || template.XPValue < 0 {
		t.Fatalf("starter stat template = %+v, want current training actor behavior", template)
	}

	if len(starter.NPCDropProfiles) != 2 {
		t.Fatalf("starter drop profiles = %+v, want training plus event profiles", starter.NPCDropProfiles)
	}
	drop := starter.NPCDropProfiles[0]
	if pool.DropProfileID != drop.DropProfileID || drop.NPCType != "training_drone" || drop.LootTableID != "training_drone_salvage" {
		t.Fatalf("starter drop profile = %+v pool=%+v, want training_drone_salvage", drop, pool)
	}
	if len(starter.NPCAggroProfiles) != 1 || pool.AggroProfileID != starter.NPCAggroProfiles[0].AggroProfileID {
		t.Fatalf("starter aggro refs = %+v pool=%+v, want referenced profile", starter.NPCAggroProfiles, pool)
	}
	if len(starter.NPCLeashProfiles) != 1 || pool.LeashProfileID != starter.NPCLeashProfiles[0].LeashProfileID {
		t.Fatalf("starter leash refs = %+v pool=%+v, want referenced profile", starter.NPCLeashProfiles, pool)
	}
	if len(starter.NPCEventSpawns) != 1 {
		t.Fatalf("starter event spawns = %+v, want disabled overseer hook", starter.NPCEventSpawns)
	}
	eventSpawn := starter.NPCEventSpawns[0]
	if eventSpawn.EventSpawnID != "starter_disabled_overseer_event" ||
		eventSpawn.EnemyPoolID != eventPool.EnemyPoolID ||
		eventSpawn.DropProfileID != eventPool.DropProfileID ||
		eventSpawn.Enabled ||
		eventSpawn.StartsAfter != time.Minute ||
		eventSpawn.MaxAlive != 1 ||
		eventSpawn.MapPolicy != NPCEventSpawnMapPolicyCurrentMapOnly {
		t.Fatalf("starter event spawn = %+v eventPool=%+v, want disabled server-only overseer hook", eventSpawn, eventPool)
	}

	second, ok := catalog.ByPublicKey("1-2")
	if !ok {
		t.Fatalf("map 1-2 missing")
	}
	if len(second.SpawnAreas) != 1 || second.SpawnAreas[0].SpawnAreaID != "outer_ring_scout_drone_area" {
		t.Fatalf("map 1-2 spawn areas = %+v, want outer ring scout area", second.SpawnAreas)
	}
	secondArea := second.SpawnAreas[0]
	if secondArea.Shape != SpawnAreaShapeCircle || secondArea.Center != (world.Vec2{X: 1800, Y: 5400}) || secondArea.Radius != 220 {
		t.Fatalf("map 1-2 spawn area = %+v, want circle at 1800,5400 radius 220", secondArea)
	}
	if !secondArea.SafeZoneExcluded || secondArea.PortalExclusionRadius != 320 {
		t.Fatalf("map 1-2 spawn area safety = %+v, want safe-zone excluded with portal exclusion", secondArea)
	}
	if westGate, ok := second.PVPBlockingSafeZoneAt(secondArea.Center); ok {
		t.Fatalf("map 1-2 spawn center %+v overlaps safe zone %+v", secondArea.Center, westGate)
	}
	for _, portal := range second.Portals {
		if portal.Visible && secondArea.Center.DistanceSquared(portal.SourcePosition) <= secondArea.PortalExclusionRadius*secondArea.PortalExclusionRadius {
			t.Fatalf("map 1-2 spawn center %+v inside portal exclusion for %+v", secondArea.Center, portal)
		}
	}

	if len(second.EnemyPools) != 1 {
		t.Fatalf("map 1-2 enemy pools = %+v, want one scout drone pool", second.EnemyPools)
	}
	secondPool := second.EnemyPools[0]
	if secondPool.EnemyPoolID != "outer_ring_scout_drone_pool" || secondPool.NPCType != "outer_ring_scout_drone" {
		t.Fatalf("map 1-2 enemy pool = %+v, want outer ring scout drone pool", secondPool)
	}
	if secondPool.MapMaxAlive != 4 || secondPool.PoolMaxAlive != 2 || secondPool.InitialAlive != 1 {
		t.Fatalf("map 1-2 pool caps = %+v, want map=4 pool=2 initial=1", secondPool)
	}
	if secondPool.SpawnInterval != 45*time.Second || secondPool.KillRespawnDelay != 45*time.Second || secondPool.SpawnJitter != 0 {
		t.Fatalf("map 1-2 pool timing = %+v, want 45s/45s/0", secondPool)
	}
	if secondPool.SpawnMode != SpawnModePeriodic || !secondPool.Enabled {
		t.Fatalf("map 1-2 pool mode/enabled = %s/%v, want periodic enabled", secondPool.SpawnMode, secondPool.Enabled)
	}
	if len(secondPool.SpawnAreaIDs) != 1 || secondPool.SpawnAreaIDs[0] != secondArea.SpawnAreaID {
		t.Fatalf("map 1-2 pool spawn refs = %+v, want %q", secondPool.SpawnAreaIDs, secondArea.SpawnAreaID)
	}

	if len(second.NPCStatTemplates) != 1 || secondPool.StatTemplateID != second.NPCStatTemplates[0].StatTemplateID {
		t.Fatalf("map 1-2 stat template refs = %+v pool=%+v, want referenced template", second.NPCStatTemplates, secondPool)
	}
	secondTemplate := second.NPCStatTemplates[0]
	if secondTemplate.NPCType != "outer_ring_scout_drone" ||
		secondTemplate.MinLevel != 1 ||
		secondTemplate.MaxLevel != 1 ||
		secondTemplate.HPMax != 36 ||
		secondTemplate.ShieldMax != 4 ||
		secondTemplate.EnergyMax != 2 ||
		secondTemplate.WeaponRange != 1 ||
		secondTemplate.Accuracy != 1 ||
		secondTemplate.RadarSignature != visibility.SignatureForEntityType(world.EntityTypeNPC).Units() ||
		secondTemplate.Speed != 0 ||
		secondTemplate.XPValue != 0 {
		t.Fatalf("map 1-2 stat template = %+v, want low-risk scout behavior", secondTemplate)
	}

	if len(second.NPCDropProfiles) != 1 || secondPool.DropProfileID != second.NPCDropProfiles[0].DropProfileID {
		t.Fatalf("map 1-2 drop profile refs = %+v pool=%+v, want referenced profile", second.NPCDropProfiles, secondPool)
	}
	secondDrop := second.NPCDropProfiles[0]
	if secondDrop.NPCType != "outer_ring_scout_drone" || secondDrop.RiskBand != "low" || secondDrop.LootTableID != "training_drone_salvage" {
		t.Fatalf("map 1-2 drop profile = %+v, want low-risk profile using existing salvage table", secondDrop)
	}
	if len(second.NPCAggroProfiles) != 1 || secondPool.AggroProfileID != second.NPCAggroProfiles[0].AggroProfileID {
		t.Fatalf("map 1-2 aggro refs = %+v pool=%+v, want referenced profile", second.NPCAggroProfiles, secondPool)
	}
	if len(second.NPCLeashProfiles) != 1 || secondPool.LeashProfileID != second.NPCLeashProfiles[0].LeashProfileID {
		t.Fatalf("map 1-2 leash refs = %+v pool=%+v, want referenced profile", second.NPCLeashProfiles, secondPool)
	}
	if len(second.NPCEventSpawns) != 0 {
		t.Fatalf("map 1-2 event spawns = %+v, want no event hooks in this seed", second.NPCEventSpawns)
	}
}

func TestEnemyCatalogValidationRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name string
		edit func([]MapDefinition) []MapDefinition
		want error
	}{
		{
			name: "duplicate enemy pool id",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].EnemyPools = append(definitions[0].EnemyPools, definitions[0].EnemyPools[0])
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "out of bounds spawn area",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].SpawnAreas[0].Center = world.Vec2{X: PlayableMaxCoordinate - 10, Y: 400}
				definitions[0].SpawnAreas[0].Radius = 20
				return definitions
			},
			want: ErrPositionOutOfBounds,
		},
		{
			name: "spawn area overlaps safe zone when excluded",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].SpawnAreas[0].Center = world.Vec2{X: 200, Y: 200}
				definitions[0].SpawnAreas[0].Radius = 180
				definitions[0].SpawnAreas[0].SafeZoneExcluded = true
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "unknown spawn area ref",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].EnemyPools[0].SpawnAreaIDs = []SpawnAreaID{"missing_spawn_area"}
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "invalid caps",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].EnemyPools[0].PoolMaxAlive = definitions[0].EnemyPools[0].MapMaxAlive + 1
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "invalid timing",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].EnemyPools[0].SpawnInterval = 0
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "invalid jitter",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].EnemyPools[0].SpawnJitter = definitions[0].EnemyPools[0].SpawnInterval + time.Second
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "unknown profile ref",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].EnemyPools[0].AggroProfileID = "missing_aggro_profile"
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "stat template npc type mismatch",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCStatTemplates[0].NPCType = "raider_drone"
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "stat template level band does not cover pool",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].EnemyPools[0].MaxLevel = 2
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "drop profile npc type mismatch",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCDropProfiles[0].NPCType = "raider_drone"
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "drop profile level band does not cover pool",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].EnemyPools[0].MaxLevel = 2
				definitions[0].NPCStatTemplates[0].MaxLevel = 2
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "drop profile risk band does not match map risk",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCDropProfiles[0].RiskBand = "medium"
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "invalid stat template accuracy",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCStatTemplates[0].Accuracy = 0
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "invalid stat template cooldown",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCStatTemplates[0].WeaponCooldown = 0
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "invalid spawn mode",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].EnemyPools[0].SpawnMode = "burst"
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "duplicate event spawn id",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCEventSpawns = append(definitions[0].NPCEventSpawns, definitions[0].NPCEventSpawns[0])
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "event spawn unknown pool ref",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCEventSpawns[0].EnemyPoolID = "missing_event_pool"
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "event spawn pool not event scheduled",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCEventSpawns[0].EnemyPoolID = definitions[0].EnemyPools[0].EnemyPoolID
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "event spawn invalid cap",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCEventSpawns[0].MaxAlive = definitions[0].EnemyPools[1].PoolMaxAlive + 1
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "event spawn invalid schedule",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCEventSpawns[0].StartsAfter = -time.Second
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "event spawn invalid map policy",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCEventSpawns[0].MapPolicy = "all_maps"
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "event spawn unknown drop profile",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCEventSpawns[0].DropProfileID = "missing_event_drop"
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "event spawn drop profile mismatch",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].NPCEventSpawns[0].DropProfileID = definitions[0].NPCDropProfiles[0].DropProfileID
				return definitions
			},
			want: ErrInvalidCatalog,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewCatalog(tc.edit(testMapDefinitions()), StarterMapID, StarterSpawnID)
			if !errors.Is(err, tc.want) {
				t.Fatalf("NewCatalog() error = %v, want %v", err, tc.want)
			}
		})
	}
}
