package contentdb

import (
	"errors"
	"testing"
)

func TestFromEnvDefaultsDisabledWhenURLAbsent(t *testing.T) {
	t.Setenv(EnvDatabaseURL, "")
	t.Setenv(EnvMode, "")
	t.Setenv(EnvMigrations, "")

	config := FromEnv()

	if config.Mode != ContentModeOff {
		t.Fatalf("Mode = %q, want %q", config.Mode, ContentModeOff)
	}
	if config.Migrations != MigrationModeOff {
		t.Fatalf("Migrations = %q, want %q", config.Migrations, MigrationModeOff)
	}
	if config.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestFromEnvDefaultsRequiredWhenURLPresent(t *testing.T) {
	t.Setenv(EnvDatabaseURL, " postgres://gameproject:pw@localhost:5432/gameproject?sslmode=disable ")
	t.Setenv(EnvMode, "")
	t.Setenv(EnvMigrations, "")

	config := FromEnv()

	if config.DatabaseURL != "postgres://gameproject:pw@localhost:5432/gameproject?sslmode=disable" {
		t.Fatalf("DatabaseURL = %q, want trimmed URL", config.DatabaseURL)
	}
	if config.Mode != ContentModeRequired {
		t.Fatalf("Mode = %q, want %q", config.Mode, ContentModeRequired)
	}
	if config.Migrations != MigrationModeAuto {
		t.Fatalf("Migrations = %q, want %q", config.Migrations, MigrationModeAuto)
	}
	if !config.Enabled() {
		t.Fatal("Enabled() = false, want true")
	}
}

func TestValidateRejectsRequiredModeWithoutURL(t *testing.T) {
	config := Config{Mode: ContentModeRequired, Migrations: MigrationModeAuto}

	err := config.Validate()

	if !errors.Is(err, ErrMissingDatabaseURL) {
		t.Fatalf("Validate() error = %v, want ErrMissingDatabaseURL", err)
	}
}

func TestValidateRejectsInvalidModes(t *testing.T) {
	if err := (Config{Mode: "maybe"}).Validate(); !errors.Is(err, ErrInvalidContentMode) {
		t.Fatalf("invalid content mode error = %v, want ErrInvalidContentMode", err)
	}
	if err := (Config{Mode: ContentModeOff, Migrations: "later"}).Validate(); !errors.Is(err, ErrInvalidMigrationMode) {
		t.Fatalf("invalid migration mode error = %v, want ErrInvalidMigrationMode", err)
	}
}
