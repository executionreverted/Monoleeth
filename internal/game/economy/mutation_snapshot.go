package economy

// WalletMutationSnapshot captures in-memory wallet mutation state for a
// higher-level domain rollback skeleton.
type WalletMutationSnapshot struct {
	nextLedgerSequence        int64
	balances                  map[walletBalanceKey]WalletBalance
	currencyLedgerEntries     []CurrencyLedgerEntry
	currencyLedgerByReference map[CurrencyLedgerReferenceLookup]CurrencyLedgerEntry
	creditReferences          map[walletReferenceKey]CreditWalletResult
	debitReferences           map[walletReferenceKey]DebitWalletResult
	transferReferences        map[walletReferenceKey]TransferCurrencyResult
}

// SnapshotMutationState returns an opaque in-memory wallet mutation snapshot.
func (service *WalletService) SnapshotMutationState() WalletMutationSnapshot {
	service.mu.Lock()
	defer service.mu.Unlock()

	return WalletMutationSnapshot{
		nextLedgerSequence:        service.nextLedgerSequence,
		balances:                  cloneWalletBalances(service.balances),
		currencyLedgerEntries:     append([]CurrencyLedgerEntry(nil), service.currencyLedgerEntries...),
		currencyLedgerByReference: cloneCurrencyLedgerByReference(service.currencyLedgerByReference),
		creditReferences:          cloneCreditWalletReferences(service.creditReferences),
		debitReferences:           cloneDebitWalletReferences(service.debitReferences),
		transferReferences:        cloneTransferWalletReferences(service.transferReferences),
	}
}

// RestoreMutationState restores an opaque in-memory wallet mutation snapshot.
func (service *WalletService) RestoreMutationState(snapshot WalletMutationSnapshot) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.nextLedgerSequence = snapshot.nextLedgerSequence
	service.balances = cloneWalletBalances(snapshot.balances)
	service.currencyLedgerEntries = append([]CurrencyLedgerEntry(nil), snapshot.currencyLedgerEntries...)
	service.currencyLedgerByReference = cloneCurrencyLedgerByReference(snapshot.currencyLedgerByReference)
	service.creditReferences = cloneCreditWalletReferences(snapshot.creditReferences)
	service.debitReferences = cloneDebitWalletReferences(snapshot.debitReferences)
	service.transferReferences = cloneTransferWalletReferences(snapshot.transferReferences)
}

// InventoryMutationSnapshot captures in-memory inventory mutation state for a
// higher-level domain rollback skeleton.
type InventoryMutationSnapshot struct {
	nextItemSequence     int64
	nextLedgerSequence   int64
	stackableItems       []StackableItem
	instanceItems        []InstanceItem
	itemLedgerEntries    []ItemLedgerEntry
	addItemReferences    map[inventoryReferenceKey]AddItemResult
	moveItemReferences   map[inventoryReferenceKey]MoveItemResult
	removeItemReferences map[inventoryReferenceKey]RemoveItemResult
}

// SnapshotMutationState returns an opaque in-memory inventory mutation snapshot.
func (service *InventoryService) SnapshotMutationState() InventoryMutationSnapshot {
	service.mu.Lock()
	defer service.mu.Unlock()

	return InventoryMutationSnapshot{
		nextItemSequence:     service.nextItemSequence,
		nextLedgerSequence:   service.nextLedgerSequence,
		stackableItems:       append([]StackableItem(nil), service.stackableItems...),
		instanceItems:        append([]InstanceItem(nil), service.instanceItems...),
		itemLedgerEntries:    append([]ItemLedgerEntry(nil), service.itemLedgerEntries...),
		addItemReferences:    cloneAddItemReferences(service.addItemReferences),
		moveItemReferences:   cloneMoveItemReferences(service.moveItemReferences),
		removeItemReferences: cloneRemoveItemReferences(service.removeItemReferences),
	}
}

// RestoreMutationState restores an opaque in-memory inventory mutation snapshot.
func (service *InventoryService) RestoreMutationState(snapshot InventoryMutationSnapshot) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.nextItemSequence = snapshot.nextItemSequence
	service.nextLedgerSequence = snapshot.nextLedgerSequence
	service.stackableItems = append([]StackableItem(nil), snapshot.stackableItems...)
	service.instanceItems = append([]InstanceItem(nil), snapshot.instanceItems...)
	service.itemLedgerEntries = append([]ItemLedgerEntry(nil), snapshot.itemLedgerEntries...)
	service.addItemReferences = cloneAddItemReferences(snapshot.addItemReferences)
	service.moveItemReferences = cloneMoveItemReferences(snapshot.moveItemReferences)
	service.removeItemReferences = cloneRemoveItemReferences(snapshot.removeItemReferences)
}

func cloneWalletBalances(balances map[walletBalanceKey]WalletBalance) map[walletBalanceKey]WalletBalance {
	if balances == nil {
		return nil
	}
	cloned := make(map[walletBalanceKey]WalletBalance, len(balances))
	for key, balance := range balances {
		cloned[key] = balance
	}
	return cloned
}

func cloneCurrencyLedgerByReference(entries map[CurrencyLedgerReferenceLookup]CurrencyLedgerEntry) map[CurrencyLedgerReferenceLookup]CurrencyLedgerEntry {
	if entries == nil {
		return nil
	}
	cloned := make(map[CurrencyLedgerReferenceLookup]CurrencyLedgerEntry, len(entries))
	for key, entry := range entries {
		cloned[key] = entry
	}
	return cloned
}

func cloneCreditWalletReferences(references map[walletReferenceKey]CreditWalletResult) map[walletReferenceKey]CreditWalletResult {
	if references == nil {
		return nil
	}
	cloned := make(map[walletReferenceKey]CreditWalletResult, len(references))
	for key, result := range references {
		cloned[key] = result
	}
	return cloned
}

func cloneDebitWalletReferences(references map[walletReferenceKey]DebitWalletResult) map[walletReferenceKey]DebitWalletResult {
	if references == nil {
		return nil
	}
	cloned := make(map[walletReferenceKey]DebitWalletResult, len(references))
	for key, result := range references {
		cloned[key] = result
	}
	return cloned
}

func cloneTransferWalletReferences(references map[walletReferenceKey]TransferCurrencyResult) map[walletReferenceKey]TransferCurrencyResult {
	if references == nil {
		return nil
	}
	cloned := make(map[walletReferenceKey]TransferCurrencyResult, len(references))
	for key, result := range references {
		cloned[key] = cloneTransferCurrencyResult(result)
	}
	return cloned
}

func cloneAddItemReferences(references map[inventoryReferenceKey]AddItemResult) map[inventoryReferenceKey]AddItemResult {
	if references == nil {
		return nil
	}
	cloned := make(map[inventoryReferenceKey]AddItemResult, len(references))
	for key, result := range references {
		cloned[key] = cloneAddItemResult(result)
	}
	return cloned
}

func cloneRemoveItemReferences(references map[inventoryReferenceKey]RemoveItemResult) map[inventoryReferenceKey]RemoveItemResult {
	if references == nil {
		return nil
	}
	cloned := make(map[inventoryReferenceKey]RemoveItemResult, len(references))
	for key, result := range references {
		cloned[key] = cloneRemoveItemResult(result)
	}
	return cloned
}
