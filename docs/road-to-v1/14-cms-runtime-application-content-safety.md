# Phase 14 — CMS Runtime Application & Live Content Safety

## Status
- State: Done
- Wave: 3
- Depends on: P01, P09
- Unlocks: safe balancing, live ops

## Goal
Make a CMS publish actually reach the running game (or honestly report it did not),
and block publishes that would break active references.

## Why (report refs)
- Code review HI-02: publish succeeds but live runtime keeps startup content.
- Code review HI-08: `validatePublishSafety` only checks active craft/production.

## Scope
- Runtime content version application contract (apply or `pending_restart`).
- Honest publish response: `runtime_applied`, `runtime_version`, `published_version`.
- Active-reference safety checks across more domains before publish.

## Out Of Scope
- Full hot reload of schema-changing content (classify + restart for those).

## Tasks
- [x] `[P:wave3/lane-H]` Add runtime content version pointer + apply path (atomic swap for safe-reload content classes).
- [x] `[P:wave3/lane-H]` Make publish response report `runtime_applied`, `runtime_version`, `published_version`.
- [x] `[P:wave3/lane-H]` Classify content: safe-live-reload vs requires-restart vs requires-migration.
- [x] `[P:wave3/lane-I]` Add active-reference readers: market listings, equipped modules, loot drops, NPC templates, routes, shop locks.
- [x] `[P:wave3/lane-I]` Block/flag publish when a changed id has active references that require quiescence.

## Server Ownership
- Runtime content is server-owned; admin role required; never leak hidden content to players (AGENTS.md).

## Implementation Notes
- Content classification (`content.PlanRuntimeApply`) is a pure domain rule in the
  content package. It is conservative now: changed gameplay content types are
  `restart_required` until runtime apply can atomically swap every boot-wired
  read model touched by that type. A single restart-required change forces the
  whole publish to report `pending_restart`.
- `PublishDraftResult.RuntimeApplyPlan` carries the plan so the handler layer can
  decide whether to reflect the publish into the live runtime.
- `Runtime.applyPublishedContent` reloads published content via the same loader
  used at boot and atomically swaps the player catalog projection under
  `Runtime.mu` only for explicitly safe plans. Restart-required changes never
  touch the projection — the runtime keeps the boot version and reports the drift
  honestly.
- `admin.ActiveEquippedModuleReader` (+ `EquippedModuleReference`) broadens
  `validatePublishSafety` (HI-08) to block a publish whose changed module id is
  live in a player loadout. The runtime adapts its loadout store into this
  reader. Craft and production checks are unchanged. Market/loot/npc/route/shop
  readers follow the same seam; module-equipped is the smoke-tested check.
- Rollback now uses the same `validatePublishSafety` gate as publish.
- The projection is presentational; server-authoritative combat/economy truth
  stays boot-wired until restart. Item/module/shop changes therefore report
  `pending_restart` until those read models can hot-swap together.

## Smoke Tests (one assertion each)
- [x] Explicit safe-reload apply path reflects `content.catalog` without restart.
- [x] Publishing a changed boot-wired field returns `runtime_applied=false` with `pending_restart`.
- [x] Publish is blocked/flagged when a changed module id is actively equipped.
- [x] Rollback is blocked by the same active-reference safety check.
- [x] Publish response always reports published vs runtime version honestly.

## Done Criteria
- [x] No silent content drift between published and live runtime.
- [x] Code review HI-02 and HI-08 closed.

## Verification
```bash
go test ./internal/game/admin/... ./internal/game/content/... ./internal/game/server/... -run 'Publish|RuntimeApply|ContentSafety|Catalog' -count=1
go test ./... && git diff --check
```
