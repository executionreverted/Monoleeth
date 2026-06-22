# Phase 10: Testing, Migration, And Rollout

## Goal

Verify and roll out the bounded multi-map rework without cross-map data leaks,
fake client state, or economy/combat regressions. This phase is the release
gate for map catalog/schema, runtime map membership, portal handoff,
safe/PvP policy, radar/stealth visibility, bounded scanner/planet logic,
planet claim/production/routes, per-map enemy pools, map-aware loot, and the
client map UI/protocol.

This document is an audit and rollout record only. It must not invent release
evidence. Rows marked `Covered` cite existing named audit/test evidence. Rows
marked `Partial` have some named coverage but still have audit gaps. Rows
marked `Open` are not implemented rollout controls yet.

## Phase10 Progress/Audit Table

| Area | Status | Owner package/files | Command/query/event names | Positive tests | Negative/abuse tests | Browser artifact | Rollout note |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Map catalog/schema | Partial | `internal/game/world/maps/catalog.go`, `internal/game/world/maps/router.go`, `docs/map-rework/phase-01-map-catalog-router.md` | `map.snapshot`, `world.snapshot`, catalog `ClientMapProjection` | `catalog/router`: `TestStarterCatalogReturnsBoundedStarterSpawnAndProjection`, `TestRouterEnsureStarterLocationCreatesOnceAndPreservesExisting`, `TestRouterValidatesActivePosition`, `TestCatalogValidationAcceptsKnownRiskBandsAndPVPPolicies` | `catalog/router`: `TestCatalogValidationRejectsInvalidDefinitions`, including invalid risk band and invalid `PVPPolicy` cases plus client projection no-internal-id checks for seeded maps including `1-3` | Covered indirectly by Phase09 desktop map screenshots only | Keep catalog seed deterministic. The seed now includes public `1-3` / Border Skirmish as a PvP-enabled map; production rollout still needs durable flag/backfill docs if DB persistence is introduced. |
| Runtime map instances/membership | Covered | `internal/game/server/runtime.go`, `internal/game/server/server_world_runtime_map_test.go`, `internal/game/world/worker` | `world.snapshot`, `move_to`, `stop`, AOI diffs, `map_subscription_epoch` | `server_world_runtime_map`: `TestRuntimeConstructsWorkerPerConfiguredMapDefinition`, `TestEnsurePlayerSessionPreservesExistingActiveMap`, `TestSessionReconnectMovesMembershipAndAOICursorToActiveMap`, `TestActiveMapSnapshotUsesActiveMapWorker`, `TestTickLoopEmitsAOIOnlyToSessionsAttachedToSameMap` | `server_world_runtime_map`: `TestMoveToAndStopMutateOnlyActiveMapWorker`; live commands during transfer covered in `server_map_transport` | Phase09 desktop screenshots prove starter and destination snapshots only | Current in-memory/dev runtime already routes through bounded multi-map behavior. Production persistence needs migration/backfill/quarantine planning. |
| Portal handoff | Partial | `internal/game/server/server_map_transport_test.go`, `internal/game/server/transport.go`, `client/src/app`, `client/src/state` | `portal.enter`, `map.transfer_started`, `map.transfer_completed`, `map.transfer_failed`, `map.changed`, `world.snapshot`, `player.protection_updated`, `map_subscription_epoch` | `server_map_transport`: `TestPortalEnterTransfersPlayerAndAllActiveSessions`, `TestPortalEnterTransfersToSeededPVPMap`, `TestWorldSnapshotBootstrapIncludesMapSubscriptionEpoch`; client e2e phase09 drives the real authenticated Origin fight/loot/scan/portal loop through a same-session browser WebSocket | `server_map_transport`: `TestPortalEnterRejectsTrustedInternalFieldsWithoutMutation`, `TestPortalEnterOutOfRangeAndCooldownAreNonMutating`, `TestPortalEnterRollbackCleansDestinationAfterSessionAttachFailure`, `TestPortalEnterDuplicateAndOldEpochQueuedEventsDoNotDuplicateOrLeak`, `TestLiveCommandsRejectWhileTransferActive`, `TestScanPulseRejectsPortalTransferInterleavingBeforeQueue`, `TestScanPulseAbortsWhenMapEpochChangesBeforeMutation` | Phase09 desktop/tablet/mobile origin and destination screenshots under `output/screenshots/ui-implementation/09/` | Browser proof now covers the release-smoke portal loop and responsive screenshots. Server proof covers `1-2` `skirmish_gate` transfer to public `1-3`; PvP browser click proof and broader rollout canaries remain open. |
| Safe/PvP policy | Partial | `internal/game/server/server_phase04_policy_test.go`, `internal/game/server/policy_protection.go`, `docs/map-rework/phase-04-portals-safe-zones-pvp.md` | `combat.use_skill`, `player.protection_updated`, `world.snapshot`, map `pvp_policy` | `server_phase04_policy`: `TestSeededPVPMapOutsideSafeZoneAllowsPvPPersistsTargetPlayerStateAndEvents`, `TestPvEAllowedInSafeAndPVEMap` | `server_phase04_policy`: `TestPvPBlockedByMapPolicyBeforeCombatMutation`, `TestSeededPVPMapSafeZoneBlocksPvPBeforeCombatMutation`, `TestSeededPVPMapProtectionBlocksBeforeCombatMutationAndInitiationBreaksProtection`; audit gaps: PvP death/cargo/checkpoint tests missing, safe-zone PvP browser click gap | Phase09 destination screenshot includes safe-zone/protection state, but no browser PvP click proof | Starter maps stay PvP-disabled/protected. Seeded `1-3` provides server-side PvP allow/safe-zone/protection policy proof. PvP rewards must not roll out until death/cargo/checkpoint and abuse coverage exist. |
| Radar/stealth visibility | Partial | `internal/game/server/server_world_visibility_test.go`, `internal/game/world/visibility`, `internal/game/world/worker` | `stealth.toggle`, `scan.pulse`, `aoi.entity_entered`, `aoi.entity_updated`, `world.snapshot` | `server_world_visibility`: `TestHiddenPlayerWitnessVisibilityIsViewerSpecificAndExpires`, `TestScanPulseRevealsHiddenPlayerWithoutPlanetIntelOrXP`, `TestWorldSnapshotProjectionUsesServerOwnedRadarStat`, `TestKnownPlanetMemoryIsFilteredToActiveMapPublicKey`; client e2e phase09 issues one real `scan.pulse` and checks the response/state/WebSocket payload leak canary | `server_world_visibility`: `TestScanPulseDoesNotRevealHiddenPlayerOutsideEffectiveRadarRange`, forbidden hidden-player leak checks in witness tests; Phase09 now OCR-scans its six generated map screenshots, captured harness Go/Vite log lines, and production debug rejection responses for the shared leak-token list only; audit gaps: production logs beyond this harness, admin/debug responses outside this rejection path, source-map/published artifact scans, non-Phase09 WebSocket paths, and non-default bundle leak canaries remain incomplete | Phase09 scan pulse is covered inside the real-server smoke, but no complete browser witness artifact exists | Server-side visibility is strong; rollout still needs broader browser/leak canary scope. |
| Bounded scanner/planets | Partial | `internal/game/discovery/candidate.go`, `internal/game/discovery/scanner.go`, `internal/game/server/server_discovery_planets_test.go`, `docs/map-rework/phase-06-bounded-scanner-planets.md` | `scan.pulse`, `scan.pulse_started`, `scan.pulse_resolved`, `scan.planet_discovered`, `discovery.known_planets`, `discovery.planet_detail` | discovery candidate/scanner: `TestPlanetCandidateGenerationLowDensityBlocksOtherwiseEligibleCell`, `TestResolveScanPulseDiscoversPlanetWritesIntelEventAndXPOnce`, `TestResolveScanPulseMaterializationAndIntelAreZoneScoped`, `server_discovery_planets`; client e2e phase09 covers one authenticated browser scan pulse in the full loop | discovery candidate/scanner/server: `TestPlanetCandidateGenerationFiltersByBiomeAndSpawnBudget`, `TestStartScanPulseCooldownBlocksSpamWithoutMutation`, `TestStartScanPulseEnergyUnavailableFailsBeforeCooldownAndMutation`, `TestResolveScanPulseRadarTooLowReturnsGenericNoSignal`, `TestResolveScanPulseResultJSONOmitsHiddenTruth`, `TestPhase07DiscoveryProductionRouteQueriesUseServerState` hidden-scan-data response/event/world snapshot assertions; audit gaps: no per-map scanner/claim/drop seed matrix or browser scanner success/no-signal matrix | Phase09 browser proof covers one scan pulse, not the full scanner success/no-signal matrix | Keep dev/test scanner seed deterministic and separate from production rarity tuning. |
| Planet claim/production/routes | Partial | `internal/game/server/server_planet_claim_test.go`, `internal/game/production/route_test.go`, `internal/game/production/route_mutation_test.go`, `docs/map-rework/phase-07-planet-claim-production-routes.md` | `discovery.claim_planet`, `planet.claimed`, `discovery.known_planets`, `discovery.planet_detail`, `planet.production_summary`, `planet.storage_summary`, `route.list`, `route.snapshot`, future `route.create/update/enable/disable/settle` | `server_planet_claim`: `TestClaimPlanetSucceedsForKnownNearbyPlanetAndEmitsSafeOwnerEvents`, `TestClaimPlanetDuplicateRetryDoesNotConsumeSecondXCore`; production route: `TestCreateRouteDistanceAndRiskCalculation`, `TestCreateRouteStoresDetachedEnabledRoute`, `TestAutomationRouteValidateRequiresMapIdentity` | `server_planet_claim`: trusted/unknown payload, missing X Core, cross-map, out-of-range, low-rank rejection tests; production route: ownership, destination, non-routeable, unsupported destination, non-positive rate, requirement, policy map identity tests; audit gap: authenticated route mutation gateway contracts open | No claim/route browser rollout artifact in Phase09 proof | Route mutation gap remains tracked in `docs/todo.md`; durable DB/outbox/backfill is still required before multi-process persistence. |
| Enemy pools/spawners | Partial | `internal/game/world/maps/enemy_catalog.go`, `internal/game/world/worker/enemy_spawner.go`, `internal/game/server/server_enemy_spawner_test.go`, `docs/map-rework/phase-08-enemy-pools-spawners-ecs.md` | Worker `InitializeEnemyPoolsCommand`, `TriggerEnemyEventSpawnCommand`, `combat.use_skill`, worker tick telemetry | `enemy_spawner`: `TestRuntimeSeedWorldInitializesStarterEnemyPoolThroughSpawner` now covers starter and `1-2` initial spawn/actor projection, `TestRuntimeMapTwoEnemyLifecycleRespawnsThroughMapInstance` covers `map_1_2` runtime death mark, fake-clock `KillRespawnDelay` respawn, same-row/entity reuse, cap-stable counts, actor projection restore, and no starter contamination, `TestNPCActorProjectionRefreshesTemplateBackedStats`, worker `enemy_spawner` respawn/fill coverage; client e2e phase09 kills starter and destination NPCs through the real browser command path, including after portal handoff | `enemy_spawner`: `TestBootstrapProjectionDoesNotLeakEnemyPoolOrDropProfileInternals`, ownership/cap/forbidden-candidate tests; audit gaps: browser respawn proof, aggro/leash browser proof, and broader cross-map matrix coverage remain open | Starter and destination browser fight proof now exist | Keep second-map enemy seed deterministic; server/runtime destination respawn and cap-stability proof now exists. Add browser respawn, aggro/leash, and broader cross-map combat/loot matrix proof before treating the area as rollout-complete. |
| Map-aware loot/drop | Partial | `internal/game/server/npc_loot_selector.go`, `internal/game/server/npc_loot_selector_test.go`, `internal/game/loot/service.go`, `internal/game/loot/service_test.go` | `combat.use_skill`, `loot.drop_created`, `loot.pickup`, `inventory.snapshot`, `wallet.snapshot` | `npc_loot_selector`: `TestNPCLootSelectorUsesSpawnRecordDropProfileLootTable`, `TestNPCLootSelectorUsesOuterRingSpawnRecordDropProfileLootTable`; loot service: `TestCreateDropsForNPCKillRollsServerSideAndIsIdempotent`, `TestPickupDropOwnerLockPublicAndExpiredWindows`; client e2e phase09 proves starter and destination loot pickup reconcile cargo from server response/state | `npc_loot_selector`: `TestNPCLootSelectorRejectsMissingInputsWithoutTrainingFallback`, `TestNPCLootSelectorRejectsOuterRingMissingTableWithoutTrainingFallback`; loot service: `TestPickupDropRejectsCrossMapViewerWithoutClaim` plus far/hidden/cargo-full, duplicate/concurrent pickup, expired/claimed tests; audit gap: full per-map/risk/rank drop matrix missing | Phase09 proof includes starter and destination fight/loot pickup | Destination-map selector and domain cross-map pickup rejection are covered; full per-map/risk/rank drop matrix remains open. |
| Client map UI/protocol | Partial | `client/src/protocol/envelope.ts`, `client/src/state/reducer.ts`, `client/src/render`, `client/src/ui`, `client/tests/e2e/phase09-map-flow.mjs`, `docs/map-rework/phase-09-client-map-ui-protocol.md` | `world.snapshot`, `map.snapshot`, `map.changed`, `map.policy_updated`, `portal.enter`, `player.protection_updated`, `combat.use_skill`, `loot.pickup`, `scan.pulse` | client e2e phase09 verifies real auth, starter map `1-1`, real starter fight/loot/cargo, real scan pulse, portal visibility, destination `1-2`, old-map cleanup, destination self visibility, destination fight/loot/cargo after portal, and desktop/tablet/mobile screenshots | Client protocol/reducer forbidden key tests; e2e scans DOM, smoke state, localStorage, sessionStorage, cookies, inbound/outbound WebSocket text frames, production debug rejection responses, captured Go/Vite stdout/stderr lines from the harness, and Tesseract OCR output from the six generated Phase09 screenshots for hidden map/spawn/seed/destination/enemy-pool internals and fixture labels; `client/tests/bundle-scan.mjs` scans the default production bundle text and source maps if present for fixture labels/ids and server-only content ids; audit gaps: safe-zone PvP browser click, production logs beyond this harness, admin/debug responses outside this rejection path, non-Phase09 WebSocket paths, and published-artifact bundle scan remain broader Phase10 work | `map-origin-{desktop,tablet,mobile}.png` and `map-outer-ring-{desktop,tablet,mobile}.png` under `output/screenshots/ui-implementation/09/` | Keep Phase09 smoke explicit; this audit leaves it out of `client` `npm run check` to avoid changing routine check cost. |
| No fake/default fixtures | Partial | `client/src/app`, `client/src/protocol/envelope.ts`, `client` bundle scan, `internal/game/server/server_auth_transport_test.go`, `docs/todo.md` | default real mode, `?demo=1` dev-only fixture path, `debug_spawn_npc`, `debug_snapshot` | Phase08J debug/demo spawn quarantine; default authenticated client path uses real Go server/session | Protocol forbidden-payload tests, production debug spawn rejection, default bundle text/source-map scan for fixture labels/ids and server-only content ids when source maps are present; Phase09 e2e DOM/smoke/localStorage/sessionStorage/cookie/WebSocket canary plus production debug rejection response, harness Go/Vite log, and screenshot OCR canaries for its own run artifacts; audit gaps: production logs beyond this harness, admin/debug responses outside this rejection path, published-artifact bundle leak scans, and screenshot paths outside Phase09 remain | Phase09 screenshots are real-server screenshots, not fixture screenshots | Do not use demo/fixture screenshots as release proof. Add broader leak canaries before rollout. |
| Rollout/migration controls | Open | `docs/running-local-game.md`, future server config/docs, future persistence migration docs | Proposed flag only: `GAME_FEATURE_BOUNDED_MULTI_MAP`; future migration/backfill jobs | This docs patch defines the runbook; no code flag/test evidence exists today | No durable flag/backfill/quarantine/rollback tests yet | N/A | Current in-memory/dev runtime already routes through bounded multi-map behavior, but production rollout still needs a durable flag/backfill plan if DB persistence is introduced. |

## Known Audit Gaps

- PvP-enabled map seed now exists as public `1-3` / Border Skirmish, reachable
  through the server-owned `1-2` `skirmish_gate` portal, with server-side policy
  coverage for allowed PvP outside safe zones, safe-zone blocking, and
  protection blocking. PvP death/cargo/checkpoint tests and real browser
  safe-zone PvP click proof remain missing.
- Second-map enemy seed exists, starter/destination browser fight/loot proof
  now exists, and `TestRuntimeMapTwoEnemyLifecycleRespawnsThroughMapInstance`
  covers destination server/runtime respawn with stable caps and no starter
  contamination. Browser respawn proof, aggro/leash browser proof, and broader
  per-map matrix coverage remain missing.
- No per-map scanner/claim/drop seed matrix.
- PvP death/cargo/checkpoint tests are missing.
- Focused scanner rarity/hidden-scan-data regression exists in
  server/discovery tests, and Phase09 now covers one browser scan pulse with
  DOM/state/storage/cookie leak checks, but the per-map scanner/claim/drop
  matrix and broader browser scan variants remain missing.
- Authenticated route mutation gateway contracts remain open; use the existing
  `docs/todo.md` route mutation TODO rather than duplicating it.
- Full per-map/risk/rank drop matrix remains open. Current server coverage
  includes starter selector coverage, `map_1_2` `outer_ring_scout_drone`
  selector/fallback coverage, domain-level cross-map pickup rejection, and
  starter/destination browser fight/loot pickup proof.
- Phase09 now covers inbound/outbound WebSocket text frames,
  DOM/smoke-state/localStorage/sessionStorage/cookie canaries, production
  `debug_snapshot`/`debug_spawn_npc` rejection responses, captured Go/Vite
  stdout/stderr lines from its own harness, and Tesseract OCR/text scans over
  its six generated real-server screenshot PNGs only. Default production bundle
  text/source-map scan now covers fake/default fixture labels/ids and
  server-only content ids when source maps are present in `dist`. Production
  logs beyond this harness, admin/debug responses outside this rejection path,
  non-Phase09 WebSocket paths, published-artifact scans, and screenshot paths
  outside the Phase09 smoke are still missing.
- Bundle hidden-token scan remains partial: `client/tests/bundle-scan.mjs`
  checks default `dist` text and source-map assets if present, not deployed or
  otherwise published artifacts.
- Safe-zone PvP browser click proof is missing.
- `client` `npm run check` does not run the Phase09 Playwright smoke.
- Active bounded-map terminology cleanup for the scoped world/progression/UI
  docs is complete. Remaining old-term search hits should be limited to
  superseded labels, legacy file paths, or historical notes.

## Rollout/Runbook

### Current Dev Runtime

The current in-memory/dev runtime already routes authenticated sessions through
bounded multi-map behavior: map catalog lookup, per-map worker instances,
server-owned active map membership, bounded movement, portal transfer, public
map snapshots, and map-scoped AOI all run in the local Go server.

There is no production rollout flag in code today. Use `GAME_FEATURE_BOUNDED_MULTI_MAP`
as the proposed future flag name if a durable DB-backed or production
deployment path is introduced. Do not document that flag as available until the
server actually reads it.

### Deterministic Seeds

Local and CI smoke should use deterministic map seeds:

- starter map `1-1` / Origin Fringe, bounded `0..10000`
- destination map `1-2` / Outer Ring, bounded `0..10000`
- PvP map `1-3` / Border Skirmish, bounded `0..10000`
- explicit bidirectional portals such as `east_gate`, `west_gate`, and
  `skirmish_gate`
- explicit safe-zone/protection projections
- explicit starter enemy pool and deterministic second-map enemy pool
- explicit scanner and planet claim/drop profiles for a per-map matrix

Production tuning must stay separate from dev/test seeds. Scanner rarity,
enemy spawn density, drop rates, route risk, and PvP rewards should not inherit
forced deterministic smoke values.

### Local Smoke

Run the focused real-server Phase09 bounded-map/portal smoke explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase09-map
```

The current expected screenshot artifacts are:

```text
output/screenshots/ui-implementation/09/map-origin-desktop.png
output/screenshots/ui-implementation/09/map-origin-tablet.png
output/screenshots/ui-implementation/09/map-origin-mobile.png
output/screenshots/ui-implementation/09/map-outer-ring-desktop.png
output/screenshots/ui-implementation/09/map-outer-ring-tablet.png
output/screenshots/ui-implementation/09/map-outer-ring-mobile.png
```

Those artifacts are captured during the current Phase09 real-server browser loop
across desktop, tablet, and mobile viewports. The smoke also proves starter and
destination fight/loot pickup from server state after portal handoff, scans
captured inbound/outbound text frames from the app and command WebSockets, and
runs a Tesseract OCR leak-token canary over those six generated PNGs. It also
sends `debug_snapshot` and `debug_spawn_npc` over the authenticated command
socket, verifies safe production rejection responses, and scans captured Go/Vite
stdout/stderr lines from the harness. It does not prove browser PvP clicks,
production logs beyond this harness, admin/debug responses outside this
rejection path, published artifact bundle scans, non-Phase09
WebSocket paths, screenshot paths outside this Phase09 run, or inclusion in
`npm run check`.

### Canary Leak Scope

Before production enablement, canary checks must scan at least:

- authenticated WebSocket payloads and queued events
- DOM text and smoke-visible app state
- local/session storage
- cookies and session metadata exposed to the browser
- screenshot OCR/text or equivalent screenshot artifact scans
- production bundle text and source maps if present
- server logs and debug/admin responses

The forbidden scope includes hidden candidates, procedural seeds, future spawn
candidates, enemy pool internals, spawn area ids, drop table ids, loot rolls,
scan rolls, internal map ids, destination spawn internals, session tokens,
passwords, password hashes, reset secrets, and fake/default fixture labels.

The default client bundle scan in `client/tests/bundle-scan.mjs` checks built
`dist` text and source-map assets, when present, for fake/default fixture labels
and concrete server-only map/content ids. It intentionally does not forbid
generic protocol guard field names such as hidden scan or loot key strings, and
it does not inspect separately deployed or published artifacts.

The Phase09 smoke currently satisfies only a narrow server-side canary subset:
captured local Go/Vite stdout/stderr lines from that harness and production
debug rejection responses for `debug_snapshot`/`debug_spawn_npc`. It is not a
complete production log or admin/debug response audit.

### Migration, Backfill, And Quarantine

If DB persistence is introduced, write a migration plan before enabling the
future flag:

- Backfill valid existing world/zone/player rows into the starter internal map.
- Preserve committed wallet, inventory, loot, claim, route, and production
  references; do not rewrite ledger truth silently.
- Quarantine rows with non-finite or out-of-bounds coordinates for manual
  repair. Do not silently clamp old data into `0..10000`.
- Quarantine rows that cannot be assigned to a known map/catalog version.
- Backfill route rows with source/destination map identity only when endpoint
  visibility/access can be proven from durable ownership/intel.
- Keep hidden scanner candidates and loot/spawn rolls server-only during any
  export or repair job.
- Reconcile old sessions by forcing fresh authenticated `world.snapshot`
  resolution through the server-owned map router.

### Rollback

Rollback must preserve committed player and economy state:

1. Disable the future `GAME_FEATURE_BOUNDED_MULTI_MAP` flag if it exists.
2. Stop accepting new `portal.enter` transfers.
3. Keep current player map state readable for reconciliation.
4. Force reconnecting sessions to a safe starter-map snapshot only through
   server-owned session resolution.
5. Preserve committed ledger, inventory, loot pickup, planet claim, production,
   and route mutations.
6. Stop PvP reward/cargo-risk paths first if safe/PvP policy or death coverage
   is suspected.
7. Run canary leak scans again after rollback to ensure old-map or hidden
   payloads are not left in DOM/app state/storage.

## Verification Policy

Docs-only updates do not require the full release gate. Code rollout must run
the normal project checks before handoff:

```bash
go test ./...
git diff --check
cd client
npm --cache /tmp/gameproject-npm-cache run check
```

Client map rollout evidence must also include the explicit Phase09 smoke until
the project intentionally wires it into `npm run check`:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase09-map
```

No release claim should be made from fixture/demo screenshots or client-local
mock state. Open contracts must stay visible in `docs/todo.md` instead of being
masked by placeholder UI data.
