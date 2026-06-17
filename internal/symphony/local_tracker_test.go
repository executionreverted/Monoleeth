package symphony

import (
	"context"
	"io"
	"path/filepath"
	"testing"
)

func TestLocalTrackerCreateListAndUpdateState(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(filepath.Join(dir, "logs"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	tracker := NewLocalTracker(Config{Tracker: TrackerConfig{
		Kind:           "local",
		LocalRoot:      filepath.Join(dir, "tasks"),
		RequiredLabels: []string{"symphony"},
		ActiveStates:   []string{"Todo", "In Progress"},
		TerminalStates: []string{"Done"},
	}}, logger)

	initial, err := tracker.ListTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if initial == nil || len(initial) != 0 {
		t.Fatalf("empty task list should be an empty slice, got %#v", initial)
	}

	task, err := tracker.CreateTask(context.Background(), CreateTaskInput{
		Title:       "  Build inventory loop  ",
		Description: "Acceptance criteria",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "local-0001" || task.Identifier != "TASK-0001" {
		t.Fatalf("unexpected task identity: %#v", task)
	}
	if task.Title != "Build inventory loop" || task.State != "Todo" || !task.AssignedToWorker {
		t.Fatalf("unexpected created task: %#v", task)
	}
	if got := len(task.Labels); got != 1 || task.Labels[0] != "symphony" {
		t.Fatalf("unexpected labels: %#v", task.Labels)
	}

	tasks, err := tracker.ListTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != task.ID {
		t.Fatalf("expected one listed task, got %#v", tasks)
	}

	candidates, err := tracker.FetchCandidateIssues(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].State != "Todo" {
		t.Fatalf("expected todo candidate, got %#v", candidates)
	}

	if err := tracker.UpdateIssueState(context.Background(), task.ID, "Done"); err != nil {
		t.Fatal(err)
	}
	candidates, err = tracker.FetchCandidateIssues(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("done task should not be a dispatch candidate: %#v", candidates)
	}
}
