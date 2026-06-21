package maps

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gameproject/internal/game/world"
)

func TestStarterCatalogReturnsBoundedStarterSpawnAndProjection(t *testing.T) {
	catalog, err := StarterCatalog("world-1")
	if err != nil {
		t.Fatalf("StarterCatalog() error = %v, want nil", err)
	}

	definition, spawn, err := catalog.StarterDefinition()
	if err != nil {
		t.Fatalf("StarterDefinition() error = %v, want nil", err)
	}
	if definition.InternalMapID != StarterMapID || definition.PublicMapKey != "1-1" {
		t.Fatalf("starter map = %s/%s, want %s/1-1", definition.InternalMapID, definition.PublicMapKey, StarterMapID)
	}
	if definition.ZoneID != StarterMapID.ZoneID() {
		t.Fatalf("starter zone = %q, want internal map id", definition.ZoneID)
	}
	if spawn.SpawnID != StarterSpawnID || !definition.Bounds.Contains(spawn.Position) {
		t.Fatalf("starter spawn = %+v bounds=%+v, want in-bounds starter", spawn, definition.Bounds)
	}

	second, ok := catalog.ByPublicKey("1-2")
	if !ok || second.InternalMapID != "map_1_2" || second.ZoneID != second.InternalMapID.ZoneID() {
		t.Fatalf("1-2 lookup = %+v ok=%v, want second starter-adjacent map", second, ok)
	}

	definitions := catalog.Definitions()
	if len(definitions) != 2 || definitions[0].InternalMapID != StarterMapID || definitions[1].InternalMapID != "map_1_2" {
		t.Fatalf("Definitions() = %+v, want sorted starter map set", definitions)
	}

	projection, err := catalog.ClientProjection(StarterMapID)
	if err != nil {
		t.Fatalf("ClientProjection() error = %v, want nil", err)
	}
	if projection.MapKey != "1-1" || projection.PublicMapKey != "1-1" || projection.Bounds != ExactPlayableBounds() {
		t.Fatalf("projection = %+v, want 1-1 with exact bounds", projection)
	}
	if len(projection.VisiblePortals) != 1 || projection.VisiblePortals[0].PortalID != "east_gate" {
		t.Fatalf("projection portals = %+v, want visible starter route", projection.VisiblePortals)
	}
	raw := string(mustMarshalMapTest(t, projection))
	for _, forbidden := range []string{
		"internal_map_id",
		"map_id",
		"zone_id",
		"worker",
		"destination_map_id",
		"destination_spawn_id",
		"map_1_1",
		"map_1_2",
		"gameplay_seed",
		"scan_seed",
		"enemy_pool",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("client projection leaked %q in %s", forbidden, raw)
		}
	}
}

func TestCatalogValidationRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name string
		edit func([]MapDefinition) []MapDefinition
		want error
	}{
		{
			name: "non exact bounds",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].Bounds.MaxX = PlayableMaxCoordinate - 1
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "duplicate map ids",
			edit: func(definitions []MapDefinition) []MapDefinition {
				duplicate := definitions[1]
				duplicate.InternalMapID = definitions[0].InternalMapID
				duplicate.ZoneID = definitions[0].ZoneID
				definitions = append(definitions, duplicate)
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "duplicate public keys",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[1].PublicMapKey = definitions[0].PublicMapKey
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "zone does not equal map id",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].ZoneID = "zone-1"
				return definitions
			},
			want: ErrInvalidMapDefinition,
		},
		{
			name: "out of bounds spawn",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].SpawnPoints[0].Position = world.Vec2{X: -1, Y: 0}
				return definitions
			},
			want: ErrPositionOutOfBounds,
		},
		{
			name: "duplicate portal ids per source",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].Portals = append(definitions[0].Portals, definitions[0].Portals[0])
				return definitions
			},
			want: ErrInvalidCatalog,
		},
		{
			name: "missing destination map",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].Portals[0].DestinationMapID = "missing_map"
				return definitions
			},
			want: ErrMapNotFound,
		},
		{
			name: "missing destination spawn",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].Portals[0].DestinationSpawnID = "missing_spawn"
				return definitions
			},
			want: ErrSpawnNotFound,
		},
		{
			name: "out of bounds portal",
			edit: func(definitions []MapDefinition) []MapDefinition {
				definitions[0].Portals[0].SourcePosition = world.Vec2{X: PlayableMaxCoordinate + 1, Y: 5000}
				return definitions
			},
			want: ErrPositionOutOfBounds,
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

func testMapDefinitions() []MapDefinition {
	catalog, err := StarterCatalog("world-1")
	if err != nil {
		panic(err)
	}
	return catalog.Definitions()
}

func mustMarshalMapTest(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal(%T) error = %v", value, err)
	}
	return data
}
