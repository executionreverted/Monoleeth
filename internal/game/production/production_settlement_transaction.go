package production

import (
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
)

// ProductionSettlementTransactionStore is the durable adapter contract for planet
// production settlement windows. DB implementations should row-lock the planet
// production/storage/building rows, enforce idempotency, and append outbox rows
// behind this single operation.
type ProductionSettlementTransactionStore interface {
	ApplyProductionSettlementTransaction(ProductionSettlementTransactionInput) (ProductionSettlementTransactionResult, error)
}

// ProductionSettlementTransactionInput describes one server-timed production
// settlement transaction.
type ProductionSettlementTransactionInput struct {
	PlanetID           foundation.PlanetID
	SettledAt          time.Time
	RequireWholeOutput bool
}

// ProductionSettlementTransactionResult returns the state and audit rows
// committed by one production settlement transaction.
type ProductionSettlementTransactionResult struct {
	Settlement    PlanetProductionSettlementResult
	Reference     *SettlementReferenceRecord
	OutboxRecords []ProductionOutboxRecord
}

// ApplyProductionSettlementTransaction settles one planet production window
// under the store lock, returning only rows created by this transaction.
// Duplicate/no-op settlements return no new audit rows.
func (store *InMemoryStore) ApplyProductionSettlementTransaction(
	input ProductionSettlementTransactionInput,
) (ProductionSettlementTransactionResult, error) {
	if store == nil {
		return ProductionSettlementTransactionResult{}, ErrInvalidProductionSettlementConfig
	}
	if err := input.Validate(); err != nil {
		return ProductionSettlementTransactionResult{}, err
	}
	catalogRows, err := MVPCatalog()
	if err != nil {
		return ProductionSettlementTransactionResult{}, err
	}
	settledAt := input.SettledAt.UTC()

	store.mu.Lock()
	defer store.mu.Unlock()

	referenceCountBefore := len(store.references)
	outboxCountBefore := len(store.outbox)
	settlement, err := store.settlePlanetProductionLocked(input.PlanetID, settledAt, catalogRows, input.RequireWholeOutput)
	if err != nil {
		return ProductionSettlementTransactionResult{}, err
	}

	result := ProductionSettlementTransactionResult{
		Settlement:    settlement,
		OutboxRecords: cloneProductionOutboxRecords(store.outbox[outboxCountBefore:]),
	}
	if len(store.references) > referenceCountBefore && !settlement.ReferenceKey.IsZero() {
		if reference, ok := store.references[settlement.ReferenceKey]; ok {
			cloned := cloneSettlementReferenceRecord(reference)
			result.Reference = &cloned
		}
	}
	return result, nil
}

// Validate reports whether the production transaction input has server-owned
// planet and time facts.
func (input ProductionSettlementTransactionInput) Validate() error {
	if err := input.PlanetID.Validate(); err != nil {
		return err
	}
	if input.SettledAt.IsZero() {
		return fmt.Errorf("settled_at: %w", ErrZeroProductionTimestamp)
	}
	return nil
}
