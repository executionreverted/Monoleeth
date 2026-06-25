package economy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"
)

func TestOutboxReplayWorkerSuccessPublishesOnce(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 20, 0, 0, 0, time.UTC)
	store := newFakeOutboxReplayStore(t, validOutboxRow(t, "outbox-worker-success", now.Add(-time.Minute)))
	published := make([]string, 0, 1)
	worker := OutboxReplayWorker{
		Store: store,
		Publisher: func(_ context.Context, row OutboxRow) error {
			if row.Status != OutboxStatusLeased || row.LeaseOwner != "worker-1" {
				t.Fatalf("publisher row = %+v, want leased by worker-1", row)
			}
			published = append(published, row.OutboxID)
			return nil
		},
		LeaseOwner:    "worker-1",
		BatchSize:     10,
		LeaseDuration: time.Minute,
		RetryDelay:    10 * time.Second,
		Now:           func() time.Time { return now },
	}

	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce() error = %v, want nil", err)
	}
	if result != (OutboxReplayResult{Loaded: 1, Leased: 1, Published: 1}) {
		t.Fatalf("RunOnce() result = %+v, want one published row", result)
	}
	if len(published) != 1 || published[0] != "outbox-worker-success" {
		t.Fatalf("published = %+v, want outbox-worker-success once", published)
	}
	stored, ok, err := store.LoadOutboxRow(ctx, "outbox-worker-success")
	if err != nil || !ok {
		t.Fatalf("LoadOutboxRow() = ok %v err %v, want true nil", ok, err)
	}
	if stored.Status != OutboxStatusPublished || stored.AttemptCount != 1 || stored.PublishedAt.IsZero() {
		t.Fatalf("stored row = %+v, want published attempt 1", stored)
	}

	result, err = worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce(second) error = %v, want nil", err)
	}
	if result != (OutboxReplayResult{}) {
		t.Fatalf("RunOnce(second) result = %+v, want no due rows", result)
	}
	if len(published) != 1 {
		t.Fatalf("published count after second run = %d, want 1", len(published))
	}
}

func TestOutboxReplayWorkerPublisherErrorRecordsFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 20, 15, 0, 0, time.UTC)
	retryDelay := 15 * time.Second
	store := newFakeOutboxReplayStore(t, validOutboxRow(t, "outbox-worker-failure", now.Add(-time.Minute)))
	worker := OutboxReplayWorker{
		Store:         store,
		Publisher:     func(context.Context, OutboxRow) error { return errors.New("bus down") },
		LeaseOwner:    "worker-1",
		BatchSize:     10,
		LeaseDuration: time.Minute,
		RetryDelay:    retryDelay,
		Now:           func() time.Time { return now },
	}

	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce() error = %v, want nil after recorded publisher failure", err)
	}
	if result != (OutboxReplayResult{Loaded: 1, Leased: 1, Failed: 1}) {
		t.Fatalf("RunOnce() result = %+v, want one recorded failure", result)
	}
	stored, ok, err := store.LoadOutboxRow(ctx, "outbox-worker-failure")
	if err != nil || !ok {
		t.Fatalf("LoadOutboxRow() = ok %v err %v, want true nil", ok, err)
	}
	if stored.Status != OutboxStatusFailed || stored.AttemptCount != 1 || stored.LastError != "bus down" {
		t.Fatalf("stored row = %+v, want failed attempt with last error", stored)
	}
	if !stored.AvailableAt.Equal(now.Add(retryDelay)) {
		t.Fatalf("available_at = %s, want %s", stored.AvailableAt, now.Add(retryDelay))
	}
}

func TestOutboxReplayWorkerRetriesFailedRow(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 20, 30, 0, 0, time.UTC)
	currentTime := now
	store := newFakeOutboxReplayStore(t, validOutboxRow(t, "outbox-worker-retry", now.Add(-time.Minute)))
	publishCalls := 0
	worker := OutboxReplayWorker{
		Store: store,
		Publisher: func(context.Context, OutboxRow) error {
			publishCalls++
			if publishCalls == 1 {
				return errors.New("temporary bus failure")
			}
			return nil
		},
		LeaseOwner:    "worker-1",
		BatchSize:     10,
		LeaseDuration: time.Minute,
		RetryDelay:    10 * time.Second,
		Now:           func() time.Time { return currentTime },
	}

	first, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce(first) error = %v, want nil", err)
	}
	if first != (OutboxReplayResult{Loaded: 1, Leased: 1, Failed: 1}) {
		t.Fatalf("RunOnce(first) result = %+v, want one failed row", first)
	}

	currentTime = now.Add(10 * time.Second)
	second, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce(second) error = %v, want nil", err)
	}
	if second != (OutboxReplayResult{Loaded: 1, Leased: 1, Published: 1}) {
		t.Fatalf("RunOnce(second) result = %+v, want failed row published on retry", second)
	}
	if publishCalls != 2 {
		t.Fatalf("publish calls = %d, want 2", publishCalls)
	}
	stored, ok, err := store.LoadOutboxRow(ctx, "outbox-worker-retry")
	if err != nil || !ok {
		t.Fatalf("LoadOutboxRow() = ok %v err %v, want true nil", ok, err)
	}
	if stored.Status != OutboxStatusPublished || stored.AttemptCount != 2 || stored.LastError != "" {
		t.Fatalf("stored row = %+v, want published after retry attempt 2", stored)
	}
}

type fakeOutboxReplayStore struct {
	rows map[string]OutboxRow
}

func newFakeOutboxReplayStore(t *testing.T, rows ...OutboxRow) *fakeOutboxReplayStore {
	t.Helper()
	store := &fakeOutboxReplayStore{rows: make(map[string]OutboxRow, len(rows))}
	for _, row := range rows {
		if err := store.InsertOutboxRow(context.Background(), row); err != nil {
			t.Fatalf("InsertOutboxRow(%q) error = %v, want nil", row.OutboxID, err)
		}
	}
	return store
}

func (store *fakeOutboxReplayStore) InsertOutboxRow(ctx context.Context, row OutboxRow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	inserted, err := NewOutboxRow(row)
	if err != nil {
		return err
	}
	if _, exists := store.rows[inserted.OutboxID]; exists {
		return fmt.Errorf("outbox %q: %w", inserted.OutboxID, ErrInvalidOutboxRow)
	}
	store.rows[inserted.OutboxID] = inserted.Clone()
	return nil
}

func (store *fakeOutboxReplayStore) LoadOutboxRow(ctx context.Context, outboxID string) (OutboxRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return OutboxRow{}, false, err
	}
	row, ok := store.rows[outboxID]
	return row.Clone(), ok, nil
}

func (store *fakeOutboxReplayStore) LoadDueOutboxRows(ctx context.Context, query OutboxDueRowsQuery) ([]OutboxRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}
	rows := make([]OutboxRow, 0, query.Limit)
	for _, row := range store.rows {
		if row.Status != OutboxStatusPending && row.Status != OutboxStatusFailed {
			continue
		}
		if row.AvailableAt.After(query.Now) {
			continue
		}
		rows = append(rows, row.Clone())
	}
	sort.Slice(rows, func(left int, right int) bool {
		if !rows[left].AvailableAt.Equal(rows[right].AvailableAt) {
			return rows[left].AvailableAt.Before(rows[right].AvailableAt)
		}
		if !rows[left].CreatedAt.Equal(rows[right].CreatedAt) {
			return rows[left].CreatedAt.Before(rows[right].CreatedAt)
		}
		return rows[left].OutboxID < rows[right].OutboxID
	})
	if len(rows) > query.Limit {
		rows = rows[:query.Limit]
	}
	return rows, nil
}

func (store *fakeOutboxReplayStore) LeaseOutboxRow(ctx context.Context, input OutboxLeaseInput) (OutboxRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return OutboxRow{}, false, err
	}
	if err := input.Validate(); err != nil {
		return OutboxRow{}, false, err
	}
	row, ok := store.rows[input.OutboxID]
	if !ok || !outboxLeaseEligible(row, input.Now) {
		return OutboxRow{}, false, nil
	}
	row.Status = OutboxStatusLeased
	row.LeaseOwner = input.LeaseOwner
	row.LeasedUntil = input.LeasedUntil
	row.UpdatedAt = input.Now
	if err := row.Validate(); err != nil {
		return OutboxRow{}, false, err
	}
	store.rows[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func (store *fakeOutboxReplayStore) MarkOutboxPublished(ctx context.Context, input OutboxPublishInput) (OutboxRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return OutboxRow{}, false, err
	}
	if err := input.Validate(); err != nil {
		return OutboxRow{}, false, err
	}
	row, ok := store.rows[input.OutboxID]
	if !ok || !outboxLeaseMatches(row, input.LeaseOwner, input.Now) {
		return OutboxRow{}, false, nil
	}
	row.Status = OutboxStatusPublished
	row.LeaseOwner = ""
	row.LeasedUntil = time.Time{}
	row.AttemptCount++
	row.LastError = ""
	row.UpdatedAt = input.Now
	row.PublishedAt = input.Now
	if err := row.Validate(); err != nil {
		return OutboxRow{}, false, err
	}
	store.rows[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func (store *fakeOutboxReplayStore) MarkOutboxFailed(ctx context.Context, input OutboxFailureInput) (OutboxRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return OutboxRow{}, false, err
	}
	if err := input.Validate(); err != nil {
		return OutboxRow{}, false, err
	}
	row, ok := store.rows[input.OutboxID]
	if !ok || !outboxLeaseMatches(row, input.LeaseOwner, input.Now) {
		return OutboxRow{}, false, nil
	}
	row.AttemptCount++
	if row.AttemptCount >= row.MaxAttempts {
		row.Status = OutboxStatusDead
	} else {
		row.Status = OutboxStatusFailed
	}
	row.AvailableAt = input.AvailableAt
	row.LeaseOwner = ""
	row.LeasedUntil = time.Time{}
	row.LastError = input.LastError
	row.UpdatedAt = input.Now
	if err := row.Validate(); err != nil {
		return OutboxRow{}, false, err
	}
	store.rows[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}
