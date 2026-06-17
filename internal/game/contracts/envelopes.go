package contracts

import (
	"encoding/json"

	"gameproject/internal/game/foundation"
)

// CurrentVersion is the MVP JSON protocol version.
const CurrentVersion = 1

// RequestEnvelope is the client-to-server command or query envelope.
//
// RequestID is transport retry identity. Domain idempotency keys are separate
// service-level concerns and are intentionally not modeled on this envelope.
type RequestEnvelope struct {
	RequestID foundation.RequestID `json:"request_id"`
	Op        string               `json:"op"`
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

// NewRequestEnvelope returns a request envelope using the current protocol version.
func NewRequestEnvelope(requestID foundation.RequestID, op string, payload json.RawMessage, clientSeq uint64) RequestEnvelope {
	return RequestEnvelope{
		RequestID: requestID,
		Op:        op,
		Payload:   payload,
		ClientSeq: clientSeq,
		Version:   CurrentVersion,
	}
}

// NewResponseEnvelope returns a successful response envelope.
func NewResponseEnvelope(requestID foundation.RequestID, payload json.RawMessage, serverTime int64) ResponseEnvelope {
	return ResponseEnvelope{
		RequestID:  requestID,
		OK:         true,
		Payload:    payload,
		ServerTime: serverTime,
		Version:    CurrentVersion,
	}
}

// NewErrorEnvelope returns a failed response envelope from a domain error.
func NewErrorEnvelope(requestID foundation.RequestID, domainErr *foundation.DomainError, retryable bool, serverTime int64) ErrorEnvelope {
	publicErr := foundation.PublicError{}
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
