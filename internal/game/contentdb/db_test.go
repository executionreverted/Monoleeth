package contentdb

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestOpenReturnsDisabledWhenContentDBOff(t *testing.T) {
	db, err := Open(context.Background(), Config{Mode: ContentModeOff})

	if db != nil {
		t.Fatal("Open() db != nil, want nil")
	}
	if !errors.Is(err, ErrContentDatabaseDisabled) {
		t.Fatalf("Open() error = %v, want ErrContentDatabaseDisabled", err)
	}
}

func TestOpenRejectsRequiredModeWithoutURL(t *testing.T) {
	db, err := Open(context.Background(), Config{Mode: ContentModeRequired})

	if db != nil {
		t.Fatal("Open() db != nil, want nil")
	}
	if !errors.Is(err, ErrMissingDatabaseURL) {
		t.Fatalf("Open() error = %v, want ErrMissingDatabaseURL", err)
	}
}

func TestOpenLivePostgresWhenEnvPresent(t *testing.T) {
	databaseURL := os.Getenv(EnvDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s unset; skipping live Postgres open test", EnvDatabaseURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := Open(ctx, Config{DatabaseURL: databaseURL, Mode: ContentModeRequired, Migrations: MigrationModeVerify})
	if err != nil {
		t.Fatalf("Open(live) error = %v, want nil", err)
	}
	defer db.Close()
}
