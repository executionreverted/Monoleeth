package worker

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/progression"
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

func TestDelayedTaskSchedulerDeduplicatesByTaskID(t *testing.T) {
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

	if _, err := zoneWorker.ScheduleTask(ScheduledTask{ID: "drop-1:despawn", DueAt: clock.Now().Add(10 * time.Second), Kind: "loot.drop_despawn"}); err != nil {
		t.Fatalf("ScheduleTask(first) error = %v", err)
	}
	if _, err := zoneWorker.ScheduleTask(ScheduledTask{ID: "drop-1:despawn", DueAt: clock.Now().Add(time.Second), Kind: "loot.drop_despawn", SubjectID: "drop-1"}); err != nil {
		t.Fatalf("ScheduleTask(duplicate) error = %v", err)
	}

	clock.Advance(time.Second)
	result := zoneWorker.Tick()
	if got, want := taskIDs(result.DueTasks), []string{"drop-1:despawn"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deduped due tasks = %v, want %v", got, want)
	}
	if result.DueTasks[0].SubjectID != "drop-1" {
		t.Fatalf("deduped task subject = %q, want replacement subject drop-1", result.DueTasks[0].SubjectID)
	}
}

func TestTickDispatchesDueScheduledTasksToRegisteredHandler(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	handler := &recordingScheduledTaskHandler{kind: "respawn"}
	zoneWorker, err := NewWorker(Config{
		WorldID:               "world-1",
		ZoneID:                "zone-1",
		TickDelta:             time.Second,
		Clock:                 clock,
		ScheduledTaskHandlers: []ScheduledTaskHandler{handler},
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	if _, err := zoneWorker.ScheduleTask(ScheduledTask{ID: "respawn-1", DueAt: clock.Now().Add(time.Second), Kind: "respawn"}); err != nil {
		t.Fatalf("ScheduleTask() error = %v", err)
	}

	clock.Advance(time.Second)
	result := zoneWorker.Tick()

	if len(result.ScheduledTaskErrors) != 0 {
		t.Fatalf("ScheduledTaskErrors = %+v, want none", result.ScheduledTaskErrors)
	}
	if got, want := taskIDs(handler.tasks), []string{"respawn-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handled task ids = %v, want %v", got, want)
	}
}

func TestTickRecordsScheduledTaskErrorsAndContinuesDispatch(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	handlerErr := errors.New("handler failed")
	handler := &recordingScheduledTaskHandler{
		kind:      "respawn",
		errByID:   map[string]error{"respawn-1": handlerErr},
		retryByID: map[string]time.Time{"respawn-2": clock.Now().Add(2 * time.Second)},
	}
	zoneWorker, err := NewWorker(Config{
		WorldID:               "world-1",
		ZoneID:                "zone-1",
		TickDelta:             time.Second,
		Clock:                 clock,
		ScheduledTaskHandlers: []ScheduledTaskHandler{handler},
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	for _, task := range []ScheduledTask{
		{ID: "respawn-1", DueAt: clock.Now().Add(time.Second), Kind: "respawn"},
		{ID: "respawn-2", DueAt: clock.Now().Add(time.Second), Kind: "respawn"},
	} {
		if _, err := zoneWorker.ScheduleTask(task); err != nil {
			t.Fatalf("ScheduleTask(%s) error = %v", task.ID, err)
		}
	}

	clock.Advance(time.Second)
	result := zoneWorker.Tick()
	if len(result.ScheduledTaskErrors) != 1 {
		t.Fatalf("ScheduledTaskErrors = %+v, want one handler error", result.ScheduledTaskErrors)
	}
	if result.ScheduledTaskErrors[0].Task.ID != "respawn-1" || !errors.Is(result.ScheduledTaskErrors[0].Err, handlerErr) {
		t.Fatalf("ScheduledTaskErrors[0] = %+v, want respawn-1 handler error", result.ScheduledTaskErrors[0])
	}
	if got, want := taskIDs(handler.tasks), []string{"respawn-1", "respawn-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handled task ids = %v, want %v", got, want)
	}

	clock.Advance(time.Second)
	result = zoneWorker.Tick()
	if len(result.ScheduledTaskErrors) != 0 {
		t.Fatalf("retry ScheduledTaskErrors = %+v, want none", result.ScheduledTaskErrors)
	}
	if got, want := taskIDs(result.DueTasks), []string{"respawn-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retried due task ids = %v, want %v", got, want)
	}
}

func TestScheduleLootDropTasksMapsDueTasksToLootContract(t *testing.T) {
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
	tasks := []loot.ScheduledDropTask{
		{
			ID:     "drop-1:owner",
			Kind:   loot.ScheduledDropTaskOwnerLockExpired,
			DropID: "drop-1",
			DueAt:  clock.Now().Add(time.Second),
		},
		{
			ID:     "drop-1:despawn",
			Kind:   loot.ScheduledDropTaskDespawn,
			DropID: "drop-1",
			DueAt:  clock.Now().Add(2 * time.Second),
		},
	}

	scheduled, err := zoneWorker.ScheduleLootDropTasks(tasks)
	if err != nil {
		t.Fatalf("ScheduleLootDropTasks() error = %v", err)
	}
	if len(scheduled) != 2 {
		t.Fatalf("scheduled len = %d, want 2", len(scheduled))
	}

	clock.Advance(time.Second)
	result := zoneWorker.Tick()
	if len(result.DueTasks) != 1 {
		t.Fatalf("due task len = %d, want 1", len(result.DueTasks))
	}
	lootTask, ok := LootScheduledDropTask(result.DueTasks[0])
	if !ok {
		t.Fatalf("LootScheduledDropTask(%+v) ok = false, want true", result.DueTasks[0])
	}
	if lootTask.Kind != loot.ScheduledDropTaskOwnerLockExpired || lootTask.DropID != "drop-1" {
		t.Fatalf("loot scheduled task = %+v, want owner-lock task for drop-1", lootTask)
	}

	clock.Advance(time.Second)
	result = zoneWorker.Tick()
	if len(result.DueTasks) != 1 {
		t.Fatalf("second due task len = %d, want 1", len(result.DueTasks))
	}
	lootTask, ok = LootScheduledDropTask(result.DueTasks[0])
	if !ok {
		t.Fatalf("LootScheduledDropTask(%+v) ok = false, want true", result.DueTasks[0])
	}
	if lootTask.Kind != loot.ScheduledDropTaskDespawn || lootTask.DropID != "drop-1" {
		t.Fatalf("loot scheduled task = %+v, want despawn task for drop-1", lootTask)
	}
}

func TestTickDispatchesLootScheduledTasksThroughLootService(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	lootService := newWorkerLootService(t, clock)
	recorder := testutil.NewEventRecorder()
	lootService.SetEventEmitter(recorder)
	created, err := lootService.CreateDropsForNPCKill(workerNPCKilledEvent(), workerLootTable(t))
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill() error = %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootCreated)
	recorder.Reset()

	handler, err := NewLootScheduledTaskHandler(lootService)
	if err != nil {
		t.Fatalf("NewLootScheduledTaskHandler() error = %v", err)
	}
	zoneWorker, err := NewWorker(Config{
		WorldID:               "world-1",
		ZoneID:                "zone-1",
		TickDelta:             time.Second,
		Clock:                 clock,
		ScheduledTaskHandlers: []ScheduledTaskHandler{handler},
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	if _, err := zoneWorker.ScheduleLootDropTasks(created.ScheduledTasks); err != nil {
		t.Fatalf("ScheduleLootDropTasks() error = %v", err)
	}

	clock.Advance(loot.DefaultOwnerLockDuration)
	result := zoneWorker.Tick()
	if len(result.ScheduledTaskErrors) != 0 {
		t.Fatalf("owner-lock ScheduledTaskErrors = %+v, want none", result.ScheduledTaskErrors)
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootOwnerLockExpired)
	recorder.Reset()

	clock.Advance(loot.DefaultTotalLifetime - loot.DefaultOwnerLockDuration)
	result = zoneWorker.Tick()
	if len(result.ScheduledTaskErrors) != 0 {
		t.Fatalf("despawn ScheduledTaskErrors = %+v, want none", result.ScheduledTaskErrors)
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootExpired)
}

func TestTickRetriesEarlyLootScheduledTaskUntilLootClockIsDue(t *testing.T) {
	start := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	workerClock := testutil.NewFakeClock(start)
	lootClock := testutil.NewFakeClock(start)
	lootService := newWorkerLootService(t, lootClock)
	recorder := testutil.NewEventRecorder()
	lootService.SetEventEmitter(recorder)
	created, err := lootService.CreateDropsForNPCKill(workerNPCKilledEvent(), workerLootTable(t))
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill() error = %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootCreated)
	recorder.Reset()

	handler, err := NewLootScheduledTaskHandler(lootService)
	if err != nil {
		t.Fatalf("NewLootScheduledTaskHandler() error = %v", err)
	}
	zoneWorker, err := NewWorker(Config{
		WorldID:               "world-1",
		ZoneID:                "zone-1",
		TickDelta:             time.Second,
		Clock:                 workerClock,
		ScheduledTaskHandlers: []ScheduledTaskHandler{handler},
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	if _, err := zoneWorker.ScheduleLootDropTasks(created.ScheduledTasks); err != nil {
		t.Fatalf("ScheduleLootDropTasks() error = %v", err)
	}

	workerClock.Advance(loot.DefaultOwnerLockDuration)
	early := zoneWorker.Tick()
	if len(early.ScheduledTaskErrors) != 0 {
		t.Fatalf("early ScheduledTaskErrors = %+v, want none", early.ScheduledTaskErrors)
	}
	if got, want := taskIDs(early.DueTasks), []string{created.ScheduledTasks[0].ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("early due task ids = %v, want %v", got, want)
	}
	testutil.AssertRecordedEventTypes(t, recorder)

	lootClock.Advance(loot.DefaultOwnerLockDuration)
	retried := zoneWorker.Tick()
	if len(retried.ScheduledTaskErrors) != 0 {
		t.Fatalf("retry ScheduledTaskErrors = %+v, want none", retried.ScheduledTaskErrors)
	}
	if got, want := taskIDs(retried.DueTasks), []string{created.ScheduledTasks[0].ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retry due task ids = %v, want %v", got, want)
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootOwnerLockExpired)
}

func TestScheduleLootDropTasksRejectsInvalidLootTask(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)

	_, err := zoneWorker.ScheduleLootDropTasks([]loot.ScheduledDropTask{
		{
			ID:     "drop-1:unknown",
			Kind:   "loot.unknown",
			DropID: "drop-1",
			DueAt:  time.Date(2026, 6, 17, 12, 0, 1, 0, time.UTC),
		},
	})
	if !errors.Is(err, ErrInvalidWorkerConfig) {
		t.Fatalf("ScheduleLootDropTasks(unknown kind) error = %v, want ErrInvalidWorkerConfig", err)
	}

	_, err = zoneWorker.ScheduleLootDropTasks([]loot.ScheduledDropTask{
		{
			ID:    "missing-drop",
			Kind:  loot.ScheduledDropTaskDespawn,
			DueAt: time.Date(2026, 6, 17, 12, 0, 1, 0, time.UTC),
		},
	})
	if err == nil {
		t.Fatal("ScheduleLootDropTasks(empty drop) error = nil, want validation error")
	}
}

type recordingScheduledTaskHandler struct {
	kind      string
	tasks     []ScheduledTask
	err       error
	errByID   map[string]error
	retryByID map[string]time.Time
}

func (handler *recordingScheduledTaskHandler) HandlesScheduledTaskKind(kind string) bool {
	return kind == handler.kind
}

func (handler *recordingScheduledTaskHandler) HandleScheduledTask(task ScheduledTask) (ScheduledTaskHandleResult, error) {
	handler.tasks = append(handler.tasks, task)
	if err := handler.errByID[task.ID]; err != nil {
		return ScheduledTaskHandleResult{}, err
	}
	if retryAt := handler.retryByID[task.ID]; !retryAt.IsZero() {
		delete(handler.retryByID, task.ID)
		return ScheduledTaskHandleResult{RetryAt: retryAt}, nil
	}
	return ScheduledTaskHandleResult{}, handler.err
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

func newWorkerLootService(t *testing.T, clock *testutil.FakeClock) *loot.Service {
	t.Helper()
	inventory := economy.NewInventoryService(clock)
	service, err := loot.NewService(loot.Config{
		Clock:       clock,
		Cargo:       economy.NewCargoService(inventory),
		Progression: progression.NewProgressionService(clock, nil),
		RNG:         testutil.NewFakeRNG([]int{0}, []float64{0}),
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func workerNPCKilledEvent() combat.NPCKilledEvent {
	return combat.NPCKilledEvent{
		SourceID:      "npc-1",
		NPCEntityID:   "npc-1",
		WorldID:       "world-1",
		ZoneID:        "zone-1",
		Position:      world.Vec2{X: 10, Y: 0},
		OwnerPlayerID: "player-1",
		KilledAt:      time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
	}
}

func workerLootTable(t *testing.T) loot.LootTable {
	t.Helper()
	source, err := catalog.NewLootTableSource("worker_loot_table", "v1")
	if err != nil {
		t.Fatalf("NewLootTableSource() error = %v", err)
	}
	itemSource, err := catalog.NewVersionedDefinitionFromStrings("raw_ore", "v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings() error = %v", err)
	}
	maxStack, err := foundation.NewQuantity(999)
	if err != nil {
		t.Fatalf("NewQuantity(max) error = %v", err)
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(weight) error = %v", err)
	}
	item, err := economy.NewItemDefinition(
		itemSource,
		"raw_ore",
		"Raw Ore",
		economy.ItemTypeStackable,
		economy.ItemRarityCommon,
		maxStack,
		weight,
		[]economy.TradeFlag{economy.TradeFlagDroppable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition() error = %v", err)
	}
	return loot.LootTable{
		Source: source,
		Rows: []loot.LootRow{
			{ItemDefinition: item, MinQuantity: 1, MaxQuantity: 1, Chance: 1},
		},
	}
}

func taskIDs(tasks []ScheduledTask) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}
