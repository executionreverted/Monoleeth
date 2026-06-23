# Items Modules Ships Shop Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete DB-backed item/module/ship/shop definitions and prove LC1-style stat edits affect runtime.

**Architecture:** Content DTOs stay CMS-facing. Assembly produces existing economy/module/ship/shop catalogs. Tests cover behavior, not old exact catalog counts.

**Tech Stack:** Go content validators, existing economy/modules/ships/catalog packages.

---

### Task 1: Tighten Item Validators

**Files:**
- Modify: `internal/game/content/items.go`
- Test: `internal/game/content/items_test.go`

**Steps:**
1. Validate item type, rarity, stack size, weight, flags, bind rules.
2. Reject raw display name equal to ID unless allowlisted.
3. Add old-version item lookup test for cargo weight/display.
4. Run item tests.

### Task 2: Tighten Module Validators

**Files:**
- Modify: `internal/game/content/modules.go`
- Test: `internal/game/content/modules_test.go`

**Steps:**
1. Validate stat kind/name enum maps to `modules` package.
2. Validate cooldown key/duration.
3. Validate energy values.
4. Validate rank/tier positive.
5. Reject duplicate stat/cooldown keys unless explicitly stackable.

### Task 3: Tighten Ship Validators

**Files:**
- Modify: `internal/game/content/ships.go`
- Test: `internal/game/content/ships_test.go`

**Steps:**
1. Validate ship ID/display/rank/tier/cargo/base stats.
2. Validate slot layout non-negative and at least starter viable.
3. Include existing gameplay fields: role tag, craft recipe, premium price, auction buy-now, repair multiplier, passive bonus, acquisition blocker.
4. Reject non-starter ship with no acquisition path unless blocker set.

### Task 4: Tighten Shop Validators

**Files:**
- Modify: `internal/game/content/shop.go`
- Test: `internal/game/content/shop_test.go`

**Steps:**
1. Validate product type, price policy, stock policy, availability.
2. Add cross-ref validator for item/module/ship/premium grant targets.
3. Test bad grant target rejected.

### Task 5: Runtime Proof Test

**Files:**
- Test: `internal/game/server/server_content_runtime_test.go`

**Steps:**
1. Build published snapshot where `laser_alpha_t1` damage differs from seed.
2. Start runtime with fake/DB content store seam.
3. Assert module catalog returns changed stat.
4. Assert documented equipped-module version behavior after stat/slot change.
5. Assert shop payload version matches content version.

### Verify

```bash
go test ./internal/game/content ./internal/game/contentassembly ./internal/game/server -run 'Item|Module|Ship|Shop|Content' -count=1
git diff --check
```

### Commit

```bash
git add internal/game/content internal/game/contentassembly internal/game/server
git commit -m "game: move core catalog content to cms"
```
