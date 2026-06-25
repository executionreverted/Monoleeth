package server

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
)

func TestNewRuntimeCoreStoreDevFallbackLeavesEconomyStoresUnwired(t *testing.T) {
	runtime, err := NewRuntime(RuntimeConfig{
		SessionTTL:        time.Hour,
		WorldID:           foundation.WorldID("world-1"),
		ContentRepository: &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)},
		ContentDB:         contentdb.Config{Mode: contentdb.ContentModeOff},
		CoreStoreMode:     contentdb.ContentModeDevFallback,
		DevMode:           true,
		Passwords:         auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v, want nil", err)
	}
	defer runtime.Close()

	requireRuntimeServiceStoreNil(t, runtime.Market, "idempotencyStore")
	requireRuntimeServiceStoreNil(t, runtime.Market, "outboxStore")
	requireRuntimeServiceStoreNil(t, runtime.Market, "listingRepository")
	requireRuntimeServiceStoreNil(t, runtime.Auction, "idempotencyStore")
	requireRuntimeServiceStoreNil(t, runtime.Premium, "idempotencyStore")
	requireRuntimeServiceStoreNil(t, runtime.Loot, "xpOutbox")
}

func TestPostgresRuntimeCoreStoreInjectsEconomyStores(t *testing.T) {
	databaseURL := os.Getenv(contentdb.EnvDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s unset; skipping runtime core economy store smoke", contentdb.EnvDatabaseURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	schemaURL := createRuntimeAuthSmokeSchema(t, ctx, databaseURL)

	runtime, err := NewRuntime(RuntimeConfig{
		SessionTTL:        time.Hour,
		WorldID:           foundation.WorldID("world-1"),
		ContentDB:         contentdb.Config{DatabaseURL: schemaURL, Mode: contentdb.ContentModeRequired, Migrations: contentdb.MigrationModeAuto},
		CoreStoreMode:     contentdb.ContentModeRequired,
		ContentRepository: &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)},
		Passwords:         auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v, want nil", err)
	}
	defer runtime.Close()

	requireRuntimeServiceStoreType(t, runtime.Market, "idempotencyStore", "*contentdb.Store")
	requireRuntimeServiceStoreType(t, runtime.Market, "outboxStore", "*contentdb.Store")
	requireRuntimeServiceStoreType(t, runtime.Market, "listingRepository", "*contentdb.MarketListingStore")
	requireRuntimeServiceStoreType(t, runtime.Auction, "idempotencyStore", "*contentdb.Store")
	requireRuntimeServiceStoreType(t, runtime.Premium, "idempotencyStore", "*contentdb.Store")
	requireRuntimeServiceStoreType(t, runtime.Loot, "xpOutbox", "*contentdb.Store")
}

func requireRuntimeServiceStoreNil(t *testing.T, service any, fieldName string) {
	t.Helper()
	field := runtimeServiceStoreField(t, service, fieldName)
	if !field.IsNil() {
		t.Fatalf("%T.%s = %s, want nil", service, fieldName, field.Elem().Type())
	}
}

func requireRuntimeServiceStoreType(t *testing.T, service any, fieldName string, want string) {
	t.Helper()
	field := runtimeServiceStoreField(t, service, fieldName)
	if field.IsNil() {
		t.Fatalf("%T.%s = nil, want %s", service, fieldName, want)
	}
	if got := field.Elem().Type().String(); got != want {
		t.Fatalf("%T.%s = %s, want %s", service, fieldName, got, want)
	}
}

func runtimeServiceStoreField(t *testing.T, service any, fieldName string) reflect.Value {
	t.Helper()
	value := reflect.ValueOf(service)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		t.Fatalf("service = %T, want non-nil pointer", service)
	}
	field := value.Elem().FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("%T.%s missing", service, fieldName)
	}
	if field.Kind() != reflect.Interface {
		t.Fatalf("%T.%s kind = %s, want interface", service, fieldName, field.Kind())
	}
	return field
}
