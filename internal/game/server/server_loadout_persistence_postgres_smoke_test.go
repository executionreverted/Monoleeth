package server

import (
	"context"
	"os"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
)

func TestPostgresRuntimeLoadoutServiceSavedLoadoutPersistsAcrossRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	databaseURL := os.Getenv(contentdb.EnvDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s unset; skipping runtime loadout persistence smoke", contentdb.EnvDatabaseURL)
	}
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
	registered, err := first.Auth.Register(ctx, auth.RegisterInput{Email: "loadout-runtime@example.com", Password: "correct-password", Callsign: "Loadout Pilot"})
	if err != nil {
		t.Fatalf("Register(first runtime) error = %v, want nil", err)
	}
	if err := first.ensurePlayerSession(registered.Session); err != nil {
		t.Fatalf("ensurePlayerSession(first runtime) error = %v, want nil", err)
	}
	laserInstanceID := starterModuleInstanceID(t, first, registered.Session.PlayerID, foundation.ItemID("laser_alpha_t1"))
	if _, err := first.Loadout.SaveLoadout(modules.SaveLoadoutInput{
		LoadoutID: "runtime-postgres-combat",
		PlayerID:  registered.Session.PlayerID,
		ShipID:    gamecontent.DefaultStarterShipID,
		Name:      "Runtime Postgres Combat",
		SlotAssignments: modules.SlotAssignments{
			modules.ModuleSlotOffensive1: laserInstanceID,
		},
	}); err != nil {
		t.Fatalf("SaveLoadout(first runtime) error = %v, want nil", err)
	}
	if _, err := first.Loadout.ApplyLoadout(modules.ApplyLoadoutInput{
		PlayerID:  registered.Session.PlayerID,
		LoadoutID: "runtime-postgres-combat",
		RequestID: "request-runtime-postgres-loadout-apply",
	}); err != nil {
		t.Fatalf("ApplyLoadout(first runtime) error = %v, want nil", err)
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
	reapplied, err := second.Loadout.ApplyLoadout(modules.ApplyLoadoutInput{
		PlayerID:  registered.Session.PlayerID,
		LoadoutID: "runtime-postgres-combat",
		RequestID: "request-runtime-postgres-loadout-reapply",
	})
	if err != nil {
		t.Fatalf("ApplyLoadout(second runtime) error = %v, want nil", err)
	}
	if !reapplied.Noop || len(reapplied.Current) != 1 || reapplied.Current[0].ItemInstanceID != laserInstanceID {
		t.Fatalf("reapplied loadout = %+v, want no-op reload with equipped laser %q", reapplied, laserInstanceID)
	}
}
