package loot

import (
	"errors"
	"fmt"
	"math"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/world"
)

const (
	DefaultOwnerLockDuration = 60 * time.Second
	DefaultPublicDuration    = 120 * time.Second
	DefaultTotalLifetime     = 180 * time.Second
	DefaultPickupRange       = 64

	LedgerReasonLootPickup = economy.LedgerReason("loot_pickup")
)

var (
	ErrInvalidLootTable     = errors.New("invalid loot table")
	ErrInvalidLootRow       = errors.New("invalid loot row")
	ErrUnknownDrop          = errors.New("unknown loot drop")
	ErrDropOwnerLocked      = errors.New("loot drop owner locked")
	ErrDropExpired          = errors.New("loot drop expired")
	ErrDropClaimed          = errors.New("loot drop already claimed")
	ErrPickupOutOfRange     = errors.New("loot pickup out of range")
	ErrPickupNotVisible     = errors.New("loot pickup not visible")
	ErrNilCargoService      = errors.New("nil cargo service")
	ErrInvalidLootDurations = errors.New("invalid loot durations")
	ErrInvalidScheduledTask = errors.New("invalid loot scheduled task")
)

type DropSourceType string

const (
	DropSourceNPCDeath    DropSourceType = "npc_kill"
	DropSourcePlayerDeath DropSourceType = "player_death"
	DropSourceGatherNode  DropSourceType = "gather_node"
	DropSourceEventCache  DropSourceType = "event_cache"
	DropSourceSystemSpawn DropSourceType = "system_spawn"
)

type DropState string

const (
	DropStateOwnerLocked DropState = "owner_locked"
	DropStatePublic      DropState = "public"
	DropStateExpired     DropState = "expired"
	DropStateClaimed     DropState = "claimed"
)

type ScheduledDropTaskKind string

const (
	ScheduledDropTaskOwnerLockExpired ScheduledDropTaskKind = "loot.owner_lock_expired"
	ScheduledDropTaskDespawn          ScheduledDropTaskKind = "loot.drop_despawn"
)

// LootRow is one server-owned loot table roll.
type LootRow struct {
	ItemDefinition economy.ItemDefinition
	MinQuantity    int64
	MaxQuantity    int64
	Chance         float64
}

// LootTable is the deterministic roll input for a source event.
type LootTable struct {
	Source catalog.VersionedDefinition
	Rows   []LootRow
}

// Drop is a world loot entity owned by the loot service.
type Drop struct {
	ID       world.EntityID
	WorldID  world.WorldID
	ZoneID   world.ZoneID
	Position world.Vec2

	ItemDefinition economy.ItemDefinition
	Quantity       int64

	OwnerPlayerID  foundation.PlayerID
	OwnerLockUntil time.Time
	PublicUntil    time.Time
	ExpiresAt      time.Time
	CreatedAt      time.Time

	SourceType DropSourceType
	SourceID   world.EntityID

	ClaimedBy foundation.PlayerID
	ClaimedAt *time.Time
}

// DropItem is one server-calculated world drop item.
type DropItem struct {
	ItemDefinition economy.ItemDefinition
	Quantity       int64
}

// CreatePlayerDeathDropsInput describes DeathService-owned cargo loss already
// resolved into concrete item stacks. Loot only creates world drops.
type CreatePlayerDeathDropsInput struct {
	SourceID      world.EntityID
	DeadPlayerID  foundation.PlayerID
	OwnerPlayerID foundation.PlayerID
	WorldID       world.WorldID
	ZoneID        world.ZoneID
	Position      world.Vec2
	Items         []DropItem
}

// DropPayload is the client-safe drop shape after visibility filtering.
type DropPayload struct {
	ID        world.EntityID    `json:"drop_id"`
	Position  world.Vec2        `json:"position"`
	ItemID    foundation.ItemID `json:"item_id"`
	Quantity  int64             `json:"quantity"`
	State     DropState         `json:"state"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// ScheduledDropTask is the loot-owned delayed work contract consumed by a zone
// worker scheduler. The worker owns timing; the loot service owns effects.
type ScheduledDropTask struct {
	ID     string
	Kind   ScheduledDropTaskKind
	DropID world.EntityID
	DueAt  time.Time
}

// ScheduledDropTaskResult reports the effect of one due scheduled task.
type ScheduledDropTaskResult struct {
	Drop    Drop
	Handled bool
}

// CreateDropsResult reports drop creation for one source event.
type CreateDropsResult struct {
	Drops          []Drop
	ScheduledTasks []ScheduledDropTask
	Duplicate      bool
}

// PickupInput describes one server-authoritative pickup attempt.
type PickupInput struct {
	PlayerID           foundation.PlayerID
	DropID             world.EntityID
	Viewer             Viewer
	ActiveCargo        economy.ItemLocation
	CargoCapacityUnits int64
}

// PickupResult reports a successful pickup mutation.
type PickupResult struct {
	Drop        Drop
	CargoResult economy.AddItemResult
	XPResult    *progression.GrantXPResult
	// XPError is non-fatal: pickup/cargo/claim succeeded, but the optional XP
	// grant hook failed and should be reconciled by a later durable reward flow.
	XPError error
}

func (item DropItem) validate() error {
	if err := item.ItemDefinition.Validate(); err != nil {
		return err
	}
	if item.Quantity <= 0 {
		return fmt.Errorf("quantity %d: %w", item.Quantity, ErrInvalidLootRow)
	}
	return nil
}

func (input CreatePlayerDeathDropsInput) validate() error {
	if err := input.SourceID.Validate(); err != nil {
		return err
	}
	if err := input.DeadPlayerID.Validate(); err != nil {
		return err
	}
	if !input.OwnerPlayerID.IsZero() {
		if err := input.OwnerPlayerID.Validate(); err != nil {
			return err
		}
	}
	if err := input.WorldID.Validate(); err != nil {
		return err
	}
	if err := input.ZoneID.Validate(); err != nil {
		return err
	}
	if err := input.Position.Validate(); err != nil {
		return err
	}
	if len(input.Items) == 0 {
		return ErrInvalidLootTable
	}
	for _, item := range input.Items {
		if err := item.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (table LootTable) validate() error {
	if err := table.Source.Validate(); err != nil {
		return err
	}
	if len(table.Rows) == 0 {
		return ErrInvalidLootTable
	}
	for _, row := range table.Rows {
		if err := row.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (row LootRow) validate() error {
	if err := row.ItemDefinition.Validate(); err != nil {
		return err
	}
	if row.MinQuantity <= 0 || row.MaxQuantity < row.MinQuantity {
		return fmt.Errorf("quantity %d..%d: %w", row.MinQuantity, row.MaxQuantity, ErrInvalidLootRow)
	}
	if math.IsNaN(row.Chance) || math.IsInf(row.Chance, 0) || row.Chance < 0 || row.Chance > 1 {
		return fmt.Errorf("chance %v: %w", row.Chance, ErrInvalidLootRow)
	}
	return nil
}

func (sourceType DropSourceType) eligibleForLootXP() bool {
	switch sourceType {
	case DropSourceNPCDeath, DropSourceGatherNode, DropSourceEventCache:
		return true
	default:
		return false
	}
}

func (drop Drop) State(now time.Time) DropState {
	if drop.ClaimedAt != nil {
		return DropStateClaimed
	}
	if !now.Before(drop.ExpiresAt) {
		return DropStateExpired
	}
	if now.Before(drop.OwnerLockUntil) {
		return DropStateOwnerLocked
	}
	return DropStatePublic
}

func cloneDrop(drop Drop) Drop {
	clone := drop
	if drop.ClaimedAt != nil {
		claimedAt := *drop.ClaimedAt
		clone.ClaimedAt = &claimedAt
	}
	return clone
}

func cloneDrops(drops []Drop) []Drop {
	cloned := make([]Drop, len(drops))
	for index, drop := range drops {
		cloned[index] = cloneDrop(drop)
	}
	return cloned
}
