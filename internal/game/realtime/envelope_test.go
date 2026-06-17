package realtime

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestDecodeRequestEnvelopeRejectsMissingRequestID(t *testing.T) {
	_, err := DecodeRequestEnvelope([]byte(`{"op":"move_to","payload":{"x":10,"y":20},"client_seq":7,"v":1}`))

	requireInvalidPayload(t, err)
}

func TestDecodeRequestEnvelopeRejectsMissingOp(t *testing.T) {
	_, err := DecodeRequestEnvelope([]byte(`{"request_id":"request-1","payload":{"x":10,"y":20},"client_seq":7,"v":1}`))

	requireInvalidPayload(t, err)
}

func TestDecodeRequestEnvelopeRejectsInvalidOrMissingPayload(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing payload",
			body: `{"request_id":"request-1","op":"move_to","client_seq":7,"v":1}`,
		},
		{
			name: "null payload",
			body: `{"request_id":"request-1","op":"move_to","payload":null,"client_seq":7,"v":1}`,
		},
		{
			name: "array payload",
			body: `{"request_id":"request-1","op":"move_to","payload":[],"client_seq":7,"v":1}`,
		},
		{
			name: "string payload",
			body: `{"request_id":"request-1","op":"move_to","payload":"bad","client_seq":7,"v":1}`,
		},
		{
			name: "malformed envelope",
			body: `{"request_id":"request-1","op":"move_to","payload":`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeRequestEnvelope([]byte(tt.body))
			requireInvalidPayload(t, err)
		})
	}
}

func TestRequestEnvelopeValidateRejectsInvalidRawPayload(t *testing.T) {
	envelope := NewRequestEnvelope(
		foundation.RequestID("request-1"),
		OperationMoveTo,
		json.RawMessage(`{`),
		7,
	)

	err := envelope.Validate()

	requireInvalidPayload(t, err)
}

func TestDecodeRequestEnvelopeAcceptsRegisteredPhase04Operation(t *testing.T) {
	envelope, err := DecodeRequestEnvelope([]byte(`{"request_id":"request-1","op":"move_to","payload":{"x":10,"y":20},"client_seq":7,"v":1}`))
	if err != nil {
		t.Fatalf("decode valid request envelope: %v", err)
	}

	if envelope.RequestID != foundation.RequestID("request-1") {
		t.Fatalf("request id = %q, want request-1", envelope.RequestID)
	}
	if envelope.Op != OperationMoveTo {
		t.Fatalf("op = %q, want %q", envelope.Op, OperationMoveTo)
	}
	if got := string(envelope.Payload); got != `{"x":10,"y":20}` {
		t.Fatalf("payload = %s, want move payload", got)
	}
}

func TestEventEnvelopeMarshalsWithoutHiddenInternalFields(t *testing.T) {
	envelope := NewEventEnvelope(
		foundation.EventID("event-1"),
		EventAOIEntityEntered,
		json.RawMessage(`{"entity_id":"entity-1","kind":"npc","x":10,"y":20}`),
		182736123,
		99122,
	)

	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json marshal event envelope: %v", err)
	}

	got := string(payload)
	want := `{"event_id":"event-1","type":"aoi.entity_entered","payload":{"entity_id":"entity-1","kind":"npc","x":10,"y":20},"server_time":182736123,"seq":99122,"v":1}`
	if got != want {
		t.Fatalf("event envelope JSON = %s, want %s", got, want)
	}

	for _, leaked := range []string{"internal", "hidden", "seed", "unfiltered"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("event envelope leaked %q in %s", leaked, got)
		}
	}
}

func requireInvalidPayload(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected invalid payload error, got nil")
	}
	if !foundation.IsCode(err, foundation.CodeInvalidPayload) {
		t.Fatalf("error code mismatch: got %v, want %s", err, foundation.CodeInvalidPayload)
	}
}
