package symphony

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWorkflowAppliesDefaultsAndEnv(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := `---
tracker:
  kind: linear
  project_slug: "game-project"
  required_labels:
    - Symphony
workspace:
  root: .symphony/workspaces
agent:
  max_concurrent_agents_by_state:
    "In Progress": 1
---
Hello {{ issue.identifier }}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	workflow, err := LoadWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}
	if workflow.Config.Tracker.APIKey != "lin_test" {
		t.Fatalf("expected env Linear token, got %q", workflow.Config.Tracker.APIKey)
	}
	if got := workflow.Config.Workspace.Root; got != filepath.Join(dir, ".symphony/workspaces") {
		t.Fatalf("workspace root = %q", got)
	}
	if !strings.Contains(workflow.PromptTemplate, "{{ issue.identifier }}") {
		t.Fatalf("prompt body was not preserved: %q", workflow.PromptTemplate)
	}
	if workflow.Config.Agent.MaxConcurrentAgentsByState["in progress"] != 1 {
		t.Fatalf("state concurrency was not normalized: %#v", workflow.Config.Agent.MaxConcurrentAgentsByState)
	}
	if workflow.Config.Codex.Command != "codex app-server" {
		t.Fatalf("codex default command = %q", workflow.Config.Codex.Command)
	}
	if workflow.Config.Codex.ApprovalPolicy != "never" {
		t.Fatalf("codex approval policy = %#v", workflow.Config.Codex.ApprovalPolicy)
	}
}

func TestLoadWorkflowNormalizesLegacyApprovalPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := `---
tracker:
  kind: local
codex:
  approval_policy:
    reject:
      sandbox_approval: true
---
Hello`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	workflow, err := LoadWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}
	if workflow.Config.Codex.ApprovalPolicy != "never" {
		t.Fatalf("legacy approval policy was not normalized: %#v", workflow.Config.Codex.ApprovalPolicy)
	}
}

func TestBuildPromptLiquidSubset(t *testing.T) {
	issue := Issue{
		ID:          "abc",
		Identifier:  "GP-1",
		Title:       "Build it",
		Description: "Do the thing",
		State:       "Todo",
		Labels:      []string{"symphony"},
	}
	template := `Ticket {{ issue.identifier }}
{% if attempt %}Attempt {{ attempt }}{% else %}First run{% endif %}
{% if issue.description %}Body: {{ issue.description }}{% else %}No body{% endif %}
Labels: {{ issue.labels }}`
	got, err := BuildPrompt(template, issue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "First run") || !strings.Contains(got, "Body: Do the thing") || !strings.Contains(got, "Labels: symphony") {
		t.Fatalf("unexpected prompt:\n%s", got)
	}
	got, err = BuildPrompt(template, issue, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Attempt 2") {
		t.Fatalf("expected retry attempt in prompt:\n%s", got)
	}
}
