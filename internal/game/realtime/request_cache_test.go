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
	waitBefore, waitAfter := observeRequestCacheInFlightWait(t)
	results := make(chan struct {
		response  CachedResponse
		duplicate bool
	}, 2)
	var builds int32

	go func() {
		response, duplicate := cache.GetOrRemember(
			SessionID("session-1"),
			foundation.RequestID("request-1"),
			func() CachedResponse {
				atomic.AddInt32(&builds, 1)
				close(firstStarted)
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
		response, duplicate := cache.GetOrRemember(
			SessionID("session-1"),
			foundation.RequestID("request-1"),
			func() CachedResponse {
				panic("in-flight duplicate built its own response")
			},
		)
		results <- struct {
			response  CachedResponse
			duplicate bool
		}{response: response, duplicate: duplicate}
	}()

	<-waitBefore
	select {
	case <-waitAfter:
		t.Fatal("in-flight duplicate finished waiting before first request completed")
	case result := <-results:
		t.Fatalf("in-flight duplicate returned before first request completed: %+v", result)
	default:
	}

	close(releaseFirst)
	<-waitAfter
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

func TestRequestCacheReleasesInFlightWhenBuildPanics(t *testing.T) {
	cache := NewRequestCache(2)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	result := make(chan any, 1)
	var builds int32

	go func() {
		defer func() { result <- recover() }()
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
	key := newRequestCacheKey(SessionID("session-1"), foundation.RequestID("request-1"))
	cache.mu.Lock()
	flight := cache.inFlight[key]
	cache.mu.Unlock()
	if flight == nil {
		t.Fatal("missing in-flight request before panic")
	}
	waiterPanic := make(chan any, 1)
	go func() {
		<-flight.done
		waiterPanic <- flight.panicValue
	}()

	close(releaseFirst)
	if panicValue := <-result; panicValue != "handler panic" {
		t.Fatalf("panic result = %#v, want handler panic", panicValue)
	}
	select {
	case <-flight.done:
	case <-time.After(time.Second):
		t.Fatal("flight was not released after panic")
	}
	if flight.panicValue != "handler panic" {
		t.Fatalf("flight panic value = %#v, want handler panic", flight.panicValue)
	}
	if panicValue := <-waiterPanic; panicValue != "handler panic" {
		t.Fatalf("waiter panic value = %#v, want handler panic", panicValue)
	}
	cache.mu.Lock()
	_, stillInFlight := cache.inFlight[key]
	cache.mu.Unlock()
	if stillInFlight {
		t.Fatal("in-flight entry was not deleted after panic")
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

func TestRequestCacheInFlightWaiterReceivesPanic(t *testing.T) {
	cache := NewRequestCache(2)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	waitBefore, waitAfter := observeRequestCacheInFlightWait(t)
	ownerResult := make(chan any, 1)
	waiterResult := make(chan any, 1)
	var builds int32

	go func() {
		defer func() { ownerResult <- recover() }()
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
		defer func() { waiterResult <- recover() }()
		_, _ = cache.GetOrRemember(
			SessionID("session-1"),
			foundation.RequestID("request-1"),
			func() CachedResponse {
				panic("waiter built a response instead of observing the in-flight panic")
			},
		)
	}()

	<-waitBefore
	select {
	case <-waitAfter:
		t.Fatal("in-flight panic waiter finished waiting before first request completed")
	case panicValue := <-waiterResult:
		t.Fatalf("in-flight panic waiter returned before first request completed: %#v", panicValue)
	default:
	}

	close(releaseFirst)
	<-waitAfter
	if panicValue := <-ownerResult; panicValue != "handler panic" {
		t.Fatalf("owner panic result = %#v, want handler panic", panicValue)
	}
	if panicValue := <-waiterResult; panicValue != "handler panic" {
		t.Fatalf("waiter panic result = %#v, want handler panic", panicValue)
	}
	if atomic.LoadInt32(&builds) != 1 {
		t.Fatalf("builds after panic = %d, want 1", builds)
	}
}

func observeRequestCacheInFlightWait(t *testing.T) (<-chan struct{}, <-chan struct{}) {
	t.Helper()
	waitBefore := make(chan struct{})
	waitAfter := make(chan struct{})
	var beforeOnce sync.Once
	var afterOnce sync.Once
	previous := requestCacheInFlightWaitHook
	requestCacheInFlightWaitHook = func(_ requestCacheKey, phase requestCacheFlightWaitPhase) {
		switch phase {
		case requestCacheFlightWaitBefore:
			beforeOnce.Do(func() { close(waitBefore) })
		case requestCacheFlightWaitAfter:
			afterOnce.Do(func() { close(waitAfter) })
		}
	}
	t.Cleanup(func() {
		requestCacheInFlightWaitHook = previous
	})
	return waitBefore, waitAfter
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
