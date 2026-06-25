# Phase 14 — CMS Runtime Application & Live Content Safety

## Status
- State: Not started
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
- [ ] `[P:wave3/lane-H]` Add runtime content version pointer + apply path (atomic swap for safe-reload content classes).
- [ ] `[P:wave3/lane-H]` Make publish response report `runtime_applied`, `runtime_version`, `published_version`.
- [ ] `[P:wave3/lane-H]` Classify content: safe-live-reload vs requires-restart vs requires-migration.
- [ ] `[P:wave3/lane-I]` Add active-reference readers: market listings, equipped modules, loot drops, NPC templates, routes, shop locks.
- [ ] `[P:wave3/lane-I]` Block/flag publish when a changed id has active references that require quiescence.

## Server Ownership
- Runtime content is server-owned; admin role required; never leak hidden content to players (AGENTS.md).

## Smoke Tests (one assertion each)
- [ ] Publishing a safe-reload field (e.g. display name) is reflected by `content.catalog` without restart.
- [ ] Publishing a restart-required field returns `runtime_applied=false` with `pending_restart`.
- [ ] Publish is blocked/flagged when a changed module id is actively equipped.
- [ ] Publish response always reports published vs runtime version honestly.

## Done Criteria
- [ ] No silent content drift between published and live runtime.
- [ ] Code review HI-02 and HI-08 closed.

## Verification
```bash
go test ./internal/game/admin/... ./internal/game/content/... ./internal/game/server/... -run 'Publish|RuntimeApply|ContentSafety|Catalog' -count=1
go test ./... && git diff --check
```
