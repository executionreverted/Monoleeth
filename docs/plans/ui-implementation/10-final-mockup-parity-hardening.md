# Phase 10: Final Mockup Parity And End-To-End Hardening

## Status

- State: Completed for final hardening; remaining full-loop mutation contracts
  are tracked in `docs/todo.md`
- Owner: Whole-client polish and release readiness
- Depends on: Phase 09
- Unlocks: credible end-to-end playtest build

## Goal

Bring the real server-backed client close to `output/mockups/final-mockup.png`,
remove remaining fake/demo paths from default operation, and verify the full MVP
loop end to end across desktop and mobile viewports.

## Visual Contract

The mockup is the target, but real data wins over visual filling.

Must match direction:
- dense dark sci-fi console
- full-bleed space map
- top status bar with real sector/resources
- left ship/nav rail
- right planet/object rail
- bottom action bar and event log
- minimap/sector map
- crisp labels and stateful controls

Must avoid:
- marketing layout
- fake decorative panels pretending to be game data
- text explaining how the UI works
- hidden debug data in visible UI
- UI cards nested inside cards
- overlapping mobile HUD text

## Implementation Notes

- 2026-06-19 UI rework slice 1 replaced the visible stacked HUD shell with a
  fixed cockpit overlay copied from `output/mockups/final-mockup.png`: full
  top status strip, left ship card/nav, right planets/target/sector map stack,
  bottom-left log, bottom-center action rail, and layered grid/star/nebula
  background. Gameplay values still come from authenticated server snapshots,
  hidden/dev panels remain non-default, and mail/social affordances stay locked
  without fake counts.
- 2026-06-19 UI rework slice 2 added a first-class HUD window registry and
  reusable modal layer. Cargo, economy, quests, intel/scanner, systems, and
  admin-only ops now open as focused cockpit windows from the left nav, with
  compact modal detail support, close-button/Escape/backdrop dismissal, and a
  mobile bottom-sheet layout. The hidden server-backed panel anchors remain for
  existing command selectors and smoke coverage, but the visible default no
  longer dumps secondary systems into the central world.
- 2026-06-19 UI rework slice 3 made browser movement server-timed and visually
  continuous. AOI/player payloads now expose safe public movement timing
  (origin, target, speed, start, arrival), mid-route clicks begin from the
  server-computed current position, immediate move spam is rate-limited without
  changing the authoritative route, and the renderer interpolates the player
  plus parallax camera background from server snapshots/events.
- 2026-06-19 UI rework slice 5 exposed server-owned pickup range and basic
  laser energy/cooldown metadata through `stats.snapshot`, then used those
  values only as UI hints. Clicking visible loot now selects it, moves toward it
  when out of range, picks it up when in range, or logs a compact reason while
  waiting for server range/position state. The action rail and target panel now
  show cooldown, capacitor, range, and approach/gather availability without
  client-authored combat or loot truth.
- 2026-06-19 UI rework slice 6 made AOI-visible entities read closer to the
  mockup without adding fake map objects: player ships have radar rings and
  engine glow, NPCs use hostile diamond/swarm language, loot uses an amber
  cache/crate marker, unknown signals use the HUD question/ring treatment, and
  minimap points carry both disposition and entity-type styling. Fixture smoke
  verifies player/NPC/loot/signal presence, target-kind selection, and updated
  screenshots.
- The default browser path remains real/authenticated only. Demo fixtures are
  still available for explicit dev/test fixture mode, but they are dev-only lazy
  imports and the production bundle scan fails on fixture labels.
- Topbar parity now includes real sector, danger, energy, cargo, credits, and
  capacitor values. Missing values render locked/empty, not fake numbers.
- The bottom action rail now exposes the real `scan.pulse` action alongside
  real laser/loot controls and locked future skill slots.
- Tablet and mobile layouts use wrapping top status cells and compact action
  labels so HUD text fits without horizontal body overflow.
- Final screenshots live under `output/screenshots/ui-implementation/10/`:
  `unauth-mobile.png`, `unauth-tablet.png`, `unauth-desktop.png`,
  `live-mobile.png`, `live-tablet.png`, `live-desktop.png`, and
  `live-admin-desktop.png`. The explicit fixture-only marker parity artifacts
  are `fixture-mobile.png` and `fixture-desktop.png`.

## End-To-End MVP Loop

Verify the browser can complete:

```text
register/login
spawn starter ship
move
see visible NPCs only
fight
kill NPC
loot raw materials into cargo
gain XP/rank progress
equip/craft module
discover planet through scanner
claim planet
produce resources
route resources
sell/buy on market
repair after death
inspect quest progress/rewards
logout/reconnect and reconcile snapshot
```

If any item is not implemented by the time this phase starts, record it in
`docs/todo.md` with the exact missing server/client contract.

## Feature Audit Table

Before visual sign-off, create or update an audit table covering Phases 01-09.
Each row must include:
- phase
- backend feature
- command/query/event names
- UI entry point
- real-server positive test
- abuse/negative test
- screenshot or browser-smoke artifact
- blocker link in `docs/todo.md` if absent

No exposed backend feature is considered complete because it appears in a mockup
or fixture. It needs a browser path that talks to the real Go server.

## Feature Audit Result

| Phase | Backend Feature | Command/Query/Event Names | UI Entry Point | Real-Server Positive Test | Abuse/Negative Test | Screenshot Artifact | Blocker |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 01 | Mail/password accounts, sessions, admin seed | `POST /api/auth/register`, `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/session` | Auth panel | Browser smoke registers, logs in, restores, logs out | Smoke invalid login; Go auth/session/origin tests | `10/unauth-*.png`, `10/live-*.png` | None |
| 02 | Go server transport and authenticated realtime | `/ws`, `session.snapshot`, `world.snapshot`, `session.ready` | HUD shell and live canvas | Browser smoke boots `cmd/game-server` and opens `/ws` with cookie session | Gateway tests resolve identity server-side and reject missing sessions/origins | `10/live-desktop.png` | None |
| 03 | Real client shell and explicit demo isolation | Session restore, real mode, explicit `?demo=1` fixture | Auth shell, HUD hidden while unauthenticated | Smoke proves unauthenticated state has no gameplay snapshots | Client lint, bundle scan, and smoke leak scan block default fake/demo labels | `10/unauth-mobile.png` | None |
| 04 | World, AOI, movement, minimap | `world.snapshot`, `move_to`, `stop`, `entity.entered`, `entity.left`, `position.corrected` | Center map, minimap, Stop/Sync controls | Smoke moves the real player, verifies server-timed interpolation/re-click origin, and checks canvas pixels | Hidden planet signal absent from smoke state; protocol rejects forbidden hidden keys; immediate move spam returns `ERR_RATE_LIMITED` without changing the route | `10/live-tablet.png` | None |
| 05 | Combat, loot, death repair | `combat.use_skill`, `loot.pickup`, `death.repair_quote`, `death.repair_ship`, `combat.*`, `loot.*`, `death.*` | Target panel, ship panel, action rail, log | Smoke kills visible NPC and picks up visible drop | Go combat/loot/repair tests cover hidden/out-of-range/duplicate/disabled paths | `10/live-desktop.png` | Death/respawn E2E in `docs/todo.md` |
| 06 | Progression, inventory, hangar, loadout, crafting read models | `progression.snapshot`, `inventory.snapshot`, `hangar.snapshot`, `loadout.snapshot`, `stats.snapshot`, `crafting.recipes` | Status, cargo, economy, systems panels | Smoke asserts real snapshots and recipes after login/reconnect | Client trust-boundary lint and reducer/protocol tests reject forged identity/value payloads | `10/live-desktop.png` | Equip/craft mutations in `docs/todo.md` |
| 07 | Discovery, scanner, planet/production/route read models | `scan.pulse`, `discovery.known_planets`, `discovery.planet_detail`, `planet.production_summary`, `planet.storage_summary`, `route.list`, `route.snapshot`, `scan.*` | Intel panel, sector map, Scan action | Smoke runs `scan.pulse`, discovers a server planet, and reconciles XP/intel | Smoke verifies hidden signal is not serialized; Go discovery tests cover hidden/fog rules | `10/live-mobile.png` | Claim/build/route mutations in `docs/todo.md` |
| 08 | Market, auction, premium economy | `market.search`, `market.create_listing`, `market.buy`, `market.cancel`, `auction.search`, `auction.bid`, `auction.buy_now`, `auction.claim_grant`, `premium.entitlements`, `premium.claim`, `premium.purchase_weekly_xcore`, economy events | Shop/economy panel | Smoke buys/cancels listings, bids/buy-now, purchases/claims premium | Go market/auction/premium tests cover server-calculated totals, replay, race, and escrow safety | `10/live-desktop.png` | None |
| 09 | Quests, admin, observability, release gates | `quest.board`, `quest.accept`, `quest.progress`, `quest.claim_reward`, `quest.reroll`, `admin.*`, `observability.*`, quest/admin/observability events | Quest window, admin-only Ops window | Smoke accepts/rerolls quests and seeded admin loads Ops/Gate/Abuse reports | Go tests cover non-admin rejection, redaction, idempotent rewards, release schema | `10/live-admin-desktop.png` | None |

## UI Hardening

- remove default demo data
- remove placeholder entity names from visible UI
- replace "debug snapshot" control with real sync/reconnect behavior
- add loading/empty/error states for every panel
- add responsive layout for mobile/tablet/desktop
- add keyboard/touch support for action bar
- add accessible labels without visible tutorial copy
- add stable dimensions for action bar, minimap, panels, and topbar
- verify text does not overflow

## Test And Verification Matrix

Backend:
- full `go test ./...`
- focused gateway/auth/runtime tests
- abuse/security tests for all exposed commands

Client:
- typecheck
- unit tests
- reducer/protocol tests
- browser smoke against real Go server
- desktop screenshot
- mobile screenshot
- canvas nonblank pixel checks
- hidden text/state scan

E2E:
- login/logout/reconnect
- movement
- combat skills: visible target positive, hidden/out-of-range/no-energy negative
- loot pickup: visible owned/public positive, hidden/far/duplicate negative
- death/repair: disabled positive, insufficient wallet/stale quote negative
- cargo/wallet updates: committed snapshot positive, forged totals negative
- inventory moves: owned positive, unowned/negative/excess negative
- hangar activation: owned positive, unowned/unusable negative
- loadout/crafting: equip/craft positive, duplicate/unowned/stale job negative
- intel/coordinate items: visible/share positive, hidden/unowned negative
- scan/planet/building/storage: visible positive, hidden/capacity/rank negative
- per-map scanner/drop seed matrix: public `1-1`/`1-2`/`1-3` active-map
  known/minimap memory scope plus NPC drop profile/table rows with internal
  map/candidate/drop metadata leak canaries
- routes/production settlement: eligible positive, duplicate/window/capacity negative
- market/auction: buy/bid/claim positive, race/stale/tampered total negative
- premium purchase/claim: eligible positive, replay/limit/stock negative
- quests: accept/reroll/reward positive, duplicate/forged progress negative
- admin/release gates: admin positive, non-admin forbidden negative

## Real-Server E2E Runner Requirements

The final browser runner must:
- boot `cmd/game-server` with deterministic dev seed/config
- create/login through mail/password auth
- use cookie session and `/ws` handshake
- drive commands through the real protocol
- assert snapshots/events from the Go runtime
- avoid JavaScript mock fixtures, default demo state, or client-authored
  gameplay truth

The runner in `client/tests/browser-smoke.mjs` covers the real implemented loop:
register/login, authenticated `/ws`, starter ship spawn, movement, visible NPC
combat, loot pickup, XP/cargo/wallet reconciliation, scanner discovery,
market/auction/premium operations, quest/admin surfaces, logout, reconnect, and
leak scans. Missing equip/craft, planet claim/build, route mutation, and
death/respawn contracts are recorded in `docs/todo.md` rather than faked.

## Region Checklist

- [x] Topbar: real sector, danger, energy, cargo, credits, and capacitor; no
      fake mail/social counts.
- [x] Left rail: real ship/player/status, plus cargo, economy, quest,
      intel/scanner, systems, and admin-only ops window toggles with
      loading/empty states.
- [x] Center map: full-bleed canvas with nonblank pixel check, AOI-visible
      entities only, and distinct player/NPC/loot/signal markers.
- [x] Bottom action/log bar: real laser, scan, loot controls; locked future
      slots; server event log.
- [x] Right rail: selected target, ship repair controls, sector map, intel, and
      admin-only ops when role-gated.
- [x] Minimap: server minimap projection with hidden signal exclusion.
- [x] Overlays/panels: unauthenticated and disconnected paths hide gameplay
      truth instead of showing fixtures; authenticated panels open as focused
      HUD windows and modal details with tested close/Escape/backdrop behavior.

## Leak Scan Matrix

- DOM/body text: `assertNoForbiddenLeaks` in browser smoke.
- App state: smoke-only `window.__SPACE_MORPG_SMOKE_STATE__` scan.
- Browser storage and cookie string: `assertNoForbiddenLeaks` storage scan.
- WebSocket payloads: protocol parser rejects forbidden server payload keys;
  real smoke runs through Go `/ws`.
- Production bundle: `npm run bundle-scan` builds `dist/` and fails on demo
  fixture labels or fake placeholder markers.
- Screenshots: generated after DOM/state/storage checks and visually inspected
  for the Phase 10 region checklist.

## Real State Rules Audit

- [x] Fake gameplay values: `createInitialState` starts every server-owned
      gameplay surface as null/empty, `clearGameplay` resets every gameplay
      panel/read model on auth restore, logout, auth failure, expiry, and demo
      entry, and reducer tests assert the full field list.
- [x] Offline/unauthenticated states: CSS hides `.hud-host` for `logged_out`,
      `restoring`, and `auth_expired`; Playwright smoke verifies login/error
      states have no player snapshot, cargo, wallet, inventory, loadout,
      crafting, or visible entities.
- [x] Demo fixtures: `?demo=1` is the only demo entry point, demo state is
      lazy-imported only under `import.meta.env.DEV`, and `npm run bundle-scan`
      fails if fixture labels/IDs appear in the production bundle.
- [x] Client intents only: command builder tests cover movement/combat/loot,
      market/auction/premium, and quest/admin commands as selector/intent
      payloads; unsafe client-authored identity, damage, wallet, quest progress,
      reward, scan, hidden, seed, and loot-table fields are rejected before send.

## Local Run And Release Gate Summary

- Local run instructions live in `docs/running-local-game.md`.
- Client release gate is `npm --cache /tmp/gameproject-npm-cache run check`;
  it runs lint, typecheck, unit tests, production build, bundle scan, and
  real-server Playwright smoke.
- Full handoff gate remains `go test ./...`,
  `npm --cache /tmp/gameproject-npm-cache run check` in `client/`, and
  `git diff --check`.

## TODO

- [x] Compare current UI screenshots against `output/mockups/final-mockup.png`.
- [x] Implement remaining visual layout gaps without faking data.
- [x] Add server-event driven target reticles, enemy HP/shield readouts, combat
      laser/damage/miss feedback, loot-spawn feedback, and pickup reward
      summaries without trusting client-authored damage or rewards.
- [x] Replace debug/dev controls from production default UI.
- [x] Add final real-server browser smoke script.
- [x] Add E2E scenario runner for the implemented MVP loop and document missing
      contracts for absent loop steps.
- [x] Add feature audit table for every Phase 01-09 command/query/event and UI
      entry point.
- [x] Add desktop screenshot verification.
- [x] Add mobile screenshot verification.
- [x] Add tablet screenshot verification.
- [x] Add region checklist for topbar, left rail, center map, bottom action/log
      bar, right rail, minimap, and overlays.
- [x] Add hidden/debug data leak scan across DOM, app state, browser storage,
      screenshots, WebSocket frames, production bundle text, and demo fixture
      labels.
- [x] Add reconnect reconciliation test.
- [x] Add docs for running the game locally.
- [x] Add release gate checklist summary.
- [x] Move any remaining feature gaps to `docs/todo.md`.

## Abuse And Safety Checklist

- [x] No hidden seeds/debug metadata appear in DOM, client state, screenshots, or
      WebSocket payloads.
- [x] No fake HP/cargo/wallet/entity/quest/planet labels appear in default DOM,
      storage, screenshots, WebSocket frames, or production bundle.
- [x] No command trusts client-authored identity or value totals.
- [x] No panel displays stale local optimistic state as committed truth.
- [x] All admin/premium/economy operations are role/policy gated.
- [x] Rate-limit posture exists for every exposed operation.
- [x] Reconnect snapshot reconciles all gameplay panels.

## Done Criteria

- UI visibly follows the final mockup direction.
- Implemented MVP slices are playable from browser with real server state; absent
  equip/craft, planet/building, route mutation, and death/respawn contracts are
  documented in `docs/todo.md`.
- Default app has no mock gameplay.
- All exposed backend features from Phases 01-09 have audited browser paths,
  real-server positive tests, and abuse/negative tests.
- Remaining non-blocking gaps are documented in `docs/todo.md`.
- Full backend, client, browser, and E2E verification pass.
