package production

import (
	"fmt"
	"sort"
	"time"

	gameevents "gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

// SettlementKind identifies the production subsystem that owns a settlement
// reference.
type SettlementKind string

const (
	SettlementKindProduction SettlementKind = "production"
	SettlementKindRoute      SettlementKind = "route"
)

// ProductionOutboxStatus identifies the delivery state of an in-memory outbox
// record.
type ProductionOutboxStatus string

const (
	ProductionOutboxStatusPending   ProductionOutboxStatus = "pending"
	ProductionOutboxStatusInFlight  ProductionOutboxStatus = "in_flight"
	ProductionOutboxStatusPublished ProductionOutboxStatus = "published"
	ProductionOutboxStatusFailed    ProductionOutboxStatus = "failed"
)

// SettlementReferenceRecord is the in-memory durable-boundary marker for one
// applied production-domain settlement window.
type SettlementReferenceRecord struct {
	ReferenceKey     foundation.IdempotencyKey
	SettlementWindow string
	Kind             SettlementKind
	PlanetID         foundation.PlanetID
	RouteID          foundation.RouteID
	AppliedAt        time.Time
	RecordedAt       time.Time
}

// ProductionOutboxRecord is the in-memory publisher boundary for a
// production-domain event envelope.
type ProductionOutboxRecord struct {
	OutboxID         string
	Sequence         uint64
	Event            gameevents.EventEnvelope
	Status           ProductionOutboxStatus
	CreatedAt        time.Time
	ClaimedAt        time.Time
	ClaimToken       string
	PublishedAt      time.Time
	FailedAt         time.Time
	RetriedAt        time.Time
	Attempts         int
	LastError        string
	ReferenceKey     foundation.IdempotencyKey
	SettlementWindow string
}

// SettlementReferences returns all recorded settlement references in
// deterministic reference-key order.
func (store *InMemoryStore) SettlementReferences() []SettlementReferenceRecord {
	store.mu.RLock()
	defer store.mu.RUnlock()

	if len(store.references) == 0 {
		return nil
	}
	keys := make([]foundation.IdempotencyKey, 0, len(store.references))
	for referenceKey := range store.references {
		keys = append(keys, referenceKey)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	records := make([]SettlementReferenceRecord, 0, len(keys))
	for _, referenceKey := range keys {
		records = append(records, cloneSettlementReferenceRecord(store.references[referenceKey]))
	}
	return records
}

// SettlementReference returns one recorded settlement reference by key.
func (store *InMemoryStore) SettlementReference(referenceKey foundation.IdempotencyKey) (SettlementReferenceRecord, bool, error) {
	if err := referenceKey.Validate(); err != nil {
		return SettlementReferenceRecord{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	record, ok := store.references[referenceKey]
	if !ok {
		return SettlementReferenceRecord{}, false, nil
	}
	return cloneSettlementReferenceRecord(record), true, nil
}

// OutboxRecords returns all production-domain outbox records in append order
// for diagnostics.
func (store *InMemoryStore) OutboxRecords() []ProductionOutboxRecord {
	store.mu.RLock()
	defer store.mu.RUnlock()

	return cloneProductionOutboxRecords(store.outbox)
}

// PendingOutboxRecords returns pending outbox records in append order.
func (store *InMemoryStore) PendingOutboxRecords(limit int) []ProductionOutboxRecord {
	store.mu.RLock()
	defer store.mu.RUnlock()

	return store.outboxRecordsByStatusLocked(ProductionOutboxStatusPending, limit)
}

// ClaimPendingOutboxRecords moves pending outbox records to in-flight in append
// order, recording one publisher attempt per claimed record.
func (store *InMemoryStore) ClaimPendingOutboxRecords(limit int, claimedAt time.Time) []ProductionOutboxRecord {
	store.mu.Lock()
	defer store.mu.Unlock()

	if limit <= 0 {
		return nil
	}
	claimedAt = claimedAt.UTC()
	if claimedAt.IsZero() {
		claimedAt = time.Unix(0, 0).UTC()
	}
	records := make([]ProductionOutboxRecord, 0, limit)
	for index := range store.outbox {
		if len(records) >= limit {
			break
		}
		if store.outbox[index].Status != ProductionOutboxStatusPending {
			continue
		}
		store.outbox[index].Status = ProductionOutboxStatusInFlight
		store.outbox[index].ClaimedAt = claimedAt
		store.outbox[index].Attempts++
		store.outbox[index].ClaimToken = productionOutboxClaimToken(store.outbox[index].OutboxID, store.outbox[index].Attempts)
		records = append(records, cloneProductionOutboxRecord(store.outbox[index]))
	}
	return records
}

// MarkClaimedOutboxPublished records successful delivery for the current claim
// attempt. Missing records, stale claim tokens, and non-in-flight records do
// not mutate state.
func (store *InMemoryStore) MarkClaimedOutboxPublished(outboxID string, claimToken string, publishedAt time.Time) (ProductionOutboxRecord, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	index, ok := store.outboxIndexLocked(outboxID)
	if !ok {
		return ProductionOutboxRecord{}, false
	}
	if !store.outboxClaimMatchesLocked(index, claimToken) {
		return ProductionOutboxRecord{}, false
	}
	store.outbox[index].Status = ProductionOutboxStatusPublished
	store.outbox[index].PublishedAt = publishedAt.UTC()
	return cloneProductionOutboxRecord(store.outbox[index]), true
}

// MarkClaimedOutboxFailed records failed delivery for the current claim
// attempt. Missing records, stale claim tokens, and non-in-flight records do
// not mutate state.
func (store *InMemoryStore) MarkClaimedOutboxFailed(outboxID string, claimToken string, reason string, failedAt time.Time) (ProductionOutboxRecord, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	index, ok := store.outboxIndexLocked(outboxID)
	if !ok {
		return ProductionOutboxRecord{}, false
	}
	if !store.outboxClaimMatchesLocked(index, claimToken) {
		return ProductionOutboxRecord{}, false
	}
	store.outbox[index].Status = ProductionOutboxStatusFailed
	store.outbox[index].FailedAt = failedAt.UTC()
	store.outbox[index].LastError = reason
	return cloneProductionOutboxRecord(store.outbox[index]), true
}

// RetryFailedOutboxRecords moves failed outbox records back to pending in
// append order while preserving the last failure evidence for diagnostics.
func (store *InMemoryStore) RetryFailedOutboxRecords(limit int, retriedAt time.Time) []ProductionOutboxRecord {
	store.mu.Lock()
	defer store.mu.Unlock()

	if limit <= 0 {
		return nil
	}
	retriedAt = retriedAt.UTC()
	records := make([]ProductionOutboxRecord, 0, limit)
	for index := range store.outbox {
		if len(records) >= limit {
			break
		}
		if store.outbox[index].Status != ProductionOutboxStatusFailed {
			continue
		}
		store.outbox[index].Status = ProductionOutboxStatusPending
		store.outbox[index].ClaimedAt = time.Time{}
		store.outbox[index].ClaimToken = ""
		store.outbox[index].RetriedAt = retriedAt
		records = append(records, cloneProductionOutboxRecord(store.outbox[index]))
	}
	return records
}

// ReleaseExpiredOutboxRecords moves stale in-flight outbox records back to
// pending in append order, preserving attempts and failure evidence.
func (store *InMemoryStore) ReleaseExpiredOutboxRecords(limit int, claimedBefore time.Time, retriedAt time.Time) []ProductionOutboxRecord {
	store.mu.Lock()
	defer store.mu.Unlock()

	if limit <= 0 || claimedBefore.IsZero() {
		return nil
	}
	claimedBefore = claimedBefore.UTC()
	retriedAt = retriedAt.UTC()
	records := make([]ProductionOutboxRecord, 0, limit)
	for index := range store.outbox {
		if len(records) >= limit {
			break
		}
		record := store.outbox[index]
		if record.Status != ProductionOutboxStatusInFlight || record.ClaimedAt.IsZero() || !record.ClaimedAt.Before(claimedBefore) {
			continue
		}
		store.outbox[index].Status = ProductionOutboxStatusPending
		store.outbox[index].ClaimedAt = time.Time{}
		store.outbox[index].ClaimToken = ""
		store.outbox[index].RetriedAt = retriedAt
		records = append(records, cloneProductionOutboxRecord(store.outbox[index]))
	}
	return records
}

func (store *InMemoryStore) appendOutboxRecordLocked(
	event gameevents.EventEnvelope,
	payload any,
	referenceKey foundation.IdempotencyKey,
	settlementWindow string,
) {
	store.nextOutboxSequence++
	sequence := store.nextOutboxSequence
	if referenceKey.IsZero() && settlementWindow == "" {
		referenceKey, settlementWindow = settlementEvidenceFromPayload(payload)
	}
	store.outbox = append(store.outbox, ProductionOutboxRecord{
		OutboxID:         fmt.Sprintf("production-outbox-%d", sequence),
		Sequence:         sequence,
		Event:            cloneProductionEventEnvelope(event),
		Status:           ProductionOutboxStatusPending,
		CreatedAt:        time.UnixMilli(event.ServerTime).UTC(),
		ReferenceKey:     referenceKey,
		SettlementWindow: settlementWindow,
	})
}

func (store *InMemoryStore) outboxRecordsByStatusLocked(status ProductionOutboxStatus, limit int) []ProductionOutboxRecord {
	if limit <= 0 {
		return nil
	}
	records := make([]ProductionOutboxRecord, 0, limit)
	for _, record := range store.outbox {
		if len(records) >= limit {
			break
		}
		if record.Status == status {
			records = append(records, cloneProductionOutboxRecord(record))
		}
	}
	return records
}

func (store *InMemoryStore) outboxIndexLocked(outboxID string) (int, bool) {
	for index, record := range store.outbox {
		if record.OutboxID == outboxID {
			return index, true
		}
	}
	return 0, false
}

func (store *InMemoryStore) outboxClaimMatchesLocked(index int, claimToken string) bool {
	record := store.outbox[index]
	return record.Status == ProductionOutboxStatusInFlight && record.ClaimToken != "" && record.ClaimToken == claimToken
}

func productionOutboxClaimToken(outboxID string, attempts int) string {
	return fmt.Sprintf("%s-attempt-%d", outboxID, attempts)
}

func settlementEvidenceFromPayload(payload any) (foundation.IdempotencyKey, string) {
	switch typed := payload.(type) {
	case ProductionSettlementPayload:
		return typed.ReferenceKey, typed.SettlementWindow
	case *ProductionSettlementPayload:
		if typed != nil {
			return typed.ReferenceKey, typed.SettlementWindow
		}
	case RouteSettlementPayload:
		return typed.ReferenceKey, typed.SettlementWindow
	case *RouteSettlementPayload:
		if typed != nil {
			return typed.ReferenceKey, typed.SettlementWindow
		}
	}
	return "", ""
}

func (store *InMemoryStore) hasSettlementReferenceLocked(referenceKey foundation.IdempotencyKey) bool {
	if referenceKey.IsZero() {
		return false
	}
	_, ok := store.references[referenceKey]
	return ok
}

func (store *InMemoryStore) recordSettlementReferenceLocked(record SettlementReferenceRecord) {
	store.references[record.ReferenceKey] = cloneSettlementReferenceRecord(record)
}

func cloneSettlementReferenceRecord(record SettlementReferenceRecord) SettlementReferenceRecord {
	record.AppliedAt = record.AppliedAt.UTC()
	record.RecordedAt = record.RecordedAt.UTC()
	return record
}

func cloneProductionOutboxRecords(records []ProductionOutboxRecord) []ProductionOutboxRecord {
	if len(records) == 0 {
		return nil
	}
	cloned := make([]ProductionOutboxRecord, len(records))
	for index, record := range records {
		cloned[index] = cloneProductionOutboxRecord(record)
	}
	return cloned
}

func cloneProductionOutboxRecord(record ProductionOutboxRecord) ProductionOutboxRecord {
	record.Event = cloneProductionEventEnvelope(record.Event)
	record.CreatedAt = record.CreatedAt.UTC()
	record.ClaimedAt = record.ClaimedAt.UTC()
	record.PublishedAt = record.PublishedAt.UTC()
	record.FailedAt = record.FailedAt.UTC()
	record.RetriedAt = record.RetriedAt.UTC()
	return record
}
