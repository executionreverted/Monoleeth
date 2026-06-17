package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"gameproject/internal/game/world"
)

var (
	// ErrInvalidCandidateOptions reports malformed candidate generation input.
	ErrInvalidCandidateOptions = errors.New("invalid candidate generation options")
	// ErrHiddenCandidateSerialization reports attempted JSON serialization of
	// hidden server-only planet candidate truth.
	ErrHiddenCandidateSerialization = errors.New("hidden planet candidate cannot be serialized")
)

// CandidateRarity is a deterministic candidate classification used by later
// scanner and planet-materialization systems.
type CandidateRarity string

const (
	CandidateRarityCommon   CandidateRarity = "common"
	CandidateRarityUncommon CandidateRarity = "uncommon"
	CandidateRarityRare     CandidateRarity = "rare"
)

// SignalBand is a coarse client-safe signal strength label for revealed
// scanner signals. It does not include hidden coordinates or candidate keys.
type SignalBand string

const (
	SignalBandWeak     SignalBand = "weak"
	SignalBandModerate SignalBand = "moderate"
	SignalBandStrong   SignalBand = "strong"
)

// CandidateGenerationOptions controls deterministic hidden planet generation.
type CandidateGenerationOptions struct {
	DiscoveryHorizon float64
	AllowedBiomes    []Biome
	SpawnBudget      int
	ScanCellSize     float64
	ChunkSize        float64
}

// PlanetCandidate is hidden server-only procedural truth. It intentionally
// lacks exported JSON fields and fails closed if marshaled directly.
type PlanetCandidate struct {
	key           uint64
	cell          ScanCellCoord
	chunk         ChunkCoord
	biome         Biome
	position      world.Vec2
	level         int
	signature     float64
	minRadarLevel int
	rarity        CandidateRarity
}

// CandidateSignalProjection is the safe shape a future scanner service can
// emit after it has server-side permission to reveal a generic signal.
type CandidateSignalProjection struct {
	Biome          Biome      `json:"biome"`
	SignalBand     SignalBand `json:"signal_band"`
	ApproxDistance string     `json:"approx_distance"`
}

// GeneratePlanetCandidates returns deterministic hidden candidates for one
// scan cell. Results are computed in memory only and are not persisted.
func GeneratePlanetCandidates(seed WorldSeed, cell ScanCellCoord, options CandidateGenerationOptions) ([]PlanetCandidate, error) {
	normalized, err := normalizeCandidateOptions(options)
	if err != nil {
		return nil, err
	}
	if !seed.Valid() {
		return nil, ErrInvalidWorldSeed
	}
	if normalized.spawnBudget <= 0 {
		return nil, nil
	}

	biome, err := ClassifyBiome(seed, cell)
	if err != nil {
		return nil, err
	}
	if !normalized.allowsBiome(biome) {
		return nil, nil
	}

	biomeBudget := biomeSpawnBudget(biome)
	if biomeBudget <= 0 {
		return nil, nil
	}

	countHash, err := CellHash(seed, cell, "planet_candidate_count")
	if err != nil {
		return nil, err
	}
	count := int(countHash % uint64(biomeBudget+1))
	if count > normalized.spawnBudget {
		count = normalized.spawnBudget
	}
	if count == 0 {
		return nil, nil
	}

	candidates := make([]PlanetCandidate, 0, count)
	for index := 0; index < count; index++ {
		candidate, err := buildPlanetCandidate(seed, cell, biome, index, normalized)
		if err != nil {
			return nil, err
		}
		if distanceFromOrigin(candidate.position) > normalized.discoveryHorizon {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

// MarshalJSON fails closed so hidden candidates cannot accidentally become
// client payloads.
func (candidate PlanetCandidate) MarshalJSON() ([]byte, error) {
	return nil, ErrHiddenCandidateSerialization
}

// Key returns the deterministic server-only candidate key.
func (candidate PlanetCandidate) Key() uint64 {
	return candidate.key
}

// Cell returns the scan cell that generated candidate.
func (candidate PlanetCandidate) Cell() ScanCellCoord {
	return candidate.cell
}

// Chunk returns the chunk containing candidate.
func (candidate PlanetCandidate) Chunk() ChunkCoord {
	return candidate.chunk
}

// Biome returns candidate's biome classification.
func (candidate PlanetCandidate) Biome() Biome {
	return candidate.biome
}

// Position returns candidate's exact hidden world position.
func (candidate PlanetCandidate) Position() world.Vec2 {
	return candidate.position
}

// Level returns candidate's generated level skeleton.
func (candidate PlanetCandidate) Level() int {
	return candidate.level
}

// Signature returns candidate's scanner signature skeleton.
func (candidate PlanetCandidate) Signature() float64 {
	return candidate.signature
}

// MinRadarLevel returns the minimum radar level needed by future scan logic.
func (candidate PlanetCandidate) MinRadarLevel() int {
	return candidate.minRadarLevel
}

// Rarity returns candidate's deterministic rarity skeleton.
func (candidate PlanetCandidate) Rarity() CandidateRarity {
	return candidate.rarity
}

// ClientSafeSignal projects candidate into a client-safe revealed signal shell.
func (candidate PlanetCandidate) ClientSafeSignal() CandidateSignalProjection {
	return CandidateSignalProjection{
		Biome:          candidate.biome,
		SignalBand:     signalBand(candidate.signature),
		ApproxDistance: approximateDistance(candidate.position),
	}
}

type normalizedCandidateOptions struct {
	discoveryHorizon float64
	allowedBiomes    map[Biome]struct{}
	spawnBudget      int
	scanCellSize     float64
	chunkSize        float64
}

func normalizeCandidateOptions(options CandidateGenerationOptions) (normalizedCandidateOptions, error) {
	scanCellSize := options.ScanCellSize
	if scanCellSize == 0 {
		scanCellSize = DefaultScanCellSize
	}
	if err := validateGridSize(scanCellSize); err != nil {
		return normalizedCandidateOptions{}, err
	}

	chunkSize := options.ChunkSize
	if chunkSize == 0 {
		chunkSize = DefaultChunkSize
	}
	if err := validateGridSize(chunkSize); err != nil {
		return normalizedCandidateOptions{}, err
	}

	if options.DiscoveryHorizon < 0 || math.IsNaN(options.DiscoveryHorizon) || math.IsInf(options.DiscoveryHorizon, 0) {
		return normalizedCandidateOptions{}, fmt.Errorf("discovery horizon %v: %w", options.DiscoveryHorizon, ErrInvalidCandidateOptions)
	}

	allowedBiomes := make(map[Biome]struct{}, len(options.AllowedBiomes))
	for _, biome := range options.AllowedBiomes {
		allowedBiomes[biome] = struct{}{}
	}

	return normalizedCandidateOptions{
		discoveryHorizon: options.DiscoveryHorizon,
		allowedBiomes:    allowedBiomes,
		spawnBudget:      options.SpawnBudget,
		scanCellSize:     scanCellSize,
		chunkSize:        chunkSize,
	}, nil
}

func (options normalizedCandidateOptions) allowsBiome(biome Biome) bool {
	if len(options.allowedBiomes) == 0 {
		return true
	}
	_, ok := options.allowedBiomes[biome]
	return ok
}

func buildPlanetCandidate(seed WorldSeed, cell ScanCellCoord, biome Biome, index int, options normalizedCandidateOptions) (PlanetCandidate, error) {
	offsetXHash, err := indexedCellHash(seed, cell, "planet_candidate_offset_x", index)
	if err != nil {
		return PlanetCandidate{}, err
	}
	offsetYHash, err := indexedCellHash(seed, cell, "planet_candidate_offset_y", index)
	if err != nil {
		return PlanetCandidate{}, err
	}
	key, err := indexedCellHash(seed, cell, "planet_candidate_key", index)
	if err != nil {
		return PlanetCandidate{}, err
	}
	rarityHash, err := indexedCellHash(seed, cell, "planet_candidate_rarity", index)
	if err != nil {
		return PlanetCandidate{}, err
	}

	position := world.Vec2{
		X: (float64(cell.X) + unitFloatFromHash(offsetXHash)) * options.scanCellSize,
		Y: (float64(cell.Y) + unitFloatFromHash(offsetYHash)) * options.scanCellSize,
	}
	chunk, err := ChunkCoordForPosition(position, options.chunkSize)
	if err != nil {
		return PlanetCandidate{}, err
	}

	distance := distanceFromOrigin(position)
	rarity := candidateRarity(rarityHash)
	level := int(math.Log(distance/2_000+1)*4) + biomeLevelModifier(biome) + rarityLevelModifier(rarity) + 1
	if level < 1 {
		level = 1
	}

	signature := 0.25 + unitFloatFromHash(rarityHash)*0.75
	return PlanetCandidate{
		key:           key,
		cell:          cell,
		chunk:         chunk,
		biome:         biome,
		position:      position,
		level:         level,
		signature:     signature,
		minRadarLevel: minRadarLevel(level, biome),
		rarity:        rarity,
	}, nil
}

func distanceFromOrigin(position world.Vec2) float64 {
	return math.Sqrt(position.X*position.X + position.Y*position.Y)
}

func candidateRarity(hash uint64) CandidateRarity {
	roll := unitFloatFromHash(hash)
	switch {
	case roll > 0.985:
		return CandidateRarityRare
	case roll > 0.88:
		return CandidateRarityUncommon
	default:
		return CandidateRarityCommon
	}
}

func rarityLevelModifier(rarity CandidateRarity) int {
	switch rarity {
	case CandidateRarityRare:
		return 3
	case CandidateRarityUncommon:
		return 1
	default:
		return 0
	}
}

func minRadarLevel(level int, biome Biome) int {
	radarLevel := 1 + level/5
	if biome == BiomeNebula || biome == BiomeDeadZone {
		radarLevel++
	}
	if radarLevel < 1 {
		return 1
	}
	return radarLevel
}

func signalBand(signature float64) SignalBand {
	switch {
	case signature >= 0.75:
		return SignalBandStrong
	case signature >= 0.5:
		return SignalBandModerate
	default:
		return SignalBandWeak
	}
}

func approximateDistance(position world.Vec2) string {
	distance := distanceFromOrigin(position)
	switch {
	case distance < 5_000:
		return "near_origin"
	case distance < 20_000:
		return "outer_drift"
	case distance < 50_000:
		return "deep_space"
	default:
		return "frontier"
	}
}

var _ json.Marshaler = PlanetCandidate{}
