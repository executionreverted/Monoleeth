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

func TestCommandLogEntryValidateRejectsRequiredFields(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*CommandLogEntry)
		wantError error
	}{
		{
			name: "missing operation",
			mutate: func(entry *CommandLogEntry) {
				entry.Operation = ""
			},
			wantError: ErrBlankOperation,
		},
		{
			name: "missing request id",
			mutate: func(entry *CommandLogEntry) {
				entry.RequestID = ""
			},
			wantError: ErrMissingCommandLogIdentity,
		},
		{
			name: "missing session id",
			mutate: func(entry *CommandLogEntry) {
				entry.SessionID = ""
			},
			wantError: ErrMissingCommandLogIdentity,
		},
		{
			name: "missing player id",
			mutate: func(entry *CommandLogEntry) {
				entry.PlayerID = ""
			},
			wantError: ErrMissingCommandLogIdentity,
		},
		{
			name: "missing status",
			mutate: func(entry *CommandLogEntry) {
				entry.Status = ""
			},
			wantError: ErrBlankCommandStatus,
		},
		{
			name: "negative duration",
			mutate: func(entry *CommandLogEntry) {
				entry.Duration = -time.Millisecond
			},
			wantError: ErrInvalidDuration,
		},
		{
			name: "zero timestamp",
			mutate: func(entry *CommandLogEntry) {
				entry.Timestamp = time.Time{}
			},
			wantError: ErrMissingCommandLogTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := validCommandLogEntry()
			tt.mutate(&entry)

			err := entry.Validate()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("error = %v, want %v", err, tt.wantError)
			}
		})
	}
}

func TestCommandLogEntryJSONContainsOperationalFieldsAndNoSecrets(t *testing.T) {
	entry := validCommandLogEntry()
	entry.ErrorCode = foundation.CodeOutOfRange
	entry.ReferenceID = foundation.IdempotencyKey("loot_pickup:drop-1")

	payload, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal command log entry: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode command log JSON: %v", err)
	}
	for _, want := range []string{
		"request_id",
		"player_id",
		"session_id",
		"op",
		"result",
		"error_code",
		"idempotency_key",
		"ref_ids",
		"duration_ms",
		"timestamp",
	} {
		if _, ok := fields[want]; !ok {
			t.Fatalf("command log JSON %s missing safe field %q", payload, want)
		}
	}
	for _, legacy := range []string{"world_id", "zone_id", "operation", "reference_id", "duration", "status"} {
		if _, ok := fields[legacy]; ok {
			t.Fatalf("command log JSON %s included legacy field %q", payload, legacy)
		}
	}

	got := string(payload)
	for _, leaked := range []string{
		"message",
		"detail",
		"cause",
		"internal",
		"Target is out of range",
		"hidden planet",
		"password",
		"token",
		"cookie",
		"hash",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("command log JSON leaked %q in %s", leaked, got)
		}
	}
}

func TestMemoryCommandLoggerSnapshotSortedAndCloneSafe(t *testing.T) {
	logger := NewMemoryCommandLogger()
	first := validCommandLogEntry()
	first.RequestID = foundation.RequestID("request-2")
	first.Operation = Operation("market.buy")
	first.Timestamp = time.Date(2026, 6, 18, 12, 0, 2, 0, time.UTC)

	second := validCommandLogEntry()
	second.RequestID = foundation.RequestID("request-1")
	second.Operation = Operation("combat.use_skill")
	second.Timestamp = time.Date(2026, 6, 18, 12, 0, 1, 0, time.UTC)

	if err := logger.Record(first); err != nil {
		t.Fatalf("record first: %v", err)
	}
	if err := logger.Record(second); err != nil {
		t.Fatalf("record second: %v", err)
	}
	first.Operation = Operation("mutated.after.record")

	snapshot := logger.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("snapshot length = %d, want 2", len(snapshot))
	}
	if snapshot[0].RequestID != foundation.RequestID("request-1") {
		t.Fatalf("snapshot[0] request id = %q, want request-1", snapshot[0].RequestID)
	}
	if snapshot[1].Operation != Operation("market.buy") {
		t.Fatalf("stored entry mutated through caller value: got %q", snapshot[1].Operation)
	}

	snapshot[0].Operation = Operation("mutated.snapshot")
	next := logger.Snapshot()
	if next[0].Operation != Operation("combat.use_skill") {
		t.Fatalf("stored entry mutated through snapshot: got %q", next[0].Operation)
	}
}

func TestJSONCommandLoggerWritesSafeStructuredLine(t *testing.T) {
	var output bytes.Buffer
	logger, err := NewJSONCommandLogger(&output)
	if err != nil {
		t.Fatalf("NewJSONCommandLogger() error = %v", err)
	}
	entry := validCommandLogEntry()
	entry.ErrorCode = foundation.CodeNotVisible
	entry.ReferenceID = foundation.IdempotencyKey("loot_pickup:drop-1")

	if err := logger.Record(entry); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	got := output.String()
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("JSON log = %q, want trailing newline", got)
	}
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("JSON log = %q, want one line", got)
	}
	for _, want := range []string{
		`"request_id":"request-1"`,
		`"player_id":"player-1"`,
		`"session_id":"session-1"`,
		`"op":"combat.use_skill"`,
		`"error_code":"ERR_NOT_VISIBLE"`,
		`"idempotency_key":"loot_pickup:drop-1"`,
		`"ref_ids":["loot_pickup:drop-1"]`,
		`"duration_ms":15`,
		`"result":"ok"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("JSON log %s missing %s", got, want)
		}
	}
	for _, leaked := range []string{
		`"operation":`,
		`"reference_id":`,
		`"duration":`,
		`"status":`,
		"payload",
		"message",
		"detail",
		"hidden planet",
		"Target is out of range",
		"password",
		"token",
		"cookie",
		"hash",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("JSON log leaked %q in %s", leaked, got)
		}
	}
}

func TestJSONCommandLoggerRejectsNilWriter(t *testing.T) {
	if _, err := NewJSONCommandLogger(nil); !errors.Is(err, ErrMissingCommandLogWriter) {
		t.Fatalf("NewJSONCommandLogger(nil) error = %v, want ErrMissingCommandLogWriter", err)
	}
}

func validCommandLogEntry() CommandLogEntry {
	return CommandLogEntry{
		RequestID:   foundation.RequestID("request-1"),
		PlayerID:    foundation.PlayerID("player-1"),
		SessionID:   SessionID("session-1"),
		WorldID:     foundation.WorldID("world-1"),
		ZoneID:      foundation.ZoneID("zone-1"),
		Operation:   Operation("combat.use_skill"),
		Duration:    15 * time.Millisecond,
		Status:      CommandStatusOK,
		Timestamp:   time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		ReferenceID: "",
	}
}
