# Task 001 Browser/E2E Rebuild Plan

## Purpose

Replace the deleted monolithic browser smoke suite with small per-flow browser
tests that prove real authenticated gameplay. This is required before Phase 11
can close.

Current truth:

- `client/tests/browser-smoke.mjs` is deleted.
- `npm run check` is intentionally smoke-free.
- `npm run check:task-001` does not exist yet.
- Screenshot/artifact verification does not exist yet.

## Rules

- No monolithic smoke file.
- Keep each flow focused and small. Soft cap: 250-300 lines per flow file.
- Shared helpers are allowed only for bootstrap, auth, command capture,
  screenshots, and common assertions.
- Release flows use authenticated real server state. Demo or fixture mode can
  exist only when the manifest marks the run as non-release.
- Add npm scripts only after their target files exist.
- Do not add `check:task-001` until artifact verification exists.

## Proposed Layout

```text
client/tests/e2e/run-flow.mjs
client/tests/e2e/shared/auth.mjs
client/tests/e2e/shared/browser.mjs
client/tests/e2e/shared/commands.mjs
client/tests/e2e/shared/screenshots.mjs
client/tests/e2e/shared/assertions.mjs
client/tests/e2e/flows/auth-session.e2e.mjs
client/tests/e2e/flows/world-radar.e2e.mjs
client/tests/e2e/flows/stealth-witness.e2e.mjs
client/tests/e2e/flows/hud-window-modal.e2e.mjs
client/tests/e2e/flows/catalog-copy.e2e.mjs
client/tests/e2e/flows/inventory-loadout-hangar.e2e.mjs
client/tests/e2e/flows/shop-economy.e2e.mjs
client/tests/e2e/flows/planets-routes.e2e.mjs
client/tests/e2e/flows/quests.e2e.mjs
client/tests/e2e/flows/controls-input.e2e.mjs
client/tests/e2e/flows/release-artifacts.e2e.mjs
```

## Flow Matrix

| Phase | Flow | Must Prove |
| --- | --- | --- |
| 01 | `auth-session` | session ready, auth expiry, pending cleanup, debug ops denied, no fake enabled controls |
| 02 | `world-radar` | bounded current-map membership, radar/stealth-filtered contacts, fog-off UI, hidden data absent, radar contacts clickable |
| 03 | `stealth-witness` | three-session baseline, stealth disappearance, scanner reveal, unrelated viewer blind, expiry |
| 04 | `hud-window-modal` | no `Inspect`, `?` help opens, content sizing, focus isolation, no internal copy |
| 05 | `catalog-copy` | display metadata is server-owned, no raw ids/snake_case in normal UI |
| 06 | `inventory-loadout-hangar` | inventory/cargo/loadout/hangar tabs, equip/unequip once, active ship state |
| 07 | `shop-economy` | category rail, product detail, buy panel, duplicate guards, no old raw ore/shop truth |
| 08 | `planets-routes` | planet catalog/detail refresh, storage/routes reconcile, unavailable actions hidden |
| 09 | `quests` | quest board layout, accept/reroll/claim or named fixture blocker, stale board handling |
| 10 | `controls-input` | `1`, `6`, `Tab`, radar clicks, modal blocks movement leaks, WASD decision |
| 11 | `release-artifacts` | manifest, exact screenshot set, hashes/mtimes, real-server provenance |

## Artifact Policy

Release artifacts live in:

```text
output/screenshots/task-001/
```

The final verifier must check:

- exact required filenames from Phase 11
- non-empty files
- mtime newer than run start
- screenshot hashes in `run-manifest.json`
- git SHA and dirty state
- server URL and auth mode
- viewport list
- whether the run used real server state

Per-flow development screenshots may use nested folders, but the release gate
must produce the flat Phase 11 screenshot set.

## Script Staging

Current scripts stay as-is:

```text
npm run check
```

Future scripts, added only when files exist:

```text
npm run e2e:task-001:auth-session
npm run e2e:task-001:shop-economy
npm run e2e:task-001:planets-routes
npm run check:task-001
```

`check:task-001` should run normal client check, the selected per-flow browser
suite, and the artifact verifier.

## Build Order

1. Shared bootstrap/auth/browser helpers.
2. `auth-session` and `hud-window-modal` flows.
3. `world-radar` and `controls-input` flows.
4. `catalog-copy`, `inventory-loadout-hangar`, and `shop-economy` flows.
5. `planets-routes` and `quests` flows.
6. `stealth-witness` three-session flow.
7. `release-artifacts` verifier and `check:task-001`.
