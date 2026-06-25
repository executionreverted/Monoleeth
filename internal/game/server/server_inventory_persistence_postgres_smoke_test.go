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

func TestPostgresRuntimeInventoryItemPersistsAcrossRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	databaseURL := os.Getenv(contentdb.EnvDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s unset; skipping runtime inventory persistence smoke", contentdb.EnvDatabaseURL)
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
	registered, err := first.Auth.Register(ctx, auth.RegisterInput{Email: "inventory@example.com", Password: "correct-password", Callsign: "Inventory Pilot"})
	if err != nil {
		t.Fatalf("Register(first runtime) error = %v, want nil", err)
	}
	definition, ok := first.itemCatalog[foundation.ItemID("raw_ore")]
	if !ok {
		t.Fatal("raw_ore missing from runtime item catalog")
	}
	location := economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID("account")}
	if _, err := first.Inventory.AddItem(economy.AddItemInput{
		PlayerID:       registered.Session.PlayerID,
		ItemDefinition: definition,
		Quantity:       11,
		Location:       location,
		Reason:         economy.LedgerReason("loot_pickup"),
		ReferenceKey:   foundation.IdempotencyKey("loot_pickup:runtime-inventory-restart"),
	}); err != nil {
		t.Fatalf("AddItem(first runtime) error = %v, want nil", err)
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
	if got := second.Inventory.TotalItemQuantity(registered.Session.PlayerID, foundation.ItemID("raw_ore"), location); got != 11 {
		t.Fatalf("TotalItemQuantity(after restart) = %d, want 11", got)
	}
}
