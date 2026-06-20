package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/coder/websocket"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

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
	client := &clientConnection{conn: conn, sessionID: resolved.SessionID}
	server.registerConnection(client)
	defer func() {
		server.unregisterConnection(client)
		_ = conn.CloseNow()
	}()
	conn.SetReadLimit(server.config.SocketReadLimit)

	events, err := server.runtime.bootstrapEvents(resolved)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "bootstrap failed")
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
			_ = conn.Close(websocket.StatusUnsupportedData, "text messages only")
			return
		}
		request, requestErr := realtime.DecodeRequestEnvelope(data)
		response := server.runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), data)
		if !server.writeResponse(client, response) {
			return
		}
		if response.HasError && isTerminalAuthError(response.Error.Error.Code) {
			_ = conn.Close(websocket.StatusPolicyViolation, "session invalid")
			return
		}
		if response.HasError && response.Error.Error.Code == foundation.CodeShipDisabled {
			events, err := server.runtime.shipDisabledRefreshEvents(resolved.SessionID, resolved.PlayerID)
			if err != nil {
				_ = conn.Close(websocket.StatusInternalError, "event publish failed")
				return
			}
			if !server.writeEvents(client, events) {
				return
			}
		}
		if requestErr != nil || response.HasError {
			continue
		}
		events, err := server.runtime.postCommandEvents(resolved.SessionID, request.Op, resolved.PlayerID)
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, "event publish failed")
			return
		}
		if !server.writeEvents(client, events) {
			return
		}
	}
}

func (server *Server) readMessage(conn *websocket.Conn) (websocket.MessageType, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), server.config.SocketReadTimeout)
	defer cancel()
	return conn.Read(ctx)
}

func (server *Server) writeResponse(client *clientConnection, response realtime.CachedResponse) bool {
	var payload []byte
	var err error
	if response.HasError {
		payload, err = json.Marshal(response.Error)
	} else {
		payload, err = json.Marshal(response.Response)
	}
	if err != nil {
		_ = client.conn.Close(websocket.StatusInternalError, "response encode failed")
		return false
	}
	return server.writeText(client, payload)
}

func (server *Server) writeEvents(client *clientConnection, events []realtime.EventEnvelope) bool {
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			_ = client.conn.Close(websocket.StatusInternalError, "event encode failed")
			return false
		}
		if !server.writeText(client, payload) {
			return false
		}
	}
	return true
}

func (server *Server) writeText(client *clientConnection, payload []byte) bool {
	ctx, cancel := context.WithTimeout(context.Background(), server.config.SocketWriteTimeout)
	defer cancel()
	client.mu.Lock()
	defer client.mu.Unlock()
	if err := client.conn.Write(ctx, websocket.MessageText, payload); err != nil {
		return false
	}
	return true
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

func (server *Server) writeEventsToSession(sessionID auth.SessionID, events []realtime.EventEnvelope) {
	if len(events) == 0 {
		return
	}
	server.conns.Range(func(key, _ any) bool {
		client, ok := key.(*clientConnection)
		if !ok || client.sessionID != sessionID {
			return true
		}
		if !server.writeEvents(client, events) {
			_ = client.conn.Close(websocket.StatusInternalError, "event publish failed")
		}
		return true
	})
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
