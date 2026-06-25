package contentdb_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/ships"
)

func TestPostgresHangarStorePersistsStarterActiveShipAcrossStoreReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-hangar-smoke")
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	hangarStore, err := contentdb.NewHangarStore(store)
	if err != nil {
		t.Fatalf("NewHangarStore() error = %v, want nil", err)
	}
	service := newPostgresHangarService(t, playerID, hangarStore)
	if _, err := service.EnsureStarterShip(playerID); err != nil {
		t.Fatalf("EnsureStarterShip() error = %v, want nil", err)
	}
	if err := hangarStore.UpdatePlayerHangar(playerID, func(record *ships.HangarRecord) error {
		starter, ok := record.PlayerShip(ships.ShipIDStarter)
		if !ok {
			t.Fatalf("starter missing before metadata update")
		}
		starter.MetadataJSON = json.RawMessage(`{"paint":"blue"}`)
		record.PutPlayerShip(starter)
		return nil
	}); err != nil {
		t.Fatalf("UpdatePlayerHangar(metadata) error = %v, want nil", err)
	}

	reopened, err := contentdb.NewHangarStore(store)
	if err != nil {
		t.Fatalf("NewHangarStore(reopen) error = %v, want nil", err)
	}
	reloadedService := newPostgresHangarService(t, playerID, reopened)
	hangar, err := reloadedService.GetHangar(playerID)
	if err != nil {
		t.Fatalf("GetHangar(reloaded) error = %v, want nil", err)
	}
	if len(hangar.Ships) != 1 || hangar.Ships[0].ShipID != ships.ShipIDStarter || hangar.Ships[0].State != ships.ShipStateActive {
		t.Fatalf("reloaded ships = %+v, want active starter", hangar.Ships)
	}
	if !hangar.HasActiveShip || hangar.ActiveShip.ShipID != ships.ShipIDStarter {
		t.Fatalf("reloaded active ship = %+v has=%v, want starter", hangar.ActiveShip, hangar.HasActiveShip)
	}
	var metadata map[string]string
	if err := json.Unmarshal(hangar.Ships[0].MetadataJSON, &metadata); err != nil {
		t.Fatalf("metadata json = %s, unmarshal error = %v, want valid json", hangar.Ships[0].MetadataJSON, err)
	}
	if metadata["paint"] != "blue" {
		t.Fatalf("metadata paint = %q, want blue", metadata["paint"])
	}
}

func newPostgresHangarService(t *testing.T, playerID foundation.PlayerID, store ships.HangarStore) *ships.HangarService {
	t.Helper()
	catalogRows, err := ships.MVPShipCatalog()
	if err != nil {
		t.Fatalf("MVPShipCatalog() error = %v, want nil", err)
	}
	service, err := ships.NewHangarService(
		catalogRows,
		store,
		ships.StaticPlayerRankProvider{playerID: 1},
		ships.BaseShipCargoCapacityProvider{},
		foundation.RealClock{},
	)
	if err != nil {
		t.Fatalf("NewHangarService() error = %v, want nil", err)
	}
	return service
}

func TestPostgresHangarStoreRollsBackFailedUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-hangar-rollback")
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	hangarStore, err := contentdb.NewHangarStore(store)
	if err != nil {
		t.Fatalf("NewHangarStore() error = %v, want nil", err)
	}
	service := newPostgresHangarService(t, playerID, hangarStore)
	if _, err := service.EnsureStarterShip(playerID); err != nil {
		t.Fatalf("EnsureStarterShip() error = %v, want nil", err)
	}

	sentinel := errors.New("abort hangar mutation")
	err = hangarStore.UpdatePlayerHangar(playerID, func(record *ships.HangarRecord) error {
		starter, ok := record.PlayerShip(ships.ShipIDStarter)
		if !ok {
			t.Fatalf("starter missing before failed update")
		}
		starter.State = ships.ShipStateDisabled
		starter.DisabledReason = ships.DisabledReasonDeath
		record.PutPlayerShip(starter)
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("failed UpdatePlayerHangar error = %v, want sentinel", err)
	}

	err = hangarStore.ViewPlayerHangar(playerID, func(record ships.HangarRecord) error {
		starter, ok := record.PlayerShip(ships.ShipIDStarter)
		if !ok || starter.State != ships.ShipStateActive || starter.DisabledReason != "" {
			t.Fatalf("starter after failed update = %+v has=%v, want unchanged active starter", starter, ok)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ViewPlayerHangar(after rollback) error = %v, want nil", err)
	}
}
