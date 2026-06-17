package symphony

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestHTTPDashboardTasksAndPresenterEndpoints(t *testing.T) {
	orchestrator, server := newHTTPTestServer(t)
	defer server.Close()

	root := getBody(t, server.URL+"/")
	for _, want := range []string{"Symphony Observability", "Rate limits", "Running sessions", "Blocked sessions", "Retry queue"} {
		if !strings.Contains(root, want) {
			t.Fatalf("dashboard missing %q:\n%s", want, root)
		}
	}
	if strings.Contains(root, "Create & run") || strings.Contains(root, "Task board") {
		t.Fatalf("observability dashboard should not contain local task UI:\n%s", root)
	}
	if !strings.Contains(root, `href="/dashboard.css"`) {
		t.Fatalf("dashboard should load the embedded dashboard stylesheet:\n%s", root)
	}

	tasksPage := getBody(t, server.URL+"/tasks")
	for _, want := range []string{"Symphony Tasks", "Create & run", "Task board", "Pause auto-run", "Run stream", "Agent stream", "Follow latest", "agentIsland"} {
		if !strings.Contains(tasksPage, want) {
			t.Fatalf("tasks page missing %q:\n%s", want, tasksPage)
		}
	}

	cssResp, err := http.Get(server.URL + "/dashboard.css")
	if err != nil {
		t.Fatal(err)
	}
	defer cssResp.Body.Close()
	cssBody, err := io.ReadAll(cssResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if cssResp.Header.Get("Content-Type") != "text/css; charset=utf-8" {
		t.Fatalf("unexpected css content type: %q", cssResp.Header.Get("Content-Type"))
	}
	if !strings.Contains(string(cssBody), ".hero-card") || !strings.Contains(string(cssBody), ".data-table-running") {
		t.Fatalf("embedded css missing upstream dashboard selectors")
	}

	payload := []byte(`{"title":"Build local task flow","description":"from test"}`)
	resp, err := http.Post(server.URL+"/api/v1/tasks", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create task status = %d", resp.StatusCode)
	}
	var created Issue
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Identifier != "TASK-0001" || created.State != "Todo" {
		t.Fatalf("unexpected created task: %#v", created)
	}
	if len(created.Labels) != 1 || created.Labels[0] != "local" {
		t.Fatalf("expected required label fallback, got %#v", created.Labels)
	}
	orchestrator.recordRunEvent(created, 0, 0, "test_event", map[string]any{"summary": "hello"}, "hello")

	var stream struct {
		Events        []RunLogEntry     `json:"events"`
		DisplayEvents []DisplayRunEvent `json:"display_events"`
	}
	decodeGET(t, server.URL+"/api/v1/tasks/"+created.ID+"/stream", &stream)
	if len(stream.Events) != 1 || stream.Events[0].Event != "test_event" {
		t.Fatalf("unexpected stream payload: %#v", stream.Events)
	}
	if len(stream.DisplayEvents) != 1 || stream.DisplayEvents[0].Title != "Test event" {
		t.Fatalf("unexpected display stream payload: %#v", stream.DisplayEvents)
	}

	var listed struct {
		Tasks []Issue `json:"tasks"`
	}
	decodeGET(t, server.URL+"/api/v1/tasks", &listed)
	if listed.Tasks == nil || len(listed.Tasks) != 1 || listed.Tasks[0].ID != created.ID {
		t.Fatalf("unexpected task listing: %#v", listed.Tasks)
	}

	orchestrator.mu.Lock()
	orchestrator.running[created.ID] = RunningEntry{
		IssueID:        created.ID,
		Identifier:     created.Identifier,
		Issue:          Issue{ID: created.ID, Identifier: created.Identifier, State: "In Progress"},
		WorkspacePath:  "/tmp/workspace",
		SessionID:      "session-1",
		TurnCount:      2,
		StartedAt:      nowUTC(),
		LastCodexEvent: "turn_completed",
	}
	orchestrator.retries["retry-1"] = RetryEntry{IssueID: "retry-1", Identifier: "TASK-0002", Attempt: 2, DueAt: nowUTC(), Error: "boom"}
	orchestrator.blocked["blocked-1"] = BlockedEntry{IssueID: "blocked-1", Identifier: "TASK-0003", Issue: Issue{ID: "blocked-1", Identifier: "TASK-0003", State: "Blocked"}, Error: "needs input", BlockedAt: nowUTC()}
	orchestrator.mu.Unlock()

	var state map[string]any
	decodeGET(t, server.URL+"/api/v1/state", &state)
	for _, key := range []string{"generated_at", "counts", "running", "retrying", "blocked", "codex_totals", "rate_limits"} {
		if _, ok := state[key]; !ok {
			t.Fatalf("state payload missing %q: %#v", key, state)
		}
	}
	counts := state["counts"].(map[string]any)
	if counts["running"].(float64) != 1 || counts["retrying"].(float64) != 1 || counts["blocked"].(float64) != 1 {
		t.Fatalf("unexpected counts: %#v", counts)
	}

	var issue map[string]any
	decodeGET(t, server.URL+"/api/v1/"+created.Identifier, &issue)
	if issue["status"] != "running" || issue["issue_identifier"] != created.Identifier {
		t.Fatalf("unexpected issue payload: %#v", issue)
	}

	req, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/autorun", strings.NewReader(`{"auto_run":false}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("autorun patch status = %d body = %s", resp.StatusCode, string(body))
	}
	if !orchestrator.AutoRunPaused() {
		t.Fatalf("expected autorun patch to pause auto-run")
	}

	raw := getBody(t, server.URL+"/api/v1/raw-state")
	if !strings.Contains(raw, "tracker_kind") {
		t.Fatalf("raw state did not include raw snapshot fields:\n%s", raw)
	}
}

func TestHTTPStartReportsBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	logger := newDiscardLogger(t)
	defer logger.Close()
	server := NewHTTPServer("127.0.0.1", port, NewOrchestrator(testWorkflow(t), logger), logger)
	if err := server.Start(); err == nil {
		t.Fatalf("expected bind failure on occupied port %s", strconv.Itoa(port))
	}
}

func newHTTPTestServer(t *testing.T) (*Orchestrator, *httptest.Server) {
	t.Helper()
	logger := newDiscardLogger(t)
	t.Cleanup(func() { logger.Close() })
	orchestrator := NewOrchestrator(testWorkflow(t), logger)
	httpServer := NewHTTPServer("127.0.0.1", 0, orchestrator, logger)
	server := httptest.NewServer(httpServer.server.Handler)
	return orchestrator, server
}

func testWorkflow(t *testing.T) WorkflowDefinition {
	t.Helper()
	dir := t.TempDir()
	return WorkflowDefinition{
		Path:           filepath.Join(dir, "WORKFLOW.md"),
		Dir:            dir,
		PromptTemplate: "Do {{ issue.identifier }}",
		Config: Config{
			Tracker: TrackerConfig{
				Kind:           "local",
				LocalRoot:      filepath.Join(dir, "tasks"),
				RequiredLabels: []string{"local"},
				ActiveStates:   []string{"Todo", "In Progress"},
				TerminalStates: []string{"Done", "Closed", "Cancelled", "Canceled", "Duplicate"},
			},
			Polling:   PollingConfig{IntervalMS: 30000},
			Workspace: WorkspaceConfig{Root: filepath.Join(dir, "workspaces")},
			Hooks:     HooksConfig{TimeoutMS: 1000},
			Agent:     AgentConfig{MaxConcurrentAgents: 2, MaxTurns: 1, MaxRetryBackoffMS: 300000, MaxConcurrentAgentsByState: map[string]int{}},
			Codex:     CodexConfig{Command: "codex app-server", ThreadSandbox: "workspace-write", TurnTimeoutMS: 1000, ReadTimeoutMS: 1000, StallTimeoutMS: 300000},
			Server:    ServerConfig{Host: "127.0.0.1"},
		},
	}
}

func newDiscardLogger(t *testing.T) *Logger {
	t.Helper()
	logger, err := NewLogger(filepath.Join(t.TempDir(), "logs"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return logger
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d body = %s", url, resp.StatusCode, string(body))
	}
	return string(body)
}

func decodeGET(t *testing.T, url string, out any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s status = %d body = %s", url, resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}
