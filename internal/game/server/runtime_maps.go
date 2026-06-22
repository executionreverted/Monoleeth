package server

import (
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

var (
	errMapInstanceNotFound = errors.New("map instance not found")
	errTransferActive      = errors.New("map transfer active")
	errScanPulseActive     = errors.New("scan pulse active")
	errMapEpochChanged     = errors.New("map subscription epoch changed")
)

type hiddenPlayerWitnessKey struct {
	ViewerPlayerID foundation.PlayerID
	TargetPlayerID foundation.PlayerID
}

type mapInstance struct {
	Definition            worldmaps.MapDefinition
	Worker                *worker.Worker
	ActiveSessions        map[auth.SessionID]foundation.PlayerID
	LastAOI               map[auth.SessionID]aoi.Snapshot
	HiddenEntities        map[world.EntityID]bool
	HiddenPlayers         map[foundation.PlayerID]bool
	HiddenPlayerWitnesses map[hiddenPlayerWitnessKey]time.Time
}

func (runtime *Runtime) playerInSafeHangarAreaLocked(playerID foundation.PlayerID) bool {
	instance, _, err := runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return false
	}
	entity, ok := instance.Worker.PlayerEntity(playerID)
	if !ok {
		return false
	}
	if entity.Movement.Moving {
		return false
	}
	safeZone, ok := instance.Definition.SafeZoneAt(entity.Position)
	return ok && safeZone.HangarActions
}

func (runtime *Runtime) activeMapInstanceLocked(playerID foundation.PlayerID) (*mapInstance, worldmaps.PlayerMapLocation, error) {
	if runtime == nil || runtime.mapRouter == nil {
		return nil, worldmaps.PlayerMapLocation{}, errMapInstanceNotFound
	}
	location, err := runtime.mapRouter.ActiveLocation(playerID)
	if err != nil {
		return nil, worldmaps.PlayerMapLocation{}, err
	}
	instance, err := runtime.mapInstanceForLocationLocked(location)
	if err != nil {
		return nil, worldmaps.PlayerMapLocation{}, err
	}
	return instance, location, nil
}

func (runtime *Runtime) mapInstanceForLocationLocked(location worldmaps.PlayerMapLocation) (*mapInstance, error) {
	instance, err := runtime.mapInstanceLocked(location.InternalMapID)
	if err != nil {
		return nil, err
	}
	if instance.Definition.WorldID != location.WorldID || instance.Definition.ZoneID != location.ZoneID {
		return nil, fmt.Errorf("active map %q location %q/%q does not match instance %q/%q: %w",
			location.InternalMapID,
			location.WorldID,
			location.ZoneID,
			instance.Definition.WorldID,
			instance.Definition.ZoneID,
			errMapInstanceNotFound)
	}
	return instance, nil
}

func (runtime *Runtime) mapInstanceLocked(mapID worldmaps.MapID) (*mapInstance, error) {
	if runtime == nil {
		return nil, errMapInstanceNotFound
	}
	instance, ok := runtime.mapInstances[mapID]
	if !ok || instance == nil || instance.Worker == nil {
		return nil, fmt.Errorf("map %q: %w", mapID, errMapInstanceNotFound)
	}
	return instance, nil
}

func (runtime *Runtime) removePlayerFromInactiveInstancesLocked(playerID foundation.PlayerID, activeMapID worldmaps.MapID) {
	state, ok := runtime.players[playerID]
	if !ok {
		return
	}
	for mapID, instance := range runtime.mapInstances {
		if mapID == activeMapID || instance == nil || instance.Worker == nil {
			continue
		}
		if _, ok := instance.Worker.PlayerEntity(playerID); ok {
			instance.Worker.RemoveEntity(state.EntityID)
		}
		delete(instance.HiddenPlayers, playerID)
		runtime.deleteHiddenPlayerWitnessesLocked(instance, playerID)
	}
}

func (runtime *Runtime) detachSessionFromInactiveInstancesLocked(sessionID auth.SessionID, activeMapID worldmaps.MapID) {
	for mapID, instance := range runtime.mapInstances {
		if mapID == activeMapID || instance == nil || instance.Worker == nil {
			continue
		}
		runtime.detachSessionFromInstanceLocked(instance, sessionID, true)
	}
}

func (runtime *Runtime) detachSessionFromAllInstancesLocked(sessionID auth.SessionID) {
	for _, instance := range runtime.mapInstances {
		if instance == nil || instance.Worker == nil {
			continue
		}
		runtime.detachSessionFromInstanceLocked(instance, sessionID, true)
	}
}

func (runtime *Runtime) attachSessionToInstanceLocked(instance *mapInstance, sessionID auth.SessionID, playerID foundation.PlayerID) {
	if instance == nil {
		return
	}
	if instance.ActiveSessions == nil {
		instance.ActiveSessions = make(map[auth.SessionID]foundation.PlayerID)
	}
	if instance.LastAOI == nil {
		instance.LastAOI = make(map[auth.SessionID]aoi.Snapshot)
	}
	instance.ActiveSessions[sessionID] = playerID
	runtime.sessions[sessionID] = playerID
	if runtime.sessionLocations[sessionID] != instance.Definition.InternalMapID || runtime.sessionEpochs[sessionID] == 0 {
		runtime.nextSessionEpoch++
		runtime.sessionEpochs[sessionID] = runtime.nextSessionEpoch
	}
	runtime.sessionLocations[sessionID] = instance.Definition.InternalMapID
}

func (runtime *Runtime) detachSessionFromInstanceLocked(instance *mapInstance, sessionID auth.SessionID, settle bool) {
	if instance == nil || instance.Worker == nil {
		return
	}
	command := worker.Command(worker.DetachSessionCommand{SessionID: realtime.SessionID(sessionID.String())})
	if settle {
		command = worker.SettleAndDetachSessionCommand{SessionID: realtime.SessionID(sessionID.String())}
	}
	_ = instance.Worker.Submit(command)
	result := instance.Worker.Tick()
	runtime.recordEnemyTelemetryLocked(instance, result)
	_ = commandErrors(result)
	delete(instance.ActiveSessions, sessionID)
	delete(instance.LastAOI, sessionID)
	if runtime.sessionLocations[sessionID] == instance.Definition.InternalMapID {
		delete(runtime.sessionLocations, sessionID)
	}
}

func (runtime *Runtime) sessionMapEpochLocked(sessionID auth.SessionID) uint64 {
	if runtime == nil {
		return 0
	}
	return runtime.sessionEpochs[sessionID]
}
