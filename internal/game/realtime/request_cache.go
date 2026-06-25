package realtime

import (
	"bytes"
	"crypto/sha256"
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
	entries  map[requestCacheKey]requestCacheEntry
	inFlight map[requestCacheKey]*requestCacheFlight
	order    []requestCacheKey
}

type requestCacheKey struct {
	sessionID SessionID
	requestID foundation.RequestID
}

type requestCacheFingerprint struct {
	op          Operation
	payloadHash [sha256.Size]byte
	version     int
}

type requestCacheEntry struct {
	fingerprint requestCacheFingerprint
	response    CachedResponse
}

type requestCacheFlight struct {
	fingerprint requestCacheFingerprint
	done        chan struct{}
	response    CachedResponse
	panicValue  any
}

type requestCacheResult string

const (
	requestCacheResultStored    requestCacheResult = "stored"
	requestCacheResultMiss      requestCacheResult = "miss"
	requestCacheResultDuplicate requestCacheResult = "duplicate"
	requestCacheResultMismatch  requestCacheResult = "mismatch"
)

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
		entries:  make(map[requestCacheKey]requestCacheEntry, capacity),
		inFlight: make(map[requestCacheKey]*requestCacheFlight),
		order:    make([]requestCacheKey, 0, capacity),
	}
}

// Lookup returns the cached completed response for an exact request replay.
func (cache *RequestCache) Lookup(sessionID SessionID, request RequestEnvelope) (CachedResponse, requestCacheResult) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	entry, ok := cache.entries[newRequestCacheKey(sessionID, request.RequestID)]
	if !ok {
		return CachedResponse{}, requestCacheResultMiss
	}
	if entry.fingerprint != newRequestCacheFingerprint(request) {
		return CachedResponse{}, requestCacheResultMismatch
	}
	return entry.response.clone(), requestCacheResultDuplicate
}

// Remember stores a completed response for an exact request replay.
func (cache *RequestCache) Remember(sessionID SessionID, request RequestEnvelope, response CachedResponse) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.rememberLocked(newRequestCacheKey(sessionID, request.RequestID), newRequestCacheFingerprint(request), response)
}

// ForgetSession removes completed transport retry responses for one session.
// In-flight requests are left intact so the active command can finish normally.
func (cache *RequestCache) ForgetSession(sessionID SessionID) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	filtered := cache.order[:0]
	for _, key := range cache.order {
		if key.sessionID == sessionID {
			delete(cache.entries, key)
			continue
		}
		filtered = append(filtered, key)
	}
	cache.order = filtered
}

// GetOrRemember returns a cached duplicate response or stores the built response.
func (cache *RequestCache) GetOrRemember(sessionID SessionID, request RequestEnvelope, build func() CachedResponse) (CachedResponse, requestCacheResult) {
	key := newRequestCacheKey(sessionID, request.RequestID)
	fingerprint := newRequestCacheFingerprint(request)
	cache.mu.Lock()
	if cached, ok := cache.entries[key]; ok {
		cache.mu.Unlock()
		if cached.fingerprint != fingerprint {
			return CachedResponse{}, requestCacheResultMismatch
		}
		return cached.response.clone(), requestCacheResultDuplicate
	}
	if flight, ok := cache.inFlight[key]; ok {
		if flight.fingerprint != fingerprint {
			cache.mu.Unlock()
			return CachedResponse{}, requestCacheResultMismatch
		}
		cache.mu.Unlock()
		notifyRequestCacheInFlightWait(key, requestCacheFlightWaitBefore)
		<-flight.done
		notifyRequestCacheInFlightWait(key, requestCacheFlightWaitAfter)
		if flight.panicValue != nil {
			panic(flight.panicValue)
		}
		return flight.response.clone(), requestCacheResultDuplicate
	}
	flight := &requestCacheFlight{fingerprint: fingerprint, done: make(chan struct{})}
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

	if cacheableResponse(response) {
		cache.rememberLocked(key, fingerprint, response)
	}
	flight.response = response.clone()
	delete(cache.inFlight, key)
	close(flight.done)
	return response.clone(), requestCacheResultStored
}

func cacheableResponse(response CachedResponse) bool {
	return !response.HasError || !response.Error.Error.Retryable
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

func (cache *RequestCache) rememberLocked(key requestCacheKey, fingerprint requestCacheFingerprint, response CachedResponse) {
	if _, exists := cache.entries[key]; !exists {
		cache.order = append(cache.order, key)
	}

	cache.entries[key] = requestCacheEntry{
		fingerprint: fingerprint,
		response:    response.clone(),
	}

	for len(cache.order) > cache.capacity {
		evicted := cache.order[0]
		copy(cache.order, cache.order[1:])
		cache.order = cache.order[:len(cache.order)-1]
		delete(cache.entries, evicted)
	}
}

func newRequestCacheFingerprint(request RequestEnvelope) requestCacheFingerprint {
	return requestCacheFingerprint{
		op:          request.Op,
		payloadHash: sha256.Sum256(bytes.TrimSpace(request.Payload)),
		version:     request.Version,
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
