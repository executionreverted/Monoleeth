package stats

import (
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestStatServiceGetEffectiveStatsAggregatesProviderInputs(t *testing.T) {
	start := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	subject := NewStatSubject("player_1", "ship_1")
	service := newTestStatService(t, newTestClock(start), NewInMemoryStatSnapshotStore(), NewInMemoryActiveStatCache(), StaticStatInputProvider{
		subject: {
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
		},
	})

	snapshot, err := service.GetEffectiveStats(subject)
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
	playerID := foundation.PlayerID("player_1")
	shipID := foundation.ShipID("ship_1")
	subject := NewStatSubject(playerID, shipID)
	inputs := StaticStatInputProvider{
		subject: {
			BaseShip: EffectiveStats{
				Core: CoreStats{
					CargoCapacity: 100,
				},
			},
		},
	}
	service := newTestStatService(t, clock, NewInMemoryStatSnapshotStore(), NewInMemoryActiveStatCache(), inputs)

	first, err := service.GetEffectiveStats(subject)
	if err != nil {
		t.Fatalf("first GetEffectiveStats() error = %v", err)
	}

	clock.Advance(time.Minute)
	inputs[subject] = StatBuildInput{
		BaseShip: EffectiveStats{
			Core: CoreStats{
				CargoCapacity: 200,
			},
		},
	}
	cached, err := service.GetEffectiveStats(subject)
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
	recalculated, err := service.GetEffectiveStats(subject)
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
	playerID := foundation.PlayerID("player_1")
	shipID := foundation.ShipID("ship_1")
	subject := NewStatSubject(playerID, shipID)
	inputs := StaticStatInputProvider{
		subject: {
			BaseShip: EffectiveStats{
				Core: CoreStats{
					Speed: 100,
				},
			},
		},
	}
	service := newTestStatService(t, clock, store, NewInMemoryActiveStatCache(), inputs)

	first, err := service.GetEffectiveStats(subject)
	if err != nil {
		t.Fatalf("first GetEffectiveStats() error = %v", err)
	}
	if err := store.SaveSnapshot(first.Invalidate(clock.Advance(time.Minute))); err != nil {
		t.Fatalf("SaveSnapshot(invalidated) error = %v", err)
	}
	inputs[subject] = StatBuildInput{
		BaseShip: EffectiveStats{
			Core: CoreStats{
				Speed: 250,
			},
		},
	}

	recalculated, err := service.GetEffectiveStats(subject)
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
	subject := NewStatSubject("player_1", "ship_1")
	service := newTestStatService(t, newTestClock(start), NewInMemoryStatSnapshotStore(), NewInMemoryActiveStatCache(), StaticStatInputProvider{
		subject: {
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
		},
	})

	snapshot, err := service.GetEffectiveStats(subject)
	if err != nil {
		t.Fatalf("GetEffectiveStats() error = %v", err)
	}

	assertFloatEqual(t, snapshot.Stats.Combat.WeaponDamage, 15)
	assertFloatEqual(t, snapshot.Stats.Core.Speed, 100)
}

func TestNewStatServiceRejectsNilInputProvider(t *testing.T) {
	_, err := NewStatService(newTestClock(time.Now()), NewInMemoryStatSnapshotStore(), NewInMemoryActiveStatCache(), nil)
	if err == nil {
		t.Fatal("NewStatService nil provider error = nil, want error")
	}
}

func TestStatServiceReturnsMissingProviderInputWithoutMutatingState(t *testing.T) {
	start := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	store := NewInMemoryStatSnapshotStore()
	subject := NewStatSubject("player_1", "ship_1")
	service := newTestStatService(t, newTestClock(start), store, NewInMemoryActiveStatCache(), StaticStatInputProvider{})

	_, err := service.GetEffectiveStats(subject)
	if err == nil {
		t.Fatal("GetEffectiveStats missing provider input error = nil, want error")
	}
	if _, ok := store.GetInvalidationState(subject.PlayerID, subject.ShipID); ok {
		t.Fatal("invalidation state exists after failed provider lookup, want no mutation")
	}
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

func newTestStatService(
	t *testing.T,
	clock foundation.Clock,
	store StatSnapshotStore,
	cache ActiveStatCache,
	inputs StatInputProvider,
) *StatService {
	t.Helper()

	service, err := NewStatService(clock, store, cache, inputs)
	if err != nil {
		t.Fatalf("NewStatService error = %v, want nil", err)
	}
	return service
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
