# Phase 11 - Verification And Release Gate

## Goal

Prove Task 001 actually fixed the playtest issues and did not regress the real
server-owned game rules. This phase is the final audit, screenshot, smoke, and
documentation update pass.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
all docs/plans/task-001 phase files
docs/todo.md
AGENTS.md
```

## Verification Matrix

Gameplay/server:

- UI/control matrix is current.
- No visible enabled control lacks a real server contract.
- Missing mutation blockers are still guarded by operation tests.
- Blocked contracts are explicitly listed with owner phase: crafting,
  inventory move, progression unlock, discovery claim, intel/coordinate,
  mail/social, building, route, auction grant, premium grant, and WASD if
  deferred. `stealth.toggle` is implemented, with module/cooldown/anti-spam
  balancing gates tracked separately.
- Economy, inventory, cargo, loadout, hangar, planet, route, combat, loot, scan,
  stealth, witness, and movement mutations remain server-authoritative.
- Fog visual removal did not remove hidden-data filtering.
- Disconnect/reconnect and movement settlement still work.
- Abuse/security evidence covers rate limits, idempotency, ledger references,
  leak-safe errors, duplicate event/retry handling, and no client-authored
  gameplay truth.
- Browser revoked-session/auth-expiry smoke revokes or expires a live session,
  sends a command, observes `auth_expired`, terminal socket close, pending
  command cleanup, and no stale gameplay state.
- Multi-client passive reconciliation is tested for market buyer/seller/passive
  viewer, auction outbid/refund/winner, and premium owner/stock viewer, or each
  missing fanout path is named as a backend blocker.

World/radar:

- Widened visibility window shows allowed nearby objects.
- Hidden objects do not leak.
- Radar/minimap shows known planets, enemies, loot, and non-hidden players.
- Radar contacts are clickable.
- Fog overlay is absent in playtest mode.
- Browser smoke no longer expects `fog.active === true` or `radar_range === 420`
  as a passing condition for Task 001.
- Stealth/witness is verified through a real scanner witness path. Because the
  Task 001 goal requires scanner reveal, a named blocker means Task 001 is not
  complete unless the goal is explicitly changed.

UI:

- `final-mockup.png` used as HUD shell target.
- Shop checked against `darkorbit-magaza-ornek-mockup.png`.
- Inventory/loadout checked against `darkorbit-envanter-ornek-layout.png`.
- Windows content-size correctly with max height.
- No generic `Inspect`.
- Contextual `?` opens tutorial/help.
- No internal/debug copy in any normal player UI.
- Shop smoke asserts game category rail, `shop.catalog` or equivalent metadata,
  selected detail, no raw ids, and no `server_recalculates` copy/state
  expectation in normal player UI.
- Impossible target actions are hidden or quiet.
- Quest board has list/detail/action layout and real or locked accept/claim/reroll.

Input:

- Movement continues while modals/windows are usable.
- Modal/window/HUD clicks do not leak into world movement.
- `1..6`, `Tab`, and WASD behavior match Phase 10.
- Valid `1` and `6` hotkeys emit exactly one expected command with world focus;
  focused/locked hotkeys emit none.
- Radar click behavior matches Phase 10: live target select, remembered planet
  detail, navigate from server-known coordinates, empty radar no-op.

Observability/release:

- Changed operations emit safe metrics/logs with op, request id/reference id,
  world/zone where relevant, and public error code without secrets or hidden
  truth.
- Ledger-changing flows expose reason/reference coverage in tests.
- Admin/observability surfaces needed for release inspection are role-gated and
  covered by screenshots or tests.
- `go test ./internal/game/observability/...` is included where packages exist.

## Subagent Review Additions - 2026-06-20

- Update smoke artifact paths from old `ui-implementation`, `ui-patch-2`, and
  `ui-patch-3` folders to `output/screenshots/task-001/` with the filenames in
  this gate.
- Remove smoke expectations that `radar_range === 420` or visual fog is active.
  Passing smoke for Task 001 must assert widened projection behavior, fog-off
  visuals, and no hidden-data serialization.
- Expand forbidden-copy assertions to visible text, `title`, and `aria-label`
  for `server-owned`, `server policy`, `server recalculates`,
  `No server-owned routes`, `server-side`, and `server contract`.
- Update unimplemented-control smoke so disabled primary placeholders cannot
  pass as acceptable player UI. Impossible actions should be hidden, quiet, or
  named blockers in the owning phase.
- Include client smoke gates for `?` tutorial help, content-sized windows,
  market/shop category detail, inventory/loadout tabs, planet claim/locked
  blocker path, quest accept/claim/reroll where available, radar contact click,
  `Tab`, hotkeys, and modal-moving click isolation.

## Second Subagent Review Additions - 2026-06-20

- Add the open browser revoked-session/auth-expiry scenario to the final gate;
  unit/server transport evidence alone is not enough.
- Do not let Task 001 pass with scanner witness blocked. The goal requires a
  real scanner reveal path, so a blocker keeps the task incomplete unless the
  goal changes.
- Enforce one screenshot path policy for release: exact flat filenames under
  `output/screenshots/task-001/`, non-empty, generated by the current run.
- Expand the screenshot matrix for tablet/mobile shop, inventory/loadout,
  hangar, planets, quests, and tutorial/help.
- Replace old shop smoke truth: no primary `Market/Sell/Auction/Premium` tab
  expectation, no `raw_ore` purchase fixture, and no `server_recalculates`
  visible/state expectation in normal player UI.
- Expand forbidden-copy scanning to normal-player text and attributes:
  `title`, `aria-label`, `alt`, and `placeholder`.
- Add blocked-contract gate coverage for `inventory.move` and
  `discovery.claim_planet` alongside crafting, building, routes, progression,
  intel/coordinate, mail/social, auction grant, premium grant, and WASD.
  `stealth.toggle` is now a real enabled command; its remaining gate is module,
  cooldown, anti-spam, and two-session witness browser coverage, not absence
  from the protocol.
- Add concrete Phase 10 smoke gates for valid `1`, valid `6`, `Tab`, WASD
  decision, radar contact click, and modal/window click isolation after timer
  expiry.
- Add a multi-client economy reconciliation gate or exact backend fanout
  blockers for market, auction, and premium passive viewers.
- Add observability/release gates from module 16: safe metrics/logs, public
  error mapping, ledger reason/reference coverage, and admin inspection.

## Third Subagent Review Additions - 2026-06-20

- The final screenshot gate can false-pass if it only runs `find`. Smoke must
  write final artifacts under `output/screenshots/task-001/`, and a script must
  assert the exact required filenames, non-empty file size, and mtime newer than
  the smoke start.
- Shop smoke must fail on old wrong truth: primary
  `Market/Sell/Auction/Premium` categories, `raw_ore` fixture purchase truth,
  and any `server_recalculates` DOM/state expectation in normal player mode.
- Planet smoke must fail when unavailable `Claim`, `Build`, `Upgrade`, `Route`,
  or `Auto` primary controls are visible disabled placeholders. Acceptable
  states are hidden, quiet game copy, or a named blocker in Phase 08.
- Browser smoke must include revoked-session/auth-expiry: revoke or expire a
  live session, send a command, observe `auth_expired`, terminal socket close,
  pending cleanup, and cleared gameplay state.
- Browser smoke must include scanner witness with multiple contexts:
  hidden target absent, scanner reveal returns safe
  `player_revealed`/`scan_revealed`, unrelated viewer cannot see, and expiry
  removes visibility.
- Quest smoke must click claim and reroll happy paths when fixtures expose them;
  otherwise the final gate must list the exact fixture blocker.
- Phase 10 smoke must include `Tab` target cycle, WASD implemented-or-absent
  check, radar/minimap contact behavior, and delayed modal/window focus
  isolation after the old suppression timeout would have expired.
- Final gate must include Phase 02 projection-source proof for worker entities,
  DB/procedural/live materialized entities, known intel, NPCs, loot, and
  players.

## Fourth Subagent Review Additions - 2026-06-20

- Add a current-run manifest at `output/screenshots/task-001/run-manifest.json`
  with smoke start time, git SHA/dirty state, command, auth mode, server URL,
  viewport list, screenshot hashes, and whether the run used real server state.
- Add a `client` task such as `npm run check:task-001` that runs the normal
  client check plus `node client/tests/verify-task-001-artifacts.mjs`; Phase 11
  should use that as the final client gate instead of relying on a manual
  artifact verifier command.
- Add a phase-closure table for Phase 01 through Phase 10. Every unchecked
  acceptance item must be marked `done`, `named blocker`, or `out-of-scope`,
  with owner file and TODO/blocker link; empty rows fail release.
- Screenshot/smoke artifacts must prove real-server provenance. Demo/fixture
  paths such as `?demo=1` or test-only fixtures may be used only when the
  manifest names them as non-release artifacts.
- Add three-session browser smoke: non-hidden player visible baseline, target
  stealths and disappears, scanner reveals with safe `scan_revealed`, unrelated
  viewer remains blind, expiry removes visibility, and witnessed target
  interaction allow/deny behavior is verified.
- Add negative real WebSocket smoke for `debug_snapshot` and `debug_spawn_npc`:
  non-dev server rejects them, and normal real-client command logs contain no
  debug operations.
- Add current-run security/observability artifact checks: server/browser logs
  are scanned for secrets, hidden truth, provider refs, witness internals, and
  debug operations; Module 16 load/simulation outputs are saved when run.
- Add mockup parity proof beyond file existence: a contact sheet or side-by-side
  screenshot set for final HUD, shop, and inventory mockups.

## Screenshot Requirements

Capture screenshots under:

```text
output/screenshots/task-001/
```

Required set:

- `live-desktop.png`
- `live-tablet.png`
- `live-mobile.png`
- `shop-desktop.png`
- `shop-tablet.png`
- `inventory-loadout-desktop.png`
- `inventory-loadout-tablet.png`
- `hangar-desktop.png`
- `planets-desktop.png`
- `quests-desktop.png`
- `radar-contacts-desktop.png`
- `modal-moving-desktop.png`
- `auth-expiry-desktop.png`
- `stealth-witness-scanner-desktop.png`
- `stealth-witness-unrelated-desktop.png`
- `stealth-witness-expired-desktop.png`
- `tab-target-cycle-desktop.png`
- `wasd-blocked-or-enabled-desktop.png`
- `planet-actions-locked-desktop.png`
- `admin-observability-desktop.png`
- `unauth-mobile.png`
- `disconnected-desktop.png`
- `empty-loading-desktop.png`
- `mockup-parity-contact-sheet.png`
- `run-manifest.json`

Add stealth/witness screenshots from the real scanner reveal path. If those are
absent because Phase 03 is blocked, Task 001 is still incomplete. Add WASD
screenshots if enabled.

Future per-flow browser/e2e harnesses must write these artifacts under
`output/screenshots/task-001/`, not the older `ui-implementation`,
`ui-patch-2`, or `ui-patch-3` directories.

Final Task 001 screenshots use the flat directory policy above. Per-phase
nested folders may be useful during development, but the final release gate
must verify the required flat filenames are present, non-empty, and generated by
the current smoke run.

Additional required final screenshots:

- `shop-mobile.png`
- `inventory-loadout-mobile.png`
- `hangar-tablet.png`
- `hangar-mobile.png`
- `planets-tablet.png`
- `planets-mobile.png`
- `quests-tablet.png`
- `quests-mobile.png`
- `tutorial-help-desktop.png`
- `tutorial-help-tablet.png`
- `tutorial-help-mobile.png`

## Commands

Run full verification before final handoff:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/observability/... -count=1
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/realtime -run 'TestOperationRegistry' -count=1
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/server -run 'Test.*(AOI|Minimap|Fog|Visibility|Hidden|Witness|Scan|Movement|Route|PlanetStorage|Auction|Repair|Death|Market|Premium|Economy|Catalog|Inventory|Loadout|Hangar|Ship|Quest|Progression)' -count=1
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/world/... ./internal/game/economy/... ./internal/game/modules ./internal/game/ships -count=1
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check:task-001
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
find output/screenshots/task-001 -maxdepth 1 -type f | sort
node client/tests/verify-task-001-artifacts.mjs
```

`npm run check` no longer includes browser smoke coverage. Browser/e2e coverage
is temporarily retired until a dedicated small per-flow harness replaces the
deleted monolithic suite.

Use narrower commands during implementation, but do not claim Task 001 complete
without the full gate.

## Audit Checklist

- [ ] Phase checklists reflect only verified completed work.
- [ ] `docs/todo.md` is updated for closed/open gameplay gaps.
- [ ] `docs/plans/ui-implementation/*` is updated if an active phase status
      changed.
- [ ] Browser smoke uses authenticated real server state.
- [ ] Screenshot/smoke run writes `run-manifest.json` with current-run
      provenance, hashes, git state, server URL, viewport list, and auth mode.
- [ ] Screenshots are current and named.
- [ ] Smoke captures Task 001 screenshots under `output/screenshots/task-001/`.
- [ ] Required screenshot filenames exist in the flat Task 001 directory,
      are non-empty, and come from the current smoke run.
- [ ] Screenshot artifact policy is script-enforced with exact filenames,
      non-empty sizes, and current-run mtimes.
- [ ] `npm run check:task-001` runs client check plus artifact verification.
- [ ] Phase 01-10 closure table marks every unchecked acceptance item as done,
      named blocker, or out-of-scope with owner file and TODO/blocker link.
- [ ] Smoke asserts no generic `Inspect`, `server-owned`, `server policy`,
      `server recalculates`, `No server-owned routes`, or similar internal copy
      in normal player UI text, `title`, and `aria-label`.
- [ ] Forbidden-copy smoke scans visible normal-player text plus `title`,
      `aria-label`, `alt`, and `placeholder`, excluding role-gated admin.
- [ ] No fake real-mode gameplay values exist in normal player UI.
- [ ] No internal/debug player-facing copy remains in normal player UI.
- [ ] No hidden/procedural truth is serialized.
- [ ] Radar/contact, keyboard, modal-moving, fog-off, quest-board, and
      stealth/witness gates are either verified or named-blocked.
- [ ] Scanner witness is verified through real scan command/browser flow; if it
      is blocked, Task 001 remains incomplete.
- [ ] Phase 10 smoke covers valid `1`, valid `6`, `Tab`, WASD decision, radar
      contact click, and modal/window click isolation after timer expiry.
- [ ] Three-session stealth/witness smoke covers visible baseline, stealth
      disappearance, scanner reveal, unrelated viewer absence, expiry removal,
      and witnessed-target interaction allow/deny.
- [ ] Negative real WebSocket smoke proves debug operations are forbidden in
      non-dev mode and never sent by normal real-client flows.
- [ ] Shop smoke fails on old `Market/Sell/Auction/Premium`, `raw_ore`, and
      `server_recalculates` expectations.
- [ ] Planet smoke fails on visible disabled unavailable primary controls.
- [ ] Projection-source proof covers worker, DB/procedural/live materialized,
      known intel, NPC, loot, and player sources.
- [ ] Server/browser logs from the release run are scanned for secrets, hidden
      truth, provider refs, witness internals, and debug operations.
- [ ] Mockup parity contact sheet or side-by-side artifact exists for final HUD,
      shop, and inventory references.
- [ ] Multi-client economy fanout/reconcile tests pass or exact backend
      blockers are listed in Phase 01/07 and `docs/todo.md`.
- [ ] `git diff --check` passes.

## Final Handoff Template

Use this shape in the implementation thread final response:

```text
Task 001 phase(s): ...
Changed: ...
Verified: ...
Screenshots: ...
Remaining blockers: ...
```
