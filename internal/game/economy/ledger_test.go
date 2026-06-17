package economy

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestLedgerEntriesRejectZeroAndNegativeAmounts(t *testing.T) {
	currencyEntry := validCurrencyLedgerEntry(t)
	currencyEntry.Amount = foundation.Money{}
	if err := currencyEntry.Validate(); !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("zero currency amount error = %v, want foundation.ErrNonPositiveAmount", err)
	}
	if _, err := foundation.NewMoney(-1); !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("negative money constructor error = %v, want foundation.ErrNonPositiveAmount", err)
	}

	itemEntry := validItemLedgerEntry(t)
	itemEntry.Quantity = foundation.Quantity{}
	if err := itemEntry.Validate(); !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("zero item quantity error = %v, want foundation.ErrNonPositiveAmount", err)
	}
	if _, err := foundation.NewQuantity(-1); !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("negative quantity constructor error = %v, want foundation.ErrNonPositiveAmount", err)
	}
}

func TestLedgerEntriesRejectBlankIDsAndReferences(t *testing.T) {
	currencyEntry := validCurrencyLedgerEntry(t)
	currencyEntry.LedgerID = ""
	if err := currencyEntry.Validate(); !errors.Is(err, ErrEmptyLedgerID) {
		t.Fatalf("blank currency ledger id error = %v, want ErrEmptyLedgerID", err)
	}

	currencyEntry = validCurrencyLedgerEntry(t)
	currencyEntry.PlayerID = ""
	if err := currencyEntry.Validate(); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank currency player id error = %v, want foundation.ErrEmptyID", err)
	}

	itemEntry := validItemLedgerEntry(t)
	itemEntry.ItemID = ""
	if err := itemEntry.Validate(); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank item id error = %v, want foundation.ErrEmptyID", err)
	}

	itemEntry = validItemLedgerEntry(t)
	itemEntry.ReferenceKey = ""
	if err := itemEntry.Validate(); !errors.Is(err, foundation.ErrEmptyIdempotencyKey) {
		t.Fatalf("blank item reference key error = %v, want foundation.ErrEmptyIdempotencyKey", err)
	}
}

func TestLedgerEntriesRequireReasonAndIdempotencyReference(t *testing.T) {
	currencyEntry := validCurrencyLedgerEntry(t)
	currencyEntry.Reason = " "
	if err := currencyEntry.Validate(); !errors.Is(err, ErrEmptyLedgerReason) {
		t.Fatalf("blank currency reason error = %v, want ErrEmptyLedgerReason", err)
	}

	currencyEntry = validCurrencyLedgerEntry(t)
	currencyEntry.ReferenceKey = ""
	if err := currencyEntry.Validate(); !errors.Is(err, foundation.ErrEmptyIdempotencyKey) {
		t.Fatalf("blank currency reference key error = %v, want foundation.ErrEmptyIdempotencyKey", err)
	}

	itemEntry := validItemLedgerEntry(t)
	itemEntry.Reason = ""
	if err := itemEntry.Validate(); !errors.Is(err, ErrEmptyLedgerReason) {
		t.Fatalf("blank item reason error = %v, want ErrEmptyLedgerReason", err)
	}
}

func TestLedgerEntriesRejectInvalidActionAndNegativeBalanceAfter(t *testing.T) {
	currencyEntry := validCurrencyLedgerEntry(t)
	currencyEntry.Action = LedgerAction("noop")
	if err := currencyEntry.Validate(); !errors.Is(err, ErrInvalidLedgerAction) {
		t.Fatalf("invalid action error = %v, want ErrInvalidLedgerAction", err)
	}

	itemEntry := validItemLedgerEntry(t)
	itemEntry.BalanceAfter = -1
	if err := itemEntry.Validate(); !errors.Is(err, ErrNegativeBalance) {
		t.Fatalf("negative item balance after error = %v, want ErrNegativeBalance", err)
	}
}

func TestCurrencyLedgerJSONAndStringBehaviorIsStable(t *testing.T) {
	entry := validCurrencyLedgerEntry(t)
	entry.CreatedAt = time.Date(2026, 6, 17, 14, 15, 0, 0, time.UTC)

	payload, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json marshal currency ledger: %v", err)
	}
	want := `{"ledger_id":"ledger-1","player_id":"player-1","currency_type":"credits","amount":100,"action":"increase","balance_after":1000,"reason":"quest_reward","reference_id":"quest_reward:quest-1","created_at":"2026-06-17T14:15:00Z"}`
	if got := string(payload); got != want {
		t.Fatalf("currency ledger JSON = %s, want %s", got, want)
	}

	if got := LedgerActionIncrease.String(); got != "increase" {
		t.Fatalf("LedgerAction.String() = %q, want increase", got)
	}
	if got := LedgerReason("quest_reward").String(); got != "quest_reward" {
		t.Fatalf("LedgerReason.String() = %q, want quest_reward", got)
	}
}

func validCurrencyLedgerEntry(t *testing.T) CurrencyLedgerEntry {
	t.Helper()

	amount := validMoney(t, 100)
	referenceKey := validReferenceKey(t, "quest_reward:quest-1")
	entry, err := NewCurrencyLedgerEntry(
		"ledger-1",
		"player-1",
		CurrencyBucketCredits,
		amount,
		LedgerActionIncrease,
		1000,
		"quest_reward",
		referenceKey,
	)
	if err != nil {
		t.Fatalf("NewCurrencyLedgerEntry valid value: %v", err)
	}
	return entry
}

func validItemLedgerEntry(t *testing.T) ItemLedgerEntry {
	t.Helper()

	quantity := validQuantity(t, 5)
	location := validLocation(t)
	referenceKey := validReferenceKey(t, "loot_pickup:drop-1")
	entry, err := NewItemLedgerEntry(
		"ledger-2",
		"player-1",
		"iron_ore",
		"stack-1",
		quantity,
		LedgerActionIncrease,
		25,
		location,
		"loot_pickup",
		referenceKey,
	)
	if err != nil {
		t.Fatalf("NewItemLedgerEntry valid value: %v", err)
	}
	return entry
}

func validMoney(t *testing.T, amount int64) foundation.Money {
	t.Helper()

	money, err := foundation.NewMoney(amount)
	if err != nil {
		t.Fatalf("NewMoney(%d): %v", amount, err)
	}
	return money
}

func validReferenceKey(t *testing.T, value string) foundation.IdempotencyKey {
	t.Helper()

	key, err := foundation.ParseIdempotencyKey(value)
	if err != nil {
		t.Fatalf("ParseIdempotencyKey(%q): %v", value, err)
	}
	return key
}
