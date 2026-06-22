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
| Map catalog/schema | Partial | `internal/game/world/maps/catalog.go`, `internal/game/world/maps/router.go`, `docs/map-rework/phase-01-map-catalog-router.md` | `map.snapshot`, `world.snapshot`, catalog `ClientMapProjection` | `catalog/router`: `TestStarterCatalogReturnsBoundedStarterSpawnAndProjection`, `TestRouterEnsureStarterLocationCreatesOnceAndPreservesExisting`, `TestRouterValidatesActivePosition`, `TestCatalogValidationAcceptsKnownRiskBandsAndPVPPolicies` | `catalog/router`: `TestCatalogValidationRejectsInvalidDefinitions`, including invalid risk band and invalid `PVPPolicy` cases | Covered indirectly by Phase09 desktop map screenshots only | Keep catalog seed deterministic. Production rollout still needs durable flag/backfill docs if DB persistence is introduced. |
| Runtime map instances/membership | Covered | `internal/game/server/runtime.go`, `internal/game/server/server_world_runtime_map_test.go`, `internal/game/world/worker` | `world.snapshot`, `move_to`, `stop`, AOI diffs, `map_subscription_epoch` | `server_world_runtime_map`: `TestRuntimeConstructsWorkerPerConfiguredMapDefinition`, `TestEnsurePlayerSessionPreservesExistingActiveMap`, `TestSessionReconnectMovesMembershipAndAOICursorToActiveMap`, `TestActiveMapSnapshotUsesActiveMapWorker`, `TestTickLoopEmitsAOIOnlyToSessionsAttachedToSameMap` | `server_world_runtime_map`: `TestMoveToAndStopMutateOnlyActiveMapWorker`; live commands during transfer covered in `server_map_transport` | Phase09 desktop screenshots prove starter and destination snapshots only | Current in-memory/dev runtime already routes through bounded multi-map behavior. Production persistence needs migration/backfill/quarantine planning. |
| Portal handoff | Partial | `internal/game/server/server_map_transport_test.go`, `internal/game/server/transport.go`, `client/src/app`, `client/src/state` | `portal.enter`, `map.transfer_started`, `map.transfer_completed`, `map.transfer_failed`, `map.changed`, `world.snapshot`, `player.protection_updated`, `map_subscription_epoch` | `server_map_transport`: `TestPortalEnterTransfersPlayerAndAllActiveSessions`, `TestWorldSnapshotBootstrapIncludesMapSubscriptionEpoch`; client e2e phase09 drives the real authenticated Origin fight/loot/scan/portal loop through a same-session browser WebSocket | `server_map_transport`: `TestPortalEnterRejectsTrustedInternalFieldsWithoutMutation`, `TestPortalEnterOutOfRangeAndCooldownAreNonMutating`, `TestPortalEnterRollbackCleansDestinationAfterSessionAttachFailure`, `TestPortalEnterDuplicateAndOldEpochQueuedEventsDoNotDuplicateOrLeak`, `TestLiveCommandsRejectWhileTransferActive`, `TestScanPulseRejectsPortalTransferInterleavingBeforeQueue`, `TestScanPulseAbortsWhenMapEpochChangesBeforeMutation` | Phase09 desktop/tablet/mobile origin and destination screenshots under `output/screenshots/ui-implementation/09/` | Browser proof now covers the release-smoke portal loop and responsive screenshots. PvP browser click proof and broader rollout canaries remain open. |
| Safe/PvP policy | Partial | `internal/game/server/server_phase04_policy_test.go`, `internal/game/server/policy_protection.go`, `docs/map-rework/phase-04-portals-safe-zones-pvp.md` | `combat.use_skill`, `player.protection_updated`, `world.snapshot`, map `pvp_policy` | `server_phase04_policy`: `TestAllowedPvPPersistsTargetPlayerStateAndEvents`, `TestPvEAllowedInSafeAndPVEMap` | `server_phase04_policy`: `TestPvPBlockedByMapPolicyBeforeCombatMutation`, `TestPvPBlockedBySafeZoneBeforeCombatMutation`, `TestPvPBlockedByProtectionBeforeCombatMutationAndInitiationBreaksProtection`; audit gaps: no PvP-enabled map seed, PvP death/cargo/checkpoint tests missing, safe-zone PvP browser click gap | Phase09 destination screenshot includes safe-zone/protection state, but no browser PvP click proof | Starter maps stay PvP-disabled/protected. PvP rewards must not roll out until death/cargo/checkpoint and abuse coverage exist. |
| Radar/stealth visibility | Partial | `internal/game/server/server_world_visibility_test.go`, `internal/game/world/visibility`, `internal/game/world/worker` | `stealth.toggle`, `scan.pulse`, `aoi.entity_entered`, `aoi.entity_updated`, `world.snapshot` | `server_world_visibility`: `TestHiddenPlayerWitnessVisibilityIsViewerSpecificAndExpires`, `TestScanPulseRevealsHiddenPlayerWithoutPlanetIntelOrXP`, `TestWorldSnapshotProjectionUsesServerOwnedRadarStat`, `TestKnownPlanetMemoryIsFilteredToActiveMapPublicKey`; client e2e phase09 issues one real `scan.pulse` and checks the response/state leak canary | `server_world_visibility`: `TestScanPulseDoesNotRevealHiddenPlayerOutsideEffectiveRadarRange`, forbidden hidden-player leak checks in witness tests; audit gap: WebSocket payload, screenshot text/OCR, server log/debug-response, and production bundle leak scans remain incomplete | Phase09 scan pulse is covered inside the real-server smoke, but no complete browser witness artifact exists | Server-side visibility is strong; rollout still needs broader browser/leak canary scope. |
| Bounded scanner/planets | Partial | `internal/game/discovery/candidate.go`, `internal/game/discovery/scanner.go`, `internal/game/server/server_discovery_planets_test.go`, `docs/map-rework/phase-06-bounded-scanner-planets.md` | `scan.pulse`, `scan.pulse_started`, `scan.pulse_resolved`, `scan.planet_discovered`, `discovery.known_planets`, `discovery.planet_detail` | discovery candidate/scanner: `TestPlanetCandidateGenerationLowDensityBlocksOtherwiseEligibleCell`, `TestResolveScanPulseDiscoversPlanetWritesIntelEventAndXPOnce`, `TestResolveScanPulseMaterializationAndIntelAreZoneScoped`, `server_discovery_planets`; client e2e phase09 covers one authenticated browser scan pulse in the full loop | discovery candidate/scanner/server: `TestPlanetCandidateGenerationFiltersByBiomeAndSpawnBudget`, `TestStartScanPulseCooldownBlocksSpamWithoutMutation`, `TestStartScanPulseEnergyUnavailableFailsBeforeCooldownAndMutation`, `TestResolveScanPulseRadarTooLowReturnsGenericNoSignal`, `TestResolveScanPulseResultJSONOmitsHiddenTruth`, `TestPhase07DiscoveryProductionRouteQueriesUseServerState` hidden-scan-data response/event/world snapshot assertions; audit gaps: no per-map scanner/claim/drop seed matrix or browser scanner success/no-signal matrix | Phase09 browser proof covers one scan pulse, not the full scanner success/no-signal matrix | Keep dev/test scanner seed deterministic and separate from production rarity tuning. |
| Planet claim/production/routes | Partial | `internal/game/server/server_planet_claim_test.go`, `internal/game/production/route_test.go`, `internal/game/production/route_mutation_test.go`, `docs/map-rework/phase-07-planet-claim-production-routes.md` | `discovery.claim_planet`, `planet.claimed`, `discovery.known_planets`, `discovery.planet_detail`, `planet.production_summary`, `planet.storage_summary`, `route.list`, `route.snapshot`, future `route.create/update/enable/disable/settle` | `server_planet_claim`: `TestClaimPlanetSucceedsForKnownNearbyPlanetAndEmitsSafeOwnerEvents`, `TestClaimPlanetDuplicateRetryDoesNotConsumeSecondXCore`; production route: `TestCreateRouteDistanceAndRiskCalculation`, `TestCreateRouteStoresDetachedEnabledRoute`, `TestAutomationRouteValidateRequiresMapIdentity` | `server_planet_claim`: trusted/unknown payload, missing X Core, cross-map, out-of-range, low-rank rejection tests; production route: ownership, destination, non-routeable, unsupported destination, non-positive rate, requirement, policy map identity tests; audit gap: authenticated route mutation gateway contracts open | No claim/route browser rollout artifact in Phase09 proof | Route mutation gap remains tracked in `docs/todo.md`; durable DB/outbox/backfill is still required before multi-process persistence. |
| Enemy pools/spawners | Partial | `internal/game/world/maps/enemy_catalog.go`, `internal/game/world/worker/enemy_spawner.go`, `internal/game/server/server_enemy_spawner_test.go`, `docs/map-rework/phase-08-enemy-pools-spawners-ecs.md` | Worker `InitializeEnemyPoolsCommand`, `TriggerEnemyEventSpawnCommand`, `combat.use_skill`, worker tick telemetry | `enemy_spawner`: `TestRuntimeSeedWorldInitializesStarterEnemyPoolThroughSpawner` now covers starter and `1-2` initial spawn/actor projection, `TestNPCActorProjectionRefreshesTemplateBackedStats`, worker `enemy_spawner` respawn/fill coverage; client e2e phase09 kills the starter NPC through the real browser command path | `enemy_spawner`: `TestBootstrapProjectionDoesNotLeakEnemyPoolOrDropProfileInternals`, ownership/cap/forbidden-candidate tests; audit gaps: second-map fight/loot/browser proof and cross-map matrix coverage remain open | Starter browser fight proof now exists; destination fight/loot after portal remains open | Keep second-map enemy seed deterministic; add cross-map combat, respawn, loot, and browser proof before treating the area as rollout-complete. |
| Map-aware loot/drop | Partial | `internal/game/server/npc_loot_selector.go`, `internal/game/server/npc_loot_selector_test.go`, `internal/game/loot/service.go`, `internal/game/loot/service_test.go` | `combat.use_skill`, `loot.drop_created`, `loot.pickup`, `inventory.snapshot`, `wallet.snapshot` | `npc_loot_selector`: `TestNPCLootSelectorUsesSpawnRecordDropProfileLootTable`, `TestNPCLootSelectorUsesOuterRingSpawnRecordDropProfileLootTable`; loot service: `TestCreateDropsForNPCKillRollsServerSideAndIsIdempotent`, `TestPickupDropOwnerLockPublicAndExpiredWindows`; client e2e phase09 proves starter `raw_ore` loot pickup reconciles cargo from server response/state | `npc_loot_selector`: `TestNPCLootSelectorRejectsMissingInputsWithoutTrainingFallback`, `TestNPCLootSelectorRejectsOuterRingMissingTableWithoutTrainingFallback`; loot service: `TestPickupDropRejectsCrossMapViewerWithoutClaim` plus far/hidden/cargo-full, duplicate/concurrent pickup, expired/claimed tests; audit gap: full per-map/risk/rank drop matrix missing | Phase09 proof includes starter-map fight/loot; destination fight/loot remains open | Destination-map selector and domain cross-map pickup rejection are covered; full per-map/risk/rank drop matrix remains open. |
| Client map UI/protocol | Partial | `client/src/protocol/envelope.ts`, `client/src/state/reducer.ts`, `client/src/render`, `client/src/ui`, `client/tests/e2e/phase09-map-flow.mjs`, `docs/map-rework/phase-09-client-map-ui-protocol.md` | `world.snapshot`, `map.snapshot`, `map.changed`, `map.policy_updated`, `portal.enter`, `player.protection_updated`, `combat.use_skill`, `loot.pickup`, `scan.pulse` | client e2e phase09 verifies real auth, starter map `1-1`, real starter fight/loot/cargo, real scan pulse, portal visibility, destination `1-2`, old-map cleanup, destination self visibility, and desktop/tablet/mobile screenshots | Client protocol/reducer forbidden key tests; e2e scans DOM, smoke state, localStorage, sessionStorage, and cookies for hidden map/spawn/seed/destination/enemy-pool internals and fixture labels; audit gaps: safe-zone PvP browser click, WebSocket payload canary, screenshot text/OCR scan, server log/debug-response scan, and production bundle scan remain broader Phase10 work | `map-origin-{desktop,tablet,mobile}.png` and `map-outer-ring-{desktop,tablet,mobile}.png` under `output/screenshots/ui-implementation/09/` | Keep Phase09 smoke explicit; this audit leaves it out of `client` `npm run check` to avoid changing routine check cost. |
| No fake/default fixtures | Partial | `client/src/app`, `client/src/protocol/envelope.ts`, `client` bundle scan, `internal/game/server/server_auth_transport_test.go`, `docs/todo.md` | default real mode, `?demo=1` dev-only fixture path, `debug_spawn_npc`, `debug_snapshot` | Phase08J debug/demo spawn quarantine; default authenticated client path uses real Go server/session | Protocol forbidden-payload tests, production debug spawn rejection, partial bundle hidden-token scan; Phase09 e2e DOM/smoke/localStorage/sessionStorage/cookie canary; audit gaps: WebSocket payload, screenshot text/OCR, server log/debug-response, and full production bundle leak scans remain | Phase09 screenshots are real-server screenshots, not fixture screenshots | Do not use demo/fixture screenshots as release proof. Add broader leak canaries before rollout. |
| Rollout/migration controls | Open | `docs/running-local-game.md`, future server config/docs, future persistence migration docs | Proposed flag only: `GAME_FEATURE_BOUNDED_MULTI_MAP`; future migration/backfill jobs | This docs patch defines the runbook; no code flag/test evidence exists today | No durable flag/backfill/quarantine/rollback tests yet | N/A | Current in-memory/dev runtime already routes through bounded multi-map behavior, but production rollout still needs a durable flag/backfill plan if DB persistence is introduced. |

## Known Audit Gaps

- No PvP-enabled map seed.
- Second-map enemy seed exists, and starter-map browser fight/loot proof now
  exists, but destination enemy fight/loot/respawn browser proof and broader
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
  selector/fallback coverage, domain-level cross-map pickup rejection, and a
  starter-map browser fight/loot pickup proof.
- WebSocket payload, screenshot text/OCR, server log/debug-response, and full
  production bundle leak scans are still missing; Phase09 now covers
  DOM/smoke-state/localStorage/sessionStorage/cookie canaries.
- Bundle hidden-token scan is partial.
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
- explicit bidirectional portals such as `east_gate` and `west_gate`
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

Those artifacts prove the current Phase09 real-server browser loop across
desktop, tablet, and mobile viewports. They do not prove browser PvP clicks,
WebSocket payload canaries, screenshot text/OCR scans, server log/debug-response
scans, full production bundle leak scans, or inclusion in `npm run check`.

### Canary Leak Scope

Before production enablement, canary checks must scan at least:

- authenticated WebSocket payloads and queued events
- DOM text and smoke-visible app state
- local/session storage
- cookies and session metadata exposed to the browser
- screenshot OCR/text or equivalent screenshot artifact scans
- production bundle text and source maps if published
- server logs and debug/admin responses

The forbidden scope includes hidden candidates, procedural seeds, future spawn
candidates, enemy pool internals, spawn area ids, drop table ids, loot rolls,
scan rolls, internal map ids, destination spawn internals, session tokens,
passwords, password hashes, reset secrets, and fake/default fixture labels.

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
