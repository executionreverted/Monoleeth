package intel

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

const (
	testScout    foundation.PlayerID = "player-scout"
	testReceiver foundation.PlayerID = "player-receiver"
	testPlanet   foundation.PlanetID = "planet-alpha"
	testWorld    foundation.WorldID  = "world-one"
	testZone     foundation.ZoneID   = "zone-one"
)

func TestSharePlanetIntelRequiresKnownNonInvalidatedSource(t *testing.T) {
	service := NewService(fixedClock{now: testTime(10)})
	input := shareInput(t, "missing", testPlanet)

	_, err := service.SharePlanetIntel(input)
	if !errors.Is(err, ErrPlanetIntelNotKnown) {
		t.Fatalf("SharePlanetIntel missing error = %v, want ErrPlanetIntelNotKnown", err)
	}

	upsertIntel(t, service, PlayerPlanetIntel{
		PlayerID:        testScout,
		PlanetID:        testPlanet,
		WorldID:         testWorld,
		ZoneID:          testZone,
		Coordinates:     world.Vec2{X: 10, Y: 20},
		State:           IntelStateInvalidated,
		Confidence:      0,
		LastSeenAt:      testTime(1),
		SourceType:      IntelSourceScanSuccess,
		SourceReference: "scan:invalidated",
	})

	_, err = service.SharePlanetIntel(input)
	if !errors.Is(err, ErrPlanetIntelInvalidated) {
		t.Fatalf("SharePlanetIntel invalidated error = %v, want ErrPlanetIntelInvalidated", err)
	}
	if _, ok, err := service.PlayerPlanetIntel(testReceiver, testPlanet); err != nil || ok {
		t.Fatalf("receiver intel ok = %v err = %v, want false nil", ok, err)
	}
}

func TestSharePlanetIntelUpsertsReceiverFromServerSourceAndIsIdempotent(t *testing.T) {
	service := NewService(fixedClock{now: testTime(10)})
	source := upsertIntel(t, service, validIntel(testScout, testPlanet, testTime(1), "scan:source"))
	input := shareInput(t, "ok", testPlanet)

	result, err := service.SharePlanetIntel(input)
	if err != nil {
		t.Fatalf("SharePlanetIntel error = %v, want nil", err)
	}
	if !result.Shared || !result.ReceiverUpdated {
		t.Fatalf("share result = %+v, want shared receiver update", result)
	}
	if result.ReceiverIntel.PlayerID != testReceiver || result.ReceiverIntel.PlanetID != testPlanet {
		t.Fatalf("receiver identity = %+v, want receiver planet", result.ReceiverIntel)
	}
	if result.ReceiverIntel.Coordinates != source.Coordinates {
		t.Fatalf("receiver coordinates = %+v, want source %+v", result.ReceiverIntel.Coordinates, source.Coordinates)
	}
	if result.ReceiverIntel.SourceType != IntelSourceShareReceived || result.ReceiverIntel.SourceReference != input.Reference.String() {
		t.Fatalf("receiver source = %s/%s, want share_received/%s", result.ReceiverIntel.SourceType, result.ReceiverIntel.SourceReference, input.Reference)
	}

	duplicate, err := service.SharePlanetIntel(input)
	if err != nil {
		t.Fatalf("duplicate SharePlanetIntel error = %v, want nil", err)
	}
	if !duplicate.Duplicate || !duplicate.Shared || duplicate.ReceiverUpdated != result.ReceiverUpdated {
		t.Fatalf("duplicate result = %+v, want duplicate original share result %+v", duplicate, result)
	}
	if duplicate.ReceiverIntel.SourceReference != result.ReceiverIntel.SourceReference {
		t.Fatalf("duplicate receiver ref = %q, want %q", duplicate.ReceiverIntel.SourceReference, result.ReceiverIntel.SourceReference)
	}
}

func TestSharePlanetIntelConflictingReferenceFailsSafely(t *testing.T) {
	service := NewService(fixedClock{now: testTime(10)})
	upsertIntel(t, service, validIntel(testScout, testPlanet, testTime(1), "scan:source"))
	input := shareInput(t, "conflict", testPlanet)
	if _, err := service.SharePlanetIntel(input); err != nil {
		t.Fatalf("SharePlanetIntel first error = %v, want nil", err)
	}

	conflict := input
	conflict.ToPlayerID = "player-other"
	_, err := service.SharePlanetIntel(conflict)
	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("SharePlanetIntel conflict error = %v, want ErrInvalidReference", err)
	}
	if _, ok, err := service.PlayerPlanetIntel(conflict.ToPlayerID, conflict.PlanetID); err != nil || ok {
		t.Fatalf("conflict receiver intel ok = %v err = %v, want false nil", ok, err)
	}
}

func TestCreateCoordinateItemRequiresKnownIntelAndUsesStoredCoordinates(t *testing.T) {
	service := NewService(fixedClock{now: testTime(10)})
	input := createInput(t, "coordinate-item-1", testPlanet)

	_, err := service.CreateCoordinateItem(input)
	if !errors.Is(err, ErrPlanetIntelNotKnown) {
		t.Fatalf("CreateCoordinateItem missing error = %v, want ErrPlanetIntelNotKnown", err)
	}

	source := upsertIntel(t, service, validIntel(testScout, testPlanet, testTime(2), "scan:item-source"))
	result, err := service.CreateCoordinateItem(input)
	if err != nil {
		t.Fatalf("CreateCoordinateItem error = %v, want nil", err)
	}
	if !result.Created || result.Item.ItemInstanceID != input.ItemInstanceID {
		t.Fatalf("create result = %+v, want created item %q", result, input.ItemInstanceID)
	}
	if result.Item.Coordinates != source.Coordinates || result.Item.LastVerifiedAt != source.LastSeenAt {
		t.Fatalf("item payload = %+v, want source coordinates/time %+v", result.Item, source)
	}
	if result.Item.SourceIntelReference != source.SourceReference {
		t.Fatalf("item source reference = %q, want %q", result.Item.SourceIntelReference, source.SourceReference)
	}

	mutated := source
	mutated.Coordinates = world.Vec2{X: 999, Y: 999}
	mutated.LastSeenAt = testTime(20)
	mutated.SourceReference = "scan:later"
	upsertIntel(t, service, mutated)

	stored, ok, err := service.CoordinateItem(input.ItemInstanceID)
	if err != nil || !ok {
		t.Fatalf("CoordinateItem ok = %v err = %v, want true nil", ok, err)
	}
	if stored.Coordinates != result.Item.Coordinates || stored.SourceIntelReference != "scan:item-source" {
		t.Fatalf("stored item changed after source mutation = %+v, want original payload %+v", stored, result.Item)
	}
}

func TestCreateCoordinateItemIdempotencyAndConflicts(t *testing.T) {
	service := NewService(fixedClock{now: testTime(10)})
	upsertIntel(t, service, validIntel(testScout, testPlanet, testTime(1), "scan:create"))
	input := createInput(t, "coordinate-item-1", testPlanet)

	first, err := service.CreateCoordinateItem(input)
	if err != nil {
		t.Fatalf("CreateCoordinateItem first error = %v, want nil", err)
	}
	duplicate, err := service.CreateCoordinateItem(input)
	if err != nil {
		t.Fatalf("CreateCoordinateItem duplicate error = %v, want nil", err)
	}
	if !duplicate.Duplicate || duplicate.Item.ItemInstanceID != first.Item.ItemInstanceID {
		t.Fatalf("duplicate result = %+v, want duplicate same item %q", duplicate, first.Item.ItemInstanceID)
	}

	conflictRef := input
	conflictRef.ItemInstanceID = "coordinate-item-other"
	_, err = service.CreateCoordinateItem(conflictRef)
	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("CreateCoordinateItem reference conflict error = %v, want ErrInvalidReference", err)
	}

	preseeded := NewService(fixedClock{now: testTime(10)})
	upsertIntel(t, preseeded, validIntel(testScout, testPlanet, testTime(1), "scan:preseeded"))
	putCoordinateItem(t, preseeded, validCoordinateItem(t, testScout, input.ItemInstanceID, testPlanet))
	_, err = preseeded.CreateCoordinateItem(input)
	if !errors.Is(err, ErrCoordinateItemAlreadyExists) {
		t.Fatalf("CreateCoordinateItem item conflict error = %v, want ErrCoordinateItemAlreadyExists", err)
	}
}

func TestUseCoordinateItemConsumesOnceAndUpsertsIntel(t *testing.T) {
	service := NewService(fixedClock{now: testTime(10)})
	item := validCoordinateItem(t, testReceiver, "coordinate-item-1", testPlanet)
	putCoordinateItem(t, service, item)

	use := useInput(t, "ok", item.ItemInstanceID, testReceiver)
	result, err := service.UseCoordinateItem(use)
	if err != nil {
		t.Fatalf("UseCoordinateItem error = %v, want nil", err)
	}
	if !result.Used || !result.IntelUpdated || result.Item.UsedAt == nil || result.Item.UsedBy != testReceiver {
		t.Fatalf("use result = %+v, want used by receiver with intel update", result)
	}
	if result.Intel.SourceType != IntelSourceCoordinateItemUsed || result.Intel.SourceReference != use.Reference.String() {
		t.Fatalf("intel source = %s/%s, want coordinate_item_used/%s", result.Intel.SourceType, result.Intel.SourceReference, use.Reference)
	}

	duplicate, err := service.UseCoordinateItem(use)
	if err != nil {
		t.Fatalf("duplicate UseCoordinateItem error = %v, want nil", err)
	}
	if !duplicate.Duplicate || !duplicate.Used || duplicate.Item.UsedAt == nil {
		t.Fatalf("duplicate use result = %+v, want duplicate used", duplicate)
	}

	reuse := useInput(t, "second", item.ItemInstanceID, testReceiver)
	_, err = service.UseCoordinateItem(reuse)
	if !errors.Is(err, ErrCoordinateItemAlreadyUsed) {
		t.Fatalf("UseCoordinateItem reused error = %v, want ErrCoordinateItemAlreadyUsed", err)
	}
}

func TestUseCoordinateItemWrongOwnerMissingAndConflictingReferenceFailSafely(t *testing.T) {
	service := NewService(fixedClock{now: testTime(10)})
	upsertIntel(t, service, validIntel(testScout, testPlanet, testTime(1), "scan:use-fail"))
	createResult, err := service.CreateCoordinateItem(createInput(t, "coordinate-item-1", testPlanet))
	if err != nil {
		t.Fatalf("CreateCoordinateItem error = %v, want nil", err)
	}

	_, err = service.UseCoordinateItem(useInput(t, "wrong-owner", createResult.Item.ItemInstanceID, testReceiver))
	if !errors.Is(err, ErrCoordinateItemNotOwned) {
		t.Fatalf("UseCoordinateItem wrong owner error = %v, want ErrCoordinateItemNotOwned", err)
	}
	_, err = service.UseCoordinateItem(useInput(t, "missing", "coordinate-item-missing", testScout))
	if !errors.Is(err, ErrCoordinateItemNotFound) {
		t.Fatalf("UseCoordinateItem missing error = %v, want ErrCoordinateItemNotFound", err)
	}

	okUse := useInput(t, "conflict", createResult.Item.ItemInstanceID, testScout)
	if _, err := service.UseCoordinateItem(okUse); err != nil {
		t.Fatalf("UseCoordinateItem first error = %v, want nil", err)
	}
	conflict := okUse
	conflict.ItemInstanceID = "coordinate-item-other"
	_, err = service.UseCoordinateItem(conflict)
	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("UseCoordinateItem reference conflict error = %v, want ErrInvalidReference", err)
	}
}

func TestReturnedCoordinateItemDoesNotLeakUsedAtAlias(t *testing.T) {
	service := NewService(fixedClock{now: testTime(10)})
	upsertIntel(t, service, validIntel(testScout, testPlanet, testTime(1), "scan:alias"))
	createResult, err := service.CreateCoordinateItem(createInput(t, "coordinate-item-1", testPlanet))
	if err != nil {
		t.Fatalf("CreateCoordinateItem error = %v, want nil", err)
	}
	useResult, err := service.UseCoordinateItem(useInput(t, "alias", createResult.Item.ItemInstanceID, testScout))
	if err != nil {
		t.Fatalf("UseCoordinateItem error = %v, want nil", err)
	}
	*useResult.Item.UsedAt = testTime(99)

	stored, ok, err := service.CoordinateItem(createResult.Item.ItemInstanceID)
	if err != nil || !ok {
		t.Fatalf("CoordinateItem ok = %v err = %v, want true nil", ok, err)
	}
	if stored.UsedAt == nil || stored.UsedAt.Equal(testTime(99)) {
		t.Fatalf("stored UsedAt = %v, want original cloned value", stored.UsedAt)
	}
}

func validIntel(playerID foundation.PlayerID, planetID foundation.PlanetID, lastSeenAt time.Time, sourceReference string) PlayerPlanetIntel {
	return PlayerPlanetIntel{
		PlayerID:        playerID,
		PlanetID:        planetID,
		WorldID:         testWorld,
		ZoneID:          testZone,
		Coordinates:     world.Vec2{X: 12.5, Y: -40.25},
		State:           IntelStateVerified,
		Confidence:      100,
		LastSeenAt:      lastSeenAt,
		SourceType:      IntelSourceScanSuccess,
		SourceReference: sourceReference,
	}
}

func validCoordinateItem(t *testing.T, ownerID foundation.PlayerID, itemInstanceID foundation.ItemID, planetID foundation.PlanetID) CoordinateItem {
	t.Helper()
	return CoordinateItem{
		ItemInstanceID:       itemInstanceID,
		OwnerPlayerID:        ownerID,
		PlanetID:             planetID,
		WorldID:              testWorld,
		ZoneID:               testZone,
		Coordinates:          world.Vec2{X: -15, Y: 32},
		State:                IntelStateVerified,
		Confidence:           90,
		LastVerifiedAt:       testTime(3),
		CreatedAt:            testTime(2),
		CreatedBy:            testScout,
		CreateReference:      mustCoordinateItemCreateKey(t, ownerID, planetID, itemInstanceID),
		SourceIntelReference: "scan:seeded",
	}
}

func upsertIntel(t *testing.T, service *Service, intel PlayerPlanetIntel) PlayerPlanetIntel {
	t.Helper()
	stored, _, err := service.UpsertPlayerPlanetIntel(intel)
	if err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel error = %v, want nil", err)
	}
	return stored
}

func putCoordinateItem(t *testing.T, service *Service, item CoordinateItem) {
	t.Helper()
	if err := item.Validate(); err != nil {
		t.Fatalf("coordinate item fixture invalid: %v", err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	service.items[item.ItemInstanceID] = cloneCoordinateItem(item)
}

func shareInput(t *testing.T, reference string, planetID foundation.PlanetID) SharePlanetIntelInput {
	t.Helper()
	return SharePlanetIntelInput{
		FromPlayerID: testScout,
		ToPlayerID:   testReceiver,
		PlanetID:     planetID,
		Reference:    mustIntelShareKey(t, testScout, testReceiver, planetID, reference),
	}
}

func createInput(t *testing.T, itemInstanceID foundation.ItemID, planetID foundation.PlanetID) CreateCoordinateItemInput {
	t.Helper()
	return CreateCoordinateItemInput{
		PlayerID:       testScout,
		PlanetID:       planetID,
		ItemInstanceID: itemInstanceID,
		Reference:      mustCoordinateItemCreateKey(t, testScout, planetID, itemInstanceID),
	}
}

func useInput(t *testing.T, reference string, itemInstanceID foundation.ItemID, playerID foundation.PlayerID) UseCoordinateItemInput {
	t.Helper()
	return UseCoordinateItemInput{
		PlayerID:       playerID,
		ItemInstanceID: itemInstanceID,
		Reference:      mustCoordinateItemUseKey(t, playerID, itemInstanceID, reference),
	}
}

func mustIntelShareKey(
	t *testing.T,
	fromPlayerID foundation.PlayerID,
	toPlayerID foundation.PlayerID,
	planetID foundation.PlanetID,
	reference string,
) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.IntelShareIdempotencyKey(fromPlayerID, toPlayerID, planetID, reference)
	if err != nil {
		t.Fatalf("IntelShareIdempotencyKey() error = %v, want nil", err)
	}
	return key
}

func mustCoordinateItemCreateKey(
	t *testing.T,
	playerID foundation.PlayerID,
	planetID foundation.PlanetID,
	itemInstanceID foundation.ItemID,
) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.CoordinateItemCreateIdempotencyKey(playerID, planetID, itemInstanceID)
	if err != nil {
		t.Fatalf("CoordinateItemCreateIdempotencyKey() error = %v, want nil", err)
	}
	return key
}

func mustCoordinateItemUseKey(
	t *testing.T,
	playerID foundation.PlayerID,
	itemInstanceID foundation.ItemID,
	reference string,
) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.CoordinateItemUseIdempotencyKey(playerID, itemInstanceID, reference)
	if err != nil {
		t.Fatalf("CoordinateItemUseIdempotencyKey() error = %v, want nil", err)
	}
	return key
}

func testTime(offset int) time.Time {
	return time.Date(2026, 6, 23, 12, offset, 0, 0, time.UTC)
}

type fixedClock struct {
	now time.Time
}

func (clock fixedClock) Now() time.Time {
	return clock.now
}
