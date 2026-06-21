# Phase 01 - Gameplay Connection Audit And Contract Parity

## Goal

Detect and fix the missing UI to server gameplay connections before the UI pass
turns placeholders into better-looking placeholders. This phase creates the
truth table for every visible menu/control and closes the small known reducer,
protocol, and double-send gaps that are safe to fix first.

## Problems Covered

- Missing UI/server gameplay links.
- Enabled or visible controls that do not perform real gameplay.
- Reducer drift for existing server responses/events.
- Double command emission from one UI click.
- Debug panels that imply real loops where the server contract is absent.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
docs/todo.md
docs/plans/ui-implementation/06-progression-inventory-loadout-crafting.md
docs/plans/ui-implementation/07-discovery-planets-production-routes.md
docs/plans/ui-implementation/08-market-auction-premium-economy.md
docs/plans/modules/15-api-events-errors.md
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/src/state/reducer.ts
client/src/ui/hud.ts
internal/game/realtime/envelope.go
internal/game/server/handlers.go
```

## Current Findings

- `planet.storage_summary` is requested and server-handled, but client state
  handling for `planet_storage` is weak or missing.
- `route.snapshot` exists, but reducer/UI handling expects route lists more than
  a singular route response.
- `death.ship_disabled` is mapped but mostly logs; disabled/repair UI state
  depends on separate snapshots.
- Market, auction, and premium broadcast events do not strongly update passive
  clients.
- `auction.bid` can be double-sent from one click path.
- `admin.repair_craft_job` exists, but crafting job creation is not exposed in
  the normal browser loop.
- Missing browser loops include `inventory.move`, `progression.unlock_skill`,
  `crafting.start`, `crafting.complete`, `crafting.cancel`,
  `discovery.claim_planet`, `planet.building_build`,
  `planet.building_upgrade`, route mutations, `intel.share`, coordinate item
  create/use, and social/mail controls.
- Session/transport state transitions need audit: WebSocket open,
  `session.ready`, reconnect, auth expiry, pending-command cleanup, and command
  gating must prove the authenticated real client reaches a command-enabled
  state only after server-resolved session readiness.

## Subagent Review Additions - 2026-06-20

- Add an explicit reducer contract for singular `route.snapshot` responses and
  events. `applySnapshotPayload` must handle `{ route: ... }` in addition to
  route lists, and tests must prove route updates reconcile without requiring a
  separate list refresh.
- Add an explicit reducer contract for standalone `planet_storage` payloads
  returned by `planet.storage_summary`. The plan must choose whether storage is
  normalized as planet-owned state or merged into the selected planet detail.
- Define passive economy fanout/reconcile behavior. Market, auction, and premium
  events must either be broadcast to affected sessions with enough safe payload
  for the reducer, or trigger a named refresh query path in `client-app.ts`.
- Treat `death.ship_disabled` as a required bridge or named blocker. Domain
  `ship.disabled` must become a public realtime event, a ship snapshot refresh,
  or an explicit blocked item before UI depends on it.
- Expand unimplemented-operation guard coverage beyond crafting/claim/routes to
  include `inventory.move`, `progression.unlock_skill`, skill respec,
  `intel.share`, coordinate item create/use, and mail/social operations.

## Second Subagent Review Additions - 2026-06-20

- Add the missing progression contract gap to the audit matrix. The browser
  currently exposes `progression.snapshot`, but Task 001 must decide whether
  rank-up and skill-tree flows are automatic/read-only for now or will expose
  real operations such as `progression.try_rank_up` and
  `progression.unlock_skill`.
- Add the missing system shop contract gap. Market, auction, and premium are not
  enough for the DarkOrbit-style shop; Phase 07 needs either `shop.catalog` plus
  `shop.buy_product` or an explicitly documented system-backed market contract.
- Clarify `auction.claim_grant`. If the current operation only refreshes auction
  state, the UI must not label it as a claim mutation until a real grant adapter
  exists for the payload type.
- Clarify premium stock intent. An empty payload that buys the current weekly
  stock is acceptable only while there is exactly one product; multi-product
  stock needs a server-owned product id/period intent and availability check.
- Add browser smoke gaps to the protocol table: `auction.buy_now`,
  completed `quest.claim_reward`, and death repair quote/repair need visible
  workflow smoke if their controls are present.
- Add trusted-payload denylist parity work. Go and TypeScript denylist/empty
  payload behavior must match for sensitive fields such as hidden truth,
  procedural data, loot rolls, target-player ids, and scan internals.
- Keep the revoked-session/auth-expiry browser smoke as an explicit open gate.
  Existing unit/server evidence is useful but does not close the browser-level
  `auth_expired` plus pending-command cleanup scenario tracked in `docs/todo.md`.

## Third Subagent Review Additions - 2026-06-20

- Add a protocol denylist parity table for Go request rejection, TypeScript
  command payload rejection, and TypeScript server-message rejection. Admin-only
  exceptions such as `admin.inspect_player.target_player_id` must be listed
  explicitly, with tests proving forbidden hidden/player/scan/provider/password
  fields are rejected everywhere else.
- Treat `server_recalculates` as a Phase 01 contract cleanup, not only Phase 07
  UI polish. Either remove it from normal public gameplay payloads or map it to
  non-internal player-facing quote state before render; smoke must fail on
  visible text, `title`, `aria-label`, `alt`, `placeholder`, and payload-derived
  UI copy containing `server_recalculates` or `server recalculates`.
- Decide `auction.claim_grant`: either rename it to a read query such as
  `auction.grants` / `auction.snapshot`, or implement a real claim mutation
  with grant id, idempotency, ledger/inventory mutation, and reducer reconcile.
- Make premium stock purchase intent product-specific before multiple stock
  rows exist. The request should include a server-owned `period_key` or product
  id, validate availability/price server-side, and keep the current empty
  payload only as a named one-product temporary blocker.
- Add enabled-control smoke assertions for local-only duplicate or impossible
  controls such as generic `Inspect`, disabled `Aim`, locked quick actions, and
  locked quest claim so Phase 01 records each as `hide`, `local-ui`, or an
  owner-phase blocker.

## Fourth Subagent Review Additions - 2026-06-20

- `session.snapshot` is classified as a production-safe authenticated session
  query, while `debug_snapshot` and `debug_spawn_npc` are dev-only operations.
  Production WebSocket tests must prove debug operations return a
  forbidden/public error outside dev mode, and real-mode client smoke must
  prove normal login, sync, and reconnect never send debug operations.
- Split rate-limit posture by operation instead of treating all intents as the
  same burst bucket. Add acceptance rows for `scan.pulse`, `market.search`,
  `quest.reroll`, `combat.use_skill`, `loot.pickup`, `stop`, and any hotkey
  wrapper that can emit them.
- Add public bridge or refresh decisions for domain transitions that currently
  risk stale UI: `loot.owner_lock_expired`, `loot.expired`,
  `planet.production_settled`, `offline.settlement_completed`, and
  `route.transfer_*`.
- Document whether loot state changes are emitted as `loot.updated` /
  `loot.removed`, trigger `world.snapshot` / `cargo.snapshot` refreshes, or
  remain named backend blockers.
- Keep dev/demo protocol affordances out of normal player chrome. Demo clients
  may call debug helpers only behind explicit dev/test mode and must not weaken
  real-mode operation parity.

## Implementation Plan

1. Build a control/contract matrix.
   - List every visible control and intent source in `client/src/ui/hud.ts`,
     `client/src/app/client-app.ts`, `client/src/render/world-renderer.ts`, and
     `client/src/render/world-view.ts`.
   - Map it to operation, query, event, reducer state, server handler, and tests.
   - Mark each as `real`, `locked`, `hide`, or `phase-owned`.
   - Add an envelope/protocol parity table for every browser operation/event:
     kind, request payload schema, success payload schema, follow-up events,
     reducer target, public error codes, retryable/idempotency posture,
     rate-limit posture, server handler, and tests.
   - Verify Go/TypeScript operation registry parity or document intentional
     server-only/client-only exceptions.
   - Cover protocol version, malformed JSON, unknown fields, non-object
     payloads, max-size/read-limit behavior, and public error code mapping.

2. Fix existing drift before adding new gameplay.
   - Parse and store `planet_storage` responses.
   - Define exact merge rules for `planet.storage_summary`: standalone storage
     state or merge into the selected planet production/storage model.
   - Parse and upsert singular `{ route: ... }` from `route.snapshot`
     responses/events.
   - Add the backend bridge from domain `ship.disabled` to public
     `death.ship_disabled`, or record it as a named blocker before UI depends
     on it.
   - Make passive economy/auction/premium events update state, trigger explicit
     refresh, or document backend fanout/session-scope policy.
   - Fix `auction.bid` so one click sends one command.
   - Define general event reconciliation rules for duplicate/stale `seq`,
     unknown events, response-lost/event-delivered recovery, and reconnect
     snapshot refresh.

3. Strengthen operation guard tests.
   - Extend Go registry tests for unimplemented browser mutations.
   - Extend TypeScript protocol tests for unimplemented or forbidden browser
     operations.
   - Include `inventory.move`, skill unlock/respec, `intel.share`, coordinate
     item create/use, and mail/social operations in guard tests or named
     deferrals.
   - Keep future operations rejected until implemented in their phase.

4. Remove or downgrade misleading controls.
   - Any visible enabled button must have a real handler and server contract.
   - Future actions can be locked only if the UI copy is player-facing and not
     internal/debug.

5. Audit diagnostics and public errors.
   - For every operation, document the user-facing message, retryable behavior,
     toast/log destination, hidden/not-found generic handling, and redaction
     requirements.
   - Never expose passwords, tokens, session ids, provider refs, hidden world
     truth, or internal/debug copy.
   - Admin diagnostics remain role-gated and excluded from normal player copy
     bans.

## Control/Contract Matrix - 2026-06-20

Verdicts:

- `real`: visible control has a server-owned command/query/event path.
- `local-ui`: presentation-only control; no gameplay truth mutation expected.
- `misleading`: backed by some code, but copy/shape implies the wrong action.
- `locked-no-op`: visible disabled/future affordance; must be hidden, quiet, or
  converted to player-facing locked copy in owner phase.
- `defer-phase`: intentionally belongs to a later Task 001 phase and is guarded
  or tracked.

| Surface | Control / intent | State | Client handler | Contract | Server handler / owner | Reconcile target | Verdict | Next |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Auth | Login / register | unauthenticated | `AuthPanel`, `AuthClient` | HTTP auth routes | auth HTTP handler | session cookie, `session.ready` | real | keep |
| Auth | Logout | authenticated | `logout()` | HTTP logout + socket close | auth HTTP handler | `authLoggedOut`, cleared gameplay | real | keep |
| Transport | Real auto-connect | authenticated real mode | session restore/connect flow | WebSocket `/ws` | `serveWebSocket` | bootstrap events, connection status | real | keep |
| Transport | Demo WS URL / connect / disconnect | demo/dev controls | `connect()`, `disconnect()` | WebSocket `/ws` | `serveWebSocket` | bootstrap events, connection status | local-ui | Keep out of normal real-mode player chrome |
| World | Canvas click movement | world focused, no modal ownership | `sendMove()` | `move_to` | `OperationMoveTo` | `position.corrected`, AOI diff, movement target | real | keep |
| World | Entity click / target select | visible entity marker | renderer selection handler | none or action-specific follow-up | none for select | selected target state | local-ui | Keep; action buttons must stay server-backed |
| World | Loot entity click | visible loot marker | loot approach/pickup path | `move_to` then `loot.pickup` | move/loot handlers | movement, loot/cargo/inventory events | real | keep |
| World | Remembered planet marker click | known planet memory marker | marker click handler | `discovery.planet_detail` | `OperationPlanetDetail` | selected planet detail/modal state | real | Phase 02/10 also add minimap click parity |
| World | Stop movement | moving / quick action | `stopMovement()` | `stop` | `OperationStop` | `movement.stopped`, `position.corrected` | real | keep |
| World | Sync / refresh | connected | `syncSnapshot()` | `world.snapshot` plus systems queries | `OperationWorldSnapshot` and query handlers | `world.snapshot`, panel snapshots | real | keep |
| Actionbar | Laser / fire | hostile target and energy/cooldown | `sendBasicSkill()` | `combat.use_skill` | `OperationCombatUseSkill` | combat events, ship/player snapshots | real | keep |
| Actionbar | Gather | selected loot or approach needed | `sendLootPickup()` / navigation | `loot.pickup` or `move_to` | `OperationLootPickup`, `OperationMoveTo` | loot/cargo/inventory events | real | keep |
| Actionbar | Scan | scan mode toggle | `toggleScanMode()` then scan loop | `scan.pulse` | `OperationScanPulse` | scan events and scan mode state | misleading | Phase 04 copy should say scanner automation, not one-shot pulse |
| Intel | Scan panel action | scanner/intel window | same scan handler or detail controls | `scan.pulse` automation | `OperationScanPulse` | scan events and known planets | misleading | Ensure all scan surfaces describe automation consistently |
| Actionbar | Repair quote | ship disabled | `deathRepairQuote()` | `death.repair_quote` | `OperationDeathRepairQuote` | repair quote response | real | keep |
| Actionbar | Repair | disabled ship + quote | `deathRepairShip()` | `death.repair_ship` | `OperationDeathRepairShip` | `death.repaired`, ship/player/wallet snapshots | real | keep |
| Target panel | Aim / No lock clutter | selected/no selected target | local render | none | none | none | locked-no-op | Phase 04/10 hide impossible primary actions |
| Target panel | Rocket / Shield / Warp | disabled future buttons | local render | none | none | none | locked-no-op | Phase 04/10 hide or move to player-facing locked module slots |
| Navigation | Left HUD windows | always visible | `openWindow()` | none | none | window state only | local-ui | keep as presentation |
| Window shell | Close / drag / focus | window open | HUD window registry | none | none | window state only | local-ui | keep; Phase 04 content sizing |
| Modal shell | Close / backdrop / drag / focus trap | modal open | modal registry handlers | none | none | modal state and input suppression | local-ui | Keep; verify world input stays blocked |
| Window shell | Generic Inspect | titlebar button | `openModal(panel)` | none | none | duplicated window body | misleading | Replace with contextual `?` tutorial modal |
| Cargo | Inventory / Cargo open | server snapshots loaded | panel render | `inventory.snapshot`, cargo snapshots | `OperationInventorySnapshot`, cargo events | inventory/cargo state | real | Phase 06 split tabs/layout |
| Cargo | Quantity / row selection | panel local | local handlers | none | none | selected row/quantity only | local-ui | keep if paired with real actions |
| Loadout | Equip selected module | compatible owned module | `loadoutEquipModule()` | `loadout.equip_module` | `OperationLoadoutEquipModule` | loadout/inventory/stats snapshots | real | keep |
| Loadout | Unequip module | equipped slot | `loadoutUnequipModule()` | `loadout.unequip_module` | `OperationLoadoutUnequipModule` | loadout/inventory/stats snapshots | real | keep |
| Loadout | Module details/select | module row/card | `module-select` | none | none | selected module UI state | local-ui | Keep as selection before real equip |
| Loadout | Drag/drop equip/unequip | module card or slot dragged | drag/drop handlers | `loadout.equip_module` / `loadout.unequip_module` | loadout handlers | loadout/inventory/stats snapshots | real | keep |
| Hangar | Select ship row | owned ship list | local `selectedHangarShipID` | none | none | selection only | local-ui | keep |
| Hangar | Activate ship | owned ship, safe context | `hangarActivateShip()` | `hangar.activate_ship` | `OperationHangarActivateShip` | hangar/ship/stats/cargo/loadout snapshots | real | keep |
| Hangar | Loadout shortcut | hangar panel button | `open-window` systems/cargo target | none | none | window focus only | local-ui | Keep if label clearly navigational |
| Shop | Category / row / quantity | shop open | local shop handlers | none | none | selection/quantity only | local-ui | keep |
| Shop | Market buy | active listing | `marketBuy()` | `market.buy` | `OperationMarketBuy` | market/wallet/inventory snapshots | real | keep |
| Shop | Create listing | owned sellable stack | `marketCreateListing()` | `market.create_listing` | `OperationMarketCreateListing` | market/inventory snapshots | real | keep |
| Shop | Cancel listing | owned active listing | `marketCancel()` | `market.cancel` | `OperationMarketCancel` | market/inventory snapshots | real | keep |
| Shop | Auction bid | active lot, not leading | `sendAuctionBid()` | `auction.bid` | `OperationAuctionBid` | auction/wallet events; pending guard | real | keep; passive reconcile still open |
| Shop | Auction buy now | active lot with buy-now | `auctionBuyNow()` | `auction.buy_now` | `OperationAuctionBuyNow` | auction/wallet/grant events | real | keep; passive reconcile still open |
| Shop | Auction grants refresh | grant row | `auctionGrants()` | `auction.grants` | `OperationAuctionGrants` | auction grant snapshot | real | Read-only grant snapshot query; concrete grant mutation remains future owner-phase work |
| Shop | Premium claim | pending entitlement | `premiumClaim()` | `premium.claim` | `OperationPremiumClaim` | premium/wallet events | real | keep; private fanout open |
| Shop | Premium stock purchase | premium stock row | `premiumWeeklyXCore(product_id, period_key)` | `premium.purchase_weekly_xcore` | `OperationPremiumWeeklyXCore` | premium/wallet events | real | Uses selected server-returned stock row identity; server still owns world, price, currency, stock, and weekly limit |
| Planets | Known planet list / select | known planet snapshot | `planetDetail()` | `discovery.planet_detail` | `OperationPlanetDetail` | selected planet detail | real | Phase 08 detail layout and claim |
| Planets | Navigate to planet | selected detail has position | `navigateToPlanet()` | `move_to` after detail | `OperationMoveTo` | movement events | real | keep |
| Planets | Claim / Build / Upgrade / Route / Auto | placeholder controls | local render | future `discovery.claim_planet`, planet/route ops | guarded unimplemented ops | none | locked-no-op | Phase 08 must wire or hide until meaningful |
| Quests | Select quest row | board loaded | local `selectedQuestKey` | none | none | selection only | local-ui | keep |
| Quests | Accept offer | offer available | `questAccept()` | `quest.accept` | `OperationQuestAccept` | quest events/board | real | keep |
| Quests | Claim reward | completed active quest | `questClaimReward()` | `quest.claim_reward` | `OperationQuestClaimReward` | quest/wallet/inventory/progression snapshots | real | keep |
| Quests | Reroll board | board loaded | `questReroll()` | `quest.reroll` | `OperationQuestReroll` | quest board/wallet events | real | keep |
| Admin Ops | Refresh admin panels | admin session | `refreshAdminOps()` | admin/observability queries | admin and observability handlers | admin summaries | real | admin-only, keep |
| Admin Ops | Repair craft job | admin job row | `adminRepairCraftJob()` | `admin.repair_craft_job` | `OperationAdminRepairCraftJob` | admin repair event/summary | real | admin-only, normal crafting still deferred |
| Topbar / social | Mail / Social / menu badges | disabled/future | local render or absent handler | none | none | none | locked-no-op | Hide until server-backed contracts exist |
| Minimap | HUD contacts / remembered planets | visible radar/memory | render-only spans today | none for click | none | none | defer-phase | Phase 02/10 add click navigation/detail |
| World map | Remembered planet markers | world memory marker | marker click handler | `discovery.planet_detail` and optional `move_to` | planet/move handlers | planet detail, navigation | real | Keep; align minimap behavior later |
| Keyboard | `1`, `3`, `6` quick actions | world focus only | `handleShortcutKeyDown()` | fire/loot/scan mapped actions | action-specific handlers | action-specific state | real | Phase 10 extend coverage |
| Keyboard | `2`, `4`, `5` quick actions | locked future actions | quick action render | none | none | none | locked-no-op | Hide or convert to player-facing module locks |
| Keyboard | `Tab` target cycle | planned | none or partial | none | none | target selection | defer-phase | Phase 10 implement |
| Keyboard | WASD movement | planned optional | none | none | none | none | defer-phase | Phase 10 implement or document blocker |

## Passive Economy Event Refresh Policy - 2026-06-20

Current client-safe rule:

- Event payloads may be viewer-relative or owner-private, so Phase 01 refreshes
  authoritative query snapshots instead of trusting partial passive payloads.
- `client/src/app/client-app.ts` debounces economy event refreshes by operation
  so event bursts from one mutation do not spam duplicate queries.
- `economy.flow_updated` is admin-only. Normal players must not receive or query
  admin economy totals from this dirty signal.
- Backend true passive fanout remains a named blocker: runtime economy handlers
  queue most market, auction, and premium events to the acting session only via
  `queueEventLocked(sessionID, ...)`. Multi-session buyer/seller/passive viewer
  fanout needs a future owner-aware broadcast policy.

| Event | Client refresh query | Direct merge allowed later | Backend/UI blocker |
| --- | --- | --- | --- |
| `market.listing_created` | `market.search`, `inventory.snapshot` | Upsert listing if payload is viewer-safe | Other shop viewers do not receive passive listing fanout yet |
| `market.listing_updated` | `market.search` | Upsert listing status/quantity | No active emitter/fanout policy verified yet |
| `market.sale_completed` | `market.search`, `wallet.snapshot`, `inventory.snapshot` | Listing only; wallet/inventory must be snapshot-owned | Seller/passive sessions are not notified; seller wallet/inventory fanout absent |
| `market.listing_cancelled` | `market.search`, `inventory.snapshot` | Mark listing cancelled or remove active listing | Only cancelling session receives event today |
| `auction.lot_updated` | `auction.search` | Global lot fields only, not viewer-relative `leading` | Fanout must personalize `leading` per session or require query refresh |
| `auction.bid_placed` | `auction.search`, `wallet.snapshot` | Lot payload for acting bidder only; reducer clears pending bid | Previous bidder refund and passive viewer lot updates need fanout/query policy |
| `auction.closed` | `auction.search`, `wallet.snapshot` | Lot safe; grant is winner-private only | Winner grant privacy and refunded bidder wallet fanout need owner-aware broadcast |
| `premium.entitlement_created` | `premium.entitlements` | Owner-only entitlement upsert | Provider/runtime fanout to owning online session is not documented |
| `premium.entitlement_claimed` | `premium.entitlements`, `wallet.snapshot` | Owner-only entitlement state update | Non-wallet grants still depend on future concrete grant adapters |
| `premium.stock_consumed` | `premium.entitlements`, `wallet.snapshot` | Global stock record only; purchase row stays owner-query-owned | World-stock decrement should fan out to all shop viewers; buyer purchase is private |
| `economy.flow_updated` | `admin.economy_dashboard` for admins only | Admin dashboard payload only | No handler emission and no player-visible scope are defined |

## Transport Readiness Evidence - 2026-06-20

Readiness contract:

- Raw WebSocket `open` is not enough for real-mode gameplay commands.
- Real-mode command sending is blocked until server-authenticated
  `session.ready` has moved client status to `connected`.
- Logout, auth expiry, new session load, and reconnect clear stale gameplay and
  pending command state through `clearGameplay`.
- Reconnect bootstrap sends `session.ready` plus authoritative snapshots with a
  reconnect cursor; stale client entities are replaced by the fresh
  `world.snapshot`.

Evidence:

| Requirement | Current evidence |
| --- | --- |
| Block real-mode commands before `session.ready` | `canSendRealtimeCommand` only allows real-mode sends when status is `connected`; unit test covers `restoring`, `logged_out`, `offline`, `connecting`, `authenticated_pending_socket`, `reconnecting`, `error`, and `auth_expired` as blocked states |
| Enable after server readiness | `session.ready` reducer transition sets authenticated session state and `connected`; command gate allows `connected` |
| Clear pending on logout/auth expiry | reducer test seeds pending gameplay state and verifies `authLoggedOut` / `authExpired` clear server-owned state and pending commands |
| Reconnect bootstrap/cursor | server test verifies reconnect cursor sequence; reducer test verifies reconnect resets stale sequence and replaces stale visible state; browser smoke verifies reload reconciliation with real server snapshots |
| Terminal auth expiry | server test verifies a command after logout returns session revoked and transport closes terminal auth errors; browser smoke now externally revokes the live session, sends `world.snapshot` over the still-open socket, observes `ERR_SESSION_REVOKED` / 1008 close evidence, enters `auth_expired`, clears pending commands, and removes gameplay state |

## Protocol Parity Table - 2026-06-20

This table records the currently exposed browser protocol surface. Future
mutations stay out of `OPERATIONS` until the owner phase wires server
validation, reducer reconciliation, and smoke coverage.

| Surface | Operations / events | Request schema | Success/event payload | Reducer target | Errors / retry posture | Server owner | Tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Auth/bootstrap | HTTP login/register/logout, WS `session.ready`, `session.snapshot` | mail/password over HTTP; no trusted player id in WS payload; `session.snapshot` payload must be empty | public session/account/player, protocol version, reconnect cursor | `auth.session`, connection state, cleared gameplay | auth errors are terminal; no token/password logging; `session.snapshot` rejects trusted payload fields | auth HTTP + realtime bootstrap | reducer auth tests, browser smoke login/logout, server bootstrap tests, production session snapshot test |
| World snapshots | `world.snapshot`, `aoi.entity_entered`, `aoi.entity_updated`, `aoi.entity_left` | empty query | sector, minimap, visible entities only | sector, minimap, `visibleEntities`, selection cleanup | hidden/not visible data omitted; snapshot can be retried | runtime AOI/visibility | reducer snapshot tests, server AOI tests, browser smoke |
| Movement | `move_to`, `stop`, `position.corrected`, `movement.stopped` | target vector for move; empty stop | server-computed position/movement timing | movement target, self entity position/route | rate limited, range/speed/server position validated, request id safe | world worker/runtime gateway | server movement tests, reducer response-lost test, browser smoke movement |
| Combat/loot | `combat.use_skill`, `loot.pickup`, combat events, loot events | target/drop ids only | cooldown, damage/miss/kill, loot create/remove/pickup, snapshots | combat log/effects, target state, cargo/inventory/progression | cooldown, energy, range, visibility, duplicate pickup guarded | combat and loot services | server combat/loot tests, reducer tests, browser smoke combat/loot |
| Death/repair | `death.repair_quote`, `death.repair_ship`, `death.ship_disabled`, `death.repaired` | empty requests | disabled state, repair quote, ship/player/wallet snapshots | ship disabled/quote, repair state | disabled movement returns `ERR_SHIP_DISABLED`; repair debits wallet server-side | death/repair service | server death bridge/repair tests, reducer disabled/repair tests |
| Progression/inventory/system snapshots | `progression.snapshot`, `inventory.snapshot`, `cargo.snapshot`, `wallet.snapshot`, `hangar.snapshot`, `loadout.snapshot`, `stats.updated`, `crafting.recipes` | empty queries | server-owned summaries | matching client summaries | query retry safe; no fake counts when absent | progression, inventory, wallet, hangar, loadout, stats, crafting | reducer snapshot tests, browser smoke |
| Hangar/loadout mutations | `hangar.activate_ship`, `loadout.equip_module`, `loadout.unequip_module` | owned ship id, slot id, item instance id | hangar/ship/cargo/loadout/inventory/stats snapshots | hangar, active ship, loadout, stats, inventory | ownership, rank, compatibility, capacity, idempotent request handling | hangar/loadout services | Go registry/server tests, TS protocol tests, browser smoke drag/drop |
| Stealth and scanner witness | `stealth.toggle`, `scan.pulse` with `player_revealed`, `stats.updated`, self AOI update, viewer-only `aoi.entity_entered`/`updated` with `scan_revealed` | `stealth.toggle` accepts `{enabled:boolean}` only; `scan.pulse` stays empty | safe `stealth.enabled`, stats, self-only `stealthed`, safe scan `player_revealed`, viewer-only revealed entity | stats, self cloak HUD state, scan mode, visible entities/minimap | hidden truth, target player id, witness expiry, scan rolls, and hidden metadata are forbidden; module/cooldown/rate rules still open gates | runtime visibility, world worker, discovery scanner bridge | server stealth/scanner tests, TS protocol/reducer tests, browser smoke quick-action guard; two-session browser witness smoke still open |
| Scanner/discovery/planets/routes queries | `scan.pulse`, `discovery.known_planets`, `discovery.planet_detail`, `planet.production_summary`, `planet.storage_summary`, `route.list`, `route.snapshot` | empty or server-known ids | scan status/results, known planets, selected detail, production/storage/routes; hidden-player reveal returns no planet intel/XP | scan mode/intel, production, planet storage, routes | cooldown/energy/visibility checks; hidden data filtered | discovery, production, route services | reducer scan/planet/route tests, server scan/route tests |
| Market | `market.search`, `market.create_listing`, `market.buy`, `market.cancel`, market events | item filter or listing/item/quantity/unit price intent | listings, counts, wallet/inventory snapshots | market, wallet, inventory; passive events trigger refresh | ownership, tradeability, funds, escrow, request id/domain idempotency | market service | server economy tests, reducer refresh tests, browser smoke shop |
| Auction | `auction.search`, `auction.bid`, `auction.buy_now`, `auction.grants`, auction events | auction id and bid amount where needed; empty read-only grant query | lots, grants, wallet snapshots | auction, wallet; passive events trigger refresh | bid/funds/lot state validation; one-click client pending guard; grant query is read-only | auction service | server economy tests, reducer pending tests, browser smoke double-bid guard |
| Premium | `premium.entitlements`, `premium.claim`, `premium.purchase_weekly_xcore`, premium events | entitlement id or stock `{product_id, period_key}` intent | entitlements, stock, purchases, wallet snapshots | premium, wallet; passive events trigger refresh | owner checks, weekly limit, stock, provider replay/domain idempotency; price/currency/world stay server-owned | premium service | server premium tests, browser smoke premium |
| Quests | `quest.board`, `quest.progress`, `quest.accept`, `quest.claim_reward`, `quest.reroll`, quest events | offer/quest ids or empty queries | board, active quests, wallet/inventory/progression snapshots | quest board, rewards, progression | reward idempotency, reroll cost/rate, server progress only | quest service | server quest tests, reducer tests, browser smoke |
| Admin/observability | admin/observability queries, `admin.repair_craft_job`, admin/observability events | empty queries or admin job id | admin-only summaries, metrics, release gate | admin panels only | role-gated; normal player UI cannot see admin totals | admin/observability services | server admin tests, admin browser smoke |
| Dev/debug protocol | `debug_snapshot`, `debug_spawn_npc` | empty or dev-only debug payloads | dev diagnostics only | demo/dev diagnostics, not normal player state | forbidden in non-dev real server with public `ERR_FORBIDDEN`; normal real client must not send debug ops | realtime gateway/dev harness | production-negative WebSocket test and real-mode smoke recorder |
| Guarded future mutations | `inventory.move`, `progression.unlock_skill`, skill respec, crafting mutations, planet claim/build/upgrade, route mutations, intel/coordinate, mail/social | not registered | rejected before handler | none | remain absent until owner phase provides server contract | future owner phases | Go registry test, TS protocol test, browser smoke dead-control guard |

## Enabled Control Guard Evidence - 2026-06-20

Browser smoke now treats visible enabled controls as a protocol contract, not
just a visual check:

- The retired monolithic browser smoke used to allowlist enabled `data-action`
  values that are real server operations or local-only presentation controls.
  Future per-flow browser/e2e coverage must restore this guard without reviving
  the monolith.
- Any visible, enabled `data-action` outside the allowlist fails smoke as an
  unknown enabled action.
- Guarded future mutations are still absent from the browser operation registry
  and from enabled UI controls: crafting start/complete/cancel,
  `inventory.move`, progression unlock/respec, planet claim/build/upgrade,
  route mutations, intel/coordinate item mutations, and mail/social mutations.
- Future affordances may remain visible only when disabled and player-facing.
  Mail/social, planet claim/build/route/automation, rocket/shield/warp, and
  other owner-phase controls no longer advertise internal contract/debug copy.
- Smoke also bans public player copy such as `server recalculates`,
  `server-owned`, `server policy`, `no server-owned routes`, `server-side`,
  `server lock`, `server move`, and `server contract`.

## Protocol Denylist Parity Evidence - 2026-06-20

Current parity rule:

- Client request payloads may carry intents only. Go rejects recursive
  server-owned keys such as player/account/session ids, world/zone ids, speed,
  hidden state, procedural/scan internals, loot rolls, provider/payment refs,
  wallet/economy truth, progression truth, and auth secrets.
- TypeScript command builders reject the same shared sensitive fields before a
  request is serialized. Outbound-only gameplay truth fields such as `xp`,
  `rank`, balances, fees, bid ownership, quest progress, and reward payloads
  stay stricter than inbound server parsing.
- TypeScript server-message parsing rejects hidden/internal/session/provider/
  password/procedural/scan/loot-roll fields in response and event payloads, but
  allows public server-owned progression summaries such as `rank`, `main_xp`,
  `combat_xp`, and nested XP rewards because the server is the source of that
  truth.
- `admin.inspect_player.target_player_id` is the only current explicit
  exception. It is allowed only as a top-level command field on the admin
  inspect handler/builder path; nested `target_player_id` and nested
  `player_id` still fail as server-owned payload.

Evidence:

| Requirement | Current evidence |
| --- | --- |
| Go rejects shared sensitive request fields recursively | `TestRejectTrustedPayloadSharedSensitiveFieldsAndAdminException` covers hidden/player/session/world/scan/provider/password/loot-roll/economy/progression-sensitive fields |
| Admin-only target inspect exception is handler-scoped | `TestPhase09QuestAdminObservabilityUseServerState` proves non-admin target inspect is forbidden and admin target inspect succeeds; helper tests prove nested target/player ids still reject |
| TS command payloads reject shared sensitive outbound fields | `client/src/protocol/request-id.test.ts` covers recursive shared sensitive keys plus outbound-only truth such as progression/economy/quest/server result fields |
| TS inbound parser rejects hidden/internal leakage without blocking public progression | `client/src/protocol/envelope.test.ts` rejects sensitive server payload keys and accepts public progression `rank`/XP summaries |

## Internal Copy Cleanup Evidence - 2026-06-20

Normal player economy payloads and UI no longer expose the old
`server_recalculates` implementation flag:

- Market listings, market mutation responses, auction lots, and auction
  mutation responses now serialize `final_price_pending`.
- Client state stores `final_price_pending`; the reducer only reads the old
  payload field as a backwards-compatible fallback.
- Shop details render player copy such as `finalized on buy`, `escrow held`,
  and `ready`, never `server recalculates`.
- Browser smoke bans `server_recalculates` in body text, the smoke state,
  storage, and visible attributes.
- The cloak quick-action tooltip no longer says `server-side`; it uses
  player-facing cloak copy.

Evidence:

| Requirement | Current evidence |
| --- | --- |
| Normal player payload no longer returns `server_recalculates` | `assertNoEconomyLeak` rejects `server_recalculates`; `TestPhase08MarketAuctionPremiumUseServerEconomyState` passes with `final_price_pending` payloads |
| Client state/smoke no longer depends on raw internal field | Retired monolithic smoke covered `final_price_pending` and `server_recalculates`; future per-flow browser/e2e coverage must restore this guard |
| Visible UI/tooltip copy is player-facing | `client/src/ui/hud.ts` uses `finalized on buy`, `escrow held`, and cloak movement copy without `server-side` |

## Rate And Backend Event Blockers - 2026-06-20

Phase 01 now names the exact remaining backend/UI blockers instead of implying
complete abuse/event coverage.

Rate-limit posture:

| Operation / source | Current protection | Phase 01 blocker |
| --- | --- | --- |
| `scan.pulse` | Server scanner cooldown, energy/capacitor validation, client pending scan loop | No explicit op-rate acceptance test beyond gameplay cooldown |
| `market.search` | Client economy refresh debounce | No server query-rate limit or test yet |
| `quest.reroll` | Wallet cost and request-reference idempotency | No server reroll rate/cooldown posture test yet |
| `combat.use_skill` | Combat cooldown and energy/range validation | No separate burst-rate posture test yet |
| `loot.pickup` | Visibility/range/capacity checks and duplicate pickup safety | No burst-rate posture test yet |
| `stop` | Server-owned stop mutation is idempotent-ish and safe | Needs explicit exemption or a shared movement throttle decision |
| Hotkeys `1..6` | UI ignores repeats, focused UI, and disabled quick actions | No hotkey-specific rate acceptance test yet |

Backend event refresh blockers:

| Transition | Current evidence | Phase 01 blocker |
| --- | --- | --- |
| Loot owner-lock expiry | Domain event/task exists | No runtime/realtime/client bridge to `loot.updated` or refresh |
| Loot expiry | Domain event exists; pickup emits `loot.removed` | Timed expiry does not emit public removal or refresh |
| Planet production settled | Production domain event exists | No public realtime event or client refresh bridge |
| Offline settlement completed | Production domain event exists | No public realtime event or client refresh bridge |
| Route transfer/lost/full/empty | Route settlement domain events exist | No public realtime event or client refresh bridge |

Named blocker: **Task001 Phase01 backend event refresh bridge missing for
passive loot/production/route domain transitions**.

## Premium Stock Intent Evidence - 2026-06-20

Weekly X Core stock purchase is no longer an empty singleton command:

- Client builder sends only `{product_id, period_key}`.
- HUD purchase button copies the selected premium stock row `period_key` into
  the command dataset with product id `weekly_xcore`.
- Server rejects empty or wrong product/period payloads before mutation.
- Server still derives world id, price, payment currency, stock record, wallet
  debit, and weekly limit from runtime/service state.

Evidence:

| Requirement | Current evidence |
| --- | --- |
| Empty stock purchase intent rejected | `TestPhase08MarketAuctionPremiumUseServerEconomyState` sends empty payload and expects `ERR_INVALID_PAYLOAD` |
| Selected row identity accepted | Same test sends `{product_id:"weekly_xcore", period_key:<current>}` and verifies wallet debit, stock decrement, and duplicate weekly limit |
| Client cannot author price/currency/world | TypeScript builder exposes only product/period; shared denylist and strict decode reject trusted/unknown fields |

## Auction Grant Query Evidence - 2026-06-20

Auction grant refresh is now framed as a read-only snapshot query:

- Realtime operation is `auction.grants`, not `auction.claim_grant`.
- Server handler returns the authenticated player's auction grant snapshot and
  performs no wallet, inventory, unlock, or grant state mutation.
- Client builder/HUD action use `auctionGrants()` and `auction-refresh`.
- UI copy says `Refresh`, not `Claim`, for skeleton grant records.
- Real claim/grant adapters for ships, modules, X Core, cosmetics, intel, and
  building blueprints remain future owner-phase work.

Evidence:

| Requirement | Current evidence |
| --- | --- |
| Read-only query name | Operation registry, command builder, and browser UI use `auction.grants` / `auction-refresh` |
| Player-scoped grant snapshot only | `TestPhase08MarketAuctionPremiumUseServerEconomyState` queries `auction.grants` after buy-now and verifies one player grant |
| No fake claim mutation in normal UI | HUD button title/action render as refresh; no normal-player `auction-claim` action remains |

## Likely Files

```text
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/src/protocol/envelope.test.ts
client/src/app/client-app.ts
client/src/render/world-renderer.ts
client/src/render/world-view.ts
client/src/state/types.ts
client/src/state/reducer.ts
client/src/state/reducer.test.ts
client/src/ui/hud.ts
internal/game/realtime/envelope.go
internal/game/realtime/envelope_test.go
internal/game/server/*_handlers.go
internal/game/server/death_events.go
internal/game/server/discovery_production_handlers.go
internal/game/server/economy_handlers.go
internal/game/server/combat_loot_helpers.go
internal/game/server/server_test.go
internal/game/death/service.go
docs/plans/task-001/01-gameplay-connection-audit.md
```

## Acceptance Criteria

- [x] A committed matrix exists in this phase file or a linked audit doc.
- [x] An envelope/protocol parity table exists for browser operations/events.
- [x] Session/reconnect tests prove real mode blocks gameplay commands before
      `session.ready`, enables them after readiness, clears pending commands on
      logout/auth expiry, and reconciles reconnect bootstrap/cursor state.
- [x] Browser-level revoked-session/auth-expiry smoke proves a live revoked
      session emits `auth_expired`, closes terminally, clears pending commands,
      and removes stale gameplay state.
- [x] No visible enabled control lacks a real server-backed operation.
- [x] Protocol denylist parity is tested across Go request rejection,
      TypeScript command rejection, and TypeScript server-message rejection,
      with explicit admin-only exceptions.
- [x] Normal player payload/UI no longer exposes `server_recalculates` /
      `server recalculates`, or the temporary blocker is named with owner phase.
- [x] `auction.claim_grant` is either a real claim mutation or renamed/reframed
      as a read-only grant snapshot query.
- [x] Premium stock purchase intent is product/period-specific before multiple
      stock rows exist, or the one-product empty-payload blocker is explicit.
- [x] Dev/debug operations are classified in the parity table and forbidden by
      production WebSocket tests.
- [x] Real-mode browser smoke proves normal client flows do not emit debug
      operations.
- [x] Per-operation rate-limit posture is documented and tested or blocked for
      scan, market search, quest reroll, combat, loot, stop, and hotkey-driven
      command paths.
- [x] Loot expiry/owner-lock expiry, production settlement, offline settlement,
      and route transfer events either reconcile through safe public events,
      trigger named refresh queries, or remain explicit backend blockers.
- [x] `planet.storage_summary` updates client state.
- [x] `route.snapshot` updates client state.
- [x] `death.ship_disabled` makes disabled/repair state visible or triggers a
      documented refresh path.
- [x] Passive market/auction/premium events reconcile, refresh, or document an
      explicit backend fanout blocker.
- [x] One auction bid click emits exactly one `auction.bid`.
- [x] Guard tests name every unimplemented browser mutation still blocked.
- [x] Reducer/event tests cover duplicate or stale `seq`, unknown events,
      response-lost/event-delivered recovery, and reconnect snapshot refresh.
- [x] Every passive event either carries enough client-safe payload to reconcile
      state or names the exact refresh query the client must issue.
- [x] Crafting, progression unlock, intel/coordinate, mail/social, and menu
      affordances are implemented in their owner phase or explicitly deferred
      with guard tests and hidden/locked UI.

## Verification

```bash
go test ./internal/game/realtime -run 'TestOperationRegistry' -count=1
go test ./internal/game/server -run 'Test.*(Route|PlanetStorage|Auction|Repair|Death)' -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/protocol src/state
```

Second-pass smoke additions:

- Browser revoked-session/auth-expiry scenario.
- `auction.buy_now`, `quest.claim_reward`, and death repair quote/repair where
  visible controls exist.
- Go/TypeScript forbidden payload key parity, including hidden-player witness
  and scan internals.
