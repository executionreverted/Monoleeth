package events

import (
	"encoding/json"
	"reflect"
	"testing"

	"gameproject/internal/game/contracts"
	"gameproject/internal/game/foundation"
)

func TestEventEnvelopeIncludesTransportFieldsAndPayload(t *testing.T) {
	envelope := NewEventEnvelope(
		foundation.EventID("event-123"),
		"loot.created",
		json.RawMessage(`{"drop_id":"drop-9","amount":3}`),
		182736123,
		99122,
	)

	if envelope.EventID != foundation.EventID("event-123") {
		t.Fatalf("EventID = %q, want event-123", envelope.EventID)
	}
	if envelope.Type != "loot.created" {
		t.Fatalf("Type = %q, want loot.created", envelope.Type)
	}
	if string(envelope.Payload) != `{"drop_id":"drop-9","amount":3}` {
		t.Fatalf("Payload = %s, want drop payload", envelope.Payload)
	}
	if envelope.ServerTime != 182736123 {
		t.Fatalf("ServerTime = %d, want 182736123", envelope.ServerTime)
	}
	if envelope.Sequence != 99122 {
		t.Fatalf("Sequence = %d, want 99122", envelope.Sequence)
	}
	if envelope.Version != contracts.CurrentVersion {
		t.Fatalf("Version = %d, want %d", envelope.Version, contracts.CurrentVersion)
	}

	eventIDField, ok := reflect.TypeOf(EventEnvelope{}).FieldByName("EventID")
	if !ok {
		t.Fatal("EventEnvelope missing EventID field")
	}
	if eventIDField.Type != reflect.TypeOf(foundation.EventID("")) {
		t.Fatalf("EventID type = %v, want foundation.EventID", eventIDField.Type)
	}
}

func TestEventEnvelopeJSONShapeIsStable(t *testing.T) {
	envelope := NewEventEnvelope(
		foundation.EventID("event-456"),
		"player.snapshot",
		json.RawMessage(`{"ship_id":"ship-7","hp":85}`),
		182736124,
		99123,
	)

	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json marshal event envelope: %v", err)
	}

	want := `{"event_id":"event-456","type":"player.snapshot","payload":{"ship_id":"ship-7","hp":85},"server_time":182736124,"seq":99123,"v":1}`
	if got := string(payload); got != want {
		t.Fatalf("event envelope JSON = %s, want %s", got, want)
	}
}

func TestEventEnvelopeCopiesPayload(t *testing.T) {
	payload := json.RawMessage(`{"ship_id":"ship-7","hp":85}`)
	envelope := NewEventEnvelope(foundation.EventID("event-456"), "player.snapshot", payload, 182736124, 99123)

	payload[12] = 'X'

	if got := string(envelope.Payload); got != `{"ship_id":"ship-7","hp":85}` {
		t.Fatalf("event payload changed after source mutation: %s", got)
	}
}
