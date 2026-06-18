package runtime

import (
	"errors"
	"math"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/testutil"
)

func TestProgressionProviderExposesRankAndRoleLevels(t *testing.T) {
	service := progression.NewProgressionService(testutil.NewFakeClock(time.Date(2026, 6, 17, 15, 0, 0, 0, time.UTC)), nil)

	if _, err := service.GrantXP(progression.GrantXPInput{
		PlayerID:       "player-1",
		Amount:         100,
		SourceType:     progression.XPSourceTypeCombat,
		SourceID:       "npc-kill-1",
		IdempotencyKey: "xp-npc-kill-1",
		Authority:      progression.XPGrantAuthorityCombatService,
		RoleXP:         []progression.RoleXPGrant{{Role: progression.RoleTypeCombat, Amount: 75}},
	}); err != nil {
		t.Fatalf("GrantXP() error = %v, want nil", err)
	}
	if _, err := service.TryRankUp(progression.TryRankUpInput{
		PlayerID:       "player-1",
		TargetRank:     2,
		Reason:         "main_level_2",
		IdempotencyKey: "rank-up-player-1-rank-2",
	}); err != nil {
		t.Fatalf("TryRankUp() error = %v, want nil", err)
	}

	provider, err := NewProgressionProvider(service)
	if err != nil {
		t.Fatalf("NewProgressionProvider() error = %v, want nil", err)
	}

	rank, err := provider.RankForPlayer("player-1")
	if err != nil {
		t.Fatalf("RankForPlayer() error = %v, want nil", err)
	}
	if rank != 2 {
		t.Fatalf("RankForPlayer() = %d, want 2", rank)
	}

	pilotProgression, err := provider.ProgressionForPlayer("player-1")
	if err != nil {
		t.Fatalf("ProgressionForPlayer() error = %v, want nil", err)
	}
	if pilotProgression.Rank != 2 {
		t.Fatalf("PilotProgression.Rank = %d, want 2", pilotProgression.Rank)
	}
	if got := pilotProgression.RoleLevels[modules.PilotRoleCombat]; got != 2 {
		t.Fatalf("combat role level = %d, want 2", got)
	}
}

func TestStatInputProviderBuildsShipAndEquippedModuleStats(t *testing.T) {
	shipCatalog := mustShipCatalog(t)
	moduleCatalog := modules.MustMVPCatalog()
	loadout := modules.NewInMemoryLoadoutStore()
	playerID := foundation.PlayerID("player-1")
	shipID := ships.ShipIDStarter

	putRuntimeModuleItem(t, loadout, "laser-instance-1", "laser_alpha_t1", playerID, 100)
	if err := loadout.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID: playerID,
		ShipID:   shipID,
		Equipped: []modules.EquippedModule{{
			PlayerID:       playerID,
			ShipID:         shipID,
			SlotID:         modules.ModuleSlotOffensive1,
			ItemInstanceID: "laser-instance-1",
			EquippedAt:     time.Date(2026, 6, 17, 16, 0, 0, 0, time.UTC),
		}},
	}); err != nil {
		t.Fatalf("ReplaceEquippedModules() error = %v, want nil", err)
	}

	provider, err := NewStatInputProvider(shipCatalog, moduleCatalog, loadout)
	if err != nil {
		t.Fatalf("NewStatInputProvider() error = %v, want nil", err)
	}

	input, err := provider.BuildStatsInput(stats.NewStatSubject(playerID, shipID))
	if err != nil {
		t.Fatalf("BuildStatsInput() error = %v, want nil", err)
	}
	got := stats.AggregateStats(input.AggregationInput())

	assertFloat(t, got.Core.HPMax, 100)
	assertFloat(t, got.Core.ShieldMax, 60)
	assertFloat(t, got.Core.CargoCapacity, 50)
	assertFloat(t, got.Combat.WeaponDamage, 12)
	assertFloat(t, got.Combat.WeaponRange, 650)
	assertFloat(t, got.Combat.Accuracy, 0.82)
	assertFloat(t, got.Combat.WeaponEnergyCost, 8)
	assertFloat(t, got.Combat.WeaponCooldown, 1.2)
}

func TestStatInputProviderBuildsScannerAndRadarStats(t *testing.T) {
	shipCatalog := mustShipCatalog(t)
	moduleCatalog := modules.MustMVPCatalog()
	loadout := modules.NewInMemoryLoadoutStore()
	playerID := foundation.PlayerID("player-1")
	shipID := ships.ShipIDScoutT1

	putRuntimeModuleItem(t, loadout, "scanner-instance-1", "scanner_t1", playerID, 100)
	putRuntimeModuleItem(t, loadout, "radar-instance-1", "radar_t1", playerID, 100)
	equippedAt := time.Date(2026, 6, 17, 16, 0, 0, 0, time.UTC)
	if err := loadout.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID: playerID,
		ShipID:   shipID,
		Equipped: []modules.EquippedModule{
			{
				PlayerID:       playerID,
				ShipID:         shipID,
				SlotID:         modules.ModuleSlotUtility1,
				ItemInstanceID: "scanner-instance-1",
				EquippedAt:     equippedAt,
			},
			{
				PlayerID:       playerID,
				ShipID:         shipID,
				SlotID:         modules.ModuleSlotUtility2,
				ItemInstanceID: "radar-instance-1",
				EquippedAt:     equippedAt,
			},
		}}); err != nil {
		t.Fatalf("ReplaceEquippedModules() error = %v, want nil", err)
	}

	provider, err := NewStatInputProvider(shipCatalog, moduleCatalog, loadout)
	if err != nil {
		t.Fatalf("NewStatInputProvider() error = %v, want nil", err)
	}
	input, err := provider.BuildStatsInput(stats.NewStatSubject(playerID, shipID))
	if err != nil {
		t.Fatalf("BuildStatsInput() error = %v, want nil", err)
	}
	got := stats.AggregateStats(input.AggregationInput())

	assertFloat(t, got.Exploration.ScanPower, 10)
	assertFloat(t, got.Exploration.ScanRadius, 450)
	assertFloat(t, got.Exploration.ScanInterval, 3)
	assertFloat(t, got.Exploration.RadarRange, 1370)
	assertFloat(t, got.Combat.WeaponCooldown, 0)
}

func TestStatInputProviderBuildsUnlockedPilotSkillPassiveStats(t *testing.T) {
	shipCatalog := mustShipCatalog(t)
	moduleCatalog := modules.MustMVPCatalog()
	loadout := modules.NewInMemoryLoadoutStore()
	playerID := foundation.PlayerID("player-1")
	shipID := ships.ShipIDStarter
	progress := progression.NewProgressionService(testutil.NewFakeClock(time.Date(2026, 6, 17, 15, 0, 0, 0, time.UTC)), nil)

	seedPilotPassivesProgression(t, progress, playerID)
	for _, nodeID := range []progression.SkillNodeID{
		"combat_weapon_calibration",
		"combat_heat_control",
		"scout_signal_tuning",
		"industry_cargo_protocols",
	} {
		if _, err := progress.UnlockPilotSkill(progression.UnlockPilotSkillInput{PlayerID: playerID, NodeID: nodeID}); err != nil {
			t.Fatalf("UnlockPilotSkill(%q) error = %v, want nil", nodeID, err)
		}
	}

	putRuntimeModuleItem(t, loadout, "laser-instance-1", "laser_alpha_t1", playerID, 100)
	if err := loadout.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID: playerID,
		ShipID:   shipID,
		Equipped: []modules.EquippedModule{{
			PlayerID:       playerID,
			ShipID:         shipID,
			SlotID:         modules.ModuleSlotOffensive1,
			ItemInstanceID: "laser-instance-1",
			EquippedAt:     time.Date(2026, 6, 17, 16, 0, 0, 0, time.UTC),
		}},
	}); err != nil {
		t.Fatalf("ReplaceEquippedModules() error = %v, want nil", err)
	}

	provider, err := NewStatInputProviderWithProgression(shipCatalog, moduleCatalog, loadout, progress)
	if err != nil {
		t.Fatalf("NewStatInputProviderWithProgression() error = %v, want nil", err)
	}
	input, err := provider.BuildStatsInput(stats.NewStatSubject(playerID, shipID))
	if err != nil {
		t.Fatalf("BuildStatsInput() error = %v, want nil", err)
	}
	got := stats.AggregateStats(input.AggregationInput())

	assertFloat(t, got.Combat.WeaponDamage, 14)
	assertFloat(t, got.Combat.WeaponEnergyCost, 7.84)
	assertFloat(t, got.Exploration.ScanPower, 1)
	assertFloat(t, got.Core.CargoCapacity, 60)
	if len(input.FlatPassives) != 3 {
		t.Fatalf("FlatPassives len = %d, want 3", len(input.FlatPassives))
	}
	if len(input.PercentPassives) != 1 || input.PercentPassives[0].SourceID != "combat_heat_control" {
		t.Fatalf("PercentPassives = %+v, want combat_heat_control", input.PercentPassives)
	}
}

func TestPilotSkillPassiveModifiersMapEveryMVPDefinition(t *testing.T) {
	playerID := foundation.PlayerID("player-1")
	now := time.Date(2026, 6, 17, 15, 30, 0, 0, time.UTC)
	definitions := progression.PilotSkillDefinitions()
	unlockedNodes := make([]progression.UnlockedSkillNodeState, 0, len(definitions))
	for _, definition := range definitions {
		unlockedNodes = append(unlockedNodes, progression.UnlockedSkillNodeState{
			PlayerID:   playerID,
			NodeID:     definition.NodeID,
			UnlockedAt: now,
		})
	}
	player, err := progression.NewPlayerProgressionState(playerID, 1500, progression.MaxMVPRank)
	if err != nil {
		t.Fatalf("NewPlayerProgressionState() error = %v, want nil", err)
	}
	snapshot, err := progression.NewProgressionSnapshot(
		player,
		nil,
		progression.SkillPointState{
			PlayerID:    playerID,
			TotalPoints: len(definitions),
			SpentPoints: len(definitions),
			UpdatedAt:   now,
		},
		unlockedNodes,
	)
	if err != nil {
		t.Fatalf("NewProgressionSnapshot() error = %v, want nil", err)
	}

	flatPassives, percentPassives, err := pilotSkillPassiveModifiers(snapshot)
	if err != nil {
		t.Fatalf("pilotSkillPassiveModifiers() error = %v, want nil", err)
	}
	input := stats.StatBuildInput{
		BaseShip: stats.EffectiveStats{
			Core: stats.CoreStats{
				Speed:         100,
				CargoCapacity: 50,
			},
			Combat: stats.CombatStats{
				WeaponDamage:     100,
				WeaponEnergyCost: 10,
			},
		},
		FlatPassives:    flatPassives,
		PercentPassives: percentPassives,
	}
	got := stats.AggregateStats(input.AggregationInput())

	if len(flatPassives)+len(percentPassives) != len(definitions) {
		t.Fatalf("passive modifier count = flat %d percent %d want total %d", len(flatPassives), len(percentPassives), len(definitions))
	}
	assertFloat(t, got.Combat.WeaponDamage, 104.04)
	assertFloat(t, got.Combat.WeaponEnergyCost, 9.8)
	assertFloat(t, got.Core.ShieldRegen, 1)
	assertFloat(t, got.Combat.Accuracy, 0.02)
	assertFloat(t, got.Exploration.ScanPower, 1)
	assertFloat(t, got.Exploration.RadarRange, 25)
	assertFloat(t, got.Exploration.FogRevealRadius, 12)
	assertFloat(t, got.Exploration.ScanSuccessBonus, 0.02)
	assertFloat(t, got.Core.Speed, 102)
	assertFloat(t, got.Core.CargoCapacity, 60)
	assertFloat(t, got.Economy.CraftSpeed, 0.02)
	assertFloat(t, got.Economy.ConstructionSpeed, 0.02)
	assertFloat(t, got.Economy.CraftMaterialRefundBonus, 0.01)
	assertFloat(t, got.Economy.RouteCargoCapacityBonus, 0.02)
}

func TestStatInputProviderIgnoresBrokenEquippedModules(t *testing.T) {
	shipCatalog := mustShipCatalog(t)
	loadout := modules.NewInMemoryLoadoutStore()
	playerID := foundation.PlayerID("player-1")
	shipID := ships.ShipIDStarter

	putRuntimeModuleItem(t, loadout, "laser-instance-1", "laser_alpha_t1", playerID, 0)
	if err := loadout.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID: playerID,
		ShipID:   shipID,
		Equipped: []modules.EquippedModule{{
			PlayerID:       playerID,
			ShipID:         shipID,
			SlotID:         modules.ModuleSlotOffensive1,
			ItemInstanceID: "laser-instance-1",
			EquippedAt:     time.Date(2026, 6, 17, 16, 0, 0, 0, time.UTC),
		}},
	}); err != nil {
		t.Fatalf("ReplaceEquippedModules() error = %v, want nil", err)
	}

	provider, err := NewStatInputProvider(shipCatalog, modules.MustMVPCatalog(), loadout)
	if err != nil {
		t.Fatalf("NewStatInputProvider() error = %v, want nil", err)
	}
	input, err := provider.BuildStatsInput(stats.NewStatSubject(playerID, shipID))
	if err != nil {
		t.Fatalf("BuildStatsInput() error = %v, want nil", err)
	}
	got := stats.AggregateStats(input.AggregationInput())

	assertFloat(t, got.Combat.WeaponDamage, 0)
	assertFloat(t, got.Combat.WeaponRange, 0)
	assertFloat(t, got.Combat.Accuracy, 0)
}

func TestStatCargoCapacityProviderUsesEffectiveStats(t *testing.T) {
	shipCatalog := mustShipCatalog(t)
	loadout := modules.NewInMemoryLoadoutStore()
	playerID := foundation.PlayerID("player-1")
	shipID := ships.ShipIDStarter

	putRuntimeModuleItem(t, loadout, "cargo-expander-instance-1", "cargo_expander_t1", playerID, 100)
	if err := loadout.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID: playerID,
		ShipID:   shipID,
		Equipped: []modules.EquippedModule{{
			PlayerID:       playerID,
			ShipID:         shipID,
			SlotID:         modules.ModuleSlotUtility1,
			ItemInstanceID: "cargo-expander-instance-1",
			EquippedAt:     time.Date(2026, 6, 17, 16, 0, 0, 0, time.UTC),
		}},
	}); err != nil {
		t.Fatalf("ReplaceEquippedModules() error = %v, want nil", err)
	}
	inputs, err := NewStatInputProvider(shipCatalog, modules.MustMVPCatalog(), loadout)
	if err != nil {
		t.Fatalf("NewStatInputProvider() error = %v, want nil", err)
	}
	statService, err := stats.NewStatService(testutil.NewFakeClock(time.Date(2026, 6, 17, 17, 0, 0, 0, time.UTC)), nil, nil, inputs)
	if err != nil {
		t.Fatalf("NewStatService() error = %v, want nil", err)
	}
	provider, err := NewStatCargoCapacityProvider(statService)
	if err != nil {
		t.Fatalf("NewStatCargoCapacityProvider() error = %v, want nil", err)
	}
	shipDefinition, err := shipCatalog.MustGet(shipID)
	if err != nil {
		t.Fatalf("MustGet() error = %v, want nil", err)
	}

	capacity, err := provider.CargoCapacityForShip(playerID, shipDefinition)
	if err != nil {
		t.Fatalf("CargoCapacityForShip() error = %v, want nil", err)
	}
	if capacity != 90 {
		t.Fatalf("CargoCapacityForShip() = %d, want 90", capacity)
	}
}

func TestRuntimeProviderConstructorsRejectNilDependencies(t *testing.T) {
	if _, err := NewProgressionProvider(nil); !errors.Is(err, ErrNilProgressionService) {
		t.Fatalf("NewProgressionProvider(nil) error = %v, want ErrNilProgressionService", err)
	}
	if _, err := NewStatInputProvider(ships.Catalog{}, modules.Catalog{}, nil); !errors.Is(err, ErrNilModuleLoadoutStore) {
		t.Fatalf("NewStatInputProvider(nil loadout) error = %v, want ErrNilModuleLoadoutStore", err)
	}
	if _, err := NewStatInputProviderWithProgression(ships.Catalog{}, modules.Catalog{}, modules.NewInMemoryLoadoutStore(), nil); !errors.Is(err, ErrNilProgressionReader) {
		t.Fatalf("NewStatInputProviderWithProgression(nil progression) error = %v, want ErrNilProgressionReader", err)
	}
	if _, err := NewStatCargoCapacityProvider(nil); !errors.Is(err, ErrNilStatService) {
		t.Fatalf("NewStatCargoCapacityProvider(nil) error = %v, want ErrNilStatService", err)
	}
	if _, err := NewModuleInventoryLedgerAdapter(nil, modules.Catalog{}); !errors.Is(err, ErrNilInventoryService) {
		t.Fatalf("NewModuleInventoryLedgerAdapter(nil) error = %v, want ErrNilInventoryService", err)
	}
}

func mustShipCatalog(t *testing.T) ships.Catalog {
	t.Helper()
	shipCatalog, err := ships.MVPShipCatalog()
	if err != nil {
		t.Fatalf("MVPShipCatalog() error = %v, want nil", err)
	}
	return shipCatalog
}

func seedPilotPassivesProgression(t *testing.T, service *progression.ProgressionService, playerID foundation.PlayerID) {
	t.Helper()
	if _, err := service.GrantXP(progression.GrantXPInput{
		PlayerID:       playerID,
		Amount:         1500,
		SourceType:     progression.XPSourceTypeAdminAdjustment,
		SourceID:       "passive-stat-seed",
		IdempotencyKey: "passive-stat-seed",
		Authority:      progression.XPGrantAuthorityAdminService,
		RoleXP:         []progression.RoleXPGrant{{Role: progression.RoleTypeCombat, Amount: 500}},
	}); err != nil {
		t.Fatalf("GrantXP() error = %v, want nil", err)
	}
	for _, input := range []progression.TryRankUpInput{
		{PlayerID: playerID, TargetRank: 2, Reason: "passive_stat_seed", IdempotencyKey: "passive-stat-rank-2"},
		{PlayerID: playerID, TargetRank: 3, Reason: "passive_stat_seed", IdempotencyKey: "passive-stat-rank-3"},
		{PlayerID: playerID, TargetRank: 4, Reason: "passive_stat_seed", IdempotencyKey: "passive-stat-rank-4"},
		{PlayerID: playerID, TargetRank: 5, Reason: "passive_stat_seed", IdempotencyKey: "passive-stat-rank-5"},
	} {
		result, err := service.TryRankUp(input)
		if err != nil {
			t.Fatalf("TryRankUp(rank %d) error = %v, want nil", input.TargetRank, err)
		}
		if !result.RankedUp {
			t.Fatalf("TryRankUp(rank %d) ranked_up = false, missing = %+v", input.TargetRank, result.MissingRequirements)
		}
	}
}

func putRuntimeModuleItem(
	t *testing.T,
	store *modules.InMemoryLoadoutStore,
	itemInstanceID foundation.ItemID,
	itemID foundation.ItemID,
	owner foundation.PlayerID,
	durability int64,
) {
	t.Helper()
	quantity, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(1) error = %v, want nil", err)
	}
	item := economy.InstanceItem{
		Source: catalog.VersionedDefinition{
			DefinitionID: catalog.DefinitionID(itemID),
			Version:      modules.ModuleCatalogVersion,
		},
		ItemInstanceID:    itemInstanceID,
		ItemID:            itemID,
		OwnerPlayerID:     owner,
		Location:          economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID(owner.String())},
		Quantity:          quantity,
		DurabilityCurrent: durability,
		BoundState:        economy.BoundStateUnbound,
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("test module item Validate() error = %v, want nil", err)
	}
	if err := store.PutModuleItem(item); err != nil {
		t.Fatalf("PutModuleItem() error = %v, want nil", err)
	}
}

func assertFloat(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("got %f, want %f", got, want)
	}
}
