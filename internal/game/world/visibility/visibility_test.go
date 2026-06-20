package visibility_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

func TestCanSendEntityToClientRejectsHiddenEntity(t *testing.T) {
	viewer := testViewer(100)
	entity := testEntity(world.Vec2{X: 10, Y: 0})
	entity.Hidden = true

	if visibility.CanSendEntityToClient(viewer, entity) {
		t.Fatal("CanSendEntityToClient() = true, want false for hidden entity")
	}
}

func TestCanSendEntityToClientAllowsHiddenSelfEntity(t *testing.T) {
	viewer := testViewer(100)
	viewer.PlayerID = "player-1"
	entity := testEntity(world.Vec2{X: 10, Y: 0})
	entity.PlayerID = "player-1"
	entity.Hidden = true

	if !visibility.CanSendEntityToClient(viewer, entity) {
		t.Fatal("CanSendEntityToClient() = false, want true for hidden self entity")
	}
}

func TestCanSendEntityToClientAllowsHiddenPlayerWithActiveWitness(t *testing.T) {
	now := time.Unix(100, 0)
	viewer := testViewer(100)
	viewer.PlayerID = "viewer-player"
	viewer.ObservedAt = now
	viewer.Witnesses = []visibility.Witness{{
		TargetPlayerID: "target-player",
		ExpiresAt:      now.Add(15 * time.Minute),
	}}
	entity := testEntity(world.Vec2{X: 10, Y: 0})
	entity.PlayerID = "target-player"
	entity.Hidden = true

	if !visibility.CanSendEntityToClient(viewer, entity) {
		t.Fatal("CanSendEntityToClient() = false, want true for witnessed hidden player")
	}
}

func TestCanSendEntityToClientRejectsHiddenPlayerWithoutMatchingActiveWitness(t *testing.T) {
	now := time.Unix(100, 0)
	tests := []struct {
		name      string
		witnesses []visibility.Witness
	}{
		{name: "none"},
		{
			name: "other target",
			witnesses: []visibility.Witness{{
				TargetPlayerID: "other-player",
				ExpiresAt:      now.Add(15 * time.Minute),
			}},
		},
		{
			name: "expired",
			witnesses: []visibility.Witness{{
				TargetPlayerID: "target-player",
				ExpiresAt:      now,
			}},
		},
		{
			name: "missing expiry",
			witnesses: []visibility.Witness{{
				TargetPlayerID: "target-player",
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			viewer := testViewer(100)
			viewer.PlayerID = "viewer-player"
			viewer.ObservedAt = now
			viewer.Witnesses = test.witnesses
			entity := testEntity(world.Vec2{X: 10, Y: 0})
			entity.PlayerID = "target-player"
			entity.Hidden = true

			if visibility.CanSendEntityToClient(viewer, entity) {
				t.Fatal("CanSendEntityToClient() = true, want false without matching active witness")
			}
		})
	}
}

func TestCanSendEntityToClientWitnessDoesNotBypassRange(t *testing.T) {
	now := time.Unix(100, 0)
	viewer := testViewer(50)
	viewer.PlayerID = "viewer-player"
	viewer.ObservedAt = now
	viewer.Witnesses = []visibility.Witness{{
		TargetPlayerID: "target-player",
		ExpiresAt:      now.Add(15 * time.Minute),
	}}
	entity := testEntity(world.Vec2{X: 60, Y: 0})
	entity.PlayerID = "target-player"
	entity.Hidden = true

	if visibility.CanSendEntityToClient(viewer, entity) {
		t.Fatal("CanSendEntityToClient() = true, want false for witnessed hidden player outside range")
	}
}

func TestCanSendEntityToClientAllowsNormalEntityInRadarRange(t *testing.T) {
	viewer := testViewer(100)
	entity := testEntity(world.Vec2{X: 60, Y: 80})

	if !visibility.CanSendEntityToClient(viewer, entity) {
		t.Fatal("CanSendEntityToClient() = false, want true for in-range normal entity")
	}
}

func TestCanSendEntityToClientRejectsEntityOutsideRange(t *testing.T) {
	viewer := testViewer(99)
	entity := testEntity(world.Vec2{X: 60, Y: 80})

	if visibility.CanSendEntityToClient(viewer, entity) {
		t.Fatal("CanSendEntityToClient() = true, want false for out-of-range entity")
	}
}

func TestCanInteractRejectsHiddenEntityWithGenericError(t *testing.T) {
	viewer := testViewer(100)
	entity := testEntity(world.Vec2{X: 10, Y: 0})
	entity.Hidden = true

	err := visibility.CanInteract(viewer, entity)
	if !errors.Is(err, visibility.ErrNotVisible) {
		t.Fatalf("CanInteract() error = %v, want ErrNotVisible", err)
	}
	if got := err.Error(); got != visibility.ErrNotVisible.Error() {
		t.Fatalf("CanInteract() error = %q, want generic %q", got, visibility.ErrNotVisible.Error())
	}
	for _, leaked := range []string{"hidden", "planet", "radar", "signature", string(entity.ID)} {
		if strings.Contains(strings.ToLower(err.Error()), leaked) {
			t.Fatalf("CanInteract() error %q leaks %q", err.Error(), leaked)
		}
	}
}

func TestCanInteractAllowsVisibleEntity(t *testing.T) {
	viewer := testViewer(100)
	entity := testEntity(world.Vec2{X: 10, Y: 0})

	if err := visibility.CanInteract(viewer, entity); err != nil {
		t.Fatalf("CanInteract() error = %v, want nil", err)
	}
}

func TestViewerRadarRangeComesFromServerStatSnapshot(t *testing.T) {
	viewerType := reflect.TypeOf(visibility.Viewer{})
	field, ok := viewerType.FieldByName("RadarRange")
	if !ok {
		t.Fatal("Viewer missing RadarRange field")
	}

	radarType := reflect.TypeOf(visibility.RadarRangeFromStatSnapshot(stats.StatSnapshot{}))
	if field.Type != radarType {
		t.Fatalf("Viewer.RadarRange type = %v, want %v from stat snapshot helper", field.Type, radarType)
	}
	if field.Type.Kind() == reflect.Float64 {
		t.Fatal("Viewer.RadarRange is raw float64, want server-provided radar wrapper")
	}

	viewer := testViewer(42)
	if got := viewer.RadarRange.Units(); got != 42 {
		t.Fatalf("RadarRangeFromStatSnapshot() = %v, want 42", got)
	}
}

func testViewer(radarRange float64) visibility.Viewer {
	snapshot := stats.NewStatSnapshot(
		"player-1",
		"ship-1",
		1,
		stats.EffectiveStats{
			Exploration: stats.ExplorationStats{
				RadarRange: radarRange,
			},
		},
		time.Unix(1, 0),
	)

	return visibility.Viewer{
		WorldID:    "world-1",
		ZoneID:     "zone-1",
		Position:   world.Vec2{X: 0, Y: 0},
		RadarRange: visibility.RadarRangeFromStatSnapshot(snapshot),
	}
}

func testEntity(position world.Vec2) visibility.Entity {
	return visibility.Entity{
		WorldID:   "world-1",
		ZoneID:    "zone-1",
		ID:        "entity-1",
		Position:  position,
		Signature: visibility.EntitySignature(1),
	}
}
