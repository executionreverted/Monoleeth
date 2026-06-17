package events

import (
	"encoding/json"

	"gameproject/internal/game/contracts"
	"gameproject/internal/game/foundation"
)

// EventEnvelope is the server-to-client realtime/domain event transport envelope.
type EventEnvelope struct {
	EventID    foundation.EventID `json:"event_id"`
	Type       string             `json:"type"`
	Payload    json.RawMessage    `json:"payload"`
	ServerTime int64              `json:"server_time"`
	Sequence   uint64             `json:"seq"`
	Version    int                `json:"v"`
}

// NewEventEnvelope returns an event envelope using the current protocol version.
func NewEventEnvelope(eventID foundation.EventID, eventType string, payload json.RawMessage, serverTime int64, sequence uint64) EventEnvelope {
	return EventEnvelope{
		EventID:    eventID,
		Type:       eventType,
		Payload:    payload,
		ServerTime: serverTime,
		Sequence:   sequence,
		Version:    contracts.CurrentVersion,
	}
}
