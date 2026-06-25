package contentdb

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/ships"
)

func TestHangarStoreValidationRejectsInvalidStateAndMetadataJSON(t *testing.T) {
	playerID := foundation.PlayerID("player-hangar-validation")

	invalidState := ships.HangarRecord{}
	invalidState.PutPlayerShip(ships.PlayerShipState{
		PlayerID:   playerID,
		ShipID:     ships.ShipIDStarter,
		UnlockedAt: time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
		State:      ships.ShipState("destroyed"),
	})
	if err := validateHangarRecord(playerID, invalidState); !errors.Is(err, ships.ErrInvalidShipState) {
		t.Fatalf("invalid state validation error = %v, want ErrInvalidShipState", err)
	}

	invalidMetadata := ships.HangarRecord{}
	invalidMetadata.PutPlayerShip(ships.PlayerShipState{
		PlayerID:     playerID,
		ShipID:       ships.ShipIDStarter,
		UnlockedAt:   time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
		State:        ships.ShipStateActive,
		MetadataJSON: []byte(`{"nickname":`),
	})
	if err := validateHangarRecord(playerID, invalidMetadata); !errors.Is(err, ships.ErrInvalidMetadataJSON) {
		t.Fatalf("invalid metadata validation error = %v, want ErrInvalidMetadataJSON", err)
	}
}

func TestHangarStoreValidationRejectsActiveShipWithoutPlayerShipRow(t *testing.T) {
	playerID := foundation.PlayerID("player-hangar-validation")
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	record := ships.HangarRecord{}
	record.PutActiveShip(ships.ActiveShipState{
		PlayerID:    playerID,
		ShipID:      ships.ShipIDStarter,
		ActivatedAt: now,
		UpdatedAt:   now,
	})

	if err := validateHangarRecord(playerID, record); !errors.Is(err, ErrActiveShipMissingRow) {
		t.Fatalf("missing active ship row validation error = %v, want ErrActiveShipMissingRow", err)
	}
}
