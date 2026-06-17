package testutil

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

func TestEventRecorderCapturesEventsInOrder(t *testing.T) {
	recorder := NewEventRecorder()

	recorder.Record(newTestEvent("event-1", "loot.created", 10, `{"drop_id":"drop-1"}`))
	recorder.Record(newTestEvent("event-2", "quest.updated", 11, `{"quest_id":"quest-1"}`))
	recorder.Record(newTestEvent("event-3", "market.sale_completed", 12, `{"listing_id":"listing-1"}`))

	got := recorder.Events()
	AssertEventTypes(t, got, "loot.created", "quest.updated", "market.sale_completed")

	if got[0].EventID != foundation.EventID("event-1") {
		t.Fatalf("first EventID = %q, want event-1", got[0].EventID)
	}
	if got[1].Sequence != 11 {
		t.Fatalf("second Sequence = %d, want 11", got[1].Sequence)
	}
	if got[2].Type != "market.sale_completed" {
		t.Fatalf("third Type = %q, want market.sale_completed", got[2].Type)
	}
}

func TestEventRecorderReturnsSnapshotCopies(t *testing.T) {
	recorder := NewEventRecorder()
	emitted := newTestEvent("event-1", "loot.created", 1, `{"drop_id":"drop-1","amount":3}`)

	recorder.Record(emitted)
	emitted.Type = "mutated.type"
	emitted.Payload[12] = 'X'

	firstSnapshot := recorder.Events()
	if firstSnapshot[0].Type != "loot.created" {
		t.Fatalf("recorded Type after source mutation = %q, want loot.created", firstSnapshot[0].Type)
	}
	if got := string(firstSnapshot[0].Payload); got != `{"drop_id":"drop-1","amount":3}` {
		t.Fatalf("recorded Payload after source mutation = %s, want original payload", got)
	}

	firstSnapshot[0].Type = "snapshot.mutation"
	firstSnapshot[0].Payload[12] = 'Y'
	secondSnapshot := recorder.Events()
	if secondSnapshot[0].Type != "loot.created" {
		t.Fatalf("recorded Type after snapshot mutation = %q, want loot.created", secondSnapshot[0].Type)
	}
	if got := string(secondSnapshot[0].Payload); got != `{"drop_id":"drop-1","amount":3}` {
		t.Fatalf("recorded Payload after snapshot mutation = %s, want original payload", got)
	}
}

func TestCheckEventTypesReportsMissingAndWrongTypesClearly(t *testing.T) {
	got := []events.EventEnvelope{
		newTestEvent("event-1", "loot.created", 1, `{}`),
		newTestEvent("event-2", "market.sale_completed", 2, `{}`),
	}

	wrong := CheckEventTypes(got, "loot.created", "quest.updated")
	assertErrorContains(t, wrong,
		"event type mismatch at index 1",
		`got "market.sale_completed"`,
		`want "quest.updated"`,
		`got event types ["loot.created" "market.sale_completed"]`,
		`want ["loot.created" "quest.updated"]`,
	)

	missing := CheckEventTypes(got[:1], "loot.created", "quest.updated")
	assertErrorContains(t, missing,
		"missing event at index 1",
		`want "quest.updated"`,
		`got event types ["loot.created"]`,
		`want ["loot.created" "quest.updated"]`,
	)
}

func TestEventRecorderResetClearsEvents(t *testing.T) {
	recorder := NewEventRecorder()
	recorder.Record(newTestEvent("event-1", "loot.created", 1, `{}`))

	recorder.Reset()

	AssertRecordedEventTypes(t, recorder)
}

func newTestEvent(eventID foundation.EventID, eventType string, sequence uint64, payload string) events.EventEnvelope {
	return events.NewEventEnvelope(
		eventID,
		eventType,
		json.RawMessage(payload),
		182736123,
		sequence,
	)
}

func assertErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()

	if err == nil {
		t.Fatal("CheckEventTypes error = nil, want error")
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("CheckEventTypes error = %q, want it to contain %q", err.Error(), part)
		}
	}
}
