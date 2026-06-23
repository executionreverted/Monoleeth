package server

import (
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

func TestE2EPlanetClaimSeedAbsentByDefault(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	resolved := createResolvedRuntimeSession(t, gameServer, "claim-seed-default@example.com", "ClaimSeedDefault")

	if got := inventoryStackQuantityForTest(gameServer, resolved.PlayerID, "x_core"); got != 0 {
		t.Fatalf("x_core quantity = %d, want 0 without E2E seed", got)
	}
	snapshot, err := gameServer.runtime.Progression.GetProgressionSnapshot(resolved.PlayerID)
	if err != nil {
		t.Fatalf("GetProgressionSnapshot() error = %v, want nil", err)
	}
	if snapshot.Player.Rank != progression.MinRank {
		t.Fatalf("progression rank = %d, want default %d without E2E seed", snapshot.Player.Rank, progression.MinRank)
	}
}

func TestE2EPlanetClaimSeedGrantsClaimProofStateIdempotently(t *testing.T) {
	gameServer, err := New(Config{
		AllowedOrigins:     []string{testOrigin},
		DevMode:            true,
		E2EPlanetClaimSeed: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	resolved := createResolvedRuntimeSession(t, gameServer, "claim-seed-enabled@example.com", "ClaimSeedEnabled")
	if got := inventoryStackQuantityForTest(gameServer, resolved.PlayerID, "x_core"); got != 1 {
		t.Fatalf("x_core quantity = %d, want 1 with E2E seed", got)
	}
	assertE2EClaimSeedRank(t, gameServer, resolved.PlayerID)

	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("ensurePlayerSession second call error = %v, want nil", err)
	}
	if got := inventoryStackQuantityForTest(gameServer, resolved.PlayerID, "x_core"); got != 1 {
		t.Fatalf("x_core quantity after second ensure = %d, want 1", got)
	}
	assertE2EClaimSeedRank(t, gameServer, resolved.PlayerID)
}

func TestE2EPlanetClaimSeedCanGrantMultipleXCoreForMatrixProof(t *testing.T) {
	gameServer, err := New(Config{
		AllowedOrigins:      []string{testOrigin},
		DevMode:             true,
		E2EPlanetClaimSeed:  true,
		E2EPlanetClaimCores: 2,
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	resolved := createResolvedRuntimeSession(t, gameServer, "claim-seed-matrix@example.com", "ClaimSeedMatrix")
	if got := inventoryStackQuantityForTest(gameServer, resolved.PlayerID, "x_core"); got != 2 {
		t.Fatalf("x_core quantity = %d, want 2 with matrix E2E seed", got)
	}
	assertE2EClaimSeedRank(t, gameServer, resolved.PlayerID)

	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("ensurePlayerSession second call error = %v, want nil", err)
	}
	if got := inventoryStackQuantityForTest(gameServer, resolved.PlayerID, "x_core"); got != 2 {
		t.Fatalf("x_core quantity after second ensure = %d, want 2", got)
	}
}

func assertE2EClaimSeedRank(t *testing.T, gameServer *Server, playerID foundation.PlayerID) {
	t.Helper()

	snapshot, err := gameServer.runtime.Progression.GetProgressionSnapshot(playerID)
	if err != nil {
		t.Fatalf("GetProgressionSnapshot() error = %v, want nil", err)
	}
	if snapshot.Player.Rank != progression.MaxMVPRank {
		t.Fatalf("progression rank = %d, want %d with E2E seed", snapshot.Player.Rank, progression.MaxMVPRank)
	}
	gameServer.runtime.mu.Lock()
	runtimeRank := gameServer.runtime.players[playerID].Rank
	gameServer.runtime.mu.Unlock()
	if runtimeRank != progression.MaxMVPRank {
		t.Fatalf("runtime player rank = %d, want %d with E2E seed", runtimeRank, progression.MaxMVPRank)
	}
}
