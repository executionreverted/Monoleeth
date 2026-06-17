package symphony

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSortIssuesForDispatch(t *testing.T) {
	high := 1
	low := 3
	now := time.Now()
	older := now.Add(-time.Hour)
	issues := []Issue{
		{Identifier: "GP-3", Priority: &low, UpdatedAt: &older},
		{Identifier: "GP-2", Priority: &high, UpdatedAt: &now},
		{Identifier: "GP-1", Priority: &high, UpdatedAt: &older},
	}
	got := SortIssuesForDispatch(issues)
	if got[0].Identifier != "GP-1" || got[1].Identifier != "GP-2" || got[2].Identifier != "GP-3" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestRequiredLabels(t *testing.T) {
	issue := Issue{Labels: []string{"symphony", "backend"}}
	if !hasRequiredLabels(issue, []string{"Symphony"}) {
		t.Fatalf("expected case-insensitive label match")
	}
	if hasRequiredLabels(issue, []string{"frontend"}) {
		t.Fatalf("did not expect missing label to match")
	}
}

func TestUpdateTaskStateClearsClaimedForActiveState(t *testing.T) {
	logger := newDiscardLogger(t)
	defer logger.Close()
	orchestrator := NewOrchestrator(testWorkflow(t), logger)
	task, err := orchestrator.CreateTask(context.Background(), CreateTaskInput{Title: "Retry me"})
	if err != nil {
		t.Fatal(err)
	}
	orchestrator.mu.Lock()
	orchestrator.claimed[task.ID] = true
	orchestrator.completed[task.ID] = true
	orchestrator.blocked[task.ID] = BlockedEntry{IssueID: task.ID, Identifier: task.Identifier}
	orchestrator.retries[task.ID] = RetryEntry{IssueID: task.ID, Identifier: task.Identifier, Attempt: 1, DueAt: nowUTC()}
	orchestrator.mu.Unlock()

	if err := orchestrator.UpdateTaskState(context.Background(), task.ID, "Todo"); err != nil {
		t.Fatal(err)
	}

	orchestrator.mu.Lock()
	defer orchestrator.mu.Unlock()
	if orchestrator.claimed[task.ID] || orchestrator.completed[task.ID] {
		t.Fatalf("active state should clear claimed/completed maps")
	}
	if _, ok := orchestrator.blocked[task.ID]; ok {
		t.Fatalf("active state should clear blocked map")
	}
	if _, ok := orchestrator.retries[task.ID]; ok {
		t.Fatalf("active state should clear retry map")
	}
}

func TestUpdateTaskStateTerminalStopsRunning(t *testing.T) {
	logger := newDiscardLogger(t)
	defer logger.Close()
	orchestrator := NewOrchestrator(testWorkflow(t), logger)
	task, err := orchestrator.CreateTask(context.Background(), CreateTaskInput{Title: "Stop me"})
	if err != nil {
		t.Fatal(err)
	}
	cancelled := make(chan struct{}, 1)
	orchestrator.mu.Lock()
	orchestrator.running[task.ID] = RunningEntry{
		IssueID:    task.ID,
		Identifier: task.Identifier,
		Issue:      task,
		StartedAt:  nowUTC(),
		Cancel: func() {
			cancelled <- struct{}{}
		},
	}
	orchestrator.claimed[task.ID] = true
	orchestrator.retries[task.ID] = RetryEntry{IssueID: task.ID, Identifier: task.Identifier, Attempt: 1, DueAt: nowUTC()}
	orchestrator.mu.Unlock()

	if err := orchestrator.UpdateTaskState(context.Background(), task.ID, "Done"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatalf("terminal state did not cancel running task")
	}
	orchestrator.mu.Lock()
	defer orchestrator.mu.Unlock()
	if _, ok := orchestrator.running[task.ID]; ok {
		t.Fatalf("terminal state should remove running entry")
	}
	if orchestrator.claimed[task.ID] {
		t.Fatalf("terminal state should clear claimed entry")
	}
	if _, ok := orchestrator.retries[task.ID]; ok {
		t.Fatalf("terminal state should clear retry entry")
	}
}

func TestRetryIssueRespectsConcurrencyLimit(t *testing.T) {
	logger := newDiscardLogger(t)
	defer logger.Close()
	workflow := testWorkflow(t)
	workflow.Config.Agent.MaxConcurrentAgents = 1
	orchestrator := NewOrchestrator(workflow, logger)
	task, err := orchestrator.CreateTask(context.Background(), CreateTaskInput{Title: "Retry later"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	orchestrator.setLifecycleContext(ctx)
	orchestrator.mu.Lock()
	orchestrator.running["busy"] = RunningEntry{IssueID: "busy", Identifier: "TASK-9999", Issue: Issue{ID: "busy", Identifier: "TASK-9999", State: "Todo"}, StartedAt: nowUTC()}
	orchestrator.retries[task.ID] = RetryEntry{IssueID: task.ID, Identifier: task.Identifier, Attempt: 2, DueAt: nowUTC(), Error: "boom"}
	orchestrator.mu.Unlock()

	orchestrator.retryIssue(context.Background(), task.ID)

	orchestrator.mu.Lock()
	defer orchestrator.mu.Unlock()
	if _, ok := orchestrator.running[task.ID]; ok {
		t.Fatalf("retry dispatched despite full concurrency")
	}
	if retry, ok := orchestrator.retries[task.ID]; !ok || retry.Attempt != 2 {
		t.Fatalf("retry should remain queued, got %#v ok=%v", retry, ok)
	}
}

func TestCanceledRetryContextDoesNotDispatch(t *testing.T) {
	logger := newDiscardLogger(t)
	defer logger.Close()
	orchestrator := NewOrchestrator(testWorkflow(t), logger)
	task, err := orchestrator.CreateTask(context.Background(), CreateTaskInput{Title: "Do not retry"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	orchestrator.setLifecycleContext(ctx)
	orchestrator.scheduleRetry(task, 1, "boom", time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	orchestrator.mu.Lock()
	defer orchestrator.mu.Unlock()
	if _, ok := orchestrator.running[task.ID]; ok {
		t.Fatalf("canceled retry context should not dispatch")
	}
}

func TestAutoRunPausedSkipsCandidateDispatch(t *testing.T) {
	logger := newDiscardLogger(t)
	defer logger.Close()
	orchestrator := NewOrchestrator(testWorkflow(t), logger)
	task, err := orchestrator.CreateTask(context.Background(), CreateTaskInput{Title: "Wait for resume"})
	if err != nil {
		t.Fatal(err)
	}
	orchestrator.SetAutoRunPaused(true)

	orchestrator.dispatchCandidates(context.Background())

	orchestrator.mu.Lock()
	defer orchestrator.mu.Unlock()
	if _, ok := orchestrator.running[task.ID]; ok {
		t.Fatalf("paused auto-run should not dispatch task")
	}
	if !orchestrator.autoRunPaused {
		t.Fatalf("expected auto-run to remain paused")
	}
}

func TestRunLogRecordsCodexEventsAndBoundsEntries(t *testing.T) {
	logger := newDiscardLogger(t)
	defer logger.Close()
	workflow := testWorkflow(t)
	orchestrator := NewOrchestrator(workflow, logger)
	task, err := orchestrator.CreateTask(context.Background(), CreateTaskInput{Title: "Log me"})
	if err != nil {
		t.Fatal(err)
	}
	orchestrator.mu.Lock()
	orchestrator.running[task.ID] = RunningEntry{
		IssueID:    task.ID,
		Identifier: task.Identifier,
		Issue:      task,
		StartedAt:  nowUTC(),
	}
	orchestrator.mu.Unlock()

	for i := 0; i < maxRunLogEntriesPerTask+5; i++ {
		orchestrator.integrateCodexEvent(task.ID, RuntimeEvent{
			Event:     "turn_delta",
			Timestamp: nowUTC(),
			Details:   map[string]any{"summary": "event"},
		})
	}

	entries := orchestrator.TaskRunLog(task.ID)
	if len(entries) != maxRunLogEntriesPerTask {
		t.Fatalf("expected bounded log length %d, got %d", maxRunLogEntriesPerTask, len(entries))
	}
	if entries[len(entries)-1].Event != "turn_delta" || entries[len(entries)-1].Summary != "event" {
		t.Fatalf("unexpected last log entry: %#v", entries[len(entries)-1])
	}
	raw, err := os.ReadFile(filepath.Join(workflow.Config.Tracker.LocalRoot, "_runs", task.ID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected persisted run log")
	}
}
