package symphony

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexClientRunsFakeAppServer(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake-codex.sh")
	script := `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*) printf '{"id":11,"result":{}}\n' ;;
    *'"method":"initialized"'*) ;;
    *'"method":"thread/start"'*) printf '{"id":12,"result":{"thread":{"id":"thr_test"}}}\n' ;;
    *'"method":"turn/start"'*) printf '{"id":13,"result":{"turn":{"id":"turn_test"}}}\n'; printf '{"method":"turn/completed","params":{"status":"ok"}}\n';;
  esac
done
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	logger, err := NewLogger(t.TempDir(), os.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	cfg := defaultConfig()
	cfg.Codex.Command = fake
	cfg.Codex.ReadTimeoutMS = 1000
	cfg.Codex.TurnTimeoutMS = 1000
	client := NewCodexClient(cfg, logger)
	var events []string
	result, err := client.Run(context.Background(), dir, "hello", Issue{Identifier: "GP-1", Title: "Test"}, func(event RuntimeEvent) {
		events = append(events, event.Event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID != "thr_test" || result.TurnID != "turn_test" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if strings.Join(events, ",") != "session_started,turn_completed" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestCodexCommandEnvExtendsLaunchdPath(t *testing.T) {
	env := codexCommandEnv([]string{"PATH=/usr/bin:/bin", "CODEX_BIN=/custom/codex"})
	path := envValue(env, "PATH")
	for _, want := range []string{"/Applications/Codex.app/Contents/Resources", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"} {
		if !strings.Contains(path, want) {
			t.Fatalf("PATH missing %q: %s", want, path)
		}
	}
	if got := envValue(env, "CODEX_BIN"); got != "/custom/codex" {
		t.Fatalf("CODEX_BIN override should be preserved, got %q", got)
	}
}
