# Phase 01 — Persistence Foundation

## Status
- State: In progress
- Wave: 1
- Depends on: none
- Unlocks: P02, P05, P07, P08, P09, P10

## Goal
Move core player/account/session/progression state from in-memory to durable
PostgreSQL behind repository interfaces, with restart recovery. Reuse the existing
`contentdb` Postgres setup; do not add a second DB stack.

## Why (report refs)
- Code review §1, §9: most game state is in-memory; restart loses progress.
- Feature-gap §6.1, §7 P0: durable persistence is the top production blocker.

## Scope
- Durable: account, session, player profile, wallet, inventory, progression, hangar, loadout.
- Repository interfaces in domain packages; pgx/SQL only under a db adapter package.
- Migrations + restart-recovery load path.

## Out Of Scope
- Market/auction/planet/route durability (P02, P08).
- Redis/NATS (later).

## Tasks
- [x] `[P:wave1/lane-A]` Add `internal/game/persistence` (or reuse `contentdb` pattern) migration set for player-state tables.
- [ ] `[P:wave1/lane-A]` Define repository interfaces in `auth`, `economy`, `progression`, `ships`, `modules` (no pgx imports in domain).
  - [x] `ships`: `HangarService` depends on `HangarStore`; in-memory store implements the interface.
  - [x] `modules`: `LoadoutService` depends on split loadout repository, active ship reader, module item reader, equipped module reader, and module item mutator interfaces; in-memory store implements them.
- [x] `[P:wave1/lane-A]` Implement pgx-backed repos in the db adapter package for auth account/player/session, wallet balance, and stackable inventory state.
  - [x] `ships`: contentdb migration + `HangarStore` adapter persists `player_ships` and `player_active_ships`; runtime uses it when core DB is enabled and reloads starter/active ship through `HangarService` after restart.
  - [x] `economy`: contentdb migration + `InventoryStore` adapter loads/upserts inventory instance item rows, item ledger rows, `AddItem` mutation references, and inventory counters; `InventoryService` persists `AddItem` as one durable commit and reloads safe counters after restart. Move/remove durable refs remain gaps.
  - [x] `modules`: contentdb migration + `LoadoutStore` adapter persists `player_loadouts` and `player_equipped_modules`, composes `HangarStore` for active ship reads, reads module items from durable inventory instance rows, and requires a module item mover before replacement changes item locations. Runtime wiring and durable move ledger/reference storage remain gaps.
- [x] `[P:wave1/lane-A]` Wire runtime to load durable auth, wallet, and stackable inventory state on boot; fail closed in real mode if DB unavailable.
- [x] `[P:wave1/lane-A]` Add `config` flag: real mode = DB, dev/test = in-memory fallback (mirror CMS policy).
- [ ] `[P:wave1/lane-A]` Keep in-memory store as explicit dev/test implementation only.

## Server Ownership
- Player id, account id, session id, balances, item rows, progression rows are server-owned and DB-persisted.
- Never persist plaintext passwords; never log tokens/hashes (AGENTS.md).

## Smoke Tests (one assertion each)
- [x] `register` persists exactly one account row.
- [x] `login` persists exactly one active session row.
- [x] wallet credit persists and reloads with same balance after restart.
- [x] inventory item persists and reloads with same quantity after restart.
- [x] inventory instance item persists and reloads from contentdb after service `AddItem`.
- [x] inventory instance `AddItem` duplicate same ref after service reload returns duplicate and does not insert a second instance row.
- [x] inventory generated instance id after service reload does not collide with loaded generated ids.
- [x] progression XP persists and reloads after restart.
- [x] hangar starter/active ship persists and reloads with valid metadata JSON after store reopen.
- [x] runtime hangar starter/active ship persists and reloads through `HangarService` after restart.
- [x] saved module loadout persists and reloads after store reopen.
- [x] equipped module row persists and reloads after store reopen.
- [x] loadout module item lookup reads the durable inventory instance row.
- [x] missing or invalid durable module item rows fail closed.
- [x] location-changing equipped-module replacement fails closed when no item mover is attached.
- [x] real mode with DB down fails boot (no silent in-memory fallback).

## Done Criteria
- [ ] Account/session/player/wallet/inventory/progression survive restart.
- [ ] Domain packages depend only on repo interfaces.
- [ ] `go test ./...` green.

## Verification
```bash
docker compose up -d postgres
go test ./internal/game/persistence/... ./internal/game/economy/... ./internal/game/progression/... -count=1
go test ./... && git diff --check
```
