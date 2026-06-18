# Project TODO

Date: 2026-06-17

This file tracks cross-phase follow-ups that should not be lost between Symphony
waves or manual review sessions. Roadmap phase files remain the source of truth
for phase status; this file is a compact pending-work index.

## Open

- [ ] Wire realtime gateway request handling to authenticated session and
  server-side player resolution before exposing Phase 04 worker commands over
  WebSocket. Source: `docs/roadmap/04-world-worker-aoi-fog-realtime.md`.
- [ ] Replace the Phase 11 browser client's offline demo harness with an
  authenticated WebSocket gateway flow, including reconnect snapshot request
  and server-authoritative player resolution. Source:
  `docs/roadmap/11-browser-client-prototype.md`.
- [ ] Add in-flight duplicate coordination to `realtime.RequestCache` when the
  gateway executes mutating commands concurrently; the current cache only
  remembers completed responses.
- [ ] Wire XP grants behind concrete domain owners such as quest, scanner,
  production, crafting, route, event, and admin services so clients cannot spoof
  XP source completion. Combat NPC kill XP and eligible loot pickup XP now have
  Phase 05 domain boundaries; remaining XP sources still need owners. Source:
  `docs/roadmap/03-progression-ships-modules-stats.md`.
- [ ] Wire the remaining Phase 03 runtime inventory ledger adapter for module
  equip/unequip. Rank/role-gate, module-aware stat input, and effective
  cargo-capacity providers exist under `internal/game/runtime`.
- [ ] Map unlocked pilot-skill passive stat effects into runtime stat input.
  The stat aggregation model has passive buckets, but current runtime providers
  compose base ship and equipped module stats only.
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
- [ ] Add an indexed wallet ledger/reference lookup for repair refund replay
  checks before wallet histories become large; the current in-memory repair
  compensation path scans ledger entries under service lock.
- [ ] Move `DeathService.ProcessDeath` from caller-supplied cargo/drop/loadout
  inputs to authoritative zone inventory, loadout, respawn, and drop-policy
  providers before exposing death processing through gateway/runtime callers.
- [ ] Block or serialize cargo transfers while a lethal/death transaction is in
  progress for the player ship; current death processing is retry-safe but does
  not lock live cargo movement.
- [ ] Wire disabled ship state into combat runtime actor ownership so a stale
  combat actor cannot attack after `DisableActiveShipForDeath`.
- [ ] Add durable completion/reconciliation for `CraftingService.CompleteCraft`
  after reservation commit; current in-memory retry path is idempotent, but a
  crash between reservation commit, output grant, XP grant, and job completion
  still needs recovery.
- [ ] Add craft location ownership/building validation before enabling
  owned-planet or planet-building recipes beyond the current station MVP.
- [ ] Wire `CraftLocationAuthorizer` to authoritative runtime station, planet,
  and building ownership providers before exposing non-station craft recipes;
  the Phase 06 service now has a pre-mutation hook, but no runtime provider is
  connected yet.
- [ ] Add gateway/security tests for craft start authorization using the
  authenticated server-side player id, including hidden or unowned planet and
  building ids with leak-safe errors.
- [ ] Replace the process-local `CraftingService.CompleteCraft` in-flight guard
  with durable per-job state transitions or row locks plus metrics for
  completion wait time and duplicate retry rate before multi-process runtime
  deployment.
- [ ] Narrow DeathService and RepairService lock scope from global service mutex
  to per-player/per-reference coordination before these services move to a
  higher-concurrency runtime path.
- [ ] Add a concrete Phase 07 quest item reward adapter from
  `QuestRewardInventoryService` to `economy.InventoryService.AddItem` once the
  quest reward item-definition catalog/provider is wired; current claim tests
  prove the quest boundary and idempotency reference but use fakes.
- [ ] Enforce rare reward caps before enabling rare quest rewards such as X Core
  or premium rewards; Phase 07 currently stores rare-cap hooks as policy markers
  and leaves the actual cap check unchecked in the roadmap.
- [ ] Make quest board queries expiry-aware before exposing them through
  gateway/client APIs; `AcceptQuest` rejects expired offers, but `BoardOffers`
  currently returns stored unaccepted offers without a clock-aware expiry filter.
- [ ] Collapse or document the preferred quest objective schema shape before the
  quest API becomes public; `ObjectiveSchema` currently supports both
  `Objectives []Objective` and legacy single-objective fields.
- [ ] Add per-player offer/active-quest indexes plus TTL/compaction or durable
  uniqueness for quest in-memory caches (`progressEvents`, `claimResults`,
  `rerollResults`) before long-running or multi-process deployment.
- [ ] Wire Phase 08 scanner capacitor/energy validation and slow/stationary scan
  state before exposing scanner pulses through authenticated realtime/API
  commands. Source: `docs/roadmap/08-world-discovery-planets-intel.md`.
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
- [ ] Enable Phase 11 browser controls for combat, loot, scanner, wallet/cargo,
  and stat snapshots only after authenticated gateway operations expose
  server-authoritative commands and safe snapshot events. Current UI controls
  remain disabled for `combat.set_target`, `combat.use_skill`, `loot.pickup`,
  and `scan.pulse`. Source: `docs/roadmap/11-browser-client-prototype.md`.
- [ ] Add a dedicated browser-client lint configuration after the Phase 11
  prototype settles. Current client verification has TypeScript typecheck,
  Vitest unit tests, Vite production build, and Playwright smoke coverage, but
  no ESLint pass. Source: `docs/roadmap/11-browser-client-prototype.md`.
- [ ] Add a Phase 11 WebSocket browser smoke fixture that sends forbidden
  server payload keys and asserts the browser client rejects them without
  mutating visible state. Unit tests cover parser/reducer rejection today, but
  the Playwright smoke currently checks only rendered body text. Source:
  Symphony review `local-0106`.
- [ ] Finish wiring Phase 12 observability through the authenticated gateway and
  remaining domain service command paths. `ObservedCommandExecutor` now records
  safe realtime command logs/metrics, and combat/loot services emit optional
  metrics, but gateway exposure still depends on authenticated session/player
  resolution and other gameplay services are not instrumented yet. Source:
  Phase 12 Task 1 and core observability wiring.
- [ ] Wire the concrete runtime adapter from discovery
  `ClaimListedIntelStaleMarker` to market/intel listing indexes once coordinate
  scroll listings leave the local domain MVP. Phase 10 now exposes the claim
  hook and `MarketService.MarkListingStale`, but no durable adapter maps
  claimed planets to active market listing IDs yet. Source: Symphony review
  `local-0104`.
- [ ] Narrow lock scope or add per-player/per-planet coordination for Phase 08
  scan, claim, share, and coordinate-scroll services before high-concurrency
  runtime deployment; current MVP services use process-local mutexes.
- [ ] Add durable production/route event outbox emission for Phase 09
  settlement summaries before exposing production and automation routes through
  runtime or gateway callers. Current services return in-memory summaries only.
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
- [ ] Add authenticated owner/access wrappers before exposing Phase 09
  `SettlePlanetProduction`, `SettleRoute`, `EnableRoute`, `DisableRoute`, or
  `UpdateRoute` through gateway/API callers. Current domain methods are
  server-internal and accept planet/route ids directly.
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

- [x] Replace the Phase 05 vertical-slice test-local stat input adapter with
  concrete Phase 03 runtime providers for the in-process backend vertical slice.
  Gateway exposure remains blocked on authenticated session/player resolution.
- [x] Add a zone-worker due-task dispatcher that invokes
  `LootService.HandleScheduledDropTask` from worker ticks instead of requiring
  in-process callers to inspect `TickResult.DueTasks` manually.
