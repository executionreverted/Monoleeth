package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world/aoi"
	"gameproject/internal/game/world/worker"
)

func (runtime *Runtime) ensurePlayerSession(resolved auth.ResolvedSession) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	state, ok := runtime.players[resolved.PlayerID]
	if !ok {
		runtime.nextPlayerEntity++
		entityID := foundation.EntityID(fmt.Sprintf("entity_pilot_%d", runtime.nextPlayerEntity))
		state = newPlayerRuntimeState(resolved.Callsign, entityID)
		runtime.players[resolved.PlayerID] = state
	} else if resolved.Callsign != "" && state.Callsign != resolved.Callsign {
		state.Callsign = resolved.Callsign
		runtime.players[resolved.PlayerID] = state
	}
	location, err := runtime.mapRouter.EnsureStarterLocation(resolved.PlayerID)
	if err != nil {
		return err
	}
	instance, err := runtime.mapInstanceForLocationLocked(location)
	if err != nil {
		return err
	}
	sessionIDs := runtime.sessionIDsForPlayerLocked(resolved.PlayerID, resolved.SessionID)
	for _, sessionID := range sessionIDs {
		runtime.detachSessionFromInactiveInstancesLocked(sessionID, location.InternalMapID)
	}
	runtime.removePlayerFromInactiveInstancesLocked(resolved.PlayerID, location.InternalMapID)
	if _, ok := instance.Worker.PlayerEntity(resolved.PlayerID); !ok {
		if err = instance.Worker.Submit(worker.SpawnPlayerCommand{
			PlayerID: resolved.PlayerID,
			EntityID: state.EntityID,
			Position: location.Position,
			Speed:    defaultPlayerSpeed,
		}); err != nil {
			return err
		}
		result := instance.Worker.Tick()
		runtime.recordEnemyTelemetryLocked(instance, result)
		err = commandErrors(result)
	}
	if err != nil {
		return err
	}
	for _, sessionID := range sessionIDs {
		if err = instance.Worker.Submit(worker.AttachSessionCommand{
			SessionID: realtime.SessionID(sessionID.String()),
			PlayerID:  resolved.PlayerID,
		}); err != nil {
			return err
		}
	}
	result := instance.Worker.Tick()
	runtime.recordEnemyTelemetryLocked(instance, result)
	err = commandErrors(result)
	if err != nil {
		return err
	}
	if err := runtime.ensurePlayerHangarLocked(resolved.PlayerID); err != nil {
		return err
	}
	if err := runtime.ensurePlayerEconomyLocked(resolved.PlayerID); err != nil {
		return err
	}
	if _, err := runtime.syncPlayerCombatActorLocked(resolved.PlayerID); err != nil {
		return err
	}
	for _, sessionID := range sessionIDs {
		runtime.attachSessionToInstanceLocked(instance, sessionID, resolved.PlayerID)
	}
	return nil
}

func (runtime *Runtime) sessionIDsForPlayerLocked(playerID foundation.PlayerID, include auth.SessionID) []auth.SessionID {
	seen := make(map[auth.SessionID]struct{}, len(runtime.sessions)+1)
	if include != "" {
		seen[include] = struct{}{}
	}
	for sessionID, sessionPlayerID := range runtime.sessions {
		if sessionPlayerID == playerID {
			seen[sessionID] = struct{}{}
		}
	}
	sessionIDs := make([]auth.SessionID, 0, len(seen))
	for sessionID := range seen {
		sessionIDs = append(sessionIDs, sessionID)
	}
	sort.Slice(sessionIDs, func(i, j int) bool {
		return sessionIDs[i] < sessionIDs[j]
	})
	return sessionIDs
}

func (runtime *Runtime) detachSession(sessionID auth.SessionID) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if mapID, ok := runtime.sessionLocations[sessionID]; ok {
		if instance, err := runtime.mapInstanceLocked(mapID); err == nil {
			runtime.detachSessionFromInstanceLocked(instance, sessionID, true)
		} else {
			runtime.detachSessionFromAllInstancesLocked(sessionID)
		}
	} else {
		runtime.detachSessionFromAllInstancesLocked(sessionID)
	}
	delete(runtime.sessions, sessionID)
	delete(runtime.sessionLocations, sessionID)
	delete(runtime.sessionEpochs, sessionID)
}

func (runtime *Runtime) bootstrapEvents(resolved auth.ResolvedSession) ([]realtime.EventEnvelope, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	state := runtime.players[resolved.PlayerID]
	if instance, _, err := runtime.activeMapInstanceLocked(resolved.PlayerID); err == nil {
		runtime.attachSessionToInstanceLocked(instance, resolved.SessionID, resolved.PlayerID)
	}
	worldSnapshot, err := runtime.worldSnapshotForSessionLocked(resolved.PlayerID, resolved.SessionID)
	if err != nil {
		return nil, err
	}
	progressionSnapshot, err := runtime.Progression.GetProgressionSnapshot(resolved.PlayerID)
	if err != nil {
		return nil, err
	}
	if instance, _, err := runtime.activeMapInstanceLocked(resolved.PlayerID); err == nil {
		instance.LastAOI[resolved.SessionID] = aoi.Snapshot{Entities: cloneAOIEntities(worldSnapshot.Entities)}
	}
	events := make([]realtime.EventEnvelope, 0, 8)
	sessionPayload := sessionReadyPayload{
		Authenticated: true,
		Account: &auth.PublicAccount{
			Email: resolved.Email.String(),
			Admin: hasRole(resolved.Roles, auth.RoleAdmin),
		},
		Player: &auth.PublicPlayer{
			Callsign: resolved.Callsign,
		},
		Roles:           roleStrings(resolved.Roles),
		ExpiresAt:       resolved.ExpiresAt.UTC().UnixMilli(),
		ProtocolVersion: realtime.CurrentVersion,
		ReconnectCursor: runtime.eventSeq[resolved.SessionID],
	}
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventSessionReady, sessionPayload))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventPlayerSnapshot, state.playerSnapshot()))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventShipSnapshot, state.Ship))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventStatsUpdated, state.Stats))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventWalletSnapshot, runtime.walletSnapshotLocked(resolved.PlayerID)))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventCargoSnapshot, state.Cargo))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventProgressionSnapshot, progressionPayload(progressionSnapshot)))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventWorldSnapshot, worldSnapshot))
	return events, nil
}

func (runtime *Runtime) postCommandEvents(sessionID auth.SessionID, op realtime.Operation, playerID foundation.PlayerID) ([]realtime.EventEnvelope, error) {
	eventsBySession, err := runtime.postCommandEventsBySession(sessionID, op, playerID)
	if err != nil {
		return nil, err
	}
	return eventsBySession[sessionID], nil
}

func (runtime *Runtime) postCommandEventsBySession(sessionID auth.SessionID, op realtime.Operation, playerID foundation.PlayerID) (map[auth.SessionID][]realtime.EventEnvelope, error) {
	switch op {
	case realtime.OperationMoveTo, realtime.OperationStop:
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		instance, _, err := runtime.activeMapInstanceLocked(playerID)
		if err != nil {
			return nil, err
		}
		entity, ok := instance.Worker.PlayerEntity(playerID)
		if !ok {
			return nil, worker.ErrUnknownPlayer
		}
		now := runtime.clock.Now()
		payload := map[string]any{
			"entity_id": entity.ID.String(),
			"position":  entity.Position,
		}
		if movement := runtime.publicMovementPayloadLocked(entity, now); movement != nil {
			payload["movement"] = movement
		}
		events := []realtime.EventEnvelope{runtime.eventAtLocked(sessionID, realtime.EventPositionCorrected, payload, now)}
		if op == realtime.OperationStop {
			events = append(events, runtime.eventAtLocked(sessionID, realtime.EventMovementStopped, payload, now))
		}
		events = append(events, runtime.aoiDiffEventsLocked(sessionID, playerID)...)
		return map[auth.SessionID][]realtime.EventEnvelope{sessionID: events}, nil
	case realtime.OperationCombatUseSkill,
		realtime.OperationLootPickup,
		realtime.OperationPortalEnter,
		realtime.OperationDeathRepairQuote,
		realtime.OperationDeathRepairShip,
		realtime.OperationHangarActivateShip,
		realtime.OperationLoadoutEquipModule,
		realtime.OperationLoadoutUnequipModule,
		realtime.OperationStealthToggle,
		realtime.OperationScanPulse,
		realtime.OperationDiscoveryClaimPlanet,
		realtime.OperationRouteCreate,
		realtime.OperationRouteEnable,
		realtime.OperationRouteDisable,
		realtime.OperationMarketCreateListing,
		realtime.OperationMarketBuy,
		realtime.OperationMarketCancel,
		realtime.OperationAuctionBid,
		realtime.OperationAuctionBuyNow,
		realtime.OperationAuctionGrants,
		realtime.OperationPremiumClaim,
		realtime.OperationPremiumWeeklyXCore,
		realtime.OperationQuestBoard,
		realtime.OperationQuestAccept,
		realtime.OperationQuestClaimReward,
		realtime.OperationQuestReroll,
		realtime.OperationAdminRepairCraftJob,
		realtime.OperationObservabilityMetric,
		realtime.OperationObservabilityGate:
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		eventsBySession := runtime.drainQueuedEventsBySessionLocked()
		actorEvents := eventsBySession[sessionID]
		if opEmitsPostCommandAOIDiff(op) {
			actorEvents = append(actorEvents, runtime.aoiDiffEventsLocked(sessionID, playerID)...)
		}
		if len(actorEvents) > 0 {
			if eventsBySession == nil {
				eventsBySession = make(map[auth.SessionID][]realtime.EventEnvelope, 1)
			}
			eventsBySession[sessionID] = actorEvents
		}
		return eventsBySession, nil
	default:
		return nil, nil
	}
}

func opEmitsPostCommandAOIDiff(op realtime.Operation) bool {
	switch op {
	case realtime.OperationMarketCreateListing,
		realtime.OperationPortalEnter,
		realtime.OperationDiscoveryClaimPlanet,
		realtime.OperationRouteEnable,
		realtime.OperationRouteDisable,
		realtime.OperationMarketBuy,
		realtime.OperationMarketCancel,
		realtime.OperationAuctionBid,
		realtime.OperationAuctionBuyNow,
		realtime.OperationPremiumClaim,
		realtime.OperationPremiumWeeklyXCore:
		return false
	default:
		return true
	}
}

func (runtime *Runtime) eventLocked(sessionID auth.SessionID, eventType realtime.ClientEventType, payload any) realtime.EventEnvelope {
	return runtime.eventAtLocked(sessionID, eventType, payload, runtime.clock.Now())
}

func (runtime *Runtime) eventAtLocked(sessionID auth.SessionID, eventType realtime.ClientEventType, payload any, at time.Time) realtime.EventEnvelope {
	runtime.eventSeq[sessionID]++
	if mapScopedEventType(eventType) {
		payload = runtime.payloadWithMapSubscriptionEpochLocked(sessionID, payload)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{}`)
	}
	return realtime.NewEventEnvelope(
		foundation.EventID(fmt.Sprintf("event_%d", runtime.eventSeq[sessionID])),
		eventType,
		data,
		at.UTC().UnixMilli(),
		runtime.eventSeq[sessionID],
	)
}

func (runtime *Runtime) payloadWithMapSubscriptionEpochLocked(sessionID auth.SessionID, payload any) any {
	epoch := runtime.sessionMapEpochLocked(sessionID)
	if epoch == 0 {
		return payload
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return payload
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		return payload
	}
	object["map_subscription_epoch"] = epoch
	return object
}

func mapScopedEventType(eventType realtime.ClientEventType) bool {
	switch eventType {
	case realtime.EventWorldSnapshot,
		realtime.EventMapTransferStarted,
		realtime.EventMapTransferCompleted,
		realtime.EventMapTransferFailed,
		realtime.EventPlayerProtection,
		realtime.EventAOIEntityEntered,
		realtime.EventAOIEntityUpdated,
		realtime.EventAOIEntityLeft,
		realtime.EventPositionCorrected,
		realtime.EventMovementStopped,
		realtime.EventTargetUpdated,
		realtime.EventCombatDamage,
		realtime.EventCombatMiss,
		realtime.EventCombatCooldownStarted,
		realtime.EventCombatNPCKilled,
		realtime.EventLootCreated,
		realtime.EventLootUpdated,
		realtime.EventLootRemoved,
		realtime.EventLootPickedUp,
		realtime.EventScanPulseStarted,
		realtime.EventScanPulseResolved,
		realtime.EventScanPlanetDiscovered,
		realtime.EventKnownPlanets,
		realtime.EventPlanetDetail,
		realtime.EventPlanetClaimed,
		realtime.EventProductionSummary,
		realtime.EventPlanetStorage,
		realtime.EventRouteUpdated,
		realtime.EventRouteList,
		realtime.EventRouteSnapshot:
		return true
	default:
		return false
	}
}

func (runtime *Runtime) filterEventsForActiveEpochLocked(sessionID auth.SessionID, events []realtime.EventEnvelope) []realtime.EventEnvelope {
	if len(events) == 0 {
		return nil
	}
	activeEpoch := runtime.sessionMapEpochLocked(sessionID)
	filtered := make([]realtime.EventEnvelope, 0, len(events))
	for _, event := range events {
		eventEpoch, ok := eventMapSubscriptionEpoch(event)
		if ok && transferLifecycleEvent(event.Type) {
			filtered = append(filtered, event)
			continue
		}
		if ok && activeEpoch != 0 && eventEpoch != activeEpoch {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func transferLifecycleEvent(eventType realtime.ClientEventType) bool {
	return eventType == realtime.EventMapTransferStarted ||
		eventType == realtime.EventMapTransferCompleted ||
		eventType == realtime.EventMapTransferFailed
}

func eventMapSubscriptionEpoch(event realtime.EventEnvelope) (uint64, bool) {
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return 0, false
	}
	raw, ok := payload["map_subscription_epoch"]
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case float64:
		if value <= 0 || math.Trunc(value) != value {
			return 0, false
		}
		return uint64(value), true
	case uint64:
		return value, true
	case int:
		if value <= 0 {
			return 0, false
		}
		return uint64(value), true
	default:
		return 0, false
	}
}

func commandErrors(result worker.TickResult) error {
	if len(result.CommandErrors) == 0 && len(result.ScheduledTaskErrors) == 0 {
		return nil
	}
	if len(result.CommandErrors) > 0 {
		return result.CommandErrors[0].Err
	}
	return result.ScheduledTaskErrors[0].Err
}

func hasRole(roles []auth.Role, want auth.Role) bool {
	for _, role := range roles {
		if role == want {
			return true
		}
	}
	return false
}

func roleStrings(roles []auth.Role) []string {
	if len(roles) == 0 {
		return nil
	}
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, string(role))
	}
	return out
}

type runtimeSessionResolver struct {
	runtime *Runtime
}

func (resolver runtimeSessionResolver) ResolveSession(sessionID realtime.SessionID) (realtime.CommandContext, error) {
	if resolver.runtime == nil || resolver.runtime.Auth == nil {
		return realtime.CommandContext{}, errors.New("nil runtime session resolver")
	}
	resolved, err := resolver.runtime.Auth.ResolveSessionID(context.Background(), auth.SessionID(sessionID.String()))
	if err != nil {
		return realtime.CommandContext{}, err
	}
	resolver.runtime.mu.Lock()
	location, err := resolver.runtime.mapRouter.ActiveLocation(resolved.PlayerID)
	resolver.runtime.mu.Unlock()
	if err != nil {
		return realtime.CommandContext{}, err
	}
	return realtime.CommandContext{
		SessionID: sessionID,
		PlayerID:  resolved.PlayerID,
		WorldID:   location.WorldID,
		ZoneID:    location.ZoneID,
	}, nil
}
