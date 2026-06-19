package discovery

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

const claimTestPlayerID foundation.PlayerID = "player_claimant"

func TestClaimPlanetWithoutIntelRejectedWithoutXCoreConsume(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	consumer := &recordingClaimXCoreConsumer{}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:    store,
		rank:     planet.Level,
		inRange:  true,
		consumer: consumer,
	})

	_, err := service.ClaimPlanet(claimInput("claim_no_intel", planet.ID))
	if !errors.Is(err, ErrPlanetClaimRequiresIntel) {
		t.Fatalf("ClaimPlanet() error = %v, want ErrPlanetClaimRequiresIntel", err)
	}
	if got := len(consumer.calls); got != 0 {
		t.Fatalf("x core consume calls = %d, want 0", got)
	}
}

func TestClaimPlanetLowRankRejectedWithoutXCoreConsume(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	consumer := &recordingClaimXCoreConsumer{}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:    store,
		rank:     planet.Level - 1,
		inRange:  true,
		consumer: consumer,
	})

	_, err := service.ClaimPlanet(claimInput("claim_low_rank", planet.ID))
	if !errors.Is(err, ErrPlanetClaimRankTooLow) {
		t.Fatalf("ClaimPlanet() error = %v, want ErrPlanetClaimRankTooLow", err)
	}
	if got := len(consumer.calls); got != 0 {
		t.Fatalf("x core consume calls = %d, want 0", got)
	}
}

func TestClaimPlanetProximityFailureRejectedWithoutXCoreConsume(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	consumer := &recordingClaimXCoreConsumer{}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:    store,
		rank:     planet.Level,
		inRange:  false,
		consumer: consumer,
	})

	_, err := service.ClaimPlanet(claimInput("claim_out_of_range", planet.ID))
	if !errors.Is(err, ErrPlanetClaimProximity) {
		t.Fatalf("ClaimPlanet() error = %v, want ErrPlanetClaimProximity", err)
	}
	if got := len(consumer.calls); got != 0 {
		t.Fatalf("x core consume calls = %d, want 0", got)
	}
}

func TestClaimPlanetRejectedDoesNotInitializeProduction(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	initializer := &recordingClaimProductionInitializer{}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:       store,
		rank:        planet.Level - 1,
		inRange:     true,
		consumer:    &recordingClaimXCoreConsumer{},
		initializer: initializer,
	})

	_, err := service.ClaimPlanet(claimInput("claim_rejected_no_init", planet.ID))
	if !errors.Is(err, ErrPlanetClaimRankTooLow) {
		t.Fatalf("ClaimPlanet() error = %v, want ErrPlanetClaimRankTooLow", err)
	}
	if got := len(initializer.calls); got != 0 {
		t.Fatalf("production initializer calls = %d, want 0", got)
	}
}

func TestClaimPlanetXCoreFailureLeavesPlanetUnownedAndEmitsNoEvent(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	consumeErr := errors.New("consume failed")
	service := newClaimTestService(t, claimTestServiceOptions{
		store:    store,
		rank:     planet.Level,
		inRange:  true,
		consumer: &recordingClaimXCoreConsumer{err: consumeErr},
	})

	_, err := service.ClaimPlanet(claimInput("claim_consume_fail", planet.ID))
	if !errors.Is(err, consumeErr) {
		t.Fatalf("ClaimPlanet() error = %v, want consumeErr", err)
	}
	stored, ok, err := store.Planet(planet.ID)
	if err != nil || !ok {
		t.Fatalf("Planet() ok = %v err = %v, want true nil", ok, err)
	}
	if !stored.OwnerPlayerID.IsZero() || stored.OwnerChangedAt != nil {
		t.Fatalf("owner after consume failure = %q at %v, want unowned", stored.OwnerPlayerID, stored.OwnerChangedAt)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events after consume failure = %d, want 0", got)
	}
}

func TestClaimPlanetSuccessConsumesXCoreSetsOwnerEmitsEventAndMarksIntelStale(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	upsertClaimIntel(t, store, "player_cartographer", planet.ID, testTime(2))
	consumer := &recordingClaimXCoreConsumer{}
	staleMarker := &recordingClaimListedIntelStaleMarker{markedCount: 3}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:       store,
		rank:        planet.Level,
		inRange:     true,
		consumer:    consumer,
		staleMarker: staleMarker,
	})

	result, err := service.ClaimPlanet(claimInput("claim_success", planet.ID))
	if err != nil {
		t.Fatalf("ClaimPlanet() error = %v, want nil", err)
	}
	if !result.Claimed || result.Planet.OwnerPlayerID != claimTestPlayerID {
		t.Fatalf("claim result = %+v, want claimed by %q", result, claimTestPlayerID)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls = %d, want 1", got)
	}
	consume := consumer.calls[0]
	if consume.PlayerID != claimTestPlayerID || consume.PlanetID != planet.ID {
		t.Fatalf("consume identity = %+v, want player %q planet %q", consume, claimTestPlayerID, planet.ID)
	}
	if consume.Quantity != defaultClaimXCoreQuantity {
		t.Fatalf("consume quantity = %d, want %d", consume.Quantity, defaultClaimXCoreQuantity)
	}
	if consume.SourceLocation.Kind != economy.LocationKindAccountInventory || consume.SourceLocation.ID.String() != claimTestPlayerID.String() {
		t.Fatalf("consume source location = %+v, want claimant account inventory", consume.SourceLocation)
	}
	if consume.ItemRef.Definition.ItemID != foundation.ItemID("x_core") {
		t.Fatalf("consume item id = %q, want x_core", consume.ItemRef.Definition.ItemID)
	}
	if consume.Reason != defaultClaimReason || consume.Reference != "claim_success" {
		t.Fatalf("consume reason/reference = %q/%q, want %q/claim_success", consume.Reason, consume.Reference, defaultClaimReason)
	}

	stored, ok, err := store.Planet(planet.ID)
	if err != nil || !ok {
		t.Fatalf("Planet() ok = %v err = %v, want true nil", ok, err)
	}
	if stored.OwnerPlayerID != claimTestPlayerID || stored.OwnerChangedAt == nil || !stored.OwnerChangedAt.Equal(claimTestTime()) {
		t.Fatalf("stored owner = %+v, want %q at %s", stored, claimTestPlayerID, claimTestTime())
	}
	events := service.Events()
	if len(events) != 1 || events[0].Type != ClaimEventPlanetClaimed {
		t.Fatalf("claim events = %+v, want one %s", events, ClaimEventPlanetClaimed)
	}
	if result.StaleIntelCount != 2 {
		t.Fatalf("stale intel count = %d, want 2", result.StaleIntelCount)
	}
	if result.StaleListingCount != 3 {
		t.Fatalf("stale listing count = %d, want 3", result.StaleListingCount)
	}
	if got := len(staleMarker.calls); got != 1 {
		t.Fatalf("stale marker calls = %d, want 1", got)
	}
	if call := staleMarker.calls[0]; call.PlayerID != claimTestPlayerID || call.PlanetID != planet.ID || call.Reason != "planet_claimed" {
		t.Fatalf("stale marker call = %+v, want player/planet/planet_claimed", call)
	}
	stale, ok, err := store.PlayerPlanetIntel("player_cartographer", planet.ID)
	if err != nil || !ok {
		t.Fatalf("PlayerPlanetIntel(cartographer) ok = %v err = %v, want true nil", ok, err)
	}
	if stale.State != IntelStateStale || stale.Confidence != staleIntelConfidence {
		t.Fatalf("cartographer intel = %+v, want stale confidence %d", stale, staleIntelConfidence)
	}
	if stale.SourceType != IntelSourcePlanetOwnerChanged || !strings.HasPrefix(stale.SourceReference, "planet.claimed:") {
		t.Fatalf("stale source = %s/%s, want planet owner changed by claim event", stale.SourceType, stale.SourceReference)
	}
}

func TestClaimPlanetNilProductionInitializerKeepsSuccessBehavior(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	consumer := &recordingClaimXCoreConsumer{}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:    store,
		rank:     planet.Level,
		inRange:  true,
		consumer: consumer,
	})

	result, err := service.ClaimPlanet(claimInput("claim_nil_initializer", planet.ID))
	if err != nil {
		t.Fatalf("ClaimPlanet() error = %v, want nil", err)
	}
	if !result.Claimed || result.Planet.OwnerPlayerID != claimTestPlayerID {
		t.Fatalf("claim result = %+v, want claimed by %q", result, claimTestPlayerID)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls = %d, want 1", got)
	}
	if got := len(service.Events()); got != 1 {
		t.Fatalf("claim events = %d, want 1", got)
	}
}

func TestClaimPlanetSuccessInitializesProductionWithClaimContext(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	initializer := &recordingClaimProductionInitializer{}
	initializer.onCall = func(input ClaimProductionInitializeInput) {
		stored, ok, err := store.Planet(input.PlanetID)
		if err != nil || !ok {
			t.Fatalf("Planet(%q) during initialization ok = %v err = %v, want true nil", input.PlanetID, ok, err)
		}
		if stored.OwnerPlayerID != claimTestPlayerID {
			t.Fatalf("owner during initialization = %q, want %q", stored.OwnerPlayerID, claimTestPlayerID)
		}
		if stored.OwnerChangedAt == nil || !stored.OwnerChangedAt.Equal(claimTestTime()) {
			t.Fatalf("owner changed at during initialization = %v, want %s", stored.OwnerChangedAt, claimTestTime())
		}
	}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:       store,
		rank:        planet.Level,
		inRange:     true,
		consumer:    &recordingClaimXCoreConsumer{},
		initializer: initializer,
	})

	result, err := service.ClaimPlanet(claimInput("claim_init_success", planet.ID))
	if err != nil {
		t.Fatalf("ClaimPlanet() error = %v, want nil", err)
	}
	if !result.Claimed {
		t.Fatalf("ClaimPlanet() claimed = false, want true")
	}
	if got := len(initializer.calls); got != 1 {
		t.Fatalf("production initializer calls = %d, want 1", got)
	}
	call := initializer.calls[0]
	if call.PlayerID != claimTestPlayerID || call.PlanetID != planet.ID {
		t.Fatalf("initializer identity = %+v, want player %q planet %q", call, claimTestPlayerID, planet.ID)
	}
	if call.PlanetLevel != planet.Level {
		t.Fatalf("initializer planet level = %d, want %d", call.PlanetLevel, planet.Level)
	}
	if !call.ClaimedAt.Equal(claimTestTime()) {
		t.Fatalf("initializer claimed_at = %s, want %s", call.ClaimedAt, claimTestTime())
	}
	if call.ClaimReference != "claim_init_success" {
		t.Fatalf("initializer claim reference = %q, want claim_init_success", call.ClaimReference)
	}
	events := service.Events()
	if len(events) != 1 || !events[0].CreatedAt.Equal(call.ClaimedAt) {
		t.Fatalf("claim events = %+v, want one event at initializer claimed_at", events)
	}
}

func TestClaimPlanetDuplicateReferenceDoesNotConsumeEmitOrInitializeAgain(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	consumer := &recordingClaimXCoreConsumer{}
	initializer := &recordingClaimProductionInitializer{}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:       store,
		rank:        planet.Level,
		inRange:     true,
		consumer:    consumer,
		initializer: initializer,
	})
	input := claimInput("claim_duplicate", planet.ID)

	if _, err := service.ClaimPlanet(input); err != nil {
		t.Fatalf("first ClaimPlanet() error = %v, want nil", err)
	}
	duplicate, err := service.ClaimPlanet(input)
	if err != nil {
		t.Fatalf("duplicate ClaimPlanet() error = %v, want nil", err)
	}
	if !duplicate.Duplicate || !duplicate.Claimed {
		t.Fatalf("duplicate result = %+v, want Duplicate and Claimed", duplicate)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after duplicate = %d, want 1", got)
	}
	if got := len(service.Events()); got != 1 {
		t.Fatalf("claim events after duplicate = %d, want 1", got)
	}
	if got := len(initializer.calls); got != 1 {
		t.Fatalf("production initializer calls after duplicate = %d, want 1", got)
	}
}

func TestClaimPlanetProductionInitializerErrorReturnsBeforeEventOrClaimCache(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	initErr := errors.New("production init failed")
	initializer := &recordingClaimProductionInitializer{err: initErr}
	consumer := &recordingClaimXCoreConsumer{}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:       store,
		rank:        planet.Level,
		inRange:     true,
		consumer:    consumer,
		initializer: initializer,
	})
	input := claimInput("claim_init_fail", planet.ID)

	_, err := service.ClaimPlanet(input)
	if !errors.Is(err, initErr) {
		t.Fatalf("ClaimPlanet() error = %v, want initErr", err)
	}
	if got := len(initializer.calls); got != 1 {
		t.Fatalf("production initializer calls = %d, want 1", got)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events after initializer failure = %d, want 0", got)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after initializer failure = %d, want 1", got)
	}
	initializer.err = nil
	retry, err := service.ClaimPlanet(input)
	if err != nil {
		t.Fatalf("retry ClaimPlanet() error = %v, want nil already-owned repair", err)
	}
	if retry.Duplicate {
		t.Fatalf("retry duplicate = true, want false because initializer failure was not cached")
	}
	if !retry.AlreadyOwned || !retry.Claimed {
		t.Fatalf("retry result = %+v, want already-owned claimed result", retry)
	}
	if got := len(initializer.calls); got != 2 {
		t.Fatalf("production initializer calls after retry = %d, want 2", got)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after retry = %d, want 1", got)
	}
}

func TestClaimPlanetProductionInitializerErrorRetryCachesRepairWithoutDuplicateSideEffects(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	initErr := errors.New("production init failed")
	initializer := &recordingClaimProductionInitializer{err: initErr}
	consumer := &recordingClaimXCoreConsumer{}
	staleMarker := &recordingClaimListedIntelStaleMarker{markedCount: 4}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:       store,
		rank:        planet.Level,
		inRange:     true,
		consumer:    consumer,
		initializer: initializer,
		staleMarker: staleMarker,
	})
	input := claimInput("claim_init_retry_repair", planet.ID)

	_, err := service.ClaimPlanet(input)
	if !errors.Is(err, initErr) {
		t.Fatalf("ClaimPlanet() error = %v, want initErr", err)
	}
	stored, ok, err := store.Planet(planet.ID)
	if err != nil || !ok {
		t.Fatalf("Planet() ok = %v err = %v, want true nil", ok, err)
	}
	if stored.OwnerPlayerID != claimTestPlayerID {
		t.Fatalf("owner after initializer failure = %q, want %q", stored.OwnerPlayerID, claimTestPlayerID)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events after initializer failure = %d, want 0", got)
	}
	if got := len(staleMarker.calls); got != 0 {
		t.Fatalf("stale marker calls after initializer failure = %d, want 0", got)
	}

	initializer.err = nil
	repaired, err := service.ClaimPlanet(input)
	if err != nil {
		t.Fatalf("retry ClaimPlanet() error = %v, want nil repair", err)
	}
	if repaired.Duplicate {
		t.Fatalf("repair duplicate = true, want false because failed attempt was not cached")
	}
	if !repaired.Claimed || !repaired.AlreadyOwned {
		t.Fatalf("repair result = %+v, want already-owned claimed repair", repaired)
	}
	if repaired.StaleListingCount != 4 {
		t.Fatalf("repair stale listing count = %d, want 4", repaired.StaleListingCount)
	}
	if got := len(initializer.calls); got != 2 {
		t.Fatalf("production initializer calls after repair = %d, want 2", got)
	}
	if got := len(staleMarker.calls); got != 1 {
		t.Fatalf("stale marker calls after repair = %d, want 1", got)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events after repair = %d, want 0", got)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after repair = %d, want 1", got)
	}

	duplicate, err := service.ClaimPlanet(input)
	if err != nil {
		t.Fatalf("duplicate ClaimPlanet() error = %v, want nil cached result", err)
	}
	if !duplicate.Duplicate || !duplicate.Claimed || !duplicate.AlreadyOwned {
		t.Fatalf("duplicate result = %+v, want cached duplicate already-owned claim", duplicate)
	}
	if duplicate.StaleListingCount != repaired.StaleListingCount {
		t.Fatalf("duplicate stale listing count = %d, want %d", duplicate.StaleListingCount, repaired.StaleListingCount)
	}
	if got := len(initializer.calls); got != 2 {
		t.Fatalf("production initializer calls after duplicate = %d, want 2", got)
	}
	if got := len(staleMarker.calls); got != 1 {
		t.Fatalf("stale marker calls after duplicate = %d, want 1", got)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events after duplicate = %d, want 0", got)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after duplicate = %d, want 1", got)
	}
}

func TestClaimPlanetListedIntelStaleMarkerFailureCanRepairOnRetry(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	consumer := &recordingClaimXCoreConsumer{}
	markerErr := errors.New("stale marker unavailable")
	staleMarker := &recordingClaimListedIntelStaleMarker{markedCount: 2, err: markerErr}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:       store,
		rank:        planet.Level,
		inRange:     true,
		consumer:    consumer,
		staleMarker: staleMarker,
	})
	input := claimInput("claim_stale_marker_retry", planet.ID)

	_, err := service.ClaimPlanet(input)
	if !errors.Is(err, markerErr) {
		t.Fatalf("ClaimPlanet stale marker error = %v, want markerErr", err)
	}
	stored, ok, err := store.Planet(planet.ID)
	if err != nil || !ok {
		t.Fatalf("Planet() ok = %v err = %v, want true nil", ok, err)
	}
	if stored.OwnerPlayerID != claimTestPlayerID {
		t.Fatalf("owner after marker failure = %q, want %q", stored.OwnerPlayerID, claimTestPlayerID)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after failure = %d, want 1", got)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events after marker failure = %d, want 0", got)
	}

	staleMarker.err = nil
	retry, err := service.ClaimPlanet(input)
	if err != nil {
		t.Fatalf("retry ClaimPlanet() error = %v, want nil already-owned repair", err)
	}
	if !retry.Claimed || !retry.AlreadyOwned || retry.StaleListingCount != 2 {
		t.Fatalf("retry result = %+v, want already-owned claimed with 2 stale listings", retry)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after retry = %d, want still 1", got)
	}
	if got := len(staleMarker.calls); got != 2 {
		t.Fatalf("stale marker calls after retry = %d, want 2", got)
	}
}

func TestClaimPlanetAlreadyOwnedBySamePlayerInitializesWithoutConsumeOrEvent(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	changedAt := testTime(5)
	planet.OwnerPlayerID = claimTestPlayerID
	planet.OwnerChangedAt = &changedAt
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	consumer := &recordingClaimXCoreConsumer{}
	initializer := &recordingClaimProductionInitializer{}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:       store,
		rank:        planet.Level,
		inRange:     true,
		consumer:    consumer,
		initializer: initializer,
	})

	result, err := service.ClaimPlanet(claimInput("claim_owned_same", planet.ID))
	if err != nil {
		t.Fatalf("ClaimPlanet() error = %v, want nil", err)
	}
	if !result.Claimed || !result.AlreadyOwned {
		t.Fatalf("ClaimPlanet() result = %+v, want claimed already-owned", result)
	}
	if got := len(consumer.calls); got != 0 {
		t.Fatalf("x core consume calls = %d, want 0", got)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events = %d, want 0", got)
	}
	if got := len(initializer.calls); got != 1 {
		t.Fatalf("production initializer calls = %d, want 1", got)
	}
	call := initializer.calls[0]
	if call.PlayerID != claimTestPlayerID || call.PlanetID != planet.ID {
		t.Fatalf("initializer identity = %+v, want player %q planet %q", call, claimTestPlayerID, planet.ID)
	}
	if !call.ClaimedAt.Equal(changedAt) {
		t.Fatalf("initializer claimed_at = %s, want owner changed at %s", call.ClaimedAt, changedAt)
	}
}

func TestClaimPlanetAlreadyOwnedByAnotherPlayerRejected(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim")
	changedAt := testTime(5)
	planet.OwnerPlayerID = "player_other"
	planet.OwnerChangedAt = &changedAt
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	consumer := &recordingClaimXCoreConsumer{}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:    store,
		rank:     planet.Level,
		inRange:  true,
		consumer: consumer,
	})

	_, err := service.ClaimPlanet(claimInput("claim_owned_other", planet.ID))
	if !errors.Is(err, ErrPlanetAlreadyOwned) {
		t.Fatalf("ClaimPlanet() error = %v, want ErrPlanetAlreadyOwned", err)
	}
	if strings.Contains(err.Error(), "player_other") {
		t.Fatalf("ClaimPlanet() error leaked owner id: %v", err)
	}
	if got := len(consumer.calls); got != 0 {
		t.Fatalf("x core consume calls = %d, want 0", got)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events = %d, want 0", got)
	}
}

type claimTestServiceOptions struct {
	store       *InMemoryStore
	rank        int
	inRange     bool
	consumer    *recordingClaimXCoreConsumer
	initializer ClaimProductionInitializer
	staleMarker ClaimListedIntelStaleMarker
}

func newClaimTestService(t *testing.T, options claimTestServiceOptions) *ClaimService {
	t.Helper()
	service, err := NewClaimService(ClaimServiceConfig{
		Store:                  options.store,
		Clock:                  fixedClaimClock{now: claimTestTime()},
		Ranks:                  fixedClaimRankProvider{rank: options.rank},
		Proximity:              fixedClaimProximityProvider{inRange: options.inRange},
		XCoreConsumer:          options.consumer,
		ProductionInitializer:  options.initializer,
		ListedIntelStaleMarker: options.staleMarker,
		XCoreItemDefinition:    claimTestXCoreDefinition(t),
	})
	if err != nil {
		t.Fatalf("NewClaimService() error = %v, want nil", err)
	}
	return service
}

func claimInput(ref PlanetClaimReference, planetID foundation.PlanetID) ClaimPlanetInput {
	return ClaimPlanetInput{
		PlayerID:       claimTestPlayerID,
		PlanetID:       planetID,
		ClaimReference: ref,
	}
}

func claimTestPlanet(id foundation.PlanetID) Planet {
	planet := testPlanet(id, testTime(0))
	planet.Level = 2
	return planet
}

func materializeClaimTestPlanet(t *testing.T, store *InMemoryStore, planet Planet) {
	t.Helper()
	if _, err := store.MaterializePlanet(MaterializePlanetInput{
		CandidateKey: PlanetMaterializationKey("candidate-" + planet.ID.String()),
		Planet:       planet,
	}); err != nil {
		t.Fatalf("MaterializePlanet() error = %v, want nil", err)
	}
}

func upsertClaimIntel(t *testing.T, store *InMemoryStore, playerID foundation.PlayerID, planetID foundation.PlanetID, seenAt time.Time) {
	t.Helper()
	if _, _, err := store.UpsertPlayerPlanetIntel(testIntel(playerID, planetID, seenAt, IntelStateVerified, 100, "scan-"+playerID.String())); err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel(%q) error = %v, want nil", playerID, err)
	}
}

func claimTestXCoreDefinition(t *testing.T) economy.ItemDefinition {
	t.Helper()
	source, err := catalog.NewVersionedDefinitionFromStrings("x_core", "items_v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings() error = %v, want nil", err)
	}
	maxStack, err := foundation.NewQuantity(99)
	if err != nil {
		t.Fatalf("NewQuantity(maxStack) error = %v, want nil", err)
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(weight) error = %v, want nil", err)
	}
	definition, err := economy.NewItemDefinition(
		source,
		"x_core",
		"X Core",
		economy.ItemTypeStackable,
		economy.ItemRarityRare,
		maxStack,
		weight,
		[]economy.TradeFlag{economy.TradeFlagTradeable},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition() error = %v, want nil", err)
	}
	return definition
}

func claimTestTime() time.Time {
	return testTime(10)
}

type fixedClaimClock struct {
	now time.Time
}

func (clock fixedClaimClock) Now() time.Time {
	return clock.now
}

type fixedClaimRankProvider struct {
	rank int
}

func (provider fixedClaimRankProvider) PlayerClaimRank(input ClaimRankInput) (ClaimRankResult, error) {
	if err := input.Validate(); err != nil {
		return ClaimRankResult{}, err
	}
	return ClaimRankResult{Rank: provider.rank}, nil
}

type fixedClaimProximityProvider struct {
	inRange bool
}

func (provider fixedClaimProximityProvider) PlayerCanClaimPlanet(input ClaimProximityInput) (ClaimProximityResult, error) {
	if err := input.Validate(); err != nil {
		return ClaimProximityResult{}, err
	}
	return ClaimProximityResult{WithinRange: provider.inRange}, nil
}

type recordingClaimXCoreConsumer struct {
	calls []ClaimXCoreConsumeInput
	err   error
}

func (consumer *recordingClaimXCoreConsumer) ConsumeClaimXCore(input ClaimXCoreConsumeInput) (ClaimXCoreConsumeResult, error) {
	if err := input.Validate(); err != nil {
		return ClaimXCoreConsumeResult{}, err
	}
	if consumer.err != nil {
		return ClaimXCoreConsumeResult{}, consumer.err
	}
	consumer.calls = append(consumer.calls, input)
	return ClaimXCoreConsumeResult{}, nil
}

type recordingClaimProductionInitializer struct {
	calls  []ClaimProductionInitializeInput
	err    error
	onCall func(ClaimProductionInitializeInput)
}

func (initializer *recordingClaimProductionInitializer) InitializeClaimProduction(input ClaimProductionInitializeInput) (ClaimProductionInitializeResult, error) {
	if err := input.Validate(); err != nil {
		return ClaimProductionInitializeResult{}, err
	}
	initializer.calls = append(initializer.calls, input)
	if initializer.onCall != nil {
		initializer.onCall(input)
	}
	if initializer.err != nil {
		return ClaimProductionInitializeResult{}, initializer.err
	}
	return ClaimProductionInitializeResult{Created: true}, nil
}

type recordingClaimListedIntelStaleMarker struct {
	calls       []ClaimListedIntelStaleInput
	markedCount int
	err         error
}

func (marker *recordingClaimListedIntelStaleMarker) MarkClaimedPlanetListingsStale(input ClaimListedIntelStaleInput) (ClaimListedIntelStaleResult, error) {
	if err := input.Validate(); err != nil {
		return ClaimListedIntelStaleResult{}, err
	}
	marker.calls = append(marker.calls, input)
	if marker.err != nil {
		return ClaimListedIntelStaleResult{}, marker.err
	}
	return ClaimListedIntelStaleResult{MarkedCount: marker.markedCount}, nil
}
