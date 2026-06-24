package contentdb_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/content"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/contentseed"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/world"
)

func TestPostgresCMSSmokeSeedsOncePreservesExistingAndLoadsLatestPublished(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}

	seedSnapshot := buildPostgresSmokeSnapshot(t, "content_mvp_seed_v1")
	seedResult, err := contentseed.EnsurePublishedSeed(ctx, store, seedSnapshot, contentseed.SeedOptions{})
	if err != nil {
		t.Fatalf("EnsurePublishedSeed(empty) error = %v, want nil", err)
	}
	if !seedResult.Seeded || seedResult.RowCount == 0 {
		t.Fatalf("seed result = %+v, want seeded rows", seedResult)
	}
	if got := countContentVersions(t, ctx, db); got != 1 {
		t.Fatalf("content_versions count after seed = %d, want 1", got)
	}

	secondSeed, err := contentseed.EnsurePublishedSeed(ctx, store, seedSnapshot, contentseed.SeedOptions{})
	if err != nil {
		t.Fatalf("EnsurePublishedSeed(existing) error = %v, want nil", err)
	}
	if secondSeed.Seeded {
		t.Fatalf("second seed result = %+v, want no-op", secondSeed)
	}
	if got := countContentVersions(t, ctx, db); got != 1 {
		t.Fatalf("content_versions count after second seed = %d, want 1", got)
	}

	latestSnapshot := buildPostgresSmokeSnapshot(t, "cms_smoke_published_v2")
	mutateLaserDamage(t, &latestSnapshot, 88)
	publishedAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	if _, err := store.PublishContentSnapshot(ctx, content.PublishSnapshotInput{
		ID:                   "11111111-1111-5111-8111-111111111111",
		Version:              latestSnapshot.Version,
		Snapshot:             latestSnapshot,
		ValidationReportJSON: json.RawMessage(`{"source":"postgres_smoke","valid":true}`),
		IdempotencyKey:       "postgres_smoke_publish_v2",
		Notes:                "postgres smoke latest published proof",
		BalanceTag:           "smoke",
		CreatedBy:            "postgres_smoke",
		PublishedBy:          "postgres_smoke",
		PublishedAt:          publishedAt,
	}); err != nil {
		t.Fatalf("PublishContentSnapshot(v2) error = %v, want nil", err)
	}
	if got := countContentVersions(t, ctx, db); got != 2 {
		t.Fatalf("content_versions count after v2 publish = %d, want 2", got)
	}

	thirdSeed, err := contentseed.EnsurePublishedSeed(ctx, store, seedSnapshot, contentseed.SeedOptions{})
	if err != nil {
		t.Fatalf("EnsurePublishedSeed(after publish) error = %v, want nil", err)
	}
	if thirdSeed.Seeded {
		t.Fatalf("third seed result = %+v, want no-op", thirdSeed)
	}
	current, err := store.LoadCurrentContentSnapshot(ctx)
	if err != nil {
		t.Fatalf("LoadCurrentContentSnapshot() error = %v, want nil", err)
	}
	if current.Version != latestSnapshot.Version {
		t.Fatalf("current version = %q, want latest %q", current.Version, latestSnapshot.Version)
	}

	repository, err := contentdb.NewRepository(store)
	if err != nil {
		t.Fatalf("NewRepository() error = %v, want nil", err)
	}
	bundle, err := content.LoadPublishedContent(ctx, repository, world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("LoadPublishedContent(latest) error = %v, want nil", err)
	}
	definition, ok := bundle.Modules.Lookup(foundation.ItemID("laser_alpha_t1"))
	if !ok {
		t.Fatal("laser_alpha_t1 missing from latest published bundle")
	}
	if got := moduleStatValue(t, definition, modules.StatWeaponDamage); got != 88 {
		t.Fatalf("latest laser damage = %d, want 88", got)
	}
}

func TestPostgresCMSSmokeInvalidPublishedSnapshotFailsClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	invalidSnapshot := buildPostgresSmokeSnapshot(t, "cms_smoke_invalid_v1")
	invalidSnapshot.Items = removeSnapshotRow(invalidSnapshot.Items, "scanner_t1")
	if _, err := store.PublishContentSnapshot(ctx, content.PublishSnapshotInput{
		ID:                   "22222222-2222-5222-8222-222222222222",
		Version:              invalidSnapshot.Version,
		Snapshot:             invalidSnapshot,
		ValidationReportJSON: json.RawMessage(`{"source":"postgres_smoke","valid":false}`),
		IdempotencyKey:       "postgres_smoke_invalid",
		Notes:                "postgres smoke invalid published proof",
		BalanceTag:           "smoke",
		CreatedBy:            "postgres_smoke",
		PublishedBy:          "postgres_smoke",
		PublishedAt:          time.Date(2026, 6, 25, 12, 5, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("PublishContentSnapshot(invalid) error = %v, want nil so repository can fail closed", err)
	}

	repository, err := contentdb.NewRepository(store)
	if err != nil {
		t.Fatalf("NewRepository() error = %v, want nil", err)
	}
	_, err = content.LoadPublishedContent(ctx, repository, world.WorldID("world-1"))
	if err == nil {
		t.Fatal("LoadPublishedContent(invalid) error = nil, want fail-closed error")
	}
}

func openPostgresSmokeStore(t *testing.T, ctx context.Context) (*sql.DB, *contentdb.Store) {
	t.Helper()
	databaseURL := os.Getenv(contentdb.EnvDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s unset; skipping Postgres CMS smoke", contentdb.EnvDatabaseURL)
	}

	db, err := contentdb.Open(ctx, contentdb.Config{
		DatabaseURL: databaseURL,
		Mode:        contentdb.ContentModeRequired,
		Migrations:  contentdb.MigrationModeOff,
	})
	if err != nil {
		t.Fatalf("Open(Postgres smoke) error = %v, want nil", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	schema := postgresSmokeSchemaName(t)
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA %s`, quotePostgresIdent(schema))); err != nil {
		_ = db.Close()
		t.Fatalf("CREATE SCHEMA %s error = %v, want nil", schema, err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`SET search_path TO %s, public`, quotePostgresIdent(schema))); err != nil {
		_ = db.Close()
		t.Fatalf("SET search_path for %s error = %v, want nil", schema, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, quotePostgresIdent(schema)))
	})

	store, err := contentdb.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore(Postgres smoke) error = %v, want nil", err)
	}
	return db, store
}

func postgresSmokeSchemaName(t *testing.T) string {
	t.Helper()
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatalf("rand.Read() error = %v, want nil", err)
	}
	name := strings.ToLower(t.Name())
	replacer := strings.NewReplacer("/", "_", "-", "_", " ", "_")
	name = replacer.Replace(name)
	var builder strings.Builder
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' {
			builder.WriteRune(char)
		}
	}
	base := builder.String()
	if len(base) > 40 {
		base = base[:40]
	}
	return "cms_smoke_" + base + "_" + hex.EncodeToString(suffix[:])
}

func quotePostgresIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func buildPostgresSmokeSnapshot(t *testing.T, version string) content.Snapshot {
	t.Helper()
	snapshot, err := contentseed.BuildMVPSnapshot(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("BuildMVPSnapshot() error = %v, want nil", err)
	}
	snapshot.Version = version
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("snapshot Validate() error = %v, want nil", err)
	}
	return snapshot
}

func mutateLaserDamage(t *testing.T, snapshot *content.Snapshot, damage int64) {
	t.Helper()
	for index := range snapshot.Modules {
		if snapshot.Modules[index].ContentID != content.ContentID("laser_alpha_t1") {
			continue
		}
		var definition modules.ModuleDefinition
		if err := json.Unmarshal(snapshot.Modules[index].DataJSON, &definition); err != nil {
			t.Fatalf("unmarshal laser module error = %v, want nil", err)
		}
		for statIndex := range definition.StatModifiers {
			if definition.StatModifiers[statIndex].Stat == modules.StatWeaponDamage {
				definition.StatModifiers[statIndex].Value = damage
				encoded, err := json.Marshal(definition)
				if err != nil {
					t.Fatalf("marshal laser module error = %v, want nil", err)
				}
				snapshot.Modules[index].DataJSON = encoded
				return
			}
		}
		t.Fatal("laser_alpha_t1 weapon damage stat missing")
	}
	t.Fatal("laser_alpha_t1 module row missing")
}

func moduleStatValue(t *testing.T, definition modules.ModuleDefinition, stat modules.StatKey) int64 {
	t.Helper()
	for _, modifier := range definition.StatModifiers {
		if modifier.Stat == stat {
			return modifier.Value
		}
	}
	t.Fatalf("module stat %q missing", stat)
	return 0
}

func removeSnapshotRow(rows []content.SnapshotRow, contentID string) []content.SnapshotRow {
	out := rows[:0]
	for _, row := range rows {
		if row.ContentID == content.ContentID(contentID) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func countContentVersions(t *testing.T, ctx context.Context, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM content_versions`).Scan(&count); err != nil {
		t.Fatalf("count content_versions error = %v, want nil", err)
	}
	return count
}
