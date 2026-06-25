package contentdb_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestPostgresEconomyIdempotencyDuplicateClaimReturnsCompletedRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)
	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}

	row := postgresIdempotencyRow(t, "loot_pickup:postgres_duplicate_claim")
	if _, err := store.ClaimIdempotencyKey(ctx, row); err != nil {
		t.Fatalf("ClaimIdempotencyKey(first) error = %v, want nil", err)
	}
	completedAt := row.UpdatedAt.Add(time.Minute)
	completed := row.Clone()
	completed.Status = economy.IdempotencyStatusCompleted
	completed.ResultJSON = json.RawMessage(`{"ledger_id":"ledger-postgres-duplicate"}`)
	completed.UpdatedAt = completedAt
	completed.CompletedAt = completedAt
	if _, err := store.CompleteIdempotencyKey(ctx, completed); err != nil {
		t.Fatalf("CompleteIdempotencyKey() error = %v, want nil", err)
	}

	candidate := row.Clone()
	candidate.ResultJSON = json.RawMessage(`{"ledger_id":"candidate"}`)
	result, err := store.ClaimIdempotencyKey(ctx, candidate)
	if err != nil {
		t.Fatalf("ClaimIdempotencyKey(duplicate) error = %v, want nil", err)
	}
	if !result.Duplicate || result.Row.Status != economy.IdempotencyStatusCompleted || !jsonEqual(result.Row.ResultJSON, completed.ResultJSON) {
		t.Fatalf("duplicate result = %+v, want completed existing row", result)
	}
}

func TestPostgresEconomyIdempotencyConflictingClaimReturnsConflict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)
	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}

	cases := []struct {
		name   string
		mutate func(*economy.IdempotencyKeyRow)
	}{
		{
			name:   "request hash",
			mutate: func(row *economy.IdempotencyKeyRow) { row.RequestHash = "sha256:other" },
		},
		{
			name:   "operation",
			mutate: func(row *economy.IdempotencyKeyRow) { row.Operation = "market_buy" },
		},
		{
			name: "player",
			mutate: func(row *economy.IdempotencyKeyRow) {
				row.PlayerID = foundation.PlayerID("player-postgres-conflict-other")
			},
		},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := postgresIdempotencyRow(t, "loot_pickup:postgres_conflict_"+string(rune('a'+index)))
			if _, err := store.ClaimIdempotencyKey(ctx, row); err != nil {
				t.Fatalf("ClaimIdempotencyKey(first) error = %v, want nil", err)
			}
			candidate := row.Clone()
			tc.mutate(&candidate)

			_, err := store.ClaimIdempotencyKey(ctx, candidate)

			if !errors.Is(err, economy.ErrIdempotencyKeyConflict) {
				t.Fatalf("ClaimIdempotencyKey(conflict) error = %v, want ErrIdempotencyKeyConflict", err)
			}
		})
	}
}

func TestPostgresEconomyIdempotencyCompleteStoresResultStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)
	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}

	completed := postgresIdempotencyRow(t, "loot_pickup:postgres_complete_store")
	completedAt := completed.UpdatedAt.Add(2 * time.Minute)
	completed.Status = economy.IdempotencyStatusCompleted
	completed.ResultJSON = json.RawMessage(`{"status":"applied","wallet_ledger":"ledger-postgres-complete"}`)
	completed.UpdatedAt = completedAt
	completed.CompletedAt = completedAt
	stored, err := store.CompleteIdempotencyKey(ctx, completed)
	if err != nil {
		t.Fatalf("CompleteIdempotencyKey(insert) error = %v, want nil", err)
	}
	if stored.Status != economy.IdempotencyStatusCompleted || !jsonEqual(stored.ResultJSON, completed.ResultJSON) || stored.CompletedAt.IsZero() {
		t.Fatalf("stored row = %+v, want completed result/status", stored)
	}
}

func TestPostgresEconomyOutboxInsertLoadRoundTripsJSONAndStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)
	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}

	now := time.Date(2026, 6, 25, 18, 0, 0, 0, time.UTC)
	inserted, err := economy.NewOutboxRow(economy.OutboxRow{
		OutboxID:         "outbox-postgres-roundtrip",
		Topic:            "economy",
		EventType:        economy.EventWalletCredited,
		AggregateType:    "player",
		AggregateID:      "player-postgres-outbox",
		IdempotencyScope: economy.IdempotencyScopeEconomy,
		IdempotencyKey:   postgresIdempotencyKey(t, "quest_reward:quest-postgres-outbox"),
		PayloadJSON:      json.RawMessage(`{"amount":25,"player_id":"player-postgres-outbox"}`),
		Status:           economy.OutboxStatusFailed,
		AvailableAt:      now.Add(time.Minute),
		AttemptCount:     2,
		MaxAttempts:      5,
		LastError:        "publish timeout",
		CreatedAt:        now,
		UpdatedAt:        now.Add(30 * time.Second),
	})
	if err != nil {
		t.Fatalf("NewOutboxRow() error = %v, want nil", err)
	}
	if err := store.InsertOutboxRow(ctx, inserted); err != nil {
		t.Fatalf("InsertOutboxRow() error = %v, want nil", err)
	}

	loaded, ok, err := store.LoadOutboxRow(ctx, inserted.OutboxID)
	if err != nil {
		t.Fatalf("LoadOutboxRow() error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("LoadOutboxRow(%q) ok = false, want true", inserted.OutboxID)
	}
	if loaded.Status != inserted.Status ||
		loaded.AttemptCount != inserted.AttemptCount ||
		loaded.MaxAttempts != inserted.MaxAttempts ||
		loaded.LastError != inserted.LastError ||
		loaded.IdempotencyKey != inserted.IdempotencyKey ||
		!jsonEqual(loaded.PayloadJSON, inserted.PayloadJSON) {
		t.Fatalf("loaded outbox row = %+v, want inserted JSON/status fields", loaded)
	}
}

func postgresIdempotencyRow(t *testing.T, key string) economy.IdempotencyKeyRow {
	t.Helper()
	now := time.Date(2026, 6, 25, 17, 0, 0, 0, time.UTC)
	return economy.IdempotencyKeyRow{
		Scope:       economy.IdempotencyScopeEconomy,
		Key:         postgresIdempotencyKey(t, key),
		Operation:   "loot_pickup",
		PlayerID:    foundation.PlayerID("player-postgres-idempotency"),
		RequestHash: "sha256:postgres",
		Status:      economy.IdempotencyStatusInProgress,
		ResultJSON:  json.RawMessage(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func postgresIdempotencyKey(t *testing.T, key string) foundation.IdempotencyKey {
	t.Helper()
	parsed, err := foundation.ParseIdempotencyKey(key)
	if err != nil {
		t.Fatalf("ParseIdempotencyKey(%q) error = %v, want nil", key, err)
	}
	return parsed
}

func jsonEqual(left json.RawMessage, right json.RawMessage) bool {
	var leftValue any
	var rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	return jsonValuesEqual(leftValue, rightValue)
}

func jsonValuesEqual(left any, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}
