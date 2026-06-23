# UI Implementation Plan Index

Date: 2026-06-19

## Purpose

This folder is the active roadmap for turning the current browser prototype into
the real authenticated game client.

The previous backend roadmap proved the domain services and server-authoritative
rules in Go. This plan connects those systems to the browser so the player can
use every implemented game feature through real server commands, queries,
snapshots, and events.

## Non-Negotiable Outcome

The UI must stop feeling like a mockup.

That means:
- default client state cannot be seeded with fake gameplay values
- all gameplay panels must read from authenticated server state
- all state-changing controls must send server intent commands
- server responses and events must reconcile client state
- hidden gameplay data must never be serialized to the browser
- visual polish must follow `output/mockups/final-mockup.png`, but visual parity
  must not come from fake data

## Visual Target

Use:

```text
output/mockups/final-mockup.png
```

The target interface is a dense operational space console:
- top status bar: sector, danger, energy, cargo, credits, capacitor, mail/social/menu
- left rail: ship status and primary navigation
- center: full-bleed active local map/canvas with current-map AOI/radar-visible
  entities only
- bottom: combat/utility action bar and server event log
- right rail: planets, selected object panel, sector map

Mockup ownership:
- sector, danger, energy, cargo, credits, capacitor: real server state by the
  phase that exposes the matching system
- mail/social/menu indicators: Phase 09 owns notification/admin/menu state
- mail/social features not implemented for MVP must render locked, empty, or
  hidden states from server config; no fake unread counts or fake friends

## Phase List

1. [Auth, Accounts, Sessions, And Admin Seed](./01-auth-accounts-sessions.md)
2. [Game Server Transport And Runtime Composition](./02-game-server-transport-runtime.md)
3. [Client Auth Shell And Demo Removal](./03-client-auth-shell-demo-removal.md)
4. [Live World, AOI, Movement, And Map State](./04-live-world-aoi-movement.md)
5. [Combat, Loot, Death, And Repair UI](./05-combat-loot-death-repair.md)
6. [Progression, Inventory, Loadout, And Crafting UI](./06-progression-inventory-loadout-crafting.md)
7. [Discovery, Planets, Production, And Routes UI](./07-discovery-planets-production-routes.md)
8. [Market, Auction, Premium, And Economy UI](./08-market-auction-premium-economy.md)
9. [Quests, Admin, Observability, And Release Gates UI](./09-quests-admin-observability-release.md)
10. [Final Mockup Parity And End-To-End Hardening](./10-final-mockup-parity-hardening.md)

## Dependency Map

```text
01 Auth
  -> 02 Game Server Transport/Runtime
  -> 03 Client Auth Shell/Demo Removal
  -> 04 Live World/AOI/Movement
  -> 05 Combat/Loot/Death/Repair
  -> 06 Progression/Inventory/Loadout/Crafting
  -> 07 Discovery/Planets/Production/Routes
  -> 08 Market/Auction/Premium/Economy
  -> 09 Quests/Admin/Observability/Release Gates
  -> 10 Final Mockup Parity/E2E Hardening
```

Some UI shell work can happen while server runtime work is in progress, but no
phase may mark gameplay complete until the browser uses real server state.

Phase 05 depends on wallet, cargo, active ship, and loadout truth before loot,
repair, or combat buttons can be marked done. If Phase 06 has not delivered the
full inventory/hangar UI yet, Phase 05 must still implement or reuse a minimal
server-backed wallet/cargo/active-loadout snapshot path for its own mutations.

## Source Backend Modules

Use these specs as source truth when wiring UI features:

```text
docs/plans/modules/01-player-progression-rank-role-skills.md
docs/plans/modules/02-inventory-cargo-wallet-ledger.md
docs/plans/modules/03-ship-hangar-loadout.md
docs/plans/modules/04-module-stat-aggregation.md
docs/plans/modules/05-combat-damage-targeting.md
docs/plans/modules/06-loot-drop-ownership.md
docs/plans/modules/07-death-repair-respawn.md
docs/plans/modules/08-crafting-recipes-materials.md
docs/plans/modules/09-market-auction-premium.md
docs/plans/modules/10-quest-board-generation.md
docs/plans/modules/11-planet-production-offline-settlement.md
docs/plans/modules/12-automation-routes.md
docs/plans/modules/13-intel-coordinate-trading.md
docs/plans/modules/14-world-aoi-fog-security.md
docs/plans/modules/15-api-events-errors.md
docs/plans/modules/16-testing-observability-balancing.md
```

## Global Contract Shape

Client request:

```json
{
  "request_id": "uuid",
  "op": "combat.use_skill",
  "payload": {
    "target_id": "npc-1",
    "skill_id": "basic_laser"
  },
  "client_seq": 42,
  "v": 1
}
```

Server response:

```json
{
  "request_id": "uuid",
  "ok": true,
  "payload": {},
  "server_time": 182736123,
  "v": 1
}
```

Server event:

```json
{
  "event_id": "uuid",
  "type": "player.snapshot",
  "payload": {},
  "server_time": 182736124,
  "seq": 99123,
  "v": 1
}
```

Every exposed operation must document enough detail to implement and test the
real server path. Command names alone are not sufficient.

For commands, document:
- client payload and forbidden client-authored fields
- server-resolved authority and ownership source
- validation gates, including current-map visibility, radar/stealth detection,
  known-intel permission, range, role, capacity, wallet, cooldown, energy, and
  item tradeability where applicable
- mutation flow using `lock -> validate -> mutate -> ledger/event -> commit`
- emitted response, snapshots, and events
- idempotency key or duplicate handling
- rate-limit posture
- browser pending/success/error/reconcile states

For queries and snapshots, document:
- request payload or filter shape
- server-side permission checks
- client-safe response schema
- hidden fields that must be omitted
- pagination, freshness, and cache/reconnect behavior where applicable

For events, document:
- payload schema
- visibility and recipient filtering
- after-commit publishing requirement
- monotonic per-session sequence or reconciliation marker
- duplicate/stale event handling

For errors and reconciliation, document:
- stable error codes
- safe public messages
- whether optimistic UI state must rollback, refresh a snapshot, or stay pending
- which snapshot repairs stale or missed events

## Deterministic Real-Server Smoke Seed

Browser smoke tests for this roadmap must boot the real Go server with a
reproducible dev seed, not a JavaScript mock WebSocket.

The seed must provide, as needed by the phase under test:
- one admin account and one normal account
- stable player ids and callsigns resolved only server-side
- starter ship, active loadout, stats, wallet, cargo, and inventory
- a sector with deterministic visible and hidden AOI entities
- at least one combat target, loot path, and repairable disabled state
- quest board offers and a reward path
- market/auction fixtures with escrow-safe ownership
- a known/discoverable planet and route/production fixture

The seed must never expose procedural seeds, hidden entities, passwords, session
tokens, or admin secrets to the browser.

## Screenshot Review Matrix

Every browser visual review must save artifacts under a deterministic path such
as `output/screenshots/ui-implementation/<phase>/`.

Required viewports:
- desktop: `1440x900`
- tablet: `1024x768`
- mobile: `390x844`

Required states as each phase makes them available:
- unauthenticated auth shell
- authenticated loading/bootstrap
- live default HUD
- selected target/object
- modal or overlay panel
- error/empty/locked state

Review criteria:
- topbar, left rail, center map, bottom action/log bar, right rail, and minimap
  are present or intentionally locked for the phase
- text does not overflow or overlap
- canvas is nonblank where world state exists
- screenshots do not contain fake/demo labels or hidden server metadata

## Global Done Criteria

- Mail/password login works.
- Admin account can be seeded reproducibly.
- Browser WebSocket resolves session server-side.
- The default client shows no fake gameplay data.
- Every visible gameplay value comes from server state.
- Every implemented backend feature has a real UI path. If blocked, the blocker
  entry in `docs/todo.md` must include owner, missing contract, unblock
  condition, severity, and acceptance test.
- Browser smoke tests use the deterministic real Go server seed, not only a
  JavaScript mock WebSocket fixture.
- Desktop, tablet, and mobile screenshots are checked against the mockup
  direction and stored with artifact paths.
- `go test ./...` passes.
- `npm --cache /tmp/gameproject-npm-cache run check` passes in `client/`.
- `git diff --check` passes.
