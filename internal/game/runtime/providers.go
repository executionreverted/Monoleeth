package runtime

import (
	"errors"
	"fmt"
	"math"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/stats"
)

var (
	ErrNilProgressionService = errors.New("nil progression service")
	ErrNilProgressionReader  = errors.New("nil progression reader")
	ErrNilModuleLoadoutStore = errors.New("nil module loadout store")
	ErrNilStatService        = errors.New("nil stat service")
	ErrInvalidCargoCapacity  = errors.New("invalid cargo capacity")
	ErrUnsupportedSkillStat  = errors.New("unsupported pilot skill stat")
	ErrInvalidSkillEffect    = errors.New("invalid pilot skill effect")
)

// ModuleLoadoutReader is the small read boundary needed to build module stat
// inputs from the authoritative loadout store.
type ModuleLoadoutReader interface {
	ModuleItem(itemInstanceID foundation.ItemID) (economy.InstanceItem, error)
	EquippedModules(playerID foundation.PlayerID, shipID foundation.ShipID) ([]modules.EquippedModule, error)
}

// ProgressionSnapshotReader is the progression read boundary needed for
// server-owned pilot passives during stat input composition.
type ProgressionSnapshotReader interface {
	GetProgressionSnapshot(playerID foundation.PlayerID) (progression.ProgressionSnapshot, error)
}

// ProgressionProvider adapts ProgressionService to the ship and module
// validation provider interfaces.
type ProgressionProvider struct {
	service *progression.ProgressionService
}

// NewProgressionProvider returns runtime progression adapters for rank and
// module role-gate validation.
func NewProgressionProvider(service *progression.ProgressionService) (*ProgressionProvider, error) {
	if service == nil {
		return nil, ErrNilProgressionService
	}
	return &ProgressionProvider{service: service}, nil
}

// RankForPlayer returns the authoritative current rank for ship unlock checks.
func (provider *ProgressionProvider) RankForPlayer(playerID foundation.PlayerID) (int, error) {
	snapshot, err := provider.service.GetProgressionSnapshot(playerID)
	if err != nil {
		return 0, err
	}
	return snapshot.Player.Rank, nil
}

// ProgressionForPlayer returns progression in the shape module loadouts need.
func (provider *ProgressionProvider) ProgressionForPlayer(playerID foundation.PlayerID) (modules.PilotProgression, error) {
	snapshot, err := provider.service.GetProgressionSnapshot(playerID)
	if err != nil {
		return modules.PilotProgression{}, err
	}

	return modules.PilotProgression{
		Rank:       snapshot.Player.Rank,
		RoleLevels: moduleRoleLevels(snapshot),
	}, nil
}

// StatInputProvider builds stat aggregation inputs from ship definitions and
// currently equipped modules and unlocked pilot passives.
type StatInputProvider struct {
	ships       ships.Catalog
	modules     modules.Catalog
	loadout     ModuleLoadoutReader
	progression ProgressionSnapshotReader
}

// NewStatInputProvider returns a catalog-backed stat input adapter.
func NewStatInputProvider(
	shipCatalog ships.Catalog,
	moduleCatalog modules.Catalog,
	loadout ModuleLoadoutReader,
) (*StatInputProvider, error) {
	if loadout == nil {
		return nil, ErrNilModuleLoadoutStore
	}
	return &StatInputProvider{
		ships:   shipCatalog,
		modules: moduleCatalog,
		loadout: loadout,
	}, nil
}

// NewStatInputProviderWithProgression returns a stat input adapter that also
// maps unlocked pilot-skill passives from authoritative progression snapshots.
func NewStatInputProviderWithProgression(
	shipCatalog ships.Catalog,
	moduleCatalog modules.Catalog,
	loadout ModuleLoadoutReader,
	progressionReader ProgressionSnapshotReader,
) (*StatInputProvider, error) {
	if progressionReader == nil {
		return nil, ErrNilProgressionReader
	}
	provider, err := NewStatInputProvider(shipCatalog, moduleCatalog, loadout)
	if err != nil {
		return nil, err
	}
	provider.progression = progressionReader
	return provider, nil
}

// BuildStatsInput returns the authoritative aggregation input for subject.
func (provider *StatInputProvider) BuildStatsInput(subject stats.StatSubject) (stats.StatBuildInput, error) {
	if err := subject.PlayerID.Validate(); err != nil {
		return stats.StatBuildInput{}, fmt.Errorf("player_id: %w", err)
	}
	if err := subject.ShipID.Validate(); err != nil {
		return stats.StatBuildInput{}, fmt.Errorf("ship_id: %w", err)
	}

	shipDefinition, err := provider.ships.MustGet(subject.ShipID)
	if err != nil {
		return stats.StatBuildInput{}, err
	}

	equipped, err := provider.loadout.EquippedModules(subject.PlayerID, subject.ShipID)
	if err != nil {
		return stats.StatBuildInput{}, err
	}

	modifiers := make([]stats.ModuleModifier, 0, len(equipped))
	for _, equippedModule := range equipped {
		item, err := provider.loadout.ModuleItem(equippedModule.ItemInstanceID)
		if err != nil {
			return stats.StatBuildInput{}, err
		}
		definition, ok := provider.modules.Lookup(item.ItemID)
		if !ok {
			return stats.StatBuildInput{}, fmt.Errorf("module item %q: %w", item.ItemID, modules.ErrUnknownModuleDefinition)
		}
		modifier := moduleDefinitionModifier(definition)
		modifier.SourceID = equippedModule.ItemInstanceID.String()
		modifier.Broken = item.DurabilityCurrent <= 0
		modifiers = append(modifiers, modifier)
	}

	flatPassives, percentPassives, err := provider.pilotPassiveModifiers(subject.PlayerID)
	if err != nil {
		return stats.StatBuildInput{}, err
	}

	return stats.StatBuildInput{
		BaseShip:        shipBaseStats(shipDefinition.BaseStats),
		Modules:         modifiers,
		FlatPassives:    flatPassives,
		PercentPassives: percentPassives,
	}, nil
}

// StatCargoCapacityProvider adapts effective stat aggregation to ship cargo
// capacity checks.
type StatCargoCapacityProvider struct {
	stats *stats.StatService
}

// NewStatCargoCapacityProvider returns a module-aware ship cargo capacity
// provider.
func NewStatCargoCapacityProvider(statsService *stats.StatService) (*StatCargoCapacityProvider, error) {
	if statsService == nil {
		return nil, ErrNilStatService
	}
	return &StatCargoCapacityProvider{stats: statsService}, nil
}

// CargoCapacityForShip returns the effective cargo capacity for target.
func (provider *StatCargoCapacityProvider) CargoCapacityForShip(playerID foundation.PlayerID, target ships.ShipDefinition) (int64, error) {
	snapshot, err := provider.stats.GetEffectiveStats(stats.NewStatSubject(playerID, target.ShipID))
	if err != nil {
		return 0, err
	}
	capacity := snapshot.Stats.Core.CargoCapacity
	if math.IsNaN(capacity) || math.IsInf(capacity, 0) || capacity < 0 || capacity > float64(foundation.MaxAmount) {
		return 0, fmt.Errorf("cargo capacity %f: %w", capacity, ErrInvalidCargoCapacity)
	}
	return int64(capacity), nil
}

func moduleRoleLevels(snapshot progression.ProgressionSnapshot) map[modules.PilotRole]int {
	levels := make(map[modules.PilotRole]int, len(progression.SupportedRoleTypes()))
	for _, role := range progression.SupportedRoleTypes() {
		roleState, ok := snapshot.RoleLevel(role)
		if !ok {
			continue
		}
		levels[modulePilotRole(role)] = roleState.Level
	}
	return levels
}

func modulePilotRole(role progression.RoleType) modules.PilotRole {
	switch role {
	case progression.RoleTypeCombat:
		return modules.PilotRoleCombat
	case progression.RoleTypeScout:
		return modules.PilotRoleScout
	case progression.RoleTypeCrafting:
		return modules.PilotRoleCrafting
	case progression.RoleTypeConstruction:
		return modules.PilotRoleConstruction
	default:
		return modules.PilotRole(role)
	}
}

func (provider *StatInputProvider) pilotPassiveModifiers(playerID foundation.PlayerID) ([]stats.FlatModifier, []stats.PercentModifier, error) {
	if provider.progression == nil {
		return nil, nil, nil
	}
	snapshot, err := provider.progression.GetProgressionSnapshot(playerID)
	if err != nil {
		return nil, nil, err
	}
	return pilotSkillPassiveModifiers(snapshot)
}

func pilotSkillPassiveModifiers(snapshot progression.ProgressionSnapshot) ([]stats.FlatModifier, []stats.PercentModifier, error) {
	unlocked := snapshot.UnlockedSkillNodes()
	flatModifiers := make([]stats.FlatModifier, 0, len(unlocked))
	percentModifiers := make([]stats.PercentModifier, 0, len(unlocked))

	for _, unlockedNode := range unlocked {
		definition, err := progression.PilotSkillDefinitionFor(unlockedNode.NodeID)
		if err != nil {
			return nil, nil, err
		}
		flat, percent, err := pilotSkillDefinitionModifier(definition)
		if err != nil {
			return nil, nil, err
		}
		sourceID := unlockedNode.NodeID.String()
		if flat != (stats.FlatStats{}) {
			flatModifiers = append(flatModifiers, stats.FlatModifier{
				Source:   stats.ModifierSourcePassive,
				SourceID: sourceID,
				Stats:    flat,
			})
		}
		if percent != (stats.PercentStats{}) {
			percentModifiers = append(percentModifiers, stats.PercentModifier{
				Source:   stats.ModifierSourcePassive,
				SourceID: sourceID,
				Stats:    percent,
			})
		}
	}
	return flatModifiers, percentModifiers, nil
}

func pilotSkillDefinitionModifier(definition progression.PilotSkillDefinition) (stats.FlatStats, stats.PercentStats, error) {
	var flat stats.FlatStats
	var percent stats.PercentStats
	for _, effect := range definition.Effects {
		if err := applyPilotSkillEffect(&flat, &percent, effect); err != nil {
			return stats.FlatStats{}, stats.PercentStats{}, fmt.Errorf("skill node %q effect %q: %w", definition.NodeID, effect.Stat, err)
		}
	}
	return flat, percent, nil
}

func applyPilotSkillEffect(flat *stats.FlatStats, percent *stats.PercentStats, effect progression.PilotSkillEffect) error {
	if math.IsNaN(effect.Value) || math.IsInf(effect.Value, 0) {
		return ErrInvalidSkillEffect
	}
	switch effect.Operation {
	case progression.PilotSkillEffectAdd:
		return applyFlatPilotSkillStat(flat, effect.Stat, effect.Value)
	case progression.PilotSkillEffectMul:
		return applyMultiplierPilotSkillStat(flat, percent, effect.Stat, effect.Value)
	default:
		return ErrInvalidSkillEffect
	}
}

func applyFlatPilotSkillStat(flat *stats.FlatStats, stat string, value float64) error {
	switch stat {
	case "laser_damage_flat":
		flat.Combat.WeaponDamage += value
	case "shield_regen_flat":
		flat.Core.ShieldRegen += value
	case "weapon_accuracy_bonus":
		flat.Combat.Accuracy += value
	case "scan_strength_flat":
		flat.Exploration.ScanPower += value
	case "radar_range_flat":
		flat.Exploration.RadarRange += value
	case "fog_reveal_radius_flat":
		flat.Exploration.FogRevealRadius += value
	case "scan_success_bonus":
		flat.Exploration.ScanSuccessBonus += value
	case "cargo_capacity_flat":
		flat.Core.CargoCapacity += value
	case "craft_material_refund_bonus":
		flat.Economy.CraftMaterialRefundBonus += value
	default:
		return ErrUnsupportedSkillStat
	}
	return nil
}

func applyMultiplierPilotSkillStat(flat *stats.FlatStats, percent *stats.PercentStats, stat string, value float64) error {
	switch stat {
	case "laser_energy_cost_mult":
		percent.Combat.WeaponEnergyCost += value - 1
	case "weapon_damage_mult":
		percent.Combat.WeaponDamage += value - 1
	case "ship_speed_mult":
		percent.Core.Speed += value - 1
	case "craft_time_mult":
		flat.Economy.CraftSpeed += 1 - value
	case "construction_time_mult":
		flat.Economy.ConstructionSpeed += 1 - value
	case "route_cargo_capacity_mult":
		flat.Economy.RouteCargoCapacityBonus += value - 1
	default:
		return ErrUnsupportedSkillStat
	}
	return nil
}

func shipBaseStats(base ships.ShipBaseStats) stats.EffectiveStats {
	return stats.EffectiveStats{
		Core: stats.CoreStats{
			HPMax:         float64(base.HP),
			ShieldMax:     float64(base.Shield),
			EnergyMax:     float64(base.Energy),
			EnergyRegen:   float64(base.EnergyRegen),
			Speed:         float64(base.Speed),
			CargoCapacity: float64(base.CargoCapacity),
		},
		Exploration: stats.ExplorationStats{
			RadarRange:      float64(base.Radar),
			SignatureRadius: float64(base.Signature),
		},
	}
}

func moduleDefinitionModifier(definition modules.ModuleDefinition) stats.ModuleModifier {
	modifier := stats.ModuleModifier{SourceID: definition.ItemID.String()}
	for _, statModifier := range definition.StatModifiers {
		applyModuleStat(&modifier, statModifier)
	}
	if definition.Energy.ActivationCost > 0 {
		modifier.Flat.Combat.WeaponEnergyCost += float64(definition.Energy.ActivationCost)
	}
	for _, cooldown := range definition.Cooldowns {
		applyModuleCooldown(&modifier.Flat, cooldown)
	}
	return modifier
}

func applyModuleCooldown(flat *stats.FlatStats, cooldown modules.Cooldown) {
	durationSeconds := float64(cooldown.DurationMS) / 1000
	switch cooldown.Key {
	case modules.CooldownBasicAttack:
		flat.Combat.WeaponCooldown += durationSeconds
	case modules.CooldownScanPulse:
		flat.Exploration.ScanInterval += durationSeconds
	case modules.CooldownRadarSweep:
		// Radar has range in the current effective stats model, but no
		// separate sweep interval field yet. Keep this cooldown in catalog
		// metadata until the radar command path has a concrete stat target.
	}
}

func applyModuleStat(modifier *stats.ModuleModifier, statModifier modules.StatModifier) {
	if statModifier.Kind == modules.StatModifierPercent {
		applyPercentModuleStat(&modifier.Percent, statModifier.Stat, float64(statModifier.Value)/10_000)
		return
	}
	applyFlatModuleStat(&modifier.Flat, statModifier.Stat, moduleFlatStatValue(statModifier))
}

func moduleFlatStatValue(modifier modules.StatModifier) float64 {
	if modifier.Stat == modules.StatAccuracy {
		return float64(modifier.Value) / 10_000
	}
	return float64(modifier.Value)
}

func applyFlatModuleStat(flat *stats.FlatStats, stat modules.StatKey, value float64) {
	switch stat {
	case modules.StatWeaponDamage:
		flat.Combat.WeaponDamage += value
	case modules.StatWeaponRange:
		flat.Combat.WeaponRange += value
	case modules.StatAccuracy:
		flat.Combat.Accuracy += value
	case modules.StatShieldMax:
		flat.Core.ShieldMax += value
	case modules.StatShieldRegen:
		flat.Core.ShieldRegen += value
	case modules.StatScanPower:
		flat.Exploration.ScanPower += value
	case modules.StatScanRadius:
		flat.Exploration.ScanRadius += value
	case modules.StatRadarRange:
		flat.Exploration.RadarRange += value
	case modules.StatCargoCapacity:
		flat.Core.CargoCapacity += value
	}
}

func applyPercentModuleStat(percent *stats.PercentStats, stat modules.StatKey, value float64) {
	switch stat {
	case modules.StatWeaponDamage:
		percent.Combat.WeaponDamage += value
	case modules.StatWeaponRange:
		percent.Combat.WeaponRange += value
	case modules.StatAccuracy:
		percent.Combat.Accuracy += value
	case modules.StatShieldMax:
		percent.Core.ShieldMax += value
	case modules.StatShieldRegen:
		percent.Core.ShieldRegen += value
	case modules.StatScanPower:
		percent.Exploration.ScanPower += value
	case modules.StatScanRadius:
		percent.Exploration.ScanRadius += value
	case modules.StatRadarRange:
		percent.Exploration.RadarRange += value
	case modules.StatCargoCapacity:
		percent.Core.CargoCapacity += value
	}
}
