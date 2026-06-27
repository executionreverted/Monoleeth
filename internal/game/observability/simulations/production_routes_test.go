package simulations_test

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/observability/simulations"
	"gameproject/internal/game/production"
)

func TestPlanetSettlementSimulationTracksOfflineProductionAndDuplicateNoOps(t *testing.T) {
	summary, err := simulations.RunPlanetSettlementSimulation(simulations.PlanetSettlementSimulationConfig{
		Planets:         4,
		OfflineDuration: time.Hour,
		StartTime:       time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunPlanetSettlementSimulation() error = %v", err)
	}

	if summary.Planets != 4 || summary.Settlements != 4 || summary.DuplicateNoOps != 4 {
		t.Fatalf(
			"planets/settlements/noops = %d/%d/%d, want 4/4/4",
			summary.Planets,
			summary.Settlements,
			summary.DuplicateNoOps,
		)
	}
	if summary.TotalProducedItems != 160 || summary.TotalConsumedInputs != 120 {
		t.Fatalf(
			"produced/consumed = %d/%d, want 160/120",
			summary.TotalProducedItems,
			summary.TotalConsumedInputs,
		)
	}
	if summary.FinalIronOre != 0 || summary.FinalRefinedAlloy != 40 {
		t.Fatalf("final iron/alloy = %d/%d, want 0/40", summary.FinalIronOre, summary.FinalRefinedAlloy)
	}

	assertItemFlow(t, summary.FlowSnapshot.ItemFaucets, "iron_ore", simulations.ReasonPlanetProduction, 120)
	assertItemFlow(t, summary.FlowSnapshot.ItemFaucets, "refined_alloy", simulations.ReasonPlanetProduction, 40)
	assertItemFlow(t, summary.FlowSnapshot.ItemSinks, "iron_ore", simulations.ReasonPlanetProduction, 120)
	if len(summary.FlowSnapshot.CurrencyFaucets) != 0 || len(summary.FlowSnapshot.CurrencySinks) != 0 {
		t.Fatalf("currency flows = %+v / %+v, want none", summary.FlowSnapshot.CurrencyFaucets, summary.FlowSnapshot.CurrencySinks)
	}
	if got := sumCounter(summary.MetricSnapshot, observability.MetricPlanetSettlements); got != 8 {
		t.Fatalf("planet settlement counter = %d, want 8", got)
	}
}

func TestEconomySimulationReportsBalancedSourceSinkForOneScenario(t *testing.T) {
	summary, err := simulations.RunPlanetSettlementSimulation(simulations.PlanetSettlementSimulationConfig{
		Planets:         1,
		OfflineDuration: time.Hour,
		StartTime:       time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunPlanetSettlementSimulation() error = %v", err)
	}

	source := itemFlowTotal(summary.FlowSnapshot.ItemFaucets, "iron_ore", simulations.ReasonPlanetProduction)
	sink := itemFlowTotal(summary.FlowSnapshot.ItemSinks, "iron_ore", simulations.ReasonPlanetProduction)
	if source == 0 || source != sink {
		t.Fatalf("iron_ore source/sink = %d/%d, want nonzero balanced flow", source, sink)
	}
}

func TestPhase13ProductionSettlementSimulationSummaryIsDeterministic(t *testing.T) {
	config := simulations.PlanetSettlementSimulationConfig{
		Planets:         2,
		OfflineDuration: time.Hour,
		StartTime:       time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	}
	first, err := simulations.RunPlanetSettlementSimulation(config)
	if err != nil {
		t.Fatalf("first RunPlanetSettlementSimulation() error = %v", err)
	}
	second, err := simulations.RunPlanetSettlementSimulation(config)
	if err != nil {
		t.Fatalf("second RunPlanetSettlementSimulation() error = %v", err)
	}

	if got, want := summarizePlanetSettlementSimulation(first), summarizePlanetSettlementSimulation(second); got != want {
		t.Fatalf("production simulation digest = %+v, want %+v", got, want)
	}
}

func TestRouteSettlementSimulationTracksLossAndDuplicateNoOps(t *testing.T) {
	summary, err := simulations.RunRouteSettlementSimulation(simulations.RouteSettlementSimulationConfig{
		Routes:             4,
		SettlementDuration: time.Hour,
		StartTime:          time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunRouteSettlementSimulation() error = %v", err)
	}

	if summary.Routes != 4 || summary.Settlements != 4 || summary.DuplicateNoOps != 4 {
		t.Fatalf(
			"routes/settlements/noops = %d/%d/%d, want 4/4/4",
			summary.Routes,
			summary.Settlements,
			summary.DuplicateNoOps,
		)
	}
	if summary.TotalWanted != 160 || summary.TotalTaken != 160 {
		t.Fatalf("wanted/taken = %d/%d, want 160/160", summary.TotalWanted, summary.TotalTaken)
	}
	if summary.TotalLost != 40 || summary.TotalDelivered != 120 || summary.TotalAdded != 120 {
		t.Fatalf(
			"lost/delivered/added = %d/%d/%d, want 40/120/120",
			summary.TotalLost,
			summary.TotalDelivered,
			summary.TotalAdded,
		)
	}
	if summary.SourceRemaining != 240 || summary.DestinationQuantity != 120 {
		t.Fatalf(
			"source/destination quantity = %d/%d, want 240/120",
			summary.SourceRemaining,
			summary.DestinationQuantity,
		)
	}

	assertItemFlow(t, summary.FlowSnapshot.ItemSinks, "refined_alloy", simulations.ReasonRouteLoss, 40)
	if len(summary.FlowSnapshot.ItemFaucets) != 0 {
		t.Fatalf("item faucets = %+v, want none for route transfer simulation", summary.FlowSnapshot.ItemFaucets)
	}
	if len(summary.FlowSnapshot.CurrencyFaucets) != 0 || len(summary.FlowSnapshot.CurrencySinks) != 0 {
		t.Fatalf("currency flows = %+v / %+v, want none", summary.FlowSnapshot.CurrencyFaucets, summary.FlowSnapshot.CurrencySinks)
	}
	if got := sumCounter(summary.MetricSnapshot, observability.MetricRouteSettlements); got != 8 {
		t.Fatalf("route settlement counter = %d, want 8", got)
	}
}

func TestPhase13RouteSettlementSimulationSummaryIsDeterministic(t *testing.T) {
	config := simulations.RouteSettlementSimulationConfig{
		Routes:             2,
		SettlementDuration: time.Hour,
		StartTime:          time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	}
	first, err := simulations.RunRouteSettlementSimulation(config)
	if err != nil {
		t.Fatalf("first RunRouteSettlementSimulation() error = %v", err)
	}
	second, err := simulations.RunRouteSettlementSimulation(config)
	if err != nil {
		t.Fatalf("second RunRouteSettlementSimulation() error = %v", err)
	}

	if got, want := summarizeRouteSettlementSimulation(first), summarizeRouteSettlementSimulation(second); got != want {
		t.Fatalf("route simulation digest = %+v, want %+v", got, want)
	}
}

func itemFlowTotal(flows []observability.ItemFlowSummary, itemID string, reason economy.LedgerReason) int64 {
	var total int64
	for _, flow := range flows {
		if flow.ItemID.String() == itemID && flow.Reason == reason {
			total += flow.Total
		}
	}
	return total
}

type planetSettlementSimulationDigest struct {
	Planets             int
	Settlements         int
	DuplicateNoOps      int
	TotalProducedItems  int64
	TotalConsumedInputs int64
	FinalIronOre        int64
	FinalRefinedAlloy   int64
	IronOreSource       int64
	IronOreSink         int64
}

func summarizePlanetSettlementSimulation(summary simulations.PlanetSettlementSimulationSummary) planetSettlementSimulationDigest {
	return planetSettlementSimulationDigest{
		Planets:             summary.Planets,
		Settlements:         summary.Settlements,
		DuplicateNoOps:      summary.DuplicateNoOps,
		TotalProducedItems:  summary.TotalProducedItems,
		TotalConsumedInputs: summary.TotalConsumedInputs,
		FinalIronOre:        summary.FinalIronOre,
		FinalRefinedAlloy:   summary.FinalRefinedAlloy,
		IronOreSource:       itemFlowTotal(summary.FlowSnapshot.ItemFaucets, "iron_ore", simulations.ReasonPlanetProduction),
		IronOreSink:         itemFlowTotal(summary.FlowSnapshot.ItemSinks, "iron_ore", simulations.ReasonPlanetProduction),
	}
}

type routeSettlementSimulationDigest struct {
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
	RouteLossSink       int64
}

func summarizeRouteSettlementSimulation(summary simulations.RouteSettlementSimulationSummary) routeSettlementSimulationDigest {
	return routeSettlementSimulationDigest{
		Routes:              summary.Routes,
		Settlements:         summary.Settlements,
		DuplicateNoOps:      summary.DuplicateNoOps,
		TotalWanted:         summary.TotalWanted,
		TotalTaken:          summary.TotalTaken,
		TotalLost:           summary.TotalLost,
		TotalDelivered:      summary.TotalDelivered,
		TotalAdded:          summary.TotalAdded,
		SourceRemaining:     summary.SourceRemaining,
		DestinationQuantity: summary.DestinationQuantity,
		RouteLossSink:       itemFlowTotal(summary.FlowSnapshot.ItemSinks, "refined_alloy", simulations.ReasonRouteLoss),
	}
}

func TestProductionRouteSettlementSimulationsRejectInvalidConfig(t *testing.T) {
	_, err := simulations.RunPlanetSettlementSimulation(simulations.PlanetSettlementSimulationConfig{Planets: -1})
	if !errors.Is(err, simulations.ErrInvalidSimulationConfig) {
		t.Fatalf("RunPlanetSettlementSimulation() count error = %v, want ErrInvalidSimulationConfig", err)
	}

	_, err = simulations.RunPlanetSettlementSimulation(simulations.PlanetSettlementSimulationConfig{
		OfflineDuration: production.DefaultMaxOfflineSettlementDuration + time.Second,
	})
	if !errors.Is(err, simulations.ErrInvalidSimulationConfig) {
		t.Fatalf("RunPlanetSettlementSimulation() duration error = %v, want ErrInvalidSimulationConfig", err)
	}

	_, err = simulations.RunRouteSettlementSimulation(simulations.RouteSettlementSimulationConfig{Routes: -1})
	if !errors.Is(err, simulations.ErrInvalidSimulationConfig) {
		t.Fatalf("RunRouteSettlementSimulation() count error = %v, want ErrInvalidSimulationConfig", err)
	}

	_, err = simulations.RunRouteSettlementSimulation(simulations.RouteSettlementSimulationConfig{
		SettlementDuration: -time.Hour,
	})
	if !errors.Is(err, simulations.ErrInvalidSimulationConfig) {
		t.Fatalf("RunRouteSettlementSimulation() duration error = %v, want ErrInvalidSimulationConfig", err)
	}

	_, err = simulations.RunRouteSettlementSimulation(simulations.RouteSettlementSimulationConfig{
		SettlementDuration: production.DefaultMaxRouteOfflineSettlementDuration + time.Second,
	})
	if !errors.Is(err, simulations.ErrInvalidSimulationConfig) {
		t.Fatalf("RunRouteSettlementSimulation() max duration error = %v, want ErrInvalidSimulationConfig", err)
	}
}
