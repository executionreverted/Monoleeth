package worker

import (
	"gameproject/internal/game/loot"
	"gameproject/internal/game/world"
)

// ScheduleLootDropTasks maps loot-owned delayed work into the worker's local
// scheduler. The worker owns timing; loot.Service owns side effects.
func (worker *Worker) ScheduleLootDropTasks(tasks []loot.ScheduledDropTask) ([]ScheduledTask, error) {
	scheduled := make([]ScheduledTask, 0, len(tasks))
	for _, task := range tasks {
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
