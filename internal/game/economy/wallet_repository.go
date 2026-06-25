package economy

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gameproject/internal/game/foundation"
)

var (
	ErrInvalidWalletCounter            = errors.New("invalid wallet counter")
	ErrInvalidWalletReferenceOperation = errors.New("invalid wallet reference operation")
	ErrInvalidWalletReferenceLedgerSet = errors.New("invalid wallet reference ledger set")
)

// WalletCounters records the last allocated wallet service sequence.
type WalletCounters struct {
	LedgerSequence int64
}

// WalletMutationOperation identifies the wallet command that owns a reference.
type WalletMutationOperation string

const (
	WalletMutationOperationCredit   WalletMutationOperation = "credit_wallet"
	WalletMutationOperationDebit    WalletMutationOperation = "debit_wallet"
	WalletMutationOperationTransfer WalletMutationOperation = "transfer_currency"
)

// WalletMutationReference records the durable duplicate result for a wallet mutation.
type WalletMutationReference struct {
	PlayerID      foundation.PlayerID
	Operation     WalletMutationOperation
	ReferenceKey  foundation.IdempotencyKey
	LedgerEntries []CurrencyLedgerEntry
}

// WalletMutationCommit is the durable write set for one wallet mutation.
type WalletMutationCommit struct {
	Balances      []WalletBalance
	LedgerEntries []CurrencyLedgerEntry
	Reference     WalletMutationReference
	Counters      WalletCounters
}

func (counters WalletCounters) Validate() error {
	if counters.LedgerSequence < 0 {
		return fmt.Errorf("ledger sequence %d: %w", counters.LedgerSequence, ErrInvalidWalletCounter)
	}
	return nil
}

func (operation WalletMutationOperation) String() string {
	return string(operation)
}

func (operation WalletMutationOperation) Validate() error {
	switch operation {
	case WalletMutationOperationCredit, WalletMutationOperationDebit, WalletMutationOperationTransfer:
		return nil
	default:
		return fmt.Errorf("wallet reference operation %q: %w", operation, ErrInvalidWalletReferenceOperation)
	}
}

func (reference WalletMutationReference) Validate() error {
	if err := reference.PlayerID.Validate(); err != nil {
		return err
	}
	if err := reference.Operation.Validate(); err != nil {
		return err
	}
	if err := reference.ReferenceKey.Validate(); err != nil {
		return err
	}
	for _, entry := range reference.LedgerEntries {
		if err := entry.Validate(); err != nil {
			return err
		}
		if entry.ReferenceKey != reference.ReferenceKey {
			return fmt.Errorf("wallet reference key %q ledger key %q", reference.ReferenceKey, entry.ReferenceKey)
		}
	}

	switch reference.Operation {
	case creditWalletOperation:
		return validateSingleWalletReferenceLedger(reference, LedgerActionIncrease)
	case debitWalletOperation:
		return validateSingleWalletReferenceLedger(reference, LedgerActionDecrease)
	case transferCurrencyOperation:
		return validateTransferWalletReferenceLedgers(reference)
	}
	return nil
}

func (commit WalletMutationCommit) Validate() error {
	for _, balance := range commit.Balances {
		if err := balance.Validate(); err != nil {
			return err
		}
	}
	for _, entry := range commit.LedgerEntries {
		if err := entry.Validate(); err != nil {
			return err
		}
	}
	if err := commit.Reference.Validate(); err != nil {
		return err
	}
	if len(commit.Reference.LedgerEntries) != len(commit.LedgerEntries) {
		return fmt.Errorf("wallet reference ledger count %d commit ledger count %d: %w", len(commit.Reference.LedgerEntries), len(commit.LedgerEntries), ErrInvalidWalletReferenceLedgerSet)
	}
	for index := range commit.LedgerEntries {
		if commit.Reference.LedgerEntries[index].LedgerID != commit.LedgerEntries[index].LedgerID {
			return fmt.Errorf("wallet reference ledger %q commit ledger %q: %w", commit.Reference.LedgerEntries[index].LedgerID, commit.LedgerEntries[index].LedgerID, ErrInvalidWalletReferenceLedgerSet)
		}
	}
	return commit.Counters.Validate()
}

func safeWalletLedgerSequence(stored int64, entries []CurrencyLedgerEntry) int64 {
	maxSequence := stored
	for _, entry := range entries {
		maxSequence = max(maxSequence, currencyLedgerSequenceSuffix(entry.LedgerID))
	}
	return maxSequence
}

func validateSingleWalletReferenceLedger(reference WalletMutationReference, action LedgerAction) error {
	if len(reference.LedgerEntries) != 1 {
		return fmt.Errorf("wallet %s reference ledger count %d: %w", reference.Operation, len(reference.LedgerEntries), ErrInvalidWalletReferenceLedgerSet)
	}
	entry := reference.LedgerEntries[0]
	if entry.PlayerID != reference.PlayerID {
		return fmt.Errorf("wallet reference player %q ledger player %q", reference.PlayerID, entry.PlayerID)
	}
	if entry.Action != action {
		return fmt.Errorf("wallet reference action %q want %q: %w", entry.Action, action, ErrInvalidWalletReferenceLedgerSet)
	}
	return nil
}

func validateTransferWalletReferenceLedgers(reference WalletMutationReference) error {
	if len(reference.LedgerEntries) != 2 {
		return fmt.Errorf("wallet transfer reference ledger count %d: %w", len(reference.LedgerEntries), ErrInvalidWalletReferenceLedgerSet)
	}
	debitEntry := reference.LedgerEntries[0]
	creditEntry := reference.LedgerEntries[1]
	if debitEntry.PlayerID != reference.PlayerID {
		return fmt.Errorf("wallet transfer reference player %q debit ledger player %q", reference.PlayerID, debitEntry.PlayerID)
	}
	if debitEntry.Action != LedgerActionDecrease || creditEntry.Action != LedgerActionIncrease {
		return fmt.Errorf("wallet transfer reference actions %q/%q: %w", debitEntry.Action, creditEntry.Action, ErrInvalidWalletReferenceLedgerSet)
	}
	if debitEntry.Currency != creditEntry.Currency ||
		debitEntry.Amount != creditEntry.Amount ||
		debitEntry.Reason != creditEntry.Reason ||
		debitEntry.ReferenceKey != creditEntry.ReferenceKey {
		return fmt.Errorf("wallet transfer reference ledger mismatch: %w", ErrInvalidWalletReferenceLedgerSet)
	}
	return nil
}

func currencyLedgerSequenceSuffix(id LedgerID) int64 {
	value := strings.TrimPrefix(id.String(), "currency-ledger-")
	if value == id.String() {
		return 0
	}
	sequence, err := strconv.ParseInt(value, 10, 64)
	if err != nil || sequence < 0 {
		return 0
	}
	return sequence
}
