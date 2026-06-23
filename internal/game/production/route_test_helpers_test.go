package production

import (
	"fmt"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

type fakeRoutePolicyProvider struct {
	policy    RouteCreatePolicy
	err       error
	calls     int
	lastInput RouteCreatePolicyInput
}

func (provider *fakeRoutePolicyProvider) RouteCreatePolicy(input RouteCreatePolicyInput) (RouteCreatePolicy, error) {
	provider.calls++
	provider.lastInput = input
	if provider.err != nil {
		return RouteCreatePolicy{}, provider.err
	}
	return provider.policy, nil
}

type fixedRouteClock struct {
	now time.Time
}

func (clock fixedRouteClock) Now() time.Time {
	return clock.now
}

func newTestRouteService(
	t *testing.T,
	store *InMemoryStore,
	provider RouteCreatePolicyProvider,
	now time.Time,
) *AutomationRouteService {
	t.Helper()
	ensureRouteProductionStateForTest(t, store, "planet-1", 100, now)
	service, err := NewAutomationRouteService(AutomationRouteServiceConfig{
		Store:  store,
		Clock:  fixedRouteClock{now: now},
		Policy: provider,
	})
	if err != nil {
		t.Fatalf("NewAutomationRouteService() error = %v, want nil", err)
	}
	return service
}

func newTestRouteSettlementService(
	t *testing.T,
	store *InMemoryStore,
	now time.Time,
	lossRoller RouteLossRoller,
) *AutomationRouteService {
	t.Helper()
	service, err := NewAutomationRouteService(AutomationRouteServiceConfig{
		Store:      store,
		Clock:      fixedRouteClock{now: now},
		Policy:     &fakeRoutePolicyProvider{policy: validRoutePolicy()},
		LossRoller: lossRoller,
	})
	if err != nil {
		t.Fatalf("NewAutomationRouteService() error = %v, want nil", err)
	}
	return service
}

func validCreateRouteInput() CreateRouteInput {
	return CreateRouteInput{
		RouteID:        "route-1",
		OwnerPlayerID:  "player-1",
		SourcePlanetID: "planet-1",
		Destination: RouteDestination{
			Type: RouteDestinationTypePlanet,
			ID:   "planet-2",
		},
		ResourceItemID: "refined_alloy",
		AmountPerHour:  40,
	}
}

func validUpdateRouteInput() UpdateRouteInput {
	return UpdateRouteInput{
		RouteID:       "route-1",
		OwnerPlayerID: "player-1",
		Destination: RouteDestination{
			Type: RouteDestinationTypePlanet,
			ID:   "planet-2",
		},
		ResourceItemID: "refined_alloy",
		AmountPerHour:  40,
	}
}

func validRoutePolicy() RouteCreatePolicy {
	return RouteCreatePolicy{
		SourcePlanetOwned:         true,
		DestinationAccessible:     true,
		ResourceRouteable:         true,
		RequirementsMet:           true,
		SourceMapID:               "map_1_1",
		DestinationMapID:          "map_1_1",
		DistanceUnits:             100,
		MaxDistanceUnits:          1_000,
		BaseLossChance:            0.02,
		DistanceLossChancePerUnit: 0.0001,
		SourceRegionRisk:          0.02,
		DestinationRegionRisk:     0.01,
		DeepSpaceRisk:             0,
		PlayerLossReduction:       0.005,
		RouteSecurityReduction:    0.005,
		MinLossPercent:            0.25,
		MaxLossPercent:            0.75,
		EnergyCostPerHour:         12,
	}
}

func noLossRoutePolicy() RouteCreatePolicy {
	policy := validRoutePolicy()
	policy.BaseLossChance = 0
	policy.DistanceLossChancePerUnit = 0
	policy.SourceRegionRisk = 0
	policy.DestinationRegionRisk = 0
	policy.DeepSpaceRisk = 0
	policy.PlayerLossReduction = 0
	policy.RouteSecurityReduction = 0
	policy.MinLossPercent = 0
	policy.MaxLossPercent = 0
	policy.EnergyCostPerHour = 24
	return policy
}

func validSettlementRoute(lastCalculatedAt time.Time) AutomationRoute {
	return AutomationRoute{
		RouteID:        "route-1",
		OwnerPlayerID:  "player-1",
		SourcePlanetID: "planet-1",
		SourceMapID:    "map_1_1",
		Destination: RouteDestination{
			Type: RouteDestinationTypePlanet,
			ID:   "planet-2",
		},
		DestinationMapID:  "map_1_1",
		ResourceItemID:    "refined_alloy",
		AmountPerHour:     40,
		EnergyCostPerHour: 12,
		Risk: RouteRisk{
			LossChance:     0,
			MinLossPercent: 0.25,
			MaxLossPercent: 0.75,
		},
		Enabled:          true,
		LastCalculatedAt: lastCalculatedAt,
		CreatedAt:        lastCalculatedAt,
		UpdatedAt:        lastCalculatedAt,
	}
}

func assertRouteEnabled(
	t *testing.T,
	store *InMemoryStore,
	routeID foundation.RouteID,
	enabled bool,
) {
	t.Helper()
	route, ok, err := store.AutomationRoute(routeID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", routeID, ok, err)
	}
	if route.Enabled != enabled {
		t.Fatalf("route Enabled = %v, want %v", route.Enabled, enabled)
	}
}

func assertRouteAmountAndTime(
	t *testing.T,
	store *InMemoryStore,
	routeID foundation.RouteID,
	amountPerHour int64,
	lastCalculatedAt time.Time,
) {
	t.Helper()
	route, ok, err := store.AutomationRoute(routeID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", routeID, ok, err)
	}
	if route.AmountPerHour != amountPerHour {
		t.Fatalf("route AmountPerHour = %d, want %d", route.AmountPerHour, amountPerHour)
	}
	if !route.LastCalculatedAt.Equal(lastCalculatedAt) || !route.UpdatedAt.Equal(lastCalculatedAt) {
		t.Fatalf("route timestamps = %s/%s, want %s", route.LastCalculatedAt, route.UpdatedAt, lastCalculatedAt)
	}
}

func assertRouteMapIdentity(
	t *testing.T,
	route AutomationRoute,
	sourceMapID RouteMapID,
	destinationMapID RouteMapID,
) {
	t.Helper()
	if route.SourceMapID != sourceMapID || route.DestinationMapID != destinationMapID {
		t.Fatalf("route map ids = %q/%q, want %q/%q", route.SourceMapID, route.DestinationMapID, sourceMapID, destinationMapID)
	}
}

func assertRouteDurableRecord(
	t *testing.T,
	store *InMemoryStore,
	routeID foundation.RouteID,
	referenceKey foundation.IdempotencyKey,
	revision uint64,
	wantRoute AutomationRoute,
) {
	t.Helper()
	record, ok, err := store.CommittedAutomationRouteDurableRecord(routeID)
	if err != nil || !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecord(%q) ok = %v err = %v, want true nil", routeID, ok, err)
	}
	if record.Revision != revision || record.ReferenceKey != referenceKey {
		t.Fatalf("durable route record revision/reference = %d/%q, want %d/%q", record.Revision, record.ReferenceKey, revision, referenceKey)
	}
	if record.Route != cloneAutomationRoute(wantRoute) {
		t.Fatalf("durable route = %+v, want %+v", record.Route, cloneAutomationRoute(wantRoute))
	}
	byReference, ok, err := store.CommittedAutomationRouteDurableRecordByReference(referenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecordByReference(%q) ok = %v err = %v, want true nil", referenceKey, ok, err)
	}
	if byReference.Route.RouteID != routeID || byReference.Revision != revision {
		t.Fatalf("durable route by reference = %+v, want route %q revision %d", byReference, routeID, revision)
	}
}

func assertRouteDurableRecordSourceEnergy(
	t *testing.T,
	store *InMemoryStore,
	routeID foundation.RouteID,
	wantPlanetID foundation.PlanetID,
	wantReserved int64,
) {
	t.Helper()
	record, ok, err := store.CommittedAutomationRouteDurableRecord(routeID)
	if err != nil || !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecord(%q) ok = %v err = %v, want true nil", routeID, ok, err)
	}
	if record.SourceProductionState == nil {
		t.Fatalf("durable route record source production state = nil, want planet %q reserved %d", wantPlanetID, wantReserved)
	}
	if record.SourceProductionState.PlanetID != wantPlanetID || record.SourceProductionState.EnergyReservedPerHour != wantReserved {
		t.Fatalf("durable route source production state = %+v, want planet %q reserved %d", record.SourceProductionState, wantPlanetID, wantReserved)
	}
	byReference, ok, err := store.CommittedAutomationRouteDurableRecordByReference(record.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecordByReference(%q) ok = %v err = %v, want true nil", record.ReferenceKey, ok, err)
	}
	if byReference.SourceProductionState == nil ||
		byReference.SourceProductionState.PlanetID != wantPlanetID ||
		byReference.SourceProductionState.EnergyReservedPerHour != wantReserved {
		t.Fatalf("durable route by reference source production state = %+v, want planet %q reserved %d", byReference.SourceProductionState, wantPlanetID, wantReserved)
	}
}

func newRouteSettlementStore(
	t *testing.T,
	route AutomationRoute,
	sourceCapacity int64,
	sourceItems []StoredItem,
	destinationCapacity int64,
	destinationItems []StoredItem,
) *InMemoryStore {
	t.Helper()
	store := NewInMemoryStore()
	ensureRouteProductionStateForTest(t, store, route.SourcePlanetID, 100, route.UpdatedAt)
	insertRouteSettlementRoute(t, store, route)
	saveRouteSettlementStorage(t, store, route.SourcePlanetID, sourceCapacity, sourceItems, route.UpdatedAt)
	if route.Destination.Type == RouteDestinationTypePlanet {
		saveRouteSettlementStorage(t, store, foundation.PlanetID(route.Destination.ID), destinationCapacity, destinationItems, route.UpdatedAt)
	}
	return store
}

func ensureRouteProductionStateForTest(
	t *testing.T,
	store *InMemoryStore,
	planetID foundation.PlanetID,
	energyCapacityPerHour int64,
	updatedAt time.Time,
) {
	t.Helper()
	if _, ok, err := store.Snapshot(planetID); err != nil {
		t.Fatalf("Snapshot(%q) error = %v, want nil", planetID, err)
	} else if ok {
		return
	}
	if _, err := store.InitializePlanetProduction(InitializePlanetProductionInput{
		PlanetID:              planetID,
		LastCalculatedAt:      updatedAt,
		StorageCapacityUnits:  1_000,
		EnergyCapacityPerHour: energyCapacityPerHour,
		UpdatedAt:             updatedAt,
	}); err != nil {
		t.Fatalf("InitializePlanetProduction(%q) error = %v, want nil", planetID, err)
	}
}

func insertRouteSettlementRoute(t *testing.T, store *InMemoryStore, route AutomationRoute) {
	t.Helper()
	if _, err := store.insertAutomationRoute(route); err != nil {
		t.Fatalf("insertAutomationRoute() error = %v, want nil", err)
	}
}

func saveRouteSettlementStorage(
	t *testing.T,
	store *InMemoryStore,
	planetID foundation.PlanetID,
	capacity int64,
	items []StoredItem,
	updatedAt time.Time,
) {
	t.Helper()
	storage, err := NewPlanetStorage(planetID, capacity, items, updatedAt)
	if err != nil {
		t.Fatalf("NewPlanetStorage(%q) error = %v, want nil", planetID, err)
	}
	if err := store.SavePlanetStorage(storage); err != nil {
		t.Fatalf("SavePlanetStorage(%q) error = %v, want nil", planetID, err)
	}
}

func assertRouteSettlementRouteTime(
	t *testing.T,
	store *InMemoryStore,
	routeID foundation.RouteID,
	lastCalculatedAt time.Time,
) {
	t.Helper()
	route, ok, err := store.AutomationRoute(routeID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", routeID, ok, err)
	}
	if !route.LastCalculatedAt.Equal(lastCalculatedAt) || !route.UpdatedAt.Equal(lastCalculatedAt) {
		t.Fatalf("route timestamps = %s/%s, want %s", route.LastCalculatedAt, route.UpdatedAt, lastCalculatedAt)
	}
}

func assertRouteSettlementStorage(
	t *testing.T,
	store *InMemoryStore,
	planetID foundation.PlanetID,
	itemID foundation.ItemID,
	quantity int64,
	updatedAt time.Time,
) {
	t.Helper()
	storage, ok, err := store.PlanetStorage(planetID)
	if err != nil || !ok {
		t.Fatalf("PlanetStorage(%q) ok = %v err = %v, want true nil", planetID, ok, err)
	}
	if got := storage.QuantityOf(itemID); got != quantity {
		t.Fatalf("storage %q item %q = %d, want %d", planetID, itemID, got, quantity)
	}
	if !storage.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("storage %q UpdatedAt = %s, want %s", planetID, storage.UpdatedAt, updatedAt)
	}
}

func assertRouteEnergyReserved(
	t *testing.T,
	store *InMemoryStore,
	planetID foundation.PlanetID,
	want int64,
) {
	t.Helper()
	state, ok, err := store.ProductionState(planetID)
	if err != nil || !ok {
		t.Fatalf("ProductionState(%q) ok = %v err = %v, want true nil", planetID, ok, err)
	}
	if state.EnergyReservedPerHour != want {
		t.Fatalf("ProductionState(%q) EnergyReservedPerHour = %d, want %d", planetID, state.EnergyReservedPerHour, want)
	}
}

func setRouteEnergyStateForTest(
	t *testing.T,
	store *InMemoryStore,
	planetID foundation.PlanetID,
	capacity int64,
	reserved int64,
	updatedAt time.Time,
) {
	t.Helper()
	state, err := NewPlanetProductionState(planetID, updatedAt, capacity, updatedAt)
	if err != nil {
		t.Fatalf("NewPlanetProductionState(%q) error = %v, want nil", planetID, err)
	}
	state.EnergyReservedPerHour = reserved
	if err := state.Validate(); err != nil {
		t.Fatalf("route energy test state Validate() error = %v, want nil", err)
	}
	if err := store.SaveProductionState(state); err != nil {
		t.Fatalf("SaveProductionState(%q) error = %v, want nil", planetID, err)
	}
	if _, ok, err := store.PlanetStorage(planetID); err != nil {
		t.Fatalf("PlanetStorage(%q) error = %v, want nil", planetID, err)
	} else if !ok {
		storage, err := NewPlanetStorage(planetID, 1_000, nil, updatedAt)
		if err != nil {
			t.Fatalf("NewPlanetStorage(%q) error = %v, want nil", planetID, err)
		}
		if err := store.SavePlanetStorage(storage); err != nil {
			t.Fatalf("SavePlanetStorage(%q) error = %v, want nil", planetID, err)
		}
	}
}

type routeStorageLedgerWant struct {
	Operation            RouteStorageLedgerOperation
	PlanetID             foundation.PlanetID
	CounterpartyPlanetID foundation.PlanetID
	Quantity             int64
	BalanceAfter         int64
	ReferenceKey         foundation.IdempotencyKey
	SettlementWindow     string
}

func assertRouteStorageLedgerEntries(t *testing.T, entries []RouteStorageLedgerEntry, wants ...routeStorageLedgerWant) {
	t.Helper()
	if len(entries) != len(wants) {
		t.Fatalf("route storage ledger rows = %d, want %d; entries = %+v", len(entries), len(wants), entries)
	}
	for index, want := range wants {
		entry := entries[index]
		if err := entry.Validate(); err != nil {
			t.Fatalf("route storage ledger[%d] validation error = %v; entry = %+v", index, err, entry)
		}
		wantLedgerID := fmt.Sprintf("route-storage-ledger-%d", index+1)
		if entry.LedgerID != wantLedgerID {
			t.Fatalf("route storage ledger[%d] id = %q, want %q", index, entry.LedgerID, wantLedgerID)
		}
		if entry.Operation != want.Operation ||
			entry.RouteID != "route-1" ||
			entry.PlanetID != want.PlanetID ||
			entry.CounterpartyPlanetID != want.CounterpartyPlanetID ||
			entry.ItemID != "refined_alloy" ||
			entry.Quantity != want.Quantity ||
			entry.BalanceAfter != want.BalanceAfter ||
			entry.ReferenceKey != want.ReferenceKey ||
			entry.SettlementWindow != want.SettlementWindow {
			t.Fatalf("route storage ledger[%d] = %+v, want op=%q planet=%q counterparty=%q qty=%d balance=%d ref=%q window=%q",
				index, entry, want.Operation, want.PlanetID, want.CounterpartyPlanetID, want.Quantity, want.BalanceAfter, want.ReferenceKey, want.SettlementWindow)
		}
		if entry.CreatedAt.IsZero() {
			t.Fatalf("route storage ledger[%d] CreatedAt is zero", index)
		}
	}
}

func assertNoRouteStorageLedger(t *testing.T, store *InMemoryStore) {
	t.Helper()
	if got := len(store.RouteStorageLedgerEntries()); got != 0 {
		t.Fatalf("route storage ledger rows = %d, want 0", got)
	}
}

func assertNoRouteEvents(t *testing.T, store *InMemoryStore) {
	t.Helper()
	if got := len(store.Events()); got != 0 {
		t.Fatalf("route event count = %d, want 0", got)
	}
}

func testRouteNow() time.Time {
	return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
}

var _ RouteCreatePolicyProvider = (*fakeRoutePolicyProvider)(nil)
var _ foundation.Clock = fixedRouteClock{}
