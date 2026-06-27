package simulations_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/observability/simulations"
)

func TestCombatLootSimulationIsDeterministicAcrossRuns(t *testing.T) {
	config := simulations.CombatLootSimulationConfig{
		Kills:                    4,
		ConcurrentPickupAttempts: 8,
		DropQuantity:             2,
		StartTime:                time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	}
	first, err := simulations.RunCombatLootSimulation(config)
	if err != nil {
		t.Fatalf("first RunCombatLootSimulation() error = %v", err)
	}
	second, err := simulations.RunCombatLootSimulation(config)
	if err != nil {
		t.Fatalf("second RunCombatLootSimulation() error = %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("simulation summaries differ\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func TestCombatLootSimulationTracksNPCDeathsAndConcurrentPickups(t *testing.T) {
	summary, err := simulations.RunCombatLootSimulation(simulations.CombatLootSimulationConfig{
		Kills:                    5,
		ConcurrentPickupAttempts: 12,
		DropQuantity:             3,
		StartTime:                time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunCombatLootSimulation() error = %v", err)
	}

	if summary.KillsCompleted != 5 || summary.AttacksResolved != 5 {
		t.Fatalf("kills/attacks = %d/%d, want 5/5", summary.KillsCompleted, summary.AttacksResolved)
	}
	if summary.DropsCreated != 5 || summary.DuplicateDeathAttempts != 5 {
		t.Fatalf("drops/duplicate deaths = %d/%d, want 5/5", summary.DropsCreated, summary.DuplicateDeathAttempts)
	}
	if summary.PickupAttempts != 60 || summary.PickupSuccesses != 5 || summary.PickupClaimedFailures != 55 {
		t.Fatalf(
			"pickup attempts/successes/claimed = %d/%d/%d, want 60/5/55",
			summary.PickupAttempts,
			summary.PickupSuccesses,
			summary.PickupClaimedFailures,
		)
	}
	if summary.CargoQuantity != 15 {
		t.Fatalf("CargoQuantity = %d, want 15", summary.CargoQuantity)
	}

	assertItemFlow(t, summary.FlowSnapshot.ItemFaucets, "raw_ore", simulations.ReasonLootCreated, 15)
	if len(summary.FlowSnapshot.ItemSinks) != 0 {
		t.Fatalf("ItemSinks = %+v, want none for cargo transfer simulation", summary.FlowSnapshot.ItemSinks)
	}
	if got := sumCounter(summary.MetricSnapshot, observability.MetricCombatActionsPerSecond); got != 5 {
		t.Fatalf("combat action counter = %d, want 5", got)
	}
	if got := sumCounter(summary.MetricSnapshot, observability.MetricLootCreatedPerSecond); got != 15 {
		t.Fatalf("loot created counter = %d, want 15", got)
	}
	if got := sumCounter(summary.MetricSnapshot, observability.MetricLootPickedPerSecond); got != 15 {
		t.Fatalf("loot picked counter = %d, want 15", got)
	}
}

func TestCombatLootSimulationRejectsInvalidConfig(t *testing.T) {
	_, err := simulations.RunCombatLootSimulation(simulations.CombatLootSimulationConfig{
		Kills: -1,
	})
	if !errors.Is(err, simulations.ErrInvalidSimulationConfig) {
		t.Fatalf("RunCombatLootSimulation() error = %v, want ErrInvalidSimulationConfig", err)
	}

	_, err = simulations.RunCombatLootSimulation(simulations.CombatLootSimulationConfig{
		ConcurrentPickupAttempts: -1,
	})
	if !errors.Is(err, simulations.ErrInvalidSimulationConfig) {
		t.Fatalf("RunCombatLootSimulation() pickup error = %v, want ErrInvalidSimulationConfig", err)
	}

	_, err = simulations.RunCombatLootSimulation(simulations.CombatLootSimulationConfig{
		DropQuantity: -1,
	})
	if !errors.Is(err, simulations.ErrInvalidSimulationConfig) {
		t.Fatalf("RunCombatLootSimulation() quantity error = %v, want ErrInvalidSimulationConfig", err)
	}
}

func assertItemFlow(t *testing.T, flows []observability.ItemFlowSummary, itemID string, reason economy.LedgerReason, wantTotal int64) {
	t.Helper()
	for _, flow := range flows {
		if flow.ItemID.String() == itemID && flow.Reason == reason {
			if flow.Total != wantTotal {
				t.Fatalf("item flow %+v total = %d, want %d", flow, flow.Total, wantTotal)
			}
			return
		}
	}
	t.Fatalf("missing item flow item_id=%q reason=%q in %+v", itemID, reason, flows)
}

func sumCounter(snapshot observability.MetricSnapshot, name string) int64 {
	var total int64
	for _, counter := range snapshot.Counters {
		if counter.Name == name {
			total += counter.Value
		}
	}
	return total
}
