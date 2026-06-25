package economy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"

	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

const (
	creditWalletOperation     = WalletMutationOperationCredit
	debitWalletOperation      = WalletMutationOperationDebit
	transferCurrencyOperation = WalletMutationOperationTransfer
)

var (
	ErrInsufficientWalletFunds = errors.New("insufficient wallet funds")
	ErrWalletBalanceOverflow   = errors.New("wallet balance overflow")
	ErrWalletSelfTransfer      = errors.New("wallet self transfer")
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

// CurrencyLedgerReferenceLookup identifies one currency ledger row by its
// replay-safe business reference fields.
type CurrencyLedgerReferenceLookup struct {
	PlayerID     foundation.PlayerID
	Currency     CurrencyBucket
	Action       LedgerAction
	Reason       LedgerReason
	ReferenceKey foundation.IdempotencyKey
}

// WalletRepository is the durable persistence boundary for wallet balances.
type WalletRepository interface {
	LoadWalletBalances(ctx context.Context) ([]WalletBalance, error)
	LoadCurrencyLedgerEntries(ctx context.Context) ([]CurrencyLedgerEntry, error)
	LoadWalletMutationReferences(ctx context.Context) ([]WalletMutationReference, error)
	LoadWalletCounters(ctx context.Context) (WalletCounters, error)
	UpsertWalletBalance(ctx context.Context, balance WalletBalance) error
	CommitWalletMutation(ctx context.Context, commit WalletMutationCommit) error
}

// WalletService is an in-memory Phase 02 currency mutation service.
type WalletService struct {
	mu    sync.Mutex
	clock foundation.Clock

	nextLedgerSequence int64

	balances                  map[walletBalanceKey]WalletBalance
	currencyLedgerEntries     []CurrencyLedgerEntry
	currencyLedgerByReference map[CurrencyLedgerReferenceLookup]CurrencyLedgerEntry
	creditReferences          map[walletReferenceKey]CreditWalletResult
	debitReferences           map[walletReferenceKey]DebitWalletResult
	transferReferences        map[walletReferenceKey]TransferCurrencyResult

	repository WalletRepository

	emitter           EventEmitter
	nextEventSequence uint64
}

type walletBalanceKey struct {
	playerID foundation.PlayerID
	currency CurrencyBucket
}

type walletReferenceKey struct {
	playerID     foundation.PlayerID
	operation    WalletMutationOperation
	referenceKey foundation.IdempotencyKey
}

// NewWalletService returns an in-memory wallet mutation service.
func NewWalletService(clock foundation.Clock) *WalletService {
	service, err := NewWalletServiceWithRepository(clock, nil)
	if err != nil {
		panic(err)
	}
	return service
}

func NewWalletServiceWithRepository(clock foundation.Clock, repository WalletRepository) (*WalletService, error) {
	if clock == nil {
		clock = foundation.RealClock{}
	}
	service := &WalletService{
		clock:                     clock,
		balances:                  make(map[walletBalanceKey]WalletBalance),
		currencyLedgerByReference: make(map[CurrencyLedgerReferenceLookup]CurrencyLedgerEntry),
		creditReferences:          make(map[walletReferenceKey]CreditWalletResult),
		debitReferences:           make(map[walletReferenceKey]DebitWalletResult),
		transferReferences:        make(map[walletReferenceKey]TransferCurrencyResult),
		repository:                repository,
	}
	if repository != nil {
		balances, err := repository.LoadWalletBalances(context.Background())
		if err != nil {
			return nil, err
		}
		for _, balance := range balances {
			if err := balance.Validate(); err != nil {
				return nil, err
			}
			service.balances[walletBalanceKey{playerID: balance.PlayerID, currency: balance.Currency}] = balance
		}
		ledgerEntries, err := repository.LoadCurrencyLedgerEntries(context.Background())
		if err != nil {
			return nil, err
		}
		for _, entry := range ledgerEntries {
			if err := entry.Validate(); err != nil {
				return nil, err
			}
			service.currencyLedgerEntries = append(service.currencyLedgerEntries, entry)
			service.indexCurrencyLedgerEntryLocked(entry)
		}
		references, err := repository.LoadWalletMutationReferences(context.Background())
		if err != nil {
			return nil, err
		}
		for _, reference := range references {
			if err := reference.Validate(); err != nil {
				return nil, err
			}
			service.indexWalletMutationReferenceLocked(reference)
		}
		counters, err := repository.LoadWalletCounters(context.Background())
		if err != nil {
			return nil, err
		}
		if err := counters.Validate(); err != nil {
			return nil, err
		}
		service.nextLedgerSequence = safeWalletLedgerSequence(counters.LedgerSequence, service.currencyLedgerEntries)
	}
	return service, nil
}

// CreditWallet credits currency once for a player/reference pair and writes a currency ledger row.
func (service *WalletService) CreditWallet(input CreditWalletInput) (CreditWalletResult, error) {
	return service.creditWallet(input, true)
}

// CreditWalletWithoutRepository credits currency in memory without committing
// through the configured repository. Higher-level transaction owners use this
// to gather write sets before committing them atomically.
func (service *WalletService) CreditWalletWithoutRepository(input CreditWalletInput) (CreditWalletResult, error) {
	return service.creditWallet(input, false)
}

func (service *WalletService) creditWallet(input CreditWalletInput, persistRepository bool) (CreditWalletResult, error) {
	amount, err := input.validate()
	if err != nil {
		return CreditWalletResult{}, err
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()
	emitter = service.emitter

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

	result := CreditWalletResult{
		Balance:     balance,
		LedgerEntry: ledgerEntry,
	}
	if err := service.persistWalletMutationLocked(WalletMutationCommit{
		Balances:      []WalletBalance{balance},
		LedgerEntries: []CurrencyLedgerEntry{ledgerEntry},
		Reference: WalletMutationReference{
			PlayerID:      input.PlayerID,
			Operation:     creditWalletOperation,
			ReferenceKey:  input.ReferenceKey,
			LedgerEntries: []CurrencyLedgerEntry{ledgerEntry},
		},
		Counters: WalletCounters{LedgerSequence: service.nextLedgerSequence},
	}, persistRepository); err != nil {
		return CreditWalletResult{}, err
	}
	service.balances[walletBalanceKey{playerID: input.PlayerID, currency: input.Currency}] = balance
	service.currencyLedgerEntries = append(service.currencyLedgerEntries, ledgerEntry)
	service.indexCurrencyLedgerEntryLocked(ledgerEntry)

	service.creditReferences[reference] = result
	if emitter != nil {
		emitted = []events.EventEnvelope{
			service.walletMutationEventLocked(EventWalletCredited, WalletMutationPayload{
				PlayerID:     input.PlayerID,
				Currency:     input.Currency,
				Amount:       input.Amount,
				BalanceAfter: balanceAfter,
				Reason:       input.Reason,
				ReferenceKey: input.ReferenceKey,
				LedgerID:     ledgerEntry.LedgerID,
			}, now),
		}
		emitted = append(emitted, service.currencyLedgerEventsLocked([]CurrencyLedgerEntry{ledgerEntry}, now)...)
	}
	return result, nil
}

// DebitWallet debits currency once for a player/reference pair and writes a currency ledger row.
func (service *WalletService) DebitWallet(input DebitWalletInput) (DebitWalletResult, error) {
	return service.debitWallet(input, true)
}

// DebitWalletWithoutRepository debits currency in memory without committing
// through the configured repository. Higher-level transaction owners use this
// to gather write sets before committing them atomically.
func (service *WalletService) DebitWalletWithoutRepository(input DebitWalletInput) (DebitWalletResult, error) {
	return service.debitWallet(input, false)
}

func (service *WalletService) debitWallet(input DebitWalletInput, persistRepository bool) (DebitWalletResult, error) {
	amount, err := input.validate()
	if err != nil {
		return DebitWalletResult{}, err
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()
	emitter = service.emitter

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

	result := DebitWalletResult{
		Balance:     balance,
		LedgerEntry: ledgerEntry,
	}
	if err := service.persistWalletMutationLocked(WalletMutationCommit{
		Balances:      []WalletBalance{balance},
		LedgerEntries: []CurrencyLedgerEntry{ledgerEntry},
		Reference: WalletMutationReference{
			PlayerID:      input.PlayerID,
			Operation:     debitWalletOperation,
			ReferenceKey:  input.ReferenceKey,
			LedgerEntries: []CurrencyLedgerEntry{ledgerEntry},
		},
		Counters: WalletCounters{LedgerSequence: service.nextLedgerSequence},
	}, persistRepository); err != nil {
		return DebitWalletResult{}, err
	}
	service.balances[walletBalanceKey{playerID: input.PlayerID, currency: input.Currency}] = balance
	service.currencyLedgerEntries = append(service.currencyLedgerEntries, ledgerEntry)
	service.indexCurrencyLedgerEntryLocked(ledgerEntry)

	service.debitReferences[reference] = result
	if emitter != nil {
		emitted = []events.EventEnvelope{
			service.walletMutationEventLocked(EventWalletDebited, WalletMutationPayload{
				PlayerID:     input.PlayerID,
				Currency:     input.Currency,
				Amount:       input.Amount,
				BalanceAfter: balanceAfter,
				Reason:       input.Reason,
				ReferenceKey: input.ReferenceKey,
				LedgerID:     ledgerEntry.LedgerID,
			}, now),
		}
		emitted = append(emitted, service.currencyLedgerEventsLocked([]CurrencyLedgerEntry{ledgerEntry}, now)...)
	}
	return result, nil
}

// TransferCurrency moves currency between players once for a source player/reference pair.
func (service *WalletService) TransferCurrency(input TransferCurrencyInput) (TransferCurrencyResult, error) {
	amount, err := input.validate()
	if err != nil {
		return TransferCurrencyResult{}, err
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()
	emitter = service.emitter

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

	result := TransferCurrencyResult{
		FromBalance:   fromBalance,
		ToBalance:     toBalance,
		LedgerEntries: []CurrencyLedgerEntry{debitEntry, creditEntry},
	}
	if err := service.persistWalletMutationLocked(WalletMutationCommit{
		Balances:      []WalletBalance{fromBalance, toBalance},
		LedgerEntries: result.LedgerEntries,
		Reference: WalletMutationReference{
			PlayerID:      input.FromPlayerID,
			Operation:     transferCurrencyOperation,
			ReferenceKey:  input.ReferenceKey,
			LedgerEntries: result.LedgerEntries,
		},
		Counters: WalletCounters{LedgerSequence: service.nextLedgerSequence},
	}, true); err != nil {
		return TransferCurrencyResult{}, err
	}
	service.balances[walletBalanceKey{playerID: input.FromPlayerID, currency: input.Currency}] = fromBalance
	service.balances[walletBalanceKey{playerID: input.ToPlayerID, currency: input.Currency}] = toBalance
	service.currencyLedgerEntries = append(service.currencyLedgerEntries, debitEntry, creditEntry)
	service.indexCurrencyLedgerEntryLocked(debitEntry)
	service.indexCurrencyLedgerEntryLocked(creditEntry)

	service.transferReferences[reference] = cloneTransferCurrencyResult(result)
	if emitter != nil {
		emitted = []events.EventEnvelope{
			service.walletMutationEventLocked(EventWalletDebited, WalletMutationPayload{
				PlayerID:     input.FromPlayerID,
				Currency:     input.Currency,
				Amount:       input.Amount,
				BalanceAfter: fromBalanceAfter,
				Reason:       input.Reason,
				ReferenceKey: input.ReferenceKey,
				LedgerID:     debitEntry.LedgerID,
			}, now),
		}
		emitted = append(emitted, service.currencyLedgerEventsLocked([]CurrencyLedgerEntry{debitEntry}, now)...)
		emitted = append(emitted, service.walletMutationEventLocked(EventWalletCredited, WalletMutationPayload{
			PlayerID:     input.ToPlayerID,
			Currency:     input.Currency,
			Amount:       input.Amount,
			BalanceAfter: toBalanceAfter,
			Reason:       input.Reason,
			ReferenceKey: input.ReferenceKey,
			LedgerID:     creditEntry.LedgerID,
		}, now))
		emitted = append(emitted, service.currencyLedgerEventsLocked([]CurrencyLedgerEntry{creditEntry}, now)...)
	}
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

// FindCurrencyLedgerEntry returns a copy of a ledger row matching lookup.
func (service *WalletService) FindCurrencyLedgerEntry(lookup CurrencyLedgerReferenceLookup) (CurrencyLedgerEntry, bool) {
	service.mu.Lock()
	defer service.mu.Unlock()

	entry, ok := service.currencyLedgerByReference[lookup]
	return entry, ok
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
	if input.FromPlayerID == input.ToPlayerID {
		return foundation.Money{}, ErrWalletSelfTransfer
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

func (service *WalletService) persistWalletBalanceLocked(balance WalletBalance) error {
	if service.repository == nil {
		return nil
	}
	return service.repository.UpsertWalletBalance(context.Background(), balance)
}

func (service *WalletService) persistWalletMutationLocked(commit WalletMutationCommit, persistRepository bool) error {
	if !persistRepository || service.repository == nil {
		return nil
	}
	return service.repository.CommitWalletMutation(context.Background(), commit)
}

func (service *WalletService) indexCurrencyLedgerEntryLocked(entry CurrencyLedgerEntry) {
	lookup := CurrencyLedgerReferenceLookup{
		PlayerID:     entry.PlayerID,
		Currency:     entry.Currency,
		Action:       entry.Action,
		Reason:       entry.Reason,
		ReferenceKey: entry.ReferenceKey,
	}
	if _, exists := service.currencyLedgerByReference[lookup]; !exists {
		service.currencyLedgerByReference[lookup] = entry
	}
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

func (service *WalletService) indexWalletMutationReferenceLocked(reference WalletMutationReference) {
	key := walletReferenceKey{
		playerID:     reference.PlayerID,
		operation:    reference.Operation,
		referenceKey: reference.ReferenceKey,
	}
	switch reference.Operation {
	case creditWalletOperation:
		entry := reference.LedgerEntries[0]
		service.creditReferences[key] = CreditWalletResult{
			Balance:     walletBalanceFromLedgerEntry(entry),
			LedgerEntry: entry,
		}
	case debitWalletOperation:
		entry := reference.LedgerEntries[0]
		service.debitReferences[key] = DebitWalletResult{
			Balance:     walletBalanceFromLedgerEntry(entry),
			LedgerEntry: entry,
		}
	case transferCurrencyOperation:
		service.transferReferences[key] = TransferCurrencyResult{
			FromBalance:   walletBalanceFromLedgerEntry(reference.LedgerEntries[0]),
			ToBalance:     walletBalanceFromLedgerEntry(reference.LedgerEntries[1]),
			LedgerEntries: append([]CurrencyLedgerEntry(nil), reference.LedgerEntries...),
		}
	}
}

func walletBalanceFromLedgerEntry(entry CurrencyLedgerEntry) WalletBalance {
	return WalletBalance{
		PlayerID:  entry.PlayerID,
		Currency:  entry.Currency,
		Balance:   entry.BalanceAfter,
		UpdatedAt: entry.CreatedAt,
	}
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
