package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

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

	defaultCandidateMapID          = "default_map"
	defaultCandidateProfileVersion = "bounded_v1"
	defaultCandidateLevelMin       = 1
	defaultCandidateLevelMax       = 6
	defaultCandidateDensity        = 1
	exactCandidateMinCoordinate    = 0
	exactCandidateMaxCoordinate    = 10000
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
	MapID          string
	ProfileVersion string
	MapBounds      CandidateMapBounds
	LevelMin       int
	LevelMax       int
	Density        float64

	// DiscoveryHorizon is deprecated. Bounded map-local generation ignores it.
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
	mapID         string
	profile       string
	cell          ScanCellCoord
	chunk         ChunkCoord
	bounds        CandidateMapBounds
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

// CandidateMapBounds describes inclusive map-local scanner bounds.
type CandidateMapBounds struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
}

// ExactCandidateMapBounds returns the required 0..10000 scanner rectangle.
func ExactCandidateMapBounds() CandidateMapBounds {
	return CandidateMapBounds{
		MinX: exactCandidateMinCoordinate,
		MinY: exactCandidateMinCoordinate,
		MaxX: exactCandidateMaxCoordinate,
		MaxY: exactCandidateMaxCoordinate,
	}
}

// IsZero reports whether bounds were omitted from options.
func (bounds CandidateMapBounds) IsZero() bool {
	return bounds == CandidateMapBounds{}
}

// ValidateExactPlayable reports whether bounds match the current map contract.
func (bounds CandidateMapBounds) ValidateExactPlayable() error {
	values := []float64{bounds.MinX, bounds.MinY, bounds.MaxX, bounds.MaxY}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("map bounds %+v: %w", bounds, ErrInvalidCandidateOptions)
		}
	}
	if bounds != ExactCandidateMapBounds() {
		return fmt.Errorf("map bounds %+v must equal 0..10000: %w", bounds, ErrInvalidCandidateOptions)
	}
	return nil
}

// Contains reports whether position is inside inclusive map-local bounds.
func (bounds CandidateMapBounds) Contains(position world.Vec2) bool {
	if err := position.Validate(); err != nil {
		return false
	}
	return position.X >= bounds.MinX &&
		position.Y >= bounds.MinY &&
		position.X <= bounds.MaxX &&
		position.Y <= bounds.MaxY
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
	if !normalized.cellMayIntersectBounds(cell) {
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
	if normalized.density < defaultCandidateDensity {
		densityHash, err := CellHash(seed, cell, normalized.profilePurpose("planet_candidate_density"))
		if err != nil {
			return nil, err
		}
		if unitFloatFromHash(densityHash) > normalized.density {
			return nil, nil
		}
	}

	candidates := make([]PlanetCandidate, 0, count)
	for index := 0; index < count; index++ {
		candidate, err := buildPlanetCandidate(seed, cell, biome, index, normalized)
		if err != nil {
			return nil, err
		}
		if !normalized.bounds.Contains(candidate.position) {
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
		ApproxDistance: approximateMapLocalSignal(candidate.position, candidate.bounds),
	}
}

type normalizedCandidateOptions struct {
	mapID          string
	profileVersion string
	bounds         CandidateMapBounds
	levelMin       int
	levelMax       int
	density        float64
	allowedBiomes  map[Biome]struct{}
	spawnBudget    int
	scanCellSize   float64
	chunkSize      float64
}

func normalizeCandidateOptions(options CandidateGenerationOptions) (normalizedCandidateOptions, error) {
	mapID := strings.TrimSpace(options.MapID)
	if mapID == "" {
		mapID = defaultCandidateMapID
	}
	if mapID != options.MapID && options.MapID != "" {
		return normalizedCandidateOptions{}, fmt.Errorf("map_id %q: %w", options.MapID, ErrInvalidCandidateOptions)
	}

	profileVersion := strings.TrimSpace(options.ProfileVersion)
	if profileVersion == "" {
		profileVersion = defaultCandidateProfileVersion
	}
	if profileVersion != options.ProfileVersion && options.ProfileVersion != "" {
		return normalizedCandidateOptions{}, fmt.Errorf("profile_version %q: %w", options.ProfileVersion, ErrInvalidCandidateOptions)
	}

	bounds := options.MapBounds
	if bounds.IsZero() {
		bounds = ExactCandidateMapBounds()
	}
	if err := bounds.ValidateExactPlayable(); err != nil {
		return normalizedCandidateOptions{}, err
	}

	levelMin := options.LevelMin
	levelMax := options.LevelMax
	if levelMin == 0 && levelMax == 0 {
		levelMin = defaultCandidateLevelMin
		levelMax = defaultCandidateLevelMax
	}
	if levelMin <= 0 || levelMax < levelMin {
		return normalizedCandidateOptions{}, fmt.Errorf("level band %d..%d: %w", levelMin, levelMax, ErrInvalidCandidateOptions)
	}

	density := options.Density
	if density == 0 {
		density = defaultCandidateDensity
	}
	if density < 0 || density > 1 || math.IsNaN(density) || math.IsInf(density, 0) {
		return normalizedCandidateOptions{}, fmt.Errorf("density %v: %w", options.Density, ErrInvalidCandidateOptions)
	}

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
		mapID:          mapID,
		profileVersion: profileVersion,
		bounds:         bounds,
		levelMin:       levelMin,
		levelMax:       levelMax,
		density:        density,
		allowedBiomes:  allowedBiomes,
		spawnBudget:    options.SpawnBudget,
		scanCellSize:   scanCellSize,
		chunkSize:      chunkSize,
	}, nil
}

func (options normalizedCandidateOptions) allowsBiome(biome Biome) bool {
	if len(options.allowedBiomes) == 0 {
		return true
	}
	_, ok := options.allowedBiomes[biome]
	return ok
}

func (options normalizedCandidateOptions) cellMayIntersectBounds(cell ScanCellCoord) bool {
	cellMinX := float64(cell.X) * options.scanCellSize
	cellMinY := float64(cell.Y) * options.scanCellSize
	cellMaxX := cellMinX + options.scanCellSize
	cellMaxY := cellMinY + options.scanCellSize
	return cellMaxX >= options.bounds.MinX &&
		cellMaxY >= options.bounds.MinY &&
		cellMinX <= options.bounds.MaxX &&
		cellMinY <= options.bounds.MaxY
}

func (options normalizedCandidateOptions) profilePurpose(purpose string) string {
	return purpose + ":" + options.profileVersion
}

func (options normalizedCandidateOptions) mapProfilePurpose(purpose string) string {
	return purpose + ":" + options.mapID + ":" + options.profileVersion
}

func (options normalizedCandidateOptions) levelFromHash(hash uint64) int {
	span := options.levelMax - options.levelMin + 1
	return options.levelMin + int(hash%uint64(span))
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
	key, err := indexedCellHash(seed, cell, options.mapProfilePurpose("planet_candidate_key"), index)
	if err != nil {
		return PlanetCandidate{}, err
	}
	rarityHash, err := indexedCellHash(seed, cell, "planet_candidate_rarity", index)
	if err != nil {
		return PlanetCandidate{}, err
	}
	levelHash, err := indexedCellHash(seed, cell, options.profilePurpose("planet_candidate_level"), index)
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

	rarity := candidateRarity(rarityHash)
	level := options.levelFromHash(levelHash)

	signature := 0.25 + unitFloatFromHash(rarityHash)*0.75
	return PlanetCandidate{
		key:           key,
		mapID:         options.mapID,
		profile:       options.profileVersion,
		cell:          cell,
		chunk:         chunk,
		bounds:        options.bounds,
		biome:         biome,
		position:      position,
		level:         level,
		signature:     signature,
		minRadarLevel: minRadarLevel(level, biome),
		rarity:        rarity,
	}, nil
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

func approximateMapLocalSignal(position world.Vec2, bounds CandidateMapBounds) string {
	width := bounds.MaxX - bounds.MinX
	height := bounds.MaxY - bounds.MinY
	if width <= 0 || height <= 0 {
		return "map_local"
	}
	center := world.Vec2{
		X: bounds.MinX + width/2,
		Y: bounds.MinY + height/2,
	}
	maxDistance := math.Sqrt(width*width+height*height) / 2
	distance := center.Distance(position)
	ratio := distance / maxDistance
	switch {
	case ratio < 0.34:
		return "map_core"
	case ratio < 0.67:
		return "map_mid"
	default:
		return "map_edge"
	}
}

var _ json.Marshaler = PlanetCandidate{}
