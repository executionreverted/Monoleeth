# Phase 09 — CMS Completion & Balance Telemetry

## Status
- State: Not started
- Wave: 3
- Depends on: P01
- Unlocks: live ops content (P11/P12)

## Goal
Finish the open CMS items (diff, audit action, quest publish, concurrency, rate
limit) and add economy source/sink telemetry so content balancing is measurable.

## Why (report refs)
- Feature-gap §4.6, §13: CMS strong but diff/audit/quest/hot-reload incomplete.
- CMS rework docs: explicit "Remaining" items in phases 08/11.

## Scope
- `admin.content.diff` API + view.
- Audit `action` field/migration + broader scrubber policy.
- Quest rows in publish/rollback; old-version accepted-quest compatibility test.
- Balance telemetry around credit/material sources and sinks.

## Out Of Scope
- Runtime hot reload (keep restart-based; document deferral).

## Tasks
- [ ] `[P:wave3/lane-F]` Implement `admin.content.diff` (admin-only, safe payloads) + UI diff view.
- [ ] `[P:wave3/lane-F]` Add audit `action` column/migration; expand secret/seed scrubber policy.
- [ ] `[P:wave3/lane-F]` Include quest rows in publish/rollback; add accepted-old-quest compatibility test.
- [ ] `[P:wave3/lane-F]` Add live-Postgres duplicate/concurrent publish coverage + zero-mutation rate-limit coverage.
- [ ] `[P:wave3/lane-G]` Add economy source/sink telemetry feeding release-gate balance checks.

## Server Ownership
- All `admin.content.*` ops require server-resolved admin role; never leak hidden content to players.

## Smoke Tests (one assertion each)
- [ ] `admin.content.diff` returns changed fields for two versions (admin only).
- [ ] Audit row records explicit `action` for publish vs rollback.
- [ ] Quest publish + rollback restores prior quest content exactly.
- [ ] Concurrent publish on stale current version is rejected (no partial write).
- [ ] Safe projection leaks no hidden loot/seed/spawn field to a player.

## Done Criteria
- [ ] CMS "Remaining" items in cms-rework phases 08/11 closed.
- [ ] Balance telemetry visible in release gate.

## Verification
```bash
go test ./internal/game/contentdb/... ./internal/game/content/... ./internal/game/admin/... -count=1
go test ./... && cd client && npm --cache /tmp/gameproject-npm-cache run check
```
