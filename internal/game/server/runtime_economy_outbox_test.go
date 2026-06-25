package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
)

// fakeEconomyOutboxStore is a minimal in-memory economy.OutboxStore used to
// drive the runtime economy outbox drain without a live content database.
type fakeEconomyOutboxStore struct {
	rows map[string]economy.OutboxRow
}

func newFakeEconomyOutboxStore(rows ...economy.OutboxRow) *fakeEconomyOutboxStore {
	store := &fakeEconomyOutboxStore{rows: make(map[string]economy.OutboxRow, len(rows))}
	for _, row := range rows {
		store.rows[row.OutboxID] = row.Clone()
	}
	return store
}

func (store *fakeEconomyOutboxStore) InsertOutboxRow(_ context.Context, row economy.OutboxRow) error {
	inserted, err := economy.NewOutboxRow(row)
	if err != nil {
		return err
	}
	if _, exists := store.rows[inserted.OutboxID]; exists {
		return fmt.Errorf("outbox %q: %w", inserted.OutboxID, economy.ErrInvalidOutboxRow)
	}
	store.rows[inserted.OutboxID] = inserted.Clone()
	return nil
}

func (store *fakeEconomyOutboxStore) LoadOutboxRow(_ context.Context, outboxID string) (economy.OutboxRow, bool, error) {
	row, ok := store.rows[outboxID]
	return row.Clone(), ok, nil
}

func (store *fakeEconomyOutboxStore) LoadDueOutboxRows(ctx context.Context, query economy.OutboxDueRowsQuery) ([]economy.OutboxRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}
	rows := make([]economy.OutboxRow, 0, query.Limit)
	for _, row := range store.rows {
		if row.Status != economy.OutboxStatusPending && row.Status != economy.OutboxStatusFailed {
			continue
		}
		if row.AvailableAt.After(query.Now) {
			continue
		}
		rows = append(rows, row.Clone())
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].OutboxID < rows[j].OutboxID })
	if len(rows) > query.Limit {
		rows = rows[:query.Limit]
	}
	return rows, nil
}

func (store *fakeEconomyOutboxStore) LeaseOutboxRow(ctx context.Context, input economy.OutboxLeaseInput) (economy.OutboxRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	row, ok := store.rows[input.OutboxID]
	if !ok || (row.Status != economy.OutboxStatusPending && row.Status != economy.OutboxStatusFailed) {
		return economy.OutboxRow{}, false, nil
	}
	row.Status = economy.OutboxStatusLeased
	row.LeaseOwner = input.LeaseOwner
	row.LeasedUntil = input.LeasedUntil
	row.UpdatedAt = input.Now
	store.rows[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func (store *fakeEconomyOutboxStore) MarkOutboxPublished(ctx context.Context, input economy.OutboxPublishInput) (economy.OutboxRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	row, ok := store.rows[input.OutboxID]
	if !ok || row.Status != economy.OutboxStatusLeased || row.LeaseOwner != input.LeaseOwner {
		return economy.OutboxRow{}, false, nil
	}
	row.Status = economy.OutboxStatusPublished
	row.LeaseOwner = ""
	row.LeasedUntil = time.Time{}
	row.AttemptCount++
	row.LastError = ""
	row.UpdatedAt = input.Now
	row.PublishedAt = input.Now
	store.rows[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func (store *fakeEconomyOutboxStore) MarkOutboxFailed(ctx context.Context, input economy.OutboxFailureInput) (economy.OutboxRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	row, ok := store.rows[input.OutboxID]
	if !ok || row.Status != economy.OutboxStatusLeased || row.LeaseOwner != input.LeaseOwner {
		return economy.OutboxRow{}, false, nil
	}
	row.AttemptCount++
	if row.AttemptCount >= row.MaxAttempts {
		row.Status = economy.OutboxStatusDead
	} else {
		row.Status = economy.OutboxStatusFailed
	}
	row.AvailableAt = input.AvailableAt
	row.LeaseOwner = ""
	row.LeasedUntil = time.Time{}
	row.LastError = input.LastError
	row.UpdatedAt = input.Now
	store.rows[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func newEconomyOutboxDrainTestServer(t *testing.T) (*Server, auth.ResolvedSession) {
	t.Helper()
	clock := testutil.NewFakeClock(time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC))
	gameServer := newRouteControlTestServer(t, clock)
	owner := createResolvedRuntimeSession(t, gameServer, "economy-outbox@example.com", "Economy Outbox")
	return gameServer, owner
}

func marketCancelEconomyOutboxRow(t *testing.T, sellerID foundation.PlayerID, now time.Time) economy.OutboxRow {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"seller_player_id": sellerID.String(),
	})
	if err != nil {
		t.Fatalf("marshal market cancel payload: %v", err)
	}
	row, err := economy.NewOutboxRow(economy.OutboxRow{
		OutboxID:         "market_cancel:economy-outbox-smoke",
		Topic:            "economy",
		EventType:        "market.listing_cancelled",
		AggregateType:    "market_listing",
		AggregateID:      "economy-outbox-smoke-listing",
		IdempotencyScope: economy.IdempotencyScopeEconomy,
		PayloadJSON:      payload,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("NewOutboxRow error = %v", err)
	}
	return row
}

// TestRuntimeEconomyOutboxReplayRedeliversOnceWithoutDuplicatingState proves a
// committed economy outbox row projects a wallet+inventory snapshot refresh to
// the affected player exactly once, and a second drain does not redeliver or
// double-apply (the worker marks the row published after the first projection).
func TestRuntimeEconomyOutboxReplayRedeliversOnceWithoutDuplicatingState(t *testing.T) {
	gameServer, owner := newEconomyOutboxDrainTestServer(t)
	runtime := gameServer.runtime
	now := runtime.clock.Now().UTC()

	row := marketCancelEconomyOutboxRow(t, owner.PlayerID, now)
	runtime.economyOutbox = newFakeEconomyOutboxStore(row)
	clearQueuedRuntimeEventsForTest(t, runtime)

	first, err := runtime.drainEconomyOutboxToRealtime(now)
	if err != nil {
		t.Fatalf("first drainEconomyOutboxToRealtime error = %v", err)
	}
	if first.Published != 1 {
		t.Fatalf("first drain published = %d, want 1", first.Published)
	}
	firstEvents := runtime.drainQueuedRealtimeEvents()
	if got := countEconomySnapshotEventsForSession(firstEvents, owner.SessionID); got != 2 {
		t.Fatalf("first drain snapshot events = %d, want wallet+inventory = 2", got)
	}

	published, ok, err := runtime.economyOutbox.LoadOutboxRow(context.Background(), row.OutboxID)
	if err != nil || !ok || published.Status != economy.OutboxStatusPublished {
		t.Fatalf("row after first drain = %+v ok %v err %v, want published", published, ok, err)
	}

	clearQueuedRuntimeEventsForTest(t, runtime)
	second, err := runtime.drainEconomyOutboxToRealtime(now.Add(time.Second))
	if err != nil {
		t.Fatalf("second drainEconomyOutboxToRealtime error = %v", err)
	}
	if second.Published != 0 {
		t.Fatalf("second drain published = %d, want 0 (no duplicate redelivery)", second.Published)
	}
	secondEvents := runtime.drainQueuedRealtimeEvents()
	if got := countEconomySnapshotEventsForSession(secondEvents, owner.SessionID); got != 0 {
		t.Fatalf("second drain snapshot events = %d, want 0 (no duplicate redelivery)", got)
	}
}

func countEconomySnapshotEventsForSession(eventsBySession map[auth.SessionID][]realtime.EventEnvelope, sessionID auth.SessionID) int {
	count := 0
	for _, event := range eventsBySession[sessionID] {
		if event.Type == realtime.EventWalletSnapshot || event.Type == realtime.EventInventorySnapshot {
			count++
		}
	}
	return count
}

// TestEconomyOutboxAffectedPlayersDecodesEachEventType guards the payload
// decode so the publisher keeps fanning out to every committed economy event
// family even as their owning services evolve their payloads.
func TestEconomyOutboxAffectedPlayersDecodesEachEventType(t *testing.T) {
	player := foundation.PlayerID("player-economy-outbox-a")
	player2 := foundation.PlayerID("player-economy-outbox-b")

	tests := []struct {
		name      string
		eventType string
		payload   map[string]any
		want      []string
	}{
		{
			name:      "market buy",
			eventType: "market.buy_completed",
			payload:   map[string]any{"buyer_player_id": player.String(), "seller_player_id": player2.String()},
			want:      []string{player.String(), player2.String()},
		},
		{
			name:      "market cancel",
			eventType: "market.listing_cancelled",
			payload:   map[string]any{"seller_player_id": player.String()},
			want:      []string{player.String()},
		},
		{
			name:      "auction bid",
			eventType: "auction.bid_placed",
			payload:   map[string]any{"bidder_player_id": player.String(), "current_bidder_id": player2.String()},
			want:      []string{player.String(), player2.String()},
		},
		{
			name:      "auction buy now",
			eventType: "auction.buy_now",
			payload:   map[string]any{"buyer_player_id": player.String(), "winning_player_id": player2.String()},
			want:      []string{player.String(), player2.String()},
		},
		{
			name:      "premium created",
			eventType: "premium.entitlement_created",
			payload:   map[string]any{"player_id": player.String()},
			want:      []string{player.String()},
		},
		{
			name:      "premium claimed",
			eventType: "premium.entitlement_claimed",
			payload:   map[string]any{"entitlement": map[string]any{"player_id": player.String()}},
			want:      []string{player.String()},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(tc.payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			got, err := economyOutboxAffectedPlayers(economy.OutboxRow{EventType: tc.eventType, PayloadJSON: payload})
			if err != nil {
				t.Fatalf("economyOutboxAffectedPlayers error = %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("affected players = %v, want %v", got, tc.want)
			}
			seen := make(map[string]bool, len(got))
			for _, id := range got {
				seen[id.String()] = true
			}
			for _, want := range tc.want {
				if !seen[want] {
					t.Fatalf("affected players = %v, missing %q", got, want)
				}
			}
		})
	}

	t.Run("unknown event errors", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]any{"player_id": player.String()})
		if _, err := economyOutboxAffectedPlayers(economy.OutboxRow{EventType: "unknown.event", PayloadJSON: payload}); !errors.Is(err, errUnknownEconomyOutboxEvent) {
			t.Fatalf("unknown event error = %v, want errUnknownEconomyOutboxEvent", err)
		}
	})
}
