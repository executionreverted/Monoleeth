package server

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
)

func assertStructuredCommandLogFieldsForTest(
	t *testing.T,
	label string,
	entry observability.CommandLogEntry,
	want map[string]string,
	referenceID foundation.IdempotencyKey,
	forbiddenMarkers []string,
) {
	t.Helper()
	rawLog, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal %s command log: %v", label, err)
	}
	var public map[string]any
	if err := json.Unmarshal(rawLog, &public); err != nil {
		t.Fatalf("decode %s command log: %v", label, err)
	}
	for field, value := range want {
		if public[field] != value {
			t.Fatalf("%s command log field %s = %#v, want %q in %s", label, field, public[field], value, rawLog)
		}
	}
	if public["duration_ms"].(float64) < 0 {
		t.Fatalf("%s command log duration_ms = %#v, want non-negative in %s", label, public["duration_ms"], rawLog)
	}
	refIDs, ok := public["ref_ids"].([]any)
	if !ok || len(refIDs) != 1 || refIDs[0] != referenceID.String() {
		t.Fatalf("%s command log ref_ids = %#v, want [%q] in %s", label, public["ref_ids"], referenceID, rawLog)
	}
	assertCommandLogNoSecretMarkersForTest(t, label, rawLog)
	for _, marker := range forbiddenMarkers {
		if strings.Contains(string(rawLog), marker) {
			t.Fatalf("%s command log leaked %q in %s", label, marker, rawLog)
		}
	}
}

func assertCommandLogNoSecretMarkersForTest(t *testing.T, label string, rawLog []byte) {
	t.Helper()
	for _, leaked := range []string{
		"account_id",
		"actor_account_id",
		"password",
		"password_hash",
		"session_token",
		"token",
		"cookie",
		"hash",
		"payload",
		"published_by",
		"rolled_back_from",
		"snapshot_json",
		"gameplay_seed",
		"procedural_seed",
		"transfer_token",
		"internal_map_id",
	} {
		if strings.Contains(string(rawLog), leaked) {
			t.Fatalf("%s command log leaked %q in %s", label, leaked, rawLog)
		}
	}
}
