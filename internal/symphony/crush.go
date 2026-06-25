package symphony

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type AgentRunner interface {
	Run(ctx context.Context, workspace, prompt string, issue Issue, onEvent CodexEventHandler) (CodexRunResult, error)
}

type CrushClient struct {
	config Config
	logger *Logger
}

func NewCrushClient(config Config, logger *Logger) *CrushClient {
	return &CrushClient{config: config, logger: logger}
}

func (c *CrushClient) Run(ctx context.Context, workspace, prompt string, issue Issue, onEvent CodexEventHandler) (CodexRunResult, error) {
	ctx, cancel := context.WithTimeout(ctx, durationFromMS(c.timeoutMS()))
	defer cancel()
	args := c.crushArgs(workspace, issue)
	cmd := exec.CommandContext(ctx, c.command(), args...)
	cmd.Dir = workspace
	cmd.Env = crushCommandEnv(os.Environ(), c.endpointForIssue(issue))
	cmd.Stdin = strings.NewReader(prompt)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runID := crushRunID(issue, prompt)
	result := CodexRunResult{SessionID: "crush-" + runID, ThreadID: "crush", TurnID: runID}
	emit(onEvent, "session_started", map[string]any{"session_id": result.SessionID, "thread_id": result.ThreadID, "turn_id": result.TurnID, "agent_backend": agentBackendCrush, "agent_model": c.modelForIssue(issue)})
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		emit(onEvent, "turn_failed", map[string]any{"error": message, "agent_backend": agentBackendCrush, "agent_model": c.modelForIssue(issue)})
		return result, fmt.Errorf("crush run failed: %s", message)
	}
	output := strings.TrimSpace(stdout.String())
	emit(onEvent, "turn_completed", map[string]any{"message": output, "agent_backend": agentBackendCrush, "agent_model": c.modelForIssue(issue)})
	return result, nil
}

func (c *CrushClient) crushArgs(workspace string, issue Issue) []string {
	args := []string{"run", "--quiet", "--cwd", workspace}
	if model := c.modelForIssue(issue); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, c.config.Crush.ExtraArgs...)
	return args
}

func (c *CrushClient) modelForIssue(issue Issue) string {
	if model := strings.TrimSpace(issue.AgentModel); model != "" {
		return model
	}
	return strings.TrimSpace(c.config.Crush.Model)
}

func (c *CrushClient) endpointForIssue(issue Issue) string {
	if endpoint := strings.TrimSpace(issue.AgentEndpoint); endpoint != "" {
		return endpoint
	}
	return strings.TrimSpace(c.config.Crush.Endpoint)
}

func (c *CrushClient) command() string {
	if cmd := strings.TrimSpace(c.config.Crush.Command); cmd != "" {
		return cmd
	}
	return "crush"
}

func (c *CrushClient) timeoutMS() int {
	if c.config.Crush.TimeoutMS > 0 {
		return c.config.Crush.TimeoutMS
	}
	if c.config.Codex.TurnTimeoutMS > 0 {
		return c.config.Codex.TurnTimeoutMS
	}
	return 3600000
}

func crushCommandEnv(base []string, endpoint string) []string {
	env := append([]string(nil), base...)
	env = setEnvValue(env, "PATH", extendedCodexPath(envValue(env, "PATH")))
	endpoint = strings.TrimSpace(endpoint)
	if endpoint != "" {
		env = setEnvValue(env, "CRUSH_BASE_URL", endpoint)
		env = setEnvValue(env, "OPENAI_BASE_URL", endpoint)
		env = setEnvValue(env, "SYMPHONY_CRUSH_ENDPOINT", endpoint)
	}
	return env
}

func crushRunID(issue Issue, prompt string) string {
	h := sha1.Sum([]byte(issue.ID + "\x00" + issue.Identifier + "\x00" + prompt))
	return hex.EncodeToString(h[:8])
}
