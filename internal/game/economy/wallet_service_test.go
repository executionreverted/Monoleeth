package economy

import (
	"context"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestCreditWalletValidatesRequiredInputs(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*CreditWalletInput)
		wantErr error
	}{
		{
			name: "blank player",
			mutate: func(input *CreditWalletInput) {
				input.PlayerID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "invalid currency",
			mutate: func(input *CreditWalletInput) {
				input.Currency = CurrencyBucket("gold")
			},
			wantErr: ErrInvalidCurrencyBucket,
		},
		{
			name: "zero amount",
			mutate: func(input *CreditWalletInput) {
				input.Amount = 0
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "negative amount",
			mutate: func(input *CreditWalletInput) {
				input.Amount = -1
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "blank reason",
			mutate: func(input *CreditWalletInput) {
				input.Reason = ""
			},
			wantErr: ErrEmptyLedgerReason,
		},
		{
			name: "blank reference",
			mutate: func(input *CreditWalletInput) {
				input.ReferenceKey = ""
			},
			wantErr: foundation.ErrEmptyIdempotencyKey,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestWalletService()
			input := validCreditWalletInput(t, "quest_reward:wallet-credit-validation")
			tc.mutate(&input)

			_, err := service.CreditWallet(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CreditWallet error = %v, want %v", err, tc.wantErr)
			}
			if got := service.Balance("player-1", CurrencyBucketCredits); got != 0 {
				t.Fatalf("Balance() = %d, want 0", got)
			}
			if got := len(service.CurrencyLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestDebitWalletValidatesRequiredInputs(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*DebitWalletInput)
		wantErr error
	}{
		{
			name: "blank player",
			mutate: func(input *DebitWalletInput) {
				input.PlayerID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "invalid currency",
			mutate: func(input *DebitWalletInput) {
				input.Currency = CurrencyBucket("gold")
			},
			wantErr: ErrInvalidCurrencyBucket,
		},
		{
			name: "zero amount",
			mutate: func(input *DebitWalletInput) {
				input.Amount = 0
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "negative amount",
			mutate: func(input *DebitWalletInput) {
				input.Amount = -1
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "blank reason",
			mutate: func(input *DebitWalletInput) {
				input.Reason = " "
			},
			wantErr: ErrEmptyLedgerReason,
		},
		{
			name: "blank reference",
			mutate: func(input *DebitWalletInput) {
				input.ReferenceKey = ""
			},
			wantErr: foundation.ErrEmptyIdempotencyKey,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestWalletService()
			input := validDebitWalletInput(t, "quest_reward:wallet-debit-validation")
			tc.mutate(&input)

			_, err := service.DebitWallet(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("DebitWallet error = %v, want %v", err, tc.wantErr)
			}
			if got := len(service.CurrencyLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestTransferCurrencyValidatesRequiredInputs(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*TransferCurrencyInput)
		wantErr error
	}{
		{
			name: "blank from player",
			mutate: func(input *TransferCurrencyInput) {
				input.FromPlayerID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "blank to player",
			mutate: func(input *TransferCurrencyInput) {
				input.ToPlayerID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "invalid currency",
			mutate: func(input *TransferCurrencyInput) {
				input.Currency = CurrencyBucket("gold")
			},
			wantErr: ErrInvalidCurrencyBucket,
		},
		{
			name: "negative amount",
			mutate: func(input *TransferCurrencyInput) {
				input.Amount = -1
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "zero amount",
			mutate: func(input *TransferCurrencyInput) {
				input.Amount = 0
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "blank reason",
			mutate: func(input *TransferCurrencyInput) {
				input.Reason = ""
			},
			wantErr: ErrEmptyLedgerReason,
		},
		{
			name: "blank reference",
			mutate: func(input *TransferCurrencyInput) {
				input.ReferenceKey = ""
			},
			wantErr: foundation.ErrEmptyIdempotencyKey,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestWalletService()
			input := validTransferCurrencyInput(t, "quest_reward:wallet-transfer-validation")
			tc.mutate(&input)

			_, err := service.TransferCurrency(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("TransferCurrency error = %v, want %v", err, tc.wantErr)
			}
			if got := len(service.CurrencyLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestCreditWalletCreditsOnceAndWritesCurrencyLedgerEntry(t *testing.T) {
	service := newTestWalletService()
	input := validCreditWalletInput(t, "quest_reward:wallet-credit-1")

	result, err := service.CreditWallet(input)
	if err != nil {
		t.Fatalf("CreditWallet: %v", err)
	}

	if result.Duplicate {
		t.Fatal("CreditWallet Duplicate = true, want false")
	}
	if got := service.Balance(input.PlayerID, input.Currency); got != input.Amount {
		t.Fatalf("Balance() = %d, want %d", got, input.Amount)
	}
	assertWalletBalance(t, result.Balance, input.PlayerID, input.Currency, input.Amount)
	assertCurrencyLedgerEntry(t, result.LedgerEntry, input.PlayerID, input.Currency, input.Amount, LedgerActionIncrease, input.Amount, input.Reason, input.ReferenceKey)
	if result.LedgerEntry.CreatedAt != testWalletNow {
		t.Fatalf("ledger created at = %s, want %s", result.LedgerEntry.CreatedAt, testWalletNow)
	}

	entries := service.CurrencyLedgerEntries()
	if len(entries) != 1 {
		t.Fatalf("ledger entries len = %d, want 1", len(entries))
	}
	if entries[0] != result.LedgerEntry {
		t.Fatalf("snapshot ledger entry = %#v, want %#v", entries[0], result.LedgerEntry)
	}
}

func TestCreditWalletDuplicateReferenceDoesNotDuplicateCreditOrLedger(t *testing.T) {
	service := newTestWalletService()
	input := validCreditWalletInput(t, "quest_reward:wallet-credit-duplicate")

	first, err := service.CreditWallet(input)
	if err != nil {
		t.Fatalf("first CreditWallet: %v", err)
	}
	second, err := service.CreditWallet(input)
	if err != nil {
		t.Fatalf("duplicate CreditWallet: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first CreditWallet Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate CreditWallet Duplicate = false, want true")
	}
	if got := service.Balance(input.PlayerID, input.Currency); got != input.Amount {
		t.Fatalf("Balance() = %d, want %d", got, input.Amount)
	}
	if got := len(service.CurrencyLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
	if second.LedgerEntry.LedgerID != first.LedgerEntry.LedgerID {
		t.Fatalf("duplicate LedgerID = %q, want %q", second.LedgerEntry.LedgerID, first.LedgerEntry.LedgerID)
	}
}

func TestDebitWalletDebitsOnceAndWritesCurrencyLedgerEntry(t *testing.T) {
	service := newTestWalletService()
	creditWalletForTest(t, service, "player-1", CurrencyBucketCredits, 500, "quest_reward:wallet-debit-seed")
	input := validDebitWalletInput(t, "quest_reward:wallet-debit-1")

	result, err := service.DebitWallet(input)
	if err != nil {
		t.Fatalf("DebitWallet: %v", err)
	}

	if result.Duplicate {
		t.Fatal("DebitWallet Duplicate = true, want false")
	}
	if got := service.Balance(input.PlayerID, input.Currency); got != 350 {
		t.Fatalf("Balance() = %d, want 350", got)
	}
	assertWalletBalance(t, result.Balance, input.PlayerID, input.Currency, 350)
	assertCurrencyLedgerEntry(t, result.LedgerEntry, input.PlayerID, input.Currency, input.Amount, LedgerActionDecrease, 350, input.Reason, input.ReferenceKey)

	entries := service.CurrencyLedgerEntries()
	if len(entries) != 2 {
		t.Fatalf("ledger entries len = %d, want 2", len(entries))
	}
	if entries[1] != result.LedgerEntry {
		t.Fatalf("debit ledger entry = %#v, want %#v", entries[1], result.LedgerEntry)
	}
}

func TestDebitWalletDuplicateReferenceDoesNotDuplicateDebitOrLedger(t *testing.T) {
	service := newTestWalletService()
	creditWalletForTest(t, service, "player-1", CurrencyBucketCredits, 500, "quest_reward:wallet-debit-duplicate-seed")
	input := validDebitWalletInput(t, "quest_reward:wallet-debit-duplicate")

	first, err := service.DebitWallet(input)
	if err != nil {
		t.Fatalf("first DebitWallet: %v", err)
	}
	second, err := service.DebitWallet(input)
	if err != nil {
		t.Fatalf("duplicate DebitWallet: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first DebitWallet Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate DebitWallet Duplicate = false, want true")
	}
	if got := service.Balance(input.PlayerID, input.Currency); got != 350 {
		t.Fatalf("Balance() = %d, want 350", got)
	}
	if got := len(service.CurrencyLedgerEntries()); got != 2 {
		t.Fatalf("ledger entries len = %d, want 2", got)
	}
	if second.LedgerEntry.LedgerID != first.LedgerEntry.LedgerID {
		t.Fatalf("duplicate LedgerID = %q, want %q", second.LedgerEntry.LedgerID, first.LedgerEntry.LedgerID)
	}
}

func TestDebitWalletRejectsInsufficientFundsWithoutMutationOrLedgerEntry(t *testing.T) {
	service := newTestWalletService()
	creditWalletForTest(t, service, "player-1", CurrencyBucketCredits, 100, "quest_reward:wallet-debit-insufficient-seed")
	input := validDebitWalletInput(t, "quest_reward:wallet-debit-insufficient")
	input.Amount = 150

	_, err := service.DebitWallet(input)
	if !errors.Is(err, ErrInsufficientWalletFunds) {
		t.Fatalf("DebitWallet error = %v, want ErrInsufficientWalletFunds", err)
	}
	if got := service.Balance(input.PlayerID, input.Currency); got != 100 {
		t.Fatalf("Balance() = %d, want 100", got)
	}
	if got := len(service.CurrencyLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestTransferCurrencyMovesFundsOnceAndWritesDebitAndCreditLedgerEntries(t *testing.T) {
	service := newTestWalletService()
	creditWalletForTest(t, service, "player-1", CurrencyBucketCredits, 500, "quest_reward:wallet-transfer-seed")
	input := validTransferCurrencyInput(t, "quest_reward:wallet-transfer-1")

	result, err := service.TransferCurrency(input)
	if err != nil {
		t.Fatalf("TransferCurrency: %v", err)
	}

	if result.Duplicate {
		t.Fatal("TransferCurrency Duplicate = true, want false")
	}
	if got := service.Balance(input.FromPlayerID, input.Currency); got != 300 {
		t.Fatalf("from Balance() = %d, want 300", got)
	}
	if got := service.Balance(input.ToPlayerID, input.Currency); got != 200 {
		t.Fatalf("to Balance() = %d, want 200", got)
	}
	assertWalletBalance(t, result.FromBalance, input.FromPlayerID, input.Currency, 300)
	assertWalletBalance(t, result.ToBalance, input.ToPlayerID, input.Currency, 200)
	if len(result.LedgerEntries) != 2 {
		t.Fatalf("result ledger entries len = %d, want 2", len(result.LedgerEntries))
	}
	assertCurrencyLedgerEntry(t, result.LedgerEntries[0], input.FromPlayerID, input.Currency, input.Amount, LedgerActionDecrease, 300, input.Reason, input.ReferenceKey)
	assertCurrencyLedgerEntry(t, result.LedgerEntries[1], input.ToPlayerID, input.Currency, input.Amount, LedgerActionIncrease, 200, input.Reason, input.ReferenceKey)

	entries := service.CurrencyLedgerEntries()
	if len(entries) != 3 {
		t.Fatalf("ledger entries len = %d, want 3", len(entries))
	}
	if entries[1] != result.LedgerEntries[0] {
		t.Fatalf("transfer debit entry = %#v, want %#v", entries[1], result.LedgerEntries[0])
	}
	if entries[2] != result.LedgerEntries[1] {
		t.Fatalf("transfer credit entry = %#v, want %#v", entries[2], result.LedgerEntries[1])
	}
}

func TestTransferCurrencyDuplicateReferenceDoesNotDuplicateTransferOrLedger(t *testing.T) {
	service := newTestWalletService()
	creditWalletForTest(t, service, "player-1", CurrencyBucketCredits, 500, "quest_reward:wallet-transfer-duplicate-seed")
	input := validTransferCurrencyInput(t, "quest_reward:wallet-transfer-duplicate")

	first, err := service.TransferCurrency(input)
	if err != nil {
		t.Fatalf("first TransferCurrency: %v", err)
	}
	second, err := service.TransferCurrency(input)
	if err != nil {
		t.Fatalf("duplicate TransferCurrency: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first TransferCurrency Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate TransferCurrency Duplicate = false, want true")
	}
	if got := service.Balance(input.FromPlayerID, input.Currency); got != 300 {
		t.Fatalf("from Balance() = %d, want 300", got)
	}
	if got := service.Balance(input.ToPlayerID, input.Currency); got != 200 {
		t.Fatalf("to Balance() = %d, want 200", got)
	}
	if got := len(service.CurrencyLedgerEntries()); got != 3 {
		t.Fatalf("ledger entries len = %d, want 3", got)
	}
	if second.LedgerEntries[0].LedgerID != first.LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate debit LedgerID = %q, want %q", second.LedgerEntries[0].LedgerID, first.LedgerEntries[0].LedgerID)
	}
	if second.LedgerEntries[1].LedgerID != first.LedgerEntries[1].LedgerID {
		t.Fatalf("duplicate credit LedgerID = %q, want %q", second.LedgerEntries[1].LedgerID, first.LedgerEntries[1].LedgerID)
	}
}

func TestTransferCurrencyRejectsSelfTransferWithoutMutationOrLedgerEntry(t *testing.T) {
	service := newTestWalletService()
	creditWalletForTest(t, service, "player-1", CurrencyBucketCredits, 500, "quest_reward:wallet-transfer-self-seed")
	input := validTransferCurrencyInput(t, "quest_reward:wallet-transfer-self")
	input.ToPlayerID = input.FromPlayerID

	_, err := service.TransferCurrency(input)
	if !errors.Is(err, ErrWalletSelfTransfer) {
		t.Fatalf("TransferCurrency error = %v, want ErrWalletSelfTransfer", err)
	}
	if got := service.Balance(input.FromPlayerID, input.Currency); got != 500 {
		t.Fatalf("Balance() = %d, want 500", got)
	}
	if got := len(service.CurrencyLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}

	input.ToPlayerID = "player-2"
	result, err := service.TransferCurrency(input)
	if err != nil {
		t.Fatalf("retry TransferCurrency with same reference after self-transfer rejection: %v", err)
	}
	if result.Duplicate {
		t.Fatal("retry TransferCurrency Duplicate = true, want false")
	}
}

func TestTransferCurrencyRejectsInsufficientFundsWithoutMutationOrLedgerEntry(t *testing.T) {
	service := newTestWalletService()
	creditWalletForTest(t, service, "player-1", CurrencyBucketCredits, 100, "quest_reward:wallet-transfer-insufficient-seed")
	input := validTransferCurrencyInput(t, "quest_reward:wallet-transfer-insufficient")
	input.Amount = 150

	_, err := service.TransferCurrency(input)
	if !errors.Is(err, ErrInsufficientWalletFunds) {
		t.Fatalf("TransferCurrency error = %v, want ErrInsufficientWalletFunds", err)
	}
	if got := service.Balance(input.FromPlayerID, input.Currency); got != 100 {
		t.Fatalf("from Balance() = %d, want 100", got)
	}
	if got := service.Balance(input.ToPlayerID, input.Currency); got != 0 {
		t.Fatalf("to Balance() = %d, want 0", got)
	}
	if got := len(service.CurrencyLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestWalletServiceFindsCurrencyLedgerEntryByLedgerReference(t *testing.T) {
	service := newTestWalletService()
	input := validCreditWalletInput(t, "quest_reward:wallet-ledger-reference")

	first, err := service.CreditWallet(input)
	if err != nil {
		t.Fatalf("CreditWallet: %v", err)
	}
	duplicate, err := service.CreditWallet(input)
	if err != nil {
		t.Fatalf("duplicate CreditWallet: %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate CreditWallet Duplicate = false, want true")
	}

	entry, ok := service.FindCurrencyLedgerEntry(CurrencyLedgerReferenceLookup{
		PlayerID:     input.PlayerID,
		Currency:     input.Currency,
		Action:       LedgerActionIncrease,
		Reason:       input.Reason,
		ReferenceKey: input.ReferenceKey,
	})
	if !ok {
		t.Fatal("FindCurrencyLedgerEntry ok = false, want true")
	}
	if entry != first.LedgerEntry {
		t.Fatalf("FindCurrencyLedgerEntry entry = %#v, want %#v", entry, first.LedgerEntry)
	}
	if entry.LedgerID != duplicate.LedgerEntry.LedgerID {
		t.Fatalf("lookup LedgerID = %q, duplicate LedgerID = %q", entry.LedgerID, duplicate.LedgerEntry.LedgerID)
	}

	entry.BalanceAfter = 999
	again, ok := service.FindCurrencyLedgerEntry(CurrencyLedgerReferenceLookup{
		PlayerID:     input.PlayerID,
		Currency:     input.Currency,
		Action:       LedgerActionIncrease,
		Reason:       input.Reason,
		ReferenceKey: input.ReferenceKey,
	})
	if !ok {
		t.Fatal("second FindCurrencyLedgerEntry ok = false, want true")
	}
	if again.BalanceAfter == 999 {
		t.Fatal("FindCurrencyLedgerEntry returned mutable internal ledger entry")
	}

	_, ok = service.FindCurrencyLedgerEntry(CurrencyLedgerReferenceLookup{
		PlayerID:     "player-2",
		Currency:     input.Currency,
		Action:       LedgerActionIncrease,
		Reason:       input.Reason,
		ReferenceKey: input.ReferenceKey,
	})
	if ok {
		t.Fatal("FindCurrencyLedgerEntry with different player ok = true, want false")
	}
}

func TestWalletSnapshotHelpersReturnCopies(t *testing.T) {
	service := newTestWalletService()
	creditWalletForTest(t, service, "player-2", CurrencyBucketCredits, 50, "quest_reward:wallet-snapshot-2")
	creditWalletForTest(t, service, "player-1", CurrencyBucketPremiumPaid, 25, "quest_reward:wallet-snapshot-1")

	balances := service.WalletBalances()
	if len(balances) != 2 {
		t.Fatalf("wallet balances len = %d, want 2", len(balances))
	}
	if balances[0].PlayerID != "player-1" {
		t.Fatalf("first balance player = %q, want player-1", balances[0].PlayerID)
	}
	balances[0].Balance = 999
	if got := service.Balance("player-1", CurrencyBucketPremiumPaid); got != 25 {
		t.Fatalf("Balance() after snapshot mutation = %d, want 25", got)
	}

	entries := service.CurrencyLedgerEntries()
	entries[0].BalanceAfter = 999
	if got := service.CurrencyLedgerEntries()[0].BalanceAfter; got == 999 {
		t.Fatal("CurrencyLedgerEntries returned mutable internal slice")
	}
}

var testWalletNow = time.Date(2026, 6, 17, 16, 0, 0, 0, time.UTC)

func TestNewWalletServiceWithRepositoryLoadsPersistedBalances(t *testing.T) {
	repository := &fakeWalletRepository{
		balances: []WalletBalance{
			{PlayerID: "player-1", Currency: CurrencyBucketCredits, Balance: 500, UpdatedAt: testWalletNow},
		},
	}

	service, err := NewWalletServiceWithRepository(testutil.NewFakeClock(testWalletNow), repository)
	if err != nil {
		t.Fatalf("NewWalletServiceWithRepository() error = %v, want nil", err)
	}

	if got := service.Balance("player-1", CurrencyBucketCredits); got != 500 {
		t.Fatalf("Balance() = %d, want loaded 500", got)
	}
}

func TestCreditWalletPersistsBalanceThroughRepository(t *testing.T) {
	repository := &fakeWalletRepository{}
	service, err := NewWalletServiceWithRepository(testutil.NewFakeClock(testWalletNow), repository)
	if err != nil {
		t.Fatalf("NewWalletServiceWithRepository() error = %v, want nil", err)
	}

	if _, err := service.CreditWallet(validCreditWalletInput(t, "quest_reward:wallet-persist-1")); err != nil {
		t.Fatalf("CreditWallet() error = %v, want nil", err)
	}

	saved, ok := repository.saved(foundation.PlayerID("player-1"), CurrencyBucketCredits)
	if !ok || saved.Balance != 250 {
		t.Fatalf("persisted balance = %+v ok=%v, want balance 250", saved, ok)
	}
}

type fakeWalletRepository struct {
	balances []WalletBalance
	upserts  []WalletBalance
}

func (repository *fakeWalletRepository) LoadWalletBalances(context.Context) ([]WalletBalance, error) {
	return append([]WalletBalance(nil), repository.balances...), nil
}

func (repository *fakeWalletRepository) UpsertWalletBalance(_ context.Context, balance WalletBalance) error {
	repository.upserts = append(repository.upserts, balance)
	for index := range repository.balances {
		if repository.balances[index].PlayerID == balance.PlayerID && repository.balances[index].Currency == balance.Currency {
			repository.balances[index] = balance
			return nil
		}
	}
	repository.balances = append(repository.balances, balance)
	return nil
}

func (repository *fakeWalletRepository) saved(playerID foundation.PlayerID, currency CurrencyBucket) (WalletBalance, bool) {
	for index := len(repository.upserts) - 1; index >= 0; index-- {
		if repository.upserts[index].PlayerID == playerID && repository.upserts[index].Currency == currency {
			return repository.upserts[index], true
		}
	}
	return WalletBalance{}, false
}

func newTestWalletService() *WalletService {
	return NewWalletService(testutil.NewFakeClock(testWalletNow))
}

func validCreditWalletInput(t *testing.T, reference string) CreditWalletInput {
	t.Helper()

	return CreditWalletInput{
		PlayerID:     "player-1",
		Currency:     CurrencyBucketCredits,
		Amount:       250,
		Reason:       "quest_reward",
		ReferenceKey: validReferenceKey(t, reference),
	}
}

func validDebitWalletInput(t *testing.T, reference string) DebitWalletInput {
	t.Helper()

	return DebitWalletInput{
		PlayerID:     "player-1",
		Currency:     CurrencyBucketCredits,
		Amount:       150,
		Reason:       "market_purchase",
		ReferenceKey: validReferenceKey(t, reference),
	}
}

func validTransferCurrencyInput(t *testing.T, reference string) TransferCurrencyInput {
	t.Helper()

	return TransferCurrencyInput{
		FromPlayerID: "player-1",
		ToPlayerID:   "player-2",
		Currency:     CurrencyBucketCredits,
		Amount:       200,
		Reason:       "market_sale",
		ReferenceKey: validReferenceKey(t, reference),
	}
}

func creditWalletForTest(t *testing.T, service *WalletService, playerID foundation.PlayerID, currency CurrencyBucket, amount int64, reference string) {
	t.Helper()

	_, err := service.CreditWallet(CreditWalletInput{
		PlayerID:     playerID,
		Currency:     currency,
		Amount:       amount,
		Reason:       "test_seed",
		ReferenceKey: validReferenceKey(t, reference),
	})
	if err != nil {
		t.Fatalf("seed CreditWallet: %v", err)
	}
}

func assertWalletBalance(t *testing.T, balance WalletBalance, playerID foundation.PlayerID, currency CurrencyBucket, amount int64) {
	t.Helper()

	if balance.PlayerID != playerID {
		t.Fatalf("balance player = %q, want %q", balance.PlayerID, playerID)
	}
	if balance.Currency != currency {
		t.Fatalf("balance currency = %q, want %q", balance.Currency, currency)
	}
	if balance.Balance != amount {
		t.Fatalf("balance amount = %d, want %d", balance.Balance, amount)
	}
	if balance.UpdatedAt != testWalletNow {
		t.Fatalf("balance updated at = %s, want %s", balance.UpdatedAt, testWalletNow)
	}
}

func assertCurrencyLedgerEntry(
	t *testing.T,
	entry CurrencyLedgerEntry,
	playerID foundation.PlayerID,
	currency CurrencyBucket,
	amount int64,
	action LedgerAction,
	balanceAfter int64,
	reason LedgerReason,
	referenceKey foundation.IdempotencyKey,
) {
	t.Helper()

	if entry.LedgerID.IsZero() {
		t.Fatal("ledger id is zero")
	}
	if entry.PlayerID != playerID {
		t.Fatalf("ledger player = %q, want %q", entry.PlayerID, playerID)
	}
	if entry.Currency != currency {
		t.Fatalf("ledger currency = %q, want %q", entry.Currency, currency)
	}
	if got := entry.Amount.Int64(); got != amount {
		t.Fatalf("ledger amount = %d, want %d", got, amount)
	}
	if entry.Action != action {
		t.Fatalf("ledger action = %q, want %q", entry.Action, action)
	}
	if entry.BalanceAfter != balanceAfter {
		t.Fatalf("ledger balance after = %d, want %d", entry.BalanceAfter, balanceAfter)
	}
	if entry.Reason != reason {
		t.Fatalf("ledger reason = %q, want %q", entry.Reason, reason)
	}
	if entry.ReferenceKey != referenceKey {
		t.Fatalf("ledger reference = %q, want %q", entry.ReferenceKey, referenceKey)
	}
	if entry.CreatedAt != testWalletNow {
		t.Fatalf("ledger created at = %s, want %s", entry.CreatedAt, testWalletNow)
	}
}
