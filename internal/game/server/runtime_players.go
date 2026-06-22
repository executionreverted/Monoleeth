package server

import (
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/worker"
)

type playerRuntimeState struct {
	EntityID world.EntityID
	Callsign string
	Rank     int
	Ship     shipSnapshotPayload
	Stats    statSnapshotPayload
	Wallet   walletSnapshotPayload
	Cargo    cargoSnapshotPayload
}

func (runtime *Runtime) ensurePlayerHangarLocked(playerID foundation.PlayerID) error {
	result, err := runtime.Hangar.EnsureStarterShip(playerID)
	if err != nil {
		return err
	}
	if result.HasActiveShip {
		return runtime.applyActiveShipLocked(playerID, result.ActiveShip.ShipID)
	}
	return nil
}

func (runtime *Runtime) applyActiveShipLocked(playerID foundation.PlayerID, shipID foundation.ShipID) error {
	definition, err := runtime.ShipCatalog.MustGet(shipID)
	if err != nil {
		return err
	}
	state, ok := runtime.players[playerID]
	if !ok {
		return worker.ErrUnknownPlayer
	}

	previousShipID := state.Ship.ActiveShipID
	state.Ship.ActiveShipID = shipID.String()
	if shipID == starterShipID {
		state.Ship.DisplayName = starterShipDisplayName
	} else {
		state.Ship.DisplayName = definition.Name
	}
	if previousShipID != "" && previousShipID != shipID.String() {
		state.Ship.Hull = int(definition.BaseStats.HP)
		state.Ship.MaxHull = int(definition.BaseStats.HP)
		state.Ship.Shield = int(definition.BaseStats.Shield)
		state.Ship.MaxShield = int(definition.BaseStats.Shield)
		state.Ship.Capacitor = int(definition.BaseStats.Energy)
		state.Ship.MaxCapacitor = int(definition.BaseStats.Energy)
		baseSpeed := float64(definition.BaseStats.Speed)
		if instance, _, err := runtime.activeMapInstanceLocked(playerID); err == nil && instance.HiddenPlayers[playerID] {
			runtime.stealthBaseSpeeds[playerID] = baseSpeed
			state.Stats.Speed = runtimePlayerSpeedForStealth(baseSpeed, true)
		} else {
			state.Stats.Speed = baseSpeed
		}
		state.Stats.RadarRange = float64(definition.BaseStats.Radar)
		state.Stats.CargoCapacity = definition.BaseStats.CargoCapacity
		state.Cargo.Capacity = definition.BaseStats.CargoCapacity
	}
	if state.Ship.RepairState == "" {
		state.Ship.RepairState = "ready"
	}
	runtime.players[playerID] = state
	return runtime.LoadoutStore.SetActiveShip(playerID, shipID)
}

func (runtime *Runtime) shipSwapContextLocked(playerID foundation.PlayerID) ships.ShipSwapContext {
	state := runtime.players[playerID]
	return ships.ShipSwapContext{
		InSafeHangarArea:  runtime.playerInSafeHangarAreaLocked(playerID),
		InCombat:          runtime.playerInCombatLocked(playerID),
		CurrentCargoUnits: state.Cargo.Used,
	}
}

func (runtime *Runtime) playerInCombatLocked(playerID foundation.PlayerID) bool {
	state, ok := runtime.players[playerID]
	if !ok {
		return false
	}
	return state.Ship.Disabled || state.Ship.RepairState == "disabled"
}

func (runtime *Runtime) setPlayerStealth(playerID foundation.PlayerID, enabled bool) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.setPlayerStealthLocked(playerID, enabled)
}

func (runtime *Runtime) setPlayerStealthLocked(playerID foundation.PlayerID, enabled bool) error {
	state, ok := runtime.players[playerID]
	if !ok {
		return worker.ErrUnknownPlayer
	}
	baseSpeed := runtime.stealthBaseSpeedLocked(playerID, state)
	speed := runtimePlayerSpeedForStealth(baseSpeed, enabled)
	instance, _, err := runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return err
	}
	if err := instance.Worker.Submit(worker.SetPlayerSpeedCommand{PlayerID: playerID, Speed: speed}); err != nil {
		return err
	}
	if err := instance.Worker.Submit(worker.SetPlayerAggroEligibilityCommand{PlayerID: playerID, Eligible: !enabled}); err != nil {
		return err
	}
	result := instance.Worker.Tick()
	if len(result.CommandErrors) > 0 {
		return result.CommandErrors[0].Err
	}
	state.Stats.Speed = speed
	runtime.players[playerID] = state
	if enabled {
		instance.HiddenPlayers[playerID] = true
		runtime.stealthBaseSpeeds[playerID] = baseSpeed
	} else {
		delete(instance.HiddenPlayers, playerID)
		delete(runtime.stealthBaseSpeeds, playerID)
		runtime.deleteHiddenPlayerWitnessesLocked(instance, playerID)
	}
	return nil
}

func (runtime *Runtime) stealthBaseSpeedLocked(playerID foundation.PlayerID, state playerRuntimeState) float64 {
	if baseSpeed := runtime.stealthBaseSpeeds[playerID]; baseSpeed > 0 {
		return baseSpeed
	}
	if state.Stats.Speed > 0 {
		if instance, _, err := runtime.activeMapInstanceLocked(playerID); err == nil && instance.HiddenPlayers[playerID] {
			return state.Stats.Speed / runtimeStealthSpeedMultiplier
		}
		return state.Stats.Speed
	}
	return defaultPlayerSpeed
}

func runtimePlayerSpeedForStealth(baseSpeed float64, enabled bool) float64 {
	if baseSpeed <= 0 {
		baseSpeed = defaultPlayerSpeed
	}
	if enabled {
		return baseSpeed * runtimeStealthSpeedMultiplier
	}
	return baseSpeed
}

func newPlayerRuntimeState(callsign string, entityID world.EntityID) playerRuntimeState {
	if callsign == "" {
		callsign = "Pilot"
	}
	return playerRuntimeState{
		EntityID: entityID,
		Callsign: callsign,
		Rank:     1,
		Ship: shipSnapshotPayload{
			ActiveShipID: starterShipID.String(),
			DisplayName:  starterShipDisplayName,
			Hull:         100,
			MaxHull:      100,
			Shield:       100,
			MaxShield:    100,
			Capacitor:    100,
			MaxCapacitor: 100,
			RepairState:  "ready",
		},
		Stats: statSnapshotPayload{
			Speed:                defaultPlayerSpeed,
			RadarRange:           defaultRadarRange,
			WeaponRange:          260,
			CargoCapacity:        60,
			LootPickupRange:      runtimeLootPickupRange,
			BasicLaserEnergyCost: runtimeBasicLaserEnergyCost,
			BasicLaserCooldownMS: runtimeBasicLaserCooldownMS,
		},
		Wallet: walletSnapshotPayload{},
		Cargo: cargoSnapshotPayload{
			Capacity: 60,
			Items:    []cargoItemStack{},
		},
	}
}

func (state playerRuntimeState) playerSnapshot() playerSnapshotPayload {
	return playerSnapshotPayload{
		Callsign:  state.Callsign,
		Rank:      state.Rank,
		HP:        state.Ship.Hull,
		MaxHP:     state.Ship.MaxHull,
		Shield:    state.Ship.Shield,
		MaxShield: state.Ship.MaxShield,
		Energy:    state.Ship.Capacitor,
		MaxEnergy: state.Ship.MaxCapacitor,
	}
}
