package production

import (
	"encoding/json"
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
	RouteRow      *AutomationRouteDurableRecord
	OutboxRecords []ProductionOutboxRecord
	StorageLedger []RouteStorageLedgerEntry
	StorageRows   []PlanetStorage
}

// DurableCommitPlan returns the validated row bundle this transaction committed
// for future durable DB/publisher adapters. Duplicate/no-op transactions return
// an empty plan.
func (result RouteSettlementTransactionResult) DurableCommitPlan() (SettlementDurableCommitPlan, error) {
	return NewSettlementDurableCommitPlanWithRows(result.Reference, result.OutboxRecords, result.StorageLedger, result.RouteRow, nil, result.StorageRows)
}

// ApplyDurableCommit validates and records the row bundle returned by this
// route settlement transaction through a durable commit adapter.
func (result RouteSettlementTransactionResult) ApplyDurableCommit(
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

	if _, routeExists := store.routes[input.RouteID]; !routeExists {
		if replay, ok, err := store.routeSettlementTransactionReplayLocked(input, settledAt); err != nil || ok {
			if err != nil {
				return RouteSettlementTransactionResult{}, err
			}
			return replay, nil
		}
		if _, _, err := store.restoreAutomationRouteReadModelFromDurableLocked(input.OwnerPlayerID, input.RouteID); err != nil {
			return RouteSettlementTransactionResult{}, err
		}
	}
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
	if result.Reference != nil {
		routeRow, ok, err := store.committedAutomationRouteDurableRecordByReferenceLocked(result.Reference.ReferenceKey)
		if err != nil {
			return RouteSettlementTransactionResult{}, err
		}
		if ok {
			result.RouteRow = cloneAutomationRouteDurableRecordPointer(&routeRow)
		}
		result.StorageRows = store.routeSettlementStorageRowsLocked(result.StorageLedger)
	}
	return result, nil
}

func (store *InMemoryStore) routeSettlementTransactionReplayLocked(
	input RouteSettlementTransactionInput,
	settledAt time.Time,
) (RouteSettlementTransactionResult, bool, error) {
	var record AutomationRouteDurableRecord
	var ok bool
	var err error
	if store.routeDurable != nil {
		record, ok, err = store.routeDurable.CommittedAutomationRouteDurableRecord(input.RouteID)
		if err != nil {
			return RouteSettlementTransactionResult{}, false, err
		}
	} else {
		record, ok = store.routeDurableRecords[input.RouteID]
	}
	if !ok {
		return RouteSettlementTransactionResult{}, false, nil
	}
	if err := validateAutomationRouteDurableRecordForRoute(record, input.RouteID); err != nil {
		return RouteSettlementTransactionResult{}, false, err
	}
	if record.Route.OwnerPlayerID != input.OwnerPlayerID {
		return RouteSettlementTransactionResult{}, false, fmt.Errorf("route %q owner %q: %w", input.RouteID, input.OwnerPlayerID, ErrRouteOwnerMismatch)
	}
	if !record.RecordedAt.Equal(settledAt) {
		return RouteSettlementTransactionResult{}, false, nil
	}
	reference, ok := store.references[record.ReferenceKey]
	if !ok {
		return RouteSettlementTransactionResult{}, false, nil
	}
	if err := validateRouteSettlementTransactionReplayReference(input, settledAt, record, reference); err != nil {
		return RouteSettlementTransactionResult{}, false, err
	}

	route := cloneAutomationRoute(record.Route)
	settlement := newRouteSettlementResult(route, settledAt)
	settlement.ReferenceKey = reference.ReferenceKey
	settlement.SettlementWindow = reference.SettlementWindow
	settlement.NoOp = true
	outboxRecords := store.routeSettlementOutboxRecordsLocked(reference.ReferenceKey, reference.SettlementWindow)
	ledgerRows := store.routeSettlementLedgerRowsLocked(input.RouteID, reference.ReferenceKey, reference.SettlementWindow)
	routeRow := cloneAutomationRouteDurableRecordPointer(&record)
	clonedReference := cloneSettlementReferenceRecord(reference)
	result := RouteSettlementTransactionResult{
		Settlement:    settlement,
		Reference:     &clonedReference,
		RouteRow:      routeRow,
		OutboxRecords: outboxRecords,
		StorageLedger: ledgerRows,
		StorageRows:   store.routeSettlementStorageRowsLocked(ledgerRows),
	}
	if err := validateRouteSettlementTransactionReplayRows(result); err != nil {
		return RouteSettlementTransactionResult{}, false, err
	}
	if _, err := result.DurableCommitPlan(); err != nil {
		return RouteSettlementTransactionResult{}, false, err
	}
	return result, true, nil
}

func (store *InMemoryStore) restoreAutomationRouteReadModelFromDurableLocked(
	ownerPlayerID foundation.PlayerID,
	routeID foundation.RouteID,
) (AutomationRoute, bool, error) {
	if route, ok := store.routes[routeID]; ok {
		if err := route.Validate(); err != nil {
			return AutomationRoute{}, false, err
		}
		if route.OwnerPlayerID != ownerPlayerID {
			return AutomationRoute{}, false, fmt.Errorf("route %q owner %q: %w", routeID, ownerPlayerID, ErrRouteOwnerMismatch)
		}
		return cloneAutomationRoute(route), true, nil
	}
	var record AutomationRouteDurableRecord
	var ok bool
	var err error
	if store.routeDurable != nil {
		record, ok, err = store.routeDurable.CommittedAutomationRouteDurableRecord(routeID)
		if err != nil {
			return AutomationRoute{}, false, err
		}
	} else {
		record, ok = store.routeDurableRecords[routeID]
	}
	if !ok {
		return AutomationRoute{}, false, nil
	}
	if err := validateAutomationRouteDurableRecordForRoute(record, routeID); err != nil {
		return AutomationRoute{}, false, err
	}
	if record.Route.OwnerPlayerID != ownerPlayerID {
		return AutomationRoute{}, false, fmt.Errorf("route %q owner %q: %w", routeID, ownerPlayerID, ErrRouteOwnerMismatch)
	}
	store.routes[routeID] = cloneAutomationRoute(record.Route)
	return cloneAutomationRoute(record.Route), true, nil
}

func (store *InMemoryStore) restoreAutomationRouteReadModelFromDurable(
	ownerPlayerID foundation.PlayerID,
	routeID foundation.RouteID,
) (AutomationRoute, bool, error) {
	if err := ownerPlayerID.Validate(); err != nil {
		return AutomationRoute{}, false, err
	}
	if err := routeID.Validate(); err != nil {
		return AutomationRoute{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	return store.restoreAutomationRouteReadModelFromDurableLocked(ownerPlayerID, routeID)
}

func validateRouteSettlementTransactionReplayReference(
	input RouteSettlementTransactionInput,
	settledAt time.Time,
	record AutomationRouteDurableRecord,
	reference SettlementReferenceRecord,
) error {
	if err := validateSettlementReferenceRecord(reference); err != nil {
		return fmt.Errorf("settlement_reference: %w: %v", ErrInvalidSettlementDurableCommit, err)
	}
	if reference.Kind != SettlementKindRoute ||
		reference.RouteID != input.RouteID ||
		reference.PlanetID != "" ||
		reference.ReferenceKey != record.ReferenceKey ||
		!reference.AppliedAt.Equal(settledAt) ||
		!reference.RecordedAt.Equal(record.RecordedAt) {
		return fmt.Errorf("settlement_reference: %w", ErrInvalidSettlementDurableCommit)
	}
	wantReference, err := routeSettlementReferenceKey(input.RouteID, reference.SettlementWindow)
	if err != nil {
		return fmt.Errorf("settlement_reference: %w: %v", ErrInvalidSettlementDurableCommit, err)
	}
	if reference.ReferenceKey != wantReference {
		return fmt.Errorf("settlement_reference: %w", ErrInvalidSettlementDurableCommit)
	}
	return nil
}

func validateRouteSettlementTransactionReplayRows(result RouteSettlementTransactionResult) error {
	payload, ok, err := routeSettlementPayloadFromReplayOutbox(result)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("route_settlement_outbox: %w", ErrInvalidSettlementDurableCommit)
	}
	if len(result.StorageLedger) == 0 {
		if routeSettlementPayloadNeedsStorageLedger(payload) {
			return fmt.Errorf("route_storage_ledger: %w", ErrInvalidSettlementDurableCommit)
		}
		return nil
	}
	if len(result.StorageRows) == 0 {
		return fmt.Errorf("storage_rows: %w", ErrInvalidSettlementDurableCommit)
	}
	if err := validateRouteSettlementReplayLedgerMatchesPayload(payload, result.StorageLedger); err != nil {
		return err
	}
	return nil
}

func routeSettlementPayloadFromReplayOutbox(
	result RouteSettlementTransactionResult,
) (RouteSettlementPayload, bool, error) {
	for _, record := range result.OutboxRecords {
		if record.Event.Type != EventRouteTransferSettled {
			continue
		}
		var payload RouteSettlementPayload
		if err := json.Unmarshal(record.Event.Payload, &payload); err != nil {
			return RouteSettlementPayload{}, false, fmt.Errorf("route_settlement_outbox: %w: %v", ErrInvalidSettlementDurableCommit, err)
		}
		if payload.RouteID != result.Settlement.RouteID ||
			payload.ReferenceKey != result.Settlement.ReferenceKey ||
			payload.SettlementWindow != result.Settlement.SettlementWindow {
			return RouteSettlementPayload{}, false, fmt.Errorf("route_settlement_outbox: %w", ErrInvalidSettlementDurableCommit)
		}
		return payload, true, nil
	}
	return RouteSettlementPayload{}, false, nil
}

func routeSettlementPayloadNeedsStorageLedger(payload RouteSettlementPayload) bool {
	return payload.TakenAmount > 0 ||
		payload.LostAmount > 0 ||
		payload.AddedAmount > 0 ||
		payload.DeliveredAmount > payload.AddedAmount
}

func validateRouteSettlementReplayLedgerMatchesPayload(
	payload RouteSettlementPayload,
	rows []RouteStorageLedgerEntry,
) error {
	quantitiesByOperation := make(map[RouteStorageLedgerOperation]int64, len(rows))
	for _, row := range rows {
		if row.ItemID != payload.ResourceItemID {
			return fmt.Errorf("route_storage_ledger.item: %w", ErrInvalidSettlementDurableCommit)
		}
		quantitiesByOperation[row.Operation] += row.Quantity
	}
	expectedByOperation := map[RouteStorageLedgerOperation]int64{
		RouteStorageLedgerSourceDebit:       payload.TakenAmount,
		RouteStorageLedgerTransferLoss:      payload.LostAmount,
		RouteStorageLedgerDestinationCredit: payload.AddedAmount,
	}
	if overflow := payload.DeliveredAmount - payload.AddedAmount; overflow > 0 {
		expectedByOperation[RouteStorageLedgerDestinationOverflow] = overflow
	}
	for operation, got := range quantitiesByOperation {
		if want := expectedByOperation[operation]; got != want {
			return fmt.Errorf("route_storage_ledger.%s: %w", operation, ErrInvalidSettlementDurableCommit)
		}
	}
	for operation, want := range expectedByOperation {
		if want > 0 && quantitiesByOperation[operation] != want {
			return fmt.Errorf("route_storage_ledger.%s: %w", operation, ErrInvalidSettlementDurableCommit)
		}
	}
	return nil
}

func (store *InMemoryStore) routeSettlementOutboxRecordsLocked(
	referenceKey foundation.IdempotencyKey,
	settlementWindow string,
) []ProductionOutboxRecord {
	var records []ProductionOutboxRecord
	for _, record := range store.outbox {
		if record.ReferenceKey == referenceKey && record.SettlementWindow == settlementWindow {
			records = append(records, cloneProductionOutboxRecord(record))
		}
	}
	return records
}

func (store *InMemoryStore) routeSettlementLedgerRowsLocked(
	routeID foundation.RouteID,
	referenceKey foundation.IdempotencyKey,
	settlementWindow string,
) []RouteStorageLedgerEntry {
	var rows []RouteStorageLedgerEntry
	for _, row := range store.routeStorageLedger {
		if row.RouteID == routeID && row.ReferenceKey == referenceKey && row.SettlementWindow == settlementWindow {
			rows = append(rows, row)
		}
	}
	return cloneRouteStorageLedgerEntries(rows)
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

func (store *InMemoryStore) routeSettlementStorageRowsLocked(
	ledgerRows []RouteStorageLedgerEntry,
) []PlanetStorage {
	if len(ledgerRows) == 0 {
		return nil
	}
	rowsByPlanet := make(map[foundation.PlanetID]PlanetStorage, len(ledgerRows))
	for _, ledger := range ledgerRows {
		if _, ok := rowsByPlanet[ledger.PlanetID]; ok {
			continue
		}
		if row, ok := store.storage[ledger.PlanetID]; ok {
			rowsByPlanet[ledger.PlanetID] = clonePlanetStorage(row)
		}
	}
	rows := make([]PlanetStorage, 0, len(rowsByPlanet))
	for _, row := range rowsByPlanet {
		rows = append(rows, row)
	}
	return clonePlanetStorageRows(rows)
}
