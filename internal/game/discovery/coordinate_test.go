package discovery

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

func TestCreateCoordinateScrollWithoutSourceIntelRejected(t *testing.T) {
	store := NewInMemoryStore()
	planet := coordinateTestPlanet("planet-scroll")
	materializeCoordinateTestPlanet(t, store, planet)
	creator := &recordingCoordinateScrollItemCreator{}
	service := newCoordinateTestService(t, coordinateTestServiceOptions{
		store:   store,
		creator: creator,
	})

	_, err := service.CreateCoordinateScroll(CreateCoordinateScrollInput{
		PlayerID:        "player-scout",
		PlanetID:        planet.ID,
		CreateReference: "create-no-intel",
	})
	if !errors.Is(err, ErrCoordinateScrollRequiresIntel) {
		t.Fatalf("CreateCoordinateScroll() error = %v, want ErrCoordinateScrollRequiresIntel", err)
	}
	if got := len(creator.calls); got != 0 {
		t.Fatalf("creator calls = %d, want 0", got)
	}
}

func TestCreateCoordinateScrollRejectedWhenSourceIntelInvalidated(t *testing.T) {
	store := NewInMemoryStore()
	planet := coordinateTestPlanet("planet-scroll")
	materializeCoordinateTestPlanet(t, store, planet)
	sourceIntel := testIntel("player-scout", planet.ID, testTime(1), IntelStateInvalidated, 0, "scan-invalidated")
	upsertCoordinateTestIntel(t, store, sourceIntel)
	creator := &recordingCoordinateScrollItemCreator{}
	service := newCoordinateTestService(t, coordinateTestServiceOptions{
		store:   store,
		creator: creator,
	})

	_, err := service.CreateCoordinateScroll(CreateCoordinateScrollInput{
		PlayerID:        sourceIntel.PlayerID,
		PlanetID:        planet.ID,
		CreateReference: "create-invalidated",
	})
	if !errors.Is(err, ErrCoordinateScrollIntelInvalidated) {
		t.Fatalf("CreateCoordinateScroll() error = %v, want ErrCoordinateScrollIntelInvalidated", err)
	}
	if got := len(creator.calls); got != 0 {
		t.Fatalf("creator calls = %d, want 0", got)
	}
}

func TestCreateCoordinateScrollStoresServerAuthoredMetadataFromKnownIntel(t *testing.T) {
	store := NewInMemoryStore()
	planet := coordinateTestPlanet("planet-scroll")
	planet.Level = 4
	planet.Type = PlanetTypeIce
	materializeCoordinateTestPlanet(t, store, planet)
	sourceIntel := testIntel("player-scout", planet.ID, testTime(3), IntelStateVerified, 95, "scan-real")
	sourceIntel.Coordinates = world.Vec2{X: 77, Y: -88}
	upsertCoordinateTestIntel(t, store, sourceIntel)
	creator := &recordingCoordinateScrollItemCreator{}
	service := newCoordinateTestService(t, coordinateTestServiceOptions{
		store:   store,
		creator: creator,
	})

	result, err := service.CreateCoordinateScroll(CreateCoordinateScrollInput{
		PlayerID:        sourceIntel.PlayerID,
		PlanetID:        planet.ID,
		CreateReference: "create-metadata",
	})
	if err != nil {
		t.Fatalf("CreateCoordinateScroll() error = %v, want nil", err)
	}
	if !result.Created || result.ScrollItemInstanceID.IsZero() {
		t.Fatalf("create result = %+v, want created with item instance id", result)
	}
	if got := len(creator.calls); got != 1 {
		t.Fatalf("creator calls = %d, want 1", got)
	}
	createCall := creator.calls[0]
	if createCall.PlayerID != sourceIntel.PlayerID || createCall.Quantity != defaultCoordinateScrollQuantity {
		t.Fatalf("create item call = %+v, want player %q quantity %d", createCall, sourceIntel.PlayerID, defaultCoordinateScrollQuantity)
	}
	if createCall.ItemDefinition.ItemID != foundation.ItemID("planet_coordinate_scroll") {
		t.Fatalf("created item id = %q, want planet_coordinate_scroll", createCall.ItemDefinition.ItemID)
	}
	if createCall.TargetLocation.Kind != economy.LocationKindAccountInventory || createCall.TargetLocation.ID.String() != sourceIntel.PlayerID.String() {
		t.Fatalf("target location = %+v, want source player account inventory", createCall.TargetLocation)
	}
	if createCall.Reason != defaultCoordinateScrollCreateReason || createCall.Reference != "create-metadata" {
		t.Fatalf("create reason/reference = %q/%q, want %q/create-metadata", createCall.Reason, createCall.Reference, defaultCoordinateScrollCreateReason)
	}

	metadata := result.Metadata
	if metadata.PlanetID != sourceIntel.PlanetID || metadata.Coordinates != sourceIntel.Coordinates {
		t.Fatalf("metadata planet/coordinates = %q/%+v, want source intel %q/%+v", metadata.PlanetID, metadata.Coordinates, sourceIntel.PlanetID, sourceIntel.Coordinates)
	}
	if metadata.PlanetLevelKnown != planet.Level || metadata.PlanetTypeKnown != planet.Type {
		t.Fatalf("metadata planet facts = level %d type %q, want level %d type %q", metadata.PlanetLevelKnown, metadata.PlanetTypeKnown, planet.Level, planet.Type)
	}
	if metadata.State != sourceIntel.State || metadata.Confidence != sourceIntel.Confidence || !metadata.LastVerifiedAt.Equal(sourceIntel.LastSeenAt) {
		t.Fatalf("metadata intel = %+v, want state/confidence/last seen from %+v", metadata, sourceIntel)
	}
	if metadata.CreatedBy != sourceIntel.PlayerID || !metadata.CreatedAt.Equal(coordinateTestNow()) {
		t.Fatalf("metadata create fields = %+v, want creator %q at %s", metadata, sourceIntel.PlayerID, coordinateTestNow())
	}

	stored, ok, err := service.coordinateScrollMetadata(result.ScrollItemInstanceID)
	if err != nil || !ok {
		t.Fatalf("coordinateScrollMetadata() ok = %v err = %v, want true nil", ok, err)
	}
	if stored != metadata {
		t.Fatalf("stored metadata = %+v, want result metadata %+v", stored, metadata)
	}
}

func TestCreateCoordinateScrollDuplicateReferenceDoesNotMintTwice(t *testing.T) {
	store := NewInMemoryStore()
	planet := coordinateTestPlanet("planet-scroll")
	materializeCoordinateTestPlanet(t, store, planet)
	sourceIntel := testIntel("player-scout", planet.ID, testTime(1), IntelStateVerified, 100, "scan-real")
	upsertCoordinateTestIntel(t, store, sourceIntel)
	creator := &recordingCoordinateScrollItemCreator{}
	service := newCoordinateTestService(t, coordinateTestServiceOptions{
		store:   store,
		creator: creator,
	})
	input := CreateCoordinateScrollInput{
		PlayerID:        sourceIntel.PlayerID,
		PlanetID:        planet.ID,
		CreateReference: "create-duplicate",
	}

	first, err := service.CreateCoordinateScroll(input)
	if err != nil {
		t.Fatalf("first CreateCoordinateScroll() error = %v, want nil", err)
	}
	duplicate, err := service.CreateCoordinateScroll(input)
	if err != nil {
		t.Fatalf("duplicate CreateCoordinateScroll() error = %v, want nil", err)
	}
	if !duplicate.Duplicate || duplicate.ScrollItemInstanceID != first.ScrollItemInstanceID {
		t.Fatalf("duplicate result = %+v, want duplicate with same item %q", duplicate, first.ScrollItemInstanceID)
	}
	if got := len(creator.calls); got != 1 {
		t.Fatalf("creator calls after duplicate = %d, want 1", got)
	}
}

func TestUseCoordinateScrollIgnoresMutatedMetadataCopy(t *testing.T) {
	store := NewInMemoryStore()
	planet := coordinateTestPlanet("planet-scroll")
	materializeCoordinateTestPlanet(t, store, planet)
	sourceIntel := testIntel("player-scout", planet.ID, testTime(2), IntelStateVerified, 100, "scan-real")
	sourceIntel.Coordinates = world.Vec2{X: 321, Y: -654}
	upsertCoordinateTestIntel(t, store, sourceIntel)
	service := newCoordinateTestService(t, coordinateTestServiceOptions{
		store: store,
	})
	createResult := createCoordinateTestScroll(t, service, sourceIntel.PlayerID, planet.ID, "create-copy-immutable")

	metadataCopy, ok, err := service.coordinateScrollMetadata(createResult.ScrollItemInstanceID)
	if err != nil || !ok {
		t.Fatalf("coordinateScrollMetadata() ok = %v err = %v, want true nil", ok, err)
	}
	metadataCopy.PlanetID = "planet-forged"
	metadataCopy.Coordinates = world.Vec2{X: 9999, Y: 9999}
	metadataCopy.Confidence = 1

	result, err := service.UseCoordinateScroll(UseCoordinateScrollInput{
		PlayerID:             "player-receiver",
		ScrollItemInstanceID: createResult.ScrollItemInstanceID,
		UseReference:         "use-copy-immutable",
	})
	if err != nil {
		t.Fatalf("UseCoordinateScroll() error = %v, want nil", err)
	}
	if result.Intel.PlanetID != sourceIntel.PlanetID || result.Intel.Coordinates != sourceIntel.Coordinates || result.Intel.Confidence != sourceIntel.Confidence {
		t.Fatalf("used intel = %+v, want original server metadata from source intel %+v", result.Intel, sourceIntel)
	}
}

func TestUseCoordinateScrollConsumesOnceAndWritesIntel(t *testing.T) {
	store := NewInMemoryStore()
	planet := coordinateTestPlanet("planet-scroll")
	materializeCoordinateTestPlanet(t, store, planet)
	sourceIntel := testIntel("player-scout", planet.ID, testTime(2), IntelStateVerified, 90, "scan-real")
	upsertCoordinateTestIntel(t, store, sourceIntel)
	consumer := &recordingCoordinateScrollConsumer{}
	service := newCoordinateTestService(t, coordinateTestServiceOptions{
		store:    store,
		consumer: consumer,
	})
	createResult := createCoordinateTestScroll(t, service, sourceIntel.PlayerID, planet.ID, "create-use-success")

	result, err := service.UseCoordinateScroll(UseCoordinateScrollInput{
		PlayerID:             "player-receiver",
		ScrollItemInstanceID: createResult.ScrollItemInstanceID,
		UseReference:         "use-success",
	})
	if err != nil {
		t.Fatalf("UseCoordinateScroll() error = %v, want nil", err)
	}
	if !result.Used || !result.IntelUpdated {
		t.Fatalf("use result = %+v, want used and intel updated", result)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("consumer calls = %d, want 1", got)
	}
	consumeCall := consumer.calls[0]
	if consumeCall.PlayerID != "player-receiver" || consumeCall.ScrollItemInstanceID != createResult.ScrollItemInstanceID {
		t.Fatalf("consume identity = %+v, want receiver and scroll item", consumeCall)
	}
	if consumeCall.ItemRef.Definition.ItemID != foundation.ItemID("planet_coordinate_scroll") ||
		consumeCall.ItemRef.ItemInstanceID != createResult.ScrollItemInstanceID {
		t.Fatalf("consume item ref = %+v, want coordinate scroll instance", consumeCall.ItemRef)
	}
	if consumeCall.SourceLocation.Kind != economy.LocationKindAccountInventory || consumeCall.SourceLocation.ID.String() != "player-receiver" {
		t.Fatalf("consume source location = %+v, want receiver account inventory", consumeCall.SourceLocation)
	}
	if consumeCall.Quantity != defaultCoordinateScrollQuantity || consumeCall.Reason != defaultCoordinateScrollUseReason || consumeCall.Reference != "use-success" {
		t.Fatalf("consume quantity/reason/reference = %d/%q/%q, want %d/%q/use-success", consumeCall.Quantity, consumeCall.Reason, consumeCall.Reference, defaultCoordinateScrollQuantity, defaultCoordinateScrollUseReason)
	}

	storedIntel, ok, err := store.PlayerPlanetIntel("player-receiver", planet.ID)
	if err != nil || !ok {
		t.Fatalf("PlayerPlanetIntel(receiver) ok = %v err = %v, want true nil", ok, err)
	}
	if storedIntel.SourceType != IntelSourceCoordinateScrollUsed || storedIntel.SourceReference != "use-success" {
		t.Fatalf("stored intel source = %s/%s, want coordinate_scroll_used/use-success", storedIntel.SourceType, storedIntel.SourceReference)
	}
	if storedIntel.Coordinates != sourceIntel.Coordinates || storedIntel.State != sourceIntel.State || storedIntel.Confidence != sourceIntel.Confidence {
		t.Fatalf("stored intel = %+v, want source scroll intel %+v", storedIntel, sourceIntel)
	}
	storedMetadata, ok, err := service.coordinateScrollMetadata(createResult.ScrollItemInstanceID)
	if err != nil || !ok {
		t.Fatalf("coordinateScrollMetadata() ok = %v err = %v, want true nil", ok, err)
	}
	if storedMetadata.UsedAt == nil || !storedMetadata.UsedAt.Equal(coordinateTestNow()) || storedMetadata.UsedBy != "player-receiver" || storedMetadata.UseReference != "use-success" {
		t.Fatalf("stored metadata used fields = %+v, want receiver use at %s", storedMetadata, coordinateTestNow())
	}
}

func TestUseCoordinateScrollDuplicateReferenceDoesNotConsumeTwice(t *testing.T) {
	store := NewInMemoryStore()
	planet := coordinateTestPlanet("planet-scroll")
	materializeCoordinateTestPlanet(t, store, planet)
	sourceIntel := testIntel("player-scout", planet.ID, testTime(2), IntelStateVerified, 100, "scan-real")
	upsertCoordinateTestIntel(t, store, sourceIntel)
	consumer := &recordingCoordinateScrollConsumer{}
	service := newCoordinateTestService(t, coordinateTestServiceOptions{
		store:    store,
		consumer: consumer,
	})
	createResult := createCoordinateTestScroll(t, service, sourceIntel.PlayerID, planet.ID, "create-use-duplicate")
	input := UseCoordinateScrollInput{
		PlayerID:             "player-receiver",
		ScrollItemInstanceID: createResult.ScrollItemInstanceID,
		UseReference:         "use-duplicate",
	}

	if _, err := service.UseCoordinateScroll(input); err != nil {
		t.Fatalf("first UseCoordinateScroll() error = %v, want nil", err)
	}
	duplicate, err := service.UseCoordinateScroll(input)
	if err != nil {
		t.Fatalf("duplicate UseCoordinateScroll() error = %v, want nil", err)
	}
	if !duplicate.Duplicate || !duplicate.Used {
		t.Fatalf("duplicate result = %+v, want Duplicate and Used", duplicate)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("consumer calls after duplicate = %d, want 1", got)
	}
}

func TestUseCoordinateScrollPreservesFresherExistingIntel(t *testing.T) {
	store := NewInMemoryStore()
	planet := coordinateTestPlanet("planet-scroll")
	materializeCoordinateTestPlanet(t, store, planet)
	sourceIntel := testIntel("player-scout", planet.ID, testTime(1), IntelStateStale, 40, "scan-old")
	upsertCoordinateTestIntel(t, store, sourceIntel)
	fresher := testIntel("player-receiver", planet.ID, testTime(10), IntelStateVerified, 100, "scan-fresher")
	upsertCoordinateTestIntel(t, store, fresher)
	consumer := &recordingCoordinateScrollConsumer{}
	service := newCoordinateTestService(t, coordinateTestServiceOptions{
		store:    store,
		consumer: consumer,
	})
	createResult := createCoordinateTestScroll(t, service, sourceIntel.PlayerID, planet.ID, "create-stale-scroll")

	result, err := service.UseCoordinateScroll(UseCoordinateScrollInput{
		PlayerID:             "player-receiver",
		ScrollItemInstanceID: createResult.ScrollItemInstanceID,
		UseReference:         "use-stale-scroll",
	})
	if err != nil {
		t.Fatalf("UseCoordinateScroll() error = %v, want nil", err)
	}
	if !result.Used || result.IntelUpdated {
		t.Fatalf("use result = %+v, want used without intel update", result)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("consumer calls = %d, want 1", got)
	}
	stored, ok, err := store.PlayerPlanetIntel("player-receiver", planet.ID)
	if err != nil || !ok {
		t.Fatalf("PlayerPlanetIntel(receiver) ok = %v err = %v, want true nil", ok, err)
	}
	if stored.SourceReference != fresher.SourceReference || stored.State != fresher.State || !stored.LastSeenAt.Equal(fresher.LastSeenAt) {
		t.Fatalf("stored intel = %+v, want preserved fresher %+v", stored, fresher)
	}
}

type coordinateTestServiceOptions struct {
	store    *InMemoryStore
	creator  *recordingCoordinateScrollItemCreator
	consumer *recordingCoordinateScrollConsumer
}

func newCoordinateTestService(t *testing.T, options coordinateTestServiceOptions) *CoordinateScrollService {
	t.Helper()
	if options.store == nil {
		options.store = NewInMemoryStore()
	}
	if options.creator == nil {
		options.creator = &recordingCoordinateScrollItemCreator{}
	}
	if options.consumer == nil {
		options.consumer = &recordingCoordinateScrollConsumer{}
	}
	service, err := NewCoordinateScrollService(CoordinateScrollServiceConfig{
		Store:                          options.store,
		Clock:                          fixedCoordinateClock{now: coordinateTestNow()},
		ItemCreator:                    options.creator,
		ItemConsumer:                   options.consumer,
		CoordinateScrollItemDefinition: coordinateScrollDefinition(t),
	})
	if err != nil {
		t.Fatalf("NewCoordinateScrollService() error = %v, want nil", err)
	}
	return service
}

func createCoordinateTestScroll(
	t *testing.T,
	service *CoordinateScrollService,
	playerID foundation.PlayerID,
	planetID foundation.PlanetID,
	reference CoordinateScrollReference,
) CreateCoordinateScrollResult {
	t.Helper()
	result, err := service.CreateCoordinateScroll(CreateCoordinateScrollInput{
		PlayerID:        playerID,
		PlanetID:        planetID,
		CreateReference: reference,
	})
	if err != nil {
		t.Fatalf("CreateCoordinateScroll(%q) error = %v, want nil", reference, err)
	}
	return result
}

func materializeCoordinateTestPlanet(t *testing.T, store *InMemoryStore, planet Planet) {
	t.Helper()
	if _, err := store.MaterializePlanet(MaterializePlanetInput{
		CandidateKey: PlanetMaterializationKey("candidate-" + planet.ID.String()),
		Planet:       planet,
	}); err != nil {
		t.Fatalf("MaterializePlanet() error = %v, want nil", err)
	}
}

func upsertCoordinateTestIntel(t *testing.T, store *InMemoryStore, intel PlayerPlanetIntel) {
	t.Helper()
	if _, _, err := store.UpsertPlayerPlanetIntel(intel); err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel() error = %v, want nil", err)
	}
}

func coordinateTestPlanet(id foundation.PlanetID) Planet {
	return testPlanet(id, testTime(0))
}

func coordinateScrollDefinition(t *testing.T) economy.ItemDefinition {
	t.Helper()
	source, err := catalog.NewVersionedDefinitionFromStrings("planet_coordinate_scroll", "items_v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings() error = %v, want nil", err)
	}
	maxStack, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(maxStack) error = %v, want nil", err)
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(weight) error = %v, want nil", err)
	}
	definition, err := economy.NewItemDefinition(
		source,
		"planet_coordinate_scroll",
		"Planet Coordinate Scroll",
		economy.ItemTypeInstance,
		economy.ItemRarityRare,
		maxStack,
		weight,
		[]economy.TradeFlag{economy.TradeFlagTradeable, economy.TradeFlagMarketTradeable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition() error = %v, want nil", err)
	}
	return definition
}

func coordinateTestNow() time.Time {
	return testTime(30)
}

type fixedCoordinateClock struct {
	now time.Time
}

func (clock fixedCoordinateClock) Now() time.Time {
	return clock.now
}

type recordingCoordinateScrollItemCreator struct {
	calls          []CoordinateScrollItemCreateInput
	err            error
	nextSequence   int
	itemInstanceID foundation.ItemID
}

func (creator *recordingCoordinateScrollItemCreator) CreateCoordinateScrollItem(input CoordinateScrollItemCreateInput) (CoordinateScrollItemCreateResult, error) {
	if err := input.Validate(); err != nil {
		return CoordinateScrollItemCreateResult{}, err
	}
	if creator.err != nil {
		return CoordinateScrollItemCreateResult{}, creator.err
	}
	creator.calls = append(creator.calls, input)
	if !creator.itemInstanceID.IsZero() {
		return CoordinateScrollItemCreateResult{ItemInstanceID: creator.itemInstanceID}, nil
	}
	creator.nextSequence++
	return CoordinateScrollItemCreateResult{
		ItemInstanceID: foundation.ItemID(fmt.Sprintf("planet_coordinate_scroll-instance-%d", creator.nextSequence)),
	}, nil
}

type recordingCoordinateScrollConsumer struct {
	calls []CoordinateScrollConsumeInput
	err   error
}

func (consumer *recordingCoordinateScrollConsumer) ConsumeCoordinateScroll(input CoordinateScrollConsumeInput) (CoordinateScrollConsumeResult, error) {
	if err := input.Validate(); err != nil {
		return CoordinateScrollConsumeResult{}, err
	}
	if consumer.err != nil {
		return CoordinateScrollConsumeResult{}, consumer.err
	}
	consumer.calls = append(consumer.calls, input)
	return CoordinateScrollConsumeResult{}, nil
}
