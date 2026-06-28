package server

import (
	"context"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

func TestPlaytestSeedGrantsClaimOnboardingStateWithoutOwnedRoutePlanets(t *testing.T) {
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		PlaytestSeed:      true,
		ContentRepository: staticContentRepositoryForTest(),
	})
	if err != nil {
		t.Fatalf("New(playtest seed) error = %v, want nil", err)
	}

	first := createResolvedRuntimeSession(t, gameServer, "playtest-seed-one@example.com", "Playtest One")
	second := createResolvedRuntimeSession(t, gameServer, "playtest-seed-two@example.com", "Playtest Two")

	assertPlaytestSeedState(t, gameServer, first.PlayerID)
	assertPlaytestSeedState(t, gameServer, second.PlayerID)

	if err := gameServer.runtime.ensurePlayerSession(first); err != nil {
		t.Fatalf("ensurePlayerSession retry error = %v, want nil", err)
	}
	assertPlaytestSeedState(t, gameServer, first.PlayerID)
}

func TestPlaytestSeedIgnoresE2EClaimCoreMatrixQuantity(t *testing.T) {
	gameServer, err := New(Config{
		AllowedOrigins:      []string{testOrigin},
		PlaytestSeed:        true,
		E2EPlanetClaimCores: 2,
		ContentRepository:   staticContentRepositoryForTest(),
	})
	if err != nil {
		t.Fatalf("New(playtest seed) error = %v, want nil", err)
	}

	resolved := createResolvedRuntimeSession(t, gameServer, "playtest-seed-cores@example.com", "Playtest Cores")
	if got := inventoryStackQuantityForTest(gameServer, resolved.PlayerID, "x_core"); got != 1 {
		t.Fatalf("x_core quantity = %d, want playtest default 1", got)
	}
}

func TestDevAccountSeedCreatesTwoAccountsWithTargetCredits(t *testing.T) {
	gameServer, err := New(Config{
		AllowedOrigins:      []string{testOrigin},
		DevMode:             true,
		DisableAuthAttempts: true,
		DevAccountSeed:      true,
		DevAccountPassword:  "dev-password",
		DevAccountCredits:   100000,
		ContentRepository:   staticContentRepositoryForTest(),
	})
	if err != nil {
		t.Fatalf("New(dev account seed) error = %v, want nil", err)
	}

	for _, email := range []string{"pilot1@example.com", "pilot2@example.com"} {
		result, err := gameServer.runtime.Auth.Login(context.Background(), auth.LoginInput{
			Email:    email,
			Password: "dev-password",
		})
		if err != nil {
			t.Fatalf("Login(%q) error = %v, want nil", email, err)
		}
		if got := gameServer.runtime.Wallet.Balance(result.Session.PlayerID, economy.CurrencyBucketCredits); got != 100000 {
			t.Fatalf("dev account %q credits = %d, want 100000", email, got)
		}
	}
}

func TestDevAccountSessionBootstrapAfterSeedIsIdempotent(t *testing.T) {
	gameServer, err := New(Config{
		AllowedOrigins:      []string{testOrigin},
		DevMode:             true,
		DisableAuthAttempts: true,
		DevAccountSeed:      true,
		DevAccountPassword:  "dev-password",
		DevAccountCredits:   100000,
		ContentRepository:   staticContentRepositoryForTest(),
	})
	if err != nil {
		t.Fatalf("New(dev account seed) error = %v, want nil", err)
	}

	result, err := gameServer.runtime.Auth.Login(context.Background(), auth.LoginInput{
		Email:    "pilot1@example.com",
		Password: "dev-password",
	})
	if err != nil {
		t.Fatalf("Login(dev account) error = %v, want nil", err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := gameServer.runtime.ensurePlayerSession(result.Session); err != nil {
			t.Fatalf("ensurePlayerSession attempt %d error = %v, want nil", attempt+1, err)
		}
	}
	if got := gameServer.runtime.Wallet.Balance(result.Session.PlayerID, economy.CurrencyBucketCredits); got != 100000 {
		t.Fatalf("credits after repeated bootstrap = %d, want dev seed target 100000", got)
	}
	if _, ok := gameServer.runtime.itemCatalog["ammunition_laser_lcb_10"]; ok {
		if got := inventoryStackQuantityForTest(gameServer, result.Session.PlayerID, "ammunition_laser_lcb_10"); got != 10000 {
			t.Fatalf("starter ammo after repeated bootstrap = %d, want one seed stack", got)
		}
	}
}

func assertPlaytestSeedState(t *testing.T, gameServer *Server, playerID foundation.PlayerID) {
	t.Helper()

	if got := inventoryStackQuantityForTest(gameServer, playerID, "x_core"); got != 1 {
		t.Fatalf("x_core quantity = %d, want 1 with playtest seed", got)
	}
	snapshot, err := gameServer.runtime.Progression.GetProgressionSnapshot(playerID)
	if err != nil {
		t.Fatalf("GetProgressionSnapshot() error = %v, want nil", err)
	}
	if snapshot.Player.Rank != progression.MaxMVPRank {
		t.Fatalf("progression rank = %d, want %d with playtest seed", snapshot.Player.Rank, progression.MaxMVPRank)
	}

	for _, planetID := range []foundation.PlanetID{playtestRoutePlanetID(playerID, "source"), playtestRoutePlanetID(playerID, "destination")} {
		if _, ok, err := gameServer.runtime.Discovery.Planet(planetID); err != nil || ok {
			t.Fatalf("Discovery.Planet(%q) ok=%v err=%v, want absent for manual playtest seed", planetID, ok, err)
		}
	}
}
