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

func TestSettlementStructuredLogJSONContainsOperationalFieldsAndNoSecrets(t *testing.T) {
	entry := validSettlementLogEntry()
	entry.ErrorCode = foundation.CodeInternal
	entry.Status = CommandStatusError

	payload, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal settlement log entry: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode settlement log JSON: %v", err)
	}
	for _, want := range []string{
		"request_id",
		"player_id",
		"op",
		"result",
		"error_code",
		"idempotency_key",
		"ref_ids",
		"duration_ms",
		"timestamp",
	} {
		if _, ok := fields[want]; !ok {
			t.Fatalf("settlement log JSON %s missing safe field %q", payload, want)
		}
	}
	for _, legacy := range []string{"reference_id", "duration", "status", "payload"} {
		if _, ok := fields[legacy]; ok {
			t.Fatalf("settlement log JSON %s included forbidden field %q", payload, legacy)
		}
	}
	for _, forbidden := range []string{
		"password",
		"token",
		"cookie",
		"hash",
		"payload",
		"request_body",
		"ledger_payload",
	} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("settlement log JSON leaked %q in %s", forbidden, payload)
		}
	}
}

func TestSettlementStructuredLogRequiresRequestPlayerAndIdempotency(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*SettlementLogEntry)
		wantError error
	}{
		{
			name: "missing request",
			mutate: func(entry *SettlementLogEntry) {
				entry.RequestID = ""
			},
			wantError: ErrMissingSettlementLogIdentity,
		},
		{
			name: "missing player",
			mutate: func(entry *SettlementLogEntry) {
				entry.PlayerID = ""
			},
			wantError: ErrMissingSettlementLogIdentity,
		},
		{
			name: "missing idempotency",
			mutate: func(entry *SettlementLogEntry) {
				entry.IdempotencyKey = ""
			},
			wantError: ErrMissingSettlementLogIdentity,
		},
		{
			name: "negative duration",
			mutate: func(entry *SettlementLogEntry) {
				entry.Duration = -time.Millisecond
			},
			wantError: ErrInvalidDuration,
		},
		{
			name: "missing timestamp",
			mutate: func(entry *SettlementLogEntry) {
				entry.Timestamp = time.Time{}
			},
			wantError: ErrMissingSettlementLogTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := validSettlementLogEntry()
			tt.mutate(&entry)

			if err := entry.Validate(); !errors.Is(err, tt.wantError) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantError)
			}
		})
	}
}

func TestJSONSettlementLoggerWritesSafeStructuredLine(t *testing.T) {
	var output bytes.Buffer
	logger, err := NewJSONSettlementLogger(&output)
	if err != nil {
		t.Fatalf("NewJSONSettlementLogger() error = %v", err)
	}

	if err := logger.RecordSettlement(validSettlementLogEntry()); err != nil {
		t.Fatalf("RecordSettlement() error = %v", err)
	}

	got := output.String()
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("JSON log = %q, want trailing newline", got)
	}
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("JSON log = %q, want one line", got)
	}
	for _, want := range []string{
		`"request_id":"request-settlement-1"`,
		`"player_id":"player-1"`,
		`"op":"market.buy"`,
		`"error_code":""`,
		`"idempotency_key":"market_buy:listing-1:player-1:request-settlement-1"`,
		`"ref_ids":["market_buy:listing-1:player-1:request-settlement-1","market_sale:listing-1:player-1:request-settlement-1"]`,
		`"duration_ms":13`,
		`"result":"ok"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("JSON log %s missing %s", got, want)
		}
	}
	for _, leaked := range []string{"payload", "password", "token", "cookie", "hash"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("JSON log leaked %q in %s", leaked, got)
		}
	}
}

func TestJSONSettlementLoggerRejectsNilWriter(t *testing.T) {
	if _, err := NewJSONSettlementLogger(nil); !errors.Is(err, ErrMissingSettlementLogWriter) {
		t.Fatalf("NewJSONSettlementLogger(nil) error = %v, want ErrMissingSettlementLogWriter", err)
	}
}

func validSettlementLogEntry() SettlementLogEntry {
	return SettlementLogEntry{
		RequestID:      foundation.RequestID("request-settlement-1"),
		PlayerID:       foundation.PlayerID("player-1"),
		Operation:      Operation("market.buy"),
		Duration:       13 * time.Millisecond,
		Status:         CommandStatusOK,
		Timestamp:      time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		IdempotencyKey: foundation.IdempotencyKey("market_buy:listing-1:player-1:request-settlement-1"),
		ReferenceIDs: []foundation.IdempotencyKey{
			foundation.IdempotencyKey("market_sale:listing-1:player-1:request-settlement-1"),
		},
	}
}
