package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestAuthTransitionStructuredLogJSONContainsRequestResultDurationAndNoSecrets(t *testing.T) {
	entry := validAuthTransitionLogEntry()
	entry.Request.Operation = Operation("auth.login")
	entry.ErrorCode = foundation.CodeUnauthenticated
	entry.Status = CommandStatusError
	entry.PlayerID = ""
	entry.SessionID = ""

	payload, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal auth transition log entry: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode auth transition log JSON: %v", err)
	}
	for _, want := range []string{
		"request",
		"op",
		"result",
		"error_code",
		"duration_ms",
		"timestamp",
	} {
		if _, ok := fields[want]; !ok {
			t.Fatalf("auth transition log JSON %s missing safe field %q", payload, want)
		}
	}
	for _, forbidden := range []string{
		"password",
		"password_hash",
		"raw_token_secret",
		"session_token",
		"cookie",
		"hash",
		"payload",
		"email@example.com",
	} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("auth transition log JSON leaked %q in %s", forbidden, payload)
		}
	}
}

func TestAuthTransitionStructuredLogSuccessRequiresSafeSessionIdentity(t *testing.T) {
	entry := validAuthTransitionLogEntry()
	entry.PlayerID = ""

	if err := entry.Validate(); !errors.Is(err, ErrMissingAuthTransitionLogIdentity) {
		t.Fatalf("Validate() error = %v, want ErrMissingAuthTransitionLogIdentity", err)
	}
}

func TestJSONAuthTransitionLoggerWritesSafeStructuredLine(t *testing.T) {
	var output bytes.Buffer
	logger, err := NewJSONAuthTransitionLogger(&output)
	if err != nil {
		t.Fatalf("NewJSONAuthTransitionLogger() error = %v", err)
	}
	entry := validAuthTransitionLogEntry()

	if err := logger.RecordAuthTransition(entry); err != nil {
		t.Fatalf("RecordAuthTransition() error = %v", err)
	}

	got := output.String()
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("JSON log = %q, want trailing newline", got)
	}
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("JSON log = %q, want one line", got)
	}
	for _, want := range []string{
		`"request":{"op":"auth.register"}`,
		`"player_id":"player-1"`,
		`"session_id":"session-1"`,
		`"op":"auth.register"`,
		`"error_code":""`,
		`"duration_ms":9`,
		`"result":"ok"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("JSON log %s missing %s", got, want)
		}
	}
	for _, leaked := range []string{
		"password",
		"password_hash",
		"token",
		"cookie",
		"hash",
		"payload",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("JSON log leaked %q in %s", leaked, got)
		}
	}
}

func TestJSONAuthTransitionLoggerRejectsNilWriter(t *testing.T) {
	if _, err := NewJSONAuthTransitionLogger(nil); !errors.Is(err, ErrMissingAuthTransitionLogWriter) {
		t.Fatalf("NewJSONAuthTransitionLogger(nil) error = %v, want ErrMissingAuthTransitionLogWriter", err)
	}
}

func validAuthTransitionLogEntry() AuthTransitionLogEntry {
	return AuthTransitionLogEntry{
		Request: AuthTransitionRequest{
			Operation: Operation("auth.register"),
		},
		PlayerID:  foundation.PlayerID("player-1"),
		SessionID: SessionID("session-1"),
		Duration:  9 * time.Millisecond,
		Status:    CommandStatusOK,
		Timestamp: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	}
}
