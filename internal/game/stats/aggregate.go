package stats

// ModifierSource is a stable label for the domain object that produced a stat
// modifier. It is intentionally string-backed so future packages can map their
// own IDs without introducing package dependencies here.
type ModifierSource string

const (
	ModifierSourceModule  ModifierSource = "module"
	ModifierSourcePassive ModifierSource = "passive"
	ModifierSourceRole    ModifierSource = "role"
	ModifierSourceBuff    ModifierSource = "buff"
	ModifierSourceDebuff  ModifierSource = "debuff"
)

// FlatModifier adds stat values during its aggregation stage.
type FlatModifier struct {
	Source   ModifierSource `json:"source"`
	SourceID string         `json:"source_id"`
	Stats    FlatStats      `json:"stats"`
}

// PercentModifier multiplies stat values during its aggregation stage.
type PercentModifier struct {
	Source   ModifierSource `json:"source"`
	SourceID string         `json:"source_id"`
	Stats    PercentStats   `json:"stats"`
}

// TemporaryModifier represents buff and debuff effects. Temporary modifiers
// are applied after persistent percent modifiers, in caller-provided order.
type TemporaryModifier struct {
	Source   ModifierSource `json:"source"`
	SourceID string         `json:"source_id"`
	Flat     FlatStats      `json:"flat"`
	Percent  PercentStats   `json:"percent"`
}

// AggregationInput is the boundary between future domain services and stat
// aggregation. It deliberately contains simple value objects instead of
// references to ship, module, progression, or buff packages.
type AggregationInput struct {
	BaseShip EffectiveStats `json:"base_ship"`

	FlatModules  []FlatModifier `json:"flat_modules"`
	FlatPassives []FlatModifier `json:"flat_passives"`
	RoleBonuses  []FlatModifier `json:"role_bonuses"`

	PercentModules  []PercentModifier `json:"percent_modules"`
	PercentPassives []PercentModifier `json:"percent_passives"`

	TemporaryModifiers []TemporaryModifier `json:"temporary_modifiers"`
}

// AggregateStats computes effective stats in the documented Phase 03 order:
// base ship, flat modules, flat passives, role bonuses, percent modules,
// percent passives, buffs/debuffs, then clamp.
func AggregateStats(input AggregationInput) EffectiveStats {
	stats := input.BaseShip

	for _, modifier := range input.FlatModules {
		stats.applyFlat(modifier.Stats)
	}
	for _, modifier := range input.FlatPassives {
		stats.applyFlat(modifier.Stats)
	}
	for _, modifier := range input.RoleBonuses {
		stats.applyFlat(modifier.Stats)
	}
	for _, modifier := range input.PercentModules {
		stats.applyPercent(modifier.Stats)
	}
	for _, modifier := range input.PercentPassives {
		stats.applyPercent(modifier.Stats)
	}
	for _, modifier := range input.TemporaryModifiers {
		stats.applyFlat(modifier.Flat)
		stats.applyPercent(modifier.Percent)
	}

	stats.Clamp()
	return stats
}

// StatAggregationService is the Phase 03 service skeleton for effective stat
// recalculation. Later slices can add snapshot storage and active-session cache
// reads without changing the aggregation order.
type StatAggregationService struct{}

// NewStatAggregationService returns a stat aggregation service.
func NewStatAggregationService() StatAggregationService {
	return StatAggregationService{}
}

// RecalculateStats computes a fresh effective stat snapshot payload from local
// aggregation inputs.
func (StatAggregationService) RecalculateStats(input AggregationInput) EffectiveStats {
	return AggregateStats(input)
}
