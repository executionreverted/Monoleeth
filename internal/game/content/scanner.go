package content

import (
	"fmt"

	"gameproject/internal/game/discovery"
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
	RadarLevelUnit    float64
	DiscoveryXPAmount int64
}

func DefaultScannerContent() ScannerContent {
	bounds := worldmaps.ExactPlayableBounds()
	return ScannerContent{
		StaticSeed: []byte(DefaultScannerStaticSeed),
		CandidateOptions: discovery.CandidateGenerationOptions{
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
		},
		RadarLevelUnit:    DefaultScannerRadarLevelUnit,
		DiscoveryXPAmount: DefaultScannerDiscoveryXP,
	}
}

func (content ScannerContent) Validate() error {
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
