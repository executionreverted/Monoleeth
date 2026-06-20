package aoi_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	"gameproject/internal/game/world/visibility"
)

func TestBuildVisibleSnapshotOmitsHiddenEntity(t *testing.T) {
	snapshot := aoi.BuildVisibleSnapshot(testViewer(100), []aoi.EntityState{
		testState("entity-hidden", world.EntityTypePlanetSignalPlaceholder, world.Vec2{X: 10}, true),
		testState("entity-visible", world.EntityTypeNPCPlaceholder, world.Vec2{X: 20}, false),
	})

	if got, want := payloadIDs(snapshot.Entities), []world.EntityID{"entity-visible"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot ids = %v, want %v", got, want)
	}
}

func TestBuildVisibleSnapshotAllowsHiddenSelfPlayerWithoutLeakingHiddenTruth(t *testing.T) {
	viewer := testViewer(100)
	viewer.PlayerID = "player-1"
	state := testState("entity-self", world.EntityTypePlayer, world.Vec2{X: 10}, true)
	state.PlayerID = "player-1"
	state.PublicStatusFlags = []aoi.StatusFlag{"self"}

	snapshot := aoi.BuildVisibleSnapshot(viewer, []aoi.EntityState{state})

	if got, want := payloadIDs(snapshot.Entities), []world.EntityID{"entity-self"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot ids = %v, want %v", got, want)
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Marshal(snapshot) error = %v", err)
	}
	payload := string(data)
	for _, leaked := range []string{"player_id", "hidden", "witness", "expires"} {
		if strings.Contains(payload, leaked) {
			t.Fatalf("hidden self payload %s leaked %q", payload, leaked)
		}
	}
}

func TestBuildVisibleSnapshotAllowsWitnessedHiddenPlayerWithSafeFlag(t *testing.T) {
	now := time.Unix(100, 0)
	viewer := testViewer(100)
	viewer.PlayerID = "viewer-player"
	viewer.ObservedAt = now
	viewer.Witnesses = []visibility.Witness{{
		TargetPlayerID: "target-player",
		ExpiresAt:      now.Add(15 * time.Minute),
	}}
	state := testState("entity-target", world.EntityTypePlayer, world.Vec2{X: 10}, true)
	state.PlayerID = "target-player"
	state.PublicStatusFlags = []aoi.StatusFlag{"scan_revealed"}

	snapshot := aoi.BuildVisibleSnapshot(viewer, []aoi.EntityState{state})

	if got, want := payloadIDs(snapshot.Entities), []world.EntityID{"entity-target"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot ids = %v, want %v", got, want)
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Marshal(snapshot) error = %v", err)
	}
	payload := string(data)
	if !strings.Contains(payload, "scan_revealed") {
		t.Fatalf("snapshot payload %s missing safe reveal flag", payload)
	}
	for _, leaked := range []string{"target-player", "viewer-player", "player_id", "hidden", "witness", "expires"} {
		if strings.Contains(payload, leaked) {
			t.Fatalf("witnessed payload %s leaked %q", payload, leaked)
		}
	}
}

func TestDiffSnapshotsReportsEnteredEntity(t *testing.T) {
	current := aoi.BuildVisibleSnapshot(testViewer(100), []aoi.EntityState{
		testState("entity-npc-1", world.EntityTypeNPCPlaceholder, world.Vec2{X: 10}, false),
	})

	diff := aoi.DiffSnapshots(aoi.Snapshot{}, current)

	if got, want := payloadIDs(diff.Entered), []world.EntityID{"entity-npc-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("entered ids = %v, want %v", got, want)
	}
	if len(diff.Updated) != 0 || len(diff.Left) != 0 {
		t.Fatalf("diff = %+v, want entered only", diff)
	}
}

func TestDiffSnapshotsReportsUpdatedEntityMovingInsideAOI(t *testing.T) {
	viewer := testViewer(100)
	previous := aoi.BuildVisibleSnapshot(viewer, []aoi.EntityState{
		testState("entity-npc-1", world.EntityTypeNPCPlaceholder, world.Vec2{X: 10}, false),
	})
	current := aoi.BuildVisibleSnapshot(viewer, []aoi.EntityState{
		testState("entity-npc-1", world.EntityTypeNPCPlaceholder, world.Vec2{X: 12}, false),
	})

	diff := aoi.DiffSnapshots(previous, current)

	if len(diff.Entered) != 0 || len(diff.Left) != 0 {
		t.Fatalf("diff = %+v, want updated only", diff)
	}
	if got, want := payloadIDs(diff.Updated), []world.EntityID{"entity-npc-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("updated ids = %v, want %v", got, want)
	}
	if diff.Updated[0].Position != (world.Vec2{X: 12}) {
		t.Fatalf("updated position = %+v, want x=12", diff.Updated[0].Position)
	}
}

func TestDiffSnapshotsReportsLeftEntity(t *testing.T) {
	viewer := testViewer(100)
	previous := aoi.BuildVisibleSnapshot(viewer, []aoi.EntityState{
		testState("entity-npc-1", world.EntityTypeNPCPlaceholder, world.Vec2{X: 10}, false),
	})
	current := aoi.BuildVisibleSnapshot(viewer, []aoi.EntityState{
		testState("entity-npc-1", world.EntityTypeNPCPlaceholder, world.Vec2{X: 101}, false),
	})

	diff := aoi.DiffSnapshots(previous, current)

	if len(diff.Entered) != 0 || len(diff.Updated) != 0 {
		t.Fatalf("diff = %+v, want left only", diff)
	}
	if got, want := diff.Left, []world.EntityID{"entity-npc-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("left ids = %v, want %v", got, want)
	}
}

func TestSnapshotPayloadOmitsHiddenInternalFields(t *testing.T) {
	state := testState("entity-planet-1", world.EntityTypePlanetSignalPlaceholder, world.Vec2{X: 10}, false)
	state.Entity.Movement = world.MovementState{Moving: true, Target: world.Vec2{X: 99, Y: 100}}
	state.PublicStatusFlags = []aoi.StatusFlag{"scannable", "neutral"}
	state.InternalMetadata = map[string]string{"secret_name": "undiscovered-planet"}
	state.GameplaySeed = "server-gameplay-seed"
	state.FutureSpawnData = []string{"future-spawn-candidate"}

	snapshot := aoi.BuildVisibleSnapshot(testViewer(100), []aoi.EntityState{state})
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Marshal(snapshot) error = %v", err)
	}

	payload := string(data)
	for _, leaked := range []string{
		"undiscovered-planet",
		"server-gameplay-seed",
		"future-spawn-candidate",
		"secret_name",
		"movement",
		"signature",
		"hidden",
		"world_id",
		"zone_id",
	} {
		if strings.Contains(payload, leaked) {
			t.Fatalf("snapshot payload %s leaked %q", payload, leaked)
		}
	}
	for _, expected := range []string{"entity_id", "entity_type", "position", "status_flags", "scannable", "neutral"} {
		if !strings.Contains(payload, expected) {
			t.Fatalf("snapshot payload %s missing %q", payload, expected)
		}
	}

	payloadType := reflect.TypeOf(aoi.EntityPayload{})
	if got, want := exportedFieldNames(payloadType), []string{"ID", "Type", "Position", "StatusFlags", "Display", "Combat", "Movement", "ProjectionSource"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("EntityPayload fields = %v, want %v", got, want)
	}
}

func TestSnapshotPayloadIncludesExplicitPublicMovementTiming(t *testing.T) {
	state := testState("entity-player-1", world.EntityTypePlayer, world.Vec2{X: 12, Y: 0}, false)
	state.PublicMovement = &aoi.EntityMovementStatus{
		Moving:      true,
		Origin:      world.Vec2{X: 10, Y: 0},
		Target:      world.Vec2{X: 100, Y: 0},
		Speed:       180,
		StartedAtMS: 1000,
		ArriveAtMS:  1500,
	}

	snapshot := aoi.BuildVisibleSnapshot(testViewer(100), []aoi.EntityState{state})
	if len(snapshot.Entities) != 1 {
		t.Fatalf("snapshot entities = %d, want 1", len(snapshot.Entities))
	}
	movement := snapshot.Entities[0].Movement
	if movement == nil {
		t.Fatal("movement payload = nil, want public movement timing")
	}
	if movement.Origin != (world.Vec2{X: 10, Y: 0}) || movement.Target != (world.Vec2{X: 100, Y: 0}) {
		t.Fatalf("movement route = %+v, want origin 10,0 target 100,0", movement)
	}
	if movement.Speed != 180 || movement.StartedAtMS != 1000 || movement.ArriveAtMS != 1500 {
		t.Fatalf("movement timing = %+v, want speed/start/arrival", movement)
	}
}

func TestBuildVisibleSnapshotStressDeterministic(t *testing.T) {
	viewer := testViewer(250)
	entities := make([]aoi.EntityState, 0, 500)
	for index := 0; index < 500; index++ {
		entityID := world.EntityID(fmt.Sprintf("entity-%04d", index))
		position := world.Vec2{
			X: float64((index%41)-20) * 6,
			Y: float64((index%23)-11) * 5,
		}
		if index%9 == 0 {
			position.X = 1_000 + float64(index)
		}
		state := testState(entityID, world.EntityTypeNPCPlaceholder, position, index%7 == 0)
		if index%11 == 0 {
			state.PublicStatusFlags = []aoi.StatusFlag{"moving", "neutral"}
		}
		entities = append(entities, state)
	}

	want := aoi.BuildVisibleSnapshot(viewer, entities)
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(want) error = %v", err)
	}

	for attempt := 0; attempt < 10; attempt++ {
		rotated := rotateStates(entities, attempt*37)
		got := aoi.BuildVisibleSnapshot(viewer, rotated)
		gotJSON, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("Marshal(got) attempt %d error = %v", attempt, err)
		}
		if string(gotJSON) != string(wantJSON) {
			t.Fatalf("snapshot attempt %d is not deterministic", attempt)
		}
		if !entityPayloadsSorted(got.Entities) {
			t.Fatalf("snapshot attempt %d entities not sorted: %v", attempt, payloadIDs(got.Entities))
		}
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
		Position:   world.Vec2{},
		RadarRange: visibility.RadarRangeFromStatSnapshot(snapshot),
	}
}

func testState(entityID world.EntityID, entityType world.EntityType, position world.Vec2, hidden bool) aoi.EntityState {
	entity, err := world.NewEntity("world-1", "zone-1", entityID, entityType, position)
	if err != nil {
		panic(err)
	}
	return aoi.EntityState{
		Entity:    entity,
		Signature: visibility.EntitySignature(1),
		Hidden:    hidden,
	}
}

func payloadIDs(payloads []aoi.EntityPayload) []world.EntityID {
	ids := make([]world.EntityID, 0, len(payloads))
	for _, payload := range payloads {
		ids = append(ids, payload.ID)
	}
	return ids
}

func exportedFieldNames(structType reflect.Type) []string {
	fields := make([]string, 0, structType.NumField())
	for index := 0; index < structType.NumField(); index++ {
		field := structType.Field(index)
		if field.IsExported() {
			fields = append(fields, field.Name)
		}
	}
	return fields
}

func rotateStates(states []aoi.EntityState, offset int) []aoi.EntityState {
	if len(states) == 0 {
		return nil
	}
	offset = offset % len(states)
	rotated := append([]aoi.EntityState(nil), states[offset:]...)
	rotated = append(rotated, states[:offset]...)
	return rotated
}

func entityPayloadsSorted(payloads []aoi.EntityPayload) bool {
	for index := 1; index < len(payloads); index++ {
		if payloads[index-1].ID > payloads[index].ID {
			return false
		}
	}
	return true
}
