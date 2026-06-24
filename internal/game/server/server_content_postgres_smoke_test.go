package server

import (
	"context"
	"crypto/rand"
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
	"gameproject/internal/game/world"
)

func TestPostgresCMSInvalidPublishedContentFailsRuntimeBoot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store := openServerPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	invalidSnapshot := buildServerPostgresSmokeSnapshot(t, "cms_smoke_invalid_boot_v1")
	invalidSnapshot.Items = removeServerPostgresSmokeSnapshotRow(invalidSnapshot.Items, "scanner_t1")
	if _, err := store.PublishContentSnapshot(ctx, content.PublishSnapshotInput{
		ID:                   "33333333-3333-5333-8333-333333333333",
		Version:              invalidSnapshot.Version,
		Snapshot:             invalidSnapshot,
		ValidationReportJSON: json.RawMessage(`{"source":"postgres_smoke","valid":false}`),
		IdempotencyKey:       "postgres_smoke_invalid_boot",
		Notes:                "postgres smoke invalid boot proof",
		BalanceTag:           "smoke",
		CreatedBy:            "postgres_smoke",
		PublishedBy:          "postgres_smoke",
		PublishedAt:          time.Date(2026, 6, 25, 12, 10, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("PublishContentSnapshot(invalid) error = %v, want nil so runtime can fail closed", err)
	}

	runtime, err := NewRuntime(RuntimeConfig{
		WorldID: foundation.WorldID("world-1"),
		ContentDB: contentdb.Config{
			DatabaseURL: "postgres://postgres-smoke-isolated",
			Mode:        contentdb.ContentModeRequired,
			Migrations:  contentdb.MigrationModeVerify,
		},
		contentDBOpen: func(context.Context, contentdb.Config) (runtimeContentStore, error) {
			return store, nil
		},
	})
	if runtime != nil {
		_ = runtime.Close()
	}
	if err == nil {
		t.Fatal("NewRuntime(invalid published content) error = nil, want fail-closed boot error")
	}
	if !strings.Contains(err.Error(), "published content") {
		t.Fatalf("NewRuntime(invalid published content) error = %v, want published content failure", err)
	}
}

func openServerPostgresSmokeStore(t *testing.T, ctx context.Context) *contentdb.Store {
	t.Helper()
	databaseURL := os.Getenv(contentdb.EnvDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s unset; skipping Postgres CMS runtime smoke", contentdb.EnvDatabaseURL)
	}

	db, err := contentdb.Open(ctx, contentdb.Config{
		DatabaseURL: databaseURL,
		Mode:        contentdb.ContentModeRequired,
		Migrations:  contentdb.MigrationModeOff,
	})
	if err != nil {
		t.Fatalf("Open(Postgres runtime smoke) error = %v, want nil", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	schema := serverPostgresSmokeSchemaName(t)
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA %s`, quoteServerPostgresIdent(schema))); err != nil {
		_ = db.Close()
		t.Fatalf("CREATE SCHEMA %s error = %v, want nil", schema, err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`SET search_path TO %s, public`, quoteServerPostgresIdent(schema))); err != nil {
		_ = db.Close()
		t.Fatalf("SET search_path for %s error = %v, want nil", schema, err)
	}
	t.Cleanup(func() {
		dropServerPostgresSmokeSchema(t, databaseURL, schema)
	})

	store, err := contentdb.NewStore(db)
	if err != nil {
		_ = db.Close()
		t.Fatalf("NewStore(Postgres runtime smoke) error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func dropServerPostgresSmokeSchema(t *testing.T, databaseURL string, schema string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := contentdb.Open(ctx, contentdb.Config{
		DatabaseURL: databaseURL,
		Mode:        contentdb.ContentModeRequired,
		Migrations:  contentdb.MigrationModeOff,
	})
	if err != nil {
		t.Logf("cleanup open for schema %s failed: %v", schema, err)
		return
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, quoteServerPostgresIdent(schema))); err != nil {
		t.Logf("cleanup drop schema %s failed: %v", schema, err)
	}
}

func serverPostgresSmokeSchemaName(t *testing.T) string {
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

func quoteServerPostgresIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func buildServerPostgresSmokeSnapshot(t *testing.T, version string) content.Snapshot {
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

func removeServerPostgresSmokeSnapshotRow(rows []content.SnapshotRow, contentID string) []content.SnapshotRow {
	out := rows[:0]
	for _, row := range rows {
		if row.ContentID == content.ContentID(contentID) {
			continue
		}
		out = append(out, row)
	}
	return out
}
