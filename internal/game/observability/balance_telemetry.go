package observability

import (
	"sort"

	"gameproject/internal/game/economy"
)

// BalanceTelemetryCategory labels a ledger reason as an economy source (value
// entering circulation) or sink (value leaving circulation).
type BalanceTelemetryCategory string

const (
	BalanceCategorySource  BalanceTelemetryCategory = "source"
	BalanceCategorySink    BalanceTelemetryCategory = "sink"
	BalanceCategoryNeutral BalanceTelemetryCategory = "neutral"
)

const (
	MetricEconomyBalanceByReason = "economy_balance_by_reason"
)

// BalanceTelemetrySummary aggregates net credit flow per ledger reason so the
// release gate can prove credits are neither created nor destroyed silently.
type BalanceTelemetrySummary struct {
	Reasons []BalanceTelemetryEntry `json:"reasons"`
	NetFlow int64                   `json:"net_flow"`
}

type BalanceTelemetryEntry struct {
	Reason   economy.LedgerReason     `json:"reason"`
	Category BalanceTelemetryCategory `json:"category"`
	Credits  int64                    `json:"credits"`
	Entries  int64                    `json:"entries"`
}

// CategorizeLedgerReason classifies a ledger reason as an economy source, sink,
// or neutral transfer. Sources inject credits into circulation (rewards, seed,
// refunds from the house); sinks remove credits (purchases, fees, repairs);
// neutral reasons move value between player-owned buckets without net change.
func CategorizeLedgerReason(reason economy.LedgerReason) BalanceTelemetryCategory {
	switch reason {
	case "quest_reward",
		"runtime_seed",
		"playtest_seed",
		"e2e_planet_claim_seed",
		"admin_compensation",
		"loot_pickup",
		"premium_claim",
		"planet_production_settled",
		"route_settlement",
		"market_sale",
		"auction_refund":
		return BalanceCategorySource
	case "shop_purchase",
		"repair_cost",
		"market_fee",
		"market_buy",
		"auction_bid",
		"auction_buy_now",
		"craft_start",
		"planet_building_build",
		"planet_building_upgrade",
		"coordinate_item_create",
		"coordinate_item_use":
		return BalanceCategorySink
	default:
		return BalanceCategoryNeutral
	}
}

// SummarizeBalanceTelemetry folds a stream of currency ledger entries into a
// per-reason source/sink summary with a signed net flow. The net flow is
// positive when the window created more credits than it destroyed.
func SummarizeBalanceTelemetry(entries []economy.CurrencyLedgerEntry) BalanceTelemetrySummary {
	byReason := make(map[economy.LedgerReason]*BalanceTelemetryEntry)
	var netFlow int64
	for index := range entries {
		entry := entries[index]
		summary, ok := byReason[entry.Reason]
		if !ok {
			summary = &BalanceTelemetryEntry{
				Reason:   entry.Reason,
				Category: CategorizeLedgerReason(entry.Reason),
			}
			byReason[entry.Reason] = summary
		}
		amount := entry.Amount.Int64()
		if entry.Action == economy.LedgerActionDecrease {
			amount = -amount
		}
		summary.Credits += amount
		summary.Entries++
		netFlow += amount
	}
	reasons := make([]BalanceTelemetryEntry, 0, len(byReason))
	for _, summary := range byReason {
		reasons = append(reasons, *summary)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if reasons[i].Category != reasons[j].Category {
			return reasons[i].Category < reasons[j].Category
		}
		return reasons[i].Reason < reasons[j].Reason
	})
	return BalanceTelemetrySummary{Reasons: reasons, NetFlow: netFlow}
}
