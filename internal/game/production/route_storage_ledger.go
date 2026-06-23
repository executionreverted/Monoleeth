package production

import (
	"fmt"
	"strings"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

// RouteStorageLedgerOperation identifies how route settlement changed or
// intentionally discarded storage units.
type RouteStorageLedgerOperation string

const (
	RouteStorageLedgerSourceDebit         RouteStorageLedgerOperation = "source_debit"
	RouteStorageLedgerTransferLoss        RouteStorageLedgerOperation = "transfer_loss"
	RouteStorageLedgerDestinationCredit   RouteStorageLedgerOperation = "destination_credit"
	RouteStorageLedgerDestinationOverflow RouteStorageLedgerOperation = "destination_overflow"
)

// RouteStorageLedgerEntry is the production-local audit row for route storage
// movement. It mirrors the future durable storage ledger boundary.
type RouteStorageLedgerEntry struct {
	LedgerID             string                      `json:"ledger_id"`
	Operation            RouteStorageLedgerOperation `json:"operation"`
	RouteID              foundation.RouteID          `json:"route_id"`
	PlanetID             foundation.PlanetID         `json:"planet_id"`
	CounterpartyPlanetID foundation.PlanetID         `json:"counterparty_planet_id"`
	ItemID               foundation.ItemID           `json:"item_id"`
	Quantity             int64                       `json:"quantity"`
	BalanceAfter         int64                       `json:"balance_after"`
	ReferenceKey         foundation.IdempotencyKey   `json:"reference_key"`
	SettlementWindow     string                      `json:"settlement_window"`
	CreatedAt            time.Time                   `json:"created_at"`
}

type routeStorageLedgerDraft struct {
	Operation            RouteStorageLedgerOperation
	PlanetID             foundation.PlanetID
	CounterpartyPlanetID foundation.PlanetID
	ItemID               foundation.ItemID
	Quantity             int64
	BalanceAfter         int64
}

// RouteStorageLedgerEntries returns route storage ledger rows in append order.
func (store *InMemoryStore) RouteStorageLedgerEntries() []RouteStorageLedgerEntry {
	store.mu.RLock()
	defer store.mu.RUnlock()

	return cloneRouteStorageLedgerEntries(store.routeStorageLedger)
}

func routeStorageLedgerDraftsForSettlement(
	sourcePlanetID foundation.PlanetID,
	destinationPlanetID foundation.PlanetID,
	itemID foundation.ItemID,
	sourceBalanceAfterDebit int64,
	destinationBalanceAfterCredit int64,
	result RouteSettlementResult,
) []routeStorageLedgerDraft {
	drafts := make([]routeStorageLedgerDraft, 0, 4)
	if result.TakenAmount > 0 {
		drafts = append(drafts, routeStorageLedgerDraft{
			Operation:            RouteStorageLedgerSourceDebit,
			PlanetID:             sourcePlanetID,
			CounterpartyPlanetID: destinationPlanetID,
			ItemID:               itemID,
			Quantity:             result.TakenAmount,
			BalanceAfter:         sourceBalanceAfterDebit,
		})
	}
	if result.LostAmount > 0 {
		drafts = append(drafts, routeStorageLedgerDraft{
			Operation:            RouteStorageLedgerTransferLoss,
			PlanetID:             sourcePlanetID,
			CounterpartyPlanetID: destinationPlanetID,
			ItemID:               itemID,
			Quantity:             result.LostAmount,
			BalanceAfter:         sourceBalanceAfterDebit,
		})
	}
	if result.AddedAmount > 0 {
		drafts = append(drafts, routeStorageLedgerDraft{
			Operation:            RouteStorageLedgerDestinationCredit,
			PlanetID:             destinationPlanetID,
			CounterpartyPlanetID: sourcePlanetID,
			ItemID:               itemID,
			Quantity:             result.AddedAmount,
			BalanceAfter:         destinationBalanceAfterCredit,
		})
	}
	if overflow := result.DeliveredAmount - result.AddedAmount; overflow > 0 {
		drafts = append(drafts, routeStorageLedgerDraft{
			Operation:            RouteStorageLedgerDestinationOverflow,
			PlanetID:             destinationPlanetID,
			CounterpartyPlanetID: sourcePlanetID,
			ItemID:               itemID,
			Quantity:             overflow,
			BalanceAfter:         destinationBalanceAfterCredit,
		})
	}
	return drafts
}

func (store *InMemoryStore) previewRouteStorageLedgerLocked(
	result RouteSettlementResult,
	drafts []routeStorageLedgerDraft,
) ([]RouteStorageLedgerEntry, uint64, error) {
	if len(drafts) == 0 {
		return nil, store.nextRouteLedgerSequence, nil
	}
	nextSequence := store.nextRouteLedgerSequence
	entries := make([]RouteStorageLedgerEntry, 0, len(drafts))
	for _, draft := range drafts {
		if draft.Quantity <= 0 {
			continue
		}
		nextSequence++
		entry := RouteStorageLedgerEntry{
			LedgerID:             fmt.Sprintf("route-storage-ledger-%d", nextSequence),
			Operation:            draft.Operation,
			RouteID:              result.RouteID,
			PlanetID:             draft.PlanetID,
			CounterpartyPlanetID: draft.CounterpartyPlanetID,
			ItemID:               draft.ItemID,
			Quantity:             draft.Quantity,
			BalanceAfter:         draft.BalanceAfter,
			ReferenceKey:         result.ReferenceKey,
			SettlementWindow:     result.SettlementWindow,
			CreatedAt:            result.SettledAt.UTC(),
		}
		if err := entry.Validate(); err != nil {
			return nil, 0, err
		}
		entries = append(entries, entry)
	}
	return entries, nextSequence, nil
}

// Validate reports whether a route storage ledger row is complete and bounded.
func (entry RouteStorageLedgerEntry) Validate() error {
	if strings.TrimSpace(entry.LedgerID) == "" {
		return fmt.Errorf("ledger_id: %w", ErrInvalidRouteStorageLedger)
	}
	if err := entry.Operation.Validate(); err != nil {
		return err
	}
	if err := entry.RouteID.Validate(); err != nil {
		return err
	}
	if err := entry.PlanetID.Validate(); err != nil {
		return err
	}
	if err := entry.CounterpartyPlanetID.Validate(); err != nil {
		return err
	}
	if err := entry.ItemID.Validate(); err != nil {
		return err
	}
	if err := validatePositiveBoundedAmount("route storage ledger quantity", entry.Quantity, ErrInvalidRouteStorageLedger); err != nil {
		return err
	}
	if entry.BalanceAfter < 0 {
		return fmt.Errorf("balance after %d: %w", entry.BalanceAfter, economy.ErrNegativeBalance)
	}
	if err := entry.ReferenceKey.Validate(); err != nil {
		return err
	}
	if err := validateSettlementWindow(entry.SettlementWindow); err != nil {
		return err
	}
	if entry.CreatedAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrZeroProductionTimestamp)
	}
	return nil
}

// Validate reports whether operation is a supported route storage ledger type.
func (operation RouteStorageLedgerOperation) Validate() error {
	switch operation {
	case RouteStorageLedgerSourceDebit,
		RouteStorageLedgerTransferLoss,
		RouteStorageLedgerDestinationCredit,
		RouteStorageLedgerDestinationOverflow:
		return nil
	default:
		return fmt.Errorf("operation %q: %w", operation, ErrInvalidRouteStorageLedger)
	}
}

func cloneRouteStorageLedgerEntries(entries []RouteStorageLedgerEntry) []RouteStorageLedgerEntry {
	if len(entries) == 0 {
		return nil
	}
	cloned := append([]RouteStorageLedgerEntry(nil), entries...)
	for index := range cloned {
		cloned[index].CreatedAt = cloned[index].CreatedAt.UTC()
	}
	return cloned
}
