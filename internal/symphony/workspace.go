package symphony

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type WorkspaceManager struct {
	config Config
	logger *Logger
}

func NewWorkspaceManager(config Config, logger *Logger) *WorkspaceManager {
	return &WorkspaceManager{config: config, logger: logger}
}

type WorkspaceResult struct {
	Path       string
	CreatedNow bool
}

func (m *WorkspaceManager) CreateForIssue(ctx context.Context, issue Issue) (WorkspaceResult, error) {
	safeID := SafeIdentifier(issue.Identifier)
	workspace := filepath.Join(m.config.Workspace.Root, safeID)
	if err := validateWorkspacePath(m.config.Workspace.Root, workspace); err != nil {
		return WorkspaceResult{}, err
	}
	created := false
	if info, err := os.Stat(workspace); err == nil && info.IsDir() {
		created = false
	} else {
		if err := os.RemoveAll(workspace); err != nil {
			return WorkspaceResult{}, err
		}
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			return WorkspaceResult{}, err
		}
		created = true
	}
	if created && strings.TrimSpace(m.config.Hooks.AfterCreate) != "" {
		if err := m.runHook(ctx, "after_create", m.config.Hooks.AfterCreate, workspace, issue); err != nil {
			return WorkspaceResult{}, err
		}
	}
	return WorkspaceResult{Path: workspace, CreatedNow: created}, nil
}

func (m *WorkspaceManager) RunBeforeRun(ctx context.Context, workspace string, issue Issue) error {
	if strings.TrimSpace(m.config.Hooks.BeforeRun) == "" {
		return nil
	}
	return m.runHook(ctx, "before_run", m.config.Hooks.BeforeRun, workspace, issue)
}

func (m *WorkspaceManager) RunAfterRun(ctx context.Context, workspace string, issue Issue) {
	if strings.TrimSpace(m.config.Hooks.AfterRun) == "" {
		return
	}
	if err := m.runHook(ctx, "after_run", m.config.Hooks.AfterRun, workspace, issue); err != nil {
		m.logger.Warn("workspace hook failed and was ignored", "hook", "after_run", "issue_id", issue.ID, "error", err)
	}
}

func (m *WorkspaceManager) RemoveIssueWorkspace(ctx context.Context, identifier string) {
	workspace := filepath.Join(m.config.Workspace.Root, SafeIdentifier(identifier))
	_ = m.Remove(ctx, workspace)
}

func (m *WorkspaceManager) Remove(ctx context.Context, workspace string) error {
	if _, err := os.Stat(workspace); err != nil {
		return os.RemoveAll(workspace)
	}
	if err := validateWorkspacePath(m.config.Workspace.Root, workspace); err != nil {
		return err
	}
	if strings.TrimSpace(m.config.Hooks.BeforeRemove) != "" {
		issue := Issue{Identifier: filepath.Base(workspace)}
		if err := m.runHook(ctx, "before_remove", m.config.Hooks.BeforeRemove, workspace, issue); err != nil {
			m.logger.Warn("workspace hook failed and was ignored", "hook", "before_remove", "workspace", workspace, "error", err)
		}
	}
	return os.RemoveAll(workspace)
}

func (m *WorkspaceManager) runHook(ctx context.Context, hookName, script, workspace string, issue Issue) error {
	timeout := durationFromMS(m.config.Hooks.TimeoutMS)
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	m.logger.Info("running workspace hook", "hook", hookName, "issue_id", issue.ID, "issue_identifier", issue.Identifier, "workspace", workspace)
	cmd := exec.CommandContext(hookCtx, "sh", "-lc", script)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(),
		"SYMPHONY_HOOK="+hookName,
		"SYMPHONY_WORKSPACE="+workspace,
		"SYMPHONY_ISSUE_ID="+issue.ID,
		"SYMPHONY_ISSUE_IDENTIFIER="+issue.Identifier,
	)
	output, err := cmd.CombinedOutput()
	if hookCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("workspace hook timeout hook=%s timeout=%s", hookName, timeout)
	}
	if err != nil {
		return fmt.Errorf("workspace hook failed hook=%s error=%w output=%q", hookName, err, truncateForLog(string(output), 2048))
	}
	return nil
}

var unsafeIdentifierChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func SafeIdentifier(identifier string) string {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		identifier = "issue"
	}
	return unsafeIdentifierChars.ReplaceAllString(identifier, "_")
}

func validateWorkspacePath(root, workspace string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(rootAbs, 0o755); err != nil {
		return err
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return err
	}
	parent := filepath.Dir(workspaceAbs)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	parentReal, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	if workspaceAbs == rootAbs {
		return errors.New("workspace must not equal workspace root")
	}
	if parentReal != rootReal {
		return fmt.Errorf("workspace outside root: %s not under %s", workspaceAbs, rootAbs)
	}
	return nil
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
