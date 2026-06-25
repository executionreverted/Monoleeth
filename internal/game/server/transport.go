package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

const outboundQueueSize = 64

type outboundMessage struct {
	payload     []byte
	closeAfter  bool
	closeStatus websocket.StatusCode
	closeReason string
}

type websocketWriter interface {
	Write(context.Context, websocket.MessageType, []byte) error
	Close(websocket.StatusCode, string) error
	CloseNow() error
}

func (server *Server) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	resolved, err := auth.ResolveCookie(r.Context(), server.runtime.Auth, auth.DefaultSessionCookieName, server.config.originPolicy(), r)
	if err != nil {
		writeHTTPError(w, err)
		return
	}
	if err := server.runtime.ensurePlayerSession(resolved); err != nil {
		writeHTTPError(w, foundation.NewDomainError(foundation.CodeInternal, "Session bootstrap failed.", foundation.WithCause(err)))
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Origin was already checked by auth.OriginPolicy above. Keeping this
		// disabled avoids a second host-pattern policy from drifting.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	client := newClientConnection(conn, resolved.SessionID)
	client.startWriter(server.config.SocketWriteTimeout)
	server.registerConnection(client)
	defer func() {
		client.closeNow()
		client.waitForWriter(server.config.SocketWriteTimeout + time.Second)
		server.unregisterConnection(client)
	}()
	conn.SetReadLimit(server.config.SocketReadLimit)

	events, err := server.runtime.bootstrapEvents(resolved)
	if err != nil {
		client.close(websocket.StatusInternalError, "bootstrap failed")
		return
	}
	if !server.writeEvents(client, events) {
		return
	}

	for {
		messageType, data, err := server.readMessage(conn)
		if err != nil {
			return
		}
		if messageType != websocket.MessageText {
			client.close(websocket.StatusUnsupportedData, "text messages only")
			return
		}
		request, requestErr := realtime.DecodeRequestEnvelope(data)
		response := server.runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), data)
		if response.HasError && isTerminalAuthError(response.Error.Error.Code) {
			_ = server.writeResponseAndClose(client, response, websocket.StatusPolicyViolation, "session invalid")
			return
		}
		if !server.writeResponse(client, response) {
			return
		}
		if response.HasError && response.Error.Error.Code == foundation.CodeShipDisabled {
			events, err := server.runtime.shipDisabledRefreshEvents(resolved.SessionID, resolved.PlayerID)
			if err != nil {
				client.close(websocket.StatusInternalError, "event publish failed")
				return
			}
			if !server.writeEvents(client, events) {
				return
			}
		}
		if requestErr != nil || response.HasError {
			continue
		}
		eventsBySession, err := server.runtime.postCommandEventsBySession(resolved.SessionID, request.Op, resolved.PlayerID)
		if err != nil {
			client.close(websocket.StatusInternalError, "event publish failed")
			return
		}
		for sessionID, events := range eventsBySession {
			server.writeEventsToSession(sessionID, events)
		}
	}
}

func newClientConnection(conn websocketWriter, sessionID auth.SessionID, queueSizeOverride ...int) *clientConnection {
	queueSize := outboundQueueSize
	if len(queueSizeOverride) > 0 && queueSizeOverride[0] > 0 {
		queueSize = queueSizeOverride[0]
	}
	return &clientConnection{
		conn:       conn,
		sessionID:  sessionID,
		outbound:   make(chan outboundMessage, queueSize),
		done:       make(chan struct{}),
		writerDone: make(chan struct{}),
	}
}

func (client *clientConnection) startWriter(writeTimeout time.Duration) {
	go client.writeLoop(writeTimeout)
}

func (client *clientConnection) writeLoop(writeTimeout time.Duration) {
	defer close(client.writerDone)
	for {
		select {
		case <-client.done:
			return
		case message := <-client.outbound:
			if !client.writeOutboundMessage(writeTimeout, message.payload) {
				client.close(websocket.StatusInternalError, "write failed")
				return
			}
			if message.closeAfter {
				client.close(message.closeStatus, message.closeReason)
				return
			}
		}
	}
}

func (client *clientConnection) writeOutboundMessage(writeTimeout time.Duration, payload []byte) bool {
	if client.conn == nil {
		return false
	}
	if writeTimeout <= 0 {
		writeTimeout = DefaultConfig().SocketWriteTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	if err := client.conn.Write(ctx, websocket.MessageText, payload); err != nil {
		return false
	}
	return true
}

func (client *clientConnection) enqueue(message outboundMessage) bool {
	select {
	case <-client.done:
		return false
	default:
	}
	select {
	case <-client.done:
		return false
	case client.outbound <- message:
		return true
	default:
		client.close(websocket.StatusPolicyViolation, "client too slow")
		return false
	}
}

func (client *clientConnection) close(status websocket.StatusCode, reason string) {
	client.closeOnce.Do(func() {
		close(client.done)
		if client.conn != nil {
			_ = client.conn.Close(status, reason)
		}
	})
}

func (client *clientConnection) closeNow() {
	client.closeOnce.Do(func() {
		close(client.done)
		if client.conn != nil {
			_ = client.conn.CloseNow()
		}
	})
}

func (client *clientConnection) waitForWriter(timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-client.writerDone:
		return true
	case <-timer.C:
		return false
	}
}

func (server *Server) readMessage(conn *websocket.Conn) (websocket.MessageType, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), server.config.SocketReadTimeout)
	defer cancel()
	return conn.Read(ctx)
}

func (server *Server) writeResponse(client *clientConnection, response realtime.CachedResponse) bool {
	payload, err := responsePayload(response)
	if err != nil {
		client.close(websocket.StatusInternalError, "response encode failed")
		return false
	}
	return server.writeText(client, payload)
}

func (server *Server) writeResponseAndClose(client *clientConnection, response realtime.CachedResponse, status websocket.StatusCode, reason string) bool {
	payload, err := responsePayload(response)
	if err != nil {
		client.close(websocket.StatusInternalError, "response encode failed")
		return false
	}
	if !server.writeTextMessage(client, outboundMessage{
		payload:     payload,
		closeAfter:  true,
		closeStatus: status,
		closeReason: reason,
	}) {
		return false
	}
	return client.waitForWriter(server.config.SocketWriteTimeout + time.Second)
}

func responsePayload(response realtime.CachedResponse) ([]byte, error) {
	var payload []byte
	var err error
	if response.HasError {
		payload, err = json.Marshal(response.Error)
	} else {
		payload, err = json.Marshal(response.Response)
	}
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (server *Server) writeEvents(client *clientConnection, events []realtime.EventEnvelope) bool {
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			client.close(websocket.StatusInternalError, "event encode failed")
			return false
		}
		if !server.writeText(client, payload) {
			return false
		}
	}
	return true
}

func (server *Server) writeText(client *clientConnection, payload []byte) bool {
	return server.writeTextMessage(client, outboundMessage{payload: append([]byte(nil), payload...)})
}

func (server *Server) writeTextMessage(client *clientConnection, message outboundMessage) bool {
	if len(message.payload) > 0 {
		message.payload = append([]byte(nil), message.payload...)
	}
	return client.enqueue(message)
}

func (server *Server) registerConnection(client *clientConnection) {
	server.connMu.Lock()
	server.sessionConnCounts[client.sessionID]++
	server.connMu.Unlock()
	server.conns.Store(client, struct{}{})
}

func (server *Server) unregisterConnection(client *clientConnection) {
	server.conns.Delete(client)
	server.connMu.Lock()
	count := server.sessionConnCounts[client.sessionID]
	if count <= 1 {
		delete(server.sessionConnCounts, client.sessionID)
		server.connMu.Unlock()
		server.runtime.detachSession(client.sessionID)
		return
	}
	server.sessionConnCounts[client.sessionID] = count - 1
	server.connMu.Unlock()
}

func (server *Server) writeEventsToSession(sessionID auth.SessionID, events []realtime.EventEnvelope) bool {
	if len(events) == 0 {
		return true
	}
	allWritten := true
	server.conns.Range(func(key, _ any) bool {
		client, isClient := key.(*clientConnection)
		if !isClient || client.sessionID != sessionID {
			return true
		}
		if !server.writeEvents(client, events) {
			allWritten = false
		}
		return true
	})
	return allWritten
}

func writeHTTPError(w http.ResponseWriter, err error) {
	var domainErr *foundation.DomainError
	if !errors.As(err, &domainErr) {
		domainErr = foundation.NewDomainError(foundation.CodeInternal, "Request failed.", foundation.WithCause(err))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatusForCode(domainErr.Code))
	_ = json.NewEncoder(w).Encode(struct {
		Error foundation.PublicError `json:"error"`
	}{Error: domainErr.Public()})
}

func httpStatusForCode(code foundation.Code) int {
	switch code {
	case foundation.CodeUnauthenticated, foundation.CodeAuthRequired, foundation.CodeSessionExpired, foundation.CodeSessionRevoked:
		return http.StatusUnauthorized
	case foundation.CodeForbidden, foundation.CodeOriginDenied:
		return http.StatusForbidden
	case foundation.CodeInvalidPayload:
		return http.StatusBadRequest
	case foundation.CodeNotFound:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func isTerminalAuthError(code foundation.Code) bool {
	switch code {
	case foundation.CodeAuthRequired, foundation.CodeSessionExpired, foundation.CodeSessionRevoked, foundation.CodeUnauthenticated:
		return true
	default:
		return false
	}
}
