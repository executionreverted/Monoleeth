package ships

import (
	"encoding/json"
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestPlayerShipStateValidatesIDsStateAndMetadata(t *testing.T) {
	playerShip, err := NewPlayerShipState("player-1", ShipIDStarter, ShipStateAvailable)
	if err != nil {
		t.Fatalf("NewPlayerShipState(valid) error = %v, want nil", err)
	}
	playerShip.MetadataJSON = json.RawMessage(`{"nickname":"skiff"}`)
	if err := playerShip.Validate(); err != nil {
		t.Fatalf("PlayerShipState.Validate() = %v, want nil", err)
	}

	if _, err := NewPlayerShipState("", ShipIDStarter, ShipStateAvailable); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank player id error = %v, want foundation.ErrEmptyID", err)
	}
	if _, err := NewPlayerShipState("player-1", "", ShipStateAvailable); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank ship id error = %v, want foundation.ErrEmptyID", err)
	}
	if _, err := NewPlayerShipState("player-1", ShipIDStarter, ShipState("destroyed")); !errors.Is(err, ErrInvalidShipState) {
		t.Fatalf("invalid ship state error = %v, want ErrInvalidShipState", err)
	}

	playerShip.MetadataJSON = json.RawMessage(`{"nickname":`)
	if err := playerShip.Validate(); !errors.Is(err, ErrInvalidMetadataJSON) {
		t.Fatalf("invalid metadata error = %v, want ErrInvalidMetadataJSON", err)
	}
}

func TestActiveShipStateValidatesIDs(t *testing.T) {
	activeShip, err := NewActiveShipState("player-1", ShipIDStarter)
	if err != nil {
		t.Fatalf("NewActiveShipState(valid) error = %v, want nil", err)
	}
	if err := activeShip.Validate(); err != nil {
		t.Fatalf("ActiveShipState.Validate() = %v, want nil", err)
	}

	if _, err := NewActiveShipState("", ShipIDStarter); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank active player id error = %v, want foundation.ErrEmptyID", err)
	}
	if _, err := NewActiveShipState("player-1", ""); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank active ship id error = %v, want foundation.ErrEmptyID", err)
	}
}

func TestSupportedShipStatesAndStringBehavior(t *testing.T) {
	states := SupportedShipStates()
	if got, want := len(states), 5; got != want {
		t.Fatalf("len(SupportedShipStates()) = %d, want %d", got, want)
	}
	for _, state := range states {
		if err := state.Validate(); err != nil {
			t.Fatalf("supported state %q Validate() = %v, want nil", state, err)
		}
	}
	if got := ShipStateRepairing.String(); got != "repairing" {
		t.Fatalf("ShipStateRepairing.String() = %q, want repairing", got)
	}
}
