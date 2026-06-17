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
	ErrNilModuleLoadoutStore = errors.New("nil module loadout store")
	ErrNilStatService        = errors.New("nil stat service")
	ErrInvalidCargoCapacity  = errors.New("invalid cargo capacity")
)

// ModuleLoadoutReader is the small read boundary needed to build module stat
// inputs from the authoritative loadout store.
type ModuleLoadoutReader interface {
	ModuleItem(itemInstanceID foundation.ItemID) (economy.InstanceItem, error)
	EquippedModules(playerID foundation.PlayerID, shipID foundation.ShipID) ([]modules.EquippedModule, error)
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
// currently equipped modules.
type StatInputProvider struct {
	ships   ships.Catalog
	modules modules.Catalog
	loadout ModuleLoadoutReader
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

	return stats.StatBuildInput{
		BaseShip: shipBaseStats(shipDefinition.BaseStats),
		Modules:  modifiers,
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
		if cooldown.Key == modules.CooldownBasicAttack {
			modifier.Flat.Combat.WeaponCooldown += float64(cooldown.DurationMS) / 1000
		}
	}
	return modifier
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
