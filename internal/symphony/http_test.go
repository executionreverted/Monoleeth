package symphony

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestHTTPDashboardTasksAndPresenterEndpoints(t *testing.T) {
	orchestrator, server := newHTTPTestServer(t)
	defer server.Close()

	root := getBody(t, server.URL+"/")
	for _, want := range []string{"Symphony Control", "Rate limits", "Running sessions", "Blocked sessions", "Retry queue"} {
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
	for _, want := range []string{"Symphony Tasks", "Create & run", "Task board", "Pause auto-run", "Run stream", "Patch review", "Agent stream", "Follow latest", "agentIsland", "taskBackend", "taskModel", "taskEndpoint"} {
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

	payload := []byte(`{"title":"Build local task flow","description":"from test","agent_backend":"crush","agent_model":"zai/glm-5.2","agent_endpoint":"https://api.z.ai/api/coding/paas/v4"}`)
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
	if created.AgentBackend != "crush" || created.AgentModel != "zai/glm-5.2" || created.AgentEndpoint != "https://api.z.ai/api/coding/paas/v4" {
		t.Fatalf("expected agent override fields to persist, got %#v", created)
	}
	missingWorkspaceResp, err := http.Get(server.URL + "/api/v1/tasks/" + created.ID + "/workspace-diff")
	if err != nil {
		t.Fatal(err)
	}
	defer missingWorkspaceResp.Body.Close()
	if missingWorkspaceResp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(missingWorkspaceResp.Body)
		t.Fatalf("missing workspace diff status = %d body = %s", missingWorkspaceResp.StatusCode, string(body))
	}

	workspace := filepath.Join(orchestrator.workflow.Config.Workspace.Root, SafeIdentifier(created.Identifier))
	setupWorkspaceDiffRepo(t, workspace)
	var diffPacket WorkspaceDiffPacket
	decodeGET(t, server.URL+"/api/v1/tasks/"+created.ID+"/workspace-diff", &diffPacket)
	if diffPacket.ID != created.ID || diffPacket.Identifier != created.Identifier || diffPacket.State != created.State {
		t.Fatalf("unexpected workspace diff identity: %#v", diffPacket)
	}
	if diffPacket.WorkspacePath != workspace {
		t.Fatalf("workspace path = %q, want %q", diffPacket.WorkspacePath, workspace)
	}
	if !strings.Contains(diffPacket.GitStatus, "M tracked.txt") {
		t.Fatalf("workspace diff git status missing tracked file: %q", diffPacket.GitStatus)
	}
	if !strings.Contains(diffPacket.DiffStat, "tracked.txt") {
		t.Fatalf("workspace diff stat missing tracked file: %q", diffPacket.DiffStat)
	}
	if !strings.Contains(diffPacket.TrackedDiff, "-before") || !strings.Contains(diffPacket.TrackedDiff, "+after") {
		t.Fatalf("workspace tracked diff missing expected content:\n%s", diffPacket.TrackedDiff)
	}
	if len(diffPacket.UntrackedFiles) != 1 || diffPacket.UntrackedFiles[0] != "new.txt" {
		t.Fatalf("workspace untracked files = %#v, want new.txt", diffPacket.UntrackedFiles)
	}
	if !strings.Contains(diffPacket.Patch, "diff --git") || !strings.Contains(diffPacket.Patch, "new.txt") {
		t.Fatalf("workspace patch missing tracked or untracked content:\n%s", diffPacket.Patch)
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

	var waitTimeout TaskWaitResult
	decodeGET(t, server.URL+"/api/v1/tasks/"+created.ID+"/wait?timeout_ms=1", &waitTimeout)
	if !waitTimeout.TimedOut || waitTimeout.Settled || waitTimeout.State != "Todo" {
		t.Fatalf("unexpected timeout wait payload: %#v", waitTimeout)
	}

	if err := orchestrator.UpdateTaskState(context.Background(), created.ID, "Done"); err != nil {
		t.Fatal(err)
	}
	var waitDone TaskWaitResult
	decodeGET(t, server.URL+"/api/v1/tasks/"+created.ID+"/wait?timeout_ms=1000", &waitDone)
	if !waitDone.Settled || waitDone.TimedOut || waitDone.State != "Done" {
		t.Fatalf("unexpected done wait payload: %#v", waitDone)
	}
	if len(waitDone.DisplayEvents) != 1 || waitDone.DisplayEvents[0].Title != "Test event" {
		t.Fatalf("unexpected done wait display events: %#v", waitDone.DisplayEvents)
	}

	missingWaitResp, err := http.Get(server.URL + "/api/v1/tasks/missing/wait?timeout_ms=1")
	if err != nil {
		t.Fatal(err)
	}
	defer missingWaitResp.Body.Close()
	if missingWaitResp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(missingWaitResp.Body)
		t.Fatalf("missing wait status = %d body = %s", missingWaitResp.StatusCode, string(body))
	}
	missingDiffResp, err := http.Get(server.URL + "/api/v1/tasks/missing/workspace-diff")
	if err != nil {
		t.Fatal(err)
	}
	defer missingDiffResp.Body.Close()
	if missingDiffResp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(missingDiffResp.Body)
		t.Fatalf("missing workspace diff status = %d body = %s", missingDiffResp.StatusCode, string(body))
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

func setupWorkspaceDiffRepo(t *testing.T, workspace string) {
	t.Helper()
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, workspace, "init")
	runTestGit(t, workspace, "config", "user.email", "symphony@example.test")
	runTestGit(t, workspace, "config", "user.name", "Symphony Test")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, workspace, "add", "tracked.txt")
	runTestGit(t, workspace, "commit", "-m", "seed")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runTestGit(t *testing.T, workspace string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v output=%s", strings.Join(args, " "), err, string(output))
	}
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
