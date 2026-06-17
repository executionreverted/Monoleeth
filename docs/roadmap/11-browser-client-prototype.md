# Phase 11: Browser Client Prototype

## Status

- State: Not started
- Owner: Browser-first client
- Depends on: Phase 04 for debug client, Phase 05 for first playable client loop
- Unlocks: user-facing playtesting and UI iteration

## Goal

Build a browser-first client that renders the authoritative world, sends intent commands, reconciles snapshots, and exposes the first playable loop without trusting client-side gameplay values.

## Source Specs

Read before implementation:

- `docs/2026-06-16-space-morpg-architecture-notes.md`
- `docs/plans/modules/15-api-events-errors.md`
- `docs/plans/modules/14-world-aoi-fog-security.md`
- `docs/plans/modules/16-testing-observability-balancing.md`

If using PixiJS, React, Vite, Tauri, WebSocket libraries, or other current libraries, fetch current docs through Context7 before implementation.

## Client Principle

The client is a renderer and input surface.

It may:

- render starfield and decorative visual seeds
- render server-approved visible entities
- render fog memory and known intel
- predict local movement visually
- interpolate remote entities
- send intent commands

It must not:

- decide damage
- decide hit/miss
- decide loot
- decide XP
- decide cooldown
- decide position truth
- receive hidden entities
- receive gameplay procedural seeds

## Recommended Tech Direction

Likely stack:

- TypeScript
- PixiJS for 2D world rendering
- React or similar DOM layer for panels
- WebSocket JSON during MVP
- Tauri later for desktop packaging

Do not start with a marketing landing page. The first screen should be the usable game client.

## TODO: Project Setup

- [ ] Decide client workspace location.
- [ ] Fetch current docs for selected frontend libraries with Context7.
- [ ] Create minimal TypeScript app.
- [ ] Add PixiJS canvas.
- [ ] Add DOM panel layer.
- [ ] Add WebSocket client.
- [ ] Add protocol envelope types matching server.
- [ ] Add local development scripts.
- [ ] Add lint/typecheck/test scripts.

## TODO: Rendering

- [ ] Render decorative starfield.
- [ ] Render local player ship.
- [ ] Render visible NPCs.
- [ ] Render visible loot.
- [ ] Render visible planet signals or known intel markers.
- [ ] Render AOI enter/leave changes.
- [ ] Render server position corrections.
- [ ] Interpolate remote entity movement.
- [ ] Show local prediction only as visual smoothing.

## TODO: Input And Commands

- [ ] Send `move_to` intent on click.
- [ ] Send `stop` intent.
- [ ] Send `combat.set_target` or attack intent.
- [ ] Send `combat.use_skill` for basic skill.
- [ ] Send `loot.pickup`.
- [ ] Send scanner activation when available.
- [ ] Include request ID for every command.
- [ ] Do not include player ID as trusted payload.
- [ ] Do not include client-calculated damage, XP, cooldown, or loot.

## TODO: UI Panels

- [ ] Player status panel.
- [ ] HP/shield/energy display from server snapshot.
- [ ] Cargo panel.
- [ ] Target panel.
- [ ] Combat log.
- [ ] Quest board panel skeleton.
- [ ] Inventory/loadout panel skeleton.
- [ ] Planet intel/map memory panel skeleton.
- [ ] Error toast using safe server error codes.

## TODO: Reconciliation

- [ ] Handle request response.
- [ ] Handle server rejection and correction.
- [ ] Handle periodic player snapshot.
- [ ] Handle entity entered/updated/left.
- [ ] Handle wallet/cargo/stat snapshot updates.
- [ ] Handle reconnect snapshot request.
- [ ] Treat server state as final.

## Tests And Verification

- [ ] Typecheck passes.
- [ ] Unit tests for protocol envelope parsing.
- [ ] Unit tests for request ID generation.
- [ ] Unit tests for state reducer handling entity enter/update/leave.
- [ ] Browser smoke test connects to local server.
- [ ] Browser screenshot verifies canvas is nonblank.
- [ ] Browser test verifies no hidden debug data appears in client state.
- [ ] Mobile viewport does not overlap HUD text.
- [ ] Desktop viewport keeps canvas and panels readable.

## Design Checks

- [ ] The first screen is the actual playable view.
- [ ] Controls are icon/tool appropriate, not explanatory text blocks.
- [ ] Text does not overlap or overflow buttons/panels.
- [ ] UI is compact and operational, not a landing-page hero.
- [ ] Visual assets are used for the game world.
- [ ] Debug labels can be toggled off.

## Asset Prep Notes

- 2026-06-17: Background exploration kept under
  `output/assets/mockup-hud/background/`. The rejected raster HUD/game sprite
  pack was removed; Phase 11 client implementation remains not started.
- 2026-06-17: Mockup-aligned HUD SVG assets generated under
  `output/assets/hud-svg/` for later browser client integration.

## Done Criteria

- [ ] Client can connect to local server.
- [ ] Player can move through server intent.
- [ ] Player can fight and loot through server intent.
- [ ] Client reconciles server snapshots.
- [ ] Client never receives hidden gameplay data.
- [ ] Tests and browser smoke check pass.
- [ ] `go test ./...` still passes for backend.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, open the local client and verify it connects to the current server contracts before adding UI. Protocol drift is the most likely blocker.
