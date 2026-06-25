package server

import (
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

func assertGatewayReplayMismatchForTest(t *testing.T, response realtime.CachedResponse, label string) {
	t.Helper()
	if !response.HasError {
		t.Fatalf("%s response = %+v, want replay mismatch error", label, response)
	}
	if response.Error.Error.Code != foundation.CodeRequestReplayMismatch {
		t.Fatalf("%s error code = %s, want %s", label, response.Error.Error.Code, foundation.CodeRequestReplayMismatch)
	}
	if response.Error.Error.Retryable {
		t.Fatalf("%s replay mismatch error retryable = true, want false", label)
	}
}
