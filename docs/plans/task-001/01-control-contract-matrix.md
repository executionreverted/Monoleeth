# Phase 01 Control Contract Matrix

## Purpose

This is the committed audit surface for Task 001 Phase 01. It maps visible
browser controls and browser protocol names to server-owned operations, client
state, backend handlers, module specs, and known blockers.

Sources checked:

- `client/src/protocol/envelope.ts`
- `client/src/protocol/commands.ts`
- `client/src/app/client-app.ts`
- `client/src/ui/hud.ts`
- `client/tests/browser-smoke.mjs`
- `internal/game/realtime/envelope.go`
- `internal/game/server/handlers.go`
- `docs/plans/modules/01-player-progression-rank-role-skills.md`
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/plans/modules/03-ship-hangar-loadout.md`
- `docs/plans/modules/05-combat-damage-targeting.md`
- `docs/plans/modules/06-loot-drop-ownership.md`
- `docs/plans/modules/07-death-repair-respawn.md`
- `docs/plans/modules/08-crafting-recipes-materials.md`
- `docs/plans/modules/09-market-auction-premium.md`
- `docs/plans/modules/10-quest-board-generation.md`
- `docs/plans/modules/11-planet-production-offline-settlement.md`
- `docs/plans/modules/12-automation-routes.md`
- `docs/plans/modules/13-intel-coordinate-trading.md`
- `docs/plans/modules/14-world-aoi-fog-security.md`
- `docs/plans/modules/15-api-events-errors.md`
- `docs/plans/modules/16-testing-observability-balancing.md`

## Status Legend

| Status | Meaning |
| --- | --- |
| `real` | Visible control has a browser command, registered server operation, handler, and reducer state. |
| `real-query` | Visible UI reads server-owned state through a query/snapshot operation. |
| `locked` | UI may show a disabled future affordance, but it must not emit a command. |
| `guarded` | Operation is intentionally absent from the browser protocol and covered by guard tests. |
| `blocked` | Contract exists partly, but a backend bridge, fanout, reducer, or UI rule remains open. |
| `admin` | Admin/diagnostic only, role-gated, not normal player gameplay. |
| `debug` | Development-only surface and excluded from player UI truth. |

## Visible Control Matrix

| UI source | Visible intent | Browser operation/event | Server handler/state owner | Status | Notes |
| --- | --- | --- | --- | --- | --- |
| Auth/connect controls | Enter real authenticated game | `session.snapshot`, `session.ready` | `handleSessionSnapshot`, auth/session | `real` | Gameplay commands must stay gated until `session.ready` resolves server identity. Browser reconnect smoke is still open. |
| Logout/disconnect | End local realtime session | socket close + local state reset | auth/session | `real` | Pending commands clear on socket loss. Auth-expiry coverage remains part of reconnect tests. |
| Sync button / bootstrap | Refresh gameplay state | `world.snapshot` plus system snapshots | `handleWorldSnapshot` and snapshot handlers | `real-query` | Bootstrap asks server for world, inventory, hangar, loadout, stats, planets, economy, quests. |
| Canvas click / planet navigate | Move toward world coordinate | `move_to`, `position.corrected`, `movement.stopped` | `handleMoveTo`, world worker | `real` | Server computes movement timing; client interpolates from movement payload. |
| Stop control | Stop movement | `stop`, `movement.stopped` | `handleStop`, world worker | `real` | Pending `move_to` is reconciled on accepted stop/correction. |
| Quick slot 1 / target Fire | Fire basic laser at selected target | `combat.use_skill` | `handleCombatUseSkill`, combat service | `real` | Disabled when no valid target/ship disabled/cooldown. Projectile visual belongs to later UI polish. |
| Quick slot 2 Rocket | Future secondary weapon | none | no public handler | `locked` | Smoke asserts locked quick actions emit no command. |
| Quick slot 3 Scan | Toggle scan mode and pulse | `scan.pulse`, scan events | `handleScanPulse`, discovery/scanner | `real` | Auto-pulse loop is client scheduling plus server cooldown validation. |
| Quick slot 4 Shield | Future shield ability | none | no public handler | `locked` | Must stay disabled until a server skill/module contract exists. |
| Quick slot 5 Warp | Future travel ability | none | no public handler | `locked` | Must stay disabled until route/warp contract exists. |
| Quick slot 6 Gather | Pickup or approach a drop | `loot.pickup` or `move_to` then `loot.pickup` | `handleLootPickup`, loot service | `real` | Pickup uses server range/cargo validation. |
| Target panel | Aim/fire/gather only when target supports it | `combat.use_skill`, `loot.pickup`, `move_to` | combat/loot/world handlers | `real` | No-lock buttons must be hidden or disabled in Phase 04/10 cleanup. |
| Repair controls | Quote and repair disabled ship | `death.repair_quote`, `death.repair_ship`, `death.ship_disabled`, `death.repaired` | `handleDeathRepairQuote`, `handleDeathRepairShip`, runtime domain-event bridge | `real` | Domain `ship.disabled` now maps to client-safe realtime events. Full combat/zone-worker death E2E remains tracked in `docs/todo.md`. |
| Planet memory list | Open planet detail without moving | `discovery.planet_detail` | `handlePlanetDetail` | `real-query` | Smoke asserts detail clicks do not emit `move_to`. |
| Planet detail Navigate | Move to known planet coordinate | `move_to` after selected server detail | world worker | `real` | Enabled only when server-known coordinate exists. |
| Planet production/storage | Show production and storage | `planet.production_summary`, `planet.storage_summary` | `handleProductionSummary`, `handlePlanetStorage` | `real-query` | Internal policy copy must be rewritten in Phase 08. |
| Planet claim/build/route buttons | Claim, build, upgrade, create route | absent future ops | planet/route services | `guarded` | `discovery.claim_planet`, `planet.building_build`, `planet.building_upgrade`, and route mutations stay rejected until Phase 08. |
| Inventory/Cargo window | Show cargo, inventory, item stacks | `inventory.snapshot`, `cargo.snapshot` event | `handleInventorySnapshot`, inventory service | `real-query` | `inventory.move` is intentionally guarded until drag/drop move rules are implemented in Phase 06. |
| Loadout slots | Equip/unequip modules | `loadout.equip_module`, `loadout.unequip_module` | loadout handlers | `real` | Drag/drop UI needs Phase 06 parity pass, but command path exists. |
| Hangar ship list | Show ships and activate one | `hangar.snapshot`, `hangar.activate_ship` | hangar handlers | `real` | Active ship selection is server-owned. |
| Crafting recipes | Show recipe catalog | `crafting.recipes` | `handleCraftingRecipes` | `real-query` | `crafting.start`, `crafting.complete`, and `crafting.cancel` are guarded future mutations. |
| Market/shop | Search, list, buy, cancel | `market.search`, `market.create_listing`, `market.buy`, `market.cancel` | market handlers | `blocked` | Command path is real; player-facing catalog/layout and passive fanout need Phase 07. |
| Auction | Search, bid, buy now, claim grant | `auction.search`, `auction.bid`, `auction.buy_now`, `auction.claim_grant` | auction handlers | `blocked` | One-click bid double-send fixed; cross-session passive update/fanout is still open. |
| Premium | Entitlements, claim, weekly X Core | `premium.entitlements`, `premium.claim`, `premium.purchase_weekly_xcore` | premium handlers | `blocked` | Server-owned, but player-facing product/catalog UX remains Phase 07. |
| Quest board | Board, accept, progress, claim, reroll | `quest.board`, `quest.accept`, `quest.progress`, `quest.claim_reward`, `quest.reroll` | quest handlers | `real` | Layout/game feel belongs to Phase 09. |
| Tutorial/help `?` | Open contextual tutorial catalog | none yet | tutorial content/catalog | `locked` | Phase 04 replaces inspect/dead controls with tutorial affordances. |
| Admin panel | Inspect/repair/economy dashboard | `admin.*` | admin handlers | `admin` | Must stay role-gated and excluded from normal player copy cleanup. |
| Observability panel | Command log, metrics, gates, abuse coverage | `observability.*` | observability handlers | `admin` | Diagnostic only. |
| Debug snapshot/spawn | Dev-only runtime inspection | `debug_snapshot`, `debug_spawn_npc` | debug handlers | `debug` | Excluded from real player UI. |
| Mail/social/intel share | Friend/mail/share/coordinate item actions | absent future ops | social/intel services | `guarded` | `intel.share`, coordinate create/use variants, mail, and social ops are rejected until owner phases define contracts. |

## Browser Operation Parity

All listed operations exist in both `client/src/protocol/envelope.ts` and
`internal/game/realtime/envelope.go`, and have a runtime handler in
`internal/game/server/handlers.go` unless marked otherwise.

| Domain | Operation | Request payload | Response/reducer target | Server handler | Rate/idempotency posture | Status |
| --- | --- | --- | --- | --- | --- | --- |
| Session | `session.snapshot` | `{}` | auth account/player, `reconnect_cursor`, connection ready | `handleSessionSnapshot` | intent burst; server session resolution | `real` |
| World | `world.snapshot` | `{}` | sector, self, visible entities, minimap memory | `handleWorldSnapshot` | intent burst; AOI filtered | `real` |
| World | `move_to` | `{ target: { x, y } }` | movement payload on self via response/event | `handleMoveTo` | intent burst; server-owned speed/position | `real` |
| World | `stop` | `{}` | stopped movement and corrected position | `handleStop` | intent burst; server-owned position | `real` |
| Debug | `debug_spawn_npc` | `{ entity_id, position }` | AOI/debug entity | `handleDebugSpawnNPC` | debug only | `debug` |
| Debug | `debug_snapshot` | `{}` | diagnostic snapshot | `handleDebugSnapshot` | debug only | `debug` |
| Combat | `combat.use_skill` | `{ target_id, skill_id }` | combat damage/miss/cooldown, target state | `handleCombatUseSkill` | intent burst plus combat cooldown | `real` |
| Loot | `loot.pickup` | `{ drop_id }` | loot removed/picked, cargo/wallet follow-up | `handleLootPickup` | intent burst plus ownership/range checks | `real` |
| Death | `death.repair_quote` | `{}` | repair quote | `handleDeathRepairQuote` | intent burst; no mutation | `real` |
| Death | `death.repair_ship` | `{}` | repaired ship/wallet state | `handleDeathRepairShip` | intent burst plus wallet transaction | `real` |
| Progression | `progression.snapshot` | `{}` | progression summary | `handleProgressionSnapshot` | intent burst | `real-query` |
| Inventory | `inventory.snapshot` | `{}` | inventory stacks/instances/counts | `handleInventorySnapshot` | intent burst | `real-query` |
| Hangar | `hangar.snapshot` | `{}` | owned ships and active ship | `handleHangarSnapshot` | intent burst | `real-query` |
| Hangar | `hangar.activate_ship` | `{ ship_id }` | active ship/loadout/stat refresh | `handleHangarActivateShip` | intent burst plus ownership validation | `real` |
| Loadout | `loadout.snapshot` | `{}` | loadout slots/modules | `handleLoadoutSnapshot` | intent burst | `real-query` |
| Loadout | `loadout.equip_module` | `{ slot_id, item_instance_id }` | loadout/inventory/stat refresh | `handleLoadoutEquipModule` | intent burst plus ownership/slot validation | `real` |
| Loadout | `loadout.unequip_module` | `{ slot_id }` | loadout/inventory/stat refresh | `handleLoadoutUnequipModule` | intent burst plus ownership/slot validation | `real` |
| Stats | `stats.snapshot` | `{}` | combat, scan, cargo, speed stats | `handleStatsSnapshot` | intent burst | `real-query` |
| Crafting | `crafting.recipes` | `{}` | recipe catalog | `handleCraftingRecipes` | intent burst | `real-query` |
| Scan | `scan.pulse` | `{}` | scan start/resolution, planet discovery | `handleScanPulse` | intent burst plus scanner cooldown | `real` |
| Discovery | `discovery.known_planets` | `{}` | known planet list/counts | `handleKnownPlanets` | intent burst; visibility filtered | `real-query` |
| Discovery | `discovery.planet_detail` | `{ planet_id }` | selected planet detail | `handlePlanetDetail` | intent burst; visibility filtered | `real-query` |
| Planet | `planet.production_summary` | `{ planet_id? }` | production summary | `handleProductionSummary` | intent burst; ownership/visibility filtered | `real-query` |
| Planet | `planet.storage_summary` | `{ planet_id? }` | planet storage summary | `handlePlanetStorage` | intent burst; ownership/visibility filtered | `real-query` |
| Routes | `route.list` | `{}` | route list | `handleRouteList` | intent burst; ownership filtered | `real-query` |
| Routes | `route.snapshot` | `{ route_id }` | single route upsert | `handleRouteSnapshot` | intent burst; ownership filtered | `real-query` |
| Wallet | `wallet.snapshot` | `{}` | balances | `handleWalletSnapshot` | intent burst | `real-query` |
| Market | `market.search` | `{ item_id? }` | listings | `handleMarketSearch` | intent burst; public listing read | `real-query` |
| Market | `market.create_listing` | `{ item_id, quantity, unit_price, source_location?, item_instance_id? }` | listing upsert, wallet/inventory side effects | `handleMarketCreateListing` | intent burst plus escrow/ledger validation | `blocked` |
| Market | `market.buy` | `{ listing_id, quantity }` | listing update, wallet/inventory side effects | `handleMarketBuy` | intent burst plus escrow/ledger validation | `blocked` |
| Market | `market.cancel` | `{ listing_id }` | listing cancelled, escrow return | `handleMarketCancel` | intent burst plus ownership validation | `blocked` |
| Auction | `auction.search` | `{}` | lots/grants | `handleAuctionSearch` | intent burst | `real-query` |
| Auction | `auction.bid` | `{ auction_id, amount }` | lot update, wallet escrow side effects | `handleAuctionBid` | intent burst plus bid/escrow validation | `blocked` |
| Auction | `auction.buy_now` | `{ auction_id }` | lot close/grant/wallet side effects | `handleAuctionBuyNow` | intent burst plus escrow validation | `blocked` |
| Auction | `auction.claim_grant` | `{}` | grants/search refresh | `handleAuctionClaimGrant` | intent burst plus grant ownership | `blocked` |
| Premium | `premium.entitlements` | `{}` | entitlements/stock | `handlePremiumEntitlements` | intent burst | `real-query` |
| Premium | `premium.claim` | `{ entitlement_id }` | entitlement claimed, wallet/item grant | `handlePremiumClaim` | intent burst plus entitlement idempotency | `blocked` |
| Premium | `premium.purchase_weekly_xcore` | `{}` | stock consumed, entitlement created | `handlePremiumWeeklyXCore` | intent burst plus weekly stock/idempotency | `blocked` |
| Quest | `quest.board` | `{}` | offers/active quest board | `handleQuestBoard` | intent burst | `real-query` |
| Quest | `quest.accept` | `{ offer_id }` | active quest added | `handleQuestAccept` | intent burst plus offer validation | `real` |
| Quest | `quest.progress` | `{}` | active quest progress | `handleQuestProgress` | intent burst | `real-query` |
| Quest | `quest.claim_reward` | `{ quest_id }` | reward claimed, wallet/inventory/progression updates | `handleQuestClaimReward` | intent burst plus reward idempotency | `real` |
| Quest | `quest.reroll` | `{}` | regenerated board | `handleQuestReroll` | intent burst plus wallet/reroll policy | `real` |
| Admin | `admin.inspect_player` | `{ target_player_id? }` | admin inspection summary | `handleAdminInspectPlayer` | role-gated | `admin` |
| Admin | `admin.repair_craft_job` | `{ job_id }` | admin repair result | `handleAdminRepairCraftJob` | role-gated | `admin` |
| Admin | `admin.economy_dashboard` | `{}` | economy dashboard | `handleAdminEconomyDashboard` | role-gated | `admin` |
| Observability | `observability.command_log` | `{}` | command log | `handleObservabilityCommandLog` | role-gated diagnostic | `admin` |
| Observability | `observability.metrics` | `{}` | metrics snapshot | `handleObservabilityMetrics` | role-gated diagnostic | `admin` |
| Observability | `observability.release_gate` | `{}` | release gate state | `handleObservabilityReleaseGate` | role-gated diagnostic | `admin` |
| Observability | `observability.abuse_coverage` | `{}` | abuse coverage state | `handleObservabilityAbuseCoverage` | role-gated diagnostic | `admin` |

## Guarded Future Operations

These names are intentionally not browser operations yet. Phase 01 guard tests
must keep them rejected until their owner phase adds server validation, reducer
state, UI copy, and tests.

| Owner phase | Guarded operation names | Required owner work before enabling |
| --- | --- | --- |
| Phase 06 inventory/crafting | `inventory.move`, `crafting.start`, `crafting.complete`, `crafting.cancel` | inventory ownership, slot/cargo capacity, job idempotency, drag/drop UI tests |
| Phase 06 progression | `progression.unlock_skill`, `progression.respec` | server skill tree validation, points ledger, stat refresh |
| Phase 08 planets/routes | `discovery.claim_planet`, `planet.building_build`, `planet.building_upgrade`, `route.create`, `route.cancel` | claim ownership, production locks, route settlement/outbox, player-facing copy |
| Phase 03/08 intel | `intel.share`, `intel.coordinate_create`, `intel.coordinate_use`, `intel.coordinate_item.create`, `intel.coordinate_item.use` | coordinate item ownership, visibility checks, tradeability, expiry rules |
| Later social/mail | `mail.send`, `mail.delete`, `social.friend_request`, `social.friend_accept`, `social.block` | abuse limits, privacy rules, persistence, UI surfaces |

## Client Event Parity

All event names below are public client event names from `CLIENT_EVENTS` and
`realtime.ClientEventType`. Reducer behavior must remain client-safe: hidden
world truth, trusted ids, rolls, procedural seeds, and provider internals stay
filtered before browser delivery.

| Event(s) | Client-safe payload | Reducer/UI target | Follow-up or blocker |
| --- | --- | --- | --- |
| `session.ready` | public account/player, roles, expiry, `reconnect_cursor` | auth, connection ready, reconnect cursor | Browser reconnect smoke still open. |
| `player.snapshot` | public player profile | player panel/state | None known. |
| `ship.snapshot` | public ship state, disabled state when present | ship panel, movement/combat gating | Full death E2E remains a browser scenario blocker. |
| `stats.updated` | derived server stats | stats, quick action ranges/cooldowns | None known. |
| `wallet.snapshot` | balances only | wallet/economy panels | None known. |
| `cargo.snapshot` | cargo contents/counts | cargo panel | Inventory/cargo UI parity remains Phase 06. |
| `world.snapshot` | visible entities, sector, minimap-safe memory | world renderer, minimap, AOI state | Snapshot refresh on reconnect still needs browser proof. |
| `aoi.entity_entered`, `aoi.entity_updated`, `aoi.entity_left` | visible entity payload/diff | visible entity map, target/minimap | Minimap click/navigation polish belongs to Phase 02. |
| `position.corrected`, `movement.stopped` | server position/movement state | self entity interpolation, pending movement cleanup | Covered by reducer tests for pending cleanup. |
| `server.notice` | public notice text | command log/toast | Copy cleanup remains owner phase work. |
| `target.updated` | selected target summary | target panel | No-lock clutter cleanup remains Phase 04/10. |
| `combat.damage`, `combat.miss`, `combat.cooldown_started`, `combat.npc_killed` | combat-safe target/result/cooldown data | target HP, cooldowns, log/projectile affordance | Projectile/HP UI polish remains later UI phases. |
| `loot.created`, `loot.updated`, `loot.removed`, `loot.picked_up` | visible loot/drop and pickup result | visible entities, cargo/log | Pickup reward copy must stay server-derived. |
| `progression.snapshot` | rank/xp/skill summary | progression state | Unlock mutations stay guarded. |
| `inventory.snapshot` | stackable/items/counts | inventory/loadout/cargo windows | Drag/drop move remains Phase 06. |
| `hangar.snapshot` | ships and active ship | hangar/systems window | None known. |
| `loadout.snapshot` | slots/modules | loadout window, stats refresh | None known. |
| `crafting.recipes` | recipe catalog | crafting panel | Job mutations stay guarded. |
| `scan.pulse_started`, `scan.pulse_resolved`, `scan.planet_discovered` | scanner status and discovered public planet signal | scan mode, planet intel, radar/minimap | Witness/stealth scan work belongs to Phase 03. |
| `discovery.known_planets`, `discovery.planet_detail` | known planet summaries/details | planet catalog, planet detail modal | Claim/build/route mutation copy remains Phase 08. |
| `planet.production_summary`, `planet.storage_summary` | production/storage summaries | production and storage state | Internal lock copy must be removed in Phase 08. |
| `route.list`, `route.snapshot` | route list or single route | route state/upsert | Route mutations/outbox remain guarded. |
| `market.listing_created`, `market.listing_updated`, `market.sale_completed`, `market.listing_cancelled` | listing or sale summary | market listings | Reducer reconciles delivered event; cross-session fanout remains backend blocker. |
| `auction.lot_updated`, `auction.bid_placed`, `auction.closed` | lot/grant summary | auction lots/grants | Reducer reconciles delivered event; cross-session fanout remains backend blocker. |
| `premium.entitlement_created`, `premium.entitlement_claimed`, `premium.stock_consumed` | entitlement/stock summary | premium panel | Reducer reconciles delivered event; product UX remains Phase 07. |
| `economy.flow_updated` | observability summary only | log/admin surface | Define safe player payload or keep admin-only. |
| `quest.board_generated`, `quest.accepted`, `quest.progressed`, `quest.completed`, `quest.reward_claimed`, `quest.board_rerolled`, `quest.abandoned` | quest board/progress/reward summaries | quest board | Passive quest reconciliation remains Phase 09 acceptance. |
| `admin.action_completed` | admin action status | admin panel/log | Role-gated only. |
| `observability.metric_updated`, `release_gate.updated` | diagnostic summary | admin/observability panel | Role-gated only. |
| `death.ship_disabled`, `death.repaired` | disabled/repair-safe public ship state | disabled UI, repair controls, movement/combat/scan pending cleanup | Domain bridge is implemented; full combat/zone-worker death E2E remains. |

## Protocol And Reducer Rules

- Request envelopes use `v`, `request_id`, `op`, `payload`, and `client_seq`.
- Response envelopes use `request_id`, `ok`, `payload` or `error`,
  `server_time`, and `v`.
- Event envelopes use `event_id`, `type`, `payload`, `server_time`, `seq`, and
  `v`.
- Browser command builders reject trusted/client-authored fields before sending.
- Server handlers also reject trusted payload keys.
- App command gates block real-mode commands until server `session.ready`
  promotes the browser state to `connected`; raw socket-open state stays
  `authenticated_pending_socket`.
- Lower stale `seq` events are ignored by the reducer.
- Unknown public events log without mutating gameplay state.
- Socket loss clears pending operations so controls do not stay blocked.
- Response-lost/event-delivered recovery is covered for movement and scan
  pending state. Reducer/server tests cover reconnect cursor and snapshot
  refresh; full browser reconnect smoke remains open.
- `request_id` is network retry metadata. Economy, quest reward, loot pickup,
  premium, auction, crafting, route, and planet mutations still need domain
  idempotency keys in their owner services before new operations are exposed.

## Named Open Blockers

| Blocker | Impact | Owner phase |
| --- | --- | --- |
| Full death/respawn E2E | Combat or zone-worker authority still needs a browser E2E that produces `death.ship_disabled` without test-only state mutation. | Phase 11 / death hardening |
| Passive economy fanout | Market/auction/premium reducer can reconcile delivered events, but other sessions may not receive updates yet. | Phase 07 |
| Full browser reconnect smoke | Reducer handles pieces, but real socket reconnect/cursor bootstrap still needs browser proof. | Phase 01 / Phase 11 |
| Dead/no-lock controls | Some visible controls still need to be hidden, locked, or replaced with tutorial `?` affordances. | Phase 04 / Phase 10 |
| Player-facing copy cleanup | Internal copy like server policy/recalculates must be replaced with game UI language. | Phase 04 / Phase 07 / Phase 08 |
| Future mutation contracts | Inventory move, crafting jobs, planet claim/build/route, progression unlock, intel coordinate, mail, and social operations remain guarded. | Owner phases listed above |
