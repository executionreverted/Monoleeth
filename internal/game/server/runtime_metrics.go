package server

import (
	"gameproject/internal/game/economy"
	"gameproject/internal/game/quests"
)

func (runtime *Runtime) recordCurrencyLedgerMetric(entry economy.CurrencyLedgerEntry) {
	if runtime == nil || runtime.Metrics == nil || entry.LedgerID.IsZero() {
		return
	}
	_ = runtime.Metrics.RecordWalletDelta(
		entry.Reason.String(),
		entry.Currency.String(),
		entry.Action.String(),
		entry.Amount.Int64(),
	)
}

func (runtime *Runtime) recordItemLedgerMetrics(entries []economy.ItemLedgerEntry) {
	if runtime == nil || runtime.Metrics == nil {
		return
	}
	for _, entry := range entries {
		if entry.LedgerID.IsZero() {
			continue
		}
		_ = runtime.Metrics.RecordItemDelta(
			entry.Reason.String(),
			entry.ItemID,
			entry.Action.String(),
			entry.Quantity.Int64(),
		)
	}
}

func (runtime *Runtime) recordQuestRewardMetrics(result quests.ClaimRewardResult) {
	if runtime == nil || runtime.Metrics == nil || result.Duplicate {
		return
	}
	for _, grant := range result.Quest.RewardPayload.Grants {
		_ = runtime.Metrics.RecordQuestReward(grant.Kind.String())
	}
	itemReason := runtimeQuestRewardLedgerReason
	if result.Credits != nil {
		itemReason = result.Credits.LedgerEntry.Reason
		runtime.recordCurrencyLedgerMetric(result.Credits.LedgerEntry)
	}
	if result.Items != nil {
		for _, item := range result.Items.Items {
			_ = runtime.Metrics.RecordItemDelta(itemReason.String(), item.ItemID, economy.LedgerActionIncrease.String(), item.Quantity)
		}
	}
}
