package loot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/world"
)

const (
	lootXPOutboxTopic         = "economy"
	lootXPOutboxAggregateType = "loot_drop"
	lootXPOutboxIDPrefix      = "loot_xp:"
)

// LootXPOutboxPayload is the durable replay payload for one server-owned loot
// pickup XP grant.
type LootXPOutboxPayload struct {
	DropID         world.EntityID               `json:"drop_id"`
	PlayerID       foundation.PlayerID          `json:"player_id"`
	Amount         int64                        `json:"amount"`
	SourceType     progression.XPSourceType     `json:"source_type"`
	SourceID       progression.XPSourceID       `json:"source_id"`
	IdempotencyKey progression.XPIdempotencyKey `json:"idempotency_key"`
}

// NewLootXPOutboxPublisher adapts the loot XP replay helper to the shared
// economy outbox worker publisher shape.
func NewLootXPOutboxPublisher(granter XPGranter) (economy.OutboxPublisher, error) {
	if granter == nil {
		return nil, ErrNilProgressionHook
	}
	return func(ctx context.Context, row economy.OutboxRow) error {
		_, err := ReplayLootXPOutboxRow(ctx, row, granter)
		return err
	}, nil
}

// ReplayLootXPOutboxRow grants loot XP from one durable outbox row. Replaying
// the same row is safe because the payload carries the canonical XP idempotency
// key consumed by progression.
func ReplayLootXPOutboxRow(
	ctx context.Context,
	row economy.OutboxRow,
	granter XPGranter,
) (progression.GrantXPResult, error) {
	if err := ctx.Err(); err != nil {
		return progression.GrantXPResult{}, err
	}
	if granter == nil {
		return progression.GrantXPResult{}, ErrNilProgressionHook
	}
	if row.Topic != lootXPOutboxTopic ||
		row.EventType != EventLootXPReconciliationRequested ||
		row.AggregateType != lootXPOutboxAggregateType {
		return progression.GrantXPResult{}, fmt.Errorf("row %q: %w", row.OutboxID, ErrInvalidLootXPOutbox)
	}

	payload, err := decodeLootXPOutboxPayload(row.PayloadJSON)
	if err != nil {
		return progression.GrantXPResult{}, err
	}
	return granter.GrantXP(progression.GrantXPInput{
		PlayerID:       payload.PlayerID,
		Amount:         payload.Amount,
		SourceType:     payload.SourceType,
		SourceID:       payload.SourceID,
		IdempotencyKey: payload.IdempotencyKey,
		Authority:      progression.XPGrantAuthorityLootService,
	})
}

func newLootXPOutboxRow(drop Drop, reconciliation LootXPReconciliation, now time.Time) (economy.OutboxRow, error) {
	if now.IsZero() {
		return economy.OutboxRow{}, fmt.Errorf("now: %w", ErrInvalidLootXPOutbox)
	}
	payload := LootXPOutboxPayload{
		DropID:         drop.ID,
		PlayerID:       reconciliation.PlayerID,
		Amount:         defaultLootXPAmount,
		SourceType:     reconciliation.SourceType,
		SourceID:       reconciliation.SourceID,
		IdempotencyKey: reconciliation.IdempotencyKey,
	}
	if err := payload.validate(); err != nil {
		return economy.OutboxRow{}, err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return economy.OutboxRow{}, err
	}
	return economy.NewOutboxRow(economy.OutboxRow{
		OutboxID:         lootXPOutboxID(reconciliation.IdempotencyKey),
		Topic:            lootXPOutboxTopic,
		EventType:        EventLootXPReconciliationRequested,
		AggregateType:    lootXPOutboxAggregateType,
		AggregateID:      drop.ID.String(),
		IdempotencyScope: economy.IdempotencyScopeEconomy,
		IdempotencyKey:   foundation.IdempotencyKey(reconciliation.IdempotencyKey.String()),
		PayloadJSON:      payloadJSON,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
}

func decodeLootXPOutboxPayload(raw json.RawMessage) (LootXPOutboxPayload, error) {
	var payload LootXPOutboxPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return LootXPOutboxPayload{}, fmt.Errorf("payload: %w", err)
	}
	if err := payload.validate(); err != nil {
		return LootXPOutboxPayload{}, err
	}
	return payload, nil
}

func (payload LootXPOutboxPayload) validate() error {
	if err := payload.DropID.Validate(); err != nil {
		return fmt.Errorf("drop_id: %w", err)
	}
	if err := payload.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if payload.Amount <= 0 {
		return fmt.Errorf("amount %d: %w", payload.Amount, ErrInvalidLootXPOutbox)
	}
	if payload.SourceType != progression.XPSourceTypeLoot {
		return fmt.Errorf("source_type %q: %w", payload.SourceType, ErrInvalidLootXPOutbox)
	}
	if err := payload.SourceID.Validate(); err != nil {
		return fmt.Errorf("source_id: %w", err)
	}
	if err := payload.IdempotencyKey.Validate(); err != nil {
		return fmt.Errorf("idempotency_key: %w", err)
	}
	if payload.SourceID.String() != payload.DropID.String() {
		return fmt.Errorf("source_id %q drop_id %q: %w", payload.SourceID, payload.DropID, ErrInvalidLootXPOutbox)
	}
	if payload.IdempotencyKey.String() != "loot_pickup:"+payload.DropID.String() {
		return fmt.Errorf("idempotency_key %q drop_id %q: %w", payload.IdempotencyKey, payload.DropID, ErrInvalidLootXPOutbox)
	}
	return nil
}

func lootXPOutboxID(key progression.XPIdempotencyKey) string {
	return lootXPOutboxIDPrefix + key.String()
}

func sameLootXPOutboxRow(left economy.OutboxRow, right economy.OutboxRow) bool {
	return left.Topic == right.Topic &&
		left.EventType == right.EventType &&
		left.AggregateType == right.AggregateType &&
		left.AggregateID == right.AggregateID &&
		left.IdempotencyScope == right.IdempotencyScope &&
		left.IdempotencyKey == right.IdempotencyKey &&
		string(left.PayloadJSON) == string(right.PayloadJSON)
}
