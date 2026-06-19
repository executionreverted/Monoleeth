# TODO Cleanup Symphony Wave Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close the currently actionable `docs/todo.md` items with conflict-safe Symphony waves, and separate durable/outbox-dependent work from small implementation tasks.

**Architecture:** The main Codex session acts as Symphony project manager: create 2-5 local tasks per wave, review every worker diff, apply one patch at a time, run narrow tests, then run full repo gates. Workers must follow `docs/symphony-worker-rules.md`, must not read `AGENTS.md` or `docs/symphony-operating-model.md`, and must not commit.

**Tech Stack:** Go domain services under `internal/game`, authenticated realtime server under `internal/game/server` and `internal/game/realtime`, browser client under `client`, local Symphony API at `http://127.0.0.1:4000`.

---

## Research Intake

Read-only Symphony research tasks completed on 2026-06-19:

- `TASK-0170`: Phase 06 crafting/death/repair.
- `TASK-0171`: Phase 08/09 discovery/production/routes.
- `TASK-0172`: Phase 07/10/12 quests/market/premium/observability.
- `TASK-0173`: browser/gateway contract gaps.

Important conclusion: many open todos are real production-hardening work but not small patch work. Anything requiring durable repositories, row locks, DB transactions, or a persistent outbox must remain open until a storage/outbox boundary exists.

## Symphony Operating Rules

Before each wave:

1. Verify `git status --short` is clean except intended manager edits.
2. Create local Symphony tasks with `POST /api/v1/tasks`.
3. Start tasks with `POST /api/v1/tasks/{id}/run` because auto-run may be disabled.
4. Wait with `/api/v1/tasks/{id}/wait?timeout_ms=3600000`.
5. Fetch `/api/v1/tasks/{id}/workspace-diff`.
6. Review and apply one diff at a time.
7. Run narrow tests after each applied task.
8. Run `go test ./...` and `git diff --check` before every commit.
9. For client tasks, also run `cd client && npm --cache /tmp/gameproject-npm-cache run check`.

Every worker prompt must include:

```text
Follow ONLY this task prompt and docs/symphony-worker-rules.md.
Do NOT read AGENTS.md.
Do NOT read docs/symphony-operating-model.md.
Do NOT spawn subagents.
Do NOT create, edit, dispatch, or manage Symphony tasks.
Do NOT commit.
Keep changes scoped to the files and behavior named in this task.
```

## Wave 1: Low-Conflict Domain Fixes

Run these in parallel. Their write sets are mostly disjoint.

### Task 1: Indexed Wallet Ledger Reference Lookup

**Files:**
- Modify: `internal/game/economy/wallet_service.go`
- Modify: `internal/game/economy/wallet_service_test.go`
- Modify: `internal/game/death/repair.go`
- Modify: `internal/game/death/repair_service_test.go`

**Step 1: Write the wallet lookup test**

Add a test proving `WalletService` can find a currency ledger entry by player, currency, action, reason, and reference key without callers scanning `CurrencyLedgerEntries()`.

**Step 2: Run the failing test**

Run:

```bash
go test ./internal/game/economy -run 'Test.*Ledger.*Reference' -count=1
```

Expected: fail until the lookup API exists.

**Step 3: Implement the lookup**

Add a small exported lookup method or interface-friendly helper on `WalletService`. Keep returned entries clone-safe. Preserve existing duplicate reference behavior.

**Step 4: Use the lookup in repair**

Replace `RepairService.previouslyRefundedRepair` full ledger scan with a narrow wallet interface that uses the new lookup when available. Keep the old reader fallback only if needed for existing fakes.

**Step 5: Verify**

Run:

```bash
go test ./internal/game/economy ./internal/game/death -run 'Test.*Ledger.*Reference|TestRepairServiceRestoreFailureAfterDebitRefundsAndCachesFailure' -count=1
git diff --check
```

**Step 6: Commit after manager review**

```bash
git add internal/game/economy/wallet_service.go internal/game/economy/wallet_service_test.go internal/game/death/repair.go internal/game/death/repair_service_test.go
git commit -m "game: index wallet repair refund lookups"
```

### Task 2: Discovery Claim Retry Repair

**Files:**
- Modify: `internal/game/discovery/claim.go`
- Modify: `internal/game/discovery/claim_test.go`

**Step 1: Write the retry test**

Add a test where planet ownership was recorded for the claimant, production initialization failed, and the same claim reference is retried. The retry must repair missing production/local claim result state exactly once.

**Step 2: Run the failing test**

Run:

```bash
go test ./internal/game/discovery -run 'ClaimPlanetProductionInitializerError|ClaimPlanetDuplicate' -count=1
```

Expected: fail on missing repair or duplicate event behavior.

**Step 3: Implement idempotent repair**

When `ClaimPlanet` sees `planet.OwnerPlayerID == input.PlayerID`, initialize production if missing, mark listed intel stale once, and cache the claim result under the same reference. Avoid duplicate `planet.claimed` events and duplicate stale-listing markers.

**Step 4: Verify**

Run:

```bash
go test ./internal/game/discovery -run 'ClaimPlanetProductionInitializerError|ClaimPlanetDuplicate' -count=1
git diff --check
```

**Step 5: Commit after manager review**

```bash
git add internal/game/discovery/claim.go internal/game/discovery/claim_test.go
git commit -m "game: repair retried planet claim initialization"
```

### Task 3: Owner-Checked Route Operation Wrappers

**Files:**
- Modify: `internal/game/production/route_service.go`
- Modify: `internal/game/production/route_controls.go`
- Modify: `internal/game/production/route_test.go`

**Step 1: Write owner rejection tests**

Add tests proving enable, disable, update, and settle wrapper methods reject wrong-owner route access with safe errors and no mutation.

**Step 2: Run the failing tests**

Run:

```bash
go test ./internal/game/production -run 'Route.*Owner|SettleRoute|EnableRoute|DisableRoute' -count=1
```

**Step 3: Implement wrapper APIs**

Add owner/access-checked methods around existing internal route operations. Do not expose realtime handlers yet. Keep existing internal methods stable for current tests.

**Step 4: Verify**

Run:

```bash
go test ./internal/game/production -run 'Route.*Owner|SettleRoute|EnableRoute|DisableRoute' -count=1
git diff --check
```

**Step 5: Commit after manager review**

```bash
git add internal/game/production/route_service.go internal/game/production/route_controls.go internal/game/production/route_test.go
git commit -m "game: add owner checked route wrappers"
```

### Task 4: Browser Fake-Count Guard Rails

**Files:**
- Modify: `client/src/ui/hud.ts`
- Modify: `client/tests/browser-smoke.mjs`
- Modify: `client/src/state/reducer.test.ts` if needed

**Step 1: Add guard assertions**

Assert default unauthenticated, authenticated, logout, and reconnect states do not show fake unread mail, friend, party, or menu notification counts.

**Step 2: Run the failing or confirming check**

Run:

```bash
cd client && npm --cache /tmp/gameproject-npm-cache run test -- --run
cd client && npm --cache /tmp/gameproject-npm-cache run smoke
```

**Step 3: Adjust UI only if the guard finds fake counts**

If visible fake counts exist, hide or lock those affordances until server-backed contracts exist. Do not add local mock counts.

**Step 4: Verify**

Run:

```bash
cd client && npm --cache /tmp/gameproject-npm-cache run check
git diff --check
```

**Step 5: Commit after manager review**

```bash
git add client/src/ui/hud.ts client/tests/browser-smoke.mjs client/src/state/reducer.test.ts
git commit -m "client: guard against fake social notification counts"
```

## Wave 2: Dependent Service Hardening

Run after Wave 1 is reviewed and committed. Avoid running Task 5 and Task 6 in parallel if both touch the same repair tests.

### Task 5: Narrow RepairService Coordination Scope

**Depends on:** Task 1.

**Files:**
- Modify: `internal/game/death/repair.go`
- Modify: `internal/game/death/repair_service_test.go`

**Step 1: Write concurrency tests**

Add tests proving two different players can repair independently while duplicate same-reference attempts still coordinate and return consistent cached or compensated results.

**Step 2: Run the failing test**

Run:

```bash
go test ./internal/game/death -run 'TestRepairService.*Concurrent|TestRepairService' -count=1
```

**Step 3: Implement per-player/per-reference coordination**

Replace broad global repair lock coverage with a per-key in-flight record or smaller critical sections. Keep wallet and ship mutations idempotent under the repair reference.

**Step 4: Verify**

Run:

```bash
go test ./internal/game/death -run TestRepairService -count=1
go test ./internal/game/death -race -run TestRepairService -count=1
git diff --check
```

**Step 5: Commit after manager review**

```bash
git add internal/game/death/repair.go internal/game/death/repair_service_test.go
git commit -m "game: narrow repair service coordination"
```

### Task 6: Idempotent Scanner Capacitor Spend

**Files:**
- Modify: `internal/game/server/scanner_providers.go`
- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/server_test.go`
- Modify: `internal/game/discovery/scanner_types.go` only if the provider contract must change

**Step 1: Write server tests**

Add tests proving `scan.pulse` deducts starter ship capacitor once, rejects insufficient capacitor before discovery mutation, and duplicate request IDs do not double-spend.

**Step 2: Run the failing test**

Run:

```bash
go test ./internal/game/server -run 'Scan|Scanner' -count=1
```

**Step 3: Implement spend boundary**

Extend the runtime scanner energy provider from read-only check to idempotent reserve/spend, or add a separate runtime-side spend cache keyed by request/pulse reference. Keep slow-scan world leases out of scope.

**Step 4: Verify**

Run:

```bash
go test ./internal/game/server -run 'Scan|Scanner' -count=1
go test ./internal/game/discovery -run Scanner -count=1
git diff --check
```

**Step 5: Commit after manager review**

```bash
git add internal/game/server/scanner_providers.go internal/game/server/runtime.go internal/game/server/server_test.go internal/game/discovery/scanner_types.go
git commit -m "game: spend scanner capacitor once"
```

### Task 7: Quest Store Index And Compaction

**Files:**
- Modify: `internal/game/quests/store.go`
- Modify: `internal/game/quests/service.go`
- Modify: `internal/game/quests/service_test.go`
- Modify: `internal/game/quests/reroll_test.go`
- Modify: `internal/game/quests/consumers_test.go`

**Step 1: Write store behavior tests**

Add tests for per-player offer lookup without whole-store scan, expiry/compaction of old unaccepted offers, duplicate progress-event no-op retention, and duplicate claim/reroll result retention boundaries.

**Step 2: Run the failing tests**

Run:

```bash
go test ./internal/game/quests -run 'TestBoardOffersExpiresOldUnacceptedOffers|TestRerollBoardDuplicateReferenceDoesNotDoubleCharge|TestConsumeDuplicateServerEventDoesNotProgressTwice' -count=1
```

**Step 3: Implement indexes and compaction**

Add per-player offer index and explicit compaction helpers. Keep accepted quests and idempotency results needed for duplicate safety. Do not pretend this is durable uniqueness.

**Step 4: Verify**

Run:

```bash
go test ./internal/game/quests -count=1
git diff --check
```

**Step 5: Commit after manager review**

```bash
git add internal/game/quests/store.go internal/game/quests/service.go internal/game/quests/service_test.go internal/game/quests/reroll_test.go internal/game/quests/consumers_test.go
git commit -m "game: compact quest in-memory indexes"
```

### Task 8: Runtime Observability Metrics Wiring

**Files:**
- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/economy_handlers.go`
- Modify: `internal/game/server/quest_admin_observability_handlers.go`
- Modify: `internal/game/server/server_test.go`

**Step 1: Write metric assertions**

Add tests proving authenticated market sale, auction bid or buy-now, premium claim/purchase, and quest reward paths record stable metrics through `MetricRecorder`.

**Step 2: Run the failing tests**

Run:

```bash
go test ./internal/game/server -run 'TestPhase08MarketAuctionPremiumUseServerEconomyState|TestPhase09QuestAdminObservabilityUseServerState' -count=1
```

**Step 3: Wire metrics**

Record existing Phase 12 metric helper series from command handlers after successful committed mutations. Avoid free-form labels and avoid logging secrets or session tokens.

**Step 4: Verify**

Run:

```bash
go test ./internal/game/server -run 'Phase08|Phase09|Observability' -count=1
go test ./internal/game/observability -run TestMetricHelpersRecordPhase12Series -count=1
git diff --check
```

**Step 5: Commit after manager review**

```bash
git add internal/game/server/runtime.go internal/game/server/economy_handlers.go internal/game/server/quest_admin_observability_handlers.go internal/game/server/server_test.go
git commit -m "game: wire runtime economy observability metrics"
```

## Wave 3: Documentation And Stale TODO Cleanup

Run after code waves so `docs/todo.md` updates reflect verified reality.

### Task 9: Quest Objective Schema Documentation

**Files:**
- Modify: `docs/plans/modules/10-quest-board-generation.md`
- Modify: `internal/game/quests/types.go`
- Modify: `internal/game/quests/model_test.go`

**Step 1: Write schema tests if missing**

Confirm MVP public payloads use `ObjectiveSchema.Objectives` and legacy single-objective fields remain internal/backcompat only.

**Step 2: Run focused tests**

```bash
go test ./internal/game/quests -run 'TestMVPObjectiveSchemasValidate|TestInvalidObjectiveSchemasRejected' -count=1
```

**Step 3: Update documentation**

Document `Objectives []Objective` as the preferred schema shape and name the legacy fields as internal/backcompat.

**Step 4: Verify**

```bash
go test ./internal/game/quests -run 'Objective' -count=1
git diff --check
```

**Step 5: Commit after manager review**

```bash
git add docs/plans/modules/10-quest-board-generation.md internal/game/quests/types.go internal/game/quests/model_test.go
git commit -m "docs: document quest objective schema"
```

### Task 10: Quest Reward Inventory Adapter TODO Audit

**Files:**
- Modify: `internal/game/server/quest_admin_observability_handlers.go`
- Modify: `internal/game/server/server_test.go`
- Modify: `docs/todo.md`

**Step 1: Add or tighten server test**

Prove the concrete `questRewardInventoryAdapter` grants quest item rewards through `InventoryService.AddItem` with the quest reward reference.

**Step 2: Run focused test**

```bash
go test ./internal/game/server -run TestPhase09QuestAdminObservabilityUseServerState -count=1
```

**Step 3: Update `docs/todo.md`**

If the adapter is truly wired and tested, move the stale quest adapter TODO from Open to Completed with a source note. Do not close durable rare cap or quest cache todos unless the relevant tasks are implemented and verified.

**Step 4: Verify**

```bash
go test ./internal/game/server -run 'Quest|Phase09' -count=1
git diff --check
```

**Step 5: Commit after manager review**

```bash
git add internal/game/server/quest_admin_observability_handlers.go internal/game/server/server_test.go docs/todo.md
git commit -m "docs: close stale quest reward adapter todo"
```

### Task 11: Unimplemented Mutation Contract Guards

**Files:**
- Modify: `internal/game/realtime/envelope_test.go`
- Modify: `client/src/protocol/envelope.test.ts`
- Modify: `client/tests/browser-smoke.mjs`

**Step 1: Add guard tests**

Assert unimplemented mutation operations are not registered or visible in default UI until their real server-owned contracts exist:

```text
loadout.equip_module
loadout.unequip_module
crafting.start
crafting.complete
crafting.cancel
discovery.claim_planet
planet.building_build
planet.building_upgrade
route.create
route.update
route.enable
route.disable
route.settle
```

**Step 2: Run tests**

```bash
go test ./internal/game/realtime -count=1
cd client && npm --cache /tmp/gameproject-npm-cache run test -- --run
cd client && npm --cache /tmp/gameproject-npm-cache run smoke
```

**Step 3: Update tests only**

Keep this task as a guard. Do not implement the mutations here.

**Step 4: Verify**

```bash
go test ./internal/game/realtime -count=1
cd client && npm --cache /tmp/gameproject-npm-cache run check
git diff --check
```

**Step 5: Commit after manager review**

```bash
git add internal/game/realtime/envelope_test.go client/src/protocol/envelope.test.ts client/tests/browser-smoke.mjs
git commit -m "test: guard unimplemented browser mutation contracts"
```

## Later Prerequisite Design

Do not start these as small Symphony tasks yet. They need design or storage primitives first.

- Durable repository/outbox boundary for loot XP reconciliation, crafting completion recovery, discovery stores, planet claim CAS, production/route events, market settlement, auction settlement, and premium revoke handling.
- Runtime crafting service factory before `crafting.start` or `crafting.complete`: wire `CraftingService`, reservation service, wallet, inventory, progression, station provider, production `CraftLocationAuthorizer`, job snapshots, and deterministic browser seed materials.
- Runtime loadout service source of truth before `loadout.equip_module`: seed real module instances and replace hardcoded starter scanner loadout snapshots with `modules.LoadoutService`.
- Runtime planet claim adapters before `discovery.claim_planet`: rank provider, proximity provider, X Core inventory seed/source/consumer, claim production initializer, and safe event/snapshot publication.
- Building construction/upgrade service before `planet.building_build` or `planet.building_upgrade`: ownership, rank/building requirements, wallet/material debit, storage capacity, idempotency, and events.
- Route mutation exposure before browser route controls: authenticated owner/access wrappers, route create policy provider, route energy/upkeep semantics, and station/storage destination adapters.
- Real death/respawn E2E: combat or zone-worker authority must produce `death.ship_disabled`; tests must not set disabled ship state by hand.
- Dedicated durable rare quest reward cap before X Core or premium quest rewards.
- Wallet-currency market listing model before applying paid-only premium bucket policy to player trades.
- Concrete grant adapters for auction and premium skeleton payloads after owning services expose durable grant primitives.

## Final Verification

After the selected waves land:

```bash
go test ./...
cd client && npm --cache /tmp/gameproject-npm-cache run check
git diff --check
```

Then update `docs/todo.md` only for work actually implemented and verified. Leave all durable/outbox and missing-contract items open with clearer blockers if needed.
