package server

import (
	"context"
	"os"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/ships"
)

func TestPostgresRuntimeHangarStarterActiveShipPersistsAcrossRestart(t *testing.T) {
	databaseURL := os.Getenv(contentdb.EnvDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s unset; skipping runtime hangar persistence smoke", contentdb.EnvDatabaseURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	schemaURL := createRuntimeAuthSmokeSchema(t, ctx, databaseURL)

	first, err := NewRuntime(RuntimeConfig{
		SessionTTL:        time.Hour,
		WorldID:           foundation.WorldID("world-1"),
		ContentDB:         contentdb.Config{DatabaseURL: schemaURL, Mode: contentdb.ContentModeRequired, Migrations: contentdb.MigrationModeAuto},
		ContentRepository: &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)},
		Passwords:         auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("NewRuntime(first) error = %v, want nil", err)
	}
	registered, err := first.Auth.Register(ctx, auth.RegisterInput{Email: "hangar-runtime@example.com", Password: "correct-password", Callsign: "Hangar Pilot"})
	if err != nil {
		t.Fatalf("Register(first runtime) error = %v, want nil", err)
	}
	if _, err := first.Hangar.EnsureStarterShip(registered.Session.PlayerID); err != nil {
		t.Fatalf("EnsureStarterShip(first runtime) error = %v, want nil", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close(first runtime) error = %v, want nil", err)
	}

	second, err := NewRuntime(RuntimeConfig{
		SessionTTL:        time.Hour,
		WorldID:           foundation.WorldID("world-1"),
		ContentDB:         contentdb.Config{DatabaseURL: schemaURL, Mode: contentdb.ContentModeRequired, Migrations: contentdb.MigrationModeAuto},
		ContentRepository: &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)},
		Passwords:         auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("NewRuntime(second) error = %v, want nil", err)
	}
	defer second.Close()
	hangar, err := second.Hangar.GetHangar(registered.Session.PlayerID)
	if err != nil {
		t.Fatalf("GetHangar(second runtime) error = %v, want nil", err)
	}
	if len(hangar.Ships) != 1 || hangar.Ships[0].ShipID != ships.ShipIDStarter || hangar.Ships[0].State != ships.ShipStateActive {
		t.Fatalf("reloaded ships = %+v, want active starter", hangar.Ships)
	}
	if !hangar.HasActiveShip || hangar.ActiveShip.ShipID != ships.ShipIDStarter {
		t.Fatalf("reloaded active ship = %+v has=%v, want starter", hangar.ActiveShip, hangar.HasActiveShip)
	}
}
