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
  progression unlock, intel/coordinate, mail/social, building, route, auction
  grant, premium grant, stealth/WASD if deferred.
- Economy, inventory, cargo, loadout, hangar, planet, route, combat, loot, scan,
  stealth, witness, and movement mutations remain server-authoritative.
- Fog visual removal did not remove hidden-data filtering.
- Disconnect/reconnect and movement settlement still work.
- Abuse/security evidence covers rate limits, idempotency, ledger references,
  leak-safe errors, duplicate event/retry handling, and no client-authored
  gameplay truth.

World/radar:

- Widened visibility window shows allowed nearby objects.
- Hidden objects do not leak.
- Radar/minimap shows known planets, enemies, loot, and non-hidden players.
- Radar contacts are clickable.
- Fog overlay is absent in playtest mode.
- Browser smoke no longer expects `fog.active === true` or `radar_range === 420`
  as a passing condition for Task 001.
- Stealth/witness is verified unless Phase 03 records a named blocker.

UI:

- `final-mockup.png` used as HUD shell target.
- Shop checked against `darkorbit-magaza-ornek-mockup.png`.
- Inventory/loadout checked against `darkorbit-envanter-ornek-layout.png`.
- Windows content-size correctly with max height.
- No generic `Inspect`.
- Contextual `?` opens tutorial/help.
- No internal/debug copy in any normal player UI.
- Impossible target actions are hidden or quiet.
- Quest board has list/detail/action layout and real or locked accept/claim/reroll.

Input:

- Movement continues while modals/windows are usable.
- Modal/window/HUD clicks do not leak into world movement.
- `1..6`, `Tab`, and WASD behavior match Phase 10.
- Radar click behavior matches Phase 10: live target select, remembered planet
  detail, navigate from server-known coordinates, empty radar no-op.

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

Add stealth/witness screenshots unless Phase 03 records a named blocker. Add
WASD screenshots if enabled.

`client/tests/browser-smoke.mjs` must write these artifacts under
`output/screenshots/task-001/`, not the older `ui-implementation`,
`ui-patch-2`, or `ui-patch-3` directories.

## Commands

Run full verification before final handoff:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/observability -count=1
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/realtime -run 'TestOperationRegistry' -count=1
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/server -run 'Test.*(AOI|Minimap|Fog|Visibility|Hidden|Witness|Scan|Movement|Route|PlanetStorage|Auction|Repair|Death|Market|Premium|Economy|Catalog|Inventory|Loadout|Hangar|Ship|Quest|Progression)' -count=1
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/world/... ./internal/game/economy/... ./internal/game/modules ./internal/game/ships -count=1
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
find output/screenshots/task-001 -maxdepth 1 -type f | sort
```

`npm run check` already includes the smoke suite. Run `npm --cache
/tmp/gameproject-npm-cache run smoke` separately only when re-capturing browser
artifacts without repeating the whole check.

Use narrower commands during implementation, but do not claim Task 001 complete
without the full gate.

## Audit Checklist

- [ ] Phase checklists reflect only verified completed work.
- [ ] `docs/todo.md` is updated for closed/open gameplay gaps.
- [ ] `docs/plans/ui-implementation/*` is updated if an active phase status
      changed.
- [ ] Browser smoke uses authenticated real server state.
- [ ] Screenshots are current and named.
- [ ] Smoke captures Task 001 screenshots under `output/screenshots/task-001/`.
- [ ] Smoke asserts no generic `Inspect`, `server-owned`, `server policy`,
      `server recalculates`, `No server-owned routes`, or similar internal copy
      in normal player UI text, `title`, and `aria-label`.
- [ ] No fake real-mode gameplay values exist in normal player UI.
- [ ] No internal/debug player-facing copy remains in normal player UI.
- [ ] No hidden/procedural truth is serialized.
- [ ] Radar/contact, keyboard, modal-moving, fog-off, quest-board, and
      stealth/witness gates are either verified or named-blocked.
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
