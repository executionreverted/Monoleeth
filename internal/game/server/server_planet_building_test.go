package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	gameevents "gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestPlanetBuildingBuildAlloyFoundryDebitsServerOwnedCostsAndQueuesOwnerEvents(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "planet-building-build-owner@example.com", "Building Owner")
	other := createResolvedRuntimeSession(t, gameServer, "planet-building-build-other@example.com", "Building Other")
	planetID := foundation.PlanetID("planet-building-build-alloy")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, planetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-building-build-alloy")
	saveRouteControlStorage(t, gameServer, planetID, []production.StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	seedPlanetBuildingWalletCredits(t, gameServer, owner.PlayerID, 500, "quest_reward:planet-building-build-seed")

	requestID := foundation.RequestID("request-planet-building-build-alloy")
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"`+requestID.String()+`","op":"planet.building_build","payload":{"planet_id":"`+planetID.String()+`","building_type":"alloy_foundry","slot":"alpha"},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("planet.building_build response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsPlanetBuildingInternals(t, "planet.building_build response", response.Response.Payload)

	wantBuildingID, err := deterministicPlanetBuildingID(planetID, production.BuildingTypeAlloyFoundry, "alpha")
	if err != nil {
		t.Fatalf("deterministicPlanetBuildingID: %v", err)
	}
	referenceKey, err := foundation.PlanetBuildingBuildIdempotencyKey(planetID, wantBuildingID.String())
	if err != nil {
		t.Fatalf("PlanetBuildingBuildIdempotencyKey: %v", err)
	}
	var payload struct {
		Building      planetBuildingPayload             `json:"building"`
		Production    planetProductionCollectionPayload `json:"production"`
		PlanetStorage planetStorageCollectionPayload    `json:"planet_storage"`
		Wallet        walletSnapshotPayload             `json:"wallet"`
		Duplicate     bool                              `json:"duplicate"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode planet.building_build payload: %v", err)
	}
	if payload.Duplicate {
		t.Fatalf("build duplicate = true, want first mutation")
	}
	if payload.Building.BuildingID != wantBuildingID.String() ||
		payload.Building.BuildingType != production.BuildingTypeAlloyFoundry.String() ||
		payload.Building.Level != 1 ||
		payload.Building.PublicMapKey != "1-1" {
		t.Fatalf("building payload = %+v, want safe alloy foundry L1 on 1-1", payload.Building)
	}
	if got := storageQuantityFromCollection(payload.PlanetStorage, planetID, "iron_ore"); got != 20 {
		t.Fatalf("response storage iron_ore = %d, want 20", got)
	}
	if payload.Wallet.Credits != starterWalletCredits+500-buildingBuildAlloyFoundryCredits {
		t.Fatalf("response wallet credits = %d, want %d", payload.Wallet.Credits, starterWalletCredits+500-buildingBuildAlloyFoundryCredits)
	}
	assertStoredRouteStorageQuantity(t, gameServer, planetID, "iron_ore", 20)
	assertStoredPlanetBuildingLevel(t, gameServer, planetID, wantBuildingID, 1)
	assertPlanetBuildingMutationBoundaryCounts(t, gameServer, owner.PlayerID, 1, 1, 2, 2, 4)
	assertPlanetBuildingDurableCommitCounts(t, gameServer, referenceKey, 1, 2, 1)

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationPlanetBuildingBuild, owner.PlayerID)
	if err != nil {
		t.Fatalf("post planet.building_build events: %v", err)
	}
	if _, leaked := eventsBySession[other.SessionID]; leaked {
		t.Fatalf("planet.building_build events leaked to non-owner session: %+v", eventsBySession[other.SessionID])
	}
	ownerEvents := eventsBySession[owner.SessionID]
	requireEventTypeForTest(t, ownerEvents, realtime.EventProductionSummary)
	requireEventTypeForTest(t, ownerEvents, realtime.EventPlanetStorage)
	requireEventTypeForTest(t, ownerEvents, realtime.EventWalletSnapshot)
	for _, event := range ownerEvents {
		assertPayloadOmitsPlanetBuildingInternals(t, string(event.Type)+" event", event.Payload)
	}

	duplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-planet-building-build-alloy-duplicate","op":"planet.building_build","payload":{"planet_id":"`+planetID.String()+`","building_type":"alloy_foundry","slot":"alpha"},"client_seq":2,"v":1}`),
	)
	if duplicate.HasError {
		t.Fatalf("duplicate planet.building_build response error = %+v, want success", duplicate.Error)
	}
	var duplicatePayload struct {
		Wallet    walletSnapshotPayload `json:"wallet"`
		Duplicate bool                  `json:"duplicate"`
	}
	if err := json.Unmarshal(duplicate.Response.Payload, &duplicatePayload); err != nil {
		t.Fatalf("decode duplicate planet.building_build payload: %v", err)
	}
	if !duplicatePayload.Duplicate {
		t.Fatalf("duplicate build duplicate = false, want true")
	}
	if duplicatePayload.Wallet.Credits != starterWalletCredits+500-buildingBuildAlloyFoundryCredits {
		t.Fatalf("duplicate build wallet credits = %d, want unchanged %d", duplicatePayload.Wallet.Credits, starterWalletCredits+500-buildingBuildAlloyFoundryCredits)
	}
	assertStoredRouteStorageQuantity(t, gameServer, planetID, "iron_ore", 20)
	assertPlanetBuildingMutationBoundaryCounts(t, gameServer, owner.PlayerID, 1, 1, 2, 2, 4)
	assertPlanetBuildingDurableCommitCounts(t, gameServer, referenceKey, 1, 2, 1)
}

func TestPlanetBuildingConcurrentBuildsSerializeBeforeWalletDebit(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "planet-building-concurrent-owner@example.com", "Building Concurrent")
	planetID := foundation.PlanetID("planet-building-concurrent")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, planetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-building-concurrent")
	saveRouteControlStorage(t, gameServer, planetID, []production.StoredItem{{ItemID: "iron_ore", Quantity: 30}})
	seedPlanetBuildingWalletCredits(t, gameServer, owner.PlayerID, 500, "quest_reward:planet-building-concurrent-seed")

	emitter := newBlockingWalletDebitEmitterForTest()
	gameServer.runtime.Wallet.SetEventEmitter(emitter)
	defer gameServer.runtime.Wallet.SetEventEmitter(nil)

	firstDone := make(chan realtime.CachedResponse, 1)
	go func() {
		firstDone <- handlePlanetBuildingBuildForTest(gameServer, owner.SessionID, planetID, "alpha", 1)
	}()
	emitter.waitForDebit(t, "first build")

	secondDone := make(chan realtime.CachedResponse, 1)
	go func() {
		secondDone <- handlePlanetBuildingBuildForTest(gameServer, owner.SessionID, planetID, "beta", 2)
	}()

	select {
	case <-emitter.debits:
		close(emitter.release)
		t.Fatal("second build reached wallet debit while the first production commit was still blocked")
	case response := <-secondDone:
		close(emitter.release)
		t.Fatalf("second build completed before first commit released: %+v", response)
	case <-time.After(250 * time.Millisecond):
	}

	close(emitter.release)
	first := waitPlanetBuildingResponseForTest(t, "first build", firstDone)
	second := waitPlanetBuildingResponseForTest(t, "second build", secondDone)

	if first.HasError {
		t.Fatalf("first build response error = %+v, want success", first.Error)
	}
	if !second.HasError || second.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("second build response = %+v, want forbidden without wallet debit", second)
	}

	alphaBuildingID, err := deterministicPlanetBuildingID(planetID, production.BuildingTypeAlloyFoundry, "alpha")
	if err != nil {
		t.Fatalf("deterministic alpha building id: %v", err)
	}
	betaBuildingID, err := deterministicPlanetBuildingID(planetID, production.BuildingTypeAlloyFoundry, "beta")
	if err != nil {
		t.Fatalf("deterministic beta building id: %v", err)
	}
	assertStoredRouteStorageQuantity(t, gameServer, planetID, "iron_ore", 0)
	assertStoredPlanetBuildingLevel(t, gameServer, planetID, alphaBuildingID, 1)
	if _, ok, lookupErr := gameServer.runtime.Production.Building(planetID, betaBuildingID); lookupErr != nil || ok {
		t.Fatalf("beta building lookup ok=%v err=%v, want no stale building commit", ok, lookupErr)
	}
	if got := gameServer.runtime.Wallet.Balance(owner.PlayerID, economy.CurrencyBucketCredits); got != starterWalletCredits+500-buildingBuildAlloyFoundryCredits {
		t.Fatalf("wallet after concurrent builds = %d, want one debit %d", got, starterWalletCredits+500-buildingBuildAlloyFoundryCredits)
	}
	assertPlanetBuildingMutationBoundaryCounts(t, gameServer, owner.PlayerID, 1, 1, 2, 2, 4)
}

func TestPlanetBuildingBuildRejectsSpoofedServerOwnedFieldsBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "planet-building-spoof@example.com", "Building Spoof")
	planetID := foundation.PlanetID("planet-building-spoof")

	seedOwnedProductionPlanetForTest(t, gameServer, resolved.PlayerID, planetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-building-spoof")
	saveRouteControlStorage(t, gameServer, planetID, []production.StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	seedPlanetBuildingWalletCredits(t, gameServer, resolved.PlayerID, 500, "quest_reward:planet-building-spoof-seed")

	tests := []struct {
		name  string
		field string
	}{
		{name: "owner player", field: `"owner_player_id":"spoofed-player"`},
		{name: "wallet", field: `"wallet":{"credits":999999}`},
		{name: "materials", field: `"materials":[{"item_id":"iron_ore","quantity":1}]`},
		{name: "storage", field: `"storage":{"iron_ore":999999}`},
		{name: "public map key", field: `"public_map_key":"1-1"`},
		{name: "level", field: `"level":2`},
		{name: "definition", field: `"definition_id":"iron_extractor_l1"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(resolved.SessionID.String()),
				[]byte(`{"request_id":"request-planet-building-spoof-`+strings.ReplaceAll(tt.name, " ", "-")+`","op":"planet.building_build","payload":{"planet_id":"`+planetID.String()+`","building_type":"alloy_foundry","slot":"spoof",`+tt.field+`},"client_seq":1,"v":1}`),
			)
			if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
				t.Fatalf("planet.building_build spoof response = %+v, want invalid payload", response)
			}
			if buildings, err := gameServer.runtime.Production.Buildings(planetID); err != nil || len(buildings) != 0 {
				t.Fatalf("buildings after spoofed %s = %+v err=%v, want none", tt.name, buildings, err)
			}
			assertStoredRouteStorageQuantity(t, gameServer, planetID, "iron_ore", 50)
			if got := gameServer.runtime.Wallet.Balance(resolved.PlayerID, economy.CurrencyBucketCredits); got != starterWalletCredits+500 {
				t.Fatalf("wallet after spoofed %s = %d, want %d", tt.name, got, starterWalletCredits+500)
			}
			assertPlanetBuildingMutationBoundaryCounts(t, gameServer, resolved.PlayerID, 0, 0, 0, 0, 3)
			assertNoQueuedEventsForSession(t, gameServer, resolved.SessionID)
		})
	}
}

func TestPlanetBuildingBuildRejectsWrongOwnerAndOtherMapWithoutMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "planet-building-access-owner@example.com", "Building Access Owner")
	other := createResolvedRuntimeSession(t, gameServer, "planet-building-access-other@example.com", "Building Access Other")
	wrongOwnerPlanetID := foundation.PlanetID("planet-building-wrong-owner")
	otherMapPlanetID := foundation.PlanetID("planet-building-other-map")

	seedOwnedProductionPlanetForTest(t, gameServer, other.PlayerID, wrongOwnerPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-building-wrong-owner")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, otherMapPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-building-other-map")
	saveRouteControlStorage(t, gameServer, wrongOwnerPlanetID, []production.StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	saveRouteControlStorage(t, gameServer, otherMapPlanetID, []production.StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	seedPlanetBuildingWalletCredits(t, gameServer, owner.PlayerID, 500, "quest_reward:planet-building-access-seed")

	tests := []struct {
		name     string
		planetID foundation.PlanetID
	}{
		{name: "wrong owner", planetID: wrongOwnerPlanetID},
		{name: "other map", planetID: otherMapPlanetID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(owner.SessionID.String()),
				[]byte(`{"request_id":"request-planet-building-access-`+strings.ReplaceAll(tt.name, " ", "-")+`","op":"planet.building_build","payload":{"planet_id":"`+tt.planetID.String()+`","building_type":"alloy_foundry","slot":"alpha"},"client_seq":1,"v":1}`),
			)
			if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
				t.Fatalf("planet.building_build %s response = %+v, want safe not-found", tt.name, response)
			}
			if buildings, err := gameServer.runtime.Production.Buildings(tt.planetID); err != nil || len(buildings) != 0 {
				t.Fatalf("buildings after %s = %+v err=%v, want none", tt.name, buildings, err)
			}
			assertStoredRouteStorageQuantity(t, gameServer, tt.planetID, "iron_ore", 50)
			if got := gameServer.runtime.Wallet.Balance(owner.PlayerID, economy.CurrencyBucketCredits); got != starterWalletCredits+500 {
				t.Fatalf("wallet after %s = %d, want %d", tt.name, got, starterWalletCredits+500)
			}
			assertNoQueuedEventsForSession(t, gameServer, owner.SessionID)
		})
	}
}

func TestPlanetBuildingBuildRejectsInsufficientMaterialsBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "planet-building-materials@example.com", "Building Materials")
	planetID := foundation.PlanetID("planet-building-materials")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, planetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-building-materials")
	saveRouteControlStorage(t, gameServer, planetID, []production.StoredItem{{ItemID: "iron_ore", Quantity: 10}})
	seedPlanetBuildingWalletCredits(t, gameServer, owner.PlayerID, 500, "quest_reward:planet-building-materials-seed")

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-planet-building-materials","op":"planet.building_build","payload":{"planet_id":"`+planetID.String()+`","building_type":"alloy_foundry","slot":"alpha"},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("insufficient materials build response = %+v, want forbidden", response)
	}
	if buildings, err := gameServer.runtime.Production.Buildings(planetID); err != nil || len(buildings) != 0 {
		t.Fatalf("buildings after insufficient materials = %+v err=%v, want none", buildings, err)
	}
	assertStoredRouteStorageQuantity(t, gameServer, planetID, "iron_ore", 10)
	if got := gameServer.runtime.Wallet.Balance(owner.PlayerID, economy.CurrencyBucketCredits); got != starterWalletCredits+500 {
		t.Fatalf("wallet after insufficient materials = %d, want %d", got, starterWalletCredits+500)
	}
	assertPlanetBuildingMutationBoundaryCounts(t, gameServer, owner.PlayerID, 0, 0, 0, 0, 3)
	assertNoQueuedEventsForSession(t, gameServer, owner.SessionID)
}

func TestPlanetBuildingBuildRejectsInsufficientWalletBeforeMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "planet-building-wallet@example.com", "Building Wallet")
	planetID := foundation.PlanetID("planet-building-wallet")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, planetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-building-wallet")
	saveRouteControlStorage(t, gameServer, planetID, []production.StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	if _, err := gameServer.runtime.Wallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     owner.PlayerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       starterWalletCredits,
		Reason:       economy.LedgerReason("test_drain"),
		ReferenceKey: foundation.IdempotencyKey("quest_reward:planet-building-wallet-drain"),
	}); err != nil {
		t.Fatalf("DebitWallet(drain) error = %v, want nil", err)
	}
	walletLedgerBefore := countPlanetBuildingWalletLedgerEntriesForPlayer(gameServer.runtime.Wallet.CurrencyLedgerEntries(), owner.PlayerID)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-planet-building-wallet","op":"planet.building_build","payload":{"planet_id":"`+planetID.String()+`","building_type":"alloy_foundry","slot":"alpha"},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotEnoughFunds {
		t.Fatalf("insufficient wallet build response = %+v, want not enough funds", response)
	}
	if buildings, err := gameServer.runtime.Production.Buildings(planetID); err != nil || len(buildings) != 0 {
		t.Fatalf("buildings after insufficient wallet = %+v err=%v, want none", buildings, err)
	}
	assertStoredRouteStorageQuantity(t, gameServer, planetID, "iron_ore", 50)
	if got := gameServer.runtime.Wallet.Balance(owner.PlayerID, economy.CurrencyBucketCredits); got != 0 {
		t.Fatalf("wallet after insufficient wallet = %d, want unchanged 0", got)
	}
	if got := countPlanetBuildingWalletLedgerEntriesForPlayer(gameServer.runtime.Wallet.CurrencyLedgerEntries(), owner.PlayerID); got != walletLedgerBefore {
		t.Fatalf("wallet ledger entries after insufficient wallet = %d, want unchanged %d", got, walletLedgerBefore)
	}
	assertPlanetBuildingMutationBoundaryCounts(t, gameServer, owner.PlayerID, 0, 0, 0, 0, walletLedgerBefore)
	assertNoQueuedEventsForSession(t, gameServer, owner.SessionID)
}

func TestPlanetBuildingDurableCommitRequiresRecordedReference(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	referenceKey, err := foundation.PlanetBuildingBuildIdempotencyKey("planet-missing", "building-missing")
	if err != nil {
		t.Fatalf("PlanetBuildingBuildIdempotencyKey: %v", err)
	}

	err = gameServer.runtime.applyBuildingMutationDurableCommit(production.BuildingMutationResult{
		ReferenceKey: referenceKey,
		OutboxRecords: []production.ProductionOutboxRecord{
			{ReferenceKey: referenceKey},
		},
	})
	if !errors.Is(err, production.ErrInvalidBuildingMutationDurableCommit) {
		t.Fatalf("applyBuildingMutationDurableCommit(missing reference) error = %v, want ErrInvalidBuildingMutationDurableCommit", err)
	}
	if got := len(gameServer.runtime.BuildingMutations.BuildingMutationReferences()); got != 0 {
		t.Fatalf("durable building references after missing reference = %d, want 0", got)
	}
}

func TestPlanetBuildingDuplicateBuildRepairsMissingDurableCommit(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "planet-building-repair-owner@example.com", "Building Repair Owner")
	planetID := foundation.PlanetID("planet-building-repair")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, planetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-building-repair")
	saveRouteControlStorage(t, gameServer, planetID, []production.StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	seedPlanetBuildingWalletCredits(t, gameServer, owner.PlayerID, 500, "quest_reward:planet-building-repair-seed")

	first := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-planet-building-repair-first","op":"planet.building_build","payload":{"planet_id":"`+planetID.String()+`","building_type":"alloy_foundry","slot":"alpha"},"client_seq":1,"v":1}`),
	)
	if first.HasError {
		t.Fatalf("first planet.building_build response error = %+v, want success", first.Error)
	}
	buildingID, err := deterministicPlanetBuildingID(planetID, production.BuildingTypeAlloyFoundry, "alpha")
	if err != nil {
		t.Fatalf("deterministicPlanetBuildingID: %v", err)
	}
	referenceKey, err := foundation.PlanetBuildingBuildIdempotencyKey(planetID, buildingID.String())
	if err != nil {
		t.Fatalf("PlanetBuildingBuildIdempotencyKey: %v", err)
	}
	gameServer.runtime.BuildingMutations = production.NewInMemoryBuildingMutationDurableCommitStore()
	if got := len(gameServer.runtime.BuildingMutations.BuildingMutationReferences()); got != 0 {
		t.Fatalf("reset durable building references = %d, want 0", got)
	}

	duplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-planet-building-repair-duplicate","op":"planet.building_build","payload":{"planet_id":"`+planetID.String()+`","building_type":"alloy_foundry","slot":"alpha"},"client_seq":2,"v":1}`),
	)
	if duplicate.HasError {
		t.Fatalf("duplicate planet.building_build response error = %+v, want durable repair success", duplicate.Error)
	}
	var duplicatePayload struct {
		Duplicate bool `json:"duplicate"`
	}
	if err := json.Unmarshal(duplicate.Response.Payload, &duplicatePayload); err != nil {
		t.Fatalf("decode duplicate planet.building_build payload: %v", err)
	}
	if !duplicatePayload.Duplicate {
		t.Fatalf("duplicate build flag = false, want true")
	}
	assertPlanetBuildingDurableCommitCounts(t, gameServer, referenceKey, 1, 2, 1)
	assertPlanetBuildingMutationBoundaryCounts(t, gameServer, owner.PlayerID, 1, 1, 2, 2, 4)
}

func TestPlanetBuildingUpgradeIronExtractorDebitsCostsOnceAndUpdatesLevel(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "planet-building-upgrade-owner@example.com", "Building Upgrade Owner")
	planetID := foundation.PlanetID("planet-building-upgrade")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, planetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-building-upgrade")
	saveRouteControlStorage(t, gameServer, planetID, []production.StoredItem{{ItemID: "iron_ore", Quantity: 40}})
	seedPlanetBuildingWalletCredits(t, gameServer, owner.PlayerID, 500, "quest_reward:planet-building-upgrade-seed")
	buildingID := seedPlanetBuildingForTest(t, gameServer, planetID, production.BuildingTypeIronExtractor, "alpha", production.ProductionDefinitionIDIronExtractorL1)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-planet-building-upgrade","op":"planet.building_upgrade","payload":{"planet_id":"`+planetID.String()+`","building_id":"`+buildingID.String()+`","target_level":2},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("planet.building_upgrade response error = %+v, want success", response.Error)
	}
	assertPayloadOmitsPlanetBuildingInternals(t, "planet.building_upgrade response", response.Response.Payload)
	var payload struct {
		Building      planetBuildingPayload          `json:"building"`
		PlanetStorage planetStorageCollectionPayload `json:"planet_storage"`
		Wallet        walletSnapshotPayload          `json:"wallet"`
		Duplicate     bool                           `json:"duplicate"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode planet.building_upgrade payload: %v", err)
	}
	if payload.Duplicate {
		t.Fatalf("upgrade duplicate = true, want first mutation")
	}
	if payload.Building.BuildingID != buildingID.String() || payload.Building.Level != 2 || payload.Building.BuildingType != production.BuildingTypeIronExtractor.String() {
		t.Fatalf("upgrade building payload = %+v, want iron extractor L2", payload.Building)
	}
	if got := storageQuantityFromCollection(payload.PlanetStorage, planetID, "iron_ore"); got != 20 {
		t.Fatalf("upgrade response storage iron_ore = %d, want 20", got)
	}
	if payload.Wallet.Credits != starterWalletCredits+500-buildingUpgradeIronExtractorCredits {
		t.Fatalf("upgrade response wallet credits = %d, want %d", payload.Wallet.Credits, starterWalletCredits+500-buildingUpgradeIronExtractorCredits)
	}
	assertStoredRouteStorageQuantity(t, gameServer, planetID, "iron_ore", 20)
	assertStoredPlanetBuildingLevel(t, gameServer, planetID, buildingID, 2)
	assertPlanetBuildingMutationBoundaryCounts(t, gameServer, owner.PlayerID, 1, 1, 2, 2, 4)
	upgradeReference, err := foundation.PlanetBuildingUpgradeIdempotencyKey(planetID, buildingID.String(), 2)
	if err != nil {
		t.Fatalf("PlanetBuildingUpgradeIdempotencyKey: %v", err)
	}
	assertPlanetBuildingDurableCommitCounts(t, gameServer, upgradeReference, 1, 2, 1)

	events, err := gameServer.runtime.postCommandEvents(owner.SessionID, realtime.OperationPlanetBuildingUpgrade, owner.PlayerID)
	if err != nil {
		t.Fatalf("post planet.building_upgrade events: %v", err)
	}
	requireEventTypeForTest(t, events, realtime.EventProductionSummary)
	requireEventTypeForTest(t, events, realtime.EventPlanetStorage)
	requireEventTypeForTest(t, events, realtime.EventWalletSnapshot)

	duplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-planet-building-upgrade-duplicate","op":"planet.building_upgrade","payload":{"planet_id":"`+planetID.String()+`","building_id":"`+buildingID.String()+`","next_level":2},"client_seq":2,"v":1}`),
	)
	if duplicate.HasError {
		t.Fatalf("duplicate planet.building_upgrade response error = %+v, want success", duplicate.Error)
	}
	var duplicatePayload struct {
		Wallet    walletSnapshotPayload `json:"wallet"`
		Duplicate bool                  `json:"duplicate"`
	}
	if err := json.Unmarshal(duplicate.Response.Payload, &duplicatePayload); err != nil {
		t.Fatalf("decode duplicate planet.building_upgrade payload: %v", err)
	}
	if !duplicatePayload.Duplicate {
		t.Fatalf("duplicate upgrade duplicate = false, want true")
	}
	if duplicatePayload.Wallet.Credits != starterWalletCredits+500-buildingUpgradeIronExtractorCredits {
		t.Fatalf("duplicate upgrade wallet credits = %d, want unchanged %d", duplicatePayload.Wallet.Credits, starterWalletCredits+500-buildingUpgradeIronExtractorCredits)
	}
	assertStoredRouteStorageQuantity(t, gameServer, planetID, "iron_ore", 20)
	assertStoredPlanetBuildingLevel(t, gameServer, planetID, buildingID, 2)
	assertPlanetBuildingMutationBoundaryCounts(t, gameServer, owner.PlayerID, 1, 1, 2, 2, 4)
	assertPlanetBuildingDurableCommitCounts(t, gameServer, upgradeReference, 1, 2, 1)

	conflict := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-planet-building-upgrade-conflict","op":"planet.building_upgrade","payload":{"planet_id":"`+planetID.String()+`","building_id":"`+buildingID.String()+`","target_level":2,"next_level":3},"client_seq":3,"v":1}`),
	)
	if !conflict.HasError || conflict.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("conflicting target/next level response = %+v, want invalid payload", conflict)
	}
	assertPlanetBuildingMutationBoundaryCounts(t, gameServer, owner.PlayerID, 1, 1, 2, 2, 4)
}

func seedPlanetBuildingWalletCredits(t *testing.T, gameServer *Server, playerID foundation.PlayerID, amount int64, reference string) {
	t.Helper()
	if _, err := gameServer.runtime.Wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       amount,
		Reason:       economy.LedgerReason("test_seed"),
		ReferenceKey: foundation.IdempotencyKey(reference),
	}); err != nil {
		t.Fatalf("CreditWallet(%q) error = %v, want nil", playerID, err)
	}
}

func seedPlanetBuildingForTest(
	t *testing.T,
	gameServer *Server,
	planetID foundation.PlanetID,
	buildingType production.BuildingType,
	slot string,
	definitionID catalog.DefinitionID,
) production.BuildingID {
	t.Helper()
	buildingID, err := deterministicPlanetBuildingID(planetID, buildingType, slot)
	if err != nil {
		t.Fatalf("deterministicPlanetBuildingID: %v", err)
	}
	definition, err := production.MustMVPCatalog().MustGet(definitionID)
	if err != nil {
		t.Fatalf("MustGet(%q) error = %v, want nil", definitionID, err)
	}
	now := gameServer.runtime.clock.Now().UTC()
	building, err := production.NewPlanetBuilding(buildingID, planetID, definition, production.BuildingStateActive, now, now)
	if err != nil {
		t.Fatalf("NewPlanetBuilding(%q) error = %v, want nil", buildingID, err)
	}
	if _, _, err := gameServer.runtime.Production.UpsertBuilding(building); err != nil {
		t.Fatalf("UpsertBuilding(%q) error = %v, want nil", buildingID, err)
	}
	return buildingID
}

func assertStoredPlanetBuildingLevel(t *testing.T, gameServer *Server, planetID foundation.PlanetID, buildingID production.BuildingID, wantLevel int) {
	t.Helper()
	building, ok, err := gameServer.runtime.Production.Building(planetID, buildingID)
	if err != nil || !ok {
		t.Fatalf("Building(%q, %q) ok=%v err=%v, want stored", planetID, buildingID, ok, err)
	}
	if building.Level != wantLevel {
		t.Fatalf("building %q level = %d, want %d", buildingID, building.Level, wantLevel)
	}
}

func assertPlanetBuildingMutationBoundaryCounts(t *testing.T, gameServer *Server, playerID foundation.PlayerID, references int, materialLedger int, productionEvents int, outbox int, walletLedger int) {
	t.Helper()
	if got := len(gameServer.runtime.Production.BuildingMutationReferences()); got != references {
		t.Fatalf("building references = %d, want %d", got, references)
	}
	if got := len(gameServer.runtime.Production.BuildingMaterialLedgerEntries()); got != materialLedger {
		t.Fatalf("building material ledger entries = %d, want %d", got, materialLedger)
	}
	if got := len(gameServer.runtime.Production.Events()); got != productionEvents {
		t.Fatalf("production domain events = %d, want %d", got, productionEvents)
	}
	if got := len(gameServer.runtime.Production.OutboxRecords()); got != outbox {
		t.Fatalf("production outbox records = %d, want %d", got, outbox)
	}
	if got := countPlanetBuildingWalletLedgerEntriesForPlayer(gameServer.runtime.Wallet.CurrencyLedgerEntries(), playerID); got != walletLedger {
		t.Fatalf("wallet ledger entries for %q = %d, want %d", playerID, got, walletLedger)
	}
}

func assertPlanetBuildingDurableCommitCounts(t *testing.T, gameServer *Server, referenceKey foundation.IdempotencyKey, references int, outbox int, materialLedger int) {
	t.Helper()
	if got := len(gameServer.runtime.BuildingMutations.BuildingMutationReferences()); got != references {
		t.Fatalf("durable building references = %d, want %d", got, references)
	}
	if got := len(gameServer.runtime.BuildingMutations.OutboxRecords()); got != outbox {
		t.Fatalf("durable building outbox records = %d, want %d", got, outbox)
	}
	if got := len(gameServer.runtime.BuildingMutations.BuildingMaterialLedgerEntries()); got != materialLedger {
		t.Fatalf("durable building material ledger entries = %d, want %d", got, materialLedger)
	}
	plan, ok, err := gameServer.runtime.BuildingMutations.CommittedBuildingMutationDurableCommitPlan(referenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedBuildingMutationDurableCommitPlan(%q) = ok %v err %v, want true nil", referenceKey, ok, err)
	}
	if plan.Reference.ReferenceKey != referenceKey || len(plan.OutboxRecords) != outbox || len(plan.MaterialLedger) != materialLedger {
		t.Fatalf("durable building plan = %+v, want ref %q outbox %d ledger %d", plan, referenceKey, outbox, materialLedger)
	}
}

func countPlanetBuildingWalletLedgerEntriesForPlayer(entries []economy.CurrencyLedgerEntry, playerID foundation.PlayerID) int {
	count := 0
	for _, entry := range entries {
		if entry.PlayerID == playerID {
			count++
		}
	}
	return count
}

func handlePlanetBuildingBuildForTest(gameServer *Server, sessionID auth.SessionID, planetID foundation.PlanetID, slot string, clientSeq int) realtime.CachedResponse {
	return gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(sessionID.String()),
		[]byte(`{"request_id":"request-planet-building-concurrent-`+slot+`","op":"planet.building_build","payload":{"planet_id":"`+planetID.String()+`","building_type":"alloy_foundry","slot":"`+slot+`"},"client_seq":`+fmt.Sprint(clientSeq)+`,"v":1}`),
	)
}

func waitPlanetBuildingResponseForTest(t *testing.T, label string, done <-chan realtime.CachedResponse) realtime.CachedResponse {
	t.Helper()
	select {
	case response := <-done:
		return response
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not finish", label)
		return realtime.CachedResponse{}
	}
}

type blockingWalletDebitEmitterForTest struct {
	debits  chan struct{}
	release chan struct{}
}

func newBlockingWalletDebitEmitterForTest() *blockingWalletDebitEmitterForTest {
	return &blockingWalletDebitEmitterForTest{
		debits:  make(chan struct{}, 4),
		release: make(chan struct{}),
	}
}

func (emitter *blockingWalletDebitEmitterForTest) Record(event gameevents.EventEnvelope) {
	if event.Type != economy.EventWalletDebited {
		return
	}
	emitter.debits <- struct{}{}
	<-emitter.release
}

func (emitter *blockingWalletDebitEmitterForTest) waitForDebit(t *testing.T, label string) {
	t.Helper()
	select {
	case <-emitter.debits:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not reach wallet debit", label)
	}
}

func storageQuantityFromCollection(payload planetStorageCollectionPayload, planetID foundation.PlanetID, itemID foundation.ItemID) int64 {
	for _, planet := range payload.Planets {
		if planet.PlanetID != planetID.String() {
			continue
		}
		for _, item := range planet.Items {
			if item.ItemID == itemID.String() {
				return item.Quantity
			}
		}
	}
	return 0
}

func assertNoQueuedEventsForSession(t *testing.T, gameServer *Server, sessionID auth.SessionID) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	if queued := len(gameServer.runtime.queuedEvents[sessionID]); queued != 0 {
		t.Fatalf("session %q queued events = %d, want none", sessionID, queued)
	}
}

func assertPayloadOmitsPlanetBuildingInternals(t *testing.T, label string, payload any) {
	t.Helper()
	assertPayloadOmitsInternalMapIdentity(t, label, payload)
	raw := payloadStringForTest(payload)
	for _, forbidden := range []string{
		"ledger",
		"reference_key",
		"reference_id",
		"wallet_debit",
		"material_ledger",
		"domain_event",
		"events",
		"source",
		"definition_id",
		"owner_player_id",
		"player_id",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}

func payloadStringForTest(payload any) string {
	switch typed := payload.(type) {
	case json.RawMessage:
		return string(typed)
	case []byte:
		return string(typed)
	case string:
		return typed
	default:
		data, _ := json.Marshal(typed)
		return string(data)
	}
}
