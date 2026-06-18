package observability

import (
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

func TestCommandLogEntryJSONContainsSafeFieldsOnly(t *testing.T) {
	entry := validCommandLogEntry()
	entry.ErrorCode = foundation.CodeOutOfRange
	entry.ReferenceID = foundation.IdempotencyKey("loot_pickup:drop-1")

	payload, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal command log entry: %v", err)
	}

	got := string(payload)
	for _, want := range []string{
		"request_id",
		"player_id",
		"session_id",
		"world_id",
		"zone_id",
		"operation",
		"error_code",
		"reference_id",
		"duration",
		"status",
		"timestamp",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("command log JSON %s missing safe field %q", got, want)
		}
	}

	for _, leaked := range []string{
		"message",
		"detail",
		"cause",
		"internal",
		"Target is out of range",
		"hidden planet",
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
