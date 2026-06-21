package server

import (
	"math"
	"time"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/worker"
)

type runtimeScannerModuleProvider struct {
	runtime *Runtime
}

func (provider runtimeScannerModuleProvider) HasEquippedScannerModule(input discovery.ScannerModuleInput) (bool, error) {
	provider.runtime.mu.Lock()
	defer provider.runtime.mu.Unlock()

	state, ok := provider.runtime.players[input.PlayerID]
	if !ok {
		return false, worker.ErrUnknownPlayer
	}
	return input.ShipID == starterShipID && state.Ship.ActiveShipID == starterShipID.String(), nil
}

type runtimeScannerStatsProvider struct {
	runtime *Runtime
}

func (provider runtimeScannerStatsProvider) ScanStats(input discovery.ScannerStatsInput) (stats.StatSnapshot, error) {
	provider.runtime.mu.Lock()
	defer provider.runtime.mu.Unlock()

	state, ok := provider.runtime.players[input.PlayerID]
	if !ok {
		return stats.StatSnapshot{}, worker.ErrUnknownPlayer
	}
	if input.ShipID != starterShipID {
		return stats.StatSnapshot{}, worker.ErrUnknownPlayer
	}
	return stats.NewStatSnapshot(input.PlayerID, input.ShipID, 1, stats.EffectiveStats{
		Core: stats.CoreStats{
			EnergyMax:   float64(state.Ship.MaxCapacitor),
			EnergyRegen: 4,
		},
		Exploration: stats.ExplorationStats{
			RadarRange:   state.Stats.RadarRange,
			ScanPower:    starterScannerScanPower,
			ScanRadius:   starterScannerScanRadius,
			ScanInterval: starterScannerScanInterval.Seconds(),
		},
	}, provider.runtime.clock.Now()), nil
}

type runtimeScannerPositionProvider struct {
	runtime *Runtime
}

func (provider runtimeScannerPositionProvider) PlayerScanPosition(input discovery.ScannerPositionInput) (discovery.ScannerPosition, error) {
	provider.runtime.mu.Lock()
	defer provider.runtime.mu.Unlock()

	instance, _, err := provider.runtime.activeMapInstanceLocked(input.PlayerID)
	if err != nil {
		return discovery.ScannerPosition{}, err
	}
	entity, ok := instance.Worker.PlayerEntity(input.PlayerID)
	if !ok {
		return discovery.ScannerPosition{}, worker.ErrUnknownPlayer
	}
	return discovery.ScannerPosition{
		WorldID:  entity.WorldID,
		ZoneID:   entity.ZoneID,
		Position: entity.Position,
		Movement: entity.Movement,
	}, nil
}

type runtimeScannerCooldownProvider struct {
	runtime *Runtime
}

type scanCapacitorSpendRecord struct {
	PlayerID foundation.PlayerID
	ShipID   foundation.ShipID
	WorldID  foundation.WorldID
	ZoneID   foundation.ZoneID
	Amount   int
}

func (provider runtimeScannerCooldownProvider) StartScanCooldown(input discovery.ScannerCooldownInput) (discovery.ScannerCooldownResult, error) {
	provider.runtime.mu.Lock()
	defer provider.runtime.mu.Unlock()

	key := scanCooldownKey{
		PlayerID: input.PlayerID,
		ShipID:   input.ShipID,
		WorldID:  input.WorldID,
		ZoneID:   input.ZoneID,
	}
	now := input.StartedAt.UTC()
	if now.IsZero() {
		now = provider.runtime.clock.Now().UTC()
	}
	if readyAt := provider.runtime.scanCooldowns[key]; !readyAt.IsZero() && now.Before(readyAt) {
		return discovery.ScannerCooldownResult{Accepted: false, ReadyAt: readyAt.UTC()}, nil
	}
	if err := provider.runtime.spendScannerCapacitorLocked(input); err != nil {
		return discovery.ScannerCooldownResult{}, err
	}
	nextReadyAt := now.Add(input.Duration)
	if input.Duration <= 0 {
		nextReadyAt = now.Add(time.Second)
	}
	provider.runtime.scanCooldowns[key] = nextReadyAt.UTC()
	return discovery.ScannerCooldownResult{Accepted: true, ReadyAt: now}, nil
}

func (runtime *Runtime) spendScannerCapacitorLocked(input discovery.ScannerCooldownInput) error {
	if spent, ok := runtime.scanCapacitorSpends[input.PulseReference]; ok {
		if !spent.matches(input) {
			return discovery.ErrScanPulseNotFound
		}
		return nil
	}

	state, ok := runtime.players[input.PlayerID]
	if !ok {
		return worker.ErrUnknownPlayer
	}
	if input.ShipID != starterShipID || state.Ship.ActiveShipID != starterShipID.String() {
		return worker.ErrUnknownPlayer
	}
	if state.Ship.Disabled || state.Ship.Capacitor < starterScannerEnergyCost {
		return discovery.ErrScannerEnergyUnavailable
	}

	state.Ship.Capacitor -= starterScannerEnergyCost
	runtime.players[input.PlayerID] = state
	runtime.scanCapacitorSpends[input.PulseReference] = scanCapacitorSpendRecord{
		PlayerID: input.PlayerID,
		ShipID:   input.ShipID,
		WorldID:  input.WorldID,
		ZoneID:   input.ZoneID,
		Amount:   starterScannerEnergyCost,
	}
	return nil
}

func (record scanCapacitorSpendRecord) matches(input discovery.ScannerCooldownInput) bool {
	return record.PlayerID == input.PlayerID &&
		record.ShipID == input.ShipID &&
		record.WorldID == input.WorldID &&
		record.ZoneID == input.ZoneID &&
		record.Amount == starterScannerEnergyCost
}

type runtimeScannerEnergyProvider struct {
	runtime *Runtime
}

func (provider runtimeScannerEnergyProvider) CheckScanEnergy(input discovery.ScannerEnergyInput) (discovery.ScannerEnergyResult, error) {
	if err := input.Validate(); err != nil {
		return discovery.ScannerEnergyResult{}, err
	}

	provider.runtime.mu.Lock()
	defer provider.runtime.mu.Unlock()

	state, ok := provider.runtime.players[input.PlayerID]
	if !ok {
		return discovery.ScannerEnergyResult{}, worker.ErrUnknownPlayer
	}
	accepted := !state.Ship.Disabled && state.Ship.Capacitor >= starterScannerEnergyCost
	return discovery.ScannerEnergyResult{Accepted: accepted}, nil
}

type runtimeScanXPProvider struct {
	progression *progression.ProgressionService
}

type runtimeScannerPlayerRevealProvider struct {
	runtime *Runtime
}

func (provider runtimeScannerPlayerRevealProvider) RevealHiddenPlayer(input discovery.ScannerPlayerRevealInput) (discovery.ScannerPlayerRevealResult, error) {
	if err := input.Validate(); err != nil {
		return discovery.ScannerPlayerRevealResult{}, err
	}
	provider.runtime.mu.Lock()
	defer provider.runtime.mu.Unlock()

	if _, ok := provider.runtime.players[input.PlayerID]; !ok {
		return discovery.ScannerPlayerRevealResult{}, worker.ErrUnknownPlayer
	}
	if input.Stats.Exploration.ScanPower <= 0 || input.Stats.Exploration.ScanRadius <= 0 {
		return discovery.ScannerPlayerRevealResult{}, nil
	}

	maxDistanceSq := input.Stats.Exploration.ScanRadius * input.Stats.Exploration.ScanRadius
	var bestTarget foundation.PlayerID
	var bestDistanceSq float64
	projectionMiss := false
	for targetID, hidden := range provider.runtime.hiddenPlayers {
		if !hidden || targetID == input.PlayerID {
			continue
		}
		instance, _, err := provider.runtime.activeMapInstanceLocked(targetID)
		if err != nil {
			continue
		}
		entity, ok := instance.Worker.PlayerEntity(targetID)
		if !ok || entity.WorldID != input.WorldID || entity.ZoneID != input.ZoneID {
			continue
		}
		distanceSq := world.DistanceSquared(input.Position, entity.Position)
		if distanceSq > maxDistanceSq {
			continue
		}
		if !withinRuntimeLiveProjectionWindow(input.Position, entity.Position) {
			projectionMiss = true
			continue
		}
		if bestTarget.IsZero() ||
			distanceSq < bestDistanceSq ||
			(distanceSq == bestDistanceSq && targetID.String() < bestTarget.String()) {
			bestTarget = targetID
			bestDistanceSq = distanceSq
		}
	}
	if bestTarget.IsZero() {
		if projectionMiss {
			return discovery.ScannerPlayerRevealResult{NoSignal: true}, nil
		}
		return discovery.ScannerPlayerRevealResult{}, nil
	}

	provider.runtime.hiddenPlayerWitnesses[hiddenPlayerWitnessKey{
		ViewerPlayerID: input.PlayerID,
		TargetPlayerID: bestTarget,
	}] = input.RevealedAt.UTC().Add(runtimeHiddenPlayerWitnessDuration)
	return discovery.ScannerPlayerRevealResult{Revealed: true}, nil
}

func withinRuntimeLiveProjectionWindow(center world.Vec2, position world.Vec2) bool {
	return math.Abs(position.X-center.X) <= runtimeLiveProjectionHalfExtent &&
		math.Abs(position.Y-center.Y) <= runtimeLiveProjectionHalfExtent
}

func (provider runtimeScanXPProvider) GrantScanXP(input discovery.ScanXPGrantInput) (discovery.ScanXPGrantResult, error) {
	result, err := provider.progression.GrantXP(progression.GrantXPInput{
		PlayerID:       input.PlayerID,
		Amount:         input.Amount,
		SourceType:     input.SourceType,
		SourceID:       input.SourceID,
		IdempotencyKey: input.IdempotencyKey,
		Authority:      input.Authority,
		RoleXP:         append([]progression.RoleXPGrant(nil), input.RoleXP...),
	})
	if err != nil {
		return discovery.ScanXPGrantResult{}, err
	}
	return discovery.ScanXPGrantResult{Duplicate: result.Duplicate}, nil
}

var _ discovery.ScannerModuleProvider = runtimeScannerModuleProvider{}
var _ discovery.ScannerStatsProvider = runtimeScannerStatsProvider{}
var _ discovery.ScannerPositionProvider = runtimeScannerPositionProvider{}
var _ discovery.ScannerCooldownProvider = runtimeScannerCooldownProvider{}
var _ discovery.ScannerEnergyProvider = runtimeScannerEnergyProvider{}
var _ discovery.ScannerPlayerRevealProvider = runtimeScannerPlayerRevealProvider{}
var _ discovery.ScanXPGrantProvider = runtimeScanXPProvider{}
