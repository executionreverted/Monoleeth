package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestProductionSummarySettlesOwnedActiveMapProductionAndQueuesSafeEvents(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "production-summary-owner@example.com", "Production Summary Owner")
	other := createResolvedRuntimeSession(t, gameServer, "production-summary-other@example.com", "Production Summary Other")
	activePlanetID := foundation.PlanetID("planet-production-summary-active")
	otherMapPlanetID := foundation.PlanetID("planet-production-summary-other-map")
	otherOwnerPlanetID := foundation.PlanetID("planet-production-summary-other-owner")

	base := clock.Now().UTC()
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, activePlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-production-summary-active")
	seedActiveProductionBuildingForTest(t, gameServer, activePlanetID, "building-production-summary-active")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, otherMapPlanetID, worldmaps.MapID("map_1_2").ZoneID(), world.Vec2{X: 1700, Y: 5200}, "candidate-production-summary-other-map")
	seedActiveProductionBuildingForTest(t, gameServer, otherMapPlanetID, "building-production-summary-other-map")
	seedOwnedProductionPlanetForTest(t, gameServer, other.PlayerID, otherOwnerPlanetID, gameServer.runtime.zoneID, world.Vec2{X: 1800, Y: 1500}, "candidate-production-summary-other-owner")
	seedActiveProductionBuildingForTest(t, gameServer, otherOwnerPlanetID, "building-production-summary-other-owner")

	settledAt := clock.Advance(2 * time.Hour).UTC()
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-production-summary-settle","op":"planet.production_summary","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("planet.production_summary response error = %+v, want success", response.Error)
	}
	assertSafeProductionRealtimePayload(t, "production summary response", response.Response.Payload, owner.PlayerID)

	var payload struct {
		Production planetProductionCollectionPayload `json:"production"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode production summary payload: %v", err)
	}
	assertProductionSummarySnapshot(t, payload.Production, activePlanetID, "1-1", "iron_ore", 60)
	assertStoredProductionSnapshot(t, gameServer, activePlanetID, "iron_ore", 60, settledAt)
	assertStoredProductionSnapshot(t, gameServer, otherMapPlanetID, "iron_ore", 0, base)
	assertStoredProductionSnapshot(t, gameServer, otherOwnerPlanetID, "iron_ore", 0, base)

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationProductionSummary, owner.PlayerID)
	if err != nil {
		t.Fatalf("post production summary events: %v", err)
	}
	if _, leaked := eventsBySession[other.SessionID]; leaked {
		t.Fatalf("production summary events leaked to non-owner session: %+v", eventsBySession[other.SessionID])
	}
	assertProductionSettlementEvents(t, eventsBySession[owner.SessionID], activePlanetID, owner.PlayerID, 60)
	eventCountAfterFirst := len(gameServer.runtime.Production.Events())

	duplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-production-summary-duplicate","op":"planet.production_summary","payload":{},"client_seq":2,"v":1}`),
	)
	if duplicate.HasError {
		t.Fatalf("duplicate production summary response error = %+v, want success", duplicate.Error)
	}
	assertStoredProductionSnapshot(t, gameServer, activePlanetID, "iron_ore", 60, settledAt)
	if got := len(gameServer.runtime.Production.Events()); got != eventCountAfterFirst {
		t.Fatalf("duplicate production summary domain events = %d, want %d", got, eventCountAfterFirst)
	}
	duplicateEvents, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationProductionSummary, owner.PlayerID)
	if err != nil {
		t.Fatalf("post duplicate production summary events: %v", err)
	}
	if got := countEventType(duplicateEvents[owner.SessionID], realtime.EventProductionSummary) + countEventType(duplicateEvents[owner.SessionID], realtime.EventPlanetStorage); got != 0 {
		t.Fatalf("duplicate production summary queued production/storage events = %d in %+v, want none", got, duplicateEvents[owner.SessionID])
	}

	nearDuplicateAt := clock.Advance(time.Second).UTC()
	nearDuplicate := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-production-summary-near-duplicate","op":"planet.storage_summary","payload":{},"client_seq":3,"v":1}`),
	)
	if nearDuplicate.HasError {
		t.Fatalf("near-duplicate storage summary response error = %+v, want success", nearDuplicate.Error)
	}
	assertStoredProductionSnapshot(t, gameServer, activePlanetID, "iron_ore", 60, settledAt)
	if got := len(gameServer.runtime.Production.Events()); got != eventCountAfterFirst {
		t.Fatalf("near-duplicate production domain events at %s = %d, want %d", nearDuplicateAt, got, eventCountAfterFirst)
	}
	nearDuplicateEvents, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationPlanetStorage, owner.PlayerID)
	if err != nil {
		t.Fatalf("post near-duplicate storage summary events: %v", err)
	}
	if got := countEventType(nearDuplicateEvents[owner.SessionID], realtime.EventProductionSummary) + countEventType(nearDuplicateEvents[owner.SessionID], realtime.EventPlanetStorage); got != 0 {
		t.Fatalf("near-duplicate storage summary queued production/storage events = %d in %+v, want none", got, nearDuplicateEvents[owner.SessionID])
	}
}

func TestProductionSummarySkipsSubUnitPollWithoutLosingFractionalProgress(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "production-summary-fractional@example.com", "Production Summary Fractional")
	planetID := foundation.PlanetID("planet-production-summary-fractional")

	base := clock.Now().UTC()
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, planetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-production-summary-fractional")
	seedActiveProductionBuildingForTest(t, gameServer, planetID, "building-production-summary-fractional")

	oneMinuteAt := clock.Advance(time.Minute).UTC()
	summary := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-production-summary-fractional-1","op":"planet.production_summary","payload":{},"client_seq":1,"v":1}`),
	)
	if summary.HasError {
		t.Fatalf("sub-unit planet.production_summary response error = %+v, want success", summary.Error)
	}
	var summaryPayload struct {
		Production planetProductionCollectionPayload `json:"production"`
	}
	if err := json.Unmarshal(summary.Response.Payload, &summaryPayload); err != nil {
		t.Fatalf("decode sub-unit production summary payload: %v", err)
	}
	assertProductionSummarySnapshot(t, summaryPayload.Production, planetID, "1-1", "iron_ore", 0)
	assertStoredProductionSnapshot(t, gameServer, planetID, "iron_ore", 0, base)
	assertNoProductionSummarySettlementEvents(t, gameServer, owner.SessionID, "one-minute production summary")
	if got := len(gameServer.runtime.Production.Events()); got != 0 {
		t.Fatalf("one-minute production domain events at %s = %d, want none", oneMinuteAt, got)
	}

	storageAt := clock.Advance(30 * time.Second).UTC()
	storage := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-production-summary-fractional-2","op":"planet.storage_summary","payload":{},"client_seq":2,"v":1}`),
	)
	if storage.HasError {
		t.Fatalf("sub-unit planet.storage_summary response error = %+v, want success", storage.Error)
	}
	var storagePayload struct {
		Storage planetStorageCollectionPayload `json:"planet_storage"`
	}
	if err := json.Unmarshal(storage.Response.Payload, &storagePayload); err != nil {
		t.Fatalf("decode sub-unit storage summary payload: %v", err)
	}
	assertStorageSummarySnapshot(t, storagePayload.Storage, planetID, "1-1", "iron_ore", 0)
	assertStoredProductionSnapshot(t, gameServer, planetID, "iron_ore", 0, base)
	assertNoProductionSummarySettlementEvents(t, gameServer, owner.SessionID, "ninety-second storage summary")
	if got := len(gameServer.runtime.Production.Events()); got != 0 {
		t.Fatalf("ninety-second production domain events at %s = %d, want none", storageAt, got)
	}

	settledAt := clock.Advance(30 * time.Second).UTC()
	finalSummary := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-production-summary-fractional-3","op":"planet.production_summary","payload":{},"client_seq":3,"v":1}`),
	)
	if finalSummary.HasError {
		t.Fatalf("two-minute planet.production_summary response error = %+v, want success", finalSummary.Error)
	}
	var finalPayload struct {
		Production planetProductionCollectionPayload `json:"production"`
	}
	if err := json.Unmarshal(finalSummary.Response.Payload, &finalPayload); err != nil {
		t.Fatalf("decode two-minute production summary payload: %v", err)
	}
	assertProductionSummarySnapshot(t, finalPayload.Production, planetID, "1-1", "iron_ore", 1)
	assertStoredProductionSnapshot(t, gameServer, planetID, "iron_ore", 1, settledAt)
	if got := len(gameServer.runtime.Production.Events()); got != 3 {
		t.Fatalf("two-minute production domain events = %d, want one settlement worth of 3", got)
	}

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationProductionSummary, owner.PlayerID)
	if err != nil {
		t.Fatalf("post two-minute production summary events: %v", err)
	}
	assertProductionSettlementEvents(t, eventsBySession[owner.SessionID], planetID, owner.PlayerID, 1)
}

func TestPlanetStorageSummarySettlesProductionWithRequestScopedNow(t *testing.T) {
	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	setupClock := testutil.NewFakeClock(base)
	gameServer := newRouteControlTestServer(t, setupClock)
	owner := createResolvedRuntimeSession(t, gameServer, "storage-summary-clock@example.com", "Storage Summary Clock")
	planetOneID := foundation.PlanetID("planet-storage-summary-clock-a")
	planetTwoID := foundation.PlanetID("planet-storage-summary-clock-b")

	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, planetOneID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-storage-summary-clock-a")
	seedActiveProductionBuildingForTest(t, gameServer, planetOneID, "building-storage-summary-clock-a")
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, planetTwoID, gameServer.runtime.zoneID, world.Vec2{X: 1500, Y: 1500}, "candidate-storage-summary-clock-b")
	seedActiveProductionBuildingForTest(t, gameServer, planetTwoID, "building-storage-summary-clock-b")

	requestNow := base.Add(time.Hour)
	gameServer.runtime.clock = &steppingRouteSettleClock{next: requestNow, step: 5 * time.Second}
	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-storage-summary-settle","op":"planet.storage_summary","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("planet.storage_summary response error = %+v, want success", response.Error)
	}
	assertSafeProductionRealtimePayload(t, "storage summary response", response.Response.Payload, owner.PlayerID)

	var payload struct {
		Storage planetStorageCollectionPayload `json:"planet_storage"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode storage summary payload: %v", err)
	}
	assertStorageSummarySnapshot(t, payload.Storage, planetOneID, "1-1", "iron_ore", 30)
	assertStorageSummarySnapshot(t, payload.Storage, planetTwoID, "1-1", "iron_ore", 30)
	assertStoredProductionSnapshot(t, gameServer, planetOneID, "iron_ore", 30, requestNow)
	assertStoredProductionSnapshot(t, gameServer, planetTwoID, "iron_ore", 30, requestNow)
}

func TestProductionSummaryRejectsSpoofedServerOwnedFieldsBeforeSettlement(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "production-summary-spoof@example.com", "Production Summary Spoof")
	planetID := foundation.PlanetID("planet-production-summary-spoof")

	base := clock.Now().UTC()
	seedOwnedProductionPlanetForTest(t, gameServer, owner.PlayerID, planetID, gameServer.runtime.zoneID, world.Vec2{X: 1300, Y: 1400}, "candidate-production-summary-spoof")
	seedActiveProductionBuildingForTest(t, gameServer, planetID, "building-production-summary-spoof")
	clock.Advance(time.Hour)

	tests := []struct {
		name  string
		op    string
		field string
	}{
		{name: "production owner", op: "planet.production_summary", field: `"owner_player_id":"spoofed-player"`},
		{name: "production map", op: "planet.production_summary", field: `"map_id":"map_1_1"`},
		{name: "production time", op: "planet.production_summary", field: `"last_calculated_at":123`},
		{name: "production output", op: "planet.production_summary", field: `"output":{"item_id":"iron_ore","quantity":999}`},
		{name: "production building", op: "planet.production_summary", field: `"building_id":"building-spoofed"`},
		{name: "storage owner", op: "planet.storage_summary", field: `"owner_player_id":"spoofed-player"`},
		{name: "storage map", op: "planet.storage_summary", field: `"internal_map_id":"map_1_1"`},
		{name: "storage contents", op: "planet.storage_summary", field: `"storage":{"items":[{"item_id":"iron_ore","quantity":999}]}`},
		{name: "storage elapsed", op: "planet.storage_summary", field: `"elapsed_ms":3600000`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(owner.SessionID.String()),
				[]byte(`{"request_id":"request-production-summary-spoof-`+strings.ReplaceAll(tt.name, " ", "-")+`","op":"`+tt.op+`","payload":{"planet_id":"`+planetID.String()+`",`+tt.field+`},"client_seq":1,"v":1}`),
			)
			if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
				t.Fatalf("%s response = %+v, want invalid payload", tt.op, response)
			}
			assertStoredProductionSnapshot(t, gameServer, planetID, "iron_ore", 0, base)
			if events := gameServer.runtime.Production.Events(); len(events) != 0 {
				t.Fatalf("spoofed %s production domain events = %+v, want none", tt.name, events)
			}
			gameServer.runtime.mu.Lock()
			queuedEvents := len(gameServer.runtime.queuedEvents[owner.SessionID])
			gameServer.runtime.mu.Unlock()
			if queuedEvents != 0 {
				t.Fatalf("spoofed %s queued events = %d, want none", tt.name, queuedEvents)
			}
		})
	}
}

func assertNoProductionSummarySettlementEvents(t *testing.T, gameServer *Server, sessionID auth.SessionID, label string) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	events := gameServer.runtime.queuedEvents[sessionID]
	if got := countEventType(events, realtime.EventProductionSummary) + countEventType(events, realtime.EventPlanetStorage); got != 0 {
		t.Fatalf("%s queued production/storage events = %d in %+v, want none", label, got, events)
	}
}

func seedActiveProductionBuildingForTest(t *testing.T, gameServer *Server, planetID foundation.PlanetID, buildingID production.BuildingID) {
	t.Helper()
	definition, err := production.MustMVPCatalog().MustGet(production.ProductionDefinitionIDIronExtractorL1)
	if err != nil {
		t.Fatalf("production catalog iron extractor: %v", err)
	}
	now := gameServer.runtime.clock.Now().UTC()
	building, err := production.NewPlanetBuilding(buildingID, planetID, definition, production.BuildingStateActive, now, now)
	if err != nil {
		t.Fatalf("NewPlanetBuilding(%q) error = %v, want nil", buildingID, err)
	}
	if _, _, err := gameServer.runtime.Production.UpsertBuilding(building); err != nil {
		t.Fatalf("UpsertBuilding(%q) error = %v, want nil", buildingID, err)
	}
}

func assertProductionSummarySnapshot(
	t *testing.T,
	payload planetProductionCollectionPayload,
	planetID foundation.PlanetID,
	publicMapKey string,
	itemID foundation.ItemID,
	wantQuantity int64,
) {
	t.Helper()
	if len(payload.Planets) != 1 {
		t.Fatalf("production planets = %+v, want one active-map planet %q", payload.Planets, planetID)
	}
	planet := payload.Planets[0]
	if planet.PlanetID != planetID.String() || planet.PublicMapKey != publicMapKey || planet.Storage.PublicMapKey != publicMapKey {
		t.Fatalf("production planet = %+v, want planet %q public map %q", planet, planetID, publicMapKey)
	}
	if got := productionStorageQuantity(planet.Storage, itemID.String()); got != wantQuantity {
		t.Fatalf("production storage %q quantity = %d, want %d in %+v", itemID, got, wantQuantity, planet.Storage)
	}
}

func assertStorageSummarySnapshot(
	t *testing.T,
	payload planetStorageCollectionPayload,
	planetID foundation.PlanetID,
	publicMapKey string,
	itemID foundation.ItemID,
	wantQuantity int64,
) {
	t.Helper()
	for _, planet := range payload.Planets {
		if planet.PlanetID != planetID.String() {
			continue
		}
		if planet.PublicMapKey != publicMapKey {
			t.Fatalf("storage planet %q public map = %q, want %q", planetID, planet.PublicMapKey, publicMapKey)
		}
		if got := productionStorageQuantity(planet, itemID.String()); got != wantQuantity {
			t.Fatalf("storage %q quantity = %d, want %d in %+v", itemID, got, wantQuantity, planet)
		}
		return
	}
	t.Fatalf("storage planets = %+v, missing planet %q", payload.Planets, planetID)
}

func assertStoredProductionSnapshot(
	t *testing.T,
	gameServer *Server,
	planetID foundation.PlanetID,
	itemID foundation.ItemID,
	wantQuantity int64,
	wantLastCalculatedAt time.Time,
) {
	t.Helper()
	snapshot, ok, err := gameServer.runtime.Production.Snapshot(planetID)
	if err != nil || !ok {
		t.Fatalf("Production.Snapshot(%q) ok=%v err=%v, want stored snapshot", planetID, ok, err)
	}
	if got := snapshot.Storage.QuantityOf(itemID); got != wantQuantity {
		t.Fatalf("stored planet %q %q quantity = %d, want %d", planetID, itemID, got, wantQuantity)
	}
	if !snapshot.State.LastCalculatedAt.Equal(wantLastCalculatedAt.UTC()) {
		t.Fatalf("stored planet %q last_calculated_at = %s, want %s", planetID, snapshot.State.LastCalculatedAt, wantLastCalculatedAt.UTC())
	}
}

func assertProductionSettlementEvents(
	t *testing.T,
	events []realtime.EventEnvelope,
	planetID foundation.PlanetID,
	ownerID foundation.PlayerID,
	wantQuantity int64,
) {
	t.Helper()
	if countEventType(events, realtime.EventProductionSummary) != 1 || countEventType(events, realtime.EventPlanetStorage) != 1 {
		t.Fatalf("production settlement events = %+v, want one production and one storage event", events)
	}
	for _, event := range events {
		switch event.Type {
		case realtime.EventAOIEntityEntered, realtime.EventAOIEntityUpdated, realtime.EventAOIEntityLeft:
			t.Fatalf("production summary emitted AOI diff event: %+v", event)
		}
		assertSafeProductionRealtimePayload(t, string(event.Type)+" event", event.Payload, ownerID)
	}

	productionEvent := requireEventTypeForTest(t, events, realtime.EventProductionSummary)
	var productionPayload planetProductionCollectionPayload
	if err := json.Unmarshal(productionEvent.Payload, &productionPayload); err != nil {
		t.Fatalf("decode production summary event: %v", err)
	}
	assertProductionSummarySnapshot(t, productionPayload, planetID, "1-1", "iron_ore", wantQuantity)

	storageEvent := requireEventTypeForTest(t, events, realtime.EventPlanetStorage)
	var storagePayload planetStorageCollectionPayload
	if err := json.Unmarshal(storageEvent.Payload, &storagePayload); err != nil {
		t.Fatalf("decode storage summary event: %v", err)
	}
	assertStorageSummarySnapshot(t, storagePayload, planetID, "1-1", "iron_ore", wantQuantity)
}

func assertSafeProductionRealtimePayload(t *testing.T, label string, payload any, ownerID foundation.PlayerID) {
	t.Helper()
	assertPayloadOmitsInternalMapIdentity(t, label, payload)
	assertPayloadOmitsPlayerOwner(t, label, payload, ownerID)
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s: %v", label, err)
	}
	raw := string(data)
	for _, forbidden := range []string{
		`"player_id"`,
		`"world_id"`,
		`"zone_id"`,
		`"map_id"`,
		`"internal_map_id"`,
		`"candidate_key"`,
		`"procedural_seed"`,
		`"seed"`,
		"map_1_",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}

func productionStorageQuantity(storage planetStoragePayload, itemID string) int64 {
	for _, item := range storage.Items {
		if item.ItemID == itemID {
			return item.Quantity
		}
	}
	return 0
}
