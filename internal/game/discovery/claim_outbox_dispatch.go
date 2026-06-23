package discovery

import (
	"errors"
	"fmt"
)

var ErrInvalidClaimOutboxDispatch = errors.New("invalid claim outbox dispatch")

// ClaimOutboxDispatchPlan is the after-commit handoff contract from a completed
// planet claim transaction to a durable publisher scheduler.
type ClaimOutboxDispatchPlan struct {
	Reference ClaimReferenceRecord
	Outbox    ClaimOutboxRecord
}

// NewClaimOutboxDispatchPlan validates that the committed claim outbox row is
// pending, carries the same claim reference evidence as the committed claim
// reference, and is safe for a publisher scheduler to claim later.
func NewClaimOutboxDispatchPlan(
	reference *ClaimReferenceRecord,
	outbox *ClaimOutboxRecord,
) (ClaimOutboxDispatchPlan, error) {
	if reference == nil {
		if outbox == nil {
			return ClaimOutboxDispatchPlan{}, nil
		}
		return ClaimOutboxDispatchPlan{}, fmt.Errorf("reference: %w", ErrInvalidClaimOutboxDispatch)
	}
	if outbox == nil {
		return ClaimOutboxDispatchPlan{}, fmt.Errorf("outbox: %w", ErrInvalidClaimOutboxDispatch)
	}
	clonedReference := cloneClaimReferenceRecord(*reference)
	clonedOutbox := cloneClaimOutboxRecord(*outbox)
	if err := validateClaimOutboxDispatchReference(clonedReference); err != nil {
		return ClaimOutboxDispatchPlan{}, err
	}
	if err := validateClaimOutboxDispatchRecord(clonedReference, clonedOutbox); err != nil {
		return ClaimOutboxDispatchPlan{}, err
	}
	return ClaimOutboxDispatchPlan{
		Reference: clonedReference,
		Outbox:    clonedOutbox,
	}, nil
}

func validateClaimOutboxDispatchReference(record ClaimReferenceRecord) error {
	if err := record.ClaimReference.Validate(); err != nil {
		return fmt.Errorf("claim_reference: %w", err)
	}
	if err := record.ReferenceKey.Validate(); err != nil {
		return fmt.Errorf("reference_key: %w", err)
	}
	if err := record.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := record.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if record.AlreadyOwned {
		return fmt.Errorf("already_owned: %w", ErrInvalidClaimOutboxDispatch)
	}
	if err := record.EventID.Validate(); err != nil {
		return fmt.Errorf("event_id: %w", err)
	}
	if record.ClaimedAt.IsZero() || record.RecordedAt.IsZero() || record.RecordedAt.Before(record.ClaimedAt) {
		return fmt.Errorf("timestamps: %w", ErrInvalidClaimOutboxDispatch)
	}
	if err := validateClaimDurableReferenceKey(record.ClaimReference, record.ReferenceKey, record.PlayerID, record.PlanetID); err != nil {
		return fmt.Errorf("reference_key: %w", err)
	}
	return nil
}

func validateClaimOutboxDispatchRecord(reference ClaimReferenceRecord, record ClaimOutboxRecord) error {
	if record.OutboxID == "" || record.Sequence == 0 {
		return fmt.Errorf("identity: %w", ErrInvalidClaimOutboxDispatch)
	}
	if record.Status != ClaimOutboxStatusPending {
		return fmt.Errorf("status %q: %w", record.Status, ErrInvalidClaimOutboxDispatch)
	}
	if record.ClaimReference != reference.ClaimReference || record.ReferenceKey != reference.ReferenceKey {
		return fmt.Errorf("reference: %w", ErrInvalidClaimOutboxDispatch)
	}
	if !record.CreatedAt.Equal(reference.ClaimedAt) {
		return fmt.Errorf("created_at: %w", ErrInvalidClaimOutboxDispatch)
	}
	if !record.ClaimedAt.IsZero() || record.ClaimToken != "" || !record.PublishedAt.IsZero() || !record.FailedAt.IsZero() {
		return fmt.Errorf("delivery_state: %w", ErrInvalidClaimOutboxDispatch)
	}
	event := record.Event
	if event.EventID != reference.EventID ||
		event.Type != ClaimEventPlanetClaimed ||
		event.PlayerID != reference.PlayerID ||
		event.PlanetID != reference.PlanetID ||
		event.ClaimReference != reference.ClaimReference ||
		!event.CreatedAt.Equal(reference.ClaimedAt) {
		return fmt.Errorf("event: %w", ErrInvalidClaimOutboxDispatch)
	}
	return nil
}
