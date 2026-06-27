# Playable v1 Gap Closure Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Turn the current locally playable build into a credible playable v1 by closing the remaining roadmap gaps, preserving server authority, and keeping each slice reviewable and verifiable.

**Architecture:** Keep the Go runtime authoritative. The browser sends intents only. Gameplay value mutations stay behind domain services with idempotency, transaction/outbox semantics, and commit-after-broadcast discipline. Client work exposes only real authenticated server state.

**Tech Stack:** Go server/runtime, Postgres-backed content/core-store modes where required, browser client, existing e2e smoke harnesses, roadmap docs under `docs/road-to-v1/`.

---

## Current Snapshot

Latest committed baseline before this plan:

- `d39f839 game: complete p13 release evidence`
- Road-to-v1 overall: about 80%.
- Playability: local single-process playtest is testable and covers auth, spawn, movement, combat, loot, scanner, claim, production, routes, portals, PvP death/repair, and current artifact gates.
- Dirty workspace currently contains a verified playtest stabilization slice: no-DB playtest `GAME_DEV_MODE`, passive capacitor recovery, repair quote-bound e2e payloads, updated attack retry budgets, and docs for running the local game.

Remaining high-value gaps:

- P11 Signal Gate endgame loop is not started.
- P12 drones/ammo/honor flavor is not started.
- P17 runtime decomposition and P05 deep `Runtime.mu` narrowing are not started.
- P15 has bounded aggro/AOI evidence, but still lacks a full 1500-session runtime AOI projection proof.
- P08 restart survival is proven, but cross-process/concurrency enforcement proof remains.
- Public test-server readiness still needs a durable-mode decision/runbook evidence, hosted artifact gate confirmation, and final asset polish.

---

## Milestone 0 - Lock The Current Playtest Stabilization Slice

Purpose: do not pile new gameplay work on top of unreviewed local-playability fixes.

Files already touched in the current slice:

- `scripts/run_playtest_server.sh`
- `scripts/package_playtest_release.sh`
- `scripts/test_playtest_release_package.sh`
- `client/tests/e2e/playtest-server-flow.mjs`
- `client/tests/e2e/phase09-map-flow.mjs`
- `client/tests/e2e/phase10-enemy-aggro-flow.mjs`
- `client/tests/e2e/phase10-pvp-death-flow.mjs`
- `client/tests/e2e/phase10-pvp-map-drop-flow.mjs`
- `internal/game/server/runtime.go`
- `internal/game/server/combat_loot_helpers.go`
- `internal/game/server/scanner_providers.go`
- `internal/game/server/server_combat_loot_death_test.go`
- `docs/running-local-game.md`
- `docs/playtest-vertical-slice-status.md`

Tasks:

- [ ] Review the dirty diff locally for unrelated changes or accidental fixture leakage.
- [ ] Run an independent code-review agent on the dirty slice.
- [ ] Fix actionable review findings.
- [ ] Re-run the narrow local playtest gate:

```bash
scripts/verify_playtest_vertical_slice.sh
```

- [ ] Re-run full verification:

```bash
go test ./...
npm --cache /tmp/gameproject-npm-cache --prefix client run check
scripts/test_playtest_release_package.sh
git diff --check
```

- [ ] Stage only the touched playtest-stabilization files and this plan if desired.
- [ ] Commit the stabilization slice:

```bash
git commit -m "game: stabilize local playtest loop"
```

Done when: the workspace is clean after commit, verification evidence is recorded, and the user can start the game with `scripts/run_playtest_server.sh`.

---

## Milestone 1 - Public Test Server Readiness Decision

Purpose: decide whether the first shared test server is process-local playtest mode or durable Postgres mode, and make that decision reproducible.

Primary files:

- `docs/running-local-game.md`
- `docs/playtest-vertical-slice-status.md`
- `docs/test-server-operations.md`
- `scripts/run_playtest_server.sh`
- `scripts/package_playtest_release.sh`
- `scripts/verify_playtest_vertical_slice.sh`

Tasks:

- [x] Add a short "first public test server mode" section that states one of:
  - process-local playtest mode is allowed for a resettable short test, or
  - durable Postgres mode is required before external testers.
- [x] If process-local is allowed, document reset expectation, wipe behavior, and what testers must not assume is persistent.
- [x] If durable mode is required, document exact env vars:

```bash
GAME_CONTENT_DATABASE_URL=...
GAME_CONTENT_MODE=required
GAME_CORE_STORE_MODE=required
GAME_CONTENT_MIGRATIONS=auto
GAME_ALLOWED_ORIGINS=...
GAME_CLIENT_STATIC_DIR=...
GAME_PLAYTEST_SEED=true
```

- [ ] Run the private test-server operations checklist against the target local/host candidate and record revision, artifact path, rollback artifact, env shape, and reset policy.
- [ ] Confirm the hosted artifact gate status when available, or keep a clear unchecked item if credentials/workflow activation are still outside the repo.

Verification:

```bash
scripts/package_playtest_release.sh
scripts/test_playtest_release_package.sh
scripts/verify_playtest_vertical_slice.sh
git diff --check
```

Done when: a maintainer can read one runbook and know exactly how to launch, reset, and rollback the first test server.

---

## Milestone 2 - P11 Signal Gate MVP

Purpose: ship one repeatable endgame loop: collect fragments, build a gate, enter an instance, clear waves plus boss, receive rewards exactly once.

Primary phase file:

- `docs/road-to-v1/11-first-endgame-signal-gate.md`

Likely server files:

- `internal/game/gates/`
- `internal/game/server/gate_handlers.go`
- `internal/game/server/runtime_gate*.go`
- `internal/game/content/`
- `internal/game/loot/`
- `internal/game/economy/`
- `internal/game/world/`

Likely client files:

- `client/src/`
- existing HUD/panel state and command transport files
- existing e2e helpers under `client/tests/e2e/`

Tasks:

- [ ] Add gate fragment item content and server-owned fragment sources from NPC/quest/scan paths.
- [ ] Add a `gates` domain package with:
  - gate state machine: unbuilt -> built -> active -> cleared/expired
  - idempotency keys for build and clear
  - participant visibility rules
  - deterministic reward definition
- [ ] Add `gate.build` server operation:
  - validates authenticated player
  - validates fragment ownership/count
  - consumes fragments exactly once
  - records built-gate state
- [ ] Add `gate.enter` and `gate.exit` server operations:
  - validates built gate ownership or allowed party participation
  - creates one active instance
  - moves player/session to instance map through existing server-owned map handoff
  - applies lives/repair/timeout policy server-side
- [ ] Add wave spawner and boss logic:
  - 5 waves plus 1 boss for MVP
  - no hidden instance state leaked to non-participants
  - deterministic enough for smoke tests
- [ ] Add reward settlement:
  - reward key `gate_clear:<instance_id>`
  - wallet/inventory ledger and outbox semantics
  - duplicate clear/replay does not double grant
- [ ] Add browser UI:
  - real gate build state
  - build/enter/exit controls
  - instance HUD wave/boss status
  - empty/locked/loading states for players without fragments
- [ ] Add smoke/e2e coverage, one behavior per test:
  - `gate.build` consumes fragments exactly once
  - entering a built gate creates one instance
  - clearing final boss grants one reward
  - duplicate clear does not double grant
  - non-participant cannot see instance-private state
  - browser build -> enter -> clear -> reward proof
- [ ] Update:
  - `docs/road-to-v1/11-first-endgame-signal-gate.md`
  - `docs/road-to-v1/00-index.md`
  - `docs/road-to-v1/GOAL.md`
  - `docs/road-to-v1/REMAINING-WORK.md`
  - `docs/todo.md`

Verification:

```bash
go test ./internal/game/gates/... ./internal/game/server/... -run 'Gate|Wave|Boss|Reward' -count=1
npm --cache /tmp/gameproject-npm-cache --prefix client run check
scripts/verify_playtest_vertical_slice.sh
go test ./...
git diff --check
```

Commit target:

```bash
git commit -m "game: add signal gate endgame loop"
```

Done when: P11 is checked as complete only if the real browser can build, enter, clear, and claim rewards from one Signal Gate loop.

---

## Milestone 3 - P12 Flavor MVP: Drones, Ammo, Honor

Purpose: add the smallest DarkOrbit-flavor layer that changes real gameplay without faking client state. P.E.T.-lite remains optional until anti-abuse review passes.

Primary phase file:

- `docs/road-to-v1/12-darkorbit-flavor.md`

Likely server files:

- `internal/game/drones/`
- `internal/game/combat/`
- `internal/game/progression/`
- `internal/game/server/`
- `internal/game/content/`

Likely client files:

- hangar/loadout panels
- combat quickslot/ammo selector
- leaderboard/social-adjacent panels if already present
- e2e harnesses

Tasks:

- [ ] Drones-lite:
  - add drone rows/store/domain
  - support drone slots/equip rules
  - reuse effective stat aggregation path
  - add drone XP/level only if it does not expand scope too far
- [ ] Ammo:
  - add ammo item content
  - add selected-ammo server state
  - consume ammo on accepted combat use
  - apply server-owned damage modifier
  - reject client-authored damage/ammo facts
- [ ] Honor:
  - add honor accrual for eligible kills or gate clear
  - add weekly leaderboard query
  - keep viewer-safe projections
- [ ] Browser UI:
  - drone slot/equip state from server snapshot/query
  - ammo selector with pending/reconciled state
  - honor display/leaderboard from server response
- [ ] P.E.T.-lite optional gate:
  - only start if server-owned auto-loot cooldown/radius/fuel rules and anti-bot posture are small enough for v1
  - otherwise document as post-v1
- [ ] Add smoke/e2e coverage:
  - equipping drone module changes effective stats
  - selected ammo consumes one ammo and changes damage
  - honor accrues and appears in weekly leaderboard
  - optional P.E.T. picks one eligible drop and respects cooldown
- [ ] Update phase/status docs.

Verification:

```bash
go test ./internal/game/drones/... ./internal/game/combat/... ./internal/game/server/... -run 'Drone|Ammo|Honor|Pet' -count=1
npm --cache /tmp/gameproject-npm-cache --prefix client run check
go test ./...
git diff --check
```

Commit target:

```bash
git commit -m "game: add darkorbit flavor mvp"
```

Done when: drones-lite, ammo, and honor are shipped with real server state and P12 progress is updated honestly. P.E.T. is done only if anti-abuse proof exists.

---

## Milestone 4 - P15 Runtime AOI Projection Proof

Purpose: close the remaining HI-07 scalability proof beyond current aggro spatial, AOI payload, and 128-session runtime tick evidence.

Primary phase file:

- `docs/road-to-v1/15-world-performance-aoi-optimization.md`

Likely files:

- `internal/game/server/runtime_world_snapshot.go`
- `internal/game/server/runtime_tick*.go`
- `internal/game/world/spatial/`
- `internal/game/world/visibility/`
- `internal/game/observability/`

Tasks:

- [ ] Add a focused 1500-session runtime AOI projection smoke that runs the real runtime AOI tick path, not only a synthetic payload envelope.
- [ ] Measure candidate checks, diff count, enqueue count, tick phase durations, and allocations if existing metrics make that cheap.
- [ ] If the proof passes within budget, document evidence and close P15.
- [ ] If it fails, add an immutable AOI read projection or copy-on-write spatial snapshot so per-session diffing avoids scanning all visible entity state.
- [ ] Add regression tests proving visibility parity before/after projection.
- [ ] Update P15/P13 evidence references and dashboard docs.

Verification:

```bash
go test ./internal/game/server/... ./internal/game/world/... ./internal/game/observability/... -run 'Phase15|AOI|Projection|Load|Tick' -count=1
go test -race ./internal/game/server/... ./internal/game/world/... -run 'Phase15|AOI|Projection|Tick' -count=1
go test ./...
git diff --check
```

Commit target:

```bash
git commit -m "game: prove runtime aoi projection scale"
```

Done when: P15 can honestly claim bounded runtime AOI projection at 1500 sessions or documents the exact remaining bottleneck with failing evidence.

---

## Milestone 5 - P08 Cross-Process Durable Enforcement

Purpose: go beyond restart-survival and prove independent runtimes cannot double-apply claim, production, route, or settlement mutations.

Primary phase file:

- `docs/road-to-v1/08-durable-planet-production-routes.md`

Likely files:

- `internal/game/contentdb/claim_durable_lifecycle_store.go`
- `internal/game/contentdb/claim_production_initialization_durable_store.go`
- `internal/game/contentdb/settlement_durable_store.go`
- `internal/game/contentdb/automation_route_durable_store.go`
- `internal/game/server/`
- `internal/game/discovery/`
- `internal/game/production/`

Tasks:

- [ ] Add DB-only Postgres concurrent two-runtime tests for:
  - simultaneous same-planet claim
  - simultaneous claim retry with same reference
  - simultaneous production-init recovery
  - simultaneous route settlement window
  - simultaneous durable outbox claim/publish
- [ ] Keep tests skipped when `GAME_DATABASE_URL` or required DB env is unset.
- [ ] If tests fail, add row-lock/CAS/unique-reference enforcement where missing.
- [ ] Verify one ledger row, one owner transition, one settlement window, one published outbox row where applicable.
- [ ] Update P08 docs and remaining-work risk notes.

Verification:

```bash
GAME_DATABASE_URL=... go test ./internal/game/server ./internal/game/contentdb/... -run 'P08|Concurrent|CrossProcess|Claim|Settlement|Outbox' -count=1
go test ./...
git diff --check
```

Commit target:

```bash
git commit -m "game: prove p08 cross-process durability"
```

Done when: cross-process P08 double-apply risk is either closed by green DB tests or captured as the next blocking risk with precise failing cases.

---

## Milestone 6 - P17 Runtime Decomposition And P05 Deep Lock Narrowing

Purpose: reduce future change risk and close the remaining P05/P17 maintainability debt without changing gameplay behavior.

Primary phase files:

- `docs/road-to-v1/17-runtime-decomposition-maintainability.md`
- `docs/road-to-v1/05-map-ownership-concurrency.md`

Likely files:

- `internal/game/server/runtime.go`
- `internal/game/server/runtime_*.go`
- `internal/game/server/combat_loot_repair.go`

Tasks:

- [ ] Extract `SessionRuntime` for session attach/detach/resolve.
- [ ] Extract `WorldRuntime` or `MapRuntime` for map instances, tick orchestration, AOI handoff, and portal/respawn routing seams.
- [ ] Extract `EconomyRuntime` for wallet/inventory/market/auction/premium coordination seams.
- [ ] Extract `DiscoveryRuntime` and `ProductionRuntime` only after P08/P11 behavior is stable.
- [ ] Narrow combat/loot/repair command locks:
  - move live-position visibility/range checks into owning services
  - keep value claim atomic
  - keep runtime lock for session/routing bookkeeping only
- [ ] Add race/behavior tests proving Map A command activity does not serialize behind Map B work except where shared authority demands it.
- [ ] Keep each extraction behavior-preserving and separately committed.

Verification per extraction:

```bash
go test ./internal/game/server/... -count=1
go test -race ./internal/game/server/... -run 'Runtime|Session|World|Economy|Combat|Loot|Repair|Race' -count=1
go test ./...
git diff --check
```

Commit targets:

```bash
git commit -m "refactor: extract session runtime coordinator"
git commit -m "refactor: extract world runtime coordinator"
git commit -m "game: narrow runtime command locks"
```

Done when: `Runtime` composes narrower coordinators, broad locks are reduced where behavior allows, and existing protocol behavior does not regress.

---

## Milestone 7 - Curated Entity Asset Catalog

Purpose: make the game look like the real asset set without accidentally shipping the full oversized source asset directory.

Source asset pool:

- `client/src/assets/entities/index.json`
- `client/src/assets/entities/*/manifest.json`
- `client/src/assets/entities/*/frames/*.png`
- `client/src/assets/entities/*/*_spin_512.gif`

Direction convention from the generated isometric assets:

```text
00 southwest
02 west
04 northwest
06 north
08 northeast
10 east
12 southeast
14 south
```

Tasks:

- [x] Add a small generated/curated metadata catalog that lists asset id, kind, display name, and frame direction mapping.
- [x] Do not import the whole `client/src/assets/entities/` source pool into the deploy bundle.
- [x] Pick a small first visual set:
  - one player ship
  - one hostile NPC ship
  - one lootbox/lootable
- [x] Optimize/copy only selected runtime-safe frames into a deploy-safe asset folder, likely under `client/src/assets/world/entities/`.
- [x] Wire `world-renderer-assets.ts` to use the curated runtime-safe assets for player, NPC, and loot sprites.
- [x] Add a renderer test proving selected entity sprite asset keys resolve from the catalog.
- [x] Keep `client/tests/bundle-scan.mjs` green by allowing only the curated runtime-safe output names, not source asset tokens like `spin_512`.
- [x] Update screenshots and playtest proof so the current game visibly uses the selected ship/lootbox assets.

First curated set:

- player ship: `client/src/assets/entities/Nebula_Vanguard_2/manifest.json`, frame `10` (`east`) copied as `client/src/assets/world/entities/ship_player_iso_east.png`
- hostile NPC: `client/src/assets/entities/Nebula_War_Crab/manifest.json`, frame `10` (`east`) copied as `client/src/assets/world/entities/npc_hostile_iso_east.png`
- lootable: `client/src/assets/entities/Nebula_Hypercube/manifest.json`, frame `10` (`east`) copied as `client/src/assets/world/entities/loot_cache_iso_east.png`

Verification:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run check
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:playtest-server
git diff --check
```

Done when: the browser playtest screenshot shows the curated ship/NPC/loot assets, bundle scan stays green, and the source `entities` folder remains a non-deployed art source pool.

---

## Milestone 8 - Public-Facing Polish And Final v1 Gate

Purpose: make the playable build presentable after core gameplay gaps are closed.

Tasks:

- [ ] Finalize world/player/NPC/loot/portal visual assets enough for a public test, using the curated entity asset catalog where appropriate.
- [ ] Add or update screenshots for the current HUD and playtest flow.
- [ ] Re-run the full vertical-slice gate after P11/P12/P15/P08/P17 changes.
- [ ] Update:
  - `docs/playtest-vertical-slice-status.md`
  - `docs/running-local-game.md`
  - `docs/test-server-operations.md`
  - `docs/road-to-v1/GOAL.md`
  - `docs/road-to-v1/00-index.md`
  - `docs/road-to-v1/REMAINING-WORK.md`
  - `docs/todo.md`
- [ ] Run independent code review on the final v1 candidate.
- [ ] Fix findings or record explicit accepted risks.

Final verification:

```bash
go test ./...
npm --cache /tmp/gameproject-npm-cache --prefix client run check
scripts/verify_playtest_vertical_slice.sh
scripts/package_playtest_release.sh
scripts/test_playtest_release_package.sh
git diff --check
```

Done when: roadmap dashboard can move from about 80% to v1-complete with real evidence, not optimism.

---

## Recommended Execution Order

1. Milestone 0: review and commit current playtest stabilization.
2. Milestone 1: make first test-server mode/runbook explicit.
3. Milestone 2: build P11 Signal Gate MVP.
4. Milestone 3: build P12 drones/ammo/honor MVP.
5. Milestone 4: close P15 full runtime AOI projection proof.
6. Milestone 5: close P08 cross-process durable enforcement.
7. Milestone 6: execute P17 runtime decomposition and P05 deep lock narrowing.
8. Milestone 7: add curated entity asset catalog and first runtime-safe selected assets.
9. Milestone 8: final public-facing polish and v1 gate.

This order optimizes for player-visible progress first while keeping durability and scalability proof work explicit before a wider rollout.
