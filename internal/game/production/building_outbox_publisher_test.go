package production

import (
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestPublishPendingProductionOutboxPublishesBuildingMutationRows(t *testing.T) {
	store, reference := queueBuildingMutationOutboxForPublisherTest(t)
	claimedAt := testTime(20)
	completedAt := testTime(21)
	published := make([]ProductionOutboxRecord, 0, 2)

	results, err := PublishPendingProductionOutbox(ProductionOutboxPublishInput{
		Store:       store,
		Limit:       10,
		ClaimedAt:   claimedAt,
		CompletedAt: completedAt,
		Publish: func(record ProductionOutboxRecord) error {
			published = append(published, record)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("PublishPendingProductionOutbox() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Fatalf("PublishPendingProductionOutbox() results = %d, want 2; results = %+v", len(results), results)
	}
	assertBuildingMutationOutboxReferences(t, published, reference, EventPlanetStorageUpdated, EventPlanetBuildingUpdated)
	for index, result := range results {
		if !result.Published || result.Failed || result.StaleClaim || result.ClaimToken == "" {
			t.Fatalf("result[%d] = %+v, want published with claim token", index, result)
		}
	}

	stored := store.OutboxRecords()
	assertBuildingMutationOutboxReferences(t, stored, reference, EventPlanetStorageUpdated, EventPlanetBuildingUpdated)
	for index, record := range stored {
		if record.Status != ProductionOutboxStatusPublished || record.ClaimToken == "" || !record.PublishedAt.Equal(completedAt) {
			t.Fatalf("stored[%d] = %+v, want published with claim token at %s", index, record, completedAt)
		}
	}
	if pending := store.PendingOutboxRecords(10); len(pending) != 0 {
		t.Fatalf("PendingOutboxRecords() = %+v, want none after successful publish", pending)
	}
}

func TestPublishPendingProductionOutboxPreservesBuildingMutationEvidenceOnFailureRetry(t *testing.T) {
	store, reference := queueBuildingMutationOutboxForPublisherTest(t)
	temporaryErr := errors.New("temporary broker outage")
	failedType := EventPlanetStorageUpdated

	results, err := PublishPendingProductionOutbox(ProductionOutboxPublishInput{
		Store:       store,
		Limit:       10,
		ClaimedAt:   testTime(30),
		CompletedAt: testTime(31),
		Publish: func(record ProductionOutboxRecord) error {
			if record.Event.Type == failedType {
				return temporaryErr
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("PublishPendingProductionOutbox() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Fatalf("PublishPendingProductionOutbox() results = %d, want 2; results = %+v", len(results), results)
	}

	var failed ProductionOutboxRecord
	for _, result := range results {
		if result.Failed {
			failed = result.Record
			break
		}
	}
	if failed.OutboxID == "" {
		t.Fatalf("results = %+v, want one failed building mutation row", results)
	}
	if failed.Status != ProductionOutboxStatusFailed || failed.LastError != temporaryErr.Error() || failed.Event.Type != failedType {
		t.Fatalf("failed record = %+v, want failed storage row with error %q", failed, temporaryErr.Error())
	}
	assertBuildingMutationOutboxReferences(t, []ProductionOutboxRecord{failed}, reference, EventPlanetStorageUpdated)

	retried := store.RetryFailedOutboxRecords(1, testTime(32))
	assertBuildingMutationOutboxReferences(t, retried, reference, EventPlanetStorageUpdated)
	if retried[0].Status != ProductionOutboxStatusPending || retried[0].ClaimToken != "" {
		t.Fatalf("retried row = %+v, want pending without stale claim token", retried[0])
	}
	if retried[0].LastError != temporaryErr.Error() || retried[0].FailedAt.IsZero() {
		t.Fatalf("retried failure evidence = %+v, want preserved error and failure time", retried[0])
	}

	republished, err := PublishPendingProductionOutbox(ProductionOutboxPublishInput{
		Store:       store,
		Limit:       1,
		ClaimedAt:   testTime(33),
		CompletedAt: testTime(34),
		Publish: func(ProductionOutboxRecord) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("PublishPendingProductionOutbox(retry) error = %v, want nil", err)
	}
	if len(republished) != 1 ||
		republished[0].Record.Status != ProductionOutboxStatusPublished ||
		!republished[0].Record.FailedAt.IsZero() ||
		republished[0].Record.LastError != "" {
		t.Fatalf("republished building mutation row = %+v, want published without stale failure evidence", republished)
	}
}

func queueBuildingMutationOutboxForPublisherTest(t *testing.T) (*InMemoryStore, foundation.IdempotencyKey) {
	t.Helper()
	store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	service := newTestBuildingMutationService(t, store, MustMVPCatalog(), nil, mapBuildingCosts{
		{operation: BuildingMutationBuild, definitionID: ProductionDefinitionIDAlloyFoundryL1}: {
			Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 20}},
		},
	})
	reference := mustBuildingBuildKey(t, "planet-1", "building-1")
	if _, err := service.BuildPlanetBuilding(BuildPlanetBuildingInput{
		PlanetID:     "planet-1",
		BuildingID:   "building-1",
		DefinitionID: ProductionDefinitionIDAlloyFoundryL1,
		RequestedAt:  testTime(1),
		ReferenceKey: reference,
	}); err != nil {
		t.Fatalf("BuildPlanetBuilding() error = %v, want nil", err)
	}
	assertBuildingMutationOutboxReferences(t, store.PendingOutboxRecords(10), reference, EventPlanetStorageUpdated, EventPlanetBuildingUpdated)
	return store, reference
}
