package server

import (
	"encoding/json"
	"errors"
	"fmt"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

type portalCooldownKey struct {
	PlayerID    foundation.PlayerID
	SourceMapID worldmaps.MapID
	PortalID    worldmaps.PortalID
}

type portalRequestKey struct {
	PlayerID  foundation.PlayerID
	RequestID foundation.RequestID
}

type portalTransferState struct {
	PlayerID    foundation.PlayerID
	SourceMapID worldmaps.MapID
	PortalID    worldmaps.PortalID
	RequestID   foundation.RequestID
}

type portalTransferRecord struct {
	Payload json.RawMessage
}

type portalTransferInterleaveStage string

const (
	portalTransferAfterDestinationPlayerAttach  portalTransferInterleaveStage = "after_destination_player_attach"
	portalTransferAfterDestinationSessionAttach portalTransferInterleaveStage = "after_destination_session_attach"
)

type portalTransferInterleaveContext struct {
	PlayerID         foundation.PlayerID
	DestinationMapID worldmaps.MapID
	SessionIDs       []auth.SessionID
}

// portalTransferInterleaveTestHook is nil in production. Same-package tests
// install it to force transfer rollback failures at deterministic boundaries.
var portalTransferInterleaveTestHook func(portalTransferInterleaveStage, *Runtime, portalTransferInterleaveContext) error

func (runtime *Runtime) handlePortalEnter(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	intent, err := decodePortalEnterIntent(request.Payload)
	if err != nil {
		return nil, err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	requestKey := portalRequestKey{PlayerID: ctx.PlayerID, RequestID: request.RequestID}
	if previous, ok := runtime.portalAttempts[requestKey]; ok {
		return append(json.RawMessage(nil), previous.Payload...), nil
	}
	if err := runtime.validateNoActiveTransferLocked(ctx.PlayerID); err != nil {
		return nil, err
	}
	if err := runtime.validateNoActiveScanPulseLocked(ctx.PlayerID); err != nil {
		return nil, err
	}

	payload, err := runtime.transferThroughPortalLocked(authSessionID(ctx.SessionID), ctx.PlayerID, request.RequestID, intent.PortalID)
	if err != nil {
		return nil, err
	}
	runtime.portalAttempts[requestKey] = portalTransferRecord{Payload: append(json.RawMessage(nil), payload...)}
	return payload, nil
}

func (runtime *Runtime) validateNoActiveScanPulseLocked(playerID foundation.PlayerID) error {
	if _, active := runtime.activeScanPulses[playerID]; active {
		return foundation.NewDomainError(foundation.CodeForbidden, "Scan pulse is active.", foundation.WithCause(errScanPulseActive))
	}
	return nil
}

func decodePortalEnterIntent(payload json.RawMessage) (portalEnterIntent, error) {
	var intent portalEnterIntent
	if err := decodeStrict(payload, &intent); err != nil {
		return portalEnterIntent{}, err
	}
	if err := intent.PortalID.Validate(); err != nil {
		return portalEnterIntent{}, invalidPayload("portal_id is required.", err)
	}
	return intent, nil
}

func (runtime *Runtime) transferThroughPortalLocked(sessionID auth.SessionID, playerID foundation.PlayerID, requestID foundation.RequestID, portalID worldmaps.PortalID) (json.RawMessage, error) {
	source, sourceLocation, err := runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	portal, ok := runtime.mapCatalog.Portal(sourceLocation.InternalMapID, portalID)
	if !ok || !portal.Visible {
		return nil, foundation.NewDomainError(foundation.CodeNotVisible, "Portal is not visible.")
	}
	destinationDefinition, ok := runtime.mapCatalog.Get(portal.DestinationMapID)
	if !ok {
		return nil, foundation.NewDomainError(foundation.CodeForbidden, "Portal destination is unavailable.", foundation.WithCause(worldmaps.ErrMapNotFound))
	}
	destinationSpawn, ok := runtime.mapCatalog.Spawn(portal.DestinationMapID, portal.DestinationSpawnID)
	if !ok {
		return nil, foundation.NewDomainError(foundation.CodeForbidden, "Portal destination is unavailable.", foundation.WithCause(worldmaps.ErrSpawnNotFound))
	}
	if err := runtime.mapCatalog.ValidatePosition(portal.DestinationMapID, destinationSpawn.Position); err != nil {
		return nil, foundation.NewDomainError(foundation.CodeForbidden, "Portal destination is unavailable.", foundation.WithCause(err))
	}
	destination, err := runtime.mapInstanceLocked(portal.DestinationMapID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	playerEntity, ok := source.Worker.PlayerEntity(playerID)
	if !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	if playerEntity.Position.Distance(portal.SourcePosition) > portal.InteractionRadius {
		return nil, foundation.NewDomainError(foundation.CodeOutOfRange, "Portal is out of range.")
	}
	now := runtime.clock.Now()
	cooldownKey := portalCooldownKey{
		PlayerID:    playerID,
		SourceMapID: sourceLocation.InternalMapID,
		PortalID:    portalID,
	}
	if readyAt := runtime.portalCooldowns[cooldownKey]; readyAt.After(now) {
		return nil, foundation.NewDomainError(foundation.CodeCooldown, "Portal is on cooldown.")
	}

	sessionIDs := runtime.sessionIDsForPlayerLocked(playerID, sessionID)
	startedPayload := mapTransferStartedPayload{
		PortalID:             portal.PortalID.String(),
		FromPublicMapKey:     source.Definition.PublicMapKey.String(),
		ToPublicMapKey:       destinationDefinition.PublicMapKey.String(),
		MapSubscriptionEpoch: runtime.sessionMapEpochLocked(sessionID),
	}
	for _, playerSessionID := range sessionIDs {
		runtime.queueEventLocked(playerSessionID, realtime.EventMapTransferStarted, startedPayload)
	}

	runtime.activeTransfers[playerID] = portalTransferState{
		PlayerID:    playerID,
		SourceMapID: sourceLocation.InternalMapID,
		PortalID:    portalID,
		RequestID:   requestID,
	}
	defer delete(runtime.activeTransfers, playerID)

	sourceEntity := playerEntity
	sourceSpeed, _ := source.Worker.EntitySpeed(sourceEntity.ID)
	for _, playerSessionID := range sessionIDs {
		runtime.detachSessionFromInstanceLocked(source, playerSessionID, true)
	}
	source.Worker.RemoveEntity(sourceEntity.ID)
	delete(source.HiddenPlayers, playerID)
	runtime.deleteHiddenPlayerWitnessesLocked(source, playerID)

	location, err := runtime.mapRouter.SetActiveLocationFromSpawn(playerID, portal.DestinationMapID, portal.DestinationSpawnID)
	if err != nil {
		runtime.restoreSourceAfterFailedTransferLocked(source, sourceLocation, sourceEntity, sourceSpeed, sessionIDs, playerID)
		runtime.queueTransferFailedLocked(sessionIDs, portal, source, "Portal transfer failed.")
		return nil, domainErrorForRuntime(err)
	}
	if location.InternalMapID != destination.Definition.InternalMapID {
		runtime.restoreSourceAfterFailedTransferLocked(source, sourceLocation, sourceEntity, sourceSpeed, sessionIDs, playerID)
		runtime.queueTransferFailedLocked(sessionIDs, portal, source, "Portal transfer failed.")
		return nil, domainErrorForRuntime(fmt.Errorf("destination map mismatch: %w", errMapInstanceNotFound))
	}

	if err := runtime.attachPlayerToDestinationLocked(destination, playerID, sourceEntity.ID, destinationSpawn.Position); err != nil {
		runtime.cleanupDestinationAfterFailedTransferLocked(destination, sessionIDs, playerID, sourceEntity.ID)
		runtime.restoreSourceAfterFailedTransferLocked(source, sourceLocation, sourceEntity, sourceSpeed, sessionIDs, playerID)
		runtime.queueTransferFailedLocked(sessionIDs, portal, source, "Portal transfer failed.")
		return nil, domainErrorForRuntime(err)
	}
	if err := notifyPortalTransferInterleaveTestHook(portalTransferAfterDestinationPlayerAttach, runtime, portalTransferInterleaveContext{
		PlayerID:         playerID,
		DestinationMapID: destination.Definition.InternalMapID,
		SessionIDs:       append([]auth.SessionID(nil), sessionIDs...),
	}); err != nil {
		runtime.cleanupDestinationAfterFailedTransferLocked(destination, sessionIDs, playerID, sourceEntity.ID)
		runtime.restoreSourceAfterFailedTransferLocked(source, sourceLocation, sourceEntity, sourceSpeed, sessionIDs, playerID)
		runtime.queueTransferFailedLocked(sessionIDs, portal, source, "Portal transfer failed.")
		return nil, domainErrorForRuntime(err)
	}
	for _, playerSessionID := range sessionIDs {
		if err := destination.Worker.Submit(worker.AttachSessionCommand{
			SessionID: realtime.SessionID(playerSessionID.String()),
			PlayerID:  playerID,
		}); err != nil {
			runtime.cleanupDestinationAfterFailedTransferLocked(destination, sessionIDs, playerID, sourceEntity.ID)
			runtime.restoreSourceAfterFailedTransferLocked(source, sourceLocation, sourceEntity, sourceSpeed, sessionIDs, playerID)
			runtime.queueTransferFailedLocked(sessionIDs, portal, source, "Portal transfer failed.")
			return nil, domainErrorForRuntime(err)
		}
	}
	result := destination.Worker.Tick()
	runtime.recordEnemyTelemetryLocked(destination, result)
	if err := commandErrors(result); err != nil {
		runtime.cleanupDestinationAfterFailedTransferLocked(destination, sessionIDs, playerID, sourceEntity.ID)
		runtime.restoreSourceAfterFailedTransferLocked(source, sourceLocation, sourceEntity, sourceSpeed, sessionIDs, playerID)
		runtime.queueTransferFailedLocked(sessionIDs, portal, source, "Portal transfer failed.")
		return nil, domainErrorForRuntime(err)
	}
	if err := notifyPortalTransferInterleaveTestHook(portalTransferAfterDestinationSessionAttach, runtime, portalTransferInterleaveContext{
		PlayerID:         playerID,
		DestinationMapID: destination.Definition.InternalMapID,
		SessionIDs:       append([]auth.SessionID(nil), sessionIDs...),
	}); err != nil {
		runtime.cleanupDestinationAfterFailedTransferLocked(destination, sessionIDs, playerID, sourceEntity.ID)
		runtime.restoreSourceAfterFailedTransferLocked(source, sourceLocation, sourceEntity, sourceSpeed, sessionIDs, playerID)
		runtime.queueTransferFailedLocked(sessionIDs, portal, source, "Portal transfer failed.")
		return nil, domainErrorForRuntime(err)
	}

	runtime.removePlayerFromInactiveInstancesLocked(playerID, destination.Definition.InternalMapID)
	for _, playerSessionID := range sessionIDs {
		runtime.attachSessionToInstanceLocked(destination, playerSessionID, playerID)
	}
	runtime.portalCooldowns[cooldownKey] = now.Add(runtimePortalCooldown)
	protection, err := runtime.startPortalProtectionLocked(playerID, destination.Definition.InternalMapID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	runtime.queueProtectionUpdatedLocked(sessionIDs, protection)

	snapshotsBySession := make(map[auth.SessionID]worldSnapshotPayload, len(sessionIDs))
	for _, playerSessionID := range sessionIDs {
		snapshot, err := runtime.worldSnapshotForSessionLocked(playerID, playerSessionID)
		if err != nil {
			return nil, domainErrorForRuntime(err)
		}
		destination.LastAOI[playerSessionID] = aoi.Snapshot{Entities: cloneAOIEntities(snapshot.Entities)}
		snapshotsBySession[playerSessionID] = snapshot
		runtime.queueEventLocked(playerSessionID, realtime.EventMapTransferCompleted, mapTransferCompletedPayload{
			PortalID:             portal.PortalID.String(),
			FromPublicMapKey:     source.Definition.PublicMapKey.String(),
			ToPublicMapKey:       destination.Definition.PublicMapKey.String(),
			Position:             destinationSpawn.Position,
			MapSubscriptionEpoch: runtime.sessionMapEpochLocked(playerSessionID),
			Snapshot:             snapshot,
		})
	}

	responseSnapshot := snapshotsBySession[sessionID]
	return marshalPayload(map[string]any{
		"accepted":               true,
		"portal_id":              portal.PortalID.String(),
		"from_public_map_key":    source.Definition.PublicMapKey.String(),
		"to_public_map_key":      destination.Definition.PublicMapKey.String(),
		"position":               destinationSpawn.Position,
		"map_subscription_epoch": runtime.sessionMapEpochLocked(sessionID),
		"snapshot":               responseSnapshot,
	})
}

func (runtime *Runtime) attachPlayerToDestinationLocked(instance *mapInstance, playerID foundation.PlayerID, entityID world.EntityID, position world.Vec2) error {
	if entity, ok := instance.Worker.PlayerEntity(playerID); ok {
		entity.Position = position
		entity.Movement = world.MovementState{}
		return instance.Worker.UpdateEntity(entity)
	}
	return runtime.submitWorkerCommandAndRecordMetricsLocked(instance, worker.SpawnPlayerCommand{
		PlayerID: playerID,
		EntityID: entityID,
		Position: position,
		Speed:    defaultPlayerSpeed,
	})
}

func notifyPortalTransferInterleaveTestHook(stage portalTransferInterleaveStage, runtime *Runtime, context portalTransferInterleaveContext) error {
	if portalTransferInterleaveTestHook == nil {
		return nil
	}
	return portalTransferInterleaveTestHook(stage, runtime, context)
}

func (runtime *Runtime) cleanupDestinationAfterFailedTransferLocked(destination *mapInstance, sessionIDs []auth.SessionID, playerID foundation.PlayerID, entityID world.EntityID) {
	if destination == nil || destination.Worker == nil {
		return
	}
	for _, sessionID := range sessionIDs {
		_ = destination.Worker.Submit(worker.DetachSessionCommand{SessionID: realtime.SessionID(sessionID.String())})
	}
	result := destination.Worker.Tick()
	runtime.recordEnemyTelemetryLocked(destination, result)
	_ = commandErrors(result)
	destination.Worker.RemoveEntity(entityID)
	delete(destination.HiddenPlayers, playerID)
	runtime.deleteHiddenPlayerWitnessesLocked(destination, playerID)
	for _, sessionID := range sessionIDs {
		delete(destination.ActiveSessions, sessionID)
		delete(destination.LastAOI, sessionID)
		if runtime.sessionLocations[sessionID] == destination.Definition.InternalMapID {
			delete(runtime.sessionLocations, sessionID)
		}
	}
}

func (runtime *Runtime) restoreSourceAfterFailedTransferLocked(source *mapInstance, sourceLocation worldmaps.PlayerMapLocation, sourceEntity world.Entity, sourceSpeed float64, sessionIDs []auth.SessionID, playerID foundation.PlayerID) {
	_, _ = runtime.mapRouter.SetActiveLocationFromSpawn(playerID, sourceLocation.InternalMapID, sourceLocation.SpawnID)
	if _, ok := source.Worker.PlayerEntity(playerID); !ok {
		if _, exists := source.Worker.Entity(sourceEntity.ID); exists {
			source.Worker.RemoveEntity(sourceEntity.ID)
		}
		if err := runtime.submitWorkerCommandAndRecordMetricsLocked(source, worker.SpawnPlayerCommand{
			PlayerID: playerID,
			EntityID: sourceEntity.ID,
			Position: sourceEntity.Position,
			Speed:    sourceSpeed,
		}); err == nil {
			_ = source.Worker.UpdateEntity(sourceEntity)
		}
	}
	for _, sessionID := range sessionIDs {
		if err := source.Worker.Submit(worker.AttachSessionCommand{
			SessionID: realtime.SessionID(sessionID.String()),
			PlayerID:  playerID,
		}); err != nil && !errors.Is(err, worker.ErrPlayerAlreadyExists) {
			continue
		}
		result := source.Worker.Tick()
		runtime.recordEnemyTelemetryLocked(source, result)
		_ = commandErrors(result)
		runtime.attachSessionToInstanceLocked(source, sessionID, playerID)
	}
}

func (runtime *Runtime) queueTransferFailedLocked(sessionIDs []auth.SessionID, portal worldmaps.PortalDefinition, source *mapInstance, reason string) {
	for _, sessionID := range sessionIDs {
		runtime.queueEventLocked(sessionID, realtime.EventMapTransferFailed, mapTransferFailedPayload{
			PortalID:             portal.PortalID.String(),
			FromPublicMapKey:     source.Definition.PublicMapKey.String(),
			Reason:               reason,
			MapSubscriptionEpoch: runtime.sessionMapEpochLocked(sessionID),
		})
	}
}
