package worker

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/testutil"
)

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
