package economy

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"

	"gameproject/internal/game/foundation"
)

const (
	creditWalletOperation     = "credit_wallet"
	debitWalletOperation      = "debit_wallet"
	transferCurrencyOperation = "transfer_currency"
)

var (
	ErrInsufficientWalletFunds = errors.New("insufficient wallet funds")
	ErrWalletBalanceOverflow   = errors.New("wallet balance overflow")
)

// CreditWalletInput describes one authoritative currency grant.
type CreditWalletInput struct {
	PlayerID     foundation.PlayerID       `json:"player_id"`
	Currency     CurrencyBucket            `json:"currency_type"`
	Amount       int64                     `json:"amount"`
	Reason       LedgerReason              `json:"reason"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
}

// DebitWalletInput describes one authoritative currency spend.
type DebitWalletInput struct {
	PlayerID     foundation.PlayerID       `json:"player_id"`
	Currency     CurrencyBucket            `json:"currency_type"`
	Amount       int64                     `json:"amount"`
	Reason       LedgerReason              `json:"reason"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
}

// TransferCurrencyInput describes one authoritative currency movement between players.
type TransferCurrencyInput struct {
	FromPlayerID foundation.PlayerID       `json:"from_player_id"`
	ToPlayerID   foundation.PlayerID       `json:"to_player_id"`
	Currency     CurrencyBucket            `json:"currency_type"`
	Amount       int64                     `json:"amount"`
	Reason       LedgerReason              `json:"reason"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
}

// CreditWalletResult reports the balance and ledger row created by CreditWallet.
type CreditWalletResult struct {
	Balance     WalletBalance       `json:"balance"`
	LedgerEntry CurrencyLedgerEntry `json:"ledger_entry"`
	Duplicate   bool                `json:"duplicate"`
}

// DebitWalletResult reports the balance and ledger row created by DebitWallet.
type DebitWalletResult struct {
	Balance     WalletBalance       `json:"balance"`
	LedgerEntry CurrencyLedgerEntry `json:"ledger_entry"`
	Duplicate   bool                `json:"duplicate"`
}

// TransferCurrencyResult reports balances and ledger rows created by TransferCurrency.
type TransferCurrencyResult struct {
	FromBalance   WalletBalance         `json:"from_balance"`
	ToBalance     WalletBalance         `json:"to_balance"`
	LedgerEntries []CurrencyLedgerEntry `json:"ledger_entries"`
	Duplicate     bool                  `json:"duplicate"`
}

// WalletService is an in-memory Phase 02 currency mutation service.
type WalletService struct {
	mu    sync.Mutex
	clock foundation.Clock

	nextLedgerSequence int64

	balances              map[walletBalanceKey]WalletBalance
	currencyLedgerEntries []CurrencyLedgerEntry
	creditReferences      map[walletReferenceKey]CreditWalletResult
	debitReferences       map[walletReferenceKey]DebitWalletResult
	transferReferences    map[walletReferenceKey]TransferCurrencyResult
}

type walletBalanceKey struct {
	playerID foundation.PlayerID
	currency CurrencyBucket
}

type walletReferenceKey struct {
	playerID     foundation.PlayerID
	operation    string
	referenceKey foundation.IdempotencyKey
}

// NewWalletService returns an in-memory wallet mutation service.
func NewWalletService(clock foundation.Clock) *WalletService {
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &WalletService{
		clock:              clock,
		balances:           make(map[walletBalanceKey]WalletBalance),
		creditReferences:   make(map[walletReferenceKey]CreditWalletResult),
		debitReferences:    make(map[walletReferenceKey]DebitWalletResult),
		transferReferences: make(map[walletReferenceKey]TransferCurrencyResult),
	}
}

// CreditWallet credits currency once for a player/reference pair and writes a currency ledger row.
func (service *WalletService) CreditWallet(input CreditWalletInput) (CreditWalletResult, error) {
	amount, err := input.validate()
	if err != nil {
		return CreditWalletResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	reference := walletReferenceKey{
		playerID:     input.PlayerID,
		operation:    creditWalletOperation,
		referenceKey: input.ReferenceKey,
	}
	if previous, ok := service.creditReferences[reference]; ok {
		result := previous
		result.Duplicate = true
		return result, nil
	}

	now := service.clock.Now()
	currentBalance := service.balanceAmountLocked(input.PlayerID, input.Currency)
	balanceAfter, err := addWalletAmount(currentBalance, amount.Int64())
	if err != nil {
		return CreditWalletResult{}, err
	}
	balance := WalletBalance{
		PlayerID:  input.PlayerID,
		Currency:  input.Currency,
		Balance:   balanceAfter,
		UpdatedAt: now,
	}
	if err := balance.Validate(); err != nil {
		return CreditWalletResult{}, err
	}

	ledgerEntry, err := service.newCurrencyLedgerEntryLocked(input.PlayerID, input.Currency, amount, LedgerActionIncrease, balanceAfter, input.Reason, input.ReferenceKey)
	if err != nil {
		return CreditWalletResult{}, err
	}
	ledgerEntry.CreatedAt = now

	service.balances[walletBalanceKey{playerID: input.PlayerID, currency: input.Currency}] = balance
	service.currencyLedgerEntries = append(service.currencyLedgerEntries, ledgerEntry)

	result := CreditWalletResult{
		Balance:     balance,
		LedgerEntry: ledgerEntry,
	}
	service.creditReferences[reference] = result
	return result, nil
}

// DebitWallet debits currency once for a player/reference pair and writes a currency ledger row.
func (service *WalletService) DebitWallet(input DebitWalletInput) (DebitWalletResult, error) {
	amount, err := input.validate()
	if err != nil {
		return DebitWalletResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	reference := walletReferenceKey{
		playerID:     input.PlayerID,
		operation:    debitWalletOperation,
		referenceKey: input.ReferenceKey,
	}
	if previous, ok := service.debitReferences[reference]; ok {
		result := previous
		result.Duplicate = true
		return result, nil
	}

	now := service.clock.Now()
	currentBalance := service.balanceAmountLocked(input.PlayerID, input.Currency)
	if currentBalance < amount.Int64() {
		return DebitWalletResult{}, fmt.Errorf("have %d need %d: %w", currentBalance, amount.Int64(), ErrInsufficientWalletFunds)
	}
	balanceAfter := currentBalance - amount.Int64()
	balance := WalletBalance{
		PlayerID:  input.PlayerID,
		Currency:  input.Currency,
		Balance:   balanceAfter,
		UpdatedAt: now,
	}
	if err := balance.Validate(); err != nil {
		return DebitWalletResult{}, err
	}

	ledgerEntry, err := service.newCurrencyLedgerEntryLocked(input.PlayerID, input.Currency, amount, LedgerActionDecrease, balanceAfter, input.Reason, input.ReferenceKey)
	if err != nil {
		return DebitWalletResult{}, err
	}
	ledgerEntry.CreatedAt = now

	service.balances[walletBalanceKey{playerID: input.PlayerID, currency: input.Currency}] = balance
	service.currencyLedgerEntries = append(service.currencyLedgerEntries, ledgerEntry)

	result := DebitWalletResult{
		Balance:     balance,
		LedgerEntry: ledgerEntry,
	}
	service.debitReferences[reference] = result
	return result, nil
}

// TransferCurrency moves currency between players once for a source player/reference pair.
func (service *WalletService) TransferCurrency(input TransferCurrencyInput) (TransferCurrencyResult, error) {
	amount, err := input.validate()
	if err != nil {
		return TransferCurrencyResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	reference := walletReferenceKey{
		playerID:     input.FromPlayerID,
		operation:    transferCurrencyOperation,
		referenceKey: input.ReferenceKey,
	}
	if previous, ok := service.transferReferences[reference]; ok {
		result := cloneTransferCurrencyResult(previous)
		result.Duplicate = true
		return result, nil
	}

	now := service.clock.Now()
	fromCurrentBalance := service.balanceAmountLocked(input.FromPlayerID, input.Currency)
	if fromCurrentBalance < amount.Int64() {
		return TransferCurrencyResult{}, fmt.Errorf("have %d need %d: %w", fromCurrentBalance, amount.Int64(), ErrInsufficientWalletFunds)
	}
	fromBalanceAfter := fromCurrentBalance - amount.Int64()
	toCurrentBalance := service.balanceAmountLocked(input.ToPlayerID, input.Currency)
	if input.FromPlayerID == input.ToPlayerID {
		toCurrentBalance = fromBalanceAfter
	}
	toBalanceAfter, err := addWalletAmount(toCurrentBalance, amount.Int64())
	if err != nil {
		return TransferCurrencyResult{}, err
	}

	debitEntry, err := service.newCurrencyLedgerEntryLocked(input.FromPlayerID, input.Currency, amount, LedgerActionDecrease, fromBalanceAfter, input.Reason, input.ReferenceKey)
	if err != nil {
		return TransferCurrencyResult{}, err
	}
	debitEntry.CreatedAt = now

	creditEntry, err := service.newCurrencyLedgerEntryLocked(input.ToPlayerID, input.Currency, amount, LedgerActionIncrease, toBalanceAfter, input.Reason, input.ReferenceKey)
	if err != nil {
		return TransferCurrencyResult{}, err
	}
	creditEntry.CreatedAt = now

	fromBalance := WalletBalance{
		PlayerID:  input.FromPlayerID,
		Currency:  input.Currency,
		Balance:   fromBalanceAfter,
		UpdatedAt: now,
	}
	if err := fromBalance.Validate(); err != nil {
		return TransferCurrencyResult{}, err
	}
	toBalance := WalletBalance{
		PlayerID:  input.ToPlayerID,
		Currency:  input.Currency,
		Balance:   toBalanceAfter,
		UpdatedAt: now,
	}
	if err := toBalance.Validate(); err != nil {
		return TransferCurrencyResult{}, err
	}

	service.balances[walletBalanceKey{playerID: input.FromPlayerID, currency: input.Currency}] = fromBalance
	service.balances[walletBalanceKey{playerID: input.ToPlayerID, currency: input.Currency}] = toBalance
	service.currencyLedgerEntries = append(service.currencyLedgerEntries, debitEntry, creditEntry)

	result := TransferCurrencyResult{
		FromBalance:   fromBalance,
		ToBalance:     toBalance,
		LedgerEntries: []CurrencyLedgerEntry{debitEntry, creditEntry},
	}
	service.transferReferences[reference] = cloneTransferCurrencyResult(result)
	return cloneTransferCurrencyResult(result), nil
}

// WalletBalances returns a stable snapshot of in-memory wallet balances.
func (service *WalletService) WalletBalances() []WalletBalance {
	service.mu.Lock()
	defer service.mu.Unlock()

	balances := make([]WalletBalance, 0, len(service.balances))
	for _, balance := range service.balances {
		balances = append(balances, balance)
	}
	sort.Slice(balances, func(i, j int) bool {
		if balances[i].PlayerID != balances[j].PlayerID {
			return balances[i].PlayerID < balances[j].PlayerID
		}
		return balances[i].Currency < balances[j].Currency
	})
	return balances
}

// CurrencyLedgerEntries returns a snapshot of in-memory currency ledger rows.
func (service *WalletService) CurrencyLedgerEntries() []CurrencyLedgerEntry {
	service.mu.Lock()
	defer service.mu.Unlock()

	return append([]CurrencyLedgerEntry(nil), service.currencyLedgerEntries...)
}

// Balance returns the current balance for a player/currency tuple.
func (service *WalletService) Balance(playerID foundation.PlayerID, currency CurrencyBucket) int64 {
	service.mu.Lock()
	defer service.mu.Unlock()

	return service.balanceAmountLocked(playerID, currency)
}

func (input CreditWalletInput) validate() (foundation.Money, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return foundation.Money{}, err
	}
	if err := input.Currency.Validate(); err != nil {
		return foundation.Money{}, err
	}
	amount, err := foundation.NewMoney(input.Amount)
	if err != nil {
		return foundation.Money{}, err
	}
	if err := input.Reason.Validate(); err != nil {
		return foundation.Money{}, err
	}
	if err := input.ReferenceKey.Validate(); err != nil {
		return foundation.Money{}, err
	}
	return amount, nil
}

func (input DebitWalletInput) validate() (foundation.Money, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return foundation.Money{}, err
	}
	if err := input.Currency.Validate(); err != nil {
		return foundation.Money{}, err
	}
	amount, err := foundation.NewMoney(input.Amount)
	if err != nil {
		return foundation.Money{}, err
	}
	if err := input.Reason.Validate(); err != nil {
		return foundation.Money{}, err
	}
	if err := input.ReferenceKey.Validate(); err != nil {
		return foundation.Money{}, err
	}
	return amount, nil
}

func (input TransferCurrencyInput) validate() (foundation.Money, error) {
	if err := input.FromPlayerID.Validate(); err != nil {
		return foundation.Money{}, err
	}
	if err := input.ToPlayerID.Validate(); err != nil {
		return foundation.Money{}, err
	}
	if err := input.Currency.Validate(); err != nil {
		return foundation.Money{}, err
	}
	amount, err := foundation.NewMoney(input.Amount)
	if err != nil {
		return foundation.Money{}, err
	}
	if err := input.Reason.Validate(); err != nil {
		return foundation.Money{}, err
	}
	if err := input.ReferenceKey.Validate(); err != nil {
		return foundation.Money{}, err
	}
	return amount, nil
}

func (service *WalletService) balanceAmountLocked(playerID foundation.PlayerID, currency CurrencyBucket) int64 {
	return service.balances[walletBalanceKey{playerID: playerID, currency: currency}].Balance
}

func (service *WalletService) newCurrencyLedgerEntryLocked(
	playerID foundation.PlayerID,
	currency CurrencyBucket,
	amount foundation.Money,
	action LedgerAction,
	balanceAfter int64,
	reason LedgerReason,
	referenceKey foundation.IdempotencyKey,
) (CurrencyLedgerEntry, error) {
	entry, err := NewCurrencyLedgerEntry(
		service.nextLedgerID(),
		playerID,
		currency,
		amount,
		action,
		balanceAfter,
		reason,
		referenceKey,
	)
	if err != nil {
		return CurrencyLedgerEntry{}, err
	}
	return entry, nil
}

func (service *WalletService) nextLedgerID() LedgerID {
	service.nextLedgerSequence++
	return LedgerID(fmt.Sprintf("currency-ledger-%d", service.nextLedgerSequence))
}

func addWalletAmount(current int64, amount int64) (int64, error) {
	if amount > math.MaxInt64-current {
		return 0, ErrWalletBalanceOverflow
	}
	return current + amount, nil
}

func cloneTransferCurrencyResult(result TransferCurrencyResult) TransferCurrencyResult {
	result.LedgerEntries = append([]CurrencyLedgerEntry(nil), result.LedgerEntries...)
	return result
}
