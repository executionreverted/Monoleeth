# Phase 09: Quests, Admin, Observability, And Release Gates UI

## Status

- State: Planned
- Owner: Guidance, operations, and readiness UI
- Depends on: Phase 08
- Unlocks: full MVP playtest readiness

## Goal

Expose quest board/guided progression, admin repair tools, observability
summaries, balancing dashboards, and release gate reports through real
server-backed UI paths.

## Source Specs

Read before implementation:
- `docs/plans/modules/10-quest-board-generation.md`
- `docs/plans/modules/16-testing-observability-balancing.md`
- `internal/game/quests`
- `internal/game/admin`
- `internal/game/observability`

## Server Features To Expose

- quest board query
- quest accept
- quest progress snapshot
- quest claim reward
- quest reroll
- quest event log/notifications
- admin stuck craft repair
- admin market/auction/ledger inspection where implemented
- command log and metrics snapshots
- abuse coverage report
- release gate report
- economy balancing summaries

## Commands And Queries

```text
quest.board
quest.accept
quest.progress
quest.claim_reward
quest.reroll
admin.repair_craft_job
admin.inspect_player
admin.economy_dashboard
observability.command_log
observability.metrics
observability.release_gate
observability.abuse_coverage
```

## Events

```text
quest.board_updated
quest.accepted
quest.progressed
quest.completed
quest.reward_claimed
quest.rerolled
admin.action_completed
observability.metric_updated
release_gate.updated
```

## UI Surfaces

Mockup areas covered:
- left navigation can open quest/operations overlays
- bottom log uses server events
- topbar mail/notification indicator can surface quest/admin notices
- admin-only panels are hidden for non-admin sessions

## TODO

- [ ] Add quest board/progress/reward/reroll handlers.
- [ ] Wire quest progress event consumers to browser snapshots/events.
- [ ] Add quest board UI and active quest tracker.
- [ ] Add quest reward claim flow with wallet/inventory/progression snapshots.
- [ ] Add safe quest reroll cost preview/confirmation.
- [ ] Add admin role guard middleware/helpers.
- [ ] Add admin repair/inspection UI for implemented admin services.
- [ ] Add observability query handlers for command logs/metrics/release gates.
- [ ] Add admin-only dashboards and release report views.
- [ ] Add notification/log integration for quest/admin/observability events.

## Abuse And Safety Checklist

- [ ] Client cannot set quest progress.
- [ ] Quest reward claim is idempotent.
- [ ] Quest reroll cost is server-calculated.
- [ ] Rare reward cap remains server-side.
- [ ] Admin commands require admin session.
- [ ] Admin responses do not leak secrets.
- [ ] Observability dashboards redact sensitive auth/session data.

## Tests

- [ ] Quest board returns offers valid for player snapshot.
- [ ] Accept quest is idempotent/safe.
- [ ] Server event progresses quest once.
- [ ] Claim reward grants values once.
- [ ] Reroll debits wallet and replaces board safely.
- [ ] Non-admin cannot call admin endpoints.
- [ ] Admin repair action logs audit event.
- [ ] Browser quest panel updates from server state.
- [ ] Browser release gate panel renders current report.

## Done Criteria

- Quest guidance works from browser.
- Admin/observability panels are real and role-gated.
- Release readiness is visible from the client/admin UI.
- Tests and browser smoke pass.
