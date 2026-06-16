# Infinite Space Game - World System Summary

The game takes place in an infinite 2D space universe.

Everyone starts near the origin point, `0,0`. From there, players slowly push outward into unknown space. There is no fixed map border. The farther players travel, the deeper and more dangerous space becomes.

## The Main Idea

The universe is infinite, but civilization only exists where players have explored, discovered planets, built colonies, and connected them into networks.

Players do not simply open a map and see everything. Most of space is hidden by fog of war.

To progress, players must:

- Fly into unexplored space.
- Use scanners to search for signals.
- Discover planets, resources, anomalies, and threats.
- Colonize valuable planets.
- Build production and energy networks.
- Create warp/wormhole routes between planets.
- Trade or share planet coordinates with other players.

## Planets

Planets are not randomly created just for one player. They exist in a shared global universe.

If one player discovers a planet, another player can also find that same planet later. Someone else might even colonize it before the first player returns.

This makes exploration social and competitive.

Finding a planet gives knowledge, not ownership. To own it, a player must actually travel there and colonize it.

## Discovery and Scanning

Planets are hidden until discovered.

A player needs scanner modules to search an area. When scanning, the ship slows down or stops and sends scan pulses over time. Each pulse has a chance to detect hidden signals.

Higher-level planets require better radar or scanner levels. Deep space planets are harder to find, but more valuable.

This means exploration is not instant. Players have to travel, scan, take risks, and invest in better ships and modules.

## Infinite Scaling

The farther away from `0,0` a planet is, the higher its potential level becomes.

Near the center:

- Easier planets.
- Lower rewards.
- Safer space.

Farther out:

- Higher-level planets.
- Better resources.
- Stronger enemies.
- More PvP risk.
- Better long-term rewards.

The game can scale forever because planet levels and rewards grow with distance, but in a controlled way.

## Colonization

To colonize a planet, players need two things:

- Enough player rank.
- A rare item called `X Core` for now.

A planet has a level. The player must have at least that rank to claim it.

For example:

```text
Level 3 planet -> requires player rank 3+
Level 7 planet -> requires player rank 7+
```

The `X Core` item prevents players from claiming every planet they see. Colonization becomes a serious decision.

## Player Knowledge and Fog of War

The world is shared, but knowledge is personal.

If you discover a planet, it appears in your fog of war memory. Other players do not automatically know about it.

You can share planet information manually with another player or a clan member. This can reveal that planet in their own map memory.

This creates a real information economy.

## Coordinate Trading

Planet coordinates can become valuable items.

A scout might discover a rare planet but not be strong enough to colonize it. Instead, they can sell the coordinates to another player.

Possible items:

- Star chart.
- Coordinate scroll.
- Intel packet.

These items can be traded, mailed, or sold on the market.

If the planet gets colonized before someone buys the coordinate item, the intel can become stale or invalid.

## Procedural World

The server does not store the entire infinite map in the database.

Instead, the world is generated from hidden server seeds. As long as the seed stays the same, the same planets and objects can always be generated again.

Only important things are saved permanently:

- Discovered planets.
- Claimed planets.
- Planet ownership.
- Buildings.
- Production.
- Player intel.
- Wormhole routes.
- Marketed coordinate items.

This keeps the world infinite without needing to store everything.

## Daily and Live Events

The base universe is persistent, but live events can add temporary content on top of it.

Examples:

- Temporary loot waves.
- NPC invasions.
- Resource blooms.
- Rare anomalies.
- Special event zones.
- PvP hotspots.

These events make space feel alive without changing the permanent universe.

## Why This Is Cool

The game is not just about fighting.

It creates many roles:

- Explorers search for hidden planets.
- Miners gather resources.
- Traders move cargo.
- Scouts sell coordinates.
- Pirates hunt trade routes.
- Clans build planet networks.
- Fighters defend valuable frontier worlds.

The core loop is:

```text
Explore -> Scan -> Discover -> Colonize -> Build -> Trade -> Expand
```

The deeper players go into space, the more dangerous and valuable the universe becomes.

In short:

```text
Build your own galaxy by pushing into the unknown.
```

