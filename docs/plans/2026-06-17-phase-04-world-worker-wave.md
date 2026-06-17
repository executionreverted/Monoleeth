# Phase 04 World Worker Wave Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use superpowers:executing-plans and superpowers:subagent-driven-development to implement this plan task-by-task through Symphony.

**Goal:** Build the first server-authoritative world worker foundation: world/entity types, movement intents, spatial AOI, visibility filtering, and realtime envelope contracts.

**Architecture:** The work is split into conflict-light packages so Symphony can run several workers in parallel. The main Codex session acts as conductor: create tasks, wait with `/wait`, review patch packets, apply one patch at a time, run verification, commit small slices, and keep `docs/roadmap/04-world-worker-aoi-fog-realtime.md` truthful. Workers must only read `docs/symphony-worker-rules.md`, not `AGENTS.md` or `docs/symphony-operating-model.md`.

**Tech Stack:** Go, in-memory domain stores/test harnesses, standard `testing`, existing `internal/game/foundation`, `internal/game/contracts`, `internal/game/events`, and Phase 03 stat snapshot value objects.

---

## Current State

- Phase 01 and Phase 02 are complete/audited.
- Phase 03 is effectively ready for Phase 04, with one deliberate follow-up still open: XP source completion spoofing will be closed when XP grants are wired behind real domain owners.
- Worktree may contain unrelated user changes; do not stage or overwrite them.

## Required Reading

Main Codex session:

- `AGENTS.md`
- `docs/symphony-operating-model.md`
- `docs/roadmap/00-index.md`
- `docs/roadmap/04-world-worker-aoi-fog-realtime.md`
- `docs/2026-06-16-space-morpg-architecture-notes.md`
- `docs/2026-06-17-world-system-design.md`
- `docs/plans/modules/14-world-aoi-fog-security.md`
- `docs/plans/modules/15-api-events-errors.md`
- `docs/plans/modules/16-testing-observability-balancing.md`

Symphony workers:

- `docs/symphony-worker-rules.md`
- `docs/roadmap/00-index.md`
- `docs/roadmap/04-world-worker-aoi-fog-realtime.md`
- the module specs explicitly named in their task prompt

Workers must not read:

- `AGENTS.md`
- `docs/symphony-operating-model.md`

## Wave Strategy

Run Wave 1 with three parallel tasks because they can own mostly separate packages:

1. `world` model and movement primitives.
2. `world/spatial` spatial hash and AOI query.
3. `realtime` JSON envelope contracts and request cache skeleton.

After those are committed, run Wave 2 with two or three tasks:

1. world worker tick loop and command mailbox.
2. visibility filtering and interaction checks.
3. AOI visible diff/snapshot payloads.

Do not start combat or loot yet. Phase 04 must first prove position authority and hidden-data filtering.

## Wave 1 Task A: World Model And Movement Primitives

**Symphony Task Title:** `Phase 04 W1-A: world model and movement primitives`

**Files:**

- Create: `internal/game/world/doc.go`
- Create: `internal/game/world/types.go`
- Create: `internal/game/world/movement.go`
- Create: `internal/game/world/movement_test.go`
- Modify only after main review: `docs/roadmap/04-world-worker-aoi-fog-realtime.md`

**Scope:**

- Define `WorldID`, `ZoneID`, `EntityID`, `EntityType`, `Vec2`, `Entity`, `MovementState`, and `MovementIntent`.
- Define entity kinds needed by the roadmap: player, NPC placeholder, loot placeholder, planet signal placeholder.
- Implement server-owned movement toward target:
  - input is current position, target, speed, tick delta
  - output is new position and done flag
  - never accept client final position
  - clamp zero/negative speed and zero/negative delta safely
- Add distance helpers for AOI/visibility reuse.

**Tests:**

- Movement advances by server speed and tick delta.
- Movement stops exactly at target without overshoot.
- Client supplied final position is not part of the movement API.
- Invalid IDs/types/positions are rejected.

**Validation:**

```bash
go test ./internal/game/world -count=1
git diff --check
```

**Commit after review:**

```bash
git add internal/game/world docs/roadmap/04-world-worker-aoi-fog-realtime.md
git commit -m "game: add world movement primitives"
```

## Wave 1 Task B: Spatial Hash And AOI Query

**Symphony Task Title:** `Phase 04 W1-B: spatial hash and AOI query`

**Files:**

- Create: `internal/game/world/spatial/doc.go`
- Create: `internal/game/world/spatial/hash.go`
- Create: `internal/game/world/spatial/hash_test.go`
- Create: `internal/game/world/spatial/index.go`
- Create: `internal/game/world/spatial/index_test.go`
- Modify only after main review: `docs/roadmap/04-world-worker-aoi-fog-realtime.md`

**Scope:**

- Define cell coordinate calculation using `math.Floor`, including negative coordinates.
- Implement fixed-cell spatial index:
  - insert entity position
  - update membership when position changes
  - remove entity
  - radius query with exact distance filter
- Use value objects or interfaces so this package does not own gameplay visibility.

**Tests:**

- Cell coordinate calculation handles positive, zero, and negative positions.
- Radius query returns nearby entities.
- Radius query excludes far entities after exact distance check.
- Moving an entity updates cell membership.
- Removing an entity removes it from future queries.
- Deterministic ordering for query results.

**Validation:**

```bash
go test ./internal/game/world/spatial -count=1
git diff --check
```

**Commit after review:**

```bash
git add internal/game/world/spatial docs/roadmap/04-world-worker-aoi-fog-realtime.md
git commit -m "game: add world spatial hash"
```

## Wave 1 Task C: Realtime Envelope Contracts

**Symphony Task Title:** `Phase 04 W1-C: realtime envelope contracts`

**Files:**

- Create: `internal/game/realtime/doc.go`
- Create: `internal/game/realtime/envelope.go`
- Create: `internal/game/realtime/envelope_test.go`
- Create: `internal/game/realtime/request_cache.go`
- Create: `internal/game/realtime/request_cache_test.go`
- Modify only after main review: `docs/roadmap/04-world-worker-aoi-fog-realtime.md`

**Scope:**

- Define JSON request, response, error, and event envelopes.
- Require `request_id`, `op`, and payload presence for requests.
- Define operation constants for Phase 04:
  - `move_to`
  - `stop`
  - `debug_spawn_npc`
  - `debug_snapshot`
- Define client event constants:
  - `player.snapshot`
  - `aoi.entity_entered`
  - `aoi.entity_left`
  - `position.corrected`
- Implement request ID cache skeleton for safe retry behavior:
  - remember response by session/request id
  - duplicate request returns cached response
  - bounded capacity or explicit test-friendly eviction
- Add placeholder rate-limit posture type, but do not build a real limiter yet.

**Tests:**

- Missing request id rejected.
- Missing op rejected.
- Invalid payload rejected.
- Duplicate request id returns cached response.
- Event envelope marshals without hidden/internal fields.

**Validation:**

```bash
go test ./internal/game/realtime -count=1
git diff --check
```

**Commit after review:**

```bash
git add internal/game/realtime docs/roadmap/04-world-worker-aoi-fog-realtime.md
git commit -m "game: add realtime envelope contracts"
```

## Wave 2 Task A: Single Zone Worker Tick Loop

**Symphony Task Title:** `Phase 04 W2-A: single zone worker tick loop`

**Files:**

- Create: `internal/game/world/worker/doc.go`
- Create: `internal/game/world/worker/commands.go`
- Create: `internal/game/world/worker/worker.go`
- Create: `internal/game/world/worker/worker_test.go`
- Modify: `internal/game/world/types.go` only if Wave 1 model needs a small additive field.
- Modify: `docs/roadmap/04-world-worker-aoi-fog-realtime.md`

**Depends on:** Wave 1 Task A.

**Scope:**

- Implement in-process worker harness, not network gateway.
- Define command mailbox interface.
- Implement deterministic tick step:
  - drain commands in arrival order
  - apply movement intents
  - fixed delta support
  - delayed task scheduler skeleton
- Implement entity insert/remove/update.
- Implement player session attachment model.

**Tests:**

- Worker can spawn a player.
- `move_to` command updates position by server speed.
- `stop` clears movement target.
- Impossible client final position has no field/path to mutate state.
- Command drain order is deterministic.

**Validation:**

```bash
go test ./internal/game/world/worker -count=1
go test ./internal/game/world/... -count=1
git diff --check
```

**Commit after review:**

```bash
git add internal/game/world docs/roadmap/04-world-worker-aoi-fog-realtime.md
git commit -m "game: add single zone worker loop"
```

## Wave 2 Task B: Visibility And Fog Skeleton

**Symphony Task Title:** `Phase 04 W2-B: visibility and fog skeleton`

**Files:**

- Create: `internal/game/world/visibility/doc.go`
- Create: `internal/game/world/visibility/visibility.go`
- Create: `internal/game/world/visibility/visibility_test.go`
- Create: `internal/game/world/visibility/fog.go`
- Create: `internal/game/world/visibility/fog_test.go`
- Modify: `docs/roadmap/04-world-worker-aoi-fog-realtime.md`

**Depends on:** Wave 1 Task A.

**Scope:**

- Implement `CanSendEntityToClient`.
- Implement `CanInteract`.
- Inputs:
  - viewer position
  - viewer radar range from server stat snapshot
  - entity position
  - entity signature
  - hidden flag
  - world/zone identity
- Hidden entity returns generic not visible/not found style error.
- Fog memory skeleton can remember known planet/intel summaries, but it must not grant live interaction permission.
- Scanner bridge event skeleton only; no scan roll mechanics.

**Tests:**

- Hidden entity is not serializable.
- Normal entity in radar/AOI can be sent.
- Entity outside range cannot be sent.
- Interaction with hidden entity fails.
- Fog memory does not allow live interaction.
- Radar spoof defense is represented by accepting radar range as server-provided input, not client payload.

**Validation:**

```bash
go test ./internal/game/world/visibility -count=1
go test ./internal/game/world/... -count=1
git diff --check
```

**Commit after review:**

```bash
git add internal/game/world/visibility docs/roadmap/04-world-worker-aoi-fog-realtime.md
git commit -m "game: add world visibility filtering"
```

## Wave 2 Task C: AOI Snapshot And Visible Diff

**Symphony Task Title:** `Phase 04 W2-C: AOI visible diff snapshots`

**Files:**

- Create: `internal/game/world/aoi/doc.go`
- Create: `internal/game/world/aoi/snapshot.go`
- Create: `internal/game/world/aoi/snapshot_test.go`
- Modify: `internal/game/world/worker/worker.go` only if needed to expose snapshot inputs.
- Modify: `docs/roadmap/04-world-worker-aoi-fog-realtime.md`

**Depends on:** Wave 1 Task B and Wave 2 Task B.

**Scope:**

- Build visible snapshot payload containing only filtered public entity data.
- Implement entered/updated/left diff between previous and current visible entity sets.
- Ensure internal entity can contain hidden fields that never appear in client payload.
- Define public payload fields intentionally:
  - entity id
  - entity type
  - public position
  - public status flags only
  - no seed, no hidden metadata, no future spawn data

**Tests:**

- Hidden entity never appears in snapshot.
- Entity entering AOI emits entered.
- Entity moving inside AOI emits updated.
- Entity leaving AOI emits left.
- Snapshot payload omits hidden/internal fields.
- AOI stress test with many entities stays deterministic.

**Validation:**

```bash
go test ./internal/game/world/aoi -count=1
go test ./internal/game/world/... -count=1
git diff --check
```

**Commit after review:**

```bash
git add internal/game/world/aoi docs/roadmap/04-world-worker-aoi-fog-realtime.md
git commit -m "game: add AOI visible snapshots"
```

## Final Phase 04 Wave Audit

After Wave 1 and Wave 2 commits:

```bash
go test ./...
go test -race ./internal/game/world/... ./internal/game/realtime
git diff --check
```

Review checklist:

- Security: no hidden entity or gameplay seed can serialize to client payloads.
- Security: interaction helpers recheck visibility/range.
- Performance: spatial hash radius query has bounded candidate cells and exact distance filtering.
- Code quality: world, worker, spatial, visibility, aoi, and realtime package boundaries stay separate.
- Roadmap: check only Phase 04 TODOs proven by tests.

Final commit if docs-only updates remain:

```bash
git add docs/roadmap/04-world-worker-aoi-fog-realtime.md docs/roadmap/00-index.md
git commit -m "docs: record phase 04 world worker progress"
```

## Deferred On Purpose

- No combat damage.
- No loot pickup.
- No planet generation truth.
- No scanner roll/discovery mechanics.
- No polished WebSocket gateway.
- No browser client.
- No DB persistence for world state.
- No distributed multi-zone routing.

These remain later roadmap work.
