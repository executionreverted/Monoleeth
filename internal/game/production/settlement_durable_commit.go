package production

import (
	"errors"
	"fmt"
)

var ErrInvalidSettlementDurableCommit = errors.New("invalid settlement durable commit")

// SettlementDurableCommitPlan validates the row bundle a future durable DB
// transaction must commit for one settlement window: idempotency reference,
// pending outbox rows, and route storage ledger rows when the settlement moved
// route storage.
type SettlementDurableCommitPlan struct {
	Reference          SettlementReferenceRecord
	Outbox             SettlementOutboxDispatchPlan
	RouteStorageLedger []RouteStorageLedgerEntry
}

// NewSettlementDurableCommitPlan validates a settlement transaction result as a
// durable row bundle. Empty reference/outbox/ledger input is a no-op plan.
func NewSettlementDurableCommitPlan(
	reference *SettlementReferenceRecord,
	outbox []ProductionOutboxRecord,
	routeLedger []RouteStorageLedgerEntry,
) (SettlementDurableCommitPlan, error) {
	if reference == nil {
		if len(outbox) == 0 && len(routeLedger) == 0 {
			return SettlementDurableCommitPlan{}, nil
		}
		return SettlementDurableCommitPlan{}, fmt.Errorf("reference: %w", ErrInvalidSettlementDurableCommit)
	}
	dispatch, err := NewSettlementOutboxDispatchPlan(reference, outbox)
	if err != nil {
		return SettlementDurableCommitPlan{}, fmt.Errorf("%w: %v", ErrInvalidSettlementDurableCommit, err)
	}
	clonedReference := cloneSettlementReferenceRecord(*reference)
	clonedLedger := cloneRouteStorageLedgerEntries(routeLedger)
	if err := validateSettlementDurableCommitLedger(clonedReference, clonedLedger); err != nil {
		return SettlementDurableCommitPlan{}, err
	}
	return SettlementDurableCommitPlan{
		Reference:          clonedReference,
		Outbox:             dispatch,
		RouteStorageLedger: clonedLedger,
	}, nil
}

func validateSettlementDurableCommitLedger(reference SettlementReferenceRecord, rows []RouteStorageLedgerEntry) error {
	if reference.Kind == SettlementKindProduction && len(rows) > 0 {
		return fmt.Errorf("route_storage_ledger: %w", ErrInvalidSettlementDurableCommit)
	}
	for index, row := range rows {
		if err := row.Validate(); err != nil {
			return fmt.Errorf("route_storage_ledger[%d]: %w: %v", index, ErrInvalidSettlementDurableCommit, err)
		}
		if row.ReferenceKey != reference.ReferenceKey ||
			row.SettlementWindow != reference.SettlementWindow ||
			row.RouteID != reference.RouteID {
			return fmt.Errorf("route_storage_ledger[%d]: %w", index, ErrInvalidSettlementDurableCommit)
		}
	}
	return nil
}
