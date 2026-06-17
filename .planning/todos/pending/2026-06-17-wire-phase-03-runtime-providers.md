---
created: 2026-06-17T12:06:28.661Z
title: Wire Phase 03 Runtime Providers
area: gameplay
files:
  - docs/roadmap/03-progression-ships-modules-stats.md
  - internal/game/progression/
  - internal/game/ships/
  - internal/game/modules/
  - internal/game/stats/
  - internal/game/economy/
---

## Problem

Phase 03 hardening closed the local service-level spoofing and consistency
issues, but several fixes intentionally stop at explicit provider boundaries
until runtime composition and persistence exist.

Remaining follow-ups:

- Wire `PlayerRankProvider` to the authoritative progression snapshot/store.
- Wire `PilotProgressionProvider` to authoritative rank and role progression.
- Wire `ShipCargoCapacityProvider` to authoritative effective stat snapshots so
  ship swap cargo validation includes module/passive cargo bonuses.
- Wire `StatInputProvider` to ship, equipped module, progression, role, passive,
  and temporary modifier records.
- Add a runtime `InventoryService` ledger adapter for module equip/unequip so
  `ship_equipped` location moves emit durable item ledger rows.
- Keep XP grants behind concrete domain owners such as quest, combat, scanner,
  production, or crafting completion services before exposing player-facing
  reward flows.

## Solution

Implement this as a runtime composition slice before combat, scanner, or
cargo-heavy flows consume Phase 03 services.

Suggested order:

1. Add read-only adapters from progression and hangar/module records to the
   provider interfaces.
2. Build a stat input provider that derives base ship, equipped module
   modifiers, role bonuses, passives, and temporary modifiers from server-owned
   records.
3. Add equip/unequip inventory ledger integration while preserving the existing
   loadout-store atomic index/location behavior.
4. Add integration tests proving client/caller payloads cannot spoof rank, role,
   cargo capacity, or effective stats through the runtime composition layer.
5. Update Phase 03 roadmap notes when the adapters are wired and verified.
