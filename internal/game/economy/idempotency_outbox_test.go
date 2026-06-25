package economy

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestIdempotencyDuplicateKeyReturnsStableExistingRow(t *testing.T) {
	store := newMemoryEconomyContractStore()
	completedAt := time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC)
	existing := IdempotencyKeyRow{
		Scope:       IdempotencyScopeEconomy,
		Key:         validReferenceKey(t, "loot_pickup:drop-stable"),
		Operation:   "loot_pickup",
		PlayerID:    "player-1",
		RequestHash: "sha256:first",
		Status:      IdempotencyStatusCompleted,
		ResultJSON:  json.RawMessage(`{"ledger_id":"ledger-1"}`),
		CreatedAt:   completedAt.Add(-time.Minute),
		UpdatedAt:   completedAt,
		CompletedAt: completedAt,
	}
	if _, err := store.CompleteIdempotencyKey(existing); err != nil {
		t.Fatalf("CompleteIdempotencyKey(existing) error = %v, want nil", err)
	}
	candidate := existing.Clone()
	candidate.Status = IdempotencyStatusInProgress
	candidate.ResultJSON = json.RawMessage(`{"ledger_id":"candidate"}`)
	candidate.CompletedAt = time.Time{}

	result, err := store.ClaimIdempotencyKey(candidate)
	if err != nil {
		t.Fatalf("ClaimIdempotencyKey(duplicate) error = %v, want nil", err)
	}
	if !result.Duplicate {
		t.Fatal("ClaimIdempotencyKey duplicate = false, want true")
	}
	if string(result.Row.ResultJSON) != `{"ledger_id":"ledger-1"}` || result.Row.Status != IdempotencyStatusCompleted {
		t.Fatalf("duplicate row = %+v, want stable completed existing row", result.Row)
	}
	result.Row.ResultJSON[14] = 'X'
	if string(existing.ResultJSON) != `{"ledger_id":"ledger-1"}` {
		t.Fatalf("existing ResultJSON mutated to %s", existing.ResultJSON)
	}
}

func TestIdempotencyDuplicateKeyConflictRejected(t *testing.T) {
	store := newMemoryEconomyContractStore()
	existing := validIdempotencyKeyRow(t)
	if _, err := store.ClaimIdempotencyKey(existing); err != nil {
		t.Fatalf("ClaimIdempotencyKey(existing) error = %v, want nil", err)
	}
	candidate := existing.Clone()
	candidate.Operation = "market_buy"

	_, err := store.ClaimIdempotencyKey(candidate)

	if !errors.Is(err, ErrIdempotencyKeyConflict) {
		t.Fatalf("ClaimIdempotencyKey(conflict) error = %v, want ErrIdempotencyKeyConflict", err)
	}
}

func TestOutboxRowInsertReadShape(t *testing.T) {
	store := newMemoryEconomyContractStore()
	createdAt := time.Date(2026, 6, 25, 16, 0, 0, 0, time.UTC)
	payload := json.RawMessage(`{"player_id":"player-1","amount":25}`)
	row, err := NewOutboxRow(OutboxRow{
		OutboxID:         "outbox-1",
		Topic:            "economy",
		EventType:        EventWalletCredited,
		AggregateType:    "player",
		AggregateID:      "player-1",
		IdempotencyScope: IdempotencyScopeEconomy,
		IdempotencyKey:   validReferenceKey(t, "quest_reward:outbox-shape"),
		PayloadJSON:      payload,
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt,
	})
	if err != nil {
		t.Fatalf("NewOutboxRow() error = %v, want nil", err)
	}

	payload[14] = 'X'
	if string(row.PayloadJSON) != `{"player_id":"player-1","amount":25}` {
		t.Fatalf("row PayloadJSON = %s, want cloned insert payload", row.PayloadJSON)
	}
	if row.Status != OutboxStatusPending || row.AttemptCount != 0 || row.MaxAttempts != 20 || !row.AvailableAt.Equal(createdAt) {
		t.Fatalf("row retry shape = %+v, want pending attempt 0 max 20 available_at created_at", row)
	}
	if err := store.InsertOutboxRow(row); err != nil {
		t.Fatalf("InsertOutboxRow() error = %v, want nil", err)
	}

	read, ok, err := store.LoadOutboxRow(row.OutboxID)
	if err != nil {
		t.Fatalf("LoadOutboxRow() error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("LoadOutboxRow(%q) ok = false, want true", row.OutboxID)
	}
	if read.OutboxID != row.OutboxID ||
		read.Topic != row.Topic ||
		read.EventType != row.EventType ||
		read.IdempotencyKey != row.IdempotencyKey ||
		string(read.PayloadJSON) != `{"player_id":"player-1","amount":25}` {
		t.Fatalf("read row = %+v, want inserted outbox shape", read)
	}
	read.PayloadJSON[29] = '9'
	again, ok, err := store.LoadOutboxRow(row.OutboxID)
	if err != nil || !ok {
		t.Fatalf("LoadOutboxRow(again) = ok %v err %v, want true nil", ok, err)
	}
	if string(again.PayloadJSON) != `{"player_id":"player-1","amount":25}` {
		t.Fatalf("stored PayloadJSON after read mutation = %s", again.PayloadJSON)
	}
	if err := read.Validate(); err != nil {
		t.Fatalf("read Validate() error = %v, want nil", err)
	}
}

func TestOutboxMissedPendingRowCanBeLeased(t *testing.T) {
	store := newMemoryEconomyContractStore()
	now := time.Date(2026, 6, 25, 18, 0, 0, 0, time.UTC)
	row := validOutboxRow(t, "outbox-pending-lease", now.Add(-time.Minute))
	if err := store.InsertOutboxRow(row); err != nil {
		t.Fatalf("InsertOutboxRow() error = %v, want nil", err)
	}
	due, err := store.LoadDueOutboxRows(OutboxDueRowsQuery{Now: now, Limit: 1})
	if err != nil {
		t.Fatalf("LoadDueOutboxRows() error = %v, want nil", err)
	}
	if len(due) != 1 || due[0].OutboxID != row.OutboxID {
		t.Fatalf("due rows = %+v, want pending row", due)
	}

	leased, ok, err := store.LeaseOutboxRow(OutboxLeaseInput{
		OutboxID:    row.OutboxID,
		LeaseOwner:  "publisher-1",
		Now:         now,
		LeasedUntil: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("LeaseOutboxRow() error = %v, want nil", err)
	}
	if !ok || leased.Status != OutboxStatusLeased || leased.LeaseOwner != "publisher-1" || leased.AttemptCount != 0 {
		t.Fatalf("lease result = %+v ok %v, want leased without attempt increment", leased, ok)
	}
}

func TestOutboxPublishedRowNoLongerDue(t *testing.T) {
	store := newMemoryEconomyContractStore()
	now := time.Date(2026, 6, 25, 18, 15, 0, 0, time.UTC)
	row := validOutboxRow(t, "outbox-published-not-due", now.Add(-time.Minute))
	if err := store.InsertOutboxRow(row); err != nil {
		t.Fatalf("InsertOutboxRow() error = %v, want nil", err)
	}
	if _, ok, err := store.LeaseOutboxRow(OutboxLeaseInput{
		OutboxID:    row.OutboxID,
		LeaseOwner:  "publisher-1",
		Now:         now,
		LeasedUntil: now.Add(time.Minute),
	}); err != nil || !ok {
		t.Fatalf("LeaseOutboxRow() = ok %v err %v, want true nil", ok, err)
	}
	published, ok, err := store.MarkOutboxPublished(OutboxPublishInput{
		OutboxID:   row.OutboxID,
		LeaseOwner: "publisher-1",
		Now:        now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("MarkOutboxPublished() error = %v, want nil", err)
	}
	if !ok || published.Status != OutboxStatusPublished || published.AttemptCount != 1 || published.PublishedAt.IsZero() {
		t.Fatalf("published row = %+v ok %v, want published attempt increment", published, ok)
	}

	due, err := store.LoadDueOutboxRows(OutboxDueRowsQuery{Now: now.Add(time.Minute), Limit: 10})
	if err != nil {
		t.Fatalf("LoadDueOutboxRows(after publish) error = %v, want nil", err)
	}
	if len(due) != 0 {
		t.Fatalf("due rows after publish = %+v, want none", due)
	}
}

func TestOutboxFailedRowRetryDue(t *testing.T) {
	store := newMemoryEconomyContractStore()
	now := time.Date(2026, 6, 25, 18, 30, 0, 0, time.UTC)
	row := validOutboxRow(t, "outbox-failed-retry-due", now.Add(-time.Minute))
	row.MaxAttempts = 3
	if err := store.InsertOutboxRow(row); err != nil {
		t.Fatalf("InsertOutboxRow() error = %v, want nil", err)
	}
	if _, ok, err := store.LeaseOutboxRow(OutboxLeaseInput{
		OutboxID:    row.OutboxID,
		LeaseOwner:  "publisher-1",
		Now:         now,
		LeasedUntil: now.Add(time.Minute),
	}); err != nil || !ok {
		t.Fatalf("LeaseOutboxRow() = ok %v err %v, want true nil", ok, err)
	}

	failed, ok, err := store.MarkOutboxFailed(OutboxFailureInput{
		OutboxID:    row.OutboxID,
		LeaseOwner:  "publisher-1",
		LastError:   "publish timeout",
		Now:         now.Add(10 * time.Second),
		AvailableAt: now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("MarkOutboxFailed() error = %v, want nil", err)
	}
	if !ok || failed.Status != OutboxStatusFailed || failed.AttemptCount != 1 || failed.LastError != "publish timeout" {
		t.Fatalf("failed row = %+v ok %v, want failed retry row", failed, ok)
	}
	due, err := store.LoadDueOutboxRows(OutboxDueRowsQuery{Now: now.Add(10 * time.Second), Limit: 10})
	if err != nil {
		t.Fatalf("LoadDueOutboxRows(retry) error = %v, want nil", err)
	}
	if len(due) != 1 || due[0].OutboxID != row.OutboxID || due[0].Status != OutboxStatusFailed {
		t.Fatalf("due rows after failure = %+v, want failed retry due", due)
	}
}

func TestOutboxDuplicateLeaseGuarded(t *testing.T) {
	store := newMemoryEconomyContractStore()
	now := time.Date(2026, 6, 25, 18, 45, 0, 0, time.UTC)
	row := validOutboxRow(t, "outbox-duplicate-lease", now.Add(-time.Minute))
	if err := store.InsertOutboxRow(row); err != nil {
		t.Fatalf("InsertOutboxRow() error = %v, want nil", err)
	}
	first, ok, err := store.LeaseOutboxRow(OutboxLeaseInput{
		OutboxID:    row.OutboxID,
		LeaseOwner:  "publisher-1",
		Now:         now,
		LeasedUntil: now.Add(time.Minute),
	})
	if err != nil || !ok {
		t.Fatalf("first LeaseOutboxRow() = ok %v err %v, want true nil", ok, err)
	}

	second, ok, err := store.LeaseOutboxRow(OutboxLeaseInput{
		OutboxID:    row.OutboxID,
		LeaseOwner:  "publisher-2",
		Now:         now.Add(10 * time.Second),
		LeasedUntil: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("second LeaseOutboxRow() error = %v, want nil", err)
	}
	if ok || second.OutboxID != "" {
		t.Fatalf("second lease = %+v ok %v, want guarded miss", second, ok)
	}
	stored, ok, err := store.LoadOutboxRow(row.OutboxID)
	if err != nil || !ok {
		t.Fatalf("LoadOutboxRow() = ok %v err %v, want true nil", ok, err)
	}
	if stored.LeaseOwner != first.LeaseOwner || !stored.LeasedUntil.Equal(first.LeasedUntil) {
		t.Fatalf("stored lease = %+v, want first lease unchanged", stored)
	}
}

func TestOutboxFailureAtMaxAttemptsMarksDead(t *testing.T) {
	store := newMemoryEconomyContractStore()
	now := time.Date(2026, 6, 25, 19, 0, 0, 0, time.UTC)
	row := validOutboxRow(t, "outbox-dead-after-max", now.Add(-time.Minute))
	row.AttemptCount = 1
	row.MaxAttempts = 2
	if err := store.InsertOutboxRow(row); err != nil {
		t.Fatalf("InsertOutboxRow() error = %v, want nil", err)
	}
	if _, ok, err := store.LeaseOutboxRow(OutboxLeaseInput{
		OutboxID:    row.OutboxID,
		LeaseOwner:  "publisher-1",
		Now:         now,
		LeasedUntil: now.Add(time.Minute),
	}); err != nil || !ok {
		t.Fatalf("LeaseOutboxRow() = ok %v err %v, want true nil", ok, err)
	}

	dead, ok, err := store.MarkOutboxFailed(OutboxFailureInput{
		OutboxID:    row.OutboxID,
		LeaseOwner:  "publisher-1",
		LastError:   "permanent failure",
		Now:         now.Add(10 * time.Second),
		AvailableAt: now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("MarkOutboxFailed(max attempts) error = %v, want nil", err)
	}
	if !ok || dead.Status != OutboxStatusDead || dead.AttemptCount != 2 {
		t.Fatalf("dead row = %+v ok %v, want dead after max attempts", dead, ok)
	}
	due, err := store.LoadDueOutboxRows(OutboxDueRowsQuery{Now: now.Add(time.Minute), Limit: 10})
	if err != nil {
		t.Fatalf("LoadDueOutboxRows(dead) error = %v, want nil", err)
	}
	if len(due) != 0 {
		t.Fatalf("due rows after dead = %+v, want none", due)
	}
}

func validIdempotencyKeyRow(t *testing.T) IdempotencyKeyRow {
	t.Helper()
	now := time.Date(2026, 6, 25, 15, 30, 0, 0, time.UTC)
	return IdempotencyKeyRow{
		Scope:       IdempotencyScopeEconomy,
		Key:         foundation.IdempotencyKey("loot_pickup:drop-conflict"),
		Operation:   "loot_pickup",
		PlayerID:    "player-1",
		RequestHash: "sha256:first",
		Status:      IdempotencyStatusInProgress,
		ResultJSON:  json.RawMessage(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func validOutboxRow(t *testing.T, outboxID string, availableAt time.Time) OutboxRow {
	t.Helper()
	row, err := NewOutboxRow(OutboxRow{
		OutboxID:         outboxID,
		Topic:            "economy",
		EventType:        EventWalletCredited,
		AggregateType:    "player",
		AggregateID:      "player-1",
		IdempotencyScope: IdempotencyScopeEconomy,
		IdempotencyKey:   validReferenceKey(t, "quest_reward:"+outboxID),
		PayloadJSON:      json.RawMessage(`{"player_id":"player-1","amount":25}`),
		AvailableAt:      availableAt,
		CreatedAt:        availableAt.Add(-time.Minute),
		UpdatedAt:        availableAt.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("NewOutboxRow(%q) error = %v, want nil", outboxID, err)
	}
	return row
}

type memoryEconomyContractStore struct {
	idempotency map[string]IdempotencyKeyRow
	outbox      map[string]OutboxRow
}

func newMemoryEconomyContractStore() *memoryEconomyContractStore {
	return &memoryEconomyContractStore{
		idempotency: make(map[string]IdempotencyKeyRow),
		outbox:      make(map[string]OutboxRow),
	}
}

func (store *memoryEconomyContractStore) ClaimIdempotencyKey(row IdempotencyKeyRow) (IdempotencyClaimResult, error) {
	key := idempotencyStoreKey(row.Scope, row.Key)
	if existing, ok := store.idempotency[key]; ok {
		return ResolveIdempotencyClaim(&existing, row)
	}
	result, err := ResolveIdempotencyClaim(nil, row)
	if err != nil {
		return IdempotencyClaimResult{}, err
	}
	store.idempotency[key] = result.Row.Clone()
	return result, nil
}

func (store *memoryEconomyContractStore) CompleteIdempotencyKey(row IdempotencyKeyRow) (IdempotencyKeyRow, error) {
	if err := row.Validate(); err != nil {
		return IdempotencyKeyRow{}, err
	}
	key := idempotencyStoreKey(row.Scope, row.Key)
	store.idempotency[key] = row.Clone()
	return row.Clone(), nil
}

func (store *memoryEconomyContractStore) InsertOutboxRow(row OutboxRow) error {
	inserted, err := NewOutboxRow(row)
	if err != nil {
		return err
	}
	if _, exists := store.outbox[inserted.OutboxID]; exists {
		return fmt.Errorf("outbox %q: %w", inserted.OutboxID, ErrInvalidOutboxRow)
	}
	store.outbox[inserted.OutboxID] = inserted.Clone()
	return nil
}

func (store *memoryEconomyContractStore) LoadOutboxRow(outboxID string) (OutboxRow, bool, error) {
	row, ok := store.outbox[outboxID]
	return row.Clone(), ok, nil
}

func (store *memoryEconomyContractStore) LoadDueOutboxRows(query OutboxDueRowsQuery) ([]OutboxRow, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	rows := make([]OutboxRow, 0, query.Limit)
	for _, row := range store.outbox {
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

func (store *memoryEconomyContractStore) LeaseOutboxRow(input OutboxLeaseInput) (OutboxRow, bool, error) {
	if err := input.Validate(); err != nil {
		return OutboxRow{}, false, err
	}
	row, ok := store.outbox[input.OutboxID]
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
	store.outbox[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func (store *memoryEconomyContractStore) MarkOutboxPublished(input OutboxPublishInput) (OutboxRow, bool, error) {
	if err := input.Validate(); err != nil {
		return OutboxRow{}, false, err
	}
	row, ok := store.outbox[input.OutboxID]
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
	store.outbox[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func (store *memoryEconomyContractStore) MarkOutboxFailed(input OutboxFailureInput) (OutboxRow, bool, error) {
	if err := input.Validate(); err != nil {
		return OutboxRow{}, false, err
	}
	row, ok := store.outbox[input.OutboxID]
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
	store.outbox[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func outboxLeaseEligible(row OutboxRow, now time.Time) bool {
	if (row.Status == OutboxStatusPending || row.Status == OutboxStatusFailed) && !row.AvailableAt.After(now) {
		return true
	}
	return row.Status == OutboxStatusLeased && !row.LeasedUntil.IsZero() && !row.LeasedUntil.After(now)
}

func outboxLeaseMatches(row OutboxRow, owner string, now time.Time) bool {
	return row.Status == OutboxStatusLeased &&
		row.LeaseOwner == owner &&
		!row.LeasedUntil.IsZero() &&
		row.LeasedUntil.After(now)
}

func idempotencyStoreKey(scope string, key foundation.IdempotencyKey) string {
	return scope + "\x00" + key.String()
}
