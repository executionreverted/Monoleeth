package production

import (
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestInMemoryStoreBoundaryReadMethodsReturnDetachedRecords(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)

	result, err := store.SettlePlanetProduction("planet-1", testTime(0).Add(time.Hour))
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}

	references := store.SettlementReferences()
	if len(references) != 1 {
		t.Fatalf("SettlementReferences() len = %d, want 1", len(references))
	}
	references[0].SettlementWindow = "mutated"
	references[0].Kind = SettlementKindRoute

	storedReference, ok, err := store.SettlementReference(result.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("SettlementReference() ok = %v err = %v, want true nil", ok, err)
	}
	if storedReference.SettlementWindow != result.SettlementWindow || storedReference.Kind != SettlementKindProduction {
		t.Fatalf("stored reference = %+v, want original production reference/window", storedReference)
	}

	outbox := store.OutboxRecords()
	if len(outbox) == 0 {
		t.Fatal("OutboxRecords() len = 0, want records")
	}
	if len(outbox[0].Event.Payload) == 0 {
		t.Fatal("OutboxRecords()[0].Event.Payload empty, want JSON payload")
	}
	originalType := outbox[0].Event.Type
	originalPayloadFirstByte := outbox[0].Event.Payload[0]
	outbox[0].Status = ProductionOutboxStatus("sent")
	outbox[0].Event.Type = "mutated"
	outbox[0].Event.Payload[0] = 'x'

	storedOutbox := store.OutboxRecords()
	if storedOutbox[0].Status != ProductionOutboxStatusPending {
		t.Fatalf("stored outbox status = %q, want pending", storedOutbox[0].Status)
	}
	if storedOutbox[0].Event.Type != originalType {
		t.Fatalf("stored outbox event type = %q, want %q", storedOutbox[0].Event.Type, originalType)
	}
	if storedOutbox[0].Event.Payload[0] != originalPayloadFirstByte {
		t.Fatalf("stored outbox payload first byte = %q, want original %q", storedOutbox[0].Event.Payload[0], originalPayloadFirstByte)
	}
}

func TestSettlePlanetProductionRecordedReferenceReuseNoOpsBeforeMutation(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	now := testTime(0).Add(time.Hour)
	window := wantSettlementWindow(testTime(0), now)
	reference := mustOfflineSettlementKey(t, "planet-1", window)

	store.mu.Lock()
	store.ensureMapsLocked()
	store.recordSettlementReferenceLocked(SettlementReferenceRecord{
		ReferenceKey:     reference,
		SettlementWindow: window,
		Kind:             SettlementKindProduction,
		PlanetID:         "planet-1",
		AppliedAt:        now,
		RecordedAt:       now,
	})
	store.mu.Unlock()

	result, err := store.SettlePlanetProduction("planet-1", now)
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	if !result.NoOp {
		t.Fatal("NoOp = false, want true for recorded reference reuse")
	}
	if got := result.After.Storage.QuantityOf("iron_ore"); got != 0 {
		t.Fatalf("after iron_ore = %d, want 0", got)
	}
	if got := len(store.Events()); got != 0 {
		t.Fatalf("events after reference reuse = %d, want 0", got)
	}
	if got := len(store.OutboxRecords()); got != 0 {
		t.Fatalf("outbox after reference reuse = %d, want 0", got)
	}
	if got := len(store.SettlementReferences()); got != 1 {
		t.Fatalf("references after reference reuse = %d, want 1", got)
	}
}

func TestSettleRouteRecordedReferenceReuseNoOpsBeforeMutation(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	window := wantSettlementWindow(last, now)
	reference := mustRouteSettlementKey(t, route.RouteID, window)

	store.mu.Lock()
	store.ensureMapsLocked()
	store.recordSettlementReferenceLocked(SettlementReferenceRecord{
		ReferenceKey:     reference,
		SettlementWindow: window,
		Kind:             SettlementKindRoute,
		RouteID:          route.RouteID,
		AppliedAt:        now,
		RecordedAt:       now,
	})
	store.mu.Unlock()

	service := newTestRouteSettlementService(t, store, now, nil)
	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if !result.NoOp {
		t.Fatal("NoOp = false, want true for recorded reference reuse")
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
	if got := len(store.Events()); got != 0 {
		t.Fatalf("events after reference reuse = %d, want 0", got)
	}
	if got := len(store.OutboxRecords()); got != 0 {
		t.Fatalf("outbox after reference reuse = %d, want 0", got)
	}
	if got := len(store.SettlementReferences()); got != 1 {
		t.Fatalf("references after reference reuse = %d, want 1", got)
	}
}

func TestOutboxPendingRecordsFilterByStatusLimitAndAppendOrder(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	all := store.OutboxRecords()
	if len(all) < 4 {
		t.Fatalf("OutboxRecords() len = %d, want at least 4", len(all))
	}

	pending := store.PendingOutboxRecords(2)
	assertOutboxSequences(t, pending, 1, 2)

	claimed := store.ClaimPendingOutboxRecords(1, testTime(10))
	assertOutboxSequences(t, claimed, 1)
	pending = store.PendingOutboxRecords(10)
	assertOutboxSequences(t, pending, 2, 3, 4)

	if got := store.PendingOutboxRecords(0); got != nil {
		t.Fatalf("PendingOutboxRecords(0) = %+v, want nil", got)
	}
}

func TestOutboxClaimMovesPendingToInFlightWithDetachedRecords(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	claimedAt := testTime(11)

	claimed := store.ClaimPendingOutboxRecords(2, claimedAt)
	if len(claimed) != 2 {
		t.Fatalf("ClaimPendingOutboxRecords() len = %d, want 2", len(claimed))
	}
	for index, record := range claimed {
		if record.Status != ProductionOutboxStatusInFlight {
			t.Fatalf("claimed[%d] status = %q, want in_flight", index, record.Status)
		}
		if !record.ClaimedAt.Equal(claimedAt) {
			t.Fatalf("claimed[%d] ClaimedAt = %s, want %s", index, record.ClaimedAt, claimedAt)
		}
		if record.Attempts != 1 {
			t.Fatalf("claimed[%d] Attempts = %d, want 1", index, record.Attempts)
		}
		wantClaimToken := productionOutboxClaimToken(record.OutboxID, record.Attempts)
		if record.ClaimToken != wantClaimToken {
			t.Fatalf("claimed[%d] ClaimToken = %q, want %q", index, record.ClaimToken, wantClaimToken)
		}
	}
	if len(claimed[0].Event.Payload) == 0 {
		t.Fatal("claimed[0] payload empty, want payload")
	}
	originalPayloadFirstByte := claimed[0].Event.Payload[0]
	claimed[0].Status = ProductionOutboxStatusPublished
	claimed[0].Event.Payload[0] = 'x'

	stored := store.OutboxRecords()
	if stored[0].Status != ProductionOutboxStatusInFlight {
		t.Fatalf("stored[0] status = %q, want in_flight", stored[0].Status)
	}
	if stored[0].Event.Payload[0] != originalPayloadFirstByte {
		t.Fatalf("stored[0] payload first byte = %q, want original %q", stored[0].Event.Payload[0], originalPayloadFirstByte)
	}
}

func TestOutboxPublishedRecordsAreNotPendingOrRetryable(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	claimed := store.ClaimPendingOutboxRecords(2, testTime(12))
	publishedAt := testTime(13)

	published, ok := store.MarkClaimedOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, publishedAt)
	if !ok {
		t.Fatal("MarkClaimedOutboxPublished() ok = false, want true")
	}
	if published.Status != ProductionOutboxStatusPublished {
		t.Fatalf("published status = %q, want published", published.Status)
	}
	if !published.PublishedAt.Equal(publishedAt) {
		t.Fatalf("PublishedAt = %s, want %s", published.PublishedAt, publishedAt)
	}

	beforeLateFailure := store.OutboxRecords()
	if record, ok := store.MarkClaimedOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "late failure", testTime(14)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxFailed(published) = %+v/%v, want zero/false", record, ok)
	}
	assertOutboxUnchanged(t, store, beforeLateFailure, "late failure after publish")
	retryable := store.RetryFailedOutboxRecords(10, testTime(15))
	if len(retryable) != 0 {
		t.Fatalf("RetryFailedOutboxRecords() len = %d, want 0 for published record", len(retryable))
	}
	pending := store.PendingOutboxRecords(10)
	for _, record := range pending {
		if record.OutboxID == published.OutboxID {
			t.Fatalf("published record %q appeared in pending records: %+v", published.OutboxID, pending)
		}
	}
	stored := store.OutboxRecords()[0]
	if stored.Status != ProductionOutboxStatusPublished || !stored.PublishedAt.Equal(publishedAt) {
		t.Fatalf("stored published record = %+v, want published with original PublishedAt %s", stored, publishedAt)
	}
}

func TestOutboxFailedRecordsRetryBackToPendingWithoutPayloadAliases(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	claimed := store.ClaimPendingOutboxRecords(1, testTime(16))
	failedAt := testTime(17)

	failed, ok := store.MarkClaimedOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "temporary broker outage", failedAt)
	if !ok {
		t.Fatal("MarkClaimedOutboxFailed() ok = false, want true")
	}
	if failed.Status != ProductionOutboxStatusFailed {
		t.Fatalf("failed status = %q, want failed", failed.Status)
	}
	if failed.LastError != "temporary broker outage" || !failed.FailedAt.Equal(failedAt) {
		t.Fatalf("failure evidence = %q/%s, want reason/%s", failed.LastError, failed.FailedAt, failedAt)
	}
	failed.Event.Payload[0] = 'x'

	storedFailed := store.OutboxRecords()[0]
	if storedFailed.Event.Payload[0] == 'x' {
		t.Fatal("mutating failed returned payload changed stored payload")
	}

	retriedAt := testTime(18)
	retried := store.RetryFailedOutboxRecords(1, retriedAt)
	if len(retried) != 1 {
		t.Fatalf("RetryFailedOutboxRecords() len = %d, want 1", len(retried))
	}
	if retried[0].Status != ProductionOutboxStatusPending {
		t.Fatalf("retried status = %q, want pending", retried[0].Status)
	}
	if retried[0].ClaimToken != "" {
		t.Fatalf("retried ClaimToken = %q, want empty", retried[0].ClaimToken)
	}
	if !retried[0].FailedAt.Equal(failedAt) || retried[0].LastError != "temporary broker outage" {
		t.Fatalf("retried failure evidence = %q/%s, want preserved %q/%s", retried[0].LastError, retried[0].FailedAt, failed.LastError, failedAt)
	}
	if !retried[0].RetriedAt.Equal(retriedAt) {
		t.Fatalf("RetriedAt = %s, want %s", retried[0].RetriedAt, retriedAt)
	}
	retried[0].Event.Payload[0] = 'y'
	if store.OutboxRecords()[0].Event.Payload[0] == 'y' {
		t.Fatal("mutating retried returned payload changed stored payload")
	}

	reclaimed := store.ClaimPendingOutboxRecords(1, testTime(19))
	if len(reclaimed) != 1 {
		t.Fatalf("reclaim len = %d, want 1", len(reclaimed))
	}
	if reclaimed[0].Attempts != 2 {
		t.Fatalf("reclaimed Attempts = %d, want 2", reclaimed[0].Attempts)
	}
	wantClaimToken := productionOutboxClaimToken(reclaimed[0].OutboxID, reclaimed[0].Attempts)
	if reclaimed[0].ClaimToken != wantClaimToken {
		t.Fatalf("reclaimed ClaimToken = %q, want %q", reclaimed[0].ClaimToken, wantClaimToken)
	}
}

func TestOutboxWrongAndStaleClaimTokensDoNotMutateRecords(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	claimed := store.ClaimPendingOutboxRecords(1, testTime(20))
	first := claimed[0]

	beforeWrongPublish := store.OutboxRecords()
	if record, ok := store.MarkClaimedOutboxPublished(first.OutboxID, "wrong-token", testTime(21)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxPublished(wrong token) = %+v/%v, want zero/false", record, ok)
	}
	assertOutboxUnchanged(t, store, beforeWrongPublish, "wrong publish token")

	beforeWrongFail := store.OutboxRecords()
	if record, ok := store.MarkClaimedOutboxFailed(first.OutboxID, "wrong-token", "wrong token failure", testTime(22)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxFailed(wrong token) = %+v/%v, want zero/false", record, ok)
	}
	assertOutboxUnchanged(t, store, beforeWrongFail, "wrong fail token")

	if _, ok := store.MarkClaimedOutboxFailed(first.OutboxID, first.ClaimToken, "temporary broker outage", testTime(23)); !ok {
		t.Fatal("MarkClaimedOutboxFailed(current token) ok = false, want true")
	}
	retried := store.RetryFailedOutboxRecords(1, testTime(24))
	if len(retried) != 1 {
		t.Fatalf("RetryFailedOutboxRecords() len = %d, want 1", len(retried))
	}
	if retried[0].ClaimToken != "" {
		t.Fatalf("retried ClaimToken = %q, want empty", retried[0].ClaimToken)
	}

	beforePendingStale := store.OutboxRecords()
	if record, ok := store.MarkClaimedOutboxPublished(first.OutboxID, first.ClaimToken, testTime(25)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxPublished(stale pending token) = %+v/%v, want zero/false", record, ok)
	}
	if record, ok := store.MarkClaimedOutboxFailed(first.OutboxID, first.ClaimToken, "stale pending failure", testTime(26)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxFailed(stale pending token) = %+v/%v, want zero/false", record, ok)
	}
	assertOutboxUnchanged(t, store, beforePendingStale, "stale token while pending")

	reclaimed := store.ClaimPendingOutboxRecords(1, testTime(27))
	if len(reclaimed) != 1 {
		t.Fatalf("reclaim len = %d, want 1", len(reclaimed))
	}
	second := reclaimed[0]
	if second.ClaimToken == first.ClaimToken {
		t.Fatalf("reclaimed ClaimToken = first token %q, want new attempt token", second.ClaimToken)
	}

	beforeReclaimedStale := store.OutboxRecords()
	if record, ok := store.MarkClaimedOutboxPublished(second.OutboxID, first.ClaimToken, testTime(28)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxPublished(stale reclaimed token) = %+v/%v, want zero/false", record, ok)
	}
	if record, ok := store.MarkClaimedOutboxFailed(second.OutboxID, first.ClaimToken, "stale reclaimed failure", testTime(29)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxFailed(stale reclaimed token) = %+v/%v, want zero/false", record, ok)
	}
	assertOutboxUnchanged(t, store, beforeReclaimedStale, "stale token after reclaim")

	published, ok := store.MarkClaimedOutboxPublished(second.OutboxID, second.ClaimToken, testTime(30))
	if !ok {
		t.Fatal("MarkClaimedOutboxPublished(new token) ok = false, want true")
	}
	if published.Status != ProductionOutboxStatusPublished {
		t.Fatalf("published status = %q, want published", published.Status)
	}
}

func TestOutboxUnknownClaimedMarkReturnsFalseWithoutMutation(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	before := store.OutboxRecords()

	if record, ok := store.MarkClaimedOutboxPublished("missing-outbox", "missing-token", testTime(31)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxPublished(missing) = %+v/%v, want zero/false", record, ok)
	}
	if record, ok := store.MarkClaimedOutboxFailed("missing-outbox", "missing-token", "missing", testTime(32)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxFailed(missing) = %+v/%v, want zero/false", record, ok)
	}
	after := store.OutboxRecords()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("outbox mutated after unknown mark\nbefore=%+v\nafter=%+v", before, after)
	}
}

func assertOutboxUnchanged(t *testing.T, store *InMemoryStore, before []ProductionOutboxRecord, context string) {
	t.Helper()
	after := store.OutboxRecords()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("outbox mutated after %s\nbefore=%+v\nafter=%+v", context, before, after)
	}
}

func assertSettlementReferenceRecord(
	t *testing.T,
	records []SettlementReferenceRecord,
	kind SettlementKind,
	planetID foundation.PlanetID,
	routeID foundation.RouteID,
	reference foundation.IdempotencyKey,
	window string,
	appliedAt time.Time,
) {
	t.Helper()
	if len(records) != 1 {
		t.Fatalf("SettlementReferences() len = %d, want 1; records = %+v", len(records), records)
	}
	record := records[0]
	if record.ReferenceKey != reference || record.SettlementWindow != window || record.Kind != kind {
		t.Fatalf("reference record key/window/kind = %q/%q/%q, want %q/%q/%q", record.ReferenceKey, record.SettlementWindow, record.Kind, reference, window, kind)
	}
	if record.PlanetID != planetID || record.RouteID != routeID {
		t.Fatalf("reference record planet/route = %q/%q, want %q/%q", record.PlanetID, record.RouteID, planetID, routeID)
	}
	if !record.AppliedAt.Equal(appliedAt) || !record.RecordedAt.Equal(appliedAt) {
		t.Fatalf("reference record applied/recorded = %s/%s, want %s", record.AppliedAt, record.RecordedAt, appliedAt)
	}
}

func assertOutboxEventTypes(t *testing.T, records []ProductionOutboxRecord, want ...string) {
	t.Helper()
	if len(records) != len(want) {
		t.Fatalf("OutboxRecords() len = %d, want %d; records = %+v", len(records), len(want), records)
	}
	for index, record := range records {
		if record.Event.Type != want[index] {
			t.Fatalf("outbox[%d] type = %q, want %q; records = %+v", index, record.Event.Type, want[index], records)
		}
		if record.Sequence != uint64(index+1) {
			t.Fatalf("outbox[%d] sequence = %d, want %d", index, record.Sequence, index+1)
		}
		if record.OutboxID == "" {
			t.Fatalf("outbox[%d] id is empty", index)
		}
		if record.Status != ProductionOutboxStatusPending {
			t.Fatalf("outbox[%d] status = %q, want pending", index, record.Status)
		}
		if record.CreatedAt.IsZero() {
			t.Fatalf("outbox[%d] CreatedAt is zero", index)
		}
		if len(record.Event.Payload) == 0 {
			t.Fatalf("outbox[%d] event payload is empty", index)
		}
	}
}

func assertOutboxRecordEvidence(
	t *testing.T,
	records []ProductionOutboxRecord,
	eventType string,
	reference foundation.IdempotencyKey,
	window string,
) {
	t.Helper()
	for _, record := range records {
		if record.Event.Type != eventType {
			continue
		}
		if record.ReferenceKey != reference || record.SettlementWindow != window {
			t.Fatalf("outbox %q evidence = %q/%q, want %q/%q", eventType, record.ReferenceKey, record.SettlementWindow, reference, window)
		}
		return
	}
	t.Fatalf("outbox event %q missing in %+v", eventType, records)
}

func newOutboxStateMachineStore(t *testing.T) *InMemoryStore {
	t.Helper()
	store := newSettlementStore(t, "planet-1", testTime(0), 200, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	addSettlementBuilding(t, store, "planet-1", "building-2", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	if _, err := store.SettlePlanetProduction("planet-1", testTime(0).Add(time.Hour)); err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	return store
}

func assertOutboxSequences(t *testing.T, records []ProductionOutboxRecord, want ...uint64) {
	t.Helper()
	if len(records) != len(want) {
		t.Fatalf("outbox records len = %d, want %d; records = %+v", len(records), len(want), records)
	}
	for index, record := range records {
		if record.Sequence != want[index] {
			t.Fatalf("record[%d] sequence = %d, want %d; records = %+v", index, record.Sequence, want[index], records)
		}
	}
}
