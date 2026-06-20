# Project TODO

Date: 2026-06-17

This file tracks cross-phase follow-ups that should not be lost between Symphony
waves or manual review sessions. Roadmap phase files remain the source of truth
for phase status; this file is a compact pending-work index.

## Open
- [ ] Add owner-aware passive economy fanout for online market, auction, and
  premium viewers. The Task 001 Phase 01 client now refreshes `market.search`,
  `auction.search`, `premium.entitlements`, wallet, inventory, and admin economy
  dashboard snapshots after economy events, but the runtime still queues most
  events only to the acting session. Future backend work must notify affected
  sellers, buyers, previous bidders, winners, entitlement owners, and passive
  shop viewers without leaking private grants, provider refs, or
  viewer-relative auction `leading` state. Source:
  `docs/plans/task-001/01-gameplay-connection-audit.md`.
- [ ] Add a durable reward/outbox reconciliation path for Phase 05 loot XP
  grants; current pickup records in-memory `LootXPReconciliation` metadata but
  there is no durable repair worker or cross-service transaction yet.
- [ ] Map gateway/API request ids to the required
  `CraftingService.StartCraft` domain `ReferenceKey` before exposing craft
  start externally; the Phase 06 domain service is now idempotent when callers
  provide a stable player-scoped `craft_start:*` reference.
- [ ] Replace `RepairService` compensating wallet refunds with a durable
  transaction/outbox boundary when wallet and ship state move out of the
  in-memory Phase 06 slice. Restore failure after debit is currently net-zero
  compensated, but it is not a true atomic rollback. Source:
  `docs/roadmap/06-death-repair-crafting.md`.
- [ ] Move `DeathService.ProcessDeath` from caller-supplied cargo/drop,
  respawn, and drop-policy inputs to authoritative zone inventory, respawn, and
  drop-policy providers before exposing death processing through gateway/runtime
  callers. Equipped module ids are now read from server-owned loadout state.
- [ ] Add durable completion/reconciliation for `CraftingService.CompleteCraft`
  after reservation commit; current in-memory retry path is idempotent, but a
  crash between reservation commit, output grant, XP grant, and job completion
  still needs recovery.
- [ ] Use the Phase 06 `CraftXPObservation` stream to implement or tune
  low-tier craft XP diminishing returns and daily soft caps before public
  economy balancing. The domain hook now records non-duplicate craft XP grants,
  but it does not reduce XP by itself.
- [ ] Wire `production.CraftLocationAuthorizer` into the concrete runtime craft
  service factory before exposing owned-planet or planet-building recipes, and
  add station/special-event station providers for public craft start APIs.
- [ ] Add gateway/security tests for craft start authorization using the
  authenticated server-side player id, including hidden or unowned planet and
  building ids with leak-safe errors.
- [ ] Replace the process-local `CraftingService.CompleteCraft` in-flight guard
  with durable per-job state transitions or row locks plus metrics for
  completion wait time and duplicate retry rate before multi-process runtime
  deployment.
- [ ] Narrow `DeathService` lock scope from the global service mutex to
  per-player/per-reference coordination before death processing moves to a
  higher-concurrency runtime path. `RepairService` now uses scoped in-flight
  coordination for repair attempts and duplicate references.
- [ ] Wire a durable Phase 07 rare reward cap policy into
  `BoardGenerationInput`/`RerollBoardInput` before enabling X Core or premium
  quest rewards in a multi-process runtime. The hook can now block generated
  offers before storage or reroll debit, but no durable usage counter is wired
  by default.
- [ ] Add active-quest indexes plus TTL/compaction or durable uniqueness for
  quest in-memory caches (`progressEvents`, `claimResults`, `rerollResults`)
  before long-running or multi-process deployment. Board offers now have a
  per-player index and stale unaccepted-offer compaction.
- [ ] Apply/release world-worker slow-scan leases atomically with scanner
  cooldown/pulse creation before moving scanner pulses beyond the in-process
  authenticated runtime. The runtime now debits live ship capacitor once per
  accepted pulse reference, and the domain service validates energy availability
  and stationary movement through provider gates. Source:
  `docs/roadmap/08-world-discovery-planets-intel.md`.
- [ ] Replace Phase 08 in-memory discovery stores, idempotency maps, and local
  event slices with durable repositories/outbox records before multi-process
  runtime or DB-backed deployment.
- [ ] Move Phase 08 planet claim into a durable transaction/CAS boundary that
  ties unowned-owner transition, X Core reservation/consume, idempotency, and
  event emission together; current MVP blocks owner overwrite in-memory but
  does not provide cross-process atomicity.
- [ ] Add claim-production initialization recovery to the durable Phase 08/09
  planet claim transaction. Current in-memory flow can repair production state
  on retry, but initializer failure after owner mutation does not recover the
  missing claim event/cache atomically.
- [ ] Add pending/complete or compensation handling around Phase 08 coordinate
  scroll item mint/consume plus metadata/intel writes before using real durable
  economy storage.
- [ ] Add gateway/session authorization for discovery commands so client input
  can only express scan/share/claim/use intents for the authenticated player;
  never accept client-authored coordinates, planet candidates, XP, X Core
  consumption, or scroll metadata.
- [x] Add authenticated browser loadout mutation contracts for
  `loadout.equip_module` and `loadout.unequip_module`. Server handlers must
  resolve player, active ship, slot, owned module instance, rank, compatibility,
  cargo/inventory destination capacity, and idempotency references such as
  `module_equip:<player_id>:<item_instance_id>:<slot_id>` from server context;
  the client needs systems-panel actions that reconcile only from
  `loadout.snapshot`, `inventory.snapshot`, and `stats.snapshot`. Source:
  Phase 10 audit. Implemented in UI Patch 3 Phase 04 with inventory/loadout
  window drag/drop and button fallback.
- [ ] Add authenticated browser crafting mutation contracts for
  `crafting.start`, `crafting.complete`, and `crafting.cancel`. Server handlers
  must map request ids to stable domain references such as
  `craft_start:<player_id>:<recipe_id>:<location_id>`,
  `craft_complete:<job_id>`, and `craft_cancel:<job_id>`, validate materials,
  wallet, rank, location authorization, queue limits, and output capacity, then
  emit crafting/inventory/wallet/progression snapshots after commit. Source:
  Phase 10 audit.
- [ ] Add authenticated planet ownership/building mutation contracts for
  `discovery.claim_planet`, `planet.building_build`, and
  `planet.building_upgrade`. Server handlers must re-check visibility/fog,
  range or claim policy, ownership, required X Core/materials/wallet balance,
  storage capacity, rank/building requirements, and idempotency keys before
  publishing `planet.claimed`, `planet.storage_updated`, and
  `planet.building_updated` events. Source: Phase 10 audit.
- [ ] Add authenticated automation route mutation contracts for `route.create`,
  `route.update`, `route.enable`, `route.disable`, and `route.settle`. Server
  handlers must validate endpoint visibility/access, ownership, route capacity,
  energy/upkeep policy, duplicate settlement windows, and storage capacity, then
  reconcile the browser through `route.list`, `route.snapshot`, and
  `route.updated`/`route.settled` events. Source: Phase 10 audit.
- [ ] Add a real browser death/respawn E2E scenario. Combat or zone-worker
  authority should produce `death.ship_disabled` for the authenticated active
  ship without client-authored damage or death state; the browser can then use
  `death.repair_quote` and `death.repair_ship` and reconcile wallet/ship
  snapshots. Source: Phase 10 audit.
- [ ] Add server-backed mail/social/menu contracts before enabling those topbar
  affordances. The default browser smoke now guards against fake unread mail,
  friend, party, menu, or social notification counts; future contracts should
  define query names, empty states, and role/visibility rules before UI
  indicators are enabled. Source: Phase 10 audit.
- [ ] Add a dedicated browser-client ESLint/style configuration after the Phase
  11 prototype settles. Current client verification has a trust-boundary lint
  script, TypeScript typecheck, Vitest unit tests, Vite production build, and
  Playwright smoke coverage, but no ESLint pass. Source:
  `docs/roadmap/11-browser-client-prototype.md`.
- [ ] Finish wiring Phase 12 observability through remaining concrete
  authenticated gateway command handlers and domain service command paths.
  `ObservedCommandExecutor` records safe realtime command logs/metrics, the
  gateway resolves sessions server-side before handlers run, combat/loot emit
  optional metrics, and market, auction, premium, and quest reward runtime paths
  now record stable metric series. Source: Phase 12 Task 1 and core
  observability wiring.
- [ ] Wire the concrete runtime adapter from discovery
  `ClaimListedIntelStaleMarker` to market/intel listing indexes once coordinate
  scroll listings leave the local domain MVP. Phase 10 now exposes the claim
  hook and `MarketService.MarkListingStale`, but no durable adapter maps
  claimed planets to active market listing IDs yet. Source: Symphony review
  `local-0104`.
- [ ] Narrow lock scope or add per-player/per-planet coordination for Phase 08
  scan, claim, share, and coordinate-scroll services before high-concurrency
  runtime deployment; current MVP services use process-local mutexes.
- [ ] Replace Phase 09 in-memory production/route settlement event envelopes
  with a durable outbox before exposing production and automation routes through
  runtime or gateway callers. Current services append process-local settlement
  events but do not persist or publish them across processes.
- [ ] Add station/storage destination settlement adapters for Phase 09
  automation routes. Current `SettleRoute` supports planet-to-planet storage and
  rejects generic `storage` or `station` destinations with an explicit error.
- [ ] Replace Phase 09 in-memory production, storage, and route repositories with
  durable per-planet/per-route transactions or row locks before multi-process
  runtime deployment.
- [ ] Wire route energy upkeep to authoritative planet energy budget or upkeep
  policy before exposing automation route controls to players. Current route
  creation stores server-policy energy cost but settlement does not reserve or
  consume route energy.
- [ ] Add an authenticated owner/access wrapper before exposing Phase 09
  `SettlePlanetProduction` through gateway/API callers. Route settle, enable,
  disable, and update wrappers now check the server-resolved owner before
  mutation.
- [ ] Replace global Phase 09 production store locking with per-planet/per-route
  coordination before high-concurrency login or inspection settlement. Current
  in-memory MVP intentionally serializes unrelated production and route work.
- [ ] Replace Phase 10 in-memory market settlement serialization with a durable
  wallet/item/listing transaction or outbox-backed recovery path before moving
  market storage out of process. Current `MarketService` prevalidates and holds
  its service lock across wallet and escrow calls, but it is not a persistent
  rollback boundary; Symphony review `local-0103` flagged injected downstream
  wallet or inventory failure after debit as the concrete risk to cover.
- [ ] Formalize the Phase 10 market fee sink account in durable wallet
  provisioning and audit reports. The MVP credits the explicit service-owned
  `market-fee-sink` player id.
- [ ] Replace Phase 10 in-memory auction bid/refund/buy-now serialization with
  a durable wallet/lot transaction or outbox-backed recovery path before moving
  auction storage out of process. Current `AuctionService` prevalidates and
  holds its service lock across wallet calls, but it is not a persistent
  rollback boundary; Symphony review `local-0103` flagged injected downstream
  wallet failure after debit as the concrete risk to cover.
- [ ] Wire Phase 10 auction skeleton payload grants to concrete ship unlock,
  module blueprint, X Core, material, cosmetic, intel, and building blueprint
  adapters once those owning services expose durable grant primitives.
- [ ] Wire Phase 10 paid-only premium bucket policy into future wallet-currency
  market listings before allowing premium currency trades. Current
  `PremiumEntitlementService` exposes `ValidatePaidPremiumUse`, but the fixed
  price market MVP lists items only.
- [ ] Replace Phase 10 premium entitlement skeleton grants with concrete
  loadout-slot, X Core item/claim, cosmetic, and badge adapters once those
  owning services expose durable grant primitives.
- [ ] Add durable premium provider fraud, chargeback, and entitlement revoke
  handling before real payment provider webhooks are enabled. Current provider
  reference uniqueness blocks replay but does not model post-claim clawback.

## Completed

Note: completed items below that cite `client/tests/browser-smoke.mjs` are
historical evidence only. That monolithic file was deleted in `27fb02c`; current
Task 001 release proof must be rebuilt through
`docs/plans/task-001/browser-e2e-rebuild-plan.md`.

- [x] Add a browser-level revoked-session/auth-expiry smoke scenario for Task
  001 Phase 01. Browser smoke now externally revokes a live authenticated
  session, sends `world.snapshot` over the still-open WebSocket, observes the
  terminal auth error/1008 close path, enters `auth_expired`, clears pending
  commands, and removes gameplay state. Source:
  `docs/plans/task-001/01-gameplay-connection-audit.md`,
  `client/tests/browser-smoke.mjs`.
- [x] Finish mockup-level entity asset parity for planets/signals/loot/NPCs.
  Visible objects are selectable and interactive, and the renderer now uses
  mockup-aligned HUD marker language: player radar/ship glow, hostile
  NPC diamond/swarm markers, amber loot cache/crate markers, unknown signal
  question/ring markers, per-kind labels, and minimap entity-type styling.
  Covered by fixture browser smoke and updated screenshots. Source:
  `docs/plans/2026-06-19-ui-rework-GOAL.md`.
- [x] Finish entity selection, combat feedback, and loot pickup presentation in
  the mockup HUD. Visible objects select with reticles, target panels show
  server-safe HP/shield/status, firing produces laser/damage/miss reactions,
  combat buttons show server-owned cooldown/energy availability, loot clicks
  approach or pick up based on server-owned pickup range, and pickup rewards
  reconcile from server events/snapshots only. Covered by reducer, server, and
  browser smoke tests. Source:
  `docs/plans/2026-06-19-ui-rework-GOAL.md`.
- [x] Add server-authoritative continuous movement timing for browser movement.
  The client receives server-owned origin, destination, speed, start, and
  arrival timing; re-clicks while in transit start from the server-computed
  current position; the renderer interpolates movement/parallax instead of
  snapping; immediate move spam is rate-limited without corrupting the
  authoritative route. Source:
  `docs/plans/2026-06-19-ui-rework-GOAL.md`.
- [x] Complete the browser UI rework panel/window registry and reusable modal
  primitive. Cargo, economy, quests, intel/scanner, systems, and admin-only ops
  now open as focused HUD windows from the mockup-style left nav; reusable modal
  details support close button, Escape, and backdrop dismissal; mobile uses a
  bottom-sheet window layout; browser smoke verifies the behavior against real
  authenticated server state. Source:
  `docs/plans/2026-06-19-ui-rework-GOAL.md`.
- [x] Add an indexed wallet ledger/reference lookup for repair refund replay
  checks. `WalletService` now exposes clone-safe reference lookup coverage, and
  `RepairService` uses it for refund replay checks instead of scanning wallet
  histories. Source: `internal/game/economy/wallet_service.go`,
  `internal/game/death/repair.go`.
- [x] Narrow `RepairService` lock scope from the global service mutex to
  per-player/per-reference in-flight coordination, with concurrent repair and
  duplicate-reference tests covering scoped waiting and cache behavior. Source:
  `internal/game/death/repair.go`,
  `internal/game/death/repair_service_test.go`.
- [x] Add owner-checked Phase 09 route operation wrappers for route settlement,
  enable, disable, and update flows. Wrong-owner calls now reject without
  mutating route state. Source: `internal/game/production/route_controls.go`,
  `internal/game/production/route_service.go`,
  `internal/game/production/route_test.go`.
- [x] Spend live runtime scanner capacitor exactly once per accepted scan pulse
  reference. Duplicate scan retries reuse the original spend/result without
  double-debiting ship capacitor, while insufficient capacitor rejects before
  pulse mutation. Source: `internal/game/server/scanner_providers.go`,
  `internal/game/server/server_test.go`.
- [x] Add regression coverage for retried Phase 08 planet claims repairing
  missing production initialization when ownership was already recorded for the
  claimant. Durable transaction/CAS recovery remains tracked separately.
  Source: `internal/game/discovery/claim_test.go`.
- [x] Add per-player board-offer indexes and stale unaccepted-offer compaction
  for the Phase 07 in-memory quest store. Duplicate/reroll/claim/progress
  caches are preserved during compaction. Source:
  `internal/game/quests/store.go`, `internal/game/quests/service_test.go`.
- [x] Guard the default browser against fake mail/social/menu notification
  counts. The real-server smoke now scans unauthenticated, invalid-login,
  authenticated, reconnect, admin, and logout states for enabled fake topbar
  count affordances. Source: `client/tests/browser-smoke.mjs`.
- [x] Wire concrete runtime observability metrics for authenticated market sale,
  auction bid/clearing, premium wallet delta, and quest reward claim paths.
  Metric tests assert stable Phase 12 series and duplicate-safe recording.
  Source: `internal/game/server/economy_handlers.go`,
  `internal/game/server/runtime.go`, `internal/game/server/server_test.go`.
- [x] Add guard tests proving unimplemented browser mutation contracts are not
  registered or visible by default. Realtime, client protocol, and browser smoke
  checks cover crafting, inventory move, progression skill unlock/respec,
  planet claim/building, route mutation, intel share, coordinate item, and
  mail/social mutation ops until real server-owned contracts exist. Source:
  `internal/game/realtime/envelope_test.go`,
  `client/src/protocol/envelope.test.ts`,
  `client/tests/browser-smoke.mjs`.
- [x] Add a concrete Phase 07 quest item reward adapter from
  `QuestRewardInventoryService` to `economy.InventoryService.AddItem`. The
  runtime now wires `questRewardInventoryAdapter` to the concrete
  `InventoryService`, and `TestPhase09QuestAdminObservabilityUseServerState`
  verifies the AddItem ledger row uses the `quest_reward:<player_quest_id>`
  reference and duplicate claims do not create another item ledger entry.
  Source: `internal/game/server/quest_admin_observability_handlers.go`,
  `internal/game/server/server_test.go`.
- [x] Document the preferred quest objective schema shape before the quest API
  becomes public. Public/MVP payloads now specify
  `ObjectiveSchema.Objectives []Objective` as authoritative while legacy
  single-objective fields remain internal/backcompat only, with tests covering
  both the public shape and legacy compatibility. Source:
  `docs/plans/modules/10-quest-board-generation.md`,
  `internal/game/quests/model_test.go`.
- [x] Wire Phase 11 browser combat, loot, scanner, wallet/cargo, and stat
  controls to authenticated Go gateway/runtime handlers. The real browser now
  emits `combat.use_skill`, `loot.pickup`, and `scan.pulse` intents over the
  authenticated Go WebSocket gateway, receives server-owned wallet/cargo/stat
  snapshots, and verifies the flow in the Phase 10 real-server smoke. Source:
  `docs/plans/ui-implementation/10-final-mockup-parity-hardening.md`.
- [x] Replace the Phase 11 browser client's offline demo/local smoke harness
  default with an authenticated Go WebSocket gateway flow and server-owned
  player/session resolution. Default startup now restores `/api/session`, shows
  the auth shell when unauthenticated, connects `/ws` only after login, and the
  default browser smoke boots `cmd/game-server`; the old JavaScript fixture is
  explicit `--fixture` / `?demo=1` fallback only. Source:
  `docs/plans/ui-implementation/03-client-auth-shell-demo-removal.md`.
- [x] Add a Phase 11 WebSocket browser smoke fixture that sends forbidden server
  payload keys and asserts the browser client rejects them without mutating
  smoke-visible client state. The smoke now connects desktop and mobile browser
  viewports to a local WebSocket fixture, requests a reconnect snapshot, sends
  combat, loot, scan, and move intents, checks canvas pixels/layout, and scans
  rendered text plus smoke state for hidden debug data. Source:
  `docs/roadmap/11-browser-client-prototype.md`.
- [x] Add Phase 09 process-local production and route settlement event envelopes
  for production, building output, storage-full, energy-insufficient,
  route-settled, route-loss, source-empty, destination-full, and
  offline-complete conditions. Durable outbox persistence remains tracked
  separately. Source: `docs/roadmap/09-planet-production-routes.md`.
- [x] Wire Phase 08 scanner capacitor/energy availability validation and
  stationary movement gating into the scanner domain service before cooldown,
  pulse, event, planet, intel, or XP mutation. Durable live energy spending and
  slow-scan lease application remain tracked as runtime integration work.
  Source: `docs/roadmap/08-world-discovery-planets-intel.md`.
- [x] Block process-local player-facing cargo transfers while a lethal/death
  transaction is in progress for the player ship. `DeathService` now implements
  the economy cargo transfer guard with short transfer leases for Phase 06,
  while durable/provider-backed death processing remains tracked separately.
- [x] Replace the Phase 05 vertical-slice test-local stat input adapter with
  concrete Phase 03 runtime providers for the in-process backend vertical slice.
  Gateway exposure remains blocked on authenticated session/player resolution.
- [x] Add a zone-worker due-task dispatcher that invokes
  `LootService.HandleScheduledDropTask` from worker ticks instead of requiring
  in-process callers to inspect `TickResult.DueTasks` manually.
- [x] Add in-flight duplicate coordination to `realtime.RequestCache` so
  concurrent duplicate request IDs wait for the first completed response instead
  of executing the handler twice.
- [x] Wire XP grants behind server-owned domain authorities so clients cannot
  spoof XP source completion. `GrantXP` and `GrantRoleXP` now require a
  server-only `XPGrantAuthority` matching the source family before mutation;
  combat, loot, quest, scanner, and crafting grant paths supply their owning
  authority, and future construction/route/event/admin grant paths must do the
  same. Source: `docs/roadmap/03-progression-ships-modules-stats.md`.
- [x] Wire the Phase 03 runtime inventory ledger adapter for module
  equip/unequip. Loadout stores can now call the runtime
  `ModuleInventoryLedgerAdapter`, which batches quiet
  `InventoryService.SystemMoveItemsWithoutEvents` transitions with
  `module_equip:*` and `module_unequip:*` references before committing
  in-memory equipped-module indexes. Source:
  `docs/roadmap/03-progression-ships-modules-stats.md`.
- [x] Map unlocked pilot-skill passive stat effects into runtime stat input.
  `runtime.StatInputProvider` can now read authoritative progression snapshots
  and map every MVP pilot-skill effect into stat aggregation passive buckets,
  including combat, scanner/fog, cargo, craft, construction, and route-capacity
  targets. Source: `docs/roadmap/03-progression-ships-modules-stats.md`.
- [x] Wire realtime gateway request handling to authenticated session and
  server-side player resolution. `realtime.Gateway` now decodes request
  envelopes, resolves `CommandContext` through a server-side session resolver,
  ignores client payload identity such as `player_id`, executes registered
  operation handlers through `ObservedCommandExecutor`, and caches completed
  responses by session/request id. Source:
  `docs/roadmap/04-world-worker-aoi-fog-realtime.md`.
- [x] Add the Phase 05 client timestamp regression around combat intents.
  `combat.use_skill` is now registered and handled by
  `runtime.CombatCommandHandler` for the MVP basic laser; the handler resolves
  the attacker from authenticated server context and ignores
  `client_timestamp` while `CombatService` enforces cooldowns with server time.
  Source: `docs/roadmap/05-combat-loot-vertical-slice.md`.
- [x] Wire disabled active ship state into the realtime combat command path.
  `runtime.CombatCommandHandler` now requires an authoritative active ship guard
  and executes `combat.use_skill` mutations inside the same hangar-owner lease
  used by death disable, so disabled or concurrently disabled active ships cannot
  spend energy, start cooldowns, or deal damage through a stale combat actor.
  Source: `docs/roadmap/06-death-repair-crafting.md`.
- [x] Add craft location ownership/building validation before enabling
  owned-planet or planet-building recipes beyond the current station MVP.
  `CraftingService.StartCraft` now fails closed for planet/building recipes
  without a location authorizer, and `production.CraftLocationAuthorizer`
  validates discovery ownership, production storage initialization, and active
  building state before reservation, wallet debit, or job creation. Source:
  `docs/roadmap/06-death-repair-crafting.md`.
- [x] Add a low-tier craft XP tracking hook for later balancing. Phase 06
  crafting now emits `CraftXPObservation` telemetry for successful,
  non-duplicate craft XP grants and tags rank-1, <=30m recipes as low tier.
  Source: `docs/roadmap/06-death-repair-crafting.md`.
