package realtime

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestRequestCacheDuplicateRequestIDReturnsCachedResponse(t *testing.T) {
	cache := NewRequestCache(2)
	var builds int

	first, duplicate := cache.GetOrRemember(
		SessionID("session-1"),
		foundation.RequestID("request-1"),
		func() CachedResponse {
			builds++
			return CachedSuccess(NewResponseEnvelope(
				foundation.RequestID("request-1"),
				json.RawMessage(`{"status":"accepted"}`),
				100,
			))
		},
	)
	if duplicate {
		t.Fatal("first request unexpectedly reported duplicate")
	}
	if builds != 1 {
		t.Fatalf("builds = %d, want 1", builds)
	}

	second, duplicate := cache.GetOrRemember(
		SessionID("session-1"),
		foundation.RequestID("request-1"),
		func() CachedResponse {
			builds++
			return CachedSuccess(NewResponseEnvelope(
				foundation.RequestID("request-1"),
				json.RawMessage(`{"status":"different"}`),
				200,
			))
		},
	)
	if !duplicate {
		t.Fatal("duplicate request did not return cached response")
	}
	if builds != 1 {
		t.Fatalf("duplicate request rebuilt response: builds = %d, want 1", builds)
	}
	if second.HasError {
		t.Fatal("cached duplicate returned an error response")
	}
	if second.Response.RequestID != first.Response.RequestID ||
		second.Response.OK != first.Response.OK ||
		second.Response.ServerTime != first.Response.ServerTime ||
		second.Response.Version != first.Response.Version {
		t.Fatalf("cached response metadata = %+v, want %+v", second.Response, first.Response)
	}
	if got := string(second.Response.Payload); got != `{"status":"accepted"}` {
		t.Fatalf("cached payload = %s, want first response payload", got)
	}
}

func TestRequestCacheKeysDuplicatesBySessionAndRequestID(t *testing.T) {
	cache := NewRequestCache(2)
	cache.Remember(
		SessionID("session-1"),
		foundation.RequestID("request-1"),
		CachedSuccess(NewResponseEnvelope(
			foundation.RequestID("request-1"),
			json.RawMessage(`{"session":"one"}`),
			100,
		)),
	)

	response, duplicate := cache.GetOrRemember(
		SessionID("session-2"),
		foundation.RequestID("request-1"),
		func() CachedResponse {
			return CachedSuccess(NewResponseEnvelope(
				foundation.RequestID("request-1"),
				json.RawMessage(`{"session":"two"}`),
				200,
			))
		},
	)
	if duplicate {
		t.Fatal("same request id in a different session should not be duplicate")
	}
	if got := string(response.Response.Payload); got != `{"session":"two"}` {
		t.Fatalf("response payload = %s, want second session payload", got)
	}
}

func TestRequestCacheEvictsOldestResponseAtCapacity(t *testing.T) {
	cache := NewRequestCache(2)

	cache.Remember(SessionID("session-1"), foundation.RequestID("request-1"), cachedPayload("request-1", `{"n":1}`))
	cache.Remember(SessionID("session-1"), foundation.RequestID("request-2"), cachedPayload("request-2", `{"n":2}`))
	cache.Remember(SessionID("session-1"), foundation.RequestID("request-3"), cachedPayload("request-3", `{"n":3}`))

	if _, ok := cache.Lookup(SessionID("session-1"), foundation.RequestID("request-1")); ok {
		t.Fatal("oldest response was not evicted")
	}
	if _, ok := cache.Lookup(SessionID("session-1"), foundation.RequestID("request-2")); !ok {
		t.Fatal("request-2 should remain cached")
	}
	if _, ok := cache.Lookup(SessionID("session-1"), foundation.RequestID("request-3")); !ok {
		t.Fatal("request-3 should remain cached")
	}
	if got := cache.Len(); got != 2 {
		t.Fatalf("cache len = %d, want 2", got)
	}
}

func TestRequestCacheCanStoreErrorResponses(t *testing.T) {
	cache := NewRequestCache(1)
	domainErr := foundation.NewDomainError(foundation.CodeRateLimited, "Request rate limited.")

	cache.Remember(
		SessionID("session-1"),
		foundation.RequestID("request-1"),
		CachedError(NewErrorEnvelope(foundation.RequestID("request-1"), domainErr, true, 100)),
	)

	response, ok := cache.Lookup(SessionID("session-1"), foundation.RequestID("request-1"))
	if !ok {
		t.Fatal("cached error response not found")
	}
	if !response.HasError {
		t.Fatal("cached response did not preserve error state")
	}
	if response.Error.Error.Code != foundation.CodeRateLimited {
		t.Fatalf("cached error code = %s, want %s", response.Error.Error.Code, foundation.CodeRateLimited)
	}
}

func cachedPayload(requestID foundation.RequestID, payload string) CachedResponse {
	return CachedSuccess(NewResponseEnvelope(
		requestID,
		json.RawMessage(payload),
		100,
	))
}
