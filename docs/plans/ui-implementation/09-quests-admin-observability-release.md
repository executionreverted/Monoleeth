# Phase 09: Quests, Admin, Observability, And Release Gates UI

## Status

- State: Completed
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

## Operation Contracts

| Operation | Client Payload | Server Contract |
| --- | --- | --- |
| `quest.board` | empty | player snapshot, eligibility, server-generated offers |
| `quest.accept` | `offer_id` | idempotent accept for eligible offer |
| `quest.progress` | empty | server-owned progress snapshot; client cannot advance |
| `quest.claim_reward` | `quest_id` | `quest_reward:<player_quest_id>` idempotency, wallet/inventory/progression ledger flow |
| `quest.reroll` | empty | server-calculated cost, wallet debit, board replacement |
| `admin.repair_craft_job` | `job_id` | admin session, action event, safe repair mutation or unavailable status |
| `admin.inspect_player` | optional `target_player_id` | admin session, allowlisted public/operational fields |
| `admin.economy_dashboard` | empty | admin session, aggregate data only |
| `observability.command_log` | empty | admin session, redacted command records |
| `observability.metrics` | empty | admin session, safe metrics snapshot |
| `observability.release_gate` | empty | admin session, release gate report schema |
| `observability.abuse_coverage` | empty | admin session, command coverage report |

## Events

```text
quest.board_generated
quest.accepted
quest.progressed
quest.completed
quest.reward_claimed
quest.board_rerolled
quest.abandoned
admin.action_completed
observability.metric_updated
release_gate.updated
```

If an implementation already emits compatibility aliases such as
`quest.board_updated`, document the mapping to the canonical event names above
and keep reducer tests for both until aliases are removed.

## Admin, Redaction, And Release Gate Contract

All admin and observability queries require an admin role resolved from the
server session. Non-admin requests must return a safe authorization error and no
partial dashboard payload.

Admin/observability responses are allowlist-based. They must omit or redact:
passwords, password hashes, session tokens/cookies, reset secrets, raw auth
headers, provider webhook payload secrets, private payment/provider metadata,
and hidden gameplay fields not needed for the admin task.

Release gate reports use this shape per gate:

```json
{
  "gate": "go_test",
  "status": "passed",
  "evidence": "go test ./...",
  "fresh_at": 182736123,
  "missing": []
}
```

Required gates: backend tests, client checks, browser smoke, E2E smoke, abuse
coverage, metrics availability, admin inspection, stable error codes, ledger
reason coverage, load/performance smoke, and `git diff --check`.

## UI Surfaces

Mockup areas covered:
- left navigation can open quest/operations overlays
- bottom log uses server events
- topbar mail/notification indicator can surface quest/admin notices
- admin-only panels are hidden for non-admin sessions

## Implementation Notes

- Browser smoke artifacts live under
  `output/screenshots/ui-implementation/09/`, including a seeded admin desktop
  run that renders the Ops release/abuse gate values.
- UI Patch 3 replaces the old single-focus quest card with a real Quest Board
  window: category sections for offers/active/claimable/completed quests,
  local selection, server objective/reward detail, and real
  `quest.accept`/`quest.claim_reward`/`quest.reroll` controls. Smoke captures
  the surface under `output/screenshots/ui-patch-3/quests-{viewport}.png`.
- Admin craft repair returns `unavailable` in the current runtime when the
  crafting repair service is not wired, while still preserving the admin guard
  and action event path.
- Release gate freshness is represented by `generated_at`; release evidence is
  summarized by per-module evidence counts for client-safe UI payload size.

## TODO

- [x] Add quest board/progress/reward/reroll handlers.
- [x] Add per-operation payload/error contracts for quest, admin, and
      observability operations.
- [x] Wire quest progress event consumers to browser snapshots/events.
- [x] Add quest board UI and active quest tracker.
- [x] Add quest reward claim flow with wallet/inventory/progression snapshots.
- [x] Add safe quest reroll cost preview/confirmation.
- [x] Add admin role guard middleware/helpers.
- [x] Add admin repair/inspection UI for implemented admin services.
- [x] Add observability query handlers for command logs/metrics/release gates.
- [x] Add admin guard to every observability and release query.
- [x] Add allowlisted redaction models and snapshot tests for admin responses.
- [x] Add release gate schema with passed/missing/evidence/freshness fields.
- [x] Add admin-only dashboards and release report views.
- [x] Add notification/log integration for quest/admin/observability events.

## Abuse And Safety Checklist

- [x] Client cannot set quest progress.
- [x] Quest reward claim is idempotent.
- [x] Quest reroll cost is server-calculated.
- [x] Rare reward cap remains server-side.
- [x] Admin commands require admin session.
- [x] Observability and release gate queries require admin session.
- [x] Admin responses do not leak secrets.
- [x] Observability dashboards redact sensitive auth/session data.
- [x] Quest event names match canonical spec or have tested alias mappings.

## Tests

- [x] Quest board returns offers valid for player snapshot.
- [x] Accept quest is idempotent/safe.
- [x] Server event progresses quest once.
- [x] Claim reward grants values once.
- [x] Reroll debits wallet and replaces board safely.
- [x] Non-admin cannot call admin endpoints.
- [x] Non-admin cannot call observability or release gate endpoints.
- [x] Admin repair action logs audit event.
- [x] Admin/observability snapshots omit passwords, hashes, tokens, cookies,
      reset secrets, raw auth headers, and sensitive provider data.
- [x] Release gate report renders passed/missing/evidence/freshness for required
      gates.
- [x] Browser quest panel updates from server state.
- [x] Browser release gate panel renders current report.

## Done Criteria

- Quest guidance works from browser.
- Admin/observability panels are real and role-gated.
- Release readiness is visible from the client/admin UI.
- Tests and browser smoke pass.
