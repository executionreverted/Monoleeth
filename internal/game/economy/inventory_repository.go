package economy

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gameproject/internal/game/foundation"
)

// ErrInvalidInventoryReferenceLedgerSet reports a malformed durable inventory reference result.
var ErrInvalidInventoryReferenceLedgerSet = errors.New("invalid inventory reference ledger set")

func (counters InventoryCounters) Validate() error {
	if counters.ItemSequence < 0 {
		return fmt.Errorf("item sequence %d: %w", counters.ItemSequence, ErrInvalidInventoryCounter)
	}
	if counters.LedgerSequence < 0 {
		return fmt.Errorf("ledger sequence %d: %w", counters.LedgerSequence, ErrInvalidInventoryCounter)
	}
	return nil
}

func (reference AddItemReference) Validate() error {
	if err := reference.PlayerID.Validate(); err != nil {
		return err
	}
	if err := reference.ReferenceKey.Validate(); err != nil {
		return err
	}
	if err := reference.Result.LedgerEntry.Validate(); err != nil {
		return err
	}
	if reference.Result.LedgerEntry.PlayerID != reference.PlayerID {
		return fmt.Errorf("add item reference player %q ledger player %q", reference.PlayerID, reference.Result.LedgerEntry.PlayerID)
	}
	if reference.Result.LedgerEntry.ReferenceKey != reference.ReferenceKey {
		return fmt.Errorf("add item reference key %q ledger key %q", reference.ReferenceKey, reference.Result.LedgerEntry.ReferenceKey)
	}
	for _, item := range reference.Result.StackableItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range reference.Result.InstanceItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (reference MoveItemReference) Validate() error {
	if err := reference.PlayerID.Validate(); err != nil {
		return err
	}
	if err := reference.ReferenceKey.Validate(); err != nil {
		return err
	}
	return validateMoveItemReferenceResult(reference.PlayerID, reference.ReferenceKey, reference.Result)
}

func (reference RemoveItemReference) Validate() error {
	if err := reference.PlayerID.Validate(); err != nil {
		return err
	}
	if err := reference.ReferenceKey.Validate(); err != nil {
		return err
	}
	return validateRemoveItemReferenceResult(reference.PlayerID, reference.ReferenceKey, reference.Result)
}

func (commit InventoryAddItemCommit) Validate() error {
	for _, item := range commit.StackableItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range commit.InstanceItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	if err := commit.LedgerEntry.Validate(); err != nil {
		return err
	}
	if err := commit.Reference.Validate(); err != nil {
		return err
	}
	if commit.Reference.Result.LedgerEntry.LedgerID != commit.LedgerEntry.LedgerID {
		return fmt.Errorf("add item reference ledger %q commit ledger %q", commit.Reference.Result.LedgerEntry.LedgerID, commit.LedgerEntry.LedgerID)
	}
	return commit.Counters.Validate()
}

func (commit InventoryMoveItemCommit) Validate() error {
	for _, item := range commit.StackableItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range commit.DeletedStackableItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range commit.InstanceItems {
		if err := item.Validate(); err != nil {
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
	if len(commit.Reference.Result.LedgerEntries) != len(commit.LedgerEntries) {
		return fmt.Errorf("move item reference ledger count %d commit ledger count %d: %w", len(commit.Reference.Result.LedgerEntries), len(commit.LedgerEntries), ErrInvalidInventoryReferenceLedgerSet)
	}
	for index := range commit.LedgerEntries {
		if commit.Reference.Result.LedgerEntries[index].LedgerID != commit.LedgerEntries[index].LedgerID {
			return fmt.Errorf("move item reference ledger %q commit ledger %q: %w", commit.Reference.Result.LedgerEntries[index].LedgerID, commit.LedgerEntries[index].LedgerID, ErrInvalidInventoryReferenceLedgerSet)
		}
	}
	return commit.Counters.Validate()
}

func (commit InventoryRemoveItemCommit) Validate() error {
	for _, item := range commit.StackableItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range commit.DeletedStackableItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range commit.DeletedInstanceItems {
		if err := item.Validate(); err != nil {
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
	if len(commit.Reference.Result.LedgerEntries) != len(commit.LedgerEntries) {
		return fmt.Errorf("remove item reference ledger count %d commit ledger count %d: %w", len(commit.Reference.Result.LedgerEntries), len(commit.LedgerEntries), ErrInvalidInventoryReferenceLedgerSet)
	}
	for index := range commit.LedgerEntries {
		if commit.Reference.Result.LedgerEntries[index].LedgerID != commit.LedgerEntries[index].LedgerID {
			return fmt.Errorf("remove item reference ledger %q commit ledger %q: %w", commit.Reference.Result.LedgerEntries[index].LedgerID, commit.LedgerEntries[index].LedgerID, ErrInvalidInventoryReferenceLedgerSet)
		}
	}
	return commit.Counters.Validate()
}

func validateMoveItemReferenceResult(playerID foundation.PlayerID, referenceKey foundation.IdempotencyKey, result MoveItemResult) error {
	if len(result.LedgerEntries) != 2 {
		return fmt.Errorf("move item reference ledger count %d: %w", len(result.LedgerEntries), ErrInvalidInventoryReferenceLedgerSet)
	}
	sourceEntry := result.LedgerEntries[0]
	destinationEntry := result.LedgerEntries[1]
	if sourceEntry.PlayerID != playerID {
		return fmt.Errorf("move item reference player %q source ledger player %q", playerID, sourceEntry.PlayerID)
	}
	if sourceEntry.ReferenceKey != referenceKey || destinationEntry.ReferenceKey != referenceKey {
		return fmt.Errorf("move item reference key %q ledger keys %q/%q: %w", referenceKey, sourceEntry.ReferenceKey, destinationEntry.ReferenceKey, ErrInvalidInventoryReferenceLedgerSet)
	}
	if sourceEntry.Action != LedgerActionDecrease || destinationEntry.Action != LedgerActionIncrease {
		return fmt.Errorf("move item reference actions %q/%q: %w", sourceEntry.Action, destinationEntry.Action, ErrInvalidInventoryReferenceLedgerSet)
	}
	if sourceEntry.ItemID != destinationEntry.ItemID ||
		sourceEntry.ItemInstanceID != destinationEntry.ItemInstanceID ||
		sourceEntry.Quantity != destinationEntry.Quantity ||
		sourceEntry.Reason != destinationEntry.Reason {
		return fmt.Errorf("move item reference ledger mismatch: %w", ErrInvalidInventoryReferenceLedgerSet)
	}
	for _, entry := range result.LedgerEntries {
		if err := entry.Validate(); err != nil {
			return err
		}
	}
	for _, item := range result.StackableItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range result.DeletedStackableItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range result.InstanceItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validateRemoveItemReferenceResult(playerID foundation.PlayerID, referenceKey foundation.IdempotencyKey, result RemoveItemResult) error {
	if len(result.LedgerEntries) != 1 {
		return fmt.Errorf("remove item reference ledger count %d: %w", len(result.LedgerEntries), ErrInvalidInventoryReferenceLedgerSet)
	}
	entry := result.LedgerEntries[0]
	if err := entry.Validate(); err != nil {
		return err
	}
	if entry.PlayerID != playerID {
		return fmt.Errorf("remove item reference player %q ledger player %q", playerID, entry.PlayerID)
	}
	if entry.ReferenceKey != referenceKey {
		return fmt.Errorf("remove item reference key %q ledger key %q: %w", referenceKey, entry.ReferenceKey, ErrInvalidInventoryReferenceLedgerSet)
	}
	if entry.Action != LedgerActionDecrease {
		return fmt.Errorf("remove item reference action %q: %w", entry.Action, ErrInvalidInventoryReferenceLedgerSet)
	}
	for _, item := range result.StackableItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range result.DeletedStackableItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range result.InstanceItems {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func safeInventoryItemSequence(stored int64, stackables []StackableItem, instances []InstanceItem) int64 {
	maxSequence := stored
	for _, item := range stackables {
		maxSequence = max(maxSequence, generatedSequenceSuffix(item.ItemInstanceID.String()))
	}
	for _, item := range instances {
		maxSequence = max(maxSequence, generatedSequenceSuffix(item.ItemInstanceID.String()))
	}
	return maxSequence
}

func safeInventoryLedgerSequence(stored int64, entries []ItemLedgerEntry) int64 {
	maxSequence := stored
	for _, entry := range entries {
		maxSequence = max(maxSequence, ledgerSequenceSuffix(entry.LedgerID))
	}
	return maxSequence
}

func generatedSequenceSuffix(id string) int64 {
	index := strings.LastIndex(id, "-")
	if index < 0 || index == len(id)-1 {
		return 0
	}
	sequence, err := strconv.ParseInt(id[index+1:], 10, 64)
	if err != nil || sequence < 0 {
		return 0
	}
	return sequence
}

func ledgerSequenceSuffix(id LedgerID) int64 {
	value := strings.TrimPrefix(id.String(), "item-ledger-")
	if value == id.String() {
		return 0
	}
	sequence, err := strconv.ParseInt(value, 10, 64)
	if err != nil || sequence < 0 {
		return 0
	}
	return sequence
}
