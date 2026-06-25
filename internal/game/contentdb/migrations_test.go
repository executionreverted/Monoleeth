package contentdb

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEmbeddedMigrationsHaveChecksums(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	if len(migrations) != 7 {
		t.Fatalf("len(migrations) = %d, want 7", len(migrations))
	}
	if migrations[0].Version != "0001_schema_migrations" {
		t.Fatalf("Version = %q, want 0001_schema_migrations", migrations[0].Version)
	}
	if migrations[1].Version != "0002_content_schema" {
		t.Fatalf("Version = %q, want 0002_content_schema", migrations[1].Version)
	}
	if migrations[2].Version != "0003_player_state_schema" {
		t.Fatalf("Version = %q, want 0003_player_state_schema", migrations[2].Version)
	}
	if migrations[3].Version != "0004_inventory_stackable_columns" {
		t.Fatalf("Version = %q, want 0004_inventory_stackable_columns", migrations[3].Version)
	}
	if migrations[4].Version != "0005_inventory_allows_system_owner" {
		t.Fatalf("Version = %q, want 0005_inventory_allows_system_owner", migrations[4].Version)
	}
	if migrations[5].Version != "0006_progression_state_columns" {
		t.Fatalf("Version = %q, want 0006_progression_state_columns", migrations[5].Version)
	}
	if migrations[6].Version != "0007_hangar_state_schema" {
		t.Fatalf("Version = %q, want 0007_hangar_state_schema", migrations[6].Version)
	}
	if migrations[0].Checksum == "" || migrations[0].SQL == "" {
		t.Fatalf("migration = %+v, want SQL and checksum", migrations[0])
	}
}

func TestHangarStateSchemaMigrationHasShipTables(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[6].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS player_ships",
		"CREATE TABLE IF NOT EXISTS player_active_ships",
		"state text NOT NULL CHECK (state IN ('available', 'active', 'disabled', 'repairing', 'locked'))",
		"metadata_json jsonb",
		"PRIMARY KEY (player_id, ship_id)",
		"FOREIGN KEY (player_id, ship_id) REFERENCES player_ships(player_id, ship_id)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("hangar state schema migration missing %q", fragment)
		}
	}
}

func TestPlayerStateSchemaMigrationHasAuthTables(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[2].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS accounts",
		"CREATE TABLE IF NOT EXISTS players",
		"CREATE TABLE IF NOT EXISTS auth_sessions",
		"UNIQUE (email)",
		"UNIQUE (token_hash)",
		"REFERENCES accounts(id)",
		"REFERENCES players(id)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("player state schema migration missing %q", fragment)
		}
	}
}

func TestContentSchemaMigrationHasDraftTablesAndCurrentIndex(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[1].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS content_versions",
		"CREATE TABLE IF NOT EXISTS content_audit_log",
		"CREATE TABLE IF NOT EXISTS content_items",
		"CREATE TABLE IF NOT EXISTS content_craft_recipes",
		"CREATE UNIQUE INDEX IF NOT EXISTS content_versions_one_current",
		"WHERE is_current",
		"jsonb_typeof(data_json) = 'object'",
		"jsonb_typeof(display_json) = 'object'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("content schema migration missing %q", fragment)
		}
	}
}

func TestPendingMigrationsRejectsChecksumMismatch(t *testing.T) {
	migration := Migration{Version: "0001_schema_migrations", SQL: "select 1", Checksum: "abc"}

	_, err := PendingMigrations(map[string]string{"0001_schema_migrations": "def"}, []Migration{migration})

	if !errors.Is(err, ErrMigrationChecksumMismatch) {
		t.Fatalf("PendingMigrations() error = %v, want ErrMigrationChecksumMismatch", err)
	}
}

func TestApplyMigrationsAutoAppliesPendingInOrder(t *testing.T) {
	store := &fakeMigrationStore{}
	migrations := []Migration{
		{Version: "0001_schema_migrations", SQL: "select 1", Checksum: "a"},
		{Version: "0002_content_schema", SQL: "select 2", Checksum: "b"},
	}

	err := ApplyMigrations(context.Background(), store, migrations, MigrationModeAuto)

	if err != nil {
		t.Fatalf("ApplyMigrations() error = %v, want nil", err)
	}
	if got := store.appliedOrder; len(got) != 2 || got[0] != "0001_schema_migrations" || got[1] != "0002_content_schema" {
		t.Fatalf("appliedOrder = %+v, want both migrations in order", got)
	}
}

func TestApplyMigrationsVerifyRejectsPending(t *testing.T) {
	store := &fakeMigrationStore{}
	migrations := []Migration{{Version: "0001_schema_migrations", SQL: "select 1", Checksum: "a"}}

	err := ApplyMigrations(context.Background(), store, migrations, MigrationModeVerify)

	if !errors.Is(err, ErrPendingMigrations) {
		t.Fatalf("ApplyMigrations(verify) error = %v, want ErrPendingMigrations", err)
	}
	if len(store.appliedOrder) != 0 {
		t.Fatalf("appliedOrder = %+v, want none in verify mode", store.appliedOrder)
	}
}

type fakeMigrationStore struct {
	applied      map[string]string
	appliedOrder []string
}

func (store *fakeMigrationStore) AppliedMigrations(context.Context) (map[string]string, error) {
	out := make(map[string]string, len(store.applied))
	for key, value := range store.applied {
		out[key] = value
	}
	return out, nil
}

func (store *fakeMigrationStore) ApplyMigration(_ context.Context, migration Migration) error {
	if store.applied == nil {
		store.applied = make(map[string]string)
	}
	store.applied[migration.Version] = migration.Checksum
	store.appliedOrder = append(store.appliedOrder, migration.Version)
	return nil
}
