package discovery

import (
	"errors"
	"fmt"

	"gameproject/internal/game/foundation"
)

var ErrInvalidClaimDurableCommit = errors.New("invalid claim durable commit")

// ClaimDurableCommitPlan validates the row bundle a future durable claim DB
// transaction must commit for one completed planet claim: owner-CAS boundary,
// idempotency reference, event row, pending outbox row, and optional X Core
// debit evidence.
type ClaimDurableCommitPlan struct {
	Boundary         ClaimBoundaryRecord
	Reference        ClaimReferenceRecord
	Event            ClaimEventRecord
	Outbox           ClaimOutboxRecord
	XCoreConsumption ClaimXCoreConsumptionRecord
}

// DurableCommitPlan returns the validated row bundle this completed claim
// boundary committed. Duplicate completions replay the same committed rows.
func (result CompletePlanetClaimBoundaryResult) DurableCommitPlan() (ClaimDurableCommitPlan, error) {
	return NewClaimDurableCommitPlan(&result.Boundary, &result.Reference, &result.Event, &result.Outbox, nil)
}

// NewClaimDurableCommitPlan validates one completed claim row bundle. Empty
// input is a no-op plan.
func NewClaimDurableCommitPlan(
	boundary *ClaimBoundaryRecord,
	reference *ClaimReferenceRecord,
	event *ClaimEventRecord,
	outbox *ClaimOutboxRecord,
	xcore *ClaimXCoreConsumptionRecord,
) (ClaimDurableCommitPlan, error) {
	if boundary == nil {
		if reference == nil && event == nil && outbox == nil && xcore == nil {
			return ClaimDurableCommitPlan{}, nil
		}
		return ClaimDurableCommitPlan{}, fmt.Errorf("boundary: %w", ErrInvalidClaimDurableCommit)
	}
	if reference == nil || event == nil || outbox == nil {
		return ClaimDurableCommitPlan{}, fmt.Errorf("artifacts: %w", ErrInvalidClaimDurableCommit)
	}

	clonedBoundary := cloneClaimBoundaryRecord(*boundary)
	clonedReference := cloneClaimReferenceRecord(*reference)
	clonedEvent := cloneClaimEventRecord(*event)
	clonedOutbox := cloneClaimOutboxRecord(*outbox)

	if err := validateClaimDurableCommitBoundary(clonedBoundary); err != nil {
		return ClaimDurableCommitPlan{}, err
	}
	if err := validateClaimDurableCommitReference(clonedBoundary, clonedReference); err != nil {
		return ClaimDurableCommitPlan{}, err
	}
	if err := validateClaimDurableCommitEvent(clonedBoundary, clonedEvent); err != nil {
		return ClaimDurableCommitPlan{}, err
	}
	if err := validateClaimDurableCommitOutbox(clonedBoundary, clonedEvent, clonedOutbox); err != nil {
		return ClaimDurableCommitPlan{}, err
	}

	plan := ClaimDurableCommitPlan{
		Boundary:  clonedBoundary,
		Reference: clonedReference,
		Event:     clonedEvent,
		Outbox:    clonedOutbox,
	}
	if xcore != nil {
		clonedXCore := cloneClaimXCoreConsumptionRecord(*xcore)
		if err := validateClaimDurableCommitXCore(clonedBoundary, clonedXCore); err != nil {
			return ClaimDurableCommitPlan{}, err
		}
		plan.XCoreConsumption = clonedXCore
	}
	return plan, nil
}

func validateClaimDurableCommitBoundary(record ClaimBoundaryRecord) error {
	if err := record.ClaimReference.Validate(); err != nil {
		return fmt.Errorf("boundary.claim_reference: %w", err)
	}
	if err := record.ReferenceKey.Validate(); err != nil {
		return fmt.Errorf("boundary.reference_key: %w", err)
	}
	if err := record.PlayerID.Validate(); err != nil {
		return fmt.Errorf("boundary.player_id: %w", err)
	}
	if err := record.PlanetID.Validate(); err != nil {
		return fmt.Errorf("boundary.planet_id: %w", err)
	}
	if record.Status != ClaimBoundaryStatusComplete {
		return fmt.Errorf("boundary.status %q: %w", record.Status, ErrInvalidClaimDurableCommit)
	}
	if err := record.EventID.Validate(); err != nil {
		return fmt.Errorf("boundary.event_id: %w", err)
	}
	if record.ClaimedAt.IsZero() || record.RecordedAt.IsZero() || record.CompletedAt.IsZero() {
		return fmt.Errorf("boundary.timestamps: %w", ErrInvalidClaimDurableCommit)
	}
	if record.StaleIntelCount < 0 || record.StaleListingCount < 0 {
		return fmt.Errorf("boundary.stale_counts: %w", ErrInvalidClaimDurableCommit)
	}
	if err := validateClaimDurableReferenceKey(record.ClaimReference, record.ReferenceKey, record.PlayerID, record.PlanetID); err != nil {
		return fmt.Errorf("boundary.reference_key: %w", err)
	}
	return nil
}

func validateClaimDurableCommitReference(boundary ClaimBoundaryRecord, record ClaimReferenceRecord) error {
	if record.ClaimReference != boundary.ClaimReference ||
		record.ReferenceKey != boundary.ReferenceKey ||
		record.PlayerID != boundary.PlayerID ||
		record.PlanetID != boundary.PlanetID ||
		record.EventID != boundary.EventID {
		return fmt.Errorf("reference: %w", ErrInvalidClaimDurableCommit)
	}
	if record.AlreadyOwned {
		return fmt.Errorf("reference.already_owned: %w", ErrInvalidClaimDurableCommit)
	}
	if !record.ClaimedAt.Equal(boundary.ClaimedAt) || !record.RecordedAt.Equal(boundary.CompletedAt) {
		return fmt.Errorf("reference.timestamps: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func validateClaimDurableCommitEvent(boundary ClaimBoundaryRecord, record ClaimEventRecord) error {
	if record.EventID != boundary.EventID ||
		record.Type != ClaimEventPlanetClaimed ||
		record.PlayerID != boundary.PlayerID ||
		record.PlanetID != boundary.PlanetID ||
		record.ClaimReference != boundary.ClaimReference ||
		!record.CreatedAt.Equal(boundary.ClaimedAt) {
		return fmt.Errorf("event: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func validateClaimDurableCommitOutbox(boundary ClaimBoundaryRecord, event ClaimEventRecord, record ClaimOutboxRecord) error {
	if record.OutboxID == "" || record.Sequence == 0 {
		return fmt.Errorf("outbox.identity: %w", ErrInvalidClaimDurableCommit)
	}
	if record.Status != ClaimOutboxStatusPending {
		return fmt.Errorf("outbox.status %q: %w", record.Status, ErrInvalidClaimDurableCommit)
	}
	if record.ClaimReference != boundary.ClaimReference || record.ReferenceKey != boundary.ReferenceKey {
		return fmt.Errorf("outbox.reference: %w", ErrInvalidClaimDurableCommit)
	}
	if !record.CreatedAt.Equal(event.CreatedAt) {
		return fmt.Errorf("outbox.created_at: %w", ErrInvalidClaimDurableCommit)
	}
	if !claimDurableCommitEventsMatch(record.Event, event) {
		return fmt.Errorf("outbox.event: %w", ErrInvalidClaimDurableCommit)
	}
	if !record.ClaimedAt.IsZero() || record.ClaimToken != "" || !record.PublishedAt.IsZero() || !record.FailedAt.IsZero() {
		return fmt.Errorf("outbox.delivery_state: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func validateClaimDurableCommitXCore(boundary ClaimBoundaryRecord, record ClaimXCoreConsumptionRecord) error {
	if record.ClaimReference != boundary.ClaimReference ||
		record.ReferenceKey != boundary.ReferenceKey ||
		record.PlayerID != boundary.PlayerID ||
		record.PlanetID != boundary.PlanetID {
		return fmt.Errorf("x_core: %w", ErrInvalidClaimDurableCommit)
	}
	if err := record.SourceLocation.Validate(); err != nil {
		return fmt.Errorf("x_core.source_location: %w", err)
	}
	if record.Quantity != defaultClaimXCoreQuantity {
		return fmt.Errorf("x_core.quantity %d: %w", record.Quantity, ErrInvalidClaimDurableCommit)
	}
	if err := record.Reason.Validate(); err != nil {
		return fmt.Errorf("x_core.reason: %w", err)
	}
	if record.ConsumedAt.IsZero() {
		return fmt.Errorf("x_core.consumed_at: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func claimDurableCommitEventsMatch(left ClaimEventRecord, right ClaimEventRecord) bool {
	return left.EventID == right.EventID &&
		left.Type == right.Type &&
		left.PlayerID == right.PlayerID &&
		left.PlanetID == right.PlanetID &&
		left.ClaimReference == right.ClaimReference &&
		left.CreatedAt.Equal(right.CreatedAt)
}

func validateClaimDurableReferenceKey(
	ref PlanetClaimReference,
	key foundation.IdempotencyKey,
	playerID foundation.PlayerID,
	planetID foundation.PlanetID,
) error {
	expected, ok := ref.IdempotencyKey(playerID, planetID)
	if !ok || key != expected {
		return ErrInvalidClaimDurableCommit
	}
	return nil
}
