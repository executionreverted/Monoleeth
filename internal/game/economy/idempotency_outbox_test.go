package economy

import (
	"encoding/json"
	"errors"
	"fmt"
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

func idempotencyStoreKey(scope string, key foundation.IdempotencyKey) string {
	return scope + "\x00" + key.String()
}
