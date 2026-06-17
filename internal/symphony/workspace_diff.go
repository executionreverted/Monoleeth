package symphony

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ErrWorkspaceNotFound = errors.New("workspace not found")

func (o *Orchestrator) WorkspaceDiff(ctx context.Context, issueID string) (WorkspaceDiffPacket, error) {
	snapshot := o.Snapshot()
	var task Issue
	found := false
	for _, candidate := range snapshot.Tasks {
		if candidate.ID == issueID {
			task = candidate
			found = true
			break
		}
	}
	if !found {
		return WorkspaceDiffPacket{}, ErrTaskNotFound
	}
	workspace := filepath.Join(snapshot.WorkspaceRoot, SafeIdentifier(task.Identifier))
	if _, err := os.Stat(workspace); err != nil {
		if os.IsNotExist(err) {
			return WorkspaceDiffPacket{}, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, workspace)
		}
		return WorkspaceDiffPacket{}, err
	}
	if err := validateWorkspacePath(snapshot.WorkspaceRoot, workspace); err != nil {
		return WorkspaceDiffPacket{}, err
	}

	status, err := gitOutput(ctx, workspace, "status", "--short")
	if err != nil {
		return WorkspaceDiffPacket{}, err
	}
	stat, err := gitOutput(ctx, workspace, "diff", "--stat")
	if err != nil {
		return WorkspaceDiffPacket{}, err
	}
	trackedDiff, err := gitOutput(ctx, workspace, "diff", "--binary")
	if err != nil {
		return WorkspaceDiffPacket{}, err
	}
	untrackedRaw, err := gitOutput(ctx, workspace, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return WorkspaceDiffPacket{}, err
	}
	untracked := splitGitLines(untrackedRaw)
	untrackedPatch, err := untrackedFilesPatch(ctx, workspace, untracked)
	if err != nil {
		return WorkspaceDiffPacket{}, err
	}
	patch := strings.TrimRight(trackedDiff+"\n"+untrackedPatch, "\n")

	return WorkspaceDiffPacket{
		ID:             task.ID,
		Identifier:     task.Identifier,
		State:          task.State,
		WorkspacePath:  workspace,
		GitStatus:      status,
		DiffStat:       stat,
		TrackedDiff:    trackedDiff,
		UntrackedFiles: untracked,
		Patch:          patch,
	}, nil
}

func gitOutput(ctx context.Context, workspace string, args ...string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "git", args...)
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if commandCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("git %s timed out", strings.Join(args, " "))
	}
	if err != nil {
		return "", fmt.Errorf("git %s: %w output=%q", strings.Join(args, " "), err, truncateForLog(string(output), 2048))
	}
	return string(output), nil
}

func untrackedFilesPatch(ctx context.Context, workspace string, files []string) (string, error) {
	var out bytes.Buffer
	for _, file := range files {
		if strings.TrimSpace(file) == "" {
			continue
		}
		if filepath.IsAbs(file) || strings.Contains(file, "..") {
			return "", fmt.Errorf("unsafe untracked path: %s", file)
		}
		commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		cmd := exec.CommandContext(commandCtx, "git", "diff", "--no-index", "--binary", "--", "/dev/null", file)
		cmd.Dir = workspace
		output, err := cmd.CombinedOutput()
		cancel()
		if commandCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("git diff untracked %s timed out", file)
		}
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				return "", fmt.Errorf("git diff untracked %s: %w output=%q", file, err, truncateForLog(string(output), 2048))
			}
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.Write(output)
	}
	return out.String(), nil
}

func splitGitLines(raw string) []string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
