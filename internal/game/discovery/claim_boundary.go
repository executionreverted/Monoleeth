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

	ClaimRecoveryReasonAlreadyOwnedRepair = "already_owned_repair"
)

// ClaimReferenceRecord is the process-local durable-boundary marker for one
// successfully cached planet claim result. It is not durable or cross-process.
type ClaimReferenceRecord struct {
	ClaimReference PlanetClaimReference
	ReferenceKey   foundation.IdempotencyKey
	PlayerID       foundation.PlayerID
	PlanetID       foundation.PlanetID
	ClaimedAt      time.Time
	RecordedAt     time.Time
	AlreadyOwned   bool
	EventID        foundation.EventID
}

// ClaimRecoveryRecord marks a successful repair of a claim whose ownership was
// already authoritative before this claim attempt completed all side effects.
type ClaimRecoveryRecord struct {
	ClaimReference    PlanetClaimReference
	ReferenceKey      foundation.IdempotencyKey
	PlayerID          foundation.PlayerID
	PlanetID          foundation.PlanetID
	RecoveredAt       time.Time
	OriginalClaimedAt time.Time
	Reason            string
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
	ReferenceKey   foundation.IdempotencyKey
}

// ClaimReferences returns store-owned claim reference records in deterministic
// claim-reference order.
func (store *InMemoryStore) ClaimReferences() []ClaimReferenceRecord {
	store.mu.RLock()
	defer store.mu.RUnlock()

	if len(store.claimReferences) == 0 {
		return nil
	}
	refs := make([]PlanetClaimReference, 0, len(store.claimReferences))
	for ref := range store.claimReferences {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i] < refs[j]
	})
	records := make([]ClaimReferenceRecord, 0, len(refs))
	for _, ref := range refs {
		records = append(records, cloneClaimReferenceRecord(store.claimReferences[ref]))
	}
	return records
}

// ClaimReference returns one store-owned claim reference record.
func (store *InMemoryStore) ClaimReference(ref PlanetClaimReference) (ClaimReferenceRecord, bool, error) {
	if err := ref.Validate(); err != nil {
		return ClaimReferenceRecord{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	record, ok := store.claimReferences[ref]
	if !ok {
		return ClaimReferenceRecord{}, false, nil
	}
	return cloneClaimReferenceRecord(record), true, nil
}

// ClaimEvents returns store-owned claim events in append order.
func (store *InMemoryStore) ClaimEvents() []ClaimEventRecord {
	store.mu.RLock()
	defer store.mu.RUnlock()

	events := make([]ClaimEventRecord, len(store.claimEvents))
	for index, event := range store.claimEvents {
		events[index] = cloneClaimEventRecord(event)
	}
	return events
}

// ClaimOutboxRecords returns store-owned claim outbox records in append order.
func (store *InMemoryStore) ClaimOutboxRecords() []ClaimOutboxRecord {
	store.mu.RLock()
	defer store.mu.RUnlock()

	return cloneClaimOutboxRecords(store.claimOutbox)
}

// PendingClaimOutboxRecords returns pending store-owned claim outbox records in
// append order.
func (store *InMemoryStore) PendingClaimOutboxRecords(limit int) []ClaimOutboxRecord {
	store.mu.RLock()
	defer store.mu.RUnlock()

	return store.claimOutboxRecordsByStatusLocked(ClaimOutboxStatusPending, limit)
}

// ClaimPendingClaimOutboxRecords moves pending store-owned claim outbox records
// to in-flight in append order, recording one publisher attempt per record.
func (store *InMemoryStore) ClaimPendingClaimOutboxRecords(limit int, claimedAt time.Time) []ClaimOutboxRecord {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if limit <= 0 {
		return nil
	}
	claimedAt = claimedAt.UTC()
	if claimedAt.IsZero() {
		claimedAt = time.Unix(0, 0).UTC()
	}
	records := make([]ClaimOutboxRecord, 0, limit)
	for index := range store.claimOutbox {
		if len(records) >= limit {
			break
		}
		if store.claimOutbox[index].Status != ClaimOutboxStatusPending {
			continue
		}
		store.claimOutbox[index].Status = ClaimOutboxStatusInFlight
		store.claimOutbox[index].ClaimedAt = claimedAt
		store.claimOutbox[index].Attempts++
		store.claimOutbox[index].ClaimToken = claimOutboxClaimToken(store.claimOutbox[index].OutboxID, store.claimOutbox[index].Attempts)
		records = append(records, cloneClaimOutboxRecord(store.claimOutbox[index]))
	}
	return records
}

// MarkClaimedClaimOutboxPublished records successful delivery for the current
// store-owned claim attempt.
func (store *InMemoryStore) MarkClaimedClaimOutboxPublished(outboxID string, claimToken string, publishedAt time.Time) (ClaimOutboxRecord, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	index, ok := store.claimOutboxIndexLocked(outboxID)
	if !ok {
		return ClaimOutboxRecord{}, false
	}
	if !store.claimOutboxClaimMatchesLocked(index, claimToken) {
		return ClaimOutboxRecord{}, false
	}
	store.claimOutbox[index].Status = ClaimOutboxStatusPublished
	store.claimOutbox[index].PublishedAt = publishedAt.UTC()
	return cloneClaimOutboxRecord(store.claimOutbox[index]), true
}

// MarkClaimedClaimOutboxFailed records failed delivery for the current
// store-owned claim attempt.
func (store *InMemoryStore) MarkClaimedClaimOutboxFailed(outboxID string, claimToken string, reason string, failedAt time.Time) (ClaimOutboxRecord, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	index, ok := store.claimOutboxIndexLocked(outboxID)
	if !ok {
		return ClaimOutboxRecord{}, false
	}
	if !store.claimOutboxClaimMatchesLocked(index, claimToken) {
		return ClaimOutboxRecord{}, false
	}
	store.claimOutbox[index].Status = ClaimOutboxStatusFailed
	store.claimOutbox[index].FailedAt = failedAt.UTC()
	store.claimOutbox[index].LastError = reason
	return cloneClaimOutboxRecord(store.claimOutbox[index]), true
}

// RetryFailedClaimOutboxRecords moves failed store-owned claim outbox records
// back to pending in append order while preserving failure evidence.
func (store *InMemoryStore) RetryFailedClaimOutboxRecords(limit int, retriedAt time.Time) []ClaimOutboxRecord {
	store.mu.Lock()
	defer store.mu.Unlock()

	if limit <= 0 {
		return nil
	}
	retriedAt = retriedAt.UTC()
	records := make([]ClaimOutboxRecord, 0, limit)
	for index := range store.claimOutbox {
		if len(records) >= limit {
			break
		}
		if store.claimOutbox[index].Status != ClaimOutboxStatusFailed {
			continue
		}
		store.claimOutbox[index].Status = ClaimOutboxStatusPending
		store.claimOutbox[index].ClaimedAt = time.Time{}
		store.claimOutbox[index].ClaimToken = ""
		store.claimOutbox[index].RetriedAt = retriedAt
		records = append(records, cloneClaimOutboxRecord(store.claimOutbox[index]))
	}
	return records
}

// ReleaseExpiredClaimOutboxRecords moves stale in-flight store-owned claim
// outbox records back to pending in append order.
func (store *InMemoryStore) ReleaseExpiredClaimOutboxRecords(limit int, claimedBefore time.Time, retriedAt time.Time) []ClaimOutboxRecord {
	store.mu.Lock()
	defer store.mu.Unlock()

	if limit <= 0 || claimedBefore.IsZero() {
		return nil
	}
	claimedBefore = claimedBefore.UTC()
	retriedAt = retriedAt.UTC()
	records := make([]ClaimOutboxRecord, 0, limit)
	for index := range store.claimOutbox {
		if len(records) >= limit {
			break
		}
		record := store.claimOutbox[index]
		if record.Status != ClaimOutboxStatusInFlight || record.ClaimedAt.IsZero() || !record.ClaimedAt.Before(claimedBefore) {
			continue
		}
		store.claimOutbox[index].Status = ClaimOutboxStatusPending
		store.claimOutbox[index].ClaimedAt = time.Time{}
		store.claimOutbox[index].ClaimToken = ""
		store.claimOutbox[index].RetriedAt = retriedAt
		records = append(records, cloneClaimOutboxRecord(store.claimOutbox[index]))
	}
	return records
}

// ClaimReferences returns process-local claim reference records in deterministic
// claim-reference order.
func (service *ClaimService) ClaimReferences() []ClaimReferenceRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	storeRecords := service.store.ClaimReferences()
	refs := make([]PlanetClaimReference, 0, len(service.references))
	storeRefs := make(map[PlanetClaimReference]struct{}, len(storeRecords))
	for _, record := range storeRecords {
		storeRefs[record.ClaimReference] = struct{}{}
	}
	for ref := range service.references {
		if _, ok := storeRefs[ref]; ok {
			continue
		}
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i] < refs[j]
	})
	records := make([]ClaimReferenceRecord, 0, len(storeRecords)+len(refs))
	records = append(records, storeRecords...)
	for _, ref := range refs {
		records = append(records, cloneClaimReferenceRecord(service.references[ref]))
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].ClaimReference < records[j].ClaimReference
	})
	return records
}

// ClaimReference returns one process-local claim reference record.
func (service *ClaimService) ClaimReference(ref PlanetClaimReference) (ClaimReferenceRecord, bool, error) {
	if err := ref.Validate(); err != nil {
		return ClaimReferenceRecord{}, false, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()

	if record, ok, err := service.store.ClaimReference(ref); err != nil || ok {
		return record, ok, err
	}
	record, ok := service.references[ref]
	if !ok {
		return ClaimReferenceRecord{}, false, nil
	}
	return cloneClaimReferenceRecord(record), true, nil
}

// ClaimRecoveries returns successful same-owner claim repairs in append order.
func (service *ClaimService) ClaimRecoveries() []ClaimRecoveryRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	return cloneClaimRecoveryRecords(service.recoveries)
}

// ClaimOutboxRecords returns claim outbox records in append order.
func (service *ClaimService) ClaimOutboxRecords() []ClaimOutboxRecord {
	return service.store.ClaimOutboxRecords()
}

// PendingClaimOutboxRecords returns pending claim outbox records in append
// order.
func (service *ClaimService) PendingClaimOutboxRecords(limit int) []ClaimOutboxRecord {
	return service.store.PendingClaimOutboxRecords(limit)
}

// ClaimPendingClaimOutboxRecords moves pending claim outbox records to
// in-flight in append order, recording one publisher attempt per record.
func (service *ClaimService) ClaimPendingClaimOutboxRecords(limit int, claimedAt time.Time) []ClaimOutboxRecord {
	return service.store.ClaimPendingClaimOutboxRecords(limit, claimedAt)
}

// MarkClaimedClaimOutboxPublished records successful delivery for the current
// claim attempt. Missing records, stale tokens, and non-in-flight records do
// not mutate state.
func (service *ClaimService) MarkClaimedClaimOutboxPublished(outboxID string, claimToken string, publishedAt time.Time) (ClaimOutboxRecord, bool) {
	return service.store.MarkClaimedClaimOutboxPublished(outboxID, claimToken, publishedAt)
}

// MarkClaimedClaimOutboxFailed records failed delivery for the current claim
// attempt. Missing records, stale tokens, and non-in-flight records do not
// mutate state.
func (service *ClaimService) MarkClaimedClaimOutboxFailed(outboxID string, claimToken string, reason string, failedAt time.Time) (ClaimOutboxRecord, bool) {
	return service.store.MarkClaimedClaimOutboxFailed(outboxID, claimToken, reason, failedAt)
}

// RetryFailedClaimOutboxRecords moves failed claim outbox records back to
// pending in append order while preserving failure evidence for diagnostics.
func (service *ClaimService) RetryFailedClaimOutboxRecords(limit int, retriedAt time.Time) []ClaimOutboxRecord {
	return service.store.RetryFailedClaimOutboxRecords(limit, retriedAt)
}

// ReleaseExpiredClaimOutboxRecords moves stale in-flight claim outbox records
// back to pending in append order, preserving attempts and failure evidence.
func (service *ClaimService) ReleaseExpiredClaimOutboxRecords(limit int, claimedBefore time.Time, retriedAt time.Time) []ClaimOutboxRecord {
	return service.store.ReleaseExpiredClaimOutboxRecords(limit, claimedBefore, retriedAt)
}

func (service *ClaimService) recordClaimReferenceLocked(input ClaimPlanetInput, claimedAt time.Time, recordedAt time.Time, alreadyOwned bool, eventID foundation.EventID) {
	referenceKey, _ := input.ClaimReference.IdempotencyKey(input.PlayerID, input.PlanetID)
	service.references[input.ClaimReference] = cloneClaimReferenceRecord(ClaimReferenceRecord{
		ClaimReference: input.ClaimReference,
		ReferenceKey:   referenceKey,
		PlayerID:       input.PlayerID,
		PlanetID:       input.PlanetID,
		ClaimedAt:      claimedAt.UTC(),
		RecordedAt:     recordedAt.UTC(),
		AlreadyOwned:   alreadyOwned,
		EventID:        eventID,
	})
}

func (service *ClaimService) appendClaimRecoveryLocked(input ClaimPlanetInput, claimedAt time.Time, recoveredAt time.Time, reason string) {
	referenceKey, _ := input.ClaimReference.IdempotencyKey(input.PlayerID, input.PlanetID)
	service.recoveries = append(service.recoveries, ClaimRecoveryRecord{
		ClaimReference:    input.ClaimReference,
		ReferenceKey:      referenceKey,
		PlayerID:          input.PlayerID,
		PlanetID:          input.PlanetID,
		RecoveredAt:       recoveredAt.UTC(),
		OriginalClaimedAt: claimedAt.UTC(),
		Reason:            reason,
	})
}

func (service *ClaimService) appendClaimOutboxRecordLocked(event ClaimEventRecord) {
	service.nextOutboxSequence++
	sequence := service.nextOutboxSequence
	referenceKey, _ := event.ClaimReference.IdempotencyKey(event.PlayerID, event.PlanetID)
	service.outbox = append(service.outbox, ClaimOutboxRecord{
		OutboxID:       fmt.Sprintf("claim-outbox-%d", sequence),
		Sequence:       sequence,
		Event:          cloneClaimEventRecord(event),
		Status:         ClaimOutboxStatusPending,
		CreatedAt:      event.CreatedAt.UTC(),
		ClaimReference: event.ClaimReference,
		ReferenceKey:   referenceKey,
	})
}

func (store *InMemoryStore) appendClaimOutboxRecordLocked(event ClaimEventRecord) ClaimOutboxRecord {
	store.nextClaimOutboxSequence++
	sequence := store.nextClaimOutboxSequence
	referenceKey, _ := event.ClaimReference.IdempotencyKey(event.PlayerID, event.PlanetID)
	record := ClaimOutboxRecord{
		OutboxID:       fmt.Sprintf("claim-outbox-%d", sequence),
		Sequence:       sequence,
		Event:          cloneClaimEventRecord(event),
		Status:         ClaimOutboxStatusPending,
		CreatedAt:      event.CreatedAt.UTC(),
		ClaimReference: event.ClaimReference,
		ReferenceKey:   referenceKey,
	}
	store.claimOutbox = append(store.claimOutbox, cloneClaimOutboxRecord(record))
	return cloneClaimOutboxRecord(record)
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

func (store *InMemoryStore) claimOutboxRecordsByStatusLocked(status ClaimOutboxStatus, limit int) []ClaimOutboxRecord {
	if limit <= 0 {
		return nil
	}
	records := make([]ClaimOutboxRecord, 0, limit)
	for _, record := range store.claimOutbox {
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

func (store *InMemoryStore) claimOutboxIndexLocked(outboxID string) (int, bool) {
	for index, record := range store.claimOutbox {
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

func (store *InMemoryStore) claimOutboxClaimMatchesLocked(index int, claimToken string) bool {
	record := store.claimOutbox[index]
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

func cloneClaimRecoveryRecords(records []ClaimRecoveryRecord) []ClaimRecoveryRecord {
	if len(records) == 0 {
		return nil
	}
	cloned := make([]ClaimRecoveryRecord, len(records))
	for index, record := range records {
		cloned[index] = cloneClaimRecoveryRecord(record)
	}
	return cloned
}

func cloneClaimRecoveryRecord(record ClaimRecoveryRecord) ClaimRecoveryRecord {
	record.RecoveredAt = record.RecoveredAt.UTC()
	record.OriginalClaimedAt = record.OriginalClaimedAt.UTC()
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
