# Feature Gap & Completion Analysis vs DarkOrbit-like Target

> Scope: current repository state under `/Users/canersevince/gameproject`, local docs, local Go/TypeScript game code, and public DarkOrbit/reference-game research.  
> Date: 2026-06-25  
> Constraint followed: no gameplay/source code changed; this report is the only intended output.  
> Companion report: `docs/code-review/game-systems-code-review.md` covers production code risks in more detail. This document focuses on **feature completeness, promised-vs-delivered gaps, DarkOrbit comparison, and roadmap priority**.

---

## 1. Executive Summary

The project is no longer just a mockup. It has a real authenticated browser game loop with server-owned state: register/login, session-owned WebSocket, deterministic starter spawn, bounded maps, AOI, movement, NPC combat, loot pickup, shield repair, scanner/planet discovery, planet claim with X Core consumption, production/building basics, route create/settle, portals to additional maps, PvP death/repair proof, shop/market/auction/premium/quests/admin/observability UI surfaces, and an active PostgreSQL-backed CMS content effort.

The project is, however, still **far from a full DarkOrbit-class game**. It is best described as:

```text
Production architecture direction: strong
Playable vertical slice: medium-to-strong
DarkOrbit feature parity: early/mid MVP
Production persistence/scalability: early
Social/live-ops/endgame: mostly missing
```

### Overall completion estimate

| Lens | Estimated completion | Meaning |
| --- | ---: | --- |
| Playable local MVP vertical slice | **70%** | The core fly/fight/loot/scan/claim/build/route loop works locally with real browser/server state. |
| Active UI implementation plan checklist | **96% raw checklist** | Most UI phase checkboxes are closed, but some phase docs mark MVP complete while deferring important durable/mutation work to `docs/todo.md`. |
| Backend domain breadth vs internal design docs | **65%** | Most planned domain services exist at least as MVP/in-memory implementations. |
| Production readiness | **30%** | Durable player state, multi-process ownership, real rate limits, transactional economy, and operational scale are not ready. |
| DarkOrbit-like feature parity | **38%** | The skeleton of maps/combat/economy/progression exists, but drones/P.E.T./Skylab/Galaxy Gates/clans/chat/factions/rank ladders/ammo/deep equipment/endgame are mostly absent. |
| Long-term MORPG retention loop | **45%** | Planet/production/route mechanics are promising and distinct, but social, live ops, goals, and durable economy need major work. |

### Biggest “you said you would do it, but it is not done yet” items

1. **Durable player/game persistence**: architecture docs call for PostgreSQL for accounts, inventory, economy, market, progression, and world state; current CMS content has PostgreSQL, but most player/game state is still in-memory.
2. **Real rate limiting**: operation registry has `RateLimitPosture`, but `internal/game/realtime/envelope.go` explicitly says it is metadata only.
3. **Actor-style map worker architecture**: docs target map actors behind gateway/router; current runtime still centralizes many operations through `Runtime.mu`.
4. **Inventory movement and skill unlock**: planned in Phase 06, still open (`inventory.move`, `progression.unlock_skill`, skill tree UI/actions).
5. **Cargo/stat recalculation after modules**: docs and earlier audits call out effective stats; runtime still has gaps around stat/cargo refresh after module/loadout mutation.
6. **Non-starter ship acquisition loop**: hangar exists, ship products/content exist, but a meaningful buy/craft/auction-to-hangar progression path is not complete enough for a DarkOrbit-like ship ladder.
7. **Social layer**: chat, clan/alliance, groups/party, friends, mail/social topbar states are missing/locked.
8. **DarkOrbit endgame equivalents**: Galaxy Gates, group wave gates, Clan Battle Stations/territory wars, drone/P.E.T. systems, Skylab/refinery-like production, ammo/rocket/consumable depth, rankings/honor/companies are not implemented.
9. **CMS completeness**: DB-backed content load/publish is strong, but quest-row publish coverage, diff API/view, audit action field/migration, hot reload, and full admin surfaces are still incomplete.
10. **Production release foundation**: no durable transactions/outbox across most economy/gameplay state, no Redis/NATS/JetStream, no external observability stack, no real abuse/bot posture.

### Recommended next strategic direction

For this specific game, I would **not** try to clone every DarkOrbit feature immediately. The strongest unique hook in your docs is:

```text
Fly -> Fight/Gather/Scan -> Loot -> Craft/Sell -> Upgrade -> Colonize -> Produce -> Expand
```

That means the best next milestone should be:

1. **Make persistence real** for player/account/economy/world core state.
2. **Finish progression/equipment correctness**: skill unlock, inventory move, effective stats, real cargo capacity, ship acquisition.
3. **Add social retention minimum**: chat + party/group + clan foundation.
4. **Add one DarkOrbit-style endgame loop**: Galaxy Gate-like instanced wave gate OR Clan Battle Station-like contested structures. Do one very well.
5. **Then add drones/P.E.T.-like companions** only after equipment/stat systems are stable.

---

## 2. Methodology

This analysis combines:

- Local code inspection:
  - `internal/game/*`
  - `client/src/*`
  - realtime operation registry in `internal/game/realtime/envelope.go`
  - runtime/server handlers under `internal/game/server/*`
- Local design/status docs:
  - `GOAL.md`
  - `docs/2026-06-16-space-morpg-architecture-notes.md`
  - `docs/2026-06-17-world-system-design.md`
  - `docs/2026-06-17-progression-economy-systems-design.md`
  - `docs/plans/modules/*`
  - `docs/plans/ui-implementation/*`
  - `docs/plans/ui-patch-3/*`
  - `docs/plans/cms-rework/*`
  - `docs/map-rework/*`
  - `docs/todo.md`
  - `docs/playtest-vertical-slice-status.md`
- Public game reference research:
  - DarkOrbit Reloaded wiki/pages for Galaxy Gates, Skylab, P.E.T., Drones, Hangar, UI, Missions, Clan Battle Stations, factions/clans/PvP framing.
  - EVE Online official pages for corporations/alliances, player economy, industry, missions, and sandbox retention ideas.
  - Dark Forest references for fog-of-war/incomplete-information exploration design.
  - Similar browser-space MMO references for practical modern expectations.

### Percentages are estimates

The percentages below are **product-readiness estimates**, not exact test coverage. A system can have 100% phase checklist completion but only 50% production readiness if it is still in-memory, lacks social/live-op depth, or has MVP-only content.

---

## 3. Current Game Identity vs DarkOrbit

### Your game’s current identity

Your design docs position the game as:

- Browser-first 2D space MORPG.
- Server-authoritative Go backend.
- DarkOrbit-style movement/combat/HUD inspiration.
- OGame/Travian-style long-term production and persistent strategy.
- Dark Forest-style hidden information, scanning, and intel economy.
- Real authenticated browser client with no fake gameplay state.

This identity is coherent and actually more interesting than a pure DarkOrbit clone. The **planet discovery + intel + production route** axis is a differentiator.

### DarkOrbit baseline feature set

A DarkOrbit-like game generally includes these feature pillars:

| DarkOrbit pillar | Reference behavior | Your current state |
| --- | --- | --- |
| Browser 2D/space MMO surface | Top-down/isometric space combat with maps, portals, UI windows, quickslots | **Partial/strong MVP**: PixiJS map, HUD, portals, quick actions, real browser state. |
| Three companies/factions | Players choose corporate allegiance; factions frame PvP/social identity | **Missing**: no companies/factions. |
| Clans/social hierarchy | Clans, clan stats, clan station ownership, group/social UI | **Mostly missing**: no clan, group, chat, friends, mail. |
| PvE aliens/NPC farming | Kill aliens for XP/currency/cargo, respawn loops, bosses | **Partial**: seeded NPCs, aggro/leash, drops, respawn proof, but content variety is thin. |
| PvP maps/policy | Rival players, safe zones/protection, PvP death/repair | **Partial**: public `1-3` PvP/death/repair proof exists; no faction/honor/bounty/ranking loop. |
| Hangar/equipment | Ship list, equipment slots, inventory drag/drop, multiple configs | **Partial**: hangar/loadout/equip exists; module grid/filtering and stat correctness incomplete. |
| Drones/formations | Drones add equip slots, levels, formations, designs | **Missing**. |
| P.E.T. companion | Autonomous pet levels, gear, auto-loot/resource, combat/support roles | **Missing**. |
| Skylab/refining | Ore/refinery/transport/building-like resource economy | **Partial but different**: planet production/buildings/routes exist, not Skylab/refinery UX. |
| Galaxy Gates | Build gate parts, enter wave instances, earn major rewards | **Missing**. |
| Missions/story/daily/event | Standard/daily/challenge/event/epic missions | **Partial**: quest board/accept/progress/claim/reroll exists; content and narrative variety limited. |
| Auction/shop/assembly | Store, auction, assembly/crafting, premium currency | **Partial**: shop/market/auction/premium/crafting exist; production-grade economy not durable. |
| Pilot Bio/skill tree | Skill points/passives/build specialization | **Partial backend, missing public action/UI**: progression snapshot exists, unlock action open. |
| Rankings/honor/title | Experience, honor, titles, rankings, faction/clan hierarchy | **Mostly missing**: rank/progression exists, but no public ladder/honor/fame. |
| Bonus boxes/resources | Collect boxes/ore/cargo, often P.E.T.-assisted | **Partial**: loot drops/cargo; no field resource/bonus-box ecosystem. |
| Live events/endgame | gates, event missions, invasion gates, CBS prime time, seasonal content | **Mostly missing**: observability/release gate exists, live content mechanics not. |

---

## 4. What Is Done Now

### 4.1 Real authenticated browser game shell — **done for MVP**

Evidence:

- `internal/game/auth/*`
- `internal/game/realtime/*`
- `internal/game/server/server.go`
- `internal/game/server/runtime.go`
- `client/src/auth/auth-client.ts`
- `client/src/net/realtime-client.ts`
- `client/src/state/reducer-auth.test.ts`
- `docs/plans/ui-implementation/01-auth-accounts-sessions.md`
- `docs/plans/ui-implementation/02-game-server-transport-runtime.md`
- `docs/plans/ui-implementation/03-client-auth-shell-demo-removal.md`

Delivered:

- Mail/password registration/login.
- Password hashing.
- HttpOnly cookie session flow.
- Session resolver for WebSocket.
- Server-owned account/player/session identity.
- No default fake gameplay state.
- Auth shell and disconnected/locked/loading states.

Remaining risk:

- Auth/session store is not production-durable enough for real user lifecycle unless backed by a real DB/session backend.
- Login/register rate limit posture exists but not robust enforced production throttling.

Estimated completion:

| Scope | Completion |
| --- | ---: |
| Local MVP | **90%** |
| Production-ready identity/session | **55%** |

### 4.2 World, movement, maps, AOI, portals — **solid MVP**

Evidence:

- `internal/game/world/*`
- `internal/game/world/worker/*`
- `internal/game/world/maps/*`
- `internal/game/server/runtime_maps.go`
- `internal/game/server/portal_handlers.go`
- `internal/game/server/server_map_transport_test.go`
- `docs/map-rework/*`
- `docs/plans/ui-implementation/04-live-world-aoi-movement.md`

Delivered:

- Bounded map catalog.
- Runtime worker per configured map.
- Movement intent, stop, server speed/position authority.
- AOI snapshots and events.
- Hidden entity filtering.
- Portal handoff across public maps.
- PvP map policy and safe/protection states.
- Reconnect snapshot/cursor posture.

Remaining:

- Runtime still has a central coordination lock (`Runtime.mu`) in many flows.
- No multi-process map ownership/handoff protocol.
- No Redis/NATS/world-router layer.
- Browser remembered map/intel projection is still limited.

Estimated completion:

| Scope | Completion |
| --- | ---: |
| Local MVP maps/movement | **80%** |
| Scalable MORPG map architecture | **40%** |
| DarkOrbit map/portal parity | **55%** |

### 4.3 Combat, loot, death, repair — **playable but mechanically shallow**

Evidence:

- `internal/game/combat/*`
- `internal/game/loot/*`
- `internal/game/death/*`
- `internal/game/server/combat_loot_*.go`
- `internal/game/server/death_*.go`
- `internal/game/server/shield_repair.go`
- `client/src/render/world-renderer-effects.ts`
- `client/src/state/reducer-combat-loot.test.ts`
- `docs/plans/ui-implementation/05-combat-loot-death-repair.md`

Delivered:

- Server-side skill use: `combat.use_skill`.
- Range/visibility/cooldown/energy validation.
- NPC kill and loot creation.
- Loot pickup with ownership/range/cargo checks.
- PvP death/repair proof in seeded PvP map.
- Shield repair tick inspired by DarkOrbit-style out-of-combat repair.
- Client action bar, target panel, combat log, projectile feedback.

Open from local docs:

- `Add death/disabled ship event mapper` is still open in Phase 05 checklist.
- Repair tests around stale/tampered quote, insufficient wallet, and debit/re-enable remain open in Phase 05 checklist.
- Durable reward/outbox reconciliation for loot XP is open in `docs/todo.md`.
- `DeathService.ProcessDeath` still has ownership/provider hardening items in `docs/todo.md`.

DarkOrbit gaps:

- No ammo types, laser ammo, rocket/launcher/hellstorm/rocket CPU depth.
- No drones/formations changing combat geometry/stats.
- No P.E.T. combat/auto-loot/kamikaze support.
- No boss/wave gate combat depth beyond seeded NPCs.
- No faction/honor/reputation consequences for PvP.

Estimated completion:

| Scope | Completion |
| --- | ---: |
| Local fight/loot/repair MVP | **72%** |
| Production-safe death/economy integration | **45%** |
| DarkOrbit combat depth | **30%** |

### 4.4 Economy, wallet, inventory, cargo — **domain-rich but persistence-limited**

Evidence:

- `internal/game/economy/*`
- `internal/game/market/*`
- `internal/game/auction/*`
- `internal/game/premium/*`
- `internal/game/server/economy_handlers.go`
- `internal/game/server/shop_handlers.go`
- `client/src/state/reducer-economy.ts`
- `client/src/ui/hud-render-economy.ts`
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/plans/ui-implementation/08-market-auction-premium-economy.md`

Delivered:

- Wallet service, ledger concepts, reservation service.
- Inventory/cargo snapshots.
- Shop catalog and buy product operation.
- Market search/create/buy/cancel.
- Auction search/bid/buy-now/grants.
- Premium entitlement and weekly X Core purchase.
- Admin economy dashboard.
- Browser economy panels and no forged totals.

Open/gaps:

- `inventory.move` remains open in Phase 06.
- Most services are in-memory and not DB-transactional.
- Market/auction/premium fanout to passive viewers/sellers/bidders is open in `docs/todo.md`.
- Durable settlement boundaries for market/auction/premium are open.
- Normal player ledger/history UI is not a gameplay feature yet.

DarkOrbit gaps:

- DarkOrbit-like auction/store/assembly exists only as a skeleton; there is no mature scarcity/pricing cycle.
- No premium monetization policy hardening beyond skeleton entitlement logic.
- No credit sink/source balancing analysis.
- No shop/auction-driven ship ladder comparable to DarkOrbit’s long gear curve.

Estimated completion:

| Scope | Completion |
| --- | ---: |
| MVP economy commands | **65%** |
| Durable production economy | **30%** |
| DarkOrbit economy depth | **35%** |

### 4.5 Progression, skills, ships, modules, stats — **partially connected**

Evidence:

- `internal/game/progression/*`
- `internal/game/ships/*`
- `internal/game/modules/*`
- `internal/game/stats/*`
- `internal/game/server/progression_inventory_handlers.go`
- `client/src/ui/hud-render-inventory.ts`
- `docs/plans/ui-implementation/06-progression-inventory-loadout-crafting.md`
- `docs/plans/ui-patch-3/04-inventory-loadout-drag-drop.md`
- `docs/plans/ui-patch-3/05-hangar-ship-management.md`

Delivered:

- Progression/rank/role/skill tree domain packages exist.
- Stats aggregation package exists.
- Ship catalog/hangar state exists.
- Module catalog/loadout/equip/unequip exists.
- Browser hangar and inventory/loadout surfaces exist.
- Crafting contributes progression snapshots/events.

Open/gaps:

- `progression.unlock_skill` authenticated command is open.
- Skill tree/progression panel and unlock action are open.
- Inventory move is open.
- Module grid/filtering and browser equip/unequip smoke gaps remain in UI Patch 3.
- Effective stat/cargo recalculation after equip/ship changes remains a critical design gap from earlier audits.
- Non-starter ship acquisition is still weak/locked/MVP-only.

DarkOrbit gaps:

- No drone inventory/equipment/levels/formations.
- No P.E.T. equipment/levels/gears.
- No two-config ship equipment model.
- No ammo/rocket/CPU/extras inventory depth.
- No item upgrade levels equivalent to DarkOrbit upgrade charts.

Estimated completion:

| Scope | Completion |
| --- | ---: |
| Backend progression/equipment primitives | **65%** |
| Browser progression/equipment workflow | **55%** |
| DarkOrbit gear-depth parity | **20%** |

### 4.6 Crafting, CMS content, and balancing — **strong direction, still in progress**

Evidence:

- `internal/game/content/*`
- `internal/game/contentdb/*`
- `internal/game/contentseed/*`
- `internal/game/admin/content_service.go`
- `docs/plans/cms-rework/*`
- `docs/plans/2026-06-24-content-foundation*.md`
- `client/src/state/reducer-content-admin.ts`
- `client/src/ui/hud-render-admin-content.ts`

Delivered:

- `GameplayContent` bundle and validation.
- PostgreSQL `contentdb` foundation.
- Draft rows and immutable published snapshots.
- Seed source and published bootstrap.
- Runtime can load published DB content or explicit static fallback.
- Items/modules/ships/shop/NPC/enemy pools/loot/crafting/production definitions are DB-backed for runtime after restart.
- Admin content list/get/update/validate/publish/rollback/audit versions operations exist.
- Admin CMS UI slices exist.

Open/gaps from CMS docs:

- Diff API/view remains open.
- Audit `action` field/migration and broader scrubber policy remain open.
- Live Postgres duplicate/concurrent publish coverage remains open.
- Explicit rate-limit zero-mutation coverage remains open.
- Admin-only publish event fanout remains open.
- Quest rows must be included in admin publish/rollback before full quest CMS coverage.
- Weighted/multiple reward-table selection is deferred.
- Hot reload is deferred; MVP is restart-based.

Estimated completion:

| Scope | Completion |
| --- | ---: |
| CMS technical foundation | **75%** |
| CMS admin workflow completeness | **60%** |
| Runtime hot/live balancing | **25%** |

### 4.7 Discovery, planet ownership, production, routes — **best unique feature area**

Evidence:

- `internal/game/discovery/*`
- `internal/game/production/*`
- `internal/game/server/discovery_production_handlers.go`
- `internal/game/server/planet_*_handlers.go`
- `internal/game/server/route_*_handlers.go`
- `client/src/state/reducer-discovery.ts`
- `client/src/state/reducer-routes.test.ts`
- `docs/plans/ui-implementation/07-discovery-planets-production-routes.md`
- `docs/2026-06-17-world-system-design.md`

Delivered:

- Server-owned scan pulses.
- Bounded map-scoped candidates.
- Known planet intel and safe planet detail.
- Planet claim with X Core consumption and idempotency evidence.
- Coordinate item create/use and intel share controls.
- Production summary/storage summary.
- Building build/upgrade handlers.
- Route create/update/enable/disable/settle/list/snapshot.
- Durable-plan style contracts and process-local outbox/recovery scaffolding.
- Browser claim/build/route proof.

Open/gaps:

- Durable DB rows, cross-process locks, scheduled publisher workers remain open for many flows.
- Several checklist items explicitly say durable settlement is not DB-enforced yet.
- Broader per-map scanner/claim/drop seed matrix remains open.
- Wormhole network, alliance/shared infrastructure, outposts, contested structures are not there yet.

DarkOrbit comparison:

This is not a DarkOrbit clone feature; it is closer to OGame/Travian/Dark Forest. It is the strongest reason to keep the project’s identity distinct. DarkOrbit’s nearest equivalent is Skylab/CBS/resource sectors, but your planet network can become deeper than DarkOrbit if made durable/social.

Estimated completion:

| Scope | Completion |
| --- | ---: |
| Local MVP planet/route loop | **78%** |
| Durable persistent strategy layer | **45%** |
| Unique long-term differentiator | **55%** |

### 4.8 Quests, admin, observability, release gates — **good MVP, content shallow**

Evidence:

- `internal/game/quests/*`
- `internal/game/admin/*`
- `internal/game/observability/*`
- `internal/game/server/quest_admin_observability_handlers.go`
- `client/src/ui/hud-render-quests.ts`
- `docs/plans/ui-implementation/09-quests-admin-observability-release.md`

Delivered:

- Quest board/accept/progress/claim/reroll.
- Quest progress from server events.
- Reward claim idempotency.
- Admin inspect/repair/economy/content operations.
- Metrics/command log/release gate reports.
- Client surfaces for quests/admin/release gate.

Open/gaps:

- Quest content depth is shallow.
- Daily/challenge/event/epic mission variants are not rich.
- Quest CMS publish coverage still incomplete.
- Admin/observability is in-process; no Prometheus/Grafana/OpenTelemetry stack.

Estimated completion:

| Scope | Completion |
| --- | ---: |
| MVP quest/admin UI | **75%** |
| DarkOrbit mission-system parity | **35%** |
| Production observability | **35%** |

---

## 5. DarkOrbit Feature Gap Matrix

Legend:

- **Done**: usable in current MVP.
- **Partial**: exists but shallow/in-memory/not fully wired.
- **Missing**: not implemented as gameplay.
- **Not recommended now**: skip or redesign for your game.

| Feature | DarkOrbit role | Current game status | Completion | Recommendation |
| --- | --- | --- | ---: | --- |
| Browser map combat | Core real-time loop | Partial/strong MVP | **70%** | Continue; harden tick/net/persistence. |
| Server-auth movement | Anti-cheat foundation | Done for MVP | **80%** | Keep as core strength. |
| Multi-map portals | World structure | Partial | **60%** | Add more map content and rollout gates. |
| Companies/factions | Identity/PvP framing | Missing | **0%** | Add later or replace with player corps/clans. |
| Clans | Social retention | Missing | **0%** | High priority after persistence. |
| Group/party | Coop combat/gates | Missing | **0%** | Needed before wave gates/bosses. |
| Chat | MMO social minimum | Missing | **0%** | High priority; must include moderation/rate limits. |
| Mail/friends | Retention/social UI | Missing/locked | **5%** | Add after chat/clan foundation. |
| NPC farming | Core PvE income | Partial | **55%** | Add enemy variety, bosses, rewards. |
| Bosses/world bosses | Group PvE goal | Missing/very thin | **10%** | Add one public boss with contribution rewards. |
| PvP maps | Risk/reward | Partial | **40%** | Add reward/rank/honor consequences. |
| Safe zones/protection | Newbie/respawn protection | Partial | **60%** | Keep tightening edge cases. |
| Death/repair | Loss/recovery loop | Partial | **55%** | Make durable and UX-complete. |
| Cargo drop | Risk/economy | Partial | **45%** | Finish death/cargo provider ownership. |
| Hangar | Ship ownership UI | Partial | **60%** | Finish real ship ladder/acquisition. |
| Equipment drag/drop | Build workflow | Partial | **55%** | Finish inventory grid/filter/smokes. |
| Two equipment configs | DarkOrbit build depth | Missing | **0%** | Add after stats are correct. |
| Drones | Major gear progression | Missing | **0%** | Add after ship/module economy is stable. |
| Drone formations | Build specialization | Missing | **0%** | Add with drones, not before. |
| P.E.T. companion | Auto-loot/resource/support | Missing | **0%** | Good later feature; beware bot-like automation. |
| Ammo/rockets/mines | Combat consumable depth | Missing/thin | **10%** | Add ammo/rockets before drones/P.E.T. |
| Pilot Bio/skill tree | Long-term passive builds | Partial backend | **35%** | Finish `progression.unlock_skill` and UI. |
| Levels/ranks/roles | Progression backbone | Partial | **55%** | Good foundation; add visible milestones. |
| Honor/fame/leaderboards | PvP/social prestige | Missing | **0%** | Add after PvP consequences. |
| Missions | Guided progression | Partial | **45%** | Add daily/challenge/epic categories. |
| Galaxy Gates | Endgame instanced waves | Missing | **0%** | High-value DarkOrbit feature to add. |
| Invasion/group gates | Coop wave content | Missing | **0%** | Requires party/group first. |
| Skylab/refining | Offline resource production | Partial/different | **45%** | Reframe as planet industry, not clone. |
| Bonus boxes/ore fields | Short-session gathering | Missing/thin | **15%** | Add resource nodes/boxes for active play. |
| Auction | Economy sink/trade | Partial | **45%** | Make durable and more meaningful. |
| Assembly/crafting | Item creation path | Partial | **55%** | Good; finish recipe/content coverage. |
| Shop | Store/catalog | Partial | **55%** | Unlock real ship/equipment purchase path. |
| Premium currency | Monetization/resource | Partial/skeleton | **35%** | Define ethical/no-pay-to-win policy early. |
| Clan Battle Station | Territory endgame | Missing | **0%** | Consider as later planet/outpost warfare. |
| Events/live ops | Retention | Mostly missing | **10%** | CMS can become foundation for this. |
| Anti-bot/rate-limit | DarkOrbit lesson | Weak | **20%** | Must fix before public playtest. |

---

## 6. Promised in Docs but Not Yet Fully Delivered

### 6.1 Architecture promises

| Promise/design intent | Source docs | Current delivery | Gap |
| --- | --- | --- | --- |
| PostgreSQL for account, inventory, economy, market, progression, world data | Architecture notes | PostgreSQL exists mainly for CMS content. Most player/game state remains in-memory. | Huge production gap. |
| Redis for session/presence/cache/locks | Architecture notes | Not implemented as core runtime dependency. | Missing. |
| NATS / JetStream for service messaging/jobs/events | Architecture notes | Not implemented. | Missing. |
| Actor-style map workers behind gateway/router | Architecture/map docs | Worker package exists, multi-map exists, but runtime still centralizes many operations and locks. | Partial. |
| Binary protocol later | Architecture notes | JSON protocol only. | OK for now; not a blocker until scale. |
| External observability stack | Architecture notes | Internal metrics/log/release-gate domain exists. | Missing Prometheus/Grafana/OTel integration. |

### 6.2 Progression/equipment promises

| Promise | Current state | Needed |
| --- | --- | --- |
| Main level + rank + role levels + pilot passive skill tree | Domain models/snapshots exist. Public unlock/respec flow is incomplete. | `progression.unlock_skill`, skill tree UI, respec, tests. |
| Role XP: Combat/Scout/Crafting/Construction/Trade | Role concepts exist but not fully surfaced as balanced gameplay. | Role-specific XP sources/sinks/rewards. |
| Ships via craft/market/auction, not direct trade | Ship catalog/hangar/shop skeleton exists. Non-starter acquisition is not a real loop. | Decide first non-starter ship path and ship unlock events. |
| Modules meaningfully change stats | Module equip works, stats package exists. Effective runtime stat/cargo recalculation has gaps. | Make `StatService` authoritative for active runtime values. |
| Cargo death drop 50%-100% | Death/cargo drop exists in tests/MVP. Provider ownership/durable integration incomplete. | Durable death transaction and cargo provider integration. |
| Craft XP diminishing returns/daily soft caps | Observation hook exists; reduction/tuning is not active. | Implement balancing policy. |

### 6.3 World/discovery promises

| Promise | Current state | Needed |
| --- | --- | --- |
| Server-only procedural seeds never sent to client | Strongly tested in several places. | Broader leak canaries for production logs/admin paths. |
| Bounded map catalog/profile rollout | Implemented MVP. | More maps, rollout controls, DB-backed map/content policy. |
| Shared global planets + personal intel | Implemented MVP. | Durable DB rows and social/clan sharing. |
| Wormhole network/outposts/frontier events | Not implemented. | Add after route/planet persistence. |
| Intel/coordinate trading economy | Coordinate items/share exists. | Durable market/coordinate item transaction and UI depth. |

### 6.4 Economy/persistence promises

| Promise | Current state | Needed |
| --- | --- | --- |
| Every value mutation through service/ledger | Many services follow this pattern in-memory. | DB transactions + unique reference keys. |
| Broadcast after commit | Modeled in docs; many flows are process-local. | Durable outbox and recovery workers. |
| Idempotent market/auction/premium/webhook | Domain idempotency exists. | Real durable provider/webhook ingest, chargeback/revoke. |
| Passive fanout to affected market/auction viewers | Open in `docs/todo.md`. | Owner-aware event routing/fanout. |

### 6.5 UI promises

| Promise | Current state | Needed |
| --- | --- | --- |
| No fake real-mode gameplay data | Strongly delivered. | Keep regression tests. |
| Every implemented backend feature exposed through real UI | Mostly delivered for MVP slices. | Open items: skill unlock, inventory move, deeper CMS, social/mail. |
| Mockup parity | Completed for current HUD composition. | Visual polish/final asset pass still needed. |
| Desktop/tablet/mobile screenshots | Exists per docs. | Maintain as UI changes. |

### 6.6 CMS promises

| Promise | Current state | Needed |
| --- | --- | --- |
| Admin editable DB-backed content | Implemented for many content groups. | Finish all groups and quest publish coverage. |
| Publish/rollback/audit | Exists. | Diff API/view, audit action migration, concurrency coverage. |
| Runtime uses DB published content | Implemented after restart. | Hot reload deferred. |
| Safe projections | Exists. | Broader leak tests and player-facing catalog clarity. |

---

## 7. What Is Missing vs DarkOrbit: Priority Order

### P0 — Must fix before public test with real users

These are not “nice features”; they are necessary to avoid losing data, breaking economy, or inviting abuse.

| Item | Why it matters | Suggested milestone |
| --- | --- | --- |
| Durable player/account/economy/world persistence | Public users cannot lose accounts/wallet/progression on restart. | Persistence foundation milestone. |
| Durable idempotency/outbox for value mutations | Prevent dupes/lost rewards/market settlement bugs. | Persistence foundation milestone. |
| Real rate limiting/backpressure | DarkOrbit-like games attract bot/spam behavior. Metadata posture is not enough. | Security hardening milestone. |
| Async WebSocket write queues | Slow clients should not stall simulation/fanout. | Realtime hardening milestone. |
| Stat/cargo recalculation correctness | Equipment must actually matter; otherwise progression feels fake. | Equipment correctness milestone. |
| Inventory move + skill unlock | These are explicitly planned and player-visible holes. | Progression/equipment milestone. |
| First non-starter ship acquisition | DarkOrbit-like progression needs a visible next ship. | Progression/equipment milestone. |
| Chat moderation/rate limits | MMO without chat feels dead; chat without abuse controls is dangerous. | Social MVP milestone. |

### P1 — Most valuable DarkOrbit-like additions

| Feature | Why add it | Minimal version |
| --- | --- | --- |
| Galaxy Gate-like wave instance | Gives repeatable PvE endgame, material sink, and reward chase. | Solo training gate: build from parts, 5 waves, one boss, deterministic rewards. |
| Drones-lite | Adds equipment depth and iconic DarkOrbit feel. | 2-4 drone slots, equip laser/shield modules, simple levels, no formations first. |
| P.E.T.-lite | Adds automation fantasy but dangerous for bot feel. | Server-owned companion with one gear: auto-loot within radar, fuel/upkeep, strict rate limits. |
| Daily missions/challenges | Short-session retention. | Daily kill/scan/route/market tasks with server-owned progress. |
| Resource nodes/bonus boxes | Makes empty space feel playable between fights. | Visible AOI resource boxes with server-owned spawn/pickup/cooldowns. |
| Public ranking/honor | Adds status and PvP motivation. | Weekly kill/NPC/scan/route contribution leaderboard. |

### P2 — Endgame/social expansion

| Feature | Why add it | Minimal version |
| --- | --- | --- |
| Clans | Social retention foundation. | Clan create/join/leave, ranks, clan chat. |
| Clan stations / planet outposts | Your version of CBS should use planet/outpost architecture. | Clan-owned relay/outpost on contested planet, timed vulnerability window. |
| Factions/companies | DarkOrbit identity and safe/enemy rules. | Pick one company at account/player creation; simple company chat/rank. |
| Party/group | Needed for group gates/bosses. | Invite/accept, shared target and contribution reward. |
| Mail/friends | Retention/social utility. | Friend list + direct messages/mail with rate limits. |
| Event maps/live ops | Makes CMS valuable. | Time-limited map profile with event NPC pool/rewards. |

---

## 8. Must-Steal Features From Similar Games

### 8.1 From DarkOrbit — steal, but adapt

| Feature | Steal? | Adaptation for this game |
| --- | --- | --- |
| Galaxy Gates | **Yes** | Use as “Signal Gates” discovered by scanner or built from fragments. Keep rewards useful but not pay-to-win. |
| Drones/formations | **Yes, later** | Use “satellite drones” attached to ship loadout. Formations become tactical stance with cooldown. |
| P.E.T. | **Yes, carefully** | Make companion server-owned; avoid turning gameplay into auto-bot. Start with auto-loot only. |
| Skylab | **Partly** | Do not clone. Your planet production network already fills this role better. Add refinery/transport UX. |
| Clan Battle Station | **Yes, later** | Reimagine as clan outposts/wormhole relays/planet defenses. Use vulnerability windows. |
| Auction | **Yes** | Already started. Needs durable escrow and real item scarcity. |
| Pilot Bio | **Yes** | Already planned. Finish skill unlock UI. |
| Quickslot/action bar | **Yes** | Already started. Add ammo/consumable slots only after server-owned ammo exists. |

### 8.2 From EVE Online — steal systems, not complexity

EVE’s strongest lesson is not “make everything complex.” It is that **player organizations, markets, industry, and territorial stakes create stories**. For your game:

| EVE idea | Recommended adaptation |
| --- | --- |
| Corporations/alliances | Start with simple clans, then alliances after territory exists. |
| Player-driven economy | Keep NPC/shop bootstrap, but let player market own mid/late equipment flow. |
| Industry/logistics | Your planet routes can become a lightweight logistics system. Make route risk/visibility matter. |
| War eligibility | Use structure ownership to opt clans into wars rather than always-on griefing. |
| Corp projects | Clan quests: “scan 10 signals,” “deliver 500 alloy,” “destroy 20 raiders.” |

### 8.3 From Dark Forest — protect hidden information as core gameplay

Your docs already have this instinct. Keep it.

| Dark Forest idea | Recommended adaptation |
| --- | --- |
| Incomplete information | Never send hidden planets/future spawns/seeds. You already do this; expand leak tests. |
| Exploration as strategy | Make scanner choices meaningful: wide/weak vs narrow/deep scans. |
| Intel trading | You already have coordinate items. Make them durable and market-integrated. |
| Player memory | Persist known intel and stale freshness. Display confidence/freshness in UI. |

### 8.4 From modern DarkOrbit alternatives / browser MMOs

| Idea | Why it matters |
| --- | --- |
| Faster early progression | DarkOrbit’s old grind/pay curve is a known pain point. Your early loop should reach first ship/module/planet quickly. |
| Bot-resistant daily goals | Replace “grind boxes forever” with capped, varied, server-validated tasks. |
| No pay-to-win promise | If premium exists, define paid/free premium split now and avoid selling direct combat dominance. |
| Low-friction browser performance | Keep bundle size/leak canaries and built-client proof. This is already a strength. |

---

## 9. Completion Dashboard by System

| System | Local MVP | Production readiness | DarkOrbit parity | Notes |
| --- | ---: | ---: | ---: | --- |
| Auth/session | 90% | 55% | 70% | Works locally; durable/session hardening needed. |
| Realtime protocol | 80% | 45% | 60% | Strong envelope/authority; write queues/replay/rate limits missing. |
| World/movement/AOI | 80% | 40% | 55% | Good bounded-map MVP. |
| Portals/multi-map | 70% | 35% | 50% | More maps/rollout needed. |
| PvE combat | 65% | 45% | 35% | Needs enemy/content variety, ammo, bosses. |
| PvP/death/repair | 55% | 35% | 35% | Good proof, not full reward/ranking loop. |
| Loot/cargo | 60% | 35% | 40% | Loot pickup works; durable XP/outbox and field resources missing. |
| Economy/wallet/ledger | 65% | 30% | 40% | Rich in-memory domain; needs DB transactions. |
| Inventory/loadout | 55% | 35% | 30% | Equip works; inventory move/stat correctness open. |
| Ships/hangar | 55% | 35% | 35% | Hangar UI exists; ship acquisition ladder weak. |
| Stats/modules | 50% | 30% | 25% | Aggregation exists; runtime effect gaps. |
| Progression/skills | 45% | 30% | 35% | Snapshot exists; skill unlock/UI missing. |
| Crafting | 65% | 40% | 45% | Start/complete/cancel exists; content/version hardening needed. |
| Scanner/discovery | 75% | 45% | 50% | Strong unique feature; durable stores/matrix missing. |
| Planet claim/production/routes | 78% | 45% | 45% | Strong unique feature; durable strategy layer missing. |
| Shop/market/auction | 60% | 30% | 45% | Commands exist; durable escrow/passive fanout missing. |
| Premium | 45% | 25% | 35% | Skeleton exists; provider/fraud/revoke missing. |
| Quests | 70% | 45% | 45% | Board works; missions/dailies/events thin. |
| CMS | 70% | 55% | N/A | Strong content foundation; quest/diff/hot reload open. |
| Admin/observability | 65% | 35% | N/A | In-game admin exists; external ops stack missing. |
| Chat/social/clans | 5% | 0% | 0% | Major missing MMO layer. |
| Drones/P.E.T. | 0% | 0% | 0% | Major DarkOrbit parity gap. |
| Galaxy Gates/endgame | 0% | 0% | 0% | Biggest DarkOrbit loop missing. |
| Live events | 10% | 10% | 10% | CMS can enable later. |
| Anti-cheat/abuse | 50% | 25% | 35% | Authority good; bot/rate-limit not enough. |

### Aggregate estimates

| Category | Completion |
| --- | ---: |
| Core local playable game | **70%** |
| Core local + browser UI | **72%** |
| Internal design-doc feature promise | **60-65%** |
| DarkOrbit-like full feature parity | **35-40%** |
| Production-grade live service | **25-35%** |

---

## 10. “Done / Not Done / Should Do” by Player Loop

### 10.1 New player loop

| Step | Current state | Completion | Needed |
| --- | --- | ---: | --- |
| Register/login | Works | 90% | Durable auth/session store. |
| Spawn starter ship | Works | 85% | Align starter seed/catalog naming if still divergent. |
| Understand HUD | Mostly works | 75% | Better tutorial/onboarding. |
| Move and select target | Works | 80% | Polish selection/target feedback. |
| Kill first NPC | Works | 75% | More starter enemy variety. |
| Pick loot | Works | 70% | More drop types + resource nodes. |
| Buy/equip upgrade | Partial | 45% | Real first upgrade path with stat refresh. |
| Claim first planet | Works in proof | 70% | Durable ownership + clearer UX. |
| Start route/production | Works in proof | 70% | Better explainers + durable settlement. |

### 10.2 Mid-game progression loop

| Step | Current state | Completion | Needed |
| --- | --- | ---: | --- |
| Unlock skills | Backend partial, UI/action missing | 25% | Skill tree command/UI. |
| Upgrade ship | Partial/locked | 25% | First real non-starter ship path. |
| Build specialized loadout | Partial | 35% | Effective stat recalculation, module variety. |
| Farm higher maps | Partial | 40% | More map/enemy/progression tiers. |
| Use market/auction | Partial | 45% | Durable trades, meaningful supply/demand. |
| Craft better modules | Partial | 45% | More recipes and material sources. |
| Expand planet network | Partial | 55% | More planet/building/route strategy. |

### 10.3 Endgame/social loop

| Step | Current state | Completion | Needed |
| --- | --- | ---: | --- |
| Join clan | Missing | 0% | Clan system. |
| Group PvE | Missing | 0% | Party + wave/boss gates. |
| Territory war | Missing | 0% | Clan outposts/planet war/CBS equivalent. |
| Ranked PvP/honor | Missing | 0% | PvP score, weekly ranking, anti-farm rules. |
| Galaxy gate farming | Missing | 0% | Instanced wave gates/fragments. |
| Live events | Mostly missing | 10% | CMS-powered event profiles. |

---

## 11. Specific Missing Features List

### 11.1 DarkOrbit-critical missing features

1. Factions/companies.
2. Clans.
3. Chat.
4. Party/group.
5. Mail/friends/social notifications.
6. Drones.
7. Drone levels/designs/formations.
8. P.E.T. companion.
9. P.E.T. gear/protocols/fuel/upkeep.
10. Galaxy Gates / instanced wave maps.
11. Group gates / invasion gates.
12. Ammo types.
13. Rockets/rocket launchers/mines/CPUs/extras.
14. Bonus boxes/resource fields.
15. Refining/resource upgrade path like Skylab.
16. Pilot skill unlock UI and respec.
17. Honor/fame/company ranking.
18. Clan Battle Station equivalent.
19. World bosses/contribution rewards.
20. Seasonal/event missions.

### 11.2 Your own design-doc missing features

1. Durable PostgreSQL player state.
2. Redis presence/cache/locks.
3. NATS/JetStream event/outbox infrastructure.
4. Real operation rate limits.
5. Multi-process zone/map ownership.
6. Cross-zone handoff state machine beyond in-process map transfer.
7. `inventory.move` public mutation.
8. `progression.unlock_skill` public mutation.
9. Skill tree UI.
10. Effective stat refresh after loadout/ship changes.
11. Cargo capacity provider fully backed by effective stats.
12. First non-starter ship acquisition and verification.
13. Saved loadouts/configurations.
14. Wormholes/frontier expansion/outposts.
15. Clan/alliance planet/network sharing.
16. Intel freshness/decay/trading depth.
17. Durable market/auction/premium provider integration.
18. Live ops event layers.
19. Full CMS diff/audit/concurrency/rate-limit coverage.
20. Production observability stack.

---

## 12. Suggested Fix / Build Plan

### Phase A — Persistence & correctness hardening

Goal: stop pretending in-memory state can survive public players.

Deliverables:

1. Durable account/player/session persistence or explicit decision that auth stays dev-only until DB lands.
2. Durable wallet/inventory/ledger minimal schema.
3. Durable request/domain idempotency table.
4. Durable outbox table and replay worker for value events.
5. First DB-backed player progression row.
6. First DB-backed ship/hangar/loadout row.
7. `go test ./...`, race-focused tests, and restart-recovery tests.

Completion unlocked:

- Production readiness moves from ~30% to ~45%.
- Public playtest reset risk becomes manageable.

### Phase B — Equipment/progression closure

Goal: make upgrades actually matter.

Deliverables:

1. `inventory.move`.
2. `progression.unlock_skill`.
3. Skill tree UI.
4. Effective `StatService` integration into runtime.
5. Cargo capacity recalculation after module equip/unequip and ship activation.
6. Real first non-starter ship acquisition via shop/craft/quest.
7. Browser proof: buy/craft/equip module -> stats/cargo change -> fight/loot capacity reflects it.

Completion unlocked:

- DarkOrbit parity moves from ~38% to ~45%.
- Player progression stops feeling cosmetic.

### Phase C — Social MVP

Goal: make it feel like an MMO, not a solo tech demo.

Deliverables:

1. Server-authenticated chat with channel types: system, local map, clan later.
2. Chat rate limits, moderation hooks, redaction/logging policy.
3. Party/group invite/accept/leave.
4. Group target/contribution event foundation.
5. Clan foundation: create/join/leave/ranks/clan tag.

Completion unlocked:

- Retention and DarkOrbit MMO feel improve massively.

### Phase D — First endgame loop

Pick one:

#### Option 1: Signal Gate / Galaxy Gate MVP

- Gate fragments from NPC/quest/scan.
- Build one gate.
- Enter private/group wave map.
- 5 waves + boss.
- Rewards: credits, materials, one module/drone fragment.
- Lives/repair policy server-owned.

#### Option 2: Clan Outpost / CBS-like MVP

- Clan claims contested planet/outpost slot.
- Build hull/deflector/turret modules.
- Vulnerability window.
- Attack/defense contribution rewards.
- Clan-wide small booster.

Recommendation: **Signal Gate first**. It requires less social/territory complexity and gives repeatable content. Build Clan Outpost after clans and persistence are stable.

### Phase E — Drones/P.E.T. and deeper DarkOrbit equipment

Add only after stats/equipment are correct:

1. Drone inventory rows.
2. Drone slots and equip rules.
3. Drone XP/level.
4. Simple formations.
5. Companion/P.E.T. entity as server-owned helper.
6. Auto-loot gear with strict range/fuel/cooldown.
7. Anti-bot posture.

---

## 13. Risks If You Add CMS Before Persistence

The active CMS work is useful, but there is a product risk: **balancing content before durable player state can create false confidence**.

CMS lets you edit:

- items;
- modules;
- ships;
- shop;
- NPCs;
- loot;
- crafting;
- production;
- quests.

But if player wallets/inventory/progression/market/planet state are still mostly in-memory, then real balance cannot be measured across restarts or real users.

Recommended CMS sequencing:

1. Finish CMS publish/diff/audit/safe projection enough for developer balancing.
2. Do not over-invest in rich CMS UI until durable player/economy state lands.
3. Add balance telemetry around sources/sinks before increasing content volume.
4. Every CMS field that affects economy must have a simulator or release gate.

---

## 14. Anti-Cheat / Abuse Missing Features

DarkOrbit-like games are bot magnets. Your server-authoritative posture is good, but not sufficient.

Missing/weak areas:

| Area | Current | Needed |
| --- | --- | --- |
| Rate limits | Metadata only | Enforced per account/session/IP/op buckets. |
| Login/register abuse | Basic posture | Slowdown, lockouts, IP/device heuristics. |
| Bot movement/combat | Server validates truth | Pattern detection, command cadence telemetry. |
| Market abuse | Domain checks exist | Durable anti-duplication, wash-trade detection. |
| Multi-account farming | Not addressed | Account/IP/session correlation and soft limits. |
| Chat abuse | Chat missing | Moderation from day one. |
| Client tampering | Good forbidden-field tests | Broader protocol fuzzing and replay mismatch checks. |
| Hidden data leaks | Good focused leak canaries | Expand to logs/admin/debug/CMS projections. |

Do not add auto-loot/P.E.T.-style automation until this is stronger.

---

## 15. Suggested Public Roadmap View

### Milestone 1 — “Persistent Pilot”

Completion target: production readiness 45%.

- DB-backed player/wallet/inventory/progression/hangar/loadout.
- Durable idempotency/outbox.
- Restart recovery tests.
- Real rate limits for auth/gameplay/economy.

### Milestone 2 — “Meaningful Upgrades”

Completion target: DarkOrbit parity 45%.

- Skill unlock UI.
- Inventory move.
- Effective stats/cargo after equipment.
- First non-starter ship path.
- First module upgrade/crafting path.

### Milestone 3 — “Alive MMO”

Completion target: social layer 35%.

- Chat.
- Party/group.
- Clan MVP.
- Local/map presence.
- Basic leaderboards.

### Milestone 4 — “First Endgame”

Completion target: retention loop 60%.

- Signal Gate/Galaxy Gate MVP.
- Daily/challenge missions.
- World boss/contribution reward.
- Event map/content profile through CMS.

### Milestone 5 — “DarkOrbit Flavor Pack”

Completion target: DarkOrbit parity 60%.

- Drones-lite.
- Drone formations-lite.
- P.E.T.-lite.
- Ammo/rocket/consumable depth.
- Clan outpost/CBS-like design.

---

## 16. Feature-by-Feature Current Status Table

| Feature | Status | % | Affected refs | Notes / next action |
| --- | --- | ---: | --- | --- |
| Auth register/login/session | Done MVP | 90 | `internal/game/auth`, UI Phase 01 | Durable store/rate limit next. |
| WebSocket session authority | Done MVP | 85 | `internal/game/realtime`, `internal/game/server/transport.go` | Add writer queues and replay policy. |
| No fake default client state | Done | 95 | `client/src/state`, UI Phase 03/10 | Keep tests. |
| Movement/AOI | Done MVP | 80 | `internal/game/world`, UI Phase 04 | Scale architecture open. |
| Portals | Partial | 70 | `portal.enter`, map-rework Phase 03 | More maps/rollout. |
| Safe/PvP policy | Partial | 60 | `policy_protection.go`, PvP tests | Add honor/ranking/faction rules. |
| NPC spawn/aggro/leash | Partial | 65 | `world/worker/enemy_*` | More NPC types/bosses. |
| Combat skill | Partial | 60 | `combat.use_skill` | Add ammo/rockets/skills. |
| Loot pickup | Partial | 70 | `loot.pickup` | Durable XP/outbox. |
| Death/repair | Partial | 55 | `death.*`, Phase 05 | Complete quote/debit tests and durable flow. |
| Shield repair tick | Done MVP | 75 | `repair.shield_tick` | Good DarkOrbit-style small feature. |
| Wallet snapshot | Done MVP | 70 | `wallet.snapshot` | DB persistence. |
| Inventory snapshot | Done MVP | 65 | `inventory.snapshot` | Add `inventory.move`. |
| Hangar activate | Partial | 60 | `hangar.activate_ship` | Real ship acquisition ladder. |
| Loadout equip/unequip | Partial | 60 | `loadout.*` | Stat recalculation and browser smoke. |
| Stats aggregation | Partial | 45 | `internal/game/stats` | Wire to runtime truth. |
| Skill tree | Partial backend | 30 | `internal/game/progression` | Public command/UI. |
| Crafting | Partial | 65 | `crafting.*` | Durable jobs/versioning. |
| Shop | Partial | 55 | `shop.*` | Unlock real products. |
| Market | Partial | 55 | `market.*` | Durable escrow/fanout. |
| Auction | Partial | 45 | `auction.*` | Real lot lifecycle/grants. |
| Premium | Partial | 35 | `premium.*` | Provider webhook/fraud/revoke. |
| Quests | Partial | 70 | `quest.*` | Daily/event/epic depth. |
| Scanner | Partial strong | 75 | `scan.pulse` | More scan modes/matrix. |
| Planet detail/claim | Partial strong | 75 | `discovery.*` | Durable DB/cross-process. |
| Production/buildings | Partial | 70 | `planet.*` | Durable rows/balancing. |
| Routes | Partial | 75 | `route.*` | Durable windows/workers. |
| Intel/coordinates | Partial | 60 | `intel.*` | Durable market/intel freshness. |
| CMS DB | Partial strong | 70 | `contentdb`, `admin.content.*` | Diff/audit/quest coverage. |
| Admin/observability | Partial | 65 | `admin.*`, `observability.*` | External metrics/logs. |
| Chat | Missing | 0 | none | Add social MVP. |
| Clans | Missing | 0 | none | Add after persistence. |
| Group/party | Missing | 0 | none | Needed for gates. |
| Drones | Missing | 0 | none | Add later. |
| P.E.T. | Missing | 0 | none | Add later carefully. |
| Galaxy Gates | Missing | 0 | none | Best first endgame loop. |
| Clan Battle Stations | Missing | 0 | none | Later clan/outpost warfare. |
| Factions/companies | Missing | 0 | none | Decide if clone or own identity. |
| Resource fields/bonus boxes | Missing/thin | 15 | loot/cargo only | Add active gathering content. |
| Live events | Missing/thin | 10 | CMS foundation | Add after CMS/persistence. |

---

## 17. Final Assessment

### What you have done well

- You built a real server-authoritative browser loop instead of a fake UI demo.
- You separated gameplay domain from Symphony/orchestration code.
- You documented abuse vectors and server ownership rules clearly.
- You made hidden-information/fog/scanner design a first-class concern.
- You added a lot of domain tests around duplicate/idempotent flows.
- You created a content/CMS direction that can support live balancing.
- You have a distinct long-term strategy hook with planets/routes, not just DarkOrbit nostalgia.

### What is most incomplete

- Durable player/game state.
- Social/clan/chat layer.
- DarkOrbit-style gear depth: drones, P.E.T., ammo, formations, gates.
- Real progression actions: skill unlock, inventory move, stat/cargo correctness.
- Endgame PvE/PvP loops.
- Production abuse/bot/rate-limit posture.

### Most important recommendation

Do **not** chase every DarkOrbit feature next. First make the current loop durable and correct:

```text
Persistence -> Equipment correctness -> Social MVP -> First endgame gate -> Drones/P.E.T./CBS
```

If you do that, the game can become a stronger modern DarkOrbit-like project instead of a shallow clone.

---

## 18. Reference Links Used

DarkOrbit / DarkOrbit-like references:

- Official DarkOrbit Reloaded Wiki: `https://darkorbit-archive.fandom.com/wiki/Dark_Orbit_Wiki`
- DarkOrbit Reloaded overview: `https://darkorbit-archive.fandom.com/wiki/DarkOrbit_Reloaded`
- Galaxy Gates: `https://darkorbit-archive.fandom.com/wiki/Galaxy_Gates`
- Hades Gate group gate: `https://darkorbit-archive.fandom.com/wiki/Hades_Gate`
- Invasion Gate: `https://darkorbit-archive.fandom.com/wiki/Invasion_Gate`
- Skylab: `https://darkorbit-archive.fandom.com/wiki/Skylab`
- Hangar: `https://darkorbit-archive.fandom.com/wiki/Hangar`
- Drones: `https://darkorbit-archive.fandom.com/wiki/Drone`
- P.E.T. 10: `https://darkorbit-archive.fandom.com/wiki/P.E.T._10`
- P.E.T. Gear: `https://darkorbit-archive.fandom.com/wiki/P.E.T_and_P.E.T._Gear`
- User Interface: `https://darkorbit-archive.fandom.com/wiki/User_interface`
- Missions: `https://darkorbit-archive.fandom.com/wiki/Missions`
- Clan Battle Station: `https://darkorbit-archive.fandom.com/wiki/Clan_Battle_Station`

Comparable-game references:

- EVE Online Gameplay & Features: `https://support.eveonline.com/hc/en-us/categories/200527101-Gameplay-Features`
- EVE Online corporations/alliances: `https://support.eveonline.com/hc/en-us/sections/201141672-Corporations-Alliances`
- EVE Online ISK and PLEX: `https://support.eveonline.com/hc/en-us/articles/14141550499612-ISK-and-PLEX`
- EVE Online official MMORPG overview: `https://www.eveonline.com/ko/p/mmorpg`
- Dark Forest announcement: `https://blog.zkga.me/announcing-darkforest`
- Dark Forest incomplete-information/fog reference: `https://dfpunk.xyz/`
- Space Aces Steam page: `https://store.steampowered.com/app/4220440`
- SpaceExpanse reference: `https://spaceexpanse.app/`
