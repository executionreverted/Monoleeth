package symphony

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrushClientRunsConfiguredModel(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake-crush.sh")
	script := `#!/bin/sh
printf 'args:%s\n' "$*" > "$CRUSH_FAKE_LOG"
cat > "$CRUSH_FAKE_PROMPT"
printf 'done from crush\n'
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "args.txt")
	promptPath := filepath.Join(dir, "prompt.txt")
	t.Setenv("CRUSH_FAKE_LOG", logPath)
	t.Setenv("CRUSH_FAKE_PROMPT", promptPath)
	logger := newDiscardLogger(t)
	defer logger.Close()
	cfg := defaultConfig()
	cfg.Crush.Command = fake
	cfg.Crush.Model = "zai/glm-5.2"
	cfg.Crush.TimeoutMS = 1000
	client := NewCrushClient(cfg, logger)
	var events []string
	result, err := client.Run(context.Background(), dir, "hello crush", Issue{ID: "local-1", Identifier: "TASK-1", Title: "Use GLM"}, func(event RuntimeEvent) {
		events = append(events, event.Event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID == "" || result.ThreadID == "" || result.TurnID == "" {
		t.Fatalf("expected stable crush run identifiers, got %#v", result)
	}
	if strings.Join(events, ",") != "session_started,turn_completed" {
		t.Fatalf("unexpected events: %#v", events)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"run", "--quiet", "--cwd " + dir, "--model zai/glm-5.2"} {
		if !strings.Contains(string(args), want) {
			t.Fatalf("crush args missing %q: %s", want, string(args))
		}
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(prompt) != "hello crush" {
		t.Fatalf("prompt = %q", string(prompt))
	}
}

func TestCrushClientAllowsIssueModelOverride(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake-crush.sh")
	script := `#!/bin/sh
printf '%s\n' "$*" > "$CRUSH_FAKE_LOG"
printf 'ok\n'
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "args.txt")
	t.Setenv("CRUSH_FAKE_LOG", logPath)
	logger := newDiscardLogger(t)
	defer logger.Close()
	cfg := defaultConfig()
	cfg.Crush.Command = fake
	cfg.Crush.Model = "zai/glm-5.2"
	cfg.Crush.TimeoutMS = 1000
	client := NewCrushClient(cfg, logger)
	_, err := client.Run(context.Background(), dir, "prompt", Issue{ID: "local-1", Identifier: "TASK-1", AgentModel: "openrouter/sakana/fugu-ultra", AgentEndpoint: "https://example.test/v1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--model openrouter/sakana/fugu-ultra") {
		t.Fatalf("issue model did not override default: %s", string(args))
	}
}

func TestCrushCommandEnvUsesIssueEndpointOverride(t *testing.T) {
	env := crushCommandEnv([]string{"PATH=/usr/bin"}, "https://example.test/v1")
	if got := envValue(env, "CRUSH_BASE_URL"); got != "https://example.test/v1" {
		t.Fatalf("CRUSH_BASE_URL = %q", got)
	}
	if got := envValue(env, "OPENAI_BASE_URL"); got != "https://example.test/v1" {
		t.Fatalf("OPENAI_BASE_URL = %q", got)
	}
	if got := envValue(env, "SYMPHONY_CRUSH_ENDPOINT"); got != "https://example.test/v1" {
		t.Fatalf("SYMPHONY_CRUSH_ENDPOINT = %q", got)
	}
}
