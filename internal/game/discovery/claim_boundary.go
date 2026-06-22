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
	ClaimOutboxStatusPending ClaimOutboxStatus = "pending"
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
	return record
}

func cloneClaimEventRecord(record ClaimEventRecord) ClaimEventRecord {
	record.CreatedAt = record.CreatedAt.UTC()
	return record
}
