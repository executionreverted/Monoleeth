package discovery

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestClaimPlanetBoundaryReadAPIsReturnDetachedRecordsAndValidateLookup(t *testing.T) {
	store := NewInMemoryStore()
	planetA := claimTestPlanet("planet-claim-a")
	planetB := claimTestPlanet("planet-claim-b")
	materializeClaimTestPlanet(t, store, planetB)
	materializeClaimTestPlanet(t, store, planetA)
	upsertClaimIntel(t, store, claimTestPlayerID, planetA.ID, testTime(1))
	upsertClaimIntel(t, store, claimTestPlayerID, planetB.ID, testTime(1))
	service := newClaimTestService(t, claimTestServiceOptions{
		store:    store,
		rank:     planetA.Level,
		inRange:  true,
		consumer: &recordingClaimXCoreConsumer{},
	})

	if _, err := service.ClaimPlanet(claimInput("claim_ref_b", planetB.ID)); err != nil {
		t.Fatalf("ClaimPlanet(B) error = %v, want nil", err)
	}
	if _, err := service.ClaimPlanet(claimInput("claim_ref_a", planetA.ID)); err != nil {
		t.Fatalf("ClaimPlanet(A) error = %v, want nil", err)
	}

	references := service.ClaimReferences()
	if len(references) != 2 {
		t.Fatalf("ClaimReferences() len = %d, want 2", len(references))
	}
	if references[0].ClaimReference != "claim_ref_a" || references[1].ClaimReference != "claim_ref_b" {
		t.Fatalf("ClaimReferences() order = %+v, want sorted by reference", references)
	}
	lookup, ok, err := service.ClaimReference("claim_ref_a")
	if err != nil || !ok {
		t.Fatalf("ClaimReference(claim_ref_a) ok = %v err = %v, want true nil", ok, err)
	}
	if lookup.PlanetID != planetA.ID {
		t.Fatalf("ClaimReference(claim_ref_a) planet = %q, want %q", lookup.PlanetID, planetA.ID)
	}
	if _, ok, err := service.ClaimReference(""); err == nil || ok {
		t.Fatalf("ClaimReference(invalid) ok = %v err = %v, want false error", ok, err)
	}

	originalReferenceTime := references[0].ClaimedAt
	references[0].ClaimedAt = testTime(99)
	references[0].RecordedAt = testTime(100)
	references[0].AlreadyOwned = true
	references[0].EventID = "event_mutated"
	storedReference, ok, err := service.ClaimReference("claim_ref_a")
	if err != nil || !ok {
		t.Fatalf("ClaimReference(after mutate) ok = %v err = %v, want true nil", ok, err)
	}
	if !storedReference.ClaimedAt.Equal(originalReferenceTime) || storedReference.AlreadyOwned {
		t.Fatalf("stored reference after returned mutation = %+v, want original time and not already-owned", storedReference)
	}
	if storedReference.EventID == "event_mutated" {
		t.Fatalf("stored reference event id mutated through returned record")
	}

	outbox := service.ClaimOutboxRecords()
	if len(outbox) != 2 {
		t.Fatalf("ClaimOutboxRecords() len = %d, want 2", len(outbox))
	}
	originalStatus := outbox[0].Status
	originalCreatedAt := outbox[0].CreatedAt
	originalEventType := outbox[0].Event.Type
	originalEventCreatedAt := outbox[0].Event.CreatedAt
	outbox[0].Status = ClaimOutboxStatus("sent")
	outbox[0].CreatedAt = testTime(101)
	outbox[0].Event.Type = ClaimEventType("mutated")
	outbox[0].Event.CreatedAt = testTime(102)

	storedOutbox := service.ClaimOutboxRecords()
	if storedOutbox[0].Status != originalStatus || !storedOutbox[0].CreatedAt.Equal(originalCreatedAt) {
		t.Fatalf("stored outbox after returned mutation = %+v, want status/time unchanged", storedOutbox[0])
	}
	if storedOutbox[0].Event.Type != originalEventType || !storedOutbox[0].Event.CreatedAt.Equal(originalEventCreatedAt) {
		t.Fatalf("stored outbox event after returned mutation = %+v, want event unchanged", storedOutbox[0].Event)
	}

	events := service.Events()
	originalReturnedEventType := events[0].Type
	events[0].Type = ClaimEventType("mutated_event")
	storedEvents := service.Events()
	if storedEvents[0].Type != originalReturnedEventType {
		t.Fatalf("stored event type after returned mutation = %q, want %q", storedEvents[0].Type, originalReturnedEventType)
	}
}

func TestClaimBoundaryRecordsCanonicalIdempotencyKeyEvidence(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-canonical")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	service := newClaimTestService(t, claimTestServiceOptions{
		store:    store,
		rank:     planet.Level,
		inRange:  true,
		consumer: &recordingClaimXCoreConsumer{},
	})
	key, err := foundation.PlanetClaimIdempotencyKey(claimTestPlayerID, planet.ID)
	if err != nil {
		t.Fatalf("PlanetClaimIdempotencyKey: %v", err)
	}
	ref := PlanetClaimReference(key.String())

	if _, err := service.ClaimPlanet(claimInput(ref, planet.ID)); err != nil {
		t.Fatalf("ClaimPlanet() error = %v, want nil", err)
	}

	reference, ok, err := service.ClaimReference(ref)
	if err != nil || !ok {
		t.Fatalf("ClaimReference() ok = %v err = %v, want true nil", ok, err)
	}
	if reference.ReferenceKey != key {
		t.Fatalf("claim reference key = %q, want %q", reference.ReferenceKey, key)
	}
	outbox := service.ClaimOutboxRecords()
	if len(outbox) != 1 {
		t.Fatalf("ClaimOutboxRecords() len = %d, want 1", len(outbox))
	}
	if outbox[0].ReferenceKey != key {
		t.Fatalf("claim outbox reference key = %q, want %q", outbox[0].ReferenceKey, key)
	}

	reference.ReferenceKey = "mutated_reference"
	outbox[0].ReferenceKey = "mutated_outbox"
	storedReference, _, _ := service.ClaimReference(ref)
	storedOutbox := service.ClaimOutboxRecords()
	if storedReference.ReferenceKey != key || storedOutbox[0].ReferenceKey != key {
		t.Fatalf("stored reference/outbox evidence mutated = %q/%q, want %q", storedReference.ReferenceKey, storedOutbox[0].ReferenceKey, key)
	}
}

func TestPlanetClaimReferenceIdempotencyKeyRequiresExpectedClaimEntity(t *testing.T) {
	planetID := foundation.PlanetID("planet-claim-expected")
	key, err := foundation.PlanetClaimIdempotencyKey(claimTestPlayerID, planetID)
	if err != nil {
		t.Fatalf("PlanetClaimIdempotencyKey: %v", err)
	}
	if got, ok := PlanetClaimReference(key.String()).IdempotencyKey(claimTestPlayerID, planetID); !ok || got != key {
		t.Fatalf("IdempotencyKey(expected) = %q/%v, want %q/true", got, ok, key)
	}
	if got, ok := PlanetClaimReference(key.String()).IdempotencyKey(claimTestPlayerID, "planet-claim-other"); ok || !got.IsZero() {
		t.Fatalf("IdempotencyKey(wrong planet) = %q/%v, want zero/false", got, ok)
	}
	questKey, err := foundation.QuestRewardIdempotencyKey("quest-claim-not-a-claim")
	if err != nil {
		t.Fatalf("QuestRewardIdempotencyKey: %v", err)
	}
	if got, ok := PlanetClaimReference(questKey.String()).IdempotencyKey(claimTestPlayerID, planetID); ok || !got.IsZero() {
		t.Fatalf("IdempotencyKey(wrong domain) = %q/%v, want zero/false", got, ok)
	}
}

func TestClaimOutboxPendingRecordsFilterByStatusLimitAndAppendOrder(t *testing.T) {
	service := newClaimOutboxStateMachineService(t, 4)

	all := service.ClaimOutboxRecords()
	if len(all) != 4 {
		t.Fatalf("ClaimOutboxRecords() len = %d, want 4", len(all))
	}

	pending := service.PendingClaimOutboxRecords(2)
	assertClaimOutboxSequences(t, pending, 1, 2)

	claimed := service.ClaimPendingClaimOutboxRecords(1, testTime(30))
	assertClaimOutboxSequences(t, claimed, 1)
	pending = service.PendingClaimOutboxRecords(10)
	assertClaimOutboxSequences(t, pending, 2, 3, 4)

	if got := service.PendingClaimOutboxRecords(0); got != nil {
		t.Fatalf("PendingClaimOutboxRecords(0) = %+v, want nil", got)
	}
}

func TestClaimOutboxClaimMovesPendingToInFlightWithDetachedRecords(t *testing.T) {
	service := newClaimOutboxStateMachineService(t, 3)
	claimedAt := testTime(31)

	claimed := service.ClaimPendingClaimOutboxRecords(2, claimedAt)
	if len(claimed) != 2 {
		t.Fatalf("ClaimPendingClaimOutboxRecords() len = %d, want 2", len(claimed))
	}
	for index, record := range claimed {
		if record.Status != ClaimOutboxStatusInFlight {
			t.Fatalf("claimed[%d] status = %q, want in_flight", index, record.Status)
		}
		if !record.ClaimedAt.Equal(claimedAt) {
			t.Fatalf("claimed[%d] ClaimedAt = %s, want %s", index, record.ClaimedAt, claimedAt)
		}
		if record.Attempts != 1 {
			t.Fatalf("claimed[%d] Attempts = %d, want 1", index, record.Attempts)
		}
		wantClaimToken := claimOutboxClaimToken(record.OutboxID, record.Attempts)
		if record.ClaimToken != wantClaimToken {
			t.Fatalf("claimed[%d] ClaimToken = %q, want %q", index, record.ClaimToken, wantClaimToken)
		}
	}

	originalEventCreatedAt := claimed[0].Event.CreatedAt
	claimed[0].Status = ClaimOutboxStatusPublished
	claimed[0].ClaimedAt = testTime(99)
	claimed[0].Event.CreatedAt = testTime(100)

	stored := service.ClaimOutboxRecords()
	if stored[0].Status != ClaimOutboxStatusInFlight {
		t.Fatalf("stored[0] status = %q, want in_flight", stored[0].Status)
	}
	if !stored[0].ClaimedAt.Equal(claimedAt) {
		t.Fatalf("stored[0] ClaimedAt = %s, want %s", stored[0].ClaimedAt, claimedAt)
	}
	if !stored[0].Event.CreatedAt.Equal(originalEventCreatedAt) {
		t.Fatalf("stored[0] event CreatedAt = %s, want original %s", stored[0].Event.CreatedAt, originalEventCreatedAt)
	}
}

func TestClaimOutboxPublishedRecordsAreTerminal(t *testing.T) {
	service := newClaimOutboxStateMachineService(t, 3)
	claimed := service.ClaimPendingClaimOutboxRecords(2, testTime(32))
	publishedAt := testTime(33)

	published, ok := service.MarkClaimedClaimOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, publishedAt)
	if !ok {
		t.Fatal("MarkClaimedClaimOutboxPublished() ok = false, want true")
	}
	if published.Status != ClaimOutboxStatusPublished {
		t.Fatalf("published status = %q, want published", published.Status)
	}
	if !published.PublishedAt.Equal(publishedAt) {
		t.Fatalf("PublishedAt = %s, want %s", published.PublishedAt, publishedAt)
	}

	beforeLateFailure := service.ClaimOutboxRecords()
	if record, ok := service.MarkClaimedClaimOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "late failure", testTime(34)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedClaimOutboxFailed(published) = %+v/%v, want zero/false", record, ok)
	}
	assertClaimOutboxUnchanged(t, service, beforeLateFailure, "late failure after publish")
	retryable := service.RetryFailedClaimOutboxRecords(10, testTime(35))
	for _, record := range retryable {
		if record.OutboxID == published.OutboxID {
			t.Fatalf("published record %q appeared in retryable records: %+v", published.OutboxID, retryable)
		}
	}
	pending := service.PendingClaimOutboxRecords(10)
	for _, record := range pending {
		if record.OutboxID == published.OutboxID {
			t.Fatalf("published record %q appeared in pending records: %+v", published.OutboxID, pending)
		}
	}
	stored := service.ClaimOutboxRecords()[0]
	if stored.Status != ClaimOutboxStatusPublished || !stored.PublishedAt.Equal(publishedAt) {
		t.Fatalf("stored published record = %+v, want published with original PublishedAt %s", stored, publishedAt)
	}
}

func TestClaimOutboxFailRetryAndStaleClaimTokens(t *testing.T) {
	service := newClaimOutboxStateMachineService(t, 1)
	claimed := service.ClaimPendingClaimOutboxRecords(1, testTime(36))
	first := claimed[0]
	failedAt := testTime(37)

	failed, ok := service.MarkClaimedClaimOutboxFailed(first.OutboxID, first.ClaimToken, "temporary broker outage", failedAt)
	if !ok {
		t.Fatal("MarkClaimedClaimOutboxFailed() ok = false, want true")
	}
	if failed.Status != ClaimOutboxStatusFailed {
		t.Fatalf("failed status = %q, want failed", failed.Status)
	}
	if failed.LastError != "temporary broker outage" || !failed.FailedAt.Equal(failedAt) {
		t.Fatalf("failure evidence = %q/%s, want reason/%s", failed.LastError, failed.FailedAt, failedAt)
	}

	retriedAt := testTime(38)
	retried := service.RetryFailedClaimOutboxRecords(1, retriedAt)
	if len(retried) != 1 {
		t.Fatalf("RetryFailedClaimOutboxRecords() len = %d, want 1", len(retried))
	}
	if retried[0].Status != ClaimOutboxStatusPending {
		t.Fatalf("retried status = %q, want pending", retried[0].Status)
	}
	if retried[0].ClaimToken != "" || !retried[0].ClaimedAt.IsZero() {
		t.Fatalf("retried claim evidence = token %q claimed_at %s, want cleared", retried[0].ClaimToken, retried[0].ClaimedAt)
	}
	if !retried[0].FailedAt.Equal(failedAt) || retried[0].LastError != "temporary broker outage" {
		t.Fatalf("retried failure evidence = %q/%s, want preserved %q/%s", retried[0].LastError, retried[0].FailedAt, failed.LastError, failedAt)
	}
	if !retried[0].RetriedAt.Equal(retriedAt) {
		t.Fatalf("RetriedAt = %s, want %s", retried[0].RetriedAt, retriedAt)
	}

	beforePendingStale := service.ClaimOutboxRecords()
	if record, ok := service.MarkClaimedClaimOutboxPublished(first.OutboxID, first.ClaimToken, testTime(39)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedClaimOutboxPublished(stale pending token) = %+v/%v, want zero/false", record, ok)
	}
	if record, ok := service.MarkClaimedClaimOutboxFailed(first.OutboxID, first.ClaimToken, "stale pending failure", testTime(40)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedClaimOutboxFailed(stale pending token) = %+v/%v, want zero/false", record, ok)
	}
	assertClaimOutboxUnchanged(t, service, beforePendingStale, "stale token while pending")

	reclaimed := service.ClaimPendingClaimOutboxRecords(1, testTime(41))
	if len(reclaimed) != 1 {
		t.Fatalf("reclaim len = %d, want 1", len(reclaimed))
	}
	second := reclaimed[0]
	if second.Attempts != 2 {
		t.Fatalf("reclaimed Attempts = %d, want 2", second.Attempts)
	}
	if second.ClaimToken == first.ClaimToken {
		t.Fatalf("reclaimed ClaimToken = first token %q, want new attempt token", second.ClaimToken)
	}
	wantClaimToken := claimOutboxClaimToken(second.OutboxID, second.Attempts)
	if second.ClaimToken != wantClaimToken {
		t.Fatalf("reclaimed ClaimToken = %q, want %q", second.ClaimToken, wantClaimToken)
	}

	beforeReclaimedStale := service.ClaimOutboxRecords()
	if record, ok := service.MarkClaimedClaimOutboxPublished(second.OutboxID, first.ClaimToken, testTime(42)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedClaimOutboxPublished(stale reclaimed token) = %+v/%v, want zero/false", record, ok)
	}
	if record, ok := service.MarkClaimedClaimOutboxFailed(second.OutboxID, first.ClaimToken, "stale reclaimed failure", testTime(43)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedClaimOutboxFailed(stale reclaimed token) = %+v/%v, want zero/false", record, ok)
	}
	assertClaimOutboxUnchanged(t, service, beforeReclaimedStale, "stale token after reclaim")

	published, ok := service.MarkClaimedClaimOutboxPublished(second.OutboxID, second.ClaimToken, testTime(44))
	if !ok {
		t.Fatal("MarkClaimedClaimOutboxPublished(new token) ok = false, want true")
	}
	if published.Status != ClaimOutboxStatusPublished {
		t.Fatalf("published status = %q, want published", published.Status)
	}
}

func TestClaimOutboxWrongTokenAndMissingIDDoNotMutateRecords(t *testing.T) {
	service := newClaimOutboxStateMachineService(t, 1)
	claimed := service.ClaimPendingClaimOutboxRecords(1, testTime(45))
	first := claimed[0]

	beforeWrongPublish := service.ClaimOutboxRecords()
	if record, ok := service.MarkClaimedClaimOutboxPublished(first.OutboxID, "wrong-token", testTime(46)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedClaimOutboxPublished(wrong token) = %+v/%v, want zero/false", record, ok)
	}
	assertClaimOutboxUnchanged(t, service, beforeWrongPublish, "wrong publish token")

	beforeWrongFail := service.ClaimOutboxRecords()
	if record, ok := service.MarkClaimedClaimOutboxFailed(first.OutboxID, "wrong-token", "wrong token failure", testTime(47)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedClaimOutboxFailed(wrong token) = %+v/%v, want zero/false", record, ok)
	}
	assertClaimOutboxUnchanged(t, service, beforeWrongFail, "wrong fail token")

	beforeMissing := service.ClaimOutboxRecords()
	if record, ok := service.MarkClaimedClaimOutboxPublished("missing-outbox", "missing-token", testTime(48)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedClaimOutboxPublished(missing) = %+v/%v, want zero/false", record, ok)
	}
	if record, ok := service.MarkClaimedClaimOutboxFailed("missing-outbox", "missing-token", "missing", testTime(49)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedClaimOutboxFailed(missing) = %+v/%v, want zero/false", record, ok)
	}
	assertClaimOutboxUnchanged(t, service, beforeMissing, "missing outbox id")
}

func TestPublishPendingClaimOutboxPublishesAndFailsWithClaimTokens(t *testing.T) {
	service := newClaimOutboxStateMachineService(t, 3)
	claimAt := testTime(50)
	completedAt := testTime(51)
	temporaryErr := errors.New("temporary broker outage")
	publishedIDs := make([]string, 0, 2)

	results, err := PublishPendingClaimOutbox(ClaimOutboxPublishInput{
		Store:       service,
		Limit:       2,
		ClaimedAt:   claimAt,
		CompletedAt: completedAt,
		Publish: func(record ClaimOutboxRecord) error {
			publishedIDs = append(publishedIDs, record.OutboxID)
			if len(publishedIDs) == 2 {
				return temporaryErr
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("PublishPendingClaimOutbox() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Fatalf("PublishPendingClaimOutbox() len = %d, want 2; results = %+v", len(results), results)
	}
	if !results[0].Published || results[0].Failed || results[0].StaleClaim {
		t.Fatalf("first publish result = %+v, want published only", results[0])
	}
	if !results[1].Failed || results[1].Published || results[1].StaleClaim || results[1].Error != temporaryErr.Error() {
		t.Fatalf("second publish result = %+v, want failed with error", results[1])
	}
	if results[0].ClaimToken == "" || results[1].ClaimToken == "" {
		t.Fatalf("publish claim tokens = %q/%q, want non-empty", results[0].ClaimToken, results[1].ClaimToken)
	}

	stored := service.ClaimOutboxRecords()
	if stored[0].Status != ClaimOutboxStatusPublished || !stored[0].PublishedAt.Equal(completedAt) {
		t.Fatalf("stored first claim outbox = %+v, want published at %s", stored[0], completedAt)
	}
	if stored[1].Status != ClaimOutboxStatusFailed || stored[1].LastError != temporaryErr.Error() || !stored[1].FailedAt.Equal(completedAt) {
		t.Fatalf("stored second claim outbox = %+v, want failed with error at %s", stored[1], completedAt)
	}
	pending := service.PendingClaimOutboxRecords(10)
	for _, record := range pending {
		if record.OutboxID == stored[0].OutboxID || record.OutboxID == stored[1].OutboxID {
			t.Fatalf("published/failed claim record appeared as pending: %+v", pending)
		}
	}
}

func TestPublishPendingClaimOutboxRejectsInvalidPublisher(t *testing.T) {
	if results, err := PublishPendingClaimOutbox(ClaimOutboxPublishInput{}); !errors.Is(err, ErrInvalidClaimOutboxPublisher) || results != nil {
		t.Fatalf("PublishPendingClaimOutbox(invalid) = %+v/%v, want invalid publisher error", results, err)
	}
	service := newClaimOutboxStateMachineService(t, 1)
	if results, err := PublishPendingClaimOutbox(ClaimOutboxPublishInput{
		Store: service,
		Limit: 1,
	}); !errors.Is(err, ErrInvalidClaimOutboxPublisher) || results != nil {
		t.Fatalf("PublishPendingClaimOutbox(nil publish) = %+v/%v, want invalid publisher error", results, err)
	}
}

func TestPublishPendingClaimOutboxReportsStaleClaimWhenMarkRejected(t *testing.T) {
	temporaryErr := errors.New("temporary broker outage")

	successStore := staleClaimOutboxPublisherStore{
		ClaimService: newClaimOutboxStateMachineService(t, 1),
		stalePublish: true,
	}
	successResults, err := PublishPendingClaimOutbox(ClaimOutboxPublishInput{
		Store:       successStore,
		Limit:       1,
		ClaimedAt:   testTime(52),
		CompletedAt: testTime(53),
		Publish:     func(ClaimOutboxRecord) error { return nil },
	})
	if err != nil {
		t.Fatalf("PublishPendingClaimOutbox(stale publish) error = %v, want nil", err)
	}
	if len(successResults) != 1 || !successResults[0].StaleClaim || successResults[0].Published || successResults[0].Failed || successResults[0].StoreError {
		t.Fatalf("stale publish result = %+v, want stale claim only", successResults)
	}

	failStore := staleClaimOutboxPublisherStore{
		ClaimService: newClaimOutboxStateMachineService(t, 1),
		staleFail:    true,
	}
	failResults, err := PublishPendingClaimOutbox(ClaimOutboxPublishInput{
		Store:       failStore,
		Limit:       1,
		ClaimedAt:   testTime(54),
		CompletedAt: testTime(55),
		Publish:     func(ClaimOutboxRecord) error { return temporaryErr },
	})
	if err != nil {
		t.Fatalf("PublishPendingClaimOutbox(stale fail) error = %v, want nil", err)
	}
	if len(failResults) != 1 || !failResults[0].StaleClaim || failResults[0].Published || failResults[0].Failed || failResults[0].StoreError || failResults[0].Error != temporaryErr.Error() {
		t.Fatalf("stale fail result = %+v, want stale claim with publish error", failResults)
	}
}

type staleClaimOutboxPublisherStore struct {
	*ClaimService
	stalePublish bool
	staleFail    bool
}

func (store staleClaimOutboxPublisherStore) MarkClaimOutboxPublished(outboxID string, claimToken string, publishedAt time.Time) (ClaimOutboxRecord, bool, error) {
	if store.stalePublish {
		return ClaimOutboxRecord{}, false, nil
	}
	return store.ClaimService.MarkClaimOutboxPublished(outboxID, claimToken, publishedAt)
}

func (store staleClaimOutboxPublisherStore) MarkClaimOutboxFailed(outboxID string, claimToken string, reason string, failedAt time.Time) (ClaimOutboxRecord, bool, error) {
	if store.staleFail {
		return ClaimOutboxRecord{}, false, nil
	}
	return store.ClaimService.MarkClaimOutboxFailed(outboxID, claimToken, reason, failedAt)
}

func newClaimOutboxStateMachineService(t *testing.T, planetCount int) *ClaimService {
	t.Helper()
	store := NewInMemoryStore()
	service := newClaimTestService(t, claimTestServiceOptions{
		store:    store,
		rank:     10,
		inRange:  true,
		consumer: &recordingClaimXCoreConsumer{},
	})
	for index := 0; index < planetCount; index++ {
		planetID := foundation.PlanetID("planet-claim-outbox-" + string(rune('a'+index)))
		planet := claimTestPlanet(planetID)
		materializeClaimTestPlanet(t, store, planet)
		upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
		ref := PlanetClaimReference("claim_outbox_" + string(rune('a'+index)))
		if _, err := service.ClaimPlanet(claimInput(ref, planet.ID)); err != nil {
			t.Fatalf("ClaimPlanet(%q) error = %v, want nil", ref, err)
		}
	}
	return service
}

func assertClaimOutboxUnchanged(t *testing.T, service *ClaimService, before []ClaimOutboxRecord, context string) {
	t.Helper()
	after := service.ClaimOutboxRecords()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("claim outbox mutated after %s:\nbefore=%+v\nafter=%+v", context, before, after)
	}
}

func assertClaimOutboxSequences(t *testing.T, records []ClaimOutboxRecord, want ...uint64) {
	t.Helper()
	if len(records) != len(want) {
		t.Fatalf("claim outbox records len = %d, want %d; records = %+v", len(records), len(want), records)
	}
	for index, wantSequence := range want {
		if records[index].Sequence != wantSequence {
			t.Fatalf("claim outbox records[%d] sequence = %d, want %d; records = %+v", index, records[index].Sequence, wantSequence, records)
		}
	}
}
