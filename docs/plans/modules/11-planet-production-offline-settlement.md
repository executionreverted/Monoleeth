# Planet Production And Offline Settlement

Date: 2026-06-17

## Purpose

Bu modül kolonize edilmiş planetlerin resource üretimini ve oyuncu offline iken biriken üretimin hesaplanmasını yönetir.

Core idea:

```text
Online iken production tick işler.
Offline iken sürekli tick koşmaz.
Oyuncu login/inspection yaptığında elapsed time settlement yapılır.
Storage capacity üretimi sınırlar.
```

## Owns

```text
PlanetProductionService
OfflineSettlementService
```

## Does Not Own

- Planet discovery/claim
- Route transfer
- Inventory primitive
- Building construction validation
- Combat/PvP

## Production Inputs

Planet production etkileyen şeyler:

- planet type
- planet level
- biome
- buildings
- building levels
- energy availability
- input materials
- storage capacity
- owner bonuses
- live ops modifiers

## Planet Storage

```text
planet_id
item_id
quantity
capacity_used
capacity_max
```

Capacity item weight/unit ile hesaplanabilir.

MVP:

```text
1 item quantity = 1 storage unit unless item definition says otherwise
```

## Building Production Definition

```text
building_type
level
outputs[]
inputs[]
energy_cost_per_hour
base_rate_per_hour
requirements
```

Example:

```json
{
  "building_type": "crystal_extractor",
  "level": 2,
  "outputs": [{"item_id": "crystal_fragment", "amount_per_hour": 40}],
  "energy_cost_per_hour": 8
}
```

Refinery:

```json
{
  "building_type": "alloy_foundry",
  "level": 1,
  "inputs": [{"item_id": "iron_ore", "amount_per_hour": 30}],
  "outputs": [{"item_id": "refined_alloy", "amount_per_hour": 10}],
  "energy_cost_per_hour": 5
}
```

## Production State

```text
planet_id
last_calculated_at
production_enabled
priority_json
updated_at
```

Each planet has one settlement timestamp.

## Settlement Algorithm

```go
func SettlePlanetProduction(ctx context.Context, planetID PlanetID, now time.Time) error {
	return db.Tx(ctx, func(tx Tx) error {
		state := tx.Production().LockPlanetState(planetID)
		elapsed := now.Sub(state.LastCalculatedAt)
		if elapsed <= 0 {
			return nil
		}
		elapsed = MinDuration(elapsed, MaxOfflineDuration)

		buildings := tx.Buildings().ListActive(planetID)
		storage := tx.Storage().LockPlanetStorage(planetID)

		for _, b := range buildings {
			result := CalculateBuildingOutput(b, elapsed, storage)
			ApplyProductionResult(storage, result)
		}

		state.LastCalculatedAt = now
		tx.Production().Save(state)
		tx.Storage().Save(storage)
		return nil
	})
}
```

## Storage Capacity Clamp

```go
func ApplyOutput(storage *Storage, item ItemID, amount int64) int64 {
	free := storage.FreeUnits()
	add := Min(amount, free)
	storage.Add(item, add)
	return add
}
```

If full:

```text
production stops/lost for that elapsed portion
```

MVP:

- no backpressure queue
- storage full means output not produced

## Input Consumption

For input-based buildings:

```go
func CalculateCraftLikeProduction(b Building, elapsed time.Duration, storage Storage) ProductionResult {
	cycles := elapsed.Hours() * b.RatePerHour
	maxByInputs := storage.MaxCyclesFromInputs(b.Inputs)
	actualCycles := MinFloat(cycles, maxByInputs)
	return b.ResultForCycles(actualCycles)
}
```

Inputs consumed proportional to actual output.

## Energy

Two models:

1. Energy as resource item.
2. Energy as planet stat/rate.

MVP recommendation:

```text
Energy as planet production stat + optional Energy Cell item for recipes.
```

If insufficient energy:

```text
scale production down proportionally or priority order.
```

MVP simpler:

```text
building disabled if planet energy budget insufficient.
```

## Online Tick

Online players:

```text
settle every 60s or 300s
```

No need per-second production.

Client can display estimated progress:

```text
server sends rate + last_calculated_at
client estimates UI only
server truth on settlement
```

## Offline Settlement

On login:

```text
load owned planets
settle each planet up to max_offline_hours
settle routes after production or in deterministic order
return summary
```

Max offline:

```text
24h or 72h
```

Storage cap already limits output.

## Settlement Order

Important if routes exist:

MVP deterministic order:

```text
1. settle source planet production
2. settle route transfer
3. settle destination planet production only when inspected or next login
```

Better later:

```text
global offline settlement planner
```

## Events Emitted

```text
planet.production_settled
planet.storage_full
planet.energy_insufficient
planet.building_produced
offline.settlement_completed
```

## Edge Cases

- last_calculated_at in future due to clock issue.
- building upgraded during offline period.
- storage fills halfway through elapsed period.
- input resource runs out halfway.
- planet ownership changes.
- production definition changes while offline.
- settlement runs twice concurrently.

## Abuse Vectors

### Time Skip Abuse

Risk:

- Client sends fake offline duration.

Defense:

- server timestamps only
- DB last_calculated_at
- max offline cap

### Duplicate Settlement

Risk:

- Login + planet inspection settle same planet twice.

Defense:

- lock production state
- update last_calculated_at in transaction

### Storage Overflow

Risk:

- Production adds beyond capacity due to race.

Defense:

- lock storage rows
- capacity clamp inside transaction

### Ownership Race

Risk:

- Planet transferred/raided while offline settlement claims output.

Defense:

- settle ownership intervals later
- MVP lock planet owner and disallow transfer during settlement

## Testing Checklist

- 1 hour production output correct.
- Storage cap clamps output.
- Input shortage reduces output.
- Offline cap applies.
- Double settlement does not duplicate.
- Future timestamp handled safely.
- Energy insufficient disables or scales.
- Login settlement summary correct.

## Implementation Notes

MVP:

- one timestamp per planet
- simple building production
- storage cap
- no second-by-second jobs for offline players
- settlement on login/inspection

