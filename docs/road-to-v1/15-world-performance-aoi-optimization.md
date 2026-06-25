# Phase 15 — World Performance & AOI/Aggro Optimization

## Status
- State: Not started
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
- [ ] `[P:wave4/lane-F]` Replace `nearestAggroTarget` linear scan with spatial-index radius query.
- [ ] `[P:wave4/lane-F]` Add entity-type/layer indexes so aggro skips irrelevant entities.
- [ ] `[P:wave4/lane-G]` Collect one per-map entity snapshot per tick; build per-session AOI from it.
- [ ] `[P:wave4/lane-G]` Add entity version numbers; skip serializing unchanged entities in AOI diffs.
- [ ] `[P:wave4/lane-G]` Add tick sub-phase metrics: movement ms, aggro ms, AOI ms, enqueue ms.

## Server Ownership
- Visibility/radar/stealth still recomputed server-side; optimization must not leak hidden entities.

## Smoke Tests (one assertion each)
- [ ] Aggro target selection uses spatial query (no full player scan) — assert via instrumented count.
- [ ] An unchanged entity is not re-serialized in the next AOI diff.
- [ ] Hidden entity stays excluded after AOI snapshot sharing.
- [ ] Tick sub-phase metrics are emitted.

## Done Criteria
- [ ] Aggro/AOI no longer scale O(N×M) on the hot path.
- [ ] Code review HI-07 and §13 AOI items closed.

## Verification
```bash
go test ./internal/game/world/... ./internal/game/server/... -run 'Aggro|AOI|Spatial|EntityVersion' -count=1 -race
go test ./... && git diff --check
```
