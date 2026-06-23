# Automation Routes

Date: 2026-06-17

## Purpose

Bu modül planetler ve storage arasında otomatik resource akışını yönetir.

Fantasy:

```text
Planet X -> Planet Y
40 Refined Alloy / h
Risk 8%
Energy 12 / h
```

Star map üzerinde üretim ağı çizgilerle görünür.

## Owns

```text
AutomationRouteService
RouteSettlementService
```

## Does Not Own

- Planet production
- Storage primitive
- Physical convoy combat, future
- Planet ownership claim

## MVP Route Model

İlk sürümde route fiziksel convoy değildir.

Sanal transfer:

```text
every route settlement:
  source storage'dan resource al
  risk/loss hesapla
  destination storage'a ekle
```

Uzun vadede:

- cargo drone
- scanner ile detect
- PvP saldırı
- escort contract
- insurance

## Route Data

```text
route_id
owner_player_id
source_planet_id
destination_type
destination_id
resource_id
amount_per_hour
energy_cost_per_hour
loss_chance
loss_min_percent
loss_max_percent
enabled
last_calculated_at
created_at
updated_at
```

Destination type:

```text
planet
storage
station
```

## Create Route Validation

```text
source planet owned by player
destination owned/accessible
resource routeable
amount_per_hour positive
owned route count below server-owned route-slot capacity
enabled route energy upkeep fits source planet energy budget
player rank/building requirement met
source has route building or automation unlocked
distance within allowed range or wormhole link exists
```

Go-ish:

```go
func CanCreateRoute(player PlayerID, req CreateRouteRequest, state RouteCheckState) error {
	if !state.SourceOwnedBy(player) {
		return ErrSourceNotOwned
	}
	if !state.DestinationAccessibleBy(player) {
		return ErrDestinationNotAccessible
	}
	if req.AmountPerHour <= 0 {
		return ErrInvalidRate
	}
	if !state.ResourceRouteable(req.ResourceID) {
		return ErrResourceNotRouteable
	}
	if state.CurrentOwnedRouteCount() >= state.MaxOwnedRouteCount() {
		return ErrRouteCapacityExceeded
	}
	if !state.SourcePlanetEnergyBudgetCanReserve(req.NewEnergyCostPerHour) {
		return ErrRouteEnergyUnavailable
	}
	return nil
}
```

Route capacity is server-owned. The browser may display returned route counts or
locked states, but it must not send current/max route count as trusted truth.
The current runtime MVP uses a fixed per-player route-slot cap; progression,
buildings, modules, or premium bonuses can later feed the same policy boundary.

Route upkeep is also server-owned. Enabled routes reserve their
`energy_cost_per_hour` against the source planet production state, and
production settlement treats `energy_reserved_per_hour` as already consumed
before running buildings. Create/enable reserve energy, disable releases it
after settlement, and update applies only the enabled-route energy delta so
same-cost edits remain allowed at capacity.

Durable route mutation plans carry the changed source production state whenever
route energy reservation changes. Pure settlement cursor commits omit it because
they do not change route upkeep reservation.

## Risk Formula v0

Inputs:

- distance
- source region risk
- destination region risk
- route crosses deep space
- player construction/trade bonus
- route building/security upgrades

Simple:

```text
loss_chance = base_region_risk
            + distance_factor
            - route_security_bonus
            - player_bonus
clamp 0%..40%
```

MVP can precompute on create and recalc when route/buildings change.

## Route Settlement

```go
func SettleRoute(ctx context.Context, routeID RouteID, now time.Time) error {
	return db.Tx(ctx, func(tx Tx) error {
		route := tx.Routes().Lock(routeID)
		if !route.Enabled {
			return nil
		}
		elapsed := MinDuration(now.Sub(route.LastCalculatedAt), MaxRouteOfflineDuration)
		if elapsed <= 0 {
			return nil
		}

		wanted := int64(elapsed.Hours() * float64(route.AmountPerHour))
		source := tx.Storage().LockPlanet(route.SourcePlanetID)
		taken := source.RemoveUpTo(route.ResourceID, wanted)

		delivered := ApplyRouteLoss(taken, route.Risk, rng)
		dest := tx.Storage().Lock(route.DestinationType, route.DestinationID)
		added := dest.AddUpToCapacity(route.ResourceID, delivered)

		route.LastCalculatedAt = now
		tx.Routes().Save(route)
		tx.Events().Insert("route.transfer_settled", routeID, taken, added)
		return nil
	})
}
```

## Loss Model

Partial loss:

```go
func ApplyRouteLoss(amount int64, risk RouteRisk, rng RNG) int64 {
	if amount <= 0 {
		return 0
	}
	if rng.Float64() > risk.LossChance {
		return amount
	}
	lossPct := rng.Float64Range(risk.MinLossPct, risk.MaxLossPct)
	lost := int64(float64(amount) * lossPct)
	return amount - lost
}
```

This is less frustrating than full tick loss.

## Energy Cost

Options:

1. Deduct Energy Cell item from source.
2. Use planet energy budget.
3. Use route upkeep credits.

MVP:

```text
planet energy budget route upkeep
if insufficient energy, route disabled or scaled
```

## UI Data

Route graph needs:

```text
source name
destination name
resource
amount_per_hour
expected_delivered_per_hour
loss_chance
energy_cost_per_hour
enabled
storage bottleneck warnings
```

## Events Emitted

```text
route.created
route.updated
route.enabled
route.disabled
route.transfer_settled
route.transfer_lost
route.destination_full
route.source_empty
```

## Edge Cases

- Source storage empty.
- Destination storage full.
- Player loses source planet.
- Player loses destination planet.
- Route resource disabled by item definition change.
- Concurrent settlement from login and background worker.
- Route amount changed mid-period.

## Abuse Vectors

### Infinite Transfer Duplication

Risk:

- Route settlement repeats same elapsed time.

Defense:

- lock route
- update last_calculated_at in transaction
- idempotent settlement reference

### Destination Capacity Bypass

Risk:

- Transfer adds beyond storage cap.

Defense:

- lock destination storage
- add up to capacity

### Unauthorized Destination

Risk:

- Client creates route to another player's planet.

Defense:

- server ownership/access validation
- no trust in destination id

### Risk Avoidance By Toggling

Risk:

- Player toggles route around settlement to avoid loss.

Defense:

- last_calculated_at handling on disable/enable
- route changes settle previous period first
- minimum route interval

## Testing Checklist

- Create route ownership validation.
- Empty source transfers 0.
- Full destination clamps.
- Loss chance applies in range.
- Double settlement no duplicate.
- Disable/enable preserves timestamp correctly.
- Unauthorized destination fails.
- Route update settles old state first.

## Implementation Notes

MVP:

- virtual route
- partial loss
- source/destination storage lock
- star map route summary API
- local settlement event envelopes for route settled, route loss, source empty,
  and destination full conditions
- authenticated realtime gateway handlers now cover `route.create`,
  `route.update`, `route.enable`, `route.disable`, and `route.settle`; the
  settle gateway accepts only `route_id` or `{}` owner reconcile intent,
  derives owner from the session, returns safe settlement payloads, and emits
  owner-scoped `route.settled` plus route reconciliation events
- durable outbox/publisher remains a later persistence boundary
- durable DB route rows, row locks, and route/window idempotency references
  remain future persistence work
- no physical convoy yet
