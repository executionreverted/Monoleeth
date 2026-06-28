# DarkOrbit Feel Acceptance Criteria

Date: 2026-06-28

## Purpose

This document defines what "closer to DarkOrbit" means for this project in
observable product terms. It is not a demand to clone every DarkOrbit feature.
It is a guard against declaring completion when the game only proves backend
correctness.

Use these criteria before marking a polish phase complete.

## First 20 Minutes Goal

A fresh real account should produce this experience without fake gameplay data:

```text
login -> spawn -> understand ship status -> see multiple real contacts ->
move toward a target -> lock target -> enter attack stance -> fight while
moving -> take or narrowly avoid damage -> kill -> loot -> see progress toward
an upgrade -> make a meaningful next-map or next-upgrade decision
```

The player should not need to know the implementation roadmap to feel the loop.

## Session Acceptance Criteria

### 1. Sector Density

For desktop `1440x900`, a live authenticated starter or early-risk map should
show, within the first minute:

- the player ship
- at least 3 visible non-player world interests
- at least 2 attackable or scannable contacts
- at least 1 clear destination or portal anchor
- at least 1 loot/resource/unknown-signal reason to move
- minimap contacts that match the world state

No client-side fake contacts are allowed.

### 2. Combat Engagement

The player should be able to:

- select/lock a visible target
- start an attack stance once
- see server-owned cadence continue while valid
- move while the attack stance is active
- see range/visibility/cooldown/energy break the stance when invalid
- stop attack intentionally
- receive server-owned damage/miss/cooldown/target events

Required contract direction:

```text
combat.set_target
combat.start_attack
combat.stop_attack
combat.attack_started
combat.attack_stopped
combat.shot_started
combat.shot_resolved
combat.state_snapshot
```

Names may change, but the semantics must exist.

### 3. NPC Threat

At least one early-risk NPC type should:

- acquire a valid player target
- chase or maintain range
- fire using server-owned cooldown/range/visibility/energy rules
- visibly damage shield/hull or force evasive action
- reset at safe zones/leash boundaries
- emit client-safe combat events without leaking aggro internals

Passive starter targets may remain, but they cannot be the only early combat
experience.

### 4. Movement Feel

Server truth remains authoritative, but the renderer should make movement feel
less like a coordinate form:

- destination marker is obvious
- engine trail shows current movement
- camera uses render-only deadzone/lead/easing
- selected target and movement target can be read simultaneously
- weapon range and pickup range are visible when relevant
- long travel does not feel like client-side route bookkeeping

### 5. Feedback Punch

Combat and loot should be readable without watching the log:

- shot start or muzzle flash
- projectile/beam travel or channel
- shield impact distinct from hull impact
- target HP/shield delta animation
- miss feedback
- kill burst
- loot reveal
- pickup confirmation

The source of truth is still server events. The client may animate accepted
pending states, but damage/resource mutation must reconcile to server truth.

### 6. Early Upgrade Desire

Within 20 minutes, a fresh player should understand at least one concrete
upgrade goal:

- next laser / shield / generator / scanner / cargo upgrade
- next ship choice
- ammo or rocket supply pressure
- gate fragment or signal progress
- risky-map loot reason

The UI should show progress toward that goal using real server state.

### 7. Risk/Reward Clarity

The player should understand why a dangerous area is worth entering:

- better loot or XP
- unique drops
- honor/bounty/ranking value
- gate fragments
- rare signals/planets
- route or production upside

Danger labels alone are not enough. The reward must be concrete.

### 8. HUD Priority

The first viewport should prioritize:

- ship state
- sector danger
- target state
- world/minimap
- action bar
- combat/loot feedback

Account, logout, sync, admin, and long-form management windows must not dominate
prime combat space.

### 9. Mobile Tactical Mode

Mobile does not need to show every desktop panel. A mobile-authenticated combat
view should prioritize:

- world
- target
- hotbar
- minimap/radar
- compact ship status

Inventory, shop, social, admin, production, and long logs can be drawerized.

### 10. Playtest Evidence

Do not mark a feel pass complete without:

- desktop screenshot
- mobile screenshot
- at least one short scripted 10-minute real-server session
- no fake/default data leak scan
- notes answering:
  - What did the player want next?
  - What created danger?
  - What created reward anticipation?
  - What felt like a form or debug panel?
  - What felt like a game?

## Status Labels

Use these labels instead of a single "done":

- `contract complete`: protocol/server/client wiring exists
- `vertical slice complete`: one real path works
- `feel incomplete`: playable, but not yet emotionally convincing
- `production incomplete`: durability/scale/ops still not ready
- `polish complete`: meets this document's experience criteria

## Current Evidence

2026-06-28 `npm --cache /tmp/gameproject-npm-cache --prefix client run
e2e:darkorbit-feel` is a partial feel gate, not a polish-complete claim. It
proves a real DB-seeded browser account can enter attack stance, emit minimized
`combat.start_attack`, receive server-driven shot cadence while moving, kill a
default-data Origin NPC, receive and pick up server-created loot into cargo,
travel from `1-1` through `1-2` into `1-3`, acquire a fresh current-map `1-3`
NPC, take NPC return-fire damage, capture desktop and mobile screenshots, and
pass smoke/WebSocket/log leak scans.

The proof review lives in
`docs/polish/11-darkorbit-feel-browser-proof-review.md`. Still missing for
this document: a default scripted 10-minute session note. The e2e can run that
loop with `DARKORBIT_FEEL_LONG_RUN_MS=600000`, but the fast gate does not enable
it by default.
