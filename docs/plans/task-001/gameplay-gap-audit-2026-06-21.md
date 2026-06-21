# Gameplay Gap Audit - 2026-06-21

## Purpose

This is a research note for Task 001. It collects gameplay systems that are
implemented in backend/domain code, partially wired through runtime/browser
contracts, or still blocked by missing data, missing client UI, or missing
server-authoritative mutation paths.

Do not close phase checklist items from this document alone. Close checklist
items only after code, tests, and browser proof verify the exact acceptance
criteria in the owning phase file.

Primary player-reported examples covered here:

- Non-starter ship purchase is not usable.
- Equipping a cargo module does not increase visible/effective cargo capacity.
- A scanned/discovered planet can be viewed, but cannot be claimed.

## Current Truth

Task 001 is not complete. The current status report says:

```text
137 / 227 checked = 60.4%
90 open checklist items remain.
```

The deleted monolithic browser smoke suite also means browser/e2e proof is
currently absent until the per-flow harness is rebuilt.

Use these files as the active execution authority:

- `docs/plans/task-001/00-index.md`
- `docs/plans/task-001/task-001-status-report-2026-06-21.md`
- `docs/plans/task-001/browser-e2e-rebuild-plan.md`
- the owning `docs/plans/task-001/*.md` phase file
- the relevant module spec under `docs/plans/modules/`

Older `docs/plans/ui-implementation/*` phase files are useful context, but the
Task 001 status report supersedes any old "done" language.

## Source Sweep

This audit was built from:

- Task 001 plans and status docs.
- Active UI implementation docs.
- Module specs index and relevant system notes.
- Server runtime, realtime operation registry, and domain services.
- Client protocol builders, reducer, app command handlers, and HUD surfaces.
- Read-only subagent sweeps for docs/status, backend bridge gaps,
  client-facing gaps, and seed/data/catalog gaps.

No gameplay code was changed by this audit.

## Player-Reported Issues

### Ship purchase is not usable

Current state:

- `shop.buy_product` exists in the browser protocol and server handlers.
- The server can grant ships through the shop grant path.
- Four ship definitions exist.
- Non-starter ship shop products are currently locked with:

```text
Ship purchase unavailable in this playtest.
```

Root cause:

- This is not only a missing button. The server catalog intentionally marks
  ship products unavailable.
- The phase plan still treats non-starter ship acquisition as open Phase 05 /
  Phase 07 work.
- The Scout crafting path also requires `warp_coil`, which is not defined or
  obtainable in the runtime item catalog.
- Runtime starter ship state diverges from the ship catalog:
  runtime starts a player as `Sparrow`, while the catalog starter is
  `Starter Skiff`.

Needed work:

- Decide which non-starter ships are playtest-buyable, craftable, auctioned, or
  explicitly locked.
- Add server-owned prices/rank requirements/acquisition paths.
- Remove the playtest lock only for products with real grant, wallet, hangar,
  event, and reconciliation coverage.
- Align runtime starter ship seed with the ship catalog.
- Add browser proof for buy success, insufficient funds, duplicate click, and
  hangar refresh.

Owning phases:

- Phase 05: content catalog.
- Phase 07: shop/catalog/economy UI and reconciliation.
- Phase 06: hangar display and active ship reconciliation.

### Cargo module does not increase cargo capacity

Current state:

- `cargo_expander_t1` exists and defines a cargo-capacity stat modifier.
- `StatService` exists and can aggregate ship, module, progression, and buff
  inputs.
- `StatCargoCapacityProvider` exists in runtime providers and tests.
- The browser can equip/unequip modules through real server commands.

Root cause:

- Runtime does not compose/use `StatService` for the active game player state.
- Active ship stats are applied from catalog base values.
- After loadout mutation, the runtime queues `stats.updated` using cached
  `runtime.players[playerID].Stats`.
- Cargo snapshots and visible HUD cargo capacity primarily read
  `state.cargo.capacity` or selected ship base capacity.
- No loadout mutation path invalidates/recalculates stats and cargo capacity,
  then emits refreshed stats, cargo, hangar, and loadout snapshots together.

Needed work:

- Construct runtime `StatService` with server-owned input provider.
- On ship activation and module equip/unequip, invalidate and recalculate
  effective stats.
- Use effective cargo capacity as the authoritative capacity provider.
- Refresh `stats.updated`, `inventory.snapshot`, `hangar.snapshot`,
  `loadout.snapshot`, and cargo capacity after mutation.
- Add tests proving `cargo_expander_t1` changes both visible and server-side
  cargo capacity, including over-capacity behavior.

Owning phases:

- Phase 06: inventory/cargo/loadout/hangar.
- Module spec: `docs/plans/modules/04-module-stat-aggregation.md`.
- Inventory/cargo spec: `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`.

### Scanned planet can be found but not claimed

Current state:

- Scan, known planets, planet detail, production summary, storage summary, and
  route list/snapshot are browser-readable.
- Planet claiming domain code exists.
- Route create/update/enable/disable/settle domain code exists.
- Planet detail can return `available_commands`, but currently only read
  commands are exposed for owned planets.

Root cause:

- Runtime does not compose a browser-safe `ClaimService`.
- Runtime does not compose a browser-safe route service/policy wrapper.
- Realtime registry tests explicitly reject `discovery.claim_planet`,
  `planet.building_build`, `planet.building_upgrade`, and route mutations.
- Client protocol tests also reject those future mutation builders.
- The normal client has no claim/build/route mutation handlers.

Needed work:

- Wire `ClaimService` into runtime only after server-side visibility, range,
  ownership, rank, cost, idempotency, and recovery rules are enforced.
- Register `discovery.claim_planet` only when it is truly server-authoritative.
- On claim success, refresh known planets, selected detail, production, storage,
  wallet/inventory, and quest progress.
- Keep claim/build/route primary controls hidden while mutation contracts are
  blocked.
- Add route mutation contracts only after route ownership, source/destination,
  capacity, settlement, duplicate safety, and event reconciliation are covered.

Owning phase:

- Phase 08: planets, claim, production, and routes.

## Contract Matrix

| Domain | Backend/domain state | Browser state | Main gap |
| --- | --- | --- | --- |
| Auth/session | Mail/password auth, server sessions, websocket session resolution exist. | Real auth shell and websocket connection path exist. | Keep browser proof current. |
| Movement/world/AOI | Runtime owns player position and AOI snapshots. | Move/stop, world snapshot, minimap/radar render. | Browser proof now absent after smoke deletion. |
| Scan/discovery | Scan pulse and known planets are wired. | Scan UI updates last scan and can show known planets. | Scan discovery event does not directly upsert known planet list; planet claim/intel share blocked. |
| Fog/intel/coordinates | Intel share and coordinate item services exist. | No browser mutation loop. | `intel.share`, coordinate create/use rejected by registry. |
| Combat/loot | NPC combat and loot pickup are wired. | Target panel, skill use, loot pickup exist. | NPC catalog/drop tables too thin; combat is NPC-only; broader death/drop/durability not integrated. |
| Death/repair | Death/repair services exist in backend packages. | Runtime has bespoke repair quote/repair ship path. | Full respawn/drop/durability service not composed into runtime; browser e2e incomplete. |
| Wallet/ledger | Wallet, ledger, and economy events exist. | Wallet snapshot exists; shop/market/auction use wallet paths. | No normal player ledger query/event stream; state is in-memory runtime seed. |
| Inventory/cargo | Inventory move and cargo service exist. | `inventory.snapshot` exists. | `inventory.move`/cargo transfer browser mutation rejected; cargo capacity not stat-aware. |
| Hangar/ships | Ship catalog, hangar unlock/swap service exist. | Hangar snapshot and activate ship exist. | No ship unlock/acquisition command except locked shop products; starter seed/catalog drift. |
| Loadout/modules/stats | Module catalog, loadout equip/unequip, saved loadout service, stat service exist. | Equip/unequip exists. | Runtime does not recalculate effective stats/cargo after module changes; saved loadouts not exposed. |
| Progression/skills | Progression snapshot exists and skill systems are planned. | Read-only progression panel. | `progression.unlock_skill` and respec are rejected/not exposed. |
| Crafting | Recipe catalog/service code exists. | `crafting.recipes` read-only; UI says station unavailable. | `crafting.start`, `crafting.complete`, `crafting.cancel` rejected; recipe/item refs mismatch. |
| Shop | Registry-backed shop catalog and buy handler exist. | Shop products render and buy command exists. | Ship products locked; categories/data thin; market/auction/premium reachability needs UI cleanup. |
| Market | Search/create listing/buy/cancel handlers exist. | Client callbacks exist. | Active UI reachability and duplicate/reconcile browser proof need closure. |
| Auction | Search/bid/buy-now/grants handlers exist. | Client callbacks exist. | Lot creation/closing scheduler/provider grants are seed/service-side only; grants are incomplete. |
| Premium | Entitlements/claim/weekly purchase handlers exist. | Client callbacks exist. | Provider webhook/runtime entitlement ingress missing; grants/adapters thin. |
| Quests | Board/accept/progress/claim/reroll exposed. | Quest UI can render and use those commands. | Craft/build/delivery/scan consumers cannot advance through real runtime systems yet; data refs mismatch. |
| Planets/production/routes | Read models wired; claim and route services exist as domain code. | Planet read/detail/production/storage/route list mostly read-only. | Claim/build/route mutations rejected; storage/route rows expose raw ids; no claimed route seed. |
| Admin/observability | Admin and observability handlers exist. | Admin window exists for admin role. | Keep separated from gameplay domain and not required for normal player loops. |
| Persistence/seeds | Runtime seeds catalogs/stores in memory. | Client consumes snapshots. | No migrations/seeds directory found; restart loses most gameplay/economy/discovery/quest/production runtime state. |

## Explicitly Blocked Browser Operations

Backend and client protocol tests intentionally reject these as public browser
gameplay operations today:

```text
crafting.start
crafting.complete
crafting.cancel
inventory.move
progression.unlock_skill
progression.respec_skills
discovery.claim_planet
planet.building_build
planet.building_upgrade
route.create
route.update
route.enable
route.disable
route.settle
intel.share
intel.coordinate_item_create
intel.coordinate_item_use
coordinate_scroll.create
coordinate_scroll.use
mail.send
social.friend_request
social.party_invite
```

These should stay rejected until their owning phase adds:

- authenticated command registration
- server-side ownership/visibility/range/capacity/cost validation
- idempotency keys
- transaction/ledger/event behavior
- client reducer reconciliation
- browser proof

## Backend Exists, But Runtime/Browser Bridge Is Missing

### Inventory and cargo movement

Implemented pieces:

- inventory move service
- cargo service
- wallet/ledger/events

Missing bridge:

- no browser `inventory.move`
- no cargo transfer UI backed by server mutation
- no player-facing ledger/event stream
- no persistent store in audited runtime path

Risk:

- Cargo panels can show real snapshots, but the player cannot reorganize cargo
  through a real command.

### Stat aggregation

Implemented pieces:

- stat service
- runtime stat input provider pieces
- module stat definitions, including cargo capacity

Missing bridge:

- runtime active player state still uses base ship stats/cached stats
- no stat invalidation/recalculation after loadout mutation
- no cargo capacity provider wired to effective stats

Risk:

- Equipping modules can look successful while derived gameplay numbers do not
  change.

### Planet claim and routes

Implemented pieces:

- claim service
- route service
- production storage/read models

Missing bridge:

- no runtime `ClaimService` wiring for browser
- no browser op for claim
- no route mutation registration
- no claim/build/route UI action path

Risk:

- Discovery feels broken: player can find a planet but cannot own or use it.

### Intel and coordinate trading

Implemented pieces:

- intel share service
- coordinate item service

Missing bridge:

- no browser commands
- no UI affordances
- no event/reconciliation path

Risk:

- Fog/discovery economy remains backend-only.

### Crafting jobs

Implemented/readable pieces:

- recipe definitions
- `crafting.recipes` query
- crafting UI data slots

Missing bridge:

- no start/complete/cancel browser command
- no active job snapshot in normal gameplay
- missing recipe inputs/outputs in item catalog

Risk:

- Crafting is visible but inert, and some quest/recipe references cannot be
  completed.

### Quest event consumers

Implemented pieces:

- quest board, accept, progress refresh, claim, reroll
- combat and loot progress hooks
- craft/scan/build/delivery consumer shapes

Missing bridge:

- runtime only advances combat and loot through real gameplay
- scan/build/delivery consumers are no-op or not connected to live systems
- quest catalog references unavailable NPCs, stations, recipes, and buildings

Risk:

- Quest UI can work mechanically, but many generated objectives are impossible.

## Client/UI Gaps

### Planet UI

Current client can show:

- right-rail planet entries
- Planets window/catalog
- planet detail
- Navigate when coordinates are known

Missing:

- Claim button/action backed by real command
- build/upgrade actions
- route create/update/enable/disable actions
- capability-driven action rendering from server `available_commands` or
  explicit `can_*` fields
- display metadata for planet storage/routes

Bad visible risk:

- `item_id` / `resource_item_id` can leak as player-facing labels.

### Crafting UI

Current client can show recipes.

Missing:

- start/complete/cancel buttons backed by real commands
- active job list
- recipe input/output metadata guarantee

Bad visible risk:

- The UI can look like a system exists while all mutation paths are locked.

### Economy/shop UI

Current client has command callbacks for:

- shop buy
- market search/create/buy/cancel
- auction bid/buy-now/grants
- premium claim/purchase weekly X Core

Observed gap:

- The active Shop window path appears centered on `state.shopCatalog.products`;
  market/auction/premium sections and detail functions need reachability and
  layout verification.
- Ship shop products exist but are server-locked.

Needed:

- Make Shop, Market, Auction, and Premium reachable as distinct real systems.
- Keep unavailable grants hidden or locked with game copy, not implementation
  copy.
- Prove duplicate-click and reconciliation behavior in browser tests.

### Pending command state

Current risk:

- Client can mark a command pending before `RealtimeClient.send()` confirms the
  websocket accepted it.
- If socket is closed, `send()` returns false and there is no replay queue.

Needed:

- Pending UI state should reflect accepted-send or explicit local queued state.
- Failed send should clear pending and show disconnected/failed state.

### Scan/known planet reconciliation

Current risk:

- `scan.pulse.resolved` / `discovery.planet_discovered` update last-scan state.
- Known planet catalog depends on later `discovery.known_planets` or detail
  refresh.

Needed:

- Either upsert discovered planet intel from server-safe event payloads, or
  immediately request known planets/detail after discovery.

## Data And Seed Gaps

### Existing seed shape

Current runtime seed is very small:

- one NPC
- one hidden signal
- one system market listing
- one auction fixture
- premium weekly stock
- starter wallet
- starter premium entitlement
- starter modules/loadout

Most audited gameplay stores are in memory. No `migrations` or `seeds`
directory was found for these gameplay systems.

### Catalog gaps

The current content registry validates shop product references, but it does not
yet validate the full gameplay graph:

- recipes
- quest objectives and rewards
- NPC drops
- loot boxes
- production outputs
- route cargo/resources
- auction lots
- premium grant targets
- starter inventory
- starter loadout

Needed:

- One canonical server-owned catalog/validator for all player-visible content.
- Startup should fail or loudly block when a visible system references missing
  content.

### Concrete mismatches found

Item/catalog mismatches:

- Recipes require `laser_lens` and `warp_coil`, but runtime item definitions
  do not define them.
- `x_core_fragment_bundle` appears as an auction fixture source, not as a
  normal item/catalog target.
- Several stackable items use raw ids as display names.

Quest/content mismatches:

- Quest catalog references `energy_cell_batch`, but the crafting catalog does
  not define that recipe/output.
- Quest build objectives reference `extractor_t1` / `storage_t1`, while
  production building ids differ.
- Delivery objective references `station_frontier`, but no station/delivery
  catalog was found in the runtime path.
- Kill quests target `pirate`, `raider`, and `void_raider`, while runtime seed
  only creates a training drone.

NPC/loot gaps:

- NPC combat falls back to Training Drone style stats.
- NPC kills use a one-row raw ore loot table.
- No broad NPC archetype/drop table catalog is wired for playtest.

Shop/catalog gaps:

- Shop categories include ships, weapons, ammo, launchers, shields, speed,
  modules, scanner/radar, stealth, cargo/utility, boosters, and resources.
- Only a narrow set of module products exists.
- Ship products are hard-locked.
- Empty categories should either receive real products or stay hidden/locked
  with explicit game copy.

Planet/production data gaps:

- Production has a small building catalog.
- No claimed planet seed was found.
- No route seed was found.
- No deterministic planet archetype set exists for full claim/build/route
  playtesting.

## DarkOrbit-Style Temporary Math Data

User-approved direction for playtest data:

- Use DarkOrbit-style mathematical baselines/categories where useful.
- Do not copy visuals/assets.
- Use original names, original ids, and original presentation.
- Keep any borrowed-style values marked as temporary playtest balance.
- All values still live server-side and are exposed through real snapshots or
  query responses.

Recommended temporary placeholders:

### Items

Add real item definitions and display metadata for:

```text
laser_lens
warp_coil
x_core_fragment
iron_ore
carbon_shards
energy_cell
scanner_circuit
refined_alloy
helium_dust
```

Use player-facing names such as:

```text
Laser Lens
Warp Coil
X-Core Fragment
Iron Ore
Carbon Shards
Energy Cell
Scanner Circuit
Refined Alloy
Helium Dust
```

### NPCs and drops

Initial playtest archetypes:

| NPC | HP | Shield | Drops |
| --- | ---: | ---: | --- |
| `training_drone` | 30 | 0 | `raw_ore` x3 |
| `pirate` | 120 | 30 | `iron_ore` 4-8, `carbon_shards` 1-3 at 35% |
| `raider` | 180 | 120 | `iron_ore` 6-10, `carbon_shards` 2-5, `laser_lens` at 20% |
| `void_raider` | 550 | 450 | `refined_alloy` 1-3, `scanner_circuit` 1-2, `warp_coil` at 10%, `x_core_fragment` at 2% |

### Recipes

Add:

```text
energy_cell_batch:
  inputs: helium_dust x6, carbon_shards x3
  output: energy_cell x5
  duration: 3 minutes
  credit_cost: 50
```

Keep `laser_alpha_t1` and `scout_t1_unlock`, but make every input obtainable.

### Quests

Align quest templates with seeded content:

- kill targets must match spawned NPC archetypes
- craft objectives must match recipe ids
- build objectives must match production building ids
- delivery objectives need a real station or explicit blocker
- claimable quest fixtures need all prerequisites seeded

### Ships

Unlock non-starter ships through one of:

- shop price plus rank requirement
- crafting recipe
- quest reward
- auction lot
- explicit playtest lock

Also align runtime starter ship stats/name with the catalog.

### Modules and shop

Fill only categories backed by real mechanics first:

- weapon damage
- shield capacity
- speed
- radar/scanner
- cargo capacity
- repair/support if the backend path is real

Keep stealth, ammo, launchers, boosters, cosmetics, and other unsupported
categories hidden or locked until mechanics exist.

### Planets

Seed 2-3 deterministic planet archetypes:

- iron world
- helium world
- salvage world

Temporary baseline:

```text
storage_capacity: 500
claim_cost: 500 credits
claim_rank: 2
```

Reuse current production rates until sinks and routes are implemented.

## Recommended Execution Order

1. Phase 05: canonical content/catalog validator.
   - Fix missing item refs, quest refs, ship acquisition, NPC/drop breadth, raw
     display names, and startup validation.

2. Phase 06 side slice: stat/cargo/loadout correctness.
   - Runtime `StatService`, cargo capacity provider, module recalculation,
     over-capacity handling, and UI refresh.

3. Phase 07: shop/economy/hangar acquisition.
   - Unlock selected ship products, finish shop/category layout, prove buy and
     reconciliation, and make market/auction/premium reachable.

4. Phase 08: planet claim and production/routes.
   - Wire claim first. Keep build/route mutations hidden until real. Then add
     route mutation services with strict validation.

5. Phase 06/09: crafting, inventory move, and quest objective closure.
   - Add crafting jobs and inventory move only when their server contracts are
     complete. Align quest objectives with systems that can actually progress.

6. Phase 03: scanner/witness/browser proof.
   - Finish stealth/scan witness UI and multi-session proof.

7. Phase 11: rebuild browser/e2e release gate.
   - Per-flow tests, screenshot/artifact verifier, and `check:task-001`.

## Minimum Acceptance Checklist For The Next Fixes

Use this as a triage checklist, not as a phase-completion checklist.

- Ship products either buy successfully through `shop.buy_product` or show a
  server-owned playtest lock.
- Buying a ship debits wallet, unlocks hangar entry, refreshes hangar/shop, and
  survives duplicate click/retry.
- Equipping `cargo_expander_t1` changes effective stats and cargo capacity in
  both server validation and visible UI.
- Cargo over-capacity behavior is defined and tested after a capacity decrease.
- `discovery.claim_planet` stays absent until runtime claim service is real.
- Claim success refreshes planet detail, known planets, production, storage,
  wallet/inventory, and quests.
- Planet storage/routes render display metadata, not raw ids.
- `inventory.move` stays absent until ownership/capacity/ledger/idempotency
  tests exist.
- Crafting stays read-only until job start/complete/cancel exists with real
  wallet/item ledger behavior.
- Quest board does not offer impossible objectives unless clearly blocked by
  test-only gating.
- Market/auction/premium are reachable only as real server-backed systems.
- Browser/e2e proof is restored before Task 001 is called done.

## Key Files To Inspect When Implementing

Plans:

- `docs/plans/task-001/05-seeded-game-content-catalog.md`
- `docs/plans/task-001/06-inventory-cargo-loadout-hangar.md`
- `docs/plans/task-001/07-shop-market-catalog-rework.md`
- `docs/plans/task-001/08-planets-production-routes-claim.md`
- `docs/plans/task-001/09-quest-board-game-layout.md`
- `docs/plans/task-001/11-verification-release-gate.md`

Backend:

- `internal/game/realtime/envelope.go`
- `internal/game/realtime/envelope_test.go`
- `internal/game/server/runtime.go`
- `internal/game/server/handlers.go`
- `internal/game/server/content_registry.go`
- `internal/game/server/progression_inventory_handlers.go`
- `internal/game/server/discovery_production_handlers.go`
- `internal/game/server/shop_handlers.go`
- `internal/game/server/economy_handlers.go`
- `internal/game/server/economy_seed.go`
- `internal/game/server/combat_loot_catalog.go`
- `internal/game/runtime/providers.go`
- `internal/game/stats/service.go`
- `internal/game/ships/catalog.go`
- `internal/game/modules/catalog.go`
- `internal/game/crafting/catalog.go`
- `internal/game/quests/catalog.go`
- `internal/game/discovery/claim.go`
- `internal/game/production/route_service.go`

Client:

- `client/src/protocol/envelope.ts`
- `client/src/protocol/envelope.test.ts`
- `client/src/protocol/commands.ts`
- `client/src/app/client-app.ts`
- `client/src/app/command-gate.ts`
- `client/src/net/realtime-client.ts`
- `client/src/state/types.ts`
- `client/src/state/reducer.ts`
- `client/src/ui/hud.ts`

