package symphony

import "time"

type Issue struct {
	ID               string       `json:"id"`
	Identifier       string       `json:"identifier"`
	Title            string       `json:"title"`
	Description      string       `json:"description,omitempty"`
	Priority         *int         `json:"priority,omitempty"`
	State            string       `json:"state"`
	BranchName       string       `json:"branch_name,omitempty"`
	URL              string       `json:"url,omitempty"`
	Labels           []string     `json:"labels"`
	BlockedBy        []BlockerRef `json:"blocked_by"`
	AssigneeID       string       `json:"assignee_id,omitempty"`
	AssignedToWorker bool         `json:"assigned_to_worker"`
	CreatedAt        *time.Time   `json:"created_at,omitempty"`
	UpdatedAt        *time.Time   `json:"updated_at,omitempty"`
}

type BlockerRef struct {
	ID         string `json:"id,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	State      string `json:"state,omitempty"`
}

type WorkflowDefinition struct {
	Path           string
	Dir            string
	Config         Config
	PromptTemplate string
	ModTime        time.Time
}

type RuntimeEvent struct {
	Event     string         `json:"event"`
	Timestamp time.Time      `json:"timestamp"`
	Details   map[string]any `json:"details,omitempty"`
}

type RunningEntry struct {
	IssueID            string         `json:"issue_id"`
	Identifier         string         `json:"identifier"`
	Issue              Issue          `json:"issue"`
	Attempt            int            `json:"attempt"`
	WorkspacePath      string         `json:"workspace_path,omitempty"`
	SessionID          string         `json:"session_id,omitempty"`
	ThreadID           string         `json:"thread_id,omitempty"`
	TurnID             string         `json:"turn_id,omitempty"`
	CodexAppServerPID  int            `json:"codex_app_server_pid,omitempty"`
	StartedAt          time.Time      `json:"started_at"`
	LastCodexEvent     string         `json:"last_codex_event,omitempty"`
	LastCodexTimestamp *time.Time     `json:"last_codex_timestamp,omitempty"`
	LastCodexMessage   map[string]any `json:"last_codex_message,omitempty"`
	Cancel             func()         `json:"-"`
}

type BlockedEntry struct {
	IssueID       string     `json:"issue_id"`
	Identifier    string     `json:"identifier"`
	Issue         Issue      `json:"issue"`
	WorkspacePath string     `json:"workspace_path,omitempty"`
	SessionID     string     `json:"session_id,omitempty"`
	Error         string     `json:"error"`
	BlockedAt     time.Time  `json:"blocked_at"`
	LastEvent     string     `json:"last_codex_event,omitempty"`
	LastTimestamp *time.Time `json:"last_codex_timestamp,omitempty"`
}

type RetryEntry struct {
	IssueID    string    `json:"issue_id"`
	Identifier string    `json:"identifier"`
	Attempt    int       `json:"attempt"`
	DueAt      time.Time `json:"due_at"`
	Error      string    `json:"error,omitempty"`
}

type Snapshot struct {
	WorkflowPath        string                  `json:"workflow_path"`
	PollIntervalMS      int                     `json:"poll_interval_ms"`
	NextPollDueAt       *time.Time              `json:"next_poll_due_at,omitempty"`
	PollCheckInProgress bool                    `json:"poll_check_in_progress"`
	Running             map[string]RunningEntry `json:"running"`
	Blocked             map[string]BlockedEntry `json:"blocked"`
	Retries             map[string]RetryEntry   `json:"retries"`
	Completed           []string                `json:"completed"`
	Claimed             []string                `json:"claimed"`
	LastRefreshError    string                  `json:"last_refresh_error,omitempty"`
}
