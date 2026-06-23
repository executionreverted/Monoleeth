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
  event/outbox slices with durable repositories/outbox records before
  multi-process runtime or DB-backed deployment. Phase07M adds process-local
  claim reference records plus claim outbox rows under the claim service lock,
  and Phase07N adds process-local delivery state plus claim-token guards, but
  the rows are not durable or cross-process.
- [ ] Move Phase 08 planet claim into a durable transaction/CAS boundary that
  ties unowned-owner transition, X Core reservation/consume, idempotency, and
  event/outbox emission together. Phase07M now records successful cached claim
  references and pending `planet.claimed` outbox rows in process-local memory,
  including duplicate/conflict/no-boundary-on-failure coverage. Phase07N adds
  process-local pending/in-flight/published/failed delivery state, explicit
  retry, and claim-token publish/fail guards for stale publisher callbacks, but
  durable DB rows, cross-process row locks/CAS, durable idempotency-table
  enforcement, durable outbox persistence, and durable publisher/recovery
  workers remain open. Phase07V preserves typed claim idempotency-key evidence
  on process-local claim reference and outbox rows for canonical gateway
  references, but it does not make those rows durable or transactional.
  Phase07Z adds a store-owned transaction-shaped claim boundary for owner-CAS
  begin plus side-effect completion evidence; the current claim service still
  needs durable DB rows. Phase07AA wires `ClaimService` through that boundary
  so failed side effects leave pending evidence, retries complete it without a
  second X Core consume, and claim-reference conflicts are rejected before
  another consume. Phase07AB moves claim reference, `planet.claimed` event, and
  pending claim outbox creation into the store-owned boundary completion so
  duplicate completion replays or repairs existing artifacts instead of minting
  another event/outbox row, and completed-boundary claim replays after process
  cache loss no longer become already-owned repairs. Phase07AC routes
  `ClaimService` through a `ClaimBoundaryStore` interface so durable DB
  adapters can own begin/complete/read claim boundary operations behind
  row-lock/CAS semantics; the default implementation remains in-memory. X Core
  consumption is still not atomically coupled to DB begin/owner-CAS failure
  handling. Phase07AD adds process-local X Core consumption evidence so a
  transient begin failure after debit can retry without calling the X Core
  consumer again, while conflicting same-reference player/planet attempts are
  rejected before another consume. Phase07AX carries the runtime inventory
  `RemoveItemResult` through `ClaimXCoreConsumeResult` and validates its
  decrease ledger/touched item rows against the claim X Core debit evidence.
  Phase07AY makes that storage mutation plan mandatory for non-empty durable
  claim-begin plans, while preserving debit-only begin-failure recovery
  evidence without pretending owner-CAS committed. Phase07AU adds claim durable
  begin-plan validation for X Core debit evidence plus pending owner-CAS
  boundary, owned planet, and stale-intel evidence. Phase07AT adds claim
  durable commit-plan validation for
  the completed owner-CAS boundary, claim reference, event, pending outbox row,
  and optional X Core debit evidence. Phase07AV adds claim production-init
  durable-plan validation for production recovery evidence tied to
  pending/complete claim boundaries. Phase07AW adds claim durable lifecycle-plan
  validation tying begin, optional production-init, and completion/outbox
  evidence into one coherent completed claim bundle. Phase07BA adds a claim
  durable lifecycle-store adapter contract with idempotent exact replay,
  conflict rejection, claim-reference readback, and a lifecycle-plan handoff
  helper that revalidates nested begin/commit/production-init rows. Phase07BB
  applies completed `discovery.claim_planet` lifecycle bundles through the
  runtime claim lifecycle-store adapter, with duplicate exact-replay and no
  rows for failed claims. Phase07BC adds claim outbox dispatch-plan validation
  plus committed lifecycle-store readback for durable `planet.claimed`
  publisher scheduling. Phase07BI lets the same claim lifecycle-store adapter
  satisfy the claim outbox publisher and lease-reaper contracts for committed
  `planet.claimed` rows with claim-token guards. Phase07BK adds a runtime
  durable outbox drain handoff that can release stale leases and publish
  committed claim lifecycle rows through caller-provided server callbacks.
  Phase07BL adds runtime-owned realtime projection callbacks that recompute
  client-safe claim, known-planets, detail, production/storage, route, and
  inventory snapshots from committed durable rows and flush queued owner events
  during runtime ticks with an event sink.
  Phase07BM couples that drain with filtered per-session event collection so
  committed realtime projections are handed to the sink delivery path instead
  of remaining in the command-event queue after publish. Phase07CN makes the
  claim lifecycle-store publisher/reaper mutations revalidate committed
  lifecycle/outbox readbacks before claiming, publishing, failing, releasing,
  or retrying rows, so corrupt process-local rows fail closed without partial
  worker mutation.
  Durable DB rows, cross-process leases, scheduled publisher workers, and
  cross-process enforcement remain open.
- [ ] Add claim-production initialization recovery to the durable Phase 08/09
  planet claim transaction. Current in-memory flow can repair production state
  on retry, and Phase07W now records process-local claim recovery evidence
  when retries repair already-authoritative ownership after production-init or
  stale-listing failures. Phase07AE records successful production initialization
  evidence, Phase07AV validates that evidence against pending/complete claim
  boundaries, and Phase07BE adds a production-init durable-store adapter
  contract with exact replay, conflict rejection, and committed-plan readback.
  Phase07BF wires successful runtime claim commands to that adapter so
  production-init recovery evidence is committed beside the completed claim
  lifecycle and duplicate retries exact-replay without extra rows. Phase07BG
  also persists pending production-init durable evidence when a claim command
  fails after production initialization but before later side effects, then
  advances that row to complete evidence on successful retry. Phase07BW adds
  runtime/store readback proof that the completed retry evidence is also
  embedded in the committed claim lifecycle plan. These contracts preserve
  evidence by claim reference so later side-effect retries do not call the
  initializer twice. Phase07CC adds deterministic pending production-init
  readback so recovery workers can scan initialized-but-incomplete claim side
  effects without replaying completed rows. Phase07CF tightens the durable
  store/readback contract so production-init rows without pending or complete
  claim-boundary evidence are rejected instead of becoming orphan recovery
  state. Real durable claim/production DB rows, cross-service row locks/CAS, an
  atomic claim/production transaction, and scheduled recovery workers remain
  open.
- [ ] Add pending/complete or compensation handling around Phase 08 coordinate
  scroll item mint/consume plus metadata/intel writes before using real durable
  economy storage. Phase07T now mints and consumes the real account-inventory
  `planet_coordinate_scroll` instance through the inventory service with item
  ledger rows and snapshot fanout. Phase07AG now transfers the server-owned
  intel coordinate payload owner after market purchase with the same market-buy
  idempotency key, so duplicate buy retries can repair a missing transfer and
  buyers can use bought coordinate scrolls once. Coordinate item use now allows
  same-reference domain replay after transport cache misses and restores the
  scroll if the command fails after inventory consume but before intel use; a
  retry cleans up the restored scroll with repair ledger evidence. The
  intel/economy writes are still process-local and not wrapped in a durable
  cross-service transaction.
- [x] Finish gateway/session authorization for remaining discovery commands.
  `scan.pulse` and the Phase07A backend `discovery.claim_planet` handler now
  resolve the authenticated player server-side and reject client-authored
  coordinates, planet candidates, XP, map/position truth, and X Core
  consumption. Phase07S adds authenticated `intel.share`,
  `intel.coordinate_item.create`, and `intel.coordinate_item.use` handlers
  that derive sender/source intel/item payloads server-side and reject
  client-authored coordinates, ownership, source, confidence, timestamp, and
  inventory truth.
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
- [ ] Finish authenticated planet ownership/building mutation contracts.
  Phase07A landed the backend `discovery.claim_planet` handler with
  authenticated player resolution, active-map range checks, rank validation,
  one-X-Core inventory idempotency, production initialization, and owner-scoped
  `planet.claimed` fanout. Phase10 now has client protocol/HUD/reducer wiring
  and a focused real-browser claim proof that sends only `planet_id`, uses an
  E2E-only Inventory X Core plus Progression rank seed, reconciles
  production/inventory, and handles `planet.claimed` without an unhandled-event
  log. Phase07M adds a process-local claim reference/outbox boundary for
  successful cached claim results, and Phase07N adds process-local claim
  outbox delivery state plus claim-token guards. Phase07O adds a
  process-local production-domain foundation for building build/upgrade:
  catalog-backed active building creation, next-level upgrades, material debits
  from `PlanetStorage`, production-local material ledger rows, optional wallet
  debit adapter ordering, duplicate reference replay, and
  `planet.storage_updated` / `planet.building_updated` events through the
  in-memory outbox path. Phase07P adds authenticated
  `planet.building_build` and `planet.building_upgrade` gateway handlers with
  server-derived owner, active-map scope, deterministic building ids,
  catalog-backed definitions, material/wallet costs, idempotency references,
  spoof-field rejection, duplicate replay, and owner-scoped production/storage/
  wallet reconciliation. Phase07Q tags building storage/building outbox rows
  with the committed building mutation idempotency reference and proves the
  publisher state machine preserves that evidence. Phase07AN adds a
  process-local durable commit-store adapter for successful authenticated
  building mutations; the gateway now hands the mutation reference, pending
  outbox rows, and production-local material ledger rows to that boundary,
  rejects missing references/conflicting replays, and keeps duplicate requests
  from appending durable rows. Phase07BD adds building mutation outbox
  dispatch-plan validation and committed durable-store readback for future
  `planet.building_updated` publisher scheduling. Phase07BJ lets the building
  mutation durable commit-store adapter satisfy the production outbox publisher
  and lease-reaper contracts for committed storage/building outbox rows.
  Phase07CO makes those building publisher/reaper mutations revalidate
  committed reference/outbox/material-ledger readbacks before claiming,
  publishing, failing, releasing, or retrying rows, so corrupt process-local
  building durable rows fail closed without partial worker mutation. Building
  mutation duplicate retries now rebuild the durable commit plan from the
  canonical production reference evidence, so a missing durable building commit
  can be repaired without repeating material or wallet debits.
  Phase07R starts the
  server-authoritative intel/coordinate domain foundation for share,
  coordinate-item creation, coordinate-item use, canonical idempotency keys,
  and consume-once item state. Phase07S wires those intel operations into the
  authenticated realtime gateway and TypeScript protocol with discovery
  read-model/event reconciliation. Phase07T backs coordinate scroll
  create/use with real inventory instances, item ledger rows, and inventory
  snapshot fanout. Durable DB/outbox claim recovery, cross-process CAS/locks,
  idempotency-table enforcement, durable outbox persistence/publisher workers,
  broader building requirement/cost balancing, browser HUD controls, coordinate
  item durable transaction/compensation, market/listing staleness hooks, and
  intel quotas remain open. Phase07Z adds a store-owned begin/complete claim
  boundary row that models the future owner-CAS plus pending-side-effects DB
  transaction; Phase07AA wires `ClaimService` through that boundary while
  Phase07AB records reference/event/outbox completion artifacts under the same
  store lock and replays completed boundaries without duplicate repair side
  effects. Phase07AC adds the claim boundary adapter contract and retry/error
  coverage around repository read/complete failures, but not yet X Core debit
  rollback/recovery for begin failures. Phase07AD records process-local X Core
  consumption evidence for begin-failure retries, but durable DB rows, rollback
  or compensation, and recovery workers remain open. Phase07AE adds
  process-local production-init evidence for later side-effect retries, while
  durable production recovery rows remain open.
  Source: Phase 10 audit, Phase07A, Phase07O, Phase07P, Phase07Q, Phase07R,
  Phase07S, and
  `docs/map-rework/phase-10-testing-rollout.md`.
- [ ] Finish durable authenticated automation route persistence and rollout
  hardening. Server/browser work now has a focused real-client
  create/update/control/settle proof; remaining work is durable route rows,
  endpoint visibility/access policy beyond the owned same-map MVP, route
  capacity progression/balancing, durable route energy/upkeep DB adapter
  rollout, durable settlement idempotency table enforcement, storage capacity
  persistence, and outbox reconciliation through `route.list`,
  `route.snapshot`, and `route.updated`/`route.settled` events.
  Phase07B map-tagged the route domain rows and read payloads for
  `route.list`/`route.snapshot`; Phase07C landed authenticated `route.create`
  as an owned planet-to-planet gateway slice that rejects client-authored
  owner/map/energy/risk fields, derives owner/route id/map ids server-side, and
  queues owner-scoped safe route events. Phase07D landed authenticated
  `route.enable` and `route.disable` as owned-route gateway controls that
  accept only `route_id`, derive owner server-side, reject spoofed route facts
  and wrong-owner attempts without mutation/events, and queue owner-scoped safe
  route events. Disable also reconciles active-map production/storage snapshots
  when server-owned settlement touches storage. Phase07E landed authenticated
  `route.update` as an owned-route gateway mutation that accepts only
  `route_id`, `destination_planet_id`, `resource_item_id`, and
  `amount_per_hour`, derives owner server-side, preserves source truth from the
  route row, rejects spoofed route facts, wrong-owner attempts, and
  X Core/non-routeable resources without mutation/events, queues owner-scoped
  safe route events, and reconciles active-map production/storage snapshots
  when update settlement touches storage. Phase07F landed authenticated
  `route.settle` as an owned-route gateway mutation that accepts only
  `route_id` or `{}` owner reconcile intent, derives owner server-side, rejects
  spoofed settlement/window/storage/risk/amount/resource facts and wrong-owner
  attempts without mutation/events, returns safe settlement payloads, queues
  owner-scoped `route.settled` plus route reconciliation events without AOI
  diffs, and reconciles active-map production/storage snapshots when settlement
  touches storage. Phase07G added browser protocol builders, HUD controls,
  reducer reconciliation, a dev-only guarded `GAME_E2E_ROUTE_SEED`, and the
  focused real-browser `e2e:phase10-route` proof for route
  create/update/disable/enable/settle/reconcile. Phase07I added
  server-derived route settlement reference/window evidence to domain results
  and outbox-safe domain payloads while leaving browser-safe realtime payloads
  unchanged. Phase07J added process-local store-owned settlement references
  and pending in-memory outbox records for applied route settlements, including
  duplicate reference no-op guards and cloned read APIs. Phase07K narrows
  in-process publisher/retry behavior with pending, in-flight, published, and
  failed outbox states plus explicit retry, but does not make those records
  durable or cross-process. Phase07AN adds an explicit route settlement
  transaction boundary that future DB adapters can implement for owner-scoped
  settlement window idempotency, storage ledger writes, and pending outbox rows
  under one commit. Phase07BN adds the separate automation route durable-row
  contract with idempotency references, revision CAS, exact replay, stale
  revision rejection, conflict rejection, detached readback, and owner-scoped
  route recovery queries. Phase07BO wires runtime route create/update/enable/
  disable to write durable route-row snapshots with server-derived references
  and revision advancement under the production store lock. Phase07BP wires
  pure route settlement cursor advancement to write durable route-row snapshots
  with the server-derived `route_settlement:<route_id>:<window>` reference
  under that same store lock. Phase07BQ makes route settlement durable commit
  bundles require that committed route-row snapshot beside settlement evidence,
  route ledger rows, and pending outbox rows, while production settlement
  bundles reject route rows. DB adapters still need to co-commit route rows
  with settlement evidence, storage ledger rows, and outbox rows where
  mutations settle old terms. Phase07BR adds a server-owned per-player MVP
  route-slot cap to `route.create`, rejects client-authored route count/capacity
  fields, and preserves existing-route updates at cap. Phase07BU moves
  `route.create` through an explicit route-create transaction boundary that
  rechecks owner route-slot capacity under the insert lock before committing
  the route row, route-create idempotency reference, source energy reservation,
  and durable route record; real DB row locks/CAS and durable idempotency-table
  enforcement remain open.
  Phase07BS wires enabled route upkeep into the source planet production energy
  budget in the in-memory store: create/enable reserve energy, disable releases
  after settlement, and update applies the enabled-route energy delta while
  preserving same-cost edits at capacity. Durable DB adapters still need to
  co-commit route rows with source production-state energy reservations.
  Phase07BT adds that source production-state row to route durable commit plans
  and records, and the in-memory production store applies it under the same lock
  as the durable route row; real DB row locks/CAS remain open.
  Route durable recovery/idempotent readbacks now revalidate committed route
  rows, reference rows, revision evidence, reference-to-route-row consistency,
  and source production energy evidence before returning them to callers, so
  corrupt in-memory durable rows fail closed instead of being used as recovery
  truth.
  Phase07BV adds a focused runtime proof that a committed route settlement
  durable outbox row drains into owner-scoped realtime `route.settled`,
  route snapshot/list, production, and storage events without leaking to another
  active session. Phase07CD adds committed production-state and storage-row
  evidence to production settlement durable commit bundles, and ledger-backed
  changed storage-row evidence to route settlement durable commit bundles
  beside route-row, route-ledger, reference, and outbox evidence.
  Durable DB rows, row locks/CAS, idempotency table enforcement, and durable
  outbox publishing remain open.
  Source: Phase 10
  audit, Phase07C, Phase07D, Phase07E, Phase07F, Phase07G, Phase07I, Phase07J,
  Phase07K, Phase07AN, Phase07BN, Phase07BO, Phase07BP, Phase07BQ,
  Phase07BR, Phase07BS, Phase07BT, Phase07BV, and Phase07CD.
- [ ] Complete the remaining Phase10 PvP rollout matrix. The deterministic
  catalog now includes public `1-3` / Border Skirmish as a PvP-enabled seed,
  reachable through the server-owned `1-2` `skirmish_gate` portal, and server
  tests cover allowed PvP outside safe zones plus safe-zone and protection
  blocking on that seeded map. Death-domain coverage now proves
  `DeathService.ProcessDeath` preserves a PvP killer, killer-owned loot drop,
  zone cargo policy, and checkpoint/respawn id in results/events. Runtime
  coverage now proves lethal seeded-map PvP invokes `DeathService.ProcessDeath`
  with server-owned target, cargo, killer, zone, and checkpoint data, disables
  the target ship, creates killer-owned player-death cargo drops, keeps death
  internals out of queued client events, blocks target actions before repair,
  and repairs by snapping the target to the server-selected checkpoint with
  respawn protection and client-safe position/protection events. Phase10 browser
  command-socket coverage now proves two real authenticated browser sessions,
  real target `raw_ore` cargo from NPC loot, public portal travel
  `1-1` -> `1-2` -> `1-3`, lethal PvP `combat.use_skill`, attacker-visible
  death cargo drop, target `death.repair_quote` / `death.repair_ship`, repaired
  public `1-3` checkpoint/protection reconciliation, protected/safe-spawn
  `ERR_PVP_BLOCKED`, disabled-action `ERR_SHIP_DISABLED`, and focused
  death/repair leak canaries. The same Phase10 browser proof now selects the
  visible protected player through real HUD target selection, clicks
  `[data-action="fire"]`, captures outbound UI `combat.use_skill`, and verifies
  inbound `ERR_PVP_BLOCKED`. Remaining work is broader rollout canaries. Source:
  `docs/map-rework/phase-10-testing-rollout.md`.
- [ ] Complete second/PvP-map enemy rollout coverage. The deterministic
  `outer_ring_scout_drone` and `border_raider_drone` seeds, initial spawn,
  actor projection for public `1-1`/`1-2`/`1-3`, bootstrap no-hidden-pool leak
  check, server/runtime destination respawn proof with stable spawn caps, and
  seeded passive `1-1`/`1-2` server aggro/leash non-acquisition proof now
  exist. Seeded public `1-3` aggressive border raider aggro/leash behavior is
  covered by both the server/worker proof and a focused Phase10 real-browser
  proof that uses a stealthed observer plus non-stealthed lure to verify public
  NPC chase and leash-reset movement without exposing pool/spawn/profile
  internals; Phase09 now proves starter and destination browser fight/loot
  pickup after portal handoff and includes a same-entity public `1-2` browser
  respawn assertion after `KillRespawnDelay`. Remaining work is broader matrix
  coverage. Source:
  `docs/map-rework/phase-10-testing-rollout.md`.
- [ ] Add a deterministic per-map scanner/claim/drop seed matrix. Focused
  server/discovery scanner rarity/hidden-scan-data regression coverage now
  exists, Phase09 now covers one browser `scan.pulse` with
  DOM/state/storage/cookie leak checks, server-only claim success covers seeded
  destination map `map_1_2` / public `1-2` and PvP map `map_1_3` / public
  `1-3`, and server-only scanner scope proof now shows active `map_1_2` and
  `map_1_3` sessions keep `scan.pulse`
  response/event/read-model memory on public `1-2` and `1-3`. Domain scanner
  materialization/intel now covers seeded public maps `1-1`, `1-2`, and `1-3`,
  and server-only drop matrix coverage now includes public `1-1`, `1-2`, and
  medium PvP `1-3`. Phase10 now adds a focused browser claim proof for public
  `1-1`; remaining work is browser drop flow plus broader browser claim/drop
  and scan success/no-signal variants. Source:
  `docs/map-rework/phase-10-testing-rollout.md`.
- [ ] Complete broader per-map/risk/rank drop balance matrix coverage across
  seeded maps. Current server tests cover starter selection, `map_1_2`
  `outer_ring_scout_drone` spawn-record/drop-profile selection, accepted
  starter, destination, and medium PvP `1-3` matrix rows, full selector mismatch
  guards for wrong map/world-zone, NPC type, level/rank band, risk band,
  missing inputs, and missing table cases without starter fallback.
  Phase10 now also rejects seeded destination/PvP level/risk/table-unavailable
  and table-source mismatches without falling back to starter loot, and
  domain-level hidden/far/cross-map pickup rejection remains covered. Phase09
  now covers starter and destination browser fight/loot pickup; broader balance
  tuning and additional rollout/browser proof remain open. Source:
  `docs/map-rework/phase-10-testing-rollout.md`.
- [x] Extend the Phase09 browser map smoke into a full real-server
  fight/loot/scan/portal loop with desktop, tablet, and mobile screenshots,
  including destination-map fight/loot pickup after portal handoff. The command
  remains explicit as `e2e:phase09-map` and is not wired into `client`
  `npm run check` to avoid changing routine check cost. Source:
  `docs/map-rework/phase-10-testing-rollout.md`.
- [ ] Complete remaining Phase10 leak canaries over production logs beyond the
  Phase09 harness, admin/debug responses outside the normal-session
  `debug_snapshot`/`debug_spawn_npc` rejection path, CI/deploy-fed published
  bundle artifacts, non-Phase09 WebSocket paths, and screenshot artifacts
  outside the Phase09 smoke for hidden map/scan/spawn/loot internals and
  fake/default fixture labels. Phase09 now covers DOM/app state, local/session
  storage, cookies, inbound/outbound app plus command WebSocket text frames,
  production debug rejection responses for those two ops, captured Go/Vite
  stdout/stderr lines from its own harness, and Tesseract OCR over its six
  generated real-server screenshot PNGs, and the bundle scan covers
  fake/default fixture labels/ids plus server-only content ids in default
  `dist` and explicit extra artifact roots. Production logs/admin responses
  outside that path, non-Phase09 paths, wiring the real deployed/published
  artifact set into CI/deploy, and screenshots outside Phase09 remain open.
  Source:
  `docs/map-rework/phase-10-testing-rollout.md`.
- [x] Clean up active legacy semantic contradictions in the scoped
  world/progression/module/UI docs. The bounded-map rework now defines active
  maps as `0..10000`, current-map membership, radar/stealth visibility, known
  planet intel memory, portal travel, and per-map risk/profile tuning in the
  touched design docs. The older world-system summary and architecture notes
  now describe bounded maps, portals, known intel, and radar/stealth filtering
  instead of an infinite origin plane or client fog-of-war reveal. Remaining
  old-term search hits should be limited to superseded labels, legacy file
  paths, or historical notes. Phase07CP updates active UI Patch 3, Task 001,
  UI implementation, combat, loot, intel/coordinate, and GOAL references so
  visual fog is inactive and radar/stealth plus known-intel memory is the
  current-map model. Source:
  `docs/map-rework/phase-10-testing-rollout.md`.
- [ ] Finalize production bounded multi-map rollout controls if DB persistence
  is introduced: implement or document the future
  `GAME_FEATURE_BOUNDED_MULTI_MAP` flag, deterministic seed selection,
  backfill/quarantine commands, rollback order, and no-silent-clamp migration
  checks. Source: `docs/map-rework/phase-10-testing-rollout.md`.
- [x] Add a real browser death/respawn E2E scenario. The explicit
  `e2e:phase10-pvp-death` command starts the real Go server and Vite client,
  registers two isolated real browser sessions, moves both through public
  portal intents to `1-3`, produces `death.ship_disabled` through
  server-authoritative PvP combat without client-authored damage/death state,
  then uses `death.repair_quote` and `death.repair_ship` and reconciles
  repaired ship, checkpoint position, and respawn protection in browser smoke
  state. Source: Phase 10 audit.
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
- [x] Wire the concrete runtime adapter from discovery
  `ClaimListedIntelStaleMarker` to market/intel listing indexes once coordinate
  scroll listings leave the local domain MVP. Phase07AF now wires the runtime
  claim hook to active/stale `planet_coordinate_scroll` market listings by
  resolving the server-owned coordinate item instance through the intel service
  and marking matching claimed-planet listings stale via
  `MarketService.MarkListingStale`; stale listings are no longer buyable.
  Durable planet-to-listing indexes remain tracked by the broader durable
  persistence/outbox tasks. Source: Symphony review `local-0104`, Phase07AF.
- [ ] Narrow lock scope or add per-player/per-planet coordination for Phase 08
  scan, claim, share, and coordinate-scroll services before high-concurrency
  runtime deployment; current MVP services use process-local mutexes.
- [ ] Replace Phase07J/Phase07K/Phase07L in-memory production/route settlement reference
  and outbox delivery boundary with durable DB rows, idempotency-table
  enforcement, and a publisher/retry worker before multi-process production
  deployment.
  Phase07I added server-derived reference/window evidence to production and
  route domain results plus outbox-safe domain payloads. Phase07J now records
  process-local settlement references and pending outbox rows under the same
  store lock as state mutation/event append. Phase07K now narrows
  process-local publisher/retry behavior with pending, in-flight, published,
  failed, retry, and lease release state. Phase07AN adds a DB-adapter-ready
  route settlement transaction contract for owner-scoped route windows, but
  Phase07AO adds the matching production settlement transaction contract for
  planet production windows. Phase07AP adds the matching claim X Core
  debit-plus-owner-CAS begin contract for future durable claim/storage adapters.
  Phase07AX adds X Core storage-mutation durable-plan validation over the
  runtime inventory remove result, ledger entry, touched item rows, and claim
  debit evidence. Phase07AY wires that storage-mutation evidence into durable
  claim-begin plans so raw X Core consumption alone no longer satisfies a
  non-empty begin transaction contract.
  Phase07AQ adds after-commit settlement outbox dispatch-plan validation for
  future durable publisher scheduling. Phase07AR adds durable settlement
  commit-plan validation that ties settlement idempotency references, outbox
  dispatch rows, and route storage ledger rows together. Phase07AS exposes that
  validated plan directly from production and route settlement transaction
  results. Phase07AZ adds the durable settlement commit-store adapter contract
  plus transaction-result and durable-plan handoff helpers so future DB
  adapters can accept the validated reference/outbox/route-ledger bundle with
  idempotent exact replay and conflict rejection, plus readback helpers so
  recovery/publisher workers can rebuild committed durable commit and outbox
  dispatch plans by settlement reference key. Phase07BH adds window-based
  readback for production and route settlement commit/dispatch plans and lets
  the durable settlement commit-store adapter satisfy the production outbox
  publisher/lease-reaper contracts for committed settlement outbox rows. The
  runtime production/storage summary handlers now hand server-owned production
  settlement transaction rows to the durable commit-store adapter while
  duplicate/sub-unit polls remain no-op, and route settlement handlers now hand
  server-owned route settlement references, pending outbox rows, and route
  storage ledger rows to the same durable commit-store adapter. Phase07AU adds
  claim-begin durable plan
  validation for X Core debit plus owner-CAS evidence. Phase07AT adds the
  matching completed-claim durable commit-plan validation for claim
  boundary/reference/event/outbox/X Core evidence. Phase07AV adds claim
  production-init durable-plan validation for recovery rows tied to
  pending/complete claim boundaries. Phase07BE adds the production-init
  durable-store adapter contract for committing those recovery rows before the
  full lifecycle is complete. Phase07AW ties those claim
  begin/init/commit plans into one completed lifecycle validation helper.
  Phase07BA adds a claim durable lifecycle-store adapter contract with
  idempotent exact replay, conflict rejection, claim-reference readback, and a
  lifecycle-plan handoff helper that revalidates nested
  begin/commit/production-init rows. Phase07BB applies completed claim
  lifecycle bundles through the runtime claim lifecycle-store adapter after
  successful authenticated claim commands, without duplicate rows on retries.
  Phase07BC adds claim outbox dispatch-plan validation and committed
  lifecycle-store readback for future durable `planet.claimed` publisher
  scheduling. Phase07BI adds claim lifecycle-store publisher and lease-reaper
  contract support for committed `planet.claimed` outbox rows. Phase07BK adds a
  runtime durable outbox drain handoff for committed claim, settlement, and
  building mutation rows. Phase07BL wires those committed rows to server-owned
  client-safe realtime projections without exposing raw durable payloads.
  Phase07BM adds a drain-and-collect helper used by runtime sink delivery so
  those safe projections are flushed when the durable row is marked published.
  Phase07CD adds committed production-state and storage-row evidence to
  production settlement durable commit bundles, plus ledger-backed changed
  storage-row evidence to route settlement bundles, with detached readback and
  exact replay conflict validation. Phase07CE lets claim lifecycle,
  settlement/route, and building mutation durable-store readbacks rebuild
  committed plans after publisher delivery-state changes while returning the
  current outbox evidence. Phase07CG makes settlement durable outbox worker
  paths revalidate committed settlement bundles before claim, publish/fail,
  lease release, or retry mutation so corrupt outbox evidence cannot be handed
  to publishers.
  Durable claim, production, route settlement, and building mutation tables
  plus publisher scheduling remain open.
  Those records are still not durable, cross-process, or delivered by a durable
  publisher process. Phase07L adds
  process-local claim tokens so publish/fail callbacks require the current
  in-flight attempt and stale callbacks cannot mutate retried or reclaimed
  attempts. Phase07U adds interface-backed publisher drain helpers for claim
  and production-domain outbox records, covering production settlements, route
  settlements, and building mutations while preserving claim-token
  publish/fail semantics behind a DB-adapter-ready contract. Phase07X adds
  process-local in-flight lease expiry release back to pending for claim and
  production/route settlement outbox rows. Phase07Y exposes that stale-lease
  release through small interface-backed helper contracts for future DB
  adapters and scheduled reaper workers. Durable DB rows, row locks/CAS,
  cross-process leases, idempotency-table enforcement, and a scheduled durable
  publisher still need to preserve that semantic across processes.
- [ ] Add station/storage destination settlement adapters for Phase 09
  automation routes. Phase07BX adds the storage-destination MVP adapter:
  domain route create/update can pass `storage` endpoints through the policy
  boundary, and route settlement transfers into the named storage aggregate
  with normal route ledger, durable route-row, reference/window, and outbox
  evidence. Phase07BY extends the same adapter contract to `station`
  destinations while adding public gateway guardrails so browser route
  create/update still cannot author non-planet endpoints. Phase07BZ proves
  existing owner-scoped storage/station routes can settle through the
  authenticated `route.settle` gateway with safe payloads/events, masking
  storage/station aggregate IDs while preserving public destination type, and
  durable settlement evidence. Phase07CA extends that safety proof through
  durable outbox realtime replay so replayed route events keep non-planet
  aggregate IDs masked. Phase07CB adds explicit failed-row retry contracts for
  claim, settlement/route, and building durable outbox rows so transient
  publisher failures can be returned to pending without losing failure evidence.
  Phase07CD adds ledger-backed changed storage-row evidence to route settlement
  durable bundles. Phase07CG makes settlement durable outbox worker paths
  revalidate committed settlement bundles before claim, publish/fail, lease
  release, or retry mutation so corrupt outbox evidence cannot be handed to
  publishers; DB-backed storage/station endpoint rows and row locks remain open.
  Runtime/browser non-planet route create/update access policy and durable
  DB-backed storage/station endpoint rows remain open.
- [ ] Replace Phase 09 in-memory production, storage, and route repositories with
  durable per-planet/per-route transactions or row locks before multi-process
  runtime deployment.
- [ ] Carry route energy upkeep reservations into the future durable route DB
  adapter transaction. Phase07BS wires enabled route upkeep into the in-memory
  source planet production energy budget, and Phase07BT records the changed
  source production state in route durable commit plans/records, but DB
  adapters still need real row locks/CAS for route rows plus source
  production-state reservation changes.
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
  including combat, scanner/visibility, cargo, craft, construction, and route-capacity
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
