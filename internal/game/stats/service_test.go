package stats

import (
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestStatServiceGetEffectiveStatsAggregatesCallerProvidedInputs(t *testing.T) {
	start := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	service := NewStatService(newTestClock(start), NewInMemoryStatSnapshotStore(), NewInMemoryActiveStatCache())

	snapshot, err := service.GetEffectiveStats(GetEffectiveStatsInput{
		PlayerID: foundation.PlayerID("player_1"),
		ShipID:   foundation.ShipID("ship_1"),
		BaseShip: EffectiveStats{
			Core: CoreStats{
				Speed:         100,
				CargoCapacity: 50,
			},
			Combat: CombatStats{
				WeaponDamage: 100,
			},
		},
		Modules: []ModuleModifier{
			{
				SourceID: "laser_alpha_1",
				Flat: FlatStats{
					Combat: CombatStats{
						WeaponDamage: 10,
					},
				},
				Percent: PercentStats{
					Combat: CombatStats{
						WeaponDamage: 0.10,
					},
				},
			},
		},
		FlatPassives: []FlatModifier{
			{
				Source:   ModifierSourcePassive,
				SourceID: "pilot_damage_1",
				Stats: FlatStats{
					Combat: CombatStats{
						WeaponDamage: 5,
					},
				},
			},
		},
		RoleBonuses: []FlatModifier{
			{
				Source:   ModifierSourceRole,
				SourceID: "scout_level_2",
				Stats: FlatStats{
					Core: CoreStats{
						Speed: 20,
					},
				},
			},
		},
		PercentPassives: []PercentModifier{
			{
				Source:   ModifierSourcePassive,
				SourceID: "pilot_speed_percent_1",
				Stats: PercentStats{
					Core: CoreStats{
						Speed: 0.25,
					},
				},
			},
		},
		TemporaryModifiers: []TemporaryModifier{
			{
				Source:   ModifierSourceDebuff,
				SourceID: "damage_dampener",
				Flat: FlatStats{
					Core: CoreStats{
						CargoCapacity: 10,
					},
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
	})
	if err != nil {
		t.Fatalf("GetEffectiveStats() error = %v", err)
	}

	if snapshot.Version != SnapshotVersion(1) {
		t.Fatalf("Version = %d, want 1", snapshot.Version)
	}
	if !snapshot.CreatedAt.Equal(start) {
		t.Fatalf("CreatedAt = %s, want %s", snapshot.CreatedAt, start)
	}
	assertFloatEqual(t, snapshot.Stats.Combat.WeaponDamage, 60.75)
	assertFloatEqual(t, snapshot.Stats.Core.Speed, 150)
	assertFloatEqual(t, snapshot.Stats.Core.CargoCapacity, 60)
}

func TestStatServiceUsesCachedSnapshotUntilInvalidated(t *testing.T) {
	start := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	clock := newTestClock(start)
	service := NewStatService(clock, NewInMemoryStatSnapshotStore(), NewInMemoryActiveStatCache())
	playerID := foundation.PlayerID("player_1")
	shipID := foundation.ShipID("ship_1")

	first, err := service.GetEffectiveStats(GetEffectiveStatsInput{
		PlayerID: playerID,
		ShipID:   shipID,
		BaseShip: EffectiveStats{
			Core: CoreStats{
				CargoCapacity: 100,
			},
		},
	})
	if err != nil {
		t.Fatalf("first GetEffectiveStats() error = %v", err)
	}

	clock.Advance(time.Minute)
	cached, err := service.GetEffectiveStats(GetEffectiveStatsInput{
		PlayerID: playerID,
		ShipID:   shipID,
		BaseShip: EffectiveStats{
			Core: CoreStats{
				CargoCapacity: 200,
			},
		},
	})
	if err != nil {
		t.Fatalf("cached GetEffectiveStats() error = %v", err)
	}
	if cached.Version != first.Version {
		t.Fatalf("cached Version = %d, want %d", cached.Version, first.Version)
	}
	assertFloatEqual(t, cached.Stats.Core.CargoCapacity, 100)
	if !cached.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("cached CreatedAt = %s, want %s", cached.CreatedAt, first.CreatedAt)
	}

	if err := service.InvalidateStats(InvalidateStatsInput{
		PlayerID: playerID,
		ShipID:   shipID,
		Reason:   InvalidationReasonModuleEquipped,
	}); err != nil {
		t.Fatalf("InvalidateStats() error = %v", err)
	}
	clock.Advance(time.Minute)
	recalculated, err := service.GetEffectiveStats(GetEffectiveStatsInput{
		PlayerID: playerID,
		ShipID:   shipID,
		BaseShip: EffectiveStats{
			Core: CoreStats{
				CargoCapacity: 200,
			},
		},
	})
	if err != nil {
		t.Fatalf("recalculated GetEffectiveStats() error = %v", err)
	}
	if recalculated.Version != SnapshotVersion(2) {
		t.Fatalf("recalculated Version = %d, want 2", recalculated.Version)
	}
	assertFloatEqual(t, recalculated.Stats.Core.CargoCapacity, 200)
}

func TestStatServiceRecalculatesWhenStoredSnapshotIsInvalidated(t *testing.T) {
	start := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	clock := newTestClock(start)
	store := NewInMemoryStatSnapshotStore()
	service := NewStatService(clock, store, NewInMemoryActiveStatCache())
	playerID := foundation.PlayerID("player_1")
	shipID := foundation.ShipID("ship_1")

	first, err := service.GetEffectiveStats(GetEffectiveStatsInput{
		PlayerID: playerID,
		ShipID:   shipID,
		BaseShip: EffectiveStats{
			Core: CoreStats{
				Speed: 100,
			},
		},
	})
	if err != nil {
		t.Fatalf("first GetEffectiveStats() error = %v", err)
	}
	if err := store.SaveSnapshot(first.Invalidate(clock.Advance(time.Minute))); err != nil {
		t.Fatalf("SaveSnapshot(invalidated) error = %v", err)
	}

	recalculated, err := service.GetEffectiveStats(GetEffectiveStatsInput{
		PlayerID: playerID,
		ShipID:   shipID,
		BaseShip: EffectiveStats{
			Core: CoreStats{
				Speed: 250,
			},
		},
	})
	if err != nil {
		t.Fatalf("recalculated GetEffectiveStats() error = %v", err)
	}
	if recalculated.Version != SnapshotVersion(2) {
		t.Fatalf("recalculated Version = %d, want 2", recalculated.Version)
	}
	assertFloatEqual(t, recalculated.Stats.Core.Speed, 250)
}

func TestStatServiceExcludesBrokenModuleModifiers(t *testing.T) {
	start := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	service := NewStatService(newTestClock(start), NewInMemoryStatSnapshotStore(), NewInMemoryActiveStatCache())

	snapshot, err := service.GetEffectiveStats(GetEffectiveStatsInput{
		PlayerID: foundation.PlayerID("player_1"),
		ShipID:   foundation.ShipID("ship_1"),
		BaseShip: EffectiveStats{
			Core: CoreStats{
				Speed: 100,
			},
			Combat: CombatStats{
				WeaponDamage: 10,
			},
		},
		Modules: []ModuleModifier{
			{
				SourceID: "laser_working",
				Flat: FlatStats{
					Combat: CombatStats{
						WeaponDamage: 5,
					},
				},
			},
			{
				SourceID: "laser_broken",
				Broken:   true,
				Flat: FlatStats{
					Combat: CombatStats{
						WeaponDamage: 1000,
					},
				},
				Percent: PercentStats{
					Core: CoreStats{
						Speed: 1,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("GetEffectiveStats() error = %v", err)
	}

	assertFloatEqual(t, snapshot.Stats.Combat.WeaponDamage, 15)
	assertFloatEqual(t, snapshot.Stats.Core.Speed, 100)
}

func TestInMemoryActiveStatCacheKeysByPlayerShipAndVersion(t *testing.T) {
	cache := NewInMemoryActiveStatCache()
	snapshot := NewStatSnapshot(
		foundation.PlayerID("player_1"),
		foundation.ShipID("ship_1"),
		SnapshotVersion(1),
		EffectiveStats{Core: CoreStats{CargoCapacity: 100}},
		time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
	)

	cache.Put(snapshot)

	if got, ok := cache.Get(StatSnapshotKey{
		PlayerID: foundation.PlayerID("player_1"),
		ShipID:   foundation.ShipID("ship_1"),
		Version:  SnapshotVersion(1),
	}); !ok {
		t.Fatal("Get(exact key) ok = false, want true")
	} else {
		assertFloatEqual(t, got.Stats.Core.CargoCapacity, 100)
	}

	for _, key := range []StatSnapshotKey{
		{PlayerID: foundation.PlayerID("player_1"), ShipID: foundation.ShipID("ship_1"), Version: SnapshotVersion(2)},
		{PlayerID: foundation.PlayerID("player_1"), ShipID: foundation.ShipID("ship_2"), Version: SnapshotVersion(1)},
		{PlayerID: foundation.PlayerID("player_2"), ShipID: foundation.ShipID("ship_1"), Version: SnapshotVersion(1)},
	} {
		if _, ok := cache.Get(key); ok {
			t.Fatalf("Get(%+v) ok = true, want false", key)
		}
	}
}

type testClock struct {
	now time.Time
}

func newTestClock(now time.Time) *testClock {
	return &testClock{now: now}
}

func (clock *testClock) Now() time.Time {
	return clock.now
}

func (clock *testClock) Advance(duration time.Duration) time.Time {
	clock.now = clock.now.Add(duration)
	return clock.now
}
