package realtime

import (
	"sync"

	"gameproject/internal/game/foundation"
)

// SessionID identifies one authenticated realtime connection/session.
type SessionID string

// CachedResponse stores the completed envelope returned for a request.
type CachedResponse struct {
	Response ResponseEnvelope
	Error    ErrorEnvelope
	HasError bool
}

// CachedSuccess returns a cache value for a successful response.
func CachedSuccess(response ResponseEnvelope) CachedResponse {
	return CachedResponse{
		Response: cloneResponseEnvelope(response),
	}
}

// CachedError returns a cache value for a failed response.
func CachedError(response ErrorEnvelope) CachedResponse {
	return CachedResponse{
		Error:    cloneErrorEnvelope(response),
		HasError: true,
	}
}

// RequestCache remembers completed responses by session and request id.
//
// It is only transport retry memory. Domain idempotency keys, transaction
// locks, and value mutation safety still belong to the owning services.
type RequestCache struct {
	mu       sync.Mutex
	capacity int
	entries  map[requestCacheKey]CachedResponse
	inFlight map[requestCacheKey]*requestCacheFlight
	order    []requestCacheKey
}

type requestCacheKey struct {
	sessionID SessionID
	requestID foundation.RequestID
}

type requestCacheFlight struct {
	done       chan struct{}
	response   CachedResponse
	panicValue any
}

type requestCacheFlightWaitPhase string

const (
	requestCacheFlightWaitBefore requestCacheFlightWaitPhase = "before"
	requestCacheFlightWaitAfter  requestCacheFlightWaitPhase = "after"
)

// requestCacheInFlightWaitHook is nil in production; same-package tests install
// it to make in-flight waiter synchronization deterministic.
var requestCacheInFlightWaitHook func(requestCacheKey, requestCacheFlightWaitPhase)

// NewRequestCache returns a bounded cache. Capacity less than one stores one item.
func NewRequestCache(capacity int) *RequestCache {
	if capacity < 1 {
		capacity = 1
	}
	return &RequestCache{
		capacity: capacity,
		entries:  make(map[requestCacheKey]CachedResponse, capacity),
		inFlight: make(map[requestCacheKey]*requestCacheFlight),
		order:    make([]requestCacheKey, 0, capacity),
	}
}

// Lookup returns the cached completed response for sessionID/requestID.
func (cache *RequestCache) Lookup(sessionID SessionID, requestID foundation.RequestID) (CachedResponse, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	response, ok := cache.entries[newRequestCacheKey(sessionID, requestID)]
	if !ok {
		return CachedResponse{}, false
	}
	return response.clone(), true
}

// Remember stores a completed response for sessionID/requestID.
func (cache *RequestCache) Remember(sessionID SessionID, requestID foundation.RequestID, response CachedResponse) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.rememberLocked(newRequestCacheKey(sessionID, requestID), response)
}

// GetOrRemember returns a cached duplicate response or stores the built response.
func (cache *RequestCache) GetOrRemember(sessionID SessionID, requestID foundation.RequestID, build func() CachedResponse) (CachedResponse, bool) {
	key := newRequestCacheKey(sessionID, requestID)
	cache.mu.Lock()
	if cached, ok := cache.entries[key]; ok {
		cache.mu.Unlock()
		return cached.clone(), true
	}
	if flight, ok := cache.inFlight[key]; ok {
		cache.mu.Unlock()
		notifyRequestCacheInFlightWait(key, requestCacheFlightWaitBefore)
		<-flight.done
		notifyRequestCacheInFlightWait(key, requestCacheFlightWaitAfter)
		if flight.panicValue != nil {
			panic(flight.panicValue)
		}
		return flight.response.clone(), true
	}
	flight := &requestCacheFlight{done: make(chan struct{})}
	cache.inFlight[key] = flight
	cache.mu.Unlock()

	defer func() {
		if panicValue := recover(); panicValue != nil {
			cache.mu.Lock()
			flight.panicValue = panicValue
			delete(cache.inFlight, key)
			close(flight.done)
			cache.mu.Unlock()
			panic(panicValue)
		}
	}()

	response := build()

	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.rememberLocked(key, response)
	flight.response = response.clone()
	delete(cache.inFlight, key)
	close(flight.done)
	return response.clone(), false
}

func notifyRequestCacheInFlightWait(key requestCacheKey, phase requestCacheFlightWaitPhase) {
	if requestCacheInFlightWaitHook != nil {
		requestCacheInFlightWaitHook(key, phase)
	}
}

// Len returns the current number of cached responses.
func (cache *RequestCache) Len() int {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return len(cache.entries)
}

func (cache *RequestCache) rememberLocked(key requestCacheKey, response CachedResponse) {
	if _, exists := cache.entries[key]; !exists {
		cache.order = append(cache.order, key)
	}

	cache.entries[key] = response.clone()

	for len(cache.order) > cache.capacity {
		evicted := cache.order[0]
		copy(cache.order, cache.order[1:])
		cache.order = cache.order[:len(cache.order)-1]
		delete(cache.entries, evicted)
	}
}

func newRequestCacheKey(sessionID SessionID, requestID foundation.RequestID) requestCacheKey {
	return requestCacheKey{
		sessionID: sessionID,
		requestID: requestID,
	}
}

func (response CachedResponse) clone() CachedResponse {
	if response.HasError {
		return CachedError(response.Error)
	}
	return CachedSuccess(response.Response)
}

func cloneResponseEnvelope(response ResponseEnvelope) ResponseEnvelope {
	response.Payload = cloneRawMessage(response.Payload)
	return response
}

func cloneErrorEnvelope(response ErrorEnvelope) ErrorEnvelope {
	return response
}
