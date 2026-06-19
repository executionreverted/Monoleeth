# Browser UI Rework Brief

Date: 2026-06-19

## Problem Statement

The current browser UI is not acceptable as a game HUD. It reads like a stacked
debug/form surface instead of the playable space MORPG client targeted by
`output/mockups/final-mockup.png`.

Current player-facing issues:

- The screen is mostly vertical stacked content.
- There is no coherent modal/panel system.
- Panels do not feel like game windows that open, close, layer, and focus.
- Too much gameplay state is always visible in the center instead of being
  organized into HUD regions.
- The user falls into a scroll/form-heavy page instead of a playable cockpit.
- The visual result does not match the final mockup direction.
- The background/world surface lacks depth, parallax, and motion cues.
- Movement does not feel continuous because visual interpolation is weak or
  absent.
- Clicking entities does not produce enough selection, targeting, or feedback.
- Combat does not visibly react: target selection, HP bars, damage feedback, and
  firing effects are missing or too quiet.
- Loot/resource objects do not clearly communicate that they can be approached,
  collected, or what reward was gained.

## Design Target

The first screen after login must be the actual playable HUD, not a long page.

Primary visual target:

```text
output/mockups/final-mockup.png
```

The HUD should feel like:

- full-screen space game cockpit
- central world/canvas as the main surface
- edge-anchored panels and toolbars
- modal windows for secondary workflows
- dense but controlled sci-fi UI
- no page scroll as the default gameplay experience
- no visible fake gameplay values

## Required UI Architecture

### Layout Shell

- Replace stacked page layout with a fixed full-viewport game shell.
- Central world/canvas owns the primary space.
- HUD regions:
  - top status/nav strip
  - left target/nearby/intel stack
  - right inventory/quest/economy stack
  - bottom action rail
  - modal/window layer above the HUD
- The page body should not become the gameplay scroll container.

### Panel System

Add a real panel/window system:

- panels can open and close
- only relevant panels are visible by default
- secondary systems open as overlays/windows instead of inline page sections
- panels have titles, close controls, empty/loading/error states
- panel state is client UI state, while all gameplay values remain server-owned
- mobile uses drawers/tabs instead of dumping everything vertically

Core panels:

- Target
- Cargo/Inventory
- Quests
- Intel/Scanner
- Market/Auction/Premium
- Systems/Loadout/Crafting read models
- Admin/Ops only for admin users

### Modal System

Add reusable modal primitives for:

- details
- confirmations
- reward summaries
- error details
- admin-only actions

Rules:

- no nested cards inside cards
- modals are visually distinct from HUD panels
- escape/backdrop/close button behavior must be predictable
- modal content must be compact and scannable

## World Feel

### Background And Parallax

The world needs depth:

- layered starfield
- slow parallax response to camera/player movement
- subtle nebula/noise/sector texture
- far stars, mid dust, near particles
- motion should be visible even without UI text

### Movement Presentation

Movement should feel continuous:

- server remains authoritative
- client clicks are intents
- server movement state should represent origin, destination, speed, and timing
- client interpolates current position from server timestamps
- reclicking mid-route should use the server-calculated current position as the
  new movement origin
- visual ship movement should not appear as instant teleporting
- spam clicks should be rate-limited or coalesced without breaking server truth

## Interaction Feedback

### Entity Selection

Clicking an NPC, player, loot, or signal should:

- select the entity
- open/update the Target panel
- show name/type/distance/status when visible
- visually highlight the selected object
- never reveal hidden/fogged data

### Combat Feedback

Combat needs visible reactions:

- selected target HP/shield bar
- target bracket or reticle
- weapon firing effect
- hit/miss/damage log line
- damage flash or small floating number
- cooldown/energy feedback on action buttons
- target death/loot spawn feedback

### Loot Feedback

Loot/resource objects need a clear loop:

- loot object visible and identifiable when allowed by server visibility
- click or action should either move toward it, pickup if in range, or show why
  it cannot be picked up
- successful pickup should show what was gained
- cargo/inventory snapshot should update from server truth
- pickup errors should be explicit but compact

## Mockup Parity Requirements

Before this UI rework is considered acceptable:

- first viewport must resemble `output/mockups/final-mockup.png`
- no default gameplay page scroll on desktop
- major systems are accessible through HUD panels/windows
- central canvas/background carries the game visually
- typography, spacing, borders, and panel density are consistent
- buttons use icon/tool affordances where appropriate
- mobile has a usable drawer/tab layout, not a broken stacked desktop

## Suggested Implementation Slices

1. HUD shell and no-scroll layout.
2. Panel/window registry with open/close/focus state.
3. Move existing always-visible sections into panels.
4. Add background/parallax world rendering.
5. Add selected entity state and target panel behavior.
6. Add combat visual feedback and target HP/shield display.
7. Add loot interaction feedback and reward toast/log.
8. Add movement interpolation polish and click coalescing/rate-limit UX.
9. Match mockup spacing/color/panel styling.
10. Browser verification across desktop and mobile.

## Acceptance Checklist

- [ ] Logging in lands in a fixed full-screen HUD.
- [ ] No form-scroll hell in the default game screen.
- [ ] At least one real modal can open/close predictably.
- [ ] Panels are opened intentionally, not all dumped on screen.
- [ ] Background has visible layered parallax.
- [ ] Movement visually interpolates instead of teleporting.
- [ ] Clicking a target selects it and updates a target panel.
- [ ] Combat visibly changes target state.
- [ ] Loot pickup shows gain/error feedback.
- [ ] Desktop screenshot is close to `final-mockup.png`.
- [ ] Mobile has drawers/tabs and no incoherent overlap.
- [ ] `go test ./...`, client check, browser smoke, and `git diff --check`
  pass before handoff.

