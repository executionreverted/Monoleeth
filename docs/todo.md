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
- [ ] Add request-id idempotency for `CraftingService.StartCraft` before
  exposing craft start through a realtime/API gateway; the Phase 06 in-memory
  domain service currently creates a new job per accepted start call.

## Completed

- [x] Replace the Phase 05 vertical-slice test-local stat input adapter with
  concrete Phase 03 runtime providers for the in-process backend vertical slice.
  Gateway exposure remains blocked on authenticated session/player resolution.
- [x] Add a zone-worker due-task dispatcher that invokes
  `LootService.HandleScheduledDropTask` from worker ticks instead of requiring
  in-process callers to inspect `TickResult.DueTasks` manually.
