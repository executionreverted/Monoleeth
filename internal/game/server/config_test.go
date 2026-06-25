package server

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/ships"
)

func TestConfigFromEnvE2EPlanetClaimSeedDefaultsOff(t *testing.T) {
	t.Setenv(EnvE2EPlanetClaimSeed, "")

	config := ConfigFromEnv()

	if config.E2EPlanetClaimSeed {
		t.Fatal("E2EPlanetClaimSeed = true, want false when env is absent")
	}
}

func TestConfigFromEnvE2EPlanetClaimSeedOptIn(t *testing.T) {
	t.Setenv(EnvE2EPlanetClaimSeed, "true")

	config := ConfigFromEnv()

	if !config.E2EPlanetClaimSeed {
		t.Fatal("E2EPlanetClaimSeed = false, want true when env is true")
	}
}

func TestConfigFromEnvE2EPlanetClaimCoresOptIn(t *testing.T) {
	t.Setenv(EnvE2EPlanetClaimCores, "2")

	config := ConfigFromEnv().withDefaults()

	if config.E2EPlanetClaimCores != 2 {
		t.Fatalf("E2EPlanetClaimCores = %d, want 2", config.E2EPlanetClaimCores)
	}
}

func TestConfigFromEnvE2EPlanetClaimCoresInvalidFallsBack(t *testing.T) {
	t.Setenv(EnvE2EPlanetClaimCores, "-4")

	config := ConfigFromEnv().withDefaults()

	if config.E2EPlanetClaimCores != defaultE2EClaimCores {
		t.Fatalf("E2EPlanetClaimCores = %d, want default %d", config.E2EPlanetClaimCores, defaultE2EClaimCores)
	}
}

func TestConfigFromEnvE2ERouteSeedDefaultsOff(t *testing.T) {
	t.Setenv(EnvE2ERouteSeed, "")

	config := ConfigFromEnv()

	if config.E2ERouteSeed {
		t.Fatal("E2ERouteSeed = true, want false when env is absent")
	}
}

func TestConfigFromEnvE2ERouteSeedOptIn(t *testing.T) {
	t.Setenv(EnvE2ERouteSeed, "true")

	config := ConfigFromEnv()

	if !config.E2ERouteSeed {
		t.Fatal("E2ERouteSeed = false, want true when env is true")
	}
}

func TestConfigFromEnvE2EScanNoPlanetSeedOptIn(t *testing.T) {
	t.Setenv(EnvE2EScanNoPlanetSeed, "true")

	config := ConfigFromEnv()

	if !config.E2EScanNoPlanetSeed {
		t.Fatal("E2EScanNoPlanetSeed = false, want true when env is true")
	}
}

func TestConfigFromEnvClientStaticDir(t *testing.T) {
	t.Setenv(EnvClientStaticDir, " client/dist ")

	config := ConfigFromEnv()

	if config.ClientStaticDir != "client/dist" {
		t.Fatalf("ClientStaticDir = %q, want client/dist", config.ClientStaticDir)
	}
}

func TestConfigFromEnvPlaytestSeedOptIn(t *testing.T) {
	t.Setenv(EnvPlaytestSeed, "true")

	config := ConfigFromEnv()

	if !config.PlaytestSeed {
		t.Fatal("PlaytestSeed = false, want true when env is true")
	}
}

func TestConfigFromEnvContentDB(t *testing.T) {
	t.Setenv(contentdb.EnvDatabaseURL, " postgres://gameproject:pw@localhost:5432/gameproject?sslmode=disable ")
	t.Setenv(contentdb.EnvMode, string(contentdb.ContentModeRequired))
	t.Setenv(contentdb.EnvMigrations, string(contentdb.MigrationModeVerify))

	config := ConfigFromEnv()

	if config.ContentDB.DatabaseURL != "postgres://gameproject:pw@localhost:5432/gameproject?sslmode=disable" {
		t.Fatalf("ContentDB.DatabaseURL = %q, want trimmed URL", config.ContentDB.DatabaseURL)
	}
	if config.ContentDB.Mode != contentdb.ContentModeRequired {
		t.Fatalf("ContentDB.Mode = %q, want required", config.ContentDB.Mode)
	}
	if config.ContentDB.Migrations != contentdb.MigrationModeVerify {
		t.Fatalf("ContentDB.Migrations = %q, want verify", config.ContentDB.Migrations)
	}
}

func TestConfigFromEnvCoreStoreMode(t *testing.T) {
	t.Setenv(EnvCoreStoreMode, string(contentdb.ContentModeRequired))

	config := ConfigFromEnv()

	if config.CoreStoreMode != contentdb.ContentModeRequired {
		t.Fatalf("CoreStoreMode = %q, want required", config.CoreStoreMode)
	}
}

func TestNewRejectsRequiredContentDBWithoutURL(t *testing.T) {
	_, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		ContentDB:      contentdb.Config{Mode: contentdb.ContentModeRequired},
	})
	if !errors.Is(err, contentdb.ErrMissingDatabaseURL) {
		t.Fatalf("New() error = %v, want ErrMissingDatabaseURL", err)
	}
}

func TestNewRuntimeRejectsRequiredCoreStoreWithoutURL(t *testing.T) {
	_, err := NewRuntime(RuntimeConfig{
		SessionTTL:        time.Hour,
		WorldID:           foundation.WorldID("world-1"),
		ContentRepository: &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)},
		ContentDB:         contentdb.Config{Mode: contentdb.ContentModeOff},
		CoreStoreMode:     contentdb.ContentModeRequired,
		Passwords:         auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if !errors.Is(err, contentdb.ErrMissingDatabaseURL) {
		t.Fatalf("NewRuntime() error = %v, want ErrMissingDatabaseURL", err)
	}
}

func TestNewRuntimeCoreStoreDevFallbackWithoutURLUsesMemory(t *testing.T) {
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
	if _, ok := runtime.HangarStore.(*ships.InMemoryHangarStore); !ok {
		t.Fatalf("HangarStore = %T, want *ships.InMemoryHangarStore", runtime.HangarStore)
	}
}

func TestNewRejectsStaticContentFallbackOutsideDevMode(t *testing.T) {
	_, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		ContentDB:      contentdb.Config{Mode: contentdb.ContentModeOff},
	})
	if !errors.Is(err, contentdb.ErrContentDatabaseDisabled) {
		t.Fatalf("New() error = %v, want ErrContentDatabaseDisabled", err)
	}
}

func TestNewRejectsE2EPlanetClaimSeedOutsideDevMode(t *testing.T) {
	_, err := New(Config{
		AllowedOrigins:     []string{testOrigin},
		E2EPlanetClaimSeed: true,
	})
	if err == nil {
		t.Fatal("New() error = nil, want E2E seed guard")
	}
	if !strings.Contains(err.Error(), EnvE2EPlanetClaimSeed) || !strings.Contains(err.Error(), EnvDevMode) {
		t.Fatalf("New() error = %q, want seed/dev-mode guard", err)
	}
}

func TestNewRejectsE2ERouteSeedOutsideDevMode(t *testing.T) {
	_, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		E2ERouteSeed:   true,
	})
	if err == nil {
		t.Fatal("New() error = nil, want E2E route seed guard")
	}
	if !strings.Contains(err.Error(), EnvE2ERouteSeed) || !strings.Contains(err.Error(), EnvDevMode) {
		t.Fatalf("New() error = %q, want route seed/dev-mode guard", err)
	}
}

func TestNewRejectsE2EScanNoPlanetSeedOutsideDevMode(t *testing.T) {
	_, err := New(Config{
		AllowedOrigins:      []string{testOrigin},
		E2EScanNoPlanetSeed: true,
	})
	if err == nil {
		t.Fatal("New() error = nil, want E2E scan no-planet seed guard")
	}
	if !strings.Contains(err.Error(), EnvE2EScanNoPlanetSeed) || !strings.Contains(err.Error(), EnvDevMode) {
		t.Fatalf("New() error = %q, want scan seed/dev-mode guard", err)
	}
}

func TestNewAllowsE2ERouteSeedInDevMode(t *testing.T) {
	if _, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		DevMode:        true,
		E2ERouteSeed:   true,
	}); err != nil {
		t.Fatalf("New() error = %v, want nil in dev mode", err)
	}
}

func TestNewAllowsE2EPlanetClaimSeedInDevMode(t *testing.T) {
	if _, err := New(Config{
		AllowedOrigins:     []string{testOrigin},
		DevMode:            true,
		E2EPlanetClaimSeed: true,
	}); err != nil {
		t.Fatalf("New() error = %v, want nil in dev mode", err)
	}
}

func TestNewAllowsE2EScanNoPlanetSeedInDevMode(t *testing.T) {
	if _, err := New(Config{
		AllowedOrigins:      []string{testOrigin},
		DevMode:             true,
		E2EScanNoPlanetSeed: true,
	}); err != nil {
		t.Fatalf("New() error = %v, want nil in dev mode", err)
	}
}

func TestNewRuntimeRejectsE2EPlanetClaimSeedOutsideDevMode(t *testing.T) {
	_, err := NewRuntime(RuntimeConfig{E2EPlanetClaimSeed: true})
	if err == nil {
		t.Fatal("NewRuntime() error = nil, want E2E seed guard")
	}
	if !strings.Contains(err.Error(), EnvE2EPlanetClaimSeed) || !strings.Contains(err.Error(), EnvDevMode) {
		t.Fatalf("NewRuntime() error = %q, want seed/dev-mode guard", err)
	}
}

func TestNewRuntimeRejectsE2ERouteSeedOutsideDevMode(t *testing.T) {
	_, err := NewRuntime(RuntimeConfig{E2ERouteSeed: true})
	if err == nil {
		t.Fatal("NewRuntime() error = nil, want E2E route seed guard")
	}
	if !strings.Contains(err.Error(), EnvE2ERouteSeed) || !strings.Contains(err.Error(), EnvDevMode) {
		t.Fatalf("NewRuntime() error = %q, want route seed/dev-mode guard", err)
	}
}

func TestNewRuntimeRejectsE2EScanNoPlanetSeedOutsideDevMode(t *testing.T) {
	_, err := NewRuntime(RuntimeConfig{E2EScanNoPlanetSeed: true})
	if err == nil {
		t.Fatal("NewRuntime() error = nil, want E2E scan seed guard")
	}
	if !strings.Contains(err.Error(), EnvE2EScanNoPlanetSeed) || !strings.Contains(err.Error(), EnvDevMode) {
		t.Fatalf("NewRuntime() error = %q, want scan seed/dev-mode guard", err)
	}
}

func TestNewRuntimeRejectsRequiredContentDBWithoutURL(t *testing.T) {
	_, err := NewRuntime(RuntimeConfig{ContentDB: contentdb.Config{Mode: contentdb.ContentModeRequired}})
	if !errors.Is(err, contentdb.ErrMissingDatabaseURL) {
		t.Fatalf("NewRuntime() error = %v, want ErrMissingDatabaseURL", err)
	}
}
