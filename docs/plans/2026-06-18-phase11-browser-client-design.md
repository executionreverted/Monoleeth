# Phase 11 Browser Client Design

Date: 2026-06-18

## Context

Phase 11 builds the first browser-first client prototype. `GOAL.md` asks for
autonomous implementation, so this document records the choices made without a
separate approval stop.

The client must render only server-approved state, send intent commands with
request ids, and avoid receiving or storing hidden gameplay data. Current backend
realtime contracts expose Phase 04 operations and events:

- Requests: `move_to`, `stop`, `debug_snapshot`, `debug_spawn_npc`
- Events: `player.snapshot`, `aoi.entity_entered`, `aoi.entity_left`,
  `position.corrected`
- Snapshot payloads: client-safe AOI entities with `entity_id`,
  `entity_type`, `position`, and public `status_flags`

Combat, loot, scan, cargo, market, and quest UI can be represented as panels and
disabled controls, but the first client slice must not send unsupported or
client-trusting operations.

## Options Considered

1. **Vite + TypeScript + PixiJS + vanilla DOM HUD**
   - Smallest browser client workspace.
   - Matches the current Phase 04 contract without introducing app framework
     state before it is useful.
   - Recommended for this slice.

2. **Vite + React + PixiJS**
   - Better once inventory, quest, market, and station panels become large.
   - Adds framework dependency and more integration surface before the server
     exposes the full playable loop.

3. **Phaser or full game framework**
   - Useful if client-owned gameplay were needed.
   - Poor fit for a server-authoritative renderer because it invites duplicate
     simulation logic.

## Selected Stack

- Workspace: `client/`
- Tooling: Vite plain TypeScript app
- Renderer: PixiJS v8
- HUD: vanilla TypeScript DOM layer
- Tests: Vitest
- Protocol: WebSocket JSON envelopes matching `internal/game/realtime`

Context7 documentation fetched:

- `/vitejs/vite`: Vite app structure, scripts, `index.html`, TypeScript client
  types
- `/pixijs/pixijs`: async `Application.init`, canvas append, ticker, resize
- `/vitest-dev/vitest`: `.test.ts` files and imported `test`/`expect`

## Architecture

```text
client/
  index.html
  package.json
  src/
    main.ts
    app/client-app.ts
    protocol/
    state/
    net/
    render/
    ui/
```

`main.ts` boots a `ClientApp`, mounts PixiJS into a canvas host, mounts the HUD
into the DOM, and wires input events. Domain modules stay pure enough to unit
test:

- `protocol/` owns envelope types, safe parsing, operation names, and request id
  generation.
- `state/` owns reducer logic for snapshots, AOI enter/update/leave, player
  status, safe errors, connection status, and an outbound command log.
- `net/` owns WebSocket connection state and message dispatch. It never trusts
  client-authored player id, damage, cooldown, XP, loot, price totals, or hidden
  visibility data.
- `render/` owns PixiJS drawing of a decorative starfield, local ship, visible
  NPCs, visible loot, and visible planet-signal/intel markers.
- `ui/` owns compact operational panels: player status, target, cargo, combat
  log, quest/inventory/planet-intel skeletons, errors, and connection controls.

## Data Flow

1. User clicks the world canvas.
2. Client transforms screen coordinates into a target point and sends
   `move_to` with `{ "target": { "x": number, "y": number } }`.
3. Client includes `request_id`, `client_seq`, and `v: 1`.
4. Server validates authenticated player/session and sends response/events.
5. Reducer applies only server response/event payloads.
6. Renderer draws from reducer state. Local prediction is limited to visual
   smoothing and never overwrites authoritative state.

## UI Direction

The first screen is the actual game client:

- Full-bleed PixiJS space scene.
- Compact console HUD with restrained, operational styling.
- Mobile-first layout with bottom sheets and touch targets at least 44px.
- Desktop layout with left status/nav, right target/cargo panels, and bottom log.
- No marketing hero, no explanatory feature text, and no hidden debug fields.

## Testing And Verification

Client tests:

- Protocol envelope parsing rejects invalid messages and hidden/debug leak keys.
- Request id generation creates non-empty unique ids.
- Reducer handles entity enter/update/leave and server corrections.
- Command builders omit trusted player id and gameplay truth fields.

Browser verification:

- Vite dev server opens locally.
- Canvas has nonblank pixels on desktop and mobile viewports.
- HUD text does not overlap on mobile.
- Hidden/debug strings such as `gameplay_seed`, `future_spawn`, and
  `internal_metadata` do not appear in client state rendering.

Backend verification remains required:

- `go test ./...`
- `git diff --check`
