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
	Settlement      PlanetProductionSettlementResult
	Reference       *SettlementReferenceRecord
	ProductionState *PlanetProductionState
	StorageRows     []PlanetStorage
	OutboxRecords   []ProductionOutboxRecord
}

// DurableCommitPlan returns the validated row bundle this transaction committed
// for future durable DB/publisher adapters. Duplicate/no-op transactions return
// an empty plan.
func (result ProductionSettlementTransactionResult) DurableCommitPlan() (SettlementDurableCommitPlan, error) {
	return NewSettlementDurableCommitPlanWithRows(result.Reference, result.OutboxRecords, nil, nil, result.ProductionState, result.StorageRows)
}

// ApplyDurableCommit validates and records the row bundle returned by this
// production settlement transaction through a durable commit adapter.
func (result ProductionSettlementTransactionResult) ApplyDurableCommit(
	store SettlementDurableCommitStore,
) (SettlementDurableCommitResult, error) {
	if store == nil {
		return SettlementDurableCommitResult{}, ErrInvalidSettlementDurableCommit
	}
	plan, err := result.DurableCommitPlan()
	if err != nil {
		return SettlementDurableCommitResult{}, err
	}
	return store.ApplySettlementDurableCommitPlan(plan)
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
	settledAt := input.SettledAt.UTC()

	store.mu.Lock()
	defer store.mu.Unlock()

	catalogRows, err := store.catalogLocked()
	if err != nil {
		return ProductionSettlementTransactionResult{}, err
	}
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
	if result.Reference != nil {
		state := cloneProductionState(settlement.After.State)
		result.ProductionState = &state
		if settlement.After.Storage.UpdatedAt.Equal(settledAt) {
			result.StorageRows = []PlanetStorage{clonePlanetStorage(settlement.After.Storage)}
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
