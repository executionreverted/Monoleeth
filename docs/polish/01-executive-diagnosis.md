# Executive Diagnosis

Date: 2026-06-28

## Short Verdict

The disappointment is valid.

The current game has become "something," and a lot of the hard engineering is
real, but it does not yet feel like DarkOrbit. The reason is not mainly art or
asset quality. The reason is that the core DarkOrbit feeling is a stack of
systems that are still missing or too thin:

- continuous target lock combat
- rhythmic auto-fire and ammo/rocket decisions
- NPC retaliation and danger pressure
- dense sector ecology
- clear risk/reward maps
- loot/resource anticipation
- long equipment chase
- honor/rank/social pressure
- gate or wave-style repeatable goals

The repo currently has a strong server-authoritative scaffold, but the play loop
often feels like issuing correct commands to a real backend rather than living
inside a dangerous space MMO.

## What The Project Got Right

The architecture is not unserious. Several decisions are correct and should be
kept:

- The UI roadmap explicitly bans fake gameplay state and requires authenticated
  server state: `docs/plans/ui-implementation/00-index.md`.
- The browser client uses real auth/session state, with no default fake HP,
  wallet, cargo, planets, or NPCs: `client/src/state/reducer.ts`.
- WebSocket identity is server-resolved, not trusted from client command
  payloads: `internal/game/realtime/gateway.go`,
  `internal/game/server/transport.go`.
- Movement is server-owned and client-interpolated:
  `internal/game/world/movement.go`,
  `client/src/state/movement.ts`.
- Combat use validates visibility, range, cooldown, energy, PvP policy, kill,
  XP, and loot server-side: `internal/game/server/combat_loot_repair.go`.
- AOI/visibility filtering is a real hidden-data boundary:
  `internal/game/server/runtime_world_snapshot.go`,
  `internal/game/world/aoi/snapshot.go`.
- The e2e playtest drives a real authenticated browser/server loop:
  `client/tests/e2e/playtest-server-flow.mjs`.

This means the answer is not "throw away server authority." The answer is "make
server authority own a more alive game loop."

## What Is Most Wrong

### 1. Combat Is A Command, Not A State

The current combat model is too close to:

```text
select target -> click Fire -> server resolves one basic laser shot
```

DarkOrbit's feel is closer to:

```text
lock target -> enter attack stance -> server ticks weapon cadence -> player
moves/kites/changes ammo/uses rockets/escapes while combat continues
```

Evidence:

- `client/src/app/client-app-commands.ts` sends one `combat.use_skill`.
- `internal/game/combat/service.go` exposes `ExecuteBasicAttack`.
- `client/src/ui/hud-render-panels.ts` has one live laser slot while rocket and
  warp are locked.
- There is no public active-target/auto-attack contract such as
  `combat.set_target`, `combat.set_auto_attack`, `combat.started`, or
  `combat.stopped`.

Impact:

- The player feels like they are pressing a backend endpoint, not fighting.
- Combat rhythm depends on manual UI cadence instead of a live server cadence.
- Cooldowns are visible, but not embodied as sustained combat pressure.

### 2. NPCs Do Not Yet Create Enough Fear

The worker has aggro/leash behavior, but the current reviewed path centers on
NPC movement/chase, not a strong NPC offensive damage loop.

Evidence:

- `internal/game/world/worker/enemy_aggro.go` moves hostile NPCs toward targets.
- `docs/map-rework/phase-08-enemy-pools-spawners-ecs.md` explicitly deferred
  public combat events / richer combat behavior in its landed slices.
- Starter and `1-2` pools are intentionally passive in map content.

Impact:

- A hostile contact can look like a moving marker rather than a threat.
- The player's shield/hull does not become an always-present emotional surface.
- Safe maps feel correct, but early danger is too delayed and too thin.

### 3. The Universe Is Too Sparse

The bounded map architecture is good, but the seeded universe is far too empty
for the target fantasy.

Evidence:

- Starter world is currently a proof set around public `1-1`, `1-2`, and `1-3`.
- Enemy pools and e2e proofs focus on one or two deterministic NPCs per map.
- `docs/playtest-vertical-slice-status.md` honestly calls the current state a
  test-server readiness snapshot, not a completion claim.

Impact:

- A 10000x10000 sector with one or two meaningful contacts feels like a debug
  map.
- Gates, planets, loot, and NPCs are technically real but not dense enough to
  create "where do I go next?" pressure.

### 4. The Progression Spine Is Too Short

The backend supports catalogs, ledgers, inventories, crafting, shops, auctions,
quests, production, and routes. But the authored ladder is too small.

Evidence:

- `docs/road-to-v1/11-first-endgame-signal-gate.md` is not started.
- `docs/road-to-v1/12-darkorbit-flavor.md` is not started.
- The starter balance currently has four recognizable seeded ships:
  Phoenix, Goliath K2, Vengeance, Bigboy.
- MVP module/recipe/loot breadth is still low compared to a DarkOrbit-style
  chase.

Impact:

- Players can prove systems work but do not feel a long equipment hunger.
- Shops and deterministic rewards flatten anticipation.
- The game lacks a strong "one more kill, one more drop, one more upgrade" loop.

### 5. The HUD Looks Correct-ish, But Feels Like A Tool

The mockup target is dense and alive. The implementation has many pieces of it,
but the composition still reads like a server-backed web cockpit.

Evidence:

- `output/mockups/final-mockup.png` has dense sector objects, planets, selected
  panel, minimap, action bar, log, and top status in one strong combat screen.
- `client/src/ui/hud-render-shell.ts` puts Stop, Sync, Mail, Chat, Social, and
  Logout in prime topbar space.
- `client/src/ui/hud-render-panels.ts` has locked rocket/warp slots and many
  explanatory empty states.
- `client/src/render/world-renderer-sprites.ts` uses low alpha/scale values for
  primary sprites.

Impact:

- The player sees a functional UI, not a combat cockpit.
- Empty/locked states are honest but emotionally dead.
- Feedback shows events but does not yet punch.

## What Not To Do

Do not solve this by:

- adding fake NPCs to the client
- making fake wallet/cargo/loot values
- hiding the problem behind prettier static art
- weakening server validation so combat feels faster
- adding many more backend surfaces before the first 20 minutes feel good

## What To Do First

The highest-impact polish direction is:

```text
server-owned continuous combat + NPC return fire + denser first sector +
short early upgrade ladder + stronger HUD/game-feel feedback
```

That can be sliced without violating server authority:

1. Add server-owned target/attack stance.
2. Add NPC return-fire through the same combat rules.
3. Fill one starter-to-risk map path with more real NPCs, loot, signals, and
   resource reasons.
4. Add a 3-5 hour early progression contract.
5. Add ammo/rocket/honor/drone-lite as narrow DarkOrbit-flavor slices.
6. Make the HUD serve combat first, admin/account utilities second.

## Bottom Line

This project is not doomed. It has more real backend discipline than many game
prototypes.

But the current build is still architecture-first. DarkOrbit feeling will not
emerge automatically from correctness. It needs to become a named product goal
with acceptance criteria, playtest evidence, and dedicated polish phases.

