package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/stats"
)

// Validate reports whether ref is a non-empty server-generated pulse reference.
func (ref ScanPulseReference) Validate() error {
	if !validDiscoveryToken(string(ref)) {
		return fmt.Errorf("pulse_reference %q: %w", ref, ErrInvalidScanPulse)
	}
	return nil
}

// Validate reports whether input contains the required server-resolved IDs.
func (input StartScanPulseInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.WorldID.Validate(); err != nil {
		return fmt.Errorf("world_id: %w", err)
	}
	if err := input.ZoneID.Validate(); err != nil {
		return fmt.Errorf("zone_id: %w", err)
	}
	if err := input.ShipID.Validate(); err != nil {
		return fmt.Errorf("ship_id: %w", err)
	}
	if err := input.PulseReference.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether input identifies one existing server pulse.
func (input ResolveScanPulseInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.WorldID.Validate(); err != nil {
		return fmt.Errorf("world_id: %w", err)
	}
	if err := input.ZoneID.Validate(); err != nil {
		return fmt.Errorf("zone_id: %w", err)
	}
	if err := input.PulseReference.Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateFor reports whether position belongs to the requested server context.
func (position ScannerPosition) ValidateFor(_ foundation.PlayerID, worldID foundation.WorldID, zoneID foundation.ZoneID) error {
	if position.WorldID != worldID || position.ZoneID != zoneID {
		return ErrScannerUnavailable
	}
	if err := position.WorldID.Validate(); err != nil {
		return fmt.Errorf("position_world_id: %w", err)
	}
	if err := position.ZoneID.Validate(); err != nil {
		return fmt.Errorf("position_zone_id: %w", err)
	}
	if err := position.Position.Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateStationaryForScan reports whether the server-owned movement state
// permits a scanner pulse to begin.
func (position ScannerPosition) ValidateStationaryForScan() error {
	if err := position.Movement.Validate(); err != nil {
		return err
	}
	if position.Movement.Moving {
		return ErrScanMovementRestricted
	}
	return nil
}

// Validate reports whether input is a well-formed scanner energy check.
func (input ScannerEnergyInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.ShipID.Validate(); err != nil {
		return fmt.Errorf("ship_id: %w", err)
	}
	if err := input.WorldID.Validate(); err != nil {
		return fmt.Errorf("world_id: %w", err)
	}
	if err := input.ZoneID.Validate(); err != nil {
		return fmt.Errorf("zone_id: %w", err)
	}
	if err := input.PulseReference.Validate(); err != nil {
		return err
	}
	if input.CheckedAt.IsZero() {
		return fmt.Errorf("checked_at: %w", ErrInvalidScanPulse)
	}
	values := []float64{
		input.Stats.Core.EnergyMax,
		input.Stats.Core.EnergyRegen,
	}
	for _, value := range values {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return ErrInvalidScannerStats
		}
	}
	return nil
}

// Validate reports whether input is a progression-compatible scan XP grant.
func (input ScanXPGrantInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if input.Amount <= 0 {
		return fmt.Errorf("amount %d: %w", input.Amount, ErrInvalidScannerXPGrant)
	}
	if input.SourceType != progression.XPSourceTypeScan {
		return fmt.Errorf("source_type %q: %w", input.SourceType, ErrInvalidScannerXPGrant)
	}
	if err := input.SourceType.Validate(); err != nil {
		return err
	}
	if err := input.Authority.ValidateForSource(input.SourceType); err != nil {
		return err
	}
	if err := input.SourceID.Validate(); err != nil {
		return err
	}
	if err := input.IdempotencyKey.Validate(); err != nil {
		return err
	}
	for _, grant := range input.RoleXP {
		if err := grant.Role.Validate(); err != nil {
			return err
		}
		if grant.Amount <= 0 {
			return fmt.Errorf("role_xp amount %d: %w", grant.Amount, ErrInvalidScannerXPGrant)
		}
	}
	return nil
}

func normalizeScannerConfig(config ScannerServiceConfig) (ScannerServiceConfig, error) {
	if !config.WorldSeed.Valid() {
		return ScannerServiceConfig{}, fmt.Errorf("world_seed: %w", ErrInvalidScannerConfig)
	}
	if config.Modules == nil {
		return ScannerServiceConfig{}, fmt.Errorf("modules: %w", ErrInvalidScannerConfig)
	}
	if config.Stats == nil {
		return ScannerServiceConfig{}, fmt.Errorf("stats: %w", ErrInvalidScannerConfig)
	}
	if config.Positions == nil {
		return ScannerServiceConfig{}, fmt.Errorf("positions: %w", ErrInvalidScannerConfig)
	}
	if config.Cooldowns == nil {
		return ScannerServiceConfig{}, fmt.Errorf("cooldowns: %w", ErrInvalidScannerConfig)
	}
	if config.Energy == nil {
		return ScannerServiceConfig{}, fmt.Errorf("energy: %w", ErrInvalidScannerConfig)
	}
	if config.XP == nil {
		return ScannerServiceConfig{}, fmt.Errorf("xp: %w", ErrInvalidScannerConfig)
	}
	if config.Store == nil {
		config.Store = NewInMemoryStore()
	}
	if config.Clock == nil {
		config.Clock = foundation.RealClock{}
	}
	if config.ScanCellSize == 0 {
		config.ScanCellSize = DefaultScanCellSize
	}
	if err := validateGridSize(config.ScanCellSize); err != nil {
		return ScannerServiceConfig{}, err
	}
	if config.ChunkSize == 0 {
		config.ChunkSize = DefaultChunkSize
	}
	if err := validateGridSize(config.ChunkSize); err != nil {
		return ScannerServiceConfig{}, err
	}
	if config.RadarLevelUnit == 0 {
		config.RadarLevelUnit = DefaultScanCellSize
	}
	if err := validateGridSize(config.RadarLevelUnit); err != nil {
		return ScannerServiceConfig{}, err
	}
	if config.DiscoveryXPAmount == 0 {
		config.DiscoveryXPAmount = defaultScannerXPAmount
	}
	if config.DiscoveryXPAmount < 0 {
		return ScannerServiceConfig{}, fmt.Errorf("discovery_xp_amount %d: %w", config.DiscoveryXPAmount, ErrInvalidScannerConfig)
	}
	if config.CandidateOptions.DiscoveryHorizon == 0 {
		config.CandidateOptions.DiscoveryHorizon = defaultScannerDiscoveryHorizon
	}
	if config.CandidateOptions.SpawnBudget == 0 {
		config.CandidateOptions.SpawnBudget = defaultScannerSpawnBudget
	}
	if config.CandidateOptions.ScanCellSize == 0 {
		config.CandidateOptions.ScanCellSize = config.ScanCellSize
	}
	if config.CandidateOptions.ChunkSize == 0 {
		config.CandidateOptions.ChunkSize = config.ChunkSize
	}
	if _, err := normalizeCandidateOptions(config.CandidateOptions); err != nil {
		return ScannerServiceConfig{}, err
	}
	return config, nil
}

func validateScannerSnapshot(snapshot stats.StatSnapshot, playerID foundation.PlayerID, shipID foundation.ShipID) (stats.EffectiveStats, error) {
	if snapshot.PlayerID != playerID || snapshot.ShipID != shipID || snapshot.IsInvalidated() {
		return stats.EffectiveStats{}, ErrScannerUnavailable
	}
	if err := snapshot.PlayerID.Validate(); err != nil {
		return stats.EffectiveStats{}, err
	}
	if err := snapshot.ShipID.Validate(); err != nil {
		return stats.EffectiveStats{}, err
	}
	values := []float64{
		snapshot.Stats.Exploration.RadarRange,
		snapshot.Stats.Exploration.ScanPower,
		snapshot.Stats.Exploration.ScanRadius,
		snapshot.Stats.Exploration.ScanInterval,
		snapshot.Stats.Core.EnergyMax,
		snapshot.Stats.Core.EnergyRegen,
	}
	for _, value := range values {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return stats.EffectiveStats{}, ErrInvalidScannerStats
		}
	}
	return snapshot.Stats, nil
}

func scannerCooldownDuration(effective stats.EffectiveStats) time.Duration {
	if effective.Exploration.ScanInterval <= 0 {
		return time.Second
	}
	return time.Duration(effective.Exploration.ScanInterval * float64(time.Second))
}

func scannerRadarLevel(effective stats.EffectiveStats, radarLevelUnit float64) int {
	if radarLevelUnit <= 0 || effective.Exploration.RadarRange <= 0 {
		return 0
	}
	level := int(math.Floor(effective.Exploration.RadarRange / radarLevelUnit))
	if level < 0 {
		return 0
	}
	return level
}

func scannerDetectionChance(effective stats.EffectiveStats, candidate PlanetCandidate, distance float64) float64 {
	if effective.Exploration.ScanPower <= 0 || effective.Exploration.ScanRadius <= 0 {
		return 0
	}
	distanceFactor := 1 - (distance / effective.Exploration.ScanRadius * 0.5)
	if distanceFactor < 0 {
		return 0
	}
	difficulty := float64(candidate.Level()) * 20
	if difficulty < 20 {
		difficulty = 20
	}
	chance := (effective.Exploration.ScanPower * candidate.Signature() * distanceFactor) / difficulty
	if chance > 1 {
		return 1
	}
	if chance < 0 {
		return 0
	}
	return chance
}

func scannerDetectionRoll(seed WorldSeed, pulse scanPulse, candidate PlanetCandidate) (float64, error) {
	purpose := "scanner_detection:" + string(pulse.reference) + ":" + strconv.FormatUint(candidate.Key(), 36)
	hash, err := CellHash(seed, pulse.cell, purpose)
	if err != nil {
		return 0, err
	}
	return unitFloatFromHash(hash), nil
}

func scanPulseMatchesStartInput(pulse scanPulse, input StartScanPulseInput) bool {
	return pulse.playerID == input.PlayerID &&
		pulse.worldID == input.WorldID &&
		pulse.zoneID == input.ZoneID &&
		pulse.shipID == input.ShipID
}

func scanPulseMatchesResolveInput(pulse scanPulse, input ResolveScanPulseInput) bool {
	return pulse.playerID == input.PlayerID &&
		pulse.worldID == input.WorldID &&
		pulse.zoneID == input.ZoneID
}

func scannerMaterializationKey(
	worldID foundation.WorldID,
	cell ScanCellCoord,
	candidate PlanetCandidate,
) PlanetMaterializationKey {
	digest := scannerDigest("materialization", worldID.String(), cell.X, cell.Y, candidate.Key())
	return PlanetMaterializationKey("candidate_" + hex.EncodeToString(digest[:12]))
}

func scannerPlanetID(key PlanetMaterializationKey) foundation.PlanetID {
	digest := scannerDigest("planet_id", string(key))
	return foundation.PlanetID("planet_" + hex.EncodeToString(digest[:10]))
}

func scannerPlanetType(key PlanetMaterializationKey) PlanetType {
	planetTypes := []PlanetType{
		PlanetTypeBarren,
		PlanetTypeTerrestrial,
		PlanetTypeIce,
		PlanetTypeOcean,
		PlanetTypeGasGiant,
	}
	digest := scannerDigest("planet_type", string(key))
	return planetTypes[int(digest[0])%len(planetTypes)]
}

func scannerPlanetBiome(biome Biome) PlanetBiome {
	switch biome {
	case BiomeOriginBelt:
		return PlanetBiomeOriginBelt
	case BiomeOuterDrift:
		return PlanetBiomeOuterDrift
	case BiomeNebula:
		return PlanetBiomeNebula
	case BiomeDeepSpace:
		return PlanetBiomeDeepSpace
	case BiomeDeadZone:
		return PlanetBiomeDeadZone
	default:
		return PlanetBiomeOriginBelt
	}
}

func scannerPlanetRarity(rarity CandidateRarity) PlanetRarity {
	switch rarity {
	case CandidateRarityRare:
		return PlanetRarityRare
	case CandidateRarityUncommon:
		return PlanetRarityUncommon
	default:
		return PlanetRarityCommon
	}
}

func scannerIntelConfidence(candidate PlanetCandidate) int {
	confidence := int(math.Round(candidate.Signature() * 100))
	if confidence < 60 {
		return 60
	}
	if confidence > 100 {
		return 100
	}
	return confidence
}

func newScannerEvent(
	eventType ScannerEventType,
	pulse scanPulse,
	planetID foundation.PlanetID,
	createdAt time.Time,
) ScannerEventRecord {
	digest := scannerDigest("scanner_event", string(eventType), string(pulse.reference), planetID.String())
	return ScannerEventRecord{
		EventID:        foundation.EventID("event_" + hex.EncodeToString(digest[:10])),
		Type:           eventType,
		PlayerID:       pulse.playerID,
		WorldID:        pulse.worldID,
		ZoneID:         pulse.zoneID,
		PulseReference: pulse.reference,
		PlanetID:       planetID,
		CreatedAt:      createdAt.UTC(),
	}
}

func scannerDigest(parts ...any) [sha256.Size]byte {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = fmt.Fprint(hash, part)
		_, _ = hash.Write([]byte{0})
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest
}

func cloneResolveScanPulseResult(result ResolveScanPulseResult) ResolveScanPulseResult {
	if result.Signal != nil {
		signal := *result.Signal
		result.Signal = &signal
	}
	return result
}
