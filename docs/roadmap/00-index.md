# Game Roadmap Index

Date: 2026-06-17

## Purpose

This roadmap breaks the game into implementation phases that can be resumed later without reconstructing context from memory.

Each phase file is written as an execution guide:

- what the phase builds
- which module specs to read first
- what must already exist
- TODO tasks in practical order
- tests and abuse cases
- completion criteria
- next-phase handoff notes

## Core Direction

The project is a browser-first 2D space MORPG with a Go-first, server-authoritative backend.

Non-negotiable rules:

- Client sends intent, never truth.
- Gameplay and economy logic stay separate from Symphony/orchestration code.
- Hidden gameplay data must never be serialized to the client.
- Item and currency mutations go through services and ledgers.
- State-changing operations use transaction-style flows.
- Broadcast happens after commit.
- Duplicate requests, retries, workers, and webhooks must not duplicate value.

## Roadmap Phases

1. [Foundation Platform](./01-foundation-platform.md) - complete, audited 2026-06-17
2. [Economy Ledger And Inventory](./02-economy-ledger-inventory.md) - complete, audited 2026-06-17
3. [Progression, Ships, Modules, And Stats](./03-progression-ships-modules-stats.md)
4. [World Worker, AOI, Fog, And Realtime Contracts](./04-world-worker-aoi-fog-realtime.md)
5. [Combat And Loot Vertical Slice](./05-combat-loot-vertical-slice.md)
6. [Death, Repair, And Crafting](./06-death-repair-crafting.md)
7. [Quest Board And Guided Progression](./07-quest-board-guided-progression.md)
8. [World Discovery, Planets, And Intel](./08-world-discovery-planets-intel.md)
9. [Planet Production And Automation Routes](./09-planet-production-routes.md)
10. [Market, Auction, And Premium](./10-market-auction-premium.md)
11. [Browser Client Prototype](./11-browser-client-prototype.md)
12. [Observability, Balancing, And Release Gates](./12-observability-balancing-release-gates.md)

## Suggested Execution Rule

Work one phase at a time. Inside each phase:

1. Re-read `AGENTS.md`.
2. Re-read the phase file.
3. Re-read every source spec listed in the phase.
4. Create or update a detailed implementation plan if the phase is large.
5. Implement in small vertical slices.
6. Run narrow tests while developing.
7. Run `go test ./...`.
8. Run `git diff --check`.
9. Update the phase file's checklist only for work actually completed.

## MVP Playable Loop Target

The MVP should prove this loop:

```text
login
spawn starter ship
move
see visible NPCs only
fight
kill NPC
loot raw materials into cargo
gain XP/rank progress
equip crafted module
discover planet through scanner in our procedural world
claim planet with rank + X Core validation
produce resources
route resources
sell/buy on market
repair after death
```

## Phase Dependency Map

```text
01 Foundation
  -> 02 Economy Ledger
  -> 03 Progression/Ship/Stats
  -> 04 World Worker/AOI/Fog
  -> 05 Combat/Loot
  -> 06 Death/Craft
  -> 07 Quest Board
  -> 08 Planet Discovery/Intel
  -> 09 Production/Routes
  -> 10 Market/Auction/Premium
  -> 11 Client Prototype
  -> 12 Observability/Release Gates
```

Some frontend work may start after phase 04 with a debug client, but the polished client should wait until server contracts are stable.

## Current Project State

As of the 2026-06-17 Phase 01 audit:

- Phase 01 foundation gameplay code exists under `internal/game/...`.
- Shared primitives are available in `internal/game/foundation`, `internal/game/contracts`, `internal/game/catalog`, `internal/game/events`, and `internal/game/testutil`.
- Later gameplay phases should reference these foundation packages instead of defining duplicate IDs, clocks, RNG, error codes, envelopes, catalog version refs, idempotency keys, or event test helpers.
- Existing Symphony/orchestration code remains separate under `internal/symphony`.
- Do not mix OpenAI/Symphony orchestration concerns into game domain packages.

## Global Verification Commands

Run before claiming roadmap implementation work complete:

```bash
go test ./...
git diff --check
```

## Global TODO

- [ ] Keep this index updated when phase files are added, renamed, or completed.
- [x] Add phase status markers after implementation begins.
- [ ] Add links to implementation plans under `docs/plans/` when created.
- [ ] Add links to commits or PRs when each phase is delivered.
