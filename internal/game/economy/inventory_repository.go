package economy

import (
	"fmt"
	"strconv"
	"strings"
)

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
