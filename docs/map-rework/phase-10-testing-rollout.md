# Phase 10: Testing, Migration, And Rollout

## Goal

Verify and roll out the bounded multi-map rework without cross-map data leaks,
fake client state, or economy/combat regressions. This phase is the release gate
for map catalog, map workers, portal handoff, safe/PvP policy, radar/stealth,
bounded scanner/planet logic, per-map enemy pools, map-aware loot, and the
client map UI/protocol.

## Current State To Replace/Reuse

Replace or expand:

- `docs/map-rework/00-index.md:23-50` records canonical rework decisions:
  bounded `0..10000` maps, local radar visibility, no fog-of-war wave,
  map-owned workers/content, portal validation, rare planet scanning, map-owned
  enemy pools, and data-oriented worker/spawner storage.
- `docs/map-rework/00-index.md:85-99` defines cross-phase invariants such as no
  cross-map leakage, broadcast after mutation, server-owned map membership,
  bounds enforcement, safe/PvP server enforcement, radar/stealth authority, and
  hidden seed exclusion.
- `docs/plans/ui-implementation/05-combat-loot-death-repair.md:77-100`
  documents the existing combat/loot/death event contract and recipient-safe
  payload expectations.
- `docs/plans/ui-implementation/05-combat-loot-death-repair.md:108-135`
  documents the current real fight/loot loop and notes that repair/death
  hardening still has gaps.
- `docs/plans/ui-implementation/10-final-mockup-parity-hardening.md:35-64`
  documents the current HUD state, real-data requirement, action rail, minimap,
  marker parity, and screenshot artifacts.
- `docs/plans/ui-implementation/10-final-mockup-parity-hardening.md:158-201`
  lists the existing backend/client/E2E verification matrix.
- `docs/plans/ui-implementation/10-final-mockup-parity-hardening.md:207-218`
  requires the browser runner to use the real Go server, mail/password auth,
  cookie session, `/ws`, real protocol, and no JavaScript fixtures.
- `docs/plans/ui-implementation/10-final-mockup-parity-hardening.md:309-318`
  records the release gate: `go test ./...`, client
  `npm --cache /tmp/gameproject-npm-cache run check`, and `git diff --check`.

Reuse:

- `client/src/protocol/envelope.ts:207-317` as the client-side forbidden
  payload scan gate for hidden seeds, future spawns, scan candidates, loot rolls,
  loot tables, trusted ids, and secrets.
- `client/src/state/reducer.ts:3286-3331` as the gameplay clearing model for
  auth/logout/session failure and as the pattern for map-change clearing.
- `internal/game/loot/service.go:187-227` and
  `internal/game/loot/service.go:608-614` for duplicate-safe drop creation from
  one NPC death source.
- `internal/game/loot/service.go:337-345` for ledger/idempotency on loot pickup.
- `docs/2026-06-17-progression-economy-systems-design.md:2785-2808` for the
  rule that the client cannot decide damage, hit/miss, loot drop, pickup
  validity, XP, production, ship death, repair cost, or planet ownership.
- `docs/2026-06-17-progression-economy-systems-design.md:3018-3036` for the
  loot drop persistence fields that already carry world/zone, position,
  item/quantity, owner lock, source type/id, and timestamps.

## Target Model

Rollout proceeds behind deterministic map seeds and explicit feature gates until
the real server browser smoke covers the full map loop:

```text
register/login
restore authenticated session
spawn in starter map
receive map snapshot with bounds, policy, portals, minimap, and AOI
move within bounds
fight per-map NPCs from map enemy pools
create map/risk-aware drops
pick up visible owned/public drops
scan for rare planet signal in bounded map
portal to another map
verify old-map contacts disappear
verify safe-zone PvP is blocked
verify PvP-map policy allows eligible PvP later
reconnect and reconcile current map/member state
```

No rollout step may use client fixtures as proof of gameplay behavior. Demo and
fixture modes can remain only for explicit dev/test paths and must be excluded
from default authenticated play.

## Data Structures/Contracts To Add Or Change

Testing fixtures/seeds:

- Deterministic starter account/session seed path for real auth smoke.
- Deterministic map catalog with at least:
  - `map_id = starter_map`
  - `bounds = 0..10000`
  - one safe zone
  - one PvP-enabled map or PvP zone
  - two portals linking two maps
  - one enemy pool per map
  - one scanner profile with rare planet chance
  - one claimable planet candidate/profile for controlled tests
- Deterministic enemy pool seed with caps and respawn delays small enough for
  tests but clearly marked as test/dev tuning.
- Deterministic drop profile seed for `npc_type + map_id/risk/rank_band`.

Release gate reports:

- Add a map rework audit table with rows for:
  - map catalog/schema
  - map worker ownership
  - portal handoff
  - safe/PvP policy
  - radar/stealth visibility
  - bounded scanner/planet discovery
  - planet claim/production/routes
  - enemy pools/spawners
  - drop table selection
  - client map UI/protocol
  - no fake/default fixtures
- Each row records command/query/event names, owner package, positive test,
  abuse/negative test, browser artifact, and rollout status.

Runtime/observability contracts:

- Metrics for map worker count, players per map, AOI recipients per map,
  portal attempts/success/failure, cross-map rejection, safe-zone PvP rejection,
  spawn cap skips, respawns, NPC kills by map/pool/type, drop profile selected
  by map/risk/rank, scanner success/no-signal, and hidden payload rejection.
- Logs must include public map keys and operation ids but not passwords, session
  tokens, seeds, scan rolls, loot rolls, future spawn candidates, or hidden
  planet data.

Rollout controls:

- Feature flag for bounded multi-map runtime, disabled by default until the
  phase gate passes.
- Deterministic dev/test map seed separate from production tuning.
- Backfill plan for old world/zone/planet/route rows:
  - valid starter data maps to starter map id
  - out-of-bounds coordinates are quarantined for manual repair
  - no silent coordinate clamping
- Rollback plan:
  - stop new portal transfers
  - keep current player map state readable
  - restore starter-map spawn fallback
  - preserve ledger/inventory/claim/route mutations already committed
- Canary leak checks for WebSocket payloads, logs, DOM/app state, screenshots,
  local storage, cookies, and production bundle text.
- Debug/admin leak audit:
  - debug commands require explicit dev/admin mode
  - admin map targets resolve server-side
  - public responses omit hidden candidates, spawn pools, drop tables, seeds,
    roll results, and transfer internals

Starter and PvP rollout policy:

- Starter maps are PvP-disabled or protected by default.
- Graduation into PvP maps is controlled by rank, quest, portal unlock, or
  explicit player action.
- PvP maps must define cargo drop policy, repair/checkpoint policy, honor/XP or
  bounty hooks if enabled, anti-farming constraints, and kill credit rules.
- PvP rewards must not ship before abuse tests exist for repeated kills,
  alternate accounts, safe-zone baiting, portal camping, and disconnect abuse.

## Implementation Tasks In Order

1. Create a rollout checklist that maps each map-rework phase to concrete tests,
   browser smoke assertions, screenshots, and release gate evidence.
2. Add a bounded-map feature flag and rollout/rollback checklist before routing
   real sessions through the new runtime.
3. Add deterministic dev/test map catalog seed covering two maps, a bidirectional
   portal, a starter safe zone, a PvP zone/map, enemy pools, scanner profile,
   and planet scan/claim profile.
4. Add backend unit tests for map catalog validation and bounded coordinate
   clamping/rejection.
5. Add worker tests for map membership isolation, AOI same-map filtering,
   radar/stealth visibility, and no cross-map entity leakage.
6. Add portal handoff tests for proximity, cooldown, destination validation,
   spawn position, old worker removal, destination worker insertion, session
   stream scoping, reconnect recovery, and duplicate/retry safety.
7. Add safe-zone/PvP tests for attack blocking, death/cargo policy, NPC aggro
   reset, and portal arrival protection.
8. Add scanner tests for bounded map profiles, rarity, no fog-wave dependency,
   no hidden seed/candidate serialization, and no cross-map planet results.
9. Add planet claim/production/route tests for discovered-intel requirement,
   proximity, rank, X Core/inventory/ledger mutation, idempotency, storage cap,
   offline settlement, and route map endpoints.
10. Add enemy pool/spawner tests for caps, respawn delay, map ownership,
   aggro/leash, duplicate kill handling, and no safe-zone spawns unless
   explicitly allowed.
11. Add loot/drop tests for map-aware table selection, owner lock, visible
    pickup, hidden/far pickup rejection, cargo capacity, duplicate pickup, and
    quest/progression idempotency.
12. Add client reducer/protocol tests for map summary, bounds, portal events,
    map change clearing, stale old-map event rejection, minimap contacts, and
    forbidden payload keys.
13. Add real-server browser smoke that runs the full bounded map loop and writes
    desktop/tablet/mobile screenshots for authenticated map state.
14. Add leak scans over DOM text, app state, WebSocket payloads, storage, cookies,
    screenshots, and production bundle text for fake/default fixtures and hidden
    map/scan/spawn/loot internals.
15. Add admin/debug leak audit tests for map target resolution, dev-mode gates,
    redacted logs, and no hidden map/scan/spawn/drop internals in responses.
16. Run narrow packages during development, then run the full verification gate.
17. Update docs and rollout checklist only for behavior actually implemented and
    verified.

## Tests To Add/Update

Backend:

- Map catalog rejects invalid bounds, out-of-bounds portals/spawn areas/zones,
  duplicate ids, missing destination maps, and invalid risk policy.
- Movement beyond `0..10000` is rejected or corrected by the server; client
  clamping is not treated as authority.
- AOI snapshot includes only entities in current map and radar range.
- Cross-map movement, combat, loot pickup, scan result, portal, planet claim,
  production, route, market/admin debug, and minimap payloads do not leak hidden
  entities.
- Portal entry validates session, current map, proximity, cooldown,
  destination, spawn point, transfer state, and duplicate request behavior.
- Map-scoped events carry `map_subscription_epoch`, and stale old-epoch events
  are dropped or ignored.
- Safe-zone PvP is blocked without energy/cooldown/cargo/death mutation.
- Enemy spawner maintains per-map and per-pool caps under concurrent kills,
  duplicate death events, despawns, and respawns.
- NPC kill drop selection uses `npc_type + map_id/risk/rank_band`.
- Scanner rarity uses server-owned bounded map profile and does not serialize
  seed, roll, candidate, or hidden planet information.
- Planet claim requires discovered intel, same-map/proximity, rank, X Core,
  inventory/ledger mutation, and idempotency.

Client:

- Protocol parser accepts public map summaries, bounds, portals, safe/PvP flags,
  map minimap fields, and map transfer events.
- Protocol parser rejects forbidden hidden/trusted keys in map, portal, scan,
  combat, loot, and admin payloads.
- Reducer initializes map state empty and clears it on auth/logout/demo/session
  failure.
- Reducer clears old-map entities, selected target, loot, minimap contacts,
  transient effects, and movement target on map change.
- Reducer ignores stale old-map AOI/combat/loot/scan events after handoff.
- Renderer draws map bounds, radar contacts, portals, and safe/PvP hints only
  when server state provides them.
- UI never renders fake HP/shield/energy, cargo, wallet, quest counts, planets,
  NPCs, loot, market, portal, or premium data in default real mode.

Browser/E2E:

- Login and `/ws` use real mail/password auth and cookie session.
- Starter map snapshot shows `10000x10000` bounds, map name, policy, self, and
  only same-map AOI contacts.
- Fight/loot loop kills an NPC from the starter map pool and picks up a visible
  owned/public drop.
- Spawn cap test kills enough NPCs to prove caps and respawn delay behavior.
- Portal handoff removes old-map entities and shows destination map entities
  only after server completion.
- Safe-zone PvP attempt is rejected and does not mutate combat state.
- Starter maps are PvP-disabled or protected by default.
- PvP map smoke covers configured death/cargo/repair/checkpoint risk without
  shipping untested reward abuse vectors.
- Scanner smoke verifies no-signal and rare success paths under deterministic
  test config, without leaked roll/candidate data.
- Reconnect restores current map membership and reconciles map/AOI/minimap.
- Desktop, tablet, and mobile screenshots show bounded map HUD without overlap.

## Migration/Doc Updates

- Rewrite or supersede old infinite-world and fog-wave language in world,
  discovery, UI, and module docs after implementation lands.
- Update progression/economy docs so death cargo risk, loot drops, scanner XP,
  planet production, and route risk refer to bounded map/risk policy.
- Update running-local docs with deterministic map seed flags and unsafe dev
  defaults.
- Add migration docs for backfill, quarantine, feature flag rollout, rollback,
  and canary leak checks.
- Update `docs/todo.md` with any missing map loop contracts rather than faking
  UI data.
- Update release/audit docs with exact test names, screenshot paths, and known
  rollout limitations.

## Risks And Acceptance Criteria

Risks:

- Old infinite-plane assumptions can survive in tests that only cover one map.
- Portal handoff can leak stale AOI events if server stream ownership and client
  clearing are not both verified.
- Safe-zone policy can be shown in UI but missed in combat/death services.
- Scanner rarity can become impossible to test unless deterministic test config
  is separate from production tuning.
- Spawn and planet tests can pass in unit isolation but fail under real
  authenticated browser flow.
- Fixture/demo content can hide missing server contracts if included in default
  screenshots or bundle output.

Acceptance criteria:

- Every map-rework phase has positive, negative/abuse, and browser evidence.
- No cross-map leakage is demonstrated across snapshots, AOI events, minimap,
  combat, loot, scanner, portals, planet intel, reconnect, and admin/debug
  surfaces.
- Portal handoff, safe-zone PvP blocking, spawn caps, map-aware drops, scanner
  rarity, and reconnect reconciliation are covered by automated tests.
- Browser screenshots cover desktop, tablet, and mobile authenticated map UI.
- Default real mode contains no fake gameplay values or fixture labels.
- Full verification passes before handoff:

```bash
go test ./...
git diff --check
cd client
npm --cache /tmp/gameproject-npm-cache run check
```

- No code rollout is considered complete until failed/missing contracts are
  documented in `docs/todo.md` instead of being masked by client placeholders.
