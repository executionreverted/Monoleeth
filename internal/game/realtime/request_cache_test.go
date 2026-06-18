package realtime

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestRequestCacheCoordinatesInFlightDuplicateRequestID(t *testing.T) {
	cache := NewRequestCache(2)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	secondBuildCalled := make(chan struct{})
	results := make(chan struct {
		response  CachedResponse
		duplicate bool
	}, 2)
	var builds int32
	var closeFirstStarted sync.Once

	go func() {
		response, duplicate := cache.GetOrRemember(
			SessionID("session-1"),
			foundation.RequestID("request-1"),
			func() CachedResponse {
				atomic.AddInt32(&builds, 1)
				closeFirstStarted.Do(func() { close(firstStarted) })
				<-releaseFirst
				return CachedSuccess(NewResponseEnvelope(
					foundation.RequestID("request-1"),
					json.RawMessage(`{"status":"accepted"}`),
					100,
				))
			},
		)
		results <- struct {
			response  CachedResponse
			duplicate bool
		}{response: response, duplicate: duplicate}
	}()

	<-firstStarted
	go func() {
		close(secondStarted)
		response, duplicate := cache.GetOrRemember(
			SessionID("session-1"),
			foundation.RequestID("request-1"),
			func() CachedResponse {
				close(secondBuildCalled)
				atomic.AddInt32(&builds, 1)
				return CachedSuccess(NewResponseEnvelope(
					foundation.RequestID("request-1"),
					json.RawMessage(`{"status":"different"}`),
					200,
				))
			},
		)
		results <- struct {
			response  CachedResponse
			duplicate bool
		}{response: response, duplicate: duplicate}
	}()
	<-secondStarted

	select {
	case <-secondBuildCalled:
		t.Fatal("in-flight duplicate built its own response")
	case result := <-results:
		t.Fatalf("duplicate returned before first request completed: %+v", result)
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseFirst)
	first := <-results
	second := <-results

	if atomic.LoadInt32(&builds) != 1 {
		t.Fatalf("builds = %d, want 1", builds)
	}
	if first.duplicate == second.duplicate {
		t.Fatalf("duplicate flags = %t/%t, want exactly one duplicate", first.duplicate, second.duplicate)
	}
	for _, result := range []CachedResponse{first.response, second.response} {
		if result.HasError {
			t.Fatal("cached in-flight result returned error")
		}
		if got := string(result.Response.Payload); got != `{"status":"accepted"}` {
			t.Fatalf("cached payload = %s, want first response payload", got)
		}
	}
}

func TestRequestCacheReleasesInFlightDuplicateWhenBuildPanics(t *testing.T) {
	cache := NewRequestCache(2)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	secondBuildCalled := make(chan struct{})
	results := make(chan any, 2)
	var builds int32

	go func() {
		defer func() { results <- recover() }()
		_, _ = cache.GetOrRemember(
			SessionID("session-1"),
			foundation.RequestID("request-1"),
			func() CachedResponse {
				atomic.AddInt32(&builds, 1)
				close(firstStarted)
				<-releaseFirst
				panic("handler panic")
			},
		)
	}()

	<-firstStarted
	go func() {
		defer func() { results <- recover() }()
		close(secondStarted)
		_, _ = cache.GetOrRemember(
			SessionID("session-1"),
			foundation.RequestID("request-1"),
			func() CachedResponse {
				close(secondBuildCalled)
				atomic.AddInt32(&builds, 1)
				return CachedSuccess(NewResponseEnvelope(
					foundation.RequestID("request-1"),
					json.RawMessage(`{"status":"different"}`),
					200,
				))
			},
		)
	}()
	<-secondStarted

	select {
	case <-secondBuildCalled:
		t.Fatal("in-flight duplicate built its own response after first panic")
	case result := <-results:
		t.Fatalf("duplicate returned before first panic completed: %+v", result)
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseFirst)
	firstPanic := <-results
	secondPanic := <-results
	if firstPanic != "handler panic" || secondPanic != "handler panic" {
		t.Fatalf("panic results = %#v/%#v, want propagated handler panic", firstPanic, secondPanic)
	}
	if atomic.LoadInt32(&builds) != 1 {
		t.Fatalf("builds after panic = %d, want 1", builds)
	}

	retry, duplicate := cache.GetOrRemember(
		SessionID("session-1"),
		foundation.RequestID("request-1"),
		func() CachedResponse {
			atomic.AddInt32(&builds, 1)
			return CachedSuccess(NewResponseEnvelope(
				foundation.RequestID("request-1"),
				json.RawMessage(`{"status":"retry"}`),
				300,
			))
		},
	)
	if duplicate {
		t.Fatal("retry after panic reported duplicate")
	}
	if atomic.LoadInt32(&builds) != 2 {
		t.Fatalf("builds after retry = %d, want 2", builds)
	}
	if got := string(retry.Response.Payload); got != `{"status":"retry"}` {
		t.Fatalf("retry payload = %s, want retry response", got)
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
