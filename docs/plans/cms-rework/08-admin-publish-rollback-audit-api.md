# Phase 08 - Admin Publish Rollback And Audit API

## Goal

Add admin server API for content draft edits, validation, publish, rollback, and
audit log.

No rich UI required here.

## Admin Operations

```text
admin.content.list
admin.content.get
admin.content.update_draft
admin.content.validate_draft
admin.content.publish
admin.content.versions
admin.content.rollback
admin.content.diff
admin.content.audit_log
```

Names may adapt to existing realtime operation style.

## Auth

All ops require server-resolved admin role.

Client must not send trusted:

```text
account_id
admin role
published_by
audit actor
server time
player_id
session_id
```

Runtime resolves from authenticated session.

Admin content edit DTOs may contain gameplay stat field names such as damage,
rank, cooldown, or range. Those fields must pass only through explicit
`admin.content.*` DTO gates after `requireAdmin`. Do not broadly weaken normal
anti-spoof payload guards.

## Publish Flow

```text
BEGIN
lock content publish key
load draft rows
build snapshot
validate snapshot
insert content_versions(status=published, is_current=true)
set previous current version is_current=false
write audit log
COMMIT
broadcast admin-only content.version_published event
```

MVP gameplay uses restart-based load. Publish does not hot-swap runtime yet.

Publish idempotency is DB-backed, not transport-cache-backed:

```text
content_publish:<draft_revision_or_snapshot_hash>
```

Unique key returns same version on retry after reconnect/restart/cache eviction.

## Rollback Flow

Rollback creates new published version from older snapshot.

Do not mutate old version rows.

```text
old published snapshot -> new content_versions row
rolled_back_from = old version id
audit action = rollback
```

Rollback idempotency key:

```text
content_rollback:<target_version>:<request_id>
```

## Audit Log

`content_audit_log` records:

```text
id
actor_account_id
action
content_type
content_id
version_id nullable
old_value_json
new_value_json
reason/notes
created_at
```

Never log passwords, session tokens, secrets, or raw cookies.
Audit payloads must be scrubbed and bounded:

- no procedural seeds
- no hidden liveops internals not needed for diff
- no admin-only private notes in player-visible responses
- max payload size and pagination required

## Validation Response

Validation returns structured errors:

```text
path: "modules[item.laser.lc1].cooldowns[0].duration_ms"
code: "invalid_positive_duration"
message: safe admin-facing text
```

## Rate Limit

Admin content writes need abuse posture:

- validate/list can be moderate
- update/publish/rollback stricter
- failed publish must not partially mutate content
- rate-limit rejection must produce zero draft/version/audit mutation

## Tests

- non-admin forbidden
- publish invalid draft rejected
- publish writes immutable version and audit rows
- rollback creates new published version
- duplicate publish/rollback after reconnect/cache clear returns same version
- concurrent publish conflict handling
- admin DTO accepts content stat fields but rejects actor/session/player/server
  fields
- audit scrubber removes secrets/seeds/hidden fields

## Done

- admin can mutate drafts through server API
- publish/rollback/audit exist
- runtime still restart-loads current published version

## Implemented Slice

- Added first `admin.content.*` operation: `admin.content.versions`.
- Operation is admin-only via server-resolved session role.
- Runtime can keep a DB-backed content admin version store open separately from
  boot-time published snapshot loading.
- Response returns version metadata only; no snapshot JSON, loot rows, spawn
  internals, procedural seeds, or audit payloads.

Remaining:

- draft list/get/update
- draft validation response
- publish transaction and DB idempotency
- rollback transaction and DB idempotency
- diff and audit-log query with scrubber/pagination
- admin-only publish event fanout
