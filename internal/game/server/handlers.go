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
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

var trustedClientPayloadKeys = map[string]struct{}{
	"account_id":              {},
	"client_player_id":        {},
	"player_id":               {},
	"session_id":              {},
	"world_id":                {},
	"zone_id":                 {},
	"map_id":                  {},
	"internal_map_id":         {},
	"public_map_key":          {},
	"map_key":                 {},
	"map":                     {},
	"source_map_id":           {},
	"source_map_key":          {},
	"source_map":              {},
	"source_spawn_id":         {},
	"destination_map_id":      {},
	"destination_map_key":     {},
	"destination_map":         {},
	"destination_spawn_id":    {},
	"spawn_id":                {},
	"worker":                  {},
	"worker_id":               {},
	"map_worker_id":           {},
	"worker_topology":         {},
	"transfer_id":             {},
	"transfer_token":          {},
	"destination_worker":      {},
	"origin_worker":           {},
	"speed":                   {},
	"damage":                  {},
	"xp":                      {},
	"main_xp":                 {},
	"combat_xp":               {},
	"role_xp":                 {},
	"rank":                    {},
	"skill_points":            {},
	"loot":                    {},
	"wallet_amount":           {},
	"balance":                 {},
	"balance_after":           {},
	"total":                   {},
	"total_amount":            {},
	"server_total":            {},
	"price_total":             {},
	"fee":                     {},
	"fee_amount":              {},
	"server_fee":              {},
	"seller_proceeds":         {},
	"quest_progress":          {},
	"progress":                {},
	"progress_json":           {},
	"objective_progress":      {},
	"completed":               {},
	"completed_at":            {},
	"claimed_at":              {},
	"reward":                  {},
	"reward_payload":          {},
	"reward_claimed_at":       {},
	"generated_payload":       {},
	"generated_seed":          {},
	"rare_cap":                {},
	"reference_id":            {},
	"escrow":                  {},
	"escrow_location":         {},
	"source_return_location":  {},
	"seller_player_id":        {},
	"buyer_player_id":         {},
	"bidder_player_id":        {},
	"current_bid":             {},
	"current_bidder_id":       {},
	"winning_player_id":       {},
	"stock_total":             {},
	"stock_remaining":         {},
	"provider":                {},
	"provider_reference":      {},
	"entitlement_state":       {},
	"cooldown":                {},
	"radar_range":             {},
	"detection_power":         {},
	"jammer_resistance":       {},
	"stealth_detection_bonus": {},
	"scan_power":              {},
	"scan_radius":             {},
	"signature":               {},
	"entity_signature":        {},
	"stealth_score":           {},
	"jammer_strength":         {},
	"detection_score":         {},
	"detection_threshold":     {},
	"hit":                     {},
	"crit":                    {},
	"hidden":                  {},
	"internal":                {},
	"internal_metadata":       {},
	"gameplay_seed":           {},
	"procedural_seed":         {},
	"world_seed":              {},
	"future_spawn":            {},
	"future_spawn_data":       {},
	"spawn_candidates":        {},
	"candidate":               {},
	"candidate_key":           {},
	"planet_candidate":        {},
	"detection_roll":          {},
	"scan_roll":               {},
	"scan_cell":               {},
	"scan_result":             {},
	"scan_candidate":          {},
	"scan_candidates":         {},
	"candidate_data":          {},
	"target_player_id":        {},
	"witness_expires_at":      {},
	"witness_expiry":          {},
	"hidden_target_metadata":  {},
	"loot_roll":               {},
	"loot_table":              {},
	"password":                {},
	"password_hash":           {},
	"token":                   {},
	"session_token":           {},
	"reset_secret":            {},
	"auth_header":             {},
	"cookie":                  {},
}

func (runtime *Runtime) commandHandlers() map[realtime.Operation]realtime.CommandHandler {
	return map[realtime.Operation]realtime.CommandHandler{
		realtime.OperationSessionSnapshot:      runtime.handleSessionSnapshot,
		realtime.OperationWorldSnapshot:        runtime.handleWorldSnapshot,
		realtime.OperationMoveTo:               runtime.handleMoveTo,
		realtime.OperationStop:                 runtime.handleStop,
		realtime.OperationPortalEnter:          runtime.handlePortalEnter,
		realtime.OperationCombatUseSkill:       runtime.handleCombatUseSkill,
		realtime.OperationLootPickup:           runtime.handleLootPickup,
		realtime.OperationDeathRepairQuote:     runtime.handleDeathRepairQuote,
		realtime.OperationDeathRepairShip:      runtime.handleDeathRepairShip,
		realtime.OperationProgressionSnapshot:  runtime.handleProgressionSnapshot,
		realtime.OperationInventorySnapshot:    runtime.handleInventorySnapshot,
		realtime.OperationHangarSnapshot:       runtime.handleHangarSnapshot,
		realtime.OperationHangarActivateShip:   runtime.handleHangarActivateShip,
		realtime.OperationLoadoutSnapshot:      runtime.handleLoadoutSnapshot,
		realtime.OperationLoadoutEquipModule:   runtime.handleLoadoutEquipModule,
		realtime.OperationLoadoutUnequipModule: runtime.handleLoadoutUnequipModule,
		realtime.OperationStatsSnapshot:        runtime.handleStatsSnapshot,
		realtime.OperationStealthToggle:        runtime.handleStealthToggle,
		realtime.OperationCraftingRecipes:      runtime.handleCraftingRecipes,
		realtime.OperationScanPulse:            runtime.handleScanPulse,
		realtime.OperationKnownPlanets:         runtime.handleKnownPlanets,
		realtime.OperationPlanetDetail:         runtime.handlePlanetDetail,
		realtime.OperationProductionSummary:    runtime.handleProductionSummary,
		realtime.OperationPlanetStorage:        runtime.handlePlanetStorage,
		realtime.OperationRouteList:            runtime.handleRouteList,
		realtime.OperationRouteSnapshot:        runtime.handleRouteSnapshot,
		realtime.OperationWalletSnapshot:       runtime.handleWalletSnapshot,
		realtime.OperationShopCatalog:          runtime.handleShopCatalog,
		realtime.OperationShopBuyProduct:       runtime.handleShopBuyProduct,
		realtime.OperationMarketSearch:         runtime.handleMarketSearch,
		realtime.OperationMarketCreateListing:  runtime.handleMarketCreateListing,
		realtime.OperationMarketBuy:            runtime.handleMarketBuy,
		realtime.OperationMarketCancel:         runtime.handleMarketCancel,
		realtime.OperationAuctionSearch:        runtime.handleAuctionSearch,
		realtime.OperationAuctionBid:           runtime.handleAuctionBid,
		realtime.OperationAuctionBuyNow:        runtime.handleAuctionBuyNow,
		realtime.OperationAuctionGrants:        runtime.handleAuctionGrants,
		realtime.OperationPremiumEntitlements:  runtime.handlePremiumEntitlements,
		realtime.OperationPremiumClaim:         runtime.handlePremiumClaim,
		realtime.OperationPremiumWeeklyXCore:   runtime.handlePremiumWeeklyXCore,
		realtime.OperationQuestBoard:           runtime.handleQuestBoard,
		realtime.OperationQuestAccept:          runtime.handleQuestAccept,
		realtime.OperationQuestProgress:        runtime.handleQuestProgress,
		realtime.OperationQuestClaimReward:     runtime.handleQuestClaimReward,
		realtime.OperationQuestReroll:          runtime.handleQuestReroll,
		realtime.OperationAdminInspectPlayer:   runtime.handleAdminInspectPlayer,
		realtime.OperationAdminRepairCraftJob:  runtime.handleAdminRepairCraftJob,
		realtime.OperationAdminEconomyDash:     runtime.handleAdminEconomyDashboard,
		realtime.OperationObservabilityLog:     runtime.handleObservabilityCommandLog,
		realtime.OperationObservabilityMetric:  runtime.handleObservabilityMetrics,
		realtime.OperationObservabilityGate:    runtime.handleObservabilityReleaseGate,
		realtime.OperationObservabilityAbuse:   runtime.handleObservabilityAbuseCoverage,
		realtime.OperationDebugSnapshot:        runtime.handleDebugSnapshot,
		realtime.OperationDebugSpawnNPC:        runtime.handleDebugSpawnNPC,
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
	sessionID := authSessionID(ctx.SessionID)
	instance, _, instanceErr := runtime.activeMapInstanceLocked(ctx.PlayerID)
	if instanceErr == nil {
		runtime.attachSessionToInstanceLocked(instance, sessionID, ctx.PlayerID)
	}
	payload, err := runtime.worldSnapshotForSessionLocked(ctx.PlayerID, sessionID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if instanceErr == nil {
		instance.LastAOI[sessionID] = aoi.Snapshot{Entities: cloneAOIEntities(payload.Entities)}
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
	if err := runtime.validateMoveIntentLocked(ctx.PlayerID, intent); err != nil {
		return nil, err
	}
	instance, _, err := runtime.activeMapInstanceLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if err := instance.Worker.Submit(worker.MoveToCommand{PlayerID: ctx.PlayerID, Intent: intent}); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if err := commandErrors(instance.Worker.Tick()); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	snapshot, err := runtime.worldSnapshotForSessionLocked(ctx.PlayerID, authSessionID(ctx.SessionID))
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	return marshalPayload(struct {
		Accepted     bool                          `json:"accepted"`
		PublicMapKey string                        `json:"public_map_key"`
		Map          worldmaps.ClientMapProjection `json:"map"`
		Entities     []aoi.EntityPayload           `json:"entities"`
		Minimap      minimapPayload                `json:"minimap"`
		Epoch        uint64                        `json:"map_subscription_epoch"`
	}{
		Accepted:     true,
		PublicMapKey: snapshot.Map.PublicMapKey,
		Map:          snapshot.Map,
		Entities:     snapshot.Entities,
		Minimap:      snapshot.Minimap,
		Epoch:        snapshot.MapSubscriptionEpoch,
	})
}

func (runtime *Runtime) handleStop(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.validateShipCanMoveLocked(ctx.PlayerID); err != nil {
		return nil, err
	}
	instance, _, err := runtime.activeMapInstanceLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if err := instance.Worker.Submit(worker.StopCommand{PlayerID: ctx.PlayerID}); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if err := commandErrors(instance.Worker.Tick()); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	snapshot, err := runtime.worldSnapshotForSessionLocked(ctx.PlayerID, authSessionID(ctx.SessionID))
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	return marshalPayload(struct {
		Accepted bool                `json:"accepted"`
		Entities []aoi.EntityPayload `json:"entities"`
		Minimap  minimapPayload      `json:"minimap"`
		Epoch    uint64              `json:"map_subscription_epoch"`
	}{Accepted: true, Entities: snapshot.Entities, Minimap: snapshot.Minimap, Epoch: snapshot.MapSubscriptionEpoch})
}

func (runtime *Runtime) validateMoveIntentLocked(playerID foundation.PlayerID, intent world.MovementIntent) error {
	if err := runtime.validateShipCanMoveLocked(playerID); err != nil {
		return err
	}
	if err := runtime.mapRouter.ValidateActivePosition(playerID, intent.Target); err != nil {
		if errors.Is(err, worldmaps.ErrPositionOutOfBounds) {
			return foundation.NewDomainError(foundation.CodeOutOfRange, "Move target is outside map bounds.", foundation.WithCause(err))
		}
		return err
	}
	instance, _, err := runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return err
	}
	entity, ok := instance.Worker.PlayerEntity(playerID)
	if !ok {
		return domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	if entity.Position.Distance(intent.Target) > defaultMaxMoveDistance {
		return foundation.NewDomainError(foundation.CodeOutOfRange, "Move target is out of range.")
	}
	now := runtime.clock.Now()
	if lastMove := runtime.lastMove[playerID]; !lastMove.IsZero() && now.Sub(lastMove) < minMoveCommandInterval {
		return foundation.NewDomainError(foundation.CodeRateLimited, "Movement intent rate limit exceeded.")
	}
	runtime.lastMove[playerID] = now
	return nil
}

func (runtime *Runtime) validateShipCanMoveLocked(playerID foundation.PlayerID) error {
	if err := runtime.validateNoActiveTransferLocked(playerID); err != nil {
		return err
	}
	state, ok := runtime.players[playerID]
	if !ok {
		return domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	if state.Ship.Disabled || state.Ship.RepairState == "disabled" {
		return foundation.NewDomainError(foundation.CodeShipDisabled, "Ship is disabled.")
	}
	return nil
}

func (runtime *Runtime) validateNoActiveTransferLocked(playerID foundation.PlayerID) error {
	if _, active := runtime.activeTransfers[playerID]; active {
		return foundation.NewDomainError(foundation.CodeForbidden, "Map transfer is active.", foundation.WithCause(errTransferActive))
	}
	return nil
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
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.mapRouter.ValidateActivePosition(ctx.PlayerID, payload.Position); err != nil {
		if errors.Is(err, worldmaps.ErrPositionOutOfBounds) {
			return nil, foundation.NewDomainError(foundation.CodeOutOfRange, "Debug spawn position is outside map bounds.", foundation.WithCause(err))
		}
		return nil, domainErrorForRuntime(err)
	}
	instance, _, err := runtime.activeMapInstanceLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if err := instance.Worker.Submit(worker.DebugSpawnNPCCommand{EntityID: entityID, Position: payload.Position}); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	if err := commandErrors(instance.Worker.Tick()); err != nil {
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
	return rejectTrustedPayloadAllowing(payload)
}

func rejectTrustedPayloadWithAdditional(payload json.RawMessage, additionalKeys ...string) error {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return invalidPayload("Invalid payload.", err)
	}
	if found := findTrustedPayloadKey(value, nil, 0); found != "" {
		return invalidPayload(fmt.Sprintf("Payload field %q is server-owned.", found), nil)
	}
	additional := make(map[string]struct{}, len(additionalKeys))
	for _, key := range additionalKeys {
		additional[strings.ToLower(key)] = struct{}{}
	}
	if found := findPayloadKey(value, additional); found != "" {
		return invalidPayload(fmt.Sprintf("Payload field %q is server-owned.", found), nil)
	}
	return nil
}

func rejectEmptyIntentPayload(payload json.RawMessage, additionalTrustedKeys ...string) error {
	if err := rejectTrustedPayloadWithAdditional(payload, additionalTrustedKeys...); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return invalidPayload("Invalid payload.", err)
	}
	if fields == nil {
		return invalidPayload("Payload must be an empty object.", nil)
	}
	for key := range fields {
		return invalidPayload(fmt.Sprintf("Payload field %q is not accepted for this command.", key), nil)
	}
	return nil
}

func rejectTrustedPayloadAllowing(payload json.RawMessage, allowedKeys ...string) error {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return invalidPayload("Invalid payload.", err)
	}
	allowed := make(map[string]struct{}, len(allowedKeys))
	for _, key := range allowedKeys {
		allowed[strings.ToLower(key)] = struct{}{}
	}
	if found := findTrustedPayloadKey(value, allowed, 0); found != "" {
		return invalidPayload(fmt.Sprintf("Payload field %q is server-owned.", found), nil)
	}
	return nil
}

func findTrustedPayloadKey(value any, allowed map[string]struct{}, depth int) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(key)
			if _, forbidden := trustedClientPayloadKeys[normalized]; forbidden {
				if _, ok := allowed[normalized]; !ok || depth != 0 {
					return key
				}
			}
			if found := findTrustedPayloadKey(child, allowed, depth+1); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findTrustedPayloadKey(child, allowed, depth+1); found != "" {
				return found
			}
		}
	}
	return ""
}

func findPayloadKey(value any, keys map[string]struct{}) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if _, forbidden := keys[strings.ToLower(key)]; forbidden {
				return key
			}
			if found := findPayloadKey(child, keys); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findPayloadKey(child, keys); found != "" {
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
