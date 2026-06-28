# Polish Backlog

Date: 2026-06-28

## Goal

Turn the current server-authoritative vertical slice into a game that feels
closer to DarkOrbit without faking gameplay data.

## Current Implementation Pass Boundary

This pass owns only the first feel-changing vertical slice:

- server-owned player attack stance
- NPC return fire
- combat feedback
- dense starter-to-risk content slice
- real browser feel e2e proof

This pass does not own:

- ammo economy
- rockets
- drones
- P.E.T.
- full Signal Gate
- factions/companies
- broad endgame balance

## Live Playtest Note

The 2026-06-28 two-pilot local playtest closed a real stability blocker: the
authenticated gameplay WebSocket no longer drops after 30 idle seconds. That
does not close the product-feel gap. Human feedback after the fix was still
that the build lacks a convincing game feel, so the backlog below remains
focused on moment-to-moment combat readability, sector density, upgrade hunger,
and a less web-app-like HUD.

## P0: Combat Becomes A Loop

### P0.1 Server-Owned Attack Stance

Add server-owned combat engagement:

- active target
- start/stop attack
- selected weapon/ammo placeholder
- next-fire cadence
- stop reasons
- combat state snapshot

Acceptance:

- client sends one start attack intent, not repeated shot spam
- server fires basic laser on cooldown while valid
- movement can continue while attacking
- range/visibility/energy/cooldown/ship state can stop or pause attack
- reconnect snapshot restores attack/combat state safely

### P0.2 NPC Return Fire

Make at least one early hostile NPC actually attack the player:

- server-owned target acquisition
- range/cooldown validation
- shield/hull damage
- safe-zone/leash reset
- client-safe events

Acceptance:

- player can take visible shield damage from a real NPC
- NPC stops at safe-zone/leash conditions
- no hidden aggro internals leak

### P0.3 Combat Feedback Pass

Add:

- muzzle flash
- beam/projectile cadence
- shield impact
- hull impact
- target flash
- kill burst
- loot reveal

Acceptance:

- a combat screenshot shows who fired, who was hit, and what changed without
  relying on log text

## P1: First Sector Feels Populated

### P1.1 Dense Starter-To-Risk Map Pass

Author real server content for one connected path:

```text
1-1 onboarding
1-2 PvE farming
1-3 risky/PvP reward
```

Acceptance:

- starter map has multiple visible real contacts
- PvE map has several enemy bands
- risky map has better rewards
- no fake client objects

### P1.2 Gate Anchors

Make portals/gates feel like world anchors:

- stronger world visuals
- approach radius
- label
- minimap priority
- cooldown/protection readout

Acceptance:

- a player can visually understand where a gate is and why it matters

### P1.3 Radar Drama

Add client-safe radar states:

- unknown contact
- lost contact
- scan revealed
- hostile near
- jammed/interference if supported

Acceptance:

- hidden truth remains hidden
- radar creates curiosity/danger instead of only exact known dots

## P2: Early Upgrade Hunger

### P2.1 Early Ladder Contract

Write and implement a 3-5 hour balance table:

- Phoenix starter
- first laser/shield/cargo upgrades
- first ship choice
- first risky-map reason
- first gate-fragment goal

Acceptance:

- fresh player sees one clear next upgrade goal within 20 minutes

### P2.2 Content Breadth

Add minimum:

- 8-12 NPC profiles
- 4-6 loot tables
- 10-15 modules
- 6-10 recipes
- map-specific drops/resources

Acceptance:

- safe maps are slower
- risky maps are materially better
- drops create anticipation

### P2.3 Scarcity And Sinks

Tune:

- better gear not all unlimited direct shop stock
- ammo or consumable sink
- repair cost visibility
- market/auction fees where appropriate
- rare fragments/items

Acceptance:

- player has a reason to farm, craft, trade, or enter risk instead of simply
  buying everything directly

## P3: First Retention Engine

### P3.1 Mini Signal Gate

Implement a small Galaxy-Gate-like loop:

- collect fragments
- build/activate gate
- enter wave instance or bounded encounter
- defeat waves/boss
- claim rewards

Acceptance:

- one repeatable PvE goal exists beyond normal map farming
- reward is exciting but server-owned and idempotent

## P4: DarkOrbit Flavor Slices

### P4.1 Ammo

Add ammo types:

- selected ammo state
- server-owned consumption
- damage/cost modifiers
- quickslot selection

Acceptance:

- ammo changes combat cadence/economy immediately

### P4.2 Rocket / Burst Slot

Unlock a second active combat button:

- cooldown
- range
- ammo/stock
- visible impact

Acceptance:

- combat has at least one timing decision beyond laser cadence

### P4.3 Honor / Leaderboard

Add basic honor:

- PvE/PvP accrual rules
- abuse-safe source events
- weekly leaderboard query

Acceptance:

- risky play has social/rank value

### P4.4 Drone-Lite

Add one simple drone slot path:

- unlock/acquire
- equip module/stat path
- level or XP later

Acceptance:

- player sees visible long-term equipment chase

### P4.5 P.E.T.-Lite Later

Do not rush P.E.T. before abuse design:

- server-owned auto-loot only
- cooldown/range/fuel/upkeep
- no client bot behavior

## P5: HUD And Input Polish

### P5.1 Combat-First Topbar

Move non-combat utility out of prime combat space:

- logout
- sync/debug
- social/mail
- admin

Acceptance:

- topbar mostly communicates sector, danger, ship, cargo, credits, capacitor,
  and critical notifications

### P5.2 Stronger Target Panel

Add:

- lock state
- range state
- attack stance state
- target HP/shield deltas
- stop reason

Acceptance:

- selected target feels like combat center, not inspection panel

### P5.3 Mobile Tactical Layout

Build mobile as a tactical combat layout, not compressed desktop.

Acceptance:

- mobile first viewport keeps world/target/hotbar/minimap usable

## P6: Verification

### P6.1 10-Minute Feel Script

Create a scripted browser/server session:

```text
register -> spawn -> move -> lock -> auto attack -> take damage -> kill ->
loot -> upgrade progress -> portal/risky-map decision
```

Acceptance:

- screenshot/video artifact
- no fake data
- no hidden leaks
- written feel notes

### P6.2 Feel Regression Checklist

For every polish phase, answer:

- What did the player want next?
- What created danger?
- What created reward anticipation?
- What felt too much like a web app?
- What changed from the previous screenshot?

## Suggested Immediate Next Phase

Start with P0 + a narrow P1 slice:

```text
server-owned attack stance
NPC return fire
stronger combat feedback
one denser starter-to-risk path
```

Do not begin with broad UI beautification. It will make the same empty loop
prettier, not better.
