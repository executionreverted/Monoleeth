package production

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestSettleRouteMissingRouteReturnsClearError(t *testing.T) {
	store := NewInMemoryStore()
	service := newTestRouteSettlementService(t, store, testRouteNow(), nil)

	_, err := service.SettleRoute("route-missing")
	if !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("SettleRoute() error = %v, want ErrRouteNotFound", err)
	}
}

func TestSettleRouteForOwnerRejectsWrongOwnerWithoutMutation(t *testing.T) {
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
	service := newTestRouteSettlementService(t, store, now, nil)

	_, err := service.SettleRouteForOwner("player-2", route.RouteID)
	if !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("SettleRouteForOwner() error = %v, want ErrRouteOwnerMismatch", err)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteEnabled(t, store, route.RouteID, true)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
	assertNoRouteEvents(t, store)
}

func TestSettleRouteEmptySourceTransfersZeroAndUpdatesTimestamps(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(t, route, 100, nil, 100, nil)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if result.NoOp {
		t.Fatal("NoOp = true, want false because route timestamp advances")
	}
	if result.WantedAmount != 40 || result.TakenAmount != 0 || result.LostAmount != 0 || result.DeliveredAmount != 0 || result.AddedAmount != 0 {
		t.Fatalf("amounts = wanted %d taken %d lost %d delivered %d added %d, want 40/0/0/0/0",
			result.WantedAmount, result.TakenAmount, result.LostAmount, result.DeliveredAmount, result.AddedAmount)
	}
	if !result.SourceEmpty {
		t.Fatal("SourceEmpty = false, want true")
	}
	if result.DestinationFull || result.LossApplied {
		t.Fatalf("DestinationFull/LossApplied = %v/%v, want false/false", result.DestinationFull, result.LossApplied)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, now)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 0, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, now)
}

func TestSettleRouteEmitsSettlementAndConditionEvents(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		10,
		[]StoredItem{{ItemID: "void_salt", Quantity: 10}},
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute(first) error = %v, want nil", err)
	}
	if !result.DestinationFull || result.AddedAmount != 0 {
		t.Fatalf("DestinationFull/AddedAmount = %v/%d, want true/0", result.DestinationFull, result.AddedAmount)
	}
	wantWindow := wantSettlementWindow(last, now)
	wantReference := mustRouteSettlementKey(t, route.RouteID, wantWindow)
	if result.SettlementWindow != wantWindow || result.ReferenceKey != wantReference {
		t.Fatalf("settlement evidence = %q/%q, want %q/%q", result.SettlementWindow, result.ReferenceKey, wantWindow, wantReference)
	}
	assertProductionEventTypes(t, store.Events(),
		EventRouteDestinationFull,
		EventRouteTransferSettled,
	)
	assertRouteSettlementEventPayloadEvidence(t, store.Events()[0].Payload, wantReference, wantWindow)
	assertRouteSettlementEventPayloadEvidence(t, store.Events()[1].Payload, wantReference, wantWindow)
	assertSettlementReferenceRecord(t, store.SettlementReferences(), SettlementKindRoute, "", route.RouteID, wantReference, wantWindow, now)
	assertOutboxEventTypes(t, store.OutboxRecords(),
		EventRouteDestinationFull,
		EventRouteTransferSettled,
	)
	assertOutboxRecordEvidence(t, store.OutboxRecords(), EventRouteDestinationFull, wantReference, wantWindow)
	assertOutboxRecordEvidence(t, store.OutboxRecords(), EventRouteTransferSettled, wantReference, wantWindow)
	firstEventCount := len(store.Events())
	firstOutboxCount := len(store.OutboxRecords())
	firstReferenceCount := len(store.SettlementReferences())

	duplicate, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute(second) error = %v, want nil", err)
	}
	if !duplicate.NoOp {
		t.Fatal("duplicate NoOp = false, want true")
	}
	if duplicate.ReferenceKey != "" || duplicate.SettlementWindow != "" {
		t.Fatalf("duplicate evidence = %q/%q, want empty", duplicate.ReferenceKey, duplicate.SettlementWindow)
	}
	if got := len(store.Events()); got != firstEventCount {
		t.Fatalf("event count after duplicate route settlement = %d, want unchanged %d", got, firstEventCount)
	}
	if got := len(store.OutboxRecords()); got != firstOutboxCount {
		t.Fatalf("outbox count after duplicate route settlement = %d, want unchanged %d", got, firstOutboxCount)
	}
	if got := len(store.SettlementReferences()); got != firstReferenceCount {
		t.Fatalf("reference count after duplicate route settlement = %d, want unchanged %d", got, firstReferenceCount)
	}
}

func assertRouteSettlementEventPayloadEvidence(t *testing.T, eventPayload json.RawMessage, reference foundation.IdempotencyKey, window string) {
	t.Helper()
	var payload RouteSettlementPayload
	if err := json.Unmarshal(eventPayload, &payload); err != nil {
		t.Fatalf("json.Unmarshal(route settlement payload) error = %v, want nil", err)
	}
	if payload.ReferenceKey != reference || payload.SettlementWindow != window {
		t.Fatalf("event evidence = %q/%q, want %q/%q", payload.ReferenceKey, payload.SettlementWindow, reference, window)
	}
}

func TestSettleRouteEmitsLossAndSourceEmptyEvents(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	route.Risk = RouteRisk{
		LossChance:     MaxRouteLossChance,
		MinLossPercent: 0.50,
		MaxLossPercent: 0.50,
	}
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 40}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, testutil.NewFakeRNG(nil, []float64{0.10}))

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if !result.LossApplied || !result.SourceEmpty || result.LostAmount != 20 || result.AddedAmount != 20 {
		t.Fatalf("loss/source/amounts = %v/%v/%d/%d, want true/true/20/20",
			result.LossApplied, result.SourceEmpty, result.LostAmount, result.AddedAmount)
	}
	assertProductionEventTypes(t, store.Events(),
		EventRouteTransferLost,
		EventRouteSourceEmpty,
		EventRouteTransferSettled,
	)
}

func TestSettleRouteFullDestinationClampsDelivery(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		10,
		[]StoredItem{{ItemID: "void_salt", Quantity: 10}},
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if result.WantedAmount != 40 || result.TakenAmount != 40 || result.DeliveredAmount != 40 || result.AddedAmount != 0 {
		t.Fatalf("amounts = wanted %d taken %d delivered %d added %d, want 40/40/40/0",
			result.WantedAmount, result.TakenAmount, result.DeliveredAmount, result.AddedAmount)
	}
	if !result.DestinationFull {
		t.Fatal("DestinationFull = false, want true")
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, now)
}

func TestSettleRouteLossChanceAppliesInConfiguredRange(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	route.Risk = RouteRisk{
		LossChance:     MaxRouteLossChance,
		MinLossPercent: 0.25,
		MaxLossPercent: 0.75,
	}
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, testutil.NewFakeRNG(nil, []float64{0.10, 0.50}))

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if !result.LossApplied {
		t.Fatal("LossApplied = false, want true")
	}
	if result.LossPercent != 0.50 {
		t.Fatalf("LossPercent = %.2f, want 0.50", result.LossPercent)
	}
	if result.LostAmount != 20 || result.DeliveredAmount != 20 || result.AddedAmount != 20 {
		t.Fatalf("loss amounts = lost %d delivered %d added %d, want 20/20/20",
			result.LostAmount, result.DeliveredAmount, result.AddedAmount)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 20, now)
}

func TestSettleRouteLossAppliedToSingleUnitCannotRoundToZero(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	route.AmountPerHour = 1
	route.Risk = RouteRisk{
		LossChance:     MaxRouteLossChance,
		MinLossPercent: 0.01,
		MaxLossPercent: 0.01,
	}
	store := newRouteSettlementStore(
		t,
		route,
		10,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 1}},
		10,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, testutil.NewFakeRNG(nil, []float64{0.01}))

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if !result.LossApplied || result.LostAmount != 1 || result.DeliveredAmount != 0 || result.AddedAmount != 0 {
		t.Fatalf("loss applied/lost/delivered/added = %v/%d/%d/%d, want true/1/0/0",
			result.LossApplied, result.LostAmount, result.DeliveredAmount, result.AddedAmount)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 0, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, now)
}

func TestSettleRouteDoubleSettlementDoesNotDuplicateTransfer(t *testing.T) {
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
	service := newTestRouteSettlementService(t, store, now, nil)

	first, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute(first) error = %v, want nil", err)
	}
	second, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute(second) error = %v, want nil", err)
	}
	if first.AddedAmount != 40 {
		t.Fatalf("first AddedAmount = %d, want 40", first.AddedAmount)
	}
	assertRouteMapIdentity(t, first.BeforeRoute, route.SourceMapID, route.DestinationMapID)
	assertRouteMapIdentity(t, first.AfterRoute, route.SourceMapID, route.DestinationMapID)
	if !second.NoOp || second.AddedAmount != 0 || second.ElapsedApplied != 0 {
		t.Fatalf("second NoOp/added/applied = %v/%d/%s, want true/0/0", second.NoOp, second.AddedAmount, second.ElapsedApplied)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, now)
}

func TestSettleRouteFutureTimestampSafe(t *testing.T) {
	now := testRouteNow()
	futureLast := now.Add(time.Hour)
	route := validSettlementRoute(now)
	route.LastCalculatedAt = futureLast
	route.UpdatedAt = futureLast
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if !result.NoOp || result.ElapsedApplied != 0 {
		t.Fatalf("NoOp/applied = %v/%s, want true/0", result.NoOp, result.ElapsedApplied)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, futureLast)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, futureLast)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, futureLast)
}

func TestSettleRouteDisabledRouteNoOpPreservesTimestamp(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	route.Enabled = false
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if !result.NoOp || result.ElapsedRequested != time.Hour || result.ElapsedApplied != 0 {
		t.Fatalf("NoOp/requested/applied = %v/%s/%s, want true/1h/0", result.NoOp, result.ElapsedRequested, result.ElapsedApplied)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

func TestSettleRouteUnsupportedDestinationReturnsErrorWithoutMutation(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	route.Destination = RouteDestination{Type: RouteDestinationTypeStorage, ID: "storage-1"}
	store := NewInMemoryStore()
	insertRouteSettlementRoute(t, store, route)
	saveRouteSettlementStorage(t, store, "planet-1", 100, []StoredItem{{ItemID: "refined_alloy", Quantity: 100}}, last)
	service := newTestRouteSettlementService(t, store, now, nil)

	_, err := service.SettleRoute(route.RouteID)
	if !errors.Is(err, ErrUnsupportedRouteDestination) {
		t.Fatalf("SettleRoute() error = %v, want ErrUnsupportedRouteDestination", err)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
}

func TestSettleRouteOfflineCapApplies(t *testing.T) {
	now := testRouteNow()
	last := now.Add(-100 * time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		5_000,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 5_000}},
		5_000,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if result.ElapsedRequested != 100*time.Hour {
		t.Fatalf("ElapsedRequested = %s, want 100h", result.ElapsedRequested)
	}
	if result.ElapsedApplied != DefaultMaxRouteOfflineSettlementDuration {
		t.Fatalf("ElapsedApplied = %s, want %s", result.ElapsedApplied, DefaultMaxRouteOfflineSettlementDuration)
	}
	wantWindow := wantSettlementWindow(last, last.Add(DefaultMaxRouteOfflineSettlementDuration))
	wantReference := mustRouteSettlementKey(t, route.RouteID, wantWindow)
	if result.SettlementWindow != wantWindow || result.ReferenceKey != wantReference {
		t.Fatalf("capped settlement evidence = %q/%q, want %q/%q", result.SettlementWindow, result.ReferenceKey, wantWindow, wantReference)
	}
	if result.WantedAmount != 2_880 || result.AddedAmount != 2_880 {
		t.Fatalf("wanted/added = %d/%d, want 2880/2880", result.WantedAmount, result.AddedAmount)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, now)
}

func TestSettleRouteSubUnitWantedAdvancesRouteTimestampWithoutTransfer(t *testing.T) {
	last := testRouteNow()
	now := last.Add(30 * time.Minute)
	route := validSettlementRoute(last)
	route.AmountPerHour = 1
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 10}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if result.NoOp || result.WantedAmount != 0 || result.TakenAmount != 0 || result.AddedAmount != 0 {
		t.Fatalf("NoOp/wanted/taken/added = %v/%d/%d/%d, want false/0/0/0",
			result.NoOp, result.WantedAmount, result.TakenAmount, result.AddedAmount)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, now)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 10, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}
