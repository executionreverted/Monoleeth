# Admin Publish Rollback Audit API Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add admin API for draft edits, validation, publish, rollback, diff, and audit.

**Architecture:** Admin handlers call CMS service. CMS service owns DB transaction, validation, version creation, audit rows. Runtime gameplay still reloads on restart.

**Tech Stack:** Existing realtime command handlers, `internal/game/admin`, content/contentdb services.

---

### Task 1: Add Content Admin Service

**Files:**
- Create: `internal/game/admin/content_service.go`
- Test: `internal/game/admin/content_service_test.go`

**Steps:**
1. Define service deps: content store, clock.
2. Add methods:
   - `ListVersions`
   - `ValidateDraft`
   - `PublishDraft`
   - `Rollback`
   - `Diff`
   - `AuditLog`
3. Unit-test missing deps and invalid draft.

**Status:** `ListVersions`, `ListDraftRows`, `GetDraftRow`,
`UpdateDraftRow`, and `ValidateDraft` are implemented. Service normalizes
pagination, stamps generated metadata from the server clock, validates draft
row JSON before write, assembles all draft tables into a snapshot, and runs
that snapshot through the same `contentdb` runtime mapper validator used for
published content. Publish, rollback, diff, and audit-log methods remain open.

### Task 2: Add Draft Store Methods

**Files:**
- Modify: `internal/game/contentdb/store.go`
- Modify: `internal/game/contentdb/store_postgres.go`
- Test: `internal/game/contentdb/store_test.go`

**Steps:**
1. Add typed draft row upsert/list/get methods.
2. Add publish transaction method with DB idempotency key.
3. Add rollback transaction method with DB idempotency key.
4. Add audit query method with pagination and scrubbed payloads.

**Status:** `ListContentVersions`, `LoadDraftRows`, and existing
`UpsertDraftRow`/`UpsertDraftRows` now back the admin read/update API.
`contentdb.ValidateSnapshot` exposes the runtime published-content mapper for
draft validation. Publish, rollback, and audit query remain open.

### Task 3: Add Realtime Ops

**Files:**
- Modify: `internal/game/contracts/realtime` files if op constants exist there
- Modify: `internal/game/server/handlers.go`
- Create: `internal/game/server/content_admin_handlers.go`
- Test: `internal/game/server/server_content_admin_test.go`

**Steps:**
1. Add operations:
   - `admin.content.list`
   - `admin.content.get`
   - `admin.content.update_draft`
   - `admin.content.validate_draft`
   - `admin.content.publish`
   - `admin.content.versions`
   - `admin.content.rollback`
   - `admin.content.diff`
   - `admin.content.audit_log`
2. Use `requireAdmin`.
3. Reject non-admin.
4. Never trust actor/session/player/server fields from payload.
5. Add explicit admin content DTO gate so stat fields like damage/rank/cooldown are accepted only for `admin.content.*`.

**Status:** `admin.content.versions`, `admin.content.list`,
`admin.content.get`, `admin.content.update_draft`, and
`admin.content.validate_draft` are registered and admin-gated. Draft update
uses a CMS-specific payload gate so nested stat keys such as `damage`,
`cooldown_ms`, and map/content fields can live inside `data_json`, while
top-level actor/session/admin spoof fields still fail before mutation.
Publish/diff/audit ops remain open.

### Task 4: Idempotency And Rate Posture

**Files:**
- Modify: `internal/game/server/content_admin_handlers.go`
- Test: `internal/game/server/server_content_admin_test.go`

**Steps:**
1. Do not rely on realtime request cache for publish/rollback idempotency.
2. Use DB unique keys: `content_publish:<draft_revision_or_snapshot_hash>` and `content_rollback:<target_version>:<request_id>`.
3. Duplicate publish/rollback after reconnect/cache clear returns same version.
4. Failed publish or rate-limit rejection must not partially mutate.
5. Concurrent publish/rollback conflict test required.

### Verify

```bash
go test ./internal/game/admin ./internal/game/contentdb ./internal/game/server -run 'Content|Admin|Publish|Rollback|Audit' -count=1
git diff --check
```

### Commit

```bash
git add internal/game/admin internal/game/contentdb internal/game/server internal/game/contracts
git commit -m "admin: add content publish workflow"
```
