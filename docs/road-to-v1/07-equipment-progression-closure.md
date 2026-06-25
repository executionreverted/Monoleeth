# Phase 07 — Equipment & Progression Closure

## Status
- State: In progress
- Wave: 3
- Depends on: P01, P02
- Unlocks: P11, P12

## Goal
Make upgrades actually matter: inventory move, skill unlock, effective-stat and
cargo recalculation after equipment changes, and a real first non-starter ship path.

## Why (report refs)
- Feature-gap §4.5, §6.2: open `inventory.move`, `progression.unlock_skill`,
  stat/cargo recalculation, ship acquisition.
- Gameplay gap audit: cargo module does not increase effective cargo.

## Scope
- `inventory.move` and `progression.unlock_skill` authenticated commands.
- Runtime `StatService` as authoritative source for active stats + cargo capacity.
- One real non-starter ship acquisition (shop/craft/quest), with reconciliation.

## Out Of Scope
- Drones/P.E.T./two-config loadouts (P12).

## Tasks
- [ ] `[P:wave3/lane-A]` Add `inventory.move` (ownership/amount/capacity checks, idempotent).
- [x] `[P:wave3/lane-A]` Add `progression.unlock_skill` (point check, prereqs, consume once).
- [ ] `[P:wave3/lane-B]` Wire `StatService` into runtime; recalc on equip/unequip/ship activate.
- [ ] `[P:wave3/lane-B]` Make effective cargo capacity authoritative; emit `stats.updated` + cargo/inventory/hangar/loadout snapshots together.
- [ ] `[P:wave3/lane-C]` Define first buyable/craftable non-starter ship with server price/rank; grant to hangar with events.
- [ ] `[P:wave3/lane-C]` Add skill tree UI panel + unlock action (client).

## Server Ownership
- XP, points, stats, cargo capacity, ownership are server-owned; client sends intent only.

## Smoke Tests (one assertion each)
- [ ] `inventory.move` rejects unowned/negative/over-capacity amount.
- [ ] Duplicate `inventory.move` cannot duplicate a stack.
- [x] Skill unlock consumes exactly one point.
- [x] Duplicate skill unlock does not double-spend.
- [ ] Equipping a cargo module increases server + visible cargo capacity.
- [ ] Buying/crafting the first non-starter ship adds it to hangar once.

## Done Criteria
- [ ] Equipment/skills measurably change effective stats and cargo.
- [ ] Phase 06 open progression/inventory items closed.

## Verification
```bash
go test ./internal/game/stats/... ./internal/game/modules/... ./internal/game/progression/... ./internal/game/ships/... ./internal/game/server/... -run 'InventoryMove|Skill|Stat|Cargo|ShipAcquire' -count=1
go test ./... && cd client && npm --cache /tmp/gameproject-npm-cache run check
```
