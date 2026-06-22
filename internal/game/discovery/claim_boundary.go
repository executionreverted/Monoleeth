package discovery

import (
	"fmt"
	"sort"
	"time"

	"gameproject/internal/game/foundation"
)

// ClaimOutboxStatus identifies the local delivery state of an in-memory claim
// outbox record.
type ClaimOutboxStatus string

const (
	ClaimOutboxStatusPending   ClaimOutboxStatus = "pending"
	ClaimOutboxStatusInFlight  ClaimOutboxStatus = "in_flight"
	ClaimOutboxStatusPublished ClaimOutboxStatus = "published"
	ClaimOutboxStatusFailed    ClaimOutboxStatus = "failed"
)

// ClaimReferenceRecord is the process-local durable-boundary marker for one
// successfully cached planet claim result. It is not durable or cross-process.
type ClaimReferenceRecord struct {
	ClaimReference PlanetClaimReference
	PlayerID       foundation.PlayerID
	PlanetID       foundation.PlanetID
	ClaimedAt      time.Time
	RecordedAt     time.Time
	AlreadyOwned   bool
	EventID        foundation.EventID
}

// ClaimOutboxRecord is the process-local publisher boundary for a claim event.
// It is appended only when a successful new owner change appends a claim event.
type ClaimOutboxRecord struct {
	OutboxID       string
	Sequence       uint64
	Event          ClaimEventRecord
	Status         ClaimOutboxStatus
	CreatedAt      time.Time
	ClaimedAt      time.Time
	ClaimToken     string
	PublishedAt    time.Time
	FailedAt       time.Time
	RetriedAt      time.Time
	Attempts       int
	LastError      string
	ClaimReference PlanetClaimReference
}

// ClaimReferences returns process-local claim reference records in deterministic
// claim-reference order.
func (service *ClaimService) ClaimReferences() []ClaimReferenceRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	if len(service.references) == 0 {
		return nil
	}
	refs := make([]PlanetClaimReference, 0, len(service.references))
	for ref := range service.references {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i] < refs[j]
	})
	records := make([]ClaimReferenceRecord, 0, len(refs))
	for _, ref := range refs {
		records = append(records, cloneClaimReferenceRecord(service.references[ref]))
	}
	return records
}

// ClaimReference returns one process-local claim reference record.
func (service *ClaimService) ClaimReference(ref PlanetClaimReference) (ClaimReferenceRecord, bool, error) {
	if err := ref.Validate(); err != nil {
		return ClaimReferenceRecord{}, false, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()

	record, ok := service.references[ref]
	if !ok {
		return ClaimReferenceRecord{}, false, nil
	}
	return cloneClaimReferenceRecord(record), true, nil
}

// ClaimOutboxRecords returns claim outbox records in append order.
func (service *ClaimService) ClaimOutboxRecords() []ClaimOutboxRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	return cloneClaimOutboxRecords(service.outbox)
}

// PendingClaimOutboxRecords returns pending claim outbox records in append
// order.
func (service *ClaimService) PendingClaimOutboxRecords(limit int) []ClaimOutboxRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	return service.claimOutboxRecordsByStatusLocked(ClaimOutboxStatusPending, limit)
}

// ClaimPendingClaimOutboxRecords moves pending claim outbox records to
// in-flight in append order, recording one publisher attempt per record.
func (service *ClaimService) ClaimPendingClaimOutboxRecords(limit int, claimedAt time.Time) []ClaimOutboxRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	if limit <= 0 {
		return nil
	}
	claimedAt = claimedAt.UTC()
	records := make([]ClaimOutboxRecord, 0, limit)
	for index := range service.outbox {
		if len(records) >= limit {
			break
		}
		if service.outbox[index].Status != ClaimOutboxStatusPending {
			continue
		}
		service.outbox[index].Status = ClaimOutboxStatusInFlight
		service.outbox[index].ClaimedAt = claimedAt
		service.outbox[index].Attempts++
		service.outbox[index].ClaimToken = claimOutboxClaimToken(service.outbox[index].OutboxID, service.outbox[index].Attempts)
		records = append(records, cloneClaimOutboxRecord(service.outbox[index]))
	}
	return records
}

// MarkClaimedClaimOutboxPublished records successful delivery for the current
// claim attempt. Missing records, stale tokens, and non-in-flight records do
// not mutate state.
func (service *ClaimService) MarkClaimedClaimOutboxPublished(outboxID string, claimToken string, publishedAt time.Time) (ClaimOutboxRecord, bool) {
	service.mu.Lock()
	defer service.mu.Unlock()

	index, ok := service.claimOutboxIndexLocked(outboxID)
	if !ok {
		return ClaimOutboxRecord{}, false
	}
	if !service.claimOutboxClaimMatchesLocked(index, claimToken) {
		return ClaimOutboxRecord{}, false
	}
	service.outbox[index].Status = ClaimOutboxStatusPublished
	service.outbox[index].PublishedAt = publishedAt.UTC()
	return cloneClaimOutboxRecord(service.outbox[index]), true
}

// MarkClaimedClaimOutboxFailed records failed delivery for the current claim
// attempt. Missing records, stale tokens, and non-in-flight records do not
// mutate state.
func (service *ClaimService) MarkClaimedClaimOutboxFailed(outboxID string, claimToken string, reason string, failedAt time.Time) (ClaimOutboxRecord, bool) {
	service.mu.Lock()
	defer service.mu.Unlock()

	index, ok := service.claimOutboxIndexLocked(outboxID)
	if !ok {
		return ClaimOutboxRecord{}, false
	}
	if !service.claimOutboxClaimMatchesLocked(index, claimToken) {
		return ClaimOutboxRecord{}, false
	}
	service.outbox[index].Status = ClaimOutboxStatusFailed
	service.outbox[index].FailedAt = failedAt.UTC()
	service.outbox[index].LastError = reason
	return cloneClaimOutboxRecord(service.outbox[index]), true
}

// RetryFailedClaimOutboxRecords moves failed claim outbox records back to
// pending in append order while preserving failure evidence for diagnostics.
func (service *ClaimService) RetryFailedClaimOutboxRecords(limit int, retriedAt time.Time) []ClaimOutboxRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	if limit <= 0 {
		return nil
	}
	retriedAt = retriedAt.UTC()
	records := make([]ClaimOutboxRecord, 0, limit)
	for index := range service.outbox {
		if len(records) >= limit {
			break
		}
		if service.outbox[index].Status != ClaimOutboxStatusFailed {
			continue
		}
		service.outbox[index].Status = ClaimOutboxStatusPending
		service.outbox[index].ClaimedAt = time.Time{}
		service.outbox[index].ClaimToken = ""
		service.outbox[index].RetriedAt = retriedAt
		records = append(records, cloneClaimOutboxRecord(service.outbox[index]))
	}
	return records
}

func (service *ClaimService) recordClaimReferenceLocked(input ClaimPlanetInput, claimedAt time.Time, recordedAt time.Time, alreadyOwned bool, eventID foundation.EventID) {
	service.references[input.ClaimReference] = cloneClaimReferenceRecord(ClaimReferenceRecord{
		ClaimReference: input.ClaimReference,
		PlayerID:       input.PlayerID,
		PlanetID:       input.PlanetID,
		ClaimedAt:      claimedAt.UTC(),
		RecordedAt:     recordedAt.UTC(),
		AlreadyOwned:   alreadyOwned,
		EventID:        eventID,
	})
}

func (service *ClaimService) appendClaimOutboxRecordLocked(event ClaimEventRecord) {
	service.nextOutboxSequence++
	sequence := service.nextOutboxSequence
	service.outbox = append(service.outbox, ClaimOutboxRecord{
		OutboxID:       fmt.Sprintf("claim-outbox-%d", sequence),
		Sequence:       sequence,
		Event:          cloneClaimEventRecord(event),
		Status:         ClaimOutboxStatusPending,
		CreatedAt:      event.CreatedAt.UTC(),
		ClaimReference: event.ClaimReference,
	})
}

func (service *ClaimService) claimOutboxRecordsByStatusLocked(status ClaimOutboxStatus, limit int) []ClaimOutboxRecord {
	if limit <= 0 {
		return nil
	}
	records := make([]ClaimOutboxRecord, 0, limit)
	for _, record := range service.outbox {
		if len(records) >= limit {
			break
		}
		if record.Status == status {
			records = append(records, cloneClaimOutboxRecord(record))
		}
	}
	return records
}

func (service *ClaimService) claimOutboxIndexLocked(outboxID string) (int, bool) {
	for index, record := range service.outbox {
		if record.OutboxID == outboxID {
			return index, true
		}
	}
	return 0, false
}

func (service *ClaimService) claimOutboxClaimMatchesLocked(index int, claimToken string) bool {
	record := service.outbox[index]
	return record.Status == ClaimOutboxStatusInFlight && record.ClaimToken != "" && record.ClaimToken == claimToken
}

func claimOutboxClaimToken(outboxID string, attempts int) string {
	return fmt.Sprintf("%s-attempt-%d", outboxID, attempts)
}

func cloneClaimReferenceRecord(record ClaimReferenceRecord) ClaimReferenceRecord {
	record.ClaimedAt = record.ClaimedAt.UTC()
	record.RecordedAt = record.RecordedAt.UTC()
	return record
}

func cloneClaimOutboxRecords(records []ClaimOutboxRecord) []ClaimOutboxRecord {
	if len(records) == 0 {
		return nil
	}
	cloned := make([]ClaimOutboxRecord, len(records))
	for index, record := range records {
		cloned[index] = cloneClaimOutboxRecord(record)
	}
	return cloned
}

func cloneClaimOutboxRecord(record ClaimOutboxRecord) ClaimOutboxRecord {
	record.Event = cloneClaimEventRecord(record.Event)
	record.CreatedAt = record.CreatedAt.UTC()
	record.ClaimedAt = record.ClaimedAt.UTC()
	record.PublishedAt = record.PublishedAt.UTC()
	record.FailedAt = record.FailedAt.UTC()
	record.RetriedAt = record.RetriedAt.UTC()
	return record
}

func cloneClaimEventRecord(record ClaimEventRecord) ClaimEventRecord {
	record.CreatedAt = record.CreatedAt.UTC()
	return record
}
