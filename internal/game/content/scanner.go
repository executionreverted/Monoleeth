package content

import (
	"fmt"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	worldmaps "gameproject/internal/game/world/maps"
)

const (
	DefaultScannerProfileVersion = "runtime_phase06_bounded_v1"
	DefaultScannerStaticSeed     = "phase07-static-seed"
	DefaultScannerRadarLevelUnit = 420
	DefaultScannerDiscoveryXP    = 25
)

// ScannerContent is server-only scanner/planet generation config. Static seed
// material and candidate options must never be sent to clients.
type ScannerContent struct {
	StaticSeed        []byte
	CandidateOptions  discovery.CandidateGenerationOptions
	MapProfiles       []ScannerMapProfile
	RadarLevelUnit    float64
	DiscoveryXPAmount int64
}

type ScannerMapProfile struct {
	MapID          worldmaps.MapID
	ProfileVersion string
	LevelMin       int
	LevelMax       int
	Density        float64
	SpawnBudget    int
	AllowedBiomes  []discovery.Biome
}

func DefaultScannerContent() ScannerContent {
	bounds := worldmaps.ExactPlayableBounds()
	base := discovery.CandidateGenerationOptions{
		ProfileVersion: DefaultScannerProfileVersion,
		MapBounds: discovery.CandidateMapBounds{
			MinX: bounds.MinX,
			MinY: bounds.MinY,
			MaxX: bounds.MaxX,
			MaxY: bounds.MaxY,
		},
		LevelMin:     1,
		LevelMax:     4,
		Density:      1,
		SpawnBudget:  8,
		ScanCellSize: discovery.DefaultScanCellSize,
	}
	return ScannerContent{
		StaticSeed:       []byte(DefaultScannerStaticSeed),
		CandidateOptions: base,
		MapProfiles: []ScannerMapProfile{
			newScannerMapProfile(worldmaps.StarterMapID, "scanner_1_1_origin_v1", 1, 3, 1, 4),
			newScannerMapProfile("map_1_2", "scanner_1_2_outer_ring_v1", 1, 4, 1, 5),
			newScannerMapProfile("map_1_3", DefaultScannerProfileVersion, 1, 4, 1, 6),
		},
		RadarLevelUnit:    DefaultScannerRadarLevelUnit,
		DiscoveryXPAmount: DefaultScannerDiscoveryXP,
	}
}

func newScannerMapProfile(mapID worldmaps.MapID, profileVersion string, levelMin int, levelMax int, density float64, spawnBudget int) ScannerMapProfile {
	return ScannerMapProfile{
		MapID:          mapID,
		ProfileVersion: profileVersion,
		LevelMin:       levelMin,
		LevelMax:       levelMax,
		Density:        density,
		SpawnBudget:    spawnBudget,
	}
}

func (content ScannerContent) Validate(mapCatalog *worldmaps.Catalog) error {
	seed, err := content.WorldSeed()
	if err != nil {
		return err
	}
	if content.RadarLevelUnit <= 0 {
		return fmt.Errorf("radar level unit %v: %w", content.RadarLevelUnit, ErrInvalidScannerContent)
	}
	if content.DiscoveryXPAmount <= 0 {
		return fmt.Errorf("discovery xp amount %d: %w", content.DiscoveryXPAmount, ErrInvalidScannerContent)
	}
	if _, err := discovery.GeneratePlanetCandidates(seed, discovery.ScanCellCoord{}, content.CandidateOptions); err != nil {
		return fmt.Errorf("candidate options: %w", err)
	}
	seenProfiles := make(map[worldmaps.MapID]struct{}, len(content.MapProfiles))
	for _, profile := range content.MapProfiles {
		if _, exists := seenProfiles[profile.MapID]; exists {
			return fmt.Errorf("scanner map profile %q: %w", profile.MapID, ErrInvalidScannerContent)
		}
		seenProfiles[profile.MapID] = struct{}{}
		if mapCatalog != nil {
			if _, ok := mapCatalog.Get(profile.MapID); !ok {
				return fmt.Errorf("scanner map profile %q: %w", profile.MapID, ErrInvalidScannerContent)
			}
		}
		options := content.candidateOptionsForProfile(profile)
		if _, err := discovery.GeneratePlanetCandidates(seed, discovery.ScanCellCoord{}, options); err != nil {
			return fmt.Errorf("scanner map profile %q: %w", profile.MapID, err)
		}
	}
	return nil
}

func (content ScannerContent) WorldSeed() (discovery.WorldSeed, error) {
	seed, err := discovery.NewWorldSeed(discovery.WorldSeedInput{StaticSeed: append([]byte(nil), content.StaticSeed...)})
	if err != nil {
		return discovery.WorldSeed{}, fmt.Errorf("scanner seed: %w", err)
	}
	return seed, nil
}

func (content ScannerContent) CandidateOptionsForRuntime(e2eNoPlanetSeed bool) discovery.CandidateGenerationOptions {
	options := content.CandidateOptions
	options.AllowedBiomes = append([]discovery.Biome(nil), content.CandidateOptions.AllowedBiomes...)
	if e2eNoPlanetSeed {
		options.AllowedBiomes = []discovery.Biome{"e2e_no_planet"}
	}
	return options
}

func (content ScannerContent) CandidateOptionsForZone(zoneID foundation.ZoneID) (discovery.CandidateGenerationOptions, bool) {
	mapID := worldmaps.MapID(zoneID.String())
	for _, profile := range content.MapProfiles {
		if profile.MapID == mapID {
			return content.candidateOptionsForProfile(profile), true
		}
	}
	options := content.CandidateOptions
	options.MapID = zoneID.String()
	options.AllowedBiomes = append([]discovery.Biome(nil), content.CandidateOptions.AllowedBiomes...)
	return options, false
}

func (content ScannerContent) candidateOptionsForProfile(profile ScannerMapProfile) discovery.CandidateGenerationOptions {
	options := content.CandidateOptions
	options.MapID = profile.MapID.String()
	options.ProfileVersion = profile.ProfileVersion
	options.LevelMin = profile.LevelMin
	options.LevelMax = profile.LevelMax
	options.Density = profile.Density
	options.SpawnBudget = profile.SpawnBudget
	options.AllowedBiomes = append([]discovery.Biome(nil), profile.AllowedBiomes...)
	return options
}
