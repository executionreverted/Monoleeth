# NPC Enemy Pools Loot Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Load NPC templates, enemy pools, and loot tables from CMS snapshot.

**Architecture:** CMS owns definitions. World/map projection remains server-safe. Loot selector consumes assembled loot tables; client never receives chances/pool internals.

**Tech Stack:** Go content validators, `internal/game/world/maps`, `internal/game/loot`, server NPC loot selector.

---

### Task 1: Validate Loot Tables

**Files:**
- Modify: `internal/game/content/loot.go`
- Test: `internal/game/content/loot_test.go`

**Steps:**
1. Validate table ID, row item ID, min/max qty, chance `0..1`.
2. Use `roll_mode=independent_chance` for MVP.
3. Reject `weight`/free-form conditions until typed roll modes exist.
4. Cross-ref item IDs against content item set.

### Task 2: Validate NPC Templates

**Files:**
- Modify: `internal/game/content/npc.go`
- Test: `internal/game/content/npc_test.go`

**Steps:**
1. Validate HP/range/cooldown/accuracy/speed/radar signature.
2. Validate level band.
3. Validate aggro/leash profile bounds.

### Task 3: Validate Enemy Pools

**Files:**
- Modify: `internal/game/content/npc.go`
- Test: `internal/game/content/enemy_pool_test.go`

**Steps:**
1. Validate pool caps.
2. Validate spawn timings.
3. Validate references to templates/drop/aggro/leash/spawn areas.
4. Add PvP/starter loot guard as allowlistable rule.
5. Validate spawn area radius, map bounds, safe-zone exclusion, portal exclusion.
6. Validate event spawn pool/drop refs and caps.

### Task 4: Assemble Loot And NPC Maps

**Files:**
- Modify: `internal/game/contentassembly/loot.go`
- Modify: `internal/game/contentassembly/maps_npc.go`
- Test: `internal/game/contentassembly/npc_loot_test.go`

**Steps:**
1. Convert CMS loot rows to `loot.LootTable`.
2. Convert CMS NPC definitions to `world/maps` structs.
3. Preserve `json:"-"` server-only behavior.
4. Test missing loot table/spawn area/drop profile/aggro/leash ref rejects assembly.

### Task 5: Server Integration

**Files:**
- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/npc_loot_selector.go`
- Test: `internal/game/server/npc_loot_selector_test.go`

**Steps:**
1. Ensure runtime loot tables come from assembled catalogs.
2. Test changed DB loot table affects selector.
3. Test safe NPC/map payload omits loot chance, roll internals, spawn timers, pool caps.

### Verify

```bash
go test ./internal/game/content ./internal/game/contentassembly ./internal/game/server -run 'NPC|Enemy|Loot|Content' -count=1
git diff --check
```

### Commit

```bash
git add internal/game/content internal/game/contentassembly internal/game/server
git commit -m "game: move npc and loot content to cms"
```
