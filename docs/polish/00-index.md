# Polish Analysis Index

Date: 2026-06-28

## Purpose

This folder captures a hard polish audit of why the current browser-first 2D
space MORPG does not yet feel close to DarkOrbit, even though the server
authority and vertical-slice foundations are real.

The core finding is not "assets are bad." The core finding is:

```text
The project proves correctness better than it delivers fantasy.
```

The current build is a credible server-authoritative vertical slice. It is not
yet a convincing DarkOrbit-like game because the first-session loop is too
sparse, too manual, too safe, too panel-driven, and too short on desire.

## Reports

1. [Executive Diagnosis](./01-executive-diagnosis.md)
2. [DarkOrbit Feel Acceptance Criteria](./02-darkorbit-feel-acceptance-criteria.md)
3. [Game Feel Gap Analysis](./03-game-feel-gap-analysis.md)
4. [Client HUD And Renderer Polish Review](./04-client-hud-renderer-polish-review.md)
5. [Server Authoritative Loop Review](./05-server-authoritative-loop-review.md)
6. [World Sector And Risk Review](./06-world-sector-risk-review.md)
7. [Progression Economy And Content Review](./07-progression-economy-content-review.md)
8. [Engineering And Process Review](./08-engineering-process-review.md)
9. [Polish Backlog](./09-polish-backlog.md)
10. [Kalaazu Reference Content Source](./10-kalaazu-reference-content-source.md)
11. [DarkOrbit Feel Browser Proof Review](./11-darkorbit-feel-browser-proof-review.md)

## Read This First

Start with:

```text
docs/polish/01-executive-diagnosis.md
docs/polish/02-darkorbit-feel-acceptance-criteria.md
docs/polish/09-polish-backlog.md
```

Then read the specialist reviews for evidence.

## Non-Negotiable Constraint

Do not fix the problem by faking a full game client.

The existing project rule still stands:

- no fake HP, cargo, wallet, planets, NPCs, loot, quests, or market data
- no hidden server truth serialized to the browser
- no client-authored damage, position, rewards, or economy facts

Polish must make real server-owned state feel better. It must not fake gameplay
fullness.

## Highest-Level Diagnosis

What is right:

- server-owned auth/session identity
- real authenticated WebSocket bootstrap
- strict command envelopes and forbidden trusted payload checks
- server-owned movement, AOI, fog/visibility, combat, loot, death, repair, and
  economy mutations
- Pixi world renderer with real state, no default fake gameplay values
- meaningful e2e proof that an authenticated loop can happen

What is wrong:

- combat is one-shot command driven, not a server-owned rhythmic engagement
- NPCs can chase, but the dangerous return-fire loop is not the center of play
- maps are too sparse to feel like living sectors
- the content ladder is too short to create upgrade hunger
- DarkOrbit-flavor systems are explicitly not started
- the HUD is functional but still reads like a web application/debug cockpit
- tests prove invariants and smoke paths, not whether a 20-minute session is fun

## Next Milestone Name

Use this as the next product milestone:

```text
Make the first 20 minutes feel like a dangerous, rewarding space MMO.
```

## Implementation Plan

The code-facing plan created from these findings lives here:

```text
docs/plans/2026-06-28-darkorbit-feel-design.md
docs/plans/2026-06-28-darkorbit-feel-implementation.md
```
