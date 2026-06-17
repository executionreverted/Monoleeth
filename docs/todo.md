# Project TODO

Date: 2026-06-17

This file tracks cross-phase follow-ups that should not be lost between Symphony
waves or manual review sessions. Roadmap phase files remain the source of truth
for phase status; this file is a compact pending-work index.

## Open

- [ ] Wire realtime gateway request handling to authenticated session and
  server-side player resolution before exposing Phase 04 worker commands over
  WebSocket. Source: `docs/roadmap/04-world-worker-aoi-fog-realtime.md`.
- [ ] Add in-flight duplicate coordination to `realtime.RequestCache` when the
  gateway executes mutating commands concurrently; the current cache only
  remembers completed responses.
- [ ] Wire XP grants behind concrete domain owners such as quest, scanner,
  production, crafting, route, event, and admin services so clients cannot spoof
  XP source completion. Combat NPC kill XP and eligible loot pickup XP now have
  Phase 05 domain boundaries; remaining XP sources still need owners. Source:
  `docs/roadmap/03-progression-ships-modules-stats.md`.
- [ ] Wire the remaining Phase 03 runtime inventory ledger adapter for module
  equip/unequip. Rank/role-gate, module-aware stat input, and effective
  cargo-capacity providers exist under `internal/game/runtime`.
- [ ] Map unlocked pilot-skill passive stat effects into runtime stat input.
  The stat aggregation model has passive buckets, but current runtime providers
  compose base ship and equipped module stats only.
- [ ] Add a durable reward/outbox reconciliation path for Phase 05 loot XP
  grants; current pickup records in-memory `LootXPReconciliation` metadata but
  there is no durable repair worker or cross-service transaction yet.
- [ ] Map gateway/API request ids to the required
  `CraftingService.StartCraft` domain `ReferenceKey` before exposing craft
  start externally; the Phase 06 domain service is now idempotent when callers
  provide a stable player-scoped `craft_start:*` reference.
- [ ] Replace `RepairService` compensating wallet refunds with a durable
  transaction/outbox boundary when wallet and ship state move out of the
  in-memory Phase 06 slice. Restore failure after debit is currently net-zero
  compensated, but it is not a true atomic rollback. Source:
  `docs/roadmap/06-death-repair-crafting.md`.
- [ ] Add an indexed wallet ledger/reference lookup for repair refund replay
  checks before wallet histories become large; the current in-memory repair
  compensation path scans ledger entries under service lock.
- [ ] Move `DeathService.ProcessDeath` from caller-supplied cargo/drop/loadout
  inputs to authoritative zone inventory, loadout, respawn, and drop-policy
  providers before exposing death processing through gateway/runtime callers.
- [ ] Block or serialize cargo transfers while a lethal/death transaction is in
  progress for the player ship; current death processing is retry-safe but does
  not lock live cargo movement.
- [ ] Wire disabled ship state into combat runtime actor ownership so a stale
  combat actor cannot attack after `DisableActiveShipForDeath`.
- [ ] Add durable completion/reconciliation for `CraftingService.CompleteCraft`
  after reservation commit; current in-memory retry path is idempotent, but a
  crash between reservation commit, output grant, XP grant, and job completion
  still needs recovery.
- [ ] Add craft location ownership/building validation before enabling
  owned-planet or planet-building recipes beyond the current station MVP.
- [ ] Wire `CraftLocationAuthorizer` to authoritative runtime station, planet,
  and building ownership providers before exposing non-station craft recipes;
  the Phase 06 service now has a pre-mutation hook, but no runtime provider is
  connected yet.
- [ ] Add gateway/security tests for craft start authorization using the
  authenticated server-side player id, including hidden or unowned planet and
  building ids with leak-safe errors.
- [ ] Replace the process-local `CraftingService.CompleteCraft` in-flight guard
  with durable per-job state transitions or row locks plus metrics for
  completion wait time and duplicate retry rate before multi-process runtime
  deployment.
- [ ] Narrow DeathService and RepairService lock scope from global service mutex
  to per-player/per-reference coordination before these services move to a
  higher-concurrency runtime path.
- [ ] Add a concrete Phase 07 quest item reward adapter from
  `QuestRewardInventoryService` to `economy.InventoryService.AddItem` once the
  quest reward item-definition catalog/provider is wired; current claim tests
  prove the quest boundary and idempotency reference but use fakes.
- [ ] Enforce rare reward caps before enabling rare quest rewards such as X Core
  or premium rewards; Phase 07 currently stores rare-cap hooks as policy markers
  and leaves the actual cap check unchecked in the roadmap.
- [ ] Make quest board queries expiry-aware before exposing them through
  gateway/client APIs; `AcceptQuest` rejects expired offers, but `BoardOffers`
  currently returns stored unaccepted offers without a clock-aware expiry filter.
- [ ] Collapse or document the preferred quest objective schema shape before the
  quest API becomes public; `ObjectiveSchema` currently supports both
  `Objectives []Objective` and legacy single-objective fields.
- [ ] Add per-player offer/active-quest indexes plus TTL/compaction or durable
  uniqueness for quest in-memory caches (`progressEvents`, `claimResults`,
  `rerollResults`) before long-running or multi-process deployment.

## Completed

- [x] Replace the Phase 05 vertical-slice test-local stat input adapter with
  concrete Phase 03 runtime providers for the in-process backend vertical slice.
  Gateway exposure remains blocked on authenticated session/player resolution.
- [x] Add a zone-worker due-task dispatcher that invokes
  `LootService.HandleScheduledDropTask` from worker ticks instead of requiring
  in-process callers to inspect `TickResult.DueTasks` manually.
