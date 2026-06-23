# Content Snapshot Schema Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add CMS snapshot domain model and Postgres schema tables.

**Architecture:** `internal/game/content` validates snapshot structs. `internal/game/contentdb` persists typed rows and immutable published versions.

**Tech Stack:** Go structs, JSON validation, Postgres `jsonb`, SQL migrations.

---

### Task 1: Add Snapshot Types

**Files:**
- Create: `internal/game/content/snapshot.go`
- Create: `internal/game/content/ids.go`
- Test: `internal/game/content/snapshot_test.go`

**Steps:**
1. Define `Snapshot` with version and slices for items/modules/ships/shop/NPC/pools/loot/recipes/production.
2. Include spawn areas, NPC drop profiles, aggro profiles, leash profiles, and event spawns.
3. Define ID aliases and `ValidateID`.
4. Write tests for empty version, duplicate IDs, valid minimal snapshot.
4. Run:
   ```bash
   go test ./internal/game/content -run Snapshot -count=1
   ```

### Task 2: Add Definition DTOs

**Files:**
- Create: `internal/game/content/items.go`
- Create: `internal/game/content/modules.go`
- Create: `internal/game/content/ships.go`
- Create: `internal/game/content/shop.go`
- Create: `internal/game/content/npc.go`
- Create: `internal/game/content/loot.go`
- Create: `internal/game/content/crafting.go`
- Create: `internal/game/content/production.go`
- Create: `internal/game/content/quests.go`

**Steps:**
1. Keep DTOs flat enough for JSON.
2. Use nested structs for repeated lists: stats, cooldowns, inputs, rows.
3. Add `ValidateBasic` per type only for local constraints.
4. No cross-domain validation yet.

### Task 3: Add Validator

**Files:**
- Create: `internal/game/content/validation.go`
- Test: `internal/game/content/validation_test.go`

**Steps:**
1. Validate unique IDs per type.
2. Validate positive durations/amounts.
3. Validate chance `0..1`.
4. Validate finite non-negative stats.
5. Return path-coded errors.

### Task 4: Add DB Schema Migration

**Files:**
- Create: `internal/game/contentdb/migrations/0002_content_schema.sql`
- Modify: `internal/game/contentdb/migrations.go`

**Steps:**
1. Create `content_versions`.
2. Create `content_audit_log`.
3. Create typed content tables listed in design.
4. Add `is_current boolean not null default false`, `idempotency_key text unique`.
5. Add partial unique index so only one current published version exists.
6. Use `jsonb not null`, unique `(content_id)`, indexes on status/version/content IDs.
7. Add check constraints for non-empty IDs and status values.

### Task 5: Add Store Interfaces

**Files:**
- Create: `internal/game/contentdb/store.go`
- Create: `internal/game/contentdb/store_postgres.go`
- Test: `internal/game/contentdb/store_test.go`

**Steps:**
1. Define methods:
   - `LoadCurrentPublishedSnapshot`
   - `InsertPublishedSnapshot`
   - `HasAnyContent`
   - `InsertAudit`
2. Runtime must use only `LoadCurrentPublishedSnapshot` backed by `is_current=true`.
3. Postgres implementation uses `database/sql`.
4. Tests use small fake store unless live DB env set.

### Verify

```bash
go test ./internal/game/content ./internal/game/contentdb -count=1
git diff --check
```

### Commit

```bash
git add internal/game/content internal/game/contentdb
git commit -m "game: add content snapshot schema"
```
