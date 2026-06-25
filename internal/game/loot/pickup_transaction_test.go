package loot_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
)

func TestPickupDropTransactionOutboxFailureRollsBackClaimAndCargo(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 25, 23, 30, 0, 0, time.UTC))
	inventory := economy.NewInventoryService(clock)
	cargo := economy.NewCargoService(inventory)
	repository := newFakeLootPickupTransactionRepository()
	repository.failOutboxWith = errors.New("injected loot outbox failure")
	service, err := loot.NewService(loot.Config{
		Clock:              clock,
		RNG:                testutil.NewFakeRNG([]int{0}, []float64{0}),
		Cargo:              cargo,
		Progression:        progression.NewProgressionService(clock, nil),
		PickupTransactions: repository,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	drop := createOneDrop(t, service)
	cargoLocation := mustCargoLocation(t, "ship_1")

	_, err = service.PickupDrop(loot.PickupInput{
		PlayerID:           drop.OwnerPlayerID,
		DropID:             drop.ID,
		Viewer:             viewerAt(drop.Position),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 100,
	})
	if !errors.Is(err, repository.failOutboxWith) {
		t.Fatalf("PickupDrop() error = %v, want injected outbox failure", err)
	}

	stored, ok := service.Drop(drop.ID)
	if !ok || stored.ClaimedAt != nil || stored.ClaimedBy != "" {
		t.Fatalf("drop after rollback = %+v ok %v, want unclaimed", stored, ok)
	}
	if got := inventory.TotalItemQuantity(drop.OwnerPlayerID, rawOreDefinition(t).ItemID, cargoLocation); got != 0 {
		t.Fatalf("cargo quantity after rollback = %d, want 0", got)
	}
	if got := repository.inventoryCommitCount(); got != 0 {
		t.Fatalf("inventory commits after rollback = %d, want 0", got)
	}
	if got := repository.outboxCount(); got != 0 {
		t.Fatalf("outbox rows after rollback = %d, want 0", got)
	}
	if got := repository.claimCount(); got != 0 {
		t.Fatalf("claim rows after rollback = %d, want 0", got)
	}
}

type fakeLootPickupTransactionRepository struct {
	mu               sync.Mutex
	claims           map[world.EntityID]loot.Drop
	inventoryCommits []economy.InventoryAddItemCommit
	outboxRows       map[string]economy.OutboxRow
	failOutboxWith   error
}

func newFakeLootPickupTransactionRepository() *fakeLootPickupTransactionRepository {
	return &fakeLootPickupTransactionRepository{
		claims:     make(map[world.EntityID]loot.Drop),
		outboxRows: make(map[string]economy.OutboxRow),
	}
}

func (repository *fakeLootPickupTransactionRepository) WithLootPickupTransaction(
	_ context.Context,
	fn func(loot.LootPickupTransaction) error,
) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	tx := &fakeLootPickupTransaction{
		repository:       repository,
		claims:           cloneLootClaimRows(repository.claims),
		inventoryCommits: append([]economy.InventoryAddItemCommit(nil), repository.inventoryCommits...),
		outboxRows:       cloneLootOutboxRows(repository.outboxRows),
		failOutboxWith:   repository.failOutboxWith,
	}
	if err := fn(tx); err != nil {
		return err
	}
	repository.claims = cloneLootClaimRows(tx.claims)
	repository.inventoryCommits = append([]economy.InventoryAddItemCommit(nil), tx.inventoryCommits...)
	repository.outboxRows = cloneLootOutboxRows(tx.outboxRows)
	return nil
}

func (repository *fakeLootPickupTransactionRepository) inventoryCommitCount() int {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return len(repository.inventoryCommits)
}

func (repository *fakeLootPickupTransactionRepository) outboxCount() int {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return len(repository.outboxRows)
}

func (repository *fakeLootPickupTransactionRepository) claimCount() int {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return len(repository.claims)
}

type fakeLootPickupTransaction struct {
	repository       *fakeLootPickupTransactionRepository
	claims           map[world.EntityID]loot.Drop
	inventoryCommits []economy.InventoryAddItemCommit
	outboxRows       map[string]economy.OutboxRow
	failOutboxWith   error
}

func (tx *fakeLootPickupTransaction) SaveLootDropClaim(_ context.Context, drop loot.Drop) error {
	if drop.ClaimedAt == nil || drop.ClaimedBy.IsZero() {
		return fmt.Errorf("drop %q: missing claim", drop.ID)
	}
	if _, exists := tx.claims[drop.ID]; exists {
		return fmt.Errorf("drop %q: duplicate claim", drop.ID)
	}
	tx.claims[drop.ID] = drop
	return nil
}

func (tx *fakeLootPickupTransaction) CommitInventoryAddItem(_ context.Context, commit economy.InventoryAddItemCommit) error {
	if err := commit.Validate(); err != nil {
		return err
	}
	tx.inventoryCommits = append(tx.inventoryCommits, commit)
	return nil
}

func (tx *fakeLootPickupTransaction) InsertOutboxRow(_ context.Context, row economy.OutboxRow) error {
	if tx.failOutboxWith != nil {
		return tx.failOutboxWith
	}
	inserted, err := economy.NewOutboxRow(row)
	if err != nil {
		return err
	}
	if _, exists := tx.outboxRows[inserted.OutboxID]; exists {
		return fmt.Errorf("outbox %q: %w", inserted.OutboxID, economy.ErrInvalidOutboxRow)
	}
	tx.outboxRows[inserted.OutboxID] = inserted.Clone()
	return nil
}

func cloneLootClaimRows(rows map[world.EntityID]loot.Drop) map[world.EntityID]loot.Drop {
	cloned := make(map[world.EntityID]loot.Drop, len(rows))
	for key, row := range rows {
		cloned[key] = row
	}
	return cloned
}

func cloneLootOutboxRows(rows map[string]economy.OutboxRow) map[string]economy.OutboxRow {
	cloned := make(map[string]economy.OutboxRow, len(rows))
	for key, row := range rows {
		cloned[key] = row.Clone()
	}
	return cloned
}

var _ loot.LootPickupTransactionRepository = (*fakeLootPickupTransactionRepository)(nil)
var _ loot.LootPickupTransaction = (*fakeLootPickupTransaction)(nil)
var _ foundation.Clock = (*testutil.FakeClock)(nil)
