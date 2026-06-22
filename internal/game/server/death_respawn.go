package server

import (
	"gameproject/internal/game/auth"
	"gameproject/internal/game/combat"
	deathdomain "gameproject/internal/game/death"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

type pendingRespawnTarget struct {
	PlayerID foundation.PlayerID
	WorldID  world.WorldID
	ZoneID   world.ZoneID
	MapID    worldmaps.MapID
	SpawnID  worldmaps.SpawnID
}

type repairRespawnDestination struct {
	MapID        worldmaps.MapID
	SpawnID      worldmaps.SpawnID
	PublicMapKey worldmaps.PublicMapKey
	Position     world.Vec2
}

type repairRespawnResult struct {
	PublicMapKey string
	Position     world.Vec2
	Protection   worldmaps.ClientProtectionSummary
}

func (runtime *Runtime) recordPendingRespawnTargetLocked(record deathdomain.DeathRecord) {
	if record.PlayerID.IsZero() || record.RespawnLocationID.IsZero() {
		return
	}
	mapID := worldmaps.MapID(record.ZoneID.String())
	if location, err := runtime.mapRouter.ActiveLocation(record.PlayerID); err == nil &&
		location.WorldID == record.WorldID &&
		location.ZoneID == record.ZoneID {
		mapID = location.InternalMapID
	}
	if runtime.pendingRespawns == nil {
		runtime.pendingRespawns = make(map[foundation.PlayerID]pendingRespawnTarget)
	}
	runtime.pendingRespawns[record.PlayerID] = pendingRespawnTarget{
		PlayerID: record.PlayerID,
		WorldID:  record.WorldID,
		ZoneID:   record.ZoneID,
		MapID:    mapID,
		SpawnID:  worldmaps.SpawnID(record.RespawnLocationID.String()),
	}
}

func (runtime *Runtime) repairRespawnPlayerLocked(playerID foundation.PlayerID, includeSessionID auth.SessionID) (repairRespawnResult, error) {
	destination, err := runtime.resolveRepairRespawnDestinationLocked(playerID)
	if err != nil {
		return repairRespawnResult{}, err
	}
	location, err := runtime.mapRouter.SetActiveLocationFromSpawn(playerID, destination.MapID, destination.SpawnID)
	if err != nil {
		fallback, fallbackErr := runtime.starterRepairRespawnDestinationLocked()
		if fallbackErr != nil {
			return repairRespawnResult{}, err
		}
		destination = fallback
		location, err = runtime.mapRouter.SetActiveLocationFromSpawn(playerID, destination.MapID, destination.SpawnID)
		if err != nil {
			return repairRespawnResult{}, err
		}
	}
	instance, err := runtime.mapInstanceForLocationLocked(location)
	if err != nil {
		return repairRespawnResult{}, err
	}
	state, ok := runtime.players[playerID]
	if !ok {
		return repairRespawnResult{}, worker.ErrUnknownPlayer
	}
	sessionIDs := runtime.sessionIDsForPlayerLocked(playerID, includeSessionID)
	for _, sessionID := range sessionIDs {
		runtime.detachSessionFromInactiveInstancesLocked(sessionID, location.InternalMapID)
	}
	runtime.removePlayerFromInactiveInstancesLocked(playerID, location.InternalMapID)
	if err := runtime.attachPlayerToDestinationLocked(instance, playerID, state.EntityID, destination.Position); err != nil {
		return repairRespawnResult{}, err
	}
	for _, sessionID := range sessionIDs {
		if err := instance.Worker.Submit(worker.AttachSessionCommand{
			SessionID: realtime.SessionID(sessionID.String()),
			PlayerID:  playerID,
		}); err != nil {
			return repairRespawnResult{}, err
		}
	}
	tickResult := instance.Worker.Tick()
	runtime.recordEnemyTelemetryLocked(instance, tickResult)
	if err := commandErrors(tickResult); err != nil {
		return repairRespawnResult{}, err
	}
	for _, sessionID := range sessionIDs {
		runtime.attachSessionToInstanceLocked(instance, sessionID, playerID)
	}
	delete(runtime.lastMove, playerID)
	actor, err := runtime.syncPlayerCombatActorLocked(playerID)
	if err != nil {
		return repairRespawnResult{}, err
	}
	actor.Dead = false
	actor.DiedAt = nil
	actor.Cooldowns = combat.CooldownState{}
	actor.Contributions = make(map[foundation.PlayerID]float64)
	if err := runtime.Combat.UpsertActor(actor); err != nil {
		return repairRespawnResult{}, err
	}
	protection, err := runtime.startPlayerProtectionLocked(playerID, location.InternalMapID, protectionReasonRespawn)
	if err != nil {
		return repairRespawnResult{}, err
	}
	runtime.queueProtectionUpdatedLocked(sessionIDs, protection)
	runtime.queuePositionCorrectedLocked(sessionIDs, state.EntityID, destination.Position)
	delete(runtime.pendingRespawns, playerID)
	return repairRespawnResult{
		PublicMapKey: destination.PublicMapKey.String(),
		Position:     destination.Position,
		Protection:   protection.clientSummary(),
	}, nil
}

func (runtime *Runtime) queuePositionCorrectedLocked(sessionIDs []auth.SessionID, entityID world.EntityID, position world.Vec2) {
	payload := map[string]any{
		"entity_id": entityID.String(),
		"position":  position,
	}
	for _, sessionID := range sessionIDs {
		runtime.queueEventLocked(sessionID, realtime.EventPositionCorrected, payload)
	}
}

func (runtime *Runtime) resolveRepairRespawnDestinationLocked(playerID foundation.PlayerID) (repairRespawnDestination, error) {
	if pending, ok := runtime.pendingRespawns[playerID]; ok {
		if destination, ok := runtime.repairRespawnDestinationFromSpawnLocked(pending.MapID, pending.SpawnID); ok {
			return destination, nil
		}
	}
	if location, err := runtime.mapRouter.ActiveLocation(playerID); err == nil {
		if destination, ok := runtime.repairRespawnDestinationFromSpawnLocked(location.InternalMapID, location.SpawnID); ok {
			return destination, nil
		}
		if definition, ok := runtime.mapCatalog.Get(location.InternalMapID); ok && len(definition.SpawnPoints) > 0 {
			if destination, ok := runtime.repairRespawnDestinationFromSpawnLocked(location.InternalMapID, definition.SpawnPoints[0].SpawnID); ok {
				return destination, nil
			}
		}
	}
	return runtime.starterRepairRespawnDestinationLocked()
}

func (runtime *Runtime) starterRepairRespawnDestinationLocked() (repairRespawnDestination, error) {
	definition, spawn, err := runtime.mapCatalog.StarterDefinition()
	if err != nil {
		return repairRespawnDestination{}, err
	}
	return repairRespawnDestination{
		MapID:        definition.InternalMapID,
		SpawnID:      spawn.SpawnID,
		PublicMapKey: definition.PublicMapKey,
		Position:     spawn.Position,
	}, nil
}

func (runtime *Runtime) repairRespawnDestinationFromSpawnLocked(mapID worldmaps.MapID, spawnID worldmaps.SpawnID) (repairRespawnDestination, bool) {
	definition, ok := runtime.mapCatalog.Get(mapID)
	if !ok {
		return repairRespawnDestination{}, false
	}
	spawn, ok := runtime.mapCatalog.Spawn(mapID, spawnID)
	if !ok {
		return repairRespawnDestination{}, false
	}
	if err := runtime.mapCatalog.ValidatePosition(mapID, spawn.Position); err != nil {
		return repairRespawnDestination{}, false
	}
	if _, err := runtime.mapInstanceLocked(mapID); err != nil {
		return repairRespawnDestination{}, false
	}
	return repairRespawnDestination{
		MapID:        mapID,
		SpawnID:      spawn.SpawnID,
		PublicMapKey: definition.PublicMapKey,
		Position:     spawn.Position,
	}, true
}
