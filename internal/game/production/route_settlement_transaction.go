package production

import (
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
)

// RouteSettlementTransactionStore is the durable adapter seam for route
// settlement windows. Production DB implementations should enforce row locks,
// CAS/idempotency rows, storage ledger writes, and outbox appends behind this
// single operation.
type RouteSettlementTransactionStore interface {
	ApplyRouteSettlementTransaction(RouteSettlementTransactionInput) (RouteSettlementTransactionResult, error)
}

// RouteSettlementTransactionInput describes the durable route settlement
// transaction boundary. A DB adapter should row-lock the route/storage rows,
// enforce the settlement idempotency reference, and append outbox rows in one
// commit.
type RouteSettlementTransactionInput struct {
	OwnerPlayerID foundation.PlayerID
	RouteID       foundation.RouteID
	SettledAt     time.Time
	LossRoller    RouteLossRoller
}

// RouteSettlementTransactionResult returns the state and audit rows committed
// by one route settlement transaction.
type RouteSettlementTransactionResult struct {
	Settlement    RouteSettlementResult
	Reference     *SettlementReferenceRecord
	OutboxRecords []ProductionOutboxRecord
	StorageLedger []RouteStorageLedgerEntry
}

// DurableCommitPlan returns the validated row bundle this transaction committed
// for future durable DB/publisher adapters. Duplicate/no-op transactions return
// an empty plan.
func (result RouteSettlementTransactionResult) DurableCommitPlan() (SettlementDurableCommitPlan, error) {
	return NewSettlementDurableCommitPlan(result.Reference, result.OutboxRecords, result.StorageLedger)
}

// ApplyRouteSettlementTransaction settles one owner-scoped route under the
// store lock, returning only the reference/outbox/ledger rows created by this
// transaction. Duplicate/no-op settlements return no new audit rows.
func (store *InMemoryStore) ApplyRouteSettlementTransaction(
	input RouteSettlementTransactionInput,
) (RouteSettlementTransactionResult, error) {
	if store == nil {
		return RouteSettlementTransactionResult{}, ErrInvalidRouteSettlementConfig
	}
	if err := input.Validate(); err != nil {
		return RouteSettlementTransactionResult{}, err
	}
	lossRoller := input.LossRoller
	if lossRoller == nil {
		lossRoller = defaultRouteLossRoller{}
	}
	settledAt := input.SettledAt.UTC()

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if err := store.requireRouteOwnerLocked(input.OwnerPlayerID, input.RouteID); err != nil {
		return RouteSettlementTransactionResult{}, err
	}
	referenceCountBefore := len(store.references)
	outboxCountBefore := len(store.outbox)
	ledgerCountBefore := len(store.routeStorageLedger)

	settlement, err := store.settleRouteLocked(input.RouteID, settledAt, lossRoller)
	if err != nil {
		return RouteSettlementTransactionResult{}, err
	}

	result := RouteSettlementTransactionResult{
		Settlement:    settlement,
		OutboxRecords: cloneProductionOutboxRecords(store.outbox[outboxCountBefore:]),
		StorageLedger: cloneRouteStorageLedgerEntries(store.routeStorageLedger[ledgerCountBefore:]),
	}
	if len(store.references) > referenceCountBefore && !settlement.ReferenceKey.IsZero() {
		if reference, ok := store.references[settlement.ReferenceKey]; ok {
			cloned := cloneSettlementReferenceRecord(reference)
			result.Reference = &cloned
		}
	}
	return result, nil
}

// Validate reports whether the route transaction input has server-owned owner,
// route, and time facts.
func (input RouteSettlementTransactionInput) Validate() error {
	if err := input.OwnerPlayerID.Validate(); err != nil {
		return err
	}
	if err := input.RouteID.Validate(); err != nil {
		return err
	}
	if input.SettledAt.IsZero() {
		return fmt.Errorf("settled_at: %w", ErrZeroProductionTimestamp)
	}
	return nil
}
