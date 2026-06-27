# Phase 15 — World Performance & AOI/Aggro Optimization

## Status
- State: In progress (90%)
- Wave: 4
- Depends on: P05
- Unlocks: higher player/NPC counts

## Goal
Remove the per-tick hot-path costs: NPC aggro O(NPC×player) scans, per-session AOI
rebuilds, and re-serialization of unchanged entities.

## Why (report refs)
- Code review HI-07: aggro and AOI become O(N×M) bottlenecks.
- Code review §13: AOI snapshots rebuilt per session; no entity versioning.

## Scope
- Spatial-query target acquisition for aggro.
- Per-tick per-map entity snapshot collected once.
- Entity version numbers to skip unchanged payloads.

## Out Of Scope
- Multi-process sharding (post-v1).

## Tasks
- [x] `[P:wave4/lane-F]` Replace `nearestAggroTarget` linear scan with spatial-index radius query.
- [x] `[P:wave4/lane-F]` Add entity-type/layer indexes so aggro skips irrelevant entities.
- [x] `[P:wave4/lane-G]` Collect one per-map entity snapshot per tick; build per-session AOI from it.
- [x] `[P:wave4/lane-G]` Add entity version numbers; skip serializing unchanged entities in AOI diffs.
- [x] `[P:wave4/lane-G]` Add tick sub-phase metrics: movement ms, aggro ms, AOI ms, enqueue ms.
- [x] `[P:wave4/lane-G]` HI-07 load-envelope proof through P13 evidence:
  - [x] `TestPhase13P15WorldRealtimeLoadEnvelopeKeepsAOIWorkBounded` runs 1500
    concurrent AOI viewers over 1552 simulated entity states and keeps visible
    payloads inside the configured 50-100 entity envelope.
  - [x] `TestPhase13P15AggroLoadEnvelopeKeepsCandidateChecksBounded` runs 1500
    players and proves one NPC tick performs one aggro candidate check through
    the player spatial index.
  - [x] `TestPhase13P15RuntimeAOITickStabilityKeepsDurationBudget` runs the
    runtime AOI tick path with 128 active sessions and keeps wall-clock duration
    within the 3s targeted-race budget.

## Server Ownership
- Visibility/radar/stealth still recomputed server-side; optimization must not leak hidden entities.

## Smoke Tests (one assertion each)
- [x] Aggro target selection uses spatial query (no full player scan) — assert via instrumented count.
- [x] An unchanged entity is not re-serialized in the next AOI diff.
- [x] Hidden entity stays excluded after AOI snapshot sharing.
- [x] Tick sub-phase metrics are emitted.
- [x] P13/P15 load envelope keeps AOI payloads and aggro candidate checks bounded.
- [x] Runtime AOI tick stability smoke stays inside the configured wall-clock budget.

## Done Criteria
- [ ] Aggro/AOI no longer scale O(N×M) on the full runtime hot-path envelope.
  - [x] Aggro candidate acquisition is bounded by player spatial radius query.
  - [x] AOI visible payload size is bounded in the load-envelope smoke.
  - [x] AOI runtime work budget has a 128-active-session wall-clock smoke proof.
  - [ ] Full 1500-session runtime AOI projection proof remains open because the
    server still computes per-session AOI diffs from the shared worker snapshot.
- [ ] Code review HI-07 and §13 AOI items closed.
  - [ ] AOI read projection spatial/copy-on-write follow-up remains open for the
    full 1500-session runtime envelope.

## Verification
```bash
go test -race ./internal/game/server/... ./internal/game/world/... -run 'Load|Tick|AOI|Aggro|Race|Phase13|Phase15|Trace' -count=1
go test ./... && git diff --check
```
