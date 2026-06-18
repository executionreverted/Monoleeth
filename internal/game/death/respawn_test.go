package death_test

import (
	"errors"
	"reflect"
	"testing"

	"gameproject/internal/game/death"
	"gameproject/internal/game/world"
)

func TestDefaultRespawnPriorityMatchesDesign(t *testing.T) {
	want := []death.RespawnLocationKind{
		death.RespawnLocationKindCheckpoint,
		death.RespawnLocationKindOwnedPlanet,
		death.RespawnLocationKindSafeStation,
		death.RespawnLocationKindOriginStation,
	}
	if got := death.DefaultRespawnPriority(); !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultRespawnPriority() = %v, want %v", got, want)
	}
}

func TestSelectRespawnLocationPrefersLastCheckpoint(t *testing.T) {
	checkpoint := respawnLocation("checkpoint-alpha", death.RespawnLocationKindCheckpoint, 500, 500)

	selection, err := death.SelectRespawnLocation(death.SelectRespawnLocationInput{
		PlayerID:       "player-1",
		DeathPosition:  world.Vec2{X: 10, Y: 10},
		LastCheckpoint: &checkpoint,
		OwnedPlanets: []death.RespawnLocation{
			respawnLocation("planet-close", death.RespawnLocationKindOwnedPlanet, 11, 10),
		},
		SafeStations: []death.RespawnLocation{
			respawnLocation("station-close", death.RespawnLocationKindSafeStation, 10, 11),
		},
		Origin: respawnOrigin(),
	})
	if err != nil {
		t.Fatalf("SelectRespawnLocation() error = %v, want nil", err)
	}
	if selection.Location.ID != "checkpoint-alpha" || selection.PriorityIndex != 0 || selection.OriginFallback {
		t.Fatalf("selection = %+v, want checkpoint priority", selection)
	}
}

func TestSelectRespawnLocationUsesNearestOwnedPlanetBeforeSafeStation(t *testing.T) {
	selection, err := death.SelectRespawnLocation(death.SelectRespawnLocationInput{
		PlayerID:      "player-1",
		DeathPosition: world.Vec2{X: 0, Y: 0},
		OwnedPlanets: []death.RespawnLocation{
			respawnLocation("planet-far", death.RespawnLocationKindOwnedPlanet, 100, 0),
			respawnLocation("planet-near", death.RespawnLocationKindOwnedPlanet, 8, 0),
		},
		SafeStations: []death.RespawnLocation{
			respawnLocation("station-nearest", death.RespawnLocationKindSafeStation, 1, 0),
		},
		Origin: respawnOrigin(),
	})
	if err != nil {
		t.Fatalf("SelectRespawnLocation() error = %v, want nil", err)
	}
	if selection.Location.ID != "planet-near" || selection.PriorityIndex != 1 || selection.OriginFallback {
		t.Fatalf("selection = %+v, want nearest owned planet before station", selection)
	}
}

func TestSelectRespawnLocationUsesNearestSafeStationBeforeOrigin(t *testing.T) {
	selection, err := death.SelectRespawnLocation(death.SelectRespawnLocationInput{
		PlayerID:      "player-1",
		DeathPosition: world.Vec2{X: 0, Y: 0},
		SafeStations: []death.RespawnLocation{
			respawnLocation("station-b", death.RespawnLocationKindSafeStation, 10, 0),
			respawnLocation("station-a", death.RespawnLocationKindSafeStation, -5, 0),
		},
		Origin: respawnOrigin(),
	})
	if err != nil {
		t.Fatalf("SelectRespawnLocation() error = %v, want nil", err)
	}
	if selection.Location.ID != "station-a" || selection.PriorityIndex != 2 || selection.OriginFallback {
		t.Fatalf("selection = %+v, want nearest safe station before origin", selection)
	}
}

func TestRespawnServiceSelectLocationUsesConfiguredOriginForMVP(t *testing.T) {
	service, err := death.NewRespawnService(death.RespawnConfig{Origin: respawnOrigin()})
	if err != nil {
		t.Fatalf("NewRespawnService() error = %v, want nil", err)
	}

	selection, err := service.SelectLocation(death.SelectRespawnLocationInput{
		PlayerID:      "player-1",
		DeathPosition: world.Vec2{X: 20, Y: 30},
	})
	if err != nil {
		t.Fatalf("SelectLocation(origin) error = %v, want nil", err)
	}
	if selection.Location.ID != "origin-station" || selection.PriorityIndex != 3 || !selection.OriginFallback {
		t.Fatalf("origin selection = %+v, want origin fallback", selection)
	}

	checkpoint := respawnLocation("checkpoint-alpha", death.RespawnLocationKindCheckpoint, 25, 30)
	selection, err = service.SelectLocation(death.SelectRespawnLocationInput{
		PlayerID:       "player-1",
		DeathPosition:  world.Vec2{X: 20, Y: 30},
		LastCheckpoint: &checkpoint,
	})
	if err != nil {
		t.Fatalf("SelectLocation(checkpoint) error = %v, want nil", err)
	}
	if selection.Location.ID != "checkpoint-alpha" || selection.PriorityIndex != 0 || selection.OriginFallback {
		t.Fatalf("checkpoint selection = %+v, want checkpoint", selection)
	}
}

func TestSelectRespawnLocationRejectsWrongCandidateKind(t *testing.T) {
	_, err := death.SelectRespawnLocation(death.SelectRespawnLocationInput{
		PlayerID:      "player-1",
		DeathPosition: world.Vec2{},
		OwnedPlanets: []death.RespawnLocation{
			respawnLocation("station-forged", death.RespawnLocationKindSafeStation, 5, 0),
		},
		Origin: respawnOrigin(),
	})
	if !errors.Is(err, death.ErrInvalidRespawnLocation) {
		t.Fatalf("SelectRespawnLocation() error = %v, want ErrInvalidRespawnLocation", err)
	}
}

func respawnOrigin() death.RespawnLocation {
	return respawnLocation("origin-station", death.RespawnLocationKindOriginStation, 0, 0)
}

func respawnLocation(id death.RespawnLocationID, kind death.RespawnLocationKind, x float64, y float64) death.RespawnLocation {
	return death.RespawnLocation{
		ID:       id,
		Kind:     kind,
		Position: world.Vec2{X: x, Y: y},
	}
}
