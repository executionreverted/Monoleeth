package server

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestRuntimePublishesPendingDurableOutboxRowsAcrossStores(t *testing.T) {
	gameServer, owner := newRuntimeDurableOutboxTestServer(t)

	claimRows := make([]discovery.ClaimOutboxRecord, 0, 1)
	settlementRows := make([]production.ProductionOutboxRecord, 0, 2)
	buildingRows := make([]production.ProductionOutboxRecord, 0, 2)
	result, err := gameServer.runtime.DrainDurableOutboxes(RuntimeDurableOutboxDrainInput{
		Limit: 10,
		Now:   durableOutboxTestTime(91),
		PublishClaim: func(record discovery.ClaimOutboxRecord) error {
			claimRows = append(claimRows, record)
			return nil
		},
		PublishSettlement: func(record production.ProductionOutboxRecord) error {
			settlementRows = append(settlementRows, record)
			return nil
		},
		PublishBuildingMutation: func(record production.ProductionOutboxRecord) error {
			buildingRows = append(buildingRows, record)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("publishPendingDurableOutbox() error = %v, want nil", err)
	}
	if len(result.Claims) != 1 || len(claimRows) != 1 {
		t.Fatalf("claim durable publish result rows=%d callback=%d, want 1/1", len(result.Claims), len(claimRows))
	}
	if len(result.Settlements) == 0 || len(settlementRows) == 0 {
		t.Fatalf("settlement durable publish result rows=%d callback=%d, want published rows", len(result.Settlements), len(settlementRows))
	}
	if len(result.BuildingMutations) != 2 || len(buildingRows) != 2 {
		t.Fatalf("building durable publish result rows=%d callback=%d, want 2/2", len(result.BuildingMutations), len(buildingRows))
	}
	for _, publish := range result.Claims {
		if !publish.Published || publish.Record.Status != discovery.ClaimOutboxStatusPublished {
			t.Fatalf("claim publish result = %+v, want published", publish)
		}
	}
	assertProductionPublishResultsForTest(t, "settlement", result.Settlements)
	assertProductionPublishResultsForTest(t, "building", result.BuildingMutations)
	assertClaimDurableOutboxStatusForTest(t, gameServer.runtime.ClaimLifecycles.OutboxRecords(), discovery.ClaimOutboxStatusPublished)
	assertProductionDurableOutboxStatusForTest(t, "settlement", gameServer.runtime.Settlements.OutboxRecords(), production.ProductionOutboxStatusPublished)
	assertProductionDurableOutboxStatusForTest(t, "building", gameServer.runtime.BuildingMutations.OutboxRecords(), production.ProductionOutboxStatusPublished)
	if ledger := gameServer.runtime.BuildingMutations.BuildingMaterialLedgerEntries(); len(ledger) != 1 || ledger[0].ReferenceKey.IsZero() {
		t.Fatalf("building material ledger after publish = %+v, want committed ledger untouched", ledger)
	}
	if owner.PlayerID.IsZero() {
		t.Fatal("owner player id missing")
	}
}

func TestRuntimeReleasesExpiredDurableOutboxLeasesAcrossStores(t *testing.T) {
	gameServer, _ := newRuntimeDurableOutboxTestServer(t)
	claimedAt := durableOutboxTestTime(100)
	claimedBefore := durableOutboxTestTime(101)
	releasedAt := durableOutboxTestTime(102)

	if claimed, err := gameServer.runtime.ClaimLifecycles.ClaimPendingClaimOutboxRecordsForPublish(10, claimedAt); err != nil || len(claimed) != 1 {
		t.Fatalf("claim committed outbox claim = %+v/%v, want one row nil", claimed, err)
	}
	if claimed, err := gameServer.runtime.Settlements.ClaimPendingProductionOutboxRecords(10, claimedAt); err != nil || len(claimed) == 0 {
		t.Fatalf("settlement committed outbox claim = %+v/%v, want rows nil", claimed, err)
	}
	if claimed, err := gameServer.runtime.BuildingMutations.ClaimPendingProductionOutboxRecords(10, claimedAt); err != nil || len(claimed) != 2 {
		t.Fatalf("building committed outbox claim = %+v/%v, want two rows nil", claimed, err)
	}

	result, err := gameServer.runtime.DrainDurableOutboxes(RuntimeDurableOutboxDrainInput{
		Limit:                10,
		Now:                  releasedAt,
		ReleaseExpiredLeases: true,
		LeaseTimeout:         releasedAt.Sub(claimedBefore),
	})
	if err != nil {
		t.Fatalf("releaseExpiredDurableOutboxLeases() error = %v, want nil", err)
	}
	if len(result.ReleasedClaims) != 1 {
		t.Fatalf("released claim rows = %+v, want one row", result.ReleasedClaims)
	}
	if len(result.ReleasedSettlements) == 0 {
		t.Fatalf("released settlement rows = %+v, want rows", result.ReleasedSettlements)
	}
	if len(result.ReleasedBuildingMutations) != 2 {
		t.Fatalf("released building rows = %+v, want two rows", result.ReleasedBuildingMutations)
	}
	assertClaimDurableOutboxStatusForTest(t, result.ReleasedClaims, discovery.ClaimOutboxStatusPending)
	assertProductionDurableOutboxStatusForTest(t, "released settlement", result.ReleasedSettlements, production.ProductionOutboxStatusPending)
	assertProductionDurableOutboxStatusForTest(t, "released building", result.ReleasedBuildingMutations, production.ProductionOutboxStatusPending)
	for _, record := range append(result.ReleasedSettlements, result.ReleasedBuildingMutations...) {
		if !record.ClaimedAt.IsZero() || record.ClaimToken != "" || !record.RetriedAt.Equal(releasedAt) {
			t.Fatalf("released production row = %+v, want cleared lease and retried_at %s", record, releasedAt)
		}
	}
}

func TestRuntimeDurableOutboxRecordsPublishFailures(t *testing.T) {
	gameServer, _ := newRuntimeDurableOutboxTestServer(t)
	temporaryErr := errors.New("broker unavailable")

	result, err := gameServer.runtime.DrainDurableOutboxes(RuntimeDurableOutboxDrainInput{
		Limit: 1,
		Now:   durableOutboxTestTime(111),
		PublishClaim: func(discovery.ClaimOutboxRecord) error {
			return temporaryErr
		},
		PublishSettlement: func(production.ProductionOutboxRecord) error {
			return temporaryErr
		},
		PublishBuildingMutation: func(production.ProductionOutboxRecord) error {
			return temporaryErr
		},
	})
	if err != nil {
		t.Fatalf("publishPendingDurableOutbox(failures) error = %v, want nil", err)
	}
	if len(result.Claims) != 1 || !result.Claims[0].Failed || result.Claims[0].Record.LastError != temporaryErr.Error() {
		t.Fatalf("claim failure result = %+v, want failed with error", result.Claims)
	}
	if len(result.Settlements) != 1 || !result.Settlements[0].Failed || result.Settlements[0].Record.LastError != temporaryErr.Error() {
		t.Fatalf("settlement failure result = %+v, want failed with error", result.Settlements)
	}
	if len(result.BuildingMutations) != 1 || !result.BuildingMutations[0].Failed || result.BuildingMutations[0].Record.LastError != temporaryErr.Error() {
		t.Fatalf("building failure result = %+v, want failed with error", result.BuildingMutations)
	}
}

func TestRuntimeDurableOutboxRealtimeProjectionQueuesSafeOwnerEvents(t *testing.T) {
	gameServer, owner := newRuntimeDurableOutboxTestServer(t)
	other := createResolvedRuntimeSession(t, gameServer, "runtime-durable-outbox-other@example.com", "Other Pilot")
	clearQueuedRuntimeEventsForTest(t, gameServer.runtime)

	result, err := gameServer.runtime.DrainDurableOutboxesToRealtime(RuntimeDurableOutboxRealtimeInput{
		Limit: 10,
		Now:   durableOutboxTestTime(121),
	})
	if err != nil {
		t.Fatalf("DrainDurableOutboxesToRealtime() error = %v, want nil", err)
	}
	if len(result.Claims) != 1 || len(result.Settlements) == 0 || len(result.BuildingMutations) != 2 {
		t.Fatalf("realtime durable drain result = %+v, want claim, settlement, and building rows", result)
	}
	assertProductionPublishResultsForTest(t, "settlement realtime", result.Settlements)
	assertProductionPublishResultsForTest(t, "building realtime", result.BuildingMutations)
	for _, publish := range result.Claims {
		if !publish.Published || publish.Record.Status != discovery.ClaimOutboxStatusPublished {
			t.Fatalf("claim realtime publish result = %+v, want published", publish)
		}
	}

	gameServer.runtime.mu.Lock()
	eventsBySession := gameServer.runtime.drainQueuedEventsBySessionLocked()
	gameServer.runtime.mu.Unlock()
	ownerEvents := eventsBySession[owner.SessionID]
	if len(ownerEvents) == 0 {
		t.Fatalf("owner realtime events = 0, want durable projection events")
	}
	if events := eventsBySession[other.SessionID]; len(events) != 0 {
		t.Fatalf("other realtime events = %+v, want none", events)
	}

	assertClaimedEventSafeForTest(t, requireEventTypeForTest(t, ownerEvents, realtime.EventPlanetClaimed), owner.PlayerID)
	requireEventTypeForTest(t, ownerEvents, realtime.EventKnownPlanets)
	requireEventTypeForTest(t, ownerEvents, realtime.EventPlanetDetail)
	requireEventTypeForTest(t, ownerEvents, realtime.EventInventorySnapshot)
	requireEventTypeForTest(t, ownerEvents, realtime.EventWalletSnapshot)
	assertSafeProductionRealtimePayload(t, "durable production summary event", requireEventTypeForTest(t, ownerEvents, realtime.EventProductionSummary).Payload, owner.PlayerID)
	assertSafeProductionRealtimePayload(t, "durable storage summary event", requireEventTypeForTest(t, ownerEvents, realtime.EventPlanetStorage).Payload, owner.PlayerID)
}

func TestRuntimeDurableOutboxRealtimeProjectionNoActiveSessionPublishesNoOp(t *testing.T) {
	gameServer, owner := newRuntimeDurableOutboxTestServer(t)
	clearQueuedRuntimeEventsForTest(t, gameServer.runtime)
	gameServer.runtime.mu.Lock()
	delete(gameServer.runtime.sessions, owner.SessionID)
	delete(gameServer.runtime.sessionLocations, owner.SessionID)
	delete(gameServer.runtime.sessionEpochs, owner.SessionID)
	gameServer.runtime.mu.Unlock()

	result, err := gameServer.runtime.DrainDurableOutboxesToRealtime(RuntimeDurableOutboxRealtimeInput{
		Limit: 10,
		Now:   durableOutboxTestTime(122),
	})
	if err != nil {
		t.Fatalf("DrainDurableOutboxesToRealtime(no active session) error = %v, want nil", err)
	}
	if len(result.Claims) != 1 || len(result.Settlements) == 0 || len(result.BuildingMutations) != 2 {
		t.Fatalf("no-session realtime durable drain result = %+v, want published rows", result)
	}
	gameServer.runtime.mu.Lock()
	queuedEvents := len(gameServer.runtime.queuedEvents)
	gameServer.runtime.mu.Unlock()
	if queuedEvents != 0 {
		t.Fatalf("queued realtime events = %d, want none for no active session", queuedEvents)
	}
}

func TestRuntimeDurableOutboxRealtimeDrainCollectsSinkEvents(t *testing.T) {
	gameServer, owner := newRuntimeDurableOutboxTestServer(t)
	clearQueuedRuntimeEventsForTest(t, gameServer.runtime)

	result, err := gameServer.runtime.DrainDurableOutboxesToRealtimeAndCollectEvents(RuntimeDurableOutboxRealtimeInput{
		Limit: 10,
		Now:   durableOutboxTestTime(123),
	})
	if err != nil {
		t.Fatalf("DrainDurableOutboxesToRealtimeAndCollectEvents() error = %v, want nil", err)
	}
	if len(result.Drain.Claims) != 1 || len(result.Drain.Settlements) == 0 || len(result.Drain.BuildingMutations) != 2 {
		t.Fatalf("collected realtime durable drain = %+v, want published rows", result.Drain)
	}
	ownerEvents := result.EventsBySession[owner.SessionID]
	if len(ownerEvents) == 0 {
		t.Fatalf("collected owner events = 0, want events for sink delivery")
	}
	requireEventTypeForTest(t, ownerEvents, realtime.EventPlanetClaimed)
	requireEventTypeForTest(t, ownerEvents, realtime.EventProductionSummary)
	requireEventTypeForTest(t, ownerEvents, realtime.EventPlanetStorage)
	requireEventTypeForTest(t, ownerEvents, realtime.EventWalletSnapshot)

	gameServer.runtime.mu.Lock()
	queuedEvents := len(gameServer.runtime.queuedEvents)
	gameServer.runtime.mu.Unlock()
	if queuedEvents != 0 {
		t.Fatalf("queued realtime events after collect = %d, want flushed for sink delivery", queuedEvents)
	}
}

func TestRuntimeDurableOutboxRealtimePumpTickReleasesExpiredLeasesAndFlushesEvents(t *testing.T) {
	gameServer, owner := newRuntimeDurableOutboxTestServer(t)
	clearQueuedRuntimeEventsForTest(t, gameServer.runtime)
	oldClaimedAt := gameServer.runtime.clock.Now().UTC().Add(-2 * runtimeDurableOutboxRealtimePumpLeaseTimeout)
	if claimed, err := gameServer.runtime.ClaimLifecycles.ClaimPendingClaimOutboxRecordsForPublish(10, oldClaimedAt); err != nil || len(claimed) != 1 {
		t.Fatalf("claim durable outbox setup claim = %+v/%v, want one row", claimed, err)
	}
	if claimed, err := gameServer.runtime.Settlements.ClaimPendingProductionOutboxRecords(10, oldClaimedAt); err != nil || len(claimed) == 0 {
		t.Fatalf("settlement durable outbox setup claim = %+v/%v, want rows", claimed, err)
	}
	if claimed, err := gameServer.runtime.BuildingMutations.ClaimPendingProductionOutboxRecords(10, oldClaimedAt); err != nil || len(claimed) != 2 {
		t.Fatalf("building durable outbox setup claim = %+v/%v, want two rows", claimed, err)
	}

	eventsBySession := gameServer.runtime.runDurableOutboxRealtimePumpTick()
	ownerEvents := eventsBySession[owner.SessionID]
	if len(ownerEvents) == 0 {
		t.Fatalf("pump owner events = 0, want released rows published to sink")
	}
	requireEventTypeForTest(t, ownerEvents, realtime.EventPlanetClaimed)
	requireEventTypeForTest(t, ownerEvents, realtime.EventProductionSummary)
	requireEventTypeForTest(t, ownerEvents, realtime.EventPlanetStorage)
	assertClaimDurableOutboxStatusForTest(t, gameServer.runtime.ClaimLifecycles.OutboxRecords(), discovery.ClaimOutboxStatusPublished)
	assertProductionDurableOutboxStatusForTest(t, "settlement pump", gameServer.runtime.Settlements.OutboxRecords(), production.ProductionOutboxStatusPublished)
	assertProductionDurableOutboxStatusForTest(t, "building pump", gameServer.runtime.BuildingMutations.OutboxRecords(), production.ProductionOutboxStatusPublished)
}

func newRuntimeDurableOutboxTestServer(t *testing.T) (*Server, auth.ResolvedSession) {
	t.Helper()
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "runtime-durable-outbox@example.com", "Durable Outbox")

	claimPlanetID := foundation.PlanetID("runtime-durable-outbox-claim")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, claimPlanetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 1)
	grantClaimXCoreForTest(t, gameServer, owner.PlayerID, 1, "runtime-durable-outbox-xcore")
	if response := claimPlanetForTest(t, gameServer, owner.SessionID, "request-runtime-durable-outbox-claim", claimPlanetID); response.HasError {
		t.Fatalf("claim setup response error = %+v, want success", response.Error)
	}

	settlementPlanetID := foundation.PlanetID("runtime-durable-outbox-settlement")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, settlementPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-runtime-durable-outbox-settlement")
	seedActiveProductionBuildingForTest(t, gameServer, settlementPlanetID, "runtime-durable-outbox-iron")
	clock.Advance(2 * time.Hour)
	if response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-runtime-durable-outbox-production","op":"planet.production_summary","payload":{},"client_seq":2,"v":1}`),
	); response.HasError {
		t.Fatalf("production summary setup response error = %+v, want success", response.Error)
	}

	buildingPlanetID := foundation.PlanetID("runtime-durable-outbox-building")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, buildingPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1500, Y: 1400}, "candidate-runtime-durable-outbox-building")
	saveRouteControlStorage(t, gameServer, buildingPlanetID, []production.StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	seedPlanetBuildingWalletCredits(t, gameServer, owner.PlayerID, 500, "quest_reward:runtime-durable-outbox-building")
	if response := handlePlanetBuildingBuildForTest(gameServer, owner.SessionID, buildingPlanetID, "alpha", 3); response.HasError {
		t.Fatalf("building setup response error = %+v, want success", response.Error)
	}
	return gameServer, owner
}

func clearQueuedRuntimeEventsForTest(t *testing.T, runtime *Runtime) {
	t.Helper()
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.queuedEvents = make(map[auth.SessionID][]realtime.EventEnvelope)
}

func assertProductionPublishResultsForTest(t *testing.T, label string, results []production.ProductionOutboxPublishResult) {
	t.Helper()
	for _, result := range results {
		if !result.Published || result.Failed || result.StaleClaim || result.Record.Status != production.ProductionOutboxStatusPublished {
			t.Fatalf("%s publish result = %+v, want published", label, result)
		}
	}
}

func assertClaimDurableOutboxStatusForTest(t *testing.T, records []discovery.ClaimOutboxRecord, status discovery.ClaimOutboxStatus) {
	t.Helper()
	if len(records) == 0 {
		t.Fatalf("claim durable outbox rows = 0, want status %q", status)
	}
	for _, record := range records {
		if record.Status != status {
			t.Fatalf("claim durable outbox row = %+v, want status %q", record, status)
		}
	}
}

func assertProductionDurableOutboxStatusForTest(t *testing.T, label string, records []production.ProductionOutboxRecord, status production.ProductionOutboxStatus) {
	t.Helper()
	if len(records) == 0 {
		t.Fatalf("%s durable outbox rows = 0, want status %q", label, status)
	}
	for _, record := range records {
		if record.Status != status {
			t.Fatalf("%s durable outbox row = %+v, want status %q", label, record, status)
		}
	}
}

func durableOutboxTestTime(seconds int64) time.Time {
	return time.Unix(seconds, 0).UTC()
}
