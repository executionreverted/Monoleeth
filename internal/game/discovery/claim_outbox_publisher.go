package discovery

import (
	"errors"
	"time"
)

var ErrInvalidClaimOutboxPublisher = errors.New("invalid claim outbox publisher")

// ClaimOutboxPublisherStore is the durable publisher boundary required by
// pending planet-claim outbox delivery. DB-backed stores should implement these
// methods with row-lock/CAS semantics around the claim token.
type ClaimOutboxPublisherStore interface {
	ClaimPendingClaimOutboxRecordsForPublish(limit int, claimedAt time.Time) ([]ClaimOutboxRecord, error)
	MarkClaimOutboxPublished(outboxID string, claimToken string, publishedAt time.Time) (ClaimOutboxRecord, bool, error)
	MarkClaimOutboxFailed(outboxID string, claimToken string, reason string, failedAt time.Time) (ClaimOutboxRecord, bool, error)
}

// ClaimOutboxLeaseReaperStore is the durable recovery boundary for stale
// in-flight claim outbox leases. DB-backed stores should implement this with a
// row-lock/CAS update that clears only rows whose claim lease is older than the
// cutoff.
type ClaimOutboxLeaseReaperStore interface {
	ReleaseExpiredClaimOutboxRecordsForPublish(limit int, claimedBefore time.Time, releasedAt time.Time) ([]ClaimOutboxRecord, error)
}

// ClaimOutboxRetryStore is the durable recovery boundary for explicitly
// retrying failed claim outbox rows after an operator or scheduled retry policy
// decides the failure is safe to replay.
type ClaimOutboxRetryStore interface {
	RetryFailedClaimOutboxRecordsForPublish(limit int, retriedAt time.Time) ([]ClaimOutboxRecord, error)
}

type ClaimOutboxPublishFunc func(ClaimOutboxRecord) error

type ClaimOutboxPublishInput struct {
	Store       ClaimOutboxPublisherStore
	Limit       int
	ClaimedAt   time.Time
	CompletedAt time.Time
	Publish     ClaimOutboxPublishFunc
}

type ClaimOutboxPublishResult struct {
	OutboxID   string
	ClaimToken string
	Record     ClaimOutboxRecord
	Published  bool
	Failed     bool
	StaleClaim bool
	StoreError bool
	Error      string
}

type ClaimOutboxLeaseReleaseInput struct {
	Store         ClaimOutboxLeaseReaperStore
	Limit         int
	ClaimedBefore time.Time
	ReleasedAt    time.Time
}

type ClaimOutboxRetryInput struct {
	Store     ClaimOutboxRetryStore
	Limit     int
	RetriedAt time.Time
}

// PublishPendingClaimOutbox claims pending records and records publish or
// failure through the same claim token returned by the store.
func PublishPendingClaimOutbox(input ClaimOutboxPublishInput) ([]ClaimOutboxPublishResult, error) {
	if input.Store == nil || input.Publish == nil {
		return nil, ErrInvalidClaimOutboxPublisher
	}
	if input.Limit <= 0 {
		return nil, nil
	}
	claimedAt := input.ClaimedAt.UTC()
	completedAt := input.CompletedAt.UTC()
	if completedAt.IsZero() {
		completedAt = claimedAt
	}

	claimed, err := input.Store.ClaimPendingClaimOutboxRecordsForPublish(input.Limit, claimedAt)
	if err != nil {
		return nil, err
	}
	results := make([]ClaimOutboxPublishResult, 0, len(claimed))
	for _, record := range claimed {
		result := ClaimOutboxPublishResult{
			OutboxID:   record.OutboxID,
			ClaimToken: record.ClaimToken,
			Record:     record,
		}
		if err := input.Publish(record); err != nil {
			result.Error = err.Error()
			failed, ok, failErr := input.Store.MarkClaimOutboxFailed(record.OutboxID, record.ClaimToken, result.Error, completedAt)
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
		published, ok, publishErr := input.Store.MarkClaimOutboxPublished(record.OutboxID, record.ClaimToken, completedAt)
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

// ReleaseExpiredClaimOutboxLeases returns stale in-flight claim outbox rows to
// pending through the same store boundary a durable lease reaper should use.
func ReleaseExpiredClaimOutboxLeases(input ClaimOutboxLeaseReleaseInput) ([]ClaimOutboxRecord, error) {
	if input.Store == nil {
		return nil, ErrInvalidClaimOutboxPublisher
	}
	if input.Limit <= 0 || input.ClaimedBefore.IsZero() {
		return nil, nil
	}
	return input.Store.ReleaseExpiredClaimOutboxRecordsForPublish(input.Limit, input.ClaimedBefore.UTC(), input.ReleasedAt.UTC())
}

// RetryFailedClaimOutboxRows returns failed claim outbox rows to pending through
// the explicit durable retry boundary. It preserves failure evidence while
// clearing stale claim lease fields.
func RetryFailedClaimOutboxRows(input ClaimOutboxRetryInput) ([]ClaimOutboxRecord, error) {
	if input.Store == nil {
		return nil, ErrInvalidClaimOutboxPublisher
	}
	if input.Limit <= 0 {
		return nil, nil
	}
	return input.Store.RetryFailedClaimOutboxRecordsForPublish(input.Limit, input.RetriedAt.UTC())
}

func (service *ClaimService) ClaimPendingClaimOutboxRecordsForPublish(limit int, claimedAt time.Time) ([]ClaimOutboxRecord, error) {
	if service == nil {
		return nil, ErrInvalidClaimOutboxPublisher
	}
	return service.ClaimPendingClaimOutboxRecords(limit, claimedAt), nil
}

func (service *ClaimService) MarkClaimOutboxPublished(outboxID string, claimToken string, publishedAt time.Time) (ClaimOutboxRecord, bool, error) {
	if service == nil {
		return ClaimOutboxRecord{}, false, ErrInvalidClaimOutboxPublisher
	}
	record, ok := service.MarkClaimedClaimOutboxPublished(outboxID, claimToken, publishedAt)
	return record, ok, nil
}

func (service *ClaimService) MarkClaimOutboxFailed(outboxID string, claimToken string, reason string, failedAt time.Time) (ClaimOutboxRecord, bool, error) {
	if service == nil {
		return ClaimOutboxRecord{}, false, ErrInvalidClaimOutboxPublisher
	}
	record, ok := service.MarkClaimedClaimOutboxFailed(outboxID, claimToken, reason, failedAt)
	return record, ok, nil
}

func (service *ClaimService) ReleaseExpiredClaimOutboxRecordsForPublish(limit int, claimedBefore time.Time, releasedAt time.Time) ([]ClaimOutboxRecord, error) {
	if service == nil {
		return nil, ErrInvalidClaimOutboxPublisher
	}
	return service.ReleaseExpiredClaimOutboxRecords(limit, claimedBefore, releasedAt), nil
}

func (service *ClaimService) RetryFailedClaimOutboxRecordsForPublish(limit int, retriedAt time.Time) ([]ClaimOutboxRecord, error) {
	if service == nil {
		return nil, ErrInvalidClaimOutboxPublisher
	}
	return service.RetryFailedClaimOutboxRecords(limit, retriedAt), nil
}
