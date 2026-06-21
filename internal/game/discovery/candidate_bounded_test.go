package discovery_test

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/discovery"
)

func TestPlanetCandidateGenerationIsBoundedToExactPlayableMap(t *testing.T) {
	seed := testSeed(t, "bounded-seed")
	options := testOptions()
	bounds := discovery.ExactCandidateMapBounds()

	for y := int64(-3); y <= 23; y++ {
		for x := int64(-3); x <= 23; x++ {
			cell := discovery.ScanCellCoord{X: x, Y: y}
			candidates, err := discovery.GeneratePlanetCandidates(seed, cell, options)
			if err != nil {
				t.Fatalf("GeneratePlanetCandidates(%+v) error = %v", cell, err)
			}
			for _, candidate := range candidates {
				if !bounds.Contains(candidate.Position()) {
					t.Fatalf("candidate position %+v from cell %+v outside bounds %+v", candidate.Position(), cell, bounds)
				}
			}
		}
	}
}

func TestPlanetCandidateGenerationOutsideAndEdgeCellsStayBounded(t *testing.T) {
	seed := testSeed(t, "bounded-edge-seed")
	options := testOptions()
	bounds := discovery.ExactCandidateMapBounds()
	cells := []discovery.ScanCellCoord{
		{X: -1, Y: 0},
		{X: 0, Y: -1},
		{X: -1, Y: -1},
		{X: 20, Y: 0},
		{X: 0, Y: 20},
		{X: 20, Y: 20},
		{X: 21, Y: 0},
		{X: 0, Y: 21},
		{X: 21, Y: 21},
	}

	for _, cell := range cells {
		candidates, err := discovery.GeneratePlanetCandidates(seed, cell, options)
		if err != nil {
			t.Fatalf("GeneratePlanetCandidates(%+v) error = %v", cell, err)
		}
		for _, candidate := range candidates {
			if !bounds.Contains(candidate.Position()) {
				t.Fatalf("candidate position %+v from edge/outside cell %+v outside bounds", candidate.Position(), cell)
			}
		}
	}
}

func TestPlanetCandidateKeysIncludeMapAndProfileIdentity(t *testing.T) {
	seed := testSeed(t, "bounded-identity-seed")
	cell, candidates := findCellWithCandidates(t, seed, 1)
	base := candidates[0]

	mapOptions := testOptions()
	mapOptions.MapID = "other_map"
	otherMap, err := discovery.GeneratePlanetCandidates(seed, cell, mapOptions)
	if err != nil {
		t.Fatalf("GeneratePlanetCandidates(other map) error = %v", err)
	}
	if len(otherMap) == 0 {
		t.Fatalf("other map cell %+v produced no candidates, want same cell candidate for key comparison", cell)
	}
	if base.Key() == otherMap[0].Key() {
		t.Fatalf("candidate keys match across maps: %d", base.Key())
	}

	profileOptions := testOptions()
	profileOptions.ProfileVersion = "other_profile_v2"
	otherProfile, err := discovery.GeneratePlanetCandidates(seed, cell, profileOptions)
	if err != nil {
		t.Fatalf("GeneratePlanetCandidates(other profile) error = %v", err)
	}
	if len(otherProfile) == 0 {
		t.Fatalf("other profile cell %+v produced no candidates, want same cell candidate for key comparison", cell)
	}
	if base.Key() == otherProfile[0].Key() {
		t.Fatalf("candidate keys match across profiles: %d", base.Key())
	}
}

func TestPlanetCandidateLevelComesFromConfiguredBand(t *testing.T) {
	seed := testSeed(t, "bounded-level-band-seed")
	options := testOptions()
	options.LevelMin = 7
	options.LevelMax = 7

	_, candidates := findCellWithCandidatesForOptions(t, seed, options, 1)
	if got := candidates[0].Level(); got != 7 {
		t.Fatalf("candidate level = %d, want configured band level 7", got)
	}
}

func TestClientSafeSignalUsesMapLocalLabelsOnly(t *testing.T) {
	seed := testSeed(t, "bounded-signal-seed")
	_, candidates := findCellWithCandidates(t, seed, 1)
	signal := candidates[0].ClientSafeSignal()
	if signal.ApproxDistance != "map_core" && signal.ApproxDistance != "map_mid" && signal.ApproxDistance != "map_edge" {
		t.Fatalf("signal approximate distance = %q, want map-local label", signal.ApproxDistance)
	}
	data, err := json.Marshal(signal)
	if err != nil {
		t.Fatalf("Marshal(ClientSafeSignal) error = %v", err)
	}
	payload := string(data)
	for _, leaked := range []string{
		"near_origin",
		"deep_space",
		"frontier",
		"other_map",
		"test_profile_v1",
		"roll",
		"seed",
		"signature",
		"level",
		"key",
		`"x"`,
		`"y"`,
	} {
		if strings.Contains(payload, leaked) {
			t.Fatalf("client-safe signal %s leaked %q", payload, leaked)
		}
	}
}

func findCellWithCandidatesForOptions(
	t *testing.T,
	seed discovery.WorldSeed,
	options discovery.CandidateGenerationOptions,
	minCandidates int,
) (discovery.ScanCellCoord, []discovery.PlanetCandidate) {
	t.Helper()
	for y := int64(-2); y <= 22; y++ {
		for x := int64(-2); x <= 22; x++ {
			cell := discovery.ScanCellCoord{X: x, Y: y}
			candidates, err := discovery.GeneratePlanetCandidates(seed, cell, options)
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
