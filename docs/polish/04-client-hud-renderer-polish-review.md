# Client HUD And Renderer Polish Review

Date: 2026-06-28

## Verdict

The browser client has the right bones for a DarkOrbit-like client:

- full-screen Pixi world
- HUD overlay
- click-to-move
- target selection
- hotbar keys
- minimap contacts
- real combat/loot/scan/planet paths
- no default fake gameplay state

But the current feel is closer to:

```text
server-backed debug cockpit with sci-fi styling
```

than:

```text
visceral 2D space combat MMO HUD
```

## What Works

### Full-Bleed World Layer

`client/src/app/client-app.ts` mounts the world and HUD as layered surfaces:

```text
.world-host
.hud-host
```

That is the right architecture for a browser-first 2D space MMO.

### Pixi Layer Stack

`client/src/render/world-renderer.ts` sets up:

- starfield layer
- nebula/grid
- map overlay
- scan layer
- world entities
- memory markers
- marker/effect layer

This is the correct rendering shape.

### Real Input Model

`client/src/render/world-renderer-entities.ts` supports:

- canvas click to select visible entity
- canvas click to select remembered marker
- canvas click to send movement intent
- HUD input suppression so UI clicks do not leak into world movement

### Hotbar Direction

`client/src/ui/hud-render-panels.ts` defines six quick action slots:

- laser
- rocket locked
- scan
- stealth
- warp locked
- gather

`client/src/ui/hud.ts` supports keyboard handling for `1-6` and target cycling.

This is a good base.

### Real-State Discipline

The client is strict about no fake default gameplay values:

- `client/src/state/reducer.ts`
- `client/src/state/reducer-auth.test.ts`
- `client/src/state/reducer-helpers.ts`

This is one of the best parts of the project.

## What Undermines The Feel

### 1. The Mockup Is Dense; The Live Client Is Sparse

The visual target in `output/mockups/final-mockup.png` has:

- dense right rail
- selected object detail
- planet list
- minimap
- bottom action bar
- bottom-left log
- multiple world interests
- central ship/readable range
- hostile/loot/unknown/planet signals

The live screenshots and e2e playtest prove the world works, but the composition
often looks sparse. A live starter map can show too few meaningful things in a
large grid.

This is partly content density, but the client also makes primary objects too
subtle.

### 2. Sprites Are Too Small And Faint

`client/src/render/world-renderer-sprites.ts` uses low alpha and small scales for
key entity sprites.

Impact:

- player ship does not dominate the center
- hostile silhouettes are not scary
- loot does not feel valuable
- world entities read as HUD glyphs rather than physical objects

Recommendation:

- Increase player, hostile, loot, and portal visual hierarchy.
- Reduce global tint wash where it flattens sprites.
- Use engine glow/trails to make the player ship feel alive.

### 3. Feedback Is Too Thin

`client/src/render/world-renderer-effects.ts` contains good proof-of-effect
logic, but many effects are still vector-line / floating-text first.

Needed:

- source muzzle flash
- shield impact ring
- hull impact spark
- target hit flash
- reticle pulse
- kill explosion/debris
- loot sparkle/reveal
- pickup beam
- scan pulse drama

The effect should answer "what happened?" without the log.

### 4. HUD Utilities Occupy Combat Space

`client/src/ui/hud-render-shell.ts` puts these controls in the top toolbar:

- Stop
- Sync
- Mail
- Chat
- Social
- Logout

Stop is combat-relevant. Sync may be useful for debug. Mail/Social/Logout should
not dominate prime combat space.

Recommendation:

- Keep topbar for sector, danger, energy/capacitor, cargo, credits, and compact
  critical indicators.
- Move account/admin/system utilities to a secondary menu/drawer.

### 5. UI Focus Interrupts Combat

The input authority layer is safe, but the result can feel like windows/forms
own the game:

- `client/src/input/world-input-authority.ts`
- `client/src/ui/hud.ts`

Recommendation:

- Keep text inputs safe.
- Allow combat keys (`1-6`, Tab, stop/escape) while non-text HUD panels have
  focus.
- Avoid opening large windows during core combat unless the player explicitly
  chooses management mode.

### 6. Honest Empty States Feel Dead

Examples:

- `AWAITING SERVER SNAPSHOT`
- `--`
- `Select a contact.`
- `Awaiting systems data.`

These are honest and preferable to fake data. But they look like web-app empty
states.

Recommendation:

- Reframe empty states as cockpit modules:
  - locked readout
  - signal unavailable
  - no contact
  - link pending
  - scanner quiet
- Keep copy short and visual.

### 7. Mobile Carries Too Much Desktop

The responsive CSS preserves many desktop panels. On a phone-sized viewport,
the world can become crowded by topbar, rails, right panels, and actionbar.

Evidence:

- `client/src/styles/responsive.css`

Recommendation:

- Treat mobile as tactical mode, not compressed desktop.
- Default to:
  - world
  - compact ship state
  - target
  - hotbar
  - minimap/radar
- Drawerize inventory/shop/social/admin/log.

## Practical Client Polish Plan

### P0: Visual Hierarchy

- Increase player/NPC/loot sprite presence.
- Make selected target visually dominant.
- Make hostile and loot colors read instantly.
- Add contextual range rings.

### P1: Combat Feedback

- Add muzzle flash.
- Add shield/hull hit distinction.
- Add target HP delta animation.
- Add kill/loot punch.
- Add reticle pulse.

### P2: HUD Composition

- Move utility/account controls out of top combat bar.
- Bring right rail closer to mockup density.
- Keep target, planets, minimap, and actionbar visible without window clutter.

### P3: Input Flow

- Keep combat shortcuts live unless a text field/modal truly owns input.
- Add explicit attack stance/auto-fire affordance once server contract exists.
- Avoid large management windows interrupting combat.

### P4: Responsive Tactical Mode

- Build a separate mobile combat layout.
- Hide non-combat management by default.

### P5: Visual Regression

Existing e2e tests prove nonblank pixels and leak safety. Add visual checks for:

- central ship size/presence
- target panel present
- actionbar present and not overflowing
- minimap visible
- no text overlap
- primary world contacts visible
- desktop/mobile composition close to intent

## Bottom Line

The client is honest and technically grounded. That is good.

Now it needs to become less explanatory and more embodied. The player should
feel the ship, the target, the danger, and the reward before they read the log.

