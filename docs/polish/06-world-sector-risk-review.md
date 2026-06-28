# World Sector And Risk Review

Date: 2026-06-28

## Verdict

The project has the right server-authoritative foundation for a DarkOrbit-like
universe, but the playable world currently feels too sparse, too safe, and too
infrastructure-first.

The backend knows about:

- bounded maps
- portals
- safe/PvP policy
- radar/AOI
- hidden visibility
- enemy pools
- scanner/planets
- routes/production

What is missing is sector ecology:

- many enemy groups
- meaningful gate lanes
- live PvP pressure
- auto-attacking NPCs
- contested rewards
- faction/social threat
- dense visible reasons to move

## What Is Strong

### Bounded Multi-Map Direction

`docs/map-rework/00-index.md` sets the correct direction: bounded DarkOrbit-like
maps, radar visibility, portals, safe/PvP areas, rare planet discovery, and
per-map enemy pools.

### Server-Owned Map Authority

`internal/game/world/maps` defines map bounds, public map keys, risk bands,
safe zones, portals, and server-only enemy content.

### Portal Authority

`internal/game/server/portal_handlers.go` validates active map, portal
visibility, range, cooldown, destination spawn, transfer, protection, and
snapshot/event publication server-side.

### Visibility Boundary

`internal/game/server/runtime_world_snapshot.go`,
`internal/game/world/aoi/snapshot.go`, and `internal/game/world/visibility`
filter hidden entities and internal map/spawn/seed data before the browser sees
anything.

### Client State Hygiene

`client/src/state/reducer-world.ts`, `client/src/state/reducer-map.ts`, and
`client/src/protocol/envelope.ts` treat map/AOI/protocol data as public
projections, not hidden truth.

## Why The World Feels Unlike DarkOrbit

### 1. The Universe Is Too Small

The current public path is effectively:

```text
1-1 safe -> 1-2 PvE -> 1-3 PvP
```

That is enough for a proof. It is not enough for a universe.

DarkOrbit feeling depends on recognizable sector identity:

- safe home maps
- escalating NPC bands
- risky PvP maps
- gate routes
- social/faction geography
- resource identity

### 2. Maps Are Too Empty

The e2e and map-rework evidence proves initial spawn, kill, respawn, and aggro
behavior, but the authored density is still too low.

If a large bounded map has one or two important contacts, the player reads it as
a test arena.

### 3. Early NPCs Are Too Harmless

Starter/passive enemies are useful for onboarding, but if the first maps mostly
contain passive targets, the player learns:

```text
hostile means target, not threat
```

That is the wrong emotional foundation.

### 4. Risk Is Metadata, Not Texture

The project has `risk_band`, `pvp_policy`, safe zones, protection, cargo-drop
rules, and death/repair. But danger is not yet a lived world texture.

Missing:

- ambush lanes
- more visible hostile clusters
- better rewards in risky maps
- rare boss/event spawns
- bounty/honor pressure
- contested loot/resource zones
- visible route/production vulnerability

### 5. Routes And Planets Are Secure But Not Dangerous

Planet claim, production, and routes are strong strategic hooks. But the live
world does not yet make them feel dangerous:

- no visible convoys
- no pirate pressure
- no route interception
- no clan contest
- no player conflict around production

Do not add all of that at once. But at least one visible risk/reward bridge is
needed.

### 6. Radar Replaced Fog, But Needs Drama

Radar/AOI is correct for server security. But emotional fog is weaker.

Add public presentation for:

- unknown blips
- lost contacts
- scan-revealed contacts
- jammer/interference zones
- hostile warning pulses
- radar classes rather than exact full certainty everywhere

Hidden truth must stay hidden. Public uncertainty can still be exciting.

## Recommendations

### 1. Build A Dense Starter-To-Risk Sector Pass

Before adding many new abstract systems, author a small but dense world:

- 3 starter-safe points of interest
- 2-3 passive/training enemies
- 2-4 mildly hostile enemies in the next map
- 1 risky map with materially better rewards
- 1 visible gate lane
- 1 rare signal/planet loop
- 1 event/boss or wave trigger

### 2. Increase Real NPC Density

Target early sector density:

- starter map: 6-12 live low-risk contacts
- PvE map: 12-25 live contacts across enemy bands
- PvP/risk map: 15-40 live contacts plus better drops

Use server-owned spawners. Do not fake client contacts.

### 3. Add NPC Auto-Combat

Aggro without damage is not enough. NPCs must attack with server-owned cadence.

### 4. Make Gates World Anchors

Gates should be visible, valuable, and navigationally obvious:

- stronger world art
- approach ring
- label
- minimap priority
- nearby traffic/enemy ecology
- cooldown/protection feedback

### 5. Turn One Risk Map Into A Real Test Bed

For `1-3` or a new equivalent:

- more NPCs
- better drops
- PvP cargo-loss clarity
- honor/bounty scaffold
- death/repair feedback
- safe-zone boundary readability

### 6. Add Radar Drama

Add public, safe, non-leaking UI states:

- `unknown_contact`
- `signal_lost`
- `scan_revealed`
- `jammed`
- `hostile_near`

## Bottom Line

The world architecture is enabling. The world content is not yet convincing.

The next sector pass should be authored like a playable place, not a proof
matrix.

