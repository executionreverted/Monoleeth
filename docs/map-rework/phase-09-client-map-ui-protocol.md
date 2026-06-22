# Phase 09: Client Map UI And Protocol

## Goal

Expose bounded multi-map gameplay in the browser without fake gameplay state.
The client must understand the current map, map bounds, safe/PvP policy,
portals, radar-driven minimap contacts, selected visible entities, scanner
state, and portal handoff status entirely from authenticated server snapshots,
responses, and events.

## Current State To Replace/Reuse

Replace or extend:

- `client/src/protocol/envelope.ts` now includes `portal.enter` plus map
  snapshot/changed/transfer, map policy, and portal cooldown client event
  constants. Continue to keep portal entry as an intent-only operation and do
  not add trusted map, destination, player, position, speed, or cooldown fields
  to client payloads.
- `client/src/protocol/envelope.ts:130-161` models entities as player, NPC,
  loot, or planet signal with position, display, combat, movement, and public
  status flags, but not portal entities or map-boundary objects.
- `client/src/state/types.ts` now has nullable server-owned `currentMap`, map
  bounds, public map keys, visible portals, safe zones, safe-zone/protection
  summaries, portal cooldowns, map transfer state, map subscription epoch, and
  minimap map extensions.
- `client/src/state/reducer.ts:1538-1551` parses `sector` and `minimap`
  snapshots from generic payloads; this is the natural extension point for map
  summary parsing.
- `client/src/state/reducer.ts:1001-1008` replaces visible entities from
  `world.snapshot`; map snapshot application must clear old-map entities before
  applying new-map entities.
- `client/src/state/reducer.ts:1010-1047` applies AOI entity enter/update/left
  events directly; new handlers must reject or ignore stale events from prior
  map streams.
- `client/src/render/world-renderer.ts` now exposes `mapOverlay` debug state
  instead of UI-facing fog state, and `client/src/render/map-overlay.ts` projects
  map bounds, visible portals, and safe zones from `currentMap`/`minimap`.
  Continue to keep scanner/radar feedback separate from removed fog-wave UI.
- The center world camera remains player-centered for navigation while the
  overlay/minimap layers project bounded map information for UX only; server
  bounds remain authoritative.

Reuse:

- `client/src/protocol/envelope.ts:207-264` rejects forbidden keys including
  `world_id`, `zone_id`, hidden/procedural seeds, spawn candidates, detection
  and scan rolls, loot rolls, and loot tables.
- `client/src/protocol/envelope.ts:266-317` rejects forbidden payload keys on
  responses and events before state receives them.
- `client/src/state/reducer.ts:372-397` already rebuilds visible entities from
  server snapshots and drops stale selected targets.
- `client/src/state/reducer.ts:399-493` already derives minimap live contacts
  from visible entities while preserving public projection source only.
- `client/src/state/reducer.ts:722-757` already handles server-owned scan pulse
  events and known planet minimap refreshes.
- `client/src/state/reducer.ts:3286-3331` clears gameplay state on auth, demo,
  logout, auth expiry, and failure.
- `client/src/render/world-renderer.ts:175-235` renders only the entities given
  by state and shows `AWAITING SERVER SNAPSHOT` when there are none.
- `client/src/render/world-renderer.ts:677-710` draws scanner/radar pulse
  feedback without treating the client as scanner truth.

## Target Model

The browser receives a map-scoped authenticated snapshot after login, reconnect,
and portal handoff. The snapshot includes a public map summary, current bounds,
safe/PvP policy, portals visible or known to the current player, current
server-owned entity AOI, radar minimap contacts, remembered known planet intel,
and available actions. The client never chooses a trusted map id for movement,
combat, scan, loot, planet, or portal commands.

The HUD should show the current map as the primary location concept, with sector
as optional flavor underneath it. The center canvas remains the playable map
surface. It renders the self ship, visible NPCs, visible player contacts,
visible loot, visible or known portal markers, and rare scanner/planet signals
only when present in server payloads. Empty/offline/unauthenticated states show
locked or loading UI, not demo contacts.

The minimap is radar-driven. It shows current-map live contacts in radar range,
current-map portal markers that the server chooses to reveal, map bounds, safe
zone/PvP indicators, and remembered known planet intel when safe. It must not
show cross-map entities, hidden planets, future spawns, seeds, or fake contacts.

## Data Structures/Contracts To Add Or Change

Protocol additions:

- Add operations:
  - `map.snapshot`
  - `portal.enter`
  - optional `portal.list` if map snapshots become too large
- Add client events:
  - `map.snapshot`
  - `map.changed`
  - `map.transfer_started`
  - `map.transfer_completed`
  - `map.transfer_failed`
  - `portal.cooldown_started`
  - `map.policy_updated`
- Extend `EntityType` with:
  - `portal`
  - optional `map_object` only if needed for non-interactive server objects
- Add public map summary:

```ts
interface MapSummary {
  public_map_key: string;
  display_name: string;
  bounds: { min_x: number; min_y: number; max_x: number; max_y: number };
  risk_band: string;
  pvp_enabled: boolean;
  safe_zone: boolean;
  safe_zones: Array<{ id: string; shape: string; label?: string }>;
  pvp_zones: Array<{ id: string; shape: string; label?: string }>;
  theme?: { background_key?: string; accent?: string };
  portals: PortalSummary[];
}
```

- Add public portal summary:

```ts
interface PortalSummary {
  portal_id: string;
  label: string;
  position: Vec2;
  radius: number;
  destination_label: string;
  state: 'available' | 'cooldown' | 'locked' | 'offline';
  cooldown_ready_at_ms?: number;
  locked_reason?: string;
}
```

- Extend minimap summary:
  - `bounds`
  - `public_map_key`
  - `portals`
  - `safe_zones`
  - `pvp_zones`
  - `radar_range`
  - `live_contacts`
  - `remembered`
- Add client state:
  - `currentMap: MapSummary | null`
  - `portalCooldowns: Record<string, number>`
  - `mapTransfer: { portal_id: string; state: string; started_at_ms: number } | null`
  - `mapSubscriptionEpoch: string | null`
- Extend status flags with safe public values such as:
  - `portal`
  - `safe_zone`
  - `pvp_zone`
  - `portal_locked`

Command payloads:

- `portal.enter` payload is intent only:

```json
{
  "portal_id": "portal_x1_to_x2",
  "request_id": "client-generated retry id"
}
```

- The client must not include trusted `map_id`, `internal_map_id`,
  `public_map_key`, destination map id, player id, position, speed, cooldown,
  or spawn position. Server resolves all of that from session and current map
  membership.

Snapshot/event rules:

- `map.snapshot` and `world.snapshot` may be separate events or one combined
  event with a `map` object, but the reducer must apply them atomically enough
  to avoid rendering old-map entities under new-map labels.
- `map.changed` must clear `visibleEntities`, `knownLoot`, `selectedTargetID`,
  pending movement target, combat effects, and map-scoped minimap contacts
  before applying destination map snapshot.
- AOI events must be stream-scoped by server-issued `map_subscription_epoch`
  after handoff. Stale prior-map AOI events must not reinsert entities into the
  destination map.
- Forbidden payload key checks must keep blocking hidden map ids where they are
  not public, procedural seeds, future spawn data, planet candidates, scan
  rolls, loot rolls, and loot tables.

## Implementation Tasks In Order

1. Define public `MapSummary`, `PortalSummary`, map policy, and minimap extension
   types in the client protocol/state layer.
2. Add operation/event constants for map snapshot, portal enter, map transfer,
   portal cooldown, map changed, and map policy update.
3. Add command builders/tests for `portal.enter` that accept only
   `portal_id` and request metadata.
4. Add reducer parsing for `map` snapshots beside existing `sector` and
   `minimap` parsing.
5. Add map-change reducer behavior that clears old-map entities, selected
   target, known loot, movement target, transient effects, and stale portal
   transfer state.
6. Add stale-event protections for AOI and loot/combat events around portal
   handoff. Use server-issued `map_subscription_epoch` rather than trusting
   client-supplied map ids.
7. Extend the renderer to draw map bounds, portal markers, safe/PvP region
   hints, and radar/minimap overlays from `currentMap` and `minimap` only.
8. Remove fog terminology from UI-facing renderer/debug state as part of the map
   contract. Scanner pulse/radar feedback remains.
9. Update the HUD location/topbar text to prefer map display name and risk band;
   sector is secondary if still present.
10. Update minimap UI to show bounded map frame, radar range, live contacts,
    public portals, safe/PvP zone indicators, and remembered known planet intel.
11. Add portal interaction UX: click/select visible portal, show destination
    label/state/cooldown, and send `portal.enter` only when server state says
    available.
12. Add disconnected/loading/locked/empty states for map snapshot, portal list,
    and minimap.
13. Update browser smoke/screenshots for login, movement, combat, scan, portal
    transfer, safe-zone PvP blocked state, and no cross-map leakage.

## Tests To Add/Update

- Protocol parser accepts public `map`, `bounds`, `portals`, safe/PvP flags, and
  radar minimap fields.
- Protocol parser rejects hidden seeds, spawn candidates, planet candidates,
  scan rolls, loot rolls, loot tables, and trusted identity/position fields in
  map/portal/combat/loot payloads.
- Command builder for `portal.enter` rejects client-authored map id,
  destination map id, player id, position, speed, cooldown, or spawn position.
- Reducer applies `map.snapshot` and `world.snapshot` without fake defaults.
- Reducer clears old visible entities, selected target, known loot, movement
  target, and effects on `map.changed`.
- Reducer ignores stale pre-handoff AOI events and does not resurrect old-map
  NPCs, loot, players, or planet signals.
- Reducer stores `map_subscription_epoch` from server snapshots and ignores
  old-epoch AOI/combat/loot/scan/portal events.
- Minimap contacts are rebuilt from same-map visible entities and server minimap
  payload only.
- Portal cooldown events update portal state without trusting client timers as
  gameplay truth.
- Renderer screenshot tests show map bounds and portal markers when supplied,
  and no fake portals when absent.
- Browser smoke verifies the default unauthenticated/offline UI has no fake map,
  NPC, planet, portal, cargo, wallet, or quest values.
- Browser smoke verifies a real server portal handoff changes map labels,
  removes old-map contacts, and shows destination-map AOI only.

## Migration/Doc Updates

- 2026-06-22 Phase09A progress: client state now has nullable
  server-owned `currentMap`, map-safe portal/safe-zone/protection summaries,
  portal cooldown UI state, nested and direct public `map.snapshot` parsing,
  and reducer tests for default empty state, malformed public map entries,
  forbidden payload keys, and transfer snapshot replacement. Follow-up
  code-quality fix clears map-scoped live state before changed-map or
  changed-epoch `map.snapshot` application, handles `map.changed` with or
  without a destination summary, and preserves same-map same-epoch metadata
  refreshes. Renderer and minimap drawing remained open at that point; portal
  click UX remained open.
- 2026-06-22 Phase09B progress: the Pixi renderer now receives server-owned
  `currentMap`, publishes it through smoke state, draws a lightweight canvas map
  overlay from `currentMap`/`minimap` bounds, visible portals, and safe zones
  only when those fields are present, and exposes `worldView.mapOverlay` debug
  state instead of the old UI-facing `fog` field. Portal click UX, HUD
  location/topbar changes, and minimap DOM work remained open.
- 2026-06-22 Phase09C progress: the HUD topbar now prefers server-owned map
  display/public keys and current map risk/PvP/safe-zone/protection state before
  falling back to legacy sector labels. The DOM minimap now renders a bounded
  map frame from `currentMap` or `minimap.bounds` even when the server reports
  zero contacts, projects contacts/memory/portal/safe-zone markers by bounds
  when available, filters remembered planet memory by current public map key,
  and keeps portal markers display-only with no portal command action. Portal
  click UX and browser smoke for portal traversal remain open.
- 2026-06-22 Phase09D progress: client portal summaries now retain sanitized
  public `label`/`display_name`, `destination_label`, `state`,
  `cooldown_ready_at_ms`, and `locked_reason` fields while still dropping
  malformed required portal entries. Portal summary parsing rejects hidden
  destination/spawn/map aliases and nested destination/spawn objects before state
  mutation. The DOM minimap now renders server-provided portal markers as
  selectable HUD-local controls scoped by current public map identity plus
  `map_subscription_epoch`, shows a compact portal detail/action strip from
  server-owned public fields only, disables Enter for stale selections,
  unavailable states, active portal cooldowns, pending `portal.enter`, or
  disconnected realtime state, and dispatches `portal.enter` through the guarded
  command path with only `{ portal_id }`. Final review patch also rejects
  outbound cooldown timing aliases `ready_at_ms` and `expires_at` and renders
  the basic laser cooldown/readiness from the same server-time basis used by
  cooldown scheduling. Browser smoke for real portal traversal and cross-map
  leakage remains open.
- 2026-06-22 Phase09E progress: `map.policy_updated` now applies
  same-map/same-epoch public policy deltas to an existing server-owned
  `currentMap` without treating the event as a handoff. The reducer accepts
  public `region`, `risk_band`, `pvp_policy`, `visual_theme_key`, viewer
  `safe_zone`/`protection`, and `safe_zones` updates, mirrors only supplied
  safe-zone projections into the minimap, preserves live AOI/minimap contacts,
  loot, target, movement, transfer, and portal cooldown state, rejects hidden
  internal map payload fields before mutation, and ignores cross-map public key
  mismatches. Backend emission and browser smoke for live policy updates remain
  open.
- 2026-06-22 Phase09F progress: Task 12 now distinguishes missing map snapshot
  UI states without adding fake map data. Restoring, connecting,
  authenticated-pending-socket, and connected-before-snapshot states render
  loading/awaiting minimap and portal-list copy; logged-out and auth-expired
  states render locked copy; offline, error, and reconnecting states render
  disconnected copy. Bounds-only server snapshots still render bounded minimap
  metadata with no fake contacts and a no-visible-portals strip. When a current
  map snapshot remains visible while realtime is not ready, portal selection is
  cleared and portal entry stays locked while the last server-owned markers may
  remain visible. Focused Vitest coverage lives in
  `client/src/ui/hud-render-planets.test.ts`; browser smoke for real portal
  traversal remains open.
- Update UI implementation docs after the protocol lands to state that
  `currentMap`, portals, minimap bounds, and safe/PvP flags are server-owned.
- Update local run/smoke docs with a deterministic two-map seed that includes at
  least one portal, one safe zone, one PvP zone, and one enemy pool per map.
- Update world/AOI docs to replace fog terminology with radar, stealth,
  same-map membership, and public remembered intel.
- Update final mockup parity notes to describe bounded map HUD behavior without
  adding visual filler or fake contacts.

## Risks And Acceptance Criteria

Risks:

- The client can briefly render old-map entities after a portal handoff if map
  and world snapshots are not applied in a safe order.
- Public map ids are useful UI state, but private destination internals,
  procedural seeds, future spawns, and hidden planet candidates must stay off
  the wire.
- Client-side bounds clamping can accidentally look authoritative if server
  rejection/correction paths are not visible.
- Portal markers can become fake navigation UI if rendered from static client
  fixtures instead of snapshots.
- Removing fog UI must not remove radar/scanner gameplay feedback.

Acceptance criteria:

- Authenticated state carries a real server-owned current map summary with
  `10000x10000` bounds or the configured map bounds.
- The browser shows map name, risk band, safe/PvP policy, minimap bounds,
  portals, and radar contacts only from server payloads.
- Portal entry is an intent-only command and the server owns destination,
  spawn position, cooldown, transfer state, and session membership.
- After portal handoff, old-map NPCs, players, loot, combat effects, selected
  targets, and minimap contacts are gone.
- No default client path shows fake portals, fake enemies, fake planets, fake
  wallet/cargo, or fake quest counts.
- Full verification for the phase includes:

```bash
go test ./...
git diff --check
cd client
npm --cache /tmp/gameproject-npm-cache run check
```
