package realtime

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"gameproject/internal/game/contracts"
	"gameproject/internal/game/foundation"
)

// CurrentVersion is the JSON realtime protocol version.
const CurrentVersion = contracts.CurrentVersion

// Operation is a client request operation name.
type Operation string

const (
	OperationMoveTo        Operation = "move_to"
	OperationStop          Operation = "stop"
	OperationDebugSpawnNPC Operation = "debug_spawn_npc"
	OperationDebugSnapshot Operation = "debug_snapshot"
)

// ClientEventType is an event name that may be sent to a client after filtering.
type ClientEventType string

const (
	EventPlayerSnapshot    ClientEventType = "player.snapshot"
	EventAOIEntityEntered  ClientEventType = "aoi.entity_entered"
	EventAOIEntityLeft     ClientEventType = "aoi.entity_left"
	EventPositionCorrected ClientEventType = "position.corrected"
)

// RateLimitPosture names the future per-operation abuse posture.
//
// This is intentionally metadata only. It does not enforce limits or affect
// gameplay truth.
type RateLimitPosture string

const (
	RateLimitPostureUnspecified RateLimitPosture = "unspecified"
	RateLimitPostureIntentBurst RateLimitPosture = "intent_burst"
	RateLimitPostureDebugOnly   RateLimitPosture = "debug_only"
)

// OperationSpec describes one registered realtime operation.
type OperationSpec struct {
	Operation        Operation
	RateLimitPosture RateLimitPosture
}

var phase04Operations = map[Operation]OperationSpec{
	OperationMoveTo: {
		Operation:        OperationMoveTo,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationStop: {
		Operation:        OperationStop,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationDebugSpawnNPC: {
		Operation:        OperationDebugSpawnNPC,
		RateLimitPosture: RateLimitPostureDebugOnly,
	},
	OperationDebugSnapshot: {
		Operation:        OperationDebugSnapshot,
		RateLimitPosture: RateLimitPostureDebugOnly,
	},
}

// LookupOperation returns the phase-04 operation spec for op.
func LookupOperation(op Operation) (OperationSpec, bool) {
	spec, ok := phase04Operations[op]
	return spec, ok
}

// OperationRegistry returns a copy of the phase-04 operation registry.
func OperationRegistry() map[Operation]OperationSpec {
	registry := make(map[Operation]OperationSpec, len(phase04Operations))
	for op, spec := range phase04Operations {
		registry[op] = spec
	}
	return registry
}

// RequestEnvelope is the client-to-server command/query envelope.
type RequestEnvelope struct {
	RequestID foundation.RequestID `json:"request_id"`
	Op        Operation            `json:"op"`
	Payload   json.RawMessage      `json:"payload"`
	ClientSeq uint64               `json:"client_seq"`
	Version   int                  `json:"v"`
}

// ResponseEnvelope is the successful server response envelope for a request.
type ResponseEnvelope struct {
	RequestID  foundation.RequestID `json:"request_id"`
	OK         bool                 `json:"ok"`
	Payload    json.RawMessage      `json:"payload"`
	ServerTime int64                `json:"server_time"`
	Version    int                  `json:"v"`
}

// ErrorPayload is the client-safe error body carried by an ErrorEnvelope.
type ErrorPayload struct {
	foundation.PublicError
	Retryable bool `json:"retryable"`
}

// ErrorEnvelope is the failed server response envelope for a request.
type ErrorEnvelope struct {
	RequestID  foundation.RequestID `json:"request_id"`
	OK         bool                 `json:"ok"`
	Error      ErrorPayload         `json:"error"`
	ServerTime int64                `json:"server_time"`
	Version    int                  `json:"v"`
}

// EventEnvelope is the server-to-client realtime event envelope.
type EventEnvelope struct {
	EventID    foundation.EventID `json:"event_id"`
	Type       ClientEventType    `json:"type"`
	Payload    json.RawMessage    `json:"payload"`
	ServerTime int64              `json:"server_time"`
	Sequence   uint64             `json:"seq"`
	Version    int                `json:"v"`
}

// NewRequestEnvelope returns a request envelope using the current protocol version.
func NewRequestEnvelope(requestID foundation.RequestID, op Operation, payload json.RawMessage, clientSeq uint64) RequestEnvelope {
	return RequestEnvelope{
		RequestID: requestID,
		Op:        op,
		Payload:   cloneRawMessage(payload),
		ClientSeq: clientSeq,
		Version:   CurrentVersion,
	}
}

// DecodeRequestEnvelope decodes and validates a request envelope.
func DecodeRequestEnvelope(data []byte) (RequestEnvelope, error) {
	var envelope RequestEnvelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return RequestEnvelope{}, foundation.NewDomainError(
			foundation.CodeInvalidPayload,
			"Invalid request envelope.",
			foundation.WithCause(err),
		)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return RequestEnvelope{}, foundation.NewDomainError(
			foundation.CodeInvalidPayload,
			"Invalid request envelope.",
		)
	}
	if err := envelope.Validate(); err != nil {
		return RequestEnvelope{}, err
	}
	return envelope, nil
}

// Validate checks envelope-level fields common to every realtime request.
func (envelope RequestEnvelope) Validate() error {
	if err := envelope.RequestID.Validate(); err != nil {
		return invalidRequest("request_id is required.", err)
	}
	if envelope.Version != 0 && envelope.Version != CurrentVersion {
		return invalidRequest("protocol version is not supported.", nil)
	}
	if strings.TrimSpace(string(envelope.Op)) == "" {
		return invalidRequest("op is required.", nil)
	}
	if _, ok := LookupOperation(envelope.Op); !ok {
		return invalidRequest("op is not registered.", nil)
	}
	if err := validateRequestPayload(envelope.Payload); err != nil {
		return err
	}
	return nil
}

// NewResponseEnvelope returns a successful response envelope.
func NewResponseEnvelope(requestID foundation.RequestID, payload json.RawMessage, serverTime int64) ResponseEnvelope {
	return ResponseEnvelope{
		RequestID:  requestID,
		OK:         true,
		Payload:    cloneRawMessage(payload),
		ServerTime: serverTime,
		Version:    CurrentVersion,
	}
}

// NewErrorEnvelope returns a failed response envelope from a domain error.
func NewErrorEnvelope(requestID foundation.RequestID, domainErr *foundation.DomainError, retryable bool, serverTime int64) ErrorEnvelope {
	publicErr := foundation.PublicError{
		Code:    foundation.CodeInternal,
		Message: "Request failed.",
	}
	if domainErr != nil {
		publicErr = domainErr.Public()
	}

	return ErrorEnvelope{
		RequestID: requestID,
		OK:        false,
		Error: ErrorPayload{
			PublicError: publicErr,
			Retryable:   retryable,
		},
		ServerTime: serverTime,
		Version:    CurrentVersion,
	}
}

// NewEventEnvelope returns a filtered server-to-client realtime event envelope.
func NewEventEnvelope(eventID foundation.EventID, eventType ClientEventType, payload json.RawMessage, serverTime int64, sequence uint64) EventEnvelope {
	return EventEnvelope{
		EventID:    eventID,
		Type:       eventType,
		Payload:    cloneRawMessage(payload),
		ServerTime: serverTime,
		Sequence:   sequence,
		Version:    CurrentVersion,
	}
}

func validateRequestPayload(payload json.RawMessage) error {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return invalidRequest("payload is required.", nil)
	}
	if !json.Valid(trimmed) {
		return invalidRequest("payload must be valid JSON.", nil)
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return invalidRequest("payload is required.", nil)
	}
	if trimmed[0] != '{' {
		return invalidRequest("payload must be a JSON object.", nil)
	}
	return nil
}

func invalidRequest(message string, cause error) *foundation.DomainError {
	opts := make([]foundation.DomainErrorOption, 0, 1)
	if cause != nil {
		opts = append(opts, foundation.WithCause(cause))
	}
	return foundation.NewDomainError(foundation.CodeInvalidPayload, message, opts...)
}

func cloneRawMessage(payload json.RawMessage) json.RawMessage {
	if payload == nil {
		return nil
	}
	return append(json.RawMessage(nil), payload...)
}
