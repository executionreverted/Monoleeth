package realtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gameproject/internal/game/foundation"
)

const defaultGatewayRequestCacheCapacity = 128

var (
	ErrInvalidSessionID   = errors.New("invalid session id")
	ErrNilSessionResolver = errors.New("nil session resolver")
)

// SessionResolver maps a transport-authenticated session to server-owned
// command identity. Implementations must not read identity from request
// payloads.
type SessionResolver interface {
	ResolveSession(sessionID SessionID) (CommandContext, error)
}

// GatewayOptions configures the transport-agnostic realtime request boundary.
type GatewayOptions struct {
	Clock    foundation.Clock
	Sessions SessionResolver
	Cache    *RequestCache
	Executor ObservedCommandExecutor
	Handlers map[Operation]CommandHandler
}

// Gateway decodes realtime envelopes, resolves authenticated session identity,
// executes operation handlers with server-owned context, and caches completed
// responses by session/request id.
type Gateway struct {
	clock    foundation.Clock
	sessions SessionResolver
	cache    *RequestCache
	executor ObservedCommandExecutor
	handlers map[Operation]CommandHandler
}

// NewGateway returns an in-process gateway boundary for WebSocket/API adapters.
func NewGateway(options GatewayOptions) (*Gateway, error) {
	if options.Sessions == nil {
		return nil, ErrNilSessionResolver
	}
	clock := options.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	cache := options.Cache
	if cache == nil {
		cache = NewRequestCache(defaultGatewayRequestCacheCapacity)
	}
	executor := options.Executor
	if executor.Clock == nil {
		executor.Clock = clock
	}
	return &Gateway{
		clock:    clock,
		sessions: options.Sessions,
		cache:    cache,
		executor: executor,
		handlers: cloneCommandHandlers(options.Handlers),
	}, nil
}

// HandleRequest processes one raw client request for an already-authenticated
// transport session. Player, world, and zone identity come only from the
// SessionResolver.
func (gateway *Gateway) HandleRequest(sessionID SessionID, data []byte) CachedResponse {
	request, err := DecodeRequestEnvelope(data)
	if err != nil {
		return gateway.cachedError("", err)
	}
	if err := sessionID.Validate(); err != nil {
		return gateway.cachedError(request.RequestID, foundation.NewDomainError(
			foundation.CodeUnauthenticated,
			"Authenticated session is required.",
			foundation.WithCause(err),
		))
	}
	response, result := gateway.cache.GetOrRemember(sessionID, request, func() CachedResponse {
		return gateway.executeResolved(sessionID, request)
	})
	if result == requestCacheResultMismatch {
		return gateway.cachedError(request.RequestID, requestReplayMismatchError())
	}
	return response
}

// ForgetSessionCache drops completed retry responses for one transport session.
func (gateway *Gateway) ForgetSessionCache(sessionID SessionID) {
	if gateway == nil || gateway.cache == nil {
		return
	}
	gateway.cache.ForgetSession(sessionID)
}

func (gateway *Gateway) executeResolved(sessionID SessionID, request RequestEnvelope) CachedResponse {
	ctx, err := gateway.sessions.ResolveSession(sessionID)
	if err != nil {
		return gateway.cachedError(request.RequestID, err)
	}
	if ctx.SessionID != "" && ctx.SessionID != sessionID {
		return gateway.cachedError(request.RequestID, foundation.NewDomainError(
			foundation.CodeUnauthenticated,
			"Authenticated session is invalid.",
			foundation.WithDetail(fmt.Sprintf("resolver returned session %q for transport session %q", ctx.SessionID, sessionID)),
		))
	}
	ctx.SessionID = sessionID

	payload, err := gateway.executor.Execute(ctx, request, gateway.handlers[request.Op])
	if err != nil {
		return gateway.cachedError(request.RequestID, err)
	}
	return CachedSuccess(NewResponseEnvelope(request.RequestID, normalizeResponsePayload(payload), gateway.serverTime()))
}

func (gateway *Gateway) cachedError(requestID foundation.RequestID, err error) CachedResponse {
	domainErr := domainErrorForGateway(err)
	return CachedError(NewErrorEnvelope(requestID, domainErr, domainErr.Public().Code == foundation.CodeInternal, gateway.serverTime()))
}

func (gateway *Gateway) serverTime() int64 {
	if gateway == nil || gateway.clock == nil {
		return foundation.RealClock{}.Now().UTC().UnixMilli()
	}
	return gateway.clock.Now().UTC().UnixMilli()
}

func domainErrorForGateway(err error) *foundation.DomainError {
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	return foundation.NewDomainError(foundation.CodeInternal, "Request failed.", foundation.WithCause(err))
}

func requestReplayMismatchError() *foundation.DomainError {
	return foundation.NewDomainError(foundation.CodeRequestReplayMismatch, "Request replay does not match original request.")
}

func normalizeResponsePayload(payload json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return json.RawMessage(`{}`)
	}
	return cloneRawMessage(payload)
}

func cloneCommandHandlers(handlers map[Operation]CommandHandler) map[Operation]CommandHandler {
	if len(handlers) == 0 {
		return nil
	}
	cloned := make(map[Operation]CommandHandler, len(handlers))
	for op, handler := range handlers {
		cloned[op] = handler
	}
	return cloned
}

// Validate reports whether id can identify an authenticated realtime session.
func (id SessionID) Validate() error {
	value := string(id)
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || strings.Contains(value, ":") {
		return fmt.Errorf("session %q: %w", id, ErrInvalidSessionID)
	}
	return nil
}

// String returns the stable session id representation.
func (id SessionID) String() string {
	return string(id)
}

// IsZero reports whether id is the zero value.
func (id SessionID) IsZero() bool {
	return id == ""
}
