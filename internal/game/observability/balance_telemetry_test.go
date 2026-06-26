package observability

import (
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func mustTestMoney(t *testing.T, amount int64) foundation.Money {
	t.Helper()
	money, err := foundation.NewMoney(amount)
	if err != nil {
		t.Fatalf("NewMoney(%d) error = %v", amount, err)
	}
	return money
}

func TestCategorizeLedgerReasonClassifiesKnownSourceSinkAndNeutral(t *testing.T) {
	sources := []economy.LedgerReason{"quest_reward", "loot_pickup", "premium_claim", "market_sale", "planet_production_settled"}
	for _, reason := range sources {
		if got := CategorizeLedgerReason(reason); got != BalanceCategorySource {
			t.Fatalf("CategorizeLedgerReason(%q) = %q, want source", reason, got)
		}
	}
	sinks := []economy.LedgerReason{"shop_purchase", "repair_cost", "market_fee", "craft_start", "planet_building_build"}
	for _, reason := range sinks {
		if got := CategorizeLedgerReason(reason); got != BalanceCategorySink {
			t.Fatalf("CategorizeLedgerReason(%q) = %q, want sink", reason, got)
		}
	}
	if got := CategorizeLedgerReason("inventory_move"); got != BalanceCategoryNeutral {
		t.Fatalf("CategorizeLedgerReason(inventory_move) = %q, want neutral", got)
	}
}

func TestSummarizeBalanceTelemetryComputesNetFlowPerReason(t *testing.T) {
	entries := []economy.CurrencyLedgerEntry{
		{LedgerID: "l1", PlayerID: foundation.PlayerID("p1"), Currency: economy.CurrencyBucketCredits, Amount: mustTestMoney(t, 100), Action: economy.LedgerActionIncrease, Reason: "quest_reward", CreatedAt: time.Now()},
		{LedgerID: "l2", PlayerID: foundation.PlayerID("p1"), Currency: economy.CurrencyBucketCredits, Amount: mustTestMoney(t, 30), Action: economy.LedgerActionDecrease, Reason: "shop_purchase", CreatedAt: time.Now()},
		{LedgerID: "l3", PlayerID: foundation.PlayerID("p1"), Currency: economy.CurrencyBucketCredits, Amount: mustTestMoney(t, 50), Action: economy.LedgerActionIncrease, Reason: "quest_reward", CreatedAt: time.Now()},
	}
	summary := SummarizeBalanceTelemetry(entries)
	if summary.NetFlow != 120 {
		t.Fatalf("net flow = %d, want 120 (150 in - 30 out)", summary.NetFlow)
	}
	byReason := make(map[economy.LedgerReason]BalanceTelemetryEntry, len(summary.Reasons))
	for _, entry := range summary.Reasons {
		byReason[entry.Reason] = entry
	}
	if quest := byReason["quest_reward"]; quest.Credits != 150 || quest.Entries != 2 || quest.Category != BalanceCategorySource {
		t.Fatalf("quest_reward entry = %+v, want credits 150 entries 2 source", quest)
	}
	if shop := byReason["shop_purchase"]; shop.Credits != -30 || shop.Entries != 1 || shop.Category != BalanceCategorySink {
		t.Fatalf("shop_purchase entry = %+v, want credits -30 entries 1 sink", shop)
	}
}
