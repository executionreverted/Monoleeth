# Rollout Versioning Balancing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add rollout guards, version evidence, balancing metadata, and release checks.

**Architecture:** Restart-based content version loading remains MVP. Metrics/logs carry safe content version. Release gate fails invalid CMS state or projection leaks.

**Tech Stack:** Existing observability/release gate code, content validators, admin audit.

---

### Task 1: Add Version Evidence

**Files:**
- Modify: `internal/game/server/runtime.go`
- Modify: relevant snapshot payload files
- Test: `internal/game/server/server_content_version_test.go`

**Steps:**
1. Runtime stores current content version.
2. Safe player/admin payloads include content version where useful.
3. Add durable state version matrix checks for inventory, loadout, craft, production, loot, shop, NPC.
4. Test version present and no hidden fields added.

### Task 2: Add Balancing Metadata Requirements

**Files:**
- Modify: `internal/game/admin/content_service.go`
- Test: `internal/game/admin/content_service_test.go`

**Steps:**
1. Require publish notes and optional `balance_tag`.
2. Validate tag format.
3. Audit log stores notes/tag.

**Status:** Publish and rollback metadata is hardened in the admin content
service. Notes are required before validation or storage work starts, optional
`balance_tag` values must be lowercase letters, digits, `_`, or `-` and fit a
64-character limit, and publish/rollback audit rows preserve the normalized
notes/tag metadata.

### Task 3: Add Release Gate Checks

**Files:**
- Modify: existing observability/release gate files under `internal/game/observability` and server handler tests.
- Test: matching release gate test file.

**Steps:**
1. Check current published content exists.
2. Check snapshot validates.
3. Check safe projection leak test passes.
4. Report evidence path and version.

### Task 4: Add Rollback Proof Tests

**Files:**
- Test: `internal/game/server/server_content_rollback_runtime_test.go`

**Steps:**
1. Publish v1 LC1 damage.
2. Publish v2 changed damage.
3. Rollback to v1 as new published v3.
4. Restart/runtime load sees v3 values equal v1.
5. Add old craft job/building version proof or publish-block proof.

### Task 5: Docs Update

**Files:**
- Modify: `docs/running-local-game.md` if exists
- Modify: `docs/test-server-operations.md`
- Modify: `docs/todo.md` only for real remaining gaps

**Steps:**
1. Document Docker Postgres startup.
2. Document seed/publish/restart flow.
3. Document rollback command/API.
4. Document no hot reload yet.

### Verify

```bash
go test ./internal/game/admin ./internal/game/content ./internal/game/server ./internal/game/observability -run 'Content|Release|Rollback|Version' -count=1
go test ./...
git diff --check
```

### Commit

```bash
git add internal/game docs
git commit -m "game: harden content cms rollout"
```
