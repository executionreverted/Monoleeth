# Phase 07 Quest Board Symphony Plan

> **For Codex Manager:** execute through Symphony-managed workers. Do not give
> workers `AGENTS.md` or `docs/symphony-operating-model.md`; workers read
> `WORKFLOW.md`, `docs/symphony-worker-rules.md`, roadmap/spec files, and their
> task prompt only.

**Goal:** Build the Phase 07 server-authoritative quest board foundation:
10 generated offers, 3 active quests, server-event progress, reroll, and
idempotent rewards.

**Architecture:** Main Codex acts as project manager. Symphony workers implement
small, non-overlapping slices in isolated workspaces; main Codex reviews patch
packets, applies to the feature worktree, verifies, and commits one slice at a
time. Quest code stays in `internal/game/quests`; it calls wallet, inventory,
progression, combat/loot/craft event shapes through service boundaries and does
not trust client-supplied progress or reward data.

**Tech Stack:** Go domain services, in-memory stores matching current project
phase, existing `foundation`, `economy`, `progression`, `combat`, `loot`,
`crafting`, and `events` packages, Symphony local tracker.

---

## Preflight Gate

### Task 0: Merge Phase 06

**Files:**
- Existing worktree:
  `/Users/canersevince/.config/superpowers/worktrees/gameproject/phase06-death-repair-crafting`
- Main repo:
  `/Users/canersevince/gameproject`

**Steps:**
1. In the Phase 06 worktree run:
   `go test ./...`
2. Run:
   `go test -race ./internal/game/death ./internal/game/crafting ./internal/game/ships ./internal/game/economy ./internal/game/loot ./internal/game/foundation`
3. Run:
   `go vet ./...`
4. Run:
   `git diff --check`
5. In main repo on `master`, fast-forward merge `phase06-death-repair-crafting`.
6. Do not push unless explicitly requested.

**Expected:** `master` includes commit `8c98db5 game: harden phase06 retry safety`
or its descendant, and worktree is clean.

---

## Branch Setup

Create a new feature branch/worktree from updated `master`:

```bash
phase07-quest-board-guided-progression
```

Every Symphony task workspace must be prepared by the manager to this branch
HEAD before `/run`.

---

## Wave 1: Models, Catalog, Board

These tasks are mostly parallel if file ownership stays separate.

### Task 1: Quest Domain Model

**Package:** `internal/game/quests`

**Files:**
- Create: `internal/game/quests/doc.go`
- Create: `internal/game/quests/types.go`
- Create: `internal/game/quests/errors.go`
- Create: `internal/game/quests/model.go`
- Test: `internal/game/quests/model_test.go`
- Modify: `docs/roadmap/07-quest-board-guided-progression.md`

**Worker prompt essentials:**
- Read `docs/roadmap/00-index.md`
- Read `docs/roadmap/07-quest-board-guided-progression.md`
- Read `docs/plans/modules/10-quest-board-generation.md`
- Do not implement generation, accept, progress, reroll, or rewards yet.

**Implement:**
- `QuestType`
- `QuestState`
- `QuestTemplate`
- `QuestOffer`
- `PlayerQuest`
- objective schema types for MVP: kill, collect, craft, scan, build, deliver
- reward payload types: credits, items, main XP, role XP, rare hooks placeholder
- validation helpers

**Tests:**
- invalid quest states rejected
- invalid objective schemas rejected
- reward payload validates positive amounts and known reward kinds
- generated offer stores generated payload and reward payload
- player quest state transitions reject invalid jumps

**Validation:**
`go test ./internal/game/quests -count=1`

**Commit:** `game: add quest domain model`

### Task 2: Quest Catalog And Deterministic Board Generator

**Package:** `internal/game/quests`

**Files:**
- Create: `internal/game/quests/catalog.go`
- Create: `internal/game/quests/board.go`
- Test: `internal/game/quests/catalog_test.go`
- Test: `internal/game/quests/board_test.go`
- Modify: `docs/roadmap/07-quest-board-guided-progression.md`

**Depends on:** Task 1.

**Implement:**
- `QuestCatalog`
- MVP templates for kill, scan skeleton, collect, craft, build skeleton, deliver
  skeleton
- `GenerateBoard(input)` returning exactly 10 offers
- deterministic seed handling
- rank/requirement filtering
- simple player-need weighting hook without overfitting
- upfront reward generation
- offer expiration timestamp

**Tests:**
- board generates exactly 10 offers
- same seed/player snapshot produces same offers/rewards
- rank filters low-rank templates
- generated rewards are stored at offer time
- offer expiration set from server clock

**Validation:**
`go test ./internal/game/quests -count=1`

**Commit:** `game: generate quest board offers`

---

## Wave 2: Accept, Progress, Claim

Run these sequentially unless Task 3 is fully committed first.

### Task 3: Quest Store And AcceptQuest

**Package:** `internal/game/quests`

**Files:**
- Create: `internal/game/quests/store.go`
- Create: `internal/game/quests/service.go`
- Test: `internal/game/quests/service_accept_test.go`
- Modify: `docs/roadmap/07-quest-board-guided-progression.md`

**Implement:**
- in-memory store for offers and player quests
- `AcceptQuest(input)`
- offer exists / not expired validation
- active quest count below 3
- requirements still met using player snapshot provider
- accepted quest inserted
- offer marked accepted or removed

**Tests:**
- accept expired offer fails
- accept over 3 active quests fails
- accept requirement mismatch fails
- accepted quest is active and offer cannot be accepted twice

**Validation:**
`go test ./internal/game/quests -count=1`

**Commit:** `game: accept quest board offers`

### Task 4: Server Event Progress Consumers

**Package:** `internal/game/quests`

**Files:**
- Create: `internal/game/quests/progress.go`
- Test: `internal/game/quests/progress_test.go`
- Modify: `docs/roadmap/07-quest-board-guided-progression.md`

**Implement:**
- event input structs/adapters for:
  - `combat.npc_killed`
  - `loot.picked_up`
  - `craft.job_completed`
- skeleton consumers for scan/build/deliver that validate shape but may no-op
  if upstream runtime event does not exist yet
- update only matching active quests
- cap progress at objective target
- mark quest completed exactly once

**Tests:**
- kill event progresses only matching quest
- loot event progresses only matching quest
- craft event progresses only matching quest
- completed quest does not overflow
- client-style direct progress method does not exist

**Validation:**
`go test ./internal/game/quests ./internal/game/combat ./internal/game/loot ./internal/game/crafting -count=1`

**Commit:** `game: progress quests from server events`

### Task 5: ClaimReward

**Package:** `internal/game/quests`

**Files:**
- Create: `internal/game/quests/reward.go`
- Test: `internal/game/quests/reward_test.go`
- Modify: `internal/game/foundation/idempotency.go` if a quest reward helper is
  missing or insufficient
- Modify: `docs/roadmap/07-quest-board-guided-progression.md`

**Implement:**
- `ClaimReward(input)`
- completed and unclaimed validation
- mark claimed before grants or use retry-safe claim record
- grant credits through `WalletService`
- grant items through `InventoryService`
- grant XP through `ProgressionService`
- use `quest_reward:<player_quest_id>`
- duplicate claim returns cached result and does not duplicate value

**Tests:**
- non-completed quest cannot claim
- duplicate claim does not duplicate XP/items/currency
- partial grant failure is either retry-safe or recorded in `docs/todo.md`
- reward error messages do not leak hidden targets

**Validation:**
`go test ./internal/game/quests ./internal/game/economy ./internal/game/progression ./internal/game/foundation -count=1`

**Commit:** `game: claim quest rewards once`

---

## Wave 3: Reroll, Review, Docs

### Task 6: Board Reroll

**Package:** `internal/game/quests`

**Files:**
- Create or modify: `internal/game/quests/reroll.go`
- Test: `internal/game/quests/reroll_test.go`
- Modify: `docs/roadmap/07-quest-board-guided-progression.md`

**Implement:**
- `RerollBoard(input)`
- debit reroll credits through wallet
- expire old unaccepted offers
- leave accepted quests intact
- generate fresh 10 offers
- rare reward cap hook
- reroll cost scaling hook placeholder

**Tests:**
- reroll charges credits
- insufficient credits leaves board unchanged
- accepted quests remain
- rare cap hook can block excessive rare offers

**Validation:**
`go test ./internal/game/quests ./internal/game/economy -count=1`

**Commit:** `game: reroll quest board offers`

### Task 7: Review Workers

Create three review-only Symphony tasks after implementation commits:

1. Security and abuse review
2. Performance/concurrency review
3. Code quality/readability/docs truth review

**Review scope:**
- `internal/game/quests`
- `internal/game/foundation/idempotency.go`
- `docs/roadmap/07-quest-board-guided-progression.md`
- `docs/todo.md`

**Validation:**
- narrow quest tests
- `go test ./...`
- `go test -race ./internal/game/quests ./internal/game/economy ./internal/game/progression`
- `go vet ./...`
- `git diff --check`

**Commit:** fixes as one or more small commits, e.g.
`game: harden quest reward idempotency` or `docs: clarify phase07 follow-ups`.

---

## Symphony Execution Rules

For each implementation task:

1. Create local Symphony task with explicit description.
2. Worker prompt must include:
   - read `WORKFLOW.md`
   - read `docs/symphony-worker-rules.md`
   - do not read `AGENTS.md`
   - do not read `docs/symphony-operating-model.md`
   - do not spawn subagents
   - do not commit
   - do not manage Symphony
3. Manager prepares workspace branch.
4. Run task.
5. Wait with long `/wait`, no stream polling unless debugging.
6. Fetch workspace diff.
7. Manager reviews patch.
8. Apply to feature worktree.
9. Run narrow tests, then `go test ./...` and `git diff --check`.
10. Commit one slice.

## Final Phase Verification

Before claiming Phase 07 slice complete:

```bash
go test ./...
go test -race ./internal/game/quests ./internal/game/economy ./internal/game/progression ./internal/game/foundation
go vet ./...
git diff --check
```

Roadmap checkboxes must only be checked for behavior implemented and verified.

## Deferred On Purpose

- Realtime/API quest endpoints
- Database persistence/outbox
- Market wash-trade quest types
- Planet/building ownership validation beyond skeletons unless the phase work
  explicitly adds required providers
- Full rare reward economy balancing
- Client UI

