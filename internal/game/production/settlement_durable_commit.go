package production

import (
	"errors"
	"fmt"

	"gameproject/internal/game/foundation"
)

var ErrInvalidSettlementDurableCommit = errors.New("invalid settlement durable commit")

// SettlementDurableCommitPlan validates the row bundle a future durable DB
// transaction must commit for one settlement window: idempotency reference,
// pending outbox rows, production/storage rows touched by the settlement, a
// route row for route settlements, and route storage ledger rows when the
// settlement moved route storage.
type SettlementDurableCommitPlan struct {
	Reference          SettlementReferenceRecord
	Outbox             SettlementOutboxDispatchPlan
	ProductionState    *PlanetProductionState
	StorageRows        []PlanetStorage
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
	return newSettlementDurableCommitPlan(reference, outbox, routeLedger, routeRow, nil, nil, false)
}

// NewSettlementDurableCommitPlanWithRows validates a settlement transaction
// result as a durable row bundle including the production/storage rows a future
// DB adapter would CAS-write in the same commit.
func NewSettlementDurableCommitPlanWithRows(
	reference *SettlementReferenceRecord,
	outbox []ProductionOutboxRecord,
	routeLedger []RouteStorageLedgerEntry,
	routeRow *AutomationRouteDurableRecord,
	productionState *PlanetProductionState,
	storageRows []PlanetStorage,
) (SettlementDurableCommitPlan, error) {
	return newSettlementDurableCommitPlan(reference, outbox, routeLedger, routeRow, productionState, storageRows, true)
}

func newSettlementDurableCommitPlan(
	reference *SettlementReferenceRecord,
	outbox []ProductionOutboxRecord,
	routeLedger []RouteStorageLedgerEntry,
	routeRow *AutomationRouteDurableRecord,
	productionState *PlanetProductionState,
	storageRows []PlanetStorage,
	requireRowEvidence bool,
) (SettlementDurableCommitPlan, error) {
	if reference == nil {
		if len(outbox) == 0 && len(routeLedger) == 0 && routeRow == nil && productionState == nil && len(storageRows) == 0 {
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
	clonedProductionState, err := validateSettlementDurableCommitProductionState(clonedReference, productionState, requireRowEvidence)
	if err != nil {
		return SettlementDurableCommitPlan{}, err
	}
	clonedStorageRows, err := validateSettlementDurableCommitStorageRows(clonedReference, clonedLedger, storageRows, requireRowEvidence)
	if err != nil {
		return SettlementDurableCommitPlan{}, err
	}
	return SettlementDurableCommitPlan{
		Reference:          clonedReference,
		Outbox:             dispatch,
		ProductionState:    clonedProductionState,
		StorageRows:        clonedStorageRows,
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

func validateSettlementDurableCommitProductionState(
	reference SettlementReferenceRecord,
	state *PlanetProductionState,
	requireRowEvidence bool,
) (*PlanetProductionState, error) {
	if state == nil {
		if requireRowEvidence && reference.Kind == SettlementKindProduction {
			return nil, fmt.Errorf("production_state: %w", ErrInvalidSettlementDurableCommit)
		}
		return nil, nil
	}
	if reference.Kind != SettlementKindProduction {
		return nil, fmt.Errorf("production_state: %w", ErrInvalidSettlementDurableCommit)
	}
	cloned := cloneProductionState(*state)
	if err := cloned.Validate(); err != nil {
		return nil, fmt.Errorf("production_state: %w: %v", ErrInvalidSettlementDurableCommit, err)
	}
	if cloned.PlanetID != reference.PlanetID ||
		!cloned.LastCalculatedAt.Equal(reference.AppliedAt) ||
		!cloned.UpdatedAt.Equal(reference.AppliedAt) {
		return nil, fmt.Errorf("production_state: %w", ErrInvalidSettlementDurableCommit)
	}
	return &cloned, nil
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

func validateSettlementDurableCommitStorageRows(
	reference SettlementReferenceRecord,
	ledgerRows []RouteStorageLedgerEntry,
	storageRows []PlanetStorage,
	requireRowEvidence bool,
) ([]PlanetStorage, error) {
	clonedRows := clonePlanetStorageRows(storageRows)
	if len(clonedRows) == 0 {
		if requireRowEvidence && reference.Kind == SettlementKindRoute && len(ledgerRows) > 0 {
			return nil, fmt.Errorf("storage_rows: %w", ErrInvalidSettlementDurableCommit)
		}
		return nil, nil
	}
	seen := make(map[foundation.PlanetID]PlanetStorage, len(clonedRows))
	for index, row := range clonedRows {
		if err := row.Validate(); err != nil {
			return nil, fmt.Errorf("storage_rows[%d]: %w: %v", index, ErrInvalidSettlementDurableCommit, err)
		}
		if !row.UpdatedAt.Equal(reference.AppliedAt) {
			return nil, fmt.Errorf("storage_rows[%d]: %w", index, ErrInvalidSettlementDurableCommit)
		}
		if _, ok := seen[row.PlanetID]; ok {
			return nil, fmt.Errorf("storage_rows[%d]: %w", index, ErrInvalidSettlementDurableCommit)
		}
		seen[row.PlanetID] = row
	}
	switch reference.Kind {
	case SettlementKindProduction:
		if len(clonedRows) > 1 || clonedRows[0].PlanetID != reference.PlanetID {
			return nil, fmt.Errorf("storage_rows: %w", ErrInvalidSettlementDurableCommit)
		}
	case SettlementKindRoute:
		if len(ledgerRows) == 0 {
			return nil, fmt.Errorf("storage_rows: %w", ErrInvalidSettlementDurableCommit)
		}
		required := make(map[foundation.PlanetID]struct{}, len(ledgerRows))
		for index, ledger := range ledgerRows {
			row, ok := seen[ledger.PlanetID]
			if !ok {
				return nil, fmt.Errorf("storage_rows[%d]: %w", index, ErrInvalidSettlementDurableCommit)
			}
			if row.QuantityOf(ledger.ItemID) != ledger.BalanceAfter {
				return nil, fmt.Errorf("storage_rows[%d]: %w", index, ErrInvalidSettlementDurableCommit)
			}
			required[ledger.PlanetID] = struct{}{}
		}
		if len(required) != len(clonedRows) {
			return nil, fmt.Errorf("storage_rows: %w", ErrInvalidSettlementDurableCommit)
		}
	default:
		return nil, fmt.Errorf("reference.kind: %w", ErrInvalidSettlementDurableCommit)
	}
	return clonedRows, nil
}
