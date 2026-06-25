package symphony

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CreateTaskInput struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Labels        []string `json:"labels"`
	Priority      *int     `json:"priority,omitempty"`
	AgentBackend  string   `json:"agent_backend,omitempty"`
	AgentModel    string   `json:"agent_model,omitempty"`
	AgentEndpoint string   `json:"agent_endpoint,omitempty"`
}

type LocalTracker struct {
	config Config
	logger *Logger
	mu     sync.Mutex
}

func NewLocalTracker(config Config, logger *Logger) *LocalTracker {
	return &LocalTracker{config: config, logger: logger}
}

func NewTracker(config Config, logger *Logger) Tracker {
	switch config.Tracker.Kind {
	case "local", "memory":
		return NewLocalTracker(config, logger)
	default:
		return NewLinearClient(config, logger)
	}
}

func (t *LocalTracker) FetchCandidateIssues(ctx context.Context) ([]Issue, error) {
	return t.FetchIssuesByStates(ctx, t.config.Tracker.ActiveStates)
}

func (t *LocalTracker) FetchIssuesByStates(_ context.Context, states []string) ([]Issue, error) {
	tasks, err := t.ListTasks(context.Background())
	if err != nil {
		return nil, err
	}
	var out []Issue
	for _, task := range tasks {
		if activeIssueState(task.State, states) && hasRequiredLabels(task, t.config.Tracker.RequiredLabels) {
			out = append(out, task)
		}
	}
	return out, nil
}

func (t *LocalTracker) FetchIssueStatesByIDs(_ context.Context, ids []string) ([]Issue, error) {
	tasks, err := t.ListTasks(context.Background())
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	order := map[string]int{}
	for i, id := range ids {
		want[id] = true
		order[id] = i
	}
	var out []Issue
	for _, task := range tasks {
		if want[task.ID] {
			out = append(out, task)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return order[out[i].ID] < order[out[j].ID]
	})
	return out, nil
}

func (t *LocalTracker) GraphQL(context.Context, string, map[string]any, string) (map[string]any, error) {
	return nil, errors.New("local tracker does not support GraphQL")
}

func (t *LocalTracker) ListTasks(_ context.Context) ([]Issue, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := os.MkdirAll(t.config.Tracker.LocalRoot, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(t.config.Tracker.LocalRoot)
	if err != nil {
		return nil, err
	}
	tasks := []Issue{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		task, err := t.readTaskLocked(filepath.Join(t.config.Tracker.LocalRoot, entry.Name()))
		if err != nil {
			t.logger.Warn("skipping unreadable local task", "file", entry.Name(), "error", err)
			continue
		}
		tasks = append(tasks, task)
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].CreatedAt != nil && tasks[j].CreatedAt != nil && !tasks[i].CreatedAt.Equal(*tasks[j].CreatedAt) {
			return tasks[i].CreatedAt.Before(*tasks[j].CreatedAt)
		}
		return tasks[i].Identifier < tasks[j].Identifier
	})
	return tasks, nil
}

func (t *LocalTracker) CreateTask(_ context.Context, input CreateTaskInput) (Issue, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return Issue{}, errors.New("title is required")
	}
	if err := os.MkdirAll(t.config.Tracker.LocalRoot, 0o755); err != nil {
		return Issue{}, err
	}
	now := nowUTC()
	next, err := t.nextTaskNumberLocked()
	if err != nil {
		return Issue{}, err
	}
	var issue Issue
	for attempts := 0; attempts < 100; attempts++ {
		number := next + attempts
		issue = Issue{
			ID:               fmt.Sprintf("local-%04d", number),
			Identifier:       fmt.Sprintf("TASK-%04d", number),
			Title:            title,
			Description:      strings.TrimSpace(input.Description),
			Priority:         input.Priority,
			State:            "Todo",
			URL:              "",
			Labels:           t.defaultLabels(input.Labels),
			BlockedBy:        []BlockerRef{},
			AssignedToWorker: true,
			AgentBackend:     normalizeOptionalBackend(input.AgentBackend),
			AgentModel:       strings.TrimSpace(input.AgentModel),
			AgentEndpoint:    strings.TrimSpace(input.AgentEndpoint),
			CreatedAt:        &now,
			UpdatedAt:        &now,
		}
		if err := t.createTaskFileLocked(issue); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return Issue{}, err
		}
		return issue, nil
	}
	return Issue{}, errors.New("could not allocate local task id")
}

func (t *LocalTracker) UpdateIssueState(_ context.Context, issueID string, state string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	state = strings.TrimSpace(state)
	if state == "" {
		return errors.New("state is required")
	}
	path := t.pathForID(issueID)
	task, err := t.readTaskLocked(path)
	if err != nil {
		return err
	}
	now := nowUTC()
	task.State = state
	task.UpdatedAt = &now
	return t.writeTaskLocked(task)
}

func (t *LocalTracker) nextTaskNumberLocked() (int, error) {
	entries, err := os.ReadDir(t.config.Tracker.LocalRoot)
	if err != nil {
		return 0, err
	}
	maxSeen := 0
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		name = strings.TrimPrefix(name, "local-")
		value, err := strconv.Atoi(name)
		if err == nil && value > maxSeen {
			maxSeen = value
		}
	}
	return maxSeen + 1, nil
}

func (t *LocalTracker) readTaskLocked(path string) (Issue, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Issue{}, err
	}
	var issue Issue
	if err := json.Unmarshal(raw, &issue); err != nil {
		return Issue{}, err
	}
	issue.AssignedToWorker = true
	issue.Labels = normalizeLabels(issue.Labels)
	if issue.BlockedBy == nil {
		issue.BlockedBy = []BlockerRef{}
	}
	return issue, nil
}

func (t *LocalTracker) writeTaskLocked(issue Issue) error {
	raw, path, err := t.marshalTaskLocked(issue)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(t.config.Tracker.LocalRoot, ".task-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func (t *LocalTracker) createTaskFileLocked(issue Issue) error {
	raw, path, err := t.marshalTaskLocked(issue)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(raw, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (t *LocalTracker) marshalTaskLocked(issue Issue) ([]byte, string, error) {
	if issue.ID == "" {
		return nil, "", errors.New("task id is required")
	}
	if issue.Identifier == "" {
		issue.Identifier = strings.ToUpper(issue.ID)
	}
	if issue.State == "" {
		issue.State = "Todo"
	}
	issue.AssignedToWorker = true
	issue.Labels = normalizeLabels(issue.Labels)
	if issue.UpdatedAt == nil {
		now := time.Now().UTC()
		issue.UpdatedAt = &now
	}
	raw, err := json.MarshalIndent(issue, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return raw, t.pathForID(issue.ID), nil
}

func (t *LocalTracker) pathForID(issueID string) string {
	return filepath.Join(t.config.Tracker.LocalRoot, SafeIdentifier(issueID)+".json")
}

func (t *LocalTracker) defaultLabels(labels []string) []string {
	normalized := normalizeLabels(labels)
	if len(normalized) > 0 {
		return normalized
	}
	if len(t.config.Tracker.RequiredLabels) > 0 {
		return append([]string(nil), t.config.Tracker.RequiredLabels...)
	}
	return []string{"local"}
}
