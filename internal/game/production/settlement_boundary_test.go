package production

import (
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
