package contracts

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestRequestEnvelopeModelsRequestIDSeparatelyFromDomainIdempotency(t *testing.T) {
	envelope := NewRequestEnvelope(
		foundation.RequestID("request-123"),
		"market.buy",
		json.RawMessage(`{"listing_id":"listing-9"}`),
		42,
	)

	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json marshal request envelope: %v", err)
	}

	got := string(payload)
	want := `{"request_id":"request-123","op":"market.buy","payload":{"listing_id":"listing-9"},"client_seq":42,"v":1}`
	if got != want {
		t.Fatalf("request envelope JSON = %s, want %s", got, want)
	}
	for _, unexpected := range []string{"idempotency", "domain_key"} {
		if strings.Contains(got, unexpected) {
			t.Fatalf("request envelope included domain idempotency concern %q in %s", unexpected, got)
		}
	}

	requestIDField, ok := reflect.TypeOf(RequestEnvelope{}).FieldByName("RequestID")
	if !ok {
		t.Fatal("RequestEnvelope missing RequestID field")
	}
	if requestIDField.Type != reflect.TypeOf(foundation.RequestID("")) {
		t.Fatalf("RequestID type = %v, want foundation.RequestID", requestIDField.Type)
	}
	if _, ok := reflect.TypeOf(RequestEnvelope{}).FieldByName("IdempotencyKey"); ok {
		t.Fatal("RequestEnvelope modeled a domain idempotency key")
	}
}

func TestResponseEnvelopeJSONShapeIsStable(t *testing.T) {
	envelope := NewResponseEnvelope(
		foundation.RequestID("request-456"),
		json.RawMessage(`{"credits":1250,"cargo_slots":8}`),
		182736123,
	)

	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json marshal response envelope: %v", err)
	}

	want := `{"request_id":"request-456","ok":true,"payload":{"credits":1250,"cargo_slots":8},"server_time":182736123,"v":1}`
	if got := string(payload); got != want {
		t.Fatalf("response envelope JSON = %s, want %s", got, want)
	}
}

func TestRequestAndResponseEnvelopesCopyPayloads(t *testing.T) {
	requestPayload := json.RawMessage(`{"listing_id":"listing-9"}`)
	request := NewRequestEnvelope(foundation.RequestID("request-123"), "market.buy", requestPayload, 42)
	requestPayload[15] = 'X'
	if got := string(request.Payload); got != `{"listing_id":"listing-9"}` {
		t.Fatalf("request payload changed after source mutation: %s", got)
	}

	responsePayload := json.RawMessage(`{"credits":1250}`)
	response := NewResponseEnvelope(foundation.RequestID("request-456"), responsePayload, 182736123)
	responsePayload[11] = '9'
	if got := string(response.Payload); got != `{"credits":1250}` {
		t.Fatalf("response payload changed after source mutation: %s", got)
	}
}

func TestErrorEnvelopeJSONShapeIsStable(t *testing.T) {
	domainErr := foundation.NewDomainError(
		foundation.CodeOutOfRange,
		"Target is out of range.",
	)
	envelope := NewErrorEnvelope(foundation.RequestID("request-789"), domainErr, false, 182736124)

	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json marshal error envelope: %v", err)
	}

	want := `{"request_id":"request-789","ok":false,"error":{"code":"ERR_OUT_OF_RANGE","message":"Target is out of range.","retryable":false},"server_time":182736124,"v":1}`
	if got := string(payload); got != want {
		t.Fatalf("error envelope JSON = %s, want %s", got, want)
	}
}

func TestErrorEnvelopeNilDomainErrorUsesSafeInternalFallback(t *testing.T) {
	envelope := NewErrorEnvelope(foundation.RequestID("request-nil"), nil, true, 182736126)

	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json marshal nil error envelope: %v", err)
	}

	want := `{"request_id":"request-nil","ok":false,"error":{"code":"ERR_INTERNAL","message":"Request failed.","retryable":true},"server_time":182736126,"v":1}`
	if got := string(payload); got != want {
		t.Fatalf("nil error envelope JSON = %s, want %s", got, want)
	}
}

func TestErrorEnvelopeDoesNotLeakDomainErrorInternals(t *testing.T) {
	domainErr := foundation.NewDomainError(
		foundation.CodeNotVisible,
		"No valid signal found.",
		foundation.WithDetail("hidden planet planet-9 at 200,300 requires radar 4"),
		foundation.WithCause(errors.New("sql row player-123 exposed hidden target")),
	)
	envelope := NewErrorEnvelope(foundation.RequestID("request-hidden"), domainErr, false, 182736125)

	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json marshal error envelope: %v", err)
	}

	got := string(payload)
	for _, leaked := range []string{"hidden planet", "planet-9", "radar 4", "sql row", "player-123", "detail", "cause"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("error envelope leaked %q in %s", leaked, got)
		}
	}

	want := `{"request_id":"request-hidden","ok":false,"error":{"code":"ERR_NOT_VISIBLE","message":"No valid signal found.","retryable":false},"server_time":182736125,"v":1}`
	if got != want {
		t.Fatalf("error envelope JSON = %s, want %s", got, want)
	}
}
