# Task 001 Plan Index

Date: 2026-06-20

## Purpose

Task 001 is a hardening and game-system pass for the authenticated browser game.
The current client can authenticate, move, scan, render the world, open windows,
and use some real server contracts, but the next playtest problems show three
large gaps:

- many UI controls are not fully connected to server gameplay contracts
- radar/map/visibility is too narrow or visually misleading for playtesting
- the main menu surfaces still feel like generic panels instead of game systems

This plan converts those complaints into implementation phases with explicit
server-ownership, UI layout, and verification gates.

## Primary References

```text
docs/plans/task-001-goal.md
docs/plans/task-001/task-001-status-report-2026-06-21.md
docs/plans/task-001/gameplay-gap-audit-2026-06-21.md
docs/plans/task-001/browser-e2e-rebuild-plan.md
output/mockups/final-mockup.png
output/mockups/darkorbit-magaza-ornek-mockup.png
output/mockups/darkorbit-envanter-ornek-layout.png
output/assets/hud-svg/
docs/todo.md
docs/plans/ui-implementation/00-index.md
docs/plans/ui-implementation/04-live-world-aoi-movement.md
docs/plans/ui-implementation/06-progression-inventory-loadout-crafting.md
docs/plans/ui-implementation/07-discovery-planets-production-routes.md
docs/plans/ui-implementation/08-market-auction-premium-economy.md
docs/plans/ui-implementation/10-final-mockup-parity-hardening.md
docs/plans/modules/00-index.md
docs/2026-06-17-world-system-design.md
docs/2026-06-17-progression-economy-systems-design.md
```

## DarkOrbit Reference Notes

Use these only for interaction and layout references:

```text
https://darkorbit.fandom.com/wiki/Hangar
https://darkorbit-archive.fandom.com/wiki/Hangar
https://darkorbit.fandom.com/wiki/Generators
https://darkorbit.fandom.com/wiki/Cloak
https://darkorbitwiki.com/equipment/
https://darkorbit-archive.fandom.com/wiki/Cargo_box
https://darkorbit-archive.fandom.com/wiki/Bonus_box
```

Key patterns to adapt:

- Hangar is owned ships, active ship, and manage/equipment layout.
- Equipment has ship preview, slot groups, filters, and available inventory.
- Shop has category rail, product grid/list, selected detail, and buy panel.
- Equipment categories should map to this game as ships, weapons/lasers,
  launchers, ammo, shield generators, speed generators, scanners, stealth,
  cargo, repair/support, boosters, and modules.
- Cloak/stealth must be server-owned and countered through scanner/radar rules.

## Current Audit Snapshot

Server/UI connection gaps found during the planning audit:

- `planet.storage_summary` is requested and handled, but reducer handling is
  weak or missing for `planet_storage`.
- `route.snapshot` exists server-side, but UI entry and singular route parsing
  are weak.
- `death.ship_disabled` logs but does not reliably surface a repair/disabled
  ship UI unless a separate ship snapshot arrives.
- Market, auction, and premium events are mostly log-only for passive clients.
- `auction.bid` can be double-sent from one click path and needs a smoke guard.
- `inventory.move`, `progression.unlock_skill`, crafting mutations, planet
  claim/build/upgrade, route mutations, intel share, and coordinate item flows
  are not exposed as browser gameplay loops.

World/radar findings:

- Current live AOI feel is based on `defaultRadarRange = 420`.
- Movement max distance and client navigation chunking are separate concerns.
- Scanner discovery radius is already `2000`, but it is not the live AOI/radar
  window.
- Minimap is built from live AOI plus remembered planet intel, but remembered
  entries need stable ids and click behavior.
- Client fog overlay is visual only and can be removed without exposing hidden
  server data.

UI findings:

- Windows use fixed dimensions that create empty space or cramped scroll.
- Generic `Inspect` duplicates window bodies and feels like a debug button.
- Target panels show `No lock` plus disabled actions as clutter.
- Shop says internal implementation copy such as `Server: recalculates`.
- Inventory/Cargo/Loadout are stacked in one surface instead of distinct game
  layouts.
- Planet detail displays internal lock copy and disabled controls regardless of
  whether the player can use them.
- The minimap/radar is visually weak, inert, and does not feel like a circular
  sector map.

## Browser Smoke Retirement - 2026-06-21

The monolithic browser smoke coverage is temporarily retired. Future browser/e2e
coverage must be rebuilt as small per-flow files under a dedicated test harness;
`npm run check` no longer runs a browser smoke suite. Existing smoke references
in phase notes are historical or future acceptance targets until that harness is
rebuilt.

See [`browser-e2e-rebuild-plan.md`](./browser-e2e-rebuild-plan.md).

## Current Task State - 2026-06-21

Current checklist state is
[`task-001-status-report-2026-06-21.md`](./task-001-status-report-2026-06-21.md).

Summary:

- Task 001 is not complete.
- Phase checklist progress is `137 / 227 = 60.4%`.
- Browser/e2e release proof is absent until the per-flow harness is rebuilt.
- Gameplay integration gaps are tracked in
  [`gameplay-gap-audit-2026-06-21.md`](./gameplay-gap-audit-2026-06-21.md).
- Primary execution spine is Phase 05 -> Phase 07 -> Phase 08 -> Phase 11.
- Phase 06, Phase 03, and Phase 04 side gates must close before Phase 11 can
  honestly pass.

## Subagent Review Pass - 2026-06-20

Fresh read-only subagents reviewed Task 001 by phase group and cross-checked
the phase plans against the backend module specs under `docs/plans/modules/`.
All agents were closed after reporting. Their findings are folded into the
phase files as planning additions, not completed checklist items.

Key additions from the review:

- Phase 01 needs explicit reducer contracts for singular `route.snapshot`,
  standalone `planet_storage`, passive economy event reconciliation or refresh,
  `death.ship_disabled` runtime bridging, and broader unimplemented-operation
  guard lists.
- Phase 02 needs an exact worker/spatial-index projection task, minimap
  reconciliation from AOI diffs, stable minimap DOM/server identities, and smoke
  rewrites for fog removal.
- Phase 03 needs viewer/target identity in visibility, a separate player
  stealth state instead of reusing generic hidden markers, worker speed mutation
  for stealth toggles, a live-player scan reveal branch, and witness leak tests.
- Phase 04 and Phase 10 need a single world-focus/input ownership rule, real
  `?` tutorial topics instead of `Inspect`, content-sized windows, dead-control
  cleanup, `Tab` target cycling, radar contact click contracts, and WASD
  decision/testing.
- Phase 05 and Phase 07 need an explicit server-owned content/catalog model,
  `shop.catalog` or equivalent system-product query, system shop vs player
  market separation, DarkOrbit-style category mapping, passive economy
  reconciliation, and anti-debug-copy tests.
- Phase 06, Phase 08, and Phase 09 need stronger backend/UI contracts for
  display metadata, idempotency, planet storage/routes, quest passive events,
  and smoke coverage for real buttons instead of disabled placeholder clutter.
- Phase 11 must move future browser/e2e artifacts to
  `output/screenshots/task-001/`, remove old `radar_range === 420` and
  active-fog expectations, and assert forbidden debug copy in visible text,
  `title`, and `aria-label`.

## Second Subagent Review Pass - 2026-06-20

Fresh read-only caveman-mode subagents reviewed the current Task 001 plan and
dirty worktree again after the first implementation slices. They did not edit
files or run tests. Their new backend/UI gaps are folded into the owning phase
files as planning additions.

Second-pass findings to preserve:

- Phase 01 still needs explicit tracking for progression rank/skill contracts,
  system shop catalog/buy contracts, auction grant semantics, product-specific
  premium stock intent, strict trusted-payload denylist parity, and the open
  browser revoked-session/auth-expiry smoke.
- Phase 02 needs a projection source contract for DB overlays, procedural/live
  materialization, and worker-owned entities so the widened radar is not only a
  larger view of current in-memory fixtures.
- Phase 03 is not yet playable end to end: server witness foundations exist,
  but public stealth activation, scanner-created witnesses, client leak guards,
  and `scan_revealed` UI treatment remain open.
- Phase 04 and Phase 10 still have live code evidence for generic `Inspect`,
  fixed-height windows, timer-only input suppression, missing `Tab`, undecided
  WASD, and incomplete radar click semantics.
- Phase 05 and Phase 07 need exact content/shop contracts: `ContentRegistry`,
  `ShopProductDefinition`, `shop.catalog`, `shop.buy_product` vs `market.buy`,
  server-owned category/detail metadata, and replacement of old `raw_ore` /
  `server_recalculates` smoke expectations.
- Phase 06 needs a real public contract matrix for inventory/cargo/loadout/
  hangar, richer cargo display metadata, duplicate command guards, and a named
  runtime blocker for module-aware cargo capacity during ship activation.
- Phase 08 and Phase 09 need smoke and plan alignment: disabled planet
  placeholders must fail, detail open must refresh production/storage/routes,
  production settlement needs an owner wrapper, quests need server-owned action
  state, and quest events must remove stale offers.
- Phase 11 must not pass release with old screenshots, old shop smoke, missing
  auth-expiry smoke, missing scanner witness, weak forbidden-copy scans,
  incomplete Phase 10 input coverage, or missing multi-client economy fanout
  evidence.

## Third Subagent Review Pass - 2026-06-20

Fresh read-only subagents were closed and respawned by task area to review the
current dirty worktree against the Task 001 phase plans and the backend module
specs under `docs/plans/modules/`. They did not edit files or run tests. Their
new findings are folded into the owning phase files as open planning gates.

Third-pass findings to preserve:

- Phase 01 still has protocol denylist drift between Go request rejection,
  TypeScript command rejection, and TypeScript server-message parsing. Admin
  exceptions such as `target_player_id` must be explicit and tested.
- Phase 02's active-map visibility source contract still needs tests for worker
  entities, DB/procedural/live materialization, known intel, NPCs, loot, and
  players entering the bounded `0..10000` map candidate set before
  AOI/radar/stealth filtering.
- Phase 03 still lacks browser-level two/three-session scanner witness smoke,
  visible `scan_revealed` treatment, and final stealth module/energy/cooldown/
  anti-spam rules.
- Phase 04 and Phase 10 still have live code evidence for generic `Inspect`,
  fixed-height windows, timer-only world input suppression, missing `Tab`,
  undecided WASD, dead target clutter, and generic minimap click behavior.
- Phase 05 is a root blocker: there is no canonical content registry,
  system shop catalog, broad NPC/loot archetypes, or complete recipe/content
  reference validation. Downstream UI phases must not fake around that gap.
- Phase 06 still has raw/thin cargo payloads, no `inventory.move` contract,
  no top-level Equipment/Inventory/Cargo/Crafting tabs, no per-action pending
  guards, and runtime hangar activation is not module-aware.
- Phase 07 still has no real `shop.catalog` or `shop.buy_product`, still uses
  raw ore market fixtures as system shop truth, and still lacks passive economy
  fanout/refresh proof for market, auction, premium, and shop catalog viewers.
- Phase 08 and Phase 09 still expose disabled primary placeholders where the
  backend registry is read-only, and their payloads lack action-state,
  revision, settlement, and display metadata needed for real UI.
- Phase 11 can false-pass today: smoke writes old screenshot directories, shop
  smoke proves old wrong categories/raw ore/server-recalculates truth, planet
  smoke accepts disabled placeholders, and artifact freshness is manual.

## Fourth Subagent Review Pass - 2026-06-20

Fresh read-only caveman-mode subagents reviewed every Task 001 phase against
the current worktree and the relevant backend module specs. They did not edit
files or run tests. Their findings are folded into the owning phase files as
open planning gates.

Fourth-pass findings to preserve:

- Phase 01 needs explicit classification and production-negative tests for
  `session.snapshot`, `debug_snapshot`, and `debug_spawn_npc`; per-operation
  rate-limit posture for scan, market search, quest reroll, combat, and loot;
  and public bridge/refresh decisions for loot expiry, production settlement,
  offline settlement, and route transfer events.
- Phase 02 needs scan/known-planet updates to refresh minimap and world memory
  without requiring a manual Sync, a far-memory policy that does not clamp old
  intel into a fake near contact, and tests for square projection versus
  circular radar rendering.
- Phase 03 needs scanner reveal success to guarantee the revealed hidden player
  is serializable in the current server AOI projection, stealth speed to resync
  when base stats change while cloaked, and browser smoke mocks that emit the
  self AOI `stealthed` flag instead of assuming local truth.
- Phase 04 and Phase 10 need repeat-key spam guards, a clear `Tab` split
  between world target cycling and modal/input native focus behavior,
  op-specific quick-action rate-limit gates, and mobile/tablet pointer leak
  smoke for moving plus modal/window interaction.
- Phase 05 needs a concrete `energy_cell_batch` quest/crafting mismatch fixed
  or blocked by registry validation, crafting/loot display metadata contracts,
  and a non-starter ship acquisition invariant.
- Phase 06 needs a crafting-tab policy now that `crafting.recipes` exists, a
  fail-closed unknown-catalog cargo policy, active-ship hangar buttons hidden in
  favor of status badges, and broader raw-id smoke across visible/title/aria
  text.
- Phase 07 needs sell/listing eligibility metadata, retry-safe
  `market.create_listing`, auction wallet selection by `lot.currency_type`,
  product-specific premium stock identity, and duplicate-send guards for every
  economy mutation, not only `auction.bid`.
- Phase 08 and Phase 09 need `planet.storage_summary` collection/singular
  payload parity, inbound and outbound route display metadata, planet actions
  driven by server `available_commands` or `can_*` state, and quest expiry/reset
  contracts with stale-board smoke.
- Phase 11 needs a future per-flow browser/e2e harness, current-run manifest,
  task-specific artifact gate, hard phase-closure table, real-server
  provenance for screenshots, negative debug WebSocket coverage, three-session
  witness interaction coverage, and visual parity contact sheets instead of
  file-existence-only proof.

## User Issue Coverage

| User issue | Covered by |
| --- | --- |
| Detect all missing UI/server gameplay links | Phase 01 |
| Pull wider nearby objects, originally proven with a `2000x2000` window and now superseded by bounded active-map visibility | Phase 02 |
| Revise menus from DarkOrbit examples and content-sized windows | Phase 04, 06, 07 |
| Replace `Inspect` with `?` tutorial modal | Phase 04 |
| Market catalog should be real game catalog, not raw ore | Phase 05, 07 |
| Map/radar should show known nearby objects, players, enemies | Phase 02 |
| Hidden player, speed penalty, scan witness for 15 minutes | Phase 03 |
| Seed DarkOrbit-style dummy content with renamed data | Phase 05 |
| Use shop/inventory local mockups | Phase 06, 07 |
| Remove internal planet copy, wire claim | Phase 08 |
| Inventory/Cargo tabs and final mockup monochrome style | Phase 04, 06 |
| Quest board must be a real game surface | Phase 09 |
| Moving while modal open, modal blocks movement clicks | Phase 10 |
| Hide impossible target actions | Phase 04, 10 |
| Sector map/radar shape, contents, click navigation | Phase 02, 10 |
| Remove fog of war for now | Phase 02 |
| Hotkeys `1..6`, `Tab`, WASD | Phase 10 |

## Phase List

1. [Gameplay Connection Audit And Contract Parity](./01-gameplay-connection-audit.md)
2. [AOI, Radar, Map Visibility, And Fog Removal](./02-aoi-radar-map-visibility.md)
3. [Stealth, Scanner Witness, And Hidden Player Visibility](./03-stealth-scan-witness.md)
4. [HUD Window, Modal, Tutorial, And Dead-Control Cleanup](./04-hud-modal-tutorial-window-system.md)
5. [Seeded Game Content Catalog](./05-seeded-game-content-catalog.md)
6. [Inventory, Cargo, Loadout, And Hangar Game Layouts](./06-inventory-cargo-loadout-hangar.md)
7. [Shop, Market, Auction, And Catalog Rework](./07-shop-market-catalog-rework.md)
8. [Planets, Claim, Production, And Routes](./08-planets-production-routes-claim.md)
9. [Quest Board Game Layout](./09-quest-board-game-layout.md)
10. [Controls, Hotkeys, Radar Clicks, And Input Rules](./10-controls-hotkeys-input-rules.md)
11. [Verification And Release Gate](./11-verification-release-gate.md)

## Suggested Subagent Split

- Contract agent: phase 01 protocol/reducer/server operation audit.
- World agent: phases 02 and 03 AOI/radar/known-intel/stealth/witness.
- UX shell agent: phase 04 modal/window/tutorial/dead-control cleanup.
- Content/catalog agent: phase 05 canonical content registry and seed data.
- Systems UI agent: phase 06 inventory/cargo/loadout/hangar.
- Economy agent: phase 07 shop/catalog/market/auction/premium.
- Planet agent: phase 08 claim/production/routes.
- Quest agent: phase 09 quest board/progression reward surface.
- QA agent: phases 10 and 11 smoke, screenshots, interaction gates.

Workers must follow `docs/symphony-worker-rules.md`, must not spawn subagents,
must not manage Symphony, and must not commit.

## Global Implementation Rules

- No fake real-mode gameplay values.
- No hidden world or procedural truth leaks.
- Client requests are intents only.
- Unsupported actions are absent, quiet, or locked with player-facing game copy.
- Do not show internal implementation copy to players.
- Newly enabled mutations must include server validation, protocol command,
  handler, reducer reconciliation, and targeted tests in the same slice.
  Browser/e2e coverage must be added through the future small per-flow harness,
  not a new monolith.
- Every phase inherits the goal requirement to read `docs/todo.md` before
  implementation, even when its local reading list is shorter.
- UI state may hold presentation state only: tabs, selection, windows, drag
  hover, local animation, tutorial topic.
- Server state owns all gameplay truth.

## Verification Commands

Use narrow commands while implementing. Current interim verification before
handoff:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
```

The Phase 11 final release gate must restore task-specific browser/e2e and
artifact verification after
[`browser-e2e-rebuild-plan.md`](./browser-e2e-rebuild-plan.md) is implemented.

UI screenshots for this task should live under:

```text
output/screenshots/task-001/
```

## Done Criteria

- Every phase file checklist is updated only after verified work.
- Every user-reported problem is fixed or tracked as a named blocker.
- Full verification passes.
- Desktop/tablet/mobile screenshots show visible progress toward the mockups.
- No client fake data or debug/internal player copy remains in normal player UI.
- Final browser/e2e artifact policy is enforced by script, not manual `find`:
  exact `output/screenshots/task-001/*` set, non-empty files, and current-run
  mtime.
