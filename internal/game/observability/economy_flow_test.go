package observability

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestEconomyFlowAccumulatorRejectsDuplicateValueLineWithoutDoubleCounting(t *testing.T) {
	accumulator := NewEconomyFlowAccumulator()
	entry := mustCurrencyFlowEntry(t, economy.CurrencyBucketCredits, 100, "quest_reward", "quest_reward:quest-1", ValueFlowDirectionFaucet)

	if err := accumulator.Record(entry); err != nil {
		t.Fatalf("record first flow: %v", err)
	}
	if err := accumulator.Record(entry); !errors.Is(err, ErrDuplicateEconomyFlowReference) {
		t.Fatalf("duplicate error = %v, want %v", err, ErrDuplicateEconomyFlowReference)
	}

	snapshot := accumulator.Snapshot()
	if len(snapshot.CurrencyFaucets) != 1 {
		t.Fatalf("currency faucet summaries = %d, want 1", len(snapshot.CurrencyFaucets))
	}
	if got := snapshot.CurrencyFaucets[0].Total; got != 100 {
		t.Fatalf("duplicate mutated total = %d, want 100", got)
	}
}

func TestEconomyFlowAccumulatorAllowsSameReferenceForDifferentValueLines(t *testing.T) {
	accumulator := NewEconomyFlowAccumulator()
	referenceID := foundation.IdempotencyKey("quest_reward:quest-2")

	entries := []EconomyFlowEntry{
		mustCurrencyFlowEntryWithReference(t, economy.CurrencyBucketCredits, 10, "quest_reward", referenceID, ValueFlowDirectionFaucet),
		mustCurrencyFlowEntryWithReference(t, economy.CurrencyBucketPremiumEarned, 15, "quest_reward", referenceID, ValueFlowDirectionFaucet),
		mustCurrencyFlowEntryWithReference(t, economy.CurrencyBucketCredits, 20, "quest_reward", referenceID, ValueFlowDirectionSink),
		mustCurrencyFlowEntryWithReference(t, economy.CurrencyBucketCredits, 30, "daily_bonus", referenceID, ValueFlowDirectionFaucet),
		mustItemFlowEntryWithReference(t, foundation.ItemID("item-ore"), 40, "quest_reward", referenceID, ValueFlowDirectionFaucet),
		mustItemFlowEntryWithReference(t, foundation.ItemID("item-crystal"), 50, "quest_reward", referenceID, ValueFlowDirectionFaucet),
	}
	for i, entry := range entries {
		if err := accumulator.Record(entry); err != nil {
			t.Fatalf("record entry %d: %v", i, err)
		}
	}

	snapshot := accumulator.Snapshot()
	if len(snapshot.CurrencyFaucets) != 3 {
		t.Fatalf("currency faucets = %d, want 3: %#v", len(snapshot.CurrencyFaucets), snapshot.CurrencyFaucets)
	}
	if len(snapshot.CurrencySinks) != 1 {
		t.Fatalf("currency sinks = %d, want 1: %#v", len(snapshot.CurrencySinks), snapshot.CurrencySinks)
	}
	if len(snapshot.ItemFaucets) != 2 {
		t.Fatalf("item faucets = %d, want 2: %#v", len(snapshot.ItemFaucets), snapshot.ItemFaucets)
	}
}

func TestEconomyFlowSnapshotSeparatesCurrencyFaucetsAndSinksByCurrencyAndReason(t *testing.T) {
	accumulator := NewEconomyFlowAccumulator()
	entries := []EconomyFlowEntry{
		mustCurrencyFlowEntry(t, economy.CurrencyBucketCredits, 50, "quest_reward", "quest_reward:quest-3", ValueFlowDirectionFaucet),
		mustCurrencyFlowEntry(t, economy.CurrencyBucketCredits, 25, "quest_reward", "quest_reward:quest-4", ValueFlowDirectionFaucet),
		mustCurrencyFlowEntry(t, economy.CurrencyBucketCredits, 10, "market_fee", "market_fee:listing-1:player-1:request-1", ValueFlowDirectionSink),
		mustCurrencyFlowEntry(t, economy.CurrencyBucketPremiumEarned, 5, "daily_bonus", "quest_reward:quest-5", ValueFlowDirectionFaucet),
	}
	for _, entry := range entries {
		if err := accumulator.Record(entry); err != nil {
			t.Fatalf("record flow: %v", err)
		}
	}

	snapshot := accumulator.Snapshot()
	wantFaucets := []CurrencyFlowSummary{
		{Currency: economy.CurrencyBucketCredits, Reason: economy.LedgerReason("quest_reward"), Total: 75},
		{Currency: economy.CurrencyBucketPremiumEarned, Reason: economy.LedgerReason("daily_bonus"), Total: 5},
	}
	assertCurrencyFlowSummaries(t, snapshot.CurrencyFaucets, wantFaucets)
	assertCurrencyFlowSummaries(t, snapshot.CurrencySinks, []CurrencyFlowSummary{
		{Currency: economy.CurrencyBucketCredits, Reason: economy.LedgerReason("market_fee"), Total: 10},
	})
}

func TestEconomyFlowSnapshotSeparatesItemFaucetsAndSinksByItemAndReason(t *testing.T) {
	accumulator := NewEconomyFlowAccumulator()
	entries := []EconomyFlowEntry{
		mustItemFlowEntry(t, foundation.ItemID("item-ore"), 10, "loot_pickup", "loot_pickup:drop-1", ValueFlowDirectionFaucet),
		mustItemFlowEntry(t, foundation.ItemID("item-ore"), 15, "loot_pickup", "loot_pickup:drop-2", ValueFlowDirectionFaucet),
		mustItemFlowEntry(t, foundation.ItemID("item-ore"), 4, "craft_input", "craft_start:job-1", ValueFlowDirectionSink),
		mustItemFlowEntry(t, foundation.ItemID("item-crystal"), 2, "loot_pickup", "loot_pickup:drop-3", ValueFlowDirectionFaucet),
	}
	for _, entry := range entries {
		if err := accumulator.Record(entry); err != nil {
			t.Fatalf("record flow: %v", err)
		}
	}

	snapshot := accumulator.Snapshot()
	assertItemFlowSummaries(t, snapshot.ItemFaucets, []ItemFlowSummary{
		{ItemID: foundation.ItemID("item-crystal"), Reason: economy.LedgerReason("loot_pickup"), Total: 2},
		{ItemID: foundation.ItemID("item-ore"), Reason: economy.LedgerReason("loot_pickup"), Total: 25},
	})
	assertItemFlowSummaries(t, snapshot.ItemSinks, []ItemFlowSummary{
		{ItemID: foundation.ItemID("item-ore"), Reason: economy.LedgerReason("craft_input"), Total: 4},
	})
}

func TestEconomyFlowEntryValidateRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*EconomyFlowEntry)
		wantError error
	}{
		{
			name: "zero amount",
			mutate: func(entry *EconomyFlowEntry) {
				entry.Amount = 0
			},
			wantError: foundation.ErrNonPositiveAmount,
		},
		{
			name: "negative amount",
			mutate: func(entry *EconomyFlowEntry) {
				entry.Amount = -1
			},
			wantError: foundation.ErrNonPositiveAmount,
		},
		{
			name: "missing value identity",
			mutate: func(entry *EconomyFlowEntry) {
				entry.Currency = ""
			},
			wantError: ErrMissingEconomyFlowValueIdentity,
		},
		{
			name: "ambiguous value identity",
			mutate: func(entry *EconomyFlowEntry) {
				entry.ItemID = foundation.ItemID("item-ore")
			},
			wantError: ErrAmbiguousEconomyFlowValueIdentity,
		},
		{
			name: "missing reason",
			mutate: func(entry *EconomyFlowEntry) {
				entry.Reason = ""
			},
			wantError: ErrMissingEconomyFlowReason,
		},
		{
			name: "missing reference",
			mutate: func(entry *EconomyFlowEntry) {
				entry.ReferenceID = ""
			},
			wantError: ErrMissingEconomyFlowReference,
		},
		{
			name: "missing direction",
			mutate: func(entry *EconomyFlowEntry) {
				entry.Direction = ""
			},
			wantError: ErrInvalidValueFlowDirection,
		},
		{
			name: "invalid value kind",
			mutate: func(entry *EconomyFlowEntry) {
				entry.ValueKind = EconomyFlowValueKind("ship")
			},
			wantError: ErrInvalidEconomyFlowValueKind,
		},
		{
			name: "zero timestamp",
			mutate: func(entry *EconomyFlowEntry) {
				entry.Timestamp = time.Time{}
			},
			wantError: ErrMissingEconomyFlowTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := mustCurrencyFlowEntry(t, economy.CurrencyBucketCredits, 100, "quest_reward", "quest_reward:quest-6", ValueFlowDirectionFaucet)
			tt.mutate(&entry)

			err := entry.Validate()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("error = %v, want %v", err, tt.wantError)
			}
		})
	}
}

func TestEconomyFlowSnapshotIsSortedAndCloneSafe(t *testing.T) {
	accumulator := NewEconomyFlowAccumulator()
	entries := []EconomyFlowEntry{
		mustCurrencyFlowEntry(t, economy.CurrencyBucketPremiumPaid, 1, "daily_bonus", "quest_reward:quest-7", ValueFlowDirectionFaucet),
		mustCurrencyFlowEntry(t, economy.CurrencyBucketCredits, 1, "market_sale", "market_sale:listing-2:player-1:request-2", ValueFlowDirectionFaucet),
		mustCurrencyFlowEntry(t, economy.CurrencyBucketCredits, 1, "quest_reward", "quest_reward:quest-8", ValueFlowDirectionFaucet),
	}
	for _, entry := range entries {
		if err := accumulator.Record(entry); err != nil {
			t.Fatalf("record flow: %v", err)
		}
	}

	snapshot := accumulator.Snapshot()
	assertCurrencyFlowSummaries(t, snapshot.CurrencyFaucets, []CurrencyFlowSummary{
		{Currency: economy.CurrencyBucketCredits, Reason: economy.LedgerReason("market_sale"), Total: 1},
		{Currency: economy.CurrencyBucketCredits, Reason: economy.LedgerReason("quest_reward"), Total: 1},
		{Currency: economy.CurrencyBucketPremiumPaid, Reason: economy.LedgerReason("daily_bonus"), Total: 1},
	})

	snapshot.CurrencyFaucets[0].Total = 999
	next := accumulator.Snapshot()
	if next.CurrencyFaucets[0].Total != 1 {
		t.Fatalf("snapshot mutation changed stored total: got %d", next.CurrencyFaucets[0].Total)
	}
}

func mustCurrencyFlowEntry(
	t *testing.T,
	currency economy.CurrencyBucket,
	amount int64,
	reason economy.LedgerReason,
	reference string,
	direction ValueFlowDirection,
) EconomyFlowEntry {
	t.Helper()
	return mustCurrencyFlowEntryWithReference(t, currency, amount, reason, foundation.IdempotencyKey(reference), direction)
}

func mustCurrencyFlowEntryWithReference(
	t *testing.T,
	currency economy.CurrencyBucket,
	amount int64,
	reason economy.LedgerReason,
	reference foundation.IdempotencyKey,
	direction ValueFlowDirection,
) EconomyFlowEntry {
	t.Helper()
	entry, err := NewCurrencyFlowEntry(currency, amount, reason, reference, direction, validFlowTime())
	if err != nil {
		t.Fatalf("new currency flow entry: %v", err)
	}
	return entry
}

func mustItemFlowEntry(
	t *testing.T,
	itemID foundation.ItemID,
	amount int64,
	reason economy.LedgerReason,
	reference string,
	direction ValueFlowDirection,
) EconomyFlowEntry {
	t.Helper()
	return mustItemFlowEntryWithReference(t, itemID, amount, reason, foundation.IdempotencyKey(reference), direction)
}

func mustItemFlowEntryWithReference(
	t *testing.T,
	itemID foundation.ItemID,
	amount int64,
	reason economy.LedgerReason,
	reference foundation.IdempotencyKey,
	direction ValueFlowDirection,
) EconomyFlowEntry {
	t.Helper()
	entry, err := NewItemFlowEntry(itemID, amount, reason, reference, direction, validFlowTime())
	if err != nil {
		t.Fatalf("new item flow entry: %v", err)
	}
	return entry
}

func validFlowTime() time.Time {
	return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
}

func assertCurrencyFlowSummaries(t *testing.T, got, want []CurrencyFlowSummary) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("currency summaries length = %d, want %d: got %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("currency summary[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func assertItemFlowSummaries(t *testing.T, got, want []ItemFlowSummary) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("item summaries length = %d, want %d: got %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item summary[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
