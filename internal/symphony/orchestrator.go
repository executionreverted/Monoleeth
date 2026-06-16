package symphony

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Orchestrator struct {
	mu          sync.Mutex
	workflow    WorkflowDefinition
	tracker     Tracker
	workspace   *WorkspaceManager
	codex       *CodexClient
	logger      *Logger
	forcePoll   chan struct{}
	done        chan struct{}
	running     map[string]RunningEntry
	completed   map[string]bool
	claimed     map[string]bool
	blocked     map[string]BlockedEntry
	retries     map[string]RetryEntry
	nextPollDue *time.Time
	polling     bool
	lastErr     string
}

func NewOrchestrator(workflow WorkflowDefinition, logger *Logger) *Orchestrator {
	return &Orchestrator{
		workflow:  workflow,
		tracker:   NewLinearClient(workflow.Config, logger),
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
	}
}

func (o *Orchestrator) Run(ctx context.Context) error {
	defer close(o.done)
	o.cleanupTerminalWorkspaces(ctx)
	o.scheduleNextPoll(0)
	for {
		delay := o.pollDelay()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			o.stopAllRunning()
			return nil
		case <-o.forcePoll:
			timer.Stop()
			o.poll(ctx)
		case <-timer.C:
			o.poll(ctx)
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
		PollIntervalMS:      o.workflow.Config.Polling.IntervalMS,
		NextPollDueAt:       o.nextPollDue,
		PollCheckInProgress: o.polling,
		Running:             running,
		Blocked:             blocked,
		Retries:             retries,
		Completed:           sortedKeys(o.completed),
		Claimed:             sortedKeys(o.claimed),
		LastRefreshError:    o.lastErr,
	}
}

func (o *Orchestrator) IssueSnapshot(identifier string) (RunningEntry, *BlockedEntry, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, entry := range o.running {
		if entry.Identifier == identifier {
			entry.Cancel = nil
			return entry, nil, true
		}
	}
	for _, entry := range o.blocked {
		if entry.Identifier == identifier {
			copy := entry
			return RunningEntry{}, &copy, true
		}
	}
	return RunningEntry{}, nil, false
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

	if next, changed, err := ReloadWorkflowIfChanged(o.workflow); err != nil {
		o.recordError(fmt.Sprintf("workflow reload failed: %v", err))
	} else if changed {
		o.logger.Info("workflow reloaded", "path", next.Path)
		o.updateWorkflow(next)
	}

	o.reconcileRunning(ctx)
	o.dispatchCandidates(ctx)

	o.mu.Lock()
	o.polling = false
	due := time.Now().Add(durationFromMS(o.workflow.Config.Polling.IntervalMS))
	o.nextPollDue = &due
	o.mu.Unlock()
}

func (o *Orchestrator) updateWorkflow(workflow WorkflowDefinition) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.workflow = workflow
	o.tracker = NewLinearClient(workflow.Config, o.logger)
	o.workspace = NewWorkspaceManager(workflow.Config, o.logger)
	o.codex = NewCodexClient(workflow.Config, o.logger)
}

func (o *Orchestrator) dispatchCandidates(ctx context.Context) {
	if err := o.workflow.Config.ValidateDispatch(); err != nil {
		o.recordError(err.Error())
		return
	}
	issues, err := o.tracker.FetchCandidateIssues(ctx)
	if err != nil {
		o.recordError(err.Error())
		return
	}
	o.recordError("")
	issues = SortIssuesForDispatch(issues)
	for _, issue := range issues {
		if !o.shouldDispatch(issue) {
			continue
		}
		if o.availableSlotsForState(issue.State) <= 0 {
			continue
		}
		o.dispatch(ctx, issue, 0)
	}
}

func (o *Orchestrator) shouldDispatch(issue Issue) bool {
	if issue.ID == "" || issue.Identifier == "" || !issue.AssignedToWorker {
		return false
	}
	if !hasRequiredLabels(issue, o.workflow.Config.Tracker.RequiredLabels) {
		return false
	}
	if !activeIssueState(issue.State, o.workflow.Config.Tracker.ActiveStates) {
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

func (o *Orchestrator) availableSlotsForState(state string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	limit := o.workflow.Config.MaxConcurrentAgentsForState(state)
	count := 0
	for _, entry := range o.running {
		if normalizeState(entry.Issue.State) == normalizeState(state) {
			count++
		}
	}
	globalAvailable := o.workflow.Config.Agent.MaxConcurrentAgents - len(o.running)
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
	o.logger.Info("dispatching issue to agent", "issue_id", issue.ID, "issue_identifier", issue.Identifier, "attempt", attempt)

	go func() {
		err := o.runIssue(runCtx, issue, attempt)
		cancel()
		o.handleRunFinished(issue, attempt, err)
	}()
}

func (o *Orchestrator) runIssue(ctx context.Context, issue Issue, attempt int) error {
	workspace, err := o.workspace.CreateForIssue(ctx, issue)
	if err != nil {
		return err
	}
	o.updateRunning(issue.ID, func(entry *RunningEntry) {
		entry.WorkspacePath = workspace.Path
	})
	if err := o.workspace.RunBeforeRun(ctx, workspace.Path, issue); err != nil {
		return err
	}
	defer o.workspace.RunAfterRun(context.Background(), workspace.Path, issue)

	prompt, err := BuildPrompt(o.workflow.PromptTemplate, issue, attempt)
	if err != nil {
		return err
	}
	for turn := 0; turn < o.workflow.Config.Agent.MaxTurns; turn++ {
		result, err := o.codex.Run(ctx, workspace.Path, prompt, issue, func(event RuntimeEvent) {
			o.integrateCodexEvent(issue.ID, event)
		})
		o.updateRunning(issue.ID, func(entry *RunningEntry) {
			entry.SessionID = result.SessionID
			entry.ThreadID = result.ThreadID
			entry.TurnID = result.TurnID
			entry.CodexAppServerPID = result.PID
		})
		if err != nil {
			return err
		}
		if turn+1 >= o.workflow.Config.Agent.MaxTurns {
			return nil
		}
		current, err := o.tracker.FetchIssueStatesByIDs(ctx, []string{issue.ID})
		if err != nil || len(current) == 0 {
			return err
		}
		issue = current[0]
		if !activeIssueState(issue.State, o.workflow.Config.Tracker.ActiveStates) {
			return nil
		}
		prompt, err = BuildPrompt(o.workflow.PromptTemplate, issue, attempt+turn+1)
		if err != nil {
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
		o.mu.Lock()
		o.completed[issue.ID] = true
		delete(o.claimed, issue.ID)
		o.mu.Unlock()
		o.scheduleRetry(issue, 1, "", time.Second)
		return
	}

	if inputRequiredError(err) {
		o.logger.Warn("agent task blocked", "issue_id", issue.ID, "issue_identifier", issue.Identifier, "error", err)
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
	o.scheduleRetry(issue, nextAttempt, err.Error(), o.retryDelay(nextAttempt))
}

func (o *Orchestrator) scheduleRetry(issue Issue, attempt int, errorText string, delay time.Duration) {
	due := time.Now().Add(delay)
	o.mu.Lock()
	o.retries[issue.ID] = RetryEntry{IssueID: issue.ID, Identifier: issue.Identifier, Attempt: attempt, DueAt: due, Error: errorText}
	o.mu.Unlock()
	go func() {
		if sleepContext(context.Background(), delay) {
			o.retryIssue(issue.ID)
		}
	}()
}

func (o *Orchestrator) retryIssue(issueID string) {
	o.mu.Lock()
	retry, ok := o.retries[issueID]
	o.mu.Unlock()
	if !ok {
		return
	}
	ctx := context.Background()
	issues, err := o.tracker.FetchIssueStatesByIDs(ctx, []string{issueID})
	if err != nil || len(issues) == 0 {
		if err == nil {
			err = fmt.Errorf("issue not visible")
		}
		o.recordError(fmt.Sprintf("retry lookup failed: %v", err))
		return
	}
	issue := issues[0]
	if !o.shouldRetryDispatch(issue) {
		o.mu.Lock()
		delete(o.claimed, issueID)
		delete(o.retries, issueID)
		o.mu.Unlock()
		return
	}
	o.dispatch(ctx, issue, retry.Attempt)
}

func (o *Orchestrator) shouldRetryDispatch(issue Issue) bool {
	if terminalIssueState(issue.State, o.workflow.Config.Tracker.TerminalStates) {
		o.workspace.RemoveIssueWorkspace(context.Background(), issue.Identifier)
		return false
	}
	if !activeIssueState(issue.State, o.workflow.Config.Tracker.ActiveStates) {
		return false
	}
	if !hasRequiredLabels(issue, o.workflow.Config.Tracker.RequiredLabels) || !issue.AssignedToWorker {
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
	maxDelay := durationFromMS(o.workflow.Config.Agent.MaxRetryBackoffMS)
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

func (o *Orchestrator) reconcileRunning(ctx context.Context) {
	o.restartStalled()
	o.mu.Lock()
	ids := make([]string, 0, len(o.running))
	for id := range o.running {
		ids = append(ids, id)
	}
	o.mu.Unlock()
	if len(ids) == 0 {
		return
	}
	issues, err := o.tracker.FetchIssueStatesByIDs(ctx, ids)
	if err != nil {
		o.logger.Debug("failed to refresh running issue states", "error", err)
		return
	}
	visible := map[string]Issue{}
	for _, issue := range issues {
		visible[issue.ID] = issue
		if terminalIssueState(issue.State, o.workflow.Config.Tracker.TerminalStates) {
			o.stopRunning(issue.ID, true)
			continue
		}
		if !activeIssueState(issue.State, o.workflow.Config.Tracker.ActiveStates) || !hasRequiredLabels(issue, o.workflow.Config.Tracker.RequiredLabels) || !issue.AssignedToWorker {
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
	timeout := durationFromMS(o.workflow.Config.Codex.StallTimeoutMS)
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
	if cleanup {
		o.workspace.RemoveIssueWorkspace(context.Background(), entry.Identifier)
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
	if o.workflow.Config.Tracker.Kind != "linear" || o.workflow.Config.ValidateDispatch() != nil {
		return
	}
	issues, err := o.tracker.FetchIssuesByStates(ctx, o.workflow.Config.Tracker.TerminalStates)
	if err != nil {
		o.logger.Debug("terminal workspace cleanup skipped", "error", err)
		return
	}
	for _, issue := range issues {
		o.workspace.RemoveIssueWorkspace(ctx, issue.Identifier)
	}
}

func (o *Orchestrator) integrateCodexEvent(issueID string, event RuntimeEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	entry, ok := o.running[issueID]
	if !ok {
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
