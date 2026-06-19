package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	"gameproject/internal/game/world/worker"
)

var trustedClientPayloadKeys = map[string]struct{}{
	"account_id":       {},
	"player_id":        {},
	"session_id":       {},
	"world_id":         {},
	"zone_id":          {},
	"speed":            {},
	"damage":           {},
	"xp":               {},
	"wallet_amount":    {},
	"cooldown":         {},
	"hit":              {},
	"crit":             {},
	"hidden":           {},
	"internal":         {},
	"gameplay_seed":    {},
	"future_spawn":     {},
	"spawn_candidates": {},
}

func (runtime *Runtime) commandHandlers() map[realtime.Operation]realtime.CommandHandler {
	return map[realtime.Operation]realtime.CommandHandler{
		realtime.OperationSessionSnapshot: runtime.handleSessionSnapshot,
		realtime.OperationWorldSnapshot:   runtime.handleWorldSnapshot,
		realtime.OperationMoveTo:          runtime.handleMoveTo,
		realtime.OperationStop:            runtime.handleStop,
		realtime.OperationDebugSnapshot:   runtime.handleDebugSnapshot,
		realtime.OperationDebugSpawnNPC:   runtime.handleDebugSpawnNPC,
	}
}

func (runtime *Runtime) handleSessionSnapshot(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	resolved, err := runtime.Auth.ResolveSessionID(context.Background(), authSessionID(ctx.SessionID))
	if err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	payload := sessionReadyPayload{
		Authenticated: true,
		Account: &auth.PublicAccount{
			Email: resolved.Email.String(),
			Admin: hasRole(resolved.Roles, auth.RoleAdmin),
		},
		Player:          &auth.PublicPlayer{Callsign: resolved.Callsign},
		Roles:           roleStrings(resolved.Roles),
		ExpiresAt:       resolved.ExpiresAt.UTC().UnixMilli(),
		ProtocolVersion: realtime.CurrentVersion,
		ReconnectCursor: runtime.eventSeq[resolved.SessionID],
	}
	return marshalPayload(payload)
}

func (runtime *Runtime) handleWorldSnapshot(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	payload, err := runtime.worldSnapshotLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	return marshalPayload(payload)
}

func (runtime *Runtime) handleMoveTo(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	intent, err := decodeMoveIntent(request.Payload)
	if err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.Worker.Submit(worker.MoveToCommand{PlayerID: ctx.PlayerID, Intent: intent}); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if err := commandErrors(runtime.Worker.Tick()); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	snapshot, err := runtime.worldSnapshotLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	return marshalPayload(struct {
		Accepted bool                `json:"accepted"`
		Entities []aoi.EntityPayload `json:"entities"`
	}{Accepted: true, Entities: snapshot.Entities})
}

func (runtime *Runtime) handleStop(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.Worker.Submit(worker.StopCommand{PlayerID: ctx.PlayerID}); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if err := commandErrors(runtime.Worker.Tick()); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	snapshot, err := runtime.worldSnapshotLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	return marshalPayload(struct {
		Accepted bool                `json:"accepted"`
		Entities []aoi.EntityPayload `json:"entities"`
	}{Accepted: true, Entities: snapshot.Entities})
}

func (runtime *Runtime) handleDebugSnapshot(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if !runtime.devMode {
		return nil, foundation.NewDomainError(foundation.CodeForbidden, "Debug command is disabled.")
	}
	return runtime.handleWorldSnapshot(ctx, request)
}

func (runtime *Runtime) handleDebugSpawnNPC(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if !runtime.devMode {
		return nil, foundation.NewDomainError(foundation.CodeForbidden, "Debug command is disabled.")
	}
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		EntityID string     `json:"entity_id"`
		Position world.Vec2 `json:"position"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	entityID, err := foundation.ParseEntityID(payload.EntityID)
	if err != nil {
		return nil, invalidPayload("Entity is invalid.", err)
	}
	entity, err := world.NewEntity(runtime.worldID, runtime.zoneID, entityID, world.EntityTypeNPCPlaceholder, payload.Position)
	if err != nil {
		return nil, invalidPayload("Entity is invalid.", err)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.Worker.InsertEntity(entity, 0); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	return marshalPayload(map[string]any{"accepted": true})
}

func decodeMoveIntent(payload json.RawMessage) (world.MovementIntent, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return world.MovementIntent{}, invalidPayload("Move target is invalid.", err)
	}
	if _, ok := raw["target"]; !ok {
		if _, hasX := raw["x"]; !hasX {
			return world.MovementIntent{}, invalidPayload("Move target is required.", nil)
		}
		if _, hasY := raw["y"]; !hasY {
			return world.MovementIntent{}, invalidPayload("Move target is required.", nil)
		}
	}
	var nested struct {
		Target world.Vec2 `json:"target"`
	}
	if _, ok := raw["target"]; ok {
		if err := decodeStrict(payload, &nested); err != nil {
			return world.MovementIntent{}, err
		}
		intent, err := world.NewMovementIntent(nested.Target)
		if err != nil {
			return world.MovementIntent{}, invalidPayload("Move target is invalid.", err)
		}
		return intent, nil
	}
	var direct struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := decodeStrict(payload, &direct); err != nil {
		return world.MovementIntent{}, invalidPayload("Move target is invalid.", err)
	}
	intent, err := world.NewMovementIntent(world.Vec2{X: direct.X, Y: direct.Y})
	if err != nil {
		return world.MovementIntent{}, invalidPayload("Move target is invalid.", err)
	}
	return intent, nil
}

func rejectTrustedPayload(payload json.RawMessage) error {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return invalidPayload("Invalid payload.", err)
	}
	if found := findTrustedPayloadKey(value); found != "" {
		return invalidPayload(fmt.Sprintf("Payload field %q is server-owned.", found), nil)
	}
	return nil
}

func findTrustedPayloadKey(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(key)
			if _, forbidden := trustedClientPayloadKeys[normalized]; forbidden {
				return key
			}
			if found := findTrustedPayloadKey(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findTrustedPayloadKey(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func decodeStrict(payload json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return invalidPayload("Invalid payload.", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return invalidPayload("Invalid payload.", err)
	}
	return nil
}

func marshalPayload(payload any) (json.RawMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func domainErrorForRuntime(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	if errors.Is(err, worker.ErrUnknownPlayer) || errors.Is(err, worker.ErrUnknownEntity) {
		return foundation.NewDomainError(foundation.CodeNotFound, "World object was not found.", foundation.WithCause(err))
	}
	if errors.Is(err, world.ErrInvalidPosition) {
		return invalidPayload("Position is invalid.", err)
	}
	return foundation.NewDomainError(foundation.CodeInternal, "Runtime command failed.", foundation.WithCause(err))
}

func invalidPayload(message string, cause error) *foundation.DomainError {
	opts := make([]foundation.DomainErrorOption, 0, 1)
	if cause != nil {
		opts = append(opts, foundation.WithCause(cause))
	}
	return foundation.NewDomainError(foundation.CodeInvalidPayload, message, opts...)
}

func authSessionID(sessionID realtime.SessionID) auth.SessionID {
	return auth.SessionID(sessionID.String())
}
