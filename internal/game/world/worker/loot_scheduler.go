package worker

import (
	"errors"
	"fmt"

	"gameproject/internal/game/loot"
	"gameproject/internal/game/world"
)

var ErrNilLootService = errors.New("nil loot service")

// LootScheduledTaskHandler dispatches due loot tasks to the loot service.
type LootScheduledTaskHandler struct {
	service *loot.Service
}

// NewLootScheduledTaskHandler returns a worker scheduled-task handler for loot.
func NewLootScheduledTaskHandler(service *loot.Service) (LootScheduledTaskHandler, error) {
	if service == nil {
		return LootScheduledTaskHandler{}, ErrNilLootService
	}
	return LootScheduledTaskHandler{service: service}, nil
}

// HandlesScheduledTaskKind reports whether kind belongs to the loot scheduler.
func (handler LootScheduledTaskHandler) HandlesScheduledTaskKind(kind string) bool {
	switch loot.ScheduledDropTaskKind(kind) {
	case loot.ScheduledDropTaskOwnerLockExpired, loot.ScheduledDropTaskDespawn:
		return true
	default:
		return false
	}
}

// HandleScheduledTask applies one due loot task through LootService.
func (handler LootScheduledTaskHandler) HandleScheduledTask(task ScheduledTask) error {
	if handler.service == nil {
		return ErrNilLootService
	}
	lootTask, ok := LootScheduledDropTask(task)
	if !ok {
		return fmt.Errorf("loot scheduled task %q: %w", task.Kind, ErrInvalidWorkerConfig)
	}
	_, err := handler.service.HandleScheduledDropTask(lootTask)
	return err
}

// ScheduleLootDropTasks maps loot-owned delayed work into the worker's local
// scheduler. The worker owns timing; loot.Service owns side effects.
func (worker *Worker) ScheduleLootDropTasks(tasks []loot.ScheduledDropTask) ([]ScheduledTask, error) {
	scheduled := make([]ScheduledTask, 0, len(tasks))
	for _, task := range tasks {
		if err := validateLootScheduledDropTask(task); err != nil {
			return nil, err
		}
		mapped := ScheduledTask{
			ID:        task.ID,
			DueAt:     task.DueAt,
			Kind:      string(task.Kind),
			SubjectID: task.DropID.String(),
		}
		accepted, err := worker.ScheduleTask(mapped)
		if err != nil {
			return nil, err
		}
		scheduled = append(scheduled, accepted)
	}
	return scheduled, nil
}

// LootScheduledDropTask converts a due worker task back into the loot-owned
// contract. Non-loot tasks return false.
func LootScheduledDropTask(task ScheduledTask) (loot.ScheduledDropTask, bool) {
	switch loot.ScheduledDropTaskKind(task.Kind) {
	case loot.ScheduledDropTaskOwnerLockExpired, loot.ScheduledDropTaskDespawn:
		if task.SubjectID == "" {
			return loot.ScheduledDropTask{}, false
		}
		return loot.ScheduledDropTask{
			ID:     task.ID,
			DueAt:  task.DueAt,
			Kind:   loot.ScheduledDropTaskKind(task.Kind),
			DropID: world.EntityID(task.SubjectID),
		}, true
	default:
		return loot.ScheduledDropTask{}, false
	}
}

func validateLootScheduledDropTask(task loot.ScheduledDropTask) error {
	switch task.Kind {
	case loot.ScheduledDropTaskOwnerLockExpired, loot.ScheduledDropTaskDespawn:
	default:
		return fmt.Errorf("loot scheduled task kind %q: %w", task.Kind, ErrInvalidWorkerConfig)
	}
	if err := task.DropID.Validate(); err != nil {
		return fmt.Errorf("loot scheduled task drop: %w", err)
	}
	return nil
}
