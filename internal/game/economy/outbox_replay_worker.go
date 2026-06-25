package economy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	defaultOutboxReplayBatchSize     = 10
	defaultOutboxReplayLeaseDuration = 30 * time.Second
	defaultOutboxReplayRetryDelay    = time.Second
)

var ErrInvalidOutboxReplayWorker = errors.New("invalid outbox replay worker")

type OutboxPublisher func(ctx context.Context, row OutboxRow) error

type OutboxReplayWorker struct {
	Store         OutboxStore
	Publisher     OutboxPublisher
	LeaseOwner    string
	BatchSize     int
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	Now           func() time.Time
}

type OutboxReplayResult struct {
	Loaded    int
	Leased    int
	Published int
	Failed    int
	Skipped   int
}

func (worker OutboxReplayWorker) RunOnce(ctx context.Context) (OutboxReplayResult, error) {
	worker, err := worker.withDefaults()
	if err != nil {
		return OutboxReplayResult{}, err
	}

	now := worker.now()
	due, err := worker.Store.LoadDueOutboxRows(ctx, OutboxDueRowsQuery{
		Now:   now,
		Limit: worker.BatchSize,
	})
	if err != nil {
		return OutboxReplayResult{}, err
	}

	result := OutboxReplayResult{Loaded: len(due)}
	for _, row := range due {
		leased, ok, err := worker.Store.LeaseOutboxRow(ctx, OutboxLeaseInput{
			OutboxID:    row.OutboxID,
			LeaseOwner:  worker.LeaseOwner,
			Now:         now,
			LeasedUntil: now.Add(worker.LeaseDuration),
		})
		if err != nil {
			return result, err
		}
		if !ok {
			result.Skipped++
			continue
		}
		result.Leased++

		if err := worker.Publisher(ctx, leased.Clone()); err != nil {
			marked, ok, markErr := worker.Store.MarkOutboxFailed(ctx, OutboxFailureInput{
				OutboxID:    leased.OutboxID,
				LeaseOwner:  worker.LeaseOwner,
				LastError:   outboxReplayError(err),
				Now:         now,
				AvailableAt: now.Add(worker.RetryDelay),
			})
			if markErr != nil {
				return result, markErr
			}
			if !ok {
				return result, fmt.Errorf("mark outbox %q failed miss: %w", leased.OutboxID, ErrInvalidOutboxReplayWorker)
			}
			if marked.Status == OutboxStatusFailed || marked.Status == OutboxStatusDead {
				result.Failed++
			}
			continue
		}

		if _, ok, err := worker.Store.MarkOutboxPublished(ctx, OutboxPublishInput{
			OutboxID:   leased.OutboxID,
			LeaseOwner: worker.LeaseOwner,
			Now:        now,
		}); err != nil {
			return result, err
		} else if !ok {
			return result, fmt.Errorf("mark outbox %q published miss: %w", leased.OutboxID, ErrInvalidOutboxReplayWorker)
		}
		result.Published++
	}

	return result, nil
}

func (worker OutboxReplayWorker) withDefaults() (OutboxReplayWorker, error) {
	if worker.Store == nil {
		return OutboxReplayWorker{}, fmt.Errorf("store: %w", ErrInvalidOutboxReplayWorker)
	}
	if worker.Publisher == nil {
		return OutboxReplayWorker{}, fmt.Errorf("publisher: %w", ErrInvalidOutboxReplayWorker)
	}
	if strings.TrimSpace(worker.LeaseOwner) == "" || worker.LeaseOwner != strings.TrimSpace(worker.LeaseOwner) {
		return OutboxReplayWorker{}, fmt.Errorf("lease_owner %q: %w", worker.LeaseOwner, ErrInvalidOutboxReplayWorker)
	}
	if worker.BatchSize == 0 {
		worker.BatchSize = defaultOutboxReplayBatchSize
	}
	if worker.BatchSize < 0 || worker.BatchSize > OutboxMaxDueRowsLimit {
		return OutboxReplayWorker{}, fmt.Errorf("batch_size %d: %w", worker.BatchSize, ErrInvalidOutboxReplayWorker)
	}
	if worker.LeaseDuration == 0 {
		worker.LeaseDuration = defaultOutboxReplayLeaseDuration
	}
	if worker.LeaseDuration < 0 {
		return OutboxReplayWorker{}, fmt.Errorf("lease_duration %s: %w", worker.LeaseDuration, ErrInvalidOutboxReplayWorker)
	}
	if worker.RetryDelay == 0 {
		worker.RetryDelay = defaultOutboxReplayRetryDelay
	}
	if worker.RetryDelay < 0 {
		return OutboxReplayWorker{}, fmt.Errorf("retry_delay %s: %w", worker.RetryDelay, ErrInvalidOutboxReplayWorker)
	}
	if worker.Now == nil {
		worker.Now = time.Now
	}
	if worker.now().IsZero() {
		return OutboxReplayWorker{}, fmt.Errorf("now: %w", ErrInvalidOutboxReplayWorker)
	}
	return worker, nil
}

func (worker OutboxReplayWorker) now() time.Time {
	return worker.Now().UTC()
}

func outboxReplayError(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "publisher failed"
	}
	return message
}
