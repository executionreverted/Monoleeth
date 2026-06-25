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

func TestPremiumTransitionStructuredLogJSONContainsOperationalFieldsAndNoSecrets(t *testing.T) {
	entry := validPremiumTransitionLogEntry()
	entry.Status = CommandStatusError
	entry.ErrorCode = foundation.CodeInternal

	payload, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal premium transition log entry: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode premium transition log JSON: %v", err)
	}
	for _, want := range []string{
		"player_id",
		"request_id",
		"op",
		"result",
		"error_code",
		"idempotency_key",
		"ref_ids",
		"provider_ref_ids",
		"duration_ms",
		"timestamp",
	} {
		if _, ok := fields[want]; !ok {
			t.Fatalf("premium transition log JSON %s missing safe field %q", payload, want)
		}
	}
	for _, forbiddenField := range []string{
		"request_ref",
		"duration",
		"status",
		"payload",
	} {
		if _, ok := fields[forbiddenField]; ok {
			t.Fatalf("premium transition log JSON %s included forbidden field %q", payload, forbiddenField)
		}
	}
	for _, leaked := range []string{
		"password",
		"token",
		"cookie",
		"hash",
		"secret",
	} {
		if strings.Contains(string(payload), leaked) {
			t.Fatalf("premium transition log JSON leaked %q in %s", leaked, payload)
		}
	}
}

func TestPremiumTransitionStructuredLogDropsUnsafeProviderRefsNoSecrets(t *testing.T) {
	entry := validPremiumTransitionLogEntry()
	entry.RequestRef = "request-token-secret"
	entry.IdempotencyKey = foundation.IdempotencyKey("premium_webhook:stripe.event-token-secret-hash")
	entry.ReferenceIDs = []string{"entitlement.entitlement-1", "claim-token-secret"}
	entry.ProviderRefIDs = []string{"stripe.event-token-secret-hash"}

	payload, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal premium transition log entry: %v", err)
	}
	for _, leaked := range []string{"request-token-secret", "event-token-secret-hash", "claim-token-secret", "token", "secret", "hash"} {
		if strings.Contains(string(payload), leaked) {
			t.Fatalf("premium transition log JSON leaked %q in %s", leaked, payload)
		}
	}
}

func TestPremiumTransitionStructuredLogRequiresSafeIdentity(t *testing.T) {
	entry := validPremiumTransitionLogEntry()
	entry.PlayerID = ""

	if err := entry.Validate(); !errors.Is(err, ErrMissingPremiumTransitionLogIdentity) {
		t.Fatalf("Validate() error = %v, want ErrMissingPremiumTransitionLogIdentity", err)
	}
}

func TestJSONPremiumTransitionLoggerWritesSafeStructuredLine(t *testing.T) {
	var output bytes.Buffer
	logger, err := NewJSONPremiumTransitionLogger(&output)
	if err != nil {
		t.Fatalf("NewJSONPremiumTransitionLogger() error = %v", err)
	}

	if err := logger.RecordPremiumTransition(validPremiumTransitionLogEntry()); err != nil {
		t.Fatalf("RecordPremiumTransition() error = %v", err)
	}

	got := output.String()
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("JSON log = %q, want trailing newline", got)
	}
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("JSON log = %q, want one line", got)
	}
	for _, want := range []string{
		`"player_id":"player-1"`,
		`"request_id":"request-premium-1"`,
		`"op":"premium_claim"`,
		`"error_code":""`,
		`"idempotency_key":"premium_webhook:claim.entitlement-1.player-1"`,
		`"ref_ids":["premium_webhook:claim.entitlement-1.player-1","entitlement.entitlement-1"]`,
		`"provider_ref_ids":["stripe.event-1"]`,
		`"duration_ms":17`,
		`"result":"ok"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("JSON log %s missing %s", got, want)
		}
	}
	for _, leaked := range []string{"payload", "password", "token", "cookie", "hash", "secret"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("JSON log leaked %q in %s", leaked, got)
		}
	}
}

func TestJSONPremiumTransitionLoggerRejectsNilWriter(t *testing.T) {
	if _, err := NewJSONPremiumTransitionLogger(nil); !errors.Is(err, ErrMissingPremiumTransitionLogWriter) {
		t.Fatalf("NewJSONPremiumTransitionLogger(nil) error = %v, want ErrMissingPremiumTransitionLogWriter", err)
	}
}

func validPremiumTransitionLogEntry() PremiumTransitionLogEntry {
	return PremiumTransitionLogEntry{
		PlayerID:       foundation.PlayerID("player-1"),
		RequestRef:     "request-premium-1",
		Operation:      Operation("premium_claim"),
		IdempotencyKey: foundation.IdempotencyKey("premium_webhook:claim.entitlement-1.player-1"),
		ReferenceIDs:   []string{"entitlement.entitlement-1"},
		ProviderRefIDs: []string{"stripe.event-1"},
		Duration:       17 * time.Millisecond,
		Status:         CommandStatusOK,
		Timestamp:      time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	}
}
