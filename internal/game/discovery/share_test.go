package discovery

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

const (
	shareTestFromPlayerID foundation.PlayerID = "player_scout"
	shareTestToPlayerID   foundation.PlayerID = "player_receiver"
)

func TestSharePlanetIntelWithoutSourceIntelRejectedWithoutQuotaOrReceiverMutation(t *testing.T) {
	store := NewInMemoryStore()
	quota := &recordingShareQuota{allowed: true}
	service := newShareTestService(t, store, quota)
	input := shareInput("share_no_source", "planet-share")

	_, err := service.SharePlanetIntel(input)
	if !errors.Is(err, ErrPlanetIntelShareRequiresSourceIntel) {
		t.Fatalf("SharePlanetIntel() error = %v, want ErrPlanetIntelShareRequiresSourceIntel", err)
	}
	if got := len(quota.checks); got != 0 {
		t.Fatalf("quota checks = %d, want 0", got)
	}
	if got := len(quota.consumes); got != 0 {
		t.Fatalf("quota consumes = %d, want 0", got)
	}
	if _, ok, err := store.PlayerPlanetIntel(input.ToPlayerID, input.PlanetID); err != nil || ok {
		t.Fatalf("receiver intel ok = %v err = %v, want false nil", ok, err)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("share events = %d, want 0", got)
	}
}

func TestSharePlanetIntelQuotaDenialRejectedWithoutReceiverMutation(t *testing.T) {
	store := NewInMemoryStore()
	planetID := foundation.PlanetID("planet-share")
	upsertShareIntel(t, store, shareTestFromPlayerID, planetID, testTime(1), "scan-source")
	quota := &recordingShareQuota{allowed: false}
	service := newShareTestService(t, store, quota)
	input := shareInput("share_denied", planetID)

	_, err := service.SharePlanetIntel(input)
	if !errors.Is(err, ErrPlanetIntelShareQuotaReached) {
		t.Fatalf("SharePlanetIntel() error = %v, want ErrPlanetIntelShareQuotaReached", err)
	}
	if got := len(quota.checks); got != 1 {
		t.Fatalf("quota checks = %d, want 1", got)
	}
	if got := len(quota.consumes); got != 0 {
		t.Fatalf("quota consumes = %d, want 0", got)
	}
	if _, ok, err := store.PlayerPlanetIntel(input.ToPlayerID, input.PlanetID); err != nil || ok {
		t.Fatalf("receiver intel ok = %v err = %v, want false nil", ok, err)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("share events = %d, want 0", got)
	}
}

func TestSharePlanetIntelSuccessConsumesQuotaUpsertsReceiverAndEmitsEvent(t *testing.T) {
	store := NewInMemoryStore()
	planetID := foundation.PlanetID("planet-share")
	source := upsertShareIntel(t, store, shareTestFromPlayerID, planetID, testTime(1), "scan-source")
	quota := &recordingShareQuota{allowed: true}
	service := newShareTestService(t, store, quota)
	input := shareInput("share_success", planetID)

	result, err := service.SharePlanetIntel(input)
	if err != nil {
		t.Fatalf("SharePlanetIntel() error = %v, want nil", err)
	}
	if !result.Shared || !result.ReceiverUpdated {
		t.Fatalf("share result = %+v, want shared and receiver updated", result)
	}
	if got := len(quota.checks); got != 1 {
		t.Fatalf("quota checks = %d, want 1", got)
	}
	if got := len(quota.consumes); got != 1 {
		t.Fatalf("quota consumes = %d, want 1", got)
	}
	consume := quota.consumes[0]
	if consume.FromPlayerID != input.FromPlayerID || consume.ToPlayerID != input.ToPlayerID ||
		consume.PlanetID != input.PlanetID || consume.ShareReference != input.ShareReference {
		t.Fatalf("quota consume input = %+v, want share input identity", consume)
	}

	receiver, ok, err := store.PlayerPlanetIntel(input.ToPlayerID, input.PlanetID)
	if err != nil || !ok {
		t.Fatalf("PlayerPlanetIntel(receiver) ok = %v err = %v, want true nil", ok, err)
	}
	if receiver.SourceType != IntelSourceShareReceived || receiver.SourceReference != string(input.ShareReference) {
		t.Fatalf("receiver source = %s/%s, want share_received/%s", receiver.SourceType, receiver.SourceReference, input.ShareReference)
	}
	if receiver.Coordinates != source.Coordinates || !receiver.LastSeenAt.Equal(source.LastSeenAt) {
		t.Fatalf("receiver intel = %+v, want shared source coordinates and last_seen_at %+v", receiver, source)
	}

	events := service.Events()
	if len(events) != 1 || events[0].Type != PlanetIntelShareEventShared {
		t.Fatalf("share events = %+v, want one %s", events, PlanetIntelShareEventShared)
	}
	event := events[0]
	if event.FromPlayerID != input.FromPlayerID || event.ToPlayerID != input.ToPlayerID ||
		event.PlanetID != input.PlanetID || event.ShareReference != input.ShareReference {
		t.Fatalf("share event = %+v, want share input identity", event)
	}
	if !event.CreatedAt.Equal(shareTestTime()) {
		t.Fatalf("event created_at = %s, want %s", event.CreatedAt, shareTestTime())
	}
}

func TestSharePlanetIntelPreservesFresherReceiverIntel(t *testing.T) {
	store := NewInMemoryStore()
	planetID := foundation.PlanetID("planet-share")
	upsertShareIntel(t, store, shareTestFromPlayerID, planetID, testTime(1), "scan-source-old")
	freshReceiver := testIntel(shareTestToPlayerID, planetID, testTime(10), IntelStateVerified, 100, "scan-receiver-fresh")
	if _, updated, err := store.UpsertPlayerPlanetIntel(freshReceiver); err != nil || !updated {
		t.Fatalf("UpsertPlayerPlanetIntel(fresh receiver) updated = %v err = %v, want true nil", updated, err)
	}
	quota := &recordingShareQuota{allowed: true}
	service := newShareTestService(t, store, quota)
	input := shareInput("share_preserve_fresh", planetID)

	result, err := service.SharePlanetIntel(input)
	if err != nil {
		t.Fatalf("SharePlanetIntel() error = %v, want nil", err)
	}
	if !result.Shared || result.ReceiverUpdated {
		t.Fatalf("share result = %+v, want shared without receiver update", result)
	}
	receiver, ok, err := store.PlayerPlanetIntel(input.ToPlayerID, input.PlanetID)
	if err != nil || !ok {
		t.Fatalf("PlayerPlanetIntel(receiver) ok = %v err = %v, want true nil", ok, err)
	}
	if receiver.SourceReference != freshReceiver.SourceReference || !receiver.LastSeenAt.Equal(freshReceiver.LastSeenAt) {
		t.Fatalf("receiver intel = %+v, want preserved fresh receiver %+v", receiver, freshReceiver)
	}
	if got := len(quota.consumes); got != 1 {
		t.Fatalf("quota consumes = %d, want 1", got)
	}
	if got := len(service.Events()); got != 1 {
		t.Fatalf("share events = %d, want 1", got)
	}
}

func TestSharePlanetIntelDuplicateReferenceDoesNotConsumeQuotaOrEmitAgain(t *testing.T) {
	store := NewInMemoryStore()
	planetID := foundation.PlanetID("planet-share")
	upsertShareIntel(t, store, shareTestFromPlayerID, planetID, testTime(1), "scan-source")
	quota := &recordingShareQuota{allowed: true}
	service := newShareTestService(t, store, quota)
	input := shareInput("share_duplicate", planetID)

	if _, err := service.SharePlanetIntel(input); err != nil {
		t.Fatalf("first SharePlanetIntel() error = %v, want nil", err)
	}
	duplicate, err := service.SharePlanetIntel(input)
	if err != nil {
		t.Fatalf("duplicate SharePlanetIntel() error = %v, want nil", err)
	}
	if !duplicate.Duplicate || !duplicate.Shared || duplicate.ReceiverUpdated {
		t.Fatalf("duplicate result = %+v, want duplicate shared without receiver update", duplicate)
	}
	if got := len(quota.checks); got != 1 {
		t.Fatalf("quota checks after duplicate = %d, want 1", got)
	}
	if got := len(quota.consumes); got != 1 {
		t.Fatalf("quota consumes after duplicate = %d, want 1", got)
	}
	if got := len(service.Events()); got != 1 {
		t.Fatalf("share events after duplicate = %d, want 1", got)
	}
}

func newShareTestService(t *testing.T, store *InMemoryStore, quota *recordingShareQuota) *ShareService {
	t.Helper()
	service, err := NewShareService(ShareServiceConfig{
		Store:         store,
		Clock:         fixedShareClock{now: shareTestTime()},
		QuotaProvider: quota,
		QuotaConsumer: quota,
	})
	if err != nil {
		t.Fatalf("NewShareService() error = %v, want nil", err)
	}
	return service
}

func shareInput(ref PlanetIntelShareReference, planetID foundation.PlanetID) SharePlanetIntelInput {
	return SharePlanetIntelInput{
		FromPlayerID:   shareTestFromPlayerID,
		ToPlayerID:     shareTestToPlayerID,
		PlanetID:       planetID,
		ShareReference: ref,
	}
}

func upsertShareIntel(
	t *testing.T,
	store *InMemoryStore,
	playerID foundation.PlayerID,
	planetID foundation.PlanetID,
	lastSeenAt time.Time,
	sourceReference string,
) PlayerPlanetIntel {
	t.Helper()
	intel := testIntel(playerID, planetID, lastSeenAt, IntelStateVerified, 100, sourceReference)
	if _, updated, err := store.UpsertPlayerPlanetIntel(intel); err != nil || !updated {
		t.Fatalf("UpsertPlayerPlanetIntel(%q) updated = %v err = %v, want true nil", playerID, updated, err)
	}
	return intel
}

func shareTestTime() time.Time {
	return testTime(20)
}

type fixedShareClock struct {
	now time.Time
}

func (clock fixedShareClock) Now() time.Time {
	return clock.now
}

type recordingShareQuota struct {
	allowed bool

	checks   []ShareQuotaCheckInput
	consumes []ShareQuotaConsumeInput

	checkErr   error
	consumeErr error
}

func (quota *recordingShareQuota) ShareQuotaAvailable(input ShareQuotaCheckInput) (ShareQuotaCheckResult, error) {
	if err := input.Validate(); err != nil {
		return ShareQuotaCheckResult{}, err
	}
	quota.checks = append(quota.checks, input)
	if quota.checkErr != nil {
		return ShareQuotaCheckResult{}, quota.checkErr
	}
	return ShareQuotaCheckResult{Allowed: quota.allowed}, nil
}

func (quota *recordingShareQuota) ConsumeShareQuota(input ShareQuotaConsumeInput) (ShareQuotaConsumeResult, error) {
	if err := input.Validate(); err != nil {
		return ShareQuotaConsumeResult{}, err
	}
	quota.consumes = append(quota.consumes, input)
	if quota.consumeErr != nil {
		return ShareQuotaConsumeResult{}, quota.consumeErr
	}
	return ShareQuotaConsumeResult{Consumed: true}, nil
}
