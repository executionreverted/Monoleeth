package simulations

import (
	"fmt"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/production"
)

const (
	ReasonPlanetProduction economy.LedgerReason = "planet_production"
	ReasonRouteLoss        economy.LedgerReason = "route_loss"

	simulationRouteSourceMapID      production.RouteMapID = "simulation_route_map_1"
	simulationRouteDestinationMapID production.RouteMapID = "simulation_route_map_1"
)

// PlanetSettlementSimulationConfig tunes deterministic offline production
// settlement simulations.
type PlanetSettlementSimulationConfig struct {
	Planets         int
	OfflineDuration time.Duration
	StartTime       time.Time
}

// PlanetSettlementSimulationSummary reports deterministic offline production
// settlement totals and duplicate-settlement checks.
type PlanetSettlementSimulationSummary struct {
	Planets             int
	Settlements         int
	DuplicateNoOps      int
	TotalProducedItems  int64
	TotalConsumedInputs int64
	FinalIronOre        int64
	FinalRefinedAlloy   int64
	FlowSnapshot        observability.EconomyFlowSnapshot
	MetricSnapshot      observability.MetricSnapshot
}

// RouteSettlementSimulationConfig tunes deterministic route settlement
// simulations.
type RouteSettlementSimulationConfig struct {
	Routes             int
	SettlementDuration time.Duration
	StartTime          time.Time
}

// RouteSettlementSimulationSummary reports deterministic route settlement totals
// and duplicate-settlement checks.
type RouteSettlementSimulationSummary struct {
	Routes              int
	Settlements         int
	DuplicateNoOps      int
	TotalWanted         int64
	TotalTaken          int64
	TotalLost           int64
	TotalDelivered      int64
	TotalAdded          int64
	SourceRemaining     int64
	DestinationQuantity int64
	FlowSnapshot        observability.EconomyFlowSnapshot
	MetricSnapshot      observability.MetricSnapshot
}

type normalizedSettlementSimulationConfig struct {
	count    int
	duration time.Duration
	start    time.Time
}

// RunPlanetSettlementSimulation runs server-timed offline planet settlements
// against the Phase 09 production store and records economy flow observations.
func RunPlanetSettlementSimulation(config PlanetSettlementSimulationConfig) (PlanetSettlementSimulationSummary, error) {
	normalized, err := normalizeSettlementSimulationConfig(
		config.Planets,
		config.OfflineDuration,
		config.StartTime,
		production.DefaultMaxOfflineSettlementDuration,
	)
	if err != nil {
		return PlanetSettlementSimulationSummary{}, err
	}

	store := production.NewInMemoryStore()
	flows := observability.NewEconomyFlowAccumulator()
	metrics := observability.NewMetricRecorder()
	summary := PlanetSettlementSimulationSummary{Planets: normalized.count}
	settledAt := normalized.start.Add(normalized.duration)
	window := simulationSettlementWindow(normalized.start, settledAt)

	for index := 0; index < normalized.count; index++ {
		planetID := foundation.PlanetID(fmt.Sprintf("simulation_production_planet_%d", index+1))
		if err := initializeSimulationProductionPlanet(store, planetID, normalized.start, 1_000, 16); err != nil {
			return PlanetSettlementSimulationSummary{}, err
		}
		if err := addSimulationProductionBuilding(store, planetID, production.BuildingID(fmt.Sprintf("extractor_%d", index+1)), production.ProductionDefinitionIDIronExtractorL1, normalized.start); err != nil {
			return PlanetSettlementSimulationSummary{}, err
		}
		if err := addSimulationProductionBuilding(store, planetID, production.BuildingID(fmt.Sprintf("foundry_%d", index+1)), production.ProductionDefinitionIDAlloyFoundryL1, normalized.start); err != nil {
			return PlanetSettlementSimulationSummary{}, err
		}

		result, err := store.SettlePlanetProduction(planetID, settledAt)
		if err != nil {
			return PlanetSettlementSimulationSummary{}, err
		}
		if result.NoOp ||
			result.EnergyInsufficient ||
			result.StorageFull ||
			result.ElapsedApplied != normalized.duration ||
			len(result.ProducedItems) == 0 ||
			len(result.ConsumedInputs) == 0 {
			return PlanetSettlementSimulationSummary{}, fmt.Errorf("planet settlement %q result %+v: %w", planetID, result, ErrInvalidSimulationConfig)
		}
		if err := recordPlanetSettlementFlows(flows, result, window); err != nil {
			return PlanetSettlementSimulationSummary{}, err
		}
		if err := metrics.RecordPlanetSettlement("settled"); err != nil {
			return PlanetSettlementSimulationSummary{}, err
		}
		summary.Settlements++
		summary.TotalProducedItems += totalProductionDeltas(result.ProducedItems)
		summary.TotalConsumedInputs += totalProductionDeltas(result.ConsumedInputs)
		summary.FinalIronOre += result.After.Storage.QuantityOf("iron_ore")
		summary.FinalRefinedAlloy += result.After.Storage.QuantityOf("refined_alloy")

		duplicate, err := store.SettlePlanetProduction(planetID, settledAt)
		if err != nil {
			return PlanetSettlementSimulationSummary{}, err
		}
		if !duplicate.NoOp ||
			duplicate.ElapsedApplied != 0 ||
			len(duplicate.ProducedItems) != 0 ||
			len(duplicate.ConsumedInputs) != 0 ||
			duplicate.After.Storage.QuantityOf("iron_ore") != result.After.Storage.QuantityOf("iron_ore") ||
			duplicate.After.Storage.QuantityOf("refined_alloy") != result.After.Storage.QuantityOf("refined_alloy") {
			return PlanetSettlementSimulationSummary{}, fmt.Errorf("planet duplicate settlement %q result %+v: %w", planetID, duplicate, ErrInvalidSimulationConfig)
		}
		if err := metrics.RecordPlanetSettlement("noop"); err != nil {
			return PlanetSettlementSimulationSummary{}, err
		}
		summary.DuplicateNoOps++
	}

	if summary.Settlements != normalized.count || summary.DuplicateNoOps != normalized.count {
		return PlanetSettlementSimulationSummary{}, fmt.Errorf("planet settlement summary %+v: %w", summary, ErrInvalidSimulationConfig)
	}
	summary.FlowSnapshot = flows.Snapshot()
	summary.MetricSnapshot = metrics.Snapshot()
	return summary, nil
}

// RunRouteSettlementSimulation runs server-timed route settlements against the
// Phase 09 automation route service and records loss as an item sink.
func RunRouteSettlementSimulation(config RouteSettlementSimulationConfig) (RouteSettlementSimulationSummary, error) {
	normalized, err := normalizeSettlementSimulationConfig(
		config.Routes,
		config.SettlementDuration,
		config.StartTime,
		production.DefaultMaxRouteOfflineSettlementDuration,
	)
	if err != nil {
		return RouteSettlementSimulationSummary{}, err
	}

	store := production.NewInMemoryStore()
	clock := &simulationClock{now: normalized.start}
	flows := observability.NewEconomyFlowAccumulator()
	metrics := observability.NewMetricRecorder()
	service, err := production.NewAutomationRouteService(production.AutomationRouteServiceConfig{
		Store:      store,
		Clock:      clock,
		Policy:     simulationRoutePolicyProvider{policy: simulationRoutePolicy()},
		LossRoller: fixedRouteLossRoller{value: 0},
	})
	if err != nil {
		return RouteSettlementSimulationSummary{}, err
	}

	routeIDs := make([]foundation.RouteID, 0, normalized.count)
	for index := 0; index < normalized.count; index++ {
		sourcePlanetID := foundation.PlanetID(fmt.Sprintf("simulation_route_source_%d", index+1))
		destinationPlanetID := foundation.PlanetID(fmt.Sprintf("simulation_route_destination_%d", index+1))
		if err := saveSimulationPlanetStorage(store, sourcePlanetID, 100, []production.StoredItem{{ItemID: "refined_alloy", Quantity: 100}}, normalized.start); err != nil {
			return RouteSettlementSimulationSummary{}, err
		}
		if err := saveSimulationPlanetStorage(store, destinationPlanetID, 100, nil, normalized.start); err != nil {
			return RouteSettlementSimulationSummary{}, err
		}
		destination, err := production.NewPlanetRouteDestination(destinationPlanetID)
		if err != nil {
			return RouteSettlementSimulationSummary{}, err
		}
		routeID := foundation.RouteID(fmt.Sprintf("simulation_route_%d", index+1))
		if _, err := service.CreateRoute(production.CreateRouteInput{
			RouteID:        routeID,
			OwnerPlayerID:  "simulation_route_owner",
			SourcePlanetID: sourcePlanetID,
			Destination:    destination,
			ResourceItemID: "refined_alloy",
			AmountPerHour:  40,
		}); err != nil {
			return RouteSettlementSimulationSummary{}, err
		}
		routeIDs = append(routeIDs, routeID)
	}

	clock.Advance(normalized.duration)
	settledAt := normalized.start.Add(normalized.duration)
	window := simulationSettlementWindow(normalized.start, settledAt)
	summary := RouteSettlementSimulationSummary{Routes: normalized.count}
	for _, routeID := range routeIDs {
		result, err := service.SettleRoute(routeID)
		if err != nil {
			return RouteSettlementSimulationSummary{}, err
		}
		if result.NoOp ||
			!result.LossApplied ||
			result.ElapsedApplied != normalized.duration ||
			result.WantedAmount != result.TakenAmount ||
			result.TakenAmount != result.LostAmount+result.DeliveredAmount ||
			result.DeliveredAmount != result.AddedAmount ||
			result.LostAmount == 0 {
			return RouteSettlementSimulationSummary{}, fmt.Errorf("route settlement %q result %+v: %w", routeID, result, ErrInvalidSimulationConfig)
		}
		if err := recordRouteLossFlow(flows, result, window); err != nil {
			return RouteSettlementSimulationSummary{}, err
		}
		if err := metrics.RecordRouteSettlement("settled"); err != nil {
			return RouteSettlementSimulationSummary{}, err
		}
		summary.Settlements++
		summary.TotalWanted += result.WantedAmount
		summary.TotalTaken += result.TakenAmount
		summary.TotalLost += result.LostAmount
		summary.TotalDelivered += result.DeliveredAmount
		summary.TotalAdded += result.AddedAmount

		source, ok, err := store.PlanetStorage(result.BeforeRoute.SourcePlanetID)
		if err != nil || !ok {
			return RouteSettlementSimulationSummary{}, fmt.Errorf("source storage %q ok=%v err=%v: %w", result.BeforeRoute.SourcePlanetID, ok, err, ErrInvalidSimulationConfig)
		}
		destination, ok, err := store.PlanetStorage(foundation.PlanetID(result.BeforeRoute.Destination.ID))
		if err != nil || !ok {
			return RouteSettlementSimulationSummary{}, fmt.Errorf("destination storage %q ok=%v err=%v: %w", result.BeforeRoute.Destination.ID, ok, err, ErrInvalidSimulationConfig)
		}
		sourceQuantity := source.QuantityOf("refined_alloy")
		destinationQuantity := destination.QuantityOf("refined_alloy")
		summary.SourceRemaining += sourceQuantity
		summary.DestinationQuantity += destinationQuantity

		duplicate, err := service.SettleRoute(routeID)
		if err != nil {
			return RouteSettlementSimulationSummary{}, err
		}
		if !duplicate.NoOp ||
			duplicate.ElapsedApplied != 0 ||
			duplicate.TakenAmount != 0 ||
			duplicate.LostAmount != 0 ||
			duplicate.AddedAmount != 0 {
			return RouteSettlementSimulationSummary{}, fmt.Errorf("route duplicate settlement %q result %+v: %w", routeID, duplicate, ErrInvalidSimulationConfig)
		}
		sourceAfterDuplicate, ok, err := store.PlanetStorage(result.BeforeRoute.SourcePlanetID)
		if err != nil || !ok {
			return RouteSettlementSimulationSummary{}, fmt.Errorf("source duplicate storage %q ok=%v err=%v: %w", result.BeforeRoute.SourcePlanetID, ok, err, ErrInvalidSimulationConfig)
		}
		destinationAfterDuplicate, ok, err := store.PlanetStorage(foundation.PlanetID(result.BeforeRoute.Destination.ID))
		if err != nil || !ok {
			return RouteSettlementSimulationSummary{}, fmt.Errorf("destination duplicate storage %q ok=%v err=%v: %w", result.BeforeRoute.Destination.ID, ok, err, ErrInvalidSimulationConfig)
		}
		if sourceAfterDuplicate.QuantityOf("refined_alloy") != sourceQuantity ||
			destinationAfterDuplicate.QuantityOf("refined_alloy") != destinationQuantity {
			return RouteSettlementSimulationSummary{}, fmt.Errorf("route duplicate settlement %q mutated storage: %w", routeID, ErrInvalidSimulationConfig)
		}
		if err := metrics.RecordRouteSettlement("noop"); err != nil {
			return RouteSettlementSimulationSummary{}, err
		}
		summary.DuplicateNoOps++
	}

	if summary.Settlements != normalized.count ||
		summary.DuplicateNoOps != normalized.count ||
		summary.TotalTaken != summary.TotalLost+summary.TotalDelivered ||
		summary.TotalDelivered != summary.TotalAdded {
		return RouteSettlementSimulationSummary{}, fmt.Errorf("route settlement summary %+v: %w", summary, ErrInvalidSimulationConfig)
	}
	summary.FlowSnapshot = flows.Snapshot()
	summary.MetricSnapshot = metrics.Snapshot()
	return summary, nil
}

func normalizeSettlementSimulationConfig(
	count int,
	duration time.Duration,
	start time.Time,
	maxDuration time.Duration,
) (normalizedSettlementSimulationConfig, error) {
	normalized := normalizedSettlementSimulationConfig{
		count:    count,
		duration: duration,
		start:    start,
	}
	if normalized.count == 0 {
		normalized.count = 1
	}
	if normalized.duration == 0 {
		normalized.duration = time.Hour
	}
	if normalized.start.IsZero() {
		normalized.start = time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	}
	if normalized.count < 1 {
		return normalizedSettlementSimulationConfig{}, fmt.Errorf("count %d: %w", normalized.count, ErrInvalidSimulationConfig)
	}
	if normalized.duration <= 0 || normalized.duration > maxDuration {
		return normalizedSettlementSimulationConfig{}, fmt.Errorf("duration %s: %w", normalized.duration, ErrInvalidSimulationConfig)
	}
	return normalized, nil
}

func initializeSimulationProductionPlanet(
	store *production.InMemoryStore,
	planetID foundation.PlanetID,
	start time.Time,
	storageCapacity int64,
	energyCapacity int64,
) error {
	_, err := store.InitializePlanetProduction(production.InitializePlanetProductionInput{
		PlanetID:              planetID,
		LastCalculatedAt:      start,
		StorageCapacityUnits:  storageCapacity,
		EnergyCapacityPerHour: energyCapacity,
		UpdatedAt:             start,
	})
	return err
}

func addSimulationProductionBuilding(
	store *production.InMemoryStore,
	planetID foundation.PlanetID,
	buildingID production.BuildingID,
	definitionID catalog.DefinitionID,
	start time.Time,
) error {
	definition, err := production.MustMVPCatalog().MustGet(definitionID)
	if err != nil {
		return err
	}
	building, err := production.NewPlanetBuilding(buildingID, planetID, definition, production.BuildingStateActive, start, start)
	if err != nil {
		return err
	}
	_, _, err = store.UpsertBuilding(building)
	return err
}

func saveSimulationPlanetStorage(
	store *production.InMemoryStore,
	planetID foundation.PlanetID,
	capacity int64,
	items []production.StoredItem,
	updatedAt time.Time,
) error {
	storage, err := production.NewPlanetStorage(planetID, capacity, items, updatedAt)
	if err != nil {
		return err
	}
	return store.SavePlanetStorage(storage)
}

func recordPlanetSettlementFlows(
	accumulator *observability.EconomyFlowAccumulator,
	result production.PlanetProductionSettlementResult,
	window string,
) error {
	reference, err := foundation.OfflineSettlementIdempotencyKey(result.PlanetID, window)
	if err != nil {
		return err
	}
	for _, delta := range result.ProducedItems {
		if err := recordItemFlow(accumulator, delta.ItemID, delta.Quantity, ReasonPlanetProduction, reference, observability.ValueFlowDirectionFaucet, result.SettledAt); err != nil {
			return err
		}
	}
	for _, delta := range result.ConsumedInputs {
		if err := recordItemFlow(accumulator, delta.ItemID, delta.Quantity, ReasonPlanetProduction, reference, observability.ValueFlowDirectionSink, result.SettledAt); err != nil {
			return err
		}
	}
	return nil
}

func recordRouteLossFlow(
	accumulator *observability.EconomyFlowAccumulator,
	result production.RouteSettlementResult,
	window string,
) error {
	if result.LostAmount == 0 {
		return nil
	}
	reference, err := foundation.RouteSettlementIdempotencyKey(result.RouteID, window)
	if err != nil {
		return err
	}
	return recordItemFlow(accumulator, result.BeforeRoute.ResourceItemID, result.LostAmount, ReasonRouteLoss, reference, observability.ValueFlowDirectionSink, result.SettledAt)
}

func recordItemFlow(
	accumulator *observability.EconomyFlowAccumulator,
	itemID foundation.ItemID,
	quantity int64,
	reason economy.LedgerReason,
	reference foundation.IdempotencyKey,
	direction observability.ValueFlowDirection,
	timestamp time.Time,
) error {
	entry, err := observability.NewItemFlowEntry(itemID, quantity, reason, reference, direction, timestamp)
	if err != nil {
		return err
	}
	return accumulator.Record(entry)
}

func totalProductionDeltas(deltas []production.SettlementItemDelta) int64 {
	var total int64
	for _, delta := range deltas {
		total += delta.Quantity
	}
	return total
}

func simulationSettlementWindow(start time.Time, end time.Time) string {
	return start.UTC().Format("20060102T150405Z") + "_" + end.UTC().Format("20060102T150405Z")
}

type simulationRoutePolicyProvider struct {
	policy production.RouteCreatePolicy
}

func (provider simulationRoutePolicyProvider) RouteCreatePolicy(input production.RouteCreatePolicyInput) (production.RouteCreatePolicy, error) {
	if err := input.Validate(); err != nil {
		return production.RouteCreatePolicy{}, err
	}
	return provider.policy, nil
}

func simulationRoutePolicy() production.RouteCreatePolicy {
	return production.RouteCreatePolicy{
		SourcePlanetOwned:     true,
		DestinationAccessible: true,
		ResourceRouteable:     true,
		RequirementsMet:       true,
		SourceMapID:           simulationRouteSourceMapID,
		DestinationMapID:      simulationRouteDestinationMapID,
		DistanceUnits:         100,
		MaxDistanceUnits:      1_000,
		BaseLossChance:        production.MaxRouteLossChance,
		MinLossPercent:        0.25,
		MaxLossPercent:        0.25,
		EnergyCostPerHour:     12,
	}
}

type fixedRouteLossRoller struct {
	value float64
}

func (roller fixedRouteLossRoller) Float64() float64 {
	return roller.value
}
