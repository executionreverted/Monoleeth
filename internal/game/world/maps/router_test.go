package maps

import (
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

func TestRouterActiveLocationReadOnly(t *testing.T) {
	catalog, err := StarterCatalog("world-1")
	if err != nil {
		t.Fatalf("StarterCatalog() error = %v, want nil", err)
	}
	router, err := NewRouter(catalog)
	if err != nil {
		t.Fatalf("NewRouter() error = %v, want nil", err)
	}

	if _, err := router.ActiveLocation("player-1"); !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("ActiveLocation(missing) error = %v, want %v", err, ErrLocationNotFound)
	}
	if len(router.locations) != 0 {
		t.Fatalf("ActiveLocation mutated locations = %+v, want none", router.locations)
	}
}

func TestRouterEnsureStarterLocationCreatesOnceAndPreservesExisting(t *testing.T) {
	catalog, err := StarterCatalog("world-1")
	if err != nil {
		t.Fatalf("StarterCatalog() error = %v, want nil", err)
	}
	router, err := NewRouter(catalog)
	if err != nil {
		t.Fatalf("NewRouter() error = %v, want nil", err)
	}

	location, err := router.EnsureStarterLocation(foundation.PlayerID("player-1"))
	if err != nil {
		t.Fatalf("EnsureStarterLocation() error = %v, want nil", err)
	}
	if location.InternalMapID != StarterMapID || location.PublicMapKey != "1-1" || location.ZoneID != StarterMapID.ZoneID() {
		t.Fatalf("location = %+v, want starter map with zone == map", location)
	}
	if got, err := router.EnsureStarterLocation("player-1"); err != nil || got != location {
		t.Fatalf("EnsureStarterLocation(existing) = %+v/%v, want preserved %+v", got, err, location)
	}

	want, err := router.SetActiveLocationFromSpawn("player-1", "map_1_2", "west_gate")
	if err != nil {
		t.Fatalf("SetActiveLocationFromSpawn() error = %v, want nil", err)
	}
	for i := 0; i < 3; i++ {
		got, err := router.EnsureStarterLocation("player-1")
		if err != nil {
			t.Fatalf("EnsureStarterLocation(%d) error = %v, want nil", i, err)
		}
		if got != want {
			t.Fatalf("EnsureStarterLocation(%d) = %+v, want preserved %+v", i, got, want)
		}
	}
}

func TestRouterValidatesActivePosition(t *testing.T) {
	catalog, err := StarterCatalog("world-1")
	if err != nil {
		t.Fatalf("StarterCatalog() error = %v, want nil", err)
	}
	router, err := NewRouter(catalog)
	if err != nil {
		t.Fatalf("NewRouter() error = %v, want nil", err)
	}
	if _, err := router.EnsureStarterLocation("player-1"); err != nil {
		t.Fatalf("EnsureStarterLocation() error = %v, want nil", err)
	}
	if err := router.ValidateActivePosition("player-1", world.Vec2{X: 10000, Y: 10000}); err != nil {
		t.Fatalf("ValidateActivePosition(in bounds) error = %v, want nil", err)
	}
	if err := router.ValidateActivePosition("player-1", world.Vec2{X: 10000.1, Y: 0}); !errors.Is(err, ErrPositionOutOfBounds) {
		t.Fatalf("ValidateActivePosition(out of bounds) error = %v, want %v", err, ErrPositionOutOfBounds)
	}
}
