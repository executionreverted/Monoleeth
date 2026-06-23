package production

import (
	"errors"
	"testing"
	"time"

	gameevents "gameproject/internal/game/events"
)

func TestValidateProductionOutboxReadbackStateRejectsPublishedFailureEvidence(t *testing.T) {
	invalid := errors.New("invalid test outbox")
	record := productionOutboxReadbackValidationRecord(ProductionOutboxStatusPublished)
	record.FailedAt = testTime(4)
	record.LastError = "temporary publisher failure"

	err := validateProductionOutboxReadbackState(record, invalid)
	if !errors.Is(err, invalid) {
		t.Fatalf("validateProductionOutboxReadbackState(published with failure evidence) = %v, want invalid", err)
	}
}

func TestValidateProductionOutboxReadbackStateAllowsPendingRetryFailureEvidence(t *testing.T) {
	invalid := errors.New("invalid test outbox")
	record := productionOutboxReadbackValidationRecord(ProductionOutboxStatusPending)
	record.ClaimedAt = testTime(2)
	record.ClaimToken = "claim-token-1"
	record.FailedAt = testTime(4)
	record.LastError = "temporary publisher failure"

	if err := validateProductionOutboxReadbackState(record, invalid); !errors.Is(err, invalid) {
		t.Fatalf("claimed pending row error = %v, want invalid", err)
	}

	record.ClaimedAt = time.Time{}
	record.ClaimToken = ""
	if err := validateProductionOutboxReadbackState(record, invalid); err != nil {
		t.Fatalf("validateProductionOutboxReadbackState(pending retry evidence) error = %v, want nil", err)
	}
}

func productionOutboxReadbackValidationRecord(status ProductionOutboxStatus) ProductionOutboxRecord {
	record := ProductionOutboxRecord{
		OutboxID:     "outbox-1",
		Sequence:     1,
		Status:       status,
		ReferenceKey: "reference-1",
		CreatedAt:    testTime(1),
		Event: gameevents.EventEnvelope{
			Type:    EventPlanetStorageUpdated,
			Payload: []byte(`{"ok":true}`),
		},
	}
	switch status {
	case ProductionOutboxStatusInFlight:
		record.ClaimedAt = testTime(2)
		record.ClaimToken = "claim-token-1"
	case ProductionOutboxStatusPublished:
		record.ClaimedAt = testTime(2)
		record.ClaimToken = "claim-token-1"
		record.PublishedAt = testTime(3)
	case ProductionOutboxStatusFailed:
		record.ClaimedAt = testTime(2)
		record.ClaimToken = "claim-token-1"
		record.FailedAt = testTime(4)
		record.LastError = "temporary publisher failure"
	}
	return record
}
