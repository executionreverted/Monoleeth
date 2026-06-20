# Workspace Triage 02 - Client UI

Date: 2026-06-21

Scope: HUD modal/tutorial/radar/input/quest remnants in `TASK-0185`,
`TASK-0186`, `TASK-0187`, `TASK-0190`, `TASK-0196`, and `TASK-0198`.

Baseline: authoritative main repo and this worker workspace are on
`master` at `9447f6b04edffb90003b597fc0f0309f30bcf6e5`. Main repo status was
clean during audit.

## Counts

| Classification | Count |
| --- | ---: |
| APPLIED | 0 |
| SUPERSEDED | 6 |
| AUDIT_ONLY | 0 |
| LOST_PATCH | 0 |
| NEEDS_HUMAN_REVIEW | 0 |

No lost patch found. No prod/client/server patch should be applied from these
workspaces.

## Method

- Read worker rules and Task 001 phase docs for radar, HUD/modal, input, quest,
  and Phase 07 economy context.
- For each workspace, inspected `git status --short`, `git diff --stat`, and
  `git diff --name-status`.
- Read dirty patch hunks.
- Compared patch intent and changed files against current main `master`.

## Per-Workspace Results

### TASK-0185 - SUPERSEDED

Workspace HEAD: `b6c7df232168897e9e5619fc42804eb160cdc17b`

Dirty files:

- `client/src/ui/hud.ts` - `6` changed lines.

Patch intent:

- Use `projection_window_size / 2` for minimap projection scale.
- Keep far remembered intel off radar instead of edge-clamping it into a fake
  nearby contact.

Main evidence:

- `client/src/ui/hud.ts:3189` to `client/src/ui/hud.ts:3206` uses
  `projection_window_size` and filters remembered points through
  `shouldRenderRememberedMinimapMemory`.
- `client/src/state/world-memory.ts:90` to `client/src/state/world-memory.ts:123`
  applies square projection filtering.
- `client/tests/browser-smoke.mjs:931` to
  `client/tests/browser-smoke.mjs:940` fails if far remembered intel renders as
  radar contact.

Recommended action: drop workspace patch. Main has broader implementation.

### TASK-0186 - SUPERSEDED

Workspace HEAD: `949c078b3e9bca0897ce02105599d6f5b011c6c6`

Dirty files:

- `client/src/state/reducer.test.ts`
- `client/tests/browser-smoke.mjs`
- `docs/plans/task-001/02-aoi-radar-map-visibility.md`

Diff stat: `73` insertions.

Patch intent:

- Add square/circular radar boundary coverage for `{x: 1000, y: 1000}`.
- Add fixture `npc-corner-01`.
- Document fixture smoke blocked by sandbox Chromium permissions.

Main evidence:

- `client/tests/browser-smoke.mjs:1063` to
  `client/tests/browser-smoke.mjs:1097` covers `npc-corner-01`, `2000`
  projection window, `1000` radar range, corner distance beyond circle, and
  `worker_projection`.
- `client/src/state/world-memory.test.ts:16` covers square projection corners.
- `docs/plans/task-001/02-aoi-radar-map-visibility.md:265` records corner
  contact and far-memory behavior as implemented.

Recommended action: drop workspace patch. Main has same test intent plus source
metadata and current docs.

### TASK-0187 - SUPERSEDED

Workspace HEAD: `3b7211e1968c99381304407a9e7b55c3f28342c7`

Dirty files:

- `client/src/state/reducer.test.ts`
- `client/src/state/reducer.ts`
- `client/src/state/types.ts`
- `client/src/state/world-memory.test.ts`
- `client/src/state/world-memory.ts`
- `client/src/ui/hud.ts`
- `docs/plans/task-001/02-aoi-radar-map-visibility.md`

Diff stat: `330` insertions, `27` deletions.

Patch intent:

- Preserve `sector_key`, `invalidated`, and `projection_source` on remembered
  intel/minimap contacts.
- Filter invalidated and wrong-sector memories.
- Keep stale current-sector memory clickable/detail-only.
- Preserve projection source through reducer rebuilds and HUD attributes.

Main evidence:

- `client/src/state/world-memory.ts:42` to
  `client/src/state/world-memory.ts:99` implements stale/invalidated/sector
  policy.
- `client/src/ui/hud.ts:3201` to `client/src/ui/hud.ts:3220` emits
  `data-projection-source`, sector, freshness, and detail action attrs.
- `client/tests/browser-smoke.mjs:999` to
  `client/tests/browser-smoke.mjs:1059` covers stale clickable memory,
  invalidated filtering, wrong-zone filtering, wrong-sector no-render, and no
  movement on stale detail click.
- `docs/plans/task-001/02-aoi-radar-map-visibility.md:269` to
  `docs/plans/task-001/02-aoi-radar-map-visibility.md:293` records source and
  remembered-intel policy; acceptance is checked at line `325`.

Recommended action: drop workspace patch. Main supersedes it with server/source
contract evidence and stronger smoke.

### TASK-0190 - SUPERSEDED

Workspace HEAD: `9738faa27731837f37171577fb12cb3729628b8d`

Dirty files:

- `client/src/ui/hud.ts`
- `client/tests/browser-smoke.mjs`
- `docs/plans/task-001/04-hud-modal-tutorial-window-system.md`

Diff stat: `83` insertions, `8` deletions.

Patch intent:

- Remove empty target `No lock`/`No drop` copy.
- Hide dead `Aim`.
- Show `Fire` only for NPC targets and `Gather`/`Approach` only for loot.
- Add browser smoke for empty, NPC, loot, and no-action target states.

Main evidence:

- `client/src/ui/hud.ts:2829` to `client/src/ui/hud.ts:2850` renders quiet
  empty target state and target-specific action buttons.
- `client/src/ui/hud.ts:3404` to `client/src/ui/hud.ts:3412` and
  `client/src/ui/hud.ts:3522` to `client/src/ui/hud.ts:3530` use `Standby`
  instead of dead lock/drop copy.
- `client/tests/browser-smoke.mjs:3999` to
  `client/tests/browser-smoke.mjs:4034` fails on visible dead target clutter.
- `docs/plans/task-001/04-hud-modal-tutorial-window-system.md:128` to
  `docs/plans/task-001/04-hud-modal-tutorial-window-system.md:136` records
  cleanup and smoke evidence.

Recommended action: drop workspace patch. Main has stronger target gating,
including hostile-only NPC fire.

### TASK-0196 - SUPERSEDED

Workspace HEAD: `1d65725437077afac353b2a7c0b4c50e35a456fe`

Dirty files:

- `client/src/input/world-input-authority.ts`
- `client/src/render/world-renderer.ts`
- `client/tests/browser-smoke.mjs`
- `docs/plans/task-001/10-controls-hotkeys-input-rules.md`

Diff stat: `83` insertions, `3` deletions.

Patch intent:

- Treat focused standalone HUD controls as world-input blockers after short HUD
  suppression expires.
- First blocked canvas click releases that HUD focus without sending `move_to`.
- Add browser smoke for focused standalone HUD control isolation.

Main evidence:

- `client/src/input/world-input-authority.ts:29` to
  `client/src/input/world-input-authority.ts:60` implements transient HUD
  focus release and focused HUD control blocking.
- `client/src/render/world-renderer.ts:336` to
  `client/src/render/world-renderer.ts:343` releases transient HUD focus when a
  blocked canvas click is swallowed.
- `client/tests/browser-smoke.mjs:3375` to
  `client/tests/browser-smoke.mjs:3394` verifies focused HUD control isolation
  and focus release.
- `docs/plans/task-001/10-controls-hotkeys-input-rules.md:216` to
  `docs/plans/task-001/10-controls-hotkeys-input-rules.md:224` records the
  behavior.

Recommended action: drop workspace patch. Main has same behavior with renamed
helpers and broader focus handling.

### TASK-0198 - SUPERSEDED

Workspace HEAD: `112e7a00f4144e1bbb93df8d9726f8bfa31408b7`

Dirty files:

- `client/src/protocol/envelope.test.ts`
- `client/src/state/reducer.test.ts`
- `client/src/state/reducer.ts`
- `client/src/state/types.ts`
- `docs/plans/task-001/09-quest-board-game-layout.md`
- `internal/game/server/quest_admin_observability_handlers.go`
- `internal/game/server/server_test.go`

Diff stat: `270` insertions, `66` deletions.

Patch intent:

- Add quest board action state: `can_accept`, `can_reroll`, `reset_at`,
  revision, offer locked reason, and claim state.
- Let `quest.accepted` remove stale accepted offer.
- Cover server/client quest state tests.

Main evidence:

- `internal/game/server/quest_admin_observability_handlers.go:24` to
  `internal/game/server/quest_admin_observability_handlers.go:68` has board,
  offer, and quest action-state payload fields.
- `internal/game/server/quest_admin_observability_handlers.go:251` to
  `internal/game/server/quest_admin_observability_handlers.go:263` queues
  accepted events with `accepted_offer_id` and returns refreshed board payload.
- `internal/game/server/quest_admin_observability_handlers.go:647` to
  `internal/game/server/quest_admin_observability_handlers.go:705` computes
  reroll/action state, reset, revision, and offer locked reason.
- `client/src/state/reducer.ts:2540` to `client/src/state/reducer.ts:2573`
  parses board action state, revision, and reset.
- `client/src/state/reducer.ts:2577` to `client/src/state/reducer.ts:2594`
  parses offer `can_accept`, expiry, and locked reason.
- `client/src/state/reducer.ts:3202` to `client/src/state/reducer.ts:3216`
  removes accepted offers from board state through `accepted_offer_id`.
- `client/src/ui/hud.ts:2740` to `client/src/ui/hud.ts:2750` renders real
  accept/claim buttons or quiet status, avoiding disabled primary claim clutter.
- `internal/game/server/server_test.go:2437` verifies quest board action state.

Recommended action: drop workspace patch. Main supersedes it with later quest
UI, stale revision, expiry, and display-metadata work. The older patch's
`claim_locked_reason` string field is not needed because main renders quiet
quest status instead of disabled `Claim Locked` controls.

## Follow-Up Prompt

None. `LOST_PATCH` count is zero.
