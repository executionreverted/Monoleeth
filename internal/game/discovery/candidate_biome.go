package discovery

import (
	"math"
)

// Biome identifies a coarse procedural world region classification.
type Biome string

const (
	BiomeOriginBelt Biome = "origin_belt"
	BiomeOuterDrift Biome = "outer_drift"
	BiomeNebula     Biome = "nebula"
	BiomeDeepSpace  Biome = "deep_space"
	BiomeDeadZone   Biome = "dead_zone"
)

// ClassifyBiome returns a deterministic biome skeleton for a scan cell.
func ClassifyBiome(seed WorldSeed, cell ScanCellCoord) (Biome, error) {
	center, err := cell.Center(DefaultScanCellSize)
	if err != nil {
		return "", err
	}
	noiseHash, err := CellHash(seed, cell, "biome")
	if err != nil {
		return "", err
	}

	distance := math.Sqrt(center.X*center.X + center.Y*center.Y)
	noise := unitFloatFromHash(noiseHash)

	switch {
	case distance > 50_000 && noise < 0.16:
		return BiomeDeadZone, nil
	case distance > 1_500 && noise > 0.92:
		return BiomeNebula, nil
	case distance > 20_000:
		return BiomeDeepSpace, nil
	case distance > 4_000:
		return BiomeOuterDrift, nil
	default:
		return BiomeOriginBelt, nil
	}
}

func biomeSpawnBudget(biome Biome) int {
	switch biome {
	case BiomeOriginBelt:
		return 1
	case BiomeOuterDrift:
		return 2
	case BiomeNebula:
		return 2
	case BiomeDeepSpace:
		return 3
	case BiomeDeadZone:
		return 1
	default:
		return 0
	}
}

func biomeLevelModifier(biome Biome) int {
	switch biome {
	case BiomeOriginBelt:
		return 0
	case BiomeOuterDrift:
		return 1
	case BiomeNebula:
		return 2
	case BiomeDeepSpace:
		return 4
	case BiomeDeadZone:
		return 6
	default:
		return 0
	}
}
