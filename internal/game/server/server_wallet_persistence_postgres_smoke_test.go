package server

import (
	"context"
	"os"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestPostgresRuntimeWalletCreditPersistsAcrossRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	databaseURL := os.Getenv(contentdb.EnvDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s unset; skipping runtime wallet persistence smoke", contentdb.EnvDatabaseURL)
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
	registered, err := first.Auth.Register(ctx, auth.RegisterInput{Email: "wallet@example.com", Password: "correct-password", Callsign: "Wallet Pilot"})
	if err != nil {
		t.Fatalf("Register(first runtime) error = %v, want nil", err)
	}
	if _, err := first.Wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     registered.Session.PlayerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       900,
		Reason:       economy.LedgerReason("test_wallet_restart"),
		ReferenceKey: foundation.IdempotencyKey("admin_compensation:wallet-restart:credit-1"),
	}); err != nil {
		t.Fatalf("CreditWallet(first runtime) error = %v, want nil", err)
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
	if got := second.Wallet.Balance(registered.Session.PlayerID, economy.CurrencyBucketCredits); got != 900 {
		t.Fatalf("Balance(after restart) = %d, want 900", got)
	}
}
