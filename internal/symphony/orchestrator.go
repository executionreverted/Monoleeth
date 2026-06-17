package symphony

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxRunLogEntriesPerTask = 1000

type Orchestrator struct {
	mu            sync.Mutex
	workflow      WorkflowDefinition
	tracker       Tracker
	workspace     *WorkspaceManager
	codex         *CodexClient
	logger        *Logger
	forcePoll     chan struct{}
	done          chan struct{}
	lifecycleCtx  context.Context
	running       map[string]RunningEntry
	completed     map[string]bool
	claimed       map[string]bool
	blocked       map[string]BlockedEntry
	retries       map[string]RetryEntry
	runLogs       map[string][]RunLogEntry
	autoRunPaused bool
	nextPollDue   *time.Time
	polling       bool
	lastErr       string
}

func NewOrchestrator(workflow WorkflowDefinition, logger *Logger) *Orchestrator {
	return &Orchestrator{
		workflow:  workflow,
		tracker:   NewTracker(workflow.Config, logger),
		workspace: NewWorkspaceManager(workflow.Config, logger),
		codex:     NewCodexClient(workflow.Config, logger),
		logger:    logger,
		forcePoll: make(chan struct{}, 1),
		done:      make(chan struct{}),
		running:   map[string]RunningEntry{},
		completed: map[string]bool{},
		claimed:   map[string]bool{},
		blocked:   map[string]BlockedEntry{},
		retries:   map[string]RetryEntry{},
		runLogs:   map[string][]RunLogEntry{},
	}
}

func (o *Orchestrator) Run(ctx context.Context) error {
	defer close(o.done)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	o.setLifecycleContext(runCtx)
	o.cleanupTerminalWorkspaces(runCtx)
	o.scheduleNextPoll(0)
	for {
		delay := o.pollDelay()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			cancel()
			o.stopAllRunning()
			return nil
		case <-o.forcePoll:
			timer.Stop()
			o.poll(runCtx)
		case <-timer.C:
			o.poll(runCtx)
		}
	}
}

func (o *Orchestrator) ForcePoll() {
	select {
	case o.forcePoll <- struct{}{}:
	default:
	}
}

func (o *Orchestrator) Snapshot() Snapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	running := make(map[string]RunningEntry, len(o.running))
	for k, v := range o.running {
		v.Cancel = nil
		running[k] = v
	}
	blocked := make(map[string]BlockedEntry, len(o.blocked))
	for k, v := range o.blocked {
		blocked[k] = v
	}
	retries := make(map[string]RetryEntry, len(o.retries))
	for k, v := range o.retries {
		retries[k] = v
	}
	return Snapshot{
		WorkflowPath:        o.workflow.Path,
		WorkspaceRoot:       o.workflow.Config.Workspace.Root,
		PollIntervalMS:      o.workflow.Config.Polling.IntervalMS,
		TrackerKind:         o.workflow.Config.Tracker.Kind,
		NextPollDueAt:       o.nextPollDue,
		PollCheckInProgress: o.polling,
		AutoRunPaused:       o.autoRunPaused,
		Tasks:               o.localTasksLocked(),
		Running:             running,
		Blocked:             blocked,
		Retries:             retries,
		Completed:           sortedKeys(o.completed),
		Claimed:             sortedKeys(o.claimed),
		LastRefreshError:    o.lastErr,
	}
}

func (o *Orchestrator) StatePayload() map[string]any {
	return StatePayload(o.Snapshot())
}

func (o *Orchestrator) SetAutoRunPaused(paused bool) {
	o.mu.Lock()
	changed := o.autoRunPaused != paused
	o.autoRunPaused = paused
	retryIDs := make([]string, 0, len(o.retries))
	if changed && !paused {
		for id := range o.retries {
			retryIDs = append(retryIDs, id)
		}
	}
	o.mu.Unlock()
	if changed && !paused {
		o.ForcePoll()
		ctx := o.retryContext()
		for _, id := range retryIDs {
			go o.retryIssue(ctx, id)
		}
	}
}

func (o *Orchestrator) AutoRunPaused() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.autoRunPaused
}

func (o *Orchestrator) TaskRunLog(issueID string) []RunLogEntry {
	o.mu.Lock()
	workflow := o.workflow
	defer o.mu.Unlock()
	entries := o.runLogs[issueID]
	if len(entries) == 0 {
		entries = readRunLogFile(workflow, issueID)
		if len(entries) > maxRunLogEntriesPerTask {
			entries = entries[len(entries)-maxRunLogEntriesPerTask:]
		}
		if len(entries) > 0 {
			o.runLogs[issueID] = append([]RunLogEntry(nil), entries...)
		}
	}
	out := make([]RunLogEntry, len(entries))
	copy(out, entries)
	return out
}

func (o *Orchestrator) IssuePayload(identifier string) (map[string]any, bool) {
	return IssuePayload(o.Snapshot(), identifier)
}

func (o *Orchestrator) setLifecycleContext(ctx context.Context) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.lifecycleCtx = ctx
}

func (o *Orchestrator) retryContext() context.Context {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lifecycleCtx == nil {
		return context.Background()
	}
	return o.lifecycleCtx
}

func (o *Orchestrator) workflowSnapshot() WorkflowDefinition {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.workflow
}

func (o *Orchestrator) components() (WorkflowDefinition, Tracker, *WorkspaceManager, *CodexClient) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.workflow, o.tracker, o.workspace, o.codex
}

func (o *Orchestrator) pollDelay() time.Duration {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.nextPollDue == nil {
		return 0
	}
	delay := time.Until(*o.nextPollDue)
	if delay < 0 {
		return 0
	}
	return delay
}

func (o *Orchestrator) scheduleNextPoll(delay time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	due := time.Now().Add(delay)
	o.nextPollDue = &due
}

func (o *Orchestrator) poll(ctx context.Context) {
	o.mu.Lock()
	o.polling = true
	o.nextPollDue = nil
	o.mu.Unlock()

	workflow := o.workflowSnapshot()
	if next, changed, err := ReloadWorkflowIfChanged(workflow); err != nil {
		o.recordError(fmt.Sprintf("workflow reload failed: %v", err))
	} else if changed {
		o.logger.Info("workflow reloaded", "path", next.Path)
		o.updateWorkflow(next)
	}

	o.reconcileRunning(ctx)
	o.dispatchCandidates(ctx)

	o.mu.Lock()
	o.polling = false
	workflow = o.workflow
	due := time.Now().Add(durationFromMS(workflow.Config.Polling.IntervalMS))
	o.nextPollDue = &due
	o.mu.Unlock()
}

func (o *Orchestrator) updateWorkflow(workflow WorkflowDefinition) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.workflow = workflow
	o.tracker = NewTracker(workflow.Config, o.logger)
	o.workspace = NewWorkspaceManager(workflow.Config, o.logger)
	o.codex = NewCodexClient(workflow.Config, o.logger)
}

func (o *Orchestrator) CreateTask(ctx context.Context, input CreateTaskInput) (Issue, error) {
	workflow, tracker, _, _ := o.components()
	creator, ok := tracker.(LocalTaskCreator)
	if !ok {
		return Issue{}, fmt.Errorf("tracker %q does not support local task creation", workflow.Config.Tracker.Kind)
	}
	issue, err := creator.CreateTask(ctx, input)
	if err != nil {
		return Issue{}, err
	}
	o.ForcePoll()
	return issue, nil
}

func (o *Orchestrator) ListTasks(ctx context.Context) ([]Issue, error) {
	workflow, tracker, _, _ := o.components()
	creator, ok := tracker.(LocalTaskCreator)
	if !ok {
		return nil, fmt.Errorf("tracker %q does not support local task listing", workflow.Config.Tracker.Kind)
	}
	return creator.ListTasks(ctx)
}

func (o *Orchestrator) UpdateTaskState(ctx context.Context, issueID string, state string) error {
	workflow, tracker, _, _ := o.components()
	updater, ok := tracker.(IssueStateUpdater)
	if !ok {
		return fmt.Errorf("tracker %q does not support state updates", workflow.Config.Tracker.Kind)
	}
	if err := updater.UpdateIssueState(ctx, issueID, state); err != nil {
		return err
	}
	if activeIssueState(state, workflow.Config.Tracker.ActiveStates) {
		o.mu.Lock()
		delete(o.completed, issueID)
		delete(o.blocked, issueID)
		delete(o.retries, issueID)
		delete(o.claimed, issueID)
		o.mu.Unlock()
		o.ForcePoll()
		return nil
	}
	if terminalIssueState(state, workflow.Config.Tracker.TerminalStates) {
		o.stopRunning(issueID, true)
		o.mu.Lock()
		o.completed[issueID] = true
		delete(o.blocked, issueID)
		delete(o.retries, issueID)
		delete(o.claimed, issueID)
		o.mu.Unlock()
		o.ForcePoll()
	}
	return nil
}

func (o *Orchestrator) RunTask(ctx context.Context, issueID string) error {
	if err := o.UpdateTaskState(ctx, issueID, "Todo"); err != nil {
		return err
	}
	workflow, tracker, _, _ := o.components()
	issues, err := tracker.FetchIssueStatesByIDs(ctx, []string{issueID})
	if err != nil {
		return err
	}
	if len(issues) == 0 {
		return fmt.Errorf("task %q not found", issueID)
	}
	issue := issues[0]
	if !o.shouldDispatch(issue, workflow) {
		return nil
	}
	if o.availableSlotsForState(issue.State, workflow) <= 0 {
		o.ForcePoll()
		return nil
	}
	o.dispatch(o.retryContext(), issue, 0)
	return nil
}

func (o *Orchestrator) dispatchCandidates(ctx context.Context) {
	if o.AutoRunPaused() {
		o.recordError("")
		return
	}
	workflow, tracker, _, _ := o.components()
	if err := workflow.Config.ValidateDispatch(); err != nil {
		o.recordError(err.Error())
		return
	}
	issues, err := tracker.FetchCandidateIssues(ctx)
	if err != nil {
		o.recordError(err.Error())
		return
	}
	o.recordError("")
	issues = SortIssuesForDispatch(issues)
	for _, issue := range issues {
		if !o.shouldDispatch(issue, workflow) {
			continue
		}
		if o.availableSlotsForState(issue.State, workflow) <= 0 {
			continue
		}
		o.dispatch(ctx, issue, 0)
	}
}

func (o *Orchestrator) shouldDispatch(issue Issue, workflow WorkflowDefinition) bool {
	if issue.ID == "" || issue.Identifier == "" || !issue.AssignedToWorker {
		return false
	}
	if !hasRequiredLabels(issue, workflow.Config.Tracker.RequiredLabels) {
		return false
	}
	if !activeIssueState(issue.State, workflow.Config.Tracker.ActiveStates) {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.claimed[issue.ID] || o.completed[issue.ID] {
		return false
	}
	if _, ok := o.blocked[issue.ID]; ok {
		return false
	}
	return true
}

func (o *Orchestrator) availableSlotsForState(state string, workflow WorkflowDefinition) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	limit := workflow.Config.MaxConcurrentAgentsForState(state)
	count := 0
	for _, entry := range o.running {
		if normalizeState(entry.Issue.State) == normalizeState(state) {
			count++
		}
	}
	globalAvailable := workflow.Config.Agent.MaxConcurrentAgents - len(o.running)
	stateAvailable := limit - count
	if globalAvailable < stateAvailable {
		return globalAvailable
	}
	return stateAvailable
}

func (o *Orchestrator) dispatch(ctx context.Context, issue Issue, attempt int) {
	runCtx, cancel := context.WithCancel(ctx)
	entry := RunningEntry{
		IssueID:    issue.ID,
		Identifier: issue.Identifier,
		Issue:      issue,
		Attempt:    attempt,
		StartedAt:  nowUTC(),
		Cancel:     cancel,
	}
	o.mu.Lock()
	o.claimed[issue.ID] = true
	o.running[issue.ID] = entry
	delete(o.retries, issue.ID)
	o.mu.Unlock()
	o.recordRunEvent(issue, attempt, 0, "dispatch_started", map[string]any{"state": issue.State}, "Dispatched to agent")
	o.logger.Info("dispatching issue to agent", "issue_id", issue.ID, "issue_identifier", issue.Identifier, "attempt", attempt)
	o.updateTrackerState(context.Background(), issue.ID, "In Progress")

	go func() {
		err := o.runIssue(runCtx, issue, attempt)
		cancel()
		o.handleRunFinished(issue, attempt, err)
	}()
}

func (o *Orchestrator) runIssue(ctx context.Context, issue Issue, attempt int) error {
	workflow, tracker, workspaceManager, codex := o.components()
	workspace, err := workspaceManager.CreateForIssue(ctx, issue)
	if err != nil {
		o.recordRunEvent(issue, attempt, 0, "workspace_create_failed", map[string]any{"error": err.Error()}, err.Error())
		return err
	}
	o.updateRunning(issue.ID, func(entry *RunningEntry) {
		entry.WorkspacePath = workspace.Path
	})
	if err := workspaceManager.RunBeforeRun(ctx, workspace.Path, issue); err != nil {
		o.recordRunEvent(issue, attempt, 0, "before_run_failed", map[string]any{"error": err.Error()}, err.Error())
		return err
	}
	defer workspaceManager.RunAfterRun(context.Background(), workspace.Path, issue)

	prompt, err := BuildPrompt(workflow.PromptTemplate, issue, attempt)
	if err != nil {
		o.recordRunEvent(issue, attempt, 0, "prompt_failed", map[string]any{"error": err.Error()}, err.Error())
		return err
	}
	for turn := 0; turn < workflow.Config.Agent.MaxTurns; turn++ {
		result, err := codex.Run(ctx, workspace.Path, prompt, issue, func(event RuntimeEvent) {
			o.integrateCodexEvent(issue.ID, event)
		})
		o.updateRunning(issue.ID, func(entry *RunningEntry) {
			entry.SessionID = result.SessionID
			entry.ThreadID = result.ThreadID
			entry.TurnID = result.TurnID
			entry.CodexAppServerPID = result.PID
			entry.TurnCount = turn + 1
		})
		if err != nil {
			o.recordRunEvent(issue, attempt, turn+1, "turn_error", map[string]any{"error": err.Error()}, err.Error())
			return err
		}
		if workflow.Config.Tracker.Kind == "local" || workflow.Config.Tracker.Kind == "memory" {
			return nil
		}
		if turn+1 >= workflow.Config.Agent.MaxTurns {
			o.recordRunEvent(issue, attempt, turn+1, "max_turns_reached", nil, "Max turns reached")
			return nil
		}
		current, err := tracker.FetchIssueStatesByIDs(ctx, []string{issue.ID})
		if err != nil || len(current) == 0 {
			if err != nil {
				o.recordRunEvent(issue, attempt, turn+1, "state_refresh_failed", map[string]any{"error": err.Error()}, err.Error())
			}
			return err
		}
		issue = current[0]
		if !activeIssueState(issue.State, workflow.Config.Tracker.ActiveStates) {
			o.recordRunEvent(issue, attempt, turn+1, "state_no_longer_active", map[string]any{"state": issue.State}, "Task state is no longer active")
			return nil
		}
		prompt, err = BuildPrompt(workflow.PromptTemplate, issue, attempt+turn+1)
		if err != nil {
			o.recordRunEvent(issue, attempt, turn+1, "prompt_failed", map[string]any{"error": err.Error()}, err.Error())
			return err
		}
	}
	return nil
}

func (o *Orchestrator) handleRunFinished(issue Issue, attempt int, err error) {
	o.mu.Lock()
	entry := o.running[issue.ID]
	delete(o.running, issue.ID)
	o.mu.Unlock()

	if err == nil {
		o.logger.Info("agent task completed", "issue_id", issue.ID, "issue_identifier", issue.Identifier)
		o.recordRunEvent(issue, attempt, entry.TurnCount, "task_completed", nil, "Task completed")
		o.updateTrackerState(context.Background(), issue.ID, "Done")
		o.mu.Lock()
		o.completed[issue.ID] = true
		delete(o.claimed, issue.ID)
		workflow := o.workflow
		o.mu.Unlock()
		if workflow.Config.Tracker.Kind == "local" || workflow.Config.Tracker.Kind == "memory" {
			return
		}
		o.scheduleRetry(issue, 1, "", time.Second)
		return
	}

	if inputRequiredError(err) {
		o.logger.Warn("agent task blocked", "issue_id", issue.ID, "issue_identifier", issue.Identifier, "error", err)
		o.recordRunEvent(issue, attempt, entry.TurnCount, "task_blocked", map[string]any{"error": err.Error()}, err.Error())
		o.updateTrackerState(context.Background(), issue.ID, "Blocked")
		o.mu.Lock()
		o.blocked[issue.ID] = BlockedEntry{
			IssueID:       issue.ID,
			Identifier:    issue.Identifier,
			Issue:         issue,
			WorkspacePath: entry.WorkspacePath,
			SessionID:     entry.SessionID,
			Error:         err.Error(),
			BlockedAt:     nowUTC(),
			LastEvent:     entry.LastCodexEvent,
			LastMessage:   entry.LastCodexMessage,
			LastTimestamp: entry.LastCodexTimestamp,
		}
		o.mu.Unlock()
		return
	}

	nextAttempt := attempt + 1
	if nextAttempt <= 0 {
		nextAttempt = 1
	}
	o.logger.Warn("agent task failed; scheduling retry", "issue_id", issue.ID, "issue_identifier", issue.Identifier, "attempt", nextAttempt, "error", err)
	o.recordRunEvent(issue, nextAttempt, entry.TurnCount, "task_failed_retry_scheduled", map[string]any{"error": err.Error()}, err.Error())
	o.scheduleRetry(issue, nextAttempt, err.Error(), o.retryDelay(nextAttempt))
}

func (o *Orchestrator) updateTrackerState(ctx context.Context, issueID string, state string) {
	_, tracker, _, _ := o.components()
	updater, ok := tracker.(IssueStateUpdater)
	if !ok {
		return
	}
	if err := updater.UpdateIssueState(ctx, issueID, state); err != nil {
		o.logger.Warn("failed to update tracker issue state", "issue_id", issueID, "state", state, "error", err)
	}
}

func (o *Orchestrator) scheduleRetry(issue Issue, attempt int, errorText string, delay time.Duration) {
	due := nowUTC().Add(delay)
	o.mu.Lock()
	o.retries[issue.ID] = RetryEntry{IssueID: issue.ID, Identifier: issue.Identifier, Attempt: attempt, DueAt: due, Error: errorText}
	o.mu.Unlock()
	o.recordRunEvent(issue, attempt, 0, "retry_scheduled", map[string]any{"due_at": iso8601(due), "error": errorText}, "Retry scheduled")
	ctx := o.retryContext()
	go func() {
		if sleepContext(ctx, delay) {
			o.retryIssue(ctx, issue.ID)
		}
	}()
}

func (o *Orchestrator) retryIssue(ctx context.Context, issueID string) {
	if o.AutoRunPaused() {
		return
	}
	o.mu.Lock()
	retry, ok := o.retries[issueID]
	o.mu.Unlock()
	if !ok {
		return
	}
	workflow, tracker, _, _ := o.components()
	issues, err := tracker.FetchIssueStatesByIDs(ctx, []string{issueID})
	if err != nil || len(issues) == 0 {
		if err == nil {
			err = fmt.Errorf("issue not visible")
		}
		o.recordError(fmt.Sprintf("retry lookup failed: %v", err))
		return
	}
	issue := issues[0]
	if !o.shouldRetryDispatch(issue, workflow) {
		o.mu.Lock()
		delete(o.claimed, issueID)
		delete(o.retries, issueID)
		o.mu.Unlock()
		return
	}
	if o.availableSlotsForState(issue.State, workflow) <= 0 {
		o.scheduleRetry(issue, retry.Attempt, retry.Error, time.Second)
		return
	}
	o.dispatch(ctx, issue, retry.Attempt)
}

func (o *Orchestrator) shouldRetryDispatch(issue Issue, workflow WorkflowDefinition) bool {
	if terminalIssueState(issue.State, workflow.Config.Tracker.TerminalStates) {
		_, _, workspace, _ := o.components()
		workspace.RemoveIssueWorkspace(context.Background(), issue.Identifier)
		return false
	}
	if !activeIssueState(issue.State, workflow.Config.Tracker.ActiveStates) {
		return false
	}
	if !hasRequiredLabels(issue, workflow.Config.Tracker.RequiredLabels) || !issue.AssignedToWorker {
		return false
	}
	return true
}

func (o *Orchestrator) retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base := 10 * time.Second
	delay := base << min(attempt-1, 10)
	workflow := o.workflowSnapshot()
	maxDelay := durationFromMS(workflow.Config.Agent.MaxRetryBackoffMS)
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

func (o *Orchestrator) reconcileRunning(ctx context.Context) {
	o.restartStalled()
	workflow, tracker, _, _ := o.components()
	o.mu.Lock()
	ids := make([]string, 0, len(o.running))
	for id := range o.running {
		ids = append(ids, id)
	}
	o.mu.Unlock()
	if len(ids) == 0 {
		return
	}
	issues, err := tracker.FetchIssueStatesByIDs(ctx, ids)
	if err != nil {
		o.logger.Debug("failed to refresh running issue states", "error", err)
		return
	}
	visible := map[string]Issue{}
	for _, issue := range issues {
		visible[issue.ID] = issue
		if terminalIssueState(issue.State, workflow.Config.Tracker.TerminalStates) {
			o.stopRunning(issue.ID, true)
			continue
		}
		if !activeIssueState(issue.State, workflow.Config.Tracker.ActiveStates) || !hasRequiredLabels(issue, workflow.Config.Tracker.RequiredLabels) || !issue.AssignedToWorker {
			o.stopRunning(issue.ID, false)
			continue
		}
		o.updateRunning(issue.ID, func(entry *RunningEntry) { entry.Issue = issue })
	}
	for _, id := range ids {
		if _, ok := visible[id]; !ok {
			o.stopRunning(id, false)
		}
	}
}

func (o *Orchestrator) restartStalled() {
	workflow := o.workflowSnapshot()
	timeout := durationFromMS(workflow.Config.Codex.StallTimeoutMS)
	if timeout <= 0 {
		return
	}
	var stalled []RunningEntry
	o.mu.Lock()
	for _, entry := range o.running {
		last := entry.StartedAt
		if entry.LastCodexTimestamp != nil {
			last = *entry.LastCodexTimestamp
		}
		if time.Since(last) > timeout {
			stalled = append(stalled, entry)
		}
	}
	o.mu.Unlock()
	for _, entry := range stalled {
		o.stopRunning(entry.IssueID, false)
		o.scheduleRetry(entry.Issue, entry.Attempt+1, "stalled without codex activity", o.retryDelay(entry.Attempt+1))
	}
}

func (o *Orchestrator) stopRunning(issueID string, cleanup bool) {
	_, _, workspace, _ := o.components()
	o.mu.Lock()
	entry, ok := o.running[issueID]
	if ok {
		delete(o.running, issueID)
		delete(o.claimed, issueID)
		delete(o.retries, issueID)
	}
	o.mu.Unlock()
	if !ok {
		return
	}
	if entry.Cancel != nil {
		entry.Cancel()
	}
	o.recordRunEvent(entry.Issue, entry.Attempt, entry.TurnCount, "task_stopped", map[string]any{"cleanup": cleanup}, "Task stopped")
	if cleanup {
		workspace.RemoveIssueWorkspace(context.Background(), entry.Identifier)
	}
}

func (o *Orchestrator) stopAllRunning() {
	o.mu.Lock()
	entries := make([]RunningEntry, 0, len(o.running))
	for _, entry := range o.running {
		entries = append(entries, entry)
	}
	o.mu.Unlock()
	for _, entry := range entries {
		if entry.Cancel != nil {
			entry.Cancel()
		}
	}
}

func (o *Orchestrator) cleanupTerminalWorkspaces(ctx context.Context) {
	workflow, tracker, workspace, _ := o.components()
	if workflow.Config.Tracker.Kind != "linear" || workflow.Config.ValidateDispatch() != nil {
		return
	}
	issues, err := tracker.FetchIssuesByStates(ctx, workflow.Config.Tracker.TerminalStates)
	if err != nil {
		o.logger.Debug("terminal workspace cleanup skipped", "error", err)
		return
	}
	for _, issue := range issues {
		workspace.RemoveIssueWorkspace(ctx, issue.Identifier)
	}
}

func (o *Orchestrator) localTasksLocked() []Issue {
	creator, ok := o.tracker.(LocalTaskCreator)
	if !ok {
		return nil
	}
	tasks, err := creator.ListTasks(context.Background())
	if err != nil {
		return nil
	}
	return tasks
}

func (o *Orchestrator) integrateCodexEvent(issueID string, event RuntimeEvent) {
	o.mu.Lock()
	entry, ok := o.running[issueID]
	if !ok {
		o.mu.Unlock()
		return
	}
	entry.LastCodexEvent = event.Event
	entry.LastCodexTimestamp = &event.Timestamp
	entry.LastCodexMessage = event.Details
	if event.Event == "session_started" {
		if v, _ := event.Details["session_id"].(string); v != "" {
			entry.SessionID = v
		}
		if v, _ := event.Details["thread_id"].(string); v != "" {
			entry.ThreadID = v
		}
		if v, _ := event.Details["turn_id"].(string); v != "" {
			entry.TurnID = v
		}
		if v := intID(event.Details["codex_app_server_pid"]); v > 0 {
			entry.CodexAppServerPID = v
		}
	}
	o.running[issueID] = entry
	issue := entry.Issue
	attempt := entry.Attempt
	turnCount := entry.TurnCount
	o.mu.Unlock()
	o.recordRunEvent(issue, attempt, turnCount, event.Event, event.Details, runtimeEventSummary(event))
}

func (o *Orchestrator) recordRunEvent(issue Issue, attempt int, turnCount int, event string, details map[string]any, summary string) {
	if issue.ID == "" {
		return
	}
	entry := RunLogEntry{
		Timestamp:  nowUTC(),
		IssueID:    issue.ID,
		Identifier: issue.Identifier,
		Event:      event,
		Attempt:    attempt,
		TurnCount:  turnCount,
		Summary:    summary,
		Details:    details,
	}
	o.mu.Lock()
	entries := append(o.runLogs[issue.ID], entry)
	if len(entries) > maxRunLogEntriesPerTask {
		entries = append([]RunLogEntry(nil), entries[len(entries)-maxRunLogEntriesPerTask:]...)
	}
	o.runLogs[issue.ID] = entries
	workflow := o.workflow
	o.mu.Unlock()
	if err := appendRunLogFile(workflow, entry); err != nil {
		o.logger.Warn("failed to append task run log", "issue_id", issue.ID, "error", err)
	}
}

func appendRunLogFile(workflow WorkflowDefinition, entry RunLogEntry) error {
	dir := runLogDir(workflow)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, safeRunLogName(entry.IssueID)+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

func readRunLogFile(workflow WorkflowDefinition, issueID string) []RunLogEntry {
	file, err := os.Open(filepath.Join(runLogDir(workflow), safeRunLogName(issueID)+".jsonl"))
	if err != nil {
		return nil
	}
	defer file.Close()
	var entries []RunLogEntry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var entry RunLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			entries = append(entries, entry)
		}
	}
	return entries
}

func runLogDir(workflow WorkflowDefinition) string {
	if strings.TrimSpace(workflow.Config.Tracker.LocalRoot) == "" {
		return filepath.Join(workflow.Dir, ".symphony", "tasks", "_runs")
	}
	return filepath.Join(workflow.Config.Tracker.LocalRoot, "_runs")
}

func safeRunLogName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func runtimeEventSummary(event RuntimeEvent) string {
	if text, ok := summarizeCodexMessage(event.Details).(string); ok {
		return text
	}
	return event.Event
}

func (o *Orchestrator) updateRunning(issueID string, update func(*RunningEntry)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	entry, ok := o.running[issueID]
	if !ok {
		return
	}
	update(&entry)
	o.running[issueID] = entry
}

func (o *Orchestrator) recordError(message string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.lastErr = message
	if message != "" {
		o.logger.Error("orchestrator error", "error", message)
	}
}

func SortIssuesForDispatch(issues []Issue) []Issue {
	out := append([]Issue(nil), issues...)
	sort.SliceStable(out, func(i, j int) bool {
		leftPriority := 999999
		rightPriority := 999999
		if out[i].Priority != nil {
			leftPriority = *out[i].Priority
		}
		if out[j].Priority != nil {
			rightPriority = *out[j].Priority
		}
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if out[i].UpdatedAt != nil && out[j].UpdatedAt != nil && !out[i].UpdatedAt.Equal(*out[j].UpdatedAt) {
			return out[i].UpdatedAt.Before(*out[j].UpdatedAt)
		}
		return out[i].Identifier < out[j].Identifier
	})
	return out
}

func hasRequiredLabels(issue Issue, required []string) bool {
	if len(required) == 0 {
		return true
	}
	labels := map[string]bool{}
	for _, label := range issue.Labels {
		labels[normalizeState(label)] = true
	}
	for _, label := range required {
		if !labels[normalizeState(label)] {
			return false
		}
	}
	return true
}

func activeIssueState(state string, active []string) bool {
	state = normalizeState(state)
	for _, candidate := range active {
		if normalizeState(candidate) == state {
			return true
		}
	}
	return false
}

func terminalIssueState(state string, terminal []string) bool {
	state = normalizeState(state)
	for _, candidate := range terminal {
		if normalizeState(candidate) == state {
			return true
		}
	}
	return false
}

func inputRequiredError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "operator input required") || strings.Contains(text, "approval required")
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
