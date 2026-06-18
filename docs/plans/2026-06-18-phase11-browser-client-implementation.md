# Phase 11 Browser Client Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the first browser client prototype that renders server-approved world state, sends Phase 04 intent commands, and verifies protocol/state safety.

**Architecture:** Add a new `client/` Vite TypeScript workspace. Keep protocol parsing, request building, state reduction, WebSocket transport, PixiJS rendering, and DOM HUD code in separate modules so server-authoritative boundaries are easy to test.

**Tech Stack:** Vite, TypeScript, PixiJS v8, Vitest, WebSocket JSON envelopes.

---

### Task 1: Client Workspace Scaffold

**Files:**
- Create: `client/package.json`
- Create: `client/package-lock.json`
- Create: `client/index.html`
- Create: `client/tsconfig.json`
- Create: `client/vite.config.ts`
- Create: `client/src/main.ts`
- Create: `client/src/styles.css`

**Steps:**

1. Create `client/package.json` with scripts:
   - `dev`: `vite --host 127.0.0.1`
   - `build`: `tsc --noEmit && vite build`
   - `typecheck`: `tsc --noEmit`
   - `test`: `vitest run`
   - `test:watch`: `vitest`
2. Add dependencies:
   - `@vitejs/plugin-basic-ssl` is not needed.
   - Production: `pixi.js`
   - Development: `@vitejs/plugin-legacy` is not needed, `typescript`, `vite`,
     `vitest`, `playwright` if browser smoke tests are implemented locally.
3. Create `index.html` with `<div id="app"></div>` and
   `<script type="module" src="/src/main.ts"></script>`.
4. Create strict TypeScript config with `types: ["vite/client"]`.
5. Add a minimal `main.ts` that boots a placeholder and imports `styles.css`.
6. Run `npm install` in `client/`.
7. Run `npm run typecheck`.
8. Commit as `client: scaffold phase11 workspace`.

### Task 2: Protocol And Request Safety

**Files:**
- Create: `client/src/protocol/envelope.ts`
- Create: `client/src/protocol/request-id.ts`
- Create: `client/src/protocol/commands.ts`
- Create: `client/src/protocol/envelope.test.ts`
- Create: `client/src/protocol/request-id.test.ts`

**Steps:**

1. Define protocol version `1`, operation names, client request envelope, server
   response envelope, error envelope, and event envelope types matching
   `internal/game/realtime/envelope.go`.
2. Add safe parsing helpers that reject non-object payloads and payloads
   containing hidden/debug leak keys:
   - `gameplay_seed`
   - `future_spawn`
   - `internal_metadata`
   - `hidden`
3. Add `createRequestId()` using `crypto.randomUUID()` with a timestamp/random
   fallback for browsers that do not expose it.
4. Add command builders for `move_to`, `stop`, and `debug_snapshot`. Builders
   must include `request_id`, `client_seq`, `v`, and operation payload, and must
   never include `player_id`, `damage`, `xp`, `loot`, `cooldown`, or
   `wallet_amount`.
5. Write Vitest tests for valid parsing, invalid parsing, request id uniqueness,
   and forbidden payload fields.
6. Run `npm test -- --runInBand` or `npm test` if Vitest does not support that
   flag in the installed version.
7. Commit as `client: add realtime protocol safety`.

### Task 3: Client State Reducer

**Files:**
- Create: `client/src/state/types.ts`
- Create: `client/src/state/reducer.ts`
- Create: `client/src/state/reducer.test.ts`

**Steps:**

1. Define client state:
   - connection status
   - sequence counters
   - player snapshot
   - visible entity map
   - selected target id
   - combat/log lines
   - cargo summary skeleton
   - quest/inventory/planet-intel skeleton state
   - last safe error
2. Implement reducer actions:
   - `connectionChanged`
   - `requestQueued`
   - `responseReceived`
   - `eventReceived`
   - `serverCorrection`
   - `selectTarget`
3. Handle event types:
   - `player.snapshot`
   - `aoi.entity_entered`
   - `aoi.entity_left`
   - `position.corrected`
4. Treat unknown events as log lines, not mutations.
5. Tests verify enter/update/leave, correction wins over local visual state, and
   hidden/debug keys do not enter state.
6. Run `npm test`.
7. Commit as `client: add authoritative state reducer`.

### Task 4: WebSocket Client And Demo Harness

**Files:**
- Create: `client/src/net/realtime-client.ts`
- Create: `client/src/app/demo-state.ts`
- Modify: `client/src/main.ts`

**Steps:**

1. Implement `RealtimeClient` with:
   - configurable URL
   - `connect`, `disconnect`, `send`
   - JSON message parsing through protocol helpers
   - connection status callbacks
2. Add an offline demo harness that seeds a local player, NPCs, loot, and planet
   signal entities through reducer actions when no WebSocket is connected.
3. Keep the demo harness explicit and non-authoritative: it exists only so the
   browser prototype is visually testable before a gateway exists.
4. Run `npm test` and `npm run typecheck`.
5. Commit as `client: add websocket transport harness`.

### Task 5: PixiJS Renderer

**Files:**
- Create: `client/src/render/world-renderer.ts`
- Create: `client/src/render/world-view.ts`
- Modify: `client/src/main.ts`
- Modify: `client/src/styles.css`

**Steps:**

1. Initialize PixiJS v8 with `new Application()` and `await app.init(...)`
   before using `app.canvas`.
2. Mount the canvas in a full-bleed game viewport.
3. Render:
   - decorative starfield
   - local player ship
   - visible NPCs
   - visible loot
   - visible planet signal/intel marker
   - movement target/correction markers
4. Use PixiJS ticker for subtle motion and remote interpolation.
5. Add click-to-world coordinate transform. Click sends `move_to`; a stop
   control sends `stop`.
6. Do not draw hidden entities or future spawn candidates.
7. Run `npm run typecheck`.
8. Commit as `client: render phase11 world view`.

### Task 6: Responsive DOM HUD

**Files:**
- Create: `client/src/ui/hud.ts`
- Create: `client/src/ui/toast.ts`
- Modify: `client/src/styles.css`
- Modify: `client/src/main.ts`

**Steps:**

1. Build the first screen as the playable view: canvas plus HUD.
2. Add compact panels:
   - player status
   - HP/shield/energy
   - cargo skeleton
   - target panel
   - combat log
   - quest board skeleton
   - inventory/loadout skeleton
   - planet intel/map memory skeleton
   - safe error toast
3. Use mobile-first CSS:
   - bottom sheet layout on small screens
   - side rails on desktop
   - minimum 44px interactive targets
   - stable panel dimensions
   - no overlapping text
4. Keep controls operational, not explanatory. Use clear short labels and
   tooltips where needed.
5. Run `npm run typecheck`.
6. Commit as `client: add responsive game hud`.

### Task 7: Browser Smoke Verification

**Files:**
- Create: `client/tests/browser-smoke.mjs`
- Modify: `client/package.json`
- Modify: `client/README.md`

**Steps:**

1. Add a smoke script that uses Playwright to:
   - open local Vite URL
   - verify canvas exists
   - check sampled canvas pixels are nonblank
   - capture desktop and mobile screenshots under `client/output/`
   - assert hidden/debug strings do not appear in visible text
2. Add `smoke`: `node tests/browser-smoke.mjs`.
3. Document local commands in `client/README.md`.
4. Start Vite with `npm run dev -- --port <free-port>`.
5. Run `npm run smoke -- --url http://127.0.0.1:<port>`.
6. Stop the dev server.
7. Commit as `client: add browser smoke verification`.

### Task 8: Roadmap, Review, And Final Verification

**Files:**
- Modify: `docs/roadmap/11-browser-client-prototype.md`
- Modify: `docs/todo.md` if any Phase 11 gaps remain.

**Steps:**

1. Check only completed Phase 11 items.
2. Add follow-ups for unsupported server ops, missing gateway, or partial
   playable-loop gaps.
3. Run:
   - `npm test` in `client/`
   - `npm run build` in `client/`
   - browser smoke script
   - `go test ./...` at repo root
   - `git diff --check`
4. Dispatch a Symphony review worker for Phase 11.
5. Fix review findings or log follow-ups in `docs/todo.md`.
6. Inspect staged diff.
7. Commit as `client: complete phase11 prototype`.
