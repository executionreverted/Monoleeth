package discovery

import (
	"fmt"
	"time"
)

func validateClaimOutboxReadbackState(record ClaimOutboxRecord) error {
	if (record.FailedAt.IsZero()) != (record.LastError == "") {
		return fmt.Errorf("failure_evidence: %w", ErrInvalidClaimDurableCommit)
	}
	switch record.Status {
	case ClaimOutboxStatusPending:
		if !record.ClaimedAt.IsZero() || record.ClaimToken != "" || !record.PublishedAt.IsZero() {
			return fmt.Errorf("delivery_state: %w", ErrInvalidClaimDurableCommit)
		}
	case ClaimOutboxStatusInFlight:
		if record.ClaimedAt.IsZero() || record.ClaimToken == "" || !record.PublishedAt.IsZero() {
			return fmt.Errorf("delivery_state: %w", ErrInvalidClaimDurableCommit)
		}
	case ClaimOutboxStatusPublished:
		if record.PublishedAt.IsZero() || record.ClaimedAt.IsZero() || record.ClaimToken == "" {
			return fmt.Errorf("delivery_state: %w", ErrInvalidClaimDurableCommit)
		}
	case ClaimOutboxStatusFailed:
		if record.FailedAt.IsZero() || record.LastError == "" || record.ClaimedAt.IsZero() || record.ClaimToken == "" || !record.PublishedAt.IsZero() {
			return fmt.Errorf("delivery_state: %w", ErrInvalidClaimDurableCommit)
		}
	default:
		return fmt.Errorf("status %q: %w", record.Status, ErrInvalidClaimDurableCommit)
	}
	return nil
}

func pendingClaimOutboxRecordForCommitValidation(record ClaimOutboxRecord) ClaimOutboxRecord {
	pending := cloneClaimOutboxRecord(record)
	pending.Status = ClaimOutboxStatusPending
	pending.ClaimedAt = time.Time{}
	pending.ClaimToken = ""
	pending.PublishedAt = time.Time{}
	pending.FailedAt = time.Time{}
	pending.RetriedAt = time.Time{}
	pending.Attempts = 0
	pending.LastError = ""
	return pending
}
