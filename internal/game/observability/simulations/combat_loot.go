package simulations

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

const (
	ReasonLootCreated economy.LedgerReason = "loot_created"
)

var (
	ErrInvalidSimulationConfig = errors.New("invalid simulation config")
)

// CombatLootSimulationConfig tunes the deterministic NPC-kill and loot-pickup
// simulation.
type CombatLootSimulationConfig struct {
	Kills                    int
	ConcurrentPickupAttempts int
	DropQuantity             int64
	StartTime                time.Time
	WorldID                  world.WorldID
	ZoneID                   world.ZoneID
}

// CombatLootSimulationSummary reports the deterministic simulation outcome.
type CombatLootSimulationSummary struct {
	KillsCompleted         int
	AttacksResolved        int
	DropsCreated           int
	DuplicateDeathAttempts int
	PickupAttempts         int
	PickupSuccesses        int
	PickupClaimedFailures  int
	CargoQuantity          int64
	FlowSnapshot           observability.EconomyFlowSnapshot
	MetricSnapshot         observability.MetricSnapshot
}

type normalizedCombatLootConfig struct {
	kills                    int
	concurrentPickupAttempts int
	dropQuantity             int64
	startTime                time.Time
	worldID                  world.WorldID
	zoneID                   world.ZoneID
}

// RunCombatLootSimulation runs deterministic NPC kills, retry-safe drop
// creation, and concurrent pickup attempts against the live domain services.
func RunCombatLootSimulation(config CombatLootSimulationConfig) (CombatLootSimulationSummary, error) {
	normalized, err := normalizeCombatLootConfig(config)
	if err != nil {
		return CombatLootSimulationSummary{}, err
	}

	clock := &simulationClock{now: normalized.startTime}
	metrics := observability.NewMetricRecorder()
	flows := observability.NewEconomyFlowAccumulator()

	combatService := combat.NewService(clock, nil)
	combatService.SetMetricRecorder(metrics)

	inventory := economy.NewInventoryService(clock)
	cargo := economy.NewCargoService(inventory)
	progressionService := progression.NewProgressionService(clock, nil)
	lootService, err := loot.NewService(loot.Config{
		Clock:       clock,
		Cargo:       cargo,
		Progression: progressionService,
	})
	if err != nil {
		return CombatLootSimulationSummary{}, err
	}
	lootService.SetMetricRecorder(metrics)

	itemDefinition, err := simulationRawOreDefinition()
	if err != nil {
		return CombatLootSimulationSummary{}, err
	}
	table, err := simulationLootTable(itemDefinition, normalized.dropQuantity)
	if err != nil {
		return CombatLootSimulationSummary{}, err
	}

	summary := CombatLootSimulationSummary{}
	for index := 0; index < normalized.kills; index++ {
		context, err := upsertCombatLootActors(combatService, normalized, index, clock.Now())
		if err != nil {
			return CombatLootSimulationSummary{}, err
		}

		attack, err := combatService.ExecuteBasicAttack(combat.BasicAttackInput{
			AttackerID: context.playerEntityID,
			TargetID:   context.npcEntityID,
		})
		if err != nil {
			return CombatLootSimulationSummary{}, err
		}
		summary.AttacksResolved++
		if !attack.Killed || attack.KillEvent == nil {
			return CombatLootSimulationSummary{}, fmt.Errorf("simulation kill %d did not produce NPC kill event: %w", index, ErrInvalidSimulationConfig)
		}
		summary.KillsCompleted++

		drops, err := lootService.CreateDropsForNPCKill(*attack.KillEvent, table)
		if err != nil {
			return CombatLootSimulationSummary{}, err
		}
		if len(drops.Drops) != 1 {
			return CombatLootSimulationSummary{}, fmt.Errorf("simulation kill %d drops = %d: %w", index, len(drops.Drops), ErrInvalidSimulationConfig)
		}
		drop := drops.Drops[0]
		summary.DropsCreated++
		if err := recordLootFaucet(flows, drop, clock.Now()); err != nil {
			return CombatLootSimulationSummary{}, err
		}

		duplicate, err := lootService.CreateDropsForNPCKill(*attack.KillEvent, table)
		if err != nil {
			return CombatLootSimulationSummary{}, err
		}
		if !duplicate.Duplicate || len(duplicate.Drops) != 1 || duplicate.Drops[0].ID != drop.ID {
			return CombatLootSimulationSummary{}, fmt.Errorf("simulation kill %d duplicate drop result = %+v: %w", index, duplicate, ErrInvalidSimulationConfig)
		}
		summary.DuplicateDeathAttempts++

		pickups, err := runConcurrentPickups(lootService, drop, context.activeCargo, normalized.concurrentPickupAttempts)
		if err != nil {
			return CombatLootSimulationSummary{}, err
		}
		summary.PickupAttempts += pickups.attempts
		summary.PickupSuccesses += pickups.successes
		summary.PickupClaimedFailures += pickups.claimedFailures
		summary.CargoQuantity += inventory.TotalItemQuantity(drop.OwnerPlayerID, drop.ItemDefinition.ItemID, context.activeCargo)

		clock.Advance(time.Second)
	}

	summary.FlowSnapshot = flows.Snapshot()
	summary.MetricSnapshot = metrics.Snapshot()
	return summary, nil
}

func normalizeCombatLootConfig(config CombatLootSimulationConfig) (normalizedCombatLootConfig, error) {
	normalized := normalizedCombatLootConfig{
		kills:                    config.Kills,
		concurrentPickupAttempts: config.ConcurrentPickupAttempts,
		dropQuantity:             config.DropQuantity,
		startTime:                config.StartTime,
		worldID:                  config.WorldID,
		zoneID:                   config.ZoneID,
	}
	if normalized.kills == 0 {
		normalized.kills = 1
	}
	if normalized.concurrentPickupAttempts == 0 {
		normalized.concurrentPickupAttempts = 2
	}
	if normalized.dropQuantity == 0 {
		normalized.dropQuantity = 1
	}
	if normalized.startTime.IsZero() {
		normalized.startTime = time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	}
	if normalized.worldID.IsZero() {
		normalized.worldID = "world_1"
	}
	if normalized.zoneID.IsZero() {
		normalized.zoneID = "zone_1"
	}

	if normalized.kills < 1 {
		return normalizedCombatLootConfig{}, fmt.Errorf("kills %d: %w", normalized.kills, ErrInvalidSimulationConfig)
	}
	if normalized.concurrentPickupAttempts < 1 {
		return normalizedCombatLootConfig{}, fmt.Errorf("concurrent pickup attempts %d: %w", normalized.concurrentPickupAttempts, ErrInvalidSimulationConfig)
	}
	if err := foundation.ValidatePositiveAmount(normalized.dropQuantity); err != nil {
		return normalizedCombatLootConfig{}, fmt.Errorf("drop quantity %d: %w", normalized.dropQuantity, ErrInvalidSimulationConfig)
	}
	if err := normalized.worldID.Validate(); err != nil {
		return normalizedCombatLootConfig{}, err
	}
	if err := normalized.zoneID.Validate(); err != nil {
		return normalizedCombatLootConfig{}, err
	}
	return normalized, nil
}

type simulationActorContext struct {
	playerEntityID world.EntityID
	npcEntityID    world.EntityID
	activeCargo    economy.ItemLocation
}

func upsertCombatLootActors(service *combat.Service, config normalizedCombatLootConfig, index int, now time.Time) (simulationActorContext, error) {
	playerID := foundation.PlayerID(fmt.Sprintf("player_%d", index+1))
	shipID := foundation.ShipID(fmt.Sprintf("ship_%d", index+1))
	playerEntityID := world.EntityID(fmt.Sprintf("player_entity_%d", index+1))
	npcEntityID := world.EntityID(fmt.Sprintf("npc_%d", index+1))
	baseX := float64(index * 1000)

	player, err := combat.NewActorFromSnapshot(combat.ActorFromSnapshotInput{
		EntityID:  playerEntityID,
		Type:      world.EntityTypePlayer,
		PlayerID:  playerID,
		WorldID:   config.worldID,
		ZoneID:    config.zoneID,
		Position:  world.Vec2{X: baseX, Y: 0},
		Signature: visibility.EntitySignature(1),
		Snapshot:  simulationPlayerStats(playerID, shipID, now),
	})
	if err != nil {
		return simulationActorContext{}, err
	}
	npc, err := combat.NewActorFromSnapshot(combat.ActorFromSnapshotInput{
		EntityID:  npcEntityID,
		Type:      world.EntityTypeNPCPlaceholder,
		NPCType:   "pirate",
		WorldID:   config.worldID,
		ZoneID:    config.zoneID,
		Position:  world.Vec2{X: baseX + 10, Y: 0},
		Signature: visibility.EntitySignature(1),
		Snapshot:  simulationNPCStats(foundation.ShipID(fmt.Sprintf("npc_ship_%d", index+1)), now),
	})
	if err != nil {
		return simulationActorContext{}, err
	}
	if err := service.UpsertActor(player); err != nil {
		return simulationActorContext{}, err
	}
	if err := service.UpsertActor(npc); err != nil {
		return simulationActorContext{}, err
	}
	activeCargo, err := economy.NewItemLocation(economy.LocationKindShipCargo, shipID.String())
	if err != nil {
		return simulationActorContext{}, err
	}
	return simulationActorContext{
		playerEntityID: playerEntityID,
		npcEntityID:    npcEntityID,
		activeCargo:    activeCargo,
	}, nil
}

func simulationPlayerStats(playerID foundation.PlayerID, shipID foundation.ShipID, now time.Time) stats.StatSnapshot {
	return stats.NewStatSnapshot(
		playerID,
		shipID,
		1,
		stats.EffectiveStats{
			Core: stats.CoreStats{
				HPMax:         100,
				ShieldMax:     20,
				EnergyMax:     100,
				EnergyRegen:   100,
				CargoCapacity: 10000,
			},
			Combat: stats.CombatStats{
				WeaponDamage:     30,
				WeaponRange:      100,
				WeaponCooldown:   1,
				WeaponEnergyCost: 1,
				Accuracy:         1,
			},
			Exploration: stats.ExplorationStats{
				RadarRange: 500,
			},
		},
		now,
	)
}

func simulationNPCStats(shipID foundation.ShipID, now time.Time) stats.StatSnapshot {
	return stats.NewStatSnapshot(
		"",
		shipID,
		1,
		stats.EffectiveStats{
			Core: stats.CoreStats{
				HPMax:     24,
				EnergyMax: 1,
			},
			Exploration: stats.ExplorationStats{
				RadarRange: 1,
			},
		},
		now,
	)
}

func simulationLootTable(itemDefinition economy.ItemDefinition, quantity int64) (loot.LootTable, error) {
	source, err := catalog.NewLootTableSource("simulation_loot_table", "v1")
	if err != nil {
		return loot.LootTable{}, err
	}
	table := loot.LootTable{
		Source: source,
		Rows: []loot.LootRow{{
			ItemDefinition: itemDefinition,
			MinQuantity:    quantity,
			MaxQuantity:    quantity,
			Chance:         1,
		}},
	}
	return table, nil
}

func simulationRawOreDefinition() (economy.ItemDefinition, error) {
	source, err := catalog.NewVersionedDefinitionFromStrings("raw_ore", "v1")
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	maxStack, err := foundation.NewQuantity(999999)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return economy.NewItemDefinition(
		source,
		"raw_ore",
		"Raw Ore",
		economy.ItemTypeStackable,
		economy.ItemRarityCommon,
		maxStack,
		weight,
		[]economy.TradeFlag{economy.TradeFlagDroppable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
}

func recordLootFaucet(accumulator *observability.EconomyFlowAccumulator, drop loot.Drop, now time.Time) error {
	reference, err := foundation.LootPickupIdempotencyKey(drop.ID.String())
	if err != nil {
		return err
	}
	entry, err := observability.NewItemFlowEntry(
		drop.ItemDefinition.ItemID,
		drop.Quantity,
		ReasonLootCreated,
		reference,
		observability.ValueFlowDirectionFaucet,
		now,
	)
	if err != nil {
		return err
	}
	return accumulator.Record(entry)
}

type pickupSimulationResult struct {
	attempts        int
	successes       int
	claimedFailures int
}

func runConcurrentPickups(service *loot.Service, drop loot.Drop, activeCargo economy.ItemLocation, attempts int) (pickupSimulationResult, error) {
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for attempt := 0; attempt < attempts; attempt++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.PickupDrop(loot.PickupInput{
				PlayerID:           drop.OwnerPlayerID,
				DropID:             drop.ID,
				Viewer:             simulationViewerForDrop(drop),
				ActiveCargo:        activeCargo,
				CargoCapacityUnits: 10000,
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	result := pickupSimulationResult{attempts: attempts}
	for err := range errs {
		if err == nil {
			result.successes++
			continue
		}
		if errors.Is(err, loot.ErrDropClaimed) {
			result.claimedFailures++
			continue
		}
		return pickupSimulationResult{}, err
	}
	return result, nil
}

func simulationViewerForDrop(drop loot.Drop) loot.Viewer {
	snapshot := stats.NewStatSnapshot(
		"simulation_viewer",
		"simulation_ship",
		1,
		stats.EffectiveStats{
			Exploration: stats.ExplorationStats{
				RadarRange: 500,
			},
		},
		drop.CreatedAt,
	)
	return loot.Viewer{
		WorldID:    drop.WorldID,
		ZoneID:     drop.ZoneID,
		Position:   drop.Position,
		RadarRange: visibility.RadarRangeFromStatSnapshot(snapshot),
	}
}

type simulationClock struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *simulationClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	return clock.now
}

func (clock *simulationClock) Advance(elapsed time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	clock.now = clock.now.Add(elapsed)
}
