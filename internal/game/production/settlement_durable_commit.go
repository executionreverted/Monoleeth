package production

import (
	"errors"
	"fmt"
)

var ErrInvalidSettlementDurableCommit = errors.New("invalid settlement durable commit")

// SettlementDurableCommitPlan validates the row bundle a future durable DB
// transaction must commit for one settlement window: idempotency reference,
// pending outbox rows, a route row for route settlements, and route storage
// ledger rows when the settlement moved route storage.
type SettlementDurableCommitPlan struct {
	Reference          SettlementReferenceRecord
	Outbox             SettlementOutboxDispatchPlan
	RouteRow           *AutomationRouteDurableRecord
	RouteStorageLedger []RouteStorageLedgerEntry
}

// ApplyDurableCommit validates and records this durable settlement plan through
// a durable commit adapter.
func (plan SettlementDurableCommitPlan) ApplyDurableCommit(
	store SettlementDurableCommitStore,
) (SettlementDurableCommitResult, error) {
	if store == nil {
		return SettlementDurableCommitResult{}, ErrInvalidSettlementDurableCommit
	}
	return store.ApplySettlementDurableCommitPlan(plan)
}

// NewSettlementDurableCommitPlan validates a settlement transaction result as a
// durable row bundle. Empty reference/outbox/ledger input is a no-op plan.
func NewSettlementDurableCommitPlan(
	reference *SettlementReferenceRecord,
	outbox []ProductionOutboxRecord,
	routeLedger []RouteStorageLedgerEntry,
	routeRows ...*AutomationRouteDurableRecord,
) (SettlementDurableCommitPlan, error) {
	routeRow, err := optionalSettlementDurableRouteRow(routeRows)
	if err != nil {
		return SettlementDurableCommitPlan{}, err
	}
	if reference == nil {
		if len(outbox) == 0 && len(routeLedger) == 0 && routeRow == nil {
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
	clonedRouteRow, err := validateSettlementDurableCommitRouteRow(clonedReference, routeRow)
	if err != nil {
		return SettlementDurableCommitPlan{}, err
	}
	return SettlementDurableCommitPlan{
		Reference:          clonedReference,
		Outbox:             dispatch,
		RouteRow:           clonedRouteRow,
		RouteStorageLedger: clonedLedger,
	}, nil
}

func optionalSettlementDurableRouteRow(
	routeRows []*AutomationRouteDurableRecord,
) (*AutomationRouteDurableRecord, error) {
	if len(routeRows) == 0 {
		return nil, nil
	}
	if len(routeRows) > 1 {
		return nil, fmt.Errorf("route_row: %w", ErrInvalidSettlementDurableCommit)
	}
	return routeRows[0], nil
}

func validateSettlementDurableCommitRouteRow(
	reference SettlementReferenceRecord,
	routeRow *AutomationRouteDurableRecord,
) (*AutomationRouteDurableRecord, error) {
	if reference.Kind == SettlementKindProduction {
		if routeRow != nil {
			return nil, fmt.Errorf("route_row: %w", ErrInvalidSettlementDurableCommit)
		}
		return nil, nil
	}
	if reference.Kind != SettlementKindRoute {
		return nil, fmt.Errorf("reference.kind: %w", ErrInvalidSettlementDurableCommit)
	}
	if routeRow == nil {
		return nil, fmt.Errorf("route_row: %w", ErrInvalidSettlementDurableCommit)
	}
	cloned := cloneAutomationRouteDurableRecord(*routeRow)
	if err := cloned.Route.Validate(); err != nil {
		return nil, fmt.Errorf("route_row.route: %w: %v", ErrInvalidSettlementDurableCommit, err)
	}
	if cloned.Route.RouteID != reference.RouteID ||
		cloned.ReferenceKey != reference.ReferenceKey ||
		!cloned.RecordedAt.Equal(reference.RecordedAt) {
		return nil, fmt.Errorf("route_row: %w", ErrInvalidSettlementDurableCommit)
	}
	if cloned.Revision == 0 ||
		!cloned.Route.LastCalculatedAt.Equal(reference.AppliedAt) ||
		!cloned.Route.UpdatedAt.Equal(reference.AppliedAt) {
		return nil, fmt.Errorf("route_row: %w", ErrInvalidSettlementDurableCommit)
	}
	return &cloned, nil
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
