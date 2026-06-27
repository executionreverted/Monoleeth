package server

import (
	"sort"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/visibility"
	"gameproject/internal/game/world/worker"
)

const (
	runtimeAOITickPhaseAOI     = "aoi"
	runtimeAOITickPhaseEnqueue = "enqueue"
)

func (runtime *Runtime) worldSnapshotLocked(playerID foundation.PlayerID) (worldSnapshotPayload, error) {
	snapshot, radarRange, tick, err := runtime.aoiSnapshotForPlayerLocked(playerID)
	if err != nil {
		return worldSnapshotPayload{}, err
	}
	minimap, err := runtime.minimapForPlayerLocked(playerID, snapshot, radarRange)
	if err != nil {
		return worldSnapshotPayload{}, err
	}
	mapProjection, err := runtime.mapRouter.ClientProjection(playerID)
	if err != nil {
		return worldSnapshotPayload{}, err
	}
	mapProjection = runtime.mapProjectionWithViewerPolicyLocked(playerID, mapProjection)
	return worldSnapshotPayload{
		Sector:         sectorPayloadFromMap(mapProjection),
		Map:            mapProjection,
		Entities:       cloneAOIEntities(snapshot.Entities),
		Minimap:        minimap,
		SnapshotCursor: tick,
	}, nil
}

func (runtime *Runtime) worldSnapshotForSessionLocked(playerID foundation.PlayerID, sessionID auth.SessionID) (worldSnapshotPayload, error) {
	snapshot, err := runtime.worldSnapshotLocked(playerID)
	if err != nil {
		return worldSnapshotPayload{}, err
	}
	snapshot.MapSubscriptionEpoch = runtime.sessionMapEpochLocked(sessionID)
	return snapshot, nil
}

func (runtime *Runtime) currentMinimapPayload(playerID foundation.PlayerID) (minimapPayload, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	snapshot, radarRange, _, err := runtime.aoiSnapshotForPlayerLocked(playerID)
	if err != nil {
		return minimapPayload{}, err
	}
	return runtime.minimapForPlayerLocked(playerID, snapshot, radarRange)
}

func (runtime *Runtime) aoiSnapshotForPlayerLocked(playerID foundation.PlayerID) (aoi.Snapshot, float64, uint64, error) {
	instance, location, err := runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return aoi.Snapshot{}, 0, 0, err
	}
	playerEntity, ok := instance.Worker.PlayerEntity(playerID)
	if !ok {
		return aoi.Snapshot{}, 0, 0, worker.ErrUnknownPlayer
	}
	now := runtime.clock.Now()
	statSnapshot := runtime.visibilityStatSnapshotLocked(playerID, now)
	projectionRange := visibility.RadarRangeFromStatSnapshot(statSnapshot)
	workerSnapshot, err := instance.Worker.EntitiesWithinRadius(playerEntity.Position, projectionRange.Units())
	if err != nil {
		return aoi.Snapshot{}, 0, 0, err
	}
	snapshot := runtime.aoiSnapshotFromWorkerSnapshotLocked(playerID, instance, location, playerEntity, workerSnapshot)
	return snapshot, projectionRange.Units(), workerSnapshot.Tick, nil
}

func (runtime *Runtime) aoiSnapshotForPlayerFromSharedWorkerSnapshotLocked(playerID foundation.PlayerID, instance *mapInstance, location worldmaps.PlayerMapLocation, workerSnapshot worker.Snapshot) (aoi.Snapshot, uint64, error) {
	playerEntity, ok := runtime.playerEntityFromWorkerSnapshotLocked(playerID, workerSnapshot)
	if !ok {
		return aoi.Snapshot{}, 0, worker.ErrUnknownPlayer
	}
	snapshot := runtime.aoiSnapshotFromWorkerSnapshotLocked(playerID, instance, location, playerEntity, workerSnapshot)
	return snapshot, workerSnapshot.Tick, nil
}

func (runtime *Runtime) playerEntityFromWorkerSnapshotLocked(playerID foundation.PlayerID, workerSnapshot worker.Snapshot) (world.Entity, bool) {
	state, ok := runtime.players[playerID]
	if !ok {
		return world.Entity{}, false
	}
	for _, entity := range workerSnapshot.Entities {
		if entity.ID == state.EntityID {
			return entity, true
		}
	}
	return world.Entity{}, false
}

func (runtime *Runtime) aoiSnapshotFromWorkerSnapshotLocked(playerID foundation.PlayerID, instance *mapInstance, location worldmaps.PlayerMapLocation, playerEntity world.Entity, workerSnapshot worker.Snapshot) aoi.Snapshot {
	now := runtime.clock.Now()
	statSnapshot := runtime.visibilityStatSnapshotLocked(playerID, now)
	projectionRange := visibility.RadarRangeFromStatSnapshot(statSnapshot)
	viewer := visibility.Viewer{
		PlayerID:       playerID,
		WorldID:        location.WorldID,
		ZoneID:         location.ZoneID,
		Position:       playerEntity.Position,
		RadarRange:     projectionRange,
		DetectionStats: visibility.DetectionStatsFromStatSnapshot(statSnapshot),
		Witnesses:      runtime.hiddenPlayerWitnessesForViewerLocked(instance, playerID, now),
		ObservedAt:     now,
	}
	states := make([]aoi.EntityState, 0, len(workerSnapshot.Entities))
	for _, entity := range workerSnapshot.Entities {
		flags, display, combatStatus := runtime.publicEntityMetadataLocked(instance, playerID, entity)
		entityPlayerID, _, _ := runtime.playerByEntityLocked(entity.ID)
		hidden := instance.HiddenEntities[entity.ID]
		if !entityPlayerID.IsZero() && instance.HiddenPlayers[entityPlayerID] {
			hidden = true
			if entityPlayerID != playerID && runtime.hiddenPlayerWitnessActiveLocked(instance, playerID, entityPlayerID, now) {
				flags = append(flags, "scan_revealed")
			}
		}
		signature, stealthScore, jammerStrength := runtime.visibilityInputsForEntityLocked(entity, entityPlayerID, hidden)
		if entity.Type == world.EntityTypeNPC {
			if npcSignature, npcStealthScore, npcJammerStrength, ok := runtime.npcVisibilityInputsLocked(instance, entity, hidden); ok {
				signature = npcSignature
				stealthScore = npcStealthScore
				jammerStrength = npcJammerStrength
			}
		}
		states = append(states, aoi.EntityState{
			Entity:            entity,
			PlayerID:          entityPlayerID,
			Signature:         signature,
			StealthScore:      stealthScore,
			JammerStrength:    jammerStrength,
			Hidden:            hidden,
			PublicStatusFlags: flags,
			PublicDisplay:     display,
			PublicCombat:      combatStatus,
			PublicMovement:    runtime.publicMovementPayloadLocked(entity, now),
			ProjectionSource:  runtimeProjectionSourceWorker,
		})
	}
	return aoi.BuildVisibleSnapshot(viewer, states)
}

func (runtime *Runtime) effectiveRadarRangeUnitsLocked(playerID foundation.PlayerID) float64 {
	state, ok := runtime.players[playerID]
	if !ok {
		// Conservative server fallback for bootstrap/test harnesses before a
		// stat provider has materialized an effective radar snapshot.
		return defaultRadarRange
	}
	return runtime.explorationStatsForPlayerStateLocked(state).RadarRange
}

func (runtime *Runtime) hiddenPlayerWitnessesForViewerLocked(instance *mapInstance, viewerID foundation.PlayerID, now time.Time) []visibility.Witness {
	witnesses := make([]visibility.Witness, 0)
	if instance == nil {
		return witnesses
	}
	for key, expiresAt := range instance.HiddenPlayerWitnesses {
		if !expiresAt.After(now) {
			delete(instance.HiddenPlayerWitnesses, key)
			continue
		}
		if key.ViewerPlayerID != viewerID {
			continue
		}
		witnesses = append(witnesses, visibility.Witness{
			TargetPlayerID: key.TargetPlayerID,
			ExpiresAt:      expiresAt,
		})
	}
	return witnesses
}

func (runtime *Runtime) hiddenPlayerWitnessActiveLocked(instance *mapInstance, viewerID foundation.PlayerID, targetID foundation.PlayerID, now time.Time) bool {
	if instance == nil {
		return false
	}
	key := hiddenPlayerWitnessKey{
		ViewerPlayerID: viewerID,
		TargetPlayerID: targetID,
	}
	expiresAt, ok := instance.HiddenPlayerWitnesses[key]
	if !ok {
		return false
	}
	if !expiresAt.After(now) {
		delete(instance.HiddenPlayerWitnesses, key)
		return false
	}
	return true
}

func (runtime *Runtime) deleteHiddenPlayerWitnessesLocked(instance *mapInstance, playerID foundation.PlayerID) {
	if instance == nil {
		return
	}
	for key := range instance.HiddenPlayerWitnesses {
		if key.ViewerPlayerID == playerID || key.TargetPlayerID == playerID {
			delete(instance.HiddenPlayerWitnesses, key)
		}
	}
}

func (runtime *Runtime) tickAndCollectAOIEvents() map[auth.SessionID][]realtime.EventEnvelope {
	runtime.tickMu.Lock()
	defer runtime.tickMu.Unlock()

	tickedInstances := runtime.tickMapInstances()

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	failedInstances := make(map[*mapInstance]struct{})
	sharedSnapshots := make(map[*mapInstance]worker.Snapshot, len(tickedInstances))
	for _, ticked := range tickedInstances {
		instance := ticked.instance
		result := ticked.result
		runtime.recordAOITickPhaseDurationsLocked(instance, result.PhaseDurations)
		runtime.recordEnemyTelemetryLocked(instance, result)
		if err := commandErrors(result); err != nil {
			runtime.recordAOITickErrorLocked(instance, "worker_tick", err)
			failedInstances[instance] = struct{}{}
			continue
		}
		if err := runtime.syncAliveNPCCombatActorProjectionsLocked(instance); err != nil {
			runtime.recordAOITickErrorLocked(instance, "npc_projection", err)
			failedInstances[instance] = struct{}{}
			continue
		}
		sharedSnapshots[instance] = instance.Worker.Snapshot()
	}
	eventsBySession := make(map[auth.SessionID][]realtime.EventEnvelope)
	for _, instance := range runtime.sortedMapInstancesLocked() {
		if _, failed := failedInstances[instance]; failed {
			continue
		}
		sharedSnapshot, hasSharedSnapshot := sharedSnapshots[instance]
		var aoiDuration time.Duration
		var enqueueDuration time.Duration
		for _, sessionID := range sortedSessionIDs(instance.ActiveSessions) {
			playerID := instance.ActiveSessions[sessionID]
			diffStarted := time.Now()
			diff, ok := runtime.aoiDiffForInstanceLocked(instance, sessionID, playerID, sharedSnapshot, hasSharedSnapshot)
			aoiDuration += time.Since(diffStarted)
			if !ok {
				continue
			}
			enqueueStarted := time.Now()
			events := runtime.eventsForAOIDiffLocked(sessionID, diff)
			if len(events) > 0 {
				eventsBySession[sessionID] = append(eventsBySession[sessionID], events...)
			}
			enqueueDuration += time.Since(enqueueStarted)
		}
		runtime.recordAOITickPhaseDurationLocked(instance, runtimeAOITickPhaseAOI, aoiDuration)
		runtime.recordAOITickPhaseDurationLocked(instance, runtimeAOITickPhaseEnqueue, enqueueDuration)
	}
	runtime.recordReplayEventsBySessionLocked(eventsBySession)
	return eventsBySession
}

type tickedMapInstance struct {
	instance *mapInstance
	result   worker.TickResult
}

func (runtime *Runtime) tickMapInstances() []tickedMapInstance {
	if runtime == nil || len(runtime.mapTickInstances) == 0 {
		return nil
	}
	results := make([]tickedMapInstance, 0, len(runtime.mapTickInstances))
	for _, instance := range runtime.mapTickInstances {
		if instance == nil || instance.Worker == nil {
			continue
		}
		results = append(results, tickedMapInstance{
			instance: instance,
			result:   instance.Worker.Tick(),
		})
	}
	return results
}

func (runtime *Runtime) recordAOITickErrorLocked(instance *mapInstance, stage string, err error) {
	if runtime == nil || runtime.Metrics == nil || instance == nil || err == nil {
		return
	}
	code, ok := foundation.CodeOf(err)
	if !ok {
		code = foundation.CodeInternal
	}
	_ = runtime.Metrics.AddCounter(observability.MetricErrorsByCode, observability.Labels{
		"code":     code.String(),
		"op":       "runtime_aoi_tick",
		"stage":    stage,
		"world_id": instance.Definition.WorldID.String(),
		"zone_id":  instance.Definition.ZoneID.String(),
	}, 1)
}

func (runtime *Runtime) aoiDiffEventsLocked(sessionID auth.SessionID, playerID foundation.PlayerID) []realtime.EventEnvelope {
	instance, _, err := runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return nil
	}
	return runtime.aoiDiffEventsForInstanceLocked(instance, sessionID, playerID, worker.Snapshot{}, false)
}

func (runtime *Runtime) aoiDiffEventsForInstanceLocked(instance *mapInstance, sessionID auth.SessionID, playerID foundation.PlayerID, sharedSnapshot worker.Snapshot, hasSharedSnapshot bool) []realtime.EventEnvelope {
	diff, ok := runtime.aoiDiffForInstanceLocked(instance, sessionID, playerID, sharedSnapshot, hasSharedSnapshot)
	if !ok {
		return nil
	}
	return runtime.eventsForAOIDiffLocked(sessionID, diff)
}

func (runtime *Runtime) aoiDiffForInstanceLocked(instance *mapInstance, sessionID auth.SessionID, playerID foundation.PlayerID, sharedSnapshot worker.Snapshot, hasSharedSnapshot bool) (aoi.Diff, bool) {
	if instance == nil {
		return aoi.Diff{}, false
	}
	location, err := runtime.mapRouter.ActiveLocation(playerID)
	if err != nil || location.InternalMapID != instance.Definition.InternalMapID {
		delete(instance.ActiveSessions, sessionID)
		delete(instance.LastAOI, sessionID)
		if runtime.sessionLocations[sessionID] == instance.Definition.InternalMapID {
			delete(runtime.sessionLocations, sessionID)
		}
		return aoi.Diff{}, false
	}
	if runtime.sessionLocations[sessionID] != instance.Definition.InternalMapID {
		delete(instance.LastAOI, sessionID)
		runtime.attachSessionToInstanceLocked(instance, sessionID, playerID)
	}
	var current aoi.Snapshot
	if hasSharedSnapshot {
		shared, _, err := runtime.aoiSnapshotForPlayerFromSharedWorkerSnapshotLocked(playerID, instance, location, sharedSnapshot)
		if err != nil {
			return aoi.Diff{}, false
		}
		current = shared
	} else {
		snapshot, _, _, err := runtime.aoiSnapshotForPlayerLocked(playerID)
		if err != nil {
			return aoi.Diff{}, false
		}
		current = snapshot
	}
	previous := instance.LastAOI[sessionID]
	diff := aoi.DiffSnapshots(previous, current)
	instance.LastAOI[sessionID] = aoi.Snapshot{Entities: cloneAOIEntities(current.Entities)}
	return diff, true
}

func (runtime *Runtime) eventsForAOIDiffLocked(sessionID auth.SessionID, diff aoi.Diff) []realtime.EventEnvelope {
	events := make([]realtime.EventEnvelope, 0, len(diff.Entered)+len(diff.Updated)+len(diff.Left))
	for _, entity := range diff.Entered {
		events = append(events, runtime.eventLocked(sessionID, realtime.EventAOIEntityEntered, entity))
	}
	for _, entity := range diff.Updated {
		events = append(events, runtime.eventLocked(sessionID, realtime.EventAOIEntityUpdated, entity))
	}
	for _, entityID := range diff.Left {
		events = append(events, runtime.eventLocked(sessionID, realtime.EventAOIEntityLeft, map[string]string{"entity_id": entityID.String()}))
	}
	return events
}

func (runtime *Runtime) recordAOITickPhaseDurationsLocked(instance *mapInstance, phases map[string]time.Duration) {
	for phase, duration := range phases {
		runtime.recordAOITickPhaseDurationLocked(instance, phase, duration)
	}
}

func (runtime *Runtime) recordAOITickPhaseDurationLocked(instance *mapInstance, phase string, duration time.Duration) {
	if runtime == nil || runtime.Metrics == nil || instance == nil || phase == "" || duration < 0 {
		return
	}
	_ = runtime.Metrics.RecordZoneTickPhaseDuration(instance.Definition.WorldID, instance.Definition.ZoneID, phase, duration)
}

func (runtime *Runtime) sortedMapInstancesLocked() []*mapInstance {
	return sortedMapInstances(runtime.mapInstances)
}

func sortedMapInstances(mapInstances map[worldmaps.MapID]*mapInstance) []*mapInstance {
	if len(mapInstances) == 0 {
		return nil
	}
	mapIDs := make([]worldmaps.MapID, 0, len(mapInstances))
	for mapID := range mapInstances {
		mapIDs = append(mapIDs, mapID)
	}
	sort.Slice(mapIDs, func(i, j int) bool {
		return mapIDs[i] < mapIDs[j]
	})
	instances := make([]*mapInstance, 0, len(mapIDs))
	for _, mapID := range mapIDs {
		if instance := mapInstances[mapID]; instance != nil && instance.Worker != nil {
			instances = append(instances, instance)
		}
	}
	return instances
}

func sortedSessionIDs(sessions map[auth.SessionID]foundation.PlayerID) []auth.SessionID {
	if len(sessions) == 0 {
		return nil
	}
	sessionIDs := make([]auth.SessionID, 0, len(sessions))
	for sessionID := range sessions {
		sessionIDs = append(sessionIDs, sessionID)
	}
	sort.Slice(sessionIDs, func(i, j int) bool {
		return sessionIDs[i] < sessionIDs[j]
	})
	return sessionIDs
}

func (runtime *Runtime) publicEntityMetadataLocked(instance *mapInstance, viewerID foundation.PlayerID, entity world.Entity) ([]aoi.StatusFlag, *aoi.EntityDisplay, *aoi.EntityCombatStatus) {
	switch entity.Type {
	case world.EntityTypePlayer:
		if playerID, playerState, ok := runtime.playerByEntityLocked(entity.ID); ok {
			if playerID == viewerID {
				flags := []aoi.StatusFlag{"friendly", "self"}
				if instance != nil && instance.HiddenPlayers[playerID] {
					flags = append(flags, "stealthed")
				}
				return flags, &aoi.EntityDisplay{Label: playerState.Callsign, Disposition: "self"}, nil
			}
			return []aoi.StatusFlag{"friendly"}, &aoi.EntityDisplay{Label: playerState.Callsign, Disposition: "friendly"}, nil
		}
		return []aoi.StatusFlag{"friendly"}, &aoi.EntityDisplay{Label: "Pilot", Disposition: "friendly"}, nil
	case world.EntityTypeNPC:
		return runtime.publicNPCMetadataLocked(instance, entity)
	case world.EntityTypeLoot:
		return []aoi.StatusFlag{"loot"}, &aoi.EntityDisplay{Label: "Loot Cache", Disposition: "neutral"}, nil
	case world.EntityTypePlanetSignal:
		return []aoi.StatusFlag{"unknown_signal"}, &aoi.EntityDisplay{Label: "Unknown Signal", Disposition: "unknown"}, nil
	default:
		return nil, nil, nil
	}
}

func (runtime *Runtime) playerByEntityLocked(entityID world.EntityID) (foundation.PlayerID, playerRuntimeState, bool) {
	for playerID, state := range runtime.players {
		if state.EntityID == entityID {
			return playerID, state, true
		}
	}
	return "", playerRuntimeState{}, false
}

func (runtime *Runtime) minimapForPlayerLocked(playerID foundation.PlayerID, snapshot aoi.Snapshot, radarRange float64) (minimapPayload, error) {
	payload := minimapFromAOI(snapshot, radarRange)
	remembered, err := runtime.rememberedMinimapPayloadLocked(playerID)
	if err != nil {
		return minimapPayload{}, err
	}
	payload.Remembered = remembered
	return payload, nil
}

func (runtime *Runtime) rememberedMinimapPayloadLocked(playerID foundation.PlayerID) ([]minimapMemoryPayload, error) {
	intelRows, err := runtime.Discovery.PlayerPlanetIntelRecords(playerID)
	if err != nil {
		return nil, err
	}
	location, err := runtime.mapRouter.ActiveLocation(playerID)
	if err != nil {
		return nil, err
	}
	mapProjection, err := runtime.mapCatalog.ClientProjection(location.InternalMapID)
	if err != nil {
		return nil, err
	}
	publicMapKey := publicMapKeyFromProjection(mapProjection)
	remembered := make([]minimapMemoryPayload, 0, len(intelRows))
	for _, intel := range intelRows {
		planet, ok, err := runtime.Discovery.Planet(intel.PlanetID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if !intelAndPlanetMatchActiveMap(intel, planet, location.WorldID, location.ZoneID) {
			continue
		}
		remembered = append(remembered, minimapMemoryPayload{
			Kind:             "known_planet",
			SectorKey:        publicMapKey,
			PublicMapKey:     publicMapKey,
			PlanetID:         intel.PlanetID.String(),
			DetailID:         intel.PlanetID.String(),
			Label:            planetMemoryLabel(planet),
			Position:         intel.Coordinates,
			Freshness:        string(intel.State),
			ProjectionSource: runtimeProjectionSourceKnownIntel,
		})
	}
	return remembered, nil
}

func (runtime *Runtime) publicMovementPayloadLocked(entity world.Entity, _ time.Time) *aoi.EntityMovementStatus {
	if !entity.Movement.Moving {
		return nil
	}
	if entity.Movement.Speed <= 0 || entity.Movement.ArriveAtMS < entity.Movement.StartedAtMS {
		return nil
	}
	return &aoi.EntityMovementStatus{
		Moving:      true,
		Origin:      entity.Movement.Origin,
		Target:      entity.Movement.Target,
		Speed:       entity.Movement.Speed,
		StartedAtMS: entity.Movement.StartedAtMS,
		ArriveAtMS:  entity.Movement.ArriveAtMS,
	}
}
