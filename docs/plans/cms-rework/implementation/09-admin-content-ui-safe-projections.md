# Admin Content UI Safe Projections Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add admin CMS UI and ensure normal players receive safe published projections only.

**Architecture:** Client admin UI calls admin content ops. Player UI reads safe catalog projection; no hidden loot/spawn/admin fields in normal payloads.

**Tech Stack:** Existing client state/UI/net code, realtime commands, Go safe projection helpers.

---

### Task 1: Add Safe Projection Tests

**Files:**
- Modify: `internal/game/content/projection.go`
- Test: `internal/game/content/projection_test.go`

**Steps:**
1. Add player-safe projection from snapshot.
2. Include display metadata and visible stats.
3. Omit loot chances, spawn timers, pool caps, audit notes.
4. Implement projection as explicit allowlist DTOs.
5. Test forbidden field names and sentinel hidden values absent from marshaled JSON.

### Task 2: Add Server Query Payload

**Files:**
- Modify: `internal/game/server/economy_handlers.go` or catalog handler file
- Test: `internal/game/server/server_content_projection_test.go`

**Steps:**
1. Return safe catalog projection to authenticated players.
2. Admin-only payloads stay behind admin ops.
3. Add operation-aware admin parser/handler path so admin CMS payload can contain content stat fields while player stream remains strict.
4. Test non-admin cannot fetch admin content.

### Task 3: Add Client State Slice

**Files:**
- Modify/Create: `client/src/state/content.ts`
- Modify: `client/src/state/reducer.ts`
- Test: `client/src/state/reducer-content.test.ts`

**Steps:**
1. Store content version, categories, items/modules/shop metadata.
2. Store admin versions/drafts only under admin state.
3. Reducer handles loading/error/success.

### Task 4: Add Admin CMS Views

**Files:**
- Create/Modify under `client/src/ui/`
- Use current UI panel/window conventions.

**Steps:**
1. Add MVP Content Versions panel.
2. Add Equipment/Modules table.
3. Add LC1 edit form with validation errors.
4. Add Publish/Rollback/Diff controls.
5. Defer Items/Ships/Shop/NPC/Loot/Craft/Production full editors to later UI slices.

### Task 5: Browser Smoke

**Files:**
- Add/modify Playwright or existing smoke harness files.

**Steps:**
1. Admin login opens CMS.
2. Non-admin cannot open CMS.
3. Player shop/inventory uses display metadata.
4. Admin CMS payload accepted only for admin op; player payload guard still rejects trusted fields.
5. Screenshot admin CMS desktop/tablet/mobile if UI phase requires.

### Verify

```bash
go test ./internal/game/content ./internal/game/server -run 'Projection|Content|Admin' -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run check
git diff --check
```

### Commit

```bash
git add internal/game/content internal/game/server client
git commit -m "client: add admin content cms"
```
