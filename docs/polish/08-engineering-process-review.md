# Engineering And Process Review

Date: 2026-06-28

## Verdict

The project is not failing because the engineering is weak.

It is failing the DarkOrbit feel because the workflow has optimized for
checklist closure and safety proofs faster than it has optimized for coherent
player fantasy, dense map life, progression desire, and repeatable loops.

The current build is a credible server-authoritative vertical slice. It is not a
convincing DarkOrbit-like game yet.

## What Helps

### Strong Server-Authority Rules

The UI implementation plan bans fake gameplay state and hidden-truth leaks.
That is exactly right.

Evidence:

- `docs/plans/ui-implementation/00-index.md`
- `AGENTS.md`

### Transport And Protocol Hardening

The project has invested in:

- authenticated WebSocket session resolution
- forbidden trusted payload keys
- request envelopes
- event sequencing/replay posture
- bundle/leak scans
- e2e WebSocket frame canaries

Evidence:

- `internal/game/realtime`
- `internal/game/server/transport.go`
- `client/src/protocol/envelope.ts`
- `client/tests/e2e`

### Test Breadth

The repo has unusually high automated test breadth for a game prototype:

- hundreds of Go test files
- dozens of client unit tests
- multiple Playwright/browser e2e scripts
- artifact and bundle scan scripts

This is a real asset.

### Honest Playtest Status

`docs/playtest-vertical-slice-status.md` is appropriately cautious. It calls the
current state a status snapshot, not a completion claim.

That tone should become the standard.

## Where Process Hurts The Game

### 1. "Done" Sometimes Means Contract Done, Not Feel Done

Some phase docs can read as complete while central player-feel gaps remain.

Example pattern:

```text
phase checklist closed -> remaining work moved to docs/todo.md -> player still
does not feel the loop
```

This is dangerous for a game. A backend contract can be complete while the
feature is not emotionally complete.

Use separate statuses:

- contract complete
- vertical slice complete
- feel incomplete
- production incomplete
- polish complete

### 2. Large Coordinators Slow Iteration

Several files are large enough to indicate coordinator pressure:

- `internal/game/server/runtime.go`
- `internal/game/market/service.go`
- `client/src/ui/hud.ts`
- `client/src/ui/hud-render-panels.ts`
- large e2e scripts

Large coordinators are not automatically wrong, but DarkOrbit feel requires
rapid iteration on:

- combat cadence
- UI feedback
- encounter density
- content tables
- reward pacing

If those loops require touching a giant runtime/HUD coordinator every time, the
project will keep shipping correctness slower than feel.

### 3. Tests Prove Safety More Than Fun

Current tests are good at proving:

- no fake data
- no hidden leak
- command payload safety
- server-owned mutation
- e2e path can happen

They are weaker at proving:

- first 20 minutes are compelling
- sector feels populated
- combat decisions matter
- progression creates desire
- a player knows what they want next

Games need both.

### 4. Dev Mode Can Hide Product Feel

Playtest scripts often use:

- `GAME_DEV_MODE=true`
- static content
- process-local stores
- deterministic seeds
- test accounts

That is fine for verification, but product-feel reviews need a fresh account
path with minimal shortcuts.

## Process Recommendations

### 1. Create Feel Gates Beside Safety Gates

Every major gameplay polish phase should answer:

- What does the player want next?
- What creates danger?
- What creates reward anticipation?
- What changed in the first 20 minutes?
- What was tested in real browser/server state?

### 2. Write Product Loop Specs Before More Systems

Before coding more broad systems, write small loop contracts:

- starter combat loop
- first upgrade loop
- first risky map loop
- first mini-gate loop
- first PvP/honor loop

Each loop should list:

- server contracts
- UI surfaces
- content rows
- balance targets
- screenshots/e2e proofs
- feel acceptance checks

### 3. Split By Player-Facing Loop

When decomposing code, split ownership around loops, not only technical domains:

- combat engagement loop
- navigation/world loop
- loot/reward loop
- progression/equipment loop
- social/risk loop
- admin/content loop

### 4. Treat Mockup Parity As Experience, Not Pixels

The mockup matters because it expresses:

- density
- threat
- readable objects
- fast actions
- map awareness
- operational fantasy

Do not reduce it to borders/colors. Verify whether the live screen creates the
same decision pressure.

### 5. Add Playtest Realism Gates

Before public test-server claims:

- fresh account
- no fake state
- durable mode where possible
- 10-minute scripted session
- screenshot/video artifacts
- written feel notes
- no dev shortcut reliance in the product claim

## Bottom Line

Keep the engineering discipline. It is one of the best parts of the project.

But add product-feel discipline beside it. The next milestone should not be a
technical checklist. It should be:

```text
Make the first 20 minutes feel like a dangerous, rewarding space MMO.
```

