package production

import (
	"fmt"
	"time"
)

func validateProductionOutboxReadbackState(record ProductionOutboxRecord, invalid error) error {
	if (record.FailedAt.IsZero()) != (record.LastError == "") {
		return fmt.Errorf("failure_evidence: %w", invalid)
	}
	switch record.Status {
	case ProductionOutboxStatusPending:
		if !record.ClaimedAt.IsZero() || record.ClaimToken != "" || !record.PublishedAt.IsZero() {
			return fmt.Errorf("delivery_state: %w", invalid)
		}
	case ProductionOutboxStatusInFlight:
		if record.ClaimedAt.IsZero() || record.ClaimToken == "" || !record.PublishedAt.IsZero() {
			return fmt.Errorf("delivery_state: %w", invalid)
		}
	case ProductionOutboxStatusPublished:
		if record.PublishedAt.IsZero() || record.ClaimedAt.IsZero() || record.ClaimToken == "" {
			return fmt.Errorf("delivery_state: %w", invalid)
		}
	case ProductionOutboxStatusFailed:
		if record.FailedAt.IsZero() || record.LastError == "" || record.ClaimedAt.IsZero() || record.ClaimToken == "" || !record.PublishedAt.IsZero() {
			return fmt.Errorf("delivery_state: %w", invalid)
		}
	default:
		return fmt.Errorf("status %q: %w", record.Status, invalid)
	}
	return nil
}

func validateProductionOutboxReadbackStates(records []ProductionOutboxRecord, invalid error) error {
	for index, record := range records {
		if err := validateProductionOutboxReadbackState(record, invalid); err != nil {
			return fmt.Errorf("outbox[%d]: %w", index, err)
		}
	}
	return nil
}

func pendingProductionOutboxRecordsForCommitValidation(records []ProductionOutboxRecord) []ProductionOutboxRecord {
	pending := cloneProductionOutboxRecords(records)
	for index := range pending {
		pending[index].Status = ProductionOutboxStatusPending
		pending[index].ClaimedAt = time.Time{}
		pending[index].ClaimToken = ""
		pending[index].PublishedAt = time.Time{}
		pending[index].FailedAt = time.Time{}
		pending[index].RetriedAt = time.Time{}
		pending[index].Attempts = 0
		pending[index].LastError = ""
	}
	return pending
}
