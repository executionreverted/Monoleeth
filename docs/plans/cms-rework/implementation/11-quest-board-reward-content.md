# Quest Board Reward Content Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Load quest board templates and reward tables from CMS snapshot.

**Architecture:** Quest definitions move to CMS. Accepted quest progress remains player state and keeps generated payload/source version.

**Tech Stack:** Go content validators, `internal/game/quests`, `contentdb` mapping into `GameplayContent`.

---

### Task 1: Add Quest Content DTOs

**Files:**
- Modify: `internal/game/content/snapshot.go`
- Create: `internal/game/content/quests.go`
- Test: `internal/game/content/quests_test.go`

**Steps:**
1. Add quest templates and reward tables to snapshot.
2. Validate objective schema, rewards, rank/role gates, board weights.
3. Cross-ref item, ship, NPC, recipe, production/building IDs.

### Task 2: Add Seed Compiler

**Files:**
- Create: `internal/game/contentseed/quests.go`
- Test: `internal/game/contentseed/quests_test.go`

**Steps:**
1. Convert `quests.MustMVPQuestCatalog()` data into CMS quest DTOs.
2. Preserve existing IDs in first migration.
3. Test seed quest refs resolve against seed item/NPC/recipe content.

### Task 3: Add DB Mapping

**Files:**
- Create: `internal/game/contentdb/map_quests.go`
- Test: `internal/game/contentdb/map_quests_test.go`

**Steps:**
1. Convert CMS quest rows into existing quest catalog structs inside the DB repository mapping.
2. Set source version to CMS content version.
3. Test old accepted quest source remains usable or publish blocks incompatible change.

**Status:** DB repository mapping landed for enabled quest templates. Mapped
templates force source version to published snapshot version and validate refs
against mapped item/ship/recipe/production/map content. Old accepted quest
compatibility remains a follow-up because accepted quest payload/source is
player state, not CMS content.

Reward table mapping now attaches one enabled CMS reward payload to each enabled
template and fails load if a template has zero or multiple enabled reward rows.
Weighted/multiple reward-table selection remains deferred.

### Task 4: Wire Runtime

**Files:**
- Modify: `internal/game/server/runtime.go`
- Test: `internal/game/server/server_quests_admin_observability_test.go`

**Steps:**
1. Runtime quest service receives assembled quest catalog in CMS mode.
2. Replace real-mode `quests.MustMVPQuestCatalog()` call.
3. Test DB quest offer appears on board.

**Status:** Runtime quest service now receives `contentBundle.Quests`.
`runtime.go` no longer calls `quests.MustMVPQuestCatalog()` in real runtime
composition. Static MVP quest catalog remains seed/dev/test fallback only and
now uses current content refs for NPC, recipe, and production/building
objectives.

### Remaining Reward Work

Generated board offers now use CMS reward payloads when present on the mapped
template, then copy that payload into accepted quest state. Claim still uses the
accepted/generated quest payload, not current draft/published rows. Admin
publish and accepted quest version tests still need to cover rollback/restart
policy before calling the whole quest reward CMS phase complete.

### Verify

```bash
go test ./internal/game/content ./internal/game/contentseed ./internal/game/contentdb ./internal/game/quests ./internal/game/server -run 'Quest|Content' -count=1
git diff --check
```

### Commit

```bash
git add internal/game/content internal/game/contentseed internal/game/contentdb internal/game/server internal/game/quests
git commit -m "game: move quest content to cms"
```
