package visibility_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

func TestFogMemoryRemembersKnownPlanetSummary(t *testing.T) {
	memory := visibility.NewFogMemory()
	summary := testPlanetIntel("planet-1", "Beta")

	if err := memory.RememberPlanet(summary); err != nil {
		t.Fatalf("RememberPlanet() error = %v, want nil", err)
	}

	got, ok := memory.KnownPlanet("planet-1")
	if !ok {
		t.Fatal("KnownPlanet() ok = false, want true")
	}
	if !reflect.DeepEqual(got, summary) {
		t.Fatalf("KnownPlanet() = %+v, want %+v", got, summary)
	}
}

func TestFogMemoryDoesNotAllowLiveInteraction(t *testing.T) {
	memory := visibility.NewFogMemory()
	summary := testPlanetIntel("planet-1", "Beta")
	if err := memory.RememberPlanet(summary); err != nil {
		t.Fatalf("RememberPlanet() error = %v, want nil", err)
	}
	if _, ok := memory.KnownPlanet("planet-1"); !ok {
		t.Fatal("KnownPlanet() ok = false, want true")
	}

	viewer := testViewer(10)
	entity := testEntity(summary.LastKnownPosition)

	err := visibility.CanInteract(viewer, entity)
	if !errors.Is(err, visibility.ErrNotVisible) {
		t.Fatalf("CanInteract() error = %v, want ErrNotVisible despite fog memory", err)
	}
}

func TestKnownPlanetsReturnsDeterministicOrder(t *testing.T) {
	memory := visibility.NewFogMemory()
	for _, summary := range []visibility.PlanetIntelSummary{
		testPlanetIntel("planet-c", "C"),
		testPlanetIntel("planet-a", "A"),
		testPlanetIntel("planet-b", "B"),
	} {
		if err := memory.RememberPlanet(summary); err != nil {
			t.Fatalf("RememberPlanet() error = %v, want nil", err)
		}
	}

	got := memory.KnownPlanets()
	gotIDs := make([]foundation.PlanetID, 0, len(got))
	for _, summary := range got {
		gotIDs = append(gotIDs, summary.PlanetID)
	}
	wantIDs := []foundation.PlanetID{"planet-a", "planet-b", "planet-c"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("KnownPlanets() ids = %v, want %v", gotIDs, wantIDs)
	}
}

func TestRememberPlanetRejectsInvalidSummary(t *testing.T) {
	memory := visibility.NewFogMemory()

	if err := memory.RememberPlanet(visibility.PlanetIntelSummary{}); !errors.Is(err, visibility.ErrInvalidPlanetIntel) {
		t.Fatalf("RememberPlanet(empty) error = %v, want ErrInvalidPlanetIntel", err)
	}
}

func TestScannerBridgeEventSkeletonNamesScannerEventsOnly(t *testing.T) {
	event := visibility.ScannerBridgeEvent{
		Type:       visibility.ScannerEventPlanetDiscovered,
		PlayerID:   "player-1",
		WorldID:    "world-1",
		ZoneID:     "zone-1",
		PlanetID:   "planet-1",
		Position:   world.Vec2{X: 25, Y: 0},
		OccurredAt: time.Unix(1, 0),
	}

	if event.Type != visibility.ScannerBridgeEventType("scanner.planet_discovered") {
		t.Fatalf("ScannerBridgeEvent.Type = %q, want scanner.planet_discovered", event.Type)
	}
	if event.Position != (world.Vec2{X: 25, Y: 0}) {
		t.Fatalf("ScannerBridgeEvent.Position = %+v, want skeleton event position", event.Position)
	}
}

func testPlanetIntel(planetID foundation.PlanetID, displayName string) visibility.PlanetIntelSummary {
	return visibility.PlanetIntelSummary{
		PlanetID:          planetID,
		WorldID:           "world-1",
		ZoneID:            "zone-1",
		LastKnownPosition: world.Vec2{X: 100, Y: 0},
		DisplayName:       displayName,
		Source:            visibility.IntelSourceScanner,
		UpdatedAt:         time.Unix(1, 0),
	}
}
