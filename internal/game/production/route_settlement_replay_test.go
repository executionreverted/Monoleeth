package production

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestApplyRouteSettlementTransactionDuplicateReplaysWithoutLiveRouteRow(t *testing.T) {
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

	first, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: route.OwnerPlayerID,
		RouteID:       route.RouteID,
		SettledAt:     now,
	})
	if err != nil {
		t.Fatalf("ApplyRouteSettlementTransaction(first) error = %v, want nil", err)
	}
	delete(store.routes, route.RouteID)

	duplicate, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: route.OwnerPlayerID,
		RouteID:       route.RouteID,
		SettledAt:     now,
	})
	if err != nil {
		t.Fatalf("ApplyRouteSettlementTransaction(duplicate) error = %v, want durable replay", err)
	}
	if !duplicate.Settlement.NoOp {
		t.Fatal("duplicate settlement NoOp = false, want replay without mutation")
	}
	if duplicate.Settlement.ReferenceKey != reference || duplicate.Settlement.SettlementWindow != window {
		t.Fatalf("duplicate settlement evidence = %q/%q, want %q/%q",
			duplicate.Settlement.ReferenceKey,
			duplicate.Settlement.SettlementWindow,
			reference,
			window,
		)
	}
	if duplicate.Reference == nil || duplicate.Reference.ReferenceKey != reference {
		t.Fatalf("duplicate reference = %+v, want committed reference %q", duplicate.Reference, reference)
	}
	if duplicate.RouteRow == nil || duplicate.RouteRow.ReferenceKey != reference || duplicate.RouteRow.Route != first.Settlement.AfterRoute {
		t.Fatalf("duplicate route row = %+v, want committed first route row", duplicate.RouteRow)
	}
	if !reflect.DeepEqual(duplicate.OutboxRecords, first.OutboxRecords) ||
		!reflect.DeepEqual(duplicate.StorageLedger, first.StorageLedger) ||
		len(duplicate.StorageRows) != len(first.StorageRows) {
		t.Fatalf("duplicate row bundle = outbox %+v ledger %+v storage %+v, want first outbox %+v ledger %+v storage count %d",
			duplicate.OutboxRecords,
			duplicate.StorageLedger,
			duplicate.StorageRows,
			first.OutboxRecords,
			first.StorageLedger,
			len(first.StorageRows),
		)
	}
	if duplicate.Settlement.AfterRoute != first.Settlement.AfterRoute {
		t.Fatalf("duplicate after route = %+v, want first %+v", duplicate.Settlement.AfterRoute, first.Settlement.AfterRoute)
	}
	durableStore := NewInMemorySettlementDurableCommitStore()
	committed, err := duplicate.ApplyDurableCommit(durableStore)
	if err != nil {
		t.Fatalf("duplicate ApplyDurableCommit() error = %v, want nil", err)
	}
	if committed.Reference == nil ||
		committed.RouteRow == nil ||
		len(committed.OutboxRecords) != len(first.OutboxRecords) ||
		len(committed.RouteStorageLedger) != len(first.StorageLedger) {
		t.Fatalf("duplicate durable commit = %+v, want committed route settlement rows", committed)
	}
	assertRouteDurableRecord(t, store, route.RouteID, reference, 2, first.Settlement.AfterRoute)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, now)
}

func TestApplyRouteSettlementTransactionReplayRejectsWrongOwnerWithoutLiveRouteRow(t *testing.T) {
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

	if _, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: route.OwnerPlayerID,
		RouteID:       route.RouteID,
		SettledAt:     now,
	}); err != nil {
		t.Fatalf("ApplyRouteSettlementTransaction(first) error = %v, want nil", err)
	}
	delete(store.routes, route.RouteID)

	_, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: "player-2",
		RouteID:       route.RouteID,
		SettledAt:     now,
	})
	if !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("ApplyRouteSettlementTransaction(wrong owner) error = %v, want ErrRouteOwnerMismatch", err)
	}
}

func TestApplyRouteSettlementTransactionReplayDoesNotMaskFutureSettlementWithoutLiveRouteRow(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	future := now.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)

	if _, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: route.OwnerPlayerID,
		RouteID:       route.RouteID,
		SettledAt:     now,
	}); err != nil {
		t.Fatalf("ApplyRouteSettlementTransaction(first) error = %v, want nil", err)
	}
	delete(store.routes, route.RouteID)

	_, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: route.OwnerPlayerID,
		RouteID:       route.RouteID,
		SettledAt:     future,
	})
	if !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("ApplyRouteSettlementTransaction(future) error = %v, want ErrRouteNotFound", err)
	}
}

func TestApplyRouteSettlementTransactionReplayRejectsMissingHandoffRowsWithoutLiveRouteRow(t *testing.T) {
	cases := map[string]func(*InMemoryStore){
		"missing outbox": func(store *InMemoryStore) {
			store.outbox = nil
		},
		"missing ledger": func(store *InMemoryStore) {
			store.routeStorageLedger = nil
		},
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
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

			if _, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
				OwnerPlayerID: route.OwnerPlayerID,
				RouteID:       route.RouteID,
				SettledAt:     now,
			}); err != nil {
				t.Fatalf("ApplyRouteSettlementTransaction(first) error = %v, want nil", err)
			}
			delete(store.routes, route.RouteID)
			corrupt(store)

			_, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
				OwnerPlayerID: route.OwnerPlayerID,
				RouteID:       route.RouteID,
				SettledAt:     now,
			})
			if !errors.Is(err, ErrInvalidSettlementDurableCommit) {
				t.Fatalf("ApplyRouteSettlementTransaction(%s) error = %v, want ErrInvalidSettlementDurableCommit", name, err)
			}
		})
	}
}
