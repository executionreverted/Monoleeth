package server

import (
	"context"
	"os"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

func TestPostgresRuntimeProgressionXPPersistsAcrossRestart(t *testing.T) {
	databaseURL := os.Getenv(contentdb.EnvDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s unset; skipping runtime progression persistence smoke", contentdb.EnvDatabaseURL)
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
	registered, err := first.Auth.Register(ctx, auth.RegisterInput{Email: "progression@example.com", Password: "correct-password", Callsign: "XP Pilot"})
	if err != nil {
		t.Fatalf("Register(first runtime) error = %v, want nil", err)
	}
	if _, err := first.Progression.GrantXP(progression.GrantXPInput{
		PlayerID:       registered.Session.PlayerID,
		Amount:         275,
		SourceType:     progression.XPSourceTypeAdminAdjustment,
		SourceID:       progression.XPSourceID("runtime-progression-restart"),
		IdempotencyKey: progression.XPIdempotencyKey("admin_xp:runtime-progression-restart"),
		Authority:      progression.XPGrantAuthorityAdminService,
	}); err != nil {
		t.Fatalf("GrantXP(first runtime) error = %v, want nil", err)
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
	snapshot, err := second.Progression.GetProgressionSnapshot(registered.Session.PlayerID)
	if err != nil {
		t.Fatalf("GetProgressionSnapshot(second runtime) error = %v, want nil", err)
	}
	if snapshot.Player.MainXP != 275 {
		t.Fatalf("MainXP(after restart) = %d, want 275", snapshot.Player.MainXP)
	}
}
