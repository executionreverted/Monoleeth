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
- [ ] Wire XP grants behind concrete domain owners such as combat, quest,
  scanner, production, and crafting completion services so clients cannot spoof
  XP source completion. Combat NPC kill XP now has a domain boundary; remaining
  XP sources still need owners. Source:
  `docs/roadmap/03-progression-ships-modules-stats.md`.
- [ ] Wire the remaining Phase 03 runtime inventory ledger adapter for module
  equip/unequip. Rank, pilot progression, module-aware stat input, and
  effective cargo-capacity providers exist under `internal/game/runtime`.
- [ ] Add a durable reward/outbox reconciliation path for Phase 05 loot XP
  grants; current pickup persists in-memory `LootXPReconciliation` metadata but
  there is no durable repair worker or cross-service transaction yet.

## Completed

- [x] Replace the Phase 05 vertical-slice test-local stat input adapter with
  concrete Phase 03 runtime providers before exposing combat/loot gateway
  commands. Completed in `internal/game/runtime`.
- [x] Add a zone-worker due-task dispatcher that invokes
  `LootService.HandleScheduledDropTask` in the runtime loop instead of requiring
  callers to inspect `TickResult.DueTasks` manually.
