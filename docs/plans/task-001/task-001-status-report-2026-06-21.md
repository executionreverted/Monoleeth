# Task 001 Status Report - 2026-06-21

## Summary

Task 001 is not done.

Current phase checklist progress:

```text
137 / 227 checked = 60.4%
90 open checklist items remain.
```

This count is documentation state only. It does not prove release readiness.
The deleted monolithic browser smoke suite means browser/e2e proof is currently
absent until the per-flow harness is rebuilt.

Do not close more checklist items from this report. Close them only from
verified implementation and test evidence.

## Git State

Audit-time state:

```text
branch: master
tracking: master...origin/master [ahead 30]
latest commit: 27fb02c test: remove monolithic browser smoke
worktree before this docs update: clean
push: not done
```

## Symphony State

Fresh read-only Symphony workers reviewed the plan state:

- `TASK-0245` - status/count audit.
- `TASK-0246` - browser/e2e rebuild audit.
- `TASK-0247` - next executable order audit.

They made no repo edits and no commits. Their findings are reflected here and
in the browser/e2e rebuild plan.

## Phase Counts

| Phase | Done | Open | Total | State |
| --- | ---: | ---: | ---: | --- |
| 01 gameplay connection | 22 | 0 | 22 | done by checklist |
| 02 AOI/radar/map | 20 | 0 | 20 | done by checklist |
| 03 stealth/scan/witness | 14 | 8 | 22 | partly done |
| 04 HUD/modal/tutorial | 10 | 3 | 13 | partly done |
| 05 content catalog | 4 | 15 | 19 | blocker |
| 06 inventory/cargo/loadout/hangar | 12 | 11 | 23 | partly done |
| 07 shop/catalog | 18 | 5 | 23 | partly done |
| 08 planets/routes/claim | 0 | 20 | 20 | blocker |
| 09 quests | 17 | 0 | 17 | done by checklist |
| 10 controls/input | 20 | 0 | 20 | done by checklist |
| 11 release gate | 0 | 28 | 28 | blocker |

## Major Blockers

- Phase 05 still blocks downstream UI polish: content registry breadth,
  cross-catalog validation, display metadata, NPC/loot/content names,
  non-starter acquisition, and `energy_cell_batch` mismatch.
- Phase 07 still needs final shop layout, buy/reconcile behavior, duplicate
  guards for economy mutations, and real/hidden grant handling.
- Phase 06 still has inventory/hangar/crafting policy gaps that should be
  closed before final release proof.
- Phase 08 is not implemented by checklist: planet catalog/detail, claim,
  production, storage, routes, and capability-driven actions remain open.
- Phase 03 still needs scanner witness browser/UI closure.
- Phase 04 still needs final copy/focus/palette gates.
- Phase 11 cannot run today: no per-flow browser/e2e harness, no
  `check:task-001`, no artifact verifier, no current screenshot manifest.

## Browser/E2E State

`client/tests/browser-smoke.mjs` was deleted in commit `27fb02c`.

Current truth:

- `npm run check` does not run browser smoke.
- `npm run check:task-001` does not exist.
- `client/tests/verify-task-001-artifacts.mjs` does not exist.
- Existing phase notes that say "Browser smoke now" are historical evidence or
  future acceptance targets, not current release proof.

Rebuild plan:

- Use [`browser-e2e-rebuild-plan.md`](./browser-e2e-rebuild-plan.md).
- Keep flows small and per-domain.
- Do not recreate a monolithic smoke file.

## Next Execution Order

Primary implementation spine:

1. Phase 05 - content catalog blockers.
2. Phase 07 - shop/catalog/economy UI and reconciliation.
3. Phase 08 - planets, claim, production, routes.
4. Phase 11 - browser/e2e rebuild and release gate.

Mandatory side gates before Phase 11:

- Phase 06 inventory/hangar/crafting policy and display gaps.
- Phase 03 scanner witness browser/UI proof.
- Phase 04 final modal/help/copy/palette cleanup.

## First Worker Tasks Next

- Phase 05 worker: add or tighten canonical content/reference validation and
  block raw-id display fallbacks.
- Phase 07 worker: finish shop category/detail/buy layout and one-click
  pending guards against real server operations.
- Phase 08 worker: clean planet catalog/detail read path first, then expose
  claim/build/route only when server contracts are real.
- Phase 11 worker: rebuild browser/e2e as small per-flow files after the above
  systems have real behavior to prove.
