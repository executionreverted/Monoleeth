package server

import (
	"encoding/json"

	deathdomain "gameproject/internal/game/death"
	gameevents "gameproject/internal/game/events"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world/worker"
)

// Record accepts server-owned domain events and maps the client-safe subset to
// realtime events. It satisfies domain services' optional EventEmitter hook.
func (runtime *Runtime) Record(event gameevents.EventEnvelope) {
	if runtime == nil {
		return
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.recordDomainEventLocked(event)
}

func (runtime *Runtime) recordDomainEventLocked(event gameevents.EventEnvelope) {
	switch event.Type {
	case deathdomain.EventShipDisabled:
		var payload deathdomain.ShipDisabledEvent
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		runtime.applyShipDisabledDomainEventLocked(payload)
	}
}

func (runtime *Runtime) applyShipDisabledDomainEventLocked(payload deathdomain.ShipDisabledEvent) {
	if err := payload.PlayerID.Validate(); err != nil {
		return
	}
	if err := payload.ShipID.Validate(); err != nil {
		return
	}
	state, ok := runtime.players[payload.PlayerID]
	if !ok || state.Ship.ActiveShipID != payload.ShipID.String() {
		return
	}

	state.Ship.Disabled = true
	state.Ship.RepairState = "disabled"
	state.Ship.Hull = 0
	state.Ship.Shield = 0
	state.Ship.Capacitor = 0
	runtime.players[payload.PlayerID] = state

	_, _ = runtime.syncPlayerCombatActorLocked(payload.PlayerID)
	var activeInstance *mapInstance
	if instance, _, err := runtime.activeMapInstanceLocked(payload.PlayerID); err == nil {
		activeInstance = instance
		if err := instance.Worker.Submit(worker.StopCommand{PlayerID: payload.PlayerID}); err == nil {
			result := instance.Worker.Tick()
			runtime.recordEnemyTelemetryLocked(instance, result)
			_ = commandErrors(result)
		}
	}

	publicPayload := map[string]any{
		"ship_id":         payload.ShipID.String(),
		"disabled_reason": payload.DisabledReason,
		"ship":            state.Ship,
		"repair_quote":    runtime.repairQuoteLocked(state),
	}
	for sessionID, playerID := range runtime.sessions {
		if playerID != payload.PlayerID {
			continue
		}
		runtime.queueEventLocked(sessionID, realtime.EventDeathShipDisabled, publicPayload)
		runtime.queueEventLocked(sessionID, realtime.EventShipSnapshot, state.Ship)
		runtime.queueEventLocked(sessionID, realtime.EventPlayerSnapshot, state.playerSnapshot())
		if activeInstance != nil {
			entity, ok := activeInstance.Worker.PlayerEntity(payload.PlayerID)
			if !ok {
				continue
			}
			runtime.queueEventLocked(sessionID, realtime.EventMovementStopped, map[string]any{
				"entity_id": entity.ID.String(),
				"position":  entity.Position,
			})
		}
	}
}
