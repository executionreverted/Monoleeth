# Phase 10: Final Mockup Parity And End-To-End Hardening

## Status

- State: Planned
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

## TODO

- [ ] Compare current UI screenshots against `output/mockups/final-mockup.png`.
- [ ] Implement remaining visual layout gaps without faking data.
- [ ] Replace debug/dev controls from production default UI.
- [ ] Add final real-server browser smoke script.
- [ ] Add E2E scenario runner for the MVP loop.
- [ ] Add feature audit table for every Phase 01-09 command/query/event and UI
      entry point.
- [ ] Add desktop screenshot verification.
- [ ] Add mobile screenshot verification.
- [ ] Add tablet screenshot verification.
- [ ] Add region checklist for topbar, left rail, center map, bottom action/log
      bar, right rail, minimap, and overlays.
- [ ] Add hidden/debug data leak scan across DOM, app state, browser storage,
      screenshots, WebSocket frames, production bundle text, and demo fixture
      labels.
- [ ] Add reconnect reconciliation test.
- [ ] Add docs for running the game locally.
- [ ] Add release gate checklist summary.
- [ ] Move any remaining feature gaps to `docs/todo.md`.

## Abuse And Safety Checklist

- [ ] No hidden seeds/debug metadata appear in DOM, client state, screenshots, or
      WebSocket payloads.
- [ ] No fake HP/cargo/wallet/entity/quest/planet labels appear in default DOM,
      storage, screenshots, WebSocket frames, or production bundle.
- [ ] No command trusts client-authored identity or value totals.
- [ ] No panel displays stale local optimistic state as committed truth.
- [ ] All admin/premium/economy operations are role/policy gated.
- [ ] Rate-limit posture exists for every exposed operation.
- [ ] Reconnect snapshot reconciles all gameplay panels.

## Done Criteria

- UI visibly follows the final mockup direction.
- Full MVP loop is playable from browser with real server state.
- Default app has no mock gameplay.
- All exposed backend features from Phases 01-09 have audited browser paths,
  real-server positive tests, and abuse/negative tests.
- Remaining non-blocking gaps are documented in `docs/todo.md`.
- Full backend, client, browser, and E2E verification pass.
