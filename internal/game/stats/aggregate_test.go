package stats

import (
	"math"
	"testing"
)

func TestAggregateStatsAppliesDocumentedOrder(t *testing.T) {
	input := AggregationInput{
		BaseShip: EffectiveStats{
			Core: CoreStats{
				Speed:         100,
				CargoCapacity: 50,
			},
			Combat: CombatStats{
				WeaponDamage:     100,
				WeaponEnergyCost: 20,
			},
		},
		FlatModules: []FlatModifier{
			{
				Source: ModifierSourceModule,
				Stats: FlatStats{
					Core: CoreStats{
						CargoCapacity: 10,
					},
					Combat: CombatStats{
						WeaponDamage:     10,
						WeaponEnergyCost: -2,
					},
				},
			},
		},
		FlatPassives: []FlatModifier{
			{
				Source: ModifierSourcePassive,
				Stats: FlatStats{
					Combat: CombatStats{
						WeaponDamage: 1,
					},
				},
			},
		},
		RoleBonuses: []FlatModifier{
			{
				Source: ModifierSourceRole,
				Stats: FlatStats{
					Core: CoreStats{
						Speed: 5,
					},
					Combat: CombatStats{
						WeaponDamage:     4,
						WeaponEnergyCost: -1,
					},
				},
			},
		},
		PercentModules: []PercentModifier{
			{
				Source: ModifierSourceModule,
				Stats: PercentStats{
					Combat: CombatStats{
						WeaponDamage:     0.10,
						WeaponEnergyCost: -0.10,
					},
				},
			},
		},
		PercentPassives: []PercentModifier{
			{
				Source: ModifierSourcePassive,
				Stats: PercentStats{
					Core: CoreStats{
						Speed: 0.20,
					},
					Combat: CombatStats{
						WeaponDamage: 0.20,
					},
				},
			},
		},
		TemporaryModifiers: []TemporaryModifier{
			{
				Source: ModifierSourceDebuff,
				Flat: FlatStats{
					Combat: CombatStats{
						WeaponDamage: -5,
					},
				},
				Percent: PercentStats{
					Combat: CombatStats{
						WeaponDamage: -0.50,
					},
				},
			},
		},
	}

	got := AggregateStats(input)

	assertFloatEqual(t, got.Combat.WeaponDamage, 73.4)
	assertFloatEqual(t, got.Combat.WeaponEnergyCost, 15.3)
	assertFloatEqual(t, got.Core.Speed, 126)
	assertFloatEqual(t, got.Core.CargoCapacity, 60)
}

func TestAggregateStatsClampsEffectiveStats(t *testing.T) {
	input := AggregationInput{
		BaseShip: EffectiveStats{
			Core: CoreStats{
				Speed:         -1,
				CargoCapacity: math.Inf(1),
			},
			Combat: CombatStats{
				Accuracy:         1.5,
				CritChance:       -0.5,
				ResistLaser:      2,
				WeaponDamage:     10,
				WeaponCooldown:   3,
				WeaponEnergyCost: math.Inf(1),
				ResistExplosive:  -1,
			},
			Exploration: ExplorationStats{
				DetectionPower:        -1,
				JammerResistance:      math.Inf(1),
				StealthDetectionBonus: -2,
				ScanSuccessBonus:      2,
				FogRevealRadius:       -1,
			},
			Economy: EconomyStats{
				ConstructionSpeed:        -1,
				MarketFeeReduction:       4,
				RouteLossReduction:       -2,
				RouteCargoCapacityBonus:  -3,
				CraftMaterialRefundBonus: 2,
			},
		},
		PercentModules: []PercentModifier{
			{
				Source: ModifierSourceModule,
				Stats: PercentStats{
					Combat: CombatStats{
						WeaponDamage: -2,
					},
				},
			},
		},
	}

	got := AggregateStats(input)

	assertFloatEqual(t, got.Core.Speed, 0)
	assertFloatEqual(t, got.Core.CargoCapacity, 0)
	assertFloatEqual(t, got.Combat.WeaponDamage, 0)
	assertFloatEqual(t, got.Combat.WeaponEnergyCost, 0)
	assertFloatEqual(t, got.Combat.Accuracy, 1)
	assertFloatEqual(t, got.Combat.CritChance, 0)
	assertFloatEqual(t, got.Combat.ResistLaser, 1)
	assertFloatEqual(t, got.Combat.ResistExplosive, 0)
	assertFloatEqual(t, got.Exploration.DetectionPower, 0)
	assertFloatEqual(t, got.Exploration.JammerResistance, 0)
	assertFloatEqual(t, got.Exploration.StealthDetectionBonus, 0)
	assertFloatEqual(t, got.Exploration.ScanSuccessBonus, 1)
	assertFloatEqual(t, got.Exploration.FogRevealRadius, 0)
	assertFloatEqual(t, got.Economy.ConstructionSpeed, 0)
	assertFloatEqual(t, got.Economy.MarketFeeReduction, 1)
	assertFloatEqual(t, got.Economy.RouteLossReduction, 0)
	assertFloatEqual(t, got.Economy.RouteCargoCapacityBonus, 0)
	assertFloatEqual(t, got.Economy.CraftMaterialRefundBonus, 1)
}

func TestAggregateStatsAppliesDetectionExplorationFields(t *testing.T) {
	input := AggregationInput{
		BaseShip: EffectiveStats{
			Exploration: ExplorationStats{
				DetectionPower:        10,
				JammerResistance:      4,
				StealthDetectionBonus: 2,
			},
		},
		FlatModules: []FlatModifier{
			{
				Source: ModifierSourceModule,
				Stats: FlatStats{Exploration: ExplorationStats{
					DetectionPower:        5,
					JammerResistance:      2,
					StealthDetectionBonus: 1,
				}},
			},
		},
		PercentModules: []PercentModifier{
			{
				Source: ModifierSourceModule,
				Stats: PercentStats{Exploration: ExplorationStats{
					DetectionPower:        0.10,
					JammerResistance:      0.50,
					StealthDetectionBonus: 1,
				}},
			},
		},
	}

	got := AggregateStats(input)

	assertFloatEqual(t, got.Exploration.DetectionPower, 16.5)
	assertFloatEqual(t, got.Exploration.JammerResistance, 9)
	assertFloatEqual(t, got.Exploration.StealthDetectionBonus, 6)
}

func TestAggregateStatsDoesNotMutateInput(t *testing.T) {
	input := AggregationInput{
		BaseShip: EffectiveStats{
			Combat: CombatStats{
				WeaponDamage: 10,
			},
		},
		FlatModules: []FlatModifier{
			{
				Source: ModifierSourceModule,
				Stats: FlatStats{
					Combat: CombatStats{
						WeaponDamage: 5,
					},
				},
			},
		},
	}

	_ = AggregateStats(input)

	assertFloatEqual(t, input.BaseShip.Combat.WeaponDamage, 10)
	assertFloatEqual(t, EffectiveStats(input.FlatModules[0].Stats).Combat.WeaponDamage, 5)
}

func TestStatAggregationServiceRecalculatesStats(t *testing.T) {
	service := NewStatAggregationService()
	input := AggregationInput{
		BaseShip: EffectiveStats{
			Core: CoreStats{
				CargoCapacity: 100,
			},
		},
		FlatModules: []FlatModifier{
			{
				Source: ModifierSourceModule,
				Stats: FlatStats{
					Core: CoreStats{
						CargoCapacity: 25,
					},
				},
			},
		},
	}

	got := service.RecalculateStats(input)

	assertFloatEqual(t, got.Core.CargoCapacity, 125)
}

func assertFloatEqual(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("got %v, want %v", got, want)
	}
}
