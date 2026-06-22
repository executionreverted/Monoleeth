package server

import (
	"strings"
	"testing"
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
