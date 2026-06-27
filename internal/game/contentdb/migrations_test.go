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
	if len(migrations) != 21 {
		t.Fatalf("len(migrations) = %d, want 21", len(migrations))
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
	if migrations[7].Version != "0008_inventory_instance_items" {
		t.Fatalf("Version = %q, want 0008_inventory_instance_items", migrations[7].Version)
	}
	if migrations[8].Version != "0009_inventory_ledger_references_counters" {
		t.Fatalf("Version = %q, want 0009_inventory_ledger_references_counters", migrations[8].Version)
	}
	if migrations[9].Version != "0010_loadout_state_schema" {
		t.Fatalf("Version = %q, want 0010_loadout_state_schema", migrations[9].Version)
	}
	if migrations[10].Version != "0011_economy_idempotency_outbox" {
		t.Fatalf("Version = %q, want 0011_economy_idempotency_outbox", migrations[10].Version)
	}
	if migrations[11].Version != "0012_wallet_ledger_references" {
		t.Fatalf("Version = %q, want 0012_wallet_ledger_references", migrations[11].Version)
	}
	if migrations[12].Version != "0013_inventory_move_remove_references" {
		t.Fatalf("Version = %q, want 0013_inventory_move_remove_references", migrations[12].Version)
	}
	if migrations[13].Version != "0014_market_listing_state_schema" {
		t.Fatalf("Version = %q, want 0014_market_listing_state_schema", migrations[13].Version)
	}
	if migrations[14].Version != "0015_auction_lot_state_schema" {
		t.Fatalf("Version = %q, want 0015_auction_lot_state_schema", migrations[14].Version)
	}
	if migrations[15].Version != "0016_premium_entitlement_state_schema" {
		t.Fatalf("Version = %q, want 0016_premium_entitlement_state_schema", migrations[15].Version)
	}
	if migrations[16].Version != "0017_loot_drop_claims_schema" {
		t.Fatalf("Version = %q, want 0017_loot_drop_claims_schema", migrations[16].Version)
	}
	if migrations[17].Version != "0018_content_audit_action" {
		t.Fatalf("Version = %q, want 0018_content_audit_action", migrations[17].Version)
	}
	if migrations[18].Version != "0019_planet_claim_durable_lifecycle" {
		t.Fatalf("Version = %q, want 0019_planet_claim_durable_lifecycle", migrations[18].Version)
	}
	if migrations[19].Version != "0020_settlement_building_mutation_durable" {
		t.Fatalf("Version = %q, want 0020_settlement_building_mutation_durable", migrations[19].Version)
	}
	if migrations[20].Version != "0021_automation_route_durable" {
		t.Fatalf("Version = %q, want 0021_automation_route_durable", migrations[20].Version)
	}
	if migrations[0].Checksum == "" || migrations[0].SQL == "" {
		t.Fatalf("migration = %+v, want SQL and checksum", migrations[0])
	}
}

func TestLootDropClaimsSchemaMigrationHasDurableClaimTable(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[16].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS loot_drop_claims",
		"drop_id text PRIMARY KEY",
		"player_id text NOT NULL CHECK",
		"quantity bigint NOT NULL CHECK (quantity > 0)",
		"payload_json jsonb NOT NULL",
		"CREATE INDEX IF NOT EXISTS loot_drop_claims_player_claimed_idx",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("loot drop claim schema migration missing %q", fragment)
		}
	}
}

func TestPremiumEntitlementSchemaMigrationHasDurableEntitlementTable(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[15].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS premium_entitlements",
		"entitlement_id text PRIMARY KEY",
		"entitlement_type text NOT NULL CHECK",
		"state text NOT NULL CHECK (state IN ('pending', 'claimed', 'revoked'))",
		"provider_source text NOT NULL CHECK",
		"provider_reference text NOT NULL CHECK",
		"payload_json jsonb NOT NULL",
		"UNIQUE (provider_source, provider_reference)",
		"CHECK (provider_confirmed_at <= created_at)",
		"CHECK (state <> 'claimed' OR claimed_at IS NOT NULL)",
		"CREATE INDEX IF NOT EXISTS premium_entitlements_player_state_idx",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("premium entitlement schema migration missing %q", fragment)
		}
	}
}

func TestAuctionLotSchemaMigrationHasDurableLotTable(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[14].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS auction_lots",
		"auction_id text PRIMARY KEY",
		"payload_json jsonb NOT NULL",
		"buy_now_price bigint CHECK (buy_now_price IS NULL OR buy_now_price > 0)",
		"current_bid bigint NOT NULL DEFAULT 0 CHECK (current_bid >= 0)",
		"status text NOT NULL CHECK (status IN ('upcoming', 'active', 'closed', 'expired'))",
		"CHECK (buy_now_price IS NULL OR buy_now_price >= start_price)",
		"CHECK ((current_bid = 0 AND current_bidder_id = '') OR (current_bid > 0 AND btrim(current_bidder_id) <> ''))",
		"CREATE INDEX IF NOT EXISTS auction_lots_status_ends_idx",
		"CREATE INDEX IF NOT EXISTS auction_lots_world_status_idx",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("auction lot schema migration missing %q", fragment)
		}
	}
}

func TestMarketListingSchemaMigrationHasDurableListingTables(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[13].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS market_listings",
		"listing_id text PRIMARY KEY",
		"seller_player_id text NOT NULL CHECK (btrim(seller_player_id) <> '')",
		"item_definition_json jsonb NOT NULL",
		"remaining_quantity bigint NOT NULL CHECK (remaining_quantity >= 0)",
		"escrow_location_kind text NOT NULL CHECK (escrow_location_kind = 'market_escrow')",
		"CHECK (remaining_quantity <= original_quantity)",
		"CHECK (status <> 'stale' OR stale_at IS NOT NULL)",
		"CREATE INDEX IF NOT EXISTS market_listings_seller_status_idx",
		"CREATE INDEX IF NOT EXISTS market_listings_active_expiry_idx",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("market listing schema migration missing %q", fragment)
		}
	}
}

func TestInventoryMoveRemoveReferencesMigrationHasDurableEvidenceTables(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[12].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS player_inventory_move_item_references",
		"primary_ledger_id text NOT NULL REFERENCES player_inventory_item_ledger(ledger_id)",
		"ledger_ids jsonb NOT NULL DEFAULT '[]'::jsonb",
		"result_json jsonb NOT NULL DEFAULT '{}'::jsonb",
		"PRIMARY KEY (player_id, reference_key)",
		"CREATE TABLE IF NOT EXISTS player_inventory_remove_item_references",
		"player_inventory_remove_item_references_ledger_idx",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("inventory move/remove ref migration missing %q", fragment)
		}
	}
}

func TestWalletLedgerReferencesMigrationHasDurableEvidenceTables(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[11].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS player_wallet_ledger",
		"ledger_id text PRIMARY KEY",
		"currency_type text NOT NULL",
		"reference_key text NOT NULL",
		"CREATE TABLE IF NOT EXISTS player_wallet_references",
		"operation text NOT NULL CHECK (operation IN ('credit_wallet', 'debit_wallet', 'transfer_currency'))",
		"primary_ledger_id text NOT NULL REFERENCES player_wallet_ledger(ledger_id)",
		"ledger_ids jsonb NOT NULL DEFAULT '[]'::jsonb",
		"PRIMARY KEY (player_id, operation, reference_key)",
		"CREATE TABLE IF NOT EXISTS player_wallet_counters",
		"ledger_sequence bigint NOT NULL DEFAULT 0 CHECK (ledger_sequence >= 0)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("wallet ledger/ref migration missing %q", fragment)
		}
	}
}

func TestEconomyIdempotencyOutboxMigrationHasDurableTables(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[10].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS idempotency_keys",
		"PRIMARY KEY (scope, idempotency_key)",
		"status text NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'completed', 'failed'))",
		"result_json jsonb NOT NULL DEFAULT '{}'::jsonb",
		"CREATE INDEX IF NOT EXISTS idempotency_keys_operation_player_idx",
		"CREATE TABLE IF NOT EXISTS outbox",
		"payload_json jsonb NOT NULL CHECK (jsonb_typeof(payload_json) = 'object')",
		"status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'leased', 'published', 'failed', 'dead'))",
		"lease_owner text NOT NULL DEFAULT ''",
		"leased_until timestamptz",
		"attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0)",
		"max_attempts integer NOT NULL DEFAULT 20 CHECK (max_attempts > 0)",
		"CREATE INDEX IF NOT EXISTS outbox_status_available_idx",
		"WHERE status IN ('pending', 'failed')",
		"CREATE INDEX IF NOT EXISTS outbox_lease_idx",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("economy idempotency/outbox migration missing %q", fragment)
		}
	}
}

func TestLoadoutStateSchemaMigrationHasLoadoutAndEquippedTables(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[9].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS player_loadouts",
		"slot_assignments_json jsonb NOT NULL DEFAULT '{}'::jsonb",
		"PRIMARY KEY (player_id, loadout_id)",
		"FOREIGN KEY (player_id, ship_id) REFERENCES player_ships(player_id, ship_id)",
		"CREATE TABLE IF NOT EXISTS player_equipped_modules",
		"PRIMARY KEY (player_id, ship_id, slot_id)",
		"UNIQUE (item_instance_id)",
		"REFERENCES player_inventory_instance_items(item_instance_id)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("loadout state schema migration missing %q", fragment)
		}
	}
}

func TestInventoryLedgerRefsCountersMigrationHasDurableEvidenceTables(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[8].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS player_inventory_item_ledger",
		"ledger_id text PRIMARY KEY",
		"reference_key text NOT NULL",
		"CREATE TABLE IF NOT EXISTS player_inventory_add_item_references",
		"item_instance_ids jsonb NOT NULL DEFAULT '[]'::jsonb",
		"PRIMARY KEY (player_id, reference_key)",
		"CREATE TABLE IF NOT EXISTS player_inventory_counters",
		"item_sequence bigint NOT NULL DEFAULT 0 CHECK (item_sequence >= 0)",
		"ledger_sequence bigint NOT NULL DEFAULT 0 CHECK (ledger_sequence >= 0)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("inventory ledger/ref/counter migration missing %q", fragment)
		}
	}
}

func TestInventoryInstanceItemsMigrationHasInstanceTable(t *testing.T) {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v, want nil", err)
	}
	sql := migrations[7].SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS player_inventory_instance_items",
		"item_instance_id text PRIMARY KEY",
		"quantity bigint NOT NULL DEFAULT 1 CHECK (quantity = 1)",
		"durability_current bigint NOT NULL DEFAULT 0 CHECK (durability_current >= 0)",
		"bound_state text NOT NULL DEFAULT 'unbound'",
		"metadata_json jsonb",
		"player_inventory_instance_items_player_idx",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("inventory instance migration missing %q", fragment)
		}
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
