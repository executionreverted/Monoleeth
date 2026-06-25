package worker

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/spatial"
)

func TestWorkerCanSpawnPlayer(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)

	if err := zoneWorker.Submit(SpawnPlayerCommand{
		PlayerID:  "player-1",
		EntityID:  "entity-player-1",
		Position:  world.Vec2{X: 10, Y: 20},
		Speed:     15,
		SessionID: "session-1",
	}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	result := zoneWorker.Tick()
	assertNoCommandErrors(t, result)

	entity, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	if entity.ID != "entity-player-1" {
		t.Fatalf("player entity id = %q, want entity-player-1", entity.ID)
	}
	if entity.Type != world.EntityTypePlayer {
		t.Fatalf("player entity type = %q, want %q", entity.Type, world.EntityTypePlayer)
	}
	if entity.Position != (world.Vec2{X: 10, Y: 20}) {
		t.Fatalf("player position = %+v, want {10 20}", entity.Position)
	}
	if playerID, ok := zoneWorker.AttachedPlayer("session-1"); !ok || playerID != "player-1" {
		t.Fatalf("AttachedPlayer() = %q, %t; want player-1, true", playerID, ok)
	}
}

func TestBoundedSpatialProjectionQueryReturnsOnlyEntitiesInsideWindow(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 10)
	mustInsertWorkerEntity(t, zoneWorker, "entity-corner", world.EntityTypeNPC, world.Vec2{X: 1000, Y: 1000})
	mustInsertWorkerEntity(t, zoneWorker, "entity-outside", world.EntityTypeNPC, world.Vec2{X: 1001, Y: 0})

	snapshot, err := zoneWorker.EntitiesWithinWindow(world.Vec2{}, 1000)
	if err != nil {
		t.Fatalf("EntitiesWithinWindow() = %v, want nil", err)
	}

	if got, want := workerSnapshotEntityIDs(snapshot), []world.EntityID{"entity-corner", "entity-player-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window entity ids = %v, want %v", got, want)
	}
}

func TestBoundedSpatialProjectionQueryUsesUpdatedIndexPositions(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	mustInsertWorkerEntity(t, zoneWorker, "entity-drifter", world.EntityTypeNPC, world.Vec2{X: 1500, Y: 0})

	initial, err := zoneWorker.EntitiesWithinWindow(world.Vec2{}, 1000)
	if err != nil {
		t.Fatalf("initial EntitiesWithinWindow() = %v, want nil", err)
	}
	if hasWorkerSnapshotEntityID(initial, "entity-drifter") {
		t.Fatalf("initial window included outside entity: %+v", initial.Entities)
	}

	entity, ok := zoneWorker.Entity("entity-drifter")
	if !ok {
		t.Fatal("Entity(entity-drifter) ok = false")
	}
	entity.Position = world.Vec2{X: 999, Y: 0}
	if err := zoneWorker.UpdateEntity(entity); err != nil {
		t.Fatalf("UpdateEntity() = %v, want nil", err)
	}

	updated, err := zoneWorker.EntitiesWithinWindow(world.Vec2{}, 1000)
	if err != nil {
		t.Fatalf("updated EntitiesWithinWindow() = %v, want nil", err)
	}
	if !hasWorkerSnapshotEntityID(updated, "entity-drifter") {
		t.Fatalf("updated window missing moved entity: %+v", updated.Entities)
	}
}

func TestMoveToCommandUpdatesPositionByServerSpeed(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 10)

	intent, err := world.NewMovementIntent(world.Vec2{X: 100, Y: 0})
	if err != nil {
		t.Fatalf("NewMovementIntent() error = %v", err)
	}
	if err := zoneWorker.Submit(MoveToCommand{PlayerID: "player-1", Intent: intent}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	result := zoneWorker.Tick()
	assertNoCommandErrors(t, result)

	entity, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	assertVecNear(t, entity.Position, world.Vec2{X: 10, Y: 0})
	if !entity.Movement.Moving {
		t.Fatal("entity movement = stopped, want moving")
	}
	if entity.Movement.Origin != (world.Vec2{}) {
		t.Fatalf("movement origin = %+v, want spawn position", entity.Movement.Origin)
	}
	if entity.Movement.Target != (world.Vec2{X: 100, Y: 0}) {
		t.Fatalf("movement target = %+v, want 100,0", entity.Movement.Target)
	}
	if entity.Movement.Speed != 10 {
		t.Fatalf("movement speed = %v, want server speed 10", entity.Movement.Speed)
	}
	if entity.Movement.StartedAtMS == 0 || entity.Movement.ArriveAtMS <= entity.Movement.StartedAtMS {
		t.Fatalf("movement timing = %+v, want start and arrival", entity.Movement)
	}
}

func TestMoveToWhileMovingStartsFromServerCurrentPosition(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 10)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MoveToCommand{
		PlayerID: "player-1",
		Intent:   mustMovementIntent(t, world.Vec2{X: 100, Y: 0}),
	}))
	inFlight, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	assertVecNear(t, inFlight.Position, world.Vec2{X: 10, Y: 0})

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MoveToCommand{
		PlayerID: "player-1",
		Intent:   mustMovementIntent(t, world.Vec2{X: 0, Y: 100}),
	}))
	retargeted, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	assertVecNear(t, retargeted.Movement.Origin, inFlight.Position)
	if retargeted.Movement.Target != (world.Vec2{X: 0, Y: 100}) {
		t.Fatalf("retargeted movement target = %+v, want 0,100", retargeted.Movement.Target)
	}
}

func TestMoveToWhileMovingStartsFromServerTimedPosition(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	clock := zoneWorker.clock.(*testutil.FakeClock)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 10)

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MoveToCommand{
		PlayerID: "player-1",
		Intent:   mustMovementIntent(t, world.Vec2{X: 100, Y: 0}),
	}))
	inFlight, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	assertVecNear(t, inFlight.Position, world.Vec2{X: 10, Y: 0})

	clock.Advance(4 * time.Second)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MoveToCommand{
		PlayerID: "player-1",
		Intent:   mustMovementIntent(t, world.Vec2{X: 0, Y: 100}),
	}))
	retargeted, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	assertVecNear(t, retargeted.Movement.Origin, world.Vec2{X: 40, Y: 0})
	if retargeted.Movement.Target != (world.Vec2{X: 0, Y: 100}) {
		t.Fatalf("retargeted movement target = %+v, want 0,100", retargeted.Movement.Target)
	}
}

func TestStopCommandClearsMovementTarget(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 10)

	intent := mustMovementIntent(t, world.Vec2{X: 100, Y: 0})
	if err := zoneWorker.Submit(MoveToCommand{PlayerID: "player-1", Intent: intent}); err != nil {
		t.Fatalf("Submit(move_to) error = %v", err)
	}
	assertNoCommandErrors(t, zoneWorker.Tick())

	if err := zoneWorker.Submit(StopCommand{PlayerID: "player-1"}); err != nil {
		t.Fatalf("Submit(stop) error = %v", err)
	}
	assertNoCommandErrors(t, zoneWorker.Tick())

	entity, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	if entity.Movement.Moving {
		t.Fatal("entity movement = moving, want stopped")
	}
	if entity.Movement != (world.MovementState{}) {
		t.Fatalf("movement state = %+v, want zero value", entity.Movement)
	}
	assertVecNear(t, entity.Position, world.Vec2{X: 10, Y: 0})
}

func TestStopCommandSettlesMovementAtServerTimedPosition(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	clock := zoneWorker.clock.(*testutil.FakeClock)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 10)
	target := world.Vec2{X: 100, Y: 0}

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MoveToCommand{
		PlayerID: "player-1",
		Intent:   mustMovementIntent(t, target),
	}))
	clock.Advance(4 * time.Second)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, StopCommand{PlayerID: "player-1"}))

	entity, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	assertVecNear(t, entity.Position, world.Vec2{X: 40, Y: 0})
	if entity.Position == target {
		t.Fatalf("settled position = target %+v, want in-flight server position", entity.Position)
	}
	if entity.Movement != (world.MovementState{}) {
		t.Fatalf("movement state = %+v, want zero value", entity.Movement)
	}
}

func TestSettleAndDetachSessionStopsMovementAtServerTimedPosition(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	clock := zoneWorker.clock.(*testutil.FakeClock)
	if err := zoneWorker.Submit(SpawnPlayerCommand{
		PlayerID:  "player-1",
		EntityID:  "entity-player-1",
		Position:  world.Vec2{},
		Speed:     10,
		SessionID: "session-1",
	}); err != nil {
		t.Fatalf("Submit(spawn) error = %v", err)
	}
	assertNoCommandErrors(t, zoneWorker.Tick())
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MoveToCommand{
		PlayerID: "player-1",
		Intent:   mustMovementIntent(t, world.Vec2{X: 100, Y: 0}),
	}))

	clock.Advance(4 * time.Second)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, SettleAndDetachSessionCommand{SessionID: "session-1"}))

	entity, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	assertVecNear(t, entity.Position, world.Vec2{X: 40, Y: 0})
	if entity.Movement.Moving {
		t.Fatalf("settled movement = %+v, want stopped", entity.Movement)
	}
	if _, ok := zoneWorker.AttachedPlayer("session-1"); ok {
		t.Fatal("AttachedPlayer(session-1) ok = true, want detached")
	}
}

func TestSetPlayerSpeedCommandRecalculatesMovingRouteFromServerPosition(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	clock := zoneWorker.clock.(*testutil.FakeClock)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 10)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MoveToCommand{
		PlayerID: "player-1",
		Intent:   mustMovementIntent(t, world.Vec2{X: 100, Y: 0}),
	}))

	clock.Advance(4 * time.Second)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, SetPlayerSpeedCommand{
		PlayerID: "player-1",
		Speed:    7,
	}))

	entity, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	speed, ok := zoneWorker.EntitySpeed("entity-player-1")
	if !ok || speed != 7 {
		t.Fatalf("EntitySpeed() = %v, %v; want 7,true", speed, ok)
	}
	assertVecNear(t, entity.Movement.Origin, world.Vec2{X: 40, Y: 0})
	assertVecNear(t, entity.Position, world.Vec2{X: 47, Y: 0})
	if entity.Movement.Target != (world.Vec2{X: 100, Y: 0}) {
		t.Fatalf("movement target = %+v, want original target 100,0", entity.Movement.Target)
	}
	if entity.Movement.Speed != 7 {
		t.Fatalf("movement speed = %v, want recalculated speed 7", entity.Movement.Speed)
	}
	if entity.Movement.ArriveAtMS <= entity.Movement.StartedAtMS {
		t.Fatalf("movement timing = %+v, want future arrival at slower speed", entity.Movement)
	}
}

func TestSetPlayerSpeedCommandZeroSpeedSettlesAndStopsRoute(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	clock := zoneWorker.clock.(*testutil.FakeClock)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 10)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, MoveToCommand{
		PlayerID: "player-1",
		Intent:   mustMovementIntent(t, world.Vec2{X: 100, Y: 0}),
	}))

	clock.Advance(4 * time.Second)
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, SetPlayerSpeedCommand{
		PlayerID: "player-1",
		Speed:    0,
	}))

	entity, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	speed, ok := zoneWorker.EntitySpeed("entity-player-1")
	if !ok || speed != 0 {
		t.Fatalf("EntitySpeed() = %v, %v; want 0,true", speed, ok)
	}
	assertVecNear(t, entity.Position, world.Vec2{X: 40, Y: 0})
	if entity.Movement.Moving {
		t.Fatalf("movement = %+v, want stopped after zero speed", entity.Movement)
	}
}

func TestMoveToCommandDoesNotExposeClientFinalPosition(t *testing.T) {
	commandType := reflect.TypeOf(MoveToCommand{})
	if commandType.NumField() != 2 {
		t.Fatalf("MoveToCommand fields = %v, want PlayerID and Intent only", exportedFieldNames(commandType))
	}
	if _, ok := commandType.FieldByName("PlayerID"); !ok {
		t.Fatalf("MoveToCommand fields = %v, want PlayerID", exportedFieldNames(commandType))
	}
	if _, ok := commandType.FieldByName("Intent"); !ok {
		t.Fatalf("MoveToCommand fields = %v, want Intent", exportedFieldNames(commandType))
	}

	for _, forbidden := range []string{
		"Position",
		"CurrentPosition",
		"ClientPosition",
		"FinalPosition",
		"NewPosition",
		"Speed",
		"Delta",
	} {
		if _, ok := commandType.FieldByName(forbidden); ok {
			t.Fatalf("MoveToCommand exposes client-supplied %s", forbidden)
		}
	}

	zoneWorker := newTestWorker(t, time.Second)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 7)
	if err := zoneWorker.Submit(MoveToCommand{
		PlayerID: "player-1",
		Intent:   mustMovementIntent(t, world.Vec2{X: 1_000_000, Y: 0}),
	}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	assertNoCommandErrors(t, zoneWorker.Tick())

	entity, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	assertVecNear(t, entity.Position, world.Vec2{X: 7, Y: 0})
}

func TestTickReportsSpatialUpdateFailureDuringMovement(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 10)

	if err := zoneWorker.Submit(MoveToCommand{
		PlayerID: "player-1",
		Intent:   mustMovementIntent(t, world.Vec2{X: 100, Y: 0}),
	}); err != nil {
		t.Fatalf("Submit(move_to) error = %v", err)
	}
	assertNoCommandErrors(t, zoneWorker.Tick())

	before, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	zoneWorker.index.Remove(spatial.EntityID("entity-player-1"))

	result := zoneWorker.Tick()
	if len(result.CommandErrors) != 1 {
		t.Fatalf("command errors = %+v, want one spatial update error", result.CommandErrors)
	}
	if result.CommandErrors[0].Index != -1 {
		t.Fatalf("command error index = %d, want -1 for movement advance", result.CommandErrors[0].Index)
	}
	if !errors.Is(result.CommandErrors[0].Err, spatial.ErrEntityNotIndexed) {
		t.Fatalf("command error = %v, want ErrEntityNotIndexed", result.CommandErrors[0].Err)
	}

	after, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() after tick ok = false, want true")
	}
	if after.Position != before.Position {
		t.Fatalf("position mutated after failed spatial update: got %+v, want %+v", after.Position, before.Position)
	}
}
