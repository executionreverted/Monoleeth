package server

import (
	"math"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

const runtimeHiddenPlayerStealthScore = 125

func (runtime *Runtime) visibilityStatSnapshotLocked(playerID foundation.PlayerID, now time.Time) stats.StatSnapshot {
	state, ok := runtime.players[playerID]
	exploration := stats.ExplorationStats{RadarRange: defaultRadarRange}
	if ok {
		exploration = runtime.explorationStatsForPlayerStateLocked(state)
	}
	return stats.NewStatSnapshot(playerID, starterShipID, 1, stats.EffectiveStats{
		Exploration: exploration,
	}, now)
}

func (runtime *Runtime) explorationStatsForPlayerStateLocked(state playerRuntimeState) stats.ExplorationStats {
	return stats.ExplorationStats{
		RadarRange:            positiveOrDefault(state.Stats.RadarRange, defaultRadarRange),
		DetectionPower:        nonNegativeFinite(state.Stats.DetectionPower),
		JammerResistance:      nonNegativeFinite(state.Stats.JammerResistance),
		StealthDetectionBonus: nonNegativeFinite(state.Stats.StealthDetectionBonus),
	}
}

func (runtime *Runtime) visibilityInputsForEntityLocked(entity world.Entity, playerID foundation.PlayerID, hidden bool) (visibility.EntitySignature, float64, float64) {
	signature := runtime.signatureForEntityLocked(entity, playerID)
	stealthScore := 0.0
	jammerStrength := 0.0
	if hidden {
		stealthScore = stealthScoreForHiddenEntity(entity.Type, signature)
	}
	return signature, stealthScore, jammerStrength
}

func (runtime *Runtime) signatureForEntityLocked(entity world.Entity, playerID foundation.PlayerID) visibility.EntitySignature {
	if entity.Type == world.EntityTypePlayer && !playerID.IsZero() {
		return runtime.playerSignatureLocked(playerID)
	}
	return visibility.SignatureForEntityType(entity.Type)
}

func (runtime *Runtime) playerSignatureLocked(playerID foundation.PlayerID) visibility.EntitySignature {
	state, ok := runtime.players[playerID]
	if !ok || state.Ship.ActiveShipID == "" {
		return visibility.EntitySignaturePlayer
	}
	definition, err := runtime.ShipCatalog.MustGet(foundation.ShipID(state.Ship.ActiveShipID))
	if err != nil || definition.BaseStats.Signature <= 0 {
		return visibility.EntitySignaturePlayer
	}
	return visibility.EntitySignature(definition.BaseStats.Signature)
}

func stealthScoreForHiddenEntity(entityType world.EntityType, signature visibility.EntitySignature) float64 {
	if entityType == world.EntityTypePlayer {
		return runtimeHiddenPlayerStealthScore
	}
	return signature.Units() + 25
}

func positiveOrDefault(value float64, fallback float64) float64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return fallback
	}
	return value
}

func nonNegativeFinite(value float64) float64 {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}
