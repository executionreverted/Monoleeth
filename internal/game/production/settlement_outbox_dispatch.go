package production

import (
	"errors"
	"fmt"
)

var ErrInvalidSettlementOutboxDispatch = errors.New("invalid settlement outbox dispatch")

// SettlementOutboxDispatchPlan is the after-commit handoff contract from a
// production/route settlement transaction to a durable publisher scheduler.
// Durable adapters should build this from rows committed in the same
// transaction as the settlement reference.
type SettlementOutboxDispatchPlan struct {
	Reference     SettlementReferenceRecord
	OutboxRecords []ProductionOutboxRecord
}

// NewSettlementOutboxDispatchPlan validates that committed settlement outbox
// rows are pending, carry the same idempotency/window evidence as the committed
// settlement reference, and are safe to hand to a publisher scheduler.
func NewSettlementOutboxDispatchPlan(
	reference *SettlementReferenceRecord,
	records []ProductionOutboxRecord,
) (SettlementOutboxDispatchPlan, error) {
	if reference == nil {
		if len(records) == 0 {
			return SettlementOutboxDispatchPlan{}, nil
		}
		return SettlementOutboxDispatchPlan{}, fmt.Errorf("reference: %w", ErrInvalidSettlementOutboxDispatch)
	}
	if err := validateSettlementReferenceRecord(*reference); err != nil {
		return SettlementOutboxDispatchPlan{}, err
	}
	clonedReference := cloneSettlementReferenceRecord(*reference)
	clonedRecords := cloneProductionOutboxRecords(records)
	hasSettlementEvidence := false
	for index, record := range clonedRecords {
		matched, err := validateSettlementOutboxDispatchRecord(clonedReference, record)
		if err != nil {
			return SettlementOutboxDispatchPlan{}, fmt.Errorf("outbox[%d]: %w", index, err)
		}
		hasSettlementEvidence = hasSettlementEvidence || matched
	}
	if len(clonedRecords) > 0 && !hasSettlementEvidence {
		return SettlementOutboxDispatchPlan{}, fmt.Errorf("outbox: %w", ErrInvalidSettlementOutboxDispatch)
	}
	return SettlementOutboxDispatchPlan{
		Reference:     clonedReference,
		OutboxRecords: clonedRecords,
	}, nil
}

func validateSettlementReferenceRecord(record SettlementReferenceRecord) error {
	if err := record.ReferenceKey.Validate(); err != nil {
		return fmt.Errorf("reference_key: %w", err)
	}
	if record.SettlementWindow == "" {
		return fmt.Errorf("settlement_window: %w", ErrInvalidSettlementOutboxDispatch)
	}
	switch record.Kind {
	case SettlementKindProduction:
		if err := record.PlanetID.Validate(); err != nil {
			return fmt.Errorf("planet_id: %w", err)
		}
		if !record.RouteID.IsZero() {
			return fmt.Errorf("route_id: %w", ErrInvalidSettlementOutboxDispatch)
		}
	case SettlementKindRoute:
		if err := record.RouteID.Validate(); err != nil {
			return fmt.Errorf("route_id: %w", err)
		}
		if !record.PlanetID.IsZero() {
			return fmt.Errorf("planet_id: %w", ErrInvalidSettlementOutboxDispatch)
		}
	default:
		return fmt.Errorf("kind %q: %w", record.Kind, ErrInvalidSettlementOutboxDispatch)
	}
	if record.AppliedAt.IsZero() || record.RecordedAt.IsZero() {
		return fmt.Errorf("recorded_at: %w", ErrInvalidSettlementOutboxDispatch)
	}
	return nil
}

func validateSettlementOutboxDispatchRecord(reference SettlementReferenceRecord, record ProductionOutboxRecord) (bool, error) {
	if record.OutboxID == "" || record.Sequence == 0 {
		return false, ErrInvalidSettlementOutboxDispatch
	}
	if record.Status != ProductionOutboxStatusPending {
		return false, fmt.Errorf("status %q: %w", record.Status, ErrInvalidSettlementOutboxDispatch)
	}
	hasEvidence := !record.ReferenceKey.IsZero() || record.SettlementWindow != ""
	if hasEvidence && (record.ReferenceKey != reference.ReferenceKey || record.SettlementWindow != reference.SettlementWindow) {
		return false, fmt.Errorf("evidence %q/%q: %w", record.ReferenceKey, record.SettlementWindow, ErrInvalidSettlementOutboxDispatch)
	}
	if record.CreatedAt.IsZero() {
		return false, fmt.Errorf("created_at: %w", ErrInvalidSettlementOutboxDispatch)
	}
	if record.Event.Type == "" || len(record.Event.Payload) == 0 {
		return false, fmt.Errorf("event: %w", ErrInvalidSettlementOutboxDispatch)
	}
	return hasEvidence, nil
}
