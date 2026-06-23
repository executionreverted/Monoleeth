package production

import (
	"errors"
	"time"
)

var ErrInvalidProductionOutboxPublisher = errors.New("invalid production outbox publisher")

// ProductionOutboxPublisherStore is the durable publisher boundary required by
// pending production-domain outbox delivery, including production settlements,
// route settlements, and building mutations. DB-backed stores should implement
// these methods with row-lock/CAS semantics around the claim token.
type ProductionOutboxPublisherStore interface {
	ClaimPendingProductionOutboxRecords(limit int, claimedAt time.Time) ([]ProductionOutboxRecord, error)
	MarkProductionOutboxPublished(outboxID string, claimToken string, publishedAt time.Time) (ProductionOutboxRecord, bool, error)
	MarkProductionOutboxFailed(outboxID string, claimToken string, reason string, failedAt time.Time) (ProductionOutboxRecord, bool, error)
}

// ProductionOutboxLeaseReaperStore is the durable recovery boundary for stale
// in-flight production-domain outbox leases, including production settlements,
// route settlements, and building mutations. DB-backed stores should implement
// this with a row-lock/CAS update that clears only rows whose claim lease is
// older than the cutoff.
type ProductionOutboxLeaseReaperStore interface {
	ReleaseExpiredProductionOutboxRecords(limit int, claimedBefore time.Time, releasedAt time.Time) ([]ProductionOutboxRecord, error)
}

// ProductionOutboxRetryStore is the durable recovery boundary for explicitly
// retrying failed production-domain outbox rows after a retry policy decides
// the failure is safe to replay.
type ProductionOutboxRetryStore interface {
	RetryFailedProductionOutboxRecords(limit int, retriedAt time.Time) ([]ProductionOutboxRecord, error)
}

type ProductionOutboxPublishFunc func(ProductionOutboxRecord) error

type ProductionOutboxPublishInput struct {
	Store       ProductionOutboxPublisherStore
	Limit       int
	ClaimedAt   time.Time
	CompletedAt time.Time
	Publish     ProductionOutboxPublishFunc
}

type ProductionOutboxPublishResult struct {
	OutboxID   string
	ClaimToken string
	Record     ProductionOutboxRecord
	Published  bool
	Failed     bool
	StaleClaim bool
	StoreError bool
	Error      string
}

type ProductionOutboxLeaseReleaseInput struct {
	Store         ProductionOutboxLeaseReaperStore
	Limit         int
	ClaimedBefore time.Time
	ReleasedAt    time.Time
}

type ProductionOutboxRetryInput struct {
	Store     ProductionOutboxRetryStore
	Limit     int
	RetriedAt time.Time
}

// PublishPendingProductionOutbox claims pending records and records publish or
// failure through the same claim token returned by the store.
func PublishPendingProductionOutbox(input ProductionOutboxPublishInput) ([]ProductionOutboxPublishResult, error) {
	if input.Store == nil || input.Publish == nil {
		return nil, ErrInvalidProductionOutboxPublisher
	}
	if input.Limit <= 0 {
		return nil, nil
	}
	claimedAt := input.ClaimedAt.UTC()
	completedAt := input.CompletedAt.UTC()
	if completedAt.IsZero() {
		completedAt = claimedAt
	}

	claimed, err := input.Store.ClaimPendingProductionOutboxRecords(input.Limit, claimedAt)
	if err != nil {
		return nil, err
	}
	results := make([]ProductionOutboxPublishResult, 0, len(claimed))
	for _, record := range claimed {
		result := ProductionOutboxPublishResult{
			OutboxID:   record.OutboxID,
			ClaimToken: record.ClaimToken,
			Record:     record,
		}
		if err := input.Publish(record); err != nil {
			result.Error = err.Error()
			failed, ok, failErr := input.Store.MarkProductionOutboxFailed(record.OutboxID, record.ClaimToken, result.Error, completedAt)
			if failErr != nil {
				result.Error = failErr.Error()
				result.StoreError = true
				results = append(results, result)
				return results, failErr
			}
			if ok {
				result.Record = failed
				result.Failed = true
			} else {
				result.StaleClaim = true
			}
			results = append(results, result)
			continue
		}
		published, ok, publishErr := input.Store.MarkProductionOutboxPublished(record.OutboxID, record.ClaimToken, completedAt)
		if publishErr != nil {
			result.Error = publishErr.Error()
			result.StoreError = true
			results = append(results, result)
			return results, publishErr
		}
		if ok {
			result.Record = published
			result.Published = true
		} else {
			result.StaleClaim = true
		}
		results = append(results, result)
	}
	return results, nil
}

// ReleaseExpiredProductionOutboxLeases returns stale in-flight production-domain
// outbox rows to pending through the same store boundary a durable lease reaper
// should use.
func ReleaseExpiredProductionOutboxLeases(input ProductionOutboxLeaseReleaseInput) ([]ProductionOutboxRecord, error) {
	if input.Store == nil {
		return nil, ErrInvalidProductionOutboxPublisher
	}
	if input.Limit <= 0 || input.ClaimedBefore.IsZero() {
		return nil, nil
	}
	return input.Store.ReleaseExpiredProductionOutboxRecords(input.Limit, input.ClaimedBefore.UTC(), input.ReleasedAt.UTC())
}

// RetryFailedProductionOutboxRows returns failed production-domain outbox rows
// to pending through the explicit durable retry boundary. It preserves failure
// evidence while clearing stale claim lease fields.
func RetryFailedProductionOutboxRows(input ProductionOutboxRetryInput) ([]ProductionOutboxRecord, error) {
	if input.Store == nil {
		return nil, ErrInvalidProductionOutboxPublisher
	}
	if input.Limit <= 0 {
		return nil, nil
	}
	return input.Store.RetryFailedProductionOutboxRecords(input.Limit, input.RetriedAt.UTC())
}

func (store *InMemoryStore) ClaimPendingProductionOutboxRecords(limit int, claimedAt time.Time) ([]ProductionOutboxRecord, error) {
	if store == nil {
		return nil, ErrInvalidProductionOutboxPublisher
	}
	return store.ClaimPendingOutboxRecords(limit, claimedAt), nil
}

func (store *InMemoryStore) MarkProductionOutboxPublished(outboxID string, claimToken string, publishedAt time.Time) (ProductionOutboxRecord, bool, error) {
	if store == nil {
		return ProductionOutboxRecord{}, false, ErrInvalidProductionOutboxPublisher
	}
	record, ok := store.MarkClaimedOutboxPublished(outboxID, claimToken, publishedAt)
	return record, ok, nil
}

func (store *InMemoryStore) MarkProductionOutboxFailed(outboxID string, claimToken string, reason string, failedAt time.Time) (ProductionOutboxRecord, bool, error) {
	if store == nil {
		return ProductionOutboxRecord{}, false, ErrInvalidProductionOutboxPublisher
	}
	record, ok := store.MarkClaimedOutboxFailed(outboxID, claimToken, reason, failedAt)
	return record, ok, nil
}

func (store *InMemoryStore) ReleaseExpiredProductionOutboxRecords(limit int, claimedBefore time.Time, releasedAt time.Time) ([]ProductionOutboxRecord, error) {
	if store == nil {
		return nil, ErrInvalidProductionOutboxPublisher
	}
	return store.ReleaseExpiredOutboxRecords(limit, claimedBefore, releasedAt), nil
}

func (store *InMemoryStore) RetryFailedProductionOutboxRecords(limit int, retriedAt time.Time) ([]ProductionOutboxRecord, error) {
	if store == nil {
		return nil, ErrInvalidProductionOutboxPublisher
	}
	return store.RetryFailedOutboxRecords(limit, retriedAt), nil
}
