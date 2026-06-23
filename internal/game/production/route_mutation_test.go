package production

import (
	"errors"
	"testing"
	"time"
)

func TestDisableRouteSettlesOldRouteBeforeDisabling(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		200,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.DisableRouteForOwnerWithRequest("player-1", route.RouteID, "request-disable-route-1")
	if err != nil {
		t.Fatalf("DisableRoute() error = %v, want nil", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	if result.Route.Enabled {
		t.Fatal("route Enabled = true, want false")
	}
	assertRouteMapIdentity(t, result.Route, route.SourceMapID, route.DestinationMapID)
	assertRouteMapIdentity(t, result.Settlement.BeforeRoute, route.SourceMapID, route.DestinationMapID)
	assertRouteMapIdentity(t, result.Settlement.AfterRoute, route.SourceMapID, route.DestinationMapID)
	if result.Settlement.AddedAmount != 40 || result.Settlement.TakenAmount != 40 {
		t.Fatalf("settlement taken/added = %d/%d, want 40/40", result.Settlement.TakenAmount, result.Settlement.AddedAmount)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, now)
	assertRouteEnabled(t, store, route.RouteID, false)
	assertRouteEnergyReserved(t, store, route.SourcePlanetID, 0)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, now)
	assertRouteDurableRecord(t, store, route.RouteID, "route_disable:player-1:route-1:request-disable-route-1", 3, result.Route)
	assertRouteDurableRecordSourceEnergy(t, store, route.RouteID, route.SourcePlanetID, 0)
}

func TestDisableRouteSettlesSourceProductionBeforeReleasingEnergy(t *testing.T) {
	last := testTime(0)
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
	setRouteEnergyStateForTest(t, store, route.SourcePlanetID, route.EnergyCostPerHour, route.EnergyCostPerHour, last)
	addSettlementBuilding(t, store, route.SourcePlanetID.String(), "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	service := newTestRouteSettlementService(t, store, now, nil)

	result, err := service.DisableRoute(route.RouteID)
	if err != nil {
		t.Fatalf("DisableRoute() error = %v, want nil", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	assertRouteEnergyReserved(t, store, route.SourcePlanetID, 0)
	sourceSnapshot, ok, err := store.Snapshot(route.SourcePlanetID)
	if err != nil || !ok {
		t.Fatalf("Snapshot(%q) ok=%v err=%v, want true nil", route.SourcePlanetID, ok, err)
	}
	if got := sourceSnapshot.Storage.QuantityOf("iron_ore"); got != 0 {
		t.Fatalf("source iron_ore after disable = %d, want 0 because route energy occupied the elapsed window", got)
	}
	if !sourceSnapshot.State.LastCalculatedAt.Equal(now.UTC()) {
		t.Fatalf("source LastCalculatedAt = %s, want %s", sourceSnapshot.State.LastCalculatedAt, now.UTC())
	}
}

func TestDisableRouteForOwnerRejectsWrongOwnerWithoutMutation(t *testing.T) {
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

	_, err := service.DisableRouteForOwner("player-2", route.RouteID)
	if !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("DisableRouteForOwner() error = %v, want ErrRouteOwnerMismatch", err)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteEnabled(t, store, route.RouteID, true)
	assertRouteEnergyReserved(t, store, route.SourcePlanetID, 12)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
	assertNoRouteEvents(t, store)
}

func TestEnableRouteResetsLastCalculatedAtSoDisabledElapsedDoesNotTransfer(t *testing.T) {
	last := testRouteNow()
	enableAt := last.Add(10 * time.Hour)
	settleAt := enableAt.Add(time.Hour)
	route := validSettlementRoute(last)
	route.Enabled = false
	store := newRouteSettlementStore(
		t,
		route,
		1_000,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 1_000}},
		1_000,
		nil,
	)
	enableService := newTestRouteSettlementService(t, store, enableAt, nil)

	enableResult, err := enableService.EnableRouteForOwnerWithRequest("player-1", route.RouteID, "request-enable-route-1")
	if err != nil {
		t.Fatalf("EnableRoute() error = %v, want nil", err)
	}
	if !enableResult.Changed || !enableResult.Route.Enabled {
		t.Fatalf("enable Changed/Enabled = %v/%v, want true/true", enableResult.Changed, enableResult.Route.Enabled)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, enableAt)
	assertRouteEnergyReserved(t, store, route.SourcePlanetID, 12)
	assertRouteDurableRecord(t, store, route.RouteID, "route_enable:player-1:route-1:request-enable-route-1", 2, enableResult.Route)
	assertRouteDurableRecordSourceEnergy(t, store, route.RouteID, route.SourcePlanetID, enableResult.Route.EnergyCostPerHour)

	settleService := newTestRouteSettlementService(t, store, settleAt, nil)
	settleResult, err := settleService.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if settleResult.AddedAmount != 40 || settleResult.ElapsedApplied != time.Hour {
		t.Fatalf("settlement added/applied = %d/%s, want 40/1h", settleResult.AddedAmount, settleResult.ElapsedApplied)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 960, settleAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, settleAt)
}

func TestEnableRouteForOwnerRejectsWrongOwnerWithoutMutation(t *testing.T) {
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

	_, err := service.EnableRouteForOwner("player-2", route.RouteID)
	if !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("EnableRouteForOwner() error = %v, want ErrRouteOwnerMismatch", err)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteEnabled(t, store, route.RouteID, false)
	assertRouteEnergyReserved(t, store, route.SourcePlanetID, 0)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
	assertNoRouteEvents(t, store)
}

func TestEnableRouteRejectsEnergyReservationOverCapacityWithoutMutation(t *testing.T) {
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
	setRouteEnergyStateForTest(t, store, route.SourcePlanetID, 10, 0, last)
	service := newTestRouteSettlementService(t, store, now, nil)

	_, err := service.EnableRoute(route.RouteID)
	if !errors.Is(err, ErrRouteEnergyUnavailable) {
		t.Fatalf("EnableRoute() error = %v, want ErrRouteEnergyUnavailable", err)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteEnabled(t, store, route.RouteID, false)
	assertRouteEnergyReserved(t, store, route.SourcePlanetID, 0)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

func TestEnableRouteDoesNotMoveFutureTimestampBackward(t *testing.T) {
	now := testRouteNow()
	futureLast := now.Add(time.Hour)
	route := validSettlementRoute(now)
	route.Enabled = false
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

	result, err := service.EnableRoute(route.RouteID)
	if err != nil {
		t.Fatalf("EnableRoute() error = %v, want nil", err)
	}
	if !result.Changed || !result.Route.Enabled {
		t.Fatalf("enable Changed/Enabled = %v/%v, want true/true", result.Changed, result.Route.Enabled)
	}
	stored, ok, err := store.AutomationRoute(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", route.RouteID, ok, err)
	}
	if !stored.LastCalculatedAt.Equal(futureLast) {
		t.Fatalf("LastCalculatedAt = %s, want unchanged future %s", stored.LastCalculatedAt, futureLast)
	}
	if !stored.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %s, want enable time %s", stored.UpdatedAt, now)
	}
}

func TestUpdateRouteSettlesOldAmountBeforeApplyingNewAmount(t *testing.T) {
	last := testRouteNow()
	updateAt := last.Add(time.Hour)
	settleAt := updateAt.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		1_000,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 1_000}},
		1_000,
		nil,
	)
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	provider.policy.DestinationMapID = "map_1_2"
	service := newTestRouteService(t, store, provider, updateAt)
	input := validUpdateRouteInput()
	input.AmountPerHour = 80
	input.RequestID = "request-update-route-1"

	result, err := service.UpdateRoute(input)
	if err != nil {
		t.Fatalf("UpdateRoute() error = %v, want nil", err)
	}
	if !result.Updated {
		t.Fatal("Updated = false, want true")
	}
	if result.Settlement.AddedAmount != 40 || result.Settlement.WantedAmount != 40 {
		t.Fatalf("settlement wanted/added = %d/%d, want 40/40", result.Settlement.WantedAmount, result.Settlement.AddedAmount)
	}
	if result.Route.AmountPerHour != 80 || result.Route.EnergyCostPerHour != provider.policy.EnergyCostPerHour {
		t.Fatalf("updated amount/energy = %d/%d, want 80/%d", result.Route.AmountPerHour, result.Route.EnergyCostPerHour, provider.policy.EnergyCostPerHour)
	}
	if result.Route.SourcePlanetID != route.SourcePlanetID || result.Route.OwnerPlayerID != route.OwnerPlayerID || !result.Route.CreatedAt.Equal(route.CreatedAt) {
		t.Fatalf("source/owner/created changed: %+v, want source %q owner %q created %s", result.Route, route.SourcePlanetID, route.OwnerPlayerID, route.CreatedAt)
	}
	assertRouteMapIdentity(t, result.Route, route.SourceMapID, provider.policy.DestinationMapID)
	if provider.calls != 1 || provider.lastInput.SourcePlanetID != route.SourcePlanetID || provider.lastInput.AmountPerHour != 80 {
		t.Fatalf("policy calls/input = %d/%+v, want one call with source %q amount 80", provider.calls, provider.lastInput, route.SourcePlanetID)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, updateAt)
	assertRouteEnergyReserved(t, store, route.SourcePlanetID, provider.policy.EnergyCostPerHour)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 960, updateAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, updateAt)
	storedAfterUpdate, ok, err := store.AutomationRoute(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", route.RouteID, ok, err)
	}
	assertRouteMapIdentity(t, storedAfterUpdate, route.SourceMapID, provider.policy.DestinationMapID)
	assertRouteDurableRecord(t, store, route.RouteID, "route_update:player-1:route-1:request-update-route-1", 3, storedAfterUpdate)
	assertRouteDurableRecordSourceEnergy(t, store, route.RouteID, route.SourcePlanetID, storedAfterUpdate.EnergyCostPerHour)

	settleService := newTestRouteSettlementService(t, store, settleAt, nil)
	settleResult, err := settleService.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if settleResult.AddedAmount != 80 || settleResult.WantedAmount != 80 {
		t.Fatalf("post-update wanted/added = %d/%d, want 80/80", settleResult.WantedAmount, settleResult.AddedAmount)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 880, settleAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 120, settleAt)
	settledRoute, ok, err := store.AutomationRoute(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", route.RouteID, ok, err)
	}
	assertRouteMapIdentity(t, settledRoute, route.SourceMapID, provider.policy.DestinationMapID)
}

func TestUpdateRouteStorageDestinationUsesPolicyAndSettlesIntoStorageAggregate(t *testing.T) {
	last := testRouteNow()
	updateAt := last.Add(time.Hour)
	settleAt := updateAt.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		1_000,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 1_000}},
		1_000,
		nil,
	)
	saveRouteSettlementStorage(t, store, "storage-1", 1_000, nil, updateAt)
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	provider.policy.DestinationMapID = "map_storage"
	service := newTestRouteService(t, store, provider, updateAt)
	input := validUpdateRouteInput()
	input.Destination = RouteDestination{Type: RouteDestinationTypeStorage, ID: "storage-1"}
	input.AmountPerHour = 80
	input.RequestID = "request-update-route-storage"

	result, err := service.UpdateRoute(input)
	if err != nil {
		t.Fatalf("UpdateRoute(storage destination) error = %v, want nil", err)
	}
	if provider.calls != 1 || provider.lastInput.Destination != input.Destination {
		t.Fatalf("policy calls/input = %d/%+v, want storage destination policy lookup", provider.calls, provider.lastInput)
	}
	if result.Route.Destination != input.Destination {
		t.Fatalf("updated route destination = %+v, want %+v", result.Route.Destination, input.Destination)
	}
	assertRouteMapIdentity(t, result.Route, route.SourceMapID, "map_storage")

	settleService := newTestRouteSettlementService(t, store, settleAt, nil)
	settleResult, err := settleService.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute(storage destination after update) error = %v, want nil", err)
	}
	if settleResult.WantedAmount != 80 || settleResult.AddedAmount != 80 {
		t.Fatalf("storage post-update settlement wanted/added = %d/%d, want 80/80", settleResult.WantedAmount, settleResult.AddedAmount)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 880, settleAt)
	assertRouteSettlementStorage(t, store, "storage-1", "refined_alloy", 80, settleAt)
}

func TestUpdateRouteStationDestinationUsesPolicyAndSettlesIntoStationAggregate(t *testing.T) {
	last := testRouteNow()
	updateAt := last.Add(time.Hour)
	settleAt := updateAt.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		1_000,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 1_000}},
		1_000,
		nil,
	)
	saveRouteSettlementStorage(t, store, "station-1", 1_000, nil, updateAt)
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	provider.policy.DestinationMapID = "map_station"
	service := newTestRouteService(t, store, provider, updateAt)
	input := validUpdateRouteInput()
	input.Destination = RouteDestination{Type: RouteDestinationTypeStation, ID: "station-1"}
	input.AmountPerHour = 80
	input.RequestID = "request-update-route-station"

	result, err := service.UpdateRoute(input)
	if err != nil {
		t.Fatalf("UpdateRoute(station destination) error = %v, want nil", err)
	}
	if provider.calls != 1 || provider.lastInput.Destination != input.Destination {
		t.Fatalf("policy calls/input = %d/%+v, want station destination policy lookup", provider.calls, provider.lastInput)
	}
	if result.Route.Destination != input.Destination {
		t.Fatalf("updated route destination = %+v, want %+v", result.Route.Destination, input.Destination)
	}
	assertRouteMapIdentity(t, result.Route, route.SourceMapID, "map_station")

	settleService := newTestRouteSettlementService(t, store, settleAt, nil)
	settleResult, err := settleService.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute(station destination after update) error = %v, want nil", err)
	}
	if settleResult.WantedAmount != 80 || settleResult.AddedAmount != 80 {
		t.Fatalf("station post-update settlement wanted/added = %d/%d, want 80/80", settleResult.WantedAmount, settleResult.AddedAmount)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 880, settleAt)
	assertRouteSettlementStorage(t, store, "station-1", "refined_alloy", 80, settleAt)
}

func TestUpdateRouteAtExistingEnergyCapacitySucceedsWhenCostIsUnchanged(t *testing.T) {
	last := testRouteNow()
	updateAt := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		1_000,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 1_000}},
		1_000,
		nil,
	)
	setRouteEnergyStateForTest(t, store, route.SourcePlanetID, route.EnergyCostPerHour, route.EnergyCostPerHour, last)
	provider := &fakeRoutePolicyProvider{policy: validRoutePolicy()}
	provider.policy.DestinationMapID = "map_1_2"
	provider.policy.EnergyCostPerHour = route.EnergyCostPerHour
	service := newTestRouteService(t, store, provider, updateAt)
	input := validUpdateRouteInput()
	input.RequestID = "request-update-route-at-energy-cap"

	result, err := service.UpdateRoute(input)
	if err != nil {
		t.Fatalf("UpdateRoute() error = %v, want nil", err)
	}
	if !result.Updated {
		t.Fatal("Updated = false, want true")
	}
	assertRouteEnergyReserved(t, store, route.SourcePlanetID, route.EnergyCostPerHour)
	record, ok, err := store.CommittedAutomationRouteDurableRecord(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecord(%q) ok=%v err=%v, want true nil", route.RouteID, ok, err)
	}
	if record.SourceProductionState != nil {
		t.Fatalf("same-cost update durable source production state = %+v, want nil", record.SourceProductionState)
	}
}

func TestUpdateRouteRejectsEnergyReservationOverCapacityWithoutSettlement(t *testing.T) {
	last := testRouteNow()
	updateAt := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		1_000,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 1_000}},
		1_000,
		nil,
	)
	setRouteEnergyStateForTest(t, store, route.SourcePlanetID, 20, route.EnergyCostPerHour, last)
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	provider.policy.DestinationMapID = "map_1_2"
	service := newTestRouteService(t, store, provider, updateAt)
	input := validUpdateRouteInput()
	input.AmountPerHour = 80
	input.RequestID = "request-update-route-energy-overflow"

	_, err := service.UpdateRoute(input)
	if !errors.Is(err, ErrRouteEnergyUnavailable) {
		t.Fatalf("UpdateRoute() error = %v, want ErrRouteEnergyUnavailable", err)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteEnergyReserved(t, store, route.SourcePlanetID, route.EnergyCostPerHour)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 1_000, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

func TestUpdateRouteDuplicateRequestDoesNotSettleTwice(t *testing.T) {
	last := testRouteNow()
	firstUpdateAt := last.Add(time.Hour)
	duplicateAt := firstUpdateAt.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		1_000,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 1_000}},
		1_000,
		nil,
	)
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	provider.policy.DestinationMapID = "map_1_2"
	input := validUpdateRouteInput()
	input.AmountPerHour = 80
	input.RequestID = "request-update-duplicate"

	firstService := newTestRouteService(t, store, provider, firstUpdateAt)
	first, err := firstService.UpdateRoute(input)
	if err != nil {
		t.Fatalf("UpdateRoute(first) error = %v, want nil", err)
	}
	if first.Settlement.AddedAmount != 40 {
		t.Fatalf("first settlement added = %d, want 40", first.Settlement.AddedAmount)
	}

	duplicateService := newTestRouteService(t, store, provider, duplicateAt)
	duplicate, err := duplicateService.UpdateRoute(input)
	if err != nil {
		t.Fatalf("UpdateRoute(duplicate) error = %v, want nil", err)
	}
	if duplicate.Updated || !duplicate.Settlement.RouteID.IsZero() {
		t.Fatalf("duplicate Updated/settlement = %v/%+v, want replay without mutation", duplicate.Updated, duplicate.Settlement)
	}
	if !duplicate.Route.UpdatedAt.Equal(first.Route.UpdatedAt) || duplicate.Route.AmountPerHour != first.Route.AmountPerHour {
		t.Fatalf("duplicate route = %+v, want first durable route %+v", duplicate.Route, first.Route)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 960, firstUpdateAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, firstUpdateAt)
}

func TestUpdateRouteRejectsSourceMapPolicyMismatchWithoutMutation(t *testing.T) {
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
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	provider.policy.SourceMapID = "map_1_2"
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.AmountPerHour = 80

	_, err := service.UpdateRoute(input)
	if !errors.Is(err, ErrInvalidRouteMapID) {
		t.Fatalf("UpdateRoute() error = %v, want ErrInvalidRouteMapID", err)
	}
	assertRouteAmountAndTime(t, store, route.RouteID, 40, last)
	stored, ok, lookupErr := store.AutomationRoute(route.RouteID)
	if lookupErr != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", route.RouteID, ok, lookupErr)
	}
	assertRouteMapIdentity(t, stored, route.SourceMapID, route.DestinationMapID)
	assertNoRouteEvents(t, store)
}

func TestUpdateRouteRejectsOwnerMismatchWithoutMutation(t *testing.T) {
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
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.OwnerPlayerID = "player-2"
	input.AmountPerHour = 80

	_, err := service.UpdateRoute(input)
	if !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("UpdateRoute() error = %v, want ErrRouteOwnerMismatch", err)
	}
	if provider.calls != 0 {
		t.Fatalf("policy calls = %d, want 0", provider.calls)
	}
	assertRouteAmountAndTime(t, store, route.RouteID, 40, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

func TestUpdateRouteForOwnerRejectsWrongOwnerWithoutMutation(t *testing.T) {
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
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.OwnerPlayerID = "player-1"
	input.AmountPerHour = 80

	_, err := service.UpdateRouteForOwner("player-2", input)
	if !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("UpdateRouteForOwner() error = %v, want ErrRouteOwnerMismatch", err)
	}
	if provider.calls != 0 {
		t.Fatalf("policy calls = %d, want 0", provider.calls)
	}
	assertRouteAmountAndTime(t, store, route.RouteID, 40, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
	assertNoRouteEvents(t, store)
}

func TestUpdateRouteInvalidDestinationFailsBeforePolicyLookup(t *testing.T) {
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
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.Destination = RouteDestination{Type: RouteDestinationType("wormhole"), ID: "wormhole-1"}

	_, err := service.UpdateRoute(input)
	if !errors.Is(err, ErrInvalidRouteDestinationType) {
		t.Fatalf("UpdateRoute() error = %v, want ErrInvalidRouteDestinationType", err)
	}
	if provider.calls != 0 {
		t.Fatalf("policy calls = %d, want 0", provider.calls)
	}
	assertRouteAmountAndTime(t, store, route.RouteID, 40, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

func TestUpdateRouteRejectsInvalidPolicyWithoutMutation(t *testing.T) {
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
	policy := noLossRoutePolicy()
	policy.ResourceRouteable = false
	provider := &fakeRoutePolicyProvider{policy: policy}
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.AmountPerHour = 80

	_, err := service.UpdateRoute(input)
	if !errors.Is(err, ErrRouteResourceNotRouteable) {
		t.Fatalf("UpdateRoute() error = %v, want ErrRouteResourceNotRouteable", err)
	}
	if provider.calls != 1 {
		t.Fatalf("policy calls = %d, want 1", provider.calls)
	}
	assertRouteAmountAndTime(t, store, route.RouteID, 40, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

func TestDisableRouteDuplicateIsIdempotent(t *testing.T) {
	last := testRouteNow()
	firstDisableAt := last.Add(time.Hour)
	secondDisableAt := firstDisableAt.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)

	firstService := newTestRouteSettlementService(t, store, firstDisableAt, nil)
	first, err := firstService.DisableRoute(route.RouteID)
	if err != nil {
		t.Fatalf("DisableRoute(first) error = %v, want nil", err)
	}
	if !first.Changed || first.Settlement.AddedAmount != 40 {
		t.Fatalf("first Changed/added = %v/%d, want true/40", first.Changed, first.Settlement.AddedAmount)
	}

	secondService := newTestRouteSettlementService(t, store, secondDisableAt, nil)
	second, err := secondService.DisableRoute(route.RouteID)
	if err != nil {
		t.Fatalf("DisableRoute(second) error = %v, want nil", err)
	}
	if second.Changed {
		t.Fatal("second Changed = true, want false")
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, firstDisableAt)
	assertRouteEnabled(t, store, route.RouteID, false)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, firstDisableAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, firstDisableAt)
}

func TestDisableRouteDuplicateRequestDoesNotSettleTwice(t *testing.T) {
	last := testRouteNow()
	firstDisableAt := last.Add(time.Hour)
	duplicateAt := firstDisableAt.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)

	firstService := newTestRouteSettlementService(t, store, firstDisableAt, nil)
	first, err := firstService.DisableRouteForOwnerWithRequest("player-1", route.RouteID, "request-disable-duplicate")
	if err != nil {
		t.Fatalf("DisableRoute(first) error = %v, want nil", err)
	}
	if !first.Changed || first.Settlement.AddedAmount != 40 {
		t.Fatalf("first Changed/added = %v/%d, want true/40", first.Changed, first.Settlement.AddedAmount)
	}

	duplicateService := newTestRouteSettlementService(t, store, duplicateAt, nil)
	duplicate, err := duplicateService.DisableRouteForOwnerWithRequest("player-1", route.RouteID, "request-disable-duplicate")
	if err != nil {
		t.Fatalf("DisableRoute(duplicate) error = %v, want nil", err)
	}
	if duplicate.Changed || !duplicate.Settlement.RouteID.IsZero() {
		t.Fatalf("duplicate Changed/settlement = %v/%+v, want replay without mutation", duplicate.Changed, duplicate.Settlement)
	}
	if !duplicate.Route.UpdatedAt.Equal(first.Route.UpdatedAt) || duplicate.Route.Enabled {
		t.Fatalf("duplicate route = %+v, want first disabled route %+v", duplicate.Route, first.Route)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, firstDisableAt)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, firstDisableAt)
}

func TestUpdateRouteFutureTimestampSafe(t *testing.T) {
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
	provider := &fakeRoutePolicyProvider{policy: noLossRoutePolicy()}
	service := newTestRouteService(t, store, provider, now)
	input := validUpdateRouteInput()
	input.AmountPerHour = 80

	result, err := service.UpdateRoute(input)
	if err != nil {
		t.Fatalf("UpdateRoute() error = %v, want nil", err)
	}
	if !result.Settlement.NoOp || result.Settlement.ElapsedApplied != 0 {
		t.Fatalf("settlement NoOp/applied = %v/%s, want true/0", result.Settlement.NoOp, result.Settlement.ElapsedApplied)
	}
	if result.Route.AmountPerHour != 80 {
		t.Fatalf("AmountPerHour = %d, want 80", result.Route.AmountPerHour)
	}
	stored, ok, err := store.AutomationRoute(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("AutomationRoute(%q) ok = %v err = %v, want true nil", route.RouteID, ok, err)
	}
	if !stored.LastCalculatedAt.Equal(futureLast) {
		t.Fatalf("LastCalculatedAt = %s, want unchanged future %s", stored.LastCalculatedAt, futureLast)
	}
	if !stored.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %s, want update time %s", stored.UpdatedAt, now)
	}
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, futureLast)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, futureLast)
}
