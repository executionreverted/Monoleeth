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
| Portal handoff | Partial | `internal/game/server/server_map_transport_test.go`, `internal/game/server/transport.go`, `client/src/app`, `client/src/state` | `portal.enter`, `map.transfer_started`, `map.transfer_completed`, `map.transfer_failed`, `map.changed`, `world.snapshot`, `player.protection_updated`, `map_subscription_epoch` | `server_map_transport`: `TestPortalEnterTransfersPlayerAndAllActiveSessions`, `TestPortalEnterTransfersToSeededPVPMap`, `TestWorldSnapshotBootstrapIncludesMapSubscriptionEpoch`; client e2e phase09 drives the real authenticated Origin fight/loot/scan/portal loop through a same-session browser WebSocket | `server_map_transport`: `TestPortalEnterRejectsTrustedInternalFieldsWithoutMutation`, `TestPortalEnterOutOfRangeAndCooldownAreNonMutating`, `TestPortalEnterRollbackCleansDestinationAfterSessionAttachFailure`, `TestPortalEnterDuplicateAndOldEpochQueuedEventsDoNotDuplicateOrLeak`, `TestLiveCommandsRejectWhileTransferActive`, `TestScanPulseRejectsPortalTransferInterleavingBeforeQueue`, `TestScanPulseAbortsWhenMapEpochChangesBeforeMutation` | Phase09 desktop/tablet/mobile origin and destination screenshots under `output/screenshots/ui-implementation/09/` | Browser proof now covers the release-smoke portal loop and responsive screenshots. Server proof covers `1-2` `skirmish_gate` transfer to public `1-3`; broader rollout canaries remain open. |
| Safe/PvP policy | Partial | `internal/game/server/server_phase04_policy_test.go`, `internal/game/server/server_pvp_death_regression_test.go`, `internal/game/server/policy_protection.go`, `internal/game/death/service_test.go`, `client/tests/e2e/phase10-pvp-death-flow.mjs`, `docs/map-rework/phase-04-portals-safe-zones-pvp.md` | `combat.use_skill`, `death.repair_quote`, `death.repair_ship`, `player.protection_updated`, `position.corrected`, `world.snapshot`, map `pvp_policy`; domain `DeathService.ProcessDeath` | `server_phase04_policy`: `TestSeededPVPMapOutsideSafeZoneAllowsPvPPersistsTargetPlayerStateAndEvents`, `TestSeededPVPMapLethalDeathFlowDisablesTargetDropsCargoAndBlocksActions`, `TestPvEAllowedInSafeAndPVEMap`; `server_pvp_death_regression`: `TestSeededPVPMapDeathRepairRespawnsAtCheckpointWithProtectionAndDuplicateRepairIsIdempotent` covers repair snapping to the server-selected checkpoint, restored actor state, respawn protection, client-safe events, and duplicate repair idempotency; `death`: `TestDeathServiceProcessDeathPvPKillerOwnedDropUsesZonePolicyAndCheckpoint` covers ProcessDeath preserving a PvP killer, killer-owned loot drop, zone cargo policy, and checkpoint/respawn id in domain result/events; client e2e Phase10 proves two real authenticated browser sessions, real target `raw_ore` cargo from NPC loot, public portal travel `1-1` -> `1-2` -> `1-3`, lethal PvP `combat.use_skill`, attacker-visible death cargo drop, target `death.repair_quote` / `death.repair_ship`, and repaired public `1-3` checkpoint/protection reconciliation | `server_phase04_policy`: `TestPvPBlockedByMapPolicyBeforeCombatMutation`, `TestSeededPVPMapSafeZoneBlocksPvPBeforeCombatMutation`, `TestSeededPVPMapProtectionBlocksBeforeCombatMutationAndInitiationBreaksProtection`; client e2e Phase10 command-socket proof rejects protected/safe-spawn PvP with `ERR_PVP_BLOCKED`, UI-click proof selects the visible protected player through real HUD target selection, clicks real `[data-action="fire"]`, captures outbound UI `combat.use_skill`, verifies inbound `ERR_PVP_BLOCKED`, rejects disabled target actions with `ERR_SHIP_DISABLED`, and scans death/repair responses, selected smoke state, WebSocket frames, storage, cookies, and harness logs for death/respawn internals | Phase10 command-socket and UI-click browser proof via `npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-pvp-death`; no screenshot artifact | Starter maps stay PvP-disabled/protected. Seeded `1-3` now has server and real-browser proof for PvP death, cargo drop, repair quote, checkpoint repair respawn, respawn protection, and safe-zone/protection UI-click rejection. Broader rollout canaries remain open. |
| Radar/stealth visibility | Partial | `internal/game/server/server_world_visibility_test.go`, `internal/game/world/visibility`, `internal/game/world/worker` | `stealth.toggle`, `scan.pulse`, `aoi.entity_entered`, `aoi.entity_updated`, `world.snapshot` | `server_world_visibility`: `TestHiddenPlayerWitnessVisibilityIsViewerSpecificAndExpires`, `TestScanPulseRevealsHiddenPlayerWithoutPlanetIntelOrXP`, `TestWorldSnapshotProjectionUsesServerOwnedRadarStat`, `TestKnownPlanetMemoryIsFilteredToActiveMapPublicKey`; client e2e phase09 issues one real `scan.pulse` and checks the response/state/WebSocket payload leak canary | `server_world_visibility`: `TestScanPulseDoesNotRevealHiddenPlayerOutsideEffectiveRadarRange`, forbidden hidden-player leak checks in witness tests; Phase09 now OCR-scans its six generated map screenshots, captured harness Go/Vite log lines, and production debug rejection responses for the shared leak-token list only; audit gaps: production logs beyond this harness, admin/debug responses outside this rejection path, CI/deploy-fed published artifact scans, non-Phase09 WebSocket paths, and non-default bundle leak canaries remain incomplete | Phase09 scan pulse is covered inside the real-server smoke, but no complete browser witness artifact exists | Server-side visibility is strong; rollout still needs broader browser/leak canary scope. |
| Bounded scanner/planets | Partial | `internal/game/discovery/candidate.go`, `internal/game/discovery/scanner.go`, `internal/game/server/server_discovery_planets_test.go`, `docs/map-rework/phase-06-bounded-scanner-planets.md` | `scan.pulse`, `scan.pulse_started`, `scan.pulse_resolved`, `scan.planet_discovered`, `discovery.known_planets`, `discovery.planet_detail` | discovery candidate/scanner: `TestPlanetCandidateGenerationLowDensityBlocksOtherwiseEligibleCell`, `TestResolveScanPulseDiscoversPlanetWritesIntelEventAndXPOnce`, `TestResolveScanPulseMaterializationAndIntelAreSeededMapScoped` covers domain scanner materialization/intel across seeded public maps `1-1`, `1-2`, and `1-3`, `server_discovery_planets`; `TestScanPulseUsesActiveSeededMapScope` proves authenticated runtime sessions active on `map_1_2` and `map_1_3` scan through server-owned active map scope and keep response/event/read-model memory on public `1-2` and `1-3`; client e2e phase09 covers one authenticated browser scan pulse in the full loop | discovery candidate/scanner/server: `TestPlanetCandidateGenerationFiltersByBiomeAndSpawnBudget`, `TestStartScanPulseCooldownBlocksSpamWithoutMutation`, `TestStartScanPulseEnergyUnavailableFailsBeforeCooldownAndMutation`, `TestResolveScanPulseRadarTooLowReturnsGenericNoSignal`, `TestResolveScanPulseResultJSONOmitsHiddenTruth`, `TestPhase07DiscoveryProductionRouteQueriesUseServerState` and `TestScanPulseUsesActiveSeededMapScope` hidden-scan-data response/event/world snapshot assertions; audit gaps: no full per-map scanner/claim/drop seed matrix or browser scanner success/no-signal matrix | Phase09 browser proof covers one scan pulse, not the full scanner success/no-signal matrix | Keep dev/test scanner seed deterministic and separate from production rarity tuning. |
| Planet claim/production/routes | Partial | `internal/game/server/server_planet_claim_test.go`, `internal/game/server/server_discovery_planets_test.go`, `internal/game/server/server_production_summary_settlement_test.go`, `internal/game/server/server_route_control_test.go`, `internal/game/server/server_route_update_test.go`, `internal/game/server/server_route_settle_test.go`, `internal/game/server/server_e2e_route_seed_test.go`, `internal/game/production/route_test.go`, `internal/game/production/route_mutation_test.go`, `client/tests/e2e/phase10-planet-claim-flow.mjs`, `client/tests/e2e/phase10-route-flow.mjs`, `docs/map-rework/phase-07-planet-claim-production-routes.md` | `discovery.claim_planet`, `planet.claimed`, `discovery.known_planets`, `discovery.planet_detail`, `planet.production_summary`, `planet.storage_summary`, `route.create`, `route.update`, `route.enable`, `route.disable`, `route.settle`, `route.list`, `route.snapshot`, `route.updated`, `route.settled` | `server_planet_claim`: `TestClaimPlanetSucceedsForKnownNearbyPlanetAndEmitsSafeOwnerEvents`, `TestClaimPlanetDuplicateRetryDoesNotConsumeSecondXCore`, `TestClaimPlanetSucceedsOnSeededDestinationMap` for public `1-2`, `TestClaimPlanetSucceedsOnSeededPVPMap` for public `1-3`; focused Phase10 browser claim proof registers a real user, scans, opens real planet detail, sends the real HUD `discovery.claim_planet` click path, consumes one E2E-seeded X Core through Inventory, uses E2E-seeded Progression rank for claim eligibility, receives `planet.claimed`, reconciles production/inventory, and checks leak canaries; server production summary: `TestProductionSummarySettlesOwnedActiveMapProductionAndQueuesSafeEvents` proves `planet.production_summary` settles only owned active-map production, advances storage/time, emits owner-scoped safe production/storage events without AOI diffs, leaves other-owner/other-map planets unchanged, and no-ops immediate duplicate queries; `TestPlanetStorageSummarySettlesProductionWithRequestScopedNow` proves `planet.storage_summary` reflects settled storage and uses one request-scoped settlement timestamp across multiple planets; production route: `TestCreateRouteDistanceAndRiskCalculation`, `TestCreateRouteStoresDetachedEnabledRoute`, `TestAutomationRouteValidateRequiresMapIdentity`; server route gateway: `TestRouteCreateCreatesOwnedPlanetRouteThroughGateway` proves `route.create` derives owner, route id, endpoint map ids, and owner-scoped safe route events server-side; `TestRouteUpdateChangesOwnedRouteTermsThroughGateway` proves `route.update` derives owner, preserves source truth, changes destination/resource/rate, and emits owner-scoped safe route events; `TestRouteUpdateSettlesElapsedStorageAndQueuesActiveMapSnapshots` proves elapsed update settlement reconciles active-map production/storage snapshots; `TestRouteControlDisableThenEnableThroughGatewayQueuesSafeOwnerEvents` proves `route.disable` and `route.enable` toggle the owned route through safe response/list/snapshot/event payloads; `TestRouteDisableSettlesStorageAndQueuesSafeProductionStorageSnapshots` proves elapsed disable settlement reconciles active-map production/storage snapshots; `TestRouteSettleTransfersStorageAndQueuesSafeOwnerEvents` proves single-route settle transfers storage, advances the cursor, returns safe route/routes/settlement/production/storage payloads, and queues owner-scoped `route.settled` plus reconciliation events without AOI diff; `TestRouteSettleImmediateDuplicateReturnsNoOpWithoutDuplicateTransfer` proves immediate no-op duplicate settlement does not move storage twice; `TestRouteSettleEmptyPayloadReconcilesOwnedRoutesOnly` proves `{}` settles only authenticated-owner routes and emits one route list plus one production/storage pair; browser route proof clicks real HUD controls for `route.create`, `route.update`, `route.disable`, `route.enable`, single-route `route.settle`, and empty-payload owner reconcile, then verifies exact client-safe outbound keys and `state.routes` reconciliation | `server_planet_claim`: trusted/unknown payload, missing X Core, cross-map, out-of-range, low-rank rejection tests; protocol/reducer/UI tests prove the browser claim intent only sends `planet_id`, clears pending state, and handles `planet.claimed` without an unhandled-event log; server production summary: `TestProductionSummaryRejectsSpoofedServerOwnedFieldsBeforeSettlement` rejects forged owner/map/time/output/storage/building facts before mutation/events; production route: ownership, destination, non-routeable, unsupported destination, non-positive rate, requirement, policy map identity tests; server route gateway: `TestRouteCreateRejectsSpoofedServerOwnedFieldsBeforeMutation` rejects forged owner/map/energy/risk fields before mutation, `TestRouteUpdateRejectsSpoofedServerOwnedFieldsBeforeMutation` rejects forged owner/map/source/destination/enabled/settlement/storage/energy/risk fields before mutation, `TestRouteUpdateRejectsWrongOwnerWithoutMutationOrEvents` rejects wrong-owner updates without mutation/events, `TestRouteUpdateRejectsXCoreResourceBeforeMutation` rejects X Core/non-routeable resources without mutation/events, `TestRouteControlRejectsSpoofedServerOwnedFieldsBeforeMutation` rejects forged owner/map/enabled/settlement/source/destination/storage/risk fields before mutation, `TestRouteControlRejectsWrongOwnerWithoutMutationOrEvents` rejects wrong-owner controls without mutation/events, `TestRouteControlMapsStoredRouteConfigErrorsToInternal` keeps stored-route/config errors internal, `TestRouteSettleRejectsSpoofedServerOwnedFieldsBeforeMutation` rejects forged owner/map/source/destination/enabled/settlement/window/storage/energy/risk/amount/resource facts before mutation, `TestRouteSettleRejectsWrongOwnerWithoutMutationOrEvents` rejects wrong-owner settle without mutation/events, and `TestRouteSettleMapsErrorsSafely` keeps stored/config errors internal while mapping not-found endpoint cases safely; protocol/HUD/browser proof rejects client-authored route owner/map/risk/storage/settlement/timing fields from outbound payloads and scans DOM/state/WebSocket/log surfaces for internals | Focused claim proof via `npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-planet-claim`; focused route proof via `npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-route` | Destination-map server claim success is covered for seeded `map_1_2` / public `1-2` and PvP `map_1_3` / public `1-3`, including safe payload/event and one-X-Core consumption assertions. Production summary/storage queries now have focused server-side offline settlement reconcile coverage. Browser claim proof now covers the starter-map real-client claim path with an E2E-only Inventory X Core plus Progression rank seed. Browser route proof now covers the owned same-map MVP path with a dev-only `GAME_E2E_ROUTE_SEED`. Browser drop flow, the full claim/drop matrix, broader route policy matrix, and durable DB/outbox/backfill/window idempotency remain open. |
| Enemy pools/spawners | Partial | `internal/game/world/maps/enemy_catalog.go`, `internal/game/world/worker/enemy_spawner.go`, `internal/game/server/server_enemy_spawner_test.go`, `docs/map-rework/phase-08-enemy-pools-spawners-ecs.md` | Worker `InitializeEnemyPoolsCommand`, `TriggerEnemyEventSpawnCommand`, `combat.use_skill`, `stealth.toggle`, worker tick telemetry | `enemy_spawner`: `TestRuntimeSeedWorldInitializesStarterEnemyPoolThroughSpawner` now covers starter, `1-2`, and `1-3` initial spawn/actor projection, `TestRuntimeMapTwoEnemyLifecycleRespawnsThroughMapInstance` covers `map_1_2` runtime death mark, fake-clock `KillRespawnDelay` respawn, same-row/entity reuse, cap-stable counts, actor projection restore, and no starter contamination, `TestNPCActorProjectionRefreshesTemplateBackedStats`, worker `enemy_spawner` respawn/fill coverage; worker `enemy_aggro`: `TestSeededPassiveEnemyAggroProfilesDoNotAcquireOrMoveAcrossMaps` proves seeded starter `1-1` and destination `1-2` passive NPC rows do not acquire a nearby player or start moving on initial fill, and `TestSeededBorderSkirmishAggressiveEnemyAggroLeashUsesCatalogSeed` proves seeded public `1-3` aggressive border raider acquisition uses catalog-owned aggro/leash/stat rows and resets/returns on seeded leash break; client e2e phase09 kills starter and destination NPCs through the real browser command path, including after portal handoff, and now waits for the same killed public `1-2` NPC `entity_id` to return as a live visible NPC after `KillRespawnDelay`; client e2e Phase10 enemy aggro uses two authenticated browser clients on public `1-3`, a stealthed observer, and a non-stealthed lure to prove the same hostile seed NPC exposes public movement toward the lure inside aggro radius and then no longer targets the lure after a seeded leash break, with return-to-origin public movement when observed | `enemy_spawner`: `TestBootstrapProjectionDoesNotLeakEnemyPoolOrDropProfileInternals`, ownership/cap/forbidden-candidate tests; client e2e Phase10 enemy aggro scans command responses, smoke state, DOM, WebSocket frames, browser storage/cookies, and harness process logs for server-only map/enemy internals including `border_raider_drone_pool`, spawn area, stat/drop/aggro/leash profile ids, leash origin, aggro target memory, and fake/default fixture labels; audit gap: broader cross-map matrix coverage remains open | Starter and destination browser fight proof now exist; Phase09 now includes a real-client visible `1-2` same-entity respawn assertion; Phase10 enemy aggro now includes a focused real-browser public `1-3` aggro/leash proof | Keep second/PvP-map enemy seeds deterministic; server/runtime destination respawn, cap-stability, `1-3` initial spawn/actor projection, seeded passive aggro/leash non-acquisition proof, seeded `1-3` aggressive aggro/leash worker proof, browser-visible `1-2` same-entity respawn proof, and focused browser `1-3` aggro/leash proof now exist. Add broader cross-map combat/loot matrix proof before treating the area as rollout-complete. |
| Map-aware loot/drop | Partial | `internal/game/server/npc_loot_selector.go`, `internal/game/server/npc_loot_selector_test.go`, `internal/game/loot/service.go`, `internal/game/loot/service_test.go` | `combat.use_skill`, `loot.drop_created`, `loot.pickup`, `inventory.snapshot`, `wallet.snapshot` | `npc_loot_selector`: `TestNPCLootSelectorUsesSpawnRecordDropProfileLootTable`, `TestNPCLootSelectorUsesOuterRingSpawnRecordDropProfileLootTable`, `TestNPCLootSelectorAcceptsSeededMapMatrixRows` covers public `1-1`, `1-2`, and medium PvP `1-3`; loot service: `TestCreateDropsForNPCKillRollsServerSideAndIsIdempotent`, `TestPickupDropOwnerLockPublicAndExpiredWindows`; client e2e phase09 proves starter and destination loot pickup reconcile cargo from server response/state | `npc_loot_selector`: `TestNPCLootSelectorRejectsMissingInputsWithoutTrainingFallback`, `TestNPCLootSelectorRejectsOuterRingMissingTableWithoutTrainingFallback`, `TestNPCLootSelectorRejectsMatrixMismatchesWithoutTrainingFallback`, and `TestNPCLootSelectorRejectsSeededMapMatrixMismatchesWithoutStarterFallback` cover destination/PvP seeded map level/risk/table-unavailable/table-source mismatch rejection without starter fallback; loot service: `TestPickupDropRejectsCrossMapViewerWithoutClaim` plus far/hidden/cargo-full, duplicate/concurrent pickup, expired/claimed tests; audit gap: browser rollout proof and broader balance tuning coverage remain incomplete | Phase09 proof includes starter and destination fight/loot pickup | Seeded map selector rows, seeded destination/PvP mismatch guards, and domain cross-map pickup rejection are covered; broader balance tuning and additional rollout/browser proof remain open. |
| Client map UI/protocol | Partial | `client/src/protocol/envelope.ts`, `client/src/state/reducer.ts`, `client/src/render`, `client/src/ui`, `client/src/assets/world`, `client/tests/e2e/phase09-map-flow.mjs`, `docs/map-rework/phase-09-client-map-ui-protocol.md` | `world.snapshot`, `map.snapshot`, `map.changed`, `map.policy_updated`, `portal.enter`, `player.protection_updated`, `combat.use_skill`, `loot.pickup`, `scan.pulse` | client e2e phase09 verifies real auth, starter map `1-1`, real starter fight/loot/cargo, real scan pulse, portal visibility, destination `1-2`, old-map cleanup, destination self visibility, destination fight/loot/cargo after portal, and desktop/tablet/mobile screenshots; Phase10 verifies protected safe-zone PvP through real HUD target selection and Fire click; `world-renderer-assets.test.ts` proves the first world asset set has concrete client URLs for map background, player ship, hostile NPC, laser projectile, loot crate, planet signal, portal gate, safe zone, and radar warning roles; built-client `e2e:playtest-server` asserts player, hostile NPC, portal, and safe-zone sprite assets render from authenticated server state, captures `output/screenshots/ui-implementation/playtest/asset-sprites-desktop.png`, verifies nonblank/diverse screenshot pixels, and completes the playtest loop | Client protocol/reducer forbidden key tests; e2e scans DOM, smoke state, localStorage, sessionStorage, cookies, inbound/outbound WebSocket text frames, production debug rejection responses, captured Go/Vite stdout/stderr lines from the harness, and Tesseract OCR output from the six generated Phase09 screenshots for hidden map/spawn/seed/destination/enemy-pool internals and fixture labels; `client/tests/bundle-scan.mjs` scans the default production bundle plus explicit extra artifact roots for fixture labels/ids and server-only content ids; asset registry tests reject server-only map/scan/drop terms in the client manifest; audit gaps: production logs beyond this harness, admin/debug responses outside this rejection path, non-Phase09 WebSocket paths, and wiring the real deployed/published artifact set into CI/deploy remain broader Phase10 work | `map-origin-{desktop,tablet,mobile}.png` and `map-outer-ring-{desktop,tablet,mobile}.png` under `output/screenshots/ui-implementation/09/`; built-client playtest sprite proof writes `asset-sprites-desktop.png` under `output/screenshots/ui-implementation/playtest/` | Keep Phase09 smoke explicit; this audit leaves it out of `client` `npm run check` to avoid changing routine check cost. |
| No fake/default fixtures | Partial | `client/src/app`, `client/src/protocol/envelope.ts`, `client` bundle scan, `internal/game/server/server_auth_transport_test.go`, `docs/todo.md` | default real mode, `?demo=1` dev-only fixture path, `debug_spawn_npc`, `debug_snapshot` | Phase08J debug/demo spawn quarantine; default authenticated client path uses real Go server/session | Protocol forbidden-payload tests, production debug spawn rejection, default bundle scan for fixture labels/ids and server-only content ids in `dist` plus explicit extra artifact roots; Phase09 e2e DOM/smoke/localStorage/sessionStorage/cookie/WebSocket canary plus production debug rejection response, harness Go/Vite log, and screenshot OCR canaries for its own run artifacts; audit gaps: production logs beyond this harness, admin/debug responses outside this rejection path, CI/deploy passing the real published artifact set to the scanner, and screenshot paths outside Phase09 remain | Phase09 screenshots are real-server screenshots, not fixture screenshots | Do not use demo/fixture screenshots as release proof. Add broader leak canaries before rollout. |
| Rollout/migration controls | Open | `docs/running-local-game.md`, future server config/docs, future persistence migration docs | Proposed flag only: `GAME_FEATURE_BOUNDED_MULTI_MAP`; future migration/backfill jobs | This docs patch defines the runbook; no code flag/test evidence exists today | No durable flag/backfill/quarantine/rollback tests yet | N/A | Current in-memory/dev runtime already routes through bounded multi-map behavior, but production rollout still needs a durable flag/backfill plan if DB persistence is introduced. |

## Known Audit Gaps

- PvP-enabled map seed now exists as public `1-3` / Border Skirmish, reachable
  through the server-owned `1-2` `skirmish_gate` portal, with server-side policy
  coverage for allowed PvP outside safe zones, safe-zone blocking, and
  protection blocking. Domain-level `DeathService.ProcessDeath` coverage proves
  PvP killer ownership, zone cargo policy, and checkpoint/respawn ids are
  preserved in results/events. Runtime coverage now proves lethal seeded-map PvP
  invokes `DeathService.ProcessDeath`, disables the target ship, removes and
  drops real cargo to a killer-owned player-death loot entity, keeps death ids
  out of queued client events, blocks target actions before repair, and repairs
  by snapping the target to the server-selected checkpoint with respawn
  protection and client-safe position/protection events. Phase10 browser
  command-socket proof now covers real target cargo, lethal PvP, attacker-owned
  death cargo drop, repair quote, repair ship, public `1-3` checkpoint
  reconciliation, and respawn protection. Phase10 UI-click proof now selects
  the visible protected player through real HUD target selection, clicks real
  `[data-action="fire"]`, captures outbound UI `combat.use_skill`, and verifies
  inbound `ERR_PVP_BLOCKED`.
- Second/PvP-map enemy seeds exist, starter/destination browser fight/loot proof
  now exists, `TestRuntimeSeedWorldInitializesStarterEnemyPoolThroughSpawner`
  covers initial spawn/actor projection for public `1-1`, `1-2`, and `1-3`,
  `TestRuntimeMapTwoEnemyLifecycleRespawnsThroughMapInstance` covers
  destination server/runtime respawn with stable caps and no starter
  contamination, and `TestSeededPassiveEnemyAggroProfilesDoNotAcquireOrMoveAcrossMaps`
  proves seeded passive `1-1`/`1-2` NPC rows do not acquire or chase a nearby
  player on initial fill. `TestSeededBorderSkirmishAggressiveEnemyAggroLeashUsesCatalogSeed`
  proves seeded public `1-3` aggressive border raider aggro/leash behavior in
  the server worker. Phase09 now includes a browser proof that the same killed
  public `1-2` NPC `entity_id` returns as a live visible NPC after
  `KillRespawnDelay`. Phase10 enemy aggro now includes a focused real-browser
  proof that two authenticated clients reach public `1-3`, a stealthed observer
  sees the seeded hostile NPC near `{x:5400,y:5200}`, a non-stealthed lure
  causes the same NPC to publish movement toward the lure inside aggro radius,
  and a seeded leash break clears the lure target and returns the NPC toward the
  seed origin without exposing pool/spawn/profile internals. Broader per-map
  matrix coverage remains missing.
- No full browser scanner/claim/drop seed matrix. Focused server-only proofs now
  cover seeded destination map `map_1_2` / public `1-2` and PvP map
  `map_1_3` / public `1-3` for claim success,
  `scan.pulse` active-map scan scope/read-model isolation, and domain scanner
  materialization/intel across seeded public maps `1-1`, `1-2`, and `1-3`.
  Browser reducer coverage now preserves server-provided known-planet
  `public_map_key` through known-planets and claim events, keeping bounded
  multi-map planet intel tied to its public map context.
  Server-side drop matrix coverage now includes seeded public maps `1-1`,
  `1-2`, and medium PvP `1-3`. `TestPlayableVerticalServerAuthoritativeLoop`
  now ties a single server-authoritative loop across authenticated `move_to`,
  portal handoff, destination combat/loot, destination scan, planet detail,
  Core claim, production initialization, and route create/settle.
  Claim recovery coverage now also proves duplicate retries after production
  live-state loss queue fresh claim and production summary events carrying the
  recovered server-owned production/storage snapshot.
  `TestPlayableVerticalClaimedPlanetCanSourceRouteSettlement` now proves a
  freshly scanned-and-claimed planet can source an authenticated station route
  settlement through initialized production/storage, including safe
  `route.settled` event reconciliation without exposing the masked station
  endpoint id. A focused browser claim proof now exists for public `1-1`.
  The single-process built-client `e2e:playtest-server` proof now exercises
  `GAME_PLAYTEST_SEED=true` through real browser starter combat/drop/pickup,
  HUD scan, HUD planet claim, production initialization, and route
  create/settle, then transfers through `east_gate` to public `1-2` and proves
  destination-map combat/drop/pickup in the same built-client package with
  DOM/state/storage, WebSocket, and process-log leak canaries. The focused
  built-client `e2e:phase10-pvp-map-drop` proof now registers a normal browser
  player, travels `1-1` -> `1-2` -> `1-3`, resolves browser `scan.pulse`
  successfully on public `1-2` and public `1-3`, kills a Border Skirmish NPC,
  and picks up the server-created `carbon_shards` drop without leaking
  map/scan/drop internals. Broader browser scanner/claim/drop matrix variants
  remain open.
- Browser PvP death/cargo/checkpoint command-socket proof and safe-zone
  UI-click rejection proof exist in
  `client/tests/e2e/phase10-pvp-death-flow.mjs`. The same flow now also runs as
  the single-process built-client `e2e:playtest-server-pvp` proof against
  `cmd/game-server` serving `client/dist` without Vite; remaining PvP browser
  gaps are broader deployed artifact/log canaries.
- Focused scanner rarity/hidden-scan-data regression exists in
  server/discovery tests, Phase10 now has destination/PvP-map server
  scan-scope proof for public `1-2` and `1-3`, domain scanner
  materialization/intel proof across public `1-1`/`1-2`/`1-3`, and Phase09
  now covers one browser scan pulse with DOM/state/storage/cookie leak checks,
  Phase10 now has a focused browser claim proof, and the server playable
  vertical harness proves scan/detail/claim after a real portal transfer. The
  built-client playtest runner now adds one deployable-package browser scan,
  claim, starter drop, `1-1` -> `1-2` portal canary, and destination-map drop
  pickup proof. The focused built-client PvP-map drop proof now covers
  browser scan success on public `1-2` and `1-3` plus the public `1-3`
  combat/drop/pickup path. Broader no-signal browser scan variants and full
  per-map drop matrix variants remain missing.
- Authenticated `route.create`, `route.update`, `route.enable`, and
  `route.disable` now have server gateway slices for owned planet-to-planet MVP
  routes. `planet.production_summary` and `planet.storage_summary` now settle
  eligible owned active-map production using server time before returning safe
  snapshots, queue owner-scoped production/storage reconciliation events only
  when whole-output settlement changes state, no-op immediate/sequential
  duplicate or sub-unit polls without advancing `last_calculated_at`, reject
  spoofed owner/map/time/output/storage/building facts before mutation, and
  leave other-owner/other-map planets untouched. Durable production
  DB/outbox/window idempotency remains open, though production and route domain
  settlement results/events now carry deterministic server-derived
  `reference_key` plus colon-free applied settlement windows and the production
  store now records process-local settlement references plus pending in-memory
  outbox records for applied production and route settlements. Focused server
  coverage proves create derives owner, route id, and
  endpoint map ids server-side; update derives owner from the authenticated
  context, preserves server-owned source truth, changes destination/resource/rate
  through policy validation, settles elapsed old terms before replacement, and
  reconciles active-map production/storage snapshots when settlement touches
  storage; enable/disable derive owner from the authenticated context, toggle
  the stored route through safe response/list/snapshot/event payloads, reconcile
  active-map production/storage snapshots when disable settlement touches
  storage, reject wrong-owner attempts without mutation/events, keep
  stored-route/config errors internal, and reject forged
  owner/map/enabled/settlement/source/destination/storage/risk fields before
  mutation. `route.settle` now derives owner from the authenticated context,
  accepts only `route_id` or `{}` owner reconcile intent, transfers storage
  once, returns immediate duplicate no-op settlements, emits owner-scoped
  `route.settled` plus route reconciliation events without AOI diffs, rejects
  wrong-owner and forged settlement/window/storage/risk/amount facts before
  mutation, and keeps stored-route/config errors internal. Store-level boundary
  coverage proves outbox reads clone event payloads/status/event envelopes and
  duplicate settlement reference reuse no-ops before mutation/events/outbox.
  The focused Phase10 route browser proof now clicks real HUD controls for
  create, update,
  disable, enable, single-route settle, and empty-payload owner reconcile,
  asserts exact outbound safe payload keys, verifies `state.routes`
  reconciliation, and scans browser/log surfaces for route internals. Durable
  DB rows, row locks/CAS, idempotency table enforcement, and outbox publishing
  remain open.
- Full server-side selector mismatch guard is now covered for wrong kill-event
  map/world-zone, NPC type, starter level/rank band, starter risk band, missing
  inputs, missing table cases, and seeded destination/PvP level/risk/table
  unavailable/table-source mismatch cases without starter fallback. Seeded
  cross-map spawn-record mismatch coverage now proves a public `1-2` NPC kill
  event cannot be resolved through the public `1-3` PvP map selector or fall
  back to starter loot. Current positive coverage includes seeded public `1-1`,
  `1-2`, and medium PvP `1-3` matrix rows, domain-level cross-map pickup
  rejection, and
  starter/destination browser fight/loot pickup proof, including the
  single-process built-client playtest package. A focused built-client
  PvP-map browser proof now kills a public `1-3` Border Skirmish NPC and picks
  up `carbon_shards` through the real loot path. Broader balance tuning and
  full matrix rollout proof remain open.
- Phase09 now covers inbound/outbound WebSocket text frames,
  DOM/smoke-state/localStorage/sessionStorage/cookie canaries, production
  `debug_snapshot`/`debug_spawn_npc` rejection responses, captured Go/Vite
  stdout/stderr lines from its own harness, and Tesseract OCR/text scans over
  its six generated real-server screenshot PNGs only. Default production bundle
  text/source-map scan now covers fake/default fixture labels/ids and
  server-only content ids in `dist` plus explicit extra artifact roots. The
  single-process playtest runner now has a `GAME_PLAYTEST_BUILD_ONLY=true`
  build/artifact-scan gate before server startup. Production logs beyond this
  harness, admin/debug responses outside this rejection path, non-Phase09
  WebSocket paths, CI/deploy wiring that passes the real deployed/published
  artifact set, and screenshot paths outside the Phase09 smoke are still
  missing.
- Bundle hidden-token scan remains partial: `client/tests/bundle-scan.mjs`
  checks default `dist` text and source-map assets if present, and can now scan
  explicit extra artifact roots with the same forbidden snippet list through
  either CLI arguments or `GAME_ARTIFACT_SCAN_ROOTS`. The local
  `scripts/run_playtest_server.sh` build-only mode now runs this scan as part
  of the deployable playtest package gate. CI/deploy still needs to pass the
  real deployed or otherwise published artifact set.
- Broader PvP rollout canaries beyond the focused safe-zone UI click proof are
  still missing.
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
- explicit starter, second-map, and PvP-map enemy pools
- explicit scanner and planet claim/drop profiles for a per-map matrix

The focused browser planet-claim proof may set `GAME_E2E_PLANET_CLAIM_SEED=1`
inside its local harness with `GAME_DEV_MODE=1`. Server startup rejects the seed
outside dev mode. That E2E-only seed grants each ensured player exactly one
`x_core` through the real Inventory service and prepares claim eligibility
through the real Progression service with deterministic idempotency references
so the browser can prove `discovery.claim_planet` without adding normal runtime
X Core or rank grant paths. Do not use or document this as a production feature
flag.

The focused browser route proof may set `GAME_E2E_ROUTE_SEED=1` inside its
local harness with `GAME_DEV_MODE=1`. Server startup rejects the seed outside
dev mode. That E2E-only seed creates two owned same-map production planets and
routeable source storage for each ensured player so the browser can prove the
route create/update/control/settle HUD flow without exposing a setup endpoint.
Server gateway coverage also proves that the seeded state supports authenticated
`route.create` followed by elapsed `route.settle` storage transfer.
Do not use or document this as a production feature flag.

The single-process local playtest runner sets `GAME_PLAYTEST_SEED=true`. This
is a test-server onboarding seed, not a production feature flag: each new player
receives one real Inventory X Core, Progression claim eligibility, and two
owned route-test production planets with source storage through the same
server-owned services used by the E2E proofs. It exists so the deployed
playtest loop can reach planet claim and route actions without manual admin
setup.

Production tuning must stay separate from dev/test seeds. Scanner rarity,
enemy spawn density, drop rates, route risk, and PvP rewards should not inherit
forced deterministic smoke values.

### Local Smoke

Run the focused real-server Phase09 bounded-map/portal smoke explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase09-map
```

Run the focused real-server Phase10 PvP death/repair browser proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-pvp-death
```

That command uses the Vite client path. Run the built-client single-server
variant explicitly when validating deploy-like local artifacts:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:playtest-server-pvp
```

Run the focused real-server Phase10 Border Skirmish enemy aggro/leash browser
proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-enemy-aggro
```

Run the focused built-client Phase10 Border Skirmish NPC drop browser proof
explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-pvp-map-drop
```

That proof travels through `1-1` -> `1-2` -> `1-3`, proves browser
`scan.pulse` success on public `1-2` and public `1-3`, kills a public Border
Skirmish NPC, picks up the server-created `carbon_shards` drop, and scans
DOM/state/storage/WebSocket/process-log surfaces for hidden map/scan/drop
internals without Vite.

Run the focused real-server Phase10 planet claim browser proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-planet-claim
```

Run the focused real-server Phase10 automation route browser proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-route
```

Run the single-process built-client playtest proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:playtest-server
```

That proof builds the production client bundle, serves it from `cmd/game-server`
with `GAME_CLIENT_STATIC_DIR=client/dist` and `GAME_PLAYTEST_SEED=true`,
registers a real browser user over the same origin, verifies the playtest X Core
and route-production onboarding seed, then clicks real HUD route create/settle
controls while scanning smoke state, WebSocket frames, browser storage/cookies,
and local server logs for hidden/internal leak tokens.

Run the full built-client playtest vertical-slice gate explicitly:

```bash
scripts/verify_playtest_vertical_slice.sh
```

This chains the playtest build/artifact scan gate, built-client main playtest
loop, built-client PvP/death/repair loop, and destination/PvP scanner plus
Border Skirmish drop canary. Use
`GAME_PLAYTEST_VERIFY_DRY_RUN=true scripts/verify_playtest_vertical_slice.sh`
to print the command sequence without launching the browser proofs.
Last verified locally on 2026-06-23: the full gate passed, including
`e2e:playtest-server`, `e2e:playtest-server-pvp`, and
`e2e:phase10-pvp-map-drop`.

The planet claim proof starts the local Go server with
`GAME_DEV_MODE=1` and `GAME_E2E_PLANET_CLAIM_SEED=1`, registers a real browser
user, scans for a real server-discovered planet, uses the real planet-detail and
Claim HUD controls, and verifies the server-owned claim response,
`planet.claimed` event, production initialization, E2E-seeded X Core inventory
consumption, Progression rank eligibility, pending-command clear, and leak
canaries. It does not capture screenshots.

The route proof starts the local Go server with `GAME_DEV_MODE=1` and
`GAME_E2E_ROUTE_SEED=1`, registers a real browser user, loads two owned
same-map production planets plus routeable source storage through normal
authenticated snapshots, clicks real HUD controls for `route.create`,
`route.update`, `route.disable`, `route.enable`, single-route `route.settle`,
and empty-payload route reconcile, then verifies exact client-safe outbound
payload keys, `state.routes` reconciliation, and leak canaries. It does not
capture screenshots.

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
rejection path, CI/deploy-fed published artifact bundle scans, non-Phase09
WebSocket paths, screenshot paths outside this Phase09 run, or inclusion in
`npm run check`.

The Phase10 PvP proof uses two isolated real browser sessions, command
WebSockets, and the real HUD click path. It proves protected safe-zone PvP is
rejected both through the command socket and through real HUD target selection
plus the real `[data-action="fire"]` UI click. It does not capture screenshots.

The Phase10 enemy aggro proof uses two isolated real browser sessions, command
WebSockets, public portal travel from `1-1` to `1-2` to `1-3`, a stealthed
observer near the Border Skirmish seed area, and a non-stealthed lure. It
proves the same browser-visible hostile seed NPC starts public movement toward
the lure inside aggro radius, then stops targeting the lure and returns toward
the `{x:5400,y:5200}` seed origin after a leash break. It does not capture
screenshots.

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
and concrete server-only map/content ids. It also accepts explicit extra
artifact roots as CLI arguments or through the path-delimited
`GAME_ARTIFACT_SCAN_ROOTS` environment variable, so staging or publish
directories can be scanned with the same forbidden snippet list. It
intentionally does not forbid generic protocol guard field names such as hidden
scan or loot key strings. `GAME_PLAYTEST_BUILD_ONLY=true
scripts/run_playtest_server.sh` now builds `client/dist` and runs the scan
without starting the long-running server. CI/deploy still needs to pass the real
deployed or otherwise published artifact set.

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

PvP death/repair rollout evidence should include the explicit Phase10 browser
proof until the project intentionally wires it into `npm run check`:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-pvp-death
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:playtest-server-pvp
```

Planet claim rollout evidence should include the explicit focused Phase10
browser proof when touching claim UI/protocol/runtime seed behavior:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-planet-claim
```

No release claim should be made from fixture/demo screenshots or client-local
mock state. Open contracts must stay visible in `docs/todo.md` instead of being
masked by placeholder UI data.
