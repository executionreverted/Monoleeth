package production

import (
	"encoding/json"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

func TestSettlePlanetProductionOneHourOutput(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)

	result, err := store.SettlePlanetProduction("planet-1", testTime(0).Add(time.Hour))
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	if result.ElapsedRequested != time.Hour || result.ElapsedApplied != time.Hour {
		t.Fatalf("elapsed requested/applied = %s/%s, want 1h/1h", result.ElapsedRequested, result.ElapsedApplied)
	}
	assertSettlementDelta(t, result.ProducedItems, "iron_ore", 30)
	if got := result.After.Storage.QuantityOf("iron_ore"); got != 30 {
		t.Fatalf("after iron_ore = %d, want 30", got)
	}
	if got := result.After.State.LastCalculatedAt; !got.Equal(testTime(0).Add(time.Hour)) {
		t.Fatalf("LastCalculatedAt = %s, want %s", got, testTime(0).Add(time.Hour))
	}
}

func TestSettlePlanetProductionEmitsSettlementEventsOnce(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	now := testTime(0).Add(time.Hour)

	result, err := store.SettlePlanetProduction("planet-1", now)
	if err != nil {
		t.Fatalf("SettlePlanetProduction(first) error = %v, want nil", err)
	}
	assertSettlementDelta(t, result.ProducedItems, "iron_ore", 30)
	wantWindow := wantSettlementWindow(testTime(0), now)
	wantReference := mustOfflineSettlementKey(t, "planet-1", wantWindow)
	if result.SettlementWindow != wantWindow || result.ReferenceKey != wantReference {
		t.Fatalf("settlement evidence = %q/%q, want %q/%q", result.SettlementWindow, result.ReferenceKey, wantWindow, wantReference)
	}
	assertProductionEventTypes(t, store.Events(),
		EventPlanetBuildingProduced,
		EventPlanetProductionSettled,
		EventOfflineSettlementCompleted,
	)
	assertProductionSettlementEventPayloadEvidence(t, store.Events()[1].Payload, wantReference, wantWindow)
	assertProductionSettlementEventPayloadEvidence(t, store.Events()[2].Payload, wantReference, wantWindow)
	firstEventCount := len(store.Events())

	duplicate, err := store.SettlePlanetProduction("planet-1", now)
	if err != nil {
		t.Fatalf("SettlePlanetProduction(second) error = %v, want nil", err)
	}
	if !duplicate.NoOp {
		t.Fatal("duplicate NoOp = false, want true")
	}
	if duplicate.ReferenceKey != "" || duplicate.SettlementWindow != "" {
		t.Fatalf("duplicate evidence = %q/%q, want empty", duplicate.ReferenceKey, duplicate.SettlementWindow)
	}
	if got := len(store.Events()); got != firstEventCount {
		t.Fatalf("event count after duplicate settlement = %d, want unchanged %d", got, firstEventCount)
	}
}

func assertProductionSettlementEventPayloadEvidence(t *testing.T, eventPayload json.RawMessage, reference foundation.IdempotencyKey, window string) {
	t.Helper()
	var payload ProductionSettlementPayload
	if err := json.Unmarshal(eventPayload, &payload); err != nil {
		t.Fatalf("json.Unmarshal(production settlement payload) error = %v, want nil", err)
	}
	if payload.ReferenceKey != reference || payload.SettlementWindow != window {
		t.Fatalf("event evidence = %q/%q, want %q/%q", payload.ReferenceKey, payload.SettlementWindow, reference, window)
	}
}

func TestSettlePlanetProductionEmitsStorageAndEnergyEvents(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 10, 4)
	replaceSettlementStorage(t, store, "planet-1", 10, []StoredItem{{ItemID: "void_salt", Quantity: 5}}, testTime(0))
	addSettlementBuilding(t, store, "planet-1", "building-a", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	addSettlementBuilding(t, store, "planet-1", "building-b", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)

	result, err := store.SettlePlanetProduction("planet-1", testTime(0).Add(time.Hour))
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	if !result.StorageFull || !result.EnergyInsufficient {
		t.Fatalf("StorageFull/EnergyInsufficient = %v/%v, want true/true", result.StorageFull, result.EnergyInsufficient)
	}
	assertProductionEventTypes(t, store.Events(),
		EventPlanetBuildingProduced,
		EventPlanetStorageFull,
		EventPlanetEnergyInsufficient,
		EventPlanetProductionSettled,
		EventOfflineSettlementCompleted,
	)
}

func TestSettlePlanetProductionStorageCapClampsOutput(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 10, 10)
	replaceSettlementStorage(t, store, "planet-1", 10, []StoredItem{{ItemID: "void_salt", Quantity: 5}}, testTime(0))
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)

	result, err := store.SettlePlanetProduction("planet-1", testTime(0).Add(time.Hour))
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	if !result.StorageFull || !result.BuildingResults[0].StorageFull {
		t.Fatalf("storage full flags = result %v building %v, want true true", result.StorageFull, result.BuildingResults[0].StorageFull)
	}
	assertSettlementDelta(t, result.ProducedItems, "iron_ore", 5)
	if got := result.After.Storage.UsedUnits(); got != 10 {
		t.Fatalf("after used units = %d, want 10", got)
	}
}

func TestSettlePlanetProductionInputShortageReducesRefineryOutputAndConsumption(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	replaceSettlementStorage(t, store, "planet-1", 100, []StoredItem{{ItemID: "iron_ore", Quantity: 15}}, testTime(0))
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDAlloyFoundryL1, BuildingStateActive)

	result, err := store.SettlePlanetProduction("planet-1", testTime(0).Add(time.Hour))
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	if !result.BuildingResults[0].InputShortage {
		t.Fatal("building InputShortage = false, want true")
	}
	assertSettlementDelta(t, result.ProducedItems, "refined_alloy", 5)
	assertSettlementDelta(t, result.ConsumedInputs, "iron_ore", 15)
	if got := result.After.Storage.QuantityOf("iron_ore"); got != 0 {
		t.Fatalf("after iron_ore = %d, want 0", got)
	}
	if got := result.After.Storage.QuantityOf("refined_alloy"); got != 5 {
		t.Fatalf("after refined_alloy = %d, want 5", got)
	}
}

func TestSettlePlanetProductionOfflineCapApplies(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 10_000, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	now := testTime(0).Add(100 * time.Hour)

	result, err := store.SettlePlanetProduction("planet-1", now)
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	if result.ElapsedRequested != 100*time.Hour {
		t.Fatalf("ElapsedRequested = %s, want 100h", result.ElapsedRequested)
	}
	if result.ElapsedApplied != DefaultMaxOfflineSettlementDuration {
		t.Fatalf("ElapsedApplied = %s, want %s", result.ElapsedApplied, DefaultMaxOfflineSettlementDuration)
	}
	wantWindow := wantSettlementWindow(testTime(0), testTime(0).Add(DefaultMaxOfflineSettlementDuration))
	wantReference := mustOfflineSettlementKey(t, "planet-1", wantWindow)
	if result.SettlementWindow != wantWindow || result.ReferenceKey != wantReference {
		t.Fatalf("capped settlement evidence = %q/%q, want %q/%q", result.SettlementWindow, result.ReferenceKey, wantWindow, wantReference)
	}
	assertSettlementDelta(t, result.ProducedItems, "iron_ore", 2160)
	if got := result.After.State.LastCalculatedAt; !got.Equal(now) {
		t.Fatalf("LastCalculatedAt = %s, want %s", got, now)
	}
}

func TestSettlePlanetProductionDoubleSettlementDoesNotDuplicateOutput(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	now := testTime(0).Add(time.Hour)

	first, err := store.SettlePlanetProduction("planet-1", now)
	if err != nil {
		t.Fatalf("SettlePlanetProduction(first) error = %v, want nil", err)
	}
	second, err := store.SettlePlanetProduction("planet-1", now)
	if err != nil {
		t.Fatalf("SettlePlanetProduction(second) error = %v, want nil", err)
	}

	assertSettlementDelta(t, first.ProducedItems, "iron_ore", 30)
	if !second.NoOp || second.ElapsedApplied != 0 {
		t.Fatalf("second NoOp/applied = %v/%s, want true/0", second.NoOp, second.ElapsedApplied)
	}
	if len(second.ProducedItems) != 0 {
		t.Fatalf("second ProducedItems = %+v, want empty", second.ProducedItems)
	}
	if got := second.After.Storage.QuantityOf("iron_ore"); got != 30 {
		t.Fatalf("after second iron_ore = %d, want 30", got)
	}
}

func TestSettlePlanetProductionFutureTimestampSafe(t *testing.T) {
	futureLast := testTime(0).Add(time.Hour)
	store := newSettlementStore(t, "planet-1", futureLast, 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)

	result, err := store.SettlePlanetProduction("planet-1", testTime(0))
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	if !result.NoOp || result.ElapsedApplied != 0 {
		t.Fatalf("NoOp/applied = %v/%s, want true/0", result.NoOp, result.ElapsedApplied)
	}
	if got := result.After.State.LastCalculatedAt; !got.Equal(futureLast) {
		t.Fatalf("LastCalculatedAt = %s, want unchanged %s", got, futureLast)
	}
	if got := result.After.Storage.QuantityOf("iron_ore"); got != 0 {
		t.Fatalf("after iron_ore = %d, want 0", got)
	}
}

func TestSettlePlanetProductionEnergyInsufficientSkipsAffectedBuilding(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 4)
	addSettlementBuilding(t, store, "planet-1", "building-a", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	addSettlementBuilding(t, store, "planet-1", "building-b", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)

	result, err := store.SettlePlanetProduction("planet-1", testTime(0).Add(time.Hour))
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	if !result.EnergyInsufficient {
		t.Fatal("EnergyInsufficient = false, want true")
	}
	if len(result.SkippedBuildings) != 1 || result.SkippedBuildings[0].BuildingID != "building-b" {
		t.Fatalf("SkippedBuildings = %+v, want building-b", result.SkippedBuildings)
	}
	if result.SkippedBuildings[0].Reason != SettlementSkipReasonEnergyInsufficient {
		t.Fatalf("skip reason = %q, want energy_insufficient", result.SkippedBuildings[0].Reason)
	}
	assertSettlementDelta(t, result.ProducedItems, "iron_ore", 30)
	if got := result.After.Storage.QuantityOf("iron_ore"); got != 30 {
		t.Fatalf("after iron_ore = %d, want 30", got)
	}
}

func TestSettlePlanetProductionDisabledPlanetAdvancesTimestampWithoutOutput(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	state, ok, err := store.ProductionState("planet-1")
	if err != nil || !ok {
		t.Fatalf("ProductionState() ok = %v err = %v, want true nil", ok, err)
	}
	state.ProductionEnabled = false
	state.UpdatedAt = testTime(0)
	if err := store.SaveProductionState(state); err != nil {
		t.Fatalf("SaveProductionState() error = %v, want nil", err)
	}
	now := testTime(0).Add(time.Hour)

	result, err := store.SettlePlanetProduction("planet-1", now)
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	if result.ProductionEnabled {
		t.Fatal("ProductionEnabled = true, want false")
	}
	if result.NoOp || len(result.ProducedItems) != 0 || len(result.BuildingResults) != 0 {
		t.Fatalf("result NoOp/produced/buildings = %v/%+v/%+v, want elapsed no-output settlement", result.NoOp, result.ProducedItems, result.BuildingResults)
	}
	if got := result.After.State.LastCalculatedAt; !got.Equal(now) {
		t.Fatalf("LastCalculatedAt = %s, want %s", got, now)
	}
	if got := result.After.Storage.QuantityOf("iron_ore"); got != 0 {
		t.Fatalf("after iron_ore = %d, want 0", got)
	}
}

func TestSettlePlanetProductionIfWholeOutputAvailableSkipsSubUnitWithoutAdvancing(t *testing.T) {
	base := testTime(0)
	store := newSettlementStore(t, "planet-1", base, 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)

	first, err := store.SettlePlanetProductionIfWholeOutputAvailable("planet-1", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("SettlePlanetProductionIfWholeOutputAvailable(first) error = %v, want nil", err)
	}
	if !first.NoOp || first.ElapsedRequested != time.Minute || first.ElapsedApplied != 0 {
		t.Fatalf("first NoOp/requested/applied = %v/%s/%s, want true/1m/0", first.NoOp, first.ElapsedRequested, first.ElapsedApplied)
	}
	if got := first.After.State.LastCalculatedAt; !got.Equal(base) {
		t.Fatalf("first LastCalculatedAt = %s, want unchanged %s", got, base)
	}
	if got := len(store.Events()); got != 0 {
		t.Fatalf("first events = %d, want none", got)
	}

	second, err := store.SettlePlanetProductionIfWholeOutputAvailable("planet-1", base.Add(90*time.Second))
	if err != nil {
		t.Fatalf("SettlePlanetProductionIfWholeOutputAvailable(second) error = %v, want nil", err)
	}
	if !second.NoOp {
		t.Fatal("second NoOp = false, want true")
	}
	if got := second.After.State.LastCalculatedAt; !got.Equal(base) {
		t.Fatalf("second LastCalculatedAt = %s, want unchanged %s", got, base)
	}
	if got := len(store.Events()); got != 0 {
		t.Fatalf("second events = %d, want none", got)
	}

	settledAt := base.Add(2 * time.Minute)
	settled, err := store.SettlePlanetProductionIfWholeOutputAvailable("planet-1", settledAt)
	if err != nil {
		t.Fatalf("SettlePlanetProductionIfWholeOutputAvailable(settled) error = %v, want nil", err)
	}
	if settled.NoOp || settled.ElapsedApplied != 2*time.Minute {
		t.Fatalf("settled NoOp/applied = %v/%s, want false/2m", settled.NoOp, settled.ElapsedApplied)
	}
	assertSettlementDelta(t, settled.ProducedItems, "iron_ore", 1)
	if got := settled.After.State.LastCalculatedAt; !got.Equal(settledAt) {
		t.Fatalf("settled LastCalculatedAt = %s, want %s", got, settledAt)
	}
	if got := settled.After.Storage.QuantityOf("iron_ore"); got != 1 {
		t.Fatalf("settled iron_ore = %d, want 1", got)
	}
	assertProductionEventTypes(t, store.Events(),
		EventPlanetBuildingProduced,
		EventPlanetProductionSettled,
		EventOfflineSettlementCompleted,
	)
}

func TestSettlePlanetProductionIfWholeOutputAvailableDuplicateSubUnitAfterSettlementNoOps(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	settledAt := testTime(0).Add(time.Hour)

	first, err := store.SettlePlanetProductionIfWholeOutputAvailable("planet-1", settledAt)
	if err != nil {
		t.Fatalf("SettlePlanetProductionIfWholeOutputAvailable(first) error = %v, want nil", err)
	}
	assertSettlementDelta(t, first.ProducedItems, "iron_ore", 30)
	eventCountAfterFirst := len(store.Events())

	duplicate, err := store.SettlePlanetProductionIfWholeOutputAvailable("planet-1", settledAt.Add(time.Second))
	if err != nil {
		t.Fatalf("SettlePlanetProductionIfWholeOutputAvailable(duplicate) error = %v, want nil", err)
	}
	if !duplicate.NoOp || duplicate.ElapsedRequested != time.Second || duplicate.ElapsedApplied != 0 {
		t.Fatalf("duplicate NoOp/requested/applied = %v/%s/%s, want true/1s/0", duplicate.NoOp, duplicate.ElapsedRequested, duplicate.ElapsedApplied)
	}
	if got := duplicate.After.State.LastCalculatedAt; !got.Equal(settledAt) {
		t.Fatalf("duplicate LastCalculatedAt = %s, want unchanged %s", got, settledAt)
	}
	if got := duplicate.After.Storage.QuantityOf("iron_ore"); got != 30 {
		t.Fatalf("duplicate iron_ore = %d, want 30", got)
	}
	if got := len(store.Events()); got != eventCountAfterFirst {
		t.Fatalf("duplicate events = %d, want unchanged %d", got, eventCountAfterFirst)
	}
}

func TestSettlePlanetProductionIfWholeOutputAvailableDisabledPlanetNoOpsWithoutAdvancing(t *testing.T) {
	base := testTime(0)
	store := newSettlementStore(t, "planet-1", base, 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	state, ok, err := store.ProductionState("planet-1")
	if err != nil || !ok {
		t.Fatalf("ProductionState() ok = %v err = %v, want true nil", ok, err)
	}
	state.ProductionEnabled = false
	state.UpdatedAt = base
	if err := store.SaveProductionState(state); err != nil {
		t.Fatalf("SaveProductionState() error = %v, want nil", err)
	}

	result, err := store.SettlePlanetProductionIfWholeOutputAvailable("planet-1", base.Add(time.Hour))
	if err != nil {
		t.Fatalf("SettlePlanetProductionIfWholeOutputAvailable() error = %v, want nil", err)
	}
	if !result.NoOp || result.ProductionEnabled {
		t.Fatalf("NoOp/ProductionEnabled = %v/%v, want true/false", result.NoOp, result.ProductionEnabled)
	}
	if got := result.After.State.LastCalculatedAt; !got.Equal(base) {
		t.Fatalf("LastCalculatedAt = %s, want unchanged %s", got, base)
	}
	if got := len(store.Events()); got != 0 {
		t.Fatalf("events = %d, want none", got)
	}
}

func TestSettlePlanetProductionSummaryIncludesRuntimeElapsedAndSnapshots(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 200, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	now := time.Date(2026, 6, 18, 20, 0, 0, 0, time.FixedZone("UTC+4", 4*60*60))

	result, err := store.SettlePlanetProduction("planet-1", now)
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	if result.SettledAt.Location() != time.UTC {
		t.Fatalf("SettledAt location = %v, want UTC", result.SettledAt.Location())
	}
	if result.MaxOfflineSettlementDuration != DefaultMaxOfflineSettlementDuration {
		t.Fatalf("MaxOfflineSettlementDuration = %s, want %s", result.MaxOfflineSettlementDuration, DefaultMaxOfflineSettlementDuration)
	}
	if result.ElapsedRequested != 4*time.Hour || result.ElapsedApplied != 4*time.Hour {
		t.Fatalf("elapsed requested/applied = %s/%s, want 4h/4h", result.ElapsedRequested, result.ElapsedApplied)
	}
	if result.Before.Storage.QuantityOf("iron_ore") != 0 {
		t.Fatalf("before iron_ore = %d, want 0", result.Before.Storage.QuantityOf("iron_ore"))
	}
	if result.After.Storage.QuantityOf("iron_ore") != 120 {
		t.Fatalf("after iron_ore = %d, want 120", result.After.Storage.QuantityOf("iron_ore"))
	}
	if len(result.BuildingResults) != 1 || result.BuildingResults[0].ElapsedApplied != 4*time.Hour {
		t.Fatalf("BuildingResults = %+v, want one 4h result", result.BuildingResults)
	}
}

func newSettlementStore(
	t *testing.T,
	planetID string,
	lastCalculatedAt time.Time,
	storageCapacity int64,
	energyCapacity int64,
) *InMemoryStore {
	t.Helper()
	store := NewInMemoryStore()
	if _, err := store.InitializePlanetProduction(InitializePlanetProductionInput{
		PlanetID:              foundation.PlanetID(planetID),
		LastCalculatedAt:      lastCalculatedAt,
		StorageCapacityUnits:  storageCapacity,
		EnergyCapacityPerHour: energyCapacity,
		UpdatedAt:             lastCalculatedAt,
	}); err != nil {
		t.Fatalf("InitializePlanetProduction() error = %v, want nil", err)
	}
	return store
}

func replaceSettlementStorage(
	t *testing.T,
	store *InMemoryStore,
	planetID string,
	capacity int64,
	items []StoredItem,
	updatedAt time.Time,
) {
	t.Helper()
	storage, err := NewPlanetStorage(foundation.PlanetID(planetID), capacity, items, updatedAt)
	if err != nil {
		t.Fatalf("NewPlanetStorage() error = %v, want nil", err)
	}
	if err := store.SavePlanetStorage(storage); err != nil {
		t.Fatalf("SavePlanetStorage() error = %v, want nil", err)
	}
}

func addSettlementBuilding(
	t *testing.T,
	store *InMemoryStore,
	planetID string,
	buildingID BuildingID,
	definitionID catalog.DefinitionID,
	state BuildingState,
) {
	t.Helper()
	definition, err := MustMVPCatalog().MustGet(definitionID)
	if err != nil {
		t.Fatalf("MustGet(%q) error = %v, want nil", definitionID, err)
	}
	building, err := NewPlanetBuilding(buildingID, foundation.PlanetID(planetID), definition, state, testTime(0), testTime(0))
	if err != nil {
		t.Fatalf("NewPlanetBuilding() error = %v, want nil", err)
	}
	if _, _, err := store.UpsertBuilding(building); err != nil {
		t.Fatalf("UpsertBuilding() error = %v, want nil", err)
	}
}

func assertSettlementDelta(t *testing.T, deltas []SettlementItemDelta, itemID string, quantity int64) {
	t.Helper()
	for _, delta := range deltas {
		if delta.ItemID == foundation.ItemID(itemID) {
			if delta.Quantity != quantity {
				t.Fatalf("delta %q = %d, want %d", itemID, delta.Quantity, quantity)
			}
			return
		}
	}
	t.Fatalf("delta %q missing in %+v", itemID, deltas)
}
