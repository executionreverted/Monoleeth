package worker

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
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
	if entity.Movement.Target != (world.Vec2{}) {
		t.Fatalf("movement target = %+v, want zero value", entity.Movement.Target)
	}
	assertVecNear(t, entity.Position, world.Vec2{X: 10, Y: 0})
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

func TestCommandDrainOrderIsDeterministic(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 10)

	commands := []Command{
		MoveToCommand{PlayerID: "player-1", Intent: mustMovementIntent(t, world.Vec2{X: 100, Y: 0})},
		StopCommand{PlayerID: "player-1"},
		MoveToCommand{PlayerID: "player-1", Intent: mustMovementIntent(t, world.Vec2{X: 0, Y: 100})},
	}
	for _, command := range commands {
		if err := zoneWorker.Submit(command); err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
	}

	result := zoneWorker.Tick()
	assertNoCommandErrors(t, result)
	if result.DrainedCommands != len(commands) {
		t.Fatalf("DrainedCommands = %d, want %d", result.DrainedCommands, len(commands))
	}

	entity, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	assertVecNear(t, entity.Position, world.Vec2{X: 0, Y: 10})
	if entity.Movement.Target != (world.Vec2{X: 0, Y: 100}) {
		t.Fatalf("movement target = %+v, want last command target", entity.Movement.Target)
	}
}

func TestWorkerEntityInsertUpdateRemove(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)

	entity, err := world.NewEntity("world-1", "zone-1", "entity-npc-1", world.EntityTypeNPCPlaceholder, world.Vec2{X: 1, Y: 2})
	if err != nil {
		t.Fatalf("NewEntity() error = %v", err)
	}
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InsertEntityCommand{Entity: entity}))

	updated := entity
	updated.Position = world.Vec2{X: 3, Y: 4}
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, UpdateEntityCommand{Entity: updated}))

	got, ok := zoneWorker.Entity("entity-npc-1")
	if !ok {
		t.Fatal("Entity() ok = false, want true")
	}
	if got.Position != updated.Position {
		t.Fatalf("updated position = %+v, want %+v", got.Position, updated.Position)
	}

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, RemoveEntityCommand{EntityID: "entity-npc-1"}))
	if _, ok := zoneWorker.Entity("entity-npc-1"); ok {
		t.Fatal("Entity() ok = true after remove, want false")
	}
}

func TestDelayedTaskSchedulerDrainsDueTasksInOrder(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	zoneWorker, err := NewWorker(Config{
		WorldID:   "world-1",
		ZoneID:    "zone-1",
		TickDelta: time.Second,
		Clock:     clock,
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}

	_, err = zoneWorker.ScheduleTask(ScheduledTask{ID: "task-b", DueAt: clock.Now().Add(time.Second), Kind: "respawn"})
	if err != nil {
		t.Fatalf("ScheduleTask(task-b) error = %v", err)
	}
	_, err = zoneWorker.ScheduleTask(ScheduledTask{ID: "task-a", DueAt: clock.Now().Add(time.Second), Kind: "despawn"})
	if err != nil {
		t.Fatalf("ScheduleTask(task-a) error = %v", err)
	}
	_, err = zoneWorker.ScheduleTask(ScheduledTask{ID: "task-later", DueAt: clock.Now().Add(2 * time.Second), Kind: "unlock"})
	if err != nil {
		t.Fatalf("ScheduleTask(task-later) error = %v", err)
	}

	result := zoneWorker.Tick()
	if len(result.DueTasks) != 0 {
		t.Fatalf("due task count before due time = %d, want 0", len(result.DueTasks))
	}

	clock.Advance(time.Second)
	result = zoneWorker.Tick()
	if got, want := taskIDs(result.DueTasks), []string{"task-a", "task-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("due tasks = %v, want %v", got, want)
	}
}

func newTestWorker(t *testing.T, tickDelta time.Duration) *Worker {
	t.Helper()

	zoneWorker, err := NewWorker(Config{
		WorldID:   "world-1",
		ZoneID:    "zone-1",
		TickDelta: tickDelta,
		Clock:     testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	return zoneWorker
}

func spawnPlayer(t *testing.T, zoneWorker *Worker, playerID foundation.PlayerID, entityID world.EntityID, position world.Vec2, speed float64) {
	t.Helper()

	if err := zoneWorker.Submit(SpawnPlayerCommand{
		PlayerID: playerID,
		EntityID: entityID,
		Position: position,
		Speed:    speed,
	}); err != nil {
		t.Fatalf("Submit(spawn) error = %v", err)
	}
	assertNoCommandErrors(t, zoneWorker.Tick())
}

func tickSubmitted(t *testing.T, zoneWorker *Worker, command Command) TickResult {
	t.Helper()

	if err := zoneWorker.Submit(command); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	return zoneWorker.Tick()
}

func mustMovementIntent(t *testing.T, target world.Vec2) world.MovementIntent {
	t.Helper()

	intent, err := world.NewMovementIntent(target)
	if err != nil {
		t.Fatalf("NewMovementIntent() error = %v", err)
	}
	return intent
}

func assertNoCommandErrors(t *testing.T, result TickResult) {
	t.Helper()

	if len(result.CommandErrors) != 0 {
		t.Fatalf("command errors = %+v, want none", result.CommandErrors)
	}
}

func assertVecNear(t *testing.T, got world.Vec2, want world.Vec2) {
	t.Helper()

	const tolerance = 1e-9
	if math.Abs(got.X-want.X) > tolerance || math.Abs(got.Y-want.Y) > tolerance {
		t.Fatalf("position = %+v, want %+v", got, want)
	}
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

func taskIDs(tasks []ScheduledTask) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}
