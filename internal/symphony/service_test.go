package symphony

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestServiceRunReportsHTTPBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	dir := t.TempDir()
	workflowPath := filepath.Join(dir, "WORKFLOW.md")
	content := `---
tracker:
  kind: local
  local_root: .symphony/tasks
  required_labels:
    - local
  active_states:
    - Todo
    - In Progress
  terminal_states:
    - Done
polling:
  interval_ms: 30000
workspace:
  root: .symphony/workspaces
hooks:
  timeout_ms: 1000
agent:
  max_concurrent_agents: 1
  max_turns: 1
  max_retry_backoff_ms: 300000
codex:
  command: codex app-server
  thread_sandbox: workspace-write
  turn_timeout_ms: 1000
  read_timeout_ms: 1000
  stall_timeout_ms: 300000
server:
  host: 127.0.0.1
  port: ` + strconv.Itoa(port) + `
---
Do {{ issue.identifier }}
`
	if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	logger, err := NewLogger(filepath.Join(dir, "logs"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	service := NewService(ServiceOptions{WorkflowPath: workflowPath, PortOverride: -1, Logger: logger})
	if err := service.Run(context.Background()); err == nil {
		t.Fatalf("expected service to report HTTP bind failure")
	}
}
