package testutil

import (
	"fmt"
	"sync"
	"testing"

	"gameproject/internal/game/events"
)

// EventRecorder records emitted gameplay events for service tests.
type EventRecorder struct {
	mu     sync.Mutex
	events []events.EventEnvelope
}

// NewEventRecorder creates an empty event recorder.
func NewEventRecorder() *EventRecorder {
	return &EventRecorder{}
}

// Record stores a snapshot of event.
func (recorder *EventRecorder) Record(event events.EventEnvelope) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	recorder.events = append(recorder.events, cloneEventEnvelope(event))
}

// Events returns a snapshot of recorded events in emission order.
func (recorder *EventRecorder) Events() []events.EventEnvelope {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	return cloneEventEnvelopes(recorder.events)
}

// Reset removes recorded events.
func (recorder *EventRecorder) Reset() {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	recorder.events = nil
}

// AssertRecordedEventTypes fails the test if recorder has not captured exactly
// the expected event types in order.
func AssertRecordedEventTypes(t testing.TB, recorder *EventRecorder, want ...string) {
	t.Helper()

	if recorder == nil {
		t.Fatal("event recorder is nil")
	}
	AssertEventTypes(t, recorder.Events(), want...)
}

// AssertEventTypes fails the test if got does not contain exactly the expected
// event types in order.
func AssertEventTypes(t testing.TB, got []events.EventEnvelope, want ...string) {
	t.Helper()

	if err := CheckEventTypes(got, want...); err != nil {
		t.Fatal(err)
	}
}

// CheckEventTypes reports whether got contains exactly the expected event types
// in order.
func CheckEventTypes(got []events.EventEnvelope, want ...string) error {
	gotTypes := make([]string, len(got))
	for i, event := range got {
		gotTypes[i] = event.Type
	}

	shared := len(gotTypes)
	if len(want) < shared {
		shared = len(want)
	}
	for i := 0; i < shared; i++ {
		if gotTypes[i] != want[i] {
			return fmt.Errorf("event type mismatch at index %d: got %q, want %q (got event types %q; want %q)", i, gotTypes[i], want[i], gotTypes, want)
		}
	}

	if len(gotTypes) < len(want) {
		index := len(gotTypes)
		return fmt.Errorf("missing event at index %d: want %q (got event types %q; want %q)", index, want[index], gotTypes, want)
	}
	if len(gotTypes) > len(want) {
		index := len(want)
		return fmt.Errorf("unexpected event at index %d: got %q (got event types %q; want %q)", index, gotTypes[index], gotTypes, want)
	}

	return nil
}

func cloneEventEnvelopes(source []events.EventEnvelope) []events.EventEnvelope {
	if source == nil {
		return nil
	}

	clones := make([]events.EventEnvelope, len(source))
	for i, event := range source {
		clones[i] = cloneEventEnvelope(event)
	}
	return clones
}

func cloneEventEnvelope(event events.EventEnvelope) events.EventEnvelope {
	if event.Payload == nil {
		return event
	}

	payload := make([]byte, len(event.Payload))
	copy(payload, event.Payload)
	event.Payload = payload
	return event
}
