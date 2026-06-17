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
  XP source completion. Source:
  `docs/roadmap/03-progression-ships-modules-stats.md`.
- [ ] Wire Phase 03 runtime providers to authoritative stores before later
  gameplay depends on them: `PlayerRankProvider`, `PilotProgressionProvider`,
  `ShipCargoCapacityProvider`, `StatInputProvider`, and the inventory ledger
  adapter for module equip/unequip.
