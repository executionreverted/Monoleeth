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
	server.conns.Store(conn, struct{}{})
	defer func() {
		server.conns.Delete(conn)
		server.runtime.detachSession(resolved.SessionID)
		_ = conn.CloseNow()
	}()
	conn.SetReadLimit(server.config.SocketReadLimit)

	events, err := server.runtime.bootstrapEvents(resolved)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "bootstrap failed")
		return
	}
	if !server.writeEvents(conn, events) {
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
		if !server.writeResponse(conn, response) {
			return
		}
		if response.HasError && isTerminalAuthError(response.Error.Error.Code) {
			_ = conn.Close(websocket.StatusPolicyViolation, "session invalid")
			return
		}
		if requestErr != nil || response.HasError {
			continue
		}
		events, err := server.runtime.postCommandEvents(resolved.SessionID, request.Op, resolved.PlayerID)
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, "event publish failed")
			return
		}
		if !server.writeEvents(conn, events) {
			return
		}
	}
}

func (server *Server) readMessage(conn *websocket.Conn) (websocket.MessageType, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), server.config.SocketReadTimeout)
	defer cancel()
	return conn.Read(ctx)
}

func (server *Server) writeResponse(conn *websocket.Conn, response realtime.CachedResponse) bool {
	var payload []byte
	var err error
	if response.HasError {
		payload, err = json.Marshal(response.Error)
	} else {
		payload, err = json.Marshal(response.Response)
	}
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "response encode failed")
		return false
	}
	return server.writeText(conn, payload)
}

func (server *Server) writeEvents(conn *websocket.Conn, events []realtime.EventEnvelope) bool {
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, "event encode failed")
			return false
		}
		if !server.writeText(conn, payload) {
			return false
		}
	}
	return true
}

func (server *Server) writeText(conn *websocket.Conn, payload []byte) bool {
	ctx, cancel := context.WithTimeout(context.Background(), server.config.SocketWriteTimeout)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		return false
	}
	return true
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
