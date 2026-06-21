package visibility_test

import (
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

func TestSignatureForEntityTypeUsesContentDrivenValues(t *testing.T) {
	signatures := map[world.EntityType]visibility.EntitySignature{
		world.EntityTypePlayer:       visibility.SignatureForEntityType(world.EntityTypePlayer),
		world.EntityTypeNPC:          visibility.SignatureForEntityType(world.EntityTypeNPC),
		world.EntityTypeLoot:         visibility.SignatureForEntityType(world.EntityTypeLoot),
		world.EntityTypePlanetSignal: visibility.SignatureForEntityType(world.EntityTypePlanetSignal),
	}

	seen := make(map[visibility.EntitySignature]world.EntityType, len(signatures))
	for entityType, signature := range signatures {
		if signature == visibility.EntitySignature(1) || signature.Units() <= 1 {
			t.Fatalf("%s signature = %v, want content value above placeholder", entityType, signature)
		}
		if previous, exists := seen[signature]; exists {
			t.Fatalf("%s and %s share signature %v, want content-specific values", entityType, previous, signature)
		}
		seen[signature] = entityType
	}
}

func TestDetectionForHiddenEntityUsesServerDetectionStats(t *testing.T) {
	entity := testEntity(world.Vec2{})
	entity.Hidden = true
	entity.Signature = visibility.EntitySignaturePlayer
	entity.StealthScore = 125

	if visibility.DetectionForEntity(testDetectionViewer(100, stats.ExplorationStats{}), entity).Passed {
		t.Fatal("DetectionForEntity() passed with no server detection stats, want false")
	}
	if visibility.CanSendEntityToClient(testDetectionViewer(100, stats.ExplorationStats{}), entity) {
		t.Fatal("CanSendEntityToClient() = true with no server detection stats, want false")
	}

	viewer := testDetectionViewer(100, stats.ExplorationStats{DetectionPower: 40})
	result := visibility.DetectionForEntity(viewer, entity)
	if !result.Passed || result.Score <= result.Threshold {
		t.Fatalf("DetectionForEntity() = %+v, want passing score above threshold", result)
	}
	if !visibility.CanSendEntityToClient(viewer, entity) {
		t.Fatal("CanSendEntityToClient() = false with server detection stats, want true")
	}
}

func TestDetectionForHiddenEntityAppliesJammerResistance(t *testing.T) {
	entity := testEntity(world.Vec2{})
	entity.Hidden = true
	entity.Signature = visibility.EntitySignaturePlayer
	entity.StealthScore = 125
	entity.JammerStrength = 20

	viewerWithoutResistance := testDetectionViewer(100, stats.ExplorationStats{DetectionPower: 40})
	if visibility.DetectionForEntity(viewerWithoutResistance, entity).Passed {
		t.Fatal("DetectionForEntity() passed through jammer without resistance, want false")
	}

	viewerWithResistance := testDetectionViewer(100, stats.ExplorationStats{
		DetectionPower:   40,
		JammerResistance: 20,
	})
	if !visibility.DetectionForEntity(viewerWithResistance, entity).Passed {
		t.Fatal("DetectionForEntity() failed with matching jammer resistance, want true")
	}
}

func TestCanSendEntityToClientAllowsNormalEntityByRangeWithoutDetectionStats(t *testing.T) {
	entity := testEntity(world.Vec2{X: 60, Y: 80})
	entity.Signature = visibility.EntitySignatureLoot
	entity.StealthScore = 999
	entity.JammerStrength = 999

	if !visibility.CanSendEntityToClient(testDetectionViewer(100, stats.ExplorationStats{}), entity) {
		t.Fatal("CanSendEntityToClient() = false for non-hidden entity in range, want true")
	}
}

func TestViewerDetectionStatsComeFromServerStatSnapshot(t *testing.T) {
	viewerType := reflect.TypeOf(visibility.Viewer{})
	field, ok := viewerType.FieldByName("DetectionStats")
	if !ok {
		t.Fatal("Viewer missing DetectionStats field")
	}
	detectionType := reflect.TypeOf(visibility.DetectionStatsFromStatSnapshot(stats.StatSnapshot{}))
	if field.Type != detectionType {
		t.Fatalf("Viewer.DetectionStats type = %v, want %v from stat snapshot helper", field.Type, detectionType)
	}

	snapshot := stats.NewStatSnapshot(
		"player-1",
		"ship-1",
		1,
		stats.EffectiveStats{Exploration: stats.ExplorationStats{
			DetectionPower:        12,
			JammerResistance:      3,
			StealthDetectionBonus: 2,
		}},
		time.Unix(1, 0),
	)
	detection := visibility.DetectionStatsFromStatSnapshot(snapshot)
	if detection.DetectionPower() != 12 || detection.JammerResistance() != 3 || detection.StealthDetectionBonus() != 2 {
		t.Fatalf("DetectionStatsFromStatSnapshot() = power %v jammer %v stealth %v, want 12/3/2",
			detection.DetectionPower(),
			detection.JammerResistance(),
			detection.StealthDetectionBonus(),
		)
	}
}

func testDetectionViewer(radarRange float64, exploration stats.ExplorationStats) visibility.Viewer {
	exploration.RadarRange = radarRange
	snapshot := stats.NewStatSnapshot(
		"player-1",
		"ship-1",
		1,
		stats.EffectiveStats{Exploration: exploration},
		time.Unix(1, 0),
	)
	return visibility.Viewer{
		PlayerID:       "player-1",
		WorldID:        "world-1",
		ZoneID:         "zone-1",
		Position:       world.Vec2{},
		RadarRange:     visibility.RadarRangeFromStatSnapshot(snapshot),
		DetectionStats: visibility.DetectionStatsFromStatSnapshot(snapshot),
	}
}
