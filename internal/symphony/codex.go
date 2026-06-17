package symphony

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type CodexClient struct {
	config Config
	logger *Logger
}

func NewCodexClient(config Config, logger *Logger) *CodexClient {
	return &CodexClient{config: config, logger: logger}
}

type CodexRunResult struct {
	SessionID string
	ThreadID  string
	TurnID    string
	PID       int
}

type CodexEventHandler func(RuntimeEvent)

func (c *CodexClient) Run(ctx context.Context, workspace, prompt string, issue Issue, onEvent CodexEventHandler) (CodexRunResult, error) {
	session, err := c.StartSession(ctx, workspace)
	if err != nil {
		return CodexRunResult{}, err
	}
	defer session.Stop()
	return session.RunTurn(ctx, prompt, issue, onEvent)
}

type CodexSession struct {
	config   Config
	logger   *Logger
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	lines    chan []byte
	waitErr  chan error
	writeMu  sync.Mutex
	threadID string
	pid      int
	nextID   int
}

func (c *CodexClient) StartSession(ctx context.Context, workspace string) (*CodexSession, error) {
	cmd := exec.CommandContext(ctx, "bash", "-lc", c.config.Codex.Command)
	cmd.Dir = workspace
	cmd.Env = codexCommandEnv(os.Environ())
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	session := &CodexSession{
		config:  c.config,
		logger:  c.logger,
		cmd:     cmd,
		stdin:   stdin,
		lines:   make(chan []byte, 256),
		waitErr: make(chan error, 1),
		pid:     cmd.Process.Pid,
		nextID:  10,
	}
	go session.scan(stdout)
	go session.scan(stderr)
	go func() {
		session.waitErr <- cmd.Wait()
		close(session.waitErr)
	}()
	if err := session.initialize(ctx); err != nil {
		session.Stop()
		return nil, err
	}
	threadID, err := session.startThread(ctx, workspace)
	if err != nil {
		session.Stop()
		return nil, err
	}
	session.threadID = threadID
	return session, nil
}

func codexCommandEnv(base []string) []string {
	env := append([]string(nil), base...)
	env = setEnvValue(env, "PATH", extendedCodexPath(envValue(env, "PATH")))
	if envValue(env, "CODEX_BIN") == "" {
		for _, candidate := range []string{
			"/Applications/Codex.app/Contents/Resources/codex",
			"/opt/homebrew/bin/codex",
			"/usr/local/bin/codex",
		} {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				env = setEnvValue(env, "CODEX_BIN", candidate)
				break
			}
		}
	}
	return env
}

func extendedCodexPath(current string) string {
	parts := strings.Split(current, ":")
	out := make([]string, 0, len(parts)+6)
	seen := map[string]bool{}
	for _, part := range append(parts,
		"/Applications/Codex.app/Contents/Resources",
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
		"/usr/sbin",
		"/sbin",
	) {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return strings.Join(out, ":")
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func setEnvValue(env []string, key string, value string) []string {
	prefix := key + "="
	next := prefix + value
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = next
			return env
		}
	}
	return append(env, next)
}

func (s *CodexSession) Stop() {
	_ = s.stdin.Close()
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

func (s *CodexSession) initialize(ctx context.Context) error {
	id := s.nextRequestID()
	if err := s.send(map[string]any{
		"method": "initialize",
		"id":     id,
		"params": map[string]any{
			"capabilities": map[string]any{"experimentalApi": true},
			"clientInfo": map[string]any{
				"name":    "symphony-go-orchestrator",
				"title":   "Symphony Go Orchestrator",
				"version": "0.1.0",
			},
		},
	}); err != nil {
		return err
	}
	if _, err := s.awaitResponse(ctx, id, durationFromMS(s.config.Codex.ReadTimeoutMS)); err != nil {
		return err
	}
	return s.send(map[string]any{"method": "initialized", "params": map[string]any{}})
}

func (s *CodexSession) startThread(ctx context.Context, workspace string) (string, error) {
	id := s.nextRequestID()
	if err := s.send(map[string]any{
		"method": "thread/start",
		"id":     id,
		"params": map[string]any{
			"approvalPolicy": s.config.Codex.ApprovalPolicy,
			"sandbox":        s.config.Codex.ThreadSandbox,
			"cwd":            workspace,
			"dynamicTools":   s.dynamicToolSpecs(),
		},
	}); err != nil {
		return "", err
	}
	payload, err := s.awaitResponse(ctx, id, durationFromMS(s.config.Codex.ReadTimeoutMS))
	if err != nil {
		return "", err
	}
	result, _ := getMap(payload, "result")
	thread, ok := getMap(result, "thread")
	if !ok {
		thread, _ = getMap(payload, "thread")
	}
	threadID, _ := thread["id"].(string)
	if threadID == "" {
		return "", fmt.Errorf("invalid thread payload: %v", payload)
	}
	return threadID, nil
}

func (s *CodexSession) RunTurn(ctx context.Context, prompt string, issue Issue, onEvent CodexEventHandler) (CodexRunResult, error) {
	id := s.nextRequestID()
	title := strings.TrimSpace(issue.Identifier + ": " + issue.Title)
	if err := s.send(map[string]any{
		"method": "turn/start",
		"id":     id,
		"params": map[string]any{
			"threadId": s.threadID,
			"input": []map[string]any{
				{"type": "text", "text": prompt},
			},
			"cwd":            s.cmd.Dir,
			"title":          title,
			"approvalPolicy": s.config.Codex.ApprovalPolicy,
			"sandboxPolicy":  s.config.TurnSandboxPolicy(s.cmd.Dir),
		},
	}); err != nil {
		return CodexRunResult{}, err
	}
	payload, err := s.awaitResponse(ctx, id, durationFromMS(s.config.Codex.ReadTimeoutMS))
	if err != nil {
		return CodexRunResult{}, err
	}
	result, _ := getMap(payload, "result")
	turn, ok := getMap(result, "turn")
	if !ok {
		turn, _ = getMap(payload, "turn")
	}
	turnID, _ := turn["id"].(string)
	if turnID == "" {
		return CodexRunResult{}, fmt.Errorf("invalid turn payload: %v", payload)
	}
	sessionID := s.threadID + "-" + turnID
	emit(onEvent, "session_started", map[string]any{"session_id": sessionID, "thread_id": s.threadID, "turn_id": turnID, "codex_app_server_pid": s.pid})
	if err := s.awaitTurnCompletion(ctx, onEvent); err != nil {
		return CodexRunResult{SessionID: sessionID, ThreadID: s.threadID, TurnID: turnID, PID: s.pid}, err
	}
	return CodexRunResult{SessionID: sessionID, ThreadID: s.threadID, TurnID: turnID, PID: s.pid}, nil
}

func (s *CodexSession) awaitTurnCompletion(ctx context.Context, onEvent CodexEventHandler) error {
	timeout := durationFromMS(s.config.Codex.TurnTimeoutMS)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-s.waitErr:
			if err == nil {
				return errors.New("codex app-server exited")
			}
			return err
		case <-timer.C:
			return errors.New("turn timeout")
		case line := <-s.lines:
			if len(line) == 0 {
				continue
			}
			handled, err := s.handleTurnLine(ctx, line, onEvent)
			if err != nil {
				return err
			}
			if handled {
				return nil
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeout)
		}
	}
}

func (s *CodexSession) handleTurnLine(ctx context.Context, line []byte, onEvent CodexEventHandler) (bool, error) {
	var payload map[string]any
	if err := json.Unmarshal(line, &payload); err != nil {
		raw := string(line)
		if looksLikeProtocol(raw) {
			emit(onEvent, "malformed", map[string]any{"raw": raw})
		}
		return false, nil
	}
	method, _ := payload["method"].(string)
	switch method {
	case "turn/completed":
		emit(onEvent, "turn_completed", map[string]any{"payload": payload})
		return true, nil
	case "turn/failed":
		emit(onEvent, "turn_failed", map[string]any{"payload": payload})
		return false, fmt.Errorf("turn failed: %v", payload["params"])
	case "turn/cancelled":
		emit(onEvent, "turn_cancelled", map[string]any{"payload": payload})
		return false, fmt.Errorf("turn cancelled: %v", payload["params"])
	case "item/commandExecution/requestApproval", "execCommandApproval", "applyPatchApproval", "item/fileChange/requestApproval":
		if s.config.Codex.ApprovalPolicy == "never" {
			decision := "acceptForSession"
			if method == "execCommandApproval" || method == "applyPatchApproval" {
				decision = "approved_for_session"
			}
			_ = s.send(map[string]any{"id": payload["id"], "result": map[string]any{"decision": decision}})
			emit(onEvent, "approval_auto_approved", map[string]any{"method": method, "decision": decision})
			return false, nil
		}
		emit(onEvent, "approval_required", map[string]any{"payload": payload})
		return false, fmt.Errorf("approval required: %s", method)
	case "item/tool/call":
		result := s.handleToolCall(ctx, payload)
		_ = s.send(map[string]any{"id": payload["id"], "result": result})
		emit(onEvent, "tool_call_completed", map[string]any{"payload": payload, "result": result})
		return false, nil
	case "item/tool/requestUserInput", "mcpServer/elicitation/request":
		emit(onEvent, "turn_input_required", map[string]any{"payload": payload})
		return false, fmt.Errorf("operator input required: %s", method)
	default:
		if method != "" {
			emit(onEvent, "notification", map[string]any{"payload": payload})
		}
		return false, nil
	}
}

func (s *CodexSession) awaitResponse(ctx context.Context, id int, timeout time.Duration) (map[string]any, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-s.waitErr:
			if err == nil {
				err = errors.New("codex app-server exited")
			}
			return nil, err
		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for response id=%d", id)
		case line := <-s.lines:
			var payload map[string]any
			if err := json.Unmarshal(line, &payload); err != nil {
				continue
			}
			if intID(payload["id"]) != id {
				continue
			}
			if errPayload, ok := payload["error"]; ok {
				return nil, fmt.Errorf("codex app-server error: %v", errPayload)
			}
			return payload, nil
		}
	}
}

func (s *CodexSession) send(payload map[string]any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.stdin.Write(append(encoded, '\n'))
	return err
}

func (s *CodexSession) scan(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		select {
		case s.lines <- line:
		default:
			s.logger.Warn("dropping codex app-server line because buffer is full")
		}
	}
}

func (s *CodexSession) nextRequestID() int {
	s.nextID++
	return s.nextID
}

func (s *CodexSession) dynamicToolSpecs() []map[string]any {
	if s.config.Tracker.Kind != "linear" || s.config.Tracker.APIKey == "" {
		return nil
	}
	return []map[string]any{
		{
			"name":        "linear_graphql",
			"description": "Execute a raw GraphQL query or mutation against Linear using Symphony's configured auth.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":         map[string]any{"type": "string"},
					"variables":     map[string]any{"type": "object"},
					"operationName": map[string]any{"type": "string"},
				},
				"required": []string{"query"},
			},
		},
	}
}

func (s *CodexSession) handleToolCall(ctx context.Context, payload map[string]any) map[string]any {
	params, _ := getMap(payload, "params")
	name := toolCallName(params)
	args := toolCallArguments(params)
	if name != "linear_graphql" {
		return toolResult(false, "unsupported tool: "+name)
	}
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return toolResult(false, "linear_graphql requires query")
	}
	variables, _ := args["variables"].(map[string]any)
	if variables == nil {
		variables = map[string]any{}
	}
	operationName, _ := args["operationName"].(string)
	client := NewLinearClient(s.config, s.logger)
	body, err := client.GraphQL(ctx, query, variables, operationName)
	if err != nil {
		return toolResult(false, err.Error())
	}
	encoded, _ := json.MarshalIndent(body, "", "  ")
	return toolResult(true, string(encoded))
}

func toolCallName(params map[string]any) string {
	for _, key := range []string{"name", "toolName", "tool_name"} {
		if value, _ := params[key].(string); value != "" {
			return value
		}
	}
	tool, _ := getMap(params, "tool")
	if value, _ := tool["name"].(string); value != "" {
		return value
	}
	return ""
}

func toolCallArguments(params map[string]any) map[string]any {
	for _, key := range []string{"arguments", "args", "input"} {
		if value, ok := params[key].(map[string]any); ok {
			return value
		}
		if raw, ok := params[key].(string); ok && raw != "" {
			var decoded map[string]any
			if json.Unmarshal([]byte(raw), &decoded) == nil {
				return decoded
			}
		}
	}
	return map[string]any{}
}

func toolResult(success bool, output string) map[string]any {
	return map[string]any{
		"success": success,
		"output":  output,
		"contentItems": []map[string]any{
			{"type": "inputText", "text": output},
		},
	}
}

func emit(handler CodexEventHandler, event string, details map[string]any) {
	if handler == nil {
		return
	}
	handler(RuntimeEvent{Event: event, Timestamp: nowUTC(), Details: details})
}

func looksLikeProtocol(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.HasPrefix(raw, "{") || strings.Contains(raw, `"method"`) || strings.Contains(raw, `"id"`)
}

func intID(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}
