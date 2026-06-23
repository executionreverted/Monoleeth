# Phase 09 - Quest Board Game Layout

## Goal

Make Quests a real selectable game board instead of a single generic card or a
renamed Galaxy panel. The surface should expose server-owned offers, active
quests, objective progress, rewards, accept/claim/reroll actions, and clean
locked/empty states without fake counts or client-authored progress.

## Problems Covered

- Task 001 goal promises Quests as a game system, but no previous phase owned
  it.
- The old Galaxy-style menu does not behave like a quest board.
- Quest selection, objectives, rewards, and available actions need a real
  list/detail/action layout.
- Progression unlocks and quest reward claims must not become client-authored.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
docs/todo.md
docs/plans/modules/01-player-progression-rank-role-skills.md
docs/plans/modules/10-quest-board-generation.md
docs/plans/modules/15-api-events-errors.md
docs/plans/modules/16-testing-observability-balancing.md
client/src/ui/hud.ts
client/src/state/reducer.ts
client/src/protocol/commands.ts
internal/game/server/quest_admin_observability_handlers.go
internal/game/quest/
```

## Design Contract

- Window label is `Quests`.
- Layout is list/detail/action:
  - left: selectable quest categories and rows
  - center: selected quest objectives and progress
  - right/footer: rewards and allowed actions
- Categories: available offers, active quests, claimable quests, completed
  quests, reroll/board state where supported.
- Objective progress and rewards come from server snapshots/events only.
- Accept, claim, and reroll are real contracts or absent/locked with player
  game copy.
- Progression unlocks and skill/rank effects remain server-owned. The UI can
  show requirements and rewards but cannot submit completion truth.
- Reward claim events must either carry safe updated snapshots or trigger
  explicit refreshes for quest board, wallet, progression, inventory/cargo, and
  rank. Visible balances update only from server truth.

## Implementation Plan

1. Build the board model.
   - Define or verify quest snapshot/query payloads for board offers, active
     quests, claimable rewards, completed state, reroll cooldown/cost, and
     selected detail.
   - Keep client state limited to selected row/category.

2. Replace generic panel structure.
   - Remove any Galaxy wording from the normal player HUD.
   - Render quest rows with stable ids, status, objective summary, and reward
     preview.
   - Render selected detail with objective progress bars/rows and rewards.

3. Wire real actions.
   - Accept uses server-owned availability and idempotency.
   - Claim uses server reward state machine and duplicate-claim protection.
   - Reroll uses server cooldown/cost/rate-limit or remains hidden.
   - `progression.unlock_skill` remains blocked unless implemented with a real
     server contract in this phase.

4. Reconcile events.
   - Quest accept/progress/claim/reroll responses update board state.
   - Passive quest events either carry safe payloads or trigger refresh queries.
   - Reward grants reconcile wallet, XP, inventory/cargo, rank, and quest state.
   - `quest.reward_claimed` must use the domain idempotency key
     `quest_reward:<player_quest_id>` and duplicate events/requests must not
     duplicate visible value.
   - Public quest payloads must support `objectives[]`; multi-objective quests
     cannot collapse into a single generic progress row.

5. Add tests.
   - Selection and tab/category switching.
   - Accept/claim/reroll happy paths and locked paths.
   - Duplicate reward claim rejection/idempotency.
   - No client-authored objective progress.

## Likely Files

```text
internal/game/server/quest_admin_observability_handlers.go
internal/game/server/server_test.go
internal/game/realtime/envelope.go
internal/game/realtime/envelope_test.go
internal/game/quest/
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/src/state/types.ts
client/src/state/reducer.ts
client/src/ui/hud.ts
client/src/styles.css
client/tests/browser-smoke.mjs
docs/plans/task-001/09-quest-board-game-layout.md
```

## Acceptance Criteria

- [ ] Quests has an owning Task 001 phase and visible board layout.
- [ ] The menu label and window title are `Quests`, not `Galaxy`.
- [ ] Quest rows are selectable and show server-owned state.
- [ ] Selected quest detail shows objectives, progress, and rewards.
- [ ] Accept, claim, and reroll actions are real or absent/locked with game copy.
- [ ] Reward claim is idempotent and cannot duplicate XP/currency/items.
- [ ] `quest.reward_claimed` carries safe updated snapshots or triggers
      explicit quest board, wallet, progression, inventory/cargo, and rank
      refreshes.
- [ ] Visible reward balances update only from server snapshots/responses, not
      from client-estimated rewards.
- [ ] Client cannot author quest completion/progress.
- [ ] Passive quest events reconcile or trigger explicit refresh.
- [ ] Multi-objective quest payloads render as multiple objective rows with
      server-owned progress.
- [ ] Browser smoke captures quest board screenshots under
      `output/screenshots/task-001/09/` or the final Task 001 screenshot set.

## Verification

```bash
go test ./internal/game/server -run 'Test.*(Quest|Progression|Reward|Reroll)' -count=1
go test ./internal/game/realtime -run 'TestOperationRegistry' -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/protocol src/state
npm --cache /tmp/gameproject-npm-cache run smoke
```
