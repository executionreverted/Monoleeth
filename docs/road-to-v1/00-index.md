# Road To v1 — Main Goal & Phase Index

> Source of truth for this plan: `docs/code-review/game-systems-code-review.md` and
> `docs/code-review/feature-gap-analysis.md`. This folder turns those two reports
> into executable phases. One file per phase. Keep slices small, server-authoritative,
> and AGENTS.md-compliant.

## Objective

Take the current playable MVP (≈72% local loop) to a credible, durable, social,
DarkOrbit-like v1:

```text
Persistence -> Correctness -> Hardening -> Equipment/Progression -> Social -> First Endgame -> Flavor -> Release
```

v1 done means: durable player/economy/world state, enforced abuse protection,
correct equipment/progression, a real social layer, one repeatable endgame loop,
and a green release gate.

## How To Use This Folder

- Each phase file is a self-contained work order with TODO checkboxes.
- Track progress by checking boxes inside each phase file AND the dashboard below.
- Do not mark a box done until code + a focused test + (for UI) a browser proof pass.
- Keep changes scoped to the phase. Cross-phase work needs an explicit note.
- Follow `AGENTS.md`: small files, domain names, `lock -> validate -> mutate ->
  ledger/event -> commit -> broadcast`, idempotency keys, no monolith, no fake state.
- Use Context7 MCP before DB/pgx/Redis/NATS/library syntax work.

## Smoke Test Rule

Every smoke/e2e test in these phases must assert exactly ONE behavior. No long
multi-step mega-smokes. If you need to cover N behaviors, write N short tests.

## Symphony Parallelization Rule

- Tasks marked `[P:wave-N/lane-X]` can run as parallel Symphony agents in the same wave.
- Lanes inside a wave must have disjoint write sets (no two agents editing the same files).
- Agents are not alone in the codebase: do not revert others' edits; adapt to them.
- Keep Symphony/orchestration code separate from gameplay domain code.

## Phase List

1. [Persistence Foundation](./01-persistence-foundation.md)
2. [Transactional Economy & Outbox](./02-transactional-economy-outbox.md)
3. [Realtime Hardening (Writer Queue, Replay, Cache Key)](./03-realtime-hardening.md)
4. [Rate Limiting & Abuse Posture](./04-rate-limiting-abuse.md)
5. [Map Worker Ownership & Concurrency](./05-map-ownership-concurrency.md)
6. [Movement, Combat & Death Correctness](./06-movement-combat-death-correctness.md)
7. [Equipment & Progression Closure](./07-equipment-progression-closure.md)
8. [Durable Planet, Production & Routes](./08-durable-planet-production-routes.md)
9. [CMS Completion & Balance Telemetry](./09-cms-completion-balance.md)
10. [Social MVP (Chat, Party, Clan)](./10-social-mvp.md)
11. [First Endgame Loop (Signal Gate)](./11-first-endgame-signal-gate.md)
12. [DarkOrbit Flavor (Drones, P.E.T., Ammo, Honor)](./12-darkorbit-flavor.md)
13. [Observability, Simulation & Release Gate](./13-observability-simulation-release.md)
14. [CMS Runtime Application & Live Content Safety](./14-cms-runtime-application-content-safety.md)
15. [World Performance & AOI/Aggro Optimization](./15-world-performance-aoi-optimization.md)
16. [Production Config & Operational Hardening](./16-production-config-operational-hardening.md)
17. [Runtime Decomposition & Maintainability](./17-runtime-decomposition-maintainability.md)

## Dependency / Wave Map

```text
Wave 1 (parallel):  P01 persistence | P03 realtime hardening | P04 rate limiting
Wave 2 (parallel):  P02 economy (P01) | P05 concurrency (P01) | P06 combat correctness | P16 prod config (P01)
Wave 3 (parallel):  P07 equipment (P02) | P08 planet durable (P02) | P09 CMS (P01) | P14 CMS runtime apply (P01,P09)
Wave 4 (parallel):  P10 social (P01) | P13 observability/sim (P02,P05) | P15 world perf (P05)
Wave 5:             P11 endgame gate (P06,P07)
Wave 6:             P12 flavor (P07,P11) | P17 runtime decomposition (continuous, P02,P05)
```

## Report Finding Coverage Map

Confirms every Critical/High/Medium finding from the code review has a home.

| Finding | Owning phase |
| --- | --- |
| CR-01 volatile core state | P01, P08 |
| CR-02 slow-client blocks tick | P03 |
| CR-03 non-transactional economy | P02 |
| HI-01 global runtime mutex | P05 |
| HI-02 CMS publish not applied to runtime | P14 ✅ |
| HI-03 stop does not settle position | P06 |
| HI-04 no durable event replay | P03 |
| HI-05 rate limits metadata-only | P04 |
| HI-06 worker state not synchronized | P05 |
| HI-07 aggro/AOI O(N×M) | P15 |
| HI-08 narrow CMS publish safety | P14 ✅ |
| MD-01 request cache mismatch | P03 |
| MD-02 prod secure cookie | P16 |
| MD-03 combat/loot stale position | P06 |
| MD-04 debug ops in protocol | P16 |
| MD-05 telemetry errors invisible | P16, P13 |
| §12 structured operational logs | P16 |
| §13 load/scalability | P13, P15 |
| §14 simulation/race tests | P13 |
| §15 Runtime too large | P17 |
| Feature-gap persistence/economy | P01, P02, P08 |
| Feature-gap equipment/progression | P07 |
| Feature-gap social (chat/party/clan) | P10 |
| Feature-gap endgame (gates) | P11 |
| Feature-gap drones/P.E.T./ammo/honor | P12 |

## Progress Dashboard

Update the bar as boxes close inside each phase file. Bar = 10 cells
(`█` done, `░` remaining). Suggested status: Not started / In progress / Done / Blocked.

| # | Phase | Wave | Status | Progress |
| --- | --- | :---: | --- | --- |
| 01 | Persistence Foundation | 1 | Done | `██████████` 100% |
| 02 | Transactional Economy & Outbox | 2 | Done | `██████████` 100% |
| 03 | Realtime Hardening | 1 | Done | `██████████` 100% |
| 04 | Rate Limiting & Abuse Posture | 1 | Done | `██████████` 100% |
| 05 | Map Worker Ownership & Concurrency | 2 | In progress | `█████████░` 90% |
| 06 | Movement, Combat & Death Correctness | 2 | Done | `██████████` 100% |
| 07 | Equipment & Progression Closure | 3 | Done | `██████████` 100% |
| 08 | Durable Planet, Production & Routes | 3 | In progress | `█████████░` 90% |
| 09 | CMS Completion & Balance Telemetry | 3 | Done | `██████████` 100% |
| 10 | Social MVP | 4 | Done | `██████████` 100% |
| 11 | First Endgame Loop (Signal Gate) | 5 | Not started | `░░░░░░░░░░` 0% |
| 12 | DarkOrbit Flavor | 6 | Not started | `░░░░░░░░░░` 0% |
| 13 | Observability, Simulation & Release Gate | 4 | In progress | `███░░░░░░░` 30% |
| 14 | CMS Runtime Application & Content Safety | 3 | Done | `██████████` 100% |
| 15 | World Performance & AOI/Aggro Optimization | 4 | In progress | `███████░░░` 70% |
| 16 | Production Config & Operational Hardening | 2 | Done | `██████████` 100% |
| 17 | Runtime Decomposition & Maintainability | 6 | Not started | `░░░░░░░░░░` 0% |
| — | **Overall v1** | — | In progress | `███████░░░` 74% |

### Progress bar legend

```text
░░░░░░░░░░ 0%     ██████░░░░ 60%
███░░░░░░░ 30%    ██████████ 100%
```

## Global Verification (run before any phase handoff)

```bash
go test ./...
cd client && npm --cache /tmp/gameproject-npm-cache run check
git diff --check
```

## v1 Exit Criteria

- [ ] No core player/economy/world state lost on server restart.
- [ ] Every value mutation is transactional, idempotent, and broadcast after commit.
- [ ] Every client op has an enforced rate limit; abuse cannot mutate truth.
- [ ] Equipment/skills change effective stats and visible/server cargo correctly.
- [ ] CMS publish reaches live runtime or honestly reports pending restart.
- [ ] Aggro/AOI hot path no longer scales O(N×M).
- [ ] Unsafe production config cannot boot; critical transitions are traceable.
- [x] Chat + party + clan MVP work with moderation/rate limits.
- [ ] One repeatable endgame gate loop works end-to-end.
- [ ] Release gate is green with simulation/load/race evidence.
