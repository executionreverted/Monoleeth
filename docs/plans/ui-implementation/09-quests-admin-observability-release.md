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

## Operation Contracts

| Operation | Client Payload | Server Contract |
| --- | --- | --- |
| `quest.board` | empty or board filter | player snapshot, eligibility, server-generated offers |
| `quest.accept` | quest offer id | idempotent accept for eligible offer |
| `quest.progress` | empty or quest id | server-owned progress snapshot; client cannot advance |
| `quest.claim_reward` | player quest id | `quest_reward:<player_quest_id>` idempotency, wallet/inventory/progression ledger flow |
| `quest.reroll` | board id or empty | server-calculated cost, wallet debit, board replacement |
| `admin.repair_craft_job` | job id/action | admin session, audit log, safe repair mutation |
| `admin.inspect_player` | player/account lookup | admin session, allowlisted public/operational fields |
| `admin.economy_dashboard` | filters | admin session, aggregate data only |
| `observability.command_log` | filters/page | admin session, redacted command records |
| `observability.metrics` | filters | admin session, safe metrics snapshot |
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

## TODO

- [ ] Add quest board/progress/reward/reroll handlers.
- [ ] Add per-operation payload/error contracts for quest, admin, and
      observability operations.
- [ ] Wire quest progress event consumers to browser snapshots/events.
- [ ] Add quest board UI and active quest tracker.
- [ ] Add quest reward claim flow with wallet/inventory/progression snapshots.
- [ ] Add safe quest reroll cost preview/confirmation.
- [ ] Add admin role guard middleware/helpers.
- [ ] Add admin repair/inspection UI for implemented admin services.
- [ ] Add observability query handlers for command logs/metrics/release gates.
- [ ] Add admin guard to every observability and release query.
- [ ] Add allowlisted redaction models and snapshot tests for admin responses.
- [ ] Add release gate schema with passed/missing/evidence/freshness fields.
- [ ] Add admin-only dashboards and release report views.
- [ ] Add notification/log integration for quest/admin/observability events.

## Abuse And Safety Checklist

- [ ] Client cannot set quest progress.
- [ ] Quest reward claim is idempotent.
- [ ] Quest reroll cost is server-calculated.
- [ ] Rare reward cap remains server-side.
- [ ] Admin commands require admin session.
- [ ] Observability and release gate queries require admin session.
- [ ] Admin responses do not leak secrets.
- [ ] Observability dashboards redact sensitive auth/session data.
- [ ] Quest event names match canonical spec or have tested alias mappings.

## Tests

- [ ] Quest board returns offers valid for player snapshot.
- [ ] Accept quest is idempotent/safe.
- [ ] Server event progresses quest once.
- [ ] Claim reward grants values once.
- [ ] Reroll debits wallet and replaces board safely.
- [ ] Non-admin cannot call admin endpoints.
- [ ] Non-admin cannot call observability or release gate endpoints.
- [ ] Admin repair action logs audit event.
- [ ] Admin/observability snapshots omit passwords, hashes, tokens, cookies,
      reset secrets, raw auth headers, and sensitive provider data.
- [ ] Release gate report renders passed/missing/evidence/freshness for required
      gates.
- [ ] Browser quest panel updates from server state.
- [ ] Browser release gate panel renders current report.

## Done Criteria

- Quest guidance works from browser.
- Admin/observability panels are real and role-gated.
- Release readiness is visible from the client/admin UI.
- Tests and browser smoke pass.
