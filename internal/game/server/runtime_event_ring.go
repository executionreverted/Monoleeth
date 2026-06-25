package server

import "gameproject/internal/game/realtime"

const sessionEventRingCapacity = 128

type sessionEventRing struct {
	capacity int
	order    []uint64
	events   map[uint64]realtime.EventEnvelope
}

func newSessionEventRing(capacity int) *sessionEventRing {
	if capacity <= 0 {
		capacity = sessionEventRingCapacity
	}
	return &sessionEventRing{
		capacity: capacity,
		order:    make([]uint64, 0, capacity),
		events:   make(map[uint64]realtime.EventEnvelope, capacity),
	}
}

func (ring *sessionEventRing) append(event realtime.EventEnvelope) {
	if ring == nil || event.Sequence == 0 {
		return
	}
	if _, exists := ring.events[event.Sequence]; exists {
		ring.events[event.Sequence] = cloneEventEnvelope(event)
		return
	}
	if len(ring.order) >= ring.capacity {
		oldest := ring.order[0]
		delete(ring.events, oldest)
		copy(ring.order, ring.order[1:])
		ring.order = ring.order[:len(ring.order)-1]
	}
	ring.order = append(ring.order, event.Sequence)
	ring.events[event.Sequence] = cloneEventEnvelope(event)
}

func (ring *sessionEventRing) replayAfter(lastSeq uint64) ([]realtime.EventEnvelope, bool) {
	if ring == nil || len(ring.order) == 0 {
		return nil, true
	}
	oldest := ring.order[0]
	latest := ring.order[len(ring.order)-1]
	if lastSeq >= latest {
		return nil, true
	}
	if lastSeq+1 < oldest {
		return nil, false
	}
	events := make([]realtime.EventEnvelope, 0, len(ring.order))
	for _, seq := range ring.order {
		if seq <= lastSeq {
			continue
		}
		event, ok := ring.events[seq]
		if !ok {
			return nil, false
		}
		events = append(events, cloneEventEnvelope(event))
	}
	return events, true
}

func cloneEventEnvelope(event realtime.EventEnvelope) realtime.EventEnvelope {
	event.Payload = append([]byte(nil), event.Payload...)
	return event
}
