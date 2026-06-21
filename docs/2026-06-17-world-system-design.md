# World System Design

Date: 2026-06-17

## Purpose

This document defines how the game world works: what the map is, what a zone is,
how bounded map-local space is generated, how planets are discovered, what gets
stored in the database, what stays procedural, and how players turn unknown
space into a galaxy network.

## 2026-06-21 Bounded Map Supersession

The playable scanner/planet model is now bounded-map first. For scanner planet
discovery, legacy infinite-plane, discovery-horizon, distance-from-origin level
scaling, and fog-wave expansion language below is superseded by:

- active maps are exact local `0..10000` coordinate spaces
- scanner candidates are generated only inside the active server-owned map
- candidate identity includes hidden map id and profile version
- candidate level comes from the map profile level band
- client-safe signal labels are map-local, not origin-distance labels
- known planet intel is memory, not live fog-wave visibility

The core fantasy:

```text
Start in a bounded map.
Push into unexplored local space.
Discover planets.
Build a network.
Trade information.
Travel through server-owned maps.
```

## Core Decisions

- The universe is persistent.
- The current playable world is composed of bounded `0..10000` maps.
- The base universe is generated from server-only procedural seeds.
- The map is not fully stored in the database.
- Permanent things become database records only after discovery, claim, ownership, or player modification.
- Known planet intel is personal by default.
- Live visibility is current-map membership plus radar/stealth rules.
- Planets are global shared objects, not personal instances.
- Planet intel is personal unless explicitly shared.
- Live ops content is layered on top of the persistent universe.

## Vocabulary

### Universe

The persistent shard containing all bounded maps.

```text
world = persistent shard
map = bounded local coordinate space
```

Players move between maps through server-validated traversal such as portals,
wormholes, docking, or event transport.

### Map

The player's current playable coordinate space plus server-approved memory.

"Map" means an exact `0..10000` local coordinate space containing visible
entities, discovered planets, known routes, radar contacts, wormholes, unknown
signals, and player-owned network nodes.

The browser may render remembered intel for other maps, but live scanning and
interactions are scoped to the authenticated player's active server-owned map.

### Region

A map profile or catalog grouping used for biome, risk, density, and progression
feel.

Examples:

```text
Origin Belt
Outer Drift
Deep Space
Void Frontier
Nebula Scar
Dead Zone
```

Regions are catalog/profile data for bounded maps. A region influences what can
appear there without relying on distance from origin.

### Zone

A server-side active simulation area.

Players do not need to know about zones. A zone is an implementation detail for map workers and world routing.

The zone owns active simulation:

- Players currently inside the area.
- NPCs currently active.
- Projectiles and combat.
- Loot and gather entities.
- Local visibility checks.
- Local procedural cells near players.
- Persistent overlay objects loaded for that area.

### Chunk

A procedural generation and persistence lookup unit.

Chunks are larger than radar range. They are used to batch-generate or cache area data.

Example:

```text
chunk_size = 5,000 or 10,000 world units
chunk_x = floor(x / chunk_size)
chunk_y = floor(y / chunk_size)
```

Chunks are useful for:

- Biome sampling.
- Candidate POI generation.
- Redis cache keys.
- DB overlay queries.
- World worker batching.

### Scan Cell

A smaller area used for scan pulse and radar checks.

Scan cells should feel like a few seconds of early-game travel distance, not a whole zone.

Example:

```text
scan_cell_size = 300 to 800 world units
```

The exact number should be tuned with movement speed, radar radius, and scan cadence.

### Point of Interest

Anything meaningful in space:

- Planet.
- Asteroid field.
- Resource bloom.
- Loot cache.
- Wreck.
- Wormhole signature.
- NPC nest.
- Anomaly.
- Relay.
- Player structure.

## Layered World Model

The world is built from four layers.

```text
1. Static Procedural Layer
2. Epoch / Live Ops Procedural Layer
3. Redis Materialization Cache
4. Persistent DB Overlay
```

### 1. Static Procedural Layer

This layer is generated from a server-only static world seed.

It is stable over time.

It controls:

- Permanent planet candidates.
- Permanent rare anomalies.
- Base biome field.
- Large resource tendencies.
- Long-term POI distribution.

The static seed must never be sent to the client.

If the client can calculate planet candidates, the entire fog-of-war and coordinate economy collapses.

### 2. Epoch / Live Ops Procedural Layer

This layer is generated from an epoch seed.

Epochs can be daily, weekly, or event-based.

It controls temporary content:

- Loot waves.
- Wreck fields.
- Meteor showers.
- NPC surges.
- Resource blooms.
- Temporary anomalies.
- Event signatures.
- Pirate patrols.

The epoch layer gives the universe a living feel without changing permanent planet coordinates.

Planets should not move or disappear because the daily seed changed.

### 3. Redis Materialization Cache

Redis stores already-computed procedural results for speed.

Redis is not the source of truth.

If Redis is cleared, the server can recompute the same procedural results from seed.

Typical Redis keys:

```text
world:cell:{epoch}:{cell_x}:{cell_y}
world:chunk:{epoch}:{chunk_x}:{chunk_y}
loot:cooldown:{object_id}
scan:recent:{player_id}:{cell_id}
```

TTL:

- Static cache can live longer.
- Epoch cache should expire at epoch end.
- Loot cooldowns should expire when respawn/despawn rules say so.

### 4. Persistent DB Overlay

The database stores things that changed because players interacted with the world.

Examples:

- Discovered planets.
- Planet ownership.
- Colony state.
- Buildings.
- Production.
- Player/alliance planet intel.
- Wormhole links.
- Radar relays.
- Marketed intel items.
- Important loot claims.
- Economy-impact item grants.
- Player routes and automation rules.

The DB overlay is applied on top of the procedural base.

```text
effective_world = static_seed_world + epoch_world + persistent_overlay
```

## Seed and Noise Strategy

### Static World Seed

Used for permanent world structure.

```text
static_seed = private server secret
```

Uses:

- Biome noise.
- Permanent POI candidates.
- Planet candidate positions.
- Planet base traits.
- Permanent anomaly candidates.

### Epoch Seed

Used for temporary content.

```text
epoch_seed = hash(static_seed, epoch_id)
```

Uses:

- Daily/weekly events.
- Temporary loot.
- NPC patrol density.
- Resource blooms.
- Wrecks.
- Event anomalies.

### Visual Seed

Used by the client for decoration only.

The client can receive a visual seed for:

- Parallax stars.
- Nebula background.
- Decorative dust.
- Non-gameplay particles.

The visual seed must not reveal gameplay POIs.

## Biome Generation

Biome can be derived from:

- Distance from origin.
- Low-frequency noise.
- Region modifiers.
- Live ops overrides.

Example:

```text
distance = sqrt(x*x + y*y)
noise = perlin(static_seed, x / biome_scale, y / biome_scale)
biome = classify(distance, noise)
```

Biome affects:

- Planet type weights.
- Planet level modifiers.
- Resource profiles.
- Scan interference.
- NPC threat.
- PvP risk.
- Visual background style.
- Event spawn tables.

Example biomes:

```text
Origin Belt:
  safer
  lower yield
  low-level planets

Outer Drift:
  moderate resource
  moderate threat

Nebula:
  harder scanning
  better rare materials
  radar interference

Deep Space:
  higher-level planets
  stronger NPCs
  better yield

Dead Zone:
  high danger
  weak radar
  rare anomalies
```

## Procedural Object Generation

For a given cell or chunk, the server calculates candidate objects using deterministic hashes.

Example:

```text
cell_x = floor(x / scan_cell_size)
cell_y = floor(y / scan_cell_size)

roll = hash(static_seed, cell_x, cell_y, "planet_candidate")
offset = hash(static_seed, cell_x, cell_y, "offset")
```

The offset places the object inside the cell so it does not always appear at the cell center.

For temporary objects:

```text
roll = hash(epoch_seed, cell_x, cell_y, "loot_cache")
offset = hash(epoch_seed, cell_x, cell_y, "offset")
object_id = hash(epoch_seed, cell_x, cell_y, local_index, object_type)
```

Generated objects must be filtered by:

- Active map bounds.
- Map profile and biome.
- Map profile level band.
- Spawn budget.
- Existing DB overlay.
- Loot cooldown state.
- Visibility and scan rules.

## Discovery Horizon

Superseded for scanner planets.

`DiscoveryHorizon` was the old infinite-plane gate. Bounded scanner planets now
use active map bounds, map profile density/spawn budget, map profile level band,
radar tier, scanner power, cooldown, and energy cost. Future expansion should
add or unlock maps rather than stretch an origin-distance horizon.

## Planet Generation

Planets are static procedural candidates until discovered.

Before discovery:

```text
planet exists as a deterministic candidate
not stored as full DB record
not visible to client
not tradeable
not claimable
```

After successful discovery:

```text
planet materializes into DB
planet gets persistent id
planet intel record is created for discoverer
planet can be shared, sold, claimed, or revisited
```

After claim:

```text
ownership becomes global
buildings/production become DB overlay
planet joins player/alliance network
```

## Planet Level Scaling

Superseded for scanner planets.

Planet level now comes from the active map profile level band. Rarity, biome, or
live-ops modifiers may tune inside that band only if they remain bounded by the
profile.

Example:

```text
planet_level = deterministic_roll(map_id, profile_version, cell, index)
planet_level = clamp(planet_level, profile.level_min, profile.level_max)
```

The map profile sets the expected level range. Noise and rarity can create
variation inside that range.

Rules:

- Harder maps can define higher level bands.
- Starter maps can keep rare planets low enough for early progression.
- Rare rolls can create standout planets inside a band.
- Biome modifies both value and difficulty.

Planet level affects:

- Minimum radar level.
- Scan difficulty.
- Player rank required to colonize.
- X Core tier required to claim.
- Energy production potential.
- Resource quality.
- Building cap.
- Defense baseline.
- NPC threat.
- PvP value.

## Planet Discovery Flow

Planets are hidden until discovered.

Discovery is scanner-driven and roll-based.

High-level flow:

```text
1. Player flies through space.
2. Server resolves the player's active map and map-local scan cell.
3. Server checks bounded static planet candidates for that active map/profile.
4. Player activates scanner utility.
5. Ship slows heavily or becomes stationary.
6. Scanner emits pulses every X seconds.
7. Each pulse performs server-side detection rolls.
8. If radar requirements and roll pass, a signal is revealed.
9. Further scan success can confirm the planet.
10. Confirmed planet is materialized into DB.
11. Player receives planet intel.
```

Scanner is an active utility skill/module.

Activation cost:

- Ship slows heavily, or
- Ship becomes stationary, and
- Capacitor/energy is consumed.

Phase 08 domain MVP gates scanner starts with server-owned state before any
cooldown or pulse record is created: the zone position provider must report a
stationary movement state, and a scanner energy provider must accept the
server-derived player, ship, pulse, time, and stat snapshot. Durable live energy
spend and world-worker slow-state leases remain runtime integration work.

Pulse behavior:

```text
scan_pulse_interval = every X seconds
each pulse = server-side roll
```

Chance can include:

```text
scanner_power
ship_explorer_bonus
radar_level
planet_signature
planet_level
distance_to_candidate
map_profile_interference
jammer_interference
live_ops_modifiers
```

Minimum requirement:

```text
if player_radar_level < planet_min_radar_level:
  cannot discover planet
```

If requirement is met:

```text
roll detection chance
```

This creates room for:

- Scout ships.
- Scanner modules.
- Radar progression.
- Jammers.
- Nebula interference.
- Rare low-signature planets.

## Scanner and Jammer Gameplay

### Scanner

Scanner is for discovery, target classification, and fog interaction.

Possible scanner actions:

- Short scan pulse.
- Deep scan channel.
- Planet survey.
- Resource sweep.
- Signature classify.

Tradeoffs:

- Slows/stops the ship.
- Consumes capacitor.
- Reveals player activity through scan waves.
- Can be interrupted by danger.

### Jammer

Jammer opposes scanner gameplay.

Possible jammer effects:

- Increases scan difficulty in radius.
- Masks ship signature.
- Creates false signals.
- Reduces enemy radar confidence.
- Protects hidden routes or frontier colonies.

Jammer should be server authoritative and visible as a tactical choice, not a client-only visual.

## Planet Claiming

Discovery does not grant ownership.

To colonize a planet:

```text
player_rank >= planet_level
required X Core item is consumed
player must be near the planet
claim action must complete successfully
```

`X Core` is a placeholder name for a rare claim resource.

Purpose of X Core:

- Prevents claim spam.
- Creates a rare item sink.
- Gives PvE/PvP/events valuable drops.
- Makes colonization a meaningful decision.
- Supports market demand.

Recommended claim flow:

```text
1. Player discovers planet.
2. Player travels to planet.
3. Server checks player rank.
4. Server checks X Core requirement.
5. Player starts claim channel.
6. Ship is slowed/stationary during claim.
7. If interrupted, claim fails or pauses.
8. On success, X Core is consumed.
9. Planet owner is set globally.
10. Planet enters persistent overlay.
```

MVP can simplify the channel, but final gameplay benefits from a risky claim window.

## Shared Planets, Personal Intel

Planets are global.

Planet knowledge is personal by default.

```text
Planet existence:
  global

Planet ownership:
  global

Planet intel:
  personal by default

Alliance intel:
  only through explicit share or later alliance systems
```

If Player A discovers a planet, Player B does not automatically know it.

If Player B later colonizes it, ownership changes globally. Player A's old intel becomes stale until refreshed.

## Sharing and Intel Trade

Players can share planet intel deliberately.

Initial scope:

- Share discovered planets only.
- Share to another player.
- Share to an alliance member.
- Daily or rate-limited sharing.

Later possible share types:

- Route.
- Anomaly.
- Resource field.
- Enemy structure.
- Wormhole signature.

Coordinate intel can become an item:

```text
Star Chart
Coordinate Scroll
Intel Packet
```

These can be:

- Sent through mail.
- Shared manually.
- Sold on the market.
- Consumed to add planet intel to fog memory.

If the planet changes ownership while an intel item is listed, the listing should become stale and be hidden from default market search until reverified.

## Temporary Loot and Cooldowns

Temporary loot can be procedural and epoch-based.

Example:

```text
object_id = hash(epoch_seed, cell_x, cell_y, local_index, object_type)
```

If a player loots it:

```text
loot_cooldown:{object_id} = consumed
```

Cooldown prevents another player from immediately looting the same procedural object.

Storage:

- Redis is fine for low-value temporary loot cooldowns.
- DB audit is needed for valuable/economy-impact loot.

Rules:

- If object is consumed, do not show it as loot.
- It may appear depleted, or disappear entirely.
- Cooldown/expiry should not outlive the epoch unless designed.

## Live Ops Layer

The universe is persistent, but live ops can add temporary layers.

Live ops examples:

- Loot waves.
- Frontier storms.
- NPC invasions.
- Rare scanner signatures.
- Temporary resource blooms.
- Holiday/event anomalies.
- PvP hotspot contracts.
- Wormhole instability windows.

Live ops should not rewrite the core permanent universe unless intentionally running a major world event.

Live ops uses:

```text
epoch_seed
event_config
event_region_filters
event_reward_tables
```

## Server Flow

When a player moves:

```text
1. Active map worker updates authoritative map-local position.
2. Worker determines nearby scan cells/chunks.
3. Worker loads DB overlay for relevant area.
4. Worker fetches or computes Redis procedural cache.
5. Worker applies current-map radar/stealth visibility rules.
6. Worker sends only allowed visible/intel data to client.
```

When a player scans:

```text
1. Client sends scanner activation intent.
2. Server resolves session, player, active map, and authoritative ship state.
3. Server rejects any non-empty scan payload before cooldown, energy, pulse,
   planet, intel, XP, or event mutation.
4. Server validates stationary or slow-scan state and capacitor availability.
5. Server starts cooldown and creates the pulse only after those gates pass.
6. Each pulse checks bounded map-local candidates and DB overlay.
7. Server rolls detection/discovery.
8. Successful discoveries materialize persistent records.
9. Client receives signal/planet intel updates.
```

When a player claims:

```text
1. Client sends claim intent.
2. Server checks planet exists and is discovered by player.
3. Server checks proximity.
4. Server checks player rank >= planet level.
5. Server checks X Core requirement.
6. Server starts claim channel.
7. On success, server consumes X Core and sets global owner.
8. DB overlay updates.
9. Relevant intel/notifications update.
```

## Client Rules

The client can render:

- Visual starfield.
- Nebula parallax.
- Decorative particles.
- UI panels.
- Server-approved known planet memory.
- Server-approved visible entities.
- Server-approved intel.

The client must not know:

- Static gameplay seed.
- Epoch gameplay seed.
- Hidden planet candidates.
- Hidden rare POIs.
- Hidden loot objects.
- Hidden NPC/player positions.
- Hidden map id, profile version, candidate key, scan cell, signature, level,
  or roll data.

All gameplay discovery is server-owned.

## Open Tuning Questions

These should be tuned through prototype testing:

- Exact scan cell size.
- Chunk size.
- Early ship speed.
- Radar radius.
- Scan pulse interval.
- Scan capacitor cost.
- Scan success rates.
- Planet density per map profile.
- Rare planet rate.
- X Core drop rate.
- Claim channel duration.
- Redis TTL strategy.
- Which temporary loot needs DB audit.

## First Prototype Scope

The first world prototype should prove:

```text
1. Bounded map-local movement.
2. Server-only procedural cells.
3. Colorful canvas with radar/minimap and simple POIs.
4. Static bounded planet candidate generation.
5. Active scanner pulse.
6. Roll-based planet discovery.
7. Planet materialization into DB.
8. Personal planet intel.
9. Rank + X Core claim validation.
10. Redis cache for computed cells.
11. Epoch-based temporary loot with cooldown.
```

This will validate the heart of the world before adding large content volume.
