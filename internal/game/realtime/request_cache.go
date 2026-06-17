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
	order    []requestCacheKey
}

type requestCacheKey struct {
	sessionID SessionID
	requestID foundation.RequestID
}

// NewRequestCache returns a bounded cache. Capacity less than one stores one item.
func NewRequestCache(capacity int) *RequestCache {
	if capacity < 1 {
		capacity = 1
	}
	return &RequestCache{
		capacity: capacity,
		entries:  make(map[requestCacheKey]CachedResponse, capacity),
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
//
// The build function creates the completed response for the first observed
// request. This skeleton does not coordinate in-flight duplicate execution.
func (cache *RequestCache) GetOrRemember(sessionID SessionID, requestID foundation.RequestID, build func() CachedResponse) (CachedResponse, bool) {
	if cached, ok := cache.Lookup(sessionID, requestID); ok {
		return cached, true
	}

	response := build()

	cache.mu.Lock()
	defer cache.mu.Unlock()

	key := newRequestCacheKey(sessionID, requestID)
	if cached, ok := cache.entries[key]; ok {
		return cached.clone(), true
	}

	cache.rememberLocked(key, response)
	return response.clone(), false
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
