package symphony

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceCreateRunsAfterCreateHook(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces")
	cfg := defaultConfig()
	cfg.Workspace.Root = root
	cfg.Hooks.AfterCreate = `printf '%s' "$SYMPHONY_ISSUE_IDENTIFIER" > issue.txt`
	logger, err := NewLogger(t.TempDir(), os.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	manager := NewWorkspaceManager(cfg, logger)
	workspace, err := manager.CreateForIssue(context.Background(), Issue{ID: "1", Identifier: "GP/1"})
	if err != nil {
		t.Fatal(err)
	}
	if !workspace.CreatedNow {
		t.Fatalf("expected new workspace")
	}
	raw, err := os.ReadFile(filepath.Join(workspace.Path, "issue.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "GP/1" {
		t.Fatalf("hook env issue identifier = %q", raw)
	}
	if filepath.Base(workspace.Path) != "GP_1" {
		t.Fatalf("workspace was not sanitized: %s", workspace.Path)
	}
}

func TestValidateWorkspacePathRejectsRoot(t *testing.T) {
	root := t.TempDir()
	if err := validateWorkspacePath(root, root); err == nil {
		t.Fatalf("expected root workspace rejection")
	}
}
