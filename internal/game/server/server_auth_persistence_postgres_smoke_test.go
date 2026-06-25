package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
)

func TestPostgresRuntimeAuthPersistsLoginAcrossRestart(t *testing.T) {
	databaseURL := os.Getenv(contentdb.EnvDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s unset; skipping runtime auth persistence smoke", contentdb.EnvDatabaseURL)
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
	if _, err := first.Auth.Register(ctx, auth.RegisterInput{Email: "pilot@example.com", Password: "correct-password", Callsign: "Frontier-01"}); err != nil {
		t.Fatalf("Register(first runtime) error = %v, want nil", err)
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
	loggedIn, err := second.Auth.Login(ctx, auth.LoginInput{Email: "pilot@example.com", Password: "correct-password"})
	if err != nil {
		t.Fatalf("Login(second runtime) error = %v, want nil", err)
	}
	if loggedIn.Session.PlayerID.IsZero() || loggedIn.Response.Player.Callsign != "Frontier-01" {
		t.Fatalf("logged in session = %+v response = %+v, want persisted player", loggedIn.Session, loggedIn.Response)
	}
}

func createRuntimeAuthSmokeSchema(t *testing.T, ctx context.Context, databaseURL string) string {
	t.Helper()
	db, err := contentdb.Open(ctx, contentdb.Config{DatabaseURL: databaseURL, Mode: contentdb.ContentModeRequired, Migrations: contentdb.MigrationModeOff})
	if err != nil {
		t.Fatalf("Open(runtime auth smoke) error = %v, want nil", err)
	}
	schema := runtimeAuthSmokeSchemaName(t)
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA %s`, quoteRuntimeAuthPostgresIdent(schema))); err != nil {
		_ = db.Close()
		t.Fatalf("CREATE SCHEMA %s error = %v, want nil", schema, err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, quoteRuntimeAuthPostgresIdent(schema)))
		_ = db.Close()
	})
	return databaseURLWithSearchPath(t, databaseURL, schema)
}

func runtimeAuthSmokeSchemaName(t *testing.T) string {
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
	return "runtime_auth_" + base + "_" + hex.EncodeToString(suffix[:])
}

func databaseURLWithSearchPath(t *testing.T, databaseURL string, schema string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema+",public")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func quoteRuntimeAuthPostgresIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
