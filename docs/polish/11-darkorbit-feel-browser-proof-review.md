# DarkOrbit Feel Browser Proof Review

Date: 2026-06-28

## Evidence Reviewed

- Desktop screenshot:
  `output/screenshots/ui-implementation/darkorbit-feel/darkorbit-feel-desktop-17826644061249bb55903099bc8.png`
- Mobile screenshot:
  `output/screenshots/ui-implementation/darkorbit-feel/darkorbit-feel-mobile-17826644061249bb55903099bc8.png`
- Run notes:
  `output/screenshots/ui-implementation/darkorbit-feel/darkorbit-feel-notes-17826644061249bb55903099bc8.json`

## What Is Now Proven

- A real authenticated account reaches the browser cockpit with server state.
- The client starts a server-owned attack stance with `combat.start_attack`.
- Server shot cadence continues while the player moves.
- A default-data Origin NPC can be killed and server-created loot can be picked
  up into cargo.
- The player travels from `1-1` through `1-2` into `1-3`.
- A fresh `1-3` NPC can engage and return fire, producing real player damage.
- Desktop and mobile screenshots are captured from the same real run.
- The browser proof scans command frames and process logs for hidden/fake data
  token leaks.

## What Felt Better

- The screen now contains a real hostile, real player damage, real cargo/wallet
  values, and real sector danger.
- The early path no longer feels like an empty backend smoke test; `1-3` has
  actual risk.
- The action bar and damage popups give the player immediate combat context
  without reading backend logs.

## What Still Hurt The DarkOrbit Feel

- Live human playtest feedback after the transport fixes was still blunt:
  stability improved, movement/chat worked, but the build still did not feel
  like a game yet.
- The HUD still reads more like a dense web cockpit than a combat-first space
  MMO interface.
- Prime mobile space is crowded by management panels; a true mobile tactical
  mode should drawerize inventory/shop/quests/planets/social.
- The default browser gate is a short proof. A 10-minute observation loop is
  available through `DARKORBIT_FEEL_LONG_RUN_MS=600000`, but it is not part of
  the normal fast gate.
- Rocket/ammo/drone/P.E.T.-style desire systems remain future flavor slices.

## Follow-Up Fixed From This Review

- During active combat, the Target panel could still show "Select a contact" if
  manual selection was cleared while the server engagement remained active.
  The HUD now falls back to the server-owned active engagement target when that
  target is visible, so combat lock and the Stop action remain readable.
- The topbar duplicated Stop, Sync, Chat, Social, Mail, and Logout controls in
  prime combat space even though the game menu already owns those panel
  entries. The topbar is now a compact one-row status strip only.
- The browser proof exposed that WebSocket policy close `1008` was too broadly
  treated as auth expiry on the client. Only the server's explicit
  `session invalid` close reason now clears auth; slow-client policy closes can
  reconnect instead of wiping authenticated gameplay state.
- The live two-pilot playtest exposed a separate 30-second idle WebSocket drop:
  persistent gameplay sockets were using the same bounded timeout shape as
  individual reads. Commit `3011c783` makes the default read side wait
  indefinitely while preserving bounded writes and slow-client queue drops.
  Manual canaries proved 45 seconds idle plus chat and a longer idle run that
  stayed connected through minute 6 before the tester intentionally stopped it.
