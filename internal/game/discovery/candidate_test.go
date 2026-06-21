package discovery_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/world"
)

func TestCoordinateModelsUseFloorForNegativeWorldPositions(t *testing.T) {
	chunk, err := discovery.ChunkCoordForPosition(world.Vec2{X: -0.01, Y: -5_000.01}, discovery.DefaultChunkSize)
	if err != nil {
		t.Fatalf("ChunkCoordForPosition() error = %v", err)
	}
	if want := (discovery.ChunkCoord{X: -1, Y: -2}); chunk != want {
		t.Fatalf("chunk = %+v, want %+v", chunk, want)
	}

	cell, err := discovery.ScanCellCoordForPosition(world.Vec2{X: -0.01, Y: -500.01}, discovery.DefaultScanCellSize)
	if err != nil {
		t.Fatalf("ScanCellCoordForPosition() error = %v", err)
	}
	if want := (discovery.ScanCellCoord{X: -1, Y: -2}); cell != want {
		t.Fatalf("cell = %+v, want %+v", cell, want)
	}
}

func TestCellHashIsDeterministicAndDomainSeparated(t *testing.T) {
	seed := testSeed(t, "static-seed-a")
	cell := discovery.ScanCellCoord{X: -12, Y: 34}

	got, err := discovery.CellHash(seed, cell, "planet_candidate")
	if err != nil {
		t.Fatalf("CellHash() error = %v", err)
	}
	again, err := discovery.CellHash(seed, cell, "planet_candidate")
	if err != nil {
		t.Fatalf("CellHash() again error = %v", err)
	}
	if got != again {
		t.Fatalf("CellHash() = %d then %d, want deterministic", got, again)
	}

	otherPurpose, err := discovery.CellHash(seed, cell, "biome")
	if err != nil {
		t.Fatalf("CellHash(other purpose) error = %v", err)
	}
	if otherPurpose == got {
		t.Fatalf("CellHash() reused value across purposes: %d", got)
	}

	otherSeed := testSeed(t, "static-seed-b")
	otherSeedHash, err := discovery.CellHash(otherSeed, cell, "planet_candidate")
	if err != nil {
		t.Fatalf("CellHash(other seed) error = %v", err)
	}
	if otherSeedHash == got {
		t.Fatalf("CellHash() reused value across seeds: %d", got)
	}
}

func TestBiomeClassificationIsDeterministic(t *testing.T) {
	seed := testSeed(t, "biome-seed")
	cell := discovery.ScanCellCoord{X: 42, Y: -17}

	got, err := discovery.ClassifyBiome(seed, cell)
	if err != nil {
		t.Fatalf("ClassifyBiome() error = %v", err)
	}
	again, err := discovery.ClassifyBiome(seed, cell)
	if err != nil {
		t.Fatalf("ClassifyBiome() again error = %v", err)
	}
	if got != again {
		t.Fatalf("ClassifyBiome() = %q then %q, want deterministic", got, again)
	}
	if got == "" {
		t.Fatal("ClassifyBiome() returned empty biome")
	}
}

func TestSameSeedAndCellGenerateSameCandidates(t *testing.T) {
	seed := testSeed(t, "planet-generation-seed")
	cell, _ := findCellWithCandidates(t, seed, 1)
	options := testOptions()

	got, err := discovery.GeneratePlanetCandidates(seed, cell, options)
	if err != nil {
		t.Fatalf("GeneratePlanetCandidates() error = %v", err)
	}
	again, err := discovery.GeneratePlanetCandidates(seed, cell, options)
	if err != nil {
		t.Fatalf("GeneratePlanetCandidates() again error = %v", err)
	}
	if !reflect.DeepEqual(got, again) {
		t.Fatalf("GeneratePlanetCandidates() not deterministic:\n got: %+v\nagain: %+v", got, again)
	}
}

func TestPlanetCandidateGenerationFiltersByBiomeAndSpawnBudget(t *testing.T) {
	seed := testSeed(t, "biome-budget-seed")
	cell, candidates := findCellWithCandidates(t, seed, 1)
	biome := candidates[0].Biome()
	blockingBiome := differentBiome(biome)

	filtered, err := discovery.GeneratePlanetCandidates(seed, cell, discovery.CandidateGenerationOptions{
		MapID:          "test_map",
		ProfileVersion: "test_profile_v1",
		MapBounds:      discovery.ExactCandidateMapBounds(),
		LevelMin:       1,
		LevelMax:       6,
		Density:        1,
		AllowedBiomes:  []discovery.Biome{blockingBiome},
		SpawnBudget:    3,
	})
	if err != nil {
		t.Fatalf("GeneratePlanetCandidates(biome-filtered) error = %v", err)
	}
	if len(filtered) != 0 {
		t.Fatalf("biome-filtered candidates = %d, want none", len(filtered))
	}

	zeroBudget, err := discovery.GeneratePlanetCandidates(seed, cell, discovery.CandidateGenerationOptions{
		MapID:          "test_map",
		ProfileVersion: "test_profile_v1",
		MapBounds:      discovery.ExactCandidateMapBounds(),
		LevelMin:       1,
		LevelMax:       6,
		Density:        1,
		SpawnBudget:    0,
	})
	if err != nil {
		t.Fatalf("GeneratePlanetCandidates(zero budget) error = %v", err)
	}
	if len(zeroBudget) != 0 {
		t.Fatalf("zero-budget candidates = %d, want none", len(zeroBudget))
	}

	oneBudget, err := discovery.GeneratePlanetCandidates(seed, cell, discovery.CandidateGenerationOptions{
		MapID:          "test_map",
		ProfileVersion: "test_profile_v1",
		MapBounds:      discovery.ExactCandidateMapBounds(),
		LevelMin:       1,
		LevelMax:       6,
		Density:        1,
		SpawnBudget:    1,
	})
	if err != nil {
		t.Fatalf("GeneratePlanetCandidates(one budget) error = %v", err)
	}
	if len(oneBudget) > 1 {
		t.Fatalf("one-budget candidates = %d, want at most one", len(oneBudget))
	}
}

func TestSeedAndHiddenCandidateSerializationFailClosed(t *testing.T) {
	input := discovery.WorldSeedInput{
		StaticSeed: []byte("server-static-gameplay-seed"),
		EpochSeed:  []byte("server-epoch-gameplay-seed"),
	}
	if _, err := json.Marshal(input); !errors.Is(err, discovery.ErrWorldSeedSerialization) {
		t.Fatalf("Marshal(WorldSeedInput) error = %v, want ErrWorldSeedSerialization", err)
	}

	seed, err := discovery.NewWorldSeed(input)
	if err != nil {
		t.Fatalf("NewWorldSeed() error = %v", err)
	}
	if _, err := json.Marshal(seed); !errors.Is(err, discovery.ErrWorldSeedSerialization) {
		t.Fatalf("Marshal(WorldSeed) error = %v, want ErrWorldSeedSerialization", err)
	}

	_, candidates := findCellWithCandidates(t, seed, 1)
	if _, err := json.Marshal(candidates[0]); !errors.Is(err, discovery.ErrHiddenCandidateSerialization) {
		t.Fatalf("Marshal(PlanetCandidate) error = %v, want ErrHiddenCandidateSerialization", err)
	}
}

func TestClientSafeSignalProjectionOmitsSeedAndHiddenCandidateInternals(t *testing.T) {
	seed := testSeed(t, "server-static-gameplay-seed")
	_, candidates := findCellWithCandidates(t, seed, 1)

	data, err := json.Marshal(candidates[0].ClientSafeSignal())
	if err != nil {
		t.Fatalf("Marshal(ClientSafeSignal) error = %v", err)
	}
	payload := string(data)
	for _, leaked := range []string{
		"server-static-gameplay-seed",
		"test_map",
		"test_profile_v1",
		"candidate",
		"key",
		"position",
		"cell",
		"chunk",
		"signature",
		"level",
		`"x"`,
		`"y"`,
	} {
		if strings.Contains(payload, leaked) {
			t.Fatalf("client-safe projection %s leaked %q", payload, leaked)
		}
	}
	for _, expected := range []string{"biome", "signal_band", "approx_distance"} {
		if !strings.Contains(payload, expected) {
			t.Fatalf("client-safe projection %s missing %q", payload, expected)
		}
	}
}

func testSeed(t *testing.T, material string) discovery.WorldSeed {
	t.Helper()
	seed, err := discovery.NewWorldSeed(discovery.WorldSeedInput{
		StaticSeed: []byte(material),
	})
	if err != nil {
		t.Fatalf("NewWorldSeed() error = %v", err)
	}
	return seed
}

func testOptions() discovery.CandidateGenerationOptions {
	return discovery.CandidateGenerationOptions{
		MapID:          "test_map",
		ProfileVersion: "test_profile_v1",
		MapBounds:      discovery.ExactCandidateMapBounds(),
		LevelMin:       1,
		LevelMax:       6,
		Density:        1,
		SpawnBudget:    3,
	}
}

func findCellWithCandidates(t *testing.T, seed discovery.WorldSeed, minCandidates int) (discovery.ScanCellCoord, []discovery.PlanetCandidate) {
	t.Helper()
	for y := int64(-2); y <= 22; y++ {
		for x := int64(-2); x <= 22; x++ {
			cell := discovery.ScanCellCoord{X: x, Y: y}
			candidates, err := discovery.GeneratePlanetCandidates(seed, cell, testOptions())
			if err != nil {
				t.Fatalf("GeneratePlanetCandidates(%+v) error = %v", cell, err)
			}
			if len(candidates) >= minCandidates {
				return cell, candidates
			}
		}
	}
	t.Fatalf("no deterministic test cell produced at least %d candidates", minCandidates)
	return discovery.ScanCellCoord{}, nil
}

func differentBiome(biome discovery.Biome) discovery.Biome {
	if biome == discovery.BiomeOriginBelt {
		return discovery.BiomeDeepSpace
	}
	return discovery.BiomeOriginBelt
}
