package stats

import "math"

// EffectiveStats is the server-calculated stat snapshot consumed by gameplay
// systems. Clients may read snapshots, but they must not submit these values as
// gameplay truth.
type EffectiveStats struct {
	Core        CoreStats        `json:"core"`
	Combat      CombatStats      `json:"combat"`
	Exploration ExplorationStats `json:"exploration"`
	Economy     EconomyStats     `json:"economy"`
}

// FlatStats describes additive stat deltas.
type FlatStats EffectiveStats

// PercentStats describes multiplicative percent deltas as decimal fractions.
//
// A value of 0.10 means +10%; a value of -0.25 means -25%.
type PercentStats EffectiveStats

// CoreStats are shared by movement, survivability, energy, and cargo checks.
type CoreStats struct {
	HPMax         float64 `json:"hp_max"`
	ShieldMax     float64 `json:"shield_max"`
	ShieldRegen   float64 `json:"shield_regen"`
	EnergyMax     float64 `json:"energy_max"`
	EnergyRegen   float64 `json:"energy_regen"`
	Speed         float64 `json:"speed"`
	CargoCapacity float64 `json:"cargo_capacity"`
}

// CombatStats are used by combat resolution and defensive mitigation.
type CombatStats struct {
	WeaponDamage    float64 `json:"weapon_damage"`
	WeaponRange     float64 `json:"weapon_range"`
	WeaponCooldown  float64 `json:"weapon_cooldown"`
	Accuracy        float64 `json:"accuracy"`
	Tracking        float64 `json:"tracking"`
	Evasion         float64 `json:"evasion"`
	Penetration     float64 `json:"penetration"`
	CritChance      float64 `json:"crit_chance"`
	CritMultiplier  float64 `json:"crit_multiplier"`
	ResistLaser     float64 `json:"resist_laser"`
	ResistExplosive float64 `json:"resist_explosive"`
	ResistKinetic   float64 `json:"resist_kinetic"`
}

// ExplorationStats are used by radar, scanner, stealth, and jamming systems.
type ExplorationStats struct {
	RadarRange      float64 `json:"radar_range"`
	ScanPower       float64 `json:"scan_power"`
	ScanRadius      float64 `json:"scan_radius"`
	ScanInterval    float64 `json:"scan_interval"`
	SignatureRadius float64 `json:"signature_radius"`
	StealthStrength float64 `json:"stealth_strength"`
	JammerStrength  float64 `json:"jammer_strength"`
}

// EconomyStats are read by production, routing, crafting, and market rules.
type EconomyStats struct {
	CraftSpeed         float64 `json:"craft_speed"`
	RouteLossReduction float64 `json:"route_loss_reduction"`
	ProductionBonus    float64 `json:"production_bonus"`
	MarketFeeReduction float64 `json:"market_fee_reduction"`
}

// Clamp normalizes effective stats after all modifiers have been applied.
func (stats *EffectiveStats) Clamp() {
	if stats == nil {
		return
	}

	stats.Core.HPMax = clampNonNegative(stats.Core.HPMax)
	stats.Core.ShieldMax = clampNonNegative(stats.Core.ShieldMax)
	stats.Core.ShieldRegen = clampNonNegative(stats.Core.ShieldRegen)
	stats.Core.EnergyMax = clampNonNegative(stats.Core.EnergyMax)
	stats.Core.EnergyRegen = clampNonNegative(stats.Core.EnergyRegen)
	stats.Core.Speed = clampNonNegative(stats.Core.Speed)
	stats.Core.CargoCapacity = clampNonNegative(stats.Core.CargoCapacity)

	stats.Combat.WeaponDamage = clampNonNegative(stats.Combat.WeaponDamage)
	stats.Combat.WeaponRange = clampNonNegative(stats.Combat.WeaponRange)
	stats.Combat.WeaponCooldown = clampNonNegative(stats.Combat.WeaponCooldown)
	stats.Combat.Accuracy = clampUnit(stats.Combat.Accuracy)
	stats.Combat.Tracking = clampNonNegative(stats.Combat.Tracking)
	stats.Combat.Evasion = clampNonNegative(stats.Combat.Evasion)
	stats.Combat.Penetration = clampNonNegative(stats.Combat.Penetration)
	stats.Combat.CritChance = clampUnit(stats.Combat.CritChance)
	stats.Combat.CritMultiplier = clampNonNegative(stats.Combat.CritMultiplier)
	stats.Combat.ResistLaser = clampUnit(stats.Combat.ResistLaser)
	stats.Combat.ResistExplosive = clampUnit(stats.Combat.ResistExplosive)
	stats.Combat.ResistKinetic = clampUnit(stats.Combat.ResistKinetic)

	stats.Exploration.RadarRange = clampNonNegative(stats.Exploration.RadarRange)
	stats.Exploration.ScanPower = clampNonNegative(stats.Exploration.ScanPower)
	stats.Exploration.ScanRadius = clampNonNegative(stats.Exploration.ScanRadius)
	stats.Exploration.ScanInterval = clampNonNegative(stats.Exploration.ScanInterval)
	stats.Exploration.SignatureRadius = clampNonNegative(stats.Exploration.SignatureRadius)
	stats.Exploration.StealthStrength = clampNonNegative(stats.Exploration.StealthStrength)
	stats.Exploration.JammerStrength = clampNonNegative(stats.Exploration.JammerStrength)

	stats.Economy.CraftSpeed = clampNonNegative(stats.Economy.CraftSpeed)
	stats.Economy.RouteLossReduction = clampUnit(stats.Economy.RouteLossReduction)
	stats.Economy.ProductionBonus = clampNonNegative(stats.Economy.ProductionBonus)
	stats.Economy.MarketFeeReduction = clampUnit(stats.Economy.MarketFeeReduction)
}

func (stats *EffectiveStats) applyFlat(flat FlatStats) {
	stats.add(EffectiveStats(flat))
}

func (stats *EffectiveStats) applyPercent(percent PercentStats) {
	modifier := EffectiveStats(percent)

	stats.Core.HPMax *= percentMultiplier(modifier.Core.HPMax)
	stats.Core.ShieldMax *= percentMultiplier(modifier.Core.ShieldMax)
	stats.Core.ShieldRegen *= percentMultiplier(modifier.Core.ShieldRegen)
	stats.Core.EnergyMax *= percentMultiplier(modifier.Core.EnergyMax)
	stats.Core.EnergyRegen *= percentMultiplier(modifier.Core.EnergyRegen)
	stats.Core.Speed *= percentMultiplier(modifier.Core.Speed)
	stats.Core.CargoCapacity *= percentMultiplier(modifier.Core.CargoCapacity)

	stats.Combat.WeaponDamage *= percentMultiplier(modifier.Combat.WeaponDamage)
	stats.Combat.WeaponRange *= percentMultiplier(modifier.Combat.WeaponRange)
	stats.Combat.WeaponCooldown *= percentMultiplier(modifier.Combat.WeaponCooldown)
	stats.Combat.Accuracy *= percentMultiplier(modifier.Combat.Accuracy)
	stats.Combat.Tracking *= percentMultiplier(modifier.Combat.Tracking)
	stats.Combat.Evasion *= percentMultiplier(modifier.Combat.Evasion)
	stats.Combat.Penetration *= percentMultiplier(modifier.Combat.Penetration)
	stats.Combat.CritChance *= percentMultiplier(modifier.Combat.CritChance)
	stats.Combat.CritMultiplier *= percentMultiplier(modifier.Combat.CritMultiplier)
	stats.Combat.ResistLaser *= percentMultiplier(modifier.Combat.ResistLaser)
	stats.Combat.ResistExplosive *= percentMultiplier(modifier.Combat.ResistExplosive)
	stats.Combat.ResistKinetic *= percentMultiplier(modifier.Combat.ResistKinetic)

	stats.Exploration.RadarRange *= percentMultiplier(modifier.Exploration.RadarRange)
	stats.Exploration.ScanPower *= percentMultiplier(modifier.Exploration.ScanPower)
	stats.Exploration.ScanRadius *= percentMultiplier(modifier.Exploration.ScanRadius)
	stats.Exploration.ScanInterval *= percentMultiplier(modifier.Exploration.ScanInterval)
	stats.Exploration.SignatureRadius *= percentMultiplier(modifier.Exploration.SignatureRadius)
	stats.Exploration.StealthStrength *= percentMultiplier(modifier.Exploration.StealthStrength)
	stats.Exploration.JammerStrength *= percentMultiplier(modifier.Exploration.JammerStrength)

	stats.Economy.CraftSpeed *= percentMultiplier(modifier.Economy.CraftSpeed)
	stats.Economy.RouteLossReduction *= percentMultiplier(modifier.Economy.RouteLossReduction)
	stats.Economy.ProductionBonus *= percentMultiplier(modifier.Economy.ProductionBonus)
	stats.Economy.MarketFeeReduction *= percentMultiplier(modifier.Economy.MarketFeeReduction)
}

func (stats *EffectiveStats) add(delta EffectiveStats) {
	stats.Core.HPMax += delta.Core.HPMax
	stats.Core.ShieldMax += delta.Core.ShieldMax
	stats.Core.ShieldRegen += delta.Core.ShieldRegen
	stats.Core.EnergyMax += delta.Core.EnergyMax
	stats.Core.EnergyRegen += delta.Core.EnergyRegen
	stats.Core.Speed += delta.Core.Speed
	stats.Core.CargoCapacity += delta.Core.CargoCapacity

	stats.Combat.WeaponDamage += delta.Combat.WeaponDamage
	stats.Combat.WeaponRange += delta.Combat.WeaponRange
	stats.Combat.WeaponCooldown += delta.Combat.WeaponCooldown
	stats.Combat.Accuracy += delta.Combat.Accuracy
	stats.Combat.Tracking += delta.Combat.Tracking
	stats.Combat.Evasion += delta.Combat.Evasion
	stats.Combat.Penetration += delta.Combat.Penetration
	stats.Combat.CritChance += delta.Combat.CritChance
	stats.Combat.CritMultiplier += delta.Combat.CritMultiplier
	stats.Combat.ResistLaser += delta.Combat.ResistLaser
	stats.Combat.ResistExplosive += delta.Combat.ResistExplosive
	stats.Combat.ResistKinetic += delta.Combat.ResistKinetic

	stats.Exploration.RadarRange += delta.Exploration.RadarRange
	stats.Exploration.ScanPower += delta.Exploration.ScanPower
	stats.Exploration.ScanRadius += delta.Exploration.ScanRadius
	stats.Exploration.ScanInterval += delta.Exploration.ScanInterval
	stats.Exploration.SignatureRadius += delta.Exploration.SignatureRadius
	stats.Exploration.StealthStrength += delta.Exploration.StealthStrength
	stats.Exploration.JammerStrength += delta.Exploration.JammerStrength

	stats.Economy.CraftSpeed += delta.Economy.CraftSpeed
	stats.Economy.RouteLossReduction += delta.Economy.RouteLossReduction
	stats.Economy.ProductionBonus += delta.Economy.ProductionBonus
	stats.Economy.MarketFeeReduction += delta.Economy.MarketFeeReduction
}

func percentMultiplier(delta float64) float64 {
	return 1 + delta
}

func clampNonNegative(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	return value
}

func clampUnit(value float64) float64 {
	value = clampNonNegative(value)
	if value > 1 {
		return 1
	}
	return value
}
