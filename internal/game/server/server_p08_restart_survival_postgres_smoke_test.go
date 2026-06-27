package server

import (
	"context"
	"os"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestP08PostgresClaimDuplicateAfterRuntimeRestartConsumesOneXCore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	schemaURL := p08RestartSurvivalPostgresSchemaURL(t, ctx)
	clockTime := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

	first := p08RestartSurvivalServer(t, schemaURL, testutil.NewFakeClock(clockTime))
	owner := createResolvedRuntimeSession(t, first, "p08-claim-duplicate@example.com", "P08 Claim Duplicate")
	planetID := foundation.PlanetID("p08-claim-duplicate-planet")
	seedKnownClaimPlanetForTest(t, first, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 1)
	grantClaimXCoreForTest(t, first, owner.PlayerID, 1, "p08-claim-duplicate-xcore")
	if response := claimPlanetForTest(t, first, owner.SessionID, "request-p08-claim-duplicate-first", planetID); response.HasError {
		t.Fatalf("first claim response error = %+v, want success", response.Error)
	}
	if err := first.runtime.Close(); err != nil {
		t.Fatalf("Close(first runtime) error = %v, want nil", err)
	}

	second := p08RestartSurvivalServer(t, schemaURL, testutil.NewFakeClock(clockTime))
	defer second.runtime.Close()
	claimReference, err := planetClaimReference(owner.PlayerID, planetID)
	if err != nil {
		t.Fatalf("planetClaimReference: %v", err)
	}
	lifecycle, ok, err := second.runtime.ClaimLifecycles.CommittedClaimDurableLifecyclePlan(claimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(after restart) = ok %v err %v, want true nil", ok, err)
	}
	if _, err := second.runtime.Discovery.MaterializePlanet(discovery.MaterializePlanetInput{
		CandidateKey: "candidate-p08-claim-duplicate-restart",
		Planet:       lifecycle.Begin.Planet,
	}); err != nil {
		t.Fatalf("MaterializePlanet(restart owner replay) error = %v, want nil", err)
	}
	p08UpsertClaimIntelForTest(t, second, owner.PlayerID, lifecycle.Begin.Planet)
	duplicate, err := second.runtime.Claim.ClaimPlanet(discovery.ClaimPlanetInput{
		PlayerID:       owner.PlayerID,
		PlanetID:       planetID,
		ClaimReference: claimReference,
	})
	if err != nil {
		t.Fatalf("ClaimPlanet(duplicate after restart) error = %v, want nil", err)
	}
	if !duplicate.Claimed || (!duplicate.Duplicate && !duplicate.AlreadyOwned) {
		t.Fatalf("duplicate claim after restart = %+v, want accepted replay without another debit", duplicate)
	}

	if got := inventoryStackQuantityForTest(second, owner.PlayerID, "x_core"); got != 0 {
		t.Fatalf("x_core quantity after restart duplicate = %d, want 0", got)
	}
	if got := claimXCoreDecreaseLedgerCountForTest(second, owner.PlayerID); got != 1 {
		t.Fatalf("x_core decrease ledger entries after restart duplicate = %d, want one", got)
	}
}

func TestP08PostgresClaimProductionInitRecoveryAfterRuntimeRestartRestoresLiveState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	schemaURL := p08RestartSurvivalPostgresSchemaURL(t, ctx)
	clockTime := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

	first := p08RestartSurvivalServer(t, schemaURL, testutil.NewFakeClock(clockTime))
	owner := createResolvedRuntimeSession(t, first, "p08-claim-init-recovery@example.com", "P08 Claim Init Recovery")
	planetID := foundation.PlanetID("p08-claim-init-recovery-planet")
	seedKnownClaimPlanetForTest(t, first, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 1)
	grantClaimXCoreForTest(t, first, owner.PlayerID, 1, "p08-claim-init-recovery-xcore")
	staleMarker := &failingClaimListedIntelStaleMarker{
		err:         errP08RestartSurvivalStaleMarker,
		markedCount: 1,
	}
	installRuntimeClaimServiceForTest(t, first, staleMarker)

	failed := claimPlanetForTest(t, first, owner.SessionID, "request-p08-claim-init-recovery-fail", planetID)
	if !failed.HasError {
		t.Fatal("claim response error missing, want pending side-effect failure")
	}
	claimReference, err := planetClaimReference(owner.PlayerID, planetID)
	if err != nil {
		t.Fatalf("planetClaimReference: %v", err)
	}
	staleMarker.err = nil
	if _, err := first.runtime.Claim.ClaimPlanet(discovery.ClaimPlanetInput{
		PlayerID:       owner.PlayerID,
		PlanetID:       planetID,
		ClaimReference: claimReference,
	}); err != nil {
		t.Fatalf("ClaimPlanet(domain retry) error = %v, want nil", err)
	}
	lifecycle, ok, err := first.runtime.Claim.ClaimDurableLifecyclePlan(claimReference)
	if err != nil || !ok {
		t.Fatalf("ClaimDurableLifecyclePlan() = ok %v err %v, want true nil", ok, err)
	}
	if _, err := lifecycle.ApplyDurableLifecycle(first.runtime.ClaimLifecycles); err != nil {
		t.Fatalf("ApplyDurableLifecycle() error = %v, want nil", err)
	}
	if err := first.runtime.Close(); err != nil {
		t.Fatalf("Close(first runtime) error = %v, want nil", err)
	}

	second := p08RestartSurvivalServer(t, schemaURL, testutil.NewFakeClock(clockTime))
	defer second.runtime.Close()
	drain, err := second.runtime.DrainDurableOutboxes(RuntimeDurableOutboxDrainInput{
		Limit:                                 10,
		RecoverClaimProductionInitializations: true,
	})
	if err != nil {
		t.Fatalf("DrainDurableOutboxes(recover init after restart) error = %v, want nil", err)
	}
	recovered := drain.RecoveredClaimProductionInitializations
	if recovered.Scanned != 1 || recovered.Completed != 1 || len(recovered.References) != 1 || recovered.References[0] != claimReference {
		t.Fatalf("recovered production init after restart = %+v, want one completed reference %q", recovered, claimReference)
	}
	if snapshot, ok, err := second.runtime.Production.Snapshot(planetID); err != nil || !ok || !snapshot.State.ProductionEnabled || snapshot.Storage.CapacityUnits == 0 {
		t.Fatalf("production snapshot after restart recovery = %+v ok %v err %v, want live initialized state", snapshot, ok, err)
	}
}

func TestP08PostgresRouteSettlementAfterRuntimeRestartAppliesOneWindowOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	schemaURL := p08RestartSurvivalPostgresSchemaURL(t, ctx)
	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

	first := p08RestartSurvivalServer(t, schemaURL, testutil.NewFakeClock(base))
	owner := createResolvedRuntimeSession(t, first, "p08-route-settle@example.com", "P08 Route Settle")
	sourcePlanetID := foundation.PlanetID("p08-route-settle-source")
	destinationPlanetID := foundation.PlanetID("p08-route-settle-destination")
	routeID := foundation.RouteID("p08-route-settle")
	seedOwnedProductionPlanetForTest(t, first, owner.PlayerID, sourcePlanetID, first.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-p08-route-settle-source")
	seedOwnedProductionPlanetForTest(t, first, owner.PlayerID, destinationPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-p08-route-settle-destination")
	saveRouteControlStorage(t, first, sourcePlanetID, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}})
	seedAutomationRouteForTest(t, first, owner.PlayerID, routeID, sourcePlanetID, destinationPlanetID, "map_1_1", "map_1_2")
	firstSettle, err := first.runtime.applyRouteSettlement(owner.PlayerID, routeID, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("applyRouteSettlement(before restart) error = %v, want nil", err)
	}
	if firstSettle.Settlement.NoOp || firstSettle.Settlement.TakenAmount != 40 || firstSettle.Settlement.AddedAmount != 40 {
		t.Fatalf("route settlement before restart = %+v, want one applied transfer", firstSettle.Settlement)
	}
	if err := first.runtime.Close(); err != nil {
		t.Fatalf("Close(first runtime) error = %v, want nil", err)
	}

	second := p08RestartSurvivalServer(t, schemaURL, testutil.NewFakeClock(base.Add(time.Hour)))
	defer second.runtime.Close()
	ledgerRowsBefore := len(second.runtime.Settlements.RouteStorageLedgerEntries())
	if ledgerRowsBefore != 2 {
		t.Fatalf("route settlement durable ledger rows after restart = %d, want one source/debit window", ledgerRowsBefore)
	}

	route, err := second.runtime.routeSettleRouteForOwner(owner.PlayerID, routeID)
	if err != nil {
		t.Fatalf("routeSettleRouteForOwner(after restart) error = %v, want durable route", err)
	}
	if route.RouteID != routeID || route.OwnerPlayerID != owner.PlayerID {
		t.Fatalf("durable route after restart = %+v, want owner route %q", route, routeID)
	}
	secondSettle, err := second.runtime.applyRouteSettlement(owner.PlayerID, routeID, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("applyRouteSettlement(after restart replay) error = %v, want nil", err)
	}
	if !secondSettle.Settlement.NoOp {
		t.Fatalf("route settlement after restart replay = %+v, want no-op replay", secondSettle.Settlement)
	}
	if got := len(second.runtime.Settlements.RouteStorageLedgerEntries()); got != ledgerRowsBefore {
		t.Fatalf("route settlement durable ledger rows after restart replay = %d, want unchanged %d", got, ledgerRowsBefore)
	}
}

func TestP08PostgresDurableOutboxAfterRuntimeRestartPublishesOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	schemaURL := p08RestartSurvivalPostgresSchemaURL(t, ctx)
	clockTime := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

	first := p08RestartSurvivalServer(t, schemaURL, testutil.NewFakeClock(clockTime))
	owner := createResolvedRuntimeSession(t, first, "p08-durable-outbox@example.com", "P08 Durable Outbox")
	planetID := foundation.PlanetID("p08-durable-outbox-planet")
	seedKnownClaimPlanetForTest(t, first, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 1)
	grantClaimXCoreForTest(t, first, owner.PlayerID, 1, "p08-durable-outbox-xcore")
	if response := claimPlanetForTest(t, first, owner.SessionID, "request-p08-durable-outbox-claim", planetID); response.HasError {
		t.Fatalf("claim setup response error = %+v, want success", response.Error)
	}
	if err := first.runtime.Close(); err != nil {
		t.Fatalf("Close(first runtime) error = %v, want nil", err)
	}

	second := p08RestartSurvivalServer(t, schemaURL, testutil.NewFakeClock(clockTime))
	defer second.runtime.Close()
	published := 0
	result, err := second.runtime.DrainDurableOutboxes(RuntimeDurableOutboxDrainInput{
		Limit: 10,
		Now:   clockTime.Add(time.Minute),
		PublishClaim: func(discovery.ClaimOutboxRecord) error {
			published++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("DrainDurableOutboxes(publish claim after restart) error = %v, want nil", err)
	}
	if len(result.Claims) != 1 || published != 1 {
		t.Fatalf("claim outbox publish after restart result rows=%d callback=%d, want 1/1", len(result.Claims), published)
	}
	again, err := second.runtime.DrainDurableOutboxes(RuntimeDurableOutboxDrainInput{
		Limit: 10,
		Now:   clockTime.Add(2 * time.Minute),
		PublishClaim: func(discovery.ClaimOutboxRecord) error {
			published++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("second DrainDurableOutboxes(publish claim after restart) error = %v, want nil", err)
	}
	if len(again.Claims) != 0 || published != 1 {
		t.Fatalf("second claim outbox publish rows=%d callback total=%d, want 0/1", len(again.Claims), published)
	}
	records := second.runtime.ClaimLifecycles.OutboxRecords()
	if len(records) != 1 || records[0].Status != discovery.ClaimOutboxStatusPublished {
		t.Fatalf("claim outbox records after restart publish = %+v, want one published row", records)
	}
}

var errP08RestartSurvivalStaleMarker = foundation.NewDomainError(foundation.CodeInternal, "p08 stale marker unavailable")

func p08RestartSurvivalPostgresSchemaURL(t *testing.T, ctx context.Context) string {
	t.Helper()
	databaseURL := os.Getenv(contentdb.EnvDatabaseURL)
	if databaseURL == "" {
		databaseURL = os.Getenv("GAME_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skipf("%s/GAME_DATABASE_URL unset; skipping P08 restart-survival Postgres smoke", contentdb.EnvDatabaseURL)
	}
	return createRuntimeAuthSmokeSchema(t, ctx, databaseURL)
}

func p08RestartSurvivalServer(t *testing.T, schemaURL string, clock foundation.Clock) *Server {
	t.Helper()
	runtime := p08RestartSurvivalRuntime(t, schemaURL, clock)
	return &Server{runtime: runtime}
}

func p08RestartSurvivalRuntime(t *testing.T, schemaURL string, clock foundation.Clock) *Runtime {
	t.Helper()
	runtime, err := NewRuntime(RuntimeConfig{
		Clock:             clock,
		SessionTTL:        time.Hour,
		WorldID:           foundation.WorldID("world-1"),
		ContentDB:         contentdb.Config{DatabaseURL: schemaURL, Mode: contentdb.ContentModeRequired, Migrations: contentdb.MigrationModeAuto},
		CoreStoreMode:     contentdb.ContentModeRequired,
		ContentRepository: &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)},
		Passwords:         auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v, want nil", err)
	}
	return runtime
}

func p08UpsertClaimIntelForTest(t *testing.T, gameServer *Server, playerID foundation.PlayerID, planet discovery.Planet) {
	t.Helper()
	if _, _, err := gameServer.runtime.Discovery.UpsertPlayerPlanetIntel(discovery.PlayerPlanetIntel{
		PlayerID:        playerID,
		PlanetID:        planet.ID,
		WorldID:         planet.WorldID,
		ZoneID:          planet.ZoneID,
		Coordinates:     planet.Coordinates,
		State:           discovery.IntelStateVerified,
		Confidence:      100,
		LastSeenAt:      gameServer.runtime.clock.Now().UTC(),
		SourceType:      discovery.IntelSourceScanSuccess,
		SourceReference: "restart-" + planet.ID.String(),
	}); err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel(restart %s) error = %v, want nil", planet.ID, err)
	}
}
