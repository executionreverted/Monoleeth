# Phase 12 — DarkOrbit Flavor (Drones, P.E.T., Ammo, Honor)

## Status
- State: Not started
- Wave: 6
- Depends on: P07 (equipment/stats correct), P11 (endgame for rewards)
- Unlocks: DarkOrbit parity feel

## Goal
Add iconic DarkOrbit-style depth on top of a correct equipment system:
drones-lite, P.E.T.-lite, ammo/consumables, and a basic honor/leaderboard.

## Why (report refs)
- Feature-gap §7 P1, §8.1, §11: drones/P.E.T./ammo/honor are the flavor parity gaps.

## Scope (each sub-feature is independently shippable)
- Drones-lite: extra equip slots + levels (no formations first).
- P.E.T.-lite: server-owned companion with auto-loot gear (strict limits).
- Ammo/consumables: server-owned ammo affecting damage; quickslot consumption.
- Honor/leaderboard: weekly contribution ranking.

## Out Of Scope
- Drone formations/designs, P.E.T. protocol depth, full company/faction systems.

## Tasks
- [ ] `[P:wave6/lane-A]` Add drone rows + slots + equip rules + XP/level (reuse module/stat path).
- [ ] `[P:wave6/lane-B]` Add P.E.T.-lite entity: server-owned auto-loot within radar, fuel/upkeep, anti-bot rate limits.
- [ ] `[P:wave6/lane-C]` Add ammo/consumable items: server-validated consumption changes damage; quickslot UI.
- [ ] `[P:wave6/lane-D]` Add honor/contribution accrual + weekly leaderboard query.

## Server Ownership
- Drone stats, P.E.T. actions, ammo effects, honor scores are server-owned; client sends intent only.
- P.E.T. auto actions must not behave like a client bot; all decisions server-side and rate-limited.

## Smoke Tests (one assertion each)
- [ ] Equipping a drone module changes effective stats.
- [ ] P.E.T. auto-loot picks one eligible drop within radar and respects cooldown.
- [ ] Firing with ammo selected consumes one ammo unit and applies its damage modifier.
- [ ] Honor accrues from a kill and appears in the weekly leaderboard.

## Done Criteria
- [ ] At least drones-lite + ammo + honor shipped; P.E.T.-lite optional-gated by anti-abuse review.

## Verification
```bash
go test ./internal/game/drones/... ./internal/game/pet/... ./internal/game/combat/... ./internal/game/server/... -run 'Drone|Pet|Ammo|Honor' -count=1
go test ./... && cd client && npm --cache /tmp/gameproject-npm-cache run check
```
